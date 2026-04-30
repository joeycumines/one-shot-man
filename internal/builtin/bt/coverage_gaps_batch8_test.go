package bt

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	gojanodejsconsole "github.com/dop251/goja_nodejs/console"
	gojarequire "github.com/dop251/goja_nodejs/require"
	goeventloop "github.com/joeycumines/go-eventloop"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==========================================================================
// bridge.go — NewBridgeWithEventLoop nil VM panic (line 84-85)
// ==========================================================================

// TestBridge_NewBridgeWithEventLoop_NilVMPanic verifies that passing a non-nil
// loop with a nil VM triggers the "goja runtime must not be nil" panic.
// The existing NilLoopPanic test passes nil for both, hitting the loop
// check first — this test isolates the VM nil-guard specifically.
func TestBridge_NewBridgeWithEventLoop_NilVMPanic(t *testing.T) {
	t.Parallel()

	loop, err := goeventloop.New()
	require.NoError(t, err)
	vm := goja.New()
	gojarequire.NewRegistry().Enable(vm)
	gojanodejsconsole.Enable(vm)
	adapter, err := gojaeventloop.New(loop, vm)
	require.NoError(t, err)
	require.NoError(t, adapter.Bind())

	loopCtx, loopCancel := context.WithCancel(context.Background())
	go loop.Run(loopCtx)
	t.Cleanup(func() {
		loopCancel()
		loop.Shutdown(context.Background())
	})

	assert.PanicsWithValue(t, "goja runtime must not be nil", func() {
		NewBridgeWithEventLoop(context.Background(), loop, nil, nil)
	})
}
