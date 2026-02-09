package pabt

import (
	"testing"

	bt "github.com/joeycumines/go-behaviortree"
	pabtpkg "github.com/joeycumines/go-pabt"
)

func TestSimpleCond_Key(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  any
	}{
		{"string key", "myKey"},
		{"int key", 42},
		{"struct key", struct{ Name string }{"test"}},
		{"nil key", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := NewSimpleCond(tt.key, nil)
			if got := cond.Key(); got != tt.key {
				t.Errorf("Key() = %v, want %v", got, tt.key)
			}
		})
	}
}

func TestSimpleCond_Match(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		match func(any) bool
		value any
		want  bool
	}{
		{
			name:  "nil match function returns false",
			match: nil,
			value: "anything",
			want:  false,
		},
		{
			name:  "equality match - true",
			match: func(v any) bool { return v == "expected" },
			value: "expected",
			want:  true,
		},
		{
			name:  "equality match - false",
			match: func(v any) bool { return v == "expected" },
			value: "different",
			want:  false,
		},
		{
			name:  "type assertion match",
			match: func(v any) bool { n, ok := v.(int); return ok && n > 10 },
			value: 15,
			want:  true,
		},
		{
			name:  "type assertion match - wrong type",
			match: func(v any) bool { n, ok := v.(int); return ok && n > 10 },
			value: "not an int",
			want:  false,
		},
		{
			name:  "nil value check",
			match: func(v any) bool { return v == nil },
			value: nil,
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := NewSimpleCond("key", tt.match)
			if got := cond.Match(tt.value); got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSimpleCond_ImplementsCondition(t *testing.T) {
	t.Parallel()
	var _ pabtpkg.Condition = (*SimpleCond)(nil)
}

func TestSimpleEffect_KeyValue(t *testing.T) {
	t.Parallel()

	type customKey struct {
		ID int
	}

	type customValue struct {
		X, Y int
	}

	tests := []struct {
		name  string
		key   any
		value any
	}{
		{"string key and value", "position", "10,20"},
		{"struct key and value", customKey{ID: 1}, customValue{X: 10, Y: 20}},
		{"int key, nil value", 42, nil},
		{"nil key and value", nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			effect := NewSimpleEffect(tt.key, tt.value)
			if got := effect.Key(); got != tt.key {
				t.Errorf("Key() = %v, want %v", got, tt.key)
			}
			if got := effect.Value(); got != tt.value {
				t.Errorf("Value() = %v, want %v", got, tt.value)
			}
		})
	}
}

func TestSimpleEffect_ImplementsEffect(t *testing.T) {
	t.Parallel()
	var _ pabtpkg.Effect = (*SimpleEffect)(nil)
}

func TestSimpleAction_Conditions(t *testing.T) {
	t.Parallel()

	cond1 := NewSimpleCond("key1", func(v any) bool { return true })
	cond2 := NewSimpleCond("key2", func(v any) bool { return true })
	cond3 := NewSimpleCond("key3", func(v any) bool { return true })

	action := NewSimpleAction(
		[]pabtpkg.IConditions{
			{cond1, cond2}, // AND group 1
			{cond3},        // AND group 2 (OR'd with group 1)
		},
		nil,
		nil,
	)

	conditions := action.Conditions()
	if len(conditions) != 2 {
		t.Fatalf("Conditions() returned %d groups, want 2", len(conditions))
	}
	if len(conditions[0]) != 2 {
		t.Errorf("First group has %d conditions, want 2", len(conditions[0]))
	}
	if len(conditions[1]) != 1 {
		t.Errorf("Second group has %d conditions, want 1", len(conditions[1]))
	}
}

func TestSimpleAction_Effects(t *testing.T) {
	t.Parallel()

	effect1 := NewSimpleEffect("key1", "value1")
	effect2 := NewSimpleEffect("key2", "value2")

	action := NewSimpleAction(
		nil,
		pabtpkg.Effects{effect1, effect2},
		nil,
	)

	effects := action.Effects()
	if len(effects) != 2 {
		t.Fatalf("Effects() returned %d effects, want 2", len(effects))
	}
}

func TestSimpleAction_Node(t *testing.T) {
	t.Parallel()

	node := bt.New(bt.Sequence)
	action := NewSimpleAction(nil, nil, node)

	// bt.Node is a function type, so we can only check for nil
	if action.Node() == nil {
		t.Error("Node() should not be nil")
	}
}

func TestSimpleAction_NilComponents(t *testing.T) {
	t.Parallel()

	action := NewSimpleAction(nil, nil, nil)

	if action.Conditions() != nil {
		t.Error("Conditions() should be nil")
	}
	if action.Effects() != nil {
		t.Error("Effects() should be nil")
	}
	if action.Node() != nil {
		t.Error("Node() should be nil")
	}
}

func TestSimpleAction_ImplementsIAction(t *testing.T) {
	t.Parallel()
	var _ pabtpkg.IAction = (*SimpleAction)(nil)
}

func TestSimpleActionBuilder(t *testing.T) {
	t.Parallel()

	node := bt.New(bt.Sequence)
	action := NewActionBuilder().
		WithConditions(NewSimpleCond("key1", func(v any) bool { return true })).
		WithConditions(NewSimpleCond("key2", func(v any) bool { return true })).
		WithEffect("effectKey1", "effectValue1").
		WithEffect("effectKey2", "effectValue2").
		WithNode(node).
		Build()

	if len(action.Conditions()) != 2 {
		t.Errorf("Expected 2 condition groups, got %d", len(action.Conditions()))
	}
	if len(action.Effects()) != 2 {
		t.Errorf("Expected 2 effects, got %d", len(action.Effects()))
	}
	// bt.Node is a function type, so we can only check for nil
	if action.Node() == nil {
		t.Error("Node should not be nil")
	}
}

func TestEqualityCond(t *testing.T) {
	t.Parallel()

	cond := EqualityCond("myKey", "expected")

	if cond.Key() != "myKey" {
		t.Errorf("Key() = %v, want 'myKey'", cond.Key())
	}
	if !cond.Match("expected") {
		t.Error("Match('expected') should be true")
	}
	if cond.Match("other") {
		t.Error("Match('other') should be false")
	}
	if cond.Match(nil) {
		t.Error("Match(nil) should be false")
	}
}

func TestNotNilCond(t *testing.T) {
	t.Parallel()

	cond := NotNilCond("key")

	if !cond.Match("value") {
		t.Error("Match('value') should be true")
	}
	if !cond.Match(0) {
		t.Error("Match(0) should be true")
	}
	if cond.Match(nil) {
		t.Error("Match(nil) should be false")
	}
}

func TestNilCond(t *testing.T) {
	t.Parallel()

	cond := NilCond("key")

	if cond.Match("value") {
		t.Error("Match('value') should be false")
	}
	if !cond.Match(nil) {
		t.Error("Match(nil) should be true")
	}
}
