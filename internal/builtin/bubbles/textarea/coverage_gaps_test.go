package textarea

import (
	"testing"

	"github.com/dop251/goja"
)

// =============================================================================
// runeWidth tests
// =============================================================================

func TestRuneWidth_ASCII(t *testing.T) {
	if w := runeWidth('A'); w != 1 {
		t.Errorf("expected 1, got %d", w)
	}
}

func TestRuneWidth_CJK(t *testing.T) {
	// CJK characters are 2 cells wide
	if w := runeWidth('你'); w != 2 {
		t.Errorf("expected 2, got %d", w)
	}
}

func TestRuneWidth_ZeroWidth(t *testing.T) {
	// Combining marks (zero-width in runewidth) should return 1
	// U+0300 COMBINING GRAVE ACCENT has zero width
	if w := runeWidth('\u0300'); w != 1 {
		t.Errorf("expected 1 for combining mark, got %d", w)
	}
}

func TestRuneWidth_ControlChar(t *testing.T) {
	// Control characters like BEL have 0 width in runewidth
	if w := runeWidth('\x07'); w != 1 {
		t.Errorf("expected 1 for control char, got %d", w)
	}
}

func TestRuneWidth_NullChar(t *testing.T) {
	// Null byte
	if w := runeWidth('\x00'); w != 1 {
		t.Errorf("expected 1 for null char, got %d", w)
	}
}

// =============================================================================
// Helper to create a textarea for testing
// =============================================================================

func newTestTextarea(t *testing.T) (*goja.Runtime, goja.Value) {
	t.Helper()
	runtime := goja.New()
	module := runtime.NewObject()
	Require()(runtime, module)
	exports := module.Get("exports").ToObject(runtime)
	newFn, ok := goja.AssertFunction(exports.Get("new"))
	if !ok {
		t.Fatal("new is not a function")
	}
	result, err := newFn(goja.Undefined())
	if err != nil {
		t.Fatalf("new() failed: %v", err)
	}
	return runtime, result
}

func callMethod(t *testing.T, runtime *goja.Runtime, obj goja.Value, method string, args ...goja.Value) goja.Value {
	t.Helper()
	fn, ok := goja.AssertFunction(obj.ToObject(runtime).Get(method))
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
// setMaxHeight / setMaxWidth
// =============================================================================

func TestSetMaxHeight(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	result := callMethod(t, runtime, ta, "setMaxHeight", runtime.ToValue(10))
	// Should return obj for chaining
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setMaxHeight should return obj for chaining")
	}
}

func TestSetMaxWidth(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	result := callMethod(t, runtime, ta, "setMaxWidth", runtime.ToValue(80))
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setMaxWidth should return obj for chaining")
	}
}

// =============================================================================
// insertString / insertRune
// =============================================================================

func TestInsertString(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "focus")
	callMethod(t, runtime, ta, "insertString", runtime.ToValue("hello"))
	val := callMethod(t, runtime, ta, "value")
	if val.String() != "hello" {
		t.Errorf("expected 'hello', got %q", val.String())
	}
}

func TestInsertRune(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "focus")
	callMethod(t, runtime, ta, "insertRune", runtime.ToValue("X"))
	val := callMethod(t, runtime, ta, "value")
	if val.String() != "X" {
		t.Errorf("expected 'X', got %q", val.String())
	}
}

func TestInsertRune_EmptyString(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "focus")
	// Empty string — should not insert anything
	callMethod(t, runtime, ta, "insertRune", runtime.ToValue(""))
	val := callMethod(t, runtime, ta, "value")
	if val.String() != "" {
		t.Errorf("expected empty after inserting empty string, got %q", val.String())
	}
}

func TestInsertRune_MultiChar(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "focus")
	// Only first rune should be inserted
	callMethod(t, runtime, ta, "insertRune", runtime.ToValue("AB"))
	val := callMethod(t, runtime, ta, "value")
	if val.String() != "A" {
		t.Errorf("expected 'A' (first rune only), got %q", val.String())
	}
}

// =============================================================================
// length
// =============================================================================

func TestLength(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("Hello"))
	length := callMethod(t, runtime, ta, "length")
	if length.ToInteger() != 5 {
		t.Errorf("expected length 5, got %d", length.ToInteger())
	}
}

func TestLength_Empty(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	length := callMethod(t, runtime, ta, "length")
	if length.ToInteger() != 0 {
		t.Errorf("expected length 0, got %d", length.ToInteger())
	}
}

// =============================================================================
// lineInfo
// =============================================================================

func TestLineInfo(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("Hello World"))
	callMethod(t, runtime, ta, "setCursor", runtime.ToValue(5))

	li := callMethod(t, runtime, ta, "lineInfo")
	obj := li.ToObject(runtime)

	// Verify all fields are present and numeric
	fields := []string{"width", "charWidth", "height", "startColumn", "columnOffset", "rowOffset", "charOffset"}
	for _, f := range fields {
		v := obj.Get(f)
		if v == nil || goja.IsUndefined(v) {
			t.Errorf("lineInfo missing field: %s", f)
		}
	}
}

// =============================================================================
// reset
// =============================================================================

func TestReset(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("Hello"))
	result := callMethod(t, runtime, ta, "reset")
	// Should return obj for chaining
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("reset should return obj for chaining")
	}
	// Value should be empty after reset
	val := callMethod(t, runtime, ta, "value")
	if val.String() != "" {
		t.Errorf("expected empty after reset, got %q", val.String())
	}
}

// =============================================================================
// Cursor movement: cursorUp, cursorDown, cursorStart, cursorEnd
// =============================================================================

func TestCursorUp(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("Line 0\nLine 1\nLine 2"))
	callMethod(t, runtime, ta, "setPosition", runtime.ToValue(2), runtime.ToValue(0))

	result := callMethod(t, runtime, ta, "cursorUp")
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("cursorUp should return obj for chaining")
	}
	line := callMethod(t, runtime, ta, "line")
	if line.ToInteger() != 1 {
		t.Errorf("expected line 1 after cursorUp, got %d", line.ToInteger())
	}
}

func TestCursorDown(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("Line 0\nLine 1\nLine 2"))
	callMethod(t, runtime, ta, "setPosition", runtime.ToValue(0), runtime.ToValue(0))

	result := callMethod(t, runtime, ta, "cursorDown")
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("cursorDown should return obj for chaining")
	}
	line := callMethod(t, runtime, ta, "line")
	if line.ToInteger() != 1 {
		t.Errorf("expected line 1 after cursorDown, got %d", line.ToInteger())
	}
}

func TestCursorStart(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("Hello World"))
	callMethod(t, runtime, ta, "setCursor", runtime.ToValue(5))

	result := callMethod(t, runtime, ta, "cursorStart")
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("cursorStart should return obj for chaining")
	}
	col := callMethod(t, runtime, ta, "col")
	if col.ToInteger() != 0 {
		t.Errorf("expected col 0 after cursorStart, got %d", col.ToInteger())
	}
}

func TestCursorEnd(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("Hello"))
	callMethod(t, runtime, ta, "setCursor", runtime.ToValue(0))

	result := callMethod(t, runtime, ta, "cursorEnd")
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("cursorEnd should return obj for chaining")
	}
	col := callMethod(t, runtime, ta, "col")
	if col.ToInteger() != 5 {
		t.Errorf("expected col 5 after cursorEnd, got %d", col.ToInteger())
	}
}

// =============================================================================
// Configuration: setPrompt, setPlaceholder, setCharLimit
// =============================================================================

func TestSetPrompt(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	result := callMethod(t, runtime, ta, "setPrompt", runtime.ToValue("> "))
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setPrompt should return obj for chaining")
	}
}

func TestSetPlaceholder(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	result := callMethod(t, runtime, ta, "setPlaceholder", runtime.ToValue("Type here..."))
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setPlaceholder should return obj for chaining")
	}
}

func TestSetCharLimit(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	result := callMethod(t, runtime, ta, "setCharLimit", runtime.ToValue(100))
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setCharLimit should return obj for chaining")
	}
}

// =============================================================================
// Styles: applyStyleConfig edge cases
// =============================================================================

func TestSetFocusedStyle_EmptyObject(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	// Empty config should not panic
	config := runtime.NewObject()
	result := callMethod(t, runtime, ta, "setFocusedStyle", config)
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setFocusedStyle should return obj for chaining")
	}
}

func TestSetFocusedStyle_WithStyleAttributes(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	config := runtime.NewObject()

	// Create a style object for "base" key
	baseStyle := runtime.NewObject()
	_ = baseStyle.Set("foreground", "red")
	_ = baseStyle.Set("background", "blue")
	_ = baseStyle.Set("bold", true)
	_ = baseStyle.Set("italic", true)
	_ = baseStyle.Set("underline", true)
	_ = config.Set("base", baseStyle)

	result := callMethod(t, runtime, ta, "setFocusedStyle", config)
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setFocusedStyle should return obj for chaining")
	}
}

func TestSetFocusedStyle_WithAllStyleKeys(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	config := runtime.NewObject()

	// Set style for each sub-key
	keys := []string{"base", "cursorLine", "cursorLineNumber", "endOfBuffer", "lineNumber", "placeholder", "prompt", "text"}
	for _, key := range keys {
		style := runtime.NewObject()
		_ = style.Set("foreground", "#ff0000")
		_ = config.Set(key, style)
	}

	result := callMethod(t, runtime, ta, "setFocusedStyle", config)
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setFocusedStyle should return obj for chaining")
	}
}

func TestSetFocusedStyle_NullStyleKey(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	config := runtime.NewObject()
	// Set a key to null — should be skipped
	_ = config.Set("base", goja.Null())
	result := callMethod(t, runtime, ta, "setFocusedStyle", config)
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setFocusedStyle should return obj for chaining")
	}
}

func TestSetFocusedStyle_UndefinedStyleKey(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	config := runtime.NewObject()
	// Set a key to undefined
	_ = config.Set("base", goja.Undefined())
	result := callMethod(t, runtime, ta, "setFocusedStyle", config)
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setFocusedStyle should return obj for chaining")
	}
}

func TestSetFocusedStyle_StyleResetEmptyObject(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	config := runtime.NewObject()
	// Empty style object {} should reset the style
	emptyStyle := runtime.NewObject()
	_ = config.Set("cursorLine", emptyStyle)
	result := callMethod(t, runtime, ta, "setFocusedStyle", config)
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setFocusedStyle should return obj for chaining")
	}
}

func TestSetFocusedStyle_StyleResetObjectWithUndefinedValues(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	config := runtime.NewObject()
	// Object with only undefined values should reset
	style := runtime.NewObject()
	_ = style.Set("foreground", goja.Undefined())
	_ = style.Set("background", goja.Null())
	_ = config.Set("text", style)
	result := callMethod(t, runtime, ta, "setFocusedStyle", config)
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setFocusedStyle should return obj for chaining")
	}
}

func TestSetFocusedStyle_NoArgs(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	fn, ok := goja.AssertFunction(ta.ToObject(runtime).Get("setFocusedStyle"))
	if !ok {
		t.Fatal("setFocusedStyle is not a function")
	}
	result, err := fn(ta) // No arguments
	if err != nil {
		t.Fatalf("setFocusedStyle() failed: %v", err)
	}
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setFocusedStyle with no args should return obj")
	}
}

func TestSetBlurredStyle(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	config := runtime.NewObject()
	style := runtime.NewObject()
	_ = style.Set("foreground", "#aabbcc")
	_ = style.Set("background", "#112233")
	_ = style.Set("bold", false)
	_ = style.Set("italic", false)
	_ = style.Set("underline", false)
	_ = config.Set("base", style)
	result := callMethod(t, runtime, ta, "setBlurredStyle", config)
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setBlurredStyle should return obj for chaining")
	}
}

func TestSetBlurredStyle_NoArgs(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	fn, ok := goja.AssertFunction(ta.ToObject(runtime).Get("setBlurredStyle"))
	if !ok {
		t.Fatal("setBlurredStyle is not a function")
	}
	result, err := fn(ta) // No arguments
	if err != nil {
		t.Fatalf("setBlurredStyle() failed: %v", err)
	}
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setBlurredStyle with no args should return obj")
	}
}

func TestSetCursorStyle(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	config := runtime.NewObject()
	_ = config.Set("foreground", "#ff0000")
	_ = config.Set("background", "#00ff00")
	result := callMethod(t, runtime, ta, "setCursorStyle", config)
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setCursorStyle should return obj for chaining")
	}
}

func TestSetCursorStyle_NoArgs(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	fn, ok := goja.AssertFunction(ta.ToObject(runtime).Get("setCursorStyle"))
	if !ok {
		t.Fatal("setCursorStyle is not a function")
	}
	result, err := fn(ta) // No arguments
	if err != nil {
		t.Fatalf("setCursorStyle() failed: %v", err)
	}
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setCursorStyle with no args should return obj")
	}
}

func TestSetCursorStyle_NullForeground(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	config := runtime.NewObject()
	_ = config.Set("foreground", goja.Null())
	_ = config.Set("background", goja.Null())
	result := callMethod(t, runtime, ta, "setCursorStyle", config)
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setCursorStyle should return obj for chaining")
	}
}

func TestSetCursorStyle_UndefinedValues(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	config := runtime.NewObject()
	_ = config.Set("foreground", goja.Undefined())
	_ = config.Set("background", goja.Undefined())
	result := callMethod(t, runtime, ta, "setCursorStyle", config)
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setCursorStyle should return obj for chaining")
	}
}

// =============================================================================
// Convenience style setters
// =============================================================================

func TestSetTextForeground(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	result := callMethod(t, runtime, ta, "setTextForeground", runtime.ToValue("#ff0000"))
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setTextForeground should return obj for chaining")
	}
}

func TestSetTextForeground_NoArgs(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	fn, ok := goja.AssertFunction(ta.ToObject(runtime).Get("setTextForeground"))
	if !ok {
		t.Fatal("not a function")
	}
	result, err := fn(ta)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("should return obj")
	}
}

func TestSetPlaceholderForeground(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	result := callMethod(t, runtime, ta, "setPlaceholderForeground", runtime.ToValue("#aabbcc"))
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setPlaceholderForeground should return obj for chaining")
	}
}

func TestSetPlaceholderForeground_NoArgs(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	fn, ok := goja.AssertFunction(ta.ToObject(runtime).Get("setPlaceholderForeground"))
	if !ok {
		t.Fatal("not a function")
	}
	result, err := fn(ta)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("should return obj")
	}
}

func TestSetCursorForeground(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	result := callMethod(t, runtime, ta, "setCursorForeground", runtime.ToValue("#00ff00"))
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setCursorForeground should return obj for chaining")
	}
}

func TestSetCursorForeground_NoArgs(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	fn, ok := goja.AssertFunction(ta.ToObject(runtime).Get("setCursorForeground"))
	if !ok {
		t.Fatal("not a function")
	}
	result, err := fn(ta)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("should return obj")
	}
}

func TestSetCursorLineForeground(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	result := callMethod(t, runtime, ta, "setCursorLineForeground", runtime.ToValue("#0000ff"))
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setCursorLineForeground should return obj for chaining")
	}
}

func TestSetCursorLineForeground_NoArgs(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	fn, ok := goja.AssertFunction(ta.ToObject(runtime).Get("setCursorLineForeground"))
	if !ok {
		t.Fatal("not a function")
	}
	result, err := fn(ta)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("should return obj")
	}
}

// =============================================================================
// update edge cases
// =============================================================================

func TestUpdate_NoArgs(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	fn, ok := goja.AssertFunction(ta.ToObject(runtime).Get("update"))
	if !ok {
		t.Fatal("update is not a function")
	}
	result, err := fn(ta) // No arguments
	if err != nil {
		t.Fatalf("update() with no args failed: %v", err)
	}
	arr := result.ToObject(runtime)
	if arr.Get("0").ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("result[0] should be the textarea object")
	}
	if !goja.IsNull(arr.Get("1")) {
		t.Error("result[1] should be null when no args")
	}
}

func TestUpdate_NilMsg(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	// Pass an object that doesn't match any known message type
	msg := runtime.NewObject()
	_ = msg.Set("type", "UnknownType")
	result := callMethod(t, runtime, ta, "update", msg)
	arr := result.ToObject(runtime)
	if arr.Get("0").ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("result[0] should be the textarea object")
	}
	if !goja.IsNull(arr.Get("1")) {
		t.Error("result[1] should be null for unrecognized message")
	}
}

// =============================================================================
// view
// =============================================================================

func TestView(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("Hello World"))
	result := callMethod(t, runtime, ta, "view")
	if result.String() == "" {
		t.Error("view should return non-empty string")
	}
}

// =============================================================================
// _type property
// =============================================================================

func TestTypeProperty(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	typeVal := ta.ToObject(runtime).Get("_type")
	if typeVal.String() != "bubbles/textarea" {
		t.Errorf("expected _type 'bubbles/textarea', got %q", typeVal.String())
	}
}

// =============================================================================
// yOffset / reservedInnerWidth with nil viewport
// =============================================================================

func TestYOffset_NilViewport(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	// Fresh textarea — viewport may be nil before SetWidth is called
	result := callMethod(t, runtime, ta, "yOffset")
	if result.ToInteger() != 0 {
		t.Errorf("expected yOffset 0 with nil viewport, got %d", result.ToInteger())
	}
}

func TestReservedInnerWidth_NilViewport(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	// Fresh textarea — viewport is nil before dimensions are set
	result := callMethod(t, runtime, ta, "reservedInnerWidth")
	// Should fallback to promptWidth
	if result.ToInteger() < 0 {
		t.Errorf("reservedInnerWidth should be non-negative, got %d", result.ToInteger())
	}
}

// =============================================================================
// setViewportContext edge cases
// =============================================================================

func TestSetViewportContext_NoArgs(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	fn, ok := goja.AssertFunction(ta.ToObject(runtime).Get("setViewportContext"))
	if !ok {
		t.Fatal("setViewportContext is not a function")
	}
	result, err := fn(ta) // No arguments
	if err != nil {
		t.Fatalf("setViewportContext() failed: %v", err)
	}
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("should return obj for chaining")
	}
}

func TestSetViewportContext_EmptyObject(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	config := runtime.NewObject()
	result := callMethod(t, runtime, ta, "setViewportContext", config)
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("should return obj for chaining")
	}
}

func TestSetViewportContext_PartialFields(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	config := runtime.NewObject()
	_ = config.Set("outerYOffset", 5)
	// Other fields missing — should still work
	result := callMethod(t, runtime, ta, "setViewportContext", config)
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("should return obj for chaining")
	}
}

func TestSetViewportContext_AllFields(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	config := runtime.NewObject()
	_ = config.Set("outerYOffset", 10)
	_ = config.Set("textareaContentTop", 2)
	_ = config.Set("textareaContentLeft", 5)
	_ = config.Set("outerViewportHeight", 20)
	_ = config.Set("preContentHeight", 2)
	_ = config.Set("titleHeight", 3)
	result := callMethod(t, runtime, ta, "setViewportContext", config)
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("should return obj for chaining")
	}
}

func TestSetViewportContext_NullFieldValues(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	config := runtime.NewObject()
	_ = config.Set("outerYOffset", goja.Null())
	_ = config.Set("textareaContentTop", goja.Undefined())
	result := callMethod(t, runtime, ta, "setViewportContext", config)
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("should return obj for chaining")
	}
}

// =============================================================================
// handleClickAtScreenCoords edge cases
// =============================================================================

func TestHandleClickAtScreenCoords_LessThan2Args(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	fn, ok := goja.AssertFunction(ta.ToObject(runtime).Get("handleClickAtScreenCoords"))
	if !ok {
		t.Fatal("handleClickAtScreenCoords is not a function")
	}
	// 0 args
	result, err := fn(ta)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	obj := result.ToObject(runtime)
	if obj.Get("hit").ToBoolean() {
		t.Error("should not hit with 0 args")
	}

	// 1 arg
	result, err = fn(ta, runtime.ToValue(10))
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	obj = result.ToObject(runtime)
	if obj.Get("hit").ToBoolean() {
		t.Error("should not hit with 1 arg")
	}
}

func TestHandleClickAtScreenCoords_EmptyDocument(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(40))
	// Setup viewport context
	config := runtime.NewObject()
	_ = config.Set("outerYOffset", 0)
	_ = config.Set("textareaContentTop", 0)
	_ = config.Set("textareaContentLeft", 0)
	_ = config.Set("outerViewportHeight", 10)
	_ = config.Set("preContentHeight", 0)
	_ = config.Set("titleHeight", 0)
	callMethod(t, runtime, ta, "setViewportContext", config)

	// Empty document — value is "" but textarea.value has at least 1 empty line
	result := callMethod(t, runtime, ta, "handleClickAtScreenCoords", runtime.ToValue(5), runtime.ToValue(0))
	obj := result.ToObject(runtime)
	// Should hit since viewport is initialized and click is in bounds
	// (empty textarea still has 1 visual line)
	if obj.Get("hit").ToBoolean() {
		row := obj.Get("row").ToInteger()
		if row != 0 {
			t.Errorf("expected row 0 for empty doc click, got %d", row)
		}
	}
}

func TestHandleClickAtScreenCoords_NegativeVisualX(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(40))
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("Hello World"))

	config := runtime.NewObject()
	_ = config.Set("outerYOffset", 0)
	_ = config.Set("textareaContentTop", 0)
	_ = config.Set("textareaContentLeft", 10)
	_ = config.Set("outerViewportHeight", 10)
	_ = config.Set("preContentHeight", 0)
	_ = config.Set("titleHeight", 0)
	callMethod(t, runtime, ta, "setViewportContext", config)

	// screenX < textareaContentLeft → negative visualX → should clamp to 0
	result := callMethod(t, runtime, ta, "handleClickAtScreenCoords", runtime.ToValue(3), runtime.ToValue(0))
	obj := result.ToObject(runtime)
	if obj.Get("hit").ToBoolean() {
		col := obj.Get("col").ToInteger()
		if col != 0 {
			t.Errorf("expected col 0 for negative visualX, got %d", col)
		}
	}
}

func TestHandleClickAtScreenCoords_AboveContent(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(40))
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("Hello"))

	config := runtime.NewObject()
	_ = config.Set("outerYOffset", 0)
	_ = config.Set("textareaContentTop", 5)
	_ = config.Set("textareaContentLeft", 0)
	_ = config.Set("outerViewportHeight", 10)
	_ = config.Set("preContentHeight", 5)
	_ = config.Set("titleHeight", 0)
	callMethod(t, runtime, ta, "setViewportContext", config)

	// Click at Y=2 which is above textareaContentTop=5 → visualY negative → miss
	result := callMethod(t, runtime, ta, "handleClickAtScreenCoords", runtime.ToValue(5), runtime.ToValue(2))
	obj := result.ToObject(runtime)
	if obj.Get("hit").ToBoolean() {
		t.Error("should not hit above textarea content")
	}
}

func TestHandleClickAtScreenCoords_ZeroWidthContent(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	// Don't set width — content width may be 0
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("A"))

	config := runtime.NewObject()
	_ = config.Set("outerYOffset", 0)
	_ = config.Set("textareaContentTop", 0)
	_ = config.Set("textareaContentLeft", 0)
	_ = config.Set("outerViewportHeight", 10)
	_ = config.Set("preContentHeight", 0)
	_ = config.Set("titleHeight", 0)
	callMethod(t, runtime, ta, "setViewportContext", config)

	// This exercises the zero-width content paths
	result := callMethod(t, runtime, ta, "handleClickAtScreenCoords", runtime.ToValue(5), runtime.ToValue(0))
	_ = result.ToObject(runtime)
	// No crash = pass
}

// =============================================================================
// getScrollSyncInfo edge cases
// =============================================================================

func TestGetScrollSyncInfo_EmptyDocument(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	// Don't set value — empty doc

	result := callMethod(t, runtime, ta, "getScrollSyncInfo")
	obj := result.ToObject(runtime)

	if obj.Get("cursorVisualLine").ToInteger() != 0 {
		t.Error("expected cursorVisualLine 0 for empty doc")
	}
	if obj.Get("totalVisualLines").ToInteger() != 1 {
		t.Error("expected totalVisualLines 1 for empty doc")
	}
	if obj.Get("cursorRow").ToInteger() != 0 {
		t.Error("expected cursorRow 0")
	}
	if obj.Get("cursorCol").ToInteger() != 0 {
		t.Error("expected cursorCol 0")
	}
	if obj.Get("lineCount").ToInteger() != 1 {
		t.Error("expected lineCount 1")
	}
}

func TestGetScrollSyncInfo_CursorAboveViewport(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(40))
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("Line 0\nLine 1\nLine 2\nLine 3\nLine 4"))

	// Set viewport context with outerYOffset placing cursor above viewport
	config := runtime.NewObject()
	_ = config.Set("outerYOffset", 3) // Viewport starts at line 3
	_ = config.Set("textareaContentTop", 0)
	_ = config.Set("textareaContentLeft", 0)
	_ = config.Set("outerViewportHeight", 2)
	_ = config.Set("preContentHeight", 0)
	callMethod(t, runtime, ta, "setViewportContext", config)

	// Cursor at line 0, which is above viewport offset 3
	callMethod(t, runtime, ta, "setPosition", runtime.ToValue(0), runtime.ToValue(0))
	result := callMethod(t, runtime, ta, "getScrollSyncInfo")
	obj := result.ToObject(runtime)

	suggestedYOffset := obj.Get("suggestedYOffset").ToInteger()
	cursorAbsY := obj.Get("cursorAbsY").ToInteger()
	if cursorAbsY != 0 {
		t.Errorf("expected cursorAbsY 0, got %d", cursorAbsY)
	}
	// Should suggest scrolling up to cursor
	if suggestedYOffset != 0 {
		t.Errorf("expected suggestedYOffset 0 (scroll to cursor), got %d", suggestedYOffset)
	}
}

func TestGetScrollSyncInfo_NegativeSuggestedYOffset(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(40))
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("A"))

	// Set viewport with negative preContentHeight scenario
	config := runtime.NewObject()
	_ = config.Set("outerYOffset", 0)
	_ = config.Set("textareaContentTop", 0)
	_ = config.Set("textareaContentLeft", 0)
	_ = config.Set("outerViewportHeight", 10)
	_ = config.Set("preContentHeight", 0)
	callMethod(t, runtime, ta, "setViewportContext", config)

	result := callMethod(t, runtime, ta, "getScrollSyncInfo")
	obj := result.ToObject(runtime)
	suggestedYOffset := obj.Get("suggestedYOffset").ToInteger()
	if suggestedYOffset < 0 {
		t.Errorf("suggestedYOffset should not be negative, got %d", suggestedYOffset)
	}
}

// =============================================================================
// handleClick edge cases
// =============================================================================

func TestHandleClick_LessThan3Args(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("Hello"))
	fn, ok := goja.AssertFunction(ta.ToObject(runtime).Get("handleClick"))
	if !ok {
		t.Fatal("handleClick is not a function")
	}
	// 0 args — should return obj without crash
	result, err := fn(ta)
	if err != nil {
		t.Fatalf("handleClick with 0 args failed: %v", err)
	}
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("should return obj for chaining")
	}

	// 2 args
	result, err = fn(ta, runtime.ToValue(5), runtime.ToValue(0))
	if err != nil {
		t.Fatalf("handleClick with 2 args failed: %v", err)
	}
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("should return obj for chaining")
	}
}

func TestHandleClick_NegativeCoords(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(20))
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("Hello World"))

	// Negative clickX — should clamp to 0 in the zero-width path
	// Actually, this tests the path where clickX < 0 in the else branch
	result := callMethod(t, runtime, ta, "handleClick", runtime.ToValue(-5), runtime.ToValue(0), runtime.ToValue(0))
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("should return obj for chaining")
	}
	col := callMethod(t, runtime, ta, "col")
	if col.ToInteger() != 0 {
		t.Errorf("expected col 0 for negative clickX, got %d", col.ToInteger())
	}
}

func TestHandleClick_ZeroWidth(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	// Don't call setWidth -> internal width stays at 0
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("Hello"))

	result := callMethod(t, runtime, ta, "handleClick", runtime.ToValue(3), runtime.ToValue(0), runtime.ToValue(0))
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("should return obj for chaining")
	}
}

func TestHandleClick_ZeroWidth_ClickBeyondLength(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	// Don't call setWidth -> internal width=0, triggers else branch
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("AB"))
	// Click at x=100 -> targetCol=100 > len([A,B])=2 -> clamp to 2
	callMethod(t, runtime, ta, "handleClick", runtime.ToValue(100), runtime.ToValue(0), runtime.ToValue(0))
	col := callMethod(t, runtime, ta, "col")
	if col.ToInteger() != 2 {
		t.Errorf("expected col 2 (clamped), got %d", col.ToInteger())
	}
}

func TestHandleClick_ZeroWidth_NegativeClickX(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	// Don't call setWidth -> width=0, triggers else branch
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("AB"))
	// Negative clickX -> targetCol < 0 -> clamp to 0
	callMethod(t, runtime, ta, "handleClick", runtime.ToValue(-5), runtime.ToValue(0), runtime.ToValue(0))
	col := callMethod(t, runtime, ta, "col")
	if col.ToInteger() != 0 {
		t.Errorf("expected col 0 (clamped from negative), got %d", col.ToInteger())
	}
}

func TestHandleClick_ClickBeyondContentClamps(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(20))
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("AB"))

	// Click at column 100 — should clamp to line length
	callMethod(t, runtime, ta, "handleClick", runtime.ToValue(100), runtime.ToValue(0), runtime.ToValue(0))
	col := callMethod(t, runtime, ta, "col")
	if col.ToInteger() != 2 {
		t.Errorf("expected col 2 (clamped), got %d", col.ToInteger())
	}
}

// =============================================================================
// performHitTest edge cases
// =============================================================================

func TestPerformHitTest_LessThan2Args(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	fn, ok := goja.AssertFunction(ta.ToObject(runtime).Get("performHitTest"))
	if !ok {
		t.Fatal("performHitTest is not a function")
	}
	// 0 args
	result, err := fn(ta)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	obj := result.ToObject(runtime)
	// Should return empty object
	if obj == nil {
		t.Error("should return object")
	}
	// 1 arg
	result, err = fn(ta, runtime.ToValue(5))
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	obj = result.ToObject(runtime)
	if obj == nil {
		t.Error("should return object")
	}
}

func TestPerformHitTest_EmptyDocument(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(40))
	result := callMethod(t, runtime, ta, "performHitTest", runtime.ToValue(5), runtime.ToValue(0))
	obj := result.ToObject(runtime)
	if obj.Get("row").ToInteger() != 0 || obj.Get("col").ToInteger() != 0 {
		t.Error("performHitTest on empty doc should return row=0, col=0")
	}
}

func TestPerformHitTest_NegativeVisualY(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(40))
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("Hello"))

	result := callMethod(t, runtime, ta, "performHitTest", runtime.ToValue(0), runtime.ToValue(-5))
	obj := result.ToObject(runtime)
	if obj.Get("row").ToInteger() != 0 {
		t.Error("negative visualY should clamp to first line")
	}
}

func TestPerformHitTest_ZeroContentWidth(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	// Don't set width — 0 content width
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("Hello"))

	result := callMethod(t, runtime, ta, "performHitTest", runtime.ToValue(3), runtime.ToValue(0))
	obj := result.ToObject(runtime)
	// Should use the "no width" path
	col := obj.Get("col").ToInteger()
	if col < 0 || col > 5 {
		t.Errorf("col should be in range [0,5], got %d", col)
	}
}

// =============================================================================
// selectAll edge cases
// =============================================================================

func TestSelectAll_SingleLine(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("Hello"))
	callMethod(t, runtime, ta, "selectAll")
	// With single line, should move to end
	col := callMethod(t, runtime, ta, "col")
	if col.ToInteger() != 5 {
		t.Errorf("expected col 5 after selectAll on single line, got %d", col.ToInteger())
	}
}

// =============================================================================
// setRow with empty document
// =============================================================================

func TestSetRow_EmptyDocument(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	// Don't set value — empty document
	result := callMethod(t, runtime, ta, "setRow", runtime.ToValue(5))
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("should return obj for chaining")
	}
}

// =============================================================================
// setPosition with empty document
// =============================================================================

func TestSetPosition_EmptyDocument(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	result := callMethod(t, runtime, ta, "setPosition", runtime.ToValue(5), runtime.ToValue(10))
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("should return obj for chaining")
	}
}

// =============================================================================
// visualLineCount / cursorVisualLine edge cases
// =============================================================================

func TestVisualLineCount_ZeroWidth(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	// Don't call setWidth — content width 0
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("Hello"))
	result := callMethod(t, runtime, ta, "visualLineCount")
	// Each logical line should be 1 visual line when width <= 0
	if result.ToInteger() < 1 {
		t.Error("visualLineCount should be >= 1")
	}
}

func TestCursorVisualLine_EmptyDocument(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	result := callMethod(t, runtime, ta, "cursorVisualLine")
	if result.ToInteger() != 0 {
		t.Errorf("expected 0 for empty doc, got %d", result.ToInteger())
	}
}

func TestCursorVisualLine_ZeroWidth(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	// Don't set width
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("Hello\nWorld"))
	callMethod(t, runtime, ta, "setPosition", runtime.ToValue(1), runtime.ToValue(0))
	result := callMethod(t, runtime, ta, "cursorVisualLine")
	if result.ToInteger() < 1 {
		t.Errorf("expected >= 1 for second line, got %d", result.ToInteger())
	}
}

// =============================================================================
// setShowLineNumbers method
// =============================================================================

func TestSetShowLineNumbers(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	result := callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(true))
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("should return obj for chaining")
	}
	result = callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("should return obj for chaining")
	}
}

// =============================================================================
// width / height getters
// =============================================================================

func TestWidthGetter(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(60))
	w := callMethod(t, runtime, ta, "width")
	if w.ToInteger() <= 0 {
		t.Errorf("width should be positive, got %d", w.ToInteger())
	}
}

func TestHeightGetter(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setHeight", runtime.ToValue(20))
	h := callMethod(t, runtime, ta, "height")
	if h.ToInteger() <= 0 {
		t.Errorf("height should be positive, got %d", h.ToInteger())
	}
}

// =============================================================================
// setCursor chaining
// =============================================================================

func TestSetCursor_Chaining(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("Hello"))
	result := callMethod(t, runtime, ta, "setCursor", runtime.ToValue(3))
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("setCursor should return obj for chaining")
	}
}

// =============================================================================
// focus/blur chaining
// =============================================================================

func TestFocus_Chaining(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	result := callMethod(t, runtime, ta, "focus")
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("focus should return obj for chaining")
	}
}

func TestBlur_Chaining(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	result := callMethod(t, runtime, ta, "blur")
	if result.ToObject(runtime) != ta.ToObject(runtime) {
		t.Error("blur should return obj for chaining")
	}
}

// =============================================================================
// lineCount return value
// =============================================================================

func TestLineCount_MultiLine(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("A\nB\nC"))
	lc := callMethod(t, runtime, ta, "lineCount")
	if lc.ToInteger() != 3 {
		t.Errorf("expected 3, got %d", lc.ToInteger())
	}
}

// =============================================================================
// promptWidth / contentWidth
// =============================================================================

func TestPromptWidth_DefaultPrompt(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	pw := callMethod(t, runtime, ta, "promptWidth")
	// Default prompt is "┃ " (2 visible chars)
	if pw.ToInteger() < 0 {
		t.Errorf("promptWidth should be non-negative, got %d", pw.ToInteger())
	}
}

func TestContentWidth_AfterSetWidth(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(50))
	cw := callMethod(t, runtime, ta, "contentWidth")
	if cw.ToInteger() != 50 {
		t.Errorf("expected contentWidth 50 with no prompt/lineNumbers, got %d", cw.ToInteger())
	}
}

// =============================================================================
// setRow column clamping - when cursor col exceeds new row's length
// =============================================================================

func TestSetRow_ColumnClamping(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("LongLineHere\nAB"))
	// Place cursor at col 10 on the first (long) line
	callMethod(t, runtime, ta, "setPosition", runtime.ToValue(0), runtime.ToValue(10))
	col := callMethod(t, runtime, ta, "col")
	if col.ToInteger() != 10 {
		t.Errorf("expected col 10, got %d", col.ToInteger())
	}
	// Move to row 1 which only has 2 chars — col should clamp to 2
	callMethod(t, runtime, ta, "setRow", runtime.ToValue(1))
	col = callMethod(t, runtime, ta, "col")
	if col.ToInteger() != 2 {
		t.Errorf("expected col 2 (clamped), got %d", col.ToInteger())
	}
}

// =============================================================================
// calculateVisualLineWithinRow - char wider than viewport width
// =============================================================================

func TestCursorVisualLine_CJKWiderThanViewport(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	// Set width=1 so CJK chars (width=2) are wider than viewport
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(1))
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("A你好B"))
	// Place cursor past the CJK chars
	callMethod(t, runtime, ta, "setCursor", runtime.ToValue(4))
	result := callMethod(t, runtime, ta, "cursorVisualLine")
	// With width=1 and mixed-width chars, there will be multiple visual lines
	if result.ToInteger() < 1 {
		t.Errorf("expected >= 1 visual lines for CJK with narrow width, got %d", result.ToInteger())
	}
}

func TestVisualLineCount_CJKWiderThanViewport(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(1))
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("A你好"))
	result := callMethod(t, runtime, ta, "visualLineCount")
	// Each char wraps to its own visual line at width=1
	if result.ToInteger() < 3 {
		t.Errorf("expected >= 3 visual lines, got %d", result.ToInteger())
	}
}

// =============================================================================
// performHitTest wrapping edge cases — hit test on wrapped CJK content
// =============================================================================

func TestPerformHitTest_WrappedContent(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(5))
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("ABCDEFGHIJKLMNOP"))
	// Hit on visual line 1 (second wrapped segment) at x=2
	result := callMethod(t, runtime, ta, "performHitTest", runtime.ToValue(2), runtime.ToValue(1))
	obj := result.ToObject(runtime)
	col := obj.Get("col").ToInteger()
	// Should be 7 (5 chars from first segment + 2 into second)
	if col != 7 {
		t.Errorf("expected col 7 for wrapped hit, got %d", col)
	}
}

func TestPerformHitTest_WrappedContent_BeyondEndOfLine(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(5))
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("ABCDEFGH"))
	// Hit on visual line 1, x=10 — beyond the 3 chars in the second segment
	result := callMethod(t, runtime, ta, "performHitTest", runtime.ToValue(10), runtime.ToValue(1))
	obj := result.ToObject(runtime)
	col := obj.Get("col").ToInteger()
	// Should clamp to 8 (full line length)
	if col != 8 {
		t.Errorf("expected col 8 (clamped), got %d", col)
	}
}

func TestPerformHitTest_WrappedCJK(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	// Width 3: 'A' (1) + '你' (2) = 3 fills visual line 0
	// '好' (2) starts visual line 1
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(3))
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("A你好B"))
	// Hit on visual line 1, x=0
	result := callMethod(t, runtime, ta, "performHitTest", runtime.ToValue(0), runtime.ToValue(1))
	obj := result.ToObject(runtime)
	col := obj.Get("col").ToInteger()
	// '好' is at index 2
	if col != 2 {
		t.Errorf("expected col 2 for wrapped CJK, got %d", col)
	}
}

// =============================================================================
// handleClick with wrapping — exercises segment traversal paths
// =============================================================================

func TestHandleClick_WrappedContent(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(5))
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("ABCDEFGHIJ"))
	// Click on second visual line (visual line 1, which is the second 5-char segment)
	callMethod(t, runtime, ta, "handleClick", runtime.ToValue(2), runtime.ToValue(1), runtime.ToValue(0))
	col := callMethod(t, runtime, ta, "col")
	// Expect col 7 (5 chars passed + 2 into second segment)
	if col.ToInteger() != 7 {
		t.Errorf("expected col 7, got %d", col.ToInteger())
	}
}

func TestHandleClick_VisualLineBeyondDocument(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(20))
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("Line 0\nLine 1"))
	// Click at visual line 100 — way past end of document — should clamp
	callMethod(t, runtime, ta, "handleClick", runtime.ToValue(0), runtime.ToValue(100), runtime.ToValue(0))
	line := callMethod(t, runtime, ta, "line")
	if line.ToInteger() != 1 {
		t.Errorf("expected line 1 (last line), got %d", line.ToInteger())
	}
}

func TestHandleClick_CJKWithWrapping(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(3))
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("A你好B"))
	// Click on visual line 1, x=0 -> should land on '好' at index 2
	callMethod(t, runtime, ta, "handleClick", runtime.ToValue(0), runtime.ToValue(1), runtime.ToValue(0))
	col := callMethod(t, runtime, ta, "col")
	if col.ToInteger() != 2 {
		t.Errorf("expected col 2 for wrapped CJK click, got %d", col.ToInteger())
	}
}

// =============================================================================
// handleClickAtScreenCoords wrapping segment paths
// =============================================================================

func TestHandleClickAtScreenCoords_WrappedContent(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(5))
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("ABCDEFGHIJ"))

	config := runtime.NewObject()
	_ = config.Set("outerYOffset", 0)
	_ = config.Set("textareaContentTop", 0)
	_ = config.Set("textareaContentLeft", 0)
	_ = config.Set("outerViewportHeight", 10)
	_ = config.Set("preContentHeight", 0)
	_ = config.Set("titleHeight", 0)
	callMethod(t, runtime, ta, "setViewportContext", config)

	// Click on visual line 1 (second wrapped segment) at x=2
	result := callMethod(t, runtime, ta, "handleClickAtScreenCoords", runtime.ToValue(2), runtime.ToValue(1))
	obj := result.ToObject(runtime)
	if obj.Get("hit").ToBoolean() {
		col := obj.Get("col").ToInteger()
		if col != 7 {
			t.Errorf("expected col 7 for wrapped hit, got %d", col)
		}
	}
}

// =============================================================================
// Segment traversal edge cases: CJK char breaking mid-segment
// =============================================================================

func TestPerformHitTest_CJK_SegmentBreak(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	// Width=2: 'A'(1) fills partially, '你'(2) would exceed: 1+2=3 > 2
	// This triggers the segmentWidth>0 && segmentWidth+rw>contentWidth break
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(2))
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("A你B"))
	// Visual layout: line 0="A", line 1="你", line 2="B"
	// Hit on visual line 1 (the '你' char)
	result := callMethod(t, runtime, ta, "performHitTest", runtime.ToValue(0), runtime.ToValue(1))
	obj := result.ToObject(runtime)
	col := obj.Get("col").ToInteger()
	if col != 1 {
		t.Errorf("expected col 1 (the CJK char), got %d", col)
	}
}

func TestHandleClick_CJK_SegmentBreak(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	// Width=2: 'A'(1) then '你'(2) exceeds width -> breaks segment
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(2))
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("A你B"))
	callMethod(t, runtime, ta, "handleClick", runtime.ToValue(0), runtime.ToValue(1), runtime.ToValue(0))
	col := callMethod(t, runtime, ta, "col")
	if col.ToInteger() != 1 {
		t.Errorf("expected col 1 for CJK segment break, got %d", col.ToInteger())
	}
}

func TestPerformHitTest_CJK_ColumnBreak(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	// Width=3: "AAAB你C"
	// Visual line 0: "AAA" (3/3), visual line 1: "B你" (1+2=3), then "C" on visual line 2
	// Click on visual line 1 at x=4 (beyond segment width)
	// This triggers: widthConsumed+rw > contentWidth break in column finding
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(3))
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("AAAB你C"))
	result := callMethod(t, runtime, ta, "performHitTest", runtime.ToValue(4), runtime.ToValue(1))
	obj := result.ToObject(runtime)
	col := obj.Get("col").ToInteger()
	// Column should be 5 (indices 0-2=AAA, 3=B, 4=你 -> after '你' at position 5)
	if col < 4 || col > 6 {
		t.Errorf("expected col in [4,6], got %d", col)
	}
}

func TestHandleClick_CJK_ColumnBreak(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	// Width=3, "AAAB你C", click at visual line 1, x=4
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(3))
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("AAAB你C"))
	callMethod(t, runtime, ta, "handleClick", runtime.ToValue(4), runtime.ToValue(1), runtime.ToValue(0))
	col := callMethod(t, runtime, ta, "col")
	if col.ToInteger() < 4 || col.ToInteger() > 6 {
		t.Errorf("expected col in [4,6], got %d", col.ToInteger())
	}
}

func TestHandleClickAtScreenCoords_CJK_SegmentBreak(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(2))
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("A你B"))

	config := runtime.NewObject()
	_ = config.Set("outerYOffset", 0)
	_ = config.Set("textareaContentTop", 0)
	_ = config.Set("textareaContentLeft", 0)
	_ = config.Set("outerViewportHeight", 10)
	_ = config.Set("preContentHeight", 0)
	_ = config.Set("titleHeight", 0)
	callMethod(t, runtime, ta, "setViewportContext", config)

	result := callMethod(t, runtime, ta, "handleClickAtScreenCoords", runtime.ToValue(0), runtime.ToValue(1))
	obj := result.ToObject(runtime)
	if obj.Get("hit").ToBoolean() {
		col := obj.Get("col").ToInteger()
		if col != 1 {
			t.Errorf("expected col 1 for CJK segment break, got %d", col)
		}
	}
}

func TestHandleClickAtScreenCoords_CJK_ColumnBreak(t *testing.T) {
	runtime, ta := newTestTextarea(t)
	callMethod(t, runtime, ta, "setPrompt", runtime.ToValue(""))
	callMethod(t, runtime, ta, "setShowLineNumbers", runtime.ToValue(false))
	callMethod(t, runtime, ta, "setWidth", runtime.ToValue(3))
	callMethod(t, runtime, ta, "setValue", runtime.ToValue("AAAB你C"))

	config := runtime.NewObject()
	_ = config.Set("outerYOffset", 0)
	_ = config.Set("textareaContentTop", 0)
	_ = config.Set("textareaContentLeft", 0)
	_ = config.Set("outerViewportHeight", 10)
	_ = config.Set("preContentHeight", 0)
	_ = config.Set("titleHeight", 0)
	callMethod(t, runtime, ta, "setViewportContext", config)

	result := callMethod(t, runtime, ta, "handleClickAtScreenCoords", runtime.ToValue(4), runtime.ToValue(1))
	obj := result.ToObject(runtime)
	if obj.Get("hit").ToBoolean() {
		col := obj.Get("col").ToInteger()
		if col < 4 || col > 6 {
			t.Errorf("expected col in [4,6], got %d", col)
		}
	}
}
