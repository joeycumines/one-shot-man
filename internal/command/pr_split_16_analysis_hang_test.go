package command

import (
	"testing"
)

// ---------------------------------------------------------------------------
//  T001: Analysis pipeline hang audit tests
//
//  Verifies that runAnalysisAsync correctly handles thrown errors from
//  applyStrategyAsync and createSplitPlanAsync (previously uncaught),
//  and that confirmCancel clears analysisRunning to stop orphaned ticks.
// ---------------------------------------------------------------------------

// TestChunk16_AnalysisPipeline_GroupingThrow verifies that when
// applyStrategyAsync throws, the error is caught INLINE by the T001
// try-catch (not deferred to the outer .then(_, reject) handler),
// setting isProcessing=false, errorDetails, and transitioning to ERROR.
//
// Uses await to drain the Goja microtask queue so the async pipeline
// actually reaches Step 2 where the throw occurs.
func TestChunk16_AnalysisPipeline_GroupingThrow(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		setupPlanCache();
		var s = initState('CONFIG');
		s.outputLines = [];
		s.outputAutoScroll = true;

		// Mock analyzeDiffAsync to resolve immediately (Step 1 succeeds).
		var origAnalyze = prSplit.analyzeDiffAsync;
		prSplit.analyzeDiffAsync = function() {
			return Promise.resolve({
				files: ['a.go', 'b.go'],
				currentBranch: 'feature',
				fileStatuses: { 'a.go': 'A', 'b.go': 'A' }
			});
		};

		// Mock applyStrategyAsync to THROW (Step 2 fails).
		var origStrategy = prSplit.applyStrategyAsync;
		prSplit.applyStrategyAsync = function() {
			throw new Error('grouping exploded');
		};

		// Heuristic mode — no baseline verify, no Claude.
		prSplit.runtime.mode = 'heuristic';
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.dir = '.';
		prSplit.runtime.strategy = 'directory';
		prSplit.runtime.verifyCommand = '';

		s.focusIndex = -1;
		s.configFieldEditing = null;

		// Trigger Enter to start analysis.
		sendKey(s, 'enter');

		// Drain microtask queue so runAnalysisAsync completes:
		//  tick 1: analyzeDiffAsync promise resolves (Step 1 ← await)
		//  tick 2: applyStrategyAsync sync throw → rejected-promise resolves in catch
		//  tick 3: .then(resolve) handler runs (analysisRunning=false)
		await Promise.resolve();
		await Promise.resolve();
		await Promise.resolve();
		await Promise.resolve(); // safety margin

		// Restore mocks.
		prSplit.analyzeDiffAsync = origAnalyze;
		prSplit.applyStrategyAsync = origStrategy;

		// T001: The inline try-catch should have set isProcessing=false.
		if (s.isProcessing) {
			return 'FAIL: isProcessing should be false after Step 2 throw';
		}

		// T001: errorDetails should contain the grouping error message.
		if (!s.errorDetails || s.errorDetails.indexOf('Grouping failed') < 0) {
			return 'FAIL: errorDetails should contain "Grouping failed", got: ' + s.errorDetails;
		}

		// T001: analysisRunning should be false (set by .then callback).
		if (s.analysisRunning) {
			return 'FAIL: analysisRunning should be false after pipeline completion';
		}

		// T001: wizardState should be ERROR (transition happened inline).
		if (s.wizardState !== 'ERROR') {
			return 'FAIL: expected ERROR wizardState, got: ' + s.wizardState;
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("grouping throw: %v", raw)
	}
}

// TestChunk16_AnalysisPipeline_PlanCreationThrow verifies that when
// createSplitPlanAsync throws (Step 3), the error is caught INLINE
// with the descriptive prefix "Plan creation failed:".
func TestChunk16_AnalysisPipeline_PlanCreationThrow(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		setupPlanCache();
		var s = initState('CONFIG');
		s.outputLines = [];
		s.outputAutoScroll = true;

		// Mock analyzeDiffAsync to resolve immediately (Step 1 succeeds).
		var origAnalyze = prSplit.analyzeDiffAsync;
		prSplit.analyzeDiffAsync = function() {
			return Promise.resolve({
				files: ['a.go', 'b.go'],
				currentBranch: 'feature',
				fileStatuses: { 'a.go': 'A', 'b.go': 'A' }
			});
		};

		// Mock applyStrategyAsync to succeed (Step 2 passes).
		var origStrategy = prSplit.applyStrategyAsync;
		prSplit.applyStrategyAsync = function() {
			return Promise.resolve([{name: 'group1', files: ['a.go', 'b.go']}]);
		};

		// Mock createSplitPlanAsync to THROW (Step 3 fails).
		var origPlan = prSplit.createSplitPlanAsync;
		prSplit.createSplitPlanAsync = function() {
			throw new Error('plan creation kaboom');
		};

		prSplit.runtime.mode = 'heuristic';
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.dir = '.';
		prSplit.runtime.strategy = 'directory';
		prSplit.runtime.verifyCommand = '';

		s.focusIndex = -1;
		s.configFieldEditing = null;

		sendKey(s, 'enter');

		// Drain: Step 1 await, Step 2 await, Step 3 rejected-await, .then handler.
		await Promise.resolve();
		await Promise.resolve();
		await Promise.resolve();
		await Promise.resolve();
		await Promise.resolve(); // safety margin

		prSplit.analyzeDiffAsync = origAnalyze;
		prSplit.applyStrategyAsync = origStrategy;
		prSplit.createSplitPlanAsync = origPlan;

		if (s.isProcessing) {
			return 'FAIL: isProcessing should be false after Step 3 throw';
		}
		if (!s.errorDetails || s.errorDetails.indexOf('Plan creation failed') < 0) {
			return 'FAIL: errorDetails should contain "Plan creation failed", got: ' + s.errorDetails;
		}
		if (s.analysisRunning) {
			return 'FAIL: analysisRunning should be false after pipeline completion';
		}
		if (s.wizardState !== 'ERROR') {
			return 'FAIL: expected ERROR wizardState, got: ' + s.wizardState;
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("plan creation throw: %v", raw)
	}
}

// TestChunk16_ConfirmCancel_ClearsAnalysisRunning verifies that
// confirmCancel sets analysisRunning=false to prevent orphaned
// poll ticks after cancellation.
func TestChunk16_ConfirmCancel_ClearsAnalysisRunning(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('CONFIG');
		s.isProcessing = true;
		s.analysisRunning = true;
		s.autoSplitRunning = true;
		s.showConfirmCancel = true;
		s.confirmCancelFocus = 0;
		s.width = 80;
		s.height = 24;
		s.outputLines = [];
		s.outputAutoScroll = true;

		// Simulate user confirming cancel with 'y' while cancel overlay is showing.
		var r = sendKey(s, 'y');
		var state = r[0];

		// T001: analysisRunning should be cleared.
		if (state.analysisRunning) {
			return 'FAIL: analysisRunning should be false after confirmCancel';
		}

		// T001: autoSplitRunning should be cleared.
		if (state.autoSplitRunning) {
			return 'FAIL: autoSplitRunning should be false after confirmCancel';
		}

		// isProcessing should be cleared.
		if (state.isProcessing) {
			return 'FAIL: isProcessing should be false after confirmCancel';
		}

		// Wizard state should be CANCELLED.
		if (state.wizardState !== 'CANCELLED') {
			return 'FAIL: expected CANCELLED, got ' + state.wizardState;
		}

		// confirmCancel returns tea.quit() command.
		if (r[1] === null) {
			return 'FAIL: expected quit command, got null';
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("confirmCancel clears analysisRunning: %v", raw)
	}
}

// TestChunk16_HandleAnalysisPoll_StopsImmediatelyAfterCancel verifies
// that after confirmCancel clears both isProcessing and analysisRunning,
// the handleAnalysisPoll returns [s, null] (stops) immediately.
func TestChunk16_HandleAnalysisPoll_StopsImmediatelyAfterCancel(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('CONFIG');

		// Simulate post-cancel state.
		s.isProcessing = false;
		s.analysisRunning = false;
		s.analysisError = null;

		var r = update({type: 'Tick', id: 'analysis-poll'}, s);
		var cmd = r[1];

		// Should return null (stop polling), not a tick.
		if (cmd !== null) {
			return 'FAIL: expected null cmd (stop polling), got non-null';
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("poll stops after cancel: %v", raw)
	}
}

// TestChunk16_HandleAnalysisPoll_ErrorPathSetsProcessingFalse verifies
// that when analysisError is set (by the outer reject handler), the poll
// clears isProcessing and transitions to ERROR.
func TestChunk16_HandleAnalysisPoll_ErrorPathSetsProcessingFalse(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('CONFIG');
		s.wizard.transition('PLAN_GENERATION');
		s.isProcessing = true;
		s.analysisRunning = false; // promise resolved (rejected)
		s.analysisError = 'grouping exploded';

		var r = update({type: 'Tick', id: 'analysis-poll'}, s);
		var state = r[0];
		var cmd = r[1];

		if (state.isProcessing) {
			return 'FAIL: isProcessing should be false after error handling';
		}
		if (state.errorDetails !== 'grouping exploded') {
			return 'FAIL: expected error details, got: ' + state.errorDetails;
		}
		if (cmd !== null) {
			return 'FAIL: should stop polling after error';
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("poll error path: %v", raw)
	}
}
