package bubbletea

import (
	"strings"
	"testing"
)

// TestKeyDefsStringParity verifies that generated KeyDefs have String values
// that match the actual key string representation.
func TestKeyDefsStringParity(t *testing.T) {
	for stringVal, keyDef := range KeyDefs {
		t.Run(keyDef.Name, func(t *testing.T) {
			// The key in the map should match the String field
			if stringVal != keyDef.String {
				t.Errorf("map key %q doesn't match keyDef.String %q", stringVal, keyDef.String)
			}
		})
	}
}

// TestKeyDefsByNameParity verifies that KeyDefsByName correctly references KeyDefs.
// Note: Multiple constant names may map to the same KeyDef (e.g., KeyEsc and KeyEscape).
// In such cases, the KeyDef.Name field contains the canonical name, not the alias.
func TestKeyDefsByNameParity(t *testing.T) {
	// Define known aliases that map to canonical names in v2.
	// In v2, ultraviolet uses different canonical names than v1:
	// - "KeyEscape" is canonical (not "KeyEsc")
	// - "KeyEnter" is canonical (not "KeyReturn")
	// - "KeyCtrlC" is canonical (no "KeyBreak" alias in v2)
	// - "KeyBackspace" is canonical
	// - "KeyTab" is canonical
	// - "KeyCtrlAt" is canonical
	canonicalNames := map[string]string{
		"KeyEsc":              "KeyEscape",
		"KeyCtrlOpenBracket":  "KeyEscape", // "esc" maps to KeyEscape in v2
		"KeyCtrlM":            "KeyEnter",  // "enter" maps to KeyEnter in v2
		"KeyReturn":           "KeyEnter",  // "enter" maps to KeyEnter in v2
		"KeyCtrlQuestionMark": "KeyBackspace",
		"KeyCtrlI":            "KeyTab",
		"KeyNull":             "KeyCtrlAt",
	}

	for name, keyDef := range KeyDefsByName {
		t.Run(name, func(t *testing.T) {
			// Check if this is a known alias
			expectedName := name
			if canonical, ok := canonicalNames[name]; ok {
				expectedName = canonical
			}

			// Name should match the expected (canonical) name
			if keyDef.Name != expectedName {
				t.Errorf("KeyDefsByName[%q].Name = %q, want %q", name, keyDef.Name, expectedName)
			}

			// Should be able to look up in KeyDefs by String
			byString, ok := KeyDefs[keyDef.String]
			if !ok {
				t.Errorf("KeyDefs[%q] not found for KeyDefsByName[%q]", keyDef.String, name)
				return
			}

			// Both should reference the same canonical name
			if byString.Name != keyDef.Name {
				t.Errorf("KeyDefs[%q].Name = %q, KeyDefsByName[%q].Name = %q", keyDef.String, byString.Name, name, keyDef.Name)
			}
		})
	}
}

// TestAllKeyCodesContainsCoreKeys verifies that essential keys are present in
// KeyDefs, that their Code values are non-zero, their Names start with "Key",
// and that each code appears in AllKeyCodes.
func TestAllKeyCodesContainsCoreKeys(t *testing.T) {
	essentialKeys := []struct {
		keyStr string // lookup key in KeyDefs map
	}{
		{"enter"},
		{"esc"},
		{"backspace"},
		{"tab"},
		{"delete"},
		{"up"},
		{"down"},
		{"left"},
		{"right"},
		{"home"},
		{"end"},
		{"pgup"},
		{"pgdown"},
		{"ctrl+c"},
	}

	// Build a set of codes from AllKeyCodes for quick lookup.
	codeSet := make(map[rune]bool)
	for _, code := range AllKeyCodes {
		codeSet[code] = true
	}

	for _, ek := range essentialKeys {
		t.Run(ek.keyStr, func(t *testing.T) {
			// 1. The key string must exist in KeyDefs.
			kd, ok := KeyDefs[ek.keyStr]
			if !ok {
				t.Fatalf("KeyDefs missing essential key string: %q", ek.keyStr)
			}

			// 2. The Code must be non-zero.
			if kd.Code == 0 {
				t.Errorf("KeyDefs[%q].Code is zero", ek.keyStr)
			}

			// 3. The Name must start with "Key".
			if !strings.HasPrefix(kd.Name, "Key") {
				t.Errorf("KeyDefs[%q].Name = %q, want prefix %q", ek.keyStr, kd.Name, "Key")
			}

			// 4. The Code must appear in AllKeyCodes.
			if !codeSet[kd.Code] {
				t.Errorf("AllKeyCodes missing essential code %q for key %q", kd.Code, ek.keyStr)
			}
		})
	}
}

// TestKeyDefsCoversAllKeyCodes verifies that KeyDefs has an entry for each code in AllKeyCodes.
func TestKeyDefsCoversAllKeyCodes(t *testing.T) {
	for _, code := range AllKeyCodes {
		found := false
		for _, keyDef := range KeyDefs {
			if keyDef.Code == code {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("KeyDefs missing entry for code %q", code)
		}
	}
}

// TestFunctionKeysCovered verifies F1-F20 function keys are present.
func TestFunctionKeysCovered(t *testing.T) {
	functionKeyNames := []string{
		"KeyF1", "KeyF2", "KeyF3", "KeyF4", "KeyF5",
		"KeyF6", "KeyF7", "KeyF8", "KeyF9", "KeyF10",
		"KeyF11", "KeyF12", "KeyF13", "KeyF14", "KeyF15",
		"KeyF16", "KeyF17", "KeyF18", "KeyF19", "KeyF20",
	}

	for _, name := range functionKeyNames {
		if _, ok := KeyDefsByName[name]; !ok {
			t.Errorf("KeyDefsByName missing function key: %q", name)
		}
	}
}

// TestCtrlKeysCovered verifies ctrl+letter keys are present.
// In v2, ctrl+letter keys are not separate constants - they're represented
// as the letter rune with ModCtrl. The KeyDefs map contains entries for
// the string representations like "ctrl+a", "ctrl+b", etc.
func TestCtrlKeysCovered(t *testing.T) {
	// In v2, ctrl keys are handled via Mod modifier, not separate constants.
	// Verify that the key string representations exist in KeyDefs.
	ctrlStrings := []string{
		"ctrl+a", "ctrl+b", "ctrl+c", "ctrl+d", "ctrl+e",
		"ctrl+f", "ctrl+g", "ctrl+h", "ctrl+j", "ctrl+k",
		"ctrl+l", "ctrl+n", "ctrl+o", "ctrl+p", "ctrl+q",
		"ctrl+r", "ctrl+s", "ctrl+t", "ctrl+u", "ctrl+v",
		"ctrl+w", "ctrl+x", "ctrl+y", "ctrl+z",
	}

	for _, str := range ctrlStrings {
		if _, ok := KeyDefs[str]; !ok {
			t.Errorf("KeyDefs missing ctrl key string: %q", str)
		}
	}
}
