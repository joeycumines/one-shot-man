package pabt

import (
	bt "github.com/joeycumines/go-behaviortree"
	pabtpkg "github.com/joeycumines/go-pabt"
)

// SimpleCond is a simple condition implementation matching go-pabt simpleCond.
// It holds a key used for lookup and a match function for evaluation.
type SimpleCond struct {
	key   any
	match func(value any) bool
}

var _ pabtpkg.Condition = (*SimpleCond)(nil)

// NewSimpleCond creates a new SimpleCond with the given key and match function.
func NewSimpleCond(key any, match func(value any) bool) *SimpleCond {
	return &SimpleCond{
		key:   key,
		match: match,
	}
}

// Key implements pabt.Condition.Key.
func (c *SimpleCond) Key() any {
	return c.key
}

// Match implements pabt.Condition.Match.
func (c *SimpleCond) Match(value any) bool {
	if c.match == nil {
		return false
	}
	return c.match(value)
}

// SimpleEffect is a simple effect implementation matching go-pabt simpleEffect.
// It holds a key-value pair representing a state change.
type SimpleEffect struct {
	key   any
	value any
}

var _ pabtpkg.Effect = (*SimpleEffect)(nil)

// NewSimpleEffect creates a new SimpleEffect with the given key and value.
func NewSimpleEffect(key, value any) *SimpleEffect {
	return &SimpleEffect{
		key:   key,
		value: value,
	}
}

// Key implements pabt.Effect.Key.
func (e *SimpleEffect) Key() any {
	return e.key
}

// Value implements pabt.Effect.Value.
func (e *SimpleEffect) Value() any {
	return e.value
}

// SimpleAction is a simple action implementation matching go-pabt simpleAction.
// It combines conditions, effects, and a behavior tree node.
type SimpleAction struct {
	conditions []pabtpkg.IConditions
	effects    pabtpkg.Effects
	node       bt.Node
}

var _ pabtpkg.IAction = (*SimpleAction)(nil)

// NewSimpleAction creates a new SimpleAction with the given parameters.
func NewSimpleAction(conditions []pabtpkg.IConditions, effects pabtpkg.Effects, node bt.Node) *SimpleAction {
	return &SimpleAction{
		conditions: conditions,
		effects:    effects,
		node:       node,
	}
}

// Conditions implements pabt.IAction.Conditions.
// Returns the condition groups - each group is an AND, groups are OR'd together.
func (a *SimpleAction) Conditions() []pabtpkg.IConditions {
	return a.conditions
}

// Effects implements pabt.IAction.Effects.
func (a *SimpleAction) Effects() pabtpkg.Effects {
	return a.effects
}

// Node implements pabt.IAction.Node.
func (a *SimpleAction) Node() bt.Node {
	return a.node
}

// SimpleActionBuilder provides a fluent API for building SimpleAction instances.
type SimpleActionBuilder struct {
	conditions []pabtpkg.IConditions
	effects    pabtpkg.Effects
	node       bt.Node
}

// NewActionBuilder creates a new SimpleActionBuilder.
func NewActionBuilder() *SimpleActionBuilder {
	return &SimpleActionBuilder{}
}

// WithConditions adds a condition group (AND logic within group).
func (b *SimpleActionBuilder) WithConditions(conds ...pabtpkg.Condition) *SimpleActionBuilder {
	b.conditions = append(b.conditions, conds)
	return b
}

// WithEffect adds an effect to the action.
func (b *SimpleActionBuilder) WithEffect(key, value any) *SimpleActionBuilder {
	b.effects = append(b.effects, NewSimpleEffect(key, value))
	return b
}

// WithNode sets the behavior tree node for this action.
func (b *SimpleActionBuilder) WithNode(node bt.Node) *SimpleActionBuilder {
	b.node = node
	return b
}

// Build creates the SimpleAction.
func (b *SimpleActionBuilder) Build() *SimpleAction {
	return &SimpleAction{
		conditions: b.conditions,
		effects:    b.effects,
		node:       b.node,
	}
}

// EqualityCond creates a SimpleCond that checks for value equality.
func EqualityCond(key, expected any) *SimpleCond {
	return NewSimpleCond(key, func(value any) bool {
		return value == expected
	})
}

// NotNilCond creates a SimpleCond that checks for non-nil values.
func NotNilCond(key any) *SimpleCond {
	return NewSimpleCond(key, func(value any) bool {
		return value != nil
	})
}

// NilCond creates a SimpleCond that checks for nil values.
func NilCond(key any) *SimpleCond {
	return NewSimpleCond(key, func(value any) bool {
		return value == nil
	})
}
