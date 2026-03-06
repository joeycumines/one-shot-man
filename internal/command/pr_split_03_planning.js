'use strict';
// pr_split_03_planning.js — createSplitPlan, savePlan, loadPlan
// Dependencies: chunks 00-02 must be loaded first.
//
// Reads from prSplit: _sanitizeBranchName, _padIndex, _gitExec, runtime,
//   _modules.osmod, _state (shared mutable caches).
// Late-bound: prSplit.getConversationHistory (set by chunk 11).
// Attaches to prSplit: createSplitPlan, savePlan, loadPlan, DEFAULT_PLAN_PATH.

(function(prSplit) {
    var runtime = prSplit.runtime;
    var gitExec = prSplit._gitExec;
    var resolveDir = prSplit._resolveDir;
    var sanitizeBranchName = prSplit._sanitizeBranchName;
    var padIndex = prSplit._padIndex;
    var osmod = prSplit._modules.osmod;
    var state = prSplit._state;

    var DEFAULT_PLAN_PATH = '.pr-split-plan.json';

    // -----------------------------------------------------------------------
    //  createSplitPlan — builds a plan from group objects
    // -----------------------------------------------------------------------
    function createSplitPlan(groups, config) {
        if (!groups || typeof groups !== 'object') groups = {};
        config = config || {};
        var dir = resolveDir(config.dir || '.');
        var baseBranch = config.baseBranch || runtime.baseBranch;
        var branchPrefix = config.branchPrefix || runtime.branchPrefix;
        var commitPrefix = config.commitPrefix || '';
        var verifyCommand = config.verifyCommand || runtime.verifyCommand;
        var fileStatuses = config.fileStatuses || {};

        var sourceBranch = config.sourceBranch;
        if (!sourceBranch) {
            var result = gitExec(dir, ['rev-parse', '--abbrev-ref', 'HEAD']);
            sourceBranch = result.code === 0 ? result.stdout.trim() : 'HEAD';
        }

        var groupNames = Object.keys(groups).sort();
        var splits = [];

        for (var i = 0; i < groupNames.length; i++) {
            var name = groupNames[i];
            var groupData = groups[name];
            // Support both new format {files: [...], description: "..."} and
            // legacy format (plain array of files).
            var files = Array.isArray(groupData) ? groupData : (groupData.files || []);
            var description = (typeof groupData === 'object' && !Array.isArray(groupData))
                ? (groupData.description || '')
                : '';
            splits.push({
                name: sanitizeBranchName(branchPrefix + padIndex(i + 1) + '-' + name),
                files: files.slice().sort(),
                message: description || (commitPrefix + name),
                order: i,
                dependencies: i === 0 ? [] : [splits[i - 1].name]
            });
        }

        return {
            baseBranch: baseBranch,
            sourceBranch: sourceBranch,
            dir: dir,
            verifyCommand: verifyCommand,
            fileStatuses: fileStatuses,
            splits: splits
        };
    }

    // -----------------------------------------------------------------------
    //  savePlan — persists plan + caches to a JSON file
    // -----------------------------------------------------------------------
    // An optional second argument specifies the lastCompletedStep name for
    // crash recovery.  When provided, the snapshot version is bumped to 2.
    function savePlan(path, lastCompletedStep) {
        path = path || DEFAULT_PLAN_PATH;
        if (!osmod) {
            return { error: 'osm:os module not available — cannot persist plan' };
        }
        if (!state.planCache) {
            return { error: 'no plan to save — run "plan" or "run" first' };
        }

        // Late-bound: conversationHistory comes from chunk 11 utilities.
        var getHistory = (typeof prSplit.getConversationHistory === 'function')
            ? prSplit.getConversationHistory
            : function() { return []; };

        var snapshot = {
            version: lastCompletedStep ? 2 : 1,
            savedAt: new Date().toISOString(),
            runtime: {
                baseBranch:    runtime.baseBranch,
                strategy:      runtime.strategy,
                maxFiles:      runtime.maxFiles,
                branchPrefix:  runtime.branchPrefix,
                verifyCommand: runtime.verifyCommand,
                dryRun:        runtime.dryRun
            },
            analysis: state.analysisCache ? {
                files:          state.analysisCache.files,
                fileStatuses:   state.analysisCache.fileStatuses,
                baseBranch:     state.analysisCache.baseBranch,
                currentBranch:  state.analysisCache.currentBranch
            } : null,
            groups: state.groupsCache,
            plan: state.planCache,
            executed: state.executionResultCache || [],
            conversations: getHistory()
        };
        if (lastCompletedStep) {
            snapshot.lastCompletedStep = lastCompletedStep;
        }

        try {
            osmod.writeFile(path, JSON.stringify(snapshot, null, 2));
            return { path: path, error: null };
        } catch (e) {
            return { error: 'failed to write plan: ' + String(e) };
        }
    }

    // -----------------------------------------------------------------------
    //  loadPlan — reads a previously-saved plan snapshot from disk
    // -----------------------------------------------------------------------
    function loadPlan(path) {
        path = path || DEFAULT_PLAN_PATH;
        if (!osmod) {
            return { error: 'osm:os module not available — cannot load plan' };
        }

        var result = osmod.readFile(path);
        if (result.error) {
            return { error: 'failed to read plan: ' + result.error };
        }

        var snapshot;
        try {
            snapshot = JSON.parse(result.content);
        } catch (e) {
            return { error: 'invalid JSON in plan file: ' + String(e) };
        }

        if (!snapshot.version || snapshot.version < 1) {
            return { error: 'unsupported plan version: ' + String(snapshot.version) };
        }
        if (!snapshot.plan || !snapshot.plan.splits) {
            return { error: 'plan file "' + path + '" missing splits — file may be corrupt or from an incompatible version' };
        }

        // Restore runtime config.
        if (snapshot.runtime) {
            runtime.baseBranch    = snapshot.runtime.baseBranch    || runtime.baseBranch;
            runtime.strategy      = snapshot.runtime.strategy      || runtime.strategy;
            runtime.maxFiles      = snapshot.runtime.maxFiles      || runtime.maxFiles;
            runtime.branchPrefix  = snapshot.runtime.branchPrefix  || runtime.branchPrefix;
            runtime.verifyCommand = snapshot.runtime.verifyCommand || runtime.verifyCommand;
            if (snapshot.runtime.dryRun !== undefined) {
                runtime.dryRun = snapshot.runtime.dryRun;
            }
        }

        // Restore caches.
        if (snapshot.analysis) {
            state.analysisCache = snapshot.analysis;
        }
        if (snapshot.groups) {
            state.groupsCache = snapshot.groups;
        }
        state.planCache = snapshot.plan;
        state.executionResultCache = snapshot.executed || [];

        // Restore conversation history if present (late-bound via chunk 11).
        if (snapshot.conversations && Array.isArray(snapshot.conversations)) {
            if (!state.conversationHistory) state.conversationHistory = [];
            for (var c = 0; c < snapshot.conversations.length; c++) {
                state.conversationHistory.push(snapshot.conversations[c]);
            }
        }

        return {
            path: path,
            error: null,
            totalSplits:      state.planCache.splits.length,
            executedSplits:   state.executionResultCache.length,
            pendingSplits:    state.planCache.splits.length - state.executionResultCache.length,
            lastCompletedStep: snapshot.lastCompletedStep || null
        };
    }

    // -----------------------------------------------------------------------
    //  Exports
    // -----------------------------------------------------------------------
    prSplit.DEFAULT_PLAN_PATH = DEFAULT_PLAN_PATH;
    prSplit.createSplitPlan = createSplitPlan;
    prSplit.savePlan = savePlan;
    prSplit.loadPlan = loadPlan;
})(globalThis.prSplit);
