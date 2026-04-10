package lipgloss

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Helper ---

func setupModule(t *testing.T) (*goja.Runtime, *goja.Object) {
	t.Helper()
	manager := NewManager()
	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))
	requireFn := Require(manager)
	requireFn(vm, module)
	_ = vm.Set("lipgloss", module.Get("exports"))
	return vm, module
}

// --- UnwrapStyle ---

func TestUnwrapStyle_ValidStyle(t *testing.T) {
	vm, _ := setupModule(t)

	styleVal, err := vm.RunString(`lipgloss.newStyle().bold(true)`)
	require.NoError(t, err)

	style, err := UnwrapStyle(vm, styleVal)
	require.NoError(t, err)
	assert.NotNil(t, style)
}

func TestUnwrapStyle_NilValue(t *testing.T) {
	vm := goja.New()
	_, err := UnwrapStyle(vm, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "null or undefined")
}

func TestUnwrapStyle_UndefinedValue(t *testing.T) {
	vm := goja.New()
	_, err := UnwrapStyle(vm, goja.Undefined())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "null or undefined")
}

func TestUnwrapStyle_NullValue(t *testing.T) {
	vm := goja.New()
	_, err := UnwrapStyle(vm, goja.Null())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "null or undefined")
}

func TestUnwrapStyle_NonStyleObject(t *testing.T) {
	vm := goja.New()
	obj := vm.NewObject()
	_, err := UnwrapStyle(vm, obj)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a lipgloss style")
}

func TestUnwrapStyle_InvalidInternalState(t *testing.T) {
	vm := goja.New()
	obj := vm.NewObject()
	_ = obj.Set(internalStateKey, "not a styleState")
	_, err := UnwrapStyle(vm, obj)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid internal state")
}

func TestUnwrapStyle_ErrorState(t *testing.T) {
	vm, _ := setupModule(t)

	// Create an error style via invalid width
	styleVal, err := vm.RunString(`lipgloss.newStyle().width(-10)`)
	require.NoError(t, err)

	_, err = UnwrapStyle(vm, styleVal)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot use style with error")
}

// --- parseColor edge cases ---

func TestParseColor_NullReturnsNoColor(t *testing.T) {
	vm := goja.New()
	color, err := parseColor(vm, goja.Null(), false)
	require.NoError(t, err)
	assert.NotNil(t, color) // NoColor{} is a valid color
}

func TestParseColor_UndefinedReturnsNoColor(t *testing.T) {
	vm := goja.New()
	color, err := parseColor(vm, goja.Undefined(), false)
	require.NoError(t, err)
	assert.NotNil(t, color)
}

func TestParseColor_EmptyStringReturnsNoColor(t *testing.T) {
	vm := goja.New()
	color, err := parseColor(vm, vm.ToValue(""), false)
	require.NoError(t, err)
	assert.NotNil(t, color)
}

func TestParseColor_WhitespaceOnlyReturnsNoColor(t *testing.T) {
	vm := goja.New()
	color, err := parseColor(vm, vm.ToValue("   "), false)
	require.NoError(t, err)
	assert.NotNil(t, color)
}

func TestParseColor_InvalidHexColor(t *testing.T) {
	vm := goja.New()
	_, err := parseColor(vm, vm.ToValue("#GGG"), false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid hex color")
}

func TestParseColor_InvalidHex6Char(t *testing.T) {
	vm := goja.New()
	_, err := parseColor(vm, vm.ToValue("#GGGGGG"), false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid hex color")
}

func TestParseColor_ValidHex3(t *testing.T) {
	vm := goja.New()
	color, err := parseColor(vm, vm.ToValue("#F00"), false)
	require.NoError(t, err)
	assert.NotNil(t, color)
}

func TestParseColor_ValidHex6(t *testing.T) {
	vm := goja.New()
	color, err := parseColor(vm, vm.ToValue("#FF0000"), false)
	require.NoError(t, err)
	assert.NotNil(t, color)
}

func TestParseColor_ANSIMin(t *testing.T) {
	vm := goja.New()
	color, err := parseColor(vm, vm.ToValue("0"), false)
	require.NoError(t, err)
	assert.NotNil(t, color)
}

func TestParseColor_ANSIMax(t *testing.T) {
	vm := goja.New()
	color, err := parseColor(vm, vm.ToValue("255"), false)
	require.NoError(t, err)
	assert.NotNil(t, color)
}

func TestParseColor_ANSIOutOfRange(t *testing.T) {
	vm := goja.New()
	_, err := parseColor(vm, vm.ToValue("256"), false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ANSI color must be 0-255")
}

func TestParseColor_ANSINegative(t *testing.T) {
	vm := goja.New()
	_, err := parseColor(vm, vm.ToValue("-1"), false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ANSI color must be 0-255")
}

func TestParseColor_NamedColor(t *testing.T) {
	vm := goja.New()
	color, err := parseColor(vm, vm.ToValue("red"), false)
	require.NoError(t, err)
	assert.NotNil(t, color)
}

func TestParseColor_AdaptiveColor(t *testing.T) {
	vm := goja.New()
	obj := vm.NewObject()
	_ = obj.Set("light", "#FFFFFF")
	_ = obj.Set("dark", "#000000")

	color, err := parseColor(vm, obj, false)
	require.NoError(t, err)
	assert.NotNil(t, color)
}

func TestParseColor_AdaptiveColor_MissingLight(t *testing.T) {
	vm := goja.New()
	obj := vm.NewObject()
	_ = obj.Set("dark", "#000000")

	_, err := parseColor(vm, obj, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'light' or 'dark'")
}

func TestParseColor_AdaptiveColor_MissingDark(t *testing.T) {
	vm := goja.New()
	obj := vm.NewObject()
	_ = obj.Set("light", "#FFFFFF")

	_, err := parseColor(vm, obj, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'light' or 'dark'")
}

func TestParseColor_AdaptiveColor_InvalidLight(t *testing.T) {
	vm := goja.New()
	obj := vm.NewObject()
	_ = obj.Set("light", "#GGGGGG")
	_ = obj.Set("dark", "#000000")

	_, err := parseColor(vm, obj, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid light color")
}

func TestParseColor_AdaptiveColor_InvalidDark(t *testing.T) {
	vm := goja.New()
	obj := vm.NewObject()
	_ = obj.Set("light", "#FFFFFF")
	_ = obj.Set("dark", "#GGGGGG")

	_, err := parseColor(vm, obj, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid dark color")
}

// --- parseWhitespaceOptions edge cases ---

func TestParseWhitespaceOptions_NilValue(t *testing.T) {
	vm := goja.New()
	opts, err := parseWhitespaceOptions(vm, goja.Null(), false)
	require.NoError(t, err)
	assert.Empty(t, opts)
}

func TestParseWhitespaceOptions_UndefinedValue(t *testing.T) {
	vm := goja.New()
	opts, err := parseWhitespaceOptions(vm, goja.Undefined(), false)
	require.NoError(t, err)
	assert.Empty(t, opts)
}

func TestParseWhitespaceOptions_WithForeground(t *testing.T) {
	vm := goja.New()
	obj := vm.NewObject()
	_ = obj.Set("whitespaceForeground", "#FF0000")
	opts, err := parseWhitespaceOptions(vm, obj, false)
	require.NoError(t, err)
	assert.Len(t, opts, 1)
}

func TestParseWhitespaceOptions_WithBackground(t *testing.T) {
	vm := goja.New()
	obj := vm.NewObject()
	_ = obj.Set("whitespaceBackground", "#00FF00")
	opts, err := parseWhitespaceOptions(vm, obj, false)
	require.NoError(t, err)
	assert.Len(t, opts, 1)
}

func TestParseWhitespaceOptions_WithAllOptions(t *testing.T) {
	vm := goja.New()
	obj := vm.NewObject()
	_ = obj.Set("whitespaceChars", ".")
	_ = obj.Set("whitespaceForeground", "#FF0000")
	_ = obj.Set("whitespaceBackground", "#00FF00")
	opts, err := parseWhitespaceOptions(vm, obj, false)
	require.NoError(t, err)
	// v2 lipgloss only accepts 2 options: WithWhitespaceChars + WithWhitespaceStyle.
	// The foreground/background are combined into a single WithWhitespaceStyle.
	assert.Len(t, opts, 2)
}

func TestParseWhitespaceOptions_ForegroundError(t *testing.T) {
	vm := goja.New()
	obj := vm.NewObject()
	_ = obj.Set("whitespaceForeground", "#GGGGGG")
	_, err := parseWhitespaceOptions(vm, obj, false)
	assert.Error(t, err)
}

func TestParseWhitespaceOptions_BackgroundError(t *testing.T) {
	vm := goja.New()
	obj := vm.NewObject()
	_ = obj.Set("whitespaceBackground", "#GGGGGG")
	_, err := parseWhitespaceOptions(vm, obj, false)
	assert.Error(t, err)
}

func TestParseWhitespaceOptions_UndefinedForeground(t *testing.T) {
	vm := goja.New()
	obj := vm.NewObject()
	// foreground is missing (undefined) — should skip
	_ = obj.Set("whitespaceChars", "x")
	opts, err := parseWhitespaceOptions(vm, obj, false)
	require.NoError(t, err)
	assert.Len(t, opts, 1) // only whitespaceChars
}

func TestParseWhitespaceOptions_NullForeground(t *testing.T) {
	// Note: null foreground passes the nil/undefined check and gets processed
	// by parseColor which returns NoColor{}, so a whitespace option IS created.
	// This is benign (NoColor{} is effectively a no-op).
	vm := goja.New()
	obj := vm.NewObject()
	_ = obj.Set("whitespaceForeground", goja.Null())
	opts, err := parseWhitespaceOptions(vm, obj, false)
	require.NoError(t, err)
	assert.Len(t, opts, 1) // null still creates a NoColor{} option
}

// --- jsToBorder edge cases ---

func TestJsToBorder_NullValue(t *testing.T) {
	vm := goja.New()
	b := jsToBorder(vm, goja.Null())
	assert.Equal(t, "", b.Top)
	assert.Equal(t, "", b.Bottom)
}

func TestJsToBorder_UndefinedValue(t *testing.T) {
	vm := goja.New()
	b := jsToBorder(vm, goja.Undefined())
	assert.Equal(t, "", b.Top)
}

func TestJsToBorder_PartialProperties(t *testing.T) {
	vm := goja.New()
	obj := vm.NewObject()
	_ = obj.Set("top", "-")
	// Leave others as default
	b := jsToBorder(vm, obj)
	assert.Equal(t, "-", b.Top)
	assert.Equal(t, "", b.Bottom)
}

func TestJsToBorder_NullProperties(t *testing.T) {
	vm := goja.New()
	obj := vm.NewObject()
	_ = obj.Set("top", goja.Null())
	_ = obj.Set("bottom", goja.Null())
	b := jsToBorder(vm, obj)
	assert.Equal(t, "", b.Top)
	assert.Equal(t, "", b.Bottom)
}

// --- Style method nil state paths ---

func TestStyleMethods_NilState(t *testing.T) {
	vm, _ := setupModule(t)

	// Create an object that looks like a style but has no __style_state
	_, err := vm.RunString(`
		var fakeStyle = lipgloss.newStyle();
		// Corrupt: remove internal state
		delete fakeStyle.__style_state;
	`)
	require.NoError(t, err)

	// All methods should handle nil state gracefully
	methods := []string{
		"fakeStyle.render('test')",
		"fakeStyle.copy()",
		"fakeStyle.inherit(lipgloss.newStyle())",
		"fakeStyle.string()",
		"fakeStyle.value()",
		"fakeStyle.bold(true)",
		"fakeStyle.italic(true)",
		"fakeStyle.underline(true)",
		"fakeStyle.strikethrough(true)",
		"fakeStyle.blink(true)",
		"fakeStyle.faint(true)",
		"fakeStyle.reverse(true)",
		"fakeStyle.foreground('#FF0000')",
		"fakeStyle.background('#FF0000')",
		"fakeStyle.width(10)",
		"fakeStyle.height(10)",
		"fakeStyle.padding(1)",
		"fakeStyle.margin(1)",
		"fakeStyle.border(lipgloss.normalBorder())",
		"fakeStyle.borderTop(true)",
		"fakeStyle.align(0.5)",
		"fakeStyle.alignHorizontal(0.5)",
		"fakeStyle.alignVertical(0.5)",
		"fakeStyle.unsetBold()",
	}

	for _, expr := range methods {
		t.Run(expr, func(t *testing.T) {
			result, err := vm.RunString(expr)
			require.NoError(t, err)
			// nil state methods return undefined or empty string
			if result != nil && !goja.IsUndefined(result) {
				// render returns ""
				assert.True(t, result.String() == "" || goja.IsUndefined(result))
			}
		})
	}
}

// --- Error state propagation ---

func TestStyleMethods_ErrorStatePropagation(t *testing.T) {
	vm, _ := setupModule(t)

	// Create an error style
	_, err := vm.RunString(`var errStyle = lipgloss.newStyle().width(-10);`)
	require.NoError(t, err)

	// Methods on error style should propagate the error
	tests := []struct {
		expr string
	}{
		{`errStyle.bold(true).hasError`},
		{`errStyle.italic(true).hasError`},
		{`errStyle.foreground('#FF0000').hasError`},
		{`errStyle.width(10).hasError`},
		{`errStyle.padding(1).hasError`},
		{`errStyle.margin(1).hasError`},
		{`errStyle.border(lipgloss.normalBorder()).hasError`},
		{`errStyle.borderTop(true).hasError`},
		{`errStyle.align(0.5).hasError`},
		{`errStyle.copy().hasError`},
		{`errStyle.inherit(lipgloss.newStyle()).hasError`},
		{`errStyle.unsetBold().hasError`},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			result, err := vm.RunString(tt.expr)
			require.NoError(t, err)
			assert.True(t, result.ToBoolean(), "Expected error to propagate for: "+tt.expr)
		})
	}

	// Render on error style returns empty
	result, err := vm.RunString(`errStyle.render('test')`)
	require.NoError(t, err)
	assert.Equal(t, "", result.String())
}

// --- getState accessor properties ---

func TestStyle_HasError_NoState(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var obj = lipgloss.newStyle();
		delete obj.__style_state;
		obj.hasError
	`)
	require.NoError(t, err)
	assert.False(t, result.ToBoolean())
}

func TestStyle_Error_NoState(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var obj = lipgloss.newStyle();
		delete obj.__style_state;
		obj.error
	`)
	require.NoError(t, err)
	assert.True(t, goja.IsNull(result))
}

func TestStyle_ErrorCode_NoState(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var obj = lipgloss.newStyle();
		delete obj.__style_state;
		obj.errorCode
	`)
	require.NoError(t, err)
	assert.True(t, goja.IsNull(result))
}

func TestStyle_Error_NoError(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`lipgloss.newStyle().error`)
	require.NoError(t, err)
	assert.True(t, goja.IsNull(result))
}

func TestStyle_ErrorCode_NoError(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`lipgloss.newStyle().errorCode`)
	require.NoError(t, err)
	assert.True(t, goja.IsNull(result))
}

func TestStyle_ErrorMessage_WithError(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`lipgloss.newStyle().width(-10).error`)
	require.NoError(t, err)
	assert.Contains(t, result.String(), "LG002")
	assert.Contains(t, result.String(), "dimension must be non-negative")
}

func TestStyle_ErrorCode_WithError(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`lipgloss.newStyle().width(-10).errorCode`)
	require.NoError(t, err)
	assert.Equal(t, "LG002", result.String())
}

// --- Inherit method ---

func TestStyle_Inherit_NoArgs(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var s = lipgloss.newStyle().bold(true);
		var r = s.inherit();
		r === s
	`)
	require.NoError(t, err)
	assert.True(t, result.ToBoolean(), "inherit with no args should return this")
}

func TestStyle_Inherit_WithValidStyle(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var base = lipgloss.newStyle().bold(true);
		var other = lipgloss.newStyle().italic(true);
		var inherited = base.inherit(other);
		inherited.hasError === false
	`)
	require.NoError(t, err)
	assert.True(t, result.ToBoolean())
}

func TestStyle_Inherit_WithNonStyleObject(t *testing.T) {
	vm, _ := setupModule(t)

	// Object without __style_state — inherit should return a new style (no-op inherit)
	result, err := vm.RunString(`
		var base = lipgloss.newStyle().bold(true);
		var notAStyle = {};
		var inherited = base.inherit(notAStyle);
		inherited.hasError === false
	`)
	require.NoError(t, err)
	assert.True(t, result.ToBoolean())
}

func TestStyle_Inherit_WithObjectHavingInvalidState(t *testing.T) {
	vm, _ := setupModule(t)

	// Object with wrong type for __style_state
	result, err := vm.RunString(`
		var base = lipgloss.newStyle().bold(true);
		var badObj = { __style_state: "not_a_state" };
		var inherited = base.inherit(badObj);
		inherited.hasError === false
	`)
	require.NoError(t, err)
	assert.True(t, result.ToBoolean())
}

// --- String and Value methods ---

func TestStyle_StringMethod(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var style = lipgloss.newStyle().bold(true);
		typeof style.string() === 'string'
	`)
	require.NoError(t, err)
	assert.True(t, result.ToBoolean())
}

func TestStyle_ValueMethod(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var style = lipgloss.newStyle().bold(true);
		typeof style.value() === 'string'
	`)
	require.NoError(t, err)
	assert.True(t, result.ToBoolean())
}

// --- Bool methods: default true ---

func TestBoolMethods_DefaultTrue(t *testing.T) {
	vm, _ := setupModule(t)

	// Call bool methods with no arguments — should default to true
	methods := []string{
		"bold", "italic", "underline", "strikethrough",
		"blink", "faint", "reverse", "inline",
		"underlineSpaces", "strikethroughSpaces",
	}

	for _, m := range methods {
		t.Run(m, func(t *testing.T) {
			result, err := vm.RunString(`
				var s = lipgloss.newStyle().` + m + `();
				s.hasError === false
			`)
			require.NoError(t, err)
			assert.True(t, result.ToBoolean())
		})
	}
}

// --- Border side methods: default true ---

func TestBorderSideMethods_DefaultTrue(t *testing.T) {
	vm, _ := setupModule(t)

	methods := []string{"borderTop", "borderRight", "borderBottom", "borderLeft"}

	for _, m := range methods {
		t.Run(m, func(t *testing.T) {
			result, err := vm.RunString(`
				var s = lipgloss.newStyle().` + m + `();
				s.hasError === false
			`)
			require.NoError(t, err)
			assert.True(t, result.ToBoolean())
		})
	}
}

// --- Color methods: no args returns this ---

func TestColorMethods_NoArgs(t *testing.T) {
	vm, _ := setupModule(t)

	methods := []string{
		"foreground", "background",
		"borderForeground", "borderBackground",
		"borderTopForeground", "borderRightForeground",
		"borderBottomForeground", "borderLeftForeground",
		"borderTopBackground", "borderRightBackground",
		"borderBottomBackground", "borderLeftBackground",
	}

	for _, m := range methods {
		t.Run(m, func(t *testing.T) {
			result, err := vm.RunString(`
				var s = lipgloss.newStyle();
				var r = s.` + m + `();
				r === s
			`)
			require.NoError(t, err)
			assert.True(t, result.ToBoolean(), m+" with no args should return this")
		})
	}
}

// --- Color error propagation ---

func TestColorMethods_InvalidColor(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var s = lipgloss.newStyle().foreground('#GGGGGG');
		JSON.stringify({
			hasError: s.hasError,
			errorCode: s.errorCode
		})
	`)
	require.NoError(t, err)
	assert.Contains(t, result.String(), `"hasError":true`)
	assert.Contains(t, result.String(), `"errorCode":"LG001"`)
}

// --- Scalar methods: no args returns this ---

func TestScalarMethods_NoArgs(t *testing.T) {
	vm, _ := setupModule(t)

	methods := []string{
		"width", "height", "maxWidth", "maxHeight",
		"paddingTop", "paddingRight", "paddingBottom", "paddingLeft",
		"marginTop", "marginRight", "marginBottom", "marginLeft",
		"tabWidth",
	}

	for _, m := range methods {
		t.Run(m, func(t *testing.T) {
			result, err := vm.RunString(`
				var s = lipgloss.newStyle();
				var r = s.` + m + `();
				r === s
			`)
			require.NoError(t, err)
			assert.True(t, result.ToBoolean(), m+" with no args should return this")
		})
	}
}

// --- tabWidth can be negative (NoTabConversion = -1) ---

func TestTabWidth_NegativeAllowed(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var s = lipgloss.newStyle().tabWidth(-1);
		s.hasError === false
	`)
	require.NoError(t, err)
	assert.True(t, result.ToBoolean())
}

// --- Padding variadic ---

func TestPadding_ZeroArgs(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var s = lipgloss.newStyle().padding();
		s.hasError === false
	`)
	require.NoError(t, err)
	assert.True(t, result.ToBoolean())
}

func TestPadding_TwoArgs(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var s = lipgloss.newStyle().padding(1, 2);
		s.render('test').length > 4
	`)
	require.NoError(t, err)
	assert.True(t, result.ToBoolean())
}

func TestPadding_ThreeArgs(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var s = lipgloss.newStyle().padding(1, 2, 3);
		s.hasError === false
	`)
	require.NoError(t, err)
	assert.True(t, result.ToBoolean())
}

func TestPadding_FourArgs(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var s = lipgloss.newStyle().padding(1, 2, 3, 4);
		s.hasError === false
	`)
	require.NoError(t, err)
	assert.True(t, result.ToBoolean())
}

// --- Margin variadic ---

func TestMargin_ZeroArgs(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var s = lipgloss.newStyle().margin();
		s.hasError === false
	`)
	require.NoError(t, err)
	assert.True(t, result.ToBoolean())
}

func TestMargin_TwoArgs(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var s = lipgloss.newStyle().margin(1, 2);
		s.hasError === false
	`)
	require.NoError(t, err)
	assert.True(t, result.ToBoolean())
}

func TestMargin_ThreeArgs(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var s = lipgloss.newStyle().margin(1, 2, 3);
		s.hasError === false
	`)
	require.NoError(t, err)
	assert.True(t, result.ToBoolean())
}

func TestMargin_FourArgs(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var s = lipgloss.newStyle().margin(1, 2, 3, 4);
		s.hasError === false
	`)
	require.NoError(t, err)
	assert.True(t, result.ToBoolean())
}

func TestMargin_Negative(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var s = lipgloss.newStyle().margin(-1);
		s.hasError
	`)
	require.NoError(t, err)
	assert.True(t, result.ToBoolean())
}

// --- Border method ---

func TestBorder_NoArgs(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var s = lipgloss.newStyle();
		var r = s.border();
		r === s
	`)
	require.NoError(t, err)
	assert.True(t, result.ToBoolean())
}

func TestBorder_WithSides(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var s = lipgloss.newStyle().border(lipgloss.normalBorder(), true, false, true, false);
		s.hasError === false
	`)
	require.NoError(t, err)
	assert.True(t, result.ToBoolean())
}

func TestBorder_NullBorder(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var s = lipgloss.newStyle().border(null);
		s.hasError === false
	`)
	require.NoError(t, err)
	assert.True(t, result.ToBoolean())
}

// --- Alignment ---

func TestAlignment_NoArgs(t *testing.T) {
	vm, _ := setupModule(t)

	methods := []string{"align", "alignHorizontal", "alignVertical"}
	for _, m := range methods {
		t.Run(m, func(t *testing.T) {
			result, err := vm.RunString(`
				var s = lipgloss.newStyle();
				var r = s.` + m + `();
				r === s
			`)
			require.NoError(t, err)
			assert.True(t, result.ToBoolean())
		})
	}
}

func TestAlignment_OutOfRange(t *testing.T) {
	vm, _ := setupModule(t)

	tests := []struct {
		name string
		expr string
	}{
		{"align negative", `lipgloss.newStyle().align(-0.1).hasError`},
		{"align > 1", `lipgloss.newStyle().align(1.1).hasError`},
		{"alignHorizontal negative", `lipgloss.newStyle().alignHorizontal(-0.5).hasError`},
		{"alignVertical > 1", `lipgloss.newStyle().alignVertical(2.0).hasError`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := vm.RunString(tt.expr)
			require.NoError(t, err)
			assert.True(t, result.ToBoolean())
		})
	}
}

func TestAlignment_OutOfRange_ErrorCode(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var s = lipgloss.newStyle().align(5);
		s.errorCode
	`)
	require.NoError(t, err)
	assert.Equal(t, "LG003", result.String())
}

// --- Unset methods ---

func TestUnsetMethods(t *testing.T) {
	vm, _ := setupModule(t)

	// Test a representative sample of unset methods
	methods := []string{
		"unsetBold", "unsetItalic", "unsetUnderline", "unsetStrikethrough",
		"unsetBlink", "unsetFaint", "unsetReverse", "unsetInline",
		"unsetForeground", "unsetBackground",
		"unsetBorderTop", "unsetBorderRight", "unsetBorderBottom", "unsetBorderLeft",
		"unsetBorderForeground", "unsetBorderBackground",
		"unsetPadding", "unsetMargins",
		"unsetWidth", "unsetHeight", "unsetMaxWidth", "unsetMaxHeight",
		"unsetAlign", "unsetAlignHorizontal", "unsetAlignVertical",
		"unsetTabWidth", "unsetUnderlineSpaces", "unsetStrikethroughSpaces",
		"unsetBorderTopForeground", "unsetBorderRightForeground",
		"unsetBorderBottomForeground", "unsetBorderLeftForeground",
		"unsetBorderTopBackground", "unsetBorderRightBackground",
		"unsetBorderBottomBackground", "unsetBorderLeftBackground",
		"unsetMarginTop", "unsetMarginRight", "unsetMarginBottom", "unsetMarginLeft",
		"unsetPaddingTop", "unsetPaddingRight", "unsetPaddingBottom", "unsetPaddingLeft",
	}

	for _, m := range methods {
		t.Run(m, func(t *testing.T) {
			result, err := vm.RunString(`
				var s = lipgloss.newStyle().bold(true).` + m + `();
				s.hasError === false
			`)
			require.NoError(t, err)
			assert.True(t, result.ToBoolean(), m+" should not cause error")
		})
	}
}

// --- Utilities: edge cases ---

func TestJoinVertical_InsufficientArgs(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`lipgloss.joinVertical(0)`)
	require.NoError(t, err)
	assert.Equal(t, "", result.String())
}

func TestPlace_WithWhitespaceOptions(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		lipgloss.place(10, 3, 0.5, 0.5, 'Hi', {
			whitespaceChars: '.',
			whitespaceForeground: '#FF0000',
			whitespaceBackground: '#0000FF'
		});
	`)
	require.NoError(t, err)
	assert.Contains(t, result.String(), "Hi")
}

func TestPlaceHorizontal_Basic(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`lipgloss.placeHorizontal(20, 0.5, 'center')`)
	require.NoError(t, err)
	assert.Contains(t, result.String(), "center")
}

func TestPlaceHorizontal_InsufficientArgs(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`lipgloss.placeHorizontal(20)`)
	require.NoError(t, err)
	assert.Equal(t, "", result.String())
}

func TestPlaceHorizontal_WithOptions(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		lipgloss.placeHorizontal(20, 0.5, 'Hi', { whitespaceChars: '.' })
	`)
	require.NoError(t, err)
	assert.Contains(t, result.String(), "Hi")
}

func TestPlaceVertical_Basic(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`lipgloss.placeVertical(5, 0.5, 'middle')`)
	require.NoError(t, err)
	assert.Contains(t, result.String(), "middle")
}

func TestPlaceVertical_InsufficientArgs(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`lipgloss.placeVertical(5)`)
	require.NoError(t, err)
	assert.Equal(t, "", result.String())
}

func TestPlaceVertical_WithOptions(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		lipgloss.placeVertical(5, 0.5, 'Hi', { whitespaceChars: '.' })
	`)
	require.NoError(t, err)
	assert.Contains(t, result.String(), "Hi")
}

// --- HasDarkBackground ---

func TestHasDarkBackground(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`typeof lipgloss.hasDarkBackground()`)
	require.NoError(t, err)
	assert.Equal(t, "boolean", result.String())
}

// --- Size with no args ---

func TestSize_NoArgs(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`
		var s = lipgloss.size();
		JSON.stringify(s)
	`)
	require.NoError(t, err)
	assert.Contains(t, result.String(), `"width":0`)
	assert.Contains(t, result.String(), `"height":0`)
}

// --- Height with no args ---

func TestHeight_NoArgs(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`lipgloss.height()`)
	require.NoError(t, err)
	assert.Equal(t, int64(0), result.ToInteger())
}

// --- Additional border factories ---

func TestBorderFactories_BlockAndHalfBlock(t *testing.T) {
	vm, _ := setupModule(t)

	// Test block border factory
	result, err := vm.RunString(`
		var b = lipgloss.blockBorder();
		typeof b.top === 'string' && typeof b.bottom === 'string'
	`)
	require.NoError(t, err)
	assert.True(t, result.ToBoolean())

	// Test outerHalfBlockBorder
	result, err = vm.RunString(`
		var b = lipgloss.outerHalfBlockBorder();
		typeof b.top === 'string'
	`)
	require.NoError(t, err)
	assert.True(t, result.ToBoolean())

	// Test innerHalfBlockBorder
	result, err = vm.RunString(`
		var b = lipgloss.innerHalfBlockBorder();
		typeof b.top === 'string'
	`)
	require.NoError(t, err)
	assert.True(t, result.ToBoolean())
}

// --- Place with whitespace options error ---

func TestPlace_WithInvalidWhitespaceOptions(t *testing.T) {
	vm, _ := setupModule(t)

	_, err := vm.RunString(`
		lipgloss.place(10, 3, 0.5, 0.5, 'Hi', {
			whitespaceForeground: '#GGGGGG'
		});
	`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid hex color")
}

func TestPlaceHorizontal_WithInvalidWhitespaceOptions(t *testing.T) {
	vm, _ := setupModule(t)

	_, err := vm.RunString(`
		lipgloss.placeHorizontal(20, 0.5, 'Hi', {
			whitespaceBackground: '#GGGGGG'
		});
	`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid hex color")
}

func TestPlaceVertical_WithInvalidWhitespaceOptions(t *testing.T) {
	vm, _ := setupModule(t)

	_, err := vm.RunString(`
		lipgloss.placeVertical(5, 0.5, 'Hi', {
			whitespaceForeground: '#GGGGGG'
		});
	`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid hex color")
}

// --- NoTabConversion constant ---

func TestNoTabConversion_Constant(t *testing.T) {
	vm, _ := setupModule(t)

	result, err := vm.RunString(`lipgloss.NoTabConversion`)
	require.NoError(t, err)
	assert.Equal(t, int64(-1), result.ToInteger())
}

// --- Border color methods ---

func TestBorderColorMethods(t *testing.T) {
	vm, _ := setupModule(t)

	methods := []string{
		"borderTopForeground", "borderRightForeground",
		"borderBottomForeground", "borderLeftForeground",
		"borderTopBackground", "borderRightBackground",
		"borderBottomBackground", "borderLeftBackground",
	}

	for _, m := range methods {
		t.Run(m, func(t *testing.T) {
			result, err := vm.RunString(`
				var s = lipgloss.newStyle().` + m + `('#FF0000');
				s.hasError === false
			`)
			require.NoError(t, err)
			assert.True(t, result.ToBoolean())
		})
	}
}

// --- Negative scalar dimensions ---

func TestNegativeScalarDimensions(t *testing.T) {
	vm, _ := setupModule(t)

	methods := []string{
		"height", "maxWidth", "maxHeight",
		"paddingTop", "paddingRight", "paddingBottom", "paddingLeft",
		"marginTop", "marginRight", "marginBottom", "marginLeft",
	}

	for _, m := range methods {
		t.Run(m, func(t *testing.T) {
			result, err := vm.RunString(`lipgloss.newStyle().` + m + `(-1).hasError`)
			require.NoError(t, err)
			assert.True(t, result.ToBoolean(), m+" with negative should have error")
		})
	}
}

// --- extractInts ---

func TestExtractInts_Empty(t *testing.T) {
	result := extractInts(nil)
	assert.Nil(t, result)
}

func TestExtractInts_Multiple(t *testing.T) {
	vm := goja.New()
	args := []goja.Value{vm.ToValue(1), vm.ToValue(2), vm.ToValue(3)}
	result := extractInts(args)
	assert.Equal(t, []int{1, 2, 3}, result)
}
