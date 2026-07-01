package coordinate

import (
	"testing"

	"github.com/joeycumines/goja"
	coordsb "github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/stretchr/testify/require"
)

func setupRuntime(t *testing.T) *goja.Runtime {
	t.Helper()
	rt := goja.New()
	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).String()
		switch arg {
		case "osm:termui/coordinate":
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

	for _, name := range []string{"position", "size", "rect", "layer", "fromPaneGeometry", "fromLayer"} {
		val := exports.Get(name)
		require.False(t, goja.IsUndefined(val), "exports.%s should be defined", name)
		_, ok := goja.AssertFunction(val)
		require.True(t, ok, "exports.%s should be a function", name)
	}
}

func TestJS_API_Surface(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const coord = require('osm:termui/coordinate');

		// Position
		const p = coord.position({x: 3, y: 5});
		if (p.x !== 3) throw new Error('p.x expected 3 got ' + p.x);
		if (p.y !== 5) throw new Error('p.y expected 5 got ' + p.y);
		if (p.toString() !== '(3,5)') throw new Error('position toString: ' + p.toString());

		// Size
		const s = coord.size({width: 80, height: 24});
		if (s.width !== 80) throw new Error('s.width expected 80');
		if (s.height !== 24) throw new Error('s.height expected 24');
		if (s.area() !== 1920) throw new Error('area expected 1920 got ' + s.area());
		if (s.toString() !== '80x24') throw new Error('size toString: ' + s.toString());

		// Rect
		const r = coord.rect({x: 0, y: 0, width: 80, height: 24});
		if (r.x !== 0) throw new Error('r.x expected 0');
		if (r.y !== 0) throw new Error('r.y expected 0');
		if (r.width !== 80) throw new Error('r.width expected 80');
		if (r.height !== 24) throw new Error('r.height expected 24');
		if (r.toString() !== '(0,0) 80x24') throw new Error('rect toString: ' + r.toString());

		// Layer
		const l = coord.layer({x: 1, y: 2, width: 40, height: 12, z: 3});
		if (l.x !== 1) throw new Error('l.x expected 1');
		if (l.y !== 2) throw new Error('l.y expected 2');
		if (l.width !== 40) throw new Error('l.width expected 40');
		if (l.height !== 12) throw new Error('l.height expected 12');
		if (l.z !== 3) throw new Error('l.z expected 3');
		if (l.toString() !== '(1,2) 40x12 z:3') throw new Error('layer toString: ' + l.toString());

		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestCreatePositionObject_GoInterop(t *testing.T) {
	rt := goja.New()

	p := &coordsb.Position{X: 10, Y: 20}
	objVal := createPositionObject(rt, p)
	obj := objVal.ToObject(rt)

	// Getters
	require.Equal(t, int64(10), obj.Get("x").ToInteger())
	require.Equal(t, int64(20), obj.Get("y").ToInteger())

	// Setters via accessor property — set via obj.Set
	_ = obj.Set("x", rt.ToValue(99))
	require.Equal(t, 99, p.X)

	_ = obj.Set("y", rt.ToValue(88))
	require.Equal(t, 88, p.Y)

	// add
	addFn, _ := goja.AssertFunction(obj.Get("add"))
	other := rt.NewObject()
	_ = other.Set("x", 1)
	_ = other.Set("y", 2)
	res, err := addFn(goja.Undefined(), other)
	require.NoError(t, err)
	resObj := res.ToObject(rt)
	require.Equal(t, int64(100), resObj.Get("x").ToInteger())
	require.Equal(t, int64(90), resObj.Get("y").ToInteger())

	// sub
	subFn, _ := goja.AssertFunction(obj.Get("sub"))
	res, err = subFn(goja.Undefined(), other)
	require.NoError(t, err)
	resObj = res.ToObject(rt)
	require.Equal(t, int64(98), resObj.Get("x").ToInteger())
	require.Equal(t, int64(86), resObj.Get("y").ToInteger())

	// in
	inFn, _ := goja.AssertFunction(obj.Get("in"))
	rectObj := rt.NewObject()
	_ = rectObj.Set("x", 0)
	_ = rectObj.Set("y", 0)
	_ = rectObj.Set("width", 200)
	_ = rectObj.Set("height", 200)
	res, err = inFn(goja.Undefined(), rectObj)
	require.NoError(t, err)
	require.True(t, res.ToBoolean())

	// toString
	toStringFn, _ := goja.AssertFunction(obj.Get("toString"))
	res, err = toStringFn(goja.Undefined())
	require.NoError(t, err)
	require.Equal(t, "(99,88)", res.String())
}

func TestCreateSizeObject_GoInterop(t *testing.T) {
	rt := goja.New()

	s := &coordsb.Size{Width: 80, Height: 24}
	objVal := createSizeObject(rt, s)
	obj := objVal.ToObject(rt)

	// Getters
	require.Equal(t, int64(80), obj.Get("width").ToInteger())
	require.Equal(t, int64(24), obj.Get("height").ToInteger())

	// Setters
	_ = obj.Set("width", rt.ToValue(100))
	require.Equal(t, 100, s.Width)
	_ = obj.Set("height", rt.ToValue(50))
	require.Equal(t, 50, s.Height)

	// area
	areaFn, _ := goja.AssertFunction(obj.Get("area"))
	res, err := areaFn(goja.Undefined())
	require.NoError(t, err)
	require.Equal(t, int64(5000), res.ToInteger())

	// contains — larger contains smaller
	containsFn, _ := goja.AssertFunction(obj.Get("contains"))
	smaller := rt.NewObject()
	_ = smaller.Set("width", 40)
	_ = smaller.Set("height", 25)
	res, err = containsFn(goja.Undefined(), smaller)
	require.NoError(t, err)
	require.True(t, res.ToBoolean())

	// contains — does not contain larger
	bigger := rt.NewObject()
	_ = bigger.Set("width", 200)
	_ = bigger.Set("height", 50)
	res, err = containsFn(goja.Undefined(), bigger)
	require.NoError(t, err)
	require.False(t, res.ToBoolean())

	// toString
	toStringFn, _ := goja.AssertFunction(obj.Get("toString"))
	res, err = toStringFn(goja.Undefined())
	require.NoError(t, err)
	require.Equal(t, "100x50", res.String())
}

func TestCreateRectObject_GoInterop(t *testing.T) {
	rt := goja.New()

	r := &coordsb.Rect{
		Position: coordsb.Position{X: 0, Y: 0},
		Size:     coordsb.Size{Width: 80, Height: 24},
	}
	objVal := createRectObject(rt, r)
	obj := objVal.ToObject(rt)

	// Getters
	require.Equal(t, int64(0), obj.Get("x").ToInteger())
	require.Equal(t, int64(0), obj.Get("y").ToInteger())
	require.Equal(t, int64(80), obj.Get("width").ToInteger())
	require.Equal(t, int64(24), obj.Get("height").ToInteger())

	// Setters
	_ = obj.Set("x", rt.ToValue(5))
	require.Equal(t, 5, r.Position.X)
	_ = obj.Set("y", rt.ToValue(10))
	require.Equal(t, 10, r.Position.Y)
	_ = obj.Set("width", rt.ToValue(60))
	require.Equal(t, 60, r.Size.Width)
	_ = obj.Set("height", rt.ToValue(20))
	require.Equal(t, 20, r.Size.Height)

	// contains — position inside
	containsFn, _ := goja.AssertFunction(obj.Get("contains"))
	pos := rt.NewObject()
	_ = pos.Set("x", 6)
	_ = pos.Set("y", 15)
	res, err := containsFn(goja.Undefined(), pos)
	require.NoError(t, err)
	require.True(t, res.ToBoolean())

	// contains — position outside
	posOut := rt.NewObject()
	_ = posOut.Set("x", 100)
	_ = posOut.Set("y", 100)
	res, err = containsFn(goja.Undefined(), posOut)
	require.NoError(t, err)
	require.False(t, res.ToBoolean())

	// overlaps — overlapping rect
	overlapsFn, _ := goja.AssertFunction(obj.Get("overlaps"))
	other := rt.NewObject()
	_ = other.Set("x", 50)
	_ = other.Set("y", 0)
	_ = other.Set("width", 30)
	_ = other.Set("height", 20)
	res, err = overlapsFn(goja.Undefined(), other)
	require.NoError(t, err)
	require.True(t, res.ToBoolean())

	// overlaps — non-overlapping rect
	farAway := rt.NewObject()
	_ = farAway.Set("x", 200)
	_ = farAway.Set("y", 200)
	_ = farAway.Set("width", 10)
	_ = farAway.Set("height", 10)
	res, err = overlapsFn(goja.Undefined(), farAway)
	require.NoError(t, err)
	require.False(t, res.ToBoolean())

	// inset
	insetFn, _ := goja.AssertFunction(obj.Get("inset"))
	insetSize := rt.NewObject()
	_ = insetSize.Set("width", 2)
	_ = insetSize.Set("height", 1)
	res, err = insetFn(goja.Undefined(), insetSize)
	require.NoError(t, err)
	insetObj := res.ToObject(rt)
	require.Equal(t, int64(7), insetObj.Get("x").ToInteger())       // 5+2
	require.Equal(t, int64(11), insetObj.Get("y").ToInteger())      // 10+1
	require.Equal(t, int64(56), insetObj.Get("width").ToInteger())  // 60-4
	require.Equal(t, int64(18), insetObj.Get("height").ToInteger()) // 20-2

	// split — horizontal
	splitFn, _ := goja.AssertFunction(obj.Get("split"))
	res, err = splitFn(goja.Undefined(), rt.ToValue(0.5), rt.ToValue(true))
	require.NoError(t, err)
	arr := res.ToObject(rt)
	require.Equal(t, int64(2), arr.Get("length").ToInteger())
	first := arr.Get("0").ToObject(rt)
	second := arr.Get("1").ToObject(rt)
	require.Equal(t, int64(30), first.Get("width").ToInteger())
	require.Equal(t, int64(30), second.Get("width").ToInteger())

	// split — vertical
	res, err = splitFn(goja.Undefined(), rt.ToValue(0.5), rt.ToValue(false))
	require.NoError(t, err)
	arr = res.ToObject(rt)
	first = arr.Get("0").ToObject(rt)
	second = arr.Get("1").ToObject(rt)
	require.Equal(t, int64(10), first.Get("height").ToInteger())
	require.Equal(t, int64(10), second.Get("height").ToInteger())

	// intersect
	intersectFn, _ := goja.AssertFunction(obj.Get("intersect"))
	overlapRect := rt.NewObject()
	_ = overlapRect.Set("x", 30)
	_ = overlapRect.Set("y", 10)
	_ = overlapRect.Set("width", 50)
	_ = overlapRect.Set("height", 20)
	res, err = intersectFn(goja.Undefined(), overlapRect)
	require.NoError(t, err)
	intObj := res.ToObject(rt)
	require.Equal(t, int64(30), intObj.Get("x").ToInteger())
	require.Equal(t, int64(10), intObj.Get("y").ToInteger())

	// union
	unionFn, _ := goja.AssertFunction(obj.Get("union"))
	res, err = unionFn(goja.Undefined(), overlapRect)
	require.NoError(t, err)
	unionObj := res.ToObject(rt)
	require.Equal(t, int64(5), unionObj.Get("x").ToInteger())
	require.Equal(t, int64(10), unionObj.Get("y").ToInteger())

	// asPaneGeometry
	asPGFn, _ := goja.AssertFunction(obj.Get("asPaneGeometry"))
	res, err = asPGFn(goja.Undefined())
	require.NoError(t, err)
	pg := res.ToObject(rt)
	require.Equal(t, int64(10), pg.Get("row").ToInteger())  // y
	require.Equal(t, int64(5), pg.Get("col").ToInteger())   // x
	require.Equal(t, int64(20), pg.Get("rows").ToInteger()) // height
	require.Equal(t, int64(60), pg.Get("cols").ToInteger()) // width

	// toString
	toStringFn, _ := goja.AssertFunction(obj.Get("toString"))
	res, err = toStringFn(goja.Undefined())
	require.NoError(t, err)
	require.Equal(t, "(5,10) 60x20", res.String())
}

func TestCreateLayerObject_GoInterop(t *testing.T) {
	rt := goja.New()

	l := &coordsb.Layer{
		Rect: coordsb.Rect{
			Position: coordsb.Position{X: 1, Y: 2},
			Size:     coordsb.Size{Width: 40, Height: 12},
		},
		Z: 5,
	}
	objVal := createLayerObject(rt, l)
	obj := objVal.ToObject(rt)

	// Getters
	require.Equal(t, int64(1), obj.Get("x").ToInteger())
	require.Equal(t, int64(2), obj.Get("y").ToInteger())
	require.Equal(t, int64(40), obj.Get("width").ToInteger())
	require.Equal(t, int64(12), obj.Get("height").ToInteger())
	require.Equal(t, int64(5), obj.Get("z").ToInteger())

	// Setters
	_ = obj.Set("x", rt.ToValue(10))
	require.Equal(t, 10, l.Rect.Position.X)
	_ = obj.Set("y", rt.ToValue(20))
	require.Equal(t, 20, l.Rect.Position.Y)
	_ = obj.Set("width", rt.ToValue(50))
	require.Equal(t, 50, l.Rect.Size.Width)
	_ = obj.Set("height", rt.ToValue(15))
	require.Equal(t, 15, l.Rect.Size.Height)
	_ = obj.Set("z", rt.ToValue(99))
	require.Equal(t, 99, l.Z)

	// asLayer returns a lipgloss.Layer
	asLayerFn, _ := goja.AssertFunction(obj.Get("asLayer"))
	res, err := asLayerFn(goja.Undefined())
	require.NoError(t, err)
	require.NotNil(t, res)

	// toString
	toStringFn, _ := goja.AssertFunction(obj.Get("toString"))
	res, err = toStringFn(goja.Undefined())
	require.NoError(t, err)
	require.Equal(t, "(10,20) 50x15 z:99", res.String())
}

func TestPaneGeometryRect(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const coord = require('osm:termui/coordinate');
		const r = coord.fromPaneGeometry({row: 2, col: 5, rows: 24, cols: 80});
		if (r.x !== 5) throw new Error('x expected 5 got ' + r.x);
		if (r.y !== 2) throw new Error('y expected 2 got ' + r.y);
		if (r.width !== 80) throw new Error('width expected 80');
		if (r.height !== 24) throw new Error('height expected 24');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestLipglossLayer_PlainObject(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const coord = require('osm:termui/coordinate');
		const l = coord.fromLayer({x: 3, y: 4, width: 50, height: 20, z: 7});
		if (l.x !== 3) throw new Error('x expected 3');
		if (l.y !== 4) throw new Error('y expected 4');
		if (l.width !== 50) throw new Error('width expected 50');
		if (l.height !== 20) throw new Error('height expected 20');
		if (l.z !== 7) throw new Error('z expected 7');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestPosition_NoArgs(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const coord = require('osm:termui/coordinate');
		const p = coord.position();
		if (p.x !== 0) throw new Error('x expected 0');
		if (p.y !== 0) throw new Error('y expected 0');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestSize_NoArgs(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const coord = require('osm:termui/coordinate');
		const s = coord.size();
		if (s.width !== 0) throw new Error('width expected 0');
		if (s.height !== 0) throw new Error('height expected 0');
		if (s.area() !== 0) throw new Error('area expected 0');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestRect_NoArgs(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const coord = require('osm:termui/coordinate');
		const r = coord.rect();
		if (r.x !== 0) throw new Error('x expected 0');
		if (r.y !== 0) throw new Error('y expected 0');
		if (r.width !== 0) throw new Error('width expected 0');
		if (r.height !== 0) throw new Error('height expected 0');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestLayer_NoArgs(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const coord = require('osm:termui/coordinate');
		const l = coord.layer();
		if (l.x !== 0) throw new Error('x expected 0');
		if (l.y !== 0) throw new Error('y expected 0');
		if (l.width !== 0) throw new Error('width expected 0');
		if (l.height !== 0) throw new Error('height expected 0');
		if (l.z !== 0) throw new Error('z expected 0');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestNoArgsReturnUndefined(t *testing.T) {
	rt := goja.New()

	// Position setters return undefined
	p := &coordsb.Position{}
	obj := createPositionObject(rt, p).ToObject(rt)

	// x setter returns undefined
	_ = obj.Set("x", rt.ToValue(5))
	require.Equal(t, 5, p.X)

	// y setter returns undefined
	_ = obj.Set("y", rt.ToValue(10))
	require.Equal(t, 10, p.Y)

	// Size setters return undefined
	s := &coordsb.Size{}
	sObj := createSizeObject(rt, s).ToObject(rt)
	_ = sObj.Set("width", rt.ToValue(80))
	require.Equal(t, 80, s.Width)
	_ = sObj.Set("height", rt.ToValue(24))
	require.Equal(t, 24, s.Height)

	// Rect setters return undefined
	r := &coordsb.Rect{}
	rObj := createRectObject(rt, r).ToObject(rt)
	_ = rObj.Set("x", rt.ToValue(1))
	require.Equal(t, 1, r.Position.X)

	// Layer setters return undefined
	l := &coordsb.Layer{}
	lObj := createLayerObject(rt, l).ToObject(rt)
	_ = lObj.Set("z", rt.ToValue(3))
	require.Equal(t, 3, l.Z)
}

func TestPosition_In_Rect(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const coord = require('osm:termui/coordinate');
		const r = coord.rect({x: 5, y: 5, width: 10, height: 10});
		const inside = coord.position({x: 8, y: 8});
		const outside = coord.position({x: 20, y: 20});
		const edgeTopLeft = coord.position({x: 5, y: 5});
		const edgeBottomRight = coord.position({x: 15, y: 15});
		if (!inside.in(r)) throw new Error('inside should be in rect');
		if (outside.in(r)) throw new Error('outside should not be in rect');
		if (!edgeTopLeft.in(r)) throw new Error('top-left edge should be in rect');
		if (edgeBottomRight.in(r)) throw new Error('bottom-right edge should not be in rect (exclusive)');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestSize_Contains(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const coord = require('osm:termui/coordinate');
		const big = coord.size({width: 100, height: 50});
		const small = coord.size({width: 40, height: 20});
		const wider = coord.size({width: 200, height: 20});
		if (!big.contains(small)) throw new Error('big should contain small');
		if (small.contains(big)) throw new Error('small should not contain big');
		if (big.contains(wider)) throw new Error('big should not contain wider');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestRect_Split_Defaults(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const coord = require('osm:termui/coordinate');
		const r = coord.rect({x: 0, y: 0, width: 100, height: 50});
		// split with no args defaults to ratio=0.5, horizontal=false (vertical)
		const parts = r.split();
		if (parts.length !== 2) throw new Error('expected 2 parts');
		if (parts[0].height !== 25) throw new Error('first height expected 25 got ' + parts[0].height);
		if (parts[1].height !== 25) throw new Error('second height expected 25 got ' + parts[1].height);
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestRect_Intersect_NoOverlap(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const coord = require('osm:termui/coordinate');
		const a = coord.rect({x: 0, y: 0, width: 10, height: 10});
		const b = coord.rect({x: 20, y: 20, width: 10, height: 10});
		const i = a.intersect(b);
		if (i.width !== 0 || i.height !== 0) throw new Error('no overlap should yield zero rect');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestRect_Union_OneEmpty(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const coord = require('osm:termui/coordinate');
		const a = coord.rect({x: 5, y: 5, width: 10, height: 10});
		const empty = coord.rect({x: 0, y: 0, width: 0, height: 0});
		const u = a.union(empty);
		if (u.x !== 5) throw new Error('union with empty should equal a');
		if (u.width !== 10) throw new Error('union width expected 10');
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestPaneGeometryRect_Error_NoArgs(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const coord = require('osm:termui/coordinate');
		try {
			coord.fromPaneGeometry();
			throw new Error('should have thrown');
		} catch (e) {
			if (!e.message.includes('fromPaneGeometry requires')) throw new Error('wrong error: ' + e.message);
		}
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}

func TestLipglossLayer_Error_NoArgs(t *testing.T) {
	rt := setupRuntime(t)

	script := `
		const coord = require('osm:termui/coordinate');
		try {
			coord.fromLayer();
			throw new Error('should have thrown');
		} catch (e) {
			if (!e.message.includes('fromLayer requires')) throw new Error('wrong error: ' + e.message);
		}
		'ok';
	`

	res, err := rt.RunString(script)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Export())
}
