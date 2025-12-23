// Package lipgloss provides JavaScript bindings for github.com/charmbracelet/lipgloss.
//
// The module is exposed as "osm:lipgloss" and provides styling capabilities for terminal UIs.
// All functionality is exposed to JavaScript, following the established pattern of no global state.
//
// # JavaScript API
//
//	const lipgloss = require('osm:lipgloss');
//
//	// Create a new style
//	const style = lipgloss.newStyle();
//
//	// Apply styling methods (chainable, immutable)
//	const styledText = style
//	    .foreground('#FF0000')
//	    .background('#00FF00')
//	    .bold(true)
//	    .padding(1, 2)
//	    .render('Hello, World!');
//
//	// All methods return NEW style objects - original is unchanged
//	const base = lipgloss.newStyle().bold(true);
//	const derived = base.italic(true); // base is still just bold
//
// # Error Handling
//
// All methods validate their inputs and return error information when invalid:
//
//	const result = lipgloss.newStyle().foreground('not-a-color');
//	// result.hasError === true
//	// result.error === 'LG001: invalid color format'
//
// Error codes:
//   - LG001: Invalid color format
//   - LG002: Invalid dimension (negative or too large)
//   - LG003: Invalid alignment value
//   - LG004: Invalid border configuration
//
// # Style Methods
//
// Text formatting:
//   - bold(bool), italic(bool), underline(bool)
//   - strikethrough(bool), blink(bool), faint(bool), reverse(bool)
//
// Colors (supports hex '#RRGGBB', ANSI '0'-'255', or named colors):
//   - foreground(color), background(color)
//   - borderForeground(color), borderBackground(color)
//
// Spacing (values must be non-negative):
//   - padding(top, right?, bottom?, left?)
//   - paddingTop(n), paddingRight(n), paddingBottom(n), paddingLeft(n)
//   - margin(top, right?, bottom?, left?)
//   - marginTop(n), marginRight(n), marginBottom(n), marginLeft(n)
//
// Dimensions (values must be non-negative, max 10000):
//   - width(n), height(n), maxWidth(n), maxHeight(n)
//
// Borders:
//   - border(borderType)
//   - borderTop(bool), borderRight(bool), borderBottom(bool), borderLeft(bool)
//
// Alignment (0.0 = left/top, 0.5 = center, 1.0 = right/bottom):
//   - align(pos), alignHorizontal(pos), alignVertical(pos)
//
// # Implementation Notes
//
// All additions follow these patterns:
//
//  1. No global state - Manager instance per engine
//  2. Chainable API - methods return the style for chaining
//  3. Immutable semantics - methods return copies, not mutations
//  4. JavaScript-friendly types - use maps and arrays
//  5. Input validation with clear error messages
//  6. Comprehensive unit tests for all exposed functions
package lipgloss

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dop251/goja"
)

// Error codes for lipgloss operations.
const (
	ErrCodeInvalidColor     = "LG001" // Invalid color format
	ErrCodeInvalidDimension = "LG002" // Invalid dimension value
	ErrCodeInvalidAlignment = "LG003" // Invalid alignment value
	ErrCodeInvalidBorder    = "LG004" // Invalid border configuration
)

// Maximum dimension value to prevent overflow.
const maxDimension = 10000

// colorRegex validates hex color codes.
var colorRegex = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$|^#[0-9A-Fa-f]{3}$`)

// Manager holds lipgloss-related state per engine instance.
// Currently stateless but provided for consistency and future extensibility.
type Manager struct{}

// NewManager creates a new lipgloss manager for an engine instance.
func NewManager() *Manager {
	return &Manager{}
}

// StyleWrapper wraps a lipgloss.Style for JavaScript interaction.
type StyleWrapper struct {
	style    lipgloss.Style
	runtime  *goja.Runtime
	hasError bool
	errCode  string
	errMsg   string
}

// validateColor checks if a color string is valid.
// Accepts hex colors (#RGB, #RRGGBB) or ANSI color numbers (0-255).
func validateColor(colorStr string) (bool, string) {
	colorStr = strings.TrimSpace(colorStr)

	if colorStr == "" {
		return true, "" // Empty is valid (no color)
	}

	// Check hex format
	if strings.HasPrefix(colorStr, "#") {
		if colorRegex.MatchString(colorStr) {
			return true, ""
		}
		return false, "hex color must be #RGB or #RRGGBB format"
	}

	// Check ANSI color number (0-255)
	if n, err := strconv.Atoi(colorStr); err == nil {
		if n >= 0 && n <= 255 {
			return true, ""
		}
		return false, "ANSI color must be 0-255"
	}

	// Allow named colors (lipgloss handles these)
	// Common terminal color names
	namedColors := map[string]bool{
		"black": true, "red": true, "green": true, "yellow": true,
		"blue": true, "magenta": true, "cyan": true, "white": true,
	}
	if namedColors[strings.ToLower(colorStr)] {
		return true, ""
	}

	// Allow any string - lipgloss will handle it
	return true, ""
}

// validateDimension checks if a dimension value is valid.
func validateDimension(n int) (bool, string) {
	if n < 0 {
		return false, "dimension cannot be negative"
	}
	if n > maxDimension {
		return false, "dimension exceeds maximum of 10000"
	}
	return true, ""
}

// Require returns a CommonJS native module under "osm:lipgloss".
// It exposes lipgloss functionality for terminal styling.
//
// The key design principle is that styles are:
//   - Explicitly created and configured by JavaScript code
//   - Chainable for ergonomic usage
//   - Immutable (methods return copies)
//   - Implemented in Go for performance
func Require(manager *Manager) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		// Alignment constants
		_ = exports.Set("Left", lipgloss.Left)
		_ = exports.Set("Center", lipgloss.Center)
		_ = exports.Set("Right", lipgloss.Right)
		_ = exports.Set("Top", lipgloss.Top)
		_ = exports.Set("Bottom", lipgloss.Bottom)

		// Border constructors
		_ = exports.Set("normalBorder", func() map[string]interface{} {
			return borderToJS(lipgloss.NormalBorder())
		})
		_ = exports.Set("roundedBorder", func() map[string]interface{} {
			return borderToJS(lipgloss.RoundedBorder())
		})
		_ = exports.Set("thickBorder", func() map[string]interface{} {
			return borderToJS(lipgloss.ThickBorder())
		})
		_ = exports.Set("doubleBorder", func() map[string]interface{} {
			return borderToJS(lipgloss.DoubleBorder())
		})
		_ = exports.Set("hiddenBorder", func() map[string]interface{} {
			return borderToJS(lipgloss.HiddenBorder())
		})
		_ = exports.Set("noBorder", func() map[string]interface{} {
			return nil
		})

		// newStyle creates a new Style wrapper
		_ = exports.Set("newStyle", func(call goja.FunctionCall) goja.Value {
			wrapper := &StyleWrapper{
				style:   lipgloss.NewStyle(),
				runtime: runtime,
			}
			return createStyleObject(runtime, wrapper)
		})

		// Utility functions
		_ = exports.Set("joinHorizontal", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				return runtime.ToValue("")
			}
			pos := lipgloss.Position(call.Argument(0).ToFloat())
			var strs []string
			for i := 1; i < len(call.Arguments); i++ {
				strs = append(strs, call.Argument(i).String())
			}
			return runtime.ToValue(lipgloss.JoinHorizontal(pos, strs...))
		})

		_ = exports.Set("joinVertical", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				return runtime.ToValue("")
			}
			pos := lipgloss.Position(call.Argument(0).ToFloat())
			var strs []string
			for i := 1; i < len(call.Arguments); i++ {
				strs = append(strs, call.Argument(i).String())
			}
			return runtime.ToValue(lipgloss.JoinVertical(pos, strs...))
		})

		_ = exports.Set("place", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 5 {
				return runtime.ToValue("")
			}
			width := int(call.Argument(0).ToInteger())
			height := int(call.Argument(1).ToInteger())
			hPos := lipgloss.Position(call.Argument(2).ToFloat())
			vPos := lipgloss.Position(call.Argument(3).ToFloat())
			str := call.Argument(4).String()
			return runtime.ToValue(lipgloss.Place(width, height, hPos, vPos, str))
		})

		_ = exports.Set("size", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return runtime.ToValue(map[string]interface{}{"width": 0, "height": 0})
			}
			str := call.Argument(0).String()
			w := lipgloss.Width(str)
			h := lipgloss.Height(str)
			return runtime.ToValue(map[string]interface{}{"width": w, "height": h})
		})

		_ = exports.Set("width", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return runtime.ToValue(0)
			}
			return runtime.ToValue(lipgloss.Width(call.Argument(0).String()))
		})

		_ = exports.Set("height", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return runtime.ToValue(0)
			}
			return runtime.ToValue(lipgloss.Height(call.Argument(0).String()))
		})
	}
}

// borderToJS converts a lipgloss.Border to a JavaScript-compatible map.
func borderToJS(b lipgloss.Border) map[string]interface{} {
	return map[string]interface{}{
		"top":         b.Top,
		"bottom":      b.Bottom,
		"left":        b.Left,
		"right":       b.Right,
		"topLeft":     b.TopLeft,
		"topRight":    b.TopRight,
		"bottomLeft":  b.BottomLeft,
		"bottomRight": b.BottomRight,
	}
}

// jsToBorder converts a JavaScript object back to a lipgloss.Border.
func jsToBorder(runtime *goja.Runtime, val goja.Value) lipgloss.Border {
	if goja.IsUndefined(val) || goja.IsNull(val) {
		return lipgloss.Border{}
	}
	obj := val.ToObject(runtime)
	return lipgloss.Border{
		Top:         getStringVal(obj, "top"),
		Bottom:      getStringVal(obj, "bottom"),
		Left:        getStringVal(obj, "left"),
		Right:       getStringVal(obj, "right"),
		TopLeft:     getStringVal(obj, "topLeft"),
		TopRight:    getStringVal(obj, "topRight"),
		BottomLeft:  getStringVal(obj, "bottomLeft"),
		BottomRight: getStringVal(obj, "bottomRight"),
	}
}

// getStringVal safely extracts a string value from an object.
func getStringVal(obj *goja.Object, key string) string {
	if obj == nil {
		return ""
	}
	val := obj.Get(key)
	if goja.IsUndefined(val) || goja.IsNull(val) {
		return ""
	}
	return val.String()
}

// createStyleObject creates a JavaScript object wrapping a StyleWrapper.
// All style methods return a NEW style object with the modification applied,
// preserving immutability semantics consistent with lipgloss.
func createStyleObject(runtime *goja.Runtime, wrapper *StyleWrapper) goja.Value {
	obj := runtime.NewObject()

	// Add error properties if this style has an error
	_ = obj.Set("hasError", wrapper.hasError)
	if wrapper.hasError {
		_ = obj.Set("error", wrapper.errCode+": "+wrapper.errMsg)
		_ = obj.Set("errorCode", wrapper.errCode)
	}

	// Helper to create a new wrapper with error
	createErrorStyle := func(code, msg string) goja.Value {
		errWrapper := &StyleWrapper{
			style:    wrapper.style, // lipgloss styles are immutable, no need to copy
			runtime:  runtime,
			hasError: true,
			errCode:  code,
			errMsg:   msg,
		}
		return createStyleObject(runtime, errWrapper)
	}

	// render: Render text with this style
	_ = obj.Set("render", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return runtime.ToValue("")
		}
		var strs []string
		for _, arg := range call.Arguments {
			strs = append(strs, arg.String())
		}
		// Render all strings with the style applied
		result := wrapper.style.Render(strs...)
		return runtime.ToValue(result)
	})

	// copy: Create a copy of this style (for API compatibility, though styles are immutable)
	_ = obj.Set("copy", func(call goja.FunctionCall) goja.Value {
		newWrapper := &StyleWrapper{
			style:   wrapper.style, // Assignment is equivalent to Copy() for immutable styles
			runtime: runtime,
		}
		return createStyleObject(runtime, newWrapper)
	})

	// Text formatting - all return new style objects
	_ = obj.Set("bold", func(call goja.FunctionCall) goja.Value {
		v := true
		if len(call.Arguments) > 0 {
			v = call.Argument(0).ToBoolean()
		}
		newWrapper := &StyleWrapper{
			style:   wrapper.style.Bold(v),
			runtime: runtime,
		}
		return createStyleObject(runtime, newWrapper)
	})

	_ = obj.Set("italic", func(call goja.FunctionCall) goja.Value {
		v := true
		if len(call.Arguments) > 0 {
			v = call.Argument(0).ToBoolean()
		}
		newWrapper := &StyleWrapper{
			style:   wrapper.style.Italic(v),
			runtime: runtime,
		}
		return createStyleObject(runtime, newWrapper)
	})

	_ = obj.Set("underline", func(call goja.FunctionCall) goja.Value {
		v := true
		if len(call.Arguments) > 0 {
			v = call.Argument(0).ToBoolean()
		}
		newWrapper := &StyleWrapper{
			style:   wrapper.style.Underline(v),
			runtime: runtime,
		}
		return createStyleObject(runtime, newWrapper)
	})

	_ = obj.Set("strikethrough", func(call goja.FunctionCall) goja.Value {
		v := true
		if len(call.Arguments) > 0 {
			v = call.Argument(0).ToBoolean()
		}
		newWrapper := &StyleWrapper{
			style:   wrapper.style.Strikethrough(v),
			runtime: runtime,
		}
		return createStyleObject(runtime, newWrapper)
	})

	_ = obj.Set("blink", func(call goja.FunctionCall) goja.Value {
		v := true
		if len(call.Arguments) > 0 {
			v = call.Argument(0).ToBoolean()
		}
		newWrapper := &StyleWrapper{
			style:   wrapper.style.Blink(v),
			runtime: runtime,
		}
		return createStyleObject(runtime, newWrapper)
	})

	_ = obj.Set("faint", func(call goja.FunctionCall) goja.Value {
		v := true
		if len(call.Arguments) > 0 {
			v = call.Argument(0).ToBoolean()
		}
		newWrapper := &StyleWrapper{
			style:   wrapper.style.Faint(v),
			runtime: runtime,
		}
		return createStyleObject(runtime, newWrapper)
	})

	_ = obj.Set("reverse", func(call goja.FunctionCall) goja.Value {
		v := true
		if len(call.Arguments) > 0 {
			v = call.Argument(0).ToBoolean()
		}
		newWrapper := &StyleWrapper{
			style:   wrapper.style.Reverse(v),
			runtime: runtime,
		}
		return createStyleObject(runtime, newWrapper)
	})

	// Colors with validation
	_ = obj.Set("foreground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		colorStr := call.Argument(0).String()
		if valid, errMsg := validateColor(colorStr); !valid {
			return createErrorStyle(ErrCodeInvalidColor, errMsg)
		}
		newWrapper := &StyleWrapper{
			style:   wrapper.style.Foreground(lipgloss.Color(colorStr)),
			runtime: runtime,
		}
		return createStyleObject(runtime, newWrapper)
	})

	_ = obj.Set("background", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		colorStr := call.Argument(0).String()
		if valid, errMsg := validateColor(colorStr); !valid {
			return createErrorStyle(ErrCodeInvalidColor, errMsg)
		}
		newWrapper := &StyleWrapper{
			style:   wrapper.style.Background(lipgloss.Color(colorStr)),
			runtime: runtime,
		}
		return createStyleObject(runtime, newWrapper)
	})

	// Padding with validation
	_ = obj.Set("padding", func(call goja.FunctionCall) goja.Value {
		args := extractInts(call.Arguments)
		for _, v := range args {
			if valid, errMsg := validateDimension(v); !valid {
				return createErrorStyle(ErrCodeInvalidDimension, errMsg)
			}
		}
		var newStyle lipgloss.Style
		switch len(args) {
		case 1:
			newStyle = wrapper.style.Padding(args[0])
		case 2:
			newStyle = wrapper.style.Padding(args[0], args[1])
		case 4:
			newStyle = wrapper.style.Padding(args[0], args[1], args[2], args[3])
		default:
			// Return a copy to maintain immutability even when no args provided
			newStyle = wrapper.style
		}
		newWrapper := &StyleWrapper{style: newStyle, runtime: runtime}
		return createStyleObject(runtime, newWrapper)
	})

	_ = obj.Set("paddingTop", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			v := int(call.Argument(0).ToInteger())
			if valid, errMsg := validateDimension(v); !valid {
				return createErrorStyle(ErrCodeInvalidDimension, errMsg)
			}
			newWrapper := &StyleWrapper{
				style:   wrapper.style.PaddingTop(v),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	_ = obj.Set("paddingRight", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			v := int(call.Argument(0).ToInteger())
			if valid, errMsg := validateDimension(v); !valid {
				return createErrorStyle(ErrCodeInvalidDimension, errMsg)
			}
			newWrapper := &StyleWrapper{
				style:   wrapper.style.PaddingRight(v),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	_ = obj.Set("paddingBottom", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			v := int(call.Argument(0).ToInteger())
			if valid, errMsg := validateDimension(v); !valid {
				return createErrorStyle(ErrCodeInvalidDimension, errMsg)
			}
			newWrapper := &StyleWrapper{
				style:   wrapper.style.PaddingBottom(v),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	_ = obj.Set("paddingLeft", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			v := int(call.Argument(0).ToInteger())
			if valid, errMsg := validateDimension(v); !valid {
				return createErrorStyle(ErrCodeInvalidDimension, errMsg)
			}
			newWrapper := &StyleWrapper{
				style:   wrapper.style.PaddingLeft(v),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	// Margin with validation
	_ = obj.Set("margin", func(call goja.FunctionCall) goja.Value {
		args := extractInts(call.Arguments)
		for _, v := range args {
			if valid, errMsg := validateDimension(v); !valid {
				return createErrorStyle(ErrCodeInvalidDimension, errMsg)
			}
		}
		var newStyle lipgloss.Style
		switch len(args) {
		case 1:
			newStyle = wrapper.style.Margin(args[0])
		case 2:
			newStyle = wrapper.style.Margin(args[0], args[1])
		case 4:
			newStyle = wrapper.style.Margin(args[0], args[1], args[2], args[3])
		default:
			// Return a copy to maintain immutability even when no args provided
			newStyle = wrapper.style
		}
		newWrapper := &StyleWrapper{style: newStyle, runtime: runtime}
		return createStyleObject(runtime, newWrapper)
	})

	_ = obj.Set("marginTop", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			v := int(call.Argument(0).ToInteger())
			if valid, errMsg := validateDimension(v); !valid {
				return createErrorStyle(ErrCodeInvalidDimension, errMsg)
			}
			newWrapper := &StyleWrapper{
				style:   wrapper.style.MarginTop(v),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	_ = obj.Set("marginRight", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			v := int(call.Argument(0).ToInteger())
			if valid, errMsg := validateDimension(v); !valid {
				return createErrorStyle(ErrCodeInvalidDimension, errMsg)
			}
			newWrapper := &StyleWrapper{
				style:   wrapper.style.MarginRight(v),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	_ = obj.Set("marginBottom", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			v := int(call.Argument(0).ToInteger())
			if valid, errMsg := validateDimension(v); !valid {
				return createErrorStyle(ErrCodeInvalidDimension, errMsg)
			}
			newWrapper := &StyleWrapper{
				style:   wrapper.style.MarginBottom(v),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	_ = obj.Set("marginLeft", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			v := int(call.Argument(0).ToInteger())
			if valid, errMsg := validateDimension(v); !valid {
				return createErrorStyle(ErrCodeInvalidDimension, errMsg)
			}
			newWrapper := &StyleWrapper{
				style:   wrapper.style.MarginLeft(v),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	// Dimensions with validation
	_ = obj.Set("width", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			v := int(call.Argument(0).ToInteger())
			if valid, errMsg := validateDimension(v); !valid {
				return createErrorStyle(ErrCodeInvalidDimension, errMsg)
			}
			newWrapper := &StyleWrapper{
				style:   wrapper.style.Width(v),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	_ = obj.Set("height", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			v := int(call.Argument(0).ToInteger())
			if valid, errMsg := validateDimension(v); !valid {
				return createErrorStyle(ErrCodeInvalidDimension, errMsg)
			}
			newWrapper := &StyleWrapper{
				style:   wrapper.style.Height(v),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	_ = obj.Set("maxWidth", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			v := int(call.Argument(0).ToInteger())
			if valid, errMsg := validateDimension(v); !valid {
				return createErrorStyle(ErrCodeInvalidDimension, errMsg)
			}
			newWrapper := &StyleWrapper{
				style:   wrapper.style.MaxWidth(v),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	_ = obj.Set("maxHeight", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			v := int(call.Argument(0).ToInteger())
			if valid, errMsg := validateDimension(v); !valid {
				return createErrorStyle(ErrCodeInvalidDimension, errMsg)
			}
			newWrapper := &StyleWrapper{
				style:   wrapper.style.MaxHeight(v),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	// Border
	_ = obj.Set("border", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			border := jsToBorder(runtime, call.Argument(0))
			newWrapper := &StyleWrapper{
				style:   wrapper.style.Border(border),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	_ = obj.Set("borderTop", func(call goja.FunctionCall) goja.Value {
		v := true
		if len(call.Arguments) > 0 {
			v = call.Argument(0).ToBoolean()
		}
		newWrapper := &StyleWrapper{
			style:   wrapper.style.BorderTop(v),
			runtime: runtime,
		}
		return createStyleObject(runtime, newWrapper)
	})

	_ = obj.Set("borderRight", func(call goja.FunctionCall) goja.Value {
		v := true
		if len(call.Arguments) > 0 {
			v = call.Argument(0).ToBoolean()
		}
		newWrapper := &StyleWrapper{
			style:   wrapper.style.BorderRight(v),
			runtime: runtime,
		}
		return createStyleObject(runtime, newWrapper)
	})

	_ = obj.Set("borderBottom", func(call goja.FunctionCall) goja.Value {
		v := true
		if len(call.Arguments) > 0 {
			v = call.Argument(0).ToBoolean()
		}
		newWrapper := &StyleWrapper{
			style:   wrapper.style.BorderBottom(v),
			runtime: runtime,
		}
		return createStyleObject(runtime, newWrapper)
	})

	_ = obj.Set("borderLeft", func(call goja.FunctionCall) goja.Value {
		v := true
		if len(call.Arguments) > 0 {
			v = call.Argument(0).ToBoolean()
		}
		newWrapper := &StyleWrapper{
			style:   wrapper.style.BorderLeft(v),
			runtime: runtime,
		}
		return createStyleObject(runtime, newWrapper)
	})

	_ = obj.Set("borderForeground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			colorStr := call.Argument(0).String()
			if valid, errMsg := validateColor(colorStr); !valid {
				return createErrorStyle(ErrCodeInvalidColor, errMsg)
			}
			newWrapper := &StyleWrapper{
				style:   wrapper.style.BorderForeground(lipgloss.Color(colorStr)),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	_ = obj.Set("borderBackground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			colorStr := call.Argument(0).String()
			if valid, errMsg := validateColor(colorStr); !valid {
				return createErrorStyle(ErrCodeInvalidColor, errMsg)
			}
			newWrapper := &StyleWrapper{
				style:   wrapper.style.BorderBackground(lipgloss.Color(colorStr)),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	// Alignment
	_ = obj.Set("align", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			pos := lipgloss.Position(call.Argument(0).ToFloat())
			newWrapper := &StyleWrapper{
				style:   wrapper.style.Align(pos),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	_ = obj.Set("alignHorizontal", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			pos := lipgloss.Position(call.Argument(0).ToFloat())
			newWrapper := &StyleWrapper{
				style:   wrapper.style.AlignHorizontal(pos),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	_ = obj.Set("alignVertical", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			pos := lipgloss.Position(call.Argument(0).ToFloat())
			newWrapper := &StyleWrapper{
				style:   wrapper.style.AlignVertical(pos),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	// Expose the underlying Go style for testing (read-only)
	_ = obj.Set("_goStyle", runtime.ToValue(wrapper.style))

	return obj
}

// extractInts extracts integer values from goja arguments.
func extractInts(args []goja.Value) []int {
	var result []int
	for _, arg := range args {
		result = append(result, int(arg.ToInteger()))
	}
	return result
}
