package command

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// TestChunk16_BaselineExecFailureIsNotBaselineFailure is the TUI-side
// regression for the review-11/12 P1: the baseline verifySplitAsync now
// resolves (never rejects) with {passed:false, execFailed:true} when the
// exec layer breaks during baseline worktree setup. The TUI baseline
// callback must classify that as "no information" (failed:false), NOT as a
// baseline verification failure — otherwise a genuinely failing split is
// labelled pre-existing and its dependents bypass the dependency guard.
//
// Scenario: baseline spawn rejects; split/1 exits 1; split/2 depends on
// split/1. Assert: cached baseline failed===false; split/1 result
// preExisting===false; split/2 is skipped with the dependency-failed reason.
func TestChunk16_BaselineExecFailureIsNotBaselineFailure(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		setupPlanCache();
		// Replace the 3-split plan with two splits where split/2 depends on split/1.
		globalThis.prSplit._state.planCache.splits = [
			{name: 'split/1', files: ['a.go']},
			{name: 'split/2', files: ['b.go'], dependencies: ['split/1']}
		];
		globalThis.prSplit.runtime.verifyCommand = 'make test';
		globalThis.prSplit.runtime.dir = '/tmp/repo';

		// Install the git mock (worktree add/remove succeed) but break the
		// verify child spawn: the FIRST spawned verify child is the baseline.
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
		globalThis.__spawnCount = 0;
		execMod.spawn = function(cmd, args, opts) {
			if (cmd === 'git') return origSpawn.apply(this, arguments);
			globalThis.__spawnCount++;
			var isBaseline = globalThis.__spawnCount === 1;
			function doneStream() {
				return { read: function() { return Promise.resolve({done: true}); } };
			}
			return {
				stdout: doneStream(),
				stderr: doneStream(),
				wait: function() {
					if (isBaseline) return Promise.reject(new Error('spawn failed: resource temporarily unavailable'));
					return Promise.resolve({code: 1, signal: null});
				},
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

		// Drive the verify-branch handler for both splits. Force the fallback
		// (textonly) path so runVerifyFallbackAsync -> verifySplitAsync runs.
		globalThis.prSplit.canSpawnInteractiveShell = function() { return false; };
		globalThis.prSplit.startVerifySession = function() { return { error: 'no capture session', session: null }; };

		async function tickVerify() {
			for (var i = 0; i < 50; i++) {
				var r = update({type: 'Tick', id: 'verify-branch'}, s);
				s = r[0];
				// Let the baseline promise and the fallback promise settle:
				// the event loop resolves tracked promises on its own poll,
				// so drain both microtasks and a macrotask boundary.
				for (var j = 0; j < 5; j++) await Promise.resolve();
				await new Promise(function(res) { setTimeout(res, 0); });
				var r2 = update({type: 'Tick', id: 'verify-fallback-poll'}, s);
				s = r2[0];
				for (var k = 0; k < 5; k++) await Promise.resolve();
				await new Promise(function(res) { setTimeout(res, 0); });
				if (s.verifyingIdx >= 2 && !s.verifyFallbackRunning) break;
			}
			globalThis.prSplit._gitExecAsync = origGitAsync;
			return { started: s._baselineVerifyStarted, idx: s.verifyingIdx, results: s.verificationResults.length, epoch: s._verifyRunEpoch, processing: s.isProcessing, phase: s.verifyPhase, baseline: s._baselineVerifyResult };
		}
		var dbg = await tickVerify();
		if (dbg.started !== true) return 'FAIL: baseline never started; dbg=' + JSON.stringify(dbg);
		if (!s._baselineVerifyResult) return 'FAIL: baseline started but result never cached; dbg=' + JSON.stringify(dbg);

		var baseline = s._baselineVerifyResult;
		if (baseline.failed !== false) return 'FAIL: an execFailed baseline must be classified as no-info (failed:false), got ' + JSON.stringify(baseline);
		var r0 = s.verificationResults[0];
		var r1 = s.verificationResults[1];
		if (!r0) return 'FAIL: no result for split/1';
		if (r0.preExisting) return 'FAIL: split/1 failure must NOT be preExisting when the baseline verdict is unknown, got ' + JSON.stringify(r0);
		if (!r1) return 'FAIL: no result for split/2';
		if (!r1.skipped) return 'FAIL: split/2 must be skipped because its dependency split/1 failed, got ' + JSON.stringify(r1);
		if (!r1.error || r1.error.indexOf('dependency split/1 failed') < 0) return 'FAIL: split/2 skip reason wrong: ' + r1.error;
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("TUI baseline execFailed regression: %v", raw)
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
