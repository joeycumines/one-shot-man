package bubbletea

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

var parseKeyTests = []struct {
	name                  string
	input                 string
	wantKeyType           tea.KeyType
	wantRunes             []rune
	wantAlt               bool
	wantPaste             bool
	wantAmbiguous         bool
	wantNotMatchingString bool
}{
	{
		name:        "shift+tab returns KeyShiftTab",
		input:       "shift+tab",
		wantKeyType: tea.KeyShiftTab,
	},
	{
		name:        "tab returns KeyTab",
		input:       "tab",
		wantKeyType: tea.KeyTab,
	},
	{
		name:        "enter returns KeyEnter",
		input:       "enter",
		wantKeyType: tea.KeyEnter,
	},
	{
		name:          "space word returns KeyRunes ambiguous",
		input:         "space",
		wantKeyType:   tea.KeyRunes,
		wantRunes:     []rune("space"),
		wantAmbiguous: true,
	},
	{
		name:        "literal space returns KeySpace",
		input:       " ",
		wantKeyType: tea.KeySpace,
		wantRunes:   []rune{' '},
	},

	{
		name:                  "empty string returns zero value",
		input:                 "",
		wantKeyType:           0,    // KeyNull/Zero
		wantAmbiguous:         true, // !ok because input is empty
		wantNotMatchingString: true,
	},
	{
		name:        "ctrl+@ returns KeyNull",
		input:       "ctrl+@",
		wantKeyType: tea.KeyNull,
	},

	{
		name:          "alt+ prefix only (invalid named key)",
		input:         "alt+",
		wantKeyType:   tea.KeyRunes,
		wantRunes:     []rune{},
		wantAlt:       true,
		wantAmbiguous: true, // Runes is empty (len 0), string is empty.
	},
	{
		name:        "alt+enter returns KeyEnter with Alt",
		input:       "alt+enter",
		wantKeyType: tea.KeyEnter,
		wantAlt:     true,
	},
	{
		name:        "alt+a returns KeyRunes 'a' with Alt",
		input:       "alt+a",
		wantKeyType: tea.KeyRunes,
		wantRunes:   []rune{'a'},
		wantAlt:     true,
	},
	{
		name:        "alt+space returns KeySpace with Alt",
		input:       "alt+ ",
		wantKeyType: tea.KeySpace,
		wantAlt:     true,
		wantRunes:   []rune{' '},
	},
	{
		name:          "alt+alt+ double prefix",
		input:         "alt+alt+",
		wantKeyType:   tea.KeyRunes,
		wantRunes:     []rune("alt+"),
		wantAlt:       true, // First alt+ stripped, second remains as runes
		wantAmbiguous: true,
	},

	{
		name:        "paste [a]",
		input:       "[a]",
		wantKeyType: tea.KeyRunes,
		wantRunes:   []rune{'a'},
		wantPaste:   true,
	},
	{
		name: "paste empty content [] (ignored as paste, fallback to runes)",
		// Logic: len(s) > 2 check fails for "[]".
		input:         "[]",
		wantKeyType:   tea.KeyRunes,
		wantRunes:     []rune{'[', ']'},
		wantAmbiguous: true,
	},
	{
		name:        "paste [ ] with space",
		input:       "[ ]",
		wantKeyType: tea.KeyRunes,
		wantRunes:   []rune{' '},
		wantPaste:   true,
	},
	{
		name:        "paste [abc] multichar",
		input:       "[abc]",
		wantKeyType: tea.KeyRunes,
		wantRunes:   []rune("abc"),
		wantPaste:   true,
	},
	{
		name: "paste with alt prefix alt+[a]",
		// Logic: Strip "alt+", s="[a]". Check paste on "[a]" -> True.
		input:       "alt+[a]",
		wantKeyType: tea.KeyRunes,
		wantRunes:   []rune{'a'},
		wantAlt:     true,
		wantPaste:   true,
	},
	{
		name:        "nested brackets [[a]]",
		input:       "[[a]]",
		wantKeyType: tea.KeyRunes,
		wantRunes:   []rune("[a]"),
		wantPaste:   true,
	},
	{
		name:          "malformed paste [a (missing closing)",
		input:         "[a",
		wantKeyType:   tea.KeyRunes,
		wantRunes:     []rune("[a"),
		wantAmbiguous: true,
	},

	// --- Named Keys (Exhaustive Spot Checks) ---
	{
		name:        "ctrl+c (KeyBreak/CtrlC)",
		input:       "ctrl+c",
		wantKeyType: tea.KeyBreak, // Aliased in map
	},
	{
		name:        "esc (KeyEsc)",
		input:       "esc",
		wantKeyType: tea.KeyEsc,
	},
	{
		name:        "backspace",
		input:       "backspace",
		wantKeyType: tea.KeyBackspace,
	},
	{
		name:        "ctrl+@ (KeyNull)",
		input:       "ctrl+@",
		wantKeyType: tea.KeyNull,
	},

	{
		name:        "single rune unambiguous",
		input:       "x",
		wantKeyType: tea.KeyRunes,
		wantRunes:   []rune{'x'},
	},
	{
		name:          "multi-rune sequence ambiguous",
		input:         "xyz",
		wantKeyType:   tea.KeyRunes,
		wantRunes:     []rune("xyz"),
		wantAmbiguous: true,
	},
	{
		name:        "unicode single char (beta)",
		input:       "β",
		wantKeyType: tea.KeyRunes,
		wantRunes:   []rune{'β'},
	},
	{
		name: "emoji single width-2 char (clown)",
		// Logic: len(s) > 1 (bytes), len(runes) == 1. Unambiguous.
		input:       "🤡",
		wantKeyType: tea.KeyRunes,
		wantRunes:   []rune{'🤡'},
	},
	{
		name: "grapheme cluster (e + acute accent)",
		// Logic: 2 runes, but uniseg.StringWidth is 1. Should be Unambiguous (ok=true).
		input:         "e\u0301",
		wantKeyType:   tea.KeyRunes,
		wantRunes:     []rune{'e', '\u0301'},
		wantAmbiguous: false,
	},
	{
		name: "ZWJ sequence (Family)",
		// Logic: Multiple runes, Width 2. Should be Ambiguous.
		// 👨 + ZWJ + 👩 + ZWJ + 👧 + ZWJ + 👦
		input:         "\U0001f468\u200d\U0001f469\u200d\U0001f467\u200d\U0001f466",
		wantKeyType:   tea.KeyRunes,
		wantRunes:     []rune("\U0001f468\u200d\U0001f469\u200d\U0001f467\u200d\U0001f466"),
		wantAmbiguous: true, // Width > 1, Runes > 1, len > 1
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

			// 2. Check KeyType
			if k.Type != tt.wantKeyType {
				t.Errorf("ParseKey(%q) Type = %v, want %v", tt.input, k.Type, tt.wantKeyType)
			}

			// 3. Check Modifiers
			if k.Alt != tt.wantAlt {
				t.Errorf("ParseKey(%q) Alt = %v, want %v", tt.input, k.Alt, tt.wantAlt)
			}
			if k.Paste != tt.wantPaste {
				t.Errorf("ParseKey(%q) Paste = %v, want %v", tt.input, k.Paste, tt.wantPaste)
			}

			// 4. Check Runes
			if len(k.Runes) != len(tt.wantRunes) {
				t.Errorf("ParseKey(%q) runes length = %d, want %d", tt.input, len(k.Runes), len(tt.wantRunes))
			} else {
				for i, r := range tt.wantRunes {
					if k.Runes[i] != r {
						t.Errorf("ParseKey(%q) runes[%d] = %q, want %q", tt.input, i, k.Runes[i], r)
					}
				}
			}

			// 5. Check String Match (k.String())
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
			// If it's unambiguous, it must either be a named key found in map
			// OR a single rune/cluster.
			isNamed := k.Type != tea.KeyRunes

			// Note: KeySpace (0) is a specific type, so we exclude it from "zero value" check logic
			// if we were checking k.Type == 0.

			if !isNamed && len(k.Runes) == 0 {
				// Ambiguous should be false if we have no runes and no type (unless input was empty, handled by first check)
				if input != "" {
					t.Errorf("ParseKey(%q) returned unambiguous but had no Type and no Runes", input)
				}
			}
		}

		// Property 3: Re-parsing contract (Sanity Check).
		// Note: We cannot strictly enforce ParseKey(k.String()) == k because k.String()
		// normalizes output (e.g. keyNUL -> "ctrl+@").
		// However, we can verify that the output Key struct is not "impossible" (e.g. Paste=true but Runes=nil).
		if k.Paste && len(k.Runes) == 0 {
			// Implementation Detail: ParseKey sets Runes to s[1:len-1].
			// If input is "[]", ParseKey returns k.Runes=[]rune{ '[', ']' } and Paste=false (ambiguous).
			// If input is "[ ]", Runes=[' '].
			// It should represent physically possible keys.
			t.Errorf("ParseKey(%q) returned Paste=true but empty Runes", input)
		}
	})
}
