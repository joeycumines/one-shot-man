//go:generate go run ../../../internal/cmd/generate-bubbletea-key-mapping

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
//		}
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
//	// msg.type === 'Key'
//	// msg.key - the key name ('q', 'enter', 'up', 'down', etc.)
//	// msg.runes - array of runes (for unicode/IME input)
//	// msg.alt - alt modifier
//	// msg.ctrl - ctrl modifier
//	// msg.paste - true if this is part of a bracketed paste
//
//	// Mouse events (when enabled)
//	// msg.type === 'Mouse'
//	// msg.x, msg.y - coordinates
//	// msg.button - button name
//	// msg.action - 'press', 'release', 'motion'
//	// msg.alt, msg.ctrl, msg.shift - modifiers
//
//	// Window size events
//	// msg.type === 'WindowSize'
//	// msg.width, msg.height - terminal dimensions
//
//	// Focus events (when reportFocus enabled)
//	// msg.type === 'Focus'  - terminal gained focus
//	// msg.type === 'Blur'   - terminal lost focus
//
//	// Tick events (from tick command)
//	// msg.type === 'Tick'
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
	"io"
	"log/slog"
	"maps"
	"os"
	"os/signal"
	"runtime/debug"
	"slices"
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

// JSRunner provides thread-safe JavaScript execution on the event loop.
// This interface abstracts the event loop synchronization mechanism,
// allowing BubbleTea's jsModel to safely call JavaScript functions from
// the BubbleTea goroutine without violating goja.Runtime's thread-safety
// requirements.
//
// CRITICAL: goja.Runtime is NOT thread-safe. All JS calls MUST go through
// RunJSSync when called from goroutines other than the event loop goroutine.
// This is especially important for BubbleTea's Init/Update/View methods
// which run on the BubbleTea goroutine, not the event loop goroutine.
type JSRunner interface {
	// RunJSSync schedules a function on the event loop and waits for completion.
	// The provided goja.Runtime is the event loop's runtime instance.
	// Returns an error if the event loop is not running or stops while waiting.
	// This method blocks until the callback completes.
	RunJSSync(fn func(*goja.Runtime) error) error
}

// AsyncJSRunner extends JSRunner with non-blocking async execution.
// This is required for tea.run() to return a Promise without blocking
// the event loop, allowing the Promise to be resolved/rejected when
// the BubbleTea program finishes.
type AsyncJSRunner interface {
	JSRunner
	// RunOnLoop schedules a function on the event loop WITHOUT blocking.
	// Returns true if the function was successfully scheduled.
	// Returns false if the event loop is not running.
	RunOnLoop(fn func(*goja.Runtime)) bool
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
	jsRunner     JSRunner     // REQUIRED: thread-safe JS execution via event loop
}

// NewManager creates a new bubbletea manager for an engine instance.
// Input and output can be nil to use os.Stdin and os.Stdout.
// Automatically detects TTY and sets up proper terminal handling.
//
// CRITICAL: jsRunner is REQUIRED. It provides thread-safe JS execution from
// BubbleTea's goroutine. Without it, JS calls would cause data races.
// Use *bt.Bridge which implements JSRunner directly.
// Panics if jsRunner is nil.
func NewManager(
	ctx context.Context,
	input io.Reader,
	output io.Writer,
	jsRunner JSRunner,
	signalNotify func(c chan<- os.Signal, sig ...os.Signal),
	signalStop func(c chan<- os.Signal),
) *Manager {
	if jsRunner == nil {
		panic("bubbletea.NewManager: jsRunner is REQUIRED - cannot be nil; provide a JSRunner implementation (e.g., *bt.Bridge)")
	}
	return NewManagerWithStderr(ctx, input, output, nil, jsRunner, signalNotify, signalStop)
}

// NewManagerWithStderr creates a new bubbletea manager with explicit stderr for error logging.
// Input, output, and stderr can be nil to use os.Stdin, os.Stdout, and os.Stderr.
// Automatically detects TTY and sets up proper terminal handling.
//
// CRITICAL: jsRunner is REQUIRED. It provides thread-safe JS execution from
// BubbleTea's goroutine. Without it, JS calls would cause data races.
// Use *bt.Bridge which implements JSRunner directly.
// Panics if jsRunner is nil.
func NewManagerWithStderr(
	ctx context.Context,
	input io.Reader,
	output io.Writer,
	stderr io.Writer,
	jsRunner JSRunner,
	signalNotify func(c chan<- os.Signal, sig ...os.Signal),
	signalStop func(c chan<- os.Signal),
) *Manager {
	if jsRunner == nil {
		panic("bubbletea.NewManagerWithStderr: jsRunner is REQUIRED - cannot be nil; provide a JSRunner implementation (e.g., *bt.Bridge)")
	}
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
		jsRunner:     jsRunner, // REQUIRED: set at construction time
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

// SetJSRunner configures the Manager to use thread-safe JS execution.
// This MUST be called before any BubbleTea programs are run.
// The JSRunner ensures all JS calls from BubbleTea's goroutine are safely
// routed through the event loop.
//
// CRITICAL: JSRunner is MANDATORY. Without it, JS calls from BubbleTea's
// Init/Update/View methods would execute directly on the BubbleTea goroutine,
// causing data races with the event loop. This function panics if runner is nil.
func (m *Manager) SetJSRunner(runner JSRunner) {
	if runner == nil {
		panic("bubbletea: SetJSRunner called with nil runner - JSRunner is mandatory for thread-safe operation")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jsRunner = runner
}

// GetJSRunner returns the configured JSRunner, or nil if not set.
func (m *Manager) GetJSRunner() JSRunner {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.jsRunner
}

// IsTTY returns whether the manager has access to a TTY.
func (m *Manager) IsTTY() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isTTY
}

// JsToTeaMsg converts a JavaScript message object to a tea.Msg.
// This provides a standard way to decode events sent from JS components to Go models.
// It handles standard "Key", "Mouse", and "WindowSize" event types.
// Returns nil if the message cannot be converted (invalid object, unknown type, etc.)
func JsToTeaMsg(runtime *goja.Runtime, obj *goja.Object) tea.Msg {
	if obj == nil || runtime == nil {
		return nil
	}
	typeVal := obj.Get("type")
	if typeVal == nil || goja.IsUndefined(typeVal) || goja.IsNull(typeVal) {
		return nil
	}

	msgType := typeVal.String()

	switch msgType {
	case "Key":
		keyVal := obj.Get("key")
		if keyVal == nil || goja.IsUndefined(keyVal) || goja.IsNull(keyVal) {
			return nil
		}
		key, _ := ParseKey(keyVal.String())
		return tea.KeyMsg(key)

	case "Mouse":
		x := int(obj.Get("x").ToInteger())
		y := int(obj.Get("y").ToInteger())
		buttonStr := obj.Get("button").String()
		actionStr := obj.Get("action").String()

		// Modifiers
		alt := false
		ctrl := false
		shift := false

		if v := obj.Get("alt"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			alt = v.ToBoolean()
		}
		if v := obj.Get("ctrl"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			ctrl = v.ToBoolean()
		}
		if v := obj.Get("shift"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			shift = v.ToBoolean()
		}

		// Use JSToMouseEvent which uses the generated MouseButtonDefs/MouseActionDefs
		return JSToMouseEvent(buttonStr, actionStr, x, y, alt, ctrl, shift)

	case "WindowSize":
		w := int(obj.Get("width").ToInteger())
		h := int(obj.Get("height").ToInteger())
		return tea.WindowSizeMsg{
			Width:  w,
			Height: h,
		}
	}

	return nil
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
	jsRunner    JSRunner // Optional: thread-safe JS execution via event loop

	// Render throttling state (optional, opt-in feature)
	throttleEnabled    bool            // Whether throttling is enabled (default: false)
	throttleIntervalMs int64           // Minimum ms between renders (default: 16)
	alwaysRenderTypes  map[string]bool // Message types that force immediate render
	cachedView         string          // Cached view output
	lastRenderTime     time.Time       // Time of last actual render
	forceNextRender    bool            // Force next render (e.g., for Tick messages)
	throttleMu         sync.Mutex      // Protects throttle timer state
	throttleTimerSet   bool            // True if a delayed render is already scheduled
	program            *tea.Program    // Reference for sending delayed render messages

	// Throttle timer cancellation - prevents goroutine leak on program exit
	// Set when program starts, cancelled when program exits
	throttleCtx    context.Context
	throttleCancel context.CancelFunc
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
// CRITICAL: This is called from BubbleTea's goroutine, NOT the event loop goroutine.
// JSRunner MUST be set to safely marshal JS execution to the event loop.
func (m *jsModel) Init() tea.Cmd {
	if m == nil || m.initFn == nil || m.runtime == nil {
		return nil
	}

	// JSRunner is MANDATORY - panic if not set.
	// This ensures thread-safe JS execution from BubbleTea's goroutine.
	if m.jsRunner == nil {
		panic("bubbletea: jsModel.Init called without JSRunner - this is a programming error; SetJSRunner must be called before running any BubbleTea program")
	}

	var cmd tea.Cmd
	err := m.jsRunner.RunJSSync(func(vm *goja.Runtime) error {
		cmd = m.initDirect()
		return nil
	})
	if err != nil {
		m.initError = fmt.Sprintf("Init error (event loop): %v", err)
		return nil
	}
	return cmd
}

// initDirect performs the actual init call. MUST be called from event loop goroutine.
func (m *jsModel) initDirect() tea.Cmd {
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
		return nil
	}

	// Check if result is an array [state, cmd] (like update returns)
	resultObj := result.ToObject(m.runtime)
	if resultObj != nil && resultObj.ClassName() == "Array" {
		// Extract state from index 0
		if newState := resultObj.Get("0"); !goja.IsUndefined(newState) && !goja.IsNull(newState) {
			m.state = newState
		} else {
			m.state = m.runtime.NewObject()
		}
		// Extract command from index 1
		cmdVal := resultObj.Get("1")
		return m.valueToCmd(cmdVal)
	}

	// Otherwise, result is just the state object (no initial command)
	m.state = result
	return nil
}

// Update implements tea.Model.
// CRITICAL: This is called from BubbleTea's goroutine, NOT the event loop goroutine.
// JSRunner MUST be set to safely marshal JS execution to the event loop.
func (m *jsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m == nil || m.updateFn == nil || m.runtime == nil {
		return m, nil
	}

	// Handle renderRefreshMsg specially - it just forces the next view to render
	if _, ok := msg.(renderRefreshMsg); ok {
		m.throttleMu.Lock()
		m.throttleTimerSet = false // Timer has fired
		m.forceNextRender = true   // Force the next View() to actually render
		m.throttleMu.Unlock()
		return m, nil
	}

	jsMsg := m.msgToJS(msg)
	if jsMsg == nil {
		return m, nil
	}

	// Log Update calls for debugging tick loop issues
	if msgType, ok := jsMsg["type"].(string); ok && msgType == "Tick" {
		slog.Debug("bubbletea: Update called with Tick message", "id", jsMsg["id"])
	}

	// Check if this message type should force an immediate render
	if m.throttleEnabled && m.alwaysRenderTypes != nil {
		if msgType, ok := jsMsg["type"].(string); ok {
			if m.alwaysRenderTypes[msgType] {
				m.throttleMu.Lock()
				m.forceNextRender = true
				m.throttleMu.Unlock()
			}
		}
	}

	// JSRunner is MANDATORY - panic if not set.
	// This ensures thread-safe JS execution from BubbleTea's goroutine.
	if m.jsRunner == nil {
		panic("bubbletea: jsModel.Update called without JSRunner - this is a programming error; SetJSRunner must be called before running any BubbleTea program")
	}

	var cmd tea.Cmd
	err := m.jsRunner.RunJSSync(func(vm *goja.Runtime) error {
		cmd = m.updateDirect(jsMsg)
		return nil
	})
	if err != nil {
		// Event loop error - return current state unchanged
		// NOTE: Using fmt.Fprintf to stderr so it's VISIBLE in test output (slog goes to file)
		fmt.Fprintf(os.Stderr, "\n!!! CRITICAL: bubbletea Update RunJSSync error: %v (msgType=%v) - TICK LOOP BROKEN !!!\n", err, jsMsg["type"])
		slog.Error("bubbletea: Update: RunJSSync error, returning nil cmd (breaks tick loop!)", "error", err, "msgType", jsMsg["type"])
		return m, nil
	}
	return m, cmd
}

// updateDirect performs the actual update call. MUST be called from event loop goroutine.
func (m *jsModel) updateDirect(jsMsg map[string]interface{}) tea.Cmd {
	// Ensure state is not nil before passing to JS
	state := m.state
	if state == nil || goja.IsUndefined(state) || goja.IsNull(state) {
		state = m.runtime.NewObject()
	}

	result, err := m.updateFn(goja.Undefined(), m.runtime.ToValue(jsMsg), state)
	if err != nil {
		slog.Error("bubbletea: updateDirect: JS update function error", "error", err, "msgType", jsMsg["type"])
		fmt.Fprintf(os.Stderr, "\n!!! JS UPDATE ERROR: %v (msgType=%v) !!!\n", err, jsMsg["type"])
		return nil
	}
	slog.Debug("bubbletea: updateDirect: JS update returned successfully", "msgType", jsMsg["type"])

	// Result should be [newState, cmd] array
	resultObj := result.ToObject(m.runtime)
	if resultObj == nil || resultObj.ClassName() != "Array" {
		slog.Error("bubbletea: updateDirect: update did not return [state, cmd] array", "resultObj", resultObj, "className", func() string {
			if resultObj == nil {
				return "nil"
			} else {
				return resultObj.ClassName()
			}
		}())
		return nil
	}

	// Extract new state
	if newState := resultObj.Get("0"); !goja.IsUndefined(newState) && !goja.IsNull(newState) {
		m.state = newState
	}

	// Extract command
	cmdVal := resultObj.Get("1")
	return m.valueToCmd(cmdVal)
}

// View implements tea.Model.
// CRITICAL: This is called from BubbleTea's goroutine, NOT the event loop goroutine.
// JSRunner MUST be set to safely marshal JS execution to the event loop.
//
// Render Throttling: When throttleEnabled is true, this method may return a cached
// view if the minimum interval has not elapsed. A delayed renderRefreshMsg is scheduled
// to ensure the view is eventually re-rendered.
func (m *jsModel) View() string {
	if m == nil || m.viewFn == nil || m.runtime == nil {
		return "[BT] View: nil model/viewFn/runtime"
	}

	// If init had an error, show it
	if m.initError != "" {
		return "[BT] " + m.initError
	}

	// Render throttling logic
	if m.throttleEnabled {
		m.throttleMu.Lock()
		now := time.Now()
		elapsed := now.Sub(m.lastRenderTime)
		intervalDur := time.Duration(m.throttleIntervalMs) * time.Millisecond

		// Check if we should throttle this render
		shouldThrottle := !m.forceNextRender && elapsed < intervalDur && m.cachedView != ""

		if shouldThrottle {
			// Schedule a delayed render if not already scheduled
			if !m.throttleTimerSet && m.program != nil && m.throttleCtx != nil {
				m.throttleTimerSet = true
				delay := intervalDur - elapsed
				prog := m.program
				throttleCtx := m.throttleCtx
				go func() {
					timer := time.NewTimer(delay)
					defer timer.Stop()
					select {
					case <-timer.C:
						// Timer fired - send render refresh message
						// prog.Send is documented as safe to call even after program
						// exits (it becomes a no-op), but we check context to avoid
						// unnecessary work
						prog.Send(renderRefreshMsg{})
					case <-throttleCtx.Done():
						// Program is exiting - don't send, just return
						return
					}
				}()
			}
			cached := m.cachedView
			m.throttleMu.Unlock()
			return cached
		}

		// Will do an actual render - update state
		m.forceNextRender = false
		m.lastRenderTime = now
		m.throttleMu.Unlock()
	}

	// JSRunner is MANDATORY - panic if not set.
	// This ensures thread-safe JS execution from BubbleTea's goroutine.
	if m.jsRunner == nil {
		panic("bubbletea: jsModel.View called without JSRunner - this is a programming error; SetJSRunner must be called before running any BubbleTea program")
	}

	var viewStr string
	err := m.jsRunner.RunJSSync(func(vm *goja.Runtime) error {
		viewStr = m.viewDirect()
		return nil
	})
	if err != nil {
		return fmt.Sprintf("[BT] View error (event loop): %v", err)
	}

	// Cache the view if throttling is enabled
	if m.throttleEnabled {
		m.throttleMu.Lock()
		m.cachedView = viewStr
		m.throttleMu.Unlock()
	}

	return viewStr
}

// viewDirect performs the actual view call. MUST be called from event loop goroutine.
func (m *jsModel) viewDirect() string {
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
			"type":  "Key",
			"key":   keyStr,
			"runes": runes,
			"alt":   key.Alt,
			"ctrl":  isCtrlKey(key.Type),
			"paste": key.Paste, // Bracketed paste indicator
		}

	case tea.MouseMsg:
		// Use the generated MouseEventToJS which ensures consistency with tea.MouseEvent.String()
		return MouseEventToJS(msg)

	case tea.WindowSizeMsg:
		return map[string]interface{}{
			"type":   "WindowSize",
			"width":  msg.Width,
			"height": msg.Height,
		}

	case tea.FocusMsg:
		return map[string]interface{}{
			"type": "Focus",
		}

	case tea.BlurMsg:
		return map[string]interface{}{
			"type": "Blur",
		}

	case tickMsg:
		return map[string]interface{}{
			"type": "Tick",
			"id":   msg.id,
			"time": msg.time.UnixMilli(),
		}

	case quitMsg:
		m.quitCalled = true
		return map[string]interface{}{
			"type": "Quit",
		}

	case clearScreenMsg:
		return map[string]interface{}{
			"type": "ClearScreen",
		}

	case stateRefreshMsg:
		return map[string]interface{}{
			"type": "StateRefresh",
			"key":  msg.key,
		}

	case renderRefreshMsg:
		// Internal message for render throttle - returns nil to skip JS processing
		// The Update method handles this specially to force a render
		return nil

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

// valueToCmd converts a JavaScript value to a tea.Cmd.
// Handles two types of command values:
// 1. Directly wrapped Go tea.Cmd functions (from bubbles components via WrapCmd)
// 2. Command descriptor objects (e.g., {_cmdType: "quit"} from JS tea.quit())
func (m *jsModel) valueToCmd(val goja.Value) (ret tea.Cmd) {
	if m == nil || m.runtime == nil {
		slog.Warn("bubbletea: valueToCmd: nil model or runtime")
		return nil
	}
	// Returning null or undefined for the command slot is valid and expected
	// (e.g., [model, null] from JavaScript). No warning needed.
	if goja.IsUndefined(val) || goja.IsNull(val) {
		return nil
	}

	// First, try to extract a directly wrapped tea.Cmd (from bubbles components)
	// This uses goja's native Export() which returns the original Go value
	// if it was wrapped via runtime.ToValue()
	if exported := val.Export(); exported != nil {
		if cmd, ok := exported.(tea.Cmd); ok {
			return cmd
		}
	}

	// Not a wrapped Go function - try command descriptor object
	obj := val.ToObject(m.runtime)
	if obj == nil {
		slog.Warn("bubbletea: valueToCmd: cmd is not an object")
		return nil
	}

	var cmdType goja.Value
	if m.runtime.Try(func() {
		cmdType = obj.Get("_cmdType")
	}) != nil || cmdType == nil || !cmdType.ToBoolean() {
		slog.Warn("bubbletea: valueToCmd: cmd object has no _cmdType (may be a foreign Go func)")
		return nil
	}

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
	if cmdsVal == nil || goja.IsUndefined(cmdsVal) || goja.IsNull(cmdsVal) {
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
	if cmdsVal == nil || goja.IsUndefined(cmdsVal) || goja.IsNull(cmdsVal) {
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
	// Use bubbletea.Sequence to preserve the intended semantics: commands
	// should be executed one-at-a-time by the bubbletea runtime. This allows
	// nested sequence/batch commands to be handled correctly by the runtime
	// machinery rather than forcing immediate execution here.
	return tea.Sequence(cmds...)
}

// extractTickCmd extracts a tick command.
func (m *jsModel) extractTickCmd(obj *goja.Object) tea.Cmd {
	durationVal := obj.Get("duration")
	if durationVal == nil || goja.IsUndefined(durationVal) || goja.IsNull(durationVal) {
		slog.Warn("bubbletea: extractTickCmd: duration is nil/undefined")
		return nil
	}
	durationMs := durationVal.ToInteger()
	if durationMs <= 0 {
		slog.Warn("bubbletea: extractTickCmd: duration <= 0", "durationMs", durationMs)
		return nil
	}

	idVal := obj.Get("id")
	id := ""
	if idVal != nil && !goja.IsUndefined(idVal) && !goja.IsNull(idVal) {
		id = idVal.String()
	}

	slog.Debug("bubbletea: extractTickCmd: scheduling tick", "id", id, "durationMs", durationMs)
	duration := time.Duration(durationMs) * time.Millisecond
	return tea.Tick(duration, func(t time.Time) tea.Msg {
		slog.Debug("bubbletea: tick callback fired", "id", id, "time", t)
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

// renderRefreshMsg is used internally by the render throttle mechanism.
// When a render is throttled, a delayed renderRefreshMsg is scheduled to
// ensure the view is eventually re-rendered with the latest state.
type renderRefreshMsg struct{}

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

		// mouseActions exposes mouse action definitions for JS to access action metadata
		// The keys are the string representations (e.g., "press", "release", "motion")
		mouseActionsObj := runtime.NewObject()
		for _, stringVal := range slices.Sorted(maps.Keys(MouseActionDefs)) {
			actionDef := MouseActionDefs[stringVal]
			actionDefJS := runtime.NewObject()
			_ = actionDefJS.Set("name", actionDef.Name)
			_ = actionDefJS.Set("string", actionDef.String)
			_ = mouseActionsObj.Set(stringVal, actionDefJS)
		}
		_ = exports.Set("mouseActions", mouseActionsObj)

		// mouseActionsByName exposes mouse action definitions by their Go constant name
		// (e.g., "MouseActionPress", "MouseActionRelease", "MouseActionMotion")
		mouseActionsByNameObj := runtime.NewObject()
		for _, name := range slices.Sorted(maps.Keys(MouseActionDefsByName)) {
			actionDef := MouseActionDefsByName[name]
			actionDefJS := runtime.NewObject()
			_ = actionDefJS.Set("name", actionDef.Name)
			_ = actionDefJS.Set("string", actionDef.String)
			_ = mouseActionsByNameObj.Set(name, actionDefJS)
		}
		_ = exports.Set("mouseActionsByName", mouseActionsByNameObj)

		// isValidTextareaInput validates if a key event should be forwarded to a textarea.
		// Uses WHITELIST approach: only explicitly allowed inputs pass through.
		// This prevents garbage (fragmented escape sequences) from corrupting content.
		// Parameters: keyStr (string), isPaste (boolean)
		// Returns: { valid: boolean, reason: string }
		_ = exports.Set("isValidTextareaInput", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return runtime.ToValue(map[string]interface{}{
					"valid":  false,
					"reason": "missing keyStr argument",
				})
			}
			keyStr := call.Argument(0).String()
			isPaste := false
			if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
				isPaste = call.Argument(1).ToBoolean()
			}
			result := ValidateTextareaInput(keyStr, isPaste)
			return runtime.ToValue(map[string]interface{}{
				"valid":  result.Valid,
				"reason": result.Reason,
			})
		})

		// isValidLabelInput validates if a key event should be accepted for a label field.
		// More restrictive: only single printable characters and backspace.
		// Parameters: keyStr (string), isPaste (boolean)
		// Returns: { valid: boolean, reason: string }
		_ = exports.Set("isValidLabelInput", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return runtime.ToValue(map[string]interface{}{
					"valid":  false,
					"reason": "missing keyStr argument",
				})
			}
			keyStr := call.Argument(0).String()
			isPaste := false
			if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
				isPaste = call.Argument(1).ToBoolean()
			}
			result := ValidateLabelInput(keyStr, isPaste)
			return runtime.ToValue(map[string]interface{}{
				"valid":  result.Valid,
				"reason": result.Reason,
			})
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

			// Get JSRunner from manager if available (for thread-safe JS execution)
			var jsRunner JSRunner
			if manager != nil {
				jsRunner = manager.GetJSRunner()
			}

			model := &jsModel{
				runtime:     runtime,
				initFn:      initFn,
				updateFn:    updateFn,
				viewFn:      viewFn,
				state:       runtime.NewObject(), // Initialize with empty object to avoid nil
				validCmdIDs: make(map[uint64]bool),
				jsRunner:    jsRunner,
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
						model.throttleIntervalMs = intervalVal.ToInteger()
						if model.throttleIntervalMs < 1 {
							model.throttleIntervalMs = 1
						}
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

			// Run the program synchronously. This blocks until the BubbleTea
			// program exits (user quits, error, etc.).
			//
			// Threading model:
			// - This function is called from the goroutine running ExecuteScript()
			// - BubbleTea runs its event loop on its own goroutine
			// - BubbleTea calls Init/Update/View via JSRunner.RunJSSync()
			// - RunJSSync schedules on the event loop goroutine (separate from here)
			// - The event loop is free to process requests while we block here
			//
			// This is NOT a deadlock because:
			// 1. We're blocking the ExecuteScript goroutine, NOT the event loop
			// 2. The event loop goroutine is free to process RunJSSync callbacks
			// 3. BubbleTea's goroutine can call RunJSSync and get responses
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
	if m.program != nil {
		m.mu.Unlock()
		return fmt.Errorf("runProgram: program is already running")
	}
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

	// Also store program reference in jsModel for render throttling
	if jm, ok := model.(*jsModel); ok {
		// Set up throttle cancellation context to prevent goroutine leak
		// when program exits while a throttle timer is sleeping
		throttleCtx, throttleCancel := context.WithCancel(ctx)
		jm.throttleCtx = throttleCtx
		jm.throttleCancel = throttleCancel
		jm.program = p
		defer func() {
			// Cancel any pending throttle timers FIRST, then nil program
			throttleCancel()
			jm.throttleCtx = nil
			jm.throttleCancel = nil
			jm.program = nil
		}()
	}

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signalNotify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer signalStop(sigCh)

	// Channel to signal that Run() has finished
	programFinished := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-programFinished:
			// Program finished naturally, no need to call Quit
			return
		case <-ctx.Done():
			// Context cancelled externally
		case <-sigCh:
			// OS Signal received
		}
		p.Quit()
	}()

	_, runErr := p.Run()
	close(programFinished) // Signal that Run() returned
	cancel(nil)            // Signal the goroutine to exit (via ctx.Done path if race, but programFinished priority)
	wg.Wait()              // Wait for the goroutine to finish

	if runErr != nil {
		return fmt.Errorf("failed to run program: %w", runErr)
	}

	return nil
}
