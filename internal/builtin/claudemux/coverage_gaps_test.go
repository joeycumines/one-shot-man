package claudemux

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"path/filepath"
	"testing"
	"time"
)

// ============================================================================
// Pool — TryAcquire state machine coverage
// ============================================================================

func TestPool_TryAcquire_Closed(t *testing.T) {
	t.Parallel()
	p := NewPool(DefaultPoolConfig())
	_ = p.Start()
	_, _ = p.AddWorker("w1", nil)
	p.Close()

	_, err := p.TryAcquire()
	if !errors.Is(err, ErrPoolClosed) {
		t.Errorf("TryAcquire after Close: got %v, want ErrPoolClosed", err)
	}
}

func TestPool_TryAcquire_Draining(t *testing.T) {
	t.Parallel()
	p := NewPool(DefaultPoolConfig())
	_ = p.Start()
	_, _ = p.AddWorker("w1", nil)
	p.Drain()

	_, err := p.TryAcquire()
	if !errors.Is(err, ErrPoolDraining) {
		t.Errorf("TryAcquire while draining: got %v, want ErrPoolDraining", err)
	}
	p.Close()
}

func TestPool_TryAcquire_NotRunning(t *testing.T) {
	t.Parallel()
	p := NewPool(DefaultPoolConfig())
	// Pool is in Idle state — never started.
	_, err := p.TryAcquire()
	if !errors.Is(err, ErrPoolNotRunning) {
		t.Errorf("TryAcquire on Idle pool: got %v, want ErrPoolNotRunning", err)
	}
}

func TestPool_TryAcquire_Empty(t *testing.T) {
	t.Parallel()
	p := NewPool(DefaultPoolConfig())
	_ = p.Start()
	// Running but no workers added.
	_, err := p.TryAcquire()
	if !errors.Is(err, ErrPoolEmpty) {
		t.Errorf("TryAcquire on empty pool: got %v, want ErrPoolEmpty", err)
	}
	p.Close()
}

func TestPool_TryAcquire_AllBusy_Coverage(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 1})
	_ = p.Start()
	_, _ = p.AddWorker("w1", nil)

	// Acquire the only worker.
	w, err := p.TryAcquire()
	if err != nil {
		t.Fatalf("first TryAcquire: %v", err)
	}
	if w == nil {
		t.Fatal("first TryAcquire returned nil worker")
	}

	// Second attempt — all busy.
	_, err = p.TryAcquire()
	if !errors.Is(err, ErrPoolEmpty) {
		t.Errorf("TryAcquire all busy: got %v, want ErrPoolEmpty", err)
	}
	p.Close()
}

func TestPool_FindWorker_NotFound(t *testing.T) {
	t.Parallel()
	p := NewPool(DefaultPoolConfig())
	_ = p.Start()
	_, _ = p.AddWorker("w1", nil)

	if w := p.FindWorker("nonexistent"); w != nil {
		t.Errorf("FindWorker(nonexistent) = %v, want nil", w)
	}
	p.Close()
}

// ============================================================================
// Control Server — input validation and error handling
// ============================================================================

// mockControlHandler implements ControlHandler for testing.
type mockControlHandler struct {
	enqueueFn  func(string) (int, error)
	interruptFn func() error
	statusFn   func() GetStatusResult
}

func (m *mockControlHandler) EnqueueTask(task string) (int, error) {
	if m.enqueueFn != nil {
		return m.enqueueFn(task)
	}
	return 0, nil
}

func (m *mockControlHandler) InterruptCurrent() error {
	if m.interruptFn != nil {
		return m.interruptFn()
	}
	return nil
}

func (m *mockControlHandler) GetStatus() GetStatusResult {
	if m.statusFn != nil {
		return m.statusFn()
	}
	return GetStatusResult{}
}

func TestControlServer_InvalidJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	srv := NewControlServer(sockPath, &mockControlHandler{})
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = srv.Close() }()

	// Connect and send garbage.
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = conn.Write([]byte("not json at all\n"))

	// Read response — should be an error response.
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	var resp ControlResponse
	if err := json.Unmarshal(buf[:n], &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.OK {
		t.Error("expected OK=false for invalid JSON")
	}
	if resp.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestControlServer_EmptyTask(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	srv := NewControlServer(sockPath, &mockControlHandler{})
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = srv.Close() }()

	// Send EnqueueTask with empty task.
	client := NewControlClient(sockPath)
	_, err := client.EnqueueTask("")
	if err == nil {
		t.Error("expected error for empty task")
	}
}

func TestControlServer_HandlerError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	handler := &mockControlHandler{
		enqueueFn: func(s string) (int, error) {
			return 0, fmt.Errorf("queue is full")
		},
	}
	srv := NewControlServer(sockPath, handler)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = srv.Close() }()

	client := NewControlClient(sockPath)
	_, err := client.EnqueueTask("some task")
	if err == nil {
		t.Error("expected error from handler")
	}
}

func TestControlServer_InvalidParams(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	srv := NewControlServer(sockPath, &mockControlHandler{})
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = srv.Close() }()

	// Send a valid command but with bad params JSON.
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	req := `{"command":"EnqueueTask","params":"not-an-object"}` + "\n"
	_, _ = conn.Write([]byte(req))

	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	var resp ControlResponse
	if err := json.Unmarshal(buf[:n], &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.OK {
		t.Error("expected OK=false for invalid params")
	}
}

// ============================================================================
// ManagedSession — lifecycle state machine
// ============================================================================

func TestManagedSession_StartFromNonIdle_Coverage(t *testing.T) {
	t.Parallel()
	cfg := DefaultManagedSessionConfig()
	s := NewManagedSession(context.Background(), "test", cfg)
	if err := s.Start(); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	// Second Start from Active state.
	if err := s.Start(); err == nil {
		t.Error("expected error starting from Active state")
	}
}

func TestManagedSession_CheckTimeout_GuardTimeout(t *testing.T) {
	t.Parallel()
	cfg := DefaultManagedSessionConfig()
	cfg.Guard.OutputTimeout.Enabled = true
	cfg.Guard.OutputTimeout.Timeout = 100 * time.Millisecond
	cfg.MCPGuard.NoCallTimeout.Enabled = false // test only output guard

	s := NewManagedSession(context.Background(), "timeout-test", cfg)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Process a line to start the output timer.
	now := time.Now()
	s.ProcessLine("hello", now)

	// Check timeout before it fires — should be nil.
	noTimeout := s.CheckTimeout(now.Add(50 * time.Millisecond))
	if noTimeout != nil {
		t.Error("expected no timeout after 50ms")
	}

	// Check timeout after it fires.
	timeout := s.CheckTimeout(now.Add(200 * time.Millisecond))
	if timeout == nil {
		t.Error("expected guard event after 200ms timeout")
	}
	if s.State() != SessionFailed {
		t.Errorf("state = %s, want SessionFailed", ManagedSessionStateName(s.State()))
	}
}

func TestManagedSession_CheckTimeout_MCPTimeout(t *testing.T) {
	t.Parallel()
	cfg := DefaultManagedSessionConfig()
	cfg.Guard.OutputTimeout.Enabled = false // test only MCP guard
	cfg.MCPGuard.NoCallTimeout.Enabled = true
	cfg.MCPGuard.NoCallTimeout.Timeout = 100 * time.Millisecond

	s := NewManagedSession(context.Background(), "mcp-timeout-test", cfg)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Process a tool call to start the MCP timer.
	now := time.Now()
	s.ProcessToolCall(MCPToolCall{
		ToolName:  "test-tool",
		Arguments: "{}",
		Timestamp: now,
	})

	// Advance past timeout.
	timeout := s.CheckTimeout(now.Add(200 * time.Millisecond))
	if timeout == nil {
		t.Error("expected MCP guard event after 200ms")
	}
	if s.State() != SessionFailed {
		t.Errorf("state = %s, want SessionFailed", ManagedSessionStateName(s.State()))
	}
}

func TestManagedSession_ProcessToolCall_FrequencyExceeded(t *testing.T) {
	t.Parallel()
	cfg := DefaultManagedSessionConfig()
	cfg.MCPGuard.FrequencyLimit.Enabled = true
	cfg.MCPGuard.FrequencyLimit.Window = 1 * time.Second
	cfg.MCPGuard.FrequencyLimit.MaxCalls = 3

	s := NewManagedSession(context.Background(), "freq-test", cfg)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	now := time.Now()
	var lastResult ToolCallResult
	for i := 0; i < 5; i++ {
		lastResult = s.ProcessToolCall(MCPToolCall{
			ToolName:  "test-tool",
			Arguments: fmt.Sprintf(`{"i":%d}`, i),
			Timestamp: now.Add(time.Duration(i) * 10 * time.Millisecond),
		})
	}

	if lastResult.GuardEvent == nil {
		t.Error("expected guard event after exceeding frequency limit")
	}
}

func TestManagedSession_ProcessToolCall_RepeatDetection(t *testing.T) {
	t.Parallel()
	cfg := DefaultManagedSessionConfig()
	cfg.MCPGuard.RepeatDetection.Enabled = true
	cfg.MCPGuard.RepeatDetection.MaxRepeats = 3
	cfg.MCPGuard.RepeatDetection.WindowSize = 10
	cfg.MCPGuard.RepeatDetection.MatchTool = true
	cfg.MCPGuard.RepeatDetection.MatchArgHash = true

	s := NewManagedSession(context.Background(), "repeat-test", cfg)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	now := time.Now()
	var lastResult ToolCallResult
	// Send 5 identical calls — should trigger after MaxRepeats.
	for i := 0; i < 5; i++ {
		lastResult = s.ProcessToolCall(MCPToolCall{
			ToolName:  "same-tool",
			Arguments: `{"key":"same"}`,
			Timestamp: now.Add(time.Duration(i) * time.Second),
		})
	}

	if lastResult.GuardEvent == nil {
		t.Error("expected guard event for repeated identical calls")
	}
}

// ============================================================================
// Safety Validator — policy enforcement
// ============================================================================

func TestSafetyValidator_BlockThreshold(t *testing.T) {
	t.Parallel()
	cfg := DefaultSafetyConfig()
	cfg.BlockThreshold = 0.1  // Very low — trigger on sensitive patterns.
	cfg.SensitivePatterns = []string{`(?i)password`}

	sv := NewSafetyValidator(cfg)
	assessment := sv.Validate(SafetyAction{
		Type: "agent_output",
		Raw:  "found password: abc123",
	})

	if assessment.Action != PolicyBlock {
		t.Errorf("action = %v, want PolicyBlock for low threshold + sensitive content", assessment.Action)
	}
}

func TestSafetyValidator_MatchBlockedPath_BaseName(t *testing.T) {
	t.Parallel()
	cfg := DefaultSafetyConfig()
	cfg.BlockedPaths = []string{"*.key"}

	sv := NewSafetyValidator(cfg)
	assessment := sv.Validate(SafetyAction{
		Type:      "file_write",
		FilePaths: []string{"/some/deep/dir/secret.key"},
	})

	if assessment.Action != PolicyBlock {
		t.Errorf("action = %v, want PolicyBlock for blocked path pattern *.key", assessment.Action)
	}
}

func TestSafetyValidator_AllowedPaths_Violation(t *testing.T) {
	t.Parallel()
	cfg := DefaultSafetyConfig()
	cfg.AllowedPaths = []string{"/safe/dir/*"}

	sv := NewSafetyValidator(cfg)
	assessment := sv.Validate(SafetyAction{
		Type:      "file_write",
		FilePaths: []string{"/unsafe/dir/evil.sh"},
	})

	if assessment.Action != PolicyBlock {
		t.Errorf("action = %v, want PolicyBlock for path outside allowlist", assessment.Action)
	}
}

// ============================================================================
// Recovery Supervisor — escalation chain
// ============================================================================

func TestSupervisor_MaxRetriesEscalation(t *testing.T) {
	t.Parallel()
	cfg := DefaultSupervisorConfig()
	cfg.MaxRetries = 2

	ctx := context.Background()
	s := NewSupervisor(ctx, cfg)
	s.Start()

	// Send enough errors to exceed MaxRetries.
	var lastDecision RecoveryDecision
	for i := 0; i < 5; i++ {
		lastDecision = s.HandleError(fmt.Sprintf("error %d", i), ErrorClassMCPMalformed)
	}

	if lastDecision.Action != RecoveryEscalate {
		t.Errorf("action = %s after max retries, want Escalate",
			RecoveryActionName(lastDecision.Action))
	}
}

func TestSupervisor_ForceKillChain(t *testing.T) {
	t.Parallel()
	cfg := DefaultSupervisorConfig()
	cfg.MaxRetries = 10 // High to test force-kill path.

	ctx := context.Background()
	s := NewSupervisor(ctx, cfg)
	s.Start()

	// Send crash errors to trigger force-kill path.
	var lastDecision RecoveryDecision
	for i := 0; i < 15; i++ {
		lastDecision = s.HandleError(fmt.Sprintf("crash %d", i), ErrorClassPTYCrash)
	}

	// Should eventually escalate after exhausting retries.
	if lastDecision.Action != RecoveryEscalate {
		t.Errorf("action = %s after force-kill chain, want Escalate",
			RecoveryActionName(lastDecision.Action))
	}
}

// ============================================================================
// Guard — backoff overflow protection
// ============================================================================

func TestGuard_ComputeBackoff_Overflow(t *testing.T) {
	t.Parallel()
	cfg := DefaultGuardConfig()
	cfg.RateLimit.Enabled = true
	cfg.RateLimit.InitialDelay = time.Second
	cfg.RateLimit.MaxDelay = 30 * time.Second
	cfg.RateLimit.Multiplier = 2.0

	g := NewGuard(cfg)

	// Simulate a huge rate limit count that would overflow float64.
	now := time.Now()
	for i := 0; i < 1000; i++ {
		g.ProcessEvent(OutputEvent{
			Type: EventRateLimit,
			Line: "rate limit",
		}, now.Add(time.Duration(i)*time.Millisecond))
	}

	// computeBackoff should be clamped, not overflow or produce NaN.
	delay := g.computeBackoff()
	if delay <= 0 {
		t.Errorf("delay = %v, want > 0", delay)
	}
	if delay > cfg.RateLimit.MaxDelay {
		t.Errorf("delay = %v, want <= MaxDelay %v", delay, cfg.RateLimit.MaxDelay)
	}
	if math.IsInf(float64(delay), 0) || math.IsNaN(float64(delay)) {
		t.Errorf("delay is Inf or NaN: %v", delay)
	}
}

// ============================================================================
// Panel — scrollback trimming
// ============================================================================

func TestPanel_ScrollbackTrimming(t *testing.T) {
	t.Parallel()
	// Use a small scrollback so the test runs fast.
	cfg := PanelConfig{MaxPanes: 9, ScrollbackSize: 100}
	panel := NewPanel(cfg)
	if err := panel.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer panel.Close()

	_, err := panel.AddPane("test", "Test Pane")
	if err != nil {
		t.Fatalf("AddPane: %v", err)
	}

	// Append more lines than ScrollbackSize (100).
	for i := 0; i < 150; i++ {
		if err := panel.AppendOutput("test", fmt.Sprintf("line %d", i)); err != nil {
			t.Fatalf("AppendOutput %d: %v", i, err)
		}
	}

	// Use GetVisibleLines to verify lines are present and trimmed.
	// Request a full view — height = ScrollbackSize.
	lines, err := panel.GetVisibleLines("test", cfg.ScrollbackSize)
	if err != nil {
		t.Fatalf("GetVisibleLines: %v", err)
	}
	if len(lines) > cfg.ScrollbackSize {
		t.Errorf("visible lines len = %d, want <= %d", len(lines), cfg.ScrollbackSize)
	}
	// Last line should be the most recent.
	if len(lines) > 0 {
		lastLine := lines[len(lines)-1]
		expected := "line 149"
		if lastLine != expected {
			t.Errorf("last visible line = %q, want %q", lastLine, expected)
		}
	}
}

// ============================================================================
// Model Navigation — arrow-up path
// ============================================================================

func TestNavigateToModel_ArrowUp(t *testing.T) {
	t.Parallel()
	// Build a menu where the target is ABOVE the current selection.
	menu := &ModelMenu{
		Models:        []string{"modelA", "modelB", "modelC"},
		SelectedIndex: 2, // "modelC" is selected
	}

	keys, err := NavigateToModel(menu, "modelA")
	if err != nil {
		t.Fatalf("NavigateToModel: %v", err)
	}

	// Should contain 2 ArrowUp keystrokes + Enter.
	arrowUpCount := 0
	for i := 0; i < len(keys); i++ {
		if keys[i] == '\x1b' && i+2 < len(keys) && keys[i+1] == '[' && keys[i+2] == 'A' {
			arrowUpCount++
		}
	}
	if arrowUpCount != 2 {
		t.Errorf("expected 2 ArrowUp sequences, got %d in %q", arrowUpCount, keys)
	}
}
