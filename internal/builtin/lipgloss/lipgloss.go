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
//	// Apply styling methods (chainable)
//	style.foreground('#FF0000');
//	style.background('#00FF00');
//	style.bold(true);
//	style.italic(true);
//	style.underline(true);
//	style.strikethrough(true);
//	style.blink(true);
//	style.faint(true);
//	style.reverse(true);
//
//	// Set padding and margins
//	style.padding(1, 2, 1, 2);  // top, right, bottom, left
//	style.margin(1, 2, 1, 2);   // top, right, bottom, left
//	style.paddingTop(1);
//	style.paddingRight(2);
//	style.paddingBottom(1);
//	style.paddingLeft(2);
//	style.marginTop(1);
//	style.marginRight(2);
//	style.marginBottom(1);
//	style.marginLeft(2);
//
//	// Set borders
//	style.border(lipgloss.normalBorder());
//	style.borderForeground('#FF0000');
//	style.borderBackground('#00FF00');
//	style.borderTop(true);
//	style.borderRight(true);
//	style.borderBottom(true);
//	style.borderLeft(true);
//
//	// Set dimensions
//	style.width(80);
//	style.height(10);
//	style.maxWidth(100);
//	style.maxHeight(20);
//
//	// Set alignment
//	style.align(lipgloss.Center);  // Left, Center, Right
//	style.alignHorizontal(lipgloss.Center);
//	style.alignVertical(lipgloss.Center);
//
//	// Render text with the style
//	const rendered = style.render('Hello, World!');
//
//	// Copy a style
//	const copy = style.copy();
//
//	// Border types
//	lipgloss.normalBorder()
//	lipgloss.roundedBorder()
//	lipgloss.thickBorder()
//	lipgloss.doubleBorder()
//	lipgloss.hiddenBorder()
//	lipgloss.noBorder()
//
//	// Alignment constants
//	lipgloss.Left      // 0.0
//	lipgloss.Center    // 0.5
//	lipgloss.Right     // 1.0
//	lipgloss.Top       // 0.0
//	lipgloss.Bottom    // 1.0
//
//	// Utility functions
//	lipgloss.joinHorizontal(pos, ...strs);  // Join strings horizontally
//	lipgloss.joinVertical(pos, ...strs);    // Join strings vertically
//	lipgloss.place(width, height, hPos, vPos, str);  // Place string in a box
//	lipgloss.size(str);  // Returns {width, height}
//	lipgloss.width(str); // Returns width
//	lipgloss.height(str); // Returns height
//
// # Pathway to Full Support
//
// This implementation exposes the core lipgloss API. The following describes
// the pathway to achieving full parity with the native Go lipgloss library.
//
// ## Currently Implemented
//
//   - Style creation with newStyle()
//   - Text formatting: bold, italic, underline, strikethrough, blink, faint, reverse
//   - Colors: foreground, background (supports hex, ANSI)
//   - Spacing: padding, margin (all sides and individual)
//   - Dimensions: width, height, maxWidth, maxHeight
//   - Borders: all border types, colors, and individual side control
//   - Alignment: align, alignHorizontal, alignVertical
//   - Layout: joinHorizontal, joinVertical, place
//   - Measurement: size, width, height
//   - Border presets: normalBorder, roundedBorder, thickBorder, doubleBorder, hiddenBorder
//
// ## Phase 1: Color Enhancements (recommended next steps)
//
//   - lipgloss.color(value) - Explicit color constructor
//   - lipgloss.adaptiveColor({light, dark}) - Color based on terminal background
//   - lipgloss.completeColor({trueColor, ansi256, ansi}) - Fallback color support
//   - lipgloss.completeAdaptiveColor({light, dark}) - Combined adaptive/complete
//   - Color profile detection (trueColor, ansi256, ansi, noColor)
//
// ## Phase 2: Advanced Styling
//
//   - Tab width control
//   - Inline styles (no newlines)
//   - Transform functions (uppercase, lowercase, etc.)
//   - WhiteSpace handling (normal, nowrap, pre, preWrap)
//   - UnderlineSpaces, StrikethroughSpaces options
//   - Custom border construction
//
// ## Phase 3: Layout Utilities
//
//   - lipgloss.placeHorizontal(width, pos, str)
//   - lipgloss.placeVertical(height, pos, str)
//   - lipgloss.placeOverlay(x, y, fg, bg) - Overlay strings
//   - lipgloss.table() - Table rendering support
//   - lipgloss.list() - List rendering support
//
// ## Phase 4: Style Management
//
//   - Style inheritance and composition
//   - Style diffing and merging
//   - Named style presets
//   - Theme support with color palettes
//   - Runtime style updates and observers
//
// ## Implementation Notes
//
// All additions should follow these patterns:
//
//  1. No global state - Manager instance per engine
//  2. Chainable API - methods return the style for chaining
//  3. Immutable semantics - methods return copies, not mutations
//  4. JavaScript-friendly types - use maps and arrays
//  5. Comprehensive unit tests for all exposed functions
package lipgloss

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/dop251/goja"
)

// Manager holds lipgloss-related state per engine instance.
// Currently stateless but provided for consistency and future extensibility.
type Manager struct{}

// NewManager creates a new lipgloss manager for an engine instance.
func NewManager() *Manager {
	return &Manager{}
}

// StyleWrapper wraps a lipgloss.Style for JavaScript interaction.
type StyleWrapper struct {
	style   lipgloss.Style
	runtime *goja.Runtime
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

	// copy: Create a copy of this style
	_ = obj.Set("copy", func(call goja.FunctionCall) goja.Value {
		newWrapper := &StyleWrapper{
			style:   wrapper.style.Copy(),
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

	// Colors
	_ = obj.Set("foreground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		colorStr := call.Argument(0).String()
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
		newWrapper := &StyleWrapper{
			style:   wrapper.style.Background(lipgloss.Color(colorStr)),
			runtime: runtime,
		}
		return createStyleObject(runtime, newWrapper)
	})

	// Padding
	_ = obj.Set("padding", func(call goja.FunctionCall) goja.Value {
		args := extractInts(call.Arguments)
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
			newStyle = wrapper.style.Copy()
		}
		newWrapper := &StyleWrapper{style: newStyle, runtime: runtime}
		return createStyleObject(runtime, newWrapper)
	})

	_ = obj.Set("paddingTop", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			newWrapper := &StyleWrapper{
				style:   wrapper.style.PaddingTop(int(call.Argument(0).ToInteger())),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	_ = obj.Set("paddingRight", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			newWrapper := &StyleWrapper{
				style:   wrapper.style.PaddingRight(int(call.Argument(0).ToInteger())),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	_ = obj.Set("paddingBottom", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			newWrapper := &StyleWrapper{
				style:   wrapper.style.PaddingBottom(int(call.Argument(0).ToInteger())),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	_ = obj.Set("paddingLeft", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			newWrapper := &StyleWrapper{
				style:   wrapper.style.PaddingLeft(int(call.Argument(0).ToInteger())),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	// Margin
	_ = obj.Set("margin", func(call goja.FunctionCall) goja.Value {
		args := extractInts(call.Arguments)
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
			newStyle = wrapper.style.Copy()
		}
		newWrapper := &StyleWrapper{style: newStyle, runtime: runtime}
		return createStyleObject(runtime, newWrapper)
	})

	_ = obj.Set("marginTop", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			newWrapper := &StyleWrapper{
				style:   wrapper.style.MarginTop(int(call.Argument(0).ToInteger())),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	_ = obj.Set("marginRight", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			newWrapper := &StyleWrapper{
				style:   wrapper.style.MarginRight(int(call.Argument(0).ToInteger())),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	_ = obj.Set("marginBottom", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			newWrapper := &StyleWrapper{
				style:   wrapper.style.MarginBottom(int(call.Argument(0).ToInteger())),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	_ = obj.Set("marginLeft", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			newWrapper := &StyleWrapper{
				style:   wrapper.style.MarginLeft(int(call.Argument(0).ToInteger())),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	// Dimensions
	_ = obj.Set("width", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			newWrapper := &StyleWrapper{
				style:   wrapper.style.Width(int(call.Argument(0).ToInteger())),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	_ = obj.Set("height", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			newWrapper := &StyleWrapper{
				style:   wrapper.style.Height(int(call.Argument(0).ToInteger())),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	_ = obj.Set("maxWidth", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			newWrapper := &StyleWrapper{
				style:   wrapper.style.MaxWidth(int(call.Argument(0).ToInteger())),
				runtime: runtime,
			}
			return createStyleObject(runtime, newWrapper)
		}
		return obj
	})

	_ = obj.Set("maxHeight", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			newWrapper := &StyleWrapper{
				style:   wrapper.style.MaxHeight(int(call.Argument(0).ToInteger())),
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
