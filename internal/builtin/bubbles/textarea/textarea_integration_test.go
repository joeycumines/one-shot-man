package textarea

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/builtin/bubbletea"
	"github.com/stretchr/testify/require"
)

// TestTextareaIntegration_CommandPropagation verifies that a parent JS model
// that delegates to textarea.update() and returns the textarea command will
// receive a wrapped command that can be extracted and executed by Go.
func TestTextareaIntegration_CommandPropagation(t *testing.T) {
	ctx := context.Background()
	manager := bubbletea.NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := bubbletea.Require(ctx, manager)
	requireFn(vm, module)

	// Expose tea
	_ = vm.Set("tea", module.Get("exports"))

	// Require textarea module
	taModule := vm.NewObject()
	require.NoError(t, taModule.Set("exports", vm.NewObject()))
	Require()(vm, taModule)
	_ = vm.Set("textarea", taModule.Get("exports"))

	res, err := vm.RunString(`(function() {
		const ta = textarea.new();
		ta.setWidth(40);
		ta.focus();
		const r = ta.update({type: 'Key', key: 'a'});
		return r[1];
	})()`)
	require.NoError(t, err)
	cmdVal := res
	require.False(t, goja.IsNull(cmdVal))
	require.False(t, goja.IsUndefined(cmdVal))
	fn, ok := goja.AssertFunction(cmdVal)
	require.True(t, ok, "command should be callable in JS")
	out, err := fn(cmdVal)
	require.NoError(t, err)
	require.False(t, goja.IsNull(out))
	require.False(t, goja.IsUndefined(out))
}

// TestTextareaIntegration_BatchWithTextareaCommand verifies a JS-side batch
// that includes two textarea commands stores both commands and they are callable.
func TestTextareaIntegration_BatchWithTextareaCommand(t *testing.T) {
	ctx := context.Background()
	manager := bubbletea.NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := bubbletea.Require(ctx, manager)
	requireFn(vm, module)
	_ = vm.Set("tea", module.Get("exports"))

	// Require textarea
	taModule := vm.NewObject()
	require.NoError(t, taModule.Set("exports", vm.NewObject()))
	Require()(vm, taModule)
	_ = vm.Set("textarea", taModule.Get("exports"))

	res, err := vm.RunString(`(function() {
		const ta1 = textarea.new(); ta1.setWidth(40); ta1.focus();
		const ta2 = textarea.new(); ta2.setWidth(40); ta2.focus();
		const c1 = ta1.update({type:'Key', key:'x'})[1];
		const c2 = ta2.update({type:'Key', key:'y'})[1];
		return tea.batch(c1, c2);
	})()`)
	require.NoError(t, err)
	batchVal := res.ToObject(vm)
	require.Equal(t, "batch", batchVal.Get("_cmdType").String())
	cmds := batchVal.Get("cmds").ToObject(vm)
	require.Equal(t, int64(2), cmds.Get("length").ToInteger())

	first := cmds.Get("0")
	second := cmds.Get("1")
	fFn, ok := goja.AssertFunction(first)
	require.True(t, ok)
	sFn, ok := goja.AssertFunction(second)
	require.True(t, ok)

	_, err = fFn(first)
	require.NoError(t, err)
	_, err = sFn(second)
	require.NoError(t, err)
}

// TestTextareaIntegration_SequenceWithTextareaCommand verifies that sequence
// descriptors mixing wrapped and descriptor commands expose callable commands in order.
func TestTextareaIntegration_SequenceWithTextareaCommand(t *testing.T) {
	ctx := context.Background()
	manager := bubbletea.NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := bubbletea.Require(ctx, manager)
	requireFn(vm, module)
	_ = vm.Set("tea", module.Get("exports"))

	// Prepare two textarea instances
	// Require textarea module (missing require previously caused ReferenceError)
	taModule := vm.NewObject()
	require.NoError(t, taModule.Set("exports", vm.NewObject()))
	Require()(vm, taModule)
	_ = vm.Set("textarea", taModule.Get("exports"))

	res, err := vm.RunString(`(function() {
		const ta1 = textarea.new(); ta1.setWidth(40); ta1.focus();
		const ta2 = textarea.new(); ta2.setWidth(40); ta2.focus();
		const c1 = ta1.update({type:'Key', key:'1'})[1];
		const c2 = ta2.update({type:'Key', key:'2'})[1];
		return ({ _cmdType: 'sequence', _cmdID: 99, cmds: [ c1, tea.clearScreen(), c2 ] });
	})()`)
	require.NoError(t, err)
	val := res.ToObject(vm)
	cmds := val.Get("cmds").ToObject(vm)
	first := cmds.Get("0")
	third := cmds.Get("2")
	fFn, ok := goja.AssertFunction(first)
	require.True(t, ok)
	_, err = fFn(first)
	require.NoError(t, err)
	// middle is descriptor - verify its type
	middle := cmds.Get("1").ToObject(vm)
	require.Equal(t, "clearScreen", middle.Get("_cmdType").String())
	// third element callable
	tFn, ok := goja.AssertFunction(third)
	require.True(t, ok)
	_, err = tFn(third)
	require.NoError(t, err)
}
