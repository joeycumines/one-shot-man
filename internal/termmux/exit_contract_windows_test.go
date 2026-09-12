//go:build windows

package termmux

import (
	"context"
	"testing"
)

// TestCaptureSession_Wait_ExitModes_Windows verifies the CaptureSession.Wait
// contract on the ConPTY path. It is the Windows counterpart to
// exit_contract_unix_test.go and ensures the non-zero contract proven at
// 20e9476 is not regressed by ConPTY ClosePseudoConsole flushes.
func TestCaptureSession_Wait_ExitModes_Windows(t *testing.T) {
	t.Parallel()

	t.Run("zero", func(t *testing.T) {
		t.Parallel()
		cs := NewCaptureSession(CaptureConfig{
			Command: "powershell.exe",
			Args:    []string{"-NoProfile", "-Command", "exit 0"},
		})
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
		code2, err2 := cs.Wait()
		if code2 != 0 || err2 != nil {
			t.Fatalf("second Wait zero: (%d,%v) want (0,nil)", code2, err2)
		}
	})

	t.Run("nonzero42", func(t *testing.T) {
		t.Parallel()
		cs := NewCaptureSession(CaptureConfig{
			Command: "powershell.exe",
			Args:    []string{"-NoProfile", "-Command", "exit 42"},
		})
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

	t.Run("genuine_wait_failure_minus1", func(t *testing.T) {
		t.Parallel()
		cs := NewCaptureSession(CaptureConfig{Command: "C:\\nonexistent\\binary\\that\\does\\not\\exist.exe"})
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
