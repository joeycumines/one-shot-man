package command

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// The real verifier, collector, and cleanup path run unchanged; only native
// process transport is replaced to establish the ordering of status and output.
func TestVerifySplitAsync_ObserverFailurePreservesObservedStatus(t *testing.T) {
	skipSlow(t)
	eval := prsplittest.NewChunkEngine(t, nil, "00_core", "06_verification")
	for _, tc := range []struct {
		name      string
		mode      string
		code      int
		truncated bool
	}{
		{name: "already_reaped_zero", mode: "already_reaped", code: 0},
		{name: "already_reaped_nonzero", mode: "already_reaped", code: 7},
		{name: "cleanup_reap_zero", mode: "cleanup_reap", code: 0},
		{name: "cleanup_reap_nonzero", mode: "cleanup_reap", code: 7},
		{name: "already_reaped_incomplete_capture", mode: "already_reaped", code: 0, truncated: true},
		{name: "rejected_wait_remains_unknown", mode: "wait_rejected", code: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := eval(fmt.Sprintf("globalThis.__observerCase = {mode:%q, code:%d, truncated:%t};\n%s",
				tc.mode, tc.code, tc.truncated, verifyObserverFailureScenario))
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Result struct {
					Passed           bool
					ExitCode         int
					ExecFailed       bool
					Truncated        bool
					CleanupFailed    bool
					WorktreeRetained bool
					Error            string
					Output           string
				}
				HasExitCode           bool
				Waits, Kills, Removes int
			}
			if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
				t.Fatal(err)
			}
			if !got.HasExitCode || got.Result.ExitCode != tc.code {
				t.Fatalf("observed exit status changed: %s; want code %d", raw, tc.code)
			}
			if got.Result.Passed || !got.Result.ExecFailed || !strings.Contains(got.Result.Error, "observer failed") ||
				got.Result.Output != "captured prefix\n" || got.Result.Truncated != tc.truncated {
				t.Fatalf("independent observer/capture facts changed: %s", raw)
			}
			wantWaits, wantKills, wantRemoves := 1, 0, 1
			if tc.mode != "already_reaped" {
				wantWaits, wantKills = 2, 1
			}
			unknown := tc.mode == "wait_rejected"
			if unknown {
				wantRemoves = 0
			}
			if got.Waits != wantWaits || got.Kills != wantKills || got.Removes != wantRemoves ||
				got.Result.CleanupFailed != unknown || got.Result.WorktreeRetained != unknown {
				t.Fatalf("status propagation changed cleanup ownership: %s", raw)
			}
		})
	}
}

const verifyObserverFailureScenario = `(async function() {
    var test = __observerCase, p = prSplit, native = p._modules.exec;
    var originalSpawn = native.spawn, releaseOutput, releaseWait;
    var outputGate = new Promise(function(resolve) { releaseOutput = resolve; });
    var waitGate = new Promise(function(resolve) { releaseWait = resolve; });
    var waits = 0, kills = 0, removes = 0, sent = false;
    native.spawn = async function(command, args) {
        if (command === 'git') {
            var index = args.indexOf('worktree');
            if (index >= 0 && args[index + 1] === 'remove') removes++;
            return {
                stdout: {read: async function() { return {done:true}; }},
                stderr: {read: async function() { return {done:true}; }},
                wait: async function() { return {code:0, signal:null}; }
            };
        }
        return {
            stdout: {read: async function() {
                if (sent) return {done:true};
                sent = true;
                if (test.mode !== 'cleanup_reap') {
                    await outputGate;
                    // The wait observer records its fulfilled/rejected result
                    // before this stream delivers the throwing callback.
                    await Promise.resolve();
                }
                return {done:false, value:'captured prefix\n'};
            }},
            stderr: {read: async function() {
                return {done:true, truncated:test.truncated};
            }},
            wait: async function() {
                waits++;
                if (test.mode === 'cleanup_reap') await waitGate;
                else releaseOutput();
                if (test.mode === 'wait_rejected') throw new Error('exit status unavailable');
                return {code:test.code, signal:null};
            },
            kill: async function() { kills++; releaseWait(); }
        };
    };
    try {
        var result = await p.verifySplitAsync('split/observer', {
            dir:p.runtime.dir, verifyCommand:'verify-command', verifyTimeoutMs:600000,
            outputFn:function() { throw new Error('observer failed'); }
        });
        return JSON.stringify({result:result, waits:waits, kills:kills, removes:removes,
            hasExitCode:Object.prototype.hasOwnProperty.call(result, 'exitCode')});
    } finally {
        releaseOutput(); releaseWait(); native.spawn = originalSpawn;
    }
})()
`
