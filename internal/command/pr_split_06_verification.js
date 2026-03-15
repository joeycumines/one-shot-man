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
        // T103: Use system temp dir to avoid fragile dir + '/../' pattern.
        var worktreeDir = worktreeTmpPath('osm-verify-');

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
    //  verifyEquivalence — tree-SHA comparison of last split vs source
    //
    //  DESIGN NOTE (T094): This check relies on the CUMULATIVE CHAIN model.
    //  executeSplit/executeSplitAsync in pr_split_05_execution.js builds
    //  branches as a stacked chain: each split inherits ALL content from
    //  the previous split (via `currentBase = split.name`). The last split
    //  is therefore the accumulation of baseBranch + ALL changed files
    //  (each checked out from sourceBranch). Comparing its tree SHA against
    //  sourceBranch^{tree} is a bit-perfect integrity check.
    //
    //  If the execution model ever changes to INDEPENDENT branches (each
    //  starting from baseBranch), this check would need redesign — likely
    //  a combined merge or tree-walk approach.
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
        // T103: Use system temp dir to avoid fragile dir + '/../' pattern.
        var worktreeDir = worktreeTmpPath('osm-verify-');

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
    //  DESIGN NOTE (T003): PTY Verify Pipeline Audit
    //
    //  The async verification pipeline (startVerifySession → pollVerifySession
    //  → runVerifyBranch in chunk 16) was audited for correctness:
    //
    //  1. CaptureSession lifecycle: readerLoop stores exitCode BEFORE closing
    //     the done channel, so isDone() → exitCode() is always safe. The TUI
    //     polls isDone() at 100ms intervals — no busy-wait or race.
    //
    //  2. Worktree cleanup: cleanupVerifyWorktree is best-effort (fire-and-forget
    //     `git worktree remove --force`). Failures are silent by design — a
    //     dangling worktree is harmless and will be GC'd on next prune.
    //
    //  3. Dual-path architecture: runVerifyBranch tries CaptureSession first
    //     (PTY + VTerm for live output) and falls back to verifySplitAsync
    //     (exec.spawn) if CaptureSession creation fails (e.g., PTY unavailable
    //     on the platform). Note: require('osm:termmux') must succeed;
    //     a missing module is a fatal error.
    //
    //  4. preExisting detection: pollVerifySession hard-codes preExisting=false
    //     because CaptureSession results don't carry baseline info. T115 will
    //     add proper pre-existing detection to the PTY path.
    //
    //  5. Async variants (verifySplitAsync, verifySplitsAsync,
    //     verifyEquivalenceAsync) now have unit test coverage via mocked
    //     _gitExecAsync and exec.spawn infrastructure.
    // -----------------------------------------------------------------------

    // -----------------------------------------------------------------------
    //  Async versions for pipeline use (T31)
    //  Uses gitExecAsync (exec.spawn) so the event loop stays responsive.
    // -----------------------------------------------------------------------

    // verifySplitAsync is the non-blocking version of verifySplit.
    // Git worktree setup/teardown uses gitExecAsync. The actual verify
    // command uses exec.spawn (see T34 for full CaptureSession-based verify).
    async function verifySplitAsync(branchName, config) {
        var gitExecAsync = prSplit._gitExecAsync;
        config = config || {};
        var dir = resolveDir(config.dir || '.');
        var command = config.verifyCommand || prSplit.runtime.verifyCommand;
        var timeoutMs = config.verifyTimeoutMs || 0;
        var outputFn = config.outputFn || null;

        if (!command) {
            return { name: branchName, passed: true, output: '', error: null, skipped: true, duration: 0 };
        }

        var worktreeDir = worktreeTmpPath('osm-verify-');

        var wtAdd = await gitExecAsync(dir, ['worktree', 'add', worktreeDir, branchName]);
        if (wtAdd.code !== 0) {
            await gitExecAsync(dir, ['worktree', 'remove', '--force', worktreeDir]);
            wtAdd = await gitExecAsync(dir, ['worktree', 'add', '--detach', worktreeDir, branchName]);
            if (wtAdd.code !== 0) {
                return {
                    name: branchName,
                    passed: false,
                    output: '',
                    error: 'create worktree failed: ' + wtAdd.stderr.trim()
                };
            }
        }

        async function cleanupWorktreeAsync() {
            await gitExecAsync(dir, ['worktree', 'remove', '--force', worktreeDir]);
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

        // Non-blocking: use exec.spawn() so the event loop stays responsive
        // during verification. Replaces the former blocking exec.execStream().
        var child = exec.spawn('sh', ['-c', shellCmd]);

        // Read stdout and stderr concurrently via async streams.
        async function readStream(stream, onChunk) {
            var buf = '';
            while (true) {
                var chunk = await stream.read();
                if (chunk.done) break;
                if (chunk.value !== undefined && chunk.value !== null) {
                    var text = String(chunk.value);
                    buf += text;
                    onChunk(text);
                }
            }
            return buf;
        }

        var streamResults = await Promise.all([
            readStream(child.stdout, function(chunk) {
                stdoutBuf += chunk;
                emitChunk(chunk);
            }),
            readStream(child.stderr, function(chunk) {
                stderrBuf += chunk;
                emitChunk(chunk);
            }),
            child.wait()
        ]);

        var exitResult = streamResults[2];
        var exitCode = (exitResult && exitResult.code !== undefined) ? exitResult.code : 1;
        var elapsedMs = Date.now() - startMs;

        await cleanupWorktreeAsync();

        if (timeoutMs > 0 && (exitCode === 124 || elapsedMs >= timeoutMs)) {
            return {
                name: branchName,
                passed: false,
                output: stdoutBuf,
                error: 'verify timeout after ' + elapsedMs + 'ms (limit: ' + timeoutMs + 'ms)'
            };
        }

        return {
            name: branchName,
            passed: exitCode === 0,
            output: stdoutBuf,
            error: exitCode !== 0 ? 'verify failed (exit ' + exitCode + '): ' + stderrBuf : null
        };
    }

    // verifySplitsAsync is the non-blocking version of verifySplits.
    async function verifySplitsAsync(plan, options) {
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
        // branch first.
        var baselineFailure = null;
        if (plan.verifyCommand && plan.sourceBranch) {
            var baselineResult = await verifySplitAsync(plan.sourceBranch, {
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
            if (prSplit.isCancelled() || prSplit.isForceCancelled()) {  // T117: honor force-cancel
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
            var result = await verifySplitAsync(split.name, {
                dir: dir,
                verifyCommand: scopedCmd,
                verifyTimeoutMs: verifyTimeoutMs,
                outputFn: branchOutputFn
            });
            if (scopedCmd !== baseCmd) {
                result.scopedVerify = scopedCmd;
            }

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
                error: 'failed to get split tree: ' + splitTreeResult.stderr.trim()
            };
        }
        var splitTree = splitTreeResult.stdout.trim();

        var sourceTreeResult = await gitExecAsync(dir, ['rev-parse', plan.sourceBranch + '^{tree}']);
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
        }

        var diffNamesResult = await gitExecAsync(dir, ['diff', '--name-only', lastSplit, plan.sourceBranch]);
        if (diffNamesResult.code === 0 && diffNamesResult.stdout.trim() !== '') {
            base.diffFiles = diffNamesResult.stdout.trim().split('\n').filter(function(f) {
                return f !== '';
            });
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
                errors.push(name + ': ' + result.stderr.trim());
            }
        }

        return { deleted: deleted, errors: errors };
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
    prSplit.verifySplitAsync = verifySplitAsync;
    prSplit.verifySplitsAsync = verifySplitsAsync;
    prSplit.verifyEquivalenceAsync = verifyEquivalenceAsync;
    prSplit.verifyEquivalenceDetailedAsync = verifyEquivalenceDetailedAsync;  // T075
    prSplit.cleanupBranchesAsync = cleanupBranchesAsync;
})(globalThis.prSplit);
