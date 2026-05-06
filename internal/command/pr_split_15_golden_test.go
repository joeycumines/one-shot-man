package command

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  T346: Golden file tests for PR Split TUI visual regression
// ---------------------------------------------------------------------------

var updateGolden = flag.Bool("update-golden", false, "update golden files")

// skipGoldenWindows skips golden tests on Windows as lipgloss rendering
// may differ slightly across platforms (ANSI handling, etc.).
func skipGoldenWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("skipping golden test on Windows (platform-specific rendering)")
	}
}

func testGolden(t *testing.T, name string, got string) {
	t.Helper()
	goldenFile := filepath.Join("testdata", "golden", name+".golden")

	if *updateGolden {
		dir := filepath.Dir(goldenFile)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenFile, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("updated %s", goldenFile)
		return
	}

	want, err := os.ReadFile(goldenFile)
	if os.IsNotExist(err) {
		t.Fatalf("golden file %s does not exist; run with -update-golden to create", goldenFile)
	}
	if err != nil {
		t.Fatal(err)
	}

	if got != string(want) {
		t.Errorf("golden mismatch for %s:\n--- want ---\n%s\n--- got ---\n%s", name, string(want), got)
	}
}

// TestGolden_VerifyPane_Running renders the verify pane with an active
// session in running state and compares against the stored golden file.
func TestGolden_VerifyPane_Running(t *testing.T) {
	skipGoldenWindows(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._renderVerifyPane({
		verifyScreen: 'Running tests...\ntest_utils.go:15: ok\ntest_main.go:42: ok\ntest_api.go:8: FAIL',
		activeVerifySession: true,
		splitViewFocus: 'claude',
		splitViewTab: 'verify',
		verifyPaused: false,
		verifyViewportOffset: 0,
		verifyAutoScroll: true,
		activeVerifyBranch: 'split/01-types',
		verifyElapsedMs: 12500
	}, 80, 20)`)
	if err != nil {
		t.Fatal(err)
	}
	testGolden(t, "verify-pane-running", raw.(string))
}

// TestGolden_VerifyPane_Paused renders the verify pane in paused state
// and compares against the stored golden file.
func TestGolden_VerifyPane_Paused(t *testing.T) {
	skipGoldenWindows(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._renderVerifyPane({
		verifyScreen: 'Running tests...\ntest_utils.go:15: ok\ntest_main.go:42: ok\ntest_api.go:8: FAIL\ntest_db.go:99: ok',
		activeVerifySession: true,
		splitViewFocus: 'wizard',
		splitViewTab: 'verify',
		verifyPaused: true,
		verifyViewportOffset: 0,
		verifyAutoScroll: true,
		activeVerifyBranch: 'split/02-impl',
		verifyElapsedMs: 45200
	}, 80, 20)`)
	if err != nil {
		t.Fatal(err)
	}
	testGolden(t, "verify-pane-paused", raw.(string))
}

// Task 8: TestGolden_ShellPane_Active removed — shell tab unified into verify pane.

// Task 8: TestGolden_TabBar_AllTabs renders the pane divider tab bar with
// all 3 tabs (Claude, Output, Verify) visible and compares against the golden file.
func TestGolden_TabBar_AllTabs(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('BRANCH_BUILDING');
		s.splitViewEnabled = true;
		s.splitViewTab = 'claude';
		s.splitViewFocus = 'wizard';
		s.width = 80;
		s.height = 24;
		s.isProcessing = true;
		s.executionResults = [{sha: 'abc'}];
		s.executingIdx = 1;
		s.verifyingIdx = 0;
		s.verificationResults = [];
		s.outputLines = [];

		// Mock verify session (makes Verify tab visible).
		s.activeVerifySession = {
			isDone: function() { return false; },
			exitCode: function() { return 0; },
			output: function() { return 'Running...'; },
			screen: function() { return 'Running...'; },
			write: function() {},
			close: function() {},
			kill: function() {},
			pause: function() {},
			resume: function() {}
		};
		s.activeVerifyBranch = 'split/01-types';
		s.activeVerifyStartTime = Date.now();
		s.verifyElapsedMs = 0;
		s.verifyScreen = '';
		s.verifyViewportOffset = 0;
		s.verifyAutoScroll = true;

		// Task 8: Shell tab removed — only Claude, Output, Verify tabs.

		var view = globalThis.prSplit._wizardView(s);
		var lines = view.split('\n');
		for (var i = 0; i < lines.length; i++) {
			if (lines[i].indexOf('\u2524') >= 0) {
				return lines[i];
			}
		}
		return 'FAIL: pane divider line not found';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	got := raw.(string)
	if got == "FAIL: pane divider line not found" {
		t.Fatal(got)
	}
	testGolden(t, "tab-bar-all-tabs", got)
}

// TestGolden_TabBar_VerifyOnly renders the pane divider tab bar with only
// Claude, Output, and Verify tabs (no Shell) and compares against the golden file.
func TestGolden_TabBar_VerifyOnly(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('BRANCH_BUILDING');
		s.splitViewEnabled = true;
		s.splitViewTab = 'claude';
		s.splitViewFocus = 'wizard';
		s.width = 80;
		s.height = 24;
		s.isProcessing = true;
		s.executionResults = [{sha: 'abc'}];
		s.executingIdx = 1;
		s.verifyingIdx = 0;
		s.verificationResults = [];
		s.outputLines = [];

		// Mock verify session (makes Verify tab visible).
		s.activeVerifySession = {
			isDone: function() { return false; },
			exitCode: function() { return 0; },
			output: function() { return 'Running...'; },
			screen: function() { return 'Running...'; },
			write: function() {},
			close: function() {},
			kill: function() {},
			pause: function() {},
			resume: function() {}
		};
		s.activeVerifyBranch = 'split/01-types';
		s.activeVerifyStartTime = Date.now();
		s.verifyElapsedMs = 0;
		s.verifyScreen = '';
		s.verifyViewportOffset = 0;
		s.verifyAutoScroll = true;

		// Task 8: No shell session — Shell tab does not exist.

		var view = globalThis.prSplit._wizardView(s);
		var lines = view.split('\n');
		for (var i = 0; i < lines.length; i++) {
			if (lines[i].indexOf('\u2524') >= 0) {
				return lines[i];
			}
		}
		return 'FAIL: pane divider line not found';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	got := raw.(string)
	if got == "FAIL: pane divider line not found" {
		t.Fatal(got)
	}
	testGolden(t, "tab-bar-verify-only", got)
}
