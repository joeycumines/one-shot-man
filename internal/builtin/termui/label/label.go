// Package label provides JavaScript bindings for the termui Label component,
// implemented in [github.com/joeycumines/one-shot-man/internal/termui/label].
//
// The module is exposed as "osm:termui/label" and provides a Label component
// for terminal rendering with chainable configuration.
//
// # JavaScript API
//
//	const lbl = require('osm:termui/label');
//
//	// Label — renders styled text within bounds
//	const l = lbl('Hello', { style: lipglossStyle, maxWidth: 40 });
//	l.style(otherStyle).maxWidth(60).maxHeight(5);
//	l.render(rect);  // → string
package label

import (
	"github.com/dop251/goja"

	lipglossjs "github.com/joeycumines/one-shot-man/internal/builtin/lipgloss"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/label"
)

// Require returns a CommonJS native module under "osm:termui/label".
func Require() func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		// label(text, opts?) — creates a Label JS object
		_ = exports.Set("label", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
				panic(runtime.NewTypeError("label requires a text string"))
			}
			text := call.Argument(0).String()

			var opts []label.LabelOption
			if len(call.Arguments) >= 2 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
				opts = extractLabelOptions(runtime, call.Argument(1))
			}

			l := label.NewLabel(text, opts...)
			return createLabelObject(runtime, l)
		})
	}
}

// createLabelObject wraps a *label.Label as a Goja Object with chainable
// configuration methods and a render method.
func createLabelObject(runtime *goja.Runtime, l *label.Label) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("_type", "termui/label")
	_ = obj.Set("_goComp", l)

	// render(bounds) — renders the label within the given bounds, returns string
	_ = obj.Set("render", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(runtime.NewTypeError("render requires a bounds object {x, y, width, height}"))
		}
		bounds := extractRect(runtime, call.Argument(0))
		return runtime.ToValue(l.Render(bounds))
	})

	// style(s) — sets the lipgloss style, chainable
	_ = obj.Set("style", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(runtime.NewTypeError("style requires a lipgloss style object"))
		}
		s, err := lipglossjs.UnwrapStyle(runtime, call.Argument(0))
		if err != nil {
			panic(runtime.NewTypeError("style: " + err.Error()))
		}
		label.WithLabelStyle(s)(l)
		return obj
	})

	// maxWidth(w) — sets max width constraint, chainable
	_ = obj.Set("maxWidth", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) {
			return obj
		}
		w := int(call.Argument(0).ToInteger())
		label.WithLabelMaxWidth(w)(l)
		return obj
	})

	// maxHeight(h) — sets max height constraint, chainable
	_ = obj.Set("maxHeight", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) {
			return obj
		}
		h := int(call.Argument(0).ToInteger())
		label.WithLabelMaxHeight(h)(l)
		return obj
	})

	return obj
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

// extractLabelOptions parses an optional JS object into label.LabelOption values.
func extractLabelOptions(runtime *goja.Runtime, val goja.Value) []label.LabelOption {
	obj := val.ToObject(runtime)
	if obj == nil {
		return nil
	}

	var opts []label.LabelOption

	if v := obj.Get("style"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		s, err := lipglossjs.UnwrapStyle(runtime, v)
		if err != nil {
			panic(runtime.NewTypeError("label style: " + err.Error()))
		}
		opts = append(opts, label.WithLabelStyle(s))
	}

	if v := obj.Get("maxWidth"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		opts = append(opts, label.WithLabelMaxWidth(int(v.ToInteger())))
	}

	if v := obj.Get("maxHeight"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		opts = append(opts, label.WithLabelMaxHeight(int(v.ToInteger())))
	}

	return opts
}
