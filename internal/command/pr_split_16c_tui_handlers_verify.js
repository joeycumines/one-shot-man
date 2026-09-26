'use strict';
// pr_split_16c_tui_handlers_verify.js — TUI: verify handlers, confirm cancel, Agent conversation, error resolution
// Dependencies: chunks 00-16b must be loaded first.
// Requires Go-injected globals: tui, ctx, output, log, prSplitConfig.

(function(prSplit) {

    if (typeof tui === 'undefined' || typeof ctx === 'undefined' ||
        typeof output === 'undefined') { return; }

    // Cross-chunk imports.
    var tea = prSplit._tea;
    var zone = prSplit._zone;
    var st = prSplit._state;
    var C = prSplit._TUI_CONSTANTS;
    var getInteractivePaneSession = prSplit._getInteractivePaneSession;
    var clearVerifyPaneSession = prSplit._clearVerifyPaneSession;
    var nextPaneCleanupDelay = prSplit._nextPaneCleanupDelay;
    var paneCleanupPending = prSplit._paneCleanupPending;
    var paneCleanupExpired = prSplit._paneCleanupExpired;
    var paneCleanupErrorDetail = prSplit._paneCleanupErrorDetail;
    var handleErrorResolutionState = prSplit._handleErrorResolutionState;
    // Late-bound cross-chunk references (defined in later chunks, resolved at call time).
    function startAnalysis(s) { return prSplit._startAnalysis(s); }
    function startEquivCheck(s) { return prSplit._startEquivCheck(s); }
    function getVerifyMode(s, activeVerifySession) {
        if (s.verifyMode) return s.verifyMode;
        return activeVerifySession ? 'interactive' : 'textonly';
    }

    function effectiveVerifyTimeoutMs(value) {
        if (typeof prSplit._effectiveVerifyTimeoutMs === 'function') {
            return prSplit._effectiveVerifyTimeoutMs(value);
        }
        var defaults = prSplit.AUTOMATED_DEFAULTS || {};
        return (typeof value === 'number' && value > 0) ? value : defaults.verifyTimeoutMs;
    }

    function currentVerifyRunEpoch(s) {
        if (typeof s._verifyRunEpoch !== 'number' || s._verifyRunEpoch !== s._verifyRunEpoch) {
            s._verifyRunEpoch = 0;
        }
        return s._verifyRunEpoch;
    }

    // --- Update Handlers — screen-specific input handling ---
    function updateConfirmCancel(msg, s) {
        // Helper to clean up any active verify session before quitting.
        function cleanupActiveSession() {
            // T325: Reset tab before clearing session for atomic state transition.
            if (s.splitViewTab === 'verify') {
                s.splitViewTab = 'output';
            }
            clearVerifyPaneSession(s, { debugPrefix: 'cleanup', keepDisplay: false });
        }

        // T031: Helper to confirm cancel and quit.
        function confirmCancel() {
            s.showConfirmCancel = false;
            s.confirmCancelFocus = 0;  // reset for next open
            s.isProcessing = false;
            s.analysisRunning = false; // T001: stop orphaned analysis poll ticks
            s.autoSplitRunning = false; // T001: same for auto-split pipeline
            if (typeof prSplit._resetVerifyRunState === 'function') {
                prSplit._resetVerifyRunState(s);
            }
            if (typeof prSplit._invalidateAsyncConfig === 'function') {
                prSplit._invalidateAsyncConfig(s);
            }
            cleanupActiveSession();
            // T393: Clean up Agent executor and MCP callback on wizard exit.
            // Deferred quit: confirmCancel is sync so async teardown
            // (executor close, MCP close) runs on the promise chain and the
            // wizard quits on the next tick after close settles.
            s.wizardQuitting = true;
            s.wizardQuitSent = false;
            // This path gates on four teardown steps (executor, MCP,
            // persistence, pane) and publishes its own result through
            // wizardQuitSent, so the wizard-quit tick must not treat it as
            // pane-gated.
            s.wizardQuitPaneGated = false;
            var mcpDone = false;
            var execDone = false;
            var persistenceDone = false;
            var paneDone = false;
            function maybeQuit() {
                if (execDone && mcpDone && persistenceDone && paneDone && !s.wizardQuitSent) {
                    s.wizardQuitSent = true;
                }
            }
            if (st && st.agentExecutor) {
                try {
                    var closeResult = st.agentExecutor.close();
                    if (closeResult && typeof closeResult.catch === 'function') {
                        closeResult.then(function() {
                            execDone = true;
                            maybeQuit();
                        }, function(e) {
                            log.debug('cleanup agentExec close failed', { error: e.message || String(e) });
                            execDone = true;
                            maybeQuit();
                        });
                    } else {
                        execDone = true;
                    }
                } catch (e) {
                    log.debug('cleanup agentExec close failed', { error: e.message || String(e) });
                    execDone = true;
                }
            } else {
                execDone = true;
            }
            if (typeof prSplit._destroyAgentTermpane === 'function') {
                try {
                    var paneResult = prSplit._destroyAgentTermpane();
                    var paneSettled = function() {
                        paneDone = true;
                        maybeQuit();
                    };
                    prSplit._trackPaneOutcome(s, paneResult, paneSettled, function(e) {
                        log.debug('cleanup live pane close failed', { error: e.message || String(e) });
                        paneSettled();
                    });
                } catch (e) {
                    log.debug('cleanup live pane close failed', { error: e.message || String(e) });
                    paneDone = true;
                }
            } else {
                paneDone = true;
            }
            var mcpCb = prSplit._mcpCallbackObj;
            if (mcpCb) {
                try {
                    var mcpResult = mcpCb.close();
                    if (mcpResult && typeof mcpResult.catch === 'function') {
                        mcpResult.then(function() {
                            prSplit._mcpCallbackObj = null;
                            if (st) st.mcpCallbackObj = null;
                            mcpDone = true;
                            maybeQuit();
                        }, function(e) {
                            log.debug('cleanup mcp close failed', { error: e.message || String(e) });
                            prSplit._mcpCallbackObj = null;
                            if (st) st.mcpCallbackObj = null;
                            mcpDone = true;
                            maybeQuit();
                        });
                    } else {
                        prSplit._mcpCallbackObj = null;
                        if (st) st.mcpCallbackObj = null;
                        mcpDone = true;
                    }
                } catch (e) {
                    log.debug('cleanup mcp close failed', { error: e.message || String(e) });
                    prSplit._mcpCallbackObj = null;
                    if (st) st.mcpCallbackObj = null;
                    mcpDone = true;
                }
            } else {
                mcpDone = true;
            }
            // Task 10: Clean exit removes state file so next startup
            // doesn't offer stale resume data.
            if (prSplit.persistence && typeof prSplit.persistence.cleanup === 'function') {
                try {
                    var persistenceResult = prSplit.persistence.cleanup();
                    if (persistenceResult && typeof persistenceResult.then === 'function') {
                        persistenceResult.then(function() {
                            persistenceDone = true;
                            maybeQuit();
                        }, function(e) {
                            log.debug('cleanup persistence cleanup failed', { error: e.message || String(e) });
                            persistenceDone = true;
                            maybeQuit();
                        });
                    } else {
                        persistenceDone = true;
                    }
                } catch (e) {
                    log.debug('cleanup persistence cleanup failed', { error: e.message || String(e) });
                    persistenceDone = true;
                }
            } else {
                persistenceDone = true;
            }
            maybeQuit();
            // Task 9: Unwire Agent lifecycle event handlers.
            if (typeof prSplit._unwireAgentLifecycleEvents === 'function') {
                try { prSplit._unwireAgentLifecycleEvents(); } catch (e) { log.debug('cleanup unwireAgentLifecycleEvents failed', { error: e.message || String(e) }); }
            }
            s.wizard.cancel();
            s.wizardState = 'CANCELLED';
            if (s.wizardQuitSent) {
                return [s, tea.quit()];
            }
            return [s, tea.tick(C.TICK_INTERVAL_MS, 'wizard-quit')];
        }

        // T031: Helper to dismiss overlay (keep going).
        function dismissOverlay() {
            s.showConfirmCancel = false;
            s.confirmCancelFocus = 0;  // reset for next open
            return [s, null];
        }

        // T031: Ensure focus index is initialized and valid.
        if (typeof s.confirmCancelFocus !== 'number' || s.confirmCancelFocus < 0 || s.confirmCancelFocus > 1 || s.confirmCancelFocus !== s.confirmCancelFocus) {
            s.confirmCancelFocus = 0;
        }

        if (msg.type === 'Key') {
            var k = msg.key;
            // T031: Tab / Shift+Tab cycles focus between Yes (0) and No (1).
            if (k === 'tab') {
                s.confirmCancelFocus = (s.confirmCancelFocus + 1) % 2;
                return [s, null];
            }
            if (k === 'shift+tab') {
                s.confirmCancelFocus = (s.confirmCancelFocus - 1 + 2) % 2;
                return [s, null];
            }
            // T031: Enter activates the focused button.
            if (k === 'enter') {
                if (s.confirmCancelFocus === 0) {
                    return confirmCancel();
                }
                return dismissOverlay();
            }
            // y always confirms, n/esc always dismisses (regardless of focus).
            if (k === 'y') {
                return confirmCancel();
            }
            if (k === 'n' || k === 'esc') {
                return dismissOverlay();
            }
        }
        if (msg.type === 'MouseClick') {
            if (zone.inBounds('confirm-yes', msg)) {
                return confirmCancel();
            }
            if (zone.inBounds('confirm-no', msg)) {
                return dismissOverlay();
            }
        }
        return [s, null];
    }

    // --- Pre-existing failure detection ---
    function _isPreExistingFailure(s) {
        return !!(s._baselineVerifyResult && s._baselineVerifyResult.failed);
    }
    function _preExistingAnnotation(s) {
        if (!s._baselineVerifyResult || !s._baselineVerifyResult.sourceBranch) return '';
        return ' (pre-existing on ' + s._baselineVerifyResult.sourceBranch + ')';
    }

    function isThenable(value) {
        return value && typeof value.then === 'function';
    }

    function advanceVerifyIfReady(s) {
        if (!s._verifyAdvanceAfterCleanup) return 'none';
        if (s._verifyPaneCleanupPending || s._verifyTimeoutKillPending) return 'waiting';
        s._verifyAdvanceAfterCleanup = false;
        s.verifyingIdx++;
        return 'advanced';
    }

    function nextVerifySetupEpoch(s) {
        if (typeof s._verifySetupEpoch !== 'number') s._verifySetupEpoch = 0;
        s._verifySetupEpoch++;
        return s._verifySetupEpoch;
    }

    function disposeVerifySetupResult(result) {
        if (!result) return;
        var cleaned = false;
        var cleanupWorktree = function() {
            if (cleaned) return null;
            cleaned = true;
            if (!result.worktreeDir || !result.dir ||
                typeof prSplit.cleanupVerifyWorktree !== 'function') {
                return null;
            }
            try {
                return prSplit.cleanupVerifyWorktree(result.dir, result.worktreeDir);
            } catch (e) {
                log.debug('verifySetup: stale worktree cleanup failed', { error: e.message || String(e) });
                return null;
            }
        };

        var closeResult = null;
        if (result.session && typeof result.session.close === 'function') {
            try {
                closeResult = result.session.close();
            } catch (e) {
                log.debug('verifySetup: stale session close failed', { error: e.message || String(e) });
            }
        }
        var disposeResult = null;
        if (isThenable(closeResult)) {
            disposeResult = Promise.resolve(closeResult).then(cleanupWorktree, cleanupWorktree);
        } else {
            disposeResult = cleanupWorktree();
        }
        if (isThenable(disposeResult) && typeof prSplit._trackPaneOperation === 'function') {
            prSplit._trackPaneOperation(s, disposeResult);
        }
    }


    function invalidateVerifySetup(s) {
        // Bump the epoch even when no promise is pending so a late callback
        // from an earlier run cannot attach a result to a replacement run.
        nextVerifySetupEpoch(s);
        var state = s._verifySetup;
        s._verifySetup = null;
        if (!state || state.cancelled) return;
        state.cancelled = true;
        if (!state.pending) {
            disposeVerifySetupResult(state.result);
        }
    }

    function deferVerifySetup(s, kind, branchName, promise) {
        invalidateVerifySetup(s);
        var state = {
            kind: kind,
            branchName: branchName,
            epoch: s._verifySetupEpoch,
            pending: true,
            cancelled: false,
            result: null,
            promise: promise
        };
        s._verifySetup = state;
        var settle = function(result) {
            if (!state.cancelled && s._verifySetup === state && s._verifySetupEpoch === state.epoch) {
                state.result = result;
                state.pending = false;
                return;
            }
            disposeVerifySetupResult(result);
        };
        try {
            promise.then(settle, function(err) {
                settle({ error: (err && err.message) ? err.message : String(err) });
            });
            if (typeof prSplit._trackPaneOperation === 'function') {
                prSplit._trackPaneOperation(s, promise);
            }
        } catch (err) {
            settle({ error: (err && err.message) ? err.message : String(err) });
        }
    }

    function pendingVerifySetup(s, kind, branchName) {
        var state = s._verifySetup;
        if (!state || state.cancelled || state.kind !== kind || state.branchName !== branchName) {
            return null;
        }
        if (state.pending) {
            return { pending: true, result: null };
        }
        s._verifySetup = null;
        return { pending: false, result: state.result };
    }

    function startInteractiveSession(session, worktreeDir, dir) {
        if (!session) {
            return { pending: false, result: { error: 'interactive shell unavailable' } };
        }
        if (typeof session.start !== 'function') {
            return { pending: false, result: { session: session } };
        }

        var started;
        try {
            started = session.start();
        } catch (e) {
            return {
                pending: false,
                result: {
                    error: (e && e.message) ? e.message : String(e),
                    worktreeDir: worktreeDir,
                    dir: dir
                }
            };
        }
        if (!isThenable(started)) {
            return { pending: false, result: { session: session } };
        }
        return {
            pending: true,
            promise: started.then(function() {
                return { session: session, worktreeDir: worktreeDir, dir: dir };
            }, function(e) {
                var failure = {
                    error: (e && e.message) ? e.message : String(e),
                    worktreeDir: worktreeDir,
                    dir: dir
                };
                var closeResult = null;
                if (session && typeof session.close === 'function') {
                    try {
                        closeResult = session.close();
                    } catch (closeErr) {
                        log.debug('startInteractiveSession: failed to close failed session', {
                            error: closeErr.message || String(closeErr)
                        });
                    }
                }
                if (isThenable(closeResult)) {
                    return closeResult.then(function() { return failure; }, function() { return failure; });
                }
                return failure;
            })
        };
    }

    // --- Per-branch verification (persistent shell, Task 7) ---
    // Verifies one branch at a time using a PERSISTENT INTERACTIVE SHELL
    // in the branch worktree — NOT a one-shot verify command.
    //
    // Key differences from the old one-shot model:
    //   - The verify pane shows a live interactive shell (PTY + VTerm)
    //   - The user types commands (e.g. make test, go test ./...) manually
    //   - The user signals completion with explicit PASS/FAIL/CONTINUE
    //     (keyboard: p/f/c, or buttons in the verify pane footer)
    //   - The session NEVER exits on its own — command exit is ignored;
    //     only user signal advances to the next branch
    //   - The worktree persists for the duration of the session, so the
    //     user can run commands, fix code, and re-verify iteratively
    //
    // failVerifyOnStuckTeardown escalates a verify wait that outlived the
    // teardown budget. Every verify loop calls this instead of re-arming
    // forever: the branch is recorded as failed with a diagnosable error and
    // polling stops, so the wizard reaches a terminal outcome the user can act
    // on rather than a permanent spinner.
    function failVerifyOnStuckTeardown(s) {
        var branch = s.activeVerifyBranch || '';
        var detail = paneCleanupErrorDetail('Verification of ' + (branch || 'the current branch') + ' was abandoned.');
        log.warn('verify abandoned after stuck teardown', { branch: branch });
        try {
            if (s.outputLines) s.outputLines.push(detail);
            if (branch && s.verificationResults) {
                s.verificationResults.push({
                    name: branch,
                    status: prSplit._branchStatuses.FAILED,
                    passed: false,
                    skipped: false,
                    error: detail,
                    output: '',
                    duration: s.verifyElapsedMs || 0,
                    preExisting: false
                });
            }
        } catch (e) {
            log.debug('verify teardown escalation failed', { error: e.message || String(e) });
        }
        s._verifyAdvanceAfterCleanup = true;
        s.verifyDeadline = 0;
        s.activeVerifyBranch = null;
        s.isProcessing = false;
        s.verifyShellExited = false;
        return prSplit._enterErrorState(s, detail);
    }

    // Platform support:
    //   - PTY platforms (Unix/Linux/macOS): persistent shell via spawnShellSession
    //   - Non-PTY platforms (Windows fallback): uses async verifySplitAsync
    //     which still runs one-shot commands (persistent shell is a Unix feature)
    function runVerifyBranch(s) {
        if (!s.isProcessing) return [s, null];
        var advanceState = advanceVerifyIfReady(s);
        if (advanceState === 'advanced') return [s, tea.tick(1, 'verify-branch')];
        if (advanceState !== 'advanced' && paneCleanupExpired(s)) {
            return failVerifyOnStuckTeardown(s);
        }
        if (advanceState === 'waiting' || paneCleanupPending(s)) {
            return [s, tea.tick(nextPaneCleanupDelay(s), 'verify-branch')];
        }
        if (s.wizard && s.wizard.current === 'CANCELLED') {
            invalidateVerifySetup(s);
            if (typeof prSplit._resetVerifyRunState === 'function') {
                prSplit._resetVerifyRunState(s);
            }
            return [s, null];
        }

        // Guard: clean up any existing verify session before starting a new one.
        // Prevents session leaks if runVerifyBranch is called while a previous
        // verify session is still active (e.g., rapid branch transitions, error
        // recovery, pipeline restarts, or duplicate tick scheduling).
        if (s.activeVerifySession) {
            clearVerifyPaneSession(s, { keepDisplay: false, debugPrefix: 'runVerifyBranch-guard' });
            if (s._verifyPaneCleanupPending) {
                return [s, tea.tick(10, 'verify-branch')];
            }
        }

        var splits = st.planCache.splits;
        if (!splits || s.verifyingIdx >= splits.length) {
            // All branches verified — transition to equiv check phase.
            prSplit._transitionVerifyPhase(s, prSplit._verifyPhases.EQUIV_CHECK);
            return startEquivCheck(s);
        }

        // First call: transition from NOT_STARTED to RUNNING.
        if (s.verifyPhase === prSplit._verifyPhases.NOT_STARTED) {
            prSplit._transitionVerifyPhase(s, prSplit._verifyPhases.RUNNING);
        }

        // T115: On the very first branch, kick off an async baseline check
        // against the source branch. The result is cached on `s` so that
        // pollVerifySession can tag failures as pre-existing when the baseline
        // also fails.
        if (s.verifyingIdx === 0 && !s._baselineVerifyStarted) {
            var sourceBranch = st.planCache.sourceBranch;
            if (sourceBranch && prSplit.runtime.verifyCommand) {
                s._baselineVerifyStarted = true;
                s._baselineVerifyResult = null;
                var baseDir = prSplit.runtime.dir || '.';
                var baselineEpoch = currentVerifyRunEpoch(s);
                var baseTimeoutMs = effectiveVerifyTimeoutMs(
                    (typeof prSplitConfig !== 'undefined') ? prSplitConfig.timeoutMs : 0
                );
                var baselineStillCurrent = function() {
                    return s._verifyRunEpoch === baselineEpoch &&
                        s.isProcessing &&
                        (!s.wizard || s.wizard.current !== 'CANCELLED');
                };
                prSplit.verifySplitAsync(sourceBranch, {
                    dir: baseDir,
                    verifyCommand: prSplit.runtime.verifyCommand,
                    verifyTimeoutMs: baseTimeoutMs,
                    isCancelled: function() {
                        return !baselineStillCurrent();
                    },
                    outputFn: null
                }).then(function(result) {
                    if (!baselineStillCurrent()) return;
                    s._baselineVerifyResult = {
                        failed: !result.passed,
                        sourceBranch: sourceBranch
                    };
                }, function() {
                    if (!baselineStillCurrent()) return;
                    // Baseline check errored — conservatively treat as no info.
                    s._baselineVerifyResult = { failed: false, sourceBranch: sourceBranch };
                });
            }
        }

        var split = splits[s.verifyingIdx];
        var branchName = split.name;

        // Check dependency chain — skip if any dependency failed.
        var deps = split.dependencies || [];
        var skipReason = '';
        for (var d = 0; d < deps.length; d++) {
            for (var r = 0; r < s.verificationResults.length; r++) {
                if (s.verificationResults[r].name === deps[d] &&
                    !s.verificationResults[r].passed &&
                    !s.verificationResults[r].preExisting) {
                    skipReason = 'skipped: dependency ' + deps[d] + ' failed';
                    break;
                }
            }
            if (skipReason) break;
        }

        if (skipReason) {
            s.verificationResults.push({
                name: branchName,
                status: prSplit._branchStatuses.SKIPPED,
                passed: false,
                skipped: true,
                error: skipReason,
                duration: 0,
                preExisting: false
            });
            s.verifyingIdx++;
            return [s, tea.tick(1, 'verify-branch')];
        }

        var dir = prSplit.runtime.dir || '.';
        var scopedCmd = prSplit.runtime.verifyCommand;
        // Scoped verify: filter command based on split's files if applicable.
        if (typeof prSplit.scopedVerifyCommand === 'function' && split.files) {
            var scoped = prSplit.scopedVerifyCommand(split.files, scopedCmd);
            if (scoped) scopedCmd = scoped;
        }

        // Reset per-branch verify mode state before selecting the new path.
        s.verifyMode = null;
        s.verifyHint = '';
        s.verifyShellExited = false;
        s.verifyFallbackRunning = false;
        s.verifyFallbackError = null;
        s.verifyDeadline = 0;

        // --- Canonical mode selection ---
        // Determine the verify mode BEFORE creating any sessions.
        // Priority: interactive > oneshot > textonly.
        var spawnShellFn = (typeof prSplit.spawnShellSession === 'function')
            ? prSplit.spawnShellSession
            : null;
        var canSpawnShell = (typeof prSplit.canSpawnInteractiveShell === 'function')
            ? prSplit.canSpawnInteractiveShell()
            : false;
        var wantInteractive = spawnShellFn && canSpawnShell;

        // --- Interactive path (canonical): prepare worktree, spawn shell ---
        if (wantInteractive) {
            var setup = pendingVerifySetup(s, 'interactive', branchName);
            if (setup && setup.pending) {
                return [s, tea.tick(10, 'verify-branch')];
            }
            var wt = setup ? setup.result : prSplit.prepareVerifyWorktree(branchName, {
                dir: dir,
                verifyCommand: scopedCmd
            });
            if (isThenable(wt)) {
                deferVerifySetup(s, 'interactive', branchName, wt);
                return [s, tea.tick(10, 'verify-branch')];
            }

            if (wt && wt.skipped) {
                s.verificationResults.push({
                    name: branchName,
                    status: prSplit._branchStatuses.SKIPPED,
                    passed: true,
                    skipped: true,
                    error: null,
                    output: '',
                    duration: 0,
                    preExisting: false
                });
                s.verifyingIdx++;
                return [s, tea.tick(1, 'verify-branch')];
            }

            if (!wt || wt.error) {
                s.verificationResults.push({
                    name: branchName,
                    status: prSplit._branchStatuses.FAILED,
                    passed: false,
                    skipped: false,
                    error: wt && wt.error ? wt.error : 'prepare verify worktree returned no result',
                    output: '',
                    duration: 0,
                    preExisting: false
                });
                s.verifyingIdx++;
                return [s, tea.tick(1, 'verify-branch')];
            }

            // T325+T380: Auto-open split-view with Verify tab.
            if (!s.splitViewEnabled && s.height >= C.INLINE_VIEW_HEIGHT) {
                s.splitViewEnabled = true;
                s.splitViewFocus = 'agent';
                s.splitViewTab = 'verify';
                if (typeof prSplit._syncMainViewport === 'function') {
                    prSplit._syncMainViewport(s);
                }
            } else if (s.splitViewEnabled) {
                s.splitViewTab = 'verify';
            }

            var paneRows = C.DEFAULT_ROWS;
            var paneCols = Math.max(80, (s.width || 80) - 8);
            var persistentShell = null;
            var startSetup = pendingVerifySetup(s, 'interactive-start', branchName);
            if (startSetup && startSetup.pending) {
                return [s, tea.tick(10, 'verify-branch')];
            }
            if (startSetup) {
                if (startSetup.result && !startSetup.result.error) {
                    persistentShell = startSetup.result.session;
                } else {
                    log.debug('runVerifyBranch: interactive shell startup failed', {
                        error: startSetup.result && startSetup.result.error
                    });
                }
            } else {
                try {
                    var spawnedShell = spawnShellFn(wt.worktreeDir, {
                        rows: paneRows,
                        cols: paneCols,
                        deferStart: true
                    });
                    if (isThenable(spawnedShell)) {
                        var creation = Promise.resolve(spawnedShell).then(function(session) {
                            return startInteractiveSession(session, wt.worktreeDir, wt.dir);
                        }, function(e) {
                            return {
                                pending: false,
                                result: {
                                    error: (e && e.message) ? e.message : String(e),
                                    worktreeDir: wt.worktreeDir,
                                    dir: wt.dir
                                }
                            };
                        });
                        deferVerifySetup(s, 'interactive-start', branchName, creation.then(function(startState) {
                            return startState.pending ? startState.promise : startState.result;
                        }));
                        return [s, tea.tick(10, 'verify-branch')];
                    }
                    var startState = startInteractiveSession(spawnedShell, wt.worktreeDir, wt.dir);
                    if (startState.pending) {
                        deferVerifySetup(s, 'interactive-start', branchName, startState.promise);
                        return [s, tea.tick(10, 'verify-branch')];
                    }
                    if (startState.result && !startState.result.error) {
                        persistentShell = startState.result.session;
                    } else {
                        log.debug('runVerifyBranch: interactive shell startup failed', {
                            error: startState.result && startState.result.error
                        });
                    }
                } catch (e) {
                    log.debug('runVerifyBranch: spawnShellSession failed', { error: e.message || String(e) });
                    persistentShell = null;
                }
            }

            if (persistentShell) {
                var registrationEpoch = currentVerifyRunEpoch(s);
                s.verifyMode = 'interactive';
                s.activeVerifySession = persistentShell;
                s._verifySessionRef = persistentShell;

                // Register persistent shell with SessionManager off the JS loop.
                if (typeof tuiMux !== 'undefined' && tuiMux && typeof tuiMux.register === 'function') {
                    try {
                        var registration = tuiMux.register(persistentShell, { kind: 'verify', name: branchName });
                        if (registration && typeof registration.then === 'function') {
                            registration.then(function(id) {
                                if (s._verifyRunEpoch === registrationEpoch &&
                                    (!s.wizard || s.wizard.current !== 'CANCELLED') &&
                                    s.activeVerifySession === persistentShell) {
                                    s.activeVerifySession = id;
                                }
                            }).catch(function(e) {
                                log.debug('runVerifyBranch: tuiMux.register shell failed', { error: e.message || String(e) });
                            });
                        } else if (registration != null && s.activeVerifySession === persistentShell) {
                            s.activeVerifySession = registration;
                        }
                    } catch (e) {
                        log.debug('runVerifyBranch: tuiMux.register shell failed', { error: e.message || String(e) });
                    }
                }

                s.activeVerifyWorktree = wt.worktreeDir;
                s.activeVerifyBranch = branchName;
                s.activeVerifyDir = wt.dir;
                s.activeVerifyStartTime = Date.now();
                s.verifyElapsedMs = 0;
                s.verifyScreen = '';
                s.verifyViewportOffset = 0;
                s.verifyAutoScroll = true;
                s.verifyShellExited = false;

                s.verifySignal = false;
                s.verifySignalChoice = null;
                s.verifySignalBranch = branchName;

                s.verifyHint = scopedCmd || prSplit.runtime.verifyCommand || '';

                return [s, tea.tick(C.TICK_INTERVAL_MS, 'verify-poll')];
            }

            // Shell spawn failed — clean up worktree and fall through to
            // one-shot path below.
            try {
                var gitExecFn = prSplit._gitExec;
                if (gitExecFn) gitExecFn(wt.dir, ['worktree', 'remove', '--force', wt.worktreeDir]);
            } catch (e) {
                log.debug('runVerifyBranch: worktree cleanup failed', { error: e.message || String(e) });
            }
            // Fall through to one-shot path.
        }

        // --- One-shot path (degraded fallback): worktree + CaptureSession ---
        var sessionSetup = pendingVerifySetup(s, 'oneshot', branchName);
        if (sessionSetup && sessionSetup.pending) {
            return [s, tea.tick(10, 'verify-branch')];
        }
        var sessionResult = sessionSetup ? sessionSetup.result : prSplit.startVerifySession(branchName, {
            dir: dir,
            verifyCommand: scopedCmd,
            timeoutMs: effectiveVerifyTimeoutMs(
                (typeof prSplitConfig !== 'undefined') ? prSplitConfig.timeoutMs : 0
            ),
            rows: C.DEFAULT_ROWS,
            cols: Math.max(80, (s.width || 80) - 8)
        });
        if (isThenable(sessionResult)) {
            deferVerifySetup(s, 'oneshot', branchName, sessionResult);
            return [s, tea.tick(10, 'verify-branch')];
        }

        if (sessionResult && sessionResult.skipped) {
            s.verificationResults.push({
                name: branchName,
                status: prSplit._branchStatuses.SKIPPED,
                passed: true,
                skipped: true,
                error: null,
                output: '',
                duration: 0,
                preExisting: false
            });
            s.verifyingIdx++;
            return [s, tea.tick(1, 'verify-branch')];
        }

        if (!sessionResult || (sessionResult.error && !sessionResult.session)) {
            // CaptureSession failed — use async text-only fallback.
            s.verifyMode = 'textonly';
            s.verifyFallbackRunning = true;
            s.verifyFallbackError = null;

            s.activeVerifyBranch = branchName;
            s.activeVerifyStartTime = Date.now();
            s.verifyElapsedMs = 0;
            s.verifyScreen = '';
            s.verifyAutoScroll = true;
            s.verifyViewportOffset = 0;
            s.verifyOutput[branchName] = [];

            // T325+T380: Auto-open split-view with Verify tab in fallback path.
            if (!s.splitViewEnabled && s.height >= C.INLINE_VIEW_HEIGHT) {
                s.splitViewEnabled = true;
                s.splitViewFocus = 'agent';
                s.splitViewTab = 'verify';
                if (typeof prSplit._syncMainViewport === 'function') {
                    prSplit._syncMainViewport(s);
                }
            } else if (s.splitViewEnabled) {
                s.splitViewTab = 'verify';
            }

            var timeoutMs = effectiveVerifyTimeoutMs(
                (typeof prSplitConfig !== 'undefined') ? prSplitConfig.timeoutMs : 0
            );
            var fallbackEpoch = currentVerifyRunEpoch(s);
            runVerifyFallbackAsync(s, branchName, dir, scopedCmd, timeoutMs).then(
                function() {
                    if (s._verifyRunEpoch === fallbackEpoch && s.activeVerifyBranch === branchName) {
                        s.verifyFallbackRunning = false;
                    }
                },
                function(err) {
                    if (s._verifyRunEpoch !== fallbackEpoch || s.activeVerifyBranch !== branchName) return;
                    s.verifyFallbackRunning = false;
                    s.verifyFallbackError = (err && err.message) ? err.message : String(err);
                }
            );
            return [s, tea.tick(C.TICK_INTERVAL_MS, 'verify-fallback-poll')];
        }

        // One-shot CaptureSession started successfully.
        // Register with SessionManager async (fire-and-forget state update
        // so the verify poll picks up the numeric SessionID on the next tick).
        s.verifyMode = 'oneshot';
        s.activeVerifySession = sessionResult.session;
        s._verifySessionRef = sessionResult.session;

        var oneShotRegistrationEpoch = currentVerifyRunEpoch(s);
        if (typeof tuiMux !== 'undefined' && tuiMux && typeof tuiMux.register === 'function') {
            try {
                var registration = tuiMux.register(sessionResult.session, { kind: 'verify', name: branchName });
                if (registration && typeof registration.then === 'function') {
                    registration.then(function(id) {
                        if (s._verifyRunEpoch === oneShotRegistrationEpoch &&
                            (!s.wizard || s.wizard.current !== 'CANCELLED') &&
                            s.activeVerifySession === sessionResult.session) {
                            s.activeVerifySession = id;
                        }
                    }).catch(function(e) {
                        log.debug('runVerifyBranch: tuiMux.register oneshot failed', { error: e.message || String(e) });
                    });
                } else if (registration != null) {
                    s.activeVerifySession = registration;
                }
            } catch (e) {
                log.debug('runVerifyBranch: tuiMux.register oneshot failed', { error: e.message || String(e) });
            }
        }
        s.activeVerifyWorktree = sessionResult.worktreeDir;
        s.activeVerifyBranch = branchName;
        s.activeVerifyDir = sessionResult.dir;
        s.activeVerifyStartTime = sessionResult.startTime;
        var oneShotTimeoutMs = effectiveVerifyTimeoutMs(
            (typeof prSplitConfig !== 'undefined') ? prSplitConfig.timeoutMs : 0
        );
        s.verifyDeadline = oneShotTimeoutMs > 0 ? s.activeVerifyStartTime + oneShotTimeoutMs : 0;
        s.verifyElapsedMs = 0;
        s.verifyScreen = '';
        s.verifyViewportOffset = 0;
        s.verifyAutoScroll = true;

        // T325+T380: Auto-open split-view with Verify tab.
        if (!s.splitViewEnabled && s.height >= C.INLINE_VIEW_HEIGHT) {
            s.splitViewEnabled = true;
            s.splitViewFocus = 'agent';
            s.splitViewTab = 'verify';
            if (typeof prSplit._syncMainViewport === 'function') {
                prSplit._syncMainViewport(s);
            }
        } else if (s.splitViewEnabled) {
            s.splitViewTab = 'verify';
        }

        return [s, tea.tick(C.TICK_INTERVAL_MS, 'verify-poll')];
    }

        // --- Live verification poll ---
        // Polls the active verify session according to the explicit mode selected
        // in runVerifyBranch:
        //   - interactive: canonical persistent shell, user decides p/f/c
        //   - oneshot: degraded CaptureSession fallback, command exit decides
        //   - textonly: handled by handleVerifyFallbackPoll, not here
        function pollVerifySession(s) {
        var advanceState = advanceVerifyIfReady(s);
        if (advanceState === 'advanced') return [s, tea.tick(1, 'verify-branch')];
        if (advanceState !== 'advanced' && paneCleanupExpired(s)) {
            return failVerifyOnStuckTeardown(s);
        }
        if (advanceState === 'waiting' || paneCleanupPending(s)) {
            return [s, tea.tick(nextPaneCleanupDelay(s), 'verify-poll')];
        }
        var activeVerifySession = getInteractivePaneSession(s, 'verify');
        if (!activeVerifySession) return [s, null];
        var verifyMode = getVerifyMode(s, activeVerifySession);

        // T058: Update elapsed time on each tick for live display.
        s.verifyElapsedMs = Date.now() - s.activeVerifyStartTime;

        // T321: Capture ANSI-styled VTerm screen for the Verify tab.
        try { s.verifyScreen = activeVerifySession.screen(); } catch (e) { log.debug('pollVerify: session.screen failed: ' + (e.message || e)); }

        // T350: Auto-scroll main viewport to keep inline terminal visible.
        if (s.vp && s.verifyAutoScroll !== false) {
            try { s.vp.gotoBottom(); } catch (e) { log.debug('pollVerify: viewport.gotoBottom failed: ' + (e.message || e)); }
        }

        // Canonical interactive path: only user signal completes the branch.
        if (verifyMode === 'interactive' &&
            s.verifySignal && s.verifySignalBranch === s.activeVerifyBranch) {
            var branchName = s.activeVerifyBranch;
            var duration = Date.now() - s.activeVerifyStartTime;
            var choice = s.verifySignalChoice;
            var passed = (choice === 'pass');
            var continued = (choice === 'continue'); // skip/continue

            // Capture shell output for expandable display.
            var output = '';
            try { output = activeVerifySession.output(); } catch (e) { /* ignore */ }
            var outputLines = output.split('\n').filter(function(line) { return line.length > 0; });
            s.verifyOutput[branchName] = outputLines;

            // T44: Pipe to Output tab.
            if (s.outputLines) {
                s.outputLines.push('\u2500\u2500\u2500 Verify: ' + branchName + ' (' + choice + ') \u2500\u2500\u2500');
                for (var voi = 0; voi < outputLines.length; voi++) {
                    s.outputLines.push(outputLines[voi]);
                }
                if (s.outputLines.length > C.OUTPUT_BUFFER_CAP) {
                    s.outputLines = s.outputLines.slice(-C.OUTPUT_BUFFER_CAP);
                }
                if (s.outputAutoScroll) {
                    s.outputViewOffset = 0;
                }
            }

            // T115: Detect pre-existing failures.
            var preExisting = (!passed && !continued && _isPreExistingFailure(s));

            s.verificationResults.push({
                name: branchName,
                status: passed ? prSplit._branchStatuses.PASSED
                    : (continued ? prSplit._branchStatuses.SKIPPED : prSplit._branchStatuses.FAILED),
                passed: passed,
                skipped: continued,
                error: passed ? null : (continued ? 'skipped by user' : null),
                output: output,
                duration: duration,
                preExisting: preExisting
            });

            // T380: Preserve display state for post-mortem viewing.
            s._verifyAdvanceAfterCleanup = true;
            clearVerifyPaneSession(s, { debugPrefix: 'verifyDone', keepDisplay: true });

            // T007: Clear user signal state.
            s.verifySignal = false;
            s.verifySignalChoice = null;
            s.verifySignalBranch = null;

            if (advanceVerifyIfReady(s) === 'advanced') {
                return [s, tea.tick(1, 'verify-branch')];
            }
            return [s, tea.tick(1, 'verify-poll')];
        }

        if (verifyMode === 'oneshot' && s.verifyDeadline && Date.now() >= s.verifyDeadline) {
            var timedOutBranch = s.activeVerifyBranch;
            var timeoutState = {
                branch: timedOutBranch,
                epoch: s._verifyRunEpoch,
                done: false
            };
            s._verifyTimeoutKill = timeoutState;
            s._verifyTimeoutKillPending = true;
            var finishTimeout = function() {
                if (timeoutState.done) return;
                timeoutState.done = true;
                if (s._verifyTimeoutKill !== timeoutState) return;
                s._verifyTimeoutKill = null;
                s._verifyTimeoutKillPending = false;
                if (s._verifyRunEpoch !== timeoutState.epoch ||
                    s._verifyPaneCleanupPending ||
                    s.activeVerifyBranch !== timedOutBranch ||
                    (s.wizard && s.wizard.current === 'CANCELLED')) {
                    return;
                }
                var timedOutOutput = '';
                try { timedOutOutput = activeVerifySession.output(); } catch (e) { /* best effort */ }
                if (s.outputLines && timedOutOutput) s.outputLines.push('Verify timed out: ' + timedOutBranch);
                s.verificationResults.push({
                    name: timedOutBranch,
                    status: prSplit._branchStatuses.FAILED,
                    passed: false,
                    skipped: false,
                    error: 'verify timeout after ' + s.verifyElapsedMs + 'ms',
                    output: timedOutOutput,
                    duration: s.verifyElapsedMs,
                    preExisting: false
                });
                s._verifyAdvanceAfterCleanup = true;
                s.verifyDeadline = 0;
                clearVerifyPaneSession(s, { debugPrefix: 'verifyTimeout', keepDisplay: true });
            };
            var timeoutKill = null;
            try {
                timeoutKill = activeVerifySession.kill();
            } catch (e) {
                log.debug('pollVerify: one-shot timeout kill failed', { error: e.message || String(e) });
            }
            if (isThenable(timeoutKill)) {
                var onTimeoutKillFailure = function(e) {
                    log.debug('pollVerify: one-shot timeout kill failed', { error: e.message || String(e) });
                    finishTimeout();
                };
                prSplit._trackPaneOutcome(s, timeoutKill, finishTimeout, onTimeoutKillFailure);
            } else {
                finishTimeout();
            }
            if (advanceVerifyIfReady(s) === 'advanced') {
                return [s, tea.tick(1, 'verify-branch')];
            }
            return [s, tea.tick(C.TICK_INTERVAL_MS, 'verify-poll')];
        }

        var shellExited = false;
        try { shellExited = activeVerifySession.isDone(); } catch (e) { shellExited = true; }

        if (shellExited) {
            if (verifyMode === 'interactive') {
                s.verifyShellExited = true;
                s.spinnerFrame = (s.spinnerFrame || 0) + 1;
                return [s, tea.tick(C.TICK_INTERVAL_MS, 'verify-poll')];
            }

            // Degraded one-shot path: command exit decides the branch result.
            var exitCode = 0;
            try {
                if (typeof activeVerifySession.exitCode === 'function') {
                    exitCode = activeVerifySession.exitCode();
                }
            } catch (e) { /* use default 0 */ }

            var branchName = s.activeVerifyBranch;
            var duration = Date.now() - s.activeVerifyStartTime;
            var passed = (exitCode === 0);
            var output = '';
            try { output = activeVerifySession.output(); } catch (e) { /* ignore */ }
            var outputLines = output.split('\n').filter(function(line) { return line.length > 0; });
            s.verifyOutput[branchName] = outputLines;

            // T44: Pipe to Output tab.
            if (s.outputLines) {
                s.outputLines.push('\u2500\u2500\u2500 Verify: ' + branchName + ' (exit ' + exitCode + ') \u2500\u2500\u2500');
                for (var voi = 0; voi < outputLines.length; voi++) {
                    s.outputLines.push(outputLines[voi]);
                }
                if (s.outputLines.length > C.OUTPUT_BUFFER_CAP) {
                    s.outputLines = s.outputLines.slice(-C.OUTPUT_BUFFER_CAP);
                }
                if (s.outputAutoScroll) {
                    s.outputViewOffset = 0;
                }
            }

            // T115: Detect pre-existing failures.
            var preExisting = (!passed && _isPreExistingFailure(s));
            var errorMsg = passed
                ? null
                : ((preExisting)
                    ? _preExistingAnnotation(s)
                    : ('verification command failed (exit ' + exitCode + ')'));

            s.verificationResults.push({
                name: branchName,
                status: passed ? prSplit._branchStatuses.PASSED : prSplit._branchStatuses.FAILED,
                passed: passed,
                skipped: false,
                error: errorMsg,
                output: output,
                duration: duration,
                preExisting: preExisting
            });

            // T380: Preserve display state for post-mortem viewing.
            s._verifyAdvanceAfterCleanup = true;
            clearVerifyPaneSession(s, { debugPrefix: 'verifyDone', keepDisplay: true });

            if (advanceVerifyIfReady(s) === 'advanced') {
                return [s, tea.tick(1, 'verify-branch')];
            }
            return [s, tea.tick(1, 'verify-poll')];
        }

        // Still running — schedule next poll.
        s.spinnerFrame = (s.spinnerFrame || 0) + 1;
        return [s, tea.tick(C.TICK_INTERVAL_MS, 'verify-poll')];
    }

    // --- Async verify fallback (when CaptureSession unavailable) ---
    // Uses verifySplitAsync for non-blocking verification. The result
    // is stored directly on s so the poll handler can consume it.
    async function runVerifyFallbackAsync(s, branchName, dir, scopedCmd, timeoutMs) {
        var runEpoch = currentVerifyRunEpoch(s);
        var fallbackCurrent = function() {
            return s._verifyRunEpoch === runEpoch &&
                s.isProcessing &&
                s.activeVerifyBranch === branchName &&
                (!s.wizard || s.wizard.current !== 'CANCELLED');
        };
        if (!fallbackCurrent()) return;

        // T352: Use the pre-initialized array on state so the poll handler
        // can read accumulated output for live display.
        var outputLines = s.verifyOutput[branchName] || [];
        if (!s.verifyOutput[branchName]) s.verifyOutput[branchName] = outputLines;
        var branchStart = Date.now();
        var verifyResult = await prSplit.verifySplitAsync(branchName, {
            dir: dir,
            verifyCommand: scopedCmd,
            verifyTimeoutMs: timeoutMs,
            outputFn: function(line) {
                if (!fallbackCurrent()) return;
                outputLines.push(line);
                // T352: Populate verifyScreen with latest fallback output so
                // the inline terminal and Verify tab show live output.
                var rows = Math.min(C.DEFAULT_ROWS, outputLines.length);
                s.verifyScreen = outputLines.slice(-rows).join('\n');
            }
        });
        var duration = Date.now() - branchStart;

        if (!fallbackCurrent()) return;

        s.verifyOutput[branchName] = outputLines;

        // T115: Detect pre-existing failures using cached baseline result.
        // verifySplitAsync (singular) never sets preExisting — only the batch
        // verifySplitsAsync does, so we must check the baseline cache here.
        var preExisting = false;
        var errorMsg = verifyResult.error || null;
        if (!verifyResult.passed && !verifyResult.skipped && _isPreExistingFailure(s)) {
            preExisting = true;
            if (errorMsg) {
                errorMsg += _preExistingAnnotation(s);
            }
        }

        s.verificationResults.push({
            name: branchName,
            status: verifyResult.skipped ? prSplit._branchStatuses.SKIPPED
                : (verifyResult.passed ? prSplit._branchStatuses.PASSED : prSplit._branchStatuses.FAILED),
            passed: verifyResult.passed,
            skipped: verifyResult.skipped || false,
            error: errorMsg,
            output: verifyResult.output || '',
            duration: duration,
            preExisting: preExisting
        });

        s.verifyingIdx++;
    }

    // handleVerifyFallbackPoll: Called every 100ms during async fallback
    // verification. When complete, continues to the next branch.
    function handleVerifyFallbackPoll(s) {
        // T352: Update elapsed time for fallback display.
        if (s.activeVerifyStartTime) {
            s.verifyElapsedMs = Date.now() - s.activeVerifyStartTime;
        }

        // T352: Auto-scroll main viewport during fallback verification.
        if (s.vp && s.verifyAutoScroll !== false) {
            try { s.vp.gotoBottom(); } catch (e) { log.debug('fallbackPoll: viewport.gotoBottom failed: ' + (e.message || e)); }
        }

        // Still running — keep polling.
        if (s.verifyFallbackRunning) {
            s.spinnerFrame = (s.spinnerFrame || 0) + 1;
            return [s, tea.tick(C.TICK_INTERVAL_MS, 'verify-fallback-poll')];
        }

        // T380: Preserve activeVerifyBranch, verifyElapsedMs, and verifyScreen
        // for post-mortem viewing in fallback path (consistent with CaptureSession path).
        s.activeVerifyStartTime = 0;
        s.verifyAutoScroll = true;
        s.verifyViewportOffset = 0;
        // T380: Keep verify tab visible for post-mortem review.

        // Error in the .then rejection handler — record a failure result.
        if (s.verifyFallbackError) {
            var branchName = (st.planCache && st.planCache.splits &&
                s.verifyingIdx < st.planCache.splits.length)
                ? st.planCache.splits[s.verifyingIdx].name : 'unknown';

            // T115: Detect pre-existing failures using cached baseline result.
            var preExisting = _isPreExistingFailure(s);
            var fallbackError = s.verifyFallbackError;
            if (preExisting) {
                fallbackError += _preExistingAnnotation(s);
            }

            s.verificationResults.push({
                name: branchName,
                status: prSplit._branchStatuses.FAILED,
                passed: false,
                skipped: false,
                error: fallbackError,
                output: '',
                duration: 0,
                preExisting: preExisting
            });
            s.verifyingIdx++;
            s.verifyFallbackError = null;
        }

        // Completed — advance to next branch.
        return [s, tea.tick(1, 'verify-branch')];
    }

    // --- Agent Conversation (T16) ---

    // ensureMCPCallback creates the MCP callback transport for Agent
    // IPC if one does not already exist. Called on-demand by
    // openAgentConvo so that "Ask Agent" works regardless of whether
    // the automated pipeline was ever started.
    async function ensureMCPCallback() {
        if (prSplit._mcpCallbackObj) {
            return { error: null };
        }
        try {
            var mcpMod = require('osm:mcp');
            var MCPCallbackMod = require('osm:mcpcallback');
            var srv = mcpMod.createServer('pr-split-callback', '1.0.0');
            var mcpCallbackObj = MCPCallbackMod.MCPCallback({ server: srv });

            mcpCallbackObj.addTool('reportSplitPlan',
                'Report a split plan for PR splitting following Commit-Loom discipline.',
                {
                    type: 'object',
                    properties: {
                        stages: {
                            type: 'array',
                            items: {
                                type: 'object',
                                properties: {
                                    name: { type: 'string', description: 'Branch name suffix' },
                                    files: { type: 'array', items: { type: 'string' } },
                                    message: { type: 'string', description: 'Commit message' },
                                    order: { type: 'number', description: 'Execution order' },
                                    title: { type: 'string', description: 'Imperative PR title' },
                                    summary: { type: 'string', description: 'Architectural summary of this PR layer' },
                                    keyChanges: { type: 'array', items: { type: 'string' }, description: 'Key changes / bullet points' },
                                    verificationSteps: { type: 'string', description: 'Commands to verify this layer in isolation' },
                                    rationale: { type: 'string', description: 'Why this PR is self-contained and layering rationale' }
                                },
                                required: ['name', 'files']
                            }
                        }
                    },
                    required: ['stages']
                });

            mcpCallbackObj.addTool('reportResolution',
                'Report conflict resolution for a PR split branch.',
                {
                    type: 'object',
                    properties: {
                        patches: { type: 'array', items: { type: 'object', properties: { file: { type: 'string' }, content: { type: 'string' } } } },
                        commands: { type: 'array', items: { type: 'string' } },
                        preExistingFailure: { type: 'boolean' },
                        preExistingDetails: { type: 'string' },
                        reSplitSuggested: { type: 'boolean' },
                        reSplitReason: { type: 'string' }
                    }
                });

            mcpCallbackObj.addTool('heartbeat',
                'Send a heartbeat to indicate Agent is still actively working.',
                {
                    type: 'object',
                    properties: {
                        status: { type: 'string', description: 'Optional status message' }
                    }
                });

            await mcpCallbackObj.init();
            prSplit._mcpCallbackObj = mcpCallbackObj;
            log.printf('on-demand MCP callback initialized at %s', mcpCallbackObj.address || '(unknown)');
            return { error: null };
        } catch (e) {
            return { error: 'MCP callback setup failed: ' + (e.message || String(e)) };
        }
    }

    // spawnAgentOnDemand creates the MCP callback transport and spawns
    // a Agent executor when "Ask Agent" is used outside the automated
    // pipeline. Updates s.agentOnDemandSpawning and s.agentConvo
    // state for tick-based progress rendering.
    async function spawnAgentOnDemand(s) {
        try {
            s.agentConvo.spawnProgress = 'Setting up MCP transport\u2026';

            var mcpResult = await ensureMCPCallback();
            if (mcpResult.error) {
                s.agentConvo.lastError = mcpResult.error;
                s.agentOnDemandSpawning = false;
                return;
            }

            // Create executor if needed (T42 may have already created one).
            var executor = st.agentExecutor;
            if (!executor) {
                if (typeof prSplitConfig === 'undefined') {
                    s.agentConvo.lastError = 'Configuration not available.';
                    s.agentOnDemandSpawning = false;
                    return;
                }
                executor = new (prSplit.AgentCodeExecutor)(prSplitConfig);
                st.agentExecutor = executor;
            }

            // Resolve binary path if not already resolved.
            if (!executor.resolved) {
                s.agentConvo.spawnProgress = 'Resolving Agent binary\u2026';
                var resolveResult = await executor.resolveAsync(function(msg) {
                    s.agentConvo.spawnProgress = msg;
                });
                if (resolveResult.error) {
                    s.agentConvo.lastError = resolveResult.error;
                    s.agentOnDemandSpawning = false;
                    return;
                }
            }

            // Spawn the Agent process.
            s.agentConvo.spawnProgress = 'Starting Agent process\u2026';
            var mcpCb = prSplit._mcpCallbackObj;
            var spawnResult = await executor.spawn(null, {
                mcpConfigPath: mcpCb.mcpConfigPath
            });
            if (spawnResult && spawnResult.error) {
                s.agentConvo.lastError = spawnResult.error;
                s.agentOnDemandSpawning = false;
                return;
            }

            // Success — clear error and progress state.
            s.agentConvo.lastError = null;
            s.agentConvo.spawnProgress = null;
            s.agentOnDemandSpawning = false;
            log.printf('on-demand Agent spawn succeeded (session=%s)', spawnResult.sessionId || '');
        } catch (e) {
            s.agentConvo.lastError = 'Agent spawn error: ' + (e.message || String(e));
            s.agentOnDemandSpawning = false;
        }
    }

    // openAgentConvo opens the conversation overlay.
    function openAgentConvo(s, context) {
        // Check Agent availability.
        var executor = st.agentExecutor;

        // Happy path: executor alive — open conversation immediately.
        if (executor && executor.handle &&
            (typeof executor.handle.isAlive !== 'function' || executor.handle.isAlive())) {
            s.agentConvo.active = true;
            s.agentConvo.context = context;
            s.agentConvo.inputText = '';
            s.agentConvo.lastError = null;
            s.agentConvo.scrollOffset = 0;
            return [s, null];
        }

        // Dead handle — report and allow retry.
        if (executor && executor.handle &&
            typeof executor.handle.isAlive === 'function' && !executor.handle.isAlive()) {
            s.agentConvo.lastError = 'Agent process has exited. Restart analysis to reconnect.';
            s.agentConvo.active = true;
            s.agentConvo.context = context;
            return [s, null];
        }

        // No executor or no handle — try on-demand spawn.
        // Gate: Agent binary was already checked and is unavailable.
        if (s.agentCheckStatus === 'unavailable') {
            s.agentConvo.lastError = 'No agent command configured. Set --agent-command to the agent CLI executable.';
            s.agentConvo.active = true;
            s.agentConvo.context = context;
            return [s, null];
        }

        // Already spawning — don't double-launch, but re-open the overlay
        // if the user requested it (e.g. closed with Escape, then re-clicked).
        if (s.agentOnDemandSpawning) {
            s.agentConvo.active = true;
            s.agentConvo.context = context;
            return [s, null];
        }

        // Start on-demand spawn.
        s.agentConvo.active = true;
        s.agentConvo.context = context;
        s.agentConvo.inputText = '';
        s.agentConvo.lastError = null;
        s.agentConvo.scrollOffset = 0;
        s.agentOnDemandSpawning = true;
        s.agentConvo.spawnProgress = 'Connecting to Agent\u2026';

        spawnAgentOnDemand(s);

        return [s, tea.tick(C.AGENT_CHECK_POLL_MS, 'agent-spawn-poll')];
    }

    // closeAgentConvo dismisses the conversation overlay.
    function closeAgentConvo(s) {
        s.agentConvo.active = false;
        s.agentConvo.inputText = '';
        s.agentConvo.scrollOffset = 0;
        s.agentConvo.spawnProgress = null;
        // Note: agentOnDemandSpawning is intentionally NOT cleared here.
        // The async spawn continues in the background so the session is
        // ready when the user re-opens the overlay.
        // Keep history and context for re-opening.
        return [s, null];
    }

    // updateAgentConvo handles input while conversation overlay is active.
    function updateAgentConvo(msg, s) {
        var convo = s.agentConvo;

        if (msg.type === 'Key') {
            var k = msg.key;

            // Escape closes the overlay.
            if (k === 'esc') {
                return closeAgentConvo(s);
            }

            // Enter submits the current input (if not already sending or spawning).
            if (k === 'enter' && !convo.sending && !s.agentOnDemandSpawning) {
                var text = (convo.inputText || '').trim();
                if (text.length > 0) {
                    return submitAgentMessage(s, text);
                }
                return [s, null];
            }

            // Backspace.
            if (k === 'backspace' && !convo.sending && !s.agentOnDemandSpawning) {
                var t = convo.inputText || '';
                if (t.length > 0) {
                    convo.inputText = t.substring(0, t.length - 1);
                }
                return [s, null];
            }

            // Ctrl+U: clear input line.
            if (k === 'ctrl+u' && !convo.sending && !s.agentOnDemandSpawning) {
                convo.inputText = '';
                return [s, null];
            }

            // Scroll history: up/pgup to scroll back.
            if (k === 'up' || k === 'pgup') {
                convo.scrollOffset = (convo.scrollOffset || 0) + 3;
                return [s, null];
            }
            if (k === 'down' || k === 'pgdown') {
                convo.scrollOffset = Math.max(0, (convo.scrollOffset || 0) - 3);
                return [s, null];
            }

            // Single printable char — accumulate (when not sending or spawning).
            if (k.length === 1 && !convo.sending && !s.agentOnDemandSpawning) {
                convo.inputText = (convo.inputText || '') + k;
                return [s, null];
            }

            return [s, null];
        }

        // Mouse wheel scrolls history.
        if (msg.type === 'MouseWheel') {
            if (msg.button === 'wheel up') {
                convo.scrollOffset = (convo.scrollOffset || 0) + 3;
                return [s, null];
            }
            if (msg.button === 'wheel down') {
                convo.scrollOffset = Math.max(0, (convo.scrollOffset || 0) - 3);
                return [s, null];
            }
        }

        // Clicking outside the overlay area could close it (but since this is
        // a full overlay interceptor, just consume to prevent leakage).
        return [s, null];
    }

    // buildAgentPrompt constructs the prompt based on conversation context.
    function buildAgentPrompt(context, userMessage, s) {
        var parts = [];

        if (context === 'plan-review') {
            parts.push('The user is reviewing the current PR split plan and has feedback.');
            if (st.planCache) {
                parts.push('Current plan has ' + st.planCache.splits.length + ' splits:');
                for (var i = 0; i < st.planCache.splits.length; i++) {
                    var sp = st.planCache.splits[i];
                    parts.push('  ' + (i + 1) + '. ' + (sp.name || 'split-' + i) +
                        ' (' + sp.files.length + ' files)');
                }
            }
            parts.push('');
            parts.push('User feedback: ' + userMessage);
            parts.push('');
            parts.push('Please call the reportSplitPlan tool with your revised split plan based on this feedback.');
        } else if (context === 'error-resolution') {
            parts.push('An error occurred during PR split execution. The user needs help resolving it.');
            if (s.errorDetails) {
                parts.push('Error: ' + s.errorDetails);
            }
            // Include failed branch context if available.
            if (s.manualFixContext && s.manualFixContext.failedBranches) {
                parts.push('Failed branches: ' + s.manualFixContext.failedBranches.join(', '));
            }
            parts.push('');
            parts.push('User message: ' + userMessage);
            parts.push('');
            parts.push('Please call the reportResolution tool with your suggested fix.');
        } else {
            // Generic conversation.
            parts.push(userMessage);
        }

        return parts.join('\n');
    }

    // submitAgentMessage launches the async send + wait operation.
    function submitAgentMessage(s, text) {
        var convo = s.agentConvo;
        var executor = st.agentExecutor;

        if (!executor || !executor.handle) {
            convo.lastError = 'Agent is not running.';
            return [s, null];
        }

        // Add user message to history.
        convo.history.push({ role: 'user', text: text, ts: Date.now() });
        convo.inputText = '';
        convo.sending = true;
        convo.lastError = null;
        convo.scrollOffset = 0; // scroll to bottom

        // Determine MCP tool to wait for based on context.
        var toolToWait = null;
        var timeoutMs = C.CONVO_TIMEOUT_MS; // 2 minutes default
        if (convo.context === 'plan-review') {
            toolToWait = 'reportSplitPlan';
            timeoutMs = C.PLAN_REVISION_TIMEOUT_MS; // 3 minutes for plan revision
        } else if (convo.context === 'error-resolution') {
            toolToWait = 'reportResolution';
            timeoutMs = C.CONVO_TIMEOUT_MS;
        }
        convo.waitingForTool = toolToWait;

        // Build the prompt.
        var prompt = buildAgentPrompt(convo.context, text, s);

        // Launch async operation: sendToHandle → waitForLogged.
        // Use same pattern as automatedSplit: Promise + tick polling.
        var convoState = convo; // capture reference for closure
        prSplit.sendToHandle(executor.handle, prompt).then(
            async function(sendResult) {
                if (sendResult && sendResult.error) {
                    convoState.lastError = 'Send failed: ' + sendResult.error;
                    convoState.sending = false;
                    convoState.waitingForTool = null;
                    return;
                }

                if (toolToWait) {
                    // Wait for Agent to call the expected MCP tool.
                    var waitResult = await prSplit.waitForLogged(toolToWait, timeoutMs, {
                        aliveCheck: function() {
                            return (executor.handle &&
                                typeof executor.handle.isAlive === 'function' &&
                                executor.handle.isAlive());
                        }
                    });
                    if (waitResult && waitResult.data) {
                        convoState.history.push({
                            role: 'agent',
                            text: formatAgentResponse(toolToWait, waitResult.data),
                            ts: Date.now()
                        });
                        // Process structured response.
                        processAgentConvoResult(convoState, toolToWait, waitResult.data);
                    } else if (waitResult && waitResult.error) {
                        convoState.lastError = 'Agent: ' + waitResult.error;
                    } else {
                        // No tool call received — Agent may have responded in free text.
                        // Capture the latest pinned Agent snapshot as response.
                        var shot = null;
                        if (typeof prSplit._readAgentPlainText === 'function') {
                            try {
                                shot = prSplit._readAgentPlainText();
                            } catch (e) {
                                log.debug('screenCapture: pinnedAgentSnapshot failed', { error: e.message || String(e) });
                            }
                        }
                        if (shot) {
                            convoState.history.push({
                                role: 'agent',
                                text: '[screenshot]\n' + shot.substring(shot.length - C.SCREENSHOT_CAPTURE_CHARS),
                                ts: Date.now()
                            });
                        }
                    }
                } else {
                    // No specific tool to wait for — just note the send succeeded.
                    convoState.history.push({
                        role: 'agent',
                        text: '(message sent — check Agent pane for response)',
                        ts: Date.now()
                    });
                }
                convoState.sending = false;
                convoState.waitingForTool = null;
            },
            function(err) {
                convoState.lastError = (err && err.message) ? err.message : String(err);
                convoState.sending = false;
                convoState.waitingForTool = null;
            }
        );

        // Start tick polling for completion.
        return [s, tea.tick(C.CONVO_POLL_MS, 'agent-convo-poll')];
    }

    // formatAgentResponse formats structured MCP tool response for display.
    function formatAgentResponse(toolName, data) {
        // T122: MCP schema uses 'stages'; accept both field names.
        var splits = (toolName === 'reportSplitPlan' && data) ? (data.stages || data.splits) : null;
        if (splits && splits.length > 0) {
            var parts = ['Revised plan (' + splits.length + ' splits):'];
            for (var i = 0; i < splits.length; i++) {
                var sp = splits[i];
                parts.push('  ' + (i + 1) + '. ' + (sp.name || 'split-' + i) +
                    ' (' + (sp.files ? sp.files.length : 0) + ' files)');
            }
            return parts.join('\n');
        }
        if (toolName === 'reportResolution' && data) {
            return 'Resolution: ' + (data.description || data.action || JSON.stringify(data));
        }
        return JSON.stringify(data, null, 2);
    }

    // processAgentConvoResult applies structured result to wizard state.
    function processAgentConvoResult(convo, toolName, data) {
        // T122: MCP schema uses 'stages'; accept both field names.
        var splits = (toolName === 'reportSplitPlan' && data) ? (data.stages || data.splits) : null;
        if (splits) {
            // Update the plan cache with the revised plan from Agent.
            if (st.planCache) {
                st.planCache.splits = splits;
                if (data.baseBranch) {
                    st.planCache.baseBranch = data.baseBranch;
                }
                // Mark that plan was revised so TUI can reset selectedSplitIdx.
                st.planRevised = true;
            }
        }
        // reportResolution results are handled by the existing error resolution flow.
        // The user can manually apply the suggestion or use auto-resolve.
    }

    // pollAgentConvo checks async send/wait progress.
    function pollAgentConvo(s) {
        var convo = s.agentConvo;

        // If still sending, keep polling.
        if (convo.sending) {
            return [s, tea.tick(C.CONVO_POLL_MS, 'agent-convo-poll')];
        }

        // T122: If plan was revised by Agent, reset split selection.
        if (st.planRevised) {
            st.planRevised = false;
            s.selectedSplitIdx = 0;
        }

        // Async operation completed. UI will update on next render.
        return [s, null];
    }

    // --- Error Resolution (T16) ---
    function handleErrorResolutionChoice(s, choice) {
        // Crash-recovery choices bypass the wizard state machine entirely
        // because handleErrorResolutionState treats unknown choices as
        // 'abort' and calls wizard.cancel(). Instead, crash-recovery
        // resets the wizard to a resumable state (PLAN_GENERATION) so
        // startAnalysis can take over.
        if (choice === 'restart-agent') {
            var executor = st.agentExecutor;
            if (!executor) {
                s.errorDetails = 'No Agent executor available for restart.';
                return [s, null];
            }
            var restartOpts = {};
            if (prSplit._mcpCallbackObj && prSplit._mcpCallbackObj.mcpConfigPath) {
                restartOpts.mcpConfigPath = prSplit._mcpCallbackObj.mcpConfigPath;
            }
            // Non-blocking restart: launch as Promise, poll via tick.
            s.agentRestarting = true;
            s.restartResult = null;
            s.errorDetails = 'Restarting Agent...';
            executor.restart(null, restartOpts).then(function(restartResult) {
                s.agentRestarting = false;
                s.restartResult = restartResult;
            }, function(err) {
                s.agentRestarting = false;
                s.restartResult = { error: 'Agent restart error: ' + ((err && err.message) || String(err)) };
            });
            return [s, tea.tick(C.AUTO_SPLIT_POLL_MS, 'restart-agent-poll')];
        }

        if (choice === 'fallback-heuristic') {
            s.agentCrashDetected = false;
            prSplit.runtime.mode = 'heuristic';
            // Reset verification phase — restarting from plan generation.
            prSplit._resetVerifyPhase(s);
            // Reset wizard to PLAN_GENERATION so startAnalysis picks up.
            s.wizard.transition('PLAN_GENERATION');
            s.wizardState = 'PLAN_GENERATION';
            return startAnalysis(s);
        }

        var result = handleErrorResolutionState(s.wizard, choice);
        s.wizardState = s.wizard.current;

        if (result && result.error) {
            s.errorDetails = result.error;
            return [s, null];
        }

        switch (choice) {
        case 'auto-resolve':
            // Dispatch resolveConflicts as async Promise with tick polling
            // (same pattern as startAutoAnalysis).
            s.isProcessing = true;
            s.resolveRunning = true;
            s.resolveResult = null;

            var resolveOpts = {
                verifyCommand: prSplit.runtime.verifyCommand,
                verifyTimeoutMs: effectiveVerifyTimeoutMs(
                    (typeof prSplitConfig !== 'undefined') ? prSplitConfig.timeoutMs : 0
                ),
                retryBudget: prSplit.runtime.retryBudget
            };
            prSplit.resolveConflicts(st.planCache, resolveOpts).then(
                function(res) {
                    s.resolveResult = res;
                    s.resolveRunning = false;
                },
                function(err) {
                    s.resolveResult = { error: (err && err.message) ? err.message : String(err) };
                    s.resolveRunning = false;
                }
            );
            return [s, tea.tick(C.RESOLVE_POLL_MS, 'resolve-poll')];

        case 'manual':
            // Switch to Agent pane — user fixes manually. Store context
            // so the execution screen can show instructions when user
            // returns from Agent.
            s.manualFixContext = {
                failedBranches: (result && result.failedBranches) ||
                    (s.wizard.data && s.wizard.data.failedBranches) || []
            };
            // Task 5: Use pinned Agent SessionID proxy for passthrough.
            var agentPaneSession = getInteractivePaneSession(s, 'agent');
            if (agentPaneSession && typeof agentPaneSession.passthrough === 'function' &&
                typeof agentPaneSession.isRunning === 'function' &&
                agentPaneSession.isRunning()) {
                (async function() { await agentPaneSession.passthrough(); })();
            }
            return [s, null];

        case 'skip':
            // Transition to EQUIV_CHECK happened in handleErrorResolutionState.
            // Reset verify phase (enterErrorState set it to ERROR) then move to equiv.
            prSplit._resetVerifyPhase(s);
            prSplit._transitionVerifyPhase(s, prSplit._verifyPhases.EQUIV_CHECK);
            s.isProcessing = true;
            return startEquivCheck(s);

        case 'retry':
            // Transition to PLAN_GENERATION happened. Re-run analysis.
            prSplit._resetVerifyPhase(s);
            return startAnalysis(s);

        case 'abort':
            // Transition to CANCELLED happened. Quit the wizard.
            return [s, tea.quit()];

        default:
            return [s, null];
        }
    }

    // --- handleVerifySignal — record user PASS/FAIL/CONTINUE from keyboard shortcut ---
    // Called by pr_split_16e_tui_update.js when the user presses p/f/c during
    // active verification with a persistent shell.
    // T007 (Task 7): This is the core user-completion signal for the persistent
    // verify shell model. The choice is stored on state and consumed by
    // pollVerifySession on the next tick to record the result and advance.
    function handleVerifySignal(s, choice) {
        var activeVerifySession = getInteractivePaneSession(s, 'verify');
        if (!activeVerifySession || !s.activeVerifyBranch) return [s, null];
        if (getVerifyMode(s, activeVerifySession) !== 'interactive') return [s, null];
        if (!s.verifyShellExited) return [s, null];
        // Ignore if already signaled for this branch.
        if (s.verifySignal && s.verifySignalBranch === s.activeVerifyBranch) return [s, null];

        s.verifySignal = true;
        s.verifySignalChoice = choice; // 'pass' | 'fail' | 'continue'
        s.verifySignalBranch = s.activeVerifyBranch;
        log.debug('verify user signal', {choice: choice, branch: s.activeVerifyBranch});
        return [s, null];
    }

    // Cross-chunk exports.
    prSplit._updateConfirmCancel = updateConfirmCancel;
    prSplit._runVerifyBranch = runVerifyBranch;
    prSplit._pollVerifySession = pollVerifySession;
    // Task 8: _pollShellSession removed — shell tab unified into verify pane.
    prSplit._handleVerifySignal = handleVerifySignal;
    prSplit._handleVerifyFallbackPoll = handleVerifyFallbackPoll;
    prSplit._openAgentConvo = openAgentConvo;
    prSplit._closeAgentConvo = closeAgentConvo;
    prSplit._updateAgentConvo = updateAgentConvo;
    prSplit._pollAgentConvo = pollAgentConvo;
    prSplit._handleErrorResolutionChoice = handleErrorResolutionChoice;
    prSplit._invalidateVerifySetup = invalidateVerifySetup;
    prSplit._disposeVerifySetupResult = disposeVerifySetupResult;

})(globalThis.prSplit);
