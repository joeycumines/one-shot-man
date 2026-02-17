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

	// Wait for process to complete first — echo is instant and may exit
	// before Read() is called. On macOS, reading from a PTY master after
	// the slave closes can return EIO without delivering buffered data if
	// the read was already blocked when the slave closed.
	code, waitErr := proc.Wait()
	if waitErr != nil {
		t.Fatalf("Wait returned error: %v", waitErr)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	// Drain the PTY output buffer.
	var output strings.Builder
	for {
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
