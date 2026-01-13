package bt

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	gojarequire "github.com/dop251/goja_nodejs/require"
	bt "github.com/joeycumines/go-behaviortree"
	"github.com/stretchr/testify/require"
)

// waitForTreeCompletion polls the tree until it returns a non-Running status or timeout.
func waitForTreeCompletion(t *testing.T, tree bt.Node, timeout time.Duration) (bt.Status, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatal("timeout waiting for tree completion")
			return bt.Failure, ctx.Err()
		case <-ticker.C:
			status, err := tree.Tick()
			if status != bt.Running || err != nil {
				return status, err
			}
		}
	}
}

// TestIntegration_GoCompositeWithJSLeaves demonstrates the Go-Centric architecture:
// go-behaviortree composites (Sequence, Selector) with JavaScript leaf behaviors.
func TestIntegration_GoCompositeWithJSLeaves(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)
	bb := new(Blackboard)
	bb.Set("health", 100)
	bb.Set("hasTarget", true)

	err := bridge.ExposeBlackboard("ctx", bb)
	require.NoError(t, err)

	// Load JavaScript leaf behaviors
	err = bridge.LoadScript("leaves.js", `
		// Condition: Check if we have a target
		async function hasTarget() {
			return ctx.get("hasTarget") ? bt.success : bt.failure;
		}

		// Condition: Check if health is high enough
		async function isHealthy() {
			return ctx.get("health") > 50 ? bt.success : bt.failure;
		}

		// Action: Attack the target
		async function attack() {
			var damage = ctx.get("damage") || 0;
			ctx.set("damage", damage + 10);
			return bt.success;
		}

		// Action: Heal self
		async function heal() {
			var health = ctx.get("health") || 0;
			ctx.set("health", health + 20);
			return bt.success;
		}

		// Action: Retreat
		async function retreat() {
			ctx.set("retreated", true);
			return bt.success;
		}
	`)
	require.NoError(t, err)

	// Build the behavior tree using Go composites with JS leaves
	// Tree structure:
	// Selector (try in order until one succeeds)
	//   Sequence (attack if has target and healthy)
	//     hasTarget (JS leaf)
	//     isHealthy (JS leaf)
	//     attack (JS leaf)
	//   heal (fallback)

	hasTargetFn, err := bridge.GetCallable("hasTarget")
	require.NoError(t, err)
	hasTargetNode := blockingJSLeaveNoVM(context.TODO(), bridge, hasTargetFn, nil)

	isHealthyFn, err := bridge.GetCallable("isHealthy")
	require.NoError(t, err)
	isHealthyNode := blockingJSLeaveNoVM(context.TODO(), bridge, isHealthyFn, nil)

	attackFn, err := bridge.GetCallable("attack")
	require.NoError(t, err)
	attackNode := blockingJSLeaveNoVM(context.TODO(), bridge, attackFn, nil)

	healFn, err := bridge.GetCallable("heal")
	require.NoError(t, err)
	healNode := blockingJSLeaveNoVM(context.TODO(), bridge, healFn, nil)

	tree := bt.New(
		bt.Selector,
		// Attack sequence
		bt.New(
			bt.Sequence,
			hasTargetNode,
			isHealthyNode,
			attackNode,
		),
		// Fallback: heal
		healNode,
	)

	// Tick the tree - should succeed via the attack sequence
	status, err := tree.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Success, status)

	// Check the attack was executed
	require.Equal(t, int64(10), bb.Get("damage"))

	// Now test the heal path - set health low
	bb.Set("health", 30)

	status, err = tree.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Success, status)

	// isHealthy failed, so heal should have triggered via Selector fallback
	require.Equal(t, int64(50), bb.Get("health"))
}

// TestIntegration_AsyncJSLeafWithTicker demonstrates using JS leaves with bt.Ticker
func TestIntegration_AsyncJSLeafWithTicker(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)
	bb := new(Blackboard)
	bb.Set("counter", 0)

	err := bridge.ExposeBlackboard("ctx", bb)
	require.NoError(t, err)

	// A leaf that increments counter and succeeds (always succeeds, no running)
	err = bridge.LoadScript("counter.js", `
		async function incrementCounter() {
			var count = ctx.get("counter") || 0;
			count++;
			ctx.set("counter", count);
			// Success immediately - ticker will keep ticking
			return bt.success;
		}
	`)
	require.NoError(t, err)

	// Use blocking leaf for ticker (simpler and more predictable)
	incrementCounterFn, err := bridge.GetCallable("incrementCounter")
	require.NoError(t, err)
	counterNode := blockingJSLeaveNoVM(context.TODO(), bridge, incrementCounterFn, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Create a ticker that ticks every 10ms
	ticker := bt.NewTicker(ctx, 10*time.Millisecond, counterNode)

	// Wait for completion (context timeout)
	<-ticker.Done()

	// NewTicker reports context deadline exceeded as error (expected behavior)
	// We just verify the counter was incremented before the timeout
	countVal := bb.Get("counter")
	require.NotNil(t, countVal, "counter should have been set by JS")
	count, ok := countVal.(int64)
	require.True(t, ok, "counter should be int64, got %T", countVal)
	require.Greater(t, count, int64(0), "counter should have been incremented")
}

// TestIntegration_Memorize demonstrates using bt.Memorize with async JS leaves
func TestIntegration_Memorize(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)
	bb := new(Blackboard)
	bb.Set("step1_count", 0)
	bb.Set("step2_count", 0)

	err := bridge.ExposeBlackboard("ctx", bb)
	require.NoError(t, err)

	// Steps that run multiple times before succeeding
	err = bridge.LoadScript("steps.js", `
		async function step1() {
			var count = (ctx.get("step1_count") || 0) + 1;
			ctx.set("step1_count", count);
			return count >= 2 ? bt.success : bt.running;
		}

		async function step2() {
			var count = (ctx.get("step2_count") || 0) + 1;
			ctx.set("step2_count", count);
			return count >= 2 ? bt.success : bt.running;
		}
	`)
	require.NoError(t, err)

	step1Fn, err := bridge.GetCallable("step1")
	require.NoError(t, err)
	step1Node := NewJSLeafAdapter(context.TODO(), bridge, step1Fn, nil)

	step2Fn, err := bridge.GetCallable("step2")
	require.NoError(t, err)
	step2Node := NewJSLeafAdapter(context.TODO(), bridge, step2Fn, nil)

	// Without Memorize, each tick of the sequence would re-tick step1
	// With Memorize, once step1 succeeds, it's cached until the sequence completes
	tree := bt.New(
		bt.Memorize(bt.Sequence),
		step1Node,
		step2Node,
	)

	// First tick: step1 starts (returns Running)
	status, err := tree.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Running, status)

	// Wait for completion using helper
	status, err = waitForTreeCompletion(t, tree, time.Second)
	require.NoError(t, err)
	require.Equal(t, bt.Success, status)

	// Check counts - step1 should be 2, step2 should be 2
	require.Equal(t, int64(2), bb.Get("step1_count"))
	require.Equal(t, int64(2), bb.Get("step2_count"))
}

// TestIntegration_Fork demonstrates using bt.Fork with parallel JS leaves
func TestIntegration_Fork(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)
	bb := new(Blackboard)

	err := bridge.ExposeBlackboard("ctx", bb)
	require.NoError(t, err)

	// Three tasks that can run in parallel
	err = bridge.LoadScript("parallel.js", `
		async function task1() {
			ctx.set("task1", true);
			return bt.success;
		}

		async function task2() {
			ctx.set("task2", true);
			return bt.success;
		}

		async function task3() {
			ctx.set("task3", true);
			return bt.success;
		}
	`)
	require.NoError(t, err)

	task1Fn, err := bridge.GetCallable("task1")
	require.NoError(t, err)
	task1 := blockingJSLeaveNoVM(context.TODO(), bridge, task1Fn, nil)

	task2Fn, err := bridge.GetCallable("task2")
	require.NoError(t, err)
	task2 := blockingJSLeaveNoVM(context.TODO(), bridge, task2Fn, nil)

	task3Fn, err := bridge.GetCallable("task3")
	require.NoError(t, err)
	task3 := blockingJSLeaveNoVM(context.TODO(), bridge, task3Fn, nil)

	// Fork runs all children and waits for all to complete
	tree := bt.New(
		bt.Fork(),
		task1,
		task2,
		task3,
	)

	status, err := tree.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Success, status)

	// All tasks should have completed
	require.True(t, bb.Get("task1").(bool))
	require.True(t, bb.Get("task2").(bool))
	require.True(t, bb.Get("task3").(bool))
}

// TestIntegration_SharedModeManagerShutdown verifies CRITICAL #1 fix:
// Manager.done() promise resolves without hanging when bridge is stopped (shared mode)
func TestIntegration_SharedModeManagerShutdown(t *testing.T) {
	t.Parallel()

	// Create external event loop (shared mode owner)
	reg := gojarequire.NewRegistry()
	loop := eventloop.NewEventLoop(eventloop.WithRegistry(reg))
	loop.Start()

	// Create bridge in shared mode (does NOT own the loop)
	ctx := context.Background()
	bridge := NewBridgeWithEventLoop(ctx, loop, reg)
	defer func() {
		bridge.Stop()
		loop.Stop() // We own the loop, so we must stop it
	}()

	// Create a Manager via bt.newManager()
	err := bridge.LoadScript("manager.js", `
		const bt = require('osm:bt');
		// Create a ticker that keeps running (returns "running" forever)
		globalThis.testTicker = bt.newTicker(50, bt.node(() => "running"));
		// Create a manager and add the ticker to it
		globalThis.testManager = bt.newManager();
		globalThis.testManager.add(testTicker);
	`)
	require.NoError(t, err)

	// Channel for promise settlement notification
	promiseSettled := make(chan struct{}, 1)
	var promiseError error

	// Attach .then() callback with Go-native notification
	err = bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		managerObj := vm.Get("testManager").ToObject(vm)
		doneFn := managerObj.Get("done")
		doneCallable, ok := goja.AssertFunction(doneFn)
		if !ok {
			return errors.New("done is not callable")
		}
		promiseVal, err := doneCallable(managerObj)
		if err != nil {
			return fmt.Errorf("calling done() failed: %w", err)
		}

		// Attach .then() callback
		promiseObj := promiseVal.ToObject(vm)
		thenProp := promiseObj.Get("then")
		thenCallable, ok := goja.AssertFunction(thenProp)
		if !ok {
			return errors.New("promise does not have callable then()")
		}

		// Create Go callback that notifies when promise settles
		onFulfilled := func(call goja.FunctionCall) goja.Value {
			// Success case - promise resolved
			select {
			case promiseSettled <- struct{}{}:
			default:
			}
			return goja.Undefined()
		}

		onRejected := func(call goja.FunctionCall) goja.Value {
			// Rejection case - extract error
			errMsg := call.Argument(0).String()
			promiseError = errors.New(errMsg)
			select {
			case promiseSettled <- struct{}{}:
			default:
			}
			return goja.Undefined()
		}

		_, err = thenCallable(promiseVal, vm.ToValue(onFulfilled), vm.ToValue(onRejected))
		return err
	})
	require.NoError(t, err)

	// Give the ticker a chance to start running
	time.Sleep(100 * time.Millisecond)

	// Stop the BRIDGE first (while manager and ticker are still active)
	// This simulates shutdown where bridge.stopped = true but loop is still alive
	bridge.Stop()

	// Now stop the manager DIRECTLY on the loop (using channel for sync)
	// The background goroutine will receive manager.Done() closing and
	// try bridge.RunOnLoop() (which fails due to bridge stopped).
	// The fallback mechanism should trigger and call bridge.loop.RunOnLoop()
	// to settle the promise.
	stopCh := make(chan error, 1)
	scheduled := loop.RunOnLoop(func(vm *goja.Runtime) {
		managerObj := vm.Get("testManager").ToObject(vm)
		stopFn := managerObj.Get("stop")
		stopCallable, ok := goja.AssertFunction(stopFn)
		if !ok {
			stopCh <- errors.New("stop is not callable")
			return
		}
		_, stopErr := stopCallable(managerObj)
		stopCh <- stopErr
	})
	require.True(t, scheduled, "Failed to schedule manager.stop() on loop")
	select {
	case stopErr := <-stopCh:
		require.NoError(t, stopErr)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout stopping manager on loop")
	}

	// Wait for promise to settle with timeout
	// The fallback mechanism MUST be triggered because:
	// 1. bridge.Stop() sets b.stopped = true
	// 2. manager.Stop() triggers Done() closure
	// 3. Background goroutine tries bridge.RunOnLoop (fails)
	// 4. Fallback to bridge.loop.RunOnLoop must work
	select {
	case <-promiseSettled:
		if promiseError != nil {
			t.Logf("Promise rejected with error: %v", promiseError)
		} else {
			t.Log("Shared mode manager shutdown test passed - fallback mechanism worked, promise settled with bridge stopped")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("TIMEOUT: Promise did not settle within 2 seconds - fatal: fallback mechanism FAILED!")
	}
}

// TestIntegration_SharedModeTickerShutdown verifies createTickerJSWrapper fallback
func TestIntegration_SharedModeTickerShutdown(t *testing.T) {
	t.Parallel()

	// Create external event loop (shared mode owner)
	reg := gojarequire.NewRegistry()
	loop := eventloop.NewEventLoop(eventloop.WithRegistry(reg))
	loop.Start()

	// Create bridge in shared mode
	ctx := context.Background()
	bridge := NewBridgeWithEventLoop(ctx, loop, reg)
	defer func() {
		bridge.Stop()
		loop.Stop()
	}()

	// Create a ticker
	err := bridge.LoadScript("ticker.js", `
		const bt = require('osm:bt');
		globalThis.testTicker = bt.newTicker(100, bt.node(() => "running"));
	`)
	require.NoError(t, err)

	// Channel for promise settlement notification
	promiseSettled := make(chan struct{}, 1)
	var promiseError error

	// Get the ticker's done promise and attach .then() callback with Go notification
	err = bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		tickerObj := vm.Get("testTicker").ToObject(vm)
		doneFn := tickerObj.Get("done")
		doneCallable, ok := goja.AssertFunction(doneFn)
		if !ok {
			return errors.New("done is not callable")
		}
		promiseVal, err := doneCallable(tickerObj)
		if err != nil {
			return fmt.Errorf("calling done() failed: %w", err)
		}

		// Attach .then() callback
		promiseObj := promiseVal.ToObject(vm)
		thenProp := promiseObj.Get("then")
		thenCallable, ok := goja.AssertFunction(thenProp)
		if !ok {
			return errors.New("promise does not have callable then()")
		}

		// Create Go callback that notifies when promise settles
		onFulfilled := func(call goja.FunctionCall) goja.Value {
			// Success case - promise resolved
			select {
			case promiseSettled <- struct{}{}:
			default:
			}
			return goja.Undefined()
		}

		onRejected := func(call goja.FunctionCall) goja.Value {
			// Rejection case - extract error
			errMsg := call.Argument(0).String()
			promiseError = errors.New(errMsg)
			select {
			case promiseSettled <- struct{}{}:
			default:
			}
			return goja.Undefined()
		}

		_, err = thenCallable(promiseVal, vm.ToValue(onFulfilled), vm.ToValue(onRejected))
		return err
	})
	require.NoError(t, err)

	// Stop the bridge while ticker is still running
	// This should cause the manager to stop, which stops the ticker
	bridge.Stop()

	// Wait for promise to settle with timeout
	select {
	case <-promiseSettled:
		if promiseError != nil {
			t.Logf("Promise rejected with error (expected in shutdown): %v", promiseError)
		}
		t.Log("Shared mode ticker shutdown test passed - promise settled correctly")
	case <-time.After(2 * time.Second):
		t.Fatal("TIMEOUT: Promise did not settle within 2 seconds - hanging promise detected!")
	}
}
