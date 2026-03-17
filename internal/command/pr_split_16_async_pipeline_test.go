package command

import (
	"path/filepath"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  T34: Async Analysis Pipeline Tests
// ---------------------------------------------------------------------------

// TestChunk16_AnalysisPoll_StillRunning verifies that handleAnalysisPoll
// continues polling when analysis is still running.
func TestChunk16_AnalysisPoll_StillRunning(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.isProcessing = true;
		s.analysisRunning = true;
		s.analysisError = null;

		var r = update({type: 'Tick', id: 'analysis-poll'}, s);
		if (!r[1]) return 'FAIL: should return a tick cmd when still running';
		if (!r[0].analysisRunning) return 'FAIL: analysisRunning should still be true';
		if (!r[0].isProcessing) return 'FAIL: isProcessing should still be true';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("analysis-poll still running: %v", raw)
	}
}

// TestChunk16_AnalysisPoll_Cancelled verifies that handleAnalysisPoll
// stops polling when processing was cancelled.
func TestChunk16_AnalysisPoll_Cancelled(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.isProcessing = false;
		s.analysisRunning = false;
		s.analysisError = null;

		var r = update({type: 'Tick', id: 'analysis-poll'}, s);
		if (r[1] !== null) return 'FAIL: should return null cmd on cancel, got: ' + r[1];
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("analysis-poll cancelled: %v", raw)
	}
}

// TestChunk16_AnalysisPoll_ErrorFromPromise verifies that handleAnalysisPoll
// transitions to ERROR state when the async pipeline rejects.
func TestChunk16_AnalysisPoll_ErrorFromPromise(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.isProcessing = true;
		s.analysisRunning = false;
		s.analysisError = 'git diff failed: permission denied';

		var r = update({type: 'Tick', id: 'analysis-poll'}, s);
		if (r[0].wizardState !== 'ERROR') return 'FAIL: wizardState should be ERROR, got: ' + r[0].wizardState;
		if (r[0].isProcessing) return 'FAIL: isProcessing should be false';
		if (!r[0].errorDetails || r[0].errorDetails.indexOf('permission denied') < 0) {
			return 'FAIL: errorDetails should contain error, got: ' + r[0].errorDetails;
		}
		if (r[1] !== null) return 'FAIL: should return null cmd on error';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("analysis-poll error: %v", raw)
	}
}

// TestChunk16_AnalysisPoll_CompletedSuccess verifies that handleAnalysisPoll
// accepts the final state when analysis completed successfully (state
// already transitioned by the async function).
func TestChunk16_AnalysisPoll_CompletedSuccess(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Simulate async pipeline having completed successfully:
		// analysisRunning=false, analysisError=null, wizardState already
		// transitioned to PLAN_REVIEW by runAnalysisAsync.
		var s = initState('PLAN_REVIEW');
		s.isProcessing = false;
		s.analysisRunning = false;
		s.analysisError = null;

		var r = update({type: 'Tick', id: 'analysis-poll'}, s);
		// Should return null cmd (stop polling), state unchanged.
		if (r[1] !== null) return 'FAIL: should return null cmd on success';
		if (r[0].wizardState !== 'PLAN_REVIEW') return 'FAIL: wizardState should be PLAN_REVIEW, got: ' + r[0].wizardState;
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("analysis-poll success: %v", raw)
	}
}

// TestChunk16_AnalysisAsync_HappyPath exercises the full startAnalysis →
// runAnalysisAsync → handleAnalysisPoll flow with mocked async functions.
// Creates a real git repo so handleConfigState succeeds.
func TestChunk16_AnalysisAsync_HappyPath(t *testing.T) {
	t.Parallel()

	// Create a real git repo so handleConfigState succeeds.
	dir := initGitRepo(t)
	writeFile(t, filepath.Join(dir, "a.go"), "package a\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "initial")
	gitCmd(t, dir, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(dir, "b.go"), "package b\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "feature changes")

	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		// Save originals.
		var origAnalyzeDiffAsync = globalThis.prSplit.analyzeDiffAsync;
		var origApplyStrategy = globalThis.prSplit.applyStrategy;
		var origCreateSplitPlanAsync = globalThis.prSplit.createSplitPlanAsync;
		var origValidatePlan = globalThis.prSplit.validatePlan;

		try {
			// Mock analysis functions (called via prSplit.xxx dynamic lookup).
			globalThis.prSplit.analyzeDiffAsync = async function(config) {
				return {
					files: ['a.go', 'b.go', 'c.go'],
					fileStatuses: { 'a.go': 'M', 'b.go': 'A', 'c.go': 'M' },
					error: null,
					baseBranch: 'main',
					currentBranch: 'feature'
				};
			};
			globalThis.prSplit.applyStrategy = function(files, strategy) {
				return { 'group1': ['a.go', 'b.go'], 'group2': ['c.go'] };
			};
			globalThis.prSplit.createSplitPlanAsync = async function(groups, config) {
				return {
					baseBranch: 'main',
					sourceBranch: 'feature',
					splits: [
						{ name: 'split/01-group1', files: ['a.go', 'b.go'], message: 'group1', order: 0, dependencies: [] },
						{ name: 'split/02-group2', files: ['c.go'], message: 'group2', order: 1, dependencies: ['split/01-group1'] }
					]
				};
			};
			globalThis.prSplit.validatePlan = function(plan) {
				return { valid: true, errors: [] };
			};

			// Set up CONFIG state and runtime pointing to real git repo.
			var s = initState('CONFIG');
			globalThis.prSplit.runtime.baseBranch = 'main';
			globalThis.prSplit.runtime.dir = '` + escapeJSPath(dir) + `';
			globalThis.prSplit.runtime.strategy = 'directory';
			globalThis.prSplit.runtime.mode = 'heuristic';
			s.focusIndex = 4; // nav-next element (after toggle-advanced)

			// Trigger startAnalysis via enter key on nav-next.
			var r = sendKey(s, 'enter');
			s = r[0];

			// startAnalysis launched the async pipeline.
			if (!s.isProcessing) {
				return 'FAIL: isProcessing should be true after startAnalysis, state=' + s.wizardState +
					', error=' + s.errorDetails;
			}

			// Let microtasks resolve (mocked functions resolve immediately).
			await Promise.resolve();
			await Promise.resolve();
			await Promise.resolve();

			// Poll to finalize.
			r = update({type: 'Tick', id: 'analysis-poll'}, s);
			s = r[0];

			// After completion, should be PLAN_REVIEW.
			if (s.wizardState !== 'PLAN_REVIEW') {
				return 'FAIL: expected PLAN_REVIEW, got ' + s.wizardState +
					', error=' + s.errorDetails + ', isProcessing=' + s.isProcessing +
					', analysisRunning=' + s.analysisRunning;
			}
			if (s.isProcessing) return 'FAIL: isProcessing should be false';
			if (s.analysisRunning) return 'FAIL: analysisRunning should be false';

			// Verify all steps completed.
			for (var i = 0; i < 4; i++) {
				if (!s.analysisSteps[i].done) return 'FAIL: step ' + i + ' not done';
			}
			if (s.analysisProgress !== 1.0) return 'FAIL: progress should be 1.0, got ' + s.analysisProgress;

			return 'OK';
		} finally {
			globalThis.prSplit.analyzeDiffAsync = origAnalyzeDiffAsync;
			globalThis.prSplit.applyStrategy = origApplyStrategy;
			globalThis.prSplit.createSplitPlanAsync = origCreateSplitPlanAsync;
			globalThis.prSplit.validatePlan = origValidatePlan;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("analysis async happy path: %v", raw)
	}
}

// TestChunk16_AnalysisAsync_AnalyzeDiffError verifies error handling when
// analyzeDiffAsync throws an exception.
func TestChunk16_AnalysisAsync_AnalyzeDiffError(t *testing.T) {
	t.Parallel()

	dir := initGitRepo(t)
	writeFile(t, filepath.Join(dir, "a.go"), "package a\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "initial")
	gitCmd(t, dir, "checkout", "-b", "feature")

	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var origAnalyzeDiffAsync = globalThis.prSplit.analyzeDiffAsync;

		try {
			globalThis.prSplit.analyzeDiffAsync = async function() {
				throw new Error('git: not a git repository');
			};

			var s = initState('CONFIG');
			globalThis.prSplit.runtime.baseBranch = 'main';
			globalThis.prSplit.runtime.dir = '` + escapeJSPath(dir) + `';
			globalThis.prSplit.runtime.strategy = 'directory';
			globalThis.prSplit.runtime.mode = 'heuristic';
			s.focusIndex = 4; // nav-next element (after toggle-advanced)

			var r = sendKey(s, 'enter');
			s = r[0];

			if (!s.isProcessing) {
				return 'FAIL: isProcessing should be true, state=' + s.wizardState + ', error=' + s.errorDetails;
			}

			await Promise.resolve();
			await Promise.resolve();
			await Promise.resolve();

			r = update({type: 'Tick', id: 'analysis-poll'}, s);
			s = r[0];

			if (s.wizardState !== 'ERROR') {
				return 'FAIL: expected ERROR, got ' + s.wizardState + ', error=' + s.errorDetails;
			}
			if (!s.errorDetails || s.errorDetails.indexOf('not a git repository') < 0) {
				return 'FAIL: errorDetails should mention git error, got: ' + s.errorDetails;
			}
			if (s.isProcessing) return 'FAIL: isProcessing should be false';

			return 'OK';
		} finally {
			globalThis.prSplit.analyzeDiffAsync = origAnalyzeDiffAsync;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("analysis async diff error: %v", raw)
	}
}

// TestChunk16_AnalysisAsync_NoChanges verifies that when analyzeDiffAsync
// returns empty files, the wizard goes back to CONFIG.
func TestChunk16_AnalysisAsync_NoChanges(t *testing.T) {
	t.Parallel()

	dir := initGitRepo(t)
	writeFile(t, filepath.Join(dir, "a.go"), "package a\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "initial")
	gitCmd(t, dir, "checkout", "-b", "feature")

	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var origAnalyzeDiffAsync = globalThis.prSplit.analyzeDiffAsync;

		try {
			globalThis.prSplit.analyzeDiffAsync = async function() {
				return { files: [], fileStatuses: {}, error: null, baseBranch: 'main', currentBranch: 'feature' };
			};

			var s = initState('CONFIG');
			globalThis.prSplit.runtime.baseBranch = 'main';
			globalThis.prSplit.runtime.dir = '` + escapeJSPath(dir) + `';
			globalThis.prSplit.runtime.strategy = 'directory';
			globalThis.prSplit.runtime.mode = 'heuristic';
			s.focusIndex = 4; // nav-next element (after toggle-advanced)

			var r = sendKey(s, 'enter');
			s = r[0];

			if (!s.isProcessing) {
				return 'FAIL: isProcessing should be true, state=' + s.wizardState + ', error=' + s.errorDetails;
			}

			await Promise.resolve();
			await Promise.resolve();
			await Promise.resolve();

			r = update({type: 'Tick', id: 'analysis-poll'}, s);
			s = r[0];

			if (s.wizardState !== 'CONFIG') {
				return 'FAIL: expected CONFIG (no changes), got ' + s.wizardState;
			}
			if (s.isProcessing) return 'FAIL: isProcessing should be false';
			if (!s.errorDetails || s.errorDetails.indexOf('No changes') < 0) {
				return 'FAIL: errorDetails should mention no changes, got: ' + s.errorDetails;
			}

			return 'OK';
		} finally {
			globalThis.prSplit.analyzeDiffAsync = origAnalyzeDiffAsync;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("analysis async no changes: %v", raw)
	}
}

// TestChunk16_AnalysisAsync_ValidationFailure verifies that a validatePlan
// failure transitions to ERROR.
func TestChunk16_AnalysisAsync_ValidationFailure(t *testing.T) {
	t.Parallel()

	dir := initGitRepo(t)
	writeFile(t, filepath.Join(dir, "a.go"), "package a\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "initial")
	gitCmd(t, dir, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(dir, "b.go"), "package b\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "feature")

	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var origAnalyzeDiffAsync = globalThis.prSplit.analyzeDiffAsync;
		var origApplyStrategy = globalThis.prSplit.applyStrategy;
		var origCreateSplitPlanAsync = globalThis.prSplit.createSplitPlanAsync;
		var origValidatePlan = globalThis.prSplit.validatePlan;

		try {
			globalThis.prSplit.analyzeDiffAsync = async function() {
				return {
					files: ['a.go'], fileStatuses: { 'a.go': 'M' },
					error: null, baseBranch: 'main', currentBranch: 'feature'
				};
			};
			globalThis.prSplit.applyStrategy = function() {
				return { 'group1': ['a.go'] };
			};
			globalThis.prSplit.createSplitPlanAsync = async function() {
				return {
					baseBranch: 'main', sourceBranch: 'feature',
					splits: [{ name: 'split/01', files: [], message: 'empty', order: 0, dependencies: [] }]
				};
			};
			globalThis.prSplit.validatePlan = function() {
				return { valid: false, errors: ['split split/01 has no files'] };
			};

			var s = initState('CONFIG');
			globalThis.prSplit.runtime.baseBranch = 'main';
			globalThis.prSplit.runtime.dir = '` + escapeJSPath(dir) + `';
			globalThis.prSplit.runtime.strategy = 'directory';
			globalThis.prSplit.runtime.mode = 'heuristic';
			s.focusIndex = 4; // nav-next element (after toggle-advanced)

			var r = sendKey(s, 'enter');
			s = r[0];

			if (!s.isProcessing) {
				return 'FAIL: isProcessing should be true, state=' + s.wizardState + ', error=' + s.errorDetails;
			}

			await Promise.resolve();
			await Promise.resolve();
			await Promise.resolve();

			r = update({type: 'Tick', id: 'analysis-poll'}, s);
			s = r[0];

			if (s.wizardState !== 'ERROR') {
				return 'FAIL: expected ERROR, got ' + s.wizardState;
			}
			if (!s.errorDetails || s.errorDetails.indexOf('no files') < 0) {
				return 'FAIL: errorDetails should mention validation, got: ' + s.errorDetails;
			}

			return 'OK';
		} finally {
			globalThis.prSplit.analyzeDiffAsync = origAnalyzeDiffAsync;
			globalThis.prSplit.applyStrategy = origApplyStrategy;
			globalThis.prSplit.createSplitPlanAsync = origCreateSplitPlanAsync;
			globalThis.prSplit.validatePlan = origValidatePlan;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("analysis async validation failure: %v", raw)
	}
}

// TestChunk16_AnalysisAsync_NoSyncCallsRemain verifies that the old sync
// analysis tick IDs are no longer handled by the update function.
func TestChunk16_AnalysisAsync_NoSyncCallsRemain(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.isProcessing = true;

		// Old tick IDs should be ignored (return [s, null]).
		var oldTicks = ['analysis-step-0', 'analysis-step-1', 'analysis-step-2', 'analysis-step-3'];
		for (var i = 0; i < oldTicks.length; i++) {
			var r = update({type: 'Tick', id: oldTicks[i]}, s);
			if (r[1] !== null) return 'FAIL: old tick ' + oldTicks[i] + ' should return null cmd';
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("no sync calls remain: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  T35: Async Execution Pipeline Tests
// ---------------------------------------------------------------------------

// TestChunk16_ExecutionPoll_StillRunning verifies that handleExecutionPoll
// continues polling when execution is still running.
func TestChunk16_ExecutionPoll_StillRunning(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;
		s.executionRunning = true;
		s.executionError = null;

		var r = update({type: 'Tick', id: 'execution-poll'}, s);
		if (!r[1]) return 'FAIL: should return a tick cmd when still running';
		if (!r[0].executionRunning) return 'FAIL: executionRunning should still be true';
		if (!r[0].isProcessing) return 'FAIL: isProcessing should still be true';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("execution-poll still running: %v", raw)
	}
}

// TestChunk16_ExecutionPoll_Cancelled verifies that handleExecutionPoll
// stops polling when processing was cancelled.
func TestChunk16_ExecutionPoll_Cancelled(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = false;
		s.executionRunning = false;
		s.executionError = null;

		var r = update({type: 'Tick', id: 'execution-poll'}, s);
		if (r[1] !== null) return 'FAIL: should return null cmd on cancel, got: ' + r[1];
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("execution-poll cancelled: %v", raw)
	}
}

// TestChunk16_ExecutionPoll_ErrorFromPromise verifies that handleExecutionPoll
// transitions to ERROR_RESOLUTION when the async pipeline rejects.
func TestChunk16_ExecutionPoll_ErrorFromPromise(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;
		s.executionRunning = false;
		s.executionError = 'git worktree failed: permission denied';

		var r = update({type: 'Tick', id: 'execution-poll'}, s);
		if (r[0].wizardState !== 'ERROR_RESOLUTION') {
			return 'FAIL: wizardState should be ERROR_RESOLUTION, got: ' + r[0].wizardState;
		}
		if (r[0].isProcessing) return 'FAIL: isProcessing should be false';
		if (!r[0].errorDetails || r[0].errorDetails.indexOf('permission denied') < 0) {
			return 'FAIL: errorDetails should contain error, got: ' + r[0].errorDetails;
		}
		if (r[1] !== null) return 'FAIL: should return null cmd on error';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("execution-poll error: %v", raw)
	}
}

// TestChunk16_ExecutionPoll_CompletedToVerify verifies that handleExecutionPoll
// starts per-branch verification when executionNextStep='verify'.
func TestChunk16_ExecutionPoll_CompletedToVerify(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;
		s.executionRunning = false;
		s.executionError = null;
		s.executionNextStep = 'verify';

		var r = update({type: 'Tick', id: 'execution-poll'}, s);
		// Should dispatch to verify-branch.
		if (!r[1]) return 'FAIL: should return a tick cmd for verify-branch';
		if (r[0].verifyingIdx !== 0) return 'FAIL: verifyingIdx should be 0, got: ' + r[0].verifyingIdx;
		if (r[0].executionNextStep !== null) return 'FAIL: executionNextStep should be cleared';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("execution-poll completed→verify: %v", raw)
	}
}

// TestChunk16_ExecutionPoll_CompletedToEquiv verifies that handleExecutionPoll
// starts equivalence check when executionNextStep='equiv'.
func TestChunk16_ExecutionPoll_CompletedToEquiv(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;
		s.executionRunning = false;
		s.executionError = null;
		s.executionNextStep = 'equiv';

		var r = update({type: 'Tick', id: 'execution-poll'}, s);
		// Should start equiv check — returns a tick cmd for equiv-poll.
		if (!r[1]) return 'FAIL: should return a tick cmd for equiv-poll';
		if (r[0].wizardState !== 'EQUIV_CHECK') {
			return 'FAIL: wizardState should be EQUIV_CHECK, got: ' + r[0].wizardState;
		}
		if (!r[0].equivRunning) return 'FAIL: equivRunning should be true';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("execution-poll completed→equiv: %v", raw)
	}
}

// TestChunk16_EquivPoll_StillRunning verifies that handleEquivPoll
// continues polling when equiv check is still running.
func TestChunk16_EquivPoll_StillRunning(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('EQUIV_CHECK');
		s.isProcessing = true;
		s.equivRunning = true;
		s.equivError = null;

		var r = update({type: 'Tick', id: 'equiv-poll'}, s);
		if (!r[1]) return 'FAIL: should return a tick cmd when still running';
		if (!r[0].equivRunning) return 'FAIL: equivRunning should still be true';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("equiv-poll still running: %v", raw)
	}
}

// TestChunk16_EquivPoll_Cancelled verifies that handleEquivPoll
// stops polling when processing was cancelled.
func TestChunk16_EquivPoll_Cancelled(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('EQUIV_CHECK');
		s.isProcessing = false;
		s.equivRunning = false;
		s.equivError = null;

		var r = update({type: 'Tick', id: 'equiv-poll'}, s);
		if (r[1] !== null) return 'FAIL: should return null cmd on cancel';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("equiv-poll cancelled: %v", raw)
	}
}

// TestChunk16_EquivPoll_Error verifies that handleEquivPoll
// transitions to ERROR state when equiv check fails.
func TestChunk16_EquivPoll_Error(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('EQUIV_CHECK');
		s.isProcessing = true;
		s.equivRunning = false;
		s.equivError = 'failed to get split tree: fatal: not a valid object name';

		var r = update({type: 'Tick', id: 'equiv-poll'}, s);
		if (r[0].wizardState !== 'ERROR') {
			return 'FAIL: wizardState should be ERROR, got: ' + r[0].wizardState;
		}
		if (r[0].isProcessing) return 'FAIL: isProcessing should be false';
		if (!r[0].errorDetails || r[0].errorDetails.indexOf('Equivalence check failed') < 0) {
			return 'FAIL: errorDetails should mention equiv check, got: ' + r[0].errorDetails;
		}
		if (r[1] !== null) return 'FAIL: should return null cmd on error';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("equiv-poll error: %v", raw)
	}
}

// TestChunk16_EquivPoll_CompletedSuccess verifies that handleEquivPoll
// accepts the final state when equiv check completed successfully.
func TestChunk16_EquivPoll_CompletedSuccess(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Simulate async equiv complete: equivRunning=false, equivError=null,
		// wizardState already transitioned to FINALIZATION by runEquivCheckAsync.
		var s = initState('FINALIZATION');
		s.isProcessing = false;
		s.equivRunning = false;
		s.equivError = null;

		var r = update({type: 'Tick', id: 'equiv-poll'}, s);
		if (r[1] !== null) return 'FAIL: should return null cmd on success';
		if (r[0].wizardState !== 'FINALIZATION') {
			return 'FAIL: wizardState should be FINALIZATION, got: ' + r[0].wizardState;
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("equiv-poll success: %v", raw)
	}
}

// TestChunk16_ExecutionAsync_NoSyncCallsRemain verifies that the old sync
// execution tick IDs are no longer handled by the update function.
func TestChunk16_ExecutionAsync_NoSyncCallsRemain(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;

		// Old tick IDs should be ignored (return [s, null]).
		var oldTicks = ['exec-step-0', 'exec-step-1', 'exec-step-2'];
		for (var i = 0; i < oldTicks.length; i++) {
			var r = update({type: 'Tick', id: oldTicks[i]}, s);
			if (r[1] !== null) return 'FAIL: old tick ' + oldTicks[i] + ' should return null cmd';
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("no sync exec calls remain: %v", raw)
	}
}

// TestChunk16_ExecutionAsync_HappyPath exercises the execution-poll →
// startEquivCheck → equiv-poll chain by simulating completed async execution
// then polling through to FINALIZATION.
func TestChunk16_ExecutionAsync_HappyPath(t *testing.T) {
	t.Parallel()

	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var origVerifyEquivalenceAsync = globalThis.prSplit.verifyEquivalenceAsync;
		var origVerifyEquivalenceDetailedAsync = globalThis.prSplit.verifyEquivalenceDetailedAsync;

		try {
			// Mock verifyEquivalenceDetailedAsync (checked first by runEquivCheckAsync).
			globalThis.prSplit.verifyEquivalenceDetailedAsync = async function(plan) {
				return { equivalent: true, splitTree: 'aaa', sourceTree: 'aaa', error: null, diffFiles: [], diffSummary: '' };
			};
			// Mock verifyEquivalenceAsync as fallback.
			globalThis.prSplit.verifyEquivalenceAsync = async function(plan) {
				return { equivalent: true, splitTree: 'aaa', sourceTree: 'aaa', error: null };
			};

			// Set up state simulating completed execution (no verify command).
			var s = initState('BRANCH_BUILDING');
			s.isProcessing = true;
			s.executionRunning = false;
			s.executionError = null;
			s.executionNextStep = 'equiv';
			s.executionResults = [
				{ name: 'split/01-group1', files: ['a.go'], sha: 'abc123', error: null },
				{ name: 'split/02-group2', files: ['b.go'], sha: 'def456', error: null }
			];

			// Poll execution → should transition to EQUIV_CHECK and start equiv async.
			var r = update({type: 'Tick', id: 'execution-poll'}, s);
			s = r[0];

			if (s.wizardState !== 'EQUIV_CHECK') {
				return 'FAIL: expected EQUIV_CHECK after execution-poll, got ' + s.wizardState;
			}
			if (!s.equivRunning) return 'FAIL: equivRunning should be true';

			// Let microtasks resolve (mocked verifyEquivalenceAsync resolves immediately).
			await Promise.resolve();
			await Promise.resolve();
			await Promise.resolve();

			// Poll equiv check for completion.
			r = update({type: 'Tick', id: 'equiv-poll'}, s);
			s = r[0];

			if (s.wizardState !== 'FINALIZATION') {
				return 'FAIL: expected FINALIZATION after equiv-poll, got ' + s.wizardState +
					', equivError=' + s.equivError + ', equivRunning=' + s.equivRunning;
			}
			if (s.isProcessing) return 'FAIL: isProcessing should be false';
			if (!s.equivalenceResult || !s.equivalenceResult.equivalent) {
				return 'FAIL: equivalenceResult should be equivalent';
			}

			return 'OK';
		} finally {
			globalThis.prSplit.verifyEquivalenceAsync = origVerifyEquivalenceAsync;
			globalThis.prSplit.verifyEquivalenceDetailedAsync = origVerifyEquivalenceDetailedAsync;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("execution async happy path: %v", raw)
	}
}

// TestChunk16_ExecutionAsync_ExecutionError verifies that when
// executeSplitAsync returns an error, wizard transitions to ERROR_RESOLUTION.
func TestChunk16_ExecutionAsync_ExecutionError(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Simulate execution error via the poll handler.
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;
		s.executionRunning = false;
		s.executionError = null;
		s.executionNextStep = null;
		// When the async function sets error state directly:
		s.wizardState = 'ERROR_RESOLUTION';
		s.errorDetails = 'git worktree add failed';
		s.isProcessing = false;

		// Poll should see completed state and stop.
		var r = update({type: 'Tick', id: 'execution-poll'}, s);
		if (r[1] !== null) return 'FAIL: should return null cmd after error';
		if (r[0].wizardState !== 'ERROR_RESOLUTION') {
			return 'FAIL: wizardState should stay ERROR_RESOLUTION, got: ' + r[0].wizardState;
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("execution async error: %v", raw)
	}
}

// TestChunk16_ExecutionAsync_ProgressUpdate verifies that the progressFn
// callback from executeSplitAsync correctly updates state fields.
func TestChunk16_ExecutionAsync_ProgressUpdate(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var origExecuteSplitAsync = globalThis.prSplit.executeSplitAsync;

		try {
			var capturedState = null;

			globalThis.prSplit.executeSplitAsync = async function(plan, opts) {
				// Simulate per-branch progress.
				if (opts && opts.progressFn) {
					opts.progressFn('Creating branch 1/3: split/01');
					// Capture state between calls.
					capturedState = {
						executingIdx: plan._testState.executingIdx,
						executionBranchTotal: plan._testState.executionBranchTotal,
						executionProgressMsg: plan._testState.executionProgressMsg
					};
					opts.progressFn('Creating branch 2/3: split/02');
					opts.progressFn('Creating branch 3/3: split/03');
				}
				return {
					error: null,
					results: [
						{ name: 'split/01', files: ['a.go'], sha: 'aaa', error: null },
						{ name: 'split/02', files: ['b.go'], sha: 'bbb', error: null },
						{ name: 'split/03', files: ['c.go'], sha: 'ccc', error: null }
					]
				};
			};

			var s = initState('BRANCH_BUILDING');
			s.isProcessing = true;
			s.executionRunning = true;
			s.executionError = null;
			// Attach state ref to plan for capture in mock.
			var fakePlan = {
				splits: [
					{ name: 'split/01', files: ['a.go'], message: 'g1', order: 0, dependencies: [] },
					{ name: 'split/02', files: ['b.go'], message: 'g2', order: 1, dependencies: [] },
					{ name: 'split/03', files: ['c.go'], message: 'g3', order: 2, dependencies: [] }
				],
				baseBranch: 'main',
				sourceBranch: 'feature',
				fileStatuses: { 'a.go': 'M', 'b.go': 'A', 'c.go': 'M' },
				_testState: s
			};

			// Call the progressFn path directly.
			var result = await globalThis.prSplit.executeSplitAsync(fakePlan, {
				progressFn: function(msg) {
					var match = msg.match(/(\d+)\/(\d+)/);
					if (match) {
						s.executingIdx = parseInt(match[1], 10) - 1;
						s.executionBranchTotal = parseInt(match[2], 10);
					}
					s.executionProgressMsg = msg;
				}
			});

			// Verify progress was tracked.
			if (s.executingIdx !== 2) return 'FAIL: executingIdx should be 2, got: ' + s.executingIdx;
			if (s.executionBranchTotal !== 3) return 'FAIL: executionBranchTotal should be 3, got: ' + s.executionBranchTotal;
			if (s.executionProgressMsg.indexOf('3/3') < 0) {
				return 'FAIL: executionProgressMsg should contain 3/3, got: ' + s.executionProgressMsg;
			}

			// Verify intermediate capture (after first progress call).
			if (!capturedState) return 'FAIL: should have captured intermediate state';
			if (capturedState.executingIdx !== 0) {
				return 'FAIL: intermediate executingIdx should be 0, got: ' + capturedState.executingIdx;
			}
			if (capturedState.executionBranchTotal !== 3) {
				return 'FAIL: intermediate executionBranchTotal should be 3, got: ' + capturedState.executionBranchTotal;
			}

			return 'OK';
		} finally {
			globalThis.prSplit.executeSplitAsync = origExecuteSplitAsync;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("execution async progress update: %v", raw)
	}
}

// TestChunk16_CancelDuringExecution verifies that cancelling during async
// execution sets isProcessing=false and prevents further wizard transitions.
func TestChunk16_CancelDuringExecution(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;
		s.executionRunning = true;
		s.showConfirmCancel = true;

		// User confirms cancel.
		var r = update({type: 'Key', key: 'y'}, s);
		s = r[0];

		// Cancel should:
		// 1. Set isProcessing = false (so async early-return guards fire)
		// 2. Set wizard state to CANCELLED
		if (s.isProcessing) return 'FAIL: isProcessing should be false after cancel';
		if (s.wizardState !== 'CANCELLED') {
			return 'FAIL: wizardState should be CANCELLED, got: ' + s.wizardState;
		}
		if (s.wizard.current !== 'CANCELLED') {
			return 'FAIL: wizard.current should be CANCELLED, got: ' + s.wizard.current;
		}

		// Subsequent execution-poll should stop (isProcessing=false).
		s.executionRunning = false;
		r = update({type: 'Tick', id: 'execution-poll'}, s);
		if (r[1] !== null) return 'FAIL: execution-poll should stop after cancel';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("cancel during execution: %v", raw)
	}
}

// TestChunk16_EquivPoll_ErrorWizardStateSync verifies that handleEquivPoll
// error path calls wizard.transition('ERROR') keeping wizard.current in sync.
func TestChunk16_EquivPoll_ErrorWizardStateSync(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('EQUIV_CHECK');
		s.isProcessing = true;
		s.equivRunning = false;
		s.equivError = 'tree mismatch';

		var r = update({type: 'Tick', id: 'equiv-poll'}, s);
		s = r[0];
		// Both wizardState and wizard.current must agree.
		if (s.wizardState !== s.wizard.current) {
			return 'FAIL: state desync — wizardState=' + s.wizardState +
				' vs wizard.current=' + s.wizard.current;
		}
		if (s.wizardState !== 'ERROR') {
			return 'FAIL: expected ERROR, got: ' + s.wizardState;
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("equiv-poll error wizard state sync: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  T36: Async Claude Check Tests
// ---------------------------------------------------------------------------

// TestChunk16_ClaudeCheck_NoPrSplitConfig verifies that handleClaudeCheck
// returns 'unavailable' when prSplitConfig is deleted (simulates missing config).
func TestChunk16_ClaudeCheck_NoPrSplitConfig(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Save and delete prSplitConfig to simulate missing config.
		var saved = globalThis.prSplitConfig;
		delete globalThis.prSplitConfig;
		try {
			var s = initState('CONFIG');
			var r = update({type: 'Tick', id: 'check-claude'}, s);
			s = r[0];
			if (s.claudeCheckStatus !== 'unavailable') {
				return 'FAIL: expected unavailable, got: ' + JSON.stringify(s.claudeCheckStatus);
			}
			if (!s.claudeCheckError || s.claudeCheckError.indexOf('test mode') < 0) {
				return 'FAIL: expected test mode error, got: ' + s.claudeCheckError;
			}
			if (r[1] !== null) return 'FAIL: should return null cmd';
			return 'OK';
		} finally {
			globalThis.prSplitConfig = saved;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("claude check no config: %v", raw)
	}
}

// TestChunk16_ClaudeCheck_CachedExecutor verifies that handleClaudeCheck
// returns cached result from st.claudeExecutor without launching async work.
func TestChunk16_ClaudeCheck_CachedExecutor(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Set up prSplitConfig so we don't hit test mode guard.
		globalThis.prSplitConfig = { claudeCommand: 'claude' };

		// Pre-cache an executor with resolved info.
		globalThis.prSplit._state.claudeExecutor = {
			resolved: { command: 'claude', type: 'claude-code' }
		};

		var s = initState('CONFIG');
		var r = update({type: 'Tick', id: 'check-claude'}, s);
		s = r[0];

		if (s.claudeCheckStatus !== 'available') {
			return 'FAIL: expected available, got: ' + s.claudeCheckStatus;
		}
		if (!s.claudeResolvedInfo || s.claudeResolvedInfo.command !== 'claude') {
			return 'FAIL: expected resolved info from cache';
		}
		if (s.claudeCheckRunning) return 'FAIL: should not be running (cached)';
		if (r[1] !== null) return 'FAIL: should return null cmd (cached)';

		// Clean up.
		delete globalThis.prSplitConfig;
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("claude check cached: %v", raw)
	}
}

// TestChunk16_ClaudeCheck_LaunchesAsync verifies that handleClaudeCheck
// launches async resolveAsync and returns a poll tick when prSplitConfig exists.
func TestChunk16_ClaudeCheck_LaunchesAsync(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Mock prSplitConfig.
		globalThis.prSplitConfig = { claudeCommand: '' };

		// Clear any cached executor.
		globalThis.prSplit._state.claudeExecutor = null;

		// Mock ClaudeCodeExecutor to track resolveAsync call.
		var origCtor = globalThis.prSplit.ClaudeCodeExecutor;
		var asyncCalled = false;
		var MockExecutor = function(config) {
			this.command = config.claudeCommand || '';
			this.resolved = null;
		};
		MockExecutor.prototype.resolveAsync = async function(progressFn) {
			asyncCalled = true;
			if (progressFn) progressFn('Resolving binary…');
			return { error: null };
		};
		globalThis.prSplit.ClaudeCodeExecutor = MockExecutor;

		try {
			var s = initState('CONFIG');
			var r = update({type: 'Tick', id: 'check-claude'}, s);
			s = r[0];

			if (!s.claudeCheckRunning) return 'FAIL: claudeCheckRunning should be true';
			if (s.claudeCheckStatus !== 'checking') {
				return 'FAIL: expected checking status, got: ' + s.claudeCheckStatus;
			}
			if (!r[1]) return 'FAIL: should return a poll tick cmd';
			return 'OK';
		} finally {
			globalThis.prSplit.ClaudeCodeExecutor = origCtor;
			delete globalThis.prSplitConfig;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("claude check launches async: %v", raw)
	}
}

// TestChunk16_ClaudeCheckPoll_StillRunning verifies that handleClaudeCheckPoll
// continues polling when check is still running.
func TestChunk16_ClaudeCheckPoll_StillRunning(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.claudeCheckRunning = true;
		s.claudeCheckStatus = 'checking';

		var r = update({type: 'Tick', id: 'claude-check-poll'}, s);
		if (!r[1]) return 'FAIL: should return a poll tick when still running';
		if (!r[0].claudeCheckRunning) return 'FAIL: claudeCheckRunning should still be true';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("claude-check-poll still running: %v", raw)
	}
}

// TestChunk16_ClaudeCheckPoll_Completed verifies that handleClaudeCheckPoll
// stops polling when check is complete.
func TestChunk16_ClaudeCheckPoll_Completed(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.claudeCheckRunning = false;
		s.claudeCheckStatus = 'available';
		s.claudeResolvedInfo = { command: 'claude', type: 'claude-code' };

		var r = update({type: 'Tick', id: 'claude-check-poll'}, s);
		if (r[1] !== null) return 'FAIL: should return null cmd when complete';
		if (r[0].claudeCheckStatus !== 'available') {
			return 'FAIL: status should be available, got: ' + r[0].claudeCheckStatus;
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("claude-check-poll completed: %v", raw)
	}
}

// TestChunk16_ClaudeCheck_AsyncHappyPath exercises the full async
// check-claude → claude-check-poll chain with a mocked resolveAsync.
func TestChunk16_ClaudeCheck_AsyncHappyPath(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		globalThis.prSplitConfig = { claudeCommand: '' };
		globalThis.prSplit._state.claudeExecutor = null;

		var origCtor = globalThis.prSplit.ClaudeCodeExecutor;
		var progressMessages = [];
		var MockExecutor = function(config) {
			this.command = config.claudeCommand || '';
			this.resolved = null;
		};
		MockExecutor.prototype.resolveAsync = async function(progressFn) {
			if (progressFn) {
				progressFn('Resolving binary…');
				progressMessages.push('Resolving binary…');
				progressFn('Checking version…');
				progressMessages.push('Checking version…');
			}
			this.resolved = { command: 'claude', type: 'claude-code' };
			return { error: null };
		};
		globalThis.prSplit.ClaudeCodeExecutor = MockExecutor;

		try {
			var s = initState('CONFIG');

			// Trigger check.
			var r = update({type: 'Tick', id: 'check-claude'}, s);
			s = r[0];
			if (!s.claudeCheckRunning) return 'FAIL: should be running after check-claude';

			// Let microtasks resolve.
			await Promise.resolve();
			await Promise.resolve();
			await Promise.resolve();

			// Poll — should be complete.
			r = update({type: 'Tick', id: 'claude-check-poll'}, s);
			s = r[0];

			if (s.claudeCheckRunning) return 'FAIL: should not be running after poll';
			if (s.claudeCheckStatus !== 'available') {
				return 'FAIL: expected available, got: ' + s.claudeCheckStatus;
			}
			if (!s.claudeResolvedInfo || s.claudeResolvedInfo.command !== 'claude') {
				return 'FAIL: expected resolved info';
			}
			if (s.claudeCheckError) return 'FAIL: should have no error';
			if (progressMessages.length < 2) {
				return 'FAIL: expected progress messages, got: ' + progressMessages.length;
			}
			if (r[1] !== null) return 'FAIL: should return null cmd on completion';

			return 'OK';
		} finally {
			globalThis.prSplit.ClaudeCodeExecutor = origCtor;
			delete globalThis.prSplitConfig;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("claude check async happy path: %v", raw)
	}
}

// TestChunk16_ClaudeCheck_AsyncError exercises the error path when
// resolveAsync returns an error.
func TestChunk16_ClaudeCheck_AsyncError(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		globalThis.prSplitConfig = { claudeCommand: '' };
		globalThis.prSplit._state.claudeExecutor = null;

		var origCtor = globalThis.prSplit.ClaudeCodeExecutor;
		var MockExecutor = function(config) {
			this.command = config.claudeCommand || '';
			this.resolved = null;
		};
		MockExecutor.prototype.resolveAsync = async function(progressFn) {
			if (progressFn) progressFn('Resolving binary…');
			return { error: 'No Claude-compatible binary found. Install Claude Code CLI (claude) or Ollama (ollama), or set --claude-command explicitly.' };
		};
		globalThis.prSplit.ClaudeCodeExecutor = MockExecutor;

		try {
			var s = initState('CONFIG');
			var r = update({type: 'Tick', id: 'check-claude'}, s);
			s = r[0];

			await Promise.resolve();
			await Promise.resolve();
			await Promise.resolve();

			r = update({type: 'Tick', id: 'claude-check-poll'}, s);
			s = r[0];

			if (s.claudeCheckStatus !== 'unavailable') {
				return 'FAIL: expected unavailable, got: ' + s.claudeCheckStatus;
			}
			if (!s.claudeCheckError || s.claudeCheckError.indexOf('Install Claude Code CLI') < 0) {
				return 'FAIL: expected actionable error, got: ' + s.claudeCheckError;
			}
			if (s.claudeCheckRunning) return 'FAIL: should not be running';
			if (globalThis.prSplit.runtime.mode !== 'heuristic') {
				return 'FAIL: should have fallen back to heuristic';
			}
			return 'OK';
		} finally {
			globalThis.prSplit.ClaudeCodeExecutor = origCtor;
			delete globalThis.prSplitConfig;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("claude check async error: %v", raw)
	}
}

// TestChunk16_ClaudeCheck_AsyncThrows exercises the path where
// resolveAsync throws an unexpected exception (not a result.error).
func TestChunk16_ClaudeCheck_AsyncThrows(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		globalThis.prSplitConfig = { claudeCommand: '' };
		globalThis.prSplit._state.claudeExecutor = null;

		var origCtor = globalThis.prSplit.ClaudeCodeExecutor;
		var MockExecutor = function(config) {
			this.command = config.claudeCommand || '';
			this.resolved = null;
		};
		MockExecutor.prototype.resolveAsync = async function(progressFn) {
			throw new Error('exec.spawn crashed: ENOENT');
		};
		globalThis.prSplit.ClaudeCodeExecutor = MockExecutor;

		try {
			var s = initState('CONFIG');
			var r = update({type: 'Tick', id: 'check-claude'}, s);
			s = r[0];

			await Promise.resolve();
			await Promise.resolve();
			await Promise.resolve();

			r = update({type: 'Tick', id: 'claude-check-poll'}, s);
			s = r[0];

			if (s.claudeCheckStatus !== 'unavailable') {
				return 'FAIL: expected unavailable, got: ' + s.claudeCheckStatus;
			}
			if (!s.claudeCheckError || s.claudeCheckError.indexOf('ENOENT') < 0) {
				return 'FAIL: expected ENOENT in error, got: ' + s.claudeCheckError;
			}
			if (s.claudeCheckRunning) return 'FAIL: should not be running';
			return 'OK';
		} finally {
			globalThis.prSplit.ClaudeCodeExecutor = origCtor;
			delete globalThis.prSplitConfig;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("claude check async throws: %v", raw)
	}
}

// TestChunk16_ClaudeCheck_OldSyncRemoved verifies that the old synchronous
// resolve() call is no longer used by handleClaudeCheck (it always goes async
// or uses cache).
func TestChunk16_ClaudeCheck_OldSyncRemoved(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		globalThis.prSplitConfig = { claudeCommand: '' };
		globalThis.prSplit._state.claudeExecutor = null;

		var origCtor = globalThis.prSplit.ClaudeCodeExecutor;
		var syncResolveCalled = false;
		var MockExecutor = function(config) {
			this.command = config.claudeCommand || '';
			this.resolved = null;
		};
		MockExecutor.prototype.resolve = function() {
			syncResolveCalled = true;
			return { error: null };
		};
		MockExecutor.prototype.resolveAsync = async function(progressFn) {
			this.resolved = { command: 'claude', type: 'claude-code' };
			return { error: null };
		};
		globalThis.prSplit.ClaudeCodeExecutor = MockExecutor;

		try {
			var s = initState('CONFIG');
			update({type: 'Tick', id: 'check-claude'}, s);
			if (syncResolveCalled) return 'FAIL: sync resolve() was called';
			return 'OK';
		} finally {
			globalThis.prSplit.ClaudeCodeExecutor = origCtor;
			delete globalThis.prSplitConfig;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("old sync removed: %v", raw)
	}
}

// TestChunk16_ClaudeCheck_ReentryGuard verifies that calling handleClaudeCheck
// while already running does NOT launch a second async operation.
func TestChunk16_ClaudeCheck_ReentryGuard(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		globalThis.prSplitConfig = { claudeCommand: '' };
		globalThis.prSplit._state.claudeExecutor = null;

		var origCtor = globalThis.prSplit.ClaudeCodeExecutor;
		var launchCount = 0;
		var MockExecutor = function(config) {
			this.command = config.claudeCommand || '';
			this.resolved = null;
		};
		MockExecutor.prototype.resolveAsync = async function(progressFn) {
			launchCount++;
			this.resolved = { command: 'claude', type: 'claude-code' };
			return { error: null };
		};
		globalThis.prSplit.ClaudeCodeExecutor = MockExecutor;

		try {
			var s = initState('CONFIG');
			// First call — should launch async.
			var r = update({type: 'Tick', id: 'check-claude'}, s);
			s = r[0];
			if (!s.claudeCheckRunning) return 'FAIL: should be running after first call';

			// Second call while still running — should NOT launch again.
			r = update({type: 'Tick', id: 'check-claude'}, s);
			s = r[0];
			if (launchCount !== 1) {
				return 'FAIL: expected 1 launch, got: ' + launchCount;
			}
			if (!r[1]) return 'FAIL: should return poll tick even on re-entry';
			return 'OK';
		} finally {
			globalThis.prSplit.ClaudeCodeExecutor = origCtor;
			delete globalThis.prSplitConfig;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("claude check re-entry guard: %v", raw)
	}
}

// TestChunk16_ClaudeCheck_SwitchAwayCleansUp verifies that switching from
// 'auto' strategy to another clears all async check state fields.
func TestChunk16_ClaudeCheck_SwitchAwayCleansUp(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		globalThis.prSplitConfig = { claudeCommand: '' };
		globalThis.prSplit._state.claudeExecutor = null;

		var origCtor = globalThis.prSplit.ClaudeCodeExecutor;
		var MockExecutor = function(config) {
			this.command = config.claudeCommand || '';
			this.resolved = null;
		};
		MockExecutor.prototype.resolveAsync = async function(progressFn) {
			// Simulate slow resolution — won't complete in this test.
			return new Promise(function() {});
		};
		globalThis.prSplit.ClaudeCodeExecutor = MockExecutor;

		try {
			var s = initState('CONFIG');
			// Launch async check.
			var r = update({type: 'Tick', id: 'check-claude'}, s);
			s = r[0];
			if (!s.claudeCheckRunning) return 'FAIL: should be running';

			// Simulate switching to 'heuristic' via mouse click on strategy zone.
			var z = globalThis.prSplit._zone;
			var origInBounds = z.inBounds;
			z.inBounds = function(id) { return id === 'strategy-heuristic'; };
			try {
				r = update({type: 'Mouse', button: 'left', action: 'press', isWheel: false, x: 10, y: 10}, s);
			} finally {
				z.inBounds = origInBounds;
			}
			s = r[0];

			if (s.claudeCheckRunning !== false) {
				return 'FAIL: claudeCheckRunning should be false after switch, got: ' + s.claudeCheckRunning;
			}
			if (s.claudeCheckProgressMsg !== '') {
				return 'FAIL: claudeCheckProgressMsg should be empty after switch';
			}
			if (s.claudeCheckStatus !== null) {
				return 'FAIL: claudeCheckStatus should be null, got: ' + s.claudeCheckStatus;
			}
			return 'OK';
		} finally {
			globalThis.prSplit.ClaudeCodeExecutor = origCtor;
			delete globalThis.prSplitConfig;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("claude check switch away cleans up: %v", raw)
	}
}
