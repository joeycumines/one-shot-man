package lipgloss

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	manager := NewManager()
	assert.NotNil(t, manager)
}

func TestRequire_ExportsCorrectAPI(t *testing.T) {
	manager := NewManager()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	// Call the require function
	requireFn := Require(manager)
	requireFn(vm, module)

	// Verify exports
	exports := module.Get("exports").ToObject(vm)
	require.NotNil(t, exports)

	// Check alignment constants
	assert.Equal(t, 0.0, exports.Get("Left").ToFloat())
	assert.Equal(t, 0.5, exports.Get("Center").ToFloat())
	assert.Equal(t, 1.0, exports.Get("Right").ToFloat())
	assert.Equal(t, 0.0, exports.Get("Top").ToFloat())
	assert.Equal(t, 1.0, exports.Get("Bottom").ToFloat())

	// Check functions are exported
	for _, fn := range []string{
		"newStyle",
		"normalBorder",
		"roundedBorder",
		"thickBorder",
		"doubleBorder",
		"hiddenBorder",
		"noBorder",
		"joinHorizontal",
		"joinVertical",
		"place",
		"size",
		"width",
		"height",
	} {
		val := exports.Get(fn)
		assert.False(t, goja.IsUndefined(val), "Function %s should be exported", fn)
		assert.False(t, goja.IsNull(val), "Function %s should not be null", fn)
		_, ok := goja.AssertFunction(val)
		assert.True(t, ok, "Export %s should be a function", fn)
	}
}

func TestNewStyle_ReturnsStyleObject(t *testing.T) {
	manager := NewManager()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(manager)
	requireFn(vm, module)

	exports := module.Get("exports").ToObject(vm)
	newStyleFn, _ := goja.AssertFunction(exports.Get("newStyle"))

	result, err := newStyleFn(goja.Undefined())
	require.NoError(t, err)
	assert.False(t, goja.IsUndefined(result))
	assert.False(t, goja.IsNull(result))

	// Check style has expected methods
	styleObj := result.ToObject(vm)
	for _, method := range []string{
		"render", "copy", "bold", "italic", "underline",
		"strikethrough", "blink", "faint", "reverse",
		"foreground", "background",
		"padding", "paddingTop", "paddingRight", "paddingBottom", "paddingLeft",
		"margin", "marginTop", "marginRight", "marginBottom", "marginLeft",
		"width", "height", "maxWidth", "maxHeight",
		"border", "borderTop", "borderRight", "borderBottom", "borderLeft",
		"borderForeground", "borderBackground",
		"align", "alignHorizontal", "alignVertical",
	} {
		val := styleObj.Get(method)
		assert.False(t, goja.IsUndefined(val), "Method %s should exist", method)
		_, ok := goja.AssertFunction(val)
		assert.True(t, ok, "Method %s should be a function", method)
	}
}

func TestStyleRender(t *testing.T) {
	manager := NewManager()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(manager)
	requireFn(vm, module)

	// Set up lipgloss global for use in RunString
	_ = vm.Set("lipgloss", module.Get("exports"))

	// Create a style and render text
	result, err := vm.RunString(`
		const style = lipgloss.newStyle();
		style.render('Hello');
	`)
	require.NoError(t, err)
	assert.Contains(t, result.String(), "Hello")
}

func TestStyleCopy(t *testing.T) {
	manager := NewManager()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(manager)
	requireFn(vm, module)

	_ = vm.Set("lipgloss", module.Get("exports"))

	// Create a style, set bold, copy it, and verify both work independently
	_, err := vm.RunString(`
		const style1 = lipgloss.newStyle().bold(true);
		const style2 = style1.copy();
		// Both should be valid style objects
		style1.render('Test');
		style2.render('Test');
	`)
	require.NoError(t, err)
}

func TestStyleChaining(t *testing.T) {
	manager := NewManager()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(manager)
	requireFn(vm, module)

	_ = vm.Set("lipgloss", module.Get("exports"))

	// Test that methods are chainable
	result, err := vm.RunString(`
		const style = lipgloss.newStyle()
			.bold(true)
			.italic(true)
			.underline(true)
			.foreground('#FF0000')
			.background('#00FF00')
			.padding(1, 2)
			.margin(1);
		style.render('Chained');
	`)
	require.NoError(t, err)
	// Result should contain our text (may have styling escape codes)
	assert.NotEmpty(t, result.String())
}

func TestBorderFunctions(t *testing.T) {
	manager := NewManager()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(manager)
	requireFn(vm, module)

	exports := module.Get("exports").ToObject(vm)

	tests := []struct {
		name string
	}{
		{"normalBorder"},
		{"roundedBorder"},
		{"thickBorder"},
		{"doubleBorder"},
		{"hiddenBorder"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn, _ := goja.AssertFunction(exports.Get(tt.name))
			result, err := fn(goja.Undefined())
			require.NoError(t, err)

			// Border should be an object with expected properties
			borderObj := result.ToObject(vm)
			for _, prop := range []string{"top", "bottom", "left", "right", "topLeft", "topRight", "bottomLeft", "bottomRight"} {
				val := borderObj.Get(prop)
				assert.False(t, goja.IsUndefined(val), "Border should have %s property", prop)
			}
		})
	}
}

func TestNoBorder(t *testing.T) {
	manager := NewManager()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(manager)
	requireFn(vm, module)

	exports := module.Get("exports").ToObject(vm)
	fn, _ := goja.AssertFunction(exports.Get("noBorder"))
	result, err := fn(goja.Undefined())
	require.NoError(t, err)
	assert.True(t, goja.IsNull(result) || goja.IsUndefined(result))
}

func TestStyleWithBorder(t *testing.T) {
	manager := NewManager()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(manager)
	requireFn(vm, module)

	_ = vm.Set("lipgloss", module.Get("exports"))

	result, err := vm.RunString(`
		const style = lipgloss.newStyle()
			.border(lipgloss.roundedBorder())
			.borderForeground('#FF0000')
			.padding(1, 2);
		style.render('Bordered');
	`)
	require.NoError(t, err)
	assert.NotEmpty(t, result.String())
}

func TestJoinHorizontal(t *testing.T) {
	manager := NewManager()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(manager)
	requireFn(vm, module)

	_ = vm.Set("lipgloss", module.Get("exports"))

	result, err := vm.RunString(`
		lipgloss.joinHorizontal(lipgloss.Top, 'Hello', ' ', 'World');
	`)
	require.NoError(t, err)
	assert.Contains(t, result.String(), "Hello")
	assert.Contains(t, result.String(), "World")
}

func TestJoinVertical(t *testing.T) {
	manager := NewManager()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(manager)
	requireFn(vm, module)

	_ = vm.Set("lipgloss", module.Get("exports"))

	result, err := vm.RunString(`
		lipgloss.joinVertical(lipgloss.Left, 'Line1', 'Line2');
	`)
	require.NoError(t, err)
	assert.Contains(t, result.String(), "Line1")
	assert.Contains(t, result.String(), "Line2")
}

func TestPlace(t *testing.T) {
	manager := NewManager()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(manager)
	requireFn(vm, module)

	_ = vm.Set("lipgloss", module.Get("exports"))

	result, err := vm.RunString(`
		lipgloss.place(20, 5, lipgloss.Center, lipgloss.Center, 'Hi');
	`)
	require.NoError(t, err)
	// Should have the text "Hi" somewhere
	assert.Contains(t, result.String(), "Hi")
}

func TestSizeWidthHeight(t *testing.T) {
	manager := NewManager()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(manager)
	requireFn(vm, module)

	_ = vm.Set("lipgloss", module.Get("exports"))

	// Test width
	widthResult, err := vm.RunString(`
		lipgloss.width('Hello');
	`)
	require.NoError(t, err)
	assert.Equal(t, int64(5), widthResult.ToInteger())

	// Test height
	heightResult, err := vm.RunString(`
		lipgloss.height('Hello\nWorld');
	`)
	require.NoError(t, err)
	assert.Equal(t, int64(2), heightResult.ToInteger())

	// Test size
	sizeResult, err := vm.RunString(`
		JSON.stringify(lipgloss.size('Hello\nWorld'));
	`)
	require.NoError(t, err)
	assert.Contains(t, sizeResult.String(), `"width":5`)
	assert.Contains(t, sizeResult.String(), `"height":2`)
}

func TestDimensionMethods(t *testing.T) {
	manager := NewManager()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(manager)
	requireFn(vm, module)

	_ = vm.Set("lipgloss", module.Get("exports"))

	result, err := vm.RunString(`
		const style = lipgloss.newStyle()
			.width(20)
			.height(5)
			.maxWidth(30)
			.maxHeight(10);
		style.render('Test');
	`)
	require.NoError(t, err)
	assert.NotEmpty(t, result.String())
}

func TestPaddingVariants(t *testing.T) {
	manager := NewManager()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(manager)
	requireFn(vm, module)

	_ = vm.Set("lipgloss", module.Get("exports"))

	// Test all padding methods
	_, err := vm.RunString(`
		const style = lipgloss.newStyle()
			.padding(1)
			.paddingTop(2)
			.paddingRight(3)
			.paddingBottom(4)
			.paddingLeft(5);
		style.render('Test');
	`)
	require.NoError(t, err)
}

func TestMarginVariants(t *testing.T) {
	manager := NewManager()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(manager)
	requireFn(vm, module)

	_ = vm.Set("lipgloss", module.Get("exports"))

	// Test all margin methods
	_, err := vm.RunString(`
		const style = lipgloss.newStyle()
			.margin(1)
			.marginTop(2)
			.marginRight(3)
			.marginBottom(4)
			.marginLeft(5);
		style.render('Test');
	`)
	require.NoError(t, err)
}

func TestBorderSides(t *testing.T) {
	manager := NewManager()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(manager)
	requireFn(vm, module)

	_ = vm.Set("lipgloss", module.Get("exports"))

	// Test border side methods
	_, err := vm.RunString(`
		const style = lipgloss.newStyle()
			.border(lipgloss.normalBorder())
			.borderTop(true)
			.borderRight(true)
			.borderBottom(true)
			.borderLeft(true)
			.borderForeground('#FF0000')
			.borderBackground('#00FF00');
		style.render('Test');
	`)
	require.NoError(t, err)
}

func TestAlignmentMethods(t *testing.T) {
	manager := NewManager()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(manager)
	requireFn(vm, module)

	_ = vm.Set("lipgloss", module.Get("exports"))

	// Test alignment methods
	_, err := vm.RunString(`
		const style = lipgloss.newStyle()
			.width(20)
			.height(5)
			.align(lipgloss.Center)
			.alignHorizontal(lipgloss.Right)
			.alignVertical(lipgloss.Bottom);
		style.render('Test');
	`)
	require.NoError(t, err)
}

func TestTextFormattingMethods(t *testing.T) {
	manager := NewManager()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(manager)
	requireFn(vm, module)

	_ = vm.Set("lipgloss", module.Get("exports"))

	// Test all text formatting methods
	_, err := vm.RunString(`
		const style = lipgloss.newStyle()
			.bold(true)
			.italic(true)
			.underline(true)
			.strikethrough(true)
			.blink(true)
			.faint(true)
			.reverse(true);
		style.render('Test');
	`)
	require.NoError(t, err)
}

func TestColorMethods(t *testing.T) {
	manager := NewManager()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(manager)
	requireFn(vm, module)

	_ = vm.Set("lipgloss", module.Get("exports"))

	// Test color methods with different color formats
	_, err := vm.RunString(`
		// Hex colors
		const style1 = lipgloss.newStyle()
			.foreground('#FF0000')
			.background('#00FF00');
		style1.render('Test');
		
		// ANSI color numbers
		const style2 = lipgloss.newStyle()
			.foreground('1')
			.background('2');
		style2.render('Test');
	`)
	require.NoError(t, err)
}

func TestEdgeCases(t *testing.T) {
	manager := NewManager()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(manager)
	requireFn(vm, module)

	_ = vm.Set("lipgloss", module.Get("exports"))

	// Test render with no arguments
	result, err := vm.RunString(`
		const style = lipgloss.newStyle();
		style.render();
	`)
	require.NoError(t, err)
	assert.Equal(t, "", result.String())

	// Test joinHorizontal with insufficient args
	result, err = vm.RunString(`
		lipgloss.joinHorizontal(0);
	`)
	require.NoError(t, err)
	assert.Equal(t, "", result.String())

	// Test place with insufficient args
	result, err = vm.RunString(`
		lipgloss.place(10, 5);
	`)
	require.NoError(t, err)
	assert.Equal(t, "", result.String())

	// Test width/height with no args
	result, err = vm.RunString(`
		lipgloss.width();
	`)
	require.NoError(t, err)
	assert.Equal(t, int64(0), result.ToInteger())
}

func TestRenderMultipleStrings(t *testing.T) {
	manager := NewManager()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(manager)
	requireFn(vm, module)

	_ = vm.Set("lipgloss", module.Get("exports"))

	result, err := vm.RunString(`
		const style = lipgloss.newStyle();
		style.render('Hello', ' ', 'World');
	`)
	require.NoError(t, err)
	// lipgloss.Render applies the style to all string arguments
	output := result.String()
	assert.True(t, strings.Contains(output, "Hello") && strings.Contains(output, "World"))
}

func TestStyleImmutability(t *testing.T) {
	manager := NewManager()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(manager)
	requireFn(vm, module)

	_ = vm.Set("lipgloss", module.Get("exports"))

	// Test that modifying a derived style doesn't affect the original
	// Using padding which adds visible spaces even without TTY
	result, err := vm.RunString(`
		const base = lipgloss.newStyle();
		const derived = base.padding(2);
		
		// Render base - should not have padding
		const baseRendered = base.render('test');
		
		// Render derived - should have padding (extra spaces)
		const derivedRendered = derived.render('test');
		
		// Base should be different from derived
		JSON.stringify({
			baseLen: baseRendered.length,
			derivedLen: derivedRendered.length,
			different: baseRendered.length !== derivedRendered.length
		});
	`)
	require.NoError(t, err)

	// Parse the result - derived should be longer due to padding
	resultStr := result.String()
	assert.Contains(t, resultStr, `"different":true`, "Base style should not be affected by derived style modifications")
}
