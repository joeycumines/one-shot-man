package command

import (
	"fmt"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// Exercise the public orchestrator, including its real step wrapper and final
// cleanup policy. Native command results are injected at stage boundaries so a
// branch execution cannot erase a later resource owner's failure, including
// cleanup after an ordinary execution error and failed recovery snapshots.
func TestAutomatedSplit_PipelineFailuresPreserveOwnership(t *testing.T) {
	skipSlow(t)
	for _, phase := range []string{"verify", "resolve", "resolve_throw", "resplit_cleanup", "cleanup_throw", "failure_cleanup", "reverify", "clean",
		"execution_cleanup", "execution_owned", "execution_owned_empty", "execution_row_owned", "execution_checkpoint"} {
		faults := []string{"tree", "capture"}
		switch phase {
		case "resplit_cleanup", "failure_cleanup":
			faults = append(faults, "exit")
		case "clean":
			faults = []string{"tree"}
		case "execution_cleanup":
			faults = []string{"tree", "capture", "exit", "reject", "rejected-tree", "clean"}
		case "execution_owned", "execution_owned_empty", "execution_row_owned":
			faults = []string{"cleanup", "retained"}
		case "execution_checkpoint":
			faults = []string{"returned-error", "rejected-save", "missing-save"}
		}
		for _, fault := range faults {
			t.Run(phase+"/"+fault, func(t *testing.T) {
				eval := prsplittest.NewChunkEngine(t, map[string]any{"dir": t.TempDir()},
					"00_core", "10a_pipeline_config", "10d_pipeline_orchestrator")
				value, err := eval(fmt.Sprintf(`(async function() {
                    var p = prSplit, phase = %q, fault = %q, calls = [], checkpoints = [], messages = [];
                    var branchRows = [{name:'split/1', sha:'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', code:0, truncated:false}];
                    if (phase === 'execution_owned_empty') branchRows = [];
                    var failure = {name:'split/1', error:'native stage failed', code:0, exitCode:0, truncated:false};
                    if (fault === 'tree' || fault === 'rejected-tree') Object.assign(failure, {execFailed:true, cleanupFailed:true,
                        treeCleanupError:'tree close denied', worktreeRetained:true, worktreeDir:'/owned/repair'});
                    if (fault === 'capture') Object.assign(failure, {execFailed:true, truncated:true});
                    if (fault === 'exit') Object.assign(failure, {code:128, exitCode:128, error:'branch deletion denied'});
                    var plan = {dir:p.runtime.dir, baseBranch:'main', sourceBranch:'feature', verifyCommand:'verify',
                        splits:[{name:'split/1', files:['a.go'], message:'split', order:0}]};
                    var realRequire = require, oldRead = p._modules.osmod.readFile, oldPrint = output.print;
                    output.print = function(message){messages.push(String(message));};
                    var mcp = {addTool:function(){}, init:async function(){}, resetWaiter:function(){},
                        close:async function(){calls.push('close-mcp');}, mcpConfigPath:'mock.json'};
                    require = function(name) {
                        if (name === 'osm:mcp') return {createServer:function(){return {};}};
                        if (name === 'osm:mcpcallback') return {MCPCallback:function(){return mcp;}};
                        return realRequire(name);
                    };
                    p._modules.osmod.readFile = async function(){return {content:'{}'};};
                    p.resolvePlanPath = function(){return 'mock-plan.json';};
                    p.savePlan = async function(){
                        checkpoints.push(JSON.parse(JSON.stringify(p._state.executionOutcomeCache)));
                        if (phase === 'execution_checkpoint' && executions) {
                            if (fault === 'returned-error') return {error:'checkpoint write denied'};
                            if (fault === 'rejected-save') throw new Error('checkpoint write rejected');
                            return undefined;
                        }
                        return {path:'mock-plan.json'};
                    };
                    p.analyzeDiffAsync = async function(){return {files:['a.go'], baseBranch:'main', currentBranch:'feature', fileStatuses:{}, fileRenames:{}};};
                    p.AgentCodeExecutor = function(){
                        this.handle = {isAlive:function(){return true;}};
                        this.resolveAsync = async function(){return {};};
                        this.spawn = async function(){return {sessionId:1};};
                    };
                    p.renderClassificationPrompt = function(){return {text:'classify'};};
                    p.sendToHandle = async function(){return {};};
                    var classifications = 0;
                    p.waitForLogged = async function(name){
                        if (name !== 'reportClassification') return {error:'no optional plan'};
                        classifications++; calls.push('classify');
                        return {data:{'a.go':'changes'}};
                    };
                    p.classificationToGroups = function(){return {};};
                    p.createSplitPlanAsync = async function(){return plan;};
                    p.validateClassification = p.validatePlan = function(){return {valid:true};};
                    var executions = 0, verifications = 0, resolutions = 0, cleanups = 0, executionOutcome;
                    p.executeSplitAsync = async function(_plan, options){
                        executions++; calls.push('execute');
                        if (!options || !('completedOutcome' in options)) throw new Error('execution omitted owner checkpoint');
                        if (phase.indexOf('execution_') === 0) {
                            executionOutcome = {error:'primary commit failure', code:7, exitCode:7, truncated:false, results:branchRows.slice()};
                            if (phase !== 'execution_cleanup') {
                                var owned = {worktreeDir:'/owned/execution'};
                                if (fault === 'retained') owned.worktreeRetained = true;
                                else owned.cleanupFailed = true;
                                if (phase === 'execution_checkpoint') Object.assign(owned, {
                                    treeCleanupError:'execution tree close denied', worktreeRetained:true
                                });
                                if (phase === 'execution_row_owned') Object.assign(executionOutcome.results[0], owned);
                                else Object.assign(executionOutcome, owned);
                            }
                            return executionOutcome;
                        }
                        return {error:null, code:0, truncated:false, results:branchRows.slice()};
                    };
                    p.verifySplitsAsync = async function(){
                        verifications++; calls.push('verify');
                        if (phase === 'verify' || (phase === 'reverify' && verifications === 2)) {
                            return {error:'verification failed', allPassed:false, results:[Object.assign({passed:false}, failure)]};
                        }
                        return {allPassed:verifications > 1, results:[{name:'split/1', passed:verifications > 1}]};
                    };
                    p.resolveConflictsWithAgent = async function(){
                        resolutions++; calls.push('resolve');
                        if (phase === 'resolve_throw') throw Object.assign(new Error(failure.error), failure);
                        if (phase === 'resolve') return Object.assign({reSplitNeeded:true}, failure);
                        return phase === 'failure_cleanup' ? {error:'verification still fails', reSplitNeeded:false}
                            : {reSplitNeeded:true, reSplitReason:'separate changes'};
                    };
                    p.cleanupBranchesAsync = async function(){
                        cleanups++; calls.push('cleanup-branches');
                        if (phase === 'cleanup_throw' || (phase === 'execution_cleanup' &&
                            (fault === 'reject' || fault === 'rejected-tree'))) throw Object.assign(new Error(failure.error), failure);
                        if (phase === 'resplit_cleanup' || phase === 'failure_cleanup' ||
                            (phase === 'execution_cleanup' && fault !== 'clean')) {
                            return Object.assign({}, failure, {deleted:['split/old'], errors:['branch deletion failed']});
                        }
                        return {deleted:['split/1'], errors:[]};
                    };
                    p.verifyEquivalenceAsync = async function(){calls.push('equivalence'); return {equivalent:true};};
                    p.cleanupExecutor = async function(){calls.push('close-agent');};
                    try {
                        var result = await p.automatedSplit({disableTUI:true, cleanupOnFailure:true, maxReSplits:1,
                            pipelineTimeoutMs:60000, stepTimeoutMs:60000, watchdogIdleMs:60000});
                        if (phase.indexOf('execution_') === 0) {
                            if (!result.error || result.error.indexOf('primary commit failure') < 0 ||
                                result.report.error !== result.error || result.report.execution !== executionOutcome ||
                                executionOutcome.error !== 'primary commit failure' || executionOutcome.code !== 7 ||
                                executionOutcome.truncated !== false) return 'cleanup replaced execution facts: ' + JSON.stringify(result);
                            if (executions !== 1 || classifications !== 1 || verifications || resolutions || calls.indexOf('equivalence') >= 0) return 'execution failure continued the pipeline';
                            if (calls.indexOf('close-agent') < 0 || calls.indexOf('close-mcp') < 0 || checkpoints.length !== 2 ||
                                p._pipelinePending()) return 'execution failure bypassed terminal cleanup/checkpoint';
                            if (phase === 'execution_cleanup') {
                                var recordedCleanup = result.report.branchCleanup;
                                if (cleanups !== 1 || !recordedCleanup) return 'execution cleanup was not recorded';
                                if (fault === 'clean') {
                                    if (result.error !== 'primary commit failure' || recordedCleanup.errors.length) return 'clean branch cleanup changed primary failure';
                                } else {
                                    if (result.error.indexOf('Branch cleanup failed') < 0 || recordedCleanup.code !== failure.code ||
                                        recordedCleanup.truncated !== failure.truncated) return 'branch cleanup diagnostic or facts lost: ' + JSON.stringify(result);
                                    if (fault === 'tree' || fault === 'rejected-tree') {
                                        var cleanupOwner = p._state.executionOutcomeCache;
                                        if (!cleanupOwner || cleanupOwner.resourceFailure !== recordedCleanup ||
                                            cleanupOwner.treeCleanupError !== 'tree close denied' || cleanupOwner.results[0].sha !== branchRows[0].sha ||
                                            checkpoints[1].treeCleanupError !== 'tree close denied') return 'execution cleanup owner was not checkpointed';
                                    }
                                }
                            } else {
                                if (cleanups || result.report.branchCleanup || p._state.executionOutcomeCache !== executionOutcome ||
                                    !p._executionCleanupFailure(branchRows, executionOutcome)) return 'branch deletion ignored unresolved execution owner';
                            }
                            if (phase === 'execution_checkpoint') {
                                var saveReason = fault === 'returned-error' ? 'checkpoint write denied' :
                                    fault === 'rejected-save' ? 'checkpoint write rejected' : 'no save result';
                                if (!result.checkpointSaveError || result.checkpointSaveError.indexOf('mock-plan.json') < 0 ||
                                    result.checkpointSaveError.indexOf(saveReason) < 0 || result.error.indexOf(result.checkpointSaveError) < 0 ||
                                    result.report.checkpointSaveError !== result.checkpointSaveError) return 'checkpoint failure was hidden';
                                if (messages.some(function(message){return message.indexOf('Plan saved to:') >= 0 ||
                                    message.indexOf('To resume:') >= 0;})) return 'failed checkpoint claimed durable recovery';
                            }
                            return 'OK';
                        }
                        if (phase === 'clean') {
                            if (result.error || executions !== 2 || classifications !== 2 || cleanups !== 1) return 'clean re-split failed: ' + JSON.stringify({result:result,calls:calls});
                            if (result.report.execution.code !== 0 || result.report.execution.results.length !== 1) return 'successful re-execution facts lost';
                            return 'OK';
                        }
                        if (!result.error || !result.report.error) return 'failure became success: ' + JSON.stringify({result:result,calls:calls});
                        if (phase === 'verify' && resolutions) return 'repair ran after verification uncertainty';
                        if ((phase === 'resolve' || phase === 'resolve_throw') && cleanups) return 'branch deletion ran after repair uncertainty';
                        if (phase !== 'reverify' && (executions !== 1 || classifications !== 1)) return 're-split continued after failure: ' + JSON.stringify(calls);
                        if (phase === 'reverify' && cleanups !== 1) return 'cleanupOnFailure retried after unknown re-verification';
                        if (calls.indexOf('equivalence') >= 0) return 'equivalence ran after pipeline failure';
                        var r = result.report;
                        var recorded = phase === 'verify' ? r.verification.results[0]
                            : phase === 'reverify' ? r.reverification.results[0]
                            : phase === 'resolve' || phase === 'resolve_throw' ? r.resolution : r.branchCleanup;
                        if (!recorded || recorded.code !== failure.code || recorded.exitCode !== failure.exitCode || recorded.truncated !== failure.truncated) return 'original command facts lost: ' + JSON.stringify(result);
                        if (r.execution.error || r.execution.code !== 0 || r.execution.truncated !== false || r.execution.results.length !== 1) return 'later failure overwrote successful branch facts';
                        if (fault === 'tree') {
                            var owner = p._state.executionOutcomeCache;
                            if (recorded.treeCleanupError !== 'tree close denied' || !owner || !owner.resourceFailure ||
                                owner.worktreeDir !== '/owned/repair' || owner.treeCleanupError !== 'tree close denied' ||
                                owner.results.length !== 1 || owner.results[0].sha !== branchRows[0].sha) return 'owner checkpoint lost: ' + JSON.stringify(owner);
                            if (!checkpoints.length || checkpoints[checkpoints.length - 1].treeCleanupError !== 'tree close denied') return 'owner was not saved on failure';
                            if (!p._executionCleanupFailure(branchRows, owner)) return 'next resume would forget unresolved owner';
                        }
                        if (phase === 'failure_cleanup' && result.error.indexOf('verification still fails') < 0) return 'cleanup replaced primary failure';
                        return 'OK';
                    } finally { require = realRequire; p._modules.osmod.readFile = oldRead; output.print = oldPrint; }
                })()`, phase, fault))
				if err != nil {
					t.Fatal(err)
				}
				if value != "OK" {
					t.Fatal(value)
				}
			})
		}
	}
}

func TestAutomatedSplit_PendingExecutionDoesNotResetOwner(t *testing.T) {
	skipSlow(t)
	eval := prsplittest.NewChunkEngine(t, nil, "00_core", "10a_pipeline_config", "10d_pipeline_orchestrator")
	value, err := eval(`(async function(){
        var owner = {error:null, results:[{name:'active'}]};
        prSplit._state.executionOutcomeCache = owner;
        prSplit._state.executionResultCache = owner.results;
        prSplit._executionPending = function(){return true;};
        var result = await prSplit.automatedSplit({});
        return result.executionPending && result.error && prSplit._state.executionOutcomeCache === owner &&
            prSplit._state.executionResultCache === owner.results ? 'OK' : JSON.stringify(result);
    })()`)
	if err != nil {
		t.Fatal(err)
	}
	if value != "OK" {
		t.Fatal(value)
	}
}
