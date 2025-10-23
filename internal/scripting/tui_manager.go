package scripting

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/joeycumines/go-prompt"
	istrings "github.com/joeycumines/go-prompt/strings"
	"github.com/joeycumines/one-shot-man/internal/argv"
	"github.com/joeycumines/one-shot-man/internal/scripting/storage"
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
func NewTUIManagerWithConfig(ctx context.Context, engine *Engine, input io.Reader, output io.Writer, sessionID, storageBackend string) *TUIManager {
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
		history:          make(map[string][]HistoryEntry),
		pendingContracts: make(map[string]*StateContract),
		sharedContracts:  make([]*StateContract, 0),
		sharedState:      make(map[goja.Value]interface{}),
		sessionID:        actualSessionID,
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
	stateManager, err := initializeStateManager(actualSessionID, storageBackend)
	if err != nil {
		// Log the error but continue - the TUI can function without persistence
		fmt.Fprintf(output, "Warning: Failed to initialize state persistence: %v\n", err)
		fmt.Fprintln(output, "Session will run in ephemeral mode (state will not be persisted)")
		manager.commandHistory = make([]string, 0)
	} else {
		manager.stateManager = stateManager
		// Load command history *safely* using the new accessor
		manager.commandHistory = extractCommandHistory(manager.stateManager.GetSessionHistory())
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

	mode, exists := tm.modes[modeName]
	if !exists {
		tm.mu.Unlock()
		return fmt.Errorf("mode %s not found", modeName)
	}

	// Get the current mode and exit callback while holding the lock
	currentMode := tm.currentMode
	var onExitCallback goja.Callable
	if currentMode != nil && currentMode.OnExit != nil {
		onExitCallback = currentMode.OnExit
	}

	// Release lock before calling OnExit to avoid deadlock when callback accesses state
	tm.mu.Unlock()

	// Exit current mode (outside the lock)
	if onExitCallback != nil {
		// Inject StateAccessor for OnExit
		stateArg := tm.engine.vm.NewObject()
		accessor := NewStateAccessor(tm)
		_ = stateArg.Set("state", accessor.ToJS(tm.engine.vm))
		// Call onExit with signature: function(this, stateObj)
		// Pass goja.Undefined() as `this` and goja.Undefined() as first param (unused), stateArg as second
		if _, err := onExitCallback(goja.Undefined(), goja.Undefined(), stateArg); err != nil {
			fmt.Fprintf(tm.output, "Error exiting mode %s: %v\n", currentMode.Name, err)
		}
	}

	// Reacquire lock for mode switching
	tm.mu.Lock()

	fmt.Fprintf(tm.output, "Switched to mode: %s\n", mode.Name)

	// Enter new mode
	tm.currentMode = mode

	// Initialize state from contract before OnEnter runs
	if mode.StateContract != nil {
		tm.initModeState(mode)
	}

	// Capture callbacks and builder state before releasing the lock
	builder := mode.CommandsBuilder
	needBuild := false
	if builder != nil {
		mode.mu.RLock()
		needBuild = len(mode.Commands) == 0
		mode.mu.RUnlock()
	}

	var onEnterCallback goja.Callable
	if mode.OnEnter != nil {
		onEnterCallback = mode.OnEnter
	}

	// Release lock before invoking potentially re-entrant callbacks/builders
	tm.mu.Unlock()

	// Build commands outside the lock to avoid deadlocks if builder touches state
	if builder != nil && needBuild {
		if err := tm.buildModeCommands(mode); err != nil {
			fmt.Fprintf(tm.output, "Error building commands for mode %s: %v\n", mode.Name, err)
			// Note: We don't return the error to allow mode entry to continue
		}
	}

	// Call OnEnter outside the lock
	if onEnterCallback != nil {
		// Inject StateAccessor for OnEnter
		stateArg := tm.engine.vm.NewObject()
		accessor := NewStateAccessor(tm)
		_ = stateArg.Set("state", accessor.ToJS(tm.engine.vm))
		// Call onEnter with signature: function(this, stateObj)
		// Pass goja.Undefined() as `this` and goja.Undefined() as first param (unused), stateArg as second
		if _, err := onEnterCallback(goja.Undefined(), goja.Undefined(), stateArg); err != nil {
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

	// Create StateAccessor for this mode
	accessor := NewStateAccessor(tm)
	stateJS := accessor.ToJS(tm.engine.vm)

	// Call the CommandsBuilder function with state accessor
	result, err := mode.CommandsBuilder(goja.Undefined(), stateJS)
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
		if dErr := execCtx.runDeferred(); dErr != nil {
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
	writer := &syncWriter{tm.output}
	// Route engine output through a queue we flush at safe points
	tm.engine.logger.SetTUISink(func(msg string) {
		tm.outputMu.Lock()
		tm.outputQueue = append(tm.outputQueue, msg)
		tm.outputMu.Unlock()
	})
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

// jsCreateStateContractInternal is called by jsCreateStateContract to register
// the Symbol-to-string mapping and store the contract for later association with a mode.
func (tm *TUIManager) jsCreateStateContractInternal(modeName string, symbolsObj goja.Value, definitions map[string]interface{}, isShared bool) error {
	runtime := tm.engine.vm

	// Register the contract with the engine's symbol registry
	contract, err := RegisterContract(tm.engine.symbolRegistry, modeName, runtime, symbolsObj, definitions, isShared)
	if err != nil {
		return err
	}

	// Store it in pendingContracts for jsRegisterMode to retrieve
	tm.contractMu.Lock()
	tm.pendingContracts[modeName] = contract
	tm.contractMu.Unlock()

	return nil
}

// getStateBySymbol retrieves a value from the current mode's state using a Symbol key.
// Implements fallback to shared state if the key is from a shared contract.
func (tm *TUIManager) getStateBySymbol(symbolKey goja.Value) goja.Value {
	tm.mu.RLock()
	currentMode := tm.currentMode
	tm.mu.RUnlock()

	if currentMode == nil {
		return goja.Undefined()
	}

	// Extract the symbol description for lookup
	symbolDesc := normalizeSymbolDescription(symbolKey.String())
	if symbolDesc == "" {
		return goja.Undefined()
	}

	// Step 1: Look in the current mode's state
	currentMode.mu.RLock()
	val, ok := currentMode.State[symbolKey]
	currentMode.mu.RUnlock()

	if ok {
		return tm.engine.vm.ToValue(val)
	}

	// Step 2: If not found, check if this is a shared state key
	tm.engine.symbolRegistry.RLock()
	def, isRegistered := tm.engine.symbolRegistry.registry[symbolDesc]
	tm.engine.symbolRegistry.RUnlock()

	if isRegistered {
		// Check if the key belongs to a shared contract using the persistent sharedContracts list
		tm.contractMu.Lock()
		for _, contract := range tm.sharedContracts {
			if _, exists := contract.Definitions[symbolDesc]; exists {
				// This is a shared key, check sharedState
				tm.contractMu.Unlock()
				tm.sharedMu.RLock()
				sharedVal, sharedOk := tm.sharedState[symbolKey]
				tm.sharedMu.RUnlock()

				if sharedOk {
					return tm.engine.vm.ToValue(sharedVal)
				}

				// Not in shared state either, return default
				if def.DefaultValue != nil {
					return tm.engine.vm.ToValue(def.DefaultValue)
				}
				return goja.Undefined()
			}
		}
		tm.contractMu.Unlock()

		// Not a shared key, check for default value in current mode's contract
		if currentMode.StateContract != nil {
			if def, exists := currentMode.StateContract.Definitions[symbolDesc]; exists {
				if def.DefaultValue != nil {
					return tm.engine.vm.ToValue(def.DefaultValue)
				}
			}
		}
	}

	return goja.Undefined()
}

// setStateBySymbol sets a value in the current mode's state using a Symbol key.
// If the key belongs to a shared contract, writes to shared state instead.
func (tm *TUIManager) setStateBySymbol(symbolKey goja.Value, value interface{}) {
	tm.mu.RLock()
	currentMode := tm.currentMode
	tm.mu.RUnlock()

	if currentMode == nil {
		return
	}

	// Extract the symbol description for lookup
	symbolDesc := normalizeSymbolDescription(symbolKey.String())
	if symbolDesc == "" {
		return
	}

	// Check if this is a shared state key using the persistent sharedContracts list
	tm.contractMu.Lock()
	isShared := false
	for _, contract := range tm.sharedContracts {
		if _, exists := contract.Definitions[symbolDesc]; exists {
			isShared = true
			break
		}
	}
	tm.contractMu.Unlock()

	if isShared {
		// Write to shared state
		tm.sharedMu.Lock()
		if tm.sharedState == nil {
			tm.sharedState = make(map[goja.Value]interface{})
		}
		tm.sharedState[symbolKey] = value
		tm.sharedMu.Unlock()
	} else {
		// Write to mode-specific state
		currentMode.mu.Lock()
		if currentMode.State == nil {
			currentMode.State = make(map[goja.Value]interface{})
		}
		currentMode.State[symbolKey] = value
		currentMode.mu.Unlock()
	}
}

// initModeState initializes a mode's state to its contract's default values.
func (tm *TUIManager) initModeState(mode *ScriptMode) {
	if mode.StateContract == nil {
		return
	}

	mode.mu.Lock()
	defer mode.mu.Unlock()

	// Ensure the in-memory map is initialized
	if mode.State == nil {
		mode.State = make(map[goja.Value]interface{})
	}

	// **CRITICAL FIX**: For shared contracts, do NOT initialize defaults in mode.State!
	// Shared state lives in tm.sharedState, not in individual mode.State maps.
	if mode.StateContract.IsShared {
		// Shared contracts don't get mode-level defaults
		return
	}

	// Iterate over the contract definitions
	for _, def := range mode.StateContract.Definitions {
		// Check if the Symbol-keyed value exists in the current state
		if _, ok := mode.State[def.Symbol]; !ok {
			// If not present, set the default value
			if def.DefaultValue != nil {
				mode.State[def.Symbol] = def.DefaultValue
			}
		}
	}
}

// === TEST HELPER METHODS ===
// These methods provide test-only access to state using string keys.
// They bridge the gap between Go tests and the Symbol-based state system.

// SetStateForTest sets a state value using a persistent string key (for testing only).
// This method looks up the Symbol associated with the string key and sets the value.
func (tm *TUIManager) SetStateForTest(persistentKey string, value interface{}) error {
	tm.engine.symbolRegistry.RLock()
	def, exists := tm.engine.symbolRegistry.registry[persistentKey]
	tm.engine.symbolRegistry.RUnlock()

	if !exists {
		return fmt.Errorf("state key '%s' not found in registry", persistentKey)
	}

	tm.setStateBySymbol(def.Symbol, value)
	return nil
}

// GetStateForTest retrieves a state value using a persistent string key (for testing only).
// This method looks up the Symbol associated with the string key and returns the value.
func (tm *TUIManager) GetStateForTest(persistentKey string) (interface{}, error) {
	tm.engine.symbolRegistry.RLock()
	def, exists := tm.engine.symbolRegistry.registry[persistentKey]
	tm.engine.symbolRegistry.RUnlock()

	if !exists {
		return nil, fmt.Errorf("state key '%s' not found in registry", persistentKey)
	}

	val := tm.getStateBySymbol(def.Symbol)
	if goja.IsUndefined(val) || goja.IsNull(val) {
		return nil, nil
	}

	return val.Export(), nil
}

// captureHistorySnapshot serializes the current mode's state and logs it as a history entry.
// Phase 4.3: Updated to collect complete application state (all modes + shared) and
// delegate to StateManager.CaptureSnapshot for persistence.
func (tm *TUIManager) captureHistorySnapshot(commandName string, commandArgs []string) {
	if tm.stateManager == nil {
		// No state manager, skip persistence
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

	// Collect complete application state from all modes + shared state
	stateMap := make(map[string]string)

	// 1. Serialize shared state
	tm.sharedMu.RLock()
	sharedStateCopy := make(map[goja.Value]interface{}, len(tm.sharedState))
	for k, v := range tm.sharedState {
		sharedStateCopy[k] = v
	}
	tm.sharedMu.RUnlock()

	if len(sharedStateCopy) > 0 {
		sharedJSON, err := SerializeState(tm.engine.symbolRegistry, tm.engine.vm, sharedStateCopy)
		if err != nil {
			fmt.Fprintf(tm.output, "Warning: Failed to serialize shared state for history: %v\n", err)
		} else {
			stateMap["__shared__"] = sharedJSON
		}
	}

	// 2. Serialize all mode states
	tm.mu.RLock()
	modes := make(map[string]*ScriptMode, len(tm.modes))
	for name, mode := range tm.modes {
		modes[name] = mode
	}
	tm.mu.RUnlock()

	for modeName, mode := range modes {
		mode.mu.RLock()
		modeCopy := make(map[goja.Value]interface{}, len(mode.State))
		for k, v := range mode.State {
			modeCopy[k] = v
		}
		mode.mu.RUnlock()

		if len(modeCopy) > 0 {
			modeJSON, err := SerializeState(tm.engine.symbolRegistry, tm.engine.vm, modeCopy)
			if err != nil {
				fmt.Fprintf(tm.output, "Warning: Failed to serialize state for mode %s: %v\n", modeName, err)
			} else {
				stateMap[modeName] = modeJSON
			}
		}
	}

	// Delegate to StateManager to capture the snapshot with contract hashes
	modeID := currentMode.Name
	if err := tm.stateManager.CaptureSnapshot(modeID, cmdString, stateMap); err != nil {
		fmt.Fprintf(tm.output, "Warning: Failed to capture history snapshot: %v\n", err)
	}
}

// === TEST-ONLY HELPERS ===
// These methods provide test-only access to state through the Symbol registry.
// Unlike the removed tui.getState/setState production APIs, these are explicitly
// test-only and use the same Symbol lookup mechanism that production code uses.

// GetStateViaJS retrieves a state value using a persistent string key (for testing only).
// This method looks up the Symbol via the global registry (the same mechanism JS code uses)
// and retrieves the value through the standard state accessor path.
func (tm *TUIManager) GetStateViaJS(persistentKey string) (interface{}, error) {
	tm.engine.symbolRegistry.RLock()
	def, exists := tm.engine.symbolRegistry.registry[persistentKey]
	tm.engine.symbolRegistry.RUnlock()

	if !exists {
		return nil, fmt.Errorf("state key '%s' not found in registry", persistentKey)
	}

	val := tm.getStateBySymbol(def.Symbol)
	if goja.IsUndefined(val) || goja.IsNull(val) {
		return nil, nil
	}

	return val.Export(), nil
}

// SetStateViaJS sets a state value using a persistent string key (for testing only).
// This method looks up the Symbol via the global registry (the same mechanism JS code uses)
// and sets the value through the standard state accessor path.
func (tm *TUIManager) SetStateViaJS(persistentKey string, value interface{}) error {
	tm.engine.symbolRegistry.RLock()
	def, exists := tm.engine.symbolRegistry.registry[persistentKey]
	tm.engine.symbolRegistry.RUnlock()

	if !exists {
		return fmt.Errorf("state key '%s' not found in registry", persistentKey)
	}

	tm.setStateBySymbol(def.Symbol, value)
	return nil
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

// resetSharedState atomically clears and re-initializes the global sharedState
// map based on the default values in all registered shared contracts.
func (tm *TUIManager) resetSharedState() {
	tm.contractMu.Lock()
	defer tm.contractMu.Unlock()

	tm.sharedMu.Lock()
	defer tm.sharedMu.Unlock()

	// Create a new map to hold the reset state
	newState := make(map[goja.Value]interface{})

	// Iterate over all registered shared contracts
	for _, contract := range tm.sharedContracts {
		// Populate the new state map with default values
		for _, def := range contract.Definitions {
			if def.DefaultValue != nil {
				newState[def.Symbol] = def.DefaultValue
			}
		}
	}

	// Atomically replace the old state map with the new one
	tm.sharedState = newState
}

// resetAllModeStates iterates over every registered ScriptMode and atomically
// resets its mode-specific state to its contract's default values.
func (tm *TUIManager) resetAllModeStates() {
	// Read-lock the main TUIManager to safely iterate the tm.modes map
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	for _, mode := range tm.modes {
		// Acquire a write-lock for each specific mode to reset its state
		mode.mu.Lock()

		newState := make(map[goja.Value]interface{})

		// Only reset state for modes that have a non-shared contract
		if mode.StateContract != nil && !mode.StateContract.IsShared {
			// Populate the new state map with default values
			for _, def := range mode.StateContract.Definitions {
				if def.DefaultValue != nil {
					newState[def.Symbol] = def.DefaultValue
				}
			}
		}

		// Atomically replace the mode's old state map
		mode.State = newState
		mode.mu.Unlock()
	}
}

// resetAllState performs a complete reset of all in-memory state,
// including shared state and all mode-specific states.
func (tm *TUIManager) resetAllState() {
	tm.resetSharedState()
	tm.resetAllModeStates()
}

// rehydrateContextManager re-populates the ContextManager from a restored state.
// This function looks for an "items" array in the state (as defined by ctxutil.js)
// and re-adds each file-type entry to the ContextManager.
//
// This is called after state deserialization to ensure the Go-side ContextManager
// is synchronized with the persisted JavaScript state, making context commands
// like remove and toTxtar functional across session restarts.
func (tm *TUIManager) rehydrateContextManager(restoredState map[goja.Value]interface{}, contract *StateContract) {
	if contract == nil || tm.engine == nil || tm.engine.contextManager == nil {
		return
	}

	// Step 1: Locate the "items" symbol in the contract
	var itemsSymbolString string
	for persistentKey, def := range contract.Definitions {
		// The persistent key has format "namespace:key", we need to match the key part
		if strings.HasSuffix(persistentKey, ":items") || persistentKey == "items" {
			// After deserialization, state keys are strings (Symbol.toString() representation)
			itemsSymbolString = def.Symbol.String()
			break
		}
	}

	if itemsSymbolString == "" {
		// No "items" key in this contract, nothing to rehydrate
		return
	}

	// Step 2: Extract the items array from the restored state
	// We must iterate and match against the string representation
	var itemsValue interface{}
	var exists bool
	for k, v := range restoredState {
		keyString := k.String()
		if keyString == itemsSymbolString {
			itemsValue = v
			exists = true
			break
		}
	}

	if !exists {
		// No items in the restored state
		return
	}

	// Step 3: Convert the JavaScript array to a Go slice
	var items []map[string]interface{}
	itemsGojaValue := tm.engine.vm.ToValue(itemsValue)
	if err := tm.engine.vm.ExportTo(itemsGojaValue, &items); err != nil {
		fmt.Fprintf(tm.output, "Warning: Failed to convert items array for context rehydration: %v\n", err)
		return
	}

	// Step 4: Iterate through items and re-populate the ContextManager
	for _, item := range items {
		// Verify the item has the expected structure
		itemType, hasType := item["type"].(string)
		label, hasLabel := item["label"].(string)

		if !hasType || !hasLabel {
			// Skip malformed items
			continue
		}

		// Only process file-type items
		if itemType == "file" {
			if err := tm.engine.contextManager.AddPath(label); err != nil {
				// If the file no longer exists, log it but continue
				if os.IsNotExist(err) {
					fmt.Fprintf(tm.output, "Info: file from previous session not found: %s\n", label)
				} else {
					fmt.Fprintf(tm.output, "Info: could not restore context file %s: %v\n", label, err)
				}
			}
		}
	}
}
