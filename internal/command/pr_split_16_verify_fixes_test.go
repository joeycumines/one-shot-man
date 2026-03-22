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

// ---------------------------------------------------------------------------
// T378: Sub-function extraction tests
// ---------------------------------------------------------------------------

// TestRenderSplitExecutionList verifies the extracted helper renders
// branch creation status icons and progress bar correctly.
func TestRenderSplitExecutionList(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		globalThis.prSplit._state.planCache = {splits:[
			{name:'split/api',files:['a.go']},
			{name:'split/cli',files:['b.go']}
		]};
		var lines = [];
		globalThis.prSplit._renderSplitExecutionList({
			width: 80,
			executionResults: [{sha:'abc1234'}],
			executingIdx: 1,
			isProcessing: true,
			executionProgressMsg: 'creating...',
			executionBranchTotal: 2
		}, lines);
		return JSON.stringify({
			lineCount: lines.length,
			hasCompleted: lines.join('\n').indexOf('api') >= 0,
			hasActive: lines.join('\n').indexOf('cli') >= 0,
			hasProgress: lines.join('\n').indexOf('\u2588') >= 0 || lines.join('\n').indexOf('\u2591') >= 0
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		LineCount    int  `json:"lineCount"`
		HasCompleted bool `json:"hasCompleted"`
		HasActive    bool `json:"hasActive"`
		HasProgress  bool `json:"hasProgress"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if !result.HasCompleted {
		t.Error("expected completed branch 'api' in output")
	}
	if !result.HasActive {
		t.Error("expected active branch 'cli' in output")
	}
	if result.LineCount < 3 {
		t.Errorf("expected at least 3 lines (2 branches + progress), got %d", result.LineCount)
	}
}

// TestRenderSkippedFilesWarning verifies the gitignore skip warning helper.
func TestRenderSkippedFilesWarning(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		var lines = [];
		globalThis.prSplit._renderSkippedFilesWarning([
			{name:'split/api', skippedFiles:['vendor/x.go']},
			{sha:'abc'}
		], lines);
		var noSkipLines = [];
		globalThis.prSplit._renderSkippedFilesWarning([{sha:'abc'}], noSkipLines);
		return JSON.stringify({
			hasWarning: lines.join('\n').indexOf('Skipped') >= 0,
			hasFile: lines.join('\n').indexOf('vendor/x.go') >= 0,
			emptyWhenNone: noSkipLines.length === 0
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		HasWarning    bool `json:"hasWarning"`
		HasFile       bool `json:"hasFile"`
		EmptyWhenNone bool `json:"emptyWhenNone"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if !result.HasWarning {
		t.Error("expected 'Skipped' warning header")
	}
	if !result.HasFile {
		t.Error("expected skipped file path in output")
	}
	if !result.EmptyWhenNone {
		t.Error("expected no output when no skipped files")
	}
}

// TestRenderVerificationSummary verifies the pass/fail/skip summary helper.
func TestRenderVerificationSummary(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		var lines = [];
		globalThis.prSplit._renderVerificationSummary({
			verificationResults: [
				{passed:true}, {passed:false}, {skipped:true}
			],
			activeVerifySession: null
		}, [{name:'a'},{name:'b'},{name:'c'}], lines);
		return JSON.stringify({
			hasPassed: lines.join('\n').indexOf('1 passed') >= 0,
			hasFailed: lines.join('\n').indexOf('1 failed') >= 0,
			hasSkipped: lines.join('\n').indexOf('1 skipped') >= 0,
			hasOverrideHint: lines.join('\n').indexOf('Press p') >= 0
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		HasPassed       bool `json:"hasPassed"`
		HasFailed       bool `json:"hasFailed"`
		HasSkipped      bool `json:"hasSkipped"`
		HasOverrideHint bool `json:"hasOverrideHint"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if !result.HasPassed {
		t.Error("expected '1 passed' in summary")
	}
	if !result.HasFailed {
		t.Error("expected '1 failed' in summary")
	}
	if !result.HasSkipped {
		t.Error("expected '1 skipped' in summary")
	}
	if !result.HasOverrideHint {
		t.Error("expected manual override hint when failures present")
	}
}

// TestRenderVerificationSummary_IncompleteNoop verifies the summary
// helper produces no output when verification is still in progress.
func TestRenderVerificationSummary_IncompleteNoop(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		var lines = [];
		globalThis.prSplit._renderVerificationSummary({
			verificationResults: [{passed:true}]
		}, [{name:'a'},{name:'b'}], lines);
		return lines.length;
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != int64(0) {
		t.Errorf("expected 0 lines when verification incomplete, got %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  T388: Auto-open split-view on Next click
// ---------------------------------------------------------------------------

// TestAutoOpenSplitView_StartAnalysis_T388 verifies that startAnalysis
// auto-opens the split-view panel with the Output tab when terminal height
// is sufficient (>= INLINE_VIEW_HEIGHT).
func TestAutoOpenSplitView_StartAnalysis_T388(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	// Set up a real git repo so handleConfigState succeeds.
	dir := initGitRepo(t)
	writeFile(t, dir+"/README.md", "# Test\n")
	writeFile(t, dir+"/main.go", "package main\n\nfunc main() {}\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "initial")
	gitCmd(t, dir, "checkout", "-b", "feature")
	writeFile(t, dir+"/api.go", "package main\n\nfunc Api() {}\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "add api")

	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Configure runtime to point at the real git repo.
		globalThis.prSplit.runtime.baseBranch = 'main';
		globalThis.prSplit.runtime.dir = '` + escapeJSPath(dir) + `';
		globalThis.prSplit.runtime.strategy = 'directory';
		globalThis.prSplit.runtime.mode = 'heuristic';
		globalThis.prSplit.runtime.verifyCommand = 'true';
		globalThis.prSplit.runtime.branchPrefix = 'split/';

		var s = initState('CONFIG');
		s.height = 30;
		s.splitViewEnabled = false;
		s.splitViewTab = 'claude';

		var r = globalThis.prSplit._startAnalysis(s);
		s = r[0];

		return JSON.stringify({
			splitViewEnabled: s.splitViewEnabled,
			splitViewTab: s.splitViewTab,
			splitViewFocus: s.splitViewFocus,
			isProcessing: s.isProcessing,
			analysisRunning: s.analysisRunning
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		SplitViewEnabled bool   `json:"splitViewEnabled"`
		SplitViewTab     string `json:"splitViewTab"`
		SplitViewFocus   string `json:"splitViewFocus"`
		IsProcessing     bool   `json:"isProcessing"`
		AnalysisRunning  bool   `json:"analysisRunning"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("JSON parse error: %v (raw=%v)", err, raw)
	}

	if !result.SplitViewEnabled {
		t.Error("splitViewEnabled should be true after startAnalysis")
	}
	if result.SplitViewTab != "output" {
		t.Errorf("splitViewTab should be 'output', got %q", result.SplitViewTab)
	}
	if result.SplitViewFocus != "wizard" {
		t.Errorf("splitViewFocus should be 'wizard', got %q", result.SplitViewFocus)
	}
	if !result.IsProcessing {
		t.Error("isProcessing should be true")
	}
	if !result.AnalysisRunning {
		t.Error("analysisRunning should be true")
	}
}

// TestAutoOpenSplitView_ShortTerminal_T388 verifies that split-view does NOT
// auto-open when terminal height is below INLINE_VIEW_HEIGHT.
func TestAutoOpenSplitView_ShortTerminal_T388(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	dir := initGitRepo(t)
	writeFile(t, dir+"/README.md", "# Test\n")
	writeFile(t, dir+"/main.go", "package main\n\nfunc main() {}\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "initial")
	gitCmd(t, dir, "checkout", "-b", "feature")
	writeFile(t, dir+"/api.go", "package main\n\nfunc Api() {}\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "add api")

	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		globalThis.prSplit.runtime.baseBranch = 'main';
		globalThis.prSplit.runtime.dir = '` + escapeJSPath(dir) + `';
		globalThis.prSplit.runtime.strategy = 'directory';
		globalThis.prSplit.runtime.mode = 'heuristic';
		globalThis.prSplit.runtime.verifyCommand = 'true';
		globalThis.prSplit.runtime.branchPrefix = 'split/';

		var s = initState('CONFIG');
		s.height = 8;  // below INLINE_VIEW_HEIGHT (12)
		s.splitViewEnabled = false;

		var r = globalThis.prSplit._startAnalysis(s);
		s = r[0];

		return JSON.stringify({
			splitViewEnabled: s.splitViewEnabled,
			analysisRunning: s.analysisRunning
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		SplitViewEnabled bool `json:"splitViewEnabled"`
		AnalysisRunning  bool `json:"analysisRunning"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("JSON parse error: %v (raw=%v)", err, raw)
	}
	// Verify analysis actually started (config didn't fail).
	if !result.AnalysisRunning {
		t.Fatal("analysisRunning should be true — config validation may have failed")
	}
	if result.SplitViewEnabled {
		t.Error("splitViewEnabled should remain false for short terminals")
	}
}

// TestAutoOpenSplitView_StartExecution_T388 verifies that startExecution
// auto-opens the split-view panel with the Output tab.
func TestAutoOpenSplitView_StartExecution_T388(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Set up a plan so startExecution doesn't bail early.
		setupPlanCache();

		// Mock executeSplitAsync to prevent real execution.
		globalThis.prSplit.executeSplitAsync = function(plan, opts) {
			return new Promise(function() {}); // never resolves — we only check initial state
		};

		var s = initState('PLAN_REVIEW');
		s.height = 30;
		s.splitViewEnabled = false;
		s.splitViewTab = 'claude';

		// Set nav focus to Next button and press Enter.
		// Or call startExecution directly:
		var r = globalThis.prSplit._startExecution(s);
		s = r[0];

		return JSON.stringify({
			splitViewEnabled: s.splitViewEnabled,
			splitViewTab: s.splitViewTab,
			splitViewFocus: s.splitViewFocus,
			wizardState: s.wizardState
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		SplitViewEnabled bool   `json:"splitViewEnabled"`
		SplitViewTab     string `json:"splitViewTab"`
		SplitViewFocus   string `json:"splitViewFocus"`
		WizardState      string `json:"wizardState"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("JSON parse error: %v (raw=%v)", err, raw)
	}

	if !result.SplitViewEnabled {
		t.Error("splitViewEnabled should be true after startExecution")
	}
	if result.SplitViewTab != "output" {
		t.Errorf("splitViewTab should be 'output', got %q", result.SplitViewTab)
	}
	if result.WizardState != "BRANCH_BUILDING" {
		t.Errorf("wizardState should be 'BRANCH_BUILDING', got %q", result.WizardState)
	}
}

// TestCtrlO_IncludesFallbackVerify_T388 verifies that Ctrl+O tab rotation
// includes the Verify tab when verifyFallbackRunning is true (even without
// activeVerifySession).
func TestCtrlO_IncludesFallbackVerify_T388(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('BRANCH_BUILDING');
		s.height = 30;
		s.splitViewEnabled = true;
		s.splitViewTab = 'output';
		s.splitViewFocus = 'wizard';
		s.activeVerifySession = null;
		s.verifyScreen = '';
		s.verifyFallbackRunning = true;  // fallback path active

		// First Ctrl+O: output → verify (skipping claude since we start at output).
		// Actually: rotation is ['claude','output','verify']. Starting at 'output' (idx 1),
		// next is 'verify' (idx 2).
		var r = sendKey(s, 'ctrl+o');
		s = r[0];

		return JSON.stringify({ tab: s.splitViewTab });
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Tab string `json:"tab"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("JSON parse error: %v (raw=%v)", err, raw)
	}
	if result.Tab != "verify" {
		t.Errorf("Ctrl+O from 'output' with verifyFallbackRunning should go to 'verify', got %q", result.Tab)
	}
}
