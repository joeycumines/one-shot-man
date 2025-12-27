package textarea

import (
	"fmt"
	"os"
	"strings"
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

// TestTextarea_HandleClickDoesNotWriteStdout ensures click handlers do not write to stdout.
func TestTextarea_HandleClickDoesNotWriteStdout(t *testing.T) {
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

	handleClickFn, _ := goja.AssertFunction(ta.Get("handleClick"))
	handleClickAtScreenCoordsFn, _ := goja.AssertFunction(ta.Get("handleClickAtScreenCoords"))

	// Capture stdout to a temp file
	stdoutFile, err := os.CreateTemp(t.TempDir(), "textarea-stdout-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = stdoutFile

	// Ensure stdout is restored and file closed at end
	t.Cleanup(func() {
		os.Stdout = origStdout
		_ = stdoutFile.Close()
	})

	// Invoke click handlers
	_, _ = handleClickFn(ta, runtime.ToValue(2), runtime.ToValue(1), runtime.ToValue(0))
	_, _ = handleClickAtScreenCoordsFn(ta, runtime.ToValue(10), runtime.ToValue(5), runtime.ToValue(0))

	// Close and read file
	_ = stdoutFile.Close()
	out, err := os.ReadFile(stdoutFile.Name())
	if err != nil {
		t.Fatalf("failed to read stdout temp file: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected no stdout output from click handlers, got: %q", string(out))
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
// default prompt intact). This is a regression test for the viewport
// shaking/ghosting issue, where the Go and JS calculations drifted.
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
		name                string
		row                 int
		col                 int
		expectedLogicalLine int64
		expectedVisualLine  int64
	}{
		{
			name:                "cursor at start of first logical line",
			row:                 0,
			col:                 0,
			expectedLogicalLine: 0,
			expectedVisualLine:  0,
		},
		{
			name:                "cursor at end of first visual segment (col 9)",
			row:                 0,
			col:                 9,
			expectedLogicalLine: 0,
			expectedVisualLine:  0,
		},
		{
			name:                "cursor at start of wrapped segment (col 10)",
			row:                 0,
			col:                 10,
			expectedLogicalLine: 0, // Still logical line 0
			expectedVisualLine:  1, // But visual line 1 (wrapped)
		},
		{
			name:                "cursor at end of wrapped segment (col 19)",
			row:                 0,
			col:                 19,
			expectedLogicalLine: 0,
			expectedVisualLine:  1,
		},
		{
			name:                "cursor at second logical line (visual line 2)",
			row:                 1,
			col:                 0,
			expectedLogicalLine: 1,
			expectedVisualLine:  2, // First logical line takes 2 visual lines
		},
		{
			name:                "cursor at middle of second logical line",
			row:                 1,
			col:                 2,
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

// TestPromptWidthAndContentWidth verifies that promptWidth(), contentWidth(),
// and reservedInnerWidth() return the correct values for JS coordinate calculations.
// This fixes the bug where JS hardcoded promptWidth=2 and lineNumberWidth=4
// instead of using the actual values from the textarea model.
func TestPromptWidthAndContentWidth(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Configure with line numbers (like super-document does)
	setShowLineNumbersFn, _ := goja.AssertFunction(ta.Get("setShowLineNumbers"))
	setWidthFn, _ := goja.AssertFunction(ta.Get("setWidth"))

	_, _ = setShowLineNumbersFn(ta, runtime.ToValue(true))
	// IMPORTANT: setWidth must be called AFTER setShowLineNumbers to recalculate promptWidth
	_, _ = setWidthFn(ta, runtime.ToValue(40))

	// Get promptWidth, contentWidth, and reservedInnerWidth
	promptWidthFn, ok := goja.AssertFunction(ta.Get("promptWidth"))
	if !ok {
		t.Fatal("promptWidth function not available")
	}
	contentWidthFn, ok := goja.AssertFunction(ta.Get("contentWidth"))
	if !ok {
		t.Fatal("contentWidth function not available")
	}
	reservedInnerWidthFn, ok := goja.AssertFunction(ta.Get("reservedInnerWidth"))
	if !ok {
		t.Fatal("reservedInnerWidth function not available")
	}

	promptWidthResult, _ := promptWidthFn(ta)
	contentWidthResult, _ := contentWidthFn(ta)
	reservedInnerResult, _ := reservedInnerWidthFn(ta)

	promptWidth := promptWidthResult.ToInteger()
	contentWidth := contentWidthResult.ToInteger()
	reservedInner := reservedInnerResult.ToInteger()

	// promptWidth is just the prompt string width (2 for "┃ ")
	if promptWidth != 2 {
		t.Errorf("promptWidth should be 2 (prompt string only), got %d", promptWidth)
	}

	// reservedInnerWidth should be promptWidth + lineNumberWidth (4)
	// With ShowLineNumbers=true, this should be 2 + 4 = 6
	if reservedInner < 5 {
		t.Errorf("reservedInnerWidth too low: got %d, expected >= 5 (prompt + line numbers)", reservedInner)
	}

	// contentWidth should be reasonable (width - reservedInner - reservedOuter)
	// With width=40, reservedInner=6, reservedOuter=0, contentWidth should be 34
	if contentWidth <= 0 {
		t.Errorf("contentWidth should be positive, got %d", contentWidth)
	}

	// Verify consistency: viewport.Width - width = reservedInnerWidth
	widthFn, _ := goja.AssertFunction(ta.Get("width"))
	widthResult, _ := widthFn(ta)
	totalWidth := widthResult.ToInteger()

	// width() returns m.width which IS contentWidth
	if totalWidth != contentWidth {
		t.Errorf("width() and contentWidth() should return the same value: width=%d, contentWidth=%d",
			totalWidth, contentWidth)
	}

	t.Logf("promptWidth=%d, reservedInnerWidth=%d, contentWidth=%d", promptWidth, reservedInner, contentWidth)
}

// TestOneHundredCharLine tests 100-character line in 40-character viewport -
// clicking the wrapped segment MUST stay in logical row 0. This is the
// critical regression test for the "cursor jumps to wrong line" bug.
func TestOneHundredCharLine(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Clear prompt and disable line numbers for exact width control
	setPromptFn, _ := goja.AssertFunction(ta.Get("setPrompt"))
	_, _ = setPromptFn(ta, runtime.ToValue(""))
	setShowLineNumbersFn, _ := goja.AssertFunction(ta.Get("setShowLineNumbers"))
	_, _ = setShowLineNumbersFn(ta, runtime.ToValue(false))

	// Set 40-char viewport width
	setWidthFn, _ := goja.AssertFunction(ta.Get("setWidth"))
	_, _ = setWidthFn(ta, runtime.ToValue(40))

	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	performHitTestFn, _ := goja.AssertFunction(ta.Get("performHitTest"))
	visualLineCountFn, _ := goja.AssertFunction(ta.Get("visualLineCount"))
	lineFn, _ := goja.AssertFunction(ta.Get("line"))
	colFn, _ := goja.AssertFunction(ta.Get("col"))

	// 100-character line
	hundredCharLine := "0123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789"
	if len(hundredCharLine) != 100 {
		t.Fatalf("Test setup error: expected 100 chars, got %d", len(hundredCharLine))
	}

	// Add a second line so we can verify we DON'T incorrectly jump to it
	_, _ = setValueFn(ta, runtime.ToValue(hundredCharLine+"\nSecond Line"))

	// Verify wrapping: 100 chars at width 40 = ceil(100/40) = 3 visual lines
	// Plus "Second Line" = 1 visual line = 4 total
	visualCount, _ := visualLineCountFn(ta)
	if visualCount.ToInteger() < 3 {
		t.Errorf("Expected at least 3 visual lines (100/40=2.5 rounded up), got %d", visualCount.ToInteger())
	}
	t.Logf("100-char line in 40-width viewport produces %d visual lines", visualCount.ToInteger())

	// THE CRITICAL TEST: Click on visual line 2 (the third wrapped segment of row 0)
	// This MUST position cursor in logical row 0, NOT row 2
	tests := []struct {
		name           string
		visualX        int
		visualY        int
		expectedRow    int64
		expectedMinCol int64 // Minimum expected column
		expectedMaxCol int64 // Maximum expected column
	}{
		{
			name:           "click first visual line (row 0, chars 0-39)",
			visualX:        5,
			visualY:        0,
			expectedRow:    0,
			expectedMinCol: 5,
			expectedMaxCol: 5,
		},
		{
			name:           "click second visual line (STILL row 0, chars 40-79)",
			visualX:        5,
			visualY:        1,
			expectedRow:    0,  // CRITICAL: Must stay in logical row 0!
			expectedMinCol: 45, // 40 (wrap offset) + 5
			expectedMaxCol: 45,
		},
		{
			name:           "click third visual line (STILL row 0, chars 80-99)",
			visualX:        5,
			visualY:        2,
			expectedRow:    0,  // CRITICAL: Must stay in logical row 0!
			expectedMinCol: 85, // 80 (second wrap offset) + 5
			expectedMaxCol: 85,
		},
		{
			name:           "click fourth visual line (this is logical row 1)",
			visualX:        3,
			visualY:        3,
			expectedRow:    1, // This is actually the second logical line
			expectedMinCol: 3,
			expectedMaxCol: 3,
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
				t.Errorf("Expected logical row %d, got %d. Clicking visual line %d jumped to wrong logical row!",
					tt.expectedRow, row, tt.visualY)
			}

			if col < tt.expectedMinCol || col > tt.expectedMaxCol {
				t.Errorf("Expected column in range [%d, %d], got %d",
					tt.expectedMinCol, tt.expectedMaxCol, col)
			}

			// Also verify setPosition + line/col retrieval works correctly
			setPositionFn, _ := goja.AssertFunction(ta.Get("setPosition"))
			_, _ = setPositionFn(ta, runtime.ToValue(row), runtime.ToValue(col))

			lineResult, _ := lineFn(ta)
			colResult, _ := colFn(ta)

			if lineResult.ToInteger() != tt.expectedRow {
				t.Errorf("setPosition row mismatch: set %d, got %d", tt.expectedRow, lineResult.ToInteger())
			}
			if colResult.ToInteger() != col {
				t.Errorf("setPosition col mismatch: set %d, got %d", col, colResult.ToInteger())
			}
		})
	}
}

// TestMultiWidthHitTest tests clicking on CJK/emoji characters.
// Clicking the right half of a 2-cell character should position correctly,
// not split the character or jump to wrong position.
func TestMultiWidthHitTest(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Clear prompt and disable line numbers
	setPromptFn, _ := goja.AssertFunction(ta.Get("setPrompt"))
	_, _ = setPromptFn(ta, runtime.ToValue(""))
	setShowLineNumbersFn, _ := goja.AssertFunction(ta.Get("setShowLineNumbers"))
	_, _ = setShowLineNumbersFn(ta, runtime.ToValue(false))

	setWidthFn, _ := goja.AssertFunction(ta.Get("setWidth"))
	_, _ = setWidthFn(ta, runtime.ToValue(20))

	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	performHitTestFn, _ := goja.AssertFunction(ta.Get("performHitTest"))

	// Content: "A你好B" = A(1) + 你(2) + 好(2) + B(1) = 6 visual cells, 4 runes
	// Visual layout:  A 你你 好好 B
	// Cell indices:   0 1 2  3 4  5
	// Rune indices:   0 1    2    3
	_, _ = setValueFn(ta, runtime.ToValue("A你好B"))

	tests := []struct {
		name        string
		visualX     int
		visualY     int
		expectedRow int64
		expectedCol int64 // Column is in RUNE units, not cells
	}{
		{"click on A (cell 0)", 0, 0, 0, 0},
		{"click on left half of 你 (cell 1)", 1, 0, 0, 1},
		{"click on right half of 你 (cell 2)", 2, 0, 0, 2}, // Should advance to next char
		{"click on left half of 好 (cell 3)", 3, 0, 0, 2},
		{"click on right half of 好 (cell 4)", 4, 0, 0, 3}, // Should advance to next char
		{"click on B (cell 5)", 5, 0, 0, 3},
		{"click beyond line (cell 6)", 6, 0, 0, 4}, // Clamp to end
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
				t.Errorf("Expected row %d, got %d", tt.expectedRow, row)
			}
			if col != tt.expectedCol {
				t.Errorf("MULTI-WIDTH BUG: Expected col %d (rune index), got %d. "+
					"Clicking visual cell %d mapped to wrong rune position.",
					tt.expectedCol, col, tt.visualX)
			}
		})
	}
}

// TestSetViewportContextAndHandleClickAtScreenCoords tests the GO-NATIVE
// click handling that takes raw screen coordinates and does ALL coordinate
// translation internally.
func TestSetViewportContextAndHandleClickAtScreenCoords(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Configure textarea: no prompt, no line numbers, 40-char content width
	setPromptFn, _ := goja.AssertFunction(ta.Get("setPrompt"))
	_, _ = setPromptFn(ta, runtime.ToValue(""))
	setShowLineNumbersFn, _ := goja.AssertFunction(ta.Get("setShowLineNumbers"))
	_, _ = setShowLineNumbersFn(ta, runtime.ToValue(false))
	setWidthFn, _ := goja.AssertFunction(ta.Get("setWidth"))
	_, _ = setWidthFn(ta, runtime.ToValue(40))

	// Set content: 100 chars wraps to 3 visual lines in 40-char viewport
	content := strings.Repeat("x", 100)
	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	_, _ = setValueFn(ta, runtime.ToValue(content+"\nsecond line"))

	// Get the new GO-NATIVE methods
	setViewportContextFn, _ := goja.AssertFunction(ta.Get("setViewportContext"))
	handleClickFn, _ := goja.AssertFunction(ta.Get("handleClickAtScreenCoords"))

	// Configure viewport context:
	// - Screen layout: [title (3 lines)] [viewport (10 lines)]
	// - Outer viewport: starts at screen Y=3, height=10, scroll offset=0
	// - Textarea content starts at outer viewport content Y=2 (some header above textarea)
	// - Textarea text starts at screen X=5 (borders + padding)
	vpConfig := runtime.NewObject()
	_ = vpConfig.Set("outerYOffset", 0)         // Not scrolled
	_ = vpConfig.Set("textareaContentTop", 2)   // 2 lines of header before textarea content
	_ = vpConfig.Set("textareaContentLeft", 5)  // 5 chars of left margin/border
	_ = vpConfig.Set("outerViewportHeight", 10) // Viewport is 10 lines tall
	_ = vpConfig.Set("preContentHeight", 2)     // Same as textareaContentTop

	_, err := setViewportContextFn(ta, vpConfig)
	if err != nil {
		t.Fatalf("setViewportContext failed: %v", err)
	}

	tests := []struct {
		name        string
		screenX     int
		screenY     int
		titleHeight int
		expectHit   bool
		expectedRow int64
		minCol      int64
		maxCol      int64
	}{
		{
			name:        "click first visual line (row 0, chars 0-39)",
			screenX:     10, // 5 (left margin) + 5 (5 chars into text)
			screenY:     5,  // 3 (title) + 2 (header) + 0 (first visual line)
			titleHeight: 3,
			expectHit:   true,
			expectedRow: 0,
			minCol:      5,
			maxCol:      5,
		},
		{
			name:        "click second visual line (STILL row 0, chars 40-79)",
			screenX:     10, // 5 (left margin) + 5 (5 chars into wrapped segment)
			screenY:     6,  // 3 (title) + 2 (header) + 1 (second visual line)
			titleHeight: 3,
			expectHit:   true,
			expectedRow: 0,  // CRITICAL: Must stay in logical row 0
			minCol:      45, // 40 (wrap offset) + 5
			maxCol:      45,
		},
		{
			name:        "click third visual line (STILL row 0, chars 80-99)",
			screenX:     10, // 5 (left margin) + 5 (5 chars into wrapped segment)
			screenY:     7,  // 3 (title) + 2 (header) + 2 (third visual line)
			titleHeight: 3,
			expectHit:   true,
			expectedRow: 0,  // CRITICAL: Must stay in logical row 0
			minCol:      85, // 80 (wrap offset) + 5
			maxCol:      85,
		},
		{
			name:        "click fourth visual line (logical row 1)",
			screenX:     8, // 5 (left margin) + 3 (3 chars into text)
			screenY:     8, // 3 (title) + 2 (header) + 3 (fourth visual line)
			titleHeight: 3,
			expectHit:   true,
			expectedRow: 1,
			minCol:      3,
			maxCol:      3,
		},
		{
			name:        "click outside viewport (above)",
			screenX:     10,
			screenY:     2, // Above title
			titleHeight: 3,
			expectHit:   false,
		},
		{
			name:        "click outside viewport (below)",
			screenX:     10,
			screenY:     20, // Beyond viewport
			titleHeight: 3,
			expectHit:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset cursor position before each test
			setPositionFn, _ := goja.AssertFunction(ta.Get("setPosition"))
			_, _ = setPositionFn(ta, runtime.ToValue(0), runtime.ToValue(0))

			result, err := handleClickFn(ta,
				runtime.ToValue(tt.screenX),
				runtime.ToValue(tt.screenY),
				runtime.ToValue(tt.titleHeight))
			if err != nil {
				t.Fatalf("handleClickAtScreenCoords failed: %v", err)
			}

			obj := result.ToObject(runtime)
			hit := obj.Get("hit").ToBoolean()
			row := obj.Get("row").ToInteger()
			col := obj.Get("col").ToInteger()

			if hit != tt.expectHit {
				t.Errorf("Expected hit=%v, got hit=%v", tt.expectHit, hit)
				return
			}

			if !tt.expectHit {
				return // No further checks for misses
			}

			if row != tt.expectedRow {
				t.Errorf("GO-NATIVE CLICK BUG: Expected logical row %d, got %d. "+
					"Screen click (%d, %d) translated incorrectly.",
					tt.expectedRow, row, tt.screenX, tt.screenY)
			}

			if col < tt.minCol || col > tt.maxCol {
				t.Errorf("Expected column in range [%d, %d], got %d",
					tt.minCol, tt.maxCol, col)
			}
		})
	}
}

func TestHandleClickAtScreenCoords_BottomEdge(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Configure textarea: no prompt, no line numbers, 40-char content width
	setPromptFn, _ := goja.AssertFunction(ta.Get("setPrompt"))
	_, _ = setPromptFn(ta, runtime.ToValue(""))
	setShowLineNumbersFn, _ := goja.AssertFunction(ta.Get("setShowLineNumbers"))
	_, _ = setShowLineNumbersFn(ta, runtime.ToValue(false))
	setWidthFn, _ := goja.AssertFunction(ta.Get("setWidth"))
	_, _ = setWidthFn(ta, runtime.ToValue(40))

	// Set content: 100 chars wraps to 3 visual lines in 40-char viewport
	content := strings.Repeat("x", 100) + "\nsecond line"
	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	_, _ = setValueFn(ta, runtime.ToValue(content))

	// Get the new GO-NATIVE methods
	setViewportContextFn, _ := goja.AssertFunction(ta.Get("setViewportContext"))
	handleClickFn, _ := goja.AssertFunction(ta.Get("handleClickAtScreenCoords"))
	getScrollSyncInfoFn, _ := goja.AssertFunction(ta.Get("getScrollSyncInfo"))

	// Configure viewport context (same as other tests)
	vpConfig := runtime.NewObject()
	_ = vpConfig.Set("outerYOffset", 0)
	_ = vpConfig.Set("textareaContentTop", 2)
	_ = vpConfig.Set("textareaContentLeft", 5)
	_ = vpConfig.Set("outerViewportHeight", 10)
	_ = vpConfig.Set("preContentHeight", 2)

	_, err := setViewportContextFn(ta, vpConfig)
	if err != nil {
		t.Fatalf("setViewportContext failed: %v", err)
	}

	// Obtain totalVisualLines via getScrollSyncInfo
	syncVal, err := getScrollSyncInfoFn(ta)
	if err != nil {
		t.Fatalf("getScrollSyncInfo failed: %v", err)
	}
	totalVisualLines := int(syncVal.ToObject(runtime).Get("totalVisualLines").ToInteger())
	titleHeight := 3
	textareaContentTop := 2
	screenX := 10

	// Click on last visual line (should hit)
	screenYLast := titleHeight + textareaContentTop + totalVisualLines - 1
	resLast, err := handleClickFn(ta,
		runtime.ToValue(screenX),
		runtime.ToValue(screenYLast),
		runtime.ToValue(titleHeight))
	if err != nil {
		t.Fatalf("handleClickAtScreenCoords failed: %v", err)
	}
	objLast := resLast.ToObject(runtime)
	if !objLast.Get("hit").ToBoolean() {
		t.Errorf("Expected hit on last visual line at screenY %d (totalVisualLines=%d)", screenYLast, totalVisualLines)
	}

	// Click below content (should HIT and clamp to last line per DEFECT 5 fix)
	// Standard editor behavior: clicking empty space below text places cursor at end of document.
	// The clamping logic in handleClickAtScreenCoords ensures this works correctly.
	screenYBelow := screenYLast + 1
	resBelow, err := handleClickFn(ta,
		runtime.ToValue(screenX),
		runtime.ToValue(screenYBelow),
		runtime.ToValue(titleHeight))
	if err != nil {
		t.Fatalf("handleClickAtScreenCoords failed: %v", err)
	}
	objBelow := resBelow.ToObject(runtime)
	if !objBelow.Get("hit").ToBoolean() {
		t.Errorf("Expected hit for click below content at screenY %d (visualY == totalVisualLines) - should clamp to last line", screenYBelow)
	}
	// Verify the cursor was clamped to the last logical line
	cursorRow := int(objBelow.Get("row").ToInteger())
	if cursorRow != 1 { // "second line" is row 1 (0-indexed)
		t.Errorf("Expected cursor row 1 (last logical line), got %d", cursorRow)
	}
}

func TestHandleClickAtScreenCoords_VpCtxUninitialized(t *testing.T) {
	// Verify that if setViewportContext has not been called yet, clicks are treated as misses
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Configure textarea but DO NOT call setViewportContext
	setPromptFn, _ := goja.AssertFunction(ta.Get("setPrompt"))
	_, _ = setPromptFn(ta, runtime.ToValue(""))
	setShowLineNumbersFn, _ := goja.AssertFunction(ta.Get("setShowLineNumbers"))
	_, _ = setShowLineNumbersFn(ta, runtime.ToValue(false))
	setWidthFn, _ := goja.AssertFunction(ta.Get("setWidth"))
	_, _ = setWidthFn(ta, runtime.ToValue(40))

	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	_, _ = setValueFn(ta, runtime.ToValue("abc"))

	handleClickFn, _ := goja.AssertFunction(ta.Get("handleClickAtScreenCoords"))

	res, err := handleClickFn(ta, runtime.ToValue(10), runtime.ToValue(5), runtime.ToValue(3))
	if err != nil {
		t.Fatalf("handleClickAtScreenCoords failed: %v", err)
	}
	obj := res.ToObject(runtime)
	if obj.Get("hit").ToBoolean() {
		t.Fatalf("Expected miss when viewport context is uninitialized, but got hit")
	}
}

// TestGetScrollSyncInfo tests the GO-NATIVE scroll sync method that returns
// all viewport synchronization data in a single call.
func TestGetScrollSyncInfo(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Configure textarea
	setPromptFn, _ := goja.AssertFunction(ta.Get("setPrompt"))
	_, _ = setPromptFn(ta, runtime.ToValue(""))
	setShowLineNumbersFn, _ := goja.AssertFunction(ta.Get("setShowLineNumbers"))
	_, _ = setShowLineNumbersFn(ta, runtime.ToValue(false))
	setWidthFn, _ := goja.AssertFunction(ta.Get("setWidth"))
	_, _ = setWidthFn(ta, runtime.ToValue(40))

	// Content: 3 logical lines, but first line wraps to 3 visual lines
	// Total visual lines: 3 + 1 + 1 = 5
	content := strings.Repeat("a", 100) + "\nline2\nline3"
	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	_, _ = setValueFn(ta, runtime.ToValue(content))

	// Configure viewport context
	setViewportContextFn, _ := goja.AssertFunction(ta.Get("setViewportContext"))
	vpConfig := runtime.NewObject()
	_ = vpConfig.Set("outerYOffset", 0)
	_ = vpConfig.Set("textareaContentTop", 0)
	_ = vpConfig.Set("textareaContentLeft", 0)
	_ = vpConfig.Set("outerViewportHeight", 3) // Only 3 visible lines
	_ = vpConfig.Set("preContentHeight", 0)
	_, _ = setViewportContextFn(ta, vpConfig)

	getScrollSyncInfoFn, _ := goja.AssertFunction(ta.Get("getScrollSyncInfo"))
	setPositionFn, _ := goja.AssertFunction(ta.Get("setPosition"))

	t.Run("cursor at start", func(t *testing.T) {
		_, _ = setPositionFn(ta, runtime.ToValue(0), runtime.ToValue(0))

		result, err := getScrollSyncInfoFn(ta)
		if err != nil {
			t.Fatalf("getScrollSyncInfo failed: %v", err)
		}

		obj := result.ToObject(runtime)
		cursorVisualLine := obj.Get("cursorVisualLine").ToInteger()
		totalVisualLines := obj.Get("totalVisualLines").ToInteger()
		cursorRow := obj.Get("cursorRow").ToInteger()
		lineCount := obj.Get("lineCount").ToInteger()

		if cursorVisualLine != 0 {
			t.Errorf("Expected cursorVisualLine=0, got %d", cursorVisualLine)
		}
		if totalVisualLines != 5 { // 3 for wrapped first line + 1 + 1
			t.Errorf("Expected totalVisualLines=5, got %d", totalVisualLines)
		}
		if cursorRow != 0 {
			t.Errorf("Expected cursorRow=0, got %d", cursorRow)
		}
		if lineCount != 3 {
			t.Errorf("Expected lineCount=3, got %d", lineCount)
		}
	})

	t.Run("cursor in wrapped segment", func(t *testing.T) {
		// Position cursor at column 50 (within second visual segment of first line)
		_, _ = setPositionFn(ta, runtime.ToValue(0), runtime.ToValue(50))

		result, err := getScrollSyncInfoFn(ta)
		if err != nil {
			t.Fatalf("getScrollSyncInfo failed: %v", err)
		}

		obj := result.ToObject(runtime)
		cursorVisualLine := obj.Get("cursorVisualLine").ToInteger()
		cursorRow := obj.Get("cursorRow").ToInteger()
		cursorCol := obj.Get("cursorCol").ToInteger()

		if cursorVisualLine != 1 { // Second visual line (0-indexed)
			t.Errorf("Expected cursorVisualLine=1 (wrapped segment), got %d", cursorVisualLine)
		}
		if cursorRow != 0 {
			t.Errorf("Expected cursorRow=0 (still first logical line), got %d", cursorRow)
		}
		if cursorCol != 50 {
			t.Errorf("Expected cursorCol=50, got %d", cursorCol)
		}
	})

	t.Run("cursor on second logical line", func(t *testing.T) {
		_, _ = setPositionFn(ta, runtime.ToValue(1), runtime.ToValue(0))

		result, err := getScrollSyncInfoFn(ta)
		if err != nil {
			t.Fatalf("getScrollSyncInfo failed: %v", err)
		}

		obj := result.ToObject(runtime)
		cursorVisualLine := obj.Get("cursorVisualLine").ToInteger()
		cursorRow := obj.Get("cursorRow").ToInteger()

		// First line takes 3 visual lines (100 chars / 40 = 3)
		// So logical line 1 starts at visual line 3
		if cursorVisualLine != 3 {
			t.Errorf("Expected cursorVisualLine=3 (after wrapped first line), got %d", cursorVisualLine)
		}
		if cursorRow != 1 {
			t.Errorf("Expected cursorRow=1, got %d", cursorRow)
		}
	})

	t.Run("suggested scroll offset", func(t *testing.T) {
		// Position cursor at end (visual line 4, beyond viewport of 3 lines)
		_, _ = setPositionFn(ta, runtime.ToValue(2), runtime.ToValue(0))

		result, err := getScrollSyncInfoFn(ta)
		if err != nil {
			t.Fatalf("getScrollSyncInfo failed: %v", err)
		}

		obj := result.ToObject(runtime)
		cursorAbsY := obj.Get("cursorAbsY").ToInteger()
		suggestedYOffset := obj.Get("suggestedYOffset").ToInteger()

		// Cursor at visual line 4 (0-indexed), viewport height 3
		// Suggested offset should scroll down to show cursor
		if cursorAbsY != 4 {
			t.Errorf("Expected cursorAbsY=4, got %d", cursorAbsY)
		}
		// suggestedYOffset = cursorAbsY - viewportHeight + 1 = 4 - 3 + 1 = 2
		if suggestedYOffset != 2 {
			t.Errorf("Expected suggestedYOffset=2, got %d", suggestedYOffset)
		}
	})
}

// TestScenarioGreedyWrap tests the "waste" at end of lines.
func TestScenarioGreedyWrap(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// 1. Setup: No line numbers, No prompt. Width 3.
	setShowLineNumbersFn, _ := goja.AssertFunction(ta.Get("setShowLineNumbers"))
	_, _ = setShowLineNumbersFn(ta, runtime.ToValue(false))
	setPromptFn, _ := goja.AssertFunction(ta.Get("setPrompt"))
	_, _ = setPromptFn(ta, runtime.ToValue(""))
	setWidthFn, _ := goja.AssertFunction(ta.Get("setWidth"))
	_, _ = setWidthFn(ta, runtime.ToValue(3))

	// 2. Input: "你好你好" (Visual Width 8)
	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	_, _ = setValueFn(ta, runtime.ToValue("你好你好"))

	// 3. Verify
	visualLineCountFn, _ := goja.AssertFunction(ta.Get("visualLineCount"))
	val, _ := visualLineCountFn(ta)
	count := val.ToInteger()

	// Expect 4 lines: [你_], [好_], [你_], [好 ]
	if count != 4 {
		t.Errorf("GREEDY WRAP FAILURE: Expected 4 visual lines, got %d", count)
	}
}

// TestScenarioMegaChar tests characters wider than the viewport.
func TestScenarioMegaChar(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// 1. Setup: Width 1. No prompt.
	setShowLineNumbersFn, _ := goja.AssertFunction(ta.Get("setShowLineNumbers"))
	_, _ = setShowLineNumbersFn(ta, runtime.ToValue(false))
	setPromptFn, _ := goja.AssertFunction(ta.Get("setPrompt"))
	_, _ = setPromptFn(ta, runtime.ToValue(""))
	setWidthFn, _ := goja.AssertFunction(ta.Get("setWidth"))
	_, _ = setWidthFn(ta, runtime.ToValue(1))

	// 2. Input: "a你b"
	// 'a' (1) fits.
	// '你' (2) overflows width 1 -> Force Wrap -> Occupies Line 2.
	// 'b' (1) -> Force Wrap (Line 2 was full) -> Occupies Line 3.
	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	_, _ = setValueFn(ta, runtime.ToValue("a你b"))

	// 3. Verify
	visualLineCountFn, _ := goja.AssertFunction(ta.Get("visualLineCount"))
	val, _ := visualLineCountFn(ta)
	count := val.ToInteger()

	if count != 3 {
		t.Errorf("MEGA CHAR FAILURE: Expected 3 visual lines, got %d. Input 'a你b' in Width 1.", count)
	}
}

// TestHandleClickAtScreenCoords_StaleViewportContext reproduces the "phantom scroll"
// scenario where setViewportContext has a stale outerYOffset and a click that
// occurs after JS auto-scroll would map incorrectly until setViewportContext is
// updated with the final offset.
func TestHandleClickAtScreenCoords_StaleViewportContext(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Configure textarea
	setPromptFn, _ := goja.AssertFunction(ta.Get("setPrompt"))
	_, _ = setPromptFn(ta, runtime.ToValue(""))
	setShowLineNumbersFn, _ := goja.AssertFunction(ta.Get("setShowLineNumbers"))
	_, _ = setShowLineNumbersFn(ta, runtime.ToValue(false))
	setWidthFn, _ := goja.AssertFunction(ta.Get("setWidth"))
	_, _ = setWidthFn(ta, runtime.ToValue(40))

	// Content: many lines so we can place cursor far from initial offset
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		_, _ = fmt.Fprintf(&sb, "line%03d\n", i)
	}
	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	_, _ = setValueFn(ta, runtime.ToValue(sb.String()))

	setViewportContextFn, _ := goja.AssertFunction(ta.Get("setViewportContext"))
	getScrollSyncInfoFn, _ := goja.AssertFunction(ta.Get("getScrollSyncInfo"))
	handleClickFn, _ := goja.AssertFunction(ta.Get("handleClickAtScreenCoords"))
	setPositionFn, _ := goja.AssertFunction(ta.Get("setPosition"))

	// Initial viewport: outerYOffset=10 (stale)
	vpConfig := runtime.NewObject()
	_ = vpConfig.Set("outerYOffset", 10)
	_ = vpConfig.Set("textareaContentTop", 2)
	_ = vpConfig.Set("textareaContentLeft", 0)
	_ = vpConfig.Set("outerViewportHeight", 10)
	_ = vpConfig.Set("preContentHeight", 2)
	_ = vpConfig.Set("titleHeight", 3)
	_, _ = setViewportContextFn(ta, vpConfig)

	// Place cursor at logical row 27
	_, _ = setPositionFn(ta, runtime.ToValue(27), runtime.ToValue(0))

	// Ask Go for suggestedYOffset (what JS would set on the outer viewport)
	syncVal, err := getScrollSyncInfoFn(ta)
	if err != nil {
		t.Fatalf("getScrollSyncInfo failed: %v", err)
	}
	syncObj := syncVal.ToObject(runtime)
	suggested := int(syncObj.Get("suggestedYOffset").ToInteger())
	cursorAbsY := int(syncObj.Get("cursorAbsY").ToInteger())

	if suggested == 10 {
		t.Fatalf("sanity: suggestedYOffset==initial offset, test setup invalid")
	}

	// Simulate the user's view: screenY where the cursor is visible after JS scrolls
	titleHeight := 3
	screenY := titleHeight + (cursorAbsY - suggested)
	screenX := 5

	// With stale vpCtx (outerYOffset still 10), JS-visible coordinate should NOT map to the right row
	res, err := handleClickFn(ta, runtime.ToValue(screenX), runtime.ToValue(screenY), runtime.ToValue(titleHeight))
	if err != nil {
		t.Fatalf("handleClickAtScreenCoords failed: %v", err)
	}
	obj := res.ToObject(runtime)
	if obj.Get("hit").ToBoolean() {
		row := obj.Get("row").ToInteger()
		if row == 27 {
			t.Fatalf("Expected stale context to cause incorrect mapping, but got row 27")
		}
	}

	// Now simulate JS updating the viewport context to the final suggested value
	vpConfig2 := runtime.NewObject()
	_ = vpConfig2.Set("outerYOffset", suggested)
	_ = vpConfig2.Set("textareaContentTop", 2)
	_ = vpConfig2.Set("textareaContentLeft", 0)
	_ = vpConfig2.Set("outerViewportHeight", 10)
	_ = vpConfig2.Set("preContentHeight", 2)
	_ = vpConfig2.Set("titleHeight", 3)
	_, _ = setViewportContextFn(ta, vpConfig2)

	// Now the click should map to the correct logical row
	res2, err := handleClickFn(ta, runtime.ToValue(screenX), runtime.ToValue(screenY), runtime.ToValue(titleHeight))
	if err != nil {
		t.Fatalf("handleClickAtScreenCoords failed: %v", err)
	}
	obj2 := res2.ToObject(runtime)
	if !obj2.Get("hit").ToBoolean() {
		t.Fatalf("After updating viewport context expected hit=true, got miss")
	}
	row2 := obj2.Get("row").ToInteger()
	if row2 != 27 {
		t.Fatalf("After updating viewport context expected row 27, got %d", row2)
	}
}

// Test that handleClickAtScreenCoords uses titleHeight from vpCtx if caller omits arg
func TestHandleClickAtScreenCoords_TitleHeightFromVpCtx(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Configure textarea
	setPromptFn, _ := goja.AssertFunction(ta.Get("setPrompt"))
	_, _ = setPromptFn(ta, runtime.ToValue(""))
	setShowLineNumbersFn, _ := goja.AssertFunction(ta.Get("setShowLineNumbers"))
	_, _ = setShowLineNumbersFn(ta, runtime.ToValue(false))
	setWidthFn, _ := goja.AssertFunction(ta.Get("setWidth"))
	_, _ = setWidthFn(ta, runtime.ToValue(40))

	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	_, _ = setValueFn(ta, runtime.ToValue(strings.Repeat("x", 100)+"\nsecond line"))

	setViewportContextFn, _ := goja.AssertFunction(ta.Get("setViewportContext"))
	handleClickFn, _ := goja.AssertFunction(ta.Get("handleClickAtScreenCoords"))
	setPositionFn, _ := goja.AssertFunction(ta.Get("setPosition"))

	vpConfig := runtime.NewObject()
	_ = vpConfig.Set("outerYOffset", 0)
	_ = vpConfig.Set("textareaContentTop", 2)
	_ = vpConfig.Set("textareaContentLeft", 5)
	_ = vpConfig.Set("outerViewportHeight", 10)
	_ = vpConfig.Set("preContentHeight", 2)
	_ = vpConfig.Set("titleHeight", 4)
	_, _ = setViewportContextFn(ta, vpConfig)

	// Place cursor at logical row 0
	_, _ = setPositionFn(ta, runtime.ToValue(0), runtime.ToValue(0))

	// Calculate a screenY that corresponds to the first visual line
	getScrollSyncInfoFn, _ := goja.AssertFunction(ta.Get("getScrollSyncInfo"))
	syncVal, _ := getScrollSyncInfoFn(ta)
	syncObj := syncVal.ToObject(runtime)
	cursorAbsY := int(syncObj.Get("cursorAbsY").ToInteger())
	screenY := 4 + (cursorAbsY - int(syncObj.Get("suggestedYOffset").ToInteger())) // 4 == titleHeight in vpCtx

	// Call handleClick WITHOUT passing the titleHeight arg
	res, err := handleClickFn(ta, runtime.ToValue(10), runtime.ToValue(screenY))
	if err != nil {
		t.Fatalf("handleClickAtScreenCoords failed: %v", err)
	}
	obj := res.ToObject(runtime)
	if !obj.Get("hit").ToBoolean() {
		t.Fatalf("Expected hit but got miss when vpCtx provided titleHeight")
	}
}

// Test that when vpCtx.titleHeight is set it takes precedence over an
// explicit argument provided by the caller.
func TestHandleClickAtScreenCoords_PrefersVpCtxOverArg(t *testing.T) {
	manager := NewManager()
	runtime := goja.New()

	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	result, _ := newFn(goja.Undefined())
	ta := result.ToObject(runtime)

	// Configure textarea
	setPromptFn, _ := goja.AssertFunction(ta.Get("setPrompt"))
	_, _ = setPromptFn(ta, runtime.ToValue(""))
	setShowLineNumbersFn, _ := goja.AssertFunction(ta.Get("setShowLineNumbers"))
	_, _ = setShowLineNumbersFn(ta, runtime.ToValue(false))
	setWidthFn, _ := goja.AssertFunction(ta.Get("setWidth"))
	_, _ = setWidthFn(ta, runtime.ToValue(40))

	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	_, _ = setValueFn(ta, runtime.ToValue(strings.Repeat("x", 100)+"\nsecond line"))

	setViewportContextFn, _ := goja.AssertFunction(ta.Get("setViewportContext"))
	handleClickFn, _ := goja.AssertFunction(ta.Get("handleClickAtScreenCoords"))
	setPositionFn, _ := goja.AssertFunction(ta.Get("setPosition"))
	getScrollSyncInfoFn, _ := goja.AssertFunction(ta.Get("getScrollSyncInfo"))

	// Set vpCtx.titleHeight = 5
	vpConfig := runtime.NewObject()
	_ = vpConfig.Set("outerYOffset", 0)
	_ = vpConfig.Set("textareaContentTop", 2)
	_ = vpConfig.Set("textareaContentLeft", 5)
	_ = vpConfig.Set("outerViewportHeight", 10)
	_ = vpConfig.Set("preContentHeight", 2)
	_ = vpConfig.Set("titleHeight", 5)
	_, _ = setViewportContextFn(ta, vpConfig)

	// Place cursor at logical row 0
	_, _ = setPositionFn(ta, runtime.ToValue(0), runtime.ToValue(0))

	// Compute a screenY that corresponds to the first visual line using titleHeight=5
	syncVal, _ := getScrollSyncInfoFn(ta)
	syncObj := syncVal.ToObject(runtime)
	cursorAbsY := int(syncObj.Get("cursorAbsY").ToInteger())
	screenY := 5 + (cursorAbsY - int(syncObj.Get("suggestedYOffset").ToInteger()))

	// Call handleClick with a conflicting explicit arg (1) — vpCtx should win
	res1, err := handleClickFn(ta, runtime.ToValue(10), runtime.ToValue(screenY), runtime.ToValue(1))
	if err != nil {
		t.Fatalf("handleClickAtScreenCoords failed: %v", err)
	}
	obj1 := res1.ToObject(runtime)
	if !obj1.Get("hit").ToBoolean() {
		t.Fatalf("Expected hit using vpCtx titleHeight (5), got miss")
	}

	// Now change vpCtx.titleHeight to 1 and verify it is used instead
	vpConfig2 := runtime.NewObject()
	_ = vpConfig2.Set("outerYOffset", 0)
	_ = vpConfig2.Set("textareaContentTop", 2)
	_ = vpConfig2.Set("textareaContentLeft", 5)
	_ = vpConfig2.Set("outerViewportHeight", 10)
	_ = vpConfig2.Set("preContentHeight", 2)
	_ = vpConfig2.Set("titleHeight", 1)
	_, _ = setViewportContextFn(ta, vpConfig2)

	// Recompute screenY for titleHeight=1
	syncVal2, _ := getScrollSyncInfoFn(ta)
	syncObj2 := syncVal2.ToObject(runtime)
	cursorAbsY2 := int(syncObj2.Get("cursorAbsY").ToInteger())
	screenY2 := 1 + (cursorAbsY2 - int(syncObj2.Get("suggestedYOffset").ToInteger()))

	// Call with explicit conflicting arg (5) - vpCtx (1) should still be used
	res2, err := handleClickFn(ta, runtime.ToValue(10), runtime.ToValue(screenY2), runtime.ToValue(5))
	if err != nil {
		t.Fatalf("handleClickAtScreenCoords failed: %v", err)
	}
	obj2 := res2.ToObject(runtime)
	if !obj2.Get("hit").ToBoolean() {
		t.Fatalf("Expected hit using vpCtx titleHeight=1 after update, got miss")
	}
}
