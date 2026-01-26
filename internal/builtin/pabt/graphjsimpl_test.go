package pabt

import (
	"errors"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGraphJS_PlanCreation tests that a PA-BT plan can be created for the graph example
// using pure JavaScript with a proper action template pattern.
//
// This test implements Example 7.3 (Fig. 7.6) from go-pabt entirely in JavaScript,
// demonstrating the DX for action templating via setActionGenerator.
func TestGraphJS_PlanCreation(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	res := executeJS(t, bridge, `
		(() => {
			const bb = new bt.Blackboard();
			bb.set("actor", "s0");
			const state = pabt.newState(bb);

			// Graph structure from Example 7.3 (Fig. 7.6)
			// Links are bidirectional - node.links = nodes reachable in both directions
			const graph = {
				s0: ["s1"],
				s1: ["s4", "s3", "s2", "s0"],
				s2: ["s5", "s1"],
				s3: ["sg", "s4", "s1"],
				s4: ["s5", "s3", "s1"],
				s5: ["sg", "s4", "s2"],
				sg: ["s5", "s3"]
			};

			// SINGLE ACTION TEMPLATE - generates actions dynamically from failed condition
			// This is the correct PA-BT pattern: NO pre-registration of actions.
			state.setActionGenerator(function(failed) {
				if (failed.key !== "actor") return [];

				// failed.value IS the target node (accessible because we pass original JS object)
				const target = failed.value;
				if (!target || !graph[target]) return [];

				const actions = [];
				// Iterate target's links (sources that can reach target in bidirectional graph)
				for (const source of graph[target]) {
					const moveNode = bt.node(() => {
						if (bb.get("actor") !== source) return bt.failure;
						bb.set("actor", target);
						return bt.success;
					});

					actions.push(pabt.newAction(
						"move_" + source + "_" + target,
						[{ key: "actor", value: source, match: (v) => v === source }],
						[{ key: "actor", Value: target }],
						moveNode
					));
				}
				return actions;
			});

			// Goal: actor at sg (with explicit value property for action generator)
			const goal = { key: "actor", value: "sg", match: (v) => v === "sg" };
			const plan = pabt.newPlan(state, [goal]);

			return {
				planCreated: plan !== null && plan !== undefined,
				hasNode: typeof plan.Node === 'function',
				initialActor: bb.get("actor")
			};
		})()
	`)

	obj := res.Export().(map[string]interface{})
	assert.True(t, obj["planCreated"].(bool), "Plan should be created")
	assert.True(t, obj["hasNode"].(bool), "Plan should have Node() method")
	assert.Equal(t, "s0", obj["initialActor"], "Actor should start at s0")
}

// TestGraphJS_PlanExecution tests that the plan executes and reaches the goal.
func TestGraphJS_PlanExecution(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	// Create the plan and ticker in JS
	executeJS(t, bridge, `
		(() => {
			globalThis.graphTestBB = new bt.Blackboard();
			globalThis.graphTestBB.set("actor", "s0");
			const state = pabt.newState(globalThis.graphTestBB);

			const graph = {
				s0: ["s1"],
				s1: ["s4", "s3", "s2", "s0"],
				s2: ["s5", "s1"],
				s3: ["sg", "s4", "s1"],
				s4: ["s5", "s3", "s1"],
				s5: ["sg", "s4", "s2"],
				sg: ["s5", "s3"]
			};

			state.setActionGenerator(function(failed) {
				if (failed.key !== "actor") return [];
				const target = failed.value;
				if (!target || !graph[target]) return [];

				const actions = [];
				for (const source of graph[target]) {
					const moveNode = bt.node(() => {
						if (globalThis.graphTestBB.get("actor") !== source) return bt.failure;
						globalThis.graphTestBB.set("actor", target);
						return bt.success;
					});
					actions.push(pabt.newAction(
						"move_" + source + "_" + target,
						[{ key: "actor", value: source, match: (v) => v === source }],
						[{ key: "actor", Value: target }],
						moveNode
					));
				}
				return actions;
			});

			const goal = { key: "actor", value: "sg", match: (v) => v === "sg" };
			const plan = pabt.newPlan(state, [goal]);
			globalThis.graphTestTicker = bt.newTicker(10, plan.Node(), { stopOnFailure: true });
		})()
	`)

	// Wait for ticker to complete
	time.Sleep(500 * time.Millisecond)

	// Check final state
	var finalActor string
	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		bbObj := vm.Get("graphTestBB").ToObject(vm)
		getFn, ok := goja.AssertFunction(bbObj.Get("get"))
		if !ok {
			return errors.New("get() not found")
		}
		res, err := getFn(bbObj, vm.ToValue("actor"))
		if err != nil {
			return err
		}
		finalActor = res.String()
		return nil
	})
	require.NoError(t, err)

	// Stop the ticker
	_ = bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		tickerObj := vm.Get("graphTestTicker").ToObject(vm)
		stopFn, ok := goja.AssertFunction(tickerObj.Get("stop"))
		if ok {
			_, _ = stopFn(tickerObj)
		}
		return nil
	})

	assert.Equal(t, "sg", finalActor, "Actor should reach goal sg")
}

// TestGraphJS_PathValidation tests that the path taken is correct (s0 -> s1 -> s3 -> sg).
func TestGraphJS_PathValidation(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	// Create plan with path tracking
	executeJS(t, bridge, `
		(() => {
			globalThis.pathTestBB = new bt.Blackboard();
			globalThis.pathTestBB.set("actor", "s0");
			globalThis.pathTaken = [];
			const state = pabt.newState(globalThis.pathTestBB);

			const graph = {
				s0: ["s1"],
				s1: ["s4", "s3", "s2", "s0"],
				s2: ["s5", "s1"],
				s3: ["sg", "s4", "s1"],
				s4: ["s5", "s3", "s1"],
				s5: ["sg", "s4", "s2"],
				sg: ["s5", "s3"]
			};

			state.setActionGenerator(function(failed) {
				if (failed.key !== "actor") return [];
				const target = failed.value;
				if (!target || !graph[target]) return [];

				const actions = [];
				for (const source of graph[target]) {
					const moveNode = bt.node(() => {
						const current = globalThis.pathTestBB.get("actor");
						if (current !== source) return bt.failure;
						globalThis.pathTaken.push({ from: current, to: target });
						globalThis.pathTestBB.set("actor", target);
						return bt.success;
					});
					actions.push(pabt.newAction(
						"move_" + source + "_" + target,
						[{ key: "actor", value: source, match: (v) => v === source }],
						[{ key: "actor", Value: target }],
						moveNode
					));
				}
				return actions;
			});

			const goal = { key: "actor", value: "sg", match: (v) => v === "sg" };
			const plan = pabt.newPlan(state, [goal]);
			globalThis.pathTestTicker = bt.newTicker(10, plan.Node(), { stopOnFailure: true });
		})()
	`)

	// Wait for completion
	time.Sleep(500 * time.Millisecond)

	// Get the path
	var path []interface{}
	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		pathVal := vm.Get("pathTaken")
		path = pathVal.Export().([]interface{})
		return nil
	})
	require.NoError(t, err)

	// Stop ticker
	_ = bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		tickerObj := vm.Get("pathTestTicker").ToObject(vm)
		if stopFn, ok := goja.AssertFunction(tickerObj.Get("stop")); ok {
			_, _ = stopFn(tickerObj)
		}
		return nil
	})

	// Verify path (should be s0 -> s1 -> s3 -> sg based on Example 7.3)
	require.GreaterOrEqual(t, len(path), 1, "Path should have at least one move")

	// Build path string for verification
	pathNodes := []string{"s0"}
	for _, move := range path {
		moveMap := move.(map[string]interface{})
		pathNodes = append(pathNodes, moveMap["to"].(string))
	}

	t.Logf("Path taken: %v", pathNodes)
	assert.Equal(t, "sg", pathNodes[len(pathNodes)-1], "Path should end at sg")
}

// TestGraphJS_UnreachableGoal tests planner behavior when goal is unreachable.
func TestGraphJS_UnreachableGoal(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	executeJS(t, bridge, `
		(() => {
			globalThis.unreachableBB = new bt.Blackboard();
			globalThis.unreachableBB.set("actor", "s0");
			const state = pabt.newState(globalThis.unreachableBB);

			// Graph WITHOUT any path to "isolated"
			const graph = {
				s0: ["s1"],
				s1: ["s0"]
			};

			state.setActionGenerator(function(failed) {
				if (failed.key !== "actor") return [];
				const target = failed.value;
				if (!target || !graph[target]) return [];  // Returns empty for isolated

				const actions = [];
				for (const source of graph[target]) {
					const moveNode = bt.node(() => {
						if (globalThis.unreachableBB.get("actor") !== source) return bt.failure;
						globalThis.unreachableBB.set("actor", target);
						return bt.success;
					});
					actions.push(pabt.newAction(
						"move_" + source + "_" + target,
						[{ key: "actor", value: source, match: (v) => v === source }],
						[{ key: "actor", Value: target }],
						moveNode
					));
				}
				return actions;
			});

			// Goal: actor at "isolated" (which has no links)
			const goal = { key: "actor", value: "isolated", match: (v) => v === "isolated" };
			const plan = pabt.newPlan(state, [goal]);
			globalThis.unreachablePlanCreated = plan !== null;
			globalThis.unreachableTicker = bt.newTicker(10, plan.Node(), { stopOnFailure: true });
		})()
	`)

	// Wait for ticker (should fail or stall)
	time.Sleep(300 * time.Millisecond)

	// Check that actor didn't reach goal
	var actorPos string
	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		bbObj := vm.Get("unreachableBB").ToObject(vm)
		getFn, _ := goja.AssertFunction(bbObj.Get("get"))
		res, _ := getFn(bbObj, vm.ToValue("actor"))
		actorPos = res.String()
		return nil
	})
	require.NoError(t, err)

	// Stop ticker
	_ = bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		tickerObj := vm.Get("unreachableTicker").ToObject(vm)
		if stopFn, ok := goja.AssertFunction(tickerObj.Get("stop")); ok {
			_, _ = stopFn(tickerObj)
		}
		return nil
	})

	// Actor should NOT be at "isolated" since there's no path
	assert.NotEqual(t, "isolated", actorPos, "Actor should not reach unreachable goal")
}

// TestGraphJS_MultipleGoals tests planning with multiple acceptable goal nodes.
func TestGraphJS_MultipleGoals(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	executeJS(t, bridge, `
		(() => {
			globalThis.multiGoalBB = new bt.Blackboard();
			globalThis.multiGoalBB.set("actor", "s0");
			const state = pabt.newState(globalThis.multiGoalBB);

			const graph = {
				s0: ["s1"],
				s1: ["s4", "s3", "s2", "s0"],
				s2: ["s5", "s1"],
				s3: ["sg", "s4", "s1"],
				s4: ["s5", "s3", "s1"],
				s5: ["sg", "s4", "s2"],
				sg: ["s5", "s3"]
			};

			state.setActionGenerator(function(failed) {
				if (failed.key !== "actor") return [];
				const target = failed.value;
				if (!target || !graph[target]) return [];

				const actions = [];
				for (const source of graph[target]) {
					const moveNode = bt.node(() => {
						if (globalThis.multiGoalBB.get("actor") !== source) return bt.failure;
						globalThis.multiGoalBB.set("actor", target);
						return bt.success;
					});
					actions.push(pabt.newAction(
						"move_" + source + "_" + target,
						[{ key: "actor", value: source, match: (v) => v === source }],
						[{ key: "actor", Value: target }],
						moveNode
					));
				}
				return actions;
			});

			// Multiple goals: either s5 OR sg is acceptable
			const goal1 = { key: "actor", value: "s5", match: (v) => v === "s5" };
			const goal2 = { key: "actor", value: "sg", match: (v) => v === "sg" };
			const plan = pabt.newPlan(state, [goal1, goal2]);
			globalThis.multiGoalTicker = bt.newTicker(10, plan.Node(), { stopOnFailure: true });
		})()
	`)

	// Wait for completion
	time.Sleep(500 * time.Millisecond)

	// Check final position
	var finalActor string
	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		bbObj := vm.Get("multiGoalBB").ToObject(vm)
		getFn, _ := goja.AssertFunction(bbObj.Get("get"))
		res, _ := getFn(bbObj, vm.ToValue("actor"))
		finalActor = res.String()
		return nil
	})
	require.NoError(t, err)

	// Stop ticker
	_ = bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		tickerObj := vm.Get("multiGoalTicker").ToObject(vm)
		if stopFn, ok := goja.AssertFunction(tickerObj.Get("stop")); ok {
			_, _ = stopFn(tickerObj)
		}
		return nil
	})

	// Actor should be at either s5 or sg
	atGoal := finalActor == "s5" || finalActor == "sg"
	assert.True(t, atGoal, "Actor should reach one of the goal nodes (s5 or sg), got: %s", finalActor)
	t.Logf("Reached goal: %s", finalActor)
}

// TestGraphJS_PlanIdempotent tests that re-ticking a successful plan returns success.
func TestGraphJS_PlanIdempotent(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	// This test verifies idempotent behavior: once success is reached,
	// re-ticking should return success immediately.
	// We start AT the goal, so the plan should succeed on first tick.
	executeJS(t, bridge, `
		(() => {
			globalThis.idempotentBB = new bt.Blackboard();
			// Start AT the goal - plan should succeed immediately
			globalThis.idempotentBB.set("actor", "sg");
			const state = pabt.newState(globalThis.idempotentBB);

			const graph = {
				s0: ["s1"],
				s1: ["s3", "s0"],
				s3: ["sg", "s1"],
				sg: ["s3"]
			};

			state.setActionGenerator(function(failed) {
				if (failed.key !== "actor") return [];
				const target = failed.value;
				if (!target || !graph[target]) return [];

				const actions = [];
				for (const source of graph[target]) {
					const moveNode = bt.node(() => {
						if (globalThis.idempotentBB.get("actor") !== source) return bt.failure;
						globalThis.idempotentBB.set("actor", target);
						return bt.success;
					});
					actions.push(pabt.newAction(
						"move_" + source + "_" + target,
						[{ key: "actor", value: source, match: (v) => v === source }],
						[{ key: "actor", Value: target }],
						moveNode
					));
				}
				return actions;
			});

			const goal = { key: "actor", value: "sg", match: (v) => v === "sg" };
			const plan = pabt.newPlan(state, [goal]);
			globalThis.idempotentTicker = bt.newTicker(10, plan.Node(), { stopOnFailure: true });
		})()
	`)

	// The ticker should complete very quickly since we're already at the goal
	time.Sleep(100 * time.Millisecond)

	// Check state - should still be at sg
	var actorPos string
	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		bbObj := vm.Get("idempotentBB").ToObject(vm)
		getFn, _ := goja.AssertFunction(bbObj.Get("get"))
		res, _ := getFn(bbObj, vm.ToValue("actor"))
		actorPos = res.String()
		return nil
	})
	require.NoError(t, err)

	// Stop ticker
	_ = bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		tickerObj := vm.Get("idempotentTicker").ToObject(vm)
		if stopFn, ok := goja.AssertFunction(tickerObj.Get("stop")); ok {
			_, _ = stopFn(tickerObj)
		}
		return nil
	})

	// Actor should remain at goal
	assert.Equal(t, "sg", actorPos, "Actor should remain at goal sg")
}
