package scripting

import (
	"fmt"
	"os"
	"strings"

	"github.com/dop251/goja"
	"github.com/elk-language/go-prompt"
	istrings "github.com/elk-language/go-prompt/strings"
)

// JavaScript bridge methods

// jsRegisterMode allows JavaScript to register a new mode.
func (tm *TUIManager) jsRegisterMode(modeConfig interface{}) error {
	// Convert the config object to a Go struct
	if configMap, ok := modeConfig.(map[string]interface{}); ok {
		mode := &ScriptMode{
			Name:     getString(configMap, "name", ""),
			Commands: make(map[string]Command),
			State:    make(map[string]interface{}),
		}

		// Set up TUI config
		if tuiConfigRaw, exists := configMap["tui"]; exists {
			if tuiMap, ok := tuiConfigRaw.(map[string]interface{}); ok {
				mode.TUIConfig = &TUIConfig{
					Title:         getString(tuiMap, "title", ""),
					Prompt:        getString(tuiMap, "prompt", ""),
					EnableHistory: getBool(tuiMap, "enableHistory", false),
					HistoryFile:   getString(tuiMap, "historyFile", ""),
				}
			}
		}

		// Set up callbacks - store them as interface{} and handle conversion during execution
		if onEnter, exists := configMap["onEnter"]; exists {
			if val := tm.engine.vm.ToValue(onEnter); val != nil {
				if callable, ok := goja.AssertFunction(val); ok {
					mode.OnEnter = callable
				}
			}
		}

		if onExit, exists := configMap["onExit"]; exists {
			if val := tm.engine.vm.ToValue(onExit); val != nil {
				if callable, ok := goja.AssertFunction(val); ok {
					mode.OnExit = callable
				}
			}
		}

		if onPrompt, exists := configMap["onPrompt"]; exists {
			if val := tm.engine.vm.ToValue(onPrompt); val != nil {
				if callable, ok := goja.AssertFunction(val); ok {
					mode.OnPrompt = callable
				}
			}
		}

		// Register commands
		if commandsRaw, exists := configMap["commands"]; exists {
			if commandsMap, ok := commandsRaw.(map[string]interface{}); ok {
				for cmdName, cmdConfig := range commandsMap {
					if cmdMap, ok := cmdConfig.(map[string]interface{}); ok {
						cmd := Command{
							Name:        cmdName,
							Description: getString(cmdMap, "description", ""),
							Usage:       getString(cmdMap, "usage", ""),
							IsGoCommand: false,
						}

						if handler, exists := cmdMap["handler"]; exists {
							cmd.Handler = handler
							mode.Commands[cmdName] = cmd
						}
					}
				}
			}
		}

		return tm.RegisterMode(mode)
	}

	return fmt.Errorf("invalid mode configuration")
}

// jsSwitchMode allows JavaScript to switch modes.
func (tm *TUIManager) jsSwitchMode(modeName string) error {
	return tm.SwitchMode(modeName)
}

// jsGetCurrentMode returns the current mode name.
func (tm *TUIManager) jsGetCurrentMode() string {
	if mode := tm.GetCurrentMode(); mode != nil {
		return mode.Name
	}
	return ""
}

// jsSetState allows JavaScript to set state values.
func (tm *TUIManager) jsSetState(key string, value interface{}) {
	tm.SetState(key, value)
}

// jsGetState allows JavaScript to get state values.
func (tm *TUIManager) jsGetState(key string) interface{} {
	return tm.GetState(key)
}

// jsRegisterCommand allows JavaScript to register global commands.
func (tm *TUIManager) jsRegisterCommand(cmdConfig interface{}) error {
	if configMap, ok := cmdConfig.(map[string]interface{}); ok {
		cmd := Command{
			Name:        getString(configMap, "name", ""),
			Description: getString(configMap, "description", ""),
			Usage:       getString(configMap, "usage", ""),
			IsGoCommand: false,
		}

		if handler, exists := configMap["handler"]; exists {
			// Store the handler as-is, and handle the conversion during execution
			cmd.Handler = handler
			tm.RegisterCommand(cmd)
			return nil
		}

		return fmt.Errorf("command must have a handler function")
	}

	return fmt.Errorf("invalid command configuration")
}

// jsListModes returns a list of available modes.
func (tm *TUIManager) jsListModes() []string {
	return tm.ListModes()
}

// jsCreateAdvancedPrompt creates a new go-prompt instance with given configuration.
func (tm *TUIManager) jsCreateAdvancedPrompt(config interface{}) (string, error) {
	configMap, ok := config.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid prompt configuration")
	}

	// Generate a unique handle for this prompt
	name := getString(configMap, "name", fmt.Sprintf("prompt_%d", len(tm.prompts)))
	title := getString(configMap, "title", "Advanced Prompt")
	prefix := getString(configMap, "prefix", ">>> ")

	// Parse colors configuration, starting from manager defaults, then applying overrides
	colors := tm.defaultColors
	if colorsRaw, exists := configMap["colors"]; exists {
		if colorMap, ok := colorsRaw.(map[string]interface{}); ok {
			colors.ApplyFromInterfaceMap(colorMap)
		}
	}

	// Parse history configuration
	historyConfig := parseHistoryConfig(configMap)

	// Create the executor function for this prompt
	executor := func(line string) {
		if !tm.executor(line) {
			// If executor returns false, exit the prompt
			os.Exit(0)
		}
	}

	// Create the completer function as a dispatcher that can call a JS completer
	completer := func(document prompt.Document) ([]prompt.Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		// Compute selection range around the current word
		before := document.TextBeforeCursor()
		currWord := currentWord(before)
		start := runeIndex(before) - runeLen(currWord)
		end := runeIndex(before)

		// See if a custom completer is configured for this prompt
		tm.mu.RLock()
		completerName, hasCompleter := tm.promptCompleters[name]
		var jsCompleter goja.Callable
		if hasCompleter {
			jsCompleter = tm.completers[completerName]
		}
		tm.mu.RUnlock()

		if jsCompleter != nil && tm.engine != nil && tm.engine.vm != nil {
			if sugg, ok := tm.tryCallJSCompleter(jsCompleter, document); ok {
				return sugg, istrings.RuneNumber(start), istrings.RuneNumber(end)
			}
		}

		// Fallback to default suggestions
		suggestions := tm.getDefaultCompletionSuggestions(document)
		return suggestions, istrings.RuneNumber(start), istrings.RuneNumber(end)
	}

	// Configure prompt options
	options := []prompt.Option{
		prompt.WithTitle(title),
		prompt.WithPrefix(prefix),
		prompt.WithInputTextColor(colors.InputText),
		prompt.WithPrefixTextColor(colors.PrefixText),
		prompt.WithSuggestionTextColor(colors.SuggestionText),
		prompt.WithSelectedSuggestionBGColor(colors.SelectedSuggestionBG),
		prompt.WithSuggestionBGColor(colors.SuggestionBG),
		prompt.WithSelectedSuggestionTextColor(colors.SelectedSuggestionText),
		prompt.WithDescriptionBGColor(colors.DescriptionBG),
		prompt.WithDescriptionTextColor(colors.DescriptionText),
		prompt.WithSelectedDescriptionBGColor(colors.SelectedDescriptionBG),
		prompt.WithSelectedDescriptionTextColor(colors.SelectedDescriptionText),
		prompt.WithScrollbarThumbColor(colors.ScrollbarThumb),
		prompt.WithScrollbarBGColor(colors.ScrollbarBG),
		prompt.WithCompleter(completer),
	}

	// Add history if configured
	if historyConfig.Enabled && historyConfig.File != "" {
		options = append(options, prompt.WithHistory(loadHistory(historyConfig.File)))
	}

	// Add any registered key bindings
	if keyBinds := tm.buildKeyBinds(); len(keyBinds) > 0 {
		options = append(options, prompt.WithKeyBind(keyBinds...))
	}

	// Create the prompt instance
	p := prompt.New(executor, options...)

	// Store the prompt
	tm.mu.Lock()
	tm.prompts[name] = p
	tm.mu.Unlock()

	return name, nil
}

// jsRunPrompt runs a named prompt and returns the input.
func (tm *TUIManager) jsRunPrompt(name string) error {
	tm.mu.RLock()
	p, exists := tm.prompts[name]
	tm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("prompt %s not found", name)
	}

	tm.mu.Lock()
	tm.activePrompt = p
	tm.mu.Unlock()

	// Start the prompt (this will block until exit)
	p.Run()

	tm.mu.Lock()
	tm.activePrompt = nil
	tm.mu.Unlock()

	return nil
}

// jsRegisterCompleter registers a JavaScript completion function.
func (tm *TUIManager) jsRegisterCompleter(name string, completer goja.Callable) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.completers[name] = completer
	return nil
}

// jsSetCompleter sets the completer for a named prompt.
func (tm *TUIManager) jsSetCompleter(promptName, completerName string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	_, exists := tm.prompts[promptName]
	if !exists {
		return fmt.Errorf("prompt %s not found", promptName)
	}

	_, exists = tm.completers[completerName]
	if !exists {
		return fmt.Errorf("completer %s not found", completerName)
	}

	// Store the completer association for future use
	// Since go-prompt doesn't allow changing completers after creation,
	// we'll use this in the completer dispatcher pattern
	if tm.promptCompleters == nil {
		tm.promptCompleters = make(map[string]string)
	}
	tm.promptCompleters[promptName] = completerName

	return nil
}

// jsRegisterKeyBinding registers a JavaScript key binding handler.
func (tm *TUIManager) jsRegisterKeyBinding(key string, handler goja.Callable) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.keyBindings[key] = handler
	return nil
}

// parseKeyString converts a key string to a prompt.Key constant.
func parseKeyString(keyStr string) prompt.Key {
	switch strings.ToLower(keyStr) {
	case "escape", "esc":
		return prompt.Escape
	case "ctrl-a", "control-a":
		return prompt.ControlA
	case "ctrl-b", "control-b":
		return prompt.ControlB
	case "ctrl-c", "control-c":
		return prompt.ControlC
	case "ctrl-d", "control-d":
		return prompt.ControlD
	case "ctrl-e", "control-e":
		return prompt.ControlE
	case "ctrl-f", "control-f":
		return prompt.ControlF
	case "ctrl-g", "control-g":
		return prompt.ControlG
	case "ctrl-h", "control-h":
		return prompt.ControlH
	case "ctrl-i", "control-i":
		return prompt.ControlI
	case "ctrl-j", "control-j":
		return prompt.ControlJ
	case "ctrl-k", "control-k":
		return prompt.ControlK
	case "ctrl-l", "control-l":
		return prompt.ControlL
	case "ctrl-m", "control-m":
		return prompt.ControlM
	case "ctrl-n", "control-n":
		return prompt.ControlN
	case "ctrl-o", "control-o":
		return prompt.ControlO
	case "ctrl-p", "control-p":
		return prompt.ControlP
	case "ctrl-q", "control-q":
		return prompt.ControlQ
	case "ctrl-r", "control-r":
		return prompt.ControlR
	case "ctrl-s", "control-s":
		return prompt.ControlS
	case "ctrl-t", "control-t":
		return prompt.ControlT
	case "ctrl-u", "control-u":
		return prompt.ControlU
	case "ctrl-v", "control-v":
		return prompt.ControlV
	case "ctrl-w", "control-w":
		return prompt.ControlW
	case "ctrl-x", "control-x":
		return prompt.ControlX
	case "ctrl-y", "control-y":
		return prompt.ControlY
	case "ctrl-z", "control-z":
		return prompt.ControlZ
	case "up":
		return prompt.Up
	case "down":
		return prompt.Down
	case "left":
		return prompt.Left
	case "right":
		return prompt.Right
	case "home":
		return prompt.Home
	case "end":
		return prompt.End
	case "delete", "del":
		return prompt.Delete
	case "backspace":
		return prompt.Backspace
	case "tab":
		return prompt.Tab
	case "enter", "return":
		return prompt.Enter
	case "f1":
		return prompt.F1
	case "f2":
		return prompt.F2
	case "f3":
		return prompt.F3
	case "f4":
		return prompt.F4
	case "f5":
		return prompt.F5
	case "f6":
		return prompt.F6
	case "f7":
		return prompt.F7
	case "f8":
		return prompt.F8
	case "f9":
		return prompt.F9
	case "f10":
		return prompt.F10
	case "f11":
		return prompt.F11
	case "f12":
		return prompt.F12
	default:
		return prompt.NotDefined
	}
}

// buildKeyBinds creates go-prompt KeyBind array from registered JavaScript handlers.
func (tm *TUIManager) buildKeyBinds() []prompt.KeyBind {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	var keyBinds []prompt.KeyBind
	for keyStr, handler := range tm.keyBindings {
		key := parseKeyString(keyStr)
		if key != prompt.NotDefined {
			// Create a closure to capture the handler
			jsHandler := handler
			keyBinds = append(keyBinds, prompt.KeyBind{
				Key: key,
				Fn: func(p *prompt.Prompt) bool {
					// Call the JavaScript handler
					result, err := jsHandler(goja.Undefined())
					if err != nil {
						fmt.Fprintf(tm.output, "Key binding error: %v\n", err)
						return false
					}

					// Convert result to boolean (whether to re-render)
					if result != nil && !goja.IsUndefined(result) && !goja.IsNull(result) {
						return result.ToBoolean()
					}
					return false
				},
			})
		}
	}

	return keyBinds
}
