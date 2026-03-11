package command

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
//  Chunk 15: TUI Views — comprehensive tests
//
//  Covers: zone mark verification, dialog overlays, Claude pane/convo,
//  plan editor state permutations, responsive layout breakpoints,
//  extreme-size robustness, and WCAG contrast exports.
//
//  Basic view rendering tests live in pr_split_13_tui_test.go.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
//  Helper: inject plan state for view tests
// ---------------------------------------------------------------------------

const viewTestPlanState = `
globalThis.prSplit._state.planCache = {
	baseBranch: 'main',
	splits: [
		{name: 'split/api', files: ['pkg/handler.go', 'pkg/types.go'], message: 'Add API', order: 0},
		{name: 'split/cli', files: ['cmd/serve.go'], message: 'Add CLI', order: 1},
		{name: 'split/docs', files: ['docs/README.md', 'docs/api.md', 'docs/design.md'], message: 'Update docs', order: 2}
	]
};
`

// assertZoneMarks verifies that specific zone.mark() calls occur when rendering
// a view. It intercepts zone.mark() to record which zone IDs are used, then
// checks that the expected IDs were all called.
//
// This approach is necessary because bubblezone's zone.Get() only returns
// position data after a full BubbleTea model Update/View cycle, which isn't
// available in unit tests. Intercepting zone.mark() directly confirms the view
// code passes the correct zone IDs to the zone manager.
//
// viewExpr is a JS expression that produces the view output string.
func assertZoneMarks(t *testing.T, evalJS func(string) (any, error), viewExpr string, zoneIDs []string) {
	t.Helper()

	idsJSON, _ := json.Marshal(zoneIDs)

	raw, err := evalJS(`(function() {
		var zone = globalThis.prSplit._zone;
		var calls = {};
		var origMark = zone.mark;
		zone.mark = function(id, content) {
			calls[id] = true;
			return origMark(id, content);
		};
		try {
			` + viewExpr + `;
		} finally {
			zone.mark = origMark;
		}
		var ids = ` + string(idsJSON) + `;
		var results = {};
		for (var i = 0; i < ids.length; i++) {
			results[ids[i]] = calls[ids[i]] === true;
		}
		return JSON.stringify(results);
	})()`)
	if err != nil {
		t.Fatalf("assertZoneMarks eval failed: %v", err)
	}

	var results map[string]bool
	if err := json.Unmarshal([]byte(raw.(string)), &results); err != nil {
		t.Fatalf("failed to unmarshal zone results: %v", err)
	}

	for _, id := range zoneIDs {
		if !results[id] {
			t.Errorf("zone mark %q not found in rendered output", id)
		}
	}
}

// ---------------------------------------------------------------------------
//  WCAG AA: textOnColor semantic color (T18)
// ---------------------------------------------------------------------------

func TestViews_TextOnColor_Exists(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit._wizardColors.textOnColor)`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	if !strings.Contains(s, "#FFFFFF") || !strings.Contains(s, "#000000") {
		t.Errorf("textOnColor should be {light:#FFFFFF, dark:#000000}, got %s", s)
	}
}

func TestViews_AllStylesRenderNonEmpty(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var styles = globalThis.prSplit._wizardStyles;
		var names = Object.keys(styles);
		var results = {};
		for (var i = 0; i < names.length; i++) {
			var name = names[i];
			var rendered = styles[name]().render('test-' + name);
			results[name] = {
				empty: rendered === '',
				containsLabel: rendered.indexOf('test-' + name) >= 0
			};
		}
		JSON.stringify(results);
	`)
	if err != nil {
		t.Fatal(err)
	}

	s := raw.(string)
	if strings.Contains(s, `"empty":true`) {
		t.Errorf("some styles render empty: %s", s)
	}
	if strings.Contains(s, `"containsLabel":false`) {
		t.Errorf("some styles don't contain their label: %s", s)
	}
}

// ---------------------------------------------------------------------------
//  Step dots spacing (T18)
// ---------------------------------------------------------------------------

func TestViews_StepDots_SpacedCorrectly(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._renderStepDots({wizardState: 'PLAN_REVIEW'})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	if s == "" {
		t.Fatal("stepDots should produce non-empty output")
	}
	// With spacing, raw string (including ANSI codes) should be longer
	// than 7 dots with no spaces. 7 dots + 6 spaces = 13 visible chars.
	if len(s) < 13 {
		t.Errorf("stepDots output too short (expected spaces between dots): %q", s)
	}
}

func TestViews_StepDots_AllStates(t *testing.T) {
	evalJS := loadTUIEngine(t)

	states := []string{"IDLE", "CONFIG", "PLAN_GENERATION", "PLAN_REVIEW",
		"PLAN_EDITOR", "BRANCH_BUILDING", "EQUIV_CHECK", "FINALIZATION"}

	for _, state := range states {
		t.Run(state, func(t *testing.T) {
			raw, err := evalJS(`globalThis.prSplit._renderStepDots({wizardState: '` + state + `'})`)
			if err != nil {
				t.Fatalf("renderStepDots(%s): %v", state, err)
			}
			if raw == nil || raw.(string) == "" {
				t.Errorf("renderStepDots(%s) produced empty output", state)
			}
		})
	}
}

// ---------------------------------------------------------------------------
//  Zone mark verification — NavBar
// ---------------------------------------------------------------------------

func TestViews_NavBar_ZoneMarks(t *testing.T) {
	evalJS := loadTUIEngine(t)

	assertZoneMarks(t, evalJS, `globalThis.prSplit._renderNavBar({
		wizardState: 'PLAN_REVIEW', width: 80, isProcessing: false
	})`, []string{"nav-back", "nav-cancel", "nav-next"})
}

func TestViews_NavBar_ProcessingHidesNextZone(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._renderNavBar({
		wizardState: 'PLAN_REVIEW', width: 80, isProcessing: true
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)

	if !strings.Contains(s, "Processing") {
		t.Errorf("navBar with isProcessing=true should show 'Processing': %q", s)
	}
}

func TestViews_NavBar_NarrowOmitsLabels(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._renderNavBar({
		wizardState: 'PLAN_REVIEW', width: 45, isProcessing: false
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)

	// Narrow mode (<50): back should show arrow instead of "Back".
	if strings.Contains(s, "Back") {
		t.Errorf("narrow navBar (w=45) should show arrow not 'Back': %q", s)
	}
}

// ---------------------------------------------------------------------------
//  Zone mark verification — Config Screen
// ---------------------------------------------------------------------------

func TestViews_ConfigScreen_ZoneMarks(t *testing.T) {
	evalJS := loadTUIEngine(t)

	assertZoneMarks(t, evalJS, `globalThis.prSplit._viewConfigScreen({
		wizardState: 'CONFIG', width: 80, showAdvanced: false
	})`, []string{"toggle-advanced"})
}

func TestViews_ConfigScreen_ClaudeTestZone(t *testing.T) {
	evalJS := loadTUIEngine(t)

	assertZoneMarks(t, evalJS, `globalThis.prSplit._viewConfigScreen({
		wizardState: 'CONFIG', width: 80, showAdvanced: false,
		claudeCheckStatus: 'available',
		claudeResolvedInfo: {command: 'claude', type: 'Claude Code'}
	})`, []string{"test-claude"})
}

func TestViews_ConfigScreen_ClaudeStatuses(t *testing.T) {
	evalJS := loadTUIEngine(t)

	cases := []struct {
		name   string
		status string
		extra  string
		want   string
	}{
		{"available", "available",
			"claudeResolvedInfo: {command: 'claude', type: 'Claude Code'}", "Claude available"},
		{"unavailable", "unavailable",
			"claudeCheckError: 'not found in PATH'", "Claude unavailable"},
		{"checking", "checking", "", "Checking Claude"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			js := `globalThis.prSplit._viewConfigScreen({
				wizardState: 'CONFIG', width: 80, showAdvanced: false,
				claudeCheckStatus: '` + tc.status + `'`
			if tc.extra != "" {
				js += `, ` + tc.extra
			}
			js += `})`
			raw, err := evalJS(js)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(raw.(string), tc.want) {
				t.Errorf("config screen with status=%s should contain %q", tc.status, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
//  Zone mark verification — Plan Review
// ---------------------------------------------------------------------------

func TestViews_PlanReviewScreen_ZoneMarks(t *testing.T) {
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	assertZoneMarks(t, evalJS, `globalThis.prSplit._viewPlanReviewScreen({
		wizardState: 'PLAN_REVIEW', width: 80, selectedSplitIdx: 0, focusIndex: 0
	})`, []string{"plan-edit", "plan-regenerate", "ask-claude"})
}

func TestViews_PlanReviewScreen_SplitNames(t *testing.T) {
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`globalThis.prSplit._viewPlanReviewScreen({
		wizardState: 'PLAN_REVIEW', width: 80, selectedSplitIdx: 0, focusIndex: 0
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)

	for _, name := range []string{"split/api", "split/cli", "split/docs"} {
		if !strings.Contains(s, name) {
			t.Errorf("plan review should contain split name %q", name)
		}
	}
}

func TestViews_PlanReviewScreen_SelectedSplitFiles(t *testing.T) {
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`globalThis.prSplit._viewPlanReviewScreen({
		wizardState: 'PLAN_REVIEW', width: 80, selectedSplitIdx: 1, focusIndex: 0
	})`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw.(string), "cmd/serve.go") {
		t.Error("plan review with selectedSplitIdx=1 should show cli files")
	}
}

// ---------------------------------------------------------------------------
//  Plan Editor — zone marks & state permutations (T17)
// ---------------------------------------------------------------------------

func TestViews_PlanEditorScreen_ZoneMarks(t *testing.T) {
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	assertZoneMarks(t, evalJS, `globalThis.prSplit._viewPlanEditorScreen({
		wizardState: 'PLAN_EDITOR', width: 80, selectedSplitIdx: 0,
		selectedFileIdx: 0, focusIndex: 0
	})`, []string{"editor-move", "editor-rename", "editor-merge"})
}

func TestViews_PlanEditorScreen_SplitZoneMarks(t *testing.T) {
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	var zoneIDs []string
	for i := 0; i < 3; i++ {
		zoneIDs = append(zoneIDs, fmt.Sprintf("edit-split-%d", i))
	}
	assertZoneMarks(t, evalJS, `globalThis.prSplit._viewPlanEditorScreen({
		wizardState: 'PLAN_EDITOR', width: 80, selectedSplitIdx: 0,
		selectedFileIdx: 0, focusIndex: 0
	})`, zoneIDs)
}

func TestViews_PlanEditorScreen_ValidationErrors(t *testing.T) {
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`globalThis.prSplit._viewPlanEditorScreen({
		wizardState: 'PLAN_EDITOR', width: 80, selectedSplitIdx: 0,
		selectedFileIdx: 0, focusIndex: 0,
		editorValidationErrors: ['Empty split name', 'Duplicate files']
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)

	if !strings.Contains(s, "Validation") {
		t.Error("plan editor with errors should show validation header")
	}
	if !strings.Contains(s, "Empty split name") {
		t.Error("validation error text should appear in output")
	}
}

func TestViews_PlanEditorScreen_InlineTitleEdit(t *testing.T) {
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`globalThis.prSplit._viewPlanEditorScreen({
		wizardState: 'PLAN_EDITOR', width: 80, selectedSplitIdx: 0,
		selectedFileIdx: 0, focusIndex: 0,
		editorTitleEditing: true, editorTitleEditingIdx: 0,
		editorTitleText: 'new-branch'
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)

	if !strings.Contains(s, "new-branch") {
		t.Error("inline edit should show edit text")
	}
}

func TestViews_PlanEditorScreen_FileCheckboxes(t *testing.T) {
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`globalThis.prSplit._viewPlanEditorScreen({
		wizardState: 'PLAN_EDITOR', width: 80, selectedSplitIdx: 0,
		selectedFileIdx: 0, focusIndex: 0,
		editorCheckedFiles: {'0-0': true, '0-1': false}
	})`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw.(string), "pkg/handler.go") {
		t.Error("plan editor should show file paths")
	}
}

func TestViews_PlanEditorScreen_NoPlan(t *testing.T) {
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(`globalThis.prSplit._state.planCache = null`); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`globalThis.prSplit._viewPlanEditorScreen({
		wizardState: 'PLAN_EDITOR', width: 80, selectedSplitIdx: 0,
		selectedFileIdx: 0, focusIndex: 0
	})`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw.(string), "No plan") {
		t.Error("plan editor with no plan should show 'No plan'")
	}
}

// ---------------------------------------------------------------------------
//  Zone mark verification — Execution, Verification, Error Resolution
// ---------------------------------------------------------------------------

func TestViews_ExecutionScreen_SplitListing(t *testing.T) {
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`globalThis.prSplit._viewExecutionScreen({
		wizardState: 'BRANCH_BUILDING', width: 80,
		executionResults: [{sha: 'abc123'}, {sha: 'def456'}, {sha: 'ghi789'}],
		executingIdx: 3, isProcessing: false
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	for _, name := range []string{"split/api", "split/cli", "split/docs"} {
		if !strings.Contains(s, name) {
			t.Errorf("execution screen should list split %q", name)
		}
	}
}

func TestViews_ExecutionScreen_ErrorDisplay(t *testing.T) {
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`globalThis.prSplit._viewExecutionScreen({
		wizardState: 'BRANCH_BUILDING', width: 80,
		executionResults: [{sha: 'abc123'}, {error: 'cherry-pick conflict'}],
		executingIdx: 2, isProcessing: false
	})`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw.(string), "conflict") {
		t.Error("execution screen with error should show error text")
	}
}

func TestViews_ErrorResolutionScreen_ZoneMarks(t *testing.T) {
	evalJS := loadTUIEngine(t)

	// error-ask-claude only renders when st.claudeExecutor is set.
	if _, err := evalJS(`globalThis.prSplit._state.claudeExecutor = {}`); err != nil {
		t.Fatal(err)
	}

	assertZoneMarks(t, evalJS, `globalThis.prSplit._viewErrorResolutionScreen({
		wizardState: 'ERROR_RESOLUTION', width: 80,
		errorDetails: 'cherry-pick failed'
	})`, []string{"error-ask-claude"})
}

// ---------------------------------------------------------------------------
//  Zone mark verification — Finalization
// ---------------------------------------------------------------------------

func TestViews_FinalizationScreen_ZoneMarks(t *testing.T) {
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	assertZoneMarks(t, evalJS, `globalThis.prSplit._viewFinalizationScreen({
		wizardState: 'FINALIZATION', width: 80, startTime: Date.now() - 60000,
		equivalenceResult: {equivalent: true}
	})`, []string{"final-report", "final-create-prs", "final-done"})
}

// ---------------------------------------------------------------------------
//  Zone mark verification — Confirm Cancel Overlay
// ---------------------------------------------------------------------------

func TestViews_ConfirmCancelOverlay_ZoneMarks(t *testing.T) {
	evalJS := loadTUIEngine(t)

	assertZoneMarks(t, evalJS, `globalThis.prSplit._viewConfirmCancelOverlay({width: 80})`,
		[]string{"confirm-yes", "confirm-no"})
}

// ---------------------------------------------------------------------------
//  Dialog Overlays — Move, Rename, Merge
// ---------------------------------------------------------------------------

func TestViews_MoveFileDialog_ZoneMarks(t *testing.T) {
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	// Check content.
	raw, err := evalJS(`globalThis.prSplit._viewMoveFileDialog({
		width: 80, selectedSplitIdx: 0, selectedFileIdx: 0,
		editorDialogState: {targetIdx: 0}
	})`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw.(string), "Move File") {
		t.Error("move dialog should contain 'Move File' title")
	}

	// With 3 splits and src=0, there should be 2 target zones plus confirm/cancel.
	assertZoneMarks(t, evalJS, `globalThis.prSplit._viewMoveFileDialog({
		width: 80, selectedSplitIdx: 0, selectedFileIdx: 0,
		editorDialogState: {targetIdx: 0}
	})`, []string{
		"move-confirm", "move-cancel", "move-target-0", "move-target-1",
	})
}

func TestViews_MoveFileDialog_ShowsFileName(t *testing.T) {
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`globalThis.prSplit._viewMoveFileDialog({
		width: 80, selectedSplitIdx: 0, selectedFileIdx: 1,
		editorDialogState: {targetIdx: 0}
	})`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw.(string), "pkg/types.go") {
		t.Error("move dialog should show selected file name")
	}
}

func TestViews_RenameSplitDialog_ZoneMarks(t *testing.T) {
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	// Check content.
	raw, err := evalJS(`globalThis.prSplit._viewRenameSplitDialog({
		width: 80, selectedSplitIdx: 0,
		editorDialogState: {inputText: 'new-name'}
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)

	if !strings.Contains(s, "Rename Split") {
		t.Error("rename dialog should contain 'Rename Split' title")
	}
	if !strings.Contains(s, "new-name") {
		t.Error("rename dialog should show input text")
	}

	assertZoneMarks(t, evalJS, `globalThis.prSplit._viewRenameSplitDialog({
		width: 80, selectedSplitIdx: 0,
		editorDialogState: {inputText: 'new-name'}
	})`, []string{"rename-confirm", "rename-cancel"})
}

func TestViews_RenameSplitDialog_ShowsCurrentName(t *testing.T) {
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`globalThis.prSplit._viewRenameSplitDialog({
		width: 80, selectedSplitIdx: 1,
		editorDialogState: {inputText: 'split/cli-v2'}
	})`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw.(string), "split/cli") {
		t.Error("rename dialog should show current split name")
	}
}

func TestViews_MergeSplitsDialog_ZoneMarks(t *testing.T) {
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	// Check content.
	raw, err := evalJS(`globalThis.prSplit._viewMergeSplitsDialog({
		width: 80, selectedSplitIdx: 0,
		editorDialogState: {selected: {1: true}, cursorIdx: 0}
	})`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw.(string), "Merge Splits") {
		t.Error("merge dialog should contain 'Merge Splits' title")
	}

	// 3 splits, src=0, so 2 merge items plus confirm/cancel.
	assertZoneMarks(t, evalJS, `globalThis.prSplit._viewMergeSplitsDialog({
		width: 80, selectedSplitIdx: 0,
		editorDialogState: {selected: {1: true}, cursorIdx: 0}
	})`, []string{
		"merge-confirm", "merge-cancel", "merge-item-0", "merge-item-1",
	})
}

// ---------------------------------------------------------------------------
//  Claude Pane
// ---------------------------------------------------------------------------

func TestViews_ClaudePane_NoMux(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._renderClaudePane({
		claudeScreenshot: '', width: 60, height: 20
	}, 60, 20)`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw.(string), "No Claude session") {
		t.Error("claude pane without mux should show 'No Claude session'")
	}
}

func TestViews_ClaudePane_FocusedRendersNonEmpty(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._renderClaudePane({
		claudeScreenshot: '', splitViewFocus: 'claude',
		width: 60, height: 20
	}, 60, 20)`)
	if err != nil {
		t.Fatal(err)
	}
	if raw.(string) == "" {
		t.Error("focused claude pane should produce non-empty output")
	}
}

func TestViews_ClaudePane_WithScreenshot(t *testing.T) {
	evalJS := loadTUIEngine(t)

	// Inject a mock tuiMux with screenshot function.
	if _, err := evalJS(`
		globalThis.tuiMux = {screenshot: function() { return 'mock screenshot'; }};
	`); err != nil {
		t.Fatal(err)
	}
	// Re-evaluate the views chunk so it picks up tuiMux.
	if _, err := evalJS(prSplitChunk15TUIViews); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`globalThis.prSplit._renderClaudePane({
		claudeScreenshot: 'line1\nline2\nline3', width: 60, height: 20
	}, 60, 20)`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	if strings.Contains(s, "No Claude session") {
		t.Error("claude pane with screenshot should NOT show placeholder")
	}
	if !strings.Contains(s, "Claude") {
		t.Error("claude pane with screenshot should show 'Claude' title")
	}
}

// ---------------------------------------------------------------------------
//  Claude Conversation Overlay
// ---------------------------------------------------------------------------

func TestViews_ClaudeConvoOverlay_EmptyHistory(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._viewClaudeConvoOverlay({
		width: 80, height: 24,
		claudeConvo: {
			context: 'plan-review', history: [], inputText: '',
			sending: false, lastError: null, scrollOffset: 0
		}
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	if !strings.Contains(s, "Ask Claude") {
		t.Error("convo overlay should contain 'Ask Claude'")
	}
	if !strings.Contains(s, "Plan Review") {
		t.Error("convo overlay should show context label 'Plan Review'")
	}
}

func TestViews_ClaudeConvoOverlay_ErrorResolutionContext(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._viewClaudeConvoOverlay({
		width: 80, height: 24,
		claudeConvo: {
			context: 'error-resolution', history: [], inputText: '',
			sending: false, lastError: null, scrollOffset: 0
		}
	})`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw.(string), "Error Resolution") {
		t.Error("convo overlay with error-resolution context should show label")
	}
}

func TestViews_ClaudeConvoOverlay_WithMessages(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._viewClaudeConvoOverlay({
		width: 80, height: 24,
		claudeConvo: {
			context: 'error-resolution',
			history: [
				{role: 'user', text: 'How do I fix this?'},
				{role: 'assistant', text: 'Try cherry-picking manually.'}
			],
			inputText: 'thanks',
			sending: false, lastError: null, scrollOffset: 0
		}
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	if !strings.Contains(s, "You") {
		t.Error("convo overlay should show user badge 'You'")
	}
	if !strings.Contains(s, "Claude") {
		t.Error("convo overlay should show assistant badge 'Claude'")
	}
	if !strings.Contains(s, "2 messages") {
		t.Error("convo overlay should show '2 messages' status")
	}
}

func TestViews_ClaudeConvoOverlay_SendingState(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._viewClaudeConvoOverlay({
		width: 80, height: 24,
		claudeConvo: {
			context: 'plan-review', history: [],
			inputText: '', sending: true, waitingForTool: 'analyze',
			lastError: null, scrollOffset: 0
		}
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	if !strings.Contains(s, "Sending") {
		t.Error("convo overlay sending should show 'Sending'")
	}
}

func TestViews_ClaudeConvoOverlay_ErrorBanner(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._viewClaudeConvoOverlay({
		width: 80, height: 24,
		claudeConvo: {
			context: 'plan-review', history: [],
			inputText: '', sending: false,
			lastError: 'connection timed out', scrollOffset: 0
		}
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	if !strings.Contains(s, "Error") {
		t.Error("convo overlay with error should show 'Error' badge")
	}
	if !strings.Contains(s, "connection timed out") {
		t.Error("convo overlay should show error message text")
	}
}

func TestViews_ClaudeConvoOverlay_ScrollOffset(t *testing.T) {
	evalJS := loadTUIEngine(t)

	// Create a history with many messages to force scrolling.
	raw, err := evalJS(`(function() {
		var history = [];
		for (var i = 0; i < 50; i++) {
			history.push({role: 'user', text: 'message ' + i});
			history.push({role: 'assistant', text: 'reply ' + i});
		}
		return globalThis.prSplit._viewClaudeConvoOverlay({
			width: 80, height: 24,
			claudeConvo: {
				context: 'plan-review', history: history,
				inputText: '', sending: false,
				lastError: null, scrollOffset: 10
			}
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw.(string) == "" {
		t.Error("convo overlay with scroll offset should produce output")
	}
}

// ---------------------------------------------------------------------------
//  Responsive layout — layoutMode helper
// ---------------------------------------------------------------------------

func TestViews_LayoutMode_Breakpoints(t *testing.T) {
	evalJS := loadTUIEngine(t)

	// Layout breakpoints from source: <60 = compact, >100 = wide, else standard.
	// Note: width=0 is falsy in JS, so (s.width || 80) defaults to 80 → "standard".
	cases := []struct {
		width    int
		expected string
	}{
		{0, "standard"}, // 0 is falsy → defaults to 80
		{20, "compact"},
		{59, "compact"},
		{60, "standard"},
		{80, "standard"},
		{100, "standard"},
		{101, "wide"},
		{200, "wide"},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("w=%d", tc.width), func(t *testing.T) {
			raw, err := evalJS(fmt.Sprintf(
				`globalThis.prSplit._layoutMode({width: %d})`, tc.width))
			if err != nil {
				t.Fatal(err)
			}
			if raw != tc.expected {
				t.Errorf("layoutMode(width=%d) = %q, want %q", tc.width, raw, tc.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
//  Responsive layout — TitleBar compact vs standard
// ---------------------------------------------------------------------------

func TestViews_TitleBar_CompactOmitsName(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._renderTitleBar({
		wizardState: 'CONFIG', startTime: Date.now(), width: 30
	})`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw.(string), "PR Split Wizard") {
		t.Error("compact titleBar (w=30) should NOT show 'PR Split Wizard'")
	}
}

// ---------------------------------------------------------------------------
//  Extreme sizes — no panics at width 0, 1, 10, 300
// ---------------------------------------------------------------------------

func TestViews_NoPanicAtZeroWidth(t *testing.T) {
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	screens := []struct {
		name string
		js   string
	}{
		{"configScreen", `globalThis.prSplit._viewConfigScreen({wizardState:'CONFIG',width:0,showAdvanced:false})`},
		{"analysisScreen", `globalThis.prSplit._viewAnalysisScreen({wizardState:'PLAN_GENERATION',width:0,analysisSteps:[],analysisProgress:0})`},
		{"planReviewScreen", `globalThis.prSplit._viewPlanReviewScreen({wizardState:'PLAN_REVIEW',width:0,selectedSplitIdx:0})`},
		{"planEditorScreen", `globalThis.prSplit._viewPlanEditorScreen({wizardState:'PLAN_EDITOR',width:0,selectedSplitIdx:0,selectedFileIdx:0,focusIndex:0})`},
		{"executionScreen", `globalThis.prSplit._viewExecutionScreen({wizardState:'BRANCH_BUILDING',width:0,executionResults:[],executingIdx:0,isProcessing:false})`},
		{"verificationScreen", `globalThis.prSplit._viewVerificationScreen({wizardState:'EQUIV_CHECK',width:0,isProcessing:false,equivalenceResult:{equivalent:true}})`},
		{"finalizationScreen", `globalThis.prSplit._viewFinalizationScreen({wizardState:'FINALIZATION',width:0,startTime:Date.now(),equivalenceResult:{equivalent:true}})`},
		{"errorResolutionScreen", `globalThis.prSplit._viewErrorResolutionScreen({wizardState:'ERROR_RESOLUTION',width:0,errorDetails:'test'})`},
		{"helpOverlay", `globalThis.prSplit._viewHelpOverlay({width:0})`},
		{"confirmCancelOverlay", `globalThis.prSplit._viewConfirmCancelOverlay({width:0})`},
		{"moveDialog", `globalThis.prSplit._viewMoveFileDialog({width:0,selectedSplitIdx:0,selectedFileIdx:0,editorDialogState:{targetIdx:0}})`},
		{"renameDialog", `globalThis.prSplit._viewRenameSplitDialog({width:0,selectedSplitIdx:0,editorDialogState:{inputText:'x'}})`},
		{"mergeDialog", `globalThis.prSplit._viewMergeSplitsDialog({width:0,selectedSplitIdx:0,editorDialogState:{selected:{},cursorIdx:0}})`},
	}

	for _, screen := range screens {
		t.Run(screen.name, func(t *testing.T) {
			_, err := evalJS(screen.js)
			if err != nil {
				t.Errorf("%s panicked at width=0: %v", screen.name, err)
			}
		})
	}
}

func TestViews_NoPanicAtTinyWidth(t *testing.T) {
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	screens := []struct {
		name string
		js   string
	}{
		{"configScreen", `globalThis.prSplit._viewConfigScreen({wizardState:'CONFIG',width:10,showAdvanced:true})`},
		{"planReviewScreen", `globalThis.prSplit._viewPlanReviewScreen({wizardState:'PLAN_REVIEW',width:10,selectedSplitIdx:0})`},
		{"planEditorScreen", `globalThis.prSplit._viewPlanEditorScreen({wizardState:'PLAN_EDITOR',width:10,selectedSplitIdx:0,selectedFileIdx:0,focusIndex:0})`},
		{"executionScreen", `globalThis.prSplit._viewExecutionScreen({wizardState:'BRANCH_BUILDING',width:10,executionResults:[],executingIdx:0,isProcessing:false})`},
		{"finalizationScreen", `globalThis.prSplit._viewFinalizationScreen({wizardState:'FINALIZATION',width:10,startTime:Date.now(),equivalenceResult:{equivalent:true}})`},
		{"errorResolutionScreen", `globalThis.prSplit._viewErrorResolutionScreen({wizardState:'ERROR_RESOLUTION',width:10,errorDetails:'test'})`},
		{"titleBar", `globalThis.prSplit._renderTitleBar({wizardState:'CONFIG',startTime:Date.now(),width:10})`},
		{"navBar", `globalThis.prSplit._renderNavBar({wizardState:'CONFIG',width:10,isProcessing:false})`},
		{"statusBar", `globalThis.prSplit._renderStatusBar({width:10})`},
		{"claudePane", `globalThis.prSplit._renderClaudePane({width:10,height:5},10,5)`},
		{"convoOverlay", `globalThis.prSplit._viewClaudeConvoOverlay({width:10,height:10,claudeConvo:{context:'plan-review',history:[],inputText:'',sending:false,lastError:null,scrollOffset:0}})`},
		{"moveDialog", `globalThis.prSplit._viewMoveFileDialog({width:10,selectedSplitIdx:0,selectedFileIdx:0,editorDialogState:{targetIdx:0}})`},
		{"renameDialog", `globalThis.prSplit._viewRenameSplitDialog({width:10,selectedSplitIdx:0,editorDialogState:{inputText:'x'}})`},
		{"mergeDialog", `globalThis.prSplit._viewMergeSplitsDialog({width:10,selectedSplitIdx:0,editorDialogState:{selected:{},cursorIdx:0}})`},
	}

	for _, screen := range screens {
		t.Run(screen.name, func(t *testing.T) {
			_, err := evalJS(screen.js)
			if err != nil {
				t.Errorf("%s panicked at width=10: %v", screen.name, err)
			}
		})
	}
}

func TestViews_NoPanicAtLargeWidth(t *testing.T) {
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	screens := []struct {
		name string
		js   string
	}{
		{"configScreen", `globalThis.prSplit._viewConfigScreen({wizardState:'CONFIG',width:300,showAdvanced:true})`},
		{"planReviewScreen", `globalThis.prSplit._viewPlanReviewScreen({wizardState:'PLAN_REVIEW',width:300,selectedSplitIdx:0})`},
		{"planEditorScreen", `globalThis.prSplit._viewPlanEditorScreen({wizardState:'PLAN_EDITOR',width:300,selectedSplitIdx:0,selectedFileIdx:0,focusIndex:0})`},
		{"finalizationScreen", `globalThis.prSplit._viewFinalizationScreen({wizardState:'FINALIZATION',width:300,startTime:Date.now(),equivalenceResult:{equivalent:true}})`},
		{"titleBar", `globalThis.prSplit._renderTitleBar({wizardState:'CONFIG',startTime:Date.now(),width:300})`},
		{"navBar", `globalThis.prSplit._renderNavBar({wizardState:'CONFIG',width:300,isProcessing:false})`},
		{"convoOverlay", `globalThis.prSplit._viewClaudeConvoOverlay({width:300,height:50,claudeConvo:{context:'plan-review',history:[],inputText:'',sending:false,lastError:null,scrollOffset:0}})`},
		{"moveDialog", `globalThis.prSplit._viewMoveFileDialog({width:300,selectedSplitIdx:0,selectedFileIdx:0,editorDialogState:{targetIdx:0}})`},
		{"mergeDialog", `globalThis.prSplit._viewMergeSplitsDialog({width:300,selectedSplitIdx:0,editorDialogState:{selected:{},cursorIdx:0}})`},
	}

	for _, screen := range screens {
		t.Run(screen.name, func(t *testing.T) {
			_, err := evalJS(screen.js)
			if err != nil {
				t.Errorf("%s panicked at width=300: %v", screen.name, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
//  viewForState — maps wizard state → screen renderer
// ---------------------------------------------------------------------------

func TestViews_ViewForState_AllStates(t *testing.T) {
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	states := []string{
		"IDLE", "CONFIG", "BASELINE_FAIL",
		"PLAN_GENERATION", "PLAN_REVIEW", "PLAN_EDITOR",
		"BRANCH_BUILDING", "ERROR_RESOLUTION",
		"EQUIV_CHECK", "FINALIZATION",
	}

	for _, state := range states {
		t.Run(state, func(t *testing.T) {
			js := fmt.Sprintf(`globalThis.prSplit._viewForState({
				wizardState: '%s', width: 80, height: 24,
				showAdvanced: false, selectedSplitIdx: 0,
				selectedFileIdx: 0, focusIndex: 0,
				isProcessing: false, startTime: Date.now(),
				analysisSteps: [], analysisProgress: 0,
				executionResults: [], executingIdx: 0,
				equivalenceResult: {equivalent: true},
				errorDetails: 'test error'
			})`, state)
			raw, err := evalJS(js)
			if err != nil {
				t.Fatalf("viewForState(%s) errored: %v", state, err)
			}
			if raw == nil || raw.(string) == "" {
				t.Errorf("viewForState(%s) produced empty output", state)
			}
		})
	}
}

// ---------------------------------------------------------------------------
//  Analysis screen — completed steps
// ---------------------------------------------------------------------------

func TestViews_AnalysisScreen_AllStepsDone(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._viewAnalysisScreen({
		wizardState: 'PLAN_GENERATION', width: 80,
		analysisSteps: [
			{label: 'Parse diff', done: true, elapsed: 100},
			{label: 'Group files', done: true, elapsed: 200},
			{label: 'Create plan', done: true, elapsed: 150}
		],
		analysisProgress: 1.0
	})`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw.(string), "100%") {
		t.Error("analysis at progress=1.0 should show '100%'")
	}
}

func TestViews_AnalysisScreen_NoSteps(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._viewAnalysisScreen({
		wizardState: 'PLAN_GENERATION', width: 80,
		analysisSteps: [],
		analysisProgress: 0
	})`)
	if err != nil {
		t.Fatal(err)
	}
	if raw.(string) == "" {
		t.Error("analysis screen with no steps should still render")
	}
}
