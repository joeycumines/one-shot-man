package termmux

import (
	"context"
	"fmt"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// testRequire sets up a goja.Runtime with the osm:termmux module registered
// (using nil TerminalOpsProvider so it falls back to os.Stdin/os.Stdout).
func testRequire(t *testing.T) (*goja.Runtime, *goja.Object) {
	t.Helper()
	runtime := goja.New()
	registry := require.NewRegistry()

	registry.RegisterNativeModule("osm:termmux", Require(context.Background(), nil, nil))
	registry.Enable(runtime)

	v, err := runtime.RunString(`require('osm:termmux')`)
	if err != nil {
		t.Fatalf("require osm:termmux: %v", err)
	}
	obj := v.(*goja.Object)
	return runtime, obj
}

func TestModule_Constants(t *testing.T) {
	_, exports := testRequire(t)

	tests := []struct {
		name string
		want any
	}{
		{"EXIT_TOGGLE", "toggle"},
		{"EXIT_CHILD_EXIT", "childExit"},
		{"EXIT_CONTEXT", "context"},
		{"EXIT_ERROR", "error"},
		{"SIDE_OSM", "osm"},
		{"SIDE_CLAUDE", "claude"},
		{"DEFAULT_TOGGLE_KEY", int64(0x1D)},
		// Event name constants (T08).
		{"EVENT_EXIT", "exit"},
		{"EVENT_RESIZE", "resize"},
		{"EVENT_FOCUS", "focus"},
		{"EVENT_BELL", "bell"},
		{"EVENT_OUTPUT", "output"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := exports.Get(tt.name).Export()
			if got != tt.want {
				t.Errorf("got %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestExitReasonString(t *testing.T) {
	tests := []struct {
		input parent.ExitReason
		want  string
	}{
		{parent.ExitToggle, "toggle"},
		{parent.ExitChildExit, "childExit"},
		{parent.ExitContext, "context"},
		{parent.ExitError, "error"},
		{parent.ExitReason(99), "unknown(99)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := exitReasonString(tt.input)
			if got != tt.want {
				t.Errorf("exitReasonString(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ── Event system tests (T08) ────────────────────────────

func TestMuxEvents_OnOff(t *testing.T) {
	e := newMuxEvents()
	runtime := goja.New()

	cb, _ := goja.AssertFunction(runtime.ToValue(func() {}))

	id1 := e.on("exit", cb)
	id2 := e.on("exit", cb)
	id3 := e.on("resize", cb)

	if id1 == id2 || id2 == id3 {
		t.Error("IDs should be unique")
	}
	if id1 <= 0 || id2 <= 0 || id3 <= 0 {
		t.Error("IDs should be positive")
	}

	if !e.off(id1) {
		t.Error("off(id1) should return true")
	}
	if e.off(id1) {
		t.Error("off(id1) second call should return false")
	}
	if !e.off(id2) {
		t.Error("off(id2) should return true")
	}
	if !e.off(id3) {
		t.Error("off(id3) should return true")
	}
}

func TestMuxEvents_Emit(t *testing.T) {
	e := newMuxEvents()
	runtime := goja.New()

	var received []string
	cb, _ := goja.AssertFunction(runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		data := call.Argument(0).Export()
		if m, ok := data.(map[string]any); ok {
			if side, ok := m["side"].(string); ok {
				received = append(received, side)
			}
		}
		return goja.Undefined()
	}))

	e.on("focus", cb)
	e.on("focus", cb) // Two listeners for same event.

	e.emit(runtime, "focus", map[string]any{"side": "claude"})

	if len(received) != 2 {
		t.Fatalf("expected 2 callbacks, got %d", len(received))
	}
	for i, s := range received {
		if s != "claude" {
			t.Errorf("received[%d] = %q, want %q", i, s, "claude")
		}
	}
}

func TestMuxEvents_EmitNoListeners(t *testing.T) {
	e := newMuxEvents()
	runtime := goja.New()

	// Should not panic with no listeners.
	e.emit(runtime, "exit", map[string]any{"reason": "toggle"})
}

func TestMuxEvents_EmitWrongEvent(t *testing.T) {
	e := newMuxEvents()
	runtime := goja.New()

	called := false
	cb, _ := goja.AssertFunction(runtime.ToValue(func() goja.Value {
		called = true
		return goja.Undefined()
	}))

	e.on("exit", cb)
	e.emit(runtime, "resize", map[string]any{"rows": 24, "cols": 80})

	if called {
		t.Error("exit listener should not fire for resize event")
	}
}

func TestMuxEvents_Queue_Drain(t *testing.T) {
	e := newMuxEvents()
	runtime := goja.New()

	var received []map[string]any
	cb, _ := goja.AssertFunction(runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		data := call.Argument(0).Export()
		if m, ok := data.(map[string]any); ok {
			received = append(received, m)
		}
		return goja.Undefined()
	}))

	e.on("resize", cb)

	// Queue 3 events.
	e.queue("resize", map[string]any{"rows": 24, "cols": 80})
	e.queue("resize", map[string]any{"rows": 50, "cols": 120})
	e.queue("resize", map[string]any{"rows": 30, "cols": 100})

	count := e.drain(runtime)
	if count != 3 {
		t.Errorf("drain returned %d, want 3", count)
	}
	if len(received) != 3 {
		t.Fatalf("received %d events, want 3", len(received))
	}
	// Verify first and last.
	if got := fmt.Sprintf("%v", received[0]["rows"]); got != "24" {
		t.Errorf("first event rows = %v, want 24", received[0]["rows"])
	}
	if got := fmt.Sprintf("%v", received[2]["rows"]); got != "30" {
		t.Errorf("third event rows = %v, want 30", received[2]["rows"])
	}
}

func TestMuxEvents_DrainEmpty(t *testing.T) {
	e := newMuxEvents()
	runtime := goja.New()

	count := e.drain(runtime)
	if count != 0 {
		t.Errorf("drain on empty = %d, want 0", count)
	}
}

func TestMuxEvents_QueueFull_Drops(t *testing.T) {
	e := newMuxEvents()

	// Fill the channel (capacity 64).
	for i := range 64 {
		e.queue("resize", map[string]any{"i": i})
	}

	// 65th should not block (dropped).
	e.queue("resize", map[string]any{"i": 64})

	// Drain should get exactly 64.
	runtime := goja.New()
	count := e.drain(runtime)
	if count != 64 {
		t.Errorf("drain after overflow = %d, want 64", count)
	}
}

func TestMuxEvents_OffDuringEmit(t *testing.T) {
	e := newMuxEvents()
	runtime := goja.New()

	// Listener that removes itself during callback. Should not panic.
	var selfID int
	cb, _ := goja.AssertFunction(runtime.ToValue(func() goja.Value {
		e.off(selfID)
		return goja.Undefined()
	}))
	selfID = e.on("exit", cb)

	// Should not deadlock or panic.
	e.emit(runtime, "exit", map[string]any{"reason": "toggle"})
}

func TestMuxEvents_EmitToJSCallback(t *testing.T) {
	runtime := goja.New()
	events := newMuxEvents()

	// Register a JS callback via on()
	called := false
	var receivedData map[string]any
	callback := func(fc goja.FunctionCall) goja.Value {
		called = true
		if len(fc.Arguments) > 0 {
			receivedData, _ = fc.Arguments[0].Export().(map[string]any)
		}
		return goja.Undefined()
	}
	fn, ok := goja.AssertFunction(runtime.ToValue(callback))
	if !ok {
		t.Fatal("failed to create callable")
	}

	events.on("focus", fn)
	events.emit(runtime, "focus", map[string]any{"side": "claude"})

	if !called {
		t.Error("callback not called")
	}
	if receivedData["side"] != "claude" {
		t.Errorf("side = %v, want claude", receivedData["side"])
	}
}

func TestMuxEvents_QueueDrainToCallback(t *testing.T) {
	runtime := goja.New()
	events := newMuxEvents()

	calls := 0
	callback := func(fc goja.FunctionCall) goja.Value {
		calls++
		return goja.Undefined()
	}
	fn, _ := goja.AssertFunction(runtime.ToValue(callback))
	events.on("resize", fn)

	// Queue 3 resize events from a "non-JS goroutine"
	events.queue("resize", map[string]any{"rows": 24, "cols": 80})
	events.queue("resize", map[string]any{"rows": 25, "cols": 100})
	events.queue("resize", map[string]any{"rows": 30, "cols": 120})

	// Drain — should deliver all 3
	count := events.drain(runtime)
	if count != 3 {
		t.Errorf("drain count = %d, want 3", count)
	}
	if calls != 3 {
		t.Errorf("callback called %d times, want 3", calls)
	}
}

func TestMuxEvents_MultipleListenersSameEvent(t *testing.T) {
	runtime := goja.New()
	events := newMuxEvents()

	calls1, calls2 := 0, 0
	fn1, _ := goja.AssertFunction(runtime.ToValue(func(goja.FunctionCall) goja.Value {
		calls1++
		return goja.Undefined()
	}))
	fn2, _ := goja.AssertFunction(runtime.ToValue(func(goja.FunctionCall) goja.Value {
		calls2++
		return goja.Undefined()
	}))

	events.on("exit", fn1)
	events.on("exit", fn2)

	events.emit(runtime, "exit", map[string]any{"reason": "toggle"})

	if calls1 != 1 || calls2 != 1 {
		t.Errorf("calls = (%d, %d), want (1, 1)", calls1, calls2)
	}
}

func TestMuxEvents_OffRemovesOnlyTarget(t *testing.T) {
	runtime := goja.New()
	events := newMuxEvents()

	calls1, calls2 := 0, 0
	fn1, _ := goja.AssertFunction(runtime.ToValue(func(goja.FunctionCall) goja.Value {
		calls1++
		return goja.Undefined()
	}))
	fn2, _ := goja.AssertFunction(runtime.ToValue(func(goja.FunctionCall) goja.Value {
		calls2++
		return goja.Undefined()
	}))

	id1 := events.on("exit", fn1)
	events.on("exit", fn2)

	// Remove first, emit — only second should fire
	events.off(id1)
	events.emit(runtime, "exit", map[string]any{"reason": "toggle"})

	if calls1 != 0 {
		t.Errorf("removed listener called %d times", calls1)
	}
	if calls2 != 1 {
		t.Errorf("remaining listener called %d times, want 1", calls2)
	}
}

// Concurrent event system tests

func TestMuxEvents_ConcurrentOnOff(t *testing.T) {
	runtime := goja.New()
	events := newMuxEvents()

	fn, _ := goja.AssertFunction(runtime.ToValue(func(goja.FunctionCall) goja.Value {
		return goja.Undefined()
	}))

	// Register and remove many listeners concurrently
	// Note: on() and off() use sync.Mutex — safe across goroutines.
	// But emit/drain MUST only be called from the JS goroutine.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 100 {
			id := events.on("resize", fn)
			events.off(id)
		}
	}()

	// Concurrently add/remove from main goroutine
	for range 100 {
		id := events.on("resize", fn)
		events.off(id)
	}
	<-done
}

func TestMuxEvents_ConcurrentQueue(t *testing.T) {
	events := newMuxEvents()

	// Queue events from multiple goroutines (queue is thread-safe via channel)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 50 {
			events.queue("resize", map[string]any{"rows": i})
		}
	}()
	for range 50 {
		events.queue("focus", map[string]any{"side": "osm"})
	}
	<-done

	// Drain from JS goroutine
	runtime := goja.New()
	calls := 0
	fn, _ := goja.AssertFunction(runtime.ToValue(func(goja.FunctionCall) goja.Value {
		calls++
		return goja.Undefined()
	}))
	events.on("resize", fn)
	events.on("focus", fn)

	count := events.drain(runtime)
	// At least some events should be delivered (exact count depends on ordering)
	if count == 0 {
		t.Error("expected at least some events from concurrent queuing")
	}
	if count > 100 {
		t.Errorf("count %d exceeds expected max 100", count)
	}
}

func TestMuxEvents_BellQueueDrain(t *testing.T) {
	// Unit test: bell events can be queued and drained through the event system.
	e := newMuxEvents()
	runtime := goja.New()

	var received []map[string]any
	cb, _ := goja.AssertFunction(runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		data := call.Argument(0).Export()
		if m, ok := data.(map[string]any); ok {
			received = append(received, m)
		}
		return goja.Undefined()
	}))

	e.on("bell", cb)

	// Queue 3 bell events.
	e.queue("bell", map[string]any{"pane": "claude"})
	e.queue("bell", map[string]any{"pane": "claude"})
	e.queue("bell", map[string]any{"pane": "claude"})

	count := e.drain(runtime)
	if count != 3 {
		t.Errorf("drain returned %d, want 3", count)
	}
	if len(received) != 3 {
		t.Fatalf("received %d bell events, want 3", len(received))
	}
	if got := received[0]["pane"]; got != "claude" {
		t.Errorf("bell event pane = %v, want claude", got)
	}
}

// ── SessionManager session() wrapper tests ──────────────

// recordingStringIO is a test double for [parent.StringIO] that records
// all Send() calls for verification. Receive() blocks until Close().
type recordingStringIO struct {
	sent   []string
	closed chan struct{}
}

func newRecordingStringIO() *recordingStringIO {
	return &recordingStringIO{closed: make(chan struct{})}
}

func (r *recordingStringIO) Send(input string) error {
	r.sent = append(r.sent, input)
	return nil
}

func (r *recordingStringIO) Receive() (string, error) {
	<-r.closed
	return "", fmt.Errorf("closed")
}

func (r *recordingStringIO) Close() error {
	select {
	case <-r.closed:
	default:
		close(r.closed)
	}
	return nil
}

// TestSessionWrapper_WriteResize verifies that the session() convenience
// wrapper on the WrapSessionManager object exposes callable write() and
// resize() methods that correctly delegate to SessionManager.Input() and
// SessionManager.Resize() respectively.
//
// This is the core regression test for GAP-C01/C02 from the pr-split
// autopsy: the session() wrapper was missing write/resize methods,
// causing all inline Claude interactivity to silently fail.
func TestSessionWrapper_WriteResize(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	// 1. Create and start a SessionManager.
	mgr := parent.NewSessionManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	// 2. Create a recording StringIOSession and register it.
	rec := newRecordingStringIO()
	sio := parent.NewStringIOSession(rec)
	sio.Start()
	id, err := mgr.Register(sio, parent.SessionTarget{Name: "test", Kind: "pty"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if id == 0 {
		t.Fatal("Register returned zero SessionID")
	}

	// 3. Wrap the SessionManager for JS access.
	runtime := goja.New()
	tuiMux := WrapSessionManager(ctx, runtime, mgr, nil, nil, -1)
	_ = runtime.Set("tuiMux", tuiMux)

	// 4. Verify session().write exists and is callable.
	v, err := runtime.RunString(`typeof tuiMux.session().write`)
	if err != nil {
		t.Fatalf("typeof session().write: %v", err)
	}
	if v.String() != "function" {
		t.Fatalf("session().write type = %q, want 'function'", v.String())
	}

	// 5. Verify session().resize exists and is callable.
	v, err = runtime.RunString(`typeof tuiMux.session().resize`)
	if err != nil {
		t.Fatalf("typeof session().resize: %v", err)
	}
	if v.String() != "function" {
		t.Fatalf("session().resize type = %q, want 'function'", v.String())
	}

	// 6. Call session().write('hello') — bytes should reach the StringIOSession.
	_, err = runtime.RunString(`tuiMux.session().write('hello')`)
	if err != nil {
		t.Fatalf("session().write('hello'): %v", err)
	}
	if len(rec.sent) != 1 || rec.sent[0] != "hello" {
		t.Errorf("expected sent=['hello'], got %v", rec.sent)
	}

	// 7. Call session().resize(40, 120) — should not error.
	_, err = runtime.RunString(`tuiMux.session().resize(40, 120)`)
	if err != nil {
		t.Fatalf("session().resize(40, 120): %v", err)
	}

	// 8. Verify all other session() methods still work.
	v, err = runtime.RunString(`
		var s = tuiMux.session();
		var methods = ['isRunning', 'isDone', 'output', 'screen', 'target', 'setTarget', 'write', 'resize'];
		var missing = [];
		for (var i = 0; i < methods.length; i++) {
			if (typeof s[methods[i]] !== 'function') {
				missing.push(methods[i] + ':' + typeof s[methods[i]]);
			}
		}
		JSON.stringify(missing);
	`)
	if err != nil {
		t.Fatalf("method presence check: %v", err)
	}
	if v.String() != "[]" {
		t.Errorf("missing methods on session(): %s", v.String())
	}

	// Cleanup.
	cancel()
	<-errCh
}

// ── Input encoding binding tests ────────────────────────

func TestModule_KeyToTermBytes(t *testing.T) {
	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	tests := []struct {
		name string
		js   string
		want string
	}{
		{"enter", `tm.keyToTermBytes('enter')`, "\r"},
		{"tab", `tm.keyToTermBytes('tab')`, "\t"},
		{"esc", `tm.keyToTermBytes('esc')`, "\x1b"},
		{"up", `tm.keyToTermBytes('up')`, "\x1b[A"},
		{"ctrl+c", `tm.keyToTermBytes('ctrl+c')`, "\x03"},
		{"shift+tab", `tm.keyToTermBytes('shift+tab')`, "\x1b[Z"},
		{"single_char", `tm.keyToTermBytes('a')`, "a"},
		{"f5", `tm.keyToTermBytes('f5')`, "\x1b[15~"},
		{"shift+up", `tm.keyToTermBytes('shift+up')`, "\x1b[1;2A"},
		{"alt+a", `tm.keyToTermBytes('alt+a')`, "\x1ba"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.js)
			if err != nil {
				t.Fatalf("RunString: %v", err)
			}
			if v.String() != tt.want {
				t.Errorf("got %q, want %q", v.String(), tt.want)
			}
		})
	}
}

func TestModule_KeyToTermBytes_Null(t *testing.T) {
	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	v, err := runtime.RunString(`tm.keyToTermBytes('ctrl+shift+alt+x')`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !goja.IsNull(v) {
		t.Errorf("expected null for unknown key combo, got %v", v)
	}
}

func TestModule_MouseToSGR(t *testing.T) {
	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// Basic left click at (10, 5) → SGR: ESC[<0;11;6M
	v, err := runtime.RunString(`
		tm.mouseToSGR({type: 'MouseClick', button: 'left', x: 10, y: 5})
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := "\x1b[<0;11;6M"
	if v.String() != want {
		t.Errorf("got %q, want %q", v.String(), want)
	}
}

func TestModule_MouseToSGR_WithOffset(t *testing.T) {
	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	v, err := runtime.RunString(`
		tm.mouseToSGR({type: 'MouseClick', button: 'left', x: 15, y: 20}, 10, 5)
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := "\x1b[<0;11;11M"
	if v.String() != want {
		t.Errorf("got %q, want %q", v.String(), want)
	}
}

func TestModule_MouseToSGR_Modifiers(t *testing.T) {
	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	v, err := runtime.RunString(`
		tm.mouseToSGR({type: 'MouseClick', button: 'left', x: 0, y: 0, shift: true, ctrl: true})
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	// 0 + 4(shift) + 16(ctrl) = 20
	want := "\x1b[<20;1;1M"
	if v.String() != want {
		t.Errorf("got %q, want %q", v.String(), want)
	}
}

func TestModule_MouseToSGR_Release(t *testing.T) {
	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	v, err := runtime.RunString(`
		tm.mouseToSGR({type: 'MouseRelease', button: 'left', x: 5, y: 3})
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := "\x1b[<0;6;4m"
	if v.String() != want {
		t.Errorf("got %q, want %q", v.String(), want)
	}
}

func TestModule_MouseToSGR_Null(t *testing.T) {
	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// Negative coordinate after offset → null.
	v, err := runtime.RunString(`
		tm.mouseToSGR({type: 'MouseClick', button: 'left', x: 3, y: 2}, 10, 0)
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !goja.IsNull(v) {
		t.Errorf("expected null for negative offset, got %v", v)
	}
}

func TestModule_SplitLayout(t *testing.T) {
	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	v, err := runtime.RunString(`
		var layout = tm.splitLayout({
			totalChromeRows: 8,
			topPaneHeaderRows: 2,
			dividerRows: 1,
			bottomPaneHeaderRows: 2,
			leftChromeCol: 1,
			minPaneRows: 3
		});
		var result = layout.compute(40, 80, 0.6);
		JSON.stringify({
			topRow: result.top.row,
			topRows: result.top.rows,
			bottomRow: result.bottom.row,
			bottomRows: result.bottom.rows,
			bottomCol: result.bottom.col
		});
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	// vpHeight=32, topH=19, bottomContentRow=2+19+1+2=24, bottomContentH=32-19-1=12
	want := `{"topRow":2,"topRows":19,"bottomRow":24,"bottomRows":12,"bottomCol":1}`
	if v.String() != want {
		t.Errorf("got %s, want %s", v.String(), want)
	}
}

func TestModule_SplitLayout_OffsetMouse(t *testing.T) {
	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// Inside bottom pane.
	v, err := runtime.RunString(`
		var layout = tm.splitLayout({
			totalChromeRows: 8, topPaneHeaderRows: 2,
			dividerRows: 1, bottomPaneHeaderRows: 2,
			leftChromeCol: 1, minPaneRows: 3
		});
		var result = layout.compute(40, 80, 0.6);
		var hit = result.bottom.offsetMouse(26, 5);
		JSON.stringify(hit);
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	// bottom starts at row=24, col=1 → local = (26-24, 5-1) = (2, 4)
	want := `{"row":2,"col":4}`
	if v.String() != want {
		t.Errorf("inside: got %s, want %s", v.String(), want)
	}

	// Outside bottom pane → null.
	v, err = runtime.RunString(`
		var layout = tm.splitLayout({
			totalChromeRows: 8, topPaneHeaderRows: 2,
			dividerRows: 1, bottomPaneHeaderRows: 2,
			leftChromeCol: 1, minPaneRows: 3
		});
		var result = layout.compute(40, 80, 0.6);
		result.bottom.offsetMouse(0, 0);
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !goja.IsNull(v) {
		t.Errorf("outside: expected null, got %v", v)
	}
}
