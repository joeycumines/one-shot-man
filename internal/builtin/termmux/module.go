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
	"github.com/joeycumines/one-shot-man/internal/termmux/ptyio"
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

		// ── CaptureSession factory ───────────────────────────
		_ = exports.Set("newCaptureSession", func(call goja.FunctionCall) goja.Value {
			return newCaptureSession(ctx, runtime, call)
		})

		// ── SessionManager factory (experimental) ────────────
		_ = exports.Set("newSessionManager", func(call goja.FunctionCall) goja.Value {
			return newSessionManager(ctx, runtime, call)
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

	// ── output event wiring ─────────────────────────────
	// Queue an output event whenever the child PTY produces bytes. The JS
	// side can use this to refresh panes from the latest mux snapshot without
	// depending on a separate ad hoc polling contract.
	mux.SetOutputFunc(func(data []byte) {
		events.queue(EventOutput, map[string]any{
			"pane":  "claude",
			"chunk": string(data),
		})
	})

	// ── screenshot() → string ────────────────────────────
	// Returns plain-text VTerm buffer content for diagnostics/test assertions.
	_ = obj.Set("screenshot", func() string {
		return mux.ChildExitOutput()
	})

	// ── childScreen() → string ───────────────────────────
	// Returns the VTerm buffer as ANSI escape-sequence output suitable for
	// rendering in a terminal or TUI pane. Unlike screenshot() (plain text),
	// this preserves colors, cursor position, and formatting.
	_ = obj.Set("childScreen", func() string {
		return mux.ChildScreen()
	})

	// ── writeToChild(data) ───────────────────────────────
	// Sends raw bytes to the attached child process's stdin. Accepts a string.
	// Throws if no child is attached.
	_ = obj.Set("writeToChild", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("writeToChild: data argument is required"))
		}
		data := call.Argument(0).String()
		n, err := mux.WriteToChild([]byte(data))
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		return runtime.ToValue(n)
	})

	// ── session() → InteractiveSession wrapper ──────────
	// Returns an object exposing the attached PTY through the same session
	// interface used by CaptureSession bindings.
	_ = obj.Set("session", func() goja.Value {
		return wrapInteractiveSession(runtime, mux.Session(), parent.SessionKindPTY)
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

// newCaptureSession creates a [parent.CaptureSession] from JS arguments and
// returns a wrapped JS object.
//
// JS signature:
//
//	termmux.newCaptureSession(command, args?, { dir?, rows?, cols?, env? }?)
func newCaptureSession(ctx context.Context, runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 {
		panic(runtime.NewTypeError("newCaptureSession: command argument is required"))
	}

	cmd := call.Argument(0).String()
	if cmd == "" {
		panic(runtime.NewTypeError("newCaptureSession: command must be a non-empty string"))
	}

	cfg := parent.CaptureConfig{
		Command: cmd,
	}

	// Parse optional args array (second argument).
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
		argsObj := call.Argument(1).ToObject(runtime)
		if lenVal := argsObj.Get("length"); lenVal != nil && !goja.IsUndefined(lenVal) {
			arrLen := lenVal.ToInteger()
			for i := range arrLen {
				v := argsObj.Get(fmt.Sprintf("%d", i))
				if v != nil && !goja.IsUndefined(v) {
					cfg.Args = append(cfg.Args, v.String())
				}
			}
		}
	}

	// Parse optional options object (third argument).
	if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) && !goja.IsNull(call.Argument(2)) {
		optObj := call.Argument(2).ToObject(runtime)
		if v := optObj.Get("dir"); v != nil && !goja.IsUndefined(v) {
			cfg.Dir = v.String()
		}
		if v := optObj.Get("name"); v != nil && !goja.IsUndefined(v) {
			cfg.Name = v.String()
		}
		if v := optObj.Get("kind"); v != nil && !goja.IsUndefined(v) {
			cfg.Kind = parent.SessionKind(v.String())
		}
		if v := optObj.Get("rows"); v != nil && !goja.IsUndefined(v) {
			cfg.Rows = int(v.ToInteger())
		}
		if v := optObj.Get("cols"); v != nil && !goja.IsUndefined(v) {
			cfg.Cols = int(v.ToInteger())
		}
		if v := optObj.Get("env"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			envObj := v.ToObject(runtime)
			cfg.Env = make(map[string]string)
			for _, key := range envObj.Keys() {
				val := envObj.Get(key)
				if val != nil && !goja.IsUndefined(val) && !goja.IsNull(val) {
					cfg.Env[key] = val.String()
				}
			}
		}
	}

	cs := parent.NewCaptureSession(cfg)
	return WrapCaptureSession(ctx, runtime, cs)
}

// WrapCaptureSession wraps a [parent.CaptureSession] into a Goja object with
// JavaScript-callable methods. Exported so callers (e.g., pr_split.go) can
// create a Go-side CaptureSession and expose it through the same interface.
//
// AUDIT (T004/T059/T10): All 20 methods verified present and type-correct:
//
//	start, isRunning, output, screen, interrupt, kill, pause, resume,
//	isPaused, resize, wait, write, sendEOF, close, pid, exitCode, isDone,
//	target, setTarget, passthrough.
//
// The 6 methods called by runVerifyBranch/pollVerifySession (isDone,
// exitCode, output, close, interrupt, kill) are confirmed bound with
// correct signatures via module_capture_test.go.
func WrapCaptureSession(ctx context.Context, runtime *goja.Runtime, cs *parent.CaptureSession) goja.Value {
	obj := wrapInteractiveSession(runtime, cs, parent.SessionKindCapture).ToObject(runtime)

	// ── start() ──────────────────────────────────────────
	_ = obj.Set("start", func() {
		if err := cs.Start(ctx); err != nil {
			panic(runtime.NewGoError(err))
		}
	})

	// ── isRunning() → boolean ────────────────────────────
	_ = obj.Set("isRunning", func() bool {
		return cs.IsRunning()
	})

	// ── interrupt() ──────────────────────────────────────
	_ = obj.Set("interrupt", func() {
		if err := cs.Interrupt(); err != nil {
			panic(runtime.NewGoError(err))
		}
	})

	// ── kill() ───────────────────────────────────────────
	_ = obj.Set("kill", func() {
		if err := cs.Kill(); err != nil {
			panic(runtime.NewGoError(err))
		}
	})

	// ── pause() ─────────────────────────────────────────
	// T059: Send SIGSTOP to suspend the child process.
	_ = obj.Set("pause", func() {
		if err := cs.Pause(); err != nil {
			panic(runtime.NewGoError(err))
		}
	})

	// ── resume() ────────────────────────────────────────
	// T059: Send SIGCONT to resume a paused child process.
	_ = obj.Set("resume", func() {
		if err := cs.Resume(); err != nil {
			panic(runtime.NewGoError(err))
		}
	})

	// ── isPaused() → boolean ────────────────────────────
	// T059: Check if the child process is currently paused.
	_ = obj.Set("isPaused", func() bool {
		return cs.IsPaused()
	})

	// ── resize(rows, cols) ───────────────────────────────
	_ = obj.Set("resize", func(rows, cols int) {
		if err := cs.Resize(rows, cols); err != nil {
			panic(runtime.NewGoError(err))
		}
	})

	// ── wait() → { code, error? } ───────────────────────
	// BLOCKING: waits until child process exits and output is drained.
	_ = obj.Set("wait", func() map[string]any {
		code, err := cs.Wait()
		result := map[string]any{"code": code}
		if err != nil {
			result["error"] = err.Error()
		}
		return result
	})

	// ── sendEOF() ────────────────────────────────────────
	_ = obj.Set("sendEOF", func() {
		if err := cs.SendEOF(); err != nil {
			panic(runtime.NewGoError(err))
		}
	})

	// ── close() ──────────────────────────────────────────
	_ = obj.Set("close", func() {
		if err := cs.Close(); err != nil {
			panic(runtime.NewGoError(err))
		}
	})

	// ── pid() → number ──────────────────────────────────
	_ = obj.Set("pid", func() int {
		return cs.Pid()
	})

	// ── exitCode() → number ──────────────────────────────
	_ = obj.Set("exitCode", func() int {
		return cs.ExitCode()
	})

	// ── isDone() → boolean ───────────────────────────────
	// Non-blocking check: true if the child has exited and output is drained.
	_ = obj.Set("isDone", func() bool {
		select {
		case <-cs.Done():
			return true
		default:
			return false
		}
	})

	// ── passthrough(toggleKey?) → { reason, error? } ────
	// BLOCKING: enters raw passthrough mode, returns when user toggles or child exits.
	// Uses os.Stdin/os.Stdout and the real terminal state. The caller (BubbleTea's
	// toggleModel) must have already released the terminal before calling this.
	_ = obj.Set("passthrough", func(call goja.FunctionCall) map[string]any {
		toggleKey := byte(parent.DefaultToggleKey)
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			cfgObj := call.Argument(0).ToObject(runtime)
			if cfgObj != nil {
				if v := cfgObj.Get("toggleKey"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					toggleKey = byte(v.ToInteger())
				}
			}
		}

		termFd := int(os.Stdin.Fd())
		reason, err := cs.Passthrough(ctx, parent.PassthroughConfig{
			Stdin:         os.Stdin,
			Stdout:        os.Stdout,
			TermFd:        termFd,
			ToggleKey:     toggleKey,
			TermState:     ptyio.RealTermState{},
			BlockingGuard: parent.DefaultBlockingGuard(),
		})

		result := map[string]any{
			"reason": exitReasonString(reason),
		}
		if err != nil {
			result["error"] = err.Error()
		}
		return result
	})

	return obj
}

// wrapInteractiveSession wraps a [parent.InteractiveSession] into a Goja
// object with JavaScript-callable methods. This is the shared base for both
// mux [MuxSession] wrappers (via [WrapMux].session()) and [CaptureSession]
// wrappers (via [WrapCaptureSession]).
//
// Exported methods (9 total):
//
//	output, screen, target, setTarget, resize, write, close, isRunning, isDone.
//
// CaptureSession wrappers override close, resize, isRunning, and isDone
// with implementation-specific versions and add capture-only methods
// (start, interrupt, kill, pause, resume, isPaused, wait, sendEOF, pid,
// exitCode).
func wrapInteractiveSession(runtime *goja.Runtime, session parent.InteractiveSession, defaultKind parent.SessionKind) goja.Value {
	obj := runtime.NewObject()

	// Store the Go session for later retrieval by unwrapInteractiveSession.
	// Non-enumerable so it doesn't appear in Object.keys().
	_ = obj.DefineDataProperty("_goSession", runtime.ToValue(session),
		goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)

	_ = obj.Set("output", func() string {
		return session.Output()
	})

	_ = obj.Set("target", func() map[string]any {
		target := session.Target()
		return map[string]any{
			"id":   target.ID,
			"name": target.Name,
			"kind": target.Kind.String(),
		}
	})

	_ = obj.Set("screen", func() string {
		return session.Screen()
	})

	_ = obj.Set("setTarget", func(target map[string]any) {
		if target == nil {
			panic(runtime.NewTypeError("setTarget: target object is required"))
		}
		session.SetTarget(targetFromJS(target, defaultKind))
	})

	_ = obj.Set("resize", func(rows, cols int) {
		if err := session.Resize(rows, cols); err != nil {
			panic(runtime.NewGoError(err))
		}
	})

	_ = obj.Set("write", func(data string) {
		if _, err := session.Write([]byte(data)); err != nil {
			panic(runtime.NewGoError(err))
		}
	})

	_ = obj.Set("close", func() {
		if err := session.Close(); err != nil {
			panic(runtime.NewGoError(err))
		}
	})

	_ = obj.Set("isRunning", func() bool {
		return session.IsRunning()
	})

	_ = obj.Set("isDone", func() bool {
		select {
		case <-session.Done():
			return true
		default:
			return false
		}
	})

	return obj
}

func targetFromJS(raw map[string]any, defaultKind parent.SessionKind) parent.SessionTarget {
	target := parent.SessionTarget{}
	if v, ok := raw["id"]; ok && v != nil {
		target.ID = fmt.Sprint(v)
	}
	if v, ok := raw["name"]; ok && v != nil {
		target.Name = fmt.Sprint(v)
	}
	if v, ok := raw["kind"]; ok && v != nil {
		target.Kind = parent.SessionKind(fmt.Sprint(v))
	}
	if target.Kind == parent.SessionKindUnknown {
		target.Kind = defaultKind
	}
	return target
}

// unwrapInteractiveSession retrieves the Go InteractiveSession stored on a
// JS wrapper object by wrapInteractiveSession. Returns nil if the object
// does not contain a _goSession property.
func unwrapInteractiveSession(obj *goja.Object) parent.InteractiveSession {
	v := obj.Get("_goSession")
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	session, _ := v.Export().(parent.InteractiveSession)
	return session
}

// newSessionManager creates a [parent.SessionManager] from an optional JS
// options object and returns a wrapped JS object.
//
// JS signature:
//
//	termmux.newSessionManager({ rows?: number, cols?: number, requestBuffer?: number })
func newSessionManager(ctx context.Context, runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
	var opts []parent.ManagerOption

	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		cfgObj := call.Argument(0).ToObject(runtime)
		if cfgObj != nil {
			if v := cfgObj.Get("rows"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				rows := int(v.ToInteger())
				cols := 80
				if c := cfgObj.Get("cols"); c != nil && !goja.IsUndefined(c) && !goja.IsNull(c) {
					cols = int(c.ToInteger())
				}
				opts = append(opts, parent.WithTermSize(rows, cols))
			} else if v := cfgObj.Get("cols"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				opts = append(opts, parent.WithTermSize(24, int(v.ToInteger())))
			}
			if v := cfgObj.Get("requestBuffer"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				opts = append(opts, parent.WithRequestBuffer(int(v.ToInteger())))
			}
			if v := cfgObj.Get("outputBuffer"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				opts = append(opts, parent.WithMergedOutputBuffer(int(v.ToInteger())))
			}
		}
	}

	mgr := parent.NewSessionManager(opts...)
	return WrapSessionManager(ctx, runtime, mgr)
}

// WrapSessionManager wraps a [parent.SessionManager] into a Goja object with
// JavaScript-callable methods. Exported so callers can create a Go-side
// SessionManager and expose it through the same interface.
func WrapSessionManager(ctx context.Context, runtime *goja.Runtime, mgr *parent.SessionManager) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("run", func() {
		go func() {
			_ = mgr.Run(ctx)
		}()
	})

	_ = obj.Set("close", func() {
		mgr.Close()
	})

	// subscribe(bufSize?) → { id, channel }
	// Returns a subscriber ID and begins collecting events. Use
	// pollEvents to drain the channel from JS.
	_ = obj.Set("subscribe", func(call goja.FunctionCall) goja.Value {
		bufSize := 64
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			bufSize = int(call.Argument(0).ToInteger())
		}
		id, ch := mgr.Subscribe(bufSize)

		result := runtime.NewObject()
		_ = result.Set("id", id)

		// pollEvents() → Event[]
		_ = result.Set("pollEvents", func() goja.Value {
			events := make([]map[string]any, 0)
			for {
				select {
				case evt, ok := <-ch:
					if !ok {
						return runtime.ToValue(events)
					}
					events = append(events, map[string]any{
						"kind":      evt.Kind.String(),
						"sessionId": uint64(evt.SessionID),
						"time":      evt.Time.UnixMilli(),
					})
				default:
					return runtime.ToValue(events)
				}
			}
		})

		return result
	})

	// unsubscribe(id) → boolean
	_ = obj.Set("unsubscribe", func(id int) bool {
		return mgr.Unsubscribe(id)
	})

	// register(session, {name?, kind?, id?}) → sessionID (number)
	_ = obj.Set("register", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(runtime.NewTypeError("register requires at least 1 argument (session)"))
		}

		// Extract the InteractiveSession from the JS wrapper.
		sessionObj := call.Argument(0).ToObject(runtime)
		session := unwrapInteractiveSession(sessionObj)
		if session == nil {
			panic(runtime.NewTypeError("register: first argument must be an InteractiveSession wrapper"))
		}

		var target parent.SessionTarget
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			tObj := call.Argument(1).ToObject(runtime)
			if v := tObj.Get("name"); v != nil && !goja.IsUndefined(v) {
				target.Name = v.String()
			}
			if v := tObj.Get("kind"); v != nil && !goja.IsUndefined(v) {
				target.Kind = parent.SessionKind(v.String())
			}
			if v := tObj.Get("id"); v != nil && !goja.IsUndefined(v) {
				target.ID = v.String()
			}
		}

		id, err := mgr.Register(session, target)
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		return runtime.ToValue(uint64(id))
	})

	// unregister(id) → void
	_ = obj.Set("unregister", func(id uint64) {
		if err := mgr.Unregister(parent.SessionID(id)); err != nil {
			panic(runtime.NewGoError(err))
		}
	})

	// activate(id) → void
	_ = obj.Set("activate", func(id uint64) {
		if err := mgr.Activate(parent.SessionID(id)); err != nil {
			panic(runtime.NewGoError(err))
		}
	})

	// input(data) → void
	_ = obj.Set("input", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(runtime.NewTypeError("input requires 1 argument (data)"))
		}
		data := []byte(call.Argument(0).String())
		if err := mgr.Input(data); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// resize(rows, cols) → void
	_ = obj.Set("resize", func(rows, cols int) {
		if err := mgr.Resize(rows, cols); err != nil {
			panic(runtime.NewGoError(err))
		}
	})

	// snapshot(id) → {gen, plainText, ansi, fullScreen, rows, cols, timestamp} | null
	_ = obj.Set("snapshot", func(id uint64) goja.Value {
		snap := mgr.Snapshot(parent.SessionID(id))
		if snap == nil {
			return goja.Null()
		}
		result := runtime.NewObject()
		_ = result.Set("gen", snap.Gen)
		_ = result.Set("plainText", snap.PlainText)
		_ = result.Set("ansi", snap.ANSI)
		_ = result.Set("fullScreen", snap.FullScreen)
		_ = result.Set("rows", snap.Rows)
		_ = result.Set("cols", snap.Cols)
		_ = result.Set("timestamp", snap.Timestamp.UnixMilli())
		return result
	})

	// activeID() → number
	_ = obj.Set("activeID", func() uint64 {
		return uint64(mgr.ActiveID())
	})

	// sessions() → [{id, target: {name, kind, id}, state, isActive}]
	_ = obj.Set("sessions", func() goja.Value {
		infos := mgr.Sessions()
		result := make([]map[string]any, len(infos))
		for i, info := range infos {
			result[i] = map[string]any{
				"id": uint64(info.ID),
				"target": map[string]any{
					"name": info.Target.Name,
					"kind": string(info.Target.Kind),
					"id":   info.Target.ID,
				},
				"state":    info.State.String(),
				"isActive": info.IsActive,
			}
		}
		return runtime.ToValue(result)
	})

	return obj
}
