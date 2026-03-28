package termmux

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

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

func TestModule_NewMux_ReturnsObject(t *testing.T) {
	runtime, _ := testRequire(t)

	// newMux() with no args should return an object with all expected methods.
	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		var methods = ['attach', 'detach', 'hasChild', 'switchTo', 'activeSide',
			'setStatus', 'setToggleKey', 'setStatusEnabled', 'setResizeFunc', 'screenshot',
			'childScreen', 'writeToChild',
			'on', 'off', 'pollEvents', 'fromModel'];
		var missing = [];
		for (var i = 0; i < methods.length; i++) {
			if (typeof m[methods[i]] !== 'function') missing.push(methods[i]);
		}
		missing;
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}

	missing := v.Export()
	if arr, ok := missing.([]any); ok && len(arr) > 0 {
		t.Errorf("missing methods on mux: %v", arr)
	}
}

func TestModule_NewMux_WithOptions(t *testing.T) {
	runtime, _ := testRequire(t)

	// Should accept options without error.
	_, err := runtime.RunString(`
		var m = require('osm:termmux').newMux({
			toggleKey: 0x1C,
			statusEnabled: false,
			initialStatus: 'test'
		});
	`)
	if err != nil {
		t.Fatalf("newMux with options: %v", err)
	}
}

func TestModule_NewMux_HasChild_InitialFalse(t *testing.T) {
	runtime, _ := testRequire(t)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		m.hasChild();
	`)
	if err != nil {
		t.Fatalf("hasChild: %v", err)
	}
	if v.ToBoolean() {
		t.Error("hasChild() should be false before attach")
	}
}

func TestModule_NewMux_ActiveSide_InitialOsm(t *testing.T) {
	runtime, _ := testRequire(t)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		m.activeSide();
	`)
	if err != nil {
		t.Fatalf("activeSide: %v", err)
	}
	if v.String() != "osm" {
		t.Errorf("activeSide = %q, want %q", v.String(), "osm")
	}
}

func TestModule_NewMux_DetachIdempotent(t *testing.T) {
	runtime, _ := testRequire(t)

	// detach() on fresh mux should not panic.
	_, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		m.detach();
	`)
	if err != nil {
		t.Fatalf("detach on fresh mux should be idempotent: %v", err)
	}
}

func TestModule_NewMux_ScreenshotEmpty(t *testing.T) {
	runtime, _ := testRequire(t)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		m.screenshot();
	`)
	if err != nil {
		t.Fatalf("screenshot: %v", err)
	}
	if v.String() != "" {
		t.Errorf("screenshot = %q, want empty string", v.String())
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

func TestResolveChild_NilError(t *testing.T) {
	_, err := resolveChild(nil)
	if err == nil {
		t.Error("resolveChild(nil) should return error")
	}
}

func TestResolveChild_InvalidTypeError(t *testing.T) {
	_, err := resolveChild("not a handle")
	if err == nil {
		t.Error("resolveChild(string) should return error")
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

func TestModule_On_InvalidEvent(t *testing.T) {
	runtime, _ := testRequire(t)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		try {
			m.on('bogus', function() {});
			'no error';
		} catch(e) {
			e.message;
		}
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if v.String() == "no error" {
		t.Error("expected TypeError for unknown event name")
	}
}

func TestModule_On_NotAFunction(t *testing.T) {
	runtime, _ := testRequire(t)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		try {
			m.on('exit', 'not a function');
			'no error';
		} catch(e) {
			e.message;
		}
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	msg := v.String()
	if msg == "no error" {
		t.Error("expected TypeError for non-function callback")
	}
}

func TestModule_On_Off_Roundtrip(t *testing.T) {
	runtime, _ := testRequire(t)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		var id = m.on('exit', function() {});
		var removed = m.off(id);
		var removedAgain = m.off(id);
		JSON.stringify({ id: id, removed: removed, removedAgain: removedAgain });
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	got := v.String()
	want := `{"id":1,"removed":true,"removedAgain":false}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestModule_PollEvents_Empty(t *testing.T) {
	runtime, _ := testRequire(t)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		m.pollEvents();
	`)
	if err != nil {
		t.Fatalf("pollEvents: %v", err)
	}
	if v.ToInteger() != 0 {
		t.Errorf("pollEvents on fresh mux = %d, want 0", v.ToInteger())
	}
}

func TestModule_On_MissingArgs(t *testing.T) {
	runtime, _ := testRequire(t)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		try {
			m.on();
			'no error';
		} catch(e) {
			e.message;
		}
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if v.String() == "no error" {
		t.Error("expected TypeError for missing args")
	}
}

// --- T09: fromModel tests ---

func TestModule_FromModel_ReturnsStructure(t *testing.T) {
	runtime, _ := testRequire(t)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		var fakeModel = { _type: 'bubbleteaModel', _modelID: 42 };
		var result = m.fromModel(fakeModel);
		JSON.stringify({
			hasModel: result.model === fakeModel,
			hasOptions: typeof result.options === 'object',
			altScreen: result.options.altScreen,
			toggleKey: result.options.toggleKey,
			hasOnToggle: typeof result.options.onToggle === 'function',
		});
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	got := v.String()
	want := `{"hasModel":true,"hasOptions":true,"altScreen":true,"toggleKey":29,"hasOnToggle":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestModule_FromModel_CustomConfig(t *testing.T) {
	runtime, _ := testRequire(t)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		var fakeModel = { _type: 'bubbleteaModel', _modelID: 1 };
		var result = m.fromModel(fakeModel, { altScreen: false, toggleKey: 0x01 });
		JSON.stringify({
			altScreen: result.options.altScreen,
			toggleKey: result.options.toggleKey,
		});
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	got := v.String()
	want := `{"altScreen":false,"toggleKey":1}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestModule_FromModel_MissingArg(t *testing.T) {
	runtime, _ := testRequire(t)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		try {
			m.fromModel();
			'no error';
		} catch(e) {
			e.message || 'error';
		}
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if v.String() == "no error" {
		t.Error("expected TypeError for missing model arg")
	}
}

func TestModule_FromModel_NullArg(t *testing.T) {
	runtime, _ := testRequire(t)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		try {
			m.fromModel(null);
			'no error';
		} catch(e) {
			e.message || 'error';
		}
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if v.String() == "no error" {
		t.Error("expected TypeError for null model arg")
	}
}

func TestModule_FromModel_OnToggle_NoChild(t *testing.T) {
	runtime, _ := testRequire(t)

	// onToggle should return undefined when no child is attached
	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		var fakeModel = { _type: 'bubbleteaModel', _modelID: 1 };
		var result = m.fromModel(fakeModel);
		var toggleResult = result.options.onToggle();
		String(toggleResult);
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if v.String() != "undefined" {
		t.Errorf("expected 'undefined' when no child, got %q", v.String())
	}
}

// --- T10: Expanded unit tests for termmux facade ---

// resolveChild success paths

func TestResolveChild_StringIO(t *testing.T) {
	sio := &mockStringIO{}
	rwc, err := resolveChild(sio)
	if err != nil {
		t.Fatalf("resolveChild(StringIO): %v", err)
	}
	if rwc == nil {
		t.Fatal("expected non-nil ReadWriteCloser")
	}
	// Verify we can write through it (goes to StringIO.Send)
	n, err := rwc.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 5 {
		t.Errorf("Write n=%d, want 5", n)
	}
	if sio.lastSent != "hello" {
		t.Errorf("Send got %q, want %q", sio.lastSent, "hello")
	}
}

func TestResolveChild_MapWithGoHandle_StringIO(t *testing.T) {
	sio := &mockStringIO{}
	m := map[string]any{
		"_goHandle": sio,
	}
	rwc, err := resolveChild(m)
	if err != nil {
		t.Fatalf("resolveChild(map+StringIO): %v", err)
	}
	if rwc == nil {
		t.Fatal("expected non-nil ReadWriteCloser")
	}
}

func TestResolveChild_MapWithGoHandle_RWC(t *testing.T) {
	mrwc := &mockRWC{}
	m := map[string]any{
		"_goHandle": mrwc,
	}
	rwc, err := resolveChild(m)
	if err != nil {
		t.Fatalf("resolveChild(map+RWC): %v", err)
	}
	if rwc != mrwc {
		t.Error("expected resolveChild to return the RWC directly")
	}
}

func TestResolveChild_DirectRWC(t *testing.T) {
	mrwc := &mockRWC{}
	rwc, err := resolveChild(mrwc)
	if err != nil {
		t.Fatalf("resolveChild(RWC): %v", err)
	}
	if rwc != mrwc {
		t.Error("expected resolveChild to return the RWC directly")
	}
}

func TestResolveChild_MapWithNilGoHandle(t *testing.T) {
	m := map[string]any{
		"_goHandle": nil,
	}
	_, err := resolveChild(m)
	if err == nil {
		t.Fatal("expected error for nil _goHandle")
	}
}

func TestResolveChild_MapWithInvalidGoHandle(t *testing.T) {
	m := map[string]any{
		"_goHandle": "not a handle",
	}
	_, err := resolveChild(m)
	if err == nil {
		t.Fatal("expected error for invalid _goHandle type")
	}
}

func TestResolveChild_MapWithoutGoHandle(t *testing.T) {
	m := map[string]any{
		"other": "field",
	}
	_, err := resolveChild(m)
	if err == nil {
		t.Fatal("expected error for map without _goHandle")
	}
}

// attachWithRetry tests

func TestAttachWithRetry_SuccessFirstTry(t *testing.T) {
	// Create a mux and a mock child, attach should succeed
	mux := parent.New(nil, nil, -1)
	mrwc := &mockRWC{}
	err := attachWithRetry(mux, mrwc)
	if err != nil {
		t.Fatalf("attachWithRetry first try: %v", err)
	}
	if !mux.HasChild() {
		t.Error("expected HasChild() == true after attach")
	}
}

func TestAttachWithRetry_RetryOnAlreadyAttached(t *testing.T) {
	// Attach a child, then try to attach another — should detach first child, attach second.
	// Note: Detach() disconnects without calling Close(); the caller manages cleanup.
	mux := parent.New(nil, nil, -1)
	first := &mockRWC{}
	if err := mux.Attach(first); err != nil {
		t.Fatalf("initial attach: %v", err)
	}
	second := &mockRWC{}
	err := attachWithRetry(mux, second)
	if err != nil {
		t.Fatalf("attachWithRetry retry: %v", err)
	}
	if !mux.HasChild() {
		t.Error("expected HasChild() == true after retry-attach")
	}
}

// setStatus, setToggleKey, setStatusEnabled smoke tests

func TestModule_SetStatus(t *testing.T) {
	runtime, _ := testRequire(t)

	_, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		m.setStatus('Building branch 3/5...');
		m.setStatus('');
		m.setStatus('Done ✓');
	`)
	if err != nil {
		t.Fatalf("setStatus: %v", err)
	}
}

func TestModule_SetToggleKey(t *testing.T) {
	runtime, _ := testRequire(t)

	_, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		m.setToggleKey(0x1C);
		m.setToggleKey(29);
	`)
	if err != nil {
		t.Fatalf("setToggleKey: %v", err)
	}
}

func TestModule_SetStatusEnabled(t *testing.T) {
	runtime, _ := testRequire(t)

	_, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		m.setStatusEnabled(false);
		m.setStatusEnabled(true);
	`)
	if err != nil {
		t.Fatalf("setStatusEnabled: %v", err)
	}
}

// setResizeFunc → event queue → pollEvents roundtrip

func TestModule_SetResizeFunc_PollEvents(t *testing.T) {
	runtime, _ := testRequire(t)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		var resizes = [];
		m.setResizeFunc(function(rows, cols) {
			resizes.push({rows: rows, cols: cols});
		});
		// Register listener for resize events
		var resizeEvents = [];
		m.on('resize', function(data) {
			resizeEvents.push({rows: data.rows, cols: data.cols});
		});
		// pollEvents returns 0 with no pending events
		var before = m.pollEvents();
		JSON.stringify({before: before, resizes: resizes.length, events: resizeEvents.length});
	`)
	if err != nil {
		t.Fatalf("setResizeFunc: %v", err)
	}
	got := v.String()
	want := `{"before":0,"resizes":0,"events":0}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// on() with actual event emission (via internal emit)

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

// attach via JS (with exports from Go)

func TestModule_Attach_Detach_HasChild(t *testing.T) {
	// We need to inject a Go RWC into the JS runtime, then call attach()
	runtime, _ := testRequire(t)
	mrwc := &mockRWC{}

	// Set the mock handle as a global so JS can use it
	_ = runtime.Set("__testHandle", mrwc)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		var before = m.hasChild();
		m.attach(__testHandle);
		var after = m.hasChild();
		m.detach();
		var detached = m.hasChild();
		JSON.stringify({before: before, after: after, detached: detached});
	`)
	if err != nil {
		t.Fatalf("attach/detach: %v", err)
	}
	got := v.String()
	want := `{"before":false,"after":true,"detached":false}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	// Note: Detach() disconnects without calling Close(); caller manages cleanup.
}

func TestModule_Attach_MissingArg(t *testing.T) {
	runtime, _ := testRequire(t)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		try { m.attach(); 'no error'; } catch(e) { e.message || 'error'; }
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if v.String() == "no error" {
		t.Error("expected error for missing argument")
	}
}

func TestModule_Attach_InvalidType(t *testing.T) {
	runtime, _ := testRequire(t)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		try { m.attach("not a handle"); 'no error'; } catch(e) { e.message || 'error'; }
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if v.String() == "no error" {
		t.Error("expected error for string argument")
	}
}

func TestModule_Attach_RetryOnAlreadyAttached(t *testing.T) {
	runtime, _ := testRequire(t)
	first := &mockRWC{}
	second := &mockRWC{}

	_ = runtime.Set("__first", first)
	_ = runtime.Set("__second", second)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		m.attach(__first);
		var before = m.hasChild();
		m.attach(__second);
		var after = m.hasChild();
		JSON.stringify({before: before, after: after});
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	got := v.String()
	want := `{"before":true,"after":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	// Note: Detach() disconnects without calling Close(); caller manages cleanup.
}

// ActiveSide changes after switchTo context

func TestModule_ActiveSide_InitialState(t *testing.T) {
	runtime, _ := testRequire(t)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		m.activeSide();
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if v.String() != "osm" {
		t.Errorf("activeSide = %q, want osm", v.String())
	}
}

// Screenshot with no child content

func TestModule_Screenshot_NoChild(t *testing.T) {
	runtime, _ := testRequire(t)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		m.screenshot();
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if v.String() != "" {
		t.Errorf("screenshot = %q, want empty", v.String())
	}
}

// --- T10: Mock types ---

// mockStringIO satisfies parent.StringIO for resolveChild tests.
type mockStringIO struct {
	lastSent string
	closed   bool
}

func (m *mockStringIO) Send(input string) error  { m.lastSent = input; return nil }
func (m *mockStringIO) Receive() (string, error) { return "", fmt.Errorf("eof") }
func (m *mockStringIO) Close() error             { m.closed = true; return nil }

// mockRWC satisfies io.ReadWriteCloser for resolveChild/attach tests.
type mockRWC struct {
	closed bool
}

func (m *mockRWC) Read(p []byte) (int, error)  { return 0, fmt.Errorf("eof") }
func (m *mockRWC) Write(p []byte) (int, error) { return len(p), nil }
func (m *mockRWC) Close() error                { m.closed = true; return nil }

// --- T11: Full facade lifecycle integration test ---
// Exercises the complete JS → facade → Mux lifecycle without needing a real
// terminal. Tests attach, hasChild, setStatus, setToggleKey, setStatusEnabled,
// event subscription, detach, and fromModel in a single coherent flow.

func TestModule_FullLifecycle(t *testing.T) {
	runtime, _ := testRequire(t)
	child := &mockRWC{}
	_ = runtime.Set("__child", child)

	v, err := runtime.RunString(`
		var tmux = require('osm:termmux');
		var m = tmux.newMux({ toggleKey: 0x1D, statusEnabled: true, initialStatus: 'init' });
		var log = [];

		// 1. Initial state
		log.push('hasChild:' + m.hasChild());
		log.push('activeSide:' + m.activeSide());
		log.push('screenshot:' + m.screenshot());

		// 2. Attach child
		m.attach(__child);
		log.push('attached:' + m.hasChild());

		// 3. Configure
		m.setStatus('working...');
		m.setToggleKey(0x1C);
		m.setStatusEnabled(false);
		m.setStatusEnabled(true);

		// 4. Event subscription
		var focusEvents = [];
		var exitEvents = [];
		var focusId = m.on('focus', function(data) {
			focusEvents.push(data.side);
		});
		var exitId = m.on('exit', function(data) {
			exitEvents.push(data.reason || 'no-reason');
		});
		log.push('focusId:' + (focusId > 0));
		log.push('exitId:' + (exitId > 0));

		// 5. pollEvents — no events pending
		var polled = m.pollEvents();
		log.push('polled:' + polled);

		// 6. Unsubscribe exit, verify focus still works
		var offResult = m.off(exitId);
		log.push('off:' + offResult);
		var offAgain = m.off(exitId);
		log.push('offAgain:' + offAgain);

		// 7. fromModel
		var fakeModel = { _type: 'model', _id: 1 };
		var tf = m.fromModel(fakeModel);
		log.push('fromModel.model:' + (tf.model === fakeModel));
		log.push('fromModel.altScreen:' + tf.options.altScreen);
		log.push('fromModel.onToggle:' + (typeof tf.options.onToggle));

		// 8. Detach
		m.detach();
		log.push('detached:' + m.hasChild());

		// 9. Detach again — idempotent
		m.detach();
		log.push('detachAgain:' + m.hasChild());

		log.join('|');
	`)
	if err != nil {
		t.Fatalf("FullLifecycle RunString: %v", err)
	}

	got := v.String()
	want := "hasChild:false|activeSide:osm|screenshot:|attached:true|focusId:true|exitId:true|polled:0|off:true|offAgain:false|fromModel.model:true|fromModel.altScreen:true|fromModel.onToggle:function|detached:false|detachAgain:false"
	if got != want {
		t.Errorf("lifecycle log:\n  got:  %s\n  want: %s", got, want)
	}
}

// TestModule_EventListenerRegistration_ViaJS tests that event listeners can be
// registered, polled, and unregistered through the full JS → WrapMux path.
// Uses a real parent.Mux (created via Go API) wrapped with WrapMux.
// NOTE: This does NOT test the resize-callback-to-event-queue pipeline because
// the mux's resize func is only invoked during RunPassthrough (requires a real
// PTY). Instead it verifies: listener registration, setResizeFunc installation,
// pollEvents with empty queue, and multiple listener cleanup.
func TestModule_EventListenerRegistration_ViaJS(t *testing.T) {
	runtime, _ := testRequire(t)

	mux := parent.New(nil, nil, -1)
	muxObj := WrapMux(context.Background(), runtime, mux)
	_ = runtime.Set("__mux", muxObj)

	v, err := runtime.RunString(`
		var m = __mux;
		var resizeData = [];
		var focusData = [];

		// Register listeners for resize and focus events.
		var rid = m.on('resize', function(d) { resizeData.push(d.rows + 'x' + d.cols); });
		var fid = m.on('focus', function(d) { focusData.push(d.side); });

		// Install a resize handler — verifies setResizeFunc doesn't throw.
		var resizeCalls = [];
		m.setResizeFunc(function(rows, cols) {
			resizeCalls.push(rows + 'x' + cols);
		});

		// No events have fired, so pollEvents should return 0.
		var polled = m.pollEvents();

		// Unsubscribe both.
		var rOff = m.off(rid);
		var fOff = m.off(fid);

		JSON.stringify({
			ridPositive: rid > 0,
			fidPositive: fid > 0,
			polled: polled,
			resizeData: resizeData,
			focusData: focusData,
			rOff: rOff,
			fOff: fOff,
		});
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}

	got := v.String()
	want := `{"ridPositive":true,"fidPositive":true,"polled":0,"resizeData":[],"focusData":[],"rOff":true,"fOff":true}`
	if got != want {
		t.Errorf("EventListenerRegistration:\n  got:  %s\n  want: %s", got, want)
	}
}

// TestModule_AttachStringIO_ViaJS tests the full StringIO attach path through JS.
func TestModule_AttachStringIO_ViaJS(t *testing.T) {
	runtime, _ := testRequire(t)
	sio := &mockStringIO{}
	_ = runtime.Set("__sio", sio)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		m.attach(__sio);
		var has = m.hasChild();
		m.detach();
		var after = m.hasChild();
		JSON.stringify({has: has, after: after});
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	got := v.String()
	want := `{"has":true,"after":false}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestModule_AttachMapWithGoHandle_ViaJS tests the AgentHandle pattern (map with _goHandle).
func TestModule_AttachMapWithGoHandle_ViaJS(t *testing.T) {
	runtime, _ := testRequire(t)

	// Create a Go map that mimics what Goja exports for an AgentHandle
	sio := &mockStringIO{}
	handle := map[string]any{
		"_goHandle": sio,
		"send":      func(s string) {},
		"receive":   func() string { return "" },
		"close":     func() {},
	}
	_ = runtime.Set("__handle", handle)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		m.attach(__handle);
		var has = m.hasChild();
		m.detach();
		JSON.stringify({has: has});
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	got := v.String()
	want := `{"has":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// --- T31: Bell event propagation to JS ---

// pipeMockChild implements io.ReadWriteCloser using pipes (similar to termmux_test.go).
type pipeMockChild struct {
	r io.ReadCloser
	w io.Writer
}

func (p *pipeMockChild) Read(b []byte) (int, error)  { return p.r.Read(b) }
func (p *pipeMockChild) Write(b []byte) (int, error) { return p.w.Write(b) }
func (p *pipeMockChild) Close() error                { return p.r.Close() }

func TestModule_BellEvent_QueuedThroughMux(t *testing.T) {
	// Full chain: child writes BEL → VTerm → Mux.BellFn → SetBellFunc → events.queue → pollEvents → JS callback
	runtime, _ := testRequire(t)

	childR, childW := io.Pipe()
	mc := &pipeMockChild{r: childR, w: io.Discard}

	mux := parent.New(nil, io.Discard, -1)
	muxObj := WrapMux(context.Background(), runtime, mux)
	_ = runtime.Set("__mux", muxObj)

	// Register bell listener.
	_, err := runtime.RunString(`
		var m = __mux;
		var bellEvents = [];
		m.on('bell', function(data) {
			bellEvents.push(data.pane || 'unknown');
		});
	`)
	if err != nil {
		t.Fatalf("RunString setup: %v", err)
	}

	// Attach the mock child (this wires BellFn + SetBellFunc).
	if err := mux.Attach(mc); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	// Child sends text with BEL.
	childW.Write([]byte("Hello\x07World\x07"))
	childW.Close()

	// Wait for teeLoop to finish processing all output.
	// Poll until we've received the expected bell events.
	deadline := time.After(5 * time.Second)
	for {
		// Poll for events.
		_, err := runtime.RunString(`m.pollEvents()`)
		if err != nil {
			t.Fatalf("pollEvents: %v", err)
		}
		v, err := runtime.RunString(`bellEvents.length`)
		if err != nil {
			t.Fatalf("bellEvents.length: %v", err)
		}
		if v.ToInteger() >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for 2 bell events; got %d", v.ToInteger())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Verify bell event data.
	v, err := runtime.RunString(`JSON.stringify(bellEvents)`)
	if err != nil {
		t.Fatalf("RunString verify: %v", err)
	}
	got2 := v.String()
	want2 := `["claude","claude"]`
	if got2 != want2 {
		t.Errorf("bellEvents = %s, want %s", got2, want2)
	}
}

func TestModule_OutputEvent_QueuedThroughMux(t *testing.T) {
	// Full chain: child writes output → VTerm → Mux.OutputFn → events.queue → pollEvents → JS callback
	runtime, _ := testRequire(t)

	childR, childW := io.Pipe()
	mc := &pipeMockChild{r: childR, w: io.Discard}

	mux := parent.New(nil, io.Discard, -1)
	muxObj := WrapMux(context.Background(), runtime, mux)
	_ = runtime.Set("__mux", muxObj)

	_, err := runtime.RunString(`
		var m = __mux;
		var outputEvents = [];
		m.on('output', function(data) {
			outputEvents.push({
				pane: data && data.pane || 'unknown',
				chunk: data && data.chunk || ''
			});
		});
	`)
	if err != nil {
		t.Fatalf("RunString setup: %v", err)
	}

	if err := mux.Attach(mc); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	childW.Write([]byte("Hello output"))
	childW.Close()

	deadline := time.After(5 * time.Second)
	for {
		_, err := runtime.RunString(`m.pollEvents()`)
		if err != nil {
			t.Fatalf("pollEvents: %v", err)
		}
		v, err := runtime.RunString(`outputEvents.length`)
		if err != nil {
			t.Fatalf("outputEvents.length: %v", err)
		}
		if v.ToInteger() >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for output event; got %d", v.ToInteger())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	v, err := runtime.RunString(`JSON.stringify(outputEvents)`)
	if err != nil {
		t.Fatalf("RunString verify: %v", err)
	}
	got := v.String()
	if !strings.Contains(got, `"pane":"claude"`) {
		t.Errorf("outputEvents = %s, want pane claude", got)
	}
	if !strings.Contains(got, `"chunk":"Hello output"`) {
		t.Errorf("outputEvents = %s, want chunk Hello output", got)
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

// TestModule_LastActivityMs_MinusOneBeforeOutput verifies that lastActivityMs()
// returns -1 when no child output has been received yet.
func TestModule_LastActivityMs_MinusOneBeforeOutput(t *testing.T) {
	vm := goja.New()
	mux := parent.New(strings.NewReader(""), io.Discard, -1)
	obj := WrapMux(context.Background(), vm, mux)

	fn, ok := goja.AssertFunction(obj.ToObject(vm).Get("lastActivityMs"))
	if !ok {
		t.Fatal("lastActivityMs is not a function")
	}
	val, err := fn(goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	ms := val.ToInteger()
	if ms != -1 {
		t.Errorf("lastActivityMs() = %d before output; want -1", ms)
	}
}

// TestModule_LastActivityMs_PositiveAfterOutput verifies that lastActivityMs()
// returns a positive value after child output flows through teeLoop.
func TestModule_LastActivityMs_PositiveAfterOutput(t *testing.T) {
	vm := goja.New()
	mux := parent.New(strings.NewReader(""), io.Discard, -1)

	childR, childW := io.Pipe()
	mc := &pipeMockChild{r: childR, w: io.Discard}

	if err := mux.Attach(mc); err != nil {
		t.Fatal(err)
	}

	// Write some output through the child.
	childW.Write([]byte("output data\n"))
	time.Sleep(150 * time.Millisecond)

	obj := WrapMux(context.Background(), vm, mux)
	fn, ok := goja.AssertFunction(obj.ToObject(vm).Get("lastActivityMs"))
	if !ok {
		t.Fatal("lastActivityMs is not a function")
	}
	val, err := fn(goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	ms := val.ToInteger()
	if ms < 0 {
		t.Errorf("lastActivityMs() = %d after output; want >= 0", ms)
	}
	if ms > 5000 {
		t.Errorf("lastActivityMs() = %d after output; suspiciously large (want < 5000)", ms)
	}

	childW.Close()
	childR.Close()
}

// ── T27 Tests: childScreen() and writeToChild() JS bindings ────────

func TestModule_ChildScreen_NoChild(t *testing.T) {
	runtime, _ := testRequire(t)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		m.childScreen();
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if v.String() != "" {
		t.Errorf("childScreen() = %q, want empty", v.String())
	}
}

func TestModule_ChildScreen_ReturnsANSI(t *testing.T) {
	vm := goja.New()
	mux := parent.New(strings.NewReader(""), io.Discard, -1)

	childR, childW := io.Pipe()
	mc := &pipeMockChild{r: childR, w: io.Discard}

	if err := mux.Attach(mc); err != nil {
		t.Fatal(err)
	}

	// Write ANSI colored output.
	childW.Write([]byte("\x1b[31mERROR\x1b[0m ok"))
	time.Sleep(150 * time.Millisecond)

	obj := WrapMux(context.Background(), vm, mux)
	fn, ok := goja.AssertFunction(obj.ToObject(vm).Get("childScreen"))
	if !ok {
		t.Fatal("childScreen is not a function")
	}
	val, err := fn(goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	got := val.String()
	if !strings.Contains(got, "\x1b[") {
		t.Errorf("childScreen() = %q; want ANSI escape sequences", got)
	}

	childW.Close()
	childR.Close()
}

func TestModule_WriteToChild_NoChild(t *testing.T) {
	runtime, _ := testRequire(t)

	// writeToChild with no child should throw.
	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		try { m.writeToChild('hello'); 'no error'; } catch(e) { 'caught:' + e.message; }
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	got := v.String()
	if !strings.Contains(got, "caught:") {
		t.Errorf("writeToChild without child should throw; got %q", got)
	}
}

func TestModule_WriteToChild_Forwards(t *testing.T) {
	vm := goja.New()
	registry := require.NewRegistry()
	registry.RegisterNativeModule("osm:termmux", Require(context.Background(), nil, nil))
	registry.Enable(vm)

	// Create a pipe to capture what the child receives.
	childR, childW := io.Pipe()
	readR, readW := io.Pipe()
	mc := &pipeMockChild{r: childR, w: readW}

	mux := parent.New(strings.NewReader(""), io.Discard, -1)
	if err := mux.Attach(mc); err != nil {
		t.Fatal(err)
	}

	obj := WrapMux(context.Background(), vm, mux)
	_ = vm.Set("__mux", obj)

	// Write to child via JS binding.
	done := make(chan string, 1)
	go func() {
		buf := make([]byte, 256)
		n, _ := readR.Read(buf)
		done <- string(buf[:n])
	}()

	_, err := vm.RunString(`__mux.writeToChild('hello claude')`)
	if err != nil {
		t.Fatalf("writeToChild: %v", err)
	}

	select {
	case got := <-done:
		if got != "hello claude" {
			t.Errorf("child received %q; want 'hello claude'", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for child to receive data")
	}

	childW.Close()
	readW.Close()
	readR.Close()
}

func TestModule_WriteToChild_MissingArg(t *testing.T) {
	runtime, _ := testRequire(t)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		try { m.writeToChild(); 'no error'; } catch(e) { 'caught:' + e.message; }
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	got := v.String()
	if !strings.Contains(got, "caught:") {
		t.Errorf("writeToChild without args should throw; got %q", got)
	}
}
