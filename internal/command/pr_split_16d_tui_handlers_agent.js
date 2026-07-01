'use strict';
// pr_split_16d_tui_handlers_agent.js — TUI: Agent automation, key byte conversion, question detection, screenshot polling
// Dependencies: chunks 00-16c must be loaded first.
// Requires Go-injected globals: tui, ctx, output, log, prSplitConfig, tuiMux.

(function(prSplit) {

    if (typeof tui === 'undefined' || typeof ctx === 'undefined' ||
        typeof output === 'undefined') { return; }

    // Cross-chunk imports — libraries.
    var tea = prSplit._tea;
    var C = prSplit._TUI_CONSTANTS;
    var termmux = require('osm:termmux');

    // Cross-chunk imports — state and handlers from chunks 13-14.
    var st = prSplit._state;
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

    // --- Agent Lifecycle Event Wiring ---
    //
    // Registers EventBus listeners for output/exit/bell/closed filtered by
    // Agent's pinned SessionID. Callbacks set dirty flags and lifecycle
    // markers that pollAgentScreenshot reads on each tick — no redundant
    // snapshots when nothing changed, and immediate response to exit/bell.

    /**
     * wireAgentLifecycleEvents registers event handlers for Agent's pinned
     * session. Call once when st.agentSessionID is first available.
     * Stores handler IDs on st._agentEventIDs for cleanup.
     */
    function wireAgentLifecycleEvents() {
        if (typeof tuiMux === 'undefined' || !tuiMux ||
            typeof tuiMux.on !== 'function') {
            return;
        }
        // Guard: don't double-wire.
        if (st._agentEventIDs && st._agentEventIDs.length > 0) {
            return;
        }
        var cid = st.agentSessionID;
        if (!cid) {
            return;
        }

        var ids = [];

        // Output event: set dirty flag + record timestamp for adaptive polling.
        ids.push(tuiMux.on('output', function(data) {
            if (data && data.sessionId === cid) {
                st._agentOutputDirty = true;
                st._agentLastOutputMs = Date.now();
            }
        }));

        // Exit event: mark Agent as exited for immediate lifecycle response.
        ids.push(tuiMux.on('exit', function(data) {
            if (data && data.sessionId === cid) {
                st._agentExitEvent = true;
                log.info('agent lifecycle: exit event received', { sessionId: cid });
            }
        }));

        // Bell event: set flash flag for visual indicator.
        ids.push(tuiMux.on('bell', function(data) {
            if (data && data.sessionId === cid) {
                st._agentBellFlash = true;
                st._agentBellFlashAt = Date.now();
                log.debug('agent lifecycle: bell event', { sessionId: cid });
            }
        }));

        // Closed event: session unregistered from SessionManager.
        ids.push(tuiMux.on('closed', function(data) {
            if (data && data.sessionId === cid) {
                st._agentClosedEvent = true;
                log.info('agent lifecycle: closed event received', { sessionId: cid });
            }
        }));

        st._agentEventIDs = ids;
        log.debug('agent lifecycle: event handlers registered', {
            sessionId: cid,
            handlerCount: ids.length
        });
    }

    /**
     * unwireAgentLifecycleEvents removes all registered event handlers
     * and resets lifecycle tracking state.
     */
    function unwireAgentLifecycleEvents() {
        if (typeof tuiMux === 'undefined' || !tuiMux ||
            typeof tuiMux.off !== 'function') {
            return;
        }
        var ids = st._agentEventIDs;
        if (ids && ids.length > 0) {
            for (var i = 0; i < ids.length; i++) {
                tuiMux.off(ids[i]);
            }
            log.debug('agent lifecycle: event handlers removed', {
                handlerCount: ids.length
            });
        }
        st._agentEventIDs = null;
        st._agentOutputDirty = false;
        st._agentLastOutputMs = 0;
        st._agentExitEvent = false;
        st._agentBellFlash = false;
        st._agentBellFlashAt = 0;
        st._agentClosedEvent = false;
        st._agentLastSnapshotGen = 0;
    }

    /**
     * deriveAgentLifecycleState computes the user-visible lifecycle state
     * from event flags and session status.
     *
     * States: 'detached' | 'active' | 'idle' | 'waiting' | 'exited' | 'crashed'
     */
    function deriveAgentLifecycleState(s) {
        var cid = st.agentSessionID;
        if (!cid) {
            return 'detached';
        }
        // Check exit/closed events first — terminal states.
        if (st._agentClosedEvent) {
            return 'closed';
        }
        if (st._agentExitEvent) {
            // Distinguish crash from normal exit: if the pipeline is still
            // running, it's a crash. If the pipeline completed, it's normal.
            return s.autoSplitRunning ? 'crashed' : 'exited';
        }
        // Check if session is done (fallback for missed events).
        if (typeof tuiMux.isDone === 'function' && tuiMux.isDone(cid)) {
            return s.autoSplitRunning ? 'crashed' : 'exited';
        }
        // Question detected → waiting for input.
        if (s.agentQuestionDetected) {
            return 'waiting';
        }
        // Recent output → active.
        var now = Date.now();
        if (st._agentLastOutputMs &&
            (now - st._agentLastOutputMs) < C.AGENT_OUTPUT_IDLE_MS) {
            return 'active';
        }
        // No recent output → idle.
        return 'idle';
    }

    // --- Agent Check Handlers ---

    function handleAgentCheck(s) {
        // Guard: prSplitConfig is injected from Go and may be absent in tests.
        if (typeof prSplitConfig === 'undefined') {
            s.agentCheckStatus = 'unavailable';
            s.agentCheckError = 'Configuration not available (test mode)';
            s.agentResolvedInfo = null;
            return [s, null];
        }

        // Guard: already checking — don't double-launch.
        if (s.agentCheckRunning) {
            return [s, tea.tick(C.AGENT_CHECK_POLL_MS, 'agent-check-poll')];
        }

        // Use cached executor if available (avoids redundant re-checks).
        if (st.agentExecutor && st.agentExecutor.resolved) {
            s.agentCheckStatus = 'available';
            s.agentResolvedInfo = st.agentExecutor.resolved;
            s.agentCheckError = null;
            return [s, null];
        }

        var executor = new (prSplit.AgentCodeExecutor)(prSplitConfig);
        s.agentCheckStatus = 'checking';
        s.agentCheckRunning = true;
        s.agentCheckProgressMsg = 'Resolving binary\u2026';

        runAgentCheckAsync(s, executor).then(
            function() {
                s.agentCheckRunning = false;
            },
            function(err) {
                s.agentCheckStatus = 'unavailable';
                s.agentCheckError = (err && err.message) ? err.message : String(err);
                s.agentResolvedInfo = null;
                prSplit.runtime.mode = 'heuristic';
                s.agentCheckRunning = false;
            }
        );

        // Poll at 50ms for responsive status updates.
        return [s, tea.tick(C.AGENT_CHECK_POLL_MS, 'agent-check-poll')];
    }

    // runAgentCheckAsync: Async function that runs resolveAsync on the
    // executor. Updates s.agentCheckProgressMsg for the view.
    async function runAgentCheckAsync(s, executor) {
        var result = await executor.resolveAsync(function(msg) {
            s.agentCheckProgressMsg = msg;
        });

        if (result.error) {
            s.agentCheckStatus = 'unavailable';
            s.agentCheckError = result.error;
            s.agentResolvedInfo = null;
            // Auto-fallback: switch to heuristic so user can proceed.
            prSplit.runtime.mode = 'heuristic';
        } else {
            s.agentCheckStatus = 'available';
            s.agentResolvedInfo = executor.resolved; // { command, type }
            s.agentCheckError = null;
            // Cache the resolved executor for startAutoAnalysis().
            st.agentExecutor = executor;
            // T42: Auto-select 'auto' strategy when Agent detected on startup,
            // unless the user has already manually selected a different strategy.
            if (!s.userHasSelectedStrategy) {
                prSplit.runtime.mode = 'auto';
            }
        }
    }

    // handleAgentCheckPoll: Called every 50ms to check if the async
    // Agent check has completed.
    function handleAgentCheckPoll(s) {
        // Still running — keep polling.
        if (s.agentCheckRunning) {
            return [s, tea.tick(C.AGENT_CHECK_POLL_MS, 'agent-check-poll')];
        }

        // T113: If startAutoAnalysis deferred to us because the executor
        // wasn't resolved yet, dispatch it now that the check is done.
        if (s.pendingAutoAnalysis) {
            s.pendingAutoAnalysis = false;
            if (s.agentCheckStatus === 'available' && st.agentExecutor && st.agentExecutor.resolved) {
                log.printf('auto-analysis: executor resolved — resuming pipeline');
                return startAutoAnalysis(s);
            }
            // Agent unavailable — fall back to heuristic.
            log.printf('auto-analysis: Agent unavailable after async check — falling back');
            return startAnalysis(s);
        }

        // Completed — view will render the final status.
        return [s, null];
    }

    // --- Automated pipeline (Agent) ---
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
            { label: 'Spawning Agent', active: false, done: false },
            { label: 'Classifying files', active: false, done: false },
            { label: 'Generating plan', active: false, done: false },
            { label: 'Executing splits', active: false, done: false }
        ];

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

        // Process config validation result: check executor, then launch pipeline.
        function processAutoConfigResult(configResult) {
            // Config validation settled — clear the async-path gate (no-op
            // in the sync path, where the flag was never set).
            s.autoConfigValidating = false;
            if (configResult.error) {
                // T43: Stay on CONFIG with inline validation error.
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

            // Initialize Agent executor if needed (after config validation).
            if (!st.agentExecutor) {
                st.agentExecutor = new (prSplit.AgentCodeExecutor)(prSplitConfig);
            }

            // T113: Avoid calling the synchronous isAvailable() here — it invokes
            // exec.execv('which agent') which blocks the BubbleTea event loop.
            // Instead, check the cached resolution state and defer to the async
            // check-agent tick if the executor hasn't resolved yet.
            if (!st.agentExecutor.resolved) {
                log.printf('auto-analysis: executor not yet resolved — deferring to async check');
                s.pendingAutoAnalysis = true;
                return [s, tea.tick(1, 'check-agent')];
            }

            // Executor is resolved — launch the pipeline.
            s.autoSplitRunning = true;
            s.autoSplitResult = null;

            // T389: Auto-open split-view synchronously using runtime config.
            if (!s.splitViewEnabled && s.height >= C.INLINE_VIEW_HEIGHT) {
                s.splitViewEnabled = true;
                s.splitViewFocus = 'wizard';
                if (prSplit.runtime.verifyCommand &&
                    prSplit.runtime.verifyCommand !== 'true') {
                    s.verifyFallbackRunning = true;
                    s.activeVerifyBranch = 'baseline';
                    s.verifyScreen = '';
                    s.splitViewTab = 'verify';
                } else {
                    s.splitViewTab = 'output';
                }
                syncMainViewport(s);
            }

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
            (async function() {
                var bvc = baselineVerifyConfig;
                if (bvc && bvc.verifyCommand && bvc.verifyCommand !== 'true') {
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
                        throw e;
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
                s.analysisSteps[1].active = true;
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

            return [s, tea.tick(C.AUTO_SPLIT_POLL_MS, 'auto-poll')];
        }

        function handleAutoConfigError(err) {
            // Delegate to processAutoConfigResult's error path so unexpected
            // promise rejections (e.g. git crashed, analyzeDiff threw)
            // surface identically to resolved validation errors:
            // configValidationError + wizardState='CONFIG' on the CONFIG
            // screen, where the user sees the message and can retry.
            // Without this, the poll handler's early !isProcessing exit
            // would silently drop the error.
            processAutoConfigResult({ error: (err && err.message) ? err.message : String(err) });
        }

        // Run config validation. handleConfigState returns sync when gitExec
        // is sync (tests), or a Promise in production. Dispatched dynamically
        // via prSplit._handleConfigState (not captured at module load) so
        // tests can override it to inject rejection paths.
        var autoConfigPromise = prSplit._handleConfigState({
            baseBranch: prSplit.runtime.baseBranch,
            dir: prSplit.runtime.dir,
            strategy: prSplit.runtime.strategy,
            verifyCommand: prSplit.runtime.verifyCommand,
            outputFn: function(s) { log.printf('wizard: %s', s); }
        });

        if (autoConfigPromise && typeof autoConfigPromise.then === 'function') {
            // Async path (production: gitExec returns Promises). Cannot return
            // the Promise — BubbleTea's updateDirect requires an Array
            // [state, cmd], not a thenable; returning a Promise logs an error,
            // drops the command, and freezes the wizard. Instead, run config
            // validation in the background and return [s, auto-poll] now.
            // autoConfigValidating gates handleAutoSplitPoll so it keeps polling
            // (rather than treating a still-null result as "pipeline complete")
            // until processAutoConfigResult settles and sets pendingAutoAnalysis
            // (deferral) or autoSplitRunning (launch), or records an error.
            s.autoConfigValidating = true;
            s._cfgPromise = autoConfigPromise;
            autoConfigPromise.then(processAutoConfigResult, handleAutoConfigError);
            return [s, tea.tick(C.AUTO_SPLIT_POLL_MS, 'auto-poll')];
        }

        // Sync path
        return processAutoConfigResult(autoConfigPromise);
    }

    // handleAutoSplitPoll: Called every 500ms to check if the async
    // automatedSplit pipeline has completed. Updates progress indicators,
    // performs periodic Agent health checks, and handles the final result.
    function handleAutoSplitPoll(s) {
        // If cancelled, stop polling.
        if (!s.isProcessing) {
            return [s, null];
        }

        // Async config-validation still in progress (production path where
        // gitExec returns Promises). processAutoConfigResult hasn't settled
        // yet, so pendingAutoAnalysis/autoSplitRunning aren't set — keep
        // polling instead of mistaking the null result for "pipeline complete".
        if (s.autoConfigValidating) {
            return [s, tea.tick(C.AUTO_SPLIT_POLL_MS, 'auto-poll')];
        }

        // Config validation deferred to the async Agent availability check.
        // Route to the check-agent poll (mirrors the sync path, which returns
        // a 'check-agent' tick directly from processAutoConfigResult).
        if (s.pendingAutoAnalysis && !s.autoSplitRunning) {
            return [s, tea.tick(1, 'check-agent')];
        }

        // Still running — update progress from pipeline state and poll again.
        if (s.autoSplitRunning) {
            // Agent health check: use session model (event-driven via isDone)
            // with direct handle.isAlive() fallback every healthPollMs.
            var healthPollMs = typeof prSplit.AUTOMATED_DEFAULTS.agentHealthPollMs === 'number' ? prSplit.AUTOMATED_DEFAULTS.agentHealthPollMs : 5000;
            var now = Date.now();
            if (!s.lastAgentHealthCheckMs || (now - s.lastAgentHealthCheckMs >= healthPollMs)) {
                s.lastAgentHealthCheckMs = now;

                // Pinned session check: use tuiMux.isDone(agentSessionID) when
                // available. This is channel-based (event-driven) and reads
                // from the pinned SessionID — not the mutable ActiveID.
                var executor = st.agentExecutor;
                var sessionDead = false;
                var cid = st.agentSessionID;
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
                    // Agent process died — capture diagnostic output.
                    var diagnostic = '';
                    if (executor && typeof executor.captureDiagnostic === 'function') {
                        diagnostic = executor.captureDiagnostic();
                    }
                    log.printf('auto-split: Agent crash detected by TUI health check (session model)');

                    // Transition to error resolution with crash context.
                    // No st.agentCrashDetected — the pipeline's aliveCheckFn
                    // uses tuiMux.isDone(agentSessionID) directly (event-driven).
                    s.isProcessing = false;
                    s.autoSplitRunning = false;
                    s.agentCrashDetected = true;
                    s.errorDetails = 'Agent process crashed unexpectedly.' +
                        (diagnostic ? '\n\nLast output:\n' + diagnostic : '');
                    // T45: Auto-close split-view on Agent crash with notification.
                    if (s.splitViewEnabled) {
                        s.splitViewEnabled = false;
                        s.agentScreenshot = '';
                        s.agentScreen = '';
                        s.agentViewOffset = 0;
                        s.splitViewFocus = 'wizard';
                        s.splitViewTab = 'agent';
                        s.agentAutoAttachNotif = 'Agent crashed \u2014 split-view closed';
                        s.agentAutoAttachNotifAt = Date.now();
                        syncMainViewport(s); // T120: sync dimensions after close.
                    }
                    s.wizard.transition('ERROR_RESOLUTION');
                    s.wizardState = 'ERROR_RESOLUTION';
                    return [s, tea.tick(C.DISMISS_NOTIF_MS, 'dismiss-attach-notif')];
                }
            }
            // T45+T388: Auto-attach Agent pane when Agent spawns.
            // Trigger once: when tuiMux has a child (Agent attached by pipeline),
            // user hasn't manually dismissed, and terminal is tall enough.
            // T388: Removed !s.splitViewEnabled guard — split-view may already be
            // open on the Output tab (auto-opened by startAutoAnalysis). We still
            // need to switch to the Agent tab and mark auto-attached.
            // Task 5: Use pinned SessionID proxy instead of raw session() check.
            var agentAutoPane = getInteractivePaneSession(s, 'agent');
            if (!s.agentAutoAttached && !s.agentManuallyDismissed &&
                s.height >= C.INLINE_VIEW_HEIGHT &&
                agentAutoPane && typeof agentAutoPane.isRunning === 'function' &&
                agentAutoPane.isRunning()) {
                s.splitViewEnabled = true;
                s.splitViewFocus = 'wizard';   // keep wizard focused
                s.splitViewTab = 'agent';     // show Agent tab
                s.agentAutoAttached = true;
                syncMainViewport(s); // T120: sync dimensions after auto-attach.
                s.agentAutoAttachNotif = 'Agent connected \u2014 Ctrl+L to toggle, Ctrl+] for passthrough';
                s.agentAutoAttachNotifAt = Date.now();
                log.printf('auto-split: auto-attached Agent pane (height=%d)', s.height);
                // Start screenshot polling immediately via batched tick.
                // T028: Also schedule dismiss tick for the notification.
                return [s, tea.batch(
                    tea.tick(C.TICK_INTERVAL_MS, 'agent-screenshot'),
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
            //   [1] Spawning Agent
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

    // --- Restart Agent Poll ---

    function handleRestartAgentPoll(s) {
        if (s.agentRestarting) {
            // Still restarting — keep polling.
            return [s, tea.tick(C.AUTO_SPLIT_POLL_MS, 'restart-agent-poll')];
        }

        var result = s.restartResult;
        s.restartResult = null;

        if (!result || result.error) {
            s.errorDetails = (result && result.error) || 'Agent restart failed (unknown error)';
            // Keep agentCrashDetected=true so crash-specific UI stays.
            return [s, null];
        }

        // Successful restart — clear crash flags and resume pipeline.
        // Note: only s.agentCrashDetected (view state) is cleared. The
        // pipeline's aliveCheckFn uses tuiMux.isDone(agentSessionID)
        // directly — no shared module-level crash flag to reset.
        s.agentCrashDetected = false;
        s.errorDetails = null; // T114: clear stale error from restart phase
        log.printf('auto-split: Agent restarted successfully, session=%s', result.sessionId || '(none)');

        // Re-attach to tuiMux if available — capture pinned SessionID.
        var executor = st.agentExecutor;
        if (executor && executor.handle && typeof tuiMux !== 'undefined' && tuiMux &&
            typeof tuiMux.attachAsync === 'function') {
            tuiMux.attachAsync(executor.handle).then(function(cid) {
                st.agentSessionID = cid;
                log.debug('agent restart re-attached', { sessionID: cid });
            }).catch(function(e) {
                log.debug('agent spawn tuiMux attachAsync failed', { error: e.message || String(e) });
            });
        } else if (executor && executor.handle && typeof tuiMux !== 'undefined' && tuiMux &&
            typeof tuiMux.attach === 'function') {
            try {
                var cid = tuiMux.attach(executor.handle);
                st.agentSessionID = cid;
                log.debug('agent restart re-attached', { sessionID: cid });
            } catch (e) { log.debug('agent spawn tuiMux attach failed', { error: e.message || String(e) }); }
        }

        // T114: Mode-aware restart — if user was in auto mode, resume with
        // auto analysis (Agent-based), not heuristic. If a plan already
        // exists from before the crash, skip straight to execution.
        if (st.planCache && st.planCache.splits && st.planCache.splits.length > 0) {
            // Plan was generated before crash — re-execute from BRANCH_BUILDING.
            // ERROR_RESOLUTION → BRANCH_BUILDING is a valid transition.
            s.wizard.transition('BRANCH_BUILDING');
            s.wizardState = 'BRANCH_BUILDING';
            s.agentAutoAttachNotif = 'Resumed after Agent restart \u2014 re-executing plan';
            s.agentAutoAttachNotifAt = Date.now();
            return startExecution(s);
        }

        // No plan yet — restart the appropriate analysis pipeline.
        s.wizard.transition('PLAN_GENERATION');
        s.wizardState = 'PLAN_GENERATION';
        s.agentAutoAttachNotif = 'Resumed after Agent restart \u2014 re-analyzing';
        s.agentAutoAttachNotifAt = Date.now();
        if (prSplit.runtime.mode === 'auto') {
            return startAutoAnalysis(s);
        }
        return startAnalysis(s);
    }

    // --- Split-View: Key-to-Terminal-Bytes Conversion (T29) ---

    // Reserved keys that should NOT be forwarded to Agent when Agent pane
    // is focused. These stay with the wizard for pane management.
    var AGENT_RESERVED_KEYS = {
        'ctrl+tab': true,   // switch focus between panes
        'ctrl+l': true,     // close split-view
        'ctrl+o': true,     // T44: switch between Agent/Output tabs
        'ctrl+]': true,     // full Agent passthrough
        'ctrl++': true,     // adjust split ratio
        'ctrl+=': true,     // adjust split ratio
        'ctrl+-': true,     // adjust split ratio
        'up': true,         // scroll Agent pane viewport up
        'down': true,       // scroll Agent pane viewport down
        'k': true,          // scroll Agent pane viewport up (vim)
        'j': true,          // scroll Agent pane viewport down (vim)
        'pgup': true,       // scroll Agent pane up (page)
        'pgdown': true,     // scroll Agent pane down (page)
        'home': true,       // scroll Agent pane to top
        'end': true,        // scroll Agent pane to bottom
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
        'ctrl+]': true,     // full Agent passthrough
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
    // Delegates to the Go termmux module for the actual encoding.
    function keyToTermBytes(key) {
        return termmux.keyToTermBytes(key);
    }

    // Convert a BubbleTea mouse message to SGR mouse escape sequence bytes.
    // Adapts BubbleTea's mod-array format to the termmux Go module.
    function mouseToTermBytes(msg, offsetRow, offsetCol) {
        var ev = {
            type: msg.type === 'MouseWheel' ? 'MouseClick' : msg.type,
            button: msg.button,
            x: msg.x,
            y: msg.y
        };
        if (msg.mod) {
            for (var i = 0; i < msg.mod.length; i++) {
                if (msg.mod[i] === 'shift') ev.shift = true;
                if (msg.mod[i] === 'alt') ev.alt = true;
                if (msg.mod[i] === 'ctrl') ev.ctrl = true;
            }
        }
        if (msg.type === 'MouseMotion') {
            ev.type = 'MouseMotion';
        }
        return termmux.mouseToSGR(ev, offsetRow || 0, offsetCol || 0);
    }

    // --- T46: Agent Question Detection ---

    // detectAgentQuestion analyses the plain-text screenshot of Agent's
    // terminal to determine whether Agent is asking the user a question.
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

    function detectAgentQuestion(plainText, idleMs) {
        var result = { detected: false, line: '' };

        // Guard: not idle long enough — Agent is likely still streaming.
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

    // --- Split-View: Event-Aware Agent Screenshot Polling ---
    //
    // Task 9: Replaces blind 500ms polling with event-driven updates.
    // Event handlers (wireAgentLifecycleEvents) set dirty flags; this
    // function reads them and skips redundant snapshots when nothing
    // changed. Uses adaptive tick intervals: fast when Agent is outputting,
    // slow when idle.

    function pollAgentScreenshot(s) {
        // Stop polling if split view was disabled.
        if (!s.splitViewEnabled) {
            return [s, null];
        }

        // Ensure lifecycle event handlers are wired (idempotent).
        wireAgentLifecycleEvents();

        // Guard: no tuiMux or Agent's pinned session.
        var cid = st.agentSessionID;
        if (typeof tuiMux === 'undefined' || !tuiMux || !cid) {
            s.agentScreen = '';
            s.agentScreenshot = '';
            s.agentLifecycleState = 'detached';
            return [s, tea.tick(C.AGENT_SCREENSHOT_POLL_MS, 'agent-screenshot')];
        }

        // Event-driven exit detection: check event flag first, then isDone
        // as fallback. Immediate response — no 500ms wait.
        var agentExited = st._agentExitEvent || st._agentClosedEvent ||
            (typeof tuiMux.isDone === 'function' && tuiMux.isDone(cid));
        if (agentExited) {
            s.agentScreen = '';
            s.agentScreenshot = '';
            s.agentLifecycleState = deriveAgentLifecycleState(s);
            // T45: Auto-close split-view when Agent exits (auto-attached only).
            if (s.agentAutoAttached && !s.autoSplitRunning) {
                s.splitViewEnabled = false;
                s.splitViewFocus = 'wizard';
                s.splitViewTab = 'agent';
                syncMainViewport(s);
                s.agentAutoAttachNotif = 'Agent session ended \u2014 split-view closed';
                s.agentAutoAttachNotifAt = Date.now();
                unwireAgentLifecycleEvents();
                return [s, tea.tick(C.DISMISS_NOTIF_MS, 'dismiss-attach-notif')];
            }
            return [s, tea.tick(C.AGENT_SCREENSHOT_POLL_MS, 'agent-screenshot')];
        }

        // Bell flash management: clear expired bell indicator.
        if (st._agentBellFlash && st._agentBellFlashAt &&
            (Date.now() - st._agentBellFlashAt) >= C.AGENT_BELL_FLASH_MS) {
            st._agentBellFlash = false;
        }
        s.agentBellFlash = !!st._agentBellFlash;

        // Drain mux events before snapshotting so real output/bell activity
        // can update JS state through the binding's event listeners.
        // Note: wizardUpdateImpl also calls pollEvents() at the top of each
        // update cycle, but some code paths call pollAgentScreenshot
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
                    if (snap.gen == null || snap.gen !== st._agentLastSnapshotGen) {
                        st._agentLastSnapshotGen = snap.gen;
                        snapshotChanged = true;
                        if (snap.fullScreen) {
                            s.agentScreen = String(snap.fullScreen);
                        }
                        if (snap.plainText) {
                            s.agentScreenshot = String(snap.plainText);
                        }
                    }
                }
            }
        } catch (e) {
            log.debug('pollAgentScreenshot: snapshot failed', {
                sessionId: cid,
                error: e.message || String(e)
            });
        }

        // Clear the dirty flag after processing.
        st._agentOutputDirty = false;

        // Derive and store lifecycle state for the view layer.
        s.agentLifecycleState = deriveAgentLifecycleState(s);

        // Question detection: run when snapshot changed or on throttled
        // timer (preserves idle-based detection semantics).
        var agentAlive = !!(prSplit._state && prSplit._state.agentExecutor &&
                             prSplit._state.agentExecutor.handle);
        if ((s.isProcessing || agentAlive) && !s.agentQuestionInputActive) {
            var now46 = Date.now();
            var shouldCheck = snapshotChanged ||
                (!s.agentLastQuestionCheckMs ||
                 (now46 - s.agentLastQuestionCheckMs >= C.QUESTION_IDLE_MS));
            if (shouldCheck) {
                s.agentLastQuestionCheckMs = now46;

                var idleMs46 = 0;
                try {
                    if (typeof prSplit._readAgentActivityMs === 'function') {
                        idleMs46 = prSplit._readAgentActivityMs();
                    }
                } catch (e46) {
                    // Swallow — may fail if child ended.
                }

                var qResult = detectAgentQuestion(s.agentScreenshot, idleMs46);
                if (qResult.detected && !s.agentQuestionDetected) {
                    s.agentQuestionDetected = true;
                    s.agentQuestionLine = qResult.line;
                    s.agentQuestionInputText = '';
                    s.agentQuestionInputActive = false;
                    log.printf('T46: Agent question detected: %s', qResult.line);
                } else if (!qResult.detected && s.agentQuestionDetected &&
                           !s.agentQuestionInputActive) {
                    s.agentQuestionDetected = false;
                    s.agentQuestionLine = '';
                    s.agentQuestionInputText = '';
                }
            }
        }

        // Adaptive tick interval: fast when Agent is actively outputting,
        // slower when idle. Reduces CPU when Agent is thinking.
        var now = Date.now();
        var recentOutput = st._agentLastOutputMs &&
            (now - st._agentLastOutputMs) < C.AGENT_OUTPUT_IDLE_MS;
        var tickMs = recentOutput ? C.AGENT_ACTIVE_POLL_MS : C.AGENT_IDLE_POLL_MS;
        return [s, tea.tick(tickMs, 'agent-screenshot')];
    }

    // --- Cross-chunk exports ---
    prSplit._handleAgentCheck = handleAgentCheck;
    prSplit._handleAgentCheckPoll = handleAgentCheckPoll;
    prSplit._startAutoAnalysis = startAutoAnalysis;
    prSplit._handleAutoSplitPoll = handleAutoSplitPoll;
    prSplit._handleRestartAgentPoll = handleRestartAgentPoll;
    prSplit._keyToTermBytes = keyToTermBytes;
    prSplit._mouseToTermBytes = mouseToTermBytes;
    prSplit._AGENT_RESERVED_KEYS = AGENT_RESERVED_KEYS;
    prSplit._INTERACTIVE_RESERVED_KEYS = INTERACTIVE_RESERVED_KEYS;
    prSplit._detectAgentQuestion = detectAgentQuestion;
    prSplit.QUESTION_IDLE_THRESHOLD_MS = QUESTION_IDLE_THRESHOLD_MS;
    prSplit._pollAgentScreenshot = pollAgentScreenshot;
    prSplit._wireAgentLifecycleEvents = wireAgentLifecycleEvents;
    prSplit._unwireAgentLifecycleEvents = unwireAgentLifecycleEvents;
    prSplit._deriveAgentLifecycleState = deriveAgentLifecycleState;

})(globalThis.prSplit);
