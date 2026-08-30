package compositor

import (
	"testing"

	"github.com/joeycumines/goja"
	compositorsb "github.com/joeycumines/one-shot-man/internal/termui/compositor"
	"github.com/stretchr/testify/require"
)

func setupRuntime(t *testing.T) *goja.Runtime {
	t.Helper()
	rt := goja.New()
	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).String()
		switch arg {
		case "osm:termui/compositor":
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

	val := exports.Get("compositor")
	require.False(t, goja.IsUndefined(val))
	_, ok := goja.AssertFunction(val)
	require.True(t, ok)
}

func TestJS_API_Surface(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const comp = require('osm:termui/compositor');
		const c = comp.compositor({width: 80, height: 24});

		// Add panes (chainable)
		c.addPane({id: 'main', content: 'hello', bounds: {x: 0, y: 0, width: 40, height: 12}})
		 .addPane({id: 'side', content: 'world', bounds: {x: 40, y: 0, width: 40, height: 12}});

		// paneIds
		const ids = c.paneIds();
		if (!Array.isArray(ids)) throw new Error('paneIds should return array');
		if (ids.length !== 2) throw new Error('expected 2 pane ids');

		// render
		const rendered = c.render();
		if (typeof rendered !== 'string') throw new Error('render should return string');

		// hit
		const hit = c.hit(5, 5);
		if (typeof hit !== 'object') throw new Error('hit should return object');
		if (typeof hit.id !== 'string') throw new Error('hit.id should be string');
		if (typeof hit.hit !== 'boolean') throw new Error('hit.hit should be boolean');

		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestCreateCompositorObject_GoInterop(t *testing.T) {
	rt := goja.New()

	c := compositorsb.NewCompositor(80, 24)
	objVal := createCompositorObject(rt, c)
	obj := objVal.ToObject(rt)

	require.Equal(t, "termui/compositor", obj.Get("_type").String())

	// addPane — chainable
	addPaneFn, _ := goja.AssertFunction(obj.Get("addPane"))
	cfg := rt.NewObject()
	_ = cfg.Set("id", "main")
	_ = cfg.Set("content", "hello")
	bounds := rt.NewObject()
	_ = bounds.Set("x", 0)
	_ = bounds.Set("y", 0)
	_ = bounds.Set("width", 40)
	_ = bounds.Set("height", 12)
	_ = cfg.Set("bounds", bounds)
	res, err := addPaneFn(goja.Undefined(), cfg)
	require.NoError(t, err)
	require.Equal(t, obj, res) // chainable

	// paneIds
	paneIdsFn, _ := goja.AssertFunction(obj.Get("paneIds"))
	res, err = paneIdsFn(goja.Undefined())
	require.NoError(t, err)
	idsArr := res.ToObject(rt)
	require.Equal(t, int64(1), idsArr.Get("length").ToInteger())
	require.Equal(t, "main", idsArr.Get("0").String())

	// updatePane — chainable
	updatePaneFn, _ := goja.AssertFunction(obj.Get("updatePane"))
	updCfg := rt.NewObject()
	_ = updCfg.Set("id", "main")
	_ = updCfg.Set("content", "updated")
	res, err = updatePaneFn(goja.Undefined(), updCfg)
	require.NoError(t, err)
	require.Equal(t, obj, res)

	// removePane — chainable
	removePaneFn, _ := goja.AssertFunction(obj.Get("removePane"))
	res, err = removePaneFn(goja.Undefined(), rt.ToValue("main"))
	require.NoError(t, err)
	require.Equal(t, obj, res)

	// paneIds after remove
	res, err = paneIdsFn(goja.Undefined())
	require.NoError(t, err)
	idsArr = res.ToObject(rt)
	require.Equal(t, int64(0), idsArr.Get("length").ToInteger())
}

func TestNoArgsReturnUndefined(t *testing.T) {
	rt := goja.New()

	c := compositorsb.NewCompositor(80, 24)
	obj := createCompositorObject(rt, c).ToObject(rt)

	// addPane with no args should throw
	addPaneFn, _ := goja.AssertFunction(obj.Get("addPane"))
	_, err := addPaneFn(goja.Undefined())
	require.Error(t, err)

	// removePane with no args should throw
	removePaneFn, _ := goja.AssertFunction(obj.Get("removePane"))
	_, err = removePaneFn(goja.Undefined())
	require.Error(t, err)
}

func TestCompositor_ZOrdering(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const comp = require('osm:termui/compositor');
		const c = comp.compositor({width: 80, height: 24});

		// Pane at z=0 with content that fills a row
		c.addPane({id: 'pane', content: 'background', bounds: {x: 0, y: 0, width: 80, height: 24}, z: 0});

		// Chrome at z=10 overlays pane at same position
		c.addChrome({id: 'overlay', content: 'foreground', bounds: {x: 0, y: 0, width: 80, height: 1}, z: 10});

		// Hit test at (0,0) — both layers cover this cell; chrome (z=10) should win
		const hit = c.hit(0, 0);
		if (!hit.hit) throw new Error('expected hit at (0,0)');
		if (hit.id !== 'overlay') throw new Error('expected overlay id, got: ' + hit.id);

		// Hit test beyond content dimensions — lipgloss layers are sized by content,
		// so "background" (10 chars, 1 line) only covers (0,0)-(9,0). Hit at (0,5) misses.
		const hitMiss = c.hit(0, 5);
		if (hitMiss.hit) throw new Error('expected miss at (0,5), content is only 1 line tall');

		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestCompositor_UpdatePaneIfNew(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const comp = require('osm:termui/compositor');
		const c = comp.compositor({width: 80, height: 24});

		c.addPane({id: 'test', content: 'v1', bounds: {x: 0, y: 0, width: 40, height: 12}, z: 0});

		// Update with gen=1 (first update, should succeed)
		c.updatePaneIfNew({id: 'test', content: 'v2', gen: 1});

		// Update with same gen=1 (should be no-op, content stays v2)
		c.updatePaneIfNew({id: 'test', content: 'v3', gen: 1});

		// Update with gen=2 (should succeed)
		c.updatePaneIfNew({id: 'test', content: 'v4', gen: 2});

		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestCompositor_PaneIdsAfterAddRemove(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const comp = require('osm:termui/compositor');
		const c = comp.compositor({width: 80, height: 24});

		c.addPane({id: 'a', content: 'A', bounds: {x: 0, y: 0, width: 20, height: 12}, z: 0});
		c.addPane({id: 'b', content: 'B', bounds: {x: 20, y: 0, width: 20, height: 12}, z: 0});
		c.addPane({id: 'c', content: 'C', bounds: {x: 40, y: 0, width: 20, height: 12}, z: 0});

		let ids = c.paneIds();
		if (ids.length !== 3) throw new Error('expected 3 ids');
		if (ids[0] !== 'a' || ids[1] !== 'b' || ids[2] !== 'c') throw new Error('wrong order: ' + JSON.stringify(ids));

		// Remove middle
		c.removePane('b');
		ids = c.paneIds();
		if (ids.length !== 2) throw new Error('expected 2 ids after remove');
		if (ids[0] !== 'a' || ids[1] !== 'c') throw new Error('wrong order after remove: ' + JSON.stringify(ids));

		// Add replacement
		c.addPane({id: 'd', content: 'D', bounds: {x: 60, y: 0, width: 20, height: 12}, z: 0});
		ids = c.paneIds();
		if (ids.length !== 3) throw new Error('expected 3 ids after add');
		if (ids[2] !== 'd') throw new Error('new id should be last');

		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestCompositor_ChromeManagement(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const comp = require('osm:termui/compositor');
		const c = comp.compositor({width: 80, height: 24});

		// addChrome — chainable
		c.addChrome({id: 'border', content: '---', bounds: {x: 0, y: 0, width: 80, height: 1}, z: 10});

		// updateChrome — chainable
		c.updateChrome({id: 'border', content: '==='});

		// removeChrome — chainable
		c.removeChrome('border');

		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestCompositor_Resize(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const comp = require('osm:termui/compositor');
		const c = comp.compositor({width: 80, height: 24});

		// resize — chainable
		c.resize(120, 40);

		// render after resize
		const rendered = c.render();
		if (typeof rendered !== 'string') throw new Error('render should return string');

		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestCompositor_HitNoLayers(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const comp = require('osm:termui/compositor');
		const c = comp.compositor({width: 80, height: 24});

		const hit = c.hit(5, 5);
		if (hit.hit !== false) throw new Error('expected no hit on empty compositor');
		if (hit.id !== '') throw new Error('expected empty id');

		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestCompositor_RequiresConfig(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const comp = require('osm:termui/compositor');
		try {
			comp.compositor();
			throw new Error('should have thrown');
		} catch (e) {
			if (!e.message.includes('compositor requires')) throw new Error('wrong error: ' + e.message);
		}
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestCompositor_UpdatePaneNonExistent(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const comp = require('osm:termui/compositor');
		const c = comp.compositor({width: 80, height: 24});

		// updatePane on non-existent pane is a no-op (doesn't throw)
		c.updatePane({id: 'nonexistent', content: 'test'});

		// updatePaneIfNew on non-existent pane is a no-op
		c.updatePaneIfNew({id: 'nonexistent', content: 'test', gen: 1});

		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestCompositor_RemovePaneNonExistent(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const comp = require('osm:termui/compositor');
		const c = comp.compositor({width: 80, height: 24});

		// removePane on non-existent pane is a no-op
		c.removePane('nonexistent');

		// removeChrome on non-existent chrome is a no-op
		c.removeChrome('nonexistent');

		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestRenderBordered_RoundedBorder(t *testing.T) {
	rt := setupRuntime(t)

	v, err := rt.RunString(`
		var comp = require('osm:termui/compositor');
		var result = comp.renderBordered('hello', comp.roundedBorder(), 10, 3);
		result.length > 0;
	`)
	require.NoError(t, err)
	require.True(t, v.ToBoolean(), "renderBordered should produce output")
}

func TestRenderBordered_NormalBorder(t *testing.T) {
	rt := setupRuntime(t)

	v, err := rt.RunString(`
		var comp = require('osm:termui/compositor');
		var result = comp.renderBordered('world', comp.normalBorder(), 8, 2);
		result.length > 0;
	`)
	require.NoError(t, err)
	require.True(t, v.ToBoolean(), "renderBordered with normal border should produce output")
}

func TestRenderBordered_ContainsContent(t *testing.T) {
	rt := setupRuntime(t)

	v, err := rt.RunString(`
		var comp = require('osm:termui/compositor');
		var result = comp.renderBordered('test', comp.roundedBorder(), 20, 5);
		result.indexOf('test') >= 0;
	`)
	require.NoError(t, err)
	require.True(t, v.ToBoolean(), "renderBordered output should contain the content")
}

func TestRenderBordered_InvalidArgs(t *testing.T) {
	rt := setupRuntime(t)

	_, err := rt.RunString(`require('osm:termui/compositor').renderBordered('a')`)
	require.Error(t, err, "expected error with insufficient args")
}

func TestAddBoundedPane_WithContent(t *testing.T) {
	rt := setupRuntime(t)

	v, err := rt.RunString(`
		var comp = require('osm:termui/compositor');
		var c = comp.compositor({width: 80, height: 24});
		c.addBoundedPane({id: 'test', content: 'hello', bounds: {x: 0, y: 0, width: 20, height: 10}});
		var ids = c.paneIds();
		ids.length === 1 && ids[0] === 'test';
	`)
	require.NoError(t, err)
	require.True(t, v.ToBoolean(), "addBoundedPane with content should create pane")
}

func TestAddBoundedPane_WithContentFn(t *testing.T) {
	rt := setupRuntime(t)

	v, err := rt.RunString(`
		var comp = require('osm:termui/compositor');
		var c = comp.compositor({width: 80, height: 24});
		c.addBoundedPane({id: 'test', contentFn: function() { return 'dynamic'; }, bounds: {x: 0, y: 0, width: 20, height: 10}});
		var ids = c.paneIds();
		ids.length === 1;
	`)
	require.NoError(t, err)
	require.True(t, v.ToBoolean(), "addBoundedPane with contentFn should create pane")
}

func TestAddBoundedPane_Chainable(t *testing.T) {
	rt := setupRuntime(t)

	v, err := rt.RunString(`
		var comp = require('osm:termui/compositor');
		var c = comp.compositor({width: 80, height: 24});
		var result = c.addBoundedPane({id: 'a', content: 'x', bounds: {x: 0, y: 0, width: 20, height: 10}})
		              .addBoundedPane({id: 'b', content: 'y', bounds: {x: 20, y: 0, width: 20, height: 10}});
		var ids = result.paneIds();
		ids.length === 2;
	`)
	require.NoError(t, err)
	require.True(t, v.ToBoolean(), "addBoundedPane should be chainable")
}

func TestAddBoundedPane_WithZ(t *testing.T) {
	rt := setupRuntime(t)

	v, err := rt.RunString(`
		var comp = require('osm:termui/compositor');
		var c = comp.compositor({width: 80, height: 24});
		c.addBoundedPane({id: 'test', content: 'hello', bounds: {x: 0, y: 0, width: 20, height: 10}, z: 5});
		c.paneIds().length === 1;
	`)
	require.NoError(t, err)
	require.True(t, v.ToBoolean(), "addBoundedPane with z should work")
}
