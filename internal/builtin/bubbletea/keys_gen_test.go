package bubbletea

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestKeyDefsStringParity verifies that generated KeyDefs have String values
// that match the actual tea.KeyType.String() output.
func TestKeyDefsStringParity(t *testing.T) {
	for stringVal, keyDef := range KeyDefs {
		t.Run(keyDef.Name, func(t *testing.T) {
			// The key in the map should match the String field
			if stringVal != keyDef.String {
				t.Errorf("map key %q doesn't match keyDef.String %q", stringVal, keyDef.String)
			}

			// The Type's String() should match the String field
			actualString := keyDef.Type.String()
			if actualString != keyDef.String {
				t.Errorf("keyDef.Type.String() = %q, want %q", actualString, keyDef.String)
			}
		})
	}
}

// TestKeyDefsByNameParity verifies that KeyDefsByName correctly references KeyDefs.
// Note: Multiple constant names may map to the same KeyDef (e.g., KeyEsc and KeyEscape).
// In such cases, the KeyDef.Name field contains the canonical name, not the alias.
func TestKeyDefsByNameParity(t *testing.T) {
	// Define known aliases that map to canonical names
	canonicalNames := map[string]string{
		"KeyEscape":           "KeyEsc",
		"KeyCtrlOpenBracket":  "KeyEsc",
		"KeyCtrlM":            "KeyEnter",
		"KeyCtrlQuestionMark": "KeyBackspace",
		"KeyCtrlI":            "KeyTab",
		"KeyNull":             "KeyCtrlAt",
		"KeyCtrlC":            "KeyBreak", // KeyCtrlC and KeyBreak both map to "ctrl+c"
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

// TestAllKeyTypesContainsCoreKeys verifies that essential keys are present.
func TestAllKeyTypesContainsCoreKeys(t *testing.T) {
	essentialKeys := []tea.KeyType{
		tea.KeyEnter,
		tea.KeyEscape,
		tea.KeyBackspace,
		tea.KeyUp,
		tea.KeyDown,
		tea.KeyLeft,
		tea.KeyRight,
		tea.KeyHome,
		tea.KeyEnd,
		tea.KeyPgUp,
		tea.KeyPgDown,
		tea.KeyCtrlC,
		tea.KeyRunes,
	}

	keySet := make(map[tea.KeyType]bool)
	for _, kt := range AllKeyTypes {
		keySet[kt] = true
	}

	for _, kt := range essentialKeys {
		if !keySet[kt] {
			t.Errorf("AllKeyTypes missing essential key: %v (string: %q)", kt, kt.String())
		}
	}
}

// TestKeyDefsCoversAllKeyTypes verifies that KeyDefs has an entry for each key in AllKeyTypes.
func TestKeyDefsCoversAllKeyTypes(t *testing.T) {
	for _, kt := range AllKeyTypes {
		stringVal := kt.String()
		if _, ok := KeyDefs[stringVal]; !ok {
			t.Errorf("KeyDefs missing entry for %q (type: %v)", stringVal, kt)
		}
	}
}

// TestFunctionKeysCovered verifies F1-F20 function keys are present.
func TestFunctionKeysCovered(t *testing.T) {
	functionKeys := []tea.KeyType{
		tea.KeyF1, tea.KeyF2, tea.KeyF3, tea.KeyF4, tea.KeyF5,
		tea.KeyF6, tea.KeyF7, tea.KeyF8, tea.KeyF9, tea.KeyF10,
		tea.KeyF11, tea.KeyF12, tea.KeyF13, tea.KeyF14, tea.KeyF15,
		tea.KeyF16, tea.KeyF17, tea.KeyF18, tea.KeyF19, tea.KeyF20,
	}

	for _, kt := range functionKeys {
		stringVal := kt.String()
		if _, ok := KeyDefs[stringVal]; !ok {
			t.Errorf("KeyDefs missing function key: %q", stringVal)
		}
	}
}

// TestCtrlKeysCovered verifies ctrl+letter keys are present.
func TestCtrlKeysCovered(t *testing.T) {
	ctrlKeys := []tea.KeyType{
		tea.KeyCtrlA, tea.KeyCtrlB, tea.KeyCtrlC, tea.KeyCtrlD, tea.KeyCtrlE,
		tea.KeyCtrlF, tea.KeyCtrlG, tea.KeyCtrlH, tea.KeyCtrlI, tea.KeyCtrlJ,
		tea.KeyCtrlK, tea.KeyCtrlL, tea.KeyCtrlN, // KeyCtrlM is Enter
		tea.KeyCtrlO, tea.KeyCtrlP, tea.KeyCtrlQ, tea.KeyCtrlR, tea.KeyCtrlS,
		tea.KeyCtrlT, tea.KeyCtrlU, tea.KeyCtrlV, tea.KeyCtrlW, tea.KeyCtrlX,
		tea.KeyCtrlY, tea.KeyCtrlZ,
	}

	for _, kt := range ctrlKeys {
		stringVal := kt.String()
		if _, ok := KeyDefs[stringVal]; !ok {
			t.Errorf("KeyDefs missing ctrl key: %q", stringVal)
		}
	}
}
