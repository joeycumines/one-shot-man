package viewport

import (
	"testing"

	"github.com/dop251/goja"
)

// =============================================================================
// Helper
// =============================================================================

func newTestViewport(t *testing.T, width, height int) (*goja.Runtime, goja.Value) {
	t.Helper()
	rt := goja.New()
	module := rt.NewObject()
	Require()(rt, module)
	exports := module.Get("exports").ToObject(rt)
	newFn, ok := goja.AssertFunction(exports.Get("new"))
	if !ok {
		t.Fatal("new is not a function")
	}
	args := []goja.Value{}
	if width > 0 || height > 0 {
		args = append(args, rt.ToValue(width), rt.ToValue(height))
	}
	result, err := newFn(goja.Undefined(), args...)
	if err != nil {
		t.Fatalf("new() failed: %v", err)
	}
	return rt, result
}

func newTestViewportNoArgs(t *testing.T) (*goja.Runtime, goja.Value) {
	t.Helper()
	rt := goja.New()
	module := rt.NewObject()
	Require()(rt, module)
	exports := module.Get("exports").ToObject(rt)
	newFn, ok := goja.AssertFunction(exports.Get("new"))
	if !ok {
		t.Fatal("new is not a function")
	}
	// No arguments — should use default 80x24
	result, err := newFn(goja.Undefined())
	if err != nil {
		t.Fatalf("new() failed: %v", err)
	}
	return rt, result
}

func vpCallMethod(t *testing.T, rt *goja.Runtime, obj goja.Value, method string, args ...goja.Value) goja.Value {
	t.Helper()
	fn, ok := goja.AssertFunction(obj.ToObject(rt).Get(method))
	if !ok {
		t.Fatalf("%s is not a function", method)
	}
	result, err := fn(obj, args...)
	if err != nil {
		t.Fatalf("%s() failed: %v", method, err)
	}
	return result
}

// =============================================================================
// Require: default width/height when no args
// =============================================================================

func TestRequire_DefaultDimensions(t *testing.T) {
	rt, vp := newTestViewportNoArgs(t)
	w := vpCallMethod(t, rt, vp, "width")
	h := vpCallMethod(t, rt, vp, "height")
	if w.ToInteger() != 80 {
		t.Errorf("expected default width 80, got %d", w.ToInteger())
	}
	if h.ToInteger() != 24 {
		t.Errorf("expected default height 24, got %d", h.ToInteger())
	}
}

func TestRequire_WidthOnlyArg(t *testing.T) {
	rt := goja.New()
	module := rt.NewObject()
	Require()(rt, module)
	exports := module.Get("exports").ToObject(rt)
	newFn, ok := goja.AssertFunction(exports.Get("new"))
	if !ok {
		t.Fatal("new is not a function")
	}
	// Only width provided
	result, err := newFn(goja.Undefined(), rt.ToValue(50))
	if err != nil {
		t.Fatalf("new() failed: %v", err)
	}
	obj := result.ToObject(rt)
	wFn, _ := goja.AssertFunction(obj.Get("width"))
	w, _ := wFn(result)
	if w.ToInteger() != 50 {
		t.Errorf("expected width 50, got %d", w.ToInteger())
	}
	hFn, _ := goja.AssertFunction(obj.Get("height"))
	h, _ := hFn(result)
	if h.ToInteger() != 24 {
		t.Errorf("expected default height 24, got %d", h.ToInteger())
	}
}

// =============================================================================
// _type property
// =============================================================================

func TestViewport_TypeProperty(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	typeVal := vp.ToObject(rt).Get("_type")
	if typeVal.String() != "bubbles/viewport" {
		t.Errorf("expected 'bubbles/viewport', got %q", typeVal.String())
	}
}

// =============================================================================
// setContent with no args
// =============================================================================

func TestSetContent_NoArgs(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	fn, ok := goja.AssertFunction(vp.ToObject(rt).Get("setContent"))
	if !ok {
		t.Fatal("setContent is not a function")
	}
	result, err := fn(vp) // No args
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if !goja.IsUndefined(result) {
		t.Error("setContent with no args should return undefined")
	}
}

// =============================================================================
// setWidth / setHeight with no args
// =============================================================================

func TestSetWidth_NoArgs(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	fn, ok := goja.AssertFunction(vp.ToObject(rt).Get("setWidth"))
	if !ok {
		t.Fatal("setWidth is not a function")
	}
	result, err := fn(vp)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if !goja.IsUndefined(result) {
		t.Error("setWidth with no args should return undefined")
	}
}

func TestSetHeight_NoArgs(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	fn, ok := goja.AssertFunction(vp.ToObject(rt).Get("setHeight"))
	if !ok {
		t.Fatal("setHeight is not a function")
	}
	result, err := fn(vp)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if !goja.IsUndefined(result) {
		t.Error("setHeight with no args should return undefined")
	}
}

// =============================================================================
// setStyle with undefined (not null)
// =============================================================================

func TestSetStyle_Undefined(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	result := vpCallMethod(t, rt, vp, "setStyle", goja.Undefined())
	if result.ToObject(rt) != vp.ToObject(rt) {
		t.Error("should return obj for chaining")
	}
}

func TestSetStyle_NoArgs(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	fn, ok := goja.AssertFunction(vp.ToObject(rt).Get("setStyle"))
	if !ok {
		t.Fatal("setStyle is not a function")
	}
	result, err := fn(vp)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if !goja.IsUndefined(result) {
		t.Error("setStyle with no args should return undefined")
	}
}

func TestSetStyle_InvalidStyleObject(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	// Pass a plain string — UnwrapStyle returns error, code panics with GoError.
	// Goja catches this and returns it as fn() error, not a Go panic.
	fn, ok := goja.AssertFunction(vp.ToObject(rt).Get("setStyle"))
	if !ok {
		t.Fatal("setStyle is not a function")
	}
	_, err := fn(vp, rt.ToValue("notAStyle"))
	if err == nil {
		t.Error("expected error from setStyle with invalid object")
	}
}

// =============================================================================
// scrollDown / scrollUp with default (no args)
// =============================================================================

func TestScrollDown_DefaultNoArgs(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A\nB\nC\nD\nE\nF\nG\nH"))
	result := vpCallMethod(t, rt, vp, "scrollDown")
	if result.ToObject(rt) != vp.ToObject(rt) {
		t.Error("should return obj for chaining")
	}
	y := vpCallMethod(t, rt, vp, "yOffset")
	if y.ToInteger() != 1 {
		t.Errorf("expected yOffset 1 after scrollDown(1 default), got %d", y.ToInteger())
	}
}

func TestScrollUp_DefaultNoArgs(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A\nB\nC\nD\nE\nF\nG\nH"))
	// Scroll down first
	vpCallMethod(t, rt, vp, "scrollDown", rt.ToValue(3))
	result := vpCallMethod(t, rt, vp, "scrollUp")
	if result.ToObject(rt) != vp.ToObject(rt) {
		t.Error("should return obj for chaining")
	}
	y := vpCallMethod(t, rt, vp, "yOffset")
	if y.ToInteger() != 2 {
		t.Errorf("expected yOffset 2 after scrollUp(1 default), got %d", y.ToInteger())
	}
}

// =============================================================================
// scrollLeft / scrollRight with default (no args)
// =============================================================================

func TestScrollLeft_DefaultNoArgs(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A very long line that extends beyond viewport width"))
	vpCallMethod(t, rt, vp, "scrollRight", rt.ToValue(5))
	result := vpCallMethod(t, rt, vp, "scrollLeft")
	if result.ToObject(rt) != vp.ToObject(rt) {
		t.Error("should return obj for chaining")
	}
}

func TestScrollRight_DefaultNoArgs(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A very long line that extends beyond viewport width"))
	result := vpCallMethod(t, rt, vp, "scrollRight")
	if result.ToObject(rt) != vp.ToObject(rt) {
		t.Error("should return obj for chaining")
	}
}

// =============================================================================
// setXOffset / setYOffset with no args
// =============================================================================

func TestSetXOffset_NoArgs(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	fn, ok := goja.AssertFunction(vp.ToObject(rt).Get("setXOffset"))
	if !ok {
		t.Fatal("not a function")
	}
	result, err := fn(vp)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if !goja.IsUndefined(result) {
		t.Error("setXOffset with no args should return undefined")
	}
}

func TestSetYOffset_NoArgs(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	fn, ok := goja.AssertFunction(vp.ToObject(rt).Get("setYOffset"))
	if !ok {
		t.Fatal("not a function")
	}
	result, err := fn(vp)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if !goja.IsUndefined(result) {
		t.Error("setYOffset with no args should return undefined")
	}
}

// =============================================================================
// setHorizontalStep with no args
// =============================================================================

func TestSetHorizontalStep_NoArgs(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	fn, ok := goja.AssertFunction(vp.ToObject(rt).Get("setHorizontalStep"))
	if !ok {
		t.Fatal("not a function")
	}
	result, err := fn(vp)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if !goja.IsUndefined(result) {
		t.Error("setHorizontalStep with no args should return undefined")
	}
}

// =============================================================================
// setMouseWheelEnabled with no args (defaults to true)
// =============================================================================

func TestSetMouseWheelEnabled_NoArgs(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	// First disable it
	vpCallMethod(t, rt, vp, "setMouseWheelEnabled", rt.ToValue(false))
	enabled := vpCallMethod(t, rt, vp, "isMouseWheelEnabled")
	if enabled.ToBoolean() {
		t.Error("expected disabled")
	}

	// Now call with no args — should default to true
	fn, ok := goja.AssertFunction(vp.ToObject(rt).Get("setMouseWheelEnabled"))
	if !ok {
		t.Fatal("not a function")
	}
	_, err := fn(vp) // No args
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	enabled = vpCallMethod(t, rt, vp, "isMouseWheelEnabled")
	if !enabled.ToBoolean() {
		t.Error("expected true (default) when no args provided")
	}
}

// =============================================================================
// setKeyMapEnabled
// =============================================================================

func TestSetKeyMapEnabled_Disable(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	result := vpCallMethod(t, rt, vp, "setKeyMapEnabled", rt.ToValue(false))
	if result.ToObject(rt) != vp.ToObject(rt) {
		t.Error("should return obj for chaining")
	}
}

func TestSetKeyMapEnabled_Enable(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	// First disable
	vpCallMethod(t, rt, vp, "setKeyMapEnabled", rt.ToValue(false))
	// Then re-enable
	result := vpCallMethod(t, rt, vp, "setKeyMapEnabled", rt.ToValue(true))
	if result.ToObject(rt) != vp.ToObject(rt) {
		t.Error("should return obj for chaining")
	}
}

func TestSetKeyMapEnabled_NoArgs(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	fn, ok := goja.AssertFunction(vp.ToObject(rt).Get("setKeyMapEnabled"))
	if !ok {
		t.Fatal("not a function")
	}
	result, err := fn(vp)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if !goja.IsUndefined(result) {
		t.Error("setKeyMapEnabled with no args should return undefined")
	}
}

// =============================================================================
// pastBottom
// =============================================================================

func TestPastBottom(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	// No content — viewport is empty, so not past bottom
	result := vpCallMethod(t, rt, vp, "pastBottom")
	if result.ToBoolean() {
		t.Error("expected false for empty viewport")
	}

	// Add enough content and scroll to bottom
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A\nB\nC\nD\nE\nF\nG\nH\nI\nJ"))
	vpCallMethod(t, rt, vp, "gotoBottom")
	result = vpCallMethod(t, rt, vp, "pastBottom")
	// At bottom but not PAST bottom
	if result.ToBoolean() {
		t.Error("expected false when exactly at bottom")
	}
}

// =============================================================================
// horizontalScrollPercent
// =============================================================================

func TestHorizontalScrollPercent(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A very long line that extends well beyond the viewport width for testing"))
	result := vpCallMethod(t, rt, vp, "horizontalScrollPercent")
	pct := result.ToFloat()
	if pct < 0 || pct > 1 {
		t.Errorf("expected 0-1, got %f", pct)
	}
}

func TestHorizontalScrollPercent_NoContent(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	result := vpCallMethod(t, rt, vp, "horizontalScrollPercent")
	pct := result.ToFloat()
	if pct < 0 || pct > 1 {
		t.Errorf("expected 0-1, got %f", pct)
	}
}

// =============================================================================
// update edge cases
// =============================================================================

func TestUpdate_NoArgs(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	fn, ok := goja.AssertFunction(vp.ToObject(rt).Get("update"))
	if !ok {
		t.Fatal("update is not a function")
	}
	result, err := fn(vp) // No args
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	arr := result.ToObject(rt)
	if arr.Get("0").ToObject(rt) != vp.ToObject(rt) {
		t.Error("result[0] should be viewport object")
	}
	if !goja.IsNull(arr.Get("1")) {
		t.Error("result[1] should be null with no args")
	}
}

func TestUpdate_NullMsgObj(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	// goja.Null().ToObject(runtime) throws JS TypeError;
	// this is returned as err, not a Go panic.
	// The source code checks `if msgObj == nil` but goja throws before that check
	// Note: this tests the goja exception path which is different from
	// passing undefined (which returns Go nil from ToObject).
	fn, ok := goja.AssertFunction(vp.ToObject(rt).Get("update"))
	if !ok {
		t.Fatal("update is not a function")
	}
	_, err := fn(vp, goja.Null())
	// goja throws TypeError for null.ToObject — this is expected
	if err == nil {
		t.Error("expected error from update with null")
	}
}

func TestUpdate_UnrecognizedMsg(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	msg := rt.NewObject()
	_ = msg.Set("type", "SomethingUnknown")
	result := vpCallMethod(t, rt, vp, "update", msg)
	arr := result.ToObject(rt)
	if arr.Get("0").ToObject(rt) != vp.ToObject(rt) {
		t.Error("result[0] should be viewport object")
	}
}

func TestUpdate_WithWindowSizeMsg(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	msg := rt.NewObject()
	_ = msg.Set("type", "WindowSize")
	_ = msg.Set("width", 80)
	_ = msg.Set("height", 40)
	result := vpCallMethod(t, rt, vp, "update", msg)
	arr := result.ToObject(rt)
	if arr.Get("0").ToObject(rt) != vp.ToObject(rt) {
		t.Error("result[0] should be viewport object")
	}
}

// =============================================================================
// setMouseWheelDelta with no args
// =============================================================================

func TestSetMouseWheelDelta_NoArgs(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	fn, ok := goja.AssertFunction(vp.ToObject(rt).Get("setMouseWheelDelta"))
	if !ok {
		t.Fatal("not a function")
	}
	result, err := fn(vp)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if !goja.IsUndefined(result) {
		t.Error("should return undefined with no args")
	}
}

// =============================================================================
// scrollPercent
// =============================================================================

func TestScrollPercent(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A\nB\nC\nD\nE\nF\nG\nH\nI\nJ"))
	result := vpCallMethod(t, rt, vp, "scrollPercent")
	pct := result.ToFloat()
	if pct < 0 || pct > 1 {
		t.Errorf("expected 0-1, got %f", pct)
	}
}

// =============================================================================
// atTop / atBottom
// =============================================================================

func TestAtTop_Initially(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	result := vpCallMethod(t, rt, vp, "atTop")
	if !result.ToBoolean() {
		t.Error("should be at top initially")
	}
}

func TestAtBottom_AfterGotoBottom(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A\nB\nC\nD\nE\nF\nG\nH"))
	vpCallMethod(t, rt, vp, "gotoBottom")
	result := vpCallMethod(t, rt, vp, "atBottom")
	if !result.ToBoolean() {
		t.Error("should be at bottom after gotoBottom")
	}
}

// =============================================================================
// totalLineCount / visibleLineCount
// =============================================================================

func TestTotalLineCount(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A\nB\nC"))
	result := vpCallMethod(t, rt, vp, "totalLineCount")
	if result.ToInteger() != 3 {
		t.Errorf("expected 3, got %d", result.ToInteger())
	}
}

func TestVisibleLineCount(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A\nB\nC\nD\nE\nF\nG"))
	result := vpCallMethod(t, rt, vp, "visibleLineCount")
	if result.ToInteger() <= 0 {
		t.Errorf("expected >0, got %d", result.ToInteger())
	}
}

// =============================================================================
// pageUp / pageDown / halfPageUp / halfPageDown
// =============================================================================

func TestPageUp_Chaining(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A\nB\nC\nD\nE\nF\nG\nH"))
	vpCallMethod(t, rt, vp, "gotoBottom")
	result := vpCallMethod(t, rt, vp, "pageUp")
	if result.ToObject(rt) != vp.ToObject(rt) {
		t.Error("should return obj for chaining")
	}
}

func TestPageDown_Chaining(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A\nB\nC\nD\nE\nF\nG\nH"))
	result := vpCallMethod(t, rt, vp, "pageDown")
	if result.ToObject(rt) != vp.ToObject(rt) {
		t.Error("should return obj for chaining")
	}
}

func TestHalfPageUp_Chaining(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A\nB\nC\nD\nE\nF\nG\nH"))
	vpCallMethod(t, rt, vp, "gotoBottom")
	result := vpCallMethod(t, rt, vp, "halfPageUp")
	if result.ToObject(rt) != vp.ToObject(rt) {
		t.Error("should return obj for chaining")
	}
}

func TestHalfPageDown_Chaining(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A\nB\nC\nD\nE\nF\nG\nH"))
	result := vpCallMethod(t, rt, vp, "halfPageDown")
	if result.ToObject(rt) != vp.ToObject(rt) {
		t.Error("should return obj for chaining")
	}
}

// =============================================================================
// gotoTop / gotoBottom chaining
// =============================================================================

func TestGotoTop_Chaining(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A\nB\nC\nD\nE\nF"))
	vpCallMethod(t, rt, vp, "gotoBottom")
	result := vpCallMethod(t, rt, vp, "gotoTop")
	if result.ToObject(rt) != vp.ToObject(rt) {
		t.Error("should return obj for chaining")
	}
}

func TestGotoBottom_Chaining(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A\nB\nC\nD\nE\nF"))
	result := vpCallMethod(t, rt, vp, "gotoBottom")
	if result.ToObject(rt) != vp.ToObject(rt) {
		t.Error("should return obj for chaining")
	}
}

// =============================================================================
// scrollDown / scrollUp with explicit args
// =============================================================================

func TestScrollDown_ExplicitArgs(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A\nB\nC\nD\nE\nF\nG\nH"))
	vpCallMethod(t, rt, vp, "scrollDown", rt.ToValue(3))
	y := vpCallMethod(t, rt, vp, "yOffset")
	if y.ToInteger() != 3 {
		t.Errorf("expected 3, got %d", y.ToInteger())
	}
}

func TestScrollUp_ExplicitArgs(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A\nB\nC\nD\nE\nF\nG\nH"))
	vpCallMethod(t, rt, vp, "scrollDown", rt.ToValue(5))
	// 8 lines, height 5 => max yOffset = 3; scrollDown(5) clamps to 3
	// scrollUp(2) => 3 - 2 = 1
	vpCallMethod(t, rt, vp, "scrollUp", rt.ToValue(2))
	y := vpCallMethod(t, rt, vp, "yOffset")
	if y.ToInteger() != 1 {
		t.Errorf("expected 1, got %d", y.ToInteger())
	}
}

// =============================================================================
// scrollLeft / scrollRight with explicit args
// =============================================================================

func TestScrollLeft_ExplicitArgs(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A very long line that extends far beyond the viewport width for scrolling"))
	vpCallMethod(t, rt, vp, "scrollRight", rt.ToValue(10))
	vpCallMethod(t, rt, vp, "scrollLeft", rt.ToValue(3))
	x := vpCallMethod(t, rt, vp, "xOffset")
	if x.ToInteger() != 7 {
		t.Errorf("expected 7, got %d", x.ToInteger())
	}
}

func TestScrollRight_ExplicitArgs(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A very long line that extends far beyond the viewport width for scrolling"))
	vpCallMethod(t, rt, vp, "scrollRight", rt.ToValue(5))
	x := vpCallMethod(t, rt, vp, "xOffset")
	if x.ToInteger() != 5 {
		t.Errorf("expected 5, got %d", x.ToInteger())
	}
}

// =============================================================================
// xOffset getter
// =============================================================================

func TestXOffset_Getter(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	x := vpCallMethod(t, rt, vp, "xOffset")
	if x.ToInteger() != 0 {
		t.Errorf("expected 0 initially, got %d", x.ToInteger())
	}
}

// =============================================================================
// yOffset getter
// =============================================================================

func TestYOffset_Getter(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	y := vpCallMethod(t, rt, vp, "yOffset")
	if y.ToInteger() != 0 {
		t.Errorf("expected 0 initially, got %d", y.ToInteger())
	}
}

// =============================================================================
// mouseWheelDelta getter
// =============================================================================

func TestMouseWheelDelta_Getter(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	d := vpCallMethod(t, rt, vp, "mouseWheelDelta")
	if d.ToInteger() <= 0 {
		t.Errorf("expected > 0, got %d", d.ToInteger())
	}
}

// =============================================================================
// view
// =============================================================================

func TestView_ReturnsString(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("Hello\nWorld"))
	result := vpCallMethod(t, rt, vp, "view")
	if result.String() == "" {
		t.Error("view should return non-empty string")
	}
}

// =============================================================================
// isMouseWheelEnabled getter
// =============================================================================

func TestIsMouseWheelEnabled_Default(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	// Default viewport should have mouse wheel enabled
	result := vpCallMethod(t, rt, vp, "isMouseWheelEnabled")
	if !result.ToBoolean() {
		t.Error("expected mouse wheel enabled by default")
	}
}

// =============================================================================
// width / height getters
// =============================================================================

func TestWidth_Getter(t *testing.T) {
	rt, vp := newTestViewport(t, 30, 15)
	w := vpCallMethod(t, rt, vp, "width")
	if w.ToInteger() != 30 {
		t.Errorf("expected 30, got %d", w.ToInteger())
	}
}

func TestHeight_Getter(t *testing.T) {
	rt, vp := newTestViewport(t, 30, 15)
	h := vpCallMethod(t, rt, vp, "height")
	if h.ToInteger() != 15 {
		t.Errorf("expected 15, got %d", h.ToInteger())
	}
}

// =============================================================================
// setContent with value
// =============================================================================

func TestSetContent_WithValue(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	result := vpCallMethod(t, rt, vp, "setContent", rt.ToValue("Hello World"))
	if result.ToObject(rt) != vp.ToObject(rt) {
		t.Error("should return obj for chaining")
	}
}

// =============================================================================
// setXOffset / setYOffset with values
// =============================================================================

func TestSetXOffset_WithValue(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A very long line for horizontal scroll testing"))
	result := vpCallMethod(t, rt, vp, "setXOffset", rt.ToValue(5))
	if result.ToObject(rt) != vp.ToObject(rt) {
		t.Error("should return obj for chaining")
	}
}

func TestSetYOffset_WithValue(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 5)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A\nB\nC\nD\nE\nF"))
	result := vpCallMethod(t, rt, vp, "setYOffset", rt.ToValue(2))
	if result.ToObject(rt) != vp.ToObject(rt) {
		t.Error("should return obj for chaining")
	}
}

// =============================================================================
// setHorizontalStep with value
// =============================================================================

func TestSetHorizontalStep_WithValue(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	result := vpCallMethod(t, rt, vp, "setHorizontalStep", rt.ToValue(3))
	if result.ToObject(rt) != vp.ToObject(rt) {
		t.Error("should return obj for chaining")
	}
}

// =============================================================================
// setMouseWheelDelta with value
// =============================================================================

func TestSetMouseWheelDelta_WithValue(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	result := vpCallMethod(t, rt, vp, "setMouseWheelDelta", rt.ToValue(5))
	if result.ToObject(rt) != vp.ToObject(rt) {
		t.Error("should return obj for chaining")
	}
	d := vpCallMethod(t, rt, vp, "mouseWheelDelta")
	if d.ToInteger() != 5 {
		t.Errorf("expected 5, got %d", d.ToInteger())
	}
}

// =============================================================================
// setWidth / setHeight with values
// =============================================================================

func TestSetWidth_WithValue(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	result := vpCallMethod(t, rt, vp, "setWidth", rt.ToValue(50))
	if result.ToObject(rt) != vp.ToObject(rt) {
		t.Error("should return obj for chaining")
	}
	w := vpCallMethod(t, rt, vp, "width")
	if w.ToInteger() != 50 {
		t.Errorf("expected 50, got %d", w.ToInteger())
	}
}

func TestSetHeight_WithValue(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	result := vpCallMethod(t, rt, vp, "setHeight", rt.ToValue(30))
	if result.ToObject(rt) != vp.ToObject(rt) {
		t.Error("should return obj for chaining")
	}
	h := vpCallMethod(t, rt, vp, "height")
	if h.ToInteger() != 30 {
		t.Errorf("expected 30, got %d", h.ToInteger())
	}
}

// =============================================================================
// setMouseWheelEnabled with value
// =============================================================================

func TestSetMouseWheelEnabled_Disable(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	result := vpCallMethod(t, rt, vp, "setMouseWheelEnabled", rt.ToValue(false))
	if result.ToObject(rt) != vp.ToObject(rt) {
		t.Error("should return obj for chaining")
	}
	enabled := vpCallMethod(t, rt, vp, "isMouseWheelEnabled")
	if enabled.ToBoolean() {
		t.Error("expected disabled")
	}
}

// =============================================================================
// update with Key message (no-op for viewport but tests path)
// =============================================================================

func TestUpdate_KeyMsg(t *testing.T) {
	rt, vp := newTestViewport(t, 20, 10)
	vpCallMethod(t, rt, vp, "setContent", rt.ToValue("A\nB\nC\nD\nE\nF\nG\nH"))
	msg := rt.NewObject()
	_ = msg.Set("type", "Key")
	_ = msg.Set("key", "j")
	result := vpCallMethod(t, rt, vp, "update", msg)
	arr := result.ToObject(rt)
	if arr.Get("0").ToObject(rt) != vp.ToObject(rt) {
		t.Error("result[0] should be viewport object")
	}
}

// =============================================================================
// getUnexportedXOffset panic recovery (defensive dead code)
// This tests the panic-recovery path in getUnexportedXOffset
// =============================================================================

// Note: getUnexportedXOffset panic path is unreachable with a valid *viewport.Model.
// The recover() in the function wraps the panic, but since viewport.Model always
// has the xOffset field, this is defensive dead code.
