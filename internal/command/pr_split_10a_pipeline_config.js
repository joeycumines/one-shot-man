'use strict';
// pr_split_10a_pipeline_config.js — Pipeline: constants, defaults, and utility functions
// Dependencies: chunks 00-09 must be loaded first.

(function(prSplit) {

    // --- Constants ---

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
    //      → mcpCallbackObj.waitForAsync(toolName, timeoutMs, opts)
    //      → resolveConflictsWithAgent(..., { resolveWallClockTimeoutMs })
    //      → verifySplit(..., { verifyTimeoutMs })
    //
    //   SCOPE: prSplitConfig.timeoutMs fans out to classifyTimeoutMs,
    //   planTimeoutMs, resolveTimeoutMs, and verifyTimeoutMs only. It bounds
    //   waitForAsync MCP waits and the verify wrapper. It does not bound the
    //   spawned agent PTY lifetime, which is governed by pipeline, wall-clock,
    //   and watchdog timers plus explicit user cancel.
    var AUTOMATED_DEFAULTS = {
        classifyTimeoutMs: 300000,  // 5 minutes for classification (generous for LLM analysis)
        planTimeoutMs: 300000,      // 5 minutes for plan generation
        resolveTimeoutMs: 1800000,  // 30 minutes for conflict resolution
        pollIntervalMs: 500,        // Poll every 500ms for fast cancellation
        minPollIntervalMs: 50,      // Floor clamp to prevent spin-loops
        maxResolveRetries: 3,       // Retries per branch
        maxReSplits: 1,             // Maximum re-classification cycles
        resolveWallClockTimeoutMs: 7200000, // 120 minutes wall-clock cap
        verifyTimeoutMs: 600000,    // 10 minutes per branch for verify step
        pipelineTimeoutMs: 7200000, // 120 minutes overall pipeline timeout
        stepTimeoutMs: 3600000,     // 60 minutes per step
        watchdogIdleMs: 900000,     // 15 minutes no-progress watchdog
        agentHealthPollMs: 5000,   // TUI polls isAlive() every 5 seconds
        agentHeartbeatTimeoutMs: 60000, // 60 seconds heartbeat timeout
        resolveCommandTimeoutMs: 120000, // 2 minutes per resolution command
        resolveWallClockGraceMs: 60000, // Grace period added to computed wall-clock timeout
        resolveBackoffBaseMs: 2000,     // Exponential backoff base interval between retries
        resolveBackoffCapMs: 30000,     // Maximum backoff cap (30 seconds)
        spawnHealthCheckDelayMs: 300,   // Post-spawn delay before isAlive() check
        planPollTimeoutMs: 5000,    // Short poll for Agent-generated plan
        planPollCheckIntervalMs: 1000 // Check interval for plan poll
    };

    // Delay between text and newline writes to defeat PTY coalescing.
    var SEND_TEXT_NEWLINE_DELAY_MS = 10;
    // Chunk large prompts into smaller writes so PTY consumers read them
    // incrementally instead of one giant burst. Despite the name, this is
    // a CHARACTER limit (JavaScript substring operates on code units). For
    // ASCII prompts the distinction is irrelevant; multi-byte text may
    // produce larger wire payloads per chunk.
    var SEND_TEXT_CHUNK_BYTES = 512;
    // Delay between chunk writes to reduce PTY coalescing into a single
    // read burst on the Agent side.
    var SEND_TEXT_CHUNK_DELAY_MS = 2;
    // Wait for prompt/input anchors to stabilize before pressing Enter.
    // T203: Increased timeout from 1500→3000ms and reduced stable samples
    // from 3→2 to handle large prompts on slow terminals (e.g. ollama).
    var SEND_PRE_SUBMIT_STABLE_TIMEOUT_MS = 3000;
    var SEND_PRE_SUBMIT_STABLE_POLL_MS = 50;
    var SEND_PRE_SUBMIT_STABLE_SAMPLES = 2;
    var SEND_INPUT_ANCHOR_TAIL_CHARS = 28;
    // After pressing Enter, require anchor movement (prompt/input) that is
    // stable for multiple polls.
    var SEND_SUBMIT_ACK_TIMEOUT_MS = 1500;
    var SEND_SUBMIT_ACK_POLL_MS = 50;
    var SEND_SUBMIT_ACK_STABLE_SAMPLES = 2;
    var SEND_SUBMIT_MAX_NEWLINE_ATTEMPTS = 3;
    // Wait for Agent prompt marker before sending text. This prevents
    // early writes into startup/setup screens where input is not ready.
    var SEND_PROMPT_READY_TIMEOUT_MS = 10000;
    // OpenCode may spend longer initializing its interactive TUI (provider
    // loading, project scan, and first-run state) before drawing the composer.
    // Keep this provider-specific so generic CLIs retain the shorter bound.
    var SEND_OPENCODE_PROMPT_READY_TIMEOUT_MS = 60000;
    var SEND_PROMPT_READY_POLL_MS = 100;
    var SEND_PROMPT_READY_STABLE_SAMPLES = 2;

    // --- Utility functions ---

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
            openCodePromptReadyTimeoutMs: resolveNumber(prSplit.SEND_OPENCODE_PROMPT_READY_TIMEOUT_MS, SEND_OPENCODE_PROMPT_READY_TIMEOUT_MS, 1),
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

    // --- classificationToGroups — pure conversion function ---

    // Converts a classification result to groups map
    // (category→{files: [...], description: "..."}).
    // Accepts both new format (array of {name, description, files}) and
    // legacy format ({file: category} map) for backwards compatibility.
    function classificationToGroups(classification, preserveDuplicates) {
        var groups = Object.create(null);
        var assignedFiles = Object.create(null);

        function addCategory(name, files, description, title, summary, keyChanges, verificationSteps, rationale) {
            if (!name) return;
            if (!Object.prototype.hasOwnProperty.call(groups, name)) {
                var groupObj = {
                    files: [],
                    description: description || ''
                };
                if (title) groupObj.title = title;
                if (summary) groupObj.summary = summary;
                if (keyChanges) groupObj.keyChanges = keyChanges;
                if (verificationSteps) groupObj.verificationSteps = verificationSteps;
                if (rationale) groupObj.rationale = rationale;
                groups[name] = groupObj;
            } else {
                if (!groups[name].description && description) groups[name].description = description;
                if (!groups[name].title && title) groups[name].title = title;
                if (!groups[name].summary && summary) groups[name].summary = summary;
                if (!groups[name].keyChanges && keyChanges) groups[name].keyChanges = keyChanges;
                if (!groups[name].verificationSteps && verificationSteps) groups[name].verificationSteps = verificationSteps;
                if (!groups[name].rationale && rationale) groups[name].rationale = rationale;
            }
            if (!Array.isArray(files)) return;
            for (var fi = 0; fi < files.length; fi++) {
                var file = files[fi];
                if (typeof file !== 'string' || !file) continue;
                if (!preserveDuplicates && assignedFiles[file]) continue;
                assignedFiles[file] = true;
                groups[name].files.push(file);
            }
        }

        if (Array.isArray(classification)) {
            for (var i = 0; i < classification.length; i++) {
                var cat = classification[i];
                // Skip null/undefined items (was throwing TypeError on null.name access)
                if (!cat) continue;
                // Plain string item — skip
                if (typeof cat === 'string') continue;
                // Object with a name field — standard format
                if (typeof cat === 'object' && cat.name) {
                    addCategory(cat.name, cat.files, cat.description, cat.title, cat.summary, cat.keyChanges, cat.verificationSteps, cat.rationale);
                    continue;
                }
                // Object without name but with files — synthetic group
                // Only synthetic if name doesn't exist (undefined/null); empty string '' means skip
                if (typeof cat === 'object' && cat.name !== '' && !cat.name && cat.files) {
                    addCategory('group' + (i + 1),
                        Array.isArray(cat.files) ? cat.files : [cat.files],
                        cat.description, cat.title, cat.summary, cat.keyChanges, cat.verificationSteps, cat.rationale);
                    continue;
                }
                // Plain array — treat as a list of files in a synthetic group
                if (Array.isArray(cat)) {
                    addCategory('group' + (i + 1), cat, '');
                    continue;
                }
            }
        } else if (classification && typeof classification === 'object') {
            for (var path in classification) {
                var catName = classification[path];
                addCategory(catName, [path], '');
            }
        }
        return groups;
    }

    // --- cleanupExecutor — resource cleanup ---
    // Closes the Agent executor and cleans up resources. After close, the
    // session model (isDone) signals the pipeline's aliveCheckFn and the
    // TUI health poll — no explicit detach needed. Async: awaits the handle
    // close (TrackPromise Promise); every async call site awaits it. The two
    // sync discard paths (16e d-key, 16f mouse) plus the wizard-exit path
    // (16c confirmCancel) fire-and-forget the same chain because a sync
    // BubbleTea update cannot await; process exit owns teardown there.
    async function cleanupExecutor() {
        var isForceCancelled = prSplit.isForceCancelled;
        var agentExec = prSplit._state.agentExecutor;
        var forceNow = false;
        try { forceNow = !!isForceCancelled(); } catch (e) { forceNow = false; }
        log.printf('auto-split cleanupExecutor: start force=%s hasExecutor=%s', forceNow ? 'true' : 'false', agentExec ? 'true' : 'false');

        // Close the executor FIRST so the child PTY fd is released.
        if (agentExec) {
            if (forceNow && agentExec.handle &&
                typeof agentExec.handle.signal === 'function') {
                log.printf('auto-split cleanupExecutor: sending SIGKILL to Agent handle before close');
                try { agentExec.handle.signal('SIGKILL'); } catch (e) {
                    log.printf('auto-split cleanupExecutor: pre-close SIGKILL error: %s', e.message || String(e));
                }
            }
            log.printf('auto-split cleanupExecutor: closing Agent executor');
            try {
                await agentExec.close();
                log.printf('auto-split cleanupExecutor: Agent executor closed');
            } catch (e) {
                log.printf('auto-split cleanupExecutor: Agent close error: %s', e.message || String(e));
            }
        }
        // No synchronous detach. When the child PTY closes, the session
        // model fires Done() (event-driven) and tuiMux.isDone(agentSessionID)
        // returns true. The pipeline and TUI poll the pinned SessionManager
        // ID directly — the old
        // st.agentCrashDetected flag is no longer needed.
        if (typeof tuiMux !== 'undefined' && tuiMux) {
            log.printf('auto-split cleanupExecutor: child closed — pinned isDone() will signal completion');
        }
        log.printf('auto-split cleanupExecutor: done');
    }

    // --- isTransientError — classify error messages for retry decisions ---

    // isTransientError returns true if the error looks like a transient failure
    // worth retrying (rate limits, timeouts, temporary unavailability).
    function isTransientError(msg) {
        if (!msg || typeof msg !== 'string') return true; // unknown → assume transient
        var lc = msg.toLowerCase();
        // Rate-limit / quota patterns.
        if (/rate.?limit|429|too many requests|quota|throttl/i.test(lc)) return true;
        // Timeout / connectivity.
        if (/timeout|timed?.?out|econnreset|econnrefused|epipe|socket hang up|enetunreach/i.test(lc)) return true;
        // Service unavailable.
        if (/503|502|500|service unavailable|internal server error|bad gateway/i.test(lc)) return true;
        // Temporary Agent issues.
        if (/overloaded|capacity|try again/i.test(lc)) return true;
        // Likely permanent: validation, schema, argument errors.
        if (/invalid tool|malformed|schema|unknown tool|argument/i.test(lc)) return false;
        // Default: assume transient (safe to retry).
        return true;
    }

    // --- Cross-chunk exports — constants ---
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
    prSplit.SEND_OPENCODE_PROMPT_READY_TIMEOUT_MS = SEND_OPENCODE_PROMPT_READY_TIMEOUT_MS;

    // Cross-chunk exports — utility functions.
    prSplit._resolveSendConfig = resolveSendConfig;
    prSplit._getCancellationError = getCancellationError;
    prSplit._resolveNumber = resolveNumber;
    prSplit.classificationToGroups = classificationToGroups;
    prSplit.cleanupExecutor = cleanupExecutor;
    prSplit._isTransientError = isTransientError;

})(globalThis.prSplit);
