package bubbletea

import (
	"testing"
)

func TestValidateTextareaInput(t *testing.T) {
	tests := []struct {
		name     string
		keyStr   string
		wantOK   bool
		wantDesc string // substring of reason for documentation
	}{
		// Empty input rejected
		{name: "empty non-paste", keyStr: "", wantOK: false},

		// Single printable ASCII
		{name: "lowercase letter", keyStr: "a", wantOK: true},
		{name: "uppercase letter", keyStr: "Z", wantOK: true},
		{name: "digit", keyStr: "5", wantOK: true},
		{name: "space", keyStr: " ", wantOK: true},
		{name: "tilde", keyStr: "~", wantOK: true},
		{name: "exclamation", keyStr: "!", wantOK: true},

		// Unicode printable
		{name: "emoji", keyStr: "🎉", wantOK: true},
		{name: "chinese char", keyStr: "中", wantOK: true},
		{name: "japanese hiragana", keyStr: "あ", wantOK: true},
		{name: "cyrillic", keyStr: "д", wantOK: true},

		// Recognized named keys
		{name: "enter", keyStr: "enter", wantOK: true},
		{name: "backspace", keyStr: "backspace", wantOK: true},
		{name: "delete", keyStr: "delete", wantOK: true},
		{name: "up arrow", keyStr: "up", wantOK: true},
		{name: "down arrow", keyStr: "down", wantOK: true},
		{name: "left arrow", keyStr: "left", wantOK: true},
		{name: "right arrow", keyStr: "right", wantOK: true},
		{name: "home", keyStr: "home", wantOK: true},
		{name: "end", keyStr: "end", wantOK: true},
		{name: "pgup", keyStr: "pgup", wantOK: true},
		{name: "pgdown", keyStr: "pgdown", wantOK: true},
		{name: "ctrl+a", keyStr: "ctrl+a", wantOK: true},
		{name: "ctrl+c", keyStr: "ctrl+c", wantOK: true},
		{name: "ctrl+v", keyStr: "ctrl+v", wantOK: true},
		{name: "ctrl+z", keyStr: "ctrl+z", wantOK: true},
		{name: "ctrl+home", keyStr: "ctrl+home", wantOK: true},
		{name: "ctrl+end", keyStr: "ctrl+end", wantOK: true},
		{name: "shift+left", keyStr: "shift+left", wantOK: true},
		{name: "esc", keyStr: "esc", wantOK: true},

		// GARBAGE - fragmented escape sequences (MUST BE REJECTED)
		{name: "garbage escape frag 1", keyStr: "[<65;33;12M", wantOK: false},
		{name: "garbage escape frag 2", keyStr: "<65;33;12M", wantOK: false},
		{name: "garbage bracket only", keyStr: "[<", wantOK: false},
		{name: "garbage semicolon frag", keyStr: ";12M", wantOK: false},
		{name: "garbage M suffix", keyStr: "12M", wantOK: false},
		{name: "garbage random multi", keyStr: "abc", wantOK: false},
		{name: "garbage CSI", keyStr: "\x1b[A", wantOK: false},

		// Control characters (raw) - REJECTED
		{name: "null char", keyStr: "\x00", wantOK: false},
		{name: "bell char", keyStr: "\x07", wantOK: false},
		{name: "escape char raw", keyStr: "\x1b", wantOK: false},
		{name: "del char", keyStr: "\x7f", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateTextareaInput(tt.keyStr)
			if result.Valid != tt.wantOK {
				t.Errorf("ValidateTextareaInput(%q) = %v (reason: %s), want %v",
					tt.keyStr, result.Valid, result.Reason, tt.wantOK)
			}
		})
	}
}

func TestValidateLabelInput(t *testing.T) {
	tests := []struct {
		name   string
		keyStr string
		wantOK bool
	}{
		// Empty rejected
		{name: "empty", keyStr: "", wantOK: false},

		// Single printable
		{name: "letter", keyStr: "a", wantOK: true},
		{name: "digit", keyStr: "1", wantOK: true},
		{name: "space", keyStr: " ", wantOK: true},
		{name: "hyphen", keyStr: "-", wantOK: true},
		{name: "underscore", keyStr: "_", wantOK: true},

		// Backspace allowed
		{name: "backspace", keyStr: "backspace", wantOK: true},

		// Other named keys REJECTED for labels
		{name: "enter", keyStr: "enter", wantOK: false},
		{name: "delete", keyStr: "delete", wantOK: false},
		{name: "arrows", keyStr: "up", wantOK: false},
		{name: "ctrl keys", keyStr: "ctrl+a", wantOK: false},

		// Garbage REJECTED
		{name: "garbage", keyStr: "[<65;33;12M", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateLabelInput(tt.keyStr)
			if result.Valid != tt.wantOK {
				t.Errorf("ValidateLabelInput(%q) = %v (reason: %s), want %v",
					tt.keyStr, result.Valid, result.Reason, tt.wantOK)
			}
		})
	}
}
