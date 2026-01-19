package pabt

import (
	"fmt"
	"os"
	"sync"

	pabtpkg "github.com/joeycumines/go-pabt"
	btmod "github.com/joeycumines/one-shot-man/internal/builtin/bt"
)

// debugPABT controls verbose PA-BT debugging output.
// Set OSM_DEBUG_PABT=1 to enable.
var debugPABT = os.Getenv("OSM_DEBUG_PABT") == "1"

// debugOnce logs once that debugging is enabled
var debugOnce sync.Once

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
// It normalizes the key to a string for blackboard lookup.
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

	if debugPABT {
		debugOnce.Do(func() {
			fmt.Fprintln(os.Stderr, "[PA-BT DEBUG] Debugging enabled via OSM_DEBUG_PABT=1")
		})
		fmt.Fprintf(os.Stderr, "[PA-BT DEBUG] State.Variable called: key=%v keyStr=%s value=%v (%T)\n",
			key, keyStr, value, value)
	}

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

	if debugPABT {
		debugOnce.Do(func() {
			fmt.Fprintln(os.Stderr, "[PA-BT DEBUG] Debugging enabled via OSM_DEBUG_PABT=1")
		})
		fmt.Fprintf(os.Stderr, "[PA-BT DEBUG] State.Actions called: failedKey=%v (%T), registeredActionCount=%d\n",
			failedKey, failedKey, len(registeredActions))
	}

	var relevantActions []pabtpkg.IAction

	// 1. Call ActionGenerator if set (parametric actions)
	s.mu.RLock()
	generator := s.actionGenerator
	s.mu.RUnlock()

	if generator != nil {
		generatedActions, err := generator(failed)
		if err != nil {
			if debugPABT {
				fmt.Fprintf(os.Stderr, "[PA-BT DEBUG] ActionGenerator error: %v\n", err)
			}
			// Don't fail completely - fall back to static actions
		} else {
			if debugPABT {
				fmt.Fprintf(os.Stderr, "[PA-BT DEBUG] ActionGenerator returned %d actions\n", len(generatedActions))
			}
			// Filter generated actions for relevance
			for _, action := range generatedActions {
				if s.actionHasRelevantEffect(action, failedKey, failed) {
					relevantActions = append(relevantActions, action)
				}
			}
		}
	}

	// 2. Filter static registry actions for relevance
	for _, action := range registeredActions {
		if s.actionHasRelevantEffect(action, failedKey, failed) {
			relevantActions = append(relevantActions, action)
		}
	}

	if debugPABT {
		actionNames := make([]string, 0, len(relevantActions))
		for _, a := range relevantActions {
			if named, ok := a.(*Action); ok {
				actionNames = append(actionNames, named.Name)
			}
		}
		fmt.Fprintf(os.Stderr, "[PA-BT DEBUG] State.Actions result: failedKey=%v, relevantActionCount=%d, relevantActions=%v\n",
			failedKey, len(relevantActions), actionNames)
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

		if debugPABT {
			var actionName string
			if named, ok := action.(*Action); ok {
				actionName = named.Name
			}
			fmt.Fprintf(os.Stderr, "[PA-BT DEBUG] Effect comparison: action=%s effectKey=%v (%T) failedKey=%v (%T) keyMatch=%v effectValue=%v valueMatch=%v\n",
				actionName, effectKey, effectKey, failedKey, failedKey, keyMatch, effectValue, valueMatch)
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
