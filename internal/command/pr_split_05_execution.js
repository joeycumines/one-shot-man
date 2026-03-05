'use strict';
// pr_split_05_execution.js — executeSplit
// Dependencies: chunks 00 (_gitExec, isCancelled), 04 (validatePlan).
// Attaches to prSplit: executeSplit.

(function(prSplit) {
    var gitExec = prSplit._gitExec;
    var resolveDir = prSplit._resolveDir;
    var validatePlan = prSplit.validatePlan;

    // -----------------------------------------------------------------------
    //  executeSplit — creates branches for each split in a plan
    // -----------------------------------------------------------------------
    function executeSplit(plan, options) {
        options = options || {};
        var dir = resolveDir(plan.dir || '.');
        var results = [];
        var progressFn = options.progressFn || null;

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

        var savedBranch = gitExec(dir, ['rev-parse', '--abbrev-ref', 'HEAD']);
        if (savedBranch.code !== 0) {
            return { error: 'failed to get current branch', results: [] };
        }
        var restoreBranch = savedBranch.stdout.trim();

        // Pre-flight: delete any pre-existing split branches to allow re-runs.
        for (var k = 0; k < plan.splits.length; k++) {
            var existCheck = gitExec(dir, ['rev-parse', '--verify', 'refs/heads/' + plan.splits[k].name]);
            if (existCheck.code === 0) {
                gitExec(dir, ['branch', '-D', plan.splits[k].name]);
            }
        }

        var currentBase = plan.baseBranch;

        for (var i = 0; i < plan.splits.length; i++) {
            if (prSplit.isCancelled()) {
                gitExec(dir, ['checkout', restoreBranch]);
                return { error: 'cancelled by user after ' + i + ' of ' + plan.splits.length + ' branches', results: results };
            }

            var split = plan.splits[i];
            var splitResult = { name: split.name, files: split.files, sha: '', error: null };

            if (progressFn) {
                progressFn('Creating branch ' + (i + 1) + '/' + plan.splits.length + ': ' + split.name);
            }

            var co = gitExec(dir, ['checkout', '-b', split.name, currentBase]);
            if (co.code !== 0) {
                splitResult.error = 'create branch ' + split.name + ' from ' + currentBase + ' failed: ' + co.stderr.trim();
                results.push(splitResult);
                gitExec(dir, ['checkout', restoreBranch]);
                return { error: splitResult.error, results: results };
            }

            for (var j = 0; j < split.files.length; j++) {
                if (prSplit.isCancelled()) {
                    splitResult.error = 'cancelled by user after ' + j + ' of ' + split.files.length + ' files in ' + split.name;
                    results.push(splitResult);
                    gitExec(dir, ['checkout', restoreBranch]);
                    return { error: splitResult.error, results: results };
                }

                var file = split.files[j];
                var status = fileStatuses[file];

                if (progressFn && split.files.length > 5) {
                    progressFn('  ' + split.name + ': file ' + (j + 1) + '/' + split.files.length);
                }

                if (!status) {
                    splitResult.error = 'file "' + file + '" has no entry in plan.fileStatuses — '
                        + 'ensure analyzeDiff() results are passed to createSplitPlan()';
                    results.push(splitResult);
                    gitExec(dir, ['checkout', restoreBranch]);
                    return { error: splitResult.error, results: results };
                }

                if (status === 'D') {
                    var rm = gitExec(dir, ['rm', '--ignore-unmatch', '-f', file]);
                    if (rm.code !== 0) {
                        splitResult.error = 'git rm ' + file + ': ' + rm.stderr.trim();
                        results.push(splitResult);
                        gitExec(dir, ['checkout', restoreBranch]);
                        return { error: splitResult.error, results: results };
                    }
                } else {
                    // File was added, modified, renamed-to, copied-to, or type-changed.
                    if (status === 'T' && typeof log !== 'undefined' && log.warn) {
                        log.warn('pr-split: file type change for ' + file + ' — checkout from source will restore new type');
                    }
                    var checkout = gitExec(dir, ['checkout', plan.sourceBranch, '--', file]);
                    if (checkout.code !== 0) {
                        splitResult.error = 'checkout file ' + file + ': ' + checkout.stderr.trim();
                        results.push(splitResult);
                        gitExec(dir, ['checkout', restoreBranch]);
                        return { error: splitResult.error, results: results };
                    }
                }
            }

            // Stage only this split's files defensively.
            var addFiles = [];
            for (var af = 0; af < split.files.length; af++) {
                if (fileStatuses[split.files[af]] !== 'D') {
                    addFiles.push(split.files[af]);
                }
            }
            if (addFiles.length > 0) {
                var addArgs = ['add', '--'].concat(addFiles);
                var add = gitExec(dir, addArgs);
                if (add.code !== 0) {
                    splitResult.error = 'git add failed: ' + add.stderr.trim();
                    results.push(splitResult);
                    gitExec(dir, ['checkout', restoreBranch]);
                    return { error: splitResult.error, results: results };
                }
            }

            var msg = split.message || 'split: ' + split.name;
            var commit = gitExec(dir, ['commit', '-m', msg]);
            if (commit.code !== 0) {
                var commitAllow = gitExec(dir, ['commit', '--allow-empty', '-m', msg]);
                if (commitAllow.code !== 0) {
                    splitResult.error = 'git commit failed: ' + commitAllow.stderr.trim();
                    results.push(splitResult);
                    gitExec(dir, ['checkout', restoreBranch]);
                    return { error: splitResult.error, results: results };
                }
            }

            var sha = gitExec(dir, ['rev-parse', 'HEAD']);
            splitResult.sha = sha.code === 0 ? sha.stdout.trim() : '';

            results.push(splitResult);

            if (progressFn) {
                progressFn('Branch ' + (i + 1) + '/' + plan.splits.length + ' created: ' + split.name);
            }

            currentBase = split.name;
        }

        gitExec(dir, ['checkout', restoreBranch]);
        return { error: null, results: results };
    }

    // -----------------------------------------------------------------------
    //  Exports
    // -----------------------------------------------------------------------
    prSplit.executeSplit = executeSplit;
})(globalThis.prSplit);
