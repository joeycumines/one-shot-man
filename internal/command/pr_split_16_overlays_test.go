package command

import (
	"testing"
)

// ---------------------------------------------------------------------------
//  Report Overlay
// ---------------------------------------------------------------------------

func TestChunk16_ReportOverlay_Keyboard(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.showingReport = true;
		s.reportContent = 'test report content';
		// Set report viewport content so scroll is observable.
		if (s.reportVp) {
			var lines = [];
			for (var i = 0; i < 100; i++) lines.push('line ' + i);
			s.reportVp.setContent(lines.join('\n'));
			s.reportVp.setHeight(10);
		}

		// esc closes report.
		var r = sendKey(s, 'esc');
		if (r[0].showingReport) return 'FAIL: esc did not close report';

		// Re-open for next tests.
		s.showingReport = true;

		// enter closes report.
		r = sendKey(s, 'enter');
		if (r[0].showingReport) return 'FAIL: enter did not close report';
		s.showingReport = true;

		// q closes report.
		r = sendKey(s, 'q');
		if (r[0].showingReport) return 'FAIL: q did not close report';
		s.showingReport = true;

		// c copies to clipboard.
		globalThis._clipboardContent = '';
		r = sendKey(s, 'c');
		if (r[0].showingReport !== true) return 'FAIL: c closed report unexpectedly';
		if (globalThis._clipboardContent !== 'test report content') return 'FAIL: c did not copy to clipboard';

		// j/down scrolls down.
		r = sendKey(s, 'j');
		if (!r[0]) return 'FAIL: j returned invalid';
		r = sendKey(s, 'down');
		if (!r[0]) return 'FAIL: down returned invalid';

		// k/up scrolls up.
		r = sendKey(s, 'k');
		if (!r[0]) return 'FAIL: k returned invalid';
		r = sendKey(s, 'up');
		if (!r[0]) return 'FAIL: up returned invalid';

		// pgdown/space half-page down.
		r = sendKey(s, 'pgdown');
		if (!r[0]) return 'FAIL: pgdown returned invalid';
		r = sendKey(s, ' ');
		if (!r[0]) return 'FAIL: space returned invalid';

		// pgup half-page up.
		r = sendKey(s, 'pgup');
		if (!r[0]) return 'FAIL: pgup returned invalid';

		// home/g goes to top.
		r = sendKey(s, 'home');
		if (!r[0]) return 'FAIL: home returned invalid';
		r = sendKey(s, 'g');
		if (!r[0]) return 'FAIL: g returned invalid';

		// end goes to bottom.
		r = sendKey(s, 'end');
		if (!r[0]) return 'FAIL: end returned invalid';

		// Unknown key is consumed (report stays open).
		r = sendKey(s, 'x');
		if (!r[0].showingReport) return 'FAIL: unknown key closed report';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("report overlay keyboard: %v", raw)
	}
}

func TestChunk16_ReportOverlay_MouseCloseAndWheel(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.showingReport = true;
		s.reportContent = 'test';
		if (s.reportVp) {
			var lines = [];
			for (var i = 0; i < 100; i++) lines.push('line ' + i);
			s.reportVp.setContent(lines.join('\n'));
			s.reportVp.setHeight(10);
		}

		// Mouse wheel up scrolls report.
		var r = sendWheel(s, 'up');
		if (r[0].showingReport !== true) return 'FAIL: wheel-up closed report';

		// Mouse wheel down scrolls report.
		r = sendWheel(r[0], 'down');
		if (r[0].showingReport !== true) return 'FAIL: wheel-down closed report';

		// Mouse click outside closes report.
		r = sendClick(s);
		if (r[0].showingReport) return 'FAIL: click did not close report';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("report overlay mouse: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  Editor Dialogs (move, rename, merge)
// ---------------------------------------------------------------------------

func TestChunk16_EditorDialog_MoveKeyboard(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('PLAN_EDITOR');
		s.selectedSplitIdx = 0;
		s.selectedFileIdx = 0;
		s.activeEditorDialog = 'move';
		s.editorDialogState = {targetIdx: 0};

		// j/down navigates targets.
		var r = sendKey(s, 'j');
		if (r[0].editorDialogState.targetIdx !== 1) return 'FAIL: j did not advance target, got ' + r[0].editorDialogState.targetIdx;

		r = sendKey(r[0], 'down');
		if (!r[0].editorDialogState) return 'FAIL: down returned no dialog state';

		// k/up navigates back.
		r = sendKey(r[0], 'k');
		if (!r[0].editorDialogState) return 'FAIL: k returned no dialog state';

		r = sendKey(r[0], 'up');
		if (!r[0].editorDialogState) return 'FAIL: up returned no dialog state';

		// enter confirms move (moves file from split 0 to target split).
		s.selectedSplitIdx = 0;
		s.selectedFileIdx = 0;
		s.editorDialogState = {targetIdx: 0}; // targets[0] = split idx 1
		r = sendKey(s, 'enter');
		if (r[0].activeEditorDialog !== null) return 'FAIL: enter did not close move dialog';
		// File should have been moved.
		var srcFiles = globalThis.prSplit._state.planCache.splits[0].files;
		var dstFiles = globalThis.prSplit._state.planCache.splits[1].files;
		if (srcFiles.length !== 1) return 'FAIL: source files not reduced, got ' + srcFiles.length;
		if (dstFiles.length !== 3) return 'FAIL: dest files not increased, got ' + dstFiles.length;

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("move dialog keyboard: %v", raw)
	}
}

func TestChunk16_EditorDialog_MoveMouse(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('PLAN_EDITOR');
		s.selectedSplitIdx = 0;
		s.selectedFileIdx = 0;
		s.activeEditorDialog = 'move';
		s.editorDialogState = {targetIdx: 0};

		// Click move-target-1 to select target.
		var restore = mockZoneHit('move-target-1');
		try {
			var r = sendClick(s);
			if (r[0].editorDialogState.targetIdx !== 1) return 'FAIL: move-target-1 click did not set target';
		} finally { restore(); }

		// Click move-cancel to close.
		restore = mockZoneHit('move-cancel');
		try {
			var r = sendClick(s);
			if (r[0].activeEditorDialog !== null) return 'FAIL: move-cancel did not close dialog';
		} finally { restore(); }

		// Re-open and confirm via mouse.
		s.activeEditorDialog = 'move';
		s.editorDialogState = {targetIdx: 0};
		restore = mockZoneHit('move-confirm');
		try {
			var r = sendClick(s);
			if (r[0].activeEditorDialog !== null) return 'FAIL: move-confirm did not close dialog';
		} finally { restore(); }

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("move dialog mouse: %v", raw)
	}
}

func TestChunk16_EditorDialog_RenameKeyboard(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('PLAN_EDITOR');
		s.selectedSplitIdx = 0;
		s.activeEditorDialog = 'rename';
		s.editorDialogState = {inputText: 'split/api'};

		// Type characters.
		var r = sendKey(s, 'x');
		if (r[0].editorDialogState.inputText !== 'split/apix') return 'FAIL: char input failed, got ' + r[0].editorDialogState.inputText;

		// Backspace.
		r = sendKey(r[0], 'backspace');
		if (r[0].editorDialogState.inputText !== 'split/api') return 'FAIL: backspace failed';

		// Enter confirms rename.
		r = sendKey(r[0], 'enter');
		if (r[0].activeEditorDialog !== null) return 'FAIL: enter did not close rename dialog';
		if (globalThis.prSplit._state.planCache.splits[0].name !== 'split/api') return 'FAIL: rename not applied';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("rename dialog keyboard: %v", raw)
	}
}

func TestChunk16_EditorDialog_RenameMouse(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('PLAN_EDITOR');
		s.selectedSplitIdx = 0;
		s.activeEditorDialog = 'rename';
		s.editorDialogState = {inputText: 'new-name'};

		// rename-confirm.
		var restore = mockZoneHit('rename-confirm');
		try {
			var r = sendClick(s);
			if (r[0].activeEditorDialog !== null) return 'FAIL: rename-confirm did not close';
			if (globalThis.prSplit._state.planCache.splits[0].name !== 'new-name') return 'FAIL: rename not applied';
		} finally { restore(); }

		// Re-open and cancel.
		s.activeEditorDialog = 'rename';
		s.editorDialogState = {inputText: 'another'};
		restore = mockZoneHit('rename-cancel');
		try {
			var r = sendClick(s);
			if (r[0].activeEditorDialog !== null) return 'FAIL: rename-cancel did not close';
		} finally { restore(); }

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("rename dialog mouse: %v", raw)
	}
}

func TestChunk16_EditorDialog_MergeKeyboard(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('PLAN_EDITOR');
		s.selectedSplitIdx = 0;
		s.activeEditorDialog = 'merge';
		s.editorDialogState = {selected: {}, cursorIdx: 0};

		// j/down navigates items.
		var r = sendKey(s, 'j');
		if (r[0].editorDialogState.cursorIdx !== 1) return 'FAIL: j did not advance cursor, got ' + r[0].editorDialogState.cursorIdx;

		// k/up navigates back.
		r = sendKey(r[0], 'k');
		if (r[0].editorDialogState.cursorIdx !== 0) return 'FAIL: k did not go back';

		// Space toggles selection.
		r = sendKey(r[0], ' ');
		// The toggle is on mergeables[cursorIdx=0] which is index 1 (skip current=0).
		if (!r[0].editorDialogState.selected[1]) return 'FAIL: space did not toggle selection';

		// Enter confirms merge.
		var beforeLen = globalThis.prSplit._state.planCache.splits.length;
		r = sendKey(r[0], 'enter');
		if (r[0].activeEditorDialog !== null) return 'FAIL: enter did not close merge dialog';
		var afterLen = globalThis.prSplit._state.planCache.splits.length;
		if (afterLen >= beforeLen) return 'FAIL: merge did not reduce split count (before=' + beforeLen + ' after=' + afterLen + ')';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("merge dialog keyboard: %v", raw)
	}
}

func TestChunk16_EditorDialog_MergeMouse(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('PLAN_EDITOR');
		s.selectedSplitIdx = 0;
		s.activeEditorDialog = 'merge';
		s.editorDialogState = {selected: {}, cursorIdx: 0};

		// merge-item-0 toggles the first mergeable.
		var restore = mockZoneHit('merge-item-0');
		try {
			var r = sendClick(s);
			if (!r[0].editorDialogState.selected[1]) return 'FAIL: merge-item-0 click did not toggle';
		} finally { restore(); }

		// merge-cancel closes.
		s.editorDialogState = {selected: {}, cursorIdx: 0};
		restore = mockZoneHit('merge-cancel');
		try {
			var r = sendClick(s);
			if (r[0].activeEditorDialog !== null) return 'FAIL: merge-cancel did not close';
		} finally { restore(); }

		// Re-open and confirm.
		s.activeEditorDialog = 'merge';
		s.editorDialogState = {selected: {1: true}, cursorIdx: 0};
		restore = mockZoneHit('merge-confirm');
		try {
			var r = sendClick(s);
			if (r[0].activeEditorDialog !== null) return 'FAIL: merge-confirm did not close';
		} finally { restore(); }

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("merge dialog mouse: %v", raw)
	}
}

func TestChunk16_EditorDialog_EscClosesAll(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var dialogs = ['move', 'rename', 'merge'];
		for (var i = 0; i < dialogs.length; i++) {
			setupPlanCache();
			var s = initState('PLAN_EDITOR');
			s.activeEditorDialog = dialogs[i];
			s.editorDialogState = dialogs[i] === 'rename' ? {inputText: 'x'} : {targetIdx: 0, selected: {}, cursorIdx: 0};
			var r = sendKey(s, 'esc');
			if (r[0].activeEditorDialog !== null) return 'FAIL: esc did not close ' + dialogs[i] + ' dialog';
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("editor dialog esc: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  Claude Conversation Overlay
// ---------------------------------------------------------------------------

func TestChunk16_ClaudeConvo_InputAndSend(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_REVIEW');
		s.claudeConvo.active = true;
		s.claudeConvo.context = 'plan-review';
		s.claudeConvo.inputText = '';

		// Type characters.
		var r = sendKey(s, 'h');
		if (r[0].claudeConvo.inputText !== 'h') return 'FAIL: char h not appended';
		r = sendKey(r[0], 'i');
		if (r[0].claudeConvo.inputText !== 'hi') return 'FAIL: char i not appended';

		// Backspace deletes.
		r = sendKey(r[0], 'backspace');
		if (r[0].claudeConvo.inputText !== 'h') return 'FAIL: backspace failed, got ' + r[0].claudeConvo.inputText;

		// Ctrl+U clears.
		r[0].claudeConvo.inputText = 'hello world';
		r = sendKey(r[0], 'ctrl+u');
		if (r[0].claudeConvo.inputText !== '') return 'FAIL: ctrl+u did not clear';

		// Enter with empty text is no-op (doesn't crash).
		r[0].claudeConvo.inputText = '';
		r = sendKey(r[0], 'enter');
		if (r[0].claudeConvo.active !== true) return 'FAIL: enter on empty closed convo';

		// Esc closes.
		r = sendKey(r[0], 'esc');
		if (r[0].claudeConvo.active) return 'FAIL: esc did not close convo';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("claude convo input: %v", raw)
	}
}

func TestChunk16_ClaudeConvo_ScrollAndWheel(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_REVIEW');
		s.claudeConvo.active = true;
		s.claudeConvo.scrollOffset = 0;

		// up/pgup scrolls back.
		var r = sendKey(s, 'up');
		if (r[0].claudeConvo.scrollOffset !== 3) return 'FAIL: up did not scroll, got ' + r[0].claudeConvo.scrollOffset;

		r = sendKey(r[0], 'pgup');
		if (r[0].claudeConvo.scrollOffset !== 6) return 'FAIL: pgup did not scroll';

		// down/pgdown scrolls forward.
		r = sendKey(r[0], 'down');
		if (r[0].claudeConvo.scrollOffset !== 3) return 'FAIL: down did not scroll back';

		r = sendKey(r[0], 'pgdown');
		if (r[0].claudeConvo.scrollOffset !== 0) return 'FAIL: pgdown did not scroll to 0';

		// Mouse wheel.
		r = sendWheel(s, 'up');
		if (r[0].claudeConvo.scrollOffset < 1) return 'FAIL: wheel-up did not scroll';

		r = sendWheel(r[0], 'down');
		// Should decrement, possibly to 0.
		if (r[0].claudeConvo.scrollOffset < 0) return 'FAIL: wheel-down went negative';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("claude convo scroll: %v", raw)
	}
}

func TestChunk16_ClaudeConvo_SendingBlocksInput(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_REVIEW');
		s.claudeConvo.active = true;
		s.claudeConvo.sending = true;
		s.claudeConvo.inputText = 'locked';

		// Typing while sending is blocked.
		var r = sendKey(s, 'x');
		if (r[0].claudeConvo.inputText !== 'locked') return 'FAIL: typing during send not blocked';

		// Backspace while sending is blocked.
		r = sendKey(s, 'backspace');
		if (r[0].claudeConvo.inputText !== 'locked') return 'FAIL: backspace during send not blocked';

		// Ctrl+U while sending is blocked.
		r = sendKey(s, 'ctrl+u');
		if (r[0].claudeConvo.inputText !== 'locked') return 'FAIL: ctrl+u during send not blocked';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("claude convo sending blocks: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  Inline Title Editing (T17)
// ---------------------------------------------------------------------------

func TestChunk16_InlineTitleEdit_FullCycle(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('PLAN_EDITOR');
		s.editorTitleEditing = true;
		s.editorTitleEditingIdx = 0;
		s.editorTitleText = 'split/api';

		// Type characters.
		var r = sendKey(s, '-');
		if (r[0].editorTitleText !== 'split/api-') return 'FAIL: char not appended';
		r = sendKey(r[0], 'v');
		if (r[0].editorTitleText !== 'split/api-v') return 'FAIL: v not appended';
		r = sendKey(r[0], '2');
		if (r[0].editorTitleText !== 'split/api-v2') return 'FAIL: 2 not appended';

		// Backspace.
		r = sendKey(r[0], 'backspace');
		if (r[0].editorTitleText !== 'split/api-v') return 'FAIL: backspace failed';

		// Ctrl+U clears.
		r = sendKey(r[0], 'ctrl+u');
		if (r[0].editorTitleText !== '') return 'FAIL: ctrl+u did not clear';

		// Type new name and Enter to save.
		r[0].editorTitleText = 'new-api-name';
		r = sendKey(r[0], 'enter');
		if (r[0].editorTitleEditing) return 'FAIL: enter did not end editing';
		if (globalThis.prSplit._state.planCache.splits[0].name !== 'new-api-name') return 'FAIL: name not saved';

		// Re-enter editing and Esc to cancel.
		r[0].editorTitleEditing = true;
		r[0].editorTitleEditingIdx = 0;
		r[0].editorTitleText = 'should-not-save';
		r = sendKey(r[0], 'esc');
		if (r[0].editorTitleEditing) return 'FAIL: esc did not cancel editing';
		if (globalThis.prSplit._state.planCache.splits[0].name !== 'new-api-name') return 'FAIL: esc should not save';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("inline title edit: %v", raw)
	}
}

func TestChunk16_InlineTitleEdit_SwallowsUnknownKeys(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('PLAN_EDITOR');
		s.editorTitleEditing = true;
		s.editorTitleEditingIdx = 0;
		s.editorTitleText = 'test';

		// Multi-char keys like tab, f1, ctrl+c should be swallowed.
		var keys = ['tab', 'shift+tab', 'f1', 'ctrl+c', 'home', 'end', 'pgdown', 'pgup', 'ctrl+l'];
		for (var i = 0; i < keys.length; i++) {
			var r = sendKey(s, keys[i]);
			if (!r[0].editorTitleEditing) return 'FAIL: ' + keys[i] + ' exited editing';
			if (r[0].editorTitleText !== 'test') return 'FAIL: ' + keys[i] + ' modified text';
			if (r[0].showHelp) return 'FAIL: ' + keys[i] + ' leaked to help toggle';
			if (r[0].showConfirmCancel) return 'FAIL: ' + keys[i] + ' leaked to cancel';
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("inline edit swallows: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  T41: Inline title editing + navigation isolation
// ---------------------------------------------------------------------------

func TestChunk16_T41_EditNavIsolation_JKDoesNotMoveFile(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('PLAN_EDITOR');
		s.selectedFileIdx = 1;
		s.editorTitleEditing = true;
		s.editorTitleEditingIdx = 0;
		s.editorTitleText = 'hello';

		// j should add 'j' to title text, NOT move selectedFileIdx.
		var r = sendKey(s, 'j');
		if (r[0].editorTitleText !== 'helloj') return 'FAIL: j not appended to title';
		if (r[0].selectedFileIdx !== 1) return 'FAIL: j moved selectedFileIdx from 1 to ' + r[0].selectedFileIdx;

		// k should add 'k' to title text, NOT move selectedFileIdx.
		r = sendKey(r[0], 'k');
		if (r[0].editorTitleText !== 'hellojk') return 'FAIL: k not appended to title';
		if (r[0].selectedFileIdx !== 1) return 'FAIL: k moved selectedFileIdx from 1 to ' + r[0].selectedFileIdx;

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("edit nav isolation j/k: %v", raw)
	}
}

func TestChunk16_T41_EditNavIsolation_ArrowsSwallowed(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('PLAN_EDITOR');
		s.selectedFileIdx = 1;
		s.editorTitleEditing = true;
		s.editorTitleEditingIdx = 0;
		s.editorTitleText = 'test';

		// up/down should be swallowed (multi-char keys), NOT move selectedFileIdx.
		var r = sendKey(s, 'up');
		if (r[0].selectedFileIdx !== 1) return 'FAIL: up moved selectedFileIdx from 1 to ' + r[0].selectedFileIdx;
		if (r[0].editorTitleText !== 'test') return 'FAIL: up should not modify text';

		r = sendKey(r[0], 'down');
		if (r[0].selectedFileIdx !== 1) return 'FAIL: down moved selectedFileIdx from 1 to ' + r[0].selectedFileIdx;

		r = sendKey(r[0], 'pgup');
		if (r[0].selectedFileIdx !== 1) return 'FAIL: pgup moved selectedFileIdx';

		r = sendKey(r[0], 'pgdown');
		if (r[0].selectedFileIdx !== 1) return 'FAIL: pgdown moved selectedFileIdx';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("edit nav isolation arrows: %v", raw)
	}
}

func TestChunk16_T41_HandleListNavGuard_DirectCall(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('PLAN_EDITOR');
		s.selectedFileIdx = 1;
		s.editorTitleEditing = true;
		s.editorTitleEditingIdx = 0;
		s.editorTitleText = 'editing';

		// Directly call handleListNav via j key dispatch while editing.
		// The title interceptor should catch j first, but the handleListNav guard
		// provides defense-in-depth. Verify selectedFileIdx is unchanged.
		var r = sendKey(s, 'j');
		if (r[0].selectedFileIdx !== 1) return 'FAIL: handleListNav defense-in-depth failed, selectedFileIdx changed to ' + r[0].selectedFileIdx;

		// Verify the interceptor caught j and appended it.
		if (r[0].editorTitleText !== 'editingj') return 'FAIL: j not in title text';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("handleListNav guard: %v", raw)
	}
}

func TestChunk16_T41_EditNavIsolation_SplitIdxStable(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('PLAN_EDITOR');
		s.selectedSplitIdx = 0;
		s.selectedFileIdx = 0;

		// Start editing split 0 title.
		var r = sendKey(s, 'e');
		if (!r[0].editorTitleEditing) return 'FAIL: e did not start editing';
		if (r[0].editorTitleEditingIdx !== 0) return 'FAIL: wrong editing idx';

		// Type several characters including navigation keys j and k.
		r = sendKey(r[0], 'n');
		r = sendKey(r[0], 'e');
		r = sendKey(r[0], 'w');
		r = sendKey(r[0], '-');
		r = sendKey(r[0], 'j');
		r = sendKey(r[0], 'k');

		// Verify state integrity: selectedSplitIdx and selectedFileIdx unchanged.
		if (r[0].selectedSplitIdx !== 0) return 'FAIL: selectedSplitIdx changed to ' + r[0].selectedSplitIdx;
		if (r[0].selectedFileIdx !== 0) return 'FAIL: selectedFileIdx changed to ' + r[0].selectedFileIdx;
		if (r[0].editorTitleText !== 'split/apinew-jk') return 'FAIL: title text wrong: ' + r[0].editorTitleText;

		// Save with Enter.
		r = sendKey(r[0], 'enter');
		if (r[0].editorTitleEditing) return 'FAIL: editing not ended';
		if (globalThis.prSplit._state.planCache.splits[0].name !== 'split/apinew-jk') return 'FAIL: name not saved correctly';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("edit nav split idx stable: %v", raw)
	}
}

func TestChunk16_T41_EditNavIsolation_FocusCycleBlocked(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('PLAN_EDITOR');
		s.focusIndex = 2;
		s.editorTitleEditing = true;
		s.editorTitleEditingIdx = 0;
		s.editorTitleText = 'test';

		// Tab and Shift+Tab should be swallowed during editing.
		var r = sendKey(s, 'tab');
		if (r[0].focusIndex !== 2) return 'FAIL: tab changed focusIndex from 2 to ' + r[0].focusIndex;

		r = sendKey(r[0], 'shift+tab');
		if (r[0].focusIndex !== 2) return 'FAIL: shift+tab changed focusIndex from 2 to ' + r[0].focusIndex;

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("edit nav focus cycle blocked: %v", raw)
	}
}
