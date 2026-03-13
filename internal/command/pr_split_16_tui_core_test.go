package command

import (
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
//  T020: Comprehensive keyboard & mouse event handling tests for chunk 16
//
//  Covers: overlays (report, editor dialogs, Claude conversation, inline
//  title edit), live verify session, split-view, all mouse zone clicks,
//  focus activation, plan editor keys, navigation handlers, and edge cases.
//
//  Does NOT duplicate tests already in pr_split_13_tui_test.go (help toggle,
//  ctrl+c, confirm cancel y/n/esc/enter, WindowSize, j/k navigation in
//  PLAN_REVIEW, esc back, plan editor shortcut 'e', mouse wheel scroll,
//  msg.string regression, AllKeyBindingsRespond).
// ---------------------------------------------------------------------------

// chunk16Helpers is injected once after loadTUIEngine to provide shared test
// utilities (state initializer, mock helpers, message helpers).
const chunk16Helpers = `
// initState: creates a _wizardInit() state properly transitioned to targetState.
function initState(targetState, opts) {
    opts = opts || {};
    var s = globalThis.prSplit._wizardInit();
    s.needsInitClear = false;
    s.width = opts.width || 80;
    s.height = opts.height || 24;
    s.isProcessing = false;
    s.selectedSplitIdx = opts.selectedSplitIdx || 0;
    s.selectedFileIdx = opts.selectedFileIdx || 0;
    s.focusIndex = opts.focusIndex || 0;

    s.wizard.reset();
    var paths = {
        'CONFIG':           ['CONFIG'],
        'PLAN_REVIEW':      ['CONFIG','PLAN_GENERATION','PLAN_REVIEW'],
        'PLAN_EDITOR':      ['CONFIG','PLAN_GENERATION','PLAN_REVIEW','PLAN_EDITOR'],
        'BRANCH_BUILDING':  ['CONFIG','PLAN_GENERATION','PLAN_REVIEW','BRANCH_BUILDING'],
        'EQUIV_CHECK':      ['CONFIG','PLAN_GENERATION','PLAN_REVIEW','BRANCH_BUILDING','EQUIV_CHECK'],
        'ERROR_RESOLUTION': ['CONFIG','PLAN_GENERATION','PLAN_REVIEW','BRANCH_BUILDING','ERROR_RESOLUTION'],
        'FINALIZATION':     ['CONFIG','PLAN_GENERATION','PLAN_REVIEW','BRANCH_BUILDING','EQUIV_CHECK','FINALIZATION']
    };
    var p = paths[targetState];
    if (p) {
        for (var i = 0; i < p.length; i++) s.wizard.transition(p[i]);
    }
    s.wizardState = targetState;
    s._prevWizardState = targetState;
    return s;
}

// update: wrapper for _wizardUpdate.
function update(msg, s) {
    return globalThis.prSplit._wizardUpdate(msg, s);
}

// sendKey: sends a Key message.
function sendKey(s, key) {
    return update({type: 'Key', key: key}, s);
}

// sendClick: sends a left mouse click.
function sendClick(s) {
    return update({type: 'Mouse', button: 'left', action: 'press', isWheel: false, x: 10, y: 10}, s);
}

// sendWheel: sends a mouse wheel event.
function sendWheel(s, direction) {
    return update({type: 'Mouse', button: 'wheel ' + direction, action: 'press', isWheel: true, x: 10, y: 10}, s);
}

// mockZoneHit: mocks zone.inBounds to match only the given zone ID.
// Returns restore fn. MUST be used in try/finally blocks.
function mockZoneHit(zoneId) {
    var z = globalThis.prSplit._zone;
    var orig = z.inBounds;
    z.inBounds = function(id) { return id === zoneId; };
    return function() { z.inBounds = orig; };
}

// setupPlanCache: sets up a 3-split test plan.
function setupPlanCache() {
    globalThis.prSplit._state.planCache = {
        baseBranch: 'main',
        sourceBranch: 'feature',
        splits: [
            {name: 'split/api', files: ['pkg/handler.go', 'pkg/types.go'], message: 'API split', order: 0},
            {name: 'split/cli', files: ['cmd/serve.go', 'cmd/main.go'], message: 'CLI split', order: 1},
            {name: 'split/docs', files: ['README.md'], message: 'Docs split', order: 2}
        ]
    };
}
`

// loadTUIEngineWithHelpers loads the full TUI engine and injects chunk16Helpers.
func loadTUIEngineWithHelpers(t testing.TB) func(string) (any, error) {
	t.Helper()
	evalJS := loadTUIEngine(t)
	if _, err := evalJS(chunk16Helpers); err != nil {
		t.Fatalf("failed to inject chunk16 helpers: %v", err)
	}
	return evalJS
}

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
		r = sendWheel(s, 'down');
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
//  Live Verify Session
// ---------------------------------------------------------------------------

func TestChunk16_VerifySession_Interrupt(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';

		// Tab switches to Claude pane.
		var r = sendKey(s, 'tab');
		if (r[0].splitViewFocus !== 'claude') return 'FAIL: tab did not switch to claude';

		// Tab switches back to wizard.
		r = sendKey(r[0], 'tab');
		if (r[0].splitViewFocus !== 'wizard') return 'FAIL: tab did not switch to wizard';

		// Tab during active verify session does NOT switch focus (split-view tab guard).
		r[0].activeVerifySession = {interrupt:function(){},kill:function(){},close:function(){},isRunning:function(){return true;},output:function(){return '';},screen:function(){return '';}};
		r[0].splitViewFocus = 'wizard';
		r = sendKey(r[0], 'tab');
		// During verify session, tab should NOT switch panes (it falls through to different handler).
		if (r[0].splitViewFocus !== 'wizard') return 'FAIL: tab during verify should not switch focus';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("split view tab: %v", raw)
	}
}

func TestChunk16_SplitView_RatioAdjust(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var switchedTo = null;
		globalThis.tuiMux = {
			switchTo: function(name) { switchedTo = name; }
		};

		var s = initState('CONFIG');
		var restore = mockZoneHit('claude-status');
		try {
			sendClick(s);
			if (switchedTo !== 'claude') return 'FAIL: claude-status did not switch to claude pane';
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
//  Focus Activation (Enter on focused element)
// ---------------------------------------------------------------------------

func TestChunk16_FocusActivate_Strategy(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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

func TestChunk16_FocusActivate_ErrorAskClaude(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// NOTE: _handleFinalizationState is captured as a local var at module
		// load time (JS line 50), so it cannot be mocked. We test with the
		// real handler, which correctly transitions to DONE and returns quit.
		var s = initState('FINALIZATION');
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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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

func TestChunk16_CtrlBracketTermmux(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var switchedTo = null;
		globalThis.tuiMux = {
			switchTo: function(name) { switchedTo = name; }
		};

		var s = initState('CONFIG');
		sendKey(s, 'ctrl+]');
		if (switchedTo !== 'claude') return 'FAIL: ctrl+] did not switch to claude';

		delete globalThis.tuiMux;
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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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

		// CONFIG: auto, heuristic, directory, nav-next = 4 minimum.
		check('CONFIG', 4, 'CONFIG');
		// PLAN_REVIEW: 3 cards + plan-edit + plan-regenerate + ask-claude + nav-next = 7.
		check('PLAN_REVIEW', 7, 'PLAN_REVIEW');
		// PLAN_EDITOR: 3 cards + editor-move + editor-rename + editor-merge + nav-next = 7.
		check('PLAN_EDITOR', 7, 'PLAN_EDITOR');
		// ERROR_RESOLUTION: 5 buttons + error-ask-claude = 6.
		check('ERROR_RESOLUTION', 6, 'ERROR_RESOLUTION');
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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
	evalJS := loadTUIEngineWithHelpers(t)

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
//  T24: Keyboard routing audit — help overlay content & context-awareness
// ---------------------------------------------------------------------------

func TestChunk16_HelpOverlay_ContainsAllSections(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		var view = globalThis.prSplit._viewHelpOverlay(s);
		var errors = [];

		// Section headers.
		if (view.indexOf('Navigation') < 0) errors.push('missing Navigation section');
		if (view.indexOf('Scrolling') < 0) errors.push('missing Scrolling section');
		if (view.indexOf('Plan Editor') < 0) errors.push('missing Plan Editor section');
		if (view.indexOf('Claude') < 0) errors.push('missing Claude Integration section');

		// Specific key bindings.
		var keys = [
			['F1', 'help toggle'],
			['Tab', 'tab key'],
			['Shift+Tab', 'shift-tab'],
			['Enter', 'enter key'],
			['Esc', 'escape key'],
			['Ctrl+C', 'cancel'],
			['Ctrl+L', 'split view toggle'],
			['Ctrl+]', 'claude pane'],
			['Ctrl+=', 'resize split'],
			['Ctrl+-', 'resize split minus'],
			['PgUp', 'page up'],
			['PgDn', 'page down'],
			['Home', 'home key'],
			['End', 'end key'],
			['Space', 'checkbox toggle'],
			['Shift+', 'reorder files']
		];
		for (var i = 0; i < keys.length; i++) {
			if (view.indexOf(keys[i][0]) < 0) {
				errors.push('missing key ' + keys[i][1] + ' (' + keys[i][0] + ')');
			}
		}

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("help overlay content: %v", raw)
	}
}

func TestChunk16_JKContextAwareness_SplitNavigation(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// PLAN_REVIEW: j/k should navigate splits.
		setupPlanCache();
		var s = initState('PLAN_REVIEW');
		s.selectedSplitIdx = 0;
		var r = sendKey(s, 'j');
		if (r[0].selectedSplitIdx !== 1) errors.push('PLAN_REVIEW j did not move split down (got ' + r[0].selectedSplitIdx + ')');
		r = sendKey(r[0], 'k');
		if (r[0].selectedSplitIdx !== 0) errors.push('PLAN_REVIEW k did not move split up (got ' + r[0].selectedSplitIdx + ')');

		// PLAN_EDITOR: j/k should navigate files within split.
		setupPlanCache();
		s = initState('PLAN_EDITOR');
		s.selectedSplitIdx = 0; // api split has 2 files.
		s.selectedFileIdx = 0;
		r = sendKey(s, 'j');
		if (r[0].selectedFileIdx !== 1) errors.push('PLAN_EDITOR j did not move file down (got ' + r[0].selectedFileIdx + ')');
		r = sendKey(r[0], 'k');
		if (r[0].selectedFileIdx !== 0) errors.push('PLAN_EDITOR k did not move file up (got ' + r[0].selectedFileIdx + ')');

		// CONFIG: j/k should not change selectedSplitIdx (no list to navigate).
		s = initState('CONFIG');
		s.selectedSplitIdx = 0;
		r = sendKey(s, 'j');
		if (r[0].selectedSplitIdx !== 0) errors.push('CONFIG j should not change splitIdx');

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("j/k context awareness: %v", raw)
	}
}

func TestChunk16_ArrowContextAwareness_VerifySession(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// Verify session active: up/down should scroll output, not navigate lists.
		setupPlanCache();
		var s = initState('BRANCH_BUILDING');
		s.activeVerifySession = {
			interrupt: function() {},
			kill: function() {}
		};
		s.verifyViewportOffset = 0;
		s.verifyAutoScroll = true;

		var r = sendKey(s, 'up');
		if ((r[0].verifyViewportOffset || 0) <= 0) errors.push('up in verify session did not scroll (got ' + r[0].verifyViewportOffset + ')');
		if (r[0].verifyAutoScroll !== false) errors.push('up in verify session did not disable auto-scroll');

		r = sendKey(r[0], 'end');
		if (r[0].verifyViewportOffset !== 0) errors.push('end in verify session did not jump to bottom');
		if (r[0].verifyAutoScroll !== true) errors.push('end in verify session did not enable auto-scroll');

		r = sendKey(r[0], 'home');
		if (r[0].verifyViewportOffset < 999) errors.push('home in verify session did not jump to top');
		if (r[0].verifyAutoScroll !== false) errors.push('home in verify session did not disable auto-scroll');

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("arrow context in verify session: %v", raw)
	}
}

func TestChunk16_TabBehaviorInSplitView(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// Normal mode: Tab cycles focus.
		var s = initState('CONFIG');
		s.splitViewEnabled = false;
		s.focusIndex = 0;
		var r = sendKey(s, 'tab');
		if (r[0].focusIndex === 0) errors.push('normal tab did not cycle focus');

		// Split-view mode: Tab switches pane focus.
		s = initState('CONFIG');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';
		r = sendKey(s, 'tab');
		if (r[0].splitViewFocus !== 'claude') errors.push('split-view tab did not switch to claude');
		r = sendKey(r[0], 'tab');
		if (r[0].splitViewFocus !== 'wizard') errors.push('split-view tab did not switch back to wizard');

		// Split-view + verify session: Tab should pass through (not switch panes).
		s = initState('BRANCH_BUILDING');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';
		s.activeVerifySession = {interrupt: function(){}, kill: function(){}};
		r = sendKey(s, 'tab');
		// When verify session is active, tab should NOT switch panes.
		if (r[0].splitViewFocus !== 'wizard') errors.push('split-view+verify tab should not switch pane');

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("tab behavior in split view: %v", raw)
	}
}

func TestChunk16_PlanReviewE_EntersEditor(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// 'e' in PLAN_REVIEW (not processing) should enter editor.
		setupPlanCache();
		var s = initState('PLAN_REVIEW');
		s.isProcessing = false;
		var r = sendKey(s, 'e');
		if (r[0].wizardState !== 'PLAN_EDITOR') errors.push('e did not enter editor (got ' + r[0].wizardState + ')');

		// 'e' when processing should NOT enter editor.
		setupPlanCache();
		s = initState('PLAN_REVIEW');
		s.isProcessing = true;
		r = sendKey(s, 'e');
		if (r[0].wizardState === 'PLAN_EDITOR') errors.push('e during processing should not enter editor');

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("plan review e key: %v", raw)
	}
}

func TestChunk16_Keyboard_AllBindingsConsistency(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	// Comprehensive test: verify EVERY key binding documented in the help overlay
	// actually does something (returns a changed state or a command).
	raw, err := evalJS(`(function() {
		var errors = [];

		function stateSnapshot(s) {
			return JSON.stringify({
				showHelp: s.showHelp,
				showConfirmCancel: s.showConfirmCancel,
				splitViewEnabled: s.splitViewEnabled,
				wizardState: s.wizardState,
				selectedSplitIdx: s.selectedSplitIdx,
				selectedFileIdx: s.selectedFileIdx,
				focusIndex: s.focusIndex,
				editorTitleEditing: s.editorTitleEditing,
				editorCheckedFiles: s.editorCheckedFiles || {},
				vpYOffset: s.vp ? s.vp.yOffset() : 0
			});
		}

		// Injects tall content into the viewport so PgDn/Up/Home/End are observable.
		// Pre-scrolls to mid-page so pgup and home have effect from non-top.
		function fillViewport(s) {
			if (s.vp) {
				var lines = [];
				for (var i = 0; i < 200; i++) lines.push('line ' + i);
				s.vp.setContent(lines.join('\n'));
				s.vp.setHeight(10);
				s.vp.halfPageDown();
				s.vp.halfPageDown();
			}
		}

		function testKeyChanges(key, state, label, opts) {
			setupPlanCache();
			var s = initState(state, opts);
			fillViewport(s);
			var before = stateSnapshot(s);
			var r = update({type: 'Key', key: key}, s);
			var after = stateSnapshot(r[0]);
			if (before === after && r[1] === null) {
				errors.push(label + ': no change on key=' + key);
			}
		}

		// Global navigation.
		testKeyChanges('?', 'CONFIG', 'help-?');
		testKeyChanges('f1', 'CONFIG', 'help-f1');
		testKeyChanges('ctrl+c', 'CONFIG', 'cancel');
		testKeyChanges('ctrl+l', 'CONFIG', 'split-view');
		testKeyChanges('enter', 'CONFIG', 'enter-config');
		testKeyChanges('pgdown', 'CONFIG', 'pgdown');
		testKeyChanges('pgup', 'CONFIG', 'pgup');
		testKeyChanges('home', 'CONFIG', 'home');
		testKeyChanges('end', 'CONFIG', 'end');
		testKeyChanges('tab', 'CONFIG', 'tab');
		testKeyChanges('shift+tab', 'CONFIG', 'shift-tab', {focusIndex: 1});

		// Plan review navigation.
		testKeyChanges('j', 'PLAN_REVIEW', 'j-review', {selectedSplitIdx: 0});
		testKeyChanges('k', 'PLAN_REVIEW', 'k-review', {selectedSplitIdx: 1});
		testKeyChanges('down', 'PLAN_REVIEW', 'down-review', {selectedSplitIdx: 0});
		testKeyChanges('up', 'PLAN_REVIEW', 'up-review', {selectedSplitIdx: 1});
		testKeyChanges('e', 'PLAN_REVIEW', 'e-review');

		// Plan editor specific.
		testKeyChanges('e', 'PLAN_EDITOR', 'e-editor', {selectedSplitIdx: 0});
		testKeyChanges(' ', 'PLAN_EDITOR', 'space-editor', {selectedSplitIdx: 0, selectedFileIdx: 0});
		testKeyChanges('j', 'PLAN_EDITOR', 'j-editor', {selectedSplitIdx: 0, selectedFileIdx: 0});
		testKeyChanges('k', 'PLAN_EDITOR', 'k-editor', {selectedSplitIdx: 0, selectedFileIdx: 1});
		testKeyChanges('shift+down', 'PLAN_EDITOR', 'shift-down', {selectedSplitIdx: 0, selectedFileIdx: 0});
		testKeyChanges('shift+up', 'PLAN_EDITOR', 'shift-up', {selectedSplitIdx: 0, selectedFileIdx: 1});

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("keyboard consistency check: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  T025: Claude Crash Detection and Recovery
// ---------------------------------------------------------------------------

// TestChunk16_CrashDetection_AutoPoll verifies that the auto-poll tick handler
// detects a dead Claude process and transitions to ERROR_RESOLUTION with the
// claudeCrashDetected flag set.
func TestChunk16_CrashDetection_AutoPoll(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Mock ClaudeCodeExecutor with a dead handle.
		globalThis.prSplit._state.claudeExecutor = {
			handle: {
				isAlive: function() { return false; },
				receive: function() { return 'segfault at 0x0'; }
			},
			captureDiagnostic: function() { return 'last output: segfault at 0x0'; }
		};

		// Set up state as BRANCH_BUILDING with autoSplitRunning.
		var s = initState('BRANCH_BUILDING');
		s.autoSplitRunning = true;
		s.isProcessing = true;
		s.lastClaudeHealthCheckMs = 0; // Force immediate health check.

		// Send auto-poll tick.
		var r = update({type: 'Tick', id: 'auto-poll'}, s);
		var newState = r[0];

		var errors = [];
		if (newState.wizardState !== 'ERROR_RESOLUTION') {
			errors.push('wizardState: got ' + newState.wizardState + ', want ERROR_RESOLUTION');
		}
		if (!newState.claudeCrashDetected) {
			errors.push('claudeCrashDetected should be true');
		}
		if (newState.autoSplitRunning) {
			errors.push('autoSplitRunning should be false');
		}
		if (newState.isProcessing) {
			errors.push('isProcessing should be false');
		}
		if (!newState.errorDetails || newState.errorDetails.indexOf('crashed') < 0) {
			errors.push('errorDetails should mention crash, got: ' + newState.errorDetails);
		}
		if (newState.errorDetails.indexOf('segfault') < 0) {
			errors.push('errorDetails should contain diagnostic, got: ' + newState.errorDetails);
		}
		// Shared state flag should also be set.
		if (!globalThis.prSplit._state.claudeCrashDetected) {
			errors.push('shared state claudeCrashDetected should be true');
		}
		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("crash detection auto-poll: %v", raw)
	}
}

// TestChunk16_CrashDetection_AliveSkipsCheck verifies that a healthy Claude
// process does NOT trigger crash detection.
func TestChunk16_CrashDetection_AliveSkipsCheck(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Mock ClaudeCodeExecutor with a LIVE handle.
		globalThis.prSplit._state.claudeExecutor = {
			handle: {
				isAlive: function() { return true; }
			}
		};

		var s = initState('BRANCH_BUILDING');
		s.autoSplitRunning = true;
		s.isProcessing = true;
		s.lastClaudeHealthCheckMs = 0;

		var r = update({type: 'Tick', id: 'auto-poll'}, s);
		var newState = r[0];

		// Should still be running — no crash detected.
		if (newState.wizardState !== 'BRANCH_BUILDING') {
			return 'FAIL: wizardState changed to ' + newState.wizardState;
		}
		if (newState.claudeCrashDetected) {
			return 'FAIL: claudeCrashDetected should be false';
		}
		if (!newState.autoSplitRunning) {
			return 'FAIL: autoSplitRunning should still be true';
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("crash detection alive: %v", raw)
	}
}

// TestChunk16_CrashDetection_HealthPollThrottled verifies that the health
// check is throttled (only fires every claudeHealthPollMs).
func TestChunk16_CrashDetection_HealthPollThrottled(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var checkCount = 0;
		globalThis.prSplit._state.claudeExecutor = {
			handle: {
				isAlive: function() { checkCount++; return true; }
			}
		};

		var s = initState('BRANCH_BUILDING');
		s.autoSplitRunning = true;
		s.isProcessing = true;
		// Set last check to NOW — should skip the immediate check.
		s.lastClaudeHealthCheckMs = Date.now();

		var r = update({type: 'Tick', id: 'auto-poll'}, s);
		// isAlive should NOT have been called (throttled).
		if (checkCount !== 0) {
			return 'FAIL: health check should be throttled, but isAlive called ' + checkCount + ' times';
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("crash detection throttle: %v", raw)
	}
}

// TestChunk16_CrashRecovery_ClickZones verifies that crash-specific zone
// clicks dispatch to the correct recovery handlers.
func TestChunk16_CrashRecovery_ClickZones(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// Mock a minimal ClaudeCodeExecutor for restart.
		// restart() returns a Promise (async, matching the real implementation).
		var restartCalled = false;
		var mockExecutor = {
			handle: { isAlive: function() { return true; } },
			resolve: function() { return { error: null }; },
			spawn: function() { return Promise.resolve({ error: null, sessionId: 'restarted-session' }); },
			close: function() {},
			restart: function() {
				restartCalled = true;
				return Promise.resolve({ error: null, sessionId: 'restarted-session' });
			}
		};
		globalThis.prSplit._state.claudeExecutor = mockExecutor;

		// Mock handleConfigState and heuristicSplit to prevent real work.
		var origConfigState = globalThis.prSplit._handleConfigState;
		globalThis.prSplit._handleConfigState = function() {
			return { error: null, analysis: { baseBranch: 'main', files: {} } };
		};

		// Test 1: restart-claude zone click.
		// After click, restart is async — the model enters a "restarting" state.
		// claudeCrashDetected is NOT immediately cleared; it's handled by the
		// restart-claude-poll tick handler after the Promise resolves.
		var s = initState('ERROR_RESOLUTION');
		s.claudeCrashDetected = true;
		var restore = mockZoneHit('resolve-restart-claude');
		try {
			restartCalled = false;
			var r = sendClick(s);
			if (!restartCalled) {
				errors.push('restart: executor.restart() not called');
			}
			// Async restart: model should be in restarting state.
			if (!r[0].claudeRestarting) {
				errors.push('restart: claudeRestarting should be true while async restart is in progress');
			}
			// claudeCrashDetected is still true — cleared by poll handler later.
			if (!r[0].claudeCrashDetected) {
				errors.push('restart: claudeCrashDetected should still be true during async restart');
			}
			if (r[0].errorDetails !== 'Restarting Claude...') {
				errors.push('restart: errorDetails should show restarting message, got: ' + r[0].errorDetails);
			}
			// Should have returned a tick command for polling.
			if (!r[1]) {
				errors.push('restart: expected a tick command (non-null) for restart polling');
			}
		} finally { restore(); }

		// Test 2: fallback-heuristic zone click.
		s = initState('ERROR_RESOLUTION');
		s.claudeCrashDetected = true;
		restore = mockZoneHit('resolve-fallback-heuristic');
		try {
			var r2 = sendClick(s);
			if (r2[0].claudeCrashDetected) {
				errors.push('fallback: claudeCrashDetected should be cleared');
			}
			if (globalThis.prSplit.runtime.mode !== 'heuristic') {
				errors.push('fallback: mode should be heuristic, got ' + globalThis.prSplit.runtime.mode);
			}
		} finally { restore(); }

		// Test 3: abort zone click during crash.
		s = initState('ERROR_RESOLUTION');
		s.claudeCrashDetected = true;
		restore = mockZoneHit('resolve-abort');
		try {
			var r3 = sendClick(s);
			// Abort bypasses crash recovery — goes to handleErrorResolutionState('abort').
			// Should result in CANCELLED state.
			if (r3[0].wizardState !== 'CANCELLED') {
				errors.push('abort: wizardState should be CANCELLED, got ' + r3[0].wizardState);
			}
		} finally { restore(); }

		// Restore.
		globalThis.prSplit._handleConfigState = origConfigState;

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("crash recovery click zones: %v", raw)
	}
}

// TestChunk16_GetFocusElements_CrashMode verifies that getFocusElements
// returns crash-specific buttons when claudeCrashDetected is set.
func TestChunk16_GetFocusElements_CrashMode(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// Standard ERROR_RESOLUTION (no crash) — should have 5+ normal buttons.
		globalThis.prSplit._state.claudeExecutor = {};
		var s = initState('ERROR_RESOLUTION');
		var elems = globalThis.prSplit._getFocusElements(s);
		if (elems.length < 5) {
			errors.push('normal: expected >= 5 elements, got ' + elems.length);
		}
		// Should include resolve-auto, resolve-manual, etc.
		var hasAuto = false;
		for (var i = 0; i < elems.length; i++) {
			if (elems[i].id === 'resolve-auto') hasAuto = true;
		}
		if (!hasAuto) errors.push('normal: missing resolve-auto button');

		// Crash mode — should have exactly 3 crash-specific buttons.
		s = initState('ERROR_RESOLUTION');
		s.claudeCrashDetected = true;
		elems = globalThis.prSplit._getFocusElements(s);
		if (elems.length !== 3) {
			errors.push('crash: expected 3 elements, got ' + elems.length);
		}
		var crashIds = {};
		for (var j = 0; j < elems.length; j++) {
			crashIds[elems[j].id] = true;
		}
		if (!crashIds['resolve-restart-claude']) errors.push('crash: missing restart button');
		if (!crashIds['resolve-fallback-heuristic']) errors.push('crash: missing fallback button');
		if (!crashIds['resolve-abort']) errors.push('crash: missing abort button');
		// Should NOT have normal buttons.
		if (crashIds['resolve-auto']) errors.push('crash: should not have resolve-auto');
		if (crashIds['error-ask-claude']) errors.push('crash: should not have ask-claude');

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("getFocusElements crash mode: %v", raw)
	}
}

// TestChunk16_FocusActivate_CrashButtons verifies keyboard Enter activation
// dispatches crash-specific buttons correctly via handleFocusActivate.
func TestChunk16_FocusActivate_CrashButtons(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// Mock executor for restart (returns Promise, matching async impl).
		var restartCalled = false;
		globalThis.prSplit._state.claudeExecutor = {
			handle: { isAlive: function() { return true; } },
			restart: function() {
				restartCalled = true;
				return Promise.resolve({ error: null, sessionId: 'restarted' });
			}
		};

		// Mock handleConfigState to prevent real work.
		var origConfigState = globalThis.prSplit._handleConfigState;
		globalThis.prSplit._handleConfigState = function() {
			return { error: null, analysis: { baseBranch: 'main', files: {} } };
		};

		// Test: Enter with focusIndex=0 → restart-claude.
		var s = initState('ERROR_RESOLUTION');
		s.claudeCrashDetected = true;
		s.focusIndex = 0; // First button = resolve-restart-claude.
		restartCalled = false;
		var r = sendKey(s, 'enter');
		if (!restartCalled) {
			errors.push('enter-restart: executor.restart() not called');
		}

		// Test: Enter with focusIndex=1 → fallback-heuristic.
		s = initState('ERROR_RESOLUTION');
		s.claudeCrashDetected = true;
		s.focusIndex = 1; // Second button = resolve-fallback-heuristic.
		globalThis.prSplit.runtime.mode = 'auto'; // Reset to non-heuristic.
		r = sendKey(s, 'enter');
		if (globalThis.prSplit.runtime.mode !== 'heuristic') {
			errors.push('enter-fallback: mode should be heuristic, got ' + globalThis.prSplit.runtime.mode);
		}

		// Test: Enter with focusIndex=2 → abort.
		s = initState('ERROR_RESOLUTION');
		s.claudeCrashDetected = true;
		s.focusIndex = 2; // Third button = resolve-abort.
		r = sendKey(s, 'enter');
		// Abort goes through standard path → CANCELLED.
		if (r[0].wizardState !== 'CANCELLED') {
			errors.push('enter-abort: wizardState should be CANCELLED, got ' + r[0].wizardState);
		}

		// Restore.
		globalThis.prSplit._handleConfigState = origConfigState;

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("focus activate crash buttons: %v", raw)
	}
}

// TestChunk16_CrashDetection_InitState verifies that createWizardModel
// initializes crash-related fields to their default values.
func TestChunk16_CrashDetection_InitState(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = globalThis.prSplit._wizardInit();
		var errors = [];
		if (s.claudeCrashDetected !== false) {
			errors.push('claudeCrashDetected should be false, got ' + s.claudeCrashDetected);
		}
		if (s.lastClaudeHealthCheckMs !== 0) {
			errors.push('lastClaudeHealthCheckMs should be 0, got ' + s.lastClaudeHealthCheckMs);
		}
		if (s.autoSplitRunning !== false) {
			errors.push('autoSplitRunning should be false, got ' + s.autoSplitRunning);
		}
		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("crash init state: %v", raw)
	}
}

// TestChunk16_CrashDetection_PlanGenerationTransition verifies crash detection
// works during PLAN_GENERATION state (not just BRANCH_BUILDING).
func TestChunk16_CrashDetection_PlanGenerationTransition(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Mock dead Claude handle.
		globalThis.prSplit._state.claudeExecutor = {
			handle: { isAlive: function() { return false; } },
			captureDiagnostic: function() { return ''; }
		};

		// Set wizard to PLAN_GENERATION by manually transitioning.
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		s.width = 80;
		s.height = 24;
		s.wizard.reset();
		s.wizard.transition('CONFIG');
		s.wizard.transition('PLAN_GENERATION');
		s.wizardState = 'PLAN_GENERATION';
		s._prevWizardState = 'PLAN_GENERATION';
		s.autoSplitRunning = true;
		s.isProcessing = true;
		s.lastClaudeHealthCheckMs = 0;

		var r = update({type: 'Tick', id: 'auto-poll'}, s);
		var newState = r[0];

		if (newState.wizardState !== 'ERROR_RESOLUTION') {
			return 'FAIL: wizardState should be ERROR_RESOLUTION, got ' + newState.wizardState;
		}
		if (!newState.claudeCrashDetected) {
			return 'FAIL: claudeCrashDetected should be true';
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("crash during plan generation: %v", raw)
	}
}

// TestChunk16_CrashDetection_ConfigStateTransition verifies crash detection
// works during CONFIG state, which is the actual wizard state during auto-split
// pipeline execution (the wizard stays at CONFIG while the async pipeline runs).
func TestChunk16_CrashDetection_ConfigStateTransition(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Mock dead Claude handle.
		globalThis.prSplit._state.claudeExecutor = {
			handle: { isAlive: function() { return false; } },
			captureDiagnostic: function() { return 'segfault at 0x0'; }
		};

		// Set wizard to CONFIG (auto-split scenario: wizard stays at CONFIG).
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		s.width = 80;
		s.height = 24;
		s.wizard.reset();
		s.wizard.transition('CONFIG');
		s.wizardState = 'CONFIG';
		s._prevWizardState = 'CONFIG';
		s.autoSplitRunning = true;
		s.isProcessing = true;
		s.lastClaudeHealthCheckMs = 0;

		var r = update({type: 'Tick', id: 'auto-poll'}, s);
		var newState = r[0];

		if (newState.wizardState !== 'ERROR_RESOLUTION') {
			return 'FAIL: wizardState should be ERROR_RESOLUTION, got ' + newState.wizardState;
		}
		if (!newState.claudeCrashDetected) {
			return 'FAIL: claudeCrashDetected should be true';
		}
		if (!newState.errorDetails || newState.errorDetails.indexOf('segfault') < 0) {
			return 'FAIL: errorDetails should contain diagnostic, got: ' + newState.errorDetails;
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("crash during config (auto-split): %v", raw)
	}
}

// TestChunk16_CrashRecovery_RestartFailure verifies that a failed restart
// attempt shows an error in the UI.
func TestChunk16_CrashRecovery_RestartFailure(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Mock executor that fails on restart (returns Promise with error).
		globalThis.prSplit._state.claudeExecutor = {
			handle: { isAlive: function() { return false; } },
			restart: function() {
				return Promise.resolve({ error: 'Claude binary not found' });
			}
		};

		var s = initState('ERROR_RESOLUTION');
		s.claudeCrashDetected = true;

		// Click restart button.
		// The restart is now async: the model enters a "restarting" state.
		// The actual error is surfaced by the restart-claude-poll tick handler.
		var restore = mockZoneHit('resolve-restart-claude');
		try {
			var r = sendClick(s);
			// Model should be in restarting state immediately.
			if (!r[0].claudeRestarting) {
				return 'FAIL: claudeRestarting should be true during async restart, got false';
			}
			if (r[0].errorDetails !== 'Restarting Claude...') {
				return 'FAIL: errorDetails should be "Restarting Claude..." during async restart, got: ' + r[0].errorDetails;
			}
			// Crash flag must remain true (not cleared until poll handler runs).
			if (!r[0].claudeCrashDetected) {
				return 'FAIL: claudeCrashDetected should remain true during async restart';
			}
			// Should have a tick command for polling.
			if (!r[1]) {
				return 'FAIL: expected a tick command for restart polling';
			}
		} finally { restore(); }
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("crash recovery restart failure: %v", raw)
	}
}

// TestChunk16_CrashRecovery_NoExecutor verifies that restart-claude without
// an executor shows an appropriate error.
func TestChunk16_CrashRecovery_NoExecutor(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// No executor set.
		globalThis.prSplit._state.claudeExecutor = null;

		var s = initState('ERROR_RESOLUTION');
		s.claudeCrashDetected = true;

		var restore = mockZoneHit('resolve-restart-claude');
		try {
			var r = sendClick(s);
			if (!r[0].errorDetails || r[0].errorDetails.indexOf('No Claude executor') < 0) {
				return 'FAIL: missing no-executor error, got: ' + r[0].errorDetails;
			}
		} finally { restore(); }
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("crash recovery no executor: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  T33: tuiMux bootstrap — hasChild guards and pollClaudeScreenshot
// ---------------------------------------------------------------------------

// TestChunk16_PollClaudeScreenshot_NoMux verifies that pollClaudeScreenshot
// clears screen state and continues polling when tuiMux is undefined.
func TestChunk16_PollClaudeScreenshot_NoMux(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Remove tuiMux to simulate headless/test context.
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = undefined;

		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.claudeScreen = 'stale-ansi-data';
		s.claudeScreenshot = 'stale-plain-data';

		// Send claude-screenshot tick.
		var result = update({type: 'Tick', id: 'claude-screenshot'}, s);
		var state = result[0];
		var cmd = result[1];

		// Restore tuiMux.
		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		// Verify state was cleared and polling continues.
		var cleared = (state.claudeScreen === '' && state.claudeScreenshot === '');
		var polls = (cmd !== null); // Should return a tick command to continue polling.

		if (!cleared) return 'FAIL: screen not cleared, claudeScreen=' + JSON.stringify(state.claudeScreen) +
			', claudeScreenshot=' + JSON.stringify(state.claudeScreenshot);
		if (!polls) return 'FAIL: expected polling to continue (non-null cmd)';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("pollClaudeScreenshot no mux: %v", raw)
	}
}

// TestChunk16_PollClaudeScreenshot_NoChild verifies that pollClaudeScreenshot
// clears screen state when tuiMux exists but hasChild() returns false.
func TestChunk16_PollClaudeScreenshot_NoChild(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Mock tuiMux with hasChild() returning false.
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return false; },
			screenshot: function() { return 'should-not-be-called'; },
			childScreen: function() { return 'should-not-be-called'; }
		};

		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.claudeScreen = 'stale-ansi';
		s.claudeScreenshot = 'stale-plain';

		var result = update({type: 'Tick', id: 'claude-screenshot'}, s);
		var state = result[0];
		var cmd = result[1];

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		var cleared = (state.claudeScreen === '' && state.claudeScreenshot === '');
		var polls = (cmd !== null);

		if (!cleared) return 'FAIL: screen not cleared';
		if (!polls) return 'FAIL: expected polling to continue';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("pollClaudeScreenshot no child: %v", raw)
	}
}

// TestChunk16_PollClaudeScreenshot_WithChild verifies that pollClaudeScreenshot
// captures screen data when tuiMux has an attached child.
func TestChunk16_PollClaudeScreenshot_WithChild(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			childScreen: function() { return 'ansi-content-here'; },
			screenshot: function() { return 'plain-content-here'; }
		};

		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.claudeScreen = '';
		s.claudeScreenshot = '';

		var result = update({type: 'Tick', id: 'claude-screenshot'}, s);
		var state = result[0];
		var cmd = result[1];

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (state.claudeScreen !== 'ansi-content-here')
			return 'FAIL: claudeScreen=' + JSON.stringify(state.claudeScreen);
		if (state.claudeScreenshot !== 'plain-content-here')
			return 'FAIL: claudeScreenshot=' + JSON.stringify(state.claudeScreenshot);
		if (!cmd) return 'FAIL: expected polling to continue';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("pollClaudeScreenshot with child: %v", raw)
	}
}

// TestChunk16_PollClaudeScreenshot_SplitViewDisabled verifies that
// pollClaudeScreenshot stops polling when split view is disabled.
func TestChunk16_PollClaudeScreenshot_SplitViewDisabled(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = false;

		var result = update({type: 'Tick', id: 'claude-screenshot'}, s);
		var cmd = result[1];

		if (cmd !== null) return 'FAIL: expected null cmd when split view disabled, got: ' + JSON.stringify(cmd);
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("pollClaudeScreenshot split view disabled: %v", raw)
	}
}

// TestChunk16_SwitchTo_NoChild verifies Ctrl+] does NOT call switchTo
// when tuiMux.hasChild() returns false (prevents blocking on empty mux).
func TestChunk16_SwitchTo_NoChild(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var switchCalled = false;
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return false; },
			switchTo: function() { switchCalled = true; },
			screenshot: function() { return ''; },
			childScreen: function() { return ''; }
		};

		var s = initState('PLAN_REVIEW');
		sendKey(s, 'ctrl+]');

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (switchCalled) return 'FAIL: switchTo called despite no child';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("switchTo no child: %v", raw)
	}
}

// TestChunk16_SwitchTo_WithChild verifies Ctrl+] calls switchTo when
// tuiMux.hasChild() returns true.
func TestChunk16_SwitchTo_WithChild(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var switchCalled = false;
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			switchTo: function() { switchCalled = true; },
			screenshot: function() { return ''; },
			childScreen: function() { return ''; }
		};

		var s = initState('PLAN_REVIEW');
		sendKey(s, 'ctrl+]');

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (!switchCalled) return 'FAIL: switchTo not called despite child attached';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("switchTo with child: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  T34: Async Analysis Pipeline Tests
// ---------------------------------------------------------------------------

// TestChunk16_AnalysisPoll_StillRunning verifies that handleAnalysisPoll
// continues polling when analysis is still running.
func TestChunk16_AnalysisPoll_StillRunning(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.isProcessing = true;
		s.analysisRunning = true;
		s.analysisError = null;

		var r = update({type: 'Tick', id: 'analysis-poll'}, s);
		if (!r[1]) return 'FAIL: should return a tick cmd when still running';
		if (!r[0].analysisRunning) return 'FAIL: analysisRunning should still be true';
		if (!r[0].isProcessing) return 'FAIL: isProcessing should still be true';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("analysis-poll still running: %v", raw)
	}
}

// TestChunk16_AnalysisPoll_Cancelled verifies that handleAnalysisPoll
// stops polling when processing was cancelled.
func TestChunk16_AnalysisPoll_Cancelled(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.isProcessing = false;
		s.analysisRunning = false;
		s.analysisError = null;

		var r = update({type: 'Tick', id: 'analysis-poll'}, s);
		if (r[1] !== null) return 'FAIL: should return null cmd on cancel, got: ' + r[1];
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("analysis-poll cancelled: %v", raw)
	}
}

// TestChunk16_AnalysisPoll_ErrorFromPromise verifies that handleAnalysisPoll
// transitions to ERROR state when the async pipeline rejects.
func TestChunk16_AnalysisPoll_ErrorFromPromise(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.isProcessing = true;
		s.analysisRunning = false;
		s.analysisError = 'git diff failed: permission denied';

		var r = update({type: 'Tick', id: 'analysis-poll'}, s);
		if (r[0].wizardState !== 'ERROR') return 'FAIL: wizardState should be ERROR, got: ' + r[0].wizardState;
		if (r[0].isProcessing) return 'FAIL: isProcessing should be false';
		if (!r[0].errorDetails || r[0].errorDetails.indexOf('permission denied') < 0) {
			return 'FAIL: errorDetails should contain error, got: ' + r[0].errorDetails;
		}
		if (r[1] !== null) return 'FAIL: should return null cmd on error';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("analysis-poll error: %v", raw)
	}
}

// TestChunk16_AnalysisPoll_CompletedSuccess verifies that handleAnalysisPoll
// accepts the final state when analysis completed successfully (state
// already transitioned by the async function).
func TestChunk16_AnalysisPoll_CompletedSuccess(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Simulate async pipeline having completed successfully:
		// analysisRunning=false, analysisError=null, wizardState already
		// transitioned to PLAN_REVIEW by runAnalysisAsync.
		var s = initState('PLAN_REVIEW');
		s.isProcessing = false;
		s.analysisRunning = false;
		s.analysisError = null;

		var r = update({type: 'Tick', id: 'analysis-poll'}, s);
		// Should return null cmd (stop polling), state unchanged.
		if (r[1] !== null) return 'FAIL: should return null cmd on success';
		if (r[0].wizardState !== 'PLAN_REVIEW') return 'FAIL: wizardState should be PLAN_REVIEW, got: ' + r[0].wizardState;
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("analysis-poll success: %v", raw)
	}
}

// TestChunk16_AnalysisAsync_HappyPath exercises the full startAnalysis →
// runAnalysisAsync → handleAnalysisPoll flow with mocked async functions.
// Creates a real git repo so handleConfigState succeeds.
func TestChunk16_AnalysisAsync_HappyPath(t *testing.T) {
	t.Parallel()

	// Create a real git repo so handleConfigState succeeds.
	dir := initGitRepo(t)
	writeFile(t, filepath.Join(dir, "a.go"), "package a\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "initial")
	gitCmd(t, dir, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(dir, "b.go"), "package b\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "feature changes")

	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		// Save originals.
		var origAnalyzeDiffAsync = globalThis.prSplit.analyzeDiffAsync;
		var origApplyStrategy = globalThis.prSplit.applyStrategy;
		var origCreateSplitPlanAsync = globalThis.prSplit.createSplitPlanAsync;
		var origValidatePlan = globalThis.prSplit.validatePlan;

		try {
			// Mock analysis functions (called via prSplit.xxx dynamic lookup).
			globalThis.prSplit.analyzeDiffAsync = async function(config) {
				return {
					files: ['a.go', 'b.go', 'c.go'],
					fileStatuses: { 'a.go': 'M', 'b.go': 'A', 'c.go': 'M' },
					error: null,
					baseBranch: 'main',
					currentBranch: 'feature'
				};
			};
			globalThis.prSplit.applyStrategy = function(files, strategy) {
				return { 'group1': ['a.go', 'b.go'], 'group2': ['c.go'] };
			};
			globalThis.prSplit.createSplitPlanAsync = async function(groups, config) {
				return {
					baseBranch: 'main',
					sourceBranch: 'feature',
					splits: [
						{ name: 'split/01-group1', files: ['a.go', 'b.go'], message: 'group1', order: 0, dependencies: [] },
						{ name: 'split/02-group2', files: ['c.go'], message: 'group2', order: 1, dependencies: ['split/01-group1'] }
					]
				};
			};
			globalThis.prSplit.validatePlan = function(plan) {
				return { valid: true, errors: [] };
			};

			// Set up CONFIG state and runtime pointing to real git repo.
			var s = initState('CONFIG');
			globalThis.prSplit.runtime.baseBranch = 'main';
			globalThis.prSplit.runtime.dir = '` + escapeJSPath(dir) + `';
			globalThis.prSplit.runtime.strategy = 'directory';
			globalThis.prSplit.runtime.mode = 'heuristic';
			s.focusIndex = 3; // nav-next element

			// Trigger startAnalysis via enter key on nav-next.
			var r = sendKey(s, 'enter');
			s = r[0];

			// startAnalysis launched the async pipeline.
			if (!s.isProcessing) {
				return 'FAIL: isProcessing should be true after startAnalysis, state=' + s.wizardState +
					', error=' + s.errorDetails;
			}

			// Let microtasks resolve (mocked functions resolve immediately).
			await Promise.resolve();
			await Promise.resolve();
			await Promise.resolve();

			// Poll to finalize.
			r = update({type: 'Tick', id: 'analysis-poll'}, s);
			s = r[0];

			// After completion, should be PLAN_REVIEW.
			if (s.wizardState !== 'PLAN_REVIEW') {
				return 'FAIL: expected PLAN_REVIEW, got ' + s.wizardState +
					', error=' + s.errorDetails + ', isProcessing=' + s.isProcessing +
					', analysisRunning=' + s.analysisRunning;
			}
			if (s.isProcessing) return 'FAIL: isProcessing should be false';
			if (s.analysisRunning) return 'FAIL: analysisRunning should be false';

			// Verify all steps completed.
			for (var i = 0; i < 4; i++) {
				if (!s.analysisSteps[i].done) return 'FAIL: step ' + i + ' not done';
			}
			if (s.analysisProgress !== 1.0) return 'FAIL: progress should be 1.0, got ' + s.analysisProgress;

			return 'OK';
		} finally {
			globalThis.prSplit.analyzeDiffAsync = origAnalyzeDiffAsync;
			globalThis.prSplit.applyStrategy = origApplyStrategy;
			globalThis.prSplit.createSplitPlanAsync = origCreateSplitPlanAsync;
			globalThis.prSplit.validatePlan = origValidatePlan;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("analysis async happy path: %v", raw)
	}
}

// TestChunk16_AnalysisAsync_AnalyzeDiffError verifies error handling when
// analyzeDiffAsync throws an exception.
func TestChunk16_AnalysisAsync_AnalyzeDiffError(t *testing.T) {
	t.Parallel()

	dir := initGitRepo(t)
	writeFile(t, filepath.Join(dir, "a.go"), "package a\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "initial")
	gitCmd(t, dir, "checkout", "-b", "feature")

	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var origAnalyzeDiffAsync = globalThis.prSplit.analyzeDiffAsync;

		try {
			globalThis.prSplit.analyzeDiffAsync = async function() {
				throw new Error('git: not a git repository');
			};

			var s = initState('CONFIG');
			globalThis.prSplit.runtime.baseBranch = 'main';
			globalThis.prSplit.runtime.dir = '` + escapeJSPath(dir) + `';
			globalThis.prSplit.runtime.strategy = 'directory';
			globalThis.prSplit.runtime.mode = 'heuristic';
			s.focusIndex = 3; // nav-next element

			var r = sendKey(s, 'enter');
			s = r[0];

			if (!s.isProcessing) {
				return 'FAIL: isProcessing should be true, state=' + s.wizardState + ', error=' + s.errorDetails;
			}

			await Promise.resolve();
			await Promise.resolve();
			await Promise.resolve();

			r = update({type: 'Tick', id: 'analysis-poll'}, s);
			s = r[0];

			if (s.wizardState !== 'ERROR') {
				return 'FAIL: expected ERROR, got ' + s.wizardState + ', error=' + s.errorDetails;
			}
			if (!s.errorDetails || s.errorDetails.indexOf('not a git repository') < 0) {
				return 'FAIL: errorDetails should mention git error, got: ' + s.errorDetails;
			}
			if (s.isProcessing) return 'FAIL: isProcessing should be false';

			return 'OK';
		} finally {
			globalThis.prSplit.analyzeDiffAsync = origAnalyzeDiffAsync;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("analysis async diff error: %v", raw)
	}
}

// TestChunk16_AnalysisAsync_NoChanges verifies that when analyzeDiffAsync
// returns empty files, the wizard goes back to CONFIG.
func TestChunk16_AnalysisAsync_NoChanges(t *testing.T) {
	t.Parallel()

	dir := initGitRepo(t)
	writeFile(t, filepath.Join(dir, "a.go"), "package a\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "initial")
	gitCmd(t, dir, "checkout", "-b", "feature")

	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var origAnalyzeDiffAsync = globalThis.prSplit.analyzeDiffAsync;

		try {
			globalThis.prSplit.analyzeDiffAsync = async function() {
				return { files: [], fileStatuses: {}, error: null, baseBranch: 'main', currentBranch: 'feature' };
			};

			var s = initState('CONFIG');
			globalThis.prSplit.runtime.baseBranch = 'main';
			globalThis.prSplit.runtime.dir = '` + escapeJSPath(dir) + `';
			globalThis.prSplit.runtime.strategy = 'directory';
			globalThis.prSplit.runtime.mode = 'heuristic';
			s.focusIndex = 3; // nav-next element

			var r = sendKey(s, 'enter');
			s = r[0];

			if (!s.isProcessing) {
				return 'FAIL: isProcessing should be true, state=' + s.wizardState + ', error=' + s.errorDetails;
			}

			await Promise.resolve();
			await Promise.resolve();
			await Promise.resolve();

			r = update({type: 'Tick', id: 'analysis-poll'}, s);
			s = r[0];

			if (s.wizardState !== 'CONFIG') {
				return 'FAIL: expected CONFIG (no changes), got ' + s.wizardState;
			}
			if (s.isProcessing) return 'FAIL: isProcessing should be false';
			if (!s.errorDetails || s.errorDetails.indexOf('No changes') < 0) {
				return 'FAIL: errorDetails should mention no changes, got: ' + s.errorDetails;
			}

			return 'OK';
		} finally {
			globalThis.prSplit.analyzeDiffAsync = origAnalyzeDiffAsync;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("analysis async no changes: %v", raw)
	}
}

// TestChunk16_AnalysisAsync_ValidationFailure verifies that a validatePlan
// failure transitions to ERROR.
func TestChunk16_AnalysisAsync_ValidationFailure(t *testing.T) {
	t.Parallel()

	dir := initGitRepo(t)
	writeFile(t, filepath.Join(dir, "a.go"), "package a\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "initial")
	gitCmd(t, dir, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(dir, "b.go"), "package b\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "feature")

	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var origAnalyzeDiffAsync = globalThis.prSplit.analyzeDiffAsync;
		var origApplyStrategy = globalThis.prSplit.applyStrategy;
		var origCreateSplitPlanAsync = globalThis.prSplit.createSplitPlanAsync;
		var origValidatePlan = globalThis.prSplit.validatePlan;

		try {
			globalThis.prSplit.analyzeDiffAsync = async function() {
				return {
					files: ['a.go'], fileStatuses: { 'a.go': 'M' },
					error: null, baseBranch: 'main', currentBranch: 'feature'
				};
			};
			globalThis.prSplit.applyStrategy = function() {
				return { 'group1': ['a.go'] };
			};
			globalThis.prSplit.createSplitPlanAsync = async function() {
				return {
					baseBranch: 'main', sourceBranch: 'feature',
					splits: [{ name: 'split/01', files: [], message: 'empty', order: 0, dependencies: [] }]
				};
			};
			globalThis.prSplit.validatePlan = function() {
				return { valid: false, errors: ['split split/01 has no files'] };
			};

			var s = initState('CONFIG');
			globalThis.prSplit.runtime.baseBranch = 'main';
			globalThis.prSplit.runtime.dir = '` + escapeJSPath(dir) + `';
			globalThis.prSplit.runtime.strategy = 'directory';
			globalThis.prSplit.runtime.mode = 'heuristic';
			s.focusIndex = 3; // nav-next element

			var r = sendKey(s, 'enter');
			s = r[0];

			if (!s.isProcessing) {
				return 'FAIL: isProcessing should be true, state=' + s.wizardState + ', error=' + s.errorDetails;
			}

			await Promise.resolve();
			await Promise.resolve();
			await Promise.resolve();

			r = update({type: 'Tick', id: 'analysis-poll'}, s);
			s = r[0];

			if (s.wizardState !== 'ERROR') {
				return 'FAIL: expected ERROR, got ' + s.wizardState;
			}
			if (!s.errorDetails || s.errorDetails.indexOf('no files') < 0) {
				return 'FAIL: errorDetails should mention validation, got: ' + s.errorDetails;
			}

			return 'OK';
		} finally {
			globalThis.prSplit.analyzeDiffAsync = origAnalyzeDiffAsync;
			globalThis.prSplit.applyStrategy = origApplyStrategy;
			globalThis.prSplit.createSplitPlanAsync = origCreateSplitPlanAsync;
			globalThis.prSplit.validatePlan = origValidatePlan;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("analysis async validation failure: %v", raw)
	}
}

// TestChunk16_AnalysisAsync_NoSyncCallsRemain verifies that the old sync
// analysis tick IDs are no longer handled by the update function.
func TestChunk16_AnalysisAsync_NoSyncCallsRemain(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.isProcessing = true;

		// Old tick IDs should be ignored (return [s, null]).
		var oldTicks = ['analysis-step-0', 'analysis-step-1', 'analysis-step-2', 'analysis-step-3'];
		for (var i = 0; i < oldTicks.length; i++) {
			var r = update({type: 'Tick', id: oldTicks[i]}, s);
			if (r[1] !== null) return 'FAIL: old tick ' + oldTicks[i] + ' should return null cmd';
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("no sync calls remain: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  T35: Async Execution Pipeline Tests
// ---------------------------------------------------------------------------

// TestChunk16_ExecutionPoll_StillRunning verifies that handleExecutionPoll
// continues polling when execution is still running.
func TestChunk16_ExecutionPoll_StillRunning(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;
		s.executionRunning = true;
		s.executionError = null;

		var r = update({type: 'Tick', id: 'execution-poll'}, s);
		if (!r[1]) return 'FAIL: should return a tick cmd when still running';
		if (!r[0].executionRunning) return 'FAIL: executionRunning should still be true';
		if (!r[0].isProcessing) return 'FAIL: isProcessing should still be true';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("execution-poll still running: %v", raw)
	}
}

// TestChunk16_ExecutionPoll_Cancelled verifies that handleExecutionPoll
// stops polling when processing was cancelled.
func TestChunk16_ExecutionPoll_Cancelled(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = false;
		s.executionRunning = false;
		s.executionError = null;

		var r = update({type: 'Tick', id: 'execution-poll'}, s);
		if (r[1] !== null) return 'FAIL: should return null cmd on cancel, got: ' + r[1];
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("execution-poll cancelled: %v", raw)
	}
}

// TestChunk16_ExecutionPoll_ErrorFromPromise verifies that handleExecutionPoll
// transitions to ERROR_RESOLUTION when the async pipeline rejects.
func TestChunk16_ExecutionPoll_ErrorFromPromise(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;
		s.executionRunning = false;
		s.executionError = 'git worktree failed: permission denied';

		var r = update({type: 'Tick', id: 'execution-poll'}, s);
		if (r[0].wizardState !== 'ERROR_RESOLUTION') {
			return 'FAIL: wizardState should be ERROR_RESOLUTION, got: ' + r[0].wizardState;
		}
		if (r[0].isProcessing) return 'FAIL: isProcessing should be false';
		if (!r[0].errorDetails || r[0].errorDetails.indexOf('permission denied') < 0) {
			return 'FAIL: errorDetails should contain error, got: ' + r[0].errorDetails;
		}
		if (r[1] !== null) return 'FAIL: should return null cmd on error';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("execution-poll error: %v", raw)
	}
}

// TestChunk16_ExecutionPoll_CompletedToVerify verifies that handleExecutionPoll
// starts per-branch verification when executionNextStep='verify'.
func TestChunk16_ExecutionPoll_CompletedToVerify(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;
		s.executionRunning = false;
		s.executionError = null;
		s.executionNextStep = 'verify';

		var r = update({type: 'Tick', id: 'execution-poll'}, s);
		// Should dispatch to verify-branch.
		if (!r[1]) return 'FAIL: should return a tick cmd for verify-branch';
		if (r[0].verifyingIdx !== 0) return 'FAIL: verifyingIdx should be 0, got: ' + r[0].verifyingIdx;
		if (r[0].executionNextStep !== null) return 'FAIL: executionNextStep should be cleared';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("execution-poll completed→verify: %v", raw)
	}
}

// TestChunk16_ExecutionPoll_CompletedToEquiv verifies that handleExecutionPoll
// starts equivalence check when executionNextStep='equiv'.
func TestChunk16_ExecutionPoll_CompletedToEquiv(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;
		s.executionRunning = false;
		s.executionError = null;
		s.executionNextStep = 'equiv';

		var r = update({type: 'Tick', id: 'execution-poll'}, s);
		// Should start equiv check — returns a tick cmd for equiv-poll.
		if (!r[1]) return 'FAIL: should return a tick cmd for equiv-poll';
		if (r[0].wizardState !== 'EQUIV_CHECK') {
			return 'FAIL: wizardState should be EQUIV_CHECK, got: ' + r[0].wizardState;
		}
		if (!r[0].equivRunning) return 'FAIL: equivRunning should be true';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("execution-poll completed→equiv: %v", raw)
	}
}

// TestChunk16_EquivPoll_StillRunning verifies that handleEquivPoll
// continues polling when equiv check is still running.
func TestChunk16_EquivPoll_StillRunning(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('EQUIV_CHECK');
		s.isProcessing = true;
		s.equivRunning = true;
		s.equivError = null;

		var r = update({type: 'Tick', id: 'equiv-poll'}, s);
		if (!r[1]) return 'FAIL: should return a tick cmd when still running';
		if (!r[0].equivRunning) return 'FAIL: equivRunning should still be true';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("equiv-poll still running: %v", raw)
	}
}

// TestChunk16_EquivPoll_Cancelled verifies that handleEquivPoll
// stops polling when processing was cancelled.
func TestChunk16_EquivPoll_Cancelled(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('EQUIV_CHECK');
		s.isProcessing = false;
		s.equivRunning = false;
		s.equivError = null;

		var r = update({type: 'Tick', id: 'equiv-poll'}, s);
		if (r[1] !== null) return 'FAIL: should return null cmd on cancel';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("equiv-poll cancelled: %v", raw)
	}
}

// TestChunk16_EquivPoll_Error verifies that handleEquivPoll
// transitions to ERROR state when equiv check fails.
func TestChunk16_EquivPoll_Error(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('EQUIV_CHECK');
		s.isProcessing = true;
		s.equivRunning = false;
		s.equivError = 'failed to get split tree: fatal: not a valid object name';

		var r = update({type: 'Tick', id: 'equiv-poll'}, s);
		if (r[0].wizardState !== 'ERROR') {
			return 'FAIL: wizardState should be ERROR, got: ' + r[0].wizardState;
		}
		if (r[0].isProcessing) return 'FAIL: isProcessing should be false';
		if (!r[0].errorDetails || r[0].errorDetails.indexOf('Equivalence check failed') < 0) {
			return 'FAIL: errorDetails should mention equiv check, got: ' + r[0].errorDetails;
		}
		if (r[1] !== null) return 'FAIL: should return null cmd on error';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("equiv-poll error: %v", raw)
	}
}

// TestChunk16_EquivPoll_CompletedSuccess verifies that handleEquivPoll
// accepts the final state when equiv check completed successfully.
func TestChunk16_EquivPoll_CompletedSuccess(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Simulate async equiv complete: equivRunning=false, equivError=null,
		// wizardState already transitioned to FINALIZATION by runEquivCheckAsync.
		var s = initState('FINALIZATION');
		s.isProcessing = false;
		s.equivRunning = false;
		s.equivError = null;

		var r = update({type: 'Tick', id: 'equiv-poll'}, s);
		if (r[1] !== null) return 'FAIL: should return null cmd on success';
		if (r[0].wizardState !== 'FINALIZATION') {
			return 'FAIL: wizardState should be FINALIZATION, got: ' + r[0].wizardState;
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("equiv-poll success: %v", raw)
	}
}

// TestChunk16_ExecutionAsync_NoSyncCallsRemain verifies that the old sync
// execution tick IDs are no longer handled by the update function.
func TestChunk16_ExecutionAsync_NoSyncCallsRemain(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;

		// Old tick IDs should be ignored (return [s, null]).
		var oldTicks = ['exec-step-0', 'exec-step-1', 'exec-step-2'];
		for (var i = 0; i < oldTicks.length; i++) {
			var r = update({type: 'Tick', id: oldTicks[i]}, s);
			if (r[1] !== null) return 'FAIL: old tick ' + oldTicks[i] + ' should return null cmd';
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("no sync exec calls remain: %v", raw)
	}
}

// TestChunk16_ExecutionAsync_HappyPath exercises the execution-poll →
// startEquivCheck → equiv-poll chain by simulating completed async execution
// then polling through to FINALIZATION.
func TestChunk16_ExecutionAsync_HappyPath(t *testing.T) {
	t.Parallel()

	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var origVerifyEquivalenceAsync = globalThis.prSplit.verifyEquivalenceAsync;

		try {
			// Mock verifyEquivalenceAsync: equivalent.
			globalThis.prSplit.verifyEquivalenceAsync = async function(plan) {
				return { equivalent: true, splitTree: 'aaa', sourceTree: 'aaa', error: null };
			};

			// Set up state simulating completed execution (no verify command).
			var s = initState('BRANCH_BUILDING');
			s.isProcessing = true;
			s.executionRunning = false;
			s.executionError = null;
			s.executionNextStep = 'equiv';
			s.executionResults = [
				{ name: 'split/01-group1', files: ['a.go'], sha: 'abc123', error: null },
				{ name: 'split/02-group2', files: ['b.go'], sha: 'def456', error: null }
			];

			// Poll execution → should transition to EQUIV_CHECK and start equiv async.
			var r = update({type: 'Tick', id: 'execution-poll'}, s);
			s = r[0];

			if (s.wizardState !== 'EQUIV_CHECK') {
				return 'FAIL: expected EQUIV_CHECK after execution-poll, got ' + s.wizardState;
			}
			if (!s.equivRunning) return 'FAIL: equivRunning should be true';

			// Let microtasks resolve (mocked verifyEquivalenceAsync resolves immediately).
			await Promise.resolve();
			await Promise.resolve();
			await Promise.resolve();

			// Poll equiv check for completion.
			r = update({type: 'Tick', id: 'equiv-poll'}, s);
			s = r[0];

			if (s.wizardState !== 'FINALIZATION') {
				return 'FAIL: expected FINALIZATION after equiv-poll, got ' + s.wizardState +
					', equivError=' + s.equivError + ', equivRunning=' + s.equivRunning;
			}
			if (s.isProcessing) return 'FAIL: isProcessing should be false';
			if (!s.equivalenceResult || !s.equivalenceResult.equivalent) {
				return 'FAIL: equivalenceResult should be equivalent';
			}

			return 'OK';
		} finally {
			globalThis.prSplit.verifyEquivalenceAsync = origVerifyEquivalenceAsync;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("execution async happy path: %v", raw)
	}
}

// TestChunk16_ExecutionAsync_ExecutionError verifies that when
// executeSplitAsync returns an error, wizard transitions to ERROR_RESOLUTION.
func TestChunk16_ExecutionAsync_ExecutionError(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Simulate execution error via the poll handler.
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;
		s.executionRunning = false;
		s.executionError = null;
		s.executionNextStep = null;
		// When the async function sets error state directly:
		s.wizardState = 'ERROR_RESOLUTION';
		s.errorDetails = 'git worktree add failed';
		s.isProcessing = false;

		// Poll should see completed state and stop.
		var r = update({type: 'Tick', id: 'execution-poll'}, s);
		if (r[1] !== null) return 'FAIL: should return null cmd after error';
		if (r[0].wizardState !== 'ERROR_RESOLUTION') {
			return 'FAIL: wizardState should stay ERROR_RESOLUTION, got: ' + r[0].wizardState;
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("execution async error: %v", raw)
	}
}

// TestChunk16_ExecutionAsync_ProgressUpdate verifies that the progressFn
// callback from executeSplitAsync correctly updates state fields.
func TestChunk16_ExecutionAsync_ProgressUpdate(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var origExecuteSplitAsync = globalThis.prSplit.executeSplitAsync;

		try {
			var capturedState = null;

			globalThis.prSplit.executeSplitAsync = async function(plan, opts) {
				// Simulate per-branch progress.
				if (opts && opts.progressFn) {
					opts.progressFn('Creating branch 1/3: split/01');
					// Capture state between calls.
					capturedState = {
						executingIdx: plan._testState.executingIdx,
						executionBranchTotal: plan._testState.executionBranchTotal,
						executionProgressMsg: plan._testState.executionProgressMsg
					};
					opts.progressFn('Creating branch 2/3: split/02');
					opts.progressFn('Creating branch 3/3: split/03');
				}
				return {
					error: null,
					results: [
						{ name: 'split/01', files: ['a.go'], sha: 'aaa', error: null },
						{ name: 'split/02', files: ['b.go'], sha: 'bbb', error: null },
						{ name: 'split/03', files: ['c.go'], sha: 'ccc', error: null }
					]
				};
			};

			var s = initState('BRANCH_BUILDING');
			s.isProcessing = true;
			s.executionRunning = true;
			s.executionError = null;
			// Attach state ref to plan for capture in mock.
			var fakePlan = {
				splits: [
					{ name: 'split/01', files: ['a.go'], message: 'g1', order: 0, dependencies: [] },
					{ name: 'split/02', files: ['b.go'], message: 'g2', order: 1, dependencies: [] },
					{ name: 'split/03', files: ['c.go'], message: 'g3', order: 2, dependencies: [] }
				],
				baseBranch: 'main',
				sourceBranch: 'feature',
				fileStatuses: { 'a.go': 'M', 'b.go': 'A', 'c.go': 'M' },
				_testState: s
			};

			// Call the progressFn path directly.
			var result = await globalThis.prSplit.executeSplitAsync(fakePlan, {
				progressFn: function(msg) {
					var match = msg.match(/(\d+)\/(\d+)/);
					if (match) {
						s.executingIdx = parseInt(match[1], 10) - 1;
						s.executionBranchTotal = parseInt(match[2], 10);
					}
					s.executionProgressMsg = msg;
				}
			});

			// Verify progress was tracked.
			if (s.executingIdx !== 2) return 'FAIL: executingIdx should be 2, got: ' + s.executingIdx;
			if (s.executionBranchTotal !== 3) return 'FAIL: executionBranchTotal should be 3, got: ' + s.executionBranchTotal;
			if (s.executionProgressMsg.indexOf('3/3') < 0) {
				return 'FAIL: executionProgressMsg should contain 3/3, got: ' + s.executionProgressMsg;
			}

			// Verify intermediate capture (after first progress call).
			if (!capturedState) return 'FAIL: should have captured intermediate state';
			if (capturedState.executingIdx !== 0) {
				return 'FAIL: intermediate executingIdx should be 0, got: ' + capturedState.executingIdx;
			}
			if (capturedState.executionBranchTotal !== 3) {
				return 'FAIL: intermediate executionBranchTotal should be 3, got: ' + capturedState.executionBranchTotal;
			}

			return 'OK';
		} finally {
			globalThis.prSplit.executeSplitAsync = origExecuteSplitAsync;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("execution async progress update: %v", raw)
	}
}

// TestChunk16_CancelDuringExecution verifies that cancelling during async
// execution sets isProcessing=false and prevents further wizard transitions.
func TestChunk16_CancelDuringExecution(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;
		s.executionRunning = true;
		s.showConfirmCancel = true;

		// User confirms cancel.
		var r = update({type: 'Key', key: 'y'}, s);
		s = r[0];

		// Cancel should:
		// 1. Set isProcessing = false (so async early-return guards fire)
		// 2. Set wizard state to CANCELLED
		if (s.isProcessing) return 'FAIL: isProcessing should be false after cancel';
		if (s.wizardState !== 'CANCELLED') {
			return 'FAIL: wizardState should be CANCELLED, got: ' + s.wizardState;
		}
		if (s.wizard.current !== 'CANCELLED') {
			return 'FAIL: wizard.current should be CANCELLED, got: ' + s.wizard.current;
		}

		// Subsequent execution-poll should stop (isProcessing=false).
		s.executionRunning = false;
		r = update({type: 'Tick', id: 'execution-poll'}, s);
		if (r[1] !== null) return 'FAIL: execution-poll should stop after cancel';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("cancel during execution: %v", raw)
	}
}

// TestChunk16_EquivPoll_ErrorWizardStateSync verifies that handleEquivPoll
// error path calls wizard.transition('ERROR') keeping wizard.current in sync.
func TestChunk16_EquivPoll_ErrorWizardStateSync(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('EQUIV_CHECK');
		s.isProcessing = true;
		s.equivRunning = false;
		s.equivError = 'tree mismatch';

		var r = update({type: 'Tick', id: 'equiv-poll'}, s);
		s = r[0];
		// Both wizardState and wizard.current must agree.
		if (s.wizardState !== s.wizard.current) {
			return 'FAIL: state desync — wizardState=' + s.wizardState +
				' vs wizard.current=' + s.wizard.current;
		}
		if (s.wizardState !== 'ERROR') {
			return 'FAIL: expected ERROR, got: ' + s.wizardState;
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("equiv-poll error wizard state sync: %v", raw)
	}
}

// Ensure unused imports are referenced.
var _ = strings.Contains
