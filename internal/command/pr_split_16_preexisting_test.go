package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  T115: Pre-existing failure detection in PTY pipeline
//
//  Tests that pollVerifySession, handleVerifyFallbackPoll, and
//  runVerifyFallbackAsync correctly tag failures as preExisting when
//  the cached baseline verification also fails.
// ---------------------------------------------------------------------------

// TestChunk16_PollVerifySession_PreExisting_BaselineFailed verifies that
// pollVerifySession marks the result as preExisting:true when the cached
// baseline check indicates source branch also fails.
func TestChunk16_PollVerifySession_PreExisting_BaselineFailed(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;
		s.verifyingIdx = 0;
		s.verifyOutput = {};
		s.verificationResults = [];
		s.outputLines = [];
		s.outputAutoScroll = true;

		// Simulate a completed CaptureSession that FAILED (exit code 1).
		s.activeVerifySession = {
			isDone: function() { return true; },
			exitCode: function() { return 1; },
			output: function() { return 'FAIL: test assertion'; },
			screen: function() { return 'FAIL: test assertion'; },
			close: function() {},
			kill: function() {}
		};
		s.activeVerifyWorktree = null;
		s.activeVerifyDir = null;
		s.activeVerifyBranch = 'split/api';
		s.activeVerifyStartTime = Date.now() - 1000;

		// T115: Simulate baseline check having completed — baseline FAILED.
		s._baselineVerifyStarted = true;
		s._baselineVerifyResult = { failed: true, sourceBranch: 'feature' };

		var r = update({type: 'Tick', id: 'verify-poll'}, s);
		var results = r[0].verificationResults;
		if (results.length !== 1) return 'FAIL: expected 1 result, got ' + results.length;
		var res = results[0];
		if (res.preExisting !== true) return 'FAIL: expected preExisting=true, got ' + res.preExisting;
		if (res.name !== 'split/api') return 'FAIL: expected name=split/api, got ' + res.name;
		if (res.passed) return 'FAIL: should not be passed';
		if (!res.error || res.error.indexOf('pre-existing on feature') < 0) {
			return 'FAIL: error should mention pre-existing, got: ' + res.error;
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("preExisting baseline failed: %v", raw)
	}
}

// TestChunk16_PollVerifySession_PreExisting_BaselinePassed verifies that
// pollVerifySession leaves preExisting:false when baseline check passed.
func TestChunk16_PollVerifySession_PreExisting_BaselinePassed(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;
		s.verifyingIdx = 0;
		s.verifyOutput = {};
		s.verificationResults = [];
		s.outputLines = [];
		s.outputAutoScroll = true;

		s.activeVerifySession = {
			isDone: function() { return true; },
			exitCode: function() { return 1; },
			output: function() { return 'FAIL: new regression'; },
			screen: function() { return 'FAIL: new regression'; },
			close: function() {},
			kill: function() {}
		};
		s.activeVerifyWorktree = null;
		s.activeVerifyDir = null;
		s.activeVerifyBranch = 'split/api';
		s.activeVerifyStartTime = Date.now() - 500;

		// Baseline check completed — baseline PASSED.
		s._baselineVerifyStarted = true;
		s._baselineVerifyResult = { failed: false, sourceBranch: 'feature' };

		var r = update({type: 'Tick', id: 'verify-poll'}, s);
		var results = r[0].verificationResults;
		if (results.length !== 1) return 'FAIL: expected 1 result, got ' + results.length;
		var res = results[0];
		if (res.preExisting !== false) return 'FAIL: expected preExisting=false, got ' + res.preExisting;
		if (res.error && res.error.indexOf('pre-existing') >= 0) {
			return 'FAIL: error should NOT mention pre-existing, got: ' + res.error;
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("preExisting baseline passed: %v", raw)
	}
}

// TestChunk16_PollVerifySession_PreExisting_NotYetAvailable verifies that
// pollVerifySession falls back to preExisting:false when the baseline
// result hasn't been cached yet (Promise still pending).
func TestChunk16_PollVerifySession_PreExisting_NotYetAvailable(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;
		s.verifyingIdx = 0;
		s.verifyOutput = {};
		s.verificationResults = [];
		s.outputLines = [];
		s.outputAutoScroll = true;

		s.activeVerifySession = {
			isDone: function() { return true; },
			exitCode: function() { return 1; },
			output: function() { return 'some error'; },
			screen: function() { return 'some error'; },
			close: function() {},
			kill: function() {}
		};
		s.activeVerifyWorktree = null;
		s.activeVerifyDir = null;
		s.activeVerifyBranch = 'split/api';
		s.activeVerifyStartTime = Date.now() - 200;

		// Baseline started but result not yet available.
		s._baselineVerifyStarted = true;
		s._baselineVerifyResult = null;

		var r = update({type: 'Tick', id: 'verify-poll'}, s);
		var results = r[0].verificationResults;
		if (results.length !== 1) return 'FAIL: expected 1 result, got ' + results.length;
		var res = results[0];
		if (res.preExisting !== false) return 'FAIL: expected preExisting=false when not available, got ' + res.preExisting;
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("preExisting not yet available: %v", raw)
	}
}

// TestChunk16_PollVerifySession_Passing_NoPreExisting verifies that
// pollVerifySession does NOT set preExisting when the branch passes.
func TestChunk16_PollVerifySession_Passing_NoPreExisting(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;
		s.verifyingIdx = 0;
		s.verifyOutput = {};
		s.verificationResults = [];
		s.outputLines = [];
		s.outputAutoScroll = true;

		s.activeVerifySession = {
			isDone: function() { return true; },
			exitCode: function() { return 0; },
			output: function() { return 'all tests pass'; },
			screen: function() { return 'all tests pass'; },
			close: function() {},
			kill: function() {}
		};
		s.activeVerifyWorktree = null;
		s.activeVerifyDir = null;
		s.activeVerifyBranch = 'split/api';
		s.activeVerifyStartTime = Date.now() - 300;

		// Even though baseline failed, passing branch should not be marked.
		s._baselineVerifyStarted = true;
		s._baselineVerifyResult = { failed: true, sourceBranch: 'feature' };

		var r = update({type: 'Tick', id: 'verify-poll'}, s);
		var results = r[0].verificationResults;
		if (results.length !== 1) return 'FAIL: expected 1 result, got ' + results.length;
		var res = results[0];
		if (res.preExisting !== false) return 'FAIL: passing branch should have preExisting=false, got ' + res.preExisting;
		if (!res.passed) return 'FAIL: exit 0 should be passed=true';
		if (res.error) return 'FAIL: passing branch should have no error, got: ' + res.error;
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("passing branch preExisting: %v", raw)
	}
}

// TestChunk16_FallbackPoll_PreExisting_BaselineFailed verifies that
// handleVerifyFallbackPoll marks error results as preExisting when
// the cached baseline also failed.
func TestChunk16_FallbackPoll_PreExisting_BaselineFailed(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;
		s.verifyingIdx = 0;
		s.verifyOutput = {};
		s.verificationResults = [];
		s.verifyFallbackRunning = false;
		s.verifyFallbackError = 'worktree creation failed';

		// Baseline FAILED.
		s._baselineVerifyStarted = true;
		s._baselineVerifyResult = { failed: true, sourceBranch: 'feature' };

		var r = update({type: 'Tick', id: 'verify-fallback-poll'}, s);
		var results = r[0].verificationResults;
		if (results.length !== 1) return 'FAIL: expected 1 result, got ' + results.length;
		var res = results[0];
		if (res.preExisting !== true) return 'FAIL: expected preExisting=true, got ' + res.preExisting;
		if (!res.error || res.error.indexOf('pre-existing on feature') < 0) {
			return 'FAIL: error should mention pre-existing, got: ' + res.error;
		}
		if (!res.error || res.error.indexOf('worktree creation failed') < 0) {
			return 'FAIL: error should contain original error, got: ' + res.error;
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("fallback preExisting baseline failed: %v", raw)
	}
}

// TestChunk16_FallbackPoll_PreExisting_BaselinePassed verifies that
// handleVerifyFallbackPoll leaves preExisting:false when baseline passed.
func TestChunk16_FallbackPoll_PreExisting_BaselinePassed(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;
		s.verifyingIdx = 0;
		s.verifyOutput = {};
		s.verificationResults = [];
		s.verifyFallbackRunning = false;
		s.verifyFallbackError = 'process exited with code 1';

		// Baseline PASSED.
		s._baselineVerifyStarted = true;
		s._baselineVerifyResult = { failed: false, sourceBranch: 'feature' };

		var r = update({type: 'Tick', id: 'verify-fallback-poll'}, s);
		var results = r[0].verificationResults;
		if (results.length !== 1) return 'FAIL: expected 1 result, got ' + results.length;
		var res = results[0];
		if (res.preExisting !== false) return 'FAIL: expected preExisting=false, got ' + res.preExisting;
		if (res.error && res.error.indexOf('pre-existing') >= 0) {
			return 'FAIL: error should NOT mention pre-existing, got: ' + res.error;
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("fallback preExisting baseline passed: %v", raw)
	}
}

// TestChunk16_PreExisting_Helpers verifies the _isPreExistingFailure
// and _preExistingAnnotation helper functions directly.
func TestChunk16_PreExisting_Helpers(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Access helpers via the closure — they're private to the IIFE
		// but invoked via pollVerifySession. We test them indirectly
		// through the full poll path + directly via the exposed state.

		// Test 1: null baseline result → false.
		var s1 = { _baselineVerifyResult: null };
		setupPlanCache();
		var ss = initState('BRANCH_BUILDING');
		ss.isProcessing = true;
		ss.verifyingIdx = 0;
		ss.verifyOutput = {};
		ss.verificationResults = [];
		ss.outputLines = [];
		ss.outputAutoScroll = true;
		ss._baselineVerifyStarted = true;
		ss._baselineVerifyResult = null;
		ss.activeVerifySession = {
			isDone: function() { return true; },
			exitCode: function() { return 1; },
			output: function() { return 'fail'; },
			screen: function() { return 'fail'; },
			close: function() {},
			kill: function() {}
		};
		ss.activeVerifyWorktree = null;
		ss.activeVerifyDir = null;
		ss.activeVerifyBranch = 'split/api';
		ss.activeVerifyStartTime = Date.now();
		var r1 = update({type: 'Tick', id: 'verify-poll'}, ss);
		if (r1[0].verificationResults[0].preExisting !== false) {
			return 'FAIL: null baseline should give preExisting=false';
		}

		// Test 2: baseline failed → true + annotation.
		var ss2 = initState('BRANCH_BUILDING');
		ss2.isProcessing = true;
		ss2.verifyingIdx = 0;
		ss2.verifyOutput = {};
		ss2.verificationResults = [];
		ss2.outputLines = [];
		ss2.outputAutoScroll = true;
		ss2._baselineVerifyStarted = true;
		ss2._baselineVerifyResult = { failed: true, sourceBranch: 'my-feature' };
		ss2.activeVerifySession = {
			isDone: function() { return true; },
			exitCode: function() { return 2; },
			output: function() { return 'error'; },
			screen: function() { return 'error'; },
			close: function() {},
			kill: function() {}
		};
		ss2.activeVerifyWorktree = null;
		ss2.activeVerifyDir = null;
		ss2.activeVerifyBranch = 'split/cli';
		ss2.activeVerifyStartTime = Date.now();
		var r2 = update({type: 'Tick', id: 'verify-poll'}, ss2);
		var res2 = r2[0].verificationResults[0];
		if (res2.preExisting !== true) return 'FAIL: failed baseline should give preExisting=true';
		if (!res2.error || res2.error.indexOf('my-feature') < 0) {
			return 'FAIL: annotation should have sourceBranch name, got: ' + res2.error;
		}

		// Test 3: baseline with failed:false → false.
		var ss3 = initState('BRANCH_BUILDING');
		ss3.isProcessing = true;
		ss3.verifyingIdx = 0;
		ss3.verifyOutput = {};
		ss3.verificationResults = [];
		ss3.outputLines = [];
		ss3.outputAutoScroll = true;
		ss3._baselineVerifyStarted = true;
		ss3._baselineVerifyResult = { failed: false, sourceBranch: 'feature' };
		ss3.activeVerifySession = {
			isDone: function() { return true; },
			exitCode: function() { return 1; },
			output: function() { return 'error'; },
			screen: function() { return 'error'; },
			close: function() {},
			kill: function() {}
		};
		ss3.activeVerifyWorktree = null;
		ss3.activeVerifyDir = null;
		ss3.activeVerifyBranch = 'split/docs';
		ss3.activeVerifyStartTime = Date.now();
		var r3 = update({type: 'Tick', id: 'verify-poll'}, ss3);
		if (r3[0].verificationResults[0].preExisting !== false) {
			return 'FAIL: passed baseline should give preExisting=false';
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("preExisting helpers: %v", raw)
	}
}

// TestChunk16_RunVerifyBranch_KicksOffBaseline verifies that runVerifyBranch
// sets _baselineVerifyStarted = true on the first branch (verifyingIdx === 0)
// when sourceBranch is available.
func TestChunk16_RunVerifyBranch_KicksOffBaseline(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	// We need to mock verifySplitAsync so it doesn't actually try to run git.
	raw, err := evalJS(`(function() {
		setupPlanCache();

		// Mock verifySplitAsync to capture the call without real execution.
		var baselineCallBranch = null;
		var origAsync = globalThis.prSplit.verifySplitAsync;
		globalThis.prSplit.verifySplitAsync = function(branch, opts) {
			baselineCallBranch = branch;
			return Promise.resolve({ passed: false, output: '', error: 'mocked' });
		};

		// Also mock startVerifySession to avoid real PTY.
		var origStartSession = globalThis.prSplit.startVerifySession;
		globalThis.prSplit.startVerifySession = function() {
			return { skipped: true };
		};

		// Set runtime so runVerifyBranch can proceed.
		globalThis.prSplit.runtime = globalThis.prSplit.runtime || {};
		globalThis.prSplit.runtime.verifyCommand = 'make test';
		globalThis.prSplit.runtime.dir = '/tmp';

		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;
		s.verifyingIdx = 0;
		s.verifyOutput = {};
		s.verificationResults = [];

		try {
			var r = update({type: 'Tick', id: 'verify-branch'}, s);

			if (!r[0]._baselineVerifyStarted) {
				return 'FAIL: _baselineVerifyStarted should be true on idx 0';
			}
			if (baselineCallBranch !== 'feature') {
				return 'FAIL: baseline should check sourceBranch "feature", got: ' + baselineCallBranch;
			}
			return 'OK';
		} finally {
			globalThis.prSplit.verifySplitAsync = origAsync;
			globalThis.prSplit.startVerifySession = origStartSession;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("baseline kickoff: %v", raw)
	}
}

// TestChunk16_RunVerifyBranch_NoDoubleBaseline verifies that runVerifyBranch
// does NOT kick off a second baseline check when _baselineVerifyStarted
// is already true (e.g. on verifyingIdx > 0).
func TestChunk16_RunVerifyBranch_NoDoubleBaseline(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();

		var callCount = 0;
		var origAsync = globalThis.prSplit.verifySplitAsync;
		globalThis.prSplit.verifySplitAsync = function(branch, opts) {
			callCount++;
			return Promise.resolve({ passed: true, output: '', error: null });
		};

		var origStartSession = globalThis.prSplit.startVerifySession;
		globalThis.prSplit.startVerifySession = function() {
			return { skipped: true };
		};

		globalThis.prSplit.runtime = globalThis.prSplit.runtime || {};
		globalThis.prSplit.runtime.verifyCommand = 'make test';
		globalThis.prSplit.runtime.dir = '/tmp';

		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;
		s.verifyingIdx = 1;  // Second branch.
		s._baselineVerifyStarted = true;  // Already started.
		s._baselineVerifyResult = { failed: false, sourceBranch: 'feature' };
		s.verifyOutput = {};
		s.verificationResults = [];

		try {
			var r = update({type: 'Tick', id: 'verify-branch'}, s);

			if (callCount !== 0) {
				return 'FAIL: should not call verifySplitAsync again, called ' + callCount + ' times';
			}
			return 'OK';
		} finally {
			globalThis.prSplit.verifySplitAsync = origAsync;
			globalThis.prSplit.startVerifySession = origStartSession;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("no double baseline: %v", raw)
	}
}
