package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  T114: handleRestartAgentPoll mode-aware resume
//
//  Verifies that after a successful Agent restart:
//  1. With cached plan: transitions ERROR_RESOLUTION → BRANCH_BUILDING, calls startExecution
//  2. Without plan + auto mode: transitions ERROR_RESOLUTION → PLAN_GENERATION, calls startAutoAnalysis
//  3. Without plan + non-auto: transitions ERROR_RESOLUTION → PLAN_GENERATION, calls startAnalysis
//  4. Crash-recovery notification badge is set
//  5. s.errorDetails is cleared on successful restart
// ---------------------------------------------------------------------------

// TestChunk16_RestartAgentPoll_WithPlan verifies that when a cached plan
// exists, handleRestartAgentPoll transitions to BRANCH_BUILDING and starts
// execution (mode-agnostic since plan is already generated).
func TestChunk16_RestartAgentPoll_WithPlan(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		` + prsplittest.GitMockSetupJS() + `
		setupPlanCache(); // populate st.planCache

		var s = initState('ERROR_RESOLUTION');
		s.agentRestarting = false; // restart completed
		s.restartResult = { sessionId: 'abc-123' };
		s.agentCrashDetected = true;
		s.errorDetails = 'Restarting Agent...'; // stale from restart phase

		var r = update({type: 'Tick', id: 'restart-agent-poll'}, s);
		var state = r[0];

		// T114: Should transition to BRANCH_BUILDING for execution.
		if (state.wizardState !== 'BRANCH_BUILDING') {
			return 'FAIL: expected BRANCH_BUILDING, got ' + state.wizardState;
		}

		// Crash flags should be cleared.
		if (state.agentCrashDetected) {
			return 'FAIL: agentCrashDetected should be false';
		}

		// errorDetails should be cleared.
		if (state.errorDetails) {
			return 'FAIL: errorDetails should be null, got: ' + state.errorDetails;
		}

		// Notification badge should be set.
		if (!state.agentAutoAttachNotif || state.agentAutoAttachNotif.indexOf('re-executing') === -1) {
			return 'FAIL: expected re-executing notification, got: ' + state.agentAutoAttachNotif;
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

// TestChunk16_RestartAgentPoll_NoPlan_AutoMode verifies that without a plan
// in auto mode, handleRestartAgentPoll transitions to PLAN_GENERATION and
// triggers auto analysis.
func TestChunk16_RestartAgentPoll_NoPlan_AutoMode(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		` + prsplittest.GitMockSetupJS() + `

		var s = initState('ERROR_RESOLUTION');
		s.agentRestarting = false;
		s.restartResult = { sessionId: 'abc-456' };
		s.agentCrashDetected = true;
		s.errorDetails = 'Restarting Agent...';

		// Set auto mode.
		globalThis.prSplit.runtime.mode = 'auto';

		// Ensure no plan cache.
		globalThis.prSplit._state.planCache = null;

		var r = await update({type: 'Tick', id: 'restart-agent-poll'}, s);
		var state = r[0];

		// T114: Should transition to PLAN_GENERATION for re-analysis.
		if (state.wizardState !== 'PLAN_GENERATION') {
			return 'FAIL: expected PLAN_GENERATION, got ' + state.wizardState;
		}

		// Crash flags should be cleared.
		if (state.agentCrashDetected) {
			return 'FAIL: agentCrashDetected should be false';
		}

		// errorDetails should be cleared.
		if (state.errorDetails) {
			return 'FAIL: errorDetails should be null, got: ' + state.errorDetails;
		}

		// Notification badge should be set.
		if (!state.agentAutoAttachNotif || state.agentAutoAttachNotif.indexOf('re-analyzing') === -1) {
			return 'FAIL: expected re-analyzing notification, got: ' + state.agentAutoAttachNotif;
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

// TestChunk16_RestartAgentPoll_NoPlan_HeuristicMode verifies that without
// a plan in non-auto (heuristic) mode, handleRestartAgentPoll transitions to
// PLAN_GENERATION and triggers heuristic analysis.
func TestChunk16_RestartAgentPoll_NoPlan_HeuristicMode(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		` + prsplittest.GitMockSetupJS() + `

		var s = initState('ERROR_RESOLUTION');
		s.agentRestarting = false;
		s.restartResult = { sessionId: 'abc-789' };
		s.agentCrashDetected = true;
		s.errorDetails = 'Restarting Agent...';

		// Set heuristic mode.
		globalThis.prSplit.runtime.mode = 'wizard';

		// Ensure no plan cache.
		globalThis.prSplit._state.planCache = null;

		var r = await update({type: 'Tick', id: 'restart-agent-poll'}, s);
		var state = r[0];

		// T114: Should transition to PLAN_GENERATION for re-analysis.
		if (state.wizardState !== 'PLAN_GENERATION') {
			return 'FAIL: expected PLAN_GENERATION, got ' + state.wizardState;
		}

		// Should still get a notification.
		if (!state.agentAutoAttachNotif) {
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

// TestChunk16_RestartAgentPoll_StillRestarting verifies that when Agent is
// still restarting, the handler re-schedules the poll tick.
func TestChunk16_RestartAgentPoll_StillRestarting(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var s = initState('ERROR_RESOLUTION');
		s.agentRestarting = true; // still in progress
		s.restartResult = null;

		var r = update({type: 'Tick', id: 'restart-agent-poll'}, s);
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

// TestChunk16_RestartAgentPoll_Error verifies that a failed restart preserves
// crash flags and sets error details.
func TestChunk16_RestartAgentPoll_Error(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var s = initState('ERROR_RESOLUTION');
		s.agentRestarting = false;
		s.restartResult = { error: 'Connection refused' };
		s.agentCrashDetected = true;

		var r = update({type: 'Tick', id: 'restart-agent-poll'}, s);
		var state = r[0];

		// Should remain in ERROR_RESOLUTION with crash flags.
		if (!state.agentCrashDetected) {
			return 'FAIL: agentCrashDetected should remain true on error';
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
