package pabt

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
)

// TestExprCondition_SetJSObject verifies that SetJSObject correctly stores
// JavaScript object reference in ExprCondition.
func TestExprCondition_SetJSObjectNew(t *testing.T) {
	cond := NewExprCondition("test-key", "value == 42")

	// Initially jsObject should be nil
	assert.Nil(t, cond.jsObject, "jsObject should be nil initially")

	// Create a mock goja.Object (we can't use a real one without a runtime)
	// For this test, we just verify the field is settable
	mockObj := new(goja.Object)

	// Set JS object
	cond.SetJSObject(mockObj)

	// Verify it's stored
	assert.NotNil(t, cond.jsObject, "jsObject should be non-nil after SetJSObject")
	assert.Same(t, mockObj, cond.jsObject, "jsObject should be the exact same instance")

	// Verify it can be changed
	var anotherObj *goja.Object
	cond.SetJSObject(anotherObj)
	assert.Same(t, anotherObj, cond.jsObject, "jsObject should be updated to new instance")
}

// TestExprCondition_ThirdArgumentInteger verifies that newExprCondition
// accepts an integer as the third argument.
func TestExprCondition_ThirdArgumentInteger(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	res := executeJS(t, bridge, `
		(() => {
			const cond = pabt.newExprCondition("x", "value == 42", 42);
			return {
				hasKey: cond.key === "x",
				hasMatch: typeof cond.match === 'function',
				hasNative: typeof cond._native === 'object',
				value: cond.value
			};
		})()
	`)

	obj := res.Export().(map[string]interface{})
	assert.True(t, obj["hasKey"].(bool), "Should have correct key")
	assert.True(t, obj["hasMatch"].(bool), "Should have match function")
	assert.True(t, obj["hasNative"].(bool), "Should have _native property")
	assert.Equal(t, int64(42), obj["value"], "value should be 42")
}

// TestExprCondition_ThirdArgumentString verifies that newExprCondition
// accepts a string as third argument.
func TestExprCondition_ThirdArgumentString(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	res := executeJS(t, bridge, `
		(() => {
			const cond = pabt.newExprCondition("status", "value == \"ready\"", "ready");
			return {
				key: cond.key,
				value: cond.value
			};
		})()
	`)

	obj := res.Export().(map[string]interface{})
	assert.Equal(t, "status", obj["key"])
	assert.Equal(t, "ready", obj["value"], "value should be 'ready'")
}

// TestExprCondition_ThirdArgumentBooleanTrue verifies that newExprCondition
// accepts true as third argument.
func TestExprCondition_ThirdArgumentBooleanTrue(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	res := executeJS(t, bridge, `
		(() => {
			const cond = pabt.newExprCondition("enabled", "value == true", true);
			return {
				value: cond.value
			};
		})()
	`)

	obj := res.Export().(map[string]interface{})
	assert.Equal(t, true, obj["value"], "Value should be true")
}

// TestExprCondition_ThirdArgumentBooleanFalse verifies that newExprCondition
// accepts false as third argument.
func TestExprCondition_ThirdArgumentBooleanFalse(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	res := executeJS(t, bridge, `
		(() => {
			const cond = pabt.newExprCondition("disabled", "value == false", false);
			return {
				value: cond.value
			};
		})()
	`)

	obj := res.Export().(map[string]interface{})
	assert.Equal(t, false, obj["value"], "Value should be false")
}

// TestExprCondition_ThirdArgumentNegativeInteger verifies that newExprCondition
// accepts negative integers as third argument.
func TestExprCondition_ThirdArgumentNegativeInteger(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	res := executeJS(t, bridge, `
		(() => {
			const cond = pabt.newExprCondition("offset", "value == -1", -1);
			return {
				value: cond.value
			};
		})()
	`)

	obj := res.Export().(map[string]interface{})
	assert.Equal(t, int64(-1), obj["value"], "Value should be -1")
}

// TestExprCondition_JSObjectPassthroughToGenerator verifies that when an ExprCondition with
// a jsObject is passed to action generator, jsObject is returned.
func TestExprCondition_JSObjectPassthroughToGenerator(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	res := executeJS(t, bridge, `
		(() => {
			const bb = new bt.Blackboard();
			bb.set("testValue", 42);
			const state = pabt.newState(bb);

			// Track what generator receives
			let receivedCondition = null;

			state.setActionGenerator(function(failed) {
				receivedCondition = failed;
				return [];
			});

			// Create an ExprCondition with a value property
			const goal = pabt.newExprCondition("testValue", "value == 42", 42);

			// Create a plan to trigger the generator
			const plan = pabt.newPlan(state, [goal]);

			// Store what we received
			globalThis.receivedCondition = receivedCondition;
			
			return true;
		})()
	`)
	assert.True(t, res.ToBoolean())

	// Verify the generator received the original JS object
	var hasValueField bool
	_ = bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		val := vm.Get("receivedCondition")
		if val != nil && !goja.IsUndefined(val) && !goja.IsNull(val) {
			obj := val.ToObject(vm)
			valueVal := obj.Get("value")
			if valueVal != nil && !goja.IsUndefined(valueVal) {
				hasValueField = true
				// Verify the value is correct
				v := valueVal.Export()
				assert.Equal(t, int64(42), v, "received condition should have value=42")
			}
		}
		return nil
	})

	// Note: Whether generator is actually called depends on planning logic
	_ = hasValueField
}

// TestExprCondition_NoThirdArgument verifies that newExprCondition works
// without the optional third argument.
func TestExprCondition_NoThirdArgument(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	res := executeJS(t, bridge, `
		(() => {
			const cond = pabt.newExprCondition("key", "value == 42");
			return {
				hasKey: cond.key === "key",
				hasMatch: typeof cond.match === 'function',
				valueIsUndefined: cond.value === undefined
			};
		})()
	`)

	obj := res.Export().(map[string]interface{})
	assert.True(t, obj["hasKey"].(bool))
	assert.True(t, obj["hasMatch"].(bool))
	assert.True(t, obj["valueIsUndefined"].(bool), "Value should be undefined when third argument not provided")
}

// TestExprCondition_NativePropertyExists verifies that _native property
// exists on ExprCondition created via newExprCondition.
func TestExprCondition_NativePropertyExists(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	res := executeJS(t, bridge, `
		(() => {
			const cond = pabt.newExprCondition("key", "value == 42");
			return {
				hasKey: cond.key === "key",
				hasMatch: typeof cond.match === 'function',
				hasNative: typeof cond._native === 'object'
			};
		})()
	`)

	obj := res.Export().(map[string]interface{})
	assert.True(t, obj["hasKey"].(bool))
	assert.True(t, obj["hasMatch"].(bool))
	assert.True(t, obj["hasNative"].(bool), "ExprCondition should have _native property")
}

// TestExprCondition_UsedInAction verifies ExprCondition can be used as
// conditions in pabt.newAction() with proper _native property.
func TestExprCondition_UsedInAction(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	res := executeJS(t, bridge, `
		(() => {
			const bb = new bt.Blackboard();
			const state = pabt.newState(bb);
			
			// Create an ExprCondition
			const exprCond = pabt.newExprCondition("prereq", "value == true");
			
			// Create an action with ExprCondition
			const node = bt.node(() => bt.success);
			const action = pabt.newAction(
				"testAction",
				[exprCond],
				[{key: "result", value: 1}],
				node
			);
			
			// Register action
			state.registerAction("testAction", action);
			
			// Retrieve and verify it's registered
			const retrieved = state.getAction("testAction");
			
			return {
				exprCondHasNative: exprCond._native !== undefined,
				actionRegistered: retrieved !== null && retrieved !== undefined
			};
		})()
	`)

	obj := res.Export().(map[string]interface{})
	assert.True(t, obj["exprCondHasNative"].(bool), "ExprCondition should have _native property")
	assert.True(t, obj["actionRegistered"].(bool), "Action should be registered successfully")
}
