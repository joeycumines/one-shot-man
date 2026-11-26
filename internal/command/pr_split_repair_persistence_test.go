package command

import (
	"fmt"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

func TestSavePlan_OrdersSnapshotsAndRecoversAfterWriteFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript engine")
	}
	eval := prsplittest.NewChunkEngine(t, nil, "00_core", "03_planning")
	got, err := eval(`(async function() {
		var p = prSplit, native = p._modules.osmod, oldWrite = native.writeFile;
		var entered, reject, calls = [], stored = {};
		var ready = new Promise(function(resolve) { entered = resolve; });
		var gate = new Promise(function(resolve, rejectFn) { reject = rejectFn; });
		native.writeFile = async function(path, content) {
			calls.push(JSON.parse(content));
			if (calls.length === 1) { entered(); await gate; }
			stored[path] = JSON.parse(content);
		};
		try {
			p._state.planCache = {marker:'old', splits:[]};
			var first = p.savePlan(null, 'old');
			await ready;
			var replacement = {marker:'new', splits:[]};
			p._state.planCache = replacement;
			var second = p.savePlan(null, 'new');
			await Promise.resolve();
			if (calls.length !== 1) return 'newer snapshot wrote before older pending write';
			reject(new Error('first write denied'));
			var firstResult = await first, secondResult = await second;
			if (!firstResult.error || secondResult.error || calls.length !== 2 || stored[secondResult.path].plan.marker !== 'new') return 'write failure poisoned or reordered successor';
			var explicit = {planCache:{marker:'captured', splits:[]}, executionResultCache:[{name:'original'}],
				executionOutcomeCache:{cleanupFailed:true, worktreeDir:'/original/worktree'},
				runtime:{baseBranch:'original-base', verifyCommand:'original-command'}};
			var third = await p.savePlan(null, 'captured', explicit);
			var saved = stored[third.path];
			if (third.error || saved.plan.marker !== 'captured' || saved.runtime.baseBranch !== 'original-base' ||
				saved.runtime.verifyCommand !== 'original-command' || saved.executed[0].name !== 'original' ||
				p._state.planCache !== replacement) return 'explicit snapshot changed live state or lost original runtime';
			return 'OK';
		} finally {
			reject(new Error('release'));
			await Promise.all([first, second].filter(function(value) { return !!value; }));
			native.writeFile = oldWrite;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "OK" {
		t.Fatal(got)
	}
}

// Use the real TUI repair, native-result handling, save/load serializer, and
// execution admission. Native children and the filesystem transport are the
// only substituted boundaries; the saved bytes are exactly what load consumes.
func TestRepairCleanupCheckpointSurvivesQuitAndResume(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript TUI engine")
	}
	eval := prsplittest.NewTUIEngineWithHelpers(t)
	for _, mode := range []string{"cancel", "stale_same_path", "stale_other_path", "write_failure", "save_deadline", "intervening_save"} {
		t.Run(mode, func(t *testing.T) {
			got, err := eval(fmt.Sprintf(`(async function() {
				var mode = %q, p = prSplit, state = p._state, native = p._modules;
				var oldSpawn = native.exec.spawn, oldWrite = native.osmod.writeFile, oldRead = native.osmod.readFile;
				var oldWarn = log.warn, oldPrint = output.print, oldBudget = p._PANE_CLEANUP_TIMEOUT_MS;
				var oldState = Object.assign({}, state), oldVerify = p.runtime.verifyCommand;
				var oldCancel = p._cancelSource, oldActive = p._tuiOutputActive, oldDiagnostics = p._verifyCleanupDiagnostics;
				var stored = {}, writes = [], warnings = [], prints = [], calls = [], path = '';
				var release, entered, saveRelease, saveEntered, job, interveningSave;
				var ready = new Promise(function(resolve) { entered = resolve; });
				var childGate = new Promise(function(resolve) { release = resolve; });
				var saveReady = new Promise(function(resolve) { saveEntered = resolve; });
				var saveGate = new Promise(function(resolve) { saveRelease = resolve; });
				log.warn = function(message, fields) { warnings.push(fields); };
				output.print = function(message) { prints.push(String(message)); };
				p._tuiOutputActive = true; p._verifyCleanupDiagnostics = [];
				p._PANE_CLEANUP_TIMEOUT_MS = mode === 'save_deadline' ? 30 : 1000;
				native.osmod.writeFile = async function(file, content) {
					var snapshot = JSON.parse(content); writes.push({path:file, snapshot:snapshot});
					if (mode === 'intervening_save' && snapshot.executionOutcome && snapshot.executionOutcome.cleanupUnconfirmed) {
						saveEntered(); await saveGate;
					}
					if (snapshot.executionOutcome && snapshot.executionOutcome.treeCleanupError) {
						if (mode === 'write_failure') throw new Error('checkpoint write denied');
						if (mode === 'save_deadline') { saveEntered(); await saveGate; }
					}
					stored[file] = content;
				};
				native.osmod.readFile = async function(file) {
					return stored[file] ? {content:stored[file]} : {error:true, message:'file absent'};
				};
				function stream() { return {read:async function() { return {done:true}; }}; }
				native.exec.spawn = async function(command, args) {
					calls.push({command:command, args:args});
					var index = args.indexOf('worktree');
					if (command === 'git' && index >= 0 && args[index + 1] === 'add') path = args[args.length - 2];
					if (command !== 'git') entered();
					return {stdout:stream(), stderr:stream(), wait:async function() {
						if (command !== 'git') return await childGate;
						return {code:0, signal:null};
					}, kill:async function() {}};
				};
				setupPlanCache();
				var originalPlan = state.planCache;
				originalPlan.dir = p.runtime.dir; originalPlan.marker = 'original';
				originalPlan.splits = [{name:'split/1', files:['a.go']}]; originalPlan.fileStatuses = {'a.go':'M'};
				var rows = [{name:'split/1', sha:'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'}];
				state.executionResultCache = rows; state.executionOutcomeCache = {error:null, results:rows};
				p.runtime.verifyCommand = 'verify-command';
				state.agentExecutor = null; p._cancelSource = null;
				var planPath = p.resolvePlanPath(null, originalPlan.dir), s = initState('BRANCH_BUILDING');
				try {
					var beforeSave = await p.savePlan(planPath, 'Before repair');
					if (beforeSave.error) return 'initial snapshot failed';
					s.wizard.transition('ERROR_RESOLUTION'); s.wizardState = 'ERROR_RESOLUTION';
					p._handleErrorResolutionChoice(s, 'auto-resolve');
					await ready; job = s._resourceJobs[0];
					if (!job || job.worktreeDir !== path || !p._pipelinePending()) return 'repair allocation not owned';
					var replacement, replacementRows;
					if (mode.indexOf('stale_') === 0) {
						p._resetVerifyPhase(s);
						replacement = Object.assign({}, originalPlan, {marker:'replacement',
							dir:mode === 'stale_other_path' ? p.runtime.dir + '/replacement-repo' : originalPlan.dir});
						replacementRows = [{name:'replacement', sha:'replacement-sha'}];
						state.planCache = replacement; state.executionResultCache = replacementRows;
						p.runtime.verifyCommand = 'replacement-command';
						s.resolveResult = {replacement:true}; s.resolveRunning = true;
					} else {
						sendKey(s, 'ctrl+c'); sendKey(s, 'y');
						if (s.wizardQuitSent) return 'quit preceded native repair settlement';
					}
					if (mode === 'intervening_save') await saveReady;
					release({code:0, signal:null, treeCleanupError:'original tree close failed'});
					if (mode === 'intervening_save') {
						await new Promise(function(resolve) { setTimeout(resolve, 0); });
						if (!state.executionOutcomeCache.treeCleanupError) return 'final repair result did not reach checkpoint observer';
						replacement = Object.assign({}, originalPlan, {marker:'newer replacement'});
						replacementRows = [{name:'newer replacement', sha:'newer-sha'}];
						state.planCache = replacement; state.executionResultCache = replacementRows;
						interveningSave = p.savePlan(planPath, 'Newer replacement');
						saveRelease();
					}
					if (mode === 'save_deadline') {
						await saveReady;
						if (!p._pipelinePending() || s.wizardQuitSent) return 'owner released before awaited checkpoint';
						await new Promise(function(resolve) { setTimeout(resolve, 70); });
						if (!s.wizardQuitSent || !p._pipelinePending() ||
							!warnings.some(function(w) { return w.error.indexOf('snapshot is not confirmed saved') >= 0; })) return 'save deadline was unbounded or silent';
						saveRelease();
					}
					await job.promise; await Promise.resolve();
					if (interveningSave && (await interveningSave).error) return 'intervening replacement save failed';
					if (p._pipelinePending()) return 'finished save retained pipeline owner';
					if (mode.indexOf('stale_') === 0 && (state.planCache !== replacement || state.executionResultCache !== replacementRows ||
						!s.resolveResult.replacement || !s.resolveRunning)) return 'stale cleanup changed replacement UI/cache identities';
					if (mode === 'write_failure' && !warnings.some(function(w) { return w.error.indexOf('not saved') >= 0 && w.error.indexOf('checkpoint write denied') >= 0; })) return 'write failure was hidden';
					var snapshot = JSON.parse(stored[planPath]);
					var savedOwner = snapshot.executionOutcome;
					if (!savedOwner || !savedOwner.cleanupFailed || savedOwner.worktreeDir !== path ||
						savedOwner.results[0].sha !== rows[0].sha) return 'saved snapshot forgot original repair owner';
					if (mode !== 'write_failure' && savedOwner.treeCleanupError !== 'original tree close failed') return 'late cleanup failure absent from saved bytes';
					if (mode === 'stale_same_path' && (snapshot.plan.marker !== 'replacement' || snapshot.executed[0].name !== 'replacement' ||
						snapshot.runtime.verifyCommand !== 'replacement-command')) return 'stale save overwrote replacement plan or branch rows';
					if (mode === 'stale_other_path' && (snapshot.plan.marker !== 'original' || snapshot.executed[0].name !== 'split/1' ||
						snapshot.runtime.verifyCommand !== 'verify-command')) return 'original-path snapshot lost original plan/runtime';
					if (mode === 'intervening_save' && (snapshot.plan.marker !== 'newer replacement' || snapshot.executed[0].name !== 'newer replacement' ||
						state.planCache !== replacement || state.executionResultCache !== replacementRows)) return 'older repair snapshot overwrote intervening replacement save';
					state.executionOutcomeCache = null; state.executionResultCache = [];
					var loaded = await p.loadPlan(planPath);
					if (loaded.error) return 'saved owner could not be loaded';
					var before = calls.length;
					var retried = await p.executeSplitAsync(state.planCache, {completedResults:state.executionResultCache, completedOutcome:state.executionOutcomeCache});
					if (!retried.cleanupFailed || retried.worktreeDir !== path || calls.length !== before) return 'load/resume permitted mutation after retained or unconfirmed repair';
					p._tuiOutputActive = false; p._flushVerifyCleanupDiagnostics();
					if (!prints.some(function(message) { return message.indexOf(path) >= 0; })) return 'original resource path did not survive terminal exit';
					return 'OK';
				} finally {
					release({code:0, signal:null}); saveRelease();
					if (job) await job.promise.catch(function() {});
					if (interveningSave) await interveningSave;
					native.exec.spawn = oldSpawn; native.osmod.writeFile = oldWrite; native.osmod.readFile = oldRead;
					log.warn = oldWarn; output.print = oldPrint; p._PANE_CLEANUP_TIMEOUT_MS = oldBudget;
					Object.keys(state).forEach(function(key) { delete state[key]; }); Object.assign(state, oldState);
					p.runtime.verifyCommand = oldVerify; p._cancelSource = oldCancel;
					p._tuiOutputActive = oldActive; p._verifyCleanupDiagnostics = oldDiagnostics;
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
