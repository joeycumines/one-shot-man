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

func TestPromptColors_BackgroundFields(t *testing.T) {
	t.Parallel()

	t.Run("ApplyFromInterfaceMap_sets_background_colors", func(t *testing.T) {
		pc := PromptColors{}
		pc.ApplyFromInterfaceMap(map[string]interface{}{
			"inputBackground":  "red",
			"prefixBackground": "blue",
		})

		if pc.InputBG != prompt.Red {
			t.Errorf("InputBG = %v, want Red", pc.InputBG)
		}
		if pc.PrefixBG != prompt.Blue {
			t.Errorf("PrefixBG = %v, want Blue", pc.PrefixBG)
		}
	})

	t.Run("ApplyFromStringMap_sets_background_colors", func(t *testing.T) {
		pc := PromptColors{}
		pc.ApplyFromStringMap(map[string]string{
			"inputBackground":  "cyan",
			"prefixBackground": "darkgray",
		})

		if pc.InputBG != prompt.Cyan {
			t.Errorf("InputBG = %v, want Cyan", pc.InputBG)
		}
		if pc.PrefixBG != prompt.DarkGray {
			t.Errorf("PrefixBG = %v, want DarkGray", pc.PrefixBG)
		}
	})

	t.Run("zero_value_is_DefaultColor", func(t *testing.T) {
		pc := PromptColors{}
		// All prompt.Color zero values = 0 = DefaultColor
		if pc.InputBG != prompt.DefaultColor {
			t.Errorf("InputBG zero = %v, want DefaultColor", pc.InputBG)
		}
		if pc.PrefixBG != prompt.DefaultColor {
			t.Errorf("PrefixBG zero = %v, want DefaultColor", pc.PrefixBG)
		}
	})
}
