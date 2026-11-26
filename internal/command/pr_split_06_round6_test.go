package command

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// These scenarios distinguish an unknown verification result from an
// unconfirmed child termination. The latter also owns a retained worktree and
// must stop the workflow; ordinary execution failure still permits subsequent
// verification without forgiving its result as pre-existing.
var baselineExecFailureScenarios = []string{
	"spawn_rejection",
	"initial_wait_rejection_confirmed_cleanup",
	"persistent_wait_rejection",
}

// baselineExecFailureMockJS replaces only native exec and git boundaries. Both
// the batch and TUI tests retain the real verifySplitAsync implementation. The
// command's worktree identifies its branch, so baseline classification does not
// depend on the order in which concurrently launched children are scheduled.
const baselineExecFailureMockJS = `
function installBaselineExecFailureMock(sourceBranch, scenario) {
	var execMod = require('osm:exec');
	var originalSpawn = execMod.spawn;
	var originalGitAsync = prSplit._gitExecAsync;
	var fixture = {spawned:[], worktrees:Object.create(null), removed:[], baselineWaits:0, baselineKills:0};
	function gitOK() { return {stdout:'', stderr:'', code:0, error:false, message:''}; }
	prSplit._gitExecAsync = function(dir, args) {
		if (args[0] === 'worktree' && args[1] === 'add') {
			fixture.worktrees[args[args.length - 1]] = args[args.length - 2];
			return gitOK();
		}
		if (args[0] === 'worktree' && args[1] === 'remove') {
			fixture.removed.push(args[args.length - 1]);
			return gitOK();
		}
		return originalGitAsync.apply(this, arguments);
	};
	execMod.spawn = function(cmd, args, opts) {
		if (cmd === 'git') return originalSpawn.apply(this, arguments);
		var shellText = (args || []).join(' ');
		var branch = null;
		for (var name of Object.keys(fixture.worktrees)) {
			if (shellText.indexOf(prSplit._shellQuote(fixture.worktrees[name])) >= 0) branch = name;
		}
		if (branch === null) throw new Error('unexpected verify command: ' + shellText);
		fixture.spawned.push(branch);
		var baseline = branch === sourceBranch;
		if (baseline && scenario === 'spawn_rejection') {
			return Promise.reject(new Error('spawn failed: resource temporarily unavailable'));
		}
		function doneStream() {
			return {read:function() { return Promise.resolve({done:true}); }};
		}
		return {
			stdout:doneStream(), stderr:doneStream(),
			wait:function() {
				if (!baseline) return Promise.resolve({code:1, signal:null});
				fixture.baselineWaits++;
				if (fixture.baselineWaits === 1 || scenario === 'persistent_wait_rejection') {
					return Promise.reject(new Error('wait failed: no exit status'));
				}
				return Promise.resolve({code:0, signal:null});
			},
			kill:function() {
				if (baseline) fixture.baselineKills++;
				return Promise.resolve();
			},
			isAlive:function() { return false; },
			close:function() {}
		};
	};
	fixture.restore = function() {
		execMod.spawn = originalSpawn;
		prSplit._gitExecAsync = originalGitAsync;
	};
	return fixture;
}
`

// TestChunk16_BaselineExecFailureIsNotBaselineFailure exercises the complete
// verifier-to-TUI boundary. An unknown baseline with successful cleanup must
// not forgive split/1's exit 1 or unblock dependent split/2. If both exit waits
// reject, the same unknown verdict additionally retains its worktree and must
// enter ERROR before any dependent branch starts.
func TestChunk16_BaselineExecFailureIsNotBaselineFailure(t *testing.T) {
	t.Parallel()
	for _, scenario := range baselineExecFailureScenarios {
		t.Run(scenario, func(t *testing.T) {
			evalJS := prsplittest.NewTUIEngineWithHelpers(t)
			scenarioJSON, err := json.Marshal(scenario)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := evalJS(`(async function() {
				var scenario = ` + string(scenarioJSON) + `;
				` + baselineExecFailureMockJS + `
				setupPlanCache();
				prSplit._state.planCache.sourceBranch = 'feature';
				prSplit._state.planCache.splits = [
					{name:'split/1', files:['a.go']},
					{name:'split/2', files:['b.go'], dependencies:['split/1']}
				];
				prSplit.runtime.verifyCommand = 'make test';
				prSplit.runtime.dir = '/tmp/repo';
				var fixture = installBaselineExecFailureMock('feature', scenario);
				try {
					var s = initState('BRANCH_BUILDING');
					s.isProcessing = true;
					s.verifyingIdx = 0;
					s.verifyOutput = {};
					s.verificationResults = [];
					s.outputLines = [];
					s.verifyFallbackRunning = false;
					s._verifyRunEpoch = 1;
					prSplit.canSpawnInteractiveShell = function() { return false; };
					prSplit.startVerifySession = function() { return {error:'no capture session', session:null}; };

					// Preserve the existing bounded event-loop driver. Await each
					// macrotask boundary so the real baseline and fallback promises
					// settle before driving the next verification transition.
					for (var i = 0; i < 50; i++) {
						s = update({type:'Tick', id:'verify-branch'}, s)[0];
						for (var j = 0; j < 5; j++) await Promise.resolve();
						await new Promise(function(resolve) { setTimeout(resolve, 0); });
						s = update({type:'Tick', id:'verify-fallback-poll'}, s)[0];
						for (var k = 0; k < 5; k++) await Promise.resolve();
						await new Promise(function(resolve) { setTimeout(resolve, 0); });
						if (s._baselineVerifyResult && !s.verifyFallbackRunning && (s.verifyingIdx >= 2 || s.wizard.current === 'ERROR')) break;
					}
					var baseline = s._baselineVerifyResult;
					var debug = JSON.stringify({baseline:baseline, rows:s.verificationResults, fixture:fixture, wizard:s.wizard.current});
					if (s._baselineVerifyStarted !== true || !baseline) return 'FAIL: baseline never settled: ' + debug;
					if (baseline.failed !== false || baseline.passed || !baseline.execFailed || baseline.exitCode !== -1) return 'FAIL: baseline must be an unknown exec failure: ' + debug;
					if (baseline.treeCleanupError) return 'FAIL: a rejected wait must not invent a tree cleanup error: ' + debug;
					var wantsWait = scenario !== 'spawn_rejection';
					if (fixture.baselineWaits !== (wantsWait ? 2 : 0) || fixture.baselineKills !== (wantsWait ? 1 : 0)) return 'FAIL: cleanup wait/kill path was not exercised: ' + debug;
					if (fixture.spawned.length !== 2 || fixture.spawned.filter(function(name) { return name === 'feature'; }).length !== 1 || fixture.spawned.filter(function(name) { return name === 'split/1'; }).length !== 1) return 'FAIL: expected only baseline and split/1 attempts: ' + debug;
					var r0 = s.verificationResults[0];
					var r1 = s.verificationResults[1];
					if (!r0 || r0.name !== 'split/1' || r0.passed || r0.exitCode !== 1 || r0.preExisting) return 'FAIL: split/1 must remain an unforgiven exit 1: ' + debug;
					var baselineRemovals = fixture.removed.filter(function(path) { return path === fixture.worktrees.feature; }).length;
					if (scenario === 'persistent_wait_rejection') {
						if (!baseline.cleanupFailed || !baseline.worktreeRetained || baseline.worktreeDir !== fixture.worktrees.feature || baselineRemovals !== 0) return 'FAIL: unconfirmed child did not retain its worktree: ' + debug;
						if (s.wizard.current !== 'ERROR' || !s.errorDetails || s.errorDetails.indexOf(baseline.worktreeDir) < 0 || s.errorDetails.indexOf('termination was not confirmed') < 0) return 'FAIL: retained baseline must display its cleanup error: ' + debug;
						if (r1 || s.verificationResults.length !== 1) return 'FAIL: baseline cleanup failure must stop before dependent branch: ' + debug;
					} else {
						if (baseline.cleanupFailed || baseline.worktreeRetained || baselineRemovals !== 1) return 'FAIL: ordinary unknown baseline must have confirmed cleanup: ' + debug;
						if (!r1 || r1.name !== 'split/2' || !r1.skipped || r1.passed || r1.preExisting) return 'FAIL: dependent split/2 must be skipped: ' + debug;
						if (!r1.error || r1.error.indexOf('dependency split/1 failed') < 0) return 'FAIL: dependency guard reason missing: ' + debug;
						if (s.verificationResults.length !== 2) return 'FAIL: unexpected result row: ' + debug;
					}
					return 'OK';
				} finally {
					fixture.restore();
				}
			})()`)
			if err != nil {
				t.Fatal(err)
			}
			if raw != "OK" {
				t.Errorf("TUI baseline execFailed regression: %v", raw)
			}
		})
	}
}

// TestChunk16_BaselineTruncatedIsNotBaselineFailure pins the same
// classification for a truncated baseline capture.
func TestChunk16_BaselineTruncatedIsNotBaselineFailure(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		setupPlanCache();
		globalThis.prSplit._state.planCache.splits = [{name: 'split/1', files: ['a.go']}];
		globalThis.prSplit.runtime.verifyCommand = 'make test';
		globalThis.prSplit.runtime.dir = '/tmp/repo';

		var execMod = require('osm:exec');
		var origSpawn = execMod.spawn;
		var origGitAsync = globalThis.prSplit._gitExecAsync;
		// Route async git calls (worktree add/remove) through the mock table
		// instead of the real exec.spawn('git', ...) path.
		globalThis.prSplit._gitExecAsync = function(dir, args) {
			var argv = ['git'];
			if (dir && dir !== '' && dir !== '.') { argv.push('-C'); argv.push(dir); }
			for (var i = 0; i < args.length; i++) argv.push(args[i]);
			var a = argv.slice(1);
			if (a.length >= 2 && a[0] === '-C') a = a.slice(2);
			var key = a.join(' ');
			var r = globalThis._gitResponses[key];
			if (r === undefined) r = globalThis._gitResponses['worktree add'] || {stdout: '', stderr: '', code: 0, error: false, message: ''};
			return r;
		};
		globalThis.__spawnCountT = 0;
		execMod.spawn = function(cmd, args, opts) {
			if (cmd === 'git') return origSpawn.apply(this, arguments);
			globalThis.__spawnCountT++;
			var isBaseline = globalThis.__spawnCountT === 1;
			function truncatedStream() {
				return { read: function() { return Promise.resolve({value: null, done: true, truncated: true}); } };
			}
			function doneStream() {
				return { read: function() { return Promise.resolve({done: true}); } };
			}
			return {
				stdout: isBaseline ? truncatedStream() : doneStream(),
				stderr: doneStream(),
				wait: function() { return Promise.resolve({code: isBaseline ? 0 : 1, signal: null}); },
				kill: function() { return Promise.resolve(); },
				isAlive: function() { return false; },
				close: function() {}
			};
		};
		globalThis._gitResponses = globalThis._gitResponses || {};
		globalThis._gitResponses['worktree add'] = {stdout: '', stderr: '', code: 0, error: false, message: ''};
		globalThis._gitResponses['worktree remove --force'] = globalThis._gitResponses['worktree add'];

		var s = initState('BRANCH_BUILDING');
		s.isProcessing = true;
		s.verifyingIdx = 0;
		s.verifyOutput = {};
		s.verificationResults = [];
		s.outputLines = [];
		s.verifyFallbackRunning = false;
		s._verifyRunEpoch = 1;
		globalThis.prSplit.canSpawnInteractiveShell = function() { return false; };
		globalThis.prSplit.startVerifySession = function() { return { error: 'no capture session', session: null }; };

		for (var i = 0; i < 50; i++) {
			var r = update({type: 'Tick', id: 'verify-branch'}, s); s = r[0];
			for (var j = 0; j < 5; j++) await Promise.resolve();
			await new Promise(function(res) { setTimeout(res, 0); });
			var r2 = update({type: 'Tick', id: 'verify-fallback-poll'}, s); s = r2[0];
			for (var k = 0; k < 5; k++) await Promise.resolve();
			await new Promise(function(res) { setTimeout(res, 0); });
			if (s.verifyingIdx >= 1 && !s.verifyFallbackRunning) break;
		}
		globalThis.prSplit._gitExecAsync = origGitAsync;
		var baseline = s._baselineVerifyResult;
		if (!baseline) return 'FAIL: baseline not cached; started=' + s._baselineVerifyStarted + ' idx=' + s.verifyingIdx;
		if (baseline.failed !== false) return 'FAIL: truncated baseline must be failed:false, got ' + JSON.stringify(baseline);
		var r0 = s.verificationResults[0];
		if (!r0) return 'FAIL: no split result';
		if (r0.preExisting) return 'FAIL: split failure must not be preExisting on a truncated baseline';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("TUI truncated baseline: %v", raw)
	}
}

// TestVerifySplitAsync_TimeoutKeepsTruncatedFlag pins the result-contract fix:
// when a stream read rejects (capture cut short) AND the timeout condition
// also holds, the returned result must keep truncated:true so the batch
// classifier treats it as an unknown capture, not a deterministic timeout.
func TestVerifySplitAsync_TimeoutKeepsTruncatedFlag(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)
	if _, err := evalJS(prsplittest.GitMockSetupJS()); err != nil {
		t.Fatalf("git mock: %v", err)
	}
	if _, err := evalJS(`
		globalThis.__origSpawn06t = globalThis.prSplit._modules.exec.spawn;
		globalThis.prSplit._modules.exec.spawn = function() {
			function rejectingStream() {
				return { read: function() { return Promise.reject(new Error('read: context deadline exceeded')); } };
			}
			return {
				stdout: rejectingStream(),
				stderr: rejectingStream(),
				wait: function() { return Promise.resolve({code: 124, signal: null}); },
				kill: function() { return Promise.resolve(); },
				isAlive: function() { return false; },
				close: function() {}
			};
		};
	`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := evalJS(`globalThis.prSplit._modules.exec.spawn = globalThis.__origSpawn06t; globalThis.__origSpawn06t = undefined;`); err != nil {
			t.Logf("restore spawn mock: %v", err)
		}
	})

	raw, err := evalJS(`await JSON.stringify(await globalThis.prSplit.verifySplitAsync('split/01', {
		dir: '/tmp/repo',
		verifyCommand: 'make test',
		verifyTimeoutMs: 1
	}))`)
	if err != nil {
		t.Fatalf("evalJS: %v", err)
	}
	var result struct {
		Passed    bool    `json:"passed"`
		Truncated bool    `json:"truncated"`
		Error     *string `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("unmarshal %v: %v", raw, err)
	}
	if result.Passed {
		t.Error("a timeout with a cut capture must not pass")
	}
	if !result.Truncated {
		t.Error("a timeout result whose streams were cut short must keep truncated:true (unknown capture)")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "timeout") {
		t.Errorf("error = %v, want the timeout diagnostic retained", result.Error)
	}
}

// TestVerifySplitAsync_AbnormalExitAwaitsKillBeforeWorktreeRemoval pins the
// cleanup ordering: when the body exits abnormally before wait settles (a
// throwing output callback), the finally must await the kill AND the child's
// exit (bounded) before removing the worktree.
func TestVerifySplitAsync_AbnormalExitAwaitsKillBeforeWorktreeRemoval(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)
	if _, err := evalJS(prsplittest.GitMockSetupJS()); err != nil {
		t.Fatalf("git mock: %v", err)
	}
	raw, err := evalJS(`(async function() {
		var order = [];
		globalThis.__order = order;
		var origSpawn = globalThis.prSplit._modules.exec.spawn;
		globalThis.prSplit._modules.exec.spawn = function() {
			// The body must exit abnormally before wait settles: deliver one
			// stdout chunk so the throwing outputFn unwinds out of readStream,
			// while wait() stays pending until kill is requested.
			var chunkSent = false;
			function stdoutStream() {
				return { read: function() {
					if (!chunkSent) { chunkSent = true; return Promise.resolve({value: 'boom\\n', done: false}); }
					return Promise.resolve({done: true});
				} };
			}
			function doneStream() {
				return { read: function() { return Promise.resolve({done: true}); } };
			}
			var killed = false;
			var child = {
				stdout: stdoutStream(),
				stderr: doneStream(),
				wait: function() {
					if (killed) { order.push('wait-after-kill'); return Promise.resolve({code: -1, signal: 'SIGKILL'}); }
					return new Promise(function() {});
				},
				kill: function() { killed = true; order.push('kill-requested'); return Promise.resolve(); },
				isAlive: function() { return !killed; },
				close: function() {}
			};
			return child;
		};
		var origGitAsync = globalThis.prSplit._gitExecAsync;
		globalThis.prSplit._gitExecAsync = function(dir, args) {
			if (args[0] === 'worktree' && args[1] === 'remove') {
				order.push('worktree-remove killSettled:' + (order.indexOf('wait-after-kill') >= 0));
			}
			return {stdout: '', stderr: '', code: 0, error: false, message: ''};
		};
		try {
			await globalThis.prSplit.verifySplitAsync('split/01', {
				dir: '/tmp/repo',
				verifyCommand: 'make test',
				outputFn: function() { throw new Error('output callback exploded'); }
			});
		} catch (e) {
			return 'FAIL: verifySplitAsync rejected: ' + e;
		}
		globalThis.prSplit._modules.exec.spawn = origSpawn;
		globalThis.prSplit._gitExecAsync = origGitAsync;
		if (order.indexOf('wait-after-kill') < 0) return 'FAIL: kill/termination was not awaited, order=' + JSON.stringify(order);
		if (order.indexOf('worktree-remove killSettled:true') < 0) return 'FAIL: worktree removed before child terminated, order=' + JSON.stringify(order);
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("abnormal-exit cleanup ordering: %v", raw)
	}
}
