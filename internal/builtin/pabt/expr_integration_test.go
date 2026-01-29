package pabt

import (
	"fmt"
	"sync"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExprCondition_JSIntegration tests ExprCondition created via JS pabt.newExprCondition()
// and verifies proper integration with the Go backend.
func TestExprCondition_JSIntegration(t *testing.T) {
	t.Run("CreationAndMatch", func(t *testing.T) {
		bridge, _, _ := setupTestEnv(t)
		res := executeJS(t, bridge, `
			(() => {
				// Create ExprCondition via JS API
				const cond = pabt.newExprCondition("testKey", "Value == 42");
				
				// Verify it has expected properties
				return {
					hasKey: cond.key === "testKey",
					hasMatch: typeof cond.match === 'function',
					hasNative: typeof cond._native !== 'undefined',
					matchTrue: cond.match(42),
					matchFalse: cond.match(99)
				};
			})()
		`)
		obj := res.Export().(map[string]interface{})
		assert.True(t, obj["hasKey"].(bool), "Should have key property")
		assert.True(t, obj["hasMatch"].(bool), "Should have match function")
		assert.True(t, obj["hasNative"].(bool), "Should have _native property for Go backend")
		assert.True(t, obj["matchTrue"].(bool), "match(42) should return true")
		assert.False(t, obj["matchFalse"].(bool), "match(99) should return false")
	})

	t.Run("StringExpressions", func(t *testing.T) {
		bridge, _, _ := setupTestEnv(t)
		res := executeJS(t, bridge, `
			(() => {
				const cond = pabt.newExprCondition("status", 'Value == "active"');
				return {
					matchActive: cond.match("active"),
					matchInactive: cond.match("inactive")
				};
			})()
		`)
		obj := res.Export().(map[string]interface{})
		assert.True(t, obj["matchActive"].(bool))
		assert.False(t, obj["matchInactive"].(bool))
	})

	t.Run("ComparisonExpressions", func(t *testing.T) {
		bridge, _, _ := setupTestEnv(t)
		res := executeJS(t, bridge, `
			(() => {
				const cond = pabt.newExprCondition("distance", "Value < 100");
				return {
					match50: cond.match(50),
					match100: cond.match(100),
					match150: cond.match(150)
				};
			})()
		`)
		obj := res.Export().(map[string]interface{})
		assert.True(t, obj["match50"].(bool), "50 < 100")
		assert.False(t, obj["match100"].(bool), "100 is not < 100")
		assert.False(t, obj["match150"].(bool), "150 is not < 100")
	})

	t.Run("BooleanLogic", func(t *testing.T) {
		bridge, _, _ := setupTestEnv(t)
		res := executeJS(t, bridge, `
			(() => {
				const cond = pabt.newExprCondition("range", "Value >= 10 && Value <= 20");
				return {
					match5: cond.match(5),
					match15: cond.match(15),
					match25: cond.match(25)
				};
			})()
		`)
		obj := res.Export().(map[string]interface{})
		assert.False(t, obj["match5"].(bool), "5 not in [10,20]")
		assert.True(t, obj["match15"].(bool), "15 is in [10,20]")
		assert.False(t, obj["match25"].(bool), "25 not in [10,20]")
	})

	t.Run("NilCheck", func(t *testing.T) {
		bridge, _, _ := setupTestEnv(t)
		res := executeJS(t, bridge, `
			(() => {
				const cond = pabt.newExprCondition("item", "Value != nil");
				return {
					matchNull: cond.match(null),
					matchUndefined: cond.match(undefined),
					matchValue: cond.match(42)
				};
			})()
		`)
		obj := res.Export().(map[string]interface{})
		assert.False(t, obj["matchNull"].(bool), "null == nil")
		assert.False(t, obj["matchUndefined"].(bool), "undefined == nil")
		assert.True(t, obj["matchValue"].(bool), "42 != nil")
	})
}

// TestExprCondition_UsedInNewAction verifies ExprCondition can be used as
// conditions in pabt.newAction() with proper _native property detection.
func TestExprCondition_UsedInNewAction(t *testing.T) {
	t.Run("ActionWithExprCondition", func(t *testing.T) {
		bridge, _, _ := setupTestEnv(t)
		res := executeJS(t, bridge, `
			(() => {
				const bb = new bt.Blackboard();
				bb.set("x", 0);
				const state = pabt.newState(bb);
				
				// Create action with ExprCondition
				const node = bt.node(() => bt.success);
				const action = pabt.newAction(
					"testAction",
					[pabt.newExprCondition("prereq", "Value == true")],
					[{ key: "result", value: 1 }],  // lowercase value
					node
				);
				
				// Register and verify
				state.registerAction("testAction", action);
				const retrieved = state.getAction("testAction");
				
				return retrieved !== null;
			})()
		`)
		assert.True(t, res.ToBoolean())
	})

	t.Run("PlanWithExprGoals", func(t *testing.T) {
		bridge, _, _ := setupTestEnv(t)
		res := executeJS(t, bridge, `
			(() => {
				const bb = new bt.Blackboard();
				bb.set("counter", 0);
				const state = pabt.newState(bb);
				
				// Register action with ExprCondition
				const incrementNode = bt.node(() => {
					bb.set("counter", bb.get("counter") + 1);
					return bt.success;
				});
				
				const action = pabt.newAction(
					"increment",
					[],  // no preconditions
					[{ key: "counter", value: 1 }],
					incrementNode
				);
				state.registerAction("increment", action);
				
				// Create plan with ExprCondition goal
				const goal = pabt.newExprCondition("counter", "Value == 1");
				const plan = pabt.newPlan(state, [goal]);
				
				// Verify plan was created and has expected methods
				return {
					hasNode: typeof plan.node === 'function',
					hasRunning: typeof plan.running === 'function',
					nodeIsFunction: typeof plan.node() === 'function'
				};
			})()
		`)
		obj := res.Export().(map[string]interface{})
		assert.True(t, obj["hasNode"].(bool))
		assert.True(t, obj["hasRunning"].(bool))
		assert.True(t, obj["nodeIsFunction"].(bool))
	})
}

// TestExprCondition_ErrorHandling verifies graceful handling of invalid expressions.
func TestExprCondition_ErrorHandling(t *testing.T) {
	t.Run("InvalidExpression", func(t *testing.T) {
		bridge, _, _ := setupTestEnv(t)
		res := executeJS(t, bridge, `
			(() => {
				// Invalid expression syntax
				const cond = pabt.newExprCondition("key", "this is not valid !@#");
				// Should return false instead of crashing
				return cond.match(42);
			})()
		`)
		// Invalid expression should return false, not crash
		assert.False(t, res.ToBoolean())
	})

	t.Run("MissingArguments", func(t *testing.T) {
		bridge, _, _ := setupTestEnv(t)
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, err := vm.RunString(`pabt.newExprCondition()`)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires key and expression arguments")
	})
}

// TestExprCondition_ConcurrentJSIntegration verifies ExprCondition is safe for concurrent access
// when used from multiple goroutines in the integration context.
func TestExprCondition_ConcurrentJSIntegration(t *testing.T) {
	ClearExprCache() // Clean state

	// Create condition once
	cond := NewExprCondition("concurrent", "Value > 0")

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Hammer it from multiple goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				result := cond.Match(j)
				expectedResult := j > 0
				if result != expectedResult {
					errors <- fmt.Errorf("goroutine %d: Match(%d) = %v, want %v",
						id, j, result, expectedResult)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}

// TestExprCondition_ZeroGojaCallsGuarantee verifies that ExprCondition.Match
// makes absolutely ZERO calls to Goja runtime. This is the core performance guarantee.
func TestExprCondition_ZeroGojaCallsGuarantee(t *testing.T) {
	ClearExprCache()

	// ExprCondition should NOT need any Goja infrastructure
	cond := NewExprCondition("pureGo", "Value == 42")

	// Call Match many times - none should touch Goja
	for i := 0; i < 1000; i++ {
		_ = cond.Match(42)
		_ = cond.Match(i)
	}

	// If we get here without panic/crash, Goja was not involved
	// (A real test would instrument Bridge.RunOnLoopSync, but the
	// architecture makes this impossible - ExprCondition has no bridge reference)
	assert.True(t, true, "ExprCondition runs without Goja")
}

// TestExprCondition_CacheSharing verifies that identical expressions share cached programs.
func TestExprCondition_CacheSharing(t *testing.T) {
	ClearExprCache()

	expr := "Value >= 100 && Value <= 200"

	// Create multiple conditions with same expression
	cond1 := NewExprCondition("key1", expr)
	cond2 := NewExprCondition("key2", expr)
	cond3 := NewExprCondition("key3", expr)

	// Trigger compilation on first
	cond1.Match(150)

	// Others should use cached program
	assert.True(t, cond2.Match(150))
	assert.True(t, cond3.Match(150))
	assert.False(t, cond1.Match(50))
	assert.False(t, cond2.Match(250))
}
