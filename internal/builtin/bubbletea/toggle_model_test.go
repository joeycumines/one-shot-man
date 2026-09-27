package bubbletea

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/joeycumines/goja"
)

// TestToggleModel_Init delegates to inner model.
func TestToggleModel_Init(t *testing.T) {
	inner := &stubModel{initCmd: tea.ClearScreen}
	tm := &toggleModel{inner: inner, toggleKey: 0x1D}
	cmd := tm.Init()
	if cmd == nil {
		t.Fatal("expected Init to delegate to inner and return a cmd")
	}
}

// TestToggleModel_View delegates to inner model.
func TestToggleModel_View(t *testing.T) {
	inner := &stubModel{view: "hello toggle"}
	tm := &toggleModel{inner: inner, toggleKey: 0x1D}
	got := tm.View()
	if got.Content != "hello toggle" {
		t.Errorf("View().Content = %q, want %q", got.Content, "hello toggle")
	}
}

// TestToggleModel_Update_NonToggle passes through to inner model.
func TestToggleModel_Update_NonToggle(t *testing.T) {
	inner := &stubModel{view: "v1"}
	tm := &toggleModel{inner: inner, toggleKey: 0x1D}

	// Send a regular 'q' key — should NOT trigger toggle
	keyMsg := tea.KeyPressMsg{Text: "q"}
	model, cmd := tm.Update(keyMsg)
	if model != tm {
		t.Error("expected Update to return same toggleModel")
	}
	if cmd != nil {
		t.Error("expected nil cmd for non-toggle key")
	}
	if inner.updateCount != 1 {
		t.Errorf("inner.Update called %d times, want 1", inner.updateCount)
	}
}

// TestToggleModel_Update_ToggleByRune detects toggle key via rune match.
func TestToggleModel_Update_ToggleByRune(t *testing.T) {
	inner := &stubModel{}
	tm := &toggleModel{inner: inner, toggleKey: 0x1D}

	// Send a KeyPressMsg with rune 0x1D (Ctrl+])
	keyMsg := tea.KeyPressMsg{Code: 0x1D}
	model, cmd := tm.Update(keyMsg)
	if model != tm {
		t.Error("expected Update to return same toggleModel")
	}
	if cmd == nil {
		t.Error("expected a cmd for toggle key (rune match)")
	}
	if inner.updateCount != 0 {
		t.Error("inner.Update should NOT be called for toggle key")
	}
}

// TestToggleModel_Update_ToggleByCtrlCloseBracket detects Ctrl+] via KeyType.
func TestToggleModel_Update_ToggleByCtrlCloseBracket(t *testing.T) {
	inner := &stubModel{}
	tm := &toggleModel{inner: inner, toggleKey: 0x1D}

	// Send Ctrl+] — the toggle key
	keyMsg := tea.KeyPressMsg{Code: ']', Mod: tea.ModCtrl}
	model, cmd := tm.Update(keyMsg)
	if model != tm {
		t.Error("expected Update to return same toggleModel")
	}
	if cmd == nil {
		t.Error("expected a cmd for toggle key (KeyCtrlCloseBracket)")
	}
	if inner.updateCount != 0 {
		t.Error("inner.Update should NOT be called for toggle key")
	}
}

// TestToggleModel_Update_DifferentToggleKey uses a non-default toggle key.
func TestToggleModel_Update_DifferentToggleKey(t *testing.T) {
	inner := &stubModel{}
	// Use Ctrl+A (0x01) as toggle key
	tm := &toggleModel{inner: inner, toggleKey: 0x01}

	// Ctrl+] should NOT trigger toggle
	keyMsg := tea.KeyPressMsg{Code: ']', Mod: tea.ModCtrl}
	_, cmd := tm.Update(keyMsg)
	if cmd != nil {
		t.Error("Ctrl+] should not trigger with toggleKey=0x01")
	}

	// Rune 0x01 should trigger toggle
	keyMsg = tea.KeyPressMsg{Code: 0x01}
	_, cmd = tm.Update(keyMsg)
	if cmd == nil {
		t.Error("expected toggle for rune 0x01")
	}
}

// TestToggleModel_Update_NonKeyMsg passes through non-key messages.
func TestToggleModel_Update_NonKeyMsg(t *testing.T) {
	inner := &stubModel{}
	tm := &toggleModel{inner: inner, toggleKey: 0x1D}

	// WindowSize message should pass through
	msg := tea.WindowSizeMsg{Width: 80, Height: 24}
	_, cmd := tm.Update(msg)
	if cmd != nil {
		t.Error("expected nil cmd for non-key message")
	}
	if inner.updateCount != 1 {
		t.Errorf("inner.Update should be called for non-key msg, got %d", inner.updateCount)
	}
}

// TestToggleModel_ToggleCmd_WritesEscapes verifies escape sequences are written.
func TestToggleModel_ToggleCmd_WritesEscapes(t *testing.T) {
	var buf bytes.Buffer
	runtime := goja.New()

	called := false
	onToggle := func(goja.FunctionCall) goja.Value {
		called = true
		return goja.Undefined()
	}
	fn, ok := goja.AssertFunction(runtime.ToValue(onToggle))
	if !ok {
		t.Fatal("failed to create callable")
	}

	tm := &toggleModel{
		inner:     &stubModel{},
		toggleKey: 0x1D,
		onToggle:  fn,
		jsRunner:  &directJSRunner{runtime: runtime},
		output:    &buf,
	}

	cmd := tm.toggleCmd()
	if cmd == nil {
		t.Fatal("toggleCmd returned nil")
	}

	msg := cmd()
	if msg == nil {
		t.Fatal("expected toggleReturnMsg, got nil")
	}
	trm, ok := msg.(toggleReturnMsg)
	if !ok {
		t.Errorf("expected toggleReturnMsg, got %T", msg)
	}
	// Result should be nil since onToggle returns undefined
	if trm.Result != nil {
		t.Errorf("expected nil Result, got %v", trm.Result)
	}

	if !called {
		t.Error("onToggle callback was not called")
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("\x1b[?1049l")) {
		t.Error("missing alt-screen exit sequence")
	}
	if !bytes.Contains([]byte(output), []byte("\x1b[?1049h")) {
		t.Error("missing alt-screen enter sequence")
	}
}

// TestToggleModel_ToggleCmd_NilOutput handles nil output gracefully.
func TestToggleModel_ToggleCmd_NilOutput(t *testing.T) {
	runtime := goja.New()
	onToggle := func(goja.FunctionCall) goja.Value { return goja.Undefined() }
	fn, _ := goja.AssertFunction(runtime.ToValue(onToggle))

	tm := &toggleModel{
		inner:     &stubModel{},
		toggleKey: 0x1D,
		onToggle:  fn,
		jsRunner:  &directJSRunner{runtime: runtime},
		output:    nil, // No output writer
	}

	cmd := tm.toggleCmd()
	msg := cmd() // Should not panic
	if _, ok := msg.(toggleReturnMsg); !ok {
		t.Errorf("expected toggleReturnMsg, got %T", msg)
	}
}

// TestToggleModel_ToggleCmd_NilJSRunner handles nil jsRunner gracefully.
func TestToggleModel_ToggleCmd_NilJSRunner(t *testing.T) {
	var buf bytes.Buffer
	tm := &toggleModel{
		inner:     &stubModel{},
		toggleKey: 0x1D,
		jsRunner:  nil, // No JS runner
		output:    &buf,
	}

	cmd := tm.toggleCmd()
	msg := cmd() // Should not panic
	if _, ok := msg.(toggleReturnMsg); !ok {
		t.Errorf("expected toggleReturnMsg, got %T", msg)
	}
	// Escape sequences should still be written
	if !bytes.Contains(buf.Bytes(), []byte("\x1b[?1049l")) {
		t.Error("missing alt-screen exit even without jsRunner")
	}
}

// TestToggleModel_ProgramRef verifies program reference is set and cleaned up.
func TestToggleModel_ProgramRef(t *testing.T) {
	tm := &toggleModel{
		inner:     &stubModel{},
		toggleKey: 0x1D,
	}

	// Initially nil
	tm.mu.Lock()
	if tm.program != nil {
		t.Error("expected nil program initially")
	}
	tm.mu.Unlock()

	// After setting
	p := &tea.Program{}
	tm.mu.Lock()
	tm.program = p
	tm.mu.Unlock()

	tm.mu.Lock()
	got := tm.program
	tm.mu.Unlock()
	if got != p {
		t.Error("expected program to be set")
	}
}

// TestToggleReturnMsg_MsgToJS verifies the ToggleReturn message type appears in JS.
func TestToggleReturnMsg_MsgToJS(t *testing.T) {
	runtime := goja.New()
	m := &jsModel{runtime: runtime}
	result := m.msgToJS(toggleReturnMsg{})
	if result == nil {
		t.Fatal("msgToJS returned nil for toggleReturnMsg")
	}
	if result["type"] != "ToggleReturn" {
		t.Errorf("type = %v, want ToggleReturn", result["type"])
	}
}

// TestToggleModel_ToggleCmd_ReturnValue verifies the onToggle return value is captured.
func TestToggleModel_ToggleCmd_ReturnValue(t *testing.T) {
	var buf bytes.Buffer
	runtime := goja.New()

	// onToggle returns a map with reason and error (like switchTo does)
	onToggle := func(goja.FunctionCall) goja.Value {
		return runtime.ToValue(map[string]any{
			"reason": "toggle",
			"extra":  42,
		})
	}
	fn, ok := goja.AssertFunction(runtime.ToValue(onToggle))
	if !ok {
		t.Fatal("failed to create callable")
	}

	tm := &toggleModel{
		inner:     &stubModel{},
		toggleKey: 0x1D,
		onToggle:  fn,
		jsRunner:  &directJSRunner{runtime: runtime},
		output:    &buf,
	}

	cmd := tm.toggleCmd()
	msg := cmd()
	trm, ok := msg.(toggleReturnMsg)
	if !ok {
		t.Fatalf("expected toggleReturnMsg, got %T", msg)
	}
	if trm.Result == nil {
		t.Fatal("expected non-nil Result")
	}
	if trm.Result["reason"] != "toggle" {
		t.Errorf("Result[reason] = %v, want toggle", trm.Result["reason"])
	}
}

// TestToggleReturnMsg_MsgToJS_WithResult verifies Result fields are merged.
func TestToggleReturnMsg_MsgToJS_WithResult(t *testing.T) {
	runtime := goja.New()
	m := &jsModel{runtime: runtime}
	msg := toggleReturnMsg{Result: map[string]any{
		"reason": "childExit",
		"error":  "something failed",
	}}
	result := m.msgToJS(msg)
	if result == nil {
		t.Fatal("msgToJS returned nil")
	}
	if result["type"] != "ToggleReturn" {
		t.Errorf("type = %v, want ToggleReturn", result["type"])
	}
	if result["reason"] != "childExit" {
		t.Errorf("reason = %v, want childExit", result["reason"])
	}
	if result["error"] != "something failed" {
		t.Errorf("error = %v, want 'something failed'", result["error"])
	}
}

// TestToggleReturnMsg_MsgToJS_NilResult verifies nil Result produces clean output.
func TestToggleReturnMsg_MsgToJS_NilResult(t *testing.T) {
	runtime := goja.New()
	m := &jsModel{runtime: runtime}
	msg := toggleReturnMsg{Result: nil}
	result := m.msgToJS(msg)
	if result == nil {
		t.Fatal("msgToJS returned nil")
	}
	if result["type"] != "ToggleReturn" {
		t.Errorf("type = %v, want ToggleReturn", result["type"])
	}
	// Should have ONLY the type field
	if len(result) != 1 {
		t.Errorf("expected 1 field (type), got %d: %v", len(result), result)
	}
}

func TestToggleModel_ToggleCmd_CallsCallbackWithUndefinedThisAndNoArguments(t *testing.T) {
	t.Parallel()

	vm := goja.New()
	runner := newPromiseRunner(vm)
	t.Cleanup(runner.close)
	calls := make(chan struct {
		thisUndefined bool
		argumentCount int
	}, 1)

	onToggle := func(call goja.FunctionCall) goja.Value {
		calls <- struct {
			thisUndefined bool
			argumentCount int
		}{
			thisUndefined: goja.IsUndefined(call.This),
			argumentCount: len(call.Arguments),
		}
		return goja.Undefined()
	}
	fn, ok := goja.AssertFunction(vm.ToValue(onToggle))
	if !ok {
		t.Fatal("failed to create onToggle callable")
	}

	tm := &toggleModel{
		inner:     &stubModel{},
		toggleKey: 0x1D,
		onToggle:  fn,
		jsRunner:  runner,
		ctx:       context.Background(),
	}
	if msg := tm.toggleCmd()(); msg == nil {
		t.Fatal("toggleCmd returned nil")
	}

	select {
	case call := <-calls:
		if !call.thisUndefined {
			t.Error("onToggle this value was not undefined")
		}
		if call.argumentCount != 0 {
			t.Errorf("onToggle argument count = %d, want 0", call.argumentCount)
		}
	case <-time.After(time.Second):
		t.Fatal("onToggle callback was not invoked")
	}
}

func TestToggleModel_ToggleCmd_WaitsForPromiseBeforeRestoringTerminal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	tm, runner, callbacks, registered, output := newPendingToggle(t, ctx)
	result := make(chan tea.Msg, 1)
	go func() { result <- tm.toggleCmd()() }()

	select {
	case <-registered:
	case <-time.After(time.Second):
		t.Fatal("Promise then handler was not registered")
	}

	if got := output.snapshot(); len(got) != 1 || got[0] != "\x1b[?1049l" {
		t.Fatalf("writes before settlement = %q, want only alt-screen exit", got)
	}
	select {
	case msg := <-result:
		t.Fatalf("toggleCmd returned before Promise settlement: %T", msg)
	case <-time.After(100 * time.Millisecond):
	}

	var cb promiseCallbacks
	select {
	case cb = <-callbacks:
	case <-time.After(time.Second):
		t.Fatal("Promise callbacks were not captured")
	}
	if !runner.submit(func() {
		_, _ = cb.fulfill(goja.Undefined(), runner.runtime.ToValue(map[string]any{"reason": "async"}))
		_, _ = cb.fulfill(goja.Undefined(), runner.runtime.ToValue(map[string]any{"reason": "second"}))
		_, _ = cb.reject(goja.Undefined(), runner.runtime.ToValue("late rejection"))
	}) {
		t.Fatal("Promise runner stopped before fulfillment")
	}

	var msg tea.Msg
	select {
	case msg = <-result:
	case <-time.After(time.Second):
		t.Fatal("toggleCmd did not return after Promise settlement")
	}
	trm, ok := msg.(toggleReturnMsg)
	if !ok {
		t.Fatalf("message = %T, want toggleReturnMsg", msg)
	}
	if trm.Result["reason"] != "async" {
		t.Fatalf("toggle result = %#v, want first Promise result", trm.Result)
	}

	got := output.snapshot()
	want := []string{"\x1b[?1049l", "\x1b[?1049h\x1b[2J\x1b[H"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("writes = %q, want terminal exit then restore", got)
	}
}

func TestToggleModel_ToggleCmd_PropagatesContextAndCancelsPendingPromise(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	tm, runner, _, registered, output := newPendingToggle(t, ctx)
	result := make(chan tea.Msg, 1)
	go func() { result <- tm.toggleCmd()() }()

	select {
	case <-registered:
	case <-time.After(time.Second):
		t.Fatal("Promise then handler was not registered")
	}
	select {
	case got := <-runner.contexts:
		if got != ctx {
			t.Errorf("RunSync context = %v, want manager context %v", got, ctx)
		}
	case <-time.After(time.Second):
		t.Fatal("RunSync was not called")
	}

	cancel()
	select {
	case msg := <-result:
		if _, ok := msg.(toggleReturnMsg); !ok {
			t.Fatalf("message = %T, want toggleReturnMsg", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("toggleCmd did not stop after context cancellation")
	}
	got := output.snapshot()
	if len(got) != 2 || got[0] != "\x1b[?1049l" || got[1] != "\x1b[?1049h\x1b[2J\x1b[H" {
		t.Fatalf("writes after cancellation = %q, want terminal restore", got)
	}
}

// --- Helpers ---

type promiseCallbacks struct {
	fulfill goja.Callable
	reject  goja.Callable
}

type promiseRunner struct {
	runtime  *goja.Runtime
	jobs     chan func()
	contexts chan context.Context
	stop     chan struct{}
	stopped  chan struct{}
	stopOnce sync.Once
}

func newPromiseRunner(runtime *goja.Runtime) *promiseRunner {
	runner := &promiseRunner{
		runtime:  runtime,
		jobs:     make(chan func()),
		contexts: make(chan context.Context, 1),
		stop:     make(chan struct{}),
		stopped:  make(chan struct{}),
	}
	go func() {
		defer close(runner.stopped)
		for {
			select {
			case job := <-runner.jobs:
				job()
			case <-runner.stop:
				return
			}
		}
	}()
	return runner
}

func (r *promiseRunner) RunSync(ctx context.Context, fn func(*goja.Runtime) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case r.contexts <- ctx:
	default:
	}
	done := make(chan error, 1)
	select {
	case r.jobs <- func() { done <- fn(r.runtime) }:
	case <-ctx.Done():
		return ctx.Err()
	case <-r.stop:
		return context.Canceled
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-r.stop:
		return context.Canceled
	}
}

func (r *promiseRunner) submit(job func()) bool {
	select {
	case r.jobs <- job:
		return true
	case <-r.stop:
		return false
	}
}

func (r *promiseRunner) close() {
	r.stopOnce.Do(func() { close(r.stop) })
	<-r.stopped
}

func newPendingToggle(t *testing.T, ctx context.Context) (*toggleModel, *promiseRunner, <-chan promiseCallbacks, <-chan struct{}, *orderedWriter) {
	t.Helper()

	vm := goja.New()
	runner := newPromiseRunner(vm)
	t.Cleanup(runner.close)
	callbacks := make(chan promiseCallbacks, 1)
	registered := make(chan struct{})
	var registeredOnce sync.Once

	onToggle := func(goja.FunctionCall) goja.Value {
		obj := vm.NewObject()
		then := vm.ToValue(func(call goja.FunctionCall) goja.Value {
			fulfill, fulfillOK := goja.AssertFunction(call.Argument(0))
			reject, rejectOK := goja.AssertFunction(call.Argument(1))
			if !fulfillOK || !rejectOK {
				return goja.Undefined()
			}
			select {
			case callbacks <- promiseCallbacks{fulfill: fulfill, reject: reject}:
			default:
			}
			registeredOnce.Do(func() { close(registered) })
			return goja.Undefined()
		})
		_ = obj.Set("then", then)
		return obj
	}
	fn, ok := goja.AssertFunction(vm.ToValue(onToggle))
	if !ok {
		t.Fatal("failed to create onToggle callable")
	}

	output := &orderedWriter{}
	tm := &toggleModel{
		inner:     &stubModel{},
		toggleKey: 0x1D,
		onToggle:  fn,
		jsRunner:  runner,
		ctx:       ctx,
		output:    output,
	}
	return tm, runner, callbacks, registered, output
}

type orderedWriter struct {
	mu     sync.Mutex
	writes []string
}

func (w *orderedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.writes = append(w.writes, string(p))
	w.mu.Unlock()
	return len(p), nil
}

func (w *orderedWriter) snapshot() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.writes...)
}

// stubModel is a minimal tea.Model for testing toggleModel wrapping.
type stubModel struct {
	initCmd     tea.Cmd
	view        string
	updateCount int
}

func (m *stubModel) Init() tea.Cmd  { return m.initCmd }
func (m *stubModel) View() tea.View { return tea.NewView(m.view) }
func (m *stubModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.updateCount++
	return m, nil
}

// directJSRunner implements JSRunner by running the callback directly.
// Only safe for single-goroutine tests where there's no real event loop.
type directJSRunner struct {
	runtime *goja.Runtime
	mu      sync.Mutex
}

func (r *directJSRunner) RunSync(ctx context.Context, fn func(*goja.Runtime) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return fn(r.runtime)
}
