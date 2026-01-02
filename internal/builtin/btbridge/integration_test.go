package btbridge

import (
	"context"
	"testing"
	"time"

	bt "github.com/joeycumines/go-behaviortree"
	"github.com/stretchr/testify/require"
)

// TestIntegration_GoCompositeWithJSLeaves demonstrates the Go-Centric architecture:
// go-behaviortree composites (Sequence, Selector) with JavaScript leaf behaviors.
func TestIntegration_GoCompositeWithJSLeaves(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	defer bridge.Stop()

	bb := NewBlackboard()
	bb.Set("health", 100)
	bb.Set("hasTarget", true)

	err = bridge.ExposeBlackboard("ctx", bb)
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

	hasTargetNode := BlockingJSLeaf(bridge, "hasTarget", nil)
	isHealthyNode := BlockingJSLeaf(bridge, "isHealthy", nil)
	attackNode := BlockingJSLeaf(bridge, "attack", nil)
	healNode := BlockingJSLeaf(bridge, "heal", nil)

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

	bridge, err := NewBridge()
	require.NoError(t, err)
	defer bridge.Stop()

	bb := NewBlackboard()
	bb.Set("counter", 0)

	err = bridge.ExposeBlackboard("ctx", bb)
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
	counterNode := BlockingJSLeaf(bridge, "incrementCounter", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Create a ticker that ticks every 10ms
	ticker := bt.NewTicker(ctx, 10*time.Millisecond, counterNode)

	// Wait for completion (context timeout)
	<-ticker.Done()

	// NewTicker reports context deadline exceeded as error (expected behavior)
	// We just verify the counter was incremented before the timeout
	count := bb.Get("counter").(int64)
	require.Greater(t, count, int64(0), "counter should have been incremented")
}

// TestIntegration_Memorize demonstrates using bt.Memorize with async JS leaves
func TestIntegration_Memorize(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	defer bridge.Stop()

	bb := NewBlackboard()
	bb.Set("step1_count", 0)
	bb.Set("step2_count", 0)

	err = bridge.ExposeBlackboard("ctx", bb)
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

	step1Node := NewJSLeafAdapter(bridge, "step1", nil)
	step2Node := NewJSLeafAdapter(bridge, "step2", nil)

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

	// Poll until completion (wait for JS promises)
	for i := 0; i < 50; i++ {
		time.Sleep(10 * time.Millisecond)
		status, err = tree.Tick()
		require.NoError(t, err)
		if status != bt.Running {
			break
		}
	}

	// Eventually should succeed
	require.Equal(t, bt.Success, status)

	// Check counts - step1 should be 2, step2 should be 2
	require.Equal(t, int64(2), bb.Get("step1_count"))
	require.Equal(t, int64(2), bb.Get("step2_count"))
}

// TestIntegration_Fork demonstrates using bt.Fork with parallel JS leaves
func TestIntegration_Fork(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	defer bridge.Stop()

	bb := NewBlackboard()

	err = bridge.ExposeBlackboard("ctx", bb)
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

	task1 := BlockingJSLeaf(bridge, "task1", nil)
	task2 := BlockingJSLeaf(bridge, "task2", nil)
	task3 := BlockingJSLeaf(bridge, "task3", nil)

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
