// Package termmux provides JavaScript bindings for the terminal multiplexer,
// registered as the "osm:termmux" native module. It wraps
// [github.com/joeycumines/one-shot-man/internal/termmux] to expose pane
// management, passthrough control, and configuration to Goja scripts.
package termmux

import (
	"context"
	"fmt"
	"io"
	"math"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

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

func errToStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// finiteNumberOption returns value as a finite number or panics with a
// TypeError naming the offending option. Values must be integral: capture
// rows are integers, so 1.9 is rejected rather than silently truncated.
func finiteNumberOption(runtime *goja.Runtime, value goja.Value, name string) float64 {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		panic(runtime.NewTypeError("capture: " + name + " must be a finite number"))
	}
	switch value.Export().(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
	default:
		panic(runtime.NewTypeError("capture: " + name + " must be a finite number"))
	}
	f := value.ToFloat()
	if math.IsNaN(f) || math.IsInf(f, 0) {
		panic(runtime.NewTypeError("capture: " + name + " must be a finite number"))
	}
	if f != math.Trunc(f) {
		panic(runtime.NewTypeError("capture: " + name + " must be an integer"))
	}
	return f
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

		_ = exports.Set("mouseDrag", func() goja.Value { return newMouseDrag(ctx, adapter, runtime) })

		_ = exports.Set("handleMouseDrag", func(call goja.FunctionCall) goja.Value {
			return handleMouseDrag(ctx, adapter, runtime, call)
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

			if adapter == nil {
				panic(runtime.NewGoError(fmt.Errorf("handlePrefixKey: event loop adapter is required")))
			}
			return adapter.TrackPromise(ctx, func(_ context.Context, settle gojaeventloop.TrackedSettlement) {
				d := parent.NewPrefixDispatcher(mgr, parent.NewPrefixKeyHandler(""))
				res, err := d.Handle(key)
				if err != nil {
					_ = settle.Settle(true, func(owner *goja.Runtime) any {
						return owner.NewGoError(err)
					})
					return
				}
				_ = settle.Settle(false, func(owner *goja.Runtime) any {
					result := owner.NewObject()
					_ = result.Set("action", res.Action.String())
					_ = result.Set("consumed", res.Consumed)
					_ = result.Set("description", res.Description)
					_ = result.Set("result", res.Result)
					_ = result.Set("listKeys", res.ListKeys)
					return result
				})
			})
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
