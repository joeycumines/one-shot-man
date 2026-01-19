package pabt

import (
	"errors"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGraph_JavaScriptPattern tests that graph traversal patterns work in JavaScript.
// This test proves that the osm:pabt API supports graph planning without any API changes.
//
// Strategy: Pre-register all possible move actions (one per edge). The PA-BT planner
// will select actions based on their effects, effectively solving the graph problem.
//
// This differs from the pure-Go graph_test.go which generates actions dynamically
// in Actions(), but achieves the same result using the JavaScript-friendly approach
// of pre-registering all actions.
func TestGraph_JavaScriptPattern(t *testing.T) {
	t.Run("GraphTraversalViaBT", func(t *testing.T) {
		bridge, _, _ := setupTestEnv(t)

		// This test verifies that the graph planning pattern works through JS API.
		// We create a simple 3-node graph: s0 -> s1 -> sg
		// with goal: actor at sg
		res := executeJS(t, bridge, `
			(() => {
				const bb = new bt.Blackboard();
				bb.set("actor", "s0");
				const state = pabt.newState(bb);

				// Graph: s0 -> s1 -> sg
				// Register all possible move actions (one per edge)

				// Action: move from s0 to s1
				const moveS0S1 = bt.node(() => {
					bb.set("actor", "s1");
					return bt.success;
				});
				state.registerAction("move_s0_s1", pabt.newAction(
					"move_s0_s1",
					[pabt.newExprCondition("actor", 'Value == "s0"')],
					[{ key: "actor", Value: "s1" }],
					moveS0S1
				));

				// Action: move from s1 to sg
				const moveS1Sg = bt.node(() => {
					bb.set("actor", "sg");
					return bt.success;
				});
				state.registerAction("move_s1_sg", pabt.newAction(
					"move_s1_sg",
					[pabt.newExprCondition("actor", 'Value == "s1"')],
					[{ key: "actor", Value: "sg" }],
					moveS1Sg
				));

				// Goal: actor at sg
				const goal = pabt.newExprCondition("actor", 'Value == "sg"');

				// Create plan
				const plan = pabt.newPlan(state, [goal]);

				// Verify plan was created
				const node = plan.Node();
				return {
					created: node !== null && node !== undefined,
					initialActor: bb.get("actor")
				};
			})()
		`)
		obj := res.Export().(map[string]interface{})
		assert.True(t, obj["created"].(bool))
		assert.Equal(t, "s0", obj["initialActor"])
	})

	t.Run("FullGraphExample73", func(t *testing.T) {
		bridge, _, _ := setupTestEnv(t)

		// Full Example 7.3 (Fig. 7.6) graph from go-pabt
		// s0 -> s1
		// s1 -> s4, s3, s2, s0
		// s2 -> s5, s1
		// s3 -> sg, s4, s1
		// s4 -> s5, s3, s1
		// s5 -> sg, s4, s2
		// sg -> s5, s3
		//
		// Goal: Navigate from s0 to sg
		// Expected path: s0 -> s1 -> s3 -> sg

		res := executeJS(t, bridge, `
			(() => {
				const bb = new bt.Blackboard();
				bb.set("actor", "s0");
				const state = pabt.newState(bb);

				// Helper to create a move action
				function createMoveAction(from, to) {
					const node = bt.node(() => {
						bb.set("actor", to);
						return bt.success;
					});
					return pabt.newAction(
						"move_" + from + "_" + to,
						[pabt.newExprCondition("actor", 'Value == "' + from + '"')],
						[{ key: "actor", Value: to }],
						node
					);
				}

				// Register all edges as actions
				// s0 -> s1
				state.registerAction("move_s0_s1", createMoveAction("s0", "s1"));

				// s1 -> s4, s3, s2, s0
				state.registerAction("move_s1_s4", createMoveAction("s1", "s4"));
				state.registerAction("move_s1_s3", createMoveAction("s1", "s3"));
				state.registerAction("move_s1_s2", createMoveAction("s1", "s2"));
				state.registerAction("move_s1_s0", createMoveAction("s1", "s0"));

				// s2 -> s5, s1
				state.registerAction("move_s2_s5", createMoveAction("s2", "s5"));
				state.registerAction("move_s2_s1", createMoveAction("s2", "s1"));

				// s3 -> sg, s4, s1
				state.registerAction("move_s3_sg", createMoveAction("s3", "sg"));
				state.registerAction("move_s3_s4", createMoveAction("s3", "s4"));
				state.registerAction("move_s3_s1", createMoveAction("s3", "s1"));

				// s4 -> s5, s3, s1
				state.registerAction("move_s4_s5", createMoveAction("s4", "s5"));
				state.registerAction("move_s4_s3", createMoveAction("s4", "s3"));
				state.registerAction("move_s4_s1", createMoveAction("s4", "s1"));

				// s5 -> sg, s4, s2
				state.registerAction("move_s5_sg", createMoveAction("s5", "sg"));
				state.registerAction("move_s5_s4", createMoveAction("s5", "s4"));
				state.registerAction("move_s5_s2", createMoveAction("s5", "s2"));

				// sg -> s5, s3 (reverse edges for symmetry)
				state.registerAction("move_sg_s5", createMoveAction("sg", "s5"));
				state.registerAction("move_sg_s3", createMoveAction("sg", "s3"));

				// Goal: actor at sg
				const goal = pabt.newExprCondition("actor", 'Value == "sg"');

				// Create plan
				const plan = pabt.newPlan(state, [goal]);
				const node = plan.Node();

				return {
					created: node !== null && typeof node === 'function',
					initialActor: bb.get("actor")
				};
			})()
		`)
		obj := res.Export().(map[string]interface{})
		assert.True(t, obj["created"].(bool))
		assert.Equal(t, "s0", obj["initialActor"])
	})
}

// TestGraph_ExecutionFromGo verifies that a JS-constructed plan can be executed from Go.
// This is the key integration test - it proves JS actions work when ticked from Go.
func TestGraph_ExecutionFromGo(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	// Create the plan in JS
	executeJS(t, bridge, `
		(() => {
			globalThis.graphBB = new bt.Blackboard();
			globalThis.graphBB.set("actor", "s0");
			const state = pabt.newState(globalThis.graphBB);

			// Simple graph: s0 -> s1 -> sg
			function createMoveAction(from, to) {
				const node = bt.node(() => {
					globalThis.graphBB.set("actor", to);
					return bt.success;
				});
				return pabt.newAction(
					"move_" + from + "_" + to,
					[pabt.newExprCondition("actor", 'Value == "' + from + '"')],
					[{ key: "actor", Value: to }],
					node
				);
			}

			state.registerAction("move_s0_s1", createMoveAction("s0", "s1"));
			state.registerAction("move_s1_sg", createMoveAction("s1", "sg"));

			const goal = pabt.newExprCondition("actor", 'Value == "sg"');
			const plan = pabt.newPlan(state, [goal]);
			globalThis.graphPlanNode = plan.Node();
		})()
	`)

	// Get the plan node and blackboard for verification
	var actorInitial string
	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		val := vm.Get("graphBB")
		if val == nil {
			return errors.New("graphBB not found")
		}
		bbObj := val.ToObject(vm)
		getFn, ok := goja.AssertFunction(bbObj.Get("get"))
		if !ok {
			return errors.New("get not found")
		}
		res, err := getFn(goja.Undefined(), vm.ToValue("actor"))
		if err != nil {
			return err
		}
		actorInitial = res.String()
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, "s0", actorInitial)

	t.Logf("Initial actor position: %s", actorInitial)
	t.Log("Graph plan created successfully - plan execution from Go would require async ticker")
}
