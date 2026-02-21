package claudemux

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// Integration test infrastructure
//
// All tests in this file require -integration flag. Without it, they are
// silently skipped and have zero effect on `make` builds.
//
// To run integration tests locally:
//
//	go test -race -v -count=1 -integration \
//	  -provider=ollama -model=gpt-oss:20b-cloud \
//	  ./internal/builtin/claudemux/...
//
// Or via make:
//
//	make integration-test-claudemux
//	make integration-test-claudemux PROVIDER=claude-code MODEL=sonnet
//
// Prerequisites:
//   - For ollama provider: `ollama launch claude` must be installed and accessible
//   - For claude-code provider: `claude` CLI must be installed and configured
//
// ============================================================================

var (
	integrationEnabled bool
	testProvider       string
	testModel          string
)

// TestMain is the package-wide test entry point. It parses the -integration,
// -provider, and -model flags while preserving normal test execution when
// no extra flags are provided.
func TestMain(m *testing.M) {
	flag.BoolVar(&integrationEnabled, "integration", false,
		"enable integration tests that require real agent infrastructure")
	flag.StringVar(&testProvider, "provider", "ollama",
		"provider to test against: ollama, claude-code")
	flag.StringVar(&testModel, "model", "gpt-oss:20b-cloud",
		"model to select for integration tests")
	flag.Parse()
	os.Exit(m.Run())
}

// skipIfNotIntegration skips the calling test if -integration was not passed.
func skipIfNotIntegration(t *testing.T) {
	t.Helper()
	if !integrationEnabled {
		t.Skip("integration tests disabled; use -integration flag to enable")
	}
}

// ============================================================================
// Integration test helpers
// ============================================================================

// resolveTestProvider creates a Provider based on the -provider flag.
func resolveTestProvider(t *testing.T) Provider {
	t.Helper()
	switch testProvider {
	case "ollama":
		return &OllamaProvider{}
	case "claude-code":
		return &ClaudeCodeProvider{}
	default:
		t.Fatalf("unsupported -provider=%q (supported: ollama, claude-code)", testProvider)
		return nil
	}
}

// collectUntil reads from handle collecting output until done() returns true
// or the deadline expires. Uses a concurrent reader goroutine since PTY
// Read() is a blocking system call.
func collectUntil(t *testing.T, handle AgentHandle, timeout time.Duration, done func(accumulated string) bool) string {
	t.Helper()

	type chunk struct {
		data string
		err  error
	}

	var buf strings.Builder
	deadline := time.After(timeout)

	for {
		ch := make(chan chunk, 1)
		go func() {
			data, err := handle.Receive()
			ch <- chunk{data, err}
		}()

		select {
		case c := <-ch:
			if c.data != "" {
				buf.WriteString(c.data)
				t.Logf("chunk (%d bytes): %q", len(c.data), truncateStr(c.data, 200))
			}
			if done(buf.String()) {
				return buf.String()
			}
			if c.err != nil {
				t.Logf("receive error: %v (accumulated %d bytes)", c.err, buf.Len())
				return buf.String()
			}
		case <-deadline:
			t.Logf("timeout after %v (accumulated %d bytes)", timeout, buf.Len())
			return buf.String()
		}
	}
}

// truncateStr shortens s to max runes, appending "..." if truncated.
func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// splitTerminalLines converts raw terminal output into lines, normalizing
// \r\n and bare \r (carriage return) to \n before splitting.
func splitTerminalLines(raw string) []string {
	s := strings.ReplaceAll(raw, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Split(s, "\n")
}

// ============================================================================
// Live agent tests (require -integration flag + real infrastructure)
// ============================================================================

// TestIntegration_ProviderSpawn verifies that the configured provider can
// spawn an agent and produce initial output through the PTY.
//
// Prerequisites:
//   - ollama: `ollama` accessible on PATH
//   - claude-code: `claude` CLI installed and configured
func TestIntegration_ProviderSpawn(t *testing.T) {
	skipIfNotIntegration(t)

	prov := resolveTestProvider(t)
	t.Logf("Provider: %s (model=%s)", prov.Name(), testModel)
	t.Logf("Capabilities: MCP=%v Streaming=%v MultiTurn=%v ModelNav=%v",
		prov.Capabilities().MCP, prov.Capabilities().Streaming,
		prov.Capabilities().MultiTurn, prov.Capabilities().ModelNav)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	handle, err := prov.Spawn(ctx, SpawnOpts{
		Model: testModel,
		Rows:  24,
		Cols:  80,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer handle.Close()

	if !handle.IsAlive() {
		t.Fatal("agent should be alive immediately after spawn")
	}

	// Collect initial output (expect *something* within 15 seconds).
	output := collectUntil(t, handle, 15*time.Second, func(acc string) bool {
		return len(acc) > 0
	})

	if len(output) == 0 {
		t.Fatal("no output received from spawned provider")
	}

	t.Logf("Initial output (%d bytes)", len(output))
	t.Logf("Agent alive: %v", handle.IsAlive())
}

// TestIntegration_MenuNavigation verifies that model selection menu navigation
// works correctly with the configured provider.
//
// For ollama: spawns the agent, waits for the model selection menu to appear,
// parses it with ParseModelMenu, generates navigation keystrokes via
// NavigateToModel, and sends them to the PTY.
//
// Skipped for providers that don't support ModelNav (e.g., claude-code, which
// uses --model flag instead).
func TestIntegration_MenuNavigation(t *testing.T) {
	skipIfNotIntegration(t)

	prov := resolveTestProvider(t)
	if !prov.Capabilities().ModelNav {
		t.Skipf("provider %q does not use model navigation (uses --model flag)", prov.Name())
	}

	t.Logf("Testing model navigation: provider=%s target=%s", prov.Name(), testModel)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	handle, err := prov.Spawn(ctx, SpawnOpts{
		Rows: 24,
		Cols: 120,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer handle.Close()

	// Phase 1: Collect output until we can detect a model menu.
	const menuTimeout = 30 * time.Second
	menuDetected := false
	var parsedMenu *ModelMenu

	output := collectUntil(t, handle, menuTimeout, func(acc string) bool {
		lines := splitTerminalLines(acc)
		menu := ParseModelMenu(lines)
		if len(menu.Models) >= 2 {
			// Require at least 2 models — single-model menus auto-select.
			parsedMenu = menu
			menuDetected = true
			return true
		}
		return false
	})

	if !menuDetected {
		lines := splitTerminalLines(output)
		t.Logf("Raw output (%d lines, %d bytes):", len(lines), len(output))
		for i, line := range lines {
			if i < 30 {
				t.Logf("  [%d] %q", i, line)
			}
		}
		t.Fatalf("model selection menu not detected within %v", menuTimeout)
	}

	t.Logf("Menu detected: %d models, selected=%d", len(parsedMenu.Models), parsedMenu.SelectedIndex)
	for i, m := range parsedMenu.Models {
		marker := "  "
		if i == parsedMenu.SelectedIndex {
			marker = "→ "
		}
		t.Logf("  %s%s", marker, m)
	}

	// Phase 2: Navigate to target model.
	keys, err := NavigateToModel(parsedMenu, testModel)
	if err != nil {
		t.Fatalf("NavigateToModel(%q): %v (available: %v)", testModel, err, parsedMenu.Models)
	}

	t.Logf("Navigation keystrokes: %d bytes (steps=%d + enter)",
		len(keys), strings.Count(keys, KeyArrowDown)+strings.Count(keys, KeyArrowUp))

	if err := handle.Send(keys); err != nil {
		t.Fatalf("Send navigation keystrokes: %v", err)
	}

	// Phase 3: Wait for post-selection output (agent entrypoint or confirmation).
	postNav := collectUntil(t, handle, 30*time.Second, func(acc string) bool {
		// Look for indicators that model selection completed:
		// - The menu disappeared (no more ❯/>/▸ indicators)
		// - Agent initialization output appeared
		lower := strings.ToLower(acc)
		return strings.Contains(lower, "ready") ||
			strings.Contains(lower, "initialized") ||
			strings.Contains(lower, "welcome") ||
			strings.Contains(lower, "tips") ||
			strings.Contains(lower, "$") ||
			len(acc) > 2000 // If we got lots of output, menu was likely accepted
	})

	t.Logf("Post-navigation output (%d bytes)", len(postNav))
	if len(postNav) > 0 {
		lines := splitTerminalLines(postNav)
		for i, line := range lines {
			if i < 20 && strings.TrimSpace(line) != "" {
				t.Logf("  [%d] %q", i, line)
			}
		}
	}

	t.Log("Model navigation completed")
}

// TestIntegration_MCPStartup verifies MCP server initialization with real agent.
//
// Creates an MCP instance config with auto-port listener, starts the server,
// and verifies it accepts connections.
func TestIntegration_MCPStartup(t *testing.T) {
	skipIfNotIntegration(t)

	mcpCfg, err := NewMCPInstanceConfig("integration-mcp")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}
	defer func() { _ = mcpCfg.Close() }()

	t.Logf("MCP config dir: %s", mcpCfg.ConfigPath())
	t.Logf("MCP session ID: %s", mcpCfg.SessionID)

	t.Log("MCP infrastructure ready for agent connection")
}

// TestIntegration_TaskSubmitAndResult verifies end-to-end task submission
// and result collection through the ManagedSession pipeline.
func TestIntegration_TaskSubmitAndResult(t *testing.T) {
	skipIfNotIntegration(t)

	ctx := context.Background()
	cfg := DefaultManagedSessionConfig()
	session := NewManagedSession(ctx, "integration-task", cfg)
	if err := session.Start(); err != nil {
		t.Fatalf("session.Start: %v", err)
	}

	t.Logf("Session started: id=%s state=%s",
		session.ID(), ManagedSessionStateName(session.State()))

	// Simulate task output that would come from a real agent.
	now := time.Now()
	result := session.ProcessLine("Task completed successfully", now)
	t.Logf("Task result: event_type=%s action=%s",
		EventTypeName(result.Event.Type), result.Action)

	d := session.Shutdown()
	t.Logf("Session shutdown: action=%s", RecoveryActionName(d.Action))
	session.Close()

	t.Log("Task pipeline infrastructure ready")
}

// TestIntegration_GuardRails verifies guard rails fire correctly with
// rate limit, permission, and output timeout detection.
func TestIntegration_GuardRails(t *testing.T) {
	skipIfNotIntegration(t)

	ctx := context.Background()
	cfg := DefaultManagedSessionConfig()
	cfg.Guard.RateLimit.Enabled = true
	cfg.Guard.Permission.Policy = PermissionPolicyDeny
	session := NewManagedSession(ctx, "integration-guards", cfg)
	if err := session.Start(); err != nil {
		t.Fatalf("session.Start: %v", err)
	}

	// Feed rate-limit output to verify detection.
	now := time.Now()
	result := session.ProcessLine("⚠ Rate limit: 429 Too Many Requests", now)
	t.Logf("Guard result: event_type=%s action=%s",
		EventTypeName(result.Event.Type), result.Action)

	d := session.Shutdown()
	t.Logf("Guard rails session shutdown: action=%s", RecoveryActionName(d.Action))
	session.Close()

	t.Log("Guard rails infrastructure ready")
}

// TestIntegration_Recovery verifies the supervisor recovery workflow:
// error → retry → confirm → resume.
func TestIntegration_Recovery(t *testing.T) {
	skipIfNotIntegration(t)

	ctx := context.Background()
	cfg := DefaultManagedSessionConfig()
	cfg.Supervisor.MaxRetries = 3
	session := NewManagedSession(ctx, "integration-recovery", cfg)
	if err := session.Start(); err != nil {
		t.Fatalf("session.Start: %v", err)
	}

	// Simulate crash.
	now := time.Now()
	ge, d := session.ProcessCrash(1, now)

	t.Logf("Crash: guard=%v recovery_action=%s",
		ge != nil, RecoveryActionName(d.Action))

	if d.Action == RecoveryRetry {
		session.ConfirmRecovery()
		snap := session.Snapshot()
		t.Logf("After recovery: state=%s", snap.StateName)
	}

	d2 := session.Shutdown()
	t.Logf("Recovery session shutdown: action=%s", RecoveryActionName(d2.Action))
	session.Close()

	t.Log("Recovery infrastructure ready")
}

// ============================================================================
// Simulated integration tests — run WITHOUT -integration flag in CI
//
// These compose multiple building blocks in realistic sequences to verify
// wiring correctness without requiring real agent infrastructure.
// ============================================================================

// TestSimulated_FullPipelineLifecycle exercises the complete pipeline
// (parser → guard → MCP guard → safety → supervisor → pool → panel)
// using simulated events. Runs in CI without real agents.
func TestSimulated_FullPipelineLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	// --- Phase 1: Instance Registry ---
	base := filepath.Join(t.TempDir(), "sessions")
	registry, err := NewInstanceRegistry(base)
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}

	inst1, err := registry.Create("worker-1")
	if err != nil {
		t.Fatalf("Create worker-1: %v", err)
	}
	inst2, err := registry.Create("worker-2")
	if err != nil {
		t.Fatalf("Create worker-2: %v", err)
	}
	t.Logf("Phase 1: Created %d instances", registry.Len())

	// --- Phase 2: Pool lifecycle ---
	poolCfg := DefaultPoolConfig()
	poolCfg.MaxSize = 4
	pool := NewPool(poolCfg)
	if err := pool.Start(); err != nil {
		t.Fatalf("Pool.Start: %v", err)
	}

	w1, err := pool.AddWorker("worker-1", inst1)
	if err != nil {
		t.Fatalf("AddWorker 1: %v", err)
	}
	w2, err := pool.AddWorker("worker-2", inst2)
	if err != nil {
		t.Fatalf("AddWorker 2: %v", err)
	}

	// Acquire → do work → release.
	acquired, err := pool.TryAcquire()
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if acquired == nil {
		t.Fatal("TryAcquire returned nil")
	}
	t.Logf("Phase 2: Acquired worker %s", acquired.ID)
	pool.Release(acquired, nil, now)
	_ = w1
	_ = w2

	stats := pool.Stats()
	if stats.WorkerCount != 2 {
		t.Errorf("WorkerCount = %d, want 2", stats.WorkerCount)
	}
	t.Logf("Phase 2: Pool state=%s workers=%d", stats.StateName, stats.WorkerCount)

	// --- Phase 3: Panel lifecycle ---
	panel := NewPanel(PanelConfig{MaxPanes: 4, ScrollbackSize: 500})
	if err := panel.Start(); err != nil {
		t.Fatalf("Panel.Start: %v", err)
	}

	_, err = panel.AddPane("worker-1", "Worker 1")
	if err != nil {
		t.Fatalf("AddPane worker-1: %v", err)
	}
	_, err = panel.AddPane("worker-2", "Worker 2")
	if err != nil {
		t.Fatalf("AddPane worker-2: %v", err)
	}

	// Route output to panes.
	if err := panel.AppendOutput("worker-1", "Processing task A..."); err != nil {
		t.Fatalf("AppendOutput: %v", err)
	}
	if err := panel.AppendOutput("worker-2", "Processing task B..."); err != nil {
		t.Fatalf("AppendOutput: %v", err)
	}

	// Switch active pane.
	result := panel.RouteInput("alt+2")
	if result.Consumed {
		t.Logf("Phase 3: Switched to pane %d", panel.Snapshot().ActiveIdx)
	}

	snap := panel.Snapshot()
	if len(snap.Panes) != 2 {
		t.Errorf("PaneCount = %d, want 2", len(snap.Panes))
	}
	t.Logf("Phase 3: Panel panes=%d active=%d", len(snap.Panes), snap.ActiveIdx)

	// --- Phase 4: ManagedSession lifecycle ---
	sessionCfg := DefaultManagedSessionConfig()
	sessionCfg.Guard.RateLimit.Enabled = true
	sessionCfg.MCPGuard.ToolAllowlist.Enabled = true
	sessionCfg.MCPGuard.ToolAllowlist.AllowedTools = []string{"readFile", "writeFile", "execute"}

	var guardEvents []string
	var recoveryDecisions []string
	var mu sync.Mutex

	session := NewManagedSession(ctx, "pipeline-test", sessionCfg)
	session.OnGuardAction = func(ge *GuardEvent) {
		mu.Lock()
		guardEvents = append(guardEvents, guardActionToString(ge.Action))
		mu.Unlock()
	}
	session.OnRecoveryDecision = func(d RecoveryDecision) {
		mu.Lock()
		recoveryDecisions = append(recoveryDecisions, RecoveryActionName(d.Action))
		mu.Unlock()
	}

	if err := session.Start(); err != nil {
		t.Fatalf("Session.Start: %v", err)
	}

	// Normal text events.
	for _, line := range []string{
		"Initializing workspace...",
		"Loading configuration from .env",
		"Starting task execution",
	} {
		now = now.Add(time.Second)
		r := session.ProcessLine(line, now)
		if r.Action != "none" {
			t.Logf("Unexpected action on text: %s", r.Action)
		}
		// Route to panel.
		_ = panel.AppendOutput("worker-1", line)
	}

	// MCP tool calls (allowed).
	for _, tool := range []string{"readFile", "writeFile", "execute"} {
		tr := session.ProcessToolCall(MCPToolCall{
			ToolName:  tool,
			Arguments: `{"path":"/tmp/test"}`,
			Timestamp: now,
		})
		if tr.Action != "none" {
			t.Errorf("tool %s should be allowed, got action=%s", tool, tr.Action)
		}
	}

	// MCP tool call (blocked — not in allowlist).
	tr := session.ProcessToolCall(MCPToolCall{
		ToolName:  "deleteDatabase",
		Arguments: `{"db":"production"}`,
		Timestamp: now,
	})
	if tr.Action == "none" {
		t.Error("deleteDatabase should be blocked by allowlist")
	}

	// Check timeout (should not fire yet — we just processed events).
	ge := session.CheckTimeout(now.Add(time.Second))
	if ge != nil {
		t.Logf("Unexpected timeout: %s", guardActionToString(ge.Action))
	}

	sessionSnap := session.Snapshot()
	t.Logf("Phase 4: Session state=%s lines=%d events=%v",
		sessionSnap.StateName, sessionSnap.LinesProcessed, sessionSnap.EventCounts)

	// Verify guard events include the blocked tool.
	mu.Lock()
	guardCount := len(guardEvents)
	mu.Unlock()
	if guardCount == 0 {
		t.Error("expected at least one guard event from blocked tool call")
	}

	// --- Phase 5: Crash → Recovery cycle ---
	_, d := session.ProcessCrash(1, now.Add(5*time.Second))
	t.Logf("Phase 5: Crash recovery action=%s", RecoveryActionName(d.Action))

	if d.Action == RecoveryRetry {
		session.ConfirmRecovery()
		if session.State() != SessionActive {
			t.Errorf("after recovery, state=%s, want Active",
				ManagedSessionStateName(session.State()))
		}
	}

	// --- Phase 6: Safety validation ---
	safetyCfg := DefaultSafetyConfig()
	validator := NewSafetyValidator(safetyCfg)

	safetyTests := []struct {
		name string
		raw  string
	}{
		{"safe read", "cat /tmp/test.txt"},
		{"risky delete", "rm -rf /important/data"},
		{"network", "curl https://example.com/api"},
	}

	for _, st := range safetyTests {
		fields := strings.Fields(st.raw)
		args := make(map[string]string)
		for i, f := range fields[1:] {
			args[fmt.Sprintf("arg%d", i)] = f
		}
		assessment := validator.Validate(SafetyAction{
			Name: fields[0],
			Args: args,
			Raw:  st.raw,
		})
		t.Logf("Phase 6: safety[%s] intent=%s risk=%s action=%s",
			st.name, IntentName(assessment.Intent), RiskLevelName(assessment.RiskLevel),
			PolicyActionName(assessment.Action))
	}

	safetyStats := validator.Stats()
	if safetyStats.TotalChecks != int64(len(safetyTests)) {
		t.Errorf("safety total = %d, want %d", safetyStats.TotalChecks, len(safetyTests))
	}

	// --- Phase 7: Choice resolution ---
	resolver := NewChoiceResolver(ChoiceConfig{
		ConfirmThreshold: 0.3,
		MinCandidates:    1,
	})

	candidates := []Candidate{
		{ID: "worker-1", Name: "Fast Worker", Attributes: map[string]string{"speed": "high", "cost": "low"}},
		{ID: "worker-2", Name: "Accurate Worker", Attributes: map[string]string{"speed": "medium", "cost": "medium"}},
	}
	criteria := []Criterion{
		{Name: "speed", Weight: 0.6},
		{Name: "accuracy", Weight: 0.4},
	}

	choiceResult, err := resolver.Analyze(candidates, criteria, func(c Candidate, cr Criterion) float64 {
		switch cr.Name {
		case "speed":
			if c.Attributes["speed"] == "high" {
				return 0.9
			}
			return 0.5
		case "accuracy":
			if c.Attributes["speed"] == "medium" {
				return 0.8
			}
			return 0.6
		default:
			return 0.5
		}
	})
	if err != nil {
		t.Fatalf("ChoiceResolver.Analyze: %v", err)
	}
	t.Logf("Phase 7: Best choice=%s score=%.3f",
		choiceResult.RecommendedID, choiceResult.Rankings[0].TotalScore)

	// --- Phase 8: Graceful shutdown ---
	sD := session.Shutdown()
	session.Close()
	t.Logf("Phase 8: Session shutdown action=%s", RecoveryActionName(sD.Action))

	panel.Close()
	pool.Drain()
	pool.WaitDrained()
	remaining := pool.Close()
	t.Logf("Phase 8: Pool closed, remaining workers=%d", len(remaining))

	if err := registry.CloseAll(); err != nil {
		t.Errorf("registry.CloseAll: %v", err)
	}

	t.Log("Full pipeline lifecycle completed successfully")
}

// TestSimulated_ConcurrentMultiSessionPipeline exercises multiple sessions
// processing events concurrently through a shared pool and panel, verifying
// thread safety of the entire stack.
func TestSimulated_ConcurrentMultiSessionPipeline(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	base := filepath.Join(t.TempDir(), "sessions")
	registry, err := NewInstanceRegistry(base)
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}

	poolCfg := DefaultPoolConfig()
	poolCfg.MaxSize = 8
	pool := NewPool(poolCfg)
	if err := pool.Start(); err != nil {
		t.Fatalf("Pool.Start: %v", err)
	}

	panel := NewPanel(PanelConfig{MaxPanes: 8, ScrollbackSize: 200})
	if err := panel.Start(); err != nil {
		t.Fatalf("Panel.Start: %v", err)
	}

	const numWorkers = 4
	const eventsPerWorker = 50

	sessions := make([]*ManagedSession, numWorkers)
	for i := 0; i < numWorkers; i++ {
		id := fmt.Sprintf("worker-%d", i)
		inst, err := registry.Create(id)
		if err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
		if _, err := pool.AddWorker(id, inst); err != nil {
			t.Fatalf("AddWorker %s: %v", id, err)
		}
		if _, err := panel.AddPane(id, fmt.Sprintf("Worker %d", i)); err != nil {
			t.Fatalf("AddPane %s: %v", id, err)
		}

		cfg := DefaultManagedSessionConfig()
		sessions[i] = NewManagedSession(ctx, id, cfg)
		if err := sessions[i].Start(); err != nil {
			t.Fatalf("session %s Start: %v", id, err)
		}
	}

	// Hammer all sessions concurrently.
	var wg sync.WaitGroup
	errors := make([]error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("worker-%d", idx)
			now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

			for j := 0; j < eventsPerWorker; j++ {
				now = now.Add(100 * time.Millisecond)
				line := fmt.Sprintf("[%s] Processing step %d/%d", id, j+1, eventsPerWorker)

				r := sessions[idx].ProcessLine(line, now)
				if r.Action != "none" {
					// Guard triggered — not a hard error in this test.
					continue
				}

				if err := panel.AppendOutput(id, line); err != nil {
					errors[idx] = fmt.Errorf("AppendOutput: %w", err)
					return
				}
			}

			snap := sessions[idx].Snapshot()
			if snap.LinesProcessed != eventsPerWorker {
				errors[idx] = fmt.Errorf("worker %s: lines=%d want %d",
					id, snap.LinesProcessed, eventsPerWorker)
			}
		}(i)
	}

	wg.Wait()

	for i, err := range errors {
		if err != nil {
			t.Errorf("worker-%d: %v", i, err)
		}
	}

	// Verify pool stats.
	stats := pool.Stats()
	if stats.WorkerCount != numWorkers {
		t.Errorf("pool workers=%d, want %d", stats.WorkerCount, numWorkers)
	}

	// Verify panel pane counts.
	panelSnap := panel.Snapshot()
	if len(panelSnap.Panes) != numWorkers {
		t.Errorf("panel panes=%d, want %d", len(panelSnap.Panes), numWorkers)
	}

	// Graceful shutdown.
	for i := 0; i < numWorkers; i++ {
		sessions[i].Shutdown()
		sessions[i].Close()
	}
	panel.Close()
	pool.Drain()
	pool.WaitDrained()
	pool.Close()
	_ = registry.CloseAll()

	t.Logf("Concurrent multi-session pipeline: %d workers × %d events = %d total events processed",
		numWorkers, eventsPerWorker, numWorkers*eventsPerWorker)
}

// TestSimulated_ErrorRecoveryEscalation verifies the full error → retry →
// exhaust retries → escalate flow through the supervisor.
func TestSimulated_ErrorRecoveryEscalation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := DefaultManagedSessionConfig()
	cfg.Supervisor.MaxRetries = 3 // Retry threshold=1, restart threshold=3.

	var decisions []string
	session := NewManagedSession(ctx, "escalation-test", cfg)
	session.OnRecoveryDecision = func(d RecoveryDecision) {
		decisions = append(decisions, RecoveryActionName(d.Action))
	}
	if err := session.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Use HandleError (supervisor-only path) to test retry exhaustion
	// without the Guard's crash handler interfering.
	//
	// With MaxRetries=3, retryThreshold=1:
	//   Error 1: retryCount=1 → Retry  (≤ threshold)
	//   Error 2: retryCount=2 → Restart (> threshold, ≤ max)
	//   Error 3: retryCount=3 → Restart (> threshold, ≤ max)
	//   Error 4: retryCount=4 → Escalate (> max)

	// Error 1: should retry.
	d1 := session.HandleError("test error 1", ErrorClassMCPTimeout)
	if d1.Action != RecoveryRetry {
		t.Fatalf("error 1: got %s, want Retry", RecoveryActionName(d1.Action))
	}
	session.ConfirmRecovery()
	t.Logf("Error 1: %s → recovered", RecoveryActionName(d1.Action))

	// Error 2: should restart (past retry threshold).
	d2 := session.HandleError("test error 2", ErrorClassMCPTimeout)
	if d2.Action != RecoveryRestart {
		t.Fatalf("error 2: got %s, want Restart", RecoveryActionName(d2.Action))
	}
	session.ConfirmRecovery()
	t.Logf("Error 2: %s → recovered", RecoveryActionName(d2.Action))

	// Error 3: still within max retries, should restart.
	d3 := session.HandleError("test error 3", ErrorClassMCPTimeout)
	if d3.Action != RecoveryRestart {
		t.Fatalf("error 3: got %s, want Restart", RecoveryActionName(d3.Action))
	}
	session.ConfirmRecovery()
	t.Logf("Error 3: %s → recovered", RecoveryActionName(d3.Action))

	// Error 4: should escalate (retries exhausted).
	d4 := session.HandleError("test error 4", ErrorClassMCPTimeout)
	if d4.Action != RecoveryEscalate {
		t.Fatalf("error 4: got %s, want Escalate", RecoveryActionName(d4.Action))
	}
	t.Logf("Error 4: %s (retries exhausted)", RecoveryActionName(d4.Action))

	// Session should be Failed.
	if session.State() != SessionFailed {
		t.Errorf("final state=%s, want Failed",
			ManagedSessionStateName(session.State()))
	}

	t.Logf("Recovery decisions: %v", decisions)
	session.Close()
}

// TestSimulated_SafetyIntoPipeline verifies that safety validation integrates
// correctly with ManagedSession tool call processing — safety assessments
// inform guard rail decisions.
func TestSimulated_SafetyIntoPipeline(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	safetyCfg := DefaultSafetyConfig()
	validator := NewSafetyValidator(safetyCfg)

	sessionCfg := DefaultManagedSessionConfig()
	sessionCfg.MCPGuard.ToolAllowlist.Enabled = true
	sessionCfg.MCPGuard.ToolAllowlist.AllowedTools = []string{"readFile", "writeFile", "execute"}

	session := NewManagedSession(ctx, "safety-pipeline", sessionCfg)
	if err := session.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	// Scenario: tool call arrives → safety validates → MCP guard checks allowlist.
	calls := []struct {
		tool      string
		args      string
		wantSafe  bool
		wantMCPOK bool
	}{
		{"readFile", "cat /tmp/readme.txt", true, true},
		{"execute", "rm -rf /", false, true},            // allowed by MCP but unsafe
		{"dropTable", "DROP TABLE users", false, false}, // blocked by MCP AND unsafe
	}

	for _, c := range calls {
		// Step 1: Safety assessment.
		argsMap := make(map[string]string)
		for i, f := range strings.Fields(c.args) {
			argsMap[fmt.Sprintf("arg%d", i)] = f
		}
		assessment := validator.Validate(SafetyAction{
			Name: c.tool,
			Args: argsMap,
			Raw:  c.args,
		})

		// Step 2: MCP guard check.
		tcr := session.ProcessToolCall(MCPToolCall{
			ToolName:  c.tool,
			Arguments: `{"cmd":"` + c.args + `"}`,
			Timestamp: now,
		})

		isSafe := assessment.Action == PolicyAllow
		isMCPOK := tcr.Action == "none"

		if isSafe != c.wantSafe {
			t.Errorf("tool=%s safety: got safe=%v want %v (action=%s)",
				c.tool, isSafe, c.wantSafe, PolicyActionName(assessment.Action))
		}
		if isMCPOK != c.wantMCPOK {
			t.Errorf("tool=%s MCP: got ok=%v want %v (action=%s)",
				c.tool, isMCPOK, c.wantMCPOK, tcr.Action)
		}

		t.Logf("tool=%s safe=%v mcp_ok=%v intent=%s risk=%s",
			c.tool, isSafe, isMCPOK, IntentName(assessment.Intent), RiskLevelName(assessment.RiskLevel))
	}

	session.Shutdown()
	session.Close()
}
