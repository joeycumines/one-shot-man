package command

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// Exercise the real Git transports and streaming collector. Only the native
// exec boundary is replaced, so exit-zero teardown errors keep their actual
// shape all the way into execution and worktree ownership.
func TestExecuteSplit_CommandFailureKeepsWorktreeOwnership(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the JavaScript engine")
	}
	phases := []string{"check-ignore", "branch-probe", "branch-delete", "worktree-add", "branch-checkout", "rm", "file-checkout", "add", "commit", "sha", "worktree-remove"}
	for _, method := range []string{"executeSplit", "executeSplitAsync"} {
		t.Run(method, func(t *testing.T) {
			eval := prsplittest.NewChunkEngine(t, map[string]any{"dir": t.TempDir()}, "00_core", "04_validation", "05_execution")
			_, err := eval(`
                globalThis.executionProbe = async function(method, phase, mode) {
                    var calls = [], failedAt = -1, worktree = null;
                    function classify(args) {
                        if (args[0] === 'check-ignore') return 'check-ignore';
                        if (args[0] === 'rev-parse') return args[1] === 'HEAD' ? 'sha' : 'branch-probe';
                        if (args[0] === 'branch') return 'branch-delete';
                        if (args[0] === 'worktree') return 'worktree-' + args[1];
                        if (args[0] === 'checkout') return args[1] === '-b' ? 'branch-checkout' : 'file-checkout';
                        return args[0];
                    }
                    function result(rawArgs) {
                        var args = rawArgs[0] === '-C' ? rawArgs.slice(2) : rawArgs;
                        var current = classify(args);
                        calls.push(current);
                        if (current === 'worktree-add') worktree = args[3];
                        var code = current === 'check-ignore' ? 1 : 0;
                        var r = {code: code, stdout: current === 'sha' ? 'abc123\n' : '', stderr: '', error: code !== 0, truncated: false, message: ''};
                        if (current === phase) {
                            failedAt = calls.length - 1;
                            r.message = 'injected ' + mode + ' at ' + phase;
                            if (mode === 'tree' || mode === 'tree-nonzero' || mode === 'rejected-tree') r.treeCleanupError = r.message;
                            if (mode === 'tree-nonzero' || mode === 'exit') r.code = 7;
                            if (mode === 'exec-error') r.code = 0;
                            if (mode === 'truncated') r.truncated = true;
                            r.error = true;
                            if (mode === 'rejected-tree' || mode === 'rejected') {
                                var e = new Error(r.message);
                                if (r.treeCleanupError) e.treeCleanupError = r.treeCleanupError;
                                throw e;
                            }
                        }
                        if (mode === 'primary-and-removal' && (current === 'commit' || current === 'worktree-remove')) {
                            r.code = 7;
                            r.error = true;
                            r.message = current === 'commit' ? 'primary commit failure' : 'secondary removal failure';
                            r.stderr = r.message;
                        }
                        return r;
                    }
                    prSplit._modules.exec.execv = async function(argv) { return result(argv.slice(1)); };
                    prSplit._modules.exec.spawn = async function(command, args) {
                        var r = result(args), sent = false, stderrSent = false;
                        return {
                            stdout: {read: async function() {
                                if (!sent && r.stdout) { sent = true; return {done: false, value: r.stdout}; }
                                return {done: true, truncated: r.truncated};
                            }},
                            stderr: {read: async function() {
                                if (!stderrSent && r.stderr) { stderrSent = true; return {done: false, value: r.stderr}; }
                                return {done: true};
                            }},
                            wait: async function() {
                                var status = {code: r.code, signal: null};
                                if (r.treeCleanupError) status.treeCleanupError = r.treeCleanupError;
                                return status;
                            }
                        };
                    };
                    var plan = {dir: prSplit.runtime.dir, baseBranch: 'main', sourceBranch: 'feature',
                        fileStatuses: {'gone.txt': 'D', 'change.txt': 'M'},
                        splits: [{name: 'split/1', files: ['gone.txt', 'change.txt']}]};
                    var options = mode === 'callback' ? {progressFn: function() { throw new Error('observer failed'); }} : {};
                    var outcome = await prSplit[method](plan, options);
                    return JSON.stringify({outcome: outcome, calls: calls, failedAt: failedAt, worktree: worktree});
                };
            `)
			if err != nil {
				t.Fatal(err)
			}
			for _, mode := range []string{"tree", "tree-nonzero", "rejected-tree", "exit", "truncated", "rejected", "exec-error"} {
				if mode == "exec-error" && method == "executeSplitAsync" {
					continue // wait() reports status; execv also reports post-EOF pipe-close errors.
				}
				for index, phase := range phases {
					t.Run(mode+"/"+phase, func(t *testing.T) {
						raw, err := eval(`executionProbe(` + strconv.Quote(method) + `,` + strconv.Quote(phase) + `,` + strconv.Quote(mode) + `)`)
						if err != nil {
							t.Fatal(err)
						}
						var got struct {
							Outcome struct {
								Error            string `json:"error"`
								TreeCleanupError string `json:"treeCleanupError"`
								CleanupFailed    bool   `json:"cleanupFailed"`
								Retained         bool   `json:"worktreeRetained"`
								Dir              string `json:"worktreeDir"`
							} `json:"outcome"`
							Calls    []string `json:"calls"`
							Worktree string   `json:"worktree"`
							FailedAt int      `json:"failedAt"`
						}
						if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
							t.Fatal(err)
						}
						if got.Outcome.Error == "" || got.FailedAt != index {
							t.Fatalf("failure was not reported at the expected boundary: %s", raw)
						}
						tree := strings.Contains(mode, "tree")
						owns := index >= 3
						removing := phase == "worktree-remove"
						wantCalls := index + 1
						if owns && !tree && !removing {
							wantCalls++ // Ordinary failure still releases the owned worktree.
						}
						if len(got.Calls) != wantCalls || (wantCalls > index+1 && got.Calls[len(got.Calls)-1] != "worktree-remove") {
							t.Fatalf("unexpected mutation after failure: %s", raw)
						}
						if (got.Outcome.TreeCleanupError != "") != tree || got.Outcome.Retained != (tree && owns && !removing) {
							t.Fatalf("cleanup fact was lost or invented: %s", raw)
						}
						if (tree && owns) || removing {
							if !got.Outcome.CleanupFailed || got.Outcome.Dir != got.Worktree || !strings.Contains(got.Outcome.Error, got.Worktree) {
								t.Fatalf("cleanup must preserve original path and diagnostic: %s", raw)
							}
						}
						if removing && !strings.Contains(got.Outcome.Error, "removal was attempted") {
							t.Fatalf("failed removal must be reported as attempted: %s", raw)
						}
					})
				}
			}
			for _, mode := range []string{"clean", "callback", "primary-and-removal"} {
				t.Run(mode, func(t *testing.T) {
					raw, err := eval(`executionProbe(` + strconv.Quote(method) + `,"none",` + strconv.Quote(mode) + `)`)
					if err != nil {
						t.Fatal(err)
					}
					var got map[string]any
					if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
						t.Fatal(err)
					}
					outcome := got["outcome"].(map[string]any)
					calls := got["calls"].([]any)
					if calls[len(calls)-1] != "worktree-remove" {
						t.Fatalf("owned worktree was abandoned: %s", raw)
					}
					switch mode {
					case "clean":
						if outcome["error"] != nil || len(calls) != len(phases) {
							t.Fatalf("clean execution regressed: %s", raw)
						}
					case "callback":
						if !strings.Contains(outcome["error"].(string), "observer failed") {
							t.Fatalf("callback failure was lost: %s", raw)
						}
					case "primary-and-removal":
						text := outcome["error"].(string)
						if !strings.Contains(text, "primary commit failure") || !strings.Contains(text, "secondary removal failure") || outcome["cleanupFailed"] != true {
							t.Fatalf("cleanup replaced primary failure: %s", raw)
						}
					}
				})
			}
		})
	}
}
