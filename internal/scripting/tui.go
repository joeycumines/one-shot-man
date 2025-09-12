package scripting

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/dop251/goja"
	"github.com/elk-language/go-prompt"
)

// TUIManager manages rich terminal interfaces for script modes.
type TUIManager struct {
	engine      *Engine
	ctx         context.Context
	currentMode *ScriptMode
	modes       map[string]*ScriptMode
	commands    map[string]Command
	mu          sync.RWMutex
	prompt      *prompt.Prompt
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
	CustomSuggest []prompt.Suggest
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
			fmt.Printf("Error exiting mode %s: %v\n", tm.currentMode.Name, err)
		}
	}

	// Enter new mode
	tm.currentMode = mode
	if mode.OnEnter != nil {
		if _, err := mode.OnEnter(goja.Undefined()); err != nil {
			fmt.Printf("Error entering mode %s: %v\n", mode.Name, err)
		}
	}

	fmt.Printf("Switched to mode: %s\n", mode.Name)
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

// GetState gets a state value for the current mode.
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
	fmt.Println("one-shot-man Rich TUI Terminal")
	fmt.Println("Type 'help' for available commands, 'exit' to quit")
	fmt.Printf("Available modes: %s\n", strings.Join(tm.ListModes(), ", "))
	fmt.Println()

	// Create prompt with basic options for now
	tm.prompt = prompt.New(
		tm.executor,
		prompt.WithTitle("one-shot-man rich TUI"),
	)

	tm.prompt.Run()
}

// executor handles command execution.
func (tm *TUIManager) executor(input string) {
	// Print the current prompt prefix
	fmt.Print(tm.getPromptString())

	input = strings.TrimSpace(input)
	if input == "" {
		return
	}

	// Parse command and arguments
	parts := strings.Fields(input)
	cmdName := parts[0]
	args := parts[1:]

	// Handle special cases
	switch cmdName {
	case "exit", "quit":
		fmt.Println("Goodbye!")
		return
	case "help":
		tm.showHelp()
		return
	}

	// Try to execute command
	if err := tm.ExecuteCommand(cmdName, args); err != nil {
		// If not a command, try to execute as JavaScript in current mode
		if tm.currentMode != nil {
			tm.executeJavaScript(input)
		} else {
			fmt.Printf("Command not found: %s\n", cmdName)
			fmt.Println("Type 'help' for available commands or switch to a mode to execute JavaScript")
		}
	}
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
		fmt.Println("No active mode for JavaScript execution")
		return
	}

	// Create a temporary script with the current mode's context
	script := tm.engine.LoadScriptFromString(fmt.Sprintf("%s-repl", tm.currentMode.Name), code)

	// Execute with mode state available
	if err := tm.engine.ExecuteScript(script); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

// showHelp displays help information.
func (tm *TUIManager) showHelp() {
	fmt.Println("Available commands:")
	fmt.Println("  help                 - Show this help message")
	fmt.Println("  exit, quit           - Exit the terminal")
	fmt.Println("  mode <name>          - Switch to a mode")
	fmt.Println("  modes                - List available modes")
	fmt.Println("  state                - Show current mode state")
	fmt.Println()

	commands := tm.ListCommands()
	if len(commands) > 0 {
		fmt.Println("Registered commands:")
		for _, cmd := range commands {
			fmt.Printf("  %-20s - %s\n", cmd.Name, cmd.Description)
			if cmd.Usage != "" {
				fmt.Printf("    Usage: %s\n", cmd.Usage)
			}
		}
		fmt.Println()
	}

	// Show loaded scripts
	scripts := tm.engine.GetScripts()
	if len(scripts) > 0 {
		fmt.Printf("Loaded scripts: %d\n", len(scripts))
	}

	if tm.currentMode != nil {
		fmt.Printf("Current mode: %s\n", tm.currentMode.Name)
		fmt.Println("You can execute JavaScript code directly")
		fmt.Println()
		fmt.Println("JavaScript API:")
		fmt.Println("  ctx.run(name, fn)    - Run a sub-test")
		fmt.Println("  ctx.defer(fn)        - Defer function execution")
		fmt.Println("  ctx.log(...)         - Log a message")
		fmt.Println("  ctx.logf(fmt, ...)   - Log a formatted message")
	} else {
		fmt.Printf("Available modes: %s\n", strings.Join(tm.ListModes(), ", "))
		fmt.Println("Switch to a mode to execute JavaScript code")
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
			return tm.SwitchMode(args[0])
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
				fmt.Println("No modes registered")
			} else {
				fmt.Printf("Available modes: %s\n", strings.Join(modes, ", "))
				if tm.currentMode != nil {
					fmt.Printf("Current mode: %s\n", tm.currentMode.Name)
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
				fmt.Println("No active mode")
				return nil
			}

			tm.currentMode.mu.RLock()
			defer tm.currentMode.mu.RUnlock()

			fmt.Printf("Mode: %s\n", tm.currentMode.Name)
			if len(tm.currentMode.State) == 0 {
				fmt.Println("State: empty")
			} else {
				fmt.Println("State:")
				for key, value := range tm.currentMode.State {
					fmt.Printf("  %s: %v\n", key, value)
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
