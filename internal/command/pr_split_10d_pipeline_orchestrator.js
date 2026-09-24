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
        var AgentCodeExecutor = prSplit.AgentCodeExecutor;
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
        var resolveConflictsWithAgent = prSplit.resolveConflictsWithAgent;

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
            agentInteractions: 0,
            conflictsResolved: 0,
            conflictsFailed: 0,
            startTime: new Date().toISOString(),
            endTime: null
        };
        state.agentExecutor = null;
        state.agentSessionID = null;
        state.mcpCallbackObj = null;
        state.analysisCache = null;
        state.groupsCache = null;
        state.planCache = null;
        state.executionResultCache = [];
        // T089: Clear cached equivalence result so stale results from a
        // previous pipeline run are not used by buildReport().
        state.equivalenceResult = null;
        // Also reset the prSplit-level pointers used by other chunks.
        prSplit._agentExecutor = null;
        prSplit._mcpCallbackObj = null;

        var timeouts = {
            classify: typeof config.classifyTimeoutMs === 'number' ? config.classifyTimeoutMs : AUTOMATED_DEFAULTS.classifyTimeoutMs,
            plan: typeof config.planTimeoutMs === 'number' ? config.planTimeoutMs : AUTOMATED_DEFAULTS.planTimeoutMs,
            resolve: typeof config.resolveTimeoutMs === 'number' ? config.resolveTimeoutMs : AUTOMATED_DEFAULTS.resolveTimeoutMs,
            commandMs: typeof config.resolveCommandTimeoutMs === 'number' ? config.resolveCommandTimeoutMs : AUTOMATED_DEFAULTS.resolveCommandTimeoutMs
        };
        var pollInterval = typeof config.pollIntervalMs === 'number' ? config.pollIntervalMs : AUTOMATED_DEFAULTS.pollIntervalMs;
        // Use typeof check: 0 is a valid value for retry/re-split counts (meaning "none").
        var maxAttemptsPerBranch = typeof config.maxResolveRetries === 'number' ? config.maxResolveRetries : AUTOMATED_DEFAULTS.maxResolveRetries;
        var maxReSplits = typeof config.maxReSplits === 'number' ? config.maxReSplits : AUTOMATED_DEFAULTS.maxReSplits;

        // Clamp: negative values must not cause spin-loops or nonsensical retries.
        if (pollInterval < AUTOMATED_DEFAULTS.minPollIntervalMs) { pollInterval = AUTOMATED_DEFAULTS.minPollIntervalMs; }
        if (maxReSplits < 0) { maxReSplits = 0; }
        if (maxAttemptsPerBranch < 0) { maxAttemptsPerBranch = 0; }

        // Pipeline-level timeout and watchdog.
        var pipelineTimeoutMs = typeof config.pipelineTimeoutMs === 'number' ? config.pipelineTimeoutMs : AUTOMATED_DEFAULTS.pipelineTimeoutMs;
        var stepTimeoutMs = typeof config.stepTimeoutMs === 'number' ? config.stepTimeoutMs : AUTOMATED_DEFAULTS.stepTimeoutMs;
        var watchdogIdleMs = typeof config.watchdogIdleMs === 'number' ? config.watchdogIdleMs : AUTOMATED_DEFAULTS.watchdogIdleMs;
        var pipelineStartTime = Date.now();
        var lastProgressTime = Date.now();

        // Detect the active Goja/BubbleTea wizard. Output is routed through
        // the model while the fullscreen wizard owns the terminal.
        var hasTUI = !!prSplit._tuiOutputActive;

        var report = {
            mode: 'automated',
            steps: [],
            classification: null,
            plan: null,
            splits: [],
            conflicts: [],
            resolutions: [],
            independencePairs: [],
            agentInteractions: 0,
            fallbackUsed: false,
            error: null
        };

        function emitOutput(text) {
            if (hasTUI && typeof prSplit._routeTuiOutput === 'function') {
                prSplit._routeTuiOutput(text);
            } else {
                output.print(text);
            }
            lastProgressTime = Date.now();
        }

        function updateDetail(stepName, detail) {
            // Placeholder: TUI detail display removed in T27 migration.
            // detail is logged for diagnostics.
            log.printf('auto-split detail [%s]: %s', stepName, detail);
        }

        // Transcript dir: production log dir comes from the Go layer via
        // prSplitConfig.transcriptDir (same storage session dir family as
        // persistStatePath); tests override via config.transcriptDir; the
        // repo dir is the last resort for headless-no-storage runs.
        function transcriptDir() {
            try {
                if (config && config.transcriptDir) return String(config.transcriptDir);
            } catch (e) {
                log.debug('transcript dir config read failed', { error: e.message || String(e) });
            }
            try {
                if (typeof prSplitConfig !== 'undefined' && prSplitConfig && prSplitConfig.transcriptDir) {
                    return String(prSplitConfig.transcriptDir);
                }
            } catch (e) {
                log.debug('transcript dir injected read failed', { error: e.message || String(e) });
            }
            return dir;
        }

        async function captureAgentEvidence(reason) {
            var evidence = {
                reason: reason || 'classification-timeout',
                at: new Date().toISOString(),
                sessionId: state.agentSessionID || null,
                argv: null,
                mcpAddress: null,
                mcpTransport: null,
                screenPlain: '',
                screenAnsi: '',
                lastActivityMs: -1,
                handleAlive: false,
                handleHealth: null,
                heartbeatMs: 0,
                transcriptPath: ''
            };
            try {
                if (state.agentExecutor && state.agentExecutor.handle &&
                    typeof state.agentExecutor.handle.isAlive === 'function') {
                    evidence.handleAlive = !!state.agentExecutor.handle.isAlive();
                }
            } catch (e) {
                log.debug('evidence handle alive check failed', { error: e.message || String(e) });
            }
            try {
                if (state.agentExecutor && state.agentExecutor.handle &&
                    typeof state.agentExecutor.handle.health === 'function') {
                    evidence.handleHealth = state.agentExecutor.handle.health();
                }
            } catch (e) {
                log.debug('evidence handle health read failed', { error: e.message || String(e) });
            }
            // Capture-before-unregister: CaptureScreen fails after Unregister
            // (manager.go:364-380) and isDone is true for unknown IDs, so this
            // read happens while the session is live. No unregister runs on the
            // preserve path.
            try {
                if (typeof tuiMux !== 'undefined' && tuiMux && state.agentSessionID &&
                    typeof tuiMux.capture === 'function') {
                    var snap = tuiMux.capture(state.agentSessionID);
                    if (snap) {
                        evidence.screenPlain = String(snap.plain || '');
                        evidence.screenAnsi = String(snap.fullScreen || snap.ansi || '');
                    }
                }
            } catch (e) {
                log.debug('evidence screen capture failed', { error: e.message || String(e) });
            }
            try {
                if (typeof tuiMux !== 'undefined' && tuiMux &&
                    typeof tuiMux.lastActivityMs === 'function' && state.agentSessionID) {
                    evidence.lastActivityMs = tuiMux.lastActivityMs(state.agentSessionID);
                }
            } catch (e) {
                log.debug('evidence activity read failed', { error: e.message || String(e) });
            }
            try {
                if (state.mcpCallbackObj) {
                    evidence.mcpAddress = state.mcpCallbackObj.address || null;
                    evidence.mcpTransport = state.mcpCallbackObj.transport || null;
                    if (typeof state.mcpCallbackObj.lastCallTime === 'function') {
                        evidence.heartbeatMs = state.mcpCallbackObj.lastCallTime('heartbeat') || 0;
                    }
                }
            } catch (e) {
                log.debug('evidence mcp state read failed', { error: e.message || String(e) });
            }
            try {
                if (state.agentExecutor && state.agentExecutor.resolved) {
                    evidence.argv = {
                        command: state.agentExecutor.resolved.command,
                        type: state.agentExecutor.resolved.type
                    };
                }
            } catch (e) {
                log.debug('evidence argv read failed', { error: e.message || String(e) });
            }
            state.agentEvidence = evidence;
            prSplit._agentEvidence = evidence;
            return evidence;
        }

        async function writeAgentTranscript(evidence, targetDir) {
            if (!evidence) return '';
            var outDir = targetDir || transcriptDir();
            try {
                var osmod = prSplit._modules && prSplit._modules.osmod;
                if (osmod && typeof osmod.writeFile === 'function') {
                    var stamp = Date.now();
                    var path = outDir + '/pr-split-agent-' + stamp + '.log';
                    var body = 'reason: ' + evidence.reason + '\n' +
                        'at: ' + evidence.at + '\n' +
                        'sessionId: ' + String(evidence.sessionId) + '\n' +
                        'handleAlive: ' + String(evidence.handleAlive) + '\n' +
                        'lastActivityMs: ' + String(evidence.lastActivityMs) + '\n' +
                        'heartbeatMs: ' + String(evidence.heartbeatMs) + '\n' +
                        'mcpAddress: ' + String(evidence.mcpAddress) + '\n' +
                        'mcpTransport: ' + String(evidence.mcpTransport) + '\n' +
                        '--- screen ---\n' + (evidence.screenPlain || '(empty)') + '\n';
                    var wr = osmod.writeFile(path, body);
                    if (wr && typeof wr.then === 'function') { await wr; }
                    evidence.transcriptPath = path;
                    return path;
                }
            } catch (e) {
                log.debug('evidence transcript write failed', { error: e.message || String(e) });
            }
            return '';
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
                    await savePlan(resolvedPlanPath, lastDone || 'paused');
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
        async function finishTUI(result) {
            var keepSession = !!(result && result.keepSession);
            // T393: Only clean up MCP callback on error — keep alive for "Ask
            // Agent" conversation overlay on PLAN_REVIEW/ERROR_RESOLUTION.
            // On success, the wizard's quit handler handles cleanup.
            // keepSession (evidence preservation) never closes MCP or executor:
            // closure runs at discard, quit, or process exit instead.
            if (result && result.error && !keepSession) {
                var mcpCb = prSplit._mcpCallbackObj;
                if (mcpCb) {
                    try { await mcpCb.close(); } catch (e) { log.debug('cleanup mcp close failed', { error: e.message || String(e) }); }
                    prSplit._mcpCallbackObj = null;
                    state.mcpCallbackObj = null;
                }
            }

            if (result && result.error && keepSession) {
                try {
                    var kept = await captureAgentEvidence(result.evidenceReason || 'classification-timeout');
                    await writeAgentTranscript(kept);
                    result.evidence = kept;
                    report.agentEvidence = kept;
                    state.classifyCheckpoint = state.classifyCheckpoint || null;
                } catch (e) {
                    log.debug('cleanup evidence capture failed', { error: e.message || String(e) });
                }
                emitOutput('[auto-split] Agent session preserved for diagnosis.');
                emitOutput('[auto-split] Open the Agent tab to inspect the live session.');
                if (result.evidence && result.evidence.transcriptPath) {
                    emitOutput('[auto-split] Transcript: ' + result.evidence.transcriptPath);
                }
                emitOutput('[auto-split] No plan was produced so resume does not apply to this stage.');
            }

            // On error, emit resume instructions if a plan was saved.
            if (result && result.error && !keepSession && state.planCache && state.planCache.splits && state.planCache.splits.length > 0) {
                try {
                    await savePlan(resolvedPlanPath, report.lastCompletedStep || 'error');
                } catch (e) { log.debug('cleanup saveplan failed', { error: e.message || String(e) }); }

                emitOutput('\n[auto-split] Pipeline failed: ' + result.error);
                emitOutput('[auto-split] Plan saved to: ' + resolvedPlanPath);
                emitOutput('[auto-split] To resume: osm pr-split --resume\n');
            }

            if (result && result.error && !keepSession && !(state.planCache && state.planCache.splits && state.planCache.splits.length > 0)) {
                emitOutput('\n[auto-split] Pipeline failed: ' + result.error);
            }

            if (hasTUI && !config.disableTUI) {
                emitOutput('\n[auto-split] ' + (result.error ? ('Error: ' + result.error) : 'Complete'));
            }
            return result;
        }

        // Resume support: skip Steps 1-6 if resuming from a saved plan.
        var resuming = !!config.resumeFromPlan;
        if (resuming) {
            var loadResult = await loadPlan(config.resumePlanPath);
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

        // Initialize MCP callback transport for Agent IPC.
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
                                    files: { type: 'array', items: { type: 'string' }, description: 'File paths belonging to this category' },
                                    title: { type: 'string', description: 'Imperative PR title in conventional commit format' },
                                    summary: { type: 'string', description: 'Concise architectural summary of this PR layer' },
                                    keyChanges: { type: 'array', items: { type: 'string' }, description: 'Key changes and architectural decisions' },
                                    verificationSteps: { type: 'string', description: 'Commands or steps to independently verify this PR layer' },
                                    rationale: { type: 'string', description: 'Why this PR is self-contained and why it is placed at this layer' }
                                },
                                required: ['name', 'description', 'files']
                            },
                            description: 'Array of categories, each grouping related files'
                        }
                    },
                    required: ['categories']
                });

            mcpCallbackObj.addTool('reportSplitPlan',
                'Report a split plan for PR splitting following Commit-Loom discipline. Optional — if not provided, plan is generated locally from classification.',
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

            // Heartbeat tool — Agent calls periodically to signal liveness.
            // Heartbeat timeout: if Agent has sent at least one heartbeat but
            // then goes silent for longer than this, waitForLogged aborts.
            // Default: agentHeartbeatTimeoutMs from AUTOMATED_DEFAULTS (60s).
            var heartbeatTimeoutMs = typeof config.heartbeatTimeoutMs === 'number' ? config.heartbeatTimeoutMs : AUTOMATED_DEFAULTS.agentHeartbeatTimeoutMs;
            mcpCallbackObj.addTool('heartbeat',
                'Send a heartbeat to indicate Agent is still actively working. Call this periodically during long-running analysis.',
                {
                    type: 'object',
                    properties: {
                        status: { type: 'string', description: 'Optional status message (e.g., "analyzing file X")' }
                    }
                });

            await mcpCallbackObj.init();
            log.printf('auto-split: MCP callback initialized at %s (%s)', mcpCallbackObj.address, mcpCallbackObj.transport);
            log.printf('auto-split: MCP config path: %s', mcpCallbackObj.mcpConfigPath);
            try {
                var readResult = await osmod.readFile(mcpCallbackObj.mcpConfigPath);
                log.printf('auto-split: MCP config contents: %s', (!readResult.error && readResult.content) ? readResult.content : '(empty)');
            } catch (e) {
                log.printf('auto-split: could not read MCP config: %s', e.message || String(e));
            }
        } catch (e) {
            return finishTUI({ error: 'MCP callback initialization failed: ' + (e.message || String(e)), report: report });
        }

        // Steps 2-6 are skipped when resuming from a saved plan.
        var agentExecutor;
        var sessionId;
        var aliveCheckFn;

        if (!resuming) {

        // Step 2: Spawn Agent (or fall back to heuristic).
        var executor = await step('Spawn Agent', async function() {
            updateDetail('Spawn Agent', 'Resolving Agent executable...');
            agentExecutor = state.agentExecutor;
            if (!agentExecutor) {
                agentExecutor = new AgentCodeExecutor(prSplitConfig);
                state.agentExecutor = agentExecutor;
                prSplit._agentExecutor = agentExecutor;
            }
            var resolveResult = await agentExecutor.resolveAsync(function(msg) {
                updateDetail('Spawn Agent', msg);
            });
            if (resolveResult.error) {
                return { error: resolveResult.error };
            }
            updateDetail('Spawn Agent', 'Starting Agent process...');
            var spawnOpts = {};
            spawnOpts.mcpConfigPath = mcpCallbackObj.mcpConfigPath;
            var spawnResult = await agentExecutor.spawn(null, spawnOpts);
            if (spawnResult.error) {
                return { error: spawnResult.error };
            }

            // WaitReady: optionally wait for the agent to signal readiness.
            // Uses spawnHealthCheckDelayMs as the base timeout, overridable
            // via config.spawnReadyTimeoutMs. On timeout, log a warning but
            // continue — the existing isAlive() health check will catch
            // dead processes.
            if (agentExecutor.handle) {
                var readyTimeoutMs = typeof config.spawnReadyTimeoutMs === 'number' ?
                    config.spawnReadyTimeoutMs : AUTOMATED_DEFAULTS.spawnHealthCheckDelayMs;
                try {
                    if (typeof agentExecutor.handle.waitReadyAsync === 'function') {
                        await agentExecutor.handle.waitReadyAsync(readyTimeoutMs);
                    }
                } catch (e) {
                    log.printf('auto-split: WaitReady timeout (%dms) — continuing: %s',
                        readyTimeoutMs, e.message || String(e));
                }
            }

            return { error: null, sessionId: spawnResult.sessionId };
        });

        // If Agent is unavailable, fall back to heuristic mode.
        if (executor.error) {
            emitOutput('[auto-split] Agent unavailable — falling back to heuristic mode.');
            report.fallbackUsed = true;
            return finishTUI(await heuristicFallback(analysis, config, report));
        }

        sessionId = executor.sessionId;

        // Heartbeat function: checks if the Agent session is still alive.
        // Uses the pinned Agent SessionID for event-driven liveness checks,
        // with a direct handle.isAlive() fallback for headless mode.
        aliveCheckFn = function() {
            // Pinned session check: use tuiMux.isDone(agentSessionID) when
            // available. This is a channel-based (event-driven) signal that
            // fires when the PTY's Done() channel closes, independent of
            // which session is currently active.
            if (agentExecutor && agentExecutor.handle &&
                typeof tuiMux !== 'undefined' && tuiMux &&
                state.agentSessionID &&
                typeof tuiMux.isDone === 'function') {
                if (tuiMux.isDone(state.agentSessionID)) {
                    return false;
                }
            }
            // Direct handle check: covers headless mode (no tuiMux) and
            // catches process death before PTY output fully drains.
            if (!agentExecutor || !agentExecutor.handle ||
                typeof agentExecutor.handle.isAlive !== 'function' ||
                !agentExecutor.handle.isAlive()) {
                return false;
            }
            lastProgressTime = Date.now();
            return true;
        };

        // Attach Agent's PTY handle to tuiMux so ctrl+] can forward.
        // The session target was pre-configured as sessionTypes.agent at
        // bootstrap (setupEngineGlobals) — no lazy assignment needed.
        // attach() returns the pinned SessionID; store it in state so all
        // Agent reads/writes use tuiMux.capture(cid) rather than ActiveID.
        // SYNCHRONOUS INVARIANT: state.agentSessionID MUST be written in the
        // same synchronous JS turn as the attach() call. pollAgentScreenshot
        // depends on this — if a tick fires between attach and state write,
        // the guard will treat Agent as "not yet attached".
        if (agentExecutor && agentExecutor.handle && typeof tuiMux !== 'undefined' && tuiMux) {
	            if (typeof agentExecutor.handle.isAlive === 'function' && !agentExecutor.handle.isAlive()) {
	                log.printf('auto-split: Agent process died between spawn and attach — ctrl+] will not work');
	                emitOutput('[auto-split] Warning: Agent process exited unexpectedly. Toggle (Ctrl+]) unavailable.');
	            } else {
                try {
                    var cid = tuiMux.attach(agentExecutor.handle);
                    state.agentSessionID = cid;
                    if (typeof prSplit._noteAgentAttached === 'function') {
                        try { prSplit._noteAgentAttached(cid, null); } catch (e) {
                            log.debug('auto-split noteAgentAttached failed', { error: e.message || String(e) });
                        }
                    }
                    log.printf('auto-split: attached Agent (%s) handle to tuiMux, sessionID=%d',
                        typeof sessionTypes !== 'undefined' && sessionTypes.agent ? sessionTypes.agent.name : 'agent',
                        cid || 0);
                } catch (e) {
                    log.printf('auto-split: tuiMux attach warning: %s', e.message || String(e));
                }
            }
        } else if (agentExecutor && agentExecutor.handle &&
                   typeof agentExecutor.handle.drainOutputAsync === 'function') {
            // No tuiMux (headless/test mode): drain Agent's PTY output to
            // prevent buffer deadlocks.
            agentExecutor.handle.drainOutputAsync().catch(function(e) {
                log.debug('auto-split: PTY drain failed: ' + (e && e.message ? e.message : String(e)));
            });
            log.printf('auto-split: no tuiMux — started PTY output drain to prevent deadlock');
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
            updateDetail('Send classification request', 'Sending prompt to Agent...');
            var sendResult = await sendToHandle(agentExecutor.handle, renderResult.text);
            if (sendResult.error) {
                return { error: 'failed to send prompt to Agent: ' + sendResult.error };
            }
            report.agentInteractions++;
            recordConversation('classification', renderResult.text, '');
            recordTelemetry('agentInteractions', 1);
            return { error: null };
        });
        if (classifyResult.error) {
            report.error = classifyResult.error;
            await cleanupExecutor();
            return finishTUI({ error: classifyResult.error, report: report });
        }

        // Step 4: Receive classification.
        var classification = await step('Receive classification', async function() {
            updateDetail('Receive classification', 'Waiting for classification...');
            var classifyPromptSentAt = Date.now();
            var classifyLastCheckpointAt = 0;
            var pollResult = await waitForLogged('reportClassification', timeouts.classify, {
                aliveCheck: aliveCheckFn,
                heartbeatTool: 'heartbeat',
                heartbeatTimeoutMs: heartbeatTimeoutMs,
                onProgress: function(elapsed) {
                    var sec = Math.round(elapsed / 1000);
                    updateDetail('Receive classification', 'Waiting ' + sec + 's');
                    var now = Date.now();
                    if (now - classifyLastCheckpointAt < 15000) return;
                    classifyLastCheckpointAt = now;
                    var checkpoint = {
                        stage: 'receive-classification',
                        elapsedMs: elapsed,
                        timeoutMs: timeouts.classify,
                        remainingMs: Math.max(0, timeouts.classify - elapsed),
                        promptSentAt: classifyPromptSentAt,
                        lastActivityMs: -1,
                        heartbeatAgeMs: -1,
                        sessionId: state.agentSessionID || null
                    };
                    try {
                        if (typeof tuiMux !== 'undefined' && tuiMux &&
                            typeof tuiMux.lastActivityMs === 'function' && state.agentSessionID) {
                            checkpoint.lastActivityMs = tuiMux.lastActivityMs(state.agentSessionID);
                        }
                    } catch (e) {
                        log.debug('checkpoint activity read failed', { error: e.message || String(e) });
                    }
                    try {
                        if (state.mcpCallbackObj && typeof state.mcpCallbackObj.lastCallTime === 'function') {
                            var hb = state.mcpCallbackObj.lastCallTime('heartbeat') || 0;
                            checkpoint.heartbeatAgeMs = hb > 0 ? (now - hb) : -1;
                        }
                    } catch (e) {
                        log.debug('checkpoint heartbeat read failed', { error: e.message || String(e) });
                    }
                    state.classifyCheckpoint = checkpoint;
                    prSplit._classifyCheckpoint = checkpoint;
                    var remainSec = Math.round(checkpoint.remainingMs / 1000);
                    var act = checkpoint.lastActivityMs < 0 ? 'no pty output yet' : ('pty idle ' + Math.round(checkpoint.lastActivityMs / 1000) + 's');
                    var hbText = checkpoint.heartbeatAgeMs < 0 ? 'no heartbeat yet' : ('heartbeat ' + Math.round(checkpoint.heartbeatAgeMs / 1000) + 's ago');
                    emitOutput('[auto-split] Waiting for reportClassification: ' + sec + 's elapsed, ' + remainSec + 's remain (' + act + ', ' + hbText + '). Open the Agent tab to inspect, type to intervene, or cancel to abort with evidence kept.');
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
            var errText = String(classification.error || '');
            var isClassifyTimeout = errText.indexOf('timeout waiting for reportClassification') >= 0;
            var isHeartbeatStale = errText.indexOf('heartbeat timeout for reportClassification') >= 0;
            if (isClassifyTimeout || isHeartbeatStale) {
                return finishTUI({ error: classification.error, report: report, keepSession: true, evidenceReason: isHeartbeatStale ? 'heartbeat-stale' : 'classification-timeout' });
            }
            await cleanupExecutor();
            return finishTUI({ error: classification.error, report: report });
        }

        // Step 5: Generate plan (from Agent or locally).
        var planResult = await step('Generate split plan', async function() {
            updateDetail('Generate split plan', 'Checking for Agent-generated plan...');
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
                var agentPlan = planPoll.data;
                if (Array.isArray(agentPlan) && agentPlan.length > 0) {
                    var stageVal = validateSplitPlan(agentPlan);
                    if (!stageVal.valid) {
                        log.printf('auto-split: Agent split plan stage validation errors: %s', stageVal.errors.join('; '));
                    }
                    report.agentInteractions++;
                    var plan = {
                        baseBranch: analysis.baseBranch,
                        sourceBranch: analysis.currentBranch,
                        dir: '.',
                        verifyCommand: runtime.verifyCommand,
                        fileStatuses: analysis.fileStatuses || {},
                        splits: agentPlan.map(function(s, i) {
                            return {
                                name: s.name || (runtime.branchPrefix + padIndex(i, agentPlan.length)),
                                files: s.files || [],
                                message: s.message || ('Split ' + (i + 1)),
                                order: typeof s.order === 'number' ? s.order : i,
                                title: s.title || '',
                                summary: s.summary || '',
                                keyChanges: s.keyChanges || null,
                                verificationSteps: s.verificationSteps || '',
                                rationale: s.rationale || ''
                            };
                        })
                    };
                    var validation = validatePlan(plan, analysis.files);
                    if (validation.valid) {
                        report.plan = plan;
                        return { error: null, plan: plan };
                    }
                    log.printf('auto-split: Agent plan invalid: %s — generating locally', validation.errors.join('; '));
                }
            }

            // Generate plan locally from classification.
            // Preserve duplicate assignments here so validatePlan can reject
            // an ambiguous model result with a file-specific diagnostic.
            var groups = classificationToGroups(classification.classification, true);
            var plan = await createSplitPlan(groups, {
                baseBranch: analysis.baseBranch,
                sourceBranch: analysis.currentBranch,
                branchPrefix: runtime.branchPrefix,
                maxFiles: runtime.maxFiles,
                fileStatuses: analysis.fileStatuses,
                fileRenames: analysis.fileRenames
            });
            report.plan = plan;
            return { error: null, plan: plan };
        });
        if (planResult.error) {
            report.error = planResult.error;
            await cleanupExecutor();
            return finishTUI({ error: planResult.error, report: report });
        }

        var plan = planResult.plan;
        state.planCache = plan;

        // Checkpoint after plan generation.
        await savePlan(null, 'Generate split plan');

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
            await cleanupExecutor();
            return finishTUI({ error: execResult.error, report: report });
        }
        if (runtime.dryRun) {
            emitOutput('[auto-split] Dry run — skipping verification.');
            await cleanupExecutor();
            // T393: Dry-run has no interactive session after — clean up MCP
            // callback inline since finishTUI only cleans on error.
            var mcpCbDry = prSplit._mcpCallbackObj;
            if (mcpCbDry) {
                try { await mcpCbDry.close(); } catch (e) { log.debug('cleanup: mcpCb.close failed (dry-run): ' + (e.message || e)); }
                prSplit._mcpCallbackObj = null;
                state.mcpCallbackObj = null;
            }
            return finishTUI({ error: null, report: report });
        }

        // Persist plan for crash recovery / resume.
        var saveResult = await savePlan(null, 'Execute split plan');
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

            // Try to spawn Agent for conflict resolution capability.
            agentExecutor = state.agentExecutor;
            if (!agentExecutor) {
                agentExecutor = new AgentCodeExecutor(prSplitConfig);
                state.agentExecutor = agentExecutor;
                prSplit._agentExecutor = agentExecutor;
            }
            var resumeResolve = await agentExecutor.resolveAsync();
            if (!resumeResolve.error) {
                var resumeSpawn = await agentExecutor.spawn(null, { mcpConfigPath: mcpCallbackObj.mcpConfigPath });
                if (!resumeSpawn.error) {
                    sessionId = resumeSpawn.sessionId;
                    aliveCheckFn = function() {
                        // Pinned session check: use tuiMux.isDone(agentSessionID)
                        // for event-driven liveness, with handle.isAlive() fallback.
                        if (agentExecutor && agentExecutor.handle &&
                            typeof tuiMux !== 'undefined' && tuiMux &&
                            state.agentSessionID &&
                            typeof tuiMux.isDone === 'function') {
                            if (tuiMux.isDone(state.agentSessionID)) {
                                return false;
                            }
                        }
                        return agentExecutor && agentExecutor.handle &&
                               typeof agentExecutor.handle.isAlive === 'function' &&
                               agentExecutor.handle.isAlive();
                    };
                    // Re-attach to tuiMux and capture pinned SessionID.
                    if (agentExecutor && agentExecutor.handle &&
                        typeof tuiMux !== 'undefined' && tuiMux &&
                        typeof tuiMux.attach === 'function') {
                        try {
                            var resumeCid = tuiMux.attach(agentExecutor.handle);
                            state.agentSessionID = resumeCid;
                            if (typeof prSplit._noteAgentAttached === 'function') {
                                try { prSplit._noteAgentAttached(resumeCid, null); } catch (e) {
                                    log.debug('auto-split resume noteAgentAttached failed', { error: e.message || String(e) });
                                }
                            }
                            log.printf('auto-split resume: attached Agent handle to tuiMux, sessionID=%d', resumeCid || 0);
                        } catch (e) {
                            log.printf('auto-split resume: tuiMux attach warning: %s', e.message || String(e));
                        }
                    }
                } else {
                    emitOutput('[auto-split] Warning: Agent spawn failed — conflict resolution disabled.');
                }
            } else {
                emitOutput('[auto-split] Agent unavailable — conflict resolution disabled for resume.');
            }
        }

        // Step 7: Verify splits.
        var verifyResult = await step('Verify splits', async function() {
            updateDetail('Verify splits', 'Running verification command on each branch...');
            var verifyObj = await verifySplits(plan, {
                verifyTimeoutMs: typeof config.verifyTimeoutMs === 'number' ? config.verifyTimeoutMs : AUTOMATED_DEFAULTS.verifyTimeoutMs,
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
                emitOutput('[auto-split] Skipped ' + skippedResults.length +
                    ' branches due to dependency failures: ' + skippedNames.join(', '));
            }
            if (preExistingResults.length > 0) {
                var preExNames = [];
                for (var p = 0; p < preExistingResults.length; p++) {
                    preExNames.push(preExistingResults[p].name);
                }
                emitOutput('[auto-split] ' + preExistingResults.length +
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
        await savePlan(null, 'Verify splits');

        // Step 8: Resolve conflicts (if any real failures).
        var reSplitCount = 0;
        if (verifyResult.failures && verifyResult.failures.length > 0) {
            var resolved = await step('Resolve conflicts via Agent', async function() {
                return await resolveConflictsWithAgent(
                    verifyResult.failures, sessionId,
                    timeouts, pollInterval, maxAttemptsPerBranch, report, aliveCheckFn,
                    heartbeatTimeoutMs
                );
            });

            // Step 9: Re-split fallback if needed.
            if (resolved.reSplitNeeded && reSplitCount < maxReSplits) {
                reSplitCount++;
                emitOutput('[auto-split] Re-split requested — re-classifying...');
                await cleanupBranches(plan);
                var reClassifyResult = await step('Re-classify (retry ' + reSplitCount + ')', async function() {
                    var constraintPrompt = 'Re-classify these files with the constraint: ' +
                        resolved.reSplitReason + '\n\nUse the reportClassification MCP tool.\n';
                    var sendResult = await sendToHandle(agentExecutor.handle, constraintPrompt);
                    if (sendResult.error) {
                        return { error: 'failed to send re-classify prompt: ' + sendResult.error };
                    }
                    report.agentInteractions++;
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
                        var reText = String(rePoll.error || '');
                        var reTimeout = reText.indexOf('timeout waiting for reportClassification') >= 0;
                        var reStale = reText.indexOf('heartbeat timeout for reportClassification') >= 0;
                        if (reTimeout || reStale) {
                            return { error: rePoll.error, preserveSession: true, evidenceReason: reStale ? 'heartbeat-stale' : 'classification-timeout' };
                        }
                        return { error: rePoll.error };
                    }
                    return { error: null, classification: rePoll.data };
                });
                if (reClassifyResult.error) {
                    if (reClassifyResult.preserveSession) {
                        report.error = reClassifyResult.error;
                        return finishTUI({ error: reClassifyResult.error, report: report, keepSession: true, evidenceReason: reClassifyResult.evidenceReason || 'classification-timeout' });
                    }
                    report.error = reClassifyResult.error;
                }
                if (!reClassifyResult.error) {
                    var newGroups = classificationToGroups(reClassifyResult.classification, true);
                    plan = await createSplitPlan(newGroups, {
                        baseBranch: analysis.baseBranch,
                        sourceBranch: analysis.currentBranch,
                        branchPrefix: runtime.branchPrefix,
                        maxFiles: runtime.maxFiles,
                        fileStatuses: analysis.fileStatuses,
                        fileRenames: analysis.fileRenames
                    });
                    state.planCache = plan;
                    report.plan = plan;
                    var reExec = await step('Re-execute split', async function() {
                        var result = await executeSplit(plan);
                        if (result.error) return { error: result.error };
                        report.splits = result.results || [];
                        return { error: null };
                    });
                    if (reExec.error) {
                        report.error = reExec.error;
                    } else {
                        var reVerifyResult = await step('Re-verify splits', async function() {
                            // T104: Actually check whether re-verified branches pass.
                            // Previously this always returned { error: null }.
                            var rv = await verifySplits(plan, {
                                verifyTimeoutMs: typeof config.verifyTimeoutMs === 'number' ? config.verifyTimeoutMs : AUTOMATED_DEFAULTS.verifyTimeoutMs,
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
                        if (reVerifyResult.error) {
                            report.error = reVerifyResult.error;
                        }
                    }
                }
            }

            // Checkpoint after resolve/re-split.
            if (!report.error) {
                await savePlan(null, 'Resolve conflicts');
            }
        }

        // Step 10: Equivalence check and report.
        var equivResult = report.error ? { error: report.error } : await step('Verify equivalence', async function() {
            var result = await verifyEquivalence(plan);
            return { error: result.equivalent ? null : 'tree hash mismatch', result: result };
        });

        // T121: Propagate equivalence result to report so the TUI can
        // transition from BRANCH_BUILDING → EQUIV_CHECK → FINALIZATION.
        if (equivResult && equivResult.result) {
            report.equivalence = equivResult.result;
        }

        // Assess independence.
        report.independencePairs = report.error ? [] : await assessIndependence(plan, classification.classification || {});

        // Summary.
        emitOutput('');
        emitOutput(report.error ? '=== Auto-Split Failed ===' : '=== Auto-Split Complete ===');
        emitOutput('Splits: ' + plan.splits.length);
        emitOutput('Agent interactions: ' + report.agentInteractions);
        emitOutput(equivResult.result ?
            'Equivalence: ' + (equivResult.result.equivalent ? 'PASS' : 'FAIL') :
            (report.error ? 'Equivalence: SKIPPED (pipeline failed)' : 'Equivalence: FAIL'));
        if (report.independencePairs.length > 0) {
            emitOutput('Independent pairs: ' + report.independencePairs.map(function(p) {
                return p[0] + ' + ' + p[1];
            }).join(', '));
        }
        if (report.fallbackUsed) {
            emitOutput('Mode: heuristic (Agent unavailable)');
        }

        // T393: Keep Agent alive for "Ask Agent" on PLAN_REVIEW.
        // Only clean up on error — the wizard's quit handler (confirmCancel)
        // handles cleanup on success/cancel paths, and Go context cancellation
        // handles cleanup on process exit.
        if (report.error) {
            await cleanupExecutor();
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
