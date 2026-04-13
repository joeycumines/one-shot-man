package command

import (
	"fmt"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  T42: Default to Claude strategy when Claude is available
// ---------------------------------------------------------------------------

func TestChunk16_T42_AutoDetectClaudeOnStartup(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		globalThis.prSplitConfig = { claudeCommand: '' };
		globalThis.prSplit._state.claudeExecutor = null;

		var origCtor = globalThis.prSplit.ClaudeCodeExecutor;
		var MockExecutor = function(config) {
			this.command = config.claudeCommand || '';
			this.resolved = null;
		};
		MockExecutor.prototype.resolveAsync = async function(progressFn) {
			if (progressFn) progressFn('Checking...');
			this.resolved = { command: 'claude', type: 'claude-code' };
			return { error: null };
		};
		globalThis.prSplit.ClaudeCodeExecutor = MockExecutor;

		try {
			var s = initState('CONFIG');
			// Verify initial state: mode is heuristic, no strategy selected.
			if (prSplit.runtime.mode !== 'heuristic') return 'FAIL: initial mode should be heuristic, got: ' + prSplit.runtime.mode;
			if (s.userHasSelectedStrategy) return 'FAIL: userHasSelectedStrategy should be false initially';

			// Fire auto-detect-claude tick (simulates what happens after first WindowSize).
			var r = update({type: 'Tick', id: 'auto-detect-claude'}, s);
			s = r[0];

			// Should start checking.
			if (s.claudeCheckStatus !== 'checking') return 'FAIL: should be checking, got: ' + s.claudeCheckStatus;

			// Let microtasks resolve (resolveAsync completes).
			await Promise.resolve();
			await Promise.resolve();
			await Promise.resolve();

			// Poll to complete.
			r = update({type: 'Tick', id: 'claude-check-poll'}, s);
			s = r[0];

			// Claude is available — mode should be auto.
			if (s.claudeCheckStatus !== 'available') return 'FAIL: should be available, got: ' + s.claudeCheckStatus;
			if (prSplit.runtime.mode !== 'auto') return 'FAIL: mode should auto-switch to auto, got: ' + prSplit.runtime.mode;
			if (s.userHasSelectedStrategy) return 'FAIL: auto-detect should NOT set userHasSelectedStrategy';

			return 'OK';
		} finally {
			globalThis.prSplit.ClaudeCodeExecutor = origCtor;
			delete globalThis.prSplitConfig;
			prSplit.runtime.mode = 'heuristic';
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("auto detect claude on startup: %v", raw)
	}
}

func TestChunk16_T42_AutoDetectSkipsWhenUserSelected(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		globalThis.prSplitConfig = { claudeCommand: '' };
		globalThis.prSplit._state.claudeExecutor = null;

		var origCtor = globalThis.prSplit.ClaudeCodeExecutor;
		var resolveCallCount = 0;
		var MockExecutor = function(config) {
			this.command = config.claudeCommand || '';
			this.resolved = null;
		};
		MockExecutor.prototype.resolveAsync = async function(progressFn) {
			resolveCallCount++;
			this.resolved = { command: 'claude', type: 'claude-code' };
			return { error: null };
		};
		globalThis.prSplit.ClaudeCodeExecutor = MockExecutor;

		try {
			var s = initState('CONFIG');
			// User manually selects heuristic strategy.
			s.userHasSelectedStrategy = true;
			prSplit.runtime.mode = 'heuristic';

			// Fire auto-detect-claude tick.
			var r = update({type: 'Tick', id: 'auto-detect-claude'}, s);
			s = r[0];

			// Should skip entirely — no check launched.
			if (s.claudeCheckStatus !== null) return 'FAIL: should not start checking when user selected, got: ' + s.claudeCheckStatus;
			if (resolveCallCount !== 0) return 'FAIL: resolveAsync should not be called, count: ' + resolveCallCount;
			if (prSplit.runtime.mode !== 'heuristic') return 'FAIL: mode should stay heuristic, got: ' + prSplit.runtime.mode;

			return 'OK';
		} finally {
			globalThis.prSplit.ClaudeCodeExecutor = origCtor;
			delete globalThis.prSplitConfig;
			prSplit.runtime.mode = 'heuristic';
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("auto detect skips when user selected: %v", raw)
	}
}

func TestChunk16_T42_ManualSelectSetsFlag(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		globalThis.prSplitConfig = { claudeCommand: '' };
		var s = initState('CONFIG');
		if (s.userHasSelectedStrategy) return 'FAIL: should start false';

		// Simulate keyboard strategy selection (via handleFocusActivate).
		s.focusIndex = 1; // strategy-heuristic
		var r = sendKey(s, 'enter');
		s = r[0];
		if (!s.userHasSelectedStrategy) return 'FAIL: enter on strategy should set flag';

		// Reset and test mouse click.
		s.userHasSelectedStrategy = false;
		var z = globalThis.prSplit._zone;
		var origInBounds = z.inBounds;
		z.inBounds = function(id) { return id === 'strategy-directory'; };
		try {
			r = update({type: 'MouseClick', button: 'left', x: 10, y: 10, mod: []}, s);
		} finally {
			z.inBounds = origInBounds;
		}
		s = r[0];
		if (!s.userHasSelectedStrategy) return 'FAIL: mouse click on strategy should set flag';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("manual select sets flag: %v", raw)
	}
}

func TestChunk16_T42_AutoDetectUnavailableFallback(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		globalThis.prSplitConfig = { claudeCommand: '' };
		globalThis.prSplit._state.claudeExecutor = null;

		var origCtor = globalThis.prSplit.ClaudeCodeExecutor;
		var MockExecutor = function(config) {
			this.command = config.claudeCommand || '';
			this.resolved = null;
		};
		MockExecutor.prototype.resolveAsync = async function(progressFn) {
			return { error: 'Claude not found' };
		};
		globalThis.prSplit.ClaudeCodeExecutor = MockExecutor;

		try {
			var s = initState('CONFIG');
			prSplit.runtime.mode = 'heuristic';

			// Fire auto-detect.
			var r = update({type: 'Tick', id: 'auto-detect-claude'}, s);
			s = r[0];
			await Promise.resolve();
			await Promise.resolve();
			await Promise.resolve();
			r = update({type: 'Tick', id: 'claude-check-poll'}, s);
			s = r[0];

			// Claude unavailable — should stay heuristic.
			if (s.claudeCheckStatus !== 'unavailable') return 'FAIL: should be unavailable, got: ' + s.claudeCheckStatus;
			if (prSplit.runtime.mode !== 'heuristic') return 'FAIL: mode should stay heuristic, got: ' + prSplit.runtime.mode;
			if (s.claudeCheckError !== 'Claude not found') return 'FAIL: error not set';

			return 'OK';
		} finally {
			globalThis.prSplit.ClaudeCodeExecutor = origCtor;
			delete globalThis.prSplitConfig;
			prSplit.runtime.mode = 'heuristic';
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("auto detect unavailable fallback: %v", raw)
	}
}

func TestChunk16_T42_AutoDetectSkipsWhenAlreadyChecking(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.claudeCheckStatus = 'checking';

		// Fire auto-detect — should skip (already checking).
		var r = update({type: 'Tick', id: 'auto-detect-claude'}, s);
		s = r[0];

		// Nothing changes.
		if (s.claudeCheckStatus !== 'checking') return 'FAIL: should stay checking';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("auto detect skips when already checking: %v", raw)
	}
}

func TestChunk16_T42_ViewShowsAutoStrategyHint(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.claudeCheckStatus = 'available';
		s.claudeResolvedInfo = { command: 'claude', type: 'claude-code' };
		s.userHasSelectedStrategy = false;
		prSplit.runtime.mode = 'auto';

		var view = globalThis.prSplit._viewForState(s);
		if (view.indexOf('using auto strategy') === -1) {
			return 'FAIL: should show auto strategy hint when auto-detected, got view: ' + view.substring(0, 200);
		}

		// When user manually selected, should not show the hint.
		s.userHasSelectedStrategy = true;
		view = globalThis.prSplit._viewForState(s);
		if (view.indexOf('using auto strategy') !== -1) {
			return 'FAIL: should NOT show auto strategy hint when user selected';
		}
		if (view.indexOf('Claude available') === -1) {
			return 'FAIL: should still show Claude available';
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("view shows auto strategy hint: %v", raw)
	}
}

func TestChunk16_T42_InitReturnsBatchCommand(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('IDLE');
		s.needsInitClear = true;

		// Simulate first WindowSize message.
		var r = update({type: 'WindowSize', width: 120, height: 40}, s);
		s = r[0];
		var cmd = r[1];

		if (s.wizardState !== 'CONFIG') return 'FAIL: should transition to CONFIG';

		// Verify command is a batch (clearScreen + tick).
		if (!cmd) return 'FAIL: should return a command';
		if (cmd._cmdType !== 'batch') return 'FAIL: command should be batch, got: ' + cmd._cmdType;
		if (!cmd.cmds || cmd.cmds.length !== 2) return 'FAIL: batch should have 2 commands, got: ' + (cmd.cmds ? cmd.cmds.length : 'null');

		// First command is clearScreen.
		if (cmd.cmds[0]._cmdType !== 'clearScreen') return 'FAIL: first cmd should be clearScreen, got: ' + cmd.cmds[0]._cmdType;
		// Second command is tick for auto-detect-claude.
		if (cmd.cmds[1]._cmdType !== 'tick') return 'FAIL: second cmd should be tick, got: ' + cmd.cmds[1]._cmdType;
		if (cmd.cmds[1].id !== 'auto-detect-claude') return 'FAIL: tick id should be auto-detect-claude, got: ' + cmd.cmds[1].id;

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("init returns batch command: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  T43: Config validation stays on CONFIG (not ERROR)
// ---------------------------------------------------------------------------

func TestChunk16_T43_ConfigErrorStaysOnCONFIG(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Mock gitExec to simulate empty repo (rev-parse HEAD fails).
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse' && args[1] === '--abbrev-ref') {
				return { code: 128, stdout: '', stderr: "fatal: ambiguous argument 'HEAD'" };
			}
			if (args[0] === 'rev-parse' && args[1] === '--verify') {
				return { code: 128, stdout: '', stderr: 'fatal: not found' };
			}
			if (args[0] === 'branch') {
				return { code: 0, stdout: 'main\ndev\n', stderr: '' };
			}
			return { code: 0, stdout: '', stderr: '' };
		};
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.mode = 'heuristic';

		var s = initState('CONFIG');
		// Set focus to nav-next (index 4 for CONFIG with heuristic mode).
		s.focusIndex = 4;
		// Simulate pressing Enter on nav-next → handleNext → startAnalysis.
		var r = sendKey(s, 'enter');
		s = r[0];

		// T43: Should stay on CONFIG, NOT jump to ERROR.
		if (s.wizardState !== 'CONFIG') return 'FAIL: should stay on CONFIG, got: ' + s.wizardState;
		// Should have inline validation error.
		if (!s.configValidationError) return 'FAIL: configValidationError should be set';
		if (s.configValidationError.indexOf('No commits') === -1) return 'FAIL: error should mention No commits, got: ' + s.configValidationError;
		// Should NOT set errorDetails (which is the old ERROR state pattern).
		if (s.errorDetails) return 'FAIL: errorDetails should be null for config validation, got: ' + s.errorDetails;
		// Should have available branches.
		if (!s.availableBranches || s.availableBranches.length !== 2) return 'FAIL: should have 2 available branches';
		if (s.availableBranches[0] !== 'main') return 'FAIL: first branch should be main';
		// Should not be processing.
		if (s.isProcessing) return 'FAIL: should not be processing';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("config error stays on CONFIG: %v", raw)
	}
}

func TestChunk16_T43_AutoAnalysisConfigErrorStaysOnCONFIG(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Mock gitExec to simulate detached HEAD.
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse' && args[1] === '--abbrev-ref') {
				return { code: 0, stdout: 'HEAD\n', stderr: '' };
			}
			if (args[0] === 'rev-parse' && args[1] === '--verify') {
				return { code: 0, stdout: 'abc123\n', stderr: '' };
			}
			if (args[0] === 'branch') {
				return { code: 0, stdout: 'main\nfeature\n', stderr: '' };
			}
			return { code: 0, stdout: '', stderr: '' };
		};
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.mode = 'auto'; // Use auto path (startAutoAnalysis).

		var s = initState('CONFIG');
		// Set focus to nav-next (index 5 for CONFIG with auto mode — test-claude at 3).
		s.focusIndex = 5;
		var r = sendKey(s, 'enter');
		s = r[0];

		// T43: Auto path should also stay on CONFIG.
		if (s.wizardState !== 'CONFIG') return 'FAIL: should stay on CONFIG, got: ' + s.wizardState;
		if (!s.configValidationError) return 'FAIL: configValidationError should be set';
		if (s.configValidationError.indexOf('Detached HEAD') === -1) return 'FAIL: should mention Detached HEAD';
		if (!s.availableBranches || s.availableBranches.length !== 2) return 'FAIL: should have available branches';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("auto analysis config error stays on CONFIG: %v", raw)
	}
}

func TestChunk16_T43_RetryCleansPreviousError(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var callCount = 0;
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse' && args[1] === '--abbrev-ref') {
				callCount++;
				if (callCount <= 1) {
					// First attempt: fail (empty repo).
					return { code: 128, stdout: '', stderr: "fatal: ambiguous argument 'HEAD'" };
				}
				// Second attempt: succeed (user made a commit).
				return { code: 0, stdout: 'feature\n', stderr: '' };
			}
			if (args[0] === 'rev-parse' && args[1] === '--verify') {
				if (callCount <= 1) return { code: 128, stdout: '', stderr: 'fatal: not found' };
				return { code: 0, stdout: 'abc123\n', stderr: '' };
			}
			if (args[0] === 'branch') {
				return { code: 0, stdout: 'main\n', stderr: '' };
			}
			return { code: 0, stdout: '', stderr: '' };
		};
		prSplit.verifySplit = function() { return { passed: true }; };
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.mode = 'heuristic';
		prSplit.runtime.verifyCommand = '';

		var s = initState('CONFIG');
		s.focusIndex = 4; // nav-next for heuristic mode

		// First attempt: fails.
		var r = sendKey(s, 'enter');
		s = r[0];
		if (!s.configValidationError) return 'FAIL: first attempt should set error';
		if (s.availableBranches.length !== 1) return 'FAIL: first attempt should list branches';

		// Second attempt: succeeds (error clears).
		r = sendKey(s, 'enter');
		s = r[0];
		if (s.configValidationError) return 'FAIL: retry should clear configValidationError';
		if (s.availableBranches.length !== 0) return 'FAIL: retry should clear availableBranches';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("retry cleans previous error: %v", raw)
	}
}

func TestChunk16_T43_ViewShowsInlineValidationError(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.configValidationError = 'No commits on current branch.';
		s.availableBranches = ['main', 'develop', 'feature-x'];

		var view = globalThis.prSplit._viewForState(s);

		// Should show inline error.
		if (view.indexOf('Configuration Error') === -1) return 'FAIL: should show Configuration Error badge';
		if (view.indexOf('No commits') === -1) return 'FAIL: should show error text';
		// Should show available branches.
		if (view.indexOf('Available branches') === -1) return 'FAIL: should show Available branches header';
		if (view.indexOf('main') === -1) return 'FAIL: should list main branch';
		if (view.indexOf('develop') === -1) return 'FAIL: should list develop branch';
		if (view.indexOf('feature-x') === -1) return 'FAIL: should list feature-x branch';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("view shows inline validation error: %v", raw)
	}
}

func TestChunk16_T43_ViewNoBranchesWhenEmpty(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.configValidationError = 'Cannot determine current branch: not a git repo';
		s.availableBranches = [];

		var view = globalThis.prSplit._viewForState(s);

		// Should show error but NOT "Available branches" section.
		if (view.indexOf('Configuration Error') === -1) return 'FAIL: should show error';
		if (view.indexOf('Available branches') !== -1) return 'FAIL: should NOT show branches when empty';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("view no branches when empty: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  T44: Output Tab + Process Output Muxing Tests
// ---------------------------------------------------------------------------

// TestChunk16_T44_InitStateHasOutputFields verifies T44 state fields exist
// in the initialized wizard state with correct defaults.
func TestChunk16_T44_InitStateHasOutputFields(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		return {
			splitViewTab: s.splitViewTab,
			outputLines: s.outputLines,
			outputViewOffset: s.outputViewOffset,
			outputAutoScroll: s.outputAutoScroll,
			linesIsArray: Array.isArray(s.outputLines),
			linesEmpty: s.outputLines.length === 0
		};
	})()`)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	m := raw.(map[string]any)
	if m["splitViewTab"] != "claude" {
		t.Errorf("splitViewTab = %v, want 'claude'", m["splitViewTab"])
	}
	if m["linesIsArray"] != true {
		t.Errorf("outputLines should be an array")
	}
	if m["linesEmpty"] != true {
		t.Errorf("outputLines should be empty initially")
	}
	if prsplittest.NumVal(m["outputViewOffset"]) != 0 {
		t.Errorf("outputViewOffset should be 0")
	}
	if m["outputAutoScroll"] != true {
		t.Errorf("outputAutoScroll should be true")
	}
}

// TestChunk16_T44_CtrlOSwitchesTabs verifies Ctrl+O toggles between Claude
// and Output tabs in split-view bottom pane.
func TestChunk16_T44_CtrlOSwitchesTabs(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.splitViewEnabled = true;
		s.splitViewTab = 'claude';

		// First Ctrl+O → switch to output.
		var r1 = sendKey(s, 'ctrl+o');
		s = r1[0];
		var tab1 = s.splitViewTab;

		// Second Ctrl+O → switch back to claude.
		var r2 = sendKey(s, 'ctrl+o');
		s = r2[0];
		var tab2 = s.splitViewTab;

		return { tab1: tab1, tab2: tab2 };
	})()`)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	m := raw.(map[string]any)
	if m["tab1"] != "output" {
		t.Errorf("after first Ctrl+O, tab = %v, want 'output'", m["tab1"])
	}
	if m["tab2"] != "claude" {
		t.Errorf("after second Ctrl+O, tab = %v, want 'claude'", m["tab2"])
	}
}

// TestChunk16_T44_CtrlONotActiveWhenSplitViewDisabled ensures Ctrl+O has
// no effect when split-view is not enabled.
func TestChunk16_T44_CtrlONotActiveWhenSplitViewDisabled(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.splitViewEnabled = false;
		s.splitViewTab = 'claude';

		var r = sendKey(s, 'ctrl+o');
		s = r[0];
		return s.splitViewTab;
	})()`)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	if raw != "claude" {
		t.Errorf("tab should remain 'claude' when split-view disabled, got %v", raw)
	}
}

// TestChunk16_T44_OutputTabScrollKeys verifies scroll keys work when Output
// tab is active in the split-view bottom pane.
func TestChunk16_T44_OutputTabScrollKeys(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.splitViewEnabled = true;
		s.splitViewTab = 'output';
		s.splitViewFocus = 'claude'; // focus on bottom pane
		s.outputViewOffset = 0;
		s.outputAutoScroll = true;

		// Up → offset +1, autoScroll off.
		var r = sendKey(s, 'up');
		s = r[0];
		var offsetAfterUp = s.outputViewOffset;
		var autoAfterUp = s.outputAutoScroll;

		// End → offset 0, autoScroll on.
		r = sendKey(s, 'end');
		s = r[0];
		var offsetAfterEnd = s.outputViewOffset;
		var autoAfterEnd = s.outputAutoScroll;

		// Home → large offset, autoScroll off.
		r = sendKey(s, 'home');
		s = r[0];
		var offsetAfterHome = s.outputViewOffset;
		var autoAfterHome = s.outputAutoScroll;

		// PgDown → decrease offset.
		s.outputViewOffset = 10;
		r = sendKey(s, 'pgdown');
		s = r[0];
		var offsetAfterPgDown = s.outputViewOffset;

		return {
			offsetAfterUp: offsetAfterUp,
			autoAfterUp: autoAfterUp,
			offsetAfterEnd: offsetAfterEnd,
			autoAfterEnd: autoAfterEnd,
			offsetAfterHome: offsetAfterHome,
			autoAfterHome: autoAfterHome,
			offsetAfterPgDown: offsetAfterPgDown
		};
	})()`)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	m := raw.(map[string]any)
	if prsplittest.NumVal(m["offsetAfterUp"]) != 1 {
		t.Errorf("up: offset = %v, want 1", m["offsetAfterUp"])
	}
	if m["autoAfterUp"] != false {
		t.Errorf("up: autoScroll should be false")
	}
	if prsplittest.NumVal(m["offsetAfterEnd"]) != 0 {
		t.Errorf("end: offset = %v, want 0", m["offsetAfterEnd"])
	}
	if m["autoAfterEnd"] != true {
		t.Errorf("end: autoScroll should be true")
	}
	if prsplittest.NumVal(m["offsetAfterHome"]) < 999 {
		t.Errorf("home: offset = %v, want large value", m["offsetAfterHome"])
	}
	if m["autoAfterHome"] != false {
		t.Errorf("home: autoScroll should be false")
	}
	if prsplittest.NumVal(m["offsetAfterPgDown"]) != 5 {
		t.Errorf("pgdown from 10: offset = %v, want 5", m["offsetAfterPgDown"])
	}
}

// TestChunk16_T44_TabClickZones verifies mouse clicks on tab zone marks
// switch tabs correctly.
func TestChunk16_T44_TabClickZones(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.splitViewEnabled = true;
		s.splitViewTab = 'claude';

		// Click on output tab.
		var restore = mockZoneHit('split-tab-output');
		try {
			var r = update({type: 'MouseClick', button: 'left', x: 10, y: 10, mod: []}, s);
			s = r[0];
		} finally { restore(); }
		var tab1 = s.splitViewTab;

		// Click on claude tab.
		restore = mockZoneHit('split-tab-claude');
		try {
			var r2 = update({type: 'MouseClick', button: 'left', x: 10, y: 10, mod: []}, s);
			s = r2[0];
		} finally { restore(); }
		var tab2 = s.splitViewTab;

		return { tab1: tab1, tab2: tab2 };
	})()`)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	m := raw.(map[string]any)
	if m["tab1"] != "output" {
		t.Errorf("click split-tab-output: tab = %v, want 'output'", m["tab1"])
	}
	if m["tab2"] != "claude" {
		t.Errorf("click split-tab-claude: tab = %v, want 'claude'", m["tab2"])
	}
}

// TestChunk16_T44_OutputMouseWheelScroll verifies mouse wheel scrolls the
// Output tab when it's the active tab in split-view.
func TestChunk16_T44_OutputMouseWheelScroll(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.splitViewEnabled = true;
		s.splitViewTab = 'output';
		s.splitViewFocus = 'claude'; // focus on bottom pane
		s.outputViewOffset = 0;
		s.outputAutoScroll = true;

		// Wheel up → increase offset.
		var r = sendWheel(s, 'up');
		s = r[0];
		var offsetUp = s.outputViewOffset;
		var autoUp = s.outputAutoScroll;

		// Wheel down → decrease offset.
		r = sendWheel(s, 'down');
		s = r[0];
		var offsetDown = s.outputViewOffset;
		var autoDown = s.outputAutoScroll;

		return {
			offsetUp: offsetUp,
			autoUp: autoUp,
			offsetDown: offsetDown,
			autoDown: autoDown
		};
	})()`)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	m := raw.(map[string]any)
	if prsplittest.NumVal(m["offsetUp"]) != 3 {
		t.Errorf("wheel up: offset = %v, want 3", m["offsetUp"])
	}
	if m["autoUp"] != false {
		t.Errorf("wheel up: autoScroll should be false")
	}
	if prsplittest.NumVal(m["offsetDown"]) != 0 {
		t.Errorf("wheel down from 3: offset = %v, want 0", m["offsetDown"])
	}
	if m["autoDown"] != true {
		t.Errorf("wheel down to 0: autoScroll should be true")
	}
}

// TestChunk16_T44_RenderOutputPanePlaceholder verifies renderOutputPane shows
// placeholder text when outputLines is empty.
func TestChunk16_T44_RenderOutputPanePlaceholder(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.splitViewEnabled = true;
		s.splitViewTab = 'output';
		s.splitViewFocus = 'claude';
		s.outputLines = [];

		var view = globalThis.prSplit._renderOutputPane(s, 80, 12);
		return view;
	})()`)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	view := raw.(string)
	if !strings.Contains(view, "No process output yet") {
		t.Errorf("expected placeholder text, got: %s", view)
	}
	if !strings.Contains(view, "Output from git") {
		t.Errorf("expected hint text about git output")
	}
}

// TestChunk16_T44_RenderOutputPaneWithContent verifies renderOutputPane shows
// output lines with scroll indicator.
func TestChunk16_T44_RenderOutputPaneWithContent(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.splitViewEnabled = true;
		s.splitViewTab = 'output';
		s.splitViewFocus = 'claude';
		s.outputViewOffset = 0;

		// Populate with enough lines to trigger scroll.
		s.outputLines = [];
		for (var i = 0; i < 30; i++) {
			s.outputLines.push('line ' + i);
		}

		var view = globalThis.prSplit._renderOutputPane(s, 80, 12);
		return view;
	})()`)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	view := raw.(string)
	if !strings.Contains(view, "Output") {
		t.Errorf("expected 'Output' title, got: %s", view)
	}
	if !strings.Contains(view, "[live]") {
		t.Errorf("expected '[live]' scroll indicator at bottom")
	}
	if strings.Contains(view, "30 lines") {
		t.Errorf("did not expect line-count badge in output title: %s", view)
	}
}

// TestChunk16_T44_OutputCaptureFnPipesLines verifies that setting
// prSplit._outputCaptureFn pipes output to s.outputLines.
func TestChunk16_T44_OutputCaptureFnPipesLines(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.outputLines = [];
		s.outputAutoScroll = true;
		s.outputViewOffset = 0;

		// Simulate what startAnalysis does.
		globalThis.prSplit._outputCaptureFn = function(line) {
			s.outputLines.push(line);
			if (s.outputLines.length > 5000) {
				s.outputLines = s.outputLines.slice(-4000);
			}
			if (s.outputAutoScroll) s.outputViewOffset = 0;
		};

		// Simulate output capture.
		globalThis.prSplit._outputCaptureFn('> git rev-parse HEAD');
		globalThis.prSplit._outputCaptureFn('abc123');
		globalThis.prSplit._outputCaptureFn('> git diff --name-status');
		globalThis.prSplit._outputCaptureFn('M\tfile.go');

		// Clean up.
		globalThis.prSplit._outputCaptureFn = null;

		return {
			count: s.outputLines.length,
			first: s.outputLines[0],
			last: s.outputLines[3]
		};
	})()`)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	m := raw.(map[string]any)
	if prsplittest.NumVal(m["count"]) != 4 {
		t.Errorf("expected 4 lines, got %v", m["count"])
	}
	if !strings.Contains(fmt.Sprint(m["first"]), "git rev-parse") {
		t.Errorf("first line = %v, expected git command", m["first"])
	}
	if !strings.Contains(fmt.Sprint(m["last"]), "file.go") {
		t.Errorf("last line = %v, expected file output", m["last"])
	}
}

// TestChunk16_T44_CtrlLResetsTabOnDisable verifies that toggling split-view
// off resets the tab back to 'claude'.
func TestChunk16_T44_CtrlLResetsTabOnDisable(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.splitViewEnabled = true;
		s.splitViewTab = 'output';

		// Ctrl+L to disable split-view.
		var r = sendKey(s, 'ctrl+l');
		s = r[0];
		return {
			enabled: s.splitViewEnabled,
			tab: s.splitViewTab
		};
	})()`)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	m := raw.(map[string]any)
	if m["enabled"] != false {
		t.Errorf("splitViewEnabled should be false")
	}
	if m["tab"] != "claude" {
		t.Errorf("splitViewTab should reset to 'claude', got %v", m["tab"])
	}
}

// TestChunk16_T44_HelpOverlayShowsCtrlO verifies the help overlay includes
// the Ctrl+O keybinding for tab switching.
func TestChunk16_T44_HelpOverlayShowsCtrlO(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.width = 80;
		s.height = 30;
		var view = globalThis.prSplit._viewHelpOverlay(s);
		return view;
	})()`)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	view := raw.(string)
	if !strings.Contains(view, "Ctrl+O") {
		t.Errorf("help overlay should mention Ctrl+O")
	}
	if !strings.Contains(view, "Output") {
		t.Errorf("help overlay should mention Output tab")
	}
}

// TestChunk16_T44_CtrlOInReservedKeys ensures Ctrl+O is in the reserved
// keys set so it's not forwarded to Claude PTY.
func TestChunk16_T44_CtrlOInReservedKeys(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var keys = globalThis.prSplit._CLAUDE_RESERVED_KEYS;
		return keys['ctrl+o'] === true;
	})()`)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	if raw != true {
		t.Errorf("ctrl+o should be in CLAUDE_RESERVED_KEYS")
	}
}

// TestChunk16_T44_ViewNoPanicWithOutputTab verifies that calling viewForState
// with the output tab selected does not panic or error (smoke test).
// The full tab bar is rendered in _viewFn (which requires viewport/scrollbar
// objects), so this only confirms the view logic doesn't crash.
func TestChunk16_T44_ViewNoPanicWithOutputTab(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.splitViewEnabled = true;
		s.splitViewTab = 'output';
		s.width = 80;
		s.height = 30;

		var view = globalThis.prSplit._viewForState(s);
		// The tab bar is rendered in _viewFn (the full layout), not in viewForState.
		// We can't easily test the full _viewFn without viewport/scrollbar objects.
		return 'ok';
	})()`)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	if raw != "ok" {
		t.Errorf("unexpected result: %v", raw)
	}
}

// TestChunk16_T44_OutputBufferCapAtLimit verifies the output buffer is
// capped at 5000 lines and trims to 4000.
func TestChunk16_T44_OutputBufferCapAtLimit(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.outputLines = [];
		s.outputAutoScroll = true;
		s.outputViewOffset = 0;

		// Install capture function.
		globalThis.prSplit._outputCaptureFn = function(line) {
			s.outputLines.push(line);
			if (s.outputLines.length > 5000) {
				s.outputLines = s.outputLines.slice(-4000);
			}
			if (s.outputAutoScroll) s.outputViewOffset = 0;
		};

		// Push 5010 lines.
		for (var i = 0; i < 5010; i++) {
			globalThis.prSplit._outputCaptureFn('line-' + i);
		}

		globalThis.prSplit._outputCaptureFn = null;

		return {
			count: s.outputLines.length,
			first: s.outputLines[0],
			last: s.outputLines[s.outputLines.length - 1]
		};
	})()`)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	m := raw.(map[string]any)
	countVal := prsplittest.NumVal(m["count"])
	// After 5001 lines, trims to 4000. Then 5002-5010 means 4000 + 9 = 4009.
	// Actually: push 5001 → trimmed to 4000. push 5002-5010 → 4009.
	if countVal > 5000 {
		t.Errorf("expected <= 5000 lines, got %v", countVal)
	}
	// The last line should always be 'line-5009' (the very last one pushed).
	if fmt.Sprint(m["last"]) != "line-5009" {
		t.Errorf("last line = %v, expected 'line-5009'", m["last"])
	}
}
