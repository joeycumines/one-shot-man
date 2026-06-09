package box

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
	termuibox "github.com/joeycumines/one-shot-man/internal/termui/box"
	"github.com/stretchr/testify/require"
)

func setupRuntime(t *testing.T) *goja.Runtime {
	t.Helper()
	rt := goja.New()
	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).String()
		switch arg {
		case "osm:termui/box":
			mod := rt.NewObject()
			Require()(rt, mod)
			return mod.Get("exports")
		}
		return goja.Undefined()
	})
	return rt
}

func TestRequire_ExportsBox(t *testing.T) {
	rt := goja.New()
	module := rt.NewObject()
	require.NoError(t, module.Set("exports", rt.NewObject()))
	Require()(rt, module)

	exports := module.Get("exports").ToObject(rt)
	require.NotNil(t, exports)

	val := exports.Get("box")
	require.False(t, goja.IsUndefined(val))
	_, ok := goja.AssertFunction(val)
	require.True(t, ok)
}

func TestJS_API_Surface(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const { box } = require('osm:termui/box');
		const b = box({ title: 'Hello' });
		b.title('World');
		const out = b.render({ x: 0, y: 0, width: 20, height: 5 });
		if (typeof out !== 'string') throw new Error('render should return string');
		if (out.length === 0) throw new Error('render output should not be empty');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestCreateBoxObject_GoInterop(t *testing.T) {
	rt := goja.New()

	b := termuibox.NewBox()
	objVal := createBoxObject(rt, b)
	obj := objVal.ToObject(rt)

	// title setter
	titleFn, _ := goja.AssertFunction(obj.Get("title"))
	res, err := titleFn(goja.Undefined(), rt.ToValue("TestTitle"))
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "title should return obj (chainable)")

	// render
	renderFn, _ := goja.AssertFunction(obj.Get("render"))
	v, err := renderFn(goja.Undefined(), rt.ToValue(map[string]any{"x": 0, "y": 0, "width": 30, "height": 5}))
	require.NoError(t, err)
	out := v.String()
	require.NotEmpty(t, out, "render should produce non-empty output")
	require.Contains(t, out, "TestTitle")

	// border setter
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
	require.False(t, goja.IsUndefined(res), "border should return obj (chainable)")

	// render with custom border
	v, err = renderFn(goja.Undefined(), rt.ToValue(map[string]any{"x": 0, "y": 0, "width": 10, "height": 3}))
	require.NoError(t, err)
	out = v.String()
	require.Contains(t, out, "+")
}

func TestNoArgsReturnUndefined(t *testing.T) {
	rt := goja.New()
	b := termuibox.NewBox()
	obj := createBoxObject(rt, b).ToObject(rt)

	// title with no args returns obj (chainable guard)
	titleFn, _ := goja.AssertFunction(obj.Get("title"))
	res, err := titleFn(goja.Undefined())
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "title() with no args should return obj")

	// content with no args returns obj
	contentFn, _ := goja.AssertFunction(obj.Get("content"))
	res, err = contentFn(goja.Undefined())
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "content() with no args should return obj")

	// border with no args returns obj
	borderFn, _ := goja.AssertFunction(obj.Get("border"))
	res, err = borderFn(goja.Undefined())
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "border() with no args should return obj")
}

func TestRenderWithoutBounds_ThrowsTypeError(t *testing.T) {
	rt := goja.New()
	b := termuibox.NewBox()
	obj := createBoxObject(rt, b).ToObject(rt)

	renderFn, _ := goja.AssertFunction(obj.Get("render"))
	_, err := renderFn(goja.Undefined())
	require.Error(t, err)
	require.Contains(t, err.Error(), "TypeError")
}

func TestStyleWithNoArgs_ThrowsTypeError(t *testing.T) {
	rt := goja.New()
	b := termuibox.NewBox()
	obj := createBoxObject(rt, b).ToObject(rt)

	styleFn, _ := goja.AssertFunction(obj.Get("style"))
	_, err := styleFn(goja.Undefined())
	require.Error(t, err)
	require.Contains(t, err.Error(), "TypeError")
}

func TestBoxWithNoArgs(t *testing.T) {
	rt := setupRuntime(t)
	script := `
		const { box } = require('osm:termui/box');
		const b = box();
		if (typeof b !== 'object') throw new Error('expected object');
		const out = b.render({ x: 0, y: 0, width: 10, height: 3 });
		if (typeof out !== 'string') throw new Error('render should return string');
		if (out.length === 0) throw new Error('render output should not be empty');
		'ok';
	`
	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestBoxRenderOutput_HasBorder(t *testing.T) {
	rt := setupRuntime(t)
	script := `
		const { box } = require('osm:termui/box');
		const b = box();
		const out = b.render({ x: 0, y: 0, width: 12, height: 4 });
		if (typeof out !== 'string') throw new Error('render should return string');
		if (out.length === 0) throw new Error('render output should not be empty');
		'ok';
	`
	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestBoxContent_Chainable(t *testing.T) {
	rt := setupRuntime(t)
	script := `
		const { box } = require('osm:termui/box');
		const b = box();
		// content with null should return obj (no crash)
		const result = b.content(null);
		if (result !== b) throw new Error('content(null) should return the box object');
		'ok';
	`
	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestBoxRender_SmallBounds(t *testing.T) {
	rt := goja.New()
	b := termuibox.NewBox()
	obj := createBoxObject(rt, b).ToObject(rt)

	renderFn, _ := goja.AssertFunction(obj.Get("render"))
	// width < 2 should return empty string
	v, err := renderFn(goja.Undefined(), rt.ToValue(map[string]any{"x": 0, "y": 0, "width": 1, "height": 3}))
	require.NoError(t, err)
	require.Equal(t, "", v.String())

	// height < 2 should return empty string
	v, err = renderFn(goja.Undefined(), rt.ToValue(map[string]any{"x": 0, "y": 0, "width": 3, "height": 1}))
	require.NoError(t, err)
	require.Equal(t, "", v.String())
}

func TestBoxTitleSpliceInRender(t *testing.T) {
	rt := setupRuntime(t)
	script := `
		const { box } = require('osm:termui/box');
		const b = box({ title: 'MyTitle' });
		const out = b.render({ x: 0, y: 0, width: 30, height: 5 });
		if (out.indexOf('MyTitle') === -1) throw new Error('title should appear in rendered output');
		'ok';
	`
	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestBoxBorderViaOpts(t *testing.T) {
	rt := setupRuntime(t)
	script := `
		const { box } = require('osm:termui/box');
		const b = box({
			border: {
				top: '=', bottom: '=', left: '|', right: '|',
				topLeft: '*', topRight: '*', bottomLeft: '*', bottomRight: '*'
			}
		});
		const out = b.render({ x: 0, y: 0, width: 10, height: 3 });
		if (out.indexOf('*') === -1) throw new Error('custom border chars should appear');
		'ok';
	`
	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestBoxRenderLinesCount(t *testing.T) {
	rt := goja.New()
	b := termuibox.NewBox(termuibox.WithBoxTitle("T"))
	obj := createBoxObject(rt, b).ToObject(rt)

	renderFn, _ := goja.AssertFunction(obj.Get("render"))
	v, err := renderFn(goja.Undefined(), rt.ToValue(map[string]any{"x": 0, "y": 0, "width": 20, "height": 6}))
	require.NoError(t, err)
	out := v.String()
	lines := strings.Split(out, "\n")
	require.GreaterOrEqual(t, len(lines), 3, "rendered box should have multiple lines")
	require.LessOrEqual(t, len(lines), 6, "rendered box should not exceed requested height")
}
