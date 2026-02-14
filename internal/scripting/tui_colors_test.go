package scripting

import (
	"testing"

	"github.com/joeycumines/go-prompt"
)

func TestParseColor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  prompt.Color
	}{
		{"black", prompt.Black},
		{"Black", prompt.Black},
		{"BLACK", prompt.Black},
		{"red", prompt.Red},
		{"green", prompt.Green},
		{"yellow", prompt.Yellow},
		{"blue", prompt.Blue},
		{"white", prompt.White},
		{"darkred", prompt.DarkRed},
		{"darkgreen", prompt.DarkGreen},
		{"brown", prompt.Brown},
		{"darkblue", prompt.DarkBlue},
		{"purple", prompt.Purple},
		{"cyan", prompt.Cyan},
		{"lightgray", prompt.LightGray},
		{"darkgray", prompt.DarkGray},
		{"fuchsia", prompt.Fuchsia},
		{"turquoise", prompt.Turquoise},
		// Bug #2 fix: "default" must map to DefaultColor
		{"default", prompt.DefaultColor},
		{"Default", prompt.DefaultColor},
		{"DEFAULT", prompt.DefaultColor},
		// Unknown strings now return DefaultColor instead of White
		{"unknown", prompt.DefaultColor},
		{"", prompt.DefaultColor},
		{"nonexistent", prompt.DefaultColor},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseColor(tt.input)
			if got != tt.want {
				t.Errorf("parseColor(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
