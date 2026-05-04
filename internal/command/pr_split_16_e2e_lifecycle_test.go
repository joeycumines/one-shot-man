//go:build !windows

package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// TestE2E_VerifyTabLifecycle (T343) simulates the full verify tab lifecycle
// through the JS engine update loop using a mock CaptureSession.
//
// Phases:
//  1. Poll while running — verifyScreen captured from session.screen()
//  2. Send keyboard input — key forwarded to activeVerifySession.write()
//  3. Session completes — results populated, session cleaned up, idx advanced
func TestE2E_VerifyTabLifecycle(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
        var errors = [];
        setupPlanCache();
        var s = initState('BRANCH_BUILDING');
        s.isProcessing = true;
        s.verifyingIdx = 0;
        s.verificationResults = [];
        s.verifyOutput = {};
        s.outputLines = [];
        s.width = 80;
        s.height = 24;

        // ── Mock CaptureSession with phase toggle ──
        var done = false;
        s.activeVerifySession = {
            isDone: function() { return done; },
            exitCode: function() { return 0; },
            output: function() { return 'All tests passed'; },
            screen: function() { return done ? 'All tests passed' : 'Running tests...'; },
            write: function() {},
            close: function() {},
            kill: function() {},
            pause: function() {},
            resume: function() {}
        };
        s.activeVerifyWorktree = '/tmp/test-verify';
        s.activeVerifyDir = '/tmp/repo';
        s.activeVerifyBranch = 'split/01-types';
        s.activeVerifyStartTime = Date.now();
        s.verifyMode = 'oneshot';
        s.verifyElapsedMs = 0;
        s.verifyScreen = '';
        s.verifyViewportOffset = 0;
        s.verifyAutoScroll = true;

        // Auto-open split-view on verify tab.
        s.splitViewEnabled = true;
        s.splitViewTab = 'verify';
        s.splitViewFocus = 'wizard';

        // ── Phase 1: Poll while running — screen captured ──
        var r = update({type: 'Tick', id: 'verify-poll'}, s);
        s = r[0];
        if (!s.verifyScreen || s.verifyScreen.indexOf('Running') < 0) {
            errors.push('Phase1: verifyScreen should contain Running, got: ' + JSON.stringify(s.verifyScreen));
        }
        if (s.activeVerifySession === null) {
            errors.push('Phase1: session should still be active');
        }

        // ── Phase 2: Degraded one-shot verify is read-only; scroll keys move the viewport ──
        s.splitViewFocus = 'claude';  // focus bottom pane (verify tab)
        r = sendKey(s, 'up');
        s = r[0];
        if (s.wizardState !== 'BRANCH_BUILDING') {
            errors.push('Phase2: wizard state should be unchanged, got: ' + s.wizardState);
        }
        if (s.verifyViewportOffset !== 1) {
            errors.push('Phase2: verifyViewportOffset should scroll to 1, got: ' + s.verifyViewportOffset);
        }
        if (s.verifyAutoScroll !== false) {
            errors.push('Phase2: verifyAutoScroll should disable while scrolling');
        }

        // ── Phase 3: Session completes on next poll ──
        done = true;
        r = update({type: 'Tick', id: 'verify-poll'}, s);
        s = r[0];
        if (s.activeVerifySession !== null) {
            errors.push('Phase3: session should be null after completion');
        }
        if (!s.verificationResults || s.verificationResults.length < 1) {
            errors.push('Phase3: should have verification result, got: ' + (s.verificationResults ? s.verificationResults.length : 'null'));
        }
        if (s.verifyingIdx !== 1) {
            errors.push('Phase3: verifyingIdx should advance to 1, got: ' + s.verifyingIdx);
        }
        // T380: verify tab preserved after completion for post-mortem review.
        if (s.splitViewTab !== 'verify') {
            errors.push('Phase3: splitViewTab should stay on verify, got: ' + s.splitViewTab);
        }
        // T380: verifyScreen preserved for post-mortem viewing.
        if (!s.verifyScreen) {
            errors.push('Phase3: verifyScreen should be preserved after completion');
        }
        // Verify result contents.
        if (s.verificationResults && s.verificationResults.length > 0) {
            var result = s.verificationResults[0];
            if (!result.passed) {
                errors.push('Phase3: result should be passed');
            }
            if (result.name !== 'split/01-types') {
                errors.push('Phase3: result name should be split/01-types, got: ' + result.name);
            }
        }

        return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
    })()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("verify tab lifecycle: %v", raw)
	}
}

// TestE2E_VerifyFallbackLifecycle_T380 ensures the fallback (non-CaptureSession)
// verify path preserves verifyScreen, activeVerifyBranch, and verifyElapsedMs
// after completion for post-mortem viewing, matching the CaptureSession path.
func TestE2E_VerifyFallbackLifecycle_T380(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
        var errors = [];
        setupPlanCache();
        var s = initState('BRANCH_BUILDING');
        s.isProcessing = true;
        s.verifyingIdx = 0;
        s.verificationResults = [];
        s.verifyOutput = {'split/01-types': ['line1', 'line2']};
        s.outputLines = [];
        s.width = 80;
        s.height = 24;

        // Simulate fallback verify state (no CaptureSession).
        s.verifyFallbackRunning = false; // already finished
        s.verifyFallbackError = null;
        s.activeVerifyBranch = 'split/01-types';
        s.activeVerifyStartTime = Date.now() - 5000;
        s.verifyElapsedMs = 5000;
        s.verifyScreen = 'All tests passed (fallback)';
        s.verifyAutoScroll = true;
        s.verifyViewportOffset = 3;
        s.splitViewEnabled = true;
        s.splitViewTab = 'verify';
        s.splitViewFocus = 'claude';

        // Poll fallback — should complete since verifyFallbackRunning=false.
        var r = update({type: 'Tick', id: 'verify-fallback-poll'}, s);
        s = r[0];

        // T380: verifyScreen preserved for post-mortem.
        if (!s.verifyScreen || s.verifyScreen.indexOf('fallback') < 0) {
            errors.push('verifyScreen should be preserved, got: ' + JSON.stringify(s.verifyScreen));
        }
        // T380: activeVerifyBranch preserved for pane title.
        if (s.activeVerifyBranch !== 'split/01-types') {
            errors.push('activeVerifyBranch should be preserved, got: ' + s.activeVerifyBranch);
        }
        // T380: verifyElapsedMs preserved for elapsed display.
        if (s.verifyElapsedMs === 0) {
            errors.push('verifyElapsedMs should be preserved, got: ' + s.verifyElapsedMs);
        }
        // T380: splitViewTab stays on verify (not switched to output).
        if (s.splitViewTab !== 'verify') {
            errors.push('splitViewTab should stay on verify, got: ' + s.splitViewTab);
        }
        // Viewport offset reset for clean post-mortem view.
        if (s.verifyViewportOffset !== 0) {
            errors.push('verifyViewportOffset should be reset, got: ' + s.verifyViewportOffset);
        }

        return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
    })()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("verify fallback lifecycle T380: %v", raw)
	}
}

// Task 8: TestE2E_ShellTabLifecycle removed — shell tab unified into verify pane.
// The verify pane IS the interactive shell; there is no separate shell tab to test.

// Task 8: TestE2E_VerifyToShellAndBack removed — shell tab no longer exists.
// Verify pane provides the interactive shell; tab switching between verify and
// shell is no longer applicable.
