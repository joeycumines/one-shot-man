package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  Focus Activation (Enter on focused element)
// ---------------------------------------------------------------------------

func TestChunk16_FocusActivate_Strategy(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Focus on strategy-heuristic (index 1 in CONFIG focus elements).
		var s = initState('CONFIG');
		s.focusIndex = 1; // strategy-heuristic
		var r = sendKey(s, 'enter');
		if (globalThis.prSplit.runtime.mode !== 'heuristic') return 'FAIL: enter on heuristic did not select, mode=' + globalThis.prSplit.runtime.mode;

		// Focus on strategy-auto (index 0).
		s = initState('CONFIG');
		s.focusIndex = 0; // strategy-auto
		r = sendKey(s, 'enter');
		if (globalThis.prSplit.runtime.mode !== 'auto') return 'FAIL: enter on auto did not select';
		if (r[0].claudeCheckStatus !== 'checking') return 'FAIL: auto did not start claude check';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("focus activate strategy: %v", raw)
	}
}

func TestChunk16_FocusActivate_TestClaude(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		// Set mode to auto so test-claude button appears (index 3).
		globalThis.prSplit.runtime.mode = 'auto';
		s.focusIndex = 3; // test-claude
		var r = sendKey(s, 'enter');
		if (r[0].claudeCheckStatus !== 'checking') return 'FAIL: enter on test-claude did not start check';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("focus activate test-claude: %v", raw)
	}
}

func TestChunk16_FocusActivate_PlanReviewButtons(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();

		// plan-edit button is at index 3 (after 3 split cards).
		var s = initState('PLAN_REVIEW');
		s.focusIndex = 3; // plan-edit
		var r = sendKey(s, 'enter');
		if (r[0].wizardState !== 'PLAN_EDITOR') return 'FAIL: enter on plan-edit: state=' + r[0].wizardState;

		// plan-regenerate button is at index 4.
		s = initState('PLAN_REVIEW');
		s.focusIndex = 4; // plan-regenerate
		r = sendKey(s, 'enter');
		if (r[0].wizardState !== 'CONFIG') return 'FAIL: enter on plan-regenerate: state=' + r[0].wizardState;

		// ask-claude button is at index 5.
		s = initState('PLAN_REVIEW');
		s.focusIndex = 5; // ask-claude
		r = sendKey(s, 'enter');
		if (!r[0].claudeConvo.active) return 'FAIL: enter on ask-claude did not open convo';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("focus activate plan review: %v", raw)
	}
}

func TestChunk16_FocusActivate_EditorDialogButtons(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();

		// editor-move button is at index 3 (after 3 split cards).
		var s = initState('PLAN_EDITOR');
		s.selectedSplitIdx = 0;
		s.selectedFileIdx = 0;
		s.focusIndex = 3; // editor-move
		var r = sendKey(s, 'enter');
		if (r[0].activeEditorDialog !== 'move') return 'FAIL: enter on editor-move, got ' + r[0].activeEditorDialog;

		// editor-rename at index 4.
		s = initState('PLAN_EDITOR');
		s.selectedSplitIdx = 0;
		s.focusIndex = 4; // editor-rename
		r = sendKey(s, 'enter');
		if (r[0].activeEditorDialog !== 'rename') return 'FAIL: enter on editor-rename';

		// editor-merge at index 5.
		s = initState('PLAN_EDITOR');
		s.focusIndex = 5; // editor-merge
		r = sendKey(s, 'enter');
		if (r[0].activeEditorDialog !== 'merge') return 'FAIL: enter on editor-merge';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("focus activate editor dialogs: %v", raw)
	}
}

func TestChunk16_FocusActivate_ErrorButtons(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Verify focus element IDs are correct for ERROR_RESOLUTION.
		var s = initState('ERROR_RESOLUTION');
		var elems = globalThis.prSplit._getFocusElements(s);
		var expected = ['resolve-auto', 'resolve-manual', 'resolve-skip', 'resolve-retry', 'resolve-abort'];
		if (elems.length < expected.length) return 'FAIL: not enough focus elements, got ' + elems.length;
		for (var i = 0; i < expected.length; i++) {
			if (elems[i].id !== expected[i]) return 'FAIL: index ' + i + ': got ' + elems[i].id + ', want ' + expected[i];
		}

		// Test abort button — it goes through handleFocusActivate → handleErrorResolutionChoice
		// → handleErrorResolutionState → abort → tea.quit().
		// The real handler changes wizard state; abort returns quit command.
		s = initState('ERROR_RESOLUTION');
		s.focusIndex = 4; // resolve-abort
		var r = sendKey(s, 'enter');
		// abort should return a quit command (result[1] is non-null).
		if (!r[1]) return 'FAIL: abort did not return quit command';

		// Test auto-resolve — needs resolveConflicts mock since it calls that.
		var origResolve = globalThis.prSplit.resolveConflicts;
		globalThis.prSplit.resolveConflicts = function() {
			return {then: function(ok, fail) { ok({errors: []}); return {then: function(){}}; }};
		};
		try {
			s = initState('ERROR_RESOLUTION');
			s.focusIndex = 0; // resolve-auto
			r = sendKey(s, 'enter');
			if (!r[0].isProcessing) return 'FAIL: auto-resolve did not set isProcessing';
		} finally {
			globalThis.prSplit.resolveConflicts = origResolve;
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("focus activate error buttons: %v", raw)
	}
}

func TestChunk16_FocusActivate_ErrorBackRestoresSplitView(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.wizard.transition('PLAN_GENERATION');
		s.wizard.transition('ERROR');
		s.wizardState = 'ERROR';
		s.errorFromState = 'CONFIG';
		s.errorSplitViewState = {enabled: true, focus: 'claude', tab: 'verify'};
		s.splitViewEnabled = false;
		s.splitViewFocus = 'wizard';
		s.splitViewTab = 'output';

		var elems = globalThis.prSplit._getFocusElements(s);
		if (elems.length !== 2 || elems[0].id !== 'nav-back' || elems[1].id !== 'nav-cancel') {
			return 'FAIL: ERROR focus elements should be nav-back + nav-cancel, got: ' + JSON.stringify(elems);
		}

		s.focusIndex = 0; // nav-back
		var r = sendKey(s, 'enter');
		if (r[0].wizardState !== 'CONFIG') return 'FAIL: back should restore CONFIG, got: ' + r[0].wizardState;
		if (!r[0].splitViewEnabled) return 'FAIL: splitViewEnabled should be restored';
		if (r[0].splitViewFocus !== 'claude') return 'FAIL: splitViewFocus should be restored, got: ' + r[0].splitViewFocus;
		if (r[0].splitViewTab !== 'verify') return 'FAIL: splitViewTab should be restored, got: ' + r[0].splitViewTab;
		if (r[0].errorFromState) return 'FAIL: errorFromState should be cleared, got: ' + r[0].errorFromState;
		if (r[0].errorSplitViewState !== null) return 'FAIL: errorSplitViewState should be cleared';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("focus activate error back: %v", raw)
	}
}

func TestChunk16_FocusActivate_ErrorAskClaude(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// error-ask-claude only appears when claudeExecutor exists.
		globalThis.prSplit._state.claudeExecutor = {};

		var s = initState('ERROR_RESOLUTION');
		// With claudeExecutor set, focus elements are: 5 resolve buttons + error-ask-claude.
		s.focusIndex = 5; // error-ask-claude
		var r = sendKey(s, 'enter');
		if (!r[0].claudeConvo.active) return 'FAIL: enter on error-ask-claude did not open convo';
		if (r[0].claudeConvo.context !== 'error-resolution') return 'FAIL: context wrong';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("focus activate error-ask-claude: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  Plan Editor Specific Keys
// ---------------------------------------------------------------------------

func TestChunk16_PlanEditor_SpaceToggle(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('PLAN_EDITOR');
		s.selectedSplitIdx = 0;
		s.selectedFileIdx = 0;
		s.editorCheckedFiles = {};

		// Space toggles checked.
		var r = sendKey(s, ' ');
		if (!r[0].editorCheckedFiles['0-0']) return 'FAIL: space did not check file';

		// Space again unchecks.
		r = sendKey(r[0], ' ');
		if (r[0].editorCheckedFiles['0-0']) return 'FAIL: space did not uncheck file';

		// Toggle second file.
		r[0].selectedFileIdx = 1;
		r = sendKey(r[0], ' ');
		if (!r[0].editorCheckedFiles['0-1']) return 'FAIL: space did not check file 1';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("plan editor space toggle: %v", raw)
	}
}

func TestChunk16_PlanEditor_ShiftReorder(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('PLAN_EDITOR');
		s.selectedSplitIdx = 0;
		s.selectedFileIdx = 1; // Start at second file.
		s.editorCheckedFiles = {'0-1': true}; // Second file checked.

		var files = globalThis.prSplit._state.planCache.splits[0].files;
		var originalSecond = files[1]; // pkg/types.go

		// Shift+up moves file up.
		var r = sendKey(s, 'shift+up');
		if (r[0].selectedFileIdx !== 0) return 'FAIL: shift+up did not move index';
		if (files[0] !== originalSecond) return 'FAIL: shift+up did not swap files';
		// Checked state should follow the file.
		if (!r[0].editorCheckedFiles['0-0']) return 'FAIL: checked state did not follow up';
		if (r[0].editorCheckedFiles['0-1']) return 'FAIL: old checked position not cleared';

		// Shift+down moves file back down.
		r = sendKey(r[0], 'shift+down');
		if (r[0].selectedFileIdx !== 1) return 'FAIL: shift+down did not move index';

		// At boundary: shift+up at index 0 is no-op.
		r[0].selectedFileIdx = 0;
		r = sendKey(r[0], 'shift+up');
		if (r[0].selectedFileIdx !== 0) return 'FAIL: shift+up at 0 should be no-op';

		// At boundary: shift+down at last index is no-op.
		r[0].selectedFileIdx = files.length - 1;
		r = sendKey(r[0], 'shift+down');
		if (r[0].selectedFileIdx !== files.length - 1) return 'FAIL: shift+down at last should be no-op';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("plan editor shift reorder: %v", raw)
	}
}

func TestChunk16_PlanEditor_FileNavigation(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('PLAN_EDITOR');
		s.selectedSplitIdx = 0;
		s.selectedFileIdx = 0;

		// j/down navigates files (not splits) in PLAN_EDITOR.
		var r = sendKey(s, 'j');
		if (r[0].selectedFileIdx !== 1) return 'FAIL: j did not advance file, got ' + r[0].selectedFileIdx;

		r = sendKey(r[0], 'down');
		// Should stay at 1 since split 0 only has 2 files (index 0,1).
		if (r[0].selectedFileIdx !== 1) return 'FAIL: down at max not clamped';

		r = sendKey(r[0], 'k');
		if (r[0].selectedFileIdx !== 0) return 'FAIL: k did not go back';

		r = sendKey(r[0], 'up');
		if (r[0].selectedFileIdx !== 0) return 'FAIL: up at 0 not clamped';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("plan editor file nav: %v", raw)
	}
}

func TestChunk16_PlanEditor_InlineEditViaE(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('PLAN_EDITOR');
		s.selectedSplitIdx = 1;

		// 'e' in PLAN_EDITOR enters inline title edit.
		var r = sendKey(s, 'e');
		if (!r[0].editorTitleEditing) return 'FAIL: e did not enter editing';
		if (r[0].editorTitleEditingIdx !== 1) return 'FAIL: editing wrong split';
		if (r[0].editorTitleText !== 'split/cli') return 'FAIL: text not pre-filled, got ' + r[0].editorTitleText;

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("plan editor inline edit via e: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  Navigation Handlers — handleNext / handleBack
// ---------------------------------------------------------------------------

func TestChunk16_HandleNext_PlanEditorDone(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();

		var s = initState('PLAN_EDITOR');
		// Focus on nav-next (type='nav') so handleFocusActivate returns null
		// and enter falls through to handleNext.
		// With 3 splits: 0-2=cards, 3=move, 4=rename, 5=merge, 6=nav-next.
		s.focusIndex = 6;
		s.editorCheckedFiles = {'0-0': true};
		var r = sendKey(s, 'enter');
		// Enter → handleFocusActivate(null for nav) → handleNext → handlePlanEditorState('done')
		// Real handler validates plan → success → transitions to PLAN_REVIEW.
		if (r[0].wizardState !== 'PLAN_REVIEW') return 'FAIL: state=' + r[0].wizardState;
		// Inline editing state should be cleared.
		if (r[0].editorTitleEditing) return 'FAIL: editing not cleared';
		if (Object.keys(r[0].editorCheckedFiles).length !== 0) return 'FAIL: checked files not cleared';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("handleNext plan editor: %v", raw)
	}
}

func TestChunk16_HandleNext_PlanEditorValidationFail(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Set up invalid plan: splits=[] triggers real validation_failed.
		globalThis.prSplit._state.planCache = {
			baseBranch: 'main', sourceBranch: 'feature', splits: []
		};

		var s = initState('PLAN_EDITOR');
		// With 0 splits, focus elements: editor-move(0), editor-rename(1), editor-merge(2), nav-next(3).
		s.focusIndex = 3; // nav-next → handleFocusActivate returns null → handleNext called.
		var r = sendKey(s, 'enter');
		// Real handlePlanEditorState('done', {splits:[]}) returns validation_failed.
		if (r[0].wizardState !== 'PLAN_EDITOR') return 'FAIL: state=' + r[0].wizardState;
		if (!r[0].editorValidationErrors || r[0].editorValidationErrors.length === 0) return 'FAIL: errors not set';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("handleNext validation fail: %v", raw)
	}
}

func TestChunk16_HandleNext_FinalizationQuits(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// NOTE: _handleFinalizationState is captured as a local var at module
		// load time (JS line 50), so it cannot be mocked. We test with the
		// real handler, which correctly transitions to DONE and returns quit.
		var s = initState('FINALIZATION');
		s.focusIndex = 2; // final-done button.
		var r = sendKey(s, 'enter');
		if (r[0].wizardState !== 'DONE') return 'FAIL: state=' + r[0].wizardState;
		// Should return tea.quit() command.
		if (!r[1]) return 'FAIL: no quit command';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("handleNext finalization: %v", raw)
	}
}

func TestChunk16_HandleBack_PlanEditorToReview(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('PLAN_EDITOR');
		// Do NOT set editorTitleEditing — that intercepts esc before handleBack.
		// Instead, set state that handleBack clears but doesn't intercept keys.
		s.editorCheckedFiles = {'0-0': true};
		s.editorValidationErrors = ['old error'];

		var r = sendKey(s, 'esc');
		if (r[0].wizardState !== 'PLAN_REVIEW') return 'FAIL: state=' + r[0].wizardState;
		// All inline editing state should be cleared by handleBack.
		if (r[0].editorTitleEditing) return 'FAIL: editing not cleared';
		if (r[0].editorTitleEditingIdx !== -1) return 'FAIL: editingIdx not reset';
		if (r[0].editorTitleText !== '') return 'FAIL: text not cleared';
		if (Object.keys(r[0].editorCheckedFiles).length !== 0) return 'FAIL: checkedFiles not cleared';
		if (r[0].editorValidationErrors.length !== 0) return 'FAIL: validationErrors not cleared';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("handleBack plan editor: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  Viewport Scroll & Termmux
// ---------------------------------------------------------------------------

func TestChunk16_ViewportScroll_Keys(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		// Set up tall content.
		var lines = [];
		for (var i = 0; i < 200; i++) lines.push('line ' + i);
		s.vp.setContent(lines.join('\n'));
		s.vp.setHeight(10);

		// pgdown scrolls down.
		var before = s.vp.yOffset();
		var r = sendKey(s, 'pgdown');
		var after = r[0].vp.yOffset();
		if (after <= before) return 'FAIL: pgdown did not scroll (before=' + before + ' after=' + after + ')';

		// pgup scrolls back up.
		before = r[0].vp.yOffset();
		r = sendKey(r[0], 'pgup');
		after = r[0].vp.yOffset();
		if (after >= before) return 'FAIL: pgup did not scroll (before=' + before + ' after=' + after + ')';

		// end goes to bottom.
		r = sendKey(r[0], 'end');
		var endOffset = r[0].vp.yOffset();

		// home goes to top.
		r = sendKey(r[0], 'home');
		if (r[0].vp.yOffset() !== 0) return 'FAIL: home did not go to top';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("viewport scroll keys: %v", raw)
	}
}

// TestChunk16_CtrlBracketTermmux verifies that the _onToggle callback
// (used by the toggleModel wrapper) dispatches through the Claude proxy's
// passthrough (activate → switchTo → restore) when a pinned SessionID exists.
// Task 5: Updated to use session-specific passthrough pattern.
func TestChunk16_CtrlBracketTermmux(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var activateCalls = [];
		var switchCalled = false;
		globalThis.tuiMux = {
			isDone: function(id) { return false; },
			activeID: function() { return 99; },
			activate: function(id) { activateCalls.push(id); },
			switchTo: function() { switchCalled = true; return {reason: 'toggle'}; },
			snapshot: function(id) { return { fullScreen: '', plainText: '' }; }
		};
		// Set pinned Claude SessionID.
		var savedCID = prSplit._state.claudeSessionID;
		prSplit._state.claudeSessionID = 5;

		var result = globalThis.prSplit._onToggle();

		delete globalThis.tuiMux;
		if (savedCID !== undefined) prSplit._state.claudeSessionID = savedCID;
		else delete prSplit._state.claudeSessionID;

		if (!switchCalled) return 'FAIL: _onToggle did not call switchTo';
		if (result.skipped) return 'FAIL: should not be skipped';
		if (activateCalls[0] !== 5) return 'FAIL: activate called with wrong ID: ' + activateCalls[0];

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("ctrl+] termmux: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  Edge Cases
// ---------------------------------------------------------------------------

func TestChunk16_TickPassthroughOverlay(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.showHelp = true;

		// Tick messages should pass through even when help overlay is active.
		var r = update({type: 'Tick', id: 'unknown-tick'}, s);
		// Should not crash and should still have help open.
		if (!r[0].showHelp) return 'FAIL: tick closed help overlay';

		// Claude convo overlay should also let ticks through.
		s.showHelp = false;
		s.claudeConvo.active = true;
		r = update({type: 'Tick', id: 'unknown-tick'}, s);
		if (!r[0].claudeConvo.active) return 'FAIL: tick closed claude convo';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("tick passthrough: %v", raw)
	}
}

func TestChunk16_FocusResetOnTransition(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('PLAN_REVIEW');
		s.focusIndex = 3; // Some non-zero focus.
		// Simulate a state transition by changing wizardState.
		s._prevWizardState = 'CONFIG'; // Mismatch triggers reset.
		s.wizardState = 'PLAN_REVIEW';

		// Any message should trigger focus reset.
		var r = sendKey(s, 'j');
		if (r[0]._prevWizardState !== 'PLAN_REVIEW') return 'FAIL: prev not synced';
		// After the reset, focusIndex should have been zeroed and then
		// j increments it.
		if (r[0].focusIndex !== 1) return 'FAIL: focus not reset then incremented, got ' + r[0].focusIndex;

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("focus reset: %v", raw)
	}
}

func TestChunk16_ProcessingGuard(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('PLAN_REVIEW');
		s.isProcessing = true;

		// Enter during processing should be a no-op (handleNext guard).
		var r = sendKey(s, 'enter');
		// Should still be PLAN_REVIEW (not start execution).
		if (r[0].wizardState !== 'PLAN_REVIEW') return 'FAIL: processing guard failed, state=' + r[0].wizardState;

		// 'e' during processing should not enter editor.
		r = sendKey(s, 'e');
		if (r[0].wizardState === 'PLAN_EDITOR') return 'FAIL: e during processing entered editor';

		// plan-edit click during processing should be a no-op.
		var restore = mockZoneHit('plan-edit');
		try {
			r = sendClick(s);
			if (r[0].wizardState === 'PLAN_EDITOR') return 'FAIL: plan-edit click during processing';
		} finally { restore(); }

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("processing guard: %v", raw)
	}
}

func TestChunk16_OverlayPriority(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Help overlay has highest priority — swallows all keys.
		var s = initState('CONFIG');
		s.showHelp = true;
		var r = sendKey(s, 'ctrl+c');
		if (r[0].showConfirmCancel) return 'FAIL: ctrl+c leaked through help overlay';
		if (r[0].showHelp) return 'FAIL: help not closed by any-key';

		// Confirm cancel overlay next.
		s = initState('CONFIG');
		s.showConfirmCancel = true;
		r = sendKey(s, '?');
		if (r[0].showHelp) return 'FAIL: ? leaked through confirm cancel';

		// Editor dialog intercepts all.
		setupPlanCache();
		s = initState('PLAN_EDITOR');
		s.activeEditorDialog = 'rename';
		s.editorDialogState = {inputText: 'x'};
		r = sendKey(s, '?');
		if (r[0].showHelp) return 'FAIL: ? leaked through editor dialog';

		// Claude convo intercepts all.
		s = initState('PLAN_REVIEW');
		s.claudeConvo.active = true;
		r = sendKey(s, '?');
		if (r[0].showHelp) return 'FAIL: ? leaked through claude convo';

		// Inline title editing intercepts all keys.
		s = initState('PLAN_EDITOR');
		s.editorTitleEditing = true;
		s.editorTitleText = '';
		r = sendKey(s, '?');
		// '?' is a single char, so it should be typed into the title.
		if (r[0].editorTitleText !== '?') return 'FAIL: ? not typed in edit mode';
		if (r[0].showHelp) return 'FAIL: ? leaked through title edit';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("overlay priority: %v", raw)
	}
}

func TestChunk16_GetFocusElements_AllStates(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var errors = [];

		function check(state, minCount, label) {
			var s = initState(state);
			if (state === 'ERROR_RESOLUTION') {
				globalThis.prSplit._state.claudeExecutor = {};
			}
			var elems = globalThis.prSplit._getFocusElements(s);
			if (!elems || elems.length < minCount) {
				errors.push(label + ': got ' + (elems ? elems.length : 0) + ' elements, want >= ' + minCount);
			}
		}

		// CONFIG: auto, heuristic, directory, toggle-advanced, nav-next = 5 minimum.
		check('CONFIG', 5, 'CONFIG');
		// PLAN_REVIEW: 3 cards + plan-edit + plan-regenerate + ask-claude + nav-next = 7.
		check('PLAN_REVIEW', 7, 'PLAN_REVIEW');
		// PLAN_EDITOR: 3 cards + editor-move + editor-rename + editor-merge + nav-next = 7.
		check('PLAN_EDITOR', 7, 'PLAN_EDITOR');
		// ERROR_RESOLUTION: 5 buttons + error-ask-claude + nav-next = 7.
		check('ERROR_RESOLUTION', 7, 'ERROR_RESOLUTION');
		// FINALIZATION: final-report + final-create-prs + final-done + nav-next = 4.
		check('FINALIZATION', 4, 'FINALIZATION');
		// T301: EQUIV_CHECK with failed equivalence: equiv-reverify + equiv-revise + nav-back + nav-next + nav-cancel = 5.
		var eqs = initState('EQUIV_CHECK');
		eqs.isProcessing = false;
		eqs.equivalenceResult = {equivalent: false};
		var eqElems = globalThis.prSplit._getFocusElements(eqs);
		if (!eqElems || eqElems.length < 5) {
			errors.push('EQUIV_CHECK: got ' + (eqElems ? eqElems.length : 0) + ' elements, want >= 5');
		}
		// BRANCH_BUILDING has no focus elements (default case).
		var s = initState('BRANCH_BUILDING');
		var elems = globalThis.prSplit._getFocusElements(s);
		if (elems.length !== 0) errors.push('BRANCH_BUILDING: got ' + elems.length + ' elements, want 0');

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("getFocusElements: %v", raw)
	}
}

func TestChunk16_NavigationFocusSyncSplit(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('PLAN_REVIEW');
		s.focusIndex = 0;
		s.selectedSplitIdx = 0;

		// Tab cycles through all elements and syncs split selection.
		var r = sendKey(s, 'tab');
		// Focus 0→1 (split-card-1). Should sync selectedSplitIdx to 1.
		if (r[0].focusIndex !== 1) return 'FAIL: tab focus=' + r[0].focusIndex + ', want 1';
		if (r[0].selectedSplitIdx !== 1) return 'FAIL: tab split=' + r[0].selectedSplitIdx + ', want 1';

		r = sendKey(r[0], 'tab');
		// Focus 1→2 (split-card-2).
		if (r[0].focusIndex !== 2) return 'FAIL: tab2 focus=' + r[0].focusIndex;
		if (r[0].selectedSplitIdx !== 2) return 'FAIL: tab2 split=' + r[0].selectedSplitIdx;

		// Tab past cards to buttons (focus 3 = plan-edit, type=button).
		r = sendKey(r[0], 'tab');
		if (r[0].focusIndex !== 3) return 'FAIL: tab3 focus=' + r[0].focusIndex;
		// selectedSplitIdx should NOT change when focus moves to a button.
		if (r[0].selectedSplitIdx !== 2) return 'FAIL: split changed when focused on button';

		// Shift+tab wraps around.
		s = initState('PLAN_REVIEW');
		s.focusIndex = 0;
		r = sendKey(s, 'shift+tab');
		// Should wrap to last element.
		var elems = globalThis.prSplit._getFocusElements(s);
		if (r[0].focusIndex !== elems.length - 1) return 'FAIL: shift+tab did not wrap';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("navigation focus sync: %v", raw)
	}
}

// TestChunk16_EditSplitCancelsInlineEdit verifies that clicking a different
// split while inline title editing is active cancels the edit.
func TestChunk16_EditSplitCancelsInlineEdit(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('PLAN_EDITOR');
		s.selectedSplitIdx = 0;
		s.editorTitleEditing = true;
		s.editorTitleEditingIdx = 0;
		s.editorTitleText = 'unsaved';

		// Click edit-split-2 (different split).
		var restore = mockZoneHit('edit-split-2');
		try {
			var r = sendClick(s);
			if (r[0].editorTitleEditing) return 'FAIL: editing not cancelled';
			if (r[0].selectedSplitIdx !== 2) return 'FAIL: split not changed';
		} finally { restore(); }

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("edit split cancels inline edit: %v", raw)
	}
}

// TestChunk16_HandleNext_ProcessingGuardAllStates verifies handleNext is
// a no-op when isProcessing=true in every wizard state.
func TestChunk16_HandleNext_ProcessingGuardAllStates(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var states = ['CONFIG', 'PLAN_REVIEW', 'PLAN_EDITOR', 'ERROR_RESOLUTION', 'FINALIZATION'];
		for (var i = 0; i < states.length; i++) {
			var s = initState(states[i]);
			s.isProcessing = true;
			// Set focusIndex past all focus elements so handleFocusActivate returns null
			// and enter falls through to handleNext, which checks isProcessing.
			s.focusIndex = 99;
			var r = sendKey(s, 'enter');
			if (r[0].wizardState !== states[i]) {
				return 'FAIL: ' + states[i] + ': state changed to ' + r[0].wizardState;
			}
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("processing guard all states: %v", raw)
	}
}

// TestChunk16_InlineTitleEdit_EnterWithEmptyDoesNotSave verifies enter
// with whitespace-only text does not save (trim check).
func TestChunk16_InlineTitleEdit_EnterWithEmptyDoesNotSave(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var origName = globalThis.prSplit._state.planCache.splits[0].name;
		var s = initState('PLAN_EDITOR');
		s.editorTitleEditing = true;
		s.editorTitleEditingIdx = 0;
		s.editorTitleText = '   ';

		var r = sendKey(s, 'enter');
		if (r[0].editorTitleEditing) return 'FAIL: still editing';
		// Name should be unchanged (empty/whitespace-only text not saved).
		if (globalThis.prSplit._state.planCache.splits[0].name !== origName) return 'FAIL: empty name saved';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("inline edit empty save: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  T301: EQUIV_CHECK focus elements include nav-back
// ---------------------------------------------------------------------------

func TestChunk16_GetFocusElements_EquivCheckIncludesNavBack(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var errors = [];

		// Case 1: equivalence failed (equivalent=false) — all buttons visible.
		var s = initState('EQUIV_CHECK');
		s.isProcessing = false;
		s.equivalenceResult = {equivalent: false};
		var elems = globalThis.prSplit._getFocusElements(s);

		// Expect: equiv-reverify, equiv-revise, nav-back, nav-next, nav-cancel
		var expectedIds = ['equiv-reverify', 'equiv-revise', 'nav-back', 'nav-next', 'nav-cancel'];
		if (elems.length !== expectedIds.length) {
			errors.push('equiv-fail: got ' + elems.length + ' elements, want ' + expectedIds.length);
		}
		for (var i = 0; i < expectedIds.length; i++) {
			if (!elems[i] || elems[i].id !== expectedIds[i]) {
				errors.push('equiv-fail[' + i + ']: got "' + (elems[i] ? elems[i].id : 'undefined') + '", want "' + expectedIds[i] + '"');
			}
		}

		// Case 2: equivalence succeeded (equivalent=true) — no reverify/revise.
		s = initState('EQUIV_CHECK');
		s.isProcessing = false;
		s.equivalenceResult = {equivalent: true};
		elems = globalThis.prSplit._getFocusElements(s);

		// Expect: nav-back, nav-next, nav-cancel
		var expectedPass = ['nav-back', 'nav-next', 'nav-cancel'];
		if (elems.length !== expectedPass.length) {
			errors.push('equiv-pass: got ' + elems.length + ' elements, want ' + expectedPass.length);
		}
		for (var i = 0; i < expectedPass.length; i++) {
			if (!elems[i] || elems[i].id !== expectedPass[i]) {
				errors.push('equiv-pass[' + i + ']: got "' + (elems[i] ? elems[i].id : 'undefined') + '", want "' + expectedPass[i] + '"');
			}
		}

		// Case 3: isProcessing=true → no elements at all.
		s = initState('EQUIV_CHECK');
		s.isProcessing = true;
		s.equivalenceResult = {equivalent: false};
		elems = globalThis.prSplit._getFocusElements(s);
		if (elems.length !== 0) {
			errors.push('processing: got ' + elems.length + ' elements, want 0');
		}

		// Case 4: no equivalenceResult → no elements.
		s = initState('EQUIV_CHECK');
		s.isProcessing = false;
		s.equivalenceResult = null;
		elems = globalThis.prSplit._getFocusElements(s);
		if (elems.length !== 0) {
			errors.push('no-result: got ' + elems.length + ' elements, want 0');
		}

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("EQUIV_CHECK focus elements: %v", raw)
	}
}

// TestChunk16_FocusActivate_NavBack_EquivCheck verifies that pressing
// Enter on nav-back in EQUIV_CHECK calls handleBack (→ PLAN_REVIEW),
// NOT handleNext (→ FINALIZATION).
func TestChunk16_FocusActivate_NavBack_EquivCheck(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('EQUIV_CHECK');
		s.isProcessing = false;
		s.equivalenceResult = {equivalent: false};

		// Focus elements: [equiv-reverify(0), equiv-revise(1), nav-back(2), nav-next(3), nav-cancel(4)]
		s.focusIndex = 2; // nav-back

		var r = sendKey(s, 'enter');
		if (r[0].wizardState === 'FINALIZATION') {
			return 'FAIL: Enter on nav-back triggered handleNext → FINALIZATION (should call handleBack → PLAN_REVIEW)';
		}
		if (r[0].wizardState !== 'PLAN_REVIEW') {
			return 'FAIL: Enter on nav-back should transition to PLAN_REVIEW, got ' + r[0].wizardState;
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("nav-back activation: %v", raw)
	}
}
