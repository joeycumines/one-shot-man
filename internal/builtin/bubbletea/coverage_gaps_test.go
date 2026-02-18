package bubbletea

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errorJSRunner is a JSRunner that always returns an error.
type errorJSRunner struct{}

func (r *errorJSRunner) RunJSSync(func(*goja.Runtime) error) error {
	return errors.New("event loop error")
}

// noopModel is a minimal tea.Model for creating *tea.Program test values.
type noopModel struct{}

func (noopModel) Init() tea.Cmd                       { return nil }
func (noopModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return noopModel{}, nil }
func (noopModel) View() string                        { return "" }

// ========================================================================
// valueToCmd — all cmd type descriptors
// ========================================================================

func TestValueToCmd_AllCmdTypes(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	model := &jsModel{runtime: vm}

	tests := []struct {
		name      string
		cmdType   string
		extra     func(*goja.Object) // optional extra properties
		expectNil bool
	}{
		{"quit", "quit", nil, false},
		{"clearScreen", "clearScreen", nil, false},
		{"hideCursor", "hideCursor", nil, false},
		{"showCursor", "showCursor", nil, false},
		{"enterAltScreen", "enterAltScreen", nil, false},
		{"exitAltScreen", "exitAltScreen", nil, false},
		{"enableBracketedPaste", "enableBracketedPaste", nil, false},
		{"disableBracketedPaste", "disableBracketedPaste", nil, false},
		{"enableReportFocus", "enableReportFocus", nil, false},
		{"disableReportFocus", "disableReportFocus", nil, false},
		{"windowSize", "windowSize", nil, false},
		{"setWindowTitle with title", "setWindowTitle", func(o *goja.Object) {
			_ = o.Set("title", "hello")
		}, false},
		{"setWindowTitle nil title", "setWindowTitle", nil, true},
		{"unknown type", "nonexistent", nil, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// No t.Parallel() — goja.Runtime is not thread-safe
			obj := vm.NewObject()
			_ = obj.Set("_cmdType", tc.cmdType)
			if tc.extra != nil {
				tc.extra(obj)
			}
			cmd := model.valueToCmd(obj)
			if tc.expectNil {
				assert.Nil(t, cmd)
			} else {
				assert.NotNil(t, cmd, "expected non-nil cmd for %s", tc.cmdType)
			}
		})
	}
}

func TestValueToCmd_NilModelOrRuntime(t *testing.T) {
	t.Parallel()

	t.Run("nil model", func(t *testing.T) {
		t.Parallel()
		var m *jsModel
		cmd := m.valueToCmd(goja.Undefined())
		assert.Nil(t, cmd)
	})

	t.Run("nil runtime", func(t *testing.T) {
		t.Parallel()
		m := &jsModel{runtime: nil}
		cmd := m.valueToCmd(goja.Undefined())
		assert.Nil(t, cmd)
	})
}

func TestValueToCmd_NullAndUndefinedCmd(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	model := &jsModel{runtime: vm}

	assert.Nil(t, model.valueToCmd(goja.Null()))
	assert.Nil(t, model.valueToCmd(goja.Undefined()))
}

func TestValueToCmd_WrappedGoCmd(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	model := &jsModel{runtime: vm}

	// Wrap a real tea.Cmd via WrapCmd
	goCmd := WrapCmd(vm, tea.Quit)
	cmd := model.valueToCmd(goCmd)
	assert.NotNil(t, cmd, "should extract wrapped Go cmd")
}

func TestValueToCmd_ForeignGoFunc(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	model := &jsModel{runtime: vm}

	// Object without _cmdType → should warn and return nil
	obj := vm.NewObject()
	cmd := model.valueToCmd(vm.ToValue(obj))
	assert.Nil(t, cmd, "should return nil for object without _cmdType")
}

// ========================================================================
// NewManager / NewManagerWithStderr panic on nil jsRunner
// ========================================================================

func TestNewManager_NilJSRunner_Panics(t *testing.T) {
	t.Parallel()
	assert.Panics(t, func() {
		NewManager(context.Background(), nil, nil, nil, nil, nil)
	}, "should panic on nil jsRunner")
}

func TestNewManagerWithStderr_NilJSRunner_Panics(t *testing.T) {
	t.Parallel()
	assert.Panics(t, func() {
		NewManagerWithStderr(context.Background(), nil, nil, nil, nil, nil, nil)
	}, "should panic on nil jsRunner")
}

// ========================================================================
// NewManagerWithStderr — TTY detection branches
// ========================================================================

// terminalChecker implements TerminalChecker for testing.
type terminalChecker struct {
	isTerminal bool
	fd         uintptr
	data       []byte // data to read for io.Reader
	pos        int
}

func (tc *terminalChecker) IsTerminal() bool { return tc.isTerminal }
func (tc *terminalChecker) Fd() uintptr      { return tc.fd }
func (tc *terminalChecker) Read(p []byte) (int, error) {
	if tc.pos >= len(tc.data) {
		return 0, nil
	}
	n := copy(p, tc.data[tc.pos:])
	tc.pos += n
	return n, nil
}
func (tc *terminalChecker) Write(p []byte) (int, error) { return len(p), nil }

func TestNewManagerWithStderr_TerminalCheckerInput(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	runner := &SyncJSRunner{Runtime: vm}

	// Input implements TerminalChecker, is terminal, valid fd
	input := &terminalChecker{isTerminal: true, fd: 42}
	m := NewManagerWithStderr(context.Background(), input, nil, nil, runner, nil, nil)
	assert.True(t, m.isTTY)
	assert.Equal(t, 42, m.ttyFd)
}

func TestNewManagerWithStderr_TerminalCheckerInput_InvalidFd(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	runner := &SyncJSRunner{Runtime: vm}

	// Input is terminal but fd is invalid (^uintptr(0))
	input := &terminalChecker{isTerminal: true, fd: ^uintptr(0)}
	m := NewManagerWithStderr(context.Background(), input, nil, nil, runner, nil, nil)
	assert.False(t, m.isTTY, "should be false when fd is invalid")
}

func TestNewManagerWithStderr_TerminalCheckerInput_NotTerminal(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	runner := &SyncJSRunner{Runtime: vm}

	// Input is NOT a terminal
	input := &terminalChecker{isTerminal: false, fd: 42}
	m := NewManagerWithStderr(context.Background(), input, nil, nil, runner, nil, nil)
	// Without output TTY, should be false
	assert.False(t, m.isTTY)
}

func TestNewManagerWithStderr_TerminalCheckerOutput(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	runner := &SyncJSRunner{Runtime: vm}

	// Input is not TerminalChecker (plain reader), output IS TerminalChecker
	input := &terminalChecker{isTerminal: false, fd: 1}
	output := &terminalChecker{isTerminal: true, fd: 99}
	m := NewManagerWithStderr(context.Background(), input, output, nil, runner, nil, nil)
	assert.True(t, m.isTTY)
	assert.Equal(t, 99, m.ttyFd)
}

func TestNewManagerWithStderr_TerminalCheckerOutput_InvalidFd(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	runner := &SyncJSRunner{Runtime: vm}

	input := &terminalChecker{isTerminal: false, fd: 1}
	output := &terminalChecker{isTerminal: true, fd: ^uintptr(0)}
	m := NewManagerWithStderr(context.Background(), input, output, nil, runner, nil, nil)
	assert.False(t, m.isTTY, "output with invalid fd should not set TTY")
}

func TestNewManagerWithStderr_NilCtx(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	runner := &SyncJSRunner{Runtime: vm}

	// nil ctx should default to context.Background()
	m := NewManagerWithStderr(nil, nil, nil, nil, runner, nil, nil) //lint:ignore SA1012 testing nil-ctx fallback path
	assert.NotNil(t, m.ctx)
}

// ========================================================================
// initDirect — error and nil/undefined paths
// ========================================================================

func TestInitDirect_ErrorPath(t *testing.T) {
	t.Parallel()
	vm := goja.New()

	model := &jsModel{
		runtime: vm,
		initFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			return nil, errors.New("init kaboom")
		},
		state: vm.NewObject(),
	}

	cmd := model.initDirect()
	assert.Nil(t, cmd)
	assert.Contains(t, model.initError, "Init error")
	assert.Contains(t, model.initError, "init kaboom")
}

func TestInitDirect_NilUndefinedReturn(t *testing.T) {
	t.Parallel()
	vm := goja.New()

	model := &jsModel{
		runtime: vm,
		initFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			return nil, nil // returns nil
		},
		state: vm.NewObject(),
	}

	cmd := model.initDirect()
	assert.Nil(t, cmd)
	assert.Contains(t, model.initError, "nil/undefined")
}

func TestInitDirect_ArrayWithNilState(t *testing.T) {
	t.Parallel()
	vm := goja.New()

	model := &jsModel{
		runtime: vm,
		initFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			// Return [null, quit]
			quit := vm.NewObject()
			_ = quit.Set("_cmdType", "quit")
			return vm.NewArray(goja.Null(), quit), nil
		},
		state: vm.NewObject(),
	}

	cmd := model.initDirect()
	assert.NotNil(t, cmd, "should extract cmd from array even with nil state")
	assert.NotNil(t, model.state, "should use empty object for nil state")
}

// ========================================================================
// updateDirect — error and non-array paths
// ========================================================================

func TestUpdateDirect_ErrorPath(t *testing.T) {
	t.Parallel()
	vm := goja.New()

	model := &jsModel{
		runtime: vm,
		updateFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			return nil, errors.New("update kaboom")
		},
		state: vm.NewObject(),
	}

	cmd := model.updateDirect(map[string]interface{}{"type": "Key", "key": "q"})
	assert.Nil(t, cmd)
}

func TestUpdateDirect_NonArrayReturn(t *testing.T) {
	t.Parallel()
	vm := goja.New()

	model := &jsModel{
		runtime: vm,
		updateFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			// Return a plain object, not an array
			return vm.ToValue("just a string"), nil
		},
		state: vm.NewObject(),
	}

	cmd := model.updateDirect(map[string]interface{}{"type": "Key", "key": "q"})
	assert.Nil(t, cmd, "should return nil for non-array update result")
}

func TestUpdateDirect_NilState(t *testing.T) {
	t.Parallel()
	vm := goja.New()

	model := &jsModel{
		runtime: vm,
		updateFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			// Return valid [state, null]
			return vm.NewArray(vm.NewObject(), goja.Null()), nil
		},
		state: nil, // state is nil
	}

	cmd := model.updateDirect(map[string]interface{}{"type": "Key"})
	assert.Nil(t, cmd)
}

// ========================================================================
// View — nil guards and initError display
// ========================================================================

func TestView_NilGuards(t *testing.T) {
	t.Parallel()

	t.Run("nil model", func(t *testing.T) {
		t.Parallel()
		var m *jsModel
		assert.Contains(t, m.View(), "nil model/viewFn/runtime")
	})

	t.Run("nil viewFn", func(t *testing.T) {
		t.Parallel()
		vm := goja.New()
		m := &jsModel{runtime: vm, viewFn: nil}
		assert.Contains(t, m.View(), "nil model/viewFn/runtime")
	})

	t.Run("nil runtime", func(t *testing.T) {
		t.Parallel()
		m := &jsModel{
			runtime: nil,
			viewFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
				return nil, nil
			},
		}
		assert.Contains(t, m.View(), "nil model/viewFn/runtime")
	})
}

func TestView_InitError(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	m := &jsModel{
		runtime:   vm,
		initError: "Init error: something went wrong",
		viewFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			return vm.ToValue("should not appear"), nil
		},
		state: vm.NewObject(),
	}
	m.jsRunner = &SyncJSRunner{Runtime: vm}

	output := m.View()
	assert.Contains(t, output, "Init error")
	assert.Contains(t, output, "something went wrong")
	assert.NotContains(t, output, "should not appear")
}

// ========================================================================
// viewDirect — nil/undefined return
// ========================================================================

func TestViewDirect_NilReturn(t *testing.T) {
	t.Parallel()
	vm := goja.New()

	model := &jsModel{
		runtime: vm,
		viewFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			return nil, nil
		},
		state: vm.NewObject(),
	}

	output := model.viewDirect()
	assert.Equal(t, "[BT] View returned nil/undefined", output)
}

func TestViewDirect_UndefinedReturn(t *testing.T) {
	t.Parallel()
	vm := goja.New()

	model := &jsModel{
		runtime: vm,
		viewFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			return goja.Undefined(), nil
		},
		state: vm.NewObject(),
	}

	output := model.viewDirect()
	assert.Equal(t, "[BT] View returned nil/undefined", output)
}

func TestViewDirect_NilState(t *testing.T) {
	t.Parallel()
	vm := goja.New()

	model := &jsModel{
		runtime: vm,
		viewFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			return vm.ToValue("ok"), nil
		},
		state: nil,
	}

	output := model.viewDirect()
	assert.Equal(t, "[BT] View: state is nil/undefined", output)
}

// ========================================================================
// Update — nil guards
// ========================================================================

func TestUpdate_NilGuards(t *testing.T) {
	t.Parallel()

	t.Run("nil model", func(t *testing.T) {
		t.Parallel()
		var m *jsModel
		retModel, cmd := m.Update(tea.KeyMsg{})
		assert.Nil(t, retModel)
		assert.Nil(t, cmd)
	})

	t.Run("nil updateFn", func(t *testing.T) {
		t.Parallel()
		m := &jsModel{runtime: goja.New(), updateFn: nil}
		retModel, cmd := m.Update(tea.KeyMsg{})
		assert.Equal(t, m, retModel)
		assert.Nil(t, cmd)
	})
}

// ========================================================================
// Init — nil guards
// ========================================================================

func TestInit_NilGuards(t *testing.T) {
	t.Parallel()

	t.Run("nil model", func(t *testing.T) {
		t.Parallel()
		var m *jsModel
		cmd := m.Init()
		assert.Nil(t, cmd)
	})

	t.Run("nil initFn", func(t *testing.T) {
		t.Parallel()
		m := &jsModel{runtime: goja.New(), initFn: nil}
		cmd := m.Init()
		assert.Nil(t, cmd)
	})

	t.Run("nil runtime", func(t *testing.T) {
		t.Parallel()
		m := &jsModel{
			runtime: nil,
			initFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
				return nil, nil
			},
		}
		cmd := m.Init()
		assert.Nil(t, cmd)
	})
}

// ========================================================================
// JSToMouseEvent — unknown button/action fallback
// ========================================================================

func TestJSToMouseEvent_UnknownInputs(t *testing.T) {
	t.Parallel()

	t.Run("unknown button → MouseButtonNone", func(t *testing.T) {
		t.Parallel()
		msg := JSToMouseEvent("totally_bogus_button", "press", 1, 2, false, false, false)
		assert.Equal(t, tea.MouseButtonNone, tea.MouseEvent(msg).Button)
		assert.Equal(t, tea.MouseActionPress, tea.MouseEvent(msg).Action)
	})

	t.Run("unknown action → MouseActionPress", func(t *testing.T) {
		t.Parallel()
		msg := JSToMouseEvent("left", "totally_bogus_action", 3, 4, true, false, true)
		assert.Equal(t, tea.MouseButtonLeft, tea.MouseEvent(msg).Button)
		assert.Equal(t, tea.MouseActionPress, tea.MouseEvent(msg).Action)
		assert.True(t, tea.MouseEvent(msg).Alt)
		assert.True(t, tea.MouseEvent(msg).Shift)
	})

	t.Run("both unknown", func(t *testing.T) {
		t.Parallel()
		msg := JSToMouseEvent("???", "???", 0, 0, false, false, false)
		assert.Equal(t, tea.MouseButtonNone, tea.MouseEvent(msg).Button)
		assert.Equal(t, tea.MouseActionPress, tea.MouseEvent(msg).Action)
	})
}

// ========================================================================
// ValidateLabelInput — unicode printable path
// ========================================================================

func TestValidateLabelInput_UnicodePrintable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected bool
		reason   string
	}{
		{"CJK", "日", true, "unicode printable"},
		{"emoji", "🎉", true, "unicode printable"},
		{"cyrillic", "Д", true, "unicode printable"},
		{"printable ASCII", "A", true, "printable ASCII"},
		{"control char", "\x01", false, "not allowed in label"},
		{"named key arrow", "up", false, "not allowed in label"},
		{"multi-char garbage", "[<65;33;12M", false, "not allowed in label"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := ValidateLabelInput(tc.input, false)
			assert.Equal(t, tc.expected, result.Valid)
			assert.Equal(t, tc.reason, result.Reason)
		})
	}
}

// ========================================================================
// JsToTeaMsg — nil runtime path
// ========================================================================

func TestJsToTeaMsg_NilRuntime(t *testing.T) {
	t.Parallel()
	msg := JsToTeaMsg(nil, nil)
	assert.Nil(t, msg)
}

// ========================================================================
// msgToJS — unknown message type
// ========================================================================

func TestMsgToJS_UnknownMsg(t *testing.T) {
	t.Parallel()
	type customMsg struct{}
	model := &jsModel{}
	result := model.msgToJS(customMsg{})
	assert.Nil(t, result, "unknown msg type should return nil")
}

// ========================================================================
// registerCommand — nil map initialization
// ========================================================================

func TestRegisterCommand_NilMap(t *testing.T) {
	t.Parallel()
	model := &jsModel{}
	// validCmdIDs starts nil; registerCommand should initialize it
	assert.Nil(t, model.validCmdIDs)
	model.registerCommand(42)
	assert.NotNil(t, model.validCmdIDs)
	assert.True(t, model.validCmdIDs[42])
}

// ========================================================================
// Require — JS API edge cases
// ========================================================================

func TestRequire_TickNoArgs(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	manager := newTestManager(context.Background(), vm)
	module := vm.NewObject()
	_ = module.Set("exports", vm.NewObject())
	requireFn := Require(context.Background(), manager)
	requireFn(vm, module)
	exports := module.Get("exports").ToObject(vm)

	// tick() with no args → error
	tickFn, ok := goja.AssertFunction(exports.Get("tick"))
	require.True(t, ok)
	result, err := tickFn(goja.Undefined())
	require.NoError(t, err)
	// Should return an error object
	obj := result.ToObject(vm)
	errVal := obj.Get("error")
	assert.False(t, goja.IsUndefined(errVal), "should have error property")
}

func TestRequire_SetWindowTitleNoArgs(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	manager := newTestManager(context.Background(), vm)
	module := vm.NewObject()
	_ = module.Set("exports", vm.NewObject())
	requireFn := Require(context.Background(), manager)
	requireFn(vm, module)
	exports := module.Get("exports").ToObject(vm)

	// setWindowTitle() with no args → error
	fn, ok := goja.AssertFunction(exports.Get("setWindowTitle"))
	require.True(t, ok)
	result, err := fn(goja.Undefined())
	require.NoError(t, err)
	obj := result.ToObject(vm)
	errVal := obj.Get("error")
	assert.False(t, goja.IsUndefined(errVal), "should have error property")
}

func TestRequire_TickNegativeDuration(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	manager := newTestManager(context.Background(), vm)
	module := vm.NewObject()
	_ = module.Set("exports", vm.NewObject())
	requireFn := Require(context.Background(), manager)
	requireFn(vm, module)
	exports := module.Get("exports").ToObject(vm)

	tickFn, ok := goja.AssertFunction(exports.Get("tick"))
	require.True(t, ok)
	result, err := tickFn(goja.Undefined(), vm.ToValue(-100), vm.ToValue("timer"))
	require.NoError(t, err)
	obj := result.ToObject(vm)
	errVal := obj.Get("error")
	assert.False(t, goja.IsUndefined(errVal), "negative duration should return error")
}

// ========================================================================
// ValidateTextareaInput — additional edge cases
// ========================================================================

func TestValidateTextareaInput_ControlCharNotNamedKey(t *testing.T) {
	t.Parallel()
	// Control character that's not in KeyDefs
	result := ValidateTextareaInput("\x02", false) // ctrl+B raw
	assert.False(t, result.Valid)
	assert.Equal(t, "control character", result.Reason)
}

func TestValidateTextareaInput_UnicodePrintable(t *testing.T) {
	t.Parallel()
	result := ValidateTextareaInput("日", false)
	assert.True(t, result.Valid)
	assert.Equal(t, "unicode printable", result.Reason)
}

// ========================================================================
// Require exports — newModel error paths
// ========================================================================

// requireExports is a helper that sets up the Require module and returns exports.
func requireExports(t *testing.T) (*goja.Runtime, *goja.Object) {
	t.Helper()
	vm := goja.New()
	manager := newTestManager(context.Background(), vm)
	module := vm.NewObject()
	_ = module.Set("exports", vm.NewObject())
	requireFn := Require(context.Background(), manager)
	requireFn(vm, module)
	return vm, module.Get("exports").ToObject(vm)
}

func TestRequire_NewModel_NoArgs(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)
	fn, ok := goja.AssertFunction(exports.Get("newModel"))
	require.True(t, ok)
	result, err := fn(goja.Undefined())
	require.NoError(t, err)
	obj := result.ToObject(vm)
	assert.False(t, goja.IsUndefined(obj.Get("error")), "should have error for no args")
}

func TestRequire_NewModel_NoInit(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)
	fn, ok := goja.AssertFunction(exports.Get("newModel"))
	require.True(t, ok)

	// config with no init function
	config, err := vm.RunString("({update: function(){}, view: function(){return ''}})")
	require.NoError(t, err)
	result, callErr := fn(goja.Undefined(), config)
	require.NoError(t, callErr)
	obj := result.ToObject(vm)
	assert.False(t, goja.IsUndefined(obj.Get("error")), "should error for missing init")
}

func TestRequire_NewModel_NoUpdate(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)
	fn, ok := goja.AssertFunction(exports.Get("newModel"))
	require.True(t, ok)

	config, err := vm.RunString("({init: function(){return [{}]}, view: function(){return ''}})")
	require.NoError(t, err)
	result, callErr := fn(goja.Undefined(), config)
	require.NoError(t, callErr)
	obj := result.ToObject(vm)
	assert.False(t, goja.IsUndefined(obj.Get("error")), "should error for missing update")
}

func TestRequire_NewModel_NoView(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)
	fn, ok := goja.AssertFunction(exports.Get("newModel"))
	require.True(t, ok)

	config, err := vm.RunString("({init: function(){return [{}]}, update: function(){return [{}]}})")
	require.NoError(t, err)
	result, callErr := fn(goja.Undefined(), config)
	require.NoError(t, callErr)
	obj := result.ToObject(vm)
	assert.False(t, goja.IsUndefined(obj.Get("error")), "should error for missing view")
}

func TestRequire_NewModel_ValidConfig(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)
	fn, ok := goja.AssertFunction(exports.Get("newModel"))
	require.True(t, ok)

	config, err := vm.RunString("({init: function(s){return [s||{},null]}, update: function(s,m){return [s,null]}, view: function(s){return 'hello'}})")
	require.NoError(t, err)
	result, callErr := fn(goja.Undefined(), config)
	require.NoError(t, callErr)
	obj := result.ToObject(vm)
	errVal := obj.Get("error")
	assert.True(t, errVal == nil || goja.IsUndefined(errVal), "should not have error for valid config")
	assert.Equal(t, "bubbleteaModel", obj.Get("_type").String())
}

func TestRequire_NewModel_WithRenderThrottle(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)
	fn, ok := goja.AssertFunction(exports.Get("newModel"))
	require.True(t, ok)

	config, err := vm.RunString(`({
		init: function(s){return [s||{},null]},
		update: function(s,m){return [s,null]},
		view: function(s){return 'hello'},
		renderThrottle: {
			enabled: true,
			minIntervalMs: 32,
			alwaysRenderMsgTypes: ["Key", "Tick"]
		}
	})`)
	require.NoError(t, err)
	result, callErr := fn(goja.Undefined(), config)
	require.NoError(t, callErr)
	obj := result.ToObject(vm)
	errVal := obj.Get("error")
	assert.True(t, errVal == nil || goja.IsUndefined(errVal), "should not error with throttle config")
}

// ========================================================================
// Require exports — simple command functions
// ========================================================================

func TestRequire_SimpleCommandExports(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)

	// Test each simple command export returns an object with _cmdType
	cmds := []string{
		"quit", "clearScreen", "hideCursor", "showCursor",
		"enterAltScreen", "exitAltScreen",
		"enableBracketedPaste", "disableBracketedPaste",
		"enableReportFocus", "disableReportFocus",
		"windowSize",
	}

	for _, name := range cmds {
		t.Run(name, func(t *testing.T) {
			// No t.Parallel() — shared vm
			fn, ok := goja.AssertFunction(exports.Get(name))
			require.True(t, ok, "export %q not found", name)
			result, err := fn(goja.Undefined())
			require.NoError(t, err)
			obj := result.ToObject(vm)
			cmdType := obj.Get("_cmdType")
			assert.Equal(t, name, cmdType.String())
		})
	}
}

func TestRequire_BatchAndSequence(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)

	t.Run("batch with args", func(t *testing.T) {
		fn, ok := goja.AssertFunction(exports.Get("batch"))
		require.True(t, ok)
		arg1 := vm.NewObject()
		_ = arg1.Set("_cmdType", "quit")
		result, err := fn(goja.Undefined(), arg1)
		require.NoError(t, err)
		obj := result.ToObject(vm)
		assert.Equal(t, "batch", obj.Get("_cmdType").String())
	})

	t.Run("sequence with args", func(t *testing.T) {
		fn, ok := goja.AssertFunction(exports.Get("sequence"))
		require.True(t, ok)
		arg1 := vm.NewObject()
		_ = arg1.Set("_cmdType", "quit")
		result, err := fn(goja.Undefined(), arg1)
		require.NoError(t, err)
		obj := result.ToObject(vm)
		assert.Equal(t, "sequence", obj.Get("_cmdType").String())
	})
}

func TestRequire_TickWithId(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)
	fn, ok := goja.AssertFunction(exports.Get("tick"))
	require.True(t, ok)
	result, err := fn(goja.Undefined(), vm.ToValue(100), vm.ToValue("timer-1"))
	require.NoError(t, err)
	obj := result.ToObject(vm)
	assert.Equal(t, "tick", obj.Get("_cmdType").String())
	assert.Equal(t, "timer-1", obj.Get("id").String())
}

func TestRequire_SetWindowTitleWithArg(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)
	fn, ok := goja.AssertFunction(exports.Get("setWindowTitle"))
	require.True(t, ok)
	result, err := fn(goja.Undefined(), vm.ToValue("My Title"))
	require.NoError(t, err)
	obj := result.ToObject(vm)
	assert.Equal(t, "setWindowTitle", obj.Get("_cmdType").String())
	assert.Equal(t, "My Title", obj.Get("title").String())
}

func TestRequire_IsTTY(t *testing.T) {
	t.Parallel()
	_, exports := requireExports(t)
	fn, ok := goja.AssertFunction(exports.Get("isTTY"))
	require.True(t, ok)
	result, err := fn(goja.Undefined())
	require.NoError(t, err)
	// In test context, manager was created with nil input/output → NOT TTY
	assert.False(t, result.ToBoolean())
}

// ========================================================================
// Require exports — validation exports
// ========================================================================

func TestRequire_IsValidTextareaInput(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)

	t.Run("with valid key", func(t *testing.T) {
		fn, ok := goja.AssertFunction(exports.Get("isValidTextareaInput"))
		require.True(t, ok)
		result, err := fn(goja.Undefined(), vm.ToValue("a"))
		require.NoError(t, err)
		obj := result.ToObject(vm)
		assert.True(t, obj.Get("valid").ToBoolean())
	})

	t.Run("no args", func(t *testing.T) {
		fn, ok := goja.AssertFunction(exports.Get("isValidTextareaInput"))
		require.True(t, ok)
		result, err := fn(goja.Undefined())
		require.NoError(t, err)
		obj := result.ToObject(vm)
		assert.False(t, obj.Get("valid").ToBoolean())
	})

	t.Run("with isPaste", func(t *testing.T) {
		fn, ok := goja.AssertFunction(exports.Get("isValidTextareaInput"))
		require.True(t, ok)
		result, err := fn(goja.Undefined(), vm.ToValue("a"), vm.ToValue(true))
		require.NoError(t, err)
		obj := result.ToObject(vm)
		assert.True(t, obj.Get("valid").ToBoolean())
	})
}

func TestRequire_IsValidLabelInput(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)

	t.Run("with valid key", func(t *testing.T) {
		fn, ok := goja.AssertFunction(exports.Get("isValidLabelInput"))
		require.True(t, ok)
		result, err := fn(goja.Undefined(), vm.ToValue("A"))
		require.NoError(t, err)
		obj := result.ToObject(vm)
		assert.True(t, obj.Get("valid").ToBoolean())
	})

	t.Run("no args", func(t *testing.T) {
		fn, ok := goja.AssertFunction(exports.Get("isValidLabelInput"))
		require.True(t, ok)
		result, err := fn(goja.Undefined())
		require.NoError(t, err)
		obj := result.ToObject(vm)
		assert.False(t, obj.Get("valid").ToBoolean())
	})
}

// ========================================================================
// Require exports — keys/mouseButtons lookups
// ========================================================================

func TestRequire_KeysExport(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)

	keys := exports.Get("keys").ToObject(vm)
	assert.NotNil(t, keys)
	// Should have at least some keys
	assert.True(t, len(keys.Keys()) > 0, "keys should not be empty")
}

func TestRequire_KeysByNameExport(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)

	keysByName := exports.Get("keysByName").ToObject(vm)
	assert.NotNil(t, keysByName)
	assert.True(t, len(keysByName.Keys()) > 0, "keysByName should not be empty")
}

func TestRequire_MouseButtonsExport(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)

	mouseButtons := exports.Get("mouseButtons").ToObject(vm)
	assert.NotNil(t, mouseButtons)
	assert.True(t, len(mouseButtons.Keys()) > 0, "mouseButtons should not be empty")
}

func TestRequire_MouseActionsExport(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)

	mouseActions := exports.Get("mouseActions").ToObject(vm)
	assert.NotNil(t, mouseActions)
	assert.True(t, len(mouseActions.Keys()) > 0, "mouseActions should not be empty")
}

// ========================================================================
// Require — run without valid model
// ========================================================================

func TestRequire_Run_NoArgs(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)
	fn, ok := goja.AssertFunction(exports.Get("run"))
	require.True(t, ok)
	result, err := fn(goja.Undefined())
	require.NoError(t, err)
	obj := result.ToObject(vm)
	assert.False(t, goja.IsUndefined(obj.Get("error")), "should error with no args")
}

func TestRequire_Run_InvalidModelID(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)
	fn, ok := goja.AssertFunction(exports.Get("run"))
	require.True(t, ok)
	// Pass a model wrapper with a bogus ID
	bogus := vm.NewObject()
	_ = bogus.Set("_modelID", 99999)
	_ = bogus.Set("_type", "bubbleteaModel")
	result, err := fn(goja.Undefined(), bogus)
	require.NoError(t, err)
	obj := result.ToObject(vm)
	assert.False(t, goja.IsUndefined(obj.Get("error")), "should error for invalid model ID")
}

// ========================================================================
// Init/Update/View — RunJSSync error paths
// ========================================================================

func TestInit_RunJSSyncError(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	model := &jsModel{
		runtime: vm,
		initFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			return vm.NewArray(vm.NewObject(), goja.Null()), nil
		},
		state:    vm.NewObject(),
		jsRunner: &errorJSRunner{},
	}
	cmd := model.Init()
	assert.Nil(t, cmd)
	assert.Contains(t, model.initError, "event loop")
}

func TestUpdate_RunJSSyncError(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	model := &jsModel{
		runtime: vm,
		updateFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			return vm.NewArray(vm.NewObject(), goja.Null()), nil
		},
		state:    vm.NewObject(),
		jsRunner: &errorJSRunner{},
	}
	retModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	assert.Equal(t, model, retModel)
	assert.Nil(t, cmd)
}

func TestView_RunJSSyncError(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	model := &jsModel{
		runtime: vm,
		viewFn: createViewFn(vm, func(state goja.Value) string {
			return "ok"
		}),
		state:    vm.NewObject(),
		jsRunner: &errorJSRunner{},
	}
	output := model.View()
	assert.Contains(t, output, "View error (event loop)")
}

// ========================================================================
// viewDirect — error and empty string paths
// ========================================================================

func TestViewDirect_Error(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	model := &jsModel{
		runtime: vm,
		viewFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			return nil, errors.New("view failed")
		},
		state: vm.NewObject(),
	}
	output := model.viewDirect()
	assert.Contains(t, output, "View error: view failed")
}

func TestViewDirect_EmptyString(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	model := &jsModel{
		runtime: vm,
		viewFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			return vm.ToValue(""), nil
		},
		state: vm.NewObject(),
	}
	output := model.viewDirect()
	assert.Equal(t, "[BT] View returned empty string", output)
}

// ========================================================================
// View — throttle timer scheduling with program + throttleCtx
// ========================================================================

func TestView_ThrottleSchedulesTimer(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	model := &jsModel{
		runtime:            vm,
		throttleEnabled:    true,
		throttleIntervalMs: 10000,
		viewFn: createViewFn(vm, func(state goja.Value) string {
			return "view"
		}),
		state:    vm.NewObject(),
		jsRunner: &SyncJSRunner{Runtime: vm},
	}

	// First render to prime the cache
	model.View()

	// Set up program and throttleCtx (simulating real program state)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	model.throttleCtx = ctx
	model.program = tea.NewProgram(noopModel{})
	defer func() { model.program = nil }()

	// Second render should schedule timer
	model.throttleTimerSet = false
	output2 := model.View()
	assert.Equal(t, "view", output2, "should return cached view")
	assert.True(t, model.throttleTimerSet, "should have scheduled timer")

	// Cancel to prevent goroutine leak
	cancel()
}

func TestView_ThrottleSchedulesCancelledCtx(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	model := &jsModel{
		runtime:            vm,
		throttleEnabled:    true,
		throttleIntervalMs: 500, // Long enough interval for Windows timer resolution
		viewFn: createViewFn(vm, func(state goja.Value) string {
			return "view"
		}),
		state:    vm.NewObject(),
		jsRunner: &SyncJSRunner{Runtime: vm},
	}

	// First render
	model.View()

	// Set up already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	model.throttleCtx = ctx
	model.program = tea.NewProgram(noopModel{})
	defer func() { model.program = nil }()

	// Render within throttle interval — timer goroutine should exit immediately via ctx.Done()
	model.throttleTimerSet = false
	model.View()
	assert.True(t, model.throttleTimerSet, "timer flag should still be set")

	// Give goroutine time to exit cleanly
	time.Sleep(10 * time.Millisecond)
}

// ========================================================================
// Update — msgToJS returns nil for renderRefreshMsg paths
// ========================================================================

func TestUpdate_NilUpdateFnWithRuntime(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	model := &jsModel{
		runtime:  vm,
		updateFn: nil,
	}
	retModel, cmd := model.Update(tea.KeyMsg{})
	assert.Equal(t, model, retModel)
	assert.Nil(t, cmd)
}

func TestUpdate_NilRuntime(t *testing.T) {
	t.Parallel()
	model := &jsModel{
		runtime: nil,
		updateFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			return nil, nil
		},
	}
	retModel, cmd := model.Update(tea.KeyMsg{})
	assert.Equal(t, model, retModel)
	assert.Nil(t, cmd)
}

// ========================================================================
// Update — unknown message type → msgToJS returns nil → early return
// ========================================================================

func TestUpdate_UnknownMessageType(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	type unknownMsg struct{}
	model := &jsModel{
		runtime: vm,
		updateFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			panic("should not be called for unknown msg")
		},
		state:    vm.NewObject(),
		jsRunner: &SyncJSRunner{Runtime: vm},
	}
	retModel, cmd := model.Update(unknownMsg{})
	assert.Equal(t, model, retModel)
	assert.Nil(t, cmd)
}

// ========================================================================
// runProgram — error guard paths
// ========================================================================

func TestRunProgram_NilManager(t *testing.T) {
	t.Parallel()
	var m *Manager
	err := m.runProgram(noopModel{})
	assert.ErrorContains(t, err, "manager is nil")
}

func TestRunProgram_NilCtx(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	m := &Manager{
		jsRunner: &SyncJSRunner{Runtime: vm},
		// ctx is nil
	}
	err := m.runProgram(noopModel{})
	assert.ErrorContains(t, err, "manager.ctx is nil")
}

func TestRunProgram_AlreadyRunningGuard(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	m := newTestManager(context.Background(), vm)
	// Set program to non-nil to simulate "already running"
	m.program = tea.NewProgram(noopModel{})
	err := m.runProgram(noopModel{})
	assert.ErrorContains(t, err, "already running")
	m.program = nil // cleanup
}

// ========================================================================
// Require run — additional error paths
// ========================================================================

func TestRequire_Run_NullModel(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)
	fn, ok := goja.AssertFunction(exports.Get("run"))
	require.True(t, ok)
	result, err := fn(goja.Undefined(), goja.Null())
	require.NoError(t, err)
	obj := result.ToObject(vm)
	errVal := obj.Get("error")
	assert.False(t, errVal == nil || goja.IsUndefined(errVal), "should error for null model")
}

func TestRequire_Run_MissingType(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)
	fn, ok := goja.AssertFunction(exports.Get("run"))
	require.True(t, ok)
	// Object without _type
	model := vm.NewObject()
	result, err := fn(goja.Undefined(), model)
	require.NoError(t, err)
	obj := result.ToObject(vm)
	errVal := obj.Get("error")
	assert.False(t, errVal == nil || goja.IsUndefined(errVal), "should error for missing _type")
}

func TestRequire_Run_WrongType(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)
	fn, ok := goja.AssertFunction(exports.Get("run"))
	require.True(t, ok)
	model := vm.NewObject()
	_ = model.Set("_type", "notBubbleteaModel")
	result, err := fn(goja.Undefined(), model)
	require.NoError(t, err)
	obj := result.ToObject(vm)
	errVal := obj.Get("error")
	assert.False(t, errVal == nil || goja.IsUndefined(errVal), "should error for wrong _type")
}

func TestRequire_Run_MissingModelID(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)
	fn, ok := goja.AssertFunction(exports.Get("run"))
	require.True(t, ok)
	model := vm.NewObject()
	_ = model.Set("_type", "bubbleteaModel")
	// no _modelID set
	result, err := fn(goja.Undefined(), model)
	require.NoError(t, err)
	obj := result.ToObject(vm)
	errVal := obj.Get("error")
	assert.False(t, errVal == nil || goja.IsUndefined(errVal), "should error for missing _modelID")
}

// ========================================================================
// msgToJS — coverage for specific message types
// ========================================================================

func TestMsgToJS_AllMessageTypes(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	model := &jsModel{runtime: vm}

	t.Run("quitMsg", func(t *testing.T) {
		result := model.msgToJS(quitMsg{})
		assert.NotNil(t, result)
		assert.Equal(t, "Quit", result["type"])
	})

	t.Run("clearScreenMsg", func(t *testing.T) {
		result := model.msgToJS(clearScreenMsg{})
		assert.NotNil(t, result)
		assert.Equal(t, "ClearScreen", result["type"])
	})

	t.Run("stateRefreshMsg", func(t *testing.T) {
		result := model.msgToJS(stateRefreshMsg{key: "testKey"})
		assert.NotNil(t, result)
		assert.Equal(t, "StateRefresh", result["type"])
		assert.Equal(t, "testKey", result["key"])
	})

	t.Run("renderRefreshMsg_nil", func(t *testing.T) {
		result := model.msgToJS(renderRefreshMsg{})
		assert.Nil(t, result, "renderRefreshMsg should return nil")
	})

	t.Run("tickMsg", func(t *testing.T) {
		now := time.Now()
		result := model.msgToJS(tickMsg{id: "timer-1", time: now})
		assert.NotNil(t, result)
		assert.Equal(t, "Tick", result["type"])
		assert.Equal(t, "timer-1", result["id"])
	})

	t.Run("FocusMsg", func(t *testing.T) {
		result := model.msgToJS(tea.FocusMsg{})
		assert.NotNil(t, result)
		assert.Equal(t, "Focus", result["type"])
	})

	t.Run("BlurMsg", func(t *testing.T) {
		result := model.msgToJS(tea.BlurMsg{})
		assert.NotNil(t, result)
		assert.Equal(t, "Blur", result["type"])
	})

	t.Run("WindowSizeMsg", func(t *testing.T) {
		result := model.msgToJS(tea.WindowSizeMsg{Width: 80, Height: 24})
		assert.NotNil(t, result)
		assert.Equal(t, "WindowSize", result["type"])
		assert.Equal(t, 80, result["width"])
		assert.Equal(t, 24, result["height"])
	})
}

// ========================================================================
// SendStateRefresh — nil program path
// ========================================================================

func TestSendStateRefresh_NilProgram(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	m := newTestManager(context.Background(), vm)
	// program is nil by default, should be a no-op
	assert.NotPanics(t, func() {
		m.SendStateRefresh("test")
	})
}

// ========================================================================
// SetJSRunner — panic on nil
// ========================================================================

func TestSetJSRunner_Panics(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	m := newTestManager(context.Background(), vm)
	assert.Panics(t, func() {
		m.SetJSRunner(nil)
	})
}

// ========================================================================
// newModel — nil config edge case
// ========================================================================

func TestRequire_NewModel_NilConfig(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)
	fn, ok := goja.AssertFunction(exports.Get("newModel"))
	require.True(t, ok)

	// Pass undefined as config → should return error
	result, err := fn(goja.Undefined(), goja.Undefined())
	require.NoError(t, err)
	obj := result.ToObject(vm)
	errVal := obj.Get("error")
	assert.False(t, errVal == nil || goja.IsUndefined(errVal), "should error for undefined config")

	// Pass null as config → should also return error
	result2, err2 := fn(goja.Undefined(), goja.Null())
	require.NoError(t, err2)
	obj2 := result2.ToObject(vm)
	errVal2 := obj2.Get("error")
	assert.False(t, errVal2 == nil || goja.IsUndefined(errVal2), "should error for null config")
}
