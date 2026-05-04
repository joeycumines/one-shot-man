package claudemux

import (
	"testing"
	"time"
)

// --- GuardActionName ---

func TestGuardActionName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		action GuardAction
		want   string
	}{
		{GuardActionNone, "None"},
		{GuardActionPause, "Pause"},
		{GuardActionReject, "Reject"},
		{GuardActionRestart, "Restart"},
		{GuardActionEscalate, "Escalate"},
		{GuardActionTimeout, "Timeout"},
		{GuardAction(99), "Unknown(99)"},
	}
	for _, tt := range tests {
		if got := GuardActionName(tt.action); got != tt.want {
			t.Errorf("GuardActionName(%d) = %q, want %q", int(tt.action), got, tt.want)
		}
	}
}

// --- DefaultGuardConfig ---

func TestDefaultGuardConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultGuardConfig()

	if !cfg.RateLimit.Enabled {
		t.Error("RateLimit.Enabled = false")
	}
	if cfg.RateLimit.InitialDelay != 5*time.Second {
		t.Errorf("RateLimit.InitialDelay = %v, want 5s", cfg.RateLimit.InitialDelay)
	}
	if cfg.RateLimit.MaxDelay != 5*time.Minute {
		t.Errorf("RateLimit.MaxDelay = %v, want 5m", cfg.RateLimit.MaxDelay)
	}
	if cfg.RateLimit.Multiplier != 2.0 {
		t.Errorf("RateLimit.Multiplier = %f, want 2.0", cfg.RateLimit.Multiplier)
	}
	if !cfg.Permission.Enabled {
		t.Error("Permission.Enabled = false")
	}
	if cfg.Permission.Policy != PermissionPolicyDeny {
		t.Errorf("Permission.Policy = %d, want Deny", cfg.Permission.Policy)
	}
	if !cfg.Crash.Enabled {
		t.Error("Crash.Enabled = false")
	}
	if cfg.Crash.MaxRestarts != 3 {
		t.Errorf("Crash.MaxRestarts = %d, want 3", cfg.Crash.MaxRestarts)
	}
	if !cfg.OutputTimeout.Enabled {
		t.Error("OutputTimeout.Enabled = false")
	}
	if cfg.OutputTimeout.Timeout != 5*time.Minute {
		t.Errorf("OutputTimeout.Timeout = %v, want 5m", cfg.OutputTimeout.Timeout)
	}
}

// --- NewGuard ---

func TestNewGuard(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{})
	if g == nil {
		t.Fatal("NewGuard returned nil")
	}
	st := g.State()
	if st.Started {
		t.Error("new guard should not be started")
	}
	if st.CrashCount != 0 {
		t.Errorf("CrashCount = %d, want 0", st.CrashCount)
	}
}

// --- Rate Limit Monitor ---

func TestGuard_RateLimit_Basic(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		RateLimit: RateLimitGuardConfig{
			Enabled:      true,
			InitialDelay: 1 * time.Second,
			MaxDelay:     1 * time.Minute,
			Multiplier:   2.0,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ev := OutputEvent{Type: EventRateLimit, Line: "Rate limit exceeded"}
	ge := g.ProcessEvent(ev, now)

	if ge == nil {
		t.Fatal("expected guard event")
	}
	if ge.Action != GuardActionPause {
		t.Errorf("Action = %v, want Pause", GuardActionName(ge.Action))
	}
	if ge.Details["rateLimitCount"] != "1" {
		t.Errorf("rateLimitCount = %q, want 1", ge.Details["rateLimitCount"])
	}
	// First rate limit: delay = initial * 2^0 = 1s.
	if ge.Details["delay"] != "1s" {
		t.Errorf("delay = %q, want 1s", ge.Details["delay"])
	}
}

func TestGuard_RateLimit_ExponentialBackoff(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		RateLimit: RateLimitGuardConfig{
			Enabled:      true,
			InitialDelay: 1 * time.Second,
			MaxDelay:     1 * time.Minute,
			Multiplier:   2.0,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ev := OutputEvent{Type: EventRateLimit, Line: "rate limit"}

	// 1st: 1s * 2^0 = 1s
	ge := g.ProcessEvent(ev, now)
	if ge.Details["delay"] != "1s" {
		t.Errorf("delay 1 = %q, want 1s", ge.Details["delay"])
	}

	// 2nd: 1s * 2^1 = 2s
	ge = g.ProcessEvent(ev, now.Add(time.Second))
	if ge.Details["delay"] != "2s" {
		t.Errorf("delay 2 = %q, want 2s", ge.Details["delay"])
	}

	// 3rd: 1s * 2^2 = 4s
	ge = g.ProcessEvent(ev, now.Add(2*time.Second))
	if ge.Details["delay"] != "4s" {
		t.Errorf("delay 3 = %q, want 4s", ge.Details["delay"])
	}

	// 4th: 1s * 2^3 = 8s
	ge = g.ProcessEvent(ev, now.Add(3*time.Second))
	if ge.Details["delay"] != "8s" {
		t.Errorf("delay 4 = %q, want 8s", ge.Details["delay"])
	}
}

func TestGuard_RateLimit_MaxDelayCap(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		RateLimit: RateLimitGuardConfig{
			Enabled:      true,
			InitialDelay: 10 * time.Second,
			MaxDelay:     30 * time.Second,
			Multiplier:   3.0,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ev := OutputEvent{Type: EventRateLimit, Line: "rate limit"}

	// 1st: 10s * 3^0 = 10s
	ge := g.ProcessEvent(ev, now)
	if ge.Details["delay"] != "10s" {
		t.Errorf("delay 1 = %q, want 10s", ge.Details["delay"])
	}

	// 2nd: 10s * 3^1 = 30s (hits cap)
	ge = g.ProcessEvent(ev, now.Add(time.Second))
	if ge.Details["delay"] != "30s" {
		t.Errorf("delay 2 = %q, want 30s", ge.Details["delay"])
	}

	// 3rd: 10s * 3^2 = 90s, capped to 30s
	ge = g.ProcessEvent(ev, now.Add(2*time.Second))
	if ge.Details["delay"] != "30s" {
		t.Errorf("delay 3 = %q, want 30s (capped)", ge.Details["delay"])
	}
}

func TestGuard_RateLimit_RetryAfterHint(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		RateLimit: RateLimitGuardConfig{
			Enabled:      true,
			InitialDelay: 1 * time.Second,
			MaxDelay:     5 * time.Minute,
			Multiplier:   2.0,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Event with retryAfter=60 (1 minute), which is greater than backoff 1s.
	ev := OutputEvent{
		Type:   EventRateLimit,
		Line:   "try again in 60",
		Fields: map[string]string{"retryAfter": "60"},
	}
	ge := g.ProcessEvent(ev, now)

	if ge == nil {
		t.Fatal("expected guard event")
	}
	// Should use the 60s hint since it's > 1s backoff.
	if ge.Details["delay"] != "1m0s" {
		t.Errorf("delay = %q, want 1m0s", ge.Details["delay"])
	}
}

func TestGuard_RateLimit_RetryAfterShorterIgnored(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		RateLimit: RateLimitGuardConfig{
			Enabled:      true,
			InitialDelay: 30 * time.Second,
			MaxDelay:     5 * time.Minute,
			Multiplier:   2.0,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// retryAfter=5 is shorter than backoff of 30s.
	ev := OutputEvent{
		Type:   EventRateLimit,
		Line:   "try again in 5",
		Fields: map[string]string{"retryAfter": "5"},
	}
	ge := g.ProcessEvent(ev, now)

	if ge.Details["delay"] != "30s" {
		t.Errorf("delay = %q, want 30s (backoff should win)", ge.Details["delay"])
	}
}

func TestGuard_RateLimit_ResetAfterIdle(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		RateLimit: RateLimitGuardConfig{
			Enabled:      true,
			InitialDelay: 1 * time.Second,
			MaxDelay:     1 * time.Minute,
			Multiplier:   2.0,
			ResetAfter:   5 * time.Second,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rlEv := OutputEvent{Type: EventRateLimit, Line: "rate limit"}
	textEv := OutputEvent{Type: EventText, Line: "hello"}

	// Trigger 3 rate limits to build up backoff.
	g.ProcessEvent(rlEv, now)
	g.ProcessEvent(rlEv, now.Add(1*time.Second))
	g.ProcessEvent(rlEv, now.Add(2*time.Second))

	st := g.State()
	if st.RateLimitCount != 3 {
		t.Fatalf("RateLimitCount = %d, want 3", st.RateLimitCount)
	}

	// Send a normal event 10s later (>5s ResetAfter).
	g.ProcessEvent(textEv, now.Add(12*time.Second))

	st = g.State()
	if st.RateLimitCount != 0 {
		t.Errorf("RateLimitCount after reset = %d, want 0", st.RateLimitCount)
	}
	if st.CurrentDelay != 0 {
		t.Errorf("CurrentDelay after reset = %v, want 0", st.CurrentDelay)
	}
}

func TestGuard_RateLimit_NoResetIfTooSoon(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		RateLimit: RateLimitGuardConfig{
			Enabled:      true,
			InitialDelay: 1 * time.Second,
			MaxDelay:     1 * time.Minute,
			Multiplier:   2.0,
			ResetAfter:   30 * time.Second,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rlEv := OutputEvent{Type: EventRateLimit, Line: "rate limit"}
	textEv := OutputEvent{Type: EventText, Line: "hello"}

	g.ProcessEvent(rlEv, now)
	// 10s later: NOT enough (< 30s ResetAfter).
	g.ProcessEvent(textEv, now.Add(10*time.Second))

	st := g.State()
	if st.RateLimitCount != 1 {
		t.Errorf("RateLimitCount = %d, want 1 (should not reset)", st.RateLimitCount)
	}
}

func TestGuard_RateLimit_Disabled(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		RateLimit: RateLimitGuardConfig{Enabled: false},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ev := OutputEvent{Type: EventRateLimit, Line: "rate limit"}
	ge := g.ProcessEvent(ev, now)

	if ge != nil {
		t.Errorf("expected nil for disabled rate limit, got %+v", ge)
	}
}

func TestGuard_RateLimit_ZeroMultiplier(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		RateLimit: RateLimitGuardConfig{
			Enabled:      true,
			InitialDelay: 1 * time.Second,
			MaxDelay:     1 * time.Minute,
			Multiplier:   0, // should default to 2.0
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ev := OutputEvent{Type: EventRateLimit, Line: "rate limit"}

	// 1st: 1s
	ge := g.ProcessEvent(ev, now)
	if ge.Details["delay"] != "1s" {
		t.Errorf("delay = %q, want 1s", ge.Details["delay"])
	}

	// 2nd: 2s (using default multiplier 2.0)
	ge = g.ProcessEvent(ev, now.Add(time.Second))
	if ge.Details["delay"] != "2s" {
		t.Errorf("delay = %q, want 2s", ge.Details["delay"])
	}
}

func TestGuard_RateLimit_ZeroInitialDelay(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		RateLimit: RateLimitGuardConfig{
			Enabled:      true,
			InitialDelay: 0,
			Multiplier:   2.0,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ev := OutputEvent{Type: EventRateLimit, Line: "rate limit"}
	ge := g.ProcessEvent(ev, now)

	if ge == nil {
		t.Fatal("expected guard event")
	}
	if ge.Details["delay"] != "0s" {
		t.Errorf("delay = %q, want 0s", ge.Details["delay"])
	}
}

// --- Permission Monitor ---

func TestGuard_Permission_DenyPolicy(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		Permission: PermissionGuardConfig{
			Enabled: true,
			Policy:  PermissionPolicyDeny,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ev := OutputEvent{
		Type:    EventPermission,
		Line:    "Allow? [y/N]",
		Pattern: "permission-yn",
	}
	ge := g.ProcessEvent(ev, now)

	if ge == nil {
		t.Fatal("expected guard event")
	}
	if ge.Action != GuardActionReject {
		t.Errorf("Action = %v, want Reject", GuardActionName(ge.Action))
	}
	if ge.Details["line"] != "Allow? [y/N]" {
		t.Errorf("Details[line] = %q", ge.Details["line"])
	}
	if ge.Details["pattern"] != "permission-yn" {
		t.Errorf("Details[pattern] = %q", ge.Details["pattern"])
	}
}

func TestGuard_Permission_AllowPolicy(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		Permission: PermissionGuardConfig{
			Enabled: true,
			Policy:  PermissionPolicyAllow,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ev := OutputEvent{Type: EventPermission, Line: "Allow? [y/N]"}
	ge := g.ProcessEvent(ev, now)

	if ge != nil {
		t.Errorf("expected nil for PermissionPolicyAllow, got %+v", ge)
	}
}

func TestGuard_Permission_UnknownPolicy_FailsClosed(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		Permission: PermissionGuardConfig{
			Enabled: true,
			Policy:  PermissionPolicy(99), // unknown
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ev := OutputEvent{Type: EventPermission, Line: "Allow? [y/N]"}
	ge := g.ProcessEvent(ev, now)

	if ge == nil {
		t.Fatal("expected guard event for unknown policy")
	}
	if ge.Action != GuardActionReject {
		t.Errorf("Action = %v, want Reject (fail-closed)", GuardActionName(ge.Action))
	}
}

func TestGuard_Permission_Disabled(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		Permission: PermissionGuardConfig{Enabled: false},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ev := OutputEvent{Type: EventPermission, Line: "Allow? [y/N]"}
	ge := g.ProcessEvent(ev, now)

	if ge != nil {
		t.Errorf("expected nil for disabled permission, got %+v", ge)
	}
}

// --- Crash Monitor ---

func TestGuard_Crash_Restart(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		Crash: CrashGuardConfig{
			Enabled:     true,
			MaxRestarts: 3,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// First crash: restart 1/3.
	ge := g.ProcessCrash(1, now)
	if ge == nil {
		t.Fatal("expected guard event")
	}
	if ge.Action != GuardActionRestart {
		t.Errorf("Action = %v, want Restart", GuardActionName(ge.Action))
	}
	if ge.Details["restartCount"] != "1" {
		t.Errorf("restartCount = %q, want 1", ge.Details["restartCount"])
	}
	if ge.Details["exitCode"] != "1" {
		t.Errorf("exitCode = %q, want 1", ge.Details["exitCode"])
	}
}

func TestGuard_Crash_EscalateAfterMax(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		Crash: CrashGuardConfig{
			Enabled:     true,
			MaxRestarts: 2,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// 1st and 2nd crash: restart.
	ge := g.ProcessCrash(1, now)
	if ge.Action != GuardActionRestart {
		t.Errorf("crash 1: %v, want Restart", GuardActionName(ge.Action))
	}
	ge = g.ProcessCrash(1, now.Add(time.Second))
	if ge.Action != GuardActionRestart {
		t.Errorf("crash 2: %v, want Restart", GuardActionName(ge.Action))
	}

	// 3rd crash: escalate.
	ge = g.ProcessCrash(1, now.Add(2*time.Second))
	if ge.Action != GuardActionEscalate {
		t.Errorf("crash 3: %v, want Escalate", GuardActionName(ge.Action))
	}
	if ge.Details["restartCount"] != "3" {
		t.Errorf("restartCount = %q, want 3", ge.Details["restartCount"])
	}
}

func TestGuard_Crash_ZeroMaxRestarts(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		Crash: CrashGuardConfig{
			Enabled:     true,
			MaxRestarts: 0, // no restarts allowed
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ge := g.ProcessCrash(137, now)

	if ge.Action != GuardActionEscalate {
		t.Errorf("Action = %v, want Escalate (zero max restarts)", GuardActionName(ge.Action))
	}
}

func TestGuard_Crash_ResetCount(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		Crash: CrashGuardConfig{
			Enabled:     true,
			MaxRestarts: 2,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Two crashes.
	g.ProcessCrash(1, now)
	g.ProcessCrash(1, now.Add(time.Second))

	// Reset after healthy restart confirmed.
	g.ResetCrashCount()

	st := g.State()
	if st.CrashCount != 0 {
		t.Errorf("CrashCount after reset = %d, want 0", st.CrashCount)
	}

	// Should be able to restart again (count reset).
	ge := g.ProcessCrash(1, now.Add(5*time.Second))
	if ge.Action != GuardActionRestart {
		t.Errorf("Action after reset = %v, want Restart", GuardActionName(ge.Action))
	}
}

func TestGuard_Crash_Disabled(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		Crash: CrashGuardConfig{Enabled: false},
	})

	ge := g.ProcessCrash(1, time.Now())
	if ge != nil {
		t.Errorf("expected nil for disabled crash, got %+v", ge)
	}
}

// --- Output Timeout Monitor ---

func TestGuard_Timeout_Triggered(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		OutputTimeout: OutputTimeoutGuardConfig{
			Enabled: true,
			Timeout: 5 * time.Minute,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Process an event to start the clock.
	g.ProcessEvent(OutputEvent{Type: EventText, Line: "hello"}, now)

	// Check before timeout: should be nil.
	ge := g.CheckTimeout(now.Add(4 * time.Minute))
	if ge != nil {
		t.Errorf("expected nil before timeout, got %+v", ge)
	}

	// Check at timeout: should trigger.
	ge = g.CheckTimeout(now.Add(5 * time.Minute))
	if ge == nil {
		t.Fatal("expected timeout event")
	}
	if ge.Action != GuardActionTimeout {
		t.Errorf("Action = %v, want Timeout", GuardActionName(ge.Action))
	}

	// Check well past timeout — should NOT fire again (once-per-arm).
	ge = g.CheckTimeout(now.Add(10 * time.Minute))
	if ge != nil {
		t.Errorf("expected nil after timeout already fired, got %+v", ge)
	}
}

func TestGuard_Timeout_ResetByEvent(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		OutputTimeout: OutputTimeoutGuardConfig{
			Enabled: true,
			Timeout: 2 * time.Minute,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Process event at t=0.
	g.ProcessEvent(OutputEvent{Type: EventText, Line: "hi"}, now)

	// At t=1m, process another event (resets clock).
	g.ProcessEvent(OutputEvent{Type: EventText, Line: "still here"}, now.Add(1*time.Minute))

	// At t=2.5m (1.5m since last event): no timeout.
	ge := g.CheckTimeout(now.Add(2*time.Minute + 30*time.Second))
	if ge != nil {
		t.Errorf("expected nil (only 1.5m since last event), got %+v", ge)
	}

	// At t=3.5m (2.5m since last event): timeout.
	ge = g.CheckTimeout(now.Add(3*time.Minute + 30*time.Second))
	if ge == nil || ge.Action != GuardActionTimeout {
		t.Error("expected timeout 2.5m after last event")
	}
}

func TestGuard_Timeout_NotStarted(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		OutputTimeout: OutputTimeoutGuardConfig{
			Enabled: true,
			Timeout: 1 * time.Minute,
		},
	})

	// No events processed yet — should not trigger timeout.
	ge := g.CheckTimeout(time.Now().Add(10 * time.Minute))
	if ge != nil {
		t.Errorf("expected nil for not-started guard, got %+v", ge)
	}
}

func TestGuard_Timeout_Disabled(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		OutputTimeout: OutputTimeoutGuardConfig{Enabled: false},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	g.ProcessEvent(OutputEvent{Type: EventText, Line: "hi"}, now)

	ge := g.CheckTimeout(now.Add(1 * time.Hour))
	if ge != nil {
		t.Errorf("expected nil for disabled timeout, got %+v", ge)
	}
}

func TestGuard_Timeout_ZeroDuration(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		OutputTimeout: OutputTimeoutGuardConfig{
			Enabled: true,
			Timeout: 0, // zero duration should not trigger
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	g.ProcessEvent(OutputEvent{Type: EventText, Line: "hi"}, now)

	ge := g.CheckTimeout(now.Add(1 * time.Hour))
	if ge != nil {
		t.Errorf("expected nil for zero timeout, got %+v", ge)
	}
}

// --- Integration: Parser + Guard ---

func TestGuard_Integration_ParserRateLimit(t *testing.T) {
	t.Parallel()
	parser := NewParser()
	g := NewGuard(DefaultGuardConfig())

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	lines := []string{
		"Processing...",
		"Rate limit exceeded. Try again in 30 seconds.",
		"Still processing...",
	}

	var guardEvents []*GuardEvent
	for i, line := range lines {
		ev := parser.Parse(line)
		ge := g.ProcessEvent(ev, now.Add(time.Duration(i)*time.Second))
		if ge != nil {
			guardEvents = append(guardEvents, ge)
		}
	}

	if len(guardEvents) != 1 {
		t.Fatalf("expected 1 guard event, got %d", len(guardEvents))
	}
	if guardEvents[0].Action != GuardActionPause {
		t.Errorf("Action = %v, want Pause", GuardActionName(guardEvents[0].Action))
	}
	// retryAfter=30 is greater than initial 5s, so it should be used.
	if guardEvents[0].Details["delay"] != "30s" {
		t.Errorf("delay = %q, want 30s (from retryAfter)", guardEvents[0].Details["delay"])
	}
}

func TestGuard_Integration_ParserPermission(t *testing.T) {
	t.Parallel()
	parser := NewParser()
	g := NewGuard(DefaultGuardConfig())

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	lines := []string{
		"Editing file...",
		"Do you want to allow this operation?",
		"Waiting for input...",
	}

	var guardEvents []*GuardEvent
	for i, line := range lines {
		ev := parser.Parse(line)
		ge := g.ProcessEvent(ev, now.Add(time.Duration(i)*time.Second))
		if ge != nil {
			guardEvents = append(guardEvents, ge)
		}
	}

	if len(guardEvents) != 1 {
		t.Fatalf("expected 1 guard event, got %d", len(guardEvents))
	}
	if guardEvents[0].Action != GuardActionReject {
		t.Errorf("Action = %v, want Reject", GuardActionName(guardEvents[0].Action))
	}
}

func TestGuard_Integration_FullScenario(t *testing.T) {
	t.Parallel()
	parser := NewParser()
	g := NewGuard(DefaultGuardConfig())

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Simulate a session: normal output, rate limit, recovery, permission.
	lines := []struct {
		line string
		dt   time.Duration
	}{
		{"Starting task...", 0},
		{"Calling tool: edit_file", 1 * time.Second},
		{"Rate limit exceeded", 2 * time.Second},
		{"rate limit again", 3 * time.Second},
		{"Resuming work...", 4 * time.Second},
		{"Allow? [y/N]", 5 * time.Second},
		{"task completed", 6 * time.Second},
	}

	var actions []GuardAction
	for _, l := range lines {
		ev := parser.Parse(l.line)
		ge := g.ProcessEvent(ev, now.Add(l.dt))
		if ge != nil {
			actions = append(actions, ge.Action)
		}
	}

	wantActions := []GuardAction{
		GuardActionPause,  // first rate limit
		GuardActionPause,  // second rate limit
		GuardActionReject, // permission prompt
	}

	if len(actions) != len(wantActions) {
		t.Fatalf("got %d actions, want %d: %v", len(actions), len(wantActions), actions)
	}
	for i, want := range wantActions {
		if actions[i] != want {
			t.Errorf("action[%d] = %v, want %v", i, GuardActionName(actions[i]), GuardActionName(want))
		}
	}
}

// --- State ---

func TestGuard_State(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		RateLimit: RateLimitGuardConfig{
			Enabled:      true,
			InitialDelay: 1 * time.Second,
			MaxDelay:     1 * time.Minute,
			Multiplier:   2.0,
		},
		Crash: CrashGuardConfig{
			Enabled:     true,
			MaxRestarts: 3,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Initial state.
	st := g.State()
	if st.Started || st.RateLimitCount != 0 || st.CrashCount != 0 {
		t.Errorf("initial state: %+v", st)
	}

	// Process a rate limit.
	g.ProcessEvent(OutputEvent{Type: EventRateLimit, Line: "rate limit"}, now)
	st = g.State()
	if !st.Started {
		t.Error("Started should be true after event")
	}
	if st.RateLimitCount != 1 {
		t.Errorf("RateLimitCount = %d, want 1", st.RateLimitCount)
	}
	if st.CurrentDelay != 1*time.Second {
		t.Errorf("CurrentDelay = %v, want 1s", st.CurrentDelay)
	}
	if st.LastEventTime != now {
		t.Errorf("LastEventTime = %v, want %v", st.LastEventTime, now)
	}

	// Process a crash.
	g.ProcessCrash(1, now.Add(time.Second))
	st = g.State()
	if st.CrashCount != 1 {
		t.Errorf("CrashCount = %d, want 1", st.CrashCount)
	}
}

// --- Config ---

func TestGuard_Config(t *testing.T) {
	t.Parallel()
	cfg := DefaultGuardConfig()
	g := NewGuard(cfg)

	got := g.Config()
	if got.RateLimit.InitialDelay != cfg.RateLimit.InitialDelay {
		t.Errorf("Config InitialDelay mismatch")
	}
	if got.Crash.MaxRestarts != cfg.Crash.MaxRestarts {
		t.Errorf("Config MaxRestarts mismatch")
	}
}

// --- Edge Cases ---

func TestGuard_NormalEvents_NilReturn(t *testing.T) {
	t.Parallel()
	g := NewGuard(DefaultGuardConfig())
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	normalEvents := []OutputEvent{
		{Type: EventText, Line: "hello"},
		{Type: EventToolUse, Line: "calling tool: read_file"},
		{Type: EventThinking, Line: "thinking..."},
		{Type: EventCompletion, Line: "task complete"},
		{Type: EventSSOLogin, Line: "opening browser"},
		{Type: EventModelSelect, Line: "select model"},
		{Type: EventError, Line: "error: something"},
	}

	for _, ev := range normalEvents {
		ge := g.ProcessEvent(ev, now)
		if ge != nil {
			t.Errorf("expected nil for event type %d (%q), got %+v",
				ev.Type, ev.Line, ge)
		}
	}
}

func TestGuard_AllDisabled(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{}) // all disabled

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Rate limit: no action.
	ge := g.ProcessEvent(OutputEvent{Type: EventRateLimit, Line: "429"}, now)
	if ge != nil {
		t.Errorf("rate limit with all disabled: %+v", ge)
	}

	// Permission: no action.
	ge = g.ProcessEvent(OutputEvent{Type: EventPermission, Line: "allow?"}, now)
	if ge != nil {
		t.Errorf("permission with all disabled: %+v", ge)
	}

	// Crash: no action.
	ge = g.ProcessCrash(1, now)
	if ge != nil {
		t.Errorf("crash with all disabled: %+v", ge)
	}

	// Timeout: no action (no events processed anyway, but also disabled).
	ge = g.CheckTimeout(now.Add(1 * time.Hour))
	if ge != nil {
		t.Errorf("timeout with all disabled: %+v", ge)
	}
}

// --- computeBackoff overflow edge cases ---

func TestGuard_ComputeBackoff_FloatOverflow_WithMaxDelay(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		RateLimit: RateLimitGuardConfig{
			Enabled:      true,
			InitialDelay: 1 * time.Second,
			MaxDelay:     5 * time.Minute,
			Multiplier:   2.0,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Drive rateLimitCount extremely high so math.Pow(2.0, count-1) → +Inf.
	// We need count > ~1024 for float64 overflow with mult=2.0.
	// Directly set the internal state instead of grinding 1100 events.
	g.rateLimitCount = 1100

	// ProcessEvent will call handleRateLimit → computeBackoff.
	// Since factor is +Inf, it should clamp to MaxDelay (5m).
	ge := g.ProcessEvent(OutputEvent{Type: EventRateLimit, Line: "rate limit"}, now)
	if ge == nil {
		t.Fatal("expected guard event for rate limit")
	}
	if ge.Action != GuardActionPause {
		t.Fatalf("Action = %v, want Pause", GuardActionName(ge.Action))
	}
	// With +Inf factor and MaxDelay=5m, delay should be clamped to MaxDelay.
	if ge.Details["delay"] != "5m0s" {
		t.Errorf("delay = %q, want 5m0s (MaxDelay)", ge.Details["delay"])
	}
}

func TestGuard_ComputeBackoff_FloatOverflow_NoMaxDelay(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		RateLimit: RateLimitGuardConfig{
			Enabled:      true,
			InitialDelay: 1 * time.Second,
			MaxDelay:     0, // no cap
			Multiplier:   2.0,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Drive rateLimitCount to overflow float64.
	g.rateLimitCount = 1100

	ge := g.ProcessEvent(OutputEvent{Type: EventRateLimit, Line: "rate limit"}, now)
	if ge == nil {
		t.Fatal("expected guard event for rate limit")
	}
	// With +Inf factor and no MaxDelay, should fall back to InitialDelay.
	if ge.Details["delay"] != "1s" {
		t.Errorf("delay = %q, want 1s (InitialDelay fallback)", ge.Details["delay"])
	}
}

func TestGuard_ComputeBackoff_Int64Overflow_WithMaxDelay(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		RateLimit: RateLimitGuardConfig{
			Enabled:      true,
			InitialDelay: 1 * time.Second,
			MaxDelay:     10 * time.Minute,
			Multiplier:   2.0,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// count=64 → factor = 2^63 ≈ 9.2e18. InitialDelay(1s) * 9.2e18 overflows
	// int64 (max ~9.2e18 ns). On modern Go (1.13+), float64→int64 saturates
	// to MaxInt64 (positive), which is then capped by MaxDelay.
	g.rateLimitCount = 64

	ge := g.ProcessEvent(OutputEvent{Type: EventRateLimit, Line: "rate limit"}, now)
	if ge == nil {
		t.Fatal("expected guard event for rate limit")
	}
	// Saturated/overflowed delay is caught and capped to MaxDelay.
	if ge.Details["delay"] != "10m0s" {
		t.Errorf("delay = %q, want 10m0s (MaxDelay on int64 overflow)", ge.Details["delay"])
	}
}

func TestGuard_ComputeBackoff_Int64Overflow_NoMaxDelay(t *testing.T) {
	t.Parallel()
	g := NewGuard(GuardConfig{
		RateLimit: RateLimitGuardConfig{
			Enabled:      true,
			InitialDelay: 1 * time.Second,
			MaxDelay:     0, // no cap
			Multiplier:   2.0,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// count=64 produces a float64 → int64 overflow. Modern Go saturates to
	// MaxInt64 (~292 years) instead of wrapping negative. The delay < 0
	// guard is defensive for exotic platforms; on modern Go this becomes
	// a very large positive Duration. Either way, it must not panic or
	// produce zero.
	g.rateLimitCount = 64

	ge := g.ProcessEvent(OutputEvent{Type: EventRateLimit, Line: "rate limit"}, now)
	if ge == nil {
		t.Fatal("expected guard event for rate limit")
	}
	if ge.Action != GuardActionPause {
		t.Fatalf("Action = %v, want Pause", GuardActionName(ge.Action))
	}
	// The delay will be either InitialDelay (if negative was detected) or
	// a very large positive value (MaxInt64 saturated). Both are acceptable.
	// Just ensure it's positive and non-zero.
	delayStr := ge.Details["delay"]
	if delayStr == "" || delayStr == "0s" {
		t.Errorf("delay = %q, want non-zero positive delay", delayStr)
	}
}

func TestGuard_ComputeBackoff_NaNFactor(t *testing.T) {
	t.Parallel()
	// NaN can occur if multiplier is negative and count produces a
	// non-integer exponent. math.Pow(-2, 0.5) → NaN.
	// But since count-1 is always an integer, we need a special case.
	// Actually math.Pow(negative, integer) does NOT produce NaN — it works.
	// NaN can also occur with 0^0 edge, but that gives 1.
	// The NaN guard is defensive; test it by directly calling computeBackoff
	// after setting state that would trigger the NaN branch.
	// Since we're in the same package, we can set fields directly.
	g := &Guard{
		config: GuardConfig{
			RateLimit: RateLimitGuardConfig{
				Enabled:      true,
				InitialDelay: 1 * time.Second,
				MaxDelay:     30 * time.Second,
				Multiplier:   2.0,
			},
		},
		// We'll override rateLimitCount to something that combined with
		// a special factor could produce NaN. Since it's hard to trigger
		// NaN naturally, we test the code path with float overflow instead
		// which exercises the same guard clause.
	}
	g.rateLimitCount = 1100
	delay := g.computeBackoff()
	// factor is +Inf, MaxDelay is 30s, should return 30s.
	if delay != 30*time.Second {
		t.Errorf("delay = %v, want 30s", delay)
	}
}
