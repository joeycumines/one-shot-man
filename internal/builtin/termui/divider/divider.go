// Package divider provides JavaScript bindings for the termui Divider component,
// implemented in [github.com/joeycumines/one-shot-man/internal/termui/divider].
//
// The module is exposed as "osm:termui/divider" and provides a Divider component
// for terminal rendering with chainable configuration.
//
// # JavaScript API
//
//	const div = require('osm:termui/divider');
//
//	// Divider — renders a horizontal or vertical line
//	const d = div('horizontal', { style: lipglossStyle, char: '=' });
//	d.style(otherStyle).char('-');
//	d.render(rect);  // → string
//
//	// Direction constants
//	div.Direction.HORIZONTAL  // 'horizontal'
//	div.Direction.VERTICAL    // 'vertical'
package divider

import (
	"github.com/dop251/goja"

	lipglossjs "github.com/joeycumines/one-shot-man/internal/builtin/lipgloss"
	"github.com/joeycumines/one-shot-man/internal/termui/component"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/divider"
	"github.com/joeycumines/one-shot-man/internal/termui/layout"
)

// Require returns a CommonJS native module under "osm:termui/divider".
func Require() func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		// divider(direction, opts?) — creates a Divider JS object
		_ = exports.Set("divider", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
				panic(runtime.NewTypeError("divider requires a direction string ('horizontal' or 'vertical')"))
			}
			dir := parseDirection(runtime, call.Argument(0))

			var opts []divider.DividerOption
			if len(call.Arguments) >= 2 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
				opts = extractDividerOptions(runtime, call.Argument(1))
			}

			d := divider.NewDivider(dir, opts...)
			return createDividerObject(runtime, d)
		})

		// Direction constants
		dirObj := runtime.NewObject()
		_ = dirObj.Set("HORIZONTAL", "horizontal")
		_ = dirObj.Set("VERTICAL", "vertical")
		_ = exports.Set("Direction", dirObj)
	}
}

// createDividerObject wraps a *divider.Divider as a Goja Object with chainable
// configuration methods and a render method.
func createDividerObject(runtime *goja.Runtime, d *divider.Divider) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("_type", "termui/divider")
	_ = obj.Set("_goComp", d)

	// render(bounds) — renders the divider within the given bounds, returns string
	_ = obj.Set("render", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(runtime.NewTypeError("render requires a bounds object {x, y, width, height}"))
		}
		bounds := extractRect(runtime, call.Argument(0))
		return runtime.ToValue(d.Render(bounds))
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
		divider.WithDividerStyle(s)(d)
		return obj
	})

	// char(c) — sets the divider character, chainable
	_ = obj.Set("char", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			return obj
		}
		c := call.Argument(0).String()
		if len(c) > 0 {
			divider.WithDividerChar(rune(c[0]))(d)
		}
		return obj
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

// extractComponent extracts a component.Component from a Goja value.
// Supports any JS object with a _goComp field that implements component.Component
// (duck typing across Label, Divider, Box, and future component types).
func extractComponent(runtime *goja.Runtime, val goja.Value) component.Component {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil
	}
	obj := val.ToObject(runtime)
	if obj == nil {
		return nil
	}
	goComp := obj.Get("_goComp")
	if goComp == nil || goja.IsUndefined(goComp) || goja.IsNull(goComp) {
		return nil
	}
	exported := goComp.Export()
	comp, ok := exported.(component.Component)
	if !ok {
		return nil
	}
	return comp
}

// extractDividerOptions parses an optional JS object into divider.DividerOption values.
func extractDividerOptions(runtime *goja.Runtime, val goja.Value) []divider.DividerOption {
	obj := val.ToObject(runtime)
	if obj == nil {
		return nil
	}

	var opts []divider.DividerOption

	if v := obj.Get("style"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		s, err := lipglossjs.UnwrapStyle(runtime, v)
		if err != nil {
			panic(runtime.NewTypeError("divider style: " + err.Error()))
		}
		opts = append(opts, divider.WithDividerStyle(s))
	}

	if v := obj.Get("char"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		c := v.String()
		if len(c) > 0 {
			opts = append(opts, divider.WithDividerChar(rune(c[0])))
		}
	}

	return opts
}
