package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  Live Verify Session
// ---------------------------------------------------------------------------

func TestChunk16_VerifySession_Interrupt(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var interrupted = false, killed = false;
		var mockSession = {
			interrupt: function() { interrupted = true; },
			kill: function() { killed = true; },
			close: function() {},
			isRunning: function() { return true; },
			output: function() { return ''; },
			screen: function() { return ''; }
		};

		var s = initState('BRANCH_BUILDING');
		s.activeVerifySession = mockSession;
		s.lastVerifyInterruptTime = 0;

		// First Ctrl+C — graceful interrupt.
		var r = sendKey(s, 'ctrl+c');
		if (!interrupted) return 'FAIL: first ctrl+c did not interrupt';
		if (killed) return 'FAIL: first ctrl+c should not kill';
		if (r[0].lastVerifyInterruptTime <= 0) return 'FAIL: interrupt time not set';
		if (r[0].showConfirmCancel) return 'FAIL: ctrl+c during verify should not show cancel dialog';

		// Second Ctrl+C within 2s — force kill.
		interrupted = false;
		r = sendKey(r[0], 'ctrl+c');
		if (!killed) return 'FAIL: second ctrl+c did not kill';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("verify session interrupt: %v", raw)
	}
}

func TestChunk16_VerifySession_ScrollViewport(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var mockSession = {
			interrupt: function() {}, kill: function() {}, close: function() {},
			isRunning: function() { return true; },
			output: function() { return ''; }, screen: function() { return ''; }
		};

		var s = initState('BRANCH_BUILDING');
		s.activeVerifySession = mockSession;
		s.verifyAutoScroll = true;
		s.verifyViewportOffset = 0;

		// up scrolls up, disables auto-scroll.
		var r = sendKey(s, 'up');
		if (r[0].verifyViewportOffset !== 1) return 'FAIL: up did not scroll, got ' + r[0].verifyViewportOffset;
		if (r[0].verifyAutoScroll) return 'FAIL: up did not disable auto-scroll';

		// k also scrolls up.
		r = sendKey(r[0], 'k');
		if (r[0].verifyViewportOffset !== 2) return 'FAIL: k did not scroll';

		// down scrolls down.
		r = sendKey(r[0], 'down');
		if (r[0].verifyViewportOffset !== 1) return 'FAIL: down did not scroll back';

		// j also scrolls down.
		r = sendKey(r[0], 'j');
		if (r[0].verifyViewportOffset !== 0) return 'FAIL: j did not scroll to 0';
		if (!r[0].verifyAutoScroll) return 'FAIL: scroll to 0 did not re-enable auto-scroll';

		// home jumps far back.
		r = sendKey(r[0], 'home');
		if (r[0].verifyViewportOffset !== 999999) return 'FAIL: home did not jump';
		if (r[0].verifyAutoScroll) return 'FAIL: home should disable auto-scroll';

		// end jumps to bottom.
		r = sendKey(r[0], 'end');
		if (r[0].verifyViewportOffset !== 0) return 'FAIL: end did not go to bottom';
		if (!r[0].verifyAutoScroll) return 'FAIL: end should enable auto-scroll';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("verify session scroll: %v", raw)
	}
}

func TestChunk16_VerifySession_MouseWheel(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var mockSession = {
			interrupt: function() {}, kill: function() {}, close: function() {},
			isRunning: function() { return true; },
			output: function() { return ''; }, screen: function() { return ''; }
		};

		var s = initState('BRANCH_BUILDING');
		s.activeVerifySession = mockSession;
		s.verifyAutoScroll = true;
		s.verifyViewportOffset = 0;

		// Wheel up scrolls verify output.
		var r = sendWheel(s, 'up');
		if (r[0].verifyViewportOffset !== 3) return 'FAIL: wheel-up offset=' + r[0].verifyViewportOffset + ', want 3';
		if (r[0].verifyAutoScroll) return 'FAIL: wheel-up should disable auto-scroll';

		// Wheel down scrolls back.
		r = sendWheel(r[0], 'down');
		if (r[0].verifyViewportOffset !== 0) return 'FAIL: wheel-down offset=' + r[0].verifyViewportOffset + ', want 0';
		if (!r[0].verifyAutoScroll) return 'FAIL: wheel-down to 0 should re-enable auto-scroll';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("verify session mouse wheel: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  Split View
// ---------------------------------------------------------------------------

func TestChunk16_SplitView_Toggle(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');

		// Ctrl+L enables split view.
		var r = sendKey(s, 'ctrl+l');
		if (!r[0].splitViewEnabled) return 'FAIL: ctrl+l did not enable split view';
		// Should return a tick command for screenshot polling.
		if (!r[1]) return 'FAIL: ctrl+l should return tick command';

		// Ctrl+L again disables.
		r = sendKey(r[0], 'ctrl+l');
		if (r[0].splitViewEnabled) return 'FAIL: ctrl+l did not disable split view';
		if (r[0].claudeScreenshot !== '') return 'FAIL: screenshot not cleared';
		if (r[0].claudeViewOffset !== 0) return 'FAIL: claude offset not reset';
		if (r[0].splitViewFocus !== 'wizard') return 'FAIL: focus not reset to wizard';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("split view toggle: %v", raw)
	}
}

func TestChunk16_SplitView_TabFocusSwitch(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';

		// Ctrl+Tab switches to Claude pane.
		var r = sendKey(s, 'ctrl+tab');
		if (r[0].splitViewFocus !== 'claude') return 'FAIL: ctrl+tab did not switch to claude';

		// Ctrl+Tab switches back to wizard.
		r = sendKey(r[0], 'ctrl+tab');
		if (r[0].splitViewFocus !== 'wizard') return 'FAIL: ctrl+tab did not switch to wizard';

		// Ctrl+Tab during active verify session: T380 removed the guard, so it now switches focus.
		r[0].activeVerifySession = {interrupt:function(){},kill:function(){},close:function(){},isRunning:function(){return true;},output:function(){return '';},screen:function(){return '';}};
		r[0].splitViewFocus = 'wizard';
		r = sendKey(r[0], 'ctrl+tab');
		// T380: Ctrl+Tab now works during verify — switches to pane.
		if (r[0].splitViewFocus !== 'claude') return 'FAIL: ctrl+tab during verify should switch to claude';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("split view ctrl+tab: %v", raw)
	}
}

func TestChunk16_SplitView_RatioAdjust(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.splitViewEnabled = true;
		s.splitViewRatio = 0.6;

		// Ctrl+= increases ratio.
		var r = sendKey(s, 'ctrl+=');
		var ratio = Math.round(r[0].splitViewRatio * 10) / 10;
		if (ratio !== 0.7) return 'FAIL: ctrl+= ratio=' + ratio + ', want 0.7';

		// Ctrl++ also increases.
		r = sendKey(r[0], 'ctrl++');
		ratio = Math.round(r[0].splitViewRatio * 10) / 10;
		if (ratio !== 0.8) return 'FAIL: ctrl++ ratio=' + ratio + ', want 0.8';

		// At max 0.8, should not go higher.
		r = sendKey(r[0], 'ctrl+=');
		ratio = Math.round(r[0].splitViewRatio * 10) / 10;
		if (ratio !== 0.8) return 'FAIL: ratio exceeded max, got ' + ratio;

		// Ctrl+- decreases.
		r = sendKey(r[0], 'ctrl+-');
		ratio = Math.round(r[0].splitViewRatio * 10) / 10;
		if (ratio !== 0.7) return 'FAIL: ctrl+- ratio=' + ratio + ', want 0.7';

		// Decrease to min.
		for (var i = 0; i < 10; i++) r = sendKey(r[0], 'ctrl+-');
		ratio = Math.round(r[0].splitViewRatio * 10) / 10;
		if (ratio !== 0.2) return 'FAIL: ratio below min, got ' + ratio;

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("split view ratio: %v", raw)
	}
}

func TestChunk16_SplitView_ClaudePaneNav(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'claude';
		s.claudeViewOffset = 0;

		// up/k scrolls Claude pane.
		var r = sendKey(s, 'up');
		if (r[0].claudeViewOffset !== 1) return 'FAIL: up offset=' + r[0].claudeViewOffset;

		r = sendKey(r[0], 'k');
		if (r[0].claudeViewOffset !== 2) return 'FAIL: k offset=' + r[0].claudeViewOffset;

		// down/j scrolls back.
		r = sendKey(r[0], 'down');
		if (r[0].claudeViewOffset !== 1) return 'FAIL: down offset=' + r[0].claudeViewOffset;

		r = sendKey(r[0], 'j');
		if (r[0].claudeViewOffset !== 0) return 'FAIL: j offset=' + r[0].claudeViewOffset;

		// home jumps far.
		r = sendKey(r[0], 'home');
		if (r[0].claudeViewOffset !== 999999) return 'FAIL: home offset=' + r[0].claudeViewOffset;

		// end jumps to bottom.
		r = sendKey(r[0], 'end');
		if (r[0].claudeViewOffset !== 0) return 'FAIL: end offset=' + r[0].claudeViewOffset;

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("split view claude nav: %v", raw)
	}
}

func TestChunk16_SplitView_ClaudeMouseWheel(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'claude';
		s.claudeViewOffset = 0;

		// Mouse wheel up.
		var r = sendWheel(s, 'up');
		if (r[0].claudeViewOffset !== 3) return 'FAIL: wheel-up offset=' + r[0].claudeViewOffset;

		// Mouse wheel down.
		r = sendWheel(r[0], 'down');
		if (r[0].claudeViewOffset !== 0) return 'FAIL: wheel-down offset=' + r[0].claudeViewOffset;

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("split view claude wheel: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  Mouse Click — Zone Dispatch
// ---------------------------------------------------------------------------

func TestChunk16_MouseClick_NavBar(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Mock gitExec to avoid depending on git availability.
		// TUI tests should be isolated and not require git commands.
		var origGitExec = globalThis.prSplit._gitExec;
		globalThis.prSplit._gitExec = function(dir, args) {
			// Return success for common git commands used by handleConfigState
			if (args[0] === 'rev-parse' && args[1] === '--abbrev-ref' && args[2] === 'HEAD') {
				return {stdout: 'feature', stderr: '', code: 0};
			}
			if (args[0] === 'rev-parse' && args[2] === 'refs/heads/main') {
				return {stdout: 'abc123', stderr: '', code: 0};
			}
			// Default success response
			return {stdout: '', stderr: '', code: 0};
		};
		try {
			setupPlanCache();

			// nav-back: PLAN_REVIEW -> CONFIG.
			var s = initState('PLAN_REVIEW');
			var restore = mockZoneHit('nav-back');
			try {
				var r = sendClick(s);
				if (r[0].wizardState !== 'CONFIG') return 'FAIL: nav-back: state=' + r[0].wizardState;
			} finally { restore(); }

			// nav-cancel: shows confirm dialog.
			s = initState('CONFIG');
			restore = mockZoneHit('nav-cancel');
			try {
				var r = sendClick(s);
				if (!r[0].showConfirmCancel) return 'FAIL: nav-cancel did not show confirm';
			} finally { restore(); }

			// nav-next: CONFIG -> handleNext -> startAnalysis.
			// startAnalysis calls captured handleConfigState which may fail due to
			// limited runtime setup, but the click IS handled (state changes from CONFIG).
			s = initState('CONFIG');
			globalThis.prSplit.runtime.baseBranch = 'main';
			globalThis.prSplit.runtime.dir = '.';
			globalThis.prSplit.runtime.strategy = 'heuristic';
			restore = mockZoneHit('nav-next');
			try {
				var r = sendClick(s);
				// Accept any state change from CONFIG (isProcessing=true, ERROR, etc.).
				if (r[0].wizardState === 'CONFIG' && !r[0].isProcessing && !r[0].errorDetails) {
					return 'FAIL: nav-next did not start processing or transition, state=' + r[0].wizardState;
				}
			} finally {
				restore();
			}

			return 'OK';
		} finally {
			globalThis.prSplit._gitExec = origGitExec;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("mouse click nav bar: %v", raw)
	}
}

func TestChunk16_MouseClick_NavCancelDuringVerify(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var interrupted = false;
		var mockSession = {
			interrupt: function() { interrupted = true; },
			kill: function() {}, close: function() {},
			isRunning: function() { return true; },
			output: function() { return ''; }, screen: function() { return ''; }
		};

		var s = initState('BRANCH_BUILDING');
		s.activeVerifySession = mockSession;
		s.lastVerifyInterruptTime = 0;

		// nav-cancel during verify should interrupt, NOT show cancel dialog.
		var restore = mockZoneHit('nav-cancel');
		try {
			var r = sendClick(s);
			if (!interrupted) return 'FAIL: nav-cancel did not interrupt verify session';
			if (r[0].showConfirmCancel) return 'FAIL: nav-cancel during verify should not show cancel dialog';
		} finally { restore(); }

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("nav cancel during verify: %v", raw)
	}
}

func TestChunk16_MouseClick_ConfigZones(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// strategy-heuristic.
		var s = initState('CONFIG');
		var restore = mockZoneHit('strategy-heuristic');
		try {
			var r = sendClick(s);
			if (globalThis.prSplit.runtime.mode !== 'heuristic') return 'FAIL: strategy-heuristic not set';
		} finally { restore(); }

		// strategy-directory.
		s = initState('CONFIG');
		restore = mockZoneHit('strategy-directory');
		try {
			var r = sendClick(s);
			if (globalThis.prSplit.runtime.mode !== 'directory') return 'FAIL: strategy-directory not set';
		} finally { restore(); }

		// strategy-auto triggers Claude check.
		s = initState('CONFIG');
		restore = mockZoneHit('strategy-auto');
		try {
			var r = sendClick(s);
			if (globalThis.prSplit.runtime.mode !== 'auto') return 'FAIL: strategy-auto not set';
			if (r[0].claudeCheckStatus !== 'checking') return 'FAIL: auto did not start claude check';
		} finally { restore(); }

		// toggle-advanced.
		s = initState('CONFIG');
		s.showAdvanced = false;
		restore = mockZoneHit('toggle-advanced');
		try {
			var r = sendClick(s);
			if (!r[0].showAdvanced) return 'FAIL: toggle-advanced did not enable';
			r = sendClick(r[0]);
			if (r[0].showAdvanced) return 'FAIL: toggle-advanced did not disable';
		} finally { restore(); }

		// test-claude triggers check.
		s = initState('CONFIG');
		restore = mockZoneHit('test-claude');
		try {
			var r = sendClick(s);
			if (r[0].claudeCheckStatus !== 'checking') return 'FAIL: test-claude did not start check';
			if (globalThis.prSplit.runtime.mode !== 'auto') return 'FAIL: test-claude did not set mode to auto';
		} finally { restore(); }

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("config zones: %v", raw)
	}
}

func TestChunk16_MouseClick_PlanReviewZones(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();

		// split-card-1 selects split.
		var s = initState('PLAN_REVIEW');
		s.selectedSplitIdx = 0;
		var restore = mockZoneHit('split-card-1');
		try {
			var r = sendClick(s);
			if (r[0].selectedSplitIdx !== 1) return 'FAIL: split-card-1 did not select, got ' + r[0].selectedSplitIdx;
		} finally { restore(); }

		// split-card-2.
		restore = mockZoneHit('split-card-2');
		try {
			var r = sendClick(s);
			if (r[0].selectedSplitIdx !== 2) return 'FAIL: split-card-2 did not select';
		} finally { restore(); }

		// plan-edit enters editor.
		s = initState('PLAN_REVIEW');
		restore = mockZoneHit('plan-edit');
		try {
			var r = sendClick(s);
			if (r[0].wizardState !== 'PLAN_EDITOR') return 'FAIL: plan-edit: state=' + r[0].wizardState;
		} finally { restore(); }

		// plan-regenerate goes back to CONFIG.
		s = initState('PLAN_REVIEW');
		restore = mockZoneHit('plan-regenerate');
		try {
			var r = sendClick(s);
			if (r[0].wizardState !== 'CONFIG') return 'FAIL: plan-regenerate: state=' + r[0].wizardState;
		} finally { restore(); }

		// ask-claude opens conversation overlay.
		s = initState('PLAN_REVIEW');
		restore = mockZoneHit('ask-claude');
		try {
			var r = sendClick(s);
			if (!r[0].claudeConvo.active) return 'FAIL: ask-claude did not open convo';
			if (r[0].claudeConvo.context !== 'plan-review') return 'FAIL: ask-claude context wrong';
		} finally { restore(); }

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("plan review zones: %v", raw)
	}
}

func TestChunk16_MouseClick_PlanEditorZones(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();

		// edit-split-1 selects split 1 and resets file idx.
		var s = initState('PLAN_EDITOR');
		s.selectedSplitIdx = 0;
		s.selectedFileIdx = 1;
		var restore = mockZoneHit('edit-split-1');
		try {
			var r = sendClick(s);
			if (r[0].selectedSplitIdx !== 1) return 'FAIL: edit-split-1 did not select';
			if (r[0].selectedFileIdx !== 0) return 'FAIL: file idx not reset';
		} finally { restore(); }

		// edit-file-0-1: select file 1 in split 0.
		s = initState('PLAN_EDITOR');
		s.selectedSplitIdx = 0;
		restore = mockZoneHit('edit-file-0-1');
		try {
			var r = sendClick(s);
			if (r[0].selectedFileIdx !== 1) return 'FAIL: edit-file-0-1 did not select file';
		} finally { restore(); }

		// editor-move opens move dialog.
		s = initState('PLAN_EDITOR');
		s.selectedSplitIdx = 0;
		s.selectedFileIdx = 0;
		restore = mockZoneHit('editor-move');
		try {
			var r = sendClick(s);
			if (r[0].activeEditorDialog !== 'move') return 'FAIL: editor-move did not open, got ' + r[0].activeEditorDialog;
		} finally { restore(); }

		// editor-rename opens rename dialog.
		s = initState('PLAN_EDITOR');
		s.selectedSplitIdx = 0;
		restore = mockZoneHit('editor-rename');
		try {
			var r = sendClick(s);
			if (r[0].activeEditorDialog !== 'rename') return 'FAIL: editor-rename did not open';
		} finally { restore(); }

		// editor-merge opens merge dialog.
		s = initState('PLAN_EDITOR');
		restore = mockZoneHit('editor-merge');
		try {
			var r = sendClick(s);
			if (r[0].activeEditorDialog !== 'merge') return 'FAIL: editor-merge did not open';
		} finally { restore(); }

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("plan editor zones: %v", raw)
	}
}

func TestChunk16_MouseClick_ErrorResolutionZones(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// NOTE: _handleErrorResolutionState is captured as a local var at module
		// load time (JS line 48), so it cannot be mocked via globalThis. We test
		// with the real handler. Mock resolveConflicts to prevent real async.
		var origResolve = globalThis.prSplit.resolveConflicts;
		globalThis.prSplit.resolveConflicts = function() {
			return {then: function(ok) { ok({errors: []}); return {then: function(){}};}};
		};

		try {
			var choices = ['auto', 'manual', 'skip', 'retry', 'abort'];
			for (var i = 0; i < choices.length; i++) {
				var s = initState('ERROR_RESOLUTION');
				var zoneId = 'resolve-' + choices[i];
				var restore = mockZoneHit(zoneId);
				try {
					var r = sendClick(s);
					// Verify the click was dispatched via real handler (no crash).
				} finally { restore(); }
			}

			// error-ask-claude opens convo.
			globalThis.prSplit._state.claudeExecutor = {};
			var s = initState('ERROR_RESOLUTION');
			var restore = mockZoneHit('error-ask-claude');
			try {
				var r = sendClick(s);
				if (!r[0].claudeConvo.active) return 'FAIL: error-ask-claude did not open convo';
				if (r[0].claudeConvo.context !== 'error-resolution') return 'FAIL: context wrong';
			} finally { restore(); }

		} finally {
			globalThis.prSplit.resolveConflicts = origResolve;
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("error resolution zones: %v", raw)
	}
}

func TestChunk16_MouseClick_FinalizationZones(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// final-report shows report overlay.
		var s = initState('FINALIZATION');
		var restore = mockZoneHit('final-report');
		try {
			var r = sendClick(s);
			if (!r[0].showingReport) return 'FAIL: final-report did not open report';
		} finally { restore(); }

		// final-create-prs dispatches to real handleFinalizationState.
		// We can't mock the captured ref, so just verify the click was handled
		// (returns a valid 2-element array without throwing).
		s = initState('FINALIZATION');
		restore = mockZoneHit('final-create-prs');
		try {
			var r = sendClick(s);
			if (!r || !r[0]) return 'FAIL: final-create-prs returned invalid result';
		} finally { restore(); }

		// final-done sets wizardState='DONE' and returns quit.
		s = initState('FINALIZATION');
		restore = mockZoneHit('final-done');
		try {
			var r = sendClick(s);
			if (r[0].wizardState !== 'DONE') return 'FAIL: final-done state=' + r[0].wizardState;
		} finally { restore(); }

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("finalization zones: %v", raw)
	}
}

func TestChunk16_MouseClick_VerifyInterruptZone(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var interrupted = false, killed = false;
		var mockSession = {
			interrupt: function() { interrupted = true; },
			kill: function() { killed = true; },
			close: function() {},
			isRunning: function() { return true; },
			output: function() { return ''; }, screen: function() { return ''; }
		};

		var s = initState('BRANCH_BUILDING');
		s.activeVerifySession = mockSession;
		s.lastVerifyInterruptTime = 0;

		// First click — interrupt.
		var restore = mockZoneHit('verify-interrupt');
		try {
			var r = sendClick(s);
			if (!interrupted) return 'FAIL: first click did not interrupt';
			if (killed) return 'FAIL: first click should not kill';

			// Second click within 2s — force kill.
			interrupted = false;
			r = sendClick(r[0]);
			if (!killed) return 'FAIL: second click did not kill';
		} finally { restore(); }

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("verify interrupt zone: %v", raw)
	}
}

func TestChunk16_MouseClick_VerifyExpandCollapse(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('BRANCH_BUILDING');
		s.expandedVerifyBranch = null;

		// verify-expand-split/api.
		var restore = mockZoneHit('verify-expand-split/api');
		try {
			var r = sendClick(s);
			if (r[0].expandedVerifyBranch !== 'split/api') return 'FAIL: expand did not set branch';
		} finally { restore(); }

		// verify-collapse-split/api.
		s.expandedVerifyBranch = 'split/api';
		restore = mockZoneHit('verify-collapse-split/api');
		try {
			var r = sendClick(s);
			if (r[0].expandedVerifyBranch !== null) return 'FAIL: collapse did not clear';
		} finally { restore(); }

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("verify expand/collapse: %v", raw)
	}
}

func TestChunk16_MouseClick_ConfirmCancelZones(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// confirm-yes → CANCELLED.
		var s = initState('CONFIG');
		s.showConfirmCancel = true;
		var restore = mockZoneHit('confirm-yes');
		try {
			var r = sendClick(s);
			if (r[0].showConfirmCancel) return 'FAIL: confirm-yes did not dismiss overlay';
			if (r[0].wizardState !== 'CANCELLED') return 'FAIL: confirm-yes state=' + r[0].wizardState;
		} finally { restore(); }

		// confirm-no → dismiss.
		s = initState('CONFIG');
		s.showConfirmCancel = true;
		restore = mockZoneHit('confirm-no');
		try {
			var r = sendClick(s);
			if (r[0].showConfirmCancel) return 'FAIL: confirm-no did not dismiss overlay';
			if (r[0].wizardState !== 'CONFIG') return 'FAIL: confirm-no changed state';
		} finally { restore(); }

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("confirm cancel zones: %v", raw)
	}
}

// T23: Verify wheel events are NOT interpreted as clicks in the confirm
// cancel dialog. Prior to the fix, the mouse press guard was missing
// the !msg.isWheel filter, meaning a scroll over "Yes" could
// accidentally confirm cancellation.
func TestChunk16_ConfirmCancel_WheelDoesNotTriggerZones(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Wheel over confirm-yes should NOT cancel.
		var s = initState('CONFIG');
		s.showConfirmCancel = true;
		var restore = mockZoneHit('confirm-yes');
		try {
			var r = sendWheel(s, 'up');
			if (!r[0].showConfirmCancel) return 'FAIL: wheel-up dismissed confirm overlay';
			if (r[0].wizardState === 'CANCELLED') return 'FAIL: wheel-up triggered cancel';
			r = sendWheel(r[0], 'down');
			if (!r[0].showConfirmCancel) return 'FAIL: wheel-down dismissed confirm overlay';
			if (r[0].wizardState === 'CANCELLED') return 'FAIL: wheel-down triggered cancel';
		} finally { restore(); }

		// And confirm-no shouldn't trigger either.
		s = initState('CONFIG');
		s.showConfirmCancel = true;
		restore = mockZoneHit('confirm-no');
		try {
			var r = sendWheel(s, 'up');
			if (!r[0].showConfirmCancel) return 'FAIL: wheel-up on no dismissed overlay';
			r = sendWheel(r[0], 'down');
			if (!r[0].showConfirmCancel) return 'FAIL: wheel-down on no dismissed overlay';
		} finally { restore(); }

		// Real click should still work after wheels.
		s = initState('CONFIG');
		s.showConfirmCancel = true;
		restore = mockZoneHit('confirm-yes');
		try {
			sendWheel(s, 'up'); // wheel first — harmless
			var r = sendClick(s); // real click — should trigger
			if (r[0].showConfirmCancel) return 'FAIL: click after wheel did not dismiss';
			if (r[0].wizardState !== 'CANCELLED') return 'FAIL: click after wheel state=' + r[0].wizardState;
		} finally { restore(); }

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("confirm cancel wheel guard: %v", raw)
	}
}

func TestChunk16_MouseClick_ClaudeStatusBadge(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// T45: Clicking claude-status badge now opens split-view instead of
		// calling tuiMux.switchTo(), so we mock tuiMux with hasChild().
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			session: function() { return { isRunning: function() { return true; }, isDone: function() { return false; } }; },
			screenshot: function() { return ''; },
			childScreen: function() { return ''; },
			lastActivityMs: function() { return 500; }
		};

		var s = initState('CONFIG');
		var restore = mockZoneHit('claude-status');
		try {
			var r = sendClick(s);
			s = r[0];
			if (!s.splitViewEnabled) return 'FAIL: claude-status should open split-view';
			if (s.splitViewTab !== 'claude') return 'FAIL: tab should be claude, got ' + s.splitViewTab;
		} finally {
			restore();
			delete globalThis.tuiMux;
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("claude status badge: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  T307: Mouse click handling for EQUIV_CHECK buttons
// ---------------------------------------------------------------------------

func TestChunk16_MouseClick_EquivCheckZones(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();

		// equiv-reverify: starts equivalence check (isProcessing=true, equivalenceResult cleared).
		var s = initState('EQUIV_CHECK');
		s.equivalenceResult = {equivalent: false, expected: 'a', actual: 'b'};
		s.isProcessing = false;
		var restore = mockZoneHit('equiv-reverify');
		try {
			var r = sendClick(s);
			if (!r[0].isProcessing) return 'FAIL: equiv-reverify did not set isProcessing';
			if (r[0].equivalenceResult !== null) return 'FAIL: equiv-reverify did not clear equivalenceResult';
			if (!r[0].equivRunning) return 'FAIL: equiv-reverify did not set equivRunning';
		} finally { restore(); }

		// equiv-revise: transitions to PLAN_REVIEW.
		s = initState('EQUIV_CHECK');
		s.equivalenceResult = {equivalent: false, expected: 'a', actual: 'b'};
		s.equivRunning = true;
		s.equivError = 'old error';
		s.isProcessing = false;
		restore = mockZoneHit('equiv-revise');
		try {
			var r = sendClick(s);
			if (r[0].wizardState !== 'PLAN_REVIEW') return 'FAIL: equiv-revise state=' + r[0].wizardState;
			if (r[0].isProcessing) return 'FAIL: equiv-revise should not be processing';
			// T308: verify equiv state cleanup.
			if (r[0].equivRunning) return 'FAIL: equiv-revise did not clear equivRunning';
			if (r[0].equivError !== null) return 'FAIL: equiv-revise did not clear equivError';
			if (r[0].equivalenceResult !== null) return 'FAIL: equiv-revise did not clear equivalenceResult';
		} finally { restore(); }

		// nav-next on EQUIV_CHECK with cached result: transitions to FINALIZATION.
		s = initState('EQUIV_CHECK');
		s.equivalenceResult = {equivalent: true};
		s.isProcessing = false;
		restore = mockZoneHit('nav-next');
		try {
			var r = sendClick(s);
			if (r[0].wizardState !== 'FINALIZATION') return 'FAIL: nav-next state=' + r[0].wizardState;
		} finally { restore(); }

		// equiv-reverify should be no-op when isProcessing=true (guard).
		s = initState('EQUIV_CHECK');
		s.isProcessing = true;
		restore = mockZoneHit('equiv-reverify');
		try {
			var r = sendClick(s);
			// Should not change state — isProcessing guard prevents click.
			if (r[0].wizardState !== 'EQUIV_CHECK') return 'FAIL: processing reverify changed state';
		} finally { restore(); }

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("equiv check zones: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  T308: Back navigation state cleanup for EQUIV_CHECK
// ---------------------------------------------------------------------------

// TestChunk16_EquivCheck_BackNavigation verifies that pressing Back on
// EQUIV_CHECK cleans up all equivalence state (equivalenceResult, equivRunning,
// equivError) to prevent stale data and orphaned async polling.
func TestChunk16_EquivCheck_BackNavigation(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();

		// Scenario 1: nav-back mouse click from EQUIV_CHECK with stale state.
		var s = initState('EQUIV_CHECK');
		s.equivalenceResult = {equivalent: false, expected: 'aaa', actual: 'bbb'};
		s.equivRunning = true;
		s.equivError = 'previous error';
		s.isProcessing = false;
		var restore = mockZoneHit('nav-back');
		try {
			var r = sendClick(s);
			s = r[0];
			if (s.wizardState !== 'PLAN_REVIEW') return 'FAIL: back state=' + s.wizardState;
			if (s.equivalenceResult !== null) return 'FAIL: equivalenceResult not cleared';
			if (s.equivRunning) return 'FAIL: equivRunning not cleared';
			if (s.equivError !== null) return 'FAIL: equivError not cleared';
			if (s.isProcessing) return 'FAIL: isProcessing not cleared';
		} finally { restore(); }

		// Scenario 2: keyboard Enter on nav-back (handleFocusActivate → handleBack).
		s = initState('EQUIV_CHECK');
		s.equivalenceResult = {equivalent: true};
		s.equivRunning = true;
		s.equivError = 'stale keyboard error';
		s.isProcessing = false;
		// Focus on nav-back. Find its index.
		var elems = globalThis.prSplit._getFocusElements(s);
		var navBackIdx = -1;
		for (var i = 0; i < elems.length; i++) {
			if (elems[i].id === 'nav-back') { navBackIdx = i; break; }
		}
		if (navBackIdx < 0) return 'FAIL: nav-back not in focus elements';
		s.focusIndex = navBackIdx;
		var r = sendKey(s, 'enter');
		s = r[0];
		if (s.wizardState !== 'PLAN_REVIEW') return 'FAIL: keyboard back state=' + s.wizardState;
		if (s.equivalenceResult !== null) return 'FAIL: keyboard back equivalenceResult not cleared';
		if (s.equivRunning) return 'FAIL: keyboard back equivRunning not cleared';
		if (s.equivError !== null) return 'FAIL: keyboard back equivError not cleared';

		// Scenario 3: equiv-revise keyboard activation cleans up state.
		s = initState('EQUIV_CHECK');
		s.equivalenceResult = {equivalent: false};
		s.equivRunning = true;
		s.equivError = 'stale';
		s.isProcessing = false;
		elems = globalThis.prSplit._getFocusElements(s);
		var reviseIdx = -1;
		for (var i = 0; i < elems.length; i++) {
			if (elems[i].id === 'equiv-revise') { reviseIdx = i; break; }
		}
		if (reviseIdx < 0) return 'FAIL: equiv-revise not in focus elements';
		s.focusIndex = reviseIdx;
		r = sendKey(s, 'enter');
		s = r[0];
		if (s.wizardState !== 'PLAN_REVIEW') return 'FAIL: revise keyboard state=' + s.wizardState;
		if (s.equivalenceResult !== null) return 'FAIL: revise keyboard equivalenceResult not cleared';
		if (s.equivRunning) return 'FAIL: revise keyboard equivRunning not cleared';
		if (s.equivError !== null) return 'FAIL: revise keyboard equivError not cleared';

		// Scenario 4: Back is no-op when isProcessing=true.
		s = initState('EQUIV_CHECK');
		s.isProcessing = true;
		s.equivRunning = true;
		restore = mockZoneHit('nav-back');
		try {
			r = sendClick(s);
			if (r[0].wizardState !== 'EQUIV_CHECK') return 'FAIL: processing back should not navigate';
			if (!r[0].equivRunning) return 'FAIL: processing back should not clear equivRunning';
		} finally { restore(); }

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("equiv check back navigation: %v", raw)
	}
}
