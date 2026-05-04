package ptyio

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/creack/pty"
)

func skipPTYTest(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping slow PTY integration test")
	}
	if runtime.GOOS == "windows" {
		t.Skip("skipping PTY test on Windows (creack/pty not supported)")
	}
}

func TestIntegration_BufferedReader_RealPTY(t *testing.T) {
	skipPTYTest(t)
	t.Parallel()

	// Spawn a real process that writes "hello" and keeps the slave PTY
	// open briefly so ReadLoop has time to read before EIO. Without the
	// trailing sleep, on macOS the slave can close before the master
	// side delivers data, causing Read to return EIO with 0 bytes.
	cmd := exec.Command("sh", "-c", "echo hello && sleep 0.1")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatalf("pty.Start: %v", err)
	}
	defer ptmx.Close()

	br := NewBufferedReader(ptmx, 8)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go br.ReadLoop(ctx)

	// Collect all output until the reader is done.
	var got bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		for chunk := range br.Output() {
			got.Write(chunk)
		}
	}()

	// Wait for reader to finish (process exits → PTY EOF).
	select {
	case <-br.done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for reader to finish")
	}

	// Wait for collector goroutine.
	<-done

	// Verify output contains "hello". The PTY may add \r\n on some systems.
	if !bytes.Contains(got.Bytes(), []byte("hello")) {
		t.Errorf("output = %q; want to contain %q", got.String(), "hello")
	}

	// Wait for the process to exit and clean up.
	if err := cmd.Wait(); err != nil {
		// Ignore exit error — PTY processes may return spurious errors.
		t.Logf("cmd.Wait: %v (expected for PTY)", err)
	}
}

func TestIntegration_BufferedReader_RealPTY_LargeOutput(t *testing.T) {
	skipPTYTest(t)
	t.Parallel()

	// Spawn a process that generates substantial output.
	// `seq 1 1000` produces ~4KB of output.
	cmd := exec.Command("seq", "1", "1000")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatalf("pty.Start: %v", err)
	}
	defer ptmx.Close()

	br := NewBufferedReader(ptmx, 16)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go br.ReadLoop(ctx)

	var got bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		for chunk := range br.Output() {
			got.Write(chunk)
		}
	}()

	select {
	case <-br.done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for reader to finish")
	}
	<-done

	output := got.String()
	// Verify first and last numbers are present.
	if !bytes.Contains(got.Bytes(), []byte("1")) {
		t.Error("output missing '1'")
	}
	if !bytes.Contains(got.Bytes(), []byte("1000")) {
		t.Errorf("output missing '1000'; got %d bytes", len(output))
	}

	if err := cmd.Wait(); err != nil {
		t.Logf("cmd.Wait: %v (expected for PTY)", err)
	}
}

func TestIntegration_BufferedReader_RealPTY_ProcessExitClosesDone(t *testing.T) {
	skipPTYTest(t)
	t.Parallel()

	// Spawn a fast-exiting process.
	cmd := exec.Command("true")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatalf("pty.Start: %v", err)
	}
	defer ptmx.Close()

	br := NewBufferedReader(ptmx, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go br.ReadLoop(ctx)

	// The done channel should close after the process exits.
	select {
	case <-br.done:
		// Success: done channel closed.
	case <-ctx.Done():
		t.Fatal("timed out waiting for done channel to close after process exit")
	}

	if err := cmd.Wait(); err != nil {
		t.Logf("cmd.Wait: %v (expected for PTY)", err)
	}
}
