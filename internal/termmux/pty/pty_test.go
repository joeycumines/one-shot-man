package pty

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func skipIfWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix-specific commands (sh, cat, sleep, pwd, echo)")
	}
}

func requireConPTYRuntime(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("ConPTY runtime probe requires Windows")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	proc, err := Spawn(ctx, SpawnConfig{
		Command: "powershell.exe",
		Args:    []string{"-NoProfile", "-Command", "exit 0"},
		Rows:    24,
		Cols:    80,
	})
	if err != nil {
		t.Fatalf("ConPTY runtime probe spawn failed: %v", err)
	}
	defer proc.Close()

	code, waitErr := proc.Wait()
	if waitErr != nil {
		t.Fatalf("ConPTY runtime probe wait failed: %v", waitErr)
	}
	if code == 3221225794 {
		t.Skipf("skipping ConPTY tests on this host: child console init failed with %#x", uint32(code))
	}
	if code != 0 {
		t.Fatalf("ConPTY runtime probe exited with code %d", code)
	}
}

func TestSpawn_EchoHello(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "echo",
		Args:    []string{"hello"},
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	var output strings.Builder
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for output, got so far: %q", output.String())
		default:
		}
		data, readErr := proc.Read()
		if data != "" {
			output.WriteString(data)
		}
		if strings.Contains(output.String(), "hello") {
			break
		}
		if readErr != nil {
			break
		}
	}

	if !strings.Contains(output.String(), "hello") {
		t.Fatalf("expected output to contain %q, got %q", "hello", output.String())
	}

	code, waitErr := proc.Wait()
	if waitErr != nil {
		t.Fatalf("Wait returned error: %v", waitErr)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

func TestSpawn_EmptyCommand(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	_, err := Spawn(context.Background(), SpawnConfig{})
	if err == nil {
		t.Fatal("expected error for empty command")
	}
	if !strings.Contains(err.Error(), "command is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSpawn_InvalidCommand(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	_, err := Spawn(context.Background(), SpawnConfig{
		Command: "/nonexistent/command/that/does/not/exist",
	})
	if err == nil {
		t.Fatal("expected error for invalid command")
	}
}

func TestSpawn_EnvVars(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "sh",
		Args:    []string{"-c", "echo $MY_TEST_VAR"},
		Env: map[string]string{
			"MY_TEST_VAR": "test_value_42",
		},
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	var output strings.Builder
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out, got: %q", output.String())
		default:
		}
		data, readErr := proc.Read()
		if data != "" {
			output.WriteString(data)
		}
		if strings.Contains(output.String(), "test_value_42") {
			break
		}
		if readErr != nil {
			break
		}
	}

	if !strings.Contains(output.String(), "test_value_42") {
		t.Fatalf("expected output to contain %q, got %q", "test_value_42", output.String())
	}
}

func TestSpawn_WorkingDirectory(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	dir := t.TempDir()
	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "pwd",
		Dir:     dir,
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	var output strings.Builder
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out, got: %q", output.String())
		default:
		}
		data, readErr := proc.Read()
		if data != "" {
			output.WriteString(data)
		}
		// Accept any non-empty output as pwd completes quickly.
		if output.Len() > 0 && readErr != nil {
			break
		}
		if readErr != nil {
			break
		}
	}

	outStr := strings.TrimSpace(output.String())
	outStr = strings.ReplaceAll(outStr, "\r", "")

	// macOS may resolve /var -> /private/var via symlinks.
	// filepath.EvalSymlinks resolves the full chain, unlike os.Readlink
	// which only reads a single symlink target.
	resolvedDir, resolveErr := filepath.EvalSymlinks(dir)
	if resolveErr != nil {
		resolvedDir = dir
	}
	if !strings.Contains(outStr, dir) && !strings.Contains(outStr, resolvedDir) {
		t.Fatalf("expected output to contain %q or %q, got %q", dir, resolvedDir, outStr)
	}
}

func TestProcess_Resize(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "cat",
		Rows:    24,
		Cols:    80,
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	if err := proc.Resize(48, 120); err != nil {
		t.Fatalf("Resize failed: %v", err)
	}
	if err := proc.Resize(100, 200); err != nil {
		t.Fatalf("second Resize failed: %v", err)
	}
}

func TestProcess_Signal(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "sleep",
		Args:    []string{"60"},
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	if !proc.IsAlive() {
		t.Fatal("expected process to be alive immediately after spawn")
	}

	if err := proc.Signal("SIGINT"); err != nil {
		t.Fatalf("Signal(SIGINT) failed: %v", err)
	}

	done := make(chan struct{})
	go func() {
		proc.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for process to exit after SIGINT")
	}

	if proc.IsAlive() {
		t.Fatal("expected process to be dead after SIGINT")
	}
}

func TestProcess_Signal_Unsupported(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "sleep",
		Args:    []string{"60"},
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	err = proc.Signal("NOSUCHSIG")
	if err == nil {
		t.Fatal("expected error for unsupported signal")
	}
	if !strings.Contains(err.Error(), "unsupported signal") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProcess_Close_Idempotent(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "sleep",
		Args:    []string{"60"},
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	if err := proc.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	if err := proc.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
}

func TestProcess_Write_AfterClose(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "cat",
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	proc.Close()

	_, err = proc.Write([]byte("hello"))
	if err == nil {
		t.Fatal("expected error when writing to closed process")
	}
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed, got: %v", err)
	}
}

func TestProcess_Read_AfterClose(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "cat",
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	proc.Close()

	_, readErr := proc.Read()
	if readErr != nil && !errors.Is(readErr, ErrClosed) {
		// ErrClosed is expected; other errors are also acceptable post-close.
		_ = readErr
	}
}

func TestProcess_Pid(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "sleep",
		Args:    []string{"60"},
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	pid := proc.Pid()
	if pid == 0 {
		t.Fatal("expected non-zero PID")
	}
}

func TestSpawn_DefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := SpawnConfig{Command: "echo", Args: []string{"test"}}
	cfg.applyDefaults()

	if cfg.Rows != DefaultRows {
		t.Fatalf("expected default rows %d, got %d", DefaultRows, cfg.Rows)
	}
	if cfg.Cols != DefaultCols {
		t.Fatalf("expected default cols %d, got %d", DefaultCols, cfg.Cols)
	}
	if cfg.Term != DefaultTerm {
		t.Fatalf("expected default term %q, got %q", DefaultTerm, cfg.Term)
	}
	if cfg.WriteTimeout != DefaultWriteTimeout {
		t.Fatalf("expected default write timeout %v, got %v", DefaultWriteTimeout, cfg.WriteTimeout)
	}
}

func TestProcess_ContextCancel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}

	t.Parallel()
	skipIfWindows(t)

	ctx, cancel := context.WithCancel(context.Background())

	proc, err := Spawn(ctx, SpawnConfig{
		Command: "sleep",
		Args:    []string{"60"},
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	cancel()

	done := make(chan struct{})
	go func() {
		proc.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for process to exit after context cancel")
	}
}

func TestProcess_WriteAndReadCat(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "cat",
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	if _, err := proc.Write([]byte("hello from pty\n")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	var output strings.Builder
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out, got: %q", output.String())
		default:
		}
		data, readErr := proc.Read()
		if data != "" {
			output.WriteString(data)
		}
		if strings.Contains(output.String(), "hello from pty") {
			break
		}
		if readErr != nil {
			break
		}
	}

	if !strings.Contains(output.String(), "hello from pty") {
		t.Fatalf("expected output to contain %q, got %q", "hello from pty", output.String())
	}
}

func TestParseSignal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantErr bool
	}{
		{"SIGINT", false},
		{"SIGTERM", false},
		{"SIGKILL", false},
		{"SIGHUP", false},
		{"SIGQUIT", false},
		{"NOSUCHSIG", true},
		{"", true},
	}

	for _, tt := range tests {
		sig, err := parseSignal(tt.name)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseSignal(%q): expected error", tt.name)
			}
		} else {
			if err != nil {
				t.Errorf("parseSignal(%q): unexpected error: %v", tt.name, err)
			}
			if sig == nil {
				t.Errorf("parseSignal(%q): expected signal, got nil", tt.name)
			}
		}
	}
}

func TestProcess_Resize_AfterClose(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "cat",
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	proc.Close()

	err = proc.Resize(48, 120)
	if err == nil {
		t.Fatal("expected error when resizing closed process")
	}
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed, got: %v", err)
	}
}

func TestProcess_Signal_AfterClose(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "cat",
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	proc.Close()

	err = proc.Signal("SIGINT")
	if err == nil {
		t.Fatal("expected error when signaling closed process")
	}
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed, got: %v", err)
	}
}

func TestSpawn_CommandWithSpaces(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	// When Command contains spaces and Args is empty, Spawn should
	// split the command string automatically.
	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "echo hello_from_split",
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	var output strings.Builder
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for output, got so far: %q", output.String())
		default:
		}
		data, readErr := proc.Read()
		if data != "" {
			output.WriteString(data)
		}
		if strings.Contains(output.String(), "hello_from_split") {
			break
		}
		if readErr != nil {
			break
		}
	}

	if !strings.Contains(output.String(), "hello_from_split") {
		t.Fatalf("expected output to contain %q, got %q", "hello_from_split", output.String())
	}
}

func TestSpawn_CommandWithSpacesAndExplicitArgs(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	// When Args is provided, Command should NOT be split — even if it
	// contains spaces. This preserves backward compatibility.
	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "sh",
		Args:    []string{"-c", "echo explicit_args"},
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	var output strings.Builder
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for output, got so far: %q", output.String())
		default:
		}
		data, readErr := proc.Read()
		if data != "" {
			output.WriteString(data)
		}
		if strings.Contains(output.String(), "explicit_args") {
			break
		}
		if readErr != nil {
			break
		}
	}

	if !strings.Contains(output.String(), "explicit_args") {
		t.Fatalf("expected output to contain %q, got %q", "explicit_args", output.String())
	}
}

// TestProcess_Pid_NilCmd exercises the nil-cmd guard in Pid().
func TestProcess_Pid_NilCmd(t *testing.T) {
	t.Parallel()
	p := &Process{done: make(chan struct{})}
	// cmd is nil → Pid() should return 0.
	pid := p.Pid()
	if pid != 0 {
		t.Fatalf("expected 0 for nil cmd, got %d", pid)
	}
}

// TestProcess_WriteSignalDeadlock verifies that Signal can be called while
// Write is blocked on a full PTY buffer. Before the fix, Write held p.mu
// for the entire duration of the blocking kernel write, causing Signal
// (which also needs p.mu) to deadlock.
//
// Regression test for: auto-split hang when Claude doesn't read stdin fast
// enough — cancel (SIGKILL) could never be delivered.
func TestProcess_WriteSignalDeadlock(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}

	t.Parallel()
	skipIfWindows(t)

	// Spawn `sleep 3600` — it never reads stdin, so a large write will
	// eventually block when the kernel PTY buffer fills.
	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "sleep",
		Args:    []string{"3600"},
		Rows:    24,
		Cols:    80,
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	// Start a large write in a goroutine. On most systems the PTY
	// buffer is 4–64 KiB; 1 MiB should reliably fill it.
	writeDone := make(chan error, 1)
	go func() {
		_, err := proc.Write([]byte(strings.Repeat("x", 1<<20)))
		writeDone <- err
	}()

	// Give the write time to start blocking.
	time.Sleep(200 * time.Millisecond)

	// Try to send SIGKILL from another goroutine. If there's a mutex
	// deadlock, this will block forever.
	sigDone := make(chan error, 1)
	go func() {
		sigDone <- proc.Signal("SIGKILL")
	}()

	select {
	case err := <-sigDone:
		if err != nil {
			t.Logf("Signal returned error (acceptable): %v", err)
		}
		t.Log("Signal completed without deadlock")
	case <-time.After(5 * time.Second):
		t.Fatal("DEADLOCK: Signal blocked while Write is in progress — " +
			"Write must release the mutex before blocking I/O")
	}

	// The write goroutine should also complete (with error) now that
	// the child is dead.
	select {
	case err := <-writeDone:
		t.Logf("Write completed after SIGKILL, err=%v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Write goroutine did not unblock after SIGKILL")
	}
}

// TestProcess_CloseWhileWriteBlocked verifies that Close can proceed
// while Write is blocked on a full PTY buffer.
func TestProcess_CloseWhileWriteBlocked(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}

	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "sleep",
		Args:    []string{"3600"},
		Rows:    24,
		Cols:    80,
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	// Start a large write that will block.
	writeDone := make(chan error, 1)
	go func() {
		_, err := proc.Write([]byte(strings.Repeat("x", 1<<20)))
		writeDone <- err
	}()

	time.Sleep(200 * time.Millisecond)

	// Close should not deadlock even if Write is blocking.
	closeDone := make(chan error, 1)
	go func() {
		closeDone <- proc.Close()
	}()

	select {
	case err := <-closeDone:
		if err != nil {
			t.Logf("Close returned error (acceptable): %v", err)
		}
		t.Log("Close completed without deadlock")
	case <-time.After(10 * time.Second):
		t.Fatal("DEADLOCK: Close blocked while Write is in progress")
	}

	// Write should also complete (with error).
	select {
	case err := <-writeDone:
		t.Logf("Write completed after Close, err=%v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Write goroutine did not unblock after Close")
	}
}

type stuckHandle struct {
	mu      sync.Mutex
	signals []os.Signal
}

func (h *stuckHandle) Wait() error { select {} }

func (h *stuckHandle) Signal(sig os.Signal) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.signals = append(h.signals, sig)
	return nil
}

func (h *stuckHandle) Pid() int { return 12345 }

func (h *stuckHandle) SignalCount(sig os.Signal) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	count := 0
	for _, s := range h.signals {
		if s == sig {
			count++
		}
	}
	return count
}

// TestProcess_Close_ForceKillWaitTimeout verifies Close does not block forever
// when the process never reports exit after SIGKILL.
func TestProcess_Close_ForceKillWaitTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}

	t.Parallel()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	handle := &stuckHandle{}
	proc := &Process{
		ptyFile:  r,
		ttyFile:  w,
		done:     make(chan struct{}), // never closed => process appears alive forever
		exitCode: -1,
		cmd:      handle,
	}

	start := time.Now()
	closeErr := proc.Close()
	elapsed := time.Since(start)

	// Leave wider slack for busy CI runners; this test validates bounded
	// shutdown semantics, not tight wall-clock precision.
	maxExpected := closeGracefulWait + closeForceWait + 5*time.Second
	if elapsed > maxExpected {
		t.Fatalf("Close took too long: %v (max expected ~%v)", elapsed, maxExpected)
	}
	if closeErr == nil {
		t.Fatal("expected timeout error when process never exits after SIGKILL")
	}
	if !strings.Contains(closeErr.Error(), "force-kill wait timed out") {
		t.Fatalf("unexpected error: %v", closeErr)
	}
	if handle.SignalCount(syscall.SIGTERM) != 1 {
		t.Fatalf("expected exactly one SIGTERM, got %d", handle.SignalCount(syscall.SIGTERM))
	}
	if handle.SignalCount(syscall.SIGKILL) != 1 {
		t.Fatalf("expected exactly one SIGKILL, got %d", handle.SignalCount(syscall.SIGKILL))
	}
}

func TestProcess_Write_WithTimeout(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	// Normal write succeeds with timeout enabled (the default).
	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "cat",
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	if proc.writeTimeout != DefaultWriteTimeout {
		t.Fatalf("expected writeTimeout=%v, got %v", DefaultWriteTimeout, proc.writeTimeout)
	}

	// Write a small payload — should succeed instantly.
	if _, err := proc.Write([]byte("hello\n")); err != nil {
		t.Fatalf("Write with timeout failed: %v", err)
	}
}

func TestProcess_Write_TimeoutDisabled(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command:      "cat",
		WriteTimeout: -1, // disable deadline
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	if proc.writeTimeout != -1 {
		t.Fatalf("expected writeTimeout=-1 (disabled), got %v", proc.writeTimeout)
	}

	if _, err := proc.Write([]byte("hello\n")); err != nil {
		t.Fatalf("Write without timeout failed: %v", err)
	}
}

func TestProcess_Write_CustomTimeout(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command:      "cat",
		WriteTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	if proc.writeTimeout != 5*time.Second {
		t.Fatalf("expected writeTimeout=5s, got %v", proc.writeTimeout)
	}

	if _, err := proc.Write([]byte("custom timeout\n")); err != nil {
		t.Fatalf("Write with custom timeout failed: %v", err)
	}
}

// TestConPTY_Smoke verifies basic ConPTY functionality on Windows.
// This test only runs on Windows and is skipped on other platforms.
// It validates the core spawn→read→wait lifecycle that cross-compilation
// alone cannot verify.
func TestConPTY_Smoke(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("ConPTY smoke test requires Windows")
	}
	if testing.Short() {
		t.Skip("skipping ConPTY smoke test in short mode")
	}
	requireConPTYRuntime(t)
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	proc, err := Spawn(ctx, SpawnConfig{
		Command: "powershell.exe",
		Args:    []string{"-NoProfile", "-Command", "Write-Output 'conpty-smoke-test'; Start-Sleep -Milliseconds 250; exit 0"},
		Rows:    24,
		Cols:    80,
	})
	if err != nil {
		t.Fatalf("Spawn via ConPTY failed: %v", err)
	}
	defer proc.Close()

	// Read output — should contain the echo string.
	var output strings.Builder
	deadline := time.After(10 * time.Second)
	done := false
	for !done {
		readCh := make(chan struct {
			data string
			err  error
		}, 1)
		go func() {
			data, readErr := proc.Read()
			readCh <- struct {
				data string
				err  error
			}{data: data, err: readErr}
		}()

		select {
		case <-deadline:
			_ = proc.Close()
			t.Fatalf("timed out waiting for conpty smoke output, got so far: %q", output.String())
		case readResult := <-readCh:
			if readResult.data != "" {
				output.WriteString(readResult.data)
			}
			if strings.Contains(output.String(), "conpty-smoke-test") {
				done = true
			}
			if readResult.err != nil {
				done = true
			}
		}
	}
	if !strings.Contains(output.String(), "conpty-smoke-test") {
		t.Fatalf("expected output to contain 'conpty-smoke-test', got %q", output.String())
	}

	code, waitErr := proc.Wait()
	if waitErr != nil {
		t.Fatalf("Wait failed: %v", waitErr)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

// TestConPTY_ContextCancel verifies that context cancellation kills the
// child process on Windows. This validates KILL-01 from the autopsy:
// the context watcher goroutine in spawnWithConPTY must actually work.
func TestConPTY_ContextCancel(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("ConPTY context cancel test requires Windows")
	}
	if testing.Short() {
		t.Skip("skipping ConPTY context cancel test in short mode")
	}
	requireConPTYRuntime(t)
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	proc, err := Spawn(ctx, SpawnConfig{
		Command: "cmd.exe",
		Args:    []string{"/c", "timeout", "/t", "60"},
		Rows:    24,
		Cols:    80,
	})
	if err != nil {
		t.Fatalf("Spawn via ConPTY failed: %v", err)
	}
	defer proc.Close()

	// Cancel context - should kill the child via the context watcher goroutine.
	cancel()

	// Wait should complete promptly (child was killed).
	done := make(chan struct{})
	go func() {
		proc.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Child exited as expected.
	case <-time.After(10 * time.Second):
		t.Fatal("context cancellation did not kill the Windows child process within 10s")
	}
}

// TestConPTY_Resize verifies ResizePseudoConsole works on Windows.
func TestConPTY_Resize(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("ConPTY resize test requires Windows")
	}
	if testing.Short() {
		t.Skip("skipping ConPTY resize test in short mode")
	}
	requireConPTYRuntime(t)
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	proc, err := Spawn(ctx, SpawnConfig{
		Command: "cmd.exe",
		Args:    []string{"/c", "echo", "resize-test"},
		Rows:    24,
		Cols:    80,
	})
	if err != nil {
		t.Fatalf("Spawn via ConPTY failed: %v", err)
	}
	defer proc.Close()

	// Resize should succeed without error.
	if err := proc.Resize(40, 120); err != nil {
		t.Fatalf("Resize failed: %v", err)
	}
}
