// Package termpane provides JavaScript bindings for
// [github.com/joeycumines/one-shot-man/internal/termui/termpane].
//
// The module is exposed as "osm:termui/termpane" and provides a TermPane
// bubbletea v2 Model that bridges a termmux session into the bubbletea
// rendering pipeline.
//
// # JavaScript API
//
//	const tp = require('osm:termui/termpane');
//	const coord = require('osm:termui/coordinate');
//
//	// Create a TermPane model bound to a termmux session
//	const pane = tp.termpane({
//	  sessionId: 42,
//	  bounds: coord.rect({x: 0, y: 0, width: 80, height: 24})
//	});
//
//	// Accessor methods
//	pane.sessionId();           // 42
//	pane.bounds();              // Rect object {x, y, width, height}
//	pane.setBounds(coord.rect({x: 5, y: 5, width: 40, height: 12})); // returns pane (chaining)
//	pane.close();               // cleanup (unsubscribe, stop goroutine)
//
//	// Integration with bubbletea
//	const model = pane.asBubbleteaModel(); // opaque Go model for tea.run()
//
// # SessionManager
//
// The Require function accepts a *termmux.SessionManager which is required
// to create TermPane models. If nil, the termpane factory will throw a
// TypeError at runtime.
//
// # Bubbletea Integration
//
// The asBubbleteaModel() method returns an opaque wrapper around the Go
// *termpane.Model (which implements tea.Model). The wrapper has:
//
//	_type: "bubbleteaGoModel"  — identifies this as a Go-implemented tea.Model
//	_goModel: *termpane.Model   — the underlying Go model (non-enumerable)
//
// Go code can extract the original model via Export() and type-assert to
// tea.Model. The bubbletea JS run() function can be extended to accept
// "bubbleteaGoModel" wrappers alongside "bubbleteaModel" wrappers.
package termpane

import (
	"github.com/dop251/goja"

	"github.com/joeycumines/one-shot-man/internal/termmux"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/termpane"
)

// Require returns a CommonJS native module under "osm:termui/termpane".
// The manager parameter provides the termmux SessionManager needed to create
// TermPane models. If manager is nil, the termpane factory will throw a
// TypeError when called.
func Require(manager *termmux.SessionManager) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		// termpane({sessionId, bounds}) — creates a TermPane model
		_ = exports.Set("termpane", func(call goja.FunctionCall) goja.Value {
			if manager == nil {
				panic(runtime.NewTypeError("termpane: SessionManager is not available; the osm:termui/termpane module was initialized without a SessionManager"))
			}

			if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
				panic(runtime.NewTypeError("termpane requires a config object {sessionId, bounds}"))
			}

			config := call.Argument(0).ToObject(runtime)
			if config == nil {
				panic(runtime.NewTypeError("termpane: config must be an object"))
			}

			// Parse sessionId (required).
			sessionIDVal := config.Get("sessionId")
			if sessionIDVal == nil || goja.IsUndefined(sessionIDVal) || goja.IsNull(sessionIDVal) {
				panic(runtime.NewTypeError("termpane: sessionId is required"))
			}
			sessionID := termmux.SessionID(sessionIDVal.ToInteger())

			// Parse bounds (required).
			boundsVal := config.Get("bounds")
			if boundsVal == nil || goja.IsUndefined(boundsVal) || goja.IsNull(boundsVal) {
				panic(runtime.NewTypeError("termpane: bounds is required"))
			}
			bounds := extractRect(runtime, boundsVal)

			// Create the Go model.
			m := termpane.NewModel(sessionID, manager, bounds)

			return createTermpaneObject(runtime, m)
		})
	}
}

// extractRect extracts a coordinate.Rect from a Goja value.
// Supports Rect JS objects (from osm:termui/coordinate) and plain objects
// with x, y, width, height properties (duck typing).
// Mirrors the pattern from osm:termui/coordinate and osm:termui/layout.
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
// Uses accessor properties (get/set) matching the pattern from
// osm:termui/coordinate's createRectObject for consistency.
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
