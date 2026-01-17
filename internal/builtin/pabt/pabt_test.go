package pabt

import (
	"testing"

	bt "github.com/joeycumines/go-behaviortree"
	pabtpkg "github.com/joeycumines/go-pabt"
	btmod "github.com/joeycumines/one-shot-man/internal/builtin/bt"
)

func TestPABTStateInterface(t *testing.T) {
	bb := new(btmod.Blackboard)
	state := NewState(bb)
	bb.Set("test_key", "test_value")

	// Test Variable method
	value, err := state.Variable("test_key")
	if err != nil {
		t.Errorf("Variable returned error: %v", err)
	}
	if value != "test_value" {
		t.Errorf("Variable = %v, want 'test_value'", value)
	}

	// Verify state implements pabtpkg.IState interface (non-generic)
	var _ pabtpkg.IState = state

	// Test that blackboard is properly backed
	if state.Blackboard == nil {
		t.Error("Blackboard is nil")
	}
}

func TestVariableNormalization(t *testing.T) {
	tests := []struct {
		name  string
		key   any
		value any
	}{
		{"string key", "test", "value"},
		{"int key", 42, "int value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bb := new(btmod.Blackboard)
			state := NewState(bb)

			// Set value
			var keyStr string
			switch k := tt.key.(type) {
			case string:
				keyStr = k
			case int:
				keyStr = "42"
			}
			bb.Set(keyStr, tt.value)

			// Get via Variable()
			result, err := state.Variable(tt.key)
			if err != nil {
				t.Errorf("Variable(%v) returned error: %v", tt.key, err)
			}
			if result != tt.value {
				t.Errorf("Variable(%v) = %v, want %v", tt.key, result, tt.value)
			}
		})
	}
}

func TestVariableNotFound(t *testing.T) {
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	// Try to get a key that doesn't exist
	value, err := state.Variable("nonexistent")
	// pabt semantics: returns (nil, nil) for missing keys
	if err != nil {
		t.Errorf("Variable('nonexistent') returned error: %v", err)
	}
	if value != nil {
		t.Errorf("Variable('nonexistent') = %v, want nil", value)
	}
}

func TestActionRegistryThreadSafety(t *testing.T) {
	reg := NewActionRegistry()

	// Create test action
	tick := func(children []bt.Node) (bt.Status, error) {
		return bt.Success, nil
	}
	action := &Action{
		Name: "test",
		node: bt.New(tick),
	}

	// Concurrent registration
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			reg.Register("test", action)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Verify action was registered
	if got := reg.Get("test"); got == nil {
		t.Error("action not found in registry")
	}
}

func TestStateActionsFiltering(t *testing.T) {
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	// Create actions with conditions
	tick1 := func([]bt.Node) (bt.Status, error) {
		return bt.Success, nil
	}
	action1 := &Action{
		Name: "move",
		conditions: []pabtpkg.IConditions{
			{
				&NeverFailsCondition{match: func(v any) bool { return true }},
			},
		},
		node: bt.New(tick1),
	}

	tick2 := func([]bt.Node) (bt.Status, error) {
		return bt.Success, nil
	}
	action2 := &Action{
		Name: "pick",
		conditions: []pabtpkg.IConditions{
			{
				&NeverFailsCondition{match: func(v any) bool { return false }},
			},
		},
		node: bt.New(tick2),
	}

	state.RegisterAction("move", action1)
	state.RegisterAction("pick", action2)

	// Get all actions
	actions, err := state.Actions(nil)
	if err != nil {
		t.Fatalf("Actions() returned error: %v", err)
	}

	// Should only return action1 (matches condition)
	if len(actions) != 1 {
		t.Errorf("Actions() returned %d actions, want 1", len(actions))
	}
}

func TestInitialStateEmptyActions(t *testing.T) {
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	// Test Actions method (should return empty initially)
	actions, err := state.Actions(nil)
	if err != nil {
		t.Errorf("Actions returned error: %v", err)
	}
	if len(actions) != 0 {
		t.Errorf("Actions returned %d actions, want 0", len(actions))
	}
}

// NeverFailsCondition is a test helper.
type NeverFailsCondition struct {
	match func(v any) bool
}

func (c *NeverFailsCondition) Key() any {
	return "test_condition"
}

func (c *NeverFailsCondition) Match(value any) bool {
	return c.match(value)
}
