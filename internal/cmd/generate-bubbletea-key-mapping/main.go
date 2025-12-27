// Command generate-bubbletea-key-mapping generates keys_gen.go and mouse_gen.go for the bubbletea package.
//
// This generator extracts KeyType constants and MouseButton/MouseAction constants from
// github.com/charmbracelet/bubbletea and generates Go files exposing metadata that can be
// used by the JS runtime.
//
// Usage:
//
//	go run ./internal/cmd/generate-bubbletea-key-mapping
//
// The generated files will be placed at internal/builtin/bubbletea/keys_gen.go and mouse_gen.go.
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"

	"golang.org/x/tools/go/packages"
)

// KeyDef represents a key definition extracted from bubbletea.
type KeyDef struct {
	Name       string // e.g., "KeyEnter" (exported alias) or "keyCR" (unexported)
	StringVal  string // e.g., "enter" (from keyNames map)
	AliasOf    string // if this is an alias, the target constant name
	IsExported bool   // whether the constant is exported (capital letter)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load the bubbletea package to extract KeyType information
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
	}

	pkgs, err := packages.Load(cfg, "github.com/charmbracelet/bubbletea")
	if err != nil {
		return fmt.Errorf("loading bubbletea package: %w", err)
	}

	if len(pkgs) == 0 {
		return fmt.Errorf("no packages found")
	}

	if len(pkgs[0].Errors) > 0 {
		for _, e := range pkgs[0].Errors {
			fmt.Fprintf(os.Stderr, "Package error: %v\n", e)
		}
		return fmt.Errorf("package has errors")
	}

	// Parse key.go specifically for keyNames and KeyType constants
	keyNames, aliases, err := extractKeyInfo(pkgs[0])
	if err != nil {
		return fmt.Errorf("extracting key info: %w", err)
	}

	// Generate the keys output file
	keysOutput, err := generateKeysOutput(keyNames, aliases)
	if err != nil {
		return fmt.Errorf("generating keys output: %w", err)
	}

	// Extract mouse button and action information
	mouseButtons, mouseActions, err := extractMouseInfo(pkgs[0])
	if err != nil {
		return fmt.Errorf("extracting mouse info: %w", err)
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

// extractKeyInfo parses the bubbletea package and extracts keyNames map and alias mappings.
// Returns:
// - keyNames: map from unexported constant name to string value (e.g., "keyCR" -> "enter")
// - aliases: map from exported alias name to unexported target (e.g., "KeyEnter" -> "keyCR")
func extractKeyInfo(pkg *packages.Package) (keyNames map[string]string, aliases map[string]string, err error) {
	keyNames = make(map[string]string)
	aliases = make(map[string]string)

	for _, file := range pkg.Syntax {
		// Extract keyNames map values
		ast.Inspect(file, func(n ast.Node) bool {
			switch decl := n.(type) {
			case *ast.GenDecl:
				if decl.Tok == token.VAR {
					for _, spec := range decl.Specs {
						if vs, ok := spec.(*ast.ValueSpec); ok {
							for i, name := range vs.Names {
								if name.Name == "keyNames" && i < len(vs.Values) {
									extractKeyNamesMap(vs.Values[i], keyNames)
								}
							}
						}
					}
				}
			}
			return true
		})
	}

	// Second pass: extract KeyType constant aliases (e.g., KeyEnter KeyType = keyCR)
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			switch decl := n.(type) {
			case *ast.GenDecl:
				if decl.Tok == token.CONST {
					for _, spec := range decl.Specs {
						if vs, ok := spec.(*ast.ValueSpec); ok {
							// Check if type is KeyType
							if vs.Type != nil {
								if ident, ok := vs.Type.(*ast.Ident); ok && ident.Name == "KeyType" {
									for i, name := range vs.Names {
										constName := name.Name
										// Only care about exported aliases
										if !ast.IsExported(constName) {
											continue
										}

										// Check if this is an alias (value is an identifier referencing another constant)
										if i < len(vs.Values) {
											if valIdent, ok := vs.Values[i].(*ast.Ident); ok {
												// This is an alias: KeyEnter = keyCR
												aliases[constName] = valIdent.Name
											}
										}
									}
								}
							}
						}
					}
				}
			}
			return true
		})
	}

	return keyNames, aliases, nil
}

// extractKeyNamesMap extracts key-value pairs from a composite literal that looks like:
// map[KeyType]string{ ... }
func extractKeyNamesMap(expr ast.Expr, result map[string]string) {
	compLit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return
	}

	for _, elt := range compLit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		// Get key (constant name)
		var keyName string
		switch k := kv.Key.(type) {
		case *ast.Ident:
			keyName = k.Name
		default:
			continue
		}

		// Get value (string literal)
		var strVal string
		switch v := kv.Value.(type) {
		case *ast.BasicLit:
			if v.Kind == token.STRING {
				// Properly unquote the string literal to interpret escape sequences
				unquoted, err := strconv.Unquote(v.Value)
				if err != nil {
					continue
				}
				strVal = unquoted
			}
		default:
			continue
		}

		if keyName != "" && strVal != "" {
			result[keyName] = strVal
		}
	}
}

// preferredNames defines which exported constant name to prefer when multiple
// constants map to the same string value. The first name in this list that matches
// will be used as the canonical name for KeyDefs (keyed by string value).
// All aliases will still be included in KeyDefsByName.
var preferredNames = map[string][]string{
	"enter":     {"KeyEnter", "KeyCtrlM"},
	"backspace": {"KeyBackspace", "KeyCtrlQuestionMark", "KeyCtrlH"},
	"esc":       {"KeyEsc", "KeyEscape", "KeyCtrlOpenBracket"},
	"tab":       {"KeyTab", "KeyCtrlI"},
	"ctrl+@":    {"KeyCtrlAt", "KeyNull"},
}

// generateKeysOutput generates the Go source file content for keys_gen.go.
func generateKeysOutput(keyNames map[string]string, aliases map[string]string) ([]byte, error) {
	// Build the final list of key definitions
	// We want: string value -> exported constant name -> tea.ExportedConstant
	//
	// Strategy:
	// 1. keyNames maps unexported constant -> string value (e.g., "keyCR" -> "enter")
	// 2. aliases maps exported constant -> unexported constant (e.g., "KeyEnter" -> "keyCR")
	// 3. We need to find: for each string value, ALL exported constants that map to it

	// Build unexported -> []exported alias mapping (multiple exported can alias same unexported)
	unexportedToAllExported := make(map[string][]string)
	for exported, unexported := range aliases {
		unexportedToAllExported[unexported] = append(unexportedToAllExported[unexported], exported)
	}
	// Sort each slice for determinism
	for k := range unexportedToAllExported {
		sort.Strings(unexportedToAllExported[k])
	}

	// Build final mapping: string value -> all exported constant names
	type keyEntry struct {
		stringVal     string
		canonicalName string   // The preferred/canonical name for this string value
		allNames      []string // All exported names that map to this string value
	}

	entriesByString := make(map[string]*keyEntry)

	// Process keyNames to build entries
	for unexported, stringVal := range keyNames {
		exportedList := unexportedToAllExported[unexported]
		if len(exportedList) == 0 {
			// If no alias, use the unexported name only if it's actually exported
			// (some constants in keyNames are exported, like KeyRunes)
			if ast.IsExported(unexported) {
				exportedList = []string{unexported}
			} else {
				// No exported alias for this key - skip
				continue
			}
		}

		entry := entriesByString[stringVal]
		if entry == nil {
			entry = &keyEntry{stringVal: stringVal}
			entriesByString[stringVal] = entry
		}
		entry.allNames = append(entry.allNames, exportedList...)
	}

	// Sort allNames for each entry and pick canonical name
	for stringVal, entry := range entriesByString {
		sort.Strings(entry.allNames)

		// Pick canonical name: use preferredNames if available, otherwise first alphabetically
		if prefs, ok := preferredNames[stringVal]; ok {
			for _, pref := range prefs {
				for _, name := range entry.allNames {
					if name == pref {
						entry.canonicalName = pref
						break
					}
				}
				if entry.canonicalName != "" {
					break
				}
			}
		}
		if entry.canonicalName == "" && len(entry.allNames) > 0 {
			entry.canonicalName = entry.allNames[0]
		}
	}

	// Convert to sorted slice
	var entries []*keyEntry
	for _, e := range entriesByString {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].stringVal < entries[j].stringVal
	})

	var buf bytes.Buffer

	// Write header
	buf.WriteString(`// Code generated by generate-bubbletea-key-mapping. DO NOT EDIT.

package bubbletea

import tea "github.com/charmbracelet/bubbletea"

// KeyDef represents metadata about a bubbletea key type.
type KeyDef struct {
	// Name is the Go constant name (e.g., "KeyEnter").
	Name string
	// String is the string representation from Key.String() (e.g., "enter").
	String string
	// Type is the actual tea.KeyType value.
	Type tea.KeyType
}

// KeyDefs contains all bubbletea KeyType definitions.
// The map is keyed by the String() representation for JS lookup efficiency.
// When multiple KeyType constants produce the same string, a canonical name is chosen.
var KeyDefs = map[string]KeyDef{
`)

	// Write KeyDefs (one per string value, using canonical name)
	for _, e := range entries {
		buf.WriteString(fmt.Sprintf("\t%q: {Name: %q, String: %q, Type: tea.%s},\n",
			e.stringVal, e.canonicalName, e.stringVal, e.canonicalName))
	}

	buf.WriteString(`}

// KeyDefsByName contains all bubbletea KeyType definitions keyed by constant name.
// Multiple constant names may map to the same key (e.g., KeyEsc and KeyEscape both map to "esc").
var KeyDefsByName = map[string]KeyDef{
`)

	// Write by-name map (ALL exported names, not just canonical)
	// Collect all name -> stringVal pairs
	type nameEntry struct {
		name      string
		stringVal string
	}
	var allNameEntries []nameEntry
	for _, e := range entries {
		for _, name := range e.allNames {
			allNameEntries = append(allNameEntries, nameEntry{name: name, stringVal: e.stringVal})
		}
	}
	// Sort by name for stable output
	sort.Slice(allNameEntries, func(i, j int) bool {
		return allNameEntries[i].name < allNameEntries[j].name
	})
	// Dedupe by name (in case of duplicates)
	seenNames := make(map[string]bool)
	for _, ne := range allNameEntries {
		if seenNames[ne.name] {
			continue
		}
		seenNames[ne.name] = true
		buf.WriteString(fmt.Sprintf("\t%q: KeyDefs[%q],\n", ne.name, ne.stringVal))
	}

	buf.WriteString(`}

// AllKeyTypes returns all known tea.KeyType values that have string representations.
// Uses canonical constant names for deterministic output.
var AllKeyTypes = []tea.KeyType{
`)

	// Write all key types (canonical names only)
	for _, e := range entries {
		buf.WriteString(fmt.Sprintf("\ttea.%s,\n", e.canonicalName))
	}

	buf.WriteString(`}
`)

	// Format the output
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		// If formatting fails, return the raw output for debugging
		return buf.Bytes(), fmt.Errorf("formatting output: %w\nRaw output:\n%s", err, buf.String())
	}

	return formatted, nil
}

// extractMouseInfo parses the bubbletea package and extracts mouseButtons and mouseActions maps.
// Returns:
// - mouseButtons: map from constant name to string value (e.g., "MouseButtonLeft" -> "left")
// - mouseActions: map from constant name to string value (e.g., "MouseActionPress" -> "press")
func extractMouseInfo(pkg *packages.Package) (mouseButtons map[string]string, mouseActions map[string]string, err error) {
	mouseButtons = make(map[string]string)
	mouseActions = make(map[string]string)

	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			switch decl := n.(type) {
			case *ast.GenDecl:
				if decl.Tok == token.VAR {
					for _, spec := range decl.Specs {
						if vs, ok := spec.(*ast.ValueSpec); ok {
							for i, name := range vs.Names {
								if name.Name == "mouseButtons" && i < len(vs.Values) {
									extractMouseMap(vs.Values[i], mouseButtons, "MouseButton")
								}
								if name.Name == "mouseActions" && i < len(vs.Values) {
									extractMouseMap(vs.Values[i], mouseActions, "MouseAction")
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

// extractMouseMap extracts key-value pairs from a map composite literal.
// The prefix is used to ensure we only capture the correct constant names.
func extractMouseMap(expr ast.Expr, result map[string]string, prefix string) {
	compLit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return
	}

	for _, elt := range compLit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		// Get key (constant name)
		var keyName string
		switch k := kv.Key.(type) {
		case *ast.Ident:
			keyName = k.Name
		default:
			continue
		}

		// Get value (string literal)
		var strVal string
		switch v := kv.Value.(type) {
		case *ast.BasicLit:
			if v.Kind == token.STRING {
				unquoted, err := strconv.Unquote(v.Value)
				if err != nil {
					continue
				}
				strVal = unquoted
			}
		default:
			continue
		}

		if keyName != "" && strVal != "" {
			result[keyName] = strVal
		}
	}
}

// generateMouseOutput generates the Go source file content for mouse_gen.go.
func generateMouseOutput(mouseButtons, mouseActions map[string]string) ([]byte, error) {
	var buf bytes.Buffer

	// Write header
	buf.WriteString(`// Code generated by generate-bubbletea-key-mapping. DO NOT EDIT.

package bubbletea

import tea "github.com/charmbracelet/bubbletea"

// MouseButtonDef represents metadata about a bubbletea MouseButton.
type MouseButtonDef struct {
	// Name is the Go constant name (e.g., "MouseButtonLeft").
	Name string
	// String is the string representation (e.g., "left").
	String string
	// Button is the actual tea.MouseButton value.
	Button tea.MouseButton
}

// MouseActionDef represents metadata about a bubbletea MouseAction.
type MouseActionDef struct {
	// Name is the Go constant name (e.g., "MouseActionPress").
	Name string
	// String is the string representation (e.g., "press").
	String string
	// Action is the actual tea.MouseAction value.
	Action tea.MouseAction
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
	sort.Slice(buttons, func(i, j int) bool {
		return buttons[i].strVal < buttons[j].strVal
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
	sort.Slice(buttons, func(i, j int) bool {
		return buttons[i].name < buttons[j].name
	})
	for _, b := range buttons {
		buf.WriteString(fmt.Sprintf("\t%q: MouseButtonDefs[%q],\n", b.name, b.strVal))
	}

	buf.WriteString(`}

// MouseActionDefs contains all bubbletea MouseAction definitions.
// The map is keyed by the String() representation for JS lookup efficiency.
var MouseActionDefs = map[string]MouseActionDef{
`)

	// Sort actions by string value for deterministic output
	type actionEntry struct {
		name   string
		strVal string
	}
	var actions []actionEntry
	for name, strVal := range mouseActions {
		actions = append(actions, actionEntry{name: name, strVal: strVal})
	}
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].strVal < actions[j].strVal
	})

	for _, a := range actions {
		buf.WriteString(fmt.Sprintf("\t%q: {Name: %q, String: %q, Action: tea.%s},\n",
			a.strVal, a.name, a.strVal, a.name))
	}

	buf.WriteString(`}

// MouseActionDefsByName contains all bubbletea MouseAction definitions keyed by constant name.
var MouseActionDefsByName = map[string]MouseActionDef{
`)

	// Sort by name for deterministic output
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].name < actions[j].name
	})
	for _, a := range actions {
		buf.WriteString(fmt.Sprintf("\t%q: MouseActionDefs[%q],\n", a.name, a.strVal))
	}

	buf.WriteString(`}

// AllMouseButtons returns all known tea.MouseButton values that have string representations.
var AllMouseButtons = []tea.MouseButton{
`)

	// Use the original sorted-by-strVal order
	sort.Slice(buttons, func(i, j int) bool {
		return buttons[i].strVal < buttons[j].strVal
	})
	for _, b := range buttons {
		buf.WriteString(fmt.Sprintf("\ttea.%s,\n", b.name))
	}

	buf.WriteString(`}

// AllMouseActions returns all known tea.MouseAction values that have string representations.
var AllMouseActions = []tea.MouseAction{
`)

	// Use the original sorted-by-strVal order
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].strVal < actions[j].strVal
	})
	for _, a := range actions {
		buf.WriteString(fmt.Sprintf("\ttea.%s,\n", a.name))
	}

	buf.WriteString(`}

// IsWheelButton returns true if the button is a wheel button.
// This mirrors tea.MouseEvent.IsWheel() for use in parsing/validation.
func IsWheelButton(b tea.MouseButton) bool {
	return b == tea.MouseButtonWheelUp || b == tea.MouseButtonWheelDown ||
		b == tea.MouseButtonWheelLeft || b == tea.MouseButtonWheelRight
}
`)

	// Format the output
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return buf.Bytes(), fmt.Errorf("formatting mouse output: %w\nRaw output:\n%s", err, buf.String())
	}

	return formatted, nil
}
