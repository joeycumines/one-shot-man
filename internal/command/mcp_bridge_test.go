package command

import (
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMcpBridgeCommand_BadArgs(t *testing.T) {
	t.Parallel()

	cmd := NewMcpBridgeCommand()

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "no args", args: nil, wantErr: "expected 2 arguments, got 0"},
		{name: "one arg", args: []string{"unix"}, wantErr: "expected 2 arguments, got 1"},
		{name: "three args", args: []string{"unix", "/tmp/x", "extra"}, wantErr: "expected 2 arguments, got 3"},
		{name: "bad network", args: []string{"udp", "/tmp/x"}, wantErr: "unsupported network"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			err := cmd.Execute(tc.args, &stdout, &stderr)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestMcpBridgeCommand_ConnectFail(t *testing.T) {
	t.Parallel()

	cmd := NewMcpBridgeCommand()

	// Try connecting to a non-existent path.
	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"unix", "/tmp/osm-nonexistent-test-socket-" + t.Name()}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "connect") {
		t.Errorf("error %q should mention 'connect'", err.Error())
	}
}

func TestMcpBridgeCommand_BidirectionalCopy_TCP(t *testing.T) {
	t.Parallel()

	// Create a TCP listener that echoes data back.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	// Echo server: read all, send it back.
	go func() {
		conn, aErr := ln.Accept()
		if aErr != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()

	// Pipe stdin data into the bridge.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	// Write test data, then close write end to signal EOF.
	testData := "hello mcp bridge\n"
	go func() {
		_, _ = w.WriteString(testData)
		_ = w.Close()
	}()

	var stdout bytes.Buffer
	cmd := NewMcpBridgeCommand()
	cmd.Stdin = r
	err = cmd.Execute([]string{"tcp", ln.Addr().String()}, &stdout, io.Discard)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	got := stdout.String()
	if got != testData {
		t.Errorf("echoed data = %q, want %q", got, testData)
	}
}

func TestMcpBridgeCommand_BidirectionalCopy_Unix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix domain sockets not available on Windows")
	}
	t.Parallel()

	// Use a short socket path to stay within macOS 104-byte UDS limit.
	sockPath := filepath.Join("/tmp", "osm-bridge-test-"+t.Name()+".sock")
	// Sanitise: test names may contain '/' from subtests.
	sockPath = strings.ReplaceAll(sockPath, "/", "-")
	sockPath = "/tmp/" + filepath.Base(sockPath)
	os.Remove(sockPath) // remove stale socket from previous runs
	t.Cleanup(func() { os.Remove(sockPath) })

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	// Echo server.
	go func() {
		conn, aErr := ln.Accept()
		if aErr != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	testData := "unix bridge test data\n"
	go func() {
		_, _ = w.WriteString(testData)
		_ = w.Close()
	}()

	var stdout bytes.Buffer
	cmd := NewMcpBridgeCommand()
	cmd.Stdin = r
	err = cmd.Execute([]string{"unix", sockPath}, &stdout, io.Discard)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	got := stdout.String()
	if got != testData {
		t.Errorf("echoed data = %q, want %q", got, testData)
	}
}

func TestMcpBridgeCommand_LargePayload(t *testing.T) {
	t.Parallel()

	// Test with a payload larger than typical buffer sizes to verify
	// the bridge handles partial reads/writes correctly.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, aErr := ln.Accept()
		if aErr != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	// 256 KB payload
	payload := strings.Repeat("A", 256*1024) + "\n"
	go func() {
		_, _ = w.WriteString(payload)
		_ = w.Close()
	}()

	var stdout bytes.Buffer
	cmd := NewMcpBridgeCommand()
	cmd.Stdin = r
	err = cmd.Execute([]string{"tcp", ln.Addr().String()}, &stdout, io.Discard)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if stdout.Len() != len(payload) {
		t.Errorf("payload size = %d, want %d", stdout.Len(), len(payload))
	}
}

func TestMcpBridgeCommand_ServerClosesFirst(t *testing.T) {
	t.Parallel()

	// Server sends data then closes — bridge should receive it all and exit.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	serverMsg := "server says hello\n"
	go func() {
		conn, aErr := ln.Accept()
		if aErr != nil {
			return
		}
		_, _ = conn.Write([]byte(serverMsg))
		_ = conn.Close()
	}()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	// Don't write anything, just close stdin after a short delay.
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = w.Close()
	}()

	var stdout bytes.Buffer
	cmd := NewMcpBridgeCommand()
	cmd.Stdin = r

	// Run with timeout to prevent hangs.
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute([]string{"tcp", ln.Addr().String()}, &stdout, io.Discard)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("bridge did not exit within 5s")
	}

	got := stdout.String()
	if got != serverMsg {
		t.Errorf("got %q, want %q", got, serverMsg)
	}
}

func TestMcpBridgeCommand_Name(t *testing.T) {
	t.Parallel()
	cmd := NewMcpBridgeCommand()
	if cmd.Name() != "mcp-bridge" {
		t.Errorf("name = %q, want %q", cmd.Name(), "mcp-bridge")
	}
}

func TestCloseWrite(t *testing.T) {
	t.Parallel()

	// Verify closeWrite works on TCP connections (has CloseWrite method).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, _ := ln.Accept()
		if conn != nil {
			defer conn.Close()
			// Read until EOF (triggered by CloseWrite on client side).
			buf := make([]byte, 1024)
			_, _ = conn.Read(buf)
		}
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Should not panic.
	closeWrite(conn)
	wg.Wait()
}
