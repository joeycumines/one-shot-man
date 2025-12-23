package bubbletea

import (
	"context"
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	manager := NewManager(context.Background(), nil, nil, nil, nil)
	assert.NotNil(t, manager)
	assert.NotNil(t, manager.input)
	assert.NotNil(t, manager.output)
	assert.NotNil(t, manager.signalNotify)
	assert.NotNil(t, manager.signalStop)
}

func TestNewManager_WithCustomIO(t *testing.T) {
	input := os.Stdin
	output := os.Stdout
	manager := NewManager(context.Background(), input, output, nil, nil)
	assert.NotNil(t, manager)
	assert.Equal(t, input, manager.input)
	assert.Equal(t, output, manager.output)
}

func TestRequire_ExportsCorrectAPI(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
				if (msg.type === 'keyPress' && msg.key === 'q') {
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
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
