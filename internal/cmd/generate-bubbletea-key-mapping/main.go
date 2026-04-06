// Command generate-bubbletea-key-mapping generates keys_gen.go and mouse_gen.go for the bubbletea package.
//
// This generator extracts key and mouse constants from charm.land/bubbletea/v2
// and generates Go files exposing metadata that can be used by the JS runtime.
//
// In v2, key constants are rune values (not KeyType), and mouse constants
// have been renamed (e.g., MouseButtonLeft → MouseLeft).
//
// Usage:
//
//	go run ./internal/cmd/generate-bubbletea-key-mapping
//
// The generated files will be placed at internal/builtin/bubbletea/keys_gen.go and mouse_gen.go.
package main

import (
	"bytes"
	"cmp"
	"fmt"
	"go/ast"
	"go/constant"
	"go/format"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"

	"golang.org/x/tools/go/packages"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load the bubbletea package to extract constants
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
	}

	pkgs, err := packages.Load(cfg, "charm.land/bubbletea/v2")
	if err != nil {
		return fmt.Errorf("loading bubbletea package: %w", err)
	}

	if len(pkgs) == 0 {
		return fmt.Errorf("no packages found")
	}

	if len(pkgs[0].Errors) > 0 {
		for _, e := range pkgs[0].Errors {
			_, _ = fmt.Fprintf(os.Stderr, "Package error: %v\n", e)
		}
		return fmt.Errorf("package has errors")
	}

	// Extract key constants (rune-based in v2)
	keyEntries, allKeyEntries, err := extractKeyConstants(pkgs[0])
	if err != nil {
		return fmt.Errorf("extracting key constants: %w", err)
	}

	// Generate the keys output file
	keysOutput, err := generateKeysOutput(keyEntries, allKeyEntries)
	if err != nil {
		return fmt.Errorf("generating keys output: %w", err)
	}

	// Extract mouse button and action constants
	mouseButtons, mouseActions, err := extractMouseConstants(pkgs[0])
	if err != nil {
		return fmt.Errorf("extracting mouse constants: %w", err)
	}

	// Generate the mouse output file
	mouseOutput, err := generateMouseOutput(mouseButtons, mouseActions)
	if err != nil {
		return fmt.Errorf("generating mouse output: %w", err)
	}

	// Determine output path relative to this file's location
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("could not determine current file path")
	}

	// Go up to internal/cmd/generate-bubbletea-key-mapping, then to internal/builtin/bubbletea
	baseDir := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	keysOutputPath := filepath.Join(baseDir, "builtin", "bubbletea", "keys_gen.go")
	mouseOutputPath := filepath.Join(baseDir, "builtin", "bubbletea", "mouse_gen.go")

	// Write the output files
	if err := os.WriteFile(keysOutputPath, keysOutput, 0644); err != nil {
		return fmt.Errorf("writing keys output file: %w", err)
	}
	fmt.Printf("Generated %s\n", keysOutputPath)

	if err := os.WriteFile(mouseOutputPath, mouseOutput, 0644); err != nil {
		return fmt.Errorf("writing mouse output file: %w", err)
	}
	fmt.Printf("Generated %s\n", mouseOutputPath)

	return nil
}

// keyEntry represents a key constant extracted from bubbletea.
type keyEntry struct {
	Name      string // e.g., "KeyEnter"
	Code      rune   // The rune value
	StringVal string // e.g., "enter" (from Key.String())
}

// extractKeyConstants extracts all exported key rune constants from the bubbletea package.
// In v2, key constants are rune values (e.g., KeyEnter = '\r', KeyF1 = uv.KeyF1, etc.)
// We extract them via AST and then compute their String() representation.
func extractKeyConstants(pkg *packages.Package) ([]keyEntry, []keyEntry, error) {
	// Collect all exported constant names that start with "Key" and have rune type
	type constInfo struct {
		name  string
		value rune
	}
	var constants []constInfo

	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			decl, ok := n.(*ast.GenDecl)
			if !ok || decl.Tok != token.CONST {
				return true
			}

			for _, spec := range decl.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}

				// Check if type is rune (or an alias that resolves to rune)
				isRune := false
				if vs.Type != nil {
					if ident, ok := vs.Type.(*ast.Ident); ok && ident.Name == "rune" {
						isRune = true
					}
				}

				for i, name := range vs.Names {
					// Only exported constants starting with "Key"
					if !ast.IsExported(name.Name) || len(name.Name) < 4 || name.Name[:3] != "Key" {
						continue
					}

					// Skip KeyMod, KeyMsg, KeyPressMsg, KeyReleaseMsg, etc. (types, not constants)
					if name.Name == "KeyMod" || name.Name == "KeyMsg" ||
						name.Name == "KeyPressMsg" || name.Name == "KeyReleaseMsg" {
						continue
					}

					var code rune
					if isRune {
						// Direct rune constant: const KeyEnter rune = '\r'
						if i < len(vs.Values) {
							if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.CHAR {
								unquoted, err := strconv.Unquote(lit.Value)
								if err == nil && len([]rune(unquoted)) == 1 {
									code = []rune(unquoted)[0]
								}
							}
						}
					} else {
						// Might be an alias to an ultraviolet constant or a rune-typed const without explicit type
						// Try to get the value from types info
						if obj := pkg.TypesInfo.ObjectOf(name); obj != nil {
							if constObj, ok := obj.(*types.Const); ok {
								if val := constObj.Val(); val.Kind() == constant.Int {
									if i64, ok := constant.Int64Val(val); ok {
										code = rune(i64)
									}
								}
							}
						}
					}

					if code != 0 {
						constants = append(constants, constInfo{name: name.Name, value: code})
					}
				}
			}
			return true
		})
	}

	// Build known string mappings
	knownStrings := map[rune]string{
		'\x7f': "backspace", '\t': "tab", '\r': "enter", '\n': "enter",
		'\x1b': "esc", ' ': "space",
	}
	specialKeyStrings := map[string]string{
		"KeyUp": "up", "KeyDown": "down", "KeyRight": "right", "KeyLeft": "left",
		"KeyBegin": "begin", "KeyFind": "find", "KeyInsert": "insert", "KeyDelete": "delete",
		"KeySelect": "select", "KeyPgUp": "pgup", "KeyPgDown": "pgdown", "KeyHome": "home",
		"KeyEnd": "end", "KeyKpEnter": "kp enter", "KeyKpEqual": "kp equal",
		"KeyKpMultiply": "kp multiply", "KeyKpPlus": "kp plus", "KeyKpComma": "kp comma",
		"KeyKpMinus": "kp minus", "KeyKpDecimal": "kp decimal", "KeyKpDivide": "kp divide",
		"KeyKp0": "kp 0", "KeyKp1": "kp 1", "KeyKp2": "kp 2", "KeyKp3": "kp 3",
		"KeyKp4": "kp 4", "KeyKp5": "kp 5", "KeyKp6": "kp 6", "KeyKp7": "kp 7",
		"KeyKp8": "kp 8", "KeyKp9": "kp 9", "KeyKpSep": "kp sep", "KeyKpUp": "kp up",
		"KeyKpDown": "kp down", "KeyKpLeft": "kp left", "KeyKpRight": "kp right",
		"KeyKpPgUp": "kp pgup", "KeyKpPgDown": "kp pgdown", "KeyKpHome": "kp home",
		"KeyKpEnd": "kp end", "KeyKpInsert": "kp insert", "KeyKpDelete": "kp delete",
		"KeyKpBegin": "kp begin", "KeyCapsLock": "caps lock", "KeyScrollLock": "scroll lock",
		"KeyNumLock": "num lock", "KeyPrintScreen": "print screen", "KeyPause": "pause",
		"KeyMenu": "menu", "KeyMediaPlay": "media play", "KeyMediaPause": "media pause",
		"KeyMediaPlayPause": "media play pause", "KeyMediaReverse": "media reverse",
		"KeyMediaStop": "media stop", "KeyMediaFastForward": "media fast forward",
		"KeyMediaRewind": "media rewind", "KeyMediaNext": "media next", "KeyMediaPrev": "media prev",
		"KeyMediaRecord": "media record", "KeyLowerVol": "lower vol", "KeyRaiseVol": "raise vol",
		"KeyMute": "mute", "KeyLeftShift": "left shift", "KeyLeftAlt": "left alt",
		"KeyLeftCtrl": "left ctrl", "KeyLeftSuper": "left super", "KeyLeftHyper": "left hyper",
		"KeyLeftMeta": "left meta", "KeyRightShift": "right shift", "KeyRightAlt": "right alt",
		"KeyRightCtrl": "right ctrl", "KeyRightSuper": "right super", "KeyRightHyper": "right hyper",
		"KeyRightMeta": "right meta", "KeyIsoLevel3Shift": "iso level 3 shift",
		"KeyIsoLevel5Shift": "iso level 5 shift", "KeyExtended": "extended", "KeyRunes": "runes",
	}
	ctrlKeyStrings := map[string]string{
		"KeyCtrlA": "ctrl+a", "KeyCtrlB": "ctrl+b", "KeyCtrlC": "ctrl+c",
		"KeyCtrlD": "ctrl+c", "KeyCtrlE": "ctrl+e", "KeyCtrlF": "ctrl+f",
		"KeyCtrlG": "ctrl+g", "KeyCtrlH": "ctrl+h", "KeyCtrlI": "tab",
		"KeyCtrlJ": "ctrl+j", "KeyCtrlK": "ctrl+k", "KeyCtrlL": "ctrl+l",
		"KeyCtrlM": "enter", "KeyCtrlN": "ctrl+n", "KeyCtrlO": "ctrl+o",
		"KeyCtrlP": "ctrl+p", "KeyCtrlQ": "ctrl+q", "KeyCtrlR": "ctrl+r",
		"KeyCtrlS": "ctrl+s", "KeyCtrlT": "ctrl+t", "KeyCtrlU": "ctrl+u",
		"KeyCtrlV": "ctrl+v", "KeyCtrlW": "ctrl+w", "KeyCtrlX": "ctrl+x",
		"KeyCtrlY": "ctrl+y", "KeyCtrlZ": "ctrl+z", "KeyCtrlAt": "ctrl+@",
		"KeyCtrlBackslash": "ctrl+\\", "KeyCtrlCloseBracket": "ctrl+]",
		"KeyCtrlCaret": "ctrl+^", "KeyCtrlUnderscore": "ctrl+_",
		"KeyCtrlQuestionMark": "backspace", "KeyCtrlOpenBracket": "esc",
		"KeyCtrlUp": "ctrl+up", "KeyCtrlDown": "ctrl+down", "KeyCtrlLeft": "ctrl+left",
		"KeyCtrlRight": "ctrl+right", "KeyCtrlHome": "ctrl+home", "KeyCtrlEnd": "ctrl+end",
		"KeyCtrlPgUp": "ctrl+pgup", "KeyCtrlPgDown": "ctrl+pgdown",
		"KeyCtrlShiftUp": "ctrl+shift+up", "KeyCtrlShiftDown": "ctrl+shift+down",
		"KeyCtrlShiftLeft": "ctrl+shift+left", "KeyCtrlShiftRight": "ctrl+shift+right",
		"KeyCtrlShiftHome": "ctrl+shift+home", "KeyCtrlShiftEnd": "ctrl+shift+end",
	}
	shiftKeyStrings := map[string]string{
		"KeyShiftUp": "shift+up", "KeyShiftDown": "shift+down", "KeyShiftLeft": "shift+left",
		"KeyShiftRight": "shift+right", "KeyShiftHome": "shift+home", "KeyShiftEnd": "shift+end",
		"KeyShiftTab": "shift+tab",
	}
	uvConstants := []struct{ name, strVal string }{
		{"KeyF1", "f1"}, {"KeyF2", "f2"}, {"KeyF3", "f3"}, {"KeyF4", "f4"},
		{"KeyF5", "f5"}, {"KeyF6", "f6"}, {"KeyF7", "f7"}, {"KeyF8", "f8"},
		{"KeyF9", "f9"}, {"KeyF10", "f10"}, {"KeyF11", "f11"}, {"KeyF12", "f12"},
		{"KeyF13", "f13"}, {"KeyF14", "f14"}, {"KeyF15", "f15"}, {"KeyF16", "f16"},
		{"KeyF17", "f17"}, {"KeyF18", "f18"}, {"KeyF19", "f19"}, {"KeyF20", "f20"},
		{"KeyF21", "f21"}, {"KeyF22", "f22"}, {"KeyF23", "f23"}, {"KeyF24", "f24"},
		{"KeyF25", "f25"}, {"KeyF26", "f26"}, {"KeyF27", "f27"}, {"KeyF28", "f28"},
		{"KeyF29", "f29"}, {"KeyF30", "f30"}, {"KeyF31", "f31"}, {"KeyF32", "f32"},
		{"KeyF33", "f33"}, {"KeyF34", "f34"}, {"KeyF35", "f35"}, {"KeyF36", "f36"},
		{"KeyF37", "f37"}, {"KeyF38", "f38"}, {"KeyF39", "f39"}, {"KeyF40", "f40"},
		{"KeyF41", "f41"}, {"KeyF42", "f42"}, {"KeyF43", "f43"}, {"KeyF44", "f44"},
		{"KeyF45", "f45"}, {"KeyF46", "f46"}, {"KeyF47", "f47"}, {"KeyF48", "f48"},
		{"KeyF49", "f49"}, {"KeyF50", "f50"}, {"KeyF51", "f51"}, {"KeyF52", "f52"},
		{"KeyF53", "f53"}, {"KeyF54", "f54"}, {"KeyF55", "f55"}, {"KeyF56", "f56"},
		{"KeyF57", "f57"}, {"KeyF58", "f58"}, {"KeyF59", "f59"}, {"KeyF60", "f60"},
		{"KeyF61", "f61"}, {"KeyF62", "f62"}, {"KeyF63", "f63"},
	}

	getStrVal := func(c constInfo) string {
		if s, ok := specialKeyStrings[c.name]; ok {
			return s
		}
		if s, ok := ctrlKeyStrings[c.name]; ok {
			return s
		}
		if s, ok := shiftKeyStrings[c.name]; ok {
			return s
		}
		if s, ok := knownStrings[c.value]; ok {
			return s
		}
		if c.value >= 32 && c.value < 127 {
			return string(c.value)
		}
		return ""
	}

	// Build deduplicated entries (for KeyDefs)
	var entries []keyEntry
	seenStrings := make(map[string]bool)

	// Add ctrl+letter keys manually (these are defined in ultraviolet, not bubbletea,
	// so the AST parser doesn't find them directly).
	// In v2, ctrl+letter is represented as the letter rune with ModCtrl, but we still
	// need entries in KeyDefs for ParseKey to work.
	ctrlKeyEntries := []keyEntry{
		{Name: "KeyCtrlA", Code: 'a', StringVal: "ctrl+a"},
		{Name: "KeyCtrlB", Code: 'b', StringVal: "ctrl+b"},
		{Name: "KeyCtrlC", Code: 'c', StringVal: "ctrl+c"},
		{Name: "KeyCtrlD", Code: 'd', StringVal: "ctrl+d"},
		{Name: "KeyCtrlE", Code: 'e', StringVal: "ctrl+e"},
		{Name: "KeyCtrlF", Code: 'f', StringVal: "ctrl+f"},
		{Name: "KeyCtrlG", Code: 'g', StringVal: "ctrl+g"},
		{Name: "KeyCtrlH", Code: 'h', StringVal: "ctrl+h"},
		{Name: "KeyCtrlJ", Code: 'j', StringVal: "ctrl+j"},
		{Name: "KeyCtrlK", Code: 'k', StringVal: "ctrl+k"},
		{Name: "KeyCtrlL", Code: 'l', StringVal: "ctrl+l"},
		{Name: "KeyCtrlN", Code: 'n', StringVal: "ctrl+n"},
		{Name: "KeyCtrlO", Code: 'o', StringVal: "ctrl+o"},
		{Name: "KeyCtrlP", Code: 'p', StringVal: "ctrl+p"},
		{Name: "KeyCtrlQ", Code: 'q', StringVal: "ctrl+q"},
		{Name: "KeyCtrlR", Code: 'r', StringVal: "ctrl+r"},
		{Name: "KeyCtrlS", Code: 's', StringVal: "ctrl+s"},
		{Name: "KeyCtrlT", Code: 't', StringVal: "ctrl+t"},
		{Name: "KeyCtrlU", Code: 'u', StringVal: "ctrl+u"},
		{Name: "KeyCtrlV", Code: 'v', StringVal: "ctrl+v"},
		{Name: "KeyCtrlW", Code: 'w', StringVal: "ctrl+w"},
		{Name: "KeyCtrlX", Code: 'x', StringVal: "ctrl+x"},
		{Name: "KeyCtrlY", Code: 'y', StringVal: "ctrl+y"},
		{Name: "KeyCtrlZ", Code: 'z', StringVal: "ctrl+z"},
		{Name: "KeyCtrlAt", Code: '@', StringVal: "ctrl:@"},
		{Name: "KeyCtrlBackslash", Code: '\\', StringVal: "ctrl+\\"},
		{Name: "KeyCtrlCloseBracket", Code: ']', StringVal: "ctrl+]"},
		{Name: "KeyCtrlCaret", Code: '^', StringVal: "ctrl+^"},
		{Name: "KeyCtrlUnderscore", Code: '_', StringVal: "ctrl+_"},
	}
	for _, e := range ctrlKeyEntries {
		if !seenStrings[e.StringVal] {
			seenStrings[e.StringVal] = true
			entries = append(entries, e)
		}
	}

	for _, c := range constants {
		strVal := getStrVal(c)
		if strVal == "" || seenStrings[strVal] {
			continue
		}
		seenStrings[strVal] = true
		entries = append(entries, keyEntry{Name: c.name, Code: c.value, StringVal: strVal})
	}
	// Add F-keys
	for _, uv := range uvConstants {
		if seenStrings[uv.strVal] {
			continue
		}
		for _, c := range constants {
			if c.name == uv.name {
				entries = append(entries, keyEntry{Name: c.name, Code: c.value, StringVal: uv.strVal})
				seenStrings[uv.strVal] = true
				break
			}
		}
	}
	slices.SortFunc(entries, func(a, b keyEntry) int { return cmp.Compare(a.StringVal, b.StringVal) })

	// Build full list of ALL exported names (for KeyDefsByName)
	var allEntries []keyEntry
	allSeen := make(map[string]bool)

	// Add ctrl key entries to allEntries too
	for _, e := range ctrlKeyEntries {
		if !allSeen[e.StringVal] {
			allSeen[e.StringVal] = true
		}
		allEntries = append(allEntries, e)
	}

	for _, c := range constants {
		strVal := getStrVal(c)
		if strVal == "" {
			continue
		}
		allEntries = append(allEntries, keyEntry{Name: c.name, Code: c.value, StringVal: strVal})
		allSeen[strVal] = true
	}
	for _, uv := range uvConstants {
		if allSeen[uv.strVal] {
			continue
		}
		for _, c := range constants {
			if c.name == uv.name {
				allEntries = append(allEntries, keyEntry{Name: c.name, Code: c.value, StringVal: uv.strVal})
				allSeen[uv.strVal] = true
				break
			}
		}
	}

	return entries, allEntries, nil
}

// generateKeysOutput generates the Go source file content for keys_gen.go.
// entries is the deduplicated list (for KeyDefs), allEntries includes all names (for KeyDefsByName).
func generateKeysOutput(entries []keyEntry, allEntries []keyEntry) ([]byte, error) {
	var buf bytes.Buffer

	// Write header
	buf.WriteString(`// Code generated by generate-bubbletea-key-mapping. DO NOT EDIT.

package bubbletea

// KeyDef represents metadata about a bubbletea key.
type KeyDef struct {
	// Name is the Go constant name (e.g., "KeyEnter").
	Name string
	// String is the string representation from Key.String() (e.g., "enter").
	String string
	// Code is the actual rune code value.
	Code rune
}

// KeyDefs contains all bubbletea key definitions.
// The map is keyed by the String() representation for JS lookup efficiency.
// When multiple key constants produce the same string, a canonical name is chosen.
var KeyDefs = map[string]KeyDef{
`)

	// Write KeyDefs (one per string value, using canonical name)
	for _, e := range entries {
		buf.WriteString(fmt.Sprintf("\t%q: {Name: %q, String: %q, Code: %q},\n",
			e.StringVal, e.Name, e.StringVal, e.Code))
	}

	buf.WriteString(`}

// KeyDefsByName contains all bubbletea key definitions keyed by constant name.
// Multiple constant names may map to the same key (e.g., KeyEsc and KeyEscape both map to "esc").
var KeyDefsByName = map[string]KeyDef{
`)

	// Write by-name map (ALL exported names, including aliases)
	for _, e := range allEntries {
		buf.WriteString(fmt.Sprintf("\t%q: KeyDefs[%q],\n", e.Name, e.StringVal))
	}

	buf.WriteString(`}

// AllKeyCodes returns all known key rune values that have string representations.
// Uses canonical constant names for deterministic output.
var AllKeyCodes = []rune{
`)

	// Write all key codes (canonical names only)
	for _, e := range entries {
		buf.WriteString(fmt.Sprintf("\t%q, // %s\n", e.Code, e.Name))
	}

	buf.WriteString(`}
`)

	// Format the output
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return buf.Bytes(), fmt.Errorf("formatting output: %w\nRaw output:\n%s", err, buf.String())
	}

	return formatted, nil
}

// extractMouseConstants extracts mouse button constants from the bubbletea package.
// In v2, mouse buttons are: MouseLeft, MouseRight, MouseMiddle, MouseWheelUp, etc.
// (no "MouseButton" prefix). MouseAction no longer exists - mouse events are
// split into separate message types (MouseClickMsg, MouseReleaseMsg, etc.).
func extractMouseConstants(pkg *packages.Package) (mouseButtons map[string]string, mouseActions map[string]string, err error) {
	mouseButtons = make(map[string]string)
	mouseActions = make(map[string]string) // Always empty in v2

	// Known v2 mouse button constants and their string representations
	knownButtons := map[string]string{
		"MouseLeft":       "left",
		"MouseRight":      "right",
		"MouseMiddle":     "middle",
		"MouseWheelUp":    "wheel up",
		"MouseWheelDown":  "wheel down",
		"MouseWheelLeft":  "wheel left",
		"MouseWheelRight": "wheel right",
		"MouseBackward":   "backward",
		"MouseForward":    "forward",
		"MouseNone":       "none",
		"MouseButton10":   "button 10",
		"MouseButton11":   "button 11",
	}

	// Extract from AST - look for constants with MouseButton type
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			decl, ok := n.(*ast.GenDecl)
			if !ok || decl.Tok != token.CONST {
				return true
			}

			for _, spec := range decl.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}

				for i, name := range vs.Names {
					if !ast.IsExported(name.Name) {
						continue
					}

					// Check if this is a mouse button constant
					if strVal, ok := knownButtons[name.Name]; ok {
						mouseButtons[name.Name] = strVal
						continue
					}

					// Try to extract from the value if it's a string literal
					if vs.Type != nil {
						if ident, ok := vs.Type.(*ast.Ident); ok && ident.Name == "MouseButton" && i < len(vs.Values) {
							if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
								unquoted, err := strconv.Unquote(lit.Value)
								if err == nil {
									mouseButtons[name.Name] = unquoted
								}
							}
						}
					}
				}
			}
			return true
		})
	}

	return mouseButtons, mouseActions, nil
}

// generateMouseOutput generates the Go source file content for mouse_gen.go.
func generateMouseOutput(mouseButtons, mouseActions map[string]string) ([]byte, error) {
	var buf bytes.Buffer

	// Write header
	buf.WriteString(`// Code generated by generate-bubbletea-key-mapping. DO NOT EDIT.

package bubbletea

import tea "charm.land/bubbletea/v2"

// MouseButtonDef represents metadata about a bubbletea MouseButton.
type MouseButtonDef struct {
	// Name is the Go constant name (e.g., "MouseLeft").
	Name string
	// String is the string representation (e.g., "left").
	String string
	// Button is the actual tea.MouseButton value.
	Button tea.MouseButton
}

// MouseButtonDefs contains all bubbletea MouseButton definitions.
// The map is keyed by the String() representation for JS lookup efficiency.
var MouseButtonDefs = map[string]MouseButtonDef{
`)

	// Sort buttons by string value for deterministic output
	type buttonEntry struct {
		name   string
		strVal string
	}
	var buttons []buttonEntry
	for name, strVal := range mouseButtons {
		buttons = append(buttons, buttonEntry{name: name, strVal: strVal})
	}
	slices.SortFunc(buttons, func(a, b buttonEntry) int {
		return cmp.Compare(a.strVal, b.strVal)
	})

	for _, b := range buttons {
		buf.WriteString(fmt.Sprintf("\t%q: {Name: %q, String: %q, Button: tea.%s},\n",
			b.strVal, b.name, b.strVal, b.name))
	}

	buf.WriteString(`}

// MouseButtonDefsByName contains all bubbletea MouseButton definitions keyed by constant name.
var MouseButtonDefsByName = map[string]MouseButtonDef{
`)

	// Sort by name for deterministic output
	slices.SortFunc(buttons, func(a, b buttonEntry) int {
		return cmp.Compare(a.name, b.name)
	})
	for _, b := range buttons {
		buf.WriteString(fmt.Sprintf("\t%q: MouseButtonDefs[%q],\n", b.name, b.strVal))
	}

	buf.WriteString(`}

// AllMouseButtons returns all known tea.MouseButton values that have string representations.
var AllMouseButtons = []tea.MouseButton{
`)

	// Use the original sorted-by-strVal order
	slices.SortFunc(buttons, func(a, b buttonEntry) int {
		return cmp.Compare(a.strVal, b.strVal)
	})
	for _, b := range buttons {
		buf.WriteString(fmt.Sprintf("\ttea.%s,\n", b.name))
	}

	buf.WriteString(`}

// IsWheelButton returns true if the button is a wheel button.
func IsWheelButton(b tea.MouseButton) bool {
	return b == tea.MouseWheelUp || b == tea.MouseWheelDown ||
		b == tea.MouseWheelLeft || b == tea.MouseWheelRight
}
`)

	// Format the output
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return buf.Bytes(), fmt.Errorf("formatting mouse output: %w\nRaw output:\n%s", err, buf.String())
	}

	return formatted, nil
}
