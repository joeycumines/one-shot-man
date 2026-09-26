package command

import (
	"encoding/json"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

func TestPrSplitManualBranchBuildingPropagatesVerifyTimeout(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var originalExecute = prSplit.executeSplit;
		var originalVerify = prSplit.verifySplit;
		var captured = 0;
		prSplit.executeSplit = async function() {
			return { results: [{ name: 'split/api', files: [], sha: 'abc' }] };
		};
		prSplit.verifySplit = async function(branch, options) {
			captured = options.verifyTimeoutMs;
			return { passed: true, output: '', error: null };
		};
		try {
			prSplitConfig.timeoutMs = 1234;
			var s = initState('BRANCH_BUILDING');
			await prSplit._handleBranchBuildingState(s.wizard, {
				splits: [{ name: 'split/api', files: [] }],
				verifyCommand: 'make test',
				dir: '.'
			}, {});
			return captured;
		} finally {
			prSplit.executeSplit = originalExecute;
			prSplit.verifySplit = originalVerify;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != int64(1234) {
		t.Errorf("manual branch-building verify timeout = %v, want 1234", raw)
	}
}

func TestPrSplitTUIConfigPropagatesVerifyTimeout(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		prSplitConfig = prSplitConfig || {};
		prSplitConfig.timeoutMs = 4321;
		prSplit.runtime = prSplit.runtime || {};
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.dir = '.';
		prSplit.runtime.strategy = 'directory';
		prSplit.runtime.verifyCommand = 'true';
		prSplit.runtime.mode = 'heuristic';

		var seen = [];
		var oldHandleConfigState = prSplit._handleConfigState;
		prSplit._handleConfigState = function(config) {
			seen.push(config.verifyTimeoutMs);
			return { error: 'stop-after-capture' };
		};
		try {
			var heuristic = initState('CONFIG');
			await prSplit._startAnalysis(heuristic);

			prSplit.runtime.mode = 'auto';
			var automatic = initState('CONFIG');
			await prSplit._startAutoAnalysis(automatic);

			if (seen.length !== 2 || seen[0] !== 4321 || seen[1] !== 4321) {
				return 'FAIL: timeout propagation = ' + JSON.stringify(seen);
			}
			return 'OK';
		} finally {
			prSplit._handleConfigState = oldHandleConfigState;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("TUI verify timeout propagation: %v", raw)
	}
}

func TestPrSplitDefaultVerifyTimeoutIsApplied(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		return JSON.stringify({
			zero: prSplit._effectiveVerifyTimeoutMs(0),
			explicit: prSplit._effectiveVerifyTimeoutMs(1234)
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Zero     int64 `json:"zero"`
		Explicit int64 `json:"explicit"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Zero != 600000 {
		t.Errorf("default verify timeout = %d, want 600000", parsed.Zero)
	}
	if parsed.Explicit != 1234 {
		t.Errorf("explicit verify timeout = %d, want 1234", parsed.Explicit)
	}
}

func TestPrSplitTUIUsesDefaultVerifyTimeout(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var originalCanSpawn = prSplit.canSpawnInteractiveShell;
		var originalStart = prSplit.startVerifySession;
		var capturedTimeout = 0;
		try {
			setupPlanCache();
			prSplitConfig.timeoutMs = 0;
			prSplit.runtime.verifyCommand = 'make test';
			prSplit.canSpawnInteractiveShell = function() { return false; };
			prSplit.startVerifySession = function(branch, options) {
				capturedTimeout = options.timeoutMs;
				return { skipped: true, session: null, worktreeDir: null };
			};
			var s = initState('EXECUTING');
			s.isProcessing = true;
			s.verificationResults = [];
			s.verifyingIdx = 0;
			s.verifyOutput = {};
			update({type: 'Tick', id: 'verify-branch'}, s);
			return capturedTimeout;
		} finally {
			prSplit.canSpawnInteractiveShell = originalCanSpawn;
			prSplit.startVerifySession = originalStart;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != int64(600000) {
		t.Errorf("TUI default verify timeout = %v, want 600000", raw)
	}
}

func TestPrSplitOneShotVerifyDeadlineFailsSession(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var killed = false;
		var s = initState('BRANCH_BUILDING');
		s.verifyMode = 'oneshot';
		s.activeVerifyBranch = 'split/timeout';
		s.activeVerifyStartTime = Date.now() - 100;
		s.verifyDeadline = Date.now() - 1;
		s.verifyElapsedMs = 100;
		s.verifyingIdx = 0;
		s.verificationResults = [];
		s.verifyOutput = {};
		s.outputLines = [];
		s.activeVerifySession = {
			isDone: function() { return false; },
			screen: function() { return ''; },
			output: function() { return 'partial'; },
			kill: function() { killed = true; }
		};
		var r = update({type: 'Tick', id: 'verify-poll'}, s);
		if (!killed || r[0].verifyingIdx !== 1) {
			return 'FAIL: deadline did not terminate one-shot session';
		}
		if (!r[0].verificationResults[0] || r[0].verificationResults[0].passed) {
			return 'FAIL: deadline did not record a failed verification';
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("one-shot Verify deadline: %v", raw)
	}
}
