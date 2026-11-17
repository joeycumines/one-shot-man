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

        // Try proper branch checkout first (preserves branch name in HEAD).
        var wtAdd = await gitExec(dir, ['worktree', 'add', worktreeDir, branchName]);
        if (wtAdd.code !== 0) {
            // Branch might be checked out elsewhere (user's CWD); fallback to detached HEAD.
            await gitExec(dir, ['worktree', 'remove', '--force', worktreeDir]);
            wtAdd = await gitExec(dir, ['worktree', 'add', '--detach', worktreeDir, branchName]);
            if (wtAdd.code !== 0) {
                // Worktree setup is an environment/exec failure, not a verify
                // verdict: tag it execFailed so it is never forgiven as
                // pre-existing.
                return {
                    name: branchName,
                    passed: false,
                    output: '',
                    exitCode: wtAdd.code,
                    execFailed: true,
                    error: 'create worktree failed: ' + execFailReason(wtAdd)
                };
            }
        }

        async function cleanupWorktree() {
            await gitExec(dir, ['worktree', 'remove', '--force', worktreeDir]);
        }

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

        var result;
        try {
            result = await shellSpawnSync(shellCmd, {
                timeoutMs: timeoutMs,
                onStdout: function(chunk) {
                    stdoutBuf += chunk;
                    emitChunk(chunk);
                },
                onStderr: function(chunk) {
                    stderrBuf += chunk;
                    emitChunk(chunk);
                }
            });
        } finally {
            // The worktree must be removed even if the spawn throws.
            await cleanupWorktree();
        }
        var elapsedMs = Date.now() - startMs;

        // An aborted capture is an exec-layer failure whose stderr may be
        // empty: the verdict is unknown, so it can neither pass nor count as
        // a deterministic failure (and must never be forgiven as pre-existing).
        var captureCutShort = result.error === true && /cut short|aborted before EOF|incomplete/.test(String(result.message || ''));
        // A non-exit exec error (spawn/wait failure, code -1 with error) is
        // likewise an unknown verdict, tagged execFailed like the async path.
        var execLayerFailed = !captureCutShort && result.error === true && result.code === -1;

        // Timeout before truncation gate. Same classification as the async
        // path: a run whose capture was cut short stays truncated even when
        // the timeout condition also holds.
        if (timeoutMs > 0 && (result.code === 124 || result.code === 137 || result.code === 143 || elapsedMs >= timeoutMs)) {
            return {
                name: branchName,
                passed: false,
                output: stdoutBuf,
                truncated: captureCutShort || undefined,
                exitCode: result.code,
                error: 'verify timeout after ' + elapsedMs + 'ms (limit: ' + timeoutMs + 'ms)'
            };
        }

        if (captureCutShort) {
            return {
                name: branchName,
                passed: false,
                output: stdoutBuf,
                truncated: true,
                exitCode: result.code,
                error: 'verify failed: ' + (result.message || 'output capture was cut short before EOF') +
                    (stderrBuf.trim() !== '' ? ': ' + stderrBuf.trim() : '')
            };
        }

        if (execLayerFailed) {
            return {
                name: branchName,
                passed: false,
                output: stdoutBuf,
                exitCode: result.code,
                execFailed: true,
                error: 'verify exec-layer failure: ' + execFailReason(result)
            };
        }

        return {
            name: branchName,
            passed: result.code === 0,
            output: stdoutBuf,
            exitCode: result.code,
            error: result.code !== 0
                ? 'verify failed (exit ' + result.code + '): ' + execFailReason(result)
                : null
        };
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

        var wtAdd = await gitExec(dir, ['worktree', 'add', worktreeDir, branchName]);
        if (wtAdd.code !== 0) {
            // Branch might be checked out elsewhere; fallback to detached HEAD.
            await gitExec(dir, ['worktree', 'remove', '--force', worktreeDir]);
            wtAdd = await gitExec(dir, ['worktree', 'add', '--detach', worktreeDir, branchName]);
            if (wtAdd.code !== 0) {
                return {
                    error: 'create worktree failed: ' + execFailReason(wtAdd),
                    worktreeDir: null
                };
            }
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
            return { error: wt.error, session: null, worktreeDir: null };
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
            if (session && typeof session.close === 'function') {
                try {
                    var closeResult = session.close();
                    if (closeResult && typeof closeResult.then === 'function') {
                        await closeResult;
                    }
                } catch (closeErr) {
                    log.debug('startVerifySession: failed to close failed session', {
                        error: closeErr.message || String(closeErr)
                    });
                }
            }
            try {
                await gitExec(dir, ['worktree', 'remove', '--force', worktreeDir]);
            } catch (cleanupErr) {
                log.debug('startVerifySession: failed to clean worktree after startup error', {
                    error: cleanupErr.message || String(cleanupErr)
                });
            }
            return {
                error: 'start verify session failed: ' + ((e && e.message) ? e.message : String(e)),
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
    async function cleanupVerifyWorktree(dir, worktreeDir) {
        await gitExec(dir, ['worktree', 'remove', '--force', worktreeDir]);
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

        var wtAdd = await gitExecAsync(dir, ['worktree', 'add', worktreeDir, branchName]);
        if (wtAdd.code !== 0) {
            await gitExecAsync(dir, ['worktree', 'remove', '--force', worktreeDir]);
            wtAdd = await gitExecAsync(dir, ['worktree', 'add', '--detach', worktreeDir, branchName]);
            if (wtAdd.code !== 0) {
                // Worktree setup is an environment/exec failure, not a verify
                // verdict: tag it execFailed so it is never forgiven as
                // pre-existing.
                return {
                    name: branchName,
                    passed: false,
                    output: '',
                    exitCode: wtAdd.code,
                    execFailed: true,
                    error: 'create worktree failed: ' + execFailReason(wtAdd)
                };
            }
        }

        async function cleanupWorktreeAsync() {
            await gitExecAsync(dir, ['worktree', 'remove', '--force', worktreeDir]);
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
        await Promise.all([
            readStream(child.stdout, function(chunk) {
                stdoutBuf += chunk;
                emitChunk(chunk);
            }),
            readStream(child.stderr, function(chunk) {
                stderrBuf += chunk;
                emitChunk(chunk);
            }),
            child.wait().then(function(r) { waitResult = r || {}; waitSettled = true; }, function(err) { waitError = err; waitSettled = true; })
        ]);

        var exitResult = waitResult;
        var exitCode = (exitResult && exitResult.code !== undefined) ? exitResult.code : 1;
        if (waitError) {
            exitCode = -1;
        }
        var elapsedMs = Date.now() - startMs;

        if (cancelled || isCancelledNow()) {
            return {
                name: branchName,
                passed: false,
                output: stdoutBuf,
                exitCode: exitCode,
                error: 'verification cancelled by user',
                cancelled: true
            };
        }

        // A rejected wait means the exec layer never recorded an exit status.
        // Reporting it as `exit -1` with an empty explanation told the operator
        // nothing: a broken spawn/wait looked exactly like a verify command that
        // failed without writing to stderr. The reason is carried through.
        // This gate runs before the timeout gate: a wait that failed for a
        // non-timeout reason must not be mislabelled as a timeout.
        if (waitError) {
            return {
                name: branchName,
                passed: false,
                output: stdoutBuf,
                exitCode: exitCode,
                execFailed: true,
                error: 'verify wait failed after ' + elapsedMs + 'ms with no exit status: ' + String((waitError && waitError.message) || waitError)
            };
        }

        // Timeout before truncation gate. If the capture is also known to be
        // incomplete, keep truncated:true on the result: the verdict is
        // unknown, and the batch classifier must not read this as a
        // deterministic timeout that could be forgiven as pre-existing.
        if (timeoutMs > 0 && (exitCode === 124 || exitCode === 137 || exitCode === 143 || elapsedMs >= timeoutMs)) {
            return {
                name: branchName,
                passed: false,
                output: stdoutBuf,
                truncated: streamsTruncated || undefined,
                exitCode: exitCode,
                error: 'verify timeout after ' + elapsedMs + 'ms (limit: ' + timeoutMs + 'ms)'
            };
        }

        // A cut capture outranks an unknown status as the diagnosis. The
        // timeout branch above may also have fired on a run whose capture was
        // cut short: keep the truncation flag so the result stays classified
        // as an unknown capture rather than a deterministic timeout.
        if (streamsTruncated) {
            return {
                name: branchName,
                passed: false,
                output: stdoutBuf,
                truncated: true,
                exitCode: exitCode,
                error: 'verification output capture was cut short before EOF: the verdict is unknown'
            };
        }

        var signal = exitResult && exitResult.signal ? String(exitResult.signal) : '';
        var trimmedStderr = stderrBuf.trim();
        var failureDetail = trimmedStderr !== '' ? trimmedStderr : '';
        if (signal && failureDetail.indexOf(signal) < 0) {
            failureDetail = failureDetail ? (failureDetail + ' (' + signal + ')') : signal;
        }
        return {
            name: branchName,
            passed: exitCode === 0,
            output: stdoutBuf,
            exitCode: exitCode,
            error: exitCode !== 0 ? 'verify failed (exit ' + exitCode + '): ' + failureDetail : null
        };
        } finally {
            if (typeof stopCancelWatch === 'function') stopCancelWatch();
            // If the body left abnormally before the wait settled (a throwing
            // output callback, a stream error escaping readStream), the verify
            // child is still running: kill it and wait for termination (bounded
            // by the verify timeout, falling back to a short fixed bound)
            // BEFORE removing the worktree, which fails on Windows while the
            // child holds it as CWD. A kill failure stays diagnosable: it is
            // logged and surfaced on the returned result's error.
            if (!waitSettled) {
                var killBoundMs = timeoutMs > 0 ? timeoutMs : 5000;
                try {
                    var killResult = child.kill();
                    if (killResult && typeof killResult.then === 'function') {
                        await Promise.race([
                            killResult,
                            new Promise(function(resolve) { setTimeout(resolve, killBoundMs); })
                        ]);
                    }
                    // Confirm the child actually terminated: the kill promise
                    // only confirms signal delivery, so also wait for the
                    // child's exit, bounded by the same deadline.
                    await Promise.race([
                        child.wait(),
                        new Promise(function(resolve) { setTimeout(resolve, killBoundMs); })
                    ]);
                } catch (e) {
                    log.debug('verifySplitAsync: abnormal-exit kill failed', { error: e.message || String(e) });
                }
            }
            if (worktreeReady) {
                try {
                    var cleanupResult = await cleanupWorktreeAsync();
                    // A failed worktree removal must not go unnoticed: the
                    // verify result is already a structured failure on this
                    // path, so append the cleanup reason to its diagnostic.
                    if (cleanupResult && cleanupResult.code !== 0) {
                        log.debug('verifySplitAsync: worktree cleanup failed', { error: execFailReason(cleanupResult) });
                    }
                } catch (e) {
                    log.debug('verifySplitAsync: worktree cleanup failed', { error: e.message || String(e) });
                }
            }
        }
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
