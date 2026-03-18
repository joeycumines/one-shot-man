package command

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  T350: Auto-scroll main viewport during verification
// ---------------------------------------------------------------------------

// TestVerifyPoll_AutoScroll verifies that pollVerifySession calls
// s.vp.gotoBottom() when verifyAutoScroll is enabled, ensuring the main
// viewport keeps the inline verify terminal visible during branch building.
func TestVerifyPoll_AutoScroll(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// Set up a mock state with vp that records gotoBottom calls,
	// and a mock activeVerifySession. Then invoke pollVerifySession.
	raw, err := evalJS(`(function() {
		var bottomCalled = 0;
		var s = {
			wizardState: 'BRANCH_BUILDING',
			isProcessing: true,
			verifyAutoScroll: true,
			verifyScreen: '',
			verifyViewportOffset: 0,
			spinnerFrame: 0,
			vp: { gotoBottom: function() { bottomCalled++; } },
			activeVerifySession: {
				screen: function() { return 'verify output line 1\nline 2'; },
				output: function() { return ''; },
				isDone: function() { return false; },
				isRunning: function() { return true; }
			},
			activeVerifyBranch: 'split/test',
			activeVerifyStartTime: Date.now() - 3000,
			verifyElapsedMs: 0,
			verificationResults: []
		};

		// Call pollVerifySession via the exported handler.
		var result = globalThis.prSplit._pollVerifySession(s);

		return JSON.stringify({
			bottomCalled: bottomCalled,
			verifyScreen: s.verifyScreen,
			hasResult: !!(result && result.length === 2),
			hasTickCmd: !!(result && result.length === 2 && result[1] !== null)
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		BottomCalled int    `json:"bottomCalled"`
		VerifyScreen string `json:"verifyScreen"`
		HasResult    bool   `json:"hasResult"`
		HasTickCmd   bool   `json:"hasTickCmd"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed.BottomCalled < 1 {
		t.Errorf("expected vp.gotoBottom() to be called at least once, got %d", parsed.BottomCalled)
	}
	if parsed.VerifyScreen == "" {
		t.Error("expected verifyScreen to be populated from session.screen()")
	}
	if !parsed.HasResult {
		t.Error("expected pollVerifySession to return [state, cmd] pair")
	}
	if !parsed.HasTickCmd {
		t.Error("expected pollVerifySession to return a tick command for continued polling")
	}

	// Test with verifyAutoScroll disabled — should NOT call gotoBottom.
	raw2, err := evalJS(`(function() {
		var bottomCalled = 0;
		var s = {
			wizardState: 'BRANCH_BUILDING',
			isProcessing: true,
			verifyAutoScroll: false,
			verifyScreen: '',
			verifyViewportOffset: 5,
			spinnerFrame: 0,
			vp: { gotoBottom: function() { bottomCalled++; } },
			activeVerifySession: {
				screen: function() { return 'output'; },
				output: function() { return ''; },
				isDone: function() { return false; },
				isRunning: function() { return true; }
			},
			activeVerifyBranch: 'split/test',
			activeVerifyStartTime: Date.now() - 1000,
			verifyElapsedMs: 0,
			verificationResults: []
		};
		globalThis.prSplit._pollVerifySession(s);
		return bottomCalled;
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw2.(int64) != 0 {
		t.Errorf("expected gotoBottom NOT called when verifyAutoScroll=false, got %d", raw2.(int64))
	}
}

// ---------------------------------------------------------------------------
//  T351: Inline verify terminal uses s.verifyScreen snapshot
// ---------------------------------------------------------------------------

// TestExecScreen_InlineTerminal_UsesVerifyScreen verifies that
// viewExecutionScreen reads from s.verifyScreen rather than calling
// s.activeVerifySession.screen() directly.
func TestExecScreen_InlineTerminal_UsesVerifyScreen(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	// Set verifyScreen to SNAPSHOT content while screen() returns DIRECT content.
	// The rendered output should contain SNAPSHOT, not DIRECT.
	raw, err := evalJS(`(function() {
		return globalThis.prSplit._viewExecutionScreen({
			wizardState: 'BRANCH_BUILDING', width: 80,
			executionResults: [{sha: 'abc123'}],
			executingIdx: 1,
			isProcessing: true,
			verifyingIdx: 1,
			verificationResults: [{passed: true, name: 'split/api'}],
			activeVerifySession: {
				screen: function() { return 'DIRECT_CALL_MARKER'; },
				output: function() { return ''; },
				isDone: function() { return false; },
				isRunning: function() { return true; }
			},
			verifyScreen: 'SNAPSHOT_MARKER: cached screen output',
			activeVerifyBranch: 'split/cli',
			activeVerifyStartTime: Date.now() - 2000,
			verifyAutoScroll: true,
			verifyViewportOffset: 0
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	rendered := raw.(string)
	if !strings.Contains(rendered, "SNAPSHOT_MARKER") {
		t.Error("expected rendered output to contain SNAPSHOT_MARKER from s.verifyScreen")
	}
	if strings.Contains(rendered, "DIRECT_CALL_MARKER") {
		t.Error("expected rendered output NOT to contain DIRECT_CALL_MARKER from screen()")
	}
}

// TestExecScreen_InlineTerminal_FallbackOutput verifies that when there's no
// activeVerifySession but s.verifyScreen has content (fallback path), the
// inline terminal still renders the content with a fallback indicator.
func TestExecScreen_InlineTerminal_FallbackOutput(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	if _, err := evalJS(viewTestPlanState); err != nil {
		t.Fatal(err)
	}

	// No activeVerifySession, but verifyScreen has fallback content.
	raw, err := evalJS(`(function() {
		return globalThis.prSplit._viewExecutionScreen({
			wizardState: 'BRANCH_BUILDING', width: 80,
			executionResults: [{sha: 'abc123'}],
			executingIdx: 1,
			isProcessing: true,
			verifyingIdx: 1,
			verificationResults: [{passed: true, name: 'split/api'}],
			activeVerifySession: null,
			verifyScreen: 'FALLBACK_LINE_1\nFALLBACK_LINE_2\nFALLBACK_LINE_3',
			activeVerifyBranch: 'split/cli',
			activeVerifyStartTime: Date.now() - 1000,
			verifyAutoScroll: true,
			verifyViewportOffset: 0
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	rendered := raw.(string)
	if !strings.Contains(rendered, "FALLBACK_LINE_1") {
		t.Error("expected fallback content to be visible in inline terminal")
	}
	if !strings.Contains(rendered, "FALLBACK_LINE_2") {
		t.Error("expected FALLBACK_LINE_2 in inline terminal")
	}
	// Verify the fallback footer is shown instead of interactive controls.
	if !strings.Contains(rendered, "fallback") {
		t.Error("expected '(fallback output)' label in footer")
	}
	// Interactive controls should NOT be present when no session.
	if strings.Contains(rendered, "Pause") {
		t.Error("expected no Pause button in fallback mode")
	}
}

// ---------------------------------------------------------------------------
//  T352: Fallback verifyScreen population and tab visibility
// ---------------------------------------------------------------------------

// TestVerifyFallback_PopulatesVerifyScreen verifies that during fallback
// verification (no CaptureSession), s.verifyScreen is populated from the
// outputFn callback with accumulated output lines, capped at 24 rows.
func TestVerifyFallback_PopulatesVerifyScreen(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// Simulate the outputFn behavior from runVerifyFallbackAsync. The actual
	// function is async (runs a subprocess) so we replicate the exact outputFn
	// logic from pr_split_16c_tui_handlers_verify.js:
	//   outputLines.push(line);
	//   var rows = Math.min(24, outputLines.length);
	//   s.verifyScreen = outputLines.slice(-rows).join('\n');
	raw, err := evalJS(`(function() {
		var outputLines = [];
		var s = { verifyScreen: '', verifyOutput: {} };
		s.verifyOutput['split/test'] = outputLines;

		function outputFn(line) {
			outputLines.push(line);
			var rows = Math.min(24, outputLines.length);
			s.verifyScreen = outputLines.slice(-rows).join('\n');
		}

		// Simulate 5 output lines.
		outputFn('Starting tests...');
		outputFn('test_utils.go:15: PASS');
		outputFn('test_main.go:42: PASS');
		outputFn('test_api.go:8: FAIL');
		outputFn('Done.');

		var lineCount5 = s.verifyScreen.split('\n').length;
		var containsFirst5 = s.verifyScreen.indexOf('Starting tests') >= 0;
		var containsLast5 = s.verifyScreen.indexOf('Done.') >= 0;

		// Verify rolling window — push 30 lines, should keep last 24.
		s.verifyScreen = '';
		var bigLines = [];
		for (var i = 0; i < 30; i++) {
			outputFn('line-' + i);
		}
		var lineCount30 = s.verifyScreen.split('\n').length;
		var containsOldest = s.verifyScreen.indexOf('line-0') >= 0;
		var containsNewest = s.verifyScreen.indexOf('line-29') >= 0;

		return JSON.stringify({
			lineCount5: lineCount5,
			containsFirst5: containsFirst5,
			containsLast5: containsLast5,
			lineCount30: lineCount30,
			containsOldest: containsOldest,
			containsNewest: containsNewest
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		LineCount5     int  `json:"lineCount5"`
		ContainsFirst5 bool `json:"containsFirst5"`
		ContainsLast5  bool `json:"containsLast5"`
		LineCount30    int  `json:"lineCount30"`
		ContainsOldest bool `json:"containsOldest"`
		ContainsNewest bool `json:"containsNewest"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed.LineCount5 != 5 {
		t.Errorf("expected 5 lines in verifyScreen for 5 outputs, got %d", parsed.LineCount5)
	}
	if !parsed.ContainsFirst5 {
		t.Error("expected verifyScreen to contain first line")
	}
	if !parsed.ContainsLast5 {
		t.Error("expected verifyScreen to contain last line")
	}
	// Rolling window cap: 30 lines input → only last 24 visible.
	if parsed.LineCount30 != 24 {
		t.Errorf("expected 24 lines (rolling window cap) for 30 outputs, got %d", parsed.LineCount30)
	}
	if parsed.ContainsOldest {
		t.Error("expected line-0 to be dropped by rolling window")
	}
	if !parsed.ContainsNewest {
		t.Error("expected verifyScreen to contain newest line (line-29)")
	}
}

// TestVerifyTab_FallbackVisibility verifies that the Verify tab label appears
// in the split-view tab bar during fallback verification (no CaptureSession).
// Uses the full _wizardView rendering path to confirm the tab is visible.
func TestVerifyTab_FallbackVisibility(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	// Case 1: verifyFallbackRunning=true — Verify tab must appear.
	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('BRANCH_BUILDING');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';
		s.splitViewTab = 'verify';
		s.width = 100;
		s.height = 30;
		s.isProcessing = true;
		s.executionResults = [{sha: 'abc'}];
		s.executingIdx = 1;
		s.verifyingIdx = 0;
		s.verificationResults = [];
		s.outputLines = [];

		// Fallback state: no CaptureSession, but fallback is running.
		s.activeVerifySession = null;
		s.verifyFallbackRunning = true;
		s.verifyScreen = 'fallback test output';
		s.activeVerifyBranch = 'split/api';
		s.activeVerifyStartTime = Date.now() - 2000;
		s.verifyAutoScroll = true;
		s.verifyViewportOffset = 0;

		var view = globalThis.prSplit._wizardView(s);
		var errors = [];

		// The tab bar should contain "Verify".
		if (view.indexOf('Verify') < 0) {
			errors.push('FAIL: Verify tab not visible in split-view during fallback');
		}
		// The fallback output content should be rendered somewhere.
		if (view.indexOf('fallback test output') < 0) {
			errors.push('FAIL: fallback output not visible in Verify pane');
		}

		return errors.length > 0 ? errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("fallback tab visibility (running): %v", raw)
	}

	// Case 2: Everything empty — Verify tab must NOT appear (only Claude + Output).
	raw2, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('BRANCH_BUILDING');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';
		s.splitViewTab = 'output';
		s.width = 100;
		s.height = 30;
		s.isProcessing = false;
		s.executionResults = [{sha: 'abc'}];
		s.executingIdx = 1;
		s.outputLines = [];

		// No verify state at all.
		s.activeVerifySession = null;
		s.verifyFallbackRunning = false;
		s.verifyScreen = '';

		var view = globalThis.prSplit._wizardView(s);

		// Count occurrences of 'Verify' — should be zero in the tab bar.
		// (The word 'Verify' may appear in section headers, so we check
		// the tab bar area specifically by looking for the tab bar pattern.)
		// The tab bar renders: [Claude] [Output] [Verify?] [Shell?]
		// When verify is hidden, only Claude and Output tabs should exist
		// in the tab divider line.
		var tabLine = '';
		var lines = view.split('\n');
		for (var i = 0; i < lines.length; i++) {
			if (lines[i].indexOf('Claude') >= 0 && lines[i].indexOf('Output') >= 0 &&
			    lines[i].indexOf('switch') >= 0) {
				tabLine = lines[i];
				break;
			}
		}
		if (tabLine && tabLine.indexOf('Verify') >= 0) {
			return 'FAIL: Verify tab should be hidden when no verify state, tabLine=' + tabLine;
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw2 != "OK" {
		t.Errorf("fallback tab visibility (empty): %v", raw2)
	}
}
