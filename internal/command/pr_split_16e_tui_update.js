'use strict';
// pr_split_16e_tui_update.js — TUI: main update dispatch function (wizardUpdateImpl)
// Dependencies: chunks 00-16d must be loaded first.
// Requires Go-injected globals: tui, ctx, output, log, prSplitConfig, tuiMux.

(function(prSplit) {

    if (typeof tui === 'undefined' || typeof ctx === 'undefined' ||
        typeof output === 'undefined') { return; }

    // Cross-chunk imports — libraries (from chunks 15a/15d).
    var tea = prSplit._tea;
    var syncReportOverlay = prSplit._syncReportOverlay;
    var syncReportScrollbar = prSplit._syncReportScrollbar;

    // Cross-chunk imports — state (from chunk 13).
    var st = prSplit._state;

    // Cross-chunk imports — focus + navigation handlers (from chunk 16a).
    var syncMainViewport = prSplit._syncMainViewport;
    var updateEditorDialog = prSplit._updateEditorDialog;
    var handleFocusActivate = prSplit._handleFocusActivate;
    var handleListNav = prSplit._handleListNav;
    var handleNavDown = prSplit._handleNavDown;
    var handleNavUp = prSplit._handleNavUp;
    var handleBack = prSplit._handleBack;
    var handleNext = prSplit._handleNext;
    var enterPlanEditor = prSplit._enterPlanEditor;

    // Cross-chunk imports — pipeline handlers (from chunk 16b).
    var handleAnalysisPoll = prSplit._handleAnalysisPoll;
    var handleExecutionPoll = prSplit._handleExecutionPoll;
    var handleEquivPoll = prSplit._handleEquivPoll;
    var handleResolvePoll = prSplit._handleResolvePoll;
    var handlePRCreationPoll = prSplit._handlePRCreationPoll;

    // Cross-chunk imports — verify + convo handlers (from chunk 16c).
    var updateConfirmCancel = prSplit._updateConfirmCancel;
    var runVerifyBranch = prSplit._runVerifyBranch;
    var pollVerifySession = prSplit._pollVerifySession;
    var pollShellSession = prSplit._pollShellSession;
    var handleVerifyFallbackPoll = prSplit._handleVerifyFallbackPoll;
    var updateClaudeConvo = prSplit._updateClaudeConvo;
    var pollClaudeConvo = prSplit._pollClaudeConvo;

    // Cross-chunk imports — Claude automation handlers (from chunk 16d).
    var handleClaudeCheck = prSplit._handleClaudeCheck;
    var handleClaudeCheckPoll = prSplit._handleClaudeCheckPoll;
    var handleAutoSplitPoll = prSplit._handleAutoSplitPoll;
    var handleRestartClaudePoll = prSplit._handleRestartClaudePoll;
    var keyToTermBytes = prSplit._keyToTermBytes;
    var mouseToTermBytes = prSplit._mouseToTermBytes;
    var CLAUDE_RESERVED_KEYS = prSplit._CLAUDE_RESERVED_KEYS;
    var INTERACTIVE_RESERVED_KEYS = prSplit._INTERACTIVE_RESERVED_KEYS;
    var CHROME_ESTIMATE = prSplit._CHROME_ESTIMATE;
    var pollClaudeScreenshot = prSplit._pollClaudeScreenshot;
    var C = prSplit._TUI_CONSTANTS;
    var getInteractivePaneSession = prSplit._getInteractivePaneSession;
    var listSplitViewTabs = prSplit._listSplitViewTabs;

    // Late-bound shim — handleMouseClick is defined in chunk 16f (loaded after this).
    function handleMouseClick(msg, s) { return prSplit._handleMouseClick(msg, s); }

    // T327/T328: Compute screen offset where bottom pane terminal content begins.
    // Layout: titleBar(1) + divider(1) + wizard(wizardH) + paneDivider(1) + borderTop(1) + titleLine(1).
    function computeSplitPaneContentOffset(s) {
        var h = s.height || C.DEFAULT_ROWS;
        var vpHeight = Math.max(3, h - CHROME_ESTIMATE);
        var minPaneH = 3;
        var wizardH = Math.max(minPaneH, Math.floor(vpHeight * (s.splitViewRatio || 0.6)));
        wizardH = Math.min(wizardH, vpHeight - minPaneH - 1);
        return { row: 5 + wizardH, col: 1 };
    }

    // T327/T328: Write raw bytes to the child terminal of the active tab.
    function writeMouseToPane(bytes, s) {
        var tab = s.splitViewTab;
        var session = getInteractivePaneSession(s, tab);
        if (session && typeof session.write === 'function') {
            try { session.write(bytes); return true; } catch (e) { log.debug('writeInput: ' + tab + ' session.write failed: ' + (e.message || e)); return false; }
        }
        return false;
    }

    // --- wizardUpdateImpl — main BubbleTea update dispatch ---
    //  Extracted from createWizardModel._updateFn for file splitting.
    function wizardUpdateImpl(msg, s) {
        if (typeof tuiMux !== 'undefined' && tuiMux &&
            typeof tuiMux.pollEvents === 'function') {
            try {
                tuiMux.pollEvents();
            } catch (e) {
                log.debug('wizardUpdateImpl: tuiMux.pollEvents failed: ' + (e.message || e));
            }
        }

        // WindowSize — always handle.
        if (msg.type === 'WindowSize') {
            s.width = msg.width;
            s.height = msg.height;

            // T120: Sync viewport dimensions from update, not view.
            syncMainViewport(s);

            // T336: Resize verify and shell CaptureSession terminals.
            if (s.splitViewEnabled) {
                var h = s.height || C.DEFAULT_ROWS;
                var vpH = Math.max(3, h - CHROME_ESTIMATE);
                var minP = 3;
                var wH = Math.max(minP, Math.floor(vpH * (s.splitViewRatio || 0.6)));
                wH = Math.min(wH, vpH - minP - 1);
                var cH = vpH - wH - 1;
                var paneRows = Math.max(3, cH - 3);
                var paneCols = Math.max(20, (s.width || 80) - 4);
                var interactiveTabs = ['claude', 'verify', 'shell'];
                for (var ti = 0; ti < interactiveTabs.length; ti++) {
                    var resizeTab = interactiveTabs[ti];
                    var resizeSession = getInteractivePaneSession(s, resizeTab);
                    if (resizeSession && typeof resizeSession.resize === 'function') {
                        try { resizeSession.resize(paneRows, paneCols); } catch (e) { log.debug('resize: ' + resizeTab + ' session.resize failed: ' + (e.message || e)); }
                    }
                }
            }

            // Sync report overlay dimensions if currently open.
            if (s.showingReport) {
                syncReportOverlay(s);
            }

            if (s.needsInitClear) {
                s.needsInitClear = false;
                // Start the wizard on first render.
                s.wizardState = 'CONFIG';
                s.wizard.transition('CONFIG');
                // T42: Auto-detect Claude on startup to default to 'auto' strategy.
                return [s, tea.batch(tea.clearScreen(), tea.tick(1, 'auto-detect-claude'))];
            }
            return [s, null];
        }

        // Reset focus index on wizard state transition.
        if (s.wizardState !== s._prevWizardState) {
            s.focusIndex = 0;
            s._prevWizardState = s.wizardState;
            // T46: Clear inline question state on screen transition to
            // prevent orphaned input mode on screens that don't render
            // the question prompt.
            if (s.claudeQuestionDetected || s.claudeQuestionInputActive) {
                s.claudeQuestionDetected = false;
                s.claudeQuestionLine = '';
                s.claudeQuestionInputText = '';
                s.claudeQuestionInputActive = false;
            }
        }

        // Overlays intercept all user input when active, but Tick
        // messages always pass through — they are programmatic
        // continuations (e.g. verify-poll) that must not be dropped.
        if (msg.type !== 'Tick') {

        if (s.showHelp) {
            if (msg.type === 'Key') {
                // Any key closes help.
                s.showHelp = false;
                return [s, null];
            }
            return [s, null];
        }

        if (s.showConfirmCancel) {
            return updateConfirmCancel(msg, s);
        }

        // Report overlay intercepts all input when active.
        if (s.showingReport) {
            if (msg.type === 'Key') {
                var rk = msg.key;
                // Close overlay.
                if (rk === 'esc' || rk === 'enter' || rk === 'q') {
                    s.showingReport = false;
                    return [s, null];
                }
                // T073: Copy report to clipboard with success flash and error fallback.
                if (rk === 'c') {
                    try {
                        output.toClipboard(s.reportContent || '');
                        s.clipboardFlash = 'Copied to clipboard \u2713';
                    } catch (e) {
                        s.clipboardFlash = 'Copy failed: ' + (e.message || String(e));
                    }
                    s.clipboardFlashAt = Date.now();
                    return [s, tea.tick(C.CLIPBOARD_FLASH_MS, 'dismiss-clipboard-flash')];
                }
                // Scroll navigation — sync scrollbar after each scroll op.
                if (rk === 'j' || rk === 'down') {
                    if (s.reportVp) { s.reportVp.scrollDown(1); syncReportScrollbar(s); }
                    return [s, null];
                }
                if (rk === 'k' || rk === 'up') {
                    if (s.reportVp) { s.reportVp.scrollUp(1); syncReportScrollbar(s); }
                    return [s, null];
                }
                if (rk === 'pgdown' || rk === ' ') {
                    if (s.reportVp) { s.reportVp.halfPageDown(); syncReportScrollbar(s); }
                    return [s, null];
                }
                if (rk === 'pgup') {
                    if (s.reportVp) { s.reportVp.halfPageUp(); syncReportScrollbar(s); }
                    return [s, null];
                }
                if (rk === 'home' || rk === 'g') {
                    if (s.reportVp) { s.reportVp.gotoTop(); syncReportScrollbar(s); }
                    return [s, null];
                }
                if (rk === 'end') {
                    if (s.reportVp) { s.reportVp.gotoBottom(); syncReportScrollbar(s); }
                    return [s, null];
                }
                return [s, null];
            }
            // Mouse wheel scrolling within report overlay.
            if (msg.type === 'Mouse' && msg.isWheel && s.reportVp) {
                if (msg.button === 'wheel up') {
                    s.reportVp.scrollUp(3);
                    syncReportScrollbar(s);
                    return [s, null];
                }
                if (msg.button === 'wheel down') {
                    s.reportVp.scrollDown(3);
                    syncReportScrollbar(s);
                    return [s, null];
                }
            }
            // Clicking outside overlay closes it.
            if (msg.type === 'Mouse' && msg.action === 'press' && !msg.isWheel) {
                s.showingReport = false;
                return [s, null];
            }
            return [s, null];
        }

        // Editor dialog intercepts all input when active.
        if (s.activeEditorDialog) {
            return updateEditorDialog(msg, s);
        }

        // Claude conversation overlay intercepts all input when active.
        if (s.claudeConvo.active) {
            return updateClaudeConvo(msg, s);
        }

        // T46: Inline Claude question prompt input interceptor.
        // When the user is actively typing a response to Claude's question,
        // intercept all keyboard input (like a mini text editor).
        // Exception: Ctrl+L passes through so user can toggle split-view.
        if (s.claudeQuestionInputActive && msg.type === 'Key' && msg.key !== 'ctrl+l') {
            var qk = msg.key;

            // Escape — dismiss the question prompt entirely.
            if (qk === 'esc') {
                s.claudeQuestionDetected = false;
                s.claudeQuestionLine = '';
                s.claudeQuestionInputText = '';
                s.claudeQuestionInputActive = false;
                return [s, null];
            }

            // Enter — send response to Claude's PTY.
            if (qk === 'enter') {
                var responseText = (s.claudeQuestionInputText || '').trim();
                if (responseText.length > 0) {
                    // Record in conversation history.
                    s.claudeConversations.push({
                        question: s.claudeQuestionLine,
                        answer: responseText,
                        ts: Date.now()
                    });
                    if (s.claudeConversations.length > C.CONVO_HISTORY_CAP) {
                        s.claudeConversations = s.claudeConversations.slice(-C.CONVO_HISTORY_TRIM);
                    }

                    // Send to Claude PTY via tuiMux.writeToChild.
                    if (typeof tuiMux !== 'undefined' && tuiMux &&
                        typeof tuiMux.writeToChild === 'function') {
                        try {
                            tuiMux.writeToChild(responseText + '\r');
                            log.printf('T46: sent response to Claude: %s', responseText);
                        } catch (e) {
                            // T393: Surface error to user — keep claudeQuestionDetected
                            // true so renderClaudeQuestionPrompt renders the error line.
                            log.printf('T46: writeToChild failed: %s', String(e));
                            s.claudeQuestionLine = 'Error sending response: ' + String(e);
                            s.claudeQuestionInputActive = false;
                            s.claudeQuestionInputText = '';
                            return [s, null];
                        }
                    } else {
                        // T393: tuiMux not available — keep claudeQuestionDetected
                        // true so the error is visible to the user.
                        log.printf('T46: tuiMux.writeToChild not available');
                        s.claudeQuestionLine = 'Error: Claude terminal not connected';
                        s.claudeQuestionInputActive = false;
                        s.claudeQuestionInputText = '';
                        return [s, null];
                    }

                    // Clear question state.
                    s.claudeQuestionDetected = false;
                    s.claudeQuestionLine = '';
                    s.claudeQuestionInputText = '';
                    s.claudeQuestionInputActive = false;
                    // Reset throttle so we don't immediately re-detect
                    // the same question before Claude starts streaming.
                    s.claudeLastQuestionCheckMs = Date.now();
                }
                return [s, null];
            }

            // Backspace.
            if (qk === 'backspace') {
                var qt = s.claudeQuestionInputText || '';
                if (qt.length > 0) {
                    s.claudeQuestionInputText = qt.substring(0, qt.length - 1);
                }
                return [s, null];
            }

            // Ctrl+U: clear input line.
            if (qk === 'ctrl+u') {
                s.claudeQuestionInputText = '';
                return [s, null];
            }

            // Single printable character — accumulate.
            if (qk.length === 1) {
                s.claudeQuestionInputText = (s.claudeQuestionInputText || '') + qk;
                return [s, null];
            }

            // Consume all other keys (don't let them leak to navigation).
            return [s, null];
        }

        // T46: When question detected but input not yet active, any
        // printable character activates the input field.
        if (s.claudeQuestionDetected && !s.claudeQuestionInputActive && msg.type === 'Key') {
            var qak = msg.key;
            if (qak.length === 1) {
                s.claudeQuestionInputActive = true;
                s.claudeQuestionInputText = qak;
                return [s, null];
            }
            // Escape still dismisses even when not actively typing.
            if (qak === 'esc') {
                s.claudeQuestionDetected = false;
                s.claudeQuestionLine = '';
                s.claudeQuestionInputText = '';
                s.claudeQuestionInputActive = false;
                return [s, null];
            }
            // Other keys pass through to normal handling.
        }

        // Inline title editing intercepts all key input when active (T17).
        if (s.editorTitleEditing && msg.type === 'Key') {
            var ek = msg.key;
            if (ek === 'enter') {
                // Save title to the split that was being edited (not current selection).
                var eidx = s.editorTitleEditingIdx >= 0 ? s.editorTitleEditingIdx : (s.selectedSplitIdx || 0);
                if (st.planCache && st.planCache.splits && st.planCache.splits[eidx]) {
                    var newName = (s.editorTitleText || '').trim();
                    if (newName) {
                        st.planCache.splits[eidx].name = newName;
                    }
                }
                s.editorTitleEditing = false;
                s.editorTitleEditingIdx = -1;
                s.editorTitleText = '';
                return [s, null];
            }
            if (ek === 'esc') {
                // Cancel editing without saving.
                s.editorTitleEditing = false;
                s.editorTitleEditingIdx = -1;
                s.editorTitleText = '';
                return [s, null];
            }
            if (ek === 'backspace') {
                var etxt = s.editorTitleText || '';
                s.editorTitleText = etxt.slice(0, -1);
                return [s, null];
            }
            if (ek === 'ctrl+u') {
                s.editorTitleText = '';
                return [s, null];
            }
            // Single character input.
            if (ek.length === 1) {
                s.editorTitleText = (s.editorTitleText || '') + ek;
                return [s, null];
            }
            // Swallow all other keys during edit.
            return [s, null];
        }

        // Config field inline editing interceptor.
        // When a config field is being edited, capture keystrokes as text input.
        if (s.configFieldEditing && msg.type === 'Key') {
            var cfk = msg.key;
            if (cfk === 'enter') {
                // Commit edited value back to runtime.
                var field = s.configFieldEditing;
                var val = (s.configFieldValue || '').trim();
                if (field === 'maxFiles') {
                    var n = parseInt(val, 10);
                    if (!isNaN(n) && n > 0) prSplit.runtime.maxFiles = n;
                } else if (field === 'branchPrefix') {
                    if (val) prSplit.runtime.branchPrefix = val;
                } else if (field === 'verifyCommand') {
                    prSplit.runtime.verifyCommand = val || 'true';
                }
                s.configFieldEditing = null;
                s.configFieldValue = '';
                return [s, null];
            }
            if (cfk === 'esc') {
                // Cancel editing without saving.
                s.configFieldEditing = null;
                s.configFieldValue = '';
                return [s, null];
            }
            if (cfk === 'backspace') {
                var cfv = s.configFieldValue || '';
                s.configFieldValue = cfv.slice(0, -1);
                return [s, null];
            }
            if (cfk === 'ctrl+u') {
                s.configFieldValue = '';
                return [s, null];
            }
            // For maxFiles, only accept digits.
            if (s.configFieldEditing === 'maxFiles') {
                if (cfk.length === 1 && cfk >= '0' && cfk <= '9') {
                    s.configFieldValue = (s.configFieldValue || '') + cfk;
                }
                return [s, null];
            }
            // Single character input for text fields.
            if (cfk.length === 1) {
                s.configFieldValue = (s.configFieldValue || '') + cfk;
                return [s, null];
            }
            // Swallow all other keys during config field edit.
            return [s, null];
        }

        } // end: if (msg.type !== 'Tick') — overlay guard

        // Global key bindings.
        if (msg.type === 'Key') {
            var k = msg.key;
            var activeVerifySession = getInteractivePaneSession(s, 'verify');

            // Live verify session: intercept Ctrl+C to stop verification
            // instead of showing the cancel dialog.
            // First Ctrl+C sends SIGINT; second within 2s sends SIGKILL
            // (handles processes that ignore SIGINT).
            if (k === 'ctrl+c' && activeVerifySession) {
                var now = Date.now();
                if (s.lastVerifyInterruptTime > 0 && (now - s.lastVerifyInterruptTime) < C.SIGKILL_WINDOW_MS) {
                    // Double Ctrl+C — force kill.
                    try { activeVerifySession.kill(); } catch (e) { log.debug('cancelVerify: verifySession.kill failed: ' + (e.message || e)); }
                } else {
                    // First Ctrl+C — graceful interrupt.
                    try { activeVerifySession.interrupt(); } catch (e) { log.debug('cancelVerify: verifySession.interrupt failed: ' + (e.message || e)); }
                }
                s.lastVerifyInterruptTime = now;
                return [s, null];
            }

            // Live verify session: ↑/↓ scroll the output viewport.
            if (activeVerifySession) {
                if (k === 'up' || k === 'k') {
                    s.verifyAutoScroll = false;
                    s.verifyViewportOffset = (s.verifyViewportOffset || 0) + 1;
                    return [s, null];
                }
                if (k === 'down' || k === 'j') {
                    s.verifyViewportOffset = Math.max(0, (s.verifyViewportOffset || 0) - 1);
                    if (s.verifyViewportOffset === 0) {
                        s.verifyAutoScroll = true;
                    }
                    return [s, null];
                }
                if (k === 'home') {
                    s.verifyAutoScroll = false;
                    s.verifyViewportOffset = C.FAR_SCROLL_SENTINEL; // far back
                    return [s, null];
                }
                if (k === 'end') {
                    s.verifyViewportOffset = 0;
                    s.verifyAutoScroll = true;
                    return [s, null];
                }
            }

            // Split-view: Ctrl+L to toggle Claude window-in-window.
            if (k === 'ctrl+l') {
                s.splitViewEnabled = !s.splitViewEnabled;
                syncMainViewport(s); // T120: sync dimensions after toggle.
                if (s.splitViewEnabled) {
                    // T45: User re-opened — clear manual dismiss flag.
                    s.claudeManuallyDismissed = false;
                    // Start screenshot polling. Preserve focusIndex.
                    return [s, tea.tick(C.TICK_INTERVAL_MS, 'claude-screenshot')];
                } else {
                    // T45: User explicitly closed — set manual dismiss flag
                    // so auto-attach does not re-open the pane.
                    s.claudeManuallyDismissed = true;
                    // Reset split-view state on disable. Preserve focusIndex.
                    s.claudeScreenshot = '';
                    s.claudeScreen = '';
                    s.claudeViewOffset = 0;
                    s.splitViewFocus = 'wizard';
                    s.splitViewTab = 'claude'; // T44: reset tab on disable
                    // T46: Clear inline question state — the question prompt
                    // wouldn't auto-dismiss with split-view disabled (no
                    // pollClaudeScreenshot running to clear it).
                    s.claudeQuestionDetected = false;
                    s.claudeQuestionLine = '';
                    s.claudeQuestionInputText = '';
                    s.claudeQuestionInputActive = false;
                }
                return [s, null];
            }

            // Split-view keybindings (only when enabled).
            if (s.splitViewEnabled) {
                // T380: Ctrl+Tab switches focus between wizard and pane (works during verify).
                if (k === 'ctrl+tab') {
                    s.splitViewFocus = (s.splitViewFocus === 'wizard') ? 'claude' : 'wizard';
                    return [s, null];
                }
                // Ctrl+= / Ctrl+- to adjust ratio.
                if (k === 'ctrl++' || k === 'ctrl+=') {
                    s.splitViewRatio = Math.min(0.8, s.splitViewRatio + 0.1);
                    syncMainViewport(s); // T120: sync dimensions after ratio change.
                    return [s, null];
                }
                if (k === 'ctrl+-') {
                    s.splitViewRatio = Math.max(0.2, s.splitViewRatio - 0.1);
                    syncMainViewport(s); // T120: sync dimensions after ratio change.
                    return [s, null];
                }
                // T44+T322+T380+T388: Ctrl+O cycles through available tabs in split-view bottom pane.
                if (k === 'ctrl+o') {
                    var tabs = listSplitViewTabs(s);
                    var idx = tabs.indexOf(s.splitViewTab);
                    s.splitViewTab = tabs[(idx + 1) % tabs.length];
                    return [s, null];
                }
                // T29: Claude pane keyboard input forwarding.
                // When Claude pane is focused, forward non-reserved keys to child PTY.
                if (s.splitViewFocus === 'claude') {
                    // T44: Output tab scroll keys (when output tab is active).
                    if (s.splitViewTab === 'output') {
                        if (k === 'up' || k === 'k') {
                            s.outputViewOffset = (s.outputViewOffset || 0) + 1;
                            s.outputAutoScroll = false;
                            return [s, null];
                        }
                        if (k === 'down' || k === 'j') {
                            s.outputViewOffset = Math.max(0, (s.outputViewOffset || 0) - 1);
                            if (s.outputViewOffset === 0) s.outputAutoScroll = true;
                            return [s, null];
                        }
                        if (k === 'pgup') {
                            s.outputViewOffset = (s.outputViewOffset || 0) + 5;
                            s.outputAutoScroll = false;
                            return [s, null];
                        }
                        if (k === 'pgdown') {
                            s.outputViewOffset = Math.max(0, (s.outputViewOffset || 0) - 5);
                            if (s.outputViewOffset === 0) s.outputAutoScroll = true;
                            return [s, null];
                        }
                        if (k === 'home') {
                            s.outputViewOffset = C.FAR_SCROLL_SENTINEL;
                            s.outputAutoScroll = false;
                            return [s, null];
                        }
                        if (k === 'end') {
                            s.outputViewOffset = 0;
                            s.outputAutoScroll = true;
                            return [s, null];
                        }
                        // Output tab is read-only — don't forward to PTY, don't scroll Claude.
                        return [s, null];
                    }
                    // T324: Verify tab — forward input to verify CaptureSession.
                    if (s.splitViewTab === 'verify') {
                        // Scroll keys adjust the verify viewport.
                        if (k === 'up' || k === 'k') {
                            s.verifyViewportOffset = (s.verifyViewportOffset || 0) + 1;
                            s.verifyAutoScroll = false;
                            return [s, null];
                        }
                        if (k === 'down' || k === 'j') {
                            s.verifyViewportOffset = Math.max(0, (s.verifyViewportOffset || 0) - 1);
                            if (s.verifyViewportOffset === 0) s.verifyAutoScroll = true;
                            return [s, null];
                        }
                        if (k === 'pgup') {
                            s.verifyViewportOffset = (s.verifyViewportOffset || 0) + 5;
                            s.verifyAutoScroll = false;
                            return [s, null];
                        }
                        if (k === 'pgdown') {
                            s.verifyViewportOffset = Math.max(0, (s.verifyViewportOffset || 0) - 5);
                            if (s.verifyViewportOffset === 0) s.verifyAutoScroll = true;
                            return [s, null];
                        }
                        if (k === 'home') {
                            s.verifyViewportOffset = C.FAR_SCROLL_SENTINEL;
                            s.verifyAutoScroll = false;
                            return [s, null];
                        }
                        if (k === 'end') {
                            s.verifyViewportOffset = 0;
                            s.verifyAutoScroll = true;
                            return [s, null];
                        }
                        // Forward non-reserved keys to the verify CaptureSession.
                        if (!CLAUDE_RESERVED_KEYS[k]) {
                            var vBytes = keyToTermBytes(k);
                            var verifySession = getInteractivePaneSession(s, 'verify');
                            if (vBytes !== null && verifySession &&
                                typeof verifySession.write === 'function') {
                                try {
                                    verifySession.write(vBytes);
                                    s.verifyViewportOffset = 0;
                                    s.verifyAutoScroll = true;
                                } catch (e) {
                                    // Swallow — write may fail if session ended.
                                }
                            }
                            return [s, null];
                        }
                    }
                    // T334: Shell tab — forward ALL non-reserved keys to shell CaptureSession.
                    // T386: Shell is fully interactive — use minimal reserved set so
                    // arrow keys, j/k, pgup/pgdown, home/end reach the child process.
                    if (s.splitViewTab === 'shell') {
                        if (!INTERACTIVE_RESERVED_KEYS[k]) {
                            var shBytes = keyToTermBytes(k);
                            var shellSession = getInteractivePaneSession(s, 'shell');
                            if (shBytes !== null && shellSession &&
                                typeof shellSession.write === 'function') {
                                try {
                                    shellSession.write(shBytes);
                                    s.shellViewOffset = 0;
                                    s.shellAutoScroll = true;
                                } catch (e) {
                                    // Swallow — write may fail if session ended.
                                }
                            }
                            return [s, null];
                        }
                    }
                    // Viewport scroll keys — scroll the Claude pane output.
                    if (k === 'up' || k === 'k') {
                        s.claudeViewOffset = (s.claudeViewOffset || 0) + 1;
                        return [s, null];
                    }
                    if (k === 'down' || k === 'j') {
                        s.claudeViewOffset = Math.max(0, (s.claudeViewOffset || 0) - 1);
                        return [s, null];
                    }
                    if (k === 'pgup') {
                        s.claudeViewOffset = (s.claudeViewOffset || 0) + 5;
                        return [s, null];
                    }
                    if (k === 'pgdown') {
                        s.claudeViewOffset = Math.max(0, (s.claudeViewOffset || 0) - 5);
                        return [s, null];
                    }
                    if (k === 'home') {
                        s.claudeViewOffset = C.FAR_SCROLL_SENTINEL;
                        return [s, null];
                    }
                    if (k === 'end') {
                        s.claudeViewOffset = 0;
                        return [s, null];
                    }
                    // Forward non-reserved keys to Claude's PTY.
                    if (!CLAUDE_RESERVED_KEYS[k]) {
                        var bytes = keyToTermBytes(k);
                        var claudeSession = getInteractivePaneSession(s, 'claude');
                        if (bytes !== null && claudeSession &&
                            typeof claudeSession.write === 'function') {
                            try {
                                claudeSession.write(bytes);
                                // Auto-scroll to bottom on input (follow live output).
                                s.claudeViewOffset = 0;
                            } catch (e) {
                                // Swallow — write may fail if child ended.
                            }
                        }
                        return [s, null];
                    }
                }
            }

            // Help toggle.
            if (k === '?' || k === 'f1') {
                s.showHelp = true;
                return [s, null];
            }
            // Cancel.
            if (k === 'ctrl+c') {
                s.showConfirmCancel = true;
                s.confirmCancelFocus = 0;  // T031: reset focus to 'Yes' on open
                return [s, null];
            }
            // Escape — back or close.
            if (k === 'esc') {
                return handleBack(s);
            }
            // Enter — activate focused element or forward action.
            if (k === 'enter') {
                var activated = handleFocusActivate(s);
                if (activated) return activated;
                return handleNext(s);
            }
            // Navigation: j/k/up/down = list navigation (splits only).
            if (k === 'j' || k === 'down') {
                return handleListNav(s, 1);
            }
            if (k === 'k' || k === 'up') {
                return handleListNav(s, -1);
            }
            // Tab/Shift+Tab = full focus cycling across all elements.
            if (k === 'tab') {
                return handleNavDown(s);
            }
            if (k === 'shift+tab') {
                return handleNavUp(s);
            }
            // Viewport scroll.
            if (k === 'pgdown') {
                if (s.vp) s.vp.halfPageDown();
                return [s, null];
            }
            if (k === 'pgup') {
                if (s.vp) s.vp.halfPageUp();
                return [s, null];
            }
            if (k === 'home') {
                if (s.vp) s.vp.gotoTop();
                return [s, null];
            }
            if (k === 'end') {
                if (s.vp) s.vp.gotoBottom();
                return [s, null];
            }
            // Screen-specific key shortcuts.
            // BRANCH_BUILDING: 'e' toggles expand/collapse on the
            // most recently verified branch with output.
            if (k === 'e' && s.wizardState === 'BRANCH_BUILDING') {
                if (s.expandedVerifyBranch !== null && s.expandedVerifyBranch !== undefined) {
                    // Collapse the currently expanded branch.
                    s.expandedVerifyBranch = null;
                } else if (st.planCache && st.planCache.splits && s.verifyOutput) {
                    // Find the last branch that has verification output to expand.
                    for (var ei = st.planCache.splits.length - 1; ei >= 0; ei--) {
                        var eBranch = st.planCache.splits[ei].name;
                        if (s.verifyOutput[eBranch] && s.verifyOutput[eBranch].length > 0) {
                            s.expandedVerifyBranch = eBranch;
                            break;
                        }
                    }
                }
                return [s, null];
            }
            // T059: BRANCH_BUILDING: 'z' toggles pause/resume on
            // the active verify session. Only when a verify is running.
            if (k === 'z' && s.wizardState === 'BRANCH_BUILDING' && activeVerifySession) {
                if (s.verifyPaused) {
                    try { activeVerifySession.resume(); s.verifyPaused = false; prSplit._transitionVerifyPhase(s, prSplit._verifyPhases.RUNNING); } catch (e) {
                        log.printf('verify: resume failed: %s', e.message || String(e));
                    }
                } else {
                    try { activeVerifySession.pause(); s.verifyPaused = true; prSplit._transitionVerifyPhase(s, prSplit._verifyPhases.PAUSED); } catch (e) {
                        log.printf('verify: pause failed: %s', e.message || String(e));
                    }
                }
                return [s, null];
            }
            // T006: BRANCH_BUILDING: 'p' marks the most recently failed
            // verification as manually passed ("Mark as Passed").
            // Only active when verification is NOT currently running and
            // there is at least one failed (non-passed, non-skipped) result.
            if (k === 'p' && s.wizardState === 'BRANCH_BUILDING' && !activeVerifySession) {
                var vResults = s.verificationResults || [];
                // Find the last failed result.
                var failIdx = -1;
                for (var fi = vResults.length - 1; fi >= 0; fi--) {
                    if (vResults[fi] && !vResults[fi].passed && !vResults[fi].skipped) {
                        failIdx = fi;
                        break;
                    }
                }
                if (failIdx >= 0) {
                    vResults[failIdx].passed = true;
                    vResults[failIdx].manualOverride = true;
                    vResults[failIdx].error = null;
                    log.printf('verify: manually marked %s as passed', vResults[failIdx].name || '(unknown)');
                }
                return [s, null];
            }
            if (k === 'e' && s.wizardState === 'PLAN_REVIEW' && !s.isProcessing) {
                // Enter plan editor.
                return enterPlanEditor(s);
            }
            // PLAN_EDITOR: inline title rename (T17).
            if (k === 'e' && s.wizardState === 'PLAN_EDITOR') {
                var eidx = s.selectedSplitIdx || 0;
                if (st.planCache && st.planCache.splits && st.planCache.splits[eidx]) {
                    s.editorTitleEditing = true;
                    s.editorTitleEditingIdx = eidx;
                    s.editorTitleText = st.planCache.splits[eidx].name || '';
                }
                return [s, null];
            }
            // PLAN_EDITOR: Space toggles checked state on highlighted file (T17).
            if (k === ' ' && s.wizardState === 'PLAN_EDITOR') {
                var sidx = s.selectedSplitIdx || 0;
                var fidx = s.selectedFileIdx || 0;
                if (st.planCache && st.planCache.splits && st.planCache.splits[sidx] &&
                    st.planCache.splits[sidx].files && st.planCache.splits[sidx].files[fidx]) {
                    if (!s.editorCheckedFiles) s.editorCheckedFiles = {};
                    var fkey = sidx + '-' + fidx;
                    s.editorCheckedFiles[fkey] = !s.editorCheckedFiles[fkey];
                }
                return [s, null];
            }
            // PLAN_EDITOR: Shift+up/down reorder files within split (T17).
            if (k === 'shift+up' && s.wizardState === 'PLAN_EDITOR') {
                var sidx = s.selectedSplitIdx || 0;
                var fidx = s.selectedFileIdx || 0;
                if (st.planCache && st.planCache.splits && st.planCache.splits[sidx] &&
                    st.planCache.splits[sidx].files && fidx > 0) {
                    var reFiles = st.planCache.splits[sidx].files;
                    var tmp = reFiles[fidx - 1];
                    reFiles[fidx - 1] = reFiles[fidx];
                    reFiles[fidx] = tmp;
                    // Also swap checked state to follow the file.
                    if (!s.editorCheckedFiles) s.editorCheckedFiles = {};
                    var ckFrom = sidx + '-' + fidx;
                    var ckTo = sidx + '-' + (fidx - 1);
                    var tmpCk = s.editorCheckedFiles[ckFrom];
                    s.editorCheckedFiles[ckFrom] = s.editorCheckedFiles[ckTo];
                    s.editorCheckedFiles[ckTo] = tmpCk;
                    s.selectedFileIdx = fidx - 1;
                }
                return [s, null];
            }
            if (k === 'shift+down' && s.wizardState === 'PLAN_EDITOR') {
                var sidx = s.selectedSplitIdx || 0;
                var fidx = s.selectedFileIdx || 0;
                if (st.planCache && st.planCache.splits && st.planCache.splits[sidx] &&
                    st.planCache.splits[sidx].files && fidx < st.planCache.splits[sidx].files.length - 1) {
                    var reFiles = st.planCache.splits[sidx].files;
                    var tmp = reFiles[fidx + 1];
                    reFiles[fidx + 1] = reFiles[fidx];
                    reFiles[fidx] = tmp;
                    // Also swap checked state to follow the file.
                    if (!s.editorCheckedFiles) s.editorCheckedFiles = {};
                    var ckFrom = sidx + '-' + fidx;
                    var ckTo = sidx + '-' + (fidx + 1);
                    var tmpCk = s.editorCheckedFiles[ckFrom];
                    s.editorCheckedFiles[ckFrom] = s.editorCheckedFiles[ckTo];
                    s.editorCheckedFiles[ckTo] = tmpCk;
                    s.selectedFileIdx = fidx + 1;
                }
                return [s, null];
            }
            // T394: Ctrl+] passthrough is now handled by toggleModel wrapper
            // in BubbleTea (see startWizard). The wrapper properly calls
            // ReleaseTerminal before RunPassthrough, preventing stdin
            // contention. ToggleReturn message handled below.
        }

        // T394: Handle ToggleReturn from toggleModel after passthrough exits
        // (or skipped because no child was attached).
        if (msg.type === 'ToggleReturn') {
            if (msg.skipped) {
                // No Claude child — show notification.
                s.claudeAutoAttachNotif = 'Claude not available \u2014 no active Claude session';
                s.claudeAutoAttachNotifAt = Date.now();
            }
            return [s, null];
        }

        // Mouse handling.
        // Wheel events must be checked BEFORE press — wheel events
        // have action:"press" AND isWheel:true, so the press guard
        // would intercept them and send them to handleMouseClick.

        // T327/T328: Forward mouse events to focused child terminal.
        // Intercepts motion, release, and wheel before wizard-managed handlers.
        // Press events are NOT intercepted here — they go through
        // handleMouseClick for zone detection first.
        if (msg.type === 'Mouse' && s.splitViewEnabled &&
            s.splitViewFocus === 'claude' && s.splitViewTab !== 'output') {
            var fwdAction = msg.action;
            if (fwdAction === 'motion' || (fwdAction === 'release' && !msg.isWheel)) {
                var ofs = computeSplitPaneContentOffset(s);
                var mb = mouseToTermBytes(msg, ofs.row, ofs.col);
                if (mb && writeMouseToPane(mb, s)) {
                    return [s, null];
                }
            }
            if (msg.isWheel) {
                var ofs = computeSplitPaneContentOffset(s);
                var mb = mouseToTermBytes(msg, ofs.row, ofs.col);
                if (mb && writeMouseToPane(mb, s)) {
                    return [s, null];
                }
                // Fall through to wizard-managed scrolling if child unavailable.
            }
        }

        // Live verify session mouse wheel → scroll output viewport.
        // Only applies when verify output is visible (non-split or verify tab active).
        if (msg.type === 'Mouse' && msg.isWheel && getInteractivePaneSession(s, 'verify') &&
            (!s.splitViewEnabled || s.splitViewTab === 'verify')) {
            if (msg.button === 'wheel up') {
                s.verifyAutoScroll = false;
                s.verifyViewportOffset = (s.verifyViewportOffset || 0) + 3;
                return [s, null];
            }
            if (msg.button === 'wheel down') {
                s.verifyViewportOffset = Math.max(0, (s.verifyViewportOffset || 0) - 3);
                if (s.verifyViewportOffset === 0) {
                    s.verifyAutoScroll = true;
                }
                return [s, null];
            }
        }

        // Split-view Claude pane mouse wheel → scroll Claude screenshot.
        // T44: Also handle Output tab scrolling.
        if (msg.type === 'Mouse' && msg.isWheel && s.splitViewEnabled &&
            s.splitViewFocus === 'claude') {
            if (s.splitViewTab === 'output') {
                // Output tab scrolling.
                if (msg.button === 'wheel up') {
                    s.outputViewOffset = (s.outputViewOffset || 0) + 3;
                    s.outputAutoScroll = false;
                    return [s, null];
                }
                if (msg.button === 'wheel down') {
                    s.outputViewOffset = Math.max(0, (s.outputViewOffset || 0) - 3);
                    if (s.outputViewOffset === 0) s.outputAutoScroll = true;
                    return [s, null];
                }
            } else {
                // Claude tab scrolling.
                if (msg.button === 'wheel up') {
                    s.claudeViewOffset = (s.claudeViewOffset || 0) + 3;
                    return [s, null];
                }
                if (msg.button === 'wheel down') {
                    s.claudeViewOffset = Math.max(0, (s.claudeViewOffset || 0) - 3);
                    return [s, null];
                }
            }
        }

        if (msg.type === 'Mouse' && msg.isWheel && s.vp) {
            if (msg.button === 'wheel up') {
                s.vp.scrollUp(3);
                return [s, null];
            }
            if (msg.button === 'wheel down') {
                s.vp.scrollDown(3);
                return [s, null];
            }
        }

        if (msg.type === 'Mouse' && msg.action === 'press' && !msg.isWheel) {
            return handleMouseClick(msg, s);
        }

        // Tick-based polling and execution steps.
        if (msg.type === 'Tick') {
            if (msg.id === 'mux-poll') {
                return [s, tea.tick(C.TICK_INTERVAL_MS, 'mux-poll')];
            }
            // Heuristic analysis polling (async Promise+poll pattern).
            if (msg.id === 'analysis-poll') {
                return handleAnalysisPoll(s);
            }
            // Execution polling (async Promise+poll pattern).
            if (msg.id === 'execution-poll') {
                return handleExecutionPoll(s);
            }
            // Equivalence check polling (async Promise+poll pattern).
            if (msg.id === 'equiv-poll') {
                return handleEquivPoll(s);
            }
            if (msg.id === 'verify-branch') {
                return runVerifyBranch(s);
            }
            if (msg.id === 'verify-poll') {
                return pollVerifySession(s);
            }
            if (msg.id === 'shell-poll') {
                return pollShellSession(s);
            }
            if (msg.id === 'verify-fallback-poll') {
                return handleVerifyFallbackPoll(s);
            }
            // Automated pipeline polling.
            if (msg.id === 'auto-poll') {
                return handleAutoSplitPoll(s);
            }
            // Resolve-conflicts polling.
            if (msg.id === 'resolve-poll') {
                return handleResolvePoll(s);
            }
            // Claude restart polling (non-blocking restart flow).
            if (msg.id === 'restart-claude-poll') {
                return handleRestartClaudePoll(s);
            }
            // Claude availability check (CONFIG screen).
            if (msg.id === 'check-claude') {
                return handleClaudeCheck(s);
            }
            // T42: Auto-detect Claude on startup to default to 'auto' strategy.
            if (msg.id === 'auto-detect-claude') {
                // Skip if user already manually selected a strategy.
                if (s.userHasSelectedStrategy) return [s, null];
                // Skip if already checking or detected.
                if (s.claudeCheckStatus) return [s, null];
                return handleClaudeCheck(s);
            }
            if (msg.id === 'claude-check-poll') {
                return handleClaudeCheckPoll(s);
            }
            // Split-view: poll Claude screenshot.
            if (msg.id === 'claude-screenshot') {
                return pollClaudeScreenshot(s);
            }
            // Claude conversation: poll for async send/wait completion.
            if (msg.id === 'claude-convo-poll') {
                return pollClaudeConvo(s);
            }
            // T095: PR creation polling.
            if (msg.id === 'pr-creation-poll') {
                return handlePRCreationPoll(s);
            }
            // T028: Auto-dismiss transient notification after 5s.
            // Guard: only dismiss if the current notification is old enough
            // to prevent a stale tick from clearing a newer notification.
            if (msg.id === 'dismiss-attach-notif') {
                if (s.claudeAutoAttachNotifAt && (Date.now() - s.claudeAutoAttachNotifAt) >= C.AUTO_ATTACH_NOTIF_GUARD_MS) {
                    s.claudeAutoAttachNotif = '';
                    s.claudeAutoAttachNotifAt = 0;
                }
                return [s, null];
            }
            // T073: Auto-dismiss clipboard flash after 3s.
            if (msg.id === 'dismiss-clipboard-flash') {
                if (s.clipboardFlashAt && (Date.now() - s.clipboardFlashAt) >= C.CLIPBOARD_FLASH_GUARD_MS) {
                    s.clipboardFlash = '';
                    s.clipboardFlashAt = 0;
                }
                return [s, null];
            }
            return [s, null];
        }

        return [s, null];
    }

    // Cross-chunk export.
    prSplit._wizardUpdateImpl = wizardUpdateImpl;
    prSplit._computeSplitPaneContentOffset = computeSplitPaneContentOffset;
    prSplit._writeMouseToPane = writeMouseToPane;

})(globalThis.prSplit);
