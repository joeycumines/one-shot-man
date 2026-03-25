package claudemux

import (
	"testing"
	"time"
)

// --- DefaultMCPGuardConfig ---

func TestDefaultMCPGuardConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultMCPGuardConfig()

	if !cfg.NoCallTimeout.Enabled {
		t.Error("NoCallTimeout.Enabled = false")
	}
	if cfg.NoCallTimeout.Timeout != 10*time.Minute {
		t.Errorf("NoCallTimeout.Timeout = %v, want 10m", cfg.NoCallTimeout.Timeout)
	}
	if !cfg.FrequencyLimit.Enabled {
		t.Error("FrequencyLimit.Enabled = false")
	}
	if cfg.FrequencyLimit.MaxCalls != 50 {
		t.Errorf("FrequencyLimit.MaxCalls = %d, want 50", cfg.FrequencyLimit.MaxCalls)
	}
	if !cfg.RepeatDetection.Enabled {
		t.Error("RepeatDetection.Enabled = false")
	}
	if cfg.RepeatDetection.MaxRepeats != 5 {
		t.Errorf("RepeatDetection.MaxRepeats = %d, want 5", cfg.RepeatDetection.MaxRepeats)
	}
	if cfg.ToolAllowlist.Enabled {
		t.Error("ToolAllowlist should be disabled by default")
	}
}

// --- NewMCPGuard ---

func TestNewMCPGuard(t *testing.T) {
	t.Parallel()
	g := NewMCPGuard(MCPGuardConfig{})
	if g == nil {
		t.Fatal("NewMCPGuard returned nil")
	}
	st := g.State()
	if st.Started {
		t.Error("new guard should not be started")
	}
	if st.TotalCalls != 0 {
		t.Errorf("TotalCalls = %d, want 0", st.TotalCalls)
	}
}

// --- No-Call Timeout ---

func TestMCPGuard_NoCallTimeout_Triggered(t *testing.T) {
	t.Parallel()
	g := NewMCPGuard(MCPGuardConfig{
		NoCallTimeout: MCPNoCallTimeoutConfig{
			Enabled: true,
			Timeout: 5 * time.Minute,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Process one call to start the clock.
	g.ProcessToolCall(MCPToolCall{
		ToolName:  "addFile",
		Timestamp: now,
	})

	// Before timeout: nil.
	ge := g.CheckNoCallTimeout(now.Add(4 * time.Minute))
	if ge != nil {
		t.Errorf("expected nil before timeout, got %+v", ge)
	}

	// At timeout: trigger.
	ge = g.CheckNoCallTimeout(now.Add(5 * time.Minute))
	if ge == nil {
		t.Fatal("expected timeout event")
	}
	if ge.Action != GuardActionTimeout {
		t.Errorf("Action = %v, want Timeout", GuardActionName(ge.Action))
	}
}

func TestMCPGuard_NoCallTimeout_ResetByCall(t *testing.T) {
	t.Parallel()
	g := NewMCPGuard(MCPGuardConfig{
		NoCallTimeout: MCPNoCallTimeoutConfig{
			Enabled: true,
			Timeout: 2 * time.Minute,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// First call at t=0.
	g.ProcessToolCall(MCPToolCall{ToolName: "addFile", Timestamp: now})

	// Second call at t=1m (resets clock).
	g.ProcessToolCall(MCPToolCall{ToolName: "listContext", Timestamp: now.Add(1 * time.Minute)})

	// At t=2.5m (1.5m since last call): no timeout.
	ge := g.CheckNoCallTimeout(now.Add(2*time.Minute + 30*time.Second))
	if ge != nil {
		t.Errorf("expected nil (1.5m since last call), got %+v", ge)
	}

	// At t=3.5m (2.5m since last call): timeout.
	ge = g.CheckNoCallTimeout(now.Add(3*time.Minute + 30*time.Second))
	if ge == nil || ge.Action != GuardActionTimeout {
		t.Error("expected timeout 2.5m after last call")
	}
}

func TestMCPGuard_NoCallTimeout_NotStarted(t *testing.T) {
	t.Parallel()
	g := NewMCPGuard(MCPGuardConfig{
		NoCallTimeout: MCPNoCallTimeoutConfig{
			Enabled: true,
			Timeout: 1 * time.Minute,
		},
	})

	ge := g.CheckNoCallTimeout(time.Now().Add(1 * time.Hour))
	if ge != nil {
		t.Errorf("expected nil for not-started, got %+v", ge)
	}
}

func TestMCPGuard_NoCallTimeout_Disabled(t *testing.T) {
	t.Parallel()
	g := NewMCPGuard(MCPGuardConfig{
		NoCallTimeout: MCPNoCallTimeoutConfig{Enabled: false},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	g.ProcessToolCall(MCPToolCall{ToolName: "a", Timestamp: now})

	ge := g.CheckNoCallTimeout(now.Add(1 * time.Hour))
	if ge != nil {
		t.Errorf("expected nil for disabled, got %+v", ge)
	}
}

func TestMCPGuard_NoCallTimeout_ZeroDuration(t *testing.T) {
	t.Parallel()
	g := NewMCPGuard(MCPGuardConfig{
		NoCallTimeout: MCPNoCallTimeoutConfig{
			Enabled: true,
			Timeout: 0,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	g.ProcessToolCall(MCPToolCall{ToolName: "a", Timestamp: now})

	ge := g.CheckNoCallTimeout(now.Add(1 * time.Hour))
	if ge != nil {
		t.Errorf("expected nil for zero timeout, got %+v", ge)
	}
}

// --- Frequency Limit ---

func TestMCPGuard_Frequency_Exceeded(t *testing.T) {
	t.Parallel()
	g := NewMCPGuard(MCPGuardConfig{
		FrequencyLimit: MCPFrequencyLimitConfig{
			Enabled:  true,
			Window:   5 * time.Second,
			MaxCalls: 3,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// 3 calls within 5s: fine.
	for i := range 3 {
		ge := g.ProcessToolCall(MCPToolCall{
			ToolName:  "addFile",
			Timestamp: now.Add(time.Duration(i) * time.Second),
		})
		if ge != nil {
			t.Errorf("call %d: unexpected event %+v", i+1, ge)
		}
	}

	// 4th call: exceeds MaxCalls=3.
	ge := g.ProcessToolCall(MCPToolCall{
		ToolName:  "addFile",
		Timestamp: now.Add(3 * time.Second),
	})
	if ge == nil {
		t.Fatal("expected frequency limit event")
	}
	if ge.Action != GuardActionPause {
		t.Errorf("Action = %v, want Pause", GuardActionName(ge.Action))
	}
	if ge.Details["callCount"] != "4" {
		t.Errorf("callCount = %q, want 4", ge.Details["callCount"])
	}
}

func TestMCPGuard_Frequency_WindowSlides(t *testing.T) {
	t.Parallel()
	g := NewMCPGuard(MCPGuardConfig{
		FrequencyLimit: MCPFrequencyLimitConfig{
			Enabled:  true,
			Window:   5 * time.Second,
			MaxCalls: 3,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// 3 calls at t=0, t=1, t=2.
	for i := range 3 {
		g.ProcessToolCall(MCPToolCall{
			ToolName:  "addFile",
			Timestamp: now.Add(time.Duration(i) * time.Second),
		})
	}

	// 4th call at t=10 (>5s after first calls): window slides, only 1 in window.
	ge := g.ProcessToolCall(MCPToolCall{
		ToolName:  "addFile",
		Timestamp: now.Add(10 * time.Second),
	})
	if ge != nil {
		t.Errorf("expected nil (window slid), got %+v", ge)
	}
}

func TestMCPGuard_Frequency_Disabled(t *testing.T) {
	t.Parallel()
	g := NewMCPGuard(MCPGuardConfig{
		FrequencyLimit: MCPFrequencyLimitConfig{Enabled: false},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 100 {
		ge := g.ProcessToolCall(MCPToolCall{
			ToolName:  "addFile",
			Timestamp: now,
		})
		if ge != nil {
			t.Errorf("call %d: unexpected event %+v", i, ge)
			break
		}
	}
}

// --- Repeat Detection ---

func TestMCPGuard_Repeat_Detected(t *testing.T) {
	t.Parallel()
	g := NewMCPGuard(MCPGuardConfig{
		RepeatDetection: MCPRepeatDetectionConfig{
			Enabled:      true,
			MaxRepeats:   3,
			WindowSize:   10,
			MatchTool:    true,
			MatchArgHash: true,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	call := MCPToolCall{
		ToolName:  "addFile",
		Arguments: `{"path":"test.go"}`,
		Timestamp: now,
	}

	// First 3 identical calls: fine (repeats=1,2,3 which is <=3).
	for i := range 3 {
		call.Timestamp = now.Add(time.Duration(i) * time.Second)
		ge := g.ProcessToolCall(call)
		if ge != nil {
			t.Errorf("call %d: unexpected event %+v", i+1, ge)
		}
	}

	// 4th: repeats=4 > MaxRepeats=3 → escalate.
	call.Timestamp = now.Add(3 * time.Second)
	ge := g.ProcessToolCall(call)
	if ge == nil {
		t.Fatal("expected repeat detection event")
	}
	if ge.Action != GuardActionEscalate {
		t.Errorf("Action = %v, want Escalate", GuardActionName(ge.Action))
	}
	if ge.Details["repeats"] != "4" {
		t.Errorf("repeats = %q, want 4", ge.Details["repeats"])
	}
}

func TestMCPGuard_Repeat_BrokenByDifferentTool(t *testing.T) {
	t.Parallel()
	g := NewMCPGuard(MCPGuardConfig{
		RepeatDetection: MCPRepeatDetectionConfig{
			Enabled:      true,
			MaxRepeats:   2,
			WindowSize:   10,
			MatchTool:    true,
			MatchArgHash: true,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// 2 identical calls (at limit).
	g.ProcessToolCall(MCPToolCall{ToolName: "addFile", Arguments: `{"a":1}`, Timestamp: now})
	g.ProcessToolCall(MCPToolCall{ToolName: "addFile", Arguments: `{"a":1}`, Timestamp: now.Add(time.Second)})

	// Different tool breaks the chain.
	g.ProcessToolCall(MCPToolCall{ToolName: "listContext", Arguments: `{}`, Timestamp: now.Add(2 * time.Second)})

	// Same tool again: only 1 consecutive now.
	ge := g.ProcessToolCall(MCPToolCall{ToolName: "addFile", Arguments: `{"a":1}`, Timestamp: now.Add(3 * time.Second)})
	if ge != nil {
		t.Errorf("expected nil (chain broken), got %+v", ge)
	}
}

func TestMCPGuard_Repeat_BrokenByDifferentArgs(t *testing.T) {
	t.Parallel()
	g := NewMCPGuard(MCPGuardConfig{
		RepeatDetection: MCPRepeatDetectionConfig{
			Enabled:      true,
			MaxRepeats:   2,
			WindowSize:   10,
			MatchTool:    true,
			MatchArgHash: true,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// 2 identical calls (at limit).
	g.ProcessToolCall(MCPToolCall{ToolName: "addFile", Arguments: `{"path":"a.go"}`, Timestamp: now})
	g.ProcessToolCall(MCPToolCall{ToolName: "addFile", Arguments: `{"path":"a.go"}`, Timestamp: now.Add(time.Second)})

	// Same tool, different args breaks chain.
	ge := g.ProcessToolCall(MCPToolCall{ToolName: "addFile", Arguments: `{"path":"b.go"}`, Timestamp: now.Add(2 * time.Second)})
	if ge != nil {
		t.Errorf("expected nil (different args), got %+v", ge)
	}
}

func TestMCPGuard_Repeat_ToolOnlyMatch(t *testing.T) {
	t.Parallel()
	g := NewMCPGuard(MCPGuardConfig{
		RepeatDetection: MCPRepeatDetectionConfig{
			Enabled:      true,
			MaxRepeats:   2,
			WindowSize:   10,
			MatchTool:    true,
			MatchArgHash: false, // ignore args
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// 2 calls with same tool, different args.
	g.ProcessToolCall(MCPToolCall{ToolName: "addFile", Arguments: `{"path":"a.go"}`, Timestamp: now})
	g.ProcessToolCall(MCPToolCall{ToolName: "addFile", Arguments: `{"path":"b.go"}`, Timestamp: now.Add(time.Second)})

	// 3rd: repeats=3 > MaxRepeats=2 → escalate (args ignored).
	ge := g.ProcessToolCall(MCPToolCall{ToolName: "addFile", Arguments: `{"path":"c.go"}`, Timestamp: now.Add(2 * time.Second)})
	if ge == nil {
		t.Fatal("expected repeat event (tool-only match)")
	}
	if ge.Action != GuardActionEscalate {
		t.Errorf("Action = %v, want Escalate", GuardActionName(ge.Action))
	}
}

func TestMCPGuard_Repeat_Disabled(t *testing.T) {
	t.Parallel()
	g := NewMCPGuard(MCPGuardConfig{
		RepeatDetection: MCPRepeatDetectionConfig{Enabled: false},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 100 {
		ge := g.ProcessToolCall(MCPToolCall{
			ToolName:  "addFile",
			Arguments: `{"same": true}`,
			Timestamp: now,
		})
		if ge != nil {
			t.Errorf("call %d: unexpected event %+v", i, ge)
			break
		}
	}
}

// --- Tool Allowlist ---

func TestMCPGuard_Allowlist_Permitted(t *testing.T) {
	t.Parallel()
	g := NewMCPGuard(MCPGuardConfig{
		ToolAllowlist: MCPToolAllowlistConfig{
			Enabled:      true,
			AllowedTools: []string{"addFile", "listContext", "buildPrompt"},
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, tool := range []string{"addFile", "listContext", "buildPrompt"} {
		ge := g.ProcessToolCall(MCPToolCall{ToolName: tool, Timestamp: now})
		if ge != nil {
			t.Errorf("tool %q: unexpected rejection %+v", tool, ge)
		}
	}
}

func TestMCPGuard_Allowlist_Rejected(t *testing.T) {
	t.Parallel()
	g := NewMCPGuard(MCPGuardConfig{
		ToolAllowlist: MCPToolAllowlistConfig{
			Enabled:      true,
			AllowedTools: []string{"addFile", "listContext"},
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ge := g.ProcessToolCall(MCPToolCall{ToolName: "deleteEverything", Timestamp: now})

	if ge == nil {
		t.Fatal("expected rejection for non-allowed tool")
	}
	if ge.Action != GuardActionReject {
		t.Errorf("Action = %v, want Reject", GuardActionName(ge.Action))
	}
	if ge.Details["toolName"] != "deleteEverything" {
		t.Errorf("toolName = %q", ge.Details["toolName"])
	}
}

func TestMCPGuard_Allowlist_Disabled(t *testing.T) {
	t.Parallel()
	g := NewMCPGuard(MCPGuardConfig{
		ToolAllowlist: MCPToolAllowlistConfig{Enabled: false},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ge := g.ProcessToolCall(MCPToolCall{ToolName: "anyTool", Timestamp: now})
	if ge != nil {
		t.Errorf("expected nil for disabled allowlist, got %+v", ge)
	}
}

func TestMCPGuard_Allowlist_EmptyList(t *testing.T) {
	t.Parallel()
	g := NewMCPGuard(MCPGuardConfig{
		ToolAllowlist: MCPToolAllowlistConfig{
			Enabled:      true,
			AllowedTools: nil, // empty
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// With Enabled=true but no allowedSet built (nil slice), should not reject.
	ge := g.ProcessToolCall(MCPToolCall{ToolName: "anyTool", Timestamp: now})
	if ge != nil {
		t.Errorf("expected nil for empty allowlist, got %+v", ge)
	}
}

// --- Priority Order ---

func TestMCPGuard_AllowlistBeforeFrequency(t *testing.T) {
	t.Parallel()
	g := NewMCPGuard(MCPGuardConfig{
		ToolAllowlist: MCPToolAllowlistConfig{
			Enabled:      true,
			AllowedTools: []string{"addFile"},
		},
		FrequencyLimit: MCPFrequencyLimitConfig{
			Enabled:  true,
			Window:   10 * time.Second,
			MaxCalls: 1,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// First call with allowed tool.
	g.ProcessToolCall(MCPToolCall{ToolName: "addFile", Timestamp: now})

	// Second call with disallowed tool — should be Reject (allowlist), not Pause (frequency).
	ge := g.ProcessToolCall(MCPToolCall{ToolName: "badTool", Timestamp: now.Add(time.Second)})
	if ge == nil {
		t.Fatal("expected event")
	}
	if ge.Action != GuardActionReject {
		t.Errorf("Action = %v, want Reject (allowlist takes priority)", GuardActionName(ge.Action))
	}
}

// --- State ---

func TestMCPGuard_State(t *testing.T) {
	t.Parallel()
	g := NewMCPGuard(MCPGuardConfig{})

	st := g.State()
	if st.Started || st.TotalCalls != 0 || st.RecentCount != 0 {
		t.Errorf("initial state: %+v", st)
	}

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	g.ProcessToolCall(MCPToolCall{ToolName: "x", Timestamp: now})

	st = g.State()
	if !st.Started {
		t.Error("Started should be true")
	}
	if st.TotalCalls != 1 {
		t.Errorf("TotalCalls = %d, want 1", st.TotalCalls)
	}
	if st.RecentCount != 1 {
		t.Errorf("RecentCount = %d, want 1", st.RecentCount)
	}
}

// --- Config ---

func TestMCPGuard_Config(t *testing.T) {
	t.Parallel()
	cfg := DefaultMCPGuardConfig()
	g := NewMCPGuard(cfg)

	got := g.Config()
	if got.NoCallTimeout.Timeout != cfg.NoCallTimeout.Timeout {
		t.Error("Config NoCallTimeout.Timeout mismatch")
	}
	if got.FrequencyLimit.MaxCalls != cfg.FrequencyLimit.MaxCalls {
		t.Error("Config FrequencyLimit.MaxCalls mismatch")
	}
}

// --- All Disabled ---

func TestMCPGuard_AllDisabled(t *testing.T) {
	t.Parallel()
	g := NewMCPGuard(MCPGuardConfig{})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 200 {
		ge := g.ProcessToolCall(MCPToolCall{
			ToolName:  "addFile",
			Arguments: `{"same": true}`,
			Timestamp: now,
		})
		if ge != nil {
			t.Errorf("call %d: unexpected event %+v", i, ge)
			break
		}
	}

	ge := g.CheckNoCallTimeout(now.Add(24 * time.Hour))
	if ge != nil {
		t.Errorf("timeout with all disabled: %+v", ge)
	}
}

// --- History Trimming ---

func TestMCPGuard_HistoryTrimming(t *testing.T) {
	t.Parallel()
	g := NewMCPGuard(MCPGuardConfig{
		RepeatDetection: MCPRepeatDetectionConfig{
			Enabled:    true,
			MaxRepeats: 200, // will never trigger
			WindowSize: 20,
		},
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Add 300 unique calls — history should be trimmed.
	for i := range 300 {
		g.ProcessToolCall(MCPToolCall{
			ToolName:  "tool" + time.Duration(i).String(),
			Timestamp: now.Add(time.Duration(i) * time.Millisecond),
		})
	}

	st := g.State()
	if st.RecentCount > 110 { // trimmed to ~100
		t.Errorf("RecentCount = %d, should be trimmed to ~100", st.RecentCount)
	}
}
