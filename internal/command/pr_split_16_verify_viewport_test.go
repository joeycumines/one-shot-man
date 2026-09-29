package command

import (
	"encoding/json"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  T389: Verify tab pre-activation for baseline verification
// ---------------------------------------------------------------------------

// TestVerifyTabPreActivation_WithVerifyCommand_T389 verifies that when a real
// verify command is configured (not 'true'), startAnalysis pre-activates the
// Verify tab: sets verifyFallbackRunning=true, activeVerifyBranch='baseline',
// and splitViewTab='verify'.
func TestVerifyTabPreActivation_WithVerifyCommand_T389(t *testing.T) {
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

	raw, err := evalJS(`(async function() {
		globalThis.prSplit.runtime.baseBranch = 'main';
		globalThis.prSplit.runtime.dir = '` + escapeJSPath(dir) + `';
		globalThis.prSplit.runtime.strategy = 'directory';
		globalThis.prSplit.runtime.mode = 'heuristic';
		globalThis.prSplit.runtime.verifyCommand = 'make test';
		globalThis.prSplit.runtime.branchPrefix = 'split/';

		var s = initState('CONFIG');
		s.height = 30;
		s.splitViewEnabled = false;
		s.splitViewTab = 'agent';

		var r = await globalThis.prSplit._startAnalysis(s);
		s = r[0];

		return JSON.stringify({
			splitViewEnabled: s.splitViewEnabled,
			splitViewTab: s.splitViewTab,
			splitViewFocus: s.splitViewFocus,
			verifyFallbackRunning: s.verifyFallbackRunning,
			activeVerifyBranch: s.activeVerifyBranch,
			verifyScreen: s.verifyScreen,
			isProcessing: s.isProcessing,
			analysisRunning: s.analysisRunning
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		SplitViewEnabled      bool   `json:"splitViewEnabled"`
		SplitViewTab          string `json:"splitViewTab"`
		SplitViewFocus        string `json:"splitViewFocus"`
		VerifyFallbackRunning bool   `json:"verifyFallbackRunning"`
		ActiveVerifyBranch    string `json:"activeVerifyBranch"`
		VerifyScreen          string `json:"verifyScreen"`
		IsProcessing          bool   `json:"isProcessing"`
		AnalysisRunning       bool   `json:"analysisRunning"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("JSON parse error: %v (raw=%v)", err, raw)
	}

	if !result.SplitViewEnabled {
		t.Error("splitViewEnabled should be true")
	}
	if result.SplitViewTab != "verify" {
		t.Errorf("splitViewTab should be 'verify' (verify command configured), got %q", result.SplitViewTab)
	}
	if !result.VerifyFallbackRunning {
		t.Error("verifyFallbackRunning should be true (pre-activated for baseline verify)")
	}
	if result.ActiveVerifyBranch != "baseline" {
		t.Errorf("activeVerifyBranch should be 'baseline', got %q", result.ActiveVerifyBranch)
	}
	// verifyScreen should be initialized as empty string (not undefined)
	if result.VerifyScreen != "" {
		t.Errorf("verifyScreen should be empty initially, got %q", result.VerifyScreen)
	}
	if !result.IsProcessing {
		t.Error("isProcessing should be true")
	}
}

// TestVerifyTabPreActivation_NoVerifyCommand_T389 verifies that when
// verifyCommand='true' (skip), the auto-open falls back to Output tab
// without activating verifyFallbackRunning.
func TestVerifyTabPreActivation_NoVerifyCommand_T389(t *testing.T) {
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

	raw, err := evalJS(`(async function() {
		globalThis.prSplit.runtime.baseBranch = 'main';
		globalThis.prSplit.runtime.dir = '` + escapeJSPath(dir) + `';
		globalThis.prSplit.runtime.strategy = 'directory';
		globalThis.prSplit.runtime.mode = 'heuristic';
		globalThis.prSplit.runtime.verifyCommand = 'true';
		globalThis.prSplit.runtime.branchPrefix = 'split/';

		var s = initState('CONFIG');
		s.height = 30;
		s.splitViewEnabled = false;

		var r = await globalThis.prSplit._startAnalysis(s);
		s = r[0];

		return JSON.stringify({
			splitViewTab: s.splitViewTab,
			verifyFallbackRunning: s.verifyFallbackRunning || false
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		SplitViewTab          string `json:"splitViewTab"`
		VerifyFallbackRunning bool   `json:"verifyFallbackRunning"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("JSON parse error: %v (raw=%v)", err, raw)
	}

	if result.SplitViewTab != "output" {
		t.Errorf("splitViewTab should be 'output' (no verify), got %q", result.SplitViewTab)
	}
	if result.VerifyFallbackRunning {
		t.Error("verifyFallbackRunning should NOT be set when verifyCommand='true'")
	}
}

// TestVerifyTabVisible_DuringBaseline_T389 verifies that the Verify tab label
// appears in the rendered view when verifyFallbackRunning=true and
// activeVerifyBranch='baseline' (as set during baseline verification).
func TestVerifyTabVisible_DuringBaseline_T389(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('ANALYSIS');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';
		s.splitViewTab = 'verify';
		s.width = 100;
		s.height = 30;
		s.isProcessing = true;
		s.analysisProgress = 0.05;
		s.analysisSteps = [
			{ label: 'Verify baseline', active: true, done: false },
			{ label: 'Analyze diff', active: false, done: false },
			{ label: 'Group files', active: false, done: false }
		];
		s.outputLines = [];

		// Baseline verify state (T389: pre-activated).
		s.verifyFallbackRunning = true;
		s.activeVerifyBranch = 'baseline';
		s.verifyScreen = 'Running make test...';
		s.activeVerifyStartTime = Date.now() - 2000;
		s.verifyElapsedMs = 2000;

		var view = globalThis.prSplit._wizardView(s);
		var errors = [];

		// The tab bar MUST contain "Verify".
		if (view.indexOf('Verify') < 0) {
			errors.push('FAIL: Verify tab not visible in tab bar during baseline verify');
		}
		// The verify content MUST be visible.
		if (view.indexOf('Running make test') < 0) {
			errors.push('FAIL: baseline verify output not visible in Verify pane');
		}

		return errors.length > 0 ? errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("verify tab during baseline: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  T387: Verify CaptureSession resize propagation on WindowSize
// ---------------------------------------------------------------------------

// TestResizePropagation_VerifySession_T387 verifies that a WindowSize message
// calls activeVerifySession.resize(rows, cols) when split-view is enabled.
func TestResizePropagation_VerifySession_T387(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('ANALYSIS');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';
		s.splitViewTab = 'verify';
		s.width = 100;
		s.height = 30;

		// Mock activeVerifySession with a resize spy.
		var resizeCalls = [];
		s.activeVerifySession = {
			isAlive: function() { return true; },
			screen: function() { return ''; },
			resize: function(rows, cols) {
				resizeCalls.push({ rows: rows, cols: cols });
			}
		};

		// Send a WindowSize message.
		var r = globalThis.prSplit._wizardUpdate(
			{type: 'WindowSize', width: 120, height: 40}, s);
		s = r[0];

		var errors = [];
		if (s.width !== 120) errors.push('width should be 120, got ' + s.width);
		if (s.height !== 40) errors.push('height should be 40, got ' + s.height);
		if (resizeCalls.length !== 1) {
			errors.push('verifySession.resize should be called once, got ' +
				resizeCalls.length);
		} else {
			if (resizeCalls[0].rows < 3) {
				errors.push('verify rows too small: ' + resizeCalls[0].rows);
			}
			if (resizeCalls[0].cols < 20) {
				errors.push('verify cols too small: ' + resizeCalls[0].cols);
			}
		}

		return errors.length > 0 ? errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("resize propagation: %v", raw)
	}
}

// TestResizePropagation_NoSession_T387 verifies that WindowSize does NOT crash
// when activeVerifySession is null (no verify running).
func TestResizePropagation_NoSession_T387(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('ANALYSIS');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';
		s.width = 80;
		s.height = 24;
		s.activeVerifySession = null;

		// Should not crash.
		var r = globalThis.prSplit._wizardUpdate(
			{type: 'WindowSize', width: 100, height: 30}, s);
		s = r[0];

		if (s.width !== 100) return 'FAIL: width=' + s.width;
		if (s.height !== 30) return 'FAIL: height=' + s.height;
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("resize no-session: %v", raw)
	}
}
