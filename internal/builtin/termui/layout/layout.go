// Package layout provides JavaScript bindings for
// [github.com/joeycumines/one-shot-man/internal/termui/layout].
//
// The module is exposed as "osm:termui/layout" and provides pure layout
// functions that divide coordinate.Rect values into sub-rects.
//
// # JavaScript API
//
//	const layout = require('osm:termui/layout');
//
//	// Direction constants
//	layout.Direction.HORIZONTAL // "horizontal"
//	layout.Direction.VERTICAL   // "vertical"
//
//	// Split a rect into sub-rects by ratio
//	const [left, right] = layout.split(rect, 'horizontal', [0.3, 0.7]);
//
//	// Grid layout
//	const cells = layout.grid(rect, 3, 2); // 3 columns, 2 rows
//
//	// Stack items with explicit sizes
//	const items = layout.stack(rect, 'vertical', [
//	  {width: 80, height: 5},
//	  {width: 80, height: 10},
//	]);
package layout

import (
	"fmt"

	"github.com/dop251/goja"
	coord "github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	termlayout "github.com/joeycumines/one-shot-man/internal/termui/layout"
)

// Require returns a CommonJS native module under "osm:termui/layout".
func Require() func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		// Direction constants
		direction := runtime.NewObject()
		_ = direction.Set("HORIZONTAL", "horizontal")
		_ = direction.Set("VERTICAL", "vertical")
		_ = exports.Set("Direction", direction)

		// split(rect, direction, ratios) — divides rect along direction by ratios
		_ = exports.Set("split", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 3 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
				panic(runtime.NewTypeError("split requires (rect, direction, ratios)"))
			}

			r := extractRect(runtime, call.Argument(0))
			d := parseDirection(runtime, call.Argument(1))
			ratios := extractFloat64Slice(runtime, call.Argument(2))

			result := termlayout.Split(r, d, ratios)
			return rectSliceToArray(runtime, result)
		})

		// grid(rect, columns, rows) — divides rect into a uniform grid
		_ = exports.Set("grid", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 3 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
				panic(runtime.NewTypeError("grid requires (rect, columns, rows)"))
			}

			r := extractRect(runtime, call.Argument(0))
			columns := int(call.Argument(1).ToInteger())
			rows := int(call.Argument(2).ToInteger())

			result := termlayout.Grid(r, columns, rows)
			return rectSliceToArray(runtime, result)
		})

		// stack(rect, direction, sizes) — arranges items with explicit sizes
		_ = exports.Set("stack", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 3 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
				panic(runtime.NewTypeError("stack requires (rect, direction, sizes)"))
			}

			r := extractRect(runtime, call.Argument(0))
			d := parseDirection(runtime, call.Argument(1))
			sizes := extractSizeSlice(runtime, call.Argument(2))

			result := termlayout.Stack(r, d, sizes)
			return rectSliceToArray(runtime, result)
		})
	}
}

// parseDirection converts a JS direction value to a layout.Direction.
// Accepts the string "horizontal" or "vertical", or the Direction constants.
func parseDirection(runtime *goja.Runtime, val goja.Value) termlayout.Direction {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		panic(runtime.NewTypeError("direction must be 'horizontal' or 'vertical'"))
	}
	s := val.String()
	switch s {
	case "horizontal":
		return termlayout.Horizontal
	case "vertical":
		return termlayout.Vertical
	default:
		panic(runtime.NewTypeError("direction must be 'horizontal' or 'vertical', got: " + s))
	}
}

// extractRect extracts a coord.Rect from a Goja value.
// Supports Rect JS objects (from osm:termui/coordinate) and plain objects
// with x, y, width, height properties (duck typing).
func extractRect(runtime *goja.Runtime, val goja.Value) coord.Rect {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return coord.Rect{}
	}
	obj := val.ToObject(runtime)
	if obj == nil {
		return coord.Rect{}
	}
	return coord.Rect{
		Position: coord.Position{
			X: int(obj.Get("x").ToInteger()),
			Y: int(obj.Get("y").ToInteger()),
		},
		Size: coord.Size{
			Width:  int(obj.Get("width").ToInteger()),
			Height: int(obj.Get("height").ToInteger()),
		},
	}
}

// extractSize extracts a coord.Size from a Goja value.
// Supports Size JS objects and plain objects with width, height properties.
func extractSize(runtime *goja.Runtime, val goja.Value) coord.Size {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return coord.Size{}
	}
	obj := val.ToObject(runtime)
	if obj == nil {
		return coord.Size{}
	}
	return coord.Size{
		Width:  int(obj.Get("width").ToInteger()),
		Height: int(obj.Get("height").ToInteger()),
	}
}

// extractFloat64Slice extracts a []float64 from a Goja array value.
func extractFloat64Slice(runtime *goja.Runtime, val goja.Value) []float64 {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil
	}
	obj := val.ToObject(runtime)
	if obj == nil {
		return nil
	}
	length := obj.Get("length")
	if length == nil || goja.IsUndefined(length) {
		return nil
	}
	n := int(length.ToInteger())
	result := make([]float64, n)
	for i := range n {
		result[i] = obj.Get(indexStr(i)).ToFloat()
	}
	return result
}

// extractSizeSlice extracts a []coord.Size from a Goja array value.
// Each element is duck-typed as an object with width and height properties.
func extractSizeSlice(runtime *goja.Runtime, val goja.Value) []coord.Size {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil
	}
	obj := val.ToObject(runtime)
	if obj == nil {
		return nil
	}
	length := obj.Get("length")
	if length == nil || goja.IsUndefined(length) {
		return nil
	}
	n := int(length.ToInteger())
	result := make([]coord.Size, n)
	for i := range n {
		elem := obj.Get(indexStr(i))
		result[i] = extractSize(runtime, elem)
	}
	return result
}

// rectSliceToArray converts a []coord.Rect to a Goja Array of Rect JS objects.
// Uses the same wrapping pattern as osm:termui/coordinate's createRectObject.
func rectSliceToArray(runtime *goja.Runtime, rects []coord.Rect) goja.Value {
	values := make([]any, len(rects))
	for i := range rects {
		values[i] = createRectObject(runtime, &rects[i])
	}
	return runtime.NewArray(values...)
}

// createRectObject wraps a *coord.Rect as a Goja Object with
// accessor properties (x, y, width, height) and methods.
// Mirrors the pattern from osm:termui/coordinate for consistency.
func createRectObject(runtime *goja.Runtime, r *coord.Rect) goja.Value {
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

// indexStr converts an int to a string for JS array index access.
func indexStr(i int) string {
	return fmt.Sprintf("%d", i)
}
