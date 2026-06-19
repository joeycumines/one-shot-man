// Package coordinate provides JavaScript bindings for
// [github.com/joeycumines/one-shot-man/internal/termui/coordinate].
//
// The module is exposed as "osm:termui/coordinate" and provides value types
// for 2D terminal coordinate geometry: Position, Size, Rect, and Layer.
//
// # JavaScript API
//
//	const coord = require('osm:termui/coordinate');
//
//	// Constructors (factory functions)
//	const p = coord.position({x: 3, y: 5});
//	const s = coord.size({width: 80, height: 24});
//	const r = coord.rect({x: 0, y: 0, width: 80, height: 24});
//	const l = coord.layer({x: 0, y: 0, width: 80, height: 24, z: 1});
//
//	// Position: x, y properties (get/set), add, sub, in, toString
//	p.x;                    // 3
//	p.y;                    // 5
//	p.add(coord.position({x: 1, y: 2}));  // Position(4,7)
//	p.sub(coord.position({x: 1, y: 2}));  // Position(2,3)
//	p.in(r);                // true/false
//	p.toString();           // "(3,5)"
//
//	// Size: width, height properties (get/set), area, contains, toString
//	s.width;                // 80
//	s.height;               // 24
//	s.area();               // 1920
//	s.contains(coord.size({width: 40, height: 12})); // true
//	s.toString();           // "80x24"
//
//	// Rect: x, y, width, height properties (get/set), methods
//	r.contains(coord.position({x: 5, y: 5})); // true
//	r.overlaps(coord.rect({x: 70, y: 0, width: 20, height: 24})); // true
//	r.inset(coord.size({width: 2, height: 1})); // shrinks rect
//	const [left, right] = r.split(0.5, true);  // horizontal split
//	r.intersect(other);     // overlapping region
//	r.union(other);         // bounding rect
//	r.asPaneGeometry();     // {row, col, rows, cols}
//	r.toString();           // "(0,0) 80x24"
//
//	// Layer: x, y, width, height, z properties (get/set), asLayer, toString
//	l.z;                    // 1
//	l.asLayer();            // lipgloss.Layer (opaque)
//	l.toString();           // "(0,0) 80x24 z:1"
//
//	// Package-level functions
//	coord.fromPaneGeometry({row: 0, col: 0, rows: 24, cols: 80}); // → Rect
//	coord.fromLayer(lipglossLayerObj);                             // → Layer
package coordinate

import (
	"charm.land/lipgloss/v2"
	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/termmux"
	coord "github.com/joeycumines/one-shot-man/internal/termui/coordinate"
)

// Require returns a CommonJS native module under "osm:termui/coordinate".
func Require() func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		// position({x, y}) — creates a Position object
		_ = exports.Set("position", func(call goja.FunctionCall) goja.Value {
			p := &coord.Position{}
			if len(call.Arguments) >= 1 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				obj := call.Argument(0).ToObject(runtime)
				p.X = int(obj.Get("x").ToInteger())
				p.Y = int(obj.Get("y").ToInteger())
			}
			return createPositionObject(runtime, p)
		})

		// size({width, height}) — creates a Size object
		_ = exports.Set("size", func(call goja.FunctionCall) goja.Value {
			s := &coord.Size{}
			if len(call.Arguments) >= 1 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				obj := call.Argument(0).ToObject(runtime)
				s.Width = int(obj.Get("width").ToInteger())
				s.Height = int(obj.Get("height").ToInteger())
			}
			return createSizeObject(runtime, s)
		})

		// rect({x, y, width, height}) — creates a Rect object
		_ = exports.Set("rect", func(call goja.FunctionCall) goja.Value {
			r := &coord.Rect{}
			if len(call.Arguments) >= 1 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				obj := call.Argument(0).ToObject(runtime)
				r.Position.X = int(obj.Get("x").ToInteger())
				r.Position.Y = int(obj.Get("y").ToInteger())
				r.Size.Width = int(obj.Get("width").ToInteger())
				r.Size.Height = int(obj.Get("height").ToInteger())
			}
			return createRectObject(runtime, r)
		})

		// layer({x, y, width, height, z}) — creates a Layer object
		_ = exports.Set("layer", func(call goja.FunctionCall) goja.Value {
			l := &coord.Layer{}
			if len(call.Arguments) >= 1 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				obj := call.Argument(0).ToObject(runtime)
				l.Rect.Position.X = int(obj.Get("x").ToInteger())
				l.Rect.Position.Y = int(obj.Get("y").ToInteger())
				l.Rect.Size.Width = int(obj.Get("width").ToInteger())
				l.Rect.Size.Height = int(obj.Get("height").ToInteger())
				l.Z = int(obj.Get("z").ToInteger())
			}
			return createLayerObject(runtime, l)
		})

		// fromPaneGeometry({row, col, rows, cols}) — creates a Rect from pane geometry.
		// Maps: Row→Y, Col→X, Rows→Height, Cols→Width (same as Go PaneGeometryRect).
		_ = exports.Set("fromPaneGeometry", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
				panic(runtime.NewTypeError("fromPaneGeometry requires a pane geometry object {row, col, rows, cols}"))
			}
			obj := call.Argument(0).ToObject(runtime)
			pg := termmux.PaneGeometry{
				Row:  int(obj.Get("row").ToInteger()),
				Col:  int(obj.Get("col").ToInteger()),
				Rows: int(obj.Get("rows").ToInteger()),
				Cols: int(obj.Get("cols").ToInteger()),
			}
			r := coord.PaneGeometryRect(pg)
			return createRectObject(runtime, &r)
		})

		// fromLayer(lipglossLayerObj) — creates a Layer from a lipgloss layer.
		// Accepts either a Go-wrapped *lipgloss.Layer or a plain JS object
		// with x, y, width, height, z properties.
		_ = exports.Set("fromLayer", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
				panic(runtime.NewTypeError("fromLayer requires a layer object"))
			}
			arg := call.Argument(0)

			// Try to extract a Go *lipgloss.Layer via Export().
			if exported := arg.Export(); exported != nil {
				if ll, ok := exported.(*lipgloss.Layer); ok {
					l := coord.LipglossLayer(ll)
					return createLayerObject(runtime, &l)
				}
			}

			// Fall back to extracting properties from a plain JS object.
			obj := arg.ToObject(runtime)
			if obj == nil {
				panic(runtime.NewTypeError("fromLayer requires a layer object"))
			}
			l := &coord.Layer{
				Rect: coord.Rect{
					Position: coord.Position{
						X: int(obj.Get("x").ToInteger()),
						Y: int(obj.Get("y").ToInteger()),
					},
					Size: coord.Size{
						Width:  int(obj.Get("width").ToInteger()),
						Height: int(obj.Get("height").ToInteger()),
					},
				},
				Z: int(obj.Get("z").ToInteger()),
			}
			return createLayerObject(runtime, l)
		})
	}
}

// createPositionObject wraps a *coord.Position as a Goja Object with
// accessor properties (x, y) and methods (add, sub, in, toString).
func createPositionObject(runtime *goja.Runtime, p *coord.Position) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("_type", "termui/coordinate/position")

	// x — accessor property (get/set)
	_ = obj.DefineAccessorProperty("x",
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return runtime.ToValue(p.X)
		}),
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			p.X = int(call.Argument(0).ToInteger())
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE,
	)

	// y — accessor property (get/set)
	_ = obj.DefineAccessorProperty("y",
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return runtime.ToValue(p.Y)
		}),
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			p.Y = int(call.Argument(0).ToInteger())
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE,
	)

	// add(other) — returns a new Position
	_ = obj.Set("add", func(call goja.FunctionCall) goja.Value {
		other := extractPosition(runtime, call.Argument(0))
		result := p.Add(other)
		return createPositionObject(runtime, &result)
	})

	// sub(other) — returns a new Position
	_ = obj.Set("sub", func(call goja.FunctionCall) goja.Value {
		other := extractPosition(runtime, call.Argument(0))
		result := p.Sub(other)
		return createPositionObject(runtime, &result)
	})

	// in(rect) — returns boolean
	_ = obj.Set("in", func(call goja.FunctionCall) goja.Value {
		r := extractRect(runtime, call.Argument(0))
		return runtime.ToValue(p.In(r))
	})

	// toString() — returns string
	_ = obj.Set("toString", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(p.String())
	})

	return obj
}

// createSizeObject wraps a *coord.Size as a Goja Object with
// accessor properties (width, height) and methods (area, contains, toString).
func createSizeObject(runtime *goja.Runtime, s *coord.Size) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("_type", "termui/coordinate/size")

	// width — accessor property (get/set)
	_ = obj.DefineAccessorProperty("width",
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return runtime.ToValue(s.Width)
		}),
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			s.Width = int(call.Argument(0).ToInteger())
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE,
	)

	// height — accessor property (get/set)
	_ = obj.DefineAccessorProperty("height",
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return runtime.ToValue(s.Height)
		}),
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			s.Height = int(call.Argument(0).ToInteger())
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE,
	)

	// area() — returns int
	_ = obj.Set("area", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(s.Area())
	})

	// contains(other) — returns boolean
	_ = obj.Set("contains", func(call goja.FunctionCall) goja.Value {
		other := extractSize(runtime, call.Argument(0))
		return runtime.ToValue(s.Contains(other))
	})

	// toString() — returns string
	_ = obj.Set("toString", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(s.String())
	})

	return obj
}

// createRectObject wraps a *coord.Rect as a Goja Object with
// accessor properties (x, y, width, height) and methods.
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

	// contains(position) — returns boolean
	_ = obj.Set("contains", func(call goja.FunctionCall) goja.Value {
		p := extractPosition(runtime, call.Argument(0))
		return runtime.ToValue(r.Contains(p))
	})

	// overlaps(other) — returns boolean
	_ = obj.Set("overlaps", func(call goja.FunctionCall) goja.Value {
		other := extractRect(runtime, call.Argument(0))
		return runtime.ToValue(r.Overlaps(other))
	})

	// inset(size) — returns a new Rect
	_ = obj.Set("inset", func(call goja.FunctionCall) goja.Value {
		s := extractSize(runtime, call.Argument(0))
		result := r.Inset(s)
		return createRectObject(runtime, &result)
	})

	// split(ratio, horizontal) — returns [first, second] Rects
	_ = obj.Set("split", func(call goja.FunctionCall) goja.Value {
		ratio := 0.5
		horizontal := false
		if len(call.Arguments) >= 1 && !goja.IsUndefined(call.Argument(0)) {
			ratio = call.Argument(0).ToFloat()
		}
		if len(call.Arguments) >= 2 && !goja.IsUndefined(call.Argument(1)) {
			horizontal = call.Argument(1).ToBoolean()
		}
		first, second := r.Split(ratio, horizontal)
		return runtime.NewArray(
			createRectObject(runtime, &first),
			createRectObject(runtime, &second),
		)
	})

	// intersect(other) — returns a new Rect
	_ = obj.Set("intersect", func(call goja.FunctionCall) goja.Value {
		other := extractRect(runtime, call.Argument(0))
		result := r.Intersect(other)
		return createRectObject(runtime, &result)
	})

	// union(other) — returns a new Rect
	_ = obj.Set("union", func(call goja.FunctionCall) goja.Value {
		other := extractRect(runtime, call.Argument(0))
		result := r.Union(other)
		return createRectObject(runtime, &result)
	})

	// asPaneGeometry() — returns {row, col, rows, cols}
	// Maps: Y→Row, X→Col, Height→Rows, Width→Cols (same as Go AsPaneGeometry).
	_ = obj.Set("asPaneGeometry", func(call goja.FunctionCall) goja.Value {
		result := runtime.NewObject()
		_ = result.Set("row", r.Position.Y)
		_ = result.Set("col", r.Position.X)
		_ = result.Set("rows", r.Size.Height)
		_ = result.Set("cols", r.Size.Width)
		return result
	})

	// toString() — returns string
	_ = obj.Set("toString", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(r.String())
	})

	return obj
}

// createLayerObject wraps a *coord.Layer as a Goja Object with
// accessor properties (x, y, width, height, z) and methods.
func createLayerObject(runtime *goja.Runtime, l *coord.Layer) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("_type", "termui/coordinate/layer")

	// x — accessor property (get/set), maps to Rect.Position.X
	_ = obj.DefineAccessorProperty("x",
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return runtime.ToValue(l.Rect.Position.X)
		}),
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			l.Rect.Position.X = int(call.Argument(0).ToInteger())
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE,
	)

	// y — accessor property (get/set), maps to Rect.Position.Y
	_ = obj.DefineAccessorProperty("y",
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return runtime.ToValue(l.Rect.Position.Y)
		}),
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			l.Rect.Position.Y = int(call.Argument(0).ToInteger())
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE,
	)

	// width — accessor property (get/set), maps to Rect.Size.Width
	_ = obj.DefineAccessorProperty("width",
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return runtime.ToValue(l.Rect.Size.Width)
		}),
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			l.Rect.Size.Width = int(call.Argument(0).ToInteger())
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE,
	)

	// height — accessor property (get/set), maps to Rect.Size.Height
	_ = obj.DefineAccessorProperty("height",
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return runtime.ToValue(l.Rect.Size.Height)
		}),
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			l.Rect.Size.Height = int(call.Argument(0).ToInteger())
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE,
	)

	// z — accessor property (get/set)
	_ = obj.DefineAccessorProperty("z",
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			return runtime.ToValue(l.Z)
		}),
		runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			l.Z = int(call.Argument(0).ToInteger())
			return goja.Undefined()
		}),
		goja.FLAG_FALSE, goja.FLAG_TRUE,
	)

	// asLayer() — returns a lipgloss.Layer as an opaque Goja value
	_ = obj.Set("asLayer", func(call goja.FunctionCall) goja.Value {
		ll := l.AsLayer()
		return runtime.ToValue(ll)
	})

	// toString() — returns string
	_ = obj.Set("toString", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(l.String())
	})

	return obj
}

// extractPosition extracts a coord.Position from a Goja value.
// Supports Position JS objects and plain objects with x, y properties (duck typing).
func extractPosition(runtime *goja.Runtime, val goja.Value) coord.Position {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return coord.Position{}
	}
	obj := val.ToObject(runtime)
	if obj == nil {
		return coord.Position{}
	}
	return coord.Position{
		X: int(obj.Get("x").ToInteger()),
		Y: int(obj.Get("y").ToInteger()),
	}
}

// extractSize extracts a coord.Size from a Goja value.
// Supports Size JS objects and plain objects with width, height properties (duck typing).
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

// extractRect extracts a coord.Rect from a Goja value.
// Supports Rect JS objects and plain objects with x, y, width, height properties (duck typing).
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
