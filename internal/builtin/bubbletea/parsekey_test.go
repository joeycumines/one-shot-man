package bubbletea

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

var parseKeyTests = []struct {
	name                  string
	input                 string
	wantCode              rune
	wantText              string
	wantAlt               bool
	wantAmbiguous         bool
	wantNotMatchingString bool
}{
	// === Named Keys ===
	{
		name:     "tab returns KeyTab",
		input:    "tab",
		wantCode: tea.KeyTab,
	},
	{
		name:     "enter returns KeyEnter",
		input:    "enter",
		wantCode: tea.KeyEnter,
	},
	{
		name:     "escape returns KeyEscape",
		input:    "esc",
		wantCode: tea.KeyEscape,
	},
	{
		name:     "backspace",
		input:    "backspace",
		wantCode: tea.KeyBackspace,
	},

	// === Space Key ===
	// "space" (named key) maps to KeyDefs["space"] → Code=' '
	{
		name:                  "space named key",
		input:                 "space",
		wantCode:              ' ',
		wantAmbiguous:         false,
		wantNotMatchingString: false, // String() = "space" == input "space"
	},
	// " " (literal space character) not in KeyDefs, special case handles it
	{
		name:                  "literal space character",
		input:                 " ",
		wantCode:              ' ',
		wantText:              " ",
		wantAmbiguous:         false,
		wantNotMatchingString: true, // String() = "space" != input " "
	},

	// === Empty Input ===
	{
		name:                  "empty string returns zero value",
		input:                 "",
		wantCode:              0,
		wantAmbiguous:         true, // !ok because input is empty
		wantNotMatchingString: true, // String() = "\x00" != ""
	},

	// === Ctrl + Named Keys ===
	// KeyDefs["ctrl+a"] → Code='a' (no Mod needed, the Code already represents ctrl)
	{
		name:     "ctrl+a",
		input:    "ctrl+a",
		wantCode: 'a',
	},
	{
		name:     "ctrl+c",
		input:    "ctrl+c",
		wantCode: 'c',
	},
	// "ctrl+@" in KeyDefs → Code='@'
	{
		name:     "ctrl+@",
		input:    "ctrl+@",
		wantCode: '@',
	},

	// === Ctrl + Symbols ===
	// These are in KeyDefs with their respective codes
	{
		name:     "ctrl+backslash",
		input:    "ctrl+\\",
		wantCode: '\\',
	},
	{
		name:     "ctrl+close_bracket",
		input:    "ctrl+]",
		wantCode: ']',
	},
	{
		name:     "ctrl+caret",
		input:    "ctrl+^",
		wantCode: '^',
	},
	{
		name:     "ctrl+underscore",
		input:    "ctrl+_",
		wantCode: '_',
	},

	// === Shift + Named Keys ===
	{
		name:     "shift+tab",
		input:    "shift+tab",
		wantCode: tea.KeyTab,
	},

	// === Alt + Named Keys ===
	{
		name:     "alt+enter",
		input:    "alt+enter",
		wantCode: tea.KeyEnter,
		wantAlt:  true,
	},
	{
		name:     "alt+tab",
		input:    "alt+tab",
		wantCode: tea.KeyTab,
		wantAlt:  true,
	},
	{
		name:     "alt+escape",
		input:    "alt+esc",
		wantCode: tea.KeyEscape,
		wantAlt:  true,
	},

	// === Alt + Raw Characters ===
	// alt+letter: KeyDefs lookup fails (no "a" entry), step 3 handles it
	// String() returns "a" not "alt+a" in v2
	{
		name:                  "alt+a returns letter a with Alt",
		input:                 "alt+a",
		wantCode:              'a',
		wantAlt:               true,
		wantText:              "a",
		wantNotMatchingString: true, // String() = "a" != "alt+a"
	},
	// alt+space: KeyDefs lookup fails (no " " entry), step 3 handles literal space
	// Input "alt+ " has 6 chars → unambiguous is true because space is single rune
	{
		name:                  "alt+space",
		input:                 "alt+ ",
		wantCode:              ' ',
		wantAlt:               true,
		wantText:              " ",
		wantAmbiguous:         true,
		wantNotMatchingString: true, // String() = "alt+space" != "alt+ "
	},

	// === Bare Modifier Prefix ===
	// "alt+" alone → empty remainder → invalid → unambiguous=false
	{
		name:                  "alt+ prefix only",
		input:                 "alt+",
		wantCode:              0,
		wantAlt:               true,
		wantAmbiguous:         true, // !ok = unambiguous=false
		wantNotMatchingString: true, // String() = "alt+\x00" != "alt+"
	},

	// === Double Modifier Prefix ===
	// "alt+alt+" → first alt+ stripped → "alt+" → alt+ stripped → "" → invalid
	{
		name:                  "alt+alt+ double prefix",
		input:                 "alt+alt+",
		wantCode:              0,
		wantAlt:               true,
		wantAmbiguous:         true, // !ok = unambiguous=false
		wantNotMatchingString: true, // String() = "alt+" != "alt+alt+"
	},

	// === Bracket Characters (plain text in v2) ===
	// v2 doesn't have bracket paste syntax — brackets are plain text
	// Multi-char text is ambiguous (could be IME, etc.)
	{
		name:          "bracket chars as plain text",
		input:         "[a]",
		wantCode:      0,
		wantText:      "[a]",
		wantAmbiguous: true,
	},
	{
		name:          "empty brackets as plain text",
		input:         "[]",
		wantCode:      0,
		wantText:      "[]",
		wantAmbiguous: true,
	},
	{
		name:          "bracket with space as plain text",
		input:         "[ ]",
		wantCode:      0,
		wantText:      "[ ]",
		wantAmbiguous: true,
	},
	{
		name:                  "alt+bracket",
		input:                 "alt+[a]",
		wantCode:              0,
		wantText:              "[a]",
		wantAlt:               true,
		wantAmbiguous:         true,
		wantNotMatchingString: true, // String() = "[a]" != "alt+[a]"
	},
	{
		name:          "nested brackets as plain text",
		input:         "[[a]]",
		wantCode:      0,
		wantText:      "[[a]]",
		wantAmbiguous: true,
	},
	{
		name:          "malformed bracket (missing close)",
		input:         "[a",
		wantCode:      0,
		wantText:      "[a",
		wantAmbiguous: true, // multi-char is ambiguous
	},

	// === Raw Rune Fallback ===
	{
		name:     "single rune unambiguous",
		input:    "x",
		wantCode: 0,
		wantText: "x",
	},
	{
		name:          "multi-rune sequence ambiguous",
		input:         "xyz",
		wantCode:      0,
		wantText:      "xyz",
		wantAmbiguous: true,
	},
	{
		name:     "unicode single char (beta)",
		input:    "β",
		wantCode: 0,
		wantText: "β",
	},
	// Emoji is multi-byte but single rune
	{
		name:     "emoji single rune (clown)",
		input:    "🤡",
		wantCode: 0,
		wantText: "🤡",
	},
	// Precomposed emoji
	{
		name:     "precomposed emoji",
		input:    "🎉",
		wantCode: 0,
		wantText: "🎉",
	},
	// Precomposed é is single rune
	{
		name:     "grapheme cluster (precomposed é)",
		input:    "é",
		wantCode: 0,
		wantText: "é",
	},
	// ZWJ sequence is multi-rune → ambiguous
	{
		name:          "ZWJ sequence (Family)",
		input:         "\U0001f468\u200d\U0001f469\u200d\U0001f467\u200d\U0001f466",
		wantCode:      0,
		wantText:      "\U0001f468\u200d\U0001f469\u200d\U0001f467\u200d\U0001f466",
		wantAmbiguous: true,
	},
}

func TestParseKey(t *testing.T) {
	for _, tt := range parseKeyTests {
		t.Run(tt.name, func(t *testing.T) {
			k, ok := ParseKey(tt.input)

			// 1. Check Ambiguity (ok). Note: ParseKey returns 'ok' meaning "unambiguous".
			wantUnambiguous := !tt.wantAmbiguous
			if ok != wantUnambiguous {
				t.Errorf("ParseKey(%q) unambiguous = %v, want %v", tt.input, ok, wantUnambiguous)
			}

			// 2. Check Code
			if k.Code != tt.wantCode {
				t.Errorf("ParseKey(%q) Code = %v (%q), want %v (%q)", tt.input, k.Code, string(k.Code), tt.wantCode, string(tt.wantCode))
			}

			// 3. Check Text
			if k.Text != tt.wantText {
				t.Errorf("ParseKey(%q) Text = %q, want %q", tt.input, k.Text, tt.wantText)
			}

			// 4. Check Modifiers (Alt)
			gotAlt := k.Mod&tea.ModAlt != 0
			if gotAlt != tt.wantAlt {
				t.Errorf("ParseKey(%q) Alt = %v, want %v", tt.input, gotAlt, tt.wantAlt)
			}

			// 5. Check String Match
			// wantNotMatchingString=true means we expect k.String() != input
			// wantNotMatchingString=false means we expect k.String() == input
			if s := k.String(); (s == tt.input) == tt.wantNotMatchingString {
				t.Errorf("ParseKey(%q).String() = %q, want match: %v", tt.input, s, !tt.wantNotMatchingString)
			}
		})
	}
}

func FuzzParseKey(f *testing.F) {
	// Add seeds from the table-driven tests
	for _, tt := range parseKeyTests {
		f.Add(tt.input)
	}

	// Add extra edge case seeds
	f.Add("")
	f.Add("alt+")
	f.Add("alt+[")
	f.Add("[ ]")
	f.Add("ctrl+c")

	f.Fuzz(func(t *testing.T, input string) {
		k, unambiguous := ParseKey(input)

		// Property 1: Stability.
		// ParseKey should never panic. (Implicitly checked by fuzz runner).

		// Property 2: Determinism.
		// If unambiguous, verify we got *something* valid.
		if unambiguous {
			isNamed := k.Code != 0

			if !isNamed && len(k.Text) == 0 {
				// Ambiguous should be false if we have no Code and no Text
				if input != "" {
					t.Errorf("ParseKey(%q) returned unambiguous but had no Code and no Text", input)
				}
			}
		}

		// Property 3: Impossible result detection.
		// If input is non-empty, unambiguous, but Code=0 and Text empty → impossible.
		if len(k.Text) == 0 && k.Code == 0 && input != "" && unambiguous {
			t.Errorf("ParseKey(%q) returned zero Code and empty Text but was unambiguous", input)
		}
	})
}
