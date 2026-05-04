package command

// pr_split_16f_unit_test.go — T430: Unit tests for 16f mouse click handlers.
//
// Covers zone-click handlers in _handleMouseClick and _handleScreenMouseClick
// that lacked mouse-click-specific coverage:
//
//   - CONFIG inline-edit field clicks (3 tests): maxFiles reads runtime value,
//     branchPrefix reads default, verifyCommand reads default
//   - CONFIG dryRun toggle (1 test): toggles prSplit.runtime.dryRun
//   - CONFIG field blur on outside click (1 test): clicking outside cancels editing
//   - Split-view tab clicks (1 test): split-tab-verify
//   - BRANCH_BUILDING verify pause/resume (1 test): pause sets verifyPaused,
//     resume clears it
//   - PAUSED screen buttons (1 test): pause-resume and pause-quit dispatches
//   - PLAN_EDITOR title edit cancellation (1 test): switching split cancels
//     inline title editing
//   - Unrecognized zone fallthrough (1 test): state unchanged when no zone matches
//
// Total: 10 tests. Extends the 22 existing mouse tests in
// pr_split_16_split_mouse_test.go plus T45/T46 tests.

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
// CONFIG: inline-edit field clicks (maxFiles, branchPrefix, verifyCommand)
// ---------------------------------------------------------------------------

func TestChunk16f_ConfigFieldClicks_MaxFiles(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Set runtime.maxFiles to a specific value so we can verify the
		// config-field-editing zone reads it from runtime.
		globalThis.prSplit.runtime.maxFiles = 42;
		var s = initState('CONFIG');
		s.showAdvanced = true; // Advanced fields only visible when expanded.

		var restore = mockZoneHit('config-maxFiles');
		try {
			var r = sendClick(s);
			s = r[0];
			if (s.configFieldEditing !== 'maxFiles') return 'FAIL: editing=' + s.configFieldEditing;
			if (s.configFieldValue !== '42') return 'FAIL: value=' + s.configFieldValue;
		} finally { restore(); }
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("config-maxFiles click: %v", raw)
	}
}

func TestChunk16f_ConfigFieldClicks_BranchPrefix(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// branchPrefix defaults to 'split/' when not set.
		delete globalThis.prSplit.runtime.branchPrefix;
		var s = initState('CONFIG');
		s.showAdvanced = true;

		var restore = mockZoneHit('config-branchPrefix');
		try {
			var r = sendClick(s);
			s = r[0];
			if (s.configFieldEditing !== 'branchPrefix') return 'FAIL: editing=' + s.configFieldEditing;
			if (s.configFieldValue !== 'split/') return 'FAIL: value=' + JSON.stringify(s.configFieldValue);
		} finally { restore(); }
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("config-branchPrefix click: %v", raw)
	}
}

func TestChunk16f_ConfigFieldClicks_VerifyCommand(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		globalThis.prSplit.runtime.verifyCommand = 'make test';
		var s = initState('CONFIG');
		s.showAdvanced = true;

		var restore = mockZoneHit('config-verifyCommand');
		try {
			var r = sendClick(s);
			s = r[0];
			if (s.configFieldEditing !== 'verifyCommand') return 'FAIL: editing=' + s.configFieldEditing;
			if (s.configFieldValue !== 'make test') return 'FAIL: value=' + JSON.stringify(s.configFieldValue);
		} finally { restore(); }
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("config-verifyCommand click: %v", raw)
	}
}

// ---------------------------------------------------------------------------
// CONFIG: dryRun checkbox toggle
// ---------------------------------------------------------------------------

func TestChunk16f_ConfigDryRunToggle(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		globalThis.prSplit.runtime.dryRun = false;
		var s = initState('CONFIG');
		s.showAdvanced = true;

		var restore = mockZoneHit('config-dryRun');
		try {
			// First click: false → true.
			var r = sendClick(s);
			if (!globalThis.prSplit.runtime.dryRun) return 'FAIL: first click did not enable dryRun';
			// Second click: true → false.
			r = sendClick(r[0]);
			if (globalThis.prSplit.runtime.dryRun) return 'FAIL: second click did not disable dryRun';
		} finally { restore(); }
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("config-dryRun toggle: %v", raw)
	}
}

// ---------------------------------------------------------------------------
// CONFIG: field editing blur on outside click
// ---------------------------------------------------------------------------

func TestChunk16f_ConfigFieldBlur(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		// Simulate a field already being edited.
		s.configFieldEditing = 'maxFiles';
		s.configFieldValue = '42';

		// Click on a zone that does NOT match 'config-maxFiles' — should blur.
		// Use strategy-heuristic so it's a recognized zone but different field.
		var restore = mockZoneHit('strategy-heuristic');
		try {
			var r = sendClick(s);
			s = r[0];
			// The blur handler runs first, then strategy-heuristic is processed.
			if (s.configFieldEditing !== null) return 'FAIL: editing not cleared, got=' + s.configFieldEditing;
			if (s.configFieldValue !== '') return 'FAIL: value not cleared, got=' + JSON.stringify(s.configFieldValue);
			// Strategy should still be set.
			if (globalThis.prSplit.runtime.mode !== 'heuristic') return 'FAIL: strategy not set';
		} finally { restore(); }
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("config field blur: %v", raw)
	}
}

// ---------------------------------------------------------------------------
// Split-view tab clicks: split-tab-verify
// ---------------------------------------------------------------------------

func TestChunk16f_SplitTabVerify(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.splitViewEnabled = true;
		s.splitViewTab = 'claude';

		// Click split-tab-verify.
		var restore = mockZoneHit('split-tab-verify');
		try {
			var r = sendClick(s);
			if (r[0].splitViewTab !== 'verify') return 'FAIL: tab should be verify, got ' + r[0].splitViewTab;
		} finally { restore(); }

		// Verify tabs are ignored when splitView is disabled.
		s = initState('CONFIG');
		s.splitViewEnabled = false;
		s.splitViewTab = 'claude';
		restore = mockZoneHit('split-tab-verify');
		try {
			var r = sendClick(s);
			// Tab should be unchanged — zone not processed when split disabled.
			if (r[0].splitViewTab !== 'claude') return 'FAIL: disabled split should not change tab, got ' + r[0].splitViewTab;
		} finally { restore(); }

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("split tab verify: %v", raw)
	}
}

// ---------------------------------------------------------------------------
// BRANCH_BUILDING: verify pause/resume buttons
// ---------------------------------------------------------------------------

func TestChunk16f_VerifyPauseResume(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();

		// Create a mock activeVerifySession with pause/resume methods.
		var pauseCalled = false, resumeCalled = false;
		var mockSession = {
			pause: function() { pauseCalled = true; },
			resume: function() { resumeCalled = true; },
			interrupt: function() {},
			kill: function() {}
		};

		// Test verify-pause.
		var s = initState('BRANCH_BUILDING');
		s.activeVerifySession = mockSession;
		s.verifyPaused = false;
		var restore = mockZoneHit('verify-pause');
		try {
			var r = sendClick(s);
			if (!pauseCalled) return 'FAIL: pause() not called';
			if (!r[0].verifyPaused) return 'FAIL: verifyPaused should be true';
		} finally { restore(); }

		// Test verify-resume.
		pauseCalled = false;
		resumeCalled = false;
		s = initState('BRANCH_BUILDING');
		s.activeVerifySession = mockSession;
		s.verifyPaused = true;
		restore = mockZoneHit('verify-resume');
		try {
			var r = sendClick(s);
			if (!resumeCalled) return 'FAIL: resume() not called';
			if (r[0].verifyPaused) return 'FAIL: verifyPaused should be false';
		} finally { restore(); }

		// Test pause when already paused (no-op).
		pauseCalled = false;
		s = initState('BRANCH_BUILDING');
		s.activeVerifySession = mockSession;
		s.verifyPaused = true;
		restore = mockZoneHit('verify-pause');
		try {
			var r = sendClick(s);
			if (pauseCalled) return 'FAIL: pause() should NOT be called when already paused';
			if (!r[0].verifyPaused) return 'FAIL: verifyPaused should remain true';
		} finally { restore(); }

		// Test resume when not paused (no-op).
		resumeCalled = false;
		s = initState('BRANCH_BUILDING');
		s.activeVerifySession = mockSession;
		s.verifyPaused = false;
		restore = mockZoneHit('verify-resume');
		try {
			var r = sendClick(s);
			if (resumeCalled) return 'FAIL: resume() should NOT be called when not paused';
			if (r[0].verifyPaused) return 'FAIL: verifyPaused should remain false';
		} finally { restore(); }

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("verify pause/resume: %v", raw)
	}
}

// ---------------------------------------------------------------------------
// PAUSED screen: pause-resume and pause-quit buttons
// ---------------------------------------------------------------------------

func TestChunk16f_PausedScreenButtons(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// pause-quit: calls wizard.cancel(), returns tea.quit().
		var s = initState('BRANCH_BUILDING');
		s.wizard.pause(); // Transitions to PAUSED with pausedFrom='BRANCH_BUILDING'.
		s.wizardState = 'PAUSED';

		var restore = mockZoneHit('pause-quit');
		try {
			var r = sendClick(s);
			// wizard.cancel() should have been called, moving to CANCELLED.
			if (r[0].wizardState !== 'CANCELLED') return 'FAIL: quit state=' + r[0].wizardState;
			// Should return a quit command (non-null).
			if (r[1] === null || r[1] === undefined) return 'FAIL: quit should return cmd';
		} finally { restore(); }

		// pause-resume: transitions back to original state.
		s = initState('BRANCH_BUILDING');
		s.wizard.pause();
		s.wizardState = 'PAUSED';
		restore = mockZoneHit('pause-resume');
		try {
			var r = sendClick(s);
			if (r[0].wizardState !== 'BRANCH_BUILDING') return 'FAIL: resume state=' + r[0].wizardState;
		} finally { restore(); }

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("paused screen buttons: %v", raw)
	}
}

// ---------------------------------------------------------------------------
// PLAN_EDITOR: inline title edit cancellation when switching splits
// ---------------------------------------------------------------------------

func TestChunk16f_PlanEditorTitleEditCancel(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();

		// Set up inline title editing on split 0.
		var s = initState('PLAN_EDITOR');
		s.selectedSplitIdx = 0;
		s.editorTitleEditing = true;
		s.editorTitleEditingIdx = 0;
		s.editorTitleText = 'new-name';

		// Click on a different split (1) — should cancel title editing.
		var restore = mockZoneHit('edit-split-1');
		try {
			var r = sendClick(s);
			if (r[0].editorTitleEditing !== false) return 'FAIL: titleEditing not cancelled';
			if (r[0].editorTitleEditingIdx !== -1) return 'FAIL: editingIdx=' + r[0].editorTitleEditingIdx;
			if (r[0].editorTitleText !== '') return 'FAIL: text not cleared: ' + JSON.stringify(r[0].editorTitleText);
			if (r[0].selectedSplitIdx !== 1) return 'FAIL: split not selected';
			if (r[0].selectedFileIdx !== 0) return 'FAIL: file idx not reset';
		} finally { restore(); }

		// Clicking the SAME split should NOT cancel title editing.
		s = initState('PLAN_EDITOR');
		s.selectedSplitIdx = 0;
		s.editorTitleEditing = true;
		s.editorTitleEditingIdx = 0;
		s.editorTitleText = 'new-name';
		restore = mockZoneHit('edit-split-0');
		try {
			var r = sendClick(s);
			// Clicking same split: title editing should be preserved.
			if (r[0].editorTitleEditing !== true) return 'FAIL: same-split should keep editing';
			if (r[0].selectedSplitIdx !== 0) return 'FAIL: same split not selected';
		} finally { restore(); }

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("plan editor title edit cancel: %v", raw)
	}
}

// ---------------------------------------------------------------------------
// Unrecognized zone: state unchanged
// ---------------------------------------------------------------------------

func TestChunk16f_UnrecognizedZoneFallthrough(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		var origState = JSON.stringify({
			wizardState:   s.wizardState,
			showAdvanced:  s.showAdvanced,
			showConfirmCancel: s.showConfirmCancel,
			splitViewTab:  s.splitViewTab,
			configFieldEditing: s.configFieldEditing
		});

		// Mock a zone that doesn't match anything.
		var restore = mockZoneHit('nonexistent-zone-xyz');
		try {
			var r = sendClick(s);
			var afterState = JSON.stringify({
				wizardState:   r[0].wizardState,
				showAdvanced:  r[0].showAdvanced,
				showConfirmCancel: r[0].showConfirmCancel,
				splitViewTab:  r[0].splitViewTab,
				configFieldEditing: r[0].configFieldEditing
			});
			if (origState !== afterState) return 'FAIL: state changed: before=' + origState + ' after=' + afterState;
			// Should return null cmd (no side effects).
			if (r[1] !== null && r[1] !== undefined) return 'FAIL: cmd not null';
		} finally { restore(); }

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("unrecognized zone fallthrough: %v", raw)
	}
}
