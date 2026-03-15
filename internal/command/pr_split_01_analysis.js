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
    var gitExecAsync = prSplit._gitExecAsync;
    var resolveDir = prSplit._resolveDir;
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
        var dir = resolveDir(config.dir || '.');

        var createEmptyResult = function(error, currentBranch) {
            return {
                files: [],
                fileStatuses: {},
                skippedFiles: [],   // T098
                error: error,
                baseBranch: baseBranch,
                currentBranch: currentBranch || ''
            };
        };

        var branchResult = gitExec(dir, ['rev-parse', '--abbrev-ref', 'HEAD']);
        if (branchResult.code !== 0) {
            return createEmptyResult('failed to get current branch: ' + branchResult.stderr.trim(), '');
        }
        var currentBranch = branchResult.stdout.trim();

        var mergeBase = gitExec(dir, ['merge-base', baseBranch, currentBranch]);
        if (mergeBase.code !== 0) {
            return createEmptyResult('merge-base failed: ' + mergeBase.stderr.trim(), currentBranch);
        }

        // Use --name-status to capture diff status (A/M/D/R/C) per file.
        var diffResult = gitExec(dir, ['diff', '--name-status', mergeBase.stdout.trim(), currentBranch]);
        if (diffResult.code !== 0) {
            return createEmptyResult('git diff failed: ' + diffResult.stderr.trim(), currentBranch);
        }

        var raw = diffResult.stdout.trim();
        var files = [];
        var fileStatuses = {};
        var skippedFiles = [];  // T098: files with unknown git status codes

        // Valid status codes that executeSplit knows how to handle.
        // A=Added, M=Modified, D=Deleted, R=Renamed, C=Copied, T=Type-changed.
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
                    return createEmptyResult(
                        'unmerged path detected: ' + parts[1] +
                        ' — resolve all merge conflicts before splitting',
                        currentBranch
                    );
                }

                // T098: Track files with unknown status codes so callers can
                // surface a warning instead of silently losing files.
                if (!KNOWN_STATUSES[status]) {
                    skippedFiles.push({ path: parts[1], status: parts[0] });
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
            skippedFiles: skippedFiles,
            error: null,
            baseBranch: baseBranch,
            currentBranch: currentBranch
        };
    }

    // analyzeDiffAsync is the non-blocking version of analyzeDiff.
    // Uses gitExecAsync (exec.spawn) so the event loop stays responsive during BubbleTea TUI.
    // T31: async version for pipeline use.
    async function analyzeDiffAsync(config) {
        var gitExecAsync = prSplit._gitExecAsync;
        config = config || {};
        var baseBranch = config.baseBranch || runtime.baseBranch;
        var dir = resolveDir(config.dir || '.');

        var createEmptyResult = function(error, currentBranch) {
            return {
                files: [],
                fileStatuses: {},
                skippedFiles: [],   // T098
                error: error,
                baseBranch: baseBranch,
                currentBranch: currentBranch || ''
            };
        };

        var branchResult = await gitExecAsync(dir, ['rev-parse', '--abbrev-ref', 'HEAD']);
        if (branchResult.code !== 0) {
            return createEmptyResult('failed to get current branch: ' + branchResult.stderr.trim(), '');
        }
        var currentBranch = branchResult.stdout.trim();

        var mergeBase = await gitExecAsync(dir, ['merge-base', baseBranch, currentBranch]);
        if (mergeBase.code !== 0) {
            return createEmptyResult('merge-base failed: ' + mergeBase.stderr.trim(), currentBranch);
        }

        var diffResult = await gitExecAsync(dir, ['diff', '--name-status', mergeBase.stdout.trim(), currentBranch]);
        if (diffResult.code !== 0) {
            return createEmptyResult('git diff failed: ' + diffResult.stderr.trim(), currentBranch);
        }

        var raw = diffResult.stdout.trim();
        var files = [];
        var fileStatuses = {};
        var skippedFiles = [];  // T098

        // A=Added, M=Modified, D=Deleted, R=Renamed, C=Copied, T=Type-changed.
        var KNOWN_STATUSES = { A: 1, M: 1, D: 1, R: 1, C: 1, T: 1 };

        if (raw !== '') {
            var lines = raw.split('\n');
            for (var i = 0; i < lines.length; i++) {
                var line = lines[i];
                if (line === '') continue;
                var parts = line.split('\t');
                if (parts.length < 2) continue;
                var status = parts[0].charAt(0);

                if (status === 'U') {
                    return createEmptyResult(
                        'unmerged path detected: ' + parts[1] +
                        ' — resolve all merge conflicts before splitting',
                        currentBranch
                    );
                }

                // T098: Track files with unknown status codes.
                if (!KNOWN_STATUSES[status]) {
                    skippedFiles.push({ path: parts[1], status: parts[0] });
                    if (typeof log !== 'undefined') {
                        log.warn('pr-split: unknown git status "' + parts[0] + '" for ' + parts[1] + ' — skipping');
                    }
                    continue;
                }

                if (parts.length >= 3 && (status === 'R' || status === 'C')) {
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
            skippedFiles: skippedFiles,
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
        var dir = resolveDir(config.dir || '.');

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
                    // T100: git diff --numstat outputs '- - path' for binary
                    // files.  parseInt('-', 10) returns NaN, which `|| 0`
                    // silently coerced to 0 — making binary files appear
                    // weightless in size-based grouping strategies.  Detect
                    // this and flag the file as binary with null
                    // additions/deletions so callers can distinguish binary
                    // from genuinely-empty files.
                    var isBinary = (parts[0] === '-' && parts[1] === '-');
                    var entry = {
                        name: parts[2],
                        additions: isBinary ? null : (parseInt(parts[0], 10) || 0),
                        deletions: isBinary ? null : (parseInt(parts[1], 10) || 0)
                    };
                    if (isBinary) {
                        entry.binary = true;
                    }
                    files.push(entry);
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
    prSplit.analyzeDiffAsync = analyzeDiffAsync;
    prSplit.analyzeDiffStats = analyzeDiffStats;
})(globalThis.prSplit);
