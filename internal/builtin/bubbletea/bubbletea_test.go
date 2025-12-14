package bubbletea

import (
	"context"
	"os"
	"testing"

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
	modelVal := modelWrapper.Get("_model")
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
	modelVal := modelWrapper.Get("_model")
	model, ok := modelVal.Export().(*jsModel)
	require.True(t, ok)

	// Initialize
	model.Init()

	// Test View
	view := model.View()
	assert.Equal(t, "Message: Hello World", view)
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
