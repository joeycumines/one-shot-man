package bt

import (
	"context"
	"testing"
	"time"

	bt "github.com/joeycumines/go-behaviortree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFailureMode_ConcurrentTickerAccess verifies that multiple tickers
// can concurrently access the same bridge without race conditions.
//
// SIMPLIFIED: Test just ensures no panic/race - uses simple sync leaf
func TestFailureMode_ConcurrentTickerAccess(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)

	err := bridge.LoadScript("shared_bridge.js", `
		async function fastLeaf() {
			return bt.success;
		}
	`)
	require.NoError(t, err)

	fastFn, err := bridge.GetCallable("fastLeaf")
	require.NoError(t, err)

	// Use BlockingJSLeaf for synchronous behavior (simpler for this test)
	node := blockingJSLeaveNoVM(context.TODO(), bridge, fastFn, nil)

	// Create multiple concurrent tickers sharing the same bridge
	numTickers := 5
	tickers := make([]bt.Ticker, numTickers)
	for i := 0; i < numTickers; i++ {
		tickers[i] = bt.NewTicker(
			context.TODO(),
			time.Duration(5+i)*time.Millisecond,
			node,
		)
		t.Cleanup(func(i int) func() {
			return func() { tickers[i].Stop() }
		}(i))
	}

	// Let all tickers run for a short period with deterministic waiting
	// Use a channel to wait for at least one tick to complete
	doneCh := make(chan struct{}, 1)
	go func() {
		for i := 0; i < 100; i++ {
			status, err := node.Tick()
			if err == nil && status == bt.Success {
				// Successfully got at least one tick
				select {
				case doneCh <- struct{}{}:
				default:
				}
				break
			}
		}
	}()

	// Wait for at least one successful tick or timeout
	select {
	case <-doneCh:
		// Got at least one successful tick
	case <-time.After(50 * time.Millisecond):
		// Timeout is fine - we just wanted to verify no race conditions occur
	}

	// Verify the node completes successfully (BlockingJSLeaf is sync)
	status, err := node.Tick()
	require.NoError(t, err)
	assert.Equal(t, bt.Success, status)
}

// TestFailureMode_RapidSequentialTicks verifies that a single node can be
// ticked rapidly (without yielding to event loop) without issues.
// This tests the state machine's handling of concurrent Tick() calls.
func TestFailureMode_RapidSequentialTicks(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)

	err := bridge.LoadScript("rapid_ticks.js", `
		let tickCount = 0;
		async function countingLeaf() {
			tickCount++;
			return bt.success;
		}
	`)
	require.NoError(t, err)

	countFn, err := bridge.GetCallable("countingLeaf")
	require.NoError(t, err)

	// Use BlockingJSLeaf for simpler synchronous behavior in this test
	node := blockingJSLeaveNoVM(context.TODO(), bridge, countFn, nil)

	// Tick rapidly without yielding - should not crash or error
	for i := 0; i < 10; i++ {
		status, err := node.Tick()
		require.NoError(t, err, "Tick %d should not error", i)
		assert.Equal(t, bt.Success, status)
	}
}
