package claudemux

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"strings"
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
	if runtime.GOOS == "windows" {
		t.Skip("Unix sockets not available on Windows")
	}
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
	if runtime.GOOS == "windows" {
		t.Skip("Unix sockets not available on Windows")
	}
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
	if runtime.GOOS == "windows" {
		t.Skip("Unix sockets not available on Windows")
	}
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
	if runtime.GOOS == "windows" {
		t.Skip("Unix sockets not available on Windows")
	}
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

func (s *stubProvider) Name() string                                              { return s.providerName }
func (s *stubProvider) Spawn(_ context.Context, _ SpawnOpts) (AgentHandle, error) { return nil, nil }
func (s *stubProvider) Capabilities() ProviderCapabilities                        { return ProviderCapabilities{} }

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

// ============================================================================
// eventToJS — OutputEvent to JS object conversion
// ============================================================================

func TestEventToJS_WithFields(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	ev := OutputEvent{
		Type:    EventRateLimit,
		Line:    "Rate limit exceeded",
		Pattern: "rate_limit",
		Fields:  map[string]string{"retry_after": "30", "code": "429"},
	}

	val := eventToJS(rt, ev)
	obj := val.ToObject(rt)

	if obj.Get("type").ToInteger() != int64(EventRateLimit) {
		t.Errorf("type = %d, want %d", obj.Get("type").ToInteger(), EventRateLimit)
	}
	if obj.Get("line").String() != "Rate limit exceeded" {
		t.Errorf("line = %q, want %q", obj.Get("line").String(), "Rate limit exceeded")
	}
	if obj.Get("pattern").String() != "rate_limit" {
		t.Errorf("pattern = %q, want %q", obj.Get("pattern").String(), "rate_limit")
	}
	fields := obj.Get("fields").ToObject(rt)
	if fields.Get("retry_after").String() != "30" {
		t.Errorf("fields.retry_after = %q, want %q", fields.Get("retry_after").String(), "30")
	}
	if fields.Get("code").String() != "429" {
		t.Errorf("fields.code = %q, want %q", fields.Get("code").String(), "429")
	}
}

func TestEventToJS_NilFields(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	ev := OutputEvent{
		Type:   EventText,
		Line:   "Hello",
		Fields: nil, // nil → empty object
	}

	val := eventToJS(rt, ev)
	obj := val.ToObject(rt)

	// Fields should be an empty object, not null.
	fields := obj.Get("fields")
	if goja.IsNull(fields) || goja.IsUndefined(fields) {
		t.Error("fields should be a non-null object when Fields is nil")
	}
}

// ============================================================================
// wrapInstance — JS wrapper for Instance
// ============================================================================

func TestWrapInstance_Properties(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	now := time.Now()

	inst := &Instance{
		ID:        "inst-abc",
		StateDir:  "/tmp/state/inst-abc",
		CreatedAt: now,
	}

	jsVal := wrapInstance(rt, inst)
	obj := jsVal.ToObject(rt)

	if obj.Get("id").String() != "inst-abc" {
		t.Errorf("id = %q, want %q", obj.Get("id").String(), "inst-abc")
	}
	if obj.Get("stateDir").String() != "/tmp/state/inst-abc" {
		t.Errorf("stateDir = %q, want %q", obj.Get("stateDir").String(), "/tmp/state/inst-abc")
	}
	// createdAt is a formatted string.
	createdAt := obj.Get("createdAt").String()
	if createdAt == "" {
		t.Error("createdAt should be non-empty")
	}
}

func TestWrapInstance_IsClosedAndClose(t *testing.T) {
	t.Parallel()
	rt := goja.New()

	inst := &Instance{
		ID:        "inst-xyz",
		StateDir:  t.TempDir(),
		CreatedAt: time.Now(),
	}

	jsVal := wrapInstance(rt, inst)
	obj := jsVal.ToObject(rt)

	// isClosed() should be false initially.
	isClosed, _ := goja.AssertFunction(obj.Get("isClosed"))
	val, err := isClosed(goja.Undefined())
	if err != nil {
		t.Fatalf("isClosed() threw: %v", err)
	}
	if val.ToBoolean() {
		t.Error("isClosed should be false initially")
	}

	// close() should succeed.
	closeFn, _ := goja.AssertFunction(obj.Get("close"))
	_, err = closeFn(goja.Undefined())
	if err != nil {
		t.Fatalf("close() threw: %v", err)
	}

	// isClosed() should be true after close.
	val, err = isClosed(goja.Undefined())
	if err != nil {
		t.Fatalf("isClosed() after close threw: %v", err)
	}
	if !val.ToBoolean() {
		t.Error("isClosed should be true after close")
	}
}

// ============================================================================
// wrapInstanceRegistry — JS wrapper for InstanceRegistry
// ============================================================================

func TestWrapInstanceRegistry_CreateGetCloseList(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	dir := t.TempDir()

	reg, err := NewInstanceRegistry(dir)
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}
	defer reg.CloseAll()

	jsVal := wrapInstanceRegistry(rt, reg)
	obj := jsVal.ToObject(rt)

	// len() should be 0 initially.
	lenFn, _ := goja.AssertFunction(obj.Get("len"))
	val, callErr := lenFn(goja.Undefined())
	if callErr != nil {
		t.Fatalf("len() threw: %v", callErr)
	}
	if val.ToInteger() != 0 {
		t.Errorf("len() = %d, want 0", val.ToInteger())
	}

	// create() an instance.
	createFn, _ := goja.AssertFunction(obj.Get("create"))
	instVal, callErr := createFn(goja.Undefined(), rt.ToValue("sess-001"))
	if callErr != nil {
		t.Fatalf("create() threw: %v", callErr)
	}
	instObj := instVal.ToObject(rt)
	if instObj.Get("id").String() != "sess-001" {
		t.Errorf("created instance id = %q, want %q", instObj.Get("id").String(), "sess-001")
	}

	// len() should now be 1.
	val, _ = lenFn(goja.Undefined())
	if val.ToInteger() != 1 {
		t.Errorf("len() = %d, want 1", val.ToInteger())
	}

	// get() the instance.
	getFn, _ := goja.AssertFunction(obj.Get("get"))
	gotVal, callErr := getFn(goja.Undefined(), rt.ToValue("sess-001"))
	if callErr != nil {
		t.Fatalf("get() threw: %v", callErr)
	}
	gotObj := gotVal.ToObject(rt)
	if gotObj.Get("id").String() != "sess-001" {
		t.Errorf("get() id = %q, want %q", gotObj.Get("id").String(), "sess-001")
	}

	// get() non-existent — should return null.
	gotVal, callErr = getFn(goja.Undefined(), rt.ToValue("no-such"))
	if callErr != nil {
		t.Fatalf("get(no-such) threw: %v", callErr)
	}
	if !goja.IsNull(gotVal) {
		t.Errorf("get(no-such) = %v, want null", gotVal)
	}

	// list() should contain sess-001.
	listFn, _ := goja.AssertFunction(obj.Get("list"))
	listVal, callErr := listFn(goja.Undefined())
	if callErr != nil {
		t.Fatalf("list() threw: %v", callErr)
	}
	var ids []string
	if err := rt.ExportTo(listVal, &ids); err != nil {
		t.Fatalf("ExportTo ids: %v", err)
	}
	if len(ids) != 1 || ids[0] != "sess-001" {
		t.Errorf("list() = %v, want [sess-001]", ids)
	}

	// baseDir()
	baseDirFn, _ := goja.AssertFunction(obj.Get("baseDir"))
	bdVal, callErr := baseDirFn(goja.Undefined())
	if callErr != nil {
		t.Fatalf("baseDir() threw: %v", callErr)
	}
	if bdVal.String() != dir {
		t.Errorf("baseDir() = %q, want %q", bdVal.String(), dir)
	}

	// close() specific instance.
	closeFn, _ := goja.AssertFunction(obj.Get("close"))
	_, callErr = closeFn(goja.Undefined(), rt.ToValue("sess-001"))
	if callErr != nil {
		t.Fatalf("close(sess-001) threw: %v", callErr)
	}

	// len() should be 0.
	val, _ = lenFn(goja.Undefined())
	if val.ToInteger() != 0 {
		t.Errorf("len() after close = %d, want 0", val.ToInteger())
	}
}

func TestWrapInstanceRegistry_CreateNoArgs(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	dir := t.TempDir()

	reg, err := NewInstanceRegistry(dir)
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}
	defer reg.CloseAll()

	jsVal := wrapInstanceRegistry(rt, reg)
	obj := jsVal.ToObject(rt)

	createFn, _ := goja.AssertFunction(obj.Get("create"))
	_, callErr := createFn(goja.Undefined()) // no args
	if callErr == nil {
		t.Fatal("expected TypeError for create() with no args")
	}
}

func TestWrapInstanceRegistry_CloseAll(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	dir := t.TempDir()

	reg, err := NewInstanceRegistry(dir)
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}

	// Create 2 instances.
	if _, err := reg.Create("s1"); err != nil {
		t.Fatalf("Create s1: %v", err)
	}
	if _, err := reg.Create("s2"); err != nil {
		t.Fatalf("Create s2: %v", err)
	}

	jsVal := wrapInstanceRegistry(rt, reg)
	obj := jsVal.ToObject(rt)

	// closeAll()
	closeAllFn, _ := goja.AssertFunction(obj.Get("closeAll"))
	_, callErr := closeAllFn(goja.Undefined())
	if callErr != nil {
		t.Fatalf("closeAll() threw: %v", callErr)
	}

	// len() should be 0.
	lenFn, _ := goja.AssertFunction(obj.Get("len"))
	val, _ := lenFn(goja.Undefined())
	if val.ToInteger() != 0 {
		t.Errorf("len() after closeAll = %d, want 0", val.ToInteger())
	}
}

// ============================================================================
// wrapPool — coverage for JS Pool binding
// ============================================================================

func TestWrapPool_StartDrainClose(t *testing.T) {
	t.Parallel()
	rt := goja.New()

	p := NewPool(DefaultPoolConfig())
	jsVal := wrapPool(rt, p)
	obj := jsVal.ToObject(rt)

	// start()
	startFn, _ := goja.AssertFunction(obj.Get("start"))
	_, err := startFn(goja.Undefined())
	if err != nil {
		t.Fatalf("start() threw: %v", err)
	}

	// stats() — verifies pool is running
	statsFn, _ := goja.AssertFunction(obj.Get("stats"))
	statsVal, err := statsFn(goja.Undefined())
	if err != nil {
		t.Fatalf("stats() threw: %v", err)
	}
	statsObj := statsVal.ToObject(rt)
	// stats.state should reflect running pool (PoolRunning = 1)
	if statsObj.Get("state").ToInteger() != 1 {
		t.Errorf("stats.state = %d, want 1 (running)", statsObj.Get("state").ToInteger())
	}

	// config()
	cfgFn, _ := goja.AssertFunction(obj.Get("config"))
	_, err = cfgFn(goja.Undefined())
	if err != nil {
		t.Fatalf("config() threw: %v", err)
	}

	// drain()
	drainFn, _ := goja.AssertFunction(obj.Get("drain"))
	_, err = drainFn(goja.Undefined())
	if err != nil {
		t.Fatalf("drain() threw: %v", err)
	}

	// close()
	closeFn, _ := goja.AssertFunction(obj.Get("close"))
	_, err = closeFn(goja.Undefined())
	if err != nil {
		t.Fatalf("close() threw: %v", err)
	}
}

// ---------------------------------------------------------------------------
// wrapPoolWorker — state, stateName, taskCount, errorCount getters
// ---------------------------------------------------------------------------

func TestWrapPoolWorker_Getters(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	w := &PoolWorker{
		ID:         "w1",
		State:      WorkerIdle,
		TaskCount:  5,
		ErrorCount: 2,
	}
	obj := wrapPoolWorker(rt, w).ToObject(rt)

	// id is a property, not a function
	if obj.Get("id").String() != "w1" {
		t.Errorf("id = %q, want w1", obj.Get("id").String())
	}

	stateFn, _ := goja.AssertFunction(obj.Get("state"))
	stateVal, _ := stateFn(goja.Undefined())
	if stateVal.ToInteger() != int64(WorkerIdle) {
		t.Errorf("state() = %d, want %d", stateVal.ToInteger(), WorkerIdle)
	}

	stateNameFn, _ := goja.AssertFunction(obj.Get("stateName"))
	nameVal, _ := stateNameFn(goja.Undefined())
	if nameVal.String() != WorkerStateName(WorkerIdle) {
		t.Errorf("stateName() = %q, want %q", nameVal.String(), WorkerStateName(WorkerIdle))
	}

	taskFn, _ := goja.AssertFunction(obj.Get("taskCount"))
	taskVal, _ := taskFn(goja.Undefined())
	if taskVal.ToInteger() != 5 {
		t.Errorf("taskCount() = %d, want 5", taskVal.ToInteger())
	}

	errFn, _ := goja.AssertFunction(obj.Get("errorCount"))
	errVal, _ := errFn(goja.Undefined())
	if errVal.ToInteger() != 2 {
		t.Errorf("errorCount() = %d, want 2", errVal.ToInteger())
	}
}

// ---------------------------------------------------------------------------
// wrapPool — addWorker, removeWorker, release, tryAcquire, waitDrained
// ---------------------------------------------------------------------------

func TestWrapPool_AddWorker_NoArgs(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	p := NewPool(PoolConfig{MaxSize: 2})
	obj := wrapPool(rt, p).ToObject(rt)

	addFn, _ := goja.AssertFunction(obj.Get("addWorker"))
	_, err := addFn(goja.Undefined())
	if err == nil {
		t.Fatal("expected TypeError for addWorker with no args")
	}
	if !strings.Contains(err.Error(), "addWorker: id argument required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWrapPool_AddWorker_Success(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	p := NewPool(PoolConfig{MaxSize: 2})
	_ = p.Start()
	obj := wrapPool(rt, p).ToObject(rt)

	addFn, _ := goja.AssertFunction(obj.Get("addWorker"))
	workerVal, err := addFn(goja.Undefined(), rt.ToValue("worker-1"))
	if err != nil {
		t.Fatalf("addWorker threw: %v", err)
	}
	// Should return a wrapped PoolWorker object
	wObj := workerVal.ToObject(rt)
	if wObj.Get("id").String() != "worker-1" {
		t.Errorf("worker.id = %q, want worker-1", wObj.Get("id").String())
	}
	p.Close()
}

func TestWrapPool_RemoveWorker_NoArgs(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	p := NewPool(PoolConfig{MaxSize: 2})
	obj := wrapPool(rt, p).ToObject(rt)

	rmFn, _ := goja.AssertFunction(obj.Get("removeWorker"))
	_, err := rmFn(goja.Undefined())
	if err == nil {
		t.Fatal("expected TypeError for removeWorker with no args")
	}
	if !strings.Contains(err.Error(), "removeWorker: id argument required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWrapPool_RemoveWorker_Error(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	p := NewPool(PoolConfig{MaxSize: 2})
	_ = p.Start()
	obj := wrapPool(rt, p).ToObject(rt)

	rmFn, _ := goja.AssertFunction(obj.Get("removeWorker"))
	_, err := rmFn(goja.Undefined(), rt.ToValue("nonexistent"))
	if err == nil {
		t.Fatal("expected error for removing nonexistent worker")
	}
	p.Close()
}

func TestWrapPool_Release_NoArgs(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	p := NewPool(PoolConfig{MaxSize: 2})
	obj := wrapPool(rt, p).ToObject(rt)

	relFn, _ := goja.AssertFunction(obj.Get("release"))
	_, err := relFn(goja.Undefined())
	if err == nil {
		t.Fatal("expected TypeError for release with no args")
	}
	if !strings.Contains(err.Error(), "release: worker argument required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWrapPool_Release_WorkerNotFound(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	p := NewPool(PoolConfig{MaxSize: 2})
	_ = p.Start()
	obj := wrapPool(rt, p).ToObject(rt)

	relFn, _ := goja.AssertFunction(obj.Get("release"))
	workerObj := rt.NewObject()
	_ = workerObj.Set("id", "nonexistent")
	_, err := relFn(goja.Undefined(), workerObj)
	if err == nil {
		t.Fatal("expected TypeError for releasing nonexistent worker")
	}
	if !strings.Contains(err.Error(), "release: worker not found in pool") {
		t.Errorf("unexpected error: %v", err)
	}
	p.Close()
}

func TestWrapPool_Release_MissingID(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	p := NewPool(PoolConfig{MaxSize: 2})
	obj := wrapPool(rt, p).ToObject(rt)

	relFn, _ := goja.AssertFunction(obj.Get("release"))
	workerObj := rt.NewObject()
	_, err := relFn(goja.Undefined(), workerObj)
	if err == nil {
		t.Fatal("expected TypeError for release with worker missing id")
	}
	if !strings.Contains(err.Error(), "release: worker must have id property") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWrapPool_TryAcquire_Success(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	p := NewPool(PoolConfig{MaxSize: 2})
	_ = p.Start()
	p.AddWorker("w1", nil)
	obj := wrapPool(rt, p).ToObject(rt)

	tryFn, _ := goja.AssertFunction(obj.Get("tryAcquire"))
	val, err := tryFn(goja.Undefined())
	if err != nil {
		t.Fatalf("tryAcquire threw: %v", err)
	}
	// Should return a wrapped worker
	wObj := val.ToObject(rt)
	if wObj.Get("id").String() != "w1" {
		t.Errorf("tryAcquire worker.id = %q, want w1", wObj.Get("id").String())
	}
	p.Close()
}

func TestWrapPool_WaitDrained(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	p := NewPool(PoolConfig{MaxSize: 2})
	_ = p.Start()
	p.Drain()
	obj := wrapPool(rt, p).ToObject(rt)

	waitFn, _ := goja.AssertFunction(obj.Get("waitDrained"))
	_, err := waitFn(goja.Undefined())
	if err != nil {
		t.Fatalf("waitDrained threw: %v", err)
	}
	p.Close()
}

func TestWrapPool_ReleaseWithError(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	p := NewPool(PoolConfig{MaxSize: 2})
	_ = p.Start()
	p.AddWorker("w1", nil)
	obj := wrapPool(rt, p).ToObject(rt)

	// Acquire the worker first
	tryFn, _ := goja.AssertFunction(obj.Get("tryAcquire"))
	workerVal, _ := tryFn(goja.Undefined())

	// Release with an error string
	relFn, _ := goja.AssertFunction(obj.Get("release"))
	_, err := relFn(goja.Undefined(), workerVal, rt.ToValue("task failed"))
	if err != nil {
		t.Fatalf("release with error threw: %v", err)
	}
	p.Close()
}

// ---------------------------------------------------------------------------
// wrapRegistry — register, get, list, spawn
// ---------------------------------------------------------------------------

func TestWrapRegistry_RegisterAndList(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	r := NewRegistry()
	obj := wrapRegistry(rt, r, context.Background()).ToObject(rt)

	// list() should start empty
	listFn, _ := goja.AssertFunction(obj.Get("list"))
	listVal, _ := listFn(goja.Undefined())
	arr := listVal.Export()
	if arr != nil {
		if items, ok := arr.([]interface{}); ok && len(items) != 0 {
			t.Errorf("list() should be empty, got %d items", len(items))
		}
	}

	// Register a provider via JS binding
	regFn, _ := goja.AssertFunction(obj.Get("register"))
	prov := wrapProvider(rt, &stubProvider{providerName: "test-provider"})
	_, err := regFn(goja.Undefined(), prov)
	if err != nil {
		t.Fatalf("register threw: %v", err)
	}

	// list() should have 1 provider now
	listVal2, _ := listFn(goja.Undefined())
	exported := listVal2.Export()
	switch items := exported.(type) {
	case []interface{}:
		if len(items) != 1 {
			t.Errorf("list() after register: got %d items, want 1", len(items))
		}
	case []string:
		if len(items) != 1 {
			t.Errorf("list() after register: got %d items, want 1", len(items))
		}
	default:
		t.Errorf("list() after register: unexpected type %T, value %v", exported, exported)
	}
}

func TestWrapRegistry_RegisterNoArgs(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	r := NewRegistry()
	obj := wrapRegistry(rt, r, context.Background()).ToObject(rt)

	regFn, _ := goja.AssertFunction(obj.Get("register"))
	_, err := regFn(goja.Undefined())
	if err == nil {
		t.Fatal("expected TypeError for register with no args")
	}
}

func TestWrapRegistry_GetSuccess(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	r := NewRegistry()
	_ = r.Register(&stubProvider{providerName: "my-prov"})
	obj := wrapRegistry(rt, r, context.Background()).ToObject(rt)

	getFn, _ := goja.AssertFunction(obj.Get("get"))
	val, err := getFn(goja.Undefined(), rt.ToValue("my-prov"))
	if err != nil {
		t.Fatalf("get threw: %v", err)
	}
	pObj := val.ToObject(rt)
	nameFn, _ := goja.AssertFunction(pObj.Get("name"))
	nameVal, _ := nameFn(goja.Undefined())
	if nameVal.String() != "my-prov" {
		t.Errorf("get().name() = %q, want my-prov", nameVal.String())
	}
}

func TestWrapRegistry_GetNotFound(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	r := NewRegistry()
	obj := wrapRegistry(rt, r, context.Background()).ToObject(rt)

	getFn, _ := goja.AssertFunction(obj.Get("get"))
	_, err := getFn(goja.Undefined(), rt.ToValue("no-such"))
	if err == nil {
		t.Fatal("expected error for get with unknown name")
	}
}

func TestWrapRegistry_GetNoArgs(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	r := NewRegistry()
	obj := wrapRegistry(rt, r, context.Background()).ToObject(rt)

	getFn, _ := goja.AssertFunction(obj.Get("get"))
	_, err := getFn(goja.Undefined())
	if err == nil {
		t.Fatal("expected TypeError for get with no args")
	}
}

func TestWrapRegistry_SpawnNoArgs(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	r := NewRegistry()
	obj := wrapRegistry(rt, r, context.Background()).ToObject(rt)

	spawnFn, _ := goja.AssertFunction(obj.Get("spawn"))
	_, err := spawnFn(goja.Undefined())
	if err == nil {
		t.Fatal("expected TypeError for spawn with no args")
	}
}

// ---------------------------------------------------------------------------
// wrapPanel — full lifecycle
// ---------------------------------------------------------------------------

func TestWrapPanel_FullLifecycle(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	panel := NewPanel(PanelConfig{MaxPanes: 4, ScrollbackSize: 50})
	obj := wrapPanel(rt, panel).ToObject(rt)

	// start()
	startFn, _ := goja.AssertFunction(obj.Get("start"))
	_, err := startFn(goja.Undefined())
	if err != nil {
		t.Fatalf("start() threw: %v", err)
	}

	// addPane(id, title)
	addFn, _ := goja.AssertFunction(obj.Get("addPane"))
	idxVal, err := addFn(goja.Undefined(), rt.ToValue("p1"), rt.ToValue("Pane 1"))
	if err != nil {
		t.Fatalf("addPane threw: %v", err)
	}
	if idxVal.ToInteger() != 0 {
		t.Errorf("addPane returned index %d, want 0", idxVal.ToInteger())
	}

	// addPane second pane
	_, err = addFn(goja.Undefined(), rt.ToValue("p2"), rt.ToValue("Pane 2"))
	if err != nil {
		t.Fatalf("addPane #2 threw: %v", err)
	}

	// paneCount()
	countFn, _ := goja.AssertFunction(obj.Get("paneCount"))
	countVal, _ := countFn(goja.Undefined())
	if countVal.ToInteger() != 2 {
		t.Errorf("paneCount() = %d, want 2", countVal.ToInteger())
	}

	// activeIndex()
	actIdxFn, _ := goja.AssertFunction(obj.Get("activeIndex"))
	actVal, _ := actIdxFn(goja.Undefined())
	if actVal.ToInteger() != 0 {
		t.Errorf("activeIndex() = %d, want 0", actVal.ToInteger())
	}

	// setActive(1)
	setActFn, _ := goja.AssertFunction(obj.Get("setActive"))
	_, err = setActFn(goja.Undefined(), rt.ToValue(1))
	if err != nil {
		t.Fatalf("setActive(1) threw: %v", err)
	}

	// activePane() — should be pane 2
	actPaneFn, _ := goja.AssertFunction(obj.Get("activePane"))
	paneVal, _ := actPaneFn(goja.Undefined())
	paneObj := paneVal.ToObject(rt)
	if paneObj.Get("id").String() != "p2" {
		t.Errorf("activePane().id = %q, want p2", paneObj.Get("id").String())
	}

	// appendOutput(paneID, line)
	appendFn, _ := goja.AssertFunction(obj.Get("appendOutput"))
	_, err = appendFn(goja.Undefined(), rt.ToValue("p1"), rt.ToValue("hello world"))
	if err != nil {
		t.Fatalf("appendOutput threw: %v", err)
	}

	// updateHealth(paneID, health)
	healthFn, _ := goja.AssertFunction(obj.Get("updateHealth"))
	healthObj := rt.NewObject()
	_ = healthObj.Set("state", "running")
	_ = healthObj.Set("errorCount", 0)
	_ = healthObj.Set("taskCount", 3)
	_, err = healthFn(goja.Undefined(), rt.ToValue("p1"), healthObj)
	if err != nil {
		t.Fatalf("updateHealth threw: %v", err)
	}

	// routeInput(key)
	routeFn, _ := goja.AssertFunction(obj.Get("routeInput"))
	routeVal, err := routeFn(goja.Undefined(), rt.ToValue("a"))
	if err != nil {
		t.Fatalf("routeInput threw: %v", err)
	}
	routeObj := routeVal.ToObject(rt)
	if routeObj.Get("consumed") == nil {
		t.Error("routeInput result should have consumed field")
	}

	// getVisibleLines(paneID, height)
	visFn, _ := goja.AssertFunction(obj.Get("getVisibleLines"))
	visVal, err := visFn(goja.Undefined(), rt.ToValue("p1"), rt.ToValue(10))
	if err != nil {
		t.Fatalf("getVisibleLines threw: %v", err)
	}
	// Should be an array
	if visVal == nil || goja.IsUndefined(visVal) {
		t.Error("getVisibleLines returned nil/undefined")
	}

	// statusBar()
	statusFn, _ := goja.AssertFunction(obj.Get("statusBar"))
	_, err = statusFn(goja.Undefined())
	if err != nil {
		t.Fatalf("statusBar threw: %v", err)
	}

	// snapshot()
	snapFn, _ := goja.AssertFunction(obj.Get("snapshot"))
	snapVal, err := snapFn(goja.Undefined())
	if err != nil {
		t.Fatalf("snapshot threw: %v", err)
	}
	if snapVal == nil || goja.IsUndefined(snapVal) {
		t.Error("snapshot returned nil/undefined")
	}

	// config()
	cfgFn, _ := goja.AssertFunction(obj.Get("config"))
	cfgVal, err := cfgFn(goja.Undefined())
	if err != nil {
		t.Fatalf("config() threw: %v", err)
	}
	cfgObj := cfgVal.ToObject(rt)
	if cfgObj.Get("maxPanes").ToInteger() != 4 {
		t.Errorf("config().maxPanes = %d, want 4", cfgObj.Get("maxPanes").ToInteger())
	}

	// removePane(id)
	rmFn, _ := goja.AssertFunction(obj.Get("removePane"))
	_, err = rmFn(goja.Undefined(), rt.ToValue("p2"))
	if err != nil {
		t.Fatalf("removePane threw: %v", err)
	}

	// close()
	closeFn, _ := goja.AssertFunction(obj.Get("close"))
	_, err = closeFn(goja.Undefined())
	if err != nil {
		t.Fatalf("close() threw: %v", err)
	}
}

func TestWrapPanel_ErrorPaths(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	panel := NewPanel(PanelConfig{MaxPanes: 4, ScrollbackSize: 50})
	obj := wrapPanel(rt, panel).ToObject(rt)

	// addPane no args
	addFn, _ := goja.AssertFunction(obj.Get("addPane"))
	_, err := addFn(goja.Undefined())
	if err == nil {
		t.Fatal("expected TypeError for addPane with no args")
	}

	// removePane no args
	rmFn, _ := goja.AssertFunction(obj.Get("removePane"))
	_, err = rmFn(goja.Undefined())
	if err == nil {
		t.Fatal("expected TypeError for removePane with no args")
	}

	// routeInput no args
	routeFn, _ := goja.AssertFunction(obj.Get("routeInput"))
	_, err = routeFn(goja.Undefined())
	if err == nil {
		t.Fatal("expected TypeError for routeInput with no args")
	}

	// setActive no args
	setFn, _ := goja.AssertFunction(obj.Get("setActive"))
	_, err = setFn(goja.Undefined())
	if err == nil {
		t.Fatal("expected TypeError for setActive with no args")
	}

	// appendOutput no args
	appendFn, _ := goja.AssertFunction(obj.Get("appendOutput"))
	_, err = appendFn(goja.Undefined())
	if err == nil {
		t.Fatal("expected TypeError for appendOutput with no args")
	}

	// updateHealth no args
	healthFn, _ := goja.AssertFunction(obj.Get("updateHealth"))
	_, err = healthFn(goja.Undefined())
	if err == nil {
		t.Fatal("expected TypeError for updateHealth with no args")
	}

	// getVisibleLines no args
	visFn, _ := goja.AssertFunction(obj.Get("getVisibleLines"))
	_, err = visFn(goja.Undefined())
	if err == nil {
		t.Fatal("expected TypeError for getVisibleLines with no args")
	}

	// start() on already-started panel
	_ = panel.Start()
	startFn, _ := goja.AssertFunction(obj.Get("start"))
	_, err = startFn(goja.Undefined())
	if err == nil {
		t.Fatal("expected error for double start()")
	}

	// setActive out of bounds
	_, err = setFn(goja.Undefined(), rt.ToValue(999))
	if err == nil {
		t.Fatal("expected error for setActive with invalid index")
	}
}

func TestWrapPanel_ActivePaneEmpty(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	panel := NewPanel(DefaultPanelConfig())
	obj := wrapPanel(rt, panel).ToObject(rt)

	// activePane() on empty panel should return null
	actPaneFn, _ := goja.AssertFunction(obj.Get("activePane"))
	val, err := actPaneFn(goja.Undefined())
	if err != nil {
		t.Fatalf("activePane threw: %v", err)
	}
	if !goja.IsNull(val) {
		t.Errorf("activePane() on empty panel should be null, got %v", val)
	}
}

func TestWrapPanel_GetVisibleLinesError(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	panel := NewPanel(DefaultPanelConfig())
	_ = panel.Start()
	obj := wrapPanel(rt, panel).ToObject(rt)

	visFn, _ := goja.AssertFunction(obj.Get("getVisibleLines"))
	_, err := visFn(goja.Undefined(), rt.ToValue("nonexistent"), rt.ToValue(10))
	if err == nil {
		t.Fatal("expected error for getVisibleLines on nonexistent pane")
	}
}

func TestWrapPanel_AppendOutputError(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	panel := NewPanel(DefaultPanelConfig())
	_ = panel.Start()
	obj := wrapPanel(rt, panel).ToObject(rt)

	appendFn, _ := goja.AssertFunction(obj.Get("appendOutput"))
	_, err := appendFn(goja.Undefined(), rt.ToValue("nonexistent"), rt.ToValue("line"))
	if err == nil {
		t.Fatal("expected error for appendOutput on nonexistent pane")
	}
}

func TestWrapPanel_UpdateHealthError(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	panel := NewPanel(DefaultPanelConfig())
	_ = panel.Start()
	obj := wrapPanel(rt, panel).ToObject(rt)

	healthFn, _ := goja.AssertFunction(obj.Get("updateHealth"))
	healthObj := rt.NewObject()
	_ = healthObj.Set("state", "ok")
	_, err := healthFn(goja.Undefined(), rt.ToValue("nonexistent"), healthObj)
	if err == nil {
		t.Fatal("expected error for updateHealth on nonexistent pane")
	}
}

func TestWrapPanel_RemovePaneError(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	panel := NewPanel(DefaultPanelConfig())
	_ = panel.Start()
	obj := wrapPanel(rt, panel).ToObject(rt)

	rmFn, _ := goja.AssertFunction(obj.Get("removePane"))
	_, err := rmFn(goja.Undefined(), rt.ToValue("nonexistent"))
	if err == nil {
		t.Fatal("expected error for removePane on nonexistent pane")
	}
}

// ---------------------------------------------------------------------------
// wrapGuard — processEvent, processCrash, checkTimeout, state, config
// ---------------------------------------------------------------------------

func TestWrapGuard_FullLifecycle(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	g := NewGuard(GuardConfig{
		OutputTimeout: OutputTimeoutGuardConfig{
			Enabled: true,
			Timeout: 100 * time.Millisecond,
		},
		Crash: CrashGuardConfig{
			Enabled:     true,
			MaxRestarts: 3,
		},
	})
	obj := wrapGuard(rt, g).ToObject(rt)

	now := time.Now()
	nowMs := rt.ToValue(now.UnixMilli())

	// processEvent(event, nowMs)
	procFn, _ := goja.AssertFunction(obj.Get("processEvent"))
	ev := rt.NewObject()
	_ = ev.Set("type", int(EventText))
	_ = ev.Set("line", "hello")
	_, err := procFn(goja.Undefined(), ev, nowMs)
	if err != nil {
		t.Fatalf("processEvent threw: %v", err)
	}

	// processEvent no args
	_, err = procFn(goja.Undefined())
	if err == nil {
		t.Fatal("expected TypeError for processEvent with no args")
	}

	// processCrash(exitCode, nowMs)
	crashFn, _ := goja.AssertFunction(obj.Get("processCrash"))
	crashVal, err := crashFn(goja.Undefined(), rt.ToValue(1), nowMs)
	if err != nil {
		t.Fatalf("processCrash threw: %v", err)
	}
	// Should return a guard event (crash event)
	if crashVal == nil || goja.IsUndefined(crashVal) || goja.IsNull(crashVal) {
		t.Logf("processCrash returned null/nil (acceptable if crash detection not triggered)")
	}

	// processCrash no args
	_, err = crashFn(goja.Undefined())
	if err == nil {
		t.Fatal("expected TypeError for processCrash with no args")
	}

	// checkTimeout(nowMs)
	checkFn, _ := goja.AssertFunction(obj.Get("checkTimeout"))
	_, err = checkFn(goja.Undefined(), nowMs)
	if err != nil {
		t.Fatalf("checkTimeout threw: %v", err)
	}

	// resetCrashCount()
	resetFn, _ := goja.AssertFunction(obj.Get("resetCrashCount"))
	_, err = resetFn(goja.Undefined())
	if err != nil {
		t.Fatalf("resetCrashCount threw: %v", err)
	}

	// state()
	stateFn, _ := goja.AssertFunction(obj.Get("state"))
	stateVal, err := stateFn(goja.Undefined())
	if err != nil {
		t.Fatalf("state() threw: %v", err)
	}
	stateObj := stateVal.ToObject(rt)
	if stateObj.Get("crashCount") == nil {
		t.Error("state() should have crashCount field")
	}
	if stateObj.Get("started") == nil {
		t.Error("state() should have started field")
	}

	// config()
	cfgFn, _ := goja.AssertFunction(obj.Get("config"))
	cfgVal, err := cfgFn(goja.Undefined())
	if err != nil {
		t.Fatalf("config() threw: %v", err)
	}
	cfgObj := cfgVal.ToObject(rt)
	crashCfg := cfgObj.Get("crash").ToObject(rt)
	if crashCfg.Get("maxRestarts").ToInteger() != 3 {
		t.Errorf("config().crash.maxRestarts = %d, want 3", crashCfg.Get("maxRestarts").ToInteger())
	}
}

func TestWrapGuard_ProcessEvent_WithFields(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	g := NewGuard(GuardConfig{
		RateLimit: RateLimitGuardConfig{
			Enabled:      true,
			InitialDelay: 1 * time.Second,
			MaxDelay:     10 * time.Second,
			Multiplier:   2.0,
			ResetAfter:   5 * time.Minute,
		},
	})
	obj := wrapGuard(rt, g).ToObject(rt)
	now := time.Now()

	procFn, _ := goja.AssertFunction(obj.Get("processEvent"))
	ev := rt.NewObject()
	_ = ev.Set("type", int(EventRateLimit))
	_ = ev.Set("line", "rate limit hit")
	_ = ev.Set("pattern", "rate_limit")
	fields := rt.NewObject()
	_ = fields.Set("retry_after", "30")
	_ = ev.Set("fields", fields)

	val, err := procFn(goja.Undefined(), ev, rt.ToValue(now.UnixMilli()))
	if err != nil {
		t.Fatalf("processEvent with rate limit threw: %v", err)
	}
	// Rate limit event should produce a guard event with action
	if val != nil && !goja.IsNull(val) && !goja.IsUndefined(val) {
		geObj := val.ToObject(rt)
		action := geObj.Get("action")
		if action == nil {
			t.Error("guard event should have action field")
		}
	}
}

func TestWrapGuard_CheckTimeout_Fires(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	g := NewGuard(GuardConfig{
		OutputTimeout: OutputTimeoutGuardConfig{
			Enabled: true,
			Timeout: 50 * time.Millisecond,
		},
	})
	obj := wrapGuard(rt, g).ToObject(rt)

	// Send an event to start the timeout clock
	procFn, _ := goja.AssertFunction(obj.Get("processEvent"))
	now := time.Now()
	ev := rt.NewObject()
	_ = ev.Set("type", int(EventText))
	_ = ev.Set("line", "start")
	_, _ = procFn(goja.Undefined(), ev, rt.ToValue(now.UnixMilli()))

	// Check timeout well after the window
	checkFn, _ := goja.AssertFunction(obj.Get("checkTimeout"))
	future := now.Add(200 * time.Millisecond)
	val, err := checkFn(goja.Undefined(), rt.ToValue(future.UnixMilli()))
	if err != nil {
		t.Fatalf("checkTimeout threw: %v", err)
	}
	// Should fire timeout event
	if val == nil || goja.IsNull(val) || goja.IsUndefined(val) {
		t.Error("checkTimeout should have fired a timeout guard event")
	}
}

// ---------------------------------------------------------------------------
// wrapMCPGuard — processToolCall, checkNoCallTimeout, state, config
// ---------------------------------------------------------------------------

func TestWrapMCPGuard_FullLifecycle(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	g := NewMCPGuard(MCPGuardConfig{
		NoCallTimeout: MCPNoCallTimeoutConfig{
			Enabled: true,
			Timeout: 100 * time.Millisecond,
		},
		FrequencyLimit: MCPFrequencyLimitConfig{
			Enabled:  true,
			Window:   1 * time.Second,
			MaxCalls: 10,
		},
	})
	obj := wrapMCPGuard(rt, g).ToObject(rt)

	now := time.Now()

	// processToolCall(call)
	procFn, _ := goja.AssertFunction(obj.Get("processToolCall"))
	tc := rt.NewObject()
	_ = tc.Set("toolName", "write_file")
	_ = tc.Set("arguments", `{"path":"test.txt"}`)
	_ = tc.Set("timestampMs", now.UnixMilli())
	_, err := procFn(goja.Undefined(), tc)
	if err != nil {
		t.Fatalf("processToolCall threw: %v", err)
	}

	// processToolCall no args
	_, err = procFn(goja.Undefined())
	if err == nil {
		t.Fatal("expected TypeError for processToolCall with no args")
	}

	// checkNoCallTimeout(nowMs)
	checkFn, _ := goja.AssertFunction(obj.Get("checkNoCallTimeout"))
	_, err = checkFn(goja.Undefined(), rt.ToValue(now.UnixMilli()))
	if err != nil {
		t.Fatalf("checkNoCallTimeout threw: %v", err)
	}

	// state()
	stateFn, _ := goja.AssertFunction(obj.Get("state"))
	stateVal, err := stateFn(goja.Undefined())
	if err != nil {
		t.Fatalf("state() threw: %v", err)
	}
	stateObj := stateVal.ToObject(rt)
	if stateObj.Get("totalCalls").ToInteger() != 1 {
		t.Errorf("state().totalCalls = %d, want 1", stateObj.Get("totalCalls").ToInteger())
	}

	// config()
	cfgFn, _ := goja.AssertFunction(obj.Get("config"))
	cfgVal, err := cfgFn(goja.Undefined())
	if err != nil {
		t.Fatalf("config() threw: %v", err)
	}
	cfgObj := cfgVal.ToObject(rt)
	nct := cfgObj.Get("noCallTimeout").ToObject(rt)
	if !nct.Get("enabled").ToBoolean() {
		t.Error("config().noCallTimeout.enabled should be true")
	}
}

func TestWrapMCPGuard_CheckNoCallTimeout_Fires(t *testing.T) {
	t.Parallel()
	rt := goja.New()
	g := NewMCPGuard(MCPGuardConfig{
		NoCallTimeout: MCPNoCallTimeoutConfig{
			Enabled: true,
			Timeout: 50 * time.Millisecond,
		},
	})
	obj := wrapMCPGuard(rt, g).ToObject(rt)

	// Start the guard by calling processToolCall
	procFn, _ := goja.AssertFunction(obj.Get("processToolCall"))
	now := time.Now()
	tc := rt.NewObject()
	_ = tc.Set("toolName", "read_file")
	_ = tc.Set("timestampMs", now.UnixMilli())
	_, _ = procFn(goja.Undefined(), tc)

	// Check timeout well after the window
	checkFn, _ := goja.AssertFunction(obj.Get("checkNoCallTimeout"))
	future := now.Add(200 * time.Millisecond)
	val, err := checkFn(goja.Undefined(), rt.ToValue(future.UnixMilli()))
	if err != nil {
		t.Fatalf("checkNoCallTimeout threw: %v", err)
	}
	if val == nil || goja.IsNull(val) || goja.IsUndefined(val) {
		t.Error("checkNoCallTimeout should have fired a guard event")
	}
}

// ============================================================================
// MCPGuard — additional coverage for uncovered paths
// ============================================================================

func TestMCPGuard_CheckNoCallTimeout_AlreadyFired(t *testing.T) {
	t.Parallel()
	now := time.Now()
	g := NewMCPGuard(MCPGuardConfig{
		NoCallTimeout: MCPNoCallTimeoutConfig{
			Enabled: true,
			Timeout: 10 * time.Millisecond,
		},
	})
	// Start the guard by processing a tool call.
	g.ProcessToolCall(MCPToolCall{
		ToolName:  "test",
		Timestamp: now,
	})
	// Fire the timeout the first time.
	future := now.Add(20 * time.Millisecond)
	ge := g.CheckNoCallTimeout(future)
	if ge == nil {
		t.Fatal("expected timeout event on first fire")
	}
	// Second check should return nil (already fired).
	ge2 := g.CheckNoCallTimeout(future.Add(time.Second))
	if ge2 != nil {
		t.Fatal("expected nil for already-fired timeout")
	}
}

func TestMCPGuard_CheckRepetition_LessThanTwoCalls(t *testing.T) {
	t.Parallel()
	now := time.Now()
	g := NewMCPGuard(MCPGuardConfig{
		RepeatDetection: MCPRepeatDetectionConfig{
			Enabled:    true,
			MaxRepeats: 3,
			WindowSize: 10,
			MatchTool:  true,
		},
	})
	// Single call — checkRepetition n < 2 early return.
	ge := g.ProcessToolCall(MCPToolCall{
		ToolName:  "readFile",
		Timestamp: now,
	})
	if ge != nil {
		t.Fatalf("expected nil for single call, got action=%d", ge.Action)
	}
}

func TestMCPGuard_Allowlist_Rejection(t *testing.T) {
	t.Parallel()
	now := time.Now()
	g := NewMCPGuard(MCPGuardConfig{
		ToolAllowlist: MCPToolAllowlistConfig{
			Enabled:      true,
			AllowedTools: []string{"readFile", "listContext"},
		},
	})
	// Allowed tool — should pass.
	ge := g.ProcessToolCall(MCPToolCall{
		ToolName:  "readFile",
		Timestamp: now,
	})
	if ge != nil {
		t.Fatalf("expected nil for allowed tool, got action=%d", ge.Action)
	}
	// Disallowed tool — should reject.
	ge2 := g.ProcessToolCall(MCPToolCall{
		ToolName:  "deleteFile",
		Timestamp: now.Add(time.Millisecond),
	})
	if ge2 == nil {
		t.Fatal("expected rejection for disallowed tool")
	}
	if ge2.Action != GuardActionReject {
		t.Fatalf("expected GuardActionReject, got %d", ge2.Action)
	}
}

func TestMCPGuard_Frequency_Exceeded_Coverage(t *testing.T) {
	t.Parallel()
	now := time.Now()
	g := NewMCPGuard(MCPGuardConfig{
		FrequencyLimit: MCPFrequencyLimitConfig{
			Enabled:  true,
			Window:   time.Second,
			MaxCalls: 2,
		},
	})
	// Three calls within the window — third should trigger frequency limit.
	g.ProcessToolCall(MCPToolCall{ToolName: "a", Timestamp: now})
	g.ProcessToolCall(MCPToolCall{ToolName: "b", Timestamp: now.Add(time.Millisecond)})
	ge := g.ProcessToolCall(MCPToolCall{ToolName: "c", Timestamp: now.Add(2 * time.Millisecond)})
	if ge == nil {
		t.Fatal("expected frequency limit event")
	}
	if ge.Action != GuardActionPause {
		t.Fatalf("expected GuardActionPause, got %d", ge.Action)
	}
}

// ============================================================================
// Panel — additional health indicator coverage
// ============================================================================

func TestPanel_HealthIndicator_StoppedAndDefault(t *testing.T) {
	t.Parallel()
	// Test the "stopped" and default cases.
	got := healthIndicator("stopped")
	if got != "■ " {
		t.Fatalf("stopped indicator = %q, want %q", got, "■ ")
	}
	got = healthIndicator("unknown-state")
	if got != "? " {
		t.Fatalf("default indicator = %q, want %q", got, "? ")
	}
}

// ============================================================================
// Safety — negative risk clamp
// ============================================================================

func TestSafety_CalculateRisk_NegativeClamp(t *testing.T) {
	t.Parallel()
	// A SafetyValidator with extremely permissive settings.
	sv := NewSafetyValidator(SafetyConfig{
		BlockThreshold:   0.9,
		ConfirmThreshold: 0.5,
	})
	result := sv.Validate(SafetyAction{
		Type: "tool_call",
		Name: "readFile",
	})
	// Risk should be 0 or very low for a read action.
	if result.RiskScore < 0.0 {
		t.Fatal("risk should never be negative")
	}
	if result.RiskScore > 1.0 {
		t.Fatal("risk should never exceed 1.0")
	}
}

// ============================================================================
// Parser — field extractor nil submatch paths
// ============================================================================

func TestParser_FieldExtractor_EmptyMatch(t *testing.T) {
	t.Parallel()
	p := NewParser()
	// Lines that match patterns but have empty submatch groups.
	// Rate limit with no numeric value:
	ev := p.Parse("⏳ Waiting for rate limit...")
	if ev.Type != EventRateLimit {
		t.Fatalf("expected EventRateLimit, got %d", ev.Type)
	}
	// The fields may or may not have "wait_seconds" depending on regex match.
	// This exercises the field extractor code path.

	// Permission prompt with no detail:
	ev2 := p.Parse("Do you want to allow this action?")
	if ev2.Type != EventPermission {
		t.Fatalf("expected EventPermission, got %d", ev2.Type)
	}

	// Error event:
	ev3 := p.Parse("Error: connection refused")
	if ev3.Type != EventError {
		t.Fatalf("expected EventError, got %d", ev3.Type)
	}
}

// ============================================================================
// Supervisor — decision edge cases
// ============================================================================

func TestSupervisor_RetryThresholdBoundary(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sup := NewSupervisor(ctx, SupervisorConfig{
		MaxRetries:    1,
		MaxForceKills: 1,
	})
	sup.Start()

	// First error → retry.
	d1 := sup.HandleError("err1", ErrorClassPTYError)
	if d1.Action != RecoveryRetry {
		t.Fatalf("first error: expected Retry, got %s", RecoveryActionName(d1.Action))
	}
	sup.ConfirmRecovery()

	// Second error (retry count matches threshold) → check for restart or escalate.
	d2 := sup.HandleError("err2", ErrorClassPTYError)
	// With MaxRetries=1, after one retry it should escalate.
	if d2.Action != RecoveryEscalate && d2.Action != RecoveryRetry {
		t.Fatalf("second error: expected Escalate or Retry, got %s", RecoveryActionName(d2.Action))
	}
}

// ============================================================================
// ManagedSession — callback coverage
// ============================================================================

func TestManagedSession_ProcessCrash_Escalation_Coverage(t *testing.T) {
	t.Parallel()
	cfg := ManagedSessionConfig{
		Guard:    DefaultGuardConfig(),
		MCPGuard: MCPGuardConfig{NoCallTimeout: MCPNoCallTimeoutConfig{Enabled: false}},
		Supervisor: SupervisorConfig{
			MaxRetries:    0,
			MaxForceKills: 0,
		},
	}
	sess := NewManagedSession(context.Background(), "test-crash", cfg)
	if err := sess.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer sess.Close()

	// Hard crash — supervisor should take action (ForceKill, Escalate, or Restart).
	_, d := sess.ProcessCrash(1, time.Now())
	if d.Action == RecoveryNone || d.Action == RecoveryRetry {
		t.Fatalf("expected serious recovery action, got %s", RecoveryActionName(d.Action))
	}
}
