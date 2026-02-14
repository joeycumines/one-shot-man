package scripting

import (
	"strings"

	"github.com/joeycumines/go-prompt"
)

// Unified helpers to apply color overrides without duplication.
// applyFromGetter reads color overrides using a provided getter function.
func (pc *PromptColors) applyFromGetter(get func(string) (string, bool)) {
	if v, ok := get("input"); ok && v != "" {
		pc.InputText = parseColor(v)
	}
	if v, ok := get("prefix"); ok && v != "" {
		pc.PrefixText = parseColor(v)
	}
	if v, ok := get("suggestionText"); ok && v != "" {
		pc.SuggestionText = parseColor(v)
	}
	if v, ok := get("suggestionBackground"); ok && v != "" {
		pc.SuggestionBG = parseColor(v)
	}
	if v, ok := get("selectedSuggestionText"); ok && v != "" {
		pc.SelectedSuggestionText = parseColor(v)
	}
	if v, ok := get("selectedSuggestionBackground"); ok && v != "" {
		pc.SelectedSuggestionBG = parseColor(v)
	}
	if v, ok := get("descriptionText"); ok && v != "" {
		pc.DescriptionText = parseColor(v)
	}
	if v, ok := get("descriptionBackground"); ok && v != "" {
		pc.DescriptionBG = parseColor(v)
	}
	if v, ok := get("selectedDescriptionText"); ok && v != "" {
		pc.SelectedDescriptionText = parseColor(v)
	}
	if v, ok := get("selectedDescriptionBackground"); ok && v != "" {
		pc.SelectedDescriptionBG = parseColor(v)
	}
	if v, ok := get("scrollbarThumb"); ok && v != "" {
		pc.ScrollbarThumb = parseColor(v)
	}
	if v, ok := get("scrollbarBackground"); ok && v != "" {
		pc.ScrollbarBG = parseColor(v)
	}
}

// ApplyFromInterfaceMap applies overrides where values come from a JS map (map[string]interface{}).
func (pc *PromptColors) ApplyFromInterfaceMap(m map[string]interface{}) {
	if m == nil {
		return
	}
	pc.applyFromGetter(func(k string) (string, bool) {
		if v, ok := m[k]; ok {
			if s, ok2 := v.(string); ok2 {
				return s, true
			}
		}
		return "", false
	})
}

// ApplyFromStringMap applies overrides from a simple string map.
func (pc *PromptColors) ApplyFromStringMap(m map[string]string) {
	if m == nil {
		return
	}
	pc.applyFromGetter(func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	})
}

// SetDefaultColorsFromStrings allows external config to override the default colors
// using a simple map of name->colorString. Supported keys mirror PromptColors
// with the following names: input, prefix, suggestionText, suggestionBackground,
// selectedSuggestionText, selectedSuggestionBackground, descriptionText, descriptionBackground,
// selectedDescriptionText, selectedDescriptionBackground, scrollbarThumb, scrollbarBackground.
func (tm *TUIManager) SetDefaultColorsFromStrings(m map[string]string) {
	if m == nil {
		return
	}
	// start from existing defaults
	c := tm.defaultColors
	c.ApplyFromStringMap(m)
	tm.defaultColors = c
}

// parseColor converts a color string to prompt.Color.
func parseColor(colorStr string) prompt.Color {
	switch strings.ToLower(colorStr) {
	case "black":
		return prompt.Black
	case "darkred":
		return prompt.DarkRed
	case "darkgreen":
		return prompt.DarkGreen
	case "brown":
		return prompt.Brown
	case "darkblue":
		return prompt.DarkBlue
	case "purple":
		return prompt.Purple
	case "cyan":
		return prompt.Cyan
	case "lightgray":
		return prompt.LightGray
	case "darkgray":
		return prompt.DarkGray
	case "red":
		return prompt.Red
	case "green":
		return prompt.Green
	case "yellow":
		return prompt.Yellow
	case "blue":
		return prompt.Blue
	case "fuchsia":
		return prompt.Fuchsia
	case "turquoise":
		return prompt.Turquoise
	case "white":
		return prompt.White
	case "default":
		return prompt.DefaultColor
	default:
		return prompt.DefaultColor
	}
}
