//go:build windows

package pty

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
)

// TestProcess_Wait_ExitModes_Windows exercises the Windows Process.Wait
// contract. On Windows the 128+signal mapping does not exist; signaled
// termination is modeled via TerminateProcess (exit 1). This file is the
// Windows counterpart to exit_contract_unix_test.go and ensures the non-zero
// contract proven at 20e9476 is not regressed on the ConPTY descriptor path.
func TestProcess_Wait_ExitModes_Windows(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("spawns process to build test helper")
	}

	t.Run("zero", func(t *testing.T) {
		t.Parallel()
		requireConPTYRuntime(t)
		proc, err := Spawn(context.Background(), SpawnConfig{
			Command: "powershell.exe",
			Args:    []string{"-NoProfile", "-Command", "exit 0"},
		})
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
		requireConPTYRuntime(t)
		proc, err := Spawn(context.Background(), SpawnConfig{
			Command: "powershell.exe",
			Args:    []string{"-NoProfile", "-Command", "exit 42"},
		})
		if err != nil {
			t.Fatalf("Spawn 42: %v", err)
		}
		defer proc.Close()
		code, waitErr := proc.Wait()
		if waitErr != nil {
			t.Fatalf("Wait 42: expected nil error, got %v", waitErr)
		}
		if code != 42 {
			t.Fatalf("Wait 42: expected 42, got %d", code)
		}
		proc.ClosePseudoConsole()
		proc.ClosePseudoConsole()
		code2, err2 := proc.Wait()
		if code2 != 42 || err2 != nil {
			t.Fatalf("Wait after ClosePseudoConsole: (%d,%v) want (42,nil)", code2, err2)
		}
	})

	t.Run("signal_terminate", func(t *testing.T) {
		t.Parallel()
		requireConPTYRuntime(t)
		proc, err := Spawn(context.Background(), SpawnConfig{
			Command: "powershell.exe",
			Args:    []string{"-NoProfile", "-Command", "Start-Sleep -Seconds 60"},
		})
		if err != nil {
			t.Fatalf("Spawn signal: %v", err)
		}
		defer proc.Close()
		time.Sleep(200 * time.Millisecond)
		if !proc.IsAlive() {
			t.Fatal("expected process alive before SIGKILL")
		}
		if err := proc.Signal("SIGKILL"); err != nil {
			t.Fatalf("Signal SIGKILL: %v", err)
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
		case <-time.After(10 * time.Second):
			t.Fatal("timeout waiting for SIGKILL exit")
		}
		if waitErr != nil {
			t.Fatalf("Wait SIGKILL: expected nil error, got %v", waitErr)
		}
		if code == 0 {
			t.Fatalf("Wait SIGKILL: expected non-zero exit after terminate, got 0")
		}
	})

	t.Run("genuine_wait_failure_minus1", func(t *testing.T) {
		t.Parallel()
		syntheticErr := errors.New("pty: synthetic wait failure for regression")
		r, w, pipeErr := os.Pipe()
		if pipeErr != nil {
			t.Fatalf("open pipe: %v", pipeErr)
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
		proc.exitErr = syntheticErr
		close(proc.done)
		code, waitErr := proc.Wait()
		if code != -1 {
			t.Fatalf("synthetic failure: expected -1, got %d", code)
		}
		if !errors.Is(waitErr, syntheticErr) {
			t.Fatalf("synthetic failure: expected synthetic error, got %v", waitErr)
		}
	})
}

// TestProcess_Wait_Concurrent_Windows ensures concurrent Wait callers see the
// same result on the ConPTY path.
func TestProcess_Wait_Concurrent_Windows(t *testing.T) {
	t.Parallel()
	requireConPTYRuntime(t)
	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "powershell.exe",
		Args:    []string{"-NoProfile", "-Command", "exit 42"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer proc.Close()
	const callers = 8
	codes := make([]int, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		i := i
		go func() {
			defer wg.Done()
			codes[i], errs[i] = proc.Wait()
		}()
	}
	wg.Wait()
	for i := 0; i < callers; i++ {
		if errs[i] != nil {
			t.Fatalf("caller %d: expected nil error, got %v", i, errs[i])
		}
		if codes[i] != 42 {
			t.Fatalf("caller %d: expected 42, got %d", i, codes[i])
		}
	}
}

// TestProcess_ClosePseudoConsole_ConPTY verifies the thread-safe
// ClosePseudoConsole on Windows: idempotent, safe to call before/after Wait,
// concurrent, and does not alter the non-zero exit code.
func TestProcess_ClosePseudoConsole_ConPTY(t *testing.T) {
	t.Parallel()
	requireConPTYRuntime(t)
	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "powershell.exe",
		Args:    []string{"-NoProfile", "-Command", "Write-Output 'conpty-flush-test'; exit 42"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer proc.Close()
	proc.ClosePseudoConsole()
	code, waitErr := proc.Wait()
	if waitErr != nil {
		t.Fatalf("Wait after pre-Wait ClosePseudoConsole: want nil, got %v", waitErr)
	}
	if code != 42 {
		t.Fatalf("Wait after pre-Wait ClosePseudoConsole: want 42, got %d", code)
	}
	proc.ClosePseudoConsole()
	proc.ClosePseudoConsole()
	var wg sync.WaitGroup
	wg.Add(20)
	for i := 0; i < 20; i++ {
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
