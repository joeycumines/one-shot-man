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
	parts := tokenizeCommandLine(input)
	cmdName := parts[0]
	args := parts[1:]

	// Handle special cases
	switch cmdName {
	case "exit", "quit":
		// Exit current mode if any
		if tm.currentMode != nil && tm.currentMode.OnExit != nil {
			if _, err := tm.currentMode.OnExit(goja.Undefined()); err != nil {
				_, _ = fmt.Fprintf(tm.writer, "Error exiting mode %s: %v\n", tm.currentMode.Name, err)
			}
		}
		_, _ = fmt.Fprintln(tm.writer, "Goodbye!")
		return false
	case "help":
		tm.showHelp()
		// support printing extra help info
		_ = tm.ExecuteCommand(cmdName, args)
		return true
	}

	// Try to execute command
	if err := tm.ExecuteCommand(cmdName, args); err != nil {
		// If not a command, try to execute as JavaScript in current mode
		if tm.currentMode != nil {
			tm.executeJavaScript(input)
		} else {
			_, _ = fmt.Fprintf(tm.writer, "Command not found: %s\n", cmdName)
			_, _ = fmt.Fprintln(tm.writer, "Type 'help' for available commands or switch to a mode to execute JavaScript")
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

// getInitialCommand returns any [ScriptMode.InitialCommand].
func (tm *TUIManager) getInitialCommand() string {
	if tm.currentMode != nil {
		return tm.currentMode.InitialCommand
	}
	return ``
}

// executeJavaScript executes JavaScript code in the current mode context.
func (tm *TUIManager) executeJavaScript(code string) {
	if tm.currentMode == nil {
		_, _ = fmt.Fprintln(tm.writer, "No active mode for JavaScript execution")
		return
	}

	// Create a temporary script with the current mode's context
	script := tm.engine.LoadScriptFromString(fmt.Sprintf("%s-repl", tm.currentMode.Name), code)

	// Execute with mode state available
	if err := tm.engine.ExecuteScript(script); err != nil {
		_, _ = fmt.Fprintf(tm.writer, "Error: %v\n", err)
	}
}

// showHelp displays help information.
func (tm *TUIManager) showHelp() {
	writer := tm.writer

	_, _ = fmt.Fprintln(writer, "Available commands:")
	_, _ = fmt.Fprintln(writer, "  help                 - Show this help message")
	_, _ = fmt.Fprintln(writer, "  exit, quit           - Exit the terminal")
	_, _ = fmt.Fprintln(writer, "  mode <name>          - Switch to a mode")
	_, _ = fmt.Fprintln(writer, "  modes                - List available modes")
	_, _ = fmt.Fprintln(writer, "  state                - Show current mode state")
	_, _ = fmt.Fprintln(writer, "")

	commands := tm.ListCommands()
	if len(commands) > 0 {
		_, _ = fmt.Fprintln(writer, "Registered commands:")
		for _, cmd := range commands {
			_, _ = fmt.Fprintf(writer, "  %-20s - %s\n", cmd.Name, cmd.Description)
			if cmd.Usage != "" {
				_, _ = fmt.Fprintf(writer, "    Usage: %s\n", cmd.Usage)
			}
		}
		_, _ = fmt.Fprintln(writer, "")
	}

	// Show loaded scripts
	scripts := tm.engine.GetScripts()
	if len(scripts) > 0 {
		_, _ = fmt.Fprintf(writer, "Loaded scripts: %d\n", len(scripts))
	}

	if tm.currentMode != nil {
		_, _ = fmt.Fprintf(writer, "Current mode: %s\n", tm.currentMode.Name)
		_, _ = fmt.Fprintln(writer, "Note: You can execute JavaScript code directly!")
	} else {
		_, _ = fmt.Fprintf(writer, "Available modes: %s\n", strings.Join(tm.ListModes(), ", "))
		_, _ = fmt.Fprintln(writer, "Switch to a mode to execute JavaScript code.")
	}
}

// WARNING: Find usages. If you're adding or adjusting a command with args, you may need completion support.
var builtinCommands = []string{"help", "exit", "quit", "mode", "modes", "state", "reset"}

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
				_, _ = fmt.Fprintf(tm.writer, "mode %s not found\n", args[0])
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
				_, _ = fmt.Fprintln(tm.writer, "No modes registered")
			} else {
				_, _ = fmt.Fprintf(tm.writer, "Available modes: %s\n", strings.Join(modes, ", "))
				if tm.currentMode != nil {
					_, _ = fmt.Fprintf(tm.writer, "Current mode: %s\n", tm.currentMode.Name)
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
				_, _ = fmt.Fprintln(tm.writer, "No active mode")
				return nil
			}

			tm.currentMode.mu.RLock()
			defer tm.currentMode.mu.RUnlock()

			_, _ = fmt.Fprintf(tm.writer, "Mode: %s\n", tm.currentMode.Name)
			// State is now managed by StateManager, not directly on mode
			_, _ = fmt.Fprintln(tm.writer, "State: managed by StateManager (use history to view state snapshots)")
			return nil
		},
		IsGoCommand: true,
	})

	// Reset command
	tm.RegisterCommand(Command{
		Name:        "reset",
		Description: "Reset all shared and mode-specific state to default values",
		Usage:       "reset",
		Handler: func(args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("usage: reset (takes no arguments)")
			}

			// Call the new reset logic
			tm.resetAllState()

			_, _ = fmt.Fprintln(tm.writer, "All shared and mode-specific state has been reset to default values.")
			return nil
		},
		IsGoCommand: true,
	})
}
