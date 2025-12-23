package textarea

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestParseKeyString_ShiftTab(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantKeyType tea.KeyType
		wantRunes   []rune
	}{
		{
			name:        "shift+tab returns KeyShiftTab",
			input:       "shift+tab",
			wantKeyType: tea.KeyShiftTab,
			wantRunes:   nil,
		},
		{
			name:        "tab returns KeyTab",
			input:       "tab",
			wantKeyType: tea.KeyTab,
			wantRunes:   nil,
		},
		{
			name:        "enter returns KeyEnter",
			input:       "enter",
			wantKeyType: tea.KeyEnter,
			wantRunes:   nil,
		},
		{
			name:        "shift+up returns KeyShiftUp",
			input:       "shift+up",
			wantKeyType: tea.KeyShiftUp,
			wantRunes:   nil,
		},
		{
			name:        "shift+down returns KeyShiftDown",
			input:       "shift+down",
			wantKeyType: tea.KeyShiftDown,
			wantRunes:   nil,
		},
		{
			name:        "shift+left returns KeyShiftLeft",
			input:       "shift+left",
			wantKeyType: tea.KeyShiftLeft,
			wantRunes:   nil,
		},
		{
			name:        "shift+right returns KeyShiftRight",
			input:       "shift+right",
			wantKeyType: tea.KeyShiftRight,
			wantRunes:   nil,
		},
		{
			name:        "shift+home returns KeyShiftHome",
			input:       "shift+home",
			wantKeyType: tea.KeyShiftHome,
			wantRunes:   nil,
		},
		{
			name:        "shift+end returns KeyShiftEnd",
			input:       "shift+end",
			wantKeyType: tea.KeyShiftEnd,
			wantRunes:   nil,
		},
		{
			name:        "space returns KeySpace",
			input:       "space",
			wantKeyType: tea.KeySpace,
			wantRunes:   nil,
		},
		{
			name:        "single character returns KeyRunes",
			input:       "a",
			wantKeyType: tea.KeyRunes,
			wantRunes:   []rune("a"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKeyType, gotRunes := parseKeyString(tt.input)
			if gotKeyType != tt.wantKeyType {
				t.Errorf("parseKeyString(%q) keyType = %v, want %v", tt.input, gotKeyType, tt.wantKeyType)
			}
			if tt.wantRunes == nil && gotRunes != nil {
				t.Errorf("parseKeyString(%q) runes = %v, want nil", tt.input, gotRunes)
			}
			if tt.wantRunes != nil && string(gotRunes) != string(tt.wantRunes) {
				t.Errorf("parseKeyString(%q) runes = %v, want %v", tt.input, gotRunes, tt.wantRunes)
			}
		})
	}
}

func TestParseKeyString_DoesNotInsertLiteralShiftTab(t *testing.T) {
	// Critical regression test: shift+tab MUST NOT be treated as literal runes
	keyType, runes := parseKeyString("shift+tab")

	if keyType == tea.KeyRunes {
		t.Fatal("REGRESSION: shift+tab is being treated as literal runes, should be KeyShiftTab")
	}

	if len(runes) > 0 {
		t.Fatalf("REGRESSION: shift+tab returned runes %q, should return nil runes", string(runes))
	}

	if keyType != tea.KeyShiftTab {
		t.Fatalf("shift+tab returned KeyType %v, want KeyShiftTab", keyType)
	}
}
