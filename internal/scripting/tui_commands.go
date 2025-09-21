package scripting

import (
	"fmt"
	"strings"

	"github.com/dop251/goja"
)

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
				_, _ = fmt.Fprintf(tm.output, "Error exiting mode %s: %v\n", tm.currentMode.Name, err)
			}
		}
		_, _ = fmt.Fprintln(tm.output, "Goodbye!")
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
			_, _ = fmt.Fprintf(tm.output, "Command not found: %s\n", cmdName)
			_, _ = fmt.Fprintln(tm.output, "Type 'help' for available commands or switch to a mode to execute JavaScript")
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
		_, _ = fmt.Fprintln(tm.output, "No active mode for JavaScript execution")
		return
	}

	// Create a temporary script with the current mode's context
	script := tm.engine.LoadScriptFromString(fmt.Sprintf("%s-repl", tm.currentMode.Name), code)

	// Execute with mode state available
	if err := tm.engine.ExecuteScript(script); err != nil {
		_, _ = fmt.Fprintf(tm.output, "Error: %v\n", err)
	}
}

// showHelp displays help information.
func (tm *TUIManager) showHelp() {
	_, _ = fmt.Fprintln(tm.output, "Available commands:")
	_, _ = fmt.Fprintln(tm.output, "  help                 - Show this help message")
	_, _ = fmt.Fprintln(tm.output, "  exit, quit           - Exit the terminal")
	_, _ = fmt.Fprintln(tm.output, "  mode <name>          - Switch to a mode")
	_, _ = fmt.Fprintln(tm.output, "  modes                - List available modes")
	_, _ = fmt.Fprintln(tm.output, "  state                - Show current mode state")
	_, _ = fmt.Fprintln(tm.output, "")

	commands := tm.ListCommands()
	if len(commands) > 0 {
		_, _ = fmt.Fprintln(tm.output, "Registered commands:")
		for _, cmd := range commands {
			_, _ = fmt.Fprintf(tm.output, "  %-20s - %s\n", cmd.Name, cmd.Description)
			if cmd.Usage != "" {
				_, _ = fmt.Fprintf(tm.output, "    Usage: %s\n", cmd.Usage)
			}
		}
		_, _ = fmt.Fprintln(tm.output, "")
	}

	// Show loaded scripts
	scripts := tm.engine.GetScripts()
	if len(scripts) > 0 {
		_, _ = fmt.Fprintf(tm.output, "Loaded scripts: %d\n", len(scripts))
	}

	if tm.currentMode != nil {
		_, _ = fmt.Fprintf(tm.output, "Current mode: %s\n", tm.currentMode.Name)
		_, _ = fmt.Fprintln(tm.output, "You can execute JavaScript code directly")
		_, _ = fmt.Fprintln(tm.output, "")
		_, _ = fmt.Fprintln(tm.output, "JavaScript API:")
		_, _ = fmt.Fprintln(tm.output, "  ctx.run(name, fn)    - Run a sub-test")
		_, _ = fmt.Fprintln(tm.output, "  ctx.defer(fn)        - Defer function execution")
		_, _ = fmt.Fprintln(tm.output, "  ctx.log(...)         - Log a message")
		_, _ = fmt.Fprintln(tm.output, "  ctx.logf(fmt, ...)   - Log a formatted message")
	} else {
		_, _ = fmt.Fprintf(tm.output, "Available modes: %s\n", strings.Join(tm.ListModes(), ", "))
		_, _ = fmt.Fprintln(tm.output, "Switch to a mode to execute JavaScript code")
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
				_, _ = fmt.Fprintf(tm.output, "mode %s not found\n", args[0])
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
				_, _ = fmt.Fprintln(tm.output, "No modes registered")
			} else {
				_, _ = fmt.Fprintf(tm.output, "Available modes: %s\n", strings.Join(modes, ", "))
				if tm.currentMode != nil {
					_, _ = fmt.Fprintf(tm.output, "Current mode: %s\n", tm.currentMode.Name)
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
				_, _ = fmt.Fprintln(tm.output, "No active mode")
				return nil
			}

			tm.currentMode.mu.RLock()
			defer tm.currentMode.mu.RUnlock()

			_, _ = fmt.Fprintf(tm.output, "Mode: %s\n", tm.currentMode.Name)
			if len(tm.currentMode.State) == 0 {
				_, _ = fmt.Fprintln(tm.output, "State: empty")
			} else {
				_, _ = fmt.Fprintln(tm.output, "State:")
				for key, value := range tm.currentMode.State {
					_, _ = fmt.Fprintf(tm.output, "  %s: %v\n", key, value)
				}
			}
			return nil
		},
		IsGoCommand: true,
	})
}
