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
//	// Commands - all return opaque command objects
//	tea.quit();                    // Quit the program
//	tea.clearScreen();             // Clear the screen
//	tea.batch(...cmds);            // Batch multiple commands
//	tea.sequence(...cmds);         // Execute commands in sequence
//	tea.tick(durationMs, id);      // Timer command (returns tickMsg with id)
//	tea.requestWindowSize();       // Query current window size
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
	"errors"
	"fmt"
	"image/color"
	"io"
	"log/slog"
	"maps"
	"os"
	"os/signal"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
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

// TrySyncJSRunner extends JSRunner with deadlock-safe synchronous execution.
// TryRunOnLoopSync executes the callback directly when already on the event
// loop goroutine, and otherwise schedules-and-waits on the loop.
type TrySyncJSRunner interface {
	JSRunner
	TryRunOnLoopSync(currentVM *goja.Runtime, fn func(*goja.Runtime) error) error
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
	programDone  chan error   // Signals program exit; used by WaitForProgram()
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

// WaitForProgram blocks until the currently running BubbleTea program exits.
// Returns nil if no program is running. This is used by callers that need to
// block after starting a non-blocking tea.run() — for example, ExecuteScript
// routes the wizard launch through the event loop (so the Goja VM is only
// accessed from a single goroutine), and then WaitForProgram keeps the
// command goroutine alive until BubbleTea exits.
func (m *Manager) WaitForProgram() error {
	m.mu.Lock()
	done := m.programDone
	m.mu.Unlock()

	if done == nil {
		return nil
	}

	err := <-done

	m.mu.Lock()
	m.programDone = nil
	m.mu.Unlock()

	return err
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
		return key

	case "MouseClick":
		return jsToTeaMouseClick(obj)

	case "MouseRelease":
		return jsToTeaMouseRelease(obj)

	case "MouseMotion":
		return jsToTeaMouseMotion(obj)

	case "MouseWheel":
		return jsToTeaMouseWheel(obj)

	case "WindowSize":
		w := int(obj.Get("width").ToInteger())
		h := int(obj.Get("height").ToInteger())
		return tea.WindowSizeMsg{
			Width:  w,
			Height: h,
		}

	case "PasteStart":
		return tea.PasteStartMsg{}

	case "PasteEnd":
		return tea.PasteEndMsg{}

	case "Paste":
		content := ""
		if v := obj.Get("content"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			content = v.String()
		}
		return tea.PasteMsg{Content: content}
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

	// Cached declarative view fields for throttled renders.
	// When throttling, we must preserve the parsed tea.View fields
	// so that cursedRenderer sees consistent mode fields on each render.
	cachedViewAltScreen             bool
	cachedViewMouseMode             tea.MouseMode
	cachedViewReportFocus           bool
	cachedViewWindowTitle           string
	cachedViewCursor                *tea.Cursor
	cachedViewForegroundColor       string
	cachedViewBackgroundColor       string
	cachedViewKeyboardEnhancements  tea.KeyboardEnhancements
	cachedViewDisableBracketedPaste bool
	cachedViewProgressBar           *tea.ProgressBar
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

// runJSSync executes fn on the JS event loop and waits for completion.
// If the runner supports TryRunOnLoopSync, recursion on the event-loop
// goroutine is executed directly to avoid self-deadlock.
func (m *jsModel) runJSSync(fn func(*goja.Runtime) error) error {
	if m.jsRunner == nil {
		return fmt.Errorf("bubbletea: js runner is nil")
	}
	if tr, ok := m.jsRunner.(TrySyncJSRunner); ok {
		return tr.TryRunOnLoopSync(m.runtime, fn)
	}
	return m.jsRunner.RunJSSync(fn)
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
	err := m.runJSSync(func(vm *goja.Runtime) error {
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

	// Check if this message type should force an immediate render.
	// Paste events (PasteStart, Paste, PasteEnd) should always force render
	// since the textarea needs to update immediately with pasted content.
	if m.throttleEnabled {
		forceRender := false
		if m.alwaysRenderTypes != nil {
			if msgType, ok := jsMsg["type"].(string); ok && m.alwaysRenderTypes[msgType] {
				forceRender = true
			}
		}
		if !forceRender {
			// Always render for paste events
			if msgType, ok := jsMsg["type"].(string); ok {
				switch msgType {
				case "Paste", "PasteStart", "PasteEnd":
					forceRender = true
				}
			}
		}
		if forceRender {
			m.throttleMu.Lock()
			m.forceNextRender = true
			m.throttleMu.Unlock()
		}
	}

	// JSRunner is MANDATORY - panic if not set.
	// This ensures thread-safe JS execution from BubbleTea's goroutine.
	if m.jsRunner == nil {
		panic("bubbletea: jsModel.Update called without JSRunner - this is a programming error; SetJSRunner must be called before running any BubbleTea program")
	}

	var cmd tea.Cmd
	err := m.runJSSync(func(vm *goja.Runtime) error {
		cmd = m.updateDirect(jsMsg)
		return nil
	})
	if err != nil {
		// Event loop error - return current state unchanged
		slog.Error("bubbletea: Update: RunJSSync error, returning nil cmd (breaks tick loop!)", "error", err, "msgType", jsMsg["type"])
		return m, nil
	}
	return m, cmd
}

// updateDirect performs the actual update call. MUST be called from event loop goroutine.
func (m *jsModel) updateDirect(jsMsg map[string]any) tea.Cmd {
	// Ensure state is not nil before passing to JS
	state := m.state
	if state == nil || goja.IsUndefined(state) || goja.IsNull(state) {
		state = m.runtime.NewObject()
	}

	result, err := m.updateFn(goja.Undefined(), m.runtime.ToValue(jsMsg), state)
	if err != nil {
		slog.Error("bubbletea: updateDirect: JS update function error", "error", err, "msgType", jsMsg["type"])
		return nil
	}
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
func (m *jsModel) View() tea.View {
	if m == nil || m.viewFn == nil || m.runtime == nil {
		return tea.NewView("[BT] View: nil model/viewFn/runtime")
	}

	// If init had an error, show it
	if m.initError != "" {
		return tea.NewView("[BT] " + m.initError)
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
			slog.Debug("bubbletea: View throttling", "elapsed", elapsed, "interval", intervalDur, "cachedLen", len(m.cachedView))
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
			altScreen := m.cachedViewAltScreen
			mouseMode := m.cachedViewMouseMode
			reportFocus := m.cachedViewReportFocus
			windowTitle := m.cachedViewWindowTitle
			cursor := m.cachedViewCursor
			fgColor := m.cachedViewForegroundColor
			bgColor := m.cachedViewBackgroundColor
			kbEnhance := m.cachedViewKeyboardEnhancements
			disablePaste := m.cachedViewDisableBracketedPaste
			progressBar := m.cachedViewProgressBar
			m.throttleMu.Unlock()
			// Wire mode fields into the cached view so cursedRenderer.viewEquals
			// sees a change from the previous view and flushes output.
			// Without this, throttled renders return zero-valued mode fields,
			// so viewEquals compares AltScreen=false vs AltScreen=false (no change)
			// and flush() sends nothing to the PTY.
			v := tea.NewView(cached)
			v.AltScreen = altScreen
			v.MouseMode = mouseMode
			v.ReportFocus = reportFocus
			v.WindowTitle = windowTitle
			v.Cursor = cursor
			v.KeyboardEnhancements = kbEnhance
			v.DisableBracketedPasteMode = disablePaste
			v.ProgressBar = progressBar
			if fgColor != "" {
				v.ForegroundColor = parseColorValue(fgColor)
			}
			if bgColor != "" {
				v.BackgroundColor = parseColorValue(bgColor)
			}
			return v
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
	var viewAltScreen bool
	var viewMouseMode tea.MouseMode
	var viewReportFocus bool
	var viewWindowTitle string
	var viewCursor *tea.Cursor
	var viewForegroundColor string
	var viewBackgroundColor string
	var viewKeyboardEnhancements tea.KeyboardEnhancements
	var viewDisableBracketedPaste bool
	var viewProgressBar *tea.ProgressBar
	var hasViewFields bool // true if JS returned a declarative view object

	err := m.runJSSync(func(vm *goja.Runtime) error {
		result, err := m.viewDirectResult()
		if err != nil {
			viewStr = fmt.Sprintf("[BT] View error: %v", err)
			return nil
		}

		// If result is an object (not a primitive string), parse declarative fields.
		// This is the primary v2 mechanism for controlling terminal features.
		if result != nil && !goja.IsUndefined(result) && !goja.IsNull(result) {
			if obj := result.ToObject(vm); obj != nil && obj.ClassName() == "Object" {
				hasViewFields = true
				viewStr = getJSStringProp(obj, "content")
				if viewStr == "" {
					viewStr = result.String() // fallback
				}
				viewAltScreen = getJSBoolProp(obj, "altScreen")
				viewMouseMode = parseMouseModeProp(obj, "mouseMode")
				viewReportFocus = getJSBoolProp(obj, "reportFocus")
				viewWindowTitle = getJSStringProp(obj, "windowTitle")
				viewCursor = parseCursorProp(vm, obj)
				viewForegroundColor = getJSStringProp(obj, "foregroundColor")
				viewBackgroundColor = getJSStringProp(obj, "backgroundColor")
				viewKeyboardEnhancements = parseKeyboardEnhancementsProp(vm, obj)
				viewDisableBracketedPaste = getJSBoolProp(obj, "disableBracketedPasteMode")
				viewProgressBar = parseProgressBarProp(vm, obj)
				return nil
			}
		}

		// Plain string (or non-object): treat as content only (backward compatible).
		viewStr = result.String()
		if viewStr == "" {
			viewStr = "[BT] View returned empty string"
		}
		return nil
	})
	if err != nil {
		return tea.NewView(fmt.Sprintf("[BT] View error (event loop): %v", err))
	}

	// Cache the view if throttling is enabled.
	// For declarative view objects, we cache the parsed fields so that
	// cursedRenderer.viewEquals sees consistent mode fields on each render.
	if m.throttleEnabled {
		m.throttleMu.Lock()
		m.cachedView = viewStr
		m.cachedViewAltScreen = viewAltScreen
		m.cachedViewMouseMode = viewMouseMode
		m.cachedViewReportFocus = viewReportFocus
		m.cachedViewWindowTitle = viewWindowTitle
		m.cachedViewCursor = viewCursor
		m.cachedViewForegroundColor = viewForegroundColor
		m.cachedViewBackgroundColor = viewBackgroundColor
		m.cachedViewKeyboardEnhancements = viewKeyboardEnhancements
		m.cachedViewDisableBracketedPaste = viewDisableBracketedPaste
		m.cachedViewProgressBar = viewProgressBar
		m.throttleMu.Unlock()
	}

	v := tea.NewView(viewStr)
	if hasViewFields {
		// v2 declarative path: fields came from JS view() return object.
		v.AltScreen = viewAltScreen
		v.MouseMode = viewMouseMode
		v.ReportFocus = viewReportFocus
		v.WindowTitle = viewWindowTitle
		v.Cursor = viewCursor
		v.KeyboardEnhancements = viewKeyboardEnhancements
		v.DisableBracketedPasteMode = viewDisableBracketedPaste
		v.ProgressBar = viewProgressBar
		if viewForegroundColor != "" {
			v.ForegroundColor = parseColorValue(viewForegroundColor)
		}
		if viewBackgroundColor != "" {
			v.BackgroundColor = parseColorValue(viewBackgroundColor)
		}
	}
	return v
}

// viewDirectResult returns the raw JS value from the view function.
// This allows the caller to distinguish between string and object returns.
// MUST be called from event loop goroutine.
func (m *jsModel) viewDirectResult() (goja.Value, error) {
	// Ensure state is not nil before passing to JS
	state := m.state
	if state == nil || goja.IsUndefined(state) || goja.IsNull(state) {
		return goja.Null(), errors.New("state is nil/undefined")
	}

	result, err := m.viewFn(goja.Undefined(), state)
	if err != nil {
		return goja.Null(), err
	}
	if result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
		return goja.Null(), errors.New("view returned nil/undefined")
	}
	return result, nil
}

// msgToJS converts a tea.Msg to a JavaScript-compatible object.
// Handles all bubbletea message types comprehensively.
func (m *jsModel) msgToJS(msg tea.Msg) map[string]any {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		key := msg.Key()
		// v2 key contract: use msg.String() for the key field.
		// Space bar produces "space", not " ". The text field provides
		// the actual character (e.g., " " for printable space) via key.Text.
		keyStr := msg.String()
		text := key.Text
		mod := key.Mod

		return map[string]any{
			"type":        "Key",
			"key":         keyStr,
			"text":        text,
			"mod":         modToStrings(mod),
			"code":        key.Code,
			"shiftedCode": key.ShiftedCode,
			"baseCode":    key.BaseCode,
			"isRepeat":    key.IsRepeat,
		}

	case tea.KeyReleaseMsg:
		key := msg.Key()
		text := key.Text
		keyStr := msg.String()
		mod := key.Mod

		return map[string]any{
			"type":        "KeyRelease",
			"key":         keyStr,
			"text":        text,
			"mod":         modToStrings(mod),
			"code":        key.Code,
			"shiftedCode": key.ShiftedCode,
			"baseCode":    key.BaseCode,
			"isRepeat":    key.IsRepeat,
		}

	case tea.MouseMsg:
		// Use the generated MouseEventToJS which ensures consistency with tea.Mouse.String()
		return MouseEventToJS(msg)

	case tea.WindowSizeMsg:
		return map[string]any{
			"type":   "WindowSize",
			"width":  msg.Width,
			"height": msg.Height,
		}

	case tea.FocusMsg:
		return map[string]any{
			"type": "Focus",
		}

	case tea.BlurMsg:
		return map[string]any{
			"type": "Blur",
		}

	case tickMsg:
		return map[string]any{
			"type": "Tick",
			"id":   msg.id,
			"time": msg.time.UnixMilli(),
		}

	case quitMsg:
		m.quitCalled = true
		return map[string]any{
			"type": "Quit",
		}

	case clearScreenMsg:
		return map[string]any{
			"type": "ClearScreen",
		}

	case stateRefreshMsg:
		return map[string]any{
			"type": "StateRefresh",
			"key":  msg.key,
		}

	case tea.PasteMsg:
		return map[string]any{
			"type":    "Paste",
			"content": msg.Content,
		}

	case tea.PasteStartMsg:
		return map[string]any{
			"type": "PasteStart",
		}

	case tea.PasteEndMsg:
		return map[string]any{
			"type": "PasteEnd",
		}

	case renderRefreshMsg:
		// Internal message for render throttle - returns nil to skip JS processing
		// The Update method handles this specially to force a render
		return nil

	case toggleReturnMsg:
		resultMap := map[string]any{
			"type": "ToggleReturn",
		}
		maps.Copy(resultMap, msg.Result)
		return resultMap

	default:
		return nil
	}
}

// modToStrings converts a KeyMod to a slice of modifier name strings.
func modToStrings(mod tea.KeyMod) []string {
	var mods []string
	if mod.Contains(tea.ModCtrl) {
		mods = append(mods, "ctrl")
	}
	if mod.Contains(tea.ModAlt) {
		mods = append(mods, "alt")
	}
	if mod.Contains(tea.ModShift) {
		mods = append(mods, "shift")
	}
	if mod.Contains(tea.ModMeta) {
		mods = append(mods, "meta")
	}
	if mod.Contains(tea.ModHyper) {
		mods = append(mods, "hyper")
	}
	if mod.Contains(tea.ModSuper) {
		mods = append(mods, "super")
	}
	return mods
}

// getJSStringProp safely gets a string property from a JS object.
func getJSStringProp(obj *goja.Object, name string) string {
	if obj == nil {
		return ""
	}
	val := obj.Get(name)
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return ""
	}
	return val.String()
}

// getJSBoolProp safely gets a boolean property from a JS object.
func getJSBoolProp(obj *goja.Object, name string) bool {
	if obj == nil {
		return false
	}
	val := obj.Get(name)
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return false
	}
	return val.ToBoolean()
}

// parseMouseModeProp parses the mouseMode property from a JS view object.
// Accepts: "all"/"allMotion" -> AllMotion, "cell"/"cellMotion" -> CellMotion, else None.
// jsToTeaMouseClick converts a JS MouseClick message object to a tea.MouseClickMsg.
func jsToTeaMouseClick(obj *goja.Object) tea.Msg {
	if obj == nil {
		return nil
	}
	x := int(obj.Get("x").ToInteger())
	y := int(obj.Get("y").ToInteger())
	buttonStr := getJSStringProp(obj, "button")
	mod := jsModToKeyMod(obj)
	var button tea.MouseButton
	if def, ok := MouseButtonDefs[buttonStr]; ok {
		button = def.Button
	} else {
		button = tea.MouseNone
	}
	return tea.MouseClickMsg{X: x, Y: y, Button: button, Mod: mod}
}

// jsToTeaMouseRelease converts a JS MouseRelease message object to a tea.MouseReleaseMsg.
func jsToTeaMouseRelease(obj *goja.Object) tea.Msg {
	if obj == nil {
		return nil
	}
	x := int(obj.Get("x").ToInteger())
	y := int(obj.Get("y").ToInteger())
	mod := jsModToKeyMod(obj)
	return tea.MouseReleaseMsg{X: x, Y: y, Mod: mod}
}

// jsToTeaMouseMotion converts a JS MouseMotion message object to a tea.MouseMotionMsg.
func jsToTeaMouseMotion(obj *goja.Object) tea.Msg {
	if obj == nil {
		return nil
	}
	x := int(obj.Get("x").ToInteger())
	y := int(obj.Get("y").ToInteger())
	buttonStr := getJSStringProp(obj, "button")
	mod := jsModToKeyMod(obj)
	var button tea.MouseButton
	if def, ok := MouseButtonDefs[buttonStr]; ok {
		button = def.Button
	} else {
		button = tea.MouseNone
	}
	return tea.MouseMotionMsg{X: x, Y: y, Button: button, Mod: mod}
}

// jsToTeaMouseWheel converts a JS MouseWheel message object to a tea.MouseWheelMsg.
func jsToTeaMouseWheel(obj *goja.Object) tea.Msg {
	if obj == nil {
		return nil
	}
	x := int(obj.Get("x").ToInteger())
	y := int(obj.Get("y").ToInteger())
	buttonStr := getJSStringProp(obj, "button")
	mod := jsModToKeyMod(obj)
	var button tea.MouseButton
	if def, ok := MouseButtonDefs[buttonStr]; ok {
		button = def.Button
	} else {
		button = tea.MouseNone
	}
	return tea.MouseWheelMsg{X: x, Y: y, Button: button, Mod: mod}
}

// jsModToKeyMod converts a JS mod array property to a tea.KeyMod.
// The mod field must be a JS Array of modifier name strings (e.g., ["ctrl", "alt"]).
// If mod is absent, nil, or not an array, returns 0 (no modifiers).
func jsModToKeyMod(obj *goja.Object) tea.KeyMod {
	modVal := obj.Get("mod")
	if modVal == nil || goja.IsUndefined(modVal) || goja.IsNull(modVal) {
		return 0
	}
	modArr, ok := modVal.(*goja.Object)
	if !ok || modArr.ClassName() != "Array" {
		return 0
	}
	// Export array to Go []interface{} and iterate elements
	arr := modArr.Export()
	arrSlice, ok := arr.([]interface{})
	if !ok {
		return 0
	}
	var mod tea.KeyMod
	for _, elem := range arrSlice {
		v, ok := elem.(string)
		if !ok {
			continue
		}
		switch v {
		case "ctrl":
			mod |= tea.ModCtrl
		case "alt":
			mod |= tea.ModAlt
		case "shift":
			mod |= tea.ModShift
		case "meta":
			mod |= tea.ModMeta
		case "hyper":
			mod |= tea.ModHyper
		case "super":
			mod |= tea.ModSuper
		}
	}
	return mod
}

func parseMouseModeProp(obj *goja.Object, name string) tea.MouseMode {
	if obj == nil {
		return tea.MouseModeNone
	}
	val := obj.Get(name)
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return tea.MouseModeNone
	}
	modeStr := val.String()
	switch modeStr {
	case "all", "allMotion", "AllMotion":
		return tea.MouseModeAllMotion
	case "cell", "cellMotion", "CellMotion":
		return tea.MouseModeCellMotion
	default:
		return tea.MouseModeNone
	}
}

// parseCursorProp parses the cursor property from a JS view object.
// Accepts: {x: int, y: int, shape: "block"|"underline"|"bar", blink: bool, color: string}
// Returns nil if the property is absent, null, or undefined.
func parseCursorProp(vm *goja.Runtime, obj *goja.Object) *tea.Cursor {
	if obj == nil {
		return nil
	}
	val := obj.Get("cursor")
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil
	}
	cursorObj := val.ToObject(vm)
	if cursorObj == nil {
		return nil
	}

	x := int(cursorObj.Get("x").ToInteger())
	y := int(cursorObj.Get("y").ToInteger())
	cursor := tea.NewCursor(x, y)

	// Parse shape: "block" (default), "underline", "bar"
	if shapeVal := cursorObj.Get("shape"); shapeVal != nil && !goja.IsUndefined(shapeVal) && !goja.IsNull(shapeVal) {
		switch strings.ToLower(shapeVal.String()) {
		case "underline":
			cursor.Shape = tea.CursorUnderline
		case "bar":
			cursor.Shape = tea.CursorBar
		default:
			cursor.Shape = tea.CursorBlock
		}
	}

	// Parse blink
	if blinkVal := cursorObj.Get("blink"); blinkVal != nil && !goja.IsUndefined(blinkVal) && !goja.IsNull(blinkVal) {
		cursor.Blink = blinkVal.ToBoolean()
	}

	// Parse color
	if colorVal := cursorObj.Get("color"); colorVal != nil && !goja.IsUndefined(colorVal) && !goja.IsNull(colorVal) {
		colorStr := colorVal.String()
		if colorStr != "" {
			cursor.Color = parseColorValue(colorStr)
		}
	}

	return cursor
}

// parseColorValue parses a color string into a color.Color.
// Accepts hex colors ("#ff0000", "#f00", "#ff000080") and ANSI 256 color
// indices ("1", "21", "196") via lipgloss.Color.
func parseColorValue(s string) color.Color {
	if s == "" {
		return nil
	}
	return lipgloss.Color(s)
}

// parseKeyboardEnhancementsProp parses the keyboardEnhancements property.
// Accepts: true (shorthand for {reportEventTypes: true}) or {reportEventTypes: bool}
func parseKeyboardEnhancementsProp(vm *goja.Runtime, obj *goja.Object) tea.KeyboardEnhancements {
	if obj == nil {
		return tea.KeyboardEnhancements{}
	}
	val := obj.Get("keyboardEnhancements")
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return tea.KeyboardEnhancements{}
	}

	// Accept boolean true as shorthand for {reportEventTypes: true}
	if b, ok := val.Export().(bool); ok {
		return tea.KeyboardEnhancements{ReportEventTypes: b}
	}

	keObj := val.ToObject(vm)
	if keObj == nil {
		return tea.KeyboardEnhancements{}
	}
	return tea.KeyboardEnhancements{
		ReportEventTypes: getJSBoolProp(keObj, "reportEventTypes"),
	}
}

// parseProgressBarProp parses the progressBar property from a JS view object.
// Accepts: {state: "default"|"error"|"indeterminate"|"warning"|"none", value: int}
// Returns nil if the property is absent, null, or undefined.
func parseProgressBarProp(vm *goja.Runtime, obj *goja.Object) *tea.ProgressBar {
	if obj == nil {
		return nil
	}
	val := obj.Get("progressBar")
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil
	}
	pbObj := val.ToObject(vm)
	if pbObj == nil {
		return nil
	}

	stateStr := getJSStringProp(pbObj, "state")
	var state tea.ProgressBarState
	switch strings.ToLower(stateStr) {
	case "default":
		state = tea.ProgressBarDefault
	case "error":
		state = tea.ProgressBarError
	case "indeterminate":
		state = tea.ProgressBarIndeterminate
	case "warning":
		state = tea.ProgressBarWarning
	case "none", "":
		state = tea.ProgressBarNone
	default:
		state = tea.ProgressBarNone
	}

	value := 0
	if valProp := pbObj.Get("value"); valProp != nil && !goja.IsUndefined(valProp) && !goja.IsNull(valProp) {
		value = int(valProp.ToInteger())
	}

	return tea.NewProgressBar(state, value)
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

	// In v2, terminal state is controlled declaratively via tea.View fields.
	// These command types are removed. Scripts should return view objects
	// with the appropriate fields set (altScreen, mouseMode, reportFocus, etc.)
	// instead of using imperative commands.

	case "requestWindowSize":
		return tea.RequestWindowSize
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
	for i := range length {
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
	for i := range length {
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

// toggleReturnMsg is sent to the model after a toggle key lifecycle completes
// (terminal released → JS callback → terminal restored). JS receives this
// as { type: "ToggleReturn", ... } with the onToggle callback's return value
// merged into the message (e.g., reason, error from switchTo).
type toggleReturnMsg struct {
	// Result from the onToggle JS callback (nil if callback returned nothing).
	Result map[string]any
}

// toggleModel wraps a tea.Model to intercept a toggle key and execute
// terminal lifecycle management around a JS callback. This enables BubbleTea
// programs to integrate with termmux passthrough mode.
//
// When the toggle key is pressed:
//  1. ReleaseTerminal — pauses BubbleTea renderer/input
//  2. Sync write \x1b[?1049l — exit alt-screen (idempotent belt)
//  3. Call JS onToggle via RunJSSync — typically mux.switchTo() which blocks
//  4. Sync write \x1b[?1049h\x1b[2J\x1b[H — enter alt-screen (idempotent belt)
//  5. RestoreTerminal — resume BubbleTea renderer/input
//
// All other keys/messages pass through to the inner model unchanged.
type toggleModel struct {
	inner     tea.Model
	toggleKey byte          // Raw byte of the toggle key (e.g., 0x1D for Ctrl+])
	onToggle  goja.Callable // JS callback executed between Release/Restore
	jsRunner  JSRunner      // For thread-safe JS execution from BubbleTea goroutine
	output    io.Writer     // Terminal output for sync escape sequences
	mu        sync.Mutex    // Protects program
	program   *tea.Program  // Set by runProgram after NewProgram, before Run
}

func (m *toggleModel) Init() tea.Cmd {
	return m.inner.Init()
}

func (m *toggleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		if m.isToggleKey(keyMsg) {
			return m, m.toggleCmd()
		}
	}
	innerModel, cmd := m.inner.Update(msg)
	m.inner = innerModel
	return m, cmd
}

func (m *toggleModel) View() tea.View {
	return m.inner.View()
}

// isToggleKey checks if a BubbleTea key message matches the configured toggle key.
func (m *toggleModel) isToggleKey(msg tea.KeyPressMsg) bool {
	k := msg.Key()
	// Match by rune (handles most key configurations)
	if k.Code == rune(m.toggleKey) {
		return true
	}
	// Match Ctrl+] specifically (0x1D) — v2 parses this with Code = ']' and Mod = ModCtrl
	if k.Code == ']' && k.Mod.Contains(tea.ModCtrl) && m.toggleKey == 0x1D {
		return true
	}
	return false
}

// toggleCmd returns a tea.Cmd that executes the full toggle lifecycle.
// The command runs on BubbleTea's command goroutine and blocks during passthrough.
func (m *toggleModel) toggleCmd() tea.Cmd {
	return func() tea.Msg {
		m.mu.Lock()
		p := m.program
		output := m.output
		m.mu.Unlock()

		// Release BubbleTea's terminal control (renderer, raw mode, input)
		if p != nil {
			p.ReleaseTerminal()
		}
		// Sync exit alt-screen — idempotent belt for async ReleaseTerminal
		if output != nil {
			_, _ = output.Write([]byte("\x1b[?1049l"))
		}

		// Call JS toggle handler (typically mux.switchTo() — blocks during passthrough).
		// Capture the return value to forward to the model (e.g., exit reason).
		var toggleResult map[string]any
		if m.jsRunner != nil && m.onToggle != nil {
			_ = m.jsRunner.RunJSSync(func(vm *goja.Runtime) error {
				val, err := m.onToggle(goja.Undefined())
				if err != nil {
					return err
				}
				if val != nil && !goja.IsUndefined(val) && !goja.IsNull(val) {
					if exported, ok := val.Export().(map[string]any); ok {
						toggleResult = exported
					}
				}
				return nil
			})
		}

		// Sync enter alt-screen + clear — idempotent belt for async RestoreTerminal
		if output != nil {
			_, _ = output.Write([]byte("\x1b[?1049h\x1b[2J\x1b[H"))
		}
		// Restore BubbleTea's terminal control
		if p != nil {
			p.RestoreTerminal()
		}

		return toggleReturnMsg{Result: toggleResult}
	}
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
		createCommand := func(cmdType string, props map[string]any) goja.Value {
			cmdID := generateCommandID()
			if currentModel != nil {
				currentModel.registerCommand(cmdID)
			}
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

			// Set as current model for command registration
			currentModel = model

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
			// loop goroutine is NOT blocked and can process RunJSSync
			// callbacks from BubbleTea's Init/Update/View.
			//
			// Threading model:
			// - ExecuteScript routes scripts through the event loop so ALL
			//   Goja VM access happens on a single goroutine.
			// - tea.run() returns immediately; BubbleTea runs on its own goroutines.
			// - BubbleTea calls Init/Update/View via JSRunner.RunJSSync()
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

			go func() {
				done <- manager.runProgram(programModel)
			}()

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
	}
}

// runProgram runs a bubbletea program with the given model.
// Handles TTY detection, terminal state cleanup, panic recovery, and proper I/O setup.
// If a panic occurs, the terminal is restored before printing the stack trace to stderr.
//
// In v2, terminal features (altScreen, mouse, etc.) are controlled declaratively
// via tea.View fields returned by the model's View() method, not via program options.
// unwrapOSFile extracts an *os.File from v, checking first for a direct
// *os.File and then for the UnwrapFile() interface used by wrapper types
// (e.g., TUIReader/TUIWriter).  Returns nil if v is nil or not unwrappable.
func unwrapOSFile(v any) *os.File {
	if v == nil {
		return nil
	}
	if f, ok := v.(*os.File); ok {
		return f
	}
	if u, ok := v.(interface{ UnwrapFile() *os.File }); ok {
		return u.UnwrapFile()
	}
	return nil
}

func (m *Manager) runProgram(model tea.Model) (err error) {
	// Debug: check if manager is properly initialized
	if m == nil {
		return fmt.Errorf("runProgram: manager is nil")
	}
	if m.ctx == nil {
		return fmt.Errorf("runProgram: manager.ctx is nil")
	}

	// Enable bracketed paste mode so \x1b[200~...\x1b[201~ sequences are
	// recognized as paste events (tea.PasteStartMsg/PasteEndMsg/PasteMsg).
	// We send this to the terminal here, before BubbleTea's read loop starts.
	// Note: bracketed paste mode is always enabled; the jsModel-level field was
	// removed as it was dead code (never read, only written).
	if m.output != nil {
		_, _ = m.output.Write([]byte("\x1b[?2004h"))
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

	// restoreTerminal restores the terminal state and disables bracketed paste mode.
	restoreTerminal := func() {
		// Disable bracketed paste mode (DECRST 200)
		if m.output != nil {
			_, _ = m.output.Write([]byte("\x1b[?2004l"))
		}
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

	// Configure input/output.
	// In v2, BubbleTea opens /dev/tty for input when no explicit input is provided
	// via WithInput.  We need to pass the real *os.File so BubbleTea can detect
	// TTY capabilities (raw mode, signals, etc.).
	//
	// Input/output may be wrapper types (e.g., TUIReader) that aren't *os.File
	// but wrap one.  We check for the UnwrapFile() interface to recover the
	// underlying file in those cases.
	var opts []tea.ProgramOption
	if f := unwrapOSFile(input); f != nil {
		opts = append(opts, tea.WithInput(f))
	}
	if f := unwrapOSFile(output); f != nil {
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
	// If the model is wrapped in a toggleModel, unwrap to reach the jsModel
	// and set the program reference on the toggle wrapper too.
	actualModel := model
	if tm, ok := model.(*toggleModel); ok {
		tm.mu.Lock()
		tm.program = p
		tm.mu.Unlock()
		actualModel = tm.inner
		defer func() {
			tm.mu.Lock()
			tm.program = nil
			tm.mu.Unlock()
		}()
	}
	if jm, ok := actualModel.(*jsModel); ok {
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
	wg.Go(func() {
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
	})

	_, runErr := p.Run()
	close(programFinished) // Signal that Run() returned
	cancel(nil)            // Signal the goroutine to exit (via ctx.Done path if race, but programFinished priority)
	wg.Wait()              // Wait for the goroutine to finish

	if runErr != nil {
		return fmt.Errorf("failed to run program: %w", runErr)
	}

	return nil
}
