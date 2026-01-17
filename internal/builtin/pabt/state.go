package pabt

import (
	"fmt"

	pabtpkg "github.com/joeycumines/go-pabt"
	btmod "github.com/joeycumines/one-shot-man/internal/builtin/bt"
)

// State implements pabtpkg.State (which is State[Condition]) interface backed by a bt.Blackboard.
// It normalizes any key type to string for blackboard storage and provides
// access to a registry of actions.
type State struct {
	// Embed bt.Blackboard for storage
	*btmod.Blackboard

	// Registry of available actions
	actions *ActionRegistry
}

// NewState creates a new State backed by the provided blackboard.
func NewState(bb *btmod.Blackboard) *State {
	return &State{
		Blackboard: bb,
		actions:    NewActionRegistry(),
	}
}

// Variable implements pabtpkg.State.Variable(key any).
// It normalizes any type of key to a string for blackboard lookup.
// Returns (nil, nil) if key doesn't exist (pabt semantics).
func (s *State) Variable(key any) (any, error) {
	if key == nil {
		return nil, fmt.Errorf("variable key cannot be nil")
	}

	// Normalize key to string for blackboard
	var keyStr string
	switch k := key.(type) {
	case string:
		keyStr = k
	case int, int8, int16, int32, int64:
		keyStr = fmt.Sprintf("%d", k)
	case uint, uint8, uint16, uint32, uint64:
		keyStr = fmt.Sprintf("%d", k)
	case float32, float64:
		keyStr = fmt.Sprintf("%f", k)
	default:
		// Try fmt.Stringer interface (for pabtpkg.Symbol or custom types)
		if stringer, ok := key.(fmt.Stringer); ok {
			keyStr = stringer.String()
		} else {
			return nil, fmt.Errorf("unsupported key type: %T", key)
		}
	}

	// Get value from blackboard (returns nil if not found, which is correct pabt semantics)
	value := s.Blackboard.Get(keyStr)
	return value, nil
}

// Actions implements pabtpkg.State.Actions(failed Condition).
// Returns all actions that can potentially fix the failed condition.
//
// Note: The 'failed' parameter indicates which condition failed during planning.
// A smarter implementation could filter actions by whether their effects include
// keys related to the failed condition to improve efficiency.
func (s *State) Actions(failed pabtpkg.Condition) ([]pabtpkg.IAction, error) {
	registeredActions := s.actions.All()

	// Filter actions to only those with satisfied conditions
	var playableActions []pabtpkg.IAction
	for _, action := range registeredActions {
		if s.canExecuteAction(action) {
			playableActions = append(playableActions, action)
		}
	}

	return playableActions, nil
}

// canExecuteAction checks if all of an action's conditions pass against current state.
// An action has multiple IConditions groups; at least ONE group must pass (OR logic).
// Within each group, ALL conditions must pass (AND logic).
func (s *State) canExecuteAction(action pabtpkg.IAction) bool {
	allConditions := action.Conditions()

	// Try each IConditions group (there may be zero groups, meaning always executable)
	for _, conditionGroup := range allConditions {
		// Check if all conditions in this group pass
		allMatch := true
		for _, cond := range conditionGroup {
			// Skip nil conditions
			if cond == nil {
				continue
			}
			value, err := s.Variable(cond.Key())
			if err != nil {
				allMatch = false
				break
			}

			if !cond.Match(value) {
				allMatch = false
				break
			}
		}

		// If this group's conditions all pass, the action is executable
		if allMatch {
			return true
		}
	}

	// No condition group passed - action not executable
	return len(allConditions) == 0
}

// RegisterAction adds an action to the state's action registry.
func (s *State) RegisterAction(name string, action pabtpkg.IAction) {
	s.actions.Register(name, action)
}
