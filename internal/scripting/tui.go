package scripting

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/dop251/goja"
	"github.com/elk-language/go-prompt"
	istrings "github.com/elk-language/go-prompt/strings"
)

// TUIManager manages rich terminal interfaces for script modes.

// TUIManager manages rich terminal interfaces for script modes.
type TUIManager struct {
	engine           *Engine
	ctx              context.Context
	currentMode      *ScriptMode
	modes            map[string]*ScriptMode
	commands         map[string]Command
	mu               sync.RWMutex
	input            io.Reader
	output           io.Writer
	prompts          map[string]*prompt.Prompt // Manages named prompt instances
	activePrompt     *prompt.Prompt            // Pointer to the currently active prompt
	completers       map[string]goja.Callable  // JavaScript completion functions
	keyBindings      map[string]goja.Callable  // JavaScript key binding handlers
	promptCompleters map[string]string         // Maps prompt names to completer names
	// defaultColors controls the default color scheme used when running prompts
	// without explicit color configuration. It is initialized with sensible
	// defaults and can be overridden by configuration (e.g., config file).
	defaultColors PromptColors
}

// ScriptMode represents a specific script mode with its own state and commands.
type ScriptMode struct {
	Name      string
	Script    *Script
	State     map[string]interface{}
	Commands  map[string]Command
	TUIConfig *TUIConfig
	OnEnter   goja.Callable
	OnExit    goja.Callable
	OnPrompt  goja.Callable
	mu        sync.RWMutex
}

// TUIConfig defines the configuration for a rich TUI interface.
type TUIConfig struct {
	Title         string
	Prompt        string
	CompletionFn  goja.Callable
	ValidatorFn   goja.Callable
	HistoryFile   string
	EnableHistory bool
}

// Command represents a command that can be executed in the terminal.
type Command struct {
	Name        string
	Description string
	Usage       string
	Handler     interface{} // Can be goja.Callable or Go function
	IsGoCommand bool
}

// NewTUIManager creates a new TUI manager.
func NewTUIManager(ctx context.Context, engine *Engine, input io.Reader, output io.Writer) *TUIManager {
	manager := &TUIManager{
		engine:           engine,
		ctx:              ctx,
		modes:            make(map[string]*ScriptMode),
		commands:         make(map[string]Command),
		input:            input,
		output:           output,
		prompts:          make(map[string]*prompt.Prompt),
		completers:       make(map[string]goja.Callable),
		keyBindings:      make(map[string]goja.Callable),
		promptCompleters: make(map[string]string),
		defaultColors: PromptColors{
			// Choose a readable default for input that is not yellow/white-adjacent
			InputText:               prompt.Green,
			PrefixText:              prompt.Cyan,
			SuggestionText:          prompt.Yellow,
			SuggestionBG:            prompt.Black,
			SelectedSuggestionText:  prompt.Black,
			SelectedSuggestionBG:    prompt.Cyan,
			DescriptionText:         prompt.White,
			DescriptionBG:           prompt.Black,
			SelectedDescriptionText: prompt.White,
			SelectedDescriptionBG:   prompt.Blue,
			ScrollbarThumb:          prompt.DarkGray,
			ScrollbarBG:             prompt.Black,
		},
	}

	// Register built-in commands
	manager.registerBuiltinCommands()

	return manager
}

// RegisterMode registers a new script mode.
func (tm *TUIManager) RegisterMode(mode *ScriptMode) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, exists := tm.modes[mode.Name]; exists {
		return fmt.Errorf("mode %s already exists", mode.Name)
	}

	tm.modes[mode.Name] = mode
	return nil
}

// SwitchMode switches to a different script mode.
func (tm *TUIManager) SwitchMode(modeName string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	mode, exists := tm.modes[modeName]
	if !exists {
		return fmt.Errorf("mode %s not found", modeName)
	}

	// Exit current mode
	if tm.currentMode != nil && tm.currentMode.OnExit != nil {
		if _, err := tm.currentMode.OnExit(goja.Undefined()); err != nil {
			fmt.Fprintf(tm.output, "Error exiting mode %s: %v\n", tm.currentMode.Name, err)
		}
	}

	fmt.Fprintf(tm.output, "Switched to mode: %s\n", mode.Name)

	// Enter new mode
	tm.currentMode = mode
	if mode.OnEnter != nil {
		if _, err := mode.OnEnter(goja.Undefined()); err != nil {
			fmt.Fprintf(tm.output, "Error entering mode %s: %v\n", mode.Name, err)
		}
	}

	return nil
}

// GetCurrentMode returns the current active mode.
func (tm *TUIManager) GetCurrentMode() *ScriptMode {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.currentMode
}

// RegisterCommand registers a command with the TUI manager.
func (tm *TUIManager) RegisterCommand(cmd Command) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.commands[cmd.Name] = cmd
}

// ExecuteCommand executes a command by name.
func (tm *TUIManager) ExecuteCommand(name string, args []string) error {
	tm.mu.RLock()

	// Check current mode commands first
	if tm.currentMode != nil {
		tm.currentMode.mu.RLock()
		if cmd, exists := tm.currentMode.Commands[name]; exists {
			tm.currentMode.mu.RUnlock()
			tm.mu.RUnlock()
			return tm.executeCommand(cmd, args)
		}
		tm.currentMode.mu.RUnlock()
	}

	// Check global commands
	cmd, exists := tm.commands[name]
	tm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("command %s not found", name)
	}

	return tm.executeCommand(cmd, args)
}

// executeCommand handles the actual command execution.
func (tm *TUIManager) executeCommand(cmd Command, args []string) error {
	if cmd.IsGoCommand {
		// Handle Go function
		if fn, ok := cmd.Handler.(func([]string) error); ok {
			return fn(args)
		} else if fn, ok := cmd.Handler.(func(*TUIManager, []string) error); ok {
			return fn(tm, args)
		}
		return fmt.Errorf("invalid Go command handler for %s", cmd.Name)
	} else {
		// Handle JavaScript function - try different types
		// Convert args to JavaScript array
		argsJS := tm.engine.vm.NewArray()
		for i, arg := range args {
			argsJS.Set(fmt.Sprintf("%d", i), arg)
		}

		switch handler := cmd.Handler.(type) {
		case goja.Callable:
			_, err := handler(goja.Undefined(), argsJS)
			return err
		case func(goja.FunctionCall) goja.Value:
			// Create a function call with the arguments
			call := goja.FunctionCall{
				This:      goja.Undefined(),
				Arguments: []goja.Value{argsJS},
			}
			handler(call)
			return nil
		default:
			// Try to call it as a general function
			if tm.engine != nil && tm.engine.vm != nil {
				val := tm.engine.vm.ToValue(handler)
				if callable, ok := goja.AssertFunction(val); ok {
					_, err := callable(goja.Undefined(), argsJS)
					return err
				}
			}
			return fmt.Errorf("invalid JavaScript command handler for %s: %T", cmd.Name, handler)
		}
	}
}
func (tm *TUIManager) GetState(key string) interface{} {
	if tm.currentMode == nil {
		return nil
	}

	tm.currentMode.mu.RLock()
	defer tm.currentMode.mu.RUnlock()

	return tm.currentMode.State[key]
}

// SetState sets a state value for the current mode.
func (tm *TUIManager) SetState(key string, value interface{}) {
	if tm.currentMode == nil {
		return
	}

	tm.currentMode.mu.Lock()
	defer tm.currentMode.mu.Unlock()

	if tm.currentMode.State == nil {
		tm.currentMode.State = make(map[string]interface{})
	}

	tm.currentMode.State[key] = value
}

// ListModes returns a list of all registered modes.
func (tm *TUIManager) ListModes() []string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	modes := make([]string, 0, len(tm.modes))
	for name := range tm.modes {
		modes = append(modes, name)
	}
	return modes
}

// ListCommands returns a list of available commands.
func (tm *TUIManager) ListCommands() []Command {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	commands := make([]Command, 0, len(tm.commands))
	for _, cmd := range tm.commands {
		commands = append(commands, cmd)
	}

	// Add current mode commands
	if tm.currentMode != nil {
		tm.currentMode.mu.RLock()
		for _, cmd := range tm.currentMode.Commands {
			commands = append(commands, cmd)
		}
		tm.currentMode.mu.RUnlock()
	}

	return commands
}

// Run starts the TUI manager.
func (tm *TUIManager) Run() {
	writer := &syncWriter{tm.output}
	// Prominent, unavoidable warning: this TUI is ephemeral and does not persist state
	fmt.Fprintln(writer, "================================================================")
	fmt.Fprintln(writer, "WARNING: EPHEMERAL SESSION - nothing is persisted. Your work will be lost on exit.")
	fmt.Fprintln(writer, "Save or export anything you need BEFORE quitting.")
	fmt.Fprintln(writer, "================================================================")
	fmt.Fprintln(writer, "one-shot-man Rich TUI Terminal")
	fmt.Fprintln(writer, "Type 'help' for available commands, 'exit' to quit")
	modes := tm.ListModes()
	fmt.Fprintf(writer, "Available modes: %s\n", strings.Join(modes, ", "))

	fmt.Fprintln(writer, "Starting advanced go-prompt interface")
	tm.runAdvancedPrompt()
}

// runAdvancedPrompt runs a go-prompt instance with default configuration.
func (tm *TUIManager) runAdvancedPrompt() {
	// Create a default completer that provides command completion
	completer := func(document prompt.Document) ([]prompt.Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		suggestions := tm.getDefaultCompletionSuggestions(document)
		before := document.TextBeforeCursor()
		currWord := currentWord(before)
		start := runeIndex(before) - runeLen(currWord)
		end := runeIndex(before)
		return suggestions, istrings.RuneNumber(start), istrings.RuneNumber(end)
	}

	// Create the executor function
	executor := func(line string) {
		if !tm.executor(line) {
			// If executor returns false, exit the prompt
			os.Exit(0)
		}
	}

	// Configure prompt options - full configuration for go-prompt
	colors := tm.defaultColors
	options := []prompt.Option{
		prompt.WithPrefix(tm.getPromptString()),
		prompt.WithCompleter(completer),
		prompt.WithInputTextColor(colors.InputText),
		prompt.WithPrefixTextColor(colors.PrefixText),
		prompt.WithSuggestionTextColor(colors.SuggestionText),
		prompt.WithSuggestionBGColor(colors.SuggestionBG),
		prompt.WithSelectedSuggestionTextColor(colors.SelectedSuggestionText),
		prompt.WithSelectedSuggestionBGColor(colors.SelectedSuggestionBG),
		prompt.WithDescriptionTextColor(colors.DescriptionText),
		prompt.WithDescriptionBGColor(colors.DescriptionBG),
		prompt.WithSelectedDescriptionTextColor(colors.SelectedDescriptionText),
		prompt.WithSelectedDescriptionBGColor(colors.SelectedDescriptionBG),
	}

	// Add default history support
	defaultHistoryFile := ".one-shot-man_history"
	if history := loadHistory(defaultHistoryFile); len(history) > 0 {
		options = append(options, prompt.WithHistory(history))
	}

	// Add any registered key bindings
	if keyBinds := tm.buildKeyBinds(); len(keyBinds) > 0 {
		options = append(options, prompt.WithKeyBind(keyBinds...))
	}

	// Create and run the prompt
	p := prompt.New(executor, options...)

	// Store as active prompt
	tm.mu.Lock()
	tm.activePrompt = p
	tm.mu.Unlock()

	// Run the prompt (this will block until exit)
	p.Run()

	// Clear active prompt when done
	tm.mu.Lock()
	tm.activePrompt = nil
	tm.mu.Unlock()
}

// syncWriter wraps an io.Writer and calls Sync if it's an *os.File
type syncWriter struct {
	io.Writer
}

func (w *syncWriter) Write(p []byte) (n int, err error) {
	n, err = w.Writer.Write(p)
	if f, ok := w.Writer.(*os.File); ok {
		f.Sync()
	}
	return
}

// executor handles command execution.
func (tm *TUIManager) executor(input string) bool {
	input = strings.TrimSpace(input)
	if input == "" {
		return true
	}

	// Parse command and arguments
	parts := strings.Fields(input)
	cmdName := parts[0]
	args := parts[1:]

	// Handle special cases
	switch cmdName {
	case "exit", "quit":
		// Exit current mode if any
		if tm.currentMode != nil && tm.currentMode.OnExit != nil {
			if _, err := tm.currentMode.OnExit(goja.Undefined()); err != nil {
				fmt.Fprintf(tm.output, "Error exiting mode %s: %v\n", tm.currentMode.Name, err)
			}
		}
		fmt.Fprintln(tm.output, "Goodbye!")
		return false
	case "help":
		tm.showHelp()
		return true
	}

	// Try to execute command
	if err := tm.ExecuteCommand(cmdName, args); err != nil {
		// If not a command, try to execute as JavaScript in current mode
		if tm.currentMode != nil {
			tm.executeJavaScript(input)
		} else {
			fmt.Fprintf(tm.output, "Command not found: %s\n", cmdName)
			fmt.Fprintln(tm.output, "Type 'help' for available commands or switch to a mode to execute JavaScript")
		}
	}
	return true
}

// getPromptString returns the current prompt string.
func (tm *TUIManager) getPromptString() string {
	if tm.currentMode != nil {
		if tm.currentMode.TUIConfig != nil && tm.currentMode.TUIConfig.Prompt != "" {
			return tm.currentMode.TUIConfig.Prompt
		}
		return fmt.Sprintf("[%s]> ", tm.currentMode.Name)
	}
	return ">>> "
}

// executeJavaScript executes JavaScript code in the current mode context.
func (tm *TUIManager) executeJavaScript(code string) {
	if tm.currentMode == nil {
		fmt.Fprintln(tm.output, "No active mode for JavaScript execution")
		return
	}

	// Create a temporary script with the current mode's context
	script := tm.engine.LoadScriptFromString(fmt.Sprintf("%s-repl", tm.currentMode.Name), code)

	// Execute with mode state available
	if err := tm.engine.ExecuteScript(script); err != nil {
		fmt.Fprintf(tm.output, "Error: %v\n", err)
	}
}

// showHelp displays help information.
func (tm *TUIManager) showHelp() {
	fmt.Fprintln(tm.output, "Available commands:")
	fmt.Fprintln(tm.output, "  help                 - Show this help message")
	fmt.Fprintln(tm.output, "  exit, quit           - Exit the terminal")
	fmt.Fprintln(tm.output, "  mode <name>          - Switch to a mode")
	fmt.Fprintln(tm.output, "  modes                - List available modes")
	fmt.Fprintln(tm.output, "  state                - Show current mode state")
	fmt.Fprintln(tm.output, "")

	commands := tm.ListCommands()
	if len(commands) > 0 {
		fmt.Fprintln(tm.output, "Registered commands:")
		for _, cmd := range commands {
			fmt.Fprintf(tm.output, "  %-20s - %s\n", cmd.Name, cmd.Description)
			if cmd.Usage != "" {
				fmt.Fprintf(tm.output, "    Usage: %s\n", cmd.Usage)
			}
		}
		fmt.Fprintln(tm.output, "")
	}

	// Show loaded scripts
	scripts := tm.engine.GetScripts()
	if len(scripts) > 0 {
		fmt.Fprintf(tm.output, "Loaded scripts: %d\n", len(scripts))
	}

	if tm.currentMode != nil {
		fmt.Fprintf(tm.output, "Current mode: %s\n", tm.currentMode.Name)
		fmt.Fprintln(tm.output, "You can execute JavaScript code directly")
		fmt.Fprintln(tm.output, "")
		fmt.Fprintln(tm.output, "JavaScript API:")
		fmt.Fprintln(tm.output, "  ctx.run(name, fn)    - Run a sub-test")
		fmt.Fprintln(tm.output, "  ctx.defer(fn)        - Defer function execution")
		fmt.Fprintln(tm.output, "  ctx.log(...)         - Log a message")
		fmt.Fprintln(tm.output, "  ctx.logf(fmt, ...)   - Log a formatted message")
	} else {
		fmt.Fprintf(tm.output, "Available modes: %s\n", strings.Join(tm.ListModes(), ", "))
		fmt.Fprintln(tm.output, "Switch to a mode to execute JavaScript code")
	}
}

// registerBuiltinCommands registers the built-in commands.
func (tm *TUIManager) registerBuiltinCommands() {
	// Mode switching command
	tm.RegisterCommand(Command{
		Name:        "mode",
		Description: "Switch to a different mode",
		Usage:       "mode <mode-name>",
		Handler: func(args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("usage: mode <mode-name>")
			}
			err := tm.SwitchMode(args[0])
			if err != nil {
				fmt.Fprintf(tm.output, "mode %s not found\n", args[0])
				return nil // Don't return error to avoid "Command not found"
			}
			return nil
		},
		IsGoCommand: true,
	})

	// List modes command
	tm.RegisterCommand(Command{
		Name:        "modes",
		Description: "List all available modes",
		Handler: func(args []string) error {
			modes := tm.ListModes()
			if len(modes) == 0 {
				fmt.Fprintln(tm.output, "No modes registered")
			} else {
				fmt.Fprintf(tm.output, "Available modes: %s\n", strings.Join(modes, ", "))
				if tm.currentMode != nil {
					fmt.Fprintf(tm.output, "Current mode: %s\n", tm.currentMode.Name)
				}
			}
			return nil
		},
		IsGoCommand: true,
	})

	// State command
	tm.RegisterCommand(Command{
		Name:        "state",
		Description: "Show current mode state",
		Handler: func(args []string) error {
			if tm.currentMode == nil {
				fmt.Fprintln(tm.output, "No active mode")
				return nil
			}

			tm.currentMode.mu.RLock()
			defer tm.currentMode.mu.RUnlock()

			fmt.Fprintf(tm.output, "Mode: %s\n", tm.currentMode.Name)
			if len(tm.currentMode.State) == 0 {
				fmt.Fprintln(tm.output, "State: empty")
			} else {
				fmt.Fprintln(tm.output, "State:")
				for key, value := range tm.currentMode.State {
					fmt.Fprintf(tm.output, "  %s: %v\n", key, value)
				}
			}
			return nil
		},
		IsGoCommand: true,
	})
}

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

// Helper types and functions for prompt configuration

// PromptColors represents color configuration for a prompt.
type PromptColors struct {
	InputText               prompt.Color
	PrefixText              prompt.Color
	SuggestionText          prompt.Color
	SuggestionBG            prompt.Color
	SelectedSuggestionText  prompt.Color
	SelectedSuggestionBG    prompt.Color
	DescriptionText         prompt.Color
	DescriptionBG           prompt.Color
	SelectedDescriptionText prompt.Color
	SelectedDescriptionBG   prompt.Color
	ScrollbarThumb          prompt.Color
	ScrollbarBG             prompt.Color
}

// HistoryConfig represents history configuration for a prompt.
type HistoryConfig struct {
	Enabled bool
	File    string
	Size    int
}

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
		// Built-in commands
		builtinCommands := []string{"help", "exit", "quit", "mode", "modes", "state"}
		for _, cmd := range builtinCommands {
			if strings.HasPrefix(cmd, currentWord) {
				suggestions = append(suggestions, prompt.Suggest{
					Text:        cmd,
					Description: "Built-in command",
				})
			}
		}

		// Registered commands
		tm.mu.RLock()
		for _, cmd := range tm.commands {
			if strings.HasPrefix(cmd.Name, currentWord) {
				suggestions = append(suggestions, prompt.Suggest{
					Text:        cmd.Name,
					Description: cmd.Description,
				})
			}
		}

		// Current mode commands
		if tm.currentMode != nil {
			tm.currentMode.mu.RLock()
			for _, cmd := range tm.currentMode.Commands {
				if strings.HasPrefix(cmd.Name, currentWord) {
					suggestions = append(suggestions, prompt.Suggest{
						Text:        cmd.Name,
						Description: cmd.Description,
					})
				}
			}
			tm.currentMode.mu.RUnlock()
		}
		tm.mu.RUnlock()
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
