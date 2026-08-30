// Package termmux provides JavaScript bindings for the terminal multiplexer,
// registered as the "osm:termmux" native module. It wraps
// [github.com/joeycumines/one-shot-man/internal/termmux] to expose pane
// management, passthrough control, and configuration to Goja scripts.
package termmux

import (
	"context"
	"fmt"
	"image"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
	"github.com/joeycumines/one-shot-man/internal/termmux/ptyio"
	"github.com/joeycumines/one-shot-man/internal/termmux/statusbar"
	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

// managerWrapperCache avoids re-creating a wrapper for the same SessionManager
// while an event loop is running. Wrapping involves mutating the runtime via
// Object.Set, so concurrent wrapping from production scripts (which run on the
// event loop) and tests (which call runtime.RunString directly) can corrupt
// Goja's internal state. Reusing the existing wrapper is safe.
var managerWrapperCache sync.Map // key: *parent.SessionManager

type wrapperCacheEntry struct {
	obj   *goja.Object
	state *muxState
}

func toInt64(m map[string]any, key string) int64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case int32:
		return int64(n)
	default:
		return 0
	}
}

func toString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	default:
		return ""
	}
}

// copyModeSearchAdapter lets a JS function satisfy parent.ScreenSearcher.
type copyModeSearchAdapter struct {
	searchFn func(pattern string, row, col int) map[string]any
}

func (a copyModeSearchAdapter) SearchForward(pattern string, startRow, startCol int) *vt.SearchMatch {
	return a.search(pattern, startRow, startCol, true)
}

func (a copyModeSearchAdapter) SearchBackward(pattern string, startRow, startCol int) *vt.SearchMatch {
	return a.search(pattern, startRow, startCol, false)
}

func (a copyModeSearchAdapter) search(pattern string, startRow, startCol int, forward bool) *vt.SearchMatch {
	m := a.searchFn(pattern, startRow, startCol)
	if m == nil {
		return nil
	}
	found, _ := m["found"].(bool)
	if !found {
		return nil
	}
	row, _ := m["row"].(int)
	col, _ := m["col"].(int)
	return &vt.SearchMatch{
		Row: row,
		Col: col,
	}
}

func wrapSearchMatch(match *vt.SearchMatch) map[string]any {
	if match == nil {
		return map[string]any{"found": false}
	}
	return map[string]any{
		"found": true,
		"row":   match.Row,
		"col":   match.Col,
	}
}

func wrapSearchMatch1Based(match *vt.SearchMatch) map[string]any {
	if match == nil {
		return map[string]any{"found": false}
	}
	return map[string]any{
		"found": true,
		"row":   match.Row + 1,
		"col":   match.Col + 1,
	}
}

func errToStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (s *muxState) activeScreenSearcher() parent.ScreenSearcher {
	id := s.mgr.ActiveID()
	if id == 0 {
		return nil
	}
	snap := s.mgr.Snapshot(id)
	if snap == nil {
		return nil
	}
	return parent.NewScreenSnapshotSearcher(snap)
}

func (s *muxState) sessionScreenSearcher(sessionID uint64) *ScreenSearcher {
	snap := s.mgr.Snapshot(parent.SessionID(sessionID))
	if snap == nil {
		return nil
	}
	return NewScreenSearcher(snap, "")
}

// Event name constants exposed to JS.
const (
	EventExit             = "exit"
	EventResize           = "resize"
	EventFocus            = "focus"
	EventBell             = "bell"
	EventOutput           = "output"
	EventRegistered       = "registered"
	EventActivated        = "activated"
	EventClosed           = "closed"
	EventTerminalResize   = "terminal-resize"
	EventActivity         = "activity"
	EventSilence          = "silence"
	EventTitle            = "title"
	EventWorkingDirectory = "cwd"
	EventCWD              = EventWorkingDirectory
	EventClipboard        = "clipboard"
)

// isValidEventType returns true for the legacy event names supported by on().
func isValidEventType(event string) bool {
	switch event {
	case EventExit, EventResize, EventFocus, EventBell, EventOutput,
		EventRegistered, EventActivated, EventClosed, EventTerminalResize,
		EventActivity, EventSilence, EventTitle, EventWorkingDirectory,
		EventClipboard:
		return true
	default:
		return false
	}
}

// Require returns a module loader for "osm:termmux" that exposes the terminal
// multiplexer to JavaScript. The input/output parameters are optional; when nil
// the module falls back to os.Stdin/os.Stdout.
func Require(ctx context.Context, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop, input io.Reader, output io.Writer) func(*goja.Runtime, *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

		// ── Constants ────────────────────────────────────────
		_ = exports.Set("EXIT_TOGGLE", "toggle")
		_ = exports.Set("EXIT_CHILD_EXIT", "childExit")
		_ = exports.Set("EXIT_CONTEXT", "context")
		_ = exports.Set("EXIT_ERROR", "error")
		_ = exports.Set("SIDE_OSM", "osm")
		_ = exports.Set("SIDE_AGENT", "agent")
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
		_ = exports.Set("EVENT_ACTIVITY", EventActivity)
		_ = exports.Set("EVENT_SILENCE", EventSilence)
		_ = exports.Set("EVENT_TITLE", EventTitle)
		_ = exports.Set("EVENT_WORKING_DIRECTORY", EventWorkingDirectory)
		_ = exports.Set("EVENT_CWD", EventCWD)
		_ = exports.Set("EVENT_CLIPBOARD", EventClipboard)

		// ── Layout mode constants ────────────────────────────
		_ = exports.Set("LAYOUT_TILED", LayoutModeString(parent.LayoutTiled))
		_ = exports.Set("LAYOUT_STACKED", LayoutModeString(parent.LayoutStacked))
		_ = exports.Set("LAYOUT_HORIZONTAL", LayoutModeString(parent.LayoutHorizontal))
		_ = exports.Set("LAYOUT_VERTICAL", LayoutModeString(parent.LayoutVertical))
		_ = exports.Set("LAYOUT_MAIN_HORIZONTAL", LayoutModeString(parent.LayoutMainHorizontal))
		_ = exports.Set("LAYOUT_MAIN_VERTICAL", LayoutModeString(parent.LayoutMainVertical))

		// ── CaptureSession factory ───────────────────────────
		_ = exports.Set("newCaptureSession", func(call goja.FunctionCall) goja.Value {
			return newCaptureSession(ctx, adapter, loop, runtime, call)
		})

		// ── SessionManager factory (experimental) ────────────
		_ = exports.Set("newSessionManager", func(call goja.FunctionCall) goja.Value {
			return newSessionManager(ctx, adapter, loop, runtime, call)
		})

		_ = exports.Set("newBoundedSession", func(call goja.FunctionCall) goja.Value {
			return newBoundedSession(ctx, adapter, loop, runtime, nil, call)
		})

		_ = exports.Set("enableMouseForward", func(call goja.FunctionCall) goja.Value {
			return enableMouseForward(runtime, call)
		})

		_ = exports.Set("mouseDrag", func() goja.Value { return newMouseDrag(runtime) })

		_ = exports.Set("handleMouseDrag", func(call goja.FunctionCall) goja.Value {
			return handleMouseDrag(runtime, call)
		})

		_ = exports.Set("newControlRouter", func(call goja.FunctionCall) goja.Value {
			return newControlRouter(runtime, call)
		})

		_ = exports.Set("newPrefixKeyHandler", func(call goja.FunctionCall) goja.Value {
			prefix := ""
			if len(call.Arguments) > 0 && call.Argument(0) != goja.Undefined() && !goja.IsNull(call.Argument(0)) {
				prefix = call.Argument(0).String()
			}
			h := parent.NewPrefixKeyHandler(prefix)

			obj := runtime.NewObject()
			_ = obj.Set("handleKey", func(key string) goja.Value {
				handled, action := h.HandleKey(key)
				result := runtime.NewObject()
				_ = result.Set("handled", handled)
				_ = result.Set("action", action.String())
				return result
			})
			_ = obj.Set("awaiting", func() bool { return h.Awaiting() })
			_ = obj.Set("reset", func() { h.Reset() })
			_ = obj.Set("prefix", func() string { return h.Prefix() })
			_ = obj.Set("setPrefix", func(p string) { h.SetPrefix(p) })
			_ = obj.Set("setCommand", func(key string, actionName string) {
				kind := prefixActionKindFromName(actionName)
				h.SetCommand(key, kind)
			})
			_ = obj.Set("removeCommand", func(key string) { h.RemoveCommand(key) })
			_ = obj.Set("commands", func() goja.Value {
				cmds := h.Commands()
				result := runtime.NewObject()
				for k, v := range cmds {
					_ = result.Set(k, parent.PrefixAction{Kind: v}.String())
				}
				return result
			})
			return obj
		})

		// handlePrefixKey({ manager, key }) executes a prefix action directly
		// on a SessionManager and returns the result.
		_ = exports.Set("handlePrefixKey", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
				panic(runtime.NewTypeError("handlePrefixKey: options object is required"))
			}
			opts := call.Argument(0).ToObject(runtime)

			mgrObj := opts.Get("manager")
			if mgrObj == nil || goja.IsUndefined(mgrObj) || goja.IsNull(mgrObj) {
				panic(runtime.NewTypeError("handlePrefixKey: manager is required"))
			}
			mgr := UnwrapSessionManager(mgrObj.ToObject(runtime))
			if mgr == nil {
				panic(runtime.NewTypeError("handlePrefixKey: manager must be a SessionManager wrapper"))
			}

			keyVal := opts.Get("key")
			if keyVal == nil || goja.IsUndefined(keyVal) {
				panic(runtime.NewTypeError("handlePrefixKey: key is required"))
			}
			key := keyVal.String()

			d := parent.NewPrefixDispatcher(mgr, parent.NewPrefixKeyHandler(""))
			res, err := d.Handle(key)
			if err != nil {
				panic(runtime.NewGoError(err))
			}

			result := runtime.NewObject()
			_ = result.Set("action", res.Action.String())
			_ = result.Set("consumed", res.Consumed)
			_ = result.Set("description", res.Description)
			_ = result.Set("result", res.Result)
			_ = result.Set("listKeys", res.ListKeys)
			return result
		})

		// ── Input encoding utilities ────────────────────────
		// keyToTermBytes(key, appCursor?, appKeypad?) → string | null
		// When appCursor is true, arrow/home/end keys use application mode
		// sequences (SS3: ESC O{A-D/H/F) instead of CSI sequences.
		// When appKeypad is true, keypad keys use application mode sequences
		// (SS3: ESC O p–y for digits, ESC O M for enter, etc.) instead of
		// their ASCII equivalents.
		_ = exports.Set("keyToTermBytes", func(call goja.FunctionCall) goja.Value {
			key := call.Argument(0).String()
			appCursor := len(call.Arguments) > 1 && call.Argument(1).ToBoolean()
			appKeypad := len(call.Arguments) > 2 && call.Argument(2).ToBoolean()
			if s, ok := parent.KeyToTermBytes(key, appCursor, appKeypad); ok {
				return runtime.ToValue(s)
			}
			return goja.Null()
		})

		// renderMessageBar(text, row?, cols?) → string
		// Returns an ANSI sequence that draws a one-line highlighted message
		// bar at the given 1-based terminal row.
		_ = exports.Set("renderMessageBar", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(runtime.NewTypeError("renderMessageBar requires at least 1 argument (text)"))
			}
			text := call.Argument(0).String()
			row := 1
			if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
				row = int(call.Argument(1).ToInteger())
			}
			cols := 80
			if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) {
				cols = int(call.Argument(2).ToInteger())
			}
			return runtime.ToValue(parent.MessageBarLine(text, row, cols))
		})

		// mouseToSGR(event, offsetRow?, offsetCol?) → string | null
		// event: {type, button, x, y, shift?, alt?, ctrl?}
		_ = exports.Set("mouseToSGR", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(runtime.NewTypeError("mouseToSGR requires at least 1 argument (event)"))
			}
			obj := call.Argument(0).ToObject(runtime)
			ev := parent.MouseEvent{
				Type:   parent.MouseEventType(obj.Get("type").String()),
				Button: parent.MouseButton(obj.Get("button").String()),
				X:      int(obj.Get("x").ToInteger()),
				Y:      int(obj.Get("y").ToInteger()),
			}
			if v := obj.Get("shift"); v != nil && !goja.IsUndefined(v) {
				ev.Shift = v.ToBoolean()
			}
			if v := obj.Get("alt"); v != nil && !goja.IsUndefined(v) {
				ev.Alt = v.ToBoolean()
			}
			if v := obj.Get("ctrl"); v != nil && !goja.IsUndefined(v) {
				ev.Ctrl = v.ToBoolean()
			}
			var offsetRow, offsetCol int
			if len(call.Arguments) > 1 {
				offsetRow = int(call.Argument(1).ToInteger())
			}
			if len(call.Arguments) > 2 {
				offsetCol = int(call.Argument(2).ToInteger())
			}
			if s, ok := parent.MouseToSGR(ev, offsetRow, offsetCol); ok {
				return runtime.ToValue(s)
			}
			return goja.Null()
		})

		// ── Layout utilities ────────────────────────────────
		// splitLayout(config) → {compute(rows, cols, ratio) → {top, bottom}}
		_ = exports.Set("splitLayout", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(runtime.NewTypeError("splitLayout requires 1 argument (config)"))
			}
			obj := call.Argument(0).ToObject(runtime)
			layout := parent.SplitLayout{
				TotalChromeRows:      int(obj.Get("totalChromeRows").ToInteger()),
				TopPaneHeaderRows:    int(obj.Get("topPaneHeaderRows").ToInteger()),
				DividerRows:          int(obj.Get("dividerRows").ToInteger()),
				BottomPaneHeaderRows: int(obj.Get("bottomPaneHeaderRows").ToInteger()),
				LeftChromeCol:        int(obj.Get("leftChromeCol").ToInteger()),
				MinPaneRows:          int(obj.Get("minPaneRows").ToInteger()),
			}
			result := runtime.NewObject()
			_ = result.Set("compute", func(rows, cols int, ratio float64) goja.Value {
				top, bottom := layout.Compute(rows, cols, ratio)
				r := runtime.NewObject()
				_ = r.Set("top", paneGeoToJS(runtime, top))
				_ = r.Set("bottom", paneGeoToJS(runtime, bottom))
				return r
			})
			return result
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

func LayoutModeString(m parent.LayoutMode) string {
	return m.String()
}

// paneGeoToJS wraps a [parent.PaneGeometry] as a JS object with row, col,
// rows, cols fields and an offsetMouse(screenRow, screenCol) method.
func paneGeoToJS(runtime *goja.Runtime, g parent.PaneGeometry) *goja.Object {
	obj := runtime.NewObject()
	_ = obj.Set("row", g.Row)
	_ = obj.Set("col", g.Col)
	_ = obj.Set("rows", g.Rows)
	_ = obj.Set("cols", g.Cols)
	_ = obj.Set("offsetMouse", func(screenRow, screenCol int) goja.Value {
		lr, lc, inside := g.OffsetMouse(screenRow, screenCol)
		if !inside {
			return goja.Null()
		}
		r := runtime.NewObject()
		_ = r.Set("row", lr)
		_ = r.Set("col", lc)
		return r
	})
	return obj
}

// newCaptureSession creates a [parent.CaptureSession] from JS arguments and
// returns a wrapped JS object.
//
// JS signature:
//
//	termmux.newCaptureSession(command, args?, { dir?, rows?, cols?, env? }?)
func newCaptureSession(ctx context.Context, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop, runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
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
	return WrapCaptureSession(ctx, adapter, loop, runtime, cs)
}

// WrapCaptureSession wraps a [parent.CaptureSession] into a Goja object with
// JavaScript-callable methods. Exported so callers (e.g., pr_split.go) can
// create a Go-side CaptureSession and expose it through the same interface.
//
// AUDIT (T004/T059/T10/T49/T56): All 17 methods verified present and type-correct:
//
//	start, interrupt, kill, pause, resume, isPaused,
//	resize, wait, write, sendEOF, close, pid, exitCode, isDone,
//	passthrough, readAvailable.
//
// Task 56: target, setTarget, isRunning removed — all JS call sites
// use SessionManager wrappers (tuiMux.session()) instead.
//
// The 4 methods called by runVerifyBranch/pollVerifySession (isDone,
// exitCode, close, interrupt) are confirmed bound with correct signatures
// via module_capture_test.go. Screen reads go through SessionManager
// snapshots via the _buildVerifyProxy in JS (Task 48).
func WrapCaptureSession(ctx context.Context, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop, runtime *goja.Runtime, cs *parent.CaptureSession) goja.Value {
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

	// ── wait() → Promise<{ code, error? }> ─────────────────
	// Async per JS Binding Contract: waits until child process exits and output is drained.
	_ = obj.Set("wait", func(call goja.FunctionCall) goja.Value {
		return adapter.TrackPromise(ctx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
				res, err := func(ctx context.Context) (any, error) {
			code, err := cs.Wait()
			result := map[string]any{"code": code}
			if err != nil {
				result["error"] = err.Error()
			}
			return result, nil
		}(ctx)
				if err != nil {
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return rt.NewGoError(err) })
					return
				}
				_ = settle.Settle(false, func(rt *goja.Runtime) any {
					if res == nil { return goja.Undefined() }
					return res
				})
			})
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

	// ── passthrough(toggleKey?) → Promise<{ reason, error? }> ────
	// Async: enters raw passthrough mode, resolves when user toggles or child exits.
	// Uses os.Stdin/os.Stdout and the real terminal state. The caller (BubbleTea's
	// toggleModel) must have already released the terminal before calling this.
	_ = obj.Set("passthrough", func(call goja.FunctionCall) goja.Value {
		toggleKey := byte(parent.DefaultToggleKey)
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			cfgObj := call.Argument(0).ToObject(runtime)
			if cfgObj != nil {
				if v := cfgObj.Get("toggleKey"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					toggleKey = byte(v.ToInteger())
				}
			}
		}

		return adapter.TrackPromise(ctx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
				res, err := func(ctx context.Context) (any, error) {
			termFd := int(os.Stdin.Fd())
			reason, err := cs.Passthrough(ctx, parent.PassthroughConfig{
				TerminalIO: parent.TerminalIO{
					Stdin:         os.Stdin,
					Stdout:        os.Stdout,
					TermFd:        termFd,
					BlockingGuard: parent.DefaultBlockingGuard(),
				},
				PassthroughOptions: parent.PassthroughOptions{
					ToggleKey: toggleKey,
					TermState: ptyio.RealTermState{},
				},
			})
			result := map[string]any{
				"reason": exitReasonString(reason),
			}
			if err != nil {
				result["error"] = err.Error()
			}
			return result, nil
		}(ctx)
				if err != nil {
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return rt.NewGoError(err) })
					return
				}
				_ = settle.Settle(false, func(rt *goja.Runtime) any {
					if res == nil { return goja.Undefined() }
					return res
				})
			})
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
//	resize, write, close, isDone, readAvailable.
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

	_ = obj.Set("sendKeys", func(keys ...string) {
		var buf strings.Builder
		for _, key := range keys {
			seq, ok := parent.KeyToTermBytes(key, false, false)
			if !ok {
				panic(runtime.NewTypeError("sendKeys: unrecognized key " + key))
			}
			buf.WriteString(seq)
		}
		if _, err := session.Write([]byte(buf.String())); err != nil {
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

// UnwrapSessionManager retrieves the Go *SessionManager stored on a JS
// wrapper object by WrapSessionManager. Returns nil if the object does
// not contain a _goSessionManager property. Exported so other builtin
// modules (e.g., termui/splitlayout, termui/termpane) can extract the
// Go pointer from a JS-passed manager object.
func UnwrapSessionManager(obj *goja.Object) *parent.SessionManager {
	v := obj.Get("_goSessionManager")
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	mgr, _ := v.Export().(*parent.SessionManager)
	return mgr
}

// newSessionManager creates a [parent.SessionManager] from an optional JS
// options object and returns a wrapped JS object.
//
// JS signature:
//
//	termmux.newSessionManager({ rows?: number, cols?: number, requestBuffer?: number, outputBuffer?: number, title?: string })
func newSessionManager(ctx context.Context, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop, runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
	var opts []parent.ManagerOption
	var title string

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
			if v := cfgObj.Get("title"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				title = v.String()
			}
		}
	}

	mgr := parent.NewSessionManager(opts...)
	return WrapSessionManager(ctx, adapter, loop, runtime, mgr, os.Stdin, os.Stdout, -1, title)
}

// newBoundedSession creates a CaptureSession and a SessionManager in one call,
// starts the session, runs the manager, registers and activates the session.
// This replaces the common 20+ line setup pattern in JS scripts.
//
// JS signature:
//
//	termmux.newBoundedSession({ cmd, args?, dir?, rows?, cols?, env?, name?, kind? })
//
// Returns { session, mgr, sid } where session is the wrapped CaptureSession,
// mgr is the wrapped SessionManager, and sid is the session ID.
func newBoundedSession(ctx context.Context, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop, runtime *goja.Runtime, mgr *parent.SessionManager, call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
		panic(runtime.NewTypeError("newBoundedSession: options object is required"))
	}

	cfgObj := call.Argument(0).ToObject(runtime)

	cmd := ""
	if v := cfgObj.Get("cmd"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		cmd = v.String()
	}
	if cmd == "" {
		panic(runtime.NewTypeError("newBoundedSession: cmd is required"))
	}

	var args []string
	if v := cfgObj.Get("args"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		argsObj := v.ToObject(runtime)
		if lenVal := argsObj.Get("length"); lenVal != nil && !goja.IsUndefined(lenVal) {
			arrLen := lenVal.ToInteger()
			for i := range arrLen {
				av := argsObj.Get(fmt.Sprintf("%d", i))
				if av != nil && !goja.IsUndefined(av) {
					args = append(args, av.String())
				}
			}
		}
	}

	rows := 24
	if v := cfgObj.Get("rows"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		rows = int(v.ToInteger())
	}
	cols := 80
	if v := cfgObj.Get("cols"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		cols = int(v.ToInteger())
	}

	captureCfg := parent.CaptureConfig{
		Command: cmd,
		Args:    args,
		Rows:    rows,
		Cols:    cols,
	}
	if v := cfgObj.Get("dir"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		captureCfg.Dir = v.String()
	}
	if v := cfgObj.Get("env"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		envObj := v.ToObject(runtime)
		captureCfg.Env = make(map[string]string)
		for _, key := range envObj.Keys() {
			val := envObj.Get(key)
			if val != nil && !goja.IsUndefined(val) && !goja.IsNull(val) {
				captureCfg.Env[key] = val.String()
			}
		}
	}

	var name string
	if v := cfgObj.Get("name"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		name = v.String()
	}
	var kind parent.SessionKind
	if v := cfgObj.Get("kind"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		kind = parent.SessionKind(v.String())
	}

	baseCtx := ctx
	if adapter == nil {
		cs := parent.NewCaptureSession(captureCfg)

		if mgr == nil {
			mgr = parent.NewSessionManager(parent.WithTermSize(rows, cols))
			go mgr.Run(baseCtx)
			<-mgr.Started()
		}

		sid, err := mgr.Register(cs, parent.SessionTarget{
			Name: name,
			Kind: kind,
		})
		if err != nil {
			panic(runtime.NewGoError(fmt.Errorf("newBoundedSession: register failed: %w", err)))
		}

		if err := cs.Start(baseCtx); err != nil {
			panic(runtime.NewGoError(fmt.Errorf("newBoundedSession: start failed: %w", err)))
		}

		sessionVal := WrapCaptureSession(baseCtx, adapter, loop, runtime, cs)
		mgrVal := WrapSessionManager(baseCtx, adapter, loop, runtime, mgr, os.Stdin, os.Stdout, -1, "")

		result := runtime.NewObject()
		_ = result.Set("session", sessionVal)
		_ = result.Set("mgr", mgrVal)
		_ = result.Set("sid", runtime.ToValue(sid))

		return result
	}

	return adapter.TrackPromise(baseCtx, func(trackCtx context.Context, settle gojaeventloop.TrackedSettlement) {
		cs := parent.NewCaptureSession(captureCfg)

		var localMgr *parent.SessionManager
		if mgr == nil {
			localMgr = parent.NewSessionManager(parent.WithTermSize(rows, cols))
			go localMgr.Run(baseCtx)
			<-localMgr.Started()
		} else {
			localMgr = mgr
		}

		sid, err := localMgr.Register(cs, parent.SessionTarget{
			Name: name,
			Kind: kind,
		})
		if err != nil {
			_ = settle.Settle(true, func(rt *goja.Runtime) any { return rt.NewGoError(fmt.Errorf("newBoundedSession: register failed: %w", err)) })
			return
		}

		if err := cs.Start(trackCtx); err != nil {
			_ = settle.Settle(true, func(rt *goja.Runtime) any { return rt.NewGoError(fmt.Errorf("newBoundedSession: start failed: %w", err)) })
			return
		}

		_ = settle.Settle(false, func(rt *goja.Runtime) any {
			sessionVal := WrapCaptureSession(baseCtx, adapter, loop, rt, cs)
			mgrVal := WrapSessionManager(baseCtx, adapter, loop, rt, localMgr, os.Stdin, os.Stdout, -1, "")
			result := rt.NewObject()
			_ = result.Set("session", sessionVal)
			_ = result.Set("mgr", mgrVal)
			_ = result.Set("sid", rt.ToValue(sid))
			return result
		})
	})
}

// WrapSessionManager wraps a [parent.SessionManager] into a Goja object with
// JavaScript-callable methods. Exported so callers can create a Go-side
// SessionManager and expose it through the same interface.
//
// The stdin/stdout/termFd parameters provide terminal I/O for passthrough
// mode. Pass os.Stdin, os.Stdout, and -1 (or int(os.Stdin.Fd())) as defaults.
//
// The adapter must have had Bind() called so EventTarget and CustomEvent
// globals are available.
func WrapSessionManager(ctx context.Context, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop, runtime *goja.Runtime, mgr *parent.SessionManager, stdin io.Reader, stdout io.Writer, termFd int, title string) goja.Value {
	obj, _ := wrapSessionManager(ctx, adapter, loop, runtime, mgr, stdin, stdout, termFd, title)
	return obj
}

// wrapSessionManager is the internal implementation of WrapSessionManager; it
// also returns the backing *muxState for package-internal test assertions.
func wrapSessionManager(ctx context.Context, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop, runtime *goja.Runtime, mgr *parent.SessionManager, stdin io.Reader, stdout io.Writer, termFd int, title string) (*goja.Object, *muxState) {
	if adapter != nil && mgr != nil {
		if cached, ok := managerWrapperCache.Load(mgr); ok {
			entry := cached.(*wrapperCacheEntry)
			if entry.state != nil && entry.state.runtime == runtime {
				return entry.obj, entry.state
			}
		}
	}

	obj := runtime.NewObject()

	_ = obj.DefineDataProperty("_goSessionManager", runtime.ToValue(mgr),
		goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)

	s := &muxState{
		ctx:       ctx,
		runtime:   runtime,
		mgr:       mgr,
		loop:      loop,
		stdin:     stdin,
		stdout:    stdout,
		termFd:    termFd,
		adapter:   adapter,
		sb:        statusbar.New(stdout),
		toggleKey: parent.DefaultToggleKey,
	}
	s.sb.SetToggleKey(s.toggleKey)
	if title != "" {
		s.sb.SetTitle(title)
	}

	if adapter != nil {
		// EventTarget creation must run on the JS/event-loop goroutine.
		// In production script execution that is the current goroutine;
		// in tests the event loop goroutine is idle and it is safe to init
		// synchronously on the test goroutine.
		_ = s.initEventTarget()

		// EventBus → EventTarget bridge: translate SessionManager events into
		// CustomEvents delivered on the event loop.
		busID, busCh := mgr.Subscribe(4096)
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
					data := buildEventData(evt)
					if data == nil {
						continue
					}
					adapter.Submit(func(_ *goja.Runtime) {
						s.dispatchCustomEvent(data.eventType, data.detail)
					})
				}
			}
		}()
	}

	registerSessionMethods(obj, s)
	registerSnapshotMethods(obj, s)
	registerPassthroughMethods(obj, s)
	registerStatusMethods(obj, s)
	registerPersistenceMethods(obj, s)
	registerPaneMethods(obj, s)
	registerChooserMethods(obj, s)

	if adapter != nil && mgr != nil {
		managerWrapperCache.Store(mgr, &wrapperCacheEntry{obj: obj, state: s})
		// Clean up the cache entry when the context is cancelled so stale
		// entries don't leak across test runs or manager lifecycles.
		go func() {
			<-ctx.Done()
			managerWrapperCache.Delete(mgr)
		}()
	}

	return obj, s
}

type eventDispatchData struct {
	eventType string
	detail    map[string]any
}

func buildEventData(evt parent.Event) *eventDispatchData {
	sid := uint64(evt.SessionID)
	data := map[string]any{"sessionId": sid}

	switch evt.Kind {
	case parent.EventSessionRegistered:
		return &eventDispatchData{EventRegistered, data}
	case parent.EventSessionActivated:
		return &eventDispatchData{EventActivated, data}
	case parent.EventSessionExited:
		data["pane"] = "agent"
		return &eventDispatchData{EventExit, data}
	case parent.EventSessionClosed:
		return &eventDispatchData{EventClosed, data}
	case parent.EventResize:
		if dims, ok := evt.Data.([2]int); ok {
			data["rows"] = dims[0]
			data["cols"] = dims[1]
		}
		return &eventDispatchData{EventTerminalResize, data}
	case parent.EventBell:
		data["pane"] = "agent"
		return &eventDispatchData{EventBell, data}
	case parent.EventSessionOutput:
		data["pane"] = "agent"
		if raw, ok := evt.Data.([]byte); ok {
			data["chunk"] = string(raw)
		}
		return &eventDispatchData{EventOutput, data}
	case parent.EventActivity:
		return &eventDispatchData{EventActivity, data}
	case parent.EventSilence:
		return &eventDispatchData{EventSilence, data}
	case parent.EventTitle:
		if s, ok := evt.Data.(string); ok {
			data["data"] = s
		}
		return &eventDispatchData{EventTitle, data}
	case parent.EventWorkingDirectory:
		if s, ok := evt.Data.(string); ok {
			data["data"] = s
		}
		return &eventDispatchData{EventWorkingDirectory, data}
	case parent.EventClipboard:
		if s, ok := evt.Data.(string); ok {
			data["data"] = s
		}
		return &eventDispatchData{EventClipboard, data}
	default:
		return nil
	}
}

// registerSessionMethods registers lifecycle methods: run, started, close,
// subscribe, unsubscribe, register, unregister, activate, input, resize,
// resizeSession.
func registerSessionMethods(obj *goja.Object, s *muxState) {
	_ = obj.Set("run", func() {
		go func() {
			_ = s.mgr.Run(s.ctx)
		}()
	})

	_ = obj.Set("started", func() bool {
		select {
		case <-s.mgr.Started():
			return true
		case <-s.ctx.Done():
			return false
		}
	})

	_ = obj.Set("close", func() {
		s.mgr.Close()
	})

	_ = obj.Set("subscribe", func(call goja.FunctionCall) goja.Value {
		bufSize := 64
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			bufSize = int(call.Argument(0).ToInteger())
		}
		id, ch := s.mgr.Subscribe(bufSize)

		result := s.runtime.NewObject()
		_ = result.Set("id", id)

		_ = result.Set("pollEvents", func() goja.Value {
			evts := make([]map[string]any, 0)
			for {
				select {
				case evt, ok := <-ch:
					if !ok {
						return s.runtime.ToValue(evts)
					}
					evts = append(evts, map[string]any{
						"kind":      evt.Kind.String(),
						"sessionId": uint64(evt.SessionID),
						"time":      evt.Time.UnixMilli(),
					})
				default:
					return s.runtime.ToValue(evts)
				}
			}
		})

		return result
	})

	_ = obj.Set("unsubscribe", func(id int) bool {
		return s.mgr.Unsubscribe(id)
	})

	_ = obj.Set("register", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("register requires at least 1 argument (session)"))
		}

		sessionObj := call.Argument(0).ToObject(s.runtime)
		session := unwrapInteractiveSession(sessionObj)
		if session == nil {
			panic(s.runtime.NewTypeError("register: first argument must be an InteractiveSession wrapper"))
		}

		var target parent.SessionTarget
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			tObj := call.Argument(1).ToObject(s.runtime)
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

		id, err := s.mgr.Register(session, target)
		if err != nil {
			panic(s.runtime.NewGoError(err))
		}
		return s.runtime.ToValue(uint64(id))
	})

	_ = obj.Set("unregister", func(id uint64) {
		if err := s.mgr.Unregister(parent.SessionID(id)); err != nil {
			panic(s.runtime.NewGoError(err))
		}
	})

	_ = obj.Set("activate", func(id uint64) {
		if err := s.mgr.Activate(parent.SessionID(id)); err != nil {
			panic(s.runtime.NewGoError(err))
		}
	})

	_ = obj.Set("input", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("input requires 1 argument (data)"))
		}
		data := []byte(call.Argument(0).String())
		if err := s.mgr.Input(data); err != nil {
			panic(s.runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	_ = obj.Set("sendKeys", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(s.runtime.NewTypeError("sendKeys requires at least 2 arguments (sessionID, ...keys)"))
		}
		id := parent.SessionID(call.Argument(0).ToInteger())
		keys := make([]string, 0, len(call.Arguments)-1)
		for i := 1; i < len(call.Arguments); i++ {
			keys = append(keys, call.Argument(i).String())
		}
		if err := s.mgr.SendKeys(id, keys...); err != nil {
			panic(s.runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	_ = obj.Set("resize", func(rows, cols int) {
		if err := s.mgr.Resize(rows, cols); err != nil {
			panic(s.runtime.NewGoError(err))
		}
	})

	_ = obj.Set("resizeSession", func(id uint64, rows, cols int) {
		if err := s.mgr.ResizeSession(parent.SessionID(id), rows, cols); err != nil {
			panic(s.runtime.NewGoError(err))
		}
	})

	_ = obj.Set("isCopyModeActive", func(id uint64) bool {
		return s.mgr.IsCopyModeActive(parent.SessionID(id))
	})

	_ = obj.Set("scrollCopyMode", func(id uint64, delta int) bool {
		return s.mgr.ScrollCopyMode(parent.SessionID(id), delta)
	})

	_ = obj.Set("enterCopyMode", func(id uint64) {
		_ = s.mgr.EnterCopyMode(parent.SessionID(id))
	})

	_ = obj.Set("exitCopyMode", func(id uint64) {
		_ = s.mgr.ExitCopyMode(parent.SessionID(id))
	})

	_ = obj.Set("selectStart", func(id uint64, row, col int) {
		_ = s.mgr.SelectStart(parent.SessionID(id), row, col)
	})

	_ = obj.Set("selectEnd", func(id uint64, row, col int) {
		_ = s.mgr.SelectEnd(parent.SessionID(id), row, col)
	})

	_ = obj.Set("copyModeKey", func(sessionID uint64, key string) map[string]any {
		h := parent.NewCopyModeKeyHandler(0)
		action := h.HandleKey(key)
		err := s.mgr.HandleCopyModeKey(parent.SessionID(sessionID), key)
		return map[string]any{
			"action":   action.String(),
			"consumed": action.Kind != parent.CopyModeActionNone,
			"error":    errToStr(err),
		}
	})

	_ = obj.Set("searchForward", func(id uint64, pattern string) map[string]any {
		searcher := s.sessionScreenSearcher(id)
		if searcher == nil || pattern == "" {
			return map[string]any{"found": false}
		}
		return wrapSearchMatch1Based(searcher.SearchForward(pattern, 0, 0))
	})

	_ = obj.Set("searchBackward", func(id uint64, pattern string) map[string]any {
		searcher := s.sessionScreenSearcher(id)
		if searcher == nil || pattern == "" {
			return map[string]any{"found": false}
		}
		return wrapSearchMatch1Based(searcher.SearchBackwardFromEnd(pattern))
	})

	_ = obj.Set("newWindow", func(call goja.FunctionCall) goja.Value {
		name := ""
		if len(call.Arguments) > 0 && call.Argument(0) != goja.Undefined() {
			name = call.Argument(0).String()
		}
		id, err := s.mgr.NewWindow(name)
		if err != nil {
			panic(s.runtime.NewGoError(err))
		}
		return s.runtime.ToValue(uint64(id))
	})

	_ = obj.Set("nextWindow", func() goja.Value {
		id := s.mgr.NextWindow()
		return s.runtime.ToValue(uint64(id))
	})

	_ = obj.Set("prevWindow", func() goja.Value {
		id := s.mgr.PrevWindow()
		return s.runtime.ToValue(uint64(id))
	})

	_ = obj.Set("renameWindow", func(id uint64, name string) {
		if err := s.mgr.RenameWindow(parent.WindowID(id), name); err != nil {
			panic(s.runtime.NewGoError(err))
		}
	})

	_ = obj.Set("closeWindow", func(id uint64) {
		if err := s.mgr.CloseWindow(parent.WindowID(id)); err != nil {
			panic(s.runtime.NewGoError(err))
		}
	})

	_ = obj.Set("moveWindow", func(id uint64, targetIndex int) {
		if err := s.mgr.MoveWindow(parent.WindowID(id), targetIndex); err != nil {
			panic(s.runtime.NewGoError(err))
		}
	})

	_ = obj.Set("swapWindow", func(a, b uint64) {
		if err := s.mgr.SwapWindows(parent.WindowID(a), parent.WindowID(b)); err != nil {
			panic(s.runtime.NewGoError(err))
		}
	})

	_ = obj.Set("activeWindowID", func() goja.Value {
		id := s.mgr.ActiveWindowID()
		return s.runtime.ToValue(uint64(id))
	})

	_ = obj.Set("renderPaneBorders", func(width, height int, panes goja.Value) goja.Value {
		arr := panes.Export().([]any)
		paneList := make([]parent.Pane, 0, len(arr))
		for i, raw := range arr {
			o := raw.(map[string]any)
			geom := parent.PaneGeometry{
				Row:  int(toInt64(o, "row")),
				Col:  int(toInt64(o, "col")),
				Rows: int(toInt64(o, "rows")),
				Cols: int(toInt64(o, "cols")),
			}
			p := parent.Pane{
				ID:       parent.PaneID(i + 1),
				Title:    toString(o, "title"),
				Geometry: geom,
			}
			paneList = append(paneList, p)
		}
		return s.runtime.ToValue(parent.RenderPaneBorders(width, height, paneList))
	})

	_ = obj.Set("setLayoutMode", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("setLayoutMode requires 1 argument (mode)"))
		}
		name := call.Argument(0).String()
		mode, ok := parent.ParseLayoutMode(name)
		if !ok {
			panic(s.runtime.NewTypeError("setLayoutMode: unknown mode " + name))
		}
		if err := s.mgr.SetLayoutMode(s.mgr.ActiveWindowID(), mode); err != nil {
			panic(s.runtime.NewGoError(err))
		}
		return obj
	})

	_ = obj.Set("layoutMode", func() string {
		mode, err := s.mgr.LayoutMode(s.mgr.ActiveWindowID())
		if err != nil {
			panic(s.runtime.NewGoError(err))
		}
		return LayoutModeString(mode)
	})

	_ = obj.Set("windows", func() goja.Value {
		ws := s.mgr.Windows()
		items := make([]goja.Value, len(ws))
		for i, w := range ws {
			obj := s.runtime.NewObject()
			_ = obj.Set("id", uint64(w.ID))
			_ = obj.Set("name", w.Name)
			_ = obj.Set("layout", int(w.Layout))
			_ = obj.Set("active", w.ID == s.mgr.ActiveWindowID())
			items[i] = obj
		}
		return s.runtime.ToValue(items)
	})

	_ = obj.Set("windowPanes", func() goja.Value {
		wp := s.mgr.WindowPanes()
		windows := s.mgr.Windows()
		windowNames := make(map[parent.WindowID]string, len(windows))
		for _, w := range windows {
			windowNames[w.ID] = w.Name
		}
		result := make([]map[string]any, 0)
		for wid, panes := range wp {
			pList := make([]map[string]any, len(panes))
			for i, p := range panes {
				pList[i] = map[string]any{
					"id":        uint64(p.ID),
					"sessionId": uint64(p.SessionID),
					"title":     p.Title,
					"focus":     p.Focus,
					"exited":    p.Exited,
					"geometry": map[string]any{
						"row":  p.Geometry.Row,
						"col":  p.Geometry.Col,
						"rows": p.Geometry.Rows,
						"cols": p.Geometry.Cols,
					},
				}
			}
			wObj := map[string]any{
				"id":     uint64(wid),
				"name":   windowNames[wid],
				"panes":  pList,
				"active": wid == s.mgr.ActiveWindowID(),
			}
			result = append(result, wObj)
		}
		return s.runtime.ToValue(result)
	})

	_ = obj.Set("setSynchronizePanes", func(v bool) goja.Value {
		if err := s.mgr.SetSynchronizePanes(v); err != nil {
			panic(s.runtime.NewGoError(err))
		}
		return obj
	})

	_ = obj.Set("synchronizePanes", func() bool {
		return s.mgr.SynchronizePanes()
	})

	_ = obj.Set("setMonitorConfig", func(sessionID uint64, cfg goja.Value) {
		var mc parent.MonitorConfig
		if cfg != nil && !goja.IsUndefined(cfg) {
			o := cfg.ToObject(s.runtime)
			if v := o.Get("bell"); v != nil && !goja.IsUndefined(v) {
				mc.Bell = v.ToBoolean()
			}
			if v := o.Get("activity"); v != nil && !goja.IsUndefined(v) {
				mc.Activity = v.ToBoolean()
			}
			if v := o.Get("activityThreshold"); v != nil && !goja.IsUndefined(v) {
				mc.ActivityThreshold = time.Duration(v.ToFloat() * float64(time.Second))
			}
			if v := o.Get("activityResetThreshold"); v != nil && !goja.IsUndefined(v) {
				mc.ActivityResetThreshold = time.Duration(v.ToFloat() * float64(time.Second))
			}
			if v := o.Get("silence"); v != nil && !goja.IsUndefined(v) {
				mc.Silence = v.ToBoolean()
			}
			if v := o.Get("silenceThreshold"); v != nil && !goja.IsUndefined(v) {
				mc.SilenceThreshold = time.Duration(v.ToFloat() * float64(time.Second))
			}
		}
		_ = s.mgr.SetMonitorConfig(parent.SessionID(sessionID), mc)
	})

	_ = obj.Set("monitorConfig", func(sessionID uint64) goja.Value {
		cfg, err := s.mgr.MonitorConfig(parent.SessionID(sessionID))
		if err != nil {
			panic(s.runtime.NewTypeError(err.Error()))
		}
		result := s.runtime.NewObject()
		_ = result.Set("bell", cfg.Bell)
		_ = result.Set("activity", cfg.Activity)
		_ = result.Set("activityThreshold", cfg.ActivityThreshold.Seconds())
		_ = result.Set("activityResetThreshold", cfg.ActivityResetThreshold.Seconds())
		_ = result.Set("silence", cfg.Silence)
		_ = result.Set("silenceThreshold", cfg.SilenceThreshold.Seconds())
		return result
	})

	_ = obj.Set("visualBellActive", func(sessionID uint64) bool {
		active, err := s.mgr.VisualBellActive(parent.SessionID(sessionID))
		if err != nil {
			return false
		}
		return active
	})

	_ = obj.Set("checkSilenceMonitors", func() int {
		return s.mgr.CheckSilenceMonitors()
	})

	_ = obj.Set("resetActivity", func(id uint64) {
		s.mgr.ResetActivity(parent.SessionID(id))
	})

	_ = obj.Set("setRemainOnExit", func(v bool) {
		s.mgr.SetRemainOnExit(v)
	})

	_ = obj.Set("remainOnExit", func() bool {
		return s.mgr.RemainOnExit()
	})

	_ = obj.Set("setPaneRemainOnExit", func(paneID uint64, v bool) {
		_ = s.mgr.SetPaneRemainOnExit(parent.PaneID(paneID), v)
	})

	_ = obj.Set("paneRemainOnExit", func(paneID uint64) bool {
		v, _ := s.mgr.PaneRemainOnExit(parent.PaneID(paneID))
		return v
	})

	_ = obj.Set("paneExited", func(paneID uint64) bool {
		return s.mgr.PaneExited(parent.PaneID(paneID))
	})

	_ = obj.Set("respawnSession", func(sessionID uint64) uint64 {
		id, _ := s.mgr.RespawnSession(parent.SessionID(sessionID))
		return uint64(id)
	})

	_ = obj.Set("swapPanes", func(a, b uint64) map[string]any {
		if err := s.mgr.SwapPanes(parent.PaneID(a), parent.PaneID(b)); err != nil {
			panic(s.runtime.NewGoError(err))
		}
		return map[string]any{"swapped": true}
	})

	_ = obj.Set("zoomPane", func(paneID uint64) {
		s.mgr.ZoomPane(parent.PaneID(paneID))
	})

	_ = obj.Set("zoomedPane", func() uint64 {
		return uint64(s.mgr.ZoomedPane())
	})

	_ = obj.Set("breakPane", func(paneID uint64) map[string]any {
		newWID, newPID, sid, err := s.mgr.BreakPane(parent.PaneID(paneID))
		if err != nil {
			panic(s.runtime.NewGoError(err))
		}
		return map[string]any{
			"paneID":    uint64(newPID),
			"windowID":  uint64(newWID),
			"sessionId": uint64(sid),
		}
	})

	_ = obj.Set("joinPane", func(paneID uint64, targetWindowID uint64) map[string]any {
		newPID, sid, err := s.mgr.JoinPane(parent.PaneID(paneID), parent.WindowID(targetWindowID))
		if err != nil {
			panic(s.runtime.NewGoError(err))
		}
		return map[string]any{
			"paneID":    uint64(newPID),
			"windowID":  targetWindowID,
			"sessionId": uint64(sid),
		}
	})

	_ = obj.Set("setPipeFile", func(sessionID uint64, path string) {
		if err := s.mgr.SetPipeFile(parent.SessionID(sessionID), path); err != nil {
			panic(s.runtime.NewGoError(err))
		}
	})

	_ = obj.Set("pipeCommand", func(sessionID uint64, cmd string, args goja.Value) {
		if cmd == "" {
			panic(s.runtime.NewTypeError("pipeCommand: command must be a non-empty string"))
		}
		var argv []string
		if args != nil && !goja.IsUndefined(args) && !goja.IsNull(args) {
			argsObj := args.ToObject(s.runtime)
			if lenVal := argsObj.Get("length"); lenVal != nil && !goja.IsUndefined(lenVal) {
				arrLen := lenVal.ToInteger()
				for i := range arrLen {
					v := argsObj.Get(fmt.Sprintf("%d", i))
					if v != nil && !goja.IsUndefined(v) {
						argv = append(argv, v.String())
					}
				}
			}
		}
		if err := s.mgr.PipePaneCommand(parent.SessionID(sessionID), cmd, argv); err != nil {
			panic(s.runtime.NewGoError(err))
		}
	})

	_ = obj.Set("clearPipe", func(sessionID uint64) {
		if err := s.mgr.ClearPipe(parent.SessionID(sessionID)); err != nil {
			panic(s.runtime.NewGoError(err))
		}
	})

	_ = obj.Set("displayMessage", func(sessionID uint64, text string, durationMs ...int) {
		dur := 3 * time.Second
		if len(durationMs) > 0 && durationMs[0] > 0 {
			dur = time.Duration(durationMs[0]) * time.Millisecond
		}
		_ = s.mgr.DisplayMessage(parent.SessionID(sessionID), text, dur)
	})

	_ = obj.Set("activeMessage", func(sessionID uint64) string {
		return s.mgr.ActiveMessage(parent.SessionID(sessionID))
	})

	_ = obj.Set("capturePane", func(sessionID uint64, startLine, endLine int) string {
		return s.mgr.CapturePane(parent.SessionID(sessionID), startLine, endLine)
	})

	_ = obj.Set("copyPaneToClipboard", func(sessionID uint64) string {
		return s.mgr.CopyPaneToClipboard(parent.SessionID(sessionID))
	})

	_ = obj.Set("copySelection", func(sessionID uint64) string {
		return s.mgr.CopySelection(parent.SessionID(sessionID))
	})

	_ = obj.Set("lockSession", func(sessionID uint64, password string) error {
		return s.mgr.LockSession(parent.SessionID(sessionID), password)
	})

	_ = obj.Set("unlockSession", func(sessionID uint64, password string) bool {
		return s.mgr.UnlockSession(parent.SessionID(sessionID), password)
	})

	_ = obj.Set("isLocked", func(sessionID uint64) bool {
		return s.mgr.IsLocked(parent.SessionID(sessionID))
	})

	_ = obj.Set("newCopyModeKeyHandler", func(halfPageRows int) map[string]any {
		h := parent.NewCopyModeKeyHandler(halfPageRows)
		return map[string]any{
			"handleKey": func(key string) map[string]any {
				action := h.HandleKey(key)
				return map[string]any{
					"kind":   int(action.Kind),
					"n":      action.N,
					"string": action.String(),
				}
			},
		}
	})

	_ = obj.Set("newCopyModeSearcher", func() map[string]any {
		cs := parent.NewCopyModeSearcher()
		resolveSearcher := func(args []goja.Value, idx int) parent.ScreenSearcher {
			if len(args) > idx {
				arg := args[idx]
				if arg != nil && !goja.IsUndefined(arg) && !goja.IsNull(arg) {
					if fn, ok := goja.AssertFunction(arg); ok {
						return copyModeSearchAdapter{searchFn: func(pattern string, row, col int) map[string]any {
							ret, err := fn(goja.Undefined(),
								s.runtime.ToValue(pattern),
								s.runtime.ToValue(row),
								s.runtime.ToValue(col))
							if err != nil || ret == nil || goja.IsUndefined(ret) || goja.IsNull(ret) {
								return nil
							}
							m, ok := ret.Export().(map[string]any)
							if !ok {
								return nil
							}
							return m
						}}
					}
				}
			}
			return s.activeScreenSearcher()
		}
		return map[string]any{
			"startSearch": func(direction int, cursorRow, cursorCol int) {
				cs.StartSearch(parent.CopyModeSearchDirection(direction), cursorRow, cursorCol)
			},
			"direction": func() int { return int(cs.Direction()) },
			"pattern":   func() string { return cs.Pattern() },
			"appendChar": func(ch string) {
				if len(ch) > 0 {
					cs.AppendChar(rune(ch[0]))
				}
			},
			"backspace": cs.Backspace,
			"execute": func(args ...goja.Value) map[string]any {
				match := cs.Execute(resolveSearcher(args, 0))
				return wrapSearchMatch(match)
			},
			"nextMatch": func(args ...goja.Value) map[string]any {
				currentRow := 0
				currentCol := 0
				if len(args) > 0 && !goja.IsUndefined(args[0]) {
					currentRow = int(args[0].ToInteger())
				}
				if len(args) > 1 && !goja.IsUndefined(args[1]) {
					currentCol = int(args[1].ToInteger())
				}
				match := cs.NextMatch(resolveSearcher(args, 2), currentRow, currentCol)
				return wrapSearchMatch(match)
			},
			"prevMatch": func(args ...goja.Value) map[string]any {
				currentRow := 0
				currentCol := 0
				if len(args) > 0 && !goja.IsUndefined(args[0]) {
					currentRow = int(args[0].ToInteger())
				}
				if len(args) > 1 && !goja.IsUndefined(args[1]) {
					currentCol = int(args[1].ToInteger())
				}
				match := cs.PrevMatch(resolveSearcher(args, 2), currentRow, currentCol)
				return wrapSearchMatch(match)
			},
		}
	})

}

// registerSnapshotMethods registers query/snapshot methods: termSize,
// snapshot, renderRaster, activeID, isDone, sessions, eventsDropped,
// lastActivityMs.
func registerSnapshotMethods(obj *goja.Object, s *muxState) {
	_ = obj.Set("termSize", func() goja.Value {
		rows, cols := s.mgr.TermSize()
		result := s.runtime.NewObject()
		_ = result.Set("rows", rows)
		_ = result.Set("cols", cols)
		return result
	})

	_ = obj.Set("snapshot", func(id uint64) goja.Value {
		snap := s.mgr.Snapshot(parent.SessionID(id))
		if snap == nil {
			return goja.Null()
		}
		result := s.runtime.NewObject()
		_ = result.Set("gen", snap.Gen)
		_ = result.Set("plainText", snap.GetPlainText())
		_ = result.Set("ansi", snap.GetANSI())
		_ = result.Set("fullScreen", snap.GetFullScreen())
		_ = result.Set("rows", snap.Rows)
		_ = result.Set("cols", snap.Cols)
		_ = result.Set("cursorRow", snap.CursorRow)
		_ = result.Set("cursorCol", snap.CursorCol)
		_ = result.Set("mouseTracking", snap.MouseTracking)
		_ = result.Set("mouseSGR", snap.MouseSGR)
		_ = result.Set("locked", snap.Locked)
		_ = result.Set("message", snap.Message)
		_ = result.Set("timestamp", snap.Timestamp.UnixMilli())
		return result
	})

	_ = obj.Set("renderRaster", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("renderRaster: session ID argument is required"))
		}
		id := parent.SessionID(call.Argument(0).ToInteger())
		cellW := 8
		cellH := 16
		optsVal := call.Argument(1)
		if optsVal != nil && optsVal != goja.Undefined() && optsVal != goja.Null() {
			opts := optsVal.ToObject(s.runtime)
			if v := opts.Get("cellW"); v != nil && !goja.IsUndefined(v) {
				cellW = int(v.ToInteger())
			}
			if v := opts.Get("cellH"); v != nil && !goja.IsUndefined(v) {
				cellH = int(v.ToInteger())
			}
		}
		if cellW <= 0 || cellH <= 0 {
			panic(s.runtime.NewTypeError("renderRaster: cell dimensions must be positive"))
		}
		scr := s.mgr.Screen(id)
		if scr == nil {
			return goja.Null()
		}
		var img *image.RGBA
		if cellW == 8 && cellH == 16 {
			img = vt.RenderRasterDefault(scr)
		} else {
			img = vt.RenderRaster(scr, cellW, cellH)
		}
		tmpDir := os.TempDir()
		f, err := os.CreateTemp(tmpDir, fmt.Sprintf("osm-raster-%d-*.png", id))
		if err != nil {
			panic(s.runtime.NewGoError(err))
		}
		path := f.Name()
		_ = f.Close()
		if err := vt.SaveRasterPNG(img, path); err != nil {
			panic(s.runtime.NewGoError(err))
		}
		result := s.runtime.NewObject()
		_ = result.Set("width", img.Bounds().Dx())
		_ = result.Set("height", img.Bounds().Dy())
		_ = result.Set("path", path)
		return result
	})

	_ = obj.Set("activeID", func() uint64 {
		return uint64(s.mgr.ActiveID())
	})

	_ = obj.Set("isDone", func(id uint64) bool {
		for _, info := range s.mgr.Sessions() {
			if info.ID == parent.SessionID(id) {
				return info.State == parent.SessionExited || info.State == parent.SessionClosed
			}
		}
		return true
	})

	_ = obj.Set("sessions", func() goja.Value {
		infos := s.mgr.Sessions()
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
		return s.runtime.ToValue(result)
	})

	_ = obj.Set("eventsDropped", func() int64 {
		return s.mgr.EventsDropped()
	})

	_ = obj.Set("lastActivityMs", func(call goja.FunctionCall) goja.Value {
		id := s.mgr.ActiveID()
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			id = parent.SessionID(call.Argument(0).ToInteger())
		}
		if id == 0 {
			return s.runtime.ToValue(int64(-1))
		}
		snap := s.mgr.Snapshot(id)
		if snap == nil || snap.Timestamp.IsZero() {
			return s.runtime.ToValue(int64(-1))
		}
		return s.runtime.ToValue(time.Since(snap.Timestamp).Milliseconds())
	})
}

// registerPassthroughMethods registers passthrough and convenience methods:
// passthrough, attach, detach, hasChild, switchTo, screenshot, childScreen,
// writeToChild, session, fromModel, activeSide.
func registerPassthroughMethods(obj *goja.Object, s *muxState) {
	_ = obj.Set("passthrough", func(call goja.FunctionCall) goja.Value {
		cfg := parent.PassthroughConfig{
			TerminalIO: parent.TerminalIO{
				TermFd:        -1,
				BlockingGuard: parent.DefaultBlockingGuard(),
			},
			PassthroughOptions: parent.PassthroughOptions{
				ToggleKey: 0x1D,
				TermState: ptyio.RealTermState{},
			},
		}

		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			opts := call.Argument(0).ToObject(s.runtime)

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
						_, err := fn(goja.Undefined(), s.runtime.ToValue(rows), s.runtime.ToValue(cols))
						if err != nil {
							return fmt.Errorf("resizeFn: %w", err)
						}
						return nil
					}
				}
			}
		}

		if cfg.Stdin == nil {
			cfg.Stdin = os.Stdin
		}
		if cfg.Stdout == nil {
			cfg.Stdout = os.Stdout
		}

		return s.adapter.TrackPromise(s.ctx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
				res, err := func(ctx context.Context) (any, error) {
			reason, err := s.mgr.Passthrough(s.ctx, cfg)
			result := map[string]any{
				"reason": exitReasonString(reason),
			}
			if err != nil {
				result["error"] = err.Error()
			}
			return result, nil
		}(ctx)
				if err != nil {
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return rt.NewGoError(err) })
					return
				}
				_ = settle.Settle(false, func(rt *goja.Runtime) any {
					if res == nil { return goja.Undefined() }
					return res
				})
			})
	})

	_ = obj.Set("attach", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(s.runtime.NewTypeError("attach: handle argument is required"))
		}
		raw := call.Argument(0).Export()

		var session parent.InteractiveSession

		if jsObj := call.Argument(0).ToObject(s.runtime); jsObj != nil {
			if ses := unwrapInteractiveSession(jsObj); ses != nil {
				session = ses
			}
		}

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
			panic(s.runtime.NewTypeError("attach: argument must be an InteractiveSession, StringIO, or wrapped AgentHandle"))
		}

		id, err := s.mgr.Register(session, s.activeSessionTarget)
		if err != nil {
			if activeID := s.mgr.ActiveID(); activeID != 0 {
				_ = s.mgr.Unregister(activeID)
				id, err = s.mgr.Register(session, s.activeSessionTarget)
			}
			if err != nil {
				panic(s.runtime.NewGoError(err))
			}
		}
		if activateErr := s.mgr.Activate(id); activateErr != nil {
			panic(s.runtime.NewGoError(activateErr))
		}
		return s.runtime.ToValue(uint64(id))
	})

	_ = obj.Set("detach", func() {
		if id := s.mgr.ActiveID(); id != 0 {
			_ = s.mgr.Unregister(id)
		}
	})

	_ = obj.Set("hasChild", func() bool {
		return s.mgr.ActiveID() != 0
	})

	_ = obj.Set("switchTo", func() goja.Value {
		if s.mgr.ActiveID() == 0 {
			return goja.Undefined()
		}

		s.SetInPassthrough(true)
		s.dispatchEventOnLoop(EventFocus, map[string]any{
			"side": "agent", "action": "enter",
		})

		cfg := parent.PassthroughConfig{
			TerminalIO: parent.TerminalIO{
				Stdin:         s.stdin,
				Stdout:        s.stdout,
				TermFd:        s.termFd,
				BlockingGuard: parent.DefaultBlockingGuard(),
			},
			PassthroughOptions: parent.PassthroughOptions{
				ToggleKey: s.toggleKey,
				TermState: ptyio.RealTermState{},
			},
			ResizeConfig: parent.ResizeConfig{
				RestoreScreen: s.swappedOnce,
			},
		}
		if s.statusEnabled {
			cfg.StatusBar = s.sb
		}
		if s.resizeFn != nil {
			cfg.ResizeFn = s.resizeFn
		}

		var reason parent.ExitReason
		var passthroughErr error
		return s.adapter.TrackPromise(s.ctx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
				_, err := func(ctx context.Context) (any, error) {
			reason, passthroughErr = s.mgr.Passthrough(s.ctx, cfg)
			return nil, nil
		}(ctx)
				if err != nil {
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return rt.NewGoError(err) })
					return
				}
				_ = settle.Settle(false, func(rt *goja.Runtime) any {
			s.swappedOnce = true
			s.SetInPassthrough(false)

			s.dispatchEventOnLoop(EventFocus, map[string]any{
				"side": "osm", "action": "return",
			})

			res := map[string]any{
				"reason": exitReasonString(reason),
			}
			if passthroughErr != nil {
				res["error"] = passthroughErr.Error()
			}

			s.dispatchEventOnLoop(EventExit, map[string]any{
				"reason": exitReasonString(reason),
				"pane":   "agent",
			})

			if id := s.mgr.ActiveID(); id != 0 {
				if snap := s.mgr.Snapshot(id); snap != nil {
					res["childOutput"] = snap.GetPlainText()
				}
			}
			return res
		})
			})
	})

	_ = obj.Set("screenshot", func() string {
		id := s.mgr.ActiveID()
		if id == 0 {
			return ""
		}
		snap := s.mgr.Snapshot(id)
		if snap == nil {
			return ""
		}
		return snap.GetPlainText()
	})

	_ = obj.Set("childScreen", func() string {
		id := s.mgr.ActiveID()
		if id == 0 {
			return ""
		}
		snap := s.mgr.Snapshot(id)
		if snap == nil {
			return ""
		}
		return snap.GetANSI()
	})

	_ = obj.Set("writeToChild", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(s.runtime.NewTypeError("writeToChild: data argument is required"))
		}
		data := []byte(call.Argument(0).String())
		if err := s.mgr.Input(data); err != nil {
			panic(s.runtime.NewGoError(err))
		}
		return s.runtime.ToValue(len(data))
	})

	_ = obj.Set("session", func() goja.Value {
		sessionObj := s.runtime.NewObject()

		_ = sessionObj.Set("isRunning", func() bool {
			return s.mgr.ActiveID() != 0
		})

		_ = sessionObj.Set("isDone", func() bool {
			id := s.mgr.ActiveID()
			if id == 0 {
				return true
			}
			for _, info := range s.mgr.Sessions() {
				if info.ID == id {
					return info.State == parent.SessionExited || info.State == parent.SessionClosed
				}
			}
			return true
		})

		_ = sessionObj.Set("output", func() string {
			id := s.mgr.ActiveID()
			if id == 0 {
				return ""
			}
			snap := s.mgr.Snapshot(id)
			if snap == nil {
				return ""
			}
			return snap.GetPlainText()
		})

		_ = sessionObj.Set("screen", func() string {
			id := s.mgr.ActiveID()
			if id == 0 {
				return ""
			}
			snap := s.mgr.Snapshot(id)
			if snap == nil {
				return ""
			}
			return snap.GetANSI()
		})

		_ = sessionObj.Set("target", func() goja.Value {
			result := s.runtime.NewObject()
			_ = result.Set("id", s.activeSessionTarget.ID)
			_ = result.Set("name", s.activeSessionTarget.Name)
			_ = result.Set("kind", string(s.activeSessionTarget.Kind))
			return result
		})

		_ = sessionObj.Set("setTarget", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
				panic(s.runtime.NewTypeError("setTarget: target object is required"))
			}
			tObj := call.Argument(0).ToObject(s.runtime)
			if v := tObj.Get("name"); v != nil && !goja.IsUndefined(v) {
				s.activeSessionTarget.Name = v.String()
			}
			if v := tObj.Get("kind"); v != nil && !goja.IsUndefined(v) {
				s.activeSessionTarget.Kind = parent.SessionKind(v.String())
			}
			if v := tObj.Get("id"); v != nil && !goja.IsUndefined(v) {
				s.activeSessionTarget.ID = v.String()
			}
			return goja.Undefined()
		})

		_ = sessionObj.Set("write", func(data string) {
			if err := s.mgr.Input([]byte(data)); err != nil {
				panic(s.runtime.NewGoError(err))
			}
		})

		_ = sessionObj.Set("resize", func(rows, cols int) {
			if err := s.mgr.Resize(rows, cols); err != nil {
				panic(s.runtime.NewGoError(err))
			}
		})

		return sessionObj
	})

	_ = obj.Set("fromModel", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(s.runtime.NewTypeError("fromModel requires a model argument"))
		}
		model := call.Argument(0)

		altScreen := true
		toggleKeyByte := int(s.toggleKey)
		var cfgObj *goja.Object
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			cfgObj = call.Argument(1).ToObject(s.runtime)
			if cfgObj != nil {
				if v := cfgObj.Get("altScreen"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					altScreen = v.ToBoolean()
				}
				if v := cfgObj.Get("toggleKey"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					toggleKeyByte = int(v.ToInteger())
				}
			}
		}

		result := s.runtime.NewObject()
		_ = result.Set("model", model)

		runOpts := s.runtime.NewObject()
		_ = runOpts.Set("altScreen", altScreen)
		_ = runOpts.Set("toggleKey", toggleKeyByte)

		var originalOnToggle goja.Callable
		if cfgObj != nil {
			if v := cfgObj.Get("onToggle"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				if fn, ok := goja.AssertFunction(v); ok {
					originalOnToggle = fn
				}
			}
		}

		_ = runOpts.Set("onToggle", func(fc goja.FunctionCall) goja.Value {
			s.SetInPassthrough(true)
			if originalOnToggle != nil {
				_, _ = originalOnToggle(goja.Undefined(), fc.Arguments...)
			}

			if s.mgr.ActiveID() == 0 {
				s.SetInPassthrough(false)
				return goja.Undefined()
			}

			s.dispatchEventOnLoop(EventFocus, map[string]any{
				"side": "agent", "action": "enter",
			})

			cfg := parent.PassthroughConfig{
				TerminalIO: parent.TerminalIO{
					Stdin:         s.stdin,
					Stdout:        s.stdout,
					TermFd:        s.termFd,
					BlockingGuard: parent.DefaultBlockingGuard(),
				},
				PassthroughOptions: parent.PassthroughOptions{
					ToggleKey: byte(toggleKeyByte),
					TermState: ptyio.RealTermState{},
				},
				ResizeConfig: parent.ResizeConfig{
					RestoreScreen: s.swappedOnce,
				},
			}
			if s.statusEnabled {
				cfg.StatusBar = s.sb
			}
			if s.resizeFn != nil {
				cfg.ResizeFn = s.resizeFn
			}

			var reason parent.ExitReason
			var passthroughErr error
			return s.adapter.TrackPromise(s.ctx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
				_, err := func(ctx context.Context) (any, error) {
				reason, passthroughErr = s.mgr.Passthrough(s.ctx, cfg)
				return nil, nil
			}(ctx)
				if err != nil {
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return rt.NewGoError(err) })
					return
				}
				_ = settle.Settle(false, func(rt *goja.Runtime) any {
				s.swappedOnce = true
				s.SetInPassthrough(false)

				res := map[string]any{
					"reason": exitReasonString(reason),
				}
				if passthroughErr != nil {
					res["error"] = passthroughErr.Error()
				}

				s.dispatchEventOnLoop(EventFocus, map[string]any{
					"side": "osm", "action": "return",
				})

				return res
			})
			})
		})

		_ = result.Set("options", runOpts)
		return result
	})

	_ = obj.Set("activeSide", func() string {
		if s.IsPassthrough() {
			return "agent"
		}
		return "osm"
	})

	_ = obj.Set("isPassthrough", func() bool {
		return s.IsPassthrough()
	})
}

// registerStatusMethods registers status bar and event methods: setStatus,
// setToggleKey, setStatusEnabled, setResizeFunc, setStatusColors,
// setStatusPosition, addEventListener, removeEventListener, dispatchEvent,
// on, off, pollEvents.
func registerStatusMethods(obj *goja.Object, s *muxState) {
	_ = obj.Set("setStatus", func(text string) {
		s.sb.SetStatus(text)
	})

	_ = obj.Set("setToggleKey", func(k int) {
		s.toggleKey = byte(k)
		s.sb.SetToggleKey(s.toggleKey)
	})

	_ = obj.Set("setStatusEnabled", func(b bool) {
		s.statusEnabled = b
	})

	_ = obj.Set("setStatusColors", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(s.runtime.NewTypeError("setStatusColors requires 1 argument (options object)"))
		}
		opts := call.Argument(0).ToObject(s.runtime)
		fg := ""
		if v := opts.Get("fg"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			fg = v.String()
		}
		bg := ""
		if v := opts.Get("bg"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			bg = v.String()
		}
		if err := s.sb.SetColors(fg, bg); err != nil {
			panic(s.runtime.NewTypeError("setStatusColors: " + err.Error()))
		}
		return obj
	})

	_ = obj.Set("setStatusPosition", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("setStatusPosition requires 1 argument (\"top\" or \"bottom\")"))
		}
		pos, ok := statusbar.ParsePosition(call.Argument(0).String())
		if !ok {
			panic(s.runtime.NewTypeError("setStatusPosition: must be \"top\" or \"bottom\""))
		}
		s.sb.SetPosition(pos)
		return obj
	})

	_ = obj.Set("renderStatusBar", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("renderStatusBar requires at least 1 argument (width)"))
		}
		width := int(call.Argument(0).ToInteger())
		left := ""
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			left = call.Argument(1).String()
		}
		right := ""
		if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) {
			right = call.Argument(2).String()
		}
		return s.runtime.ToValue(s.sb.RenderLine(width, left, right))
	})

	_ = obj.Set("setResizeFunc", func(fn func(int, int)) {
		s.resizeFn = func(rows, cols uint16) error {
			fn(int(rows), int(cols))
			s.dispatchEventOnLoop(EventResize, map[string]any{
				"rows": int(rows),
				"cols": int(cols),
			})
			return nil
		}
	})

	_ = obj.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
		if s.addListener == nil {
			panic(s.runtime.NewTypeError("addEventListener: EventTarget not initialized"))
		}
		if len(call.Arguments) < 2 {
			panic(s.runtime.NewTypeError("addEventListener: requires (event, callback)"))
		}
		if _, ok := goja.AssertFunction(call.Argument(1)); !ok {
			panic(s.runtime.NewTypeError("addEventListener: callback must be a function"))
		}
		_, _ = s.addListener(s.jsEventTarget, call.Argument(0), call.Argument(1))
		return goja.Undefined()
	})

	_ = obj.Set("removeEventListener", func(call goja.FunctionCall) goja.Value {
		if s.removeListener == nil {
			panic(s.runtime.NewTypeError("removeEventListener: EventTarget not initialized"))
		}
		if len(call.Arguments) < 2 {
			panic(s.runtime.NewTypeError("removeEventListener: requires (event, callback)"))
		}
		_, _ = s.removeListener(s.jsEventTarget, call.Argument(0), call.Argument(1))
		return goja.Undefined()
	})

	_ = obj.Set("dispatchEvent", func(call goja.FunctionCall) goja.Value {
		if s.dispatch == nil {
			panic(s.runtime.NewTypeError("dispatchEvent: EventTarget not initialized"))
		}
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("dispatchEvent: requires an event"))
		}
		res, _ := s.dispatch(s.jsEventTarget, call.Argument(0))
		return res
	})

	_ = obj.Set("on", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(s.runtime.NewTypeError("on: requires (event, callback)"))
		}
		eventType := call.Argument(0).String()
		if !isValidEventType(eventType) {
			panic(s.runtime.NewTypeError(fmt.Sprintf("on: unknown event %q", eventType)))
		}
		if _, ok := goja.AssertFunction(call.Argument(1)); !ok {
			panic(s.runtime.NewTypeError("on: callback must be a function"))
		}
		cb := call.Argument(1)

		s.mu.Lock()
		s.nextOnID++
		id := s.nextOnID
		s.onListeners[id] = &onListener{eventType: eventType, callback: cb}
		s.mu.Unlock()

		_, _ = s.addListener(s.jsEventTarget, call.Argument(0), cb)
		return s.runtime.ToValue(id)
	})

	_ = obj.Set("off", func(id int) bool {
		s.mu.Lock()
		l, ok := s.onListeners[id]
		if ok {
			delete(s.onListeners, id)
		}
		s.mu.Unlock()

		if !ok {
			return false
		}
		if s.removeListener == nil {
			return false
		}
		_, _ = s.removeListener(s.jsEventTarget, s.runtime.ToValue(l.eventType), l.callback)
		return true
	})

	_ = obj.Set("pollEvents", func() int {
		return 0
	})
}

// registerPersistenceMethods registers state persistence methods: exportState,
// saveState, loadState, restoreState, removeState, processAlive.
func registerPersistenceMethods(obj *goja.Object, s *muxState) {
	_ = obj.Set("exportState", func(call goja.FunctionCall) goja.Value {
		state, err := s.mgr.ExportState()
		if err != nil {
			panic(s.runtime.NewGoError(err))
		}
		return s.runtime.ToValue(persistedStateToJS(state))
	})

	_ = obj.Set("saveState", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("saveState: path argument is required"))
		}
		path := call.Argument(0).String()
		if path == "" {
			panic(s.runtime.NewTypeError("saveState: path must be non-empty"))
		}
		state, err := s.mgr.ExportState()
		if err != nil {
			panic(s.runtime.NewGoError(err))
		}
		if err := parent.SaveManagerState(path, state); err != nil {
			panic(s.runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	_ = obj.Set("loadState", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("loadState: path argument is required"))
		}
		path := call.Argument(0).String()
		if path == "" {
			panic(s.runtime.NewTypeError("loadState: path must be non-empty"))
		}
		state, err := parent.LoadManagerState(path)
		if err != nil {
			panic(s.runtime.NewGoError(err))
		}
		if state == nil {
			return goja.Null()
		}
		return s.runtime.ToValue(persistedStateToJS(state))
	})

	_ = obj.Set("restoreState", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("restoreState: state argument is required"))
		}
		stateVal := call.Argument(0)
		if stateVal == goja.Null() || stateVal == goja.Undefined() {
			panic(s.runtime.NewTypeError("restoreState: state must not be null"))
		}

		stateMap := stateVal.ToObject(s.runtime)
		version := stateMap.Get("version").String()
		activeID := uint64(stateMap.Get("activeId").ToInteger())
		termRows := int(stateMap.Get("termRows").ToInteger())
		termCols := int(stateMap.Get("termCols").ToInteger())

		sessionsVal := stateMap.Get("sessions")
		var sessionsKeys []string
		var sessionsObj *goja.Object
		if sessionsVal != nil && !goja.IsUndefined(sessionsVal) && sessionsVal != goja.Null() {
			sessionsObj = sessionsVal.ToObject(s.runtime)
			sessionsKeys = sessionsObj.Keys()
		}

		state := &parent.PersistedManagerState{
			Version:  version,
			ActiveID: activeID,
			TermRows: termRows,
			TermCols: termCols,
		}
		for _, key := range sessionsKeys {
			sessVal := sessionsObj.Get(key)
			if sessVal == nil || goja.IsUndefined(sessVal) || goja.IsNull(sessVal) {
				continue
			}
			sessObj := sessVal.ToObject(s.runtime)
			ps := parent.PersistedSession{
				SessionID: uint64(sessObj.Get("sessionId").ToInteger()),
				Rows:      int(sessObj.Get("rows").ToInteger()),
				Cols:      int(sessObj.Get("cols").ToInteger()),
			}
			if v := sessObj.Get("lastActive"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				ps.LastActive = time.UnixMilli(v.ToInteger())
			}
			if v := sessObj.Get("state"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				ps.State = parent.SessionState(int(v.ToInteger()))
			}
			if v := sessObj.Get("pid"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				ps.PID = int(v.ToInteger())
			}
			if v := sessObj.Get("command"); v != nil && !goja.IsUndefined(v) {
				ps.Command = v.String()
			}
			if v := sessObj.Get("args"); v != nil && !goja.IsUndefined(v) && v != goja.Null() {
				argsObj := v.ToObject(s.runtime)
				if lenVal := argsObj.Get("length"); lenVal != nil && !goja.IsUndefined(lenVal) {
					arrLen := lenVal.ToInteger()
					for i := range arrLen {
						argV := argsObj.Get(fmt.Sprintf("%d", i))
						if argV != nil && !goja.IsUndefined(argV) && !goja.IsNull(argV) {
							ps.Args = append(ps.Args, argV.String())
						}
					}
				}
			}
			if v := sessObj.Get("dir"); v != nil && !goja.IsUndefined(v) && v != goja.Null() {
				ps.Dir = v.String()
			}
			if v := sessObj.Get("env"); v != nil && !goja.IsUndefined(v) && v != goja.Null() {
				envObj := v.ToObject(s.runtime)
				for _, key := range envObj.Keys() {
					if ps.Env == nil {
						ps.Env = make(map[string]string)
					}
					ps.Env[key] = envObj.Get(key).String()
				}
			}
			targetVal := sessObj.Get("target")
			if targetVal != nil && !goja.IsUndefined(targetVal) && targetVal != goja.Null() {
				targetObj := targetVal.ToObject(s.runtime)
				ps.Target = parent.SessionTarget{}
				if v := targetObj.Get("name"); v != nil && !goja.IsUndefined(v) {
					ps.Target.Name = v.String()
				}
				if v := targetObj.Get("id"); v != nil && !goja.IsUndefined(v) {
					ps.Target.ID = v.String()
				}
				if v := targetObj.Get("kind"); v != nil && !goja.IsUndefined(v) {
					ps.Target.Kind = parent.SessionKind(v.String())
				}
			}
			state.Sessions = append(state.Sessions, ps)
		}

		result, err := s.mgr.RestoreFromState(state, func(ps parent.PersistedSession) (parent.InteractiveSession, error) {
			cfg := parent.CaptureConfig{
				Command: ps.Command,
				Args:    ps.Args,
				Dir:     ps.Dir,
				Rows:    ps.Rows,
				Cols:    ps.Cols,
			}
			for k, v := range ps.Env {
				if cfg.Env == nil {
					cfg.Env = make(map[string]string)
				}
				cfg.Env[k] = v
			}
			if ps.Target.Name != "" {
				cfg.Name = ps.Target.Name
			}
			if ps.Target.Kind != "" {
				cfg.Kind = ps.Target.Kind
			}
			cs := parent.NewCaptureSession(cfg)
			return cs, nil
		})
		if err != nil {
			panic(s.runtime.NewGoError(err))
		}

		restored := make([]any, len(result.Restored))
		for i, id := range result.Restored {
			restored[i] = uint64(id)
		}
		failed := make([]any, len(result.Failed))
		for i, f := range result.Failed {
			failed[i] = map[string]any{
				"sessionId": uint64(f.SessionID),
				"error": func() string {
					if f.Error != nil {
						return f.Error.Error()
					}
					return ""
				}(),
			}
		}
		return s.runtime.ToValue(map[string]any{
			"restored": restored,
			"failed":   failed,
		})
	})

	_ = obj.Set("removeState", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("removeState: path argument is required"))
		}
		path := call.Argument(0).String()
		if path == "" {
			panic(s.runtime.NewTypeError("removeState: path must be non-empty"))
		}
		if err := parent.RemoveManagerState(path); err != nil {
			panic(s.runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	_ = obj.Set("processAlive", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("processAlive: pid argument is required"))
		}
		pid := int(call.Argument(0).ToInteger())
		return s.runtime.ToValue(parent.ProcessAlive(pid))
	})
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

// registerPaneMethods registers pane management methods: splitHorizontal,
// splitVertical, closePane, focusPaneUp, focusPaneDown, focusPaneLeft,
// focusPaneRight, panes, activePaneId, resizePane.
func registerPaneMethods(obj *goja.Object, s *muxState) {
	_ = obj.Set("splitHorizontal", func(call goja.FunctionCall) goja.Value {
		direction := parent.SplitDown
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			opts := call.Argument(0).ToObject(s.runtime)
			if v := opts.Get("session"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				session := unwrapInteractiveSession(v.ToObject(s.runtime))
				if session == nil {
					panic(s.runtime.NewTypeError("splitHorizontal: session must be an InteractiveSession wrapper"))
				}
				var target parent.SessionTarget
				if v := opts.Get("target"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					tObj := v.ToObject(s.runtime)
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
				id, err := s.mgr.NewPane(session, target, direction)
				if err != nil {
					panic(s.runtime.NewGoError(err))
				}
				return s.runtime.ToValue(uint64(id))
			}
		}
		panic(s.runtime.NewTypeError("splitHorizontal: options with session are required"))
	})

	_ = obj.Set("splitVertical", func(call goja.FunctionCall) goja.Value {
		direction := parent.SplitRight
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			opts := call.Argument(0).ToObject(s.runtime)
			if v := opts.Get("session"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				session := unwrapInteractiveSession(v.ToObject(s.runtime))
				if session == nil {
					panic(s.runtime.NewTypeError("splitVertical: session must be an InteractiveSession wrapper"))
				}
				var target parent.SessionTarget
				if v := opts.Get("target"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					tObj := v.ToObject(s.runtime)
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
				id, err := s.mgr.NewPane(session, target, direction)
				if err != nil {
					panic(s.runtime.NewGoError(err))
				}
				return s.runtime.ToValue(uint64(id))
			}
		}
		panic(s.runtime.NewTypeError("splitVertical: options with session are required"))
	})

	_ = obj.Set("closePane", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("closePane: pane ID argument is required"))
		}
		id := parent.PaneID(call.Argument(0).ToInteger())
		if err := s.mgr.ClosePane(id); err != nil {
			panic(s.runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	_ = obj.Set("addPaneToWindow", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("addPaneToWindow requires at least 1 argument (session)"))
		}
		sessionObj := call.Argument(0).ToObject(s.runtime)
		session := unwrapInteractiveSession(sessionObj)
		if session == nil {
			panic(s.runtime.NewTypeError("addPaneToWindow: first argument must be an InteractiveSession wrapper"))
		}
		var target parent.SessionTarget
		var windowID uint64
		var dir int = 0 // SplitRight
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			optsObj := call.Argument(1).ToObject(s.runtime)
			if v := optsObj.Get("target"); v != nil && !goja.IsUndefined(v) {
				tObj := v.ToObject(s.runtime)
				if v := tObj.Get("name"); v != nil && !goja.IsUndefined(v) {
					target.Name = v.String()
				}
				if v := tObj.Get("kind"); v != nil && !goja.IsUndefined(v) {
					target.Kind = parent.SessionKind(v.String())
				}
			}
			if v := optsObj.Get("windowId"); v != nil && !goja.IsUndefined(v) {
				windowID = uint64(v.ToInteger())
			}
			if v := optsObj.Get("direction"); v != nil && !goja.IsUndefined(v) {
				dir = int(v.ToInteger())
			}
		}
		paneID, err := s.mgr.AddPaneToWindow(session, target, parent.WindowID(windowID), parent.SplitDirection(dir))
		if err != nil {
			panic(s.runtime.NewGoError(err))
		}
		return s.runtime.ToValue(uint64(paneID))
	})

	_ = obj.Set("focusPaneUp", func() goja.Value {
		nextID := s.mgr.FocusNextPane(parent.NavUp)
		return s.runtime.ToValue(uint64(nextID))
	})

	_ = obj.Set("focusPaneDown", func() goja.Value {
		nextID := s.mgr.FocusNextPane(parent.NavDown)
		return s.runtime.ToValue(uint64(nextID))
	})

	_ = obj.Set("focusPaneLeft", func() goja.Value {
		nextID := s.mgr.FocusNextPane(parent.NavLeft)
		return s.runtime.ToValue(uint64(nextID))
	})

	_ = obj.Set("focusPaneRight", func() goja.Value {
		nextID := s.mgr.FocusNextPane(parent.NavRight)
		return s.runtime.ToValue(uint64(nextID))
	})

	_ = obj.Set("panes", func() goja.Value {
		panes := s.mgr.Panes()
		result := make([]map[string]any, len(panes))
		for i, p := range panes {
			result[i] = map[string]any{
				"id":        uint64(p.ID),
				"sessionId": uint64(p.SessionID),
				"title":     p.Title,
				"focus":     p.Focus,
				"exited":    p.Exited,
				"geometry": map[string]any{
					"row":  p.Geometry.Row,
					"col":  p.Geometry.Col,
					"rows": p.Geometry.Rows,
					"cols": p.Geometry.Cols,
				},
			}
		}
		return s.runtime.ToValue(result)
	})

	_ = obj.Set("activePaneId", func() goja.Value {
		id := s.mgr.ActivePaneID()
		return s.runtime.ToValue(uint64(id))
	})

	_ = obj.Set("resizePane", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(s.runtime.NewTypeError("resizePane: requires (paneId, ratio) arguments"))
		}
		id := parent.PaneID(call.Argument(0).ToInteger())
		ratio := call.Argument(1).ToFloat()
		if err := s.mgr.ResizePane(id, ratio); err != nil {
			panic(s.runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	_ = obj.Set("focusPaneAt", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(s.runtime.NewTypeError("focusPaneAt: requires (row, col) arguments"))
		}
		row := int(call.Argument(0).ToInteger())
		col := int(call.Argument(1).ToInteger())
		id, err := s.mgr.FocusAt(row, col)
		if err != nil {
			panic(s.runtime.NewGoError(err))
		}
		return s.runtime.ToValue(uint64(id))
	})

	_ = obj.Set("resizePaneAt", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 {
			panic(s.runtime.NewTypeError("resizePaneAt: requires (row, col, ratio) arguments"))
		}
		row := int(call.Argument(0).ToInteger())
		col := int(call.Argument(1).ToInteger())
		ratio := call.Argument(2).ToFloat()
		if err := s.mgr.ResizePaneAt(row, col, ratio); err != nil {
			panic(s.runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	_ = obj.Set("resizePaneDelta", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 {
			panic(s.runtime.NewTypeError("resizePaneDelta: requires (paneId, direction, delta) arguments"))
		}
		id := parent.PaneID(call.Argument(0).ToInteger())
		direction := call.Argument(1).String()
		switch direction {
		case "left", "right", "up", "down":
		default:
			panic(s.runtime.NewTypeError("resizePaneDelta: direction must be one of left, right, up, down"))
		}
		delta := int(call.Argument(2).ToInteger())
		if delta < 0 {
			panic(s.runtime.NewTypeError("resizePaneDelta: delta must be non-negative"))
		}
		if err := s.mgr.ResizePaneDelta(id, direction, delta); err != nil {
			panic(s.runtime.NewGoError(err))
		}
		return goja.Undefined()
	})
}

func prefixActionKindFromName(name string) parent.PrefixActionKind {
	switch name {
	case "NewWindow":
		return parent.PrefixActionNewWindow
	case "NextWindow":
		return parent.PrefixActionNextWindow
	case "PrevWindow":
		return parent.PrefixActionPrevWindow
	case "Detach":
		return parent.PrefixActionDetach
	case "ZoomPane":
		return parent.PrefixActionZoomPane
	case "ClosePane":
		return parent.PrefixActionClosePane
	case "SplitHorizontal":
		return parent.PrefixActionSplitHorizontal
	case "SplitVertical":
		return parent.PrefixActionSplitVertical
	case "CopyMode":
		return parent.PrefixActionCopyMode
	case "ListKeys":
		return parent.PrefixActionListKeys
	case "RenameWindow":
		return parent.PrefixActionRenameWindow
	case "Cancel":
		return parent.PrefixActionCancel
	default:
		return parent.PrefixActionNone
	}
}
