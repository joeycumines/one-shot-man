'use strict';
// pr_split_05_execution.js — executeSplit
// Dependencies: chunks 00 (_gitExec, isCancelled), 04 (validatePlan).
// Attaches to prSplit: executeSplit.

(function(prSplit) {
    var gitExec = prSplit._gitExec;
    var gitExecAsync = prSplit._gitExecAsync;
    var resolveDir = prSplit._resolveDir;
    var validatePlan = prSplit.validatePlan;

    // -----------------------------------------------------------------------
    //  executeSplit — creates branches for each split in a plan
    //
    //  All branch operations run in a temporary git worktree so the user's
    //  working directory remains completely untouched.
    // -----------------------------------------------------------------------
    function executeSplit(plan, options) {
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
            var checkResult = gitExec(dir, checkArgs);
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
            var existCheck = gitExec(dir, ['rev-parse', '--verify', 'refs/heads/' + plan.splits[k].name]);
            if (existCheck.code === 0) {
                gitExec(dir, ['branch', '-D', plan.splits[k].name]);
            }
        }

        // Create a temporary git worktree for isolated split operations.
        // The user's CWD remains completely untouched.
        var worktreePath = dir + '/../.osm-worktree-' + Date.now();
        var wtAdd = gitExec(dir, ['worktree', 'add', '--detach', worktreePath, plan.baseBranch]);
        if (wtAdd.code !== 0) {
            return { error: 'create worktree failed: ' + wtAdd.stderr.trim(), results: [] };
        }

        function cleanupWorktree() {
            gitExec(dir, ['worktree', 'remove', '--force', worktreePath]);
        }

        var currentBase = plan.baseBranch;

        for (var i = 0; i < plan.splits.length; i++) {
            if (prSplit.isCancelled()) {
                cleanupWorktree();
                return { error: 'cancelled by user after ' + i + ' of ' + plan.splits.length + ' branches', results: results };
            }

            var split = plan.splits[i];
            var splitResult = { name: split.name, files: split.files, sha: '', error: null, skippedFiles: [] };

            if (progressFn) {
                progressFn('Creating branch ' + (i + 1) + '/' + plan.splits.length + ': ' + split.name);
            }

            var co = gitExec(worktreePath, ['checkout', '-b', split.name, currentBase]);
            if (co.code !== 0) {
                splitResult.error = 'create branch ' + split.name + ' from ' + currentBase + ' failed: ' + co.stderr.trim();
                results.push(splitResult);
                cleanupWorktree();
                return { error: splitResult.error, results: results };
            }

            for (var j = 0; j < split.files.length; j++) {
                if (prSplit.isCancelled()) {
                    splitResult.error = 'cancelled by user after ' + j + ' of ' + split.files.length + ' files in ' + split.name;
                    results.push(splitResult);
                    cleanupWorktree();
                    return { error: splitResult.error, results: results };
                }

                var file = split.files[j];

                // Skip git-ignored files detected during pre-validation.
                if (ignoredFiles[file]) {
                    splitResult.skippedFiles.push(file);
                    if (typeof log !== 'undefined' && log.warn) {
                        log.warn('pr-split: skipping git-ignored file in ' + split.name + ': ' + file);
                    }
                    continue;
                }

                var status = fileStatuses[file];

                if (progressFn && split.files.length > 5) {
                    progressFn('  ' + split.name + ': file ' + (j + 1) + '/' + split.files.length);
                }

                if (!status) {
                    splitResult.error = 'file "' + file + '" has no entry in plan.fileStatuses — '
                        + 'ensure analyzeDiff() results are passed to createSplitPlan()';
                    results.push(splitResult);
                    cleanupWorktree();
                    return { error: splitResult.error, results: results };
                }

                if (status === 'D') {
                    var rm = gitExec(worktreePath, ['rm', '--ignore-unmatch', '-f', file]);
                    if (rm.code !== 0) {
                        splitResult.error = 'git rm ' + file + ': ' + rm.stderr.trim();
                        results.push(splitResult);
                        cleanupWorktree();
                        return { error: splitResult.error, results: results };
                    }
                } else {
                    // File was added, modified, renamed-to, copied-to, or type-changed.
                    if (status === 'T' && typeof log !== 'undefined' && log.warn) {
                        log.warn('pr-split: file type change for ' + file + ' — checkout from source will restore new type');
                    }
                    var checkout = gitExec(worktreePath, ['checkout', plan.sourceBranch, '--', file]);
                    if (checkout.code !== 0) {
                        splitResult.error = 'checkout file ' + file + ': ' + checkout.stderr.trim();
                        results.push(splitResult);
                        cleanupWorktree();
                        return { error: splitResult.error, results: results };
                    }
                }
            }

            // Stage only this split's files defensively (skip ignored + deleted).
            var addFiles = [];
            for (var af = 0; af < split.files.length; af++) {
                var afFile = split.files[af];
                if (fileStatuses[afFile] !== 'D' && !ignoredFiles[afFile]) {
                    addFiles.push(afFile);
                }
            }
            if (addFiles.length > 0) {
                var addArgs = ['add', '--'].concat(addFiles);
                var add = gitExec(worktreePath, addArgs);
                if (add.code !== 0) {
                    splitResult.error = 'git add failed: ' + add.stderr.trim();
                    results.push(splitResult);
                    cleanupWorktree();
                    return { error: splitResult.error, results: results };
                }
            }

            var msg = split.message || 'split: ' + split.name;
            var commit = gitExec(worktreePath, ['commit', '-m', msg]);
            if (commit.code !== 0) {
                var commitAllow = gitExec(worktreePath, ['commit', '--allow-empty', '-m', msg]);
                if (commitAllow.code !== 0) {
                    splitResult.error = 'git commit failed: ' + commitAllow.stderr.trim();
                    results.push(splitResult);
                    cleanupWorktree();
                    return { error: splitResult.error, results: results };
                }
            }

            var sha = gitExec(worktreePath, ['rev-parse', 'HEAD']);
            splitResult.sha = sha.code === 0 ? sha.stdout.trim() : '';

            results.push(splitResult);

            if (progressFn) {
                progressFn('Branch ' + (i + 1) + '/' + plan.splits.length + ' created: ' + split.name);
            }

            currentBase = split.name;
        }

        cleanupWorktree();
        return { error: null, results: results };
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
        var worktreePath = dir + '/../.osm-worktree-' + Date.now();
        var wtAdd = await gitExecAsync(dir, ['worktree', 'add', '--detach', worktreePath, plan.baseBranch]);
        if (wtAdd.code !== 0) {
            return { error: 'create worktree failed: ' + wtAdd.stderr.trim(), results: [] };
        }

        async function cleanupWorktreeAsync() {
            await gitExecAsync(dir, ['worktree', 'remove', '--force', worktreePath]);
        }

        var currentBase = plan.baseBranch;

        for (var i = 0; i < plan.splits.length; i++) {
            if (prSplit.isCancelled()) {
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

            for (var j = 0; j < split.files.length; j++) {
                if (prSplit.isCancelled()) {
                    splitResult.error = 'cancelled by user after ' + j + ' of ' + split.files.length + ' files in ' + split.name;
                    results.push(splitResult);
                    await cleanupWorktreeAsync();
                    return { error: splitResult.error, results: results };
                }

                var file = split.files[j];

                if (ignoredFiles[file]) {
                    splitResult.skippedFiles.push(file);
                    if (typeof log !== 'undefined' && log.warn) {
                        log.warn('pr-split: skipping git-ignored file in ' + split.name + ': ' + file);
                    }
                    continue;
                }

                var status = fileStatuses[file];

                if (progressFn && split.files.length > 5) {
                    progressFn('  ' + split.name + ': file ' + (j + 1) + '/' + split.files.length);
                }

                if (!status) {
                    splitResult.error = 'file "' + file + '" has no entry in plan.fileStatuses — '
                        + 'ensure analyzeDiff() results are passed to createSplitPlan()';
                    results.push(splitResult);
                    await cleanupWorktreeAsync();
                    return { error: splitResult.error, results: results };
                }

                if (status === 'D') {
                    var rm = await gitExecAsync(worktreePath, ['rm', '--ignore-unmatch', '-f', file]);
                    if (rm.code !== 0) {
                        splitResult.error = 'git rm ' + file + ': ' + rm.stderr.trim();
                        results.push(splitResult);
                        await cleanupWorktreeAsync();
                        return { error: splitResult.error, results: results };
                    }
                } else {
                    if (status === 'T' && typeof log !== 'undefined' && log.warn) {
                        log.warn('pr-split: file type change for ' + file + ' — checkout from source will restore new type');
                    }
                    var checkout = await gitExecAsync(worktreePath, ['checkout', plan.sourceBranch, '--', file]);
                    if (checkout.code !== 0) {
                        splitResult.error = 'checkout file ' + file + ': ' + checkout.stderr.trim();
                        results.push(splitResult);
                        await cleanupWorktreeAsync();
                        return { error: splitResult.error, results: results };
                    }
                }
            }

            // Stage only this split's files defensively.
            var addFiles = [];
            for (var af = 0; af < split.files.length; af++) {
                var afFile = split.files[af];
                if (fileStatuses[afFile] !== 'D' && !ignoredFiles[afFile]) {
                    addFiles.push(afFile);
                }
            }
            if (addFiles.length > 0) {
                var addArgs = ['add', '--'].concat(addFiles);
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
                var commitAllow = await gitExecAsync(worktreePath, ['commit', '--allow-empty', '-m', msg]);
                if (commitAllow.code !== 0) {
                    splitResult.error = 'git commit failed: ' + commitAllow.stderr.trim();
                    results.push(splitResult);
                    await cleanupWorktreeAsync();
                    return { error: splitResult.error, results: results };
                }
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
        return { error: null, results: results };
    }

    // -----------------------------------------------------------------------
    //  Exports
    // -----------------------------------------------------------------------
    prSplit.executeSplit = executeSplit;
    prSplit.executeSplitAsync = executeSplitAsync;
})(globalThis.prSplit);
