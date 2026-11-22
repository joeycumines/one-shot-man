'use strict';
// pr_split_05_execution.js — executeSplit
// Dependencies: chunks 00 (_gitExec, isCancelled), 04 (validatePlan).
// Attaches to prSplit: executeSplit.

(function(prSplit) {
    var gitExec = prSplit._gitExec;
    var resolveDir = prSplit._resolveDir;
    var validatePlan = prSplit.validatePlan;
    var worktreeTmpPath = prSplit._worktreeTmpPath;
    var execFailReason = prSplit._execFailReason;
    var commandFailed = prSplit._commandFailed;

    function isCancelledHelper() {
        if (typeof isCancelled === 'function' && isCancelled()) return true;
        if (typeof prSplit !== 'undefined' && typeof prSplit.isCancelled === 'function' && prSplit.isCancelled()) return true;
        return false;
    }

    function isForceCancelledHelper() {
        if (typeof isForceCancelled === 'function' && isForceCancelled()) return true;
        if (typeof prSplit !== 'undefined' && typeof prSplit.isForceCancelled === 'function' && prSplit.isForceCancelled()) return true;
        return false;
    }

    function collectSplitFileActions(split, fileStatuses, fileRenames, ignoredFiles, splitResult, progressFn) {
        var removeFiles = [];
        var checkoutFiles = [];
        var addFiles = [];
        var seenRemove = {};
        var seenCheckout = {};

        for (var i = 0; i < split.files.length; i++) {
            if (isCancelledHelper() || isForceCancelledHelper()) {
                return {
                    error: 'cancelled by user after ' + i + ' of ' + split.files.length + ' files in ' + split.name,
                    cancelled: true
                };
            }

            var file = split.files[i];
            if (ignoredFiles[file]) {
                splitResult.skippedFiles.push(file);
                if (typeof log !== 'undefined' && log.warn) {
                    log.warn('pr-split: skipping git-ignored file in ' + split.name + ': ' + file);
                }
                continue;
            }

            var status = fileStatuses[file];

            if (progressFn && split.files.length > 5) {
                progressFn('  ' + split.name + ': file ' + (i + 1) + '/' + split.files.length);
            }

            if (!status) {
                return {
                    error: 'file "' + file + '" has no entry in plan.fileStatuses — '
                        + 'ensure analyzeDiff() results are passed to createSplitPlan()'
                };
            }

            if (status === 'R') {
                var renameSource = fileRenames[file];
                if (!renameSource) {
                    return {
                        error: 'rename source missing for "' + file + '" in plan.fileRenames'
                    };
                }
                if (!seenRemove[renameSource]) {
                    seenRemove[renameSource] = true;
                    removeFiles.push(renameSource);
                }
                if (!seenCheckout[file]) {
                    seenCheckout[file] = true;
                    checkoutFiles.push(file);
                }
                addFiles.push(file);
            } else if (status === 'D') {
                if (!seenRemove[file]) {
                    seenRemove[file] = true;
                    removeFiles.push(file);
                }
            } else {
                if (status === 'T' && typeof log !== 'undefined' && log.warn) {
                    log.warn('pr-split: file type change for ' + file + ' — checkout from source will restore new type');
                }
                if (!seenCheckout[file]) {
                    seenCheckout[file] = true;
                    checkoutFiles.push(file);
                }
                addFiles.push(file);
            }
        }

        return {
            error: null,
            removeFiles: removeFiles,
            checkoutFiles: checkoutFiles,
            addFiles: addFiles
        };
    }

    // Both public entry points share worktree ownership and failure handling;
    // only their process transport differs (captured execv or streaming spawn).
    var executionPending = false;

    async function executePlan(plan, options, runGit) {
        options = options || {};
        if (executionPending) {
            return {
                error: 'Execution is still running; wait for the current execution and worktree cleanup to finish.',
                executionPending: true,
                results: []
            };
        }
        var pipelineOwner = options.pipelineOwner;
        var ownsPipelineLease = !prSplit._ownsPipeline(pipelineOwner);
        if (ownsPipelineLease) pipelineOwner = prSplit._claimPipelineOwner();
        if (!pipelineOwner) {
            return {
                error: 'The automated pipeline is still running; wait for verification, repair, and cleanup to finish.',
                executionPending: true,
                results: []
            };
        }
        executionPending = true;
        try {
            var blocked = prSplit._executionCleanupFailure(options.completedResults, options.completedOutcome);
            if (blocked) return Object.assign({}, blocked.execution, {error:blocked.error, results:blocked.results});
            var results = [];
            var outcome = { error: null, results: results };
            var validation = validatePlan(plan);
            if (!validation.valid) {
                outcome.error = 'invalid plan: ' + validation.errors.join('; ');
                return outcome;
            }
            var dir = resolveDir(plan.dir || '.');
            var progressFn = options.progressFn || null;
            var ignoredFiles = {};
            var completedResults = Array.isArray(options.completedResults) ? options.completedResults : [];
            var completedCount = completedResults.length;
            if (completedCount > plan.splits.length) {
                return { error: 'resume execution results exceed plan split count', results: [] };
            }
            for (var cr = 0; cr < completedCount; cr++) {
                var completed = completedResults[cr];
                if (!completed || completed.name !== plan.splits[cr].name || completed.error ||
                    completed.cleanupFailed || completed.treeCleanupError || completed.worktreeRetained) {
                    return { error: 'resume execution result does not match plan prefix at index ' + cr, results: [] };
                }
                results.push(Object.assign({}, completed));
            }
            if (!plan.fileStatuses || typeof plan.fileStatuses !== 'object') {
                return {
                    error: 'plan.fileStatuses is required — pass fileStatuses from analyzeDiff() to createSplitPlan()',
                    results: []
                };
            }
            var fileStatuses = plan.fileStatuses;
            var fileRenames = plan.fileRenames || {};
            var worktreePath = null;
            var worktreeAttempted = false;
            var treeCleanupError = '';
            var activeResult = null;

            function treeError(value) {
                return value && typeof value.treeCleanupError === 'string' ? value.treeCleanupError : '';
            }
            function reason(value) {
                return (value && (execFailReason(value) || value.message)) || String(value || 'no command result');
            }
            async function command(commandDir, args, label, acceptedCodes) {
                var result;
                try {
                    result = await runGit(commandDir, args);
                } catch (e) {
                    treeCleanupError = treeError(e) || treeCleanupError;
                    throw new Error(label + ': ' + reason(e));
                }
                treeCleanupError = treeError(result) || treeCleanupError;
                if (commandFailed(result, acceptedCodes)) {
                    throw new Error(label + ': ' + reason(result));
                }
                return result;
            }
            function recordCleanupFailure(message, retained, cleanupTreeError) {
                outcome.cleanupFailed = true;
                outcome.worktreeDir = worktreePath;
                if (retained) outcome.worktreeRetained = true;
                if (cleanupTreeError) outcome.treeCleanupError = cleanupTreeError;
                outcome.error = (outcome.error ? outcome.error + '; ' : '') + 'cleanup: ' + message;
                // A completed branch is still a real commit. Preserve its SHA and
                // command error, but reject this checkpoint on a later resume until
                // its unresolved resource ownership has been dealt with.
                if (activeResult) {
                    activeResult.cleanupFailed = true;
                    activeResult.worktreeDir = worktreePath;
                    if (retained) activeResult.worktreeRetained = true;
                    if (cleanupTreeError) activeResult.treeCleanupError = cleanupTreeError;
                }
            }

            try {
                if (completedCount < plan.splits.length) {
                    var allPlanFiles = [];
                    for (var pi = 0; pi < plan.splits.length; pi++) {
                        for (var pf = 0; pf < plan.splits[pi].files.length; pf++) {
                            allPlanFiles.push(plan.splits[pi].files[pf]);
                        }
                    }
                    if (allPlanFiles.length > 0) {
                        // check-ignore's ordinary exit 1 means no ignored files.
                        // Capture/teardown errors remain failures at either code.
                        var checkResult = await command(dir, ['check-ignore', '--no-index'].concat(allPlanFiles),
                            'git check-ignore failed', [0, 1]);
                        if (checkResult.code === 0 && checkResult.stdout.trim()) {
                            var ignoreLines = checkResult.stdout.trim().split('\n');
                            for (var il = 0; il < ignoreLines.length; il++) {
                                var ignored = ignoreLines[il].trim();
                                if (ignored) ignoredFiles[ignored] = true;
                            }
                            if (typeof log !== 'undefined' && log.warn) {
                                var ignoredList = Object.keys(ignoredFiles);
                                log.warn('pr-split: ' + ignoredList.length + ' file(s) in plan match .gitignore rules and will be skipped: ' + ignoredList.join(', '));
                            }
                        }
                    }
                    for (var k = completedCount; k < plan.splits.length; k++) {
                        var existCheck = await command(dir, ['rev-parse', '--verify', 'refs/heads/' + plan.splits[k].name],
                            'check existing branch failed', [0, 1, 128]);
                        if (existCheck.code === 0) {
                            await command(dir, ['branch', '-D', plan.splits[k].name], 'delete existing branch failed');
                        }
                    }

                    worktreePath = worktreeTmpPath('osm-worktree-');
                    // Even a rejected/failed add may have created some resources.
                    // The finally block owns them from the moment add is attempted.
                    worktreeAttempted = true;
                    await command(dir, ['worktree', 'add', '--detach', worktreePath, plan.baseBranch],
                        'create worktree failed');

                    var currentBase = completedCount > 0 ? plan.splits[completedCount - 1].name : plan.baseBranch;
                    for (var i = completedCount; i < plan.splits.length; i++) {
                        if (isCancelledHelper() || isForceCancelledHelper()) {
                            throw new Error('cancelled by user after ' + i + ' of ' + plan.splits.length + ' branches');
                        }
                        var split = plan.splits[i];
                        activeResult = { name: split.name, files: split.files, sha: '', error: null, skippedFiles: [] };
                        results.push(activeResult);
                        if (progressFn) progressFn('Creating branch ' + (i + 1) + '/' + plan.splits.length + ': ' + split.name);
                        await command(worktreePath, ['checkout', '-b', split.name, currentBase],
                            'create branch ' + split.name + ' from ' + currentBase + ' failed');
                        var actions = collectSplitFileActions(split, fileStatuses, fileRenames, ignoredFiles, activeResult, progressFn);
                        if (actions.error) throw new Error(actions.error);
                        if (actions.removeFiles.length > 0) {
                            await command(worktreePath, ['rm', '--ignore-unmatch', '-f'].concat(actions.removeFiles),
                                'git rm ' + actions.removeFiles.join(' '));
                        }
                        if (actions.checkoutFiles.length > 0) {
                            await command(worktreePath, ['checkout', plan.sourceBranch, '--'].concat(actions.checkoutFiles),
                                'checkout files for ' + split.name);
                        }
                        if (actions.addFiles.length > 0) {
                            await command(worktreePath, ['add', '--'].concat(actions.addFiles), 'git add failed');
                        }
                        await command(worktreePath, ['commit', '-m', split.message || 'split: ' + split.name], 'git commit failed');
                        var sha = await command(worktreePath, ['rev-parse', 'HEAD'], 'rev-parse HEAD after commit failed');
                        if (!sha.stdout.trim()) throw new Error('rev-parse HEAD after commit failed: empty output');
                        activeResult.sha = sha.stdout.trim();
                        if (progressFn) progressFn('Branch ' + (i + 1) + '/' + plan.splits.length + ' created: ' + split.name);
                        currentBase = split.name;
                        activeResult = null;
                    }
                }
                var overallSkipped = [];
                for (var oi = 0; oi < results.length; oi++) {
                    overallSkipped = overallSkipped.concat(results[oi].skippedFiles || []);
                }
                outcome.overallSkippedFiles = overallSkipped;
            } catch (e) {
                outcome.error = String((e && e.message) || e);
                if (activeResult) activeResult.error = outcome.error;
                if (treeCleanupError) outcome.treeCleanupError = treeCleanupError;
            } finally {
                if (worktreeAttempted) {
                    // A known tree failure withholds removal. A failed removal is
                    // an attempted operation, and cannot prove the path still exists.
                    if (!activeResult && results.length > 0) activeResult = results[results.length - 1];
                    if (treeCleanupError) {
                        recordCleanupFailure('process tree teardown failed: ' + treeCleanupError +
                            '; worktree retained at ' + worktreePath, true, treeCleanupError);
                    } else {
                        var removal;
                        var rejected = false;
                        try {
                            removal = await runGit(dir, ['worktree', 'remove', '--force', worktreePath]);
                        } catch (e) {
                            removal = e;
                            rejected = true;
                        }
                        if (rejected || commandFailed(removal)) {
                            recordCleanupFailure('worktree removal failed: ' + reason(removal) +
                                '; worktree removal was attempted at ' + worktreePath, false, treeError(removal));
                        }
                    }
                }
            }
            return outcome;
        } finally {
            executionPending = false;
            if (ownsPipelineLease) prSplit._releasePipelineOwner(pipelineOwner);
        }
    }

    async function executeSplit(plan, options) {
        return await executePlan(plan, options, gitExec);
    }

    async function executeSplitAsync(plan, options) {
        return await executePlan(plan, options, prSplit._gitExecAsync);
    }

    // --- Exports ---
    prSplit._executionPending = function() { return executionPending; };
    prSplit.executeSplit = executeSplit;
    prSplit.executeSplitAsync = executeSplitAsync;
})(globalThis.prSplit);
