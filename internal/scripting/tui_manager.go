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
func NewTUIManagerWithConfig(ctx context.Context, engine *Engine, input io.Reader, output io.Writer, sessionID, store string) *TUIManager {
	// Discover session ID with explicit override
	actualSessionID := discoverSessionID(sessionID)

	manager := &TUIManager{
		engine:           engine,
		ctx:              ctx,
		modes:            make(map[string]*ScriptMode),
		commands:         make(map[string]Command),
		commandOrder:     make([]string, 0),
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
			_, _ = fmt.Fprintf(tm.output, "Error exiting mode %s: %v\n", currentMode.Name, err)
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

		_, _ = fmt.Fprintf(tm.output, "Switched to mode: %s\n", mode.Name)

		unlocked = true
		tm.mu.Unlock()
	}

	// Rehydrate ContextManager from shared state after mode switch
	// This ensures file paths are restored from persisted contextItems
	restoredItems, restoredFiles := tm.rehydrateContextManager()

	// N.B. Avoid calling holding locks here, or risk deadlock within command factories.
	if builder != nil && needBuild {
		if err := tm.buildModeCommands(mode); err != nil {
			_, _ = fmt.Fprintf(tm.output, "Error building commands for mode %s: %v\n", mode.Name, err)
			// Note: We don't return the error to allow mode entry to continue
		}
	}

	// The intent of this message is to notify the user that state restoration occurred.
	// Excessive noise is detrimental, so we keep it concise / on one line.
	if restoredItems > 0 {
		_, _ = fmt.Fprintf(tm.output, "Session restored: %d items (%d files). 'reset' to clear.\n", restoredItems, restoredFiles)
	}

	// N.B. Similarly, mitigate deadlock risk - avoid holding locks while calling OnEnter.
	if onEnterCallback != nil {
		if _, err := onEnterCallback(goja.Undefined(), goja.Undefined(), goja.Undefined()); err != nil {
			_, _ = fmt.Fprintf(tm.output, "Error entering mode %s: %v\n", mode.Name, err)
		}
	}

	// Execute InitialCommand if configured (after OnEnter and command building)
	// This allows modes to automatically run a command when entered, such as
	// launching a TUI from a REPL mode (e.g., "tui" command to show visual UI).
	if mode.InitialCommand != "" {
		// TODO: wire up once implemented into go-prompt module
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

		cmd := Command{
			Name:          key,
			Description:   desc,
			Usage:         usage,
			IsGoCommand:   false,
			ArgCompleters: argCompleters,
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
		if execErr == nil && tm.currentMode != nil && tm.currentMode.TUIConfig != nil && tm.currentMode.TUIConfig.EnableHistory {
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
		prompt.WithMaxSuggestion(10),
		prompt.WithDynamicCompletion(true),
		// Enable auto-hiding completions when submitting input
		prompt.WithExecuteHidesCompletions(true),
		// Bind Escape key to toggle completion visibility
		prompt.WithKeyBindings(
			prompt.KeyBind{
				Key: prompt.Escape,
				Fn: func(p *prompt.Prompt) bool {
					// Toggle: if hidden, show; if visible, hide
					if p.Completion().IsHidden() {
						p.Completion().Show()
					} else {
						p.Completion().Hide()
					}
					return true
				},
			},
		),
	}

	// this enables the sync protocol when built with the `integration` build tag
	options = append(options, staticGoPromptOptions...)

	// Add command history from persistent session
	if len(tm.commandHistory) > 0 {
		options = append(options, prompt.WithHistory(tm.commandHistory))
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

// === NEW STATE MANAGEMENT METHODS ===

// jsCreateStateContractInternal is DELETED - no longer used in new architecture
// State is managed directly by JS via tui.createState() -> StateManager

// getStateBySymbol - DELETED in new architecture

// setStateBySymbol - DELETED in new architecture

// initModeState - DELETED in new architecture

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
		_, _ = fmt.Fprintf(tm.output, "Warning: Failed to capture history snapshot: %v\n", err)
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

// PersistSessionForTest persists the current session (for testing only).
func (tm *TUIManager) PersistSessionForTest() error {
	if tm.stateManager != nil {
		return tm.stateManager.PersistSession()
	}
	return nil
}

// Close releases resources held by the TUI manager, including the state manager.
func (tm *TUIManager) Close() error {
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
func (tm *TUIManager) resetAllState() {
	// Perform archive + reset via state manager (safe with file operations)
	if tm.stateManager != nil {
		archivePath, err := tm.stateManager.ArchiveAndReset()
		if err != nil {
			// If archiving fails we MUST not destroy the existing session.
			// Preserve state and warn the user so they can retry or investigate.
			_, _ = fmt.Fprintf(tm.output, "WARNING: Failed to archive session: %v\nState preserved; reset aborted.\n", err)
			return
		}

		_, _ = fmt.Fprintf(tm.output, "Session archived to: %s\n", archivePath)

		// Clear context manager only after a successful archive+reset so that
		// we don't accidentally drop context items when the reset failed.
		if tm.engine != nil && tm.engine.contextManager != nil {
			tm.engine.contextManager.Clear()
		}
	}
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
			if err != nil && (os.IsNotExist(err) || errors.Is(err, os.ErrNotExist)) && runtime.GOOS != "windows" && strings.Contains(label, "\\") {
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
				// If the normalization fallback failed, prefer surfacing
				// the fallback error rather than the original 'not exists'
				// error so callers and logs reflect what actually failed.
				if err2 != nil {
					err = fmt.Errorf("fallback normalization failed for %s -> %s: %w", label, normalized, err2)
				}
			}

			if err != nil {
				// If the file no longer exists, log it and remove from state
				if os.IsNotExist(err) {
					_, _ = fmt.Fprintf(tm.output, "Info: file from previous session not found, removing: %s\n", label)
					stateChanged = true
					continue
				} else {
					_, _ = fmt.Fprintf(tm.output, "Error restoring file %s: %v\n", label, err)
					// For other errors, we also remove it to ensure the session is valid
					stateChanged = true
					continue
				}
			}
			restoredFiles++
		}
		// Keep valid items
		validItems = append(validItems, item)
	}

	// Update state if items were removed and persist the change so that
	// normalized labels are written back to the backend storage.
	if stateChanged {
		tm.stateManager.SetState("contextItems", validItems)
		if err := tm.stateManager.PersistSession(); err != nil {
			_, _ = fmt.Fprintf(tm.output, "Warning: failed to persist rehydrated contextItems: %v\n", err)
		}
	}

	return len(validItems), restoredFiles
}
