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
