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

		// Intentionally do NOT suggest file arguments at this stage.
		// File/path suggestions should only appear after a trailing space
		// moves the cursor into the first argument position (handled below
		// when len(words) >= 1). This avoids showing files for just typing
		// the command name (e.g. "add").
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
				// GUARD: Avoid fallback when typing the first simple argument and the cursor is
				// immediately after it (no trailing space). In that scenario, fallback suggestions
				// should appear only after the user types a space. Apply this guard only to simple
				// single-token arguments (no paths with '/', no multiple args).
				// Guard is for the first simple argument while the cursor is within the arg token (no trailing space).
				// argv.BeforeCursor returns completed tokens BEFORE the cursor, excluding the current token.
				// Therefore, when typing the first argument, len(words) == 1 (words[0] is the command),
				// and currentWord is the partial argument. When typing the second argument, len(words) == 2.
				isSimpleArgument := len(words) == 1 && currentWord != "" && !strings.Contains(currentWord, "/")
				shouldAvoidFallback := isSimpleArgument && !strings.HasSuffix(before, " ")
				if hasFileCompleters && len(suggestions) == 0 && !shouldAvoidFallback {
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

// tryCallJSCompleter attempts to call a JS completer; returns (suggestions, true) on success, otherwise (nil, false)
func (tm *TUIManager) tryCallJSCompleter(callable goja.Callable, document prompt.Document) ([]prompt.Suggest, error) {
	vm := tm.engine.vm
	// Build a lightweight JS wrapper for the document
	docObj := vm.NewObject()
	_ = docObj.Set("getText", func() string { return document.Text })
	_ = docObj.Set("getTextBeforeCursor", func() string { return document.TextBeforeCursor() })
	_ = docObj.Set("getWordBeforeCursor", func() string { return currentWord(document.TextBeforeCursor()) })

	// Call the JS completer: fn(document)
	value, err := callable(goja.Undefined(), docObj)
	if err != nil {
		return nil, fmt.Errorf("completer call failed: %w", err)
	}

	// Convert the result into []prompt.Suggest
	// Support: array of strings OR array of {text, description}
	var out []prompt.Suggest

	if goja.IsUndefined(value) || goja.IsNull(value) {
		// No suggestions provided
		return nil, nil
	}

	// Try export to []interface{} then map
	var rawArr []interface{}
	if err := vm.ExportTo(value, &rawArr); err != nil {
		return nil, fmt.Errorf("completer must return an array, got %T", value.Export())
	}

	for idx, item := range rawArr {
		switch v := item.(type) {
		case string:
			out = append(out, prompt.Suggest{Text: v})
		case map[string]interface{}:
			obj, ok := any(v).(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("completer result[%d] must be an object, but got %T", idx, v)
			}
			textVal, hasText := obj["text"]
			if !hasText {
				return nil, fmt.Errorf("completer result[%d] missing required 'text' field", idx)
			}
			text, ok := textVal.(string)
			if !ok {
				return nil, fmt.Errorf("completer result[%d].text must be a string, but got %T", idx, textVal)
			}
			desc, _ := obj["description"].(string)
			out = append(out, prompt.Suggest{Text: text, Description: desc})
		default:
			return nil, fmt.Errorf("completer result[%d] has unsupported type %T", idx, v)
		}
	}

	return out, nil
}
