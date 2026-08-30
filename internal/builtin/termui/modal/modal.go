// Package modal provides JavaScript bindings for
// [github.com/joeycumines/one-shot-man/internal/termui/modal].
//
// The module is exposed as "osm:termui/modal" and provides a Modal
// composite UI component for terminal rendering with chainable configuration.
//
// # JavaScript API
//
//	const modal = require('osm:termui/modal');
//
//	// Modal — renders a centered dialog box
//	const md = modal({ content: comp, width: 40, height: 10, style: lipglossStyle, border: borderObj });
//	md.content(comp).width(40).height(10).style(lipglossStyle).border(borderObj);
//	md.render(rect);  // → string
package modal

import (
	"charm.land/lipgloss/v2"
	"github.com/joeycumines/goja"

	lipglossjs "github.com/joeycumines/one-shot-man/internal/builtin/lipgloss"
	"github.com/joeycumines/one-shot-man/internal/termui/component"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/modal"
)

// Require returns a CommonJS native module under "osm:termui/modal".
func Require() func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		// modal(opts?) — creates a Modal JS object
		_ = exports.Set("modal", func(call goja.FunctionCall) goja.Value {
			var opts []modal.ModalOption
			if len(call.Arguments) >= 1 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				opts = extractModalOptions(runtime, call.Argument(0))
			}

			m := modal.NewModal(opts...)
			return createModalObject(runtime, m)
		})
	}
}

// createModalObject wraps a *modal.Modal as a Goja Object with chainable
// configuration methods and a render method.
func createModalObject(runtime *goja.Runtime, m *modal.Modal) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("_type", "termui/modal")
	_ = obj.Set("_goComp", m)

	// render(bounds) — renders the modal within the given bounds, returns string
	_ = obj.Set("render", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(runtime.NewTypeError("render requires a bounds object {x, y, width, height}"))
		}
		bounds := extractRect(runtime, call.Argument(0))
		return runtime.ToValue(m.Render(bounds))
	})

	// content(comp) — sets the inner content component, chainable
	_ = obj.Set("content", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			return obj
		}
		comp := extractComponent(runtime, call.Argument(0))
		modal.WithModalContent(comp)(m)
		return obj
	})

	// width(w) — sets the modal width, chainable
	_ = obj.Set("width", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) {
			return obj
		}
		w := int(call.Argument(0).ToInteger())
		modal.WithModalWidth(w)(m)
		return obj
	})

	// height(h) — sets the modal height, chainable
	_ = obj.Set("height", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) {
			return obj
		}
		h := int(call.Argument(0).ToInteger())
		modal.WithModalHeight(h)(m)
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
		modal.WithModalStyle(st)(m)
		return obj
	})

	// border(b) — sets the border style, chainable
	_ = obj.Set("border", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			return obj
		}
		brd := jsToBorder(runtime, call.Argument(0))
		modal.WithModalBorder(brd)(m)
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

// extractModalOptions parses an optional JS object into
// modal.ModalOption values.
func extractModalOptions(runtime *goja.Runtime, val goja.Value) []modal.ModalOption {
	obj := val.ToObject(runtime)
	if obj == nil {
		return nil
	}

	var opts []modal.ModalOption

	if v := obj.Get("content"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		comp := extractComponent(runtime, v)
		if comp != nil {
			opts = append(opts, modal.WithModalContent(comp))
		}
	}

	if v := obj.Get("width"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		opts = append(opts, modal.WithModalWidth(int(v.ToInteger())))
	}

	if v := obj.Get("height"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		opts = append(opts, modal.WithModalHeight(int(v.ToInteger())))
	}

	if v := obj.Get("style"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		s, err := lipglossjs.UnwrapStyle(runtime, v)
		if err != nil {
			panic(runtime.NewTypeError("modal style: " + err.Error()))
		}
		opts = append(opts, modal.WithModalStyle(s))
	}

	if v := obj.Get("border"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		brd := jsToBorder(runtime, v)
		opts = append(opts, modal.WithModalBorder(brd))
	}

	return opts
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
