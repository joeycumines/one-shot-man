package pabt

import (
	"sync"

	bt "github.com/joeycumines/go-behaviortree"
	pabtpkg "github.com/joeycumines/go-pabt"
)

// ActionRegistry provides thread-safe storage for PA-BT actions.
// Uses the non-generic pabtpkg.IAction type for easier interop.
type ActionRegistry struct {
	mu      sync.RWMutex
	actions map[string]pabtpkg.IAction
}

// NewActionRegistry creates a new empty action registry.
func NewActionRegistry() *ActionRegistry {
	return &ActionRegistry{
		actions: make(map[string]pabtpkg.IAction),
	}
}

// Register adds an action to the registry with the given name.
// Replaces existing action with same name.
func (r *ActionRegistry) Register(name string, action pabtpkg.IAction) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actions[name] = action
}

// Get retrieves an action by name.
// Returns nil if action not found.
func (r *ActionRegistry) Get(name string) pabtpkg.IAction {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.actions[name]
}

// All returns all registered actions.
func (r *ActionRegistry) All() []pabtpkg.IAction {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]pabtpkg.IAction, 0, len(r.actions))
	for _, action := range r.actions {
		result = append(result, action)
	}
	return result
}

// Action implements pabtpkg.IAction interface by wrapping bt.Node.
// It represents a planning action with preconditions, effects, and behavior tree node.
type Action struct {
	// Name of the action (for debugging/identification)
	Name string

	// Preconditions that must all pass before action can execute
	// Each IConditions slice is an AND group, all groups are OR logic
	conditions []pabtpkg.IConditions

	// Effects that this action achieves (what it changes in state)
	effects pabtpkg.Effects

	// Behavior tree node that implements the action's logic
	node bt.Node
}

// Conditions implements pabtpkg.IAction.Conditions().
// Returns preconditions (each slice is an AND group, groups are OR logic).
func (a *Action) Conditions() []pabtpkg.IConditions {
	return a.conditions
}

// Effects implements pabtpkg.IAction.Effects().
// Returns what this action achieves in the state.
func (a *Action) Effects() pabtpkg.Effects {
	return a.effects
}

// Node implements pabtpkg.IAction.Node().
// Returns the behavior tree node that implements this action's logic.
func (a *Action) Node() bt.Node {
	return a.node
}

// NewAction creates a new Action with the specified components.
// This is the preferred factory function for creating actions.
//
// Parameters:
//   - name: Unique identifier for debugging/logging
//   - conditions: Preconditions (each slice is AND group, groups are OR logic)
//   - effects: What this action achieves in the state
//   - node: Behavior tree node implementing the action's logic
func NewAction(name string, conditions []pabtpkg.IConditions, effects pabtpkg.Effects, node bt.Node) *Action {
	return &Action{
		Name:       name,
		conditions: conditions,
		effects:    effects,
		node:       node,
	}
}
