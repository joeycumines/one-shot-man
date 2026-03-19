'use strict';
// pr_split_10b_pipeline_send.js — Pipeline: PTY send functions, prompt detection, anchor stability
// Dependencies: chunks 00-10a must be loaded first.

(function(prSplit) {
    // Cross-chunk imports from 10a.
    var resolveSendConfig = prSplit._resolveSendConfig;
    var getCancellationError = prSplit._getCancellationError;
    var TRUNCATION_WIDTH = 120;

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
        var pollCount = 0;
        var bestAnchorsState = null; // T203: track best observation even if transient
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
            pollCount++;
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
            // T203: Track best observation with both anchors valid.
            if (anchorsReady) {
                bestAnchorsState = state;
            }
            if (anchorsReady && stableCount >= cfg.preSubmitStableSamples) {
                return { error: null, state: state, observed: true };
            }
            await new Promise(function(resolve) { setTimeout(resolve, cfg.preSubmitStablePollMs); });
        }

        // T203: Graceful fallback — if we ever saw valid anchors, use that snapshot.
        if (bestAnchorsState) {
            log.printf('auto-split sendToHandle: anchor stabilization timeout (%dms, %d polls) — using best observed snapshot (prompt=%d, input=%d)',
                Date.now() - startMs, pollCount,
                bestAnchorsState.promptLineIndex, bestAnchorsState.inputLineIndex);
            return { error: null, state: bestAnchorsState, observed: true };
        }
        // T203: Secondary fallback — prompt-only mode for long pastes where
        // the text tail scrolled off-screen. If we have a stable prompt
        // marker, proceed cautiously.
        if (lastState &&
            lastState.promptBottom !== -1 &&
            lastState.promptLineIndex !== -1) {
            log.printf('auto-split sendToHandle: anchor stabilization timeout (%dms, %d polls) — prompt-only fallback (prompt=%d, input=%d, key=%s)',
                Date.now() - startMs, pollCount,
                lastState.promptLineIndex, lastState.inputLineIndex,
                lastState.stableKey);
            return { error: null, state: lastState, observed: true };
        }
        // T204: Diagnostic logging on hard failure.
        log.printf('auto-split sendToHandle: anchor detection FAILED (%dms, %d polls) — lastState: prompt=%d, input=%d, key=%s',
            Date.now() - startMs, pollCount,
            lastState ? lastState.promptLineIndex : -1,
            lastState ? lastState.inputLineIndex : -1,
            lastState ? lastState.stableKey : 'null');
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
        var truncated = text.length > TRUNCATION_WIDTH ? text.substring(0, TRUNCATION_WIDTH) + '...' : text;
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

    // Cross-chunk exports.
    prSplit.sendToHandle = sendToHandle;
    prSplit._getTextTailAnchor = getTextTailAnchor;
    prSplit._findPromptMarker = findPromptMarker;
    prSplit._isPromptMarkerLine = isPromptMarkerLine;
    prSplit._detectPromptBlocker = detectPromptBlocker;
    prSplit._captureInputAnchors = captureInputAnchors;
    prSplit._captureScreenshot = captureScreenshot;

})(globalThis.prSplit);
