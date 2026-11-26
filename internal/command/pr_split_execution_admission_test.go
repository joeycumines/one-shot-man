package command

import (
	"fmt"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// Agent crash recovery is a real second entry point: the auto pipeline can
// still own native Git work when the restart handler requests execution again.
func TestExecutionAdmission_ConfirmsOwnerBeforeAgentRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript TUI engine")
	}
	eval := prsplittest.NewTUIEngineWithHelpers(t)
	for _, method := range []string{"executeSplit", "executeSplitAsync"} {
		for _, phase := range []string{"setup", "cleanup"} {
			for _, treeFailure := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/tree=%t", method, phase, treeFailure), func(t *testing.T) {
					got, err := eval(fmt.Sprintf(`(async function() {
						var method = %q, phase = %q, treeFailure = %t;
						var native = prSplit._modules.exec, oldSpawn = native.spawn, oldExecv = native.execv;
						var oldAgent = prSplit._state.agentExecutor, oldOutcome = prSplit._state.executionOutcomeCache;
						var oldRows = prSplit._state.executionResultCache, oldVerify = prSplit.runtime.verifyCommand;
						var calls = [], entered, release, held = false, path = '', restartCalls = 0;
						var ready = new Promise(function(resolve) { entered = resolve; });
						var gate = new Promise(function(resolve) { release = resolve; });
						async function command(args) {
							if (args[0] === '-C') args = args.slice(2);
							calls.push(args);
							if (args[0] === 'worktree' && args[1] === 'add') path = args[3];
							var result = {code:0, stdout:'', stderr:'', error:false, truncated:false};
							if (args[0] === 'check-ignore') { result.code = 1; result.error = true; }
							if (args[0] === 'rev-parse') {
								if (args[1] === 'HEAD') result.stdout = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa';
								else { result.code = 128; result.error = true; }
							}
							if (!held && args[0] === 'worktree' && args[1] === (phase === 'setup' ? 'add' : 'remove')) {
								held = true; entered(); await gate;
								if (treeFailure) { result.treeCleanupError = 'original owner tree close failed'; result.error = true; }
							}
							return result;
						}
						native.execv = async function(argv) { return await command(argv.slice(1)); };
						native.spawn = async function(name, args) {
							var result = await command(args), sent = false;
							return {stdout:{read:async function() {
								if (!sent && result.stdout) { sent = true; return {done:false, value:result.stdout}; }
								return {done:true};
							}}, stderr:{read:async function() { return {done:true}; }},
								wait:async function() { return {code:result.code, signal:null, treeCleanupError:result.treeCleanupError}; }};
						};
						setupPlanCache();
						prSplit._state.planCache.splits = [{name:'split/1', files:['a.go']}];
						prSplit._state.planCache.fileStatuses = {'a.go':'M'};
						var sentinel = {marker:'original cache'};
						prSplit._state.executionOutcomeCache = sentinel;
						prSplit._state.executionResultCache = [];
						prSplit.runtime.verifyCommand = '';
						var original = prSplit[method](prSplit._state.planCache).then(function(result) {
							prSplit._state.executionOutcomeCache = result;
							prSplit._state.executionResultCache = result.results || [];
							return result;
						});
						try {
							await ready;
							if (!prSplit._executionPending()) return 'native execution was not owned';
							var before = calls.length;
							prSplit._state.agentExecutor = {handle:{isAlive:function() { return false; }},
								restart:async function() { restartCalls++; return {error:null}; }};
							var s = initState('CONFIG');
							s.wizard.transition('PLAN_GENERATION'); s.wizardState = 'PLAN_GENERATION';
							s.isProcessing = true; s.autoSplitRunning = true;
							prSplit._handleAutoSplitPoll(s);
							if (s.wizardState !== 'ERROR_RESOLUTION') return 'agent crash recovery path was not reached';
							prSplit._handleErrorResolutionChoice(s, 'restart-agent');
							await new Promise(function(resolve) { setTimeout(resolve, 0); });
							if (restartCalls || calls.length !== before || s.wizardState !== 'ERROR' || s.wizard.current !== 'ERROR' || s.errorDetails.indexOf('still running') < 0) return 'restart admission invariant failed: ' + JSON.stringify({restartCalls:restartCalls, commandCount:calls.length, expectedCommandCount:before, wizardState:s.wizardState, wizardCurrent:s.wizard.current, errorDetails:s.errorDetails, calls:calls});
							if (prSplit._state.executionOutcomeCache !== sentinel || prSplit._state.executionResultCache.length) return 'restart replaced the pending owner cache';
							var other = method === 'executeSplit' ? 'executeSplitAsync' : 'executeSplit';
							var refused = await prSplit[other](prSplit._state.planCache);
							if (!refused.executionPending || !refused.error || calls.length !== before) return 'public entry points did not share admission';
							release();
							var outcome = await original;
							if (prSplit._executionPending() || prSplit._state.executionOutcomeCache !== outcome) return 'finished owner did not publish or release';
							if (treeFailure) {
								if (!outcome.cleanupFailed || outcome.worktreeDir !== path || outcome.treeCleanupError !== 'original owner tree close failed' ||
									!!outcome.worktreeRetained !== (phase === 'setup')) return 'owner cleanup facts lost';
								before = calls.length;
								var blocked = await prSplit[other](prSplit._state.planCache, {completedOutcome:outcome});
								if (!blocked.cleanupFailed || calls.length !== before || prSplit._executionPending()) return 'unresolved owner permitted retry or leaked admission';
							} else {
								if (outcome.error) return 'clean owner failed';
								var next = await prSplit[other](prSplit._state.planCache);
								if (next.error || next.executionPending || prSplit._executionPending()) return 'clean subsequent execution was blocked';
								prSplit._handleErrorResolutionChoice(s, 'restart-agent');
								await new Promise(function(resolve) { setTimeout(resolve, 0); });
								if (restartCalls !== 1 || s.agentRestarting || !s.restartResult || s.restartResult.error) return 'released owner still blocked successful agent recovery';
							}
							return 'OK';
						} finally {
							release(); await original;
							native.spawn = oldSpawn; native.execv = oldExecv;
							prSplit._state.agentExecutor = oldAgent; prSplit._state.executionOutcomeCache = oldOutcome;
							prSplit._state.executionResultCache = oldRows; prSplit.runtime.verifyCommand = oldVerify;
						}
					})()`, method, phase, treeFailure))
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
}

// Branch execution can finish before the automated owner finishes verification,
// repair, or its final checkpoint. The same real crash/restart route must stay
// blocked across those later awaits, including direct public execution calls.
func TestExecutionAdmission_PipelineOwnsPostExecutionWork(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript TUI engine")
	}
	eval := prsplittest.NewTUIEngineWithHelpers(t)
	for _, phase := range []string{"verify", "repair", "final_cleanup"} {
		t.Run(phase, func(t *testing.T) {
			got, err := eval(fmt.Sprintf(`(async function() {
				var phase = %q, p = prSplit, state = p._state;
				var names = ['resolvePlanPath', 'savePlan', 'analyzeDiffAsync', 'AgentCodeExecutor',
					'renderClassificationPrompt', 'sendToHandle', 'waitForLogged', 'classificationToGroups',
					'createSplitPlanAsync', 'validateClassification', 'validatePlan', 'executeSplitAsync',
					'verifySplitsAsync', 'resolveConflictsWithAgent', 'cleanupExecutor', 'assessIndependence'];
				var saved = names.map(function(name) { return p[name]; });
				var savedState = Object.assign({}, state), oldMcp = p._mcpCallbackObj;
				var oldAgentPointer = p._agentExecutor, oldRequire = require, oldRead = p._modules.osmod.readFile;
				var oldMode = p.runtime.mode, oldCancelSource = p._cancelSource;
				var originalExecute = p.executeSplit, originalAsyncExecute = p.executeSplitAsync;
				var entered, release, pipeline, agentAlive = true, executionCalls = 0, resolveCalls = 0, restartCalls = 0;
				var ready = new Promise(function(resolve) { entered = resolve; });
				var gate = new Promise(function(resolve) { release = resolve; });
				var rows = [{name:'split/1', sha:'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'}];
				var executed = {error:null, results:rows};
				var failure = {name:'split/1', passed:false, exitCode:0, truncated:false,
					error:'original post-execution owner failed', execFailed:true, cleanupFailed:true,
					treeCleanupError:'original tree close failed', worktreeRetained:true, worktreeDir:'/original/post-execution-owner'};
				var plan = {dir:'.', baseBranch:'main', sourceBranch:'feature', verifyCommand:'verify',
					fileStatuses:{'a.go':'M'}, splits:[{name:'split/1', files:['a.go'], message:'split', order:0}]};
				var mcp = {addTool:function() {}, init:async function() {}, resetWaiter:function() {},
					mcpConfigPath:'mock.json', close:async function() {
						if (phase === 'final_cleanup') { entered(); await gate; }
					}};
				require = function(name) {
					if (name === 'osm:mcp') return {createServer:function() { return {}; }};
					if (name === 'osm:mcpcallback') return {MCPCallback:function() { return mcp; }};
					return oldRequire(name);
				};
				p._modules.osmod.readFile = async function() { return {content:'{}'}; };
				p.resolvePlanPath = function() { return 'mock-plan.json'; };
				p.savePlan = async function() { return {}; };
				p.analyzeDiffAsync = async function() { return {files:['a.go'], baseBranch:'main', currentBranch:'feature', fileStatuses:{'a.go':'M'}, fileRenames:{}}; };
				p.AgentCodeExecutor = function() {
					this.handle = {isAlive:function() { return agentAlive; }};
					this.resolveAsync = async function() { return {}; };
					this.spawn = async function() { return {sessionId:1}; };
					this.restart = async function() { restartCalls++; return {error:null}; };
				};
				p.renderClassificationPrompt = function() { return {text:'classify'}; };
				p.sendToHandle = async function() { return {}; };
				p.waitForLogged = async function(name) { return name === 'reportClassification' ? {data:{'a.go':'changes'}} : {error:'no optional plan'}; };
				p.classificationToGroups = function() { return {}; };
				p.createSplitPlanAsync = async function() { return plan; };
				p.validateClassification = p.validatePlan = function() { return {valid:true}; };
				p.executeSplitAsync = async function(_plan, options) {
					executionCalls++;
					if (!p._ownsPipeline(options.pipelineOwner)) throw new Error('automated execution lost its owner');
					return executed;
				};
				p.verifySplitsAsync = async function() {
					if (phase === 'verify') { entered(); await gate; }
					return phase === 'repair' ? {allPassed:false, results:[{name:'split/1', passed:false}]} :
						{error:'verification failed', allPassed:false, results:[failure]};
				};
				p.resolveConflictsWithAgent = async function() { resolveCalls++; entered(); await gate; return failure; };
				p.cleanupExecutor = async function() {};
				p.assessIndependence = async function() { return []; };
				state.executionOutcomeCache = null; state.executionResultCache = [];
				try {
					pipeline = p.automatedSplit({disableTUI:true, cleanupOnFailure:true, maxReSplits:0,
						pipelineTimeoutMs:60000, stepTimeoutMs:60000, watchdogIdleMs:60000});
					await ready;
					if (!p._pipelinePending() || p._executionPending()) return 'post-execution lifetime was not separately owned';
					var before = state.executionOutcomeCache;
					agentAlive = false;
					var s = initState('CONFIG');
					s.wizard.transition('PLAN_GENERATION'); s.wizardState = 'PLAN_GENERATION';
					s.isProcessing = true; s.autoSplitRunning = true;
					p._handleAutoSplitPoll(s);
					if (s.wizardState !== 'ERROR_RESOLUTION') return 'post-execution agent crash path was not reached';
					p._handleErrorResolutionChoice(s, 'restart-agent');
					await new Promise(function(resolve) { setTimeout(resolve, 0); });
					if (s.wizardState !== 'ERROR' || s.wizard.current !== 'ERROR' || s.errorDetails.indexOf('still running') < 0 ||
						restartCalls || executionCalls !== 1 || state.executionOutcomeCache !== before) return 'restart admission invariant failed: ' + JSON.stringify({restartCalls:restartCalls, executionCalls:executionCalls, sameOwner:state.executionOutcomeCache === before, wizardState:s.wizardState, wizardCurrent:s.wizard.current, errorDetails:s.errorDetails});
					var fallback = initState('CONFIG');
					fallback.wizard.transition('PLAN_GENERATION'); fallback.wizardState = 'PLAN_GENERATION';
					fallback.isProcessing = true; fallback.autoSplitRunning = true;
					p._handleAutoSplitPoll(fallback);
					if (fallback.wizardState !== 'ERROR_RESOLUTION' || !fallback.agentCrashDetected) return 'fallback agent crash path was not reached';
					var beforeMode = p.runtime.mode, beforeCancelSource = p._cancelSource;
					var beforeAnalysis = state.analysisCache, beforeGroups = state.groupsCache;
					var beforePlan = state.planCache, beforeRows = state.executionResultCache;
					var beforeVerifyEpoch = fallback._verifyRunEpoch, beforeConfigEpoch = fallback._analysisConfigEpoch;
					p._handleErrorResolutionChoice(fallback, 'fallback-heuristic');
					await new Promise(function(resolve) { setTimeout(resolve, 0); });
					if (fallback.wizardState !== 'ERROR' || fallback.wizard.current !== 'ERROR' || fallback.errorDetails.indexOf('still running') < 0 ||
						!fallback.agentCrashDetected || fallback._verifyRunEpoch !== beforeVerifyEpoch ||
						fallback._analysisConfigEpoch !== beforeConfigEpoch || p.runtime.mode !== beforeMode ||
						p._cancelSource !== beforeCancelSource || state.analysisCache !== beforeAnalysis ||
						state.groupsCache !== beforeGroups || state.planCache !== beforePlan ||
						state.executionResultCache !== beforeRows || state.executionOutcomeCache !== before ||
						executionCalls !== 1) return 'fallback replaced pending analysis/plan/verification ownership';
					// Direct callers must also be refused before installing a new
					// cancellation source or admitting asynchronous config work.
					var analysis = initState('CONFIG');
					beforeCancelSource = p._cancelSource;
					beforeConfigEpoch = analysis._analysisConfigEpoch;
					p._startAnalysis(analysis);
					await new Promise(function(resolve) { setTimeout(resolve, 0); });
					if (analysis.wizardState !== 'ERROR' || analysis.wizard.current !== 'ERROR' || analysis.errorDetails.indexOf('still running') < 0 ||
						p._cancelSource !== beforeCancelSource || analysis._analysisConfigEpoch !== beforeConfigEpoch ||
						state.analysisCache !== beforeAnalysis || state.groupsCache !== beforeGroups ||
						state.planCache !== beforePlan || state.executionResultCache !== beforeRows ||
						state.executionOutcomeCache !== before) return 'direct analysis bypassed pending pipeline admission';
					var refused = await originalExecute(plan);
					var refusedAsync = await originalAsyncExecute(plan);
					var refusedPipeline = await p.automatedSplit({});
					var refusedFallback = await p.heuristicFallback({}, {}, {});
					if (!refused.executionPending || !refusedAsync.executionPending || !refusedPipeline.executionPending ||
						!refusedFallback.executionPending || state.executionOutcomeCache !== before || !p._pipelinePending()) return 'public entry point bypassed or released another owner';
					release();
					var result = await pipeline, owner = state.executionOutcomeCache;
					if (p._pipelinePending() || p._executionPending() || !result.error || !owner ||
						owner.worktreeDir !== failure.worktreeDir || owner.treeCleanupError !== failure.treeCleanupError ||
						owner.results[0].sha !== rows[0].sha || result.report.execution !== executed) return 'settled pipeline lost its original branch/resource facts';
					if ((phase === 'repair') !== (resolveCalls === 1)) return 'unexpected repair stage count';
					var next = p._claimPipelineOwner();
					if (!next) return 'completed pipeline retained admission';
					p._releasePipelineOwner(next);
					return 'OK';
				} finally {
					release(); if (pipeline) await pipeline;
					for (var i=0; i<names.length; i++) p[names[i]] = saved[i];
					Object.keys(state).forEach(function(key) { delete state[key]; }); Object.assign(state, savedState);
					p._mcpCallbackObj = oldMcp; p._agentExecutor = oldAgentPointer;
					p.runtime.mode = oldMode; p._cancelSource = oldCancelSource;
					require = oldRequire; p._modules.osmod.readFile = oldRead;
				}
			})()`, phase))
			if err != nil {
				t.Fatal(err)
			}
			if got != "OK" {
				t.Fatal(got)
			}
		})
	}
}
