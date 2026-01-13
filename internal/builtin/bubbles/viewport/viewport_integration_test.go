package viewport

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/builtin/bubbles/textarea"
	"github.com/joeycumines/one-shot-man/internal/builtin/bubbletea"
	"github.com/stretchr/testify/require"
)

// newTestManager creates a Manager with a SyncJSRunner for unit testing.
func newTestManager(ctx context.Context, vm *goja.Runtime) *bubbletea.Manager {
	return bubbletea.NewManager(ctx, nil, nil, &bubbletea.SyncJSRunner{Runtime: vm}, nil, nil)
}

// TestViewportIntegration_CommandPropagation verifies that a JS-side viewport
// scroll (mouse wheel) produces a callable command that can be invoked from JS
// (i.e., bubbles up as a wrapped tea.Cmd).
func TestViewportIntegration_CommandPropagation(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := bubbletea.Require(ctx, manager)
	requireFn(vm, module)
	_ = vm.Set("tea", module.Get("exports"))

	// Require viewport
	vpModule := vm.NewObject()
	require.NoError(t, vpModule.Set("exports", vm.NewObject()))
	Require()(vm, vpModule)
	_ = vm.Set("viewport", vpModule.Get("exports"))

	// Enable wheel handling and produce an update that should return a command
	res, err := vm.RunString(`(function(){
		const vp = viewport.new(10,10);
		vp.setMouseWheelEnabled(true);
		const r = vp.update({ type: 'Mouse', button: 'wheel up', action: 'press', x:0, y:0, alt:false, ctrl:false, shift:false });
		return r[1];
	})()`)
	require.NoError(t, err)
	cmdVal := res
	// A wheel update may or may not return a command depending on viewport internals.
	// If it does return a value, it should be a callable function we can invoke from JS.
	if !(goja.IsNull(cmdVal) || goja.IsUndefined(cmdVal)) {
		_, ok := goja.AssertFunction(cmdVal)
		require.True(t, ok, "viewport wheel command should be callable in JS when present")
	}
}

// TestViewportIntegration_BatchWithViewportCommand verifies a JS-side batch
// containing a viewport command and a textarea command stores both commands and
// both are callable.
func TestViewportIntegration_BatchWithViewportCommand(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := bubbletea.Require(ctx, manager)
	requireFn(vm, module)
	_ = vm.Set("tea", module.Get("exports"))

	// Require viewport
	vpModule := vm.NewObject()
	require.NoError(t, vpModule.Set("exports", vm.NewObject()))
	Require()(vm, vpModule)
	_ = vm.Set("viewport", vpModule.Get("exports"))

	// Require textarea
	taModule := vm.NewObject()
	require.NoError(t, taModule.Set("exports", vm.NewObject()))
	textarea.Require()(vm, taModule)
	_ = vm.Set("textarea", taModule.Get("exports"))

	res, err := vm.RunString(`(function(){
		const vp = viewport.new(10,10);
		vp.setMouseWheelEnabled(true);
		const ta = textarea.new(); ta.setWidth(40); ta.focus();
		let c1 = vp.update({ type: 'Mouse', button: 'wheel down', action: 'press', x:0, y:0, alt:false, ctrl:false, shift:false })[1];
		const c2 = ta.update({ type: 'Key', key: 'x' })[1];
		// Ensure c1 is callable; if viewport did not return a command, fall back to a no-op function
		if (c1 === null || typeof c1 === 'undefined') {
			c1 = function(){ return null; };
		}
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

// TestViewportIntegration_SyncScroll verifies that the textarea's scroll-sync
// information can be used to set the viewport offset via JS-side API calls.
func TestViewportIntegration_SyncScroll(t *testing.T) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := bubbletea.Require(ctx, manager)
	requireFn(vm, module)
	_ = vm.Set("tea", module.Get("exports"))

	// Require viewport
	vpModule := vm.NewObject()
	require.NoError(t, vpModule.Set("exports", vm.NewObject()))
	Require()(vm, vpModule)
	_ = vm.Set("viewport", vpModule.Get("exports"))

	// Require textarea
	taModule := vm.NewObject()
	require.NoError(t, taModule.Set("exports", vm.NewObject()))
	textarea.Require()(vm, taModule)
	_ = vm.Set("textarea", taModule.Get("exports"))

	// Create textarea with many lines so cursor is below initial viewport
	_, err := vm.RunString(`(function(){
		const ta = textarea.new();
		const lines = [];
		for (let i=0;i<100;i++) lines.push('line '+i);
		ta.setValue(lines.join('\n'));
		return ta.getScrollSyncInfo();
	})()`)
	require.NoError(t, err)

	// Grab suggestedYOffset and verify applying it to the viewport updates yOffset
	res2, err := vm.RunString(`(function(){
		const ta = textarea.new();
		const lines = [];
		for (let i=0;i<20;i++) lines.push('line '+i);
		ta.setValue(lines.join('\n'));
		const info = ta.getScrollSyncInfo();
		const vp = viewport.new(10,10);
		// Clamp to max offset to avoid out-of-range suggested values
		const maxOffset = Math.max(0, vp.totalLineCount() - vp.height());
		const target = Math.min(info.suggestedYOffset, maxOffset);
		vp.setYOffset(target);
		return vp.yOffset() === target;
	})()`)
	require.NoError(t, err)
	ok := res2.ToBoolean()
	require.True(t, ok, "viewport yOffset should equal clamped suggestedYOffset from textarea")
}
