package bubblezone

import (
	"runtime"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadModule creates a goja runtime with the bubblezone module loaded and
// a live (open) Manager. The manager is closed on test cleanup.
func loadModule(t *testing.T) (*goja.Runtime, *Manager) {
	t.Helper()
	mgr := NewManager()
	t.Cleanup(mgr.Close)
	rt := goja.New()
	module := rt.NewObject()
	Require(mgr)(rt, module)
	_ = rt.Set("zone", module.Get("exports"))
	return rt, mgr
}

// loadModuleClosed creates a runtime with a closed (nil zone) manager
// so that all operations fall into the degraded/fallback paths.
func loadModuleClosed(t *testing.T) *goja.Runtime {
	t.Helper()
	mgr := NewManager()
	mgr.Close() // zone is now nil
	rt := goja.New()
	module := rt.NewObject()
	Require(mgr)(rt, module)
	_ = rt.Set("zone", module.Get("exports"))
	return rt
}

// waitForZone waits for the bubblezone worker goroutine to process scanned
// zones. Scan() is async (sends via channel), so Get() may not immediately
// return zone info. This is NOT timing-dependent: it yields to the goroutine
// scheduler until the bounded retry limit.
func waitForZone(t *testing.T, mgr *Manager, id string) {
	t.Helper()
	for i := 0; i < 1000; i++ {
		mgr.mu.RLock()
		z := mgr.zone
		mgr.mu.RUnlock()
		if z == nil {
			t.Fatal("manager zone is nil")
		}
		if info := z.Get(id); info != nil && !info.IsZero() {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("zone %q not available after Scan", id)
}

// --- NewManager ---

func TestNewManager(t *testing.T) {
	m := NewManager()
	assert.NotNil(t, m)
	assert.NotNil(t, m.zone)
	m.Close()
}

// --- Close ---

func TestClose(t *testing.T) {
	m := NewManager()
	assert.NotNil(t, m.zone)
	m.Close()
	assert.Nil(t, m.zone)
}

func TestClose_DoubleClose(t *testing.T) {
	m := NewManager()
	m.Close()
	m.Close() // must not panic
	assert.Nil(t, m.zone)
}

// --- mark ---

func TestMark_NoArgs(t *testing.T) {
	rt, _ := loadModule(t)
	v, err := rt.RunString("zone.mark()")
	require.NoError(t, err)
	assert.Equal(t, "", v.Export())
}

func TestMark_OneArg(t *testing.T) {
	rt, _ := loadModule(t)
	v, err := rt.RunString("zone.mark('id')")
	require.NoError(t, err)
	assert.Equal(t, "", v.Export())
}

func TestMark_NilZone(t *testing.T) {
	rt := loadModuleClosed(t)
	v, err := rt.RunString("zone.mark('id', 'content')")
	require.NoError(t, err)
	assert.Equal(t, "content", v.Export())
}

func TestMark_Normal(t *testing.T) {
	rt, _ := loadModule(t)
	v, err := rt.RunString("zone.mark('btn', 'Click')")
	require.NoError(t, err)
	result := v.String()
	assert.NotEqual(t, "Click", result, "marked content should differ from raw")
	assert.Contains(t, result, "Click", "marked content should contain original text")
}

// --- scan ---

func TestScan_NoArgs(t *testing.T) {
	rt, _ := loadModule(t)
	v, err := rt.RunString("zone.scan()")
	require.NoError(t, err)
	assert.Equal(t, "", v.Export())
}

func TestScan_NilZone(t *testing.T) {
	rt := loadModuleClosed(t)
	v, err := rt.RunString("zone.scan('content')")
	require.NoError(t, err)
	assert.Equal(t, "content", v.Export())
}

func TestScan_Normal(t *testing.T) {
	rt, _ := loadModule(t)
	v, err := rt.RunString("var marked = zone.mark('btn', 'Click'); zone.scan(marked)")
	require.NoError(t, err)
	assert.Equal(t, "Click", v.String())
}

func TestScan_PlainText(t *testing.T) {
	rt, _ := loadModule(t)
	v, err := rt.RunString("zone.scan('hello world')")
	require.NoError(t, err)
	assert.Equal(t, "hello world", v.String())
}

// --- inBounds ---

func TestInBounds_NoArgs(t *testing.T) {
	rt, _ := loadModule(t)
	v, err := rt.RunString("zone.inBounds()")
	require.NoError(t, err)
	assert.Equal(t, false, v.Export())
}

func TestInBounds_OneArg(t *testing.T) {
	rt, _ := loadModule(t)
	v, err := rt.RunString("zone.inBounds('id')")
	require.NoError(t, err)
	assert.Equal(t, false, v.Export())
}

func TestInBounds_NullMsg(t *testing.T) {
	rt, _ := loadModule(t)
	v, err := rt.RunString("zone.inBounds('id', null)")
	require.NoError(t, err)
	assert.Equal(t, false, v.Export(), "null msg should return false, not throw")
}

func TestInBounds_UndefinedMsg(t *testing.T) {
	rt, _ := loadModule(t)
	v, err := rt.RunString("zone.inBounds('id', undefined)")
	require.NoError(t, err)
	assert.Equal(t, false, v.Export(), "undefined msg should return false, not throw")
}

func TestInBounds_EmptyObject(t *testing.T) {
	rt, _ := loadModule(t)
	v, err := rt.RunString("zone.inBounds('id', {})")
	require.NoError(t, err)
	assert.Equal(t, false, v.Export(), "object without x/y should return false")
}

func TestInBounds_MissingY(t *testing.T) {
	rt, _ := loadModule(t)
	v, err := rt.RunString("zone.inBounds('id', {x: 0})")
	require.NoError(t, err)
	assert.Equal(t, false, v.Export())
}

func TestInBounds_MissingX(t *testing.T) {
	rt, _ := loadModule(t)
	v, err := rt.RunString("zone.inBounds('id', {y: 0})")
	require.NoError(t, err)
	assert.Equal(t, false, v.Export())
}

func TestInBounds_NilZone(t *testing.T) {
	rt := loadModuleClosed(t)
	v, err := rt.RunString("zone.inBounds('id', {x: 0, y: 0})")
	require.NoError(t, err)
	assert.Equal(t, false, v.Export())
}

func TestInBounds_UnknownZone(t *testing.T) {
	rt, _ := loadModule(t)
	v, err := rt.RunString("zone.inBounds('nonexistent', {x: 0, y: 0})")
	require.NoError(t, err)
	assert.Equal(t, false, v.Export())
}

func TestInBounds_NumberMsg(t *testing.T) {
	rt, _ := loadModule(t)
	v, err := rt.RunString("zone.inBounds('id', 42)")
	require.NoError(t, err)
	assert.Equal(t, false, v.Export())
}

func TestInBounds_ValidZone_InBounds(t *testing.T) {
	rt, mgr := loadModule(t)
	_, err := rt.RunString("var m1 = zone.mark('btn', 'Click'); zone.scan(m1)")
	require.NoError(t, err)
	waitForZone(t, mgr, "btn")
	v, err := rt.RunString("zone.inBounds('btn', {x: 0, y: 0})")
	require.NoError(t, err)
	assert.Equal(t, true, v.Export())
}

func TestInBounds_ValidZone_OutOfBounds(t *testing.T) {
	rt, mgr := loadModule(t)
	_, err := rt.RunString("var m2 = zone.mark('btn2', 'Click'); zone.scan(m2)")
	require.NoError(t, err)
	waitForZone(t, mgr, "btn2")
	v, err := rt.RunString("zone.inBounds('btn2', {x: 999, y: 999})")
	require.NoError(t, err)
	assert.Equal(t, false, v.Export())
}

// --- get ---

func TestGet_NoArgs(t *testing.T) {
	rt, _ := loadModule(t)
	v, err := rt.RunString("zone.get()")
	require.NoError(t, err)
	assert.True(t, goja.IsNull(v), "get() with no args should return null")
}

func TestGet_NilZone(t *testing.T) {
	rt := loadModuleClosed(t)
	v, err := rt.RunString("zone.get('id')")
	require.NoError(t, err)
	assert.True(t, goja.IsNull(v))
}

func TestGet_UnknownZone(t *testing.T) {
	rt, _ := loadModule(t)
	v, err := rt.RunString("zone.get('nonexistent')")
	require.NoError(t, err)
	assert.True(t, goja.IsNull(v))
}

func TestGet_ValidZone_Properties(t *testing.T) {
	rt, mgr := loadModule(t)
	_, err := rt.RunString("var gm = zone.mark('gbtn', 'ClickMe'); zone.scan(gm)")
	require.NoError(t, err)
	waitForZone(t, mgr, "gbtn")
	_, err = rt.RunString("var info = zone.get('gbtn')")
	require.NoError(t, err)

	v, err := rt.RunString("info !== null")
	require.NoError(t, err)
	assert.Equal(t, true, v.Export())

	for _, prop := range []string{"startX", "startY", "endX", "endY", "width", "height"} {
		v, err := rt.RunString("typeof info['" + prop + "']")
		require.NoError(t, err, "property %s", prop)
		assert.Equal(t, "number", v.String(), "property %s should be number", prop)
	}
}

func TestGet_ValidZone_WidthHeight(t *testing.T) {
	rt, mgr := loadModule(t)
	_, err := rt.RunString("var wm = zone.mark('wbtn', 'ABCD'); zone.scan(wm)")
	require.NoError(t, err)
	waitForZone(t, mgr, "wbtn")
	_, err = rt.RunString("var winfo = zone.get('wbtn')")
	require.NoError(t, err)

	v, err := rt.RunString("winfo.width === winfo.endX - winfo.startX")
	require.NoError(t, err)
	assert.Equal(t, true, v.Export(), "width should equal endX - startX")

	v, err = rt.RunString("winfo.height === winfo.endY - winfo.startY")
	require.NoError(t, err)
	assert.Equal(t, true, v.Export(), "height should equal endY - startY")
}

func TestGet_ValidZone_JSONRoundtrip(t *testing.T) {
	rt, mgr := loadModule(t)
	_, err := rt.RunString("var jm = zone.mark('jbtn', 'OK'); zone.scan(jm)")
	require.NoError(t, err)
	waitForZone(t, mgr, "jbtn")
	v, err := rt.RunString("JSON.stringify(zone.get('jbtn'))")
	require.NoError(t, err)
	result := v.String()
	for _, key := range []string{"startX", "startY", "endX", "endY", "width", "height"} {
		assert.Contains(t, result, key)
	}
}

// --- newPrefix ---

func TestNewPrefix_Normal(t *testing.T) {
	rt, _ := loadModule(t)
	v, err := rt.RunString("zone.newPrefix()")
	require.NoError(t, err)
	prefix := v.String()
	assert.NotEmpty(t, prefix)
}

func TestNewPrefix_Unique(t *testing.T) {
	rt, _ := loadModule(t)
	v, err := rt.RunString("var np1 = zone.newPrefix(); var np2 = zone.newPrefix(); np1 !== np2")
	require.NoError(t, err)
	assert.Equal(t, true, v.Export(), "consecutive prefixes should differ")
}

func TestNewPrefix_NilZone(t *testing.T) {
	rt := loadModuleClosed(t)
	v, err := rt.RunString("zone.newPrefix()")
	require.NoError(t, err)
	prefix := v.String()
	assert.NotEmpty(t, prefix)
	assert.Contains(t, prefix, "_")
	assert.Len(t, prefix, 2, "fallback prefix should be letter + underscore")
}

func TestNewPrefix_NilZone_Unique(t *testing.T) {
	rt := loadModuleClosed(t)
	v, err := rt.RunString("var fp1 = zone.newPrefix(); var fp2 = zone.newPrefix(); fp1 !== fp2")
	require.NoError(t, err)
	assert.Equal(t, true, v.Export(), "fallback prefixes should be unique")
}

// --- close (JS-callable) ---

func TestClose_FromJS(t *testing.T) {
	mgr := NewManager()
	rt := goja.New()
	module := rt.NewObject()
	Require(mgr)(rt, module)
	_ = rt.Set("zone", module.Get("exports"))

	v, err := rt.RunString("zone.close()")
	require.NoError(t, err)
	assert.True(t, goja.IsUndefined(v))
	assert.Nil(t, mgr.zone)
}

func TestClose_FromJS_ThenMarkDegraded(t *testing.T) {
	mgr := NewManager()
	rt := goja.New()
	module := rt.NewObject()
	Require(mgr)(rt, module)
	_ = rt.Set("zone", module.Get("exports"))

	v, err := rt.RunString("zone.close(); zone.mark('id', 'hello')")
	require.NoError(t, err)
	assert.Equal(t, "hello", v.String())
}

func TestClose_FromJS_ThenScanDegraded(t *testing.T) {
	mgr := NewManager()
	rt := goja.New()
	module := rt.NewObject()
	Require(mgr)(rt, module)
	_ = rt.Set("zone", module.Get("exports"))

	v, err := rt.RunString("zone.close(); zone.scan('text')")
	require.NoError(t, err)
	assert.Equal(t, "text", v.String())
}

func TestClose_FromJS_ThenGetDegraded(t *testing.T) {
	mgr := NewManager()
	rt := goja.New()
	module := rt.NewObject()
	Require(mgr)(rt, module)
	_ = rt.Set("zone", module.Get("exports"))

	v, err := rt.RunString("zone.close(); zone.get('id')")
	require.NoError(t, err)
	assert.True(t, goja.IsNull(v))
}

func TestClose_FromJS_ThenInBoundsDegraded(t *testing.T) {
	mgr := NewManager()
	rt := goja.New()
	module := rt.NewObject()
	Require(mgr)(rt, module)
	_ = rt.Set("zone", module.Get("exports"))

	v, err := rt.RunString("zone.close(); zone.inBounds('id', {x: 0, y: 0})")
	require.NoError(t, err)
	assert.Equal(t, false, v.Export())
}

func TestClose_FromJS_DoubleClose(t *testing.T) {
	mgr := NewManager()
	rt := goja.New()
	module := rt.NewObject()
	Require(mgr)(rt, module)
	_ = rt.Set("zone", module.Get("exports"))

	v, err := rt.RunString("zone.close(); zone.close()")
	require.NoError(t, err)
	assert.True(t, goja.IsUndefined(v))
}

// --- Integration ---

func TestIntegration_FullFlow(t *testing.T) {
	rt, mgr := loadModule(t)
	_, err := rt.RunString("var iview = zone.mark('ibtn', '[ OK ]'); var ioutput = zone.scan(iview)")
	require.NoError(t, err)

	v, err := rt.RunString("ioutput")
	require.NoError(t, err)
	assert.Equal(t, "[ OK ]", v.String())

	waitForZone(t, mgr, "ibtn")

	v, err = rt.RunString("zone.get('ibtn') !== null")
	require.NoError(t, err)
	assert.Equal(t, true, v.Export())

	v, err = rt.RunString("zone.inBounds('ibtn', {x: 0, y: 0})")
	require.NoError(t, err)
	assert.Equal(t, true, v.Export())

	v, err = rt.RunString("zone.inBounds('ibtn', {x: 999, y: 999})")
	require.NoError(t, err)
	assert.Equal(t, false, v.Export())
}

func TestIntegration_MultipleZones(t *testing.T) {
	rt, mgr := loadModule(t)
	_, err := rt.RunString("var mview = zone.mark('za', 'AAA') + '\\n' + zone.mark('zb', 'BBB'); zone.scan(mview)")
	require.NoError(t, err)

	waitForZone(t, mgr, "za")
	waitForZone(t, mgr, "zb")

	for _, id := range []string{"za", "zb"} {
		v, err := rt.RunString("zone.get('" + id + "') !== null")
		require.NoError(t, err, "zone %s", id)
		assert.Equal(t, true, v.Export(), "zone %s should exist", id)
	}
}

func TestIntegration_RescanUpdatesZones(t *testing.T) {
	rt, mgr := loadModule(t)
	_, err := rt.RunString("var rv1 = zone.mark('rz', 'SHORT'); zone.scan(rv1)")
	require.NoError(t, err)
	waitForZone(t, mgr, "rz")

	// Capture first zone state at Go level for deterministic comparison
	info1 := mgr.zone.Get("rz")
	require.NotNil(t, info1)
	endX1 := info1.EndX

	_, err = rt.RunString("var rv2 = zone.mark('rz', 'MUCH LONGER CONTENT HERE'); zone.scan(rv2)")
	require.NoError(t, err)

	// Wait for zone info to actually change (not just exist)
	for i := 0; i < 1000; i++ {
		if info := mgr.zone.Get("rz"); info != nil && info.EndX != endX1 {
			return // zone updated — pass
		}
		runtime.Gosched()
	}
	t.Fatal("zone 'rz' did not update after rescan")
}

func TestIntegration_PrefixedZoneIDs(t *testing.T) {
	rt, mgr := loadModule(t)
	_, err := rt.RunString("var pp = zone.newPrefix(); var pid = pp + 'item'; var pv = zone.mark(pid, 'Hello'); zone.scan(pv)")
	require.NoError(t, err)

	// Get the prefix-based zone ID from JS so we can wait for it in Go
	pidVal, err := rt.RunString("pid")
	require.NoError(t, err)
	waitForZone(t, mgr, pidVal.String())

	v, err := rt.RunString("zone.get(pid) !== null")
	require.NoError(t, err)
	assert.Equal(t, true, v.Export())
}

// --- Require module structure ---

func TestRequire_ExportsAllFunctions(t *testing.T) {
	mgr := NewManager()
	t.Cleanup(mgr.Close)
	rt := goja.New()
	module := rt.NewObject()
	Require(mgr)(rt, module)
	_ = rt.Set("zexports", module.Get("exports"))

	expected := []string{"mark", "scan", "inBounds", "get", "newPrefix", "close"}
	for _, name := range expected {
		v, err := rt.RunString("typeof zexports['" + name + "']")
		require.NoError(t, err)
		assert.Equal(t, "function", v.String(), "%s should be a function", name)
	}
}


