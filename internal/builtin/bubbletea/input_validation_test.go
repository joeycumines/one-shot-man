package bubbletea

import (
	"testing"
)

func TestValidateTextareaInput(t *testing.T) {
	tests := []struct {
		name     string
		keyStr   string
		isPaste  bool
		wantOK   bool
		wantDesc string // substring of reason for documentation
	}{
		// Paste events always pass
		{name: "paste single char", keyStr: "a", isPaste: true, wantOK: true},
		{name: "paste multi-line", keyStr: "[hello\nworld]", isPaste: true, wantOK: true},
		{name: "paste empty", keyStr: "", isPaste: true, wantOK: true},

		// Empty input rejected
		{name: "empty non-paste", keyStr: "", isPaste: false, wantOK: false},

		// Single printable ASCII
		{name: "lowercase letter", keyStr: "a", isPaste: false, wantOK: true},
		{name: "uppercase letter", keyStr: "Z", isPaste: false, wantOK: true},
		{name: "digit", keyStr: "5", isPaste: false, wantOK: true},
		{name: "space", keyStr: " ", isPaste: false, wantOK: true},
		{name: "tilde", keyStr: "~", isPaste: false, wantOK: true},
		{name: "exclamation", keyStr: "!", isPaste: false, wantOK: true},

		// Unicode printable
		{name: "emoji", keyStr: "🎉", isPaste: false, wantOK: true},
		{name: "chinese char", keyStr: "中", isPaste: false, wantOK: true},
		{name: "japanese hiragana", keyStr: "あ", isPaste: false, wantOK: true},
		{name: "cyrillic", keyStr: "д", isPaste: false, wantOK: true},

		// Recognized named keys
		{name: "enter", keyStr: "enter", isPaste: false, wantOK: true},
		{name: "backspace", keyStr: "backspace", isPaste: false, wantOK: true},
		{name: "delete", keyStr: "delete", isPaste: false, wantOK: true},
		{name: "up arrow", keyStr: "up", isPaste: false, wantOK: true},
		{name: "down arrow", keyStr: "down", isPaste: false, wantOK: true},
		{name: "left arrow", keyStr: "left", isPaste: false, wantOK: true},
		{name: "right arrow", keyStr: "right", isPaste: false, wantOK: true},
		{name: "home", keyStr: "home", isPaste: false, wantOK: true},
		{name: "end", keyStr: "end", isPaste: false, wantOK: true},
		{name: "pgup", keyStr: "pgup", isPaste: false, wantOK: true},
		{name: "pgdown", keyStr: "pgdown", isPaste: false, wantOK: true},
		{name: "ctrl+a", keyStr: "ctrl+a", isPaste: false, wantOK: true},
		{name: "ctrl+c", keyStr: "ctrl+c", isPaste: false, wantOK: true},
		{name: "ctrl+v", keyStr: "ctrl+v", isPaste: false, wantOK: true},
		{name: "ctrl+z", keyStr: "ctrl+z", isPaste: false, wantOK: true},
		{name: "ctrl+home", keyStr: "ctrl+home", isPaste: false, wantOK: true},
		{name: "ctrl+end", keyStr: "ctrl+end", isPaste: false, wantOK: true},
		{name: "shift+left", keyStr: "shift+left", isPaste: false, wantOK: true},
		{name: "esc", keyStr: "esc", isPaste: false, wantOK: true},

		// GARBAGE - fragmented escape sequences (MUST BE REJECTED)
		{name: "garbage escape frag 1", keyStr: "[<65;33;12M", isPaste: false, wantOK: false},
		{name: "garbage escape frag 2", keyStr: "<65;33;12M", isPaste: false, wantOK: false},
		{name: "garbage bracket only", keyStr: "[<", isPaste: false, wantOK: false},
		{name: "garbage semicolon frag", keyStr: ";12M", isPaste: false, wantOK: false},
		{name: "garbage M suffix", keyStr: "12M", isPaste: false, wantOK: false},
		{name: "garbage random multi", keyStr: "abc", isPaste: false, wantOK: false},
		{name: "garbage CSI", keyStr: "\x1b[A", isPaste: false, wantOK: false},

		// Control characters (raw) - REJECTED
		{name: "null char", keyStr: "\x00", isPaste: false, wantOK: false},
		{name: "bell char", keyStr: "\x07", isPaste: false, wantOK: false},
		{name: "escape char raw", keyStr: "\x1b", isPaste: false, wantOK: false},
		{name: "del char", keyStr: "\x7f", isPaste: false, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateTextareaInput(tt.keyStr, tt.isPaste)
			if result.Valid != tt.wantOK {
				t.Errorf("ValidateTextareaInput(%q, %v) = %v (reason: %s), want %v",
					tt.keyStr, tt.isPaste, result.Valid, result.Reason, tt.wantOK)
			}
		})
	}
}

func TestValidateLabelInput(t *testing.T) {
	tests := []struct {
		name    string
		keyStr  string
		isPaste bool
		wantOK  bool
	}{
		// Paste events pass through
		{name: "paste text", keyStr: "hello", isPaste: true, wantOK: true},

		// Empty rejected
		{name: "empty", keyStr: "", isPaste: false, wantOK: false},

		// Single printable
		{name: "letter", keyStr: "a", isPaste: false, wantOK: true},
		{name: "digit", keyStr: "1", isPaste: false, wantOK: true},
		{name: "space", keyStr: " ", isPaste: false, wantOK: true},
		{name: "hyphen", keyStr: "-", isPaste: false, wantOK: true},
		{name: "underscore", keyStr: "_", isPaste: false, wantOK: true},

		// Backspace allowed
		{name: "backspace", keyStr: "backspace", isPaste: false, wantOK: true},

		// Other named keys REJECTED for labels
		{name: "enter", keyStr: "enter", isPaste: false, wantOK: false},
		{name: "delete", keyStr: "delete", isPaste: false, wantOK: false},
		{name: "arrows", keyStr: "up", isPaste: false, wantOK: false},
		{name: "ctrl keys", keyStr: "ctrl+a", isPaste: false, wantOK: false},

		// Garbage REJECTED
		{name: "garbage", keyStr: "[<65;33;12M", isPaste: false, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateLabelInput(tt.keyStr, tt.isPaste)
			if result.Valid != tt.wantOK {
				t.Errorf("ValidateLabelInput(%q, %v) = %v (reason: %s), want %v",
					tt.keyStr, tt.isPaste, result.Valid, result.Reason, tt.wantOK)
			}
		})
	}
}
