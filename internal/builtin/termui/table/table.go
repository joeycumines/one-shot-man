// Package table provides JavaScript bindings for the termui Table component.
//
// The module is exposed as "osm:termui/table" and provides a composite UI
// component for terminal rendering with chainable configuration.
//
// # JavaScript API
//
//	const table = require('osm:termui/table');
//
//	// Table — renders a grid of headers and rows with optional border
//	const tbl = table({ headers: ['A','B'], rows: [['1','2']], headerStyle: s, cellStyle: s, border: borderObj });
//	tbl.headers(['X','Y']).rows([['3','4']]).headerStyle(s).cellStyle(s).border(b);
//	tbl.render(rect);  // → string
package table

import (
	"fmt"

	"charm.land/lipgloss/v2"
	"github.com/dop251/goja"

	lipglossjs "github.com/joeycumines/one-shot-man/internal/builtin/lipgloss"
	"github.com/joeycumines/one-shot-man/internal/termui/component"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/table"
)

// Require returns a CommonJS native module under "osm:termui/table".
func Require() func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		// table(opts?) — creates a Table JS object
		_ = exports.Set("table", func(call goja.FunctionCall) goja.Value {
			var opts []table.TableOption
			if len(call.Arguments) >= 1 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				opts = extractTableOptions(runtime, call.Argument(0))
			}

			t := table.NewTable(opts...)
			return createTableObject(runtime, t)
		})
	}
}

// createTableObject wraps a *table.Table as a Goja Object with chainable
// configuration methods and a render method.
func createTableObject(runtime *goja.Runtime, t *table.Table) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("_type", "termui/table")
	_ = obj.Set("_goComp", t)

	// render(bounds) — renders the table within the given bounds, returns string
	_ = obj.Set("render", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(runtime.NewTypeError("render requires a bounds object {x, y, width, height}"))
		}
		bounds := extractRect(runtime, call.Argument(0))
		return runtime.ToValue(t.Render(bounds))
	})

	// headers(arr) — sets the header row, chainable
	_ = obj.Set("headers", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			return obj
		}
		headers := extractStringSlice(runtime, call.Argument(0))
		table.WithTableHeaders(headers)(t)
		return obj
	})

	// rows(arr) — sets the data rows, chainable
	_ = obj.Set("rows", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			return obj
		}
		rows := extractTableRows(runtime, call.Argument(0))
		table.WithTableRows(rows)(t)
		return obj
	})

	// headerStyle(s) — sets the style for header cells, chainable
	_ = obj.Set("headerStyle", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(runtime.NewTypeError("headerStyle requires a lipgloss style object"))
		}
		s, err := lipglossjs.UnwrapStyle(runtime, call.Argument(0))
		if err != nil {
			panic(runtime.NewTypeError("headerStyle: " + err.Error()))
		}
		table.WithTableHeaderStyle(s)(t)
		return obj
	})

	// cellStyle(s) — sets the style for data cells, chainable
	_ = obj.Set("cellStyle", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(runtime.NewTypeError("cellStyle requires a lipgloss style object"))
		}
		s, err := lipglossjs.UnwrapStyle(runtime, call.Argument(0))
		if err != nil {
			panic(runtime.NewTypeError("cellStyle: " + err.Error()))
		}
		table.WithTableCellStyle(s)(t)
		return obj
	})

	// border(b) — sets the border style, chainable
	_ = obj.Set("border", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			return obj
		}
		brd := jsToBorder(runtime, call.Argument(0))
		table.WithTableBorder(brd)(t)
		return obj
	})

	return obj
}

// extractTableOptions parses an optional JS object into table.TableOption values.
func extractTableOptions(runtime *goja.Runtime, val goja.Value) []table.TableOption {
	obj := val.ToObject(runtime)
	if obj == nil {
		return nil
	}

	var opts []table.TableOption

	if v := obj.Get("headers"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		headers := extractStringSlice(runtime, v)
		opts = append(opts, table.WithTableHeaders(headers))
	}

	if v := obj.Get("rows"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		rows := extractTableRows(runtime, v)
		opts = append(opts, table.WithTableRows(rows))
	}

	if v := obj.Get("headerStyle"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		s, err := lipglossjs.UnwrapStyle(runtime, v)
		if err != nil {
			panic(runtime.NewTypeError("table headerStyle: " + err.Error()))
		}
		opts = append(opts, table.WithTableHeaderStyle(s))
	}

	if v := obj.Get("cellStyle"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		s, err := lipglossjs.UnwrapStyle(runtime, v)
		if err != nil {
			panic(runtime.NewTypeError("table cellStyle: " + err.Error()))
		}
		opts = append(opts, table.WithTableCellStyle(s))
	}

	if v := obj.Get("border"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		brd := jsToBorder(runtime, v)
		opts = append(opts, table.WithTableBorder(brd))
	}

	return opts
}

// extractStringSlice converts a JS array of strings into a []string.
func extractStringSlice(runtime *goja.Runtime, val goja.Value) []string {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil
	}
	arr := val.ToObject(runtime)
	if arr == nil {
		return nil
	}
	length := int(arr.Get("length").ToInteger())
	result := make([]string, 0, length)
	for i := 0; i < length; i++ {
		elem := arr.Get(indexStr(i))
		if elem == nil || goja.IsUndefined(elem) || goja.IsNull(elem) {
			result = append(result, "")
			continue
		}
		result = append(result, elem.String())
	}
	return result
}

// extractTableRows converts a JS array of arrays of strings into
// []table.TableRow.
func extractTableRows(runtime *goja.Runtime, val goja.Value) []table.TableRow {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil
	}
	arr := val.ToObject(runtime)
	if arr == nil {
		return nil
	}
	length := int(arr.Get("length").ToInteger())
	rows := make([]table.TableRow, 0, length)
	for i := 0; i < length; i++ {
		elem := arr.Get(indexStr(i))
		if elem == nil || goja.IsUndefined(elem) || goja.IsNull(elem) {
			continue
		}
		cells := extractStringSlice(runtime, elem)
		rows = append(rows, table.TableRow(cells))
	}
	return rows
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

// indexStr converts an int to a string for JS array index access.
func indexStr(i int) string {
	return fmt.Sprintf("%d", i)
}
