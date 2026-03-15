package command

import (
	"testing"
)

// ---------------------------------------------------------------------------
//  T121: automatedSplit report.equivalence propagation + handleNext safety net
//
//  Verifies that:
//  1. handleAutoSplitPoll transitions to EQUIV_CHECK when report.equivalence is set
//  2. handleAutoSplitPoll transitions to BRANCH_BUILDING when equivalence is missing
//  3. handleNext from BRANCH_BUILDING calls startEquivCheck (safety net)
//  4. handleNext from EQUIV_CHECK with cached result advances to FINALIZATION
// ---------------------------------------------------------------------------

// TestChunk16_AutoSplitPoll_WithEquivalence verifies that when the auto-split
// pipeline completes with report.equivalence set, the wizard transitions through
// BRANCH_BUILDING → EQUIV_CHECK.
func TestChunk16_AutoSplitPoll_WithEquivalence(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('CONFIG');
		s.isProcessing = true;
		s.autoSplitRunning = false; // pipeline just completed
		s.autoSplitError = null;
		s.analysisSteps = [
			{ label: 'a', active: false, done: false },
			{ label: 'b', active: false, done: false }
		];
		s.analysisProgress = 0;

		// Simulate a completed auto-split report with equivalence data.
		s.autoSplitResult = {
			report: {
				plan: globalThis.prSplit._state.planCache,
				splits: [
					{name: 'split/api', status: 'ok'},
					{name: 'split/cli', status: 'ok'}
				],
				equivalence: {
					equivalent: true,
					splitTree: 'abc123',
					sourceTree: 'abc123'
				}
			}
		};

		var r = update({type: 'Tick', id: 'auto-poll'}, s);
		var state = r[0];

		// T121: Should transition to EQUIV_CHECK (not stuck at BRANCH_BUILDING).
		if (state.wizardState !== 'EQUIV_CHECK') {
			return 'FAIL: expected EQUIV_CHECK, got ' + state.wizardState;
		}

		// equivalenceResult should be populated from report.
		if (!state.equivalenceResult) {
			return 'FAIL: equivalenceResult should be set from report';
		}
		if (state.equivalenceResult.splitTree !== 'abc123') {
			return 'FAIL: splitTree mismatch';
		}

		// isProcessing should be false.
		if (state.isProcessing) {
			return 'FAIL: isProcessing should be false after auto-split completes';
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("auto-split with equivalence: %v", raw)
	}
}

// TestChunk16_AutoSplitPoll_WithoutEquivalence verifies that when equivalence
// is missing from the report, wizard transitions to BRANCH_BUILDING (the
// original bug state — but now the safety net in handleNext can rescue).
func TestChunk16_AutoSplitPoll_WithoutEquivalence(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('CONFIG');
		s.isProcessing = true;
		s.autoSplitRunning = false;
		s.autoSplitError = null;
		s.analysisSteps = [
			{ label: 'a', active: false, done: false },
			{ label: 'b', active: false, done: false }
		];
		s.analysisProgress = 0;

		// Report WITHOUT equivalence data (edge case: step() was cancelled).
		s.autoSplitResult = {
			report: {
				plan: globalThis.prSplit._state.planCache,
				splits: [
					{name: 'split/api', status: 'ok'}
				]
				// No equivalence field!
			}
		};

		var r = update({type: 'Tick', id: 'auto-poll'}, s);
		var state = r[0];

		// Without equivalence, falls to BRANCH_BUILDING.
		if (state.wizardState !== 'BRANCH_BUILDING') {
			return 'FAIL: expected BRANCH_BUILDING, got ' + state.wizardState;
		}

		// equivalenceResult must NOT be populated when report lacks equivalence.
		if (state.equivalenceResult) {
			return 'FAIL: equivalenceResult should be null/undefined when report has no equivalence';
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("auto-split without equivalence: %v", raw)
	}
}

// TestChunk16_HandleNext_BranchBuilding_StartsEquivCheck verifies the T121
// safety net: pressing Enter/Next from BRANCH_BUILDING calls startEquivCheck.
func TestChunk16_HandleNext_BranchBuilding_StartsEquivCheck(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = false;
		s.focusIndex = 0;

		// Mock equiv functions so startEquivCheck doesn't fail.
		var origVerify = prSplit.verifyEquivalenceAsync;
		prSplit.verifyEquivalenceAsync = function() {
			return Promise.resolve({
				equivalent: true,
				splitTree: 'deadbeef',
				sourceTree: 'deadbeef'
			});
		};

		// Send 'enter' to trigger handleNext from BRANCH_BUILDING.
		var r = sendKey(s, 'enter');
		var state = r[0];

		prSplit.verifyEquivalenceAsync = origVerify;

		// T121: Safety net should set isProcessing=true and transition to EQUIV_CHECK.
		if (!state.isProcessing) {
			return 'FAIL: isProcessing should be true (equiv check started)';
		}
		if (state.wizardState !== 'EQUIV_CHECK') {
			return 'FAIL: expected EQUIV_CHECK, got ' + state.wizardState;
		}

		// Should return a tick command for equiv-poll.
		var cmd = r[1];
		if (cmd === null) {
			return 'FAIL: expected tick command for equiv-poll, got null';
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("handleNext BRANCH_BUILDING safety net: %v", raw)
	}
}

// TestChunk16_HandleNext_EquivCheck_CachedResult_AdvancesToFinalization verifies
// that when equivalence results are cached (from auto-split pipeline), pressing
// Enter from EQUIV_CHECK transitions to FINALIZATION.
func TestChunk16_HandleNext_EquivCheck_CachedResult_AdvancesToFinalization(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('EQUIV_CHECK');
		s.isProcessing = false;
		s.equivalenceResult = {
			equivalent: true,
			splitTree: 'abc123',
			sourceTree: 'abc123'
		};
		s.focusIndex = 0;

		// Send 'enter' to trigger handleNext from EQUIV_CHECK.
		var r = sendKey(s, 'enter');
		var state = r[0];

		// T121: Should advance to FINALIZATION.
		if (state.wizardState !== 'FINALIZATION') {
			return 'FAIL: expected FINALIZATION, got ' + state.wizardState;
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("handleNext EQUIV_CHECK with cached result: %v", raw)
	}
}
