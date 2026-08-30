// Package toast provides JavaScript bindings for
// [github.com/joeycumines/one-shot-man/internal/termui/toast].
//
// The module is exposed as "osm:termui/toast" and provides a Toast
// composite UI component for terminal rendering with chainable configuration.
//
// # JavaScript API
//
//	const toast = require('osm:termui/toast');
//
//	// Toast — renders a short notification at the bottom of bounds
//	const ts = toast({ message: 'Saved!', style: lipglossStyle, width: 30 });
//	ts.message('Saved!').style(lipglossStyle).width(30);
//	ts.render(rect);  // → string
package toast

import (
	"github.com/joeycumines/goja"

	lipglossjs "github.com/joeycumines/one-shot-man/internal/builtin/lipgloss"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/toast"
)

// Require returns a CommonJS native module under "osm:termui/toast".
func Require() func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		// toast(opts?) — creates a Toast JS object
		_ = exports.Set("toast", func(call goja.FunctionCall) goja.Value {
			var opts []toast.ToastOption
			if len(call.Arguments) >= 1 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				opts = extractToastOptions(runtime, call.Argument(0))
			}

			t := toast.NewToast(opts...)
			return createToastObject(runtime, t)
		})
	}
}

// createToastObject wraps a *toast.Toast as a Goja Object with chainable
// configuration methods and a render method.
func createToastObject(runtime *goja.Runtime, t *toast.Toast) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("_type", "termui/toast")
	_ = obj.Set("_goComp", t)

	// render(bounds) — renders the toast within the given bounds, returns string
	_ = obj.Set("render", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(runtime.NewTypeError("render requires a bounds object {x, y, width, height}"))
		}
		bounds := extractRect(runtime, call.Argument(0))
		return runtime.ToValue(t.Render(bounds))
	})

	// message(msg) — sets the notification text, chainable
	_ = obj.Set("message", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) {
			return obj
		}
		msg := call.Argument(0).String()
		toast.WithToastMessage(msg)(t)
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
		toast.WithToastStyle(st)(t)
		return obj
	})

	// width(w) — sets the toast width, chainable
	_ = obj.Set("width", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) {
			return obj
		}
		w := int(call.Argument(0).ToInteger())
		toast.WithToastWidth(w)(t)
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

// extractToastOptions parses an optional JS object into
// toast.ToastOption values.
func extractToastOptions(runtime *goja.Runtime, val goja.Value) []toast.ToastOption {
	obj := val.ToObject(runtime)
	if obj == nil {
		return nil
	}

	var opts []toast.ToastOption

	if v := obj.Get("message"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		opts = append(opts, toast.WithToastMessage(v.String()))
	}

	if v := obj.Get("style"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		s, err := lipglossjs.UnwrapStyle(runtime, v)
		if err != nil {
			panic(runtime.NewTypeError("toast style: " + err.Error()))
		}
		opts = append(opts, toast.WithToastStyle(s))
	}

	if v := obj.Get("width"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		opts = append(opts, toast.WithToastWidth(int(v.ToInteger())))
	}

	return opts
}
