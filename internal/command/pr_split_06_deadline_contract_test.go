package command

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// The clock advances only inside native command/cleanup boundaries. No sleep
// or scheduler ordering stands in for the verifier's measured body duration.
func TestVerifySplit_DeadlineFactsRemainIndependent(t *testing.T) {
	skipSlow(t)
	eval := prsplittest.NewChunkEngine(t, nil, "00_core", "06_verification")
	raw, err := eval(verifyDeadlineScenario)
	if err != nil {
		t.Fatal(err)
	}
	var reports []struct {
		Method string
		Test   struct {
			Name                                                          string
			Code, BodyMs, CleanupMs                                       int
			Truncated, Callback, CleanupFailure, TreeFailure, CleanupReap bool
		}
		Result                         treeCleanupResult
		Waits, Kills, Removes, Pending int
		HasExitCode                    bool
	}
	if err := json.Unmarshal([]byte(raw.(string)), &reports); err != nil {
		t.Fatal(err)
	}
	if len(reports) != 32 {
		t.Fatalf("got %d observations, want 32", len(reports))
	}
	for _, got := range reports {
		t.Run(got.Method+"/"+got.Test.Name, func(t *testing.T) {
			tc, r := got.Test, got.Result
			timedOut := tc.BodyMs >= 1000
			failedCleanup := tc.CleanupFailure || tc.TreeFailure
			passed := tc.Code == 0 && !timedOut && !tc.Truncated && !tc.Callback && !failedCleanup
			if !got.HasExitCode || r.ExitCode != tc.Code || r.TimedOut != timedOut || r.Truncated != tc.Truncated ||
				r.Passed != passed || r.ExecFailed != (tc.Callback || failedCleanup) || r.Output != "captured prefix\n" {
				t.Fatalf("independent command facts changed: %+v", got)
			}
			if tc.Callback {
				if r.Error == nil || !strings.Contains(*r.Error, "observer failed") {
					t.Fatalf("original observer failure was lost: %+v", got)
				}
			} else if got.Method == "verifySplit" && tc.TreeFailure && !tc.Truncated {
				if r.Error == nil || !strings.Contains(*r.Error, "verify exec-layer failure") {
					t.Fatalf("primary execution-layer diagnostic was replaced: %+v", got)
				}
			} else if timedOut {
				if r.Error == nil || !strings.Contains(*r.Error, "verify timeout after 1000ms (limit: 1000ms)") {
					t.Fatalf("deadline diagnostic must use the body duration: %+v", got)
				}
			} else if r.Error != nil && strings.Contains(*r.Error, "verify timeout") {
				t.Fatalf("ordinary exit or cleanup was mislabelled as a verification timeout: %+v", got)
			}
			waits, kills, removes := 0, 0, 1
			if got.Method == "verifySplitAsync" {
				waits = 1
			}
			if tc.CleanupReap {
				waits, kills = 2, 1
			}
			if tc.TreeFailure {
				removes = 0
			}
			if got.Waits != waits || got.Kills != kills || got.Removes != removes || got.Pending != 0 ||
				r.CleanupFailed != failedCleanup || r.WorktreeRetained != tc.TreeFailure {
				t.Fatalf("deadline attribution changed cleanup ownership or timer lifetime: %+v", got)
			}
			if tc.TreeFailure && (r.TreeCleanupError != "tree close denied" || r.WorktreeDir == "") {
				t.Fatalf("tree failure lost its diagnostic/path: %+v", got)
			}
			if tc.CleanupFailure && (r.Error == nil || !strings.Contains(*r.Error, "removal denied") || r.WorktreeDir == "") {
				t.Fatalf("attempted removal lost its diagnostic/path: %+v", got)
			}
		})
	}
}

// Real shells can return these ordinary statuses immediately. Neither native
// execv capture nor spawn/wait gives them a reserved timeout meaning. The
// generous budget tests attribution without imposing a startup speed bound.
func TestVerifySplit_NativeImmediateExitDoesNotImplyTimeout(t *testing.T) {
	skipSlow(t)
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, t.TempDir())
	}
	t.Setenv("GIT_CONFIG_COUNT", "0")
	t.Setenv("GIT_CONFIG_PARAMETERS", "")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_TEMPLATE_DIR", t.TempDir())
	dir := initGitRepo(t)
	gitCmd(t, dir, "commit", "--allow-empty", "-m", "immediate exit fixture")
	quotedDir, err := json.Marshal(dir)
	if err != nil {
		t.Fatal(err)
	}
	eval := prsplittest.NewChunkEngine(t, nil, "00_core", "06_verification")
	for _, method := range []string{"verifySplit", "verifySplitAsync"} {
		for _, code := range []int{124, 137, 143} {
			t.Run(fmt.Sprintf("%s/exit_%d", method, code), func(t *testing.T) {
				raw, err := eval(fmt.Sprintf(`(async function() {
                    return JSON.stringify(await prSplit.%s('main', {
                        dir:%s, verifyCommand:'exit %d', verifyTimeoutMs:600000
                    }));
                })()`, method, quotedDir, code))
				if err != nil {
					t.Fatal(err)
				}
				var result treeCleanupResult
				if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
					t.Fatal(err)
				}
				if result.ExitCode != code || result.Passed || result.TimedOut || result.Truncated ||
					result.ExecFailed || result.CleanupFailed || result.WorktreeRetained {
					t.Fatalf("immediate native exit must remain an ordinary command failure: %s", raw)
				}
				if result.Error == nil || !strings.Contains(*result.Error, fmt.Sprintf("exit %d", code)) ||
					strings.Contains(*result.Error, "verify timeout") {
					t.Fatalf("native failure lost the real command status: %s", raw)
				}
				if got := gitCmd(t, dir, "worktree", "list", "--porcelain"); strings.Count(got, "worktree ") != 1 {
					t.Fatalf("verification worktree was not removed: %s", got)
				}
			})
		}
	}
}

const verifyDeadlineScenario = `(async function() {
    var p = prSplit, native = p._modules.exec, reports = [];
    var cases = [
        {name:'immediate_124', code:124},
        {name:'immediate_137', code:137},
        {name:'immediate_143', code:143},
        {name:'immediate_143_failed_removal', code:143, cleanupFailure:true},
        {name:'success_before_deadline', code:0},
        {name:'ordinary_failure_before_deadline', code:7},
        {name:'slow_cleanup_only', code:0, cleanupMs:10000},
        {name:'deadline_and_capture', code:7, bodyMs:1000, truncated:true},
        {name:'deadline_capture_failed_removal', code:0, bodyMs:1000, truncated:true, cleanupFailure:true},
        {name:'deadline_capture_tree_failure', code:7, bodyMs:1000, truncated:true, treeFailure:true},
        {name:'deadline_complete_capture_tree_failure', code:0, bodyMs:1000, treeFailure:true},
        {name:'observer_at_deadline', code:7, bodyMs:1000, callback:true},
        {name:'observer_capture_at_deadline', code:0, bodyMs:1000, callback:true, truncated:true},
        {name:'observer_before_slow_cleanup', code:7, cleanupMs:10000, callback:true},
        {name:'observer_at_deadline_failed_removal', code:0, bodyMs:1000, callback:true, cleanupFailure:true}
    ];
    for (var method of ['verifySplit', 'verifySplitAsync']) {
        var methodCases = cases.slice();
        if (method === 'verifySplitAsync') {
            methodCases.push({name:'cleanup_reap_after_observer_deadline', code:7,
                bodyMs:1000, callback:true, cleanupReap:true});
            methodCases.push({name:'cleanup_reap_after_observer_before_deadline', code:0,
                cleanupMs:10000, callback:true, cleanupReap:true});
        }
        for (var test of methodCases) {
            var originalSpawn = native.spawn, originalExecv = native.execv;
            var originalNow = Date.now, originalSetTimeout = setTimeout, originalClearTimeout = clearTimeout;
            var now = 100000, waits = 0, kills = 0, removes = 0, sent = false;
            var pending = new Set(), releaseOutput, releaseWait;
            var outputGate = new Promise(function(resolve) { releaseOutput = resolve; });
            var waitGate = new Promise(function(resolve) { releaseWait = resolve; });
            Date.now = function() { return now; };
            globalThis.setTimeout = function(fn, ms) {
                var id = originalSetTimeout(function() { pending.delete(id); fn(); }, ms);
                pending.add(id);
                return id;
            };
            globalThis.clearTimeout = function(id) { pending.delete(id); return originalClearTimeout(id); };
            function gitResult(args) {
                var i = args.indexOf('worktree');
                if (i >= 0 && args[i + 1] === 'remove') {
                    removes++;
                    now += test.cleanupMs || 0;
                    if (test.cleanupFailure) return {code:128, error:true, stdout:'', stderr:'removal denied'};
                }
                return {code:0, error:false, stdout:'', stderr:''};
            }
            function waitResult() {
                var result = {code:test.code, signal:null};
                if (test.treeFailure) result.treeCleanupError = 'tree close denied';
                return result;
            }
            native.execv = async function(argv) {
                if (argv[0] === 'git') return gitResult(argv.slice(1));
                now += test.bodyMs || 0;
                var result = {code:test.code, stdout:'captured prefix\n', stderr:'',
                    error:test.code !== 0 || !!test.truncated || !!test.treeFailure,
                    truncated:!!test.truncated};
                if (test.treeFailure) result.treeCleanupError = 'tree close denied';
                return result;
            };
            native.spawn = async function(command, args) {
                if (command === 'git') {
                    var git = gitResult(args);
                    var gitOutput = git.stderr;
                    return {
                        stdout:{read:async function() { return {done:true}; }},
                        stderr:{read:async function() {
                            if (gitOutput) { var value = gitOutput; gitOutput = ''; return {done:false, value:value}; }
                            return {done:true};
                        }},
                        wait:async function() { return {code:git.code, signal:null}; }
                    };
                }
                now += test.bodyMs || 0;
                return {
                    stdout:{read:async function() {
                        if (sent) return {done:true, truncated:!!test.truncated};
                        sent = true;
                        if (!test.cleanupReap) {
                            await outputGate;
                            await Promise.resolve();
                        }
                        return {done:false, value:'captured prefix\n'};
                    }},
                    stderr:{read:async function() { return {done:true, truncated:!!test.truncated}; }},
                    wait:async function() {
                        waits++;
                        if (test.cleanupReap) await waitGate;
                        else releaseOutput();
                        return waitResult();
                    },
                    kill:async function() { kills++; releaseWait(); }
                };
            };
            try {
                var result = await p[method]('split/deadline', {
                    dir:p.runtime.dir, verifyCommand:'verify-command', verifyTimeoutMs:1000,
                    outputFn:test.callback ? function() { throw new Error('observer failed'); } : null
                });
                reports.push({method:method, test:test, result:result,
                    waits:waits, kills:kills, removes:removes, pending:pending.size,
                    hasExitCode:Object.prototype.hasOwnProperty.call(result, 'exitCode')});
            } finally {
                releaseOutput(); releaseWait();
                native.spawn = originalSpawn; native.execv = originalExecv; Date.now = originalNow;
                globalThis.setTimeout = originalSetTimeout; globalThis.clearTimeout = originalClearTimeout;
            }
        }
    }
    return JSON.stringify(reports);
})()
`

// Removal is held by a promise gate after the child's result is complete.
// A later cancellation does not reopen that verdict, while failed cleanup
// keeps its independent failure and cancellation facts.
func TestVerifySplitAsync_LateCleanupCancellationPreservesCompletedVerdict(t *testing.T) {
	skipSlow(t)
	eval := prsplittest.NewChunkEngine(t, nil, "00_core", "06_verification")
	raw, err := eval(verifyLateCleanupCancellationScenario)
	if err != nil {
		t.Fatal(err)
	}
	var reports []struct {
		FailRemoval           bool
		Result                treeCleanupResult
		Waits, Kills, Removes int
	}
	if err := json.Unmarshal([]byte(raw.(string)), &reports); err != nil {
		t.Fatal(err)
	}
	if len(reports) != 2 {
		t.Fatalf("got %d observations, want two", len(reports))
	}
	for _, got := range reports {
		r := got.Result
		if r.Passed == got.FailRemoval || r.ExitCode != 0 || r.ExecFailed != got.FailRemoval ||
			r.CleanupFailed != got.FailRemoval || r.Cancelled != got.FailRemoval || r.WorktreeRetained ||
			r.TimedOut || r.Truncated || r.Output != "completed output\n" ||
			got.Waits != 1 || got.Kills != 0 || got.Removes != 1 {
			t.Fatalf("late cancellation changed completed command or cleanup facts: %+v", got)
		}
		if got.FailRemoval {
			if r.Error == nil || !strings.Contains(*r.Error, "removal denied") || r.WorktreeDir == "" {
				t.Fatalf("failed cleanup lost its diagnostic/path: %+v", got)
			}
		} else if r.Error != nil {
			t.Fatalf("completed verification acquired an error: %+v", got)
		}
	}
}

const verifyLateCleanupCancellationScenario = `(async function() {
    var p = prSplit, native = p._modules.exec, originalSpawn = native.spawn, reports = [];
    for (var failRemoval of [false, true]) {
        var cancelled = false, waits = 0, kills = 0, removes = 0;
        var releaseRemoval, removalStarted;
        var removalGate = new Promise(function(resolve) { releaseRemoval = resolve; });
        var removalEntered = new Promise(function(resolve) { removalStarted = resolve; });
        native.spawn = async function(command, args) {
            var code = 0, stdout = '', stderr = '';
            if (command === 'git') {
                var i = args.indexOf('worktree');
                if (i >= 0 && args[i + 1] === 'remove') {
                    removes++;
                    removalStarted();
                    await removalGate;
                    if (failRemoval) { code = 128; stderr = 'removal denied'; }
                }
            } else stdout = 'completed output\n';
            return {
                stdout:{read:async function() {
                    if (stdout) { var value = stdout; stdout = ''; return {done:false, value:value}; }
                    return {done:true};
                }},
                stderr:{read:async function() {
                    if (stderr) { var value = stderr; stderr = ''; return {done:false, value:value}; }
                    return {done:true};
                }},
                wait:async function() { if (command !== 'git') waits++; return {code:code, signal:null}; },
                kill:async function() { kills++; }
            };
        };
        try {
            var verification = p.verifySplitAsync('split/completed', {
                dir:p.runtime.dir, verifyCommand:'verify-command', verifyTimeoutMs:600000,
                isCancelled:function() { return cancelled; }
            });
            // The child and both streams have completed. Cancellation begins
            // only after the actual worktree-removal call is awaiting transport.
            await removalEntered;
            cancelled = true;
            releaseRemoval();
            var result = await verification;
            reports.push({failRemoval:failRemoval, result:result, waits:waits, kills:kills, removes:removes});
        } finally {
            releaseRemoval(); native.spawn = originalSpawn;
        }
    }
    return JSON.stringify(reports);
})()
`
