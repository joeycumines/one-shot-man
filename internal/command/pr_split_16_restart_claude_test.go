package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  T114: handleRestartClaudePoll mode-aware resume
//
//  Verifies that after a successful Claude restart:
//  1. With cached plan: transitions ERROR_RESOLUTION → BRANCH_BUILDING, calls startExecution
//  2. Without plan + auto mode: transitions ERROR_RESOLUTION → PLAN_GENERATION, calls startAutoAnalysis
//  3. Without plan + non-auto: transitions ERROR_RESOLUTION → PLAN_GENERATION, calls startAnalysis
//  4. Crash-recovery notification badge is set
//  5. s.errorDetails is cleared on successful restart
// ---------------------------------------------------------------------------

// TestChunk16_RestartClaudePoll_WithPlan verifies that when a cached plan
// exists, handleRestartClaudePoll transitions to BRANCH_BUILDING and starts
// execution (mode-agnostic since plan is already generated).
func TestChunk16_RestartClaudePoll_WithPlan(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + gitMockSetupJS() + `
		setupPlanCache(); // populate st.planCache

		var s = initState('ERROR_RESOLUTION');
		s.claudeRestarting = false; // restart completed
		s.restartResult = { sessionId: 'abc-123' };
		s.claudeCrashDetected = true;
		s.errorDetails = 'Restarting Claude...'; // stale from restart phase

		var r = update({type: 'Tick', id: 'restart-claude-poll'}, s);
		var state = r[0];

		// T114: Should transition to BRANCH_BUILDING for execution.
		if (state.wizardState !== 'BRANCH_BUILDING') {
			return 'FAIL: expected BRANCH_BUILDING, got ' + state.wizardState;
		}

		// Crash flags should be cleared.
		if (state.claudeCrashDetected) {
			return 'FAIL: claudeCrashDetected should be false';
		}

		// errorDetails should be cleared.
		if (state.errorDetails) {
			return 'FAIL: errorDetails should be null, got: ' + state.errorDetails;
		}

		// Notification badge should be set.
		if (!state.claudeAutoAttachNotif || state.claudeAutoAttachNotif.indexOf('re-executing') === -1) {
			return 'FAIL: expected re-executing notification, got: ' + state.claudeAutoAttachNotif;
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("restart with plan: %v", raw)
	}
}

// TestChunk16_RestartClaudePoll_NoPlan_AutoMode verifies that without a plan
// in auto mode, handleRestartClaudePoll transitions to PLAN_GENERATION and
// triggers auto analysis.
func TestChunk16_RestartClaudePoll_NoPlan_AutoMode(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + gitMockSetupJS() + `

		var s = initState('ERROR_RESOLUTION');
		s.claudeRestarting = false;
		s.restartResult = { sessionId: 'abc-456' };
		s.claudeCrashDetected = true;
		s.errorDetails = 'Restarting Claude...';

		// Set auto mode.
		globalThis.prSplit.runtime.mode = 'auto';

		// Ensure no plan cache.
		globalThis.prSplit._state.planCache = null;

		var r = update({type: 'Tick', id: 'restart-claude-poll'}, s);
		var state = r[0];

		// T114: Should transition to PLAN_GENERATION for re-analysis.
		if (state.wizardState !== 'PLAN_GENERATION') {
			return 'FAIL: expected PLAN_GENERATION, got ' + state.wizardState;
		}

		// Crash flags should be cleared.
		if (state.claudeCrashDetected) {
			return 'FAIL: claudeCrashDetected should be false';
		}

		// errorDetails should be cleared.
		if (state.errorDetails) {
			return 'FAIL: errorDetails should be null, got: ' + state.errorDetails;
		}

		// Notification badge should be set.
		if (!state.claudeAutoAttachNotif || state.claudeAutoAttachNotif.indexOf('re-analyzing') === -1) {
			return 'FAIL: expected re-analyzing notification, got: ' + state.claudeAutoAttachNotif;
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("restart no plan auto: %v", raw)
	}
}

// TestChunk16_RestartClaudePoll_NoPlan_HeuristicMode verifies that without
// a plan in non-auto (heuristic) mode, handleRestartClaudePoll transitions to
// PLAN_GENERATION and triggers heuristic analysis.
func TestChunk16_RestartClaudePoll_NoPlan_HeuristicMode(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + gitMockSetupJS() + `

		var s = initState('ERROR_RESOLUTION');
		s.claudeRestarting = false;
		s.restartResult = { sessionId: 'abc-789' };
		s.claudeCrashDetected = true;
		s.errorDetails = 'Restarting Claude...';

		// Set heuristic mode.
		globalThis.prSplit.runtime.mode = 'wizard';

		// Ensure no plan cache.
		globalThis.prSplit._state.planCache = null;

		var r = update({type: 'Tick', id: 'restart-claude-poll'}, s);
		var state = r[0];

		// T114: Should transition to PLAN_GENERATION for re-analysis.
		if (state.wizardState !== 'PLAN_GENERATION') {
			return 'FAIL: expected PLAN_GENERATION, got ' + state.wizardState;
		}

		// Should still get a notification.
		if (!state.claudeAutoAttachNotif) {
			return 'FAIL: notification should be set';
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("restart no plan heuristic: %v", raw)
	}
}

// TestChunk16_RestartClaudePoll_StillRestarting verifies that when Claude is
// still restarting, the handler re-schedules the poll tick.
func TestChunk16_RestartClaudePoll_StillRestarting(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('ERROR_RESOLUTION');
		s.claudeRestarting = true; // still in progress
		s.restartResult = null;

		var r = update({type: 'Tick', id: 'restart-claude-poll'}, s);
		var state = r[0];
		var cmd = r[1];

		// Should keep polling.
		if (!cmd) {
			return 'FAIL: expected tick command for re-poll';
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("still restarting: %v", raw)
	}
}

// TestChunk16_RestartClaudePoll_Error verifies that a failed restart preserves
// crash flags and sets error details.
func TestChunk16_RestartClaudePoll_Error(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('ERROR_RESOLUTION');
		s.claudeRestarting = false;
		s.restartResult = { error: 'Connection refused' };
		s.claudeCrashDetected = true;

		var r = update({type: 'Tick', id: 'restart-claude-poll'}, s);
		var state = r[0];

		// Should remain in ERROR_RESOLUTION with crash flags.
		if (!state.claudeCrashDetected) {
			return 'FAIL: claudeCrashDetected should remain true on error';
		}

		// errorDetails should show the error.
		if (!state.errorDetails || state.errorDetails.indexOf('Connection refused') === -1) {
			return 'FAIL: errorDetails should contain error, got: ' + state.errorDetails;
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("restart error: %v", raw)
	}
}
