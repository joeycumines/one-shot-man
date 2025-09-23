package scripting

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/dop251/goja"
	"github.com/elk-language/go-prompt"
	"github.com/joeycumines/one-shot-man/internal/argv"
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

// getFilepathSuggestions provides file and directory path completion.
// It expands '~' and returns suggestions that properly replace the input path.
func getFilepathSuggestions(path string) []prompt.Suggest {
	// Handle the simple case of "~" separately to suggest "~/"
	if path == "~" {
		return []prompt.Suggest{{Text: "~/"}}
	}

	// Handle empty path - should list current directory contents
	if path == "" {
		entries, err := os.ReadDir(".")
		if err != nil {
			return nil
		}
		var suggestions []prompt.Suggest
		for _, entry := range entries {
			text := entry.Name()
			if entry.IsDir() {
				text += "/"
			}
			suggestions = append(suggestions, prompt.Suggest{Text: text})
		}
		return suggestions
	}

	// Expand tilde in the path
	expandedPath := path
	if strings.HasPrefix(path, "~/") {
		usr, err := user.Current()
		if err == nil { // Silently ignore error if home dir can't be found
			expandedPath = filepath.Join(usr.HomeDir, path[2:])
		}
	}

	// Determine the directory to scan and the prefix of the file/dir to match
	dirToScan := filepath.Dir(expandedPath)
	prefix := filepath.Base(expandedPath)

	// If the user's input path is the root or an existing directory,
	// list the contents of that directory.
	if expandedPath == "/" {
		dirToScan = "/"
		prefix = ""
	} else if fi, err := os.Stat(expandedPath); err == nil && fi.IsDir() {
		dirToScan = expandedPath
		prefix = ""
	}

	entries, err := os.ReadDir(dirToScan)
	if err != nil {
		return nil // Gracefully handle errors like permission denied by returning no suggestions
	}

	var suggestions []prompt.Suggest
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			// Build the full replacement text that includes the directory path
			var text string
			if dirToScan == "." {
				// For current directory, just use the entry name
				text = entry.Name()
			} else if strings.HasSuffix(path, "/") || prefix == "" {
				// If input ends with / or we're completing in a directory,
				// append the entry name to the input path
				text = path + entry.Name()
			} else {
				// Replace the basename part with the matched entry
				dirPart := filepath.Dir(path)
				if dirPart == "." {
					text = entry.Name()
				} else {
					text = dirPart + "/" + entry.Name()
				}
			}

			if entry.IsDir() {
				text += "/"
			}

			suggestions = append(suggestions, prompt.Suggest{Text: text})
		}
	}
	return suggestions
}

// getDefaultCompletionSuggestions provides default completion when no custom completer is set.
func (tm *TUIManager) getDefaultCompletionSuggestions(document prompt.Document) []prompt.Suggest {
	// Delegate to a helper that accepts explicit before/full text.
	before := document.TextBeforeCursor()
	if before == "" {
		before = document.Text
	}
	return tm.getDefaultCompletionSuggestionsFor(before, document.Text)
}

// getDefaultCompletionSuggestionsFor is the pure implementation that powers completion.
// It uses the text before the cursor (before) to determine the current token, while
// still accepting the full input line (full) for any future needs. This function is
// exported only within the package to facilitate unit testing with simulated cursor positions.
func (tm *TUIManager) getDefaultCompletionSuggestionsFor(before, full string) []prompt.Suggest {
	var suggestions []prompt.Suggest

	// Cursor-aware tokenization based solely on text before the cursor
	completed, currentTok := argv.BeforeCursor(before)
	words := completed
	currentWord := currentTok.Text
	// Note: 'words' are completed tokens BEFORE the cursor.
	// If len(words)==0, we're editing the first token -> command completion path.
	// If len(words)>=1, we're on an argument token for command words[0].

	// currentWord already refers to the token content at the cursor (quotes removed)

	// TODO: CRITICAL COMPLETION LOGIC DOCUMENTATION
	// This function handles completion with the following precedence:
	// 1. Command completion (for first word)
	// 2. Argument completion (for subsequent words)
	// 3. File completion fallback (when arg completers exist but current arg has no matches)
	//
	// The precedence for commands is: mode commands > registered commands > built-in commands
	// The precedence for arg completers is based on their order in cmd.ArgCompleters array
	//
	// IMPORTANT: When a command has file arg completers, we suggest files even if:
	// - Only the command is typed (e.g., "add" suggests files after command suggestions)
	// - Current file argument has no matches (suggests new args from CWD)

	// Provide command completion for first word
	if len(words) == 0 {
		// Collect all commands with precedence: mode commands > registered commands > built-in commands
		// Use a map for deduplication but maintain order
		commandMap := make(map[string]prompt.Suggest)
		var orderedCommandNames []string

		// Built-in commands (lowest precedence)
		builtinCommands := []string{"help", "exit", "quit", "mode", "modes", "state"}
		for _, cmd := range builtinCommands {
			if strings.HasPrefix(cmd, currentWord) {
				if _, exists := commandMap[cmd]; !exists {
					orderedCommandNames = append(orderedCommandNames, cmd)
				}
				commandMap[cmd] = prompt.Suggest{
					Text:        cmd,
					Description: "Built-in command",
				}
			}
		}

		// Registered commands (medium precedence)
		tm.mu.RLock()
		for _, cmdName := range tm.commandOrder {
			if cmd, exists := tm.commands[cmdName]; exists && strings.HasPrefix(cmd.Name, currentWord) {
				if _, exists := commandMap[cmd.Name]; !exists {
					orderedCommandNames = append(orderedCommandNames, cmd.Name)
				}
				commandMap[cmd.Name] = prompt.Suggest{
					Text:        cmd.Name,
					Description: cmd.Description,
				}
			}
		}

		// Current mode commands (highest precedence)
		if tm.currentMode != nil {
			tm.currentMode.mu.RLock()
			for _, cmdName := range tm.currentMode.CommandOrder {
				if cmd, exists := tm.currentMode.Commands[cmdName]; exists && strings.HasPrefix(cmd.Name, currentWord) {
					if _, exists := commandMap[cmd.Name]; !exists {
						orderedCommandNames = append(orderedCommandNames, cmd.Name)
					}
					commandMap[cmd.Name] = prompt.Suggest{
						Text:        cmd.Name,
						Description: cmd.Description,
					}
				}
			}
			tm.currentMode.mu.RUnlock()
		}
		tm.mu.RUnlock()

		// Convert to slice in the order we collected them
		for _, cmdName := range orderedCommandNames {
			if suggestion, exists := commandMap[cmdName]; exists {
				suggestions = append(suggestions, suggestion)
			}
		}

		// NEW: After command suggestions, check if this command supports file completion
		// and suggest files even when only the command is typed. To avoid redundant filesystem
		// reads, cache the CWD file suggestions once per completion invocation.
		func() {
			tm.mu.RLock()
			defer tm.mu.RUnlock()

			var cwdFileSuggestions []prompt.Suggest
			var cwdFileSuggestionsReady bool
			getCWD := func() []prompt.Suggest {
				if !cwdFileSuggestionsReady {
					cwdFileSuggestions = getFilepathSuggestions("")
					cwdFileSuggestionsReady = true
				}
				return cwdFileSuggestions
			}

			// helper to process a single command
			appendFileArgFor := func(cmd Command) {
				for _, ac := range cmd.ArgCompleters {
					if ac == "file" {
						for _, fs := range getCWD() {
							suggestions = append(suggestions, prompt.Suggest{
								Text:        cmd.Name + " " + fs.Text,
								Description: "Add file: " + fs.Text,
							})
						}
						break
					}
				}
			}

			// global commands
			for _, cmdName := range tm.commandOrder {
				if cmd, exists := tm.commands[cmdName]; exists && strings.HasPrefix(cmd.Name, currentWord) {
					appendFileArgFor(cmd)
				}
			}

			// mode commands
			if tm.currentMode != nil {
				tm.currentMode.mu.RLock()
				for _, cmdName := range tm.currentMode.CommandOrder {
					if cmd, exists := tm.currentMode.Commands[cmdName]; exists && strings.HasPrefix(cmd.Name, currentWord) {
						appendFileArgFor(cmd)
					}
				}
				tm.currentMode.mu.RUnlock()
			}
		}()
	} else if len(words) >= 1 {
		// If we have more than one word, check for argument completers.
		func() {
			tm.mu.RLock()
			defer tm.mu.RUnlock()

			// Find the command definition, checking the current mode first.
			var cmd *Command
			commandName := words[0]

			if tm.currentMode != nil {
				if c, ok := tm.currentMode.Commands[commandName]; ok {
					cmd = &c
				}
			}

			if cmd == nil {
				if c, ok := tm.commands[commandName]; ok {
					cmd = &c
				}
			}

			if cmd != nil {
				// TODO: CRITICAL - Handle multiple arg completers with proper precedence
				// The order in cmd.ArgCompleters should determine priority.
				// Currently we only handle "file" type, but future types should be processed
				// in the order they appear in the slice for proper precedence.
				var hasFileCompleters bool
				var fileCompleterProcessed bool

				for _, argCompleter := range cmd.ArgCompleters {
					switch argCompleter {
					case "file":
						if !fileCompleterProcessed {
							hasFileCompleters = true
							fileSuggestions := getFilepathSuggestions(currentWord)
							suggestions = append(suggestions, fileSuggestions...)
							fileCompleterProcessed = true
						}
					// TODO: Add other arg completer types here (e.g., "command", "mode", etc.)
					// and respect the order they appear in cmd.ArgCompleters
					default:
						// Unknown completer type - ignore for now but log for future implementation
						// TODO: Add logging or warning for unknown completer types
					}
				}

				// NEW: If no file suggestions were found but command supports file completion,
				// suggest new file arguments from CWD
				if hasFileCompleters && len(suggestions) == 0 {
					fallbackSuggestions := getFilepathSuggestions("")
					suggestions = append(suggestions, fallbackSuggestions...)
				}
			}
		}()
	}

	// For mode command, suggest available modes
	// Mode name completion: editing second token of 'mode <name>'
	if len(words) == 1 && words[0] == "mode" {
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
func currentWord(before string) string { _, cur := argv.BeforeCursor(before); return cur.Text }

// tokenizeCommandLine tokenizes an entire input line into arguments with shell-like rules.
// Supports single/double quotes and backslash escaping. Unclosed quotes are allowed and
// produce a final token up to the end of input. Returned tokens do not include surrounding quotes.
func tokenizeCommandLine(line string) []string { return argv.ParseSlice(line) }

// tryCallJSCompleter attempts to call a JS completer; returns (suggestions, true) on success, otherwise (nil, false)
func (tm *TUIManager) tryCallJSCompleter(callable goja.Callable, document prompt.Document) ([]prompt.Suggest, bool) {
	defer func() {
		if r := recover(); r != nil {
			_, _ = fmt.Fprintf(tm.output, "Completer panic: %v\n", r)
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
		_, _ = fmt.Fprintf(tm.output, "Completer error: %v\n", err)
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

func getStringSlice(m map[string]interface{}, key string) (result []string) {
	if val, exists := m[key]; exists {
		if arr, ok := val.([]interface{}); ok {
			for _, item := range arr {
				if str, ok := item.(string); ok {
					result = append(result, str)
				}
			}
		}
	}
	return result
}
