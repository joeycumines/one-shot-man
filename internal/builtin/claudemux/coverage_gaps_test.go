package claudemux

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/dop251/goja"
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
	enqueueFn   func(string) (int, error)
	interruptFn func() error
	statusFn    func() GetStatusResult
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
	sockPath := tempSockPath(t)

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
	sockPath := tempSockPath(t)

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
	sockPath := tempSockPath(t)

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
	sockPath := tempSockPath(t)

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
	req := `{"method":"EnqueueTask","params":"not-an-object"}` + "\n"
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
	cfg.BlockThreshold = 0.1 // Very low — trigger on sensitive patterns.
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
	// After escalation, state becomes Stopped. Subsequent errors return Abort.
	sawEscalate := false
	for i := 0; i < 5; i++ {
		d := s.HandleError(fmt.Sprintf("error %d", i), ErrorClassMCPMalformed)
		if d.Action == RecoveryEscalate {
			sawEscalate = true
		}
	}

	if !sawEscalate {
		t.Error("expected at least one Escalate decision after max retries")
	}
}

func TestSupervisor_ForceKillChain(t *testing.T) {
	t.Parallel()
	cfg := DefaultSupervisorConfig()
	cfg.MaxRetries = 10 // High to test force-kill path.

	ctx := context.Background()
	s := NewSupervisor(ctx, cfg)
	s.Start()

	// Send crash errors to trigger force-kill / escalation.
	// Track all unique actions seen.
	sawForceKill := false
	sawEscalateOrAbort := false
	for i := 0; i < 15; i++ {
		d := s.HandleError(fmt.Sprintf("crash %d", i), ErrorClassPTYCrash)
		if d.Action == RecoveryForceKill {
			sawForceKill = true
			// After force-kill, confirm recovery to allow further retries.
			s.ConfirmRecovery()
		} else if d.Action == RecoveryEscalate || d.Action == RecoveryAbort {
			sawEscalateOrAbort = true
			break
		}
	}

	if !sawForceKill {
		t.Error("expected at least one ForceKill decision for PTYCrash")
	}
	if !sawEscalateOrAbort {
		t.Error("expected eventual Escalate or Abort after force-kill chain")
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

// ============================================================================
// parseSpawnOpts — field extraction from goja object
// ============================================================================

func TestParseSpawnOpts_AllFields(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	obj := rt.NewObject()
	_ = obj.Set("model", "claude-sonnet-4-20250514")
	_ = obj.Set("dir", "/tmp/workdir")
	_ = obj.Set("rows", 40)
	_ = obj.Set("cols", 120)
	_ = obj.Set("env", map[string]interface{}{"FOO": "bar", "BAZ": "qux"})
	_ = obj.Set("args", []interface{}{"--verbose", "--debug"})

	opts := &SpawnOpts{}
	parseSpawnOpts(rt, obj, opts)

	if opts.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q, want %q", opts.Model, "claude-sonnet-4-20250514")
	}
	if opts.Dir != "/tmp/workdir" {
		t.Errorf("Dir = %q, want %q", opts.Dir, "/tmp/workdir")
	}
	if opts.Rows != 40 {
		t.Errorf("Rows = %d, want 40", opts.Rows)
	}
	if opts.Cols != 120 {
		t.Errorf("Cols = %d, want 120", opts.Cols)
	}
	if len(opts.Env) != 2 || opts.Env["FOO"] != "bar" {
		t.Errorf("Env = %v, want {FOO:bar, BAZ:qux}", opts.Env)
	}
	if len(opts.Args) != 2 || opts.Args[0] != "--verbose" {
		t.Errorf("Args = %v, want [--verbose --debug]", opts.Args)
	}
}

func TestParseSpawnOpts_NilAndUndefinedSkipped(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	obj := rt.NewObject()
	// Set fields to null/undefined — they should be skipped.
	_ = obj.Set("model", goja.Null())
	_ = obj.Set("dir", goja.Undefined())
	// rows, cols, env, args not set at all.

	opts := &SpawnOpts{Model: "original", Dir: "/original"}
	parseSpawnOpts(rt, obj, opts)

	if opts.Model != "original" {
		t.Errorf("Model = %q, want original (null should skip)", opts.Model)
	}
	if opts.Dir != "/original" {
		t.Errorf("Dir = %q, want /original (undefined should skip)", opts.Dir)
	}
}

func TestParseSpawnOpts_BadEnvType(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	obj := rt.NewObject()
	_ = obj.Set("env", "not-an-object") // wrong type

	opts := &SpawnOpts{}
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for bad env type")
		}
		exc, ok := r.(*goja.Object)
		if !ok {
			t.Fatalf("expected *goja.Object, got %T", r)
		}
		msg := exc.Get("message").String()
		if msg != "spawn: env must be a {string: string} object" {
			t.Errorf("message = %q, want env error", msg)
		}
	}()
	parseSpawnOpts(rt, obj, opts)
}

func TestParseSpawnOpts_BadArgsType(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	obj := rt.NewObject()
	_ = obj.Set("args", 42) // wrong type

	opts := &SpawnOpts{}
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for bad args type")
		}
		exc, ok := r.(*goja.Object)
		if !ok {
			t.Fatalf("expected *goja.Object, got %T", r)
		}
		msg := exc.Get("message").String()
		if msg != "spawn: args must be an array of strings" {
			t.Errorf("message = %q, want args error", msg)
		}
	}()
	parseSpawnOpts(rt, obj, opts)
}

// ============================================================================
// unwrapProvider — provider extraction from JS object
// ============================================================================

func TestUnwrapProvider_MissingGoProvider(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	// Object without _goProvider field.
	obj := rt.NewObject()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for missing _goProvider")
		}
		exc, ok := r.(*goja.Object)
		if !ok {
			t.Fatalf("expected *goja.Object, got %T", r)
		}
		msg := exc.Get("message").String()
		if msg != "register: argument is not a valid provider" {
			t.Errorf("message = %q, want provider error", msg)
		}
	}()
	unwrapProvider(rt, rt.ToValue(obj))
}

func TestUnwrapProvider_NullGoProvider(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	obj := rt.NewObject()
	_ = obj.Set("_goProvider", goja.Null())

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for null _goProvider")
		}
	}()
	unwrapProvider(rt, rt.ToValue(obj))
}

func TestUnwrapProvider_WrongType(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	obj := rt.NewObject()
	_ = obj.Set("_goProvider", "not-a-provider")

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for wrong type _goProvider")
		}
		exc, ok := r.(*goja.Object)
		if !ok {
			t.Fatalf("expected *goja.Object, got %T", r)
		}
		msg := exc.Get("message").String()
		if msg != "register: argument is not a valid provider" {
			t.Errorf("message = %q, want provider error", msg)
		}
	}()
	unwrapProvider(rt, rt.ToValue(obj))
}

func TestUnwrapProvider_Valid(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	obj := rt.NewObject()
	sp := &stubProvider{providerName: "test-provider"}
	_ = obj.Set("_goProvider", sp)

	p := unwrapProvider(rt, rt.ToValue(obj))
	if p.Name() != "test-provider" {
		t.Errorf("Name() = %q, want %q", p.Name(), "test-provider")
	}
}

// ============================================================================
// wrapAgentHandle — JS wrapper for AgentHandle
// ============================================================================

func TestWrapAgentHandle_Send(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	h := &stubAgentHandle{}
	jsObj := wrapAgentHandle(rt, h).ToObject(rt)

	send, _ := goja.AssertFunction(jsObj.Get("send"))
	_, err := send(goja.Undefined(), rt.ToValue("hello"))
	if err != nil {
		t.Fatalf("send threw: %v", err)
	}
	if h.lastSent != "hello" {
		t.Errorf("lastSent = %q, want %q", h.lastSent, "hello")
	}
}

func TestWrapAgentHandle_SendError(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	h := &stubAgentHandle{sendErr: errors.New("network down")}
	jsObj := wrapAgentHandle(rt, h).ToObject(rt)

	send, _ := goja.AssertFunction(jsObj.Get("send"))
	_, err := send(goja.Undefined(), rt.ToValue("hello"))
	if err == nil {
		t.Fatal("expected error from send")
	}
}

func TestWrapAgentHandle_SendNoArgs(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	h := &stubAgentHandle{}
	jsObj := wrapAgentHandle(rt, h).ToObject(rt)

	send, _ := goja.AssertFunction(jsObj.Get("send"))
	_, err := send(goja.Undefined()) // no arguments
	if err == nil {
		t.Fatal("expected TypeError for send with no args")
	}
}

func TestWrapAgentHandle_ReceiveEOF(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	h := &stubAgentHandle{recvData: "", recvErr: io.EOF}
	jsObj := wrapAgentHandle(rt, h).ToObject(rt)

	recv, _ := goja.AssertFunction(jsObj.Get("receive"))
	val, err := recv(goja.Undefined())
	if err != nil {
		t.Fatalf("receive threw: %v", err)
	}
	if val.String() != "" {
		t.Errorf("receive on EOF = %q, want empty", val.String())
	}
}

func TestWrapAgentHandle_ReceiveErrorWithData(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	h := &stubAgentHandle{recvData: "partial data", recvErr: errors.New("interrupted")}
	jsObj := wrapAgentHandle(rt, h).ToObject(rt)

	recv, _ := goja.AssertFunction(jsObj.Get("receive"))
	val, err := recv(goja.Undefined())
	if err != nil {
		t.Fatalf("receive threw: %v", err)
	}
	// When error has data, it returns the data.
	if val.String() != "partial data" {
		t.Errorf("receive = %q, want %q", val.String(), "partial data")
	}
}

func TestWrapAgentHandle_ReceiveErrorNoData(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	h := &stubAgentHandle{recvData: "", recvErr: errors.New("interrupted")}
	jsObj := wrapAgentHandle(rt, h).ToObject(rt)

	recv, _ := goja.AssertFunction(jsObj.Get("receive"))
	val, err := recv(goja.Undefined())
	if err != nil {
		t.Fatalf("receive threw: %v", err)
	}
	if val.String() != "" {
		t.Errorf("receive = %q, want empty for error-no-data", val.String())
	}
}

func TestWrapAgentHandle_Close(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	h := &stubAgentHandle{}
	jsObj := wrapAgentHandle(rt, h).ToObject(rt)

	cl, _ := goja.AssertFunction(jsObj.Get("close"))
	_, err := cl(goja.Undefined())
	if err != nil {
		t.Fatalf("close threw: %v", err)
	}
	if !h.closed {
		t.Error("expected Close to be called")
	}
}

func TestWrapAgentHandle_CloseError(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	h := &stubAgentHandle{closeErr: errors.New("already closed")}
	jsObj := wrapAgentHandle(rt, h).ToObject(rt)

	cl, _ := goja.AssertFunction(jsObj.Get("close"))
	_, err := cl(goja.Undefined())
	if err == nil {
		t.Fatal("expected error from close")
	}
}

func TestWrapAgentHandle_IsAlive(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	h := &stubAgentHandle{alive: true}
	jsObj := wrapAgentHandle(rt, h).ToObject(rt)

	isAlive, _ := goja.AssertFunction(jsObj.Get("isAlive"))
	val, err := isAlive(goja.Undefined())
	if err != nil {
		t.Fatalf("isAlive threw: %v", err)
	}
	if !val.ToBoolean() {
		t.Error("isAlive should return true")
	}
}

func TestWrapAgentHandle_Wait(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	h := &stubAgentHandle{waitCode: 0, waitErr: nil}
	jsObj := wrapAgentHandle(rt, h).ToObject(rt)

	wait, _ := goja.AssertFunction(jsObj.Get("wait"))
	val, err := wait(goja.Undefined())
	if err != nil {
		t.Fatalf("wait threw: %v", err)
	}
	obj := val.ToObject(rt)
	code := obj.Get("code").ToInteger()
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	errVal := obj.Get("error")
	if !goja.IsNull(errVal) {
		t.Errorf("error = %v, want null", errVal)
	}
}

func TestWrapAgentHandle_WaitWithError(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	h := &stubAgentHandle{waitCode: 1, waitErr: errors.New("exit status 1")}
	jsObj := wrapAgentHandle(rt, h).ToObject(rt)

	wait, _ := goja.AssertFunction(jsObj.Get("wait"))
	val, err := wait(goja.Undefined())
	if err != nil {
		t.Fatalf("wait threw: %v", err)
	}
	obj := val.ToObject(rt)
	code := obj.Get("code").ToInteger()
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	errStr := obj.Get("error").String()
	if errStr != "exit status 1" {
		t.Errorf("error = %q, want %q", errStr, "exit status 1")
	}
}

// ============================================================================
// jsToModelMenu — JS object to ModelMenu conversion
// ============================================================================

func TestJsToModelMenu_NullPanics(t *testing.T) {
	t.Parallel()
	rt := goja.New()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for null menu")
		}
	}()
	jsToModelMenu(rt, goja.Null())
}

func TestJsToModelMenu_UndefinedPanics(t *testing.T) {
	t.Parallel()
	rt := goja.New()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for undefined menu")
		}
	}()
	jsToModelMenu(rt, goja.Undefined())
}

func TestJsToModelMenu_ValidObject(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	obj := rt.NewObject()
	_ = obj.Set("models", []interface{}{"a", "b", "c"})
	_ = obj.Set("selectedIndex", 1)

	menu := jsToModelMenu(rt, rt.ToValue(obj))
	if len(menu.Models) != 3 {
		t.Errorf("Models len = %d, want 3", len(menu.Models))
	}
	if menu.SelectedIndex != 1 {
		t.Errorf("SelectedIndex = %d, want 1", menu.SelectedIndex)
	}
}

func TestJsToModelMenu_BadModelsArray(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	obj := rt.NewObject()
	_ = obj.Set("models", 42) // not an array

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for bad models type")
		}
	}()
	jsToModelMenu(rt, rt.ToValue(obj))
}

// ============================================================================
// modelMenuToJS — ModelMenu to JS object conversion
// ============================================================================

func TestModelMenuToJS_RoundTrip(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	menu := &ModelMenu{
		Models:        []string{"alpha", "beta"},
		SelectedIndex: 0,
	}

	jsVal := modelMenuToJS(rt, menu)
	obj := jsVal.ToObject(rt)

	modelsVal := obj.Get("models")
	var models []string
	if err := rt.ExportTo(modelsVal, &models); err != nil {
		t.Fatalf("ExportTo models: %v", err)
	}
	if len(models) != 2 || models[0] != "alpha" {
		t.Errorf("models = %v, want [alpha beta]", models)
	}
	idx := obj.Get("selectedIndex").ToInteger()
	if idx != 0 {
		t.Errorf("selectedIndex = %d, want 0", idx)
	}
}

// ============================================================================
// Mock types for module binding tests
// ============================================================================

// stubProvider implements Provider for testing unwrapProvider.
type stubProvider struct {
	providerName string
}

func (s *stubProvider) Name() string                                                { return s.providerName }
func (s *stubProvider) Spawn(_ context.Context, _ SpawnOpts) (AgentHandle, error)   { return nil, nil }
func (s *stubProvider) Capabilities() ProviderCapabilities                          { return ProviderCapabilities{} }

// stubAgentHandle implements AgentHandle for testing wrapAgentHandle.
type stubAgentHandle struct {
	lastSent string
	sendErr  error
	recvData string
	recvErr  error
	closeErr error
	closed   bool
	alive    bool
	waitCode int
	waitErr  error
}

func (s *stubAgentHandle) Send(input string) error {
	s.lastSent = input
	return s.sendErr
}

func (s *stubAgentHandle) Receive() (string, error) {
	return s.recvData, s.recvErr
}

func (s *stubAgentHandle) Close() error {
	s.closed = true
	return s.closeErr
}

func (s *stubAgentHandle) IsAlive() bool {
	return s.alive
}

func (s *stubAgentHandle) Wait() (int, error) {
	return s.waitCode, s.waitErr
}
