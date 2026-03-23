package command

// T418: Unit tests for chunk 16a focus navigation functions.
//
// Covers:
//   - getFocusElements: element lists per wizard state
//   - syncSplitSelection: focus-to-split-card sync
//   - handleNavDown / handleNavUp: focus cycling with wrap-around
//   - handleListNav: split/file list cursor clamping

import (
	"fmt"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// --- getFocusElements per wizard state ---

func TestChunk16a_GetFocusElements_CONFIG(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// CONFIG state with heuristic mode (default): strategy buttons + toggle-advanced + nav.
	val, err := evalJS(`
		(function() {
			var s = { wizardState: 'CONFIG', focusIndex: 0 };
			var elems = prSplit._getFocusElements(s);
			return JSON.stringify(elems.map(function(e) { return e.id; }));
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)

	// Expected: 3 strategies + toggle-advanced + nav-next + nav-cancel = 6.
	expected := []string{
		"strategy-auto", "strategy-heuristic", "strategy-directory",
		"toggle-advanced", "nav-next", "nav-cancel",
	}
	for _, id := range expected {
		if !strings.Contains(s, `"`+id+`"`) {
			t.Errorf("CONFIG elements missing %q\ngot: %s", id, s)
		}
	}

	// Count.
	val, err = evalJS(`
		(function() {
			var s = { wizardState: 'CONFIG', focusIndex: 0 };
			return prSplit._getFocusElements(s).length;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := val.(int64); ok && v != int64(len(expected)) {
		t.Errorf("CONFIG element count = %d, want %d", v, len(expected))
	}
}

func TestChunk16a_GetFocusElements_CONFIG_WithAdvanced(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// CONFIG with showAdvanced=true adds config fields.
	val, err := evalJS(`
		(function() {
			var s = { wizardState: 'CONFIG', showAdvanced: true, focusIndex: 0 };
			var elems = prSplit._getFocusElements(s);
			return JSON.stringify(elems.map(function(e) { return e.id; }));
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)

	// Advanced fields: maxFiles, branchPrefix, verifyCommand, dryRun.
	advFields := []string{
		"config-maxFiles", "config-branchPrefix",
		"config-verifyCommand", "config-dryRun",
	}
	for _, id := range advFields {
		if !strings.Contains(s, `"`+id+`"`) {
			t.Errorf("CONFIG+advanced missing %q\ngot: %s", id, s)
		}
	}

	// Total: 3 strategies + toggle-advanced + 4 config fields + nav-next + nav-cancel = 10.
	val, err = evalJS(`
		(function() {
			var s = { wizardState: 'CONFIG', showAdvanced: true, focusIndex: 0 };
			return prSplit._getFocusElements(s).length;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := val.(int64); ok && v != 10 {
		t.Errorf("CONFIG+advanced element count = %d, want 10", v)
	}
}

func TestChunk16a_GetFocusElements_PLAN_REVIEW(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// PLAN_REVIEW with 3 splits.
	val, err := evalJS(`
		(function() {
			prSplit._state.planCache = {
				splits: [
					{name: 'a', files: []},
					{name: 'b', files: []},
					{name: 'c', files: []}
				]
			};
			var s = { wizardState: 'PLAN_REVIEW', focusIndex: 0 };
			var elems = prSplit._getFocusElements(s);
			return JSON.stringify(elems.map(function(e) { return e.id; }));
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)

	// 3 split cards + plan-edit + plan-regenerate + ask-claude + nav-next + nav-cancel = 8.
	expected := []string{
		"split-card-0", "split-card-1", "split-card-2",
		"plan-edit", "plan-regenerate", "ask-claude",
		"nav-next", "nav-cancel",
	}
	for _, id := range expected {
		if !strings.Contains(s, `"`+id+`"`) {
			t.Errorf("PLAN_REVIEW missing %q\ngot: %s", id, s)
		}
	}

	val, err = evalJS(`
		(function() {
			var s = { wizardState: 'PLAN_REVIEW', focusIndex: 0 };
			return prSplit._getFocusElements(s).length;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := val.(int64); ok && v != int64(len(expected)) {
		t.Errorf("PLAN_REVIEW count = %d, want %d", v, len(expected))
	}

	// Cleanup.
	_, _ = evalJS(`prSplit._state.planCache = null;`)
}

func TestChunk16a_GetFocusElements_FINALIZATION(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		(function() {
			var s = { wizardState: 'FINALIZATION', focusIndex: 0 };
			var elems = prSplit._getFocusElements(s);
			return JSON.stringify(elems.map(function(e) { return e.id; }));
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)

	// final-report + final-create-prs + final-done + nav-next + nav-cancel = 5.
	expected := []string{
		"final-report", "final-create-prs", "final-done",
		"nav-next", "nav-cancel",
	}
	for _, id := range expected {
		if !strings.Contains(s, `"`+id+`"`) {
			t.Errorf("FINALIZATION missing %q\ngot: %s", id, s)
		}
	}
	if v, err2 := evalJS(`prSplit._getFocusElements({wizardState:'FINALIZATION'}).length`); err2 == nil {
		if n, ok := v.(int64); ok && n != int64(len(expected)) {
			t.Errorf("FINALIZATION count = %d, want %d", n, len(expected))
		}
	}
}

func TestChunk16a_GetFocusElements_PAUSED(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		(function() {
			var s = { wizardState: 'PAUSED' };
			var elems = prSplit._getFocusElements(s);
			return JSON.stringify(elems.map(function(e) { return e.id; }));
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)
	// pause-resume + pause-quit = 2.
	if s != `["pause-resume","pause-quit"]` {
		t.Errorf("PAUSED elements = %s, want [pause-resume, pause-quit]", s)
	}
}

func TestChunk16a_GetFocusElements_EmptyDefault(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// Unknown state returns empty array.
	val, err := evalJS(`prSplit._getFocusElements({wizardState:'BOGUS'}).length`)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := val.(int64); ok && v != 0 {
		t.Errorf("unknown state element count = %d, want 0", v)
	}
}

// --- syncSplitSelection ---

func TestChunk16a_SyncSplitSelection_CardFocus(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		(function() {
			var s = { focusIndex: 2, selectedSplitIdx: 0 };
			var elems = [
				{id: 'split-card-0', type: 'card'},
				{id: 'split-card-1', type: 'card'},
				{id: 'split-card-2', type: 'card'},
				{id: 'nav-next', type: 'nav'}
			];
			prSplit._syncSplitSelection(s, elems);
			return s.selectedSplitIdx;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := val.(int64); !ok || v != 2 {
		t.Errorf("selectedSplitIdx = %v, want 2", val)
	}
}

func TestChunk16a_SyncSplitSelection_NonCard(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// When focused on a non-card element, selectedSplitIdx stays unchanged.
	val, err := evalJS(`
		(function() {
			var s = { focusIndex: 1, selectedSplitIdx: 5 };
			var elems = [
				{id: 'split-card-0', type: 'card'},
				{id: 'nav-next', type: 'nav'}
			];
			prSplit._syncSplitSelection(s, elems);
			return s.selectedSplitIdx;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := val.(int64); !ok || v != 5 {
		t.Errorf("selectedSplitIdx should remain 5, got %v", val)
	}
}

// --- handleNavDown / handleNavUp ---

func TestChunk16a_HandleNavDown_Wraps(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// Set up a state with PAUSED (2 elements) and focusIndex at 1 (last).
	val, err := evalJS(`
		(function() {
			var s = { wizardState: 'PAUSED', focusIndex: 1 };
			var result = prSplit._handleNavDown(s);
			return result[0].focusIndex;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	// (1 + 1) % 2 = 0 → wraps to beginning.
	if v, ok := val.(int64); !ok || v != 0 {
		t.Errorf("handleNavDown wrap: focusIndex = %v, want 0", val)
	}
}

func TestChunk16a_HandleNavDown_Increment(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		(function() {
			var s = { wizardState: 'PAUSED', focusIndex: 0 };
			var result = prSplit._handleNavDown(s);
			return result[0].focusIndex;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	// (0 + 1) % 2 = 1.
	if v, ok := val.(int64); !ok || v != 1 {
		t.Errorf("handleNavDown increment: focusIndex = %v, want 1", val)
	}
}

func TestChunk16a_HandleNavUp_Wraps(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		(function() {
			var s = { wizardState: 'PAUSED', focusIndex: 0 };
			var result = prSplit._handleNavUp(s);
			return result[0].focusIndex;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	// (0 - 1) < 0 → elems.length - 1 = 1.
	if v, ok := val.(int64); !ok || v != 1 {
		t.Errorf("handleNavUp wrap: focusIndex = %v, want 1", val)
	}
}

func TestChunk16a_HandleNavUp_Decrement(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		(function() {
			var s = { wizardState: 'PAUSED', focusIndex: 1 };
			var result = prSplit._handleNavUp(s);
			return result[0].focusIndex;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	// 1 - 1 = 0.
	if v, ok := val.(int64); !ok || v != 0 {
		t.Errorf("handleNavUp decrement: focusIndex = %v, want 0", val)
	}
}

// --- handleListNav ---

func TestChunk16a_HandleListNav_PlanReview(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// Set up plan with 3 splits.
	_, err := evalJS(`
		prSplit._state.planCache = {
			splits: [
				{name: 'a', files: ['f1']},
				{name: 'b', files: ['f2']},
				{name: 'c', files: ['f3']}
			]
		};
	`)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("move_down", func(t *testing.T) {
		val, err := evalJS(`
			(function() {
				var s = { wizardState: 'PLAN_REVIEW', selectedSplitIdx: 0, focusIndex: 0 };
				var result = prSplit._handleListNav(s, 1);
				return result[0].selectedSplitIdx;
			})()
		`)
		if err != nil {
			t.Fatal(err)
		}
		if v, ok := val.(int64); !ok || v != 1 {
			t.Errorf("PLAN_REVIEW +1: selectedSplitIdx = %v, want 1", val)
		}
	})

	t.Run("clamp_at_end", func(t *testing.T) {
		val, err := evalJS(`
			(function() {
				var s = { wizardState: 'PLAN_REVIEW', selectedSplitIdx: 2, focusIndex: 2 };
				var result = prSplit._handleListNav(s, 1);
				return result[0].selectedSplitIdx;
			})()
		`)
		if err != nil {
			t.Fatal(err)
		}
		// Clamped: max(0, min(3, 3-1)) = 2.
		if v, ok := val.(int64); !ok || v != 2 {
			t.Errorf("PLAN_REVIEW clamp end: selectedSplitIdx = %v, want 2", val)
		}
	})

	t.Run("clamp_at_start", func(t *testing.T) {
		val, err := evalJS(`
			(function() {
				var s = { wizardState: 'PLAN_REVIEW', selectedSplitIdx: 0, focusIndex: 0 };
				var result = prSplit._handleListNav(s, -1);
				return result[0].selectedSplitIdx;
			})()
		`)
		if err != nil {
			t.Fatal(err)
		}
		// Clamped: max(0, min(-1, 2)) = 0.
		if v, ok := val.(int64); !ok || v != 0 {
			t.Errorf("PLAN_REVIEW clamp start: selectedSplitIdx = %v, want 0", val)
		}
	})

	// focusIndex syncs with selectedSplitIdx.
	t.Run("syncs_focusIndex", func(t *testing.T) {
		val, err := evalJS(`
			(function() {
				var s = { wizardState: 'PLAN_REVIEW', selectedSplitIdx: 0, focusIndex: 0 };
				var result = prSplit._handleListNav(s, 1);
				return result[0].focusIndex;
			})()
		`)
		if err != nil {
			t.Fatal(err)
		}
		if v, ok := val.(int64); !ok || v != 1 {
			t.Errorf("PLAN_REVIEW focusIndex sync = %v, want 1", val)
		}
	})

	// Cleanup.
	_, _ = evalJS(`prSplit._state.planCache = null;`)
}

func TestChunk16a_HandleListNav_PlanEditor(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	_, err := evalJS(`
		prSplit._state.planCache = {
			splits: [{name: 'x', files: ['a.go', 'b.go', 'c.go']}]
		};
	`)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("file_nav_down", func(t *testing.T) {
		val, err := evalJS(`
			(function() {
				var s = { wizardState: 'PLAN_EDITOR', selectedSplitIdx: 0, selectedFileIdx: 0 };
				var result = prSplit._handleListNav(s, 1);
				return result[0].selectedFileIdx;
			})()
		`)
		if err != nil {
			t.Fatal(err)
		}
		if v, ok := val.(int64); !ok || v != 1 {
			t.Errorf("PLAN_EDITOR file +1: selectedFileIdx = %v, want 1", val)
		}
	})

	t.Run("file_clamp_end", func(t *testing.T) {
		val, err := evalJS(`
			(function() {
				var s = { wizardState: 'PLAN_EDITOR', selectedSplitIdx: 0, selectedFileIdx: 2 };
				var result = prSplit._handleListNav(s, 1);
				return result[0].selectedFileIdx;
			})()
		`)
		if err != nil {
			t.Fatal(err)
		}
		// 3 files, index at 2 + 1 = clamped to 2.
		if v, ok := val.(int64); !ok || v != 2 {
			t.Errorf("PLAN_EDITOR file clamp end: selectedFileIdx = %v, want 2", val)
		}
	})

	t.Run("file_clamp_start", func(t *testing.T) {
		val, err := evalJS(`
			(function() {
				var s = { wizardState: 'PLAN_EDITOR', selectedSplitIdx: 0, selectedFileIdx: 0 };
				var result = prSplit._handleListNav(s, -1);
				return result[0].selectedFileIdx;
			})()
		`)
		if err != nil {
			t.Fatal(err)
		}
		// Clamped: max(0, 0-1) = 0.
		if v, ok := val.(int64); !ok || v != 0 {
			t.Errorf("PLAN_EDITOR file clamp start: selectedFileIdx = %v, want 0", val)
		}
	})

	// Cleanup.
	_, _ = evalJS(`prSplit._state.planCache = null;`)
}

func TestChunk16a_HandleListNav_EditorGuards(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// During title editing, list nav should be a no-op.
	t.Run("title_editing_guard", func(t *testing.T) {
		val, err := evalJS(`
			(function() {
				var s = {
					wizardState: 'PLAN_EDITOR',
					selectedSplitIdx: 0,
					selectedFileIdx: 0,
					editorTitleEditing: true
				};
				var result = prSplit._handleListNav(s, 1);
				return result[0].selectedFileIdx;
			})()
		`)
		if err != nil {
			t.Fatal(err)
		}
		// Guard fires, selectedFileIdx unchanged.
		if v, ok := val.(int64); !ok || v != 0 {
			t.Errorf("editing guard: selectedFileIdx = %v, want 0", val)
		}
	})

	// During config field editing, list nav should be a no-op.
	t.Run("config_editing_guard", func(t *testing.T) {
		val, err := evalJS(`
			(function() {
				var s = {
					wizardState: 'CONFIG',
					focusIndex: 0,
					configFieldEditing: 'maxFiles'
				};
				var result = prSplit._handleListNav(s, 1);
				return result[0].focusIndex;
			})()
		`)
		if err != nil {
			t.Fatal(err)
		}
		// Guard fires, focusIndex unchanged.
		if v, ok := val.(int64); !ok || v != 0 {
			t.Errorf("config editing guard: focusIndex = %v, want 0", val)
		}
	})
}

func TestChunk16a_HandleListNav_ConfigDelegatesToNav(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// CONFIG with delta>0 calls handleNavDown (cycling focus).
	val, err := evalJS(`
		(function() {
			var s = { wizardState: 'CONFIG', focusIndex: 0 };
			var result = prSplit._handleListNav(s, 1);
			return result[0].focusIndex;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	// handleNavDown on CONFIG with 6 elements: (0+1)%6 = 1.
	if v, ok := val.(int64); !ok || v != 1 {
		t.Errorf("CONFIG j delegates to navDown: focusIndex = %v, want 1", val)
	}

	// delta=-1 should delegate to handleNavUp.
	val, err = evalJS(`
		(function() {
			var s = { wizardState: 'CONFIG', focusIndex: 0 };
			var result = prSplit._handleListNav(s, -1);
			return result[0].focusIndex;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	// handleNavUp on CONFIG with 6 elements: (0-1) < 0 → 5.
	if v, ok := val.(int64); !ok || v != 5 {
		t.Errorf("CONFIG k delegates to navUp: focusIndex = %v, want 5", val)
	}
}

// --- ERROR_RESOLUTION focus elements ---

func TestChunk16a_GetFocusElements_ERROR_RESOLUTION(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	t.Run("standard", func(t *testing.T) {
		val, err := evalJS(`
			(function() {
				var s = { wizardState: 'ERROR_RESOLUTION', focusIndex: 0 };
				var elems = prSplit._getFocusElements(s);
				return JSON.stringify(elems.map(function(e) { return e.id; }));
			})()
		`)
		if err != nil {
			t.Fatal(err)
		}
		s, _ := val.(string)
		expected := []string{
			"resolve-auto", "resolve-manual", "resolve-skip",
			"resolve-retry", "resolve-abort",
			"nav-next", "nav-cancel",
		}
		for _, id := range expected {
			if !strings.Contains(s, `"`+id+`"`) {
				t.Errorf("ERROR_RESOLUTION missing %q\ngot: %s", id, s)
			}
		}
	})

	t.Run("crash_mode", func(t *testing.T) {
		val, err := evalJS(`
			(function() {
				var s = { wizardState: 'ERROR_RESOLUTION', claudeCrashDetected: true, focusIndex: 0 };
				var elems = prSplit._getFocusElements(s);
				return JSON.stringify(elems.map(function(e) { return e.id; }));
			})()
		`)
		if err != nil {
			t.Fatal(err)
		}
		s, _ := val.(string)
		expected := []string{
			"resolve-restart-claude", "resolve-fallback-heuristic",
			"resolve-abort", "nav-next", "nav-cancel",
		}
		for _, id := range expected {
			if !strings.Contains(s, `"`+id+`"`) {
				t.Errorf("ERROR_RESOLUTION crash missing %q\ngot: %s", id, s)
			}
		}
		// Should NOT contain standard resolve buttons.
		if strings.Contains(s, `"resolve-auto"`) {
			t.Errorf("crash mode should not contain resolve-auto, got: %s", s)
		}
		// Count: 3 crash + nav-next + nav-cancel = 5.
		cntVal, err := evalJS(`
			prSplit._getFocusElements({
				wizardState: 'ERROR_RESOLUTION', claudeCrashDetected: true
			}).length
		`)
		if err != nil {
			t.Fatal(err)
		}
		if n, ok := cntVal.(int64); ok && n != 5 {
			t.Errorf("ERROR_RESOLUTION crash count = %d, want 5", n)
		}
	})

	t.Run("standard_count", func(t *testing.T) {
		val, err := evalJS(`
			prSplit._getFocusElements({wizardState: 'ERROR_RESOLUTION'}).length
		`)
		if err != nil {
			t.Fatal(err)
		}
		// 5 resolve + nav-next + nav-cancel = 7.
		if n, ok := val.(int64); ok && n != 7 {
			t.Errorf("ERROR_RESOLUTION standard count = %d, want 7", n)
		}
	})
}

// --- EQUIV_CHECK focus elements ---

func TestChunk16a_GetFocusElements_EQUIV_CHECK(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	t.Run("processing_empty", func(t *testing.T) {
		val, err := evalJS(`
			prSplit._getFocusElements({
				wizardState: 'EQUIV_CHECK',
				isProcessing: true
			}).length
		`)
		if err != nil {
			t.Fatal(err)
		}
		// During processing, no focusable elements.
		if v, ok := val.(int64); !ok || v != 0 {
			t.Errorf("EQUIV_CHECK processing count = %v, want 0", val)
		}
	})

	t.Run("non_equivalent_result", func(t *testing.T) {
		val, err := evalJS(`
			(function() {
				var s = {
					wizardState: 'EQUIV_CHECK',
					isProcessing: false,
					equivalenceResult: { equivalent: false }
				};
				return JSON.stringify(prSplit._getFocusElements(s).map(function(e){return e.id;}));
			})()
		`)
		if err != nil {
			t.Fatal(err)
		}
		s := fmt.Sprintf("%v", val)
		// equiv-reverify + equiv-revise + nav-back + nav-next + nav-cancel = 5.
		for _, id := range []string{"equiv-reverify", "equiv-revise", "nav-back", "nav-next", "nav-cancel"} {
			if !strings.Contains(s, id) {
				t.Errorf("EQUIV_CHECK non-equiv missing %q\ngot: %s", id, s)
			}
		}
		// Count.
		cnt, err := evalJS(`
			prSplit._getFocusElements({
				wizardState: 'EQUIV_CHECK',
				isProcessing: false,
				equivalenceResult: { equivalent: false }
			}).length
		`)
		if err != nil {
			t.Fatal(err)
		}
		if n, ok := cnt.(int64); ok && n != 5 {
			t.Errorf("EQUIV_CHECK non-equiv count = %d, want 5", n)
		}
	})

	t.Run("equivalent_result", func(t *testing.T) {
		val, err := evalJS(`
			(function() {
				var s = {
					wizardState: 'EQUIV_CHECK',
					isProcessing: false,
					equivalenceResult: { equivalent: true }
				};
				return JSON.stringify(prSplit._getFocusElements(s).map(function(e){return e.id;}));
			})()
		`)
		if err != nil {
			t.Fatal(err)
		}
		s := fmt.Sprintf("%v", val)
		// When equivalent: no reverify/revise, just nav-back + nav-next + nav-cancel = 3.
		if strings.Contains(s, "equiv-reverify") {
			t.Errorf("equiv result should not have reverify, got: %s", s)
		}
		for _, id := range []string{"nav-back", "nav-next", "nav-cancel"} {
			if !strings.Contains(s, id) {
				t.Errorf("EQUIV_CHECK equiv missing %q\ngot: %s", id, s)
			}
		}
		// Count.
		cnt, err := evalJS(`
			prSplit._getFocusElements({
				wizardState: 'EQUIV_CHECK',
				isProcessing: false,
				equivalenceResult: { equivalent: true }
			}).length
		`)
		if err != nil {
			t.Fatal(err)
		}
		if n, ok := cnt.(int64); ok && n != 3 {
			t.Errorf("EQUIV_CHECK equiv count = %d, want 3", n)
		}
	})

	t.Run("null_result_no_processing", func(t *testing.T) {
		// Not processing but no equivalenceResult yet → all inside guard, returns empty.
		val, err := evalJS(`
			prSplit._getFocusElements({
				wizardState: 'EQUIV_CHECK',
				isProcessing: false
			}).length
		`)
		if err != nil {
			t.Fatal(err)
		}
		// Source: if (!s.isProcessing && s.equivalenceResult) — equivalenceResult is falsy → 0 elements.
		if n, ok := val.(int64); ok && n != 0 {
			t.Errorf("EQUIV_CHECK null result count = %d, want 0", n)
		}
	})
}

// --- IDLE and BASELINE_FAIL states (fallthrough to CONFIG) ---

func TestChunk16a_GetFocusElements_IDLE(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// IDLE falls through to same branch as CONFIG.
	val, err := evalJS(`
		(function() {
			var configElems = prSplit._getFocusElements({wizardState: 'CONFIG'});
			var idleElems = prSplit._getFocusElements({wizardState: 'IDLE'});
			return configElems.length === idleElems.length;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if b, ok := val.(bool); !ok || !b {
		t.Errorf("IDLE should produce same element count as CONFIG")
	}
}

func TestChunk16a_GetFocusElements_BASELINE_FAIL(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// BASELINE_FAIL falls through to same branch as CONFIG.
	val, err := evalJS(`
		(function() {
			var configElems = prSplit._getFocusElements({wizardState: 'CONFIG'});
			var bfElems = prSplit._getFocusElements({wizardState: 'BASELINE_FAIL'});
			return configElems.length === bfElems.length;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if b, ok := val.(bool); !ok || !b {
		t.Errorf("BASELINE_FAIL should produce same element count as CONFIG")
	}
}
