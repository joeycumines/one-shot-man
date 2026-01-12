package bt

import (
	"errors"
	"testing"

	"github.com/dop251/goja"
	bt "github.com/joeycumines/go-behaviortree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestEnv initializes a Bridge and a JS environment with the osm:bt module required.
// It returns the bridge, the VM, and the 'bt' module object exposed in the JS global scope.
func setupTestEnv(t *testing.T) (*Bridge, *goja.Runtime, *goja.Object) {
	b := testBridge(t)

	var vm *goja.Runtime
	var btObj *goja.Object

	// Initialize the JS environment on the loop
	err := b.RunOnLoopSync(func(runtime *goja.Runtime) error {
		vm = runtime
		// Require the module and expose it as global 'bt' for easier testing
		_, err := vm.RunString(`
			const bt = require('osm:bt');
			globalThis.bt = bt;
		`)
		if err != nil {
			return err
		}

		val := vm.Get("bt")
		if val == nil || goja.IsNull(val) || goja.IsUndefined(val) {
			return errors.New("failed to require osm:bt")
		}
		btObj = val.ToObject(vm)
		return nil
	})
	require.NoError(t, err, "Failed to initialize JS environment")

	return b, vm, btObj
}

// executeJS executes a JS string and returns the result export.
func executeJS(t *testing.T, b *Bridge, script string) goja.Value {
	var res goja.Value
	err := b.RunOnLoopSync(func(vm *goja.Runtime) error {
		var err error
		res, err = vm.RunString(script)
		return err
	})
	require.NoError(t, err, "JS execution failed")
	return res
}

// TestRequire_ModuleRegistration verifies that the 'osm:bt' module is correctly registered
// and exposed with the expected API surface.
func TestRequire_ModuleRegistration(t *testing.T) {
	_, _, btObj := setupTestEnv(t)

	// Verify all expected exports exist
	expectedExports := []string{
		"running", "success", "failure",
		"node", "tick", "sequence", "fallback", "selector",
		"memorize", "async", "not", "fork",
		"interval", "createLeafNode", "createBlockingLeafNode",
		"Blackboard", "newTicker",
	}

	for _, export := range expectedExports {
		assert.NotNil(t, btObj.Get(export), "Missing export: %s", export)
	}

	// Verify Constants
	assert.Equal(t, JSStatusRunning, btObj.Get("running").String())
	assert.Equal(t, JSStatusSuccess, btObj.Get("success").String())
	assert.Equal(t, JSStatusFailure, btObj.Get("failure").String())
}

// TestNode_Construction verifies bt.node(tick, ...children) argument validation
// and correct Go object construction.
func TestNode_Construction(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	t.Run("ValidConstruction", func(t *testing.T) {
		res := executeJS(t, bridge, `
			const tick = () => bt.success;
			bt.node(tick)
		`)
		// Should return a bt.Node (which is a Go function)
		assert.NotNil(t, res.Export())
		_, ok := res.Export().(bt.Node)
		assert.True(t, ok, "Result should be a bt.Node")
	})

	t.Run("MissingTick", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.node()`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires at least a tick function")
	})

	t.Run("InvalidChild", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.node(() => bt.success, null)`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a Node")
	})
}

// TestTick_Execution verifies bt.tick(node) correctly executes the node
// and returns the status string.
func TestTick_Execution(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	t.Run("Success", func(t *testing.T) {
		res := executeJS(t, bridge, `
			{ const n = bt.node(() => bt.success); bt.tick(n); }
		`)
		assert.Equal(t, "success", res.String())
	})

	t.Run("Failure", func(t *testing.T) {
		res := executeJS(t, bridge, `
			{ const n = bt.node(() => bt.failure); bt.tick(n); }
		`)
		assert.Equal(t, "failure", res.String())
	})

	t.Run("WithChildren", func(t *testing.T) {
		// Test that children are passed down (conceptually).
		// Since we are mocking the tick in JS, we verify the structure works.
		res := executeJS(t, bridge, `
			{
				const child = bt.node(() => bt.success);
				const parent = bt.node(() => bt.success, child);
				bt.tick(parent);
			}
		`)
		assert.Equal(t, "success", res.String())
	})

	t.Run("ExecutionError", func(t *testing.T) {
		// If JS throws, bt.tick should return failure
		res := executeJS(t, bridge, `
			{ const n = bt.node(() => { throw new Error("boom"); }); bt.tick(n); }
		`)
		assert.Equal(t, "failure", res.String())
	})
}

// TestUnwrap_Logic verifies the internal logic of converting JS objects to Go bt.Node/bt.Tick
// and vice versa, ensuring no double-wrapping and correct type handling.
func TestUnwrap_Logic(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	t.Run("NodeUnwrap_PureJS", func(t *testing.T) {
		// 1. Define a pure JS node function: () => [tick, children]
		var node bt.Node
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			val, err := vm.RunString(`
				(() => [ (children) => bt.success, [] ])
			`)
			if err != nil {
				return err
			}
			// Use internal unwrap function
			node, err = nodeUnwrap(bridge, vm, val)
			return err
		})
		require.NoError(t, err)
		require.NotNil(t, node)

		// 2. Tick the unwrapped node from Go
		// This should trigger the JS execution on the loop synchronously
		tick, children := node()
		require.NotNil(t, tick)
		assert.Empty(t, children)

		status, err := tick(children)
		require.NoError(t, err)
		assert.Equal(t, bt.Success, status)
	})

	t.Run("NodeUnwrap_NativeGo_NoDoubleWrap", func(t *testing.T) {
		// 1. Create a native Go node
		originalNode := bt.New(func([]bt.Node) (bt.Status, error) {
			return bt.Success, nil
		})

		var unwrappedNode bt.Node
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			// Wrap it into Goja
			val := vm.ToValue(originalNode)

			// Unwrap it back
			var err error
			unwrappedNode, err = nodeUnwrap(bridge, vm, val)
			return err
		})
		require.NoError(t, err)

		// 2. Verify identity (function pointers can't be compared easily in Go,
		// but we can check behavior and type assertion if it was a struct,
		// here we verify execution works identically)
		tick, _ := unwrappedNode()
		status, err := tick(nil)
		require.NoError(t, err)
		assert.Equal(t, bt.Success, status)
	})

	t.Run("TickUnwrap_PureJS", func(t *testing.T) {
		// Verify unwrapping a pure JS tick function: (children) => status
		var tick bt.Tick
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			val, err := vm.RunString(`(children) => bt.running`)
			if err != nil {
				return err
			}
			tick, err = tickUnwrap(bridge, vm, val)
			return err
		})
		require.NoError(t, err)

		// Executing the Go wrapper should call JS
		status, err := tick(nil)
		require.NoError(t, err)
		assert.Equal(t, bt.Running, status)
	})

	t.Run("TickUnwrap_PromiseJS", func(t *testing.T) {
		// Verify that createLeafNode properly handles async functions
		// Async ticks return Running on first tick, need second tick for result
		var node bt.Node
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			val, err := vm.RunString(`
				bt.createLeafNode(async () => {
					return bt.success;
				})
			`)
			if err != nil {
				return err
			}
			node = val.Export().(bt.Node)
			return nil
		})
		require.NoError(t, err)

		// First tick: Running
		tick, _ := node()
		status, err := tick(nil)
		require.NoError(t, err)
		assert.Equal(t, bt.Running, status)

		// Yield to event loop
		err = bridge.RunOnLoopSync(func(vm *goja.Runtime) error { return nil })
		require.NoError(t, err)

		// Second tick: Success
		status, err = tick(nil)
		require.NoError(t, err)
		assert.Equal(t, bt.Success, status)
	})
}

// TestComposites_Sequence verifies the mapping of bt.sequence to the Go implementation.
func TestComposites_Sequence(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	// Note: bt.sequence expects an array of Nodes.
	// Since we map it to Go's bt.Sequence, the children must be valid Go bt.Node objects.
	// We use bt.node() or createLeafNode() to ensure this.

	t.Run("Sequence_Success", func(t *testing.T) {
		// Note: bt.sequence expects an array of Nodes.
		// Since we map it to Go's bt.Sequence, the children must be valid Go bt.Node objects.
		// We use bt.node() or createLeafNode() to ensure this.
		// IMPORTANT: createLeafNode() creates a STATEFUL async wrapper that returns Running on first tick.
		// For synchronous tests, use bt.node() to create a simple wrapper.

		res := executeJS(t, bridge, `
			(() => {
				const l1 = bt.node(() => bt.success);
				const l2 = bt.node(() => bt.success);
				return bt.sequence([l1, l2]);
			})()
		`)
		// bt.sequence now returns string status (HIGH #4 FIX)
		statusStr := res.String()
		status := mapJSStatus(statusStr)
		assert.Equal(t, bt.Success, status)
	})

	t.Run("Sequence_Failure", func(t *testing.T) {
		res := executeJS(t, bridge, `
			(() => {
				const l1 = bt.node(() => bt.success);
				const l2 = bt.node(() => bt.failure);
				return bt.sequence([l1, l2]);
			})()
		`)
		// bt.sequence now returns string status (HIGH #4 FIX)
		statusStr := res.String()
		status := mapJSStatus(statusStr)
		assert.Equal(t, bt.Failure, status)
	})
}

// TestComposites_Parallel verifies the custom JS wrapper for parallel execution.

// TestLeaves_CreateLeafNode verifies the adapter wrapping JS async functions.
func TestLeaves_CreateLeafNode(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	t.Run("Async_Execution", func(t *testing.T) {
		// createLeafNode returns a bt.Node.
		// When ticked, it should behave like JSLeafAdapter (stateful).
		// 1st tick: Running (starts async)
		// 2nd tick: Success (after promise resolves)

		// Setup a node
		var node bt.Node
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			val, err := vm.RunString(`
				bt.createLeafNode(async () => {
					return bt.success;
				})
			`)
			if err != nil {
				return err
			}
			node = val.Export().(bt.Node)
			return nil
		})
		require.NoError(t, err)

		// 1. First Tick -> Running (starts execution)
		tick, _ := node()
		status, err := tick(nil)
		require.NoError(t, err)
		assert.Equal(t, bt.Running, status)

		// 2. Wait for event loop to process the promise
		// We simply yield or run a small no-op on the loop to allow microtasks to flush
		err = bridge.RunOnLoopSync(func(vm *goja.Runtime) error { return nil })
		require.NoError(t, err)

		// 3. Second Tick -> Success
		status, err = tick(nil)
		require.NoError(t, err)
		assert.Equal(t, bt.Success, status)
	})

	t.Run("Invalid_Args", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.createLeafNode(null)`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "argument must be a callable function")
	})
}

// TestLeaves_BlockingLeaf verifies createBlockingLeafNode.
func TestLeaves_BlockingLeaf(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	t.Run("Blocks_Until_Resolve", func(t *testing.T) {
		var node bt.Node
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			val, err := vm.RunString(`
				bt.createBlockingLeafNode(async () => {
					return bt.success;
				})
			`)
			if err != nil {
				return err
			}
			node = val.Export().(bt.Node)
			return nil
		})
		require.NoError(t, err)

		// Tick should block until success, returning Success immediately (no Running state)
		tick, _ := node()
		status, err := tick(nil)
		require.NoError(t, err)
		assert.Equal(t, bt.Success, status)
	})
}

// TestBlackboard verifies the JS Blackboard API.
func TestBlackboard(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	script := `
		const bb = new bt.Blackboard();
		bb.set("foo", "bar");
		const hasFoo = bb.has("foo");
		const val = bb.get("foo");
		bb.delete("foo");
		const hasFooAfter = bb.has("foo");

		({hasFoo, val, hasFooAfter})
	`
	res := executeJS(t, bridge, script)
	obj := res.Export().(map[string]interface{})

	assert.Equal(t, true, obj["hasFoo"])
	assert.Equal(t, "bar", obj["val"])
	assert.Equal(t, false, obj["hasFooAfter"])
}

// TestEdgeCases_InvalidTypes verifies robustness against bad JS input.
func TestEdgeCases_InvalidTypes(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	t.Run("Node_InvalidReturnShape", func(t *testing.T) {
		// Pass a JS function that doesn't return an array
		var node bt.Node
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			val, _ := vm.RunString(`
				(() => "not-an-array")
			`)
			var unwrapErr error
			node, unwrapErr = nodeUnwrap(bridge, vm, val)
			return unwrapErr
		})
		require.NoError(t, err)

		// Ticking it should return error because unwrap logic runs during execution
		tick, _ := node()
		_, err = tick(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "value is not a Tick")
	})

	t.Run("Tick_Panic", func(t *testing.T) {
		// JS function panics
		var tick bt.Tick
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			val, _ := vm.RunString(`(children) => { throw "JS Panic"; }`)
			var unwrapErr error
			tick, unwrapErr = tickUnwrap(bridge, vm, val)
			return unwrapErr
		})
		require.NoError(t, err)

		status, err := tick(nil)
		// Should capture panic/error as failure status or error
		assert.Equal(t, bt.Failure, status)
		// The adapter often returns Failure status + Error object.
		// The tickUnwrap implementation catches panics and sends them to channel.
		require.Error(t, err)
		assert.Contains(t, err.Error(), "JS Panic")
	})
}

// TestRequiresNewTicker verifies that the 'run' export has been removed
// and tests should use newTicker instead.
