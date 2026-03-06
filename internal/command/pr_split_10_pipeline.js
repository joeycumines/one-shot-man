'use strict';
// pr_split_10_pipeline.js — Automated split pipeline (largest chunk)
// Dependencies: chunks 00-09 must be loaded first.
//
// Go-injected globals used directly (available in global scope):
//   tuiMux         — TUI multiplexer for ctrl+] toggle (optional)
//   output         — output.print for user-facing messages
//   log            — log.printf for debug/diagnostic logging
//   prSplitConfig  — resolved config from Go bridge
//
// Cross-chunk references (late-bound via prSplit.* at call time):
//   Chunk 00: isCancelled, isPaused, isForceCancelled, _gitExec,
//             _gitAddChangedFiles, _padIndex, detectGoModulePath, runtime
//   Chunk 01: analyzeDiff
//   Chunk 02: applyStrategy
//   Chunk 03: createSplitPlan, savePlan, loadPlan, DEFAULT_PLAN_PATH
//   Chunk 04: validateClassification, validateSplitPlan, validatePlan, validateResolution
//   Chunk 05: executeSplit
//   Chunk 06: verifySplit, verifySplits, verifyEquivalence, cleanupBranches
//   Chunk 08: resolveConflicts
//   Chunk 09: ClaudeCodeExecutor, renderClassificationPrompt, renderConflictPrompt
//   Chunk 11: assessIndependence, recordConversation, recordTelemetry (late-bound)
//
// Shared state on prSplit._state:
//   conversationHistory, telemetryData, claudeExecutor, mcpCallbackObj,
//   analysisCache, groupsCache, planCache, executionResultCache

(function(prSplit) {

    // -----------------------------------------------------------------------
    //  Constants
    // -----------------------------------------------------------------------

    // Default polling and timeout configuration for the automated split pipeline.
    //
    // TIMEOUT ARCHITECTURE (chain from Go CLI to JS execution):
    //
    //   1. Go CLI flag: `osm pr-split --timeout 30m`
    //      → Parsed in pr_split.go as time.Duration, converted to milliseconds.
    //
    //   2. Go → JS bridge: prSplitConfig.timeoutMs (set in pr_split.go)
    //      → This is the user-facing master timeout. It caps the entire pipeline.
    //
    //   3. Per-step overrides: prSplitConfig.<step>TimeoutMs
    //      → config.classifyTimeoutMs, config.planTimeoutMs, config.resolveTimeoutMs
    //      → Set via config file (pr-split.classify-timeout-ms, etc.) or flags.
    //
    //   4. Fallback defaults: AUTOMATED_DEFAULTS.<step>TimeoutMs (below)
    //      → Used when neither CLI flag nor config provides a per-step timeout.
    //      → Applied in automatedSplit() via: config.X || AUTOMATED_DEFAULTS.X
    //
    //   5. Clamping: After || fallback, values are clamped:
    //      → pollInterval: min 50ms (prevent spin-loops)
    //      → maxReSplits: min 0 (prevent negative loop counts)
    //      → maxAttemptsPerBranch: min 0
    //
    //   6. Consumers: Each pipeline step reads its timeout from the resolved chain:
    //      → mcpCallbackObj.waitFor(toolName, timeoutMs, opts)
    //      → resolveConflictsWithClaude(..., { resolveWallClockTimeoutMs })
    //      → verifySplit(..., { verifyTimeoutMs })
    var AUTOMATED_DEFAULTS = {
        classifyTimeoutMs: 1200000, // 20 minutes for classification
        planTimeoutMs: 1200000,     // 20 minutes for plan generation
        resolveTimeoutMs: 1800000,  // 30 minutes for conflict resolution
        pollIntervalMs: 500,        // Poll every 500ms for fast cancellation
        maxResolveRetries: 3,       // Retries per branch
        maxReSplits: 1,             // Maximum re-classification cycles
        resolveWallClockTimeoutMs: 7200000, // 120 minutes wall-clock cap
        verifyTimeoutMs: 600000,    // 10 minutes per branch for verify step
        pipelineTimeoutMs: 7200000, // 120 minutes overall pipeline timeout
        stepTimeoutMs: 3600000,     // 60 minutes per step
        watchdogIdleMs: 900000      // 15 minutes no-progress watchdog
    };

    // Delay between text and newline writes to defeat PTY coalescing.
    var SEND_TEXT_NEWLINE_DELAY_MS = 10;
    // Chunk large prompts into smaller writes so PTY consumers read them
    // incrementally instead of one giant burst.
    var SEND_TEXT_CHUNK_BYTES = 512;
    // Delay between chunk writes to reduce PTY coalescing into a single
    // read burst on the Claude side.
    var SEND_TEXT_CHUNK_DELAY_MS = 2;
    // Wait for prompt/input anchors to stabilize before pressing Enter.
    var SEND_PRE_SUBMIT_STABLE_TIMEOUT_MS = 1500;
    var SEND_PRE_SUBMIT_STABLE_POLL_MS = 50;
    var SEND_PRE_SUBMIT_STABLE_SAMPLES = 3;
    var SEND_INPUT_ANCHOR_TAIL_CHARS = 28;
    // After pressing Enter, require anchor movement (prompt/input) that is
    // stable for multiple polls.
    var SEND_SUBMIT_ACK_TIMEOUT_MS = 1500;
    var SEND_SUBMIT_ACK_POLL_MS = 50;
    var SEND_SUBMIT_ACK_STABLE_SAMPLES = 2;
    var SEND_SUBMIT_MAX_NEWLINE_ATTEMPTS = 3;
    // Wait for Claude prompt marker before sending text. This prevents
    // early writes into startup/setup screens where input is not ready.
    var SEND_PROMPT_READY_TIMEOUT_MS = 10000;
    var SEND_PROMPT_READY_POLL_MS = 100;
    var SEND_PROMPT_READY_STABLE_SAMPLES = 2;

    // -----------------------------------------------------------------------
    //  sendToHandle — PTY double-write with async delay
    // -----------------------------------------------------------------------

    function resolveNumber(value, fallback, minValue) {
        var n = Number(value);
        if (!isFinite(n)) return fallback;
        n = Math.floor(n);
        if (minValue !== undefined && n < minValue) return fallback;
        return n;
    }

    function resolveSendConfig() {
        return {
            textNewlineDelayMs: resolveNumber(prSplit.SEND_TEXT_NEWLINE_DELAY_MS, SEND_TEXT_NEWLINE_DELAY_MS, 0),
            textChunkBytes: resolveNumber(prSplit.SEND_TEXT_CHUNK_BYTES, SEND_TEXT_CHUNK_BYTES, 1),
            textChunkDelayMs: resolveNumber(prSplit.SEND_TEXT_CHUNK_DELAY_MS, SEND_TEXT_CHUNK_DELAY_MS, 0),
            preSubmitStableTimeoutMs: resolveNumber(prSplit.SEND_PRE_SUBMIT_STABLE_TIMEOUT_MS, SEND_PRE_SUBMIT_STABLE_TIMEOUT_MS, 1),
            preSubmitStablePollMs: resolveNumber(prSplit.SEND_PRE_SUBMIT_STABLE_POLL_MS, SEND_PRE_SUBMIT_STABLE_POLL_MS, 1),
            preSubmitStableSamples: resolveNumber(prSplit.SEND_PRE_SUBMIT_STABLE_SAMPLES, SEND_PRE_SUBMIT_STABLE_SAMPLES, 1),
            inputAnchorTailChars: resolveNumber(prSplit.SEND_INPUT_ANCHOR_TAIL_CHARS, SEND_INPUT_ANCHOR_TAIL_CHARS, 4),
            submitAckTimeoutMs: resolveNumber(prSplit.SEND_SUBMIT_ACK_TIMEOUT_MS, SEND_SUBMIT_ACK_TIMEOUT_MS, 1),
            submitAckPollMs: resolveNumber(prSplit.SEND_SUBMIT_ACK_POLL_MS, SEND_SUBMIT_ACK_POLL_MS, 1),
            submitAckStableSamples: resolveNumber(prSplit.SEND_SUBMIT_ACK_STABLE_SAMPLES, SEND_SUBMIT_ACK_STABLE_SAMPLES, 1),
            submitMaxNewlineAttempts: resolveNumber(prSplit.SEND_SUBMIT_MAX_NEWLINE_ATTEMPTS, SEND_SUBMIT_MAX_NEWLINE_ATTEMPTS, 1),
            promptReadyTimeoutMs: resolveNumber(prSplit.SEND_PROMPT_READY_TIMEOUT_MS, SEND_PROMPT_READY_TIMEOUT_MS, 1),
            promptReadyPollMs: resolveNumber(prSplit.SEND_PROMPT_READY_POLL_MS, SEND_PROMPT_READY_POLL_MS, 1),
            promptReadyStableSamples: resolveNumber(prSplit.SEND_PROMPT_READY_STABLE_SAMPLES, SEND_PROMPT_READY_STABLE_SAMPLES, 1)
        };
    }

    function getCancellationError() {
        if (typeof prSplit.isForceCancelled === 'function' && prSplit.isForceCancelled()) {
            return 'force cancelled by user';
        }
        if (typeof prSplit.isCancelled === 'function' && prSplit.isCancelled()) {
            return 'cancelled by user';
        }
        return null;
    }

    function captureScreenshot() {
        if (typeof tuiMux === 'undefined' || !tuiMux || typeof tuiMux.screenshot !== 'function') {
            return null;
        }
        try {
            var shot = tuiMux.screenshot();
            if (shot === null || shot === undefined) return '';
            return String(shot);
        } catch (e) {
            log.printf('auto-split sendToHandle: screenshot read failed — %s', e.message || String(e));
            return null;
        }
    }

    function getTextTailAnchor(text, maxChars) {
        var s = String(text || '');
        var lines = s.split('\n');
        for (var i = lines.length - 1; i >= 0; i--) {
            var line = (lines[i] || '').trim();
            if (!line) continue;
            if (line.length > maxChars) {
                return line.substring(line.length - maxChars);
            }
            return line;
        }
        var trimmed = s.trim();
        if (!trimmed) return '';
        if (trimmed.length > maxChars) {
            return trimmed.substring(trimmed.length - maxChars);
        }
        return trimmed;
    }

    function trimLeadingPromptSpace(s) {
        return String(s || '').replace(/^[\s\u00a0]+/, '');
    }

    function isPromptMarkerLine(line) {
        var trimmed = trimLeadingPromptSpace(line);
        if (!trimmed) return false;
        var first = trimmed.charAt(0);
        if (first !== '❯' && first !== '>') return false;
        var rest = trimLeadingPromptSpace(trimmed.substring(1));
        // Exclude setup menu selectors like "❯ 1. Dark mode".
        if (/^\d+\./.test(rest)) return false;
        return true;
    }

    function detectPromptBlocker(screen) {
        var lower = String(screen || '').toLowerCase();
        if (lower.indexOf('choose the text style') !== -1 &&
            lower.indexOf("let's get started") !== -1) {
            return 'Claude is waiting on first-run setup (theme selection); complete setup in Claude first.';
        }
        return '';
    }

    function findPromptMarker(screen) {
        var lines = screen.split('\n');
        var offset = 0;
        var best = null;
        for (var i = 0; i < lines.length; i++) {
            var line = lines[i];
            if (isPromptMarkerLine(line)) {
                best = {
                    bottom: offset + line.length,
                    lineIndex: i,
                    lineText: line
                };
            }
            offset += line.length + 1;
        }
        return best;
    }

    function captureInputAnchors(text, cfg) {
        var screen = captureScreenshot();
        if (screen === null) {
            return { observed: false };
        }
        var tail = getTextTailAnchor(text, cfg.inputAnchorTailChars);
        var prompt = findPromptMarker(screen);
        var blocker = detectPromptBlocker(screen);

        var lines = screen.split('\n');
        var offset = 0;
        var lastTailCandidate = null;
        var lastPasteCandidate = null;
        for (var i = 0; i < lines.length; i++) {
            var line = lines[i];
            var bottom = offset + line.length;

            if (tail && line.indexOf(tail) !== -1) {
                lastTailCandidate = {
                    bottom: bottom,
                    lineIndex: i,
                    type: 'tail'
                };
            }
            if (line.indexOf('[Pasted text') !== -1 || line.indexOf('[Pasted') !== -1) {
                lastPasteCandidate = {
                    bottom: bottom,
                    lineIndex: i,
                    type: 'paste'
                };
            }
            offset += line.length + 1;
        }

        var candidate = null;
        if (prompt && (lastTailCandidate || lastPasteCandidate)) {
            var nearTail = (lastTailCandidate &&
                Math.abs(lastTailCandidate.lineIndex - prompt.lineIndex) <= 2) ? lastTailCandidate : null;
            var nearPaste = (lastPasteCandidate &&
                Math.abs(lastPasteCandidate.lineIndex - prompt.lineIndex) <= 2) ? lastPasteCandidate : null;
            candidate = nearTail || nearPaste || lastTailCandidate || lastPasteCandidate;
        } else {
            candidate = lastTailCandidate || lastPasteCandidate;
        }

        var inputBottom = -1;
        var inputType = '';
        var inputLineIndex = -1;
        if (candidate) {
            inputBottom = candidate.bottom;
            inputType = candidate.type;
            inputLineIndex = candidate.lineIndex;
        }
        var promptBottom = prompt ? prompt.bottom : -1;
        var promptLineIndex = prompt ? prompt.lineIndex : -1;

        return {
            observed: true,
            screen: screen,
            blocker: blocker,
            promptBottom: promptBottom,
            promptLineIndex: promptLineIndex,
            inputBottom: inputBottom,
            inputLineIndex: inputLineIndex,
            inputType: inputType,
            stableKey: String(promptBottom) + '|' + String(inputBottom)
        };
    }

    async function waitForPromptReady(cfg) {
        var startMs = Date.now();
        var lastKey = '';
        var stableCount = 0;
        var lastState = null;
        while (Date.now() - startMs < cfg.promptReadyTimeoutMs) {
            var cancelErr = getCancellationError();
            if (cancelErr) {
                return { error: cancelErr, observed: true, state: null };
            }

            var state = captureInputAnchors('', cfg);
            if (!state.observed) {
                return { error: null, observed: false, state: null };
            }
            lastState = state;
            if (state.blocker) {
                return { error: state.blocker, observed: true, state: state };
            }
            if (state.promptBottom !== -1) {
                var key = String(state.promptBottom) + '|' + String(state.promptLineIndex);
                if (key === lastKey) {
                    stableCount++;
                } else {
                    stableCount = 1;
                    lastKey = key;
                }
                if (stableCount >= cfg.promptReadyStableSamples) {
                    return { error: null, observed: true, state: state };
                }
            } else {
                stableCount = 0;
                lastKey = '';
            }

            await new Promise(function(resolve) { setTimeout(resolve, cfg.promptReadyPollMs); });
        }
        return {
            error: 'Claude prompt not ready before send: prompt marker not found',
            observed: true,
            state: lastState
        };
    }

    async function waitForStableInputAnchors(text, cfg) {
        var startMs = Date.now();
        var lastKey = '';
        var stableCount = 0;
        var lastState = null;
        while (Date.now() - startMs < cfg.preSubmitStableTimeoutMs) {
            var cancelErr = getCancellationError();
            if (cancelErr) {
                return { error: cancelErr, state: null, observed: true };
            }
            var state = captureInputAnchors(text, cfg);
            if (!state.observed) {
                return { error: null, state: null, observed: false };
            }
            lastState = state;
            if (state.stableKey === lastKey) {
                stableCount++;
            } else {
                stableCount = 1;
                lastKey = state.stableKey;
            }
            var anchorsReady = (state.inputBottom !== -1 &&
                                state.promptBottom !== -1 &&
                                state.inputLineIndex !== -1 &&
                                state.promptLineIndex !== -1 &&
                                Math.abs(state.inputLineIndex - state.promptLineIndex) <= 2);
            if (anchorsReady && stableCount >= cfg.preSubmitStableSamples) {
                return { error: null, state: state, observed: true };
            }
            await new Promise(function(resolve) { setTimeout(resolve, cfg.preSubmitStablePollMs); });
        }

        if (lastState &&
            lastState.inputBottom !== -1 &&
            lastState.promptBottom !== -1 &&
            lastState.inputLineIndex !== -1 &&
            lastState.promptLineIndex !== -1 &&
            Math.abs(lastState.inputLineIndex - lastState.promptLineIndex) <= 2) {
            return { error: null, state: lastState, observed: true };
        }
        return {
            error: 'unable to locate stable prompt/input anchors before submit',
            state: lastState,
            observed: true
        };
    }

    async function waitForSubmitAck(baselineState, text, cfg) {
        if (!baselineState || !baselineState.observed) {
            return { acknowledged: false, observed: false, state: null, error: null };
        }
        var startMs = Date.now();
        var stableCount = 0;
        var lastChangeKey = '';
        var latest = baselineState;

        while (Date.now() - startMs < cfg.submitAckTimeoutMs) {
            var cancelErr = getCancellationError();
            if (cancelErr) {
                return { acknowledged: false, observed: true, state: latest, error: cancelErr };
            }

            var state = captureInputAnchors(text, cfg);
            if (!state.observed) {
                return { acknowledged: false, observed: false, state: null, error: null };
            }
            latest = state;

            // Acknowledge only on meaningful movement:
            // - input anchor cleared,
            // - input anchor moved substantially (not minor reflow jitter),
            // - or prompt anchor moved while input is cleared.
            var anchorChanged = false;
            if (baselineState.inputBottom !== -1) {
                if (state.inputBottom === -1) {
                    anchorChanged = true;
                } else if (Math.abs(state.inputBottom - baselineState.inputBottom) >= 8) {
                    anchorChanged = true;
                } else if (state.inputType !== baselineState.inputType &&
                           state.inputBottom !== baselineState.inputBottom) {
                    anchorChanged = true;
                }
            }
            if (!anchorChanged &&
                baselineState.promptBottom !== -1 &&
                state.promptBottom !== -1 &&
                state.promptBottom !== baselineState.promptBottom &&
                state.inputBottom === -1) {
                anchorChanged = true;
            }

            if (anchorChanged) {
                if (state.stableKey === lastChangeKey) {
                    stableCount++;
                } else {
                    stableCount = 1;
                    lastChangeKey = state.stableKey;
                }
                if (stableCount >= cfg.submitAckStableSamples) {
                    return { acknowledged: true, observed: true, state: state, error: null };
                }
            } else {
                stableCount = 0;
                lastChangeKey = '';
            }

            await new Promise(function(resolve) { setTimeout(resolve, cfg.submitAckPollMs); });
        }
        return { acknowledged: false, observed: true, state: latest, error: null };
    }

    // sendToHandle writes prompt text and submits it with Enter. It uses:
    //  1) chunked text writes (to reduce giant single-write fragility),
    //  2) newline as a distinct write (not text+\n in one write),
    //  3) terminal-output observation via tuiMux.screenshot() to confirm
    //     Claude reacted to submission, retrying newline if needed.
    //
    // Returns Promise<{ error: null }> on success,
    // Promise<{ error: "message" }> on failure.
    async function sendToHandle(handle, text) {
        if (!handle) {
            log.printf('auto-split sendToHandle: handle is null — process may have exited');
            return { error: 'Claude process handle is null — process may have exited or failed to spawn. Check Claude CLI availability and MCP configuration.' };
        }
        var config = resolveSendConfig();
        var truncated = text.length > 120 ? text.substring(0, 120) + '...' : text;
        log.printf('auto-split sendToHandle: sending %d chars — %s', text.length, truncated);

        var EAGAIN_MAX_RETRIES = 3;
        var EAGAIN_RETRY_DELAY_MS = 10;
        var promptReady = await waitForPromptReady(config);
        if (promptReady.error) {
            return { error: promptReady.error };
        }
        var observedTransport = promptReady.observed;

        // Helper to send text with EAGAIN retry (returns {error: null|string}).
        async function sendWithRetry(data) {
            var lastErr;
            for (var attempt = 0; attempt <= EAGAIN_MAX_RETRIES; attempt++) {
                try {
                    handle.send(data);
                    return { error: null };
                } catch (e) {
                    lastErr = e.message || String(e);
                }
                var lower = (lastErr || '').toLowerCase();
                if (lower.indexOf('eagain') !== -1 ||
                    lower.indexOf('resource temporarily unavailable') !== -1 ||
                    lower.indexOf('would block') !== -1) {
                    if (attempt < EAGAIN_MAX_RETRIES) {
                        await new Promise(function(resolve) {
                            setTimeout(resolve, EAGAIN_RETRY_DELAY_MS);
                        });
                        continue;
                    }
                }
                return { error: lastErr };
            }
            return { error: lastErr };
        }

        // Step 1: Send text in chunks.
        var sentChunks = 0;
        for (var offset = 0; offset < text.length; offset += config.textChunkBytes) {
            var cancelErr = getCancellationError();
            if (cancelErr) {
                return { error: cancelErr };
            }
            var chunk = text.substring(offset, offset + config.textChunkBytes);
            var textResult = await sendWithRetry(chunk);
            if (textResult.error) {
                log.printf('auto-split sendToHandle: text write FAILED at chunk %d — %s', sentChunks + 1, textResult.error);
                return textResult;
            }
            sentChunks++;
            if (config.textChunkDelayMs > 0) {
                await new Promise(function(resolve) {
                    setTimeout(resolve, config.textChunkDelayMs);
                });
            }
        }
        if (text.length === 0) sentChunks = 1;
        log.printf('auto-split sendToHandle: text written in %d chunk(s)', sentChunks);

        // Step 2: Wait for stable prompt/input anchors before Enter.
        var stableAnchors = await waitForStableInputAnchors(text, config);
        if (stableAnchors.error) {
            return { error: stableAnchors.error };
        }
        observedTransport = observedTransport || stableAnchors.observed;
        var baselineState = stableAnchors.state;
        if (observedTransport && baselineState) {
            log.printf('auto-split sendToHandle: stable anchors ready (promptBottom=%d inputBottom=%d type=%s)',
                baselineState.promptBottom, baselineState.inputBottom, baselineState.inputType || '?');
        }

        // Step 3: Non-blocking delay using event-loop-integrated setTimeout.
        log.printf('auto-split sendToHandle: waiting %dms (via setTimeout, non-blocking)', config.textNewlineDelayMs);
        await new Promise(function(resolve) {
            setTimeout(resolve, config.textNewlineDelayMs);
        });

        // Step 4: Submit with newline, retrying until prompt/input anchors move.
        for (var attempt = 1; attempt <= config.submitMaxNewlineAttempts; attempt++) {
            var attemptCancelErr = getCancellationError();
            if (attemptCancelErr) {
                return { error: attemptCancelErr };
            }
            log.printf('auto-split sendToHandle: sending newline attempt %d/%d',
                attempt, config.submitMaxNewlineAttempts);
            var newlineResult = await sendWithRetry('\r');
            if (newlineResult.error) {
                log.printf('auto-split sendToHandle: newline write FAILED (attempt %d) — %s', attempt, newlineResult.error);
                return newlineResult;
            }

            // If no screenshot transport is available, we cannot observe acceptance.
            // Keep existing behavior: a successful newline write is considered success.
            if (!observedTransport) {
                log.printf('auto-split sendToHandle: screenshot transport unavailable; submit acknowledged by successful write only');
                return { error: null };
            }

            // Re-capture stable anchors if baseline is missing (e.g., first pass
            // did not have enough context, but transport is now available).
            if (!baselineState) {
                var recapture = await waitForStableInputAnchors(text, config);
                if (recapture.error) {
                    return { error: recapture.error };
                }
                baselineState = recapture.state;
            }
            var watch = await waitForSubmitAck(baselineState, text, config);
            if (watch.error) {
                return { error: watch.error };
            }
            if (watch.acknowledged) {
                log.printf('auto-split sendToHandle: observed submit acknowledgment after newline attempt %d', attempt);
                return { error: null };
            }

            log.printf('auto-split sendToHandle: no submit acknowledgment after newline attempt %d (%dms window)',
                attempt, config.submitAckTimeoutMs);
            baselineState = watch.state || baselineState;
        }
        return {
            error: 'prompt submission unconfirmed: prompt/input anchors did not move after ' +
                   config.submitMaxNewlineAttempts + ' newline attempts'
        };
    }

    // -----------------------------------------------------------------------
    //  waitForLogged — logged wrapper around mcpCallbackObj.waitFor
    // -----------------------------------------------------------------------

    // Emits before/after log entries so IPC timeouts can be diagnosed
    // post-mortem.
    function waitForLogged(toolName, timeoutMs, opts) {
        var mcpCb = prSplit._mcpCallbackObj;
        if (!mcpCb || typeof mcpCb.waitFor !== 'function') {
            return { data: null, error: 'MCP callback not initialized — ensure mcpConfigPath is provided via osm:mcpcallback module' };
        }

        opts = opts || {};
        var userAliveCheck = (typeof opts.aliveCheck === 'function') ? opts.aliveCheck : null;
        var cancelledByUser = false;
        var forceCancelledByUser = false;
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
            if (userAliveCheck) {
                return !!userAliveCheck();
            }
            return true;
        };

        log.printf('auto-split waitFor: tool=%s timeout=%dms', toolName, timeoutMs);
        var startMs = Date.now();
        var result = mcpCb.waitFor(toolName, timeoutMs, wrappedOpts);
        var elapsedMs = Date.now() - startMs;
        if (forceCancelledByUser) {
            return { data: null, error: 'force cancelled by user' };
        }
        if (cancelledByUser) {
            return { data: null, error: 'cancelled by user' };
        }
        if (result.error) {
            log.printf('auto-split waitFor: tool=%s FAILED after %dms — %s', toolName, elapsedMs, result.error);
        } else {
            log.printf('auto-split waitFor: tool=%s received after %dms', toolName, elapsedMs);
        }
        return result;
    }

    // -----------------------------------------------------------------------
    //  classificationToGroups — pure conversion function
    // -----------------------------------------------------------------------

    // Converts a classification result to groups map
    // (category→{files: [...], description: "..."}).
    // Accepts both new format (array of {name, description, files}) and
    // legacy format ({file: category} map) for backwards compatibility.
    function classificationToGroups(classification) {
        var groups = {};
        if (Array.isArray(classification)) {
            for (var i = 0; i < classification.length; i++) {
                var cat = classification[i];
                if (!cat.name) continue;
                groups[cat.name] = {
                    files: cat.files || [],
                    description: cat.description || ''
                };
            }
        } else if (classification && typeof classification === 'object') {
            for (var path in classification) {
                var catName = classification[path];
                if (!groups[catName]) groups[catName] = { files: [], description: '' };
                groups[catName].files.push(path);
            }
        }
        return groups;
    }

    // -----------------------------------------------------------------------
    //  cleanupExecutor — resource cleanup
    // -----------------------------------------------------------------------

    // Closes the Claude executor and cleans up resources. Detaches from
    // tuiMux so ctrl+] stops trying to forward to a dead child process.
    // When force-cancelled, sends SIGKILL to skip graceful shutdown.
    function cleanupExecutor() {
        var isForceCancelled = prSplit.isForceCancelled;
        var claudeExec = prSplit._state.claudeExecutor;
        var forceNow = false;
        try { forceNow = !!isForceCancelled(); } catch (e) { forceNow = false; }
        log.printf('auto-split cleanupExecutor: start force=%s hasExecutor=%s', forceNow ? 'true' : 'false', claudeExec ? 'true' : 'false');

        // Close the executor FIRST so the child PTY fd is released.
        if (claudeExec) {
            if (forceNow && claudeExec.handle &&
                typeof claudeExec.handle.signal === 'function') {
                log.printf('auto-split cleanupExecutor: sending SIGKILL to Claude handle before close');
                try { claudeExec.handle.signal('SIGKILL'); } catch (e) {
                    log.printf('auto-split cleanupExecutor: pre-close SIGKILL error: %s', e.message || String(e));
                }
            }
            log.printf('auto-split cleanupExecutor: closing Claude executor');
            try {
                claudeExec.close();
                log.printf('auto-split cleanupExecutor: Claude executor closed');
            } catch (e) {
                log.printf('auto-split cleanupExecutor: Claude close error: %s', e.message || String(e));
            }
        }
        // Avoid synchronous detach here — in some PTY edge cases Detach can
        // stall while terminal I/O is still unwinding. The child is already
        // closed above, so leaving mux detach for the next attach path keeps
        // cleanup non-blocking.
        if (typeof tuiMux !== 'undefined' && tuiMux) {
            log.printf('auto-split cleanupExecutor: skipping immediate tuiMux.detach (non-blocking cleanup)');
        }
        log.printf('auto-split cleanupExecutor: done');
    }

    // -----------------------------------------------------------------------
    //  heuristicFallback — standard heuristic split flow
    // -----------------------------------------------------------------------

    // Runs the standard heuristic split flow when Claude is unavailable.
    async function heuristicFallback(analysis, config, report) {
        // Late-bind cross-chunk dependencies.
        var runtime = prSplit.runtime;
        var applyStrategy = prSplit.applyStrategy;
        var createSplitPlan = prSplit.createSplitPlan;
        var executeSplit = prSplit.executeSplit;
        var verifySplits = prSplit.verifySplits;
        var verifyEquivalence = prSplit.verifyEquivalence;
        var resolveConflicts = prSplit.resolveConflicts;
        var state = prSplit._state;

        var strategy = config.strategy || runtime.strategy;
        var groups = applyStrategy(analysis.files, strategy, {
            fileStatuses: analysis.fileStatuses,
            maxFiles: runtime.maxFiles,
            baseBranch: analysis.baseBranch
        });
        state.groupsCache = groups;

        var plan = createSplitPlan(groups, {
            baseBranch: analysis.baseBranch,
            sourceBranch: analysis.currentBranch,
            branchPrefix: runtime.branchPrefix,
            maxFiles: runtime.maxFiles,
            fileStatuses: analysis.fileStatuses
        });
        state.planCache = plan;
        report.plan = plan;

        if (!runtime.dryRun) {
            var execResult = executeSplit(plan);
            if (execResult.error) {
                report.error = execResult.error;
                return { error: execResult.error, report: report };
            }
            report.splits = execResult.results || [];

            var verifyObj = verifySplits(plan);
            var failures = verifyObj.results.filter(function(r) { return !r.passed; });
            if (failures.length > 0) {
                await resolveConflicts(plan);
            }

            var equiv = verifyEquivalence(plan);
            if (!equiv.equivalent) {
                report.error = 'tree hash mismatch after heuristic split';
            }
        }

        output.print('=== Heuristic Split Complete ===');
        output.print('Splits: ' + plan.splits.length);

        return { error: report.error, report: report };
    }

    // -----------------------------------------------------------------------
    //  isTransientError — classify error messages for retry decisions
    // -----------------------------------------------------------------------

    /**
     * Returns true if the error message looks like a transient failure that
     * is worth retrying (rate limits, timeouts, temporary unavailability).
     * Returns false for permanent errors (invalid tool call, malformed data).
     */
    function isTransientError(msg) {
        if (!msg || typeof msg !== 'string') return true; // unknown → assume transient
        var lc = msg.toLowerCase();
        // Rate-limit / quota patterns.
        if (/rate.?limit|429|too many requests|quota|throttl/i.test(lc)) return true;
        // Timeout / connectivity.
        if (/timeout|timed?.?out|econnreset|econnrefused|epipe|socket hang up|enetunreach/i.test(lc)) return true;
        // Service unavailable.
        if (/503|502|500|service unavailable|internal server error|bad gateway/i.test(lc)) return true;
        // Temporary Claude issues.
        if (/overloaded|capacity|try again/i.test(lc)) return true;
        // Likely permanent: validation, schema, argument errors.
        if (/invalid tool|malformed|schema|unknown tool|argument/i.test(lc)) return false;
        // Default: assume transient (safe to retry).
        return true;
    }
    prSplit._isTransientError = isTransientError;

    // -----------------------------------------------------------------------
    //  resolveConflictsWithClaude — Claude-based conflict resolution
    // -----------------------------------------------------------------------

    // Attempts to fix failing splits using Claude.
    async function resolveConflictsWithClaude(failures, sessionId, timeouts, pollInterval, maxAttemptsPerBranch, report, aliveCheckFn) {
        // Late-bind cross-chunk dependencies.
        var isCancelled = prSplit.isCancelled;
        var resolveDir = prSplit._resolveDir;
        var renderConflictPrompt = prSplit.renderConflictPrompt;
        var validateResolution = prSplit.validateResolution;
        var verifySplit = prSplit.verifySplit;
        var gitExec = prSplit._gitExec;
        var gitAddChangedFiles = prSplit._gitAddChangedFiles;
        var shellQuote = prSplit._shellQuote;
        var osmod = prSplit._modules.osmod;
        var exec = prSplit._modules.exec;
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

        for (var i = 0; i < failures.length; i++) {
            if (Date.now() >= deadline) {
                return { reSplitNeeded: false, reSplitReason: 'wall-clock timeout after ' + (Date.now() - deadlineStart) + 'ms (limit: ' + wallClockMs + 'ms)' };
            }
            if (isCancelled()) {
                return { reSplitNeeded: false, reSplitReason: 'cancelled by user' };
            }

            var fail = failures[i];
            var fixed = false;

            for (var attempt = 0; attempt < maxAttemptsPerBranch && !fixed; attempt++) {
                if (Date.now() >= deadline) {
                    return { reSplitNeeded: false, reSplitReason: 'wall-clock timeout after ' + (Date.now() - deadlineStart) + 'ms (limit: ' + wallClockMs + 'ms)' };
                }
                if (isCancelled()) {
                    return { reSplitNeeded: false, reSplitReason: 'cancelled by user' };
                }

                // Exponential backoff between retry attempts (skip delay on first attempt).
                if (attempt > 0) {
                    var backoffMs = Math.min(2000 * Math.pow(2, attempt - 1), 30000);
                    log.printf('auto-split: retrying %s after %dms backoff (attempt %d/%d)',
                        fail.branch || fail.name, backoffMs, attempt + 1, maxAttemptsPerBranch);
                    await new Promise(function(resolve) { setTimeout(resolve, backoffMs); });
                    if (Date.now() >= deadline) {
                        return { reSplitNeeded: false, reSplitReason: 'wall-clock timeout during backoff' };
                    }
                    if (isCancelled()) {
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
                var resolutionPoll = waitForLogged('reportResolution', timeouts.resolve, {
                    aliveCheck: aliveCheckFn,
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
                var patchWtAdd = gitExec(dir, ['worktree', 'add', patchWorktreeDir, patchBranch]);
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
                    gitAddChangedFiles(patchWorktreeDir);
                    var patchCommit = gitExec(patchWorktreeDir, ['commit', '--amend', '--no-edit']);
                    if (patchCommit.code !== 0) {
                        log.printf('auto-split: patch commit failed for %s: %s', patchBranch, patchCommit.stderr.trim());
                    }
                }

                // Run suggested commands in worktree.
                if (resolution.commands && resolution.commands.length > 0) {
                    for (var c = 0; c < resolution.commands.length; c++) {
                        exec.execv(['sh', '-c', 'cd ' + shellQuote(patchWorktreeDir) + ' && ' + resolution.commands[c]]);
                    }
                    gitAddChangedFiles(patchWorktreeDir);
                    var cmdCommit = gitExec(patchWorktreeDir, ['commit', '--amend', '--no-edit']);
                    if (cmdCommit.code !== 0) {
                        log.printf('auto-split: command commit failed for %s: %s', patchBranch, cmdCommit.stderr.trim());
                    }
                }

                // Cleanup resolution worktree before re-verify.
                gitExec(dir, ['worktree', 'remove', '--force', patchWorktreeDir]);

                // Re-verify this branch (verifySplit creates its own worktree).
                var reVerify = verifySplit(fail.branch || fail.name, { verifyCommand: runtime.verifyCommand });
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

    // -----------------------------------------------------------------------
    //  automatedSplit — the main orchestrator
    // -----------------------------------------------------------------------

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
        var isPaused = prSplit.isPaused;
        var analyzeDiff = prSplit.analyzeDiff;
        var createSplitPlan = prSplit.createSplitPlan;
        var savePlan = prSplit.savePlan;
        var loadPlan = prSplit.loadPlan;
        var DEFAULT_PLAN_PATH = prSplit.DEFAULT_PLAN_PATH;
        var validateClassification = prSplit.validateClassification;
        var validateSplitPlan = prSplit.validateSplitPlan;
        var validatePlan = prSplit.validatePlan;
        var executeSplit = prSplit.executeSplit;
        var verifySplits = prSplit.verifySplits;
        var verifyEquivalence = prSplit.verifyEquivalence;
        var cleanupBranches = prSplit.cleanupBranches;
        var ClaudeCodeExecutor = prSplit.ClaudeCodeExecutor;
        var renderClassificationPrompt = prSplit.renderClassificationPrompt;
        var padIndex = prSplit._padIndex;
        var osmod = prSplit._modules.osmod;
        var resolveDir = prSplit._resolveDir;
        var recordConversation = prSplit.recordConversation || function() {};
        var recordTelemetry = prSplit.recordTelemetry || function() {};
        var assessIndependence = prSplit.assessIndependence || function() { return []; };
        var state = prSplit._state;
        var dir = resolveDir(config.dir || '.');

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
        // Also reset the prSplit-level pointers used by other chunks.
        prSplit._claudeExecutor = null;
        prSplit._mcpCallbackObj = null;

        var timeouts = {
            classify: config.classifyTimeoutMs || AUTOMATED_DEFAULTS.classifyTimeoutMs,
            plan: config.planTimeoutMs || AUTOMATED_DEFAULTS.planTimeoutMs,
            resolve: config.resolveTimeoutMs || AUTOMATED_DEFAULTS.resolveTimeoutMs
        };
        var pollInterval = config.pollIntervalMs || AUTOMATED_DEFAULTS.pollIntervalMs;
        // Use typeof check: 0 is a valid value for retry/re-split counts (meaning "none").
        var maxAttemptsPerBranch = typeof config.maxResolveRetries === 'number' ? config.maxResolveRetries : AUTOMATED_DEFAULTS.maxResolveRetries;
        var maxReSplits = typeof config.maxReSplits === 'number' ? config.maxReSplits : AUTOMATED_DEFAULTS.maxReSplits;

        // Clamp: negative values must not cause spin-loops or nonsensical retries.
        if (pollInterval < 50) { pollInterval = 50; }
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
            if (isCancelled()) {
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
                    savePlan(DEFAULT_PLAN_PATH, lastDone || 'paused');
                    emitOutput('[auto-split] Paused — checkpoint saved to ' + DEFAULT_PLAN_PATH);
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
            // Clean up MCP callback transport.
            var mcpCb = prSplit._mcpCallbackObj;
            if (mcpCb) {
                try { mcpCb.closeSync(); } catch (e) { /* best effort */ }
                prSplit._mcpCallbackObj = null;
                state.mcpCallbackObj = null;
            }

            // On error, emit resume instructions if a plan was saved.
            if (result.error && state.planCache && state.planCache.splits && state.planCache.splits.length > 0) {
                try {
                    savePlan(DEFAULT_PLAN_PATH, report.lastCompletedStep || 'error');
                } catch (e) { /* best effort */ }

                emitOutput('\n[auto-split] Pipeline failed: ' + result.error);
                emitOutput('[auto-split] Plan saved to: ' + DEFAULT_PLAN_PATH);
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
        analysis = await step('Analyze diff', function() {
            updateDetail('Analyze diff', 'Reading git diff...');
            var result = analyzeDiff(config);
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
            var lastHeartbeatTime = Date.now();
            var heartbeatTimeoutMs = config.heartbeatTimeoutMs || (watchdogIdleMs * 2);
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
        var executor = await step('Spawn Claude', function() {
            updateDetail('Spawn Claude', 'Resolving Claude executable...');
            claudeExecutor = state.claudeExecutor;
            if (!claudeExecutor) {
                claudeExecutor = new ClaudeCodeExecutor(prSplitConfig);
                state.claudeExecutor = claudeExecutor;
                prSplit._claudeExecutor = claudeExecutor;
            }
            var resolveResult = claudeExecutor.resolve();
            if (resolveResult.error) {
                return { error: resolveResult.error };
            }
            updateDetail('Spawn Claude', 'Starting Claude process...');
            var spawnOpts = {};
            spawnOpts.mcpConfigPath = mcpCallbackObj.mcpConfigPath;
            var spawnResult = claudeExecutor.spawn(null, spawnOpts);
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
        aliveCheckFn = function() {
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
        var classification = await step('Receive classification', function() {
            updateDetail('Receive classification', 'Waiting for classification...');
            var pollResult = waitForLogged('reportClassification', timeouts.classify, {
                aliveCheck: aliveCheckFn,
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
        var planResult = await step('Generate split plan', function() {
            updateDetail('Generate split plan', 'Checking for Claude-generated plan...');
            var planPoll = waitForLogged('reportSplitPlan', 5000, {
                aliveCheck: aliveCheckFn,
                checkIntervalMs: 1000
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
            var plan = createSplitPlan(groups, {
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
        var execResult = await step('Execute split plan', function() {
            if (runtime.dryRun) {
                return { error: null, dryRun: true };
            }
            updateDetail('Execute split plan', plan.splits.length + ' branches to create...');
            var result = executeSplit(plan, {
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
                var cleanResult = cleanupBranches(plan);
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
            var resumeResolve = claudeExecutor.resolve();
            if (!resumeResolve.error) {
                var resumeSpawn = claudeExecutor.spawn(null, { mcpConfigPath: mcpCallbackObj.mcpConfigPath });
                if (!resumeSpawn.error) {
                    sessionId = resumeSpawn.sessionId;
                    aliveCheckFn = function() {
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
        var verifyResult = await step('Verify splits', function() {
            updateDetail('Verify splits', 'Running verification command on each branch...');
            var verifyObj = verifySplits(plan, {
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
                    timeouts, pollInterval, maxAttemptsPerBranch, report, aliveCheckFn
                );
            });

            // Step 9: Re-split fallback if needed.
            if (resolved.reSplitNeeded && reSplitCount < maxReSplits) {
                reSplitCount++;
                output.print('[auto-split] Re-split requested — re-classifying...');
                cleanupBranches(plan);
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
                    var rePoll = waitForLogged('reportClassification', timeouts.classify, {
                        aliveCheck: aliveCheckFn,
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
                    plan = createSplitPlan(newGroups, {
                        baseBranch: analysis.baseBranch,
                        sourceBranch: analysis.currentBranch,
                        branchPrefix: runtime.branchPrefix,
                        maxFiles: runtime.maxFiles,
                        fileStatuses: analysis.fileStatuses
                    });
                    state.planCache = plan;
                    report.plan = plan;
                    var reExec = await step('Re-execute split', function() {
                        var result = executeSplit(plan);
                        if (result.error) return { error: result.error };
                        report.splits = result.results || [];
                        return { error: null };
                    });
                    if (!reExec.error) {
                        await step('Re-verify splits', function() {
                            return { error: null, results: verifySplits(plan) };
                        });
                    }
                }
            }

            // Checkpoint after resolve/re-split.
            savePlan(null, 'Resolve conflicts');
        }

        // Step 10: Equivalence check and report.
        var equivResult = await step('Verify equivalence', function() {
            var result = verifyEquivalence(plan);
            return { error: result.equivalent ? null : 'tree hash mismatch', result: result };
        });

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

        cleanupExecutor();

        // Clean up split branches on pipeline failure if configured.
        if (config.cleanupOnFailure && report.error && plan && plan.splits && plan.splits.length > 0) {
            emitOutput('[auto-split] Cleaning up split branches due to pipeline failure...');
            var cleanResult = cleanupBranches(plan);
            if (cleanResult.deleted.length > 0) {
                emitOutput('[auto-split] Deleted ' + cleanResult.deleted.length + ' branches');
            }
        }

        // Worktree isolation: user's branch is never modified, no restore needed.

        return finishTUI({ error: report.error, report: report });
    }

    // -----------------------------------------------------------------------
    //  Exports
    // -----------------------------------------------------------------------

    prSplit.AUTOMATED_DEFAULTS = AUTOMATED_DEFAULTS;
    prSplit.SEND_TEXT_NEWLINE_DELAY_MS = SEND_TEXT_NEWLINE_DELAY_MS;
    prSplit.SEND_TEXT_CHUNK_BYTES = SEND_TEXT_CHUNK_BYTES;
    prSplit.SEND_TEXT_CHUNK_DELAY_MS = SEND_TEXT_CHUNK_DELAY_MS;
    prSplit.SEND_PRE_SUBMIT_STABLE_TIMEOUT_MS = SEND_PRE_SUBMIT_STABLE_TIMEOUT_MS;
    prSplit.SEND_PRE_SUBMIT_STABLE_POLL_MS = SEND_PRE_SUBMIT_STABLE_POLL_MS;
    prSplit.SEND_PRE_SUBMIT_STABLE_SAMPLES = SEND_PRE_SUBMIT_STABLE_SAMPLES;
    prSplit.SEND_INPUT_ANCHOR_TAIL_CHARS = SEND_INPUT_ANCHOR_TAIL_CHARS;
    prSplit.SEND_SUBMIT_ACK_TIMEOUT_MS = SEND_SUBMIT_ACK_TIMEOUT_MS;
    prSplit.SEND_SUBMIT_ACK_POLL_MS = SEND_SUBMIT_ACK_POLL_MS;
    prSplit.SEND_SUBMIT_ACK_STABLE_SAMPLES = SEND_SUBMIT_ACK_STABLE_SAMPLES;
    prSplit.SEND_SUBMIT_MAX_NEWLINE_ATTEMPTS = SEND_SUBMIT_MAX_NEWLINE_ATTEMPTS;
    prSplit.sendToHandle = sendToHandle;
    prSplit.waitForLogged = waitForLogged;
    prSplit.classificationToGroups = classificationToGroups;
    prSplit.cleanupExecutor = cleanupExecutor;
    prSplit.heuristicFallback = heuristicFallback;
    prSplit.resolveConflictsWithClaude = resolveConflictsWithClaude;
    prSplit.automatedSplit = automatedSplit;

})(globalThis.prSplit);
