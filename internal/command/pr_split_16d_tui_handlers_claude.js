'use strict';
// pr_split_16d_tui_handlers_claude.js — TUI: Claude automation, key byte conversion, question detection, screenshot polling
// Dependencies: chunks 00-16c must be loaded first.
// Requires Go-injected globals: tui, ctx, output, log, prSplitConfig, tuiMux.

(function(prSplit) {

    if (typeof tui === 'undefined' || typeof ctx === 'undefined' ||
        typeof output === 'undefined') { return; }

    // Cross-chunk imports — libraries.
    var tea = prSplit._tea;
    var C = prSplit._TUI_CONSTANTS;

    // Cross-chunk imports — state and handlers from chunks 13-14.
    var st = prSplit._state;
    var handleConfigState = prSplit._handleConfigState;
    var getInteractivePaneSession = prSplit._getInteractivePaneSession;

    // Late-bound cross-chunk references (defined in sibling 16x chunks, resolved at call time).
    function startAnalysis(s) { return prSplit._startAnalysis(s); }
    function startExecution(s) { return prSplit._startExecution(s); }
    function syncMainViewport(s) { return prSplit._syncMainViewport(s); }
    function enterErrorState(s, details) {
        if (typeof prSplit._enterErrorState === 'function') {
            return prSplit._enterErrorState(s, details);
        }
        s.errorDetails = details || s.errorDetails || 'An unexpected error occurred.';
        s.errorFromState = s.errorFromState || (s.wizard && s.wizard.current) || s.wizardState || '';
        try { s.wizard.transition('ERROR'); } catch (te) { log.debug('wizard: transition to ERROR failed: ' + (te.message || te)); }
        s.wizardState = s.wizard.current;
        return [s, null];
    }

    // --- Claude Lifecycle Event Wiring ---
    //
    // Registers EventBus listeners for output/exit/bell/closed filtered by
    // Claude's pinned SessionID. Callbacks set dirty flags and lifecycle
    // markers that pollClaudeScreenshot reads on each tick — no redundant
    // snapshots when nothing changed, and immediate response to exit/bell.

    /**
     * wireClaudeLifecycleEvents registers event handlers for Claude's pinned
     * session. Call once when st.claudeSessionID is first available.
     * Stores handler IDs on st._claudeEventIDs for cleanup.
     */
    function wireClaudeLifecycleEvents() {
        if (typeof tuiMux === 'undefined' || !tuiMux ||
            typeof tuiMux.on !== 'function') {
            return;
        }
        // Guard: don't double-wire.
        if (st._claudeEventIDs && st._claudeEventIDs.length > 0) {
            return;
        }
        var cid = st.claudeSessionID;
        if (!cid) {
            return;
        }

        var ids = [];

        // Output event: set dirty flag + record timestamp for adaptive polling.
        ids.push(tuiMux.on('output', function(data) {
            if (data && data.sessionId === cid) {
                st._claudeOutputDirty = true;
                st._claudeLastOutputMs = Date.now();
            }
        }));

        // Exit event: mark Claude as exited for immediate lifecycle response.
        ids.push(tuiMux.on('exit', function(data) {
            if (data && data.sessionId === cid) {
                st._claudeExitEvent = true;
                log.info('claude lifecycle: exit event received', { sessionId: cid });
            }
        }));

        // Bell event: set flash flag for visual indicator.
        ids.push(tuiMux.on('bell', function(data) {
            if (data && data.sessionId === cid) {
                st._claudeBellFlash = true;
                st._claudeBellFlashAt = Date.now();
                log.debug('claude lifecycle: bell event', { sessionId: cid });
            }
        }));

        // Closed event: session unregistered from SessionManager.
        ids.push(tuiMux.on('closed', function(data) {
            if (data && data.sessionId === cid) {
                st._claudeClosedEvent = true;
                log.info('claude lifecycle: closed event received', { sessionId: cid });
            }
        }));

        st._claudeEventIDs = ids;
        log.debug('claude lifecycle: event handlers registered', {
            sessionId: cid,
            handlerCount: ids.length
        });
    }

    /**
     * unwireClaudeLifecycleEvents removes all registered event handlers
     * and resets lifecycle tracking state.
     */
    function unwireClaudeLifecycleEvents() {
        if (typeof tuiMux === 'undefined' || !tuiMux ||
            typeof tuiMux.off !== 'function') {
            return;
        }
        var ids = st._claudeEventIDs;
        if (ids && ids.length > 0) {
            for (var i = 0; i < ids.length; i++) {
                tuiMux.off(ids[i]);
            }
            log.debug('claude lifecycle: event handlers removed', {
                handlerCount: ids.length
            });
        }
        st._claudeEventIDs = null;
        st._claudeOutputDirty = false;
        st._claudeLastOutputMs = 0;
        st._claudeExitEvent = false;
        st._claudeBellFlash = false;
        st._claudeBellFlashAt = 0;
        st._claudeClosedEvent = false;
        st._claudeLastSnapshotGen = 0;
    }

    /**
     * deriveClaudeLifecycleState computes the user-visible lifecycle state
     * from event flags and session status.
     *
     * States: 'detached' | 'active' | 'idle' | 'waiting' | 'exited' | 'crashed'
     */
    function deriveClaudeLifecycleState(s) {
        var cid = st.claudeSessionID;
        if (!cid) {
            return 'detached';
        }
        // Check exit/closed events first — terminal states.
        if (st._claudeClosedEvent) {
            return 'closed';
        }
        if (st._claudeExitEvent) {
            // Distinguish crash from normal exit: if the pipeline is still
            // running, it's a crash. If the pipeline completed, it's normal.
            return s.autoSplitRunning ? 'crashed' : 'exited';
        }
        // Check if session is done (fallback for missed events).
        if (typeof tuiMux.isDone === 'function' && tuiMux.isDone(cid)) {
            return s.autoSplitRunning ? 'crashed' : 'exited';
        }
        // Question detected → waiting for input.
        if (s.claudeQuestionDetected) {
            return 'waiting';
        }
        // Recent output → active.
        var now = Date.now();
        if (st._claudeLastOutputMs &&
            (now - st._claudeLastOutputMs) < C.CLAUDE_OUTPUT_IDLE_MS) {
            return 'active';
        }
        // No recent output → idle.
        return 'idle';
    }

    // --- Claude Check Handlers ---

    function handleClaudeCheck(s) {
        // Guard: prSplitConfig is injected from Go and may be absent in tests.
        if (typeof prSplitConfig === 'undefined') {
            s.claudeCheckStatus = 'unavailable';
            s.claudeCheckError = 'Configuration not available (test mode)';
            s.claudeResolvedInfo = null;
            return [s, null];
        }

        // Guard: already checking — don't double-launch.
        if (s.claudeCheckRunning) {
            return [s, tea.tick(C.CLAUDE_CHECK_POLL_MS, 'claude-check-poll')];
        }

        // Use cached executor if available (avoids redundant re-checks).
        if (st.claudeExecutor && st.claudeExecutor.resolved) {
            s.claudeCheckStatus = 'available';
            s.claudeResolvedInfo = st.claudeExecutor.resolved;
            s.claudeCheckError = null;
            return [s, null];
        }

        var executor = new (prSplit.ClaudeCodeExecutor)(prSplitConfig);
        s.claudeCheckStatus = 'checking';
        s.claudeCheckRunning = true;
        s.claudeCheckProgressMsg = 'Resolving binary\u2026';

        runClaudeCheckAsync(s, executor).then(
            function() {
                s.claudeCheckRunning = false;
            },
            function(err) {
                s.claudeCheckStatus = 'unavailable';
                s.claudeCheckError = (err && err.message) ? err.message : String(err);
                s.claudeResolvedInfo = null;
                prSplit.runtime.mode = 'heuristic';
                s.claudeCheckRunning = false;
            }
        );

        // Poll at 50ms for responsive status updates.
        return [s, tea.tick(C.CLAUDE_CHECK_POLL_MS, 'claude-check-poll')];
    }

    // runClaudeCheckAsync: Async function that runs resolveAsync on the
    // executor. Updates s.claudeCheckProgressMsg for the view.
    async function runClaudeCheckAsync(s, executor) {
        var result = await executor.resolveAsync(function(msg) {
            s.claudeCheckProgressMsg = msg;
        });

        if (result.error) {
            s.claudeCheckStatus = 'unavailable';
            s.claudeCheckError = result.error;
            s.claudeResolvedInfo = null;
            // Auto-fallback: switch to heuristic so user can proceed.
            prSplit.runtime.mode = 'heuristic';
        } else {
            s.claudeCheckStatus = 'available';
            s.claudeResolvedInfo = executor.resolved; // { command, type }
            s.claudeCheckError = null;
            // Cache the resolved executor for startAutoAnalysis().
            st.claudeExecutor = executor;
            // T42: Auto-select 'auto' strategy when Claude detected on startup,
            // unless the user has already manually selected a different strategy.
            if (!s.userHasSelectedStrategy) {
                prSplit.runtime.mode = 'auto';
            }
        }
    }

    // handleClaudeCheckPoll: Called every 50ms to check if the async
    // Claude check has completed.
    function handleClaudeCheckPoll(s) {
        // Still running — keep polling.
        if (s.claudeCheckRunning) {
            return [s, tea.tick(C.CLAUDE_CHECK_POLL_MS, 'claude-check-poll')];
        }

        // T113: If startAutoAnalysis deferred to us because the executor
        // wasn't resolved yet, dispatch it now that the check is done.
        if (s.pendingAutoAnalysis) {
            s.pendingAutoAnalysis = false;
            if (s.claudeCheckStatus === 'available' && st.claudeExecutor && st.claudeExecutor.resolved) {
                log.printf('auto-analysis: executor resolved — resuming pipeline');
                return startAutoAnalysis(s);
            }
            // Claude unavailable — fall back to heuristic.
            log.printf('auto-analysis: Claude unavailable after async check — falling back');
            return startAnalysis(s);
        }

        // Completed — view will render the final status.
        return [s, null];
    }

    // --- Automated pipeline (Claude) ---
    // The pipeline runs on the JS event loop independently. We poll for
    // completion via ticks so BubbleTea can render progress and the user

    function startAutoAnalysis(s) {
        // Defense-in-depth: if prSplitConfig is absent (test/offline),
        // fall back immediately rather than crashing on property access.
        if (typeof prSplitConfig === 'undefined') {
            log.printf('auto-analysis: prSplitConfig unavailable — falling back to heuristic');
            return startAnalysis(s);
        }

        s.isProcessing = true;
        s.analysisProgress = 0;
        s.analysisStartedAt = Date.now();  // T002: track start time for timeout
        s.analysisSlowWarning = false;     // T002: reset slow warning
        s.configValidationError = null; // T43: clear previous validation error on retry
        s.availableBranches = [];       // T43: clear branch list on retry
        s.analysisSteps = [
            { label: 'Verify baseline', active: true, done: false },
            { label: 'Spawning Claude', active: false, done: false },
            { label: 'Classifying files', active: false, done: false },
            { label: 'Generating plan', active: false, done: false },
            { label: 'Executing splits', active: false, done: false }
        ];

        // Run config state handler (same as heuristic path).
        // Pass outputFn to suppress output.print in TUI context.
        var configResult = handleConfigState({
            baseBranch: prSplit.runtime.baseBranch,
            dir: prSplit.runtime.dir,
            strategy: prSplit.runtime.strategy,
            verifyCommand: prSplit.runtime.verifyCommand,
            outputFn: function(s) { log.printf('wizard: %s', s); }
        });

        if (configResult.error) {
            // T43: Stay on CONFIG with inline validation error instead of jumping to ERROR.
            s.isProcessing = false;
            s.configValidationError = configResult.error;
            if (configResult.availableBranches) {
                s.availableBranches = configResult.availableBranches;
            }
            s.wizardState = 'CONFIG';
            return [s, null];
        }

        // T090: Stash baseline verify config for async pre-step.
        var baselineVerifyConfig = configResult.baselineVerifyConfig || null;

        if (s.wizard.current === 'IDLE') {
            s.wizard.transition('CONFIG');
        }

        // Initialize Claude executor if needed.
        if (!st.claudeExecutor) {
            st.claudeExecutor = new (prSplit.ClaudeCodeExecutor)(prSplitConfig);
        }

        // T113: Avoid calling the synchronous isAvailable() here — it invokes
        // exec.execv('which claude') which blocks the BubbleTea event loop.
        // Instead, check the cached resolution state and defer to the async
        // check-claude tick if the executor hasn't resolved yet.
        if (!st.claudeExecutor.resolved) {
            // Executor not yet resolved — defer via async check-claude.
            log.printf('auto-analysis: executor not yet resolved — deferring to async check');
            s.pendingAutoAnalysis = true;
            return [s, tea.tick(1, 'check-claude')];
        }

        // Build config for automatedSplit (mirrors REPL 'run' command).
        var autoConfig = {
            baseBranch: prSplit.runtime.baseBranch,
            strategy: prSplit.runtime.strategy,
            cleanupOnFailure: prSplitConfig.cleanupOnFailure
        };
        if (prSplitConfig.timeoutMs > 0) {
            autoConfig.classifyTimeoutMs = prSplitConfig.timeoutMs;
            autoConfig.planTimeoutMs = prSplitConfig.timeoutMs;
            autoConfig.resolveTimeoutMs = prSplitConfig.timeoutMs;
            autoConfig.verifyTimeoutMs = prSplitConfig.timeoutMs;
        }

        // Launch the pipeline as an async Promise. It runs on the JS
        // event loop independently of BubbleTea's message loop.
        s.autoSplitRunning = true;
        s.autoSplitResult = null;

        // T44: Install global output capture to pipe git command output to Output tab.
        prSplit._outputCaptureFn = function(line) {
            s.outputLines.push(line);
            if (s.outputLines.length > C.OUTPUT_BUFFER_CAP) {
                s.outputLines = s.outputLines.slice(-C.OUTPUT_BUFFER_CAP);
            }
            if (s.outputAutoScroll) {
                s.outputViewOffset = 0;
            }
        };

        // T090: Run async baseline verify, then launch automatedSplit.
        // Both run non-blocking on the JS event loop.
        (async function() {
            // Baseline verify pre-step.
            var bvc = baselineVerifyConfig;
            if (bvc && bvc.verifyCommand && bvc.verifyCommand !== 'true') {
                // T389: Activate Verify tab for live baseline verify output.
                s.verifyFallbackRunning = true;
                s.activeVerifyBranch = 'baseline';
                s.activeVerifyStartTime = Date.now();
                s.verifyElapsedMs = 0;
                if (!s.verifyScreen) s.verifyScreen = '';
                if (s.splitViewEnabled && s.splitViewTab !== 'verify') {
                    s.splitViewTab = 'verify';
                }

                s.analysisSteps[0].active = true;
                var baseStart = Date.now();
                try {
                    var baselineResult = await prSplit.verifySplitAsync(prSplit.runtime.baseBranch, {
                        verifyCommand: bvc.verifyCommand,
                        dir: bvc.dir,
                        verifyTimeoutMs: bvc.verifyTimeoutMs,
                        outputFn: function(line) {
                            log.printf('wizard: %s', line);
                            // T389: Route verify command output to Verify tab.
                            s.verifyScreen = (s.verifyScreen || '') + line + '\n';
                        }
                    });
                    if (!baselineResult.passed) {
                        s.verifyFallbackRunning = false;
                        s.analysisSteps[0].active = false;
                        throw new Error('Baseline verification failed: ' +
                            (baselineResult.error || 'exit code non-zero'));
                    }
                } catch (e) {
                    s.verifyFallbackRunning = false;
                    s.analysisSteps[0].active = false;
                    throw e; // re-throw to outer rejection handler
                }
                s.verifyFallbackRunning = false;
                s.analysisSteps[0].done = true;
                s.analysisSteps[0].active = false;
                s.analysisSteps[0].elapsed = Date.now() - baseStart;
                log.printf('wizard: baseline verify OK (%dms)', s.analysisSteps[0].elapsed);
            } else {
                s.analysisSteps[0].done = true;
                s.analysisSteps[0].active = false;
                s.analysisSteps[0].elapsed = 0;
            }
            s.analysisSteps[1].active = true; // Activate 'Spawning Claude' step
            return await prSplit.automatedSplit(autoConfig);
        })().then(
            function(result) {
                s.autoSplitResult = result;
                s.autoSplitRunning = false;
            },
            function(err) {
                s.autoSplitResult = { error: (err && err.message) ? err.message : String(err) };
                s.autoSplitRunning = false;
            }
        );

        // T389: Auto-open split-view. If verify command configured, pre-activate
        // Verify tab for immediate baseline verify display. Otherwise Output tab.
        if (!s.splitViewEnabled && s.height >= C.INLINE_VIEW_HEIGHT) {
            s.splitViewEnabled = true;
            s.splitViewFocus = 'wizard';
            if (baselineVerifyConfig && baselineVerifyConfig.verifyCommand &&
                baselineVerifyConfig.verifyCommand !== 'true') {
                s.verifyFallbackRunning = true;
                s.activeVerifyBranch = 'baseline';
                s.verifyScreen = '';
                s.splitViewTab = 'verify';
            } else {
                s.splitViewTab = 'output';
            }
            syncMainViewport(s);
        }

        // Poll for completion every 500ms.
        return [s, tea.tick(C.AUTO_SPLIT_POLL_MS, 'auto-poll')];
    }

    // handleAutoSplitPoll: Called every 500ms to check if the async
    // automatedSplit pipeline has completed. Updates progress indicators,
    // performs periodic Claude health checks, and handles the final result.
    function handleAutoSplitPoll(s) {
        // If cancelled, stop polling.
        if (!s.isProcessing) {
            return [s, null];
        }

        // Still running — update progress from pipeline state and poll again.
        if (s.autoSplitRunning) {
            // Claude health check: use session model (event-driven via isDone)
            // with direct handle.isAlive() fallback every healthPollMs.
            var healthPollMs = typeof prSplit.AUTOMATED_DEFAULTS.claudeHealthPollMs === 'number' ? prSplit.AUTOMATED_DEFAULTS.claudeHealthPollMs : 5000;
            var now = Date.now();
            if (!s.lastClaudeHealthCheckMs || (now - s.lastClaudeHealthCheckMs >= healthPollMs)) {
                s.lastClaudeHealthCheckMs = now;

                // Pinned session check: use tuiMux.isDone(claudeSessionID) when
                // available. This is channel-based (event-driven) and reads
                // from the pinned SessionID — not the mutable ActiveID.
                var executor = st.claudeExecutor;
                var sessionDead = false;
                var cid = st.claudeSessionID;
                if (executor && executor.handle &&
                    typeof tuiMux !== 'undefined' && tuiMux && cid &&
                    typeof tuiMux.isDone === 'function') {
                    if (tuiMux.isDone(cid)) {
                        sessionDead = true;
                    }
                }

                // Direct handle fallback — catches process death before
                // PTY output fully drains (more responsive).
                if (!sessionDead && executor && executor.handle &&
                    typeof executor.handle.isAlive === 'function') {
                    if (!executor.handle.isAlive()) {
                        sessionDead = true;
                    }
                }

                if (sessionDead) {
                    // Claude process died — capture diagnostic output.
                    var diagnostic = '';
                    if (executor && typeof executor.captureDiagnostic === 'function') {
                        diagnostic = executor.captureDiagnostic();
                    }
                    log.printf('auto-split: Claude crash detected by TUI health check (session model)');

                    // Transition to error resolution with crash context.
                    // No st.claudeCrashDetected — the pipeline's aliveCheckFn
                    // uses tuiMux.isDone(claudeSessionID) directly (event-driven).
                    s.isProcessing = false;
                    s.autoSplitRunning = false;
                    s.claudeCrashDetected = true;
                    s.errorDetails = 'Claude process crashed unexpectedly.' +
                        (diagnostic ? '\n\nLast output:\n' + diagnostic : '');
                    // T45: Auto-close split-view on Claude crash with notification.
                    if (s.splitViewEnabled) {
                        s.splitViewEnabled = false;
                        s.claudeScreenshot = '';
                        s.claudeScreen = '';
                        s.claudeViewOffset = 0;
                        s.splitViewFocus = 'wizard';
                        s.splitViewTab = 'claude';
                        s.claudeAutoAttachNotif = 'Claude crashed \u2014 split-view closed';
                        s.claudeAutoAttachNotifAt = Date.now();
                        syncMainViewport(s); // T120: sync dimensions after close.
                    }
                    s.wizard.transition('ERROR_RESOLUTION');
                    s.wizardState = 'ERROR_RESOLUTION';
                    return [s, tea.tick(C.DISMISS_NOTIF_MS, 'dismiss-attach-notif')];
                }
            }
            // T45+T388: Auto-attach Claude pane when Claude spawns.
            // Trigger once: when tuiMux has a child (Claude attached by pipeline),
            // user hasn't manually dismissed, and terminal is tall enough.
            // T388: Removed !s.splitViewEnabled guard — split-view may already be
            // open on the Output tab (auto-opened by startAutoAnalysis). We still
            // need to switch to the Claude tab and mark auto-attached.
            // Task 5: Use pinned SessionID proxy instead of raw session() check.
            var claudeAutoPane = getInteractivePaneSession(s, 'claude');
            if (!s.claudeAutoAttached && !s.claudeManuallyDismissed &&
                s.height >= C.INLINE_VIEW_HEIGHT &&
                claudeAutoPane && typeof claudeAutoPane.isRunning === 'function' &&
                claudeAutoPane.isRunning()) {
                s.splitViewEnabled = true;
                s.splitViewFocus = 'wizard';   // keep wizard focused
                s.splitViewTab = 'claude';     // show Claude tab
                s.claudeAutoAttached = true;
                syncMainViewport(s); // T120: sync dimensions after auto-attach.
                s.claudeAutoAttachNotif = 'Claude connected \u2014 Ctrl+L to toggle, Ctrl+] for passthrough';
                s.claudeAutoAttachNotifAt = Date.now();
                log.printf('auto-split: auto-attached Claude pane (height=%d)', s.height);
                // Start screenshot polling immediately via batched tick.
                // T028: Also schedule dismiss tick for the notification.
                return [s, tea.batch(
                    tea.tick(C.TICK_INTERVAL_MS, 'claude-screenshot'),
                    tea.tick(C.AUTO_SPLIT_POLL_MS, 'auto-poll'),
                    tea.tick(C.DISMISS_NOTIF_MS, 'dismiss-attach-notif')
                )];
            }

            // Read progress from pipeline's telemetry state.
            var pipelineState = prSplit._state || {};
            var telemetry = pipelineState.telemetryData || {};

            // Infer progress from what caches are populated.
            // T090: Step layout is now 5 entries:
            //   [0] Verify baseline  (always done — runs before automatedSplit)
            //   [1] Spawning Claude
            //   [2] Classifying files
            //   [3] Generating plan
            //   [4] Executing splits
            // T123: Guard — analysisSteps may be empty if handleAutoSplitPoll
            // fires before startAutoAnalysis populates the step array.
            if (s.analysisSteps && s.analysisSteps.length >= 5) {
                s.analysisSteps[0].done = true; s.analysisSteps[0].active = false; // baseline always done
                if (pipelineState.planCache) {
                    s.analysisSteps[1].done = true; s.analysisSteps[1].active = false;
                    s.analysisSteps[2].done = true; s.analysisSteps[2].active = false;
                    s.analysisSteps[3].done = true; s.analysisSteps[3].active = false;
                    s.analysisSteps[4].active = true;
                    s.analysisProgress = 0.8;
                } else if (pipelineState.groupsCache) {
                    s.analysisSteps[1].done = true; s.analysisSteps[1].active = false;
                    s.analysisSteps[2].done = true; s.analysisSteps[2].active = false;
                    s.analysisSteps[3].active = true;
                    s.analysisProgress = 0.6;
                } else if (pipelineState.analysisCache) {
                    s.analysisSteps[1].done = true; s.analysisSteps[1].active = false;
                    s.analysisSteps[2].active = true;
                    s.analysisProgress = 0.4;
                }
            }

            s.spinnerFrame = (s.spinnerFrame || 0) + 1;
            return [s, tea.tick(C.AUTO_SPLIT_POLL_MS, 'auto-poll')];
        }

        // Pipeline completed — process result.
        var result = s.autoSplitResult;
        s.isProcessing = false;

        if (result && result.error) {
            return enterErrorState(s, result.error);  // T116
        }

        // Populate caches from pipeline report.
        var report = (result && result.report) || {};
        if (report.analysis) { st.analysisCache = report.analysis; }
        if (report.classification) { st.groupsCache = report.classification; }
        if (report.plan) { st.planCache = report.plan; }
        if (report.splits) {
            st.executionResultCache = report.splits;
            s.executionResults = report.splits;
        }
        if (report.equivalence) {
            s.equivalenceResult = report.equivalence;
        }

        // Mark all analysis steps done.
        for (var i = 0; i < s.analysisSteps.length; i++) {
            s.analysisSteps[i].done = true;
            s.analysisSteps[i].active = false;
        }
        s.analysisProgress = 1.0;

        // Determine which state to transition to based on what the
        // pipeline completed. If execution happened, go to EQUIV_CHECK
        // or FINALIZATION. If only planning, go to PLAN_REVIEW.
        // T121: Guard against self-transition (CONFIG→CONFIG throws).
        if (s.wizard.current === 'IDLE') {
            s.wizard.transition('CONFIG');
        }
        if (s.wizard.current === 'CONFIG') {
            s.wizard.transition('PLAN_GENERATION');
        }

        if (report.splits && report.splits.length > 0) {
            // Pipeline completed execution — go to finalization.
            s.wizard.transition('PLAN_REVIEW');
            if (report.equivalence) {
                s.wizard.transition('BRANCH_BUILDING');
                s.wizard.transition('EQUIV_CHECK');
                s.wizardState = s.wizard.current;
            } else {
                s.wizard.transition('BRANCH_BUILDING');
                s.wizardState = 'BRANCH_BUILDING';
            }
        } else if (report.plan || st.planCache) {
            // Pipeline completed planning — go to plan review.
            s.wizard.transition('PLAN_REVIEW');
            s.wizardState = 'PLAN_REVIEW';
        } else {
            // Pipeline didn't produce enough data — error.
            return enterErrorState(s, 'Automated pipeline completed without a plan.');  // T116
        }

        return [s, null];
    }

    // --- Restart Claude Poll ---

    function handleRestartClaudePoll(s) {
        if (s.claudeRestarting) {
            // Still restarting — keep polling.
            return [s, tea.tick(C.AUTO_SPLIT_POLL_MS, 'restart-claude-poll')];
        }

        var result = s.restartResult;
        s.restartResult = null;

        if (!result || result.error) {
            s.errorDetails = (result && result.error) || 'Claude restart failed (unknown error)';
            // Keep claudeCrashDetected=true so crash-specific UI stays.
            return [s, null];
        }

        // Successful restart — clear crash flags and resume pipeline.
        // Note: only s.claudeCrashDetected (view state) is cleared. The
        // pipeline's aliveCheckFn uses tuiMux.isDone(claudeSessionID)
        // directly — no shared module-level crash flag to reset.
        s.claudeCrashDetected = false;
        s.errorDetails = null; // T114: clear stale error from restart phase
        log.printf('auto-split: Claude restarted successfully, session=%s', result.sessionId || '(none)');

        // Re-attach to tuiMux if available — capture pinned SessionID.
        var executor = st.claudeExecutor;
        if (executor && executor.handle && typeof tuiMux !== 'undefined' && tuiMux &&
            typeof tuiMux.attach === 'function') {
            try {
                var cid = tuiMux.attach(executor.handle);
                st.claudeSessionID = cid;
                log.debug('claude restart re-attached', { sessionID: cid });
            } catch (e) { log.debug('claude spawn tuiMux attach failed', { error: e.message || String(e) }); }
        }

        // T114: Mode-aware restart — if user was in auto mode, resume with
        // auto analysis (Claude-based), not heuristic. If a plan already
        // exists from before the crash, skip straight to execution.
        if (st.planCache && st.planCache.splits && st.planCache.splits.length > 0) {
            // Plan was generated before crash — re-execute from BRANCH_BUILDING.
            // ERROR_RESOLUTION → BRANCH_BUILDING is a valid transition.
            s.wizard.transition('BRANCH_BUILDING');
            s.wizardState = 'BRANCH_BUILDING';
            s.claudeAutoAttachNotif = 'Resumed after Claude restart \u2014 re-executing plan';
            s.claudeAutoAttachNotifAt = Date.now();
            return startExecution(s);
        }

        // No plan yet — restart the appropriate analysis pipeline.
        s.wizard.transition('PLAN_GENERATION');
        s.wizardState = 'PLAN_GENERATION';
        s.claudeAutoAttachNotif = 'Resumed after Claude restart \u2014 re-analyzing';
        s.claudeAutoAttachNotifAt = Date.now();
        if (prSplit.runtime.mode === 'auto') {
            return startAutoAnalysis(s);
        }
        return startAnalysis(s);
    }

    // --- Split-View: Key-to-Terminal-Bytes Conversion (T29) ---

    // Reserved keys that should NOT be forwarded to Claude when Claude pane
    // is focused. These stay with the wizard for pane management.
    var CLAUDE_RESERVED_KEYS = {
        'ctrl+tab': true,   // switch focus between panes
        'ctrl+l': true,     // close split-view
        'ctrl+o': true,     // T44: switch between Claude/Output tabs
        'ctrl+]': true,     // full Claude passthrough
        'ctrl++': true,     // adjust split ratio
        'ctrl+=': true,     // adjust split ratio
        'ctrl+-': true,     // adjust split ratio
        'up': true,         // scroll Claude pane viewport up
        'down': true,       // scroll Claude pane viewport down
        'k': true,          // scroll Claude pane viewport up (vim)
        'j': true,          // scroll Claude pane viewport down (vim)
        'pgup': true,       // scroll Claude pane up (page)
        'pgdown': true,     // scroll Claude pane down (page)
        'home': true,       // scroll Claude pane to top
        'end': true,        // scroll Claude pane to bottom
        'f1': true,         // help
        // T62: Selection and clipboard keys.
        'shift+up': true,
        'shift+down': true,
        'shift+left': true,
        'shift+right': true,
        'ctrl+shift+c': true,
        'ctrl+shift+v': true
    };

    // T386: Minimal reserved keys for fully-interactive tabs (Shell).
    // Only pane-management keys are reserved; navigation keys (arrows, j/k,
    // pgup/pgdown, home/end) are forwarded to the child process.
    var INTERACTIVE_RESERVED_KEYS = {
        'ctrl+tab': true,   // switch focus between panes
        'ctrl+l': true,     // close split-view
        'ctrl+o': true,     // cycle tabs
        'ctrl+]': true,     // full Claude passthrough
        'ctrl++': true,     // adjust split ratio
        'ctrl+=': true,     // adjust split ratio
        'ctrl+-': true,     // adjust split ratio
        'f1': true,         // help
        // T62: Selection and clipboard keys.
        'shift+up': true,
        'shift+down': true,
        'shift+left': true,
        'shift+right': true,
        'ctrl+shift+c': true,
        'ctrl+shift+v': true
    };

    // Convert BubbleTea key string to terminal byte sequence for PTY forwarding.
    // Returns the bytes as a string, or null if the key can't be converted.
    //
    // Key names match BubbleTea's KeyMsg.String() output (keys_gen.go).
    function keyToTermBytes(key) {
        // Named special keys → terminal escape sequences.
        switch (key) {
            case 'enter':     return '\r';
            case 'tab':       return '\t';
            case 'shift+tab': return '\x1b[Z'; // T386: reverse tab
            case 'backspace': return '\x7f';
            // Note: BubbleTea sends ' ' (literal space) for space key,
            // which is handled by the single-char fallback below.
            case 'esc':       return '\x1b'; // T386: fixed — was 'escape'
            case 'delete':    return '\x1b[3~';
            case 'up':        return '\x1b[A';
            case 'down':      return '\x1b[B';
            case 'right':     return '\x1b[C';
            case 'left':      return '\x1b[D';
            case 'home':      return '\x1b[H';
            case 'end':       return '\x1b[F';
            case 'pgup':      return '\x1b[5~';
            case 'pgdown':    return '\x1b[6~';
            case 'insert':    return '\x1b[2~';
            case 'f1':        return '\x1bOP';
            case 'f2':        return '\x1bOQ';
            case 'f3':        return '\x1bOR';
            case 'f4':        return '\x1bOS';
            case 'f5':        return '\x1b[15~';
            case 'f6':        return '\x1b[17~';
            case 'f7':        return '\x1b[18~';
            case 'f8':        return '\x1b[19~';
            case 'f9':        return '\x1b[20~';
            case 'f10':       return '\x1b[21~';
            case 'f11':       return '\x1b[23~';
            case 'f12':       return '\x1b[24~';
        }

        // Ctrl+letter → control character (0x01-0x1A for a-z).
        if (key.length > 5 && key.substring(0, 5) === 'ctrl+') {
            var ch = key.substring(5);
            if (ch.length === 1) {
                var code = ch.charCodeAt(0);
                // a-z → 0x01-0x1A
                if (code >= 97 && code <= 122) {
                    return String.fromCharCode(code - 96);
                }
                // A-Z → 0x01-0x1A
                if (code >= 65 && code <= 90) {
                    return String.fromCharCode(code - 64);
                }
            }
        }

        // T386: Modifier+navigation keys → xterm CSI {modifier} sequences.
        // Format: ESC[1;{mod}{letter} where mod: 2=Shift, 5=Ctrl, 6=Ctrl+Shift.
        // For tilde-style keys: ESC[{num};{mod}~ (pgup, pgdown, delete, insert).
        var modNavMap = {
            'up': 'A', 'down': 'B', 'right': 'C', 'left': 'D',
            'home': 'H', 'end': 'F'
        };
        var modTildeMap = {
            'pgup': '5', 'pgdown': '6', 'delete': '3', 'insert': '2'
        };
        var modPrefixes = [
            {prefix: 'ctrl+shift+', mod: '6'},
            {prefix: 'shift+', mod: '2'},
            {prefix: 'ctrl+', mod: '5'}
        ];
        for (var mi = 0; mi < modPrefixes.length; mi++) {
            var mp = modPrefixes[mi];
            if (key.length > mp.prefix.length && key.substring(0, mp.prefix.length) === mp.prefix) {
                var navKey = key.substring(mp.prefix.length);
                if (modNavMap[navKey]) {
                    return '\x1b[1;' + mp.mod + modNavMap[navKey];
                }
                if (modTildeMap[navKey]) {
                    return '\x1b[' + modTildeMap[navKey] + ';' + mp.mod + '~';
                }
            }
        }

        // Alt+key → ESC prefix + key bytes.
        if (key.length > 4 && key.substring(0, 4) === 'alt+') {
            var inner = keyToTermBytes(key.substring(4));
            if (inner !== null) {
                return '\x1b' + inner;
            }
        }

        // Paste content: bracketed paste "[content]" → just the content.
        if (key.length > 2 && key.charAt(0) === '[' && key.charAt(key.length - 1) === ']') {
            return key.substring(1, key.length - 1);
        }

        // Single printable character → send as-is.
        if (key.length === 1) {
            return key;
        }

        // Multi-character unknown keys (e.g., Unicode) → send as-is.
        if (key.length > 1 && key.indexOf('+') === -1) {
            return key;
        }

        return null;
    }

    // Convert a BubbleTea mouse message to SGR mouse escape sequence bytes.
    // offsetRow/offsetCol adjust coordinates so (0,0) maps to the pane origin.
    // Returns the escape sequence string, or null if the event can't be mapped.
    //
    // SGR format: ESC[<Cb;Cx;CyM (press/motion) or ESC[<Cb;Cx;Cym (release)
    //   Cb = button code + modifier bits
    //   Cx = 1-based column, Cy = 1-based row
    function mouseToTermBytes(msg, offsetRow, offsetCol) {
        var x = msg.x - (offsetCol || 0);
        var y = msg.y - (offsetRow || 0);
        if (x < 0 || y < 0) return null;

        // Map BubbleTea button string to SGR button code base value.
        var btn;
        switch (msg.button) {
            case 'left':        btn = 0; break;
            case 'middle':      btn = 1; break;
            case 'right':       btn = 2; break;
            case 'wheel up':    btn = 64; break;
            case 'wheel down':  btn = 65; break;
            case 'wheel left':  btn = 66; break;
            case 'wheel right': btn = 67; break;
            case 'backward':    btn = 128; break;
            case 'forward':     btn = 129; break;
            case 'none':        btn = 3; break;
            default:            return null;
        }

        // Modifier bits.
        if (msg.mod && msg.mod.includes('shift')) btn += 4;
        if (msg.mod && msg.mod.includes('alt'))   btn += 8;
        if (msg.mod && msg.mod.includes('ctrl'))  btn += 16;

        // Motion flag (bit 5).
        if (msg.type === 'MouseMotion') btn += 32;

        // SGR uses 1-based coordinates.
        var cx = x + 1;
        var cy = y + 1;

        // Press/motion → 'M', release → 'm'.
        var suffix = (msg.type === 'MouseRelease') ? 'm' : 'M';
        return '\x1b[<' + btn + ';' + cx + ';' + cy + suffix;
    }

    // --- T46: Claude Question Detection ---

    // detectClaudeQuestion analyses the plain-text screenshot of Claude's
    // terminal to determine whether Claude is asking the user a question.
    // Heuristics: confirmation patterns, conversational question openers,
    // plain question marks. Only fires when idle ≥ idleThresholdMs (2s).
    var QUESTION_IDLE_THRESHOLD_MS = C.QUESTION_IDLE_MS;

    // Explicit confirmation prompt patterns (case-insensitive).
    var CONFIRM_PATTERNS = [
        /\(y\/n\)/i,
        /\[y\/n\]/i,
        /\[yes\/no\]/i,
        /\(yes\/no\)/i,
        /\bproceed\s*\?/i,
        /\bcontinue\s*\?/i,
        /\bconfirm\s*\?/i,
        /\boverwrite\s*\?/i,
        /\bdelete\s*\?/i,
        /\breplace\s*\?/i,
        /\baccept\s*\?/i,
        /\bapprove\s*\?/i
    ];

    // Conversational question openers (case-insensitive, anchored to line start after whitespace).
    var QUESTION_OPENERS = [
        /^\s*do you\b/i,
        /^\s*would you\b/i,
        /^\s*should I\b/i,
        /^\s*can you\b/i,
        /^\s*could you\b/i,
        /^\s*is this\b/i,
        /^\s*are you\b/i,
        /^\s*shall I\b/i,
        /^\s*want me to\b/i,
        /^\s*may I\b/i,
        /^\s*please confirm\b/i,
        /^\s*please clarify\b/i,
        /^\s*what would you\b/i,
        /^\s*which\b.*\bprefer/i,
        /^\s*how would you\b/i,
        /^\s*where should\b/i
    ];

    function detectClaudeQuestion(plainText, idleMs) {
        var result = { detected: false, line: '' };

        // Guard: not idle long enough — Claude is likely still streaming.
        if (typeof idleMs !== 'number' || idleMs < QUESTION_IDLE_THRESHOLD_MS) {
            return result;
        }

        if (!plainText || typeof plainText !== 'string') {
            return result;
        }

        // Extract trailing non-empty lines (last 15 lines of visible terminal).
        var allLines = plainText.split('\n');
        // Trim trailing blank lines (VTerm pads).
        while (allLines.length > 0 && allLines[allLines.length - 1].trim() === '') {
            allLines.pop();
        }
        if (allLines.length === 0) {
            return result;
        }

        var scanCount = Math.min(C.QUESTION_SCAN_LINES, allLines.length);
        var scanLines = allLines.slice(allLines.length - scanCount);

        // Scan from bottom to top — the question is most likely at/near the
        // bottom of the visible output.
        for (var i = scanLines.length - 1; i >= 0; i--) {
            var raw = scanLines[i];
            var trimmed = raw.trim();
            if (trimmed.length === 0) continue;

            // 1. Explicit confirmation patterns (highest confidence).
            for (var cp = 0; cp < CONFIRM_PATTERNS.length; cp++) {
                if (CONFIRM_PATTERNS[cp].test(trimmed)) {
                    result.detected = true;
                    result.line = trimmed;
                    return result;
                }
            }

            // 2. Conversational question openers.
            for (var qo = 0; qo < QUESTION_OPENERS.length; qo++) {
                if (QUESTION_OPENERS[qo].test(trimmed)) {
                    result.detected = true;
                    result.line = trimmed;
                    return result;
                }
            }

            // 3. Line ends with "?" (general question heuristic).
            //    Only match non-trivial lines (>= 10 chars) to avoid
            //    false positives like prompt strings "? " or single "?".
            if (trimmed.length >= 10 && trimmed.charAt(trimmed.length - 1) === '?') {
                result.detected = true;
                result.line = trimmed;
                return result;
            }
        }

        return result;
    }

    // --- Split-View: Event-Aware Claude Screenshot Polling ---
    //
    // Task 9: Replaces blind 500ms polling with event-driven updates.
    // Event handlers (wireClaudeLifecycleEvents) set dirty flags; this
    // function reads them and skips redundant snapshots when nothing
    // changed. Uses adaptive tick intervals: fast when Claude is outputting,
    // slow when idle.

    function pollClaudeScreenshot(s) {
        // Stop polling if split view was disabled.
        if (!s.splitViewEnabled) {
            return [s, null];
        }

        // Ensure lifecycle event handlers are wired (idempotent).
        wireClaudeLifecycleEvents();

        // Guard: no tuiMux or Claude's pinned session.
        var cid = st.claudeSessionID;
        if (typeof tuiMux === 'undefined' || !tuiMux || !cid) {
            s.claudeScreen = '';
            s.claudeScreenshot = '';
            s.claudeLifecycleState = 'detached';
            return [s, tea.tick(C.CLAUDE_SCREENSHOT_POLL_MS, 'claude-screenshot')];
        }

        // Event-driven exit detection: check event flag first, then isDone
        // as fallback. Immediate response — no 500ms wait.
        var claudeExited = st._claudeExitEvent || st._claudeClosedEvent ||
            (typeof tuiMux.isDone === 'function' && tuiMux.isDone(cid));
        if (claudeExited) {
            s.claudeScreen = '';
            s.claudeScreenshot = '';
            s.claudeLifecycleState = deriveClaudeLifecycleState(s);
            // T45: Auto-close split-view when Claude exits (auto-attached only).
            if (s.claudeAutoAttached && !s.autoSplitRunning) {
                s.splitViewEnabled = false;
                s.splitViewFocus = 'wizard';
                s.splitViewTab = 'claude';
                syncMainViewport(s);
                s.claudeAutoAttachNotif = 'Claude session ended \u2014 split-view closed';
                s.claudeAutoAttachNotifAt = Date.now();
                unwireClaudeLifecycleEvents();
                return [s, tea.tick(C.DISMISS_NOTIF_MS, 'dismiss-attach-notif')];
            }
            return [s, tea.tick(C.CLAUDE_SCREENSHOT_POLL_MS, 'claude-screenshot')];
        }

        // Bell flash management: clear expired bell indicator.
        if (st._claudeBellFlash && st._claudeBellFlashAt &&
            (Date.now() - st._claudeBellFlashAt) >= C.CLAUDE_BELL_FLASH_MS) {
            st._claudeBellFlash = false;
        }
        s.claudeBellFlash = !!st._claudeBellFlash;

        // Drain mux events before snapshotting so real output/bell activity
        // can update JS state through the binding's event listeners.
        // Note: wizardUpdateImpl also calls pollEvents() at the top of each
        // update cycle, but some code paths call pollClaudeScreenshot
        // directly, so this drain remains as a safety net (idempotent).
        if (typeof tuiMux.pollEvents === 'function') {
            try {
                tuiMux.pollEvents();
            } catch (e) {
                // Swallow — event draining is best-effort.
            }
        }

        // Snapshot with generation tracking: skip if nothing changed.
        // When gen is undefined/null (mock without gen field), always
        // snapshot to maintain backward compatibility with tests.
        var snapshotChanged = false;
        try {
            if (typeof tuiMux.snapshot === 'function') {
                var snap = tuiMux.snapshot(cid);
                if (snap) {
                    // Compare generation to detect actual screen changes.
                    // If gen is absent (mocks), always update.
                    if (snap.gen == null || snap.gen !== st._claudeLastSnapshotGen) {
                        st._claudeLastSnapshotGen = snap.gen;
                        snapshotChanged = true;
                        if (snap.fullScreen) {
                            s.claudeScreen = String(snap.fullScreen);
                        }
                        if (snap.plainText) {
                            s.claudeScreenshot = String(snap.plainText);
                        }
                    }
                }
            }
        } catch (e) {
            log.debug('pollClaudeScreenshot: snapshot failed', {
                sessionId: cid,
                error: e.message || String(e)
            });
        }

        // Clear the dirty flag after processing.
        st._claudeOutputDirty = false;

        // Derive and store lifecycle state for the view layer.
        s.claudeLifecycleState = deriveClaudeLifecycleState(s);

        // Question detection: run when snapshot changed or on throttled
        // timer (preserves idle-based detection semantics).
        var claudeAlive = !!(prSplit._state && prSplit._state.claudeExecutor &&
                             prSplit._state.claudeExecutor.handle);
        if ((s.isProcessing || claudeAlive) && !s.claudeQuestionInputActive) {
            var now46 = Date.now();
            var shouldCheck = snapshotChanged ||
                (!s.claudeLastQuestionCheckMs ||
                 (now46 - s.claudeLastQuestionCheckMs >= C.QUESTION_IDLE_MS));
            if (shouldCheck) {
                s.claudeLastQuestionCheckMs = now46;

                var idleMs46 = 0;
                try {
                    if (typeof tuiMux.lastActivityMs === 'function') {
                        idleMs46 = tuiMux.lastActivityMs();
                    }
                } catch (e46) {
                    // Swallow — may fail if child ended.
                }

                var qResult = detectClaudeQuestion(s.claudeScreenshot, idleMs46);
                if (qResult.detected && !s.claudeQuestionDetected) {
                    s.claudeQuestionDetected = true;
                    s.claudeQuestionLine = qResult.line;
                    s.claudeQuestionInputText = '';
                    s.claudeQuestionInputActive = false;
                    log.printf('T46: Claude question detected: %s', qResult.line);
                } else if (!qResult.detected && s.claudeQuestionDetected &&
                           !s.claudeQuestionInputActive) {
                    s.claudeQuestionDetected = false;
                    s.claudeQuestionLine = '';
                    s.claudeQuestionInputText = '';
                }
            }
        }

        // Adaptive tick interval: fast when Claude is actively outputting,
        // slower when idle. Reduces CPU when Claude is thinking.
        var now = Date.now();
        var recentOutput = st._claudeLastOutputMs &&
            (now - st._claudeLastOutputMs) < C.CLAUDE_OUTPUT_IDLE_MS;
        var tickMs = recentOutput ? C.CLAUDE_ACTIVE_POLL_MS : C.CLAUDE_IDLE_POLL_MS;
        return [s, tea.tick(tickMs, 'claude-screenshot')];
    }

    // --- Cross-chunk exports ---
    prSplit._handleClaudeCheck = handleClaudeCheck;
    prSplit._handleClaudeCheckPoll = handleClaudeCheckPoll;
    prSplit._startAutoAnalysis = startAutoAnalysis;
    prSplit._handleAutoSplitPoll = handleAutoSplitPoll;
    prSplit._handleRestartClaudePoll = handleRestartClaudePoll;
    prSplit._keyToTermBytes = keyToTermBytes;
    prSplit._mouseToTermBytes = mouseToTermBytes;
    prSplit._CLAUDE_RESERVED_KEYS = CLAUDE_RESERVED_KEYS;
    prSplit._INTERACTIVE_RESERVED_KEYS = INTERACTIVE_RESERVED_KEYS;
    prSplit._detectClaudeQuestion = detectClaudeQuestion;
    prSplit.QUESTION_IDLE_THRESHOLD_MS = QUESTION_IDLE_THRESHOLD_MS;
    prSplit._pollClaudeScreenshot = pollClaudeScreenshot;
    prSplit._wireClaudeLifecycleEvents = wireClaudeLifecycleEvents;
    prSplit._unwireClaudeLifecycleEvents = unwireClaudeLifecycleEvents;
    prSplit._deriveClaudeLifecycleState = deriveClaudeLifecycleState;

})(globalThis.prSplit);
