// Package termmux provides JavaScript bindings for the terminal multiplexer,
// registered as the "osm:termmux" native module. It wraps
// [github.com/joeycumines/one-shot-man/internal/termmux] to expose pane
// management, passthrough control, and configuration to Goja scripts.
package termmux

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/dop251/goja"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
	"github.com/joeycumines/one-shot-man/internal/termmux/ptyio"
	"github.com/joeycumines/one-shot-man/internal/termmux/statusbar"
)

// Event name constants exposed to JS.
const (
	EventExit           = "exit"
	EventResize         = "resize"
	EventFocus          = "focus"
	EventBell           = "bell"
	EventOutput         = "output"
	EventRegistered     = "registered"
	EventActivated      = "activated"
	EventClosed         = "closed"
	EventTerminalResize = "terminal-resize"
)

// validEvents is the set of event names accepted by on().
var validEvents = map[string]bool{
	EventExit:           true,
	EventResize:         true,
	EventFocus:          true,
	EventBell:           true,
	EventOutput:         true,
	EventRegistered:     true,
	EventActivated:      true,
	EventClosed:         true,
	EventTerminalResize: true,
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
		_ = exports.Set("EVENT_REGISTERED", EventRegistered)
		_ = exports.Set("EVENT_ACTIVATED", EventActivated)
		_ = exports.Set("EVENT_CLOSED", EventClosed)
		_ = exports.Set("EVENT_TERMINAL_RESIZE", EventTerminalResize)

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
// AUDIT (T004/T059/T10/T49/T56): All 17 methods verified present and type-correct:
//
//	start, interrupt, kill, pause, resume, isPaused,
//	resize, wait, write, sendEOF, close, pid, exitCode, isDone,
//	passthrough, reader, readAvailable.
//
// Task 56: target, setTarget, isRunning removed — all JS call sites
// use SessionManager wrappers (tuiMux.session()) instead.
//
// The 4 methods called by runVerifyBranch/pollVerifySession (isDone,
// exitCode, close, interrupt) are confirmed bound with correct signatures
// via module_capture_test.go. Screen reads go through SessionManager
// snapshots via the _buildVerifyProxy in JS (Task 48).
func WrapCaptureSession(ctx context.Context, runtime *goja.Runtime, cs *parent.CaptureSession) goja.Value {
	obj := wrapInteractiveSession(runtime, cs, parent.SessionKindCapture).ToObject(runtime)

	// ── CaptureSession-specific methods (not part of InteractiveSession) ──

	// Task 49: Output() and Screen() removed from CaptureSession.
	// Screen reads now go through SessionManager snapshots via the
	// _buildVerifyProxy in JS (Task 48).
	//
	// Task 56: target(), setTarget(), isRunning() removed from
	// CaptureSession wrapper. All JS call sites use SessionManager
	// wrappers (tuiMux.session()) for these operations.

	// ── start() ──────────────────────────────────────────
	_ = obj.Set("start", func() {
		if err := cs.Start(ctx); err != nil {
			panic(runtime.NewGoError(err))
		}
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
// [SessionManager] session wrappers (via [WrapSessionManager].session())
// and [CaptureSession] wrappers (via [WrapCaptureSession]).
//
// Exported methods (6 total, matching the trimmed InteractiveSession interface):
//
//	resize, write, close, isDone, reader, readAvailable.
//
// CaptureSession wrappers add concrete-type-specific methods
// (start, interrupt, kill, pause, resume, isPaused, wait, sendEOF,
// pid, exitCode, passthrough).
func wrapInteractiveSession(runtime *goja.Runtime, session parent.InteractiveSession, defaultKind parent.SessionKind) goja.Value {
	obj := runtime.NewObject()

	// Store the Go session for later retrieval by unwrapInteractiveSession.
	// Non-enumerable so it doesn't appear in Object.keys().
	_ = obj.DefineDataProperty("_goSession", runtime.ToValue(session),
		goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)

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

	_ = obj.Set("isDone", func() bool {
		select {
		case <-session.Done():
			return true
		default:
			return false
		}
	})

	// reader() returns a Go channel adapter: call reader() to get the next
	// chunk (blocking), or null when the channel is closed.
	_ = obj.Set("reader", func() goja.Value {
		ch := session.Reader()
		if ch == nil {
			return goja.Null()
		}
		data, ok := <-ch
		if !ok {
			return goja.Null()
		}
		return runtime.ToValue(string(data))
	})

	// readAvailable() drains all currently-buffered chunks from the Reader()
	// channel without blocking. Returns an empty string when nothing is
	// buffered and null when the channel is closed. Useful for polling loops
	// in synchronous JS contexts (Goja has no setTimeout).
	_ = obj.Set("readAvailable", func() goja.Value {
		ch := session.Reader()
		if ch == nil {
			return goja.Null()
		}
		var buf []byte
		for {
			select {
			case data, ok := <-ch:
				if !ok {
					if len(buf) > 0 {
						return runtime.ToValue(string(buf))
					}
					return goja.Null()
				}
				buf = append(buf, data...)
			default:
				return runtime.ToValue(string(buf))
			}
		}
	})

	return obj
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
	return WrapSessionManager(ctx, runtime, mgr, os.Stdin, os.Stdout, -1)
}

// WrapSessionManager wraps a [parent.SessionManager] into a Goja object with
// JavaScript-callable methods. Exported so callers can create a Go-side
// SessionManager and expose it through the same interface.
//
// The stdin/stdout/termFd parameters provide terminal I/O for passthrough
// mode. Pass os.Stdin, os.Stdout, and -1 (or int(os.Stdin.Fd())) as defaults.
func WrapSessionManager(ctx context.Context, runtime *goja.Runtime, mgr *parent.SessionManager, stdin io.Reader, stdout io.Writer, termFd int) goja.Value {
	obj := runtime.NewObject()

	// ── Closure state for Mux-equivalent convenience methods ──
	events := newMuxEvents()
	sb := statusbar.New(stdout)
	var toggleKey byte = parent.DefaultToggleKey
	sb.SetToggleKey(toggleKey)
	var statusEnabled bool
	var resizeFn func(rows, cols uint16) error
	var activeSessionTarget parent.SessionTarget
	var swappedOnce bool

	// ── EventBus → muxEvents bridge ──────────────────────
	// Subscribe to the SessionManager's EventBus and forward all event
	// kinds into the JS-side muxEvents queue. The goroutine exits when
	// ctx is cancelled.
	busID, busCh := mgr.Subscribe(64)
	go func() {
		defer mgr.Unsubscribe(busID)
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-busCh:
				if !ok {
					return
				}
				sid := uint64(evt.SessionID)
				switch evt.Kind {
				case parent.EventSessionRegistered:
					events.queue(EventRegistered, map[string]any{
						"sessionId": sid,
					})
				case parent.EventSessionActivated:
					events.queue(EventActivated, map[string]any{
						"sessionId": sid,
					})
				case parent.EventSessionExited:
					events.queue(EventExit, map[string]any{
						"pane":      "claude",
						"sessionId": sid,
					})
				case parent.EventSessionClosed:
					events.queue(EventClosed, map[string]any{
						"sessionId": sid,
					})
				case parent.EventResize:
					resizeData := map[string]any{}
					if dims, ok := evt.Data.([2]int); ok {
						resizeData["rows"] = dims[0]
						resizeData["cols"] = dims[1]
					}
					events.queue(EventTerminalResize, resizeData)
				case parent.EventBell:
					events.queue(EventBell, map[string]any{
						"pane":      "claude",
						"sessionId": sid,
					})
				case parent.EventSessionOutput:
					data := map[string]any{
						"pane":      "claude",
						"sessionId": sid,
					}
					if raw, ok := evt.Data.([]byte); ok {
						data["chunk"] = string(raw)
					}
					events.queue(EventOutput, data)
				}
			}
		}
	}()

	_ = obj.Set("run", func() {
		go func() {
			_ = mgr.Run(ctx)
		}()
	})

	// started() → Promise-like blocking call. Waits for Run to begin
	// processing requests. Returns true if the worker started, false
	// if it shut down before starting.
	_ = obj.Set("started", func() bool {
		select {
		case <-mgr.Started():
			return true
		case <-ctx.Done():
			return false
		}
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
			evts := make([]map[string]any, 0)
			for {
				select {
				case evt, ok := <-ch:
					if !ok {
						return runtime.ToValue(evts)
					}
					evts = append(evts, map[string]any{
						"kind":      evt.Kind.String(),
						"sessionId": uint64(evt.SessionID),
						"time":      evt.Time.UnixMilli(),
					})
				default:
					return runtime.ToValue(evts)
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

	// termSize() → {rows, cols}
	_ = obj.Set("termSize", func() goja.Value {
		rows, cols := mgr.TermSize()
		result := runtime.NewObject()
		_ = result.Set("rows", rows)
		_ = result.Set("cols", cols)
		return result
	})

	// snapshot(id) → {gen, plainText, ansi, fullScreen, rows, cols, cursorRow, cursorCol, timestamp} | null
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
		_ = result.Set("cursorRow", snap.CursorRow)
		_ = result.Set("cursorCol", snap.CursorCol)
		_ = result.Set("timestamp", snap.Timestamp.UnixMilli())
		return result
	})

	// activeID() → number
	_ = obj.Set("activeID", func() uint64 {
		return uint64(mgr.ActiveID())
	})

	// isDone(id) → bool
	// Returns true when the session identified by id has exited, been
	// closed, or was never registered. Callers that hold a pinned
	// SessionID can use this instead of session().isDone(), which reads
	// from the mutable ActiveID.
	_ = obj.Set("isDone", func(id uint64) bool {
		for _, info := range mgr.Sessions() {
			if info.ID == parent.SessionID(id) {
				return info.State == parent.SessionExited || info.State == parent.SessionClosed
			}
		}
		return true // not found → treat as done
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

	// eventsDropped() → number
	// Returns the cumulative count of events that could not be delivered
	// to at least one subscriber because its channel buffer was full.
	_ = obj.Set("eventsDropped", func() int64 {
		return mgr.EventsDropped()
	})

	// passthrough({stdin?, stdout?, termFd?, toggleKey?, statusBar?, restoreScreen?, resizeFn?})
	// → {reason: string, error?: string}
	// Enters passthrough mode for the active session. Blocks until
	// the user presses the toggle key, the session exits, or the
	// context is cancelled.
	_ = obj.Set("passthrough", func(call goja.FunctionCall) goja.Value {
		cfg := parent.PassthroughConfig{
			TermFd:        -1,
			ToggleKey:     0x1D, // Ctrl+]
			TermState:     ptyio.RealTermState{},
			BlockingGuard: parent.DefaultBlockingGuard(),
		}

		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			opts := call.Argument(0).ToObject(runtime)

			if v := opts.Get("stdin"); v != nil && !goja.IsUndefined(v) {
				if r, ok := v.Export().(io.Reader); ok {
					cfg.Stdin = r
				}
			}
			if v := opts.Get("stdout"); v != nil && !goja.IsUndefined(v) {
				if w, ok := v.Export().(io.Writer); ok {
					cfg.Stdout = w
				}
			}
			if v := opts.Get("termFd"); v != nil && !goja.IsUndefined(v) {
				cfg.TermFd = int(v.ToInteger())
			}
			if v := opts.Get("toggleKey"); v != nil && !goja.IsUndefined(v) {
				cfg.ToggleKey = byte(v.ToInteger())
			}
			if v := opts.Get("restoreScreen"); v != nil && !goja.IsUndefined(v) {
				cfg.RestoreScreen = v.ToBoolean()
			}
			if v := opts.Get("statusBar"); v != nil && !goja.IsUndefined(v) {
				if ssb, ok := v.Export().(*statusbar.StatusBar); ok {
					cfg.StatusBar = ssb
				}
			}
			if v := opts.Get("resizeFn"); v != nil && !goja.IsUndefined(v) {
				if fn, ok := goja.AssertFunction(v); ok {
					cfg.ResizeFn = func(rows, cols uint16) error {
						_, err := fn(goja.Undefined(), runtime.ToValue(rows), runtime.ToValue(cols))
						if err != nil {
							return fmt.Errorf("resizeFn: %w", err)
						}
						return nil
					}
				}
			}
		}

		// Default stdin/stdout to os.Stdin/os.Stdout if not provided.
		if cfg.Stdin == nil {
			cfg.Stdin = os.Stdin
		}
		if cfg.Stdout == nil {
			cfg.Stdout = os.Stdout
		}

		reason, err := mgr.Passthrough(ctx, cfg)
		result := runtime.NewObject()
		_ = result.Set("reason", reason.String())
		if err != nil {
			_ = result.Set("error", err.Error())
		}
		return result
	})

	// ── Mux-equivalent convenience methods ───────────────

	// ── attach(handle) ───────────────────────────────────
	// Accepts InteractiveSession wrappers, AgentHandle (map with _goHandle),
	// StringIO, or raw InteractiveSession. Registers and activates the session.
	// If a session is already active, unregisters it first then retries once.
	_ = obj.Set("attach", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("attach: handle argument is required"))
		}
		raw := call.Argument(0).Export()

		var session parent.InteractiveSession

		// Try to extract _goSession from a JS wrapper object first.
		if jsObj := call.Argument(0).ToObject(runtime); jsObj != nil {
			if s := unwrapInteractiveSession(jsObj); s != nil {
				session = s
			}
		}

		// Check for map with _goHandle.
		if session == nil {
			if m, ok := raw.(map[string]any); ok {
				if goHandle, exists := m["_goHandle"]; exists && goHandle != nil {
					switch h := goHandle.(type) {
					case parent.StringIO:
						sio := parent.NewStringIOSession(h)
						sio.Start()
						session = sio
					case parent.InteractiveSession:
						session = h
					}
				}
			}
		}

		// Check raw directly.
		if session == nil {
			switch h := raw.(type) {
			case parent.StringIO:
				sio := parent.NewStringIOSession(h)
				sio.Start()
				session = sio
			case parent.InteractiveSession:
				session = h
			}
		}

		if session == nil {
			panic(runtime.NewTypeError("attach: argument must be an InteractiveSession, StringIO, or wrapped AgentHandle"))
		}

		id, err := mgr.Register(session, activeSessionTarget)
		if err != nil {
			// Auto-detach: if there's an active session, unregister it and retry.
			if activeID := mgr.ActiveID(); activeID != 0 {
				_ = mgr.Unregister(activeID)
				id, err = mgr.Register(session, activeSessionTarget)
			}
			if err != nil {
				panic(runtime.NewGoError(err))
			}
		}
		if activateErr := mgr.Activate(id); activateErr != nil {
			panic(runtime.NewGoError(activateErr))
		}
		return runtime.ToValue(uint64(id))
	})

	// ── detach() ─────────────────────────────────────────
	// Unregisters the active session. Idempotent — safe to call with
	// no active session.
	_ = obj.Set("detach", func() {
		if id := mgr.ActiveID(); id != 0 {
			_ = mgr.Unregister(id)
		}
	})

	// ── hasChild() → boolean ─────────────────────────────
	_ = obj.Set("hasChild", func() bool {
		return mgr.ActiveID() != 0
	})

	// ── switchTo() → { reason, error?, childOutput? } ────
	// BLOCKING: enters raw passthrough mode, returns when user toggles or child exits.
	// Emits "focus" event on entry (side: "claude") and exit (side: "osm").
	// Emits "exit" event with the passthrough result.
	// Drains pending async events before returning.
	_ = obj.Set("switchTo", func() goja.Value {
		// Guard: no child attached.
		if mgr.ActiveID() == 0 {
			return goja.Undefined()
		}

		// Focus → claude before entering passthrough.
		events.emit(runtime, EventFocus, map[string]any{
			"side": "claude", "action": "enter",
		})

		// Build PassthroughConfig from stored state.
		cfg := parent.PassthroughConfig{
			Stdin:         stdin,
			Stdout:        stdout,
			TermFd:        termFd,
			ToggleKey:     toggleKey,
			TermState:     ptyio.RealTermState{},
			BlockingGuard: parent.DefaultBlockingGuard(),
			RestoreScreen: swappedOnce,
		}
		if statusEnabled {
			cfg.StatusBar = sb
		}
		if resizeFn != nil {
			cfg.ResizeFn = resizeFn
		}

		reason, err := mgr.Passthrough(ctx, cfg)
		swappedOnce = true

		// Focus → osm after exiting passthrough.
		events.emit(runtime, EventFocus, map[string]any{
			"side": "osm", "action": "return",
		})

		result := map[string]any{
			"reason": exitReasonString(reason),
		}
		if err != nil {
			result["error"] = err.Error()
		}

		// Include childOutput (plain text snapshot) on exit.
		if id := mgr.ActiveID(); id != 0 {
			if snap := mgr.Snapshot(id); snap != nil {
				result["childOutput"] = snap.PlainText
			}
		}

		// Emit exit event and drain pending async events.
		events.emit(runtime, EventExit, map[string]any{
			"reason": exitReasonString(reason),
			"pane":   "claude",
		})
		events.drain(runtime)

		return runtime.ToValue(result)
	})

	// ── screenshot() → string ────────────────────────────
	// Returns plain-text VTerm buffer content for the active session.
	_ = obj.Set("screenshot", func() string {
		id := mgr.ActiveID()
		if id == 0 {
			return ""
		}
		snap := mgr.Snapshot(id)
		if snap == nil {
			return ""
		}
		return snap.PlainText
	})

	// ── childScreen() → string ───────────────────────────
	// Returns the VTerm buffer as ANSI escape-sequence output for the
	// active session.
	_ = obj.Set("childScreen", func() string {
		id := mgr.ActiveID()
		if id == 0 {
			return ""
		}
		snap := mgr.Snapshot(id)
		if snap == nil {
			return ""
		}
		return snap.ANSI
	})

	// ── writeToChild(data) → number ──────────────────────
	// Sends raw bytes to the active session's stdin. Returns bytes written.
	// Throws on error (consistent with session().write()).
	_ = obj.Set("writeToChild", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("writeToChild: data argument is required"))
		}
		data := []byte(call.Argument(0).String())
		if err := mgr.Input(data); err != nil {
			panic(runtime.NewGoError(err))
		}
		return runtime.ToValue(len(data))
	})

	// ── session() → convenience wrapper ──────────────────
	// Returns an object with isRunning, isDone, output, screen, target,
	// setTarget, write, and resize for the active session.
	_ = obj.Set("session", func() goja.Value {
		sessionObj := runtime.NewObject()

		_ = sessionObj.Set("isRunning", func() bool {
			return mgr.ActiveID() != 0
		})

		// isDone() → true when: (a) no child was ever attached, or
		// (b) the active session has exited / been closed.
		_ = sessionObj.Set("isDone", func() bool {
			id := mgr.ActiveID()
			if id == 0 {
				return true
			}
			for _, info := range mgr.Sessions() {
				if info.ID == id {
					return info.State == parent.SessionExited || info.State == parent.SessionClosed
				}
			}
			return true // session not found — treat as done
		})

		_ = sessionObj.Set("output", func() string {
			id := mgr.ActiveID()
			if id == 0 {
				return ""
			}
			snap := mgr.Snapshot(id)
			if snap == nil {
				return ""
			}
			return snap.PlainText
		})

		_ = sessionObj.Set("screen", func() string {
			id := mgr.ActiveID()
			if id == 0 {
				return ""
			}
			snap := mgr.Snapshot(id)
			if snap == nil {
				return ""
			}
			return snap.ANSI
		})

		_ = sessionObj.Set("target", func() goja.Value {
			result := runtime.NewObject()
			_ = result.Set("id", activeSessionTarget.ID)
			_ = result.Set("name", activeSessionTarget.Name)
			_ = result.Set("kind", string(activeSessionTarget.Kind))
			return result
		})

		_ = sessionObj.Set("setTarget", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
				panic(runtime.NewTypeError("setTarget: target object is required"))
			}
			tObj := call.Argument(0).ToObject(runtime)
			if v := tObj.Get("name"); v != nil && !goja.IsUndefined(v) {
				activeSessionTarget.Name = v.String()
			}
			if v := tObj.Get("kind"); v != nil && !goja.IsUndefined(v) {
				activeSessionTarget.Kind = parent.SessionKind(v.String())
			}
			if v := tObj.Get("id"); v != nil && !goja.IsUndefined(v) {
				activeSessionTarget.ID = v.String()
			}
			return goja.Undefined()
		})

		// write(data) — sends bytes to the active session's PTY via
		// SessionManager.Input. Panics on error (Go error → JS exception)
		// to match the wrapInteractiveSession.write contract.
		_ = sessionObj.Set("write", func(data string) {
			if err := mgr.Input([]byte(data)); err != nil {
				panic(runtime.NewGoError(err))
			}
		})

		// resize(rows, cols) — broadcasts new dimensions to all sessions
		// via SessionManager.Resize. Panics on error (Go error → JS
		// exception) to match the wrapInteractiveSession.resize contract.
		_ = sessionObj.Set("resize", func(rows, cols int) {
			if err := mgr.Resize(rows, cols); err != nil {
				panic(runtime.NewGoError(err))
			}
		})

		return sessionObj
	})

	// ── lastActivityMs() → int64 ─────────────────────────
	// Returns milliseconds since the last snapshot update, or -1 if no
	// active session or snapshot.
	_ = obj.Set("lastActivityMs", func() int64 {
		id := mgr.ActiveID()
		if id == 0 {
			return -1
		}
		snap := mgr.Snapshot(id)
		if snap == nil || snap.Timestamp.IsZero() {
			return -1
		}
		return time.Since(snap.Timestamp).Milliseconds()
	})

	// ── setStatus(text) ──────────────────────────────────
	_ = obj.Set("setStatus", func(s string) {
		sb.SetStatus(s)
	})

	// ── setToggleKey(keyByte) ────────────────────────────
	_ = obj.Set("setToggleKey", func(k int) {
		toggleKey = byte(k)
		sb.SetToggleKey(toggleKey)
	})

	// ── setStatusEnabled(bool) ───────────────────────────
	_ = obj.Set("setStatusEnabled", func(b bool) {
		statusEnabled = b
	})

	// ── setResizeFunc(fn) ────────────────────────────────
	// Installs user resize handler AND queues "resize" events for listeners.
	_ = obj.Set("setResizeFunc", func(fn func(int, int)) {
		resizeFn = func(rows, cols uint16) error {
			fn(int(rows), int(cols))
			events.queue(EventResize, map[string]any{
				"rows": int(rows),
				"cols": int(cols),
			})
			return nil
		}
	})

	// ── on(event, callback) → id ─────────────────────────
	// Registers a listener for an event type. Returns a numeric ID for off().
	// Supported events: exit, resize, focus, bell, output, registered,
	// activated, closed, terminal-resize.
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
	// Drains pending async events and delivers them to registered listeners.
	// Returns the count of events delivered.
	_ = obj.Set("pollEvents", func() int {
		return events.drain(runtime)
	})

	// ── activeSide() → "osm" ─────────────────────────────
	// Passthrough is blocking so when JS can ask, it's always "osm".
	_ = obj.Set("activeSide", func() string {
		return "osm"
	})

	// ── fromModel(model, opts?) ──────────────────────────
	// Wraps a BubbleTea model with termmux toggle key integration.
	// Returns { model, options: { altScreen, toggleKey, onToggle } }.
	_ = obj.Set("fromModel", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(runtime.NewTypeError("fromModel requires a model argument"))
		}
		model := call.Argument(0)

		// Parse optional config overrides.
		altScreen := true
		toggleKeyByte := int(toggleKey)
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

		// onToggle calls switchTo() — blocks during passthrough.
		_ = runOpts.Set("onToggle", func(fc goja.FunctionCall) goja.Value {
			if mgr.ActiveID() == 0 {
				return goja.Undefined()
			}

			// Emit focus event: entering Claude's terminal.
			events.emit(runtime, EventFocus, map[string]any{
				"side": "claude", "action": "enter",
			})

			// Build PassthroughConfig from stored state.
			cfg := parent.PassthroughConfig{
				Stdin:         stdin,
				Stdout:        stdout,
				TermFd:        termFd,
				ToggleKey:     byte(toggleKeyByte),
				TermState:     ptyio.RealTermState{},
				BlockingGuard: parent.DefaultBlockingGuard(),
				RestoreScreen: swappedOnce,
			}
			if statusEnabled {
				cfg.StatusBar = sb
			}
			if resizeFn != nil {
				cfg.ResizeFn = resizeFn
			}

			reason, err := mgr.Passthrough(ctx, cfg)
			swappedOnce = true

			res := map[string]any{
				"reason": exitReasonString(reason),
			}
			if err != nil {
				res["error"] = err.Error()
			}

			// Emit focus event: returning to osm's terminal.
			events.emit(runtime, EventFocus, map[string]any{
				"side": "osm", "action": "return",
			})

			return runtime.ToValue(res)
		})

		_ = result.Set("options", runOpts)
		return result
	})

	// ── Persistence methods ──────────────────────────────────────────
	//
	// These expose SessionManager state export, PID liveness checks,
	// and atomic save/load for session persistence across restarts.

	// exportState() → object
	// Returns a snapshot of all managed sessions with their metadata,
	// PID (if available), terminal dimensions, and restart config.
	_ = obj.Set("exportState", func(call goja.FunctionCall) goja.Value {
		state, err := mgr.ExportState()
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		return runtime.ToValue(persistedStateToJS(state))
	})

	// saveState(path) → void
	// Atomically writes the current session manager state to a JSON file.
	_ = obj.Set("saveState", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(runtime.NewTypeError("saveState: path argument is required"))
		}
		path := call.Argument(0).String()
		if path == "" {
			panic(runtime.NewTypeError("saveState: path must be non-empty"))
		}
		state, err := mgr.ExportState()
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		if err := parent.SaveManagerState(path, state); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// loadState(path) → object | null
	// Reads a previously saved state file. Returns null if not found.
	_ = obj.Set("loadState", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(runtime.NewTypeError("loadState: path argument is required"))
		}
		path := call.Argument(0).String()
		if path == "" {
			panic(runtime.NewTypeError("loadState: path must be non-empty"))
		}
		state, err := parent.LoadManagerState(path)
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		if state == nil {
			return goja.Null()
		}
		return runtime.ToValue(persistedStateToJS(state))
	})

	// removeState(path) → void
	// Deletes a state file (no-op if it doesn't exist).
	_ = obj.Set("removeState", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(runtime.NewTypeError("removeState: path argument is required"))
		}
		path := call.Argument(0).String()
		if path == "" {
			panic(runtime.NewTypeError("removeState: path must be non-empty"))
		}
		if err := parent.RemoveManagerState(path); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// processAlive(pid) → boolean
	// Checks whether a process with the given PID exists.
	_ = obj.Set("processAlive", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(runtime.NewTypeError("processAlive: pid argument is required"))
		}
		pid := int(call.Argument(0).ToInteger())
		return runtime.ToValue(parent.ProcessAlive(pid))
	})

	return obj
}

// persistedStateToJS converts a [parent.PersistedManagerState] to a plain
// map structure suitable for the Goja runtime.
func persistedStateToJS(state *parent.PersistedManagerState) map[string]any {
	sessions := make([]any, len(state.Sessions))
	for i, s := range state.Sessions {
		sess := map[string]any{
			"sessionId":  s.SessionID,
			"state":      int(s.State),
			"pid":        s.PID,
			"rows":       s.Rows,
			"cols":       s.Cols,
			"lastActive": s.LastActive.UnixMilli(),
			"target": map[string]any{
				"id":   s.Target.ID,
				"name": s.Target.Name,
				"kind": string(s.Target.Kind),
			},
		}
		if s.Command != "" {
			sess["command"] = s.Command
		}
		if len(s.Args) > 0 {
			sess["args"] = s.Args
		}
		if s.Dir != "" {
			sess["dir"] = s.Dir
		}
		if len(s.Env) > 0 {
			sess["env"] = s.Env
		}
		sessions[i] = sess
	}
	return map[string]any{
		"version":  state.Version,
		"activeId": state.ActiveID,
		"termRows": state.TermRows,
		"termCols": state.TermCols,
		"savedAt":  state.SavedAt.UnixMilli(),
		"sessions": sessions,
	}
}
