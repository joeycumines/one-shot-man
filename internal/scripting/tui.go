package scripting

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/elk-language/go-prompt"
	istrings "github.com/elk-language/go-prompt/strings"
)

// TUIManager manages rich terminal interfaces for script modes.
type TUIManager struct {
	engine       *Engine
	ctx          context.Context
	currentMode  *ScriptMode
	modes        map[string]*ScriptMode
	commands     map[string]Command
	mu           sync.RWMutex
	output       io.Writer
	prompts      map[string]*prompt.Prompt // Manages named prompt instances
	activePrompt *prompt.Prompt            // Pointer to the currently active prompt
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
func NewTUIManager(ctx context.Context, engine *Engine) *TUIManager {
	manager := &TUIManager{
		engine:   engine,
		ctx:      ctx,
		modes:    make(map[string]*ScriptMode),
		commands: make(map[string]Command),
		output:   engine.stdout,
		prompts:  make(map[string]*prompt.Prompt),
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
	time.Sleep(1 * time.Second)
	writer := &syncWriter{os.Stdout}
	fmt.Fprintln(writer, "one-shot-man Rich TUI Terminal")
	fmt.Fprintln(writer, "Type 'help' for available commands, 'exit' to quit")
	modes := tm.ListModes()
	fmt.Fprintf(writer, "Available modes: %s\n", strings.Join(modes, ", "))

	// Use go-prompt instead of simple loop
	tm.runAdvancedPrompt()
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

// runAdvancedPrompt runs the main prompt using go-prompt
func (tm *TUIManager) runAdvancedPrompt() {
	// Create completer function that wraps the current mode's completion logic
	completer := func(d prompt.Document) ([]prompt.Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		suggestions := tm.getCompletions(d)
		// Return suggestions with start and end positions (simplified)
		word := d.GetWordBeforeCursor()
		startChar := istrings.RuneNumber(len(d.TextBeforeCursor()) - len(word))
		endChar := istrings.RuneNumber(len(d.TextBeforeCursor()))
		return suggestions, startChar, endChar
	}

	// Create options with basic configuration
	options := []prompt.Option{
		prompt.WithPrefix(tm.getPromptString()),
		prompt.WithTitle("one-shot-man"),
		prompt.WithHistory(tm.getHistory()),
		prompt.WithPrefixTextColor(prompt.Cyan),
		prompt.WithSuggestionTextColor(prompt.Blue),
		prompt.WithSelectedSuggestionBGColor(prompt.LightGray),
		prompt.WithSuggestionBGColor(prompt.DarkGray),
		prompt.WithCompleter(completer),
		prompt.WithExecuteOnEnterCallback(func(prompt *prompt.Prompt, indentSize int) (int, bool) {
			// Always execute on Enter, don't wait for more input
			return 0, true
		}),
	}

	// Create and run the prompt
	p := prompt.New(
		tm.promptExecutor,
		options...,
	)

	tm.activePrompt = p
	p.Run()
}

// promptExecutor handles command execution from go-prompt
func (tm *TUIManager) promptExecutor(input string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return
	}

	// Save to history if enabled
	tm.saveToHistory(input)

	// Handle special cases
	switch input {
	case "exit", "quit":
		// Exit current mode if any
		if tm.currentMode != nil && tm.currentMode.OnExit != nil {
			if _, err := tm.currentMode.OnExit(goja.Undefined()); err != nil {
				fmt.Fprintf(tm.output, "Error exiting mode %s: %v\n", tm.currentMode.Name, err)
			}
		}
		fmt.Fprintln(tm.output, "Goodbye!")
		os.Exit(0)
	case "help":
		tm.showHelp()
		return
	}

	// Parse command and arguments
	parts := strings.Fields(input)
	cmdName := parts[0]
	args := parts[1:]

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
}

// getCompletions provides completion suggestions for go-prompt
func (tm *TUIManager) getCompletions(d prompt.Document) []prompt.Suggest {
	var suggestions []prompt.Suggest

	// Get the word being completed
	word := d.GetWordBeforeCursor()

	// Add built-in commands
	builtinCommands := []string{"help", "exit", "quit", "mode", "modes", "state"}
	for _, cmd := range builtinCommands {
		if strings.HasPrefix(cmd, word) {
			suggestions = append(suggestions, prompt.Suggest{
				Text:        cmd,
				Description: "Built-in command",
			})
		}
	}

	// Add registered commands
	tm.mu.RLock()
	for name, cmd := range tm.commands {
		if strings.HasPrefix(name, word) {
			suggestions = append(suggestions, prompt.Suggest{
				Text:        name,
				Description: cmd.Description,
			})
		}
	}

	// Add current mode commands
	if tm.currentMode != nil {
		tm.currentMode.mu.RLock()
		for name, cmd := range tm.currentMode.Commands {
			if strings.HasPrefix(name, word) {
				suggestions = append(suggestions, prompt.Suggest{
					Text:        name,
					Description: cmd.Description,
				})
			}
		}
		tm.currentMode.mu.RUnlock()

		// Call JavaScript completion function if available
		if tm.currentMode.TUIConfig != nil && tm.currentMode.TUIConfig.CompletionFn != nil {
			if jsSuggestions := tm.callJavaScriptCompleter(d); jsSuggestions != nil {
				suggestions = append(suggestions, jsSuggestions...)
			}
		}
	}
	tm.mu.RUnlock()

	return suggestions
}

// callJavaScriptCompleter calls the JavaScript completion function
func (tm *TUIManager) callJavaScriptCompleter(d prompt.Document) []prompt.Suggest {
	if tm.currentMode == nil || tm.currentMode.TUIConfig == nil || tm.currentMode.TUIConfig.CompletionFn == nil {
		return nil
	}

	// Create a document object for JavaScript
	docObj := tm.engine.vm.NewObject()
	docObj.Set("getWordBeforeCursor", func() string { return d.GetWordBeforeCursor() })
	docObj.Set("getCurrentWord", func() string { return d.GetWordBeforeCursor() })
	docObj.Set("getText", func() string { return d.Text })
	docObj.Set("getCurrentLine", func() string { return d.CurrentLine() })

	// Call the JavaScript completer
	result, err := tm.currentMode.TUIConfig.CompletionFn(goja.Undefined(), docObj)
	if err != nil {
		return nil
	}

	// Convert JavaScript result to suggestions
	var suggestions []prompt.Suggest
	if resultObj, ok := result.Export().([]interface{}); ok {
		for _, item := range resultObj {
			if itemMap, ok := item.(map[string]interface{}); ok {
				suggestion := prompt.Suggest{
					Text:        getString(itemMap, "text", ""),
					Description: getString(itemMap, "description", ""),
				}
				if suggestion.Text != "" {
					suggestions = append(suggestions, suggestion)
				}
			}
		}
	}

	return suggestions
}

// getHistory returns command history for the prompt
func (tm *TUIManager) getHistory() []string {
	// For now, return empty history. This will be enhanced later with file-based persistence
	// TODO: Implement history loading from HistoryFile if EnableHistory is true
	return []string{}
}

// saveToHistory saves a command to history
func (tm *TUIManager) saveToHistory(input string) {
	// For now, this is a no-op. This will be enhanced later with file-based persistence
	// TODO: Implement history saving to HistoryFile if EnableHistory is true
}

// runSimpleLoop runs a simple input loop for testing compatibility.
func (tm *TUIManager) runSimpleLoop() {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Fprint(tm.output, tm.getPromptString())

		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		if !tm.executor(input) {
			break
		}
	}
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

// jsCreateAdvancedPrompt creates a new advanced prompt instance from JavaScript configuration
func (tm *TUIManager) jsCreateAdvancedPrompt(config interface{}) string {
	if configMap, ok := config.(map[string]interface{}); ok {
		// Generate a unique name for this prompt
		name := getString(configMap, "name", fmt.Sprintf("prompt-%d", len(tm.prompts)))
		
		// Create completer function if provided
		var completer prompt.Completer
		if completionFn, exists := configMap["completer"]; exists {
			if val := tm.engine.vm.ToValue(completionFn); val != nil {
				if callable, ok := goja.AssertFunction(val); ok {
					completer = func(d prompt.Document) ([]prompt.Suggest, istrings.RuneNumber, istrings.RuneNumber) {
						// Create document object for JavaScript
						docObj := tm.engine.vm.NewObject()
						docObj.Set("getWordBeforeCursor", func() string { return d.GetWordBeforeCursor() })
						docObj.Set("getText", func() string { return d.Text })
						docObj.Set("getCurrentLine", func() string { return d.CurrentLine() })

						// Call JavaScript completer
						result, err := callable(goja.Undefined(), docObj)
						if err != nil {
							return []prompt.Suggest{}, 0, 0
						}

						// Convert result to suggestions
						var suggestions []prompt.Suggest
						if resultObj, ok := result.Export().([]interface{}); ok {
							for _, item := range resultObj {
								if itemMap, ok := item.(map[string]interface{}); ok {
									suggestion := prompt.Suggest{
										Text:        getString(itemMap, "text", ""),
										Description: getString(itemMap, "description", ""),
									}
									if suggestion.Text != "" {
										suggestions = append(suggestions, suggestion)
									}
								}
							}
						}
						
						// Simple word completion boundaries
						word := d.GetWordBeforeCursor()
						startChar := istrings.RuneNumber(len(d.TextBeforeCursor()) - len(word))
						endChar := istrings.RuneNumber(len(d.TextBeforeCursor()))
						return suggestions, startChar, endChar
					}
				}
			}
		}

		// Create prompt options
		options := []prompt.Option{
			prompt.WithPrefix(getString(configMap, "prefix", ">>> ")),
			prompt.WithTitle(getString(configMap, "title", "Advanced Prompt")),
		}

		// Add completer if provided
		if completer != nil {
			options = append(options, prompt.WithCompleter(completer))
		}

		// Add color options if provided
		if colors, exists := configMap["colors"]; exists {
			if colorMap, ok := colors.(map[string]interface{}); ok {
				// Map color names to go-prompt colors (basic implementation)
				if prefixColor := getString(colorMap, "prefix", ""); prefixColor != "" {
					if color := tm.parseColor(prefixColor); color != prompt.DefaultColor {
						options = append(options, prompt.WithPrefixTextColor(color))
					}
				}
			}
		}

		// Create executor function
		executor := func(input string) {
			tm.promptExecutor(input)
		}

		// Create the prompt
		p := prompt.New(executor, options...)

		// Store the prompt
		tm.mu.Lock()
		tm.prompts[name] = p
		tm.mu.Unlock()

		return name
	}

	return ""
}

// parseColor converts a color string to go-prompt color
func (tm *TUIManager) parseColor(colorName string) prompt.Color {
	switch strings.ToLower(colorName) {
	case "black":
		return prompt.Black
	case "red":
		return prompt.Red
	case "green":
		return prompt.Green
	case "yellow":
		return prompt.Yellow
	case "blue":
		return prompt.Blue
	case "purple":
		return prompt.Purple
	case "fuchsia":
		return prompt.Fuchsia
	case "cyan":
		return prompt.Cyan
	case "turquoise":
		return prompt.Turquoise
	case "white":
		return prompt.White
	case "lightgray":
		return prompt.LightGray
	case "darkgray":
		return prompt.DarkGray
	case "darkred":
		return prompt.DarkRed
	case "darkgreen":
		return prompt.DarkGreen
	case "brown":
		return prompt.Brown
	case "darkblue":
		return prompt.DarkBlue
	default:
		return prompt.DefaultColor
	}
}

// jsRunPrompt runs a named prompt
func (tm *TUIManager) jsRunPrompt(name string) string {
	tm.mu.RLock()
	p, exists := tm.prompts[name]
	tm.mu.RUnlock()

	if !exists {
		return ""
	}

	// Set as active prompt and run
	tm.activePrompt = p
	return p.Input()
}

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
