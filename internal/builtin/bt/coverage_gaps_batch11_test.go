package bt

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	bt "github.com/joeycumines/go-behaviortree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// T144: dispatchJSWithGen — uncovered error paths
// ============================================================================

// TestJSLeafAdapter_RunLeafUndefined verifies that when runLeaf is not defined
// in the VM, dispatchJSWithGen finalizes with an error.
func TestJSLeafAdapter_RunLeafUndefined(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	// Load a script that defines a function but NOT runLeaf.
	err := bridge.LoadScript("leaf.js", `
		function myLeaf(ctx, args) {
			return bt.success;
		}
	`)
	require.NoError(t, err)

	// Remove runLeaf from the VM.
	bridge.RunOnLoop(func(vm *goja.Runtime) {
		vm.Set("runLeaf", goja.Undefined())
	})
	// Wait for it to execute.
	time.Sleep(50 * time.Millisecond)

	fn, err := bridge.GetCallable("myLeaf")
	require.NoError(t, err)

	node := NewJSLeafAdapter(context.TODO(), bridge, fn, nil)
	// First tick dispatches.
	status, err := node.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Running, status)

	// Wait for finalization.
	finalStatus, finalErr := waitForCompletion(t, node, 2*time.Second)
	assert.Equal(t, bt.Failure, finalStatus)
	require.Error(t, finalErr)
	assert.Contains(t, finalErr.Error(), "runLeaf")
}

// TestJSLeafAdapter_RunLeafNotCallable verifies that when runLeaf is set to a
// non-function value, dispatchJSWithGen reports the error.
func TestJSLeafAdapter_RunLeafNotCallable(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("leaf.js", `
		function someLeaf(ctx, args) {
			return bt.success;
		}
	`)
	require.NoError(t, err)

	// Set runLeaf to a string (not callable).
	bridge.RunOnLoop(func(vm *goja.Runtime) {
		vm.Set("runLeaf", "not-a-function")
	})
	time.Sleep(50 * time.Millisecond)

	fn, err := bridge.GetCallable("someLeaf")
	require.NoError(t, err)

	node := NewJSLeafAdapter(context.TODO(), bridge, fn, nil)
	status, err := node.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Running, status)

	finalStatus, finalErr := waitForCompletion(t, node, 2*time.Second)
	assert.Equal(t, bt.Failure, finalStatus)
	require.Error(t, finalErr)
	assert.Contains(t, finalErr.Error(), "not a callable")
}

// TestJSLeafAdapter_TickPanic verifies that a panic in the tick wrapper is
// caught by the recover in dispatchJSWithGen.
func TestJSLeafAdapter_TickPanic(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	// Create an adapter with a tick function that panics.
	panicTick := func(this goja.Value, args ...goja.Value) (goja.Value, error) {
		panic("intentional panic in tick")
	}

	adapter := &JSLeafAdapter{
		bridge: bridge,
		tick:   panicTick,
		ctx:    context.Background(),
	}
	node := bt.New(adapter.Tick)

	// First tick dispatches.
	status, err := node.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Running, status)

	// The panic should be caught, adapter should finalize with Failure.
	finalStatus, finalErr := waitForCompletion(t, node, 2*time.Second)
	assert.Equal(t, bt.Failure, finalStatus)
	require.Error(t, finalErr)
	assert.Contains(t, finalErr.Error(), "panic")
}

// TestJSLeafAdapter_CallbackWithError verifies that when the JS callback
// passes a non-null/non-undefined error argument, it's propagated.
func TestJSLeafAdapter_CallbackWithError(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("leaf.js", `
		async function errorCallbackLeaf(ctx, args) {
			throw new Error("callback error value");
		}
	`)
	require.NoError(t, err)

	fn, err := bridge.GetCallable("errorCallbackLeaf")
	require.NoError(t, err)

	node := NewJSLeafAdapter(context.TODO(), bridge, fn, nil)
	status, err := node.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Running, status)

	finalStatus, finalErr := waitForCompletion(t, node, 2*time.Second)
	assert.Equal(t, bt.Failure, finalStatus)
	require.Error(t, finalErr)
	assert.Contains(t, finalErr.Error(), "callback error value")
}

// TestJSLeafAdapter_FinalizeStaleGeneration verifies that finalize() rejects
// callbacks from stale generations after cancellation.
func TestJSLeafAdapter_FinalizeStaleGeneration(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)

	err := bridge.LoadScript("leaf.js", `
		async function slowForStale(ctx, args) {
			await new Promise(resolve => setTimeout(resolve, 500));
			return bt.success;
		}
	`)
	require.NoError(t, err)

	fn, err := bridge.GetCallable("slowForStale")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	// Create the adapter directly so we can inspect state later.
	adapter := &JSLeafAdapter{
		bridge: bridge,
		tick:   fn,
		ctx:    ctx,
	}
	node := bt.New(adapter.Tick)

	// First tick starts execution.
	status, err := node.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Running, status)

	// Cancel the context to bump generation.
	cancel()

	// The next tick should detect cancellation and return Failure.
	status, err = node.Tick()
	assert.Equal(t, bt.Failure, status)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled")

	// At this point the generation has been bumped. When the slow JS
	// eventually finishes and calls finalize with the old generation,
	// it should be silently discarded. The adapter should stay in Idle state.
	time.Sleep(150 * time.Millisecond)
	adapter.mu.Lock()
	st := adapter.state
	adapter.mu.Unlock()
	assert.Equal(t, StateIdle, st)
}

// TestJSLeafAdapter_Tick_DefaultCase verifies the invalid state default case.
func TestJSLeafAdapter_Tick_DefaultCase(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("leaf.js", `
		async function dummyLeaf(ctx, args) {
			return bt.success;
		}
	`)
	require.NoError(t, err)

	fn, err := bridge.GetCallable("dummyLeaf")
	require.NoError(t, err)

	adapter := &JSLeafAdapter{
		bridge: bridge,
		tick:   fn,
		ctx:    context.Background(),
	}

	// Force an invalid state.
	adapter.mu.Lock()
	adapter.state = AsyncState(99) // invalid
	adapter.mu.Unlock()

	status, err := adapter.Tick(nil)
	assert.Equal(t, bt.Failure, status)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid async state")
}

// ============================================================================
// T145: BlockingJSLeaf — uncovered error paths
// ============================================================================

// TestBlockingJSLeaf_RunOnLoopFalse verifies that when bridge is stopped before
// BlockingJSLeaf ticks, it returns an event loop terminated error.
func TestBlockingJSLeaf_RunOnLoopFalse(t *testing.T) {
	t.Parallel()

	bridge, stopLoop := testBridgeWithManualShutdown(t)

	err := bridge.LoadScript("leaf.js", `
		async function staleLeaf(ctx, args) {
			return bt.success;
		}
	`)
	require.NoError(t, err)

	fn, err := bridge.GetCallable("staleLeaf")
	require.NoError(t, err)

	node := blockingJSLeaveNoVM(context.TODO(), bridge, fn, nil)

	// Stop the bridge/loop BEFORE ticking.
	bridge.Stop()
	stopLoop()

	// Give it a moment to shut down.
	time.Sleep(50 * time.Millisecond)

	status, err := node.Tick()
	assert.Equal(t, bt.Failure, status)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "event loop terminated")
}

// TestBlockingJSLeaf_BridgeDoneDuringWait verifies that when the bridge stops
// while BlockingJSLeaf is waiting on the channel, it returns an error.
func TestBlockingJSLeaf_BridgeDoneDuringWait(t *testing.T) {
	t.Parallel()

	bridge, stopLoop := testBridgeWithManualShutdown(t)

	// Load a VERY slow leaf that won't complete before we stop the bridge.
	err := bridge.LoadScript("leaf.js", `
		async function verySlowLeaf(ctx, args) {
			await new Promise(resolve => setTimeout(resolve, 10000));
			return bt.success;
		}
	`)
	require.NoError(t, err)

	fn, err := bridge.GetCallable("verySlowLeaf")
	require.NoError(t, err)

	node := blockingJSLeaveNoVM(context.TODO(), bridge, fn, nil)

	// Start a goroutine that stops the bridge after a short delay.
	go func() {
		time.Sleep(100 * time.Millisecond)
		bridge.Stop()
		stopLoop()
	}()

	// This should block until the bridge stops, then return an error.
	status, err := node.Tick()
	assert.Equal(t, bt.Failure, status)
	require.Error(t, err)
	// Could be "bridge stopped" or panic recovery, depending on timing.
}

// TestBlockingJSLeaf_PanicInWrapper verifies panic recovery in the channel-based
// BlockingJSLeaf path.
func TestBlockingJSLeaf_PanicInWrapper(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	// Create a tick function that panics.
	panicTick := func(this goja.Value, args ...goja.Value) (goja.Value, error) {
		panic("wrapper panic in blocking path")
	}

	node := blockingJSLeaveNoVM(context.TODO(), bridge, panicTick, nil)

	status, err := node.Tick()
	assert.Equal(t, bt.Failure, status)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "panic")
}

// TestBlockingJSLeaf_WithGetCtxFunc verifies that the getCtx function correctly
// passes context to the JS leaf in the channel-based path.
func TestBlockingJSLeaf_WithGetCtxFunc(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("leaf.js", `
		async function ctxCheckLeaf(ctx, args) {
			if (ctx && ctx.magic === "yes") {
				return bt.success;
			}
			return bt.failure;
		}
	`)
	require.NoError(t, err)

	fn, err := bridge.GetCallable("ctxCheckLeaf")
	require.NoError(t, err)

	node := BlockingJSLeaf(context.TODO(), bridge, nil, fn, func() any {
		return map[string]any{"magic": "yes"}
	})

	status, err := node.Tick()
	require.NoError(t, err)
	assert.Equal(t, bt.Success, status)
}

// TestBlockingJSLeaf_ChannelPath_RunLeafNotFound verifies the runLeaf not found
// error in the channel-based path (not on event loop).
func TestBlockingJSLeaf_ChannelPath_RunLeafNotFound(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("leaf.js", `
		function channelLeaf(ctx, args) {
			return bt.success;
		}
	`)
	require.NoError(t, err)

	fn, err := bridge.GetCallable("channelLeaf")
	require.NoError(t, err)

	// Remove runLeaf from VM.
	bridge.RunOnLoop(func(vm *goja.Runtime) {
		vm.Set("runLeaf", goja.Undefined())
	})
	time.Sleep(50 * time.Millisecond)

	node := blockingJSLeaveNoVM(context.TODO(), bridge, fn, nil)

	status, err := node.Tick()
	assert.Equal(t, bt.Failure, status)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runLeaf")
}

// TestBlockingJSLeaf_ChannelPath_CallbackError verifies that when the JS
// function passes an error through the callback in the channel path.
func TestBlockingJSLeaf_ChannelPath_CallbackError(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("leaf.js", `
		async function errorLeaf(ctx, args) {
			throw new Error("channel error");
		}
	`)
	require.NoError(t, err)

	fn, err := bridge.GetCallable("errorLeaf")
	require.NoError(t, err)

	node := blockingJSLeaveNoVM(context.TODO(), bridge, fn, nil)

	status, err := node.Tick()
	assert.Equal(t, bt.Failure, status)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "channel error")
}
