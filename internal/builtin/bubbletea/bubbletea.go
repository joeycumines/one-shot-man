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
//	// Commands
//	tea.quit();         // Quit the program
//	tea.clearScreen();  // Clear the screen
//	tea.batch(...cmds); // Batch multiple commands
//	tea.sequence(...cmds); // Execute commands in sequence
//
//	// Key events
//	// msg.type === 'keyPress'
//	// msg.key - the key name ('q', 'enter', 'up', 'down', etc.)
//	// msg.alt - alt modifier
//	// msg.ctrl - ctrl modifier
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
//	// Program options
//	tea.run(model, {
//	    altScreen: true,     // Use alternate screen buffer
//	    mouse: true,         // Enable mouse support
//	    bracketed: true,     // Enable bracketed paste
//	});
//
// # Pathway to Full Support
//
// This implementation exposes the core bubbletea API. The following describes
// the pathway to achieving full parity with the native Go bubbletea library.
//
// ## Currently Implemented
//
//   - Model creation (init, update, view functions)
//   - Program execution with tea.run()
//   - Commands: quit, clearScreen, batch, sequence
//   - Key events (keyPress with key, alt modifier)
//   - Mouse events (mouse with x, y, button, action, modifiers)
//   - Window size events (windowSize with width, height)
//   - Program options: altScreen, mouse
//
// ## Phase 1: Enhanced Commands (recommended next steps)
//
//   - tea.tick(duration, callback) - Timer commands for animations
//   - tea.execProcess(cmd, args) - Execute external processes
//   - tea.setWindowTitle(title) - Set terminal window title
//   - tea.hideCursor() / tea.showCursor() - Cursor visibility control
//   - tea.enterAltScreen() / tea.exitAltScreen() - Runtime screen buffer control
//
// ## Phase 2: Advanced Input Handling
//
//   - Bracketed paste mode with paste events
//   - Focus events (focus, blur)
//   - Custom key bindings with rune support
//   - Raw mode for low-level input access
//   - Input filtering and transformation
//
// ## Phase 3: Program Lifecycle
//
//   - Program suspension and resumption (e.g., Ctrl+Z handling)
//   - Graceful shutdown with cleanup callbacks
//   - Multiple concurrent programs
//   - Program-to-program communication
//   - Context cancellation integration
//
// ## Phase 4: Component Library
//
//   - Spinner component with customizable styles
//   - Progress bar component
//   - Text input component with validation
//   - Table/list components with selection
//   - Viewport component for scrollable content
//   - File picker component
//   - Confirmation dialog component
//
// ## Implementation Notes
//
// All additions should follow these patterns:
//
//  1. No global state - all state managed per Manager instance
//  2. JavaScript callbacks must be properly synchronized with Go goroutines
//  3. All functionality exposed via Require() function pattern
//  4. Comprehensive unit tests using simulation screens
//  5. Deterministic testing - no timing-dependent tests
package bubbletea

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dop251/goja"
)

// Manager holds bubbletea-related state per engine instance.
type Manager struct {
	ctx          context.Context
	mu           sync.Mutex
	input        io.Reader
	output       io.Writer
	signalNotify func(c chan<- os.Signal, sig ...os.Signal)
	signalStop   func(c chan<- os.Signal)
}

// NewManager creates a new bubbletea manager for an engine instance.
// Input and output can be nil to use os.Stdin and os.Stdout.
func NewManager(
	ctx context.Context,
	input io.Reader,
	output io.Writer,
	signalNotify func(c chan<- os.Signal, sig ...os.Signal),
	signalStop func(c chan<- os.Signal),
) *Manager {
	if input == nil {
		input = os.Stdin
	}
	if output == nil {
		output = os.Stdout
	}
	if signalNotify == nil {
		signalNotify = signal.Notify
	}
	if signalStop == nil {
		signalStop = signal.Stop
	}
	return &Manager{
		ctx:          ctx,
		input:        input,
		output:       output,
		signalNotify: signalNotify,
		signalStop:   signalStop,
	}
}

// jsModel wraps a JavaScript model definition for bubbletea.
type jsModel struct {
	runtime    *goja.Runtime
	initFn     goja.Callable
	updateFn   goja.Callable
	viewFn     goja.Callable
	state      goja.Value
	quitOnce   sync.Once
	quitCalled bool
}

// Init implements tea.Model.
func (m *jsModel) Init() tea.Cmd {
	// Call JS init function to get initial state
	result, err := m.initFn(goja.Undefined())
	if err != nil {
		return nil
	}
	m.state = result
	return nil
}

// Update implements tea.Model.
func (m *jsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	jsMsg := m.msgToJS(msg)
	if jsMsg == nil {
		return m, nil
	}

	result, err := m.updateFn(goja.Undefined(), m.state, m.runtime.ToValue(jsMsg))
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
	result, err := m.viewFn(goja.Undefined(), m.state)
	if err != nil {
		return ""
	}
	return result.String()
}

// msgToJS converts a tea.Msg to a JavaScript-compatible object.
func (m *jsModel) msgToJS(msg tea.Msg) map[string]interface{} {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return map[string]interface{}{
			"type": "keyPress",
			"key":  msg.String(),
			"alt":  msg.Alt,
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
	case quitMsg:
		m.quitCalled = true
		return map[string]interface{}{
			"type": "quit",
		}
	case clearScreenMsg:
		return map[string]interface{}{
			"type": "clearScreen",
		}
	default:
		return nil
	}
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

	switch cmdType.String() {
	case "quit":
		return tea.Quit
	case "clearScreen":
		return tea.ClearScreen
	case "batch":
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
	case "sequence":
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

	return nil
}

// quitMsg is a custom message type for quit.
type quitMsg struct{}

// clearScreenMsg is a custom message type for clear screen.
type clearScreenMsg struct{}

// Require returns a CommonJS native module under "osm:bubbletea".
// It exposes bubbletea functionality for building terminal UIs.
func Require(baseCtx context.Context, manager *Manager) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		// newModel creates a new model wrapper from JS definition
		_ = exports.Set("newModel", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return runtime.ToValue(map[string]interface{}{
					"error": "newModel requires a config object",
				})
			}

			config := call.Argument(0).ToObject(runtime)
			if config == nil {
				return runtime.ToValue(map[string]interface{}{
					"error": "config must be an object",
				})
			}

			initFn, ok := goja.AssertFunction(config.Get("init"))
			if !ok {
				return runtime.ToValue(map[string]interface{}{
					"error": "init must be a function",
				})
			}

			updateFn, ok := goja.AssertFunction(config.Get("update"))
			if !ok {
				return runtime.ToValue(map[string]interface{}{
					"error": "update must be a function",
				})
			}

			viewFn, ok := goja.AssertFunction(config.Get("view"))
			if !ok {
				return runtime.ToValue(map[string]interface{}{
					"error": "view must be a function",
				})
			}

			model := &jsModel{
				runtime:  runtime,
				initFn:   initFn,
				updateFn: updateFn,
				viewFn:   viewFn,
			}

			// Return a wrapper object with the model reference
			wrapper := runtime.NewObject()
			_ = wrapper.Set("_model", runtime.ToValue(model))
			_ = wrapper.Set("_type", "bubbleteaModel")
			return wrapper
		})

		// run executes a bubbletea program
		_ = exports.Set("run", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return runtime.ToValue(map[string]interface{}{
					"error": "run requires a model",
				})
			}

			modelWrapper := call.Argument(0).ToObject(runtime)
			if modelWrapper == nil {
				return runtime.ToValue(map[string]interface{}{
					"error": "model must be an object",
				})
			}

			typeVal := modelWrapper.Get("_type")
			if typeVal == nil || goja.IsUndefined(typeVal) || goja.IsNull(typeVal) {
				return runtime.ToValue(map[string]interface{}{
					"error": "invalid model object",
				})
			}
			if typeVal.String() != "bubbleteaModel" {
				return runtime.ToValue(map[string]interface{}{
					"error": "invalid model object",
				})
			}

			modelVal := modelWrapper.Get("_model")
			if goja.IsUndefined(modelVal) || goja.IsNull(modelVal) {
				return runtime.ToValue(map[string]interface{}{
					"error": "failed to extract model",
				})
			}
			model, ok := modelVal.Export().(*jsModel)
			if !ok || model == nil {
				return runtime.ToValue(map[string]interface{}{
					"error": "failed to extract model",
				})
			}

			// Parse options
			var opts []tea.ProgramOption
			if len(call.Arguments) > 1 {
				optObj := call.Argument(1).ToObject(runtime)
				if optObj != nil {
					if altScreen := optObj.Get("altScreen"); !goja.IsUndefined(altScreen) && altScreen.ToBoolean() {
						opts = append(opts, tea.WithAltScreen())
					}
					if mouse := optObj.Get("mouse"); !goja.IsUndefined(mouse) && mouse.ToBoolean() {
						opts = append(opts, tea.WithMouseAllMotion())
					}
				}
			}

			// Run the program
			if err := manager.runProgram(model, opts...); err != nil {
				return runtime.ToValue(map[string]interface{}{
					"error": err.Error(),
				})
			}

			return goja.Undefined()
		})

		// quit returns a quit command
		_ = exports.Set("quit", func(call goja.FunctionCall) goja.Value {
			return runtime.ToValue(map[string]interface{}{
				"_cmdType": "quit",
			})
		})

		// clearScreen returns a clear screen command
		_ = exports.Set("clearScreen", func(call goja.FunctionCall) goja.Value {
			return runtime.ToValue(map[string]interface{}{
				"_cmdType": "clearScreen",
			})
		})

		// batch combines multiple commands
		_ = exports.Set("batch", func(call goja.FunctionCall) goja.Value {
			cmds := make([]interface{}, 0, len(call.Arguments))
			for _, arg := range call.Arguments {
				if !goja.IsUndefined(arg) && !goja.IsNull(arg) {
					cmds = append(cmds, arg.Export())
				}
			}
			return runtime.ToValue(map[string]interface{}{
				"_cmdType": "batch",
				"cmds":     cmds,
			})
		})

		// sequence executes commands in sequence
		_ = exports.Set("sequence", func(call goja.FunctionCall) goja.Value {
			cmds := make([]interface{}, 0, len(call.Arguments))
			for _, arg := range call.Arguments {
				if !goja.IsUndefined(arg) && !goja.IsNull(arg) {
					cmds = append(cmds, arg.Export())
				}
			}
			return runtime.ToValue(map[string]interface{}{
				"_cmdType": "sequence",
				"cmds":     cmds,
			})
		})
	}
}

// runProgram runs a bubbletea program with the given model.
func (m *Manager) runProgram(model tea.Model, opts ...tea.ProgramOption) error {
	ctx, cancel := context.WithCancelCause(m.ctx)
	defer cancel(nil)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Add input/output options
	if f, ok := m.input.(*os.File); ok {
		opts = append(opts, tea.WithInput(f))
	}
	if f, ok := m.output.(*os.File); ok {
		opts = append(opts, tea.WithOutput(f))
	}

	p := tea.NewProgram(model, opts...)

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	m.signalNotify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer m.signalStop(sigCh)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel(nil)
		select {
		case <-ctx.Done():
		case <-sigCh:
		}
		p.Quit()
	}()

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run program: %w", err)
	}

	return nil
}
