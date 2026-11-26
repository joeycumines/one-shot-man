package command

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// Keep the real Git transport, accepted-status predicate, and PR creation
// flow. Only the native execv boundary is replaced; no external push or PR
// operation runs in this test.
func TestCreatePRs_AcceptedQueryPreservesExecutionFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the JavaScript engine")
	}
	for _, execFailed := range []bool{false, true} {
		t.Run("exec_failed_"+strconv.FormatBool(execFailed), func(t *testing.T) {
			evalJS := prsplittest.NewChunkEngine(t, map[string]any{"dir": t.TempDir()}, "00_core", "07_prcreation")
			_, err := evalJS(`
                globalThis.acceptedQueryExecutionFailure = async function(execFailed) {
                    var ghCreates = 0, diffs = 0;
                    var reason = 'injected output close after EOF';
                    prSplit._modules.exec.execv = async function(argv) {
                        var result = {stdout:'', stderr:'', code:0, error:false, message:'', truncated:false};
                        if (argv[0] === 'gh') {
                            if (argv[1] === '--version') return result;
                            if (argv[1] === 'pr' && argv[2] === 'create') {
                                ghCreates++;
                                result.stdout = 'https://example.invalid/pull/1';
                                return result;
                            }
                        } else if (argv[0] === 'git') {
                            var args = argv[1] === '-C' ? argv.slice(3) : argv.slice(1);
                            if (args[0] === 'ls-remote') {
                                result.stdout = 'abc123\trefs/heads/main\n';
                                return result;
                            }
                            if (args[0] === 'push') return result;
                            if (args[0] === 'diff' && args[1] === '--quiet') {
                                diffs++;
                                result.code = 1;
                                result.error = true;
                                result.message = 'exit status 1';
                                if (execFailed) {
                                    result.execFailed = true;
                                    result.message += '\nclose command output pipe: ' + reason;
                                }
                                return result;
                            }
                        }
                        throw new Error('unexpected native command: ' + JSON.stringify(argv));
                    };
                    var outcome = await prSplit.createPRs({
                        dir:prSplit.runtime.dir, baseBranch:'main', sourceBranch:'feature',
                        splits:[{name:'split/1', files:['a.go'], message:'part 1'}]
                    });
                    return JSON.stringify({outcome:outcome, ghCreates:ghCreates, diffs:diffs});
                };
            `)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := evalJS(`acceptedQueryExecutionFailure(` + strconv.FormatBool(execFailed) + `)`)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Outcome struct {
					Results []struct {
						Error   string `json:"error"`
						PRURL   string `json:"prUrl"`
						Skipped bool   `json:"skipped"`
					} `json:"results"`
				} `json:"outcome"`
				GHCreates int `json:"ghCreates"`
				Diffs     int `json:"diffs"`
			}
			if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
				t.Fatal(err)
			}
			if got.Diffs != 1 || len(got.Outcome.Results) != 1 || got.Outcome.Results[0].Skipped {
				t.Fatalf("query was not evaluated once as an ordinary diff: %s", raw)
			}
			result := got.Outcome.Results[0]
			if execFailed {
				if got.GHCreates != 0 || result.PRURL != "" || !strings.Contains(result.Error, "injected output close after EOF") {
					t.Fatalf("accepted query status hid execution failure: %s", raw)
				}
			} else if got.GHCreates != 1 || result.Error != "" || result.PRURL != "https://example.invalid/pull/1" {
				t.Fatalf("ordinary query exit 1 stopped PR creation: %s", raw)
			}
		})
	}
}
