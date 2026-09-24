package lipgloss

import (
	"errors"
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/joeycumines/goja"
)

// styleState encapsulates the state of a style object in the JS runtime.
type styleState struct {
	style    lipgloss.Style
	hasError bool
	errCode  string
	errMsg   string
}

// UnwrapStyle attempts to retrieve the underlying lipgloss.Style from a JavaScript object.
// It returns an error if the object is not a valid style instance or contains an error state.
func UnwrapStyle(rt *goja.Runtime, v goja.Value) (lipgloss.Style, error) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return lipgloss.Style{}, errors.New("value is null or undefined")
	}

	obj := v.ToObject(rt)
	val := obj.Get(internalStateKey)
	if val == nil {
		return lipgloss.Style{}, errors.New("object is not a lipgloss style")
	}

	state, ok := val.Export().(*styleState)
	if !ok {
		return lipgloss.Style{}, errors.New("object has invalid internal state")
	}

	if state.hasError {
		return lipgloss.Style{}, fmt.Errorf("cannot use style with error: %s (%s)", state.errMsg, state.errCode)
	}

	return state.style, nil
}

// parseColor strictly validates and parses a Goja value into a color.Color.
func parseColor(runtime *goja.Runtime, val goja.Value, hasDarkBg bool) (color.Color, error) {
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

	// 2. Handle Objects (AdaptiveColor-like: {light, dark})
	// In v2, we return a LightDarkFunc which resolves at render time.
	// Since Style.Foreground/Background accept color.Color, we need to
	// store the light/dark strings and resolve them when the style is used.
	// For simplicity, we return a custom color that wraps the LightDarkFunc.
	obj := val.ToObject(runtime)
	light := obj.Get("light")
	dark := obj.Get("dark")

	if light == nil || dark == nil || goja.IsUndefined(light) || goja.IsUndefined(dark) {
		return nil, errors.New("invalid color object: missing 'light' or 'dark' properties")
	}

	// Validate inner colors
	if _, err := parseColor(runtime, light, hasDarkBg); err != nil {
		return nil, fmt.Errorf("invalid light color: %w", err)
	}
	lCol := lipgloss.Color(light.String())

	if _, err := parseColor(runtime, dark, hasDarkBg); err != nil {
		return nil, fmt.Errorf("invalid dark color: %w", err)
	}
	dCol := lipgloss.Color(dark.String())

	// Use lipgloss.LightDark to resolve the correct color variant based on
	// the detected terminal background. The hasDarkBg flag comes from
	// lipgloss.HasDarkBackground cached at Manager initialization.
	ld := lipgloss.LightDark(hasDarkBg)
	return ld(lCol, dCol), nil
}

// parseWhitespaceOptions parses options for Place calls.
func parseWhitespaceOptions(runtime *goja.Runtime, val goja.Value, hasDarkBg bool) ([]lipgloss.WhitespaceOption, error) {
	var opts []lipgloss.WhitespaceOption
	if goja.IsUndefined(val) || goja.IsNull(val) {
		return opts, nil
	}
	obj := val.ToObject(runtime)

	if chars := obj.Get("whitespaceChars"); chars != nil && !goja.IsUndefined(chars) && !goja.IsNull(chars) {
		opts = append(opts, lipgloss.WithWhitespaceChars(chars.String()))
	}
	// In v2, WithWhitespaceForeground/Background are replaced by WithWhitespaceStyle.
	// We build a style from the foreground/background properties.
	fgVal := obj.Get("whitespaceForeground")
	bgVal := obj.Get("whitespaceBackground")
	if fgVal != nil && !goja.IsUndefined(fgVal) || bgVal != nil && !goja.IsUndefined(bgVal) {
		style := lipgloss.NewStyle()
		if fgVal != nil && !goja.IsUndefined(fgVal) && !goja.IsNull(fgVal) {
			c, err := parseColor(runtime, fgVal, hasDarkBg)
			if err != nil {
				return nil, err
			}
			style = style.Foreground(c)
		}
		if bgVal != nil && !goja.IsUndefined(bgVal) && !goja.IsNull(bgVal) {
			c, err := parseColor(runtime, bgVal, hasDarkBg)
			if err != nil {
				return nil, err
			}
			style = style.Background(c)
		}
		opts = append(opts, lipgloss.WithWhitespaceStyle(style))
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

func borderToJS(b lipgloss.Border) map[string]any {
	return map[string]any{
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
