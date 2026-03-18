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
        var writtenKeys = [];
        s.activeVerifySession = {
            isDone: function() { return done; },
            exitCode: function() { return 0; },
            output: function() { return 'All tests passed'; },
            screen: function() { return done ? 'All tests passed' : 'Running tests...'; },
            write: function(v) { writtenKeys.push(v); },
            close: function() {},
            kill: function() {},
            pause: function() {},
            resume: function() {}
        };
        s.activeVerifyWorktree = '/tmp/test-verify';
        s.activeVerifyDir = '/tmp/repo';
        s.activeVerifyBranch = 'split/01-types';
        s.activeVerifyStartTime = Date.now();
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

        // ── Phase 2: Send key while bottom pane focused on verify tab ──
        s.splitViewFocus = 'claude';  // focus bottom pane (verify tab)
        r = sendKey(s, 'a');
        s = r[0];
        if (s.wizardState !== 'BRANCH_BUILDING') {
            errors.push('Phase2: wizard state should be unchanged, got: ' + s.wizardState);
        }
        if (writtenKeys.length === 0) {
            errors.push('Phase2: key should have been written to verify session');
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
        // T325: verify tab switches to output on completion.
        if (s.splitViewTab !== 'output') {
            errors.push('Phase3: splitViewTab should switch to output, got: ' + s.splitViewTab);
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

// TestE2E_ShellTabLifecycle (T344) simulates the shell tab lifecycle:
// opening a shell in the verify worktree, forwarding input, and cleanup on exit.
//
// When the shell exits, pollShellSession sets shellSession=null and switches
// the split-view tab back to 'verify' (if a verify session is active).
func TestE2E_ShellTabLifecycle(t *testing.T) {
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

        // ── Active verify session (running) ──
        s.activeVerifySession = {
            isDone: function() { return false; },
            exitCode: function() { return -1; },
            output: function() { return ''; },
            screen: function() { return 'Verify running...'; },
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
        s.verifyElapsedMs = 0;
        s.verifyScreen = '';
        s.verifyViewportOffset = 0;
        s.verifyAutoScroll = true;
        s.verifyPaused = false;

        // ── Mock shell CaptureSession ──
        var shellDone = false;
        var shellWritten = [];
        s.shellSession = {
            isDone: function() { return shellDone; },
            exitCode: function() { return 0; },
            output: function() { return '$ ls\nfile1.go  file2.go'; },
            screen: function() { return '$ ls\nfile1.go  file2.go'; },
            write: function(v) { shellWritten.push(v); },
            close: function() {},
            kill: function() {},
            pause: function() {},
            resume: function() {}
        };
        s.shellScreen = '';
        s.shellViewOffset = 0;
        s.shellAutoScroll = true;

        // Split-view with shell tab active.
        s.splitViewEnabled = true;
        s.splitViewTab = 'shell';
        s.splitViewFocus = 'claude';  // focus bottom pane (shell tab)

        // ── Step 1: Verify tab is 'shell' ──
        if (s.splitViewTab !== 'shell') {
            errors.push('Step1: splitViewTab should be shell, got: ' + s.splitViewTab);
        }

        // ── Step 2: Send key to shell tab ──
        var r = sendKey(s, 'p');
        s = r[0];
        if (s.wizardState !== 'BRANCH_BUILDING') {
            errors.push('Step2: wizard state should be unchanged, got: ' + s.wizardState);
        }
        if (shellWritten.length === 0) {
            errors.push('Step2: key should have been written to shell session');
        }

        // ── Step 3: Poll while shell is running ──
        r = update({type: 'Tick', id: 'shell-poll'}, s);
        s = r[0];
        if (s.shellSession === null) {
            errors.push('Step3: shell session should still be active');
        }
        if (!s.shellScreen || s.shellScreen.length === 0) {
            errors.push('Step3: shellScreen should be captured');
        }

        // ── Step 4: Shell exits ──
        shellDone = true;
        r = update({type: 'Tick', id: 'shell-poll'}, s);
        s = r[0];
        if (s.shellSession !== null) {
            errors.push('Step4: shellSession should be null after exit');
        }
        // Tab should auto-switch back to verify (active verify session exists).
        if (s.splitViewTab !== 'verify') {
            errors.push('Step4: splitViewTab should switch to verify, got: ' + s.splitViewTab);
        }
        if (s.shellScreen !== '') {
            errors.push('Step4: shellScreen should be cleared, got: ' + JSON.stringify(s.shellScreen));
        }

        return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
    })()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("shell tab lifecycle: %v", raw)
	}
}

// TestE2E_VerifyToShellAndBack exercises the combined lifecycle:
// verify starts → shell opens → shell closes → verify resumes → verify completes.
//
// This validates that shell and verify sessions can coexist, that the shell tab
// cleanly yields back to verify on exit, and that verify completion properly
// records results after the interleaved shell session.
func TestE2E_VerifyToShellAndBack(t *testing.T) {
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

        // ── Phase 1: Mock verify CaptureSession (running) ──
        var verifyDone = false;
        s.activeVerifySession = {
            isDone: function() { return verifyDone; },
            exitCode: function() { return 0; },
            output: function() { return 'All tests passed\nOK'; },
            screen: function() { return verifyDone ? 'All tests passed' : 'Running verify...'; },
            write: function() {},
            close: function() {},
            kill: function() {},
            pause: function() {},
            resume: function() {}
        };
        s.activeVerifyWorktree = '/tmp/test-verify';
        s.activeVerifyDir = '/tmp/repo';
        s.activeVerifyBranch = 'split/02-api';
        s.activeVerifyStartTime = Date.now();
        s.verifyElapsedMs = 0;
        s.verifyScreen = '';
        s.verifyViewportOffset = 0;
        s.verifyAutoScroll = true;
        s.verifyPaused = false;

        // Auto-open split-view on verify tab.
        s.splitViewEnabled = true;
        s.splitViewTab = 'verify';
        s.splitViewFocus = 'claude';

        // ── Phase 2: Poll verify while running ──
        var r = update({type: 'Tick', id: 'verify-poll'}, s);
        s = r[0];
        if (!s.verifyScreen || s.verifyScreen.indexOf('Running') < 0) {
            errors.push('Phase2: verifyScreen should show running state');
        }
        if (s.activeVerifySession === null) {
            errors.push('Phase2: verify session should be active');
        }

        // ── Phase 3: Open shell (simulate spawnShellSession by setting state) ──
        var shellDone = false;
        s.shellSession = {
            isDone: function() { return shellDone; },
            exitCode: function() { return 0; },
            output: function() { return '$ make test'; },
            screen: function() { return '$ make test'; },
            write: function() {},
            close: function() {},
            kill: function() {},
            pause: function() {},
            resume: function() {}
        };
        s.shellScreen = '';
        s.shellViewOffset = 0;
        s.shellAutoScroll = true;
        s.splitViewTab = 'shell';

        // Verify that the verify session is still set (paused conceptually).
        if (s.activeVerifySession === null) {
            errors.push('Phase3: verify session should still exist while shell is open');
        }
        if (s.splitViewTab !== 'shell') {
            errors.push('Phase3: tab should be shell');
        }

        // ── Phase 4: Shell exits → tab returns to verify ──
        shellDone = true;
        r = update({type: 'Tick', id: 'shell-poll'}, s);
        s = r[0];
        if (s.shellSession !== null) {
            errors.push('Phase4: shellSession should be null after exit');
        }
        if (s.splitViewTab !== 'verify') {
            errors.push('Phase4: tab should auto-switch to verify, got: ' + s.splitViewTab);
        }
        // Verify session should still be running.
        if (s.activeVerifySession === null) {
            errors.push('Phase4: verify session should still be active after shell exit');
        }

        // ── Phase 5: Verify completes ──
        verifyDone = true;
        r = update({type: 'Tick', id: 'verify-poll'}, s);
        s = r[0];
        if (s.activeVerifySession !== null) {
            errors.push('Phase5: verify session should be null after completion');
        }
        if (!s.verificationResults || s.verificationResults.length < 1) {
            errors.push('Phase5: should have verification result');
        }
        if (s.verifyingIdx !== 1) {
            errors.push('Phase5: verifyingIdx should advance to 1, got: ' + s.verifyingIdx);
        }
        // Verify result is correct.
        if (s.verificationResults && s.verificationResults.length > 0) {
            var result = s.verificationResults[0];
            if (!result.passed) {
                errors.push('Phase5: result should be passed');
            }
            if (result.name !== 'split/02-api') {
                errors.push('Phase5: result name mismatch, got: ' + result.name);
            }
            if (!result.output || result.output.indexOf('All tests passed') < 0) {
                errors.push('Phase5: result output should contain test output');
            }
        }

        return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
    })()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("verify to shell and back: %v", raw)
	}
}
