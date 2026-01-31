package pabt

import (
	"sync"
	"testing"

	pabtpkg "github.com/joeycumines/go-pabt"
	btmod "github.com/joeycumines/one-shot-man/internal/builtin/bt"
)

func TestNewState(t *testing.T) {
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	if state == nil {
		t.Fatal("NewState returned nil")
	}
	if state.Blackboard != bb {
		t.Error("Blackboard not correctly assigned")
	}
	if state.actions == nil {
		t.Error("ActionRegistry not initialized")
	}
}

func TestState_Variable_NilKey(t *testing.T) {
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	_, err := state.Variable(nil)
	if err == nil {
		t.Error("expected error for nil key")
	}
}

func TestState_Variable_StringKey(t *testing.T) {
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	// Set value directly on blackboard
	bb.Set("foo", 42)

	val, err := state.Variable("foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 42 {
		t.Errorf("expected 42, got %v", val)
	}

	// Non-existent key returns nil (pabt semantics)
	val, err = state.Variable("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != nil {
		t.Errorf("expected nil for nonexistent key, got %v", val)
	}
}

func TestState_Variable_IntKey(t *testing.T) {
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	bb.Set("123", "value")

	val, err := state.Variable(123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "value" {
		t.Errorf("expected 'value', got %v", val)
	}
}

func TestState_Variable_FloatKey(t *testing.T) {
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	bb.Set("3.14", "pi")

	val, err := state.Variable(3.14)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "pi" {
		t.Errorf("expected 'pi', got %v", val)
	}
}

// mockStringer implements fmt.Stringer for testing
type mockStringer struct {
	value string
}

func (m mockStringer) String() string {
	return m.value
}

func TestState_Variable_StringerKey(t *testing.T) {
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	bb.Set("my-key", "my-value")

	val, err := state.Variable(mockStringer{"my-key"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "my-value" {
		t.Errorf("expected 'my-value', got %v", val)
	}
}

// unsupportedKeyType is a type that cannot be converted to string
type unsupportedKeyType struct{}

func TestState_Variable_UnsupportedKey(t *testing.T) {
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	_, err := state.Variable(unsupportedKeyType{})
	if err == nil {
		t.Error("expected error for unsupported key type")
	}
}

func TestState_Actions(t *testing.T) {
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	// Initially empty (pass nil for the failed condition)
	actions, err := state.Actions(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 0 {
		t.Errorf("expected 0 actions, got %d", len(actions))
	}

	// Register an action that's always executable (no conditions)
	action1 := &Action{
		Name: "Action1",
	}
	state.RegisterAction("Action1", action1)

	actions, err = state.Actions(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(actions))
	}
}

func TestState_RegisterAction(t *testing.T) {
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	action := &Action{
		Name: "TestAction",
	}

	state.RegisterAction("TestAction", action)

	actions, err := state.Actions(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].(*Action).Name != "TestAction" {
		t.Errorf("expected 'TestAction', got '%s'", actions[0].(*Action).Name)
	}
}

func TestState_ConcurrentAccess(t *testing.T) {
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	// Set initial value
	bb.Set("counter", 0)

	// Concurrent reads
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, err := state.Variable("counter")
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		}()
	}
	wg.Wait()
}

func TestState_Variable_AllIntTypes(t *testing.T) {
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	testCases := []struct {
		key      any
		expected string
	}{
		{int(42), "42"},
		{int8(42), "42"},
		{int16(42), "42"},
		{int32(42), "42"},
		{int64(42), "42"},
		{uint(42), "42"},
		{uint8(42), "42"},
		{uint16(42), "42"},
		{uint32(42), "42"},
		{uint64(42), "42"},
	}

	for _, tc := range testCases {
		bb.Set(tc.expected, "value")
		val, err := state.Variable(tc.key)
		if err != nil {
			t.Errorf("unexpected error for %T: %v", tc.key, err)
		}
		if val != "value" {
			t.Errorf("expected 'value' for %T, got %v", tc.key, val)
		}
	}
}

func TestState_canExecuteAction_EmptyConditions(t *testing.T) {
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	// Action with no conditions is always executable
	action := &Action{
		Name:       "NoConditions",
		conditions: []pabtpkg.IConditions{},
	}

	if !canExecuteAction(state, action) {
		t.Error("action with no conditions should be executable")
	}
}

func TestState_canExecuteAction_PassingConditions(t *testing.T) {
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	bb.Set("key1", "expected")

	// Create action with passing condition
	cond := NewSimpleCond("key1", func(v any) bool {
		return v == "expected"
	})
	action := NewSimpleAction(
		[]pabtpkg.IConditions{{cond}},
		nil,
		nil,
	)

	if !canExecuteAction(state, action) {
		t.Error("action with passing conditions should be executable")
	}
}

func TestState_canExecuteAction_FailingConditions(t *testing.T) {
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	bb.Set("key1", "wrong")

	// Create action with failing condition
	cond := NewSimpleCond("key1", func(v any) bool {
		return v == "expected"
	})
	action := NewSimpleAction(
		[]pabtpkg.IConditions{{cond}},
		nil,
		nil,
	)

	if canExecuteAction(state, action) {
		t.Error("action with failing conditions should not be executable")
	}
}

func TestState_canExecuteAction_ORGroups(t *testing.T) {
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	bb.Set("key1", "wrong")
	bb.Set("key2", "expected")

	// Create action with two OR groups - second should pass
	cond1 := NewSimpleCond("key1", func(v any) bool {
		return v == "expected"
	})
	cond2 := NewSimpleCond("key2", func(v any) bool {
		return v == "expected"
	})

	action := NewSimpleAction(
		[]pabtpkg.IConditions{
			{cond1}, // This group fails
			{cond2}, // This group passes
		},
		nil,
		nil,
	)

	if !canExecuteAction(state, action) {
		t.Error("action should be executable when at least one OR group passes")
	}
}

func TestState_canExecuteAction_ANDWithinGroup(t *testing.T) {
	bb := new(btmod.Blackboard)
	state := NewState(bb)

	bb.Set("key1", "expected1")
	bb.Set("key2", "wrong")

	// Create action with AND conditions within a group
	cond1 := NewSimpleCond("key1", func(v any) bool {
		return v == "expected1"
	})
	cond2 := NewSimpleCond("key2", func(v any) bool {
		return v == "expected2"
	})

	action := NewSimpleAction(
		[]pabtpkg.IConditions{
			{cond1, cond2}, // Both must pass - second fails
		},
		nil,
		nil,
	)

	if canExecuteAction(state, action) {
		t.Error("action should not be executable when any AND condition fails")
	}
}
