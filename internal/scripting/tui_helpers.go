package scripting

import (
	"fmt"
	"os"
	"strings"

	"github.com/dop251/goja"
	"github.com/elk-language/go-prompt"
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
	if v, ok := get("suggestionBG"); ok && v != "" {
		pc.SuggestionBG = parseColor(v)
	}
	if v, ok := get("selectedSuggestionText"); ok && v != "" {
		pc.SelectedSuggestionText = parseColor(v)
	}
	if v, ok := get("selectedSuggestionBG"); ok && v != "" {
		pc.SelectedSuggestionBG = parseColor(v)
	}
	if v, ok := get("descriptionText"); ok && v != "" {
		pc.DescriptionText = parseColor(v)
	}
	if v, ok := get("descriptionBG"); ok && v != "" {
		pc.DescriptionBG = parseColor(v)
	}
	if v, ok := get("selectedDescriptionText"); ok && v != "" {
		pc.SelectedDescriptionText = parseColor(v)
	}
	if v, ok := get("selectedDescriptionBG"); ok && v != "" {
		pc.SelectedDescriptionBG = parseColor(v)
	}
	if v, ok := get("scrollbarThumb"); ok && v != "" {
		pc.ScrollbarThumb = parseColor(v)
	}
	if v, ok := get("scrollbarBG"); ok && v != "" {
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
// with the following names: input, prefix, suggestionText, suggestionBG,
// selectedSuggestionText, selectedSuggestionBG, descriptionText, descriptionBG,
// selectedDescriptionText, selectedDescriptionBG, scrollbarThumb, scrollbarBG.
func (tm *TUIManager) SetDefaultColorsFromStrings(m map[string]string) {
	if m == nil {
		return
	}
	// start from existing defaults
	c := tm.defaultColors
	c.ApplyFromStringMap(m)
	tm.defaultColors = c
}

// parseHistoryConfig parses history configuration from JavaScript config.
func parseHistoryConfig(configMap map[string]interface{}) HistoryConfig {
	config := HistoryConfig{
		Enabled: false,
		File:    "",
		Size:    1000,
	}

	if historyRaw, exists := configMap["history"]; exists {
		if historyMap, ok := historyRaw.(map[string]interface{}); ok {
			config.Enabled = getBool(historyMap, "enabled", false)
			config.File = getString(historyMap, "file", "")
			config.Size = getInt(historyMap, "size", 1000)
		}
	}

	return config
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
	default:
		return prompt.White
	}
}

// loadHistory loads history from a file.
func loadHistory(filename string) []string {
	if filename == "" {
		return []string{}
	}

	content, err := os.ReadFile(filename)
	if err != nil {
		return []string{}
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	var history []string
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			history = append(history, line)
		}
	}

	return history
}

// getDefaultCompletionSuggestions provides default completion when no custom completer is set.
func (tm *TUIManager) getDefaultCompletionSuggestions(document prompt.Document) []prompt.Suggest {
	var suggestions []prompt.Suggest

	// Get the word being typed
	text := document.TextBeforeCursor()
	// If TextBeforeCursor is empty, fall back to the full text
	if text == "" {
		text = document.Text
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return suggestions
	}

	currentWord := words[len(words)-1]

	// Provide command completion for first word
	if len(words) == 1 {
		// Collect all commands with precedence: mode commands > registered commands > built-in commands
		commandMap := make(map[string]prompt.Suggest)

		// Built-in commands (lowest precedence)
		builtinCommands := []string{"help", "exit", "quit", "mode", "modes", "state"}
		for _, cmd := range builtinCommands {
			if strings.HasPrefix(cmd, currentWord) {
				commandMap[cmd] = prompt.Suggest{
					Text:        cmd,
					Description: "Built-in command",
				}
			}
		}

		// Registered commands (medium precedence)
		tm.mu.RLock()
		for _, cmd := range tm.commands {
			if strings.HasPrefix(cmd.Name, currentWord) {
				commandMap[cmd.Name] = prompt.Suggest{
					Text:        cmd.Name,
					Description: cmd.Description,
				}
			}
		}

		// Current mode commands (highest precedence)
		if tm.currentMode != nil {
			tm.currentMode.mu.RLock()
			for _, cmd := range tm.currentMode.Commands {
				if strings.HasPrefix(cmd.Name, currentWord) {
					commandMap[cmd.Name] = prompt.Suggest{
						Text:        cmd.Name,
						Description: cmd.Description,
					}
				}
			}
			tm.currentMode.mu.RUnlock()
		}
		tm.mu.RUnlock()

		// Convert map to slice
		for _, suggestion := range commandMap {
			suggestions = append(suggestions, suggestion)
		}
	}

	// For mode command, suggest available modes
	if len(words) == 2 && words[0] == "mode" {
		tm.mu.RLock()
		for modeName := range tm.modes {
			if strings.HasPrefix(modeName, currentWord) {
				suggestions = append(suggestions, prompt.Suggest{
					Text:        modeName,
					Description: "Available mode",
				})
			}
		}
		tm.mu.RUnlock()
	}

	return suggestions
}

// Helper: length in runes for a string
func runeLen(s string) int {
	return len([]rune(s))
}

// Helper: rune index at end of the given string (same as rune length)
func runeIndex(s string) int {
	return runeLen(s)
}

// Helper: returns the current word before cursor, splitting on whitespace
func currentWord(before string) string {
	before = strings.ReplaceAll(before, "\n", " ")
	parts := strings.Fields(before)
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

// tryCallJSCompleter attempts to call a JS completer; returns (suggestions, true) on success, otherwise (nil, false)
func (tm *TUIManager) tryCallJSCompleter(callable goja.Callable, document prompt.Document) ([]prompt.Suggest, bool) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(tm.output, "Completer panic: %v\n", r)
		}
	}()

	vm := tm.engine.vm
	// Build a lightweight JS wrapper for the document
	docObj := vm.NewObject()
	_ = docObj.Set("getText", func() string { return document.Text })
	_ = docObj.Set("getTextBeforeCursor", func() string { return document.TextBeforeCursor() })
	_ = docObj.Set("getWordBeforeCursor", func() string { return currentWord(document.TextBeforeCursor()) })

	// Call the JS completer: fn(document)
	value, err := callable(goja.Undefined(), docObj)
	if err != nil {
		fmt.Fprintf(tm.output, "Completer error: %v\n", err)
		return nil, false
	}

	// Convert the result into []prompt.Suggest
	// Support: array of strings OR array of {text, description}
	var out []prompt.Suggest

	if goja.IsUndefined(value) || goja.IsNull(value) {
		return nil, false
	}

	// Try export to []interface{} then map
	var rawArr []interface{}
	if err := vm.ExportTo(value, &rawArr); err != nil {
		// Not an array - bail out
		return nil, false
	}

	for _, item := range rawArr {
		switch v := item.(type) {
		case string:
			out = append(out, prompt.Suggest{Text: v})
		case map[string]interface{}:
			text, _ := v["text"].(string)
			desc, _ := v["description"].(string)
			if text != "" {
				out = append(out, prompt.Suggest{Text: text, Description: desc})
			}
		default:
			// ignore unsupported types
		}
	}

	return out, true
}

// getInt extracts an integer value from a JavaScript object map.
func getInt(m map[string]interface{}, key string, defaultValue int) int {
	if val, exists := m[key]; exists {
		if i, ok := val.(int); ok {
			return i
		}
		if f, ok := val.(float64); ok {
			return int(f)
		}
	}
	return defaultValue
}

// Helper functions for extracting values from JavaScript objects
func getString(m map[string]interface{}, key, defaultValue string) string {
	if val, exists := m[key]; exists {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}

func getBool(m map[string]interface{}, key string, defaultValue bool) bool {
	if val, exists := m[key]; exists {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return defaultValue
}
