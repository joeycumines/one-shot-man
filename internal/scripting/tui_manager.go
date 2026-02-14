package scripting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unsafe"

	"github.com/dop251/goja"
	"github.com/joeycumines/go-prompt"
	istrings "github.com/joeycumines/go-prompt/strings"
	"github.com/joeycumines/one-shot-man/internal/argv"
	"github.com/joeycumines/one-shot-man/internal/builtin"
	"github.com/joeycumines/one-shot-man/internal/storage"
)

// extractCommandHistory converts storage.HistoryEntry slice into []string for go-prompt.
// The go-prompt history manager handles de-duplication and ordering.
// We provide the complete, chronological list of commands.
func extractCommandHistory(entries []storage.HistoryEntry) []string {
	commands := make([]string, len(entries))
	for i, entry := range entries {
		commands[i] = entry.Command
	}
	return commands
}

// NewTUIManagerWithConfig creates a new TUI manager with explicit session configuration.
// This function should be used instead of NewTUIManager to avoid data races on global state.
//
// The input and output parameters are wrapped in concrete TUIReader/TUIWriter types.
// For production use, pass nil for both to use the default stdin/stdout.
// For testing, pass custom io.Reader/io.Writer to capture output.
// If *TUIReader/*TUIWriter are passed directly, they are used as-is (no re-wrapping).
func NewTUIManagerWithConfig(ctx context.Context, engine *Engine, input io.Reader, output io.Writer, sessionID, store string) *TUIManager {
	// Discover session ID with explicit override
	actualSessionID := discoverSessionID(sessionID)

	// Create or reuse concrete reader/writer wrappers
	var reader *TUIReader
	var writer *TUIWriter

	// Check if input is already a *TUIReader (avoid double-wrapping)
	if r, ok := input.(*TUIReader); ok {
		reader = r
	} else if input == nil {
		reader = NewTUIReader() // lazily initializes to stdin
	} else {
		reader = NewTUIReaderFromIO(input)
	}

	// Check if output is already a *TUIWriter (avoid double-wrapping)
	if w, ok := output.(*TUIWriter); ok {
		writer = w
	} else if output == nil {
		writer = NewTUIWriter() // lazily initializes to stdout
	} else {
		writer = NewTUIWriterFromIO(output)
	}

	manager := &TUIManager{
		engine:           engine,
		ctx:              ctx,
		modes:            make(map[string]*ScriptMode),
		commands:         make(map[string]Command),
		commandOrder:     make([]string, 0),
		reader:           reader,
		writer:           writer,
		prompts:          make(map[string]*prompt.Prompt),
		completers:       make(map[string]goja.Callable),
		keyBindings:      make(map[string]goja.Callable),
		promptCompleters: make(map[string]string),
		writerQueue:      make(chan writeTask, 64),
		writerStop:       make(chan struct{}),
		writerDone:       make(chan struct{}),
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

	// Start the writer goroutine that executes mutation tasks under the write lock.
	// This goroutine is the only place that holds tm.mu.Lock() for JS-originated
	// mutations, preventing deadlocks when JS callbacks call back into mutating APIs.
	go manager.runWriter()

	// Initialize state manager with explicit backend
	stateManager, err := initializeStateManager(actualSessionID, store)
	if err != nil {
		const memoryBackend = "memory"
		if store == memoryBackend {
			panic(err)
		}
		_, _ = fmt.Fprintf(output, "Warning: Failed to initialize state persistence (session %q): %v\n", actualSessionID, err)
		stateManager, err = initializeStateManager(actualSessionID, memoryBackend)
		if err != nil {
			panic(err)
		}
	}

	manager.stateManager = stateManager // either our requested backend or memory fallback
	manager.commandHistory = extractCommandHistory(manager.stateManager.GetSessionHistory())

	// Register built-in commands
	manager.registerBuiltinCommands()

	return manager
}

// runWriter is the single dedicated writer goroutine that executes mutation tasks.
// All JS-originated mutations are routed through this goroutine to prevent deadlocks.
// Tasks are executed under tm.mu.Lock().
//
// This goroutine listens on both tm.writerStop (shutdown signal) and tm.writerQueue.
// The writerQueue is NEVER closed - we use writerStop to signal exit per the
// "Signal, Don't Close" pattern to prevent panics from racing senders.
func (tm *TUIManager) runWriter() {
	defer close(tm.writerDone)

	for {
		select {
		case <-tm.writerStop:
			// Shutdown signal received. Exit the loop.
			// The queue is left open for garbage collection to handle.
			return

		case task := <-tm.writerQueue:
			// Execute the task under the write lock with panic protection
			tm.mu.Lock()
			tm.debugWriteContextEnter() // Debug assertion: mark we're in write context

			var err error
			func() {
				defer func() {
					if r := recover(); r != nil {
						err = fmt.Errorf("panic in mutation task: %v", r)
					}
				}()
				err = task.fn()
			}()

			tm.debugWriteContextExit() // Debug assertion: mark we're leaving write context
			tm.mu.Unlock()

			// Send result if caller is waiting (non-blocking to avoid blocking writer)
			if task.resultCh != nil {
				select {
				case task.resultCh <- err:
				default:
					// Caller abandoned, drop result
				}
			}
		}
	}
}

// scheduleWriteAndWait queues a mutation task and waits for it to complete.
// The task runs under tm.mu.Lock(). This method blocks until the task finishes.
//
// Use this when the caller needs confirmation that the mutation succeeded,
// or when subsequent code depends on the mutation having been applied.
//
// IMPORTANT: This is the ONLY safe way for JS callbacks to perform synchronous mutations.
//
// Prevents shutdown hangs by selecting on writerStop/writerDone.
// CRITICAL: We unlock BEFORE the select to prevent deadlock when queue is full.
func (tm *TUIManager) scheduleWriteAndWait(fn func() error) error {
	resultCh := make(chan error, 1)
	task := writeTask{fn: fn, resultCh: resultCh}

	// Step 1: Check shutdown flag under lock, then unlock before select
	tm.queueMu.Lock()
	if tm.writerShutdown.IsSet() {
		tm.queueMu.Unlock()
		return errors.New("writer goroutine has shut down")
	}
	tm.queueMu.Unlock() // Unlock BEFORE select to prevent deadlock

	// The select on writerStop handles the race where shutdown happens after unlock.
	select {
	case tm.writerQueue <- task:
		// Task queued successfully, proceed to wait for result
	case <-tm.writerStop:
		return errors.New("manager shutting down")
	}

	// Step 2: Wait for result OR shutdown signal.
	// This fixes the "Shutdown Hang" defect.
	select {
	case err := <-resultCh:
		return err
	case <-tm.writerStop:
		return errors.New("manager shutting down")
	case <-tm.writerDone:
		return errors.New("manager shutting down")
	}
}

// stopWriter signals the writer to exit.
// This implements the "Signal, Don't Close" pattern to eliminate panics.
// The writerQueue channel is NEVER closed.
func (tm *TUIManager) stopWriter() {
	tm.queueMu.Lock()
	// 1. Set flag to prevent new tasks from entering
	tm.writerShutdown.Set()

	// 2. Signal the writer loop to stop via the control channel
	select {
	case <-tm.writerStop:
		// already closed
	default:
		close(tm.writerStop)
	}
	tm.queueMu.Unlock()

	// 3. Wait for writer to finish current task and cleanup
	<-tm.writerDone
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

	mode, exists := tm.modes[modeName]
	if !exists {
		tm.mu.Unlock()
		return fmt.Errorf("mode %s not found", modeName)
	}

	// Get the current mode and exit callback while holding the lock.
	currentMode := tm.currentMode
	var onExitCallback goja.Callable
	if currentMode != nil && currentMode.OnExit != nil {
		onExitCallback = currentMode.OnExit
	}

	// Release lock before calling OnExit to avoid deadlock when callback accesses state.
	tm.mu.Unlock()

	// Exit current mode (outside the lock).
	if onExitCallback != nil {
		if _, err := onExitCallback(goja.Undefined()); err != nil {
			_, _ = fmt.Fprintf(tm.writer, "Error exiting mode %s: %v\n", currentMode.Name, err)
		}
	}

	// Retrieve state but release lock before invoking callbacks/builders.
	var (
		builder         goja.Callable
		needBuild       bool
		onEnterCallback goja.Callable
	)
	{
		tm.mu.Lock()
		var unlocked bool
		defer func() {
			if !unlocked {
				tm.mu.Unlock()
			}
		}()

		tm.currentMode = mode

		// capture callbacks and builder state before releasing the lock
		builder = mode.CommandsBuilder
		needBuild = false
		if builder != nil {
			mode.mu.RLock()
			needBuild = len(mode.Commands) == 0
			mode.mu.RUnlock()
		}

		if mode.OnEnter != nil {
			onEnterCallback = mode.OnEnter
		}

		unlocked = true
		tm.mu.Unlock()
	}

	// N.B. After releasing the lock to mitigate deadlock risk.
	_, _ = fmt.Fprintf(tm.writer, "Switched to mode: %s\n", mode.Name)

	// Rehydrate ContextManager from shared state after mode switch
	// This ensures file paths are restored from persisted contextItems
	restoredItems, restoredFiles := tm.rehydrateContextManager()

	// N.B. Avoid calling holding locks here, or risk deadlock within command factories.
	if builder != nil && needBuild {
		if err := tm.buildModeCommands(mode); err != nil {
			_, _ = fmt.Fprintf(tm.writer, "Error building commands for mode %s: %v\n", mode.Name, err)
			// Note: We don't return the error to allow mode entry to continue
		}
	}

	// The intent of this message is to notify the user that state restoration occurred.
	// Excessive noise is detrimental, so we keep it concise / on one line.
	if restoredItems > 0 {
		_, _ = fmt.Fprintf(tm.writer, "Session restored: %d items (%d files). 'reset' to clear.\n", restoredItems, restoredFiles)
	}

	// N.B. Similarly, mitigate deadlock risk - avoid holding locks while calling OnEnter.
	if onEnterCallback != nil {
		if _, err := onEnterCallback(goja.Undefined(), goja.Undefined(), goja.Undefined()); err != nil {
			_, _ = fmt.Fprintf(tm.writer, "Error entering mode %s: %v\n", mode.Name, err)
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

	// If this is a new command, add it to the order slice
	if _, exists := tm.commands[cmd.Name]; !exists {
		tm.commandOrder = append(tm.commandOrder, cmd.Name)
	}
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

// buildModeCommands builds commands from CommandsBuilder function
func (tm *TUIManager) buildModeCommands(mode *ScriptMode) error {
	if mode.CommandsBuilder == nil {
		return nil
	}

	// Call the CommandsBuilder function
	// Scripts manage their own state through closures now
	result, err := mode.CommandsBuilder(goja.Undefined(), goja.Undefined())
	if err != nil {
		return fmt.Errorf("CommandsBuilder failed: %w", err)
	}

	// Convert result to commands map
	if result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
		return fmt.Errorf("CommandsBuilder returned nil/undefined")
	}

	resultObj := result.ToObject(tm.engine.vm)
	if resultObj == nil {
		return fmt.Errorf("CommandsBuilder did not return an object")
	}

	// Iterate over the returned command definitions
	for _, key := range resultObj.Keys() {
		cmdVal := resultObj.Get(key)
		if cmdVal == nil || goja.IsUndefined(cmdVal) || goja.IsNull(cmdVal) {
			continue
		}

		cmdObj := cmdVal.ToObject(tm.engine.vm)
		if cmdObj == nil {
			continue
		}

		// Extract command properties
		desc := ""
		if descVal := cmdObj.Get("description"); descVal != nil && !goja.IsUndefined(descVal) {
			desc = descVal.String()
		}

		usage := ""
		if usageVal := cmdObj.Get("usage"); usageVal != nil && !goja.IsUndefined(usageVal) {
			usage = usageVal.String()
		}

		var argCompleters []string
		if acVal := cmdObj.Get("argCompleters"); acVal != nil && !goja.IsUndefined(acVal) {
			if acObj := acVal.ToObject(tm.engine.vm); acObj != nil {
				for _, k := range acObj.Keys() {
					if v := acObj.Get(k); v != nil && !goja.IsUndefined(v) {
						argCompleters = append(argCompleters, v.String())
					}
				}
			}
		}

		var flagDefs []FlagDef
		if fdVal := cmdObj.Get("flagDefs"); fdVal != nil && !goja.IsUndefined(fdVal) {
			if fdObj := fdVal.ToObject(tm.engine.vm); fdObj != nil {
				for _, k := range fdObj.Keys() {
					if v := fdObj.Get(k); v != nil && !goja.IsUndefined(v) {
						if itemObj := v.ToObject(tm.engine.vm); itemObj != nil {
							var fd FlagDef
							if nameVal := itemObj.Get("name"); nameVal != nil && !goja.IsUndefined(nameVal) {
								fd.Name = nameVal.String()
							}
							if descVal := itemObj.Get("description"); descVal != nil && !goja.IsUndefined(descVal) {
								fd.Description = descVal.String()
							}
							if fd.Name != "" {
								flagDefs = append(flagDefs, fd)
							}
						}
					}
				}
			}
		}

		cmd := Command{
			Name:          key,
			Description:   desc,
			Usage:         usage,
			IsGoCommand:   false,
			ArgCompleters: argCompleters,
			FlagDefs:      flagDefs,
		}

		if handlerVal := cmdObj.Get("handler"); handlerVal != nil && !goja.IsUndefined(handlerVal) {
			cmd.Handler = handlerVal.Export()
			mode.mu.Lock()
			mode.Commands[key] = cmd
			mode.CommandOrder = append(mode.CommandOrder, key)
			mode.mu.Unlock()
		}
	}

	return nil
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
		// Note: State is accessed via closure from the commands builder function,
		// so we don't inject it here (removing redundant code).
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
		// runDeferred is expected to recover its deferred panics, but guard
		// again at this callsite so any unexpected panic is converted into
		// an error rather than bringing down the whole TUI manager.
		dErr := func() (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("deferred panic: %v", r)
				}
			}()
			return execCtx.runDeferred()
		}()
		if dErr != nil {
			if execErr != nil {
				execErr = fmt.Errorf("%v; deferred error: %v", execErr, dErr)
			} else {
				execErr = dErr
			}
		}

		// Capture history snapshot after successful command execution
		if execErr == nil {
			tm.captureHistorySnapshot(cmd.Name, args)
		}

		return execErr
	}
}

// OLD API - REMOVED: GetState and SetState are replaced by StateAccessor
// Use the { state } injection pattern in command handlers instead

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
	// Use ordered iteration instead of map iteration
	for _, cmdName := range tm.commandOrder {
		if cmd, exists := tm.commands[cmdName]; exists {
			commands = append(commands, cmd)
		}
	}

	// Add current mode commands
	if tm.currentMode != nil {
		tm.currentMode.mu.RLock()
		// Use ordered iteration for mode commands too
		for _, cmdName := range tm.currentMode.CommandOrder {
			if cmd, exists := tm.currentMode.Commands[cmdName]; exists {
				commands = append(commands, cmd)
			}
		}
		tm.currentMode.mu.RUnlock()
	}

	return commands
}

// Run starts the TUI manager.
func (tm *TUIManager) Run() {
	// Route engine output through a queue we flush at safe points
	tm.engine.logger.SetTUISink(func(msg string) {
		tm.outputMu.Lock()
		tm.outputQueue = append(tm.outputQueue, msg)
		tm.outputMu.Unlock()
	})
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

	p := tm.buildGoPrompt(promptBuildConfig{
		prefixCallback:          func() string { return tm.getPromptString() },
		colors:                  tm.defaultColors,
		completer:               completer,
		initialCommand:          tm.getInitialCommand(),
		history:                 tm.commandHistory,
		flushOutput:             true,
		maxSuggestion:           10,
		dynamicCompletion:       true,
		executeHidesCompletions: true,
		escapeToggle:            true,
	})

	// Store as active prompt
	tm.mu.Lock()
	tm.activePrompt = p
	tm.mu.Unlock()

	// Run the prompt (this will block until exit).
	// Use RunNoExit to prevent go-prompt from calling os.Exit on SIGTERM.
	p.RunNoExit()

	// Clear active prompt when done
	tm.mu.Lock()
	tm.activePrompt = nil
	tm.mu.Unlock()
}

// buildGoPrompt constructs a go-prompt instance from a promptBuildConfig.
// This is the shared builder used by both runAdvancedPrompt (registerMode path)
// and jsCreatePrompt (low-level JS API path) to ensure consistent feature support.
func (tm *TUIManager) buildGoPrompt(cfg promptBuildConfig) *prompt.Prompt {
	// Create the executor function.
	// When executor returns false, signal the exit checker to terminate the prompt.
	// We do NOT call os.Exit here - exit is handled gracefully via ExitChecker.
	executor := func(line string) {
		if cfg.flushOutput {
			tm.flushQueuedOutput()
		}
		if !tm.executor(line) {
			// Signal the prompt to exit via ExitChecker mechanism
			tm.RequestExit()
		}
		if cfg.flushOutput {
			tm.flushQueuedOutput()
		}
	}

	// Create the exit checker that allows the prompt to exit when requested.
	// This is called by go-prompt after each input to determine if Run() should return.
	exitChecker := func(_ string, _ bool) bool {
		return tm.IsExitRequested()
	}

	colors := cfg.colors

	// Configure prompt options
	options := []prompt.Option{
		prompt.WithCompleter(cfg.completer),
		prompt.WithExitChecker(exitChecker),
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
		prompt.WithScrollbarThumbColor(colors.ScrollbarThumb),
		prompt.WithScrollbarBGColor(colors.ScrollbarBG),
	}

	// Prefix: prefer callback over static
	if cfg.prefixCallback != nil {
		options = append(options, prompt.WithPrefixCallback(cfg.prefixCallback))
	} else {
		options = append(options, prompt.WithPrefix(cfg.prefix))
	}

	// Title (optional - only set when non-empty)
	if cfg.title != "" {
		options = append(options, prompt.WithTitle(cfg.title))
	}

	// MaxSuggestion (0 uses go-prompt default)
	if cfg.maxSuggestion > 0 {
		options = append(options, prompt.WithMaxSuggestion(cfg.maxSuggestion))
	}

	// Dynamic completion
	if cfg.dynamicCompletion {
		options = append(options, prompt.WithDynamicCompletion(true))
	}

	// Auto-hiding completions when submitting input
	if cfg.executeHidesCompletions {
		options = append(options, prompt.WithExecuteHidesCompletions(true))
	}

	// Bind Escape key to toggle completion visibility
	if cfg.escapeToggle {
		options = append(options, prompt.WithKeyBindings(
			prompt.KeyBind{
				Key: prompt.Escape,
				Fn: func(p *prompt.Prompt) bool {
					if p.Completion().IsHidden() {
						p.Completion().Show()
					} else {
						p.Completion().Hide()
					}
					return true
				},
			},
		))
	}

	// Initial command (optional)
	if cfg.initialCommand != "" {
		options = append(options, prompt.WithInitialCommand(cfg.initialCommand, false))
	}

	// This enables the sync protocol when built with the `integration` build tag
	options = append(options, staticGoPromptOptions...)

	// CRITICAL: Inject the shared reader/writer into go-prompt.
	// This ensures go-prompt uses the same terminal I/O as bubbletea and tview,
	// preventing conflicts over stdin and ensuring proper terminal state cleanup.
	options = append(options,
		prompt.WithReader(tm.reader),
		prompt.WithWriter(tm.writer),
	)

	// Add command history
	if len(cfg.history) > 0 {
		options = append(options, prompt.WithHistory(cfg.history))
	}
	if cfg.historySize > 0 {
		options = append(options, prompt.WithHistorySize(cfg.historySize))
	}

	// Add any registered key bindings
	if keyBinds := tm.buildKeyBinds(); len(keyBinds) > 0 {
		options = append(options, prompt.WithKeyBind(keyBinds...))
	}

	return prompt.New(executor, options...)
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
	for _, m := range queue {
		// Messages already include any necessary trailing newline.
		b := unsafe.Slice(unsafe.StringData(m), len(m))
		_, _ = tm.writer.Write(b)
	}
}

// === TEST HELPER METHODS ===
// These methods provide test-only access to state using string keys.
// They bridge the gap between Go tests and the Symbol-based state system.

// SetStateForTest sets a state value using a persistent string key (for testing only).
func (tm *TUIManager) SetStateForTest(persistentKey string, value interface{}) error {
	if tm.stateManager == nil {
		return fmt.Errorf("state manager not initialized")
	}
	tm.stateManager.SetState(persistentKey, value)
	return nil
}

// GetStateForTest retrieves a state value using a persistent string key (for testing only).
func (tm *TUIManager) GetStateForTest(persistentKey string) (interface{}, error) {
	if tm.stateManager == nil {
		return nil, fmt.Errorf("state manager not initialized")
	}
	val, ok := tm.stateManager.GetState(persistentKey)
	if !ok {
		return nil, nil
	}
	return val, nil
}

// captureHistorySnapshot captures a history snapshot - simplified in new architecture
func (tm *TUIManager) captureHistorySnapshot(commandName string, commandArgs []string) {
	if tm.stateManager == nil {
		return
	}

	tm.mu.RLock()
	currentMode := tm.currentMode
	tm.mu.RUnlock()

	if currentMode == nil {
		return
	}

	// Build the command string
	cmdString := commandName
	if len(commandArgs) > 0 {
		cmdString = fmt.Sprintf("%s %s", commandName, strings.Join(commandArgs, " "))
	}

	// Serialize the complete state (both script and shared zones)
	stateJSON, err := tm.stateManager.SerializeCompleteState()
	if err != nil {
		log.Printf("WARNING: Failed to serialize state for snapshot: %v", err)
		stateJSON = json.RawMessage("{}")
	}

	// Capture snapshot
	modeID := currentMode.Name
	if err := tm.stateManager.CaptureSnapshot(modeID, cmdString, stateJSON); err != nil {
		_, _ = fmt.Fprintf(tm.writer, "Warning: Failed to capture history snapshot: %v\n", err)
	}
}

// === TEST-ONLY HELPERS ===
// These methods provide test-only access to state through the Symbol registry.
// Unlike the removed tui.getState/setState production APIs, these are explicitly
// test-only and use the same Symbol lookup mechanism that production code uses.

// GetStateViaJS and SetStateViaJS - DEPRECATED aliases for backward compatibility with old tests
func (tm *TUIManager) GetStateViaJS(persistentKey string) (interface{}, error) {
	return tm.GetStateForTest(persistentKey)
}

func (tm *TUIManager) SetStateViaJS(persistentKey string, value interface{}) error {
	return tm.SetStateForTest(persistentKey, value)
}

// TriggerExit programmatically stops the prompt for graceful shutdown.
func (tm *TUIManager) TriggerExit() {
	tm.mu.Lock()
	p := tm.activePrompt
	tm.mu.Unlock()

	if p != nil {
		p.Close()
	}
}

// SetExitRequested sets the runtime-only exit request flag.
// This flag is NEVER persisted - it's purely for runtime coordination
// between JavaScript commands and the shell loop's exit checker.
func (tm *TUIManager) SetExitRequested(value bool) {
	if value {
		tm.exitRequested.Set()
	} else {
		tm.exitRequested.Clear()
	}
}

// RequestExit signals that the shell loop should exit.
// This is the preferred method to call from JavaScript.
func (tm *TUIManager) RequestExit() {
	tm.exitRequested.Set()
}

// ClearExitRequested clears the exit request flag.
// Called when restarting a prompt loop or after handling an exit.
func (tm *TUIManager) ClearExitRequested() {
	tm.exitRequested.Clear()
}

// IsExitRequested returns whether an exit has been requested.
// This is checked by the exit checker to determine if the shell loop should exit.
func (tm *TUIManager) IsExitRequested() bool {
	return tm.exitRequested.IsSet()
}

// PersistSessionForTest persists the current session (for testing only).
func (tm *TUIManager) PersistSessionForTest() error {
	if tm.stateManager != nil {
		return tm.stateManager.PersistSession()
	}
	return nil
}

// Close releases resources held by the TUI manager, including the state manager.
func (tm *TUIManager) Close() error {
	// Stop the writer goroutine first
	if tm.writerQueue != nil {
		tm.stopWriter()
	}
	if tm.stateManager != nil {
		return tm.stateManager.Close()
	}
	return nil
}

// GetStateManager returns the StateManager for this TUI manager (implements StateManagerProvider).
func (tm *TUIManager) GetStateManager() builtin.StateManager {
	// Return the concrete StateManager (which implements builtin.StateManager interface)
	if tm.stateManager == nil {
		return nil
	}
	return tm.stateManager
}

// resetAllState clears all state and archives the previous session (used by the reset REPL command).
// It performs a safe archive-and-reset operation that preserves history while clearing state.
// Returns the archive path if successful, or an error otherwise.
func (tm *TUIManager) resetAllState() (string, error) {
	// Perform archive + reset via state manager (safe with file operations)
	if tm.stateManager == nil {
		return "", fmt.Errorf("no state manager")
	}

	archivePath, err := tm.stateManager.ArchiveAndReset()
	if err != nil {
		// If archiving fails we MUST not destroy the existing session.
		// Preserve state and surface the error to the caller to decide how to handle it.
		return "", err
	}

	// Clear context manager only after a successful archive+reset so that
	// we don't accidentally drop context items when the reset failed.
	if tm.engine != nil && tm.engine.contextManager != nil {
		tm.engine.contextManager.Clear()
	}

	return archivePath, nil
}

// resetSharedState, resetAllModeStates, getStateBySymbol, setStateBySymbol - DELETED in new architecture
// State is managed through createState() API, no direct symbol access needed

// rehydrateContextManager re-populates the ContextManager from restored state.
// In the new architecture, this is called after mode switch when "contextItems" exists in shared state.
// It looks for file-type items and re-adds them to the ContextManager so commands like remove() and toTxtar() work.
func (tm *TUIManager) rehydrateContextManager() (int, int) {
	if tm.engine == nil || tm.engine.contextManager == nil || tm.stateManager == nil {
		return 0, 0
	}

	// Get the shared contextItems from state
	contextItemsRaw, ok := tm.stateManager.GetState("contextItems")
	if !ok {
		return 0, 0
	}

	// Convert to the expected format
	var items []map[string]interface{}

	// Handle different possible types
	switch v := contextItemsRaw.(type) {
	case []map[string]interface{}:
		items = v
	case []interface{}:
		// Convert each element
		for _, item := range v {
			if itemMap, ok := item.(map[string]interface{}); ok {
				items = append(items, itemMap)
			}
		}
	default:
		// Unsupported type, skip rehydration
		return 0, 0
	}

	// Iterate through items and re-populate ContextManager with file-type entries
	var validItems []interface{}
	stateChanged := false
	var restoredFiles int

	for _, srcItem := range items {
		// Work on a shallow copy so we never mutate the original in-memory
		// structure returned from the StateManager. Modifying the original
		// can leave the in-memory store in a partially-modified state when
		// early returns or errors occur.
		item := make(map[string]interface{}, len(srcItem))
		for k, v := range srcItem {
			item[k] = v
		}

		itemType, hasType := item["type"].(string)
		label, hasLabel := item["label"].(string)

		if !hasType || !hasLabel {
			// Keep malformed items? Probably not, but let's stick to filtering files.
			validItems = append(validItems, item)
			continue
		}

		// Only process file-type items
		if itemType == "file" {
			// Try the stored label exactly as-is first. This preserves valid
			// POSIX filenames that include backslashes ("foo\bar.txt") which
			// are legal on Linux/macOS and must not be silently converted to
			// directory separators.
			// Attempt to re-add using the stored label. AddRelativePath now
			// returns the canonical owner key used by the backend so we can
			// update the in-memory TUI state to keep labels in sync.
			owner, err := tm.engine.contextManager.AddRelativePath(label)

			// If initial attempt succeeded, ensure the TUI state label is
			// updated to the normalized owner returned by the backend. This
			// prevents a mismatch where the UI keeps a different label than
			// the actual backend key (which causes ghost entries that can't be removed).
			if err == nil {
				if owner != label {
					item["label"] = owner
					stateChanged = true
				}
				restoredFiles++
				validItems = append(validItems, item)
				continue
			}

			// Fallback for cross-platform snapshots: if the direct attempt
			// failed and we are on a non-Windows host, try converting
			// Windows-style backslashes to forward slashes and retry. This
			// helps rehydrate sessions created on Windows when rehydrating
			// on POSIX systems.
			// NOTE: POSIX filesystems allow '\\' within filenames. This
			// normalization is therefore a best-effort compatibility step
			// (not a perfect mapping) — it may accidentally rebind a missing
			// POSIX filename that contained literal backslashes to a
			// different existing file if one happens to exist at the
			// normalized path. The fallback is only attempted for missing
			// files and is intended to reduce Windows->POSIX rehydration
			// failures; it is not a security or correctness guarantee.
			// Only attempt normalization if the original error indicates the
			// file truly does not exist. Do not mask permission/IO errors.
			// Note: err is guaranteed non-nil here since we returned early above on success.
			if (os.IsNotExist(err) || errors.Is(err, os.ErrNotExist)) && runtime.GOOS != "windows" && strings.Contains(label, "\\") {
				normalizedLabel := strings.ReplaceAll(label, "\\", "/")
				normalized := filepath.Clean(filepath.FromSlash(normalizedLabel))
				owner2, err2 := tm.engine.contextManager.AddRelativePath(normalized)

				// If the fallback succeeded, update the in-memory TUI state
				// to the actual owner returned by the backend.
				if err2 == nil {
					label = owner2
					item["label"] = owner2
					stateChanged = true
					restoredFiles++
					validItems = append(validItems, item)
					continue
				}
				// Normalization fallback failed - prefer surfacing the fallback
				// error rather than the original 'not exists' error.
				err = fmt.Errorf("fallback normalization failed for %s -> %s: %w", label, normalized, err2)
			}

			// Handle error (err is guaranteed non-nil here since we returned early on success)
			// If the file no longer exists, log it and remove from state
			if os.IsNotExist(err) {
				_, _ = fmt.Fprintf(tm.writer, "Info: file from previous session not found, removing: %s\n", label)
				stateChanged = true
				continue
			}
			_, _ = fmt.Fprintf(tm.writer, "Error restoring file %s: %v\n", label, err)
			// For other errors, we also remove it to ensure the session is valid
			stateChanged = true
			continue
		}
		// Keep valid items
		validItems = append(validItems, item)
	}

	// Update state if items were removed and persist the change so that
	// normalized labels are written back to the backend storage.
	if stateChanged {
		tm.stateManager.SetState("contextItems", validItems)
		if err := tm.stateManager.PersistSession(); err != nil {
			_, _ = fmt.Fprintf(tm.writer, "Warning: failed to persist rehydrated contextItems: %v\n", err)
		}
	}

	return len(validItems), restoredFiles
}
