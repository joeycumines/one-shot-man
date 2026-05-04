package claudemux

import (
	"testing"
	"time"
)

// FuzzParseOutput fuzzes the Parser with arbitrary input lines. The parser
// must never panic regardless of input; the result must always have a valid
// EventType (0–8) and a non-empty Line field.
func FuzzParseOutput(f *testing.F) {
	seeds := []string{
		"",
		"hello world",
		"Try again in 30 seconds",
		"allow? [y/N]",
		"Allow tool access? [Y/n]",
		"Select a model",
		"Opening your browser to authenticate",
		"Task completed",
		"Calling tool: readFile",
		"Error: file not found",
		"Thinking...",
		"⠋ Working...",
		"rate limit exceeded, retrying in 60s",
		"\x00\xff\xfe",
		"日本語テスト",
		"\n\r\t",
		"a]]][[[]]]",
		"aaaaaaaaaa" + string(make([]byte, 4096)),
		"claude-mux: startup validation",
		"Permission denied: cannot access /etc/shadow",
		"SSO login required",
		"model-item-selected: gpt-4o",
		"`" + "`" + "`" + "backticks" + "`" + "`" + "`",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	parser := NewParser()

	f.Fuzz(func(t *testing.T, line string) {
		ev := parser.Parse(line)

		// Invariant 1: Event type must be in valid range.
		if ev.Type < EventText || ev.Type > EventThinking {
			t.Fatalf("event type %d out of range [%d, %d]; line=%q",
				ev.Type, EventText, EventThinking, line)
		}

		// Invariant 2: Line must be preserved.
		if ev.Line != line {
			t.Fatalf("event.Line != input; got %q, want %q", ev.Line, line)
		}

		// Invariant 3: EventTypeName must not panic and must return non-empty.
		name := EventTypeName(ev.Type)
		if name == "" {
			t.Fatalf("EventTypeName(%d) returned empty; line=%q", ev.Type, line)
		}

		// Invariant 4: If no pattern matched, type must be EventText and Pattern empty.
		if ev.Pattern == "" && ev.Type != EventText {
			t.Fatalf("empty pattern but type=%d (expected EventText); line=%q", ev.Type, line)
		}
	})
}

// FuzzGuardRuleEval fuzzes the Guard with arbitrary OutputEvents. The guard
// must never panic; returned actions must be valid.
func FuzzGuardRuleEval(f *testing.F) {
	// Seed with representative event types.
	for i := int(EventText); i <= int(EventThinking); i++ {
		f.Add(i, "test line", 1000) // eventType, line, timeDeltaMs
	}
	f.Add(int(EventRateLimit), "Try again in 30 seconds", 0)
	f.Add(int(EventPermission), "Allow? [y/N]", 500)
	f.Add(int(EventError), "Error: catastrophic failure", 100)

	f.Fuzz(func(t *testing.T, eventType int, line string, timeDeltaMs int) {
		if eventType < int(EventText) || eventType > int(EventThinking) {
			t.Skip("out of range event type")
		}

		guard := NewGuard(DefaultGuardConfig())

		ev := OutputEvent{
			Type: EventType(eventType),
			Line: line,
		}

		// Use a fixed base time + delta to avoid time-related panics.
		base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		if timeDeltaMs < 0 {
			timeDeltaMs = 0
		}
		if timeDeltaMs > 86400000 { // cap at 24h
			timeDeltaMs = 86400000
		}
		now := base.Add(time.Duration(timeDeltaMs) * time.Millisecond)

		// Must not panic.
		ge := guard.ProcessEvent(ev, now)

		if ge != nil {
			// Action must be in valid range.
			if ge.Action < GuardActionNone || ge.Action > GuardActionTimeout {
				t.Fatalf("guard action %d out of range; ev=%+v", ge.Action, ev)
			}

			// ActionName must not panic.
			name := GuardActionName(ge.Action)
			if name == "" {
				t.Fatalf("GuardActionName(%d) returned empty", ge.Action)
			}
		}

		// State must not panic.
		state := guard.State()
		_ = state
	})
}

// FuzzMCPPayload fuzzes the MCPGuard with arbitrary tool call payloads.
// The guard must never panic.
func FuzzMCPPayload(f *testing.F) {
	seeds := []struct {
		toolName string
		args     string
	}{
		{"readFile", `{"path": "/tmp/test.go"}`},
		{"writeFile", `{"path": "/etc/passwd", "content": "hack"}`},
		{"", ""},
		{"readFile", ""},
		{"readFile", `{}`},
		{"readFile", `{"nested": {"deep": {"value": true}}}`},
		{"readFile", string(make([]byte, 10000))},
		{"\x00\xff", "\x00\xff"},
		{"tool-with-dashes", `{"key":"value"}`},
		{"readFile", `{"path": "/tmp/test.go"}`},
		{"readFile", `{"path": "/tmp/test.go"}`}, // duplicate to test repeat detection
	}
	for _, s := range seeds {
		f.Add(s.toolName, s.args)
	}

	f.Fuzz(func(t *testing.T, toolName, args string) {
		guard := NewMCPGuard(DefaultMCPGuardConfig())

		now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

		// Process the same call multiple times (test repeat detection too).
		for range 10 {
			call := MCPToolCall{
				ToolName:  toolName,
				Arguments: args,
				Timestamp: now,
			}

			// Must not panic.
			result := guard.ProcessToolCall(call)
			_ = result

			now = now.Add(100 * time.Millisecond)
		}

		// CheckNoCallTimeout must not panic.
		far := now.Add(24 * time.Hour)
		_ = guard.CheckNoCallTimeout(far)

		// State must not panic.
		state := guard.State()
		_ = state
	})
}

// FuzzSafetyClassify fuzzes the SafetyValidator with arbitrary tool names
// and argument strings. Must never panic.
func FuzzSafetyClassify(f *testing.F) {
	seeds := []struct {
		actionType string
		name       string
		raw        string
	}{
		{"tool_call", "readFile", "/tmp/test.go"},
		{"tool_call", "writeFile", "/etc/passwd"},
		{"command", "exec", "rm -rf /"},
		{"tool_call", "fetch", "https://example.com"},
		{"", "", ""},
		{"tool_call", "readFile", ""},
		{"tool_call", "\x00\xff", "\x00\xff"},
		{"file_delete", "deleteFile", "/home/user/.ssh/id_rsa"},
		{"file_write", "writeFile", "/tmp/safe.txt"},
		{"command", "exec", "go test ./..."},
		{"tool_call", "env_get", "AWS_SECRET_ACCESS_KEY"},
		{"tool_call", "curl", "http://169.254.169.254/latest/meta-data/"},
	}
	for _, s := range seeds {
		f.Add(s.actionType, s.name, s.raw)
	}

	f.Fuzz(func(t *testing.T, actionType, name, raw string) {
		cfg := DefaultSafetyConfig()
		validator := NewSafetyValidator(cfg)

		action := SafetyAction{
			Type: actionType,
			Name: name,
			Raw:  raw,
		}

		// Must not panic.
		result := validator.Validate(action)

		// Invariant 1: Intent must be in valid range.
		if result.Intent < IntentUnknown || result.Intent > IntentCredential {
			t.Fatalf("intent %d out of range; action=%+v", result.Intent, action)
		}

		// Invariant 2: Scope must be in valid range.
		if result.Scope < ScopeUnknown || result.Scope > ScopeInfra {
			t.Fatalf("scope %d out of range; action=%+v", result.Scope, action)
		}

		// Invariant 3: Risk must be in [0, 1].
		if result.RiskScore < 0 || result.RiskScore > 1.0 {
			t.Fatalf("risk %f out of [0, 1]; action=%+v", result.RiskScore, action)
		}

		// Invariant 4: Action must be in valid range.
		if result.Action < PolicyAllow || result.Action > PolicyBlock {
			t.Fatalf("action %d out of range; action input=%+v", result.Action, action)
		}

		// Invariant 5: Name helpers must not panic and return non-empty.
		if IntentName(result.Intent) == "" {
			t.Fatalf("IntentName(%d) returned empty", result.Intent)
		}
		if ScopeName(result.Scope) == "" {
			t.Fatalf("ScopeName(%d) returned empty", result.Scope)
		}
		if PolicyActionName(result.Action) == "" {
			t.Fatalf("PolicyActionName(%d) returned empty", result.Action)
		}

		// Stats must not panic.
		stats := validator.Stats()
		if stats.TotalChecks == 0 {
			t.Fatal("stats.TotalChecks should be > 0 after Validate call")
		}
	})
}
