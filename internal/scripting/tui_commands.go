package scripting

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dop251/goja"
)

// ErrCommandNotFound is returned by [TUIManager.ExecuteCommand] when no
// command with the given name exists in either the active mode or the
// global command registry.
var ErrCommandNotFound = errors.New("command not found")

// print writes a message to the terminal, routing through the logger's
// PrintToTUI (which respects the TUI sink and writes to raw stdout when
// the sink is nil) when the engine is available. Falls back to tm.writer
// with an explicit Flush for test-only TUIManager instances without an engine.
func (tm *TUIManager) print(msg string) {
	if tm.engine != nil && tm.engine.logger != nil {
		tm.engine.logger.PrintToTUI(msg)
		return
	}
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}
	_, _ = fmt.Fprint(tm.writer, msg)
	_ = tm.writer.Flush()
}

func (tm *TUIManager) printf(format string, args ...any) {
	tm.print(fmt.Sprintf(format, args...))
}

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
				tm.printf("Error exiting mode %s: %v", tm.currentMode.Name, err)
			}
		}
		tm.print("Goodbye!")
		return false
	case "help":
		tm.showHelp()
		// support printing extra help info
		_ = tm.ExecuteCommand(cmdName, args)
		return true
	}

	// Try to execute command
	if err := tm.ExecuteCommand(cmdName, args); err != nil {
		if errors.Is(err, ErrCommandNotFound) {
			// Command not found — try JavaScript execution in current mode
			if tm.currentMode != nil {
				tm.executeJavaScript(input)
			} else {
				tm.printf("Command not found: %s", cmdName)
				tm.print("Type 'help' for available commands or switch to a mode to execute JavaScript")
			}
		} else {
			// Command was found but the handler itself returned an error
			tm.printf("Error: %v", err)
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
		tm.print("No active mode for JavaScript execution")
		return
	}

	// Create a temporary script with the current mode's context
	script := tm.engine.LoadScriptFromString(fmt.Sprintf("%s-repl", tm.currentMode.Name), code)

	// Execute with mode state available
	if err := tm.engine.ExecuteScript(script); err != nil {
		tm.printf("Error: %v", err)
	}
}

// showHelp displays help information.
func (tm *TUIManager) showHelp() {
	tm.print("Available commands:")
	tm.print("  help                 - Show this help message")
	tm.print("  exit, quit           - Exit the terminal")
	tm.print("  mode <name>          - Switch to a mode")
	tm.print("  modes                - List available modes")
	tm.print("  state                - Show current mode state")
	tm.print("")

	commands := tm.ListCommands()
	if len(commands) > 0 {
		tm.print("Registered commands:")
		for _, cmd := range commands {
			tm.printf("  %-20s - %s", cmd.Name, cmd.Description)
			if cmd.Usage != "" {
				tm.printf("    Usage: %s", cmd.Usage)
			}
		}
		tm.print("")
	}

	// Show loaded scripts
	scripts := tm.engine.GetScripts()
	if len(scripts) > 0 {
		tm.printf("Loaded scripts: %d", len(scripts))
	}

	if tm.currentMode != nil {
		tm.printf("Current mode: %s", tm.currentMode.Name)
		tm.print("Note: You can execute JavaScript code directly!")
	} else {
		tm.printf("Available modes: %s", strings.Join(tm.ListModes(), ", "))
		tm.print("Switch to a mode to execute JavaScript code.")
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
				tm.printf("mode %s not found", args[0])
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
				tm.print("No modes registered")
			} else {
				tm.printf("Available modes: %s", strings.Join(modes, ", "))
				if tm.currentMode != nil {
					tm.printf("Current mode: %s", tm.currentMode.Name)
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
				tm.print("No active mode")
				return nil
			}

			tm.currentMode.mu.RLock()
			defer tm.currentMode.mu.RUnlock()

			tm.printf("Mode: %s", tm.currentMode.Name)
			tm.print("State: managed by StateManager (use history to view state snapshots)")
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

			archivePath, err := tm.resetAllState()
			if err != nil {
				tm.printf("WARNING: Failed to archive session: %v\nState preserved; reset aborted.", err)
				return nil
			}

			if archivePath != "" {
				tm.printf("Session archived to: %s", archivePath)
			} else {
				tm.print("All shared and mode-specific state has been reset to default values.")
			}
			return nil
		},
		IsGoCommand: true,
	})
}
