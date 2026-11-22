package command

import (
	"fmt"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

func TestRepairWorktreesPreserveNativeFailureOwnership(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript engine")
	}
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, t.TempDir())
	}
	engine := prsplittest.NewEngine(t, nil)
	engine.LoadChunks(t, "00_core", "04_validation", "06_verification", "08_conflict", "10a_pipeline_config")
	eval := engine.EvalJS(t)
	if _, err := eval(`
		prSplit.sendToHandle = async function() { return {error:null}; };
		prSplit.renderConflictPrompt = function() { return {text:'repair', error:null}; };
		globalThis.output = {print:function() {}};
	`); err != nil {
		t.Fatal(err)
	}
	engine.LoadChunks(t, "10c_pipeline_resolve")
	for _, agent := range []bool{false, true} {
		for _, phase := range []string{"setup", "repair", "status", "stage", "commit", "remove", "verify", "clean"} {
			for _, fault := range []string{"tree", "reject", "capture", "tree_capture", "exit", "reject_zero", "swallowed"} {
				if phase == "clean" && fault != "tree" {
					continue
				}
				if (fault == "exit" || fault == "reject_zero") && phase != "remove" {
					continue
				}
				if fault == "tree_capture" && phase != "setup" && phase != "repair" && phase != "remove" {
					continue
				}
				if fault == "swallowed" && (agent || phase != "repair") {
					continue
				}
				t.Run(fmt.Sprintf("agent=%t/%s/%s", agent, phase, fault), func(t *testing.T) {
					got, err := eval(fmt.Sprintf(`(async function() {
						var agent = %t, targetPhase = %q, fault = %q;
						var oldSpawn = prSplit._modules.exec.spawn;
						var oldExists = prSplit._modules.osmod.fileExists, oldWrite = prSplit._modules.osmod.writeFileScoped;
						var oldMcp = prSplit._mcpCallbackObj, oldAgent = prSplit._state.agentExecutor;
						prSplit.runtime.verifyCommand = 'verify-command';
						prSplit._modules.osmod.fileExists = async function() { return {exists:true}; };
						prSplit._modules.osmod.writeFileScoped = async function() {};
						prSplit._state.agentExecutor = {handle:{}};
						prSplit._mcpCallbackObj = {
							resetWaiter:function() {},
							waitForAsync:async function() {
								return {data:{patches:[{file:'a.go', content:'package a'}], commands:['fix-command']}, error:null};
							}
						};
						var calls = [], failedIndex = -1, faultPath = '', worktreePath = '', verifyCount = 0;
						function stream(text, truncated) {
							return {read:async function() {
								if (text) { var value = text; text = ''; return {done:false, value:value}; }
								return {done:true, truncated:truncated};
							}};
						}
						prSplit._modules.exec.spawn = async function(command, args) {
							var phase, text = '', code = 0, path = worktreePath;
							var worktreeIndex = args.indexOf('worktree');
							if (command === 'git') {
								if (worktreeIndex >= 0) {
									if (args[worktreeIndex + 1] === 'add') {
										path = args[args.length - 2];
										phase = path.indexOf('.osm-verify-') >= 0 ? 'reverify_setup' : 'setup';
										if (phase === 'setup') worktreePath = path;
									} else { phase = 'remove'; path = args[args.length - 1]; }
								} else if (args.indexOf('status') >= 0) { phase = 'status'; text = ' M go.mod\n'; }
								else if (args.indexOf('add') >= 0) phase = 'stage';
								else if (args.indexOf('commit') >= 0) phase = 'commit';
								else throw new Error('unexpected Git command: ' + args.join(' '));
							} else if (args.join(' ').indexOf('verify-command') >= 0) {
								phase = 'verify'; verifyCount++;
								if (!agent && verifyCount === 1) code = 1;
								if (agent) {
									var setup = calls.filter(function(call) { return call.phase === 'reverify_setup'; });
									if (setup.length) path = setup[setup.length - 1].path;
								}
							} else phase = 'repair';
							var hit = failedIndex < 0 && phase === targetPhase;
							calls.push({phase:phase, path:path});
							if (hit) {
								failedIndex = calls.length - 1; faultPath = path;
								if (fault === 'reject') throw Object.assign(new Error('native rollback failed'), {treeCleanupError:'tree close denied'});
								if (fault === 'reject_zero') throw Object.assign(new Error('worktree removal rejected'), {code:0});
								code = fault === 'exit' ? 128 : 0;
							}
							return {stdout:stream(text, hit && (fault === 'capture' || fault === 'tree_capture')), stderr:stream(hit && fault === 'exit' ? 'removal denied' : ''),
								wait:async function() { return {code:code, signal:null, treeCleanupError:hit && (fault === 'tree' || fault === 'tree_capture' || fault === 'swallowed') ? 'tree close denied' : undefined}; },
								kill:async function() {}};
						};
						try {
							var result;
							if (agent) {
								result = await prSplit.resolveConflictsWithAgent([{branch:'split/1', error:'test failed'}],
									'session', {resolve:1000, wallClockMs:10000}, 1, 2,
									{conflicts:[], resolutions:[], agentInteractions:0});
							} else {
								var strategy = prSplit.AUTO_FIX_STRATEGIES[0];
								if (fault === 'swallowed') strategy = {
									name:'catch-first-error', detect:function() { return true; },
									fix:async function(dir, branch, plan, verifyOutput, options) {
										try { await options.commands.shell('fix-command'); } catch (error) {}
										try { await options.commands.git(dir, ['commit', '--amend', '--no-edit']); } catch (error) {}
										return {fixed:true};
									}
								};
								result = await prSplit.resolveConflicts({dir:'.', sourceBranch:'source',
									verifyCommand:'verify-command', splits:[{name:'split/1', files:['a.go']} ]},
									{strategies:[strategy], retryBudget:2});
							}
							if (targetPhase === 'clean') {
								if (result.error || (!agent && result.fixed.length !== 1)) return 'clean repair failed: ' + JSON.stringify(result);
								return 'OK';
							}
							if (failedIndex < 0) return 'fault was not reached: ' + JSON.stringify(calls);
							var failure = agent ? result : result.errors[0];
							if (!failure || !failure.error || (!agent && result.fixed.length)) return 'fault reported successful repair: ' + JSON.stringify(result);
							var later = calls.slice(failedIndex + 1);
							var treeFailed = ['tree', 'tree_capture', 'reject', 'swallowed'].indexOf(fault) >= 0;
							if (treeFailed) {
								if (later.length) return 'command ran after tree failure: ' + JSON.stringify(later);
								if (!failure.cleanupFailed || failure.treeCleanupError !== 'tree close denied' ||
									failure.worktreeDir !== faultPath) return 'cleanup failure metadata lost: ' + JSON.stringify(result);
								if (!!failure.worktreeRetained !== (targetPhase !== 'remove')) return 'removal attempt confused with withheld removal';
							} else if (later.some(function(call) { return call.phase !== 'remove'; })) {
								return 'mutation continued after unknown capture: ' + JSON.stringify(later);
							}
							if ((fault === 'exit' || fault === 'reject_zero') &&
								(!failure.cleanupFailed || failure.worktreeRetained || failure.worktreeDir !== faultPath)) return 'failed removal was not reported as attempted';
							if (fault === 'tree_capture' && targetPhase !== 'remove' && !failure.truncated) return 'combined tree and capture failure lost capture fact';
							return 'OK';
						} finally {
							prSplit._modules.exec.spawn = oldSpawn;
							prSplit._modules.osmod.fileExists = oldExists; prSplit._modules.osmod.writeFileScoped = oldWrite;
							prSplit._mcpCallbackObj = oldMcp; prSplit._state.agentExecutor = oldAgent;
						}
					})()`, agent, phase, fault))
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

func TestHeuristicFallbackPreservesResourceOwnership(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript engine")
	}
	eval := prsplittest.NewChunkEngine(t, nil, "00_core", "10a_pipeline_config", "10c_pipeline_resolve")
	for _, phase := range []string{"verify", "repair", "pending", "clean"} {
		for _, field := range []string{"cleanupFailed", "treeCleanupError", "worktreeRetained"} {
			if (phase == "pending" || phase == "clean") && field != "cleanupFailed" {
				continue
			}
			t.Run(phase+"/"+field, func(t *testing.T) {
				got, err := eval(fmt.Sprintf(`(async function() {
					var phase = %q, field = %q, state = prSplit._state;
					var names = ['applyStrategy', 'createSplitPlanAsync', 'executeSplitAsync', 'verifySplitsAsync', 'verifyEquivalenceAsync', 'resolveConflicts'];
					var saved = names.map(function(name) { return prSplit[name]; });
					var oldOutcome = state.executionOutcomeCache, oldRows = state.executionResultCache;
					var oldDryRun = prSplit.runtime.dryRun, oldOutput = globalThis.output;
					globalThis.output = {print:function() {}};
					prSplit.runtime.dryRun = false;
					var existing = {marker:'current execution owner'}, existingRows = [{name:'original', sha:'original-sha'}];
					state.executionOutcomeCache = existing; state.executionResultCache = existingRows;
					var rows = [{name:'split/1', sha:'committed-sha', error:null}];
					var executed = {error:null, results:rows};
					var failure = {name:'split/1', passed:false, error:'original resource failure',
						worktreeDir:'/original/owned-worktree', exitCode:0, truncated:false};
					failure[field] = field === 'treeCleanupError' ? 'original tree failure' : true;
					var verified = {allPassed:false, error:null, results:[phase === 'verify' ? failure : {name:'split/1', passed:phase === 'clean', error:'ordinary test failure'}]};
					var resolved = {fixed:[], errors:[failure], reSplitNeeded:true};
					var verifyCalls = 0, repairCalls = 0, equivCalls = 0;
					prSplit.applyStrategy = async function() { return {all:['a.go']}; };
					prSplit.createSplitPlanAsync = async function() {
						return {baseBranch:'main', sourceBranch:'feature', splits:[{name:'split/1', files:['a.go']}]};
					};
					prSplit.executeSplitAsync = async function() {
						return phase === 'pending' ? {error:'execution pending', executionPending:true, results:[]} : executed;
					};
					prSplit.verifySplitsAsync = async function() { verifyCalls++; return verified; };
					prSplit.resolveConflicts = async function() { repairCalls++; return resolved; };
					prSplit.verifyEquivalenceAsync = async function() { equivCalls++; return {equivalent:true}; };
					try {
						var result = await prSplit.heuristicFallback({files:['a.go'], fileStatuses:{'a.go':'M'},
							baseBranch:'main', currentBranch:'feature'}, {strategy:'directory'}, {});
						if (phase === 'pending') {
							if (!result.error || state.executionOutcomeCache !== existing || state.executionResultCache !== existingRows || verifyCalls || repairCalls || equivCalls) return 'admission refusal replaced the current execution owner';
							return 'OK';
						}
						if (result.report.execution !== executed || state.executionResultCache !== rows) return 'later resource outcome replaced committed branch facts';
						if (phase === 'clean') {
							if (result.error || state.executionOutcomeCache !== executed || equivCalls !== 1 || repairCalls) return 'clean heuristic flow regressed';
							return 'OK';
						}
						var source = phase === 'verify' ? verified : resolved;
						var owner = state.executionOutcomeCache;
						if (!result.error || owner === executed || owner.resourceFailure !== source || result.report.resourceFailure !== owner ||
							owner.results !== rows || owner.worktreeDir !== failure.worktreeDir || owner[field] !== failure[field] ||
							owner.exitCode !== 0 || owner.truncated !== false || equivCalls) return 'resource facts did not survive heuristic reporting/checkpoint';
						if (result.report.verification !== verified || (phase === 'verify' && repairCalls)) return 'unsafe repair followed verification ownership failure';
						if (phase === 'repair' && (result.report.resolution !== resolved || result.report.resolutionFailures[0] !== failure || repairCalls !== 1)) return 'complete repair result was not reported';
						if (!prSplit._executionCleanupFailure(rows, owner)) return 'resource failure would permit later execution';
						return 'OK';
					} finally {
						for (var i=0; i<names.length; i++) prSplit[names[i]] = saved[i];
						state.executionOutcomeCache = oldOutcome; state.executionResultCache = oldRows;
						prSplit.runtime.dryRun = oldDryRun; globalThis.output = oldOutput;
					}
				})()`, phase, field))
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
