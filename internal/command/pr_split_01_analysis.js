'use strict';
// pr_split_01_analysis.js — analyzeDiff, analyzeDiffStats
//
// Analyzes the git diff between the current branch and a base branch,
// producing file lists with status information (A/M/D/R/C/T) and
// per-file diff statistics (additions/deletions).
//
// Dependencies: chunk 00 (prSplit._gitExec, prSplit.runtime)
// Go-injected globals: none (uses chunk 00's modules only)
//
// See docs/architecture-pr-split-chunks.md for the full chunk contract.

(function(prSplit) {
    var gitExec = prSplit._gitExec;
    var runtime = prSplit.runtime;

    // analyzeDiff returns the list of changed files between the current
    // branch and the configured base branch, with per-file git status codes.
    //
    // Parameters:
    //   config.baseBranch — base branch name (default: runtime.baseBranch)
    //   config.dir        — working directory (default: '.')
    //
    // Returns:
    //   {files: string[], fileStatuses: {[path]: status}, error: string|null,
    //    baseBranch: string, currentBranch: string}
    function analyzeDiff(config) {
        config = config || {};
        var baseBranch = config.baseBranch || runtime.baseBranch;
        var dir = config.dir || '.';

        var emptyResult = function(error, currentBranch) {
            return {
                files: [],
                fileStatuses: {},
                error: error,
                baseBranch: baseBranch,
                currentBranch: currentBranch || ''
            };
        };

        var branchResult = gitExec(dir, ['rev-parse', '--abbrev-ref', 'HEAD']);
        if (branchResult.code !== 0) {
            return emptyResult('failed to get current branch: ' + branchResult.stderr.trim(), '');
        }
        var currentBranch = branchResult.stdout.trim();

        var mergeBase = gitExec(dir, ['merge-base', baseBranch, currentBranch]);
        if (mergeBase.code !== 0) {
            return emptyResult('merge-base failed: ' + mergeBase.stderr.trim(), currentBranch);
        }

        // Use --name-status to capture diff status (A/M/D/R/C) per file.
        var diffResult = gitExec(dir, ['diff', '--name-status', mergeBase.stdout.trim(), currentBranch]);
        if (diffResult.code !== 0) {
            return emptyResult('git diff failed: ' + diffResult.stderr.trim(), currentBranch);
        }

        var raw = diffResult.stdout.trim();
        var files = [];
        var fileStatuses = {};

        // Valid status codes that executeSplit knows how to handle.
        var KNOWN_STATUSES = { A: 1, M: 1, D: 1, R: 1, C: 1, T: 1 };

        if (raw !== '') {
            var lines = raw.split('\n');
            for (var i = 0; i < lines.length; i++) {
                var line = lines[i];
                if (line === '') continue;
                // Format: STATUS\tPATH (or STATUS\tOLD\tNEW for R/C)
                var parts = line.split('\t');
                if (parts.length < 2) continue;
                var status = parts[0].charAt(0);

                // Unmerged paths (U) mean unresolved conflicts — fail early.
                if (status === 'U') {
                    return emptyResult(
                        'unmerged path detected: ' + parts[1] +
                        ' — resolve all merge conflicts before splitting',
                        currentBranch
                    );
                }

                // Skip unknown status codes with a warning.
                if (!KNOWN_STATUSES[status]) {
                    if (typeof log !== 'undefined') {
                        log.warn('pr-split: unknown git status "' + parts[0] + '" for ' + parts[1] + ' — skipping');
                    }
                    continue;
                }

                if (parts.length >= 3 && (status === 'R' || status === 'C')) {
                    // Rename/copy: track ONLY the new (destination) path.
                    // The old path is irrelevant — the source branch has the
                    // final state, and executeSplit operates on source branch content.
                    var newPath = parts[2];
                    files.push(newPath);
                    fileStatuses[newPath] = status;
                } else {
                    var path = parts[1];
                    files.push(path);
                    fileStatuses[path] = status;
                }
            }
        }

        return {
            files: files,
            fileStatuses: fileStatuses,
            error: null,
            baseBranch: baseBranch,
            currentBranch: currentBranch
        };
    }

    // analyzeDiffStats returns per-file addition/deletion counts between
    // the current branch and the configured base branch.
    //
    // Parameters:
    //   config.baseBranch — base branch name (default: runtime.baseBranch)
    //   config.dir        — working directory (default: '.')
    //
    // Returns:
    //   {files: [{name, additions, deletions}], error: string|null,
    //    baseBranch: string, currentBranch: string}
    function analyzeDiffStats(config) {
        config = config || {};
        var baseBranch = config.baseBranch || runtime.baseBranch;
        var dir = config.dir || '.';

        var branchResult = gitExec(dir, ['rev-parse', '--abbrev-ref', 'HEAD']);
        if (branchResult.code !== 0) {
            return {
                files: [],
                error: 'failed to get current branch: ' + branchResult.stderr.trim(),
                baseBranch: baseBranch,
                currentBranch: ''
            };
        }
        var currentBranch = branchResult.stdout.trim();

        var mergeBase = gitExec(dir, ['merge-base', baseBranch, currentBranch]);
        if (mergeBase.code !== 0) {
            return {
                files: [],
                error: 'merge-base failed: ' + mergeBase.stderr.trim(),
                baseBranch: baseBranch,
                currentBranch: currentBranch
            };
        }

        var statResult = gitExec(dir, ['diff', '--numstat', mergeBase.stdout.trim(), currentBranch]);
        if (statResult.code !== 0) {
            return {
                files: [],
                error: 'git diff --numstat failed: ' + statResult.stderr.trim(),
                baseBranch: baseBranch,
                currentBranch: currentBranch
            };
        }

        var files = [];
        var raw = statResult.stdout.trim();
        if (raw !== '') {
            var lines = raw.split('\n');
            for (var i = 0; i < lines.length; i++) {
                if (lines[i] === '') continue;
                var parts = lines[i].split('\t');
                if (parts.length >= 3) {
                    files.push({
                        name: parts[2],
                        additions: parseInt(parts[0], 10) || 0,
                        deletions: parseInt(parts[1], 10) || 0
                    });
                }
            }
        }

        return {
            files: files,
            error: null,
            baseBranch: baseBranch,
            currentBranch: currentBranch
        };
    }

    // Attach exports to globalThis.prSplit.
    prSplit.analyzeDiff = analyzeDiff;
    prSplit.analyzeDiffStats = analyzeDiffStats;
})(globalThis.prSplit);
