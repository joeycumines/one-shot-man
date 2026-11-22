package command

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

func TestVerifyContinueCleanupFailureIsFailed(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript TUI engine")
	}
	eval := prsplittest.NewTUIEngineWithHelpers(t)
	for _, mode := range []string{"close_failure", "remove_failure", "clean"} {
		t.Run(mode, func(t *testing.T) {
			got, err := eval(fmt.Sprintf(`(async function() {
				var mode = %q;
				setupPlanCache();
				var splits = [{name:'split/1', files:['a.go']}];
				prSplit._state.planCache.sourceBranch = '';
				prSplit._state.planCache.splits = splits;
				var oldCleanup = prSplit.cleanupVerifyWorktree;
				var closes = 0, removes = 0;
				prSplit.cleanupVerifyWorktree = function() {
					removes++;
					if (mode === 'remove_failure') return {
						passed:false, execFailed:true, cleanupFailed:true,
						worktreeDir:'/owned/continued', error:'worktree removal failed: permission denied'
					};
				};
				try {
					var s = initState('BRANCH_BUILDING');
					s.isProcessing = true; s.verifyingIdx = 0; s._verifyRunEpoch = 1;
					s.verificationResults = []; s.verifyOutput = {}; s.outputLines = [];
					s.activeVerifySession = {
						screen:function() { return ''; }, output:function() { return ''; },
						isDone:function() { return true; },
						close:function() {
							closes++;
							return mode === 'close_failure' ? Promise.reject(new Error('native close failed')) : Promise.resolve();
						}
					};
					s.activeVerifyBranch = 'split/1'; s.activeVerifyWorktree = '/owned/continued';
					s.activeVerifyDir = '/repo'; s.activeVerifyStartTime = Date.now();
					s.verifyMode = 'interactive'; s.verifyShellExited = false;
					s.verifySignal = false; s.verifySignalChoice = null; s.verifySignalBranch = 'split/1';
					prSplit._pollVerifySession(s);
					if (!s.verifyShellExited) return 'shell EOF was not observed';
					prSplit._handleVerifySignal(s, 'continue');
					prSplit._pollVerifySession(s);
					await s._verifyPaneCleanupPromise;
					var row = s.verificationResults[0], lines = [];
					prSplit._renderVerificationStatusList(s, splits, lines);
					prSplit._renderVerificationSummary(s, splits, lines);
					var rendered = lines.join('\n');
					if (closes !== 1 || removes !== (mode === 'close_failure' ? 0 : 1)) return 'wrong resource cleanup count';
					if (mode === 'clean') {
						if (!row.skipped || row.cleanupFailed || rendered.indexOf('1 skipped') < 0) return 'clean Continue changed: ' + rendered;
					} else {
						if (!row.cleanupFailed || row.skipped || row.passed || row.preExisting ||
							row.status !== prSplit._branchStatuses.FAILED || rendered.indexOf('1 failed') < 0) {
							return JSON.stringify({row:row, rendered:rendered});
						}
						if (!!row.worktreeRetained !== (mode === 'close_failure')) return 'removal attempt confused with retention';
					}
					return 'OK';
				} finally { prSplit.cleanupVerifyWorktree = oldCleanup; }
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

// Run the real async verifier, Git collection, reset/cancel handlers and exit
// hook. Only native child handles are substituted, at their public JS boundary.
func TestVerifyOwnedWorktreeLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript TUI engine")
	}
	tmp := t.TempDir()
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, tmp)
	}
	eval := prsplittest.NewTUIEngineWithHelpers(t)
	for _, kind := range []string{"baseline", "fallback", "auto", "repair"} {
		for _, scenario := range []struct {
			Phase   string
			Action  string
			Failure string
		}{
			{Phase: "verify", Action: "replace", Failure: "tree"},
			{Phase: "setup", Action: "replace", Failure: "tree"},
			{Phase: "spawn", Action: "replace", Failure: "reject"},
			{Phase: "verify", Action: "replace", Failure: "reject_result"},
			{Phase: "remove", Action: "replace", Failure: "remove"},
			{Phase: "verify", Action: "replace"},
			{Phase: "setup", Action: "cancel", Failure: "tree"},
			{Phase: "setup", Action: "cancel"},
			{Phase: "setup", Action: "deadline"},
		} {
			if scenario.Failure == "reject_result" && kind != "repair" {
				continue
			}
			t.Run(fmt.Sprintf("%s/%s/%s/%s", kind, scenario.Phase, scenario.Action, scenario.Failure), func(t *testing.T) {
				cfg, err := json.Marshal(scenario)
				if err != nil {
					t.Fatal(err)
				}
				got, err := eval(fmt.Sprintf(`(async function() {
					var cfg = %s, kind = %q, baseline = kind === 'baseline' || kind === 'auto';
					var realVerify = prSplit.verifySplitAsync, realSpawn = prSplit._modules.exec.spawn;
					var realResolve = prSplit.resolveConflicts;
					var oldOutcome = prSplit._state.executionOutcomeCache, oldRows = prSplit._state.executionResultCache;
					var oldCanSpawn = prSplit.canSpawnInteractiveShell, oldStart = prSplit.startVerifySession;
					var oldWarn = log.warn, oldPrint = output.print, oldOutputActive = output._setTUIOutputActive;
					var oldRun = prSplit._tea.run, oldExit = globalThis.__postBubbleTeaExit;
					var oldCapture = prSplit._outputCaptureFn, oldBudget = prSplit._PANE_CLEANUP_TIMEOUT_MS;
					var oldConfig = prSplit._handleConfigState, oldAgent = prSplit._state.agentExecutor;
					var oldAutomated = prSplit.automatedSplit, oldCancelSource = prSplit._cancelSource;
					var realSetTimeout = globalThis.setTimeout, realClearTimeout = globalThis.clearTimeout;
					var timers = new Map();
					globalThis.setTimeout = function(fn, ms) {
						var id = realSetTimeout(function() { timers.delete(id); fn(); }, ms);
						timers.set(id, ms); return id;
					};
					globalThis.clearTimeout = function(id) { timers.delete(id); return realClearTimeout(id); };
					var warnings = [], prints = [], outputActive = false, headers = [];
					log.warn = function(message, fields) { warnings.push({message:message, fields:fields}); };
					output.print = function(message) { prints.push({active:outputActive, message:String(message)}); };
					output._setTUIOutputActive = function(active) { outputActive = active; };
					prSplit._tea.run = function() {};
					prSplit._verifyCleanupDiagnostics = [];
					prSplit.startWizard();
					prSplit._outputCaptureFn = function(line) { headers.push(line); };
					prSplit._PANE_CLEANUP_TIMEOUT_MS = cfg.Action === 'deadline' ? 20 : 1000;
					setupPlanCache();
					prSplit._state.planCache.sourceBranch = baseline ? 'source' : '';
					prSplit._state.planCache.splits = [{name:'split/1', files:['a.go']}];
					var committedRows = [{name:'split/1', sha:'original-committed-sha'}];
					var committedOutcome = {error:null, results:committedRows};
					prSplit._state.executionOutcomeCache = committedOutcome;
					prSplit._state.executionResultCache = committedRows;
					prSplit.runtime.verifyCommand = 'verify-command';
					prSplit.canSpawnInteractiveShell = function() { return false; };
					prSplit.startVerifySession = function() { return {error:'textonly'}; };
					var target = kind === 'auto' ? prSplit.runtime.baseBranch : baseline ? 'source' : 'split/1';
					var automatedCalls = 0;
					if (kind === 'auto') {
						prSplit._handleConfigState = function() {
							return {baselineVerifyConfig:{verifyCommand:'verify-command', dir:'.'}};
						};
						prSplit._state.agentExecutor = {resolved:true, close:function() { return Promise.resolve(); }};
						prSplit.automatedSplit = async function() { automatedCalls++; return {error:null}; };
					}
					var paths = {}, outcomes = {}, removes = [], shells = [];
					var release, reject, entered;
					var pending = new Promise(function(resolve, rejectFn) { release = resolve; reject = rejectFn; });
					var ready = new Promise(function(resolve) { entered = resolve; });
					function stream(text) {
						return {read:function() {
							if (text) { var value = text; text = ''; return Promise.resolve({value:value, done:false}); }
							return Promise.resolve({done:true});
						}};
					}
					function child(wait, stderr) {
						return {stdout:stream(''), stderr:stream(stderr), stdin:{close:function() {}},
							wait:function() { return wait; }, kill:function() { return Promise.resolve(); }};
					}
					prSplit._modules.exec.spawn = function(command, args) {
						var index = args.indexOf('worktree'), name;
						if (command === 'git' && index >= 0) {
							if (args[index + 1] === 'add') {
								name = args[args.length - 1]; paths[name] = args[args.length - 2];
								if (name === target && (cfg.Phase === 'setup' || cfg.Phase === 'spawn')) {
									entered();
									return cfg.Phase === 'spawn' ? pending : Promise.resolve(child(pending));
								}
							} else {
								var path = args[args.length - 1]; removes.push(path);
								if (path === paths[target] && cfg.Phase === 'remove') {
									entered(); return Promise.resolve(child(pending, 'permission denied'));
								}
							}
						} else {
							var commandText = args.join(' ');
							name = Object.keys(paths).filter(function(key) { return commandText.indexOf(paths[key]) >= 0; })[0];
							if (!name) throw new Error('verification shell did not identify an allocated worktree: ' + commandText);
							shells.push(name);
							if (name === target && cfg.Phase === 'verify') { entered(); return Promise.resolve(child(pending)); }
						}
						return Promise.resolve(child(Promise.resolve({code:0, signal:null})));
					};
					prSplit.verifySplitAsync = function(name, options) {
						return realVerify(name, options).then(function(outcome) { outcomes[name] = outcome; return outcome; });
					};
					prSplit.resolveConflicts = function(plan, options) {
						return realResolve(plan, options).then(function(outcome) {
							if (cfg.Failure === 'reject_result') {
								var error = Object.assign(new Error(outcome.error), outcome);
								outcomes[target] = error; throw error;
							}
							outcomes[target] = outcome; return outcome;
						});
					};
					var s = initState(kind === 'auto' ? 'CONFIG' : 'BRANCH_BUILDING');
					s.isProcessing = true; s.verifyingIdx = 0; s._verifyRunEpoch = 1;
					s.verificationResults = []; s.verifyOutput = {}; s.outputLines = [];
					try {
						if (kind === 'auto') {
							prSplit._startAutoAnalysis(s);
							prSplit._outputCaptureFn = function(line) { headers.push(line); };
						} else if (kind === 'repair') {
							s.wizard.transition('ERROR_RESOLUTION'); s.wizardState = 'ERROR_RESOLUTION';
							prSplit._handleErrorResolutionChoice(s, 'auto-resolve');
						} else prSplit._runVerifyBranch(s);
						await ready;
						await new Promise(function(resolve) { realSetTimeout(resolve, 0); });
						var job = s._resourceJobs.filter(function(job) { return job.branchName === target; })[0];
						if (!job || job.worktreeDir !== paths[target]) return 'allocation was not owned before native setup';
						if (!headers.some(function(line) { return line.indexOf(paths[target]) >= 0; })) return 'real Git setup header absent';
						if (cfg.Action === 'replace') {
							prSplit._resetVerifyPhase(s);
							prSplit._invalidateAsyncConfig(s);
							s.isProcessing = true; s.activeVerifyBranch = 'replacement';
							s.verificationResults = [{name:'replacement', marker:true}];
							s.outputLines = ['replacement output']; s.verifyScreen = 'replacement screen';
							s._baselineVerifyResult = {replacement:true}; s.verifyingIdx = 7;
							s.resolveResult = {replacement:true}; s.resolveRunning = true;
						} else {
							sendKey(s, 'ctrl+c');
							if (!s.showConfirmCancel) return 'cancel confirmation not opened';
							sendKey(s, 'y');
							if (s.wizardState !== 'CANCELLED' || s.wizardQuitSent) return 'cancel did not wait for owned verification';
							var firstJoin = prSplit._cancelOwnedWork(s);
							if (firstJoin !== prSplit._cancelOwnedWork(s)) return 'repeat cancellation created a new lifetime join';
							var deadlineTimers = Array.from(timers.values()).filter(function(ms) { return ms === prSplit._PANE_CLEANUP_TIMEOUT_MS; });
							if (deadlineTimers.length !== 1) return 'repeat cancellation created multiple deadline timers';
						}
						var stateBefore = JSON.stringify({rows:s.verificationResults, lines:s.outputLines, screen:s.verifyScreen,
							baseline:s._baselineVerifyResult, branch:s.activeVerifyBranch, index:s.verifyingIdx,
							resolution:s.resolveResult, resolving:s.resolveRunning});
						if (cfg.Action === 'deadline') {
							await new Promise(function(resolve) { realSetTimeout(resolve, 40); });
							if (!s.wizardQuitSent) return 'cancel join exceeded its bound';
							if (warnings.length !== 1 || !warnings[0].fields.cleanupUnconfirmed ||
								warnings[0].fields.worktreeRetained !== undefined ||
								warnings[0].fields.error.indexOf('unconfirmed') < 0) return 'deadline claimed cleanup facts it did not observe';
							if (kind === 'repair' && (!prSplit._state.executionOutcomeCache.cleanupUnconfirmed ||
								prSplit._state.executionOutcomeCache.worktreeDir !== paths[target])) return 'pending repair ownership was not checkpointed before quit';
							globalThis.__postBubbleTeaExit();
							if (prints.length !== 1 || prints[0].active || prints[0].message.indexOf(paths[target]) < 0 ||
								prints[0].message.indexOf(target) < 0) return 'unconfirmed resource diagnostic did not survive terminal exit';
						}
						if (cfg.Failure === 'reject') reject(Object.assign(new Error('native startup rollback failed'), {treeCleanupError:'tree close denied'}));
						else release({code:cfg.Failure === 'remove' ? 128 : 0, signal:null,
							treeCleanupError:(cfg.Failure === 'tree' || cfg.Failure === 'reject_result') ? 'tree close denied' : undefined});
						try { await job.promise; }
						catch (error) { if (cfg.Failure !== 'reject_result') throw error; }
						await new Promise(function(resolve) { realSetTimeout(resolve, 0); });
						var stateAfter = JSON.stringify({rows:s.verificationResults, lines:s.outputLines, screen:s.verifyScreen,
							baseline:s._baselineVerifyResult, branch:s.activeVerifyBranch, index:s.verifyingIdx,
							resolution:s.resolveResult, resolving:s.resolveRunning});
						if (stateBefore !== stateAfter) return 'stale owner changed replacement/cancel state: ' + stateAfter;
						if (automatedCalls) return 'stale auto baseline launched the replacement pipeline';
						if (s._resourceJobs.length) return 'settled ownership record leaked';
						if (timers.size) return 'lifetime timer leaked: ' + JSON.stringify(Array.from(timers.values()));
						var outcome = outcomes[target];
						if (!outcome) return 'real verifier did not return';
						if (cfg.Failure) {
							if (!outcome.cleanupFailed || outcome.worktreeDir !== paths[target] || warnings.length !== 1) return 'real cleanup failure lost';
							if (kind === 'repair') {
								var owner = prSplit._state.executionOutcomeCache;
								if (!owner.cleanupFailed || owner.worktreeDir !== paths[target] || owner.resourceFailure !== outcome ||
									owner.results !== committedRows) return 'stale repair failure lost original execution checkpoint';
							}
							if (warnings[0].fields.branch !== target || warnings[0].fields.worktreeDir !== paths[target]) return 'diagnostic lost original ownership';
							if (prints.length) return 'diagnostic printed into active alternate screen';
							globalThis.__postBubbleTeaExit();
							if (prints.length !== 1 || prints[0].active || prints[0].message.indexOf(paths[target]) < 0) return 'cleanup diagnostic was not printed after terminal restoration';
							globalThis.__postBubbleTeaExit();
							if (prints.length !== 1) return 'diagnostic was printed twice';
						} else {
							if (cfg.Action !== 'deadline' && (warnings.length || prints.length)) return 'clean completion emitted cleanup failure';
							if (kind === 'repair' && prSplit._state.executionOutcomeCache !== committedOutcome) return 'confirmed repair cleanup left provisional ownership behind';
						}
						var removalCount = removes.filter(function(path) { return path === paths[target]; }).length;
						if (removalCount !== (['tree', 'reject', 'reject_result'].indexOf(cfg.Failure) >= 0 ? 0 : 1)) return 'unsafe or missing worktree removal';
						if (cfg.Action !== 'replace' && shells.indexOf(target) >= 0) return 'cancelled setup launched verification';
						if (cfg.Action !== 'replace' && !s.wizardQuitSent) return 'completed owner did not release cancellation gate';
						return 'OK';
					} finally {
						release({code:0, signal:null});
						await Promise.all((s._resourceJobs || []).map(function(job) { return job.promise.catch(function() {}); }));
						for (var id of timers.keys()) realClearTimeout(id);
						globalThis.setTimeout = realSetTimeout; globalThis.clearTimeout = realClearTimeout;
						prSplit.verifySplitAsync = realVerify; prSplit._modules.exec.spawn = realSpawn;
						prSplit.resolveConflicts = realResolve;
						prSplit._state.executionOutcomeCache = oldOutcome; prSplit._state.executionResultCache = oldRows;
						prSplit.canSpawnInteractiveShell = oldCanSpawn; prSplit.startVerifySession = oldStart;
						log.warn = oldWarn; output.print = oldPrint; output._setTUIOutputActive = oldOutputActive;
						prSplit._tea.run = oldRun; globalThis.__postBubbleTeaExit = oldExit;
						prSplit._outputCaptureFn = oldCapture; prSplit._PANE_CLEANUP_TIMEOUT_MS = oldBudget;
						prSplit._handleConfigState = oldConfig; prSplit._state.agentExecutor = oldAgent;
						prSplit.automatedSplit = oldAutomated; prSplit._cancelSource = oldCancelSource;
						prSplit._tuiOutputActive = false; prSplit._verifyCleanupDiagnostics = [];
					}
				})()`, cfg, kind))
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

func TestVerifyWorktreeOwnerCallbackBeforeSetup(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript engine")
	}
	eval := prsplittest.NewTUIEngineWithHelpers(t)
	got, err := eval(`(async function() {
		var realSpawn = prSplit._modules.exec.spawn, spawned = 0;
		prSplit._modules.exec.spawn = function() { spawned++; throw new Error('unexpected native spawn'); };
		try {
			for (var reject of [false, true]) {
				var allocated = null;
				var result = await prSplit.verifySplitAsync('owned', {
					verifyCommand:'verify-command', isCancelled:function() { return false; },
					worktreeFn:function(path) {
						allocated = path;
						if (reject) return Promise.reject(new Error('owner rejected allocation'));
						throw new Error('owner rejected allocation');
					}
				});
				if (!allocated || !result.execFailed || result.error.indexOf('owner rejected allocation') < 0 ||
					spawned || result.worktreeRetained || result.cleanupFailed) return 'failed allocation callback orphaned or misreported a resource';
			}
			var cancelled = false;
			var result = await prSplit.verifySplitAsync('owned', {
				verifyCommand:'verify-command', isCancelled:function() { return cancelled; },
				worktreeFn:function() { cancelled = true; return Promise.resolve(); }
			});
			if (!result.cancelled || spawned) return 'cancellation during allocation notification still started Git';
			return 'OK';
		} finally { prSplit._modules.exec.spawn = realSpawn; }
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "OK" {
		t.Fatal(got)
	}
}

func TestVerifyQuitPathsJoinOwnedWork(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript TUI engine")
	}
	eval := prsplittest.NewTUIEngineWithHelpers(t)
	for _, mode := range []string{"pause", "abort", "done", "discard_key", "discard_mouse"} {
		t.Run(mode, func(t *testing.T) {
			got, err := eval(fmt.Sprintf(`(async function() {
				var mode = %q;
				var oldVerify = prSplit.verifySplitAsync, oldStart = prSplit.startVerifySession;
				var oldCanSpawn = prSplit.canSpawnInteractiveShell, oldQuit = prSplit._tea.quit;
				var oldEvidence = prSplit._agentEvidence;
				var quitCalls = 0, release;
				var pending = new Promise(function(resolve) { release = resolve; });
				prSplit._tea.quit = function() { quitCalls++; return 'quit'; };
				prSplit.canSpawnInteractiveShell = function() { return false; };
				prSplit.startVerifySession = function() { return {error:'textonly'}; };
				prSplit.verifySplitAsync = function(name, options) {
					if (name !== 'source') return Promise.resolve({passed:true, exitCode:0});
					options.worktreeFn('/owned/baseline');
					return pending;
				};
				setupPlanCache();
				prSplit._state.planCache.sourceBranch = 'source';
				prSplit._state.planCache.splits = [{name:'split/1', files:['a.go']}];
				prSplit.runtime.verifyCommand = 'verify-command';
				var s = initState('BRANCH_BUILDING');
				s.isProcessing = true; s.verifyingIdx = 0; s.verifyOutput = {}; s.verificationResults = [];
				try {
					prSplit._runVerifyBranch(s);
					await new Promise(function(resolve) { setTimeout(resolve, 0); });
					if (mode === 'pause') {
						s.wizard.pause(); s.wizardState = s.wizard.current;
						prSplit._handlePauseQuit(s);
					} else if (mode === 'abort') {
						s.wizard.transition('ERROR_RESOLUTION'); s.wizardState = s.wizard.current;
						prSplit._handleErrorResolutionChoice(s, 'abort');
					} else if (mode === 'done') {
						s.isProcessing = false;
						s.wizard.transition('EQUIV_CHECK'); s.wizard.transition('FINALIZATION'); s.wizardState = s.wizard.current;
						prSplit._handleNext(s);
					} else {
						s.wizard.error('reportClassification failed'); s.wizardState = s.wizard.current;
						s.errorDetails = 'reportClassification failed'; prSplit._agentEvidence = {};
						if (mode === 'discard_key') sendKey(s, 'd');
						else {
							var restoreZone = mockZoneHit('err-discard-session');
							try { prSplit._handleScreenMouseClick({type:'MouseClick'}, s); }
							finally { restoreZone(); }
						}
					}
					await new Promise(function(resolve) { setTimeout(resolve, 0); });
					var discard = mode.indexOf('discard') === 0;
					if (quitCalls || s.wizardQuitPaneReady || s.errorDiscardReady) return 'quit escaped pending owner';
					if (!s._resourceJobs[0].cancelled) return 'quit did not cancel owner';
					var first = prSplit._cancelOwnedWork(s);
					if (first !== prSplit._cancelOwnedWork(s)) return 'quit did not reuse cancellation join';
					release({passed:false, cleanupFailed:true, worktreeRetained:true,
						worktreeDir:'/owned/baseline', error:'termination failed; worktree retained at /owned/baseline'});
					await first;
					await new Promise(function(resolve) { setTimeout(resolve, 0); });
					if (discard ? !s.errorDiscardReady : !s.wizardQuitPaneReady) return 'completed owner did not release quit';
					update({type:'Tick', id:discard ? 'error-discard-quit' : 'wizard-quit'}, s);
					if (quitCalls !== 1) return 'quit was not delivered exactly once';
					return 'OK';
				} finally {
					release({passed:true});
					await Promise.all((s._resourceJobs || []).map(function(job) { return job.promise.catch(function() {}); }));
					prSplit.verifySplitAsync = oldVerify; prSplit.startVerifySession = oldStart;
					prSplit.canSpawnInteractiveShell = oldCanSpawn; prSplit._tea.quit = oldQuit;
					prSplit._agentEvidence = oldEvidence;
				}
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
