package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
// VTerm Integration Tests: Claude Pane Auto-Attach and Lifecycle
//
// These tests verify the complete lifecycle of the Claude pane:
//   auto-attach on Claude spawn → split-view opens → notification shown →
//   Ctrl+L dismiss → no re-open → child exit auto-close → cleanup
//
// Tests cover: auto-attach positive case, one-shot guard, dismiss prevents
// re-open, small terminal prevention, Claude badge rendering, notification
// set/auto-dismiss, auto-close on child exit, auto-close blocked during
// pipeline, crash detection closes split-view.
// ---------------------------------------------------------------------------

// lifecycleMuxSetup provides a standard mock with Claude active.
const lifecycleMuxSetup = `
var __savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
globalThis.tuiMux = {
	hasChild: function() { return true; },
	session: function() { return { isRunning: function() { return true; }, isDone: function() { return false; } }; },
	childScreen: function() { return 'claude output'; },
	screenshot: function() { return 'claude screenshot'; },
	lastActivityMs: function() { return 100; },
	writeToChild: function(bytes) {}
};
`

const lifecycleMuxRestore = `
if (__savedMux !== undefined) globalThis.tuiMux = __savedMux;
else delete globalThis.tuiMux;
`

// lifecycleExecutorSetup mocks claudeExecutor on prSplit._state so
// the auto-poll crash-detection check sees an alive process.
const lifecycleExecutorSetup = `
var __savedExecutor = globalThis.prSplit._state.claudeExecutor;
globalThis.prSplit._state.claudeExecutor = {
	handle: {
		isAlive: function() { return true; },
		receive: function() { return null; }
	},
	captureDiagnostic: function() { return ''; }
};
`

const lifecycleExecutorRestore = `
if (__savedExecutor !== undefined) {
	globalThis.prSplit._state.claudeExecutor = __savedExecutor;
} else {
	delete globalThis.prSplit._state.claudeExecutor;
}
`

// -- Auto-Attach Positive Case (all conditions met) -------------------------

func TestChunk16_VTerm_Lifecycle_AutoAttachFires(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + lifecycleMuxSetup + `
		` + lifecycleExecutorSetup + `
		try {
			var s = initState('PLAN_REVIEW');
			s.splitViewEnabled = false;
			s.claudeAutoAttached = false;
			s.claudeManuallyDismissed = false;
			s.autoSplitRunning = true;
			s.isProcessing = true;
			s.height = 24;  // >= 12 threshold

			// Fire auto-poll tick.
			var r = update({type: 'Tick', id: 'auto-poll'}, s);
			var ns = r[0];
			var cmd = r[1];
			var errors = [];

			// Split-view should now be enabled.
			if (ns.splitViewEnabled !== true) {
				errors.push('splitViewEnabled should be true');
			}
			// Focus stays on wizard.
			if (ns.splitViewFocus !== 'wizard') {
				errors.push('focus should stay on wizard, got: ' + ns.splitViewFocus);
			}
			// Tab should be claude.
			if (ns.splitViewTab !== 'claude') {
				errors.push('tab should be claude, got: ' + ns.splitViewTab);
			}
			// One-shot flag set.
			if (ns.claudeAutoAttached !== true) {
				errors.push('claudeAutoAttached should be true');
			}
			// Notification text set.
			if (!ns.claudeAutoAttachNotif || ns.claudeAutoAttachNotif.length === 0) {
				errors.push('notification text should be non-empty');
			}
			// Notification timestamp set.
			if (!ns.claudeAutoAttachNotifAt || ns.claudeAutoAttachNotifAt <= 0) {
				errors.push('notification timestamp should be positive');
			}
			// Command should be a batch (screenshot + auto-poll) — not null.
			if (cmd === null || cmd === undefined) {
				errors.push('should return batch command for screenshot + auto-poll');
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			` + lifecycleExecutorRestore + `
			` + lifecycleMuxRestore + `
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("auto-attach fires: %v", raw)
	}
}

// -- Auto-Attach One-Shot Guard (already attached → no re-trigger) ----------

func TestChunk16_VTerm_Lifecycle_AutoAttachOneShot(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + lifecycleMuxSetup + `
		` + lifecycleExecutorSetup + `
		try {
			var s = initState('PLAN_REVIEW');
			s.splitViewEnabled = false;  // disabled but was previously attached
			s.claudeAutoAttached = true; // one-shot already fired
			s.claudeManuallyDismissed = false;
			s.autoSplitRunning = true;
			s.isProcessing = true;
			s.height = 24;

			// Fire auto-poll tick — should NOT re-trigger.
			var r = update({type: 'Tick', id: 'auto-poll'}, s);
			var ns = r[0];
			var errors = [];

			// Split-view should remain disabled.
			if (ns.splitViewEnabled === true) {
				errors.push('should NOT re-attach when claudeAutoAttached is already true');
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			` + lifecycleExecutorRestore + `
			` + lifecycleMuxRestore + `
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("auto-attach one-shot guard: %v", raw)
	}
}

// -- Dismiss via Ctrl+L Prevents Auto Re-Open -------------------------------

func TestChunk16_VTerm_Lifecycle_DismissPreventsReOpen(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + lifecycleMuxSetup + `
		` + lifecycleExecutorSetup + `
		try {
			// Step 1: Auto-attach fires.
			var s = initState('PLAN_REVIEW');
			s.splitViewEnabled = false;
			s.claudeAutoAttached = false;
			s.claudeManuallyDismissed = false;
			s.autoSplitRunning = true;
			s.isProcessing = true;
			s.height = 24;

			var r = update({type: 'Tick', id: 'auto-poll'}, s);
			s = r[0];

			if (s.splitViewEnabled !== true) {
				return 'FAIL: step 1 auto-attach did not fire';
			}

			// Step 2: User dismisses with Ctrl+L.
			r = sendKey(s, 'ctrl+l');
			s = r[0];

			if (s.splitViewEnabled !== false) {
				return 'FAIL: step 2 ctrl+l did not disable split-view';
			}
			if (s.claudeManuallyDismissed !== true) {
				return 'FAIL: step 2 claudeManuallyDismissed should be true';
			}

			// Step 3: Reset one-shot flag to simulate another auto-poll check.
			// The claudeAutoAttached is already true, which also blocks.
			// But even if it weren't, claudeManuallyDismissed should block.
			s.claudeAutoAttached = false;  // force clear to test dismiss guard

			r = update({type: 'Tick', id: 'auto-poll'}, s);
			s = r[0];

			var errors = [];

			if (s.splitViewEnabled === true) {
				errors.push('split-view should NOT re-open after manual dismiss');
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			` + lifecycleExecutorRestore + `
			` + lifecycleMuxRestore + `
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("dismiss prevents re-open: %v", raw)
	}
}

// -- Small Terminal Prevention (height < 12) --------------------------------

func TestChunk16_VTerm_Lifecycle_SmallTerminalBlocks(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + lifecycleMuxSetup + `
		` + lifecycleExecutorSetup + `
		try {
			var heights = [6, 8, 10, 11];  // all below threshold of 12
			var errors = [];

			for (var hi = 0; hi < heights.length; hi++) {
				var s = initState('PLAN_REVIEW');
				s.splitViewEnabled = false;
				s.claudeAutoAttached = false;
				s.claudeManuallyDismissed = false;
				s.autoSplitRunning = true;
				s.isProcessing = true;
				s.height = heights[hi];

				var r = update({type: 'Tick', id: 'auto-poll'}, s);
				var ns = r[0];

				if (ns.splitViewEnabled === true) {
					errors.push('height=' + heights[hi] + ' should NOT auto-attach');
				}
			}

			// Height exactly 12 SHOULD work.
			var s12 = initState('PLAN_REVIEW');
			s12.splitViewEnabled = false;
			s12.claudeAutoAttached = false;
			s12.claudeManuallyDismissed = false;
			s12.autoSplitRunning = true;
			s12.isProcessing = true;
			s12.height = 12;

			var r12 = update({type: 'Tick', id: 'auto-poll'}, s12);
			if (r12[0].splitViewEnabled !== true) {
				errors.push('height=12 should auto-attach (boundary)');
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			` + lifecycleExecutorRestore + `
			` + lifecycleMuxRestore + `
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("small terminal blocks: %v", raw)
	}
}

// -- No Mux Prevents Auto-Attach -------------------------------------------

func TestChunk16_VTerm_Lifecycle_NoMuxNoAutoAttach(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Remove tuiMux entirely.
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		delete globalThis.tuiMux;
		try {
			var s = initState('PLAN_REVIEW');
			s.splitViewEnabled = false;
			s.claudeAutoAttached = false;
			s.claudeManuallyDismissed = false;
			s.autoSplitRunning = true;
			s.isProcessing = true;
			s.height = 24;

			var r = update({type: 'Tick', id: 'auto-poll'}, s);
			var ns = r[0];
			var errors = [];

			if (ns.splitViewEnabled === true) {
				errors.push('should NOT auto-attach when tuiMux is undefined');
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("no mux no auto-attach: %v", raw)
	}
}

// -- No Child Prevents Auto-Attach ------------------------------------------

func TestChunk16_VTerm_Lifecycle_NoChildNoAutoAttach(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return false; },
			session: function() { return { isRunning: function() { return false; }, isDone: function() { return false; } }; },
			childScreen: function() { return ''; },
			screenshot: function() { return ''; },
			lastActivityMs: function() { return -1; }
		};
		` + lifecycleExecutorSetup + `
		try {
			var s = initState('PLAN_REVIEW');
			s.splitViewEnabled = false;
			s.claudeAutoAttached = false;
			s.claudeManuallyDismissed = false;
			s.autoSplitRunning = true;
			s.isProcessing = true;
			s.height = 24;

			var r = update({type: 'Tick', id: 'auto-poll'}, s);
			var ns = r[0];
			var errors = [];

			if (ns.splitViewEnabled === true) {
				errors.push('should NOT auto-attach when hasChild() returns false');
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			` + lifecycleExecutorRestore + `
			if (savedMux !== undefined) globalThis.tuiMux = savedMux;
			else delete globalThis.tuiMux;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("no child no auto-attach: %v", raw)
	}
}

// -- Auto-Close on Child Exit (with notification) ---------------------------

func TestChunk16_VTerm_Lifecycle_AutoCloseOnChildExit(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return false; },  // child exited
			session: function() { return { isRunning: function() { return false; }, isDone: function() { return true; } }; },
			childScreen: function() { return ''; },
			screenshot: function() { return ''; },
			lastActivityMs: function() { return -1; },
			writeToChild: function() {}
		};
		try {
			var s = initState('PLAN_REVIEW');
			s.splitViewEnabled = true;        // was open
			s.claudeAutoAttached = true;      // was auto-attached
			s.autoSplitRunning = false;       // pipeline not running

			// claude-screenshot tick detects child gone.
			var r = update({type: 'Tick', id: 'claude-screenshot'}, s);
			var ns = r[0];
			var cmd = r[1];
			var errors = [];

			// Split-view auto-closed.
			if (ns.splitViewEnabled !== false) {
				errors.push('splitViewEnabled should be false after child exit');
			}
			// Focus reset to wizard.
			if (ns.splitViewFocus !== 'wizard') {
				errors.push('focus should reset to wizard');
			}
			// Notification text set.
			if (!ns.claudeAutoAttachNotif || ns.claudeAutoAttachNotif.indexOf('ended') === -1) {
				errors.push('notification should mention session ended, got: ' + ns.claudeAutoAttachNotif);
			}
			// Notification timestamp set.
			if (!ns.claudeAutoAttachNotifAt || ns.claudeAutoAttachNotifAt <= 0) {
				errors.push('notification timestamp should be positive');
			}
			// T028: pollClaudeScreenshot now returns a dismiss-attach-notif
			// tick to auto-dismiss the notification. Screenshot polling still
			// stops (no further claude-screenshot ticks are scheduled).

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			if (savedMux !== undefined) globalThis.tuiMux = savedMux;
			else delete globalThis.tuiMux;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("auto-close on child exit: %v", raw)
	}
}

// -- Auto-Close Blocked While Pipeline Running ------------------------------

func TestChunk16_VTerm_Lifecycle_AutoCloseBlockedDuringPipeline(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return false; },  // child exited
			session: function() { return { isRunning: function() { return false; }, isDone: function() { return true; } }; },
			childScreen: function() { return ''; },
			screenshot: function() { return ''; },
			lastActivityMs: function() { return -1; },
			writeToChild: function() {}
		};
		try {
			var s = initState('PLAN_REVIEW');
			s.splitViewEnabled = true;
			s.claudeAutoAttached = true;
			s.autoSplitRunning = true;  // pipeline still running!

			var r = update({type: 'Tick', id: 'claude-screenshot'}, s);
			var ns = r[0];
			var cmd = r[1];
			var errors = [];

			// Split-view should NOT close while pipeline is running
			// (child might re-attach).
			if (ns.splitViewEnabled !== true) {
				errors.push('should NOT auto-close while autoSplitRunning is true');
			}
			// Should continue polling.
			if (cmd === null || cmd === undefined) {
				errors.push('should continue polling, not stop');
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			if (savedMux !== undefined) globalThis.tuiMux = savedMux;
			else delete globalThis.tuiMux;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("auto-close blocked during pipeline: %v", raw)
	}
}

// -- Claude Badge in Status Bar Rendering -----------------------------------

func TestChunk16_VTerm_Lifecycle_ClaudeBadgeLive(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + lifecycleMuxSetup + `
		try {
			var s = initState('PLAN_REVIEW');
			s.width = 80;
			s.height = 24;

			var bar = globalThis.prSplit._renderStatusBar(s);
			var errors = [];

			// Should contain 'LIVE' badge (lastActivityMs = 100 < 2000).
			if (bar.indexOf('LIVE') === -1) {
				errors.push('status bar should contain LIVE badge, got: ' + bar.substring(0, 200));
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			` + lifecycleMuxRestore + `
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("claude badge live: %v", raw)
	}
}

func TestChunk16_VTerm_Lifecycle_ClaudeBadgeIdle(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			session: function() { return { isRunning: function() { return true; }, isDone: function() { return false; } }; },
			childScreen: function() { return 'output'; },
			screenshot: function() { return 'output'; },
			lastActivityMs: function() { return 5000; }  // 5s idle
		};
		try {
			var s = initState('PLAN_REVIEW');
			s.width = 80;
			s.height = 24;

			var bar = globalThis.prSplit._renderStatusBar(s);
			var errors = [];

			// 5000ms is between 2000–10000 → idle state.
			if (bar.indexOf('idle') === -1) {
				errors.push('status bar should contain idle indicator, got: ' + bar.substring(0, 200));
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			if (savedMux !== undefined) globalThis.tuiMux = savedMux;
			else delete globalThis.tuiMux;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("claude badge idle: %v", raw)
	}
}

func TestChunk16_VTerm_Lifecycle_ClaudeBadgeNA(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		delete globalThis.tuiMux;
		try {
			var s = initState('PLAN_REVIEW');
			s.width = 80;
			s.height = 24;

			var bar = globalThis.prSplit._renderStatusBar(s);
			var errors = [];

			// No mux → N/A badge.
			if (bar.indexOf('N/A') === -1) {
				errors.push('status bar should contain N/A badge when no mux, got: ' + bar.substring(0, 200));
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("claude badge N/A: %v", raw)
	}
}

func TestChunk16_VTerm_Lifecycle_ClaudeBadgeQuiet(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			session: function() { return { isRunning: function() { return true; }, isDone: function() { return false; } }; },
			childScreen: function() { return 'output'; },
			screenshot: function() { return 'output'; },
			lastActivityMs: function() { return 30000; }  // 30s quiet
		};
		try {
			var s = initState('PLAN_REVIEW');
			s.width = 80;
			s.height = 24;

			var bar = globalThis.prSplit._renderStatusBar(s);
			var errors = [];

			// 30000ms > 10000 → quiet state.
			if (bar.indexOf('quiet') === -1) {
				errors.push('status bar should contain quiet indicator, got: ' + bar.substring(0, 200));
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			if (savedMux !== undefined) globalThis.tuiMux = savedMux;
			else delete globalThis.tuiMux;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("claude badge quiet: %v", raw)
	}
}

func TestChunk16_VTerm_Lifecycle_ClaudeBadgeHiddenNarrow(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + lifecycleMuxSetup + `
		try {
			var s = initState('PLAN_REVIEW');
			s.width = 50;  // < 60 → narrow → badge hidden
			s.height = 24;

			var bar = globalThis.prSplit._renderStatusBar(s);
			var errors = [];

			// Narrow terminal hides the Claude badge.
			if (bar.indexOf('LIVE') !== -1 || bar.indexOf('N/A') !== -1 ||
				bar.indexOf('idle') !== -1 || bar.indexOf('quiet') !== -1) {
				errors.push('narrow terminal should hide Claude badge, got: ' + bar.substring(0, 200));
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			` + lifecycleMuxRestore + `
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("claude badge hidden narrow: %v", raw)
	}
}

// -- Notification Rendering in Status Bar -----------------------------------

func TestChunk16_VTerm_Lifecycle_NotificationRendered(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + lifecycleMuxSetup + `
		try {
			var s = initState('PLAN_REVIEW');
			s.width = 80;
			s.height = 24;
			s.claudeAutoAttachNotif = 'Claude connected';
			s.claudeAutoAttachNotifAt = Date.now();  // just set → < 5s

			var bar = globalThis.prSplit._renderStatusBar(s);
			var errors = [];

			// Notification should be rendered.
			if (bar.indexOf('Claude connected') === -1) {
				errors.push('notification text should appear in status bar, got: ' + bar.substring(0, 300));
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			` + lifecycleMuxRestore + `
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("notification rendered: %v", raw)
	}
}

func TestChunk16_VTerm_Lifecycle_NotificationAutoDismissed(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + lifecycleMuxSetup + `
		try {
			var s = initState('PLAN_REVIEW');
			s.width = 80;
			s.height = 24;
			s.claudeAutoAttachNotif = 'Claude connected';
			// Set timestamp to 6 seconds ago → expired.
			s.claudeAutoAttachNotifAt = Date.now() - 6000;

			var bar = globalThis.prSplit._renderStatusBar(s);
			var errors = [];

			// Notification should be auto-dismissed (not rendered).
			if (bar.indexOf('Claude connected') !== -1) {
				errors.push('expired notification should NOT appear in status bar');
			}
			// T028: State clearing is handled by dismiss-attach-notif tick,
			// not by the view function. Send the tick to verify the dismiss path.
			var dr = update({type: 'Tick', id: 'dismiss-attach-notif'}, s);
			s = dr[0];
			if (s.claudeAutoAttachNotif !== '') {
				errors.push('notification text should be cleared, got: ' + s.claudeAutoAttachNotif);
			}
			if (s.claudeAutoAttachNotifAt !== 0) {
				errors.push('notification timestamp should be cleared to 0');
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			` + lifecycleMuxRestore + `
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("notification auto-dismissed: %v", raw)
	}
}

// -- Auto-Attach sets Notification ------------------------------------------

func TestChunk16_VTerm_Lifecycle_AutoAttachNotificationText(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + lifecycleMuxSetup + `
		` + lifecycleExecutorSetup + `
		try {
			var s = initState('PLAN_REVIEW');
			s.splitViewEnabled = false;
			s.claudeAutoAttached = false;
			s.claudeManuallyDismissed = false;
			s.autoSplitRunning = true;
			s.isProcessing = true;
			s.height = 24;

			var r = update({type: 'Tick', id: 'auto-poll'}, s);
			var ns = r[0];
			var errors = [];

			// Notification should contain helpful keybinding info.
			if (!ns.claudeAutoAttachNotif) {
				errors.push('notification should be set on auto-attach');
			} else {
				if (ns.claudeAutoAttachNotif.indexOf('Ctrl+L') === -1) {
					errors.push('notification should mention Ctrl+L toggle');
				}
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			` + lifecycleExecutorRestore + `
			` + lifecycleMuxRestore + `
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("auto-attach notification text: %v", raw)
	}
}

// -- Crash Detection Closes Split-View --------------------------------------

func TestChunk16_VTerm_Lifecycle_CrashClosesPane(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + lifecycleMuxSetup + `
		// Executive with dead process.
		var savedExecutor = globalThis.prSplit._state.claudeExecutor;
		globalThis.prSplit._state.claudeExecutor = {
			handle: {
				isAlive: function() { return false; },
				receive: function() { return null; }
			},
			captureDiagnostic: function() { return 'panic: runtime error'; }
		};
		try {
			var s = initState('BRANCH_BUILDING');
			s.splitViewEnabled = true;
			s.splitViewFocus = 'claude';
			s.claudeScreen = 'last output';
			s.autoSplitRunning = true;
			s.isProcessing = true;
			s.height = 24;
			// Ensure health check runs immediately.
			s.lastClaudeHealthCheckMs = 0;

			var r = update({type: 'Tick', id: 'auto-poll'}, s);
			var ns = r[0];
			var errors = [];

			// Split-view should close.
			if (ns.splitViewEnabled !== false) {
				errors.push('crash should close split-view');
			}
			// Claude screen should be cleared.
			if (ns.claudeScreen !== '') {
				errors.push('claudeScreen should be cleared after crash');
			}
			// Focus reset to wizard.
			if (ns.splitViewFocus !== 'wizard') {
				errors.push('focus should reset to wizard');
			}
			// Crash detected flag set.
			if (ns.claudeCrashDetected !== true) {
				errors.push('claudeCrashDetected should be true');
			}
			// Error details should contain crash info.
			if (!ns.errorDetails || ns.errorDetails.indexOf('crashed') === -1) {
				errors.push('errorDetails should mention crash');
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			if (savedExecutor !== undefined) {
				globalThis.prSplit._state.claudeExecutor = savedExecutor;
			} else {
				delete globalThis.prSplit._state.claudeExecutor;
			}
			` + lifecycleMuxRestore + `
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("crash closes pane: %v", raw)
	}
}

// -- Ctrl+L Re-Open after Dismiss Clears Flag -------------------------------

func TestChunk16_VTerm_Lifecycle_CtrlLReOpenClearsDismissFlag(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = false;
		s.claudeManuallyDismissed = true;  // was dismissed

		// Ctrl+L toggles ON.
		var r = sendKey(s, 'ctrl+l');
		var ns = r[0];
		var errors = [];

		if (ns.splitViewEnabled !== true) {
			errors.push('ctrl+l should re-enable split-view');
		}
		// The dismiss flag should be cleared when user explicitly re-opens.
		if (ns.claudeManuallyDismissed !== false) {
			errors.push('claudeManuallyDismissed should be cleared on Ctrl+L re-open');
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("ctrl+l re-open clears dismiss flag: %v", raw)
	}
}

// -- Full Lifecycle: attach → use → dismiss → resume pipeline ---------------

func TestChunk16_VTerm_Lifecycle_FullFlow(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + lifecycleMuxSetup + `
		` + lifecycleExecutorSetup + `
		try {
			var errors = [];

			// Phase 1: Pipeline starts, auto-poll fires, auto-attach triggers.
			var s = initState('PLAN_REVIEW');
			s.splitViewEnabled = false;
			s.claudeAutoAttached = false;
			s.claudeManuallyDismissed = false;
			s.autoSplitRunning = true;
			s.isProcessing = true;
			s.height = 24;

			var r = update({type: 'Tick', id: 'auto-poll'}, s);
			s = r[0];

			if (!s.splitViewEnabled) errors.push('phase1: auto-attach should fire');
			if (!s.claudeAutoAttached) errors.push('phase1: one-shot flag not set');

			// Phase 2: User interacts with Claude pane.
			s.splitViewFocus = 'claude';
			s.claudeScreen = 'Claude is working...';

			// Phase 3: User dismisses via Ctrl+L.
			r = sendKey(s, 'ctrl+l');
			s = r[0];

			if (s.splitViewEnabled) errors.push('phase3: should be disabled after Ctrl+L');
			if (!s.claudeManuallyDismissed) errors.push('phase3: dismiss flag not set');

			// Phase 4: Auto-poll continues (pipeline still running).
			// Should NOT re-open.
			s.claudeAutoAttached = false;  // hypothetical re-trigger attempt
			r = update({type: 'Tick', id: 'auto-poll'}, s);
			s = r[0];

			if (s.splitViewEnabled) errors.push('phase4: should NOT re-open after dismiss');

			// Phase 5: User manually re-opens via Ctrl+L.
			r = sendKey(s, 'ctrl+l');
			s = r[0];

			if (!s.splitViewEnabled) errors.push('phase5: Ctrl+L should re-enable');
			if (s.claudeManuallyDismissed) errors.push('phase5: dismiss flag should clear');

			// Phase 6: Pipeline ends, child exits, auto-close fires.
			s.autoSplitRunning = false;
			s.claudeAutoAttached = true;
			globalThis.tuiMux.hasChild = function() { return false; };
			globalThis.tuiMux.session = function() { return { isRunning: function() { return false; }, isDone: function() { return true; } }; };

			r = update({type: 'Tick', id: 'claude-screenshot'}, s);
			s = r[0];

			if (s.splitViewEnabled) errors.push('phase6: should auto-close after child exit');
			if (!s.claudeAutoAttachNotif) errors.push('phase6: notification should be set');

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			` + lifecycleExecutorRestore + `
			` + lifecycleMuxRestore + `
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("full lifecycle flow: %v", raw)
	}
}

// -- Manually Opened Split-View Not Auto-Closed ----------------------------

func TestChunk16_VTerm_Lifecycle_ManualOpenNotAutoClosed(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return false; },
			session: function() { return { isRunning: function() { return false; }, isDone: function() { return true; } }; },
			childScreen: function() { return ''; },
			screenshot: function() { return ''; },
			lastActivityMs: function() { return -1; },
			writeToChild: function() {}
		};
		try {
			var s = initState('PLAN_REVIEW');
			s.splitViewEnabled = true;
			s.claudeAutoAttached = false;  // NOT auto-attached → manual open
			s.autoSplitRunning = false;

			var r = update({type: 'Tick', id: 'claude-screenshot'}, s);
			var ns = r[0];
			var errors = [];

			// Manually opened pane should NOT be auto-closed.
			// (Only auto-attached panes auto-close on child exit.)
			if (ns.splitViewEnabled !== true) {
				errors.push('manually opened pane should NOT be auto-closed on child exit');
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			if (savedMux !== undefined) globalThis.tuiMux = savedMux;
			else delete globalThis.tuiMux;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("manual open not auto-closed: %v", raw)
	}
}

// -- isProcessing=false Stops Auto-Poll ------------------------------------

func TestChunk16_VTerm_Lifecycle_StopPollingWhenNotProcessing(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + lifecycleMuxSetup + `
		try {
			var s = initState('PLAN_REVIEW');
			s.isProcessing = false;  // not processing

			var r = update({type: 'Tick', id: 'auto-poll'}, s);
			var ns = r[0];
			var cmd = r[1];
			var errors = [];

			// Should return null command (stop polling).
			if (cmd !== null && cmd !== undefined) {
				errors.push('should stop polling when isProcessing=false');
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			` + lifecycleMuxRestore + `
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("stop polling when not processing: %v", raw)
	}
}
