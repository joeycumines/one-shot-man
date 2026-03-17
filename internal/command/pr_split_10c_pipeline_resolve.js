'use strict';
// pr_split_10c_pipeline_resolve.js — Pipeline: IPC wait, heuristic fallback, Claude conflict resolution
// Dependencies: chunks 00-10b must be loaded first.

(function(prSplit) {
    // Cross-chunk IIFE-scope imports from 10a and 10b.
    var AUTOMATED_DEFAULTS = prSplit.AUTOMATED_DEFAULTS;
    var getCancellationError = prSplit._getCancellationError;
    var isTransientError = prSplit._isTransientError;
    var captureScreenshot = prSplit._captureScreenshot;
    var sendToHandle = prSplit.sendToHandle;
    var resolveNumber = prSplit._resolveNumber;

    // -----------------------------------------------------------------------
    //  waitForLogged — logged wrapper around mcpCallbackObj.waitFor
    // -----------------------------------------------------------------------

    // Emits before/after log entries so IPC timeouts can be diagnosed
    // post-mortem.
    async function waitForLogged(toolName, timeoutMs, opts) {
        var mcpCb = prSplit._mcpCallbackObj || mcpCallbackObj;
        if (!mcpCb || typeof mcpCb.waitForAsync !== 'function') {
            // T093: Synchronous waitFor fallback removed — it blocks the
            // entire Goja event loop (and therefore BubbleTea) for up to
            // timeoutMs (5+ minutes).  The Go binding ALWAYS provides
            // waitForAsync (mcpcallback.go:207-208).  If it is missing,
            // the MCP callback was not properly initialized.
            return { data: null, error: 'MCP callback missing waitForAsync — cannot proceed (tool: ' + toolName + ')' };
        }

        opts = opts || {};
        var userAliveCheck = (typeof opts.aliveCheck === 'function') ? opts.aliveCheck : null;
        var cancelledByUser = false;
        var forceCancelledByUser = false;

        // Heartbeat monitoring: if opts.heartbeatTool is set, check that the
        // named MCP tool was called within opts.heartbeatTimeoutMs. This
        // detects when Claude is alive but not making progress.
        var heartbeatTool = opts.heartbeatTool || '';
        var heartbeatTimeoutMs = opts.heartbeatTimeoutMs || 0;
        var heartbeatStale = false;

        var wrappedOpts = {};
        for (var k in opts) {
            if (Object.prototype.hasOwnProperty.call(opts, k)) {
                wrappedOpts[k] = opts[k];
            }
        }
        wrappedOpts.aliveCheck = function() {
            if (typeof prSplit.isForceCancelled === 'function' && prSplit.isForceCancelled()) {
                forceCancelledByUser = true;
                return false;
            }
            if (typeof prSplit.isCancelled === 'function' && prSplit.isCancelled()) {
                cancelledByUser = true;
                return false;
            }
            // Heartbeat freshness check. lastCallTime returns the unix ms of
            // the most recent heartbeat call, or 0 if never called.
            if (heartbeatTool && heartbeatTimeoutMs > 0 &&
                typeof mcpCb.lastCallTime === 'function') {
                var hbTime = mcpCb.lastCallTime(heartbeatTool);
                if (hbTime > 0) {
                    // At least one heartbeat received — check staleness.
                    var age = Date.now() - hbTime;
                    if (age > heartbeatTimeoutMs) {
                        log.printf('auto-split: heartbeat stale (%dms since last call to %s), aborting wait for %s',
                            age, heartbeatTool, toolName);
                        heartbeatStale = true;
                        return false;
                    }
                }
                // hbTime === 0 means no heartbeat received yet — skip check
                // (grace period until first heartbeat arrives).
            }
            if (userAliveCheck) {
                return !!userAliveCheck();
            }
            return true;
        };

        log.printf('auto-split waitFor: tool=%s timeout=%dms async=true', toolName, timeoutMs);
        var startMs = Date.now();
        var result = await mcpCb.waitForAsync(toolName, timeoutMs, wrappedOpts);
        var elapsedMs = Date.now() - startMs;
        if (forceCancelledByUser) {
            return { data: null, error: 'force cancelled by user' };
        }
        if (cancelledByUser) {
            return { data: null, error: 'cancelled by user' };
        }
        if (heartbeatStale) {
            return { data: null, error: 'Claude process unresponsive (heartbeat timeout for ' + toolName + ')' };
        }
        if (result.error) {
            log.printf('auto-split waitFor: tool=%s FAILED after %dms — %s', toolName, elapsedMs, result.error);
        } else {
            log.printf('auto-split waitFor: tool=%s received after %dms', toolName, elapsedMs);
        }
        return result;
    }

    // -----------------------------------------------------------------------
    //  heuristicFallback — standard heuristic split flow
    // -----------------------------------------------------------------------

    // Runs the standard heuristic split flow when Claude is unavailable.
    async function heuristicFallback(analysis, config, report) {
        // Late-bind cross-chunk dependencies (async versions for non-blocking).
        var runtime = prSplit.runtime;
        var applyStrategy = prSplit.applyStrategy;
        var createSplitPlan = prSplit.createSplitPlanAsync;
        var executeSplit = prSplit.executeSplitAsync;
        var verifySplits = prSplit.verifySplitsAsync;
        var verifyEquivalence = prSplit.verifyEquivalenceAsync;
        var resolveConflicts = prSplit.resolveConflicts;
        var state = prSplit._state;

        var strategy = config.strategy || runtime.strategy;
        var groups = applyStrategy(analysis.files, strategy, {
            fileStatuses: analysis.fileStatuses,
            maxFiles: runtime.maxFiles,
            baseBranch: analysis.baseBranch
        });
        state.groupsCache = groups;

        var plan = await createSplitPlan(groups, {
            baseBranch: analysis.baseBranch,
            sourceBranch: analysis.currentBranch,
            branchPrefix: runtime.branchPrefix,
            maxFiles: runtime.maxFiles,
            fileStatuses: analysis.fileStatuses
        });
        state.planCache = plan;
        report.plan = plan;

        if (!runtime.dryRun) {
            var execResult = await executeSplit(plan);
            if (execResult.error) {
                report.error = execResult.error;
                return { error: execResult.error, report: report };
            }
            report.splits = execResult.results || [];

            var verifyObj = await verifySplits(plan);
            var failures = verifyObj.results.filter(function(r) { return !r.passed; });
            if (failures.length > 0) {
                await resolveConflicts(plan);
            }

            var equiv = await verifyEquivalence(plan);
            // T121: Propagate equivalence result to report.
            report.equivalence = equiv;
            if (!equiv.equivalent) {
                report.error = 'tree hash mismatch after heuristic split';
            }
        }

        output.print('=== Heuristic Split Complete ===');
        output.print('Splits: ' + plan.splits.length);

        return { error: report.error, report: report };
    }

    // -----------------------------------------------------------------------
    //  resolveConflictsWithClaude — Claude-based conflict resolution
    // -----------------------------------------------------------------------

    // Attempts to fix failing splits using Claude.
    async function resolveConflictsWithClaude(failures, sessionId, timeouts, pollInterval, maxAttemptsPerBranch, report, aliveCheckFn, heartbeatTimeoutMs) {
        // Late-bind cross-chunk dependencies.
        var isCancelled = prSplit.isCancelled;
        var isForceCancelled = prSplit.isForceCancelled;  // T117
        var resolveDir = prSplit._resolveDir;
        var renderConflictPrompt = prSplit.renderConflictPrompt;
        var validateResolution = prSplit.validateResolution;
        var verifySplitAsync = prSplit.verifySplitAsync;
        var gitExecAsync = prSplit._gitExecAsync;
        var gitAddChangedFilesAsync = prSplit._gitAddChangedFilesAsync;
        var shellExecAsync = prSplit._shellExecAsync;
        var shellQuote = prSplit._shellQuote;
        var osmod = prSplit._modules.osmod;
        var runtime = prSplit.runtime;
        var state = prSplit._state;
        var claudeExec = state.claudeExecutor;
        var recordConversation = prSplit.recordConversation || function() {};
        var dir = resolveDir('.');

        var reSplitNeeded = false;
        var reSplitReason = '';

        // Wall-clock timeout: cap total elapsed time.
        var wallClockMs = (timeouts && typeof timeouts.wallClockMs === 'number')
            ? timeouts.wallClockMs
            : Math.min(((timeouts && timeouts.resolve) || AUTOMATED_DEFAULTS.resolveTimeoutMs) * (maxAttemptsPerBranch || 3) + 60000,
                       AUTOMATED_DEFAULTS.resolveWallClockTimeoutMs);
        var deadlineStart = Date.now();
        var deadline = deadlineStart + wallClockMs;

        // Per-command timeout for resolution commands (T109).
        var commandTimeoutMs = (timeouts && typeof timeouts.commandMs === 'number')
            ? timeouts.commandMs
            : AUTOMATED_DEFAULTS.resolveCommandTimeoutMs;

        for (var i = 0; i < failures.length; i++) {
            if (Date.now() >= deadline) {
                return { reSplitNeeded: false, reSplitReason: 'wall-clock timeout after ' + (Date.now() - deadlineStart) + 'ms (limit: ' + wallClockMs + 'ms)' };
            }
            if (isCancelled() || isForceCancelled()) {  // T117
                return { reSplitNeeded: false, reSplitReason: 'cancelled by user' };
            }

            var fail = failures[i];
            var fixed = false;

            for (var attempt = 0; attempt < maxAttemptsPerBranch && !fixed; attempt++) {
                if (Date.now() >= deadline) {
                    return { reSplitNeeded: false, reSplitReason: 'wall-clock timeout after ' + (Date.now() - deadlineStart) + 'ms (limit: ' + wallClockMs + 'ms)' };
                }
                if (isCancelled() || isForceCancelled()) {  // T117
                    return { reSplitNeeded: false, reSplitReason: 'cancelled by user' };
                }

                // Exponential backoff between retry attempts (skip delay on first attempt).
                if (attempt > 0) {
                    var backoffBaseMs = resolveNumber(prSplit._RESOLVE_BACKOFF_BASE_MS, 2000, 0);
                    var backoffMs = Math.min(backoffBaseMs * Math.pow(2, attempt - 1), 30000);
                    log.printf('auto-split: retrying %s after %dms backoff (attempt %d/%d)',
                        fail.branch || fail.name, backoffMs, attempt + 1, maxAttemptsPerBranch);
                    await new Promise(function(resolve) { setTimeout(resolve, backoffMs); });
                    if (Date.now() >= deadline) {
                        return { reSplitNeeded: false, reSplitReason: 'wall-clock timeout during backoff' };
                    }
                    if (isCancelled() || isForceCancelled()) {  // T117
                        return { reSplitNeeded: false, reSplitReason: 'cancelled during backoff' };
                    }
                }

                report.conflicts.push({
                    branch: fail.branch || fail.name,
                    attempt: attempt + 1,
                    error: fail.error || fail.output || ''
                });

                // Send conflict prompt to Claude.
                var promptResult = renderConflictPrompt({
                    branchName: fail.branch || fail.name,
                    files: fail.files || [],
                    exitCode: fail.exitCode || 1,
                    errorOutput: fail.error || fail.output || '',
                    goModContent: ''
                });
                if (promptResult.error) {
                    log.printf('auto-split: conflict prompt render failed: %s', promptResult.error);
                    break;
                }

                var sendResult = await sendToHandle(claudeExec.handle, promptResult.text);
                if (sendResult.error) {
                    log.printf('auto-split: failed to send conflict prompt: %s', sendResult.error);
                    if (!isTransientError(sendResult.error)) {
                        log.printf('auto-split: permanent send error for %s — skipping retries', fail.branch || fail.name);
                        break;
                    }
                    continue; // transient — allow backoff + retry
                }
                report.claudeInteractions++;
                recordConversation('conflict-resolution', promptResult.text, '');

                // Wait for resolution via mcpcallback.
                var mcpCb = prSplit._mcpCallbackObj;
                mcpCb.resetWaiter('reportResolution');
                var resolutionPoll = await waitForLogged('reportResolution', timeouts.resolve, {
                    aliveCheck: aliveCheckFn,
                    heartbeatTool: 'heartbeat',
                    heartbeatTimeoutMs: heartbeatTimeoutMs,
                    onProgress: function(elapsed) {},
                    checkIntervalMs: pollInterval
                });
                if (resolutionPoll.error) {
                    log.printf('auto-split: resolution timeout for %s (attempt %d): %s',
                        fail.branch || fail.name, attempt + 1, resolutionPoll.error);
                    if (!isTransientError(resolutionPoll.error)) {
                        log.printf('auto-split: permanent resolution error for %s — skipping retries', fail.branch || fail.name);
                        break;
                    }
                    continue;
                }

                var resolution = resolutionPoll.data;
                var resVal = validateResolution(resolution);
                if (!resVal.valid) {
                    log.printf('auto-split: resolution validation errors for %s: %s',
                        fail.branch || fail.name, resVal.errors.join('; '));
                    continue;
                }
                report.resolutions.push(resolution);

                // Pre-existing failure handling.
                if (resolution.preExistingFailure) {
                    report.preExistingFailures = report.preExistingFailures || [];
                    report.preExistingFailures.push({
                        branch: fail.branch || fail.name,
                        details: resolution.preExistingDetails || ''
                    });
                    fixed = true;
                    output.print('[auto-split] Pre-existing failure: ' + (fail.branch || fail.name) +
                        (resolution.preExistingDetails ? ' (' + resolution.preExistingDetails + ')' : ''));
                    break;
                }

                // Check if re-split is suggested.
                if (resolution.reSplitSuggested) {
                    reSplitNeeded = true;
                    reSplitReason = resolution.reSplitReason || 'Claude suggested re-split';
                    break;
                }

                // Apply patches and commands in a temporary worktree on the
                // failing branch. This ensures we modify the correct branch
                // without touching the user's CWD.
                var patchBranch = fail.branch || fail.name;
                var patchWorktreeDir = dir + '/../.osm-resolve-' + Date.now() + '-' + Math.floor(Math.random() * 10000);
                var patchWtAdd = await gitExecAsync(dir, ['worktree', 'add', patchWorktreeDir, patchBranch]);
                if (patchWtAdd.code !== 0) {
                    log.printf('auto-split: failed to create worktree for %s: %s', patchBranch, patchWtAdd.stderr.trim());
                    continue;
                }

                // Apply patches in worktree.
                if (resolution.patches && resolution.patches.length > 0) {
                    for (var p = 0; p < resolution.patches.length; p++) {
                        var patch = resolution.patches[p];
                        if (osmod) {
                            osmod.writeFile(patchWorktreeDir + '/' + patch.file, patch.content);
                        }
                    }
                    await gitAddChangedFilesAsync(patchWorktreeDir);
                    var patchCommit = await gitExecAsync(patchWorktreeDir, ['commit', '--amend', '--no-edit']);
                    if (patchCommit.code !== 0) {
                        log.printf('auto-split: patch commit failed for %s: %s', patchBranch, patchCommit.stderr.trim());
                    }
                }

                // Run suggested commands in worktree (async — T109: does not block event loop).
                // Each command has a per-command timeout (resolveCommandTimeoutMs).
                if (resolution.commands && resolution.commands.length > 0) {
                    var commandsAborted = false;
                    for (var c = 0; c < resolution.commands.length; c++) {
                        var cmdTimeoutPromise = new Promise(function(resolve) {
                            setTimeout(function() { resolve({ stdout: '', stderr: 'command timed out', code: -1, error: true, message: 'timed out after ' + commandTimeoutMs + 'ms' }); }, commandTimeoutMs);
                        });
                        var cmdResult = await Promise.race([
                            shellExecAsync('cd ' + shellQuote(patchWorktreeDir) + ' && ' + resolution.commands[c]),
                            cmdTimeoutPromise
                        ]);
                        if (cmdResult.code === -1 && cmdResult.message && cmdResult.message.indexOf('timed out') !== -1) {
                            log.printf('auto-split: resolution command timed out for %s after %dms: %s',
                                patchBranch, commandTimeoutMs, resolution.commands[c]);
                            commandsAborted = true;
                            break;
                        }
                        if (cmdResult.code !== 0) {
                            log.printf('auto-split: resolution command failed for %s: %s (exit %d)',
                                patchBranch, (cmdResult.stderr || cmdResult.stdout || '').trim(), cmdResult.code);
                        }
                    }
                    if (!commandsAborted) {
                        await gitAddChangedFilesAsync(patchWorktreeDir);
                        var cmdCommit = await gitExecAsync(patchWorktreeDir, ['commit', '--amend', '--no-edit']);
                        if (cmdCommit.code !== 0) {
                            log.printf('auto-split: command commit failed for %s: %s', patchBranch, cmdCommit.stderr.trim());
                        }
                    }
                }

                // Cleanup resolution worktree before re-verify.
                await gitExecAsync(dir, ['worktree', 'remove', '--force', patchWorktreeDir]);

                // Re-verify this branch (verifySplitAsync creates its own worktree).
                var reVerify = await verifySplitAsync(fail.branch || fail.name, { verifyCommand: runtime.verifyCommand });
                if (reVerify.passed) {
                    fixed = true;
                    output.print('[auto-split] Fixed: ' + (fail.branch || fail.name));
                }
            }

            if (!fixed && !reSplitNeeded) {
                log.printf('auto-split: Claude resolution exhausted for %s, trying local strategies', fail.branch || fail.name);
            }
        }

        return { reSplitNeeded: reSplitNeeded, reSplitReason: reSplitReason };
    }

    // Exports.
    prSplit.waitForLogged = waitForLogged;
    prSplit.heuristicFallback = heuristicFallback;
    prSplit.resolveConflictsWithClaude = resolveConflictsWithClaude;
})(globalThis.prSplit);
