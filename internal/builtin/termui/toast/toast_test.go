package toast

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
	termuitoast "github.com/joeycumines/one-shot-man/internal/termui/toast"
	"github.com/stretchr/testify/require"
)

func setupRuntime(t *testing.T) *goja.Runtime {
	t.Helper()
	rt := goja.New()
	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).String()
		switch arg {
		case "osm:termui/toast":
			mod := rt.NewObject()
			Require()(rt, mod)
			return mod.Get("exports")
		}
		return goja.Undefined()
	})
	return rt
}

func TestRequire_ExportsToast(t *testing.T) {
	rt := goja.New()
	module := rt.NewObject()
	require.NoError(t, module.Set("exports", rt.NewObject()))
	Require()(rt, module)

	exports := module.Get("exports").ToObject(rt)
	require.NotNil(t, exports)

	val := exports.Get("toast")
	require.False(t, goja.IsUndefined(val))
	_, ok := goja.AssertFunction(val)
	require.True(t, ok)
}

func TestJS_API_Surface(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const { toast } = require('osm:termui/toast');
		const t = toast({ message: 'Saved!' });
		t.message('Updated!');
		const out = t.render({ x: 0, y: 0, width: 30, height: 3 });
		if (typeof out !== 'string') throw new Error('render should return string');
		if (out.length === 0) throw new Error('render output should not be empty');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestCreateToastObject_GoInterop(t *testing.T) {
	rt := goja.New()

	tt := termuitoast.NewToast()
	objVal := createToastObject(rt, tt)
	obj := objVal.ToObject(rt)

	// message setter
	messageFn, _ := goja.AssertFunction(obj.Get("message"))
	res, err := messageFn(goja.Undefined(), rt.ToValue("Hello"))
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "message should return obj (chainable)")
	require.Equal(t, "Hello", tt.Message)

	// width setter
	widthFn, _ := goja.AssertFunction(obj.Get("width"))
	res, err = widthFn(goja.Undefined(), rt.ToValue(20))
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "width should return obj (chainable)")
	require.Equal(t, 20, tt.Width)

	// render
	renderFn, _ := goja.AssertFunction(obj.Get("render"))
	v, err := renderFn(goja.Undefined(), rt.ToValue(map[string]any{"x": 0, "y": 0, "width": 30, "height": 3}))
	require.NoError(t, err)
	out := v.String()
	require.NotEmpty(t, out, "render should produce non-empty output")
}

func TestNoArgsReturnUndefined(t *testing.T) {
	rt := goja.New()
	tt := termuitoast.NewToast()
	obj := createToastObject(rt, tt).ToObject(rt)

	// message with no args returns obj
	messageFn, _ := goja.AssertFunction(obj.Get("message"))
	res, err := messageFn(goja.Undefined())
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "message() with no args should return obj")

	// width with no args returns obj
	widthFn, _ := goja.AssertFunction(obj.Get("width"))
	res, err = widthFn(goja.Undefined())
	require.NoError(t, err)
	require.False(t, goja.IsUndefined(res), "width() with no args should return obj")
}

func TestRenderWithoutBounds_ThrowsTypeError(t *testing.T) {
	rt := goja.New()
	tt := termuitoast.NewToast()
	obj := createToastObject(rt, tt).ToObject(rt)

	renderFn, _ := goja.AssertFunction(obj.Get("render"))
	_, err := renderFn(goja.Undefined())
	require.Error(t, err)
	require.Contains(t, err.Error(), "TypeError")
}

func TestStyleWithNoArgs_ThrowsTypeError(t *testing.T) {
	rt := goja.New()
	tt := termuitoast.NewToast()
	obj := createToastObject(rt, tt).ToObject(rt)

	styleFn, _ := goja.AssertFunction(obj.Get("style"))
	_, err := styleFn(goja.Undefined())
	require.Error(t, err)
	require.Contains(t, err.Error(), "TypeError")
}

func TestToastWithNoArgs(t *testing.T) {
	rt := setupRuntime(t)
	script := `
		const { toast } = require('osm:termui/toast');
		const t = toast();
		if (typeof t !== 'object') throw new Error('expected object');
		'ok';
	`
	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestToastRender_SmallBounds(t *testing.T) {
	rt := goja.New()
	tt := termuitoast.NewToast(termuitoast.WithToastMessage("hi"))
	obj := createToastObject(rt, tt).ToObject(rt)

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

func TestToastMessageInRender(t *testing.T) {
	rt := setupRuntime(t)
	script := `
		const { toast } = require('osm:termui/toast');
		const t = toast({ message: 'Saved!' });
		const out = t.render({ x: 0, y: 0, width: 30, height: 3 });
		if (out.indexOf('Saved') === -1) throw new Error('message should appear in rendered output');
		'ok';
	`
	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestToastWidthConstraint(t *testing.T) {
	rt := setupRuntime(t)
	script := `
		const { toast } = require('osm:termui/toast');
		const t = toast({ message: 'Hello World', width: 5 });
		const out = t.render({ x: 0, y: 0, width: 30, height: 1 });
		if (typeof out !== 'string') throw new Error('render should return string');
		'ok';
	`
	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestToastRenderPositionedAtBottom(t *testing.T) {
	rt := goja.New()
	tt := termuitoast.NewToast(termuitoast.WithToastMessage("bottom"))
	obj := createToastObject(rt, tt).ToObject(rt)

	renderFn, _ := goja.AssertFunction(obj.Get("render"))
	v, err := renderFn(goja.Undefined(), rt.ToValue(map[string]any{"x": 0, "y": 0, "width": 20, "height": 4}))
	require.NoError(t, err)
	out := v.String()
	// Toast should be positioned at bottom, so there should be newlines before the message
	lines := strings.Split(out, "\n")
	require.Equal(t, 4, len(lines), "toast should fill the height with newlines + message")
}

func TestToastWidthViaOpts(t *testing.T) {
	rt := setupRuntime(t)
	script := `
		const { toast } = require('osm:termui/toast');
		const t = toast({ message: 'Hi', width: 10 });
		t.width(15);
		const out = t.render({ x: 0, y: 0, width: 30, height: 1 });
		if (typeof out !== 'string') throw new Error('render should return string');
		'ok';
	`
	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestToastGoInterop_WidthField(t *testing.T) {
	rt := goja.New()
	tt := termuitoast.NewToast(termuitoast.WithToastMessage("msg"), termuitoast.WithToastWidth(42))
	obj := createToastObject(rt, tt).ToObject(rt)

	require.Equal(t, 42, tt.Width)
	require.Equal(t, "msg", tt.Message)

	// Update width via JS
	widthFn, _ := goja.AssertFunction(obj.Get("width"))
	_, err := widthFn(goja.Undefined(), rt.ToValue(50))
	require.NoError(t, err)
	require.Equal(t, 50, tt.Width)
}
