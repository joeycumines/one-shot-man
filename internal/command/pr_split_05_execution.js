'use strict';
// pr_split_05_execution.js — executeSplit
// Dependencies: chunks 00 (_gitExec, isCancelled), 04 (validatePlan).
// Attaches to prSplit: executeSplit.

(function(prSplit) {
    var gitExec = prSplit._gitExec;
    var gitExecAsync = prSplit._gitExecAsync;
    var resolveDir = prSplit._resolveDir;
    var validatePlan = prSplit.validatePlan;
    var worktreeTmpPath = prSplit._worktreeTmpPath;

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

    // --- executeSplit — creates branches for each split in a plan ---
    //
    // All branch operations run in a temporary git worktree so the user's
    // working directory remains completely untouched.
    async function executeSplit(plan, options) {
        options = options || {};
        var dir = resolveDir(plan.dir || '.');
        var results = [];
        var progressFn = options.progressFn || null;
        var ignoredFiles = {};

        var validation = validatePlan(plan);
        if (!validation.valid) {
            return { error: 'invalid plan: ' + validation.errors.join('; '), results: [] };
        }

        // fileStatuses is REQUIRED — it determines whether each file
        // should be checked out (A/M/R/C/T) or removed (D).
        if (!plan.fileStatuses || typeof plan.fileStatuses !== 'object') {
            return {
                error: 'plan.fileStatuses is required — pass fileStatuses from analyzeDiff() to createSplitPlan()',
                results: []
            };
        }
        var fileStatuses = plan.fileStatuses;
        var fileRenames = plan.fileRenames || {};

        // Pre-validate: detect git-ignored files in the plan.
        // Files matching .gitignore rules won't be processable via git add
        // on fresh branches. We collect all plan files and batch-check them.
        var allPlanFiles = [];
        for (var pi = 0; pi < plan.splits.length; pi++) {
            for (var pf = 0; pf < plan.splits[pi].files.length; pf++) {
                allPlanFiles.push(plan.splits[pi].files[pf]);
            }
        }
        if (allPlanFiles.length > 0) {
            // --no-index: check purely against .gitignore rules, even if the
            // file is currently tracked (e.g., force-added). On a new branch
            // created from base, these files won't be tracked, so git add
            // would silently skip them.
            var checkArgs = ['check-ignore', '--no-index'].concat(allPlanFiles);
            var checkResult = await gitExec(dir, checkArgs);
            // exit 0 = at least one file is ignored; exit 1 = none ignored
            if (checkResult.code === 0 && checkResult.stdout.trim()) {
                var ignoreLines = checkResult.stdout.trim().split('\n');
                for (var il = 0; il < ignoreLines.length; il++) {
                    var ig = ignoreLines[il].trim();
                    if (ig) {
                        ignoredFiles[ig] = true;
                    }
                }
                if (typeof log !== 'undefined' && log.warn) {
                    var ignoredList = Object.keys(ignoredFiles);
                    log.warn('pr-split: ' + ignoredList.length + ' file(s) in plan match .gitignore rules and will be skipped: ' + ignoredList.join(', '));
                }
            }
        }

        // Pre-flight: delete any pre-existing split branches to allow re-runs.
        for (var k = 0; k < plan.splits.length; k++) {
            var existCheck = await gitExec(dir, ['rev-parse', '--verify', 'refs/heads/' + plan.splits[k].name]);
            if (existCheck.code === 0) {
                await gitExec(dir, ['branch', '-D', plan.splits[k].name]);
            }
        }

        // Create a temporary git worktree for isolated split operations.
        // The user's CWD remains completely untouched.
        // T103: Use system temp dir to avoid fragile dir + '/../' pattern.
        var worktreePath = worktreeTmpPath('osm-worktree-');
        var wtAdd = await gitExec(dir, ['worktree', 'add', '--detach', worktreePath, plan.baseBranch]);
        if (wtAdd.code !== 0) {
            return { error: 'create worktree failed: ' + wtAdd.stderr.trim(), results: [] };
        }

        async function cleanupWorktree() {
            await gitExec(dir, ['worktree', 'remove', '--force', worktreePath]);
        }

        // INVARIANT (T108): Split branches form a CUMULATIVE CHAIN.
        // Each split branches off the previous split (not baseBranch).
        // split[0] branches off baseBranch, split[1] off split[0], etc.
        // The final split therefore contains ALL files from ALL prior splits
        // plus its own new files. This is why verifyEquivalence() (T094) only
        // needs to compare the LAST split's tree SHA to sourceBranch^{tree}.
        var currentBase = plan.baseBranch;

        for (var i = 0; i < plan.splits.length; i++) {
            if (isCancelledHelper() || isForceCancelledHelper()) {  // T117: honor force-cancel
                await cleanupWorktree();
                return { error: 'cancelled by user after ' + i + ' of ' + plan.splits.length + ' branches', results: results };
            }

            var split = plan.splits[i];
            var splitResult = { name: split.name, files: split.files, sha: '', error: null, skippedFiles: [] };

            if (progressFn) {
                progressFn('Creating branch ' + (i + 1) + '/' + plan.splits.length + ': ' + split.name);
            }

            var co = await gitExec(worktreePath, ['checkout', '-b', split.name, currentBase]);
            if (co.code !== 0) {
                splitResult.error = 'create branch ' + split.name + ' from ' + currentBase + ' failed: ' + co.stderr.trim();
                results.push(splitResult);
                await cleanupWorktree();
                return { error: splitResult.error, results: results };
            }

            var actions = collectSplitFileActions(split, fileStatuses, fileRenames, ignoredFiles, splitResult, progressFn);
            if (actions.error) {
                splitResult.error = actions.error;
                results.push(splitResult);
                await cleanupWorktree();
                return { error: splitResult.error, results: results };
            }

            if (actions.removeFiles.length > 0) {
                var remove = await gitExec(worktreePath,
                    ['rm', '--ignore-unmatch', '-f'].concat(actions.removeFiles));
                if (remove.code !== 0) {
                    splitResult.error = 'git rm ' + actions.removeFiles.join(' ') + ': ' + remove.stderr.trim();
                    results.push(splitResult);
                    await cleanupWorktree();
                    return { error: splitResult.error, results: results };
                }
            }

            if (actions.checkoutFiles.length > 0) {
                var checkout = await gitExec(worktreePath,
                    ['checkout', plan.sourceBranch, '--'].concat(actions.checkoutFiles));
                if (checkout.code !== 0) {
                    splitResult.error = 'checkout files for ' + split.name + ': ' + checkout.stderr.trim();
                    results.push(splitResult);
                    await cleanupWorktree();
                    return { error: splitResult.error, results: results };
                }
            }

            if (actions.addFiles.length > 0) {
                var addArgs = ['add', '--'].concat(actions.addFiles);
                var add = await gitExec(worktreePath, addArgs);
                if (add.code !== 0) {
                    splitResult.error = 'git add failed: ' + add.stderr.trim();
                    results.push(splitResult);
                    await cleanupWorktree();
                    return { error: splitResult.error, results: results };
                }
            }

            var msg = split.message || 'split: ' + split.name;
            var commit = await gitExec(worktreePath, ['commit', '-m', msg]);
            if (commit.code !== 0) {
                splitResult.error = 'git commit failed: ' + commit.stderr.trim();
                results.push(splitResult);
                await cleanupWorktree();
                return { error: splitResult.error, results: results };
            }

            var sha = await gitExec(worktreePath, ['rev-parse', 'HEAD']);
            splitResult.sha = sha.code === 0 ? sha.stdout.trim() : '';

            results.push(splitResult);

            if (progressFn) {
                progressFn('Branch ' + (i + 1) + '/' + plan.splits.length + ' created: ' + split.name);
            }

            currentBase = split.name;
        }

        await cleanupWorktree();

        // T107: Collect all git-ignored files across all splits for top-level reporting.
        var overallSkipped = [];
        for (var oi = 0; oi < results.length; oi++) {
            if (results[oi].skippedFiles && results[oi].skippedFiles.length > 0) {
                overallSkipped = overallSkipped.concat(results[oi].skippedFiles);
            }
        }
        return { error: null, results: results, overallSkippedFiles: overallSkipped };
    }

    // executeSplitAsync is the non-blocking version of executeSplit.
    // Uses gitExecAsync (exec.spawn) so the event loop stays responsive during BubbleTea TUI.
    // T31: async version for pipeline use.
    async function executeSplitAsync(plan, options) {
        var gitExecAsync = prSplit._gitExecAsync;
        options = options || {};
        var dir = resolveDir(plan.dir || '.');
        var results = [];
        var progressFn = options.progressFn || null;
        var ignoredFiles = {};

        var validation = validatePlan(plan);
        if (!validation.valid) {
            return { error: 'invalid plan: ' + validation.errors.join('; '), results: [] };
        }

        if (!plan.fileStatuses || typeof plan.fileStatuses !== 'object') {
            return {
                error: 'plan.fileStatuses is required — pass fileStatuses from analyzeDiff() to createSplitPlan()',
                results: []
            };
        }
        var fileStatuses = plan.fileStatuses;
        var fileRenames = plan.fileRenames || {};

        // Pre-validate: detect git-ignored files in the plan.
        var allPlanFiles = [];
        for (var pi = 0; pi < plan.splits.length; pi++) {
            for (var pf = 0; pf < plan.splits[pi].files.length; pf++) {
                allPlanFiles.push(plan.splits[pi].files[pf]);
            }
        }
        if (allPlanFiles.length > 0) {
            var checkArgs = ['check-ignore', '--no-index'].concat(allPlanFiles);
            var checkResult = await gitExecAsync(dir, checkArgs);
            if (checkResult.code === 0 && checkResult.stdout.trim()) {
                var ignoreLines = checkResult.stdout.trim().split('\n');
                for (var il = 0; il < ignoreLines.length; il++) {
                    var ig = ignoreLines[il].trim();
                    if (ig) {
                        ignoredFiles[ig] = true;
                    }
                }
                if (typeof log !== 'undefined' && log.warn) {
                    var ignoredList = Object.keys(ignoredFiles);
                    log.warn('pr-split: ' + ignoredList.length + ' file(s) in plan match .gitignore rules and will be skipped: ' + ignoredList.join(', '));
                }
            }
        }

        // Pre-flight: delete any pre-existing split branches to allow re-runs.
        for (var k = 0; k < plan.splits.length; k++) {
            var existCheck = await gitExecAsync(dir, ['rev-parse', '--verify', 'refs/heads/' + plan.splits[k].name]);
            if (existCheck.code === 0) {
                await gitExecAsync(dir, ['branch', '-D', plan.splits[k].name]);
            }
        }

        // Create a temporary git worktree for isolated split operations.
        // T103: Use system temp dir to avoid fragile dir + '/../' pattern.
        var worktreePath = worktreeTmpPath('osm-worktree-');
        var wtAdd = await gitExecAsync(dir, ['worktree', 'add', '--detach', worktreePath, plan.baseBranch]);
        if (wtAdd.code !== 0) {
            return { error: 'create worktree failed: ' + wtAdd.stderr.trim(), results: [] };
        }

        async function cleanupWorktreeAsync() {
            await gitExecAsync(dir, ['worktree', 'remove', '--force', worktreePath]);
        }

        // INVARIANT (T108): Split branches form a CUMULATIVE CHAIN.
        // Each split[i] is based on split[i-1] (or plan.baseBranch for i=0).
        // After creating split[i], we set `currentBase = split[i].name` so the
        // next iteration branches from the previous split. The final split
        // therefore contains ALL changes from all prior splits PLUS its own.
        // This is why verifyEquivalence only compares the LAST split's tree
        // to the source branch's tree.
        var currentBase = plan.baseBranch;

        for (var i = 0; i < plan.splits.length; i++) {
            if (isCancelledHelper() || isForceCancelledHelper()) {  // T117: honor force-cancel
                await cleanupWorktreeAsync();
                return { error: 'cancelled by user after ' + i + ' of ' + plan.splits.length + ' branches', results: results };
            }

            var split = plan.splits[i];
            var splitResult = { name: split.name, files: split.files, sha: '', error: null, skippedFiles: [] };

            if (progressFn) {
                progressFn('Creating branch ' + (i + 1) + '/' + plan.splits.length + ': ' + split.name);
            }
            var co = await gitExecAsync(worktreePath, ['checkout', '-b', split.name, currentBase]);
            if (co.code !== 0) {
                splitResult.error = 'create branch ' + split.name + ' from ' + currentBase + ' failed: ' + co.stderr.trim();
                results.push(splitResult);
                await cleanupWorktreeAsync();
                return { error: splitResult.error, results: results };
            }

            var actions = collectSplitFileActions(split, fileStatuses, fileRenames, ignoredFiles, splitResult, progressFn);
            if (actions.error) {
                splitResult.error = actions.error;
                results.push(splitResult);
                await cleanupWorktreeAsync();
                return { error: splitResult.error, results: results };
            }

            if (actions.removeFiles.length > 0) {
                var rmRes = await gitExecAsync(worktreePath, ['rm', '--ignore-unmatch', '-f'].concat(actions.removeFiles));
                if (rmRes.code !== 0) {
                    splitResult.error = 'git rm ' + actions.removeFiles.join(' ') + ': ' + rmRes.stderr.trim();
                    results.push(splitResult);
                    await cleanupWorktreeAsync();
                    return { error: splitResult.error, results: results };
                }
            }

            if (actions.checkoutFiles.length > 0) {
                var checkoutRes = await gitExecAsync(worktreePath, ['checkout', plan.sourceBranch, '--'].concat(actions.checkoutFiles));
                if (checkoutRes.code !== 0) {
                    splitResult.error = 'checkout files failed: ' + checkoutRes.stderr.trim();
                    results.push(splitResult);
                    await cleanupWorktreeAsync();
                    return { error: splitResult.error, results: results };
                }
            }

            if (actions.addFiles.length > 0) {
                var addArgs = ['add', '--'].concat(actions.addFiles);
                var add = await gitExecAsync(worktreePath, addArgs);
                if (add.code !== 0) {
                    splitResult.error = 'git add failed: ' + add.stderr.trim();
                    results.push(splitResult);
                    await cleanupWorktreeAsync();
                    return { error: splitResult.error, results: results };
                }
            }

            var msg = split.message || 'split: ' + split.name;
            var commit = await gitExecAsync(worktreePath, ['commit', '-m', msg]);
            if (commit.code !== 0) {
                splitResult.error = 'git commit failed: ' + commit.stderr.trim();
                results.push(splitResult);
                await cleanupWorktreeAsync();
                return { error: splitResult.error, results: results };
            }

            var sha = await gitExecAsync(worktreePath, ['rev-parse', 'HEAD']);
            splitResult.sha = sha.code === 0 ? sha.stdout.trim() : '';

            results.push(splitResult);

            if (progressFn) {
                progressFn('Branch ' + (i + 1) + '/' + plan.splits.length + ' created: ' + split.name);
            }

            currentBase = split.name;
        }

        await cleanupWorktreeAsync();

        // T107: Collect all git-ignored files across all splits for top-level reporting.
        var overallSkipped = [];
        for (var oi = 0; oi < results.length; oi++) {
            if (results[oi].skippedFiles && results[oi].skippedFiles.length > 0) {
                overallSkipped = overallSkipped.concat(results[oi].skippedFiles);
            }
        }
        return { error: null, results: results, overallSkippedFiles: overallSkipped };
    }

    // --- Exports ---
    prSplit.executeSplit = executeSplit;
    prSplit.executeSplitAsync = executeSplitAsync;
})(globalThis.prSplit);
