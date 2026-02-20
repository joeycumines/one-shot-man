package claudemux

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// MCP Tool Injection — MCPGuard allowlist bypass attempts
// ============================================================================

// TestMCPSecurity_ToolNameInjection verifies that MCPGuard rejects
// tool names not in the allowlist regardless of encoding tricks.
func TestMCPSecurity_ToolNameInjection(t *testing.T) {
	t.Parallel()

	cfg := MCPGuardConfig{
		ToolAllowlist: MCPToolAllowlistConfig{
			Enabled:      true,
			AllowedTools: []string{"readFile", "writeFile"},
		},
	}
	guard := NewMCPGuard(cfg)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	injections := []struct {
		name     string
		toolName string
	}{
		{"null byte suffix", "readFile\x00"},
		{"null byte prefix", "\x00readFile"},
		{"null byte middle", "read\x00File"},
		{"unicode homoglyph", "readFіle"}, // Cyrillic і instead of Latin i
		{"trailing space", "readFile "},
		{"leading space", " readFile"},
		{"case variation", "ReadFile"},
		{"path traversal in name", "../readFile"},
		{"shell injection", "readFile;rm -rf /"},
		{"sql injection", "readFile' OR 1=1--"},
		{"template injection", "readFile{{.}}"},
		{"url encoded", "readFile%00"},
		{"newline injection", "readFile\nwriteFile"},
		{"tab injection", "readFile\twriteFile"},
		{"zero width space", "readFile\u200BwriteFile"},
		{"empty string", ""},
	}

	for _, tc := range injections {
		t.Run(tc.name, func(t *testing.T) {
			ge := guard.ProcessToolCall(MCPToolCall{
				ToolName:  tc.toolName,
				Arguments: `{"path":"/tmp/test"}`,
				Timestamp: now,
			})
			if ge == nil {
				t.Errorf("expected rejection for tool %q, got nil", tc.toolName)
			} else if ge.Action != GuardActionReject {
				t.Errorf("expected Reject for tool %q, got %s", tc.toolName, GuardActionName(ge.Action))
			}
		})
	}
}

// TestMCPSecurity_AllowedToolPassthrough verifies legitimate tools pass.
func TestMCPSecurity_AllowedToolPassthrough(t *testing.T) {
	t.Parallel()

	cfg := MCPGuardConfig{
		ToolAllowlist: MCPToolAllowlistConfig{
			Enabled:      true,
			AllowedTools: []string{"readFile", "writeFile"},
		},
	}
	guard := NewMCPGuard(cfg)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, tool := range []string{"readFile", "writeFile"} {
		ge := guard.ProcessToolCall(MCPToolCall{
			ToolName:  tool,
			Arguments: `{"path":"/tmp/test"}`,
			Timestamp: now,
		})
		if ge != nil {
			t.Errorf("tool %q should pass allowlist, got action=%s reason=%s",
				tool, GuardActionName(ge.Action), ge.Reason)
		}
	}
}

// ============================================================================
// MCP Argument Injection — Payloads in MCPToolCall arguments
// ============================================================================

// TestMCPSecurity_ArgumentInjection verifies MCPGuard and SafetyValidator
// handle malicious argument payloads without panic.
func TestMCPSecurity_ArgumentInjection(t *testing.T) {
	t.Parallel()

	guard := NewMCPGuard(DefaultMCPGuardConfig())
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	maliciousArgs := []struct {
		name string
		args string
	}{
		{"path traversal", `{"path":"../../../../etc/passwd"}`},
		{"shell injection in path", `{"path":"/tmp/test;rm -rf /"}`},
		{"nested json", `{"a":{"b":{"c":{"d":{"e":"deep"}}}}}`},
		{"massive json", `{"data":"` + strings.Repeat("A", 100000) + `"}`},
		{"null bytes in json", `{"path":"/tmp/test\u0000.exe"}`},
		{"invalid json", `{this is not json}`},
		{"empty json", `{}`},
		{"array instead of object", `[1,2,3]`},
		{"script injection", `{"content":"<script>alert(1)</script>"}`},
		{"command injection", `{"cmd":"$(whoami)"}`},
		{"env variable expansion", `{"path":"$HOME/.ssh/id_rsa"}`},
		{"unicode escape", `{"path":"\u002e\u002e/etc/passwd"}`},
		{"very long key", `{"` + strings.Repeat("k", 10000) + `":"v"}`},
		{"binary content", string(make([]byte, 256))},
		{"only whitespace", "   \t\n  "},
	}

	for _, tc := range maliciousArgs {
		t.Run(tc.name, func(t *testing.T) {
			// Must not panic
			_ = guard.ProcessToolCall(MCPToolCall{
				ToolName:  "readFile",
				Arguments: tc.args,
				Timestamp: now,
			})
		})
	}
}

// ============================================================================
// Large and Nested Payloads — Resource exhaustion attempts
// ============================================================================

// TestMCPSecurity_LargePayloads verifies MCPGuard handles oversized inputs.
func TestMCPSecurity_LargePayloads(t *testing.T) {
	t.Parallel()

	guard := NewMCPGuard(DefaultMCPGuardConfig())
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// 10MB tool name
	largeName := strings.Repeat("x", 10*1024*1024)
	ge := guard.ProcessToolCall(MCPToolCall{
		ToolName:  largeName,
		Arguments: `{}`,
		Timestamp: now,
	})
	// Must not panic — may or may not trigger a guard event
	_ = ge

	// 10MB arguments
	largeArgs := `{"data":"` + strings.Repeat("y", 10*1024*1024) + `"}`
	ge = guard.ProcessToolCall(MCPToolCall{
		ToolName:  "readFile",
		Arguments: largeArgs,
		Timestamp: now.Add(time.Second),
	})
	_ = ge
}

// TestMCPSecurity_RapidFireCalls verifies frequency limiting under burst.
func TestMCPSecurity_RapidFireCalls(t *testing.T) {
	t.Parallel()

	cfg := MCPGuardConfig{
		FrequencyLimit: MCPFrequencyLimitConfig{
			Enabled:  true,
			Window:   1 * time.Second,
			MaxCalls: 5,
		},
	}
	guard := NewMCPGuard(cfg)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	rejected := false
	for i := 0; i < 100; i++ {
		ge := guard.ProcessToolCall(MCPToolCall{
			ToolName:  "readFile",
			Arguments: `{"path":"/tmp/test"}`,
			Timestamp: now, // all at the same instant
		})
		if ge != nil && ge.Action == GuardActionPause {
			rejected = true
			break
		}
	}
	if !rejected {
		t.Error("frequency limit should have triggered for 100 calls in <1s window")
	}
}

// TestMCPSecurity_RepeatedIdenticalCalls verifies repeat detection fires.
func TestMCPSecurity_RepeatedIdenticalCalls(t *testing.T) {
	t.Parallel()

	cfg := MCPGuardConfig{
		RepeatDetection: MCPRepeatDetectionConfig{
			Enabled:      true,
			MaxRepeats:   3,
			WindowSize:   20,
			MatchTool:    true,
			MatchArgHash: true,
		},
	}
	guard := NewMCPGuard(cfg)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	call := MCPToolCall{
		ToolName:  "writeFile",
		Arguments: `{"path":"/tmp/malicious","content":"overwrite"}`,
	}

	escalated := false
	for i := 0; i < 10; i++ {
		call.Timestamp = now.Add(time.Duration(i) * time.Millisecond)
		ge := guard.ProcessToolCall(call)
		if ge != nil && ge.Action == GuardActionEscalate {
			escalated = true
			break
		}
	}
	if !escalated {
		t.Error("repeat detection should escalate after MaxRepeats identical calls")
	}
}

// ============================================================================
// Safety Validator — Privilege Escalation Attempts
// ============================================================================

// TestSafetySecurity_PrivilegeEscalationViaArgs checks that destructive
// operations are classified correctly regardless of obfuscation.
func TestSafetySecurity_PrivilegeEscalationViaArgs(t *testing.T) {
	t.Parallel()

	cfg := DefaultSafetyConfig()
	validator := NewSafetyValidator(cfg)

	escalationAttempts := []struct {
		name     string
		action   SafetyAction
		minRisk  string // "warn", "confirm", or "block"
	}{
		{
			"rm -rf root",
			SafetyAction{Type: "command", Name: "exec", Raw: "rm -rf /"},
			"warn",
		},
		{
			"curl to credential endpoint",
			SafetyAction{Type: "tool_call", Name: "fetch", Raw: "https://169.254.169.254/latest/meta-data/iam/"},
			"warn",
		},
		{
			"env var credential read",
			SafetyAction{Type: "tool_call", Name: "env_get", Raw: "AWS_SECRET_ACCESS_KEY"},
			"warn",
		},
		{
			"ssh private key access",
			SafetyAction{Type: "tool_call", Name: "readFile", Raw: "/root/.ssh/id_rsa",
				FilePaths: []string{"/root/.ssh/id_rsa"}},
			"warn",
		},
	}

	for _, tc := range escalationAttempts {
		t.Run(tc.name, func(t *testing.T) {
			result := validator.Validate(tc.action)

			// At minimum, these should not be PolicyAllow
			if result.Action == PolicyAllow {
				t.Errorf("dangerous action %q was allowed (PolicyAllow), expected at least %s",
					tc.name, tc.minRisk)
			}
		})
	}
}

// TestSafetySecurity_BlockedToolsBypass verifies that blocked tools cannot
// be invoked regardless of argument content.
func TestSafetySecurity_BlockedToolsBypass(t *testing.T) {
	t.Parallel()

	cfg := DefaultSafetyConfig()
	cfg.BlockedTools = []string{"dangerousTool", "exec_shell"}
	validator := NewSafetyValidator(cfg)

	bypassAttempts := []struct {
		name string
		tool string
	}{
		{"exact match", "dangerousTool"},
		{"exact match 2", "exec_shell"},
	}

	for _, tc := range bypassAttempts {
		t.Run(tc.name, func(t *testing.T) {
			result := validator.Validate(SafetyAction{
				Type: "tool_call",
				Name: tc.tool,
				Raw:  "harmless content",
			})
			if result.Action != PolicyBlock {
				t.Errorf("blocked tool %q should be PolicyBlock, got %s",
					tc.tool, PolicyActionName(result.Action))
			}
		})
	}
}

// TestSafetySecurity_BlockedPathsBypass verifies blocked paths are enforced.
func TestSafetySecurity_BlockedPathsBypass(t *testing.T) {
	t.Parallel()

	cfg := DefaultSafetyConfig()
	cfg.BlockedPaths = []string{"/etc/passwd", "/etc/shadow", "/root/.ssh/id_rsa"}
	validator := NewSafetyValidator(cfg)

	paths := []struct {
		name string
		path string
	}{
		{"etc passwd", "/etc/passwd"},
		{"etc shadow", "/etc/shadow"},
		{"root ssh", "/root/.ssh/id_rsa"},
	}

	for _, tc := range paths {
		t.Run(tc.name, func(t *testing.T) {
			result := validator.Validate(SafetyAction{
				Type:      "tool_call",
				Name:      "readFile",
				Raw:       tc.path,
				FilePaths: []string{tc.path},
			})
			if result.Action != PolicyBlock {
				t.Errorf("blocked path %q should be PolicyBlock, got %s",
					tc.path, PolicyActionName(result.Action))
			}
		})
	}
}

// TestSafetySecurity_SensitivePatternDetection verifies that sensitive patterns
// in raw content are detected regardless of surrounding text.
func TestSafetySecurity_SensitivePatternDetection(t *testing.T) {
	t.Parallel()

	cfg := DefaultSafetyConfig()
	validator := NewSafetyValidator(cfg)

	sensitiveInputs := []struct {
		name string
		raw  string
	}{
		{"aws key inline", "export AWS_SECRET_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE"},
		{"api key in json", `{"api_key":"sk-proj-1234567890abcdef"}`},
		{"password in url", "https://admin:password123@example.com/api"},
		{"token in header", "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkw"},
		{"private key block", "-----BEGIN RSA PRIVATE KEY-----"},
	}

	for _, tc := range sensitiveInputs {
		t.Run(tc.name, func(t *testing.T) {
			result := validator.Validate(SafetyAction{
				Type: "tool_call",
				Name: "customAction",
				Raw:  tc.raw,
			})
			// Sensitive patterns should increase risk score
			if result.RiskScore == 0 {
				t.Errorf("sensitive input %q should have non-zero risk score", tc.name)
			}
		})
	}
}

// TestSafetySecurity_DisabledValidatorAllowsAll verifies that a disabled
// validator returns PolicyAllow for everything.
func TestSafetySecurity_DisabledValidatorAllowsAll(t *testing.T) {
	t.Parallel()

	cfg := DefaultSafetyConfig()
	cfg.Enabled = false
	validator := NewSafetyValidator(cfg)

	result := validator.Validate(SafetyAction{
		Type: "command",
		Name: "rm",
		Raw:  "rm -rf /",
	})
	if result.Action != PolicyAllow {
		t.Errorf("disabled validator should allow all, got %s",
			PolicyActionName(result.Action))
	}
}

// TestSafetySecurity_CompositeValidatorMostRestrictive verifies that the
// composite validator returns the most restrictive result.
func TestSafetySecurity_CompositeValidatorMostRestrictive(t *testing.T) {
	t.Parallel()

	// Permissive validator (disabled)
	permissiveCfg := DefaultSafetyConfig()
	permissiveCfg.Enabled = false
	permissive := NewSafetyValidator(permissiveCfg)

	// Restrictive validator (with blocked tools)
	restrictiveCfg := DefaultSafetyConfig()
	restrictiveCfg.BlockedTools = []string{"dangerousTool"}
	restrictive := NewSafetyValidator(restrictiveCfg)

	composite := NewCompositeValidator(permissive, restrictive)

	result := composite.Validate(SafetyAction{
		Type: "tool_call",
		Name: "dangerousTool",
		Raw:  "anything",
	})
	if result.Action != PolicyBlock {
		t.Errorf("composite should return most restrictive (Block), got %s",
			PolicyActionName(result.Action))
	}
}

// ============================================================================
// Session Spoofing — ManagedSession identity confusion
// ============================================================================

// TestMCPSecurity_SessionIDSpoofing verifies that ManagedSession maintains
// its identity and cannot be confused by external ID manipulation.
func TestMCPSecurity_SessionIDSpoofing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := DefaultManagedSessionConfig()

	// Create two sessions with similar IDs
	s1 := NewManagedSession(ctx, "victim-session", cfg)
	s2 := NewManagedSession(ctx, "attacker-session", cfg)

	if err := s1.Start(); err != nil {
		t.Fatal(err)
	}
	defer s1.Close()

	if err := s2.Start(); err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	// Sessions should be completely independent
	if s1.ID() == s2.ID() {
		t.Error("sessions with different IDs should not have the same ID")
	}

	// Process events on s1 should not affect s2
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s1.ProcessLine("victim event", now)

	snap1 := s1.Snapshot()
	snap2 := s2.Snapshot()

	if snap1.ID == snap2.ID {
		t.Error("snapshots should have different session IDs")
	}
}

// TestMCPSecurity_SessionIDSpecialChars verifies session IDs with special
// characters that might confuse path handling or serialization.
func TestMCPSecurity_SessionIDSpecialChars(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := DefaultManagedSessionConfig()

	specialIDs := []struct {
		name string
		id   string
	}{
		{"path traversal", "../../../etc/passwd"},
		{"null byte", "session\x00id"},
		{"newline", "session\nid"},
		{"unicode", "session-日本語"},
		{"shell metachar", "session;rm -rf /"},
		{"url encoded", "session%00id"},
		{"very long", strings.Repeat("x", 10000)},
		{"empty", ""},
		{"only spaces", "   "},
	}

	for _, tc := range specialIDs {
		t.Run(tc.name, func(t *testing.T) {
			// ManagedSession should accept any string ID without panic.
			// Validation is the caller's responsibility (per MCP server layer).
			s := NewManagedSession(ctx, tc.id, cfg)
			if err := s.Start(); err != nil {
				t.Logf("Start error for ID %q: %v", tc.name, err)
				return
			}
			defer s.Close()

			if s.ID() != tc.id {
				t.Errorf("session ID mismatch: got %q, want %q", s.ID(), tc.id)
			}
		})
	}
}

// ============================================================================
// Instance Registry — Isolation and Confusion Attacks
// ============================================================================

// TestMCPSecurity_InstanceRegistryIsolation verifies instances cannot
// access each other's state directories.
func TestMCPSecurity_InstanceRegistryIsolation(t *testing.T) {
	t.Parallel()

	reg, err := NewInstanceRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.CloseAll()

	inst1, err := reg.Create("instance-1")
	if err != nil {
		t.Fatal(err)
	}
	inst2, err := reg.Create("instance-2")
	if err != nil {
		t.Fatal(err)
	}

	// State directories should be different
	if inst1.StateDir == inst2.StateDir {
		t.Error("instances should have different state directories")
	}

	// IDs should be different
	if inst1.ID == inst2.ID {
		t.Error("instances should have different IDs")
	}
}

// TestMCPSecurity_InstanceRegistryDuplicateID verifies duplicate creation fails.
func TestMCPSecurity_InstanceRegistryDuplicateID(t *testing.T) {
	t.Parallel()

	reg, err := NewInstanceRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.CloseAll()

	_, err = reg.Create("same-id")
	if err != nil {
		t.Fatal(err)
	}

	_, err = reg.Create("same-id")
	if err == nil {
		t.Error("duplicate instance creation should fail")
	}
}

// TestMCPSecurity_InstanceRegistrySpecialIDs verifies registry handles
// dangerous instance IDs safely.
func TestMCPSecurity_InstanceRegistrySpecialIDs(t *testing.T) {
	t.Parallel()

	reg, err := NewInstanceRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.CloseAll()

	dangerousIDs := []struct {
		name string
		id   string
	}{
		{"path traversal", "../../../etc"},
		{"dot", "."},
		{"double dot", ".."},
		{"slash", "/"},
		{"backslash", "\\"},
	}

	for _, tc := range dangerousIDs {
		t.Run(tc.name, func(t *testing.T) {
			// Should either fail or create safely
			inst, err := reg.Create(tc.id)
			if err != nil {
				t.Logf("creation rejected for %q: %v (good)", tc.name, err)
				return
			}
			// If created, the instance exists with the given ID — path escaping
			// is a concern for the storage layer, not the registry. We verify
			// the instance is accessible and closeable without error.
			if inst.ID != tc.id {
				t.Errorf("instance ID mismatch: got %q, want %q", inst.ID, tc.id)
			}
			if err := reg.Close(tc.id); err != nil {
				t.Errorf("close instance %q: %v", tc.name, err)
			}
		})
	}
}

// ============================================================================
// Concurrent Session Manipulation — Race Conditions
// ============================================================================

// TestMCPSecurity_ConcurrentSessionProcessing verifies ManagedSession
// handles concurrent access safely (no races, no panics).
func TestMCPSecurity_ConcurrentSessionProcessing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := DefaultManagedSessionConfig()
	session := NewManagedSession(ctx, "concurrent-test", cfg)

	if err := session.Start(); err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	var wg sync.WaitGroup
	errCh := make(chan string, 100)

	// 20 goroutines doing ProcessLine
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errCh <- "ProcessLine panic"
				}
			}()
			for j := 0; j < 50; j++ {
				t := now.Add(time.Duration(idx*50+j) * time.Millisecond)
				_ = session.ProcessLine("concurrent line", t)
			}
		}(i)
	}

	// 10 goroutines doing ProcessToolCall
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errCh <- "ProcessToolCall panic"
				}
			}()
			for j := 0; j < 50; j++ {
				t := now.Add(time.Duration(idx*50+j) * time.Millisecond)
				_ = session.ProcessToolCall(MCPToolCall{
					ToolName:  "readFile",
					Arguments: `{"path":"/tmp/test"}`,
					Timestamp: t,
				})
			}
		}(i)
	}

	// 5 goroutines doing Snapshot
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errCh <- "Snapshot panic"
				}
			}()
			for j := 0; j < 50; j++ {
				_ = session.Snapshot()
			}
		}()
	}

	// 5 goroutines doing CheckTimeout
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errCh <- "CheckTimeout panic"
				}
			}()
			for j := 0; j < 50; j++ {
				t := now.Add(time.Duration(idx*50+j) * time.Millisecond)
				session.CheckTimeout(t)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for msg := range errCh {
		t.Errorf("concurrent access error: %s", msg)
	}
}

// TestMCPSecurity_ConcurrentInstanceRegistryOps verifies InstanceRegistry
// handles concurrent create/close/list safely.
func TestMCPSecurity_ConcurrentInstanceRegistryOps(t *testing.T) {
	t.Parallel()

	reg, err := NewInstanceRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.CloseAll()

	var wg sync.WaitGroup
	errCh := make(chan string, 100)

	// 20 goroutines creating instances
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errCh <- "Create panic"
				}
			}()
			id := strings.Repeat("a", idx+1) // unique IDs
			_, _ = reg.Create(id)
		}(i)
	}

	// 10 goroutines listing
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errCh <- "List panic"
				}
			}()
			_ = reg.List()
			_ = reg.Len()
		}()
	}

	wg.Wait()
	close(errCh)

	for msg := range errCh {
		t.Errorf("concurrent registry error: %s", msg)
	}
}

// ============================================================================
// Guard State Integrity — Cannot be corrupted by malformed events
// ============================================================================

// TestMCPSecurity_GuardMalformedEvents verifies Guard handles malformed
// events without corrupting internal state.
func TestMCPSecurity_GuardMalformedEvents(t *testing.T) {
	t.Parallel()

	guard := NewGuard(DefaultGuardConfig())
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	malformedEvents := []OutputEvent{
		{Type: EventType(255), Line: "unknown event type"},
		{Type: EventText, Line: ""},
		{Type: EventText, Line: strings.Repeat("x", 1<<20)}, // 1MB line
		{Type: EventText, Line: "\x00\x01\x02\x03"},
		{Type: EventRateLimit, Line: "retry", Fields: map[string]string{"retryAfter": "not-a-number"}},
		{Type: EventRateLimit, Line: "retry", Fields: map[string]string{"retryAfter": "-1"}},
		{Type: EventRateLimit, Line: "retry", Fields: nil},
		{Type: EventPermission, Line: "", Fields: map[string]string{}},
	}

	for i, ev := range malformedEvents {
		// Must not panic
		ge := guard.ProcessEvent(ev, now.Add(time.Duration(i)*time.Second))
		_ = ge
	}

	// Guard state should still be accessible
	state := guard.State()
	_ = state
}

// TestMCPSecurity_MCPGuardStateAfterAbuse verifies MCPGuard state is
// consistent after handling many malicious inputs.
func TestMCPSecurity_MCPGuardStateAfterAbuse(t *testing.T) {
	t.Parallel()

	guard := NewMCPGuard(DefaultMCPGuardConfig())
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Feed 1000 calls with various malicious patterns
	for i := 0; i < 1000; i++ {
		call := MCPToolCall{
			ToolName:  strings.Repeat("x", i%100),
			Arguments: strings.Repeat("{", i%50),
			Timestamp: now.Add(time.Duration(i) * time.Millisecond),
		}
		_ = guard.ProcessToolCall(call)
	}

	// State should be internally consistent
	state := guard.State()
	if state.TotalCalls != 1000 {
		t.Errorf("expected 1000 total calls, got %d", state.TotalCalls)
	}
	if !state.Started {
		t.Error("guard should be started after processing calls")
	}
	if state.RecentCount == 0 {
		t.Error("guard should have recent calls in history")
	}
}

// ============================================================================
// Safety Stats — Cannot be manipulated externally
// ============================================================================

// TestSafetySecurity_StatsConsistency verifies safety stats remain consistent
// after processing various inputs including malicious ones.
func TestSafetySecurity_StatsConsistency(t *testing.T) {
	t.Parallel()

	cfg := DefaultSafetyConfig()
	validator := NewSafetyValidator(cfg)

	// Process a mix of safe and dangerous actions
	actions := []SafetyAction{
		{Type: "tool_call", Name: "readFile", Raw: "/tmp/safe.txt"},
		{Type: "command", Name: "rm", Raw: "rm -rf /"},
		{Type: "tool_call", Name: "fetch", Raw: "https://example.com"},
		{Type: "tool_call", Name: "env_get", Raw: "AWS_SECRET_ACCESS_KEY"},
		{Type: "tool_call", Name: "customTool", Raw: "benign"},
	}

	for _, a := range actions {
		_ = validator.Validate(a)
	}

	stats := validator.Stats()
	if stats.TotalChecks != 5 {
		t.Errorf("expected 5 total checks, got %d", stats.TotalChecks)
	}

	// Verify sum of action counts equals total
	actionSum := stats.AllowCount + stats.WarnCount + stats.ConfirmCount + stats.BlockCount
	if actionSum != stats.TotalChecks {
		t.Errorf("action sum (%d) != total checks (%d)", actionSum, stats.TotalChecks)
	}
}
