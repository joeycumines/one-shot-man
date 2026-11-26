package command

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

func TestResumeValidation_CleanupFailurePrecedesGitProbes(t *testing.T) {
	skipSlow(t)
	eval := prsplittest.NewChunkEngine(t, nil, "00_core", "10a_pipeline_config", "10d_pipeline_orchestrator")
	for _, field := range []string{"cleanupFailed", "treeCleanupError", "worktreeRetained"} {
		for _, source := range []string{"outcome_without_rows", "row_after_stale_prefix", "row_beyond_plan", "outcome_with_hidden_row"} {
			t.Run(field+"/"+source, func(t *testing.T) {
				raw, err := eval(`(async function() {
                    var calls = 0;
                    prSplit._gitExecAsync = async function() { calls++; throw new Error('unexpected Git probe'); };
                    var field = ` + strconv.Quote(field) + `, source = ` + strconv.Quote(source) + `;
                    var owner = {name:'split/2', sha:'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb',
                        code:0, exitCode:0, truncated:false, worktreeDir:'/original/retained-worktree',
                        error:'recorded cleanup diagnostic'};
                    owner[field] = field === 'treeCleanupError' ? 'original tree failure' : true;
                    var rows = source === 'outcome_without_rows' || source === 'outcome_with_hidden_row' ? [] : [
                        {name:'split/1', sha:'stale'}, owner
                    ];
                    var plan = {baseBranch:'main', splits:[{name:'split/1'}]};
                    if (source !== 'row_beyond_plan') plan.splits.push({name:'split/2'});
                    var outcome = source === 'outcome_without_rows' ? owner :
                        source === 'outcome_with_hidden_row' ? {error:null, results:[owner]} : null;
                    var result = await prSplit._validateResumeResults(plan, rows, '.', outcome);
                    return JSON.stringify({result:result, calls:calls, rows:rows.length});
                })()`)
				if err != nil {
					t.Fatal(err)
				}
				var got struct {
					Calls  int `json:"calls"`
					Rows   int `json:"rows"`
					Result struct {
						Error     string           `json:"error"`
						Results   []map[string]any `json:"results"`
						Execution map[string]any   `json:"execution"`
					} `json:"result"`
				}
				if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
					t.Fatal(err)
				}
				if got.Calls != 0 || len(got.Result.Results) != got.Rows || !strings.Contains(got.Result.Error, "/original/retained-worktree") {
					t.Fatalf("resume discarded an owner or probed Git before rejecting it: %s", raw)
				}
				owner := got.Result.Execution
				if owner["worktreeDir"] != "/original/retained-worktree" || owner["code"] != float64(0) || owner["exitCode"] != float64(0) || owner["truncated"] != false || owner["error"] != "recorded cleanup diagnostic" {
					t.Fatalf("resume changed the original execution facts: %s", raw)
				}
				if field == "treeCleanupError" {
					if owner[field] != "original tree failure" {
						t.Fatalf("tree diagnostic was lost: %s", raw)
					}
				} else if owner[field] != true {
					t.Fatalf("cleanup metadata was lost: %s", raw)
				}
			})
		}
	}
}

func TestResumeValidation_ProbeFailureDoesNotAuthorizeExecution(t *testing.T) {
	skipSlow(t)
	eval := prsplittest.NewChunkEngine(t, nil, "00_core", "10a_pipeline_config", "10d_pipeline_orchestrator")
	for _, phase := range []string{"rev-parse", "merge-base"} {
		for _, mode := range []string{"tree", "truncated", "exec-error", "rejected-tree"} {
			t.Run(phase+"/"+mode, func(t *testing.T) {
				raw, err := eval(`(async function() {
                    var calls = [], phase = ` + strconv.Quote(phase) + `, mode = ` + strconv.Quote(mode) + `;
                    prSplit._gitExecAsync = async function(dir, args) {
                        calls.push(args[0]);
                        var result = {code:0, stdout:'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', stderr:'', error:false, truncated:false};
                        if (args[0] === phase) {
                            result.error = true;
                            result.message = 'probe failed';
                            if (mode === 'tree' || mode === 'rejected-tree') result.treeCleanupError = 'probe tree failure';
                            if (mode === 'truncated') result.truncated = true;
                            if (mode === 'rejected-tree') throw Object.assign(new Error('probe failed'), result);
                        }
                        return result;
                    };
                    var rows = [{name:'split/1', sha:'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'}];
                    var result = await prSplit._validateResumeResults({baseBranch:'main', splits:[{name:'split/1'}]}, rows, '.');
                    return JSON.stringify({result:result, calls:calls});
                })()`)
				if err != nil {
					t.Fatal(err)
				}
				var got struct {
					Calls  []string `json:"calls"`
					Result struct {
						Error     string           `json:"error"`
						Results   []map[string]any `json:"results"`
						Execution map[string]any   `json:"execution"`
					} `json:"result"`
				}
				if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
					t.Fatal(err)
				}
				wantCalls := 1
				if phase == "merge-base" {
					wantCalls = 2
				}
				if got.Result.Error == "" || len(got.Calls) != wantCalls || len(got.Result.Results) != 1 || got.Result.Execution["code"] != float64(0) {
					t.Fatalf("probe failure was reduced to an executable stale prefix: %s", raw)
				}
				if strings.Contains(mode, "tree") && got.Result.Execution["treeCleanupError"] != "probe tree failure" {
					t.Fatalf("probe teardown error was lost: %s", raw)
				}
				if mode == "truncated" && got.Result.Execution["truncated"] != true {
					t.Fatalf("probe capture failure was lost: %s", raw)
				}
			})
		}
	}
}

func TestExecuteSplit_UnresolvedCompletedOutcomeBlocksWithoutRows(t *testing.T) {
	skipSlow(t)
	eval := prsplittest.NewChunkEngine(t, map[string]any{"dir": t.TempDir()}, "00_core", "04_validation", "05_execution")
	for _, method := range []string{"executeSplit", "executeSplitAsync"} {
		t.Run(method, func(t *testing.T) {
			raw, err := eval(`(async function() {
                var calls = 0;
                prSplit._modules.exec.execv = async function() { calls++; throw new Error('unexpected execution'); };
                prSplit._gitExecAsync = async function() { calls++; throw new Error('unexpected execution'); };
                var previous = {error:'setup failed', results:[], cleanupFailed:true, worktreeRetained:true,
                    worktreeDir:'/original/setup-worktree', treeCleanupError:'original setup tree failure', code:0, truncated:false};
                var plan = {dir:prSplit.runtime.dir, baseBranch:'main', sourceBranch:'feature',
                    fileStatuses:{'file.txt':'M'}, splits:[{name:'split/1', files:['file.txt']}]};
                var result = await prSplit[` + strconv.Quote(method) + `](plan, {completedOutcome:previous});
                return JSON.stringify({result:result, calls:calls, previous:previous});
            })()`)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Calls    int            `json:"calls"`
				Result   map[string]any `json:"result"`
				Previous map[string]any `json:"previous"`
			}
			if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
				t.Fatal(err)
			}
			if got.Calls != 0 || !strings.Contains(got.Result["error"].(string), "/original/setup-worktree") || got.Previous["error"] != "setup failed" {
				t.Fatalf("previous ownership was discarded or its caller-owned result mutated: %s", raw)
			}
			for _, key := range []string{"worktreeDir", "treeCleanupError", "cleanupFailed", "worktreeRetained", "code", "truncated"} {
				if got.Result[key] != got.Previous[key] {
					t.Fatalf("blocked execution changed %s: %s", key, raw)
				}
			}
		})
	}
}

func TestAutomatedResume_PersistsSetupFailureWithoutBranchRows(t *testing.T) {
	skipSlow(t)
	dir := t.TempDir()
	planPath := filepath.Join(dir, ".pr-split-plan.json")
	eval := prsplittest.NewChunkEngine(t, map[string]any{"dir": dir},
		"00_core", "03_planning", "10a_pipeline_config", "10d_pipeline_orchestrator")
	raw, err := eval(`(async function() {
        var planPath = ` + strconv.Quote(planPath) + `;
        var state = prSplit._state;
        state.planCache = {dir:prSplit.runtime.dir, baseBranch:'main', sourceBranch:'feature',
            fileStatuses:{'file.txt':'M'}, splits:[{name:'split/1', files:['file.txt']}]};
        state.executionResultCache = [];
        state.executionOutcomeCache = {error:'worktree add tree teardown failed', results:[],
            cleanupFailed:true, worktreeRetained:true, worktreeDir:'/original/setup-worktree',
            treeCleanupError:'original setup tree failure', code:0, truncated:false};
        var saved = await prSplit.savePlan(planPath, 'error');
        if (saved.error) throw new Error(saved.error);
        state.planCache = null;
        state.executionResultCache = [];
        state.executionOutcomeCache = null;
        var calls = 0;
        prSplit._gitExecAsync = async function() { calls++; throw new Error('unexpected Git access'); };
        prSplit.AgentCodeExecutor = function() { calls++; throw new Error('unexpected agent startup'); };
        var result = await prSplit.automatedSplit({dir:prSplit.runtime.dir, resumeFromPlan:true,
            resumePlanPath:planPath, disableTUI:true});
        var reloaded = await prSplit.loadPlan(planPath);
        if (reloaded.error) throw new Error(reloaded.error);
        var persisted = state.executionOutcomeCache;
        var rows = state.executionResultCache;
        // A deliberate replacement checkpoint can clear the diagnostic after
        // its owner has resolved cleanup. Loading it must not retain stale flags.
        state.executionOutcomeCache = null;
        state.executionResultCache = [];
        var clearedSave = await prSplit.savePlan(planPath, 'cleanup resolved');
        if (clearedSave.error) throw new Error(clearedSave.error);
        state.executionOutcomeCache = persisted;
        var clearedLoad = await prSplit.loadPlan(planPath);
        if (clearedLoad.error) throw new Error(clearedLoad.error);
        var cleared = await prSplit._validateResumeResults(state.planCache, state.executionResultCache,
            prSplit.runtime.dir, state.executionOutcomeCache);
        return JSON.stringify({result:result, calls:calls, persisted:persisted,
            rows:rows, cleared:cleared, clearedOutcome:state.executionOutcomeCache});
    })()`)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Calls     int              `json:"calls"`
		Persisted map[string]any   `json:"persisted"`
		Rows      []map[string]any `json:"rows"`
		Cleared   struct {
			Error string `json:"error"`
		} `json:"cleared"`
		ClearedOutcome any `json:"clearedOutcome"`
		Result         struct {
			Error  string `json:"error"`
			Report struct {
				Execution map[string]any `json:"execution"`
			} `json:"report"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
		t.Fatal(err)
	}
	if got.Calls != 0 || len(got.Rows) != 0 || !strings.Contains(got.Result.Error, "/original/setup-worktree") {
		t.Fatalf("setup failure permitted a new execution: %s", raw)
	}
	for _, outcome := range []map[string]any{got.Persisted, got.Result.Report.Execution} {
		if outcome["worktreeDir"] != "/original/setup-worktree" || outcome["treeCleanupError"] != "original setup tree failure" || outcome["cleanupFailed"] != true || outcome["worktreeRetained"] != true || outcome["code"] != float64(0) || outcome["truncated"] != false {
			t.Fatalf("setup failure lost its facts across save/load or report: %s", raw)
		}
	}
	if got.Cleared.Error != "" || got.ClearedOutcome != nil {
		t.Fatalf("explicitly cleared checkpoint retained stale ownership: %s", raw)
	}
}

func TestBuildReport_PreservesExecutionCleanupWithoutBranchRows(t *testing.T) {
	skipSlow(t)
	eval := prsplittest.NewTUIEngine(t)
	raw, err := eval(`(function() {
        prSplit._state.executionResultCache = [];
        prSplit._state.executionOutcomeCache = {error:'setup teardown failed', results:[],
            cleanupFailed:true, worktreeRetained:true, worktreeDir:'/original/setup-worktree',
            treeCleanupError:'original setup tree failure', code:0, truncated:false};
        return JSON.stringify(prSplit._buildReport());
    })()`)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Execution map[string]any `json:"execution"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &report); err != nil {
		t.Fatal(err)
	}
	outcome := report.Execution
	if outcome["worktreeDir"] != "/original/setup-worktree" || outcome["treeCleanupError"] != "original setup tree failure" || outcome["cleanupFailed"] != true || outcome["worktreeRetained"] != true || outcome["code"] != float64(0) || outcome["truncated"] != false {
		t.Fatalf("exported report lost execution facts without branch rows: %s", raw)
	}
}
