//go:generate go run ../../../internal/cmd/generate-bubbletea-key-mapping

// Package bubbletea provides JavaScript bindings for charm.land/bubbletea/v2.
//
// The module is exposed as "osm:bubbletea" and provides TUI program capabilities.
// All functionality is exposed to JavaScript, following the established pattern of no global state.
//
// # JavaScript API
//
//	const tea = require('osm:bubbletea');
//
//	// Create a program with a model
//	const model = tea.newModel({
//		init: function() {
//			return { count: 0 };
//		},
//		update: function(msg, model) {
//			switch (msg.type) {
//				case 'Key':
//					if (msg.key === 'q') return [model, tea.quit()];
//					if (msg.key === 'up') return [{ count: model.count + 1 }, null];
//					if (msg.key === 'down') return [{ count: model.count - 1 }, null];
//					break;
//				case 'Focus':
//					// Terminal gained focus
//					break;
//				case 'Blur':
//					// Terminal lost focus
//					break;
//			}
//			return [model, null];
//		},
//		view: function(model) {
//			return 'Count: ' + model.count + '\nPress q to quit';
//			// Or return a declarative object for terminal control:
//			return {
//				content: 'Count: ' + model.count + '\nPress q to quit',
//				altScreen: true,
//				mouseMode: 'allMotion', // 'cellMotion' or 'none'
//				reportFocus: true,
//				windowTitle: 'My App',
//				cursor: { x: 10, y: 5, shape: 'bar', blink: true, color: '#ff0000' },
//				foregroundColor: '#ffffff',
//				backgroundColor: '#000000',
//				keyboardEnhancements: { reportEventTypes: true }, // or just true
//				disableBracketedPasteMode: true,
//				progressBar: { state: 'default', value: 42 }
//			};
//		}
//	});
//
//	// Run the program — returns immediately (non-blocking)
//	tea.run(model);                          // basic inline screen
//	tea.run(model, { toggleKey: 29, onToggle: fn }); // with Termux passthrough
//
//	// Wait for the program started by run() to fully exit (Promise)
//	tea.waitForProgram().then(function () { /* safe to start the next one */ });
//
//	// Commands - all return opaque command objects
//	tea.quit();                    // Quit the program
//	tea.clearScreen();             // Clear the screen
//	tea.batch(...cmds);            // Batch multiple commands
//	tea.sequence(...cmds);         // Execute commands in sequence
//	tea.tick(durationMs, id);      // Timer command (returns tickMsg with id)
//	tea.requestWindowSize();       // Query current window size
//	tea.requestBackgroundColor();  // Query the terminal background colour
//
//	// Key events — msg.type === 'Key'
//	// msg.key    - key name ('q', 'enter', 'space', 'ctrl+c', etc.)
//	// msg.text   - text value (rune or text for printable chars)
//	// msg.mod    - modifier array (['ctrl'], ['alt', 'shift'], etc.)
//	// msg.code   - key code rune
//	// msg.shiftedCode - shifted key code rune
//	// msg.baseCode    - base key code rune (US PC-101 layout)
//	// msg.isRepeat    - true if key is auto-repeating
//
//	// KeyRelease events — msg.type === 'KeyRelease' (same fields as Key)
//
//	// Mouse events — msg.type is one of:
//	// msg.type === 'MouseClick'  — button click (msg.x, msg.y, msg.button, msg.mod)
//	// msg.type === 'MouseRelease' — button release
//	// msg.type === 'MouseMotion' — movement (no button)
//	// msg.type === 'MouseWheel'  — scroll event
//
//	// Window size events — msg.type === 'WindowSize'
//	// msg.width, msg.height - terminal dimensions
//
//	// Focus events — requires view() to return {reportFocus: true}
//	// msg.type === 'Focus' - terminal gained focus
//	// msg.type === 'Blur'  - terminal lost focus
//
//	// Tick events — from tea.tick() command
//	// msg.type === 'Tick' with msg.id and msg.time (ms since epoch)
//
//	// Paste events — bracketed paste content arrives as separate messages
//	// msg.type === 'Paste'     with msg.content
//	// msg.type === 'PasteStart' — paste sequence started
//	// msg.type === 'PasteEnd'   — paste sequence ended
//
//	// Background colour events — answer to tea.requestBackgroundColor()
//	// msg.type === 'BackgroundColor' with msg.isDark and msg.rgb, where rgb is
//	// the OSC 11 payload form "RRRR/GGGG/BBBB" ("" when the terminal reported
//	// no colour). The view-side {backgroundColor} field is unrelated: it sets
//	// the colour this program renders with, while this message reports what
//	// the terminal said about itself.
//
// # View Return Value
//
// The view() function can return a string or a declarative object:
//
//	String return:        view() returns 'content'
//	Object return:        view() returns {content:'...', altScreen:true, ...}
//
// All declarative view fields:
//
//	content        string   - rendered content string (required)
//	altScreen      bool     - enable alternate screen buffer
//	mouseMode      string   - "allMotion", "cellMotion", or "none"
//	reportFocus    bool     - enable focus/blur events
//	windowTitle    string   - terminal window title
//	cursor         object   - {x, y, shape, blink, color} — show cursor at position
//	foregroundColor string  - terminal foreground color (hex or ANSI 256)
//	backgroundColor string  - terminal background color (hex or ANSI 256)
//	keyboardEnhancements object|bool - {reportEventTypes: true} or just true
//	disableBracketedPasteMode bool  - disable bracketed paste mode
//	progressBar    object   - {state: "default"|"error"|"indeterminate"|"warning"|"none", value: 0-100}
//
// Terminal feature fields are ONLY effective when returned from view() as
// object properties. Passing them to tea.run() as options is silently
// ignored in v2.
//
// # Error Handling
//
// All command functions validate their inputs and return error objects when
// invalid arguments are provided:
//
//	const cmd = tea.tick(-100, 'timer'); // Returns { error: 'BT001: duration must be positive' }
//
// Error codes:
//   - BT001: Invalid duration (tick command)
//   - BT004: Program execution failed
//   - BT005: Invalid model object
//   - BT006: Invalid arguments
//   - BT007: Panic during program execution
//
// # Implementation Notes
//
// All additions follow these patterns:
//
//  1. No global state - all state managed per Manager instance
//  2. JavaScript callbacks properly synchronized with Go goroutines
//  3. All functionality exposed via Require() function pattern
//  4. Commands are opaque objects - JS cannot forge invalid commands
//  5. Comprehensive unit tests using simulation screens
//  6. Deterministic testing - no timing-dependent tests
//  7. Proper TTY detection with fallback for non-TTY environments
//  8. Terminal state cleanup guaranteed even on panic/force-quit
package bubbletea

import (
	"context"
	"fmt"
	"maps"
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	"github.com/joeycumines/goja"
)

// Error codes for bubbletea operations.
const (
	ErrCodeInvalidDuration = "BT001" // Invalid duration (tick command)
	ErrCodeProgramFailed   = "BT004" // Program execution failed
	ErrCodeInvalidModel    = "BT005" // Invalid model object
	ErrCodeInvalidArgs     = "BT006" // Invalid arguments
	ErrCodePanic           = "BT007" // Panic during program execution
)

// commandIDCounter generates unique IDs for command objects.
var commandIDCounter atomic.Uint64

// generateCommandID creates a unique command ID.
func generateCommandID() uint64 {
	return commandIDCounter.Add(1)
}

// WrapCmd wraps a tea.Cmd as an opaque JavaScript value.
// JavaScript receives the Go function wrapped via runtime.ToValue().
// When JavaScript passes this value back, Go can retrieve the original
// tea.Cmd using Export(). NO REGISTRY NEEDED - goja handles this natively.
//
// If cmd is nil, returns goja.Null().
//
// Usage:
//
//	// In viewport/textarea update():
//	newModel, cmd := vp.Update(msg)
//	// update the underlying model instance via pointer.
//	*vp = newModel
//	// return [<model object>, <wrapped cmd>]
//	return runtime.NewArray(
//		// Return the same JS object, which wraps the model instance / hides the Go state.
//		obj,
//		// Wrap the tea.Cmd as an opaque value - JS can pass this back and Go
//		// can retrieve the original function via Export().
//		bubbletea.WrapCmd(runtime, cmd),
//	)
//
//	// In valueToCmd():
//	if exported := val.Export(); exported != nil {
//	    if cmd, ok := exported.(tea.Cmd); ok {
//	        return cmd
//	    }
//	}
func WrapCmd(runtime *goja.Runtime, cmd tea.Cmd) goja.Value {
	if cmd == nil {
		return goja.Null()
	}
	// runtime.ToValue() wraps Go values. For functions, it creates a
	// JavaScript wrapper that, when Export()'ed, returns the original Go value.
	return runtime.ToValue(cmd)
}

// Require returns a CommonJS native module under "osm:bubbletea".
// It exposes bubbletea functionality for building terminal UIs.
func Require(baseCtx context.Context, manager *Manager) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		// Model registry to store models by ID (since goja Export can be unreliable for Go pointers)
		modelRegistry := make(map[uint64]*jsModel)
		var modelRegistryMu sync.Mutex
		var nextModelID uint64

		registerModel := func(model *jsModel) uint64 {
			modelRegistryMu.Lock()
			defer modelRegistryMu.Unlock()
			nextModelID++
			modelRegistry[nextModelID] = model
			return nextModelID
		}

		getModel := func(id uint64) *jsModel {
			modelRegistryMu.Lock()
			defer modelRegistryMu.Unlock()
			return modelRegistry[id]
		}

		// Helper to create command with unique ID
		createCommand := func(cmdType string, props map[string]any) goja.Value {
			cmdID := generateCommandID()
			result := map[string]any{
				"_cmdType": cmdType,
				"_cmdID":   cmdID,
			}
			maps.Copy(result, props)
			return runtime.ToValue(result)
		}

		// Helper to create error response
		createError := func(code, message string) goja.Value {
			return runtime.ToValue(map[string]any{
				"error":     fmt.Sprintf("%s: %s", code, message),
				"errorCode": code,
			})
		}

		// isTTY returns whether the terminal is a TTY
		_ = exports.Set("isTTY", func(call goja.FunctionCall) goja.Value {
			return runtime.ToValue(manager.IsTTY())
		})

		// keys exposes the key definitions for JS to access key metadata
		// This allows JS code to look up key information by string value or name
		keysObj := runtime.NewObject()
		for _, stringVal := range slices.Sorted(maps.Keys(KeyDefs)) {
			keyDef := KeyDefs[stringVal]
			keyDefJS := runtime.NewObject()
			_ = keyDefJS.Set("name", keyDef.Name)
			_ = keyDefJS.Set("string", keyDef.String)
			// Type is exposed as the string representation for JS convenience
			_ = keyDefJS.Set("type", keyDef.String)
			_ = keysObj.Set(stringVal, keyDefJS)
		}
		_ = exports.Set("keys", keysObj)

		// keysByName exposes key definitions by their Go constant name
		keysByNameObj := runtime.NewObject()
		for _, name := range slices.Sorted(maps.Keys(KeyDefsByName)) {
			keyDef := KeyDefsByName[name]
			keyDefJS := runtime.NewObject()
			_ = keyDefJS.Set("name", keyDef.Name)
			_ = keyDefJS.Set("string", keyDef.String)
			_ = keyDefJS.Set("type", keyDef.String)
			_ = keysByNameObj.Set(name, keyDefJS)
		}
		_ = exports.Set("keysByName", keysByNameObj)

		// mouseButtons exposes mouse button definitions for JS to access button metadata
		// The keys are the string representations (e.g., "left", "wheel up")
		mouseButtonsObj := runtime.NewObject()
		for _, stringVal := range slices.Sorted(maps.Keys(MouseButtonDefs)) {
			buttonDef := MouseButtonDefs[stringVal]
			buttonDefJS := runtime.NewObject()
			_ = buttonDefJS.Set("name", buttonDef.Name)
			_ = buttonDefJS.Set("string", buttonDef.String)
			_ = buttonDefJS.Set("isWheel", IsWheelButton(buttonDef.Button))
			_ = mouseButtonsObj.Set(stringVal, buttonDefJS)
		}
		_ = exports.Set("mouseButtons", mouseButtonsObj)

		// mouseButtonsByName exposes mouse button definitions by their Go constant name
		// (e.g., "MouseButtonLeft", "MouseButtonWheelUp")
		mouseButtonsByNameObj := runtime.NewObject()
		for _, name := range slices.Sorted(maps.Keys(MouseButtonDefsByName)) {
			buttonDef := MouseButtonDefsByName[name]
			buttonDefJS := runtime.NewObject()
			_ = buttonDefJS.Set("name", buttonDef.Name)
			_ = buttonDefJS.Set("string", buttonDef.String)
			_ = buttonDefJS.Set("isWheel", IsWheelButton(buttonDef.Button))
			_ = mouseButtonsByNameObj.Set(name, buttonDefJS)
		}
		_ = exports.Set("mouseButtonsByName", mouseButtonsByNameObj)

		// isValidTextareaInput validates if a key event should be forwarded to a textarea.
		// Uses WHITELIST approach: only explicitly allowed inputs pass through.
		// This prevents garbage (fragmented escape sequences) from corrupting content.
		// Parameters: keyStr (string)
		// Returns: { valid: boolean, reason: string }
		_ = exports.Set("isValidTextareaInput", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return runtime.ToValue(map[string]any{
					"valid":  false,
					"reason": "missing keyStr argument",
				})
			}
			keyStr := call.Argument(0).String()
			result := ValidateTextareaInput(keyStr)
			return runtime.ToValue(map[string]any{
				"valid":  result.Valid,
				"reason": result.Reason,
			})
		})

		// isValidLabelInput validates if a key event should be accepted for a label field.
		// More restrictive: only single printable characters and backspace.
		// Parameters: keyStr (string)
		// Returns: { valid: boolean, reason: string }
		_ = exports.Set("isValidLabelInput", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return runtime.ToValue(map[string]any{
					"valid":  false,
					"reason": "missing keyStr argument",
				})
			}
			keyStr := call.Argument(0).String()
			result := ValidateLabelInput(keyStr)
			return runtime.ToValue(map[string]any{
				"valid":  result.Valid,
				"reason": result.Reason,
			})
		})

		// newModel creates a new model wrapper from JS definition
		_ = exports.Set("newModel", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return createError(ErrCodeInvalidArgs, "newModel requires a config object")
			}

			configArg := call.Argument(0)
			if configArg == nil || goja.IsUndefined(configArg) || goja.IsNull(configArg) {
				return createError(ErrCodeInvalidArgs, "config must be an object")
			}

			config := configArg.ToObject(runtime)
			if config == nil {
				return createError(ErrCodeInvalidArgs, "config must be an object")
			}

			initFn, ok := goja.AssertFunction(config.Get("init"))
			if !ok {
				return createError(ErrCodeInvalidArgs, "init must be a function")
			}

			updateFn, ok := goja.AssertFunction(config.Get("update"))
			if !ok {
				return createError(ErrCodeInvalidArgs, "update must be a function")
			}

			viewFn, ok := goja.AssertFunction(config.Get("view"))
			if !ok {
				return createError(ErrCodeInvalidArgs, "view must be a function")
			}

			// Get JSRunner from manager if available (for thread-safe JS execution)
			var jsRunner JSRunner
			if manager != nil {
				jsRunner = manager.GetJSRunner()
			}

			model := &jsModel{
				runtime:  runtime,
				initFn:   initFn,
				updateFn: updateFn,
				viewFn:   viewFn,
				state:    runtime.NewObject(), // Initialize with empty object to avoid nil
				jsRunner: jsRunner,
			}

			// Parse optional renderThrottle configuration
			// Usage: { renderThrottle: { enabled: true, minIntervalMs: 16, alwaysRenderMsgTypes: ["Tick", "WindowSize"] } }
			if throttleVal := config.Get("renderThrottle"); throttleVal != nil && !goja.IsUndefined(throttleVal) && !goja.IsNull(throttleVal) {
				throttleObj := throttleVal.ToObject(runtime)
				if throttleObj != nil {
					// Check if enabled
					if enabledVal := throttleObj.Get("enabled"); enabledVal != nil && !goja.IsUndefined(enabledVal) {
						model.throttleEnabled = enabledVal.ToBoolean()
					}
					// Parse minIntervalMs (default: 16ms ~= 60fps)
					model.throttleIntervalMs = 16
					if intervalVal := throttleObj.Get("minIntervalMs"); intervalVal != nil && !goja.IsUndefined(intervalVal) {
						model.throttleIntervalMs = max(intervalVal.ToInteger(), 1)
					}
					// Parse alwaysRenderMsgTypes (message types that bypass throttling)
					model.alwaysRenderTypes = make(map[string]bool)
					// Default: always render Tick and WindowSize immediately
					model.alwaysRenderTypes["Tick"] = true
					model.alwaysRenderTypes["WindowSize"] = true
					if typesVal := throttleObj.Get("alwaysRenderMsgTypes"); typesVal != nil && !goja.IsUndefined(typesVal) {
						typesObj := typesVal.ToObject(runtime)
						if typesObj != nil && typesObj.ClassName() == "Array" {
							// Clear defaults and use user-provided list
							model.alwaysRenderTypes = make(map[string]bool)
							for _, key := range typesObj.Keys() {
								if val := typesObj.Get(key); val != nil && !goja.IsUndefined(val) {
									model.alwaysRenderTypes[val.String()] = true
								}
							}
						}
					}
				}
			}

			// Register model and store its ID (more reliable than storing pointer via goja)
			modelID := registerModel(model)

			// Return a wrapper object with the model ID
			wrapper := runtime.NewObject()
			_ = wrapper.Set("_modelID", runtime.ToValue(modelID))
			_ = wrapper.Set("_type", "bubbleteaModel")

			// Add test helper to get the actual model (for unit testing only)
			_ = wrapper.Set("_getModel", func(call goja.FunctionCall) goja.Value {
				return runtime.ToValue(getModel(modelID))
			})

			return wrapper
		})

		// run executes a bubbletea program
		_ = exports.Set("run", func(call goja.FunctionCall) (result goja.Value) {
			// Add panic recovery with detailed logging
			defer func() {
				if r := recover(); r != nil {
					stackTrace := debug.Stack()
					// Log detailed error to stderr if available
					if manager != nil && manager.stderr != nil {
						_, _ = fmt.Fprintf(manager.stderr, "\n[PANIC] in bubbletea.run(): %v\n\nStack:\n%s\n", r, string(stackTrace))
					}
					result = createError(ErrCodePanic, fmt.Sprintf("panic in run: %v", r))
				}
			}()

			if len(call.Arguments) < 1 {
				return createError(ErrCodeInvalidArgs, "run requires a model")
			}

			argVal := call.Argument(0)
			if goja.IsUndefined(argVal) || goja.IsNull(argVal) {
				return createError(ErrCodeInvalidModel, "model must be an object")
			}
			modelWrapper := argVal.ToObject(runtime)
			if modelWrapper == nil {
				return createError(ErrCodeInvalidModel, "model must be an object")
			}

			typeVal := modelWrapper.Get("_type")
			if typeVal == nil || goja.IsUndefined(typeVal) || goja.IsNull(typeVal) {
				return createError(ErrCodeInvalidModel, "invalid model object")
			}
			if typeVal.String() != "bubbleteaModel" {
				return createError(ErrCodeInvalidModel, "invalid model object")
			}

			modelIDVal := modelWrapper.Get("_modelID")
			if modelIDVal == nil || goja.IsUndefined(modelIDVal) || goja.IsNull(modelIDVal) {
				return createError(ErrCodeInvalidModel, "failed to extract model ID")
			}
			modelID := uint64(modelIDVal.ToInteger())
			model := getModel(modelID)
			if model == nil {
				return createError(ErrCodeInvalidModel, "model not found in registry")
			}

			// In v2, terminal features (altScreen, mouse, etc.) are declarative
			// View fields — set via the JS view() return object, NOT tea.run()
			// options. Only toggleKey/onToggle remain as run() options.
			// Terminal feature options (altScreen, mouse, mouseCellMotion, reportFocus,
			// windowTitle) are SILENTLY IGNORED here. They must be set by having
			// the JS view() function return an object with those properties.
			var toggleWrapper *toggleModel
			if len(call.Arguments) > 1 {
				optObj := call.Argument(1).ToObject(runtime)
				if optObj != nil {
					// Toggle key support — integrates with termmux passthrough.
					// When toggleKey and onToggle are both set, the model is wrapped
					// in a toggleModel that intercepts the toggle key and executes
					// the JS callback between ReleaseTerminal/RestoreTerminal calls.
					toggleKeyVal := optObj.Get("toggleKey")
					onToggleVal := optObj.Get("onToggle")
					if toggleKeyVal != nil && !goja.IsUndefined(toggleKeyVal) && !goja.IsNull(toggleKeyVal) &&
						onToggleVal != nil && !goja.IsUndefined(onToggleVal) && !goja.IsNull(onToggleVal) {
						toggleKeyInt := toggleKeyVal.ToInteger()
						if toggleKeyInt <= 0 || toggleKeyInt > 255 {
							return createError(ErrCodeInvalidArgs, "toggleKey must be a byte in range 1-255")
						}
						toggleKey := byte(toggleKeyInt)
						onToggleFn, ok := goja.AssertFunction(onToggleVal)
						if !ok {
							return createError(ErrCodeInvalidArgs, "onToggle must be a function")
						}
						toggleWrapper = &toggleModel{
							inner:     model,
							toggleKey: toggleKey,
							onToggle:  onToggleFn,
							jsRunner:  manager.GetJSRunner(),
							ctx:       manager.ctx,
							output:    manager.output,
						}
					}
				}
			}

			// Validate manager is not nil (model was validated above during registry lookup)
			if manager == nil {
				return createError(ErrCodeProgramFailed, "manager is nil")
			}

			// Determine the actual model to run: toggle-wrapped or plain
			var programModel tea.Model = model
			if toggleWrapper != nil {
				programModel = toggleWrapper
			}

			// Start the program NON-BLOCKING. BubbleTea runs in a separate
			// goroutine so this function returns immediately — the Goja event
			// loop goroutine is NOT blocked and can process RunSync
			// callbacks from BubbleTea's Init/Update/View.
			//
			// Threading model:
			// - ExecuteScript routes scripts through the event loop so ALL
			//   Goja VM access happens on a single goroutine.
			// - tea.run() returns immediately; BubbleTea runs on its own goroutines.
			// - BubbleTea calls Init/Update/View via JSRunner.RunSync()
			//   which schedules on the event loop goroutine.
			// - ExecuteScript and the REPL command dispatcher automatically
			//   call WaitForProgram() to block until BubbleTea exits.
			//
			// Previously, tea.run() blocked the event loop goroutine, which
			// prevented async operations (exec.spawn, Promises) from resolving
			// — causing the "Processing…" hang in pr-split.

			// Reserve the programDone slot synchronously (on the event loop
			// goroutine) to prevent a TOCTOU race: a rapid second tea.run()
			// call could otherwise bypass the "already running" guard in
			// runProgram before the first goroutine sets m.program.
			done := make(chan error, 1)
			manager.mu.Lock()
			if manager.programDone != nil {
				manager.mu.Unlock()
				return createError(ErrCodeProgramFailed, "a BubbleTea program is already running")
			}
			manager.programDone = done
			manager.mu.Unlock()

			// Run the program. Promisify keeps the event loop alive while the program runs.
			// This prevents the event loop from auto-exiting while the TUI is active.
			if manager.promisify == nil {
				panic("bubbletea.Manager.promisify is REQUIRED - ensure Engine.Register was called")
			}

			manager.promisify(baseCtx, func(_ context.Context) (any, error) {
				err := manager.runProgram(programModel)
				// Also send to done channel for WaitForProgram compatibility.
				// This is safe because done is buffered (capacity 1).
				select {
				case done <- err:
				default:
				}
				return nil, err
			})

			return goja.Undefined()
		})

		// quit returns a quit command
		_ = exports.Set("quit", func(call goja.FunctionCall) goja.Value {
			return createCommand("quit", nil)
		})

		// waitForProgram() → Promise<void>
		// Resolves once the current BubbleTea program has fully exited
		// (immediately when none is running), rejects if it exited with an
		// error. run() is non-blocking and reserves the program slot
		// synchronously — call this AFTER run() from an ASYNC CHAIN when the
		// next step must not race the dying program (the engine's own
		// WaitForProgram runs after the script body settles and shares this
		// slot: a bare top-level call in a synchronous script can race it —
		// one buffered send, two consumers). Blocked work runs off the event
		// loop via Adapter.Promisify, which also keeps the loop alive while
		// a program it is waiting on still needs RunSync callbacks.
		_ = exports.Set("waitForProgram", func() goja.Value {
			a := manager.getAdapter()
			if a == nil {
				panic(runtime.NewTypeError("waitForProgram: engine adapter is not configured"))
			}
			return a.Promisify(baseCtx, func(_ context.Context) (any, error) {
				return nil, manager.WaitForProgram()
			})
		})

		// clearScreen returns a clear screen command
		_ = exports.Set("clearScreen", func(call goja.FunctionCall) goja.Value {
			return createCommand("clearScreen", nil)
		})

		// batch combines multiple commands
		_ = exports.Set("batch", func(call goja.FunctionCall) goja.Value {
			cmds := make([]any, 0, len(call.Arguments))
			for _, arg := range call.Arguments {
				if !goja.IsUndefined(arg) && !goja.IsNull(arg) {
					cmds = append(cmds, arg.Export())
				}
			}
			return createCommand("batch", map[string]any{"cmds": cmds})
		})

		// sequence executes commands in sequence
		_ = exports.Set("sequence", func(call goja.FunctionCall) goja.Value {
			cmds := make([]any, 0, len(call.Arguments))
			for _, arg := range call.Arguments {
				if !goja.IsUndefined(arg) && !goja.IsNull(arg) {
					cmds = append(cmds, arg.Export())
				}
			}
			return createCommand("sequence", map[string]any{"cmds": cmds})
		})

		// tick returns a timer command
		_ = exports.Set("tick", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return createError(ErrCodeInvalidArgs, "tick requires duration in milliseconds")
			}

			durationMs := call.Argument(0).ToInteger()
			if durationMs <= 0 {
				return createError(ErrCodeInvalidDuration, "duration must be positive")
			}

			id := ""
			if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
				id = call.Argument(1).String()
			}

			return createCommand("tick", map[string]any{
				"duration": durationMs,
				"id":       id,
			})
		})

		// NOTE: The following v1 imperative commands are REMOVED in v2.
		// requestWindowSize queries the current window size (v2 API).
		// Returns tea.RequestWindowSize — the program will receive a WindowSizeMsg.
		_ = exports.Set("requestWindowSize", func(call goja.FunctionCall) goja.Value {
			return createCommand("requestWindowSize", nil)
		})

		// requestBackgroundColor queries the terminal background colour (v2 API).
		// Returns tea.RequestBackgroundColor — the program will receive a
		// BackgroundColor message with { isDark, rgb }.
		_ = exports.Set("requestBackgroundColor", func(call goja.FunctionCall) goja.Value {
			return createCommand("requestBackgroundColor", nil)
		})
	}
}
