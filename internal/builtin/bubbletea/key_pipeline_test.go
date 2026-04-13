package bubbletea

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resolveKeyJS replicates the defensive key resolution logic from
// super_document_script.js handleKeys():
//
//	const k = (msg.text && msg.text.length === 1
//	    && msg.text.charCodeAt(0) >= 0x20
//	    && msg.text.charCodeAt(0) <= 0x7E)
//	    ? msg.text : msg.key;
func resolveKeyJS(jsMsg map[string]any) string {
	text, _ := jsMsg["text"].(string)
	key, _ := jsMsg["key"].(string)

	runes := []rune(text)
	if len(runes) == 1 && runes[0] >= 0x20 && runes[0] <= 0x7E {
		return text
	}
	return key
}

// TestKeyPipeline_FullChain verifies the complete key pipeline:
//
//	tea.KeyPressMsg → msgToJS → JS key resolution → ValidateTextareaInput
//
// This is the definitive proof that every key type reaches the JS layer
// correctly and is resolvable by the handleKeys dispatch logic.
func TestKeyPipeline_FullChain(t *testing.T) {
	model := &jsModel{}

	type testCase struct {
		name string
		msg  tea.KeyPressMsg

		// Expected msgToJS outputs.
		wantType string // always "Key" for KeyPressMsg
		wantKey  string // msg.String() — named form (e.g. "enter", "space")
		wantText string // Key.Text — "" for special keys, char for printables

		// Expected resolved k (the value handleKeys dispatches on).
		wantK string

		// Whether ValidateTextareaInput(k) should accept this key.
		wantValid bool
	}

	tests := []testCase{
		// ── Special keys: Text must be empty ────────────────────────────
		{
			name:      "Enter",
			msg:       tea.KeyPressMsg{Code: tea.KeyEnter},
			wantType:  "Key",
			wantKey:   "enter",
			wantText:  "",
			wantK:     "enter",
			wantValid: true,
		},
		{
			name:      "Tab",
			msg:       tea.KeyPressMsg{Code: tea.KeyTab},
			wantType:  "Key",
			wantKey:   "tab",
			wantText:  "",
			wantK:     "tab",
			wantValid: true,
		},
		{
			name:      "Escape",
			msg:       tea.KeyPressMsg{Code: tea.KeyEscape},
			wantType:  "Key",
			wantKey:   "esc",
			wantText:  "",
			wantK:     "esc",
			wantValid: true,
		},
		{
			name:      "Backspace",
			msg:       tea.KeyPressMsg{Code: tea.KeyBackspace},
			wantType:  "Key",
			wantKey:   "backspace",
			wantText:  "",
			wantK:     "backspace",
			wantValid: true,
		},
		{
			name:      "Delete",
			msg:       tea.KeyPressMsg{Code: tea.KeyDelete},
			wantType:  "Key",
			wantKey:   "delete",
			wantText:  "",
			wantK:     "delete",
			wantValid: true,
		},
		{
			name:      "Up arrow",
			msg:       tea.KeyPressMsg{Code: tea.KeyUp},
			wantType:  "Key",
			wantKey:   "up",
			wantText:  "",
			wantK:     "up",
			wantValid: true,
		},
		{
			name:      "Down arrow",
			msg:       tea.KeyPressMsg{Code: tea.KeyDown},
			wantType:  "Key",
			wantKey:   "down",
			wantText:  "",
			wantK:     "down",
			wantValid: true,
		},
		{
			name:      "Left arrow",
			msg:       tea.KeyPressMsg{Code: tea.KeyLeft},
			wantType:  "Key",
			wantKey:   "left",
			wantText:  "",
			wantK:     "left",
			wantValid: true,
		},
		{
			name:      "Right arrow",
			msg:       tea.KeyPressMsg{Code: tea.KeyRight},
			wantType:  "Key",
			wantKey:   "right",
			wantText:  "",
			wantK:     "right",
			wantValid: true,
		},
		{
			name:      "Home",
			msg:       tea.KeyPressMsg{Code: tea.KeyHome},
			wantType:  "Key",
			wantKey:   "home",
			wantText:  "",
			wantK:     "home",
			wantValid: true,
		},
		{
			name:      "End",
			msg:       tea.KeyPressMsg{Code: tea.KeyEnd},
			wantType:  "Key",
			wantKey:   "end",
			wantText:  "",
			wantK:     "end",
			wantValid: true,
		},
		{
			name:      "Page Up",
			msg:       tea.KeyPressMsg{Code: tea.KeyPgUp},
			wantType:  "Key",
			wantKey:   "pgup",
			wantText:  "",
			wantK:     "pgup",
			wantValid: true,
		},
		{
			name:      "Page Down",
			msg:       tea.KeyPressMsg{Code: tea.KeyPgDown},
			wantType:  "Key",
			wantKey:   "pgdown",
			wantText:  "",
			wantK:     "pgdown",
			wantValid: true,
		},

		// ── Space: special — has named key "space" but also Text=" " ───
		{
			name:      "Space",
			msg:       tea.KeyPressMsg{Code: tea.KeySpace, Text: " "},
			wantType:  "Key",
			wantKey:   "space",
			wantText:  " ",
			wantK:     " ", // printable guard picks up the literal space
			wantValid: true,
		},

		// ── Printable ASCII: Text populated with the character ─────────
		{
			name:      "Lowercase a",
			msg:       tea.KeyPressMsg{Text: "a"},
			wantType:  "Key",
			wantKey:   "a",
			wantText:  "a",
			wantK:     "a",
			wantValid: true,
		},
		{
			name:      "Uppercase Z",
			msg:       tea.KeyPressMsg{Text: "Z", ShiftedCode: 'Z', Code: 'z'},
			wantType:  "Key",
			wantKey:   "Z",
			wantText:  "Z",
			wantK:     "Z",
			wantValid: true,
		},
		{
			name:      "Digit 5",
			msg:       tea.KeyPressMsg{Text: "5"},
			wantType:  "Key",
			wantKey:   "5",
			wantText:  "5",
			wantK:     "5",
			wantValid: true,
		},

		// ── Modifier combos: Text empty, key = "ctrl+c" etc. ──────────
		{
			name:      "Ctrl+C",
			msg:       tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl},
			wantType:  "Key",
			wantKey:   "ctrl+c",
			wantText:  "",
			wantK:     "ctrl+c",
			wantValid: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Step 1: msgToJS conversion.
			jsMsg := model.msgToJS(tc.msg)
			require.NotNil(t, jsMsg, "msgToJS must return non-nil for KeyPressMsg")

			// Verify JS object shape.
			assert.Equal(t, tc.wantType, jsMsg["type"], "type field")
			assert.Equal(t, tc.wantKey, jsMsg["key"], "key field")
			assert.Equal(t, tc.wantText, jsMsg["text"], "text field")

			// Step 2: JS key resolution (Go replication of handleKeys logic).
			k := resolveKeyJS(jsMsg)
			assert.Equal(t, tc.wantK, k, "resolved k")

			// Step 3: Validate the resolved key is acceptable for textarea.
			result := ValidateTextareaInput(k)
			assert.Equal(t, tc.wantValid, result.Valid,
				"ValidateTextareaInput(%q): expected valid=%v, got valid=%v reason=%q",
				k, tc.wantValid, result.Valid, result.Reason)
		})
	}
}

// TestKeyPipeline_SpecialKeysHaveEmptyText is a focused regression guard
// against the autopsy's incorrect assumption that Enter/Tab would have
// non-empty Key.Text. BubbleTea v2 documentation explicitly states:
//
//	"Key.Text will be empty for special keys like KeyEnter, KeyTab"
//
// If this test ever fails, the JS printable-ASCII guard in handleKeys()
// will protect against incorrect key resolution, but the failure itself
// should be investigated as a BubbleTea v2 behavioral change.
func TestKeyPipeline_SpecialKeysHaveEmptyText(t *testing.T) {
	model := &jsModel{}

	specialKeys := []struct {
		name string
		msg  tea.KeyPressMsg
	}{
		{"Enter", tea.KeyPressMsg{Code: tea.KeyEnter}},
		{"Tab", tea.KeyPressMsg{Code: tea.KeyTab}},
		{"Escape", tea.KeyPressMsg{Code: tea.KeyEscape}},
		{"Backspace", tea.KeyPressMsg{Code: tea.KeyBackspace}},
		{"Delete", tea.KeyPressMsg{Code: tea.KeyDelete}},
		{"Up", tea.KeyPressMsg{Code: tea.KeyUp}},
		{"Down", tea.KeyPressMsg{Code: tea.KeyDown}},
		{"Left", tea.KeyPressMsg{Code: tea.KeyLeft}},
		{"Right", tea.KeyPressMsg{Code: tea.KeyRight}},
		{"Home", tea.KeyPressMsg{Code: tea.KeyHome}},
		{"End", tea.KeyPressMsg{Code: tea.KeyEnd}},
		{"PgUp", tea.KeyPressMsg{Code: tea.KeyPgUp}},
		{"PgDown", tea.KeyPressMsg{Code: tea.KeyPgDown}},
	}

	for _, sk := range specialKeys {
		t.Run(sk.name, func(t *testing.T) {
			jsMsg := model.msgToJS(sk.msg)
			require.NotNil(t, jsMsg)
			text, _ := jsMsg["text"].(string)
			assert.Empty(t, text,
				"special key %s must have empty text (got %q); if non-empty, the printable-ASCII guard in handleKeys() should prevent incorrect resolution, but this change should be investigated",
				sk.name, text)
		})
	}
}

// TestKeyPipeline_PrintableGuardRejectsControlChars verifies that if
// BubbleTea ever populates Key.Text with control characters for special
// keys (e.g. "\r" for Enter, "\t" for Tab), the printable-ASCII guard
// will correctly fall through to msg.key instead of using the control char.
//
// This is a forward-looking test — currently BubbleTea sets Text="" for
// these keys, but the guard must work even if that changes.
func TestKeyPipeline_PrintableGuardRejectsControlChars(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		key      string
		expected string
	}{
		{"\\r for Enter", "\r", "enter", "enter"},
		{"\\t for Tab", "\t", "tab", "tab"},
		{"\\n for newline", "\n", "enter", "enter"},
		{"\\x1b for Escape", "\x1b", "esc", "esc"},
		{"\\x7f for Backspace", "\x7f", "backspace", "backspace"},
		{"\\x00 for null", "\x00", "ctrl+@", "ctrl+@"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate a hypothetical future msgToJS output where Text is
			// populated with the control character.
			jsMsg := map[string]any{
				"type": "Key",
				"key":  tc.key,
				"text": tc.text,
			}
			k := resolveKeyJS(jsMsg)
			assert.Equal(t, tc.expected, k,
				"printable guard must reject control char %q and fall through to key %q",
				tc.text, tc.key)
		})
	}
}

// TestKeyPipeline_SpaceUsesTextNotNamedKey verifies that the space bar
// resolves to the literal " " character (from msg.text) rather than the
// named key "space" (from msg.key). This matters because textarea
// validation accepts " " as printable ASCII but the named "space" is also
// valid, so either would work — but the JS code must produce " " to be
// consistent with the printable-char branch.
func TestKeyPipeline_SpaceUsesTextNotNamedKey(t *testing.T) {
	model := &jsModel{}

	spaceMsg := tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	jsMsg := model.msgToJS(spaceMsg)
	require.NotNil(t, jsMsg)

	k := resolveKeyJS(jsMsg)
	assert.Equal(t, " ", k, "space must resolve to literal ' ' via text, not 'space' via key")

	// Both must be valid for textarea.
	assert.True(t, ValidateTextareaInput(" ").Valid, "literal space must be valid")
	assert.True(t, ValidateTextareaInput("space").Valid, "named 'space' must also be valid")
}

// TestKeyPipeline_ParseKeyRoundTrip verifies that the critical round-trip
// through JsToTeaMsg/ParseKey correctly reconstructs a tea.KeyPressMsg
// whose .String() matches the key binding strings in bubbles textarea.
//
// This is the definitive test for the arrow key bug: if ParseKey("up")
// produces a message whose .String() != "up", the textarea's key.Matches
// comparisons will fail silently and arrow keys won't move the cursor.
func TestKeyPipeline_ParseKeyRoundTrip(t *testing.T) {
	model := &jsModel{}

	tests := []struct {
		name         string
		original     tea.KeyPressMsg
		wantString   string // what .String() must return for textarea key.Matches
		wantCode     rune
		wantCodeName string // for error messages
	}{
		{"up", tea.KeyPressMsg{Code: tea.KeyUp}, "up", tea.KeyUp, "KeyUp"},
		{"down", tea.KeyPressMsg{Code: tea.KeyDown}, "down", tea.KeyDown, "KeyDown"},
		{"left", tea.KeyPressMsg{Code: tea.KeyLeft}, "left", tea.KeyLeft, "KeyLeft"},
		{"right", tea.KeyPressMsg{Code: tea.KeyRight}, "right", tea.KeyRight, "KeyRight"},
		{"home", tea.KeyPressMsg{Code: tea.KeyHome}, "home", tea.KeyHome, "KeyHome"},
		{"end", tea.KeyPressMsg{Code: tea.KeyEnd}, "end", tea.KeyEnd, "KeyEnd"},
		{"pgup", tea.KeyPressMsg{Code: tea.KeyPgUp}, "pgup", tea.KeyPgUp, "KeyPgUp"},
		{"pgdown", tea.KeyPressMsg{Code: tea.KeyPgDown}, "pgdown", tea.KeyPgDown, "KeyPgDown"},
		{"enter", tea.KeyPressMsg{Code: tea.KeyEnter}, "enter", tea.KeyEnter, "KeyEnter"},
		{"tab", tea.KeyPressMsg{Code: tea.KeyTab}, "tab", tea.KeyTab, "KeyTab"},
		{"backspace", tea.KeyPressMsg{Code: tea.KeyBackspace}, "backspace", tea.KeyBackspace, "KeyBackspace"},
		{"delete", tea.KeyPressMsg{Code: tea.KeyDelete}, "delete", tea.KeyDelete, "KeyDelete"},
		{"esc", tea.KeyPressMsg{Code: tea.KeyEscape}, "esc", tea.KeyEscape, "KeyEscape"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Step 1: Original message → msgToJS (Go → JS)
			jsMsg := model.msgToJS(tc.original)
			require.NotNil(t, jsMsg)
			keyStr, _ := jsMsg["key"].(string)
			assert.Equal(t, tc.wantString, keyStr, "msgToJS key")

			// Step 2: ParseKey round-trip (JS key string → Go message)
			reconstructed, ok := ParseKey(keyStr)
			require.True(t, ok, "ParseKey(%q) must succeed", keyStr)

			// Step 3: Verify Code matches the original constant
			assert.Equal(t, tc.wantCode, reconstructed.Code,
				"ParseKey(%q).Code = %d (%U), want %s = %d (%U)",
				keyStr, reconstructed.Code, reconstructed.Code,
				tc.wantCodeName, tc.wantCode, tc.wantCode)

			// Step 4: The critical test — does .String() match the original?
			// The textarea uses key.Matches which compares .String() against
			// the binding's configured keys (e.g., ["up", "ctrl+p"]).
			// If this fails, the textarea silently drops the key.
			assert.Equal(t, tc.wantString, reconstructed.String(),
				"round-trip .String() must match for textarea key.Matches to work; "+
					"Code=%d (%U), Text=%q",
				reconstructed.Code, reconstructed.Code, reconstructed.Text)

			// Step 5: Verify it matches the original message's .String()
			assert.Equal(t, tc.original.String(), reconstructed.String(),
				"round-trip .String() must match original .String()")
		})
	}
}
