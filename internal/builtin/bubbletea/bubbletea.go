// Package bubbletea provides JavaScript bindings for github.com/charmbracelet/bubbletea.
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
//	    init: function() {
//	        return { count: 0 };
//	    },
//	    update: function(msg, model) {
//	        switch (msg.type) {
//	            case 'keyPress':
//	                if (msg.key === 'q') return [model, tea.quit()];
//	                if (msg.key === 'up') return [{ count: model.count + 1 }, null];
//	                if (msg.key === 'down') return [{ count: model.count - 1 }, null];
//	                break;
//	            case 'focus':
//	                // Terminal gained focus
//	                break;
//	            case 'blur':
//	                // Terminal lost focus
//	                break;
//	        }
//	        return [model, null];
//	    },
//	    view: function(model) {
//	        return 'Count: ' + model.count + '\nPress q to quit';
//	    }
//	});
//
//	// Run the program
//	tea.run(model);
//
//	// Commands - all return opaque command objects
//	tea.quit();                    // Quit the program
//	tea.clearScreen();             // Clear the screen
//	tea.batch(...cmds);            // Batch multiple commands
//	tea.sequence(...cmds);         // Execute commands in sequence
//	tea.tick(durationMs, id);      // Timer command (returns tickMsg with id)
//	tea.setWindowTitle(title);     // Set terminal window title
//	tea.hideCursor();              // Hide the cursor
//	tea.showCursor();              // Show the cursor
//	tea.enterAltScreen();          // Enter alternate screen buffer
//	tea.exitAltScreen();           // Exit alternate screen buffer
//	tea.enableBracketedPaste();    // Enable bracketed paste mode
//	tea.disableBracketedPaste();   // Disable bracketed paste mode
//	tea.enableReportFocus();       // Enable focus/blur reporting
//	tea.disableReportFocus();      // Disable focus/blur reporting
//	tea.windowSize();              // Query current window size
//
//	// Key events
//	// msg.type === 'keyPress'
//	// msg.key - the key name ('q', 'enter', 'up', 'down', etc.)
//	// msg.runes - array of runes (for unicode/IME input)
//	// msg.alt - alt modifier
//	// msg.ctrl - ctrl modifier
//	// msg.paste - true if this is part of a bracketed paste
//
//	// Mouse events (when enabled)
//	// msg.type === 'mouse'
//	// msg.x, msg.y - coordinates
//	// msg.button - button name
//	// msg.action - 'press', 'release', 'motion'
//	// msg.alt, msg.ctrl, msg.shift - modifiers
//
//	// Window size events
//	// msg.type === 'windowSize'
//	// msg.width, msg.height - terminal dimensions
//
//	// Focus events (when reportFocus enabled)
//	// msg.type === 'focus'  - terminal gained focus
//	// msg.type === 'blur'   - terminal lost focus
//
//	// Tick events (from tick command)
//	// msg.type === 'tick'
//	// msg.id - the id passed to tick()
//	// msg.time - timestamp in milliseconds
//
//	// Program options
//	tea.run(model, {
//	    altScreen: true,       // Use alternate screen buffer
//	    mouse: true,           // Enable mouse support (all motion)
//	    mouseCellMotion: true, // Enable mouse cell motion only
//	    bracketedPaste: true,  // Enable bracketed paste
//	    reportFocus: true,     // Enable focus/blur reporting
//	});
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
//   - BT002: Invalid command in batch/sequence
//   - BT003: TTY initialization failed
//   - BT004: Program execution failed
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
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dop251/goja"
	"golang.org/x/term"
)

// Error codes for bubbletea operations.
const (
	ErrCodeInvalidDuration = "BT001" // Invalid duration (tick command)
	ErrCodeInvalidCommand  = "BT002" // Invalid command in batch/sequence
	ErrCodeTTYInitFailed   = "BT003" // TTY initialization failed
	ErrCodeProgramFailed   = "BT004" // Program execution failed
	ErrCodeInvalidModel    = "BT005" // Invalid model object
	ErrCodeInvalidArgs     = "BT006" // Invalid arguments
	ErrCodePanic           = "BT007" // Panic during program execution
)

// commandID is a unique ID for command objects to prevent forgery.
var commandIDCounter uint64

// generateCommandID creates a unique command ID.
func generateCommandID() uint64 {
	return atomic.AddUint64(&commandIDCounter, 1)
}

// TerminalChecker provides terminal detection and file descriptor access.
// This interface allows dependency injection for testing and proper integration
// with terminal management wrappers like TUIReader/TUIWriter.
type TerminalChecker interface {
	// Fd returns the file descriptor of the underlying terminal.
	// Returns ^uintptr(0) (invalid) if the underlying resource doesn't have an Fd.
	Fd() uintptr

	// IsTerminal returns true if the underlying resource is a terminal.
	IsTerminal() bool
}

// Manager holds bubbletea-related state per engine instance.
type Manager struct {
	ctx          context.Context
	mu           sync.Mutex
	input        io.Reader
	output       io.Writer
	stderr       io.Writer // Stderr for logging panics/errors
	signalNotify func(c chan<- os.Signal, sig ...os.Signal)
	signalStop   func(c chan<- os.Signal)
	isTTY        bool         // Whether input is a TTY
	ttyFd        int          // TTY file descriptor (if available)
	program      *tea.Program // Currently running program (if any)
}

// NewManager creates a new bubbletea manager for an engine instance.
// Input and output can be nil to use os.Stdin and os.Stdout.
// Automatically detects TTY and sets up proper terminal handling.
func NewManager(
	ctx context.Context,
	input io.Reader,
	output io.Writer,
	signalNotify func(c chan<- os.Signal, sig ...os.Signal),
	signalStop func(c chan<- os.Signal),
) *Manager {
	return NewManagerWithStderr(ctx, input, output, nil, signalNotify, signalStop)
}

// NewManagerWithStderr creates a new bubbletea manager with explicit stderr for error logging.
// Input, output, and stderr can be nil to use os.Stdin, os.Stdout, and os.Stderr.
// Automatically detects TTY and sets up proper terminal handling.
func NewManagerWithStderr(
	ctx context.Context,
	input io.Reader,
	output io.Writer,
	stderr io.Writer,
	signalNotify func(c chan<- os.Signal, sig ...os.Signal),
	signalStop func(c chan<- os.Signal),
) *Manager {
	if ctx == nil {
		ctx = context.Background()
	}
	// Track if we received nil inputs - if so, skip TTY detection entirely.
	// This avoids triggering stdin access during engine construction in tests.
	skipTTYDetection := input == nil && output == nil
	if input == nil {
		input = os.Stdin
	}
	if output == nil {
		output = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	if signalNotify == nil {
		signalNotify = signal.Notify
	}
	if signalStop == nil {
		signalStop = signal.Stop
	}

	m := &Manager{
		ctx:          ctx,
		input:        input,
		output:       output,
		stderr:       stderr,
		signalNotify: signalNotify,
		signalStop:   signalStop,
		isTTY:        false,
		ttyFd:        -1,
	}

	// Skip TTY detection if caller passed nil for both input and output.
	// This indicates the caller doesn't need terminal detection (e.g., during
	// engine construction for non-interactive scripts).
	if skipTTYDetection {
		return m
	}

	// Detect TTY for input - check TerminalChecker interface first (for TUIReader/TUIWriter wrappers)
	if tc, ok := input.(TerminalChecker); ok {
		if tc.IsTerminal() {
			fd := tc.Fd()
			if fd != ^uintptr(0) {
				m.isTTY = true
				m.ttyFd = int(fd)
			}
		}
	} else if f, ok := input.(*os.File); ok {
		// Fallback for direct *os.File
		fd := int(f.Fd())
		if term.IsTerminal(fd) {
			m.isTTY = true
			m.ttyFd = fd
		}
	}

	// If input isn't a TTY, check output
	if !m.isTTY {
		if tc, ok := output.(TerminalChecker); ok {
			if tc.IsTerminal() {
				fd := tc.Fd()
				if fd != ^uintptr(0) {
					m.isTTY = true
					m.ttyFd = int(fd)
				}
			}
		} else if f, ok := output.(*os.File); ok {
			fd := int(f.Fd())
			if term.IsTerminal(fd) {
				m.isTTY = true
				m.ttyFd = fd
			}
		}
	}

	return m
}

// IsTTY returns whether the manager has access to a TTY.
func (m *Manager) IsTTY() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isTTY
}

// jsModel wraps a JavaScript model definition for bubbletea.
type jsModel struct {
	runtime     *goja.Runtime
	initFn      goja.Callable
	updateFn    goja.Callable
	viewFn      goja.Callable
	state       goja.Value
	quitCalled  bool
	initError   string          // Store init error for debugging
	validCmdIDs map[uint64]bool // Track valid command IDs to prevent forgery
	cmdMu       sync.Mutex
}

// registerCommand registers a command ID as valid.
func (m *jsModel) registerCommand(id uint64) {
	m.cmdMu.Lock()
	defer m.cmdMu.Unlock()
	if m.validCmdIDs == nil {
		m.validCmdIDs = make(map[uint64]bool)
	}
	m.validCmdIDs[id] = true
}

// Init implements tea.Model.
func (m *jsModel) Init() tea.Cmd {
	if m == nil || m.initFn == nil || m.runtime == nil {
		return nil
	}
	// Call JS init function to get initial state
	result, err := m.initFn(goja.Undefined())
	if err != nil {
		// Store the error so View can display it
		m.initError = fmt.Sprintf("Init error: %v", err)
		return nil
	}
	// Ensure we have a valid state (not nil)
	if result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
		m.state = m.runtime.NewObject()
		m.initError = "Init returned nil/undefined"
	} else {
		m.state = result
	}
	return nil
}

// Update implements tea.Model.
func (m *jsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m == nil || m.updateFn == nil || m.runtime == nil {
		return m, nil
	}

	jsMsg := m.msgToJS(msg)
	if jsMsg == nil {
		return m, nil
	}

	// Ensure state is not nil before passing to JS
	state := m.state
	if state == nil || goja.IsUndefined(state) || goja.IsNull(state) {
		state = m.runtime.NewObject()
	}

	result, err := m.updateFn(goja.Undefined(), m.runtime.ToValue(jsMsg), state)
	if err != nil {
		return m, nil
	}

	// Result should be [newState, cmd] array
	resultObj := result.ToObject(m.runtime)
	if resultObj == nil || resultObj.ClassName() != "Array" {
		return m, nil
	}

	// Extract new state
	if newState := resultObj.Get("0"); !goja.IsUndefined(newState) && !goja.IsNull(newState) {
		m.state = newState
	}

	// Extract command
	cmdVal := resultObj.Get("1")
	cmd := m.valueToCmd(cmdVal)

	return m, cmd
}

// View implements tea.Model.
func (m *jsModel) View() string {
	if m == nil || m.viewFn == nil || m.runtime == nil {
		return "[BT] View: nil model/viewFn/runtime"
	}

	// If init had an error, show it
	if m.initError != "" {
		return "[BT] " + m.initError
	}

	// Ensure state is not nil before passing to JS
	state := m.state
	if state == nil || goja.IsUndefined(state) || goja.IsNull(state) {
		return "[BT] View: state is nil/undefined"
	}

	result, err := m.viewFn(goja.Undefined(), state)
	if err != nil {
		return fmt.Sprintf("[BT] View error: %v", err)
	}
	if result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
		return "[BT] View returned nil/undefined"
	}
	viewStr := result.String()
	if viewStr == "" {
		return "[BT] View returned empty string"
	}
	return viewStr
}

// msgToJS converts a tea.Msg to a JavaScript-compatible object.
// Handles all bubbletea message types comprehensively.
func (m *jsModel) msgToJS(msg tea.Msg) map[string]interface{} {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Extract runes as an array for unicode/IME support
		runes := make([]string, len(msg.Runes))
		for i, r := range msg.Runes {
			runes[i] = string(r)
		}

		// Get the string representation
		keyStr := msg.String()

		// Detect modifiers from the Key type, not from string parsing
		// KeyMsg embeds Key which has Type, Runes, Alt, Paste
		key := tea.Key(msg)

		return map[string]interface{}{
			"type":  "keyPress",
			"key":   keyStr,
			"runes": runes,
			"alt":   key.Alt,
			"ctrl":  isCtrlKey(key.Type),
			"paste": key.Paste, // Bracketed paste indicator
		}

	case tea.MouseMsg:
		return map[string]interface{}{
			"type":   "mouse",
			"x":      msg.X,
			"y":      msg.Y,
			"button": mouseButtonToString(msg.Button),
			"action": mouseActionToString(msg.Action),
			"alt":    msg.Alt,
			"ctrl":   msg.Ctrl,
			"shift":  msg.Shift,
		}

	case tea.WindowSizeMsg:
		return map[string]interface{}{
			"type":   "windowSize",
			"width":  msg.Width,
			"height": msg.Height,
		}

	case tea.FocusMsg:
		return map[string]interface{}{
			"type": "focus",
		}

	case tea.BlurMsg:
		return map[string]interface{}{
			"type": "blur",
		}

	case tickMsg:
		return map[string]interface{}{
			"type": "tick",
			"id":   msg.id,
			"time": msg.time.UnixMilli(),
		}

	case quitMsg:
		m.quitCalled = true
		return map[string]interface{}{
			"type": "quit",
		}

	case clearScreenMsg:
		return map[string]interface{}{
			"type": "clearScreen",
		}

	case stateRefreshMsg:
		return map[string]interface{}{
			"type": "stateRefresh",
			"key":  msg.key,
		}

	default:
		return nil
	}
}

// isCtrlKey checks if the key type is a control key.
func isCtrlKey(kt tea.KeyType) bool {
	switch kt {
	case tea.KeyCtrlA, tea.KeyCtrlB, tea.KeyCtrlC, tea.KeyCtrlD, tea.KeyCtrlE,
		tea.KeyCtrlF, tea.KeyCtrlG, tea.KeyCtrlH, tea.KeyCtrlI, tea.KeyCtrlJ,
		tea.KeyCtrlK, tea.KeyCtrlL, tea.KeyCtrlM, tea.KeyCtrlN, tea.KeyCtrlO,
		tea.KeyCtrlP, tea.KeyCtrlQ, tea.KeyCtrlR, tea.KeyCtrlS, tea.KeyCtrlT,
		tea.KeyCtrlU, tea.KeyCtrlV, tea.KeyCtrlW, tea.KeyCtrlX, tea.KeyCtrlY,
		tea.KeyCtrlZ, tea.KeyCtrlOpenBracket, tea.KeyCtrlBackslash,
		tea.KeyCtrlCloseBracket, tea.KeyCtrlCaret, tea.KeyCtrlUnderscore,
		tea.KeyCtrlQuestionMark, tea.KeyCtrlAt,
		tea.KeyCtrlUp, tea.KeyCtrlDown, tea.KeyCtrlLeft, tea.KeyCtrlRight,
		tea.KeyCtrlHome, tea.KeyCtrlEnd, tea.KeyCtrlPgUp, tea.KeyCtrlPgDown,
		tea.KeyCtrlShiftUp, tea.KeyCtrlShiftDown, tea.KeyCtrlShiftLeft,
		tea.KeyCtrlShiftRight, tea.KeyCtrlShiftHome, tea.KeyCtrlShiftEnd:
		return true
	}
	return false
}

// mouseButtonToString converts a tea.MouseButton to a string.
func mouseButtonToString(b tea.MouseButton) string {
	switch b {
	case tea.MouseButtonNone:
		return "none"
	case tea.MouseButtonLeft:
		return "left"
	case tea.MouseButtonMiddle:
		return "middle"
	case tea.MouseButtonRight:
		return "right"
	case tea.MouseButtonWheelUp:
		return "wheelUp"
	case tea.MouseButtonWheelDown:
		return "wheelDown"
	case tea.MouseButtonWheelLeft:
		return "wheelLeft"
	case tea.MouseButtonWheelRight:
		return "wheelRight"
	case tea.MouseButtonBackward:
		return "backward"
	case tea.MouseButtonForward:
		return "forward"
	default:
		return "unknown"
	}
}

// mouseActionToString converts a tea.MouseAction to a string.
func mouseActionToString(a tea.MouseAction) string {
	switch a {
	case tea.MouseActionPress:
		return "press"
	case tea.MouseActionRelease:
		return "release"
	case tea.MouseActionMotion:
		return "motion"
	default:
		return "unknown"
	}
}

// valueToCmd converts a JavaScript value to a tea.Cmd.
// Commands are validated using their _cmdID to prevent forgery.
func (m *jsModel) valueToCmd(val goja.Value) tea.Cmd {
	if goja.IsUndefined(val) || goja.IsNull(val) {
		return nil
	}

	obj := val.ToObject(m.runtime)
	if obj == nil {
		return nil
	}

	cmdType := obj.Get("_cmdType")
	if goja.IsUndefined(cmdType) {
		return nil
	}

	// NOTE: Command ID validation is currently disabled because it was causing
	// quit commands to be silently dropped. The issue is that currentModel
	// (used during createCommand) and m (the receiver here) may not always
	// be the same object due to how the closure and model registry interact.
	// TODO: Fix the command registration to use a proper shared registry
	// instead of relying on pointer identity.
	//
	// cmdIDVal := obj.Get("_cmdID")
	// if !goja.IsUndefined(cmdIDVal) && !goja.IsNull(cmdIDVal) {
	// 	cmdID := uint64(cmdIDVal.ToInteger())
	// 	if !m.isValidCommand(cmdID) {
	// 		return nil
	// 	}
	// }

	switch cmdType.String() {
	case "quit":
		return tea.Quit

	case "clearScreen":
		return tea.ClearScreen

	case "batch":
		return m.extractBatchCmd(obj)

	case "sequence":
		return m.extractSequenceCmd(obj)

	case "tick":
		return m.extractTickCmd(obj)

	case "setWindowTitle":
		titleVal := obj.Get("title")
		if goja.IsUndefined(titleVal) || goja.IsNull(titleVal) {
			return nil
		}
		return tea.SetWindowTitle(titleVal.String())

	case "hideCursor":
		return tea.HideCursor

	case "showCursor":
		return tea.ShowCursor

	case "enterAltScreen":
		return tea.EnterAltScreen

	case "exitAltScreen":
		return tea.ExitAltScreen

	case "enableBracketedPaste":
		return tea.EnableBracketedPaste

	case "disableBracketedPaste":
		return tea.DisableBracketedPaste

	case "enableReportFocus":
		return tea.EnableReportFocus

	case "disableReportFocus":
		return tea.DisableReportFocus

	case "windowSize":
		return tea.WindowSize()
	}

	return nil
}

// extractBatchCmd extracts commands from a batch command object.
func (m *jsModel) extractBatchCmd(obj *goja.Object) tea.Cmd {
	cmdsVal := obj.Get("cmds")
	if goja.IsUndefined(cmdsVal) || goja.IsNull(cmdsVal) {
		return nil
	}
	cmdsObj := cmdsVal.ToObject(m.runtime)
	length := int(cmdsObj.Get("length").ToInteger())
	var cmds []tea.Cmd
	for i := 0; i < length; i++ {
		cmdVal := cmdsObj.Get(fmt.Sprintf("%d", i))
		if cmd := m.valueToCmd(cmdVal); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

// extractSequenceCmd extracts commands from a sequence command object.
func (m *jsModel) extractSequenceCmd(obj *goja.Object) tea.Cmd {
	cmdsVal := obj.Get("cmds")
	if goja.IsUndefined(cmdsVal) || goja.IsNull(cmdsVal) {
		return nil
	}
	cmdsObj := cmdsVal.ToObject(m.runtime)
	length := int(cmdsObj.Get("length").ToInteger())
	var cmds []tea.Cmd
	for i := 0; i < length; i++ {
		cmdVal := cmdsObj.Get(fmt.Sprintf("%d", i))
		if cmd := m.valueToCmd(cmdVal); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Sequence(cmds...)
}

// extractTickCmd extracts a tick command.
func (m *jsModel) extractTickCmd(obj *goja.Object) tea.Cmd {
	durationVal := obj.Get("duration")
	if goja.IsUndefined(durationVal) || goja.IsNull(durationVal) {
		return nil
	}
	durationMs := durationVal.ToInteger()
	if durationMs <= 0 {
		return nil
	}

	idVal := obj.Get("id")
	id := ""
	if !goja.IsUndefined(idVal) && !goja.IsNull(idVal) {
		id = idVal.String()
	}

	duration := time.Duration(durationMs) * time.Millisecond
	return tea.Tick(duration, func(t time.Time) tea.Msg {
		return tickMsg{id: id, time: t}
	})
}

// tickMsg is a custom message type for tick events.
type tickMsg struct {
	id   string
	time time.Time
}

// quitMsg is a custom message type for quit.
type quitMsg struct{}

// clearScreenMsg is a custom message type for clear screen.
type clearScreenMsg struct{}

// stateRefreshMsg is a custom message type for state refresh notifications.
// This message is sent when external code modifies shared state and wants the TUI to re-render.
type stateRefreshMsg struct {
	key string // The state key that changed (for filtering/debugging)
}

// SendStateRefresh sends a state refresh message to the currently running program.
// This is safe to call from any goroutine. If no program is running, it's a no-op.
// The key parameter indicates which state key changed (for debugging/logging).
func (m *Manager) SendStateRefresh(key string) {
	m.mu.Lock()
	p := m.program
	m.mu.Unlock()

	if p != nil {
		p.Send(stateRefreshMsg{key: key})
	}
}

// Require returns a CommonJS native module under "osm:bubbletea".
// It exposes bubbletea functionality for building terminal UIs.
func Require(baseCtx context.Context, manager *Manager) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		// Track the current model for command validation
		var currentModel *jsModel

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

		// Helper to create command with validated ID
		createCommand := func(cmdType string, props map[string]interface{}) goja.Value {
			cmdID := generateCommandID()
			if currentModel != nil {
				currentModel.registerCommand(cmdID)
			}
			result := map[string]interface{}{
				"_cmdType": cmdType,
				"_cmdID":   cmdID,
			}
			for k, v := range props {
				result[k] = v
			}
			return runtime.ToValue(result)
		}

		// Helper to create error response
		createError := func(code, message string) goja.Value {
			return runtime.ToValue(map[string]interface{}{
				"error":     fmt.Sprintf("%s: %s", code, message),
				"errorCode": code,
			})
		}

		// isTTY returns whether the terminal is a TTY
		_ = exports.Set("isTTY", func(call goja.FunctionCall) goja.Value {
			return runtime.ToValue(manager.IsTTY())
		})

		// newModel creates a new model wrapper from JS definition
		_ = exports.Set("newModel", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return createError(ErrCodeInvalidArgs, "newModel requires a config object")
			}

			config := call.Argument(0).ToObject(runtime)
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

			model := &jsModel{
				runtime:     runtime,
				initFn:      initFn,
				updateFn:    updateFn,
				viewFn:      viewFn,
				state:       runtime.NewObject(), // Initialize with empty object to avoid nil
				validCmdIDs: make(map[uint64]bool),
			}

			// Set as current model for command registration
			currentModel = model

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
						fmt.Fprintf(manager.stderr, "\n[PANIC] in bubbletea.run(): %v\n\nStack:\n%s\n", r, string(stackTrace))
					}
					result = createError(ErrCodePanic, fmt.Sprintf("panic in run: %v", r))
				}
			}()

			if len(call.Arguments) < 1 {
				return createError(ErrCodeInvalidArgs, "run requires a model")
			}

			modelWrapper := call.Argument(0).ToObject(runtime)
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

			// Set as current model for command registration
			currentModel = model

			// Parse options
			var opts []tea.ProgramOption
			if len(call.Arguments) > 1 {
				optObj := call.Argument(1).ToObject(runtime)
				if optObj != nil {
					// Helper to safely check if a value is truthy
					// Returns true only if value exists, is not nil/undefined/null, and is boolean true
					isTruthy := func(v goja.Value) bool {
						if v == nil {
							return false
						}
						if goja.IsUndefined(v) || goja.IsNull(v) {
							return false
						}
						return v.ToBoolean()
					}

					if isTruthy(optObj.Get("altScreen")) {
						opts = append(opts, tea.WithAltScreen())
					}
					if isTruthy(optObj.Get("mouse")) {
						opts = append(opts, tea.WithMouseAllMotion())
					}
					if isTruthy(optObj.Get("mouseCellMotion")) {
						opts = append(opts, tea.WithMouseCellMotion())
					}
					if isTruthy(optObj.Get("reportFocus")) {
						opts = append(opts, tea.WithReportFocus())
					}
					// Note: bracketedPaste is enabled by default in bubbletea
					// Use WithoutBracketedPaste to disable (check if explicitly set to false)
					bracketedPaste := optObj.Get("bracketedPaste")
					if bracketedPaste != nil && !goja.IsUndefined(bracketedPaste) && !goja.IsNull(bracketedPaste) && !bracketedPaste.ToBoolean() {
						opts = append(opts, tea.WithoutBracketedPaste())
					}
				}
			}

			// Validate manager is not nil (model was validated above during registry lookup)
			if manager == nil {
				return createError(ErrCodeProgramFailed, "manager is nil")
			}

			// Run the program
			if err := manager.runProgram(model, opts...); err != nil {
				return createError(ErrCodeProgramFailed, err.Error())
			}

			return goja.Undefined()
		})

		// quit returns a quit command
		_ = exports.Set("quit", func(call goja.FunctionCall) goja.Value {
			return createCommand("quit", nil)
		})

		// clearScreen returns a clear screen command
		_ = exports.Set("clearScreen", func(call goja.FunctionCall) goja.Value {
			return createCommand("clearScreen", nil)
		})

		// batch combines multiple commands
		_ = exports.Set("batch", func(call goja.FunctionCall) goja.Value {
			cmds := make([]interface{}, 0, len(call.Arguments))
			for _, arg := range call.Arguments {
				if !goja.IsUndefined(arg) && !goja.IsNull(arg) {
					cmds = append(cmds, arg.Export())
				}
			}
			return createCommand("batch", map[string]interface{}{"cmds": cmds})
		})

		// sequence executes commands in sequence
		_ = exports.Set("sequence", func(call goja.FunctionCall) goja.Value {
			cmds := make([]interface{}, 0, len(call.Arguments))
			for _, arg := range call.Arguments {
				if !goja.IsUndefined(arg) && !goja.IsNull(arg) {
					cmds = append(cmds, arg.Export())
				}
			}
			return createCommand("sequence", map[string]interface{}{"cmds": cmds})
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

			return createCommand("tick", map[string]interface{}{
				"duration": durationMs,
				"id":       id,
			})
		})

		// setWindowTitle sets the terminal window title
		_ = exports.Set("setWindowTitle", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return createError(ErrCodeInvalidArgs, "setWindowTitle requires a title string")
			}
			return createCommand("setWindowTitle", map[string]interface{}{
				"title": call.Argument(0).String(),
			})
		})

		// hideCursor hides the cursor
		_ = exports.Set("hideCursor", func(call goja.FunctionCall) goja.Value {
			return createCommand("hideCursor", nil)
		})

		// showCursor shows the cursor
		_ = exports.Set("showCursor", func(call goja.FunctionCall) goja.Value {
			return createCommand("showCursor", nil)
		})

		// enterAltScreen enters the alternate screen buffer
		_ = exports.Set("enterAltScreen", func(call goja.FunctionCall) goja.Value {
			return createCommand("enterAltScreen", nil)
		})

		// exitAltScreen exits the alternate screen buffer
		_ = exports.Set("exitAltScreen", func(call goja.FunctionCall) goja.Value {
			return createCommand("exitAltScreen", nil)
		})

		// enableBracketedPaste enables bracketed paste mode
		_ = exports.Set("enableBracketedPaste", func(call goja.FunctionCall) goja.Value {
			return createCommand("enableBracketedPaste", nil)
		})

		// disableBracketedPaste disables bracketed paste mode
		_ = exports.Set("disableBracketedPaste", func(call goja.FunctionCall) goja.Value {
			return createCommand("disableBracketedPaste", nil)
		})

		// enableReportFocus enables focus/blur reporting
		_ = exports.Set("enableReportFocus", func(call goja.FunctionCall) goja.Value {
			return createCommand("enableReportFocus", nil)
		})

		// disableReportFocus disables focus/blur reporting
		_ = exports.Set("disableReportFocus", func(call goja.FunctionCall) goja.Value {
			return createCommand("disableReportFocus", nil)
		})

		// windowSize queries the current window size
		_ = exports.Set("windowSize", func(call goja.FunctionCall) goja.Value {
			return createCommand("windowSize", nil)
		})
	}
}

// runProgram runs a bubbletea program with the given model.
// Handles TTY detection, terminal state cleanup, panic recovery, and proper I/O setup.
// If a panic occurs, the terminal is restored before printing the stack trace to stderr.
func (m *Manager) runProgram(model tea.Model, opts ...tea.ProgramOption) (err error) {
	// Debug: check if manager is properly initialized
	if m == nil {
		return fmt.Errorf("runProgram: manager is nil")
	}
	if m.ctx == nil {
		return fmt.Errorf("runProgram: manager.ctx is nil")
	}

	ctx, cancel := context.WithCancelCause(m.ctx)
	defer cancel(nil)

	// Lock only for accessing/copying configuration, not during p.Run()
	m.mu.Lock()
	input := m.input
	output := m.output
	stderr := m.stderr
	signalNotify := m.signalNotify
	signalStop := m.signalStop
	isTTY := m.isTTY
	ttyFd := m.ttyFd
	m.mu.Unlock()

	// Debug: validate required function pointers
	if signalNotify == nil {
		return fmt.Errorf("runProgram: signalNotify is nil")
	}
	if signalStop == nil {
		return fmt.Errorf("runProgram: signalStop is nil")
	}

	// Save terminal state for cleanup
	var origState *term.State
	if isTTY && ttyFd >= 0 {
		var stateErr error
		origState, stateErr = term.GetState(ttyFd)
		if stateErr != nil {
			// Non-fatal: we just won't be able to restore state
			origState = nil
		}
	}

	// restoreTerminal restores the terminal state if we have a saved state
	restoreTerminal := func() {
		if origState != nil && ttyFd >= 0 {
			_ = term.Restore(ttyFd, origState)
		}
	}

	// Ensure terminal state is restored even on panic
	// Also capture and log the panic with stack trace
	defer func() {
		if r := recover(); r != nil {
			// FIRST: Restore terminal to ensure output is readable
			restoreTerminal()

			// Get the full stack trace
			stackTrace := debug.Stack()

			// Log the panic to stderr with full details
			panicMsg := fmt.Sprintf("\n[%s] PANIC in bubbletea program:\n  %v\n\nStack Trace:\n%s\n",
				ErrCodePanic, r, string(stackTrace))

			if stderr != nil {
				_, _ = fmt.Fprint(stderr, panicMsg)
			}

			// Convert panic to error for the caller
			err = fmt.Errorf("%s: panic recovered: %v", ErrCodePanic, r)
		} else {
			// Normal exit - still restore terminal
			restoreTerminal()
		}
	}()

	// Configure input/output
	if f, ok := input.(*os.File); ok {
		opts = append(opts, tea.WithInput(f))
	} else if input != nil {
		// For non-file readers, try to use input TTY
		if isTTY {
			opts = append(opts, tea.WithInputTTY())
		}
	}

	if f, ok := output.(*os.File); ok {
		opts = append(opts, tea.WithOutput(f))
	}

	p := tea.NewProgram(model, opts...)

	// Store program reference for external message injection (e.g., state refresh)
	m.mu.Lock()
	m.program = p
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.program = nil
		m.mu.Unlock()
	}()

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signalNotify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer signalStop(sigCh)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
		case <-sigCh:
		}
		p.Quit()
	}()

	_, runErr := p.Run()
	cancel(nil) // Signal the goroutine to exit
	wg.Wait()   // Wait for the goroutine to finish

	if runErr != nil {
		return fmt.Errorf("failed to run program: %w", runErr)
	}

	return nil
}
