package layout

import (
	"testing"

	"github.com/dop251/goja"
	coord "github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	termlayout "github.com/joeycumines/one-shot-man/internal/termui/layout"
	"github.com/stretchr/testify/require"
)

func setupRuntime(t *testing.T) *goja.Runtime {
	t.Helper()
	rt := goja.New()
	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).String()
		switch arg {
		case "osm:termui/layout":
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

	for _, name := range []string{"split", "grid", "stack"} {
		val := exports.Get(name)
		require.False(t, goja.IsUndefined(val), "exports.%s should be defined", name)
		_, ok := goja.AssertFunction(val)
		require.True(t, ok, "exports.%s should be a function", name)
	}

	// Direction constants
	dir := exports.Get("Direction").ToObject(rt)
	require.NotNil(t, dir)
	require.Equal(t, "horizontal", dir.Get("HORIZONTAL").String())
	require.Equal(t, "vertical", dir.Get("VERTICAL").String())
}

func TestJS_API_Surface(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const layout = require('osm:termui/layout');

		// Direction constants
		if (layout.Direction.HORIZONTAL !== 'horizontal') throw new Error('HORIZONTAL');
		if (layout.Direction.VERTICAL !== 'vertical') throw new Error('VERTICAL');

		// split — horizontal
		const rect = {x: 0, y: 0, width: 100, height: 50};
		const parts = layout.split(rect, 'horizontal', [0.3, 0.7]);
		if (parts.length !== 2) throw new Error('split expected 2 parts');
		if (parts[0].width !== 30) throw new Error('first width expected 30 got ' + parts[0].width);
		if (parts[1].width !== 70) throw new Error('second width expected 70 got ' + parts[1].width);

		// grid — 3 columns, 2 rows
		const cells = layout.grid(rect, 3, 2);
		if (cells.length !== 6) throw new Error('grid expected 6 cells got ' + cells.length);

		// stack — vertical
		const items = layout.stack(rect, 'vertical', [
			{width: 100, height: 10},
			{width: 100, height: 20},
		]);
		if (items.length !== 2) throw new Error('stack expected 2 items');
		if (items[0].height !== 10) throw new Error('first height expected 10');
		if (items[1].height !== 20) throw new Error('second height expected 20');

		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestCreateSplitObject_GoInterop(t *testing.T) {
	rt := goja.New()

	// Test split via Go interop using extractRect and rectSliceToArray
	r := coord.Rect{
		Position: coord.Position{X: 0, Y: 0},
		Size:     coord.Size{Width: 100, Height: 50},
	}
	result := termlayout.Split(r, termlayout.Horizontal, []float64{0.5, 0.5})
	arr := rectSliceToArray(rt, result)
	arrObj := arr.ToObject(rt)
	require.Equal(t, int64(2), arrObj.Get("length").ToInteger())

	first := arrObj.Get("0").ToObject(rt)
	require.Equal(t, int64(50), first.Get("width").ToInteger())
	require.Equal(t, int64(50), first.Get("height").ToInteger())
}

func TestNoArgsReturnUndefined(t *testing.T) {
	rt := setupRuntime(t)

	// split with missing args should throw
	script := `
		const layout = require('osm:termui/layout');
		try {
			layout.split();
			throw new Error('should have thrown');
		} catch (e) {
			if (!e.message.includes('split requires')) throw new Error('wrong error: ' + e.message);
		}
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestSplit_Vertical(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const layout = require('osm:termui/layout');
		const rect = {x: 0, y: 0, width: 100, height: 50};
		const parts = layout.split(rect, 'vertical', [0.4, 0.6]);
		if (parts.length !== 2) throw new Error('expected 2 parts');
		if (parts[0].height !== 20) throw new Error('first height expected 20 got ' + parts[0].height);
		if (parts[1].height !== 30) throw new Error('second height expected 30 got ' + parts[1].height);
		if (parts[0].width !== 100) throw new Error('width should be preserved');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestSplit_ThreeWay(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const layout = require('osm:termui/layout');
		const rect = {x: 0, y: 0, width: 90, height: 24};
		const parts = layout.split(rect, 'horizontal', [1, 2, 3]);
		if (parts.length !== 3) throw new Error('expected 3 parts');
		// Ratios normalized: 1/6, 2/6, 3/6
		// Last gets remainder
		if (parts[0].width !== 15) throw new Error('first width expected 15 got ' + parts[0].width);
		if (parts[1].width !== 30) throw new Error('second width expected 30 got ' + parts[1].width);
		// Last gets remainder: 90 - 15 - 30 = 45
		if (parts[2].width !== 45) throw new Error('third width expected 45 got ' + parts[2].width);
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestGrid_Uniform(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const layout = require('osm:termui/layout');
		const rect = {x: 0, y: 0, width: 90, height: 24};
		const cells = layout.grid(rect, 3, 2);
		if (cells.length !== 6) throw new Error('expected 6 cells');
		// cellW = 30, cellH = 12
		if (cells[0].width !== 30) throw new Error('cell width expected 30 got ' + cells[0].width);
		if (cells[0].height !== 12) throw new Error('cell height expected 12 got ' + cells[0].height);
		// Row-major: (0,0), (30,0), (60,0), (0,12), (30,12), (60,12)
		if (cells[1].x !== 30) throw new Error('cell[1].x expected 30');
		if (cells[3].y !== 12) throw new Error('cell[3].y expected 12');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestGrid_InvalidArgs(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const layout = require('osm:termui/layout');
		try {
			layout.grid();
			throw new Error('should have thrown');
		} catch (e) {
			if (!e.message.includes('grid requires')) throw new Error('wrong error: ' + e.message);
		}
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestStack_Horizontal(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const layout = require('osm:termui/layout');
		const rect = {x: 0, y: 0, width: 100, height: 50};
		const items = layout.stack(rect, 'horizontal', [
			{width: 30, height: 50},
			{width: 40, height: 50},
			{width: 30, height: 50},
		]);
		if (items.length !== 3) throw new Error('expected 3 items');
		if (items[0].x !== 0) throw new Error('item[0].x expected 0');
		if (items[1].x !== 30) throw new Error('item[1].x expected 30');
		if (items[2].x !== 70) throw new Error('item[2].x expected 70');
		if (items[0].width !== 30) throw new Error('item[0].width expected 30');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestStack_InvalidArgs(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const layout = require('osm:termui/layout');
		try {
			layout.stack();
			throw new Error('should have thrown');
		} catch (e) {
			if (!e.message.includes('stack requires')) throw new Error('wrong error: ' + e.message);
		}
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestDirectionConstants(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const layout = require('osm:termui/layout');
		if (layout.Direction.HORIZONTAL !== 'horizontal') throw new Error('HORIZONTAL constant');
		if (layout.Direction.VERTICAL !== 'vertical') throw new Error('VERTICAL constant');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}
