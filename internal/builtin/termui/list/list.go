// Package list provides JavaScript bindings for the termui List component.
//
// The module is exposed as "osm:termui/list" and provides a composite UI
// component for terminal rendering with chainable configuration.
//
// # JavaScript API
//
//	const list = require('osm:termui/list');
//
//	// List — renders a vertical list of items with optional selection
//	const lst = list({ items: [{text: 'A'}, {text: 'B', style: s}], selectedStyle: hlStyle, selectedIndex: 0 });
//	lst.items([{text: 'X'}]).selectedStyle(otherStyle).selectedIndex(2);
//	lst.render(rect);  // → string
package list

import (
	"fmt"

	"github.com/dop251/goja"

	lipglossjs "github.com/joeycumines/one-shot-man/internal/builtin/lipgloss"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/list"
)

// Require returns a CommonJS native module under "osm:termui/list".
func Require() func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		// list(opts?) — creates a List JS object
		_ = exports.Set("list", func(call goja.FunctionCall) goja.Value {
			var opts []list.ListOption
			if len(call.Arguments) >= 1 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				opts = extractListOptions(runtime, call.Argument(0))
			}

			l := list.NewList(opts...)
			return createListObject(runtime, l)
		})
	}
}

// createListObject wraps a *list.List as a Goja Object with chainable
// configuration methods and a render method.
func createListObject(runtime *goja.Runtime, l *list.List) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("_type", "termui/list")
	_ = obj.Set("_goComp", l)

	// render(bounds) — renders the list within the given bounds, returns string
	_ = obj.Set("render", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(runtime.NewTypeError("render requires a bounds object {x, y, width, height}"))
		}
		bounds := extractRect(runtime, call.Argument(0))
		return runtime.ToValue(l.Render(bounds))
	})

	// items(arr) — sets the list items, chainable
	_ = obj.Set("items", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			return obj
		}
		items := extractListItems(runtime, call.Argument(0))
		list.WithListItems(items)(l)
		return obj
	})

	// selectedStyle(s) — sets the style for the selected item, chainable
	_ = obj.Set("selectedStyle", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(runtime.NewTypeError("selectedStyle requires a lipgloss style object"))
		}
		s, err := lipglossjs.UnwrapStyle(runtime, call.Argument(0))
		if err != nil {
			panic(runtime.NewTypeError("selectedStyle: " + err.Error()))
		}
		list.WithListSelectedStyle(s)(l)
		return obj
	})

	// selectedIndex(i) — sets the selected item index, chainable
	_ = obj.Set("selectedIndex", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) {
			return obj
		}
		i := int(call.Argument(0).ToInteger())
		list.WithListSelectedIndex(i)(l)
		return obj
	})

	return obj
}

// extractListOptions parses an optional JS object into list.ListOption values.
func extractListOptions(runtime *goja.Runtime, val goja.Value) []list.ListOption {
	obj := val.ToObject(runtime)
	if obj == nil {
		return nil
	}

	var opts []list.ListOption

	if v := obj.Get("items"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		items := extractListItems(runtime, v)
		opts = append(opts, list.WithListItems(items))
	}

	if v := obj.Get("selectedStyle"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		s, err := lipglossjs.UnwrapStyle(runtime, v)
		if err != nil {
			panic(runtime.NewTypeError("list selectedStyle: " + err.Error()))
		}
		opts = append(opts, list.WithListSelectedStyle(s))
	}

	if v := obj.Get("selectedIndex"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		opts = append(opts, list.WithListSelectedIndex(int(v.ToInteger())))
	}

	return opts
}

// extractListItems converts a JS array of {text, style?} objects into
// []list.ListItem.
func extractListItems(runtime *goja.Runtime, val goja.Value) []list.ListItem {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil
	}
	arr := val.ToObject(runtime)
	if arr == nil {
		return nil
	}
	length := int(arr.Get("length").ToInteger())
	items := make([]list.ListItem, 0, length)
	for i := 0; i < length; i++ {
		elem := arr.Get(indexStr(i))
		if elem == nil || goja.IsUndefined(elem) || goja.IsNull(elem) {
			continue
		}
		itemObj := elem.ToObject(runtime)
		if itemObj == nil {
			continue
		}
		text := itemObj.Get("text")
		if text == nil || goja.IsUndefined(text) || goja.IsNull(text) {
			continue
		}
		li := list.ListItem{Text: text.String()}
		if v := itemObj.Get("style"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			s, err := lipglossjs.UnwrapStyle(runtime, v)
			if err != nil {
				panic(runtime.NewTypeError("list item style: " + err.Error()))
			}
			li.Style = s
		}
		items = append(items, li)
	}
	return items
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


// indexStr converts an int to a string for JS array index access.
func indexStr(i int) string {
	return fmt.Sprintf("%d", i)
}
