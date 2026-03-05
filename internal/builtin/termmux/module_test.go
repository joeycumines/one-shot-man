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
		want interface{}
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
			'on', 'off', 'pollEvents'];
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
	if arr, ok := missing.([]interface{}); ok && len(arr) > 0 {
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
		if m, ok := data.(map[string]interface{}); ok {
			if side, ok := m["side"].(string); ok {
				received = append(received, side)
			}
		}
		return goja.Undefined()
	}))

	e.on("focus", cb)
	e.on("focus", cb) // Two listeners for same event.

	e.emit(runtime, "focus", map[string]interface{}{"side": "claude"})

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
	e.emit(runtime, "exit", map[string]interface{}{"reason": "toggle"})
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
	e.emit(runtime, "resize", map[string]interface{}{"rows": 24, "cols": 80})

	if called {
		t.Error("exit listener should not fire for resize event")
	}
}

func TestMuxEvents_Queue_Drain(t *testing.T) {
	e := newMuxEvents()
	runtime := goja.New()

	var received []map[string]interface{}
	cb, _ := goja.AssertFunction(runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		data := call.Argument(0).Export()
		if m, ok := data.(map[string]interface{}); ok {
			received = append(received, m)
		}
		return goja.Undefined()
	}))

	e.on("resize", cb)

	// Queue 3 events.
	e.queue("resize", map[string]interface{}{"rows": 24, "cols": 80})
	e.queue("resize", map[string]interface{}{"rows": 50, "cols": 120})
	e.queue("resize", map[string]interface{}{"rows": 30, "cols": 100})

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
	for i := 0; i < 64; i++ {
		e.queue("resize", map[string]interface{}{"i": i})
	}

	// 65th should not block (dropped).
	e.queue("resize", map[string]interface{}{"i": 64})

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
	e.emit(runtime, "exit", map[string]interface{}{"reason": "toggle"})
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
