// Package termmux provides JavaScript bindings for the terminal multiplexer,
// registered as the "osm:termmux" native module. It wraps
// [github.com/joeycumines/one-shot-man/internal/termmux] to expose pane
// management, passthrough control, and configuration to Goja scripts.
package termmux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/dop251/goja"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// Event name constants exposed to JS.
const (
	EventExit   = "exit"
	EventResize = "resize"
	EventFocus  = "focus"
	EventBell   = "bell"
	EventOutput = "output"
)

// validEvents is the set of event names accepted by on().
var validEvents = map[string]bool{
	EventExit:   true,
	EventResize: true,
	EventFocus:  true,
	EventBell:   true,
	EventOutput: true,
}

// eventListener is a single registered JS callback for an event type.
type eventListener struct {
	event    string
	callback goja.Callable
}

// pendingEvent is an event queued from a non-JS goroutine for later delivery.
type pendingEvent struct {
	event string
	data  map[string]any
}

// muxEvents manages per-mux event listeners and an async event queue.
type muxEvents struct {
	mu        sync.Mutex
	listeners map[int]*eventListener
	nextID    int

	// pending buffers events from non-JS goroutines (e.g., resize via SIGWINCH).
	// Drained by pollEvents() on the JS goroutine.
	pending chan pendingEvent
}

// newMuxEvents creates the event system with a buffered pending channel.
func newMuxEvents() *muxEvents {
	return &muxEvents{
		listeners: make(map[int]*eventListener),
		pending:   make(chan pendingEvent, 64),
	}
}

// on registers a listener for the given event. Returns a unique ID.
func (e *muxEvents) on(event string, callback goja.Callable) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nextID++
	id := e.nextID
	e.listeners[id] = &eventListener{event: event, callback: callback}
	return id
}

// off removes a listener by ID. Returns true if it existed.
func (e *muxEvents) off(id int) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.listeners[id]
	delete(e.listeners, id)
	return ok
}

// emit delivers an event synchronously to all matching listeners.
// MUST be called on the JS goroutine (Goja runtime is not thread-safe).
func (e *muxEvents) emit(runtime *goja.Runtime, event string, data map[string]any) {
	e.mu.Lock()
	// Snapshot listeners under lock, then release before invoking.
	type snap struct {
		id int
		cb goja.Callable
	}
	var targets []snap
	for id, l := range e.listeners {
		if l.event == event {
			targets = append(targets, snap{id, l.callback})
		}
	}
	e.mu.Unlock()

	for _, t := range targets {
		_, _ = t.cb(goja.Undefined(), runtime.ToValue(data))
	}
}

// queue enqueues an event for later delivery via pollEvents(). Thread-safe.
// If the channel is full, the event is dropped (non-blocking).
func (e *muxEvents) queue(event string, data map[string]any) {
	select {
	case e.pending <- pendingEvent{event: event, data: data}:
	default:
		// Drop event rather than block a non-JS goroutine.
	}
}

// drain delivers all pending async events. MUST be called on the JS goroutine.
func (e *muxEvents) drain(runtime *goja.Runtime) int {
	count := 0
	for {
		select {
		case ev := <-e.pending:
			e.emit(runtime, ev.event, ev.data)
			count++
		default:
			return count
		}
	}
}

// Require returns a module loader for "osm:termmux" that exposes the terminal
// multiplexer to JavaScript. The input/output parameters are optional; when nil
// the module falls back to os.Stdin/os.Stdout.
func Require(ctx context.Context, input io.Reader, output io.Writer) func(*goja.Runtime, *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

		// ── Constants ────────────────────────────────────────
		_ = exports.Set("EXIT_TOGGLE", "toggle")
		_ = exports.Set("EXIT_CHILD_EXIT", "childExit")
		_ = exports.Set("EXIT_CONTEXT", "context")
		_ = exports.Set("EXIT_ERROR", "error")
		_ = exports.Set("SIDE_OSM", "osm")
		_ = exports.Set("SIDE_CLAUDE", "claude")
		_ = exports.Set("DEFAULT_TOGGLE_KEY", int(parent.DefaultToggleKey))

		// ── Event name constants ─────────────────────────────
		_ = exports.Set("EVENT_EXIT", EventExit)
		_ = exports.Set("EVENT_RESIZE", EventResize)
		_ = exports.Set("EVENT_FOCUS", EventFocus)
		_ = exports.Set("EVENT_BELL", EventBell)
		_ = exports.Set("EVENT_OUTPUT", EventOutput)

		// ── Constructor ──────────────────────────────────────
		_ = exports.Set("newMux", func(call goja.FunctionCall) goja.Value {
			return newMux(ctx, runtime, input, output, call)
		})
	}
}

// newMux creates a [parent.Mux] from an optional JS options object and returns
// a wrapped JS object exposing all multiplexer methods.
//
// JS signature:
//
//	termmux.newMux({ toggleKey?: number, statusEnabled?: boolean, initialStatus?: string })
func newMux(ctx context.Context, runtime *goja.Runtime, input io.Reader, output io.Writer, call goja.FunctionCall) goja.Value {
	var opts []parent.Option

	// Parse optional config object.
	if arg := call.Argument(0); arg != nil && !goja.IsUndefined(arg) && !goja.IsNull(arg) {
		obj := arg.ToObject(runtime)
		if v := obj.Get("toggleKey"); v != nil && !goja.IsUndefined(v) {
			key := byte(v.ToInteger())
			opts = append(opts, func(c *parent.Config) { c.ToggleKey = key })
		}
		if v := obj.Get("statusEnabled"); v != nil && !goja.IsUndefined(v) {
			enabled := v.ToBoolean()
			opts = append(opts, func(c *parent.Config) { c.StatusEnabled = enabled })
		}
		if v := obj.Get("initialStatus"); v != nil && !goja.IsUndefined(v) {
			status := v.String()
			opts = append(opts, func(c *parent.Config) { c.InitialStatus = status })
		}
	}

	// Resolve terminal I/O from provided args or OS defaults.
	stdin := input
	stdout := output
	termFd := int(os.Stdin.Fd())
	if stdin == nil {
		stdin = os.Stdin
	}
	if stdout == nil {
		stdout = os.Stdout
	}

	mux := parent.New(stdin, stdout, termFd, opts...)
	return WrapMux(ctx, runtime, mux)
}

// WrapMux wraps a [parent.Mux] into a Goja object with JavaScript-callable methods.
// This is exported so callers (e.g., pr_split.go) can create a Go-side Mux and
// expose it to JS through the same standardized interface that newMux() provides.
func WrapMux(ctx context.Context, runtime *goja.Runtime, mux *parent.Mux) goja.Value {
	obj := runtime.NewObject()
	events := newMuxEvents()

	// ── attach(handle) ───────────────────────────────────
	// Accepts AgentHandle (map with _goHandle), StringIO, or io.ReadWriteCloser.
	// On ErrAlreadyAttached, detaches first then retries once.
	_ = obj.Set("attach", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("attach: handle argument is required"))
		}
		raw := call.Argument(0).Export()
		child, err := resolveChild(raw)
		if err != nil {
			panic(runtime.NewTypeError(fmt.Sprintf("attach: %v", err)))
		}
		if err := attachWithRetry(mux, child); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// ── detach() ─────────────────────────────────────────
	// Idempotent — safe to call with no child attached.
	_ = obj.Set("detach", func() {
		if err := mux.Detach(); err != nil {
			panic(runtime.NewGoError(err))
		}
	})

	// ── hasChild() → boolean ─────────────────────────────
	_ = obj.Set("hasChild", func() bool {
		return mux.HasChild()
	})

	// ── switchTo() → { reason, error?, childOutput? } ────
	// BLOCKING: enters raw passthrough mode, returns when user toggles or child exits.
	// Emits "focus" event on entry (side: "claude") and exit (side: "osm").
	// Emits "exit" event with the passthrough result.
	// Drains pending async events before returning.
	_ = obj.Set("switchTo", func() map[string]any {
		// Focus → claude before entering passthrough.
		events.emit(runtime, EventFocus, map[string]any{"side": "claude"})

		reason, err := mux.RunPassthrough(ctx)
		result := map[string]any{
			"reason": exitReasonString(reason),
		}
		if err != nil {
			result["error"] = err.Error()
		}
		if reason == parent.ExitChildExit {
			if output := mux.ChildExitOutput(); output != "" {
				result["childOutput"] = output
			}
		}

		// Focus → osm after exiting passthrough.
		events.emit(runtime, EventFocus, map[string]any{"side": "osm"})

		// Emit exit event with the passthrough result.
		events.emit(runtime, EventExit, result)

		// Drain any async events that accumulated during passthrough.
		events.drain(runtime)

		return result
	})

	// ── activeSide() → "osm" | "claude" ─────────────────
	_ = obj.Set("activeSide", func() string {
		if mux.ActiveSide() == parent.SideClaude {
			return "claude"
		}
		return "osm"
	})

	// ── setStatus(text) ──────────────────────────────────
	_ = obj.Set("setStatus", func(s string) {
		mux.SetClaudeStatus(s)
	})

	// ── setToggleKey(keyByte) ────────────────────────────
	_ = obj.Set("setToggleKey", func(k int) {
		mux.SetToggleKey(byte(k))
	})

	// ── setStatusEnabled(bool) ───────────────────────────
	_ = obj.Set("setStatusEnabled", func(b bool) {
		mux.SetStatusEnabled(b)
	})

	// ── setResizeFunc(fn) ────────────────────────────────
	// Installs user resize handler AND queues "resize" events for listeners.
	// The underlying resize callback runs on the SIGWINCH goroutine, so we
	// queue events rather than emitting synchronously.
	_ = obj.Set("setResizeFunc", func(fn func(int, int)) {
		mux.SetResizeFunc(func(rows, cols uint16) error {
			fn(int(rows), int(cols))
			events.queue(EventResize, map[string]any{
				"rows": int(rows),
				"cols": int(cols),
			})
			return nil
		})
	})

	// ── bell event wiring ────────────────────────────────
	// Install mux bell callback that queues "bell" events for JS listeners.
	// The callback runs on the tee goroutine so we use queue() (thread-safe)
	// rather than emit() (JS-goroutine only). JS receives bell events via
	// pollEvents() or drain().
	mux.SetBellFunc(func() {
		events.queue(EventBell, map[string]any{
			"pane": "claude",
		})
	})

	// ── screenshot() → string ────────────────────────────
	// Returns plain-text VTerm buffer content for diagnostics/test assertions.
	_ = obj.Set("screenshot", func() string {
		return mux.ChildExitOutput()
	})

	// ── lastActivityMs() → int64 ─────────────────────────
	// Returns milliseconds since the last child process output, or -1 if
	// no output has been received yet. Used by the HUD overlay to show an
	// activity indicator (live vs idle).
	_ = obj.Set("lastActivityMs", func() int64 {
		t := mux.LastWriteTime()
		if t.IsZero() {
			return -1
		}
		return time.Since(t).Milliseconds()
	})

	// ── on(event, callback) → id ─────────────────────────
	// Registers a listener for an event type. Returns a numeric ID for off().
	// Supported events: exit, resize, focus, bell, output.
	// Note: "output" events require parent termmux changes to fire.
	_ = obj.Set("on", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(runtime.NewTypeError("on: requires (event, callback)"))
		}
		event := call.Argument(0).String()
		if !validEvents[event] {
			panic(runtime.NewTypeError(fmt.Sprintf("on: unknown event %q", event)))
		}
		callback, ok := goja.AssertFunction(call.Argument(1))
		if !ok {
			panic(runtime.NewTypeError("on: callback must be a function"))
		}
		id := events.on(event, callback)
		return runtime.ToValue(id)
	})

	// ── off(id) → boolean ────────────────────────────────
	// Removes a previously registered listener. Returns true if it existed.
	_ = obj.Set("off", func(id int) bool {
		return events.off(id)
	})

	// ── pollEvents() → number ────────────────────────────
	// Drains pending async events (resize, bell, output) and delivers them to
	// registered listeners. Returns the count of events delivered. Call this
	// periodically from JS to receive events from non-JS goroutines.
	_ = obj.Set("pollEvents", func() int {
		return events.drain(runtime)
	})

	// fromModel wraps a BubbleTea model with termmux toggle key integration.
	// Returns { model: <model>, options: { altScreen: true, toggleKey: <byte>, onToggle: <fn> } }
	// where onToggle calls mux.switchTo(). Usage:
	//
	//   const tf = mux.fromModel(model);
	//   tea.run(tf.model, tf.options);
	//
	// Or with custom options:
	//
	//   const tf = mux.fromModel(model, { altScreen: false, toggleKey: 0x1D });
	//   tea.run(tf.model, Object.assign(tf.options, { reportFocus: true }));
	_ = obj.Set("fromModel", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(runtime.NewTypeError("fromModel requires a model argument"))
		}
		model := call.Argument(0)

		// Parse optional config overrides
		altScreen := true
		toggleKeyByte := int(parent.DefaultToggleKey)
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			cfgObj := call.Argument(1).ToObject(runtime)
			if cfgObj != nil {
				if v := cfgObj.Get("altScreen"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					altScreen = v.ToBoolean()
				}
				if v := cfgObj.Get("toggleKey"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					toggleKeyByte = int(v.ToInteger())
				}
			}
		}

		result := runtime.NewObject()
		_ = result.Set("model", model)

		runOpts := runtime.NewObject()
		_ = runOpts.Set("altScreen", altScreen)
		_ = runOpts.Set("toggleKey", toggleKeyByte)

		// onToggle calls mux.switchTo() — blocks during passthrough.
		// This closure captures the mux and ctx from WrapMux's scope.
		_ = runOpts.Set("onToggle", func(fc goja.FunctionCall) goja.Value {
			if !mux.HasChild() {
				return goja.Undefined()
			}

			// Emit focus event: entering Claude's terminal
			events.emit(runtime, EventFocus, map[string]any{
				"side": "claude", "action": "enter",
			})

			reason, err := mux.RunPassthrough(ctx)

			res := map[string]any{
				"reason": exitReasonString(reason),
			}
			if err != nil {
				res["error"] = err.Error()
			}

			// Emit focus event: returning to osm's terminal
			events.emit(runtime, EventFocus, map[string]any{
				"side": "osm", "action": "return",
			})

			return runtime.ToValue(res)
		})

		_ = result.Set("options", runOpts)
		return result
	})

	return obj
}

// resolveChild converts a JS-exported value into an [io.ReadWriteCloser]
// suitable for [parent.Mux.Attach]. Supports:
//   - [parent.StringIO] (Send/Receive/Close)
//   - map with "_goHandle" key containing StringIO or io.ReadWriteCloser
//   - [io.ReadWriteCloser] directly
func resolveChild(raw any) (io.ReadWriteCloser, error) {
	// Case 1: Direct StringIO (Go test callers).
	if sio, ok := raw.(parent.StringIO); ok {
		return parent.WrapStringIO(sio), nil
	}

	// Case 2: Goja-wrapped AgentHandle — map with _goHandle.
	if m, ok := raw.(map[string]any); ok {
		if goHandle, exists := m["_goHandle"]; exists && goHandle != nil {
			if sio, ok := goHandle.(parent.StringIO); ok {
				return parent.WrapStringIO(sio), nil
			}
			if rwc, ok := goHandle.(io.ReadWriteCloser); ok {
				return rwc, nil
			}
		}
	}

	// Case 3: Direct io.ReadWriteCloser.
	if rwc, ok := raw.(io.ReadWriteCloser); ok {
		return rwc, nil
	}

	return nil, errors.New("argument must implement Send/Receive/Close (or be a wrapped AgentHandle with _goHandle)")
}

// attachWithRetry calls [parent.Mux.Attach] with automatic detach-and-retry
// on [parent.ErrAlreadyAttached].
func attachWithRetry(mux *parent.Mux, child io.ReadWriteCloser) error {
	err := mux.Attach(child)
	if err != nil && errors.Is(err, parent.ErrAlreadyAttached) {
		if detachErr := mux.Detach(); detachErr != nil {
			return fmt.Errorf("detach before reattach: %w", detachErr)
		}
		err = mux.Attach(child)
	}
	return err
}

// exitReasonString maps a [parent.ExitReason] to its JS string constant.
// Known reasons use JS-style camelCase; unknown values fall back to the
// type's own [parent.ExitReason.String] method.
func exitReasonString(r parent.ExitReason) string {
	switch r {
	case parent.ExitToggle:
		return "toggle"
	case parent.ExitChildExit:
		return "childExit"
	case parent.ExitContext:
		return "context"
	case parent.ExitError:
		return "error"
	default:
		return r.String()
	}
}
