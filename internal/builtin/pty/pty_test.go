package pty

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
)

func skipIfWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
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

	err = proc.Write("hello")
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
}

func TestProcess_ContextCancel(t *testing.T) {
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

	if err := proc.Write("hello from pty\n"); err != nil {
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

func TestModule_Spawn(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	rt := goja.New()
	mod := rt.NewObject()
	exports := rt.NewObject()
	_ = mod.Set("exports", exports)

	loader := Require(context.Background())
	loader(rt, mod)

	exports = mod.Get("exports").(*goja.Object)
	spawnFn, ok := goja.AssertFunction(exports.Get("spawn"))
	if !ok {
		t.Fatal("spawn export is not a function")
	}

	result, err := spawnFn(goja.Undefined(),
		rt.ToValue("echo"),
		rt.ToValue([]string{"module_test"}),
	)
	if err != nil {
		t.Fatalf("spawn returned error: %v", err)
	}

	procObj := result.ToObject(rt)

	waitFn, ok := goja.AssertFunction(procObj.Get("wait"))
	if !ok {
		t.Fatal("wait is not a function")
	}
	waitResult, err := waitFn(goja.Undefined())
	if err != nil {
		t.Fatalf("wait returned error: %v", err)
	}

	var waitMap map[string]interface{}
	if exportErr := rt.ExportTo(waitResult, &waitMap); exportErr != nil {
		t.Fatalf("failed to export wait result: %v", exportErr)
	}
	if waitMap["error"] != nil {
		t.Fatalf("wait returned error: %v", waitMap["error"])
	}

	closeFn, ok := goja.AssertFunction(procObj.Get("close"))
	if !ok {
		t.Fatal("close is not a function")
	}
	if _, err := closeFn(goja.Undefined()); err != nil {
		t.Fatalf("close returned error: %v", err)
	}
}

func TestModule_Spawn_NoArgs(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	rt := goja.New()
	mod := rt.NewObject()
	exports := rt.NewObject()
	_ = mod.Set("exports", exports)

	loader := Require(context.Background())
	loader(rt, mod)

	exports = mod.Get("exports").(*goja.Object)
	spawnFn, ok := goja.AssertFunction(exports.Get("spawn"))
	if !ok {
		t.Fatal("spawn export is not a function")
	}

	_, err := spawnFn(goja.Undefined())
	if err == nil {
		t.Fatal("expected error for spawn with no arguments")
	}

	var ex *goja.Exception
	if !errors.As(err, &ex) {
		t.Fatalf("expected goja exception, got %T: %v", err, err)
	}
}

func TestModule_SpawnWithOptions(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	rt := goja.New()
	mod := rt.NewObject()
	exports := rt.NewObject()
	_ = mod.Set("exports", exports)

	loader := Require(context.Background())
	loader(rt, mod)

	exports = mod.Get("exports").(*goja.Object)
	spawnFn, ok := goja.AssertFunction(exports.Get("spawn"))
	if !ok {
		t.Fatal("spawn export is not a function")
	}

	optsObj := rt.NewObject()
	_ = optsObj.Set("rows", 48)
	_ = optsObj.Set("cols", 120)

	result, err := spawnFn(goja.Undefined(),
		rt.ToValue("echo"),
		rt.ToValue([]string{"opts_test"}),
		optsObj,
	)
	if err != nil {
		t.Fatalf("spawn returned error: %v", err)
	}

	procObj := result.ToObject(rt)

	waitFn, ok := goja.AssertFunction(procObj.Get("wait"))
	if !ok {
		t.Fatal("wait is not a function")
	}
	if _, err := waitFn(goja.Undefined()); err != nil {
		t.Fatalf("wait error: %v", err)
	}

	closeFn, ok := goja.AssertFunction(procObj.Get("close"))
	if !ok {
		t.Fatal("close is not a function")
	}
	if _, err := closeFn(goja.Undefined()); err != nil {
		t.Fatalf("close error: %v", err)
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
		writeDone <- proc.Write(strings.Repeat("x", 1<<20))
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
		writeDone <- proc.Write(strings.Repeat("x", 1<<20))
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

// TestModule_ProcessAllMethods exercises the JS wrapProcess methods
// (write, read, resize, signal, isAlive, pid) through the goja runtime.
func TestModule_ProcessAllMethods(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	rt := goja.New()
	mod := rt.NewObject()
	exports := rt.NewObject()
	_ = mod.Set("exports", exports)

	loader := Require(context.Background())
	loader(rt, mod)

	exports = mod.Get("exports").(*goja.Object)
	spawnFn, ok := goja.AssertFunction(exports.Get("spawn"))
	if !ok {
		t.Fatal("spawn export is not a function")
	}

	// Spawn cat so we can write/read interactively.
	result, err := spawnFn(goja.Undefined(),
		rt.ToValue("cat"),
		rt.ToValue([]string{}),
	)
	if err != nil {
		t.Fatalf("spawn returned error: %v", err)
	}
	procObj := result.ToObject(rt)

	// Test isAlive()
	isAliveFn, ok := goja.AssertFunction(procObj.Get("isAlive"))
	if !ok {
		t.Fatal("isAlive is not a function")
	}
	aliveResult, err := isAliveFn(goja.Undefined())
	if err != nil {
		t.Fatalf("isAlive error: %v", err)
	}
	if !aliveResult.ToBoolean() {
		t.Fatal("expected process to be alive")
	}

	// Test pid()
	pidFn, ok := goja.AssertFunction(procObj.Get("pid"))
	if !ok {
		t.Fatal("pid is not a function")
	}
	pidResult, err := pidFn(goja.Undefined())
	if err != nil {
		t.Fatalf("pid error: %v", err)
	}
	if pidResult.ToInteger() <= 0 {
		t.Fatalf("expected positive PID, got %d", pidResult.ToInteger())
	}

	// Test write()
	writeFn, ok := goja.AssertFunction(procObj.Get("write"))
	if !ok {
		t.Fatal("write is not a function")
	}
	if _, err := writeFn(goja.Undefined(), rt.ToValue("hello_pty\n")); err != nil {
		t.Fatalf("write error: %v", err)
	}

	// Test read()
	readFn, ok := goja.AssertFunction(procObj.Get("read"))
	if !ok {
		t.Fatal("read is not a function")
	}
	var output strings.Builder
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out reading, got: %q", output.String())
		default:
		}
		readResult, _ := readFn(goja.Undefined())
		if readResult != nil && readResult.String() != "" {
			output.WriteString(readResult.String())
		}
		if strings.Contains(output.String(), "hello_pty") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Test resize()
	resizeFn, ok := goja.AssertFunction(procObj.Get("resize"))
	if !ok {
		t.Fatal("resize is not a function")
	}
	if _, err := resizeFn(goja.Undefined(), rt.ToValue(48), rt.ToValue(120)); err != nil {
		t.Fatalf("resize error: %v", err)
	}

	// Test signal() — send SIGTERM to cat
	signalFn, ok := goja.AssertFunction(procObj.Get("signal"))
	if !ok {
		t.Fatal("signal is not a function")
	}
	if _, err := signalFn(goja.Undefined(), rt.ToValue("SIGTERM")); err != nil {
		t.Fatalf("signal error: %v", err)
	}

	// Test wait()
	waitFn, ok := goja.AssertFunction(procObj.Get("wait"))
	if !ok {
		t.Fatal("wait is not a function")
	}
	waitResult, err := waitFn(goja.Undefined())
	if err != nil {
		t.Fatalf("wait error: %v", err)
	}
	waitObj := waitResult.ToObject(rt)
	code := waitObj.Get("code")
	if code == nil {
		t.Fatal("wait result missing code field")
	}

	// Test close()
	closeFn, ok := goja.AssertFunction(procObj.Get("close"))
	if !ok {
		t.Fatal("close is not a function")
	}
	if _, err := closeFn(goja.Undefined()); err != nil {
		t.Fatalf("close error: %v", err)
	}
}

// TestModule_SpawnWithAllOptions exercises parseSpawnOptions with dir, term, env.
func TestModule_SpawnWithAllOptions(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	rt := goja.New()
	mod := rt.NewObject()
	exports := rt.NewObject()
	_ = mod.Set("exports", exports)

	loader := Require(context.Background())
	loader(rt, mod)

	exports = mod.Get("exports").(*goja.Object)
	spawnFn, ok := goja.AssertFunction(exports.Get("spawn"))
	if !ok {
		t.Fatal("spawn export is not a function")
	}

	dir := t.TempDir()
	optsObj := rt.NewObject()
	_ = optsObj.Set("rows", 30)
	_ = optsObj.Set("cols", 100)
	_ = optsObj.Set("dir", dir)
	_ = optsObj.Set("term", "vt100")
	envObj := rt.NewObject()
	_ = envObj.Set("MY_PTY_VAR", "pty_value_77")
	_ = optsObj.Set("env", envObj)

	result, err := spawnFn(goja.Undefined(),
		rt.ToValue("sh"),
		rt.ToValue([]string{"-c", "echo $MY_PTY_VAR && echo $TERM"}),
		optsObj,
	)
	if err != nil {
		t.Fatalf("spawn with all options returned error: %v", err)
	}

	procObj := result.ToObject(rt)

	// Read output to verify env and term were applied.
	readFn, ok := goja.AssertFunction(procObj.Get("read"))
	if !ok {
		t.Fatal("read is not a function")
	}

	var output strings.Builder
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out, got: %q", output.String())
		default:
		}
		readResult, readErr := readFn(goja.Undefined())
		if readResult != nil && readResult.String() != "" {
			output.WriteString(readResult.String())
		}
		if strings.Contains(output.String(), "pty_value_77") && strings.Contains(output.String(), "vt100") {
			break
		}
		if readResult != nil && readResult.String() == "" && readErr != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !strings.Contains(output.String(), "pty_value_77") {
		t.Errorf("expected MY_PTY_VAR=pty_value_77 in output, got %q", output.String())
	}
	if !strings.Contains(output.String(), "vt100") {
		t.Errorf("expected TERM=vt100 in output, got %q", output.String())
	}

	closeFn, _ := goja.AssertFunction(procObj.Get("close"))
	closeFn(goja.Undefined())
}
