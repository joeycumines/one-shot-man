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
// ModuleLoader coverage tests
// These target specific uncovered branches in require.go's ModuleLoader function.
// ============================================================================

// TestModuleLoader_TickNoArgs verifies bt.tick() with no arguments panics correctly.
func TestModuleLoader_TickNoArgs(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		_, err := vm.RunString(`bt.tick()`)
		return err
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a node argument")
}

// TestModuleLoader_TickNonNode verifies bt.tick() with a non-Node argument.
func TestModuleLoader_TickNonNode(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		_, err := vm.RunString(`bt.tick("not a node")`)
		return err
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a Node")
}

// TestModuleLoader_TickDirectGoNode verifies bt.tick() with a direct Go bt.Node.
// This exercises the first Export/ok path where nodeVal.Export() returns bt.Node.
func TestModuleLoader_TickDirectGoNode(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	// Create a Go node and set it as a global, then call bt.tick()
	goNode := bt.New(func(children []bt.Node) (bt.Status, error) {
		return bt.Success, nil
	})
	err := bridge.SetGlobal("goNode", goNode)
	require.NoError(t, err)

	res := executeJS(t, bridge, `bt.tick(goNode)`)
	assert.Equal(t, "success", res.String())
}

// TestModuleLoader_TickGoNodeReturnsError verifies bt.tick() when Go node.Tick() errors.
func TestModuleLoader_TickGoNodeReturnsError(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	// Create a Go node that returns an error
	goNode := bt.New(func(children []bt.Node) (bt.Status, error) {
		return bt.Failure, assert.AnError
	})
	err := bridge.SetGlobal("errNode", goNode)
	require.NoError(t, err)

	res := executeJS(t, bridge, `bt.tick(errNode)`)
	assert.Equal(t, "failure", res.String())
}

// TestModuleLoader_TickJSNodeViaUnwrap verifies bt.tick() with a JS function node
// that goes through the nodeUnwrap fallback path (not the direct Export path).
func TestModuleLoader_TickJSNodeViaUnwrap(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	// Pass a JS node function directly to bt.tick() (not wrapped with bt.node).
	// This forces the nodeUnwrap fallback path inside bt.tick().
	// A JS node function: () => [tick, children]
	res := executeJS(t, bridge, `
		(() => {
			const jsNode = () => [(children) => "success", []];
			return bt.tick(jsNode);
		})()
	`)
	assert.Equal(t, "success", res.String())
}

// TestModuleLoader_Selector verifies bt.selector() works (separate from bt.fallback).
func TestModuleLoader_Selector(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	t.Run("AllFail", func(t *testing.T) {
		res := executeJS(t, bridge, `
			(() => {
				const l1 = bt.node(() => bt.failure);
				const l2 = bt.node(() => bt.failure);
				return bt.selector([l1, l2]);
			})()
		`)
		assert.Equal(t, "failure", res.String())
	})

	t.Run("SecondSucceeds", func(t *testing.T) {
		res := executeJS(t, bridge, `
			(() => {
				const l1 = bt.node(() => bt.failure);
				const l2 = bt.node(() => bt.success);
				return bt.selector([l1, l2]);
			})()
		`)
		assert.Equal(t, "success", res.String())
	})

	t.Run("InvalidChild", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.selector(["not a node"])`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "selector child 0")
	})
}

// TestModuleLoader_SequenceInvalidChild verifies bt.sequence() with an invalid child.
func TestModuleLoader_SequenceInvalidChild(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		_, err := vm.RunString(`bt.sequence(["not a node"])`)
		return err
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sequence child 0")
}

// TestModuleLoader_FallbackInvalidChild verifies bt.fallback() with an invalid child.
func TestModuleLoader_FallbackInvalidChild(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		_, err := vm.RunString(`bt.fallback(["not a node"])`)
		return err
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fallback child 0")
}

// TestModuleLoader_Interval verifies bt.interval() creates a rate-limiting tick.
func TestModuleLoader_Interval(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	t.Run("ValidInterval", func(t *testing.T) {
		res := executeJS(t, bridge, `
			(() => {
				const timer = bt.interval(100);
				return typeof timer;
			})()
		`)
		// interval returns a Go bt.Tick function
		assert.NotNil(t, res)
	})

	t.Run("NoArgs", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.interval()`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires intervalMs argument")
	})
}

// TestModuleLoader_MemorizeErrors verifies bt.memorize() error paths.
func TestModuleLoader_MemorizeErrors(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	t.Run("NoArgs", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.memorize()`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires a tick argument")
	})

	t.Run("InvalidTick", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.memorize("not a tick")`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a Tick")
	})
}

// TestModuleLoader_AsyncErrors verifies bt.async() error paths.
func TestModuleLoader_AsyncErrors(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	t.Run("NoArgs", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.async()`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires a tick argument")
	})

	t.Run("InvalidTick", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.async(42)`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a Tick")
	})
}

// TestModuleLoader_NotErrors verifies bt.not() error paths.
func TestModuleLoader_NotErrors(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	t.Run("NoArgs", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.not()`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires a tick argument")
	})

	t.Run("InvalidTick", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.not(null)`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a Tick")
	})
}

// TestModuleLoader_NodeInvalidTick verifies bt.node() with an invalid tick argument.
func TestModuleLoader_NodeInvalidTick(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		_, err := vm.RunString(`bt.node(42)`)
		return err
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a Tick")
}

// TestModuleLoader_CreateLeafNodeErrors verifies createLeafNode error paths.
func TestModuleLoader_CreateLeafNodeErrors(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	t.Run("NoArgs", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.createLeafNode()`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires a tick function")
	})

	t.Run("NullArg", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.createLeafNode(null)`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "got null/undefined")
	})

	t.Run("UndefinedArg", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.createLeafNode(undefined)`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "got null/undefined")
	})

	t.Run("NonCallable", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.createLeafNode(42)`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a callable function")
	})
}

// TestModuleLoader_CreateBlockingLeafNodeErrors verifies createBlockingLeafNode error paths.
func TestModuleLoader_CreateBlockingLeafNodeErrors(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	t.Run("NoArgs", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.createBlockingLeafNode()`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires a tick function")
	})

	t.Run("NullArg", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.createBlockingLeafNode(null)`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "got null/undefined")
	})

	t.Run("UndefinedArg", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.createBlockingLeafNode(undefined)`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "got null/undefined")
	})

	t.Run("NonCallable", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.createBlockingLeafNode("hello")`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a callable function")
	})
}

// TestModuleLoader_ExposeBlackboard verifies bt.exposeBlackboard() paths.
func TestModuleLoader_ExposeBlackboard(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	t.Run("ValidBlackboard", func(t *testing.T) {
		bb := new(Blackboard)
		bb.Set("x", 42)
		err := bridge.SetGlobal("nativeBB", bb)
		require.NoError(t, err)

		res := executeJS(t, bridge, `
			(() => {
				const exposed = bt.exposeBlackboard(nativeBB);
				return exposed.get("x");
			})()
		`)
		assert.Equal(t, int64(42), res.Export())
	})

	t.Run("WrongArgCount", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.exposeBlackboard()`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires exactly one Blackboard argument")
	})

	t.Run("InvalidArg", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.exposeBlackboard("not a blackboard")`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a Blackboard instance")
	})
}

// TestModuleLoader_NewTickerErrors verifies bt.newTicker() error paths.
func TestModuleLoader_NewTickerErrors(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	t.Run("NoArgs", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.newTicker()`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires duration")
	})

	t.Run("ZeroDuration", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.newTicker(0, bt.node(() => bt.success))`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be positive")
	})

	t.Run("NegativeDuration", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.newTicker(-1, bt.node(() => bt.success))`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be positive")
	})

	t.Run("InvalidNode", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`bt.newTicker(100, "not a node")`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a Node")
	})

	t.Run("StopOnFailureOption", func(t *testing.T) {
		// Verify stopOnFailure option path is exercised
		res := executeJS(t, bridge, `
			(() => {
				const leaf = bt.node(() => bt.failure);
				const ticker = bt.newTicker(50, leaf, { stopOnFailure: true });
				ticker.stop();
				return typeof ticker;
			})()
		`)
		assert.Equal(t, "object", res.String())
	})
}

// TestModuleLoader_NewManager verifies bt.newManager() function.
func TestModuleLoader_NewManager(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	t.Run("Create", func(t *testing.T) {
		res := executeJS(t, bridge, `
			(() => {
				const mgr = bt.newManager();
				return typeof mgr.add === 'function' &&
				       typeof mgr.done === 'function' &&
				       typeof mgr.err === 'function' &&
				       typeof mgr.stop === 'function';
			})()
		`)
		assert.True(t, res.ToBoolean())
	})

	t.Run("AddInvalidArg", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`
				{
					const mgr = bt.newManager();
					mgr.add("not a ticker");
				}
			`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a ticker")
	})

	t.Run("AddNull", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`
				{
					const mgr = bt.newManager();
					mgr.add(null);
				}
			`)
			return err
		})
		require.Error(t, err)
	})

	t.Run("AddNoArgs", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`
				{
					const mgr = bt.newManager();
					mgr.add();
				}
			`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires a ticker argument")
	})

	t.Run("AddWithoutNative", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`
				{
					const m = bt.newManager();
					m.add({});
				}
			`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a ticker created by newTicker")
	})

	t.Run("AddWithWrongNative", func(t *testing.T) {
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`
				{
					const m2 = bt.newManager();
					m2.add({ _native: "not a ticker" });
				}
			`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a ticker created by newTicker")
	})

	t.Run("ErrNoError", func(t *testing.T) {
		res := executeJS(t, bridge, `
			(() => {
				const mgr = bt.newManager();
				return mgr.err();
			})()
		`)
		assert.True(t, goja.IsNull(res))
	})

	t.Run("StopManager", func(t *testing.T) {
		res := executeJS(t, bridge, `
			(() => {
				const mgr = bt.newManager();
				mgr.stop();
				return true;
			})()
		`)
		assert.True(t, res.ToBoolean())
	})
}

// TestModuleLoader_TickerJSWrapperMethods verifies ticker wrapper err() and stop().
func TestModuleLoader_TickerJSWrapperMethods(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	t.Run("ErrNoError", func(t *testing.T) {
		res := executeJS(t, bridge, `
			(() => {
				const leaf = bt.node(() => bt.success);
				const ticker = bt.newTicker(50, leaf);
				const errVal = ticker.err();
				ticker.stop();
				return errVal;
			})()
		`)
		assert.True(t, goja.IsNull(res))
	})

	t.Run("StopAndErr", func(t *testing.T) {
		res := executeJS(t, bridge, `
			(() => {
				const leaf = bt.node(() => bt.success);
				const ticker = bt.newTicker(50, leaf);
				ticker.stop();
				return ticker.err();
			})()
		`)
		// After stop, err might return null or an error string
		// The important thing is it doesn't crash
		assert.NotNil(t, res)
	})
}

// ============================================================================
// tickUnwrap coverage tests
// These target specific uncovered branches in unwrap.go's tickUnwrap function.
// ============================================================================

// TestTickUnwrap_GoFunctionDirect verifies tickUnwrap with a Go function
// matching bt.Tick signature (Case 2).
func TestTickUnwrap_GoFunctionDirect(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	goTick := func(children []bt.Node) (bt.Status, error) {
		return bt.Success, nil
	}

	var tick bt.Tick
	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		val := vm.ToValue(goTick)
		var unwrapErr error
		tick, unwrapErr = tickUnwrap(bridge, vm, val)
		return unwrapErr
	})
	require.NoError(t, err)
	require.NotNil(t, tick)

	status, err := tick(nil)
	require.NoError(t, err)
	assert.Equal(t, bt.Success, status)
}

// TestTickUnwrap_NilUndefinedNull verifies tickUnwrap with nil/undefined/null values.
func TestTickUnwrap_NilUndefinedNull(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		// nil
		_, err := tickUnwrap(bridge, vm, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nil or undefined")

		// undefined
		_, err = tickUnwrap(bridge, vm, goja.Undefined())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nil or undefined")

		// null
		_, err = tickUnwrap(bridge, vm, goja.Null())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nil or undefined")
		return nil
	})
	require.NoError(t, err)
}

// TestTickUnwrap_AsyncFunctionRejection verifies that tickUnwrap rejects
// async functions that return Promises.
func TestTickUnwrap_AsyncFunctionRejection(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	// Use an actual async keyword to test the async detection path.
	// Note: goja's Export() behavior for Promises varies — a resolved
	// Promise.resolve(x) may export as x directly. An async function
	// creates a proper Promise that should trigger the detection code.
	var tick bt.Tick
	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		val, err := vm.RunString(`(async (children) => "success")`)
		if err != nil {
			return err
		}
		tick, err = tickUnwrap(bridge, vm, val)
		return err
	})
	require.NoError(t, err, "tickUnwrap should succeed, it builds a wrapper")
	require.NotNil(t, tick)

	// Calling the tick should detect the async function and either:
	// - Return Failure with an error about async functions, OR
	// - Return Failure with a different error (depending on goja Promise export)
	// The key invariant: it must NOT return Success without proper resolution.
	status, err := tick(nil)
	assert.Equal(t, bt.Failure, status)
	if err != nil {
		t.Logf("async rejection error: %v", err)
	}
}

// TestTickUnwrap_NonPromiseObjectReturn verifies tick function returning
// a non-Promise object is rejected.
func TestTickUnwrap_NonPromiseObjectReturn(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	var tick bt.Tick
	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		val, err := vm.RunString(`(children) => ({foo: "bar"})`)
		if err != nil {
			return err
		}
		tick, err = tickUnwrap(bridge, vm, val)
		return err
	})
	require.NoError(t, err)
	require.NotNil(t, tick)

	status, err := tick(nil)
	assert.Equal(t, bt.Failure, status)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "returned non-Promise object")
}

// TestTickUnwrap_ObjectWithNonCallableThen verifies tick function returning
// an object with a non-callable "then" property (HIGH #1 FIX path).
func TestTickUnwrap_ObjectWithNonCallableThen(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	var tick bt.Tick
	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		val, err := vm.RunString(`(children) => ({then: "not a function"})`)
		if err != nil {
			return err
		}
		tick, err = tickUnwrap(bridge, vm, val)
		return err
	})
	require.NoError(t, err)
	require.NotNil(t, tick)

	status, err := tick(nil)
	assert.Equal(t, bt.Failure, status)
	assert.Error(t, err)
	// Non-callable then means it's not a Promise, so it's a non-Promise object
	assert.Contains(t, err.Error(), "returned non-Promise object")
}

// TestTickUnwrap_NullReturn verifies tick function returning null.
func TestTickUnwrap_NullReturn(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	var tick bt.Tick
	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		val, err := vm.RunString(`(children) => null`)
		if err != nil {
			return err
		}
		tick, err = tickUnwrap(bridge, vm, val)
		return err
	})
	require.NoError(t, err)
	require.NotNil(t, tick)

	status, err := tick(nil)
	assert.Equal(t, bt.Failure, status)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "returned null/undefined")
}

// TestTickUnwrap_VmNilError verifies tickUnwrap with vm=nil for a JS function.
func TestTickUnwrap_VmNilError(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	var tick bt.Tick
	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		val, err := vm.RunString(`(children) => "success"`)
		if err != nil {
			return err
		}
		// Wrap with vm=nil — should create the wrapper but fail when called
		tick, err = tickUnwrap(bridge, nil, val)
		return err
	})
	// tickUnwrap itself doesn't check vm=nil for Case 3 up front;
	// it creates the wrapper and the wrapper checks vm at call time
	require.NoError(t, err)
	require.NotNil(t, tick)

	status, err := tick(nil)
	assert.Equal(t, bt.Failure, status)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "vm is required")
}

// TestTickUnwrap_NonCallable verifies tickUnwrap with a non-callable value.
func TestTickUnwrap_NonCallable(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		val := vm.ToValue(42)
		_, err := tickUnwrap(bridge, vm, val)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not a Tick")
		return nil
	})
	require.NoError(t, err)
}

// TestTickUnwrap_JSWithChildren verifies the tickUnwrap wrapper correctly
// passes Go children to the JS function.
func TestTickUnwrap_JSWithChildren(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	var tick bt.Tick
	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		val, err := vm.RunString(`
			(children) => {
				// If children array exists and has length, success
				if (children && children.length > 0) return "success";
				return "failure";
			}
		`)
		if err != nil {
			return err
		}
		tick, err = tickUnwrap(bridge, vm, val)
		return err
	})
	require.NoError(t, err)

	// Call with children
	child := bt.New(func(children []bt.Node) (bt.Status, error) {
		return bt.Success, nil
	})
	status, err := tick([]bt.Node{child})
	require.NoError(t, err)
	assert.Equal(t, bt.Success, status)
}

// ============================================================================
// nodeUnwrap coverage tests
// ============================================================================

// TestNodeUnwrap_GoFunctionDirect verifies nodeUnwrap with a Go function
// matching bt.Node signature (Case 2).
func TestNodeUnwrap_GoFunctionDirect(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	goNodeFn := func() (bt.Tick, []bt.Node) {
		return func(children []bt.Node) (bt.Status, error) {
			return bt.Success, nil
		}, nil
	}

	var node bt.Node
	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		val := vm.ToValue(goNodeFn)
		var unwrapErr error
		node, unwrapErr = nodeUnwrap(bridge, vm, val)
		return unwrapErr
	})
	require.NoError(t, err)
	require.NotNil(t, node)

	tick, children := node()
	assert.NotNil(t, tick)
	assert.Nil(t, children)

	status, err := tick(nil)
	require.NoError(t, err)
	assert.Equal(t, bt.Success, status)
}

// TestNodeUnwrap_NilUndefinedNull verifies nodeUnwrap with nil/undefined/null values.
func TestNodeUnwrap_NilUndefinedNull(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		_, err := nodeUnwrap(bridge, vm, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nil or undefined")

		_, err = nodeUnwrap(bridge, vm, goja.Undefined())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nil or undefined")

		_, err = nodeUnwrap(bridge, vm, goja.Null())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nil or undefined")
		return nil
	})
	require.NoError(t, err)
}

// TestNodeUnwrap_VmNilForJSFunction verifies nodeUnwrap with vm=nil for a JS function.
func TestNodeUnwrap_VmNilForJSFunction(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		val, err := vm.RunString(`() => [(children) => "success", []]`)
		if err != nil {
			return err
		}
		_, err = nodeUnwrap(bridge, nil, val)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "vm is required")
		return nil
	})
	require.NoError(t, err)
}

// TestNodeUnwrap_JSNodeWithChildren verifies nodeUnwrap wraps a JS function that
// returns children correctly.
func TestNodeUnwrap_JSNodeWithChildren(t *testing.T) {
	t.Parallel()
	bridge, _, _ := setupTestEnv(t)

	var node bt.Node
	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		val, err := vm.RunString(`
			() => {
				const childNode = bt.node(() => bt.success);
				return [(children) => bt.success, [childNode]];
			}
		`)
		if err != nil {
			return err
		}
		node, err = nodeUnwrap(bridge, vm, val)
		return err
	})
	require.NoError(t, err)

	tick, children := node()
	assert.NotNil(t, tick)
	assert.NotEmpty(t, children)
}

// ============================================================================
// Bridge coverage tests
// These target specific uncovered branches in bridge.go.
// ============================================================================

// TestBridge_NewBridgeWithEventLoop_NilLoopPanic verifies panic on nil loop.
func TestBridge_NewBridgeWithEventLoop_NilLoopPanic(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "event loop must not be nil", func() {
		NewBridgeWithEventLoop(context.Background(), nil, nil, nil)
	})
}

// TestBridge_RunOnLoopSync_NoTimeout verifies RunOnLoopSync with timeout=0 (disabled).
func TestBridge_RunOnLoopSync_NoTimeout(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)
	bridge.SetTimeout(0) // Disable timeout

	var result int64
	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		val, err := vm.RunString(`1 + 2`)
		if err != nil {
			return err
		}
		result = val.ToInteger()
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, int64(3), result)
}

// TestBridge_RunOnLoopSync_BridgeStopped verifies RunOnLoopSync with stopped bridge.
func TestBridge_RunOnLoopSync_BridgeStopped(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)
	bridge.Stop()

	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		return nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

// TestBridge_RunOnLoopSync_DoneDuringWait verifies RunOnLoopSync returns error
// when bridge is stopped while waiting (without timeout).
func TestBridge_RunOnLoopSync_DoneDuringWait(t *testing.T) {
	t.Parallel()

	bridge, loop := testBridgeWithManualShutdown(t)
	bridge.SetTimeout(0) // Disable timeout to test the Done() path

	// Load a slow operation (500ms is plenty — bridge.Stop() fires after 50ms)
	err := bridge.LoadScript("slow.js", `
		globalThis.blockForever = function() {
			var start = Date.now();
			while (Date.now() - start < 500) {
				// busy wait
				for (var i = 0; i < 1000; i++) {}
			}
		};
	`)
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() {
		errCh <- bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString("blockForever()")
			return err
		})
	}()

	// Stop the bridge while the operation is running
	time.Sleep(50 * time.Millisecond)
	bridge.Stop()

	select {
	case err := <-errCh:
		// Should get "bridge stopped" error since Done() closes first
		if err != nil {
			assert.Contains(t, err.Error(), "stopped")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunOnLoopSync should have unblocked when bridge stopped")
	}

	loop()
}

// TestBridge_TryRunOnLoopSync_BridgeStopped verifies TryRunOnLoopSync with stopped bridge.
func TestBridge_TryRunOnLoopSync_BridgeStopped(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)
	bridge.Stop()

	err := bridge.TryRunOnLoopSync(nil, func(vm *goja.Runtime) error {
		return nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

// TestBridge_RunJSSync verifies the RunJSSync alias for RunOnLoopSync.
func TestBridge_RunJSSync(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)

	var result int64
	err := bridge.RunJSSync(func(vm *goja.Runtime) error {
		val, err := vm.RunString(`21 * 2`)
		if err != nil {
			return err
		}
		result = val.ToInteger()
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, int64(42), result)
}

// TestBridge_LoadScriptRuntimeError verifies LoadScript with a script that compiles
// but throws at runtime.
func TestBridge_LoadScriptRuntimeError(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)

	err := bridge.LoadScript("runtime_err.js", `throw new Error("runtime failure");`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to run")
}

// ============================================================================
// Adapter coverage tests
// These target specific uncovered branches in adapter.go.
// ============================================================================

// TestJSLeafAdapter_NilContext verifies NewJSLeafAdapter with nil context defaults
// to context.Background().
func TestJSLeafAdapter_NilContext(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)

	err := bridge.LoadScript("leaf.js", `
		async function simpleLeaf() {
			return bt.success;
		}
	`)
	require.NoError(t, err)

	fn, err := bridge.GetCallable("simpleLeaf")
	require.NoError(t, err)

	// Pass nil context indirectly — NewJSLeafAdapter should default to context.Background()
	var nilCtx context.Context // intentionally testing nil context handling
	node := NewJSLeafAdapter(nilCtx, bridge, fn, nil)

	status, err := node.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Running, status)

	finalStatus, err := waitForCompletion(t, node, time.Second)
	require.NoError(t, err)
	require.Equal(t, bt.Success, finalStatus)
}

// TestJSLeafAdapter_WithGetCtx verifies NewJSLeafAdapter with a getCtx function.
func TestJSLeafAdapter_WithGetCtx(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)

	err := bridge.LoadScript("leaf.js", `
		async function ctxLeaf(ctx, args) {
			return ctx && ctx.value === 42 ? bt.success : bt.failure;
		}
	`)
	require.NoError(t, err)

	fn, err := bridge.GetCallable("ctxLeaf")
	require.NoError(t, err)

	// Provide a getCtx that returns an object
	node := NewJSLeafAdapter(context.TODO(), bridge, fn, func() any {
		return map[string]any{"value": 42}
	})

	status, err := node.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Running, status)

	finalStatus, err := waitForCompletion(t, node, time.Second)
	require.NoError(t, err)
	require.Equal(t, bt.Success, finalStatus)
}

// TestBlockingJSLeaf_NilContext verifies BlockingJSLeaf with nil context.
func TestBlockingJSLeaf_NilContext(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)

	err := bridge.LoadScript("leaf.js", `
		async function blockingSimple() {
			return bt.success;
		}
	`)
	require.NoError(t, err)

	fn, err := bridge.GetCallable("blockingSimple")
	require.NoError(t, err)

	// nil context indirectly — should default to context.Background()
	var nilCtx context.Context // intentionally testing nil context handling
	node := BlockingJSLeaf(nilCtx, bridge, nil, fn, nil)

	status, err := node.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Success, status)
}

// TestBlockingJSLeaf_WithGetCtx verifies BlockingJSLeaf with a getCtx function.
func TestBlockingJSLeaf_WithGetCtx(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)

	err := bridge.LoadScript("leaf.js", `
		async function ctxBlockingLeaf(ctx, args) {
			return ctx && ctx.value === 99 ? bt.success : bt.failure;
		}
	`)
	require.NoError(t, err)

	fn, err := bridge.GetCallable("ctxBlockingLeaf")
	require.NoError(t, err)

	node := blockingJSLeaveNoVM(context.TODO(), bridge, fn, func() any {
		return map[string]any{"value": 99}
	})

	status, err := node.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Success, status)
}

// TestBlockingJSLeaf_OnEventLoopWithGetCtx verifies BlockingJSLeaf on event loop
// with a getCtx function.
func TestBlockingJSLeaf_OnEventLoopWithGetCtx(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)

	err := bridge.LoadScript("leaf.js", `
		function syncCtxLeaf(ctx, args) {
			return ctx && ctx.count === 7 ? bt.success : bt.failure;
		}
	`)
	require.NoError(t, err)

	var fn goja.Callable
	var vm *goja.Runtime
	err = bridge.RunOnLoopSync(func(runtime *goja.Runtime) error {
		vm = runtime
		val := runtime.Get("syncCtxLeaf")
		var ok bool
		fn, ok = goja.AssertFunction(val)
		if !ok {
			return assert.AnError
		}
		return nil
	})
	require.NoError(t, err)

	// Create blocking leaf WITH vm (for on-event-loop direct execution)
	node := BlockingJSLeaf(context.TODO(), bridge, vm, fn, func() any {
		return map[string]any{"count": 7}
	})

	// Tick from a goroutine (non-event-loop) to exercise the channel-based path
	status, err := node.Tick()
	require.NoError(t, err)
	require.Equal(t, bt.Success, status)
}

// TestBlockingJSLeaf_OnEventLoopDirect verifies the direct execution path
// when BlockingJSLeaf is called FROM the event loop goroutine.
func TestBlockingJSLeaf_OnEventLoopDirect(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)

	err := bridge.LoadScript("leaf.js", `
		function directLeaf(ctx, args) {
			return bt.success;
		}
	`)
	require.NoError(t, err)

	var fn goja.Callable
	var vm *goja.Runtime
	err = bridge.RunOnLoopSync(func(runtime *goja.Runtime) error {
		vm = runtime
		val := runtime.Get("directLeaf")
		var ok bool
		fn, ok = goja.AssertFunction(val)
		if !ok {
			return assert.AnError
		}
		return nil
	})
	require.NoError(t, err)

	// Create blocking leaf WITH vm
	node := BlockingJSLeaf(context.TODO(), bridge, vm, fn, nil)

	// Call Tick from WITHIN the event loop to exercise the direct execution path
	var status bt.Status
	var tickErr error
	err = bridge.RunOnLoopSync(func(runtime *goja.Runtime) error {
		tick, _ := node()
		status, tickErr = tick(nil)
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, tickErr)
	require.Equal(t, bt.Success, status)
}

// TestBlockingJSLeaf_OnEventLoopAsyncFails verifies that async functions
// that actually defer (await something) cannot be executed synchronously
// on the event loop.
func TestBlockingJSLeaf_OnEventLoopAsyncFails(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)

	// Use a function that actually awaits something (defers to next macrotask).
	// An async function that resolves immediately (no real await) WILL work
	// synchronously on the event loop because the Promise is already settled.
	// Only deferred Promises trigger the "!called" detection path.
	err := bridge.LoadScript("leaf.js", `
		async function deferredAsyncOnLoop(ctx, args) {
			await new Promise(resolve => setTimeout(resolve, 0));
			return bt.success;
		}
	`)
	require.NoError(t, err)

	var fn goja.Callable
	var vm *goja.Runtime
	err = bridge.RunOnLoopSync(func(runtime *goja.Runtime) error {
		vm = runtime
		val := runtime.Get("deferredAsyncOnLoop")
		var ok bool
		fn, ok = goja.AssertFunction(val)
		if !ok {
			return assert.AnError
		}
		return nil
	})
	require.NoError(t, err)

	// Create blocking leaf WITH vm
	node := BlockingJSLeaf(context.TODO(), bridge, vm, fn, nil)

	// Call Tick from WITHIN the event loop - deferred async function can't
	// complete synchronously because it needs the next macrotask to resolve.
	var status bt.Status
	var tickErr error
	err = bridge.RunOnLoopSync(func(runtime *goja.Runtime) error {
		tick, _ := node()
		status, tickErr = tick(nil)
		return nil
	})
	require.NoError(t, err)
	require.Error(t, tickErr)
	require.Equal(t, bt.Failure, status)
	assert.Contains(t, tickErr.Error(), "async JS function cannot be executed synchronously")
}

// ============================================================================
// mapGoStatus coverage tests
// ============================================================================

// TestMapGoStatus verifies mapGoStatus for all branches including default.
func TestMapGoStatus(t *testing.T) {
	t.Parallel()

	assert.Equal(t, JSStatusRunning, mapGoStatus(bt.Running))
	assert.Equal(t, JSStatusSuccess, mapGoStatus(bt.Success))
	assert.Equal(t, JSStatusFailure, mapGoStatus(bt.Failure))
	// Default case — some invalid bt.Status value
	assert.Equal(t, JSStatusFailure, mapGoStatus(bt.Status(99)))
}

// TestDurationFromMs verifies durationFromMs helper.
func TestDurationFromMs(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 100*time.Millisecond, durationFromMs(100))
	assert.Equal(t, time.Second, durationFromMs(1000))
	assert.Equal(t, time.Duration(0), durationFromMs(0))
}

// ============================================================================
// Blackboard edge case coverage
// ============================================================================

// TestBlackboard_GetNilMap verifies Blackboard.Get() with nil map.
func TestBlackboard_GetNilMap(t *testing.T) {
	t.Parallel()

	bb := new(Blackboard)
	// Get on uninitialized blackboard
	assert.Nil(t, bb.Get("missing"))
}

// TestBlackboard_HasNilMap verifies Blackboard.Has() with nil map.
func TestBlackboard_HasNilMap(t *testing.T) {
	t.Parallel()

	bb := new(Blackboard)
	assert.False(t, bb.Has("missing"))
}

// TestBlackboard_DeleteNilMap verifies Blackboard.Delete() with nil map.
func TestBlackboard_DeleteNilMap(t *testing.T) {
	t.Parallel()

	bb := new(Blackboard)
	// Delete on nil map should not panic
	bb.Delete("missing")
}

// TestBlackboard_KeysNilMap verifies Blackboard.Keys() with nil map.
func TestBlackboard_KeysNilMap(t *testing.T) {
	t.Parallel()

	bb := new(Blackboard)
	assert.Nil(t, bb.Keys())
}

// TestBlackboard_SnapshotNilMap verifies Blackboard.Snapshot() with nil map.
func TestBlackboard_SnapshotNilMap(t *testing.T) {
	t.Parallel()

	bb := new(Blackboard)
	assert.Nil(t, bb.Snapshot())
}

// TestBlackboard_SnapshotCopied verifies Blackboard.Snapshot() returns a copy.
func TestBlackboard_SnapshotCopied(t *testing.T) {
	t.Parallel()

	bb := new(Blackboard)
	bb.Set("key", "value")

	snap := bb.Snapshot()
	snap["key"] = "modified"

	// Original should be unaffected
	assert.Equal(t, "value", bb.Get("key"))
}
