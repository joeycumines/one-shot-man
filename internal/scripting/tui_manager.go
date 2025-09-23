package scripting

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/elk-language/go-prompt"
	istrings "github.com/elk-language/go-prompt/strings"
	"github.com/joeycumines/one-shot-man/internal/argv"
)

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
		// Handle JavaScript function; temporarily expose a minimal ctx.
		//
		// Ensure we restore the previous context object after execution...
		parentCtxObj := tm.engine.vm.Get(jsGlobalContextName)
		defer tm.engine.vm.Set(jsGlobalContextName, parentCtxObj)
		// ... then set up a new execution context for this command.
		execCtx := &ExecutionContext{engine: tm.engine, name: "cmd:" + cmd.Name}
		if err := tm.engine.setExecutionContext(execCtx); err != nil {
			// Treat as fatal: we cannot safely execute the command without ctx
			panic(fmt.Sprintf("unrecoverable error setting command execution context: %v", err))
		}

		// Convert args to JavaScript array
		argsJS := tm.engine.vm.NewArray()
		for i, arg := range args {
			_ = argsJS.Set(strconv.Itoa(i), arg)
		}

		// Execute the command handler with panic protection, then run defers.
		var execErr error
		func() {
			defer func() {
				if r := recover(); r != nil {
					execErr = fmt.Errorf("command panicked: %v", r)
				}
			}()
			switch handler := cmd.Handler.(type) {
			case goja.Callable:
				_, execErr = handler(goja.Undefined(), argsJS)
			case func(goja.FunctionCall) goja.Value:
				// Create a function call with the arguments
				call := goja.FunctionCall{
					This:      goja.Undefined(),
					Arguments: []goja.Value{argsJS},
				}
				handler(call)
			default:
				// Try to call it as a general function
				if tm.engine != nil && tm.engine.vm != nil {
					val := tm.engine.vm.ToValue(handler)
					if callable, ok := goja.AssertFunction(val); ok {
						_, execErr = callable(goja.Undefined(), argsJS)
						return
					}
				}
				execErr = fmt.Errorf("invalid JavaScript command handler for %s: %T", cmd.Name, handler)
			}
		}()

		// Always run deferred functions collected by execCtx
		if dErr := execCtx.runDeferred(); dErr != nil {
			if execErr != nil {
				execErr = fmt.Errorf("%v; deferred error: %v", execErr, dErr)
			} else {
				execErr = dErr
			}
		}
		return execErr
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
	// Route engine output through a queue we flush at safe points
	tm.engine.logger.SetTUISink(func(msg string) {
		tm.outputMu.Lock()
		tm.outputQueue = append(tm.outputQueue, msg)
		tm.outputMu.Unlock()
	})
	// Prominent, unavoidable warning: this TUI is ephemeral and does not persist state
	_, _ = fmt.Fprintln(writer, "================================================================")
	_, _ = fmt.Fprintln(writer, "WARNING: EPHEMERAL SESSION - nothing is persisted. Your work will be lost on exit.")
	_, _ = fmt.Fprintln(writer, "Save or export anything you need BEFORE quitting.")
	_, _ = fmt.Fprintln(writer, "================================================================")
	_, _ = fmt.Fprintln(writer, "one-shot-man Rich TUI Terminal")
	_, _ = fmt.Fprintln(writer, "Type 'help' for available commands, 'exit' to quit")
	modes := tm.ListModes()
	_, _ = fmt.Fprintf(writer, "Available modes: %s\n", strings.Join(modes, ", "))
	_, _ = fmt.Fprintln(writer, "Starting advanced go-prompt interface")
	// Flush any pending output (e.g., from onEnter) before starting prompt
	tm.flushQueuedOutput()
	tm.runAdvancedPrompt()
}

// runAdvancedPrompt runs a go-prompt instance with default configuration.
func (tm *TUIManager) runAdvancedPrompt() {
	// Create a default completer that provides command completion
	completer := func(document prompt.Document) ([]prompt.Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		suggestions := tm.getDefaultCompletionSuggestions(document)
		before := document.TextBeforeCursor()
		_, cur := argv.BeforeCursor(before)
		start, end := cur.Start, cur.End
		return suggestions, istrings.RuneNumber(start), istrings.RuneNumber(end)
	}

	// Create the executor function
	executor := func(line string) {
		// Drain any pending output before executing the command
		tm.flushQueuedOutput()
		if !tm.executor(line) {
			// If executor returns false, exit the prompt
			os.Exit(0)
		}
		// Flush any queued output synchronously after executing a line
		tm.flushQueuedOutput()
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
	defaultHistoryFile := ".osm_history"
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

// flushQueuedOutput writes any buffered output messages to the terminal
// using a syncWriter to ensure they hit the PTY immediately. Messages are
// written verbatim as provided by PrintToTUI/PrintfToTUI, which are
// responsible for ensuring trailing newlines as needed.
func (tm *TUIManager) flushQueuedOutput() {
	tm.outputMu.Lock()
	queue := tm.outputQueue
	tm.outputQueue = nil
	tm.outputMu.Unlock()
	if len(queue) == 0 {
		return
	}
	writer := &syncWriter{tm.output}
	for _, m := range queue {
		// Messages already include any necessary trailing newline.
		_, _ = writer.Write([]byte(m))
	}
}
