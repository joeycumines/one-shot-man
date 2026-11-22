package command

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

func TestCommandResult_IndependentFailureFacts(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the JavaScript engine")
	}
	eval := prsplittest.NewChunkEngine(t, nil, "00_core")
	for _, tc := range []struct {
		name, result, accepted string
		failed                 bool
	}{
		{"success", `{code:0,error:false,truncated:false}`, `[0]`, false},
		{"query_false", `{code:1,error:true}`, `[0,1]`, false},
		{"failure_status", `{code:7,error:true}`, `[0]`, true},
		{"execution_error", `{code:0,error:true}`, `[0]`, true},
		{"truncation_zero", `{code:0,truncated:true}`, `[0,1]`, true},
		{"truncation_one", `{code:1,error:true,truncated:true}`, `[0,1]`, true},
		{"tree_zero", `{code:0,treeCleanupError:'denied'}`, `[0,1]`, true},
		{"tree_one", `{code:1,error:true,treeCleanupError:'denied'}`, `[0,1]`, true},
		{"unknown_verdict", `{code:0,execFailed:true}`, `[0]`, true},
		{"timeout_at_accepted_status", `{code:1,error:true,timedOut:true}`, `[0,1]`, true},
		{"cancellation_at_accepted_status", `{code:0,cancelled:true}`, `[0,1]`, true},
		{"missing", `null`, `[0]`, true},
		{"unrecorded_status", `{error:false}`, `[0]`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := eval(`prSplit._commandFailed(` + tc.result + `,` + tc.accepted + `)`)
			if err != nil || got != tc.failed {
				t.Fatalf("failure predicate = %v, %v; want %v", got, err, tc.failed)
			}
		})
	}
}

func TestStaging_ReportsFailuresToWorktreeOwner(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the JavaScript engine")
	}
	eval := prsplittest.NewChunkEngine(t, map[string]any{"dir": t.TempDir()}, "00_core")
	for _, phase := range []string{"status", "add"} {
		for _, mode := range []string{"tree", "truncated", "exec-error", "rejected-tree"} {
			t.Run(phase+"/"+mode, func(t *testing.T) {
				raw, err := eval(`(async function() {
                    var phase = ` + strconv.Quote(phase) + `, mode = ` + strconv.Quote(mode) + `;
                    var calls = [], failure;
                    async function owner(dir, args) {
                        calls.push(args[0]);
                        var result = {code:0,error:false,stdout:' M source.go\n',stderr:''};
                        if (args[0] === phase) {
                            result.error = true;
                            result.message = 'staging ' + phase + ' ' + mode;
                            if (mode === 'tree' || mode === 'rejected-tree') result.treeCleanupError = result.message;
                            if (mode === 'truncated') result.truncated = true;
                            if (mode === 'rejected-tree') {
                                var error = new Error(result.message);
                                error.treeCleanupError = result.treeCleanupError;
                                throw error;
                            }
                        }
                        return result;
                    }
                    try { await prSplit._gitAddChangedFilesAsync(prSplit.runtime.dir, owner); }
                    catch (error) { failure = {message:error.message,treeCleanupError:error.treeCleanupError,truncated:error.truncated}; }
                    return JSON.stringify({calls:calls,failure:failure});
                })()`)
				if err != nil {
					t.Fatal(err)
				}
				var got struct {
					Calls   []string `json:"calls"`
					Failure struct {
						Message   string `json:"message"`
						TreeError string `json:"treeCleanupError"`
						Truncated bool   `json:"truncated"`
					} `json:"failure"`
				}
				if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(got.Failure.Message, "staging "+phase+" "+mode) || got.Calls[len(got.Calls)-1] != phase {
					t.Fatalf("owner did not receive the failure before another command: %s", raw)
				}
				if strings.Contains(mode, "tree") != (got.Failure.TreeError != "") || (mode == "truncated") != got.Failure.Truncated {
					t.Fatalf("structural failure metadata was lost: %s", raw)
				}
			})
		}
	}
}
