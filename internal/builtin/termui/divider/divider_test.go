package divider

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
	termuidivider "github.com/joeycumines/one-shot-man/internal/termui/divider"
	"github.com/joeycumines/one-shot-man/internal/termui/layout"
	"github.com/stretchr/testify/require"
)

func setupRuntime(t *testing.T) *goja.Runtime {
	t.Helper()
	rt := goja.New()
	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).String()
		switch arg {
		case "osm:termui/divider":
			mod := rt.NewObject()
			Require()(rt, mod)
			return mod.Get("exports")
		}
		return goja.Undefined()
	})
	return rt
}

func TestRequire_ExportsDivider(t *testing.T) {
	rt := goja.New()
	module := rt.NewObject()
	require.NoError(t, module.Set("exports", rt.NewObject()))
	Require()(rt, module)

	exports := module.Get("exports").ToObject(rt)
	require.NotNil(t, exports)

	val := exports.Get("divider")
	require.False(t, goja.IsUndefined(val))
	_, ok := goja.AssertFunction(val)
	require.True(t, ok)
}

func TestRequire_DirectionConstants(t *testing.T) {
	rt := goja.New()
	module := rt.NewObject()
	require.NoError(t, module.Set("exports", rt.NewObject()))
	Require()(rt, module)

	exports := module.Get("exports").ToObject(rt)

	dir := exports.Get("Direction").ToObject(rt)
	require.NotNil(t, dir)
	require.Equal(t, "horizontal", dir.Get("HORIZONTAL").String())
	require.Equal(t, "vertical", dir.Get("VERTICAL").String())
}

func TestJS_API_Surface(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const { divider } = require('osm:termui/divider');
		const d = divider('horizontal');
		const out = d.render({ x: 0, y: 0, width: 20, height: 1 });
		if (typeof out !== 'string') throw new Error('render should return string');
		if (out.length === 0) throw new Error('render output should not be empty');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestJS_API_VerticalDivider(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const { divider } = require('osm:termui/divider');
		const d = divider('vertical');
		const out = d.render({ x: 0, y: 0, width: 1, height: 5 });
		if (typeof out !== 'string') throw new Error('render should return string');
		if (out.length === 0) throw new Error('render output should not be empty');
		if (out.indexOf('│') === -1) throw new Error('vertical char should appear in output');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestJS_API_DirectionConstants(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const { divider, Direction } = require('osm:termui/divider');
		if (Direction.HORIZONTAL !== 'horizontal') throw new Error('HORIZONTAL constant wrong');
		if (Direction.VERTICAL !== 'vertical') throw new Error('VERTICAL constant wrong');
		const d = divider(Direction.HORIZONTAL);
		const out = d.render({ x: 0, y: 0, width: 10, height: 1 });
		if (typeof out !== 'string') throw new Error('render should return string');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestCreateDividerObject_GoInterop(t *testing.T) {
	rt := goja.New()

	d := termuidivider.NewDivider(layout.Horizontal)
	objVal := createDividerObject(rt, d)
	obj := objVal.ToObject(rt)

	// char setter
	charFn, _ := goja.AssertFunction(obj.Get("char"))
	res, err := charFn(goja.Undefined(), rt.ToValue("="))
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "char should return obj (chainable)")

	// render
	renderFn, _ := goja.AssertFunction(obj.Get("render"))
	v, err := renderFn(goja.Undefined(), rt.ToValue(map[string]any{"x": 0, "y": 0, "width": 10, "height": 1}))
	require.NoError(t, err)
	out := v.String()
	require.NotEmpty(t, out, "render should produce non-empty output")
	require.Contains(t, out, "=")
}

func TestNoArgsReturnUndefined(t *testing.T) {
	rt := goja.New()
	d := termuidivider.NewDivider(layout.Horizontal)
	obj := createDividerObject(rt, d).ToObject(rt)

	// char with no args returns obj
	charFn, _ := goja.AssertFunction(obj.Get("char"))
	res, err := charFn(goja.Undefined())
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "char() with no args should return obj")

	// char with null returns obj
	res, err = charFn(goja.Undefined(), goja.Null())
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "char(null) should return obj")
}

func TestRenderWithoutBounds_ThrowsTypeError(t *testing.T) {
	rt := goja.New()
	d := termuidivider.NewDivider(layout.Horizontal)
	obj := createDividerObject(rt, d).ToObject(rt)

	renderFn, _ := goja.AssertFunction(obj.Get("render"))
	_, err := renderFn(goja.Undefined())
	require.Error(t, err)
	require.Contains(t, err.Error(), "TypeError")
}

func TestStyleWithNoArgs_ThrowsTypeError(t *testing.T) {
	rt := goja.New()
	d := termuidivider.NewDivider(layout.Horizontal)
	obj := createDividerObject(rt, d).ToObject(rt)

	styleFn, _ := goja.AssertFunction(obj.Get("style"))
	_, err := styleFn(goja.Undefined())
	require.Error(t, err)
	require.Contains(t, err.Error(), "TypeError")
}

func TestDividerRequiresDirection(t *testing.T) {
	rt := setupRuntime(t)
	script := `
		const { divider } = require('osm:termui/divider');
		try {
			divider();
			throw new Error('should have thrown');
		} catch (e) {
			if (e.message.indexOf('requires a direction') === -1) throw new Error('wrong error: ' + e.message);
		}
		'ok';
	`
	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestDividerInvalidDirection(t *testing.T) {
	rt := setupRuntime(t)
	script := `
		const { divider } = require('osm:termui/divider');
		try {
			divider('diagonal');
			throw new Error('should have thrown');
		} catch (e) {
			if (e.message.indexOf("must be 'horizontal' or 'vertical'") === -1) throw new Error('wrong error: ' + e.message);
		}
		'ok';
	`
	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestDividerWithOpts(t *testing.T) {
	rt := setupRuntime(t)
	script := `
		const { divider } = require('osm:termui/divider');
		const d = divider('horizontal', { char: '=' });
		const out = d.render({ x: 0, y: 0, width: 10, height: 1 });
		if (out.indexOf('=') === -1) throw new Error('custom char should appear');
		'ok';
	`
	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestDividerRender_SmallBounds(t *testing.T) {
	rt := goja.New()
	d := termuidivider.NewDivider(layout.Horizontal)
	obj := createDividerObject(rt, d).ToObject(rt)

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

func TestHorizontalDivider_RenderWidth(t *testing.T) {
	rt := goja.New()
	d := termuidivider.NewDivider(layout.Horizontal)
	obj := createDividerObject(rt, d).ToObject(rt)

	renderFn, _ := goja.AssertFunction(obj.Get("render"))
	v, err := renderFn(goja.Undefined(), rt.ToValue(map[string]any{"x": 0, "y": 0, "width": 20, "height": 1}))
	require.NoError(t, err)
	out := v.String()
	// Horizontal divider should be a single line of width 20
	lines := strings.Split(out, "\n")
	require.Equal(t, 1, len(lines))
}

func TestVerticalDivider_RenderHeight(t *testing.T) {
	rt := goja.New()
	d := termuidivider.NewDivider(layout.Vertical)
	obj := createDividerObject(rt, d).ToObject(rt)

	renderFn, _ := goja.AssertFunction(obj.Get("render"))
	v, err := renderFn(goja.Undefined(), rt.ToValue(map[string]any{"x": 0, "y": 0, "width": 1, "height": 4}))
	require.NoError(t, err)
	out := v.String()
	lines := strings.Split(out, "\n")
	require.Equal(t, 4, len(lines))
}
