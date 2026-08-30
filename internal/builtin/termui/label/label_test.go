package label

import (
	"testing"

	"github.com/joeycumines/goja"
	termuilabel "github.com/joeycumines/one-shot-man/internal/termui/label"
	"github.com/stretchr/testify/require"
)

func setupRuntime(t *testing.T) *goja.Runtime {
	t.Helper()
	rt := goja.New()
	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).String()
		switch arg {
		case "osm:termui/label":
			mod := rt.NewObject()
			Require()(rt, mod)
			return mod.Get("exports")
		}
		return goja.Undefined()
	})
	return rt
}

func TestRequire_ExportsLabel(t *testing.T) {
	rt := goja.New()
	module := rt.NewObject()
	require.NoError(t, module.Set("exports", rt.NewObject()))
	Require()(rt, module)

	exports := module.Get("exports").ToObject(rt)
	require.NotNil(t, exports)

	val := exports.Get("label")
	require.False(t, goja.IsUndefined(val))
	_, ok := goja.AssertFunction(val)
	require.True(t, ok)
}

func TestJS_API_Surface(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const { label } = require('osm:termui/label');
		const l = label('Hello World');
		const out = l.render({ x: 0, y: 0, width: 40, height: 1 });
		if (typeof out !== 'string') throw new Error('render should return string');
		if (out.length === 0) throw new Error('render output should not be empty');
		if (out.indexOf('Hello') === -1) throw new Error('render should contain text');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestCreateLabelObject_GoInterop(t *testing.T) {
	rt := goja.New()

	l := termuilabel.NewLabel("test")
	objVal := createLabelObject(rt, l)
	obj := objVal.ToObject(rt)

	// maxWidth setter
	maxWidthFn, _ := goja.AssertFunction(obj.Get("maxWidth"))
	res, err := maxWidthFn(goja.Undefined(), rt.ToValue(20))
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "maxWidth should return obj (chainable)")

	// maxHeight setter
	maxHeightFn, _ := goja.AssertFunction(obj.Get("maxHeight"))
	res, err = maxHeightFn(goja.Undefined(), rt.ToValue(3))
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "maxHeight should return obj (chainable)")

	// render
	renderFn, _ := goja.AssertFunction(obj.Get("render"))
	v, err := renderFn(goja.Undefined(), rt.ToValue(map[string]any{"x": 0, "y": 0, "width": 50, "height": 10}))
	require.NoError(t, err)
	out := v.String()
	require.NotEmpty(t, out, "render should produce non-empty output")
	require.Contains(t, out, "test")
}

func TestNoArgsReturnUndefined(t *testing.T) {
	rt := goja.New()
	l := termuilabel.NewLabel("x")
	obj := createLabelObject(rt, l).ToObject(rt)

	// maxWidth with no args returns obj
	maxWidthFn, _ := goja.AssertFunction(obj.Get("maxWidth"))
	res, err := maxWidthFn(goja.Undefined())
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "maxWidth() with no args should return obj")

	// maxHeight with no args returns obj
	maxHeightFn, _ := goja.AssertFunction(obj.Get("maxHeight"))
	res, err = maxHeightFn(goja.Undefined())
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "maxHeight() with no args should return obj")
}

func TestRenderWithoutBounds_ThrowsTypeError(t *testing.T) {
	rt := goja.New()
	l := termuilabel.NewLabel("test")
	obj := createLabelObject(rt, l).ToObject(rt)

	renderFn, _ := goja.AssertFunction(obj.Get("render"))
	_, err := renderFn(goja.Undefined())
	require.Error(t, err)
	require.Contains(t, err.Error(), "TypeError")
}

func TestStyleWithNoArgs_ThrowsTypeError(t *testing.T) {
	rt := goja.New()
	l := termuilabel.NewLabel("test")
	obj := createLabelObject(rt, l).ToObject(rt)

	styleFn, _ := goja.AssertFunction(obj.Get("style"))
	_, err := styleFn(goja.Undefined())
	require.Error(t, err)
	require.Contains(t, err.Error(), "TypeError")
}

func TestLabelRequiresText(t *testing.T) {
	rt := setupRuntime(t)
	script := `
		const { label } = require('osm:termui/label');
		try {
			label();
			throw new Error('should have thrown');
		} catch (e) {
			if (e.message.indexOf('requires a text string') === -1) throw new Error('wrong error: ' + e.message);
		}
		'ok';
	`
	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestLabelWithOpts(t *testing.T) {
	rt := setupRuntime(t)
	script := `
		const { label } = require('osm:termui/label');
		const l = label('Hello', { maxWidth: 5, maxHeight: 1 });
		const out = l.render({ x: 0, y: 0, width: 40, height: 10 });
		if (typeof out !== 'string') throw new Error('render should return string');
		'ok';
	`
	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestLabelRender_SmallBounds(t *testing.T) {
	rt := goja.New()
	l := termuilabel.NewLabel("test")
	obj := createLabelObject(rt, l).ToObject(rt)

	renderFn, _ := goja.AssertFunction(obj.Get("render"))
	// zero width should return empty string
	v, err := renderFn(goja.Undefined(), rt.ToValue(map[string]any{"x": 0, "y": 0, "width": 0, "height": 5}))
	require.NoError(t, err)
	require.Equal(t, "", v.String())

	// zero height should return empty string
	v, err = renderFn(goja.Undefined(), rt.ToValue(map[string]any{"x": 0, "y": 0, "width": 5, "height": 0}))
	require.NoError(t, err)
	require.Equal(t, "", v.String())
}

func TestLabelMaxWidthConstraint(t *testing.T) {
	rt := setupRuntime(t)
	script := `
		const { label } = require('osm:termui/label');
		const l = label('ABCDEFGHIJ', { maxWidth: 5 });
		const out = l.render({ x: 0, y: 0, width: 40, height: 1 });
		// maxWidth should constrain the output
		if (out.length > 20) throw new Error('maxWidth should constrain output, got: ' + out.length);
		'ok';
	`
	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestLabelNullArg_ThrowsTypeError(t *testing.T) {
	rt := setupRuntime(t)
	script := `
		const { label } = require('osm:termui/label');
		try {
			label(null);
			throw new Error('should have thrown');
		} catch (e) {
			if (e.message.indexOf('requires a text string') === -1) throw new Error('wrong error: ' + e.message);
		}
		'ok';
	`
	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}
