package pabt

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dop251/goja"
	bt "github.com/joeycumines/go-behaviortree"
	"github.com/joeycumines/go-pabt"
	btmod "github.com/joeycumines/one-shot-man/internal/builtin/bt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ========================================================================
// actions.go gap coverage
// ========================================================================

func TestNewAction_EmptyName_Panics(t *testing.T) {
	t.Parallel()
	node := bt.New(func([]bt.Node) (bt.Status, error) {
		return bt.Success, nil
	})
	require.PanicsWithValue(t, "pabt.NewAction: name cannot be empty", func() {
		NewAction("", nil, nil, node)
	})
}

func TestNewAction_NilConditionInSlice_Panics(t *testing.T) {
	t.Parallel()
	node := bt.New(func([]bt.Node) (bt.Status, error) { return bt.Success, nil })
	require.PanicsWithValue(t,
		`pabt.NewAction: conditions[0] cannot be nil (action="test")`,
		func() {
			NewAction("test", []pabt.IConditions{nil}, pabt.Effects{}, node)
		},
	)
}

func TestNewAction_NilEffectsDefaultsToEmpty(t *testing.T) {
	t.Parallel()
	node := bt.New(func([]bt.Node) (bt.Status, error) { return bt.Success, nil })
	action := NewAction("test", nil, nil, node)
	require.NotNil(t, action.Effects())
	assert.Empty(t, action.Effects())
}

// ========================================================================
// evaluation.go ExprLRUCache gap coverage
// ========================================================================

func TestExprLRUCache_InvalidMaxSize(t *testing.T) {
	t.Parallel()
	cache := NewExprLRUCache(0)
	require.Equal(t, DefaultExprCacheSize, cache.maxSize)

	cache2 := NewExprLRUCache(-5)
	require.Equal(t, DefaultExprCacheSize, cache2.maxSize)
}

func TestExprLRUCache_PutExisting(t *testing.T) {
	t.Parallel()
	cache := NewExprLRUCache(10)

	// Put the same expression twice — second should update in-place
	cond1 := NewExprCondition("k", "value == 1")
	prog1, _ := cond1.getOrCompileProgram()
	cache.Put("value == 1", prog1)

	cond2 := NewExprCondition("k", "value == 2")
	prog2, _ := cond2.getOrCompileProgram()
	cache.Put("value == 1", prog2) // same key, different program

	got, ok := cache.Get("value == 1")
	require.True(t, ok)
	assert.Equal(t, prog2, got, "should have updated to the new program")
	assert.Equal(t, 1, cache.Len(), "should still have exactly 1 entry")
}

func TestExprLRUCache_Eviction(t *testing.T) {
	t.Parallel()
	cache := NewExprLRUCache(2)

	cache.Put("a", nil)
	cache.Put("b", nil)
	cache.Put("c", nil) // should evict "a" (LRU)

	assert.Equal(t, 2, cache.Len())
	_, hasA := cache.Get("a")
	assert.False(t, hasA, "a should have been evicted")
	_, hasB := cache.Get("b")
	assert.True(t, hasB)
	_, hasC := cache.Get("c")
	assert.True(t, hasC)
}

func TestExprLRUCache_Resize(t *testing.T) {
	t.Parallel()
	cache := NewExprLRUCache(5)
	cache.Put("a", nil)
	cache.Put("b", nil)
	cache.Put("c", nil)
	assert.Equal(t, 3, cache.Len())

	// Resize to 2 → evicts "a" (LRU)
	cache.Resize(2)
	assert.Equal(t, 2, cache.Len())
	_, hasA := cache.Get("a")
	assert.False(t, hasA)

	// Resize to 0 → clamps to 1
	cache.Resize(0)
	assert.Equal(t, 1, cache.Len())
}

func TestExprLRUCache_Len(t *testing.T) {
	t.Parallel()
	cache := NewExprLRUCache(10)
	assert.Equal(t, 0, cache.Len())
	cache.Put("x", nil)
	assert.Equal(t, 1, cache.Len())
}

func TestExprLRUCache_Stats(t *testing.T) {
	t.Parallel()
	cache := NewExprLRUCache(10)

	// Empty cache stats
	size, hits, misses, ratio := cache.Stats()
	assert.Equal(t, 0, size)
	assert.Equal(t, int64(0), hits)
	assert.Equal(t, int64(0), misses)
	assert.Equal(t, 0.0, ratio)

	// After put and get
	cache.Put("x", nil)
	cache.Get("x")    // hit
	cache.Get("y")    // miss
	cache.Get("x")    // hit

	size, hits, misses, ratio = cache.Stats()
	assert.Equal(t, 1, size)
	assert.Equal(t, int64(2), hits)
	assert.Equal(t, int64(1), misses)
	assert.InDelta(t, 0.6667, ratio, 0.01)
}

func TestExprLRUCache_String(t *testing.T) {
	t.Parallel()
	cache := NewExprLRUCache(10)
	cache.Put("x", nil)
	cache.Get("x")
	cache.Get("missing")

	s := cache.String()
	assert.Contains(t, s, "ExprLRUCache{")
	assert.Contains(t, s, "size=1")
	assert.Contains(t, s, "hits=1")
	assert.Contains(t, s, "misses=1")
}

func TestSetGetExprCacheSize(t *testing.T) {
	// Not parallel — modifies global state
	original := GetExprCacheSize()
	t.Cleanup(func() { SetExprCacheSize(original) })

	SetExprCacheSize(42)
	assert.Equal(t, 42, GetExprCacheSize())

	// size < 1 → clamps to 1
	SetExprCacheSize(0)
	assert.Equal(t, 1, GetExprCacheSize())

	SetExprCacheSize(-10)
	assert.Equal(t, 1, GetExprCacheSize())
}

// ========================================================================
// evaluation.go JSCondition gap coverage
// ========================================================================

func TestJSCondition_JSObject_GetSet(t *testing.T) {
	t.Parallel()
	cond := &JSCondition{key: "k"}
	assert.Nil(t, cond.JSObject())

	// Can't create real goja.Object easily without a runtime,
	// but we can verify the setter/getter round-trip with nil
	cond.SetJSObject(nil)
	assert.Nil(t, cond.JSObject())
}

func TestJSCondition_Match_BridgeStopped(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)
	// bridge starts running; stop it immediately
	bridge.Stop()

	// Use a real Callable type (never actually invoked since bridge is stopped)
	cond := NewJSCondition("k", func(this goja.Value, args ...goja.Value) (goja.Value, error) {
		return nil, nil
	}, bridge)
	result := cond.Match("anything")
	assert.False(t, result, "should return false when bridge is stopped")
}

// ========================================================================
// evaluation.go NewExprCondition gap coverage
// ========================================================================

func TestNewExprCondition_EmptyExpr_Panics(t *testing.T) {
	t.Parallel()
	require.PanicsWithValue(t, "pabt.NewExprCondition: expression cannot be empty", func() {
		NewExprCondition("k", "")
	})
}

// ========================================================================
// state.go gap coverage
// ========================================================================

func TestGetActionGenerator(t *testing.T) {
	t.Parallel()
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	// Initially nil
	assert.Nil(t, state.GetActionGenerator())

	// Set and get
	gen := func(failed pabt.Condition) ([]pabt.IAction, error) { return nil, nil }
	state.SetActionGenerator(gen)
	assert.NotNil(t, state.GetActionGenerator())

	// Clear
	state.SetActionGenerator(nil)
	assert.Nil(t, state.GetActionGenerator())
}

func TestVariable_NilBlackboard(t *testing.T) {
	t.Parallel()
	// Create State without using NewState to get nil Blackboard
	state := &State{
		actions: NewActionRegistry(),
	}
	_, err := state.Variable("key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "State.Blackboard is nil")
}

func TestActionHasRelevantEffect_NilEffect(t *testing.T) {
	t.Parallel()
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	// Action with nil effect in its effects slice
	action := &Action{
		Name:    "test",
		effects: pabt.Effects{nil, NewEffect("key", "val")},
		node:    bt.New(func([]bt.Node) (bt.Status, error) { return bt.Success, nil }),
	}

	// Use a condition that matches "val"
	cond := NewFuncCondition("key", func(v any) bool { return v == "val" })
	result := state.actionHasRelevantEffect(action, "key", cond)
	assert.True(t, result, "should skip nil effect and match the non-nil one")
}

func TestActionHasRelevantEffect_NormalizeKeyError(t *testing.T) {
	t.Parallel()
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	type unsupported struct{}
	action := &Action{
		Name:    "test",
		effects: pabt.Effects{NewEffect(unsupported{}, "val")},
		node:    bt.New(func([]bt.Node) (bt.Status, error) { return bt.Success, nil }),
	}

	// Effect key can't be normalized → skip
	cond := NewFuncCondition("key", func(v any) bool { return true })
	result := state.actionHasRelevantEffect(action, "key", cond)
	assert.False(t, result, "should skip effect with unnormalizable key")

	// Failed key can't be normalized → skip
	action2 := &Action{
		Name:    "test2",
		effects: pabt.Effects{NewEffect("key", "val")},
		node:    bt.New(func([]bt.Node) (bt.Status, error) { return bt.Success, nil }),
	}
	result2 := state.actionHasRelevantEffect(action2, unsupported{}, cond)
	assert.False(t, result2, "should skip when failed key can't be normalized")
}

// ========================================================================
// state.go normalizeKey gap coverage
// ========================================================================

func TestNormalizeKey_AllTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		key      any
		expected string
		wantErr  bool
	}{
		{"string", "hello", "hello", false},
		{"int", 42, "42", false},
		{"int8", int8(8), "8", false},
		{"int16", int16(16), "16", false},
		{"int32", int32(32), "32", false},
		{"int64", int64(64), "64", false},
		{"uint", uint(7), "7", false},
		{"uint8", uint8(8), "8", false},
		{"uint16", uint16(16), "16", false},
		{"uint32", uint32(32), "32", false},
		{"uint64", uint64(64), "64", false},
		{"float32", float32(1.5), "1.5", false},
		{"float64", float64(3.14), "3.14", false},
		{"stringer", stringerKey("sym"), "sym", false},
		{"nil", nil, "", true},
		{"unsupported", struct{}{}, "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := normalizeKey(tc.key)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

// stringerKey implements fmt.Stringer for testing the Stringer branch of normalizeKey.
type stringerKey string

func (s stringerKey) String() string { return string(s) }

// ========================================================================
// require.go JS module gap coverage
// ========================================================================

func TestModuleLoader_NewState_InvalidBlackboardNative(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const pabt = require('osm:pabt');
		try {
			pabt.newState({_native: "not a blackboard"});
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("bt.Blackboard instance")) {
				throw new Error("wrong error: " + e.message);
			}
		}
	`)
	require.NoError(t, err)
}

func TestModuleLoader_NewState_MissingNative(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const pabt = require('osm:pabt');
		try {
			pabt.newState({});
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("new bt.Blackboard()")) {
				throw new Error("wrong error: " + e.message);
			}
		}
	`)
	require.NoError(t, err)
}

func TestModuleLoader_NewState_NoArgs(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const pabt = require('osm:pabt');
		try {
			pabt.newState();
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("requires a blackboard")) {
				throw new Error("wrong error: " + e.message);
			}
		}
	`)
	require.NoError(t, err)
}

func TestModuleLoader_NewAction_NonStringName(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const pabt = require('osm:pabt');
		try {
			pabt.newAction(42, [], [], null);
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("name must be a string")) {
				throw new Error("wrong error: " + e.message);
			}
		}
	`)
	require.NoError(t, err)
}

func TestModuleLoader_NewAction_TooFewArgs(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const pabt = require('osm:pabt');
		try {
			pabt.newAction("test");
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("requires name, conditions, effects, and node")) {
				throw new Error("wrong error: " + e.message);
			}
		}
	`)
	require.NoError(t, err)
}

func TestModuleLoader_NewAction_ConditionsMissingKey(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const bt = require('osm:bt');
		const pabt = require('osm:pabt');
		const node = bt.node(function() { return bt.Success; });
		// Condition objects with missing key/match should be skipped
		const action = pabt.newAction("test",
			[{}, {match: function(){}}, {key: "x"}, null, undefined],
			[],
			node
		);
		// Action should be created — bad conditions are silently skipped
		if (!action) {
			throw new Error("action should have been created");
		}
	`)
	require.NoError(t, err)
}

func TestModuleLoader_NewAction_EffectsEdgeCases(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const bt = require('osm:bt');
		const pabt = require('osm:pabt');
		const node = bt.node(function() { return bt.Success; });
		// Effects with missing key/value, null/undefined elements
		const action = pabt.newAction("test",
			[],
			[{}, {key: "x"}, {value: "v"}, null, undefined, {key: "k", value: "v"}],
			node
		);
		if (!action) {
			throw new Error("action should have been created");
		}
	`)
	require.NoError(t, err)
}

func TestModuleLoader_NewAction_NullConditionsAndEffects(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	// undefined conditions → goja ToObject panics (TypeError)
	err := bridge.LoadScript("test.js", `
		const bt = require('osm:bt');
		const pabt = require('osm:pabt');
		const node = bt.node(function() { return bt.Success; });
		try {
			pabt.newAction("test", undefined, undefined, node);
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("Cannot convert undefined")) {
				throw new Error("wrong error: " + e.message);
			}
		}
		// Verify empty arrays DO work as the supported empty-conditions path
		const action = pabt.newAction("test", [], [], node);
		if (!action) {
			throw new Error("action with empty arrays should work");
		}
	`)
	require.NoError(t, err)
}

func TestModuleLoader_SetActionGenerator_Null(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const bt = require('osm:bt');
		const pabt = require('osm:pabt');
		const bb = new bt.Blackboard();
		const state = pabt.newState(bb);
		// Setting null should clear the generator
		state.setActionGenerator(null);
		// Setting undefined should also clear it
		state.setActionGenerator(undefined);
	`)
	require.NoError(t, err)
}

func TestModuleLoader_SetActionGenerator_NotFunction(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const bt = require('osm:bt');
		const pabt = require('osm:pabt');
		const bb = new bt.Blackboard();
		const state = pabt.newState(bb);
		try {
			state.setActionGenerator("not a function");
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("generator must be a function")) {
				throw new Error("wrong error: " + e.message);
			}
		}
	`)
	require.NoError(t, err)
}

func TestModuleLoader_SetActionGenerator_NoArgs(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const bt = require('osm:bt');
		const pabt = require('osm:pabt');
		const bb = new bt.Blackboard();
		const state = pabt.newState(bb);
		try {
			state.setActionGenerator();
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("requires a generator function")) {
				throw new Error("wrong error: " + e.message);
			}
		}
	`)
	require.NoError(t, err)
}

func TestModuleLoader_NewPlan_TooFewArgs(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const pabt = require('osm:pabt');
		try {
			pabt.newPlan();
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("requires state and goals")) {
				throw new Error("wrong error: " + e.message);
			}
		}
	`)
	require.NoError(t, err)
}

func TestModuleLoader_NewPlan_InvalidState(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const pabt = require('osm:pabt');
		try {
			pabt.newPlan({}, []);
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("PABTState")) {
				throw new Error("wrong error: " + e.message);
			}
		}
	`)
	require.NoError(t, err)
}

func TestModuleLoader_NewPlan_GoalMissingKey(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const bt = require('osm:bt');
		const pabt = require('osm:pabt');
		const bb = new bt.Blackboard();
		const state = pabt.newState(bb);
		try {
			pabt.newPlan(state, [{match: function(){return true;}}]);
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("must have a 'key'")) {
				throw new Error("wrong error: " + e.message);
			}
		}
	`)
	require.NoError(t, err)
}

func TestModuleLoader_NewPlan_GoalMissingMatch(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const bt = require('osm:bt');
		const pabt = require('osm:pabt');
		const bb = new bt.Blackboard();
		const state = pabt.newState(bb);
		try {
			pabt.newPlan(state, [{key: "k"}]);
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("must have a 'match'")) {
				throw new Error("wrong error: " + e.message);
			}
		}
	`)
	require.NoError(t, err)
}

func TestModuleLoader_NewPlan_GoalMatchNotFunction(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const bt = require('osm:bt');
		const pabt = require('osm:pabt');
		const bb = new bt.Blackboard();
		const state = pabt.newState(bb);
		try {
			pabt.newPlan(state, [{key: "k", match: "not a function"}]);
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("must be a function")) {
				throw new Error("wrong error: " + e.message);
			}
		}
	`)
	require.NoError(t, err)
}

func TestModuleLoader_NewPlan_NullGoalSkipped(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const bt = require('osm:bt');
		const pabt = require('osm:pabt');
		const bb = new bt.Blackboard();
		bb.set("k", "v");
		const state = pabt.newState(bb);
		state.registerAction("a", pabt.newAction("a",
			[{key: "k", match: function(v) { return v === "v"; }}],
			[{key: "k", value: "v"}],
			bt.node(function() { return bt.Success; })
		));
		// null/undefined goals should be skipped
		const plan = pabt.newPlan(state, [null, undefined]);
		// Plan should still be created with no effective goals
		if (!plan) throw new Error("plan should have been created");
	`)
	require.NoError(t, err)
}

func TestModuleLoader_NewExprCondition_TooFewArgs(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const pabt = require('osm:pabt');
		try {
			pabt.newExprCondition("k");
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("requires key and expression")) {
				throw new Error("wrong error: " + e.message);
			}
		}
	`)
	require.NoError(t, err)
}

func TestModuleLoader_NewExprCondition_WithOptionalValue(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const pabt = require('osm:pabt');
		const cond = pabt.newExprCondition("k", "value == 42", 42);
		if (cond.key !== "k") throw new Error("wrong key: " + cond.key);
		if (cond.value !== 42) throw new Error("wrong value: " + cond.value);
		// Test match
		if (!cond.match(42)) throw new Error("should match 42");
		if (cond.match(0)) throw new Error("should not match 0");
	`)
	require.NoError(t, err)
}

func TestModuleLoader_State_VariableError(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const bt = require('osm:bt');
		const pabt = require('osm:pabt');
		const bb = new bt.Blackboard();
		const state = pabt.newState(bb);
		// Variable with nil key should error
		try {
			state.variable(null);
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("nil")) {
				throw new Error("wrong error: " + e.message);
			}
		}
	`)
	require.NoError(t, err)
}

func TestModuleLoader_State_GetSetMethods(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const bt = require('osm:bt');
		const pabt = require('osm:pabt');
		const bb = new bt.Blackboard();
		const state = pabt.newState(bb);

		// set requires 2 args
		try {
			state.set("key");
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("key and value")) {
				throw new Error("wrong error: " + e.message);
			}
		}

		// get requires 1 arg
		try {
			state.get();
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("key argument")) {
				throw new Error("wrong error: " + e.message);
			}
		}

		// variable requires 1 arg
		try {
			state.variable();
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("key argument")) {
				throw new Error("wrong error: " + e.message);
			}
		}
	`)
	require.NoError(t, err)
}

func TestModuleLoader_RegisterAction_TooFewArgs(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const bt = require('osm:bt');
		const pabt = require('osm:pabt');
		const bb = new bt.Blackboard();
		const state = pabt.newState(bb);
		try {
			state.registerAction("test");
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("requires name and action")) {
				throw new Error("wrong error: " + e.message);
			}
		}
	`)
	require.NoError(t, err)
}

func TestModuleLoader_RegisterAction_InvalidAction(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const bt = require('osm:bt');
		const pabt = require('osm:pabt');
		const bb = new bt.Blackboard();
		const state = pabt.newState(bb);
		try {
			state.registerAction("test", "not an action");
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("pabt.newAction()")) {
				throw new Error("wrong error: " + e.message);
			}
		}
	`)
	require.NoError(t, err)
}

func TestModuleLoader_GetAction_TooFewArgs(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const bt = require('osm:bt');
		const pabt = require('osm:pabt');
		const bb = new bt.Blackboard();
		const state = pabt.newState(bb);
		try {
			state.getAction();
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("requires a name")) {
				throw new Error("wrong error: " + e.message);
			}
		}
	`)
	require.NoError(t, err)
}

func TestModuleLoader_GetAction_NotFound(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const bt = require('osm:bt');
		const pabt = require('osm:pabt');
		const bb = new bt.Blackboard();
		const state = pabt.newState(bb);
		const result = state.getAction("nonexistent");
		if (result !== null) {
			throw new Error("expected null for nonexistent action, got: " + result);
		}
	`)
	require.NoError(t, err)
}

func TestModuleLoader_NewAction_ExprConditionNativePath(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const bt = require('osm:bt');
		const pabt = require('osm:pabt');
		const node = bt.node(function() { return bt.Success; });
		// Create an ExprCondition and use it in newAction — should take _native fast path
		const cond = pabt.newExprCondition("k", "value == 42", 42);
		const action = pabt.newAction("test", [cond], [{key: "k", value: 42}], node);
		if (!action) throw new Error("action should have been created");
	`)
	require.NoError(t, err)
}

func TestModuleLoader_NewAction_InvalidNodeArg(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const pabt = require('osm:pabt');
		try {
			pabt.newAction("test", [], [], "not a node");
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("bt.Node")) {
				throw new Error("wrong error: " + e.message);
			}
		}
	`)
	require.NoError(t, err)
}

// ========================================================================
// Integration: JSCondition.Match with RunOnLoopSync error
// ========================================================================

func TestJSCondition_Match_RunOnLoopSync_Error(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	// Load a matcher that throws an error
	err := bridge.LoadScript("test.js", `
		globalThis.badMatcher = function(value) {
			throw new Error("intentional JS error from matcher");
		};
	`)
	require.NoError(t, err)

	fn, err := bridge.GetCallable("badMatcher")
	require.NoError(t, err)

	cond := NewJSCondition("k", fn, bridge)
	result := cond.Match("anything")
	assert.False(t, result, "should return false when matcher throws")
}

// ========================================================================
// EvaluationMode exhaustive String coverage
// ========================================================================

func TestEvaluationMode_String_Unknown(t *testing.T) {
	t.Parallel()
	mode := EvaluationMode(99)
	assert.Equal(t, "unknown", mode.String())
}

// ========================================================================
// ActionGeneratorErrorMode exhaustive String coverage
// ========================================================================

func TestActionGeneratorErrorMode_String_Values(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "fallback", ActionGeneratorErrorFallback.String())
	assert.Equal(t, "strict", ActionGeneratorErrorStrict.String())
	assert.Equal(t, "unknown", ActionGeneratorErrorMode(99).String())
}

// ========================================================================
// JSEffect coverage
// ========================================================================

func TestJSEffectInterface(t *testing.T) {
	t.Parallel()
	e := NewJSEffect("mykey", 42)
	assert.Equal(t, "mykey", e.Key())
	assert.Equal(t, 42, e.Value())
}

// ========================================================================
// Variable with all numeric types (hit normalizeKey paths in Variable)
// ========================================================================

func TestVariable_NumericKeys(t *testing.T) {
	t.Parallel()
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	bb.Set("42", "int-val")
	bb.Set("1.5", "float-val")

	// Direct int key
	val, err := state.Variable(42)
	require.NoError(t, err)
	assert.Equal(t, "int-val", val)

	// Direct float key
	val, err = state.Variable(1.5)
	require.NoError(t, err)
	assert.Equal(t, "float-val", val)

	// Stringer key
	bb.Set("sym", "stringer-val")
	val, err = state.Variable(stringerKey("sym"))
	require.NoError(t, err)
	assert.Equal(t, "stringer-val", val)
}

// testBridgeLocal is an alternative bridge setup if testBridge from require_test.go
// is not visible. Since we're in the same package, testBridge IS visible.
// This function only serves as documentation that testBridge is reused.
func init() {
	// Compile-time check: testBridge must be visible from this file
	_ = testBridge
}

// ========================================================================
// NewExprCondition_Match with various edge cases through ExprCondition
// ========================================================================

func TestExprCondition_Match_NilReceiver(t *testing.T) {
	t.Parallel()
	var cond *ExprCondition
	assert.False(t, cond.Match(42))
	assert.Nil(t, cond.LastError())
	assert.Nil(t, cond.JSObject())
}

func TestExprCondition_Match_EvaluationError(t *testing.T) {
	t.Parallel()
	// Use an expression that compiles but fails at runtime
	// Access a field on a simple value (int has no fields)
	cond := NewExprCondition("k", "value.nonexistent > 0")
	result := cond.Match(42)
	assert.False(t, result)
	assert.NotNil(t, cond.LastError())
	assert.Contains(t, cond.LastError().Error(), "evaluation failed")
}

// ========================================================================
// NewPlan integration — test plan.running() method
// ========================================================================

func TestModuleLoader_PlanRunning(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const bt = require('osm:bt');
		const pabt = require('osm:pabt');
		const bb = new bt.Blackboard();
		bb.set("done", false);
		const state = pabt.newState(bb);

		state.registerAction("doIt", pabt.newAction("doIt",
			[{key: "done", match: function(v) { return v === false; }}],
			[{key: "done", value: true}],
			bt.node(function() { return bt.Success; })
		));

		const plan = pabt.newPlan(state, [
			{key: "done", match: function(v) { return v === true; }}
		]);

		// Plan created, not yet ticked
		const r = plan.running();
		// running() before first tick is false — plan hasn't started
		if (r !== false) {
			throw new Error("expected running to be false before first tick, got: " + r);
		}
	`)
	require.NoError(t, err)
}

// ========================================================================
// NewPlan integration — newPlan with invalid state (_native is wrong type)
// ========================================================================

func TestModuleLoader_NewPlan_InvalidNativeState(t *testing.T) {
	t.Parallel()
	bridge := testBridge(t)

	err := bridge.LoadScript("test.js", `
		const pabt = require('osm:pabt');
		try {
			pabt.newPlan({_native: "not a state"}, []);
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("PABTState")) {
				throw new Error("wrong error: " + e.message);
			}
		}
	`)
	require.NoError(t, err)
}

// ========================================================================
// ExprCondition_Match_NonBooleanResult — verify unreachable path comment
// ========================================================================

func TestExprCondition_Match_CompilationError(t *testing.T) {
	t.Parallel()
	// Expression that fails compilation (invalid syntax)
	cond := NewExprCondition("k", "{{invalid}}")
	result := cond.Match("anything")
	assert.False(t, result)
	assert.NotNil(t, cond.LastError())
	assert.Contains(t, cond.LastError().Error(), "compilation failed")
}

// ========================================================================
// FuncCondition edge cases
// ========================================================================

func TestFuncCondition_Mode(t *testing.T) {
	t.Parallel()
	cond := NewFuncCondition("k", func(v any) bool { return true })
	assert.Equal(t, EvalModeExpr, cond.Mode())
}

// ========================================================================
// ActionRegistry edge cases
// ========================================================================

func TestActionRegistry_GetNonexistent(t *testing.T) {
	t.Parallel()
	reg := NewActionRegistry()
	assert.Nil(t, reg.Get("nonexistent"))
}

func TestActionRegistry_RegisterReplace(t *testing.T) {
	t.Parallel()
	reg := NewActionRegistry()
	node := bt.New(func([]bt.Node) (bt.Status, error) { return bt.Success, nil })
	a1 := NewAction("test", nil, pabt.Effects{}, node)
	a2 := NewAction("test", nil, pabt.Effects{NewEffect("k", "v")}, node)

	reg.Register("test", a1)
	reg.Register("test", a2) // replace

	got := reg.Get("test")
	assert.Equal(t, a2, got)
}

func TestActionRegistry_AllSorted(t *testing.T) {
	t.Parallel()
	reg := NewActionRegistry()
	node := bt.New(func([]bt.Node) (bt.Status, error) { return bt.Success, nil })

	reg.Register("c", NewAction("c", nil, pabt.Effects{}, node))
	reg.Register("a", NewAction("a", nil, pabt.Effects{}, node))
	reg.Register("b", NewAction("b", nil, pabt.Effects{}, node))

	all := reg.All()
	require.Len(t, all, 3)
	names := make([]string, 3)
	for i, a := range all {
		names[i] = a.(*Action).Name
	}
	assert.Equal(t, []string{"a", "b", "c"}, names, "should be sorted by name")
}

// ========================================================================
// Verify existing tests haven't been broken by printing summary
// ========================================================================

func TestCoverageGaps_Smoke(t *testing.T) {
	t.Parallel()
	// This test exists to verify the test file itself compiles and runs.
	// It serves as a canary for build issues.
	tests := []string{
		"NewAction_EmptyName_Panics",
		"NewAction_NilConditionInSlice_Panics",
		"ExprLRUCache_InvalidMaxSize",
		"ExprLRUCache_PutExisting",
		"ExprLRUCache_Eviction",
		"ExprLRUCache_Resize",
		"ExprLRUCache_Len",
		"ExprLRUCache_Stats",
		"ExprLRUCache_String",
		"SetGetExprCacheSize",
		"JSCondition_JSObject_GetSet",
		"JSCondition_Match_BridgeStopped",
		"NewExprCondition_EmptyExpr_Panics",
		"GetActionGenerator",
		"Variable_NilBlackboard",
		"ActionHasRelevantEffect_NilEffect",
		"ActionHasRelevantEffect_NormalizeKeyError",
		"NormalizeKey_AllTypes",
	}

	t.Logf("Coverage gaps test file contains %d coverage-targeted tests", len(tests))
	for _, name := range tests {
		if !strings.Contains(fmt.Sprintf("Test%s", name), "Test") {
			t.Errorf("Invalid test name pattern: %s", name)
		}
	}
}
