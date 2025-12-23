// Command generate-bubbletea-key-mapping generates keys_gen.go for the bubbletea package.
//
// This generator extracts KeyType constants from github.com/charmbracelet/bubbletea and
// generates a Go file exposing key metadata that can be used by the JS runtime.
//
// Usage:
//
//	go run ./internal/cmd/generate-bubbletea-key-mapping
//
// The generated file will be placed at internal/builtin/bubbletea/keys_gen.go.
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

	// Generate the output file
	output, err := generateOutput(keyNames, aliases)
	if err != nil {
		return fmt.Errorf("generating output: %w", err)
	}

	// Determine output path relative to this file's location
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("could not determine current file path")
	}

	// Go up to internal/cmd/generate-bubbletea-key-mapping, then to internal/builtin/bubbletea
	baseDir := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	outputPath := filepath.Join(baseDir, "builtin", "bubbletea", "keys_gen.go")

	// Write the output
	if err := os.WriteFile(outputPath, output, 0644); err != nil {
		return fmt.Errorf("writing output file: %w", err)
	}

	fmt.Printf("Generated %s\n", outputPath)
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

// generateOutput generates the Go source file content.
func generateOutput(keyNames map[string]string, aliases map[string]string) ([]byte, error) {
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
