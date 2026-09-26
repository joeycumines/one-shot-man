package command

import (
	"encoding/json"
	"runtime"
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

func TestVerifyRunBranch_InteractivePathSkipsOneShotBootstrap(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		globalThis.prSplit.runtime.dir = '.';
		globalThis.prSplit.runtime.verifyCommand = 'make test';

		var origCanSpawn = globalThis.prSplit.canSpawnInteractiveShell;
		var origPrepare = globalThis.prSplit.prepareVerifyWorktree;
		var origSpawn = globalThis.prSplit.spawnShellSession;
		var origStart = globalThis.prSplit.startVerifySession;
		var startCalled = 0;

		globalThis.prSplit.canSpawnInteractiveShell = function() { return true; };
		globalThis.prSplit.prepareVerifyWorktree = function() {
			return { worktreeDir: '/tmp/osm-verify-test', dir: '.' };
		};
		globalThis.prSplit.spawnShellSession = function() {
			return {
				screen: function() { return ''; },
				output: function() { return ''; },
				isDone: function() { return false; },
				interrupt: function() {},
				kill: function() {},
				pause: function() {},
				resume: function() {}
			};
		};
		globalThis.prSplit.startVerifySession = function() {
			startCalled++;
			return { error: 'should not be called', session: null };
		};

		try {
			var s = initState('EXECUTING');
			s.isProcessing = true;
			s.verificationResults = [];
			s.verifyingIdx = 0;
			s.verifyOutput = {};
			s.width = 100;
			s.height = 30;

			var result = update({type: 'Tick', id: 'verify-branch'}, s);
			s = result[0];

			return JSON.stringify({
				startCalled: startCalled,
				verifyMode: s.verifyMode,
				verifyHint: s.verifyHint,
				activeVerifyBranch: s.activeVerifyBranch,
				hasTick: !!(result && result[1])
			});
		} finally {
			globalThis.prSplit.canSpawnInteractiveShell = origCanSpawn;
			globalThis.prSplit.prepareVerifyWorktree = origPrepare;
			globalThis.prSplit.spawnShellSession = origSpawn;
			globalThis.prSplit.startVerifySession = origStart;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		StartCalled        int    `json:"startCalled"`
		VerifyMode         string `json:"verifyMode"`
		VerifyHint         string `json:"verifyHint"`
		ActiveVerifyBranch string `json:"activeVerifyBranch"`
		HasTick            bool   `json:"hasTick"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed.StartCalled != 0 {
		t.Fatalf("expected interactive path to skip startVerifySession, got %d calls", parsed.StartCalled)
	}
	if parsed.VerifyMode != "interactive" {
		t.Fatalf("expected verifyMode=interactive, got %q", parsed.VerifyMode)
	}
	if parsed.VerifyHint != "make test" {
		t.Fatalf("expected interactive verify hint to be set, got %q", parsed.VerifyHint)
	}
	if parsed.ActiveVerifyBranch == "" {
		t.Fatal("expected active verify branch to be set")
	}
	if !parsed.HasTick {
		t.Fatal("expected interactive path to schedule a poll tick")
	}
}

func TestVerifyRunBranch_AwaitsAsyncWorktreePreparation(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		setupPlanCache();
		globalThis.prSplit.runtime.dir = '.';
		globalThis.prSplit.runtime.verifyCommand = 'make test';

		var originalCanSpawn = globalThis.prSplit.canSpawnInteractiveShell;
		var originalPrepare = globalThis.prSplit.prepareVerifyWorktree;
		var originalSpawn = globalThis.prSplit.spawnShellSession;
		var resolvePrepare;
		var prepareCalls = 0;

		globalThis.prSplit.canSpawnInteractiveShell = function() { return true; };
		globalThis.prSplit.prepareVerifyWorktree = function() {
			prepareCalls++;
			return new Promise(function(resolve) { resolvePrepare = resolve; });
		};
		globalThis.prSplit.spawnShellSession = function(dir) {
			if (dir !== '/tmp/osm-verify-async') throw new Error('wrong worktree: ' + dir);
			return {
				screen: function() { return ''; },
				output: function() { return ''; },
				isDone: function() { return false; },
				interrupt: function() {},
				kill: function() {},
				pause: function() {},
				resume: function() {}
			};
		};

		try {
			var s = initState('EXECUTING');
			s.isProcessing = true;
			s.verificationResults = [];
			s.verifyingIdx = 0;
			s.verifyOutput = {};
			s.width = 100;
			s.height = 30;

			var first = update({type: 'Tick', id: 'verify-branch'}, s);
			if (prepareCalls !== 1 || !first[0]._verifySetup || !first[0]._verifySetup.pending) {
				return 'FAIL: async worktree preparation was not deferred';
			}
			resolvePrepare({ worktreeDir: '/tmp/osm-verify-async', dir: '.' });
			await Promise.resolve();
			await Promise.resolve();
			var second = update({type: 'Tick', id: 'verify-branch'}, first[0]);
			if (second[0].verifyMode !== 'interactive' || second[0].activeVerifyBranch === '') {
				return 'FAIL: resolved worktree did not start interactive verification';
			}
			return 'OK';
		} finally {
			globalThis.prSplit.canSpawnInteractiveShell = originalCanSpawn;
			globalThis.prSplit.prepareVerifyWorktree = originalPrepare;
			globalThis.prSplit.spawnShellSession = originalSpawn;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("async Verify worktree preparation: %v", raw)
	}
}

func TestVerifyRunBranch_InteractiveRegisterFailureUsesRawCleanup(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		globalThis.prSplit.runtime.dir = '.';
		globalThis.prSplit.runtime.verifyCommand = 'make test';

		var origCanSpawn = globalThis.prSplit.canSpawnInteractiveShell;
		var origPrepare = globalThis.prSplit.prepareVerifyWorktree;
		var origSpawn = globalThis.prSplit.spawnShellSession;
		var origCleanup = globalThis.prSplit.cleanupVerifyWorktree;
		var origTuiMux = globalThis.tuiMux;
		var origSync = globalThis.prSplit._syncMainViewport;
		var closed = 0;
		var cleanupArgs = [];

		globalThis.prSplit.canSpawnInteractiveShell = function() { return true; };
		globalThis.prSplit.prepareVerifyWorktree = function() {
			return { worktreeDir: '/tmp/osm-verify-raw', dir: '/tmp/repo' };
		};
		globalThis.prSplit.spawnShellSession = function() {
			return {
				screen: function() { return ''; },
				output: function() { return ''; },
				isDone: function() { return false; },
				close: function() { closed++; },
				interrupt: function() {},
				kill: function() {},
				pause: function() {},
				resume: function() {}
			};
		};
		globalThis.prSplit.cleanupVerifyWorktree = function(dir, worktree) {
			cleanupArgs.push([dir, worktree]);
		};
		globalThis.prSplit._syncMainViewport = function() {};
		globalThis.tuiMux = globalThis.tuiMux || {};
		globalThis.tuiMux.register = function() {
			throw new Error('register failed');
		};

		try {
			var s = initState('EXECUTING');
			s.isProcessing = true;
			s.verificationResults = [];
			s.verifyingIdx = 0;
			s.verifyOutput = {};
			s.width = 100;
			s.height = 30;

			var result = update({type: 'Tick', id: 'verify-branch'}, s);
			s = result[0];
			if (s.verifyMode !== 'interactive') return 'FAIL: expected interactive mode';
			if (!s.activeVerifySession || typeof s.activeVerifySession.close !== 'function') {
				return 'FAIL: expected raw session fallback when register throws';
			}

			globalThis.prSplit._clearVerifyPaneSession(s, { debugPrefix: 'test', keepDisplay: false });

			return JSON.stringify({
				closed: closed,
				cleanupCalls: cleanupArgs.length,
				cleared: s.activeVerifySession === null && s.activeVerifyWorktree === null
			});
		} finally {
			globalThis.prSplit.canSpawnInteractiveShell = origCanSpawn;
			globalThis.prSplit.prepareVerifyWorktree = origPrepare;
			globalThis.prSplit.spawnShellSession = origSpawn;
			globalThis.prSplit.cleanupVerifyWorktree = origCleanup;
			globalThis.prSplit._syncMainViewport = origSync;
			globalThis.tuiMux = origTuiMux;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Closed       int  `json:"closed"`
		CleanupCalls int  `json:"cleanupCalls"`
		Cleared      bool `json:"cleared"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed.Closed != 1 {
		t.Fatalf("expected raw interactive session fallback to close exactly once, got %d", parsed.Closed)
	}
	if parsed.CleanupCalls != 1 {
		t.Fatalf("expected raw interactive session fallback to clean up worktree once, got %d", parsed.CleanupCalls)
	}
	if !parsed.Cleared {
		t.Fatal("expected raw interactive session fallback state to be cleared")
	}
}

func TestVerifyRunBranch_ShellSpawnFailureFallsBackToOneShot(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		globalThis.prSplit.runtime.dir = '.';
		globalThis.prSplit.runtime.verifyCommand = 'make test';

		var origCanSpawn = globalThis.prSplit.canSpawnInteractiveShell;
		var origPrepare = globalThis.prSplit.prepareVerifyWorktree;
		var origSpawn = globalThis.prSplit.spawnShellSession;
		var origStart = globalThis.prSplit.startVerifySession;
		var origGitExec = globalThis.prSplit._gitExec;
		var gitCalls = [];
		var startCalled = 0;

		globalThis.prSplit.canSpawnInteractiveShell = function() { return true; };
		globalThis.prSplit.prepareVerifyWorktree = function() {
			return { worktreeDir: '/tmp/osm-verify-interactive', dir: '/tmp/repo' };
		};
		globalThis.prSplit.spawnShellSession = function() {
			throw new Error('pty spawn failed');
		};
		globalThis.prSplit.startVerifySession = function() {
			startCalled++;
			return {
				session: {
					screen: function() { return ''; },
					output: function() { return ''; },
					isDone: function() { return false; },
					close: function() {},
					interrupt: function() {},
					kill: function() {},
					pause: function() {},
					resume: function() {}
				},
				worktreeDir: '/tmp/osm-verify-oneshot',
				dir: '/tmp/repo',
				startTime: 123
			};
		};
		globalThis.prSplit._gitExec = function(dir, args) {
			gitCalls.push([dir, args.join(' ')]);
			return { code: 0, stdout: '', stderr: '' };
		};

		try {
			var s = initState('EXECUTING');
			s.isProcessing = true;
			s.verificationResults = [];
			s.verifyingIdx = 0;
			s.verifyOutput = {};
			s.width = 100;
			s.height = 30;

			var result = update({type: 'Tick', id: 'verify-branch'}, s);
			s = result[0];

			return JSON.stringify({
				startCalled: startCalled,
				verifyMode: s.verifyMode,
				worktree: s.activeVerifyWorktree,
				cleanupAttempted: gitCalls.some(function(call) {
					return call[1] === 'worktree remove --force /tmp/osm-verify-interactive';
				}),
				hasTick: !!(result && result[1])
			});
		} finally {
			globalThis.prSplit.canSpawnInteractiveShell = origCanSpawn;
			globalThis.prSplit.prepareVerifyWorktree = origPrepare;
			globalThis.prSplit.spawnShellSession = origSpawn;
			globalThis.prSplit.startVerifySession = origStart;
			globalThis.prSplit._gitExec = origGitExec;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		StartCalled      int    `json:"startCalled"`
		VerifyMode       string `json:"verifyMode"`
		Worktree         string `json:"worktree"`
		CleanupAttempted bool   `json:"cleanupAttempted"`
		HasTick          bool   `json:"hasTick"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed.StartCalled != 1 {
		t.Fatalf("expected one-shot fallback to start after shell spawn failure, got %d calls", parsed.StartCalled)
	}
	if parsed.VerifyMode != "oneshot" {
		t.Fatalf("expected verifyMode=oneshot after shell spawn failure, got %q", parsed.VerifyMode)
	}
	if parsed.Worktree != "/tmp/osm-verify-oneshot" {
		t.Fatalf("expected one-shot fallback worktree to replace the failed interactive one, got %q", parsed.Worktree)
	}
	if !parsed.CleanupAttempted {
		t.Fatal("expected failed interactive worktree to be cleaned before falling back to one-shot")
	}
	if !parsed.HasTick {
		t.Fatal("expected one-shot fallback to schedule a poll tick")
	}
}

func TestVerifyBaselineLateResultCannotCrossRun(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		setupPlanCache();
		prSplit.runtime.dir = '.';
		prSplit.runtime.verifyCommand = 'make test';
		var originalVerify = prSplit.verifySplitAsync;
		var originalCanSpawn = prSplit.canSpawnInteractiveShell;
		var originalStart = prSplit.startVerifySession;
		var resolveBaseline;
		var verifyCalls = 0;
		prSplit.canSpawnInteractiveShell = function() { return false; };
		prSplit.verifySplitAsync = function() {
			verifyCalls++;
			return new Promise(function(resolve) { resolveBaseline = resolve; });
		};
		prSplit.startVerifySession = function() { return { skipped: true }; };
		try {
			var s = initState('EXECUTING');
			s.isProcessing = true;
			s.verificationResults = [];
			s.verifyingIdx = 0;
			s.verifyOutput = {};
			update({type: 'Tick', id: 'verify-branch'}, s);
			if (verifyCalls !== 1 || !resolveBaseline) return 'FAIL: baseline was not started';
			prSplit._resetVerifyRunState(s);
			resolveBaseline({ passed: false });
			await Promise.resolve();
			await Promise.resolve();
			if (s._baselineVerifyResult !== null) {
				return 'FAIL: stale baseline result crossed run boundary';
			}
			return 'OK';
		} finally {
			prSplit.verifySplitAsync = originalVerify;
			prSplit.canSpawnInteractiveShell = originalCanSpawn;
			prSplit.startVerifySession = originalStart;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("baseline run isolation: %v", raw)
	}
}

func TestVerifyRunBranch_InvalidShellStartFallsBackToOneShot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell startup test")
	}
	t.Setenv("SHELL", "/definitely/missing/one-shot-man-shell")
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		setupPlanCache();
		prSplit.runtime.dir = '.';
		prSplit.runtime.verifyCommand = 'make test';
		var originalCanSpawn = prSplit.canSpawnInteractiveShell;
		var originalPrepare = prSplit.prepareVerifyWorktree;
		var originalStart = prSplit.startVerifySession;
		var originalGitExec = prSplit._gitExec;
		var cleanupCalls = [];
		prSplit.canSpawnInteractiveShell = function() { return true; };
		prSplit.prepareVerifyWorktree = function() {
			return { worktreeDir: '/tmp/invalid-shell-worktree', dir: '/tmp/repo' };
		};
		prSplit.startVerifySession = function() { return { skipped: true }; };
		prSplit._gitExec = function(dir, args) {
			cleanupCalls.push([dir, args.join(' ')]);
			return { code: 0, stdout: '', stderr: '' };
		};
		try {
			var s = initState('EXECUTING');
			s.isProcessing = true;
			s.verificationResults = [];
			s.verifyingIdx = 0;
			s.verifyOutput = {};
			var first = update({type: 'Tick', id: 'verify-branch'}, s);
			if (!first[0]._verifySetup || !first[0]._verifySetup.pending) {
				return 'FAIL: invalid shell startup was not deferred';
			}
			var cleanupDeadline = Date.now() + 2000;
			while (cleanupCalls.length === 0 && Date.now() < cleanupDeadline) {
				await new Promise(function(resolve) { setTimeout(resolve, 5); });
			}
			var second = update({type: 'Tick', id: 'verify-branch'}, first[0]);
			if (second[0].verifyMode === 'interactive' || second[0].activeVerifySession) {
				return 'FAIL: invalid shell was accepted as an interactive session';
			}
			if (cleanupCalls.length === 0 || cleanupCalls[0][1].indexOf('/tmp/invalid-shell-worktree') < 0) {
				return 'FAIL: invalid shell worktree was not cleaned: ' + JSON.stringify(cleanupCalls);
			}
			return 'OK';
		} finally {
			prSplit.canSpawnInteractiveShell = originalCanSpawn;
			prSplit.prepareVerifyWorktree = originalPrepare;
			prSplit.startVerifySession = originalStart;
			prSplit._gitExec = originalGitExec;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("invalid shell startup fallback: %v", raw)
	}
}

func TestVerifyRunBranch_AsyncShellStartFailureFallsBackToOneShot(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		setupPlanCache();
		prSplit.runtime.dir = '.';
		prSplit.runtime.verifyCommand = 'make test';
		var originalCanSpawn = prSplit.canSpawnInteractiveShell;
		var originalPrepare = prSplit.prepareVerifyWorktree;
		var originalSpawn = prSplit.spawnShellSession;
		var originalStart = prSplit.startVerifySession;
		var originalGitExec = prSplit._gitExec;
		var startCalled = 0;
		var cleanupCalls = [];

		prSplit.canSpawnInteractiveShell = function() { return true; };
		prSplit.prepareVerifyWorktree = function() {
			return { worktreeDir: '/tmp/async-start-worktree', dir: '/tmp/repo' };
		};
		prSplit.spawnShellSession = function() {
			return {
				start: function() { return Promise.reject(new Error('pty start failed')); }
			};
		};
		prSplit.startVerifySession = function() {
			startCalled++;
			return { skipped: true, session: null, worktreeDir: null };
		};
		prSplit._gitExec = function(dir, args) {
			cleanupCalls.push([dir, args.join(' ')]);
			return { code: 0, stdout: '', stderr: '' };
		};

		try {
			var s = initState('EXECUTING');
			s.isProcessing = true;
			s.verificationResults = [];
			s.verifyingIdx = 0;
			s.verifyOutput = {};
			s.width = 100;
			s.height = 30;
			var first = update({type: 'Tick', id: 'verify-branch'}, s);
			if (!first[0]._verifySetup || !first[0]._verifySetup.pending) {
				return 'FAIL: shell startup was not deferred';
			}
			await Promise.resolve();
			await Promise.resolve();
			var second = update({type: 'Tick', id: 'verify-branch'}, first[0]);
			if (startCalled !== 1 || second[0].verifyMode === 'interactive' || second[0].activeVerifySession) {
				return 'FAIL: async startup failure did not fall back cleanly: ' + JSON.stringify({
					startCalled: startCalled,
					mode: second[0].verifyMode,
					active: second[0].activeVerifySession
				});
			}
			if (cleanupCalls.length === 0 || cleanupCalls[0][1].indexOf('/tmp/async-start-worktree') < 0) {
				return 'FAIL: failed startup worktree was not cleaned: ' + JSON.stringify(cleanupCalls);
			}
			return 'OK';
		} finally {
			prSplit.canSpawnInteractiveShell = originalCanSpawn;
			prSplit.prepareVerifyWorktree = originalPrepare;
			prSplit.spawnShellSession = originalSpawn;
			prSplit.startVerifySession = originalStart;
			prSplit._gitExec = originalGitExec;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("async shell startup failure: %v", raw)
	}
}

func TestVerifyPoll_InteractiveShellExitWaitsForSignal(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		var s = {
			wizardState: 'BRANCH_BUILDING',
			isProcessing: true,
			verifyMode: 'interactive',
			verifyAutoScroll: true,
			verifyScreen: '',
			verifyViewportOffset: 0,
			spinnerFrame: 0,
			activeVerifySession: {
				screen: function() { return 'shell exited'; },
				output: function() { return 'shell exited'; },
				isDone: function() { return true; },
				exitCode: function() { return 0; }
			},
			activeVerifyBranch: 'split/test',
			activeVerifyStartTime: Date.now() - 1000,
			verifyElapsedMs: 0,
			verificationResults: [],
			verifyOutput: {},
			outputLines: [],
			outputAutoScroll: true
		};

		var result = globalThis.prSplit._pollVerifySession(s);
		return JSON.stringify({
			verifyShellExited: s.verifyShellExited,
			verifyResults: s.verificationResults.length,
			hasTick: !!(result && result[1]),
			verifyScreen: s.verifyScreen
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		VerifyShellExited bool   `json:"verifyShellExited"`
		VerifyResults     int    `json:"verifyResults"`
		HasTick           bool   `json:"hasTick"`
		VerifyScreen      string `json:"verifyScreen"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &parsed); err != nil {
		t.Fatal(err)
	}

	if !parsed.VerifyShellExited {
		t.Fatal("expected interactive shell exit to be tracked without auto-completing the branch")
	}
	if parsed.VerifyResults != 0 {
		t.Fatalf("expected interactive shell exit to wait for user signal, got %d results", parsed.VerifyResults)
	}
	if !parsed.HasTick {
		t.Fatal("expected interactive shell exit to keep polling")
	}
	if parsed.VerifyScreen == "" {
		t.Fatal("expected final shell screen to remain visible after shell exit")
	}
}

func TestVerifyPoll_OneShotTimeoutWaitsForKillBeforeAdvancing(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	val, err := evalJS(`(async function() {
		var resolveKill;
		var closeCalls = 0;
		var session = {
			screen: function() { return 'timeout'; },
			output: function() { return 'timeout output'; },
			isDone: function() { return false; },
			kill: function() {
				return new Promise(function(resolve) { resolveKill = resolve; });
			},
			close: function() { closeCalls++; }
		};
		var s = {
			wizardState: 'BRANCH_BUILDING',
			wizard: { current: 'BRANCH_BUILDING' },
			isProcessing: true,
			verifyMode: 'oneshot',
			verifyDeadline: Date.now() - 1,
			verifyElapsedMs: 25,
			activeVerifyStartTime: Date.now() - 100,
			activeVerifySession: session,
			activeVerifyBranch: 'split/test',
			verifyingIdx: 0,
			verificationResults: [],
			verifyOutput: {},
			outputLines: [],
			outputAutoScroll: true,
			verifyAutoScroll: false,
			verifyScreen: '',
			verifyViewportOffset: 0,
			paneOperations: [],
			_verifyRunEpoch: 0,
			_verifySetup: null
		};
		var result = prSplit._pollVerifySession(s);
		if (!result || result.length !== 2 || !result[1]) return 'FAIL: timeout did not schedule a tick';
		if (s.verificationResults.length !== 0 || s.verifyingIdx !== 0) {
			return 'FAIL: branch advanced before kill settled';
		}
		if (s.paneOperations.length !== 1) return 'FAIL: timeout kill was not tracked';
		resolveKill();
		await settlePaneOperations(s);
		if (s.verificationResults.length !== 1 || s.verifyingIdx !== 0) {
			return 'FAIL: timeout result did not wait for cleanup before advancement';
		}
		if (s.activeVerifySession !== null || closeCalls !== 1) {
			return 'FAIL: timeout session cleanup ordering failed';
		}
		var next = prSplit._pollVerifySession(s);
		if (s.verifyingIdx !== 1 || !next[1]) return 'FAIL: branch did not advance after settlement';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "OK" {
		t.Errorf("verify timeout settlement ordering: %v", val)
	}
}

func TestVerifyPoll_OneShotExitRecordsResult(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		var s = {
			wizardState: 'BRANCH_BUILDING',
			isProcessing: true,
			verifyMode: 'oneshot',
			verifyAutoScroll: true,
			verifyScreen: '',
			verifyViewportOffset: 0,
			spinnerFrame: 0,
			activeVerifySession: {
				screen: function() { return 'go test'; },
				output: function() { return 'go test\nFAIL'; },
				isDone: function() { return true; },
				exitCode: function() { return 1; }
			},
			activeVerifyBranch: 'split/test',
			activeVerifyStartTime: Date.now() - 1000,
			verifyElapsedMs: 0,
			verificationResults: [],
			verifyOutput: {},
			outputLines: [],
			outputAutoScroll: true
		};

		var result = globalThis.prSplit._pollVerifySession(s);
		return JSON.stringify({
			verifyShellExited: !!s.verifyShellExited,
			verifyResults: s.verificationResults.length,
			passed: s.verificationResults.length ? s.verificationResults[0].passed : null,
			error: s.verificationResults.length ? s.verificationResults[0].error : '',
			hasTick: !!(result && result[1]),
			outputSaved: s.verifyOutput['split/test'] ? s.verifyOutput['split/test'].length : 0
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		VerifyShellExited bool   `json:"verifyShellExited"`
		VerifyResults     int    `json:"verifyResults"`
		Passed            *bool  `json:"passed"`
		Error             string `json:"error"`
		HasTick           bool   `json:"hasTick"`
		OutputSaved       int    `json:"outputSaved"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed.VerifyShellExited {
		t.Fatal("expected one-shot mode to record exit instead of waiting for a manual signal")
	}
	if parsed.VerifyResults != 1 {
		t.Fatalf("expected one-shot exit to record one result, got %d", parsed.VerifyResults)
	}
	if parsed.Passed == nil || *parsed.Passed {
		t.Fatal("expected one-shot non-zero exit to record a failed result")
	}
	if parsed.Error == "" {
		t.Fatal("expected one-shot non-zero exit to record an error message")
	}
	if !parsed.HasTick {
		t.Fatal("expected one-shot completion to schedule the next branch tick")
	}
	if parsed.OutputSaved == 0 {
		t.Fatal("expected one-shot completion to preserve captured output")
	}
}
