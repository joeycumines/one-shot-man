package textarea

import (
	"testing"

	"github.com/dop251/goja"
)

// TestNewTextarea tests creating a new textarea instance.
func TestNewTextarea(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	// Set up the module
	module := runtime.NewObject()
	Require(manager)(runtime, module)

	exports := module.Get("exports").ToObject(runtime)
	if exports == nil {
		t.Fatal("exports is nil")
	}

	// Call new()
	newFn, ok := goja.AssertFunction(exports.Get("new"))
	if !ok {
		t.Fatal("new is not a function")
	}

	result, err := newFn(goja.Undefined())
	if err != nil {
		t.Fatalf("new() failed: %v", err)
	}

	obj := result.ToObject(runtime)
	if obj == nil {
		t.Fatal("new() returned nil object")
	}

	// Verify it has expected methods
	methods := []string{"setValue", "value", "setWidth", "setHeight", "focus", "blur", "view"}
	for _, method := range methods {
		if goja.IsUndefined(obj.Get(method)) {
			t.Errorf("textarea missing method: %s", method)
		}
	}
}

// TestTextareaSetPosition tests the setPosition method for cursor positioning.
func TestTextareaSetPosition(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Set some multi-line content
	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	_, _ = setValueFn(ta, runtime.ToValue("Line 0\nLine 1\nLine 2"))

	// Test setPosition
	setPositionFn, ok := goja.AssertFunction(ta.Get("setPosition"))
	if !ok {
		t.Fatal("setPosition is not a function")
	}

	// Set position to row 1, column 3
	_, err := setPositionFn(ta, runtime.ToValue(1), runtime.ToValue(3))
	if err != nil {
		t.Fatalf("setPosition failed: %v", err)
	}

	// Verify row
	lineFn, _ := goja.AssertFunction(ta.Get("line"))
	lineResult, _ := lineFn(ta)
	if lineResult.ToInteger() != 1 {
		t.Errorf("expected line 1, got %d", lineResult.ToInteger())
	}

	// Verify column
	colFn, _ := goja.AssertFunction(ta.Get("col"))
	colResult, _ := colFn(ta)
	if colResult.ToInteger() != 3 {
		t.Errorf("expected col 3, got %d", colResult.ToInteger())
	}
}

// TestTextareaSetPositionClamping tests that setPosition clamps to valid ranges.
func TestTextareaSetPositionClamping(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Set content: "ABC\nDEF"
	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	_, _ = setValueFn(ta, runtime.ToValue("ABC\nDEF"))

	setPositionFn, _ := goja.AssertFunction(ta.Get("setPosition"))
	lineFn, _ := goja.AssertFunction(ta.Get("line"))
	colFn, _ := goja.AssertFunction(ta.Get("col"))

	tests := []struct {
		name        string
		row         int
		col         int
		expectedRow int64
		expectedCol int64
	}{
		{"negative row", -5, 0, 0, 0},
		{"row beyond end", 100, 0, 1, 0},
		{"negative col", 0, -5, 0, 0},
		{"col beyond line length", 0, 100, 0, 3},
		{"col beyond line 1", 1, 100, 1, 3},
		{"valid position", 1, 2, 1, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _ = setPositionFn(ta, runtime.ToValue(tt.row), runtime.ToValue(tt.col))

			lineResult, _ := lineFn(ta)
			if lineResult.ToInteger() != tt.expectedRow {
				t.Errorf("expected row %d, got %d", tt.expectedRow, lineResult.ToInteger())
			}

			colResult, _ := colFn(ta)
			if colResult.ToInteger() != tt.expectedCol {
				t.Errorf("expected col %d, got %d", tt.expectedCol, colResult.ToInteger())
			}
		})
	}
}

// TestTextareaSetRow tests the setRow method.
func TestTextareaSetRow(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Set multi-line content
	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	_, _ = setValueFn(ta, runtime.ToValue("First\nSecond\nThird"))

	setRowFn, ok := goja.AssertFunction(ta.Get("setRow"))
	if !ok {
		t.Fatal("setRow is not a function")
	}

	lineFn, _ := goja.AssertFunction(ta.Get("line"))

	// Set to row 2
	_, _ = setRowFn(ta, runtime.ToValue(2))
	lineResult, _ := lineFn(ta)
	if lineResult.ToInteger() != 2 {
		t.Errorf("expected row 2, got %d", lineResult.ToInteger())
	}

	// Set to negative (should clamp to 0)
	_, _ = setRowFn(ta, runtime.ToValue(-1))
	lineResult, _ = lineFn(ta)
	if lineResult.ToInteger() != 0 {
		t.Errorf("expected row 0 after negative, got %d", lineResult.ToInteger())
	}

	// Set beyond end (should clamp to last row)
	_, _ = setRowFn(ta, runtime.ToValue(100))
	lineResult, _ = lineFn(ta)
	if lineResult.ToInteger() != 2 {
		t.Errorf("expected row 2 after overflow, got %d", lineResult.ToInteger())
	}
}

// TestTextareaSelectAll tests the selectAll method.
func TestTextareaSelectAll(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Set multi-line content
	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	_, _ = setValueFn(ta, runtime.ToValue("Line 1\nLine 2\nLine 3 end"))

	selectAllFn, ok := goja.AssertFunction(ta.Get("selectAll"))
	if !ok {
		t.Fatal("selectAll is not a function")
	}

	_, err := selectAllFn(ta)
	if err != nil {
		t.Fatalf("selectAll failed: %v", err)
	}

	// Verify cursor is at the last row
	lineFn, _ := goja.AssertFunction(ta.Get("line"))
	lineResult, _ := lineFn(ta)
	if lineResult.ToInteger() != 2 {
		t.Errorf("expected row 2 after selectAll, got %d", lineResult.ToInteger())
	}

	// Verify cursor is at end of last line
	colFn, _ := goja.AssertFunction(ta.Get("col"))
	colResult, _ := colFn(ta)
	// "Line 3 end" has length 10
	if colResult.ToInteger() != 10 {
		t.Errorf("expected col 10 after selectAll, got %d", colResult.ToInteger())
	}
}

// TestTextareaHandleClick tests the handleClick method.
func TestTextareaHandleClick(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Set content
	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	_, _ = setValueFn(ta, runtime.ToValue("ABCD\nEFGH\nIJKL"))

	handleClickFn, ok := goja.AssertFunction(ta.Get("handleClick"))
	if !ok {
		t.Fatal("handleClick is not a function")
	}

	lineFn, _ := goja.AssertFunction(ta.Get("line"))
	colFn, _ := goja.AssertFunction(ta.Get("col"))

	// Click at row 1 (display line 1 with yOffset 0), column 2
	_, err := handleClickFn(ta, runtime.ToValue(2), runtime.ToValue(1), runtime.ToValue(0))
	if err != nil {
		t.Fatalf("handleClick failed: %v", err)
	}

	lineResult, _ := lineFn(ta)
	if lineResult.ToInteger() != 1 {
		t.Errorf("expected row 1 after click, got %d", lineResult.ToInteger())
	}

	colResult, _ := colFn(ta)
	if colResult.ToInteger() != 2 {
		t.Errorf("expected col 2 after click, got %d", colResult.ToInteger())
	}
}

// TestTextareaCol tests the col method.
func TestTextareaCol(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Set content and position
	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	_, _ = setValueFn(ta, runtime.ToValue("Hello"))

	setCursorFn, _ := goja.AssertFunction(ta.Get("setCursor"))
	_, _ = setCursorFn(ta, runtime.ToValue(3))

	colFn, ok := goja.AssertFunction(ta.Get("col"))
	if !ok {
		t.Fatal("col is not a function")
	}

	colResult, err := colFn(ta)
	if err != nil {
		t.Fatalf("col() failed: %v", err)
	}

	if colResult.ToInteger() != 3 {
		t.Errorf("expected col 3, got %d", colResult.ToInteger())
	}
}

// TestTextareaLargeDocument tests cursor positioning in a large document (100+ lines).
// This is critical for ensuring the implementation works with scrolled content.
func TestTextareaLargeDocument(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Build a large document with 150 lines
	var content string
	for i := 0; i < 150; i++ {
		if i > 0 {
			content += "\n"
		}
		content += "Line " + string(rune('A'+i%26)) + " number " + string(rune('0'+i/100)) + string(rune('0'+(i/10)%10)) + string(rune('0'+i%10))
	}

	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	_, _ = setValueFn(ta, runtime.ToValue(content))

	// Verify line count
	lineCountFn, _ := goja.AssertFunction(ta.Get("lineCount"))
	lineCountResult, _ := lineCountFn(ta)
	if lineCountResult.ToInteger() != 150 {
		t.Fatalf("expected 150 lines, got %d", lineCountResult.ToInteger())
	}

	setPositionFn, _ := goja.AssertFunction(ta.Get("setPosition"))
	lineFn, _ := goja.AssertFunction(ta.Get("line"))
	colFn, _ := goja.AssertFunction(ta.Get("col"))

	tests := []struct {
		name        string
		row         int
		col         int
		expectedRow int64
		expectedCol int64
	}{
		{"first line", 0, 5, 0, 5},
		{"middle line", 75, 10, 75, 10},
		{"line 100", 100, 8, 100, 8},
		{"last line", 149, 5, 149, 5},
		{"beyond last line", 200, 5, 149, 5},
		{"col beyond line length", 50, 100, 50, 17}, // Each line is ~17-18 chars (variable)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _ = setPositionFn(ta, runtime.ToValue(tt.row), runtime.ToValue(tt.col))

			lineResult, _ := lineFn(ta)
			if lineResult.ToInteger() != tt.expectedRow {
				t.Errorf("expected row %d, got %d", tt.expectedRow, lineResult.ToInteger())
			}

			colResult, _ := colFn(ta)
			if colResult.ToInteger() != tt.expectedCol {
				t.Errorf("expected col %d, got %d", tt.expectedCol, colResult.ToInteger())
			}
		})
	}
}

// TestTextareaHandleClickWithScrollOffset tests handleClick with various scroll offsets.
// This simulates clicking on a large scrolled document.
func TestTextareaHandleClickWithScrollOffset(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Build a document with 50 lines
	var content string
	for i := 0; i < 50; i++ {
		if i > 0 {
			content += "\n"
		}
		content += "Line content here"
	}

	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	_, _ = setValueFn(ta, runtime.ToValue(content))

	handleClickFn, _ := goja.AssertFunction(ta.Get("handleClick"))
	lineFn, _ := goja.AssertFunction(ta.Get("line"))
	colFn, _ := goja.AssertFunction(ta.Get("col"))

	tests := []struct {
		name        string
		clickX      int
		clickY      int
		yOffset     int
		expectedRow int64
		expectedCol int64
	}{
		// No scroll: clicking display line 0 = actual line 0
		{"no scroll, click line 0", 5, 0, 0, 0, 5},
		{"no scroll, click line 5", 8, 5, 0, 5, 8},

		// Scrolled by 10 lines: clicking display line 0 = actual line 10
		{"scroll 10, click display line 0", 3, 0, 10, 10, 3},
		{"scroll 10, click display line 5", 7, 5, 10, 15, 7},

		// Scrolled by 30 lines: clicking display line 10 = actual line 40
		{"scroll 30, click display line 10", 12, 10, 30, 40, 12},

		// Edge case: click beyond content (should clamp to last line)
		{"scroll 45, click display line 10", 5, 10, 45, 49, 5}, // 45+10=55, but only 50 lines (0-49)

		// Column clamping
		{"click col beyond line length", 100, 0, 0, 0, 17}, // "Line content here" = 17 chars
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _ = handleClickFn(ta,
				runtime.ToValue(tt.clickX),
				runtime.ToValue(tt.clickY),
				runtime.ToValue(tt.yOffset))

			lineResult, _ := lineFn(ta)
			if lineResult.ToInteger() != tt.expectedRow {
				t.Errorf("expected row %d, got %d", tt.expectedRow, lineResult.ToInteger())
			}

			colResult, _ := colFn(ta)
			if colResult.ToInteger() != tt.expectedCol {
				t.Errorf("expected col %d, got %d", tt.expectedCol, colResult.ToInteger())
			}
		})
	}
}

// TestTextareaSetPositionAfterFocus tests that setPosition works correctly
// when the textarea is focused (the common case during user interaction).
func TestTextareaSetPositionAfterFocus(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Set content
	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	_, _ = setValueFn(ta, runtime.ToValue("First line\nSecond line\nThird line\nFourth line"))

	// Focus the textarea
	focusFn, _ := goja.AssertFunction(ta.Get("focus"))
	_, _ = focusFn(ta)

	// Verify focused
	focusedFn, _ := goja.AssertFunction(ta.Get("focused"))
	focusedResult, _ := focusedFn(ta)
	if !focusedResult.ToBoolean() {
		t.Error("textarea should be focused")
	}

	// Set position while focused
	setPositionFn, _ := goja.AssertFunction(ta.Get("setPosition"))
	_, _ = setPositionFn(ta, runtime.ToValue(2), runtime.ToValue(5))

	// Verify position
	lineFn, _ := goja.AssertFunction(ta.Get("line"))
	colFn, _ := goja.AssertFunction(ta.Get("col"))

	lineResult, _ := lineFn(ta)
	if lineResult.ToInteger() != 2 {
		t.Errorf("expected row 2, got %d", lineResult.ToInteger())
	}

	colResult, _ := colFn(ta)
	if colResult.ToInteger() != 5 {
		t.Errorf("expected col 5, got %d", colResult.ToInteger())
	}

	// Verify still focused (setPosition shouldn't change focus state)
	focusedResult, _ = focusedFn(ta)
	if !focusedResult.ToBoolean() {
		t.Error("textarea should still be focused after setPosition")
	}
}

// TestTextareaEmptyDocument tests handling of empty documents.
func TestTextareaEmptyDocument(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Don't set any value - document is empty

	setPositionFn, _ := goja.AssertFunction(ta.Get("setPosition"))
	lineFn, _ := goja.AssertFunction(ta.Get("line"))
	colFn, _ := goja.AssertFunction(ta.Get("col"))

	// Try to set position on empty document - should not crash
	_, err := setPositionFn(ta, runtime.ToValue(5), runtime.ToValue(10))
	if err != nil {
		t.Fatalf("setPosition on empty document failed: %v", err)
	}

	// Position should be at origin or clamped
	lineResult, _ := lineFn(ta)
	colResult, _ := colFn(ta)

	// For empty document, expect position 0,0
	if lineResult.ToInteger() != 0 {
		t.Errorf("expected row 0 for empty doc, got %d", lineResult.ToInteger())
	}
	if colResult.ToInteger() != 0 {
		t.Errorf("expected col 0 for empty doc, got %d", colResult.ToInteger())
	}
}

// TestTextareaHandleClickEmptyDocument tests handleClick on empty document.
func TestTextareaHandleClickEmptyDocument(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Empty document

	handleClickFn, _ := goja.AssertFunction(ta.Get("handleClick"))

	// Should not crash
	_, err := handleClickFn(ta, runtime.ToValue(5), runtime.ToValue(10), runtime.ToValue(0))
	if err != nil {
		t.Fatalf("handleClick on empty document failed: %v", err)
	}
}

// ============================================================================
// REGRESSION TESTS FOR SOFT-WRAPPING BUGS (from review.md)
// ============================================================================

// TestTextareaVisualLineCount tests the visualLineCount method which accounts
// for soft-wrapped lines. This is a regression test for the viewport clipping
// bug where bottom of wrapped documents was invisible.
func TestTextareaVisualLineCount(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Clear the default prompt and disable line numbers to get exact width control
	// IMPORTANT: setWidth must be called AFTER these to recalculate promptWidth
	setPromptFn, _ := goja.AssertFunction(ta.Get("setPrompt"))
	_, _ = setPromptFn(ta, runtime.ToValue(""))
	setShowLineNumbersFn, _ := goja.AssertFunction(ta.Get("setShowLineNumbers"))
	_, _ = setShowLineNumbersFn(ta, runtime.ToValue(false))

	// Set width for wrapping calculations
	setWidthFn, _ := goja.AssertFunction(ta.Get("setWidth"))
	_, _ = setWidthFn(ta, runtime.ToValue(20)) // Narrow width to force wrapping

	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	lineCountFn, _ := goja.AssertFunction(ta.Get("lineCount"))
	visualLineCountFn, ok := goja.AssertFunction(ta.Get("visualLineCount"))
	if !ok {
		t.Fatal("visualLineCount is not a function")
	}

	tests := []struct {
		name                   string
		content                string
		expectedLogicalLines   int64
		minExpectedVisualLines int64 // At least this many visual lines
	}{
		{
			name:                   "empty document",
			content:                "",
			expectedLogicalLines:   1, // bubbles textarea always has at least 1 line
			minExpectedVisualLines: 1,
		},
		{
			name:                   "short single line (no wrap)",
			content:                "Hello",
			expectedLogicalLines:   1,
			minExpectedVisualLines: 1,
		},
		{
			name:                   "long single line (wraps)",
			content:                "This is a very long line that should definitely wrap when the width is only 20 characters",
			expectedLogicalLines:   1,
			minExpectedVisualLines: 4, // Should wrap to at least 4 visual lines
		},
		{
			name:                   "multiple short lines",
			content:                "Line 1\nLine 2\nLine 3",
			expectedLogicalLines:   3,
			minExpectedVisualLines: 3,
		},
		{
			name:                   "multiple lines with wrapping",
			content:                "Short\nThis is a long line that will wrap multiple times\nAnother short line",
			expectedLogicalLines:   3,
			minExpectedVisualLines: 4, // Middle line should wrap
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _ = setValueFn(ta, runtime.ToValue(tt.content))

			logicalResult, _ := lineCountFn(ta)
			if logicalResult.ToInteger() != tt.expectedLogicalLines {
				t.Errorf("expected logical lines %d, got %d", tt.expectedLogicalLines, logicalResult.ToInteger())
			}

			visualResult, _ := visualLineCountFn(ta)
			if visualResult.ToInteger() < tt.minExpectedVisualLines {
				t.Errorf("expected at least %d visual lines, got %d", tt.minExpectedVisualLines, visualResult.ToInteger())
			}

			// Visual lines should always be >= logical lines
			if visualResult.ToInteger() < logicalResult.ToInteger() {
				t.Errorf("visual lines (%d) should be >= logical lines (%d)", visualResult.ToInteger(), logicalResult.ToInteger())
			}
		})
	}
}

// TestTextareaPerformHitTest tests the performHitTest method which maps visual
// coordinates to logical row/column. This is a regression test for the cursor
// jump bug where clicking on wrapped text placed cursor in wrong position.
func TestTextareaPerformHitTest(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Clear the prompt and disable line numbers to get predictable content width
	// IMPORTANT: setWidth must be called AFTER these to recalculate promptWidth
	setPromptFn, _ := goja.AssertFunction(ta.Get("setPrompt"))
	_, _ = setPromptFn(ta, runtime.ToValue(""))
	setShowLineNumbersFn, _ := goja.AssertFunction(ta.Get("setShowLineNumbers"))
	_, _ = setShowLineNumbersFn(ta, runtime.ToValue(false))

	// Set narrow width to force wrapping
	// With prompt cleared and line numbers disabled, content width = total width
	setWidthFn, _ := goja.AssertFunction(ta.Get("setWidth"))
	_, _ = setWidthFn(ta, runtime.ToValue(10)) // Content width = 10

	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	performHitTestFn, ok := goja.AssertFunction(ta.Get("performHitTest"))
	if !ok {
		t.Fatal("performHitTest is not a function")
	}

	// Set content that will wrap:
	// "ABCDEFGHIJKLMNOPQRST" (20 chars) at width 10 = 2 visual lines
	// "XYZ" (3 chars) = 1 visual line
	// Total: 2 logical lines, 3 visual lines
	_, _ = setValueFn(ta, runtime.ToValue("ABCDEFGHIJKLMNOPQRST\nXYZ"))

	tests := []struct {
		name        string
		visualX     int
		visualY     int
		expectedRow int64
		expectedCol int64
	}{
		{
			name:        "first char of first line",
			visualX:     0,
			visualY:     0,
			expectedRow: 0,
			expectedCol: 0,
		},
		{
			name:        "middle of first visual line (unwrapped part)",
			visualX:     5,
			visualY:     0,
			expectedRow: 0,
			expectedCol: 5,
		},
		{
			name:        "first char of wrapped continuation (visual line 1)",
			visualX:     0,
			visualY:     1,
			expectedRow: 0,  // Still logical line 0
			expectedCol: 10, // Column 10 (start of wrapped segment)
		},
		{
			name:        "middle of wrapped continuation",
			visualX:     5,
			visualY:     1,
			expectedRow: 0,  // Still logical line 0
			expectedCol: 15, // Column 15
		},
		{
			name:        "first char of second logical line (visual line 2)",
			visualX:     0,
			visualY:     2,
			expectedRow: 1, // Logical line 1
			expectedCol: 0,
		},
		{
			name:        "end of second logical line",
			visualX:     3,
			visualY:     2,
			expectedRow: 1,
			expectedCol: 3,
		},
		{
			name:        "beyond end of document (clamps)",
			visualX:     5,
			visualY:     10,
			expectedRow: 1, // Clamped to last line
			expectedCol: 3, // Clamped to line length
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hitResultVal, err := performHitTestFn(ta, runtime.ToValue(tt.visualX), runtime.ToValue(tt.visualY))
			if err != nil {
				t.Fatalf("performHitTest failed: %v", err)
			}

			hitResult := hitResultVal.ToObject(runtime)
			row := hitResult.Get("row").ToInteger()
			col := hitResult.Get("col").ToInteger()

			if row != tt.expectedRow {
				t.Errorf("expected row %d, got %d", tt.expectedRow, row)
			}
			if col != tt.expectedCol {
				t.Errorf("expected col %d, got %d", tt.expectedCol, col)
			}
		})
	}
}

// TestTextareaHandleClickWithSoftWrap tests that handleClick correctly positions
// the cursor when clicking on soft-wrapped lines. This is a critical regression
// test for the cursor jump bug identified in review.md.
func TestTextareaHandleClickWithSoftWrap(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Clear the default prompt and disable line numbers to get exact width control
	// IMPORTANT: setWidth must be called AFTER these to recalculate promptWidth
	setPromptFn, _ := goja.AssertFunction(ta.Get("setPrompt"))
	_, _ = setPromptFn(ta, runtime.ToValue(""))
	setShowLineNumbersFn, _ := goja.AssertFunction(ta.Get("setShowLineNumbers"))
	_, _ = setShowLineNumbersFn(ta, runtime.ToValue(false))

	// Set narrow width to force wrapping
	setWidthFn, _ := goja.AssertFunction(ta.Get("setWidth"))
	_, _ = setWidthFn(ta, runtime.ToValue(10))

	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	handleClickFn, _ := goja.AssertFunction(ta.Get("handleClick"))
	lineFn, _ := goja.AssertFunction(ta.Get("line"))
	colFn, _ := goja.AssertFunction(ta.Get("col"))

	// Content with wrapping: "ABCDEFGHIJKLMNOPQRST" wraps at width 10
	_, _ = setValueFn(ta, runtime.ToValue("ABCDEFGHIJKLMNOPQRST\nXYZ"))

	tests := []struct {
		name        string
		clickX      int
		clickY      int // Visual line (0 = first display line)
		yOffset     int
		expectedRow int64
		expectedCol int64
	}{
		{
			name:        "click first visual line",
			clickX:      3,
			clickY:      0,
			yOffset:     0,
			expectedRow: 0,
			expectedCol: 3,
		},
		{
			name:        "click wrapped continuation (visual line 1)",
			clickX:      3,
			clickY:      1,
			yOffset:     0,
			expectedRow: 0,  // CRITICAL: Should stay in logical line 0!
			expectedCol: 13, // 10 (start of wrap) + 3
		},
		{
			name:        "click second logical line (visual line 2)",
			clickX:      1,
			clickY:      2,
			yOffset:     0,
			expectedRow: 1,
			expectedCol: 1,
		},
		{
			name:        "click with scroll offset",
			clickX:      2,
			clickY:      0,  // Visual line 0 of viewport
			yOffset:     1,  // Scrolled down by 1, so visual line 1 is at top
			expectedRow: 0,  // Wrapped part of line 0
			expectedCol: 12, // 10 + 2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handleClickFn(ta,
				runtime.ToValue(tt.clickX),
				runtime.ToValue(tt.clickY),
				runtime.ToValue(tt.yOffset))
			if err != nil {
				t.Fatalf("handleClick failed: %v", err)
			}

			lineResult, _ := lineFn(ta)
			if lineResult.ToInteger() != tt.expectedRow {
				t.Errorf("expected row %d, got %d (BUG: cursor jumped to wrong line!)", tt.expectedRow, lineResult.ToInteger())
			}

			colResult, _ := colFn(ta)
			if colResult.ToInteger() != tt.expectedCol {
				t.Errorf("expected col %d, got %d", tt.expectedCol, colResult.ToInteger())
			}
		})
	}
}

// TestTextareaMultiWidthCharacters tests handling of CJK and emoji characters
// which occupy 2 visual cells but 1 rune index.
func TestTextareaMultiWidthCharacters(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Clear the default prompt and disable line numbers to get exact width control
	// IMPORTANT: setWidth must be called AFTER these to recalculate promptWidth
	setPromptFn, _ := goja.AssertFunction(ta.Get("setPrompt"))
	_, _ = setPromptFn(ta, runtime.ToValue(""))
	setShowLineNumbersFn, _ := goja.AssertFunction(ta.Get("setShowLineNumbers"))
	_, _ = setShowLineNumbersFn(ta, runtime.ToValue(false))

	setWidthFn, _ := goja.AssertFunction(ta.Get("setWidth"))
	_, _ = setWidthFn(ta, runtime.ToValue(20))

	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	visualLineCountFn, _ := goja.AssertFunction(ta.Get("visualLineCount"))

	// Test content with CJK characters (each takes 2 cells)
	// "你好世界" = 4 characters, 8 visual cells
	_, _ = setValueFn(ta, runtime.ToValue("你好世界"))

	visualResult, _ := visualLineCountFn(ta)
	// At width 20, 8 cells should fit in 1 line
	if visualResult.ToInteger() != 1 {
		t.Errorf("expected 1 visual line for CJK text at width 20, got %d", visualResult.ToInteger())
	}

	// Now test at narrower width where it must wrap
	_, _ = setWidthFn(ta, runtime.ToValue(6))
	visualResult, _ = visualLineCountFn(ta)
	// 8 cells at width 6 = ceil(8/6) = 2 visual lines
	if visualResult.ToInteger() < 2 {
		t.Errorf("expected at least 2 visual lines for CJK text at width 6, got %d", visualResult.ToInteger())
	}
}

// TestSuperDocumentViewportAlignment tests that visualLineCount and performHitTest
// work correctly under PRODUCTION CONDITIONS (with line numbers enabled and
// default prompt intact). This is a regression test for the viewport shaking/ghosting
// issue identified by Hana-sama where the Go and JS calculations drifted.
//
// The key insight: In production, we have:
// - ShowLineNumbers = true
// - Default prompt = "┃ " (2 chars)
// - Line numbers add variable width (e.g., " 1 " = 4 chars for 1-9 lines)
//
// The textarea internally calculates:
// - promptWidth = len(prompt) + lineNumberWidth
// - contentWidth = width - promptWidth
//
// If we set width=40 with ShowLineNumbers=true and default prompt:
// - promptWidth = 2 (prompt) + 4 (line nums) = 6
// - contentWidth = 40 - 6 = 34
//
// A line of 68 characters should wrap into 2 visual lines (68/34 = 2).
func TestSuperDocumentViewportAlignment(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// PRODUCTION CONDITIONS: DO NOT clear prompt, DO enable line numbers
	// This mirrors what super_document_script.js does
	setShowLineNumbersFn, _ := goja.AssertFunction(ta.Get("setShowLineNumbers"))
	_, _ = setShowLineNumbersFn(ta, runtime.ToValue(true))

	// Set width - must be after ShowLineNumbers to get correct promptWidth calculation
	setWidthFn, _ := goja.AssertFunction(ta.Get("setWidth"))
	_, _ = setWidthFn(ta, runtime.ToValue(40)) // Typical narrow terminal width

	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	visualLineCountFn, _ := goja.AssertFunction(ta.Get("visualLineCount"))
	lineCountFn, _ := goja.AssertFunction(ta.Get("lineCount"))

	// Test 1: A single long line that wraps
	// With default prompt "┃ " (2 chars) and line numbers (4 chars), promptWidth = 6
	// So contentWidth = 40 - 6 = 34
	// A 68-char line should wrap into 2 visual lines
	longLine := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789ABCDEF" // 68 chars
	_, _ = setValueFn(ta, runtime.ToValue(longLine))

	logicalCount, _ := lineCountFn(ta)
	visualCount, _ := visualLineCountFn(ta)

	if logicalCount.ToInteger() != 1 {
		t.Errorf("Expected 1 logical line, got %d", logicalCount.ToInteger())
	}

	// Visual lines should be >= 2 (68 chars / 34 content width = 2 visual lines)
	if visualCount.ToInteger() < 2 {
		t.Errorf("Expected at least 2 visual lines for 68-char line at width 40, got %d (promptWidth may be calculated incorrectly)", visualCount.ToInteger())
	}

	// Test 2: Multiple lines with wrapping
	multiLineContent := longLine + "\n" + longLine + "\nShort line"
	_, _ = setValueFn(ta, runtime.ToValue(multiLineContent))

	logicalCount, _ = lineCountFn(ta)
	visualCount, _ = visualLineCountFn(ta)

	if logicalCount.ToInteger() != 3 {
		t.Errorf("Expected 3 logical lines, got %d", logicalCount.ToInteger())
	}

	// Visual lines: 2 (first long) + 2 (second long) + 1 (short) = 5
	if visualCount.ToInteger() < 5 {
		t.Errorf("Expected at least 5 visual lines, got %d", visualCount.ToInteger())
	}

	// Test 3: Verify performHitTest with production conditions
	performHitTestFn, ok := goja.AssertFunction(ta.Get("performHitTest"))
	if !ok {
		t.Fatal("performHitTest is not a function")
	}

	// Click on the wrapped continuation of the first line (visual line 1)
	hitResult, err := performHitTestFn(ta, runtime.ToValue(5), runtime.ToValue(1))
	if err != nil {
		t.Fatalf("performHitTest failed: %v", err)
	}

	row := hitResult.ToObject(runtime).Get("row").ToInteger()
	col := hitResult.ToObject(runtime).Get("col").ToInteger()

	// Should still be in logical row 0, but column should be >= 34 (start of wrapped segment + click offset)
	if row != 0 {
		t.Errorf("VIEWPORT ALIGNMENT BUG: Click on wrapped line mapped to wrong logical row. Expected row 0, got %d", row)
	}

	// Column should be somewhere in the 34-40 range (wrapped segment start + visual X)
	// contentWidth ~= 34, so visual line 1 starts at column 34, and visualX=5 means column 39
	expectedMinCol := int64(30) // Be lenient due to promptWidth calculation variations
	if col < expectedMinCol {
		t.Errorf("VIEWPORT ALIGNMENT BUG: Click column %d is too low, expected >= %d", col, expectedMinCol)
	}
}

// TestViewportDoubleCounting specifically tests for the double-counting bug
// where border/padding/prompt offsets might be counted twice between JS and Go.
func TestViewportDoubleCounting(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Set up exactly like production code in configureTextarea()
	setShowLineNumbersFn, _ := goja.AssertFunction(ta.Get("setShowLineNumbers"))
	_, _ = setShowLineNumbersFn(ta, runtime.ToValue(true))

	// Simulate the JS calculation:
	// fieldWidth = max(40, termWidth - 10) = let's say 60
	// innerWidth = fieldWidth - 4 - scrollbarWidth = 60 - 4 - 1 = 55
	// setWidth(innerWidth)
	setWidthFn, _ := goja.AssertFunction(ta.Get("setWidth"))
	innerWidth := 55
	_, _ = setWidthFn(ta, runtime.ToValue(innerWidth))

	// Now visualLineCount should use:
	// contentWidth = width - promptWidth = 55 - 6 = 49 (assuming prompt=2, lineNums=4)
	//
	// If the JS then ALSO subtracts prompt/lineNum width when calculating bounds,
	// there would be a double-count.

	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	visualLineCountFn, _ := goja.AssertFunction(ta.Get("visualLineCount"))

	// Create content that would wrap differently depending on contentWidth
	// At contentWidth=49, a 100-char line wraps into 3 visual lines (100/49 = ceil(2.04) = 3)
	// At contentWidth=55, a 100-char line wraps into 2 visual lines (100/55 = ceil(1.82) = 2)
	hundredChars := "0123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789"
	_, _ = setValueFn(ta, runtime.ToValue(hundredChars))

	visualCount, _ := visualLineCountFn(ta)

	// With proper calculation (contentWidth = 55 - 6 = 49), we expect 3 visual lines
	// If there's double-counting, we might get more or fewer lines
	expectedVisualLines := int64(3) // ceil(100/49) = 3

	if visualCount.ToInteger() != expectedVisualLines {
		t.Errorf("Potential double-counting bug: Expected %d visual lines, got %d. "+
			"This suggests contentWidth calculation drift between Go and JS.",
			expectedVisualLines, visualCount.ToInteger())
	}
}

// TestCursorVisualLine tests the cursorVisualLine method which returns the visual
// line index where the cursor is located, accounting for soft-wrapping.
// This is a regression test for the viewport shaking/stuttering bug where using
// line() (logical) instead of cursorVisualLine() (visual) caused the viewport
// to track the wrong position.
func TestCursorVisualLine(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Clear prompt and disable line numbers for predictable width
	setPromptFn, _ := goja.AssertFunction(ta.Get("setPrompt"))
	_, _ = setPromptFn(ta, runtime.ToValue(""))
	setShowLineNumbersFn, _ := goja.AssertFunction(ta.Get("setShowLineNumbers"))
	_, _ = setShowLineNumbersFn(ta, runtime.ToValue(false))

	// Set narrow width to force wrapping
	setWidthFn, _ := goja.AssertFunction(ta.Get("setWidth"))
	_, _ = setWidthFn(ta, runtime.ToValue(10)) // Content width = 10

	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	setPositionFn, _ := goja.AssertFunction(ta.Get("setPosition"))
	cursorVisualLineFn, ok := goja.AssertFunction(ta.Get("cursorVisualLine"))
	if !ok {
		t.Fatal("cursorVisualLine is not a function")
	}
	lineFn, _ := goja.AssertFunction(ta.Get("line"))

	// Content: "ABCDEFGHIJKLMNOPQRST" (20 chars at width 10 = 2 visual lines)
	// Then "XYZ" on second logical line
	_, _ = setValueFn(ta, runtime.ToValue("ABCDEFGHIJKLMNOPQRST\nXYZ"))

	tests := []struct {
		name               string
		row                int
		col                int
		expectedLogicalLine int64
		expectedVisualLine  int64
	}{
		{
			name:               "cursor at start of first logical line",
			row:                0,
			col:                0,
			expectedLogicalLine: 0,
			expectedVisualLine:  0,
		},
		{
			name:               "cursor at end of first visual segment (col 9)",
			row:                0,
			col:                9,
			expectedLogicalLine: 0,
			expectedVisualLine:  0,
		},
		{
			name:               "cursor at start of wrapped segment (col 10)",
			row:                0,
			col:                10,
			expectedLogicalLine: 0, // Still logical line 0
			expectedVisualLine:  1, // But visual line 1 (wrapped)
		},
		{
			name:               "cursor at end of wrapped segment (col 19)",
			row:                0,
			col:                19,
			expectedLogicalLine: 0,
			expectedVisualLine:  1,
		},
		{
			name:               "cursor at second logical line (visual line 2)",
			row:                1,
			col:                0,
			expectedLogicalLine: 1,
			expectedVisualLine:  2, // First logical line takes 2 visual lines
		},
		{
			name:               "cursor at middle of second logical line",
			row:                1,
			col:                2,
			expectedLogicalLine: 1,
			expectedVisualLine:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _ = setPositionFn(ta, runtime.ToValue(tt.row), runtime.ToValue(tt.col))

			logicalResult, _ := lineFn(ta)
			if logicalResult.ToInteger() != tt.expectedLogicalLine {
				t.Errorf("expected logical line %d, got %d", tt.expectedLogicalLine, logicalResult.ToInteger())
			}

			visualResult, _ := cursorVisualLineFn(ta)
			if visualResult.ToInteger() != tt.expectedVisualLine {
				t.Errorf("VIEWPORT SHAKING BUG: expected cursor visual line %d, got %d. "+
					"Using line() instead of cursorVisualLine() causes viewport tracking errors.",
					tt.expectedVisualLine, visualResult.ToInteger())
			}
		})
	}
}
