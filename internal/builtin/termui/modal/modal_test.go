package modal

import (
	"testing"

	"github.com/dop251/goja"
	jslipgloss "github.com/joeycumines/one-shot-man/internal/builtin/lipgloss"
	listbinding "github.com/joeycumines/one-shot-man/internal/builtin/termui/list"
	termuilist "github.com/joeycumines/one-shot-man/internal/termui/list"
	"github.com/joeycumines/one-shot-man/internal/termui/modal"
	"github.com/stretchr/testify/require"
)

func setupRuntime(t *testing.T) *goja.Runtime {
	t.Helper()
	rt := goja.New()
	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).String()
		switch arg {
		case "osm:termui/modal":
			mod := rt.NewObject()
			Require()(rt, mod)
			return mod.Get("exports")
		case "osm:termui/list":
			mod := rt.NewObject()
			listbinding.Require()(rt, mod)
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

	val := exports.Get("modal")
	require.False(t, goja.IsUndefined(val))
	_, ok := goja.AssertFunction(val)
	require.True(t, ok)
}

func TestJS_API_Surface(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const lipgloss = require('osm:lipgloss');
		const listMod = require('osm:termui/list');
		const modalMod = require('osm:termui/modal');

		const lst = listMod.list({ items: [{text: 'Hello'}] });
		const md = modalMod.modal({ content: lst, width: 30, height: 5, border: lipgloss.roundedBorder() });
		const out = md.render({x: 0, y: 0, width: 60, height: 20});
		if (!out || out.length === 0) throw new Error('render produced empty output');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestCreateModalObject_GoInterop(t *testing.T) {
	rt := goja.New()

	m := modal.NewModal()
	objVal := createModalObject(rt, m)
	obj := objVal.ToObject(rt)

	// width setter — chainable
	widthFn, _ := goja.AssertFunction(obj.Get("width"))
	res, err := widthFn(goja.Undefined(), rt.ToValue(40))
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "width() should return obj (chainable)")

	// height setter — chainable
	heightFn, _ := goja.AssertFunction(obj.Get("height"))
	res, err = heightFn(goja.Undefined(), rt.ToValue(10))
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "height() should return obj (chainable)")

	// content setter — chainable (using a list as content via _goComp duck typing)
	lst := termuilist.NewList(termuilist.WithListItems([]termuilist.ListItem{{Text: "Item1"}}))
	lstObj := rt.NewObject()
	_ = lstObj.Set("_type", "termui/list")
	_ = lstObj.Set("_goComp", lst)
	contentFn, _ := goja.AssertFunction(obj.Get("content"))
	res, err = contentFn(goja.Undefined(), lstObj)
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "content() should return obj (chainable)")

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
	_ = boundsObj.Set("width", 60)
	_ = boundsObj.Set("height", 20)
	v, err := renderFn(goja.Undefined(), boundsObj)
	require.NoError(t, err)
	out := v.String()
	require.NotEmpty(t, out, "render should produce non-empty string")
}

func TestNoArgsReturnUndefined(t *testing.T) {
	rt := goja.New()
	m := modal.NewModal()
	obj := createModalObject(rt, m).ToObject(rt)

	// content with no args returns obj (guard)
	contentFn, _ := goja.AssertFunction(obj.Get("content"))
	res, err := contentFn(goja.Undefined())
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "content() with no args returns obj")

	// width with no args returns obj (guard)
	widthFn, _ := goja.AssertFunction(obj.Get("width"))
	res, err = widthFn(goja.Undefined())
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "width() with no args returns obj")

	// height with no args returns obj (guard)
	heightFn, _ := goja.AssertFunction(obj.Get("height"))
	res, err = heightFn(goja.Undefined())
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "height() with no args returns obj")

	// border with no args returns obj (guard)
	borderFn, _ := goja.AssertFunction(obj.Get("border"))
	res, err = borderFn(goja.Undefined())
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "border() with no args returns obj")

	// style with no args returns error (TypeError via goja)
	styleFn, _ := goja.AssertFunction(obj.Get("style"))
	_, err = styleFn(goja.Undefined())
	require.Error(t, err, "style() with no args should return error")

	// render with no args returns error (TypeError via goja)
	renderFn, _ := goja.AssertFunction(obj.Get("render"))
	_, err = renderFn(goja.Undefined())
	require.Error(t, err, "render() with no args should return error")
}

func TestModalWithNoArgs(t *testing.T) {
	rt := setupRuntime(t)
	script := `
		const md = require('osm:termui/modal').modal();
		if (typeof md !== 'object') throw new Error('expected object');
		'ok';
	`
	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestModalChaining(t *testing.T) {
	rt := setupRuntime(t)
	script := `
		const lipgloss = require('osm:lipgloss');
		const listMod = require('osm:termui/list');
		const modalMod = require('osm:termui/modal');

		const lst = listMod.list({ items: [{text: 'Content'}] });
		const md = modalMod.modal()
			.content(lst)
			.width(30)
			.height(5)
			.style(lipgloss.newStyle())
			.border(lipgloss.roundedBorder());

		const out = md.render({x: 0, y: 0, width: 60, height: 20});
		if (!out || out.length === 0) throw new Error('render produced empty output');
		'ok';
	`
	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}
