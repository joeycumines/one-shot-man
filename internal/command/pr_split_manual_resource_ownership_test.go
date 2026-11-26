package command

import (
	"fmt"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

func TestManualVerificationAndRepairPreserveResourceOwnership(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript engine")
	}
	eval := prsplittest.NewTUIEngine(t)
	for _, command := range []string{"verify", "fix"} {
		for _, shape := range []string{"top", "row", "reject", "empty_rows", "prior", "pending", "clean"} {
			for _, field := range []string{"cleanupFailed", "treeCleanupError", "worktreeRetained"} {
				if (shape == "prior" || shape == "pending" || shape == "clean") && field != "cleanupFailed" {
					continue
				}
				t.Run(command+"/"+shape+"/"+field, func(t *testing.T) {
					got, err := eval(fmt.Sprintf(`(async function() {
						var command = %q, shape = %q, field = %q, state = prSplit._state;
						var oldPlan = state.planCache, oldRows = state.executionResultCache, oldOutcome = state.executionOutcomeCache;
						var oldResolve = prSplit.resolveConflicts, oldVerify = prSplit.verifySplits, oldPrint = output.print;
						var oldSpawn = prSplit._modules.exec.spawn, oldExecv = prSplit._modules.exec.execv;
						var rows = shape === 'empty_rows' ? [] : [{name:'split/1', sha:'committed-sha', error:null}];
						var existing = {results:rows, error:null, marker:'committed branch outcome'};
						var plan = {dir:'.', sourceBranch:'source', baseBranch:'main', splits:[{name:'split/1', files:['a.go']}]};
						var failure = {name:'split/1', error:'original resource failure',
							worktreeDir:'/owned/original-worktree', exitCode:0, truncated:false};
						failure[field] = field === 'treeCleanupError' ? 'original native close failed' : true;
						var result = command === 'fix'
							? {fixed:[], errors:[failure], reSplitNeeded:false}
							: {allPassed:false, results:[failure]};
						if (shape === 'top' || shape === 'empty_rows') Object.assign(result, failure);
						var rejection = Object.assign(new Error('native close rejected'), failure);
						delete rejection.error;
						if (shape === 'clean' || shape === 'pending') result = command === 'fix'
							? {fixed:[], errors:[], reSplitNeeded:false}
							: {allPassed:true, results:[{name:'split/1', passed:true}]};
						state.planCache = plan; state.executionResultCache = rows;
						state.executionOutcomeCache = shape === 'prior' ? Object.assign({}, failure, {results:rows}) : existing;
						var prior = state.executionOutcomeCache, lines = [], calls = 0, nativeCalls = 0;
						output.print = function(line) { lines.push(String(line)); };
						prSplit._modules.exec.spawn = prSplit._modules.exec.execv = async function() {
							nativeCalls++; throw new Error('unexpected native mutation');
						};
						var release, entered;
						var gate = new Promise(function(resolve) { release = resolve; });
						var entry = new Promise(function(resolve) { entered = resolve; });
						prSplit.resolveConflicts = prSplit.verifySplits = async function(p, options) {
							calls++;
							if (!prSplit._pipelinePending()) throw new Error('manual command did not hold resource ownership');
							if (command === 'fix' && !prSplit._ownsPipeline(options.pipelineOwner)) throw new Error('nested repair did not borrow manual command owner');
							entered();
							if (shape === 'pending') await gate;
							if (shape === 'reject') throw rejection;
							return result;
						};
						var pending;
						try {
							var commands = prSplit._buildCoreCommands({});
							pending = commands[command].handler();
							if (shape === 'pending') {
								await Promise.race([entry, pending]);
								var blocked = await prSplit.executeSplit(plan);
								if (!blocked.executionPending || nativeCalls) return 'execution entered during manual native work';
								await commands[command].handler();
								if (calls !== 1 || state.executionOutcomeCache !== existing) return 'second manual command replaced the active owner';
								release();
							}
							var thrown;
							try { await pending; } catch (error) { thrown = error; }
							if (shape === 'reject' ? thrown !== rejection : !!thrown) return 'manual command changed rejection identity: ' + thrown;
							if (prSplit._pipelinePending()) return 'manual command leaked its pipeline owner';
							if (state.executionResultCache !== rows || (rows.length && rows[0].sha !== 'committed-sha')) return 'manual resource outcome replaced committed branch facts';
							if (shape === 'prior') {
								if (calls || state.executionOutcomeCache !== prior || lines.join('\n').indexOf(failure.worktreeDir) < 0) return 'prior unresolved cleanup did not block repair/verification';
								return 'OK';
							}
							if (shape === 'clean' || shape === 'pending') {
								if (calls !== 1 || state.executionOutcomeCache !== existing) return 'clean command changed execution checkpoint';
								return 'OK';
							}
							var owner = state.executionOutcomeCache;
							if (owner === existing || !owner.resourceFailure || owner[field] !== failure[field] ||
								owner.worktreeDir !== failure.worktreeDir || owner.exitCode !== 0 || owner.truncated !== false ||
								JSON.stringify(owner.results) !== JSON.stringify(rows)) return 'resource metadata was not retained: ' + JSON.stringify(owner);
							if (shape === 'reject' && owner.error !== 'native close rejected') return 'rejection diagnostic was lost';
							if (prSplit._buildReport().execution !== owner) return 'report omitted manual resource ownership';
							var resumed = await prSplit.executeSplit(plan, {completedResults:rows, completedOutcome:owner});
							if (!resumed.error || resumed.error.indexOf(failure.worktreeDir) < 0 || nativeCalls) return 'later execution ignored manual resource ownership';
							await commands[command].handler();
							if (calls !== 1 || state.executionOutcomeCache !== owner) return 'manual retry discarded unresolved cleanup';
							return 'OK';
						} finally {
							release();
							if (pending) { try { await pending; } catch (error) {} }
							state.planCache = oldPlan; state.executionResultCache = oldRows; state.executionOutcomeCache = oldOutcome;
							prSplit.resolveConflicts = oldResolve; prSplit.verifySplits = oldVerify; output.print = oldPrint;
							prSplit._modules.exec.spawn = oldSpawn; prSplit._modules.exec.execv = oldExecv;
						}
					})()`, command, shape, field))
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

func TestLocalRepairRetainsPipelineOwnerUntilCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript engine")
	}
	eval := prsplittest.NewChunkEngine(t, nil, "00_core", "04_validation", "05_execution", "08_conflict")
	for _, phase := range []string{"allocation", "setup", "verify", "remove", "cancel_allocation", "cancel_setup", "borrowed", "wrong_owner", "prior"} {
		t.Run(phase, func(t *testing.T) {
			got, err := eval(fmt.Sprintf(`(async function() {
				var phase = %q, calls = [], release, entered, cancelled = false, borrowed;
				var oldGit = prSplit._gitExecAsync, oldShell = prSplit._shellExecAsync, oldPath = prSplit._worktreeTmpPath;
				var plan = {dir:'.', verifyCommand:'verify-command', splits:[{name:'split/1', files:['a.go']}]};
				var gate = new Promise(function(resolve) { release = resolve; });
				var entry = new Promise(function(resolve) { entered = resolve; });
				function result() { return {code:0, stdout:'', stderr:'', truncated:false, error:false}; }
				async function checkpoint(name) {
					calls.push(name);
					if (name === phase || (phase === 'cancel_allocation' && name === 'allocation') || (phase === 'cancel_setup' && name === 'setup')) { entered(); await gate; }
					return result();
				}
				prSplit._worktreeTmpPath = function() { return '/owned/repair-worktree'; };
				prSplit._gitExecAsync = async function(dir, args) { return await checkpoint(args[1] === 'add' ? 'setup' : 'remove'); };
				prSplit._shellExecAsync = async function() { return await checkpoint('verify'); };
				var options = {isCancelled:function() { return cancelled; }, worktreeFn:async function(path, branch) {
					if (path !== '/owned/repair-worktree' || branch !== 'split/1') throw new Error('allocation identity missing');
					await checkpoint('allocation');
				}};
				if (phase === 'borrowed' || phase === 'wrong_owner') {
					borrowed = prSplit._claimPipelineOwner();
					options.pipelineOwner = phase === 'borrowed' ? borrowed : {};
				}
				if (phase === 'prior') options.completedOutcome = {cleanupFailed:true, worktreeDir:'/older/owned-worktree', error:'prior cleanup failed', results:[]};
				var pending;
				try {
					pending = prSplit.resolveConflicts(plan, options);
					if (['borrowed', 'wrong_owner', 'prior'].indexOf(phase) < 0) {
						await Promise.race([entry, pending]);
						var count = calls.length;
						if (!prSplit._pipelinePending()) return 'repair released owner before ' + phase;
						var blocked = await prSplit.executeSplit(plan);
						var second = await prSplit.resolveConflicts(plan);
						if (!blocked.executionPending || !second.executionPending || calls.length !== count) return 'second mutation entered during ' + phase;
						if (phase === 'cancel_allocation' || phase === 'cancel_setup') cancelled = true;
						release();
					}
					var resolved = await pending;
					if (phase === 'wrong_owner') {
						if (!resolved.executionPending || calls.length || !prSplit._ownsPipeline(borrowed)) return 'foreign caller borrowed or released live owner';
						return 'OK';
					}
					if (phase === 'prior') {
						if (!resolved.cleanupFailed || resolved.worktreeDir !== '/older/owned-worktree' || calls.length) return 'repair disregarded previous cleanup';
					} else if (phase === 'cancel_allocation') {
						if (!resolved.cancelled || calls.join(',') !== 'allocation') return 'cancelled allocation still created/removed a worktree: ' + JSON.stringify(resolved) + ' ' + calls;
					} else if (phase === 'cancel_setup') {
						if (!resolved.cancelled || calls.join(',') !== 'allocation,setup,remove') return 'cancelled setup started verification or skipped cleanup: ' + JSON.stringify(resolved) + ' ' + calls;
					} else if (resolved.error || calls.join(',') !== 'allocation,setup,verify,remove') return 'clean repair lifecycle changed: ' + JSON.stringify(resolved) + ' ' + calls;
					if (phase === 'borrowed') {
						if (!prSplit._ownsPipeline(borrowed)) return 'nested repair released caller owner';
					} else if (prSplit._pipelinePending()) return 'completed repair leaked pipeline owner';
					return 'OK';
				} finally {
					release();
					if (pending) await pending;
					if (borrowed) prSplit._releasePipelineOwner(borrowed);
					prSplit._gitExecAsync = oldGit; prSplit._shellExecAsync = oldShell; prSplit._worktreeTmpPath = oldPath;
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
