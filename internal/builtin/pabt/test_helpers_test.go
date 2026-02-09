package pabt

import pabtpkg "github.com/joeycumines/go-pabt"

// canExecuteAction is a test helper that checks if all of an action's conditions
// pass against the current state.
//
// An action has multiple IConditions groups; at least ONE group must pass (OR logic).
// Within each group, ALL conditions must pass (AND logic).
//
// Note: This function is for EXECUTION phase testing, not PLANNING phase.
// During planning, State.Actions() uses actionHasRelevantEffect to find actions
// whose effects would satisfy failed conditions.
//
// This helper was moved from state.go as it is only used in tests.
func canExecuteAction(s *State, action pabtpkg.IAction) bool {
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
