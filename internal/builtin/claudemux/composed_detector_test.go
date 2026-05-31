package claudemux

import (
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/builtin/claudemux/testutil"
)

func TestComposedDetector_ProtocolMode(t *testing.T) {
	det, err := NewComposedDetector(DetectModeProtocol, DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewComposedDetector: %v", err)
	}

	now := time.Now()

	// EventCompletion(system-init) → StateReady
	u := det.ProcessEvent(OutputEvent{Type: EventCompletion, Pattern: "system-init"}, now)
	if !u.Changed || u.State != StateReady {
		t.Errorf("system-init: Changed=%v State=%s, want Changed=true State=Ready", u.Changed, tuiStateName(u.State))
	}
	if det.State() != StateReady {
		t.Errorf("State() = %s, want Ready", tuiStateName(det.State()))
	}

	// EventThinking → StateProcessing
	u = det.ProcessEvent(OutputEvent{Type: EventThinking, Pattern: "assistant-thinking"}, now)
	if !u.Changed || u.State != StateProcessing {
		t.Errorf("thinking: Changed=%v State=%s, want Changed=true State=Processing", u.Changed, tuiStateName(u.State))
	}

	// EventError → StateError
	u = det.ProcessEvent(OutputEvent{Type: EventError, Pattern: "result-error", Fields: map[string]string{"message": "boom"}}, now)
	if !u.Changed || u.State != StateError {
		t.Errorf("error: Changed=%v State=%s, want Changed=true State=Error", u.Changed, tuiStateName(u.State))
	}

	// Reset to Ready for remaining tests.
	det.ProcessEvent(OutputEvent{Type: EventCompletion, Pattern: "system-init"}, now)

	// EventRateLimit → StateRateLimited
	u = det.ProcessEvent(OutputEvent{Type: EventRateLimit, Pattern: "rate-limit-event"}, now)
	if !u.Changed || u.State != StateRateLimited {
		t.Errorf("rate limit: Changed=%v State=%s, want Changed=true State=RateLimited", u.Changed, tuiStateName(u.State))
	}

	// Reset to Ready.
	det.ProcessEvent(OutputEvent{Type: EventCompletion, Pattern: "system-init"}, now)

	// EventPermission → StatePermissionPrompt
	u = det.ProcessEvent(OutputEvent{Type: EventPermission, Pattern: "control-request-tool-call", Fields: map[string]string{"tool": "Bash", "id": "p1"}}, now)
	if !u.Changed || u.State != StatePermissionPrompt {
		t.Errorf("permission: Changed=%v State=%s, want Changed=true State=PermissionPrompt", u.Changed, tuiStateName(u.State))
	}

	// Reset to Processing, then EventCompletion(result-success) → StateReady
	det.ProcessEvent(OutputEvent{Type: EventThinking, Pattern: "assistant-thinking"}, now)
	u = det.ProcessEvent(OutputEvent{Type: EventCompletion, Pattern: "result-success"}, now)
	if !u.Changed || u.State != StateReady {
		t.Errorf("result-success: Changed=%v State=%s, want Changed=true State=Ready", u.Changed, tuiStateName(u.State))
	}
}

func TestComposedDetector_ProtocolMode_NoChangeOnUnknown(t *testing.T) {
	det, err := NewComposedDetector(DetectModeProtocol, DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewComposedDetector: %v", err)
	}

	now := time.Now()

	// First transition to Ready so we have a known state.
	det.ProcessEvent(OutputEvent{Type: EventCompletion, Pattern: "system-init"}, now)

	// EventText should not change state.
	u := det.ProcessEvent(OutputEvent{Type: EventText, Line: "some output"}, now)
	if u.Changed {
		t.Errorf("EventText: Changed=%v, want false", u.Changed)
	}
	if det.State() != StateReady {
		t.Errorf("State() = %s, want Ready", tuiStateName(det.State()))
	}
}

func TestComposedDetector_TUIMode_ProcessRaw(t *testing.T) {
	testutil.SkipSlow(t)

	proc, _ := spawnTUI(t)
	defer proc.Close()

	det, err := NewComposedDetector(DetectModeTUI, DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewComposedDetector: %v", err)
	}

	// Read PTY output and feed through ProcessRaw until StateReady.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if det.State() == StateReady {
			break
		}
		chunk, readErr := proc.Read()
		if readErr != nil {
			t.Fatalf("PTY read: %v", readErr)
		}
		if chunk == "" {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		det.ProcessRaw([]byte(chunk), time.Now())
	}

	if det.State() != StateReady {
		t.Fatalf("State() = %s, want Ready", tuiStateName(det.State()))
	}

	// LastEvents may be empty if no parser patterns matched the TUI output.
	// The key assertion is that ProcessRaw produced state transitions.
	_ = det.LastEvents()
}

func TestComposedDetector_Reset(t *testing.T) {
	det, err := NewComposedDetector(DetectModeProtocol, DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewComposedDetector: %v", err)
	}

	now := time.Now()

	// Transition to Ready.
	det.ProcessEvent(OutputEvent{Type: EventCompletion, Pattern: "system-init"}, now)
	if det.State() != StateReady {
		t.Fatalf("State() = %s, want Ready before reset", tuiStateName(det.State()))
	}

	// Reset should return to StateInitializing.
	det.Reset()
	if det.State() != StateInitializing {
		t.Errorf("State() after Reset = %s, want Initializing", tuiStateName(det.State()))
	}
}
