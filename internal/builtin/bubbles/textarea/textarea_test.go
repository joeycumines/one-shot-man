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
