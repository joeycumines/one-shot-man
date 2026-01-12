package bt

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dop251/goja"
	bt "github.com/joeycumines/go-behaviortree"
	"github.com/stretchr/testify/require"
)

// TestConcurrent_BTTickerAndRunJSSync reproduces the shooter game scenario:
// - BubbleTea periodically calls RunJSSync for Update
// - BT tickers periodically call BlockingJSLeaf.Tick which uses RunOnLoop
//
// This test verifies both can coexist without deadlock.
func TestConcurrent_BTTickerAndRunJSSync(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	// Verify runLeaf exists
	var runLeafExists bool
	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		val := vm.Get("runLeaf")
		runLeafExists = val != nil && !goja.IsUndefined(val)
		return nil
	})
	require.NoError(t, err)
	require.True(t, runLeafExists, "runLeaf helper should exist")
	t.Logf("runLeaf exists: %v", runLeafExists)

	// Create a blocking leaf that increments a counter
	var btTickCount atomic.Int32
	var leafCalls atomic.Int32
	var node bt.Node
	err = bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		// Create a tree matching the shooter grunt structure:
		// sequence(checkAlive, fallback(sequence(checkAlive, checkInRange, shoot), moveToward))
		val, err := vm.RunString(`
			var leafCount = 0;
			console.log("[JS] Creating grunt tree like shooter...");

			// These closures capture a shared counter like the shooter captures blackboard
			var sharedState = { value: 0 };

			var checkAlive = bt.createBlockingLeafNode(function() {
				leafCount++;
				sharedState.value++;
				return bt.success;  // always alive
			});
			var checkInRange = bt.createBlockingLeafNode(function() {
				leafCount++;
				sharedState.value++;
				return bt.failure;  // never in range
			});
			var shoot = bt.createBlockingLeafNode(function() {
				leafCount++;
				sharedState.value++;
				return bt.success;
			});
			var moveToward = bt.createBlockingLeafNode(function() {
				leafCount++;
				sharedState.value++;
				console.log("[JS] moveToward called, state.value =", sharedState.value);
				return bt.running;  // keep moving
			});

			// Attack sequence: checkAlive AND checkInRange AND shoot
			var attackSequence = bt.node(bt.sequence, checkAlive, checkInRange, shoot);

			// Main behavior: try attack, fallback to move
			var mainBehavior = bt.node(bt.fallback, attackSequence, moveToward);

			// Root: check alive, then main behavior
			// NOTE: Using a FRESH checkAlive for root, like shooter does
			var rootCheckAlive = bt.createBlockingLeafNode(function() {
				leafCount++;
				return bt.success;
			});

			console.log("[JS] Tree structure created");
			bt.node(bt.sequence, rootCheckAlive, mainBehavior);
		`)
		if err != nil {
			return err
		}
		node = val.Export().(bt.Node)
		return nil
	})
	require.NoError(t, err)
	t.Log("Grunt tree created")

	// Wrap to count ticks and leaf calls
	wrappedNode := func() (bt.Tick, []bt.Node) {
		tick, children := node()
		return func(c []bt.Node) (bt.Status, error) {
			count := btTickCount.Add(1)
			t.Logf("[Go] Tree tick #%d starting...", count)
			status, err := tick(c)
			t.Logf("[Go] Tree tick #%d completed with %v, %v", count, status, err)
			// After tree tick, get leaf count from JS
			bridge.RunOnLoop(func(vm *goja.Runtime) {
				val := vm.Get("leafCount")
				if val != nil && !goja.IsUndefined(val) {
					leafCalls.Store(int32(val.ToInteger()))
				}
			})
			return status, err
		}, children
	}

	// Create tickers (like 3 enemies) - test spawning outside event loop first
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// First, just test that a single ticker works
	t.Log("Creating single ticker outside event loop...")
	var singleTickerTicks atomic.Int32
	singleTicker := bt.NewTicker(ctx, 50*time.Millisecond, func() (bt.Tick, []bt.Node) {
		tick, children := node()
		return func(c []bt.Node) (bt.Status, error) {
			singleTickerTicks.Add(1)
			return tick(c)
		}, children
	})

	// Give it some time to tick
	time.Sleep(200 * time.Millisecond)
	t.Logf("Single ticker ticked %d times", singleTickerTicks.Load())
	singleTicker.Stop()

	// Check if any ticks happened
	if singleTickerTicks.Load() == 0 {
		t.Fatal("Single ticker never ticked - deadlock in basic case!")
	}

	var tickers []bt.Ticker
	err = bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		for i := 0; i < 3; i++ {
			ticker := bt.NewTicker(ctx, 50*time.Millisecond, wrappedNode)
			tickers = append(tickers, ticker)
		}
		return nil
	})
	require.NoError(t, err)

	// Simulate BubbleTea's Update calls
	var updateCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			err := bridge.RunJSSync(func(vm *goja.Runtime) error {
				// Simulate Update - quick JS operation
				_, err := vm.RunString(`1 + 1`)
				return err
			})
			if err != nil {
				t.Logf("RunJSSync error: %v", err)
				return
			}
			updateCount.Add(1)
			time.Sleep(16 * time.Millisecond) // ~60fps
		}
	}()

	// Wait for Updates to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatalf("Test timed out - deadlock likely. BT ticks: %d, Leaf calls: %d, Updates: %d",
			btTickCount.Load(), leafCalls.Load(), updateCount.Load())
	}

	// Stop tickers
	for _, ticker := range tickers {
		ticker.Stop()
	}

	t.Logf("BT tick count: %d, Leaf calls: %d, Update count: %d",
		btTickCount.Load(), leafCalls.Load(), updateCount.Load())

	// We should have completed 50 updates
	require.Equal(t, int32(50), updateCount.Load(), "All updates should complete")
	// BT tickers should have ticked multiple times
	require.Greater(t, btTickCount.Load(), int32(0), "BT tickers should have ticked")
}
