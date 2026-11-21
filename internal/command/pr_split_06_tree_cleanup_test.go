package command

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// These fixtures model the native result contract; the exec package separately
// exercises the same metadata with real ProcessState and injected tree errors.
const treeCleanupFixtureJS = `
	globalThis.__treeCode = 0;
	globalThis.__treeError = '';
	globalThis.__treeTruncated = false;
	globalThis.__treePipeError = false;
	globalThis.__treeThrow = false;
	globalThis.__treeWaitStyle = 'resolve';
	globalThis.__treeWaitHook = function() {};
	globalThis.__treeCommandCalls = 0;
	globalThis.__treeKillCalls = 0;
	globalThis.__treeWaitCalls = 0;
	globalThis.__treeCancelled = false;
	globalThis.__treeException = function(message) {
		var e = new Error(message);
		if (__treeError) e.treeCleanupError = __treeError;
		return e;
	};
	_gitResponses['!sh'] = function() {
		__treeCommandCalls++;
		if (__treeThrow) throw __treeException('post-start attach failed');
		var r = {
			code: __treeCode, stdout: 'complete output\n', stderr: '',
			error: __treeCode !== 0 || !!__treeError || __treePipeError,
			truncated: __treeTruncated,
			message: __treeError || (__treePipeError ? 'pipe close failed after EOF' : '')
		};
		if (__treeError) r.treeCleanupError = __treeError;
		return r;
	};
	// Route async git through the same mock, including functional responses.
	globalThis.prSplit._gitExecAsync = function(dir, args) {
		return globalThis.prSplit._modules.exec.execv(['git', '-C', dir].concat(args));
	};
	globalThis.__childFactory = function() {
		__treeCommandCalls++;
		if (__treeThrow) throw __treeException('post-start attach failed');
		var sent = false;
		return {
			stdout: {read: function() {
				if (!sent) { sent = true; return Promise.resolve({done: false, value: 'complete output\n'}); }
				return Promise.resolve({done: true, truncated: __treeTruncated});
			}},
			stderr: {read: function() { return Promise.resolve({done: true, truncated: __treeTruncated}); }},
			wait: function() {
				__treeWaitCalls++;
				__treeWaitHook();
				if (__treeWaitStyle.indexOf('after_kill') === 0 && __treeWaitCalls === 1) return new Promise(function() {});
				if (__treeWaitStyle === 'reject' || __treeWaitStyle === 'after_kill_reject') return Promise.reject(__treeException('wait failed'));
				var r = {code: __treeCode, signal: null};
				if (__treeError) r.treeCleanupError = __treeError;
				return Promise.resolve(r);
			},
			kill: function() { __treeKillCalls++; return Promise.resolve(); }
		};
	};
	globalThis.__treeRemovalCount = function() {
		return _gitCalls.filter(function(call) {
			var i = call.argv.indexOf('worktree');
			return i >= 0 && call.argv[i + 1] === 'remove';
		}).length;
	};
`

type treeCleanupResult struct {
	asyncVerifyResult
	Name             string `json:"name"`
	Output           string `json:"output"`
	TreeCleanupError string `json:"treeCleanupError"`
	Cancelled        bool   `json:"cancelled"`
	Baseline         bool   `json:"baseline"`
	PreExisting      bool   `json:"preExisting"`
}

type treeCleanupObservation struct {
	Result   treeCleanupResult `json:"result"`
	Removes  int               `json:"removes"`
	Kills    int               `json:"kills"`
	Waits    int               `json:"waits"`
	Commands int               `json:"commands"`
	Pending  int               `json:"pending"`
}

func treeCleanupEval(t *testing.T) func(string) (any, error) {
	t.Helper()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)
	for _, script := range []string{prsplittest.GitMockSetupJS(), asyncVerifySetupJS, treeCleanupFixtureJS} {
		if _, err := evalJS(script); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		if _, err := evalJS(asyncVerifyTeardownJS); err != nil {
			t.Logf("teardown: %v", err)
		}
	})
	return evalJS
}

func observeTreeCleanup(t *testing.T, method, setup, extraConfig string) treeCleanupObservation {
	t.Helper()
	evalJS := treeCleanupEval(t)
	if _, err := evalJS(setup); err != nil {
		t.Fatal(err)
	}
	raw, err := evalJS(`(async function() {
		var r = await globalThis.prSplit.` + method + `('split/01', {
			dir: '/tmp/repo', verifyCommand: 'make test', verifyTimeoutMs: 5000` + extraConfig + `
		});
		return JSON.stringify({result:r, removes:__treeRemovalCount(), kills:__treeKillCalls,
			waits:__treeWaitCalls, commands:__treeCommandCalls, pending:__pendingTimerCount()});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var got treeCleanupObservation
	if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	t.Logf("observation: %s", raw)
	return got
}

func assertTreeCleanupFailure(t *testing.T, got treeCleanupObservation, code, removes int, retained bool) {
	t.Helper()
	r := got.Result
	if r.Passed || !r.ExecFailed || !r.CleanupFailed || r.TreeCleanupError != "tree close denied" {
		t.Errorf("tree failure was not a non-forgivable infrastructure failure: %+v", r)
	}
	if r.ExitCode != code || got.Removes != removes || r.WorktreeRetained != retained {
		t.Errorf("(code, removes, retained) = (%d, %d, %v), want (%d, %d, %v)", r.ExitCode, got.Removes, r.WorktreeRetained, code, removes, retained)
	}
	if r.WorktreeDir == "" || r.Error == nil || !strings.Contains(*r.Error, r.WorktreeDir) || !strings.Contains(*r.Error, "tree close denied") {
		t.Errorf("cleanup reason and path must be visible in the error and fields: %+v", r)
	}
	if got.Pending != 0 {
		t.Errorf("pending bounded-await/cancellation timers = %d, want zero", got.Pending)
	}
}

func TestVerifySplit_TreeCleanupFailure(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	for _, tc := range []struct {
		name, setup, extra string
		code               int
		truncated, timeout bool
	}{
		{name: "clean_exit_zero"},
		{name: "nonzero_truncated", setup: `__treeCode = 7; __treeTruncated = true;`, code: 7, truncated: true},
		{name: "timeout_truncated", setup: `__treeCode = 124; __treeTruncated = true;`, code: 124, truncated: true, timeout: true},
		{name: "throwing_output_callback", extra: `, outputFn: function() { throw new Error('output callback exploded'); }`},
		{name: "rejected_post_start_spawn", setup: `__treeThrow = true;`, code: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := observeTreeCleanup(t, "verifySplit", `__treeError = 'tree close denied';`+tc.setup, tc.extra)
			assertTreeCleanupFailure(t, got, tc.code, 0, true)
			if got.Result.Truncated != tc.truncated || got.Result.TimedOut != tc.timeout {
				t.Errorf("(truncated, timeout) = (%v, %v), want (%v, %v)", got.Result.Truncated, got.Result.TimedOut, tc.truncated, tc.timeout)
			}
			if tc.name == "throwing_output_callback" && (got.Result.Output != "complete output\n" || got.Result.Error == nil || !strings.Contains(*got.Result.Error, "output callback exploded")) {
				t.Errorf("callback diagnostic lost: %+v", got.Result)
			}
		})
	}
}

func TestVerifySplitAsync_TreeCleanupFailure(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	for _, tc := range []struct {
		name, setup, extra            string
		code, kills                   int
		truncated, timeout, cancelled bool
	}{
		{name: "clean_exit_zero"},
		{name: "nonzero_truncated", setup: `__treeCode = 7; __treeTruncated = true;`, code: 7, truncated: true},
		{name: "timeout_truncated", setup: `__treeCode = 124; __treeTruncated = true;`, code: 124, truncated: true, timeout: true},
		{name: "first_wait_rejects", setup: `__treeWaitStyle = 'reject';`, code: -1},
		{name: "rejected_post_start_spawn", setup: `__treeThrow = true;`, code: -1},
		{name: "cleanup_wait_resolves_after_callback_throw", setup: `__treeCode = 7; __treeWaitStyle = 'after_kill';`, extra: `, outputFn: function() { throw new Error('output callback exploded'); }`, code: 7, kills: 1},
		{name: "cleanup_wait_rejects_after_callback_throw", setup: `__treeWaitStyle = 'after_kill_reject';`, extra: `, outputFn: function() { throw new Error('output callback exploded'); }`, code: -1, kills: 1},
		{name: "cancellation_and_truncation", setup: `__treeTruncated = true; __treeWaitHook = function() { __treeCancelled = true; };`, extra: `, isCancelled: function() { return __treeCancelled; }`, truncated: true, cancelled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := observeTreeCleanup(t, "verifySplitAsync", `__treeError = 'tree close denied';`+tc.setup, tc.extra)
			assertTreeCleanupFailure(t, got, tc.code, 0, true)
			if got.Kills != tc.kills {
				t.Errorf("kill calls = %d, want %d; a known reaped-tree failure must not retry a group signal", got.Kills, tc.kills)
			}
			if got.Result.Truncated != tc.truncated || got.Result.TimedOut != tc.timeout || got.Result.Cancelled != tc.cancelled {
				t.Errorf("(truncated, timeout, cancelled) = (%v, %v, %v), want (%v, %v, %v)", got.Result.Truncated, got.Result.TimedOut, got.Result.Cancelled, tc.truncated, tc.timeout, tc.cancelled)
			}
		})
	}
}

func TestVerifySplit_WorktreeCommandTreeCleanupFailure(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	for _, method := range []string{"verifySplit", "verifySplitAsync"} {
		for _, tc := range []struct {
			name, setup             string
			code, removes, commands int
			retained                bool
		}{
			{name: "add_exit_zero", setup: `_gitResponses['worktree add'] = {code:0, error:true, message:'tree close denied', treeCleanupError:'tree close denied'};`, retained: true},
			{name: "add_rejects", setup: `_gitResponses['worktree add'] = function(){ var e=new Error('add failed after start'); e.treeCleanupError='tree close denied'; throw e; };`, code: -1, retained: true},
			{name: "fallback_remove_exit_zero", setup: `_gitResponses['worktree add'] = _gitFail('already checked out'); _gitResponses['worktree remove --force'] = {code:0, error:true, message:'tree close denied', treeCleanupError:'tree close denied'};`, removes: 1},
			{name: "fallback_remove_rejects", setup: `_gitResponses['worktree add'] = _gitFail('already checked out'); _gitResponses['worktree remove --force'] = function(){ var e=new Error('remove failed after start'); e.treeCleanupError='tree close denied'; throw e; };`, code: -1, removes: 1},
			{name: "detached_add_exit_zero", setup: `_gitResponses['worktree add'] = _gitFail('already checked out'); _gitResponses['worktree add --detach'] = {code:0, error:true, message:'tree close denied', treeCleanupError:'tree close denied'};`, removes: 1, retained: true},
			{name: "detached_add_rejects", setup: `_gitResponses['worktree add'] = _gitFail('already checked out'); _gitResponses['worktree add --detach'] = function(){ var e=new Error('detached add failed after start'); e.treeCleanupError='tree close denied'; throw e; };`, code: -1, removes: 1, retained: true},
			{name: "final_remove_exit_zero", setup: `__treeCode=7; _gitResponses['worktree remove --force'] = {code:0, error:true, message:'tree close denied', treeCleanupError:'tree close denied'};`, code: 7, removes: 1, commands: 1},
			{name: "final_remove_rejects", setup: `__treeCode=7; _gitResponses['worktree remove --force'] = function(){ var e=new Error('remove failed after start'); e.treeCleanupError='tree close denied'; throw e; };`, code: 7, removes: 1, commands: 1},
		} {
			t.Run(method+"/"+tc.name, func(t *testing.T) {
				got := observeTreeCleanup(t, method, tc.setup, "")
				assertTreeCleanupFailure(t, got, tc.code, tc.removes, tc.retained)
				if got.Commands != tc.commands {
					t.Errorf("verify commands = %d, want %d", got.Commands, tc.commands)
				}
			})
		}
	}
}

func TestVerifySplit_NoTreeCleanupFailure(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	for _, method := range []string{"verifySplit", "verifySplitAsync"} {
		for _, tc := range []struct {
			name, setup        string
			code               int
			passed, execFailed bool
		}{
			{name: "clean_exit", passed: true},
			{name: "ordinary_nonzero", setup: `__treeCode=7;`, code: 7},
			{name: "post_eof_pipe_close_error", setup: `__treePipeError=true;`, passed: true},
			{name: "ordinary_spawn_failure", setup: `__treeThrow=true;`, code: -1, execFailed: true},
		} {
			t.Run(method+"/"+tc.name, func(t *testing.T) {
				got := observeTreeCleanup(t, method, tc.setup, "")
				r := got.Result
				if r.Passed != tc.passed || r.ExitCode != tc.code || r.ExecFailed != tc.execFailed || r.TreeCleanupError != "" || r.WorktreeRetained || r.CleanupFailed || got.Removes != 1 {
					t.Errorf("ordinary result changed: %+v, removes=%d", r, got.Removes)
				}
			})
		}
	}
}

func TestVerifySplits_TreeCleanupFailureStopsBatch(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	for _, method := range []string{"verifySplits", "verifySplitsAsync"} {
		for _, scenario := range []string{"baseline", "branch", "failed_baseline_then_tree_failure", "cancelled_branch"} {
			if scenario == "cancelled_branch" && method == "verifySplits" {
				continue
			}
			t.Run(method+"/"+scenario, func(t *testing.T) {
				evalJS := treeCleanupEval(t)
				setup := `__treeError='tree close denied';`
				source := ""
				commands, removes := 1, 0
				if scenario == "baseline" {
					source = "feature"
				}
				if scenario == "cancelled_branch" {
					setup += `__treeWaitHook=function(){__treeCancelled=true;};`
				}
				if scenario == "failed_baseline_then_tree_failure" {
					source, commands, removes = "feature", 2, 1
					setup += `
						var baseCalls=0, originalSync=_gitResponses['!sh'], originalAsync=__childFactory;
						function nextResult(){ __treeCode=7; __treeError=(++baseCalls===1)?'':'tree close denied'; }
						_gitResponses['!sh']=function(){nextResult();return originalSync();};
						__childFactory=function(){nextResult();return originalAsync();};
					`
				}
				if _, err := evalJS(setup); err != nil {
					t.Fatal(err)
				}
				raw, err := evalJS(`(async function(){
					var r=await prSplit.` + method + `({dir:'/tmp/repo',sourceBranch:'` + source + `',verifyCommand:'make test',splits:[{name:'split/01',files:['a.go']},{name:'split/02',files:['b.go']}]},{verifyTimeoutMs:5000,isCancelled:function(){return __treeCancelled;}});
					return JSON.stringify({batch:r,commands:__treeCommandCalls,removes:__treeRemovalCount()});
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
				t.Logf("batch observation: %s", raw)
				if got.Batch.AllPassed || got.Commands != commands || got.Removes != removes || len(got.Batch.Results) != 1 {
					t.Fatalf("batch did not stop with one visible failed row: %+v", got)
				}
				r := got.Batch.Results[0]
				if r.Passed || r.PreExisting || !r.ExecFailed || !r.WorktreeRetained || r.TreeCleanupError != "tree close denied" || r.WorktreeDir == "" || !strings.Contains(got.Batch.Error, r.WorktreeDir) {
					t.Errorf("batch lost cleanup outcome: %+v", got)
				}
				if (scenario == "baseline") != r.Baseline || (scenario == "baseline" && r.Name != "feature") {
					t.Errorf("baseline row not identified: %+v", r)
				}
				if scenario == "cancelled_branch" && !r.Cancelled {
					t.Errorf("cancellation flag lost: %+v", r)
				}
			})
		}
	}
}

// Public worktree helpers are used by the interactive verification path. They
// must preserve the same ownership failure as the direct verifier, without
// starting a PTY or treating an attempted removal as confirmed retention.
func TestVerifyWorktree_PublicSetupTreeCleanupFailure(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	for _, method := range []string{"prepareVerifyWorktree", "startVerifySession"} {
		for _, tc := range []struct {
			name, setup   string
			code, removes int
			retained      bool
		}{
			{name: "add_exit_zero", setup: `_gitResponses['worktree add'] = {code:0, error:true, message:'tree close denied', treeCleanupError:'tree close denied'};`, retained: true},
			{name: "add_rejects", setup: `_gitResponses['worktree add'] = function(){ var e=new Error('add failed after start'); e.treeCleanupError='tree close denied'; throw e; };`, code: -1, retained: true},
			{name: "fallback_remove_exit_zero", setup: `_gitResponses['worktree add'] = _gitFail('already checked out'); _gitResponses['worktree remove --force'] = {code:0, error:true, message:'tree close denied', treeCleanupError:'tree close denied'};`, removes: 1},
			{name: "fallback_remove_rejects", setup: `_gitResponses['worktree add'] = _gitFail('already checked out'); _gitResponses['worktree remove --force'] = function(){ var e=new Error('remove failed after start'); e.treeCleanupError='tree close denied'; throw e; };`, code: -1, removes: 1},
			{name: "detached_add_exit_zero", setup: `_gitResponses['worktree add'] = _gitFail('already checked out'); _gitResponses['worktree add --detach'] = {code:0, error:true, message:'tree close denied', treeCleanupError:'tree close denied'};`, removes: 1, retained: true},
			{name: "detached_add_rejects", setup: `_gitResponses['worktree add'] = _gitFail('already checked out'); _gitResponses['worktree add --detach'] = function(){ var e=new Error('detached add failed after start'); e.treeCleanupError='tree close denied'; throw e; };`, code: -1, removes: 1, retained: true},
		} {
			t.Run(method+"/"+tc.name, func(t *testing.T) {
				evalJS := treeCleanupEval(t)
				if _, err := evalJS(tc.setup); err != nil {
					t.Fatal(err)
				}
				raw, err := evalJS(`(async function(){
					var tm=require('osm:termmux'), original=tm.newCaptureSession, creates=0;
					tm.newCaptureSession=function(){ creates++; return {start:function(){}, close:function(){}}; };
					try {
						var r=await prSplit.` + method + `('split/01',{dir:'/tmp/repo',verifyCommand:'make test'});
						return JSON.stringify({result:r,dir:r.dir,creates:creates,removes:__treeRemovalCount(),sessionIsNull:r.session===null});
					} finally { tm.newCaptureSession=original; }
				})()`)
				if err != nil {
					t.Fatal(err)
				}
				var got struct {
					treeCleanupObservation
					Dir           string `json:"dir"`
					Creates       int    `json:"creates"`
					SessionIsNull bool   `json:"sessionIsNull"`
				}
				if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
					t.Fatal(err)
				}
				t.Logf("public setup observation: %s", raw)
				assertTreeCleanupFailure(t, got.treeCleanupObservation, tc.code, tc.removes, tc.retained)
				if got.Dir == "" || got.Creates != 0 || (method == "startVerifySession" && !got.SessionIsNull) {
					t.Errorf("unsafe setup must preserve its repository and avoid starting a capture session: %+v", got)
				}
			})
		}
	}
}

func TestVerifyWorktree_PublicRemovalTreeCleanupFailure(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	for _, rejected := range []bool{false, true} {
		name := "resolved"
		if rejected {
			name = "rejected"
		}
		t.Run(name, func(t *testing.T) {
			evalJS := treeCleanupEval(t)
			setup := `_gitResponses['worktree remove --force'] = {code:0, error:true, message:'tree close denied', treeCleanupError:'tree close denied'};`
			if rejected {
				setup = `_gitResponses['worktree remove --force'] = function(){var e=new Error('remove failed after start');e.treeCleanupError='tree close denied';throw e;};`
			}
			if _, err := evalJS(setup); err != nil {
				t.Fatal(err)
			}
			raw, err := evalJS(`(async function(){
				var r=await prSplit.cleanupVerifyWorktree('/tmp/repo','/tmp/owned-worktree');
				return JSON.stringify({result:r,removes:__treeRemovalCount(),hasExitCode:!!r&&Object.prototype.hasOwnProperty.call(r,'exitCode')});
			})()`)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				treeCleanupObservation
				HasExitCode bool `json:"hasExitCode"`
			}
			if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
				t.Fatal(err)
			}
			t.Logf("public removal observation: %s", raw)
			assertTreeCleanupFailure(t, got.treeCleanupObservation, 0, 1, false)
			if got.HasExitCode || got.Result.WorktreeDir != "/tmp/owned-worktree" {
				t.Errorf("removal result must identify the path without replacing verification status: %+v", got)
			}
		})
	}
}

func TestVerifyWorktree_StartupRollbackTreeCleanupFailure(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	for _, rejected := range []bool{false, true} {
		name := "resolved"
		if rejected {
			name = "rejected"
		}
		t.Run(name, func(t *testing.T) {
			evalJS := treeCleanupEval(t)
			removeResult := `return {code:0,error:true,message:'tree close denied',treeCleanupError:'tree close denied'};`
			if rejected {
				removeResult = `var e=new Error('remove failed after start');e.treeCleanupError='tree close denied';throw e;`
			}
			raw, err := evalJS(`(async function(){
				var tm=require('osm:termmux'), original=tm.newCaptureSession, order=[];
				tm.newCaptureSession=function(){
					order.push('create');
					return {
						start:function(){order.push('start');return Promise.reject(new Error('capture startup failed'));},
						close:function(){order.push('close');return Promise.resolve().then(function(){order.push('close settled');});}
					};
				};
				_gitResponses['worktree remove --force']=function(){order.push('remove');` + removeResult + `};
				try {
					var r=await prSplit.startVerifySession('split/01',{dir:'/tmp/repo',verifyCommand:'make test'});
					return JSON.stringify({result:r,removes:__treeRemovalCount(),order:order,sessionIsNull:r.session===null,hasExitCode:Object.prototype.hasOwnProperty.call(r,'exitCode')});
				} finally {tm.newCaptureSession=original;}
			})()`)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				treeCleanupObservation
				Order         []string `json:"order"`
				SessionIsNull bool     `json:"sessionIsNull"`
				HasExitCode   bool     `json:"hasExitCode"`
			}
			if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
				t.Fatal(err)
			}
			t.Logf("public rollback observation: %s", raw)
			assertTreeCleanupFailure(t, got.treeCleanupObservation, 0, 1, false)
			if strings.Join(got.Order, ",") != "create,start,close,close settled,remove" || !got.SessionIsNull || got.HasExitCode || got.Result.Error == nil || !strings.Contains(*got.Result.Error, "capture startup failed") {
				t.Errorf("startup rollback lost its error, completion ordering or status boundary: %+v", got)
			}
		})
	}
}

func TestVerifyWorktree_PublicOrdinaryControls(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	for _, mode := range []string{"prepare", "start", "startup_failure", "clean_removal", "ordinary_removal_error", "rejected_removal_error"} {
		t.Run(mode, func(t *testing.T) {
			evalJS := treeCleanupEval(t)
			raw, err := evalJS(`(async function(){
				var mode='` + mode + `', tm=require('osm:termmux'), original=tm.newCaptureSession, creates=0, closed=false, removeAfterClose=false;
				var session={start:function(){if(mode==='startup_failure')throw new Error('capture startup failed');},close:function(){closed=true;return Promise.resolve();}};
				tm.newCaptureSession=function(){creates++;return session;};
				var ordinaryError=new Error('ordinary remove error');
				_gitResponses['worktree remove --force']=function(){
					removeAfterClose=closed;
					if(mode==='rejected_removal_error')throw ordinaryError;
					if(mode==='ordinary_removal_error')return _gitFail('ordinary remove error');
					return _gitOk();
				};
				try {
					var r, sameError=false;
					if(mode==='prepare')r=await prSplit.prepareVerifyWorktree('split/01',{dir:'/tmp/repo',verifyCommand:'make test'});
					else if(mode==='start'||mode==='startup_failure')r=await prSplit.startVerifySession('split/01',{dir:'/tmp/repo',verifyCommand:'make test'});
					else {try{r=await prSplit.cleanupVerifyWorktree('/tmp/repo','/tmp/owned-worktree');}catch(e){sameError=e===ordinaryError;}}
					return JSON.stringify({result:r,isUndefined:r===undefined,sameError:sameError,hasSession:!!r&&r.session===session,creates:creates,removes:__treeRemovalCount(),removeAfterClose:removeAfterClose});
				} finally {tm.newCaptureSession=original;}
			})()`)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Result           map[string]any `json:"result"`
				IsUndefined      bool           `json:"isUndefined"`
				SameError        bool           `json:"sameError"`
				HasSession       bool           `json:"hasSession"`
				Creates          int            `json:"creates"`
				Removes          int            `json:"removes"`
				RemoveAfterClose bool           `json:"removeAfterClose"`
			}
			if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
				t.Fatal(err)
			}
			t.Logf("ordinary public helper observation: %s", raw)
			switch mode {
			case "prepare":
				if len(got.Result) != 2 || got.Result["dir"] == nil || got.Result["worktreeDir"] == nil || got.Creates != 0 || got.Removes != 0 {
					t.Errorf("clean preparation shape changed: %+v", got)
				}
			case "start":
				if !got.HasSession || got.Creates != 1 || got.Removes != 0 || got.Result["worktreeDir"] == nil || got.Result["error"] != nil {
					t.Errorf("clean session start changed: %+v", got)
				}
			case "startup_failure":
				if len(got.Result) != 3 || got.Result["session"] != nil || got.Result["worktreeDir"] != nil || got.Removes != 1 || !got.RemoveAfterClose || !strings.Contains(got.Result["error"].(string), "capture startup failed") {
					t.Errorf("ordinary startup rollback changed: %+v", got)
				}
			case "clean_removal":
				if !got.IsUndefined || got.Removes != 1 || got.SameError {
					t.Errorf("successful removal contract changed: %+v", got)
				}
			default:
				if got.IsUndefined || got.Removes != 1 || got.SameError || got.Result["passed"] != false || got.Result["execFailed"] != true || got.Result["cleanupFailed"] != true || got.Result["worktreeDir"] != "/tmp/owned-worktree" {
					t.Errorf("ordinary removal failure must be classified with its path: %+v", got)
				}
				if got.Result["treeCleanupError"] != nil || got.Result["worktreeRetained"] != nil || got.Result["exitCode"] != nil {
					t.Errorf("attempted removal must not invent process-tree, retention or verification status: %+v", got)
				}
			}
		})
	}
}

func TestVerifyWorktree_StartupCloseFailureRetainsWorktree(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	for _, style := range []string{"throw", "reject", "false", "async_false", "missing"} {
		t.Run(style, func(t *testing.T) {
			evalJS := treeCleanupEval(t)
			raw, err := evalJS(`(async function(){
				var style='` + style + `', tm=require('osm:termmux'), original=tm.newCaptureSession, closes=0;
				tm.newCaptureSession=function(){
					var session={
						start:function(){throw new Error('capture startup failed');},
						close:function(){
							closes++;
							if(style==='throw')throw new Error('termination unconfirmed');
							if(style==='reject')return Promise.reject(new Error('termination unconfirmed'));
							if(style==='async_false')return Promise.resolve(false);
							return false;
						}
					};
					if(style==='missing')delete session.close;
					return session;
				};
				try {
					var r=await prSplit.startVerifySession('split/01',{dir:'/tmp/repo',verifyCommand:'make test'});
					return JSON.stringify({result:r,removes:__treeRemovalCount(),closes:closes,sessionIsNull:r.session===null,hasExitCode:Object.prototype.hasOwnProperty.call(r,'exitCode')});
				} finally {tm.newCaptureSession=original;}
			})()`)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				treeCleanupObservation
				Closes        int  `json:"closes"`
				SessionIsNull bool `json:"sessionIsNull"`
				HasExitCode   bool `json:"hasExitCode"`
			}
			if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
				t.Fatal(err)
			}
			t.Logf("unconfirmed capture termination observation: %s", raw)
			r := got.Result
			if r.Passed || !r.ExecFailed || !r.CleanupFailed || !r.WorktreeRetained || r.TreeCleanupError != "" || r.WorktreeDir == "" || got.Removes != 0 || !got.SessionIsNull || got.HasExitCode {
				t.Errorf("unconfirmed termination must retain worktree without inventing a tree-close error or exit status: %+v", got)
			}
			if r.Error == nil || !strings.Contains(*r.Error, "capture startup failed") || !strings.Contains(*r.Error, r.WorktreeDir) || !strings.Contains(*r.Error, "close") {
				t.Errorf("startup and cleanup diagnostics must remain visible: %+v", r)
			}
			wantCloses := 1
			if style == "missing" {
				wantCloses = 0
			}
			if got.Closes != wantCloses {
				t.Errorf("close calls = %d, want %d", got.Closes, wantCloses)
			}
		})
	}
}
