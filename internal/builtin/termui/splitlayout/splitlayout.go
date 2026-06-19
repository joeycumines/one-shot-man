// Package splitlayout provides JavaScript bindings for
// [github.com/joeycumines/one-shot-man/internal/termui/splitlayout].
//
// The module is exposed as "osm:termui/splitlayout" and provides a
// chainable API for managing split terminal layouts composed of multiple
// termmux sessions.
//
// # JavaScript API
//
//	const termmux = require('osm:termmux');
//	const sl = require('osm:termui/splitlayout');
//
//	const mgr = termmux.newSessionManager();
//	const layout = sl.splitLayout({
//	  manager: mgr,
//	  direction: 'horizontal',
//	  ratios: [0.5, 0.5],
//	  width: 80,
//	  height: 24
//	});
//
//	layout.addPane({sessionId: 1})
//	      .addPane({sessionId: 2});
//
//	const model = layout.asBubbleteaModel();
//	layout.close();  // release resources
package splitlayout

import (
	"github.com/dop251/goja"

	termmuxmod "github.com/joeycumines/one-shot-man/internal/builtin/termmux"
	"github.com/joeycumines/one-shot-man/internal/termmux"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/layout"
	"github.com/joeycumines/one-shot-man/internal/termui/splitlayout"
)

// Require returns a CommonJS native module under "osm:termui/splitlayout".
func Require() func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		// splitLayout({manager, direction, ratios, width, height}) — creates a SplitLayout
		_ = exports.Set("splitLayout", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
				panic(runtime.NewTypeError("splitlayout requires a config object {manager, direction, ratios, width, height}"))
			}

			config := call.Argument(0).ToObject(runtime)
			if config == nil {
				panic(runtime.NewTypeError("splitlayout: config must be an object"))
			}

			managerVal := config.Get("manager")
			if managerVal == nil || goja.IsUndefined(managerVal) || goja.IsNull(managerVal) {
				panic(runtime.NewTypeError("splitlayout: manager is required (pass a termmux SessionManager)"))
			}
			managerObj := managerVal.ToObject(runtime)
			manager := termmuxmod.UnwrapSessionManager(managerObj)
			if manager == nil {
				panic(runtime.NewTypeError("splitlayout: manager must be a termmux SessionManager object"))
			}

			var d layout.Direction
			if v := config.Get("direction"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				d = parseDirection(runtime, v)
			} else {
				d = layout.Horizontal
			}

			var ratios []float64
			if v := config.Get("ratios"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				ratios = extractRatios(runtime, v)
			}

			bounds := coordinate.Rect{}
			if v := config.Get("width"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				bounds.Size.Width = int(v.ToInteger())
			}
			if v := config.Get("height"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				bounds.Size.Height = int(v.ToInteger())
			}
			if v := config.Get("bounds"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				bounds = extractRect(runtime, v)
			}

			opts := []splitlayout.SplitLayoutOption{
				splitlayout.WithDirection(d),
			}
			if ratios != nil {
				opts = append(opts, splitlayout.WithRatios(ratios))
			}

			sl := splitlayout.NewSplitLayout(manager, bounds, opts...)

			return createSplitLayoutObject(runtime, sl)
		})
	}
}

// createSplitLayoutObject wraps a *splitlayout.SplitLayout as a Goja Object
// with chainable configuration methods and non-chainable accessors.
func createSplitLayoutObject(runtime *goja.Runtime, sl *splitlayout.SplitLayout) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("_type", "termui/splitlayout")
	_ = obj.Set("_goLayout", sl)

	// addPane({sessionId}) — adds a pane, chainable
	_ = obj.Set("addPane", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(runtime.NewTypeError("addPane requires a config object {sessionId}"))
		}
		cfg := call.Argument(0).ToObject(runtime)
		sessionIDVal := cfg.Get("sessionId")
		if sessionIDVal == nil || goja.IsUndefined(sessionIDVal) || goja.IsNull(sessionIDVal) {
			panic(runtime.NewTypeError("addPane: sessionId is required"))
		}
		sl.AddPane(termmux.SessionID(sessionIDVal.ToInteger()))
		return obj
	})

	// removePane({sessionId}) — removes a pane, returns error on failure
	_ = obj.Set("removePane", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(runtime.NewTypeError("removePane requires a config object {sessionId}"))
		}
		cfg := call.Argument(0).ToObject(runtime)
		sessionIDVal := cfg.Get("sessionId")
		if sessionIDVal == nil || goja.IsUndefined(sessionIDVal) || goja.IsNull(sessionIDVal) {
			panic(runtime.NewTypeError("removePane: sessionId is required"))
		}
		if err := sl.RemovePane(termmux.SessionID(sessionIDVal.ToInteger())); err != nil {
			panic(runtime.NewGoError(err))
		}
		return obj
	})

	// direction(d) — sets layout direction, chainable
	_ = obj.Set("direction", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			return obj
		}
		d := parseDirection(runtime, call.Argument(0))
		sl.SetDirection(d)
		return obj
	})

	// ratios(r1, r2, ...) — sets ratios from varargs, chainable
	_ = obj.Set("ratios", func(call goja.FunctionCall) goja.Value {
		r := make([]float64, len(call.Arguments))
		for i, arg := range call.Arguments {
			r[i] = arg.ToFloat()
		}
		sl.SetRatios(r)
		return obj
	})

	// focus(sessionId) — focuses the pane with the given session ID
	_ = obj.Set("focus", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			return goja.Undefined()
		}
		sl.FocusPane(termmux.SessionID(call.Argument(0).ToInteger()))
		return goja.Undefined()
	})

	// panes() — returns an array of session IDs
	_ = obj.Set("panes", func(call goja.FunctionCall) goja.Value {
		ids := sl.Panes()
		vals := make([]any, len(ids))
		for i, id := range ids {
			vals[i] = uint64(id)
		}
		return runtime.NewArray(vals...)
	})

	// paneBounds(sessionId) — returns {x, y, width, height} for the pane
	_ = obj.Set("paneBounds", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(runtime.NewTypeError("paneBounds requires a sessionId argument"))
		}
		r, err := sl.PaneBounds(termmux.SessionID(call.Argument(0).ToInteger()))
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		return createRectJSObject(runtime, &r)
	})

	// asBubbleteaModel() — returns the underlying bubbletea model for use
	// with the tui/bubbletea system.
	//
	// Returns an opaque wrapper with:
	//   _type: "bubbleteaGoModel"  — identifies this as a Go-implemented tea.Model
	//   _goModel: *SplitLayout      — the underlying Go model (non-enumerable)
	_ = obj.Set("asBubbleteaModel", func(call goja.FunctionCall) goja.Value {
		wrapper := runtime.NewObject()
		_ = wrapper.Set("_type", "bubbleteaGoModel")
		_ = wrapper.DefineDataProperty("_goModel", runtime.ToValue(sl),
			goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)
		return wrapper
	})

	// close() — releases resources held by the layout, non-chainable
	_ = obj.Set("close", func(call goja.FunctionCall) goja.Value {
		sl.Close()
		return goja.Undefined()
	})

	return obj
}

// parseDirection converts a JS string value to a layout.Direction.
func parseDirection(runtime *goja.Runtime, val goja.Value) layout.Direction {
	s := val.String()
	switch s {
	case "horizontal":
		return layout.Horizontal
	case "vertical":
		return layout.Vertical
	default:
		panic(runtime.NewTypeError("direction must be 'horizontal' or 'vertical', got: " + s))
	}
}

// extractRatios extracts a slice of float64 from a JS array value.
func extractRatios(_ *goja.Runtime, val goja.Value) []float64 {
	arr, ok := val.Export().([]any)
	if !ok {
		return nil
	}
	r := make([]float64, len(arr))
	for i, v := range arr {
		if f, ok := v.(float64); ok {
			r[i] = f
		}
	}
	return r
}

// extractRect extracts a coordinate.Rect from a Goja value.
func extractRect(runtime *goja.Runtime, val goja.Value) coordinate.Rect {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return coordinate.Rect{}
	}
	obj := val.ToObject(runtime)
	if obj == nil {
		return coordinate.Rect{}
	}
	return coordinate.Rect{
		Position: coordinate.Position{
			X: int(obj.Get("x").ToInteger()),
			Y: int(obj.Get("y").ToInteger()),
		},
		Size: coordinate.Size{
			Width:  int(obj.Get("width").ToInteger()),
			Height: int(obj.Get("height").ToInteger()),
		},
	}
}

// createRectJSObject creates a JS Rect object from a coordinate.Rect.
func createRectJSObject(runtime *goja.Runtime, r *coordinate.Rect) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("_type", "termui/coordinate/rect")

	_ = obj.DefineAccessorProperty("x",
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return runtime.ToValue(r.Position.X)
		}),
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			r.Position.X = int(call.Argument(0).ToInteger())
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE,
	)

	_ = obj.DefineAccessorProperty("y",
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return runtime.ToValue(r.Position.Y)
		}),
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			r.Position.Y = int(call.Argument(0).ToInteger())
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE,
	)

	_ = obj.DefineAccessorProperty("width",
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return runtime.ToValue(r.Size.Width)
		}),
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			r.Size.Width = int(call.Argument(0).ToInteger())
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE,
	)

	_ = obj.DefineAccessorProperty("height",
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return runtime.ToValue(r.Size.Height)
		}),
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			r.Size.Height = int(call.Argument(0).ToInteger())
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE,
	)

	_ = obj.Set("toString", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(r.String())
	})

	return obj
}
