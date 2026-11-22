package command

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// The native result and verifier own the cleanup decision. The TUI must keep
// that decision visible, without allowing an old run to mutate a new one.
func TestChunk16c_CleanupMetadata(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript TUI engine")
	}
	for _, tc := range []struct {
		Name     string
		Baseline bool
		Stale    bool
		Clean    bool
		Cancel   bool
		Late     bool
		Ordinary bool
		Reject   bool
		Passed   bool
	}{
		{Name: "baseline failure remains visible", Baseline: true},
		{Name: "late baseline is observed before leaving verification", Baseline: true, Late: true},
		{Name: "split failure retains flags and blocks dependent"},
		{Name: "cancelled split retains cleanup diagnostic", Cancel: true},
		{Name: "stale baseline cannot mutate new run", Baseline: true, Stale: true},
		{Name: "stale split cannot mutate new run", Stale: true},
		{Name: "clean results preserve existing behavior", Clean: true},
		{Name: "ordinary baseline cleanup remains visible", Baseline: true, Ordinary: true},
		{Name: "ordinary late baseline blocks equivalence", Baseline: true, Late: true, Ordinary: true},
		{Name: "ordinary rejected baseline keeps cleanup", Baseline: true, Ordinary: true, Reject: true},
		{Name: "ordinary split cleanup blocks dependent", Ordinary: true},
		{Name: "ordinary rejected split keeps cleanup", Ordinary: true, Reject: true},
		{Name: "ordinary cleanup cannot accompany PASS", Ordinary: true, Passed: true},
		{Name: "ordinary stale baseline keeps new run", Baseline: true, Ordinary: true, Stale: true, Reject: true},
		{Name: "ordinary stale split keeps new run", Ordinary: true, Stale: true, Reject: true},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			eval := prsplittest.NewTUIEngineWithHelpers(t)
			cfg, err := json.Marshal(tc)
			if err != nil {
				t.Fatal(err)
			}
			got, err := eval(fmt.Sprintf(`(async function() {
				var cfg = %s;
				var cleanupWarnings = [];
				log.warn = function(message, fields) {
					if (message === 'verify cleanup failed') cleanupWarnings.push(fields);
				};
				setupPlanCache();
				var plan = prSplit._state.planCache;
				plan.sourceBranch = 'source-branch';
				plan.splits = [
					{name:'split/1', files:['a.go']},
					{name:'split/2', files:['b.go'], dependencies:['split/1']}
				];
				if (cfg.Late) plan.splits = plan.splits.slice(0, 1);
				var equivCalls = 0;
				prSplit._startEquivCheck = function(state) { equivCalls++; return [state, null]; };
				prSplit.runtime.verifyCommand = 'make test';
				prSplit.canSpawnInteractiveShell = function() { return false; };
				prSplit.startVerifySession = function() { return {error:'textonly'}; };
				var calls = [];
				var release, reject;
				var pending = new Promise(function(resolve, rejectFn) { release = resolve; reject = rejectFn; });
				var diagnostic = 'cleanup: worktree retained at /isolated/verify-worktree: tree close denied';
				var cleanup = {
					passed:false, exitCode:0, error:diagnostic, output:'prefix',
					treeCleanupError:'tree close denied', execFailed:true,
					cleanupFailed:true, worktreeRetained:true,
					worktreeDir:'/isolated/verify-worktree',
					truncated:true, timedOut:true, cancelled:cfg.Cancel
				};
				if (cfg.Ordinary) {
					diagnostic = 'worktree removal failed: permission denied; worktree removal was attempted at /isolated/verify-worktree';
					cleanup.error = diagnostic;
					delete cleanup.treeCleanupError;
					delete cleanup.worktreeRetained;
				}
				if (cfg.Passed) cleanup.passed = true;
				prSplit.verifySplitAsync = function(name) {
					calls.push(name);
					var target = cfg.Baseline ? name === plan.sourceBranch : name === 'split/1';
					if (target && !cfg.Clean) return pending;
					return Promise.resolve({name:name, passed:cfg.Clean, exitCode:cfg.Clean ? 0 : 7,
						error:cfg.Clean ? null : 'deterministic failure', output:''});
				};
				var s = initState('BRANCH_BUILDING');
				s.isProcessing = true;
				s.verifyingIdx = 0;
				s.verifyOutput = {};
				s.verificationResults = [];
				s.outputLines = [];
				s.verifyFallbackRunning = false;
				s._verifyRunEpoch = 1;
				prSplit._runVerifyBranch(s);
				if (cfg.Late) {
					await new Promise(function(resolve) { setTimeout(resolve, 0); });
					if (s.verifyingIdx !== 1 || s._baselineVerifyResult !== null) return 'late-baseline setup failed';
					prSplit._runVerifyBranch(s);
					if (equivCalls !== 0) return 'left verification before baseline cleanup settled';
				}
				// The target result stays pending until any old-run invalidation
				// is published. No wall-clock timing is involved.
				if (cfg.Stale) s._verifyRunEpoch++;
				if (cfg.Reject) reject(Object.assign(new Error(diagnostic), cleanup));
				else release(cleanup);
				await new Promise(function(resolve) { setTimeout(resolve, 0); });
				if (cfg.Stale) {
					if (s.verificationResults.length !== 0) return 'stale result changed rows: ' + JSON.stringify(s.verificationResults);
					if (cfg.Baseline && s._baselineVerifyResult !== null) return 'stale baseline changed cache';
					if (cleanupWarnings.length !== 1 || cleanupWarnings[0].worktreeDir !== cleanup.worktreeDir ||
						cleanupWarnings[0].branch !== (cfg.Baseline ? plan.sourceBranch : 'split/1')) return 'stale result lost cleanup ownership diagnostic';
					return 'OK';
				}
				var baseline = s._baselineVerifyResult;
				if (!baseline) return 'baseline never settled';
				if (cfg.Clean) {
					if (s.verificationResults.length !== 1 || !s.verificationResults[0].passed) return 'clean result changed';
					if (s.verificationResults[0].treeCleanupError || baseline.failed) return 'clean result acquired failure';
					return 'OK';
				}
				var rows = s.verificationResults;
				var row = cfg.Baseline ? baseline : rows.filter(function(r) { return r.name === 'split/1'; })[0];
				if (!row) return 'cleanup failure was lost: ' + JSON.stringify(rows);
				for (var key of ['treeCleanupError','execFailed','cleanupFailed','worktreeRetained','worktreeDir','truncated','timedOut','cancelled','exitCode']) {
					if (row[key] !== cleanup[key]) return 'row lost ' + key + ': ' + JSON.stringify(row);
				}
				if (row.passed || row.preExisting || row.error.indexOf('/isolated/verify-worktree') < 0) return 'cleanup failure not visible: ' + JSON.stringify(row);
				if (cfg.Baseline) {
					if (baseline.failed || baseline.treeCleanupError !== cleanup.treeCleanupError || baseline.worktreeDir !== cleanup.worktreeDir) return 'baseline cache lost cleanup facts';
					prSplit._runVerifyBranch(s);
					if (s.wizard.current !== 'ERROR' || s.errorDetails !== diagnostic || equivCalls !== 0) return 'baseline cleanup error not displayed: ' + JSON.stringify({state:s.wizard.current, error:s.errorDetails});
					if (rows.length !== 1 || rows[0].name !== 'split/1' || calls.indexOf('split/2') >= 0) return 'baseline failure changed branch alignment or started dependent';
					return 'OK';
				}
				// A failed cleanup must not be forgiven by a deterministic
				// baseline failure, nor unblock the next dependent branch.
				prSplit._runVerifyBranch(s);
				var dependent = s.verificationResults.filter(function(r) { return r.name === 'split/2'; })[0];
				if (!dependent || !dependent.skipped || calls.indexOf('split/2') >= 0) return 'dependent was not blocked';
				return 'OK';
			})()`, cfg))
			if err != nil {
				t.Fatal(err)
			}
			if got != "OK" {
				t.Fatalf("cleanup result propagation: %v", got)
			}
		})
	}
}

func TestChunk16c_SetupCleanupFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript TUI engine")
	}
	for _, mode := range []string{"interactive", "oneshot"} {
		for _, delivery := range []string{"sync", "resolve", "reject"} {
			t.Run(mode+"/"+delivery, func(t *testing.T) {
				eval := prsplittest.NewTUIEngineWithHelpers(t)
				got, err := eval(fmt.Sprintf(`(async function() {
					setupPlanCache();
					prSplit._state.planCache.sourceBranch = '';
					prSplit._state.planCache.splits = [{name:'split/1', files:['a.go']}];
					prSplit.runtime.verifyCommand = 'make test';
					var failure = {error:'tree close failed; worktree retained at /owned/setup',
						treeCleanupError:'tree close failed', cleanupFailed:true, execFailed:true,
						worktreeRetained:true, worktreeDir:'/owned/setup', dir:'/repo',
						exitCode:7, truncated:true, timedOut:true, cancelled:true, output:'prefix'};
					var removes = 0, spawns = 0, fallbacks = 0;
					prSplit.canSpawnInteractiveShell = function() { return %q === 'interactive'; };
					prSplit.spawnShellSession = function() { spawns++; return null; };
					prSplit.cleanupVerifyWorktree = function() { removes++; };
					prSplit.verifySplitAsync = function() { fallbacks++; return Promise.resolve({passed:true}); };
					var provide = function() {
						if (%q === 'resolve') return Promise.resolve(failure);
						if (%q === 'reject') return Promise.reject(Object.assign(new Error(failure.error), failure));
						return failure;
					};
					prSplit.prepareVerifyWorktree = provide;
					prSplit.startVerifySession = provide;
					var s = initState('BRANCH_BUILDING');
					s.isProcessing = true; s.verifyingIdx = 0; s.verificationResults = []; s.verifyOutput = {};
					prSplit._runVerifyBranch(s);
					if (s._verifySetup) {
						await s._verifySetup.promise.catch(function() {});
						await Promise.resolve();
						prSplit._runVerifyBranch(s);
					}
					await new Promise(function(resolve) { setTimeout(resolve, 0); });
					if (removes || spawns || fallbacks) return 'unsafe fallback/removal: ' + JSON.stringify({removes:removes,spawns:spawns,fallbacks:fallbacks});
					if (s.verifyingIdx !== 1 || s.verificationResults.length !== 1) return 'wrong split result count';
					var row = s.verificationResults[0];
					if (row.passed || row.preExisting || row.skipped || row.status !== prSplit._branchStatuses.FAILED) return 'setup failure was forgiven';
					for (var key of Object.keys(failure)) {
						if (row[key] !== failure[key]) return 'setup failure lost ' + key + ': ' + JSON.stringify(row);
					}
					return 'OK';
				})()`, mode, delivery, delivery))
				if err != nil {
					t.Fatal(err)
				}
				if got != "OK" {
					t.Fatal(got)
				}
			})
		}
	}
}

func TestChunk16c_StaleSetupCleanupOwnership(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript TUI engine")
	}
	for _, mode := range []string{"tree_failure", "retained", "rejected_tree_failure", "clean"} {
		t.Run(mode, func(t *testing.T) {
			eval := prsplittest.NewTUIEngineWithHelpers(t)
			got, err := eval(fmt.Sprintf(`(async function() {
				setupPlanCache();
				prSplit._state.planCache.sourceBranch = '';
				prSplit.runtime.verifyCommand = 'make test';
				prSplit.canSpawnInteractiveShell = function() { return false; };
				var resolveSetup, rejectSetup, closeEntered, finishClose;
				var setup = new Promise(function(resolve,reject) { resolveSetup=resolve; rejectSetup=reject; });
				var entered = new Promise(function(resolve) { closeEntered=resolve; });
				var close = new Promise(function(resolve) { finishClose=resolve; });
				var closed=0, removed=0, wrongOwner=0;
				prSplit.startVerifySession = function() { return setup; };
				prSplit.cleanupVerifyWorktree = function(dir,path) {
					if (dir !== '/repo' || path !== '/owned/old') throw new Error('removed a different worktree');
					removed++; return Promise.resolve();
				};
				var s = initState('BRANCH_BUILDING');
				s.isProcessing=true; s.verifyingIdx=0; s.verificationResults=[]; s.verifyOutput={};
				var track = prSplit._trackPaneOperation;
				prSplit._trackPaneOperation = function(owner,op) {
					if (owner !== s) wrongOwner++;
					return track(owner,op);
				};
				prSplit._runVerifyBranch(s);
				prSplit._invalidateVerifySetup(s);
				var replacement = {kind:'oneshot',branchName:'replacement',epoch:s._verifySetupEpoch,pending:true};
				s._verifySetup=replacement; s.verifyingIdx=4;
				var rows=[{name:'replacement',passed:true}]; s.verificationResults=rows;
				var result={worktreeDir:'/owned/old',dir:'/repo',session:{close:function(){closed++;closeEntered();return close;}}};
				var mode=%q;
				if (mode !== 'clean') result.error='old cleanup failed at /owned/old';
				if (mode === 'retained') result.worktreeRetained=true;
				if (mode === 'tree_failure' || mode === 'rejected_tree_failure') result.treeCleanupError='tree close failed';
				if (mode === 'rejected_tree_failure') rejectSetup(Object.assign(new Error(result.error),result));
				else resolveSetup(result);
				await entered;
				if (removed) return 'removed before session close settled';
				finishClose();
				await prSplit._waitForPaneOperations(s);
				if (closed !== 1 || wrongOwner || removed !== (mode === 'clean' ? 1 : 0)) return 'stale ownership failed: ' + JSON.stringify({closed:closed,removed:removed,wrongOwner:wrongOwner});
				if (s._verifySetup !== replacement || s.verifyingIdx !== 4 || s.verificationResults !== rows || rows.length !== 1) return 'stale cleanup changed replacement run';
				if (result.worktreeDir !== '/owned/old') return 'disposal erased original cleanup evidence';
				return 'OK';
			})()`, mode))
			if err != nil {
				t.Fatal(err)
			}
			if got != "OK" {
				t.Fatal(got)
			}
		})
	}
}

func TestChunk16c_InteractiveSetupKeepsWorktree(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript TUI engine")
	}
	eval := prsplittest.NewTUIEngineWithHelpers(t)
	got, err := eval(`(async function() {
		setupPlanCache();
		prSplit._state.planCache.sourceBranch = '';
		prSplit.runtime.verifyCommand = 'make test';
		var prepared=0, spawned=0, started=0, removed=0, fallback=0, finishPrepare, finishStart;
		var prepare = new Promise(function(resolve) { finishPrepare=resolve; });
		var start = new Promise(function(resolve) { finishStart=resolve; });
		prSplit.canSpawnInteractiveShell = function() { return true; };
		prSplit.prepareVerifyWorktree = function() { prepared++; return prepare; };
		var session={start:function(){started++;return start;},close:function(){},screen:function(){return '';},isDone:function(){return false;}};
		prSplit.spawnShellSession = function(path) {
			if (path !== '/owned/original') throw new Error('wrong startup path');
			spawned++; return session;
		};
		prSplit.startVerifySession = function() { fallback++; return {skipped:true}; };
		prSplit.cleanupVerifyWorktree = function() { removed++; };
		var s=initState('BRANCH_BUILDING');
		s.isProcessing=true; s.verifyingIdx=0; s.verificationResults=[]; s.verifyOutput={};
		prSplit._runVerifyBranch(s);
		finishPrepare({dir:'/repo',worktreeDir:'/owned/original'});
		await prepare; await Promise.resolve();
		prSplit._runVerifyBranch(s);
		var pending=s._verifySetup;
		if (!pending || pending.kind !== 'interactive-start' || !pending.pending) return 'startup was not deferred';
		prSplit._runVerifyBranch(s);
		if (prepared !== 1 || spawned !== 1 || started !== 1 || removed || fallback || s._verifySetup !== pending) return 'pending startup lost its worktree';
		finishStart();
		await pending.promise; await Promise.resolve();
		prSplit._runVerifyBranch(s);
		if (prepared !== 1 || spawned !== 1 || started !== 1 || removed || fallback) return 'startup was repeated';
		if (s.verifyMode !== 'interactive' || s._verifySessionRef !== session || s.activeVerifyWorktree !== '/owned/original') return 'original startup did not attach';
		await prSplit._clearVerifyPaneSession(s,{keepDisplay:false});
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "OK" {
		t.Fatal(got)
	}
}

func TestChunk16c_InteractiveRollbackObservesCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript TUI engine")
	}
	for _, failure := range []bool{false, true} {
		for _, startup := range []string{"spawn_throw", "start_reject"} {
			t.Run(fmt.Sprintf("%s/cleanup_failed_%v", startup, failure), func(t *testing.T) {
				eval := prsplittest.NewTUIEngineWithHelpers(t)
				got, err := eval(fmt.Sprintf(`(async function() {
					setupPlanCache();
					prSplit._state.planCache.sourceBranch='';
					prSplit.runtime.verifyCommand='make test';
					var prepared=0, removed=0, fallback=0, closed=0, finishCleanup, cleanupEntered;
					var cleanup = new Promise(function(resolve){finishCleanup=resolve;});
					var entered = new Promise(function(resolve){cleanupEntered=resolve;});
					prSplit.canSpawnInteractiveShell=function(){return true;};
					prSplit.prepareVerifyWorktree=function(){prepared++;return Promise.resolve({dir:'/repo',worktreeDir:'/owned/rollback'});};
					prSplit.spawnShellSession=function(){
						if (%q === 'spawn_throw') throw new Error('shell creation failed');
						return {start:function(){return Promise.reject(new Error('shell start failed'));},close:function(){closed++;}};
					};
					prSplit.cleanupVerifyWorktree=function(dir,path){
						if (path !== '/owned/rollback') throw new Error('wrong rollback path');
						removed++;cleanupEntered();return cleanup;
					};
					prSplit.startVerifySession=function(){fallback++;return Promise.resolve({skipped:true});};
					var s=initState('BRANCH_BUILDING');
					s.isProcessing=true;s.verifyingIdx=0;s.verificationResults=[];s.verifyOutput={};
					prSplit._runVerifyBranch(s);
					await s._verifySetup.promise;await Promise.resolve();
					prSplit._runVerifyBranch(s);
					await entered;
					var pending=s._verifySetup;
					prSplit._runVerifyBranch(s);
					if (prepared !== 1 || removed !== 1 || fallback || s._verifySetup !== pending) return 'rollback did not gate fallback';
					var failed=%v;
					finishCleanup(failed ? {error:'remove completed but tree close failed at /owned/rollback',treeCleanupError:'tree close failed',cleanupFailed:true,execFailed:true,worktreeDir:'/owned/rollback',dir:'/repo'} : undefined);
					await pending.promise;await Promise.resolve();
					prSplit._runVerifyBranch(s);
					if (failed) {
						var row=s.verificationResults[0];
						if (fallback || !row || row.passed || row.preExisting || !row.treeCleanupError || !row.cleanupFailed || !row.execFailed || row.worktreeDir !== '/owned/rollback' || row.worktreeRetained) return 'rollback failure lost: '+JSON.stringify(row);
					} else {
						if (fallback !== 1 || !s._verifySetup || s._verifySetup.kind !== 'oneshot') return 'clean rollback did not start one-shot fallback';
						await s._verifySetup.promise;await Promise.resolve();
						prSplit._runVerifyBranch(s);
						if (fallback !== 1 || !s.verificationResults[0] || !s.verificationResults[0].skipped) return 'one-shot stage did not resume';
					}
					if (prepared !== 1 || removed !== 1 || closed !== (%q === 'start_reject' ? 1 : 0)) return 'rollback ownership was repeated';
					return 'OK';
				})()`, startup, failure, startup))
				if err != nil {
					t.Fatal(err)
				}
				if got != "OK" {
					t.Fatal(got)
				}
			})
		}
	}
}

func TestChunk16c_InteractiveStartupRetainsUnclosedWorktree(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript TUI engine")
	}
	for _, mode := range []string{"spawn_tree_reject", "start_tree_reject", "close_tree_reject", "close_reject", "close_throw", "close_false", "close_missing"} {
		t.Run(mode, func(t *testing.T) {
			eval := prsplittest.NewTUIEngineWithHelpers(t)
			got, err := eval(fmt.Sprintf(`(async function() {
				setupPlanCache();
				prSplit._state.planCache.sourceBranch='';
				prSplit.runtime.verifyCommand='make test';
				var mode=%q, removed=0, fallback=0, closed=0;
				var nativeError=Object.assign(new Error('native tree close failed'),{treeCleanupError:'native tree close failed'});
				prSplit.canSpawnInteractiveShell=function(){return true;};
				prSplit.prepareVerifyWorktree=function(){return {dir:'/repo',worktreeDir:'/owned/unclosed'};};
				prSplit.spawnShellSession=function(){
					if (mode === 'spawn_tree_reject') return Promise.reject(nativeError);
					if (mode === 'close_missing') return {start:function(){return Promise.reject(new Error('shell start failed'));}};
					return {
						start:function(){return Promise.reject(mode === 'start_tree_reject' ? nativeError : new Error('shell start failed'));},
						close:function(){
							closed++;
							if (mode === 'close_tree_reject') return Promise.reject(nativeError);
							if (mode === 'close_reject') return Promise.reject(new Error('force kill timed out'));
							if (mode === 'close_throw') throw new Error('force kill timed out');
							if (mode === 'close_false') return false;
						}
					};
				};
				prSplit.cleanupVerifyWorktree=function(){removed++;};
				prSplit.startVerifySession=function(){fallback++;return {skipped:true};};
				var s=initState('BRANCH_BUILDING');
				s.isProcessing=true;s.verifyingIdx=0;s.verificationResults=[];s.verifyOutput={};
				prSplit._runVerifyBranch(s);
				if (!s._verifySetup) return 'startup was not deferred';
				await s._verifySetup.promise;await Promise.resolve();
				prSplit._runVerifyBranch(s);
				var row=s.verificationResults[0];
				if (removed || fallback || closed !== (mode === 'spawn_tree_reject' || mode === 'close_missing' ? 0 : 1)) return 'failed session owner was discarded';
				if (!row || row.passed || row.preExisting || !row.execFailed || !row.cleanupFailed || !row.worktreeRetained || row.worktreeDir !== '/owned/unclosed' || row.error.indexOf('/owned/unclosed') < 0) return 'missing retained outcome: '+JSON.stringify(row);
				if (!!row.treeCleanupError !== (mode.indexOf('tree') >= 0)) return 'incorrect native tree attribution: '+JSON.stringify(row);
				if (mode.indexOf('close') === 0 && row.error.indexOf('shell start failed') < 0) return 'original startup failure was lost';
				return 'OK';
			})()`, mode))
			if err != nil {
				t.Fatal(err)
			}
			if got != "OK" {
				t.Fatal(got)
			}
		})
	}
}

// Keep the real public cleanup helper in this fixture: replacing it with an
// already-classified result would miss a nonzero Git result discarded there.
func TestChunk16c_OrdinaryRollbackFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript TUI engine")
	}
	for _, mode := range []string{"interactive", "oneshot", "stale"} {
		for _, rejected := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/rejected_%v", mode, rejected), func(t *testing.T) {
				eval := prsplittest.NewTUIEngineWithHelpers(t)
				got, err := eval(fmt.Sprintf(`(async function() {
					var mode=%q, rejected=%v;
					setupPlanCache();prSplit._state.planCache.sourceBranch='';
					prSplit._state.planCache.splits=[{name:'split/1',files:['a.go']}];
					prSplit.runtime.verifyCommand='make test';prSplit.runtime.dir='/repo';
					var originalExec=prSplit._modules.exec.execv, tm=require('osm:termmux'), originalCapture=tm.newCaptureSession, originalWarn=log.warn;
					var worktree='/owned/rollback', removed=0, closed=0, fallback=0, entered, release, reject, finishSetup;
					var ready=new Promise(function(resolve){entered=resolve;});
					var cleanup=new Promise(function(resolve,rejectFn){release=resolve;reject=rejectFn;});
					var warnings=[];log.warn=function(message,fields){warnings.push(fields);};
					prSplit._modules.exec.execv=function(argv){
						var index=argv.indexOf('worktree');
						if(index<0)throw new Error('unexpected command');
						if(argv[index+1]==='add'){worktree=argv[index+2];return Promise.resolve({code:0,error:false,stdout:'',stderr:''});}
						if(argv[index+1]!=='remove' || argv[argv.length-1]!==worktree)throw new Error('wrong removal target');
						removed++;entered();return cleanup;
					};
					var session={start:function(){throw new Error('session startup failed');},close:function(){closed++;return Promise.resolve();}};
					tm.newCaptureSession=function(){return session;};
					prSplit.canSpawnInteractiveShell=function(){return mode==='interactive';};
					prSplit.prepareVerifyWorktree=function(){return Promise.resolve({dir:'/repo',worktreeDir:worktree});};
					prSplit.spawnShellSession=function(){return session;};
					prSplit.verifySplitAsync=function(){fallback++;return Promise.resolve({passed:true,exitCode:0});};
					if(mode==='stale')prSplit.startVerifySession=function(){return new Promise(function(resolve){finishSetup=resolve;});};
					var s=initState('BRANCH_BUILDING');s.isProcessing=true;s.verifyingIdx=0;s.verificationResults=[];s.verifyOutput={};
					try {
						prSplit._runVerifyBranch(s);
						var replacement, rows;
						if(mode==='interactive'){
							await s._verifySetup.promise;await Promise.resolve();prSplit._runVerifyBranch(s);
						}else if(mode==='stale'){
							prSplit._invalidateVerifySetup(s);
							replacement={kind:'oneshot',branchName:'replacement',epoch:s._verifySetupEpoch,pending:true};
							s._verifySetup=replacement;s.verifyingIdx=4;rows=[{name:'replacement',passed:true}];s.verificationResults=rows;
							finishSetup({session:session,dir:'/repo',worktreeDir:worktree});
						}
						await ready;
						if(removed!==1 || closed!==1 || fallback)return 'rollback did not own close before removal';
						if(rejected)reject(new Error('permission denied removing worktree'));
						else release({code:128,error:true,stdout:'',stderr:'permission denied removing worktree',message:'exit status 128'});
						await prSplit._waitForPaneOperations(s);
						if(mode==='stale'){
							if(s._verifySetup!==replacement || s.verifyingIdx!==4 || s.verificationResults!==rows || !rows[0].passed || rows[0].cleanupFailed)return 'old disposal changed replacement';
							if(warnings.length!==1 || warnings[0].worktreeDir!==worktree || warnings[0].worktreeRetained || warnings[0].error.indexOf('permission denied')<0)return 'stale removal diagnostic lost';
							return 'OK';
						}
						prSplit._runVerifyBranch(s);
						if(fallback || removed!==1 || closed!==1)return 'failed rollback started fallback or repeated ownership';
						var row=s.verificationResults[0];
						if(s.verifyingIdx!==1 || s.verificationResults.length!==1 || !row || row.passed || row.preExisting || row.skipped ||
							row.status!==prSplit._branchStatuses.FAILED || !row.execFailed || !row.cleanupFailed ||
							row.worktreeDir!==worktree || row.treeCleanupError || row.worktreeRetained || row.exitCode!==undefined)return 'ordinary rollback outcome lost: '+JSON.stringify(row);
						if(row.error.indexOf('session startup failed')<0 || row.error.indexOf('permission denied')<0 || row.error.indexOf(worktree)<0)return 'startup or cleanup reason/path lost';
						return 'OK';
					} finally {prSplit._modules.exec.execv=originalExec;tm.newCaptureSession=originalCapture;log.warn=originalWarn;}
				})()`, mode, rejected))
				if err != nil {
					t.Fatal(err)
				}
				if got != "OK" {
					t.Fatal(got)
				}
			})
		}
	}
}

func TestChunk16c_CompletedSessionRemovalFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript TUI engine")
	}
	for _, mode := range []string{"interactive", "oneshot"} {
		for _, outcome := range []string{"nonzero", "reject", "clean"} {
			t.Run(mode+"/"+outcome, func(t *testing.T) {
				eval := prsplittest.NewTUIEngineWithHelpers(t)
				got, err := eval(fmt.Sprintf(`(async function() {
					setupPlanCache();var mode=%q,outcome=%q,removed=0,closed=0;
					var original=prSplit._modules.exec.execv;
					prSplit._modules.exec.execv=function(argv){
						if(argv.indexOf('remove')<0 || argv.indexOf('/owned/finished')<0)throw new Error('wrong removal');
						removed++;
						if(outcome==='reject')return Promise.reject(new Error('permission denied removing worktree'));
						return Promise.resolve({code:outcome==='clean'?0:128,error:outcome!=='clean',stdout:'',stderr:outcome==='clean'?'':'permission denied removing worktree'});
					};
					var s=initState('BRANCH_BUILDING');s.isProcessing=true;s.verifyingIdx=0;s.verificationResults=[];s.verifyOutput={};
					s.verifyMode=mode;s.activeVerifyBranch='split/one';s.activeVerifyDir='/repo';s.activeVerifyWorktree='/owned/finished';s.activeVerifyStartTime=Date.now();
					s.activeVerifySession={screen:function(){return '';},output:function(){return 'captured output';},isDone:function(){return true;},exitCode:function(){return 0;},close:function(){closed++;return Promise.resolve();}};
					if(mode==='interactive'){s.verifySignal=true;s.verifySignalChoice='pass';s.verifySignalBranch='split/one';}
					try {
						prSplit._pollVerifySession(s);
						await prSplit._waitForPaneOperations(s);
						prSplit._pollVerifySession(s);
						var row=s.verificationResults[0];
						if(closed!==1 || removed!==1 || s.verifyingIdx!==1 || s.verificationResults.length!==1 || !row || row.output!=='captured output')return 'completion ownership changed';
						if(outcome==='clean'){
							if(!row.passed || row.cleanupFailed || row.status!==prSplit._branchStatuses.PASSED)return 'clean verification changed';
						}else{
							if(row.passed || row.preExisting || row.status!==prSplit._branchStatuses.FAILED || !row.execFailed || !row.cleanupFailed ||
								row.worktreeDir!=='/owned/finished' || row.treeCleanupError || row.worktreeRetained || row.error.indexOf('permission denied')<0 || row.error.indexOf('/owned/finished')<0)return 'completion hid removal failure: '+JSON.stringify(row);
						}
						return 'OK';
					} finally {prSplit._modules.exec.execv=original;}
				})()`, mode, outcome))
				if err != nil {
					t.Fatal(err)
				}
				if got != "OK" {
					t.Fatal(got)
				}
			})
		}
	}
}

// The manager mock models the separately tested native retain-on-exit
// contract. The UI must request that ownership atomically, and must dispose
// a late ID even after the raw capture has already been closed by cleanup.
func TestChunk16c_VerifyRegistrationOwnership(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript TUI engine")
	}
	for _, mode := range []string{"interactive", "oneshot"} {
		for _, schedule := range []string{"natural_eof", "close_failure", "reset", "pending_close_failure", "register_reject", "unregister_reject", "unregister_false", "null_id", "timeout"} {
			if mode == "interactive" && schedule == "timeout" {
				continue // Only one-shot sessions have an automatic deadline.
			}
			t.Run(mode+"/"+schedule, func(t *testing.T) {
				eval := prsplittest.NewTUIEngineWithHelpers(t)
				got, err := eval(fmt.Sprintf(`(async function() {
					var mode=%q,schedule=%q;
					setupPlanCache();prSplit._state.planCache.sourceBranch='';
					prSplit._state.planCache.splits=[{name:'split/one',files:['a.go']}];
					prSplit.runtime.verifyCommand='make test';
					var normal=schedule==='natural_eof'||schedule==='close_failure';
					var closeFailed=schedule==='close_failure'||schedule==='pending_close_failure';
					var registers=0,unregisters=0,closes=0,kills=0,removes=0,live=false,done=false,closed=false,options;
					var releaseRegistration,rejectRegistration,releaseClose,rejectClose,closeEntered;
					var registration=new Promise(function(resolve,reject){releaseRegistration=resolve;rejectRegistration=reject;});
					var closeReady=new Promise(function(resolve){closeEntered=resolve;});
					var firstClose=new Promise(function(resolve,reject){releaseClose=resolve;rejectClose=reject;});
					var session={start:function(){},screen:function(){return 'captured';},output:function(){return 'captured';},
						isDone:function(){return done;},exitCode:function(){return 0;},kill:function(){kills++;done=true;return Promise.resolve();},
						close:function(){
							closes++;
							if(closed)return Promise.resolve();
							closed=true;done=true;closeEntered();
							if(normal)return closeFailed?Promise.reject(new Error('first native close failed')):Promise.resolve();
							return firstClose;
						}};
					var oldMux=globalThis.tuiMux,oldWarn=log.warn,warnings=[];
					log.warn=function(message,fields){warnings.push(fields);};
					var mux={register:function(candidate,target,opts){
						if(candidate!==session||target.kind!=='verify'||target.name!=='split/one')throw new Error('wrong registration owner');
						registers++;options=opts;
						return registration;
					},unregister:function(id){
						unregisters++;
						if(id!==42||!live)return Promise.reject(new Error('session not found'));
						if(schedule==='unregister_reject')return Promise.reject(new Error('late unregister failed'));
						if(schedule==='unregister_false')return Promise.resolve(false);
						live=false;return session.close();
					},capture:function(){return {plain:'captured',fullScreen:'captured'};}};
					globalThis.tuiMux=mux;
					prSplit.canSpawnInteractiveShell=function(){return mode==='interactive';};
					prSplit.prepareVerifyWorktree=function(){return {dir:'/repo',worktreeDir:'/owned/registered'};};
					prSplit.spawnShellSession=function(){return session;};
					prSplit.startVerifySession=function(){return {session:session,dir:'/repo',worktreeDir:'/owned/registered',startTime:Date.now()};};
					prSplit.cleanupVerifyWorktree=function(dir,path){if(path!=='/owned/registered')throw new Error('wrong removal');removes++;};
					var s=initState('BRANCH_BUILDING');s.isProcessing=true;s.verifyingIdx=0;s.verificationResults=[];s.verifyOutput={};s._verifyRunEpoch=3;
					try {
						prSplit._runVerifyBranch(s);
						if(registers!==1||s.activeVerifySession!==session)return 'registration did not leave raw owner available';
						if(normal){
							live=true;releaseRegistration(42);await registration;
							await prSplit._waitForPaneOperations(s);
							if(s.activeVerifySession!==42)return 'live registration did not attach';
							done=true;
							// Native default behavior removes at EOF. Opted-in verification
							// must keep the first Close outcome for explicit cleanup.
							if(!options||!options.remainOnExit){live=false;await session.close().catch(function(){});}
							if(mode==='interactive'){s.verifySignal=true;s.verifySignalChoice='pass';s.verifySignalBranch='split/one';}
							prSplit._pollVerifySession(s);await prSplit._waitForPaneOperations(s);prSplit._pollVerifySession(s);
							var row=s.verificationResults[0];
							if(!options||options.remainOnExit!==true||live||closes!==1||unregisters!==1||s.verifyingIdx!==1)return 'natural EOF lost explicit close ownership';
							if(closeFailed){
								if(removes||!row||row.passed||!row.cleanupFailed||!row.worktreeRetained||row.error.indexOf('first native close failed')<0)return 'first close failure was erased';
							}else if(removes!==1||!row||!row.passed||row.cleanupFailed)return 'clean natural exit became cleanup failure';
							return 'OK';
						}
						var oldRow={name:'split/one',passed:true,status:prSplit._branchStatuses.PASSED,exitCode:0};
						var cleanup;
						if(schedule==='timeout'){
							s.verifyDeadline=Date.now()-1;
							prSplit._pollVerifySession(s);
							await closeReady;
							if(kills!==1||closes!==1||!s._verifyPaneCleanupPending)return 'pending registration blocked deadline termination';
							cleanup=s._verifyPaneCleanupPromise;
						}else{
							s.verificationResults=[oldRow];cleanup=prSplit._clearVerifyPaneSession(s,{keepDisplay:true});await closeReady;
						}
						if(removes||!s._verifyPaneCleanupPending)return 'removed worktree before first close settled';
						var replacement={},replacementRows=[{name:'replacement',passed:true}];
						if(schedule==='reset'){
							s._verifyRunEpoch++;s.activeVerifySession=replacement;s.activeVerifyWorktree='/owned/replacement';
							s.activeVerifyDir='/new-repo';s.verificationResults=replacementRows;
							globalThis.tuiMux={unregister:function(){throw new Error('replacement manager must not be used');}};
						}
						if(schedule==='register_reject')rejectRegistration(new Error('late registration failed'));
						else if(schedule==='null_id')releaseRegistration(null);
						else{live=true;releaseRegistration(42);}
						await registration.catch(function(){});
						// The helper's settlement was registered before this await, so
						// its late-ID disposal has already been invoked without sleeps.
						var expectsID=schedule!=='register_reject'&&schedule!=='null_id';
						if(unregisters!==(expectsID?1:0)||s.activeVerifySession===42||removes)return 'late ID escaped cleanup or authorized removal';
						if(closeFailed)rejectClose(new Error('first native close failed'));
						else releaseClose();
						await cleanup;await prSplit._waitForPaneOperations(s);
						if(!options||options.remainOnExit!==true)return 'registration did not retain native session';
						if(schedule==='reset'){
							if(live||s.activeVerifySession!==replacement||s.activeVerifyWorktree!=='/owned/replacement'||
								s.activeVerifyDir!=='/new-repo'||s.verificationResults!==replacementRows||!replacementRows[0].passed||s.verifyingIdx!==0)return 'late disposal changed replacement run';
						}else if(closeFailed){
							if(removes||oldRow.passed||!oldRow.cleanupFailed||!oldRow.worktreeRetained||oldRow.error.indexOf('first native close failed')<0)return 'late unregister hid raw close failure';
						}else if(removes!==1)return 'successful raw close did not permit normal cleanup';
						var registrationFailed=schedule==='register_reject'||schedule==='unregister_reject'||schedule==='unregister_false'||schedule==='null_id';
						if(registrationFailed){
							if(warnings.length!==1||warnings[0].branch!=='split/one'||warnings[0].worktreeDir!=='/owned/registered')return 'late registration failure was not observable';
						}else if(!closeFailed&&warnings.length)return 'clean late disposal reported failure';
						if(schedule!=='unregister_reject'&&schedule!=='unregister_false'&&live)return 'late native registration leaked';
						return 'OK';
					} finally {globalThis.tuiMux=oldMux;log.warn=oldWarn;releaseClose();releaseRegistration(42);}
				})()`, mode, schedule))
				if err != nil {
					t.Fatal(err)
				}
				if got != "OK" {
					t.Fatal(got)
				}
			})
		}
	}
}
