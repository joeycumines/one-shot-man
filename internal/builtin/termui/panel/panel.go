// Package panel provides JavaScript bindings for the termui Panel component.
//
// The module is exposed as "osm:termui/panel" and provides a composite UI
// component for terminal rendering with chainable configuration.
//
// # JavaScript API
//
//	const panel = require('osm:termui/panel');
//
//	// Panel — renders a bordered container with optional title and content
//	const pnl = panel({ title: 'Title', style: lipglossStyle, border: borderObj });
//	pnl.title('New Title').content(comp).style(otherStyle).border(otherBorder);
//	pnl.render(rect);  // → string
package panel

import (
	"charm.land/lipgloss/v2"
	"github.com/dop251/goja"

	lipglossjs "github.com/joeycumines/one-shot-man/internal/builtin/lipgloss"
	"github.com/joeycumines/one-shot-man/internal/termui/component"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/panel"
)

// Require returns a CommonJS native module under "osm:termui/panel".
func Require() func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		// panel(opts?) — creates a Panel JS object
		_ = exports.Set("panel", func(call goja.FunctionCall) goja.Value {
			var opts []panel.PanelOption
			if len(call.Arguments) >= 1 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				opts = extractPanelOptions(runtime, call.Argument(0))
			}

			p := panel.NewPanel(opts...)
			return createPanelObject(runtime, p)
		})
	}
}

// createPanelObject wraps a *panel.Panel as a Goja Object with chainable
// configuration methods and a render method.
func createPanelObject(runtime *goja.Runtime, p *panel.Panel) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("_type", "termui/panel")
	_ = obj.Set("_goComp", p)

	// render(bounds) — renders the panel within the given bounds, returns string
	_ = obj.Set("render", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(runtime.NewTypeError("render requires a bounds object {x, y, width, height}"))
		}
		bounds := extractRect(runtime, call.Argument(0))
		return runtime.ToValue(p.Render(bounds))
	})

	// title(t) — sets the panel title, chainable
	_ = obj.Set("title", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) {
			return obj
		}
		t := call.Argument(0).String()
		panel.WithPanelTitle(t)(p)
		return obj
	})

	// content(comp) — sets the inner content component, chainable
	_ = obj.Set("content", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			return obj
		}
		comp := extractComponent(runtime, call.Argument(0))
		panel.WithPanelContent(comp)(p)
		return obj
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
		panel.WithPanelStyle(s)(p)
		return obj
	})

	// border(b) — sets the border style, chainable
	_ = obj.Set("border", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			return obj
		}
		brd := jsToBorder(runtime, call.Argument(0))
		panel.WithPanelBorder(brd)(p)
		return obj
	})

	return obj
}

// extractPanelOptions parses an optional JS object into panel.PanelOption values.
func extractPanelOptions(runtime *goja.Runtime, val goja.Value) []panel.PanelOption {
	obj := val.ToObject(runtime)
	if obj == nil {
		return nil
	}

	var opts []panel.PanelOption

	if v := obj.Get("title"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		opts = append(opts, panel.WithPanelTitle(v.String()))
	}

	if v := obj.Get("content"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		comp := extractComponent(runtime, v)
		if comp != nil {
			opts = append(opts, panel.WithPanelContent(comp))
		}
	}

	if v := obj.Get("style"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		s, err := lipglossjs.UnwrapStyle(runtime, v)
		if err != nil {
			panic(runtime.NewTypeError("panel style: " + err.Error()))
		}
		opts = append(opts, panel.WithPanelStyle(s))
	}

	if v := obj.Get("border"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		brd := jsToBorder(runtime, v)
		opts = append(opts, panel.WithPanelBorder(brd))
	}

	return opts
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
// Supports any JS object with a _goComp field that implements
// component.Component (duck typing across Panel, List, Table, and atom types).
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

// jsToBorder converts a JS value to a lipgloss.Border.
// Accepts a plain JS object with border character properties.
func jsToBorder(runtime *goja.Runtime, val goja.Value) lipgloss.Border {
	if goja.IsUndefined(val) || goja.IsNull(val) {
		return lipgloss.Border{}
	}
	obj := val.ToObject(runtime)
	getString := func(key string) string {
		v := obj.Get(key)
		if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
			return ""
		}
		return v.String()
	}
	return lipgloss.Border{
		Top:          getString("top"),
		Bottom:       getString("bottom"),
		Left:         getString("left"),
		Right:        getString("right"),
		TopLeft:      getString("topLeft"),
		TopRight:     getString("topRight"),
		BottomLeft:   getString("bottomLeft"),
		BottomRight:  getString("bottomRight"),
		MiddleLeft:   getString("middleLeft"),
		MiddleRight:  getString("middleRight"),
		Middle:       getString("middle"),
		MiddleTop:    getString("middleTop"),
		MiddleBottom: getString("middleBottom"),
	}
}
