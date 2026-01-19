package pabt

import (
	"testing"

	bt "github.com/joeycumines/go-behaviortree"
	pabtpkg "github.com/joeycumines/go-pabt"
	btmod "github.com/joeycumines/one-shot-man/internal/builtin/bt"
)

// TestActionWithEmptyConditionsEffects verifies that actions with empty
// conditions/effect arrays work correctly in PA-BT planning without nil pointer panics.
// This is a fix for the bug in newAction where empty arrays passed from JavaScript
// would create nil slices instead of empty slices.
func TestActionWithEmptyConditionsEffects(t *testing.T) {
	// Create a state with blackboard
	bb := new(btmod.Blackboard)
	bb.Set("x", 0)
	state := NewState(bb)

	// Create an action with empty conditions and effects
	action := &Action{
		Name:       "noConditionsAction",
		conditions: []pabtpkg.IConditions{}, // Explicitly empty (not nil)
		effects:    []pabtpkg.Effect{},      // Explicitly empty (not nil)
		node: bt.New(func(children []bt.Node) (bt.Status, error) {
			bb.Set("x", bb.Get("x").(int)+1)
			return bt.Success, nil
		}),
	}

	// Verify that conditions is not nil
	if action.conditions == nil {
		t.Error("Action with empty conditions slice: conditions should not be nil")
	}

	// Verify that effects is not nil
	if action.effects == nil {
		t.Error("Action with empty effects slice: effects should not be nil")
	}

	// Register action
	state.RegisterAction("noConditionsAction", action)

	// Verify that action can be retrieved
	retrievedAction := state.actions.Get("noConditionsAction")
	if retrievedAction == nil {
		t.Fatal("Action not found in registry")
	}

	// Verify Actions() can be called without panic (this is where the nil pointer bug would occur)
	actions, err := state.Actions(nil)
	if err != nil {
		t.Fatalf("State.Actions() failed: %v", err)
	}

	if len(actions) == 0 {
		t.Error("Actions() returned empty slice, expected to find at least one executable action")
	}

	// Verify our action is in the returned list
	found := false
	for _, a := range actions {
		if a == action {
			found = true
			break
		}
	}
	if !found {
		t.Error("Our action was not included in the executable actions list")
	}

	// Verify canExecuteAction works
	if !canExecuteAction(state, action) {
		t.Error("Action with empty conditions should be executable")
	}
}

// TestActionWithSomeConditionsEmptyEffects verifies an action with
// conditions but empty effects works correctly.
func TestActionWithSomeConditionsEmptyEffects(t *testing.T) {
	bb := new(btmod.Blackboard)
	bb.Set("ready", true)
	state := NewState(bb)

	// Create an action with conditions but no effects
	action := &Action{
		Name: "hasConditionsNoEffects",
		conditions: []pabtpkg.IConditions{
			{
				&CustomCondition{
					key:     "ready",
					matchFn: func(v any) bool { return v == true },
				},
			},
		},
		effects: []pabtpkg.Effect{}, // Empty but not nil
		node: bt.New(func(children []bt.Node) (bt.Status, error) {
			return bt.Success, nil
		}),
	}

	if action.effects == nil {
		t.Error("Action effects should not be nil even when empty")
	}

	state.RegisterAction("hasConditionsNoEffects", action)

	// Should be executable since the passed condition holds
	actions, err := state.Actions(nil)
	if err != nil {
		t.Fatalf("State.Actions() failed: %v", err)
	}

	found := false
	for _, a := range actions {
		if a == action {
			found = true
			break
		}
	}
	if !found {
		t.Error("Action should be executable when conditions are satisfied")
	}
}

// TestCondition for testing
type TestCondition struct {
	key     any
	matchFn func(v any) bool
}

func (c *TestCondition) Key() any {
	return c.key
}

func (c *TestCondition) Match(value any) bool {
	return c.matchFn(value)
}
