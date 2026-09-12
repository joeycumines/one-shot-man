//go:build !windows

package termmux

import (
	"context"
	"syscall"
	"testing"
	"time"
)

// TestCaptureSession_Wait_ExitModes verifies the CaptureSession.Wait
// contract mirrors the pty Process.Wait contract proven at 20e9476. Each
// subtest is a focused regression asserting (code, nil) vs (-1, error).
// Uses t.TempDir-owned helper binaries via buildProgram and t.Parallel
// isolation. This is the user-visible surface for the pty fix.
func TestCaptureSession_Wait_ExitModes(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	t.Run("zero", func(t *testing.T) {
		t.Parallel()
		cs := NewCaptureSession(CaptureConfig{Command: buildProgram(t, "package main\nimport \"os\"\nfunc main(){ os.Exit(0) }\n")})
		if err := cs.Start(context.Background()); err != nil {
			t.Fatalf("Start zero: %v", err)
		}
		defer cs.Close()
		code, err := cs.Wait()
		if err != nil {
			t.Fatalf("Wait zero: expected nil error, got %v", err)
		}
		if code != 0 {
			t.Fatalf("Wait zero: expected 0, got %d", code)
		}
		if cs.ExitCode() != 0 {
			t.Fatalf("ExitCode zero: expected 0, got %d", cs.ExitCode())
		}
		code2, err2 := cs.Wait()
		if code2 != 0 || err2 != nil {
			t.Fatalf("second Wait zero: (%d,%v) want (0,nil)", code2, err2)
		}
	})

	t.Run("nonzero42", func(t *testing.T) {
		t.Parallel()
		cs := NewCaptureSession(CaptureConfig{Command: buildProgram(t, "package main\nimport \"os\"\nfunc main(){ os.Exit(42) }\n")})
		if err := cs.Start(context.Background()); err != nil {
			t.Fatalf("Start 42: %v", err)
		}
		defer cs.Close()
		code, err := cs.Wait()
		if err != nil {
			t.Fatalf("Wait 42: expected nil error (20e9476 contract), got %v", err)
		}
		if code != 42 {
			t.Fatalf("Wait 42: expected 42, got %d", code)
		}
		if cs.ExitCode() != 42 {
			t.Fatalf("ExitCode 42: expected 42, got %d", cs.ExitCode())
		}
	})

	t.Run("signal_int_130", func(t *testing.T) {
		t.Parallel()
		cs := NewCaptureSession(CaptureConfig{Command: buildProgram(t, "package main\nimport \"time\"\nfunc main(){ time.Sleep(60*time.Second) }\n")})
		if err := cs.Start(context.Background()); err != nil {
			t.Fatalf("Start signal: %v", err)
		}
		defer cs.Close()
		time.Sleep(100 * time.Millisecond)
		if err := cs.Interrupt(); err != nil {
			t.Fatalf("Interrupt: %v", err)
		}
		done := make(chan struct{})
		var code int
		var waitErr error
		go func() {
			code, waitErr = cs.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for SIGINT exit")
		}
		if waitErr != nil {
			t.Fatalf("Wait SIGINT: expected nil error, got %v", waitErr)
		}
		want := 128 + int(syscall.SIGINT)
		if code != want {
			t.Fatalf("Wait SIGINT: expected %d (128+SIGINT), got %d", want, code)
		}
	})

	t.Run("genuine_wait_failure_minus1", func(t *testing.T) {
		t.Parallel()
		cs := NewCaptureSession(CaptureConfig{Command: "/nonexistent/binary/that/does/not/exist"})
		err := cs.Start(context.Background())
		if err == nil {
			t.Fatal("expected error starting nonexistent binary")
		}
		code, waitErr := cs.Wait()
		if code != -1 {
			t.Fatalf("Wait after failed Start: expected -1, got %d", code)
		}
		if waitErr == nil {
			t.Fatal("Wait after failed Start: expected non-nil error, got nil")
		}
		if cs.ExitCode() != -1 {
			t.Fatalf("ExitCode after failed Start: expected -1, got %d", cs.ExitCode())
		}
	})
}
