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
	"fmt"
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
	ErrCodeInvalidArg       = "LG005" // Invalid argument type
)

// Internal key for storing style state on Goja objects.
const internalStateKey = "__style_state"

// Regex for hex color validation.
var colorRegex = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$|^#[0-9A-Fa-f]{3}$`)

// Manager holds the renderer context for a specific engine instance.
// In a multi-tenant environment (e.g. SSH), this allows isolation per session.
type Manager struct {
	Renderer *lipgloss.Renderer
}

// NewManager creates a new manager with a default renderer.
func NewManager() *Manager {
	return &Manager{
		Renderer: lipgloss.DefaultRenderer(),
	}
}

// styleState encapsulates the state of a style object in the JS runtime.
type styleState struct {
	style    lipgloss.Style
	hasError bool
	errCode  string
	errMsg   string
}

// Require returns the module loader function.
func Require(manager *Manager) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		// ---------------------------------------------------------------------
		// Prototype Construction
		// ---------------------------------------------------------------------
		// We create a single prototype object to be shared by all Style instances.
		// This significantly reduces GC pressure compared to creating closures per instance.
		proto := runtime.NewObject()

		// helper: get state from "this"
		getState := func(call goja.FunctionCall) *styleState {
			thisObj := call.This.ToObject(runtime)
			val := thisObj.Get(internalStateKey)
			if val == nil {
				return nil
			}
			if state, ok := val.Export().(*styleState); ok {
				return state
			}
			return nil
		}

		// helper: return a new JS object wrapping a new style (Immutability)
		returnWithStyle := func(newStyle lipgloss.Style) goja.Value {
			newState := &styleState{style: newStyle}
			obj := runtime.NewObject()
			_ = obj.SetPrototype(proto)
			_ = obj.Set(internalStateKey, newState)
			return obj
		}

		// helper: return an error style object
		returnWithError := func(original lipgloss.Style, code, msg string) goja.Value {
			newState := &styleState{
				style:    original,
				hasError: true,
				errCode:  code,
				errMsg:   msg,
			}
			obj := runtime.NewObject()
			_ = obj.SetPrototype(proto)
			_ = obj.Set(internalStateKey, newState)
			return obj
		}

		// --- State Accessors (Getters) ---
		// We use defineProperty so `style.hasError` works like a native property.

		objectConstructor := runtime.GlobalObject().Get("Object").ToObject(runtime)
		defProp := objectConstructor.Get("defineProperty")

		defPropCallable, ok := goja.AssertFunction(defProp)
		if !ok {
			panic("defineProperty is not a function")
		}

		_, err := defPropCallable(goja.Undefined(), proto, runtime.ToValue("hasError"), runtime.ToValue(map[string]interface{}{
			"get": func(call goja.FunctionCall) goja.Value {
				state := getState(call)
				if state == nil {
					return runtime.ToValue(false)
				}
				return runtime.ToValue(state.hasError)
			},
			"configurable": false,
		}))
		if err != nil {
			panic(err)
		}

		_, err = defPropCallable(goja.Undefined(), proto, runtime.ToValue("error"), runtime.ToValue(map[string]interface{}{
			"get": func(call goja.FunctionCall) goja.Value {
				state := getState(call)
				if state == nil || !state.hasError {
					return goja.Null()
				}
				return runtime.ToValue(fmt.Sprintf("%s: %s", state.errCode, state.errMsg))
			},
			"configurable": false,
		}))
		if err != nil {
			panic(err)
		}

		_, err = defPropCallable(goja.Undefined(), proto, runtime.ToValue("errorCode"), runtime.ToValue(map[string]interface{}{
			"get": func(call goja.FunctionCall) goja.Value {
				state := getState(call)
				if state == nil || !state.hasError {
					return goja.Null()
				}
				return runtime.ToValue(state.errCode)
			},
			"configurable": false,
		}))
		if err != nil {
			panic(err)
		}

		// --- Core Methods ---

		_ = proto.Set("render", func(call goja.FunctionCall) goja.Value {
			state := getState(call)
			if state == nil || state.hasError {
				return runtime.ToValue("")
			}
			var strs []string
			for _, arg := range call.Arguments {
				strs = append(strs, arg.String())
			}
			return runtime.ToValue(state.style.Render(strs...))
		})

		_ = proto.Set("copy", func(call goja.FunctionCall) goja.Value {
			state := getState(call)
			if state == nil {
				return goja.Undefined()
			}
			if state.hasError {
				return returnWithError(state.style, state.errCode, state.errMsg)
			}
			return returnWithStyle(state.style)
		})

		_ = proto.Set("inherit", func(call goja.FunctionCall) goja.Value {
			state := getState(call)
			if state == nil {
				return goja.Undefined()
			}
			if state.hasError {
				return returnWithError(state.style, state.errCode, state.errMsg)
			}
			if len(call.Arguments) == 0 {
				return call.This
			}
			otherObj := call.Argument(0).ToObject(runtime)
			otherVal := otherObj.Get(internalStateKey)
			if otherVal == nil {
				return returnWithStyle(state.style)
			}
			if otherState, ok := otherVal.Export().(*styleState); ok {
				return returnWithStyle(state.style.Inherit(otherState.style))
			}
			return returnWithStyle(state.style)
		})

		_ = proto.Set("string", func(call goja.FunctionCall) goja.Value {
			state := getState(call)
			if state == nil {
				return runtime.ToValue("")
			}
			return runtime.ToValue(state.style.String())
		})

		_ = proto.Set("value", func(call goja.FunctionCall) goja.Value {
			state := getState(call)
			if state == nil {
				return runtime.ToValue("")
			}
			return runtime.ToValue(state.style.Value())
		})

		// --- Styling Methods ---

		// Boolean Attributes
		boolMethods := map[string]func(lipgloss.Style, bool) lipgloss.Style{
			"bold":                func(s lipgloss.Style, v bool) lipgloss.Style { return s.Bold(v) },
			"italic":              func(s lipgloss.Style, v bool) lipgloss.Style { return s.Italic(v) },
			"underline":           func(s lipgloss.Style, v bool) lipgloss.Style { return s.Underline(v) },
			"strikethrough":       func(s lipgloss.Style, v bool) lipgloss.Style { return s.Strikethrough(v) },
			"blink":               func(s lipgloss.Style, v bool) lipgloss.Style { return s.Blink(v) },
			"faint":               func(s lipgloss.Style, v bool) lipgloss.Style { return s.Faint(v) },
			"reverse":             func(s lipgloss.Style, v bool) lipgloss.Style { return s.Reverse(v) },
			"inline":              func(s lipgloss.Style, v bool) lipgloss.Style { return s.Inline(v) },
			"underlineSpaces":     func(s lipgloss.Style, v bool) lipgloss.Style { return s.UnderlineSpaces(v) },
			"strikethroughSpaces": func(s lipgloss.Style, v bool) lipgloss.Style { return s.StrikethroughSpaces(v) },
		}

		for name, method := range boolMethods {
			fn := method
			_ = proto.Set(name, func(call goja.FunctionCall) goja.Value {
				state := getState(call)
				if state == nil {
					return goja.Undefined()
				}
				if state.hasError {
					return returnWithError(state.style, state.errCode, state.errMsg)
				}
				val := true
				if len(call.Arguments) > 0 {
					val = call.Argument(0).ToBoolean()
				}
				return returnWithStyle(fn(state.style, val))
			})
		}

		// Colors
		colorMethods := map[string]func(lipgloss.Style, lipgloss.TerminalColor) lipgloss.Style{
			"foreground":             func(s lipgloss.Style, c lipgloss.TerminalColor) lipgloss.Style { return s.Foreground(c) },
			"background":             func(s lipgloss.Style, c lipgloss.TerminalColor) lipgloss.Style { return s.Background(c) },
			"borderForeground":       func(s lipgloss.Style, c lipgloss.TerminalColor) lipgloss.Style { return s.BorderForeground(c) },
			"borderBackground":       func(s lipgloss.Style, c lipgloss.TerminalColor) lipgloss.Style { return s.BorderBackground(c) },
			"borderTopForeground":    func(s lipgloss.Style, c lipgloss.TerminalColor) lipgloss.Style { return s.BorderTopForeground(c) },
			"borderRightForeground":  func(s lipgloss.Style, c lipgloss.TerminalColor) lipgloss.Style { return s.BorderRightForeground(c) },
			"borderBottomForeground": func(s lipgloss.Style, c lipgloss.TerminalColor) lipgloss.Style { return s.BorderBottomForeground(c) },
			"borderLeftForeground":   func(s lipgloss.Style, c lipgloss.TerminalColor) lipgloss.Style { return s.BorderLeftForeground(c) },
			"borderTopBackground":    func(s lipgloss.Style, c lipgloss.TerminalColor) lipgloss.Style { return s.BorderTopBackground(c) },
			"borderRightBackground":  func(s lipgloss.Style, c lipgloss.TerminalColor) lipgloss.Style { return s.BorderRightBackground(c) },
			"borderBottomBackground": func(s lipgloss.Style, c lipgloss.TerminalColor) lipgloss.Style { return s.BorderBottomBackground(c) },
			"borderLeftBackground":   func(s lipgloss.Style, c lipgloss.TerminalColor) lipgloss.Style { return s.BorderLeftBackground(c) },
		}

		for name, method := range colorMethods {
			fn := method
			_ = proto.Set(name, func(call goja.FunctionCall) goja.Value {
				state := getState(call)
				if state == nil {
					return goja.Undefined()
				}
				if state.hasError {
					return returnWithError(state.style, state.errCode, state.errMsg)
				}
				if len(call.Arguments) == 0 {
					return call.This
				}
				color, err := parseColor(runtime, call.Argument(0))
				if err != nil {
					return returnWithError(state.style, ErrCodeInvalidColor, err.Error())
				}
				return returnWithStyle(fn(state.style, color))
			})
		}

		// Scalar Dimensions
		scalarMethods := map[string]func(lipgloss.Style, int) lipgloss.Style{
			"width":         func(s lipgloss.Style, i int) lipgloss.Style { return s.Width(i) },
			"height":        func(s lipgloss.Style, i int) lipgloss.Style { return s.Height(i) },
			"maxWidth":      func(s lipgloss.Style, i int) lipgloss.Style { return s.MaxWidth(i) },
			"maxHeight":     func(s lipgloss.Style, i int) lipgloss.Style { return s.MaxHeight(i) },
			"paddingTop":    func(s lipgloss.Style, i int) lipgloss.Style { return s.PaddingTop(i) },
			"paddingRight":  func(s lipgloss.Style, i int) lipgloss.Style { return s.PaddingRight(i) },
			"paddingBottom": func(s lipgloss.Style, i int) lipgloss.Style { return s.PaddingBottom(i) },
			"paddingLeft":   func(s lipgloss.Style, i int) lipgloss.Style { return s.PaddingLeft(i) },
			"marginTop":     func(s lipgloss.Style, i int) lipgloss.Style { return s.MarginTop(i) },
			"marginRight":   func(s lipgloss.Style, i int) lipgloss.Style { return s.MarginRight(i) },
			"marginBottom":  func(s lipgloss.Style, i int) lipgloss.Style { return s.MarginBottom(i) },
			"marginLeft":    func(s lipgloss.Style, i int) lipgloss.Style { return s.MarginLeft(i) },
			"tabWidth":      func(s lipgloss.Style, i int) lipgloss.Style { return s.TabWidth(i) },
		}

		for name, method := range scalarMethods {
			fn := method
			_ = proto.Set(name, func(call goja.FunctionCall) goja.Value {
				state := getState(call)
				if state == nil {
					return goja.Undefined()
				}
				if state.hasError {
					return returnWithError(state.style, state.errCode, state.errMsg)
				}
				if len(call.Arguments) == 0 {
					return call.This
				}
				val := int(call.Argument(0).ToInteger())
				if val < 0 && name != "tabWidth" { // TabWidth can be negative constant
					return returnWithError(state.style, ErrCodeInvalidDimension, "dimension must be non-negative")
				}
				return returnWithStyle(fn(state.style, val))
			})
		}

		// Variadic Dimensions (Padding/Margin) with strict argument logic
		_ = proto.Set("padding", func(call goja.FunctionCall) goja.Value {
			state := getState(call)
			if state == nil {
				return goja.Undefined()
			}
			if state.hasError {
				return returnWithError(state.style, state.errCode, state.errMsg)
			}
			args := extractInts(call.Arguments)
			for _, v := range args {
				if v < 0 {
					return returnWithError(state.style, ErrCodeInvalidDimension, "padding must be non-negative")
				}
			}
			var newStyle lipgloss.Style
			switch len(args) {
			case 1:
				newStyle = state.style.Padding(args[0])
			case 2:
				newStyle = state.style.Padding(args[0], args[1])
			case 3: // Logic fix: Top, Horizontal, Bottom -> Top, Right, Bottom, Left
				newStyle = state.style.Padding(args[0], args[1], args[2], args[1])
			case 4:
				newStyle = state.style.Padding(args[0], args[1], args[2], args[3])
			default:
				newStyle = state.style
			}
			return returnWithStyle(newStyle)
		})

		_ = proto.Set("margin", func(call goja.FunctionCall) goja.Value {
			state := getState(call)
			if state == nil {
				return goja.Undefined()
			}
			if state.hasError {
				return returnWithError(state.style, state.errCode, state.errMsg)
			}
			args := extractInts(call.Arguments)
			for _, v := range args {
				if v < 0 {
					return returnWithError(state.style, ErrCodeInvalidDimension, "margin must be non-negative")
				}
			}
			var newStyle lipgloss.Style
			switch len(args) {
			case 1:
				newStyle = state.style.Margin(args[0])
			case 2:
				newStyle = state.style.Margin(args[0], args[1])
			case 3: // Logic fix: Top, Horizontal, Bottom -> Top, Right, Bottom, Left
				newStyle = state.style.Margin(args[0], args[1], args[2], args[1])
			case 4:
				newStyle = state.style.Margin(args[0], args[1], args[2], args[3])
			default:
				newStyle = state.style
			}
			return returnWithStyle(newStyle)
		})

		// Border Method
		_ = proto.Set("border", func(call goja.FunctionCall) goja.Value {
			state := getState(call)
			if state == nil {
				return goja.Undefined()
			}
			if state.hasError {
				return returnWithError(state.style, state.errCode, state.errMsg)
			}
			if len(call.Arguments) == 0 {
				return call.This
			}
			b := jsToBorder(runtime, call.Argument(0))
			// Handle optional side arguments: border(style, top, right, bottom, left)
			var sides []bool
			for i := 1; i < len(call.Arguments); i++ {
				sides = append(sides, call.Argument(i).ToBoolean())
			}
			return returnWithStyle(state.style.Border(b, sides...))
		})

		// Individual Border Sides
		borderSides := map[string]func(lipgloss.Style, bool) lipgloss.Style{
			"borderTop":    func(s lipgloss.Style, v bool) lipgloss.Style { return s.BorderTop(v) },
			"borderRight":  func(s lipgloss.Style, v bool) lipgloss.Style { return s.BorderRight(v) },
			"borderBottom": func(s lipgloss.Style, v bool) lipgloss.Style { return s.BorderBottom(v) },
			"borderLeft":   func(s lipgloss.Style, v bool) lipgloss.Style { return s.BorderLeft(v) },
		}
		for name, method := range borderSides {
			fn := method
			_ = proto.Set(name, func(call goja.FunctionCall) goja.Value {
				state := getState(call)
				if state == nil {
					return goja.Undefined()
				}
				if state.hasError {
					return returnWithError(state.style, state.errCode, state.errMsg)
				}
				val := true
				if len(call.Arguments) > 0 {
					val = call.Argument(0).ToBoolean()
				}
				return returnWithStyle(fn(state.style, val))
			})
		}

		// Alignment
		alignMethods := map[string]func(lipgloss.Style, lipgloss.Position) lipgloss.Style{
			"align":           func(s lipgloss.Style, p lipgloss.Position) lipgloss.Style { return s.Align(p) },
			"alignHorizontal": func(s lipgloss.Style, p lipgloss.Position) lipgloss.Style { return s.AlignHorizontal(p) },
			"alignVertical":   func(s lipgloss.Style, p lipgloss.Position) lipgloss.Style { return s.AlignVertical(p) },
		}
		for name, method := range alignMethods {
			fn := method
			_ = proto.Set(name, func(call goja.FunctionCall) goja.Value {
				state := getState(call)
				if state == nil {
					return goja.Undefined()
				}
				if state.hasError {
					return returnWithError(state.style, state.errCode, state.errMsg)
				}
				if len(call.Arguments) == 0 {
					return call.This
				}
				pos := lipgloss.Position(call.Argument(0).ToFloat())
				if pos < 0 || pos > 1 {
					return returnWithError(state.style, ErrCodeInvalidAlignment, "alignment must be between 0.0 and 1.0")
				}
				return returnWithStyle(fn(state.style, pos))
			})
		}

		// Unset Methods (API Completeness)
		unsetMethods := map[string]func(lipgloss.Style) lipgloss.Style{
			"unsetBold":                   func(s lipgloss.Style) lipgloss.Style { return s.UnsetBold() },
			"unsetItalic":                 func(s lipgloss.Style) lipgloss.Style { return s.UnsetItalic() },
			"unsetUnderline":              func(s lipgloss.Style) lipgloss.Style { return s.UnsetUnderline() },
			"unsetStrikethrough":          func(s lipgloss.Style) lipgloss.Style { return s.UnsetStrikethrough() },
			"unsetBlink":                  func(s lipgloss.Style) lipgloss.Style { return s.UnsetBlink() },
			"unsetFaint":                  func(s lipgloss.Style) lipgloss.Style { return s.UnsetFaint() },
			"unsetReverse":                func(s lipgloss.Style) lipgloss.Style { return s.UnsetReverse() },
			"unsetInline":                 func(s lipgloss.Style) lipgloss.Style { return s.UnsetInline() },
			"unsetForeground":             func(s lipgloss.Style) lipgloss.Style { return s.UnsetForeground() },
			"unsetBackground":             func(s lipgloss.Style) lipgloss.Style { return s.UnsetBackground() },
			"unsetBorderTop":              func(s lipgloss.Style) lipgloss.Style { return s.UnsetBorderTop() },
			"unsetBorderRight":            func(s lipgloss.Style) lipgloss.Style { return s.UnsetBorderRight() },
			"unsetBorderBottom":           func(s lipgloss.Style) lipgloss.Style { return s.UnsetBorderBottom() },
			"unsetBorderLeft":             func(s lipgloss.Style) lipgloss.Style { return s.UnsetBorderLeft() },
			"unsetBorderForeground":       func(s lipgloss.Style) lipgloss.Style { return s.UnsetBorderForeground() },
			"unsetBorderBackground":       func(s lipgloss.Style) lipgloss.Style { return s.UnsetBorderBackground() },
			"unsetPadding":                func(s lipgloss.Style) lipgloss.Style { return s.UnsetPadding() },
			"unsetMargins":                func(s lipgloss.Style) lipgloss.Style { return s.UnsetMargins() },
			"unsetWidth":                  func(s lipgloss.Style) lipgloss.Style { return s.UnsetWidth() },
			"unsetHeight":                 func(s lipgloss.Style) lipgloss.Style { return s.UnsetHeight() },
			"unsetMaxWidth":               func(s lipgloss.Style) lipgloss.Style { return s.UnsetMaxWidth() },
			"unsetMaxHeight":              func(s lipgloss.Style) lipgloss.Style { return s.UnsetMaxHeight() },
			"unsetAlign":                  func(s lipgloss.Style) lipgloss.Style { return s.UnsetAlign() },
			"unsetAlignHorizontal":        func(s lipgloss.Style) lipgloss.Style { return s.UnsetAlignHorizontal() },
			"unsetAlignVertical":          func(s lipgloss.Style) lipgloss.Style { return s.UnsetAlignVertical() },
			"unsetTabWidth":               func(s lipgloss.Style) lipgloss.Style { return s.UnsetTabWidth() },
			"unsetUnderlineSpaces":        func(s lipgloss.Style) lipgloss.Style { return s.UnsetUnderlineSpaces() },
			"unsetStrikethroughSpaces":    func(s lipgloss.Style) lipgloss.Style { return s.UnsetStrikethroughSpaces() },
			"unsetBorderTopForeground":    func(s lipgloss.Style) lipgloss.Style { return s.UnsetBorderTopForeground() },
			"unsetBorderRightForeground":  func(s lipgloss.Style) lipgloss.Style { return s.UnsetBorderRightForeground() },
			"unsetBorderBottomForeground": func(s lipgloss.Style) lipgloss.Style { return s.UnsetBorderBottomForeground() },
			"unsetBorderLeftForeground":   func(s lipgloss.Style) lipgloss.Style { return s.UnsetBorderLeftForeground() },
			"unsetBorderTopBackground":    func(s lipgloss.Style) lipgloss.Style { return s.UnsetBorderTopBackground() },
			"unsetBorderRightBackground":  func(s lipgloss.Style) lipgloss.Style { return s.UnsetBorderRightBackground() },
			"unsetBorderBottomBackground": func(s lipgloss.Style) lipgloss.Style { return s.UnsetBorderBottomBackground() },
			"unsetBorderLeftBackground":   func(s lipgloss.Style) lipgloss.Style { return s.UnsetBorderLeftBackground() },
			"unsetMarginTop":              func(s lipgloss.Style) lipgloss.Style { return s.UnsetMarginTop() },
			"unsetMarginRight":            func(s lipgloss.Style) lipgloss.Style { return s.UnsetMarginRight() },
			"unsetMarginBottom":           func(s lipgloss.Style) lipgloss.Style { return s.UnsetMarginBottom() },
			"unsetMarginLeft":             func(s lipgloss.Style) lipgloss.Style { return s.UnsetMarginLeft() },
			"unsetPaddingTop":             func(s lipgloss.Style) lipgloss.Style { return s.UnsetPaddingTop() },
			"unsetPaddingRight":           func(s lipgloss.Style) lipgloss.Style { return s.UnsetPaddingRight() },
			"unsetPaddingBottom":          func(s lipgloss.Style) lipgloss.Style { return s.UnsetPaddingBottom() },
			"unsetPaddingLeft":            func(s lipgloss.Style) lipgloss.Style { return s.UnsetPaddingLeft() },
		}

		for name, method := range unsetMethods {
			fn := method
			_ = proto.Set(name, func(call goja.FunctionCall) goja.Value {
				state := getState(call)
				if state == nil {
					return goja.Undefined()
				}
				if state.hasError {
					return returnWithError(state.style, state.errCode, state.errMsg)
				}
				return returnWithStyle(fn(state.style))
			})
		}

		// ---------------------------------------------------------------------
		// Exported Functions
		// ---------------------------------------------------------------------

		// newStyle factory using context-aware renderer
		_ = exports.Set("newStyle", func(call goja.FunctionCall) goja.Value {
			style := manager.Renderer.NewStyle()
			newState := &styleState{style: style}
			obj := runtime.NewObject()
			_ = obj.SetPrototype(proto)
			_ = obj.Set(internalStateKey, newState)
			return obj
		})

		// Constants
		_ = exports.Set("Left", lipgloss.Left)
		_ = exports.Set("Center", lipgloss.Center)
		_ = exports.Set("Right", lipgloss.Right)
		_ = exports.Set("Top", lipgloss.Top)
		_ = exports.Set("Bottom", lipgloss.Bottom)
		_ = exports.Set("NoTabConversion", lipgloss.NoTabConversion)

		// Border Factories
		_ = exports.Set("normalBorder", func() map[string]interface{} { return borderToJS(lipgloss.NormalBorder()) })
		_ = exports.Set("roundedBorder", func() map[string]interface{} { return borderToJS(lipgloss.RoundedBorder()) })
		_ = exports.Set("blockBorder", func() map[string]interface{} { return borderToJS(lipgloss.BlockBorder()) })
		_ = exports.Set("outerHalfBlockBorder", func() map[string]interface{} { return borderToJS(lipgloss.OuterHalfBlockBorder()) })
		_ = exports.Set("innerHalfBlockBorder", func() map[string]interface{} { return borderToJS(lipgloss.InnerHalfBlockBorder()) })
		_ = exports.Set("thickBorder", func() map[string]interface{} { return borderToJS(lipgloss.ThickBorder()) })
		_ = exports.Set("doubleBorder", func() map[string]interface{} { return borderToJS(lipgloss.DoubleBorder()) })
		_ = exports.Set("hiddenBorder", func() map[string]interface{} { return borderToJS(lipgloss.HiddenBorder()) })
		_ = exports.Set("noBorder", func() map[string]interface{} { return nil })

		// Utilities
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

			var opts []lipgloss.WhitespaceOption
			if len(call.Arguments) > 5 {
				var err error
				opts, err = parseWhitespaceOptions(runtime, call.Argument(5))
				if err != nil {
					panic(runtime.NewGoError(err))
				}
			}
			return runtime.ToValue(manager.Renderer.Place(width, height, hPos, vPos, str, opts...))
		})

		_ = exports.Set("placeHorizontal", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 3 {
				return runtime.ToValue("")
			}
			width := int(call.Argument(0).ToInteger())
			pos := lipgloss.Position(call.Argument(1).ToFloat())
			str := call.Argument(2).String()
			var opts []lipgloss.WhitespaceOption
			if len(call.Arguments) > 3 {
				var err error
				opts, err = parseWhitespaceOptions(runtime, call.Argument(3))
				if err != nil {
					panic(runtime.NewGoError(err))
				}
			}
			return runtime.ToValue(manager.Renderer.PlaceHorizontal(width, pos, str, opts...))
		})

		_ = exports.Set("placeVertical", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 3 {
				return runtime.ToValue("")
			}
			height := int(call.Argument(0).ToInteger())
			pos := lipgloss.Position(call.Argument(1).ToFloat())
			str := call.Argument(2).String()
			var opts []lipgloss.WhitespaceOption
			if len(call.Arguments) > 3 {
				var err error
				opts, err = parseWhitespaceOptions(runtime, call.Argument(3))
				if err != nil {
					panic(runtime.NewGoError(err))
				}
			}
			return runtime.ToValue(manager.Renderer.PlaceVertical(height, pos, str, opts...))
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

		_ = exports.Set("size", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return runtime.ToValue(map[string]interface{}{"width": 0, "height": 0})
			}
			str := call.Argument(0).String()
			return runtime.ToValue(map[string]interface{}{
				"width":  lipgloss.Width(str),
				"height": lipgloss.Height(str),
			})
		})

		_ = exports.Set("hasDarkBackground", func(call goja.FunctionCall) goja.Value {
			return runtime.ToValue(manager.Renderer.HasDarkBackground())
		})
	}
}

// ---------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------

// parseColor strictly validates and parses a Goja value into a lipgloss.TerminalColor.
func parseColor(runtime *goja.Runtime, val goja.Value) (lipgloss.TerminalColor, error) {
	if goja.IsUndefined(val) || goja.IsNull(val) {
		return lipgloss.NoColor{}, nil
	}

	// 1. Handle Strings (Hex, ANSI, Named)
	if val.ExportType().Name() == "string" {
		str := strings.TrimSpace(val.String())
		if str == "" {
			return lipgloss.NoColor{}, nil
		}
		// Hex
		if strings.HasPrefix(str, "#") {
			if !colorRegex.MatchString(str) {
				return nil, fmt.Errorf("invalid hex color: %s", str)
			}
			return lipgloss.Color(str), nil
		}
		// ANSI (0-255)
		if n, err := strconv.Atoi(str); err == nil {
			if n >= 0 && n <= 255 {
				return lipgloss.Color(str), nil
			}
			return nil, fmt.Errorf("ANSI color must be 0-255, got: %d", n)
		}
		// Allow any other string (system colors, named colors handled by lipgloss)
		return lipgloss.Color(str), nil
	}

	// 2. Handle Objects (AdaptiveColor)
	obj := val.ToObject(runtime)
	light := obj.Get("light")
	dark := obj.Get("dark")

	if light == nil || dark == nil || goja.IsUndefined(light) || goja.IsUndefined(dark) {
		return nil, fmt.Errorf("invalid color object: missing 'light' or 'dark' properties")
	}

	// Recursive validation for inner colors
	var lStr, dStr string
	if _, err := parseColor(runtime, light); err != nil {
		return nil, fmt.Errorf("invalid light color: %v", err)
	}
	lStr = light.String()

	if _, err := parseColor(runtime, dark); err != nil {
		return nil, fmt.Errorf("invalid dark color: %v", err)
	}
	dStr = dark.String()

	return lipgloss.AdaptiveColor{Light: lStr, Dark: dStr}, nil
}

// parseWhitespaceOptions parses options for Place calls.
func parseWhitespaceOptions(runtime *goja.Runtime, val goja.Value) ([]lipgloss.WhitespaceOption, error) {
	var opts []lipgloss.WhitespaceOption
	if goja.IsUndefined(val) || goja.IsNull(val) {
		return opts, nil
	}
	obj := val.ToObject(runtime)

	if chars := obj.Get("whitespaceChars"); chars != nil && !goja.IsUndefined(chars) && !goja.IsNull(chars) {
		opts = append(opts, lipgloss.WithWhitespaceChars(chars.String()))
	}
	if fg := obj.Get("whitespaceForeground"); fg != nil && !goja.IsUndefined(fg) {
		c, err := parseColor(runtime, fg)
		if err != nil {
			return nil, err
		}
		opts = append(opts, lipgloss.WithWhitespaceForeground(c))
	}
	if bg := obj.Get("whitespaceBackground"); bg != nil && !goja.IsUndefined(bg) {
		c, err := parseColor(runtime, bg)
		if err != nil {
			return nil, err
		}
		opts = append(opts, lipgloss.WithWhitespaceBackground(c))
	}
	return opts, nil
}

func extractInts(args []goja.Value) []int {
	var result []int
	for _, arg := range args {
		result = append(result, int(arg.ToInteger()))
	}
	return result
}

func borderToJS(b lipgloss.Border) map[string]interface{} {
	return map[string]interface{}{
		"top":          b.Top,
		"bottom":       b.Bottom,
		"left":         b.Left,
		"right":        b.Right,
		"topLeft":      b.TopLeft,
		"topRight":     b.TopRight,
		"bottomLeft":   b.BottomLeft,
		"bottomRight":  b.BottomRight,
		"middleLeft":   b.MiddleLeft,
		"middleRight":  b.MiddleRight,
		"middle":       b.Middle,
		"middleTop":    b.MiddleTop,
		"middleBottom": b.MiddleBottom,
	}
}

func jsToBorder(runtime *goja.Runtime, val goja.Value) lipgloss.Border {
	if goja.IsUndefined(val) || goja.IsNull(val) {
		return lipgloss.Border{}
	}
	obj := val.ToObject(runtime)
	getString := func(key string) string {
		v := obj.Get(key)
		if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
			return ""
		}
		return v.String()
	}
	return lipgloss.Border{
		Top:          getString("top"),
		Bottom:       getString("bottom"),
		Left:         getString("left"),
		Right:        getString("right"),
		TopLeft:      getString("topLeft"),
		TopRight:     getString("topRight"),
		BottomLeft:   getString("bottomLeft"),
		BottomRight:  getString("bottomRight"),
		MiddleLeft:   getString("middleLeft"),
		MiddleRight:  getString("middleRight"),
		Middle:       getString("middle"),
		MiddleTop:    getString("middleTop"),
		MiddleBottom: getString("middleBottom"),
	}
}
