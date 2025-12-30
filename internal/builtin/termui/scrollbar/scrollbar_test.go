package scrollbar

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/dop251/goja"
	termuisb "github.com/joeycumines/one-shot-man/internal/termui/scrollbar"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"
)

func setupRuntime(t *testing.T) *goja.Runtime {
	rt := goja.New()
	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).String()
		switch arg {
		case "osm:termui/scrollbar":
			mod := rt.NewObject()
			Require()(rt, mod)
			return mod.Get("exports")
		}
		return goja.Undefined()
	})
	return rt
}

func TestRequire_ExportsNew(t *testing.T) {
	rt := goja.New()
	module := rt.NewObject()
	require.NoError(t, module.Set("exports", rt.NewObject()))
	Require()(rt, module)

	exports := module.Get("exports").ToObject(rt)
	require.NotNil(t, exports)

	val := exports.Get("new")
	require.False(t, goja.IsUndefined(val))
	_, ok := goja.AssertFunction(val)
	require.True(t, ok)
}

func TestJS_API_Surface(t *testing.T) {
	rt := setupRuntime(t)
	orig := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(orig) })
	lipgloss.SetColorProfile(termenv.TrueColor)

	script := `
		const sb = require('osm:termui/scrollbar').new(8);
		sb.setContentHeight(20);
		sb.setYOffset(2);
		sb.setChars('T', '.');
		const v = sb.view();
		const rows = v.split('\n');
		if (rows.length !== 8) throw new Error('expected 8 rows');
		let foundT = false, foundDot = false;
		for (const r of rows) { if (r.indexOf('T') !== -1) foundT = true; if (r.indexOf('.') !== -1) foundDot = true; }
		if (!foundT || !foundDot) throw new Error('missing expected chars');
	`

	_, err := rt.RunString(script)
	require.NoError(t, err)
}

func TestCreateScrollbarObject_GoInterop(t *testing.T) {
	rt := goja.New()
	orig := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(orig) })
	lipgloss.SetColorProfile(termenv.TrueColor)

	m := termuisb.New()
	objVal := createScrollbarObject(rt, &m)
	obj := objVal.ToObject(rt)

	setVp, _ := goja.AssertFunction(obj.Get("setViewportHeight"))
	_, err := setVp(goja.Undefined(), rt.ToValue(6))
	require.NoError(t, err)
	require.Equal(t, 6, m.ViewportHeight)
	// Negative values are clamped to 0 by the JS binding
	_, err = setVp(goja.Undefined(), rt.ToValue(-1))
	require.NoError(t, err)
	require.Equal(t, 0, m.ViewportHeight)
	setChars, _ := goja.AssertFunction(obj.Get("setChars"))
	_, err = setChars(goja.Undefined(), rt.ToValue("X"), rt.ToValue("-"))
	require.NoError(t, err)
	require.Equal(t, "X", m.ThumbChar)
	require.Equal(t, "-", m.TrackChar)

	// Getters: contentHeight and yOffset should reflect underlying model
	contentFn, _ := goja.AssertFunction(obj.Get("contentHeight"))
	resContent, err := contentFn(goja.Undefined())
	require.NoError(t, err)
	require.Equal(t, int64(m.ContentHeight), resContent.ToInteger())

	yFn, _ := goja.AssertFunction(obj.Get("yOffset"))
	resY, err := yFn(goja.Undefined())
	require.NoError(t, err)
	require.Equal(t, int64(m.YOffset), resY.ToInteger())

	setThumbBg, _ := goja.AssertFunction(obj.Get("setThumbBackground"))
	_, err = setThumbBg(goja.Undefined(), rt.ToValue("#FF0000"))
	require.NoError(t, err)

	m.ViewportHeight = 4
	m.ContentHeight = 20
	m.YOffset = 0

	viewFn, _ := goja.AssertFunction(obj.Get("view"))
	v, err := viewFn(goja.Undefined())
	require.NoError(t, err)
	out := v.String()

	require.Contains(t, out, "48;2;255;0;0")
	lines := strings.Split(out, "\n")
	require.Equal(t, 4, len(lines))
}

func TestNoArgsReturnUndefined(t *testing.T) {
	rt := goja.New()
	m := termuisb.New()
	obj := createScrollbarObject(rt, &m).ToObject(rt)

	setVp, _ := goja.AssertFunction(obj.Get("setViewportHeight"))
	res, err := setVp(goja.Undefined())
	require.NoError(t, err)
	require.True(t, goja.IsUndefined(res))

	setChars, _ := goja.AssertFunction(obj.Get("setChars"))
	res, err = setChars(goja.Undefined())
	require.NoError(t, err)
	require.True(t, goja.IsUndefined(res))
}
