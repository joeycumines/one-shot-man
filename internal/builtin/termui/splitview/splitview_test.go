package splitview

import (
	"testing"

	"github.com/dop251/goja"
	splitviewsb "github.com/joeycumines/one-shot-man/internal/termui/splitview"
	"github.com/stretchr/testify/require"
)

func setupRuntime(t *testing.T) *goja.Runtime {
	t.Helper()
	rt := goja.New()
	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).String()
		switch arg {
		case "osm:termui/splitview":
			mod := rt.NewObject()
			Require()(rt, mod)
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

	// splitView factory
	val := exports.Get("splitView")
	require.False(t, goja.IsUndefined(val))
	_, ok := goja.AssertFunction(val)
	require.True(t, ok)

	// Direction constants
	dir := exports.Get("Direction").ToObject(rt)
	require.NotNil(t, dir)
	require.Equal(t, "horizontal", dir.Get("HORIZONTAL").String())
	require.Equal(t, "vertical", dir.Get("VERTICAL").String())
}

func TestJS_API_Surface(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const sv = require('osm:termui/splitview');

		// Create with no args
		const view = sv.splitView();
		if (typeof view !== 'object') throw new Error('expected object');

		// Direction constants
		if (sv.Direction.HORIZONTAL !== 'horizontal') throw new Error('HORIZONTAL');
		if (sv.Direction.VERTICAL !== 'vertical') throw new Error('VERTICAL');

		// Chainable methods return the object
		const chained = view.ratio(0.6).direction('vertical');
		if (chained !== view) throw new Error('chaining should return same object');

		// Render with empty components
		const rect = {x: 0, y: 0, width: 80, height: 24};
		const result = view.render(rect);
		if (typeof result !== 'string') throw new Error('render should return string');

		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestCreateSplitViewObject_GoInterop(t *testing.T) {
	rt := goja.New()

	s := splitviewsb.NewSplitView()
	objVal := createSplitViewObject(rt, s)
	obj := objVal.ToObject(rt)

	require.Equal(t, "termui/splitview", obj.Get("_type").String())

	// ratio — chainable
	ratioFn, _ := goja.AssertFunction(obj.Get("ratio"))
	res, err := ratioFn(goja.Undefined(), rt.ToValue(0.7))
	require.NoError(t, err)
	require.Equal(t, obj, res) // chainable returns same obj
	require.Equal(t, 0.7, s.Ratio)

	// direction — chainable
	dirFn, _ := goja.AssertFunction(obj.Get("direction"))
	res, err = dirFn(goja.Undefined(), rt.ToValue("vertical"))
	require.NoError(t, err)
	require.Equal(t, obj, res)

	// render
	renderFn, _ := goja.AssertFunction(obj.Get("render"))
	bounds := rt.NewObject()
	_ = bounds.Set("x", 0)
	_ = bounds.Set("y", 0)
	_ = bounds.Set("width", 80)
	_ = bounds.Set("height", 24)
	res, err = renderFn(goja.Undefined(), bounds)
	require.NoError(t, err)
	require.Equal(t, "", res.String()) // no components set, empty render
}

func TestNoArgsReturnUndefined(t *testing.T) {
	rt := goja.New()

	s := splitviewsb.NewSplitView()
	obj := createSplitViewObject(rt, s).ToObject(rt)

	// primary with no args returns obj (not undefined)
	primaryFn, _ := goja.AssertFunction(obj.Get("primary"))
	res, err := primaryFn(goja.Undefined())
	require.NoError(t, err)
	require.Equal(t, obj, res) // returns obj for chaining

	// secondary with no args returns obj
	secondaryFn, _ := goja.AssertFunction(obj.Get("secondary"))
	res, err = secondaryFn(goja.Undefined())
	require.NoError(t, err)
	require.Equal(t, obj, res)

	// ratio with no args returns obj
	ratioFn, _ := goja.AssertFunction(obj.Get("ratio"))
	res, err = ratioFn(goja.Undefined())
	require.NoError(t, err)
	require.Equal(t, obj, res)

	// direction with no args returns obj
	dirFn, _ := goja.AssertFunction(obj.Get("direction"))
	res, err = dirFn(goja.Undefined())
	require.NoError(t, err)
	require.Equal(t, obj, res)
}

func TestSplitView_WithOpts(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const sv = require('osm:termui/splitview');
		const view = sv.splitView({ ratio: 0.3, direction: 'vertical' });
		const rect = {x: 0, y: 0, width: 80, height: 24};
		const result = view.render(rect);
		if (typeof result !== 'string') throw new Error('render should return string');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestSplitView_RenderZeroBounds(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const sv = require('osm:termui/splitview');
		const view = sv.splitView();
		const result = view.render({x: 0, y: 0, width: 0, height: 0});
		if (result !== '') throw new Error('zero bounds should render empty string');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestSplitView_RenderNegativeBounds(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const sv = require('osm:termui/splitview');
		const view = sv.splitView();
		const result = view.render({x: 0, y: 0, width: -10, height: -5});
		if (result !== '') throw new Error('negative bounds should render empty string');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestSplitView_RenderRequiresBounds(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const sv = require('osm:termui/splitview');
		const view = sv.splitView();
		try {
			view.render();
			throw new Error('should have thrown');
		} catch (e) {
			if (!e.message.includes('render requires')) throw new Error('wrong error: ' + e.message);
		}
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestSplitView_StyleRequiresArg(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const sv = require('osm:termui/splitview');
		const view = sv.splitView();
		try {
			view.style();
			throw new Error('should have thrown');
		} catch (e) {
			if (!e.message.includes('style requires')) throw new Error('wrong error: ' + e.message);
		}
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestSplitView_InvalidDirection(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const sv = require('osm:termui/splitview');
		const view = sv.splitView();
		try {
			view.direction('diagonal');
			throw new Error('should have thrown');
		} catch (e) {
			if (!e.message.includes("direction must be")) throw new Error('wrong error: ' + e.message);
		}
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}
