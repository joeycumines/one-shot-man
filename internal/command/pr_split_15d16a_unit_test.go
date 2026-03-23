package command

// T417: Unit tests for chunk 15d pure helpers and chunk 16a constants.
//
// Covers:
//   - computeReportOverlayDims (pure arithmetic)
//   - syncReportScrollbar (mock viewport/scrollbar)
//   - CHROME_ESTIMATE constant
//   - viewForState dispatcher (smoke test per wizard state)

import (
	"fmt"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// --- Chunk 15d: computeReportOverlayDims ---

func TestChunk15d_ComputeReportOverlayDims_DefaultSize(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`JSON.stringify(prSplit._computeReportOverlayDims({}))`)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string, got %T", val)
	}
	// Default: width=80, height=DEFAULT_ROWS=24.
	// overlayW = min(72, 80-6) = min(72,74) = 72
	// overlayH = max(8, 24-6) = max(8,18) = 18
	// vpW = max(10, 72-4) = 68
	// vpH = max(3, 18-4) = 14
	assertContains(t, s, `"overlayW":72`, "default overlayW")
	assertContains(t, s, `"overlayH":18`, "default overlayH")
	assertContains(t, s, `"vpW":68`, "default vpW")
	assertContains(t, s, `"vpH":14`, "default vpH")
}

func TestChunk15d_ComputeReportOverlayDims_NarrowTerminal(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`JSON.stringify(prSplit._computeReportOverlayDims({width: 30, height: 12}))`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)
	// overlayW = min(72, 30-6) = min(72,24) = 24
	// overlayH = max(8, 12-6) = max(8,6) = 8
	// vpW = max(10, 24-4) = 20
	// vpH = max(3, 8-4) = 4
	assertContains(t, s, `"overlayW":24`, "narrow overlayW")
	assertContains(t, s, `"overlayH":8`, "narrow overlayH (clamped)")
	assertContains(t, s, `"vpW":20`, "narrow vpW")
	assertContains(t, s, `"vpH":4`, "narrow vpH")
}

func TestChunk15d_ComputeReportOverlayDims_WideTerminal(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`JSON.stringify(prSplit._computeReportOverlayDims({width: 200, height: 50}))`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)
	// overlayW = min(72, 200-6) = min(72,194) = 72 (caps at 72)
	// overlayH = max(8, 50-6) = max(8,44) = 44
	// vpW = max(10, 72-4) = 68
	// vpH = max(3, 44-4) = 40
	assertContains(t, s, `"overlayW":72`, "wide overlayW (capped)")
	assertContains(t, s, `"overlayH":44`, "wide overlayH")
	assertContains(t, s, `"vpW":68`, "wide vpW")
	assertContains(t, s, `"vpH":40`, "wide vpH")
}

func TestChunk15d_ComputeReportOverlayDims_TinyTerminal(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`JSON.stringify(prSplit._computeReportOverlayDims({width: 14, height: 8}))`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)
	// overlayW = min(72, 14-6) = min(72,8) = 8
	// overlayH = max(8, 8-6) = max(8,2) = 8
	// vpW = max(10, 8-4) = max(10,4) = 10 (clamped to min 10)
	// vpH = max(3, 8-4) = 4
	assertContains(t, s, `"overlayW":8`, "tiny overlayW")
	assertContains(t, s, `"overlayH":8`, "tiny overlayH (clamped)")
	assertContains(t, s, `"vpW":10`, "tiny vpW (clamped to min 10)")
	assertContains(t, s, `"vpH":4`, "tiny vpH")
}

// --- Chunk 15d: syncReportScrollbar ---

func TestChunk15d_SyncReportScrollbar_WithMocks(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// Create mock viewport and scrollbar objects that record calls.
	_, err := evalJS(`
		var _sbCalls = {};
		var mockVp = {
			totalLineCount: function() { return 42; },
			yOffset: function() { return 7; }
		};
		var mockSb = {
			setContentHeight: function(h) { _sbCalls.contentHeight = h; },
			setYOffset: function(y) { _sbCalls.yOffset = y; }
		};
	`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = evalJS(`prSplit._syncReportScrollbar({reportVp: mockVp, reportSb: mockSb})`)
	if err != nil {
		t.Fatal(err)
	}

	// Verify calls.
	val, err := evalJS(`_sbCalls.contentHeight`)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := val.(int64); !ok || v != 42 {
		t.Errorf("setContentHeight = %v, want 42", val)
	}

	val, err = evalJS(`_sbCalls.yOffset`)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := val.(int64); !ok || v != 7 {
		t.Errorf("setYOffset = %v, want 7", val)
	}
}

func TestChunk15d_SyncReportScrollbar_NoopWithoutMocks(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// Should not panic when reportVp or reportSb is missing.
	_, err := evalJS(`prSplit._syncReportScrollbar({})`)
	if err != nil {
		t.Fatalf("syncReportScrollbar({}) should not error: %v", err)
	}

	_, err = evalJS(`prSplit._syncReportScrollbar({reportVp: {totalLineCount: function(){return 1;}, yOffset: function(){return 0;}}})`)
	if err != nil {
		t.Fatalf("syncReportScrollbar with vp but no sb should not error: %v", err)
	}

	_, err = evalJS(`prSplit._syncReportScrollbar({reportSb: {setContentHeight: function(){}, setYOffset: function(){}}})`)
	if err != nil {
		t.Fatalf("syncReportScrollbar with sb but no vp should not error: %v", err)
	}
}

// --- Chunk 16a: CHROME_ESTIMATE ---

func TestChunk16a_ChromeEstimate(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`prSplit._CHROME_ESTIMATE`)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := val.(int64); !ok || v != 8 {
		t.Errorf("CHROME_ESTIMATE = %v, want 8", val)
	}
}

// --- Chunk 15d: viewForState dispatcher ---

func TestChunk15d_ViewForState_AllStates(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// viewForState(s) returns a rendered string for each known wizard state.
	// We test that it returns a non-empty string and does not throw.
	states := []struct {
		state    string
		contains string // substring we expect in the output
	}{
		{"IDLE", ""},              // CONFIG screen, empty plan is fine
		{"CONFIG", ""},            // CONFIG screen
		{"BASELINE_FAIL", ""},     // CONFIG screen (fallthrough with IDLE/CONFIG)
		{"PLAN_GENERATION", ""},   // analysis screen
		{"PLAN_REVIEW", ""},       // plan review screen
		{"PLAN_EDITOR", ""},       // plan editor screen
		{"BRANCH_BUILDING", ""},   // execution screen
		{"ERROR_RESOLUTION", ""},  // error resolution
		{"EQUIV_CHECK", ""},       // verification screen
		{"FINALIZATION", ""},      // finalization screen
		{"DONE", ""},              // same as finalization
		{"CANCELLED", "Cancelled"},
		{"FORCE_CANCEL", "Cancelled"},
		{"ERROR", "Error"},
		{"PAUSED", "Paused"},
	}

	for _, tc := range states {
		t.Run(tc.state, func(t *testing.T) {
			js := fmt.Sprintf(`
				(function() {
					var s = {
						wizardState: '%s',
						width: 80,
						height: 24,
						focusIndex: 0,
						wizard: prSplit._state.wizard || {
							current: '%s',
							data: {},
							checkpoint: null,
							transition: function(){},
							reset: function(){},
							resume: function(){ return false; },
							cancel: function(){}
						}
					};
					try {
						var result = prSplit._viewForState(s);
						return typeof result === 'string' ? result : String(result);
					} catch(e) {
						return 'ERROR:' + e.message;
					}
				})()
			`, tc.state, tc.state)

			val, err := evalJS(js)
			if err != nil {
				t.Fatalf("viewForState(%s) eval error: %v", tc.state, err)
			}
			s, _ := val.(string)
			if strings.HasPrefix(s, "ERROR:") {
				t.Errorf("viewForState(%s) threw: %s", tc.state, s)
			}
			if tc.contains != "" && !strings.Contains(s, tc.contains) {
				t.Errorf("viewForState(%s) missing %q in output of length %d",
					tc.state, tc.contains, len(s))
			}
		})
	}

	// Unknown state returns "Unknown state: XYZ".
	t.Run("UNKNOWN", func(t *testing.T) {
		val, err := evalJS(`prSplit._viewForState({wizardState: 'BOGUS'})`)
		if err != nil {
			t.Fatal(err)
		}
		s, _ := val.(string)
		if !strings.Contains(s, "Unknown state: BOGUS") {
			t.Errorf("expected 'Unknown state: BOGUS', got: %q", s)
		}
	})
}

// --- Helper ---

func assertContains(t *testing.T, haystack, needle, label string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("%s: expected %q in %q", label, needle, haystack)
	}
}
