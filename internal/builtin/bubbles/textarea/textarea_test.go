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
