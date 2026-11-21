package command

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// A removal command can fail to close its process tree after verification
// already passed. Keep the original row's position and status facts, but do
// not present that cleanup failure as a passed branch or erase its path.
func TestChunk13_VerifyCleanupMetadata(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript TUI engine")
	}
	for _, tc := range []struct {
		Name     string
		Sync     bool
		Reject   bool
		Stale    bool
		NewToken bool
		Clean    bool
	}{
		{Name: "resolved cleanup changes passed row"},
		{Name: "rejected cleanup preserves diagnostic", Reject: true},
		{Name: "synchronous cleanup preserves diagnostic", Sync: true},
		{Name: "throwing cleanup preserves diagnostic", Sync: true, Reject: true},
		{Name: "old epoch cannot clear replacement pane", Stale: true},
		{Name: "old token cannot clear replacement cleanup", NewToken: true},
		{Name: "ordinary cleanup preserves passed row", Clean: true},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			eval := prsplittest.NewTUIEngineWithHelpers(t)
			cfg, err := json.Marshal(tc)
			if err != nil {
				t.Fatal(err)
			}
			got, err := eval(fmt.Sprintf(`(async function() {
				var cfg = %s;
				var row = {name:'split/one', passed:true, status:prSplit._branchStatuses.PASSED,
					preExisting:true, exitCode:0, timedOut:true, truncated:true, cancelled:true,
					output:'captured', duration:17};
				var rows = [row];
				var replacement = {name:'split/one', passed:true, status:prSplit._branchStatuses.PASSED};
				var closeCalls = 0, cleanupCalls = 0;
				var session = {close:function() { closeCalls++; }};
				var newSession = {close:function() { throw new Error('replacement must stay open'); }};
				var s = {paneOperations:[], _verifyRunEpoch:4, activeVerifySession:session,
					activeVerifyWorktree:'/isolated/old-worktree', activeVerifyDir:'/repo',
					activeVerifyBranch:'split/one', verifyingIdx:0,
					verificationResults:rows, outputLines:[]};
				var failure = {passed:false, execFailed:true, cleanupFailed:true,
					treeCleanupError:'job close denied', worktreeDir:'/isolated/old-worktree',
					error:'cleanup failed; worktree removal was attempted at /isolated/old-worktree'};
				var release, reject;
				var warnings = [];
				var originalCleanup = prSplit.cleanupVerifyWorktree;
				var originalWarn = log.warn;
				log.warn = function(message, fields) { warnings.push(fields); };
				prSplit.cleanupVerifyWorktree = function(dir, wt) {
					cleanupCalls++;
					if (dir !== '/repo' || wt !== '/isolated/old-worktree') throw new Error('wrong cleanup target');
					if (cfg.Sync) {
						if (cfg.Reject) throw failure;
						return failure;
					}
					return new Promise(function(resolve, rejectFn) { release = resolve; reject = rejectFn; });
				};
				try {
					var operation = prSplit._clearVerifyPaneSession(s, {keepDisplay:true});
					if (closeCalls !== 1 || cleanupCalls !== 1) return 'cleanup ownership count changed';
					var newToken = {};
					if (cfg.Stale || cfg.NewToken) {
						if (cfg.Stale) s._verifyRunEpoch++;
						if (cfg.NewToken) s._verifyPaneCleanupToken = newToken;
						s.activeVerifySession = newSession;
						s.activeVerifyWorktree = '/isolated/new-worktree';
						s.activeVerifyDir = '/new-repo';
						s.verificationResults = [replacement];
					}
					if (!cfg.Sync) {
						if (cfg.Reject) reject(failure);
						else release(cfg.Clean ? undefined : failure);
					}
					var result = await operation;
					if (cfg.Stale || cfg.NewToken) {
						if (s.activeVerifySession !== newSession || s.activeVerifyWorktree !== '/isolated/new-worktree' ||
							s.activeVerifyDir !== '/new-repo' || !replacement.passed || replacement.cleanupFailed ||
							s.outputLines.length !== 0 || s.verifyingIdx !== 0) return 'old cleanup mutated replacement run';
						if (cfg.NewToken && s._verifyPaneCleanupToken !== newToken) return 'old cleanup cleared replacement token';
						if (result !== failure || warnings.length !== 1 || warnings[0].worktreeDir !== '/isolated/old-worktree') return 'stale cleanup diagnostic lost';
						return 'OK';
					}
					if (cfg.Clean) {
						if (result !== undefined || warnings.length !== 0 || !row.passed || !row.preExisting || row.cleanupFailed) return 'ordinary cleanup changed';
					} else {
						if (result !== failure || warnings.length !== 1 || warnings[0].worktreeDir !== '/isolated/old-worktree') return 'cleanup result or diagnostic lost';
					}
					if (s.activeVerifySession !== null || s.activeVerifyWorktree !== null || s._verifyPaneCleanupPending) return 'old pane did not clear';
					if (cfg.Clean) return 'OK';
					if (s.verificationResults !== rows || rows.length !== 1 || rows[0] !== row) return 'branch alignment changed';
					if (row.passed || row.preExisting || row.status !== prSplit._branchStatuses.FAILED || !row.execFailed || !row.cleanupFailed) return 'cleanup failure remained a pass';
					if (row.treeCleanupError !== failure.treeCleanupError || row.worktreeDir !== failure.worktreeDir ||
						row.worktreeRetained || row.error.indexOf(failure.worktreeDir) < 0 || s.outputLines.length !== 1) return 'cleanup location or attempted-removal truth lost';
					if (row.exitCode !== 0 || !row.timedOut || !row.truncated || !row.cancelled || row.output !== 'captured' || row.duration !== 17) return 'verification facts overwritten';
					return 'OK';
				} finally {
					prSplit.cleanupVerifyWorktree = originalCleanup;
					log.warn = originalWarn;
				}
			})()`, cfg))
			if err != nil {
				t.Fatal(err)
			}
			if got != "OK" {
				t.Fatalf("active cleanup propagation: %v", got)
			}
		})
	}
}

// CaptureSession.close can reject when forced process termination times out.
// A rejected close is not authority to remove its working directory.
func TestChunk13_FailedSessionCloseRetainsWorktree(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript TUI engine")
	}
	for _, kind := range []string{"throw", "reject", "throw_null", "reject_undefined", "false", "async_false", "missing", "stale_rejection", "registered_throw", "registered_reject", "registered_false"} {
		t.Run(kind, func(t *testing.T) {
			eval := prsplittest.NewTUIEngineWithHelpers(t)
			encoded, err := json.Marshal(kind)
			if err != nil {
				t.Fatal(err)
			}
			got, err := eval(fmt.Sprintf(`(async function() {
				var kind = %s;
				var registered = kind.indexOf('registered_') === 0;
				var mode = registered ? kind.slice('registered_'.length) : kind;
				var cleanupCalls = 0, closeCalls = 0, rejectClose;
				var originalCleanup = prSplit.cleanupVerifyWorktree;
				var originalMux = globalThis.tuiMux;
				prSplit.cleanupVerifyWorktree = function() { cleanupCalls++; };
				var row = {name:'split/one', passed:true, preExisting:true,
					status:prSplit._branchStatuses.PASSED, exitCode:0};
				var session = {};
				if (mode !== 'missing') session.close = function() {
					closeCalls++;
					if (mode === 'throw') throw new Error('force kill did not complete');
					if (mode === 'reject') return Promise.reject(new Error('force kill did not complete'));
					if (mode === 'throw_null') throw null;
					if (mode === 'reject_undefined') return Promise.reject(undefined);
					if (mode === 'false') return false;
					if (mode === 'async_false') return Promise.resolve(false);
					return new Promise(function(resolve, reject) { rejectClose = reject; });
				};
				if (registered) globalThis.tuiMux = {unregister:function(id) {
					if (id !== 42) throw new Error('wrong registered session');
					return session.close();
				}};
				var s = {paneOperations:[], _verifyRunEpoch:7, activeVerifySession:registered ? 42 : session,
					activeVerifyWorktree:'/isolated/held-worktree', activeVerifyDir:'/repo',
					activeVerifyBranch:'split/one', verificationResults:[row], outputLines:[]};
				try {
					var operation = prSplit._clearVerifyPaneSession(s, {keepDisplay:true});
					var replacement = {passed:true};
					var replacementSession = {};
					if (kind === 'stale_rejection') {
						s._verifyRunEpoch++;
						s.activeVerifySession = replacementSession;
						s.activeVerifyWorktree = '/isolated/replacement';
						s.verificationResults = [replacement];
						rejectClose(new Error('force kill did not complete'));
					}
					var result = await operation;
					if (cleanupCalls !== 0 || closeCalls !== (kind === 'missing' ? 0 : 1)) return 'removed worktree without successful close';
					if (!result || !result.cleanupFailed || !result.execFailed || !result.worktreeRetained ||
						result.worktreeDir !== '/isolated/held-worktree' || result.treeCleanupError ||
						result.error.indexOf('/isolated/held-worktree') < 0) return 'close failure lost or relabeled as tree error';
					if (kind === 'throw_null' && result.error.indexOf('null') < 0) return 'null close reason lost';
					if (kind === 'reject_undefined' && result.error.indexOf('undefined') < 0) return 'undefined close reason lost';
					if (kind === 'stale_rejection') {
						if (s.activeVerifySession !== replacementSession || s.activeVerifyWorktree !== '/isolated/replacement' ||
							!replacement.passed || replacement.cleanupFailed || s.outputLines.length !== 0) return 'old close changed replacement';
						return 'OK';
					}
					if (row.passed || row.preExisting || row.status !== prSplit._branchStatuses.FAILED ||
						!row.worktreeRetained || !row.cleanupFailed || !row.execFailed || row.treeCleanupError || row.exitCode !== 0) return 'row did not preserve the cleanup uncertainty';
					if (s.outputLines.length !== 1 || s.activeVerifySession !== null || s._verifyPaneCleanupPending) return 'cleanup did not settle visibly';
					return 'OK';
				} finally {
					prSplit.cleanupVerifyWorktree = originalCleanup;
					if (originalMux === undefined) delete globalThis.tuiMux;
					else globalThis.tuiMux = originalMux;
				}
			})()`, encoded))
			if err != nil {
				t.Fatal(err)
			}
			if got != "OK" {
				t.Fatalf("failed session close: %v", got)
			}
		})
	}
}

// Exercise the public removal helper as well as rejected cleanup callbacks:
// a successful verification command does not make failed removal successful.
func TestChunk13_OrdinaryRemovalFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript TUI engine")
	}
	for _, tc := range []struct {
		Name     string
		Mode     string
		Stale    bool
		NewToken bool
	}{
		{Name: "resolved Git failure", Mode: "git_failure"},
		{Name: "rejected Git removal", Mode: "git_rejection"},
		{Name: "rejected cleanup callback", Mode: "callback_rejection"},
		{Name: "throwing cleanup callback", Mode: "callback_throw"},
		{Name: "null cleanup rejection", Mode: "null_rejection"},
		{Name: "undefined cleanup rejection", Mode: "undefined_rejection"},
		{Name: "old epoch keeps replacement", Mode: "callback_rejection", Stale: true},
		{Name: "old token keeps replacement", Mode: "callback_rejection", NewToken: true},
		{Name: "clean removal preserves pass", Mode: "clean"},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			eval := prsplittest.NewTUIEngineWithHelpers(t)
			cfg, err := json.Marshal(tc)
			if err != nil {
				t.Fatal(err)
			}
			got, err := eval(fmt.Sprintf(`(async function() {
				var cfg=%s, cleanupCalls=0, closeCalls=0, release, reject, entered;
				var ready=new Promise(function(resolve){entered=resolve;});
				var pending=new Promise(function(resolve,rejectFn){release=resolve;reject=rejectFn;});
				var reason=Object.assign(new Error('permission denied removing worktree'),{cleanupDetail:'preserved'});
				var originalExec=prSplit._modules.exec.execv, originalCleanup=prSplit.cleanupVerifyWorktree, originalWarn=log.warn;
				var warnings=[];
				log.warn=function(message,fields){warnings.push(fields);};
				prSplit._modules.exec.execv=function(argv){
					if(argv.indexOf('worktree')<0 || argv.indexOf('remove')<0 || argv.indexOf('/owned/ordinary')<0)throw new Error('unexpected command');
					cleanupCalls++;entered();return pending;
				};
				if(cfg.Mode.indexOf('callback')===0 || cfg.Mode==='null_rejection' || cfg.Mode==='undefined_rejection') {
					prSplit.cleanupVerifyWorktree=function(dir,path){
						if(dir!=='/repo' || path!=='/owned/ordinary')throw new Error('wrong cleanup owner');
						cleanupCalls++;entered();
						if(cfg.Mode==='callback_throw')throw reason;
						return pending;
					};
				}
				var row={name:'split/one',passed:true,status:prSplit._branchStatuses.PASSED,preExisting:true,
					exitCode:0,output:'captured',duration:17,truncated:true,timedOut:true,cancelled:true};
				var rows=[row],replacement={name:'replacement',passed:true},newSession={};
				var s={paneOperations:[],_verifyRunEpoch:4,activeVerifySession:{close:function(){closeCalls++;}},
					activeVerifyWorktree:'/owned/ordinary',activeVerifyDir:'/repo',activeVerifyBranch:'split/one',
					verificationResults:rows,outputLines:[],verifyingIdx:0};
				try {
					var operation=prSplit._clearVerifyPaneSession(s,{keepDisplay:true});
					await ready;
					var newToken={};
					if(cfg.Stale || cfg.NewToken){
						if(cfg.Stale)s._verifyRunEpoch++;
						if(cfg.NewToken)s._verifyPaneCleanupToken=newToken;
						s.activeVerifySession=newSession;s.activeVerifyWorktree='/owned/replacement';
						s.activeVerifyDir='/replacement';s.verificationResults=[replacement];
					}
					if(cfg.Mode==='git_failure')release({code:128,error:true,stdout:'',stderr:'permission denied removing worktree',message:'exit status 128'});
					else if(cfg.Mode==='clean')release({code:0,error:false,stdout:'',stderr:'',message:''});
					else if(cfg.Mode==='null_rejection')reject(null);
					else if(cfg.Mode==='undefined_rejection')reject(undefined);
					else if(cfg.Mode!=='callback_throw')reject(reason);
					var result=await operation;
					if(closeCalls!==1 || cleanupCalls!==1)return 'cleanup was repeated';
					if(cfg.Mode==='clean'){
						if(result!==undefined || !row.passed || !row.preExisting || row.cleanupFailed || warnings.length)return 'clean cleanup changed';
						return 'OK';
					}
					if(!result || result.passed!==false || !result.execFailed || !result.cleanupFailed ||
						result.worktreeDir!=='/owned/ordinary' || result.worktreeRetained || result.treeCleanupError ||
						result.exitCode!==undefined || result.error.indexOf('/owned/ordinary')<0)return 'ordinary removal failure lost: '+JSON.stringify(result);
					var expectedReason=cfg.Mode==='null_rejection'?'null':cfg.Mode==='undefined_rejection'?'undefined':'permission denied';
					if(result.error.indexOf(expectedReason)<0)return 'removal reason lost';
					if(cfg.Mode.indexOf('callback')===0 && result.cleanupDetail!=='preserved')return 'rejected metadata lost';
					if(warnings.length!==1 || warnings[0].worktreeDir!=='/owned/ordinary')return 'cleanup path not reported';
					if(cfg.Stale || cfg.NewToken){
						if(s.activeVerifySession!==newSession || s.activeVerifyWorktree!=='/owned/replacement' ||
							s.activeVerifyDir!=='/replacement' || !replacement.passed || replacement.cleanupFailed ||
							s.outputLines.length || s.verifyingIdx!==0)return 'old removal changed replacement';
						if(cfg.NewToken && s._verifyPaneCleanupToken!==newToken)return 'old removal cleared new token';
						return 'OK';
					}
					if(s.verificationResults!==rows || rows.length!==1 || rows[0]!==row || row.passed || row.preExisting ||
						row.status!==prSplit._branchStatuses.FAILED || !row.cleanupFailed || !row.execFailed ||
						row.worktreeDir!=='/owned/ordinary' || row.treeCleanupError || row.worktreeRetained)return 'failed removal remained a pass';
					if(row.exitCode!==0 || row.output!=='captured' || row.duration!==17 || !row.truncated || !row.timedOut || !row.cancelled)return 'verification facts changed';
					if(s.activeVerifySession!==null || s.activeVerifyWorktree!==null || s._verifyPaneCleanupPending || s.outputLines.length!==1)return 'cleanup did not settle';
					return 'OK';
				} finally {
					prSplit._modules.exec.execv=originalExec;prSplit.cleanupVerifyWorktree=originalCleanup;log.warn=originalWarn;
				}
			})()`, cfg))
			if err != nil {
				t.Fatal(err)
			}
			if got != "OK" {
				t.Fatal(got)
			}
		})
	}
}
