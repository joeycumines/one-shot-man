package list

import (
	"testing"

	"github.com/dop251/goja"
	jslipgloss "github.com/joeycumines/one-shot-man/internal/builtin/lipgloss"
	termuilist "github.com/joeycumines/one-shot-man/internal/termui/list"
	"github.com/stretchr/testify/require"
)

func setupRuntime(t *testing.T) *goja.Runtime {
	t.Helper()
	rt := goja.New()
	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).String()
		switch arg {
		case "osm:termui/list":
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

	val := exports.Get("list")
	require.False(t, goja.IsUndefined(val))
	_, ok := goja.AssertFunction(val)
	require.True(t, ok)
}

func TestJS_API_Surface(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const lipgloss = require('osm:lipgloss');
		const listMod = require('osm:termui/list');

		const hlStyle = lipgloss.newStyle().bold(true);
		const lst = listMod.list({ items: [{text: 'Alpha'}, {text: 'Beta'}], selectedStyle: hlStyle, selectedIndex: 0 });
		const out = lst.render({x: 0, y: 0, width: 20, height: 5});
		if (!out || out.length === 0) throw new Error('render produced empty output');
		if (out.indexOf('Alpha') === -1) throw new Error('missing Alpha');
		if (out.indexOf('Beta') === -1) throw new Error('missing Beta');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestCreateListObject_GoInterop(t *testing.T) {
	rt := goja.New()

	l := termuilist.NewList()
	objVal := createListObject(rt, l)
	obj := objVal.ToObject(rt)

	// items setter — chainable
	itemsFn, _ := goja.AssertFunction(obj.Get("items"))
	itemsArg := rt.ToValue([]any{
		map[string]any{"text": "Alpha"},
		map[string]any{"text": "Beta"},
	})
	res, err := itemsFn(goja.Undefined(), itemsArg)
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "items() should return obj (chainable)")

	// selectedIndex setter — chainable
	selFn, _ := goja.AssertFunction(obj.Get("selectedIndex"))
	res, err = selFn(goja.Undefined(), rt.ToValue(1))
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "selectedIndex() should return obj (chainable)")

	// render produces non-empty string
	renderFn, _ := goja.AssertFunction(obj.Get("render"))
	boundsObj := rt.NewObject()
	_ = boundsObj.Set("x", 0)
	_ = boundsObj.Set("y", 0)
	_ = boundsObj.Set("width", 20)
	_ = boundsObj.Set("height", 5)
	v, err := renderFn(goja.Undefined(), boundsObj)
	require.NoError(t, err)
	out := v.String()
	require.NotEmpty(t, out, "render should produce non-empty string")
	require.Contains(t, out, "Alpha")
	require.Contains(t, out, "Beta")
}

func TestNoArgsReturnUndefined(t *testing.T) {
	rt := goja.New()
	l := termuilist.NewList()
	obj := createListObject(rt, l).ToObject(rt)

	// items with no args returns obj (guard)
	itemsFn, _ := goja.AssertFunction(obj.Get("items"))
	res, err := itemsFn(goja.Undefined())
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "items() with no args returns obj")

	// selectedIndex with no args returns obj
	selFn, _ := goja.AssertFunction(obj.Get("selectedIndex"))
	res, err = selFn(goja.Undefined())
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "selectedIndex() with no args returns obj")

	// render with no args returns error (TypeError via goja panic)
	renderFn, _ := goja.AssertFunction(obj.Get("render"))
	_, err = renderFn(goja.Undefined())
	require.Error(t, err, "render() with no args should return error")

	// selectedStyle with no args returns error (TypeError via goja panic)
	selStyleFn, _ := goja.AssertFunction(obj.Get("selectedStyle"))
	_, err = selStyleFn(goja.Undefined())
	require.Error(t, err, "selectedStyle() with no args should return error")
}

func TestListWithNoArgs(t *testing.T) {
	rt := setupRuntime(t)
	script := `
		const lst = require('osm:termui/list').list();
		if (typeof lst !== 'object') throw new Error('expected object');
		'ok';
	`
	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestListChaining(t *testing.T) {
	rt := setupRuntime(t)
	script := `
		const lipgloss = require('osm:lipgloss');
		const listMod = require('osm:termui/list');

		const hlStyle = lipgloss.newStyle().bold(true);
		const lst = listMod.list()
			.items([{text: 'One'}, {text: 'Two'}, {text: 'Three'}])
			.selectedStyle(hlStyle)
			.selectedIndex(2);

		const out = lst.render({x: 0, y: 0, width: 30, height: 5});
		if (!out || out.length === 0) throw new Error('render produced empty output');
		if (out.indexOf('One') === -1) throw new Error('missing One');
		if (out.indexOf('Two') === -1) throw new Error('missing Two');
		if (out.indexOf('Three') === -1) throw new Error('missing Three');
		'ok';
	`
	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}
