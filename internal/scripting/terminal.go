package scripting

import (
	"context"
	"fmt"
	"strings"

	"github.com/elk-language/go-prompt"
)

// Terminal provides interactive terminal capabilities for the scripting engine.
type Terminal struct {
	engine      *Engine
	prompt      *prompt.Prompt
	ctx         context.Context
	suggestions []prompt.Suggest
	history     []string
}

// NewTerminal creates a new terminal interface for the scripting engine.
func NewTerminal(ctx context.Context, engine *Engine) *Terminal {
	terminal := &Terminal{
		engine:      engine,
		ctx:         ctx,
		suggestions: getDefaultSuggestions(),
		history:     make([]string, 0),
	}
	
	// Use a simple prompt without complex completion for now
	terminal.prompt = prompt.New(
		terminal.executor,
		prompt.WithTitle("one-shot-man scripting terminal"),
		prompt.WithPrefix(">>> "),
	)
	
	return terminal
}

// Run starts the interactive terminal.
func (t *Terminal) Run() {
	fmt.Println("one-shot-man JavaScript Scripting Terminal")
	fmt.Println("Type 'help' for available commands, 'exit' to quit")
	fmt.Println()
	
	t.prompt.Run()
}

// executor handles command execution in the terminal.
func (t *Terminal) executor(input string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return
	}
	
	// Add to history
	t.history = append(t.history, input)
	
	// Handle special commands
	switch {
	case input == "exit" || input == "quit":
		fmt.Println("Goodbye!")
		return
	case input == "help":
		t.showHelp()
		return
	case input == "clear":
		// Clear screen - in a real implementation, you'd use terminal escape codes
		fmt.Println("\033c")
		return
	case input == "history":
		t.showHistory()
		return
	case strings.HasPrefix(input, "load "):
		scriptName := strings.TrimSpace(input[5:])
		t.loadScript(scriptName)
		return
	case input == "scripts":
		t.listScripts()
		return
	case strings.HasPrefix(input, "run "):
		scriptName := strings.TrimSpace(input[4:])
		t.runScript(scriptName)
		return
	}
	
	// Execute JavaScript code
	t.executeJavaScript(input)
}

// executeJavaScript executes JavaScript code directly.
func (t *Terminal) executeJavaScript(code string) {
	// Create a temporary script
	script := t.engine.LoadScriptFromString("terminal", code)
	
	// Execute the script
	if err := t.engine.ExecuteScript(script); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

// loadScript loads a script by name.
func (t *Terminal) loadScript(name string) {
	// In a real implementation, you'd look up the script path
	fmt.Printf("Loading script: %s\n", name)
	// This is a placeholder implementation
}

// runScript runs a loaded script by name.
func (t *Terminal) runScript(name string) {
	for _, script := range t.engine.GetScripts() {
		if script.Name == name {
			fmt.Printf("Running script: %s\n", name)
			if err := t.engine.ExecuteScript(&script); err != nil {
				fmt.Printf("Error running script: %v\n", err)
			}
			return
		}
	}
	fmt.Printf("Script not found: %s\n", name)
}

// listScripts lists all loaded scripts.
func (t *Terminal) listScripts() {
	scripts := t.engine.GetScripts()
	if len(scripts) == 0 {
		fmt.Println("No scripts loaded")
		return
	}
	
	fmt.Println("Loaded scripts:")
	for _, script := range scripts {
		fmt.Printf("  %s - %s\n", script.Name, script.Description)
	}
}

// showHelp displays help information.
func (t *Terminal) showHelp() {
	fmt.Println("Available commands:")
	fmt.Println("  help                 - Show this help message")
	fmt.Println("  exit, quit           - Exit the terminal")
	fmt.Println("  clear                - Clear the screen")
	fmt.Println("  history              - Show command history")
	fmt.Println("  scripts              - List loaded scripts")
	fmt.Println("  load <script>        - Load a script")
	fmt.Println("  run <script>         - Run a loaded script")
	fmt.Println("  <javascript>         - Execute JavaScript code")
	fmt.Println()
	fmt.Println("JavaScript API:")
	fmt.Println("  ctx.run(name, fn)    - Run a sub-test")
	fmt.Println("  ctx.defer(fn)        - Defer function execution")
	fmt.Println("  ctx.log(...)         - Log a message")
	fmt.Println("  ctx.logf(fmt, ...)   - Log a formatted message")
	fmt.Println("  console.log(...)     - Console logging")
	fmt.Println("  sleep(ms)            - Sleep for milliseconds")
	fmt.Println("  env(key)             - Get environment variable")
}

// showHistory displays command history.
func (t *Terminal) showHistory() {
	if len(t.history) == 0 {
		fmt.Println("No command history")
		return
	}
	
	fmt.Println("Command history:")
	for i, cmd := range t.history {
		fmt.Printf("  %d: %s\n", i+1, cmd)
	}
}

// getDefaultSuggestions returns default auto-completion suggestions.
func getDefaultSuggestions() []prompt.Suggest {
	return []prompt.Suggest{
		{Text: "help", Description: "Show help message"},
		{Text: "exit", Description: "Exit the terminal"},
		{Text: "quit", Description: "Exit the terminal"},
		{Text: "clear", Description: "Clear the screen"},
		{Text: "history", Description: "Show command history"},
		{Text: "scripts", Description: "List loaded scripts"},
		{Text: "load", Description: "Load a script"},
		{Text: "run", Description: "Run a script"},
		
		// JavaScript API suggestions
		{Text: "ctx.run(", Description: "Run a sub-test"},
		{Text: "ctx.defer(", Description: "Defer function execution"},
		{Text: "ctx.log(", Description: "Log a message"},
		{Text: "ctx.logf(", Description: "Log a formatted message"},
		{Text: "console.log(", Description: "Console logging"},
		{Text: "sleep(", Description: "Sleep for milliseconds"},
		{Text: "env(", Description: "Get environment variable"},
	}
}