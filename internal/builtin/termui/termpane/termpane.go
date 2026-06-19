// Package termpane provides JavaScript bindings for
// [github.com/joeycumines/one-shot-man/internal/termui/termpane].
//
// The module is exposed as "osm:termui/termpane" and provides a TermPane
// bubbletea v2 Model that bridges a termmux session into the bubbletea
// rendering pipeline.
//
// # JavaScript API
//
//	const termmux = require('osm:termmux');
//	const tp = require('osm:termui/termpane');
//	const coord = require('osm:termui/coordinate');
//
//	const mgr = termmux.newSessionManager();
//	const pane = tp.termpane({
//	  manager: mgr,
//	  sessionId: 42,
//	  bounds: coord.rect({x: 0, y: 0, width: 80, height: 24})
//	});
//
//	pane.setBounds(coord.rect({x: 5, y: 5, width: 40, height: 12}));
//	pane.close();
//
//	const model = pane.asBubbleteaModel();
package termpane

import (
	tea "charm.land/bubbletea/v2"
	"github.com/dop251/goja"

	"github.com/joeycumines/one-shot-man/internal/builtin/bubbletea"
	termmuxmod "github.com/joeycumines/one-shot-man/internal/builtin/termmux"
	"github.com/joeycumines/one-shot-man/internal/termmux"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/termpane"
)

// Require returns a CommonJS native module under "osm:termui/termpane".
func Require(runtime *goja.Runtime, module *goja.Object) {
	exports := runtime.NewObject()
	_ = module.Set("exports", exports)

	// termpane({manager, sessionId, bounds}) — creates a TermPane model
	_ = exports.Set("termpane", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(runtime.NewTypeError("termpane requires a config object {manager, sessionId, bounds}"))
		}

		config := call.Argument(0).ToObject(runtime)
		if config == nil {
			panic(runtime.NewTypeError("termpane: config must be an object"))
		}

		managerVal := config.Get("manager")
		if managerVal == nil || goja.IsUndefined(managerVal) || goja.IsNull(managerVal) {
			panic(runtime.NewTypeError("termpane: manager is required (pass a termmux SessionManager)"))
		}
		managerObj := managerVal.ToObject(runtime)
		manager := termmuxmod.UnwrapSessionManager(managerObj)
		if manager == nil {
			panic(runtime.NewTypeError("termpane: manager must be a termmux SessionManager object"))
		}

		sessionIDVal := config.Get("sessionId")
		if sessionIDVal == nil || goja.IsUndefined(sessionIDVal) || goja.IsNull(sessionIDVal) {
			panic(runtime.NewTypeError("termpane: sessionId is required"))
		}
		sessionID := termmux.SessionID(sessionIDVal.ToInteger())

		boundsVal := config.Get("bounds")
		if boundsVal == nil || goja.IsUndefined(boundsVal) || goja.IsNull(boundsVal) {
			panic(runtime.NewTypeError("termpane: bounds is required"))
		}
		bounds := extractRect(runtime, boundsVal)

		m := termpane.NewModel(sessionID, manager, bounds)

		return createTermpaneObject(runtime, m)
	})
}

// extractRect extracts a coordinate.Rect from a Goja value.
// Supports Rect JS objects (from osm:termui/coordinate) and plain objects
// with x, y, width, height properties (duck typing).
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

	// x — accessor property (get/set), maps to Position.X
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

	// y — accessor property (get/set), maps to Position.Y
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

	// width — accessor property (get/set), maps to Size.Width
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

	// height — accessor property (get/set), maps to Size.Height
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

	// toString() — returns string
	_ = obj.Set("toString", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(r.String())
	})

	return obj
}

// createTermpaneObject wraps a *termpane.Model as a Goja Object with
// accessor methods. Internal Go struct fields are not exposed directly;
// all access goes through the Model's exported methods.
func createTermpaneObject(runtime *goja.Runtime, m *termpane.Model) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("_type", "termui/termpane")

	// setBounds(rect) — updates bounds, returns this for chaining
	_ = obj.Set("setBounds", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(runtime.NewTypeError("setBounds requires a rect argument"))
		}
		bounds := extractRect(runtime, call.Argument(0))
		m.SetBounds(bounds)
		return obj
	})

	// sessionId() — returns the session ID as a number (uint64)
	_ = obj.Set("sessionId", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(uint64(m.SessionID()))
	})

	// bounds() — returns a Rect JS object
	_ = obj.Set("bounds", func(call goja.FunctionCall) goja.Value {
		r := m.Bounds()
		return createRectJSObject(runtime, &r)
	})

	// update(msg) — converts a JS BubbleTea message object to a Go tea.Msg,
	// dispatches it to the underlying model, and returns [pane, cmd].
	// The returned cmd is a wrapped Go tea.Cmd that can be returned from a
	// JavaScript BubbleTea update function.
	_ = obj.Set("update", func(call goja.FunctionCall) goja.Value {
		cmd := tea.Cmd(nil)

		if len(call.Arguments) >= 1 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			if msgObj, ok := call.Argument(0).(*goja.Object); ok && msgObj != nil {
				if msg := bubbletea.ParseMsg(runtime, msgObj); msg != nil {
					_, cmd = m.Update(msg)
				}
			}
		}

		return runtime.NewArray(obj, bubbletea.WrapCmd(runtime, cmd))
	})

	_ = obj.Set("view", func(call goja.FunctionCall) goja.Value {
		m.RefreshSnapshot()
		v := m.ANSIView()

		result := runtime.NewObject()
		_ = result.Set("content", v.Content)
		_ = result.Set("gen", m.SnapshotGen())

		if v.Cursor != nil {
			cursor := runtime.NewObject()
			_ = cursor.Set("x", v.Cursor.X)
			_ = cursor.Set("y", v.Cursor.Y)
			_ = cursor.Set("shape", "block")
			_ = cursor.Set("blink", false)
			_ = result.Set("cursor", cursor)
		}

		return result
	})

	// close() — cleanup (unsubscribe from EventBus, stop bridge goroutine)
	_ = obj.Set("close", func(call goja.FunctionCall) goja.Value {
		if err := m.Close(); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// asBubbleteaModel() — returns the underlying bubbletea model for use
	// with the tui/bubbletea system.
	//
	// Returns an opaque wrapper with:
	//   _type: "bubbleteaGoModel"  — identifies this as a Go-implemented tea.Model
	//   _goModel: *termpane.Model   — the underlying Go model (non-enumerable)
	//
	// Go code can extract the original model via Export() and type-assert
	// to tea.Model. The bubbletea JS run() function can be extended to
	// accept "bubbleteaGoModel" wrappers alongside "bubbleteaModel" wrappers.
	_ = obj.Set("asBubbleteaModel", func(call goja.FunctionCall) goja.Value {
		wrapper := runtime.NewObject()
		_ = wrapper.Set("_type", "bubbleteaGoModel")
		// Store the Go model as a non-enumerable data property so it
		// doesn't appear in Object.keys() but is accessible via Export().
		_ = wrapper.DefineDataProperty("_goModel", runtime.ToValue(m),
			goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)
		return wrapper
	})

	return obj
}
