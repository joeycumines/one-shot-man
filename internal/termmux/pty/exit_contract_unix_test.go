//go:build !windows

package pty

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestUnixExitError_ExitCode verifies the ExitCode mapping for normal,
// signal, and stopped statuses. This locks the 128+signal contract as well
// as the -1 stopped/continued sentinel.
func TestUnixExitError_ExitCode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status unix.WaitStatus
		want   int
	}{
		{"exit0", unix.WaitStatus(0), 0},
		{"exit1", unix.WaitStatus(1 << 8), 1},
		{"exit42", unix.WaitStatus(42 << 8), 42},
		{"exit255", unix.WaitStatus(255 << 8), 255},
		{"sigterm", unix.WaitStatus(syscall.SIGTERM), 128 + int(syscall.SIGTERM)},
		{"sigkill", unix.WaitStatus(syscall.SIGKILL), 128 + int(syscall.SIGKILL)},
		{"sigint", unix.WaitStatus(syscall.SIGINT), 128 + int(syscall.SIGINT)},
		{"sigquit", unix.WaitStatus(syscall.SIGQUIT), 128 + int(syscall.SIGQUIT)},
		{"stopped", unix.WaitStatus(0x7f), -1},
		{"continued", unix.WaitStatus(0xffff), -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := &unixExitError{status: tc.status}
			if got := e.ExitCode(); got != tc.want {
				t.Fatalf("ExitCode() = %d, want %d for status %#x (exited=%v signaled=%v stopped=%v)", got, tc.want, tc.status, tc.status.Exited(), tc.status.Signaled(), tc.status.Stopped())
			}
			if _, ok := errors.AsType[*unixExitError](e); !ok {
				t.Fatalf("errors.As failed for *unixExitError")
			}
			if e.Error() == "" {
				t.Fatal("Error() should not be empty")
			}
		})
	}
}

// TestProcess_Wait_ExitModes exercises the full Process.Wait contract on
// Unix via real PTY spawns. Each subtest is a focused regression for one
// exit mode asserting (code, nil) vs (-1, error). Table-driven, t.Parallel
// isolated, helper binaries are t.TempDir-owned via buildProgram.
func TestProcess_Wait_ExitModes(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	t.Run("zero", func(t *testing.T) {
		t.Parallel()
		bin := buildProgram(t, "package main\nimport \"os\"\nfunc main(){ os.Exit(0) }\n")
		proc, err := Spawn(context.Background(), SpawnConfig{Command: bin})
		if err != nil {
			t.Fatalf("Spawn zero: %v", err)
		}
		defer proc.Close()
		code, waitErr := proc.Wait()
		if waitErr != nil {
			t.Fatalf("Wait zero: expected nil error, got %v", waitErr)
		}
		if code != 0 {
			t.Fatalf("Wait zero: expected 0, got %d", code)
		}
		code2, err2 := proc.Wait()
		if code2 != 0 || err2 != nil {
			t.Fatalf("second Wait zero: (%d,%v) want (0,nil)", code2, err2)
		}
	})

	t.Run("nonzero42", func(t *testing.T) {
		t.Parallel()
		bin := buildProgram(t, "package main\nimport \"os\"\nfunc main(){ os.Exit(42) }\n")
		proc, err := Spawn(context.Background(), SpawnConfig{Command: bin})
		if err != nil {
			t.Fatalf("Spawn 42: %v", err)
		}
		defer proc.Close()
		code, waitErr := proc.Wait()
		if waitErr != nil {
			t.Fatalf("Wait 42: expected nil error (20e9476 contract), got %v", waitErr)
		}
		if code != 42 {
			t.Fatalf("Wait 42: expected 42, got %d", code)
		}
	})

	t.Run("signal_term_143", func(t *testing.T) {
		t.Parallel()
		bin := buildProgram(t, "package main\nimport \"time\"\nfunc main(){ time.Sleep(60*time.Second) }\n")
		proc, err := Spawn(context.Background(), SpawnConfig{Command: bin})
		if err != nil {
			t.Fatalf("Spawn signal: %v", err)
		}
		defer proc.Close()
		time.Sleep(100 * time.Millisecond)
		if !proc.IsAlive() {
			t.Fatal("expected process alive before SIGTERM")
		}
		if err := proc.Signal("SIGTERM"); err != nil {
			t.Fatalf("Signal SIGTERM: %v", err)
		}
		done := make(chan struct{})
		var code int
		var waitErr error
		go func() {
			code, waitErr = proc.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for SIGTERM exit")
		}
		if waitErr != nil {
			t.Fatalf("Wait SIGTERM: expected nil error, got %v", waitErr)
		}
		want := 128 + int(syscall.SIGTERM)
		if code != want {
			t.Fatalf("Wait SIGTERM: expected %d (128+SIGTERM), got %d", want, code)
		}
	})

	t.Run("genuine_wait_failure_minus1", func(t *testing.T) {
		t.Parallel()
		cmd := exec.Command("true")
		cmd.Process = &os.Process{Pid: 999999}
		h := &unixProcessHandle{cmd: cmd}
		err := h.waitWithSlaveRelease(func() {})
		if err == nil {
			t.Fatal("expected error from waitWithSlaveRelease with invalid pid")
		}
		if _, ok := errors.AsType[*unixExitError](err); ok {
			t.Fatalf("expected non-unixExitError for invalid pid, got unixExitError %v", err)
		}
		r, w, pipeErr := os.Pipe()
		if pipeErr != nil {
			t.Fatalf("os.Pipe: %v", pipeErr)
		}
		defer r.Close()
		defer w.Close()
		proc := &Process{
			ptyFile:  r,
			ttyFile:  w,
			done:     make(chan struct{}),
			exitCode: -1,
			cmd:      &stuckHandle{},
		}
		proc.exitErr = err
		close(proc.done)
		code, waitErr := proc.Wait()
		if code != -1 {
			t.Fatalf("synthetic failure: expected -1, got %d", code)
		}
		if waitErr == nil {
			t.Fatal("synthetic failure: expected non-nil error, got nil")
		}
		stopped := &unixExitError{status: unix.WaitStatus(0x7f)}
		if stopped.ExitCode() != -1 {
			t.Fatalf("stopped ExitCode want -1, got %d", stopped.ExitCode())
		}
	})
}

// TestProcess_Wait_Concurrent ensures multiple concurrent Wait callers all
// observe the same (code,nil) result — regression for the done-channel
// broadcast contract.
func TestProcess_Wait_Concurrent(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)
	bin := buildProgram(t, "package main\nimport \"os\"\nfunc main(){ os.Exit(42) }\n")
	proc, err := Spawn(context.Background(), SpawnConfig{Command: bin})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer proc.Close()
	const callers = 8
	codes := make([]int, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := range callers {
		i := i
		go func() {
			defer wg.Done()
			codes[i], errs[i] = proc.Wait()
		}()
	}
	wg.Wait()
	for i := range callers {
		if errs[i] != nil {
			t.Fatalf("caller %d: expected nil error, got %v", i, errs[i])
		}
		if codes[i] != 42 {
			t.Fatalf("caller %d: expected 42, got %d", i, codes[i])
		}
	}
}

// TestProcess_ClosePseudoConsole_IsNoopOnUnix verifies that the Unix
// no-op implementation is safe to call multiple times, concurrently, and
// does not regress the non-zero exit contract proven at 20e9476.
func TestProcess_ClosePseudoConsole_IsNoopOnUnix(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)
	bin := buildProgram(t, "package main\nimport \"os\"\nfunc main(){ os.Exit(42) }\n")
	proc, err := Spawn(context.Background(), SpawnConfig{Command: bin})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer proc.Close()
	proc.ClosePseudoConsole()
	proc.ClosePseudoConsole()
	code, waitErr := proc.Wait()
	if waitErr != nil {
		t.Fatalf("Wait after ClosePseudoConsole: want nil, got %v", waitErr)
	}
	if code != 42 {
		t.Fatalf("Wait after ClosePseudoConsole: want 42, got %d", code)
	}
	proc.ClosePseudoConsole()
	proc.ClosePseudoConsole()
	var wg sync.WaitGroup
	wg.Add(10)
	for range 10 {
		go func() {
			defer wg.Done()
			proc.ClosePseudoConsole()
		}()
	}
	wg.Wait()
	code2, err2 := proc.Wait()
	if code2 != 42 || err2 != nil {
		t.Fatalf("second Wait after concurrent ClosePseudoConsole: (%d,%v) want (42,nil)", code2, err2)
	}
}
