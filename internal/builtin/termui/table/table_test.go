package table

import (
	"testing"

	"github.com/dop251/goja"
	jslipgloss "github.com/joeycumines/one-shot-man/internal/builtin/lipgloss"
	termuitable "github.com/joeycumines/one-shot-man/internal/termui/table"
	"github.com/stretchr/testify/require"
)

func setupRuntime(t *testing.T) *goja.Runtime {
	t.Helper()
	rt := goja.New()
	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).String()
		switch arg {
		case "osm:termui/table":
			mod := rt.NewObject()
			Require()(rt, mod)
			return mod.Get("exports")
		case "osm:lipgloss":
			mod := rt.NewObject()
			lm := jslipgloss.NewManager()
			jslipgloss.Require(lm)(rt, mod)
			return mod.Get("exports")
		}
		return goja.Undefined()
	})
	return rt
}

func TestRequire_Exports(t *testing.T) {
	rt := goja.New()
	module := rt.NewObject()
	require.NoError(t, module.Set("exports", rt.NewObject()))
	Require()(rt, module)

	exports := module.Get("exports").ToObject(rt)
	require.NotNil(t, exports)

	val := exports.Get("table")
	require.False(t, goja.IsUndefined(val))
	_, ok := goja.AssertFunction(val)
	require.True(t, ok)
}

func TestJS_API_Surface(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const lipgloss = require('osm:lipgloss');
		const tableMod = require('osm:termui/table');

		const hdrStyle = lipgloss.newStyle().bold(true);
		const tbl = tableMod.table({
			headers: ['Name', 'Value'],
			rows: [['foo', 'bar'], ['baz', 'qux']],
			headerStyle: hdrStyle
		});
		const out = tbl.render({x: 0, y: 0, width: 40, height: 10});
		if (!out || out.length === 0) throw new Error('render produced empty output');
		if (out.indexOf('Name') === -1) throw new Error('missing Name header');
		if (out.indexOf('foo') === -1) throw new Error('missing foo cell');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestCreateTableObject_GoInterop(t *testing.T) {
	rt := goja.New()

	tbl := termuitable.NewTable()
	objVal := createTableObject(rt, tbl)
	obj := objVal.ToObject(rt)

	// headers setter — chainable
	headersFn, _ := goja.AssertFunction(obj.Get("headers"))
	headersArg := rt.ToValue([]any{"A", "B"})
	res, err := headersFn(goja.Undefined(), headersArg)
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "headers() should return obj (chainable)")

	// rows setter — chainable
	rowsFn, _ := goja.AssertFunction(obj.Get("rows"))
	rowsArg := rt.ToValue([]any{
		[]any{"1", "2"},
		[]any{"3", "4"},
	})
	res, err = rowsFn(goja.Undefined(), rowsArg)
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "rows() should return obj (chainable)")

	// border setter — chainable
	borderFn, _ := goja.AssertFunction(obj.Get("border"))
	borderObj := rt.NewObject()
	_ = borderObj.Set("top", "-")
	_ = borderObj.Set("bottom", "-")
	_ = borderObj.Set("left", "|")
	_ = borderObj.Set("right", "|")
	_ = borderObj.Set("topLeft", "+")
	_ = borderObj.Set("topRight", "+")
	_ = borderObj.Set("bottomLeft", "+")
	_ = borderObj.Set("bottomRight", "+")
	res, err = borderFn(goja.Undefined(), borderObj)
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "border() should return obj (chainable)")

	// render produces non-empty string
	renderFn, _ := goja.AssertFunction(obj.Get("render"))
	boundsObj := rt.NewObject()
	_ = boundsObj.Set("x", 0)
	_ = boundsObj.Set("y", 0)
	_ = boundsObj.Set("width", 40)
	_ = boundsObj.Set("height", 10)
	v, err := renderFn(goja.Undefined(), boundsObj)
	require.NoError(t, err)
	out := v.String()
	require.NotEmpty(t, out, "render should produce non-empty string")
}

func TestNoArgsReturnUndefined(t *testing.T) {
	rt := goja.New()
	tbl := termuitable.NewTable()
	obj := createTableObject(rt, tbl).ToObject(rt)

	// headers with no args returns obj (guard)
	headersFn, _ := goja.AssertFunction(obj.Get("headers"))
	res, err := headersFn(goja.Undefined())
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "headers() with no args returns obj")

	// rows with no args returns obj (guard)
	rowsFn, _ := goja.AssertFunction(obj.Get("rows"))
	res, err = rowsFn(goja.Undefined())
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "rows() with no args returns obj")

	// border with no args returns obj (guard)
	borderFn, _ := goja.AssertFunction(obj.Get("border"))
	res, err = borderFn(goja.Undefined())
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "border() with no args returns obj")

	// headerStyle with no args returns error (TypeError via goja)
	headerStyleFn, _ := goja.AssertFunction(obj.Get("headerStyle"))
	_, err = headerStyleFn(goja.Undefined())
	require.Error(t, err, "headerStyle() with no args should return error")

	// cellStyle with no args returns error (TypeError via goja)
	cellStyleFn, _ := goja.AssertFunction(obj.Get("cellStyle"))
	_, err = cellStyleFn(goja.Undefined())
	require.Error(t, err, "cellStyle() with no args should return error")

	// render with no args returns error (TypeError via goja)
	renderFn, _ := goja.AssertFunction(obj.Get("render"))
	_, err = renderFn(goja.Undefined())
	require.Error(t, err, "render() with no args should return error")
}

func TestTableWithNoArgs(t *testing.T) {
	rt := setupRuntime(t)
	script := `
		const tbl = require('osm:termui/table').table();
		if (typeof tbl !== 'object') throw new Error('expected object');
		'ok';
	`
	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestTableChaining(t *testing.T) {
	rt := setupRuntime(t)
	script := `
		const lipgloss = require('osm:lipgloss');
		const tableMod = require('osm:termui/table');

		const hdrStyle = lipgloss.newStyle().bold(true);
		const cellStyle = lipgloss.newStyle().italic(true);
		const tbl = tableMod.table()
			.headers(['Col1', 'Col2'])
			.rows([['a', 'b'], ['c', 'd']])
			.headerStyle(hdrStyle)
			.cellStyle(cellStyle)
			.border(lipgloss.roundedBorder());

		const out = tbl.render({x: 0, y: 0, width: 40, height: 10});
		if (!out || out.length === 0) throw new Error('render produced empty output');
		'ok';
	`
	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}
