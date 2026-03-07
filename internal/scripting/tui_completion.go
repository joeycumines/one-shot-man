package scripting

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/dop251/goja"
	"github.com/joeycumines/go-prompt"
	istrings "github.com/joeycumines/go-prompt/strings"
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
	} else if fi, err := os.Stat(expandedPath); err == nil && fi.IsDir() && strings.HasSuffix(path, "/") {
		// Only scan directory contents when the user typed a trailing slash.
		// Without the slash, scan the parent to suggest the directory name
		// itself (e.g. "bin/") so the user can tab-complete into it.
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
				if !strings.HasSuffix(path, "/") {
					text = path + "/" + entry.Name()
				} else {
					text = path + entry.Name()
				}
			} else {
				// Replace the basename part with the matched entry
				dirPart := filepath.Dir(path)
				if dirPart == "." {
					text = entry.Name()
				} else if dirPart == "/" {
					// filepath.Join is avoided here to ensure the suggestion text does not mutate/clean
					// the user's input.
					text = dirPart + entry.Name()
				} else if dirPart == "~" && strings.HasPrefix(path, "~//") {
					text = "~//" + entry.Name()
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

// getExecutableSuggestions provides completion suggestions for executable commands
// found in the system PATH. It scans each PATH directory for files whose names
// match the given prefix and are executable. If no prefix is provided, common
// shell built-ins are suggested instead of scanning the entire PATH (which can
// be expensive).
func getExecutableSuggestions(prefix string) []prompt.Suggest {
	// When no prefix is provided, suggest common commands rather than
	// scanning every PATH directory, which can contain thousands of entries.
	if prefix == "" {
		common := []struct {
			text string
			desc string
		}{
			{"cat", "concatenate and print files"},
			{"curl", "transfer data from URLs"},
			{"date", "display date and time"},
			{"echo", "display a line of text"},
			{"env", "display environment"},
			{"find", "search for files"},
			{"git", "version control"},
			{"grep", "search text patterns"},
			{"head", "output first part of files"},
			{"jq", "JSON processor"},
			{"ls", "list directory contents"},
			{"make", "build automation"},
			{"pwd", "print working directory"},
			{"sed", "stream editor"},
			{"sort", "sort lines of text"},
			{"tail", "output last part of files"},
			{"wc", "word, line, character count"},
		}
		var suggestions []prompt.Suggest
		for _, c := range common {
			suggestions = append(suggestions, prompt.Suggest{
				Text:        c.text,
				Description: c.desc,
			})
		}
		return suggestions
	}

	lowerPrefix := strings.ToLower(prefix)

	// If prefix contains a path separator, delegate to file completion
	// since the user is typing a path to an executable.
	if strings.ContainsRune(prefix, filepath.Separator) || strings.ContainsRune(prefix, '/') {
		return getFilepathSuggestions(prefix)
	}

	// Scan PATH directories for matching executables.
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return nil
	}

	seen := make(map[string]bool)
	var suggestions []prompt.Suggest

	for _, dir := range filepath.SplitList(pathEnv) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if seen[name] {
				continue
			}
			if !strings.HasPrefix(strings.ToLower(name), lowerPrefix) {
				continue
			}
			// Check executable bit (Unix) or extension (Windows)
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.Mode()&0111 == 0 {
				continue // not executable
			}
			seen[name] = true
			suggestions = append(suggestions, prompt.Suggest{
				Text:        name,
				Description: "executable",
			})
		}
	}
	return suggestions
}

// getGitRefSuggestions provides completion suggestions for git refs (branches, tags,
// recent commits, and common refs). It shells out to git for dynamic data. If git
// commands fail (e.g. not in a git repo), it silently falls back to common ref
// suggestions only.
func getGitRefSuggestions(prefix string) []prompt.Suggest {
	// Common refs always available (flags like --staged belong in
	// command-level flagDefs to avoid duplication with the flag completer).
	commonRefs := []struct {
		text string
		desc string
	}{
		{"HEAD", "current commit"},
		{"HEAD~1", "1 commit before HEAD"},
		{"HEAD~2", "2 commits before HEAD"},
		{"HEAD~3", "3 commits before HEAD"},
	}

	var suggestions []prompt.Suggest
	lowerPrefix := strings.ToLower(prefix)

	// Add common refs filtered by prefix
	for _, ref := range commonRefs {
		if prefix == "" || strings.HasPrefix(strings.ToLower(ref.text), lowerPrefix) {
			suggestions = append(suggestions, prompt.Suggest{
				Text:        ref.text,
				Description: ref.desc,
			})
		}
	}

	// Try to get local branches from git
	if branchOut, err := exec.Command("git", "branch", "--format=%(refname:short)").Output(); err == nil {
		for _, line := range strings.Split(string(branchOut), "\n") {
			name := strings.TrimSpace(line)
			if name == "" {
				continue
			}
			if prefix == "" || strings.HasPrefix(strings.ToLower(name), lowerPrefix) {
				suggestions = append(suggestions, prompt.Suggest{
					Text:        name,
					Description: "branch",
				})
			}
		}
	}

	// Try to get remote branches from git
	if remoteBranchOut, err := exec.Command("git", "branch", "-r", "--format=%(refname:short)").Output(); err == nil {
		for _, line := range strings.Split(string(remoteBranchOut), "\n") {
			name := strings.TrimSpace(line)
			if name == "" || strings.Contains(name, "->") {
				continue // skip HEAD -> origin/main symbolic refs
			}
			if prefix == "" || strings.HasPrefix(strings.ToLower(name), lowerPrefix) {
				suggestions = append(suggestions, prompt.Suggest{
					Text:        name,
					Description: "remote branch",
				})
			}
		}
	}

	// Try to get tags from git
	if tagOut, err := exec.Command("git", "tag", "--list").Output(); err == nil {
		for _, line := range strings.Split(string(tagOut), "\n") {
			name := strings.TrimSpace(line)
			if name == "" {
				continue
			}
			if prefix == "" || strings.HasPrefix(strings.ToLower(name), lowerPrefix) {
				suggestions = append(suggestions, prompt.Suggest{
					Text:        name,
					Description: "tag",
				})
			}
		}
	}

	// Try to get recent commit SHAs with subject lines (last 10)
	if commitOut, err := exec.Command("git", "log", "--oneline", "-10", "--format=%h %s").Output(); err == nil {
		for _, line := range strings.Split(string(commitOut), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Split into SHA and subject
			parts := strings.SplitN(line, " ", 2)
			sha := parts[0]
			subject := ""
			if len(parts) > 1 {
				subject = parts[1]
				// Truncate long subjects for display
				const maxSubjectLen = 50
				if len(subject) > maxSubjectLen {
					subject = subject[:maxSubjectLen-3] + "..."
				}
			}
			if prefix == "" || strings.HasPrefix(strings.ToLower(sha), lowerPrefix) {
				suggestions = append(suggestions, prompt.Suggest{
					Text:        sha,
					Description: subject,
				})
			}
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

	// Completion precedence:
	// 1. Command completion (for first word)
	// 2. Argument completion (for subsequent words)
	// 3. File completion fallback (when arg completers exist but current arg has no matches)
	//
	// Command precedence: mode commands > registered commands > built-in commands.
	// Arg completer precedence: order in cmd.ArgCompleters slice.
	//
	// When a command has file arg completers, files are suggested even if:
	// - Only the command is typed (after trailing space moves cursor to arg position)
	// - Current file argument has no matches (suggests from CWD)

	// Provide command completion for first word
	if len(words) == 0 {
		// Collect all commands with precedence: mode commands > registered commands > built-in commands
		// Use a map for deduplication but maintain order
		commandMap := make(map[string]prompt.Suggest)
		var orderedCommandNames []string

		// Built-in commands (lowest precedence)
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
				// Arg completers are processed in the order they appear in the
				// ArgCompleters slice. Supported types: file, executable, flag, gitref.
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
					case "executable":
						execSuggestions := getExecutableSuggestions(currentWord)
						suggestions = append(suggestions, execSuggestions...)
					case "flag":
						for _, def := range cmd.FlagDefs {
							flagText := "--" + def.Name
							if currentWord == "" || strings.HasPrefix(strings.ToLower(flagText), strings.ToLower(currentWord)) {
								suggestions = append(suggestions, prompt.Suggest{
									Text:        flagText,
									Description: def.Description,
								})
							}
						}
					case "gitref":
						gitRefSuggestions := getGitRefSuggestions(currentWord)
						suggestions = append(suggestions, gitRefSuggestions...)
					default:
						log.Printf("warning: unknown arg completer type %q for command %q", argCompleter, cmd.Name)
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
	_ = docObj.Set("getTextAfterCursor", func() string { return document.TextAfterCursor() })
	_ = docObj.Set("getWordAfterCursor", func() string { return document.GetWordAfterCursor() })
	_ = docObj.Set("getCurrentLine", func() string { return document.CurrentLine() })
	_ = docObj.Set("getCurrentLineBeforeCursor", func() string { return document.CurrentLineBeforeCursor() })
	_ = docObj.Set("getCurrentLineAfterCursor", func() string { return document.CurrentLineAfterCursor() })
	_ = docObj.Set("getCursorPositionCol", func() int { return int(document.CursorPositionCol()) })
	_ = docObj.Set("getCursorPositionRow", func() int { return int(document.CursorPositionRow()) })
	_ = docObj.Set("getLines", func() []string { return document.Lines() })
	_ = docObj.Set("getLineCount", func() int { return document.LineCount() })
	_ = docObj.Set("onLastLine", func() bool { return document.OnLastLine() })
	_ = docObj.Set("getCharRelativeToCursor", func(offset int) string {
		r := document.GetCharRelativeToCursor(istrings.RuneNumber(offset))
		if r == 0 {
			return ""
		}
		return string(r)
	})

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

	// Try export to []any then map
	var rawArr []any
	if err := vm.ExportTo(value, &rawArr); err != nil {
		return nil, fmt.Errorf("completer must return an array, got %T", value.Export())
	}

	for idx, item := range rawArr {
		switch v := item.(type) {
		case string:
			out = append(out, prompt.Suggest{Text: v})
		case map[string]any:
			obj, ok := any(v).(map[string]any)
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
