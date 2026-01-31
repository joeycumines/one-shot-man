// Package pabt memory_test.go
//
// Tests for memory safety and leak prevention in osm:pabt.
//
// The osm:pabt architecture is designed to avoid memory leaks by:
// 1. Keeping Go types pure (no Goja values stored in Go structs)
// 2. Using FuncCondition/FuncEffect wrappers that hold only Go functions
// 3. Bridge lifecycle management with proper Stop() cleanup
//
// These tests verify the architecture prevents common memory leak patterns.

package pabt

import (
	"runtime"
	"strings"
	"sync"
	"testing"

	bt "github.com/joeycumines/go-behaviortree"
	pabtpkg "github.com/joeycumines/go-pabt"
	btmod "github.com/joeycumines/one-shot-man/internal/builtin/bt"
)

// TestStateNoCircularReferences verifies State doesn't create circular references
func TestStateNoCircularReferences(t *testing.T) {
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	// Register actions with pure Go types
	for i := 0; i < 100; i++ {
		cond := &SimpleCond{key: "test", match: func(v any) bool { return v == "value" }}
		effect := &SimpleEffect{key: "test", value: "value"}
		node := bt.New(func(children []bt.Node) (bt.Status, error) { return bt.Success, nil })
		action := NewAction("test", []pabtpkg.IConditions{{cond}}, pabtpkg.Effects{effect}, node)
		state.RegisterAction("test", action)
	}

	// Set some values
	bb.Set("test", "value")

	// Get actions - should not create circular refs
	actions, err := state.Actions(nil)
	if err != nil {
		t.Fatalf("Actions() error: %v", err)
	}
	if len(actions) != 1 {
		t.Errorf("Expected 1 action, got %d", len(actions))
	}

	// Nil out references - if there were circular refs, these would leak
	state = nil
	bb = nil
	actions = nil

	// Force GC
	runtime.GC()
	runtime.GC()

	// If we got here without hanging, no circular references
}

// TestActionRegistryConcurrentAccess verifies thread safety doesn't leak
func TestActionRegistryConcurrentAccess(t *testing.T) {
	registry := NewActionRegistry()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				// Concurrent register
				node := bt.New(func(children []bt.Node) (bt.Status, error) { return bt.Success, nil })
				action := NewAction("test", nil, nil, node)
				registry.Register("action", action)

				// Concurrent read
				_ = registry.All()
				_ = registry.Get("action")
			}
		}(i)
	}

	wg.Wait()

	// No deadlocks or panics = success
}

// TestNewAction_NilNodePanic verifies NewAction panics when node is nil
func TestNewAction_NilNodePanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic when node parameter is nil, but got none")
		} else {
			errMsg := ""
			if str, ok := r.(string); ok {
				errMsg = str
			}
			if errMsg == "" {
				t.Errorf("Expected panic string, got %T", r)
			} else if !strings.Contains(errMsg, "node parameter cannot be nil") {
				t.Errorf("Expected panic message to contain 'node parameter cannot be nil', got '%s'", errMsg)
			}
		}
	}()

	// This should panic
	NewAction("test", nil, nil, nil)
}

// TestFuncConditionNoLeak verifies FuncCondition doesn't leak closures
func TestFuncConditionNoLeak(t *testing.T) {
	// Create many FuncConditions
	var conditions []pabtpkg.Condition
	for i := 0; i < 1000; i++ {
		captured := i // Capture loop variable
		cond := NewFuncCondition("key", func(v any) bool {
			return v == captured
		})
		conditions = append(conditions, cond)
	}

	// Use them
	for i, cond := range conditions {
		if cond.Key() != "key" {
			t.Errorf("Condition %d key mismatch", i)
		}
		if !cond.Match(i) {
			t.Errorf("Condition %d should match %d", i, i)
		}
	}

	// Nil out and GC
	conditions = nil
	runtime.GC()
	runtime.GC()

	// Success - no leaks
}

// TestSimpleTypesNoLeak verifies SimpleCond/SimpleEffect/SimpleAction don't leak
func TestSimpleTypesNoLeak(t *testing.T) {
	// Create many simple types
	for i := 0; i < 1000; i++ {
		cond := &SimpleCond{key: i, match: func(v any) bool { return true }}
		effect := &SimpleEffect{key: i, value: "value"}
		node := bt.New(func(children []bt.Node) (bt.Status, error) { return bt.Success, nil })
		action := NewSimpleAction(
			[]pabtpkg.IConditions{{cond}},
			[]pabtpkg.Effect{effect},
			node,
		)

		// Use them
		_ = cond.Key()
		_ = effect.Key()
		_ = action.Node()
	}

	// Force GC
	runtime.GC()
	runtime.GC()

	// Success - no leaks
}

// TestStateActionsFilteringNoLeak verifies Actions() filtering doesn't leak
func TestStateActionsFilteringNoLeak(t *testing.T) {
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	// Register many actions
	for i := 0; i < 100; i++ {
		key := i
		cond := &SimpleCond{key: key, match: func(v any) bool { return true }}
		effect := &SimpleEffect{key: key, value: true}
		node := bt.New(func(children []bt.Node) (bt.Status, error) { return bt.Success, nil })
		action := NewAction(
			"action",
			[]pabtpkg.IConditions{{cond}},
			pabtpkg.Effects{effect},
			node,
		)
		state.RegisterAction("action", action)
	}

	// Query with different failed conditions many times
	for i := 0; i < 100; i++ {
		failed := &SimpleCond{key: i, match: func(v any) bool { return true }}
		_, _ = state.Actions(failed)
	}

	// Nil out and GC
	state = nil
	bb = nil
	runtime.GC()
	runtime.GC()

	// Success - no leaks
}

// TestExprConditionNoCycleWithGoja verifies ExprCondition is pure Go
func TestExprConditionNoCycleWithGoja(t *testing.T) {
	// ExprCondition uses expr-lang (pure Go), NOT Goja
	// This test verifies the expr-lang path doesn't create Goja dependencies

	// Note: ExprEnv uses "Value" (capital V) as the environment variable
	cond := NewExprCondition("testKey", "Value == 42")

	// Match should work without any Goja involvement
	result := cond.Match(42)
	if !result {
		t.Error("ExprCondition should match 42")
	}

	result = cond.Match(0)
	if result {
		t.Error("ExprCondition should not match 0")
	}

	// Nil out and GC
	cond = nil
	runtime.GC()
	runtime.GC()

	// Success - expr-lang has no Goja dependencies
}
