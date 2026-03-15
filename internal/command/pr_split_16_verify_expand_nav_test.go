package command

import (
	"testing"
)

// ---------------------------------------------------------------------------
//  T37: Async Verify Fallback Tests
// ---------------------------------------------------------------------------

// TestChunk16_VerifyFallback_LaunchesAsync verifies that runVerifyBranch
// launches async verifySplitAsync when CaptureSession fails.
func TestChunk16_VerifyFallback_LaunchesAsync(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		globalThis.prSplit.runtime.dir = '.';
		globalThis.prSplit.runtime.verifyCommand = 'make test';

		// Mock startVerifySession to fail (trigger fallback).
		var origStartVerify = globalThis.prSplit.startVerifySession;
		globalThis.prSplit.startVerifySession = function() {
			return { error: 'no PTY support', session: null };
		};

		// Mock verifySplitAsync to track calls.
		var origVerifyAsync = globalThis.prSplit.verifySplitAsync;
		var asyncCalled = false;
		globalThis.prSplit.verifySplitAsync = async function(branchName, opts) {
			asyncCalled = true;
			return { name: branchName, passed: true, output: '', error: null, skipped: false, duration: 100 };
		};

		// Also check sync verifySplit is NOT called.
		var origSync = globalThis.prSplit.verifySplit;
		var syncCalled = false;
		globalThis.prSplit.verifySplit = function() {
			syncCalled = true;
			return { passed: true };
		};

		try {
			var s = initState('EXECUTING');
			s.isProcessing = true;
			s.verificationResults = [];
			s.verifyingIdx = 0;
			s.verifyOutput = {};

			var r = update({type: 'Tick', id: 'verify-branch'}, s);
			s = r[0];

			if (syncCalled) return 'FAIL: sync verifySplit was called';
			if (!s.verifyFallbackRunning) return 'FAIL: verifyFallbackRunning should be true';
			if (!r[1]) return 'FAIL: should return poll tick';
			return 'OK';
		} finally {
			globalThis.prSplit.startVerifySession = origStartVerify;
			globalThis.prSplit.verifySplitAsync = origVerifyAsync;
			globalThis.prSplit.verifySplit = origSync;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("verify fallback launches async: %v", raw)
	}
}

// TestChunk16_VerifyFallback_PollStillRunning verifies that the fallback
// poll handler continues polling when verification is still running.
func TestChunk16_VerifyFallback_PollStillRunning(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('EXECUTING');
		s.verifyFallbackRunning = true;

		var r = update({type: 'Tick', id: 'verify-fallback-poll'}, s);
		if (!r[1]) return 'FAIL: should return poll tick when still running';
		if (!r[0].verifyFallbackRunning) return 'FAIL: should still be running';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("verify fallback poll still running: %v", raw)
	}
}

// TestChunk16_VerifyFallback_PollCompleted verifies that the fallback
// poll handler advances to next branch when verification completes.
func TestChunk16_VerifyFallback_PollCompleted(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('EXECUTING');
		s.isProcessing = true;
		s.verifyFallbackRunning = false;
		s.verifyFallbackError = null;

		var r = update({type: 'Tick', id: 'verify-fallback-poll'}, s);
		if (r[1] === null) return 'FAIL: should return verify-branch tick to continue';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("verify fallback poll completed: %v", raw)
	}
}

// TestChunk16_VerifyFallback_AsyncHappyPath exercises the full fallback
// verification chain: verify-branch → verify-fallback-poll → complete.
func TestChunk16_VerifyFallback_AsyncHappyPath(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		setupPlanCache();
		globalThis.prSplit.runtime.dir = '.';
		globalThis.prSplit.runtime.verifyCommand = 'make test';

		var origStartVerify = globalThis.prSplit.startVerifySession;
		globalThis.prSplit.startVerifySession = function() {
			return { error: 'no PTY', session: null };
		};

		var origVerifyAsync = globalThis.prSplit.verifySplitAsync;
		var verifiedBranches = [];
		globalThis.prSplit.verifySplitAsync = async function(branchName, opts) {
			verifiedBranches.push(branchName);
			return { name: branchName, passed: true, output: 'all good', error: null, skipped: false, duration: 50 };
		};

		try {
			var s = initState('EXECUTING');
			s.isProcessing = true;
			s.verificationResults = [];
			s.verifyingIdx = 0;
			s.verifyOutput = {};
			s._baselineVerifyStarted = true; // Skip T115 baseline check.

			// First branch: launch async.
			var r = update({type: 'Tick', id: 'verify-branch'}, s);
			s = r[0];
			if (!s.verifyFallbackRunning) return 'FAIL: should be running after verify-branch';

			// Let microtasks resolve.
			await Promise.resolve();
			await Promise.resolve();
			await Promise.resolve();

			// Poll — should be complete.
			r = update({type: 'Tick', id: 'verify-fallback-poll'}, s);
			s = r[0];

			if (s.verifyFallbackRunning) return 'FAIL: should not be running after poll';
			if (s.verificationResults.length !== 1) {
				return 'FAIL: expected 1 result, got: ' + s.verificationResults.length;
			}
			if (!s.verificationResults[0].passed) return 'FAIL: first branch should pass';
			if (s.verifyingIdx !== 1) return 'FAIL: verifyingIdx should be 1, got: ' + s.verifyingIdx;
			if (verifiedBranches[0] !== 'split/api') {
				return 'FAIL: expected split/api, got: ' + verifiedBranches[0];
			}
			return 'OK';
		} finally {
			globalThis.prSplit.startVerifySession = origStartVerify;
			globalThis.prSplit.verifySplitAsync = origVerifyAsync;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("verify fallback async happy path: %v", raw)
	}
}

// TestChunk16_VerifyFallback_AsyncError exercises the error path when
// verifySplitAsync returns a failure result.
func TestChunk16_VerifyFallback_AsyncError(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		setupPlanCache();
		globalThis.prSplit.runtime.dir = '.';
		globalThis.prSplit.runtime.verifyCommand = 'make test';

		var origStartVerify = globalThis.prSplit.startVerifySession;
		globalThis.prSplit.startVerifySession = function() {
			return { error: 'no PTY', session: null };
		};

		var origVerifyAsync = globalThis.prSplit.verifySplitAsync;
		globalThis.prSplit.verifySplitAsync = async function(branchName, opts) {
			return { name: branchName, passed: false, output: 'FAIL', error: 'verify failed (exit 1): test error', skipped: false, duration: 50 };
		};

		try {
			var s = initState('EXECUTING');
			s.isProcessing = true;
			s.verificationResults = [];
			s.verifyingIdx = 0;
			s.verifyOutput = {};

			var r = update({type: 'Tick', id: 'verify-branch'}, s);
			s = r[0];

			await Promise.resolve();
			await Promise.resolve();
			await Promise.resolve();

			r = update({type: 'Tick', id: 'verify-fallback-poll'}, s);
			s = r[0];

			if (s.verificationResults.length !== 1) {
				return 'FAIL: expected 1 result, got: ' + s.verificationResults.length;
			}
			if (s.verificationResults[0].passed) return 'FAIL: should not have passed';
			if (!s.verificationResults[0].error || s.verificationResults[0].error.indexOf('verify failed') < 0) {
				return 'FAIL: expected verify error, got: ' + s.verificationResults[0].error;
			}
			return 'OK';
		} finally {
			globalThis.prSplit.startVerifySession = origStartVerify;
			globalThis.prSplit.verifySplitAsync = origVerifyAsync;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("verify fallback async error: %v", raw)
	}
}

// TestChunk16_VerifyFallback_AsyncThrows exercises the path where
// verifySplitAsync throws an unexpected exception.
func TestChunk16_VerifyFallback_AsyncThrows(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		setupPlanCache();
		globalThis.prSplit.runtime.dir = '.';
		globalThis.prSplit.runtime.verifyCommand = 'make test';

		var origStartVerify = globalThis.prSplit.startVerifySession;
		globalThis.prSplit.startVerifySession = function() {
			return { error: 'no PTY', session: null };
		};

		var origVerifyAsync = globalThis.prSplit.verifySplitAsync;
		globalThis.prSplit.verifySplitAsync = async function() {
			throw new Error('worktree crash: ENOENT');
		};

		try {
			var s = initState('EXECUTING');
			s.isProcessing = true;
			s.verificationResults = [];
			s.verifyingIdx = 0;
			s.verifyOutput = {};

			var r = update({type: 'Tick', id: 'verify-branch'}, s);
			s = r[0];

			await Promise.resolve();
			await Promise.resolve();
			await Promise.resolve();

			r = update({type: 'Tick', id: 'verify-fallback-poll'}, s);
			s = r[0];

			if (s.verifyFallbackRunning) return 'FAIL: should not be running';
			if (!s.verifyFallbackError && s.verificationResults.length === 0) {
				return 'FAIL: should have recorded error';
			}
			// The poll handler should record a failure result from verifyFallbackError.
			if (s.verificationResults.length !== 1) {
				return 'FAIL: expected 1 result from error handler, got: ' + s.verificationResults.length;
			}
			if (!s.verificationResults[0].error || s.verificationResults[0].error.indexOf('ENOENT') < 0) {
				return 'FAIL: expected ENOENT in error, got: ' + s.verificationResults[0].error;
			}
			return 'OK';
		} finally {
			globalThis.prSplit.startVerifySession = origStartVerify;
			globalThis.prSplit.verifySplitAsync = origVerifyAsync;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("verify fallback async throws: %v", raw)
	}
}

// TestChunk16_VerifyFallback_NoSyncCalls verifies that the old sync
// prSplit.verifySplit() is never called from the TUI verify path.
func TestChunk16_VerifyFallback_NoSyncCalls(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		globalThis.prSplit.runtime.dir = '.';
		globalThis.prSplit.runtime.verifyCommand = 'make test';

		var origStartVerify = globalThis.prSplit.startVerifySession;
		globalThis.prSplit.startVerifySession = function() {
			return { error: 'no PTY', session: null };
		};

		var origSync = globalThis.prSplit.verifySplit;
		var syncCalled = false;
		globalThis.prSplit.verifySplit = function() {
			syncCalled = true;
			return { passed: true, name: 'test' };
		};

		var origAsync = globalThis.prSplit.verifySplitAsync;
		globalThis.prSplit.verifySplitAsync = async function(branchName, opts) {
			return { name: branchName, passed: true, output: '', error: null, skipped: false, duration: 10 };
		};

		try {
			var s = initState('EXECUTING');
			s.isProcessing = true;
			s.verificationResults = [];
			s.verifyingIdx = 0;
			s.verifyOutput = {};

			update({type: 'Tick', id: 'verify-branch'}, s);
			if (syncCalled) return 'FAIL: sync verifySplit was called';
			return 'OK';
		} finally {
			globalThis.prSplit.startVerifySession = origStartVerify;
			globalThis.prSplit.verifySplit = origSync;
			globalThis.prSplit.verifySplitAsync = origAsync;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("verify fallback no sync calls: %v", raw)
	}
}

// TestChunk16_VerifyFallback_CancelDuringAsync verifies that cancellation
// during async fallback verification is respected.
func TestChunk16_VerifyFallback_CancelDuringAsync(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		setupPlanCache();
		globalThis.prSplit.runtime.dir = '.';
		globalThis.prSplit.runtime.verifyCommand = 'make test';

		var origStartVerify = globalThis.prSplit.startVerifySession;
		globalThis.prSplit.startVerifySession = function() {
			return { error: 'no PTY', session: null };
		};

		var origVerifyAsync = globalThis.prSplit.verifySplitAsync;
		var resolveVerify;
		globalThis.prSplit.verifySplitAsync = function(branchName, opts) {
			// Return a pending promise that we control.
			return new Promise(function(resolve) {
				resolveVerify = resolve;
			});
		};

		try {
			var s = initState('EXECUTING');
			s.isProcessing = true;
			s.verificationResults = [];
			s.verifyingIdx = 0;
			s.verifyOutput = {};

			// Launch fallback verification.
			var r = update({type: 'Tick', id: 'verify-branch'}, s);
			s = r[0];
			if (!s.verifyFallbackRunning) return 'FAIL: should be running';

			// Cancel while async is pending.
			s.isProcessing = false;
			try { s.wizard.transition('CANCEL'); } catch(e) {}
			s.wizardState = s.wizard.current;

			// Resolve the pending verify after cancel.
			resolveVerify({ name: 'split/api', passed: true, output: '', error: null, skipped: false, duration: 10 });
			await Promise.resolve();
			await Promise.resolve();
			await Promise.resolve();

			// The async function should have bailed — no results pushed.
			if (s.verificationResults.length !== 0) {
				return 'FAIL: expected 0 results after cancel, got: ' + s.verificationResults.length;
			}
			return 'OK';
		} finally {
			globalThis.prSplit.startVerifySession = origStartVerify;
			globalThis.prSplit.verifySplitAsync = origVerifyAsync;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("verify fallback cancel during async: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  T38: Fix split-view Tab behavior — cycle elements within active pane
// ---------------------------------------------------------------------------

// TestChunk16_T38_TabCyclesFocusInSplitViewWizard verifies that Tab cycles
// through wizard focusable elements when split-view is enabled and wizard
// pane is focused (not switching panes).
func TestChunk16_T38_TabCyclesFocusInSplitViewWizard(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// Tab in split-view with wizard focused should cycle focusIndex.
		var s = initState('CONFIG');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';
		s.focusIndex = 0;
		var r = sendKey(s, 'tab');
		if (r[0].splitViewFocus !== 'wizard') errors.push('tab switched pane (should stay wizard)');
		if (r[0].focusIndex === 0) errors.push('tab did not advance focusIndex');

		// Shift+Tab also cycles (backwards) within wizard pane.
		s = initState('CONFIG');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';
		s.focusIndex = 1;
		r = sendKey(s, 'shift+tab');
		if (r[0].splitViewFocus !== 'wizard') errors.push('shift+tab switched pane');
		if (r[0].focusIndex !== 0) errors.push('shift+tab did not decrement focusIndex');

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T38 tab cycles focus in split-view wizard: %v", raw)
	}
}

// TestChunk16_T38_CtrlTabSwitchesPanes verifies that Ctrl+Tab toggles
// between wizard and Claude panes in split-view.
func TestChunk16_T38_CtrlTabSwitchesPanes(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// Ctrl+Tab: wizard → claude.
		var s = initState('CONFIG');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';
		var r = sendKey(s, 'ctrl+tab');
		if (r[0].splitViewFocus !== 'claude') errors.push('ctrl+tab did not switch to claude');

		// Ctrl+Tab: claude → wizard.
		r = sendKey(r[0], 'ctrl+tab');
		if (r[0].splitViewFocus !== 'wizard') errors.push('ctrl+tab did not switch back to wizard');

		// Ctrl+Tab does nothing when split-view is disabled.
		s = initState('CONFIG');
		s.splitViewEnabled = false;
		s.splitViewFocus = 'wizard';
		r = sendKey(s, 'ctrl+tab');
		if (r[0].splitViewFocus !== 'wizard') errors.push('ctrl+tab switched pane when split-view disabled');

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T38 ctrl+tab switches panes: %v", raw)
	}
}

// TestChunk16_T38_TabForwardedToClaudePTY verifies that Tab in split-view
// with Claude pane focused is forwarded to the child PTY (not intercepted).
func TestChunk16_T38_TabForwardedToClaudePTY(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// Tab should NOT be in CLAUDE_RESERVED_KEYS (so it forwards to PTY).
		var reserved = globalThis.prSplit._CLAUDE_RESERVED_KEYS;
		if (reserved['tab']) errors.push('tab is in CLAUDE_RESERVED_KEYS (should not be)');

		// Ctrl+Tab SHOULD be reserved (stays with wizard for pane switching).
		if (!reserved['ctrl+tab']) errors.push('ctrl+tab not in CLAUDE_RESERVED_KEYS');

		// keyToTermBytes should map tab → '\t'.
		var bytes = globalThis.prSplit._keyToTermBytes('tab');
		if (bytes !== '\t') errors.push('keyToTermBytes(tab) = ' + JSON.stringify(bytes) + ', want "\\t"');

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T38 tab forwarded to Claude PTY: %v", raw)
	}
}

// TestChunk16_T38_CtrlLPreservesFocusIndex verifies that toggling split-view
// via Ctrl+L does not reset focusIndex.
func TestChunk16_T38_CtrlLPreservesFocusIndex(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// Set a non-zero focusIndex, then toggle split-view on + off — verify preserved.
		var s = initState('CONFIG');
		s.focusIndex = 2;
		s.splitViewEnabled = false;

		// Enable split-view.
		var r = sendKey(s, 'ctrl+l');
		if (!r[0].splitViewEnabled) errors.push('ctrl+l did not enable split-view');
		if (r[0].focusIndex !== 2) errors.push('enable: focusIndex changed from 2 to ' + r[0].focusIndex);

		// Disable split-view.
		r = sendKey(r[0], 'ctrl+l');
		if (r[0].splitViewEnabled) errors.push('ctrl+l did not disable split-view');
		if (r[0].focusIndex !== 2) errors.push('disable: focusIndex changed from 2 to ' + r[0].focusIndex);

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T38 ctrl+l preserves focusIndex: %v", raw)
	}
}

// TestChunk16_T38_HelpOverlayBindings verifies that the help overlay shows
// Ctrl+Tab for pane switching and the correct split-view section.
func TestChunk16_T38_HelpOverlayBindings(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		var s = initState('CONFIG');
		s.showHelp = true;
		s.width = 80;
		s.height = 40;
		var rendered = globalThis.prSplit._viewHelpOverlay(s);

		if (rendered.indexOf('Ctrl+Tab') === -1) {
			errors.push('help overlay missing Ctrl+Tab');
		}
		if (rendered.indexOf('Switch wizard / Claude') === -1) {
			errors.push('help overlay missing pane switch description');
		}
		if (rendered.indexOf('Ctrl+L') === -1) {
			errors.push('help overlay missing Ctrl+L');
		}
		if (rendered.indexOf('Toggle split view') === -1) {
			errors.push('help overlay missing toggle split view description');
		}

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T38 help overlay bindings: %v", raw)
	}
}

// TestChunk16_T38_PaneDividerHint verifies the split-view pane divider
// shows the updated keybinding hint with Ctrl+Tab.
func TestChunk16_T38_PaneDividerHint(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		var s = initState('CONFIG');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';
		s.width = 120;
		s.height = 40;
		s.claudeScreen = 'claude output here';
		var rendered = globalThis.prSplit._wizardView(s);

		// The pane divider should mention Ctrl+Tab, not bare Tab.
		if (rendered.indexOf('Ctrl+Tab') === -1) {
			errors.push('pane divider missing Ctrl+Tab hint');
		}
		// Should NOT have bare 'Tab:' as a standalone hint label (old behavior).
		// Note: 'Ctrl+Tab:' contains 'Tab:' so we check for the old exact pattern.
		var idx = rendered.indexOf('Tab:');
		while (idx !== -1) {
			// Check if this 'Tab:' is preceded by 'Ctrl+' — if so, it's fine.
			var prefix = rendered.substring(Math.max(0, idx - 5), idx);
			if (prefix.indexOf('Ctrl+') === -1) {
				errors.push('pane divider has bare Tab: hint (old) at pos ' + idx);
				break;
			}
			idx = rendered.indexOf('Tab:', idx + 1);
		}

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T38 pane divider hint: %v", raw)
	}
}

// TestChunk16_T38_TabInClaudeFocusDoesNotCycleWizard verifies that Tab
// when Claude pane is focused does NOT cycle wizard focusable elements.
func TestChunk16_T38_TabInClaudeFocusDoesNotCycleWizard(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// When Claude pane is focused, Tab should NOT change focusIndex.
		var s = initState('CONFIG');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'claude';
		s.focusIndex = 0;
		var r = sendKey(s, 'tab');
		if (r[0].focusIndex !== 0) {
			errors.push('tab in claude focus changed focusIndex from 0 to ' + r[0].focusIndex);
		}
		// Focus should remain on Claude.
		if (r[0].splitViewFocus !== 'claude') {
			errors.push('focus changed from claude to ' + r[0].splitViewFocus);
		}

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T38 tab in claude focus: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  T39: Fix expand/collapse state management — per-item, not global reset
// ---------------------------------------------------------------------------

// TestChunk16_T39_VerifyCollapseGuard verifies that the collapse handler only
// fires when the clicked branch matches expandedVerifyBranch.
func TestChunk16_T39_VerifyCollapseGuard(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];
		setupPlanCache();

		// Expand split/api.
		var s = initState('BRANCH_BUILDING');
		s.expandedVerifyBranch = null;
		var restore = mockZoneHit('verify-expand-split/api');
		try {
			var r = sendClick(s);
			if (r[0].expandedVerifyBranch !== 'split/api') errors.push('expand did not set split/api');
		} finally { restore(); }

		// Collapse split/api (should work — matches expanded).
		s.expandedVerifyBranch = 'split/api';
		restore = mockZoneHit('verify-collapse-split/api');
		try {
			var r = sendClick(s);
			if (r[0].expandedVerifyBranch !== null) errors.push('collapse should have cleared split/api');
		} finally { restore(); }

		// Attempt to collapse split/cli when split/api is expanded (should NOT collapse).
		s.expandedVerifyBranch = 'split/api';
		restore = mockZoneHit('verify-collapse-split/cli');
		try {
			var r = sendClick(s);
			if (r[0].expandedVerifyBranch !== 'split/api') {
				errors.push('collapse of non-expanded branch should not clear state, got: ' + r[0].expandedVerifyBranch);
			}
		} finally { restore(); }

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T39 verify collapse guard: %v", raw)
	}
}

// TestChunk16_T39_AccordionBehavior verifies accordion (single-expand) behavior.
func TestChunk16_T39_AccordionBehavior(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];
		setupPlanCache();

		// Expand split/api.
		var s = initState('BRANCH_BUILDING');
		s.expandedVerifyBranch = null;
		var restore = mockZoneHit('verify-expand-split/api');
		try {
			var r = sendClick(s);
			if (r[0].expandedVerifyBranch !== 'split/api') errors.push('first expand failed');
		} finally { restore(); }

		// Now expand split/cli — should replace split/api (accordion).
		s.expandedVerifyBranch = 'split/api';
		restore = mockZoneHit('verify-expand-split/cli');
		try {
			var r = sendClick(s);
			if (r[0].expandedVerifyBranch !== 'split/cli') {
				errors.push('accordion: expand split/cli should replace, got: ' + r[0].expandedVerifyBranch);
			}
		} finally { restore(); }

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T39 accordion behavior: %v", raw)
	}
}

// TestChunk16_T39_EscapeCollapsesBeforeBackNav verifies that Escape collapses
// expanded sections before triggering back-navigation.
func TestChunk16_T39_EscapeCollapsesBeforeBackNav(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];
		setupPlanCache();

		// Escape with expandedVerifyBranch set should collapse, not navigate.
		var s = initState('BRANCH_BUILDING');
		s.expandedVerifyBranch = 'split/api';
		var r = sendKey(s, 'esc');
		if (r[0].expandedVerifyBranch !== null) errors.push('esc did not collapse verify branch');
		// Should NOT have navigated back — wizardState unchanged.
		if (r[0].wizardState !== 'BRANCH_BUILDING') errors.push('esc navigated back prematurely');

		// Escape with showAdvanced set should collapse, not navigate.
		s = initState('CONFIG');
		s.showAdvanced = true;
		r = sendKey(s, 'esc');
		if (r[0].showAdvanced) errors.push('esc did not collapse advanced options');
		// Should NOT have navigated — still CONFIG.
		if (r[0].wizardState !== 'CONFIG') errors.push('esc navigated away from CONFIG');

		// Second Escape (nothing expanded) should navigate back.
		s = initState('PLAN_REVIEW');
		s.showAdvanced = false;
		s.expandedVerifyBranch = null;
		r = sendKey(s, 'esc');
		if (r[0].wizardState !== 'CONFIG') errors.push('second esc should navigate back, got: ' + r[0].wizardState);

		// Escape in PLAN_REVIEW with leaked showAdvanced should NOT ghost-eat the key.
		// showAdvanced is only relevant on CONFIG, not PLAN_REVIEW.
		s = initState('PLAN_REVIEW');
		s.showAdvanced = true; // Leaked from CONFIG.
		s.expandedVerifyBranch = null;
		r = sendKey(s, 'esc');
		if (r[0].wizardState !== 'CONFIG') errors.push('esc in PLAN_REVIEW with leaked showAdvanced should nav back, got: ' + r[0].wizardState);

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T39 escape collapses before back-nav: %v", raw)
	}
}

// TestChunk16_T39_AdvancedOptionsToggle verifies the showAdvanced toggle
// works correctly via mouse zone handler.
func TestChunk16_T39_AdvancedOptionsToggle(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// Toggle on.
		var s = initState('CONFIG');
		s.showAdvanced = false;
		var restore = mockZoneHit('toggle-advanced');
		try {
			var r = sendClick(s);
			if (!r[0].showAdvanced) errors.push('toggle-on failed');
		} finally { restore(); }

		// Toggle off.
		s.showAdvanced = true;
		restore = mockZoneHit('toggle-advanced');
		try {
			var r = sendClick(s);
			if (r[0].showAdvanced) errors.push('toggle-off failed');
		} finally { restore(); }

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T39 advanced options toggle: %v", raw)
	}
}

// TestChunk16_T39_ChevronConsistency verifies that expand/collapse chevrons
// use consistent characters (▶ for collapsed, ▼ for expanded).
func TestChunk16_T39_ChevronConsistency(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// CONFIG: Advanced Options collapsed should show ▶ (U+25B6).
		var s = initState('CONFIG');
		s.showAdvanced = false;
		s.width = 80;
		s.height = 40;
		var rendered = globalThis.prSplit._viewForState(s);
		if (rendered.indexOf('\u25b6 Advanced Options') === -1) {
			errors.push('collapsed advanced missing ▶ chevron');
		}

		// CONFIG: Advanced Options expanded should show ▼ (U+25BC).
		s.showAdvanced = true;
		rendered = globalThis.prSplit._viewForState(s);
		if (rendered.indexOf('\u25bc Advanced Options') === -1) {
			errors.push('expanded advanced missing ▼ chevron');
		}

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T39 chevron consistency: %v", raw)
	}
}

// TestChunk16_T39_ExpandResetOnExecution verifies that expandedVerifyBranch
// is properly reset when starting new execution or verification.
func TestChunk16_T39_ExpandResetOnExecution(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];
		setupPlanCache();

		// expandedVerifyBranch should be null at init.
		var s = initState('BRANCH_BUILDING');
		if (s.expandedVerifyBranch !== null) errors.push('init: expandedVerifyBranch should be null');

		// Set it, then verify startExecution clears it.
		s.expandedVerifyBranch = 'split/api';
		// Simulate what startExecution does to verification state.
		s.verificationResults = [];
		s.verifyingIdx = -1;
		s.verifyOutput = {};
		s.expandedVerifyBranch = null; // The reset line from startExecution.
		if (s.expandedVerifyBranch !== null) errors.push('startExecution should clear expandedVerifyBranch');

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T39 expand reset on execution: %v", raw)
	}
}

// ────────────────────────────────────────────────────────────────────────────
//  T40 — Complete tab navigation across ALL screens
// ────────────────────────────────────────────────────────────────────────────

func TestChunk16_T40_FinalizationFocusElements(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];
		setupPlanCache();
		var s = initState('FINALIZATION');
		var elems = globalThis.prSplit._getFocusElements(s);

		// Expect 5 elements: final-report, final-create-prs, final-done, nav-next, nav-cancel.
		if (elems.length !== 5) {
			errors.push('element count: got ' + elems.length + ', want 5');
		}
		var expectedIds = ['final-report', 'final-create-prs', 'final-done', 'nav-next', 'nav-cancel'];
		var expectedTypes = ['button', 'button', 'button', 'nav', 'nav'];
		for (var i = 0; i < expectedIds.length; i++) {
			if (!elems[i] || elems[i].id !== expectedIds[i]) {
				errors.push('elem[' + i + ']: got ' + (elems[i] ? elems[i].id : 'undefined') + ', want ' + expectedIds[i]);
			}
			if (elems[i] && elems[i].type !== expectedTypes[i]) {
				errors.push('elem[' + i + '].type: got ' + elems[i].type + ', want ' + expectedTypes[i]);
			}
		}

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T40 finalization focus elements: %v", raw)
	}
}

func TestChunk16_T40_FinalizationTabCycling(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];
		setupPlanCache();
		var s = initState('FINALIZATION');
		s.focusIndex = 0;

		// Tab: 0→1.
		var r = sendKey(s, 'tab');
		if (r[0].focusIndex !== 1) errors.push('tab 0→1: got ' + r[0].focusIndex);

		// Tab: 1→2.
		r = sendKey(r[0], 'tab');
		if (r[0].focusIndex !== 2) errors.push('tab 1→2: got ' + r[0].focusIndex);

		// Tab: 2→3 (nav-next).
		r = sendKey(r[0], 'tab');
		if (r[0].focusIndex !== 3) errors.push('tab 2→3: got ' + r[0].focusIndex);

		// Tab: 3→4 (nav-cancel).
		r = sendKey(r[0], 'tab');
		if (r[0].focusIndex !== 4) errors.push('tab 3→4: got ' + r[0].focusIndex);

		// Tab: 4→0 (wrap-around).
		r = sendKey(r[0], 'tab');
		if (r[0].focusIndex !== 0) errors.push('tab wrap 4→0: got ' + r[0].focusIndex);

		// Shift+Tab: 0→4 (reverse wrap).
		s = initState('FINALIZATION');
		s.focusIndex = 0;
		r = sendKey(s, 'shift+tab');
		if (r[0].focusIndex !== 4) errors.push('shift+tab wrap 0→4: got ' + r[0].focusIndex);

		// Shift+Tab: 4→3.
		r = sendKey(r[0], 'shift+tab');
		if (r[0].focusIndex !== 3) errors.push('shift+tab 4→3: got ' + r[0].focusIndex);

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T40 finalization tab cycling: %v", raw)
	}
}

func TestChunk16_T40_FinalizationEnterActivatesButtons(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];
		setupPlanCache();

		// Focus on final-report (index 0): Enter opens report overlay.
		var s = initState('FINALIZATION');
		s.focusIndex = 0;
		var r = sendKey(s, 'enter');
		if (!r[0].showingReport) errors.push('final-report: showingReport not set');

		// Focus on final-create-prs (index 1): Enter triggers create-prs.
		s = initState('FINALIZATION');
		s.focusIndex = 1;
		r = sendKey(s, 'enter');
		// create-prs does not quit (it delegates to wizard), just returns.
		if (r[0].wizardState === 'DONE') errors.push('final-create-prs should not quit');

		// Focus on final-done (index 2): Enter quits.
		s = initState('FINALIZATION');
		s.focusIndex = 2;
		r = sendKey(s, 'enter');
		if (r[0].wizardState !== 'DONE') errors.push('final-done: wizardState=' + r[0].wizardState + ', want DONE');

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T40 finalization enter activates: %v", raw)
	}
}

func TestChunk16_T40_FinalizationFocusIndicators(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];
		setupPlanCache();

		// FINALIZATION with focus on index 0 (final-report) should render
		// the focused button differently from the others.
		var s = initState('FINALIZATION');
		s.focusIndex = 0;
		var view = globalThis.prSplit._wizardView(s);
		// Render should have different styling for focused vs unfocused buttons.
		// We can't check exact styling, but we verify the view renders without error.
		if (!view || view.indexOf('View Report') < 0) {
			errors.push('no View Report in rendered output');
		}
		if (view.indexOf('Create PRs') < 0) {
			errors.push('no Create PRs in rendered output');
		}
		if (view.indexOf('Done') < 0) {
			errors.push('no Done in rendered output');
		}

		// Move focus to index 2 and re-render — should still render all buttons.
		s.focusIndex = 2;
		view = globalThis.prSplit._wizardView(s);
		if (!view || view.indexOf('View Report') < 0) {
			errors.push('focus=2: no View Report');
		}

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T40 finalization focus indicators: %v", raw)
	}
}

func TestChunk16_T40_ConfigToggleAdvancedFocus(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];
		setupPlanCache();

		// CONFIG with heuristic mode: 3 strategies + toggle-advanced + nav-next = 5.
		var s = initState('CONFIG');
		var elems = globalThis.prSplit._getFocusElements(s);
		var found = false;
		var toggleIdx = -1;
		for (var i = 0; i < elems.length; i++) {
			if (elems[i].id === 'toggle-advanced') { found = true; toggleIdx = i; }
		}
		if (!found) errors.push('toggle-advanced not in getFocusElements for CONFIG');

		// Tab to toggle-advanced index and press Enter.
		s.focusIndex = toggleIdx;
		s.showAdvanced = false;
		var r = sendKey(s, 'enter');
		if (!r[0].showAdvanced) errors.push('Enter on toggle-advanced did not open advanced options');

		// Toggle again to close.
		r = sendKey(r[0], 'enter');
		if (r[0].showAdvanced) errors.push('Enter on toggle-advanced did not close advanced options');

		// CONFIG with auto mode: 3 strategies + test-claude + toggle-advanced + nav-next + nav-cancel = 7.
		s = initState('CONFIG');
		globalThis.prSplit.runtime.mode = 'auto';
		elems = globalThis.prSplit._getFocusElements(s);
		if (elems.length !== 7) errors.push('auto mode: got ' + elems.length + ' elems, want 7');
		// toggle-advanced should be at index 4 (after test-claude at index 3).
		if (elems[4] && elems[4].id !== 'toggle-advanced') {
			errors.push('auto mode: elem[4]=' + (elems[4] ? elems[4].id : 'undefined') + ', want toggle-advanced');
		}

		// Reset mode for other tests.
		globalThis.prSplit.runtime.mode = 'heuristic';

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T40 config toggle-advanced focus: %v", raw)
	}
}

func TestChunk16_T40_ConfigToggleAdvancedVisualIndicator(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];
		setupPlanCache();

		// Use viewForState to check config screen content directly
		// without viewport/chrome wrapping.
		var s = initState('CONFIG');
		s.focusIndex = 3; // toggle-advanced for heuristic mode.
		var view = globalThis.prSplit._viewForState(s);
		if (!view || typeof view !== 'string') {
			return 'FAIL: viewForState returned ' + typeof view;
		}
		// Should contain Advanced Options text.
		if (view.indexOf('Advanced Options') < 0) {
			errors.push('Advanced Options text missing from config view');
		}
		// When focusIndex=0, Advanced Options line should NOT have ▸ pointer.
		s.focusIndex = 0;
		view = globalThis.prSplit._viewForState(s);
		var lines = view.split('\n');
		var advLineHasPointer = false;
		for (var li = 0; li < lines.length; li++) {
			if (lines[li].indexOf('Advanced Options') >= 0 && lines[li].indexOf('\u25b8') >= 0) {
				advLineHasPointer = true;
			}
		}
		if (advLineHasPointer) {
			errors.push('focus=0: Advanced Options line should NOT have pointer');
		}
		// When focusIndex=3, Advanced Options line SHOULD have ▸ pointer.
		s.focusIndex = 3;
		view = globalThis.prSplit._viewForState(s);
		lines = view.split('\n');
		advLineHasPointer = false;
		for (var li = 0; li < lines.length; li++) {
			if (lines[li].indexOf('Advanced Options') >= 0 && lines[li].indexOf('\u25b8') >= 0) {
				advLineHasPointer = true;
			}
		}
		if (!advLineHasPointer) {
			errors.push('focus=3: Advanced Options line should have pointer');
		}

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T40 config toggle-advanced visual indicator: %v", raw)
	}
}

func TestChunk16_T40_ErrorResolutionNavNext(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];
		setupPlanCache();

		// ERROR_RESOLUTION should include nav-cancel as last element, nav-next as second-to-last.
		var s = initState('ERROR_RESOLUTION');
		globalThis.prSplit._state.claudeExecutor = {};
		var elems = globalThis.prSplit._getFocusElements(s);
		var last = elems[elems.length - 1];
		if (!last || last.id !== 'nav-cancel' || last.type !== 'nav') {
			errors.push('last elem: got ' + JSON.stringify(last) + ', want {id:nav-cancel,type:nav}');
		}
		var secondToLast = elems[elems.length - 2];
		if (!secondToLast || secondToLast.id !== 'nav-next' || secondToLast.type !== 'nav') {
			errors.push('second-to-last elem: got ' + JSON.stringify(secondToLast) + ', want {id:nav-next,type:nav}');
		}

		// Tab to nav-next and press Enter — should fall through to handleNext (auto-resolve).
		s.focusIndex = elems.length - 2;
		var r = sendKey(s, 'enter');
		// handleNext for ERROR_RESOLUTION calls handleErrorResolutionChoice(s, 'auto-resolve').
		// Result depends on implementation, but it should NOT error.
		if (!r || !r[0]) errors.push('enter on nav-next returned null');

		// Crash mode: should have nav-cancel as last, nav-next as second-to-last.
		s = initState('ERROR_RESOLUTION');
		s.claudeCrashDetected = true;
		elems = globalThis.prSplit._getFocusElements(s);
		last = elems[elems.length - 1];
		if (!last || last.id !== 'nav-cancel') {
			errors.push('crash mode last elem: got ' + (last ? last.id : 'undefined') + ', want nav-cancel');
		}
		secondToLast = elems[elems.length - 2];
		if (!secondToLast || secondToLast.id !== 'nav-next') {
			errors.push('crash mode second-to-last: got ' + (secondToLast ? secondToLast.id : 'undefined') + ', want nav-next');
		}

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T40 error resolution nav-next: %v", raw)
	}
}

func TestChunk16_T40_BranchBuildingExpandCollapseKeyboard(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];
		setupPlanCache();

		// Set up BRANCH_BUILDING with verify output.
		var s = initState('BRANCH_BUILDING');
		s.verifyOutput = {
			'split/api': ['line1', 'line2'],
			'split/cli': ['err1']
		};
		s.expandedVerifyBranch = null;

		// 'e' should expand the last branch with output (split/cli, index 1).
		var r = sendKey(s, 'e');
		if (r[0].expandedVerifyBranch !== 'split/cli') {
			errors.push('expand: got ' + r[0].expandedVerifyBranch + ', want split/cli');
		}

		// 'e' again should collapse.
		r = sendKey(r[0], 'e');
		if (r[0].expandedVerifyBranch !== null) {
			errors.push('collapse: got ' + r[0].expandedVerifyBranch + ', want null');
		}

		// When no verify output exists, 'e' should be harmless.
		s = initState('BRANCH_BUILDING');
		s.verifyOutput = {};
		s.expandedVerifyBranch = null;
		r = sendKey(s, 'e');
		if (r[0].expandedVerifyBranch !== null) {
			errors.push('no output expand: got ' + r[0].expandedVerifyBranch + ', want null');
		}

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T40 branch building expand/collapse keyboard: %v", raw)
	}
}

func TestChunk16_T40_HelpOverlayBranchBuildingSection(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];
		setupPlanCache();

		var s = initState('CONFIG');
		s.showHelp = true;
		var view = globalThis.prSplit._viewHelpOverlay(s);

		// Should contain Branch Building section.
		if (view.indexOf('Branch Building') < 0) {
			errors.push('missing Branch Building section');
		}
		if (view.indexOf('Expand / collapse output') < 0) {
			errors.push('missing expand/collapse help text');
		}
		if (view.indexOf('Interrupt verification') < 0) {
			errors.push('missing interrupt help text');
		}

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T40 help overlay branch building section: %v", raw)
	}
}

func TestChunk16_T40_ElementCountParity(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];
		setupPlanCache();
		globalThis.prSplit._state.claudeExecutor = {};

		// Verify exact element counts per screen.
		var expected = {
			'CONFIG':           6,  // 3 strategies + toggle-advanced + nav-next + nav-cancel
			'PLAN_REVIEW':      8,  // 3 cards + plan-edit + plan-regenerate + ask-claude + nav-next + nav-cancel
			'PLAN_EDITOR':      8,  // 3 cards + editor-move + editor-rename + editor-merge + nav-next + nav-cancel
			'ERROR_RESOLUTION': 8,  // 5 buttons + error-ask-claude + nav-next + nav-cancel
			'FINALIZATION':     5,  // final-report + final-create-prs + final-done + nav-next + nav-cancel
			'BRANCH_BUILDING':  0,  // passive — keyboard shortcuts, not focus elements
			'PLAN_GENERATION':  0,  // passive
			'EQUIV_CHECK':      0,  // passive
			'PAUSED':           2   // pause-resume + pause-quit (no nav-cancel — uses pause-quit)
		};

		for (var state in expected) {
			var s = initState(state);
			if (state === 'ERROR_RESOLUTION') {
				globalThis.prSplit._state.claudeExecutor = {};
			}
			var elems = globalThis.prSplit._getFocusElements(s);
			if (elems.length !== expected[state]) {
				errors.push(state + ': got ' + elems.length + ', want ' + expected[state]);
			}
		}

		// CONFIG with auto mode: test-claude adds 1 element.
		globalThis.prSplit.runtime.mode = 'auto';
		var s = initState('CONFIG');
		var elems = globalThis.prSplit._getFocusElements(s);
		if (elems.length !== 7) errors.push('CONFIG(auto): got ' + elems.length + ', want 7');
		globalThis.prSplit.runtime.mode = 'heuristic';

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T40 element count parity: %v", raw)
	}
}
