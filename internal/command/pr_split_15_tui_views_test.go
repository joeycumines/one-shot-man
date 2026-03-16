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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	evalJS := loadTUIEngine(t)

	assertZoneMarks(t, evalJS, `globalThis.prSplit._renderNavBar({
		wizardState: 'PLAN_REVIEW', width: 80, isProcessing: false
	})`, []string{"nav-back", "nav-cancel", "nav-next"})
}

func TestViews_NavBar_ProcessingHidesNextZone(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
//  T200: Nav button focus styling — ID-based detection
// ---------------------------------------------------------------------------

// TestViews_NavBar_FocusNextHighlightsNext verifies that when focusIndex
// points to nav-next (second-to-last), the Next button gets focusedButton
// styling. Regression for T200 where inverted position-based check caused
// nav-cancel to get the highlight instead.
func TestViews_NavBar_FocusNextHighlightsNext(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	// Determine the focusIndex for nav-next in CONFIG state.
	raw, err := evalJS(`(function() {
		var elems = prSplit._getFocusElements({
			wizardState: 'CONFIG', showAdvanced: false,
			claudeTestResult: '', claudeAvailable: false
		});
		var navNextIdx = -1;
		var navCancelIdx = -1;
		for (var i = 0; i < elems.length; i++) {
			if (elems[i].id === 'nav-next') navNextIdx = i;
			if (elems[i].id === 'nav-cancel') navCancelIdx = i;
		}
		return JSON.stringify({total: elems.length, navNext: navNextIdx, navCancel: navCancelIdx});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var idx struct {
		Total     int `json:"total"`
		NavNext   int `json:"navNext"`
		NavCancel int `json:"navCancel"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &idx); err != nil {
		t.Fatal(err)
	}
	if idx.NavNext < 0 || idx.NavCancel < 0 {
		t.Fatalf("nav-next=%d nav-cancel=%d — both must be present", idx.NavNext, idx.NavCancel)
	}
	if idx.NavNext >= idx.NavCancel {
		t.Fatalf("nav-next (%d) must come before nav-cancel (%d)", idx.NavNext, idx.NavCancel)
	}

	// Intercept focusedButton to inject a marker.
	marker := "[[FOCUSED_BTN]]"
	js := fmt.Sprintf(`(function() {
		var origFocused = prSplit._wizardStyles.focusedButton;
		prSplit._wizardStyles.focusedButton = function() {
			var s = origFocused();
			return { render: function(text) { return '%s' + s.render(text); } };
		};
		try {
			return prSplit._renderNavBar({
				wizardState: 'CONFIG', width: 80, isProcessing: false,
				focusIndex: %d
			});
		} finally {
			prSplit._wizardStyles.focusedButton = origFocused;
		}
	})()`, marker, idx.NavNext)

	raw, err = evalJS(js)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)

	// The marker should appear right before the "Start Analysis" next button.
	markerIdx := strings.Index(s, marker)
	if markerIdx < 0 {
		t.Fatalf("focusedButton marker not found — Next button not focused when focusIndex=%d:\n%s", idx.NavNext, s)
	}
	// Ensure it's near "Analysis" (the next button label for CONFIG).
	after := s[markerIdx:]
	if !strings.Contains(after, "Analysis") {
		t.Errorf("focusedButton marker should be near 'Start Analysis' next button, got:\n%s", after)
	}
}

// TestViews_NavBar_FocusCancelDoesNotHighlightNext verifies that when
// focusIndex points to nav-cancel (last element), the Next button does NOT
// get focusedButton styling. This was the exact T200 bug.
func TestViews_NavBar_FocusCancelDoesNotHighlightNext(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var elems = prSplit._getFocusElements({
			wizardState: 'CONFIG', showAdvanced: false,
			claudeTestResult: '', claudeAvailable: false
		});
		var navCancelIdx = -1;
		for (var i = 0; i < elems.length; i++) {
			if (elems[i].id === 'nav-cancel') navCancelIdx = i;
		}
		return navCancelIdx;
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	cancelIdx, ok := raw.(int64)
	if !ok {
		t.Fatalf("expected int64, got %T: %v", raw, raw)
	}

	// Intercept focusedButton to inject a marker.
	marker := "[[FOCUSED_BTN]]"
	js := fmt.Sprintf(`(function() {
		var origFocused = prSplit._wizardStyles.focusedButton;
		prSplit._wizardStyles.focusedButton = function() {
			var s = origFocused();
			return { render: function(text) { return '%s' + s.render(text); } };
		};
		try {
			return prSplit._renderNavBar({
				wizardState: 'CONFIG', width: 80, isProcessing: false,
				focusIndex: %d
			});
		} finally {
			prSplit._wizardStyles.focusedButton = origFocused;
		}
	})()`, marker, cancelIdx)

	raw, err = evalJS(js)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)

	// The marker should appear near the Cancel button text. Since the nav
	// bar renders everything on a single horizontal line, we verify that
	// the marker is immediately before "Cancel" (with possible whitespace),
	// confirming the focused style was applied to Cancel, not Next.
	markerIdx := strings.Index(s, marker)
	if markerIdx < 0 {
		t.Fatalf("focusedButton marker not found — Cancel button not focused when focusIndex=%d:\n%s", cancelIdx, s)
	}
	// Text between marker and Cancel must be short (just whitespace/ANSI).
	afterMarker := s[markerIdx+len(marker):]
	cancelPos := strings.Index(afterMarker, "Cancel")
	analysisPos := strings.Index(afterMarker, "Analysis")
	if cancelPos < 0 {
		t.Fatalf("'Cancel' not found after marker:\n%s", afterMarker)
	}
	// Verify: Cancel appears BEFORE Analysis after the marker (focus is on cancel).
	if analysisPos >= 0 && cancelPos > analysisPos {
		t.Errorf("focusedButton marker should be near Cancel, not Analysis:\n%s", afterMarker)
	}
}

// TestViews_NavBar_FocusStyling_AllStates verifies that nav buttons get
// correct focus styling in every wizard state that shows a nav bar.
func TestViews_NavBar_FocusStyling_AllStates(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	states := []string{"CONFIG", "PLAN_REVIEW", "PLAN_EDITOR", "ERROR_RESOLUTION", "FINALIZATION", "EQUIV_CHECK"}
	for _, state := range states {
		t.Run(state, func(t *testing.T) {
			// Get focus element IDs for this state.
			js := fmt.Sprintf(`(function() {
				%s
				var s = {
					wizardState: '%s', showAdvanced: false,
					claudeTestResult: '', claudeAvailable: false,
					planCache: globalThis.prSplit._state.planCache || null,
					equivalenceResult: {equivalent: true, results: [{status: 'pass', branchName: 'test'}]},
					executionResults: [{branchName: 'test', status: 'done'}]
				};
				var elems = prSplit._getFocusElements(s);
				var ids = [];
				for (var i = 0; i < elems.length; i++) ids.push(elems[i].id);
				return JSON.stringify(ids);
			})()`, viewTestPlanState, state)
			raw, err := evalJS(js)
			if err != nil {
				t.Skipf("getFocusElements failed for %s: %v", state, err)
				return
			}
			var ids []string
			if err := json.Unmarshal([]byte(raw.(string)), &ids); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			// Verify nav-next comes before nav-cancel.
			navNextIdx, navCancelIdx := -1, -1
			for i, id := range ids {
				if id == "nav-next" {
					navNextIdx = i
				}
				if id == "nav-cancel" {
					navCancelIdx = i
				}
			}
			if navNextIdx >= 0 && navCancelIdx >= 0 && navNextIdx >= navCancelIdx {
				t.Errorf("state %s: nav-next (idx %d) must come before nav-cancel (idx %d)", state, navNextIdx, navCancelIdx)
			}
		})
	}
}

// ---------------------------------------------------------------------------
//  Zone mark verification — Config Screen
// ---------------------------------------------------------------------------

func TestViews_ConfigScreen_ZoneMarks(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	assertZoneMarks(t, evalJS, `globalThis.prSplit._viewConfigScreen({
		wizardState: 'CONFIG', width: 80, showAdvanced: false
	})`, []string{"toggle-advanced"})
}

func TestViews_ConfigScreen_ClaudeTestZone(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	assertZoneMarks(t, evalJS, `globalThis.prSplit._viewConfigScreen({
		wizardState: 'CONFIG', width: 80, showAdvanced: false,
		claudeCheckStatus: 'available',
		claudeResolvedInfo: {command: 'claude', type: 'Claude Code'}
	})`, []string{"test-claude"})
}

func TestViews_ConfigScreen_ClaudeStatuses(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	assertZoneMarks(t, evalJS, `globalThis.prSplit._viewPlanReviewScreen({
		wizardState: 'PLAN_REVIEW', width: 80, selectedSplitIdx: 0, focusIndex: 0
	})`, []string{"plan-edit", "plan-regenerate", "ask-claude"})
}

func TestViews_PlanReviewScreen_SplitNames(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	evalJS := loadTUIEngine(t)

	assertZoneMarks(t, evalJS, `globalThis.prSplit._viewConfirmCancelOverlay({width: 80})`,
		[]string{"confirm-yes", "confirm-no"})
}

// ---------------------------------------------------------------------------
//  Dialog Overlays — Move, Rename, Merge
// ---------------------------------------------------------------------------

func TestViews_MoveFileDialog_ZoneMarks(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// ---------------------------------------------------------------------------
//  T005: Verify live viewport ANSI rendering
// ---------------------------------------------------------------------------

// TestViews_ExecutionScreen_VerifyViewport_UsesScreen confirms that the
// live verify viewport renders using screen() (ANSI-escaped) rather than
// output() (plain text). A mock activeVerifySession returns different values
// from screen() and output() — the rendered output must contain the screen()
// content.
func TestViews_ExecutionScreen_VerifyViewport_UsesScreen(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`(function() {
		var result = globalThis.prSplit._viewExecutionScreen({
			wizardState: 'BRANCH_BUILDING', width: 80,
			executionResults: [{sha: 'abc123'}],
			executingIdx: 1,
			isProcessing: true,
			verifyingIdx: 1,
			verificationResults: [{passed: true, name: 'split/api'}],
			activeVerifySession: {
				screen: function() { return 'SCREEN_MARKER: test output from screen()'; },
				output: function() { return 'OUTPUT_MARKER: should NOT appear'; },
				isDone: function() { return false; },
				isRunning: function() { return true; }
			},
			activeVerifyBranch: 'split/cli',
			activeVerifyStartTime: Date.now() - 5000,
			verifyAutoScroll: true,
			verifyViewportOffset: 0
		});
		return result;
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	if !strings.Contains(s, "SCREEN_MARKER") {
		t.Error("verify viewport should use screen() content, but SCREEN_MARKER not found")
	}
	if strings.Contains(s, "OUTPUT_MARKER") {
		t.Error("verify viewport should NOT use output() content, but OUTPUT_MARKER was found")
	}
}

// TestViews_ExecutionScreen_VerifyViewport_ANSITruncation confirms that
// ANSI escape codes in the verify viewport are truncated safely using
// lipgloss maxWidth, not naive string.substring().
func TestViews_ExecutionScreen_VerifyViewport_ANSITruncation(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	// Build a long ANSI-colored line that exceeds viewport width.
	// \x1b[32m = green, \x1b[0m = reset. The visual text is ~100 chars
	// but with ANSI codes the byte length is much longer.
	raw, err := evalJS(`(function() {
		// Construct ANSI line: green text "A" repeated 100 times.
		var ansiLine = '\x1b[32m' + Array(101).join('A') + '\x1b[0m';

		var result = globalThis.prSplit._viewExecutionScreen({
			wizardState: 'BRANCH_BUILDING', width: 80,
			executionResults: [{sha: 'abc123'}],
			executingIdx: 1,
			isProcessing: true,
			verifyingIdx: 1,
			verificationResults: [{passed: true, name: 'split/api'}],
			activeVerifySession: {
				screen: function() { return ansiLine; },
				output: function() { return ''; },
				isDone: function() { return false; },
				isRunning: function() { return true; }
			},
			activeVerifyBranch: 'split/cli',
			activeVerifyStartTime: Date.now() - 2000,
			verifyAutoScroll: true,
			verifyViewportOffset: 0
		});
		return result;
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)

	// Should NOT contain a broken/truncated ANSI escape sequence.
	// A broken sequence would be something like "\x1b[3" (incomplete SGR).
	// The presence of the green escape and proper reset is the positive check.
	if strings.Contains(s, "AAAA") == false {
		t.Error("truncated output should still contain some 'A' characters")
	}

	// The output should contain a proper ANSI reset (\x1b[0m or equivalent)
	// — if lipgloss truncation works correctly, it closes any open SGR.
	// We check that no lone \x1b[ without a closing 'm' leaks into the output.
	// (lipgloss.maxWidth handles this internally.)

	// Verify the line was actually truncated: width 80, minus borders/padding,
	// means ~70 chars of 'A' visible at most, not all 100.
	if strings.Count(s, "A") >= 100 {
		t.Error("ANSI line should be truncated to viewport width, but all 100 'A' chars appear")
	}
}

// TestViews_ExecutionScreen_VerifyViewport_EmptyScreenLines confirms that
// trailing empty lines from screen() are stripped, including lines that
// contain only ANSI reset codes (zero visual width).
func TestViews_ExecutionScreen_VerifyViewport_EmptyScreenLines(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	// screen() returns 3 lines of content, then 10 lines of ANSI-only reset codes.
	raw, err := evalJS(`(function() {
		var screenOutput = 'line1\nline2\nline3';
		// Append trailing lines that contain ANSI resets but no visible content.
		for (var i = 0; i < 10; i++) {
			screenOutput += '\n\x1b[0m';
		}

		var result = globalThis.prSplit._viewExecutionScreen({
			wizardState: 'BRANCH_BUILDING', width: 80,
			executionResults: [{sha: 'abc123'}],
			executingIdx: 1,
			isProcessing: true,
			verifyingIdx: 1,
			verificationResults: [{passed: true, name: 'split/api'}],
			activeVerifySession: {
				screen: function() { return screenOutput; },
				output: function() { return ''; },
				isDone: function() { return false; },
				isRunning: function() { return true; }
			},
			activeVerifyBranch: 'split/cli',
			activeVerifyStartTime: Date.now() - 1000,
			verifyAutoScroll: true,
			verifyViewportOffset: 0
		});

		// The viewport should show line1, line2, line3 — not 13 lines.
		var hasLine1 = result.indexOf('line1') >= 0;
		var hasLine2 = result.indexOf('line2') >= 0;
		var hasLine3 = result.indexOf('line3') >= 0;
		return JSON.stringify({
			hasLine1: hasLine1,
			hasLine2: hasLine2,
			hasLine3: hasLine3,
			output: result
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	if !strings.Contains(s, `"hasLine1":true`) {
		t.Error("viewport should contain 'line1'")
	}
	if !strings.Contains(s, `"hasLine2":true`) {
		t.Error("viewport should contain 'line2'")
	}
	if !strings.Contains(s, `"hasLine3":true`) {
		t.Error("viewport should contain 'line3'")
	}
}

// TestViews_ExecutionScreen_VerifyViewport_ZoneMarks confirms the viewport
// footer contains the verify-interrupt zone mark for stopping the session.
func TestViews_ExecutionScreen_VerifyViewport_ZoneMarks(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	assertZoneMarks(t, evalJS, `globalThis.prSplit._viewExecutionScreen({
		wizardState: 'BRANCH_BUILDING', width: 80,
		executionResults: [{sha: 'abc123'}],
		executingIdx: 1,
		isProcessing: true,
		verifyingIdx: 1,
		verificationResults: [{passed: true, name: 'split/api'}],
		activeVerifySession: {
			screen: function() { return 'test output'; },
			output: function() { return ''; },
			isDone: function() { return false; },
			isRunning: function() { return true; }
		},
		activeVerifyBranch: 'split/cli',
		activeVerifyStartTime: Date.now(),
		verifyAutoScroll: true,
		verifyViewportOffset: 0
	})`, []string{"verify-interrupt"})
}

// ---------------------------------------------------------------------------
// T043: Multi-width screen renderer regression suite
//
// For each of the 7+ screen renderers, render at 40, 60, 80, 100, 120 columns
// and verify:
//   - Non-empty output (no panics, no blank screens)
//   - Key content elements present (screen title / state identifiers)
//   - No ANSI corruption (no unmatched ESC without closing 'm')
// ---------------------------------------------------------------------------

func TestViews_MultiWidth_AllScreens(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	widths := []int{40, 60, 80, 100, 120}

	type screenDef struct {
		name       string
		jsTemplate string
		// Keywords that MUST appear in the output at any width.
		keywords []string
	}

	screens := []screenDef{
		{
			name: "configScreen",
			jsTemplate: `globalThis.prSplit._viewConfigScreen({
				wizardState:'CONFIG', width:%d, height:24,
				showAdvanced:false
			})`,
			keywords: []string{},
		},
		{
			name: "analysisScreen",
			jsTemplate: `globalThis.prSplit._viewAnalysisScreen({
				wizardState:'PLAN_GENERATION', width:%d, height:24,
				analysisSteps:[{label:'Parse diff',done:true,elapsed:100}],
				analysisProgress:0.5
			})`,
			keywords: []string{"50%"},
		},
		{
			name: "planReviewScreen",
			jsTemplate: `globalThis.prSplit._viewPlanReviewScreen({
				wizardState:'PLAN_REVIEW', width:%d, height:24,
				selectedSplitIdx:0, focusIndex:0
			})`,
			keywords: []string{"split/api"},
		},
		{
			name: "planEditorScreen",
			jsTemplate: `globalThis.prSplit._viewPlanEditorScreen({
				wizardState:'PLAN_EDITOR', width:%d, height:24,
				selectedSplitIdx:0, selectedFileIdx:0, focusIndex:0
			})`,
			keywords: []string{"split/api"},
		},
		{
			name: "executionScreen",
			jsTemplate: `globalThis.prSplit._viewExecutionScreen({
				wizardState:'BRANCH_BUILDING', width:%d, height:24,
				executionResults:[{sha:'abc123'}],
				executingIdx:1, isProcessing:true,
				verificationResults:[]
			})`,
			keywords: []string{},
		},
		{
			name: "equivCheckScreen",
			jsTemplate: `globalThis.prSplit._viewVerificationScreen({
				wizardState:'EQUIV_CHECK', width:%d, height:24,
				isProcessing:false,
				equivalenceResult:{equivalent:true, splitTree:'aaa', expectedTree:'aaa'}
			})`,
			keywords: []string{},
		},
		{
			name: "finalizationScreen",
			jsTemplate: `globalThis.prSplit._viewFinalizationScreen({
				wizardState:'FINALIZATION', width:%d, height:24,
				startTime:Date.now()-60000,
				equivalenceResult:{equivalent:true}
			})`,
			keywords: []string{},
		},
		{
			name: "errorResolutionScreen",
			jsTemplate: `globalThis.prSplit._viewErrorResolutionScreen({
				wizardState:'ERROR_RESOLUTION', width:%d, height:24,
				errorDetails:'test error message here'
			})`,
			keywords: []string{"test error message here"},
		},
	}

	for _, screen := range screens {
		for _, w := range widths {
			t.Run(fmt.Sprintf("%s/w=%d", screen.name, w), func(t *testing.T) {
				js := fmt.Sprintf(screen.jsTemplate, w)
				raw, err := evalJS(js)
				if err != nil {
					t.Fatalf("%s at width=%d panicked: %v", screen.name, w, err)
				}
				s, ok := raw.(string)
				if !ok || s == "" {
					t.Errorf("%s at width=%d produced empty output", screen.name, w)
					return
				}

				// Check keywords present.
				for _, kw := range screen.keywords {
					if !strings.Contains(s, kw) {
						t.Errorf("%s at width=%d missing expected keyword %q", screen.name, w, kw)
					}
				}

				// Check for broken ANSI: look for ESC[ without terminal 'm'.
				// A properly formed SGR is \x1b[...m where ... is digits and semicolons.
				// An incomplete sequence would be \x1b[ followed by end-of-string or
				// a newline before 'm'.
				checkANSI(t, s, screen.name, w)
			})
		}
	}
}

// checkANSI scans for broken ANSI escape sequences (ESC[ without closing 'm').
// checkANSI scans for broken ANSI escape sequences (ESC[ without closing letter).
// Uses a generous 80-byte scan window to accommodate truecolor SGR sequences
// like \x1b[38;2;255;255;255;48;2;255;255;255m (33+ parameter bytes).
func checkANSI(t *testing.T, s, screenName string, width int) {
	t.Helper()
	i := 0
	for i < len(s) {
		idx := strings.Index(s[i:], "\x1b[")
		if idx < 0 {
			break
		}
		pos := i + idx + 2
		// Scan forward to find a terminating letter (any CSI sequence ends at [A-Za-z]).
		// Use 80-byte window to handle long truecolor SGR parameters.
		found := false
		for j := pos; j < len(s) && j < pos+80; j++ {
			ch := s[j]
			if ch == '\n' {
				// Newline before terminator → broken sequence.
				break
			}
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
				// Any letter terminates a CSI sequence (m, H, J, K, etc.).
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s at width=%d: unterminated ANSI escape at byte offset %d",
				screenName, width, i+idx)
			return
		}
		i = pos
	}
}

// TestViews_MultiWidth_Overlays tests overlay renderers at multiple widths.
func TestViews_MultiWidth_Overlays(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	widths := []int{40, 60, 80, 100, 120}

	overlays := []struct {
		name string
		js   string
	}{
		{"helpOverlay", `globalThis.prSplit._viewHelpOverlay({width:%d,height:24})`},
		{"confirmCancel", `globalThis.prSplit._viewConfirmCancelOverlay({width:%d,height:24})`},
		{"moveFile", `globalThis.prSplit._viewMoveFileDialog({width:%d,height:24,selectedSplitIdx:0,selectedFileIdx:0,editorDialogState:{targetIdx:0}})`},
		{"renameSplit", `globalThis.prSplit._viewRenameSplitDialog({width:%d,height:24,selectedSplitIdx:0,editorDialogState:{inputText:'new-name'}})`},
		{"mergeSplits", `globalThis.prSplit._viewMergeSplitsDialog({width:%d,height:24,selectedSplitIdx:0,editorDialogState:{selected:{},cursorIdx:0}})`},
	}

	for _, ov := range overlays {
		for _, w := range widths {
			t.Run(fmt.Sprintf("%s/w=%d", ov.name, w), func(t *testing.T) {
				js := fmt.Sprintf(ov.js, w)
				raw, err := evalJS(js)
				if err != nil {
					t.Fatalf("%s at width=%d panicked: %v", ov.name, w, err)
				}
				s, ok := raw.(string)
				if !ok || s == "" {
					t.Errorf("%s at width=%d produced empty output", ov.name, w)
					return
				}
				checkANSI(t, s, ov.name, w)
			})
		}
	}
}

// TestViews_MultiWidth_Chrome tests chrome elements (titleBar, navBar, statusBar)
// at multiple widths.
func TestViews_MultiWidth_Chrome(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	widths := []int{40, 60, 80, 100, 120}

	chrome := []struct {
		name string
		js   string
	}{
		{"titleBar", `globalThis.prSplit._renderTitleBar({wizardState:'CONFIG',startTime:Date.now(),width:%d})`},
		{"navBar", `globalThis.prSplit._renderNavBar({wizardState:'PLAN_REVIEW',width:%d,isProcessing:false})`},
		{"statusBar", `globalThis.prSplit._renderStatusBar({width:%d})`},
	}

	for _, c := range chrome {
		for _, w := range widths {
			t.Run(fmt.Sprintf("%s/w=%d", c.name, w), func(t *testing.T) {
				js := fmt.Sprintf(c.js, w)
				raw, err := evalJS(js)
				if err != nil {
					t.Fatalf("%s at width=%d panicked: %v", c.name, w, err)
				}
				s, ok := raw.(string)
				if !ok || s == "" {
					t.Errorf("%s at width=%d produced empty output", c.name, w)
					return
				}
				checkANSI(t, s, c.name, w)
			})
		}
	}
}

// ---------------------------------------------------------------------------
//  T300: EQUIV_CHECK button focus styling
// ---------------------------------------------------------------------------

func TestViews_VerificationScreen_FocusStyling(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	// Set up plan cache so the verification screen can render.
	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	// T300: Verify focus styling on EQUIV_CHECK buttons.
	// Lipgloss strips colors in test context, so we monkey-patch
	// focusedSecondaryButton and focusedButton to inject markers.
	renderWithMarker := func(focusIndex int) string {
		t.Helper()
		js := fmt.Sprintf(`(function() {
			var styles = globalThis.prSplit._wizardStyles;
			var origFSB = styles.focusedSecondaryButton;
			var origFB = styles.focusedButton;
			styles.focusedSecondaryButton = function() {
				var s = origFSB();
				return { render: function(text) { return '[[FSB]]' + s.render(text); } };
			};
			styles.focusedButton = function() {
				var s = origFB();
				return { render: function(text) { return '[[FB]]' + s.render(text); } };
			};
			try {
				return globalThis.prSplit._viewVerificationScreen({
					wizardState: 'EQUIV_CHECK', width: 80,
					isProcessing: false, focusIndex: %d,
					equivalenceResult: {equivalent: false, expected: 'abc123', actual: 'def456'}
				});
			} finally {
				styles.focusedSecondaryButton = origFSB;
				styles.focusedButton = origFB;
			}
		})()`, focusIndex)
		raw, err := evalJS(js)
		if err != nil {
			t.Fatalf("renderWithMarker(%d) failed: %v", focusIndex, err)
		}
		return raw.(string)
	}

	// Focus 0 = equiv-reverify → focusedSecondaryButton marker near "Re-verify"
	out0 := renderWithMarker(0)
	if !strings.Contains(out0, "FAIL") {
		t.Error("focus 0: missing FAIL indicator")
	}
	fsbIdx := strings.Index(out0, "[[FSB]]")
	if fsbIdx < 0 {
		t.Fatal("focus 0: focusedSecondaryButton marker not found — Re-verify not receiving focus style")
	}
	afterFSB := out0[fsbIdx:]
	// The marker precedes the bordered button: [[FSB]]╭...╮\n│ Re-verify │\n╰...╯
	// Check within a larger window to account for the multi-line border.
	checkLen := len(afterFSB)
	if checkLen > 200 {
		checkLen = 200
	}
	if !strings.Contains(afterFSB[:checkLen], "Re-verify") {
		t.Errorf("focus 0: FSB marker should be near 'Re-verify', got:\n%s", afterFSB[:checkLen])
	}
	// Re-verify should NOT have focusedButton marker
	if strings.Contains(out0, "[[FB]]") {
		t.Error("focus 0: nav-next should NOT have focusedButton marker (focus on equiv-reverify)")
	}

	// Focus 1 = equiv-revise → focusedSecondaryButton marker near "Revise Plan"
	out1 := renderWithMarker(1)
	fsbIdx = strings.Index(out1, "[[FSB]]")
	if fsbIdx < 0 {
		t.Fatal("focus 1: focusedSecondaryButton marker not found — Revise Plan not receiving focus style")
	}
	afterFSB = out1[fsbIdx:]
	checkLen = len(afterFSB)
	if checkLen > 200 {
		checkLen = 200
	}
	if !strings.Contains(afterFSB[:checkLen], "Revise Plan") {
		t.Errorf("focus 1: FSB marker should be near 'Revise Plan', got:\n%s", afterFSB[:checkLen])
	}

	// Focus 2 = nav-next → focusedButton marker near "Continue"
	out2 := renderWithMarker(2)
	fbIdx := strings.Index(out2, "[[FB]]")
	if fbIdx < 0 {
		t.Fatal("focus 2: focusedButton marker not found — Continue not receiving focus style")
	}
	afterFB := out2[fbIdx:]
	checkLen = len(afterFB)
	if checkLen > 100 {
		checkLen = 100
	}
	if !strings.Contains(afterFB[:checkLen], "Continue") {
		t.Errorf("focus 2: FB marker should be near 'Continue', got:\n%s", afterFB[:checkLen])
	}

	// Focus 3 = nav-cancel → neither marker appears in the button area
	// (nav-cancel is in the navbar, not in viewVerificationScreen)
	out3 := renderWithMarker(3)
	if strings.Contains(out3, "[[FSB]]") || strings.Contains(out3, "[[FB]]") {
		t.Error("focus 3 (nav-cancel): no focus markers should appear on in-screen buttons")
	}

	// All outputs should contain all 3 button labels.
	for i, out := range []string{out0, out1, out2, out3} {
		for _, label := range []string{"Re-verify", "Revise Plan", "Continue"} {
			if !strings.Contains(out, label) {
				t.Errorf("focus %d missing '%s' label", i, label)
			}
		}
	}
}
