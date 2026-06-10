package claudemux

import (
	"context"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/builtin/claudemux/testutil"
	"github.com/joeycumines/one-shot-man/internal/termmux/pty"
)

// spawnTUI spawns mockclaude with -tui flag via PTY and returns the process
// and a VTStateDetector configured for Claude Code TUI state detection.
func spawnTUI(t *testing.T) (*pty.Process, *VTStateDetector) {
	t.Helper()
	bin, err := mockBinaryPath()
	if err != nil {
		t.Fatalf("mock binary: %v", err)
	}
	proc, err := pty.Spawn(context.Background(), pty.SpawnConfig{
		Command: bin,
		Args:    []string{"-tui"},
		Env:     map[string]string{"MOCK_PROCESSING_MS": "50"},
	})
	if err != nil {
		t.Fatalf("pty spawn: %v", err)
	}
	config := DefaultClaudeCodeTUIStateConfig()
	config.StartupTimeout = 10 * time.Second
	config.ProcessingTimeout = 15 * time.Second
	det, err := NewVTStateDetector(config)
	if err != nil {
		proc.Close()
		t.Fatalf("NewVTStateDetector: %v", err)
	}
	return proc, det
}

// readUntilState reads PTY output in a loop, feeds each chunk to the
// VTStateDetector, and returns when any transition matches the target state.
// Fails the test if the target state is not reached within the timeout.
func readUntilState(t *testing.T, proc *pty.Process, det *VTStateDetector, target TUIState, timeout time.Duration) TUIStateUpdate {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if det.State() == target {
			return TUIStateUpdate{State: target, Changed: true}
		}
		chunk, err := proc.Read()
		if err != nil {
			t.Fatalf("PTY read: %v", err)
		}
		if len(chunk) == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		updates := det.ProcessRaw(chunk, time.Now())
		for _, u := range updates {
			if u.State == target {
				return u
			}
		}
	}
	t.Fatalf("timeout waiting for state %s, current state %s", tuiStateName(target), tuiStateName(det.State()))
	return TUIStateUpdate{}
}

func TestTUI_DetectReadyOnStartup(t *testing.T) {
	proc, det := spawnTUI(t)
	defer proc.Close()

	readUntilState(t, proc, det, StateReady, 10*time.Second)
}

func TestTUI_DetectProcessingThenReady(t *testing.T) {
	proc, det := spawnTUI(t)
	defer proc.Close()

	readUntilState(t, proc, det, StateReady, 10*time.Second)

	_, err := proc.Write([]byte("hello\n"))
	if err != nil {
		t.Fatalf("PTY write: %v", err)
	}

	readUntilState(t, proc, det, StateProcessing, 10*time.Second)
	readUntilState(t, proc, det, StateReady, 15*time.Second)
}

func TestTUI_DetectErrorPattern(t *testing.T) {
	proc, det := spawnTUI(t)
	defer proc.Close()

	readUntilState(t, proc, det, StateReady, 10*time.Second)

	_, err := proc.Write([]byte("MOCK_ERROR:test\n"))
	if err != nil {
		t.Fatalf("PTY write: %v", err)
	}

	readUntilState(t, proc, det, StateError, 10*time.Second)

	readUntilState(t, proc, det, StateReady, 10*time.Second)
}

func TestTUI_DetectRateLimit(t *testing.T) {
	proc, det := spawnTUI(t)
	defer proc.Close()

	readUntilState(t, proc, det, StateReady, 10*time.Second)

	_, err := proc.Write([]byte("MOCK_RATE_LIMIT:test\n"))
	if err != nil {
		t.Fatalf("PTY write: %v", err)
	}

	readUntilState(t, proc, det, StateRateLimited, 10*time.Second)

	readUntilState(t, proc, det, StateReady, 10*time.Second)
}

func TestTUI_WaitReadyWithDetector(t *testing.T) {
	testutil.SkipSlow(t)

	proc, det := spawnTUI(t)
	defer proc.Close()

	handle := &ptyAgentHandle{proc: proc, detector: det}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := handle.WaitReady(ctx); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}

	if det.State() != StateReady {
		t.Errorf("detector.State() = %s, want Ready", tuiStateName(det.State()))
	}
}

func TestTUI_ScreenText(t *testing.T) {
	proc, det := spawnTUI(t)
	defer proc.Close()

	readUntilState(t, proc, det, StateReady, 10*time.Second)

	_, err := proc.Write([]byte("hello\n"))
	if err != nil {
		t.Fatalf("PTY write: %v", err)
	}

	readUntilState(t, proc, det, StateReady, 15*time.Second)

	screen := det.ScreenText()
	if screen == "" {
		t.Error("ScreenText() is empty, expected non-empty screen content")
	}
}
