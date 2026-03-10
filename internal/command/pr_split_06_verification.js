'use strict';
// pr_split_06_verification.js — Verification, equivalence check, and cleanup
// Dependencies: chunks 00 (_gitExec, _shellQuote, isCancelled, scopedVerifyCommand,
//   runtime, _modules.exec).
// Attaches to prSplit: verifySplit, verifySplits, verifyEquivalence,
//   verifyEquivalenceDetailed, cleanupBranches.

(function(prSplit) {
    var gitExec = prSplit._gitExec;
    var resolveDir = prSplit._resolveDir;
    var shellQuote = prSplit._shellQuote;
    var scopedVerifyCommand = prSplit.scopedVerifyCommand;
    var exec = prSplit._modules.exec;

    // -----------------------------------------------------------------------
    //  verifySplit — runs verify command on a single branch
    //
    //  Uses a temporary git worktree so the user's CWD is never modified.
    //  Tries proper branch checkout first (preserves branch name for verify
    //  commands that inspect HEAD), falls back to detached HEAD if the
    //  branch is already checked out elsewhere.
    // -----------------------------------------------------------------------
    function verifySplit(branchName, config) {
        config = config || {};
        var dir = resolveDir(config.dir || '.');
        var command = config.verifyCommand || prSplit.runtime.verifyCommand;
        var timeoutMs = config.verifyTimeoutMs || 0;
        var outputFn = config.outputFn || null;

        // No verify command configured — treat as pass (verification skipped).
        if (!command) {
            return { name: branchName, passed: true, output: '', error: null, skipped: true, duration: 0 };
        }

        // Create a temporary worktree for this branch.
        var worktreeDir = dir + '/../.osm-verify-' + Date.now() + '-' + Math.floor(Math.random() * 10000);

        // Try proper branch checkout first (preserves branch name in HEAD).
        var wtAdd = gitExec(dir, ['worktree', 'add', worktreeDir, branchName]);
        if (wtAdd.code !== 0) {
            // Branch might be checked out elsewhere (user's CWD); fallback to detached HEAD.
            gitExec(dir, ['worktree', 'remove', '--force', worktreeDir]);
            wtAdd = gitExec(dir, ['worktree', 'add', '--detach', worktreeDir, branchName]);
            if (wtAdd.code !== 0) {
                return {
                    name: branchName,
                    passed: false,
                    output: '',
                    error: 'create worktree failed: ' + wtAdd.stderr.trim()
                };
            }
        }

        function cleanupWorktree() {
            gitExec(dir, ['worktree', 'remove', '--force', worktreeDir]);
        }

        var startMs = Date.now();
        var shellCmd = 'cd ' + shellQuote(worktreeDir) + ' && ' + command;

        if (timeoutMs > 0) {
            var timeoutSec = Math.ceil(timeoutMs / 1000);
            shellCmd = 'timeout ' + timeoutSec + ' sh -c ' + shellQuote(shellCmd);
        }

        var stdoutBuf = '';
        var stderrBuf = '';
        var prefix = '  [verify ' + branchName + '] ';

        function emitChunk(chunk) {
            if (!outputFn) return;
            var lines = chunk.split('\n');
            for (var i = 0; i < lines.length; i++) {
                if (lines[i]) {
                    outputFn(prefix + lines[i]);
                }
            }
        }

        var result = exec.execStream(['sh', '-c', shellCmd], {
            onStdout: function(chunk) {
                stdoutBuf += chunk;
                emitChunk(chunk);
            },
            onStderr: function(chunk) {
                stderrBuf += chunk;
                emitChunk(chunk);
            }
        });
        var elapsedMs = Date.now() - startMs;

        cleanupWorktree();

        if (timeoutMs > 0 && (result.code === 124 || elapsedMs >= timeoutMs)) {
            return {
                name: branchName,
                passed: false,
                output: stdoutBuf,
                error: 'verify timeout after ' + elapsedMs + 'ms (limit: ' + timeoutMs + 'ms)'
            };
        }

        return {
            name: branchName,
            passed: result.code === 0,
            output: stdoutBuf,
            error: result.code !== 0 ? 'verify failed (exit ' + result.code + '): ' + stderrBuf : null
        };
    }

    // -----------------------------------------------------------------------
    //  verifySplits — runs verification for all splits, respects cancellation
    //
    //  Each branch is verified in its own temporary worktree (via verifySplit),
    //  so the user's CWD is never modified.
    // -----------------------------------------------------------------------
    function verifySplits(plan, options) {
        options = options || {};
        if (!plan || !plan.splits) {
            return { allPassed: false, results: [], error: 'verifySplits: invalid plan — missing splits array' };
        }
        var dir = resolveDir(plan.dir || '.');
        var verifyTimeoutMs = options.verifyTimeoutMs || 0;
        var onBranchStart = typeof options.onBranchStart === 'function' ? options.onBranchStart : null;
        var onBranchDone = typeof options.onBranchDone === 'function' ? options.onBranchDone : null;
        var onBranchOutput = typeof options.onBranchOutput === 'function' ? options.onBranchOutput : null;
        var results = [];
        var allPassed = true;
        var failedBranches = {};

        // Pre-existing failure detection: run verification on the source
        // branch first. If it fails, split branch failures are flagged as
        // pre-existing rather than new regressions.
        var baselineFailure = null;
        if (plan.verifyCommand && plan.sourceBranch) {
            var baselineResult = verifySplit(plan.sourceBranch, {
                dir: dir,
                verifyCommand: plan.verifyCommand,
                verifyTimeoutMs: verifyTimeoutMs,
                outputFn: options.outputFn || null
            });
            if (!baselineResult.passed) {
                baselineFailure = baselineResult;
            }
        }

        for (var i = 0; i < plan.splits.length; i++) {
            if (prSplit.isCancelled()) {
                return { allPassed: false, results: results, error: 'verification cancelled by user' };
            }

            var split = plan.splits[i];

            // Skip if any dependency failed.
            var deps = split.dependencies || [];
            var skipReason = '';
            for (var d = 0; d < deps.length; d++) {
                if (failedBranches[deps[d]]) {
                    skipReason = 'skipped: dependency ' + deps[d] + ' failed';
                    break;
                }
            }
            if (skipReason) {
                if (onBranchDone) { onBranchDone(split.name, false, 0, 0, true, false); }
                results.push({ name: split.name, passed: false, skipped: true, error: skipReason });
                allPassed = false;
                failedBranches[split.name] = true;
                continue;
            }

            var baseCmd = plan.verifyCommand;
            var scopedCmd = scopedVerifyCommand(split.files, baseCmd);

            if (onBranchStart) { onBranchStart(split.name); }
            var branchStartTime = Date.now();

            var branchOutputFn = options.outputFn || null;
            if (onBranchOutput) {
                var baseFn = branchOutputFn;
                var brName = split.name;
                branchOutputFn = function(line) {
                    if (baseFn) { baseFn(line); }
                    onBranchOutput(brName, line);
                };
            }
            var result = verifySplit(split.name, {
                dir: dir,
                verifyCommand: scopedCmd,
                verifyTimeoutMs: verifyTimeoutMs,
                outputFn: branchOutputFn
            });
            if (scopedCmd !== baseCmd) {
                result.scopedVerify = scopedCmd;
            }

            // Mark pre-existing failures.
            if (!result.passed && baselineFailure) {
                result.preExisting = true;
                result.error = (result.error || 'verification failed') + ' (pre-existing on ' + plan.sourceBranch + ')';
            }

            var branchElapsedMs = Date.now() - branchStartTime;
            if (onBranchDone) {
                onBranchDone(split.name, result.passed, result.exitCode || 0, branchElapsedMs, false, !!result.preExisting);
            }

            results.push(result);
            if (!result.passed && !result.preExisting) {
                allPassed = false;
                failedBranches[split.name] = true;
            }
        }

        return { allPassed: allPassed, results: results, error: null };
    }

    // -----------------------------------------------------------------------
    //  verifyEquivalence — tree-SHA comparison between splits and source
    // -----------------------------------------------------------------------
    function verifyEquivalence(plan) {
        if (!plan) {
            return { equivalent: false, splitTree: '', sourceTree: '', error: 'invalid plan' };
        }
        var dir = resolveDir(plan.dir || '.');

        if (!plan.splits || plan.splits.length === 0) {
            return { equivalent: false, splitTree: '', sourceTree: '', error: 'no splits in plan' };
        }

        var lastSplit = plan.splits[plan.splits.length - 1].name;

        var splitTreeResult = gitExec(dir, ['rev-parse', lastSplit + '^{tree}']);
        if (splitTreeResult.code !== 0) {
            return {
                equivalent: false,
                splitTree: '',
                sourceTree: '',
                error: 'failed to get split tree: ' + splitTreeResult.stderr.trim()
            };
        }
        var splitTree = splitTreeResult.stdout.trim();

        var sourceTreeResult = gitExec(dir, ['rev-parse', plan.sourceBranch + '^{tree}']);
        if (sourceTreeResult.code !== 0) {
            return {
                equivalent: false,
                splitTree: splitTree,
                sourceTree: '',
                error: 'failed to get source tree: ' + sourceTreeResult.stderr.trim()
            };
        }
        var sourceTree = sourceTreeResult.stdout.trim();

        return {
            equivalent: splitTree === sourceTree,
            splitTree: splitTree,
            sourceTree: sourceTree,
            error: null
        };
    }

    // -----------------------------------------------------------------------
    //  verifyEquivalenceDetailed — adds per-file diff info on mismatch
    // -----------------------------------------------------------------------
    function verifyEquivalenceDetailed(plan) {
        if (!plan) {
            return { equivalent: false, splitTree: '', sourceTree: '', error: 'invalid plan', diffFiles: [], diffSummary: '' };
        }
        var dir = resolveDir(plan.dir || '.');
        var base = verifyEquivalence(plan);

        if (base.error || base.equivalent) {
            base.diffFiles = [];
            base.diffSummary = '';
            return base;
        }

        var lastSplit = plan.splits[plan.splits.length - 1].name;
        var diffResult = gitExec(dir, ['diff', '--stat', lastSplit, plan.sourceBranch]);
        base.diffSummary = diffResult.code === 0 ? diffResult.stdout.trim() : '';

        var diffNamesResult = gitExec(dir, ['diff', '--name-only', lastSplit, plan.sourceBranch]);
        if (diffNamesResult.code === 0 && diffNamesResult.stdout.trim() !== '') {
            base.diffFiles = diffNamesResult.stdout.trim().split('\n').filter(function(f) {
                return f !== '';
            });
        } else {
            base.diffFiles = [];
        }

        return base;
    }

    // -----------------------------------------------------------------------
    //  cleanupBranches — deletes split branches
    // -----------------------------------------------------------------------
    function cleanupBranches(plan) {
        if (!plan || !plan.splits) {
            return { deleted: [], errors: ['cleanupBranches: invalid plan — missing splits array'] };
        }
        var dir = resolveDir(plan.dir || '.');
        var deleted = [];
        var errors = [];

        // Try to checkout baseBranch; fall back to detaching HEAD so
        // branch -D can still succeed.
        var coRes = gitExec(dir, ['checkout', plan.baseBranch]);
        if (coRes.code !== 0) {
            gitExec(dir, ['checkout', '--detach', 'HEAD']);
        }

        for (var i = 0; i < plan.splits.length; i++) {
            var name = plan.splits[i].name;
            var result = gitExec(dir, ['branch', '-D', name]);
            if (result.code === 0) {
                deleted.push(name);
            } else {
                errors.push(name + ': ' + result.stderr.trim());
            }
        }

        return { deleted: deleted, errors: errors };
    }

    // -----------------------------------------------------------------------
    //  startVerifySession — non-blocking variant using CaptureSession
    //
    //  Creates a temporary git worktree and spawns the verify command in a
    //  CaptureSession (PTY + VTerm). Returns immediately so the TUI can
    //  poll output via ticks. Caller is responsible for cleanup via
    //  cleanupVerifyWorktree() after the session completes.
    //
    //  Returns:
    //    { session, worktreeDir, dir, branchName, startTime }  on success
    //    { skipped: true }                                     if no verify command
    //    { error: string }                                     on failure
    // -----------------------------------------------------------------------
    function startVerifySession(branchName, config) {
        config = config || {};
        var dir = resolveDir(config.dir || '.');
        var command = config.verifyCommand || prSplit.runtime.verifyCommand;

        if (!command) {
            return { skipped: true, session: null, worktreeDir: null };
        }

        // Eagerly load termmux BEFORE creating the worktree so a missing
        // module fails fast without leaving an orphaned worktree.
        var termmux = require('osm:termmux');
        var rows = config.rows || 24;
        var cols = config.cols || 120;

        // Create a temporary worktree for this branch.
        var worktreeDir = dir + '/../.osm-verify-' + Date.now() + '-' + Math.floor(Math.random() * 10000);

        var wtAdd = gitExec(dir, ['worktree', 'add', worktreeDir, branchName]);
        if (wtAdd.code !== 0) {
            // Branch might be checked out elsewhere; fallback to detached HEAD.
            gitExec(dir, ['worktree', 'remove', '--force', worktreeDir]);
            wtAdd = gitExec(dir, ['worktree', 'add', '--detach', worktreeDir, branchName]);
            if (wtAdd.code !== 0) {
                return {
                    error: 'create worktree failed: ' + wtAdd.stderr.trim(),
                    session: null,
                    worktreeDir: null
                };
            }
        }

        try {
            var session = termmux.newCaptureSession('sh', ['-c', command], {
                dir: worktreeDir,
                rows: rows,
                cols: cols
            });
            session.start();
        } catch (e) {
            gitExec(dir, ['worktree', 'remove', '--force', worktreeDir]);
            return {
                error: 'start verify session failed: ' + e.message,
                session: null,
                worktreeDir: null
            };
        }

        return {
            session: session,
            worktreeDir: worktreeDir,
            dir: dir,
            branchName: branchName,
            startTime: Date.now()
        };
    }

    // cleanupVerifyWorktree removes a temporary worktree created by
    // startVerifySession.
    function cleanupVerifyWorktree(dir, worktreeDir) {
        gitExec(dir, ['worktree', 'remove', '--force', worktreeDir]);
    }

    // -----------------------------------------------------------------------
    //  Exports
    // -----------------------------------------------------------------------
    prSplit.verifySplit = verifySplit;
    prSplit.verifySplits = verifySplits;
    prSplit.verifyEquivalence = verifyEquivalence;
    prSplit.verifyEquivalenceDetailed = verifyEquivalenceDetailed;
    prSplit.cleanupBranches = cleanupBranches;
    prSplit.startVerifySession = startVerifySession;
    prSplit.cleanupVerifyWorktree = cleanupVerifyWorktree;
})(globalThis.prSplit);
