package command

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// The mocks replace the native exec result, not the public cleanup helper:
// these controls exercise the production Git, verifier and rollback boundaries.
func assertOrdinaryCleanupFailure(t *testing.T, r treeCleanupResult, reason string) {
	t.Helper()
	if r.Passed || !r.ExecFailed || !r.CleanupFailed || r.WorktreeRetained || r.TreeCleanupError != "" {
		t.Errorf("attempted removal must be a non-forgivable failure without invented retention/tree status: %+v", r)
	}
	if r.WorktreeDir == "" || r.Error == nil || !strings.Contains(*r.Error, r.WorktreeDir) || !strings.Contains(*r.Error, reason) {
		t.Errorf("removal failure lost its reason or actionable path: %+v, reason=%q", r, reason)
	}
}

func TestVerifyWorktree_FinalRemovalOutcome(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	for _, tc := range []struct {
		name, response, reason string
		clean                  bool
	}{
		{name: "nonzero", response: `return {code:128,error:true,stderr:'fatal: removal denied'};`, reason: "removal denied"},
		{name: "exit_zero_exec_error", response: `return {code:0,error:true,message:'close after EOF failed'};`, reason: "close after EOF failed"},
		{name: "truncated", response: `return {code:0,error:false,truncated:true,message:'capture incomplete'};`, reason: "capture incomplete"},
		{name: "empty_diagnostic", response: `return {code:17,error:true,stderr:'',message:''};`, reason: "17"},
		{name: "reject_error", response: `throw new Error('removal rejected');`, reason: "removal rejected"},
		{name: "reject_primitive", response: `throw 'primitive removal rejection';`, reason: "primitive removal rejection"},
		{name: "success", response: `return _gitOk();`, clean: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evalJS := treeCleanupEval(t)
			raw, err := evalJS(`(async function(){
				_gitResponses['worktree remove --force']=function(){` + tc.response + `};
				var result, rejection;
				try{result=await prSplit.cleanupVerifyWorktree('/tmp/repo','/tmp/owned-worktree');}
				catch(e){rejection=String(e);}
				return JSON.stringify({result:result,isUndefined:result===undefined,rejection:rejection,
					removes:__treeRemovalCount(),hasExitCode:!!result&&Object.prototype.hasOwnProperty.call(result,'exitCode')});
			})()`)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Result      treeCleanupResult `json:"result"`
				IsUndefined bool              `json:"isUndefined"`
				Rejection   string            `json:"rejection"`
				Removes     int               `json:"removes"`
				HasExitCode bool              `json:"hasExitCode"`
			}
			if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
				t.Fatal(err)
			}
			t.Logf("final removal observation: %s", raw)
			if got.Removes != 1 || got.Rejection != "" || got.HasExitCode {
				t.Errorf("cleanup must resolve a classified result without a fabricated verification status: %+v", got)
			}
			if tc.clean {
				if !got.IsUndefined {
					t.Errorf("successful public cleanup must remain undefined: %+v", got)
				}
				return
			}
			assertOrdinaryCleanupFailure(t, got.Result, tc.reason)
			if got.IsUndefined || got.Result.WorktreeDir != "/tmp/owned-worktree" {
				t.Errorf("cleanup outcome or exact path missing: %+v", got)
			}
		})
	}
}

func TestVerifySplit_FinalRemovalFailurePreservesVerdict(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	for _, method := range []string{"verifySplit", "verifySplitAsync"} {
		for _, tc := range []struct {
			name, setup, extra                 string
			code                               int
			truncated, timedOut, cancelled     bool
			clean, rejected, callback, noChild bool
		}{
			{name: "exit_zero"},
			{name: "nonzero_rejected_removal", setup: `__treeCode=7;`, code: 7, rejected: true},
			{name: "truncated", setup: `__treeCode=7;__treeTruncated=true;`, code: 7, truncated: true},
			{name: "timeout_truncated", setup: `__treeCode=143;__treeTruncated=true;__treeBodyElapsedMs=5000;`, code: 143, truncated: true, timedOut: true},
			{name: "callback_throw", setup: `__treeCode=7;__treeWaitStyle='after_kill';`, extra: `, outputFn:function(){throw new Error('output callback exploded');}`, code: 7, rejected: true, callback: true},
			{name: "spawn_throw", setup: `__treeThrow=true;`, code: -1, rejected: true, noChild: true},
			{name: "clean_control", clean: true},
			{name: "cancelled_with_capture_and_timeout", setup: `__treeCode=143;__treeTruncated=true;__treeBodyElapsedMs=5000;__treeWaitHook=function(){__treeCancelled=true;};`, extra: `, isCancelled:function(){return __treeCancelled;}`, code: 143, truncated: true, timedOut: true, cancelled: true},
		} {
			if tc.cancelled && method != "verifySplitAsync" {
				continue
			}
			t.Run(method+"/"+tc.name, func(t *testing.T) {
				response := `return _gitFail('final removal denied');`
				if tc.clean {
					response = `return _gitOk();`
				} else if tc.rejected {
					response = `throw new Error('final removal denied');`
				}
				got := observeTreeCleanup(t, method, tc.setup+`_gitResponses['worktree remove --force']=function(){`+response+`};`, tc.extra)
				r := got.Result
				if tc.clean {
					if !r.Passed || r.ExecFailed || r.CleanupFailed || r.ExitCode != 0 {
						t.Errorf("clean verification/removal changed: %+v", got)
					}
				} else {
					assertOrdinaryCleanupFailure(t, r, "final removal denied")
				}
				if r.ExitCode != tc.code || r.Truncated != tc.truncated || r.TimedOut != tc.timedOut || r.Cancelled != tc.cancelled {
					t.Errorf("primary verdict facts changed: %+v; want code=%d truncated=%v timeout=%v cancelled=%v", r, tc.code, tc.truncated, tc.timedOut, tc.cancelled)
				}
				output := "complete output\n"
				if tc.noChild {
					output = ""
				}
				if r.Output != output || got.Removes != 1 || got.Commands != 1 || got.Pending != 0 {
					t.Errorf("output, removal count, or cleanup timer ownership changed: %+v", got)
				}
				if tc.callback && (r.Error == nil || !strings.Contains(*r.Error, "output callback exploded")) {
					t.Errorf("original callback failure lost: %+v", r)
				}
				if tc.noChild && (r.Error == nil || !strings.Contains(*r.Error, "post-start attach failed")) {
					t.Errorf("original spawn failure lost: %+v", r)
				}
			})
		}
	}
}

func TestVerifyWorktree_StartupRollbackOrdinaryRemovalFailure(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	for _, style := range []string{"nonzero", "rejected", "clean_control"} {
		t.Run(style, func(t *testing.T) {
			evalJS := treeCleanupEval(t)
			raw, err := evalJS(`(async function(){
				var style='` + style + `',tm=require('osm:termmux'),original=tm.newCaptureSession,order=[];
				tm.newCaptureSession=function(){return {
					start:function(){order.push('start');throw new Error('capture startup failed');},
					close:function(){order.push('close');return Promise.resolve().then(function(){order.push('closed');});}
				};};
				_gitResponses['worktree remove --force']=function(){
					order.push('remove');
					if(style==='rejected')throw new Error('rollback removal denied');
					return style==='nonzero'?_gitFail('rollback removal denied'):_gitOk();
				};
				try{
					var r=await prSplit.startVerifySession('split/01',{dir:'/tmp/repo',verifyCommand:'make test'});
					return JSON.stringify({result:r,order:order,removes:__treeRemovalCount(),sessionIsNull:r.session===null,hasExitCode:Object.prototype.hasOwnProperty.call(r,'exitCode')});
				}finally{tm.newCaptureSession=original;}
			})()`)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Result        treeCleanupResult `json:"result"`
				Order         []string          `json:"order"`
				Removes       int               `json:"removes"`
				SessionIsNull bool              `json:"sessionIsNull"`
				HasExitCode   bool              `json:"hasExitCode"`
			}
			if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
				t.Fatal(err)
			}
			t.Logf("startup removal observation: %s", raw)
			if style == "clean_control" {
				if got.Result.CleanupFailed || got.Result.WorktreeDir != "" {
					t.Errorf("clean startup rollback changed: %+v", got)
				}
			} else {
				assertOrdinaryCleanupFailure(t, got.Result, "rollback removal denied")
			}
			if strings.Join(got.Order, ",") != "start,close,closed,remove" || got.Removes != 1 || !got.SessionIsNull || got.HasExitCode || got.Result.Error == nil || !strings.Contains(*got.Result.Error, "capture startup failed") {
				t.Errorf("startup rollback lost completion ordering, original failure or status boundary: %+v", got)
			}
		})
	}
}

func TestVerifySplits_OrdinaryCleanupFailureStopsBatch(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	for _, method := range []string{"verifySplits", "verifySplitsAsync"} {
		for _, scenario := range []string{"baseline", "branch", "failed_baseline", "cancelled_branch", "unknown_baseline_control", "preexisting_control"} {
			if scenario == "cancelled_branch" && method != "verifySplitsAsync" {
				continue
			}
			t.Run(method+"/"+scenario, func(t *testing.T) {
				evalJS := treeCleanupEval(t)
				raw, err := evalJS(`(async function(){
					var scenario='` + scenario + `',source='',isControl=scenario.indexOf('_control')>=0;
					if(scenario==='baseline'||scenario==='failed_baseline'||isControl)source='feature';
					var originalSync=_gitResponses['!sh'],originalAsync=__childFactory,commands=0,removals=0;
					function nextResult(){
						commands++;
						__treeCode=(scenario==='failed_baseline'||isControl)?7:0;
						__treeTruncated=scenario==='unknown_baseline_control'&&commands===1;
					}
					_gitResponses['!sh']=function(){nextResult();return originalSync();};
					__childFactory=function(){nextResult();return originalAsync();};
					if(scenario==='cancelled_branch')__treeWaitHook=function(){__treeCancelled=true;};
					_gitResponses['worktree remove --force']=function(){
						removals++;
						return isControl||(scenario==='failed_baseline'&&removals===1)?_gitOk():_gitFail('batch removal denied');
					};
					var r=await prSplit.` + method + `({dir:'/tmp/repo',sourceBranch:source,verifyCommand:'make test',splits:[
						{name:'split/01',files:['a.go']},{name:'split/02',files:['b.go'],dependencies:['split/01']}
					]},{verifyTimeoutMs:5000,isCancelled:function(){return __treeCancelled;}});
					return JSON.stringify({batch:r,commands:commands,removes:__treeRemovalCount()});
				})()`)
				if err != nil {
					t.Fatal(err)
				}
				var got struct {
					Batch struct {
						AllPassed bool                `json:"allPassed"`
						Results   []treeCleanupResult `json:"results"`
						Error     string              `json:"error"`
					} `json:"batch"`
					Commands int `json:"commands"`
					Removes  int `json:"removes"`
				}
				if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
					t.Fatal(err)
				}
				t.Logf("batch removal observation: %s", raw)
				if strings.HasSuffix(scenario, "_control") {
					preExisting := scenario == "preexisting_control"
					commands := 2
					if preExisting {
						commands = 3
					}
					if got.Batch.AllPassed != preExisting || len(got.Batch.Results) != 2 || got.Commands != commands || got.Removes != commands {
						t.Fatalf("ordinary baseline/dependency behavior changed: %+v", got)
					}
					for _, r := range got.Batch.Results {
						if r.Passed || r.PreExisting != preExisting || r.CleanupFailed || r.TreeCleanupError != "" {
							t.Errorf("ordinary baseline result changed: %+v", r)
						}
					}
					if !preExisting && (got.Batch.Results[0].Error == nil || !strings.Contains(*got.Batch.Results[0].Error, "baseline verdict unknown") || got.Batch.Results[1].Error == nil || !strings.Contains(*got.Batch.Results[1].Error, "dependency split/01 failed")) {
						t.Errorf("unknown baseline or dependent skip was lost: %+v", got.Batch.Results)
					}
					return
				}
				commands, code := 1, 0
				if scenario == "failed_baseline" {
					commands, code = 2, 7
				}
				if got.Batch.AllPassed || got.Commands != commands || got.Removes != commands || len(got.Batch.Results) != 1 {
					t.Fatalf("batch must stop with one visible failed row: %+v", got)
				}
				r := got.Batch.Results[0]
				assertOrdinaryCleanupFailure(t, r, "batch removal denied")
				if r.PreExisting || r.ExitCode != code || r.Baseline != (scenario == "baseline") || r.Cancelled != (scenario == "cancelled_branch") || !strings.Contains(got.Batch.Error, r.WorktreeDir) {
					t.Errorf("batch lost cleanup/primary outcome or forgave infrastructure failure: %+v", got)
				}
				wantName := "split/01"
				if scenario == "baseline" {
					wantName = "feature"
				}
				if r.Name != wantName {
					t.Errorf("visible failure row name = %q, want %q", r.Name, wantName)
				}
			})
		}
	}
}

func TestVerifySplit_SetupFallbackRemovalFailureIsNotFinalCleanup(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	for _, method := range []string{"verifySplit", "verifySplitAsync"} {
		t.Run(method, func(t *testing.T) {
			got := observeTreeCleanup(t, method, `
				_gitResponses['worktree add']=_gitFail('branch already checked out');
				_gitResponses['worktree add --detach']=_gitOk();
				var removals=0;
				_gitResponses['worktree remove --force']=function(){return ++removals===1?_gitFail('not a working tree'):_gitOk();};
			`, "")
			if !got.Result.Passed || got.Result.ExitCode != 0 || got.Result.CleanupFailed || got.Result.ExecFailed || got.Removes != 2 || got.Commands != 1 {
				t.Errorf("expected setup fallback removal must not invalidate successful detached-add verification: %+v", got)
			}
		})
	}
}

func TestVerifySplit_FinalRemovalDurationDoesNotCauseTimeout(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	for _, method := range []string{"verifySplit", "verifySplitAsync"} {
		for _, bodyMs := range []int{0, 6000} {
			t.Run(fmt.Sprintf("%s/body_%dms", method, bodyMs), func(t *testing.T) {
				got := observeTreeCleanup(t, method, fmt.Sprintf(`
					var fakeNow=Date.now(),originalSync=_gitResponses['!sh'],originalAsync=__childFactory;
					Date.now=function(){return fakeNow;};
					_gitResponses['!sh']=function(){fakeNow+=%d;return originalSync();};
					__childFactory=function(){fakeNow+=%d;return originalAsync();};
					_gitResponses['worktree remove --force']=function(){fakeNow+=10000;return _gitOk();};
				`, bodyMs, bodyMs), "")
				timedOut := bodyMs >= 5000
				if got.Result.TimedOut != timedOut || got.Result.Passed == timedOut || got.Result.ExitCode != 0 || got.Result.Output != "complete output\n" || got.Result.CleanupFailed || got.Removes != 1 {
					t.Errorf("verification timing must exclude the 10s removal while preserving the actual body deadline: body=%dms, result=%+v", bodyMs, got)
				}
				if timedOut && (got.Result.Error == nil || !strings.Contains(*got.Result.Error, "after 6000ms (limit:")) {
					t.Errorf("timeout diagnostic must report the body's actual 6000ms duration: %+v", got.Result)
				}
			})
		}
	}
}
