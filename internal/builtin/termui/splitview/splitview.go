// Package splitview provides JavaScript bindings for
// [github.com/joeycumines/one-shot-man/internal/termui/splitview].
//
// The module is exposed as "osm:termui/splitview" and provides a SplitView
// composite UI component for terminal rendering with chainable configuration.
//
// # JavaScript API
//
//	const splitview = require('osm:termui/splitview');
//
//	// SplitView — renders two components side by side or stacked
//	const sv = splitview({ primary: comp1, secondary: comp2, ratio: 0.6, direction: 'horizontal' });
//	sv.primary(comp1).secondary(comp2).ratio(0.6).direction('vertical').style(lipglossStyle);
//	sv.render(rect);  // → string
//
//	// Direction constants
//	splitview.Direction.HORIZONTAL  // 'horizontal'
//	splitview.Direction.VERTICAL    // 'vertical'
package splitview

import (
	"github.com/joeycumines/goja"

	lipglossjs "github.com/joeycumines/one-shot-man/internal/builtin/lipgloss"
	"github.com/joeycumines/one-shot-man/internal/termui/component"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/layout"
	"github.com/joeycumines/one-shot-man/internal/termui/splitview"
)

// Require returns a CommonJS native module under "osm:termui/splitview".
func Require() func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		// splitview(opts?) — creates a SplitView JS object
		_ = exports.Set("splitView", func(call goja.FunctionCall) goja.Value {
			var opts []splitview.SplitViewOption
			if len(call.Arguments) >= 1 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				opts = extractSplitViewOptions(runtime, call.Argument(0))
			}

			s := splitview.NewSplitView(opts...)
			return createSplitViewObject(runtime, s)
		})

		// Direction constants
		dirObj := runtime.NewObject()
		_ = dirObj.Set("HORIZONTAL", "horizontal")
		_ = dirObj.Set("VERTICAL", "vertical")
		_ = exports.Set("Direction", dirObj)
	}
}

// createSplitViewObject wraps a *splitview.SplitView as a Goja Object with
// chainable configuration methods and a render method.
func createSplitViewObject(runtime *goja.Runtime, s *splitview.SplitView) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("_type", "termui/splitview")
	_ = obj.Set("_goComp", s)

	// render(bounds) — renders the split view within the given bounds, returns string
	_ = obj.Set("render", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(runtime.NewTypeError("render requires a bounds object {x, y, width, height}"))
		}
		bounds := extractRect(runtime, call.Argument(0))
		return runtime.ToValue(s.Render(bounds))
	})

	// primary(comp) — sets the primary (left/top) component, chainable
	_ = obj.Set("primary", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			return obj
		}
		comp := extractComponent(runtime, call.Argument(0))
		splitview.WithSplitViewPrimary(comp)(s)
		return obj
	})

	// secondary(comp) — sets the secondary (right/bottom) component, chainable
	_ = obj.Set("secondary", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			return obj
		}
		comp := extractComponent(runtime, call.Argument(0))
		splitview.WithSplitViewSecondary(comp)(s)
		return obj
	})

	// ratio(r) — sets the space allocation ratio (0.0–1.0), chainable
	_ = obj.Set("ratio", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) {
			return obj
		}
		r := call.Argument(0).ToFloat()
		splitview.WithSplitViewRatio(r)(s)
		return obj
	})

	// direction(d) — sets the split axis ('horizontal' or 'vertical'), chainable
	_ = obj.Set("direction", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			return obj
		}
		d := parseDirection(runtime, call.Argument(0))
		splitview.WithSplitViewDirection(d)(s)
		return obj
	})

	// style(s) — sets the lipgloss style, chainable
	_ = obj.Set("style", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(runtime.NewTypeError("style requires a lipgloss style object"))
		}
		st, err := lipglossjs.UnwrapStyle(runtime, call.Argument(0))
		if err != nil {
			panic(runtime.NewTypeError("style: " + err.Error()))
		}
		splitview.WithSplitViewStyle(st)(s)
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
// (duck typing across SplitView, Modal, Toast, and atom/molecule types).
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

// extractSplitViewOptions parses an optional JS object into
// splitview.SplitViewOption values.
func extractSplitViewOptions(runtime *goja.Runtime, val goja.Value) []splitview.SplitViewOption {
	obj := val.ToObject(runtime)
	if obj == nil {
		return nil
	}

	var opts []splitview.SplitViewOption

	if v := obj.Get("primary"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		comp := extractComponent(runtime, v)
		if comp != nil {
			opts = append(opts, splitview.WithSplitViewPrimary(comp))
		}
	}

	if v := obj.Get("secondary"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		comp := extractComponent(runtime, v)
		if comp != nil {
			opts = append(opts, splitview.WithSplitViewSecondary(comp))
		}
	}

	if v := obj.Get("ratio"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		opts = append(opts, splitview.WithSplitViewRatio(v.ToFloat()))
	}

	if v := obj.Get("direction"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		d := parseDirection(runtime, v)
		opts = append(opts, splitview.WithSplitViewDirection(d))
	}

	if v := obj.Get("style"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		s, err := lipglossjs.UnwrapStyle(runtime, v)
		if err != nil {
			panic(runtime.NewTypeError("splitView style: " + err.Error()))
		}
		opts = append(opts, splitview.WithSplitViewStyle(s))
	}

	return opts
}
