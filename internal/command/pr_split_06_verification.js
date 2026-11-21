'use strict';
// pr_split_06_verification.js — Verification, equivalence check, and cleanup
// Dependencies: chunks 00 (_gitExec, _shellQuote, isCancelled, scopedVerifyCommand,
//   runtime, _modules.exec).
// Attaches to prSplit: verifySplit, verifySplits, verifyEquivalence,
//   verifyEquivalenceDetailed, cleanupBranches.

(function(prSplit) {
    var gitExec = prSplit._gitExec;
    var gitExecAsync = prSplit._gitExecAsync;
    var resolveDir = prSplit._resolveDir;
    var shellQuote = prSplit._shellQuote;
    var worktreeTmpPath = prSplit._worktreeTmpPath;
    var shellSpawnSync = prSplit._shellSpawnSync;
    var shellSpawnAsync = prSplit._shellSpawnAsync;
    var isWindows = prSplit._isWindows;
    var scopedVerifyCommand = prSplit.scopedVerifyCommand;
    var exec = prSplit._modules.exec;
    // Late-bound so test mocks of _execFailReason are honored (same pattern as
    // chunk 07): an aborted capture carries its diagnostic in result.message,
    // not stderr, so failure strings must go through execFailReason.
    var execFailReason = function(result) { return prSplit._execFailReason(result); };

    async function resolveVerifyCommand(command, dir) {
        if (command) return command;
        if (typeof prSplit.discoverVerifyCommand !== 'function') return '';
        command = await prSplit.discoverVerifyCommand(dir);
        if (command) {
            prSplit.runtime = prSplit.runtime || {};
            prSplit.runtime.verifyCommand = command;
        }
        return command || '';
    }

    function getTreeCleanupError(result) {
        return result && typeof result.treeCleanupError === 'string'
            ? result.treeCleanupError : '';
    }

    function reportTreeCleanupFailure(outcome, reason, worktreeDir, retained) {
        outcome.passed = false;
        outcome.execFailed = true;
        outcome.treeCleanupError = reason;
        outcome.cleanupFailed = true;
        outcome.worktreeDir = worktreeDir;
        if (retained) outcome.worktreeRetained = true;
        var note = 'process tree teardown failed: ' + reason + '; ' +
            (retained ? 'worktree retained at ' : 'worktree removal was attempted at ') + worktreeDir;
        outcome.error = (outcome.error ? outcome.error + '; ' : '') + 'cleanup: ' + note;
    }

    // Final removal is a verification operation in its own right. Its result
    // never supplies the verification command's exit/capture status, and an
    // attempted removal cannot establish that the directory is still present.
    // The setup fallback below intentionally has a different contract: removing
    // a possibly absent worktree before detached-add may legitimately fail.
    async function removeVerificationWorktree(runGit, dir, worktreeDir) {
        var result;
        var rejected = false;
        try {
            result = await runGit(dir, ['worktree', 'remove', '--force', worktreeDir]);
        } catch (e) {
            result = e;
            rejected = true;
        }
        var treeError = getTreeCleanupError(result);
        if (!rejected && result && result.code === 0 && result.error !== true &&
            result.truncated !== true && !treeError) return;

        var reason = rejected ? String((result && result.message) || result)
            : (result ? execFailReason(result) : '');
        if (!reason) {
            if (result && result.truncated === true) reason = 'output capture was incomplete';
            else if (result && result.code !== undefined) reason = 'exit ' + result.code;
            else reason = 'no command result';
        }
        var failure = {
            dir: dir,
            passed: false,
            execFailed: true,
            cleanupFailed: true,
            worktreeDir: worktreeDir,
            error: 'worktree removal failed: ' + reason
        };
        if (treeError) {
            failure.treeCleanupError = treeError;
            if (reason.indexOf(treeError) < 0) failure.error += '; process tree teardown failed: ' + treeError;
        }
        failure.error += '; worktree removal was attempted at ' + worktreeDir;
        return failure;
    }

    function applyRemovalFailure(outcome, failure) {
        if (!failure) return;
        outcome.passed = false;
        outcome.execFailed = true;
        outcome.cleanupFailed = true;
        outcome.worktreeDir = failure.worktreeDir;
        if (failure.treeCleanupError) outcome.treeCleanupError = failure.treeCleanupError;
        outcome.error = (outcome.error ? outcome.error + '; ' : '') + 'cleanup: ' + failure.error;
    }

    // A failed tree close can accompany exit zero or a rejected post-start
    // spawn. Do not start another command or remove its worktree in either
    // case: a direct-child status does not establish process-tree teardown.
    async function setupVerificationWorktree(runGit, dir, worktreeDir, branchName) {
        function treeFailure(result, retained) {
            var reason = getTreeCleanupError(result);
            if (!reason) return null;
            var outcome = {
                name: branchName,
                passed: false,
                output: '',
                exitCode: result.code !== undefined ? result.code : -1,
                execFailed: true,
                error: 'create worktree failed: ' + execFailReason(result)
            };
            reportTreeCleanupFailure(outcome, reason, worktreeDir, retained);
            return outcome;
        }
        var removing = false;
        try {
            var added = await runGit(dir, ['worktree', 'add', worktreeDir, branchName]);
            var failure = treeFailure(added, true);
            if (failure) return failure;
            if (added.code !== 0) {
                removing = true;
                var removed = await runGit(dir, ['worktree', 'remove', '--force', worktreeDir]);
                failure = treeFailure(removed, false);
                if (failure) return failure;
                removing = false;
                added = await runGit(dir, ['worktree', 'add', '--detach', worktreeDir, branchName]);
                failure = treeFailure(added, true);
                if (failure) return failure;
                if (added.code !== 0) {
                    return {
                        name: branchName,
                        passed: false,
                        output: '',
                        exitCode: added.code,
                        execFailed: true,
                        error: 'create worktree failed: ' + execFailReason(added)
                    };
                }
            }
            return null;
        } catch (e) {
            var failure = treeFailure(e, !removing);
            if (failure) return failure;
            throw e;
        }
    }

    function failedVerifyBatch(result, results) {
        results.push(result);
        return { allPassed: false, results: results, error: result.error || 'verification failed' };
    }

    // --- verifySplit — runs verify command on a single branch ---
    async function verifySplit(branchName, config) {
        // This function must not reject: an unhandled rejection is fatal to the
        // event loop, and callers (verifySplits, pipelines) await it without a
        // catch. Every failure — a throwing spawn, a failed worktree setup, a
        // non-exit exec error — is contained here as a structured result
        // tagged execFailed so it can never be forgiven as pre-existing.
        try {
            return await verifySplitInner(branchName, config);
        } catch (e) {
            return {
                name: branchName,
                passed: false,
                output: '',
                exitCode: -1,
                execFailed: true,
                error: 'verify exec-layer failure: ' + String((e && e.message) || e)
            };
        }
    }

    async function verifySplitInner(branchName, config) {
        config = config || {};
        var dir = resolveDir(config.dir || '.');
        var command = await resolveVerifyCommand(config.verifyCommand || prSplit.runtime.verifyCommand, dir);
        var timeoutMs = prSplit._effectiveVerifyTimeoutMs(config.verifyTimeoutMs);
        var outputFn = config.outputFn || null;

        // No verify command configured — treat as pass (verification skipped).
        if (!command) {
            return { name: branchName, passed: true, output: '', error: null, skipped: true, duration: 0 };
        }

        // Create temp worktree.
        // T103: Use system temp dir to avoid fragile dir + '/../' pattern.
        var worktreeDir = worktreeTmpPath('osm-verify-');

        var setupFailure = await setupVerificationWorktree(gitExec, dir, worktreeDir, branchName);
        if (setupFailure) return setupFailure;

        var startMs = Date.now();
        var shellCmd;
        if (isWindows()) {
            shellCmd = 'cd /d ' + shellQuote(worktreeDir) + ' && ' + command;
        } else {
            shellCmd = 'cd ' + shellQuote(worktreeDir) + ' && ' + command;
        }

        if (timeoutMs > 0 && !isWindows()) {
            var timeoutSec = Math.ceil(timeoutMs / 1000);
            // Portable shell timeout.
            shellCmd = '( ' + shellCmd + ' ) & _pid=$!; ( sleep ' + timeoutSec + '; kill $_pid 2>/dev/null ) >/dev/null 2>&1 & _watch=$!; wait $_pid; _rc=$?; kill $_watch 2>/dev/null; wait $_watch 2>/dev/null; exit $_rc';
        }
        // Note: Windows timeout is an interactive command; timeout handling
        // for Windows relies on the Go-level deadline in shellSpawnSync via
        // the native exec binding.

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

        var result = null;
        var bodyError;
        var bodyFailed = false;
        var treeCleanupError = '';
        try {
            // execv has already captured both streams. Record its lifecycle
            // result before invoking an output callback, which may throw.
            result = await shellSpawnSync(shellCmd, { timeoutMs: timeoutMs });
            treeCleanupError = getTreeCleanupError(result);
            stdoutBuf = result.stdout || '';
            stderrBuf = result.stderr || '';
            if (stdoutBuf) emitChunk(stdoutBuf);
            if (stderrBuf) emitChunk(stderrBuf);
        } catch (e) {
            bodyError = e;
            bodyFailed = true;
            treeCleanupError = treeCleanupError || getTreeCleanupError(e);
        }

        // Cleanup owns a separate lifetime. Its duration must not turn an
        // already-completed verification command into a timeout.
        var elapsedMs = Date.now() - startMs;
        var removalFailure;
        if (!treeCleanupError) {
            removalFailure = await removeVerificationWorktree(gitExec, dir, worktreeDir);
        }
        var captureCutShort = !!(result && (result.truncated === true ||
            (result.error === true && /cut short|aborted before EOF|incomplete/.test(String(result.message || '')))));
        var execLayerFailed = result && !captureCutShort && result.error === true && result.code === -1;
        var timedOut = !!(result && timeoutMs > 0 &&
            (result.code === 124 || result.code === 137 || result.code === 143 || elapsedMs >= timeoutMs));
        var outcome;

        if (bodyFailed) {
            outcome = {
                name: branchName,
                passed: false,
                output: stdoutBuf,
                exitCode: result && result.code !== undefined ? result.code : -1,
                execFailed: true,
                truncated: captureCutShort || undefined,
                timedOut: timedOut || undefined,
                error: 'verify exec-layer failure: ' + String((bodyError && bodyError.message) || bodyError)
            };
        } else if (captureCutShort) {
            // Capture completeness and timeout attribution are independent.
            outcome = {
                name: branchName,
                passed: false,
                output: stdoutBuf,
                truncated: true,
                timedOut: timedOut || undefined,
                exitCode: result.code,
                error: timedOut
                    ? 'verify timeout after ' + elapsedMs + 'ms (limit: ' + timeoutMs + 'ms); output capture was cut short before EOF'
                    : 'verify failed: ' + (result.message || 'output capture was cut short before EOF') +
                        (stderrBuf.trim() !== '' ? ': ' + stderrBuf.trim() : '')
            };
        } else if (execLayerFailed) {
            outcome = {
                name: branchName,
                passed: false,
                output: stdoutBuf,
                exitCode: result.code,
                execFailed: true,
                error: 'verify exec-layer failure: ' + execFailReason(result)
            };
        } else if (timedOut) {
            outcome = {
                name: branchName,
                passed: false,
                output: stdoutBuf,
                timedOut: true,
                exitCode: result.code,
                error: 'verify timeout after ' + elapsedMs + 'ms (limit: ' + timeoutMs + 'ms)'
            };
        } else {
            outcome = {
                name: branchName,
                passed: result.code === 0,
                output: stdoutBuf,
                exitCode: result.code,
                error: result.code !== 0
                    ? 'verify failed (exit ' + result.code + '): ' + execFailReason(result)
                    : null
            };
        }
        if (treeCleanupError) {
            reportTreeCleanupFailure(outcome, treeCleanupError, worktreeDir, true);
        } else {
            applyRemovalFailure(outcome, removalFailure);
        }
        return outcome;
    }

    // --- verifySplits — runs verification for all splits, respects cancellation ---
    //
    //  Each branch is verified in its own temporary worktree (via verifySplit),
    //  so the user's CWD is never modified.
    async function verifySplits(plan, options) {
        options = options || {};
        if (!plan || !plan.splits) {
            return { allPassed: false, results: [], error: 'verifySplits: invalid plan — missing splits array' };
        }
        var dir = resolveDir(plan.dir || '.');
        var verifyCommand = await resolveVerifyCommand(plan.verifyCommand || prSplit.runtime.verifyCommand, dir);
        if (verifyCommand) plan.verifyCommand = verifyCommand;
        var verifyTimeoutMs = prSplit._effectiveVerifyTimeoutMs(options.verifyTimeoutMs);
        var onBranchStart = typeof options.onBranchStart === 'function' ? options.onBranchStart : null;
        var onBranchDone = typeof options.onBranchDone === 'function' ? options.onBranchDone : null;
        var onBranchOutput = typeof options.onBranchOutput === 'function' ? options.onBranchOutput : null;
        var results = [];
        var allPassed = true;
        var failedBranches = {};

        // Pre-existing failure detection: run verification on the source
        // branch first. If it fails, split branch failures are flagged as
        // pre-existing detection; unknown baseline disables it.
        var baselineFailure = null;
        var baselineUnknown = false;
        if (verifyCommand && plan.sourceBranch) {
            var baselineResult = await verifySplit(plan.sourceBranch, {
                dir: dir,
                verifyCommand: verifyCommand,
                verifyTimeoutMs: verifyTimeoutMs,
                outputFn: options.outputFn || null
            });
            if (baselineResult.cleanupFailed) {
                baselineResult.baseline = true;
                return failedVerifyBatch(baselineResult, results);
            }
            // A baseline whose verdict is UNKNOWN — a truncated capture or an
            // exec-layer failure with no exit status — is not a baseline
            // failure: it disables pre-existing detection rather than
            // forgiving failing branches.
            if (baselineResult.truncated || baselineResult.execFailed) {
                baselineUnknown = true;
            } else if (!baselineResult.passed) {
                baselineFailure = baselineResult;
            }
        }

        for (var i = 0; i < plan.splits.length; i++) {
            if (prSplit.isCancelled() || prSplit.isForceCancelled()) {  // T117: honor force-cancel
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

            var baseCmd = verifyCommand;
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
            var result = await verifySplit(split.name, {
                dir: dir,
                verifyCommand: scopedCmd,
                verifyTimeoutMs: verifyTimeoutMs,
                outputFn: branchOutputFn
            });
            if (scopedCmd !== baseCmd) {
                result.scopedVerify = scopedCmd;
            }

            // A truncated capture or a broken exec layer is an unknown verdict,
            // never a deterministic failure: neither may be forgiven as
            // pre-existing, even when the baseline also failed.
            if (!result.passed && !result.truncated && !result.execFailed && baselineFailure) {
                result.preExisting = true;
                result.error = (result.error || 'verification failed') + ' (pre-existing on ' + plan.sourceBranch + ')';
            } else if (!result.passed && baselineUnknown) {
                result.error = (result.error || 'verification failed') + ' (baseline verdict unknown: no deterministic baseline failure to compare against)';
            }

            var branchElapsedMs = Date.now() - branchStartTime;
            if (onBranchDone) {
                onBranchDone(split.name, result.passed, result.exitCode || 0, branchElapsedMs, false, !!result.preExisting);
            }

            if (result.cleanupFailed) return failedVerifyBatch(result, results);
            results.push(result);
            if (!result.passed && !result.preExisting) {
                allPassed = false;
                failedBranches[split.name] = true;
            }
        }

        return { allPassed: allPassed, results: results, error: null };
    }

    // --- verifyEquivalence — tree-SHA comparison of last split vs source ---
    async function verifyEquivalence(plan) {
        if (!plan) {
            return { equivalent: false, splitTree: '', sourceTree: '', error: 'invalid plan' };
        }
        var dir = resolveDir(plan.dir || '.');

        if (!plan.splits || plan.splits.length === 0) {
            return { equivalent: false, splitTree: '', sourceTree: '', error: 'no splits in plan' };
        }

        var lastSplit = plan.splits[plan.splits.length - 1].name;

        var splitTreeResult = await gitExec(dir, ['rev-parse', lastSplit + '^{tree}']);
        if (splitTreeResult.code !== 0) {
            return {
                equivalent: false,
                splitTree: '',
                sourceTree: '',
                error: 'failed to get split tree: ' + execFailReason(splitTreeResult)
            };
        }
        var splitTree = splitTreeResult.stdout.trim();

        var sourceTreeResult = await gitExec(dir, ['rev-parse', plan.sourceBranch + '^{tree}']);
        if (sourceTreeResult.code !== 0) {
            return {
                equivalent: false,
                splitTree: splitTree,
                sourceTree: '',
                error: 'failed to get source tree: ' + execFailReason(sourceTreeResult)
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

    // --- verifyEquivalenceDetailed — adds per-file diff info on mismatch ---
    async function verifyEquivalenceDetailed(plan) {
        if (!plan) {
            return { equivalent: false, splitTree: '', sourceTree: '', error: 'invalid plan', diffFiles: [], diffSummary: '' };
        }
        var dir = resolveDir(plan.dir || '.');
        var base = await verifyEquivalence(plan);

        if (base.error || base.equivalent) {
            base.diffFiles = [];
            base.diffSummary = '';
            return base;
        }

        var lastSplit = plan.splits[plan.splits.length - 1].name;
        var diffResult = await gitExec(dir, ['diff', '--stat', lastSplit, plan.sourceBranch]);
        if (diffResult.code === 0) {
            base.diffSummary = diffResult.stdout.trim();
        } else {
            // Failed diff: report reason.
            base.diffError = execFailReason(diffResult);
        }

        var diffNamesResult = await gitExec(dir, ['diff', '--name-only', lastSplit, plan.sourceBranch]);
        if (diffNamesResult.code === 0 && diffNamesResult.stdout.trim() !== '') {
            base.diffFiles = diffNamesResult.stdout.trim().split('\n').filter(function(f) {
                return f !== '';
            });
        } else {
            base.diffFiles = [];
            if (diffNamesResult.code !== 0 && !base.diffError) {
                base.diffError = execFailReason(diffNamesResult);
            }
        }

        return base;
    }

    // --- cleanupBranches — deletes split branches ---
    async function cleanupBranches(plan) {
        if (!plan || !plan.splits) {
            return { deleted: [], errors: ['cleanupBranches: invalid plan — missing splits array'] };
        }
        var dir = resolveDir(plan.dir || '.');
        var deleted = [];
        var errors = [];

        // Try to checkout baseBranch; fall back to detaching HEAD so
        // branch -D can still succeed.
        var coRes = await gitExec(dir, ['checkout', plan.baseBranch]);
        if (coRes.code !== 0) {
            await gitExec(dir, ['checkout', '--detach', 'HEAD']);
        }

        for (var i = 0; i < plan.splits.length; i++) {
            var name = plan.splits[i].name;
            var result = await gitExec(dir, ['branch', '-D', name]);
            if (result.code === 0) {
                deleted.push(name);
            } else {
                errors.push(name + ': ' + execFailReason(result));
            }
        }

        return { deleted: deleted, errors: errors };
    }

    // --- prepareVerifyWorktree — create worktree ---
    async function prepareVerifyWorktree(branchName, config) {
        config = config || {};
        var dir = resolveDir(config.dir || '.');
        var command = await resolveVerifyCommand(config.verifyCommand || prSplit.runtime.verifyCommand, dir);

        if (!command) {
            return { skipped: true, worktreeDir: null };
        }

        // Eagerly load termmux to fail fast without orphaned worktree.
        require('osm:termmux');

        var worktreeDir = worktreeTmpPath('osm-verify-');

        var setupFailure = await setupVerificationWorktree(gitExec, dir, worktreeDir, branchName);
        if (setupFailure) {
            if (setupFailure.treeCleanupError) {
                setupFailure.dir = dir;
                return setupFailure;
            }
            return { error: setupFailure.error, worktreeDir: null };
        }

        return { worktreeDir: worktreeDir, dir: dir };
    }

    // --- startVerifySession — non-blocking variant ---
    async function startVerifySession(branchName, config) {
        config = config || {};
        var dir = resolveDir(config.dir || '.');
        var command = await resolveVerifyCommand(config.verifyCommand || prSplit.runtime.verifyCommand, dir);

        if (!command) {
            return { skipped: true, session: null, worktreeDir: null };
        }

        var termmux;
        try {
            termmux = require('osm:termmux');
        } catch (e) {
            return {
                error: 'termmux unavailable: ' + (e.message || e),
                session: null,
                worktreeDir: null
            };
        }
        var rows = config.rows || 24;
        var cols = config.cols || 120;

        // Delegate worktree creation.
        var wt = await prepareVerifyWorktree(branchName, config);
        if (wt.error) {
            return Object.assign({}, wt, { session: null });
        }
        if (wt.skipped) {
            return { skipped: true, session: null, worktreeDir: null };
        }
        var worktreeDir = wt.worktreeDir;

        var session = null;
        try {
            var captureShell = isWindows() ? 'cmd.exe' : 'sh';
            var captureArgs = isWindows() ? ['/C', command] : ['-c', command];
            session = termmux.newCaptureSession(captureShell, captureArgs, {
                dir: worktreeDir,
                rows: rows,
                cols: cols
            });
            var started = session.start();
            if (started && typeof started.then === 'function') {
                await started;
            }
        } catch (e) {
            var closeFailure = '';
            if (session && typeof session.close === 'function') {
                try {
                    var closeResult = session.close();
                    if (closeResult && typeof closeResult.then === 'function') {
                        closeResult = await closeResult;
                    }
                    if (closeResult === false) closeFailure = 'capture session close did not confirm termination';
                } catch (closeErr) {
                    closeFailure = 'capture session close failed: ' + ((closeErr && closeErr.message) || String(closeErr));
                    log.debug('startVerifySession: failed to close failed session', {
                        error: (closeErr && closeErr.message) || String(closeErr)
                    });
                }
            } else if (session) {
                closeFailure = 'capture session has no close method to confirm termination';
            }
            var failedStart = {
                error: 'start verify session failed: ' + ((e && e.message) ? e.message : String(e)),
                session: null,
                worktreeDir: null
            };
            if (closeFailure) {
                failedStart.passed = false;
                failedStart.execFailed = true;
                failedStart.cleanupFailed = true;
                failedStart.worktreeRetained = true;
                failedStart.worktreeDir = worktreeDir;
                failedStart.dir = dir;
                failedStart.error += '; cleanup: ' + closeFailure + '; worktree retained at ' + worktreeDir;
                return failedStart;
            }
            var cleanupResult = await cleanupVerifyWorktree(dir, worktreeDir);
            if (cleanupResult) {
                var startError = failedStart.error;
                Object.assign(failedStart, cleanupResult);
                failedStart.error = startError + '; ' + cleanupResult.error;
            }
            return failedStart;
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
    // startVerifySession. Success remains undefined; every failed removal
    // resolves with its diagnostic and path without claiming retention or
    // replacing the verification command's recorded exit status.
    async function cleanupVerifyWorktree(dir, worktreeDir) {
        return await removeVerificationWorktree(gitExec, dir, worktreeDir);
    }

    // --- DESIGN NOTE (T003): PTY Verify Pipeline Audit ---

    // --- Async versions for pipeline use (T31) ---
    //  Uses gitExecAsync (exec.spawn) so the event loop stays responsive.

    // verifySplitAsync is the non-blocking version of verifySplit.
    // Git worktree setup/teardown uses gitExecAsync. The actual verify
    // command uses exec.spawn (see T34 for full CaptureSession-based verify).
    async function verifySplitAsync(branchName, config) {
        // This function must not reject: an unhandled rejection is fatal to the
        // event loop, and the TUI fallback and automated pipelines both await
        // it without a catch. Every source that can reject — resolveVerifyCommand,
        // gitExecAsync worktree setup, shellSpawnAsync, a throwing output
        // callback — is contained here and reported as a structured failure
        // tagged execFailed, never as an unknown verdict.
        try {
            return await verifySplitAsyncInner(branchName, config);
        } catch (e) {
            return {
                name: branchName,
                passed: false,
                output: '',
                exitCode: -1,
                execFailed: true,
                error: 'verify exec-layer failure: ' + String((e && e.message) || e)
            };
        }
    }

    async function verifySplitAsyncInner(branchName, config) {
        var gitExecAsync = prSplit._gitExecAsync;
        config = config || {};
        var cancellationCheck = typeof config.isCancelled === 'function'
            ? config.isCancelled
            : function() {
                return typeof prSplit.isCancelled === 'function' && prSplit.isCancelled();
            };
        var isCancelledNow = function() {
            try {
                return !!cancellationCheck() ||
                    (typeof prSplit.isForceCancelled === 'function' && prSplit.isForceCancelled());
            } catch (e) {
                log.debug('verify cancellation check failed', { error: e.message || String(e) });
                return false;
            }
        };
        if (isCancelledNow()) {
            return { name: branchName, passed: false, output: '', error: 'verification cancelled by user', cancelled: true };
        }
        var dir = resolveDir(config.dir || '.');
        var command = await resolveVerifyCommand(config.verifyCommand || prSplit.runtime.verifyCommand, dir);
        var timeoutMs = prSplit._effectiveVerifyTimeoutMs(config.verifyTimeoutMs);
        var outputFn = config.outputFn || null;

        if (!command) {
            return { name: branchName, passed: true, output: '', error: null, skipped: true, duration: 0 };
        }
        if (isCancelledNow()) {
            return { name: branchName, passed: false, output: '', error: 'verification cancelled by user', cancelled: true };
        }

        var worktreeDir = worktreeTmpPath('osm-verify-');

        var setupFailure = await setupVerificationWorktree(gitExecAsync, dir, worktreeDir, branchName);
        if (setupFailure) return setupFailure;

        // Cleanup runs as an explicit phase that preserves the verification
        // result: the body (or catch on abnormal exit) assigns `outcome`, and
        // finally may amend it with cleanup status, so a removal failure or
        // retained worktree is always part of the returned result instead of
        // only a debug log.
        var outcome = null;
        var stdoutBuf = '';
        var cleanupNotes = [];
        var treeCleanupError = '';
        // boundedAwait waits for promise with at most msLeft milliseconds,
        // always clearing its timer. Returns {settled: bool, value/reason}.
        function boundedAwait(promise, msLeft) {
            var left = msLeft;
            if (!(left > 0)) left = 0;
            var timer = null;
            var timeoutP = new Promise(function(resolve) {
                timer = setTimeout(function() { resolve({ timedOut: true }); }, left);
            });
            return Promise.race([promise, timeoutP]).then(function(v) {
                if (timer !== null) clearTimeout(timer);
                if (v && v.timedOut) return { settled: false };
                return { settled: true, value: v };
            }, function(e) {
                if (timer !== null) clearTimeout(timer);
                return { settled: true, rejected: true, reason: e };
            });
        }

        var worktreeReady = true;
        try {
            var startMs = Date.now();
        var shellCmd;
        if (isWindows()) {
            shellCmd = 'cd /d ' + shellQuote(worktreeDir) + ' && ' + command;
        } else {
            shellCmd = 'cd ' + shellQuote(worktreeDir) + ' && ' + command;
        }

        if (timeoutMs > 0 && !isWindows()) {
            var timeoutSec = Math.ceil(timeoutMs / 1000);
            // Portable shell timeout: works on macOS (no GNU coreutils) and Linux.
            shellCmd = '( ' + shellCmd + ' ) & _pid=$!; ( sleep ' + timeoutSec + '; kill $_pid 2>/dev/null ) >/dev/null 2>&1 & _watch=$!; wait $_pid; _rc=$?; kill $_watch 2>/dev/null; wait $_watch 2>/dev/null; exit $_rc';
        }

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

        // Non-blocking: use platform-aware shell spawn so the event loop
        // stays responsive during verification. The native binding enforces
        // timeoutMs on every platform, including Windows where the shell
        // wrapper above is intentionally omitted.
        var child = await shellSpawnAsync(shellCmd, { timeoutMs: timeoutMs });
        var cancelled = false;
        var cancelTimer = null;
        var stopCancelWatch = function() {
            if (cancelTimer !== null) {
                clearTimeout(cancelTimer);
                cancelTimer = null;
            }
        };
        var cancelChild = function() {
            if (cancelled) return;
            cancelled = true;
            try {
                var killResult = child.kill();
                if (killResult && typeof killResult.catch === 'function') {
                    killResult.catch(function(e) {
                        log.debug('verify cancellation kill failed', { error: e.message || String(e) });
                    });
                }
            } catch (e) {
                log.debug('verify cancellation kill failed', { error: e.message || String(e) });
            }
        };
        var watchCancellation = function() {
            if (isCancelledNow()) {
                cancelChild();
                return;
            }
            cancelTimer = setTimeout(watchCancellation, 50);
        };
        if (isCancelledNow()) {
            cancelChild();
        } else {
            watchCancellation();
        }

        // Read stdout/stderr concurrently; truncated stream is a prefix.
        //
        // A read can reject when the engine context ends, and this function
        // must not reject: an unhandled rejection is fatal to the event loop,
        // and several callers await it without a catch. A stream that cannot be
        // drained is reported as truncated, which is exactly what it is — the
        // verdict is unknown — so the failure stays inside the result.
        var streamsTruncated = false;
        async function readStream(stream, onChunk) {
            var buf = '';
            while (true) {
                var chunk;
                try {
                    chunk = await stream.read();
                } catch (e) {
                    streamsTruncated = true;
                    log.debug('verify stream read failed; capture is a prefix', { error: e.message || String(e) });
                    break;
                }
                if (chunk.done) {
                    if (chunk.truncated === true) streamsTruncated = true;
                    break;
                }
                if (chunk.value !== undefined && chunk.value !== null) {
                    var text = String(chunk.value);
                    buf += text;
                    onChunk(text);
                }
            }
            return buf;
        }

        // child.wait() can reject (a cancelled wait reports its context error).
        // Guarding it here matters beyond this function's result: an unhandled
        // rejection is fatal to the event loop, so an unguarded wait would tear
        // down every other pending promise in the process.
        var waitResult = null;
        var waitError = null;
        var waitSettled = false;
        // waitResolvedWithStatus distinguishes a wait that FULFILLED with an
        // observed exit status from a rejected one: only a fulfilled wait
        // establishes that the child terminated.
        var waitResolvedWithStatus = false;
        await Promise.all([
            readStream(child.stdout, function(chunk) {
                stdoutBuf += chunk;
                emitChunk(chunk);
            }),
            readStream(child.stderr, function(chunk) {
                stderrBuf += chunk;
                emitChunk(chunk);
            }),
            child.wait().then(function(r) {
                waitResult = r || {};
                treeCleanupError = getTreeCleanupError(r) || treeCleanupError;
                waitSettled = true;
                waitResolvedWithStatus = true;
            }, function(err) {
                waitError = err;
                treeCleanupError = getTreeCleanupError(err) || treeCleanupError;
                waitSettled = true;
            })
        ]);

        var exitResult = waitResult;
        var exitCode = (exitResult && exitResult.code !== undefined) ? exitResult.code : 1;
        if (waitError) {
            exitCode = -1;
        }
        var elapsedMs = Date.now() - startMs;

        if (cancelled || isCancelledNow()) {
            outcome = {
                name: branchName,
                passed: false,
                output: stdoutBuf,
                exitCode: exitCode,
                error: 'verification cancelled by user',
                cancelled: true
            };
        } else if (waitError) {
            // A rejected wait means the exec layer never recorded an exit
            // status. Reporting it as `exit -1` with an empty explanation told
            // the operator nothing: a broken spawn/wait looked exactly like a
            // verify command that failed without writing to stderr. The
            // reason is carried through. This gate runs before the timeout
            // gate: a wait that failed for a non-timeout reason must not be
            // mislabelled as a timeout.
            outcome = {
                name: branchName,
                passed: false,
                output: stdoutBuf,
                exitCode: exitCode,
                execFailed: true,
                truncated: streamsTruncated || undefined,
                error: 'verify wait failed after ' + elapsedMs + 'ms with no exit status: ' + String((waitError && waitError.message) || waitError)
            };
        } else if (timeoutMs > 0 && (exitCode === 124 || exitCode === 137 || exitCode === 143 || elapsedMs >= timeoutMs)) {
            // Timeout attribution and capture completeness are independent.
            // Keep both flags when output is incomplete so the batch
            // classifier cannot forgive an unknown verdict as pre-existing.
            outcome = {
                name: branchName,
                passed: false,
                output: stdoutBuf,
                timedOut: true,
                truncated: streamsTruncated || undefined,
                exitCode: exitCode,
                error: 'verify timeout after ' + elapsedMs + 'ms (limit: ' + timeoutMs + 'ms)'
            };
        } else if (streamsTruncated) {
            // A cut capture outranks an unknown status as the diagnosis. The
            // timeout branch above may also have fired on a run whose capture
            // was cut short: keep the truncation flag so the result stays
            // classified as an unknown capture rather than a deterministic
            // timeout.
            outcome = {
                name: branchName,
                passed: false,
                output: stdoutBuf,
                truncated: true,
                exitCode: exitCode,
                error: 'verification output capture was cut short before EOF: the verdict is unknown'
            };
        } else {
            var signal = exitResult && exitResult.signal ? String(exitResult.signal) : '';
            var trimmedStderr = stderrBuf.trim();
            var failureDetail = trimmedStderr !== '' ? trimmedStderr : '';
            if (signal && failureDetail.indexOf(signal) < 0) {
                failureDetail = failureDetail ? (failureDetail + ' (' + signal + ')') : signal;
            }
            outcome = {
                name: branchName,
                passed: exitCode === 0,
                output: stdoutBuf,
                exitCode: exitCode,
                error: exitCode !== 0 ? 'verify failed (exit ' + exitCode + '): ' + failureDetail : null
            };
        }
        } catch (e) {
            // Capture abnormal-exit errors into `outcome` before `finally` runs
            // so cleanup diagnostics (worktreeRetained, cleanupFailed,
            // worktreeDir, cleanupNotes) are always attached rather than
            // discarded when outcome is null.
            treeCleanupError = getTreeCleanupError(e) || treeCleanupError;
            outcome = {
                name: branchName,
                passed: false,
                output: stdoutBuf,
                exitCode: -1,
                execFailed: true,
                error: 'verify exec-layer failure: ' + String((e && e.message) || e)
            };
        } finally {
            if (typeof stopCancelWatch === 'function') stopCancelWatch();
            // Compute the cleanup budget here in finally — never before try —
            // and decouple it from verifyTimeoutMs so a long verify run or tiny
            // verifyTimeoutMs never leaves 0ms for killing and reaping the
            // child. Each stage also gets a floor so reaper I/O cannot lose to
            // a 0ms timer if the prior stage ran long.
            var CLEANUP_BUDGET_MS = 5000;
            var CLEANUP_STAGE_FLOOR_MS = 500;
            var cleanupDeadlineMs = Date.now() + CLEANUP_BUDGET_MS;
            var stageMsLeft = function() {
                var left = cleanupDeadlineMs - Date.now();
                return left > CLEANUP_STAGE_FLOOR_MS ? left : CLEANUP_STAGE_FLOOR_MS;
            };
            // If the body left abnormally before the wait settled (a throwing
            // output callback, a stream error escaping readStream), the verify
            // child may still be running: kill it and confirm its exit (both
            // bounded by the cleanup deadline, with every timer cleared)
            // BEFORE removing the worktree, which fails on Windows while the
            // child holds it as CWD. A fulfilled wait establishes direct-child
            // termination; a known tree-close failure separately prohibits
            // removal. Never retry a signal against an already-reaped group.
            if (!treeCleanupError && child && (!waitSettled || !waitResolvedWithStatus)) {
                var killOutcome = null;
                try {
                    var killResult = child.kill();
                    if (killResult && typeof killResult.then === 'function') {
                        killOutcome = await boundedAwait(killResult, stageMsLeft());
                    } else {
                        killOutcome = { settled: true, value: killResult };
                    }
                } catch (e) {
                    killOutcome = { settled: true, rejected: true, reason: e };
                }
                if (killOutcome && killOutcome.settled && killOutcome.rejected) {
                    treeCleanupError = getTreeCleanupError(killOutcome.reason) || treeCleanupError;
                    cleanupNotes.push('kill failed: ' + String((killOutcome.reason && killOutcome.reason.message) || killOutcome.reason));
                    log.debug('verifySplitAsync: abnormal-exit kill failed', { error: (killOutcome.reason && killOutcome.reason.message) || String(killOutcome.reason) });
                } else if (killOutcome && !killOutcome.settled) {
                    cleanupNotes.push('kill did not settle before the cleanup deadline');
                }
                // Confirm the child actually terminated: the kill promise only
                // confirms signal delivery, so also wait for the child's exit,
                // bounded by the remaining deadline (with stage floor).
                var waitOutcome = null;
                try {
                    waitOutcome = await boundedAwait(child.wait(), stageMsLeft());
                } catch (e) {
                    waitOutcome = { settled: true, rejected: true, reason: e };
                }
                if (waitOutcome && waitOutcome.settled && waitOutcome.value && waitOutcome.value.code !== undefined) {
                    waitResult = waitOutcome.value;
                    treeCleanupError = getTreeCleanupError(waitResult) || treeCleanupError;
                    waitResolvedWithStatus = true;
                } else if (waitOutcome && waitOutcome.settled && waitOutcome.rejected) {
                    treeCleanupError = getTreeCleanupError(waitOutcome.reason) || treeCleanupError;
                    cleanupNotes.push('exit wait rejected: ' + String((waitOutcome.reason && waitOutcome.reason.message) || waitOutcome.reason));
                } else if (waitOutcome && !waitOutcome.settled) {
                    cleanupNotes.push('exit wait did not settle before the cleanup deadline');
                }
            }
            if (!child && !treeCleanupError) {
                // A rejected spawn can follow process creation. The binding
                // carries a tree cleanup error when that rollback failed.
                waitResolvedWithStatus = true;
            }
            var removalFailure;
            if (worktreeReady && !treeCleanupError) {
                if (waitResolvedWithStatus) {
                    removalFailure = await removeVerificationWorktree(gitExecAsync, dir, worktreeDir);
                } else {
                    // Termination was never confirmed (rejected wait, failed
                    // kill, or deadline expiry): removing the worktree now
                    // could fail on Windows while the child still holds it as
                    // CWD, and claiming success would be a lie. Retain the
                    // worktree and report its path.
                    cleanupNotes.push('worktree retained at ' + worktreeDir + ': child termination was not confirmed');
                }
            }
            if (outcome !== null && cleanupNotes.length > 0) {
                outcome.passed = false;
                outcome.execFailed = true;
                outcome.cleanupFailed = true;
                outcome.worktreeDir = worktreeDir;
                if (!waitResolvedWithStatus && worktreeReady) {
                    outcome.worktreeRetained = true;
                }
                var note = cleanupNotes.join('; ');
                outcome.error = (outcome.error ? outcome.error + '; ' : '') + 'cleanup: ' + note;
            }
            if (outcome !== null && (treeCleanupError || removalFailure || outcome.cleanupFailed)) {
                // A cleanup failure does not erase the verification command's
                // status. Preserve it and all independently known
                // capture, timeout and cancellation facts, including when an
                // output callback threw before the first wait completed.
                if (waitResult && waitResult.code !== undefined) {
                    outcome.exitCode = waitResult.code;
                    if (timeoutMs > 0 && (waitResult.code === 124 || waitResult.code === 137 ||
                        waitResult.code === 143 || (typeof elapsedMs === 'number' && elapsedMs >= timeoutMs))) {
                        outcome.timedOut = true;
                    }
                }
                if (streamsTruncated) outcome.truncated = true;
                if (cancelled || isCancelledNow()) outcome.cancelled = true;
            }
            if (outcome !== null && treeCleanupError) {
                reportTreeCleanupFailure(outcome, treeCleanupError, worktreeDir, true);
            } else if (outcome !== null) {
                applyRemovalFailure(outcome, removalFailure);
            }
        }
        return outcome;
    }

    // verifySplitsAsync is the non-blocking version of verifySplits.
    async function verifySplitsAsync(plan, options) {
        options = options || {};
        if (!plan || !plan.splits) {
            return { allPassed: false, results: [], error: 'verifySplits: invalid plan — missing splits array' };
        }
        var cancellationCheck = typeof options.isCancelled === 'function'
            ? options.isCancelled
            : function() {
                return typeof prSplit.isCancelled === 'function' && prSplit.isCancelled();
            };
        var isCancelledNow = function() {
            try {
                return !!cancellationCheck() ||
                    (typeof prSplit.isForceCancelled === 'function' && prSplit.isForceCancelled());
            } catch (e) {
                log.debug('verify cancellation check failed', { error: e.message || String(e) });
                return false;
            }
        };
        if (isCancelledNow()) {
            return { allPassed: false, results: [], error: 'verification cancelled by user' };
        }
        var dir = resolveDir(plan.dir || '.');
        var verifyCommand = await resolveVerifyCommand(plan.verifyCommand || prSplit.runtime.verifyCommand, dir);
        if (verifyCommand) plan.verifyCommand = verifyCommand;
        var verifyTimeoutMs = prSplit._effectiveVerifyTimeoutMs(options.verifyTimeoutMs);
        var onBranchStart = typeof options.onBranchStart === 'function' ? options.onBranchStart : null;
        var onBranchDone = typeof options.onBranchDone === 'function' ? options.onBranchDone : null;
        var onBranchOutput = typeof options.onBranchOutput === 'function' ? options.onBranchOutput : null;
        var results = [];
        var allPassed = true;
        var failedBranches = {};

        // Pre-existing failure detection: run verification on the source
        // branch first. A baseline whose verdict is UNKNOWN — a truncated
        // capture OR an exec-layer failure with no exit status — is not a
        // baseline failure: it disables pre-existing detection rather than
        // forgiving failing branches.
        var baselineFailure = null;
        var baselineUnknown = false;
        if (verifyCommand && plan.sourceBranch) {
            var baselineResult = await verifySplitAsync(plan.sourceBranch, {
                dir: dir,
                verifyCommand: verifyCommand,
                verifyTimeoutMs: verifyTimeoutMs,
                outputFn: options.outputFn || null,
                isCancelled: isCancelledNow
            });
            if (baselineResult.cleanupFailed) {
                baselineResult.baseline = true;
                return failedVerifyBatch(baselineResult, results);
            }
            if (baselineResult.cancelled || isCancelledNow()) {
                return { allPassed: false, results: [], error: 'verification cancelled by user' };
            }
            if (baselineResult.truncated || baselineResult.execFailed) {
                baselineUnknown = true;
            } else if (!baselineResult.passed) {
                baselineFailure = baselineResult;
            }
        }

        for (var i = 0; i < plan.splits.length; i++) {
            if (isCancelledNow()) {
                return { allPassed: false, results: results, error: 'verification cancelled by user' };
            }

            var split = plan.splits[i];

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

            var baseCmd = verifyCommand;
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
            var result = await verifySplitAsync(split.name, {
                dir: dir,
                verifyCommand: scopedCmd,
                verifyTimeoutMs: verifyTimeoutMs,
                outputFn: branchOutputFn,
                isCancelled: isCancelledNow
            });
            if (result.cancelled || isCancelledNow()) {
                if (result.cleanupFailed) return failedVerifyBatch(result, results);
                return { allPassed: false, results: results, error: 'verification cancelled by user' };
            }
            if (scopedCmd !== baseCmd) {
                result.scopedVerify = scopedCmd;
            }

            // A truncated branch is never pre-existing. Nor is a wait/exec
            // failure with no exit status: that is an unknown verdict, and
            // forgiving it would let a broken exec layer report allPassed:true.
            if (!result.passed && !result.truncated && !result.execFailed && baselineFailure) {
                result.preExisting = true;
                result.error = (result.error || 'verification failed') + ' (pre-existing on ' + plan.sourceBranch + ')';
            } else if (!result.passed && baselineUnknown) {
                result.error = (result.error || 'verification failed') + ' (baseline verdict unknown: no deterministic baseline failure to compare against)';
            }

            var branchElapsedMs = Date.now() - branchStartTime;
            if (onBranchDone) {
                onBranchDone(split.name, result.passed, result.exitCode || 0, branchElapsedMs, false, !!result.preExisting);
            }

            if (result.cleanupFailed) return failedVerifyBatch(result, results);
            results.push(result);
            if (!result.passed && !result.preExisting) {
                allPassed = false;
                failedBranches[split.name] = true;
            }
        }

        return { allPassed: allPassed, results: results, error: null };
    }

    // verifyEquivalenceAsync is the non-blocking version of verifyEquivalence.
    async function verifyEquivalenceAsync(plan) {
        var gitExecAsync = prSplit._gitExecAsync;
        if (!plan) {
            return { equivalent: false, splitTree: '', sourceTree: '', error: 'invalid plan' };
        }
        var dir = resolveDir(plan.dir || '.');

        if (!plan.splits || plan.splits.length === 0) {
            return { equivalent: false, splitTree: '', sourceTree: '', error: 'no splits in plan' };
        }

        var lastSplit = plan.splits[plan.splits.length - 1].name;

        var splitTreeResult = await gitExecAsync(dir, ['rev-parse', lastSplit + '^{tree}']);
        if (splitTreeResult.code !== 0) {
            return {
                equivalent: false,
                splitTree: '',
                sourceTree: '',
                error: 'failed to get split tree: ' + execFailReason(splitTreeResult)
            };
        }
        var splitTree = splitTreeResult.stdout.trim();

        var sourceTreeResult = await gitExecAsync(dir, ['rev-parse', plan.sourceBranch + '^{tree}']);
        if (sourceTreeResult.code !== 0) {
            return {
                equivalent: false,
                splitTree: splitTree,
                sourceTree: '',
                error: 'failed to get source tree: ' + execFailReason(sourceTreeResult)
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

    // T075: verifyEquivalenceDetailedAsync — async variant that adds
    // per-file diff info when the trees don't match (mirrors the sync
    // verifyEquivalenceDetailed).
    async function verifyEquivalenceDetailedAsync(plan) {
        var gitExecAsync = prSplit._gitExecAsync;  // Re-capture per call (consistency)
        var base = await verifyEquivalenceAsync(plan);
        base.diffFiles = [];
        base.diffSummary = '';
        if (base.error || base.equivalent) {
            return base;
        }

        var dir = resolveDir((plan && plan.dir) || '.');
        var lastSplit = plan.splits[plan.splits.length - 1].name;

        var diffStatResult = await gitExecAsync(dir, ['diff', '--stat', lastSplit, plan.sourceBranch]);
        if (diffStatResult.code === 0) {
            base.diffSummary = diffStatResult.stdout.trim();
        } else {
            base.diffError = execFailReason(diffStatResult);
        }

        var diffNamesResult = await gitExecAsync(dir, ['diff', '--name-only', lastSplit, plan.sourceBranch]);
        if (diffNamesResult.code === 0 && diffNamesResult.stdout.trim() !== '') {
            base.diffFiles = diffNamesResult.stdout.trim().split('\n').filter(function(f) {
                return f !== '';
            });
        } else if (diffNamesResult.code !== 0 && !base.diffError) {
            base.diffError = execFailReason(diffNamesResult);
        }

        return base;
    }

    // cleanupBranchesAsync is the non-blocking version of cleanupBranches.
    async function cleanupBranchesAsync(plan) {
        var gitExecAsync = prSplit._gitExecAsync;
        if (!plan || !plan.splits) {
            return { deleted: [], errors: ['cleanupBranches: invalid plan — missing splits array'] };
        }
        var dir = resolveDir(plan.dir || '.');
        var deleted = [];
        var errors = [];

        var coRes = await gitExecAsync(dir, ['checkout', plan.baseBranch]);
        if (coRes.code !== 0) {
            await gitExecAsync(dir, ['checkout', '--detach', 'HEAD']);
        }

        for (var i = 0; i < plan.splits.length; i++) {
            var name = plan.splits[i].name;
            var result = await gitExecAsync(dir, ['branch', '-D', name]);
            if (result.code === 0) {
                deleted.push(name);
            } else {
                errors.push(name + ': ' + execFailReason(result));
            }
        }

        return { deleted: deleted, errors: errors };
    }

    // --- Exports ---
    prSplit.verifySplit = verifySplit;
    prSplit.verifySplits = verifySplits;
    prSplit.verifyEquivalence = verifyEquivalence;
    prSplit.verifyEquivalenceDetailed = verifyEquivalenceDetailed;
    prSplit.cleanupBranches = cleanupBranches;
    prSplit.prepareVerifyWorktree = prepareVerifyWorktree;
    prSplit.startVerifySession = startVerifySession;
    prSplit.cleanupVerifyWorktree = cleanupVerifyWorktree;
    prSplit.verifySplitAsync = verifySplitAsync;
    prSplit.verifySplitsAsync = verifySplitsAsync;
    prSplit.verifyEquivalenceAsync = verifyEquivalenceAsync;
    prSplit.verifyEquivalenceDetailedAsync = verifyEquivalenceDetailedAsync;  // T075
    prSplit.cleanupBranchesAsync = cleanupBranchesAsync;
})(globalThis.prSplit);
