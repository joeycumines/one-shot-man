package command

// T426: Unit tests for chunk 15c sub-renderer components.
//
// Covers the two untested sub-renderers extracted by T378:
//   - renderVerificationStatusList: 5-way branch status rendering
//   - renderLiveVerifyViewport: inline terminal viewport with scroll/pause
//
// Both functions mutate a `lines` array (push-style) rather than
// returning a string. Tests pass an empty array and inspect the result.

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  renderVerificationStatusList — 8 tests
// ---------------------------------------------------------------------------

// TestChunk15c_VerifyStatusList_SkippedBranch verifies that a skipped
// verification result renders with a dash icon and "(skipped)" suffix.
func TestChunk15c_VerifyStatusList_SkippedBranch(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var splits = [{name:'split/api'}];
		var s = {verificationResults: [{skipped:true}]};
		var lines = [];
		prSplit._renderVerificationStatusList(s, splits, lines);
		return lines.join('\n');
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, "skipped") {
		t.Error("expected '(skipped)' label for skipped branch")
	}
	if !strings.Contains(s, "split/api") {
		t.Error("expected branch name in output")
	}
}

// TestChunk15c_VerifyStatusList_PassedBranch verifies that a passed branch
// shows a check icon, duration, and optional manual override badge.
func TestChunk15c_VerifyStatusList_PassedBranch(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var splits = [{name:'split/types'}, {name:'split/manual'}];
		var s = {
			verificationResults: [
				{passed:true, duration:3500},
				{passed:true, duration:1000, manualOverride:true}
			]
		};
		var lines = [];
		prSplit._renderVerificationStatusList(s, splits, lines);
		return JSON.stringify({
			joined: lines.join('\n'),
			lineCount: lines.length
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, "3.5s") {
		t.Error("expected duration '3.5s' for first branch")
	}
	if !strings.Contains(s, "split/types") {
		t.Error("expected branch name 'split/types'")
	}
	if !strings.Contains(s, "manual") {
		t.Error("expected 'manual' override badge for second branch")
	}
}

// TestChunk15c_VerifyStatusList_FailedBranchWithError verifies that a failed
// branch shows the error message and a collapsed "Show Output" prompt.
func TestChunk15c_VerifyStatusList_FailedBranchWithError(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var splits = [{name:'split/broken'}];
		var s = {
			verificationResults: [{passed:false, error:'make: test failed', duration:5200}],
			verifyOutput: {'split/broken': ['FAIL TestFoo', 'exit status 1']}
		};
		var lines = [];
		prSplit._renderVerificationStatusList(s, splits, lines);
		return lines.join('\n');
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, "split/broken") {
		t.Error("expected branch name")
	}
	if !strings.Contains(s, "make: test failed") {
		t.Error("expected error message text")
	}
	if !strings.Contains(s, "5.2s") {
		t.Error("expected duration '5.2s'")
	}
	// Collapsed state — should show "Show Output" prompt.
	if !strings.Contains(s, "Show Output") {
		t.Error("collapsed error should show 'Show Output' prompt")
	}
	if !strings.Contains(s, "2 lines") {
		t.Error("should indicate output line count")
	}
}

// TestChunk15c_VerifyStatusList_ExpandedOutput verifies that when
// expandedVerifyBranch matches, the output lines are shown.
func TestChunk15c_VerifyStatusList_ExpandedOutput(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var splits = [{name:'split/fail'}];
		var s = {
			verificationResults: [{passed:false, error:'build failed'}],
			verifyOutput: {'split/fail': ['line-A', 'line-B', 'line-C']},
			expandedVerifyBranch: 'split/fail'
		};
		var lines = [];
		prSplit._renderVerificationStatusList(s, splits, lines);
		return lines.join('\n');
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, "Hide Output") {
		t.Error("expanded state should show 'Hide Output'")
	}
	if !strings.Contains(s, "line-A") {
		t.Error("expanded output should contain 'line-A'")
	}
	if !strings.Contains(s, "line-C") {
		t.Error("expanded output should contain 'line-C'")
	}
}

// TestChunk15c_VerifyStatusList_ExpandedOverflow verifies that output is
// capped at 20 lines with an overflow count.
func TestChunk15c_VerifyStatusList_ExpandedOverflow(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var splits = [{name:'split/big'}];
		var outputLines = [];
		for (var i = 0; i < 30; i++) outputLines.push('output-' + i);
		var s = {
			verificationResults: [{passed:false, error:'too much output'}],
			verifyOutput: {'split/big': outputLines},
			expandedVerifyBranch: 'split/big'
		};
		var lines = [];
		prSplit._renderVerificationStatusList(s, splits, lines);
		return lines.join('\n');
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, "10 more lines") {
		t.Error("30 lines capped at 20 should show '10 more lines'")
	}
	if !strings.Contains(s, "output-0") {
		t.Error("first output line should be included")
	}
	if !strings.Contains(s, "output-19") {
		t.Error("20th output line should be included")
	}
}

// TestChunk15c_VerifyStatusList_ActiveBranch verifies the currently-running
// branch shows active styling with elapsed time.
func TestChunk15c_VerifyStatusList_ActiveBranch(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var splits = [{name:'split/done'}, {name:'split/running'}];
		var s = {
			verificationResults: [{passed:true, duration:2000}],
			verifyingIdx: 1,
			isProcessing: true,
			verifyElapsedMs: 4500
		};
		var lines = [];
		prSplit._renderVerificationStatusList(s, splits, lines);
		return lines.join('\n');
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, "split/running") {
		t.Error("active branch name should appear")
	}
	if !strings.Contains(s, "(4s)") {
		t.Error("expected elapsed time '(4s)'")
	}
}

// TestChunk15c_VerifyStatusList_PendingBranch verifies that unprocessed
// branches show a dim pending style.
func TestChunk15c_VerifyStatusList_PendingBranch(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var splits = [{name:'split/first'}, {name:'split/second'}, {name:'split/third'}];
		var s = {
			verificationResults: [{passed:true, duration:1000}],
			verifyingIdx: 0,
			isProcessing: false
		};
		var lines = [];
		prSplit._renderVerificationStatusList(s, splits, lines);
		return JSON.stringify({
			joined: lines.join('\n'),
			lineCount: lines.length
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	// All 3 branches should appear.
	if !strings.Contains(s, "split/first") {
		t.Error("expected first branch")
	}
	if !strings.Contains(s, "split/second") {
		t.Error("expected second (pending) branch")
	}
	if !strings.Contains(s, "split/third") {
		t.Error("expected third (pending) branch")
	}
	// Should have 3 lines (one per branch).
	if !strings.Contains(s, `"lineCount":3`) {
		t.Errorf("expected 3 lines, got: %s", s)
	}
}

// TestChunk15c_VerifyStatusList_PreExistingBadge verifies that a failed
// result with preExisting=true shows the pre-existing warning badge.
func TestChunk15c_VerifyStatusList_PreExistingBadge(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var splits = [{name:'split/legacy'}];
		var s = {
			verificationResults: [{passed:false, preExisting:true, error:'test fail'}]
		};
		var lines = [];
		prSplit._renderVerificationStatusList(s, splits, lines);
		return lines.join('\n');
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, "pre-existing") {
		t.Error("expected 'pre-existing' badge for preExisting result")
	}
}

// ---------------------------------------------------------------------------
//  renderLiveVerifyViewport — 5 tests
// ---------------------------------------------------------------------------

// TestChunk15c_LiveViewport_EarlyReturn verifies that when neither
// activeVerifySession nor verifyScreen is set, no lines are added.
func TestChunk15c_LiveViewport_EarlyReturn(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var lines = [];
		prSplit._renderLiveVerifyViewport({}, lines);
		return lines.length;
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if n, ok := val.(int64); !ok || n != 0 {
		t.Errorf("expected 0 lines when no session or screen, got: %v", val)
	}
}

// TestChunk15c_LiveViewport_WithContent verifies that verify screen content
// renders inside a bordered viewport with title and auto-scroll.
func TestChunk15c_LiveViewport_WithContent(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var screenLines = [];
		for (var i = 0; i < 20; i++) screenLines.push('test output ' + i);
		var s = {
			activeVerifySession: true,
			verifyScreen: screenLines.join('\n'),
			activeVerifyBranch: 'split/api',
			verifyElapsedMs: 3500,
			verifyAutoScroll: true,
			width: 80
		};
		var lines = [];
		prSplit._renderLiveVerifyViewport(s, lines);
		return lines.join('\n');
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, "Verifying:") {
		t.Error("viewport title should contain 'Verifying:'")
	}
	if !strings.Contains(s, "split/api") {
		t.Error("viewport title should contain branch name")
	}
	if !strings.Contains(s, "3.5s") {
		t.Error("viewport title should contain elapsed time")
	}
	if !strings.Contains(s, "auto-scroll") {
		t.Error("auto-scroll should be indicated when verifyAutoScroll=true")
	}
	// Interactive controls.
	if !strings.Contains(s, "Pause") {
		t.Error("active session should show Pause button")
	}
	if !strings.Contains(s, "Ctrl+C") {
		t.Error("active session should show interrupt hint")
	}
}

// TestChunk15c_LiveViewport_ManualScroll verifies that with a positive
// verifyViewportOffset and auto-scroll off, a scroll percentage is shown.
func TestChunk15c_LiveViewport_ManualScroll(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var screenLines = [];
		for (var i = 0; i < 50; i++) screenLines.push('line ' + i);
		var s = {
			activeVerifySession: true,
			verifyScreen: screenLines.join('\n'),
			activeVerifyBranch: 'split/test',
			verifyElapsedMs: 1000,
			verifyAutoScroll: false,
			verifyViewportOffset: 20,
			width: 80
		};
		var lines = [];
		prSplit._renderLiveVerifyViewport(s, lines);
		var joined = lines.join('\n');
		return JSON.stringify({
			hasPct: joined.indexOf('%') >= 0,
			noAutoScroll: joined.indexOf('auto-scroll') < 0
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"hasPct":true`) {
		t.Error("manual scroll should show percentage indicator")
	}
	if !strings.Contains(s, `"noAutoScroll":true`) {
		t.Error("manual scroll should NOT show 'auto-scroll'")
	}
}

// TestChunk15c_LiveViewport_PausedState verifies that verifyPaused=true
// changes the title to show "PAUSED" and renders a Resume button.
func TestChunk15c_LiveViewport_PausedState(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var s = {
			activeVerifySession: true,
			verifyScreen: 'some output',
			activeVerifyBranch: 'split/paused',
			verifyElapsedMs: 7000,
			verifyPaused: true,
			width: 80
		};
		var lines = [];
		prSplit._renderLiveVerifyViewport(s, lines);
		return lines.join('\n');
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, "PAUSED") {
		t.Error("paused viewport should show 'PAUSED' in title")
	}
	if !strings.Contains(s, "Resume") {
		t.Error("paused viewport should show 'Resume' button")
	}
	// The non-paused Pause button text ("⏸ Pause", mixed case) must NOT
	// appear. The title uses "PAUSED" (all caps) which is distinct.
	if strings.Contains(s, "\u23f8 Pause") {
		t.Error("paused viewport should NOT render the non-paused Pause button")
	}
}

// TestChunk15c_LiveViewport_FallbackNoSession verifies that when
// activeVerifySession is falsy but verifyScreen has content, the
// fallback footer is rendered (no Pause/Resume buttons).
func TestChunk15c_LiveViewport_FallbackNoSession(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var s = {
			activeVerifySession: false,
			verifyScreen: 'leftover output line',
			activeVerifyBranch: 'split/done',
			verifyElapsedMs: 2000,
			width: 80
		};
		var lines = [];
		prSplit._renderLiveVerifyViewport(s, lines);
		return lines.join('\n');
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, "fallback") {
		t.Error("no-session fallback should show '(fallback output)' label")
	}
	if strings.Contains(s, "Pause") {
		t.Error("no-session fallback should NOT show Pause button")
	}
	if strings.Contains(s, "Resume") {
		t.Error("no-session fallback should NOT show Resume button")
	}
	if strings.Contains(s, "Ctrl+C") {
		t.Error("no-session fallback should NOT show interrupt hint")
	}
}
