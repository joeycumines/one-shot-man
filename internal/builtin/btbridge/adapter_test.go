package btbridge

import (
	"context"
	"testing"
	"time"

	bt "github.com/joeycumines/go-behaviortree"
	"github.com/stretchr/testify/require"
)

// waitForCompletion polls the node until it returns a non-Running status or timeout.
// This helper avoids brittle hardcoded iteration counts in tests.
func waitForCompletion(t *testing.T, node bt.Node, timeout time.Duration) (bt.Status, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatal("timeout waiting for node completion")
			return bt.Failure, ctx.Err()
		case <-ticker.C:
			status, err := node.Tick()
			if status != bt.Running || err != nil {
				return status, err
			}
		}
	}
}

func TestJSLeafAdapter_Success(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	defer bridge.Stop()

	// Load a simple success leaf
	err = bridge.LoadScript("leaf.js", `
		async function successLeaf(ctx, args) {
			return bt.success;
		}
	`)
	require.NoError(t, err)

	// Create the adapter node
	node := NewJSLeafAdapter(bridge, "successLeaf", nil)

	// First tick should return Running (dispatching to JS)
	status, err := node.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Running, status)

	// Wait for JS to complete
	finalStatus, err := waitForCompletion(t, node, time.Second)
	require.NoError(t, err)
	require.Equal(t, bt.Success, finalStatus)
}

func TestJSLeafAdapter_Failure(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	defer bridge.Stop()

	// Load a failure leaf
	err = bridge.LoadScript("leaf.js", `
		async function failureLeaf(ctx, args) {
			return bt.failure;
		}
	`)
	require.NoError(t, err)

	node := NewJSLeafAdapter(bridge, "failureLeaf", nil)

	// First tick returns Running
	status, err := node.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Running, status)

	// Wait for completion
	finalStatus, err := waitForCompletion(t, node, time.Second)
	require.NoError(t, err)
	require.Equal(t, bt.Failure, finalStatus)
}

func TestJSLeafAdapter_WithBlackboard(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	defer bridge.Stop()

	bb := NewBlackboard()
	bb.Set("input", 10)

	err = bridge.ExposeBlackboard("sharedCtx", bb)
	require.NoError(t, err)

	// Load a leaf that reads and writes to blackboard using the global sharedCtx
	// Note: The ctx parameter to processData comes from runLeaf, but we use the global sharedCtx
	err = bridge.LoadScript("leaf.js", `
		async function processData(ctx, args) {
			var input = sharedCtx.get("input");
			sharedCtx.set("output", input * 2);
			return bt.success;
		}
	`)
	require.NoError(t, err)

	node := NewJSLeafAdapter(bridge, "processData", nil)

	// Run the node to completion
	status, err := node.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Running, status)

	finalStatus, err := waitForCompletion(t, node, time.Second)
	require.NoError(t, err)
	require.Equal(t, bt.Success, finalStatus)
	require.Equal(t, int64(20), bb.Get("output"))
}

func TestJSLeafAdapter_Error(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	defer bridge.Stop()

	// Load a leaf that throws an error
	err = bridge.LoadScript("leaf.js", `
		async function errorLeaf(ctx, args) {
			throw new Error("test error");
		}
	`)
	require.NoError(t, err)

	node := NewJSLeafAdapter(bridge, "errorLeaf", nil)

	// First tick returns Running
	status, err := node.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Running, status)

	// Wait for completion - expect error
	finalStatus, finalErr := waitForCompletion(t, node, time.Second)
	require.Equal(t, bt.Failure, finalStatus)
	require.Error(t, finalErr)
	require.Contains(t, finalErr.Error(), "test error")
}

func TestJSLeafAdapter_NonExistentFunction(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	defer bridge.Stop()

	node := NewJSLeafAdapter(bridge, "nonExistentFunc", nil)

	// First tick returns Running (dispatching attempt)
	status, err := node.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Running, status)

	// Should fail with error about non-callable function
	finalStatus, finalErr := waitForCompletion(t, node, time.Second)
	require.Equal(t, bt.Failure, finalStatus)
	require.Error(t, finalErr)
	require.Contains(t, finalErr.Error(), "not callable")
}

func TestJSLeafAdapter_Cancellation(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	defer bridge.Stop()

	// Load a slow leaf
	err = bridge.LoadScript("leaf.js", `
		async function slowLeaf(ctx, args) {
			await new Promise(resolve => setTimeout(resolve, 1000));
			return bt.success;
		}
	`)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	node := NewJSLeafAdapterWithContext(ctx, bridge, "slowLeaf", nil)

	// First tick returns Running
	status, err := node.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Running, status)

	// Cancel while running
	cancel()

	// Next tick should fail due to cancellation
	status, err = node.Tick()
	require.Equal(t, bt.Failure, status)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cancelled")
}

func TestJSLeafAdapter_MultipleExecutions(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	defer bridge.Stop()

	bb := NewBlackboard()
	bb.Set("counter", 0)

	err = bridge.ExposeBlackboard("bb", bb)
	require.NoError(t, err)

	// Load a leaf that increments a counter
	err = bridge.LoadScript("leaf.js", `
		async function incrementCounter(ctx, args) {
			var count = bb.get("counter");
			bb.set("counter", count + 1);
			return bt.success;
		}
	`)
	require.NoError(t, err)

	node := NewJSLeafAdapter(bridge, "incrementCounter", nil)

	// Run the node multiple times
	for round := 1; round <= 3; round++ {
		// First tick returns Running
		status, err := node.Tick()
		require.NoError(t, err)
		require.Equal(t, bt.Running, status)

		// Wait for completion
		finalStatus, err := waitForCompletion(t, node, time.Second)
		require.NoError(t, err)
		require.Equal(t, bt.Success, finalStatus)
		require.Equal(t, int64(round), bb.Get("counter"))
	}
}

func TestBlockingJSLeaf_Success(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	defer bridge.Stop()

	err = bridge.LoadScript("leaf.js", `
		async function blockingSuccess(ctx, args) {
			return bt.success;
		}
	`)
	require.NoError(t, err)

	node := BlockingJSLeaf(bridge, "blockingSuccess", nil)

	// Blocking leaf should complete in one tick
	status, err := node.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Success, status)
}

func TestBlockingJSLeaf_Failure(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	defer bridge.Stop()

	err = bridge.LoadScript("leaf.js", `
		async function blockingFailure(ctx, args) {
			return bt.failure;
		}
	`)
	require.NoError(t, err)

	node := BlockingJSLeaf(bridge, "blockingFailure", nil)

	status, err := node.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Failure, status)
}

func TestBlockingJSLeaf_Error(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	defer bridge.Stop()

	err = bridge.LoadScript("leaf.js", `
		async function blockingError(ctx, args) {
			throw new Error("blocking error");
		}
	`)
	require.NoError(t, err)

	node := BlockingJSLeaf(bridge, "blockingError", nil)

	status, err := node.Tick()
	require.Equal(t, bt.Failure, status)
	require.Error(t, err)
	require.Contains(t, err.Error(), "blocking error")
}

func TestMapJSStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected bt.Status
	}{
		{"running", bt.Running},
		{"success", bt.Success},
		{"failure", bt.Failure},
		{"unknown", bt.Failure},
		{"", bt.Failure},
		{"SUCCESS", bt.Failure}, // Case sensitive
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := mapJSStatus(tc.input)
			require.Equal(t, tc.expected, result)
		})
	}
}

// TestJSLeafAdapter_GenerationGuard verifies that stale callbacks from cancelled
// runs don't corrupt the adapter state of subsequent runs (C1 fix).
func TestJSLeafAdapter_GenerationGuard(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	defer bridge.Stop()

	bb := NewBlackboard()
	err = bridge.ExposeBlackboard("state", bb)
	require.NoError(t, err)

	// The leaf records which run it is when it starts, then waits, then completes
	err = bridge.LoadScript("leaf.js", `
		var runCount = 0;
		async function slowLeaf(ctx, args) {
			runCount++;
			var myRun = runCount;
			state.set("runStarted_" + myRun, true);
			// Wait (longer for first run, shorter for second)
			await new Promise(resolve => setTimeout(resolve, myRun === 1 ? 100 : 10));
			state.set("runCompleted_" + myRun, true);
			return myRun === 1 ? bt.failure : bt.success;
		}
	`)
	require.NoError(t, err)

	// Create first adapter with cancellable context
	ctx1, cancel1 := context.WithCancel(context.Background())
	node1 := NewJSLeafAdapterWithContext(ctx1, bridge, "slowLeaf", nil)

	// Start first run
	status, err := node1.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Running, status)

	// Wait for first run to start in JS
	time.Sleep(20 * time.Millisecond)

	// Cancel the first adapter while it's waiting
	cancel1()

	// Tick again to acknowledge cancellation
	status, err = node1.Tick()
	require.Equal(t, bt.Failure, status)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cancelled")

	// Now start a SECOND adapter (different instance, fresh context)
	// and verify it gets ITS OWN result, not the stale result from run 1
	node2 := NewJSLeafAdapter(bridge, "slowLeaf", nil)

	// Start second run
	status, err = node2.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Running, status)

	// Wait for completion of second run
	finalStatus, err := waitForCompletion(t, node2, 2*time.Second)
	require.NoError(t, err)
	require.Equal(t, bt.Success, finalStatus, "Second run should succeed (returns bt.success)")

	// Both runs should have completed in JS (even the cancelled one's JS continues)
	// but the critical test is that node2 got the success result, not failure from run1
}

// TestBlockingJSLeaf_BridgeStopWhileWaiting verifies that BlockingJSLeaf
// doesn't deadlock when the bridge stops while waiting (C3 fix).
func TestBlockingJSLeaf_BridgeStopWhileWaiting(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	// Note: We'll stop the bridge manually, no defer

	err = bridge.LoadScript("leaf.js", `
		async function slowBlockingLeaf(ctx, args) {
			await new Promise(resolve => setTimeout(resolve, 10000)); // Very long
			return bt.success;
		}
	`)
	require.NoError(t, err)

	node := BlockingJSLeaf(bridge, "slowBlockingLeaf", nil)

	// Run the blocking leaf in a goroutine
	resultCh := make(chan struct {
		status bt.Status
		err    error
	}, 1)

	go func() {
		status, err := node.Tick()
		resultCh <- struct {
			status bt.Status
			err    error
		}{status, err}
	}()

	// Give it a moment to start
	time.Sleep(20 * time.Millisecond)

	// Stop the bridge while the leaf is waiting
	bridge.Stop()

	// Should unblock and return failure
	select {
	case r := <-resultCh:
		require.Equal(t, bt.Failure, r.status)
		require.Error(t, r.err)
		require.Contains(t, r.err.Error(), "bridge stopped")
	case <-time.After(time.Second):
		t.Fatal("BlockingJSLeaf should have unblocked when bridge stopped")
	}
}

// TestBlockingJSLeaf_ContextCancellation verifies that BlockingJSLeafWithContext
// respects context cancellation.
func TestBlockingJSLeaf_ContextCancellation(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	defer bridge.Stop()

	err = bridge.LoadScript("leaf.js", `
		async function verySlowLeaf(ctx, args) {
			await new Promise(resolve => setTimeout(resolve, 10000));
			return bt.success;
		}
	`)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	node := BlockingJSLeafWithContext(ctx, bridge, "verySlowLeaf", nil)

	// Run in goroutine
	resultCh := make(chan struct {
		status bt.Status
		err    error
	}, 1)

	go func() {
		status, err := node.Tick()
		resultCh <- struct {
			status bt.Status
			err    error
		}{status, err}
	}()

	// Give it a moment to start
	time.Sleep(20 * time.Millisecond)

	// Cancel context
	cancel()

	// Should unblock and return failure
	select {
	case r := <-resultCh:
		require.Equal(t, bt.Failure, r.status)
		require.Error(t, r.err)
		require.ErrorIs(t, r.err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("BlockingJSLeafWithContext should have respected context cancellation")
	}
}
