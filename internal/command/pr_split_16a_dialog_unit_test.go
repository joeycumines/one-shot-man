package command

// T422: Unit tests for chunk 16a dialog update handlers (keyboard paths).
//
// Covers:
//   - updateRenameDialog: branch name validation (INVALID_BRANCH_CHARS, "..", ".lock"),
//     character typing, backspace, empty-text close, validation error clearing
//   - updateMoveDialog: j/k navigation clamping, enter file move with splice
//   - updateMergeDialog: space toggle, enter with descending splice, selectedSplitIdx recalc
//   - updateEditorDialog: esc closes any active dialog, unknown dialog no-op

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ──────────────────────────────── Rename Dialog ────────────────────────────────

func TestChunk16a_RenameDialog_ValidName(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// Set up plan with one split, open rename dialog, type a valid name, press enter.
	val, err := evalJS(`
		(function() {
			var st = prSplit._state || {};
			st.planCache = {
				splits: [
					{name: 'old-name', files: ['a.go']},
					{name: 'split-two', files: ['b.go']},
				],
			};
			if (!prSplit._state) prSplit._state = st;

			var s = {
				activeEditorDialog: 'rename',
				editorDialogState: {inputText: 'new-feature'},
				selectedSplitIdx: 0,
			};
			var result = prSplit._updateRenameDialog({type: 'Key', key: 'enter'}, s, s.editorDialogState);
			var newS = result[0];
			return JSON.stringify({
				dialogClosed: newS.activeEditorDialog === null,
				newName: st.planCache.splits[0].name,
				otherName: st.planCache.splits[1].name,
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"dialogClosed":true`) {
		t.Errorf("dialog should be closed: %s", s)
	}
	if !strings.Contains(s, `"newName":"new-feature"`) {
		t.Errorf("split name should be updated: %s", s)
	}
	if !strings.Contains(s, `"otherName":"split-two"`) {
		t.Errorf("other split should be unchanged: %s", s)
	}
}

func TestChunk16a_RenameDialog_InvalidChars(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// Each of these names should produce a validation error on enter.
	val, err := evalJS(`
		(function() {
			var results = [];
			var badNames = [
				'has space',     // whitespace
				'has~tilde',     // tilde
				'has^caret',     // caret
				'has:colon',     // colon
				'has*star',      // asterisk
				'has?question',  // question mark
				'has[bracket',   // open bracket
			];
			for (var i = 0; i < badNames.length; i++) {
				var s = {
					activeEditorDialog: 'rename',
					editorDialogState: {inputText: badNames[i]},
					selectedSplitIdx: 0,
				};
				var result = prSplit._updateRenameDialog({type: 'Key', key: 'enter'}, s, s.editorDialogState);
				var ds = result[0].editorDialogState || {};
				results.push({
					name: badNames[i],
					blocked: result[0].activeEditorDialog === 'rename',
					error: ds.validationError || '',
				});
			}
			return JSON.stringify(results);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	// All should stay in the dialog (blocked) with a validation error.
	for _, name := range []string{
		"has space", "has~tilde", "has^caret", "has:colon",
		"has*star", "has?question", "has[bracket",
	} {
		if !strings.Contains(s, `"blocked":true`) {
			t.Errorf("name %q should be blocked, got: %s", name, s)
		}
	}
	if strings.Contains(s, `"error":""`) {
		t.Errorf("all should have validation errors, got: %s", s)
	}
}

func TestChunk16a_RenameDialog_DoubleDot(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		(function() {
			var s = {
				activeEditorDialog: 'rename',
				editorDialogState: {inputText: 'foo..bar'},
				selectedSplitIdx: 0,
			};
			var result = prSplit._updateRenameDialog({type: 'Key', key: 'enter'}, s, s.editorDialogState);
			var ds = result[0].editorDialogState || {};
			return JSON.stringify({
				blocked: result[0].activeEditorDialog === 'rename',
				error: ds.validationError || '',
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `blocked":true`) {
		t.Errorf("double-dot should be rejected: %s", s)
	}
	if !strings.Contains(s, `contains`) || !strings.Contains(s, `..`) {
		t.Errorf("error should mention '..': %s", s)
	}
}

func TestChunk16a_RenameDialog_DotLockSuffix(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		(function() {
			var s = {
				activeEditorDialog: 'rename',
				editorDialogState: {inputText: 'branch.lock'},
				selectedSplitIdx: 0,
			};
			var result = prSplit._updateRenameDialog({type: 'Key', key: 'enter'}, s, s.editorDialogState);
			var ds = result[0].editorDialogState || {};
			return JSON.stringify({
				blocked: result[0].activeEditorDialog === 'rename',
				error: ds.validationError || '',
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"blocked":true`) {
		t.Errorf(".lock suffix should be rejected: %s", s)
	}
	if !strings.Contains(s, `.lock`) {
		t.Errorf("error should mention '.lock': %s", s)
	}
}

func TestChunk16a_RenameDialog_EmptyTextCloses(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// Empty text on enter should close dialog without renaming.
	val, err := evalJS(`
		(function() {
			var st = prSplit._state || {};
			st.planCache = {
				splits: [{name: 'original', files: ['a.go']}],
			};
			if (!prSplit._state) prSplit._state = st;

			var s = {
				activeEditorDialog: 'rename',
				editorDialogState: {inputText: ''},
				selectedSplitIdx: 0,
			};
			var result = prSplit._updateRenameDialog({type: 'Key', key: 'enter'}, s, s.editorDialogState);
			return JSON.stringify({
				closed: result[0].activeEditorDialog === null,
				nameUnchanged: st.planCache.splits[0].name === 'original',
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"closed":true`) {
		t.Errorf("empty text should close dialog: %s", s)
	}
	if !strings.Contains(s, `"nameUnchanged":true`) {
		t.Errorf("name should remain unchanged: %s", s)
	}
}

func TestChunk16a_RenameDialog_TypingAndBackspace(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		(function() {
			var ds = {inputText: 'abc', validationError: 'some old error'};
			var s = {
				activeEditorDialog: 'rename',
				editorDialogState: ds,
			};

			// Type 'd' — should append and clear validation error.
			var result = prSplit._updateRenameDialog({type: 'Key', key: 'd'}, s, ds);
			var ds1 = result[0].editorDialogState;
			var afterType = ds1.inputText;
			var errorCleared = ds1.validationError === '';

			// Backspace — should remove last char.
			result = prSplit._updateRenameDialog({type: 'Key', key: 'backspace'}, result[0], ds1);
			var ds2 = result[0].editorDialogState;
			var afterBackspace = ds2.inputText;

			return JSON.stringify({
				afterType: afterType,
				errorCleared: errorCleared,
				afterBackspace: afterBackspace,
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"afterType":"abcd"`) {
		t.Errorf("typing should append: %s", s)
	}
	if !strings.Contains(s, `"errorCleared":true`) {
		t.Errorf("typing should clear validation error: %s", s)
	}
	if !strings.Contains(s, `"afterBackspace":"abc"`) {
		t.Errorf("backspace should remove last char: %s", s)
	}
}

// ──────────────────────────────── Move Dialog ────────────────────────────────

func TestChunk16a_MoveDialog_Navigation(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		(function() {
			var st = prSplit._state || {};
			st.planCache = {
				splits: [
					{name: 'A', files: ['x.go', 'y.go']},
					{name: 'B', files: ['z.go']},
					{name: 'C', files: ['w.go']},
				],
			};
			if (!prSplit._state) prSplit._state = st;

			// Selected split 0 (A) — targets are B(idx 1) and C(idx 2).
			var ds = {targetIdx: 0};
			var s = {
				activeEditorDialog: 'move',
				editorDialogState: ds,
				selectedSplitIdx: 0,
				selectedFileIdx: 0,
			};

			// Move down: 0 → 1.
			var result = prSplit._updateMoveDialog({type: 'Key', key: 'j'}, s, ds);
			var idx1 = result[0].editorDialogState.targetIdx;

			// Move down again: should clamp at 1 (only 2 targets).
			ds = result[0].editorDialogState;
			result = prSplit._updateMoveDialog({type: 'Key', key: 'down'}, result[0], ds);
			var idx2 = result[0].editorDialogState.targetIdx;

			// Move up: 1 → 0.
			ds = result[0].editorDialogState;
			result = prSplit._updateMoveDialog({type: 'Key', key: 'k'}, result[0], ds);
			var idx3 = result[0].editorDialogState.targetIdx;

			// Move up again: should clamp at 0.
			ds = result[0].editorDialogState;
			result = prSplit._updateMoveDialog({type: 'Key', key: 'up'}, result[0], ds);
			var idx4 = result[0].editorDialogState.targetIdx;

			return JSON.stringify({idx1: idx1, idx2: idx2, idx3: idx3, idx4: idx4});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"idx1":1`) {
		t.Errorf("j should move down: %s", s)
	}
	if !strings.Contains(s, `"idx2":1`) {
		t.Errorf("down should clamp at max: %s", s)
	}
	if !strings.Contains(s, `"idx3":0`) {
		t.Errorf("k should move up: %s", s)
	}
	if !strings.Contains(s, `"idx4":0`) {
		t.Errorf("up should clamp at 0: %s", s)
	}
}

func TestChunk16a_MoveDialog_ConfirmMoveFile(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		(function() {
			var st = prSplit._state || {};
			st.planCache = {
				splits: [
					{name: 'A', files: ['x.go', 'y.go']},
					{name: 'B', files: ['z.go']},
				],
			};
			if (!prSplit._state) prSplit._state = st;

			// Move first file (x.go) from A to B.
			var ds = {targetIdx: 0};  // targets[0] = split index 1 (B)
			var s = {
				activeEditorDialog: 'move',
				editorDialogState: ds,
				selectedSplitIdx: 0,
				selectedFileIdx: 0,
			};
			var result = prSplit._updateMoveDialog({type: 'Key', key: 'enter'}, s, ds);
			var newS = result[0];

			return JSON.stringify({
				closed: newS.activeEditorDialog === null,
				srcFiles: st.planCache.splits[0].files,
				dstFiles: st.planCache.splits[1].files,
				adjustedFileIdx: newS.selectedFileIdx,
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"closed":true`) {
		t.Errorf("dialog should close: %s", s)
	}
	// A now has only y.go; B has z.go and x.go.
	if !strings.Contains(s, `"srcFiles":["y.go"]`) {
		t.Errorf("source should have y.go only: %s", s)
	}
	if !strings.Contains(s, `"dstFiles":["z.go","x.go"]`) {
		t.Errorf("destination should have z.go and x.go: %s", s)
	}
}

func TestChunk16a_MoveDialog_SingleSplitAutoCloses(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		(function() {
			var st = prSplit._state || {};
			st.planCache = {
				splits: [{name: 'A', files: ['x.go']}],
			};
			if (!prSplit._state) prSplit._state = st;

			var ds = {};
			var s = {
				activeEditorDialog: 'move',
				editorDialogState: ds,
				selectedSplitIdx: 0,
			};
			var result = prSplit._updateMoveDialog({type: 'Key', key: 'j'}, s, ds);
			return JSON.stringify({closed: result[0].activeEditorDialog === null});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(val.(string), `"closed":true`) {
		t.Errorf("single-split should auto-close: %s", val)
	}
}

// ──────────────────────────────── Merge Dialog ────────────────────────────────

func TestChunk16a_MergeDialog_ToggleAndConfirm(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		(function() {
			var st = prSplit._state || {};
			st.planCache = {
				splits: [
					{name: 'A', files: ['a1.go', 'a2.go']},
					{name: 'B', files: ['b1.go']},
					{name: 'C', files: ['c1.go', 'c2.go']},
				],
			};
			if (!prSplit._state) prSplit._state = st;

			// Current split = A (index 0). Mergeables: B(idx 1), C(idx 2).
			var ds = {cursorIdx: 0, selected: {}};
			var s = {
				activeEditorDialog: 'merge',
				editorDialogState: ds,
				selectedSplitIdx: 0,
			};

			// Toggle B (space at cursor 0 → mergeables[0] = index 1).
			var result = prSplit._updateMergeDialog({type: 'Key', key: ' '}, s, ds);
			ds = result[0].editorDialogState;
			var bSelected = ds.selected[1] === true;

			// Move cursor down, toggle C.
			result = prSplit._updateMergeDialog({type: 'Key', key: 'j'}, result[0], ds);
			ds = result[0].editorDialogState;
			result = prSplit._updateMergeDialog({type: 'Key', key: ' '}, result[0], ds);
			ds = result[0].editorDialogState;
			var cSelected = ds.selected[2] === true;

			// Confirm merge: merge B and C into A.
			result = prSplit._updateMergeDialog({type: 'Key', key: 'enter'}, result[0], ds);
			var newS = result[0];

			return JSON.stringify({
				bSelected: bSelected,
				cSelected: cSelected,
				closed: newS.activeEditorDialog === null,
				splitCount: st.planCache.splits.length,
				mergedFiles: st.planCache.splits[0].files,
				mergedName: st.planCache.splits[0].name,
				adjustedIdx: newS.selectedSplitIdx,
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"bSelected":true`) {
		t.Errorf("B should be selected: %s", s)
	}
	if !strings.Contains(s, `"cSelected":true`) {
		t.Errorf("C should be selected: %s", s)
	}
	if !strings.Contains(s, `"closed":true`) {
		t.Errorf("dialog should close: %s", s)
	}
	if !strings.Contains(s, `"splitCount":1`) {
		t.Errorf("should have 1 split after merge: %s", s)
	}
	// A now has all files: original A files, then C (merged first, desc order), then B.
	if !strings.Contains(s, `"mergedFiles":["a1.go","a2.go","c1.go","c2.go","b1.go"]`) {
		t.Errorf("merged files incorrect: %s", s)
	}
	if !strings.Contains(s, `"mergedName":"A"`) {
		t.Errorf("merged split should keep A's name: %s", s)
	}
	if !strings.Contains(s, `"adjustedIdx":0`) {
		t.Errorf("selectedSplitIdx should be 0 after merge: %s", s)
	}
}

func TestChunk16a_MergeDialog_NoSelectionNoOp(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		(function() {
			var st = prSplit._state || {};
			st.planCache = {
				splits: [
					{name: 'A', files: ['a1.go']},
					{name: 'B', files: ['b1.go']},
				],
			};
			if (!prSplit._state) prSplit._state = st;

			var ds = {cursorIdx: 0, selected: {}};
			var s = {
				activeEditorDialog: 'merge',
				editorDialogState: ds,
				selectedSplitIdx: 0,
			};

			// Confirm without selecting anything.
			var result = prSplit._updateMergeDialog({type: 'Key', key: 'enter'}, s, ds);
			return JSON.stringify({
				closed: result[0].activeEditorDialog === null,
				splitCount: st.planCache.splits.length,
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"closed":true`) {
		t.Errorf("dialog should close: %s", s)
	}
	if !strings.Contains(s, `"splitCount":2`) {
		t.Errorf("no splits should be removed: %s", s)
	}
}

func TestChunk16a_MergeDialog_NavigationClamping(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		(function() {
			var st = prSplit._state || {};
			st.planCache = {
				splits: [
					{name: 'A', files: ['a1.go']},
					{name: 'B', files: ['b1.go']},
					{name: 'C', files: ['c1.go']},
				],
			};
			if (!prSplit._state) prSplit._state = st;

			var ds = {cursorIdx: 0, selected: {}};
			var s = {
				activeEditorDialog: 'merge',
				editorDialogState: ds,
				selectedSplitIdx: 0,
			};

			// Up from 0 should stay at 0.
			var result = prSplit._updateMergeDialog({type: 'Key', key: 'up'}, s, ds);
			var idx0 = result[0].editorDialogState.cursorIdx;

			// Down to 1 (max for 2 mergeables).
			ds = result[0].editorDialogState;
			result = prSplit._updateMergeDialog({type: 'Key', key: 'down'}, result[0], ds);
			var idx1 = result[0].editorDialogState.cursorIdx;

			// Down again should clamp at 1.
			ds = result[0].editorDialogState;
			result = prSplit._updateMergeDialog({type: 'Key', key: 'down'}, result[0], ds);
			var idx2 = result[0].editorDialogState.cursorIdx;

			return JSON.stringify({idx0: idx0, idx1: idx1, idx2: idx2});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"idx0":0`) {
		t.Errorf("up from 0 should stay at 0: %s", s)
	}
	if !strings.Contains(s, `"idx1":1`) {
		t.Errorf("down should go to 1: %s", s)
	}
	if !strings.Contains(s, `"idx2":1`) {
		t.Errorf("down should clamp at 1: %s", s)
	}
}

// ──────────────────────────────── Editor Dispatcher ────────────────────────────────

func TestChunk16a_EditorDialog_EscClosesAny(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// Test esc for each dialog type.
	val, err := evalJS(`
		(function() {
			var dialogTypes = ['rename', 'move', 'merge'];
			var results = [];
			for (var i = 0; i < dialogTypes.length; i++) {
				var s = {
					activeEditorDialog: dialogTypes[i],
					editorDialogState: {inputText: 'some-text'},
				};
				var result = prSplit._updateEditorDialog({type: 'Key', key: 'esc'}, s);
				results.push({
					type: dialogTypes[i],
					closed: result[0].activeEditorDialog === null,
					stateCleared: Object.keys(result[0].editorDialogState).length === 0,
				});
			}
			return JSON.stringify(results);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	for _, dtype := range []string{"rename", "move", "merge"} {
		if !strings.Contains(s, `"closed":true`) {
			t.Errorf("esc should close %s dialog: %s", dtype, s)
		}
		if !strings.Contains(s, `"stateCleared":true`) {
			t.Errorf("esc should clear %s state: %s", dtype, s)
		}
	}
}

func TestChunk16a_EditorDialog_UnknownDialogNoOp(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		(function() {
			var s = {
				activeEditorDialog: 'nonexistent',
				editorDialogState: {},
			};
			var result = prSplit._updateEditorDialog({type: 'Key', key: 'enter'}, s);
			return JSON.stringify({
				cmd: result[1],
				dialogStillSet: result[0].activeEditorDialog === 'nonexistent',
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"cmd":null`) {
		t.Errorf("unknown dialog should return null cmd: %s", s)
	}
	if !strings.Contains(s, `"dialogStillSet":true`) {
		t.Errorf("unknown dialog should not modify state: %s", s)
	}
}
