package command

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// Round-7 regression tests for review-13 P1#1 / review-15 P1#1A and P1#1B:
// an UNKNOWN baseline (exec-layer failure with no exit status, or an
// incomplete capture with a real exit code) must never be classified as a
// deterministic failure/timeout, so it must disable pre-existing detection:
// a failing split must NOT acquire preExisting, allPassed must be false, and
// a dependent split must be skipped.

// runVerifySplitsWithShMock installs the git mock, routes every non-git execv
// through the provided sh handler (call #1 = baseline), and runs verifySplits
// with a small verifyTimeoutMs plus a Date.now mock that makes elapsed time
// exceed the timeout on demand.
func runVerifySplitsWithShMock(t *testing.T, shHandler string, timeoutMs int, jumpAfter int) verifySplitsResult {
	t.Helper()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)
	if _, err := evalJS(prsplittest.GitMockSetupJS()); err != nil {
		t.Fatalf("git mock: %v", err)
	}
	if _, err := evalJS(resetGitMockJS); err != nil {
		t.Fatal(err)
	}
	if _, err := evalJS(`
		_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature\n');
		_gitResponses['checkout'] = _gitOk('');
		_gitResponses['!sh'] = ` + shHandler + `;
		(function() {
			var originalDateNow = Date.now;
			var callCount = 0;
			Date.now = function() {
				callCount++;
				if (callCount <= ` + itoa(jumpAfter) + `) return originalDateNow.call(Date);
				return originalDateNow.call(Date) + 600000;
			};
		})();
	`); err != nil {
		t.Fatal(err)
	}
	raw, err := evalJS(`JSON.stringify(await globalThis.prSplit.verifySplits({
		dir: '/tmp/test',
		sourceBranch: 'feature',
		verifyCommand: 'make test',
		splits: [
			{name: 'split/1', files: ['a.go']},
			{name: 'split/2', files: ['b.go'], dependencies: ['split/1']}
		]
	}, {verifyTimeoutMs: ` + itoa(timeoutMs) + `}))`)
	if err != nil {
		t.Fatalf("verifySplits: %v", err)
	}
	return parseVerifySplitsResult(t, raw)
}

// TestVerifySplits_UnknownBaselineAtTimeoutIsNotDeterministic pins V1:
// baseline returns code:-1 + error (wait failed, no exit status) while the
// mocked clock reports elapsed >= timeout. The baseline verdict is unknown,
// so split/1 (exit 1) must NOT be preExisting, allPassed must be false, and
// split/2 must be skipped as a dependency failure.
func TestVerifySplits_UnknownBaselineAtTimeoutIsNotDeterministic(t *testing.T) {
	t.Parallel()
	// call #1 = baseline -> unknown exec failure; calls #2,#3 = splits.
	// Date.now jumps past the timeout after 2 calls so the baseline's
	// elapsedMs >= timeoutMs while its code is -1 with error.
	r := runVerifySplitsWithShMock(t, `
		function(argv) {
			var n = (globalThis._shCalls = (globalThis._shCalls || 0) + 1);
			if (n === 1) return {stdout: '', stderr: '', code: -1, error: true, message: 'wait failed: no exit status'};
			if (n === 2) return {stdout: '', stderr: 'boom', code: 1, error: true, message: 'exit status 1'};
			return _gitOk('ok');
		}
	`, 1000, 2)

	if r.AllPassed {
		t.Errorf("allPassed = true, want false: an unknown baseline must not forgive a failing split (results=%s)", mustJSON(t, r))
	}
	if len(r.Results) != 2 {
		t.Fatalf("results = %d, want 2 (%s)", len(r.Results), mustJSON(t, r))
	}
	r0 := r.Results[0]
	if r0.PreExisting {
		t.Errorf("split/1 preExisting = true, want false with an unknown baseline (%s)", mustJSON(t, r0))
	}
	if r0.Passed {
		t.Errorf("split/1 passed = true, want false (exit 1)")
	}
	r1 := r.Results[1]
	if !r1.Skipped {
		t.Errorf("split/2 skipped = false, want true (dependency split/1 failed) (%s)", mustJSON(t, r1))
	}
	if r1.Error == nil || !strings.Contains(*r1.Error, "dependency split/1 failed") {
		t.Errorf("split/2 error = %v, want dependency-failed reason", r1.Error)
	}
}

// TestVerifySplits_UnknownSplitAtTimeoutIsNotPreExisting pins the asymmetric
// case: a deterministic baseline failure (exit 1) exists, but the SPLIT's
// verdict is unknown at timeout (code -1 + error + elapsed >= timeout). The
// split must be execFailed, never preExisting, and allPassed must be false.
func TestVerifySplits_UnknownSplitAtTimeoutIsNotPreExisting(t *testing.T) {
	t.Parallel()
	r := runVerifySplitsWithShMock(t, `
		function(argv) {
			var n = (globalThis._shCalls2 = (globalThis._shCalls2 || 0) + 1);
			if (n === 1) return {stdout: '', stderr: 'baseline fails', code: 1, error: true, message: 'exit status 1'};
			return {stdout: '', stderr: '', code: -1, error: true, message: 'wait failed: no exit status'};
		}
	`, 1000, 2)

	if r.AllPassed {
		t.Errorf("allPassed = true, want false (%s)", mustJSON(t, r))
	}
	if len(r.Results) != 2 {
		t.Fatalf("results = %d, want 2 (%s)", len(r.Results), mustJSON(t, r))
	}
	r0 := r.Results[0]
	if r0.PreExisting {
		t.Errorf("unknown split verdict acquired preExisting=true, want false (%s)", mustJSON(t, r0))
	}
	if r0.Passed {
		t.Errorf("unknown split passed = true, want false")
	}
}

// TestVerifySplits_TruncatedBaselineWithKnownExitCode pins V1b: a baseline
// whose capture is incomplete (structural truncated flag) with a REAL nonzero
// exit code 7 and an EIO-style message that does not match the old regex must
// still be an unknown verdict — not a deterministic failure.
func TestVerifySplits_TruncatedBaselineWithKnownExitCode(t *testing.T) {
	t.Parallel()
	r := runVerifySplitsWithShMock(t, `
		function(argv) {
			var n = (globalThis._shCalls3 = (globalThis._shCalls3 || 0) + 1);
			if (n === 1) return {stdout: 'partial', stderr: '', code: 7, error: true, message: 'exit status 7\ninput/output error', truncated: true};
			if (n === 2) return {stdout: '', stderr: 'boom', code: 1, error: true, message: 'exit status 1'};
			return _gitOk('ok');
		}
	`, 600000, 1000)

	if r.AllPassed {
		t.Errorf("allPassed = true, want false: an incomplete baseline capture must not forgive failing splits (%s)", mustJSON(t, r))
	}
	if len(r.Results) != 2 {
		t.Fatalf("results = %d, want 2 (%s)", len(r.Results), mustJSON(t, r))
	}
	r0 := r.Results[0]
	if r0.PreExisting {
		t.Errorf("split/1 preExisting = true, want false with a truncated baseline (%s)", mustJSON(t, r0))
	}
	r1 := r.Results[1]
	if !r1.Skipped {
		t.Errorf("split/2 skipped = false, want true (dependency failed) (%s)", mustJSON(t, r1))
	}
}

// ---------------------------------------------------------------------------
// Round-7 async cleanup tests (V2/V3/V4)
// ---------------------------------------------------------------------------

// asyncVerifySetupJS installs the git mock routing, worktree-remove tracking,
// and a setTimeout wrapper that records every scheduled handle so tests can
// count pending cleanup timers after verifySplitAsync resolves.
const asyncVerifySetupJS = `
	globalThis._gitResponses = globalThis._gitResponses || {};
	globalThis._gitResponses['worktree add'] = {stdout: '', stderr: '', code: 0, error: false, message: ''};
	globalThis._gitResponses['worktree remove --force'] = {stdout: '', stderr: '', code: 0, error: false, message: ''};
	globalThis.__removeCalls = [];
	globalThis.__origGitAsync = globalThis.prSplit._gitExecAsync;
	globalThis.prSplit._gitExecAsync = function(dir, args) {
		if (args[0] === 'worktree' && args[1] === 'remove') {
			globalThis.__removeCalls.push({dir: dir, args: args.slice()});
			return globalThis.__removeResponse || globalThis._gitResponses['worktree remove --force'];
		}
		var argv = ['git'];
		if (dir && dir !== '' && dir !== '.') { argv.push('-C'); argv.push(dir); }
		for (var i = 0; i < args.length; i++) argv.push(args[i]);
		var a = argv.slice(1);
		if (a.length >= 2 && a[0] === '-C') a = a.slice(2);
		var key = a.join(' ');
		var r = globalThis._gitResponses[key];
		if (r === undefined) r = globalThis._gitResponses['worktree add'];
		return r;
	};
	globalThis.__origSpawnR7 = globalThis.prSplit._modules.exec.spawn;
	globalThis.prSplit._modules.exec.spawn = function() {
		return globalThis.__childFactory.apply(null, arguments);
	};
	globalThis.__pending = {};
	globalThis.__timerSeq = 0;
	globalThis.__origSetTimeout = globalThis.setTimeout;
	globalThis.__origClearTimeout = globalThis.clearTimeout;
	globalThis.__timerHandles = {};
	globalThis.setTimeout = function(fn, ms) {
		var id = ++globalThis.__timerSeq;
		globalThis.__pending[id] = true;
		var h = globalThis.__origSetTimeout.call(globalThis, function() {
			delete globalThis.__pending[id];
			if (fn) fn();
		}, ms);
		globalThis.__timerHandles[id] = h;
		return h;
	};
	globalThis.clearTimeout = function(h) {
		for (var k in globalThis.__timerHandles) {
			if (globalThis.__timerHandles[k] === h) {
				delete globalThis.__pending[k];
				delete globalThis.__timerHandles[k];
			}
		}
		return globalThis.__origClearTimeout.call(globalThis, h);
	};
	globalThis.__pendingTimerCount = function() {
		var n = 0;
		for (var k in globalThis.__pending) n++;
		return n;
	};
`

// asyncVerifyTeardownJS restores the original spawn/git/timer functions.
const asyncVerifyTeardownJS = `
	globalThis.prSplit._modules.exec.spawn = globalThis.__origSpawnR7;
	globalThis.prSplit._gitExecAsync = globalThis.__origGitAsync;
	globalThis.setTimeout = globalThis.__origSetTimeout;
	globalThis.clearTimeout = globalThis.__origClearTimeout;
	globalThis.__origSpawnR7 = undefined;
	globalThis.__origGitAsync = undefined;
	globalThis.__origSetTimeout = undefined;
	globalThis.__origClearTimeout = undefined;
	globalThis.__pending = undefined;
	globalThis.__timerHandles = undefined;
	globalThis.__childFactory = undefined;
	globalThis.__removeResponse = undefined;
	globalThis.__removeCalls = undefined;
`

type asyncVerifyResult struct {
	Passed           bool    `json:"passed"`
	ExecFailed       bool    `json:"execFailed"`
	Truncated        bool    `json:"truncated"`
	TimedOut         bool    `json:"timedOut"`
	CleanupFailed    bool    `json:"cleanupFailed"`
	WorktreeRetained bool    `json:"worktreeRetained"`
	WorktreeDir      string  `json:"worktreeDir"`
	ExitCode         int     `json:"exitCode"`
	Error            *string `json:"error"`
}

// doneStreamJS is a stream factory helper used by the child factories below.
const doneStreamJS = `function doneStream() {
	return { read: function() { return Promise.resolve({done: true}); } };
}`

// runAsyncVerifyWithFactory installs the mocks, sets __childFactory, runs
// verifySplitAsync, and returns the parsed result plus the pending-timer and
// remove-call counts observed right after the call resolves.
func runAsyncVerifyWithFactory(t *testing.T, factoryJS string, extraConfig string) (asyncVerifyResult, int, int) {
	t.Helper()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)
	if _, err := evalJS(prsplittest.GitMockSetupJS()); err != nil {
		t.Fatalf("git mock: %v", err)
	}
	if _, err := evalJS(asyncVerifySetupJS); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := evalJS(asyncVerifyTeardownJS); err != nil {
			t.Logf("teardown: %v", err)
		}
	})
	if _, err := evalJS(`globalThis.__childFactory = ` + factoryJS + `;`); err != nil {
		t.Fatal(err)
	}
	cfg := `{dir: '/tmp/repo', verifyCommand: 'make test'`
	if extraConfig != "" {
		cfg += ", " + extraConfig
	}
	cfg += `}`
	raw, err := evalJS(`(async function() {
		var r = await globalThis.prSplit.verifySplitAsync('split/01', ` + cfg + `);
		return JSON.stringify({result: r, pending: globalThis.__pendingTimerCount(), removes: globalThis.__removeCalls.length});
	})()`)
	if err != nil {
		t.Fatalf("verifySplitAsync: %v", err)
	}
	var wrapped struct {
		Result  asyncVerifyResult `json:"result"`
		Pending int               `json:"pending"`
		Removes int               `json:"removes"`
	}
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string, got %T: %v", raw, raw)
	}
	if err := json.Unmarshal([]byte(s), &wrapped); err != nil {
		t.Fatalf("unmarshal %s: %v", s, err)
	}
	return wrapped.Result, wrapped.Pending, wrapped.Removes
}

// TestVerifySplitAsync_AbnormalExitLeavesNoPendingTimers pins V2 + review-16
// B2/B3: after an abnormal-exit cleanup (throwing output callback -> kill +
// bounded wait + worktree removal), every cleanup timer must have fired or
// been cleared, AND the cleanup budget must be computed in finally decoupled
// from verifyTimeoutMs so a macrotask-resolving post-kill wait (like a real
// child reaper) does not lose to an already-expired 0ms timer.
func TestVerifySplitAsync_AbnormalExitLeavesNoPendingTimers(t *testing.T) {
	t.Parallel()
	res, pending, removes := runAsyncVerifyWithFactory(t, `
		function() {
			var chunkSent = false;
			`+doneStreamJS+`
			function stdoutStream() {
				return { read: function() {
					if (!chunkSent) { chunkSent = true; return Promise.resolve({value: 'boom\n', done: false}); }
					return Promise.resolve({done: true});
				} };
			}
			var killed = false;
			return {
				stdout: stdoutStream(),
				stderr: doneStream(),
				wait: function() {
					if (killed) {
						return new Promise(function(resolve) {
							globalThis.__origSetTimeout.call(globalThis, function() {
								resolve({code: -1, signal: 'SIGKILL'});
							}, 20);
						});
					}
					return new Promise(function() {});
				},
				kill: function() { killed = true; return Promise.resolve(); },
				isAlive: function() { return !killed; },
				close: function() {}
			};
		}
	`, `verifyTimeoutMs: 1, outputFn: function() { throw new Error('output callback exploded'); }`)

	if pending != 0 {
		t.Errorf("pending cleanup timers = %d, want 0 (every boundedAwait timer must be cleared)", pending)
	}
	if removes != 1 {
		t.Errorf("worktree remove calls = %d, want 1 (termination confirmed via post-kill wait)", removes)
	}
	if !res.ExecFailed {
		t.Errorf("execFailed = false, want true when outputFn threw")
	}
	if res.CleanupFailed {
		t.Errorf("cleanupFailed = true, want false for a clean abnormal-exit cleanup (error=%v)", res.Error)
	}
	if res.WorktreeRetained {
		t.Errorf("worktreeRetained = true, want false when the post-kill wait resolved with a status")
	}
}

// TestVerifySplitAsync_ThrowingOutputFnSurfacesCleanupDiagnostics pins
// review-16 Blocker 3: when the body throws (outputFn explodes) AND cleanup
// fails (post-kill wait rejects so termination is unconfirmed), verifySplitAsync
// must construct an outcome in catch before finally so worktreeRetained,
// cleanupFailed, worktreeDir, and the cleanup note are NOT discarded.
func TestVerifySplitAsync_ThrowingOutputFnSurfacesCleanupDiagnostics(t *testing.T) {
	t.Parallel()
	res, pending, removes := runAsyncVerifyWithFactory(t, `
		function() {
			var chunkSent = false;
			`+doneStreamJS+`
			function stdoutStream() {
				return { read: function() {
					if (!chunkSent) { chunkSent = true; return Promise.resolve({value: 'boom\n', done: false}); }
					return Promise.resolve({done: true});
				} };
			}
			var killed = false;
			return {
				stdout: stdoutStream(),
				stderr: doneStream(),
				wait: function() {
					if (killed) return Promise.reject(new Error('reaper unavailable'));
					return new Promise(function() {});
				},
				kill: function() { killed = true; return Promise.resolve(); },
				isAlive: function() { return !killed; },
				close: function() {}
			};
		}
	`, `outputFn: function() { throw new Error('output callback exploded'); }`)

	if pending != 0 {
		t.Errorf("pending cleanup timers = %d, want 0", pending)
	}
	if removes != 0 {
		t.Errorf("worktree remove calls = %d, want 0 (worktree must be retained when post-kill wait rejects)", removes)
	}
	if !res.ExecFailed {
		t.Errorf("execFailed = false, want true")
	}
	if !res.CleanupFailed {
		t.Errorf("cleanupFailed = false, want true (error=%v)", res.Error)
	}
	if !res.WorktreeRetained {
		t.Errorf("worktreeRetained = false, want true (error=%v)", res.Error)
	}
	if res.WorktreeDir == "" {
		t.Errorf("worktreeDir = empty, want retained worktree path")
	}
	if res.Error == nil || !strings.Contains(*res.Error, "worktree retained") {
		t.Errorf("error = %v, want 'worktree retained' cleanup diagnostic", res.Error)
	}
}

// TestVerifySplit_SyncTimeoutPreservesTimedOutAlongsideTruncated pins review-16:
// when a sync verifySplit hits its timeout AND the exec layer reports a
// truncated capture, the result must preserve BOTH truncated:true (unknown
// verdict) AND timedOut:true with a timeout diagnostic.
func TestVerifySplit_SyncTimeoutPreservesTimedOutAlongsideTruncated(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)
	if _, err := evalJS(prsplittest.GitMockSetupJS()); err != nil {
		t.Fatalf("git mock: %v", err)
	}
	if _, err := evalJS(resetGitMockJS); err != nil {
		t.Fatal(err)
	}
	if _, err := evalJS(`
		_gitResponses['!sh'] = function() {
			return {stdout: 'partial', stderr: '', code: 124, error: true, truncated: true, message: 'command output capture aborted before EOF'};
		};
	`); err != nil {
		t.Fatal(err)
	}
	raw, err := evalJS(`JSON.stringify(await globalThis.prSplit.verifySplit('split/01', {
		dir: '/tmp/repo',
		verifyCommand: 'make test',
		verifyTimeoutMs: 500
	}))`)
	if err != nil {
		t.Fatalf("verifySplit: %v", err)
	}
	var res asyncVerifyResult
	if err := json.Unmarshal([]byte(raw.(string)), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if res.Passed {
		t.Errorf("passed = true, want false")
	}
	if !res.Truncated {
		t.Errorf("truncated = false, want true")
	}
	if !res.TimedOut {
		t.Errorf("timedOut = false, want true when verify timed out alongside truncation")
	}
	if res.Error == nil || !strings.Contains(*res.Error, "timeout") {
		t.Errorf("error = %v, want timeout attribution preserved", res.Error)
	}
}

// TestVerifySplitAsync_RejectedWaitRetainsWorktree pins V3: when the verify
// child's wait() REJECTS (no exit status), termination is never confirmed, so
// the finally must kill the child, leave the worktree in place, and report
// worktreeRetained + cleanupFailed in the result.
func TestVerifySplitAsync_RejectedWaitRetainsWorktree(t *testing.T) {
	t.Parallel()
	res, _, removes := runAsyncVerifyWithFactory(t, `
		function() {
			`+doneStreamJS+`
			var killed = false;
			return {
				stdout: doneStream(),
				stderr: doneStream(),
				wait: function() { return Promise.reject(new Error('wait failed: context cancelled')); },
				kill: function() { killed = true; return Promise.resolve(); },
				isAlive: function() { return !killed; },
				close: function() {}
			};
		}
	`, ``)

	if !res.ExecFailed {
		t.Errorf("execFailed = false, want true for a rejected wait (no exit status)")
	}
	if !res.WorktreeRetained {
		t.Errorf("worktreeRetained = %v, want true (a rejected wait never confirms termination)", res.WorktreeRetained)
	}
	if !res.CleanupFailed {
		t.Errorf("cleanupFailed = false, want true when the worktree is retained")
	}
	if removes != 0 {
		t.Errorf("worktree remove calls = %d, want 0 (worktree must be retained, not removed)", removes)
	}
	if res.Error == nil || !strings.Contains(*res.Error, "worktree retained") {
		t.Errorf("error = %v, want the 'worktree retained' cleanup note", res.Error)
	}
}

// TestVerifySplitAsync_RemovalFailureSetsCleanupFailed pins V4: when the
// worktree removal returns a nonzero code, the result must carry
// cleanupFailed:true and the removal reason — not just a debug log.
func TestVerifySplitAsync_RemovalFailureSetsCleanupFailed(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)
	if _, err := evalJS(prsplittest.GitMockSetupJS()); err != nil {
		t.Fatalf("git mock: %v", err)
	}
	if _, err := evalJS(asyncVerifySetupJS); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := evalJS(asyncVerifyTeardownJS); err != nil {
			t.Logf("teardown: %v", err)
		}
	})
	if _, err := evalJS(`
		globalThis.__removeResponse = {stdout: '', stderr: 'fatal: cannot remove', code: 128, error: true, message: 'exit status 128'};
		globalThis.__childFactory = function() {
			` + doneStreamJS + `
			return {
				stdout: doneStream(),
				stderr: doneStream(),
				wait: function() { return Promise.resolve({code: 0, signal: null}); },
				kill: function() { return Promise.resolve(); },
				isAlive: function() { return false; },
				close: function() {}
			};
		};
	`); err != nil {
		t.Fatal(err)
	}
	raw, err := evalJS(`JSON.stringify(await globalThis.prSplit.verifySplitAsync('split/01', {dir: '/tmp/repo', verifyCommand: 'make test'}))`)
	if err != nil {
		t.Fatalf("verifySplitAsync: %v", err)
	}
	var res asyncVerifyResult
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string, got %T", raw)
	}
	if err := json.Unmarshal([]byte(s), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !res.CleanupFailed {
		t.Errorf("cleanupFailed = false, want true when worktree removal returns exit 128")
	}
	if res.WorktreeRetained {
		t.Errorf("worktreeRetained = true, want false (the wait resolved with a status; removal was attempted)")
	}
	if res.Error == nil || !strings.Contains(*res.Error, "worktree removal failed") {
		t.Errorf("error = %v, want the 'worktree removal failed' cleanup note", res.Error)
	}
	if res.WorktreeDir == "" {
		t.Errorf("worktreeDir = empty, want the worktree path in the result")
	}
}
