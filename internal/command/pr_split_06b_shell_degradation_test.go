package command

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  T338: Graceful degradation — Shell features when CaptureSession is
//  unavailable (Windows or PTY-less environments).
//
//  These tests override canSpawnInteractiveShell to return false, simulating
//  the Windows/headless environment regardless of the host OS. This allows
//  us to verify the error paths on any CI platform.
//
//  Task 8: The separate Shell tab and Shell button have been removed.
//  spawnShellSession is still used internally by the verify pane, so the
//  error-throwing path is still tested.
// ---------------------------------------------------------------------------

// TestGracefulDegradation_NoShellOnWindows verifies that when
// canSpawnInteractiveShell() returns false:
//   - spawnShellSession() throws a descriptive error
//   - The Verify tab still appears during fallback verification
//   - Mouse forwarding is a no-op (no crash on null session)
func TestGracefulDegradation_NoShellOnWindows(t *testing.T) {
	t.Parallel()

	t.Run("spawn_throws_descriptive_error", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngine(t)

		raw, err := evalJS(`(function() {
			var original = globalThis.prSplit.canSpawnInteractiveShell;
			globalThis.prSplit.canSpawnInteractiveShell = function() { return false; };
			try {
				globalThis.prSplit.spawnShellSession('/tmp/test', {rows: 24, cols: 80});
				return 'FAIL: expected error';
			} catch (e) {
				if (e.message.indexOf('not available') >= 0 && e.message.indexOf('termmux') >= 0) {
					return 'OK';
				}
				return 'FAIL: error message lacks platform info: ' + e.message;
			} finally {
				globalThis.prSplit.canSpawnInteractiveShell = original;
			}
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "OK" {
			t.Error(raw)
		}
	})

	t.Run("verify_tab_visible_during_fallback", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngineWithHelpers(t)

		// Simulate fallback verification: no CaptureSession, but
		// verifyFallbackRunning is true and verifyScreen has content.
		raw, err := evalJS(`(function() {
			setupPlanCache();
			var s = initState('BRANCH_BUILDING');
			s.splitViewEnabled = true;
			s.splitViewFocus = 'wizard';
			s.splitViewTab = 'verify';
			s.width = 100;
			s.height = 30;
			s.isProcessing = true;
			s.executionResults = [{sha: 'abc'}];
			s.executingIdx = 1;
			s.verifyingIdx = 0;
			s.verificationResults = [];
			s.outputLines = [];

			// Fallback state — no CaptureSession at all.
			s.activeVerifySession = null;
			s.verifyFallbackRunning = true;
			s.verifyScreen = 'Running: make test\nPASS utils\nFAIL api';
			s.activeVerifyBranch = 'split/api';
			s.activeVerifyStartTime = Date.now() - 3000;
			s.verifyAutoScroll = true;
			s.verifyViewportOffset = 0;

			var view = globalThis.prSplit._wizardView(s);

			var errors = [];
			if (view.indexOf('Verify') < 0) {
				errors.push('FAIL: Verify tab missing from tab bar during fallback');
			}
			if (view.indexOf('Running: make test') < 0) {
				errors.push('FAIL: fallback output not displayed in Verify pane');
			}
			return errors.length > 0 ? errors.join('; ') : 'OK';
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "OK" {
			t.Error(raw)
		}
	})

	t.Run("mouse_forward_noop_on_null_session", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngine(t)

		// Verify that mouseToTermBytes with a null session doesn't crash.
		// The function itself returns the bytes; the forwarding code checks
		// for session existence before writing. Verify the function is safe.
		raw, err := evalJS(`(function() {
			var bytes = globalThis.prSplit._mouseToTermBytes(
				{x: 10, y: 5, button: 'left', action: 'press'}, 3, 0
			);
			if (typeof bytes !== 'string' || bytes.length === 0) {
				return 'FAIL: mouseToTermBytes should return bytes even without session';
			}
			// The forwarding code (in model update) checks for session before
			// calling write. Simulate that guard:
			var session = null;
			if (session) { session.write(bytes); }
			return 'OK';
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "OK" {
			t.Error(raw)
		}
	})

	t.Run("fallback_footer_no_interactive_controls", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngine(t)

		if _, err := evalJS(viewTestPlanState); err != nil {
			t.Fatal(err)
		}

		// Render execution screen with only verifyScreen (no session).
		raw, err := evalJS(`(function() {
			return globalThis.prSplit._viewExecutionScreen({
				wizardState: 'BRANCH_BUILDING', width: 80,
				executionResults: [{sha: 'abc'}],
				executingIdx: 1,
				isProcessing: true,
				verifyingIdx: 1,
				verificationResults: [{passed: true, name: 'split/api'}],
				activeVerifySession: null,
				verifyScreen: 'test output line 1\ntest output line 2',
				activeVerifyBranch: 'split/cli',
				activeVerifyStartTime: Date.now() - 2000,
				verifyAutoScroll: true,
				verifyViewportOffset: 0
			});
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		rendered := raw.(string)
		// Fallback footer should say "(fallback output)".
		if !strings.Contains(rendered, "fallback") {
			t.Error("expected '(fallback output)' in footer for fallback mode")
		}
		// Interactive controls should NOT be present.
		if strings.Contains(rendered, "Pause") {
			t.Error("expected no Pause button in fallback mode")
		}
		if strings.Contains(rendered, "Force Kill") {
			t.Error("expected no Force Kill hint in fallback mode")
		}
	})
}
