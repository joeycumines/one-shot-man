'use strict';
// pr_split_16c_tui_handlers_verify.js — TUI: verify handlers, confirm cancel, Claude conversation, error resolution
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
    var cleanupShellPaneSession = prSplit._cleanupShellPaneSession;
    var clearVerifyPaneSession = prSplit._clearVerifyPaneSession;
    var handleErrorResolutionState = prSplit._handleErrorResolutionState;
    // Late-bound cross-chunk references (defined in later chunks, resolved at call time).
    function startAnalysis(s) { return prSplit._startAnalysis(s); }
    function startEquivCheck(s) { return prSplit._startEquivCheck(s); }

    // --- Update Handlers — screen-specific input handling ---
    function updateConfirmCancel(msg, s) {
        // Helper to clean up any active verify session before quitting.
        function cleanupActiveSession() {
            // T325: Reset tab before clearing session for atomic state transition.
            if (s.splitViewTab === 'verify' || s.splitViewTab === 'shell') {
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
            cleanupActiveSession();
            // T393: Clean up Claude executor and MCP callback on wizard exit.
            if (st && st.claudeExecutor) {
                try { st.claudeExecutor.close(); } catch (e) { log.debug('cleanup: claudeExec.close failed: ' + (e.message || e)); }
            }
            var mcpCb = prSplit._mcpCallbackObj;
            if (mcpCb) {
                try { mcpCb.closeSync(); } catch (e) { log.debug('cleanup: mcpCb.closeSync failed: ' + (e.message || e)); }
                prSplit._mcpCallbackObj = null;
                if (st) st.mcpCallbackObj = null;
            }
            s.wizard.cancel();
            s.wizardState = 'CANCELLED';
            return [s, tea.quit()];
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
        if (msg.type === 'Mouse' && msg.action === 'press' && !msg.isWheel) {
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

    // --- Per-branch verification (tick-based stepping) ---
    // Verifies one branch at a time. Uses CaptureSession (PTY + VTerm)
    // for live output when available, falling back to async verifySplitAsync
    // on platforms without PTY support (Windows).
    function runVerifyBranch(s) {
        if (!s.isProcessing) return [s, null];

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
        // pollVerifySession / handleVerifyFallbackPoll can tag failures as
        // pre-existing when the baseline also fails.
        if (s.verifyingIdx === 0 && !s._baselineVerifyStarted) {
            var sourceBranch = st.planCache.sourceBranch;
            if (sourceBranch && prSplit.runtime.verifyCommand) {
                s._baselineVerifyStarted = true;
                s._baselineVerifyResult = null;
                var baseDir = prSplit.runtime.dir || '.';
                var baseTimeoutMs = (typeof prSplitConfig !== 'undefined' && prSplitConfig.timeoutMs)
                    ? prSplitConfig.timeoutMs : 0;
                prSplit.verifySplitAsync(sourceBranch, {
                    dir: baseDir,
                    verifyCommand: prSplit.runtime.verifyCommand,
                    verifyTimeoutMs: baseTimeoutMs,
                    outputFn: null
                }).then(function(result) {
                    s._baselineVerifyResult = {
                        failed: !result.passed,
                        sourceBranch: sourceBranch
                    };
                }, function() {
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

        // Try CaptureSession for live output (PTY-based).
        var sessionResult = prSplit.startVerifySession(branchName, {
            dir: dir,
            verifyCommand: scopedCmd,
            rows: C.DEFAULT_ROWS,
            cols: Math.max(80, (s.width || 80) - 8)
        });

        if (sessionResult.skipped) {
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

        if (sessionResult.error && !sessionResult.session) {
            // CaptureSession failed to start — use async verifySplitAsync.
            s.verifyFallbackRunning = true;
            s.verifyFallbackError = null;

            // T352: Set verify display state for fallback path so the inline
            // terminal and Verify tab can show live output even without PTY.
            s.activeVerifyBranch = branchName;
            s.activeVerifyStartTime = Date.now();
            s.verifyElapsedMs = 0;
            s.verifyScreen = '';
            s.verifyAutoScroll = true;
            s.verifyViewportOffset = 0;
            s.verifyOutput[branchName] = []; // pre-initialize for live outputFn

            // T352+T380: Auto-open split-view with Verify tab in fallback path.
            if (!s.splitViewEnabled && s.height >= C.INLINE_VIEW_HEIGHT) {
                s.splitViewEnabled = true;
                s.splitViewFocus = 'claude';
                s.splitViewTab = 'verify';
                if (typeof prSplit._syncMainViewport === 'function') {
                    prSplit._syncMainViewport(s);
                }
            } else if (s.splitViewEnabled) {
                s.splitViewTab = 'verify';
            }

            var timeoutMs = (typeof prSplitConfig !== 'undefined' && prSplitConfig.timeoutMs) ? prSplitConfig.timeoutMs : 0;
            runVerifyFallbackAsync(s, branchName, dir, scopedCmd, timeoutMs).then(
                function() {
                    s.verifyFallbackRunning = false;
                },
                function(err) {
                    s.verifyFallbackRunning = false;
                    s.verifyFallbackError = (err && err.message) ? err.message : String(err);
                }
            );

            // Poll at 100ms for completion.
            return [s, tea.tick(C.TICK_INTERVAL_MS, 'verify-fallback-poll')];
        }

        // CaptureSession started — store state for polling.
        s.activeVerifySession = sessionResult.session;
        s.activeVerifyWorktree = sessionResult.worktreeDir;
        s.activeVerifyBranch = branchName;
        s.activeVerifyDir = sessionResult.dir;
        s.activeVerifyStartTime = sessionResult.startTime;
        s.verifyElapsedMs = 0;   // T058: reset elapsed for new session
        s.verifyScreen = '';     // T321: clear screen for new session
        s.verifyViewportOffset = 0;
        s.verifyAutoScroll = true;

        // T325+T380: Auto-open split-view with Verify tab when verification starts.
        if (!s.splitViewEnabled && s.height >= C.INLINE_VIEW_HEIGHT) {
            s.splitViewEnabled = true;
            s.splitViewFocus = 'claude';
            s.splitViewTab = 'verify';
            if (typeof prSplit._syncMainViewport === 'function') {
                prSplit._syncMainViewport(s);
            }
        } else if (s.splitViewEnabled) {
            s.splitViewTab = 'verify';
        }

        // Poll every 100ms for live output updates.
        return [s, tea.tick(C.TICK_INTERVAL_MS, 'verify-poll')];
    }

    // --- Live verification poll ---
    // Polls the active CaptureSession for output and completion.
    // On completion, records the result and advances to next branch.
    function pollVerifySession(s) {
        var activeVerifySession = getInteractivePaneSession(s, 'verify');
        if (!activeVerifySession) return [s, null];

        // T058: Update elapsed time on each tick for live display.
        s.verifyElapsedMs = Date.now() - s.activeVerifyStartTime;

        // Check timeout.
        var timeoutMs = (typeof prSplitConfig !== 'undefined' && prSplitConfig.timeoutMs)
            ? prSplitConfig.timeoutMs : 0;
        if (timeoutMs > 0 && s.verifyElapsedMs >= timeoutMs) {
            // Timeout — kill the process.
            try { activeVerifySession.kill(); } catch (e) { log.debug('verifyCancel: session.kill failed: ' + (e.message || e)); }
        }

        // T321: Capture ANSI-styled VTerm screen for the Verify tab.
        try { s.verifyScreen = activeVerifySession.screen(); } catch (e) { log.debug('pollVerify: session.screen failed: ' + (e.message || e)); }

        // T350: Auto-scroll main viewport to keep inline terminal visible.
        // The inline verify terminal in viewExecutionScreen() is rendered at
        // the bottom of the content. Without this, the branch list fills the
        // visible area and the live terminal is below the fold. We only
        // auto-scroll when the user hasn't manually scrolled (verifyAutoScroll
        // is true) — scrolling Up sets it false, scrolling to End resets it.
        if (s.vp && s.verifyAutoScroll !== false) {
            try { s.vp.gotoBottom(); } catch (e) { log.debug('pollVerify: viewport.gotoBottom failed: ' + (e.message || e)); }
        }

        if (!activeVerifySession.isDone()) {
            // Still running — schedule next poll.
            s.spinnerFrame = (s.spinnerFrame || 0) + 1;
            return [s, tea.tick(C.TICK_INTERVAL_MS, 'verify-poll')];
        }

        // Process exited — capture result.
        var exitCode = activeVerifySession.exitCode();
        var output = activeVerifySession.output();
        var duration = Date.now() - s.activeVerifyStartTime;
        var branchName = s.activeVerifyBranch;

        // Store output lines for expandable display.
        var outputLines = output.split('\n').filter(function(line) { return line.length > 0; });
        s.verifyOutput[branchName] = outputLines;

        // T44: Pipe verification output to the Output tab buffer.
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

        // Check if timeout was the cause.
        var isTimeout = timeoutMs > 0 && duration >= timeoutMs;
        var errorMsg = null;
        if (isTimeout) {
            errorMsg = 'verify timeout after ' + duration + 'ms (limit: ' + timeoutMs + 'ms)';
        } else if (exitCode !== 0) {
            errorMsg = 'verify failed (exit ' + exitCode + ')';
        }

        // T115: Detect pre-existing failures using cached baseline result.
        var preExisting = false;
        if ((exitCode !== 0 || isTimeout) && _isPreExistingFailure(s)) {
            preExisting = true;
            if (errorMsg) {
                errorMsg += _preExistingAnnotation(s);
            }
        }

        s.verificationResults.push({
            name: branchName,
            status: (exitCode === 0 && !isTimeout) ? prSplit._branchStatuses.PASSED : prSplit._branchStatuses.FAILED,
            passed: exitCode === 0 && !isTimeout,
            skipped: false,
            error: errorMsg,
            output: output,
            duration: duration,
            preExisting: preExisting
        });

        // T380: Preserve verifyScreen, activeVerifyBranch, and verifyElapsedMs
        // for post-mortem viewing in the pane title. Cleared on next verify start.
        clearVerifyPaneSession(s, { debugPrefix: 'verifyDone', keepDisplay: true });

        s.verifyingIdx++;
        return [s, tea.tick(1, 'verify-branch')];
    }

    // --- Async verify fallback (when CaptureSession unavailable) ---
    // Uses verifySplitAsync for non-blocking verification. The result
    // is stored directly on s so the poll handler can consume it.
    async function runVerifyFallbackAsync(s, branchName, dir, scopedCmd, timeoutMs) {
        if (!s.isProcessing || s.wizard.current === 'CANCELLED') return;

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
                outputLines.push(line);
                // T352: Populate verifyScreen with latest fallback output so
                // the inline terminal and Verify tab show live output.
                var rows = Math.min(C.DEFAULT_ROWS, outputLines.length);
                s.verifyScreen = outputLines.slice(-rows).join('\n');
            }
        });
        var duration = Date.now() - branchStart;

        if (!s.isProcessing || s.wizard.current === 'CANCELLED') return;

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
        // T380: Keep verify tab visible for post-mortem review; only switch shell away.
        if (s.splitViewTab === 'shell') {
            s.splitViewTab = 'verify';
        }

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

    // --- Claude Conversation (T16) ---
    // openClaudeConvo opens the conversation overlay.
    function openClaudeConvo(s, context) {
        // Check Claude availability.
        var executor = st.claudeExecutor;
        if (!executor || !executor.handle) {
            s.claudeConvo.lastError = 'Claude is not running. Select "auto" strategy and run analysis first.';
            s.claudeConvo.active = true;
            s.claudeConvo.context = context;
            return [s, null];
        }
        if (typeof executor.handle.isAlive === 'function' && !executor.handle.isAlive()) {
            s.claudeConvo.lastError = 'Claude process has exited. Restart analysis to reconnect.';
            s.claudeConvo.active = true;
            s.claudeConvo.context = context;
            return [s, null];
        }

        s.claudeConvo.active = true;
        s.claudeConvo.context = context;
        s.claudeConvo.inputText = '';
        s.claudeConvo.lastError = null;
        s.claudeConvo.scrollOffset = 0;
        return [s, null];
    }

    // closeClaudeConvo dismisses the conversation overlay.
    function closeClaudeConvo(s) {
        s.claudeConvo.active = false;
        s.claudeConvo.inputText = '';
        s.claudeConvo.scrollOffset = 0;
        // Keep history and context for re-opening.
        return [s, null];
    }

    // updateClaudeConvo handles input while conversation overlay is active.
    function updateClaudeConvo(msg, s) {
        var convo = s.claudeConvo;

        if (msg.type === 'Key') {
            var k = msg.key;

            // Escape closes the overlay.
            if (k === 'esc') {
                return closeClaudeConvo(s);
            }

            // Enter submits the current input (if not already sending).
            if (k === 'enter' && !convo.sending) {
                var text = (convo.inputText || '').trim();
                if (text.length > 0) {
                    return submitClaudeMessage(s, text);
                }
                return [s, null];
            }

            // Backspace.
            if (k === 'backspace' && !convo.sending) {
                var t = convo.inputText || '';
                if (t.length > 0) {
                    convo.inputText = t.substring(0, t.length - 1);
                }
                return [s, null];
            }

            // Ctrl+U: clear input line.
            if (k === 'ctrl+u' && !convo.sending) {
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

            // Single printable char — accumulate (when not sending).
            if (k.length === 1 && !convo.sending) {
                convo.inputText = (convo.inputText || '') + k;
                return [s, null];
            }

            return [s, null];
        }

        // Mouse wheel scrolls history.
        if (msg.type === 'Mouse' && msg.isWheel) {
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

    // buildClaudePrompt constructs the prompt based on conversation context.
    function buildClaudePrompt(context, userMessage, s) {
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

    // submitClaudeMessage launches the async send + wait operation.
    function submitClaudeMessage(s, text) {
        var convo = s.claudeConvo;
        var executor = st.claudeExecutor;

        if (!executor || !executor.handle) {
            convo.lastError = 'Claude is not running.';
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
        var prompt = buildClaudePrompt(convo.context, text, s);

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
                    // Wait for Claude to call the expected MCP tool.
                    var waitResult = await prSplit.waitForLogged(toolToWait, timeoutMs, {
                        aliveCheck: function() {
                            return (executor.handle &&
                                typeof executor.handle.isAlive === 'function' &&
                                executor.handle.isAlive());
                        }
                    });
                    if (waitResult && waitResult.data) {
                        convoState.history.push({
                            role: 'claude',
                            text: formatClaudeResponse(toolToWait, waitResult.data),
                            ts: Date.now()
                        });
                        // Process structured response.
                        processClaudeConvoResult(convoState, toolToWait, waitResult.data);
                    } else if (waitResult && waitResult.error) {
                        convoState.lastError = 'Claude: ' + waitResult.error;
                    } else {
                        // No tool call received — Claude may have responded in free text.
                        // Capture the latest screenshot as response.
                        var shot = '';
                        if (typeof tuiMux !== 'undefined' && tuiMux &&
                            typeof tuiMux.screenshot === 'function') {
                            try { shot = String(tuiMux.screenshot() || ''); } catch (e) { log.debug('screenCapture: tuiMux.screenshot failed: ' + (e.message || e)); }
                        }
                        if (shot) {
                            convoState.history.push({
                                role: 'claude',
                                text: '[screenshot]\n' + shot.substring(shot.length - C.SCREENSHOT_CAPTURE_CHARS),
                                ts: Date.now()
                            });
                        }
                    }
                } else {
                    // No specific tool to wait for — just note the send succeeded.
                    convoState.history.push({
                        role: 'claude',
                        text: '(message sent — check Claude pane for response)',
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
        return [s, tea.tick(C.CONVO_POLL_MS, 'claude-convo-poll')];
    }

    // formatClaudeResponse formats structured MCP tool response for display.
    function formatClaudeResponse(toolName, data) {
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

    // processClaudeConvoResult applies structured result to wizard state.
    function processClaudeConvoResult(convo, toolName, data) {
        // T122: MCP schema uses 'stages'; accept both field names.
        var splits = (toolName === 'reportSplitPlan' && data) ? (data.stages || data.splits) : null;
        if (splits) {
            // Update the plan cache with the revised plan from Claude.
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

    // pollClaudeConvo checks async send/wait progress.
    function pollClaudeConvo(s) {
        var convo = s.claudeConvo;

        // If still sending, keep polling.
        if (convo.sending) {
            return [s, tea.tick(C.CONVO_POLL_MS, 'claude-convo-poll')];
        }

        // T122: If plan was revised by Claude, reset split selection.
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
        if (choice === 'restart-claude') {
            var executor = st.claudeExecutor;
            if (!executor) {
                s.errorDetails = 'No Claude executor available for restart.';
                return [s, null];
            }
            var restartOpts = {};
            if (prSplit._mcpCallbackObj && prSplit._mcpCallbackObj.mcpConfigPath) {
                restartOpts.mcpConfigPath = prSplit._mcpCallbackObj.mcpConfigPath;
            }
            // Non-blocking restart: launch as Promise, poll via tick.
            s.claudeRestarting = true;
            s.restartResult = null;
            s.errorDetails = 'Restarting Claude...';
            executor.restart(null, restartOpts).then(function(restartResult) {
                s.claudeRestarting = false;
                s.restartResult = restartResult;
            }, function(err) {
                s.claudeRestarting = false;
                s.restartResult = { error: 'Claude restart error: ' + ((err && err.message) || String(err)) };
            });
            return [s, tea.tick(C.AUTO_SPLIT_POLL_MS, 'restart-claude-poll')];
        }

        if (choice === 'fallback-heuristic') {
            s.claudeCrashDetected = false;
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
            // Switch to Claude pane — user fixes manually. Store context
            // so the execution screen can show instructions when user
            // returns from Claude.
            s.manualFixContext = {
                failedBranches: (result && result.failedBranches) ||
                    (s.wizard.data && s.wizard.data.failedBranches) || []
            };
            if (typeof tuiMux !== 'undefined' && tuiMux &&
                typeof tuiMux.switchTo === 'function' &&
                typeof tuiMux.session === 'function' &&
                tuiMux.session().isRunning()) {
                tuiMux.switchTo();
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

    // T335: Poll shell CaptureSession for screen updates and lifecycle.
    function pollShellSession(s) {
        var shellSession = getInteractivePaneSession(s, 'shell');
        if (!shellSession) return [s, null];

        // Capture screen.
        try {
            s.shellScreen = shellSession.screen();
        } catch (e) {
            s.shellScreen = '';
        }

        // Check if shell has exited.
        var done = false;
        try { done = shellSession.isDone(); } catch (e) { done = true; }

        if (done) {
            cleanupShellPaneSession(s, 'tabSwitch');

            // Resume verify if it was paused for the shell.
            var activeVerifySession = getInteractivePaneSession(s, 'verify');
            if (s.verifyPaused && activeVerifySession) {
                try { activeVerifySession.resume(); s.verifyPaused = false; } catch (e) { log.debug('tabSwitch: verifySession.resume failed: ' + (e.message || e)); }
            }

            // Switch away from shell tab.
            if (s.splitViewTab === 'shell') {
                s.splitViewTab = activeVerifySession ? 'verify' : 'output';
            }

            return [s, null];
        }

        return [s, tea.tick(C.TICK_INTERVAL_MS, 'shell-poll')];
    }
    // Cross-chunk exports.
    prSplit._updateConfirmCancel = updateConfirmCancel;
    prSplit._runVerifyBranch = runVerifyBranch;
    prSplit._pollVerifySession = pollVerifySession;
    prSplit._pollShellSession = pollShellSession;
    prSplit._handleVerifyFallbackPoll = handleVerifyFallbackPoll;
    prSplit._openClaudeConvo = openClaudeConvo;
    prSplit._closeClaudeConvo = closeClaudeConvo;
    prSplit._updateClaudeConvo = updateClaudeConvo;
    prSplit._pollClaudeConvo = pollClaudeConvo;
    prSplit._handleErrorResolutionChoice = handleErrorResolutionChoice;

})(globalThis.prSplit);
