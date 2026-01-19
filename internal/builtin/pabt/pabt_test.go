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

	// Create actions with effects
	tick1 := func([]bt.Node) (bt.Status, error) {
		return bt.Success, nil
	}
	action1 := &Action{
		Name: "move",
		effects: []pabtpkg.Effect{
			&SimpleEffect{key: "position", value: "target"},
		},
		node: bt.New(tick1),
	}

	tick2 := func([]bt.Node) (bt.Status, error) {
		return bt.Success, nil
	}
	action2 := &Action{
		Name: "pick",
		effects: []pabtpkg.Effect{
			&SimpleEffect{key: "held_item", value: "cube"},
		},
		node: bt.New(tick2),
	}

	state.RegisterAction("move", action1)
	state.RegisterAction("pick", action2)

	// Test 1: Actions(nil) returns ALL actions (legacy mode)
	allActions, err := state.Actions(nil)
	if err != nil {
		t.Fatalf("Actions(nil) returned error: %v", err)
	}
	if len(allActions) != 2 {
		t.Errorf("Actions(nil) returned %d actions, want 2", len(allActions))
	}

	// Test 2: Actions(failedCondition) filters by effect matching
	// Create a condition that wants position == "target"
	positionCondition := NewSimpleCond("position", func(v any) bool {
		return v == "target"
	})

	filteredActions, err := state.Actions(positionCondition)
	if err != nil {
		t.Fatalf("Actions(positionCondition) returned error: %v", err)
	}
	if len(filteredActions) != 1 {
		t.Fatalf("Actions(positionCondition) returned %d actions, want 1", len(filteredActions))
	}
	if filteredActions[0].(*Action).Name != "move" {
		t.Errorf("Expected 'move' action, got '%s'", filteredActions[0].(*Action).Name)
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

func TestNewAction(t *testing.T) {
	// Test factory creates action correctly
	tick := func([]bt.Node) (bt.Status, error) {
		return bt.Success, nil
	}
	node := bt.New(tick)

	conds := []pabtpkg.IConditions{
		{NewSimpleCond("key1", func(v any) bool { return v == "value1" })},
	}
	effs := pabtpkg.Effects{
		NewSimpleEffect("key2", "value2"),
	}

	action := NewAction("test_action", conds, effs, node)

	// Verify name
	if action.Name != "test_action" {
		t.Errorf("action.Name = %s, want test_action", action.Name)
	}

	// Verify conditions
	gotConds := action.Conditions()
	if len(gotConds) != 1 {
		t.Errorf("len(Conditions()) = %d, want 1", len(gotConds))
	}

	// Verify effects
	gotEffs := action.Effects()
	if len(gotEffs) != 1 {
		t.Errorf("len(Effects()) = %d, want 1", len(gotEffs))
	}
	if gotEffs[0].Key() != "key2" || gotEffs[0].Value() != "value2" {
		t.Errorf("Effects()[0] = (%v, %v), want (key2, value2)", gotEffs[0].Key(), gotEffs[0].Value())
	}

	// Verify node is non-nil (can't compare funcs directly)
	if action.Node() == nil {
		t.Error("Node() is nil")
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
