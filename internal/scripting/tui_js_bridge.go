package scripting

import (
	"fmt"
	"strings"

	"github.com/dop251/goja"
	"github.com/joeycumines/go-prompt"
	istrings "github.com/joeycumines/go-prompt/strings"
	"github.com/joeycumines/one-shot-man/internal/argv"
)

// JavaScript bridge methods

// jsRegisterMode allows JavaScript to register a new mode.
// IMPORTANT: This is a JS mutator - it uses scheduleWriteAndWait to avoid deadlocks.
// See TUIManager documentation for the locking strategy.
func (tm *TUIManager) jsRegisterMode(modeConfig interface{}) error {
	// Convert the config object to a Go struct
	if configMap, ok := modeConfig.(map[string]interface{}); ok {
		name, err := getString(configMap, "name", "")
		if err != nil {
			return err
		}
		mode := &ScriptMode{
			Name:         name,
			Commands:     make(map[string]Command),
			CommandOrder: make([]string, 0),
		}

		// N.B. JavaScript manages its own state via tui.createState() which talks directly to StateManager.

		// Process TUI configuration
		if tuiConfig, exists := configMap["tui"]; exists {
			if tuiMap, ok := tuiConfig.(map[string]interface{}); ok {
				mode.TUIConfig = &TUIConfig{}
				var err error
				mode.TUIConfig.Title, err = getString(tuiMap, "title", "")
				if err != nil {
					return err
				}
				mode.TUIConfig.Prompt, err = getString(tuiMap, "prompt", "")
				if err != nil {
					return err
				}
			}
		}

		// This allows modes to specify a command to run automatically after entering.
		if cmdStr, err := getString(configMap, "initialCommand", ""); err != nil {
			_, _ = fmt.Fprintf(tm.writer, "%#v\n", configMap)
			return fmt.Errorf("initialCommand: %v", err)
		} else {
			mode.InitialCommand = cmdStr
		}

		// Process onEnter and onExit lifecycle callbacks
		if onEnter, exists := configMap["onEnter"]; exists {
			if onEnterVal := tm.engine.vm.ToValue(onEnter); onEnterVal != nil {
				if onEnterFunc, ok := goja.AssertFunction(onEnterVal); ok {
					mode.OnEnter = onEnterFunc
				}
			}
		}

		if onExit, exists := configMap["onExit"]; exists {
			if onExitVal := tm.engine.vm.ToValue(onExit); onExitVal != nil {
				if onExitFunc, ok := goja.AssertFunction(onExitVal); ok {
					mode.OnExit = onExitFunc
				}
			}
		}

		if commandsBuilder := configMap["commands"]; commandsBuilder != nil {
			// If it's a JS function, treat it as a CommandsBuilder
			if builderVal := tm.engine.vm.ToValue(commandsBuilder); builderVal != nil {
				if builderFunc, ok := goja.AssertFunction(builderVal); ok {
					mode.CommandsBuilder = builderFunc
				}
			}

			// If it's a plain object (map) provided inline, convert it into mode.Commands
			if objMap, ok := commandsBuilder.(map[string]interface{}); ok {
				for key, raw := range objMap {
					if raw == nil {
						continue
					}
					if cmdObj, ok := raw.(map[string]interface{}); ok {
						desc, _ := getString(cmdObj, "description", "")
						usage, _ := getString(cmdObj, "usage", "")
						argCompleters, _ := getStringSlice(cmdObj, "argCompleters")
						flagDefs, _ := getFlagDefs(cmdObj, "flagDefs")

						cmd := Command{
							Name:          key,
							Description:   desc,
							Usage:         usage,
							IsGoCommand:   false,
							ArgCompleters: argCompleters,
							FlagDefs:      flagDefs,
						}

						if handler, exists := cmdObj["handler"]; exists {
							// Store raw handler; it will be executed via the JS bridge executor
							cmd.Handler = handler
							mode.Commands[key] = cmd
							mode.CommandOrder = append(mode.CommandOrder, key)
						}
					}
				}
			}
		}

		// Register the mode via the writer queue to avoid deadlocks.
		// JS callbacks run without holding locks, so we must not acquire
		// tm.mu.Lock() directly here.
		return tm.scheduleWriteAndWait(func() error {
			if tm.modes == nil {
				tm.modes = make(map[string]*ScriptMode)
			}
			tm.modes[name] = mode
			return nil
		})
	}

	return fmt.Errorf("registerMode: expected object, got %T", modeConfig)
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

// jsRegisterCommand allows JavaScript to register global commands.
// IMPORTANT: This is a JS mutator - it uses scheduleWriteAndWait to avoid deadlocks.
// See TUIManager documentation for the locking strategy.
func (tm *TUIManager) jsRegisterCommand(cmdConfig interface{}) error {
	if configMap, ok := cmdConfig.(map[string]interface{}); ok {
		name, err := getString(configMap, "name", "")
		if err != nil {
			return err
		}
		desc, err := getString(configMap, "description", "")
		if err != nil {
			return err
		}
		usage, err := getString(configMap, "usage", "")
		if err != nil {
			return err
		}
		argCompleters, err := getStringSlice(configMap, "argCompleters")
		if err != nil {
			return err
		}
		flagDefs, err := getFlagDefs(configMap, "flagDefs")
		if err != nil {
			return err
		}
		cmd := Command{
			Name:          name,
			Description:   desc,
			Usage:         usage,
			IsGoCommand:   false,
			ArgCompleters: argCompleters,
			FlagDefs:      flagDefs,
		}

		if handler, exists := configMap["handler"]; exists {
			// Store the handler as-is, and handle the conversion during execution
			cmd.Handler = handler
			// Register via the writer queue to avoid deadlocks.
			// JS callbacks run without holding locks, so we must not acquire
			// tm.mu.Lock() directly here.
			return tm.scheduleWriteAndWait(func() error {
				// If this is a new command, add it to the order slice
				if _, exists := tm.commands[cmd.Name]; !exists {
					tm.commandOrder = append(tm.commandOrder, cmd.Name)
				}
				tm.commands[cmd.Name] = cmd
				return nil
			})
		}

		return fmt.Errorf("command must have a handler function")
	}

	return fmt.Errorf("invalid command configuration")
}

// jsListModes returns a list of available modes.
func (tm *TUIManager) jsListModes() []string {
	return tm.ListModes()
}

// jsCreatePrompt creates a new go-prompt instance with given configuration.
// This is the low-level prompt API (formerly createAdvancedPrompt). For most use cases,
// prefer registerMode which provides richer integration (mode switching, state management, etc.).
//
// IMPORTANT: This is a JS mutator - it uses scheduleWriteAndWait to avoid deadlocks.
// See TUIManager documentation for the locking strategy.
func (tm *TUIManager) jsCreatePrompt(config interface{}) (string, error) {
	configMap, ok := config.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid prompt configuration")
	}

	// Generate a unique handle for this prompt
	name, err := getString(configMap, "name", fmt.Sprintf("prompt_%d", len(tm.prompts)))
	if err != nil {
		return "", err
	}
	title, err := getString(configMap, "title", "Advanced Prompt")
	if err != nil {
		return "", err
	}
	prefix, err := getString(configMap, "prefix", ">>> ")
	if err != nil {
		return "", err
	}

	// Parse colors configuration, starting from manager defaults, then applying overrides
	colors := tm.defaultColors
	if colorsRaw, exists := configMap["colors"]; exists {
		if colorMap, ok := colorsRaw.(map[string]interface{}); ok {
			colors.ApplyFromInterfaceMap(colorMap)
		}
	}

	// Parse history configuration
	historyConfig, err := parseHistoryConfig(configMap)
	if err != nil {
		return "", err
	}

	// Create the completer function as a dispatcher that can call a JS completer
	completer := func(document prompt.Document) ([]prompt.Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		// Compute selection range around the current word
		before := document.TextBeforeCursor()
		_, cur := argv.BeforeCursor(before)
		start, end := cur.Start, cur.End

		// See if a custom completer is configured for this prompt
		tm.mu.RLock()
		completerName, hasCompleter := tm.promptCompleters[name]
		var jsCompleter goja.Callable
		if hasCompleter {
			jsCompleter = tm.completers[completerName]
		}
		tm.mu.RUnlock()

		if jsCompleter != nil && tm.engine != nil && tm.engine.vm != nil {
			if sugg, err := tm.tryCallJSCompleter(jsCompleter, document); err != nil {
				_, _ = fmt.Fprintf(tm.writer, "Completer error: %v\n", err)
			} else if sugg != nil {
				return sugg, istrings.RuneNumber(start), istrings.RuneNumber(end)
			}
		}

		// Fallback to default suggestions
		suggestions := tm.getDefaultCompletionSuggestions(document)
		return suggestions, istrings.RuneNumber(start), istrings.RuneNumber(end)
	}

	// Build history from file if configured
	var history []string
	if historyConfig.Enabled && historyConfig.File != "" {
		history = loadHistory(historyConfig.File)
	}

	// Use the shared builder with consistent feature support
	p := tm.buildGoPrompt(promptBuildConfig{
		prefix:                  prefix,
		title:                   title,
		colors:                  colors,
		completer:               completer,
		history:                 history,
		maxSuggestion:           10,
		dynamicCompletion:       true,
		executeHidesCompletions: true,
		escapeToggle:            true,
	})

	// Store the prompt via the writer queue to avoid deadlocks.
	// JS callbacks run without holding locks, so we must not acquire
	// tm.mu.Lock() directly here.
	err = tm.scheduleWriteAndWait(func() error {
		tm.prompts[name] = p
		return nil
	})
	if err != nil {
		return "", err
	}

	return name, nil
}

// jsCreateAdvancedPrompt is a backward-compatible alias for jsCreatePrompt.
// It logs a deprecation warning and delegates to jsCreatePrompt.
func (tm *TUIManager) jsCreateAdvancedPrompt(config interface{}) (string, error) {
	_, _ = fmt.Fprintf(tm.writer, "Warning: tui.createAdvancedPrompt is deprecated, use tui.createPrompt instead\n")
	return tm.jsCreatePrompt(config)
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
// IMPORTANT: This is a JS mutator - it uses scheduleWriteAndWait to avoid deadlocks.
// See TUIManager documentation for the locking strategy.
func (tm *TUIManager) jsRegisterCompleter(name string, completer goja.Callable) error {
	// Register via the writer queue to avoid deadlocks.
	// JS callbacks run without holding locks, so we must not acquire
	// tm.mu.Lock() directly here.
	return tm.scheduleWriteAndWait(func() error {
		tm.completers[name] = completer
		return nil
	})
}

// jsSetCompleter sets the completer for a named prompt.
// IMPORTANT: This is a JS mutator - it uses scheduleWriteAndWait to avoid deadlocks.
// See TUIManager documentation for the locking strategy.
func (tm *TUIManager) jsSetCompleter(promptName, completerName string) error {
	// Validate and set via the writer queue to avoid deadlocks.
	// JS callbacks run without holding locks, so we must not acquire
	// tm.mu.Lock() directly here.
	return tm.scheduleWriteAndWait(func() error {
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
	})
}

// jsRegisterKeyBinding registers a JavaScript key binding handler.
// IMPORTANT: This is a JS mutator - it uses scheduleWriteAndWait to avoid deadlocks.
// See TUIManager documentation for the locking strategy.
func (tm *TUIManager) jsRegisterKeyBinding(key string, handler goja.Callable) error {
	// Register via the writer queue to avoid deadlocks.
	// JS callbacks run without holding locks, so we must not acquire
	// tm.mu.Lock() directly here.
	return tm.scheduleWriteAndWait(func() error {
		tm.keyBindings[key] = handler
		return nil
	})
}

// parseKeyString converts a key string to a prompt.Key constant.
func parseKeyString(keyStr string) prompt.Key {
	switch strings.ToLower(keyStr) {
	case "escape", "esc":
		return prompt.Escape
	case "ctrl-a", "control-a", "ctrl+a", "control+a":
		return prompt.ControlA
	case "ctrl-b", "control-b", "ctrl+b", "control+b":
		return prompt.ControlB
	case "ctrl-c", "control-c", "ctrl+c", "control+c":
		return prompt.ControlC
	case "ctrl-d", "control-d", "ctrl+d", "control+d":
		return prompt.ControlD
	case "ctrl-e", "control-e", "ctrl+e", "control+e":
		return prompt.ControlE
	case "ctrl-f", "control-f", "ctrl+f", "control+f":
		return prompt.ControlF
	case "ctrl-g", "control-g", "ctrl+g", "control+g":
		return prompt.ControlG
	case "ctrl-h", "control-h", "ctrl+h", "control+h":
		return prompt.ControlH
	case "ctrl-i", "control-i", "ctrl+i", "control+i":
		return prompt.ControlI
	case "ctrl-j", "control-j", "ctrl+j", "control+j":
		return prompt.ControlJ
	case "ctrl-k", "control-k", "ctrl+k", "control+k":
		return prompt.ControlK
	case "ctrl-l", "control-l", "ctrl+l", "control+l":
		return prompt.ControlL
	case "ctrl-m", "control-m", "ctrl+m", "control+m":
		return prompt.ControlM
	case "ctrl-n", "control-n", "ctrl+n", "control+n":
		return prompt.ControlN
	case "ctrl-o", "control-o", "ctrl+o", "control+o":
		return prompt.ControlO
	case "ctrl-p", "control-p", "ctrl+p", "control+p":
		return prompt.ControlP
	case "ctrl-q", "control-q", "ctrl+q", "control+q":
		return prompt.ControlQ
	case "ctrl-r", "control-r", "ctrl+r", "control+r":
		return prompt.ControlR
	case "ctrl-s", "control-s", "ctrl+s", "control+s":
		return prompt.ControlS
	case "ctrl-t", "control-t", "ctrl+t", "control+t":
		return prompt.ControlT
	case "ctrl-u", "control-u", "ctrl+u", "control+u":
		return prompt.ControlU
	case "ctrl-v", "control-v", "ctrl+v", "control+v":
		return prompt.ControlV
	case "ctrl-w", "control-w", "ctrl+w", "control+w":
		return prompt.ControlW
	case "ctrl-x", "control-x", "ctrl+x", "control+x":
		return prompt.ControlX
	case "ctrl-y", "control-y", "ctrl+y", "control+y":
		return prompt.ControlY
	case "ctrl-z", "control-z", "ctrl+z", "control+z":
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
						_, _ = fmt.Fprintf(tm.writer, "Key binding error: %v\n", err)
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
