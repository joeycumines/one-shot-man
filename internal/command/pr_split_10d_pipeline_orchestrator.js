'use strict';
// pr_split_10d_pipeline_orchestrator.js — Pipeline: main automatedSplit orchestrator
// Dependencies: chunks 00-10c must be loaded first.

(function(prSplit) {
    // NOTE: automatedSplit internally late-binds ~34 dependencies from prSplit.*
    // at the START of each invocation, including cross-chunk deps from 10a/10b/10c.

    // --- automatedSplit — the main orchestrator ---

    // Orchestrates the full automated PR splitting pipeline.
    // Steps: analyze → spawn → classify → receive → validate → plan →
    //        execute → verify → resolve → report.
    // Returns { error: string|null, report: object }.
    async function automatedSplit(config) {
        config = config || {};

        // Late-bind ALL cross-chunk dependencies (called at runtime, not load time).
        var runtime = prSplit.runtime;
        var gitExec = prSplit._gitExec;
        var isCancelled = prSplit.isCancelled;
        var isForceCancelled = prSplit.isForceCancelled;  // T117
        var isPaused = prSplit.isPaused;
        var analyzeDiff = prSplit.analyzeDiffAsync;
        var createSplitPlan = prSplit.createSplitPlanAsync;
        var savePlan = prSplit.savePlan;
        var loadPlan = prSplit.loadPlan;
        var resolvePlanPath = prSplit.resolvePlanPath;
        var validateClassification = prSplit.validateClassification;
        var validateSplitPlan = prSplit.validateSplitPlan;
        var validatePlan = prSplit.validatePlan;
        var executeSplit = prSplit.executeSplitAsync;
        var verifySplits = prSplit.verifySplitsAsync;
        var verifyEquivalence = prSplit.verifyEquivalenceAsync;
        var cleanupBranches = prSplit.cleanupBranchesAsync;
        var ClaudeCodeExecutor = prSplit.ClaudeCodeExecutor;
        var renderClassificationPrompt = prSplit.renderClassificationPrompt;
        var padIndex = prSplit._padIndex;
        var osmod = prSplit._modules.osmod;
        var resolveDir = prSplit._resolveDir;
        var recordConversation = prSplit.recordConversation || function() {};
        var recordTelemetry = prSplit.recordTelemetry || function() {};
        var assessIndependence = prSplit.assessIndependence || function() { return []; };
        var state = prSplit._state;

        // Late-bind cross-chunk deps from 10a (config), 10b (send), 10c (resolve).
        var AUTOMATED_DEFAULTS = prSplit.AUTOMATED_DEFAULTS;
        var getCancellationError = prSplit._getCancellationError;
        var classificationToGroups = prSplit.classificationToGroups;
        var cleanupExecutor = prSplit.cleanupExecutor;
        var captureScreenshot = prSplit._captureScreenshot;
        var sendToHandle = prSplit.sendToHandle;
        var waitForLogged = prSplit.waitForLogged;
        var heuristicFallback = prSplit.heuristicFallback;
        var resolveConflictsWithClaude = prSplit.resolveConflictsWithClaude;

        var dir = resolveDir(config.dir || '.');

        // T105: Resolve plan path relative to the configured directory so
        // save/load and display messages all reference the correct location.
        var resolvedPlanPath = resolvePlanPath(null, dir);

        // Reset module-level state to prevent leakage across multiple runs
        // within the same JS VM.
        state.conversationHistory = [];
        state.telemetryData = {
            filesAnalyzed: 0,
            splitCount: 0,
            strategy: '',
            claudeInteractions: 0,
            conflictsResolved: 0,
            conflictsFailed: 0,
            startTime: new Date().toISOString(),
            endTime: null
        };
        state.claudeExecutor = null;
        state.mcpCallbackObj = null;
        state.analysisCache = null;
        state.groupsCache = null;
        state.planCache = null;
        state.executionResultCache = [];
        // T089: Clear cached equivalence result so stale results from a
        // previous pipeline run are not used by buildReport().
        state.equivalenceResult = null;
        // Also reset the prSplit-level pointers used by other chunks.
        prSplit._claudeExecutor = null;
        prSplit._mcpCallbackObj = null;

        var timeouts = {
            classify: config.classifyTimeoutMs || AUTOMATED_DEFAULTS.classifyTimeoutMs,
            plan: config.planTimeoutMs || AUTOMATED_DEFAULTS.planTimeoutMs,
            resolve: config.resolveTimeoutMs || AUTOMATED_DEFAULTS.resolveTimeoutMs,
            commandMs: config.resolveCommandTimeoutMs || AUTOMATED_DEFAULTS.resolveCommandTimeoutMs
        };
        var pollInterval = config.pollIntervalMs || AUTOMATED_DEFAULTS.pollIntervalMs;
        // Use typeof check: 0 is a valid value for retry/re-split counts (meaning "none").
        var maxAttemptsPerBranch = typeof config.maxResolveRetries === 'number' ? config.maxResolveRetries : AUTOMATED_DEFAULTS.maxResolveRetries;
        var maxReSplits = typeof config.maxReSplits === 'number' ? config.maxReSplits : AUTOMATED_DEFAULTS.maxReSplits;

        // Clamp: negative values must not cause spin-loops or nonsensical retries.
        if (pollInterval < AUTOMATED_DEFAULTS.minPollIntervalMs) { pollInterval = AUTOMATED_DEFAULTS.minPollIntervalMs; }
        if (maxReSplits < 0) { maxReSplits = 0; }
        if (maxAttemptsPerBranch < 0) { maxAttemptsPerBranch = 0; }

        // Pipeline-level timeout and watchdog.
        var pipelineTimeoutMs = config.pipelineTimeoutMs || AUTOMATED_DEFAULTS.pipelineTimeoutMs;
        var stepTimeoutMs = config.stepTimeoutMs || AUTOMATED_DEFAULTS.stepTimeoutMs;
        var watchdogIdleMs = config.watchdogIdleMs || AUTOMATED_DEFAULTS.watchdogIdleMs;
        var pipelineStartTime = Date.now();
        var lastProgressTime = Date.now();

        // Detect the auto-split BubbleTea TUI (injected from Go).
        // NOTE: The Go BubbleTea TUI was removed in the Go→JS TUI migration (T27).
        // Progress is now reported via output.print() directly.
        var hasTUI = false;

        var report = {
            mode: 'automated',
            steps: [],
            classification: null,
            plan: null,
            splits: [],
            conflicts: [],
            resolutions: [],
            independencePairs: [],
            claudeInteractions: 0,
            fallbackUsed: false,
            error: null
        };

        function emitOutput(text) {
            output.print(text);
            lastProgressTime = Date.now();
        }

        function updateDetail(stepName, detail) {
            // Placeholder: TUI detail display removed in T27 migration.
            // detail is logged for diagnostics.
            log.printf('auto-split detail [%s]: %s', stepName, detail);
        }

        // step() wrapper for pipeline steps. Supports both sync and async callbacks.
        async function step(name, fn) {
            // Check cancellation before each step.
            if (isCancelled() || isForceCancelled()) {  // T117
                return { error: 'cancelled by user' };
            }
            // Check pause — save checkpoint and exit cleanly.
            if (isPaused()) {
                if (state.planCache) {
                    var lastDone = '';
                    for (var si = report.steps.length - 1; si >= 0; si--) {
                        if (!report.steps[si].error) {
                            lastDone = report.steps[si].name;
                            break;
                        }
                    }
                    savePlan(resolvedPlanPath, lastDone || 'paused');
                    emitOutput('[auto-split] Paused — checkpoint saved to ' + resolvedPlanPath);
                    emitOutput('[auto-split] Resume with: osm pr-split --resume');
                }
                return { error: 'paused by user (Ctrl-P)' };
            }
            // Check pipeline timeout before starting each step.
            var pipelineElapsed = Date.now() - pipelineStartTime;
            if (pipelineElapsed >= pipelineTimeoutMs) {
                return { error: 'pipeline timeout (' + Math.round(pipelineElapsed / 60000) + 'min elapsed, limit ' + Math.round(pipelineTimeoutMs / 60000) + 'min)' };
            }
            // Check watchdog — no progress for too long.
            var idleTime = Date.now() - lastProgressTime;
            if (idleTime >= watchdogIdleMs) {
                var msg = 'watchdog timeout: no progress for ' + Math.round(idleTime / 60000) + ' minutes';
                log.printf('auto-split: %s', msg);
                return { error: msg };
            }

            var t0 = Date.now();
            lastProgressTime = Date.now();
            emitOutput('[auto-split] ' + name + '...');
            log.printf('auto-split step: %s', name);
            var result;
            try {
                var fnResult = fn();
                if (fnResult && typeof fnResult.then === 'function') {
                    result = await fnResult;
                } else {
                    result = fnResult;
                }
            } catch (e) {
                result = { error: e.message || String(e) };
            }
            // Normalize null/undefined returns from step callbacks.
            if (result == null) {
                result = {};
            }
            var elapsed = Date.now() - t0;

            // Per-step timeout check (for long-running synchronous steps).
            if (!result.error && elapsed >= stepTimeoutMs) {
                result = { error: 'step timeout (' + Math.round(elapsed / 60000) + 'min elapsed, limit ' + Math.round(stepTimeoutMs / 60000) + 'min)' };
            }

            lastProgressTime = Date.now();
            report.steps.push({ name: name, elapsedMs: elapsed, error: result.error || null });
            if (result.error) {
                emitOutput('[auto-split] ' + name + ' FAILED (' + elapsed + 'ms): ' + result.error);
            } else {
                emitOutput('[auto-split] ' + name + ' OK (' + elapsed + 'ms)');
            }
            return result;
        }

        // finishTUI signals the auto-split TUI is done.
        function finishTUI(result) {
            // T393: Only clean up MCP callback on error — keep alive for "Ask
            // Claude" conversation overlay on PLAN_REVIEW/ERROR_RESOLUTION.
            // On success, the wizard's quit handler handles cleanup.
            if (result.error) {
                var mcpCb = prSplit._mcpCallbackObj;
                if (mcpCb) {
                    try { mcpCb.closeSync(); } catch (e) { log.debug('cleanup: mcpCb.closeSync failed: ' + (e.message || e)); }
                    prSplit._mcpCallbackObj = null;
                    state.mcpCallbackObj = null;
                }
            }

            // On error, emit resume instructions if a plan was saved.
            if (result.error && state.planCache && state.planCache.splits && state.planCache.splits.length > 0) {
                try {
                    savePlan(resolvedPlanPath, report.lastCompletedStep || 'error');
                } catch (e) { log.debug('cleanup: savePlan failed: ' + (e.message || e)); }

                emitOutput('\n[auto-split] Pipeline failed: ' + result.error);
                emitOutput('[auto-split] Plan saved to: ' + resolvedPlanPath);
                emitOutput('[auto-split] To resume: osm pr-split --resume\n');
            }

            if (hasTUI && !config.disableTUI) {
                emitOutput('\n[auto-split] ' + (result.error ? ('Error: ' + result.error) : 'Complete'));
            }
            return result;
        }

        // Resume support: skip Steps 1-6 if resuming from a saved plan.
        var resuming = !!config.resumeFromPlan;
        if (resuming) {
            var loadResult = loadPlan(config.resumePlanPath);
            if (loadResult.error) {
                report.error = 'Resume failed: ' + loadResult.error;
                return finishTUI({ error: report.error, report: report });
            }
            emitOutput('[auto-split] Resumed from saved plan (' + loadResult.totalSplits +
                ' splits, ' + loadResult.executedSplits + ' already executed)');
        }

        // Step 1: Analyze diff.
        var analysis;
        if (!resuming) {
        analysis = await step('Analyze diff', async function() {
            updateDetail('Analyze diff', 'Reading git diff...');
            var result = await analyzeDiff(config);
            if (!result.error && result.files) {
                updateDetail('Analyze diff', result.files.length + ' files found');
            }
            return result;
        });
        if (analysis.error) {
            report.error = analysis.error;
            return finishTUI({ error: analysis.error, report: report });
        }
        if (analysis.files.length === 0) {
            report.error = 'No changes detected';
            return finishTUI({ error: report.error, report: report });
        }
        recordTelemetry('filesAnalyzed', analysis.files.length);
        // T110: Cache analysis so pipeline resume skips re-analysis.
        state.analysisCache = analysis;
        } else {
            analysis = state.analysisCache || { files: [], fileStatuses: {}, baseBranch: '', currentBranch: '' };
        }

        // Initialize MCP callback transport for Claude IPC.
        var mcpCallbackObj;
        try {
            var mcpMod = require('osm:mcp');
            var MCPCallbackMod = require('osm:mcpcallback');
            var srv = mcpMod.createServer('pr-split-callback', '1.0.0');
            mcpCallbackObj = MCPCallbackMod.MCPCallback({ server: srv });
            state.mcpCallbackObj = mcpCallbackObj;
            prSplit._mcpCallbackObj = mcpCallbackObj;

            // Register reporting tools.
            mcpCallbackObj.addTool('reportClassification',
                'Report file classification for PR splitting. Provide categories array with name, description (commit message), and files.',
                {
                    type: 'object',
                    properties: {
                        categories: {
                            type: 'array',
                            items: {
                                type: 'object',
                                properties: {
                                    name: { type: 'string', description: 'Category name (e.g., types, impl, docs)' },
                                    description: { type: 'string', description: 'Git commit message for the split branch. Must be specific to actual changes.' },
                                    files: { type: 'array', items: { type: 'string' }, description: 'File paths belonging to this category' }
                                },
                                required: ['name', 'description', 'files']
                            },
                            description: 'Array of categories, each grouping related files'
                        }
                    },
                    required: ['categories']
                });

            mcpCallbackObj.addTool('reportSplitPlan',
                'Report a split plan for PR splitting. Optional — if not provided, plan is generated locally from classification.',
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
                                    order: { type: 'number', description: 'Execution order' }
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

            // Heartbeat tool — Claude calls periodically to signal liveness.
            // Heartbeat timeout: if Claude has sent at least one heartbeat but
            // then goes silent for longer than this, waitForLogged aborts.
            // Default: claudeHeartbeatTimeoutMs from AUTOMATED_DEFAULTS (60s).
            var heartbeatTimeoutMs = config.heartbeatTimeoutMs || AUTOMATED_DEFAULTS.claudeHeartbeatTimeoutMs;
            mcpCallbackObj.addTool('heartbeat',
                'Send a heartbeat to indicate Claude is still actively working. Call this periodically during long-running analysis.',
                {
                    type: 'object',
                    properties: {
                        status: { type: 'string', description: 'Optional status message (e.g., "analyzing file X")' }
                    }
                });

            mcpCallbackObj.initSync();
            log.printf('auto-split: MCP callback initialized at %s (%s)', mcpCallbackObj.address, mcpCallbackObj.transport);
            log.printf('auto-split: MCP config path: %s', mcpCallbackObj.mcpConfigPath);
            try {
                var readResult = osmod.readFile(mcpCallbackObj.mcpConfigPath);
                log.printf('auto-split: MCP config contents: %s', (!readResult.error && readResult.content) ? readResult.content : '(empty)');
            } catch (e) {
                log.printf('auto-split: could not read MCP config: %s', e.message || String(e));
            }
        } catch (e) {
            return finishTUI({ error: 'MCP callback initialization failed: ' + (e.message || String(e)), report: report });
        }

        // Steps 2-6 are skipped when resuming from a saved plan.
        var claudeExecutor;
        var sessionId;
        var aliveCheckFn;

        if (!resuming) {

        // Step 2: Spawn Claude (or fall back to heuristic).
        var executor = await step('Spawn Claude', async function() {
            updateDetail('Spawn Claude', 'Resolving Claude executable...');
            claudeExecutor = state.claudeExecutor;
            if (!claudeExecutor) {
                claudeExecutor = new ClaudeCodeExecutor(prSplitConfig);
                state.claudeExecutor = claudeExecutor;
                prSplit._claudeExecutor = claudeExecutor;
            }
            var resolveResult = await claudeExecutor.resolveAsync(function(msg) {
                updateDetail('Spawn Claude', msg);
            });
            if (resolveResult.error) {
                return { error: resolveResult.error };
            }
            updateDetail('Spawn Claude', 'Starting Claude process...');
            var spawnOpts = {};
            spawnOpts.mcpConfigPath = mcpCallbackObj.mcpConfigPath;
            var spawnResult = await claudeExecutor.spawn(null, spawnOpts);
            if (spawnResult.error) {
                return { error: spawnResult.error };
            }
            return { error: null, sessionId: spawnResult.sessionId };
        });

        // If Claude is unavailable, fall back to heuristic mode.
        if (executor.error) {
            emitOutput('[auto-split] Claude unavailable — falling back to heuristic mode.');
            report.fallbackUsed = true;
            return finishTUI(await heuristicFallback(analysis, config, report));
        }

        sessionId = executor.sessionId;

        // Heartbeat function: checks if the Claude process is still alive.
        // Also checks for crash detection flag set by the TUI health poll.
        aliveCheckFn = function() {
            // T106: claudeCrashDetected is only set by the TUI health monitor.
            // In headless mode (no tuiMux), ignore the flag to avoid false
            // positives from uninitialized state.
            if (tuiMux && state.claudeCrashDetected) {
                return false;
            }
            if (!claudeExecutor || !claudeExecutor.handle ||
                typeof claudeExecutor.handle.isAlive !== 'function' ||
                !claudeExecutor.handle.isAlive()) {
                return false;
            }
            lastProgressTime = Date.now();
            return true;
        };

        // Attach Claude's PTY handle to tuiMux so ctrl+] can forward.
        if (claudeExecutor && claudeExecutor.handle && typeof tuiMux !== 'undefined' && tuiMux) {
	            if (typeof claudeExecutor.handle.isAlive === 'function' && !claudeExecutor.handle.isAlive()) {
	                log.printf('auto-split: Claude process died between spawn and attach — ctrl+] will not work');
	                emitOutput('[auto-split] Warning: Claude process exited unexpectedly. Toggle (Ctrl+]) unavailable.');
	            } else {
	                try {
	                    tuiMux.attach(claudeExecutor.handle);
	                    log.printf('auto-split: attached Claude handle to tuiMux for ctrl+] toggle');
	                } catch (e) {
	                    log.printf('auto-split: tuiMux attach warning: %s', e.message || String(e));
	                }
            }
        } else if (claudeExecutor && claudeExecutor.handle &&
                   typeof claudeExecutor.handle.drainOutput === 'function') {
            // No tuiMux (headless/test mode): drain Claude's PTY output to
            // prevent buffer deadlocks.
            claudeExecutor.handle.drainOutput();
            log.printf('auto-split: no tuiMux — started PTY output drain to prevent deadlock');
        }

        // Step 2b: Dismiss Ollama launcher menu if present.
        // Ollama shows a "Run a model" launcher screen before the model
        // selection menu.  We detect it via tuiMux.screenshot() and send
        // the appropriate dismissal keystrokes.  For non-Ollama providers
        // this block is a no-op.
        if (claudeExecutor && claudeExecutor.resolved &&
            claudeExecutor.resolved.type === 'ollama' &&
            claudeExecutor.handle && claudeExecutor.cm) {
            var launcherResult = await step('Dismiss launcher', async function() {
                var LAUNCHER_POLL_MS     = AUTOMATED_DEFAULTS.launcherPollMs;
                var LAUNCHER_TIMEOUT_MS  = AUTOMATED_DEFAULTS.launcherTimeoutMs;
                var LAUNCHER_STABLE_NEED = AUTOMATED_DEFAULTS.launcherStableNeed;
                var cm = claudeExecutor.cm;
                var handle = claudeExecutor.handle;
                var startMs = Date.now();
                var stableCount = 0;
                var dismissed = false;

                while (Date.now() - startMs < LAUNCHER_TIMEOUT_MS) {
                    var cancelErr = getCancellationError();
                    if (cancelErr) { return { error: cancelErr }; }

                    var shot = captureScreenshot();
                    if (!shot) {
                        // tuiMux not available — nothing to dismiss.
                        log.printf('auto-split launcher: no screenshot available, skipping');
                        return { error: null, dismissed: false };
                    }

                    var lines = shot.split('\n');
                    var menu;
                    try { menu = cm.parseModelMenu(lines); }
                    catch (e) {
                        log.printf('auto-split launcher: parseModelMenu error: %s', e.message || String(e));
                        await new Promise(function(r) { setTimeout(r, LAUNCHER_POLL_MS); });
                        continue;
                    }

                    if (!menu || !menu.models || menu.models.length === 0) {
                        // No menu detected yet — screen might still be loading.
                        stableCount++;
                        if (stableCount >= LAUNCHER_STABLE_NEED) {
                            // No menu after stable polls — not a menu-based provider.
                            log.printf('auto-split launcher: no menu detected after %d stable polls, proceeding', stableCount);
                            return { error: null, dismissed: false };
                        }
                        await new Promise(function(r) { setTimeout(r, LAUNCHER_POLL_MS); });
                        continue;
                    }

                    // Reset stable counter — we have menu content.
                    stableCount = 0;

                    if (cm.isLauncherMenu(menu)) {
                        var keys = cm.dismissLauncherKeys(menu);
                        if (keys) {
                            log.printf('auto-split launcher: detected launcher menu (%d items), dismissing',
                                menu.models.length);
                            try { handle.send(keys); }
                            catch (e) {
                                return { error: 'failed to dismiss launcher: ' + (e.message || String(e)) };
                            }
                            // Wait briefly for screen to update after dismissal.
                            await new Promise(function(r) { setTimeout(r, AUTOMATED_DEFAULTS.launcherPostDismissMs); });
                            dismissed = true;
                            continue;  // Re-poll to check for model selection menu.
                        }
                    }

                    // Not a launcher menu — might be model selection.
                    // If we have a target model, navigate to it.
                    if (claudeExecutor.model && !dismissed) {
                        try {
                            var navKeys = cm.navigateToModel(menu, claudeExecutor.model);
                            if (navKeys) {
                                log.printf('auto-split launcher: navigating to model %s', claudeExecutor.model);
                                handle.send(navKeys);
                                await new Promise(function(r) { setTimeout(r, AUTOMATED_DEFAULTS.launcherPostDismissMs); });
                            }
                        } catch (e) {
                            log.printf('auto-split launcher: model navigation error: %s — proceeding with selected model',
                                e.message || String(e));
                        }
                    }

                    // Menu is present but not a launcher — we're past the
                    // launcher stage (or it was never shown).  Done.
                    log.printf('auto-split launcher: menu resolved (dismissed=%s)', String(dismissed));
                    return { error: null, dismissed: dismissed };
                }

                // Timeout — proceed anyway (best effort).
                log.printf('auto-split launcher: timeout after %dms — proceeding', LAUNCHER_TIMEOUT_MS);
                return { error: null, dismissed: false };
            });
            if (launcherResult.error) {
                report.error = launcherResult.error;
                cleanupExecutor();
                return finishTUI({ error: launcherResult.error, report: report });
            }
        }

        // Step 3: Send classification request.
        var classifyResult = await step('Send classification request', async function() {
            updateDetail('Send classification request', 'Rendering prompt (' + analysis.files.length + ' files)...');
            var renderResult = renderClassificationPrompt(analysis, {
                maxGroups: config.maxGroups || 0
            });
            if (renderResult.error) {
                return { error: renderResult.error };
            }
            updateDetail('Send classification request', 'Sending prompt to Claude...');
            var sendResult = await sendToHandle(claudeExecutor.handle, renderResult.text);
            if (sendResult.error) {
                return { error: 'failed to send prompt to Claude: ' + sendResult.error };
            }
            report.claudeInteractions++;
            recordConversation('classification', renderResult.text, '');
            recordTelemetry('claudeInteractions', 1);
            return { error: null };
        });
        if (classifyResult.error) {
            report.error = classifyResult.error;
            cleanupExecutor();
            return finishTUI({ error: classifyResult.error, report: report });
        }

        // Step 4: Receive classification.
        var classification = await step('Receive classification', async function() {
            updateDetail('Receive classification', 'Waiting for classification...');
            var pollResult = await waitForLogged('reportClassification', timeouts.classify, {
                aliveCheck: aliveCheckFn,
                heartbeatTool: 'heartbeat',
                heartbeatTimeoutMs: heartbeatTimeoutMs,
                onProgress: function(elapsed) {
                    var sec = Math.round(elapsed / 1000);
                    updateDetail('Receive classification', 'Waiting... ' + sec + 's');
                },
                checkIntervalMs: pollInterval
            });
            // Extract categories from full tool arguments.
            if (!pollResult.error && pollResult.data) {
                pollResult = { data: pollResult.data.categories || pollResult.data, error: null };
            }
            if (pollResult.error) {
                return { error: pollResult.error };
            }
            var classMap = pollResult.data;
            updateDetail('Receive classification', 'Validating ' + analysis.files.length + ' file classifications...');

            // Structural validation.
            if (Array.isArray(classMap)) {
                var valResult = validateClassification(classMap, analysis.files);
                if (!valResult.valid) {
                    log.printf('auto-split: classification validation errors: %s', valResult.errors.join('; '));
                }
            }

            // Build file lookup for both formats.
            var fileIsClassified;
            if (Array.isArray(classMap)) {
                var fileSet = {};
                for (var ci = 0; ci < classMap.length; ci++) {
                    var catFiles = classMap[ci].files || [];
                    for (var fi = 0; fi < catFiles.length; fi++) {
                        fileSet[catFiles[fi]] = true;
                    }
                }
                fileIsClassified = function(path) { return !!fileSet[path]; };
            } else {
                fileIsClassified = function(path) { return !!classMap[path]; };
            }

            var missing = [];
            for (var i = 0; i < analysis.files.length; i++) {
                if (!fileIsClassified(analysis.files[i])) {
                    missing.push(analysis.files[i]);
                }
            }
            if (missing.length > 0) {
                log.printf('auto-split: %d files not classified: %s', missing.length, missing.join(', '));
                if (Array.isArray(classMap)) {
                    classMap.push({ name: 'uncategorized', description: 'Uncategorized changes', files: missing });
                } else {
                    for (var j = 0; j < missing.length; j++) {
                        classMap[missing[j]] = 'uncategorized';
                    }
                }
            }
            report.classification = classMap;
            recordConversation('classification-result', '', JSON.stringify(classMap));
            return { error: null, classification: classMap };
        });
        if (classification.error) {
            report.error = classification.error;
            cleanupExecutor();
            return finishTUI({ error: classification.error, report: report });
        }

        // Step 5: Generate plan (from Claude or locally).
        var planResult = await step('Generate split plan', async function() {
            updateDetail('Generate split plan', 'Checking for Claude-generated plan...');
            var planPoll = await waitForLogged('reportSplitPlan', AUTOMATED_DEFAULTS.planPollTimeoutMs, {
                aliveCheck: aliveCheckFn,
                heartbeatTool: 'heartbeat',
                heartbeatTimeoutMs: heartbeatTimeoutMs,
                checkIntervalMs: AUTOMATED_DEFAULTS.planPollCheckIntervalMs
            });
            // Extract stages from full tool arguments.
            if (!planPoll.error && planPoll.data) {
                planPoll = { data: planPoll.data.stages || planPoll.data, error: null };
            }
            if (!planPoll.error && planPoll.data) {
                var claudePlan = planPoll.data;
                if (Array.isArray(claudePlan) && claudePlan.length > 0) {
                    var stageVal = validateSplitPlan(claudePlan);
                    if (!stageVal.valid) {
                        log.printf('auto-split: Claude split plan stage validation errors: %s', stageVal.errors.join('; '));
                    }
                    report.claudeInteractions++;
                    var plan = {
                        baseBranch: analysis.baseBranch,
                        sourceBranch: analysis.currentBranch,
                        dir: '.',
                        verifyCommand: runtime.verifyCommand,
                        fileStatuses: analysis.fileStatuses || {},
                        splits: claudePlan.map(function(s, i) {
                            return {
                                name: s.name || (runtime.branchPrefix + padIndex(i, claudePlan.length)),
                                files: s.files || [],
                                message: s.message || ('Split ' + (i + 1)),
                                order: typeof s.order === 'number' ? s.order : i
                            };
                        })
                    };
                    var validation = validatePlan(plan, analysis.files);
                    if (validation.valid) {
                        report.plan = plan;
                        return { error: null, plan: plan };
                    }
                    log.printf('auto-split: Claude plan invalid: %s — generating locally', validation.errors.join('; '));
                }
            }

            // Generate plan locally from classification.
            var groups = classificationToGroups(classification.classification);
            var plan = await createSplitPlan(groups, {
                baseBranch: analysis.baseBranch,
                sourceBranch: analysis.currentBranch,
                branchPrefix: runtime.branchPrefix,
                maxFiles: runtime.maxFiles,
                fileStatuses: analysis.fileStatuses
            });
            report.plan = plan;
            return { error: null, plan: plan };
        });
        if (planResult.error) {
            report.error = planResult.error;
            cleanupExecutor();
            return finishTUI({ error: planResult.error, report: report });
        }

        var plan = planResult.plan;
        state.planCache = plan;

        // Checkpoint after plan generation.
        savePlan(null, 'Generate split plan');

        // Step 6: Execute split.
        var execResult = await step('Execute split plan', async function() {
            if (runtime.dryRun) {
                return { error: null, dryRun: true };
            }
            updateDetail('Execute split plan', plan.splits.length + ' branches to create...');
            var result = await executeSplit(plan, {
                progressFn: function(msg) { updateDetail('Execute split plan', msg); }
            });
            if (result.error) {
                return { error: result.error };
            }
            report.splits = result.results || [];
            updateDetail('Execute split plan', report.splits.length + ' branches created');
            return { error: null };
        });
        if (execResult.error) {
            report.error = execResult.error;
            if (config.cleanupOnFailure && plan && plan.splits && plan.splits.length > 0) {
                emitOutput('[auto-split] Cleaning up split branches due to execution failure...');
                var cleanResult = await cleanupBranches(plan);
                if (cleanResult.deleted.length > 0) {
                    emitOutput('[auto-split] Deleted ' + cleanResult.deleted.length + ' branches');
                }
            }
            cleanupExecutor();
            return finishTUI({ error: execResult.error, report: report });
        }
        if (runtime.dryRun) {
            emitOutput('[auto-split] Dry run — skipping verification.');
            cleanupExecutor();
            // T393: Dry-run has no interactive session after — clean up MCP
            // callback inline since finishTUI only cleans on error.
            var mcpCbDry = prSplit._mcpCallbackObj;
            if (mcpCbDry) {
                try { mcpCbDry.closeSync(); } catch (e) { log.debug('cleanup: mcpCb.closeSync failed (dry-run): ' + (e.message || e)); }
                prSplit._mcpCallbackObj = null;
                state.mcpCallbackObj = null;
            }
            return finishTUI({ error: null, report: report });
        }

        // Persist plan for crash recovery / resume.
        var saveResult = savePlan(null, 'Execute split plan');
        if (saveResult.error) {
            log.printf('auto-split: save plan warning: %s', saveResult.error);
        } else {
            log.printf('auto-split: plan saved to %s', saveResult.path);
        }

        } else {
            // Resume path: set variables from loaded cache.
            classification = { classification: state.groupsCache || {} };
            plan = state.planCache;
            sessionId = null;
            aliveCheckFn = null;

            // Try to spawn Claude for conflict resolution capability.
            claudeExecutor = state.claudeExecutor;
            if (!claudeExecutor) {
                claudeExecutor = new ClaudeCodeExecutor(prSplitConfig);
                state.claudeExecutor = claudeExecutor;
                prSplit._claudeExecutor = claudeExecutor;
            }
            var resumeResolve = await claudeExecutor.resolveAsync();
            if (!resumeResolve.error) {
                var resumeSpawn = await claudeExecutor.spawn(null, { mcpConfigPath: mcpCallbackObj.mcpConfigPath });
                if (!resumeSpawn.error) {
                    sessionId = resumeSpawn.sessionId;
                    aliveCheckFn = function() {
                        // T106: Guard with tuiMux (headless safety).
                        if (tuiMux && state.claudeCrashDetected) {
                            return false;
                        }
                        return claudeExecutor && claudeExecutor.handle &&
                               typeof claudeExecutor.handle.isAlive === 'function' &&
                               claudeExecutor.handle.isAlive();
                    };
                } else {
                    emitOutput('[auto-split] Warning: Claude spawn failed — conflict resolution disabled.');
                }
            } else {
                emitOutput('[auto-split] Claude unavailable — conflict resolution disabled for resume.');
            }
        }

        // Step 7: Verify splits.
        var verifyResult = await step('Verify splits', async function() {
            updateDetail('Verify splits', 'Running verification command on each branch...');
            var verifyObj = await verifySplits(plan, {
                verifyTimeoutMs: config.verifyTimeoutMs || AUTOMATED_DEFAULTS.verifyTimeoutMs,
                outputFn: emitOutput,
                onBranchStart: null,
                onBranchDone: null,
                onBranchOutput: null
            });
            var realFailures = [];
            var skippedResults = [];
            var preExistingResults = [];
            for (var i = 0; i < verifyObj.results.length; i++) {
                var r = verifyObj.results[i];
                if (r.skipped) {
                    skippedResults.push(r);
                } else if (r.preExisting) {
                    preExistingResults.push(r);
                } else if (!r.passed) {
                    realFailures.push(r);
                }
            }
            if (skippedResults.length > 0) {
                var skippedNames = [];
                for (var j = 0; j < skippedResults.length; j++) {
                    skippedNames.push(skippedResults[j].name);
                }
                output.print('[auto-split] Skipped ' + skippedResults.length +
                    ' branches due to dependency failures: ' + skippedNames.join(', '));
            }
            if (preExistingResults.length > 0) {
                var preExNames = [];
                for (var p = 0; p < preExistingResults.length; p++) {
                    preExNames.push(preExistingResults[p].name);
                }
                output.print('[auto-split] ' + preExistingResults.length +
                    ' branch(es) have pre-existing failures: ' + preExNames.join(', '));
            }
            report.skippedDueToDepFailure = skippedResults;
            report.preExistingFailures = preExistingResults;
            if (realFailures.length > 0) {
                var failNames = [];
                for (var fi = 0; fi < realFailures.length; fi++) {
                    failNames.push(realFailures[fi].name || ('branch-' + fi));
                }
                return {
                    error: realFailures.length + ' branch(es) failed verification: ' + failNames.join(', '),
                    failures: realFailures,
                    allPassed: false
                };
            }
            return { error: null, failures: [], allPassed: verifyObj.allPassed };
        });

        // Checkpoint after verify.
        savePlan(null, 'Verify splits');

        // Step 8: Resolve conflicts (if any real failures).
        var reSplitCount = 0;
        if (verifyResult.failures && verifyResult.failures.length > 0) {
            var resolved = await step('Resolve conflicts via Claude', async function() {
                return await resolveConflictsWithClaude(
                    verifyResult.failures, sessionId,
                    timeouts, pollInterval, maxAttemptsPerBranch, report, aliveCheckFn,
                    heartbeatTimeoutMs
                );
            });

            // Step 9: Re-split fallback if needed.
            if (resolved.reSplitNeeded && reSplitCount < maxReSplits) {
                reSplitCount++;
                output.print('[auto-split] Re-split requested — re-classifying...');
                await cleanupBranches(plan);
                var reClassifyResult = await step('Re-classify (retry ' + reSplitCount + ')', async function() {
                    var constraintPrompt = 'Re-classify these files with the constraint: ' +
                        resolved.reSplitReason + '\n\nUse the reportClassification MCP tool.\n';
                    var sendResult = await sendToHandle(claudeExecutor.handle, constraintPrompt);
                    if (sendResult.error) {
                        return { error: 'failed to send re-classify prompt: ' + sendResult.error };
                    }
                    report.claudeInteractions++;
                    recordConversation('re-classify', constraintPrompt, '');
                    mcpCallbackObj.resetWaiter('reportClassification');
                    var rePoll = await waitForLogged('reportClassification', timeouts.classify, {
                        aliveCheck: aliveCheckFn,
                        heartbeatTool: 'heartbeat',
                        heartbeatTimeoutMs: heartbeatTimeoutMs,
                        onProgress: function(elapsed) {
                            updateDetail('Re-classify (retry ' + reSplitCount + ')', 'Waiting... ' + Math.round(elapsed / 1000) + 's');
                        },
                        checkIntervalMs: pollInterval
                    });
                    if (!rePoll.error && rePoll.data) {
                        rePoll = { data: rePoll.data.categories || rePoll.data, error: null };
                    }
                    if (rePoll.error) {
                        return { error: rePoll.error };
                    }
                    return { error: null, classification: rePoll.data };
                });
                if (!reClassifyResult.error) {
                    var newGroups = classificationToGroups(reClassifyResult.classification);
                    plan = await createSplitPlan(newGroups, {
                        baseBranch: analysis.baseBranch,
                        sourceBranch: analysis.currentBranch,
                        branchPrefix: runtime.branchPrefix,
                        maxFiles: runtime.maxFiles,
                        fileStatuses: analysis.fileStatuses
                    });
                    state.planCache = plan;
                    report.plan = plan;
                    var reExec = await step('Re-execute split', async function() {
                        var result = await executeSplit(plan);
                        if (result.error) return { error: result.error };
                        report.splits = result.results || [];
                        return { error: null };
                    });
                    if (!reExec.error) {
                        await step('Re-verify splits', async function() {
                            // T104: Actually check whether re-verified branches pass.
                            // Previously this always returned { error: null }.
                            var rv = await verifySplits(plan, {
                                verifyTimeoutMs: config.verifyTimeoutMs || AUTOMATED_DEFAULTS.verifyTimeoutMs,
                                outputFn: emitOutput
                            });
                            if (rv.error) return { error: rv.error };
                            var reFails = [];
                            if (rv.results) {
                                for (var ri = 0; ri < rv.results.length; ri++) {
                                    var r = rv.results[ri];
                                    if (!r.passed && !r.skipped && !r.preExisting) {
                                        reFails.push(r.name);
                                    }
                                }
                            }
                            if (reFails.length > 0) {
                                return { error: reFails.length + ' branch(es) still fail after re-split: ' + reFails.join(', ') };
                            }
                            return { error: null };
                        });
                    }
                }
            }

            // Checkpoint after resolve/re-split.
            savePlan(null, 'Resolve conflicts');
        }

        // Step 10: Equivalence check and report.
        var equivResult = await step('Verify equivalence', async function() {
            var result = await verifyEquivalence(plan);
            return { error: result.equivalent ? null : 'tree hash mismatch', result: result };
        });

        // T121: Propagate equivalence result to report so the TUI can
        // transition from BRANCH_BUILDING → EQUIV_CHECK → FINALIZATION.
        if (equivResult && equivResult.result) {
            report.equivalence = equivResult.result;
        }

        // Assess independence.
        report.independencePairs = assessIndependence(plan, classification.classification || {});

        // Summary.
        emitOutput('');
        emitOutput('=== Auto-Split Complete ===');
        emitOutput('Splits: ' + plan.splits.length);
        emitOutput('Claude interactions: ' + report.claudeInteractions);
        emitOutput('Equivalence: ' + (equivResult.result && equivResult.result.equivalent ? 'PASS' : 'FAIL'));
        if (report.independencePairs.length > 0) {
            emitOutput('Independent pairs: ' + report.independencePairs.map(function(p) {
                return p[0] + ' + ' + p[1];
            }).join(', '));
        }
        if (report.fallbackUsed) {
            emitOutput('Mode: heuristic (Claude unavailable)');
        }

        // T393: Keep Claude alive for "Ask Claude" on PLAN_REVIEW.
        // Only clean up on error — the wizard's quit handler (confirmCancel)
        // handles cleanup on success/cancel paths, and Go context cancellation
        // handles cleanup on process exit.
        if (report.error) {
            cleanupExecutor();
        }

        // Clean up split branches on pipeline failure if configured.
        if (config.cleanupOnFailure && report.error && plan && plan.splits && plan.splits.length > 0) {
            emitOutput('[auto-split] Cleaning up split branches due to pipeline failure...');
            var cleanResult = await cleanupBranches(plan);
            if (cleanResult.deleted.length > 0) {
                emitOutput('[auto-split] Deleted ' + cleanResult.deleted.length + ' branches');
            }
        }

        // Worktree isolation: user's branch is never modified, no restore needed.

        return finishTUI({ error: report.error, report: report });
    }

    // Export.
    prSplit.automatedSplit = automatedSplit;
})(globalThis.prSplit);
