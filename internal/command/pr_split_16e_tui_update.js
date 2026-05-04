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
    var handleVerifySignal = prSplit._handleVerifySignal;
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
    var termmux = require('osm:termmux');

    // Late-bound shim — handleMouseClick is defined in chunk 16f (loaded after this).
    function handleMouseClick(msg, s) { return prSplit._handleMouseClick(msg, s); }

    // T327/T328: Split layout for pane geometry (delegated to Go termmux module).
    var _splitLayout = termmux.splitLayout({
        totalChromeRows: CHROME_ESTIMATE,
        topPaneHeaderRows: 2,
        dividerRows: 1,
        bottomPaneHeaderRows: 2,
        leftChromeCol: 1,
        minPaneRows: 3
    });

    // T327/T328: Compute screen offset where bottom pane terminal content begins.
    function computeSplitPaneContentOffset(s) {
        var h = s.height || C.DEFAULT_ROWS;
        var geo = _splitLayout.compute(h, s.width || 80, s.splitViewRatio || 0.6);
        return { row: geo.bottom.row, col: geo.bottom.col };
    }

    // T327/T328: Write raw bytes to the child terminal of the active tab.
    function writeMouseToPane(bytes, s) {
        var tab = s.splitViewTab;
        if (tab === 'verify' &&
            (s.verifyFallbackRunning || s.verifyMode === 'oneshot' ||
                s.verifyMode === 'textonly' || s.verifyShellExited)) {
            return false;
        }
        var session = getInteractivePaneSession(s, tab);
        if (session && typeof session.write === 'function') {
            try { session.write(bytes); return true; } catch (e) { log.debug('writeInput: ' + tab + ' session.write failed: ' + (e.message || e)); return false; }
        }
        return false;
    }

    // --- T62: Selection and Copy/Paste Helpers ---

    // getPaneContentLines returns the plain-text lines for the given pane tab.
    // Used for text extraction during copy and for computing selection bounds.
    function getPaneContentLines(s) {
        var tab = s.splitViewTab;
        if (tab === 'output') {
            return s.outputLines || [];
        }
        if (tab === 'verify') {
            var vc = s.verifyScreen || '';
            return vc ? vc.split('\n') : [];
        }
        // Claude tab: prefer ANSI, fall back to plain screenshot.
        // For text extraction we always use plain text.
        var cc = s.claudeScreenshot || '';
        return cc ? cc.split('\n') : [];
    }

    // getPaneVisibleRange returns {startLine, viewH} describing which content
    // lines are currently visible in the pane viewport. Accounts for scroll
    // offset per-tab.
    function getPaneVisibleRange(s) {
        var tab = s.splitViewTab;
        var lines = getPaneContentLines(s);
        var totalLines = lines.length;

        // Compute viewH from pane dimensions.
        var h = s.height || C.DEFAULT_ROWS;
        var vpH = Math.max(3, h - CHROME_ESTIMATE);
        var minP = 3;
        var wH = Math.max(minP, Math.floor(vpH * (s.splitViewRatio || 0.6)));
        wH = Math.min(wH, vpH - minP - 1);
        var cH = vpH - wH - 1;
        var contentH = Math.max(1, cH - 2); // subtract border top/bottom
        var viewH = Math.max(1, contentH - 1); // subtract title line

        var offset = 0;
        if (tab === 'output')      offset = s.outputViewOffset || 0;
        else if (tab === 'verify') offset = s.verifyViewportOffset || 0;
        else                       offset = s.claudeViewOffset || 0;

        var startLine;
        if (offset <= 0) {
            startLine = Math.max(0, totalLines - viewH);
        } else {
            startLine = Math.max(0, totalLines - viewH - offset);
        }
        return { startLine: startLine, viewH: viewH };
    }

    // getCursorInPane returns {row, col} of the cursor within the active pane's
    // content space. For Claude and verify tabs, reads from the snapshot's cursor
    // position. For the output tab, defaults to the end of the last line.
    function getCursorInPane(s) {
        var tab = s.splitViewTab;
        if (tab === 'output') {
            var ol = s.outputLines || [];
            var lastRow = Math.max(0, ol.length - 1);
            var lastCol = ol.length > 0 ? (ol[lastRow] || '').length : 0;
            return { row: lastRow, col: lastCol };
        }
        // Claude / verify: read cursor from snapshot.
        if (typeof tuiMux !== 'undefined' && tuiMux &&
            typeof tuiMux.snapshot === 'function') {
            var sid = 0;
            if (tab === 'verify' && s.activeVerifySession) {
                sid = (typeof s.activeVerifySession === 'number')
                    ? s.activeVerifySession
                    : s.activeVerifySession.id;
            } else {
                sid = prSplit._state && prSplit._state.claudeSessionID;
            }
            if (sid) {
                var snap = tuiMux.snapshot(sid);
                if (snap && snap.cursorRow !== undefined) {
                    return { row: snap.cursorRow, col: snap.cursorCol };
                }
            }
        }
        // Fallback: bottom-left.
        var lines = getPaneContentLines(s);
        return { row: Math.max(0, lines.length - 1), col: 0 };
    }

    // startSelection initializes a new text selection at the given content
    // position. Called on the first Shift+Arrow press or on mouse press+shift.
    function startSelection(s, row, col) {
        s.selectionActive = true;
        s.selectionPane = s.splitViewTab;
        s.selectionStartRow = row;
        s.selectionStartCol = col;
        s.selectionEndRow = row;
        s.selectionEndCol = col;
        s.selectionByMouse = false;
    }

    // extendSelection moves the selection end point by (dRow, dCol).
    // Clamps to content boundaries.
    function extendSelection(s, dRow, dCol) {
        var lines = getPaneContentLines(s);
        if (lines.length === 0) return;
        var r = s.selectionEndRow + dRow;
        var c = s.selectionEndCol + dCol;
        r = Math.max(0, Math.min(r, lines.length - 1));
        var lineLen = (lines[r] || '').length;
        if (c < 0) {
            // Wrap to end of previous line.
            if (r > 0) {
                r--;
                c = (lines[r] || '').length;
            } else {
                c = 0;
            }
        } else if (c > lineLen) {
            // Wrap to start of next line.
            if (r < lines.length - 1) {
                r++;
                c = 0;
            } else {
                c = lineLen;
            }
        }
        s.selectionEndRow = r;
        s.selectionEndCol = c;
    }

    // extractSelectedText extracts the plain text within the current selection
    // boundaries. Handles both forward and backward selections.
    function extractSelectedText(s) {
        var lines = getPaneContentLines(s);
        if (lines.length === 0) return '';

        // Normalize to (startRow, startCol) <= (endRow, endCol).
        var sr = s.selectionStartRow, sc = s.selectionStartCol;
        var er = s.selectionEndRow, ec = s.selectionEndCol;
        if (sr > er || (sr === er && sc > ec)) {
            var tmp;
            tmp = sr; sr = er; er = tmp;
            tmp = sc; sc = ec; ec = tmp;
        }

        if (sr === er) {
            // Single-line selection.
            var line = lines[sr] || '';
            return line.substring(sc, ec);
        }

        // Multi-line selection.
        var result = [];
        // First line: from startCol to end.
        result.push((lines[sr] || '').substring(sc));
        // Middle lines: full lines.
        for (var i = sr + 1; i < er; i++) {
            result.push(lines[i] || '');
        }
        // Last line: from start to endCol.
        result.push((lines[er] || '').substring(0, ec));
        return result.join('\n');
    }

    // clearSelection resets all selection state.
    function clearSelection(s) {
        s.selectionActive = false;
        s.selectionPane = '';
        s.selectionStartRow = 0;
        s.selectionStartCol = 0;
        s.selectionEndRow = 0;
        s.selectionEndCol = 0;
        s.selectionByMouse = false;
        s.selectedText = '';
    }

    // --- Per-message-type handler functions ---
    // Extracted from wizardUpdateImpl for modularity and testability.

    // handleWindowResize processes WindowSize messages: sets dimensions,
    // syncs viewports, resizes interactive CaptureSession terminals, and
    // handles first-render initialization.
    function handleWindowResize(msg, s) {
        s.width = msg.width;
        s.height = msg.height;

        // T120: Sync viewport dimensions from update, not view.
        syncMainViewport(s);

        // T336: Resize interactive CaptureSession terminals.
        if (s.splitViewEnabled) {
            var h = s.height || C.DEFAULT_ROWS;
            var vpH = Math.max(3, h - CHROME_ESTIMATE);
            var minP = 3;
            var wH = Math.max(minP, Math.floor(vpH * (s.splitViewRatio || 0.6)));
            wH = Math.min(wH, vpH - minP - 1);
            var cH = vpH - wH - 1;
            var paneRows = Math.max(3, cH - 3);
            var paneCols = Math.max(20, (s.width || 80) - 4);
            // Task 8: Shell tab removed from interactive tabs.
            var interactiveTabs = ['claude', 'verify'];
            for (var ti = 0; ti < interactiveTabs.length; ti++) {
                var resizeTab = interactiveTabs[ti];
                var resizeSession = getInteractivePaneSession(s, resizeTab);
                if (resizeSession && typeof resizeSession.resize === 'function') {
                    try { resizeSession.resize(paneRows, paneCols); } catch (e) { log.debug('resize: ' + resizeTab + ' session.resize failed: ' + (e.message || e)); }
                }
            }
            // Task 44: Sync SessionManager's internal VTerm dimensions so
            // childScreen()/snapshot() return properly-sized ANSI output.
            if (typeof tuiMux !== 'undefined' && tuiMux &&
                typeof tuiMux.resize === 'function') {
                try { tuiMux.resize(paneRows, paneCols); } catch (e) {
                    log.debug("session manager resize failed", { error: e.message || String(e) });
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

    // handleOverlays intercepts user input when an overlay is active.
    // Returns [s, cmd] if the overlay consumed the message, or null
    // if no overlay was active and the caller should continue dispatch.
    function handleOverlays(msg, s) {
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
                if (rk === 'pgdown' || rk === 'space') {
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
            if (msg.type === 'MouseWheel' && s.reportVp) {
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
            if (msg.type === 'MouseClick') {
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

                    // Send to Claude PTY via the pinned Claude pane proxy.
                    var claudeQuestionSession = getInteractivePaneSession(s, 'claude');
                    if (claudeQuestionSession && typeof claudeQuestionSession.write === 'function') {
                        try {
                            claudeQuestionSession.write(responseText + '\r');
                            log.printf('T46: sent response to Claude: %s', responseText);
                        } catch (e) {
                            // T393: Surface error to user — keep claudeQuestionDetected
                            // true so renderClaudeQuestionPrompt renders the error line.
                            log.printf('T46: Claude write failed: %s', String(e));
                            s.claudeQuestionLine = 'Error sending response: ' + String(e);
                            s.claudeQuestionInputActive = false;
                            s.claudeQuestionInputText = '';
                            return [s, null];
                        }
                    } else {
                        // T393: tuiMux not available — keep claudeQuestionDetected
                        // true so the error is visible to the user.
                        log.printf('T46: Claude session write not available');
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

        // No overlay intercepted — return null so caller continues dispatch.
        return null;
    }

    // handleKeyMessage processes Key messages that were not intercepted
    // by overlays. Covers: verify session keys, split-view keybindings,
    // help/cancel/navigation, viewport scroll, and screen-specific shortcuts.
    function handleKeyMessage(msg, s) {
        var k = msg.key;
        var activeVerifySession = getInteractivePaneSession(s, 'verify');

        // Live verify session: intercept Ctrl+C to stop verification
        // instead of showing the cancel dialog.
        // First Ctrl+C sends SIGINT; second within 2s sends SIGKILL
        // (handles processes that ignore SIGINT).
        if (k === 'ctrl+c' && activeVerifySession && !s.verifyShellExited) {
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
            // T61: Ctrl+Tab cycles through all focusable targets:
            // wizard → claude → output → verify (if active) → wizard → ...
            if (k === 'ctrl+tab') {
                var allTargets = ['wizard'];
                var tabs = listSplitViewTabs(s);
                for (var ti = 0; ti < tabs.length; ti++) {
                    allTargets.push(tabs[ti]);
                }
                // Determine current position in the cycle.
                var current = (s.splitViewFocus === 'wizard')
                    ? 'wizard' : (s.splitViewTab || 'claude');
                var curIdx = allTargets.indexOf(current);
                if (curIdx < 0) curIdx = 0;
                var nextTarget = allTargets[(curIdx + 1) % allTargets.length];
                if (nextTarget === 'wizard') {
                    s.splitViewFocus = 'wizard';
                } else {
                    s.splitViewFocus = 'claude';
                    s.splitViewTab = nextTarget;
                }
                return [s, null];
            }

            // T62: Copy/paste — Ctrl+Shift+C copies selected text to clipboard.
            if (k === 'ctrl+shift+c') {
                if (s.selectionActive) {
                    var selText = extractSelectedText(s);
                    if (selText) {
                        try {
                            output.toClipboard(selText);
                            s.clipboardFlash = 'Copied ' + selText.length + ' chars';
                            s.clipboardFlashAt = Date.now();
                            s.selectedText = selText;
                        } catch (e) {
                            s.clipboardFlash = 'Copy failed: ' + (e.message || e);
                            s.clipboardFlashAt = Date.now();
                        }
                    }
                    clearSelection(s);
                }
                return [s, null];
            }

            // T62: Paste — Ctrl+Shift+V pastes clipboard into active PTY session.
            if (k === 'ctrl+shift+v') {
                var tab = s.splitViewTab;
                // Only paste into interactive panes (Claude, verify), not output.
                if (tab !== 'output') {
                    if (tab === 'verify' &&
                        (s.verifyFallbackRunning || s.verifyMode === 'oneshot' ||
                            s.verifyMode === 'textonly' || s.verifyShellExited)) {
                        s.clipboardFlash = 'Paste unavailable while verify output is read-only';
                        s.clipboardFlashAt = Date.now();
                        return [s, null];
                    }
                    try {
                        var pastedText = output.fromClipboard();
                        if (pastedText) {
                            var pasteSession = getInteractivePaneSession(s, tab);
                            if (pasteSession && typeof pasteSession.write === 'function') {
                                pasteSession.write(pastedText);
                                s.clipboardFlash = 'Pasted ' + pastedText.length + ' chars';
                                s.clipboardFlashAt = Date.now();
                            }
                        }
                    } catch (e) {
                        s.clipboardFlash = 'Paste failed: ' + (e.message || e);
                        s.clipboardFlashAt = Date.now();
                    }
                }
                return [s, null];
            }

            // T62: Shift+Arrow keys — text selection in split-view pane.
            if (k === 'shift+up' || k === 'shift+down' ||
                k === 'shift+left' || k === 'shift+right') {
                // Clear selection if pane tab changed since selection started.
                if (s.selectionActive && s.selectionPane !== s.splitViewTab) {
                    clearSelection(s);
                }
                // Start new selection if not active.
                if (!s.selectionActive) {
                    var cur = getCursorInPane(s);
                    startSelection(s, cur.row, cur.col);
                }
                // Extend selection.
                var dRow = 0, dCol = 0;
                if (k === 'shift+up')    dRow = -1;
                if (k === 'shift+down')  dRow = 1;
                if (k === 'shift+left')  dCol = -1;
                if (k === 'shift+right') dCol = 1;
                extendSelection(s, dRow, dCol);
                s.selectedText = extractSelectedText(s);
                return [s, null];
            }

            // T62: Escape clears active selection.
            if (k === 'esc' && s.selectionActive) {
                clearSelection(s);
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
                // Verify tab has one canonical interactive shell mode plus degraded
                // one-shot and text-only fallback modes.
                if (s.splitViewTab === 'verify') {
                    var verifySession = getInteractivePaneSession(s, 'verify');
                    if (s.verifyShellExited && (k === 'p' || k === 'f' || k === 'c')) {
                        var exitedChoice = (k === 'p') ? 'pass' : ((k === 'f') ? 'fail' : 'continue');
                        return handleVerifySignal(s, exitedChoice);
                    }
                    // Degraded verify modes and the post-shell-exit interactive state
                    // are read-only from the pane's perspective: users can scroll the
                    // visible output here, but terminal input is not forwarded.
                    if (s.verifyFallbackRunning || !verifySession ||
                        s.verifyMode === 'oneshot' || s.verifyMode === 'textonly' ||
                        s.verifyShellExited) {
                        if (k === 'ctrl+c') {
                            s.showConfirmCancel = true;
                            s.confirmCancelFocus = 0;
                            return [s, null];
                        }
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
                        return [s, null];
                    }
                    // Live verify session: fully interactive — only pane-management
                    // keys are reserved, everything else goes to the terminal.
                    if (!INTERACTIVE_RESERVED_KEYS[k]) {
                        var vBytes = keyToTermBytes(k);
                        if (vBytes !== null && verifySession &&
                            typeof verifySession.write === 'function') {
                            try {
                                verifySession.write(vBytes);
                                s.verifyViewportOffset = 0;
                                s.verifyAutoScroll = true;
                                if (s.verifyWriteError) {
                                    s.verifyWriteError = '';
                                }
                            } catch (e) {
                                // Task 9: Surface verify write errors.
                                s.verifyWriteError = e.message || String(e);
                                s.verifyWriteErrorAt = Date.now();
                                log.warn('verify write failed', {
                                    key: k,
                                    error: e.message || String(e)
                                });
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
                            // Clear any previous write error on success.
                            if (s.claudeWriteError) {
                                s.claudeWriteError = '';
                            }
                        } catch (e) {
                            // Task 9: Surface write errors instead of silently
                            // swallowing — user sees a transient indicator.
                            s.claudeWriteError = e.message || String(e);
                            s.claudeWriteErrorAt = Date.now();
                            log.warn('claude write failed', {
                                key: k,
                                error: e.message || String(e)
                            });
                        }
                    }
                    return [s, null];
                }
            }
            // T394: Ctrl+] passthrough is now handled by toggleModel wrapper
            // in BubbleTea (see startWizard). The wrapper properly calls
            // ReleaseTerminal before RunPassthrough, preventing stdin
            // contention. ToggleReturn message handled below.
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
        if (k === 'z' && s.wizardState === 'BRANCH_BUILDING' && activeVerifySession && !s.verifyShellExited) {
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
        // T007 (Task 7): BRANCH_BUILDING interactive verify shell — p/f/c signals.
        // 'p' — mark current branch as PASSED (verification succeeded).
        // 'f' — mark current branch as FAILED (verification failed).
        // 'c' — CONTINUE/skip this branch without marking pass or fail.
        // Only active after the canonical interactive verify shell has exited and
        // the UI is prompting for an explicit outcome.
        if (s.wizardState === 'BRANCH_BUILDING' && activeVerifySession &&
            (!s.verifyMode || s.verifyMode === 'interactive') &&
            s.verifyShellExited) {
            if (k === 'p' || k === 'f' || k === 'c') {
                var choice = (k === 'p') ? 'pass' : ((k === 'f') ? 'fail' : 'continue');
                return handleVerifySignal(s, choice);
            }
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
        if (k === 'space' && s.wizardState === 'PLAN_EDITOR') {
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
        // contention. ToggleReturn message handled by handleToggleReturn.
        return [s, null];
    }

    // handleToggleReturn processes ToggleReturn messages from the toggle
    // model after passthrough exits or was skipped.
    function handleToggleReturn(msg, s) {
        if (msg.skipped) {
            // No interactive session — show notification.
            s.claudeAutoAttachNotif = 'Passthrough not available \u2014 no active interactive session';
            s.claudeAutoAttachNotifAt = Date.now();
        }
        return [s, null];
    }

    // handleMouseMessage processes Mouse messages that were not intercepted
    // by overlays. Covers: child terminal forwarding, verify scroll, split
    // pane scroll, main viewport scroll, click dispatch, and T62 selection.
    function handleMouseMessage(msg, s) {
        // v2 split mouse types: MouseClick, MouseRelease, MouseMotion, MouseWheel.
        // Type-based dispatch replaces the v1 isWheel/action pattern.

        // T62: Mouse-based text selection in split-view pane.
        // Intercept Shift+Click to initiate/extend selection, and motion
        // events to extend an active mouse selection. These must be checked
        // before child forwarding to avoid sending selection gestures to PTY.
        var hasShift = msg.mod && msg.mod.indexOf('shift') >= 0;
        if (s.splitViewEnabled && hasShift && msg.type !== 'MouseWheel') {
            var ofs = computeSplitPaneContentOffset(s);
            var vr = getPaneVisibleRange(s);
            var mRow = msg.y - ofs.row + vr.startLine;
            var mCol = msg.x - ofs.col;
            var lines = getPaneContentLines(s);
            mRow = Math.max(0, Math.min(mRow, lines.length - 1));
            mCol = Math.max(0, Math.min(mCol, (lines[mRow] || '').length));

            if (msg.type === 'MouseClick') {
                if (!s.selectionActive) {
                    startSelection(s, mRow, mCol);
                } else {
                    // Shift+Click extends to the new position.
                    s.selectionEndRow = mRow;
                    s.selectionEndCol = mCol;
                }
                s.selectionByMouse = true;
                s.selectedText = extractSelectedText(s);
                return [s, null];
            }
            if (msg.type === 'MouseMotion' && s.selectionActive && s.selectionByMouse) {
                s.selectionEndRow = mRow;
                s.selectionEndCol = mCol;
                s.selectedText = extractSelectedText(s);
                return [s, null];
            }
        }

        // T62: Release ends mouse drag selection (no-op but prevents forwarding).
        if (s.selectionActive && s.selectionByMouse && msg.type === 'MouseRelease') {
            // Selection is finalized — user can copy with Ctrl+Shift+C.
            return [s, null];
        }

        // T327/T328: Forward mouse events to focused child terminal.
        // Intercepts motion, release, and wheel before wizard-managed handlers.
        // Press events are NOT intercepted here — they go through
        // handleMouseClick for zone detection first.
        if (s.splitViewEnabled &&
            s.splitViewFocus === 'claude' && s.splitViewTab !== 'output') {
            if (msg.type === 'MouseMotion') {
                var ofs = computeSplitPaneContentOffset(s);
                var mb = mouseToTermBytes(msg, ofs.row, ofs.col);
                if (mb && writeMouseToPane(mb, s)) {
                    return [s, null];
                }
            }
            if (msg.type === 'MouseRelease') {
                var ofs = computeSplitPaneContentOffset(s);
                var mb = mouseToTermBytes(msg, ofs.row, ofs.col);
                if (mb && writeMouseToPane(mb, s)) {
                    return [s, null];
                }
            }
            if (msg.type === 'MouseWheel') {
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
        if (msg.type === 'MouseWheel' &&
            (getInteractivePaneSession(s, 'verify') || s.verifyFallbackRunning ||
                s.verifyMode === 'oneshot' || s.verifyMode === 'textonly' ||
                (!!s.verifyScreen && (!s.splitViewEnabled || s.splitViewTab === 'verify'))) &&
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
        if (msg.type === 'MouseWheel' && s.splitViewEnabled &&
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

        if (msg.type === 'MouseWheel' && s.vp) {
            if (msg.button === 'wheel up') {
                s.vp.scrollUp(3);
                return [s, null];
            }
            if (msg.button === 'wheel down') {
                s.vp.scrollDown(3);
                return [s, null];
            }
        }

        if (msg.type === 'MouseClick') {
            return handleMouseClick(msg, s);
        }

        return [s, null];
    }

    // handleTickMessage processes Tick messages — programmatic continuations
    // dispatched by timer-based polling (analysis, execution, verify, Claude
    // screenshot, conversation, PR creation, transient notification dismiss).
    function handleTickMessage(msg, s) {
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
        // T5: On-demand Claude spawn polling (Ask Claude in non-auto modes).
        if (msg.id === 'claude-spawn-poll') {
            if (s.claudeOnDemandSpawning) {
                return [s, tea.tick(C.CLAUDE_CHECK_POLL_MS, 'claude-spawn-poll')];
            }
            // Spawn completed (success or failure) — UI will re-render.
            return [s, null];
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

    // --- wizardUpdateImpl — main BubbleTea update dispatch ---
    //
    // Thin dispatch function that routes messages to per-type handlers.
    // Each handler is independently testable. The dispatch order is:
    //   1. Poll mux events (always)
    //   2. WindowSize → handleWindowResize (before state reset)
    //   3. Wizard state transition reset
    //   4. Overlay interception (all non-Tick messages)
    //   5. Per-type dispatch: Key, ToggleReturn, Mouse, Tick
    function wizardUpdateImpl(msg, s) {
        if (typeof tuiMux !== 'undefined' && tuiMux &&
            typeof tuiMux.pollEvents === 'function') {
            try {
                tuiMux.pollEvents();
            } catch (e) {
                log.debug('wizardUpdateImpl: tuiMux.pollEvents failed: ' + (e.message || e));
            }
        }

        // WindowSize — always handle (before state reset and overlays).
        if (msg.type === 'WindowSize') {
            return handleWindowResize(msg, s);
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
            var overlayResult = handleOverlays(msg, s);
            if (overlayResult) return overlayResult;
        }

        // Per-message-type dispatch.
        if (msg.type === 'Key') return handleKeyMessage(msg, s);
        if (msg.type === 'ToggleReturn') return handleToggleReturn(msg, s);
        if (msg.type === 'MouseClick' || msg.type === 'MouseRelease' || msg.type === 'MouseMotion' || msg.type === 'MouseWheel') return handleMouseMessage(msg, s);
        if (msg.type === 'Tick') return handleTickMessage(msg, s);

        return [s, null];
    }

    // Cross-chunk export.
    prSplit._wizardUpdateImpl = wizardUpdateImpl;
    prSplit._computeSplitPaneContentOffset = computeSplitPaneContentOffset;
    prSplit._writeMouseToPane = writeMouseToPane;

    // T62: Selection helpers (exported for chrome rendering and testing).
    prSplit._getPaneContentLines = getPaneContentLines;
    prSplit._getPaneVisibleRange = getPaneVisibleRange;
    prSplit._getCursorInPane = getCursorInPane;
    prSplit._startSelection = startSelection;
    prSplit._extendSelection = extendSelection;
    prSplit._extractSelectedText = extractSelectedText;
    prSplit._clearSelection = clearSelection;

})(globalThis.prSplit);
