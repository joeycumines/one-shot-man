package pabt

import (
	"fmt"
	"log/slog"
	"sync"

	pabtpkg "github.com/joeycumines/go-pabt"
	btmod "github.com/joeycumines/one-shot-man/internal/builtin/bt"
)

// Compile-time interface check: State must implement pabtpkg.IState
var _ pabtpkg.IState = (*State)(nil)

// State implements pabtpkg.State (which is State[Condition]) interface backed by a bt.Blackboard.
// It normalizes any key type to string for blackboard storage and provides
// access to a registry of actions.
//
// The osm:pabt Go layer is deliberately minimal - it provides only the PA-BT
// primitives (State, Action, Condition, Effect). Application-specific types
// like simulation state, sprites, shapes, etc. belong in the scripting layer
// (JavaScript) where they can be customized per-application.
//
// ActionGenerator Support:
// For TRUE parametric actions (like MoveTo(entityId)), set an ActionGenerator
// callback via SetActionGenerator(). When Actions() is called with a failed
// condition, the generator is invoked to produce actions dynamically based on
// the current world state and the specific failed condition.
type State struct {
	// Embed bt.Blackboard for storage
	*btmod.Blackboard

	// Registry of available actions (static registration)
	actions *ActionRegistry

	// ActionGenerator is an optional callback for dynamic action generation.
	// When set, it is called by Actions() to generate actions based on the
	// failed condition. This enables TRUE parametric actions like MoveTo(entityId)
	// where the entity parameter is determined at planning time.
	//
	// The generator receives the failed condition and should return a slice of
	// actions that could potentially satisfy it. These are combined with any
	// statically registered actions.
	//
	// Thread safety: The generator is called from the bt.Ticker goroutine.
	// If it accesses JavaScript state, it MUST use Bridge.RunOnLoopSync.
	actionGenerator ActionGeneratorFunc

	// mu protects actionGenerator from concurrent read/write
	mu sync.RWMutex
}

// ActionGeneratorFunc is a function that generates actions dynamically based on
// the failed condition. This is the core of parametric action support.
//
// Parameters:
//   - failed: The condition that failed (triggered planning). Contains the key
//     and value that the planner is trying to achieve.
//
// Returns:
//   - actions: Slice of actions that could potentially satisfy the failed condition.
//     Each action should have effects that match the failed condition's key.
//   - error: If action generation fails.
//
// Example usage for MoveTo(entityId):
//
//	generator := func(failed pabtpkg.Condition) ([]pabtpkg.IAction, error) {
//	    key := failed.Key()
//	    if key == "atEntity" {
//	        // Generate MoveTo actions for each reachable entity
//	        for _, entity := range world.entities {
//	            actions = append(actions, createMoveToAction(entity.id))
//	        }
//	    }
//	    return actions, nil
//	}
type ActionGeneratorFunc func(failed pabtpkg.Condition) ([]pabtpkg.IAction, error)

// NewState creates a new State backed by the provided blackboard.
func NewState(bb *btmod.Blackboard) *State {
	return &State{
		Blackboard: bb,
		actions:    NewActionRegistry(),
	}
}

// SetActionGenerator sets the dynamic action generator callback.
// When set, Actions() will call this generator in addition to returning
// statically registered actions.
//
// This enables TRUE parametric actions like MoveTo(entityId) where the
// entity parameter is determined at planning time based on the failed condition.
//
// Thread safety: The generator is protected by a mutex and can be set/cleared
// at any time. The generator itself is called from the bt.Ticker goroutine.
func (s *State) SetActionGenerator(gen ActionGeneratorFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actionGenerator = gen
}

// GetActionGenerator returns the current action generator, or nil if none is set.
func (s *State) GetActionGenerator() ActionGeneratorFunc {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.actionGenerator
}

// Variable implements pabtpkg.State.Variable(key any).
//
// # Key Normalization
//
// This method normalizes any key type to a string for blackboard lookup.
// The following key types are supported:
//
//   - string: Used as-is (canonical form)
//   - int, int8, int16, int32, int64: Converted to decimal string via fmt.Sprintf("%d")
//   - uint, uint8, uint16, uint32, uint64: Converted to decimal string via fmt.Sprintf("%d")
//   - float32, float64: Converted via fmt.Sprintf("%g") (compact notation, e.g., 1.5 → "1.5")
//   - fmt.Stringer: Uses the String() method (for Symbol types or custom types)
//
// Note: Using %g for floats ensures compact representation (1.5 → "1.5", not "1.500000").
// For parametric actions, prefer string keys for consistency.
//
// Returns (nil, nil) if key doesn't exist in the blackboard (this is intentional
// PA-BT semantics - missing variables are treated as not having that property).
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
		keyStr = fmt.Sprintf("%g", k)
	default:
		// Try fmt.Stringer interface (for pabtpkg.Symbol or custom types)
		if stringer, ok := key.(fmt.Stringer); ok {
			keyStr = stringer.String()
		} else {
			return nil, fmt.Errorf("unsupported key type: %T", key)
		}
	}

	// Safety check: Blackboard is required (should never be nil if using NewState)
	if s.Blackboard == nil {
		return nil, fmt.Errorf("State.Blackboard is nil - use NewState() to construct")
	}

	// Get value from blackboard (returns nil if not found, which is correct pabt semantics)
	value := s.Blackboard.Get(keyStr)

	return value, nil
}

// Actions implements pabtpkg.State.Actions(failed Condition).
// Returns all actions whose effects could potentially satisfy the failed condition.
//
// PA-BT Algorithm: An action is relevant if it has an effect that:
// 1. Has the same key as the failed condition
// 2. Would satisfy the failed condition (failed.Match(effect.Value()) returns true)
//
// This is the core of PA-BT planning: find actions that can make progress
// toward satisfying unsatisfied conditions.
//
// Action Sources (in order):
// 1. ActionGenerator (if set) - generates parametric actions dynamically
// 2. Static registry - returns pre-registered actions
//
// Both sources are filtered to only include actions with relevant effects.
//
// Special case: If failed is nil, returns all registered actions (for backward
// compatibility and testing purposes).
func (s *State) Actions(failed pabtpkg.Condition) ([]pabtpkg.IAction, error) {
	// Get statically registered actions
	registeredActions := s.actions.All()

	// Special case: nil failed condition returns all registered actions
	if failed == nil {
		return registeredActions, nil
	}

	failedKey := failed.Key()

	var relevantActions []pabtpkg.IAction

	// Track whether ActionGenerator returned any actions for this condition.
	// If so, the generator is authoritative and we don't scan the static registry.
	generatorHandled := false

	// 1. Call ActionGenerator if set (parametric actions)
	s.mu.RLock()
	generator := s.actionGenerator
	s.mu.RUnlock()

	if generator != nil {
		generatedActions, err := generator(failed)
		if err != nil {
			// Always log generator errors at Warning level - these could hide bugs
			slog.Warn("[PA-BT] ActionGenerator error, falling back to static actions", "error", err, "failedKey", failedKey)
			// Don't fail completely - fall back to static actions
		} else {
			// Filter generated actions for relevance
			for _, action := range generatedActions {
				if s.actionHasRelevantEffect(action, failedKey, failed) {
					relevantActions = append(relevantActions, action)
				}
			}
			// If generator returned any actions (even if not all were relevant),
			// it's authoritative for this condition
			if len(generatedActions) > 0 {
				generatorHandled = true
			}
		}
	}

	// 2. Filter static registry actions for relevance
	// ONLY if ActionGenerator didn't handle this condition
	if !generatorHandled {
		for _, action := range registeredActions {
			if s.actionHasRelevantEffect(action, failedKey, failed) {
				relevantActions = append(relevantActions, action)
			}
		}
	}

	return relevantActions, nil
}

// actionHasRelevantEffect checks if an action has an effect that would
// satisfy the given failed condition.
//
// An effect is relevant if:
// 1. effect.Key() equals the failed condition's key
// 2. failed.Match(effect.Value()) returns true
func (s *State) actionHasRelevantEffect(action pabtpkg.IAction, failedKey any, failed pabtpkg.Condition) bool {
	effects := action.Effects()
	for _, effect := range effects {
		if effect == nil {
			continue
		}
		effectKey := effect.Key()
		effectValue := effect.Value()

		// Check if this effect's key matches the failed condition's key
		keyMatch := effectKey == failedKey
		var valueMatch bool
		if keyMatch {
			valueMatch = failed.Match(effectValue)
		}

		if keyMatch && valueMatch {
			return true
		}
	}
	return false
}

// RegisterAction adds an action to the state's action registry.
func (s *State) RegisterAction(name string, action pabtpkg.IAction) {
	s.actions.Register(name, action)
}
