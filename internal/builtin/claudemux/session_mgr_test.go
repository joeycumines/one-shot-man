package claudemux

import (
	"context"
	"sync"
	"testing"
	"time"
)

// --- State name helpers ---

func TestManagedSessionStateName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		state ManagedSessionState
		want  string
	}{
		{SessionIdle, "Idle"},
		{SessionActive, "Active"},
		{SessionPaused, "Paused"},
		{SessionFailed, "Failed"},
		{SessionClosed, "Closed"},
		{ManagedSessionState(99), "Unknown(99)"},
	}
	for _, tt := range tests {
		if got := ManagedSessionStateName(tt.state); got != tt.want {
			t.Errorf("ManagedSessionStateName(%d) = %q, want %q", int(tt.state), got, tt.want)
		}
	}
}

// --- DefaultManagedSessionConfig ---

func TestDefaultManagedSessionConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultManagedSessionConfig()
	if !cfg.Guard.RateLimit.Enabled {
		t.Error("Guard.RateLimit.Enabled should be true by default")
	}
	if !cfg.MCPGuard.FrequencyLimit.Enabled {
		t.Error("MCPGuard.FrequencyLimit.Enabled should be true by default")
	}
	if cfg.Supervisor.MaxRetries != 3 {
		t.Errorf("Supervisor.MaxRetries = %d, want 3", cfg.Supervisor.MaxRetries)
	}
}

// --- NewManagedSession ---

func TestNewManagedSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewManagedSession(ctx, "test-1", DefaultManagedSessionConfig())
	if s.ID() != "test-1" {
		t.Errorf("ID() = %q, want %q", s.ID(), "test-1")
	}
	if s.State() != SessionIdle {
		t.Errorf("State() = %s, want Idle", ManagedSessionStateName(s.State()))
	}
	snap := s.Snapshot()
	if snap.LinesProcessed != 0 {
		t.Errorf("LinesProcessed = %d, want 0", snap.LinesProcessed)
	}
}

// --- Start ---

func TestManagedSession_Start(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewManagedSession(ctx, "s1", DefaultManagedSessionConfig())
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if s.State() != SessionActive {
		t.Errorf("State() = %s, want Active", ManagedSessionStateName(s.State()))
	}
}

func TestManagedSession_StartFromNonIdle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewManagedSession(ctx, "s2", DefaultManagedSessionConfig())
	_ = s.Start()
	if err := s.Start(); err == nil {
		t.Error("expected error starting from Active")
	}
}

// --- ProcessLine ---

func TestManagedSession_ProcessLine_Text(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewManagedSession(ctx, "pl1", DefaultManagedSessionConfig())
	_ = s.Start()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	result := s.ProcessLine("Hello, world!", now)

	if result.Event.Type != EventText {
		t.Errorf("Event.Type = %s, want Text", EventTypeName(result.Event.Type))
	}
	if result.Action != "none" {
		t.Errorf("Action = %q, want none", result.Action)
	}
	if result.GuardEvent != nil {
		t.Error("GuardEvent should be nil for plain text")
	}

	snap := s.Snapshot()
	if snap.LinesProcessed != 1 {
		t.Errorf("LinesProcessed = %d, want 1", snap.LinesProcessed)
	}
	if snap.EventCounts["Text"] != 1 {
		t.Errorf("EventCounts[Text] = %d, want 1", snap.EventCounts["Text"])
	}
}

func TestManagedSession_ProcessLine_RateLimit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewManagedSession(ctx, "pl2", DefaultManagedSessionConfig())
	_ = s.Start()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	result := s.ProcessLine("You have been rate limited", now)

	if result.Event.Type != EventRateLimit {
		t.Errorf("Event.Type = %s, want RateLimit", EventTypeName(result.Event.Type))
	}
	if result.Action != "pause" {
		t.Errorf("Action = %q, want pause", result.Action)
	}
	if result.GuardEvent == nil {
		t.Fatal("GuardEvent should not be nil for rate limit")
	}

	// Session should transition to Paused.
	if s.State() != SessionPaused {
		t.Errorf("State() = %s, want Paused", ManagedSessionStateName(s.State()))
	}
}

func TestManagedSession_ProcessLine_Permission(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewManagedSession(ctx, "pl3", DefaultManagedSessionConfig())
	_ = s.Start()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	result := s.ProcessLine("Do you want to allow write? [Y/n]", now)

	if result.Event.Type != EventPermission {
		t.Errorf("Event.Type = %s, want Permission", EventTypeName(result.Event.Type))
	}
	if result.Action != "reject" {
		t.Errorf("Action = %q, want reject", result.Action)
	}
}

func TestManagedSession_ProcessLine_NotActive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewManagedSession(ctx, "pl4", DefaultManagedSessionConfig())
	// Idle — not started yet.

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	result := s.ProcessLine("Rate limited", now)

	// Should still parse but skip guard.
	if result.Event.Type != EventRateLimit {
		t.Errorf("Event.Type = %s, want RateLimit", EventTypeName(result.Event.Type))
	}
	if result.Action != "none" {
		t.Errorf("Action = %q, want none (not active)", result.Action)
	}
	if result.GuardEvent != nil {
		t.Error("GuardEvent should be nil when not active")
	}
}

// --- ProcessCrash ---

func TestManagedSession_ProcessCrash(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewManagedSession(ctx, "pc1", DefaultManagedSessionConfig())
	_ = s.Start()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ge, d := s.ProcessCrash(1, now)

	// Guard should report restart (crash.enabled=true).
	if ge == nil {
		t.Fatal("GuardEvent should not be nil for crash")
	}
	if ge.Action != GuardActionRestart {
		t.Errorf("ge.Action = %s, want Restart", GuardActionName(ge.Action))
	}

	// Supervisor should report force-kill for PTY crash.
	if d.Action != RecoveryForceKill {
		t.Errorf("d.Action = %s, want ForceKill", RecoveryActionName(d.Action))
	}
}

func TestManagedSession_ProcessCrash_Escalate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cfg := DefaultManagedSessionConfig()
	cfg.Guard.Crash.MaxRestarts = 0 // First crash → escalate from guard
	s := NewManagedSession(ctx, "pc2", cfg)
	_ = s.Start()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// First crash → guard escalates (0 restarts allowed).
	ge1, _ := s.ProcessCrash(1, now)
	if ge1.Action != GuardActionEscalate {
		t.Errorf("expected Escalate from guard, got %s", GuardActionName(ge1.Action))
	}

	// Session should be Failed after escalation.
	if s.State() != SessionFailed {
		t.Errorf("State() = %s, want Failed", ManagedSessionStateName(s.State()))
	}
}

// --- ProcessToolCall ---

func TestManagedSession_ProcessToolCall_Allowed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewManagedSession(ctx, "tc1", DefaultManagedSessionConfig())
	_ = s.Start()

	result := s.ProcessToolCall(MCPToolCall{
		ToolName:  "addFile",
		Arguments: "{}",
		Timestamp: time.Now(),
	})

	if result.Action != "none" {
		t.Errorf("Action = %q, want none", result.Action)
	}
	if result.GuardEvent != nil {
		t.Error("GuardEvent should be nil for allowed call")
	}
}

func TestManagedSession_ProcessToolCall_Blocked(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cfg := DefaultManagedSessionConfig()
	cfg.MCPGuard.ToolAllowlist.Enabled = true
	cfg.MCPGuard.ToolAllowlist.AllowedTools = []string{"addFile", "buildPrompt"}
	s := NewManagedSession(ctx, "tc2", cfg)
	_ = s.Start()

	result := s.ProcessToolCall(MCPToolCall{
		ToolName:  "deleteEverything",
		Arguments: "{}",
		Timestamp: time.Now(),
	})

	if result.Action != "reject" {
		t.Errorf("Action = %q, want reject", result.Action)
	}
	if result.GuardEvent == nil {
		t.Fatal("GuardEvent should not be nil for blocked call")
	}
}

// --- CheckTimeout ---

func TestManagedSession_CheckTimeout_NoTimeout(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewManagedSession(ctx, "ct1", DefaultManagedSessionConfig())
	_ = s.Start()

	// Feed an event so there's a last-event time.
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.ProcessLine("hello", now)

	// Check timeout 1 second later — should be fine.
	ge := s.CheckTimeout(now.Add(time.Second))
	if ge != nil {
		t.Errorf("expected no timeout after 1s, got %s", GuardActionName(ge.Action))
	}
}

func TestManagedSession_CheckTimeout_OutputTimeout(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cfg := DefaultManagedSessionConfig()
	cfg.Guard.OutputTimeout.Timeout = 100 * time.Millisecond
	s := NewManagedSession(ctx, "ct2", cfg)
	_ = s.Start()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.ProcessLine("hello", now)

	// Check timeout 200ms later — should trigger.
	ge := s.CheckTimeout(now.Add(200 * time.Millisecond))
	if ge == nil {
		t.Fatal("expected timeout event")
	}
	if ge.Action != GuardActionTimeout {
		t.Errorf("Action = %s, want Timeout", GuardActionName(ge.Action))
	}

	// Session should be Failed.
	if s.State() != SessionFailed {
		t.Errorf("State() = %s, want Failed", ManagedSessionStateName(s.State()))
	}
}

// --- HandleError ---

func TestManagedSession_HandleError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewManagedSession(ctx, "he1", DefaultManagedSessionConfig())
	_ = s.Start()

	d := s.HandleError("connection lost", ErrorClassPTYError)

	if d.Action != RecoveryRetry {
		t.Errorf("Action = %s, want Retry", RecoveryActionName(d.Action))
	}
}

func TestManagedSession_HandleError_Escalate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cfg := DefaultManagedSessionConfig()
	cfg.Supervisor.MaxRetries = 1
	s := NewManagedSession(ctx, "he2", cfg)
	_ = s.Start()

	// First error → Retry (retryCount=1 <= retryThreshold=1).
	d1 := s.HandleError("try1", ErrorClassPTYError)
	if d1.Action != RecoveryRetry {
		t.Fatalf("first error: Action = %s, want Retry", RecoveryActionName(d1.Action))
	}
	s.ConfirmRecovery()

	// Second error → Escalate (retryCount=2 > maxRetries=1).
	d2 := s.HandleError("try2", ErrorClassPTYError)
	if d2.Action != RecoveryEscalate {
		t.Errorf("second error: Action = %s, want Escalate", RecoveryActionName(d2.Action))
	}
	if s.State() != SessionFailed {
		t.Errorf("State() = %s, want Failed", ManagedSessionStateName(s.State()))
	}
}

// --- ConfirmRecovery ---

func TestManagedSession_ConfirmRecovery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewManagedSession(ctx, "cr1", DefaultManagedSessionConfig())
	_ = s.Start()

	// Trigger a recoverable error.
	s.HandleError("oops", ErrorClassPTYError)
	s.ConfirmRecovery()

	if s.State() != SessionActive {
		t.Errorf("State() = %s, want Active after recovery", ManagedSessionStateName(s.State()))
	}
}

// --- Resume ---

func TestManagedSession_Resume(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewManagedSession(ctx, "r1", DefaultManagedSessionConfig())
	_ = s.Start()

	// Trigger rate limit to pause.
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.ProcessLine("rate limited", now)
	if s.State() != SessionPaused {
		t.Fatalf("State() = %s, want Paused", ManagedSessionStateName(s.State()))
	}

	// Resume.
	if err := s.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if s.State() != SessionActive {
		t.Errorf("State() = %s, want Active", ManagedSessionStateName(s.State()))
	}
}

func TestManagedSession_Resume_NotPaused(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewManagedSession(ctx, "r2", DefaultManagedSessionConfig())
	_ = s.Start()

	if err := s.Resume(); err == nil {
		t.Error("expected error resuming from Active")
	}
}

// --- Shutdown + Close ---

func TestManagedSession_ShutdownAndClose(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewManagedSession(ctx, "sc1", DefaultManagedSessionConfig())
	_ = s.Start()

	d := s.Shutdown()
	if d.Action != RecoveryDrain {
		t.Errorf("Shutdown action = %s, want Drain", RecoveryActionName(d.Action))
	}

	s.Close()
	if s.State() != SessionClosed {
		t.Errorf("State() = %s, want Closed", ManagedSessionStateName(s.State()))
	}
}

// --- Snapshot ---

func TestManagedSession_Snapshot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewManagedSession(ctx, "snap1", DefaultManagedSessionConfig())
	_ = s.Start()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.ProcessLine("hello world", now)
	s.ProcessLine("Calling tool: readFile", now)
	s.ProcessLine("hello again", now)

	snap := s.Snapshot()
	if snap.ID != "snap1" {
		t.Errorf("ID = %q, want snap1", snap.ID)
	}
	if snap.StateName != "Active" {
		t.Errorf("StateName = %q, want Active", snap.StateName)
	}
	if snap.LinesProcessed != 3 {
		t.Errorf("LinesProcessed = %d, want 3", snap.LinesProcessed)
	}
	if snap.EventCounts["Text"] != 2 {
		t.Errorf("EventCounts[Text] = %d, want 2", snap.EventCounts["Text"])
	}
	if snap.EventCounts["ToolUse"] != 1 {
		t.Errorf("EventCounts[ToolUse] = %d, want 1", snap.EventCounts["ToolUse"])
	}
	if snap.LastEvent == nil {
		t.Fatal("LastEvent should not be nil")
	}
	if snap.LastEvent.Line != "hello again" {
		t.Errorf("LastEvent.Line = %q, want %q", snap.LastEvent.Line, "hello again")
	}
}

// --- Parser access ---

func TestManagedSession_Parser(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewManagedSession(ctx, "p1", DefaultManagedSessionConfig())

	p := s.Parser()
	if p == nil {
		t.Fatal("Parser() returned nil")
	}

	// Can add custom patterns.
	if err := p.AddPattern("test-custom", `test-marker`, EventCompletion); err != nil {
		t.Fatalf("AddPattern: %v", err)
	}

	_ = s.Start()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	result := s.ProcessLine("test-marker detected", now)
	if result.Event.Type != EventCompletion {
		t.Errorf("Type = %s, want Completion", EventTypeName(result.Event.Type))
	}
}

// --- Callbacks ---

func TestManagedSession_OnEvent_Callback(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewManagedSession(ctx, "cb1", DefaultManagedSessionConfig())

	var events []OutputEvent
	s.OnEvent = func(ev OutputEvent) {
		events = append(events, ev)
	}

	_ = s.Start()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.ProcessLine("hello", now)
	s.ProcessLine("world", now)

	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].Line != "hello" {
		t.Errorf("events[0].Line = %q, want hello", events[0].Line)
	}
}

func TestManagedSession_OnGuardAction_Callback(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewManagedSession(ctx, "cb2", DefaultManagedSessionConfig())

	var guardEvents []*GuardEvent
	s.OnGuardAction = func(ge *GuardEvent) {
		guardEvents = append(guardEvents, ge)
	}

	_ = s.Start()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.ProcessLine("rate limited", now)

	if len(guardEvents) != 1 {
		t.Fatalf("guardEvents = %d, want 1", len(guardEvents))
	}
	if guardEvents[0].Action != GuardActionPause {
		t.Errorf("Action = %s, want Pause", GuardActionName(guardEvents[0].Action))
	}
}

func TestManagedSession_OnRecoveryDecision_Callback(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewManagedSession(ctx, "cb3", DefaultManagedSessionConfig())

	var decisions []RecoveryDecision
	s.OnRecoveryDecision = func(d RecoveryDecision) {
		decisions = append(decisions, d)
	}

	_ = s.Start()
	s.HandleError("oops", ErrorClassPTYError)

	if len(decisions) != 1 {
		t.Fatalf("decisions = %d, want 1", len(decisions))
	}
	if decisions[0].Action != RecoveryRetry {
		t.Errorf("Action = %s, want Retry", RecoveryActionName(decisions[0].Action))
	}
}

// --- Concurrent access ---

func TestManagedSession_ConcurrentProcessLine(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewManagedSession(ctx, "conc1", DefaultManagedSessionConfig())
	_ = s.Start()

	var wg sync.WaitGroup
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			s.ProcessLine("concurrent line", now.Add(time.Duration(idx)*time.Millisecond))
		}(i)
	}
	wg.Wait()

	snap := s.Snapshot()
	if snap.LinesProcessed != 50 {
		t.Errorf("LinesProcessed = %d, want 50", snap.LinesProcessed)
	}
}

func TestManagedSession_ConcurrentMixed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewManagedSession(ctx, "conc2", DefaultManagedSessionConfig())
	_ = s.Start()

	var wg sync.WaitGroup
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Mix process lines, tool calls, snapshots, and timeout checks.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			s.ProcessLine("text line", now.Add(time.Duration(idx)*time.Millisecond))
		}(i)
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			s.ProcessToolCall(MCPToolCall{
				ToolName:  "addFile",
				Timestamp: now.Add(time.Duration(idx) * time.Millisecond),
			})
		}(i)
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = s.Snapshot()
		}(i)
	}

	wg.Wait()

	snap := s.Snapshot()
	if snap.LinesProcessed != 20 {
		t.Errorf("LinesProcessed = %d, want 20", snap.LinesProcessed)
	}
}

// --- guardActionToString ---

func TestGuardActionToString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		action GuardAction
		want   string
	}{
		{GuardActionNone, "none"},
		{GuardActionPause, "pause"},
		{GuardActionReject, "reject"},
		{GuardActionRestart, "restart"},
		{GuardActionEscalate, "escalate"},
		{GuardActionTimeout, "timeout"},
		{GuardAction(99), "unknown"},
	}
	for _, tt := range tests {
		got := guardActionToString(tt.action)
		if got != tt.want {
			t.Errorf("guardActionToString(%d) = %q, want %q", int(tt.action), got, tt.want)
		}
	}
}
