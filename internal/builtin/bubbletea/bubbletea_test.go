package bubbletea

import (
	"context"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestManager creates a Manager with a SyncJSRunner for unit testing.
// This is appropriate for tests that don't run actual BubbleTea programs.
func newTestManager(ctx context.Context, vm *goja.Runtime) *Manager {
	return NewManager(ctx, nil, nil, &SyncJSRunner{Runtime: vm}, nil, nil)
}

// newTestManagerWithIO creates a Manager with custom I/O and a SyncJSRunner for unit testing.
func newTestManagerWithIO(ctx context.Context, input, output *os.File, vm *goja.Runtime) *Manager {
	return NewManager(ctx, input, output, &SyncJSRunner{Runtime: vm}, nil, nil)
}

func TestNewManager(t *testing.T) {
	vm := goja.New()
	manager := newTestManager(context.Background(), vm)
	assert.NotNil(t, manager)
	assert.NotNil(t, manager.input)
	assert.NotNil(t, manager.output)
	assert.NotNil(t, manager.signalNotify)
	assert.NotNil(t, manager.signalStop)
}

func TestNewManager_WithCustomIO(t *testing.T) {
	vm := goja.New()
	input := os.Stdin
	output := os.Stdout
	manager := newTestManagerWithIO(context.Background(), input, output, vm)
	assert.NotNil(t, manager)
	assert.Equal(t, input, manager.input)
	assert.Equal(t, output, manager.output)
}

func TestRequire_ExportsCorrectAPI(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)

	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	// Call the require function
	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	// Verify exports
	exports := module.Get("exports").ToObject(vm)
	require.NotNil(t, exports)

	// Check functions are exported
	for _, fn := range []string{
		"newModel",
		"run",
		"quit",
		"clearScreen",
		"batch",
		"sequence",
	} {
		val := exports.Get(fn)
		assert.False(t, goja.IsUndefined(val), "Function %s should be exported", fn)
		assert.False(t, goja.IsNull(val), "Function %s should not be null", fn)
		_, ok := goja.AssertFunction(val)
		assert.True(t, ok, "Export %s should be a function", fn)
	}

	// Check that tea.keys and tea.keysByName are exported
	keysVal := exports.Get("keys")
	assert.False(t, goja.IsUndefined(keysVal), "keys should be exported")
	assert.False(t, goja.IsNull(keysVal), "keys should not be null")

	keysByNameVal := exports.Get("keysByName")
	assert.False(t, goja.IsUndefined(keysByNameVal), "keysByName should be exported")
	assert.False(t, goja.IsNull(keysByNameVal), "keysByName should not be null")
}

func TestTeaKeys_ContainsCoreKeys(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	exports := module.Get("exports").ToObject(vm)
	keysObj := exports.Get("keys").ToObject(vm)

	// Verify core keys are present via JS access
	coreKeys := []string{"enter", "esc", "backspace", "tab", "up", "down", "left", "right"}
	for _, key := range coreKeys {
		val := keysObj.Get(key)
		assert.False(t, goja.IsUndefined(val), "keys[%q] should be defined", key)
		if !goja.IsUndefined(val) {
			keyDef := val.ToObject(vm)
			name := keyDef.Get("name")
			str := keyDef.Get("string")
			assert.False(t, goja.IsUndefined(name), "keys[%q].name should be defined", key)
			assert.False(t, goja.IsUndefined(str), "keys[%q].string should be defined", key)
			assert.Equal(t, key, str.String(), "keys[%q].string should equal the key", key)
		}
	}
}

func TestTeaKeysByName_ContainsCoreKeys(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	exports := module.Get("exports").ToObject(vm)
	keysByNameObj := exports.Get("keysByName").ToObject(vm)

	// Verify core keys are present by Go constant name
	// Note: Use actual constant names from keys_gen.go, not intuitive names
	// - "KeyEsc" not "KeyEscape"
	// - "KeyCtrlQuestionMark" represents backspace
	coreKeyNames := []string{"KeyEnter", "KeyEsc", "KeyUp", "KeyDown", "KeyLeft", "KeyRight"}
	for _, name := range coreKeyNames {
		val := keysByNameObj.Get(name)
		assert.False(t, goja.IsUndefined(val), "keysByName[%q] should be defined", name)
		if !goja.IsUndefined(val) && !goja.IsNull(val) {
			keyDef := val.ToObject(vm)
			if keyDef != nil {
				keyName := keyDef.Get("name")
				if !goja.IsUndefined(keyName) && !goja.IsNull(keyName) {
					assert.Equal(t, name, keyName.String(), "keysByName[%q].name should match", name)
				}
			}
		}
	}
}

func TestNewModel_RequiresConfig(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	exports := module.Get("exports").ToObject(vm)
	newModelFn, _ := goja.AssertFunction(exports.Get("newModel"))

	// Test without arguments
	result, err := newModelFn(goja.Undefined())
	require.NoError(t, err)
	resultObj := result.ToObject(vm)
	assert.Contains(t, resultObj.Get("error").String(), "requires a config object")
}

func TestNewModel_RequiresInitFunction(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	result, err := vm.RunString(`
		tea.newModel({
			update: function(msg, model) { return [model, null]; },
			view: function(model) { return ''; }
		});
	`)
	require.NoError(t, err)
	resultObj := result.ToObject(vm)
	assert.Contains(t, resultObj.Get("error").String(), "init must be a function")
}

func TestNewModel_RequiresUpdateFunction(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	result, err := vm.RunString(`
		tea.newModel({
			init: function() { return {}; },
			view: function(model) { return ''; }
		});
	`)
	require.NoError(t, err)
	resultObj := result.ToObject(vm)
	assert.Contains(t, resultObj.Get("error").String(), "update must be a function")
}

func TestNewModel_RequiresViewFunction(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	result, err := vm.RunString(`
		tea.newModel({
			init: function() { return {}; },
			update: function(msg, model) { return [model, null]; }
		});
	`)
	require.NoError(t, err)
	resultObj := result.ToObject(vm)
	assert.Contains(t, resultObj.Get("error").String(), "view must be a function")
}

func TestNewModel_ValidConfig(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	result, err := vm.RunString(`
		tea.newModel({
			init: function() { return { count: 0 }; },
			update: function(msg, model) { return [model, null]; },
			view: function(model) { return 'Count: ' + model.count; }
		});
	`)
	require.NoError(t, err)

	resultObj := result.ToObject(vm)
	// Check if there's an error
	errorVal := resultObj.Get("error")
	hasError := errorVal != nil && !goja.IsUndefined(errorVal) && !goja.IsNull(errorVal)
	if hasError {
		t.Logf("Got error: %v", errorVal.String())
	}
	// Should not have error
	assert.False(t, hasError, "Should not have error")
	// Should have _type marker
	typeVal := resultObj.Get("_type")
	assert.False(t, goja.IsUndefined(typeVal), "_type should be defined")
	if !goja.IsUndefined(typeVal) {
		assert.Equal(t, "bubbleteaModel", typeVal.String())
	}
}

func TestQuit_ReturnsCommand(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	result, err := vm.RunString(`tea.quit()`)
	require.NoError(t, err)

	resultObj := result.ToObject(vm)
	assert.Equal(t, "quit", resultObj.Get("_cmdType").String())
}

func TestClearScreen_ReturnsCommand(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	result, err := vm.RunString(`tea.clearScreen()`)
	require.NoError(t, err)

	resultObj := result.ToObject(vm)
	assert.Equal(t, "clearScreen", resultObj.Get("_cmdType").String())
}

func TestBatch_ReturnsCommand(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	result, err := vm.RunString(`tea.batch(tea.quit(), tea.clearScreen())`)
	require.NoError(t, err)

	resultObj := result.ToObject(vm)
	assert.Equal(t, "batch", resultObj.Get("_cmdType").String())
	cmds := resultObj.Get("cmds").ToObject(vm)
	assert.Equal(t, int64(2), cmds.Get("length").ToInteger())
}

func TestSequence_ReturnsCommand(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	result, err := vm.RunString(`tea.sequence(tea.quit(), tea.clearScreen())`)
	require.NoError(t, err)

	resultObj := result.ToObject(vm)
	assert.Equal(t, "sequence", resultObj.Get("_cmdType").String())
	cmds := resultObj.Get("cmds").ToObject(vm)
	assert.Equal(t, int64(2), cmds.Get("length").ToInteger())
}

func TestRun_RequiresModel(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	result, err := vm.RunString(`tea.run()`)
	require.NoError(t, err)

	resultObj := result.ToObject(vm)
	assert.Contains(t, resultObj.Get("error").String(), "run requires a model")
}

func TestRun_RequiresValidModel(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	result, err := vm.RunString(`tea.run({})`)
	require.NoError(t, err)

	resultObj := result.ToObject(vm)
	assert.Contains(t, resultObj.Get("error").String(), "invalid model object")
}

func TestJsModel_Init(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	// Create a model with a simple init
	result, err := vm.RunString(`
		tea.newModel({
			init: function() { return { initialized: true }; },
			update: function(msg, model) { return [model, null]; },
			view: function(model) { return JSON.stringify(model); }
		});
	`)
	require.NoError(t, err)

	modelWrapper := result.ToObject(vm)
	getModelFn, ok := goja.AssertFunction(modelWrapper.Get("_getModel"))
	require.True(t, ok, "_getModel should be a function")
	modelVal, err := getModelFn(goja.Undefined())
	require.NoError(t, err)
	model, ok := modelVal.Export().(*jsModel)
	require.True(t, ok)

	// Test Init
	cmd := model.Init()
	assert.Nil(t, cmd) // Init should return nil cmd by default

	// Check state was initialized
	stateObj := model.state.ToObject(vm)
	assert.True(t, stateObj.Get("initialized").ToBoolean())
}

func TestJsModel_View(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	result, err := vm.RunString(`
		tea.newModel({
			init: function() { return { message: 'Hello World' }; },
			update: function(msg, model) { return [model, null]; },
			view: function(model) { return 'Message: ' + model.message; }
		});
	`)
	require.NoError(t, err)

	modelWrapper := result.ToObject(vm)
	getModelFn, ok := goja.AssertFunction(modelWrapper.Get("_getModel"))
	require.True(t, ok, "_getModel should be a function")
	modelVal, err := getModelFn(goja.Undefined())
	require.NoError(t, err)
	model, ok := modelVal.Export().(*jsModel)
	require.True(t, ok)

	// Initialize
	model.Init()

	// Test View
	view := model.View()
	assert.Equal(t, "Message: Hello World", view)
}

func TestJsModel_Update_QuitCommand(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	// Create a model that returns tea.quit() when 'q' is pressed
	result, err := vm.RunString(`
		tea.newModel({
			init: function() { return { count: 0 }; },
			update: function(msg, model) {
				if (msg.type === 'Key' && msg.key === 'q') {
					return [model, tea.quit()];
				}
				return [model, null];
			},
			view: function(model) { return 'Count: ' + model.count; }
		});
	`)
	require.NoError(t, err)

	modelWrapper := result.ToObject(vm)
	getModelFn, ok := goja.AssertFunction(modelWrapper.Get("_getModel"))
	require.True(t, ok, "_getModel should be a function")
	modelVal, err := getModelFn(goja.Undefined())
	require.NoError(t, err)
	model, ok := modelVal.Export().(*jsModel)
	require.True(t, ok)

	// Initialize
	model.Init()

	// Simulate 'q' key press
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	_, cmd := model.Update(keyMsg)

	// Verify that cmd is tea.Quit
	// tea.Quit is a function that returns tea.QuitMsg when called
	require.NotNil(t, cmd, "cmd should not be nil when returning tea.quit()")

	// Execute the command to get the message
	quitMsg := cmd()
	_, isQuitMsg := quitMsg.(tea.QuitMsg)
	assert.True(t, isQuitMsg, "cmd() should return tea.QuitMsg, got %T", quitMsg)
}

func TestBatchWithNoArgs(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	result, err := vm.RunString(`tea.batch()`)
	require.NoError(t, err)

	resultObj := result.ToObject(vm)
	assert.Equal(t, "batch", resultObj.Get("_cmdType").String())
}

func TestSequenceWithNoArgs(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	result, err := vm.RunString(`tea.sequence()`)
	require.NoError(t, err)

	resultObj := result.ToObject(vm)
	assert.Equal(t, "sequence", resultObj.Get("_cmdType").String())
}

// ============================================================================
// NEW COMMAND TESTS
// ============================================================================

func TestTickCommand_Valid(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	result, err := vm.RunString(`tea.tick(1000, 'timer1')`)
	require.NoError(t, err)

	resultObj := result.ToObject(vm)
	assert.Equal(t, "tick", resultObj.Get("_cmdType").String())
	assert.Equal(t, int64(1000), resultObj.Get("duration").ToInteger())
	assert.Equal(t, "timer1", resultObj.Get("id").String())
}

func TestTickCommand_InvalidDuration(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	result, err := vm.RunString(`tea.tick(-100, 'timer')`)
	require.NoError(t, err)

	resultObj := result.ToObject(vm)
	assert.Contains(t, resultObj.Get("error").String(), "BT001")
}

func TestSetWindowTitle(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	result, err := vm.RunString(`tea.setWindowTitle('My App')`)
	require.NoError(t, err)

	resultObj := result.ToObject(vm)
	assert.Equal(t, "setWindowTitle", resultObj.Get("_cmdType").String())
	assert.Equal(t, "My App", resultObj.Get("title").String())
}

func TestCursorCommands(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	// Test hideCursor
	result, err := vm.RunString(`tea.hideCursor()`)
	require.NoError(t, err)
	resultObj := result.ToObject(vm)
	assert.Equal(t, "hideCursor", resultObj.Get("_cmdType").String())

	// Test showCursor
	result, err = vm.RunString(`tea.showCursor()`)
	require.NoError(t, err)
	resultObj = result.ToObject(vm)
	assert.Equal(t, "showCursor", resultObj.Get("_cmdType").String())
}

func TestAltScreenCommands(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	// Test enterAltScreen
	result, err := vm.RunString(`tea.enterAltScreen()`)
	require.NoError(t, err)
	resultObj := result.ToObject(vm)
	assert.Equal(t, "enterAltScreen", resultObj.Get("_cmdType").String())

	// Test exitAltScreen
	result, err = vm.RunString(`tea.exitAltScreen()`)
	require.NoError(t, err)
	resultObj = result.ToObject(vm)
	assert.Equal(t, "exitAltScreen", resultObj.Get("_cmdType").String())
}

func TestBracketedPasteCommands(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	// Test enableBracketedPaste
	result, err := vm.RunString(`tea.enableBracketedPaste()`)
	require.NoError(t, err)
	resultObj := result.ToObject(vm)
	assert.Equal(t, "enableBracketedPaste", resultObj.Get("_cmdType").String())

	// Test disableBracketedPaste
	result, err = vm.RunString(`tea.disableBracketedPaste()`)
	require.NoError(t, err)
	resultObj = result.ToObject(vm)
	assert.Equal(t, "disableBracketedPaste", resultObj.Get("_cmdType").String())
}

func TestReportFocusCommands(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	// Test enableReportFocus
	result, err := vm.RunString(`tea.enableReportFocus()`)
	require.NoError(t, err)
	resultObj := result.ToObject(vm)
	assert.Equal(t, "enableReportFocus", resultObj.Get("_cmdType").String())

	// Test disableReportFocus
	result, err = vm.RunString(`tea.disableReportFocus()`)
	require.NoError(t, err)
	resultObj = result.ToObject(vm)
	assert.Equal(t, "disableReportFocus", resultObj.Get("_cmdType").String())
}

func TestWindowSizeCommand(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	result, err := vm.RunString(`tea.windowSize()`)
	require.NoError(t, err)

	resultObj := result.ToObject(vm)
	assert.Equal(t, "windowSize", resultObj.Get("_cmdType").String())
}

func TestIsTTY(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	// Just verify the function exists and returns a boolean
	result, err := vm.RunString(`typeof tea.isTTY()`)
	require.NoError(t, err)
	assert.Equal(t, "boolean", result.String())
}

func TestRequire_AllNewFunctionsExported(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	exports := module.Get("exports").ToObject(vm)
	require.NotNil(t, exports)

	// Check all new functions are exported
	newFunctions := []string{
		"tick",
		"setWindowTitle",
		"hideCursor",
		"showCursor",
		"enterAltScreen",
		"exitAltScreen",
		"enableBracketedPaste",
		"disableBracketedPaste",
		"enableReportFocus",
		"disableReportFocus",
		"windowSize",
		"isTTY",
	}

	for _, fn := range newFunctions {
		val := exports.Get(fn)
		assert.False(t, goja.IsUndefined(val), "Function %s should be exported", fn)
		assert.False(t, goja.IsNull(val), "Function %s should not be null", fn)
		_, ok := goja.AssertFunction(val)
		assert.True(t, ok, "Export %s should be a function", fn)
	}
}

func TestCommandHasID(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	// All commands should have unique _cmdID
	result, err := vm.RunString(`
		const cmd1 = tea.quit();
		const cmd2 = tea.quit();
		const id1 = cmd1._cmdID;
		const id2 = cmd2._cmdID;
		JSON.stringify({
			hasID1: typeof id1 === 'number',
			hasID2: typeof id2 === 'number',
			unique: id1 !== id2
		});
	`)
	require.NoError(t, err)

	assert.Contains(t, result.String(), `"hasID1":true`)
	assert.Contains(t, result.String(), `"hasID2":true`)
	assert.Contains(t, result.String(), `"unique":true`)
}

func TestWrapCmd_NilCommand(t *testing.T) {
	runtime := goja.New()
	val := WrapCmd(runtime, nil)
	assert.True(t, goja.IsNull(val), "WrapCmd(nil) should return goja.Null()")
}

func TestWrapCmd_ActualCommand(t *testing.T) {
	runtime := goja.New()
	val := WrapCmd(runtime, tea.Quit)
	assert.False(t, goja.IsNull(val))

	exported := val.Export()
	reqCmd, ok := exported.(tea.Cmd)
	assert.True(t, ok, "Exported value should be a tea.Cmd")
	// Execute the command to ensure it behaves like tea.Quit
	res := reqCmd()
	_, isQuit := res.(tea.QuitMsg)
	assert.True(t, isQuit, "Wrapped command should execute to a QuitMsg")
}

func TestWrapCmd_ClosureCommand(t *testing.T) {
	runtime := goja.New()
	type uniqueTestMsg struct{ id int }
	closureCmd := func() tea.Msg { return uniqueTestMsg{id: 42} }
	val := WrapCmd(runtime, closureCmd)
	assert.False(t, goja.IsNull(val))

	exported := val.Export()
	cmdFn, ok := exported.(tea.Cmd)
	assert.True(t, ok, "Exported closure should be a tea.Cmd")
	res := cmdFn()
	ut, ok := res.(uniqueTestMsg)
	assert.True(t, ok, "closure cmd should return uniqueTestMsg")
	assert.Equal(t, 42, ut.id)
}

func TestWrapCmd_CursorBlinkCommand(t *testing.T) {
	runtime := goja.New()
	// cursor.Blink() returns a tea.Msg; wrap it in a tea.Cmd closure
	blinkCmd := func() tea.Msg { return cursor.Blink() }
	val := WrapCmd(runtime, blinkCmd)
	assert.False(t, goja.IsNull(val))

	exported := val.Export()
	cmd2, ok := exported.(tea.Cmd)
	assert.True(t, ok, "Exported cursor.Blink should be a tea.Cmd")
	res := cmd2()
	assert.NotNil(t, res, "cursor.Blink() when executed should return a non-nil message")
}

// ----------------------------
// Phase 2: ValueToCmd & Roundtrip Tests
// ----------------------------

func TestValueToCmd_WrappedCommand(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	// Create a model to obtain a jsModel with runtime
	result, err := vm.RunString(`tea.newModel({ init: function() { return {}; }, update: function(msg, model) { return [model, null]; }, view: function(model) { return ''; } });`)
	require.NoError(t, err)

	modelWrapper := result.ToObject(vm)
	getModelFn, ok := goja.AssertFunction(modelWrapper.Get("_getModel"))
	require.True(t, ok, "_getModel should be a function")
	modelVal, err := getModelFn(goja.Undefined())
	require.NoError(t, err)
	m, ok := modelVal.Export().(*jsModel)
	require.True(t, ok)

	// Wrap tea.Quit using the model's runtime
	wrapped := WrapCmd(m.runtime, tea.Quit)
	cmd := m.valueToCmd(wrapped)
	require.NotNil(t, cmd)
	res := cmd()
	_, isQuit := res.(tea.QuitMsg)
	assert.True(t, isQuit, "valueToCmd should extract wrapped tea.Quit")
}

func TestValueToCmd_DescriptorQuit(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	// Create model
	result, err := vm.RunString(`tea.newModel({ init: function() { return {}; }, update: function(msg, model) { return [model, null]; }, view: function(model) { return ''; } });`)
	require.NoError(t, err)

	modelWrapper := result.ToObject(vm)
	getModelFn, ok := goja.AssertFunction(modelWrapper.Get("_getModel"))
	require.True(t, ok, "_getModel should be a function")
	modelVal, err := getModelFn(goja.Undefined())
	require.NoError(t, err)
	m, ok := modelVal.Export().(*jsModel)
	require.True(t, ok)

	// Use tea.quit() descriptor
	val, err := vm.RunString(`tea.quit()`)
	require.NoError(t, err)

	cmd := m.valueToCmd(val)
	require.NotNil(t, cmd)
	res := cmd()
	_, isQuit := res.(tea.QuitMsg)
	assert.True(t, isQuit, "descriptor quit should extract to tea.Quit")
}

func TestValueToCmd_DescriptorBatch(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	// Create model
	result, err := vm.RunString(`tea.newModel({ init: function() { return {}; }, update: function(msg, model) { return [model, null]; }, view: function(model) { return ''; } });`)
	require.NoError(t, err)

	modelWrapper := result.ToObject(vm)
	getModelFn, ok := goja.AssertFunction(modelWrapper.Get("_getModel"))
	require.True(t, ok, "_getModel should be a function")
	modelVal, err := getModelFn(goja.Undefined())
	require.NoError(t, err)
	m, ok := modelVal.Export().(*jsModel)
	require.True(t, ok)

	// Descriptor batch containing wrapped Go commands - verify both are executed
	calls := make([]int, 0)
	var callsMu sync.Mutex
	c1 := func() tea.Msg { callsMu.Lock(); defer callsMu.Unlock(); calls = append(calls, 1); return nil }
	c2 := func() tea.Msg { callsMu.Lock(); defer callsMu.Unlock(); calls = append(calls, 2); return nil }
	g1 := WrapCmd(m.runtime, c1)
	g2 := WrapCmd(m.runtime, c2)
	_ = vm.Set("g1", g1)
	_ = vm.Set("g2", g2)
	val, err := vm.RunString(`({ _cmdType: 'batch', _cmdID: 1, cmds: [ g1, g2 ] })`)
	require.NoError(t, err)

	// Confirm individual elements extract correctly
	first, err := vm.RunString(`({ _cmdType: 'batch', _cmdID: 1, cmds: [ g1, g2 ] }).cmds[0]`)
	require.NoError(t, err)
	second, err := vm.RunString(`({ _cmdType: 'batch', _cmdID: 1, cmds: [ g1, g2 ] }).cmds[1]`)
	require.NoError(t, err)
	cmd1 := m.valueToCmd(first)
	require.NotNil(t, cmd1, "first batch element should convert to a non-nil cmd")
	cmd2 := m.valueToCmd(second)
	require.NotNil(t, cmd2, "second batch element should convert to a non-nil cmd")

	cmd := m.valueToCmd(val)
	require.NotNil(t, cmd)
	// Also ensure individual extracted cmds execute correctly
	cmd1()
	cmd2()
	// Execute and wait briefly for both to run (Batch may run sub-commands concurrently)
	cmd()
	// Wait for a short period for both commands to have a chance to execute
	time.Sleep(20 * time.Millisecond)
	callsMu.Lock()
	defer callsMu.Unlock()
	require.Equal(t, 2, len(calls), "both batch commands should have executed")
	require.Equal(t, 1, calls[0])
	require.Equal(t, 2, calls[1])
}

func TestValueToCmd_DescriptorSequence(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	// Create model
	result, err := vm.RunString(`tea.newModel({ init: function() { return {}; }, update: function(msg, model) { return [model, null]; }, view: function(model) { return ''; } });`)
	require.NoError(t, err)

	modelWrapper := result.ToObject(vm)
	getModelFn, ok := goja.AssertFunction(modelWrapper.Get("_getModel"))
	require.True(t, ok, "_getModel should be a function")
	modelVal, err := getModelFn(goja.Undefined())
	require.NoError(t, err)
	m, ok := modelVal.Export().(*jsModel)
	require.True(t, ok)

	calls := make([]int, 0)
	var callsMu sync.Mutex
	c1 := func() tea.Msg { callsMu.Lock(); calls = append(calls, 1); callsMu.Unlock(); return nil }
	c2 := func() tea.Msg { callsMu.Lock(); calls = append(calls, 2); callsMu.Unlock(); return nil }
	g1 := WrapCmd(m.runtime, c1)
	g2 := WrapCmd(m.runtime, c2)
	_ = vm.Set("g1", g1)
	_ = vm.Set("g2", g2)
	// sequence with mixed wrapped and descriptor (non-terminating)
	val, err := vm.RunString(`({ _cmdType: 'sequence', _cmdID: 3, cmds: [ g1, tea.clearScreen(), g2 ] })`)
	require.NoError(t, err)

	// Also extract individual elements to validate element conversion
	first, err := vm.RunString(`({ _cmdType: 'sequence', _cmdID: 3, cmds: [ g1, tea.clearScreen(), g2 ] }).cmds[0]`)
	require.NoError(t, err)
	second, err := vm.RunString(`({ _cmdType: 'sequence', _cmdID: 3, cmds: [ g1, tea.clearScreen(), g2 ] }).cmds[1]`)
	require.NoError(t, err)
	third, err := vm.RunString(`({ _cmdType: 'sequence', _cmdID: 3, cmds: [ g1, tea.clearScreen(), g2 ] }).cmds[2]`)
	require.NoError(t, err)

	cmd1 := m.valueToCmd(first)
	require.NotNil(t, cmd1, "first sequence element should convert to non-nil cmd")
	cmd2 := m.valueToCmd(second)
	require.NotNil(t, cmd2, "second sequence element (descriptor) should convert to non-nil cmd")
	cmd3 := m.valueToCmd(third)
	require.NotNil(t, cmd3, "third sequence element should convert to non-nil cmd")

	// Execute individual elements to verify they run and append to calls
	cmd1()
	cmd3()
	time.Sleep(10 * time.Millisecond)

	callsMu.Lock()
	if len(calls) != 2 {
		// If individual elements didn't run either, fail early with diagnostic
		callsMu.Unlock()
		require.Fail(t, "individual elements did not execute; sequence element extraction failed")
	}
	callsMu.Unlock()

	// Now convert the sequence descriptor as a whole
	seqCmd := m.valueToCmd(val)
	require.NotNil(t, seqCmd)

	// Executing a sequence command returns a slice-like message containing Cmds.
	// Simulate the runtime by invoking the cmd, reflecting over the returned value
	// and executing each contained Cmd in order.
	msg := seqCmd()
	require.NotNil(t, msg)
	rv := reflect.ValueOf(msg)
	require.Equal(t, reflect.Slice, rv.Kind(), "sequence cmd should return a slice-like message")

	for i := 0; i < rv.Len(); i++ {
		if rv.Index(i).IsNil() {
			continue
		}
		elem := rv.Index(i).Interface()
		cmd, ok := elem.(tea.Cmd)
		require.True(t, ok, "sequence element should be a tea.Cmd")
		_ = cmd()
	}

	// Wait briefly for any asynchronous closures to run
	time.Sleep(20 * time.Millisecond)
	callsMu.Lock()
	defer callsMu.Unlock()
	require.Equal(t, 4, len(calls), "expect four total calls after executing sequence and individuals")
	require.Equal(t, 1, calls[0])
	require.Equal(t, 2, calls[1])
	require.Equal(t, 1, calls[2])
	require.Equal(t, 2, calls[3])
}

func TestValueToCmd_NullUndefined(t *testing.T) {
	vm := goja.New()
	ctx := context.Background()
	manager := newTestManager(ctx, vm)

	// Create model with runtime
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))
	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	result, err := vm.RunString(`tea.newModel({ init: function() { return {}; }, update: function(msg, model) { return [model, null]; }, view: function(model) { return ''; } });`)
	require.NoError(t, err)

	modelWrapper := result.ToObject(vm)
	getModelFn, ok := goja.AssertFunction(modelWrapper.Get("_getModel"))
	require.True(t, ok, "_getModel should be a function")
	modelVal, err := getModelFn(goja.Undefined())
	require.NoError(t, err)
	m, ok := modelVal.Export().(*jsModel)
	require.True(t, ok)

	nullVal := goja.Null()
	undefinedVal := goja.Undefined()

	assert.Nil(t, m.valueToCmd(nullVal), "null should map to nil cmd")
	assert.Nil(t, m.valueToCmd(undefinedVal), "undefined should map to nil cmd")
}

func TestValueToCmd_InvalidObject(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	// Create model
	result, err := vm.RunString(`tea.newModel({ init: function() { return {}; }, update: function(msg, model) { return [model, null]; }, view: function(model) { return ''; } });`)
	require.NoError(t, err)

	modelWrapper := result.ToObject(vm)
	getModelFn, ok := goja.AssertFunction(modelWrapper.Get("_getModel"))
	require.True(t, ok, "_getModel should be a function")
	modelVal, err := getModelFn(goja.Undefined())
	require.NoError(t, err)
	m, ok := modelVal.Export().(*jsModel)
	require.True(t, ok)

	val, err := vm.RunString(`({foo: 1})`)
	require.NoError(t, err)

	assert.Nil(t, m.valueToCmd(val), "object without _cmdType should return nil")
}

func TestCommandRoundtrip_ThroughJSArrayAndObject(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	// Create model
	result, err := vm.RunString(`tea.newModel({ init: function() { return {}; }, update: function(msg, model) { return [model, null]; }, view: function(model) { return ''; } });`)
	require.NoError(t, err)

	modelWrapper := result.ToObject(vm)
	getModelFn, ok := goja.AssertFunction(modelWrapper.Get("_getModel"))
	require.True(t, ok, "_getModel should be a function")
	modelVal, err := getModelFn(goja.Undefined())
	require.NoError(t, err)
	m, ok := modelVal.Export().(*jsModel)
	require.True(t, ok)

	// Create a closure command and wrap it
	type testMsg struct{ id int }
	closure := func() tea.Msg { return testMsg{id: 7} }
	wrapped := WrapCmd(m.runtime, closure)

	// Put wrapped into JS array
	_ = vm.Set("wrappedCmd", wrapped)
	valArr, err := vm.RunString(`[wrappedCmd][0]`)
	require.NoError(t, err)
	cmd := m.valueToCmd(valArr)
	require.NotNil(t, cmd)
	res := cmd()
	_, okRes := res.(testMsg)
	assert.True(t, okRes, "wrapped command should survive storing in JS array")

	// Put wrapped into JS object property
	valObj, err := vm.RunString(`({c: wrappedCmd}).c`)
	require.NoError(t, err)
	cmd2 := m.valueToCmd(valObj)
	require.NotNil(t, cmd2)
	res2 := cmd2()
	_, okRes2 := res2.(testMsg)
	assert.True(t, okRes2, "wrapped command should survive storing as object property")
}
