'use strict';
// pr_split_16_tui_core.js — TUI: BubbleTea model, update/view, handlers, pipeline, launch
// Dependencies: chunks 00-15 must be loaded first.
// Requires Go-injected globals: tui, ctx, output, log, prSplitConfig, tuiMux.

(function(prSplit) {

    if (typeof tui === 'undefined' || typeof ctx === 'undefined' ||
        typeof output === 'undefined') { return; }

    // Cross-chunk imports — libraries from chunk 15.
    var tea = prSplit._tea;
    var lipgloss = prSplit._lipgloss;
    var zone = prSplit._zone;
    var viewportLib = prSplit._viewportLib;
    var scrollbarLib = prSplit._scrollbarLib;
    var COLORS = prSplit._COLORS;
    var resolveColor = prSplit._resolveColor;
    var repeatStr = prSplit._repeatStr;
    var styles = prSplit._wizardStyles;

    // Cross-chunk imports — renderers and views from chunk 15.
    var renderTitleBar = prSplit._renderTitleBar;
    var renderNavBar = prSplit._renderNavBar;
    var renderStatusBar = prSplit._renderStatusBar;
    var renderProgressBar = prSplit._renderProgressBar;
    var renderClaudePane = prSplit._renderClaudePane;
    var renderOutputPane = prSplit._renderOutputPane;  // T44
    var viewForState = prSplit._viewForState;
    var viewHelpOverlay = prSplit._viewHelpOverlay;
    var viewConfirmCancelOverlay = prSplit._viewConfirmCancelOverlay;
    var viewReportOverlay = prSplit._viewReportOverlay;
    var viewMoveFileDialog = prSplit._viewMoveFileDialog;
    var viewRenameSplitDialog = prSplit._viewRenameSplitDialog;
    var viewMergeSplitsDialog = prSplit._viewMergeSplitsDialog;
    var viewClaudeConvoOverlay = prSplit._viewClaudeConvoOverlay;

    // Cross-chunk imports — state and handlers from chunks 13-14.
    var st = prSplit._state;
    var tuiState = prSplit._tuiState;
    var WizardState = prSplit.WizardState;
    var buildReport = prSplit._buildReport;
    var buildCommands = prSplit._buildCommands;
    var handleConfigState = prSplit._handleConfigState;
    var handlePlanReviewState = prSplit._handlePlanReviewState;
    var handlePlanEditorState = prSplit._handlePlanEditorState;
    var handleErrorResolutionState = prSplit._handleErrorResolutionState;
    var handleFinalizationState = prSplit._handleFinalizationState;

    // -----------------------------------------------------------------------
    //  BubbleTea Model — init / update / view (T020-T025)
    // -----------------------------------------------------------------------

    function createWizardModel() {
        var wizard = new WizardState();
        prSplit._wizardState = wizard;

        // Track transitions to update the TUI model state.
        wizard.onTransition(function(from, to, data) {
            log.printf('wizard: %s \u2192 %s', from, to);
        });

        var vp = viewportLib.new(80, 24);
        vp.setMouseWheelEnabled(true);
        var sb = scrollbarLib.new();

        // Dedicated viewport + scrollbar for the report overlay.
        var reportVp = viewportLib.new(80, 20);
        reportVp.setMouseWheelEnabled(true);
        var reportSb = scrollbarLib.new();

        // Named lifecycle functions — exported for unit testing.
        var _initFn = function() {
            return {
                // Wizard state.
                wizard: wizard,
                wizardState: 'IDLE',

                // Dimensions.
                width: 80,
                height: 24,

                // Viewport.
                vp: vp,
                scrollbar: sb,

                // Time.
                startTime: Date.now(),

                // UI state.
                showHelp: false,
                showConfirmCancel: false,
                showAdvanced: false,
                showingReport: false,
                reportContent: '',
                reportVp: reportVp,
                reportSb: reportSb,
                selectedSplitIdx: 0,
                selectedFileIdx: 0,
                isProcessing: false,

                // Editor dialog state.
                activeEditorDialog: null,  // 'move' | 'rename' | 'merge' | null
                editorDialogState: {},

                // Editor inline state (T17).
                editorTitleEditing: false,      // true when inline title edit is active
                editorTitleEditingIdx: -1,      // split index being edited (-1 = none)
                editorTitleText: '',            // current title text buffer
                editorCheckedFiles: {},         // { 'splitIdx-fileIdx': true } for checked files
                editorValidationErrors: [],     // validation errors from save attempt
                editorFileDetailExpanded: false, // show enhanced file detail panel

                // Focus system.
                focusIndex: 0,
                _prevWizardState: null,

                // Claude availability (CONFIG screen).
                claudeCheckStatus: null,   // null | 'checking' | 'available' | 'unavailable'
                claudeResolvedInfo: null,  // null | { command, type }
                claudeCheckError: null,    // null | error string
                claudeCheckRunning: false, // true while async resolveAsync is running
                claudeCheckProgressMsg: '', // progress message from resolveAsync
                userHasSelectedStrategy: false, // T42: true when user manually selects a strategy

                // T43: Config validation state.
                configValidationError: null,   // null | error string (shown inline on CONFIG)
                availableBranches: [],         // branches from 'git branch --list' on auto-detect failure

                // Analysis progress.
                analysisSteps: [],
                analysisProgress: -1,

                // Execution state.
                executionResults: [],
                executingIdx: 0,
                executionRunning: false,
                executionError: null,
                executionNextStep: null,
                executionBranchTotal: 0,
                executionProgressMsg: '',

                // Equivalence check state.
                equivRunning: false,
                equivError: null,

                // Verification state (per-branch, after branch creation).
                verificationResults: [],   // Array parallel to splits
                verifyingIdx: -1,          // -1=not started, 0..N=in progress, N=done
                verifyOutput: {},          // { branchName: [line, ...] }
                expandedVerifyBranch: null, // branchName for expandable output

                // Live verification session (CaptureSession).
                activeVerifySession: null,     // CaptureSession JS object (or null)
                activeVerifyWorktree: null,    // worktree path for cleanup
                activeVerifyBranch: null,      // branch name being verified
                activeVerifyDir: null,         // base dir for worktree cleanup
                activeVerifyStartTime: 0,      // start time for duration tracking
                verifyViewportOffset: 0,       // scroll offset (lines from bottom)
                verifyAutoScroll: true,        // auto-scroll to bottom
                lastVerifyInterruptTime: 0,    // timestamp of last Ctrl+C interrupt

                // Fallback verification (async, when CaptureSession unavailable).
                verifyFallbackRunning: false,  // true while async verifySplitAsync is running
                verifyFallbackError: null,     // error string from fallback verification

                // Split-view (Claude window-in-window).
                splitViewEnabled: false,       // true when split-view is active
                splitViewRatio: 0.6,           // wizard gets this fraction of content height
                splitViewFocus: 'wizard',      // 'wizard' or 'claude' — which pane is focused
                splitViewTab: 'claude',        // T44: 'claude' | 'output' — active tab in split-view bottom pane
                claudeScreenshot: '',          // cached plain-text screenshot from tuiMux
                claudeScreen: '',              // cached ANSI-styled screen from tuiMux (T28)
                claudeViewOffset: 0,           // scroll offset in Claude pane (lines from bottom)

                // T45: Auto-attach Claude pane state.
                claudeAutoAttached: false,     // true once auto-attach has fired (prevents re-trigger)
                claudeManuallyDismissed: false, // true when user explicitly closed split-view via Ctrl+L
                claudeAutoAttachNotif: '',     // transient notification text (auto-dismissed after 5s)
                claudeAutoAttachNotifAt: 0,    // Date.now() when notification was set

                // T46: Claude question detection state.
                claudeQuestionDetected: false,  // true when question pattern detected in Claude output
                claudeQuestionLine: '',         // the detected question line from Claude's output
                claudeQuestionInputText: '',    // user's response text buffer
                claudeQuestionInputActive: false, // user has focused the inline response input
                claudeConversations: [],        // Q&A history [{question: string, answer: string, ts: number}]

                // T44: Process output capture state.
                outputLines: [],               // accumulated output buffer (strings, may contain ANSI)
                outputViewOffset: 0,           // scroll offset in output pane (lines from bottom)
                outputAutoScroll: true,        // auto-scroll to latest output

                // Claude conversation (T16).
                claudeConvo: {
                    active: false,             // conversation overlay is visible
                    context: null,             // 'plan-review' | 'error-resolution' | null
                    history: [],               // [{ role: 'user'|'claude', text: string, ts: number }]
                    inputText: '',             // current text buffer
                    sending: false,            // async send in flight
                    waitingForTool: null,       // MCP tool being waited on (or null)
                    lastError: null,           // last error string
                    scrollOffset: 0            // scroll offset in history view
                },

                // Auto-split pipeline state.
                autoSplitRunning: false,
                autoSplitResult: null,

                // Claude crash detection.
                claudeCrashDetected: false,
                lastClaudeHealthCheckMs: 0,

                // Results.
                equivalenceResult: null,
                errorDetails: null,

                // First render flag.
                needsInitClear: true
            };
        };

        var _updateFn = function(msg, s) {
            // WindowSize — always handle.
            if (msg.type === 'WindowSize') {
                s.width = msg.width;
                s.height = msg.height;

                if (s.needsInitClear) {
                    s.needsInitClear = false;
                    // Start the wizard on first render.
                    s.wizardState = 'CONFIG';
                    wizard.transition('CONFIG');
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
                    // Copy report to clipboard.
                    if (rk === 'c') {
                        output.toClipboard(s.reportContent);
                        return [s, null];
                    }
                    // Scroll navigation.
                    if (rk === 'j' || rk === 'down') {
                        if (s.reportVp) s.reportVp.scrollDown(1);
                        return [s, null];
                    }
                    if (rk === 'k' || rk === 'up') {
                        if (s.reportVp) s.reportVp.scrollUp(1);
                        return [s, null];
                    }
                    if (rk === 'pgdown' || rk === ' ') {
                        if (s.reportVp) s.reportVp.halfPageDown();
                        return [s, null];
                    }
                    if (rk === 'pgup') {
                        if (s.reportVp) s.reportVp.halfPageUp();
                        return [s, null];
                    }
                    if (rk === 'home' || rk === 'g') {
                        if (s.reportVp) s.reportVp.gotoTop();
                        return [s, null];
                    }
                    if (rk === 'end') {
                        if (s.reportVp) s.reportVp.gotoBottom();
                        return [s, null];
                    }
                    return [s, null];
                }
                // Mouse wheel scrolling within report overlay.
                if (msg.type === 'Mouse' && msg.isWheel && s.reportVp) {
                    if (msg.button === 'wheel up') {
                        s.reportVp.scrollUp(3);
                        return [s, null];
                    }
                    if (msg.button === 'wheel down') {
                        s.reportVp.scrollDown(3);
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
                        // Record in conversation history (cap at 100 entries).
                        s.claudeConversations.push({
                            question: s.claudeQuestionLine,
                            answer: responseText,
                            ts: Date.now()
                        });
                        if (s.claudeConversations.length > 100) {
                            s.claudeConversations = s.claudeConversations.slice(-80);
                        }

                        // Send to Claude PTY via tuiMux.writeToChild.
                        if (typeof tuiMux !== 'undefined' && tuiMux &&
                            typeof tuiMux.writeToChild === 'function') {
                            try {
                                tuiMux.writeToChild(responseText + '\r');
                                log.printf('T46: sent response to Claude: %s', responseText);
                            } catch (e) {
                                log.printf('T46: writeToChild failed: %s', String(e));
                            }
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

            } // end: if (msg.type !== 'Tick') — overlay guard

            // Global key bindings.
            if (msg.type === 'Key') {
                var k = msg.key;

                // Live verify session: intercept Ctrl+C to stop verification
                // instead of showing the cancel dialog.
                // First Ctrl+C sends SIGINT; second within 2s sends SIGKILL
                // (handles processes that ignore SIGINT).
                if (k === 'ctrl+c' && s.activeVerifySession) {
                    var now = Date.now();
                    if (s.lastVerifyInterruptTime > 0 && (now - s.lastVerifyInterruptTime) < 2000) {
                        // Double Ctrl+C — force kill.
                        try { s.activeVerifySession.kill(); } catch (e) { /* ignore */ }
                    } else {
                        // First Ctrl+C — graceful interrupt.
                        try { s.activeVerifySession.interrupt(); } catch (e) { /* ignore */ }
                    }
                    s.lastVerifyInterruptTime = now;
                    return [s, null];
                }

                // Live verify session: ↑/↓ scroll the output viewport.
                if (s.activeVerifySession) {
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
                        s.verifyViewportOffset = 999999; // far back
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
                    if (s.splitViewEnabled) {
                        // T45: User re-opened — clear manual dismiss flag.
                        s.claudeManuallyDismissed = false;
                        // Start screenshot polling. Preserve focusIndex.
                        return [s, tea.tick(100, 'claude-screenshot')];
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
                    // Ctrl+Tab switches focus between wizard and Claude panes.
                    if (k === 'ctrl+tab' && !s.activeVerifySession) {
                        s.splitViewFocus = (s.splitViewFocus === 'wizard') ? 'claude' : 'wizard';
                        return [s, null];
                    }
                    // Ctrl+= / Ctrl+- to adjust ratio.
                    if (k === 'ctrl++' || k === 'ctrl+=') {
                        s.splitViewRatio = Math.min(0.8, s.splitViewRatio + 0.1);
                        return [s, null];
                    }
                    if (k === 'ctrl+-') {
                        s.splitViewRatio = Math.max(0.2, s.splitViewRatio - 0.1);
                        return [s, null];
                    }
                    // T44: Ctrl+O switches between Claude and Output tabs in split-view bottom pane.
                    if (k === 'ctrl+o') {
                        s.splitViewTab = (s.splitViewTab === 'claude') ? 'output' : 'claude';
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
                                s.outputViewOffset = 999999;
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
                            s.claudeViewOffset = 999999;
                            return [s, null];
                        }
                        if (k === 'end') {
                            s.claudeViewOffset = 0;
                            return [s, null];
                        }
                        // Forward non-reserved keys to Claude's PTY.
                        if (!CLAUDE_RESERVED_KEYS[k]) {
                            var bytes = keyToTermBytes(k);
                            if (bytes !== null && typeof tuiMux !== 'undefined' && tuiMux &&
                                typeof tuiMux.writeToChild === 'function') {
                                try {
                                    tuiMux.writeToChild(bytes);
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
                // termmux toggle — only if Claude child is attached.
                if (k === 'ctrl+]') {
                    if (typeof tuiMux !== 'undefined' && tuiMux &&
                        typeof tuiMux.switchTo === 'function' &&
                        (typeof tuiMux.hasChild !== 'function' || tuiMux.hasChild())) {
                        tuiMux.switchTo('claude');
                    }
                    return [s, null];
                }
            }

            // Mouse handling.
            // Wheel events must be checked BEFORE press — wheel events
            // have action:"press" AND isWheel:true, so the press guard
            // would intercept them and send them to handleMouseClick.

            // Live verify session mouse wheel → scroll output viewport.
            if (msg.type === 'Mouse' && msg.isWheel && s.activeVerifySession) {
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
                // Split-view: poll Claude screenshot.
                if (msg.id === 'claude-screenshot') {
                    return pollClaudeScreenshot(s);
                }
                // Claude conversation: poll for async send/wait completion.
                if (msg.id === 'claude-convo-poll') {
                    return pollClaudeConvo(s);
                }
                return [s, null];
            }

            return [s, null];
        };

        var _viewFn = function(s) {
            var w = s.width || 80;
            var h = s.height || 24;

            // Title bar.
            var titleBar = renderTitleBar(s);

            // Divider.
            var divider = styles.divider().render(repeatStr('\u2500', w));

            // Navigation bar — compute nav-next focus flag.
            var focusElems = getFocusElements(s);
            s._isNavNextFocused = false;
            if (focusElems.length > 0) {
                var lastElem = focusElems[focusElems.length - 1];
                if (lastElem && lastElem.type === 'nav' &&
                    (s.focusIndex || 0) === focusElems.length - 1) {
                    s._isNavNextFocused = true;
                }
            }
            var navBar = renderNavBar(s);

            // Status bar.
            var statusBar = renderStatusBar(s);

            // Screen content.
            var screenContent = viewForState(s);

            // Wrap in viewport.
            if (s.vp) {
                s.vp.setWidth(w);
                // Reserve chrome lines dynamically from actual rendered heights.
                // +2 for the two dividers (each 1 line).
                var chromeH = lipgloss.height(titleBar) + 2 + lipgloss.height(navBar) + lipgloss.height(statusBar);
                var vpHeight = Math.max(3, h - chromeH);

                if (s.splitViewEnabled) {
                    // Split-view: wizard top, Claude bottom.
                    // -1 for the pane divider between them.
                    // Minimum split requires 3 + 1 + 3 = 7 lines.
                    var minPaneH = 3;
                    var wizardH = Math.max(minPaneH, Math.floor(vpHeight * s.splitViewRatio));
                    // Clamp wizardH so Claude pane gets at least minPaneH.
                    wizardH = Math.min(wizardH, vpHeight - minPaneH - 1);
                    var claudeH = vpHeight - wizardH - 1;

                    if (wizardH < minPaneH || claudeH < minPaneH) {
                        // Terminal too small for split view; fall through to normal mode.
                        s.splitViewEnabled = false;
                    }
                }
                if (s.splitViewEnabled) {

                    // Wizard viewport.
                    s.vp.setHeight(wizardH);
                    s.vp.setContent(screenContent);

                    if (s.scrollbar) {
                        s.scrollbar.setViewportHeight(wizardH);
                        s.scrollbar.setContentHeight(s.vp.totalLineCount());
                        s.scrollbar.setYOffset(s.vp.yOffset());
                        s.scrollbar.setChars('\u2588', '\u2591');
                        s.scrollbar.setThumbForeground(resolveColor(
                            s.splitViewFocus === 'wizard' ? COLORS.primary : COLORS.border));
                        s.scrollbar.setTrackForeground(resolveColor(COLORS.border));
                    }

                    var vpView = s.vp.view();
                    var sbView = s.scrollbar ? s.scrollbar.view() : '';
                    var wizardPane = lipgloss.joinHorizontal(lipgloss.Top, vpView, sbView);

                    // Pane divider with tab bar and split-view hint.
                    // T44: Tab bar: [Claude] [Output] with active/inactive styling.
                    var claudeTabLabel = s.splitViewTab === 'claude'
                        ? styles.primaryButton().render(' Claude ')
                        : styles.dim().render(' Claude ');
                    var outputTabLabel = s.splitViewTab === 'output'
                        ? styles.primaryButton().render(' Output ')
                        : styles.dim().render(' Output ');
                    var outputCount = (s.outputLines && s.outputLines.length > 0)
                        ? styles.dim().render('(' + s.outputLines.length + ')') : '';
                    var tabBar = zone.mark('split-tab-claude', claudeTabLabel) + ' ' +
                        zone.mark('split-tab-output', outputTabLabel) +
                        (outputCount ? ' ' + outputCount : '');
                    var focusLabel = s.splitViewFocus === 'wizard'
                        ? '\u25b2 Wizard'
                        : (s.splitViewTab === 'output' ? '\u25bc Output' : '\u25bc Claude');
                    var splitHint = 'Ctrl+Tab: switch  Ctrl+O: tab  Ctrl+L: close';
                    // T44: labelW must include tabBar visual width + all separator decorators.
                    // Template: leftFill + '┤ ' + tabBar + ' · ' + focusLabel + ' · ' + splitHint + ' ├' + rightFill
                    // Decorators: ┤(1)+space(1) + ' · '(3) + ' · '(3) + space(1)+├(1) = 10
                    var tabBarW = lipgloss.width(tabBar);
                    var labelW = tabBarW + focusLabel.length + splitHint.length + 10;
                    var leftFill = repeatStr('\u2500', Math.max(1, Math.floor((w - labelW) / 2)));
                    var rightFill = repeatStr('\u2500', Math.max(1, Math.ceil((w - labelW) / 2)));
                    var paneDivider = styles.dim().render(
                        leftFill + '\u2524 ' + tabBar + ' \u00b7 ' + focusLabel + ' \u00b7 ' + splitHint + ' \u251c' + rightFill);

                    // T44: Bottom pane — switch between Claude and Output tab.
                    var bottomPane;
                    if (s.splitViewTab === 'output') {
                        bottomPane = renderOutputPane(s, w, claudeH);
                    } else {
                        bottomPane = renderClaudePane(s, w, claudeH);
                    }

                    screenContent = lipgloss.joinVertical(lipgloss.Left,
                        wizardPane, paneDivider, bottomPane);
                } else {
                    // Normal (non-split) viewport.
                    s.vp.setHeight(vpHeight);
                    s.vp.setContent(screenContent);

                    if (s.scrollbar) {
                        s.scrollbar.setViewportHeight(vpHeight);
                        s.scrollbar.setContentHeight(s.vp.totalLineCount());
                        s.scrollbar.setYOffset(s.vp.yOffset());
                        s.scrollbar.setChars('\u2588', '\u2591');
                        s.scrollbar.setThumbForeground(resolveColor(COLORS.primary));
                        s.scrollbar.setTrackForeground(resolveColor(COLORS.border));
                    }

                    var vpView = s.vp.view();
                    var sbView = s.scrollbar ? s.scrollbar.view() : '';
                    screenContent = lipgloss.joinHorizontal(lipgloss.Top, vpView, sbView);
                }
            }

            // Compose.
            var fullView = lipgloss.joinVertical(lipgloss.Left,
                titleBar,
                divider,
                screenContent,
                divider,
                navBar,
                statusBar
            );

            // Overlay: Help.
            if (s.showHelp) {
                var helpPanel = viewHelpOverlay(s);
                fullView = lipgloss.place(w, h,
                    lipgloss.Center, lipgloss.Center,
                    helpPanel,
                    {whitespaceChars: '\u2591', whitespaceForeground: COLORS.border});
            }

            // Overlay: Confirm Cancel.
            if (s.showConfirmCancel) {
                var confirmPanel = viewConfirmCancelOverlay(s);
                fullView = lipgloss.place(w, h,
                    lipgloss.Center, lipgloss.Center,
                    confirmPanel,
                    {whitespaceChars: '\u2591', whitespaceForeground: COLORS.border});
            }

            // Overlay: Report.
            if (s.showingReport) {
                var reportPanel = viewReportOverlay(s);
                fullView = lipgloss.place(w, h,
                    lipgloss.Center, lipgloss.Center,
                    reportPanel,
                    {whitespaceChars: '\u2591', whitespaceForeground: COLORS.border});
            }

            // Overlay: Editor Dialog (move/rename/merge).
            if (s.activeEditorDialog) {
                var dialogPanel = viewEditorDialog(s);
                if (dialogPanel) {
                    fullView = lipgloss.place(w, h,
                        lipgloss.Center, lipgloss.Center,
                        dialogPanel,
                        {whitespaceChars: '\u2591', whitespaceForeground: COLORS.border});
                }
            }

            // Overlay: Claude Conversation (T16).
            if (s.claudeConvo.active) {
                var convoPanel = viewClaudeConvoOverlay(s);
                fullView = lipgloss.place(w, h,
                    lipgloss.Center, lipgloss.Center,
                    convoPanel,
                    {whitespaceChars: '\u2591', whitespaceForeground: COLORS.border});
            }

            return zone.scan(fullView);
        };

        var model = tea.newModel({
            init: _initFn,

            update: _updateFn,

            view: _viewFn
        });

        // Export lifecycle functions for unit testing.
        prSplit._wizardInit = _initFn;
        prSplit._wizardUpdate = _updateFn;
        prSplit._wizardView = _viewFn;
        prSplit._getFocusElements = getFocusElements;

        return model;
    }

    // -----------------------------------------------------------------------
    //  Update Handlers — screen-specific input handling
    // -----------------------------------------------------------------------

    function updateConfirmCancel(msg, s) {
        // Helper to clean up any active verify session before quitting.
        function cleanupActiveSession() {
            if (s.activeVerifySession) {
                try { s.activeVerifySession.close(); } catch (e) { /* best effort */ }
            }
            if (s.activeVerifyWorktree && s.activeVerifyDir) {
                try { prSplit.cleanupVerifyWorktree(s.activeVerifyDir, s.activeVerifyWorktree); } catch (e) { /* best effort */ }
            }
            s.activeVerifySession = null;
            s.activeVerifyWorktree = null;
            s.activeVerifyBranch = null;
            s.activeVerifyDir = null;
            s.activeVerifyStartTime = 0;
            s.verifyViewportOffset = 0;
            s.verifyAutoScroll = true;
            s.lastVerifyInterruptTime = 0;
        }

        if (msg.type === 'Key') {
            var k = msg.key;
            if (k === 'y' || k === 'enter') {
                s.showConfirmCancel = false;
                s.isProcessing = false;
                cleanupActiveSession();
                s.wizard.cancel();
                s.wizardState = 'CANCELLED';
                return [s, tea.quit()];
            }
            if (k === 'n' || k === 'esc') {
                s.showConfirmCancel = false;
                return [s, null];
            }
        }
        if (msg.type === 'Mouse' && msg.action === 'press' && !msg.isWheel) {
            if (zone.inBounds('confirm-yes', msg)) {
                s.showConfirmCancel = false;
                s.isProcessing = false;
                cleanupActiveSession();
                s.wizard.cancel();
                s.wizardState = 'CANCELLED';
                return [s, tea.quit()];
            }
            if (zone.inBounds('confirm-no', msg)) {
                s.showConfirmCancel = false;
                return [s, null];
            }
        }
        return [s, null];
    }

    // -----------------------------------------------------------------------
    //  Editor Dialog Handler — move-file, rename-split, merge-splits
    // -----------------------------------------------------------------------

    function updateEditorDialog(msg, s) {
        var dialog = s.activeEditorDialog;
        var ds = s.editorDialogState || {};

        // Close any dialog on Esc.
        if (msg.type === 'Key' && msg.key === 'esc') {
            s.activeEditorDialog = null;
            s.editorDialogState = {};
            return [s, null];
        }

        // --- Move File Dialog ---
        if (dialog === 'move') {
            return updateMoveDialog(msg, s, ds);
        }

        // --- Rename Split Dialog ---
        if (dialog === 'rename') {
            return updateRenameDialog(msg, s, ds);
        }

        // --- Merge Splits Dialog ---
        if (dialog === 'merge') {
            return updateMergeDialog(msg, s, ds);
        }

        return [s, null];
    }

    function updateMoveDialog(msg, s, ds) {
        if (!st.planCache || !st.planCache.splits) {
            s.activeEditorDialog = null;
            return [s, null];
        }
        var splits = st.planCache.splits;
        var currentIdx = s.selectedSplitIdx || 0;

        // Build list of valid target indices (all except current).
        var targets = [];
        for (var i = 0; i < splits.length; i++) {
            if (i !== currentIdx) targets.push(i);
        }
        if (targets.length === 0) {
            s.activeEditorDialog = null;
            return [s, null];
        }

        if (msg.type === 'Key') {
            var k = msg.key;
            // Navigate targets.
            if (k === 'j' || k === 'down') {
                ds.targetIdx = Math.min((ds.targetIdx || 0) + 1, targets.length - 1);
                s.editorDialogState = ds;
                return [s, null];
            }
            if (k === 'k' || k === 'up') {
                ds.targetIdx = Math.max((ds.targetIdx || 0) - 1, 0);
                s.editorDialogState = ds;
                return [s, null];
            }
            // Confirm move.
            if (k === 'enter') {
                var targetSplitIdx = targets[ds.targetIdx || 0];
                var fileIdx = s.selectedFileIdx || 0;
                var srcSplit = splits[currentIdx];
                var dstSplit = splits[targetSplitIdx];
                if (srcSplit && srcSplit.files && srcSplit.files[fileIdx] && dstSplit) {
                    var file = srcSplit.files[fileIdx];
                    // Remove from source.
                    srcSplit.files.splice(fileIdx, 1);
                    // Add to destination.
                    if (!dstSplit.files) dstSplit.files = [];
                    dstSplit.files.push(file);
                    // Adjust selectedFileIdx if it's now out of range.
                    if (s.selectedFileIdx >= srcSplit.files.length) {
                        s.selectedFileIdx = Math.max(0, srcSplit.files.length - 1);
                    }
                    log.printf('Moved %s from %s to %s', file, srcSplit.name, dstSplit.name);
                }
                s.activeEditorDialog = null;
                s.editorDialogState = {};
                return [s, null];
            }
        }

        // Mouse: click on target zone marks.
        if (msg.type === 'Mouse' && msg.action === 'press' && !msg.isWheel) {
            for (var ti = 0; ti < targets.length; ti++) {
                if (zone.inBounds('move-target-' + ti, msg)) {
                    ds.targetIdx = ti;
                    s.editorDialogState = ds;
                    // Double-click or explicit confirm — for now just select.
                    return [s, null];
                }
            }
            if (zone.inBounds('move-confirm', msg)) {
                // Reuse enter logic by synthesizing a key.
                return updateMoveDialog({type: 'Key', key: 'enter'}, s, ds);
            }
            if (zone.inBounds('move-cancel', msg)) {
                s.activeEditorDialog = null;
                s.editorDialogState = {};
                return [s, null];
            }
        }

        return [s, null];
    }

    function updateRenameDialog(msg, s, ds) {
        if (msg.type === 'Key') {
            var k = msg.key;
            // Confirm rename.
            if (k === 'enter') {
                var text = (ds.inputText || '').trim();
                if (text.length > 0 && st.planCache && st.planCache.splits) {
                    var splitIdx = s.selectedSplitIdx || 0;
                    if (st.planCache.splits[splitIdx]) {
                        var oldName = st.planCache.splits[splitIdx].name;
                        st.planCache.splits[splitIdx].name = text;
                        log.printf('Renamed split %s to %s', oldName, text);
                    }
                }
                s.activeEditorDialog = null;
                s.editorDialogState = {};
                return [s, null];
            }
            // Backspace — delete last char.
            if (k === 'backspace') {
                var t = ds.inputText || '';
                if (t.length > 0) {
                    ds.inputText = t.substring(0, t.length - 1);
                    s.editorDialogState = ds;
                }
                return [s, null];
            }
            // Typed character — single printable char (length 1).
            if (k.length === 1) {
                ds.inputText = (ds.inputText || '') + k;
                s.editorDialogState = ds;
                return [s, null];
            }
        }

        // Mouse zone clicks.
        if (msg.type === 'Mouse' && msg.action === 'press' && !msg.isWheel) {
            if (zone.inBounds('rename-confirm', msg)) {
                return updateRenameDialog({type: 'Key', key: 'enter'}, s, ds);
            }
            if (zone.inBounds('rename-cancel', msg)) {
                s.activeEditorDialog = null;
                s.editorDialogState = {};
                return [s, null];
            }
        }

        return [s, null];
    }

    function updateMergeDialog(msg, s, ds) {
        if (!st.planCache || !st.planCache.splits) {
            s.activeEditorDialog = null;
            return [s, null];
        }
        var splits = st.planCache.splits;
        var currentIdx = s.selectedSplitIdx || 0;

        // Build list of mergeable indices (all except current).
        var mergeables = [];
        for (var i = 0; i < splits.length; i++) {
            if (i !== currentIdx) mergeables.push(i);
        }
        if (mergeables.length === 0) {
            s.activeEditorDialog = null;
            return [s, null];
        }

        if (msg.type === 'Key') {
            var k = msg.key;
            // Navigate.
            if (k === 'j' || k === 'down') {
                ds.cursorIdx = Math.min((ds.cursorIdx || 0) + 1, mergeables.length - 1);
                s.editorDialogState = ds;
                return [s, null];
            }
            if (k === 'k' || k === 'up') {
                ds.cursorIdx = Math.max((ds.cursorIdx || 0) - 1, 0);
                s.editorDialogState = ds;
                return [s, null];
            }
            // Toggle selection.
            if (k === ' ') {
                var idx = mergeables[ds.cursorIdx || 0];
                if (!ds.selected) ds.selected = {};
                ds.selected[idx] = !ds.selected[idx];
                s.editorDialogState = ds;
                return [s, null];
            }
            // Confirm merge.
            if (k === 'enter') {
                var selected = ds.selected || {};
                var dstSplit = splits[currentIdx];
                if (!dstSplit) {
                    s.activeEditorDialog = null;
                    s.editorDialogState = {};
                    return [s, null];
                }
                // Collect indices to merge, sorted in descending order
                // so splicing doesn't shift indices of later items.
                var toMerge = [];
                for (var si = 0; si < splits.length; si++) {
                    if (selected[si]) toMerge.push(si);
                }
                toMerge.sort(function(a, b) { return b - a; });

                // Move files from each selected split into destination.
                for (var mi = 0; mi < toMerge.length; mi++) {
                    var srcIdx = toMerge[mi];
                    var srcSplit = splits[srcIdx];
                    if (srcSplit && srcSplit.files) {
                        if (!dstSplit.files) dstSplit.files = [];
                        for (var fi = 0; fi < srcSplit.files.length; fi++) {
                            dstSplit.files.push(srcSplit.files[fi]);
                        }
                    }
                    // Remove the merged split.
                    splits.splice(srcIdx, 1);
                    log.printf('Merged split %s into %s',
                        srcSplit ? srcSplit.name : srcIdx, dstSplit.name);
                }

                // Adjust selectedSplitIdx if it shifted.
                // Recalculate: find the current split by reference.
                for (var ni = 0; ni < splits.length; ni++) {
                    if (splits[ni] === dstSplit) {
                        s.selectedSplitIdx = ni;
                        break;
                    }
                }

                s.activeEditorDialog = null;
                s.editorDialogState = {};
                return [s, null];
            }
        }

        // Mouse: toggle checkboxes, confirm/cancel.
        if (msg.type === 'Mouse' && msg.action === 'press' && !msg.isWheel) {
            for (var ci = 0; ci < mergeables.length; ci++) {
                if (zone.inBounds('merge-item-' + ci, msg)) {
                    var idx = mergeables[ci];
                    if (!ds.selected) ds.selected = {};
                    ds.selected[idx] = !ds.selected[idx];
                    s.editorDialogState = ds;
                    return [s, null];
                }
            }
            if (zone.inBounds('merge-confirm', msg)) {
                return updateMergeDialog({type: 'Key', key: 'enter'}, s, ds);
            }
            if (zone.inBounds('merge-cancel', msg)) {
                s.activeEditorDialog = null;
                s.editorDialogState = {};
                return [s, null];
            }
        }

        return [s, null];
    }

    // Dispatch to the correct dialog view function.
    function viewEditorDialog(s) {
        switch (s.activeEditorDialog) {
            case 'move':   return viewMoveFileDialog(s);
            case 'rename': return viewRenameSplitDialog(s);
            case 'merge':  return viewMergeSplitsDialog(s);
            default:       return null;
        }
    }

    function handleMouseClick(msg, s) {
        // Navigation bar clicks.
        if (zone.inBounds('nav-back', msg)) {
            return handleBack(s);
        }
        if (zone.inBounds('nav-cancel', msg)) {
            // During active verification, clicking cancel sends SIGINT
            // (consistent with Ctrl+C) instead of opening the cancel dialog
            // to prevent session/worktree leaks from unguarded quit.
            if (s.activeVerifySession) {
                var now = Date.now();
                if (s.lastVerifyInterruptTime > 0 && (now - s.lastVerifyInterruptTime) < 2000) {
                    try { s.activeVerifySession.kill(); } catch (e) { /* ignore */ }
                } else {
                    try { s.activeVerifySession.interrupt(); } catch (e) { /* ignore */ }
                }
                s.lastVerifyInterruptTime = now;
                return [s, null];
            }
            s.showConfirmCancel = true;
            return [s, null];
        }
        if (zone.inBounds('nav-next', msg)) {
            return handleNext(s);
        }
        // Claude status badge — T45: click to re-open split-view if closed.
        if (zone.inBounds('claude-status', msg)) {
            if (typeof tuiMux !== 'undefined' && tuiMux &&
                (typeof tuiMux.hasChild !== 'function' || tuiMux.hasChild())) {
                // T45: If split-view is not open, open it (re-clears manual dismiss).
                if (!s.splitViewEnabled) {
                    s.splitViewEnabled = true;
                    s.splitViewFocus = 'wizard';
                    s.splitViewTab = 'claude';
                    s.claudeManuallyDismissed = false;
                    return [s, tea.tick(100, 'claude-screenshot')];
                }
                // Already open — switch to Claude tab.
                s.splitViewTab = 'claude';
                s.splitViewFocus = 'claude';
            }
            return [s, null];
        }
        // T44: Split-view tab bar clicks.
        if (s.splitViewEnabled) {
            if (zone.inBounds('split-tab-claude', msg)) {
                s.splitViewTab = 'claude';
                return [s, null];
            }
            if (zone.inBounds('split-tab-output', msg)) {
                s.splitViewTab = 'output';
                return [s, null];
            }
        }
        // T46: Claude question prompt click — activate input.
        if (s.claudeQuestionDetected && zone.inBounds('claude-question-input', msg)) {
            s.claudeQuestionInputActive = true;
            return [s, null];
        }
        // Screen-specific zone clicks.
        return handleScreenMouseClick(msg, s);
    }

    function handleScreenMouseClick(msg, s) {
        // Config screen: strategy selection, advanced toggle, Test Connection.
        if (s.wizardState === 'CONFIG' || s.wizardState === 'IDLE') {
            var strategies = ['auto', 'heuristic', 'directory'];
            for (var si = 0; si < strategies.length; si++) {
                if (zone.inBounds('strategy-' + strategies[si], msg)) {
                    prSplit.runtime.mode = strategies[si];
                    s.userHasSelectedStrategy = true; // T42: manual selection overrides auto-detect
                    // Trigger Claude check for 'auto'.
                    if (strategies[si] === 'auto') {
                        s.claudeCheckStatus = 'checking';
                        return [s, tea.tick(1, 'check-claude')];
                    }
                    // Clear check status when switching away.
                    s.claudeCheckStatus = null;
                    s.claudeResolvedInfo = null;
                    s.claudeCheckError = null;
                    s.claudeCheckRunning = false;
                    s.claudeCheckProgressMsg = '';
                    return [s, null];
                }
            }
            // Test Connection button.
            if (zone.inBounds('test-claude', msg)) {
                if (s.claudeCheckStatus === 'checking') return [s, null];
                s.claudeCheckStatus = 'checking';
                prSplit.runtime.mode = 'auto';
                s.userHasSelectedStrategy = true; // T42: manual action overrides auto-detect
                return [s, tea.tick(1, 'check-claude')];
            }
            if (zone.inBounds('toggle-advanced', msg)) {
                s.showAdvanced = !s.showAdvanced;
                return [s, null];
            }
        }

        // Execution: expand/collapse verification output + interrupt.
        if (s.wizardState === 'BRANCH_BUILDING' && st.planCache && st.planCache.splits) {
            // Interrupt active verify session via stop button.
            // Same double-click pattern as Ctrl+C: first click sends
            // SIGINT, second click within 2s sends SIGKILL.
            if (s.activeVerifySession && zone.inBounds('verify-interrupt', msg)) {
                var now = Date.now();
                if (s.lastVerifyInterruptTime > 0 && (now - s.lastVerifyInterruptTime) < 2000) {
                    try { s.activeVerifySession.kill(); } catch (e) { /* ignore */ }
                } else {
                    try { s.activeVerifySession.interrupt(); } catch (e) { /* ignore */ }
                }
                s.lastVerifyInterruptTime = now;
                return [s, null];
            }
            for (var vi = 0; vi < st.planCache.splits.length; vi++) {
                var vbranch = st.planCache.splits[vi].name;
                if (zone.inBounds('verify-expand-' + vbranch, msg)) {
                    s.expandedVerifyBranch = vbranch;
                    return [s, null];
                }
                // Only collapse if the clicked branch IS the currently expanded one.
                if (vbranch === s.expandedVerifyBranch && zone.inBounds('verify-collapse-' + vbranch, msg)) {
                    s.expandedVerifyBranch = null;
                    return [s, null];
                }
            }
        }

        // Plan review: split card selection + edit button.
        if (s.wizardState === 'PLAN_REVIEW') {
            if (st.planCache && st.planCache.splits) {
                for (var i = 0; i < st.planCache.splits.length; i++) {
                    if (zone.inBounds('split-card-' + i, msg)) {
                        s.selectedSplitIdx = i;
                        return [s, null];
                    }
                }
            }
            if (zone.inBounds('plan-edit', msg) && !s.isProcessing) {
                return enterPlanEditor(s);
            }
            if (zone.inBounds('plan-regenerate', msg) && !s.isProcessing) {
                handlePlanReviewState(s.wizard, 'regenerate');
                s.wizardState = 'CONFIG';
                s.wizard.reset();
                s.wizard.transition('CONFIG');
                return [s, null];
            }
            // Ask Claude: open conversation overlay for plan review feedback.
            if (zone.inBounds('ask-claude', msg) && !s.isProcessing) {
                return openClaudeConvo(s, 'plan-review');
            }
        }

        // Plan editor: split selection, file selection, action buttons.
        if (s.wizardState === 'PLAN_EDITOR') {
            if (st.planCache && st.planCache.splits) {
                for (var i = 0; i < st.planCache.splits.length; i++) {
                    if (zone.inBounds('edit-split-' + i, msg)) {
                        // Cancel inline title edit if changing to a different split.
                        if (s.editorTitleEditing && i !== s.editorTitleEditingIdx) {
                            s.editorTitleEditing = false;
                            s.editorTitleEditingIdx = -1;
                            s.editorTitleText = '';
                        }
                        s.selectedSplitIdx = i;
                        s.selectedFileIdx = 0;
                        return [s, null];
                    }
                    // File selection within currently selected split.
                    if (i === (s.selectedSplitIdx || 0) && st.planCache.splits[i].files) {
                        for (var fi = 0; fi < st.planCache.splits[i].files.length; fi++) {
                            if (zone.inBounds('edit-file-' + i + '-' + fi, msg)) {
                                s.selectedFileIdx = fi;
                                return [s, null];
                            }
                        }
                    }
                }
            }
            // Editor action buttons.
            if (zone.inBounds('editor-move', msg)) {
                var splitIdx = s.selectedSplitIdx || 0;
                var fileIdx = s.selectedFileIdx || 0;
                if (st.planCache && st.planCache.splits &&
                    st.planCache.splits[splitIdx] &&
                    st.planCache.splits[splitIdx].files &&
                    st.planCache.splits[splitIdx].files[fileIdx] &&
                    st.planCache.splits.length > 1) {
                    s.activeEditorDialog = 'move';
                    s.editorDialogState = { targetIdx: 0 };
                }
                return [s, null];
            }
            if (zone.inBounds('editor-rename', msg)) {
                var splitIdx = s.selectedSplitIdx || 0;
                if (st.planCache && st.planCache.splits &&
                    st.planCache.splits[splitIdx]) {
                    s.activeEditorDialog = 'rename';
                    s.editorDialogState = {
                        inputText: st.planCache.splits[splitIdx].name || ''
                    };
                }
                return [s, null];
            }
            if (zone.inBounds('editor-merge', msg)) {
                if (st.planCache && st.planCache.splits &&
                    st.planCache.splits.length > 1) {
                    s.activeEditorDialog = 'merge';
                    s.editorDialogState = { selected: {}, cursorIdx: 0 };
                }
                return [s, null];
            }
        }

        // Error resolution: resolution choice buttons.
        if (s.wizardState === 'ERROR_RESOLUTION') {
            // Crash-specific recovery buttons.
            if (s.claudeCrashDetected) {
                if (zone.inBounds('resolve-restart-claude', msg)) {
                    return handleErrorResolutionChoice(s, 'restart-claude');
                }
                if (zone.inBounds('resolve-fallback-heuristic', msg)) {
                    return handleErrorResolutionChoice(s, 'fallback-heuristic');
                }
                if (zone.inBounds('resolve-abort', msg)) {
                    return handleErrorResolutionChoice(s, 'abort');
                }
            }
            // Standard resolution buttons.
            var resolveChoices = ['auto', 'manual', 'skip', 'retry', 'abort'];
            for (var ri = 0; ri < resolveChoices.length; ri++) {
                if (zone.inBounds('resolve-' + resolveChoices[ri], msg)) {
                    return handleErrorResolutionChoice(s, resolveChoices[ri] === 'auto' ? 'auto-resolve' : resolveChoices[ri]);
                }
            }
            // Ask Claude about error resolution.
            if (zone.inBounds('error-ask-claude', msg)) {
                return openClaudeConvo(s, 'error-resolution');
            }
        }

        // Finalization: action buttons.
        if (s.wizardState === 'FINALIZATION') {
            if (zone.inBounds('final-report', msg)) {
                s.reportContent = formatReportForDisplay(buildReport());
                if (s.reportVp) {
                    s.reportVp.setContent(s.reportContent);
                    s.reportVp.gotoTop();
                }
                s.showingReport = true;
                return [s, null];
            }
            if (zone.inBounds('final-create-prs', msg)) {
                handleFinalizationState(s.wizard, 'create-prs');
                return [s, null];
            }
            if (zone.inBounds('final-done', msg)) {
                handleFinalizationState(s.wizard, 'done');
                s.wizardState = 'DONE';
                return [s, tea.quit()];
            }
        }

        return [s, null];
    }

    // -----------------------------------------------------------------------
    //  Report Formatting — Human-readable display for the report overlay
    // -----------------------------------------------------------------------

    function formatReportForDisplay(report) {
        if (!report) {
            return 'Report generation failed: no data available.\n\nPress Esc to close.';
        }

        var lines = [];

        lines.push('PR Split Report');
        lines.push('═══════════════════════════════════════');
        lines.push('');

        // Metadata.
        lines.push('Version:    ' + (report.version || 'unknown'));
        lines.push('Base:       ' + (report.baseBranch || '—'));
        lines.push('Strategy:   ' + (report.strategy || '—'));
        lines.push('Dry Run:    ' + (report.dryRun ? 'yes' : 'no'));
        lines.push('');

        // Analysis.
        if (report.analysis) {
            lines.push('Analysis');
            lines.push('───────────────────────────────────────');
            lines.push('  Source Branch:  ' + (report.analysis.currentBranch || '—'));
            lines.push('  Base Branch:    ' + (report.analysis.baseBranch || '—'));
            lines.push('  File Count:     ' + (report.analysis.fileCount || 0));
            if (report.analysis.files && report.analysis.files.length > 0) {
                lines.push('');
                for (var fi = 0; fi < report.analysis.files.length; fi++) {
                    var f = report.analysis.files[fi];
                    if (!f) continue;
                    var status = '';
                    if (report.analysis.fileStatuses && report.analysis.fileStatuses[f]) {
                        status = ' (' + report.analysis.fileStatuses[f] + ')';
                    }
                    lines.push('    ' + f + status);
                }
            }
            lines.push('');
        }

        // Groups.
        if (report.groups && report.groups.length > 0) {
            lines.push('Groups');
            lines.push('───────────────────────────────────────');
            for (var gi = 0; gi < report.groups.length; gi++) {
                var g = report.groups[gi];
                if (!g) continue;
                lines.push('  ' + (g.name || '(unnamed)') + ' (' + (g.files ? g.files.length : 0) + ' files)');
                if (g.files) {
                    for (var gfi = 0; gfi < g.files.length; gfi++) {
                        if (g.files[gfi]) lines.push('    ' + g.files[gfi]);
                    }
                }
            }
            lines.push('');
        }

        // Plan.
        if (report.plan) {
            lines.push('Split Plan (' + (report.plan.splitCount || 0) + ' splits)');
            lines.push('───────────────────────────────────────');
            if (report.plan.splits) {
                for (var pi = 0; pi < report.plan.splits.length; pi++) {
                    var sp = report.plan.splits[pi];
                    if (!sp) continue;
                    lines.push('  ' + (pi + 1) + '. ' + (sp.name || '(unnamed)'));
                    lines.push('     Message:  ' + (sp.message || '—'));
                    lines.push('     Files:    ' + (sp.files ? sp.files.length : 0));
                    if (sp.files) {
                        for (var sfi = 0; sfi < sp.files.length; sfi++) {
                            if (sp.files[sfi]) lines.push('       ' + sp.files[sfi]);
                        }
                    }
                }
            }
            lines.push('');
        }

        // Equivalence.
        if (report.equivalence) {
            lines.push('Equivalence Check');
            lines.push('───────────────────────────────────────');
            lines.push('  Verified:     ' + (report.equivalence.verified ? 'YES ✅' : 'NO ⚠'));
            if (report.equivalence.splitTree) {
                lines.push('  Split Tree:   ' + report.equivalence.splitTree);
            }
            if (report.equivalence.sourceTree) {
                lines.push('  Source Tree:  ' + report.equivalence.sourceTree);
            }
            if (report.equivalence.error) {
                lines.push('  Error:        ' + report.equivalence.error);
            }
            lines.push('');
        }

        lines.push('═══════════════════════════════════════');
        lines.push('Press c to copy • Esc to close');

        return lines.join('\n');
    }

    // -----------------------------------------------------------------------
    //  Navigation Handlers — Back / Next / Up / Down
    // -----------------------------------------------------------------------

    // Transition into the plan editor, resetting all inline editing state.
    function enterPlanEditor(s) {
        s.wizard.transition('PLAN_EDITOR');
        s.wizardState = 'PLAN_EDITOR';
        s.editorTitleEditing = false;
        s.editorTitleEditingIdx = -1;
        s.editorTitleText = '';
        s.editorCheckedFiles = {};
        s.editorValidationErrors = [];
        s.selectedFileIdx = 0;
        return [s, null];
    }

    function handleBack(s) {
        // T39: Collapse any expanded section before back-navigation.
        // Only check expandedVerifyBranch on screens that use it (BRANCH_BUILDING).
        if (s.wizardState === 'BRANCH_BUILDING' && s.expandedVerifyBranch !== null && s.expandedVerifyBranch !== undefined) {
            s.expandedVerifyBranch = null;
            return [s, null];
        }
        // Only check showAdvanced on CONFIG (where it's visible).
        if ((s.wizardState === 'CONFIG' || s.wizardState === 'IDLE') && s.showAdvanced) {
            s.showAdvanced = false;
            return [s, null];
        }

        switch (s.wizardState) {
            case 'PLAN_REVIEW':
                s.wizardState = 'CONFIG';
                s.wizard.reset();
                s.wizard.transition('CONFIG');
                return [s, null];
            case 'PLAN_EDITOR':
                // Reset all inline editing state before leaving (T17).
                s.editorTitleEditing = false;
                s.editorTitleEditingIdx = -1;
                s.editorTitleText = '';
                s.editorCheckedFiles = {};
                s.editorValidationErrors = [];
                s.wizardState = 'PLAN_REVIEW';
                handlePlanEditorState(s.wizard, 'back', st.planCache);
                return [s, null];
            default:
                return [s, null];
        }
    }

    function handleNext(s) {
        if (s.isProcessing) return [s, null];

        switch (s.wizardState) {
            case 'IDLE':
            case 'CONFIG':
                // If mode is 'auto' (AI-assisted), dispatch the full
                // automated pipeline (Claude classification → plan → execute).
                if (prSplit.runtime.mode === 'auto') {
                    return startAutoAnalysis(s);
                }
                return startAnalysis(s);
            case 'PLAN_REVIEW':
                return startExecution(s);
            case 'PLAN_EDITOR': {
                // Validate and save edits, return to review (T17).
                var editorResult = handlePlanEditorState(s.wizard, 'done', st.planCache);
                if (editorResult && editorResult.action === 'validation_failed') {
                    s.editorValidationErrors = editorResult.validationErrors || [];
                    return [s, null];
                }
                s.editorValidationErrors = [];
                s.editorTitleEditing = false;
                s.editorTitleEditingIdx = -1;
                s.editorTitleText = '';
                s.editorCheckedFiles = {};
                s.wizardState = 'PLAN_REVIEW';
                return [s, null];
            }
            case 'ERROR_RESOLUTION':
                return handleErrorResolutionChoice(s, 'auto-resolve');
            case 'FINALIZATION':
                handleFinalizationState(s.wizard, 'done');
                s.wizardState = 'DONE';
                return [s, tea.quit()];
            default:
                return [s, null];
        }
    }

    // -----------------------------------------------------------------------
    //  Focus System — keyboard-driven focus cycling across interactive elements
    // -----------------------------------------------------------------------

    // Returns an array of {id, type} describing focusable elements for the
    // current wizard screen.  Both handlers and views reference this so the
    // mapping lives in one place.
    function getFocusElements(s) {
        switch (s.wizardState) {
            case 'IDLE':
            case 'CONFIG':
            case 'BASELINE_FAIL': {
                var elems = [
                    {id: 'strategy-auto',      type: 'strategy'},
                    {id: 'strategy-heuristic',  type: 'strategy'},
                    {id: 'strategy-directory',  type: 'strategy'}
                ];
                // Show Test Connection button when auto is selected or
                // a previous check result is visible.
                if ((prSplit.runtime.mode || 'heuristic') === 'auto' ||
                    s.claudeCheckStatus) {
                    elems.push({id: 'test-claude', type: 'button'});
                }
                // Advanced options toggle — always reachable by Tab.
                elems.push({id: 'toggle-advanced', type: 'button'});
                elems.push({id: 'nav-next', type: 'nav'});
                return elems;
            }
            case 'PLAN_REVIEW': {
                var elems = [];
                var n = (st.planCache && st.planCache.splits) ? st.planCache.splits.length : 0;
                for (var i = 0; i < n; i++) {
                    elems.push({id: 'split-card-' + i, type: 'card'});
                }
                elems.push({id: 'plan-edit',       type: 'button'});
                elems.push({id: 'plan-regenerate',  type: 'button'});
                elems.push({id: 'ask-claude',       type: 'button'});
                elems.push({id: 'nav-next',         type: 'nav'});
                return elems;
            }
            case 'PLAN_EDITOR': {
                var elems = [];
                var n = (st.planCache && st.planCache.splits) ? st.planCache.splits.length : 0;
                for (var i = 0; i < n; i++) {
                    elems.push({id: 'edit-split-' + i, type: 'card'});
                }
                elems.push({id: 'editor-move',    type: 'button'});
                elems.push({id: 'editor-rename',  type: 'button'});
                elems.push({id: 'editor-merge',   type: 'button'});
                elems.push({id: 'nav-next',       type: 'nav'});
                return elems;
            }
            case 'ERROR_RESOLUTION': {
                var elems = [];
                if (s.claudeCrashDetected) {
                    // Crash-specific recovery options.
                    elems.push({id: 'resolve-restart-claude',     type: 'button'});
                    elems.push({id: 'resolve-fallback-heuristic', type: 'button'});
                    elems.push({id: 'resolve-abort',              type: 'button'});
                } else {
                    elems = [
                        {id: 'resolve-auto',   type: 'button'},
                        {id: 'resolve-manual', type: 'button'},
                        {id: 'resolve-skip',   type: 'button'},
                        {id: 'resolve-retry',  type: 'button'},
                        {id: 'resolve-abort',  type: 'button'}
                    ];
                }
                if (st.claudeExecutor && !s.claudeCrashDetected) {
                    elems.push({id: 'error-ask-claude', type: 'button'});
                }
                elems.push({id: 'nav-next', type: 'nav'});
                return elems;
            }
            case 'FINALIZATION': {
                var elems = [
                    {id: 'final-report',     type: 'button'},
                    {id: 'final-create-prs', type: 'button'},
                    {id: 'final-done',       type: 'button'}
                ];
                elems.push({id: 'nav-next', type: 'nav'});
                return elems;
            }
            default:
                return [];
        }
    }

    // List navigation (j/k/up/down):
    //   PLAN_REVIEW → navigates splits, clamped, syncs focusIndex.
    //   PLAN_EDITOR → navigates files within the selected split (T17).
    function handleListNav(s, delta) {
        // T41: Defense-in-depth — skip navigation while inline title editing is active.
        // The title editing interceptor (lines 309-348) already catches keys before
        // they reach handleListNav, but this guard prevents corruption if any future
        // code path bypasses the interceptor.
        if (s.editorTitleEditing) {
            return [s, null];
        }
        if (s.wizardState === 'PLAN_REVIEW') {
            var splitCount = (st.planCache && st.planCache.splits)
                ? st.planCache.splits.length : 0;
            if (splitCount > 0) {
                var newIdx = (s.selectedSplitIdx || 0) + delta;
                newIdx = Math.max(0, Math.min(newIdx, splitCount - 1));
                s.selectedSplitIdx = newIdx;
                // Keep focusIndex in sync with the split card.
                s.focusIndex = newIdx;
            }
        } else if (s.wizardState === 'PLAN_EDITOR') {
            // File-level navigation within the selected split.
            var sidx = s.selectedSplitIdx || 0;
            var files = (st.planCache && st.planCache.splits && st.planCache.splits[sidx])
                ? st.planCache.splits[sidx].files : [];
            if (files && files.length > 0) {
                var newFileIdx = (s.selectedFileIdx || 0) + delta;
                newFileIdx = Math.max(0, Math.min(newFileIdx, files.length - 1));
                s.selectedFileIdx = newFileIdx;
            }
        }
        return [s, null];
    }

    // Full focus cycling (Tab/Shift+Tab): cycles through ALL focusable
    // elements including buttons and nav.
    function handleNavDown(s) {
        var elems = getFocusElements(s);
        if (elems.length > 0) {
            s.focusIndex = ((s.focusIndex || 0) + 1) % elems.length;
        }
        // Sync selectedSplitIdx when focus lands on a card.
        syncSplitSelection(s, elems);
        return [s, null];
    }

    function handleNavUp(s) {
        var elems = getFocusElements(s);
        if (elems.length > 0) {
            var idx = (s.focusIndex || 0) - 1;
            s.focusIndex = idx < 0 ? elems.length - 1 : idx;
        }
        // Sync selectedSplitIdx when focus lands on a card.
        syncSplitSelection(s, elems);
        return [s, null];
    }

    // When focus lands on a split card, keep selectedSplitIdx in sync.
    function syncSplitSelection(s, elems) {
        if (!elems || !elems.length) return;
        var focused = elems[s.focusIndex || 0];
        if (!focused) return;
        if (focused.type === 'card') {
            // Extract the card index from the id (e.g. 'split-card-2' → 2).
            var parts = focused.id.split('-');
            var idx = parseInt(parts[parts.length - 1], 10);
            if (!isNaN(idx)) {
                s.selectedSplitIdx = idx;
            }
        }
    }

    // Try to activate the currently focused element.
    // Returns [s, cmd] if activation happened, or null if Enter
    // should fall through to handleNext().
    function handleFocusActivate(s) {
        var elems = getFocusElements(s);
        if (!elems.length) return null;
        var focused = elems[s.focusIndex || 0];
        if (!focused) return null;

        // nav-next — let Enter fall through to handleNext().
        if (focused.type === 'nav') return null;

        // Strategy selection on CONFIG screen.
        if (focused.type === 'strategy') {
            var stratName = focused.id.replace('strategy-', '');
            prSplit.runtime.mode = stratName;
            s.userHasSelectedStrategy = true; // T42: manual selection overrides auto-detect
            // Trigger Claude availability check for 'auto' strategy.
            if (stratName === 'auto') {
                s.claudeCheckStatus = 'checking';
                return [s, tea.tick(1, 'check-claude')];
            }
            // Clear check status when switching away from 'auto'.
            s.claudeCheckStatus = null;
            s.claudeResolvedInfo = null;
            s.claudeCheckError = null;
            s.claudeCheckRunning = false;
            s.claudeCheckProgressMsg = '';
            return [s, null];
        }

        // Test Connection button re-triggers Claude check.
        if (focused.id === 'test-claude') {
            if (s.claudeCheckStatus === 'checking') return [s, null];
            s.claudeCheckStatus = 'checking';
            prSplit.runtime.mode = 'auto';
            s.userHasSelectedStrategy = true; // T42: manual action overrides auto-detect
            return [s, tea.tick(1, 'check-claude')];
        }

        // Split card selection.
        if (focused.type === 'card') {
            syncSplitSelection(s, elems);
            return [s, null];
        }

        // Button activation — simulate a click on the zone.
        if (focused.type === 'button') {
            // Plan review buttons.
            if (focused.id === 'plan-edit' && !s.isProcessing) {
                return enterPlanEditor(s);
            }
            if (focused.id === 'plan-regenerate' && !s.isProcessing) {
                handlePlanReviewState(s.wizard, 'regenerate');
                s.wizardState = 'CONFIG';
                s.wizard.reset();
                s.wizard.transition('CONFIG');
                return [s, null];
            }
            if (focused.id === 'ask-claude' && !s.isProcessing) {
                return openClaudeConvo(s, 'plan-review');
            }
            // Plan editor buttons — open dialogs.
            if (focused.id === 'editor-move') {
                var splitIdx = s.selectedSplitIdx || 0;
                var fileIdx = s.selectedFileIdx || 0;
                if (st.planCache && st.planCache.splits &&
                    st.planCache.splits[splitIdx] &&
                    st.planCache.splits[splitIdx].files &&
                    st.planCache.splits[splitIdx].files[fileIdx] &&
                    st.planCache.splits.length > 1) {
                    s.activeEditorDialog = 'move';
                    s.editorDialogState = { targetIdx: 0 };
                }
                return [s, null];
            }
            if (focused.id === 'editor-rename') {
                var splitIdx = s.selectedSplitIdx || 0;
                if (st.planCache && st.planCache.splits &&
                    st.planCache.splits[splitIdx]) {
                    s.activeEditorDialog = 'rename';
                    s.editorDialogState = {
                        inputText: st.planCache.splits[splitIdx].name || ''
                    };
                }
                return [s, null];
            }
            if (focused.id === 'editor-merge') {
                if (st.planCache && st.planCache.splits &&
                    st.planCache.splits.length > 1) {
                    s.activeEditorDialog = 'merge';
                    s.editorDialogState = { selected: {}, cursorIdx: 0 };
                }
                return [s, null];
            }
            // Error resolution buttons.
            if (focused.id.indexOf('resolve-') === 0) {
                var choice = focused.id.replace('resolve-', '');
                return handleErrorResolutionChoice(s,
                    choice === 'auto' ? 'auto-resolve' : choice);
            }
            // Ask Claude about error.
            if (focused.id === 'error-ask-claude') {
                return openClaudeConvo(s, 'error-resolution');
            }
            // Toggle advanced options on CONFIG screen.
            if (focused.id === 'toggle-advanced') {
                s.showAdvanced = !s.showAdvanced;
                return [s, null];
            }
            // Finalization buttons.
            if (focused.id === 'final-report') {
                s.reportContent = formatReportForDisplay(buildReport());
                if (s.reportVp) {
                    s.reportVp.setContent(s.reportContent);
                    s.reportVp.gotoTop();
                }
                s.showingReport = true;
                return [s, null];
            }
            if (focused.id === 'final-create-prs') {
                handleFinalizationState(s.wizard, 'create-prs');
                return [s, null];
            }
            if (focused.id === 'final-done') {
                handleFinalizationState(s.wizard, 'done');
                s.wizardState = 'DONE';
                return [s, tea.quit()];
            }
        }

        return null;
    }

    // -----------------------------------------------------------------------
    //  Async Pipeline Handlers — drive wizard state machine
    // -----------------------------------------------------------------------

    function startAnalysis(s) {
        s.isProcessing = true;
        s.analysisProgress = 0;
        s.configValidationError = null; // T43: clear previous validation error on retry
        s.availableBranches = [];       // T43: clear branch list on retry
        s.analysisSteps = [
            { label: 'Analyze diff', active: true, done: false },
            { label: 'Group files', active: false, done: false },
            { label: 'Generate plan', active: false, done: false },
            { label: 'Validate plan', active: false, done: false }
        ];

        // Run config state handler.
        // Pass outputFn to suppress output.print (which would corrupt
        // BubbleTea terminal). Use log.printf instead — safe in TUI context.
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

        // Transition to CONFIG if needed.
        if (s.wizard.current === 'IDLE') {
            s.wizard.transition('CONFIG');
        }

        // Launch all analysis steps as an async pipeline on the event loop.
        // The Promise resolves when all steps complete. We poll for
        // completion via tea.tick so BubbleTea can render progress, animate
        // the spinner, and let the user cancel with Ctrl+C.
        s.analysisRunning = true;
        s.analysisError = null;

        // T44: Install global output capture to pipe git command output to Output tab.
        prSplit._outputCaptureFn = function(line) {
            s.outputLines.push(line);
            // Cap output buffer at 5000 lines to prevent unbounded memory growth.
            if (s.outputLines.length > 5000) {
                s.outputLines = s.outputLines.slice(-4000);
            }
            // Auto-scroll to bottom when new output arrives.
            if (s.outputAutoScroll) {
                s.outputViewOffset = 0;
            }
        };

        runAnalysisAsync(s).then(
            function() {
                s.analysisRunning = false;
            },
            function(err) {
                s.analysisError = (err && err.message) ? err.message : String(err);
                s.analysisRunning = false;
            }
        );

        // Poll at 100ms for responsive spinner animation.
        return [s, tea.tick(100, 'analysis-poll')];
    }

    // runAnalysisAsync: Runs all 4 analysis steps as an async function.
    // Uses analyzeDiffAsync and createSplitPlanAsync (non-blocking I/O
    // via exec.spawn), and calls applyStrategy/validatePlan inline (pure
    // compute, sub-millisecond). Updates s.analysisSteps progress between
    // each step so the poll handler can render progress.
    async function runAnalysisAsync(s) {
        // ── Step 0: Analyze diff (I/O-bound: git rev-parse, merge-base, diff) ──
        s.analysisSteps[0].active = true;
        var analysisStart = Date.now();
        try {
            st.analysisCache = await prSplit.analyzeDiffAsync({ baseBranch: prSplit.runtime.baseBranch });
        } catch (e) {
            if (s.wizard.current === 'CANCELLED') return; // wizard already cancelled
            s.isProcessing = false;
            s.errorDetails = 'Analysis failed: ' + (e.message || String(e));
            try { s.wizard.transition('ERROR'); } catch (te) { /* terminal state */ }
            s.wizardState = s.wizard.current;
            return;
        }
        s.analysisSteps[0].done = true;
        s.analysisSteps[0].active = false;
        s.analysisSteps[0].elapsed = Date.now() - analysisStart;
        s.analysisProgress = 0.25;

        // Check for cancellation between steps.
        if (!s.isProcessing || s.wizard.current === 'CANCELLED') return;

        if (st.analysisCache.error) {
            s.isProcessing = false;
            s.errorDetails = st.analysisCache.error;
            try { s.wizard.transition('ERROR'); } catch (te) { /* terminal state */ }
            s.wizardState = s.wizard.current;
            return;
        }
        if (!st.analysisCache.files || st.analysisCache.files.length === 0) {
            s.isProcessing = false;
            s.errorDetails = 'No changes found between branches.';
            s.wizardState = 'CONFIG';
            return;
        }

        // ── Step 1: Group files (pure compute, non-blocking) ──
        if (!s.isProcessing || s.wizard.current === 'CANCELLED') return;
        s.analysisSteps[1].active = true;
        var groupStart = Date.now();
        st.groupsCache = prSplit.applyStrategy(
            st.analysisCache.files, prSplit.runtime.strategy);
        s.analysisSteps[1].done = true;
        s.analysisSteps[1].active = false;
        s.analysisSteps[1].elapsed = Date.now() - groupStart;
        s.analysisProgress = 0.5;

        // ── Step 2: Create plan (I/O-bound: optional git rev-parse) ──
        if (!s.isProcessing || s.wizard.current === 'CANCELLED') return;
        s.analysisSteps[2].active = true;
        var planStart = Date.now();
        st.planCache = await prSplit.createSplitPlanAsync(st.groupsCache, {
            baseBranch: prSplit.runtime.baseBranch,
            sourceBranch: st.analysisCache.currentBranch,
            branchPrefix: prSplit.runtime.branchPrefix,
            verifyCommand: prSplit.runtime.verifyCommand,
            fileStatuses: st.analysisCache.fileStatuses
        });
        s.analysisSteps[2].done = true;
        s.analysisSteps[2].active = false;
        s.analysisSteps[2].elapsed = Date.now() - planStart;
        s.analysisProgress = 0.75;

        // ── Step 3: Validate plan (pure compute, non-blocking) ──
        if (!s.isProcessing || s.wizard.current === 'CANCELLED') return;
        s.analysisSteps[3].active = true;
        var validation = prSplit.validatePlan(st.planCache);
        s.analysisSteps[3].done = true;
        s.analysisSteps[3].active = false;
        s.analysisProgress = 1.0;

        if (!validation.valid) {
            s.isProcessing = false;
            s.errorDetails = 'Plan validation failed: ' + validation.errors.join('; ');
            try { s.wizard.transition('ERROR'); } catch (te) { /* terminal state */ }
            s.wizardState = s.wizard.current;
            return;
        }

        s.isProcessing = false;

        // Transition wizard to PLAN_GENERATION then PLAN_REVIEW.
        // Guard against CANCELLED (user may have cancelled during last step).
        if (s.wizard.current === 'CANCELLED') return;
        if (s.wizard.current === 'CONFIG') {
            s.wizard.transition('PLAN_GENERATION');
        }
        s.wizard.transition('PLAN_REVIEW');
        s.wizardState = 'PLAN_REVIEW';
    }

    // handleAnalysisPoll: Called every 100ms to check if the async
    // analysis pipeline has completed. Updates spinner animation and
    // handles cancellation.
    function handleAnalysisPoll(s) {
        // If cancelled (Ctrl+C), stop polling.
        if (!s.isProcessing && !s.analysisRunning) {
            return [s, null];
        }

        // Still running — poll again for spinner animation.
        if (s.analysisRunning) {
            return [s, tea.tick(100, 'analysis-poll')];
        }

        // Pipeline completed. Check for error set by .then() rejection.
        if (s.analysisError) {
            s.isProcessing = false;
            s.errorDetails = s.analysisError;
            try { s.wizard.transition('ERROR'); } catch (e) { /* already terminal */ }
            s.wizardState = s.wizard.current;
            return [s, null];
        }

        // Success or handled inline (error/cancel paths set state directly).
        // The async function already transitioned the wizard state.
        return [s, null];
    }

    // startAutoAnalysis: Dispatches the full automated pipeline (Claude
    // classification → plan → execute → verify) as an async Promise.
    // ── Claude availability check (CONFIG screen) ────────────────────
    // Called via tea.tick('check-claude') when user selects 'auto'.
    // Creates a temporary executor, calls resolveAsync(), and caches the
    // result for the view layer. On failure, auto-falls back to
    // heuristic mode so the user isn't stuck.
    //
    // Uses Promise+poll pattern: resolveAsync runs non-blocking subprocess
    // checks (which, version), the poll handler checks completion every 50ms.
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
            return [s, tea.tick(50, 'claude-check-poll')];
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
        return [s, tea.tick(50, 'claude-check-poll')];
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
            return [s, tea.tick(50, 'claude-check-poll')];
        }

        // Completed — view will render the final status.
        return [s, null];
    }

    // ── Automated pipeline (Claude) ────────────────────────────────
    // The pipeline runs on the JS event loop independently. We poll for
    // completion via ticks so BubbleTea can render progress and the user
    // can cancel.
    function startAutoAnalysis(s) {
        // Defense-in-depth: if prSplitConfig is absent (test/offline),
        // fall back immediately rather than crashing on property access.
        if (typeof prSplitConfig === 'undefined') {
            log.printf('auto-analysis: prSplitConfig unavailable — falling back to heuristic');
            return startAnalysis(s);
        }

        s.isProcessing = true;
        s.analysisProgress = 0;
        s.configValidationError = null; // T43: clear previous validation error on retry
        s.availableBranches = [];       // T43: clear branch list on retry
        s.analysisSteps = [
            { label: 'Spawning Claude', active: true, done: false },
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

        if (s.wizard.current === 'IDLE') {
            s.wizard.transition('CONFIG');
        }

        // Initialize Claude executor if needed.
        if (!st.claudeExecutor) {
            st.claudeExecutor = new (prSplit.ClaudeCodeExecutor)(prSplitConfig);
        }

        // Verify Claude is available before launching pipeline.
        if (!st.claudeExecutor.isAvailable()) {
            // Fall back to heuristic analysis.
            log.printf('auto-analysis: Claude not available — falling back to heuristic');
            return startAnalysis(s);
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
            if (s.outputLines.length > 5000) {
                s.outputLines = s.outputLines.slice(-4000);
            }
            if (s.outputAutoScroll) {
                s.outputViewOffset = 0;
            }
        };

        prSplit.automatedSplit(autoConfig).then(
            function(result) {
                s.autoSplitResult = result;
                s.autoSplitRunning = false;
            },
            function(err) {
                s.autoSplitResult = { error: (err && err.message) ? err.message : String(err) };
                s.autoSplitRunning = false;
            }
        );

        // Poll for completion every 500ms.
        return [s, tea.tick(500, 'auto-poll')];
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
            // Claude health check: poll isAlive() every 5 seconds.
            var healthPollMs = prSplit.AUTOMATED_DEFAULTS.claudeHealthPollMs || 5000;
            var now = Date.now();
            if (!s.lastClaudeHealthCheckMs || (now - s.lastClaudeHealthCheckMs >= healthPollMs)) {
                s.lastClaudeHealthCheckMs = now;
                var executor = st.claudeExecutor;
                if (executor && executor.handle &&
                    typeof executor.handle.isAlive === 'function') {
                    if (!executor.handle.isAlive()) {
                        // Claude process died — capture diagnostic output.
                        var diagnostic = '';
                        if (typeof executor.captureDiagnostic === 'function') {
                            diagnostic = executor.captureDiagnostic();
                        }
                        log.printf('auto-split: Claude crash detected by TUI health poll');

                        // Signal the pipeline to abort on its next aliveCheck.
                        st.claudeCrashDetected = true;

                        // Transition to error resolution with crash context.
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
                        }
                        s.wizard.transition('ERROR_RESOLUTION');
                        s.wizardState = 'ERROR_RESOLUTION';
                        return [s, null];
                    }
                }
            }
            // T45: Auto-attach Claude pane when Claude spawns.
            // Trigger once: when tuiMux has a child (Claude attached by pipeline),
            // split-view is not yet enabled, user hasn't manually dismissed, and
            // terminal is tall enough.
            if (!s.claudeAutoAttached && !s.splitViewEnabled && !s.claudeManuallyDismissed &&
                s.height >= 12 &&
                typeof tuiMux !== 'undefined' && tuiMux &&
                typeof tuiMux.hasChild === 'function' && tuiMux.hasChild()) {
                s.splitViewEnabled = true;
                s.splitViewFocus = 'wizard';   // keep wizard focused
                s.splitViewTab = 'claude';     // show Claude tab
                s.claudeAutoAttached = true;
                s.claudeAutoAttachNotif = 'Claude connected \u2014 Ctrl+L to toggle, Ctrl+] for passthrough';
                s.claudeAutoAttachNotifAt = Date.now();
                log.printf('auto-split: auto-attached Claude pane (height=%d)', s.height);
                // Start screenshot polling immediately via batched tick.
                return [s, tea.batch(
                    tea.tick(100, 'claude-screenshot'),
                    tea.tick(500, 'auto-poll')
                )];
            }

            // Read progress from pipeline's telemetry state.
            var pipelineState = prSplit._state || {};
            var telemetry = pipelineState.telemetryData || {};

            // Infer progress from what caches are populated.
            if (pipelineState.planCache) {
                s.analysisSteps[0].done = true; s.analysisSteps[0].active = false;
                s.analysisSteps[1].done = true; s.analysisSteps[1].active = false;
                s.analysisSteps[2].done = true; s.analysisSteps[2].active = false;
                s.analysisSteps[3].active = true;
                s.analysisProgress = 0.75;
            } else if (pipelineState.groupsCache) {
                s.analysisSteps[0].done = true; s.analysisSteps[0].active = false;
                s.analysisSteps[1].done = true; s.analysisSteps[1].active = false;
                s.analysisSteps[2].active = true;
                s.analysisProgress = 0.5;
            } else if (pipelineState.analysisCache) {
                s.analysisSteps[0].done = true; s.analysisSteps[0].active = false;
                s.analysisSteps[1].active = true;
                s.analysisProgress = 0.25;
            }

            return [s, tea.tick(500, 'auto-poll')];
        }

        // Pipeline completed — process result.
        var result = s.autoSplitResult;
        s.isProcessing = false;

        if (result && result.error) {
            s.errorDetails = result.error;
            s.wizardState = 'ERROR';
            return [s, null];
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
        if (s.wizard.current === 'IDLE' || s.wizard.current === 'CONFIG') {
            s.wizard.transition('CONFIG');
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
            s.errorDetails = 'Automated pipeline completed without a plan.';
            s.wizardState = 'ERROR';
        }

        return [s, null];
    }

    // handleResolvePoll: Called every 500ms to check if the async
    // resolveConflicts operation has completed. Processes the result
    // and transitions to the appropriate state.
    function handleResolvePoll(s) {
        if (!s.isProcessing) {
            return [s, null];
        }

        if (s.resolveRunning) {
            return [s, tea.tick(500, 'resolve-poll')];
        }

        // Resolve completed — process result.
        var result = s.resolveResult;
        s.isProcessing = false;

        if (result && result.error) {
            s.errorDetails = result.error;
            s.wizard.transition('ERROR_RESOLUTION');
            s.wizardState = 'ERROR_RESOLUTION';
            return [s, null];
        }

        // Check if any branches still have errors after auto-resolve.
        var errors = (result && result.errors) || [];
        if (errors.length > 0) {
            // Some branches still failing — re-enter error resolution.
            s.wizard.data.failedBranches = errors;
            s.errorDetails = 'Auto-resolve fixed ' +
                ((result && result.fixed) ? result.fixed.length : 0) +
                ' branch(es), but ' + errors.length + ' still failing.';
            s.wizard.transition('ERROR_RESOLUTION');
            s.wizardState = 'ERROR_RESOLUTION';
            return [s, null];
        }

        // All resolved — continue to equivalence check.
        s.wizard.transition('EQUIV_CHECK');
        s.wizardState = 'EQUIV_CHECK';
        s.isProcessing = true;
        return startEquivCheck(s);
    }

    // handleRestartClaudePoll — polls for async Claude restart completion.
    // Called via tea.tick(500, 'restart-claude-poll') from the restart-claude
    // crash-recovery handler. Mirrors the resolve-poll pattern.
    function handleRestartClaudePoll(s) {
        if (s.claudeRestarting) {
            // Still restarting — keep polling.
            return [s, tea.tick(500, 'restart-claude-poll')];
        }

        var result = s.restartResult;
        s.restartResult = null;

        if (!result || result.error) {
            s.errorDetails = (result && result.error) || 'Claude restart failed (unknown error)';
            // Keep claudeCrashDetected=true so crash-specific UI stays.
            return [s, null];
        }

        // Successful restart — clear crash flags and resume pipeline.
        s.claudeCrashDetected = false;
        st.claudeCrashDetected = false;
        log.printf('auto-split: Claude restarted successfully, session=%s', result.sessionId || '(none)');

        // Re-attach to tuiMux if available.
        var executor = st.claudeExecutor;
        if (executor && executor.handle && typeof tuiMux !== 'undefined' && tuiMux &&
            typeof tuiMux.attach === 'function') {
            try { tuiMux.attach(executor.handle); } catch (e) { /* best effort */ }
        }

        // Reset wizard to PLAN_GENERATION so startAnalysis picks up.
        s.wizard.transition('PLAN_GENERATION');
        s.wizardState = 'PLAN_GENERATION';
        return startAnalysis(s);
    }

    function startExecution(s) {
        if (!st.planCache || !st.planCache.splits || st.planCache.splits.length === 0) {
            s.errorDetails = 'No plan to execute.';
            return [s, null];
        }

        s.isProcessing = true;
        s.executionResults = [];
        s.executingIdx = 0;
        // Reset verification state from any prior run.
        s.verificationResults = [];
        s.verifyingIdx = -1;
        s.verifyOutput = {};
        s.expandedVerifyBranch = null;
        // Reset live verification session state.
        if (s.activeVerifySession) {
            try { s.activeVerifySession.close(); } catch (e) { /* best effort */ }
        }
        if (s.activeVerifyWorktree && s.activeVerifyDir) {
            try { prSplit.cleanupVerifyWorktree(s.activeVerifyDir, s.activeVerifyWorktree); } catch (e) { /* best effort */ }
        }
        s.activeVerifySession = null;
        s.activeVerifyWorktree = null;
        s.activeVerifyBranch = null;
        s.activeVerifyDir = null;
        s.activeVerifyStartTime = 0;
        s.verifyViewportOffset = 0;
        s.verifyAutoScroll = true;
        s.lastVerifyInterruptTime = 0;

        // Transition: PLAN_REVIEW → BRANCH_BUILDING.
        if (s.wizard.current === 'PLAN_REVIEW') {
            handlePlanReviewState(s.wizard, 'approve');
        }
        s.wizardState = 'BRANCH_BUILDING';

        // Dry run check — skip execution entirely.
        if (prSplit.runtime.dryRun) {
            s.isProcessing = false;
            s.wizardState = 'FINALIZATION';
            s.wizard.transition('FINALIZATION');
            return [s, null];
        }

        // Launch async execution pipeline. executeSplitAsync uses
        // exec.spawn for non-blocking git operations. We pass a progressFn
        // that updates s.executingIdx in real-time so the poll handler can
        // render per-branch progress with spinner animation.
        s.executionRunning = true;
        s.executionError = null;
        runExecutionAsync(s).then(
            function() {
                s.executionRunning = false;
            },
            function(err) {
                s.executionError = (err && err.message) ? err.message : String(err);
                s.executionRunning = false;
            }
        );

        // Poll at 100ms for responsive spinner animation.
        return [s, tea.tick(100, 'execution-poll')];
    }

    // runExecutionAsync: Runs the split execution as an async function.
    // Uses executeSplitAsync (non-blocking I/O via exec.spawn) with a
    // progressFn that updates s.executingIdx for real-time per-branch
    // progress in the TUI. On completion, chains to per-branch verification
    // or equivalence check.
    async function runExecutionAsync(s) {
        var result;
        try {
            result = await prSplit.executeSplitAsync(st.planCache, {
                progressFn: function(msg) {
                    // Parse branch index from progress messages like
                    // "Creating branch 2/5: split/02-feature".
                    var match = msg.match(/(\d+)\/(\d+)/);
                    if (match) {
                        s.executingIdx = parseInt(match[1], 10) - 1;
                        s.executionBranchTotal = parseInt(match[2], 10);
                    }
                    s.executionProgressMsg = msg;
                    // T44: Pipe progress message to Output tab.
                    if (s.outputLines) {
                        s.outputLines.push('\u25b6 ' + msg);
                        if (s.outputAutoScroll) s.outputViewOffset = 0;
                    }
                }
            });
        } catch (e) {
            if (s.wizard.current === 'CANCELLED') return; // wizard already cancelled
            s.isProcessing = false;
            s.errorDetails = 'Execution error: ' + (e.message || String(e));
            try { s.wizard.transition('ERROR_RESOLUTION'); } catch (te) { /* terminal state */ }
            s.wizardState = s.wizard.current;
            return;
        }

        if (!s.isProcessing || s.wizard.current === 'CANCELLED') return; // cancelled

        if (result.error) {
            s.isProcessing = false;
            s.errorDetails = result.error;
            s.wizard.data.failedBranches = result.results ?
                result.results.filter(function(r) { return r.error; }) : [];
            try { s.wizard.transition('ERROR_RESOLUTION'); } catch (te) { /* terminal state */ }
            s.wizardState = s.wizard.current;
            return;
        }

        st.executionResultCache = result.results;
        s.executionResults = result.results || [];

        // Chain: If verify command configured, start per-branch verification.
        // Otherwise, start the equivalence check.
        // (These set state directly; the poll handler will detect completion.)
        s.executionNextStep = prSplit.runtime.verifyCommand ? 'verify' : 'equiv';
    }

    // handleExecutionPoll: Called every 100ms to check if the async
    // execution pipeline has completed. Updates spinner animation and
    // handles cancellation.
    function handleExecutionPoll(s) {
        // If cancelled, stop polling.
        if (!s.isProcessing && !s.executionRunning) {
            return [s, null];
        }

        // Still running — poll again for spinner animation.
        if (s.executionRunning) {
            return [s, tea.tick(100, 'execution-poll')];
        }

        // Pipeline completed. Check for error set by .then() rejection.
        if (s.executionError) {
            s.isProcessing = false;
            s.errorDetails = s.executionError;
            try { s.wizard.transition('ERROR_RESOLUTION'); } catch (te) { /* terminal state */ }
            s.wizardState = s.wizard.current;
            return [s, null];
        }

        // Check what the async function determined as the next step.
        if (s.executionNextStep === 'verify') {
            // Start per-branch verification.
            s.executionNextStep = null;
            s.verificationResults = [];
            s.verifyingIdx = 0;
            s.verifyOutput = {};
            s.expandedVerifyBranch = null;
            return [s, tea.tick(1, 'verify-branch')];
        }

        if (s.executionNextStep === 'equiv') {
            // Start equivalence check.
            s.executionNextStep = null;
            return startEquivCheck(s);
        }

        // Async function set state directly (error paths). Stop polling.
        return [s, null];
    }

    // startEquivCheck: Launches the async equivalence check pipeline.
    // Called after execution completes (directly or after per-branch verify).
    function startEquivCheck(s) {
        // Guard: only transition if not already in EQUIV_CHECK
        // (handleResolvePoll pre-transitions for skip/resolve paths).
        if (s.wizard.current !== 'EQUIV_CHECK') {
            s.wizard.transition('EQUIV_CHECK');
        }
        s.wizardState = 'EQUIV_CHECK';

        s.equivRunning = true;
        s.equivError = null;
        runEquivCheckAsync(s).then(
            function() {
                s.equivRunning = false;
            },
            function(err) {
                s.equivError = (err && err.message) ? err.message : String(err);
                s.equivRunning = false;
            }
        );

        return [s, tea.tick(100, 'equiv-poll')];
    }

    // runEquivCheckAsync: Runs equivalence check as an async function.
    // Uses verifyEquivalenceAsync (non-blocking I/O via exec.spawn).
    async function runEquivCheckAsync(s) {
        var equivResult;
        try {
            equivResult = await prSplit.verifyEquivalenceAsync(st.planCache);
        } catch (e) {
            if (s.wizard.current === 'CANCELLED') return; // wizard already cancelled
            s.isProcessing = false;
            s.errorDetails = 'Equivalence check failed: ' + (e.message || String(e));
            try { s.wizard.transition('ERROR'); } catch (te) { /* terminal state */ }
            s.wizardState = s.wizard.current;
            return;
        }

        if (!s.isProcessing || s.wizard.current === 'CANCELLED') return; // cancelled

        // Defensive: treat null/undefined result as error.
        if (!equivResult) {
            s.isProcessing = false;
            s.errorDetails = 'Equivalence check returned no result.';
            try { s.wizard.transition('ERROR'); } catch (te) { /* terminal state */ }
            s.wizardState = s.wizard.current;
            return;
        }

        // Annotate with skip information.
        var skipped = s.wizard.data && s.wizard.data.skippedBranches;
        if (skipped && skipped.length > 0) {
            equivResult.skippedBranches = skipped.map(function(b) { return b.name || b; });
            equivResult.incomplete = true;
        }
        s.wizard.data.equivalence = equivResult;
        s.equivalenceResult = equivResult;

        s.isProcessing = false;
        try { s.wizard.transition('FINALIZATION', { equivalence: equivResult }); } catch (te) { /* terminal state */ }
        s.wizardState = s.wizard.current;
    }

    // handleEquivPoll: Called every 100ms to check if the async
    // equivalence check has completed.
    function handleEquivPoll(s) {
        // If cancelled, stop polling.
        if (!s.isProcessing && !s.equivRunning) {
            return [s, null];
        }

        // Still running — poll again.
        if (s.equivRunning) {
            return [s, tea.tick(100, 'equiv-poll')];
        }

        // Pipeline completed. Check for error.
        if (s.equivError) {
            s.isProcessing = false;
            s.errorDetails = 'Equivalence check failed: ' + s.equivError;
            try { s.wizard.transition('ERROR'); } catch (e) { /* already terminal */ }
            s.wizardState = s.wizard.current;
            return [s, null];
        }

        // Success — state already transitioned by runEquivCheckAsync.
        return [s, null];
    }

    // ── Per-branch verification (tick-based stepping) ────────────
    // Verifies one branch at a time. Uses CaptureSession (PTY + VTerm)
    // for live output when available, falling back to async verifySplitAsync
    // on platforms without PTY support (Windows).
    function runVerifyBranch(s) {
        if (!s.isProcessing) return [s, null];

        var splits = st.planCache.splits;
        if (!splits || s.verifyingIdx >= splits.length) {
            // All branches verified — move to equiv check.
            return startEquivCheck(s);
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
                passed: false,
                skipped: true,
                error: skipReason,
                duration: 0
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
            rows: 24,
            cols: Math.max(80, (s.width || 80) - 8)
        });

        if (sessionResult.skipped) {
            s.verificationResults.push({
                name: branchName,
                passed: true,
                skipped: true,
                error: null,
                output: '',
                duration: 0
            });
            s.verifyingIdx++;
            return [s, tea.tick(1, 'verify-branch')];
        }

        if (sessionResult.error && !sessionResult.session) {
            // CaptureSession failed to start — use async verifySplitAsync.
            s.verifyFallbackRunning = true;
            s.verifyFallbackError = null;

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
            return [s, tea.tick(100, 'verify-fallback-poll')];
        }

        // CaptureSession started — store state for polling.
        s.activeVerifySession = sessionResult.session;
        s.activeVerifyWorktree = sessionResult.worktreeDir;
        s.activeVerifyBranch = branchName;
        s.activeVerifyDir = sessionResult.dir;
        s.activeVerifyStartTime = sessionResult.startTime;
        s.verifyViewportOffset = 0;
        s.verifyAutoScroll = true;

        // Poll every 100ms for live output updates.
        return [s, tea.tick(100, 'verify-poll')];
    }

    // ── Live verification poll ───────────────────────────────────
    // Polls the active CaptureSession for output and completion.
    // On completion, records the result and advances to next branch.
    function pollVerifySession(s) {
        if (!s.activeVerifySession) return [s, null];

        // Check timeout.
        var timeoutMs = (typeof prSplitConfig !== 'undefined' && prSplitConfig.timeoutMs)
            ? prSplitConfig.timeoutMs : 0;
        if (timeoutMs > 0 && (Date.now() - s.activeVerifyStartTime) >= timeoutMs) {
            // Timeout — kill the process.
            try { s.activeVerifySession.kill(); } catch (e) { /* ignore */ }
        }

        if (!s.activeVerifySession.isDone()) {
            // Still running — schedule next poll.
            return [s, tea.tick(100, 'verify-poll')];
        }

        // Process exited — capture result.
        var exitCode = s.activeVerifySession.exitCode();
        var output = s.activeVerifySession.output();
        var duration = Date.now() - s.activeVerifyStartTime;
        var branchName = s.activeVerifyBranch;

        // Close session and clean up worktree.
        try { s.activeVerifySession.close(); } catch (e) { /* ignore */ }
        if (s.activeVerifyWorktree && s.activeVerifyDir) {
            prSplit.cleanupVerifyWorktree(s.activeVerifyDir, s.activeVerifyWorktree);
        }

        // Store output lines for expandable display.
        var outputLines = output.split('\n').filter(function(line) { return line.length > 0; });
        s.verifyOutput[branchName] = outputLines;

        // T44: Pipe verification output to the Output tab buffer.
        if (s.outputLines) {
            s.outputLines.push('\u2500\u2500\u2500 Verify: ' + branchName + ' (exit ' + exitCode + ') \u2500\u2500\u2500');
            for (var voi = 0; voi < outputLines.length; voi++) {
                s.outputLines.push(outputLines[voi]);
            }
            if (s.outputLines.length > 5000) {
                s.outputLines = s.outputLines.slice(-4000);
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

        s.verificationResults.push({
            name: branchName,
            passed: exitCode === 0 && !isTimeout,
            skipped: false,
            error: errorMsg,
            output: output,
            duration: duration,
            preExisting: false
        });

        // Clear active session state.
        s.activeVerifySession = null;
        s.activeVerifyWorktree = null;
        s.activeVerifyBranch = null;
        s.activeVerifyDir = null;
        s.activeVerifyStartTime = 0;
        s.verifyViewportOffset = 0;
        s.verifyAutoScroll = true;
        s.lastVerifyInterruptTime = 0;

        s.verifyingIdx++;
        return [s, tea.tick(1, 'verify-branch')];
    }

    // ── Async verify fallback (when CaptureSession unavailable) ──
    // Uses verifySplitAsync for non-blocking verification. The result
    // is stored directly on s so the poll handler can consume it.
    async function runVerifyFallbackAsync(s, branchName, dir, scopedCmd, timeoutMs) {
        if (!s.isProcessing || s.wizard.current === 'CANCELLED') return;

        var outputLines = [];
        var branchStart = Date.now();
        var verifyResult = await prSplit.verifySplitAsync(branchName, {
            dir: dir,
            verifyCommand: scopedCmd,
            verifyTimeoutMs: timeoutMs,
            outputFn: function(line) { outputLines.push(line); }
        });
        var duration = Date.now() - branchStart;

        if (!s.isProcessing || s.wizard.current === 'CANCELLED') return;

        s.verifyOutput[branchName] = outputLines;
        s.verificationResults.push({
            name: branchName,
            passed: verifyResult.passed,
            skipped: verifyResult.skipped || false,
            error: verifyResult.error || null,
            output: verifyResult.output || '',
            duration: duration,
            preExisting: verifyResult.preExisting || false
        });

        s.verifyingIdx++;
    }

    // handleVerifyFallbackPoll: Called every 100ms during async fallback
    // verification. When complete, continues to the next branch.
    function handleVerifyFallbackPoll(s) {
        // Still running — keep polling.
        if (s.verifyFallbackRunning) {
            return [s, tea.tick(100, 'verify-fallback-poll')];
        }

        // Error in the .then rejection handler — record a failure result.
        if (s.verifyFallbackError) {
            var branchName = (st.planCache && st.planCache.splits &&
                s.verifyingIdx < st.planCache.splits.length)
                ? st.planCache.splits[s.verifyingIdx].name : 'unknown';
            s.verificationResults.push({
                name: branchName,
                passed: false,
                skipped: false,
                error: s.verifyFallbackError,
                output: '',
                duration: 0,
                preExisting: false
            });
            s.verifyingIdx++;
            s.verifyFallbackError = null;
        }

        // Completed — advance to next branch.
        return [s, tea.tick(1, 'verify-branch')];
    }

    // -----------------------------------------------------------------------
    //  Split-View: Key-to-Terminal-Bytes Conversion (T29)
    // -----------------------------------------------------------------------

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
        'f1': true          // help
    };

    // Convert BubbleTea key string to terminal byte sequence for PTY forwarding.
    // Returns the bytes as a string, or null if the key can't be converted.
    function keyToTermBytes(key) {
        // Named special keys → terminal escape sequences.
        switch (key) {
            case 'enter':     return '\r';
            case 'tab':       return '\t';
            case 'backspace': return '\x7f';
            case 'space':     return ' ';
            case 'escape':    return '\x1b';
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

    // Expose keyToTermBytes for testing (T29).
    prSplit._keyToTermBytes = keyToTermBytes;
    prSplit._CLAUDE_RESERVED_KEYS = CLAUDE_RESERVED_KEYS;

    // -----------------------------------------------------------------------
    //  T46: Claude Question Detection
    // -----------------------------------------------------------------------

    /**
     * detectClaudeQuestion — analyse the plain-text screenshot of Claude's
     * terminal to determine whether Claude is asking the user a question.
     *
     * Heuristics (ordered by descending confidence):
     *   1. Explicit prompt markers: lines ending with "? " or matching
     *      common confirmation patterns (y/n, Y/N, [yes/no], proceed?).
     *   2. Conversational question words at line start: "Do you", "Would
     *      you", "Should I", "Can you", "Could you", "Is this", "Are you",
     *      "Shall I", "Want me to", "May I", "Please confirm", "Please
     *      clarify".
     *   3. Plain question mark at end of a non-empty line.
     *
     * Detection only fires when Claude has been idle for ≥ idleThresholdMs
     * (we default to 2 000 ms) — this avoids false positives while Claude
     * is still streaming output.
     *
     * @param {string} plainText - Plain-text screenshot from tuiMux.screenshot()
     * @param {number} idleMs    - Milliseconds since last PTY activity
     * @returns {{ detected: boolean, line: string }}
     */
    var QUESTION_IDLE_THRESHOLD_MS = 2000;

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

        var scanCount = Math.min(15, allLines.length);
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

    // Expose for testing.
    prSplit._detectClaudeQuestion = detectClaudeQuestion;
    prSplit.QUESTION_IDLE_THRESHOLD_MS = QUESTION_IDLE_THRESHOLD_MS;

    // -----------------------------------------------------------------------
    //  Split-View: Claude Screenshot Polling
    // -----------------------------------------------------------------------
    function pollClaudeScreenshot(s) {
        // Stop polling if split view was disabled.
        if (!s.splitViewEnabled) {
            return [s, null];
        }

        // Guard: no tuiMux or no attached child — set empty screen so
        // renderClaudePane shows "No Claude session attached" placeholder.
        if (typeof tuiMux === 'undefined' || !tuiMux ||
            (typeof tuiMux.hasChild === 'function' && !tuiMux.hasChild())) {
            s.claudeScreen = '';
            s.claudeScreenshot = '';
            // T45: If Claude was auto-attached and the child disappears,
            // auto-close split-view and show notification.
            if (s.claudeAutoAttached && !s.autoSplitRunning) {
                s.splitViewEnabled = false;
                s.splitViewFocus = 'wizard';
                s.splitViewTab = 'claude';
                s.claudeAutoAttachNotif = 'Claude session ended \u2014 split-view closed';
                s.claudeAutoAttachNotifAt = Date.now();
                return [s, null]; // stop polling
            }
            // Continue polling — the child may attach later (e.g., during auto-split).
            return [s, tea.tick(500, 'claude-screenshot')];
        }

        // Capture ANSI screen from tuiMux if available (T28: full color rendering).
        try {
            if (typeof tuiMux.childScreen === 'function') {
                var screen = tuiMux.childScreen();
                if (screen !== null && screen !== undefined) {
                    s.claudeScreen = String(screen);
                }
            }
            // Also capture plain-text for fallback and test assertions.
            if (typeof tuiMux.screenshot === 'function') {
                var shot = tuiMux.screenshot();
                if (shot !== null && shot !== undefined) {
                    s.claudeScreenshot = String(shot);
                }
            }
        } catch (e) {
            // Swallow — screen capture may fail if Claude session ended.
        }

        // T46: Claude question detection — check if Claude is asking a
        // question, but only when processing (analysis/execution running)
        // and the user isn't already composing a response.
        if (s.isProcessing && !s.claudeQuestionInputActive) {
            var now46 = Date.now();
            // Throttle detection to every 2s to avoid churn.
            if (!s.claudeLastQuestionCheckMs ||
                (now46 - s.claudeLastQuestionCheckMs >= 2000)) {
                s.claudeLastQuestionCheckMs = now46;

                // Compute idle time from tuiMux.
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
                    // New question detected — surface it.
                    s.claudeQuestionDetected = true;
                    s.claudeQuestionLine = qResult.line;
                    s.claudeQuestionInputText = '';
                    s.claudeQuestionInputActive = false;
                    log.printf('T46: Claude question detected: %s', qResult.line);
                } else if (!qResult.detected && s.claudeQuestionDetected &&
                           !s.claudeQuestionInputActive) {
                    // Question resolved (Claude started streaming again) —
                    // only auto-dismiss if user isn't typing a response.
                    s.claudeQuestionDetected = false;
                    s.claudeQuestionLine = '';
                    s.claudeQuestionInputText = '';
                }
            }
        }

        // Schedule next poll at 500ms.
        return [s, tea.tick(500, 'claude-screenshot')];
    }

    // -----------------------------------------------------------------------
    //  Claude Conversation (T16): interactive back-and-forth
    // -----------------------------------------------------------------------

    /**
     * openClaudeConvo — opens the conversation overlay.
     * @param {string} context - 'plan-review' or 'error-resolution'
     */
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

    /**
     * closeClaudeConvo — dismisses the conversation overlay.
     */
    function closeClaudeConvo(s) {
        s.claudeConvo.active = false;
        s.claudeConvo.inputText = '';
        s.claudeConvo.scrollOffset = 0;
        // Keep history and context for re-opening.
        return [s, null];
    }

    /**
     * updateClaudeConvo — handles input while conversation overlay is active.
     */
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

    /**
     * buildClaudePrompt — constructs the prompt based on conversation context.
     */
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

    /**
     * submitClaudeMessage — launches the async send + wait operation.
     */
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
        var timeoutMs = 120000; // 2 minutes default
        if (convo.context === 'plan-review') {
            toolToWait = 'reportSplitPlan';
            timeoutMs = 180000; // 3 minutes for plan revision
        } else if (convo.context === 'error-resolution') {
            toolToWait = 'reportResolution';
            timeoutMs = 120000;
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
                            try { shot = String(tuiMux.screenshot() || ''); } catch (e) { /* ignore */ }
                        }
                        if (shot) {
                            convoState.history.push({
                                role: 'claude',
                                text: '[screenshot]\n' + shot.substring(shot.length - 500),
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
        return [s, tea.tick(200, 'claude-convo-poll')];
    }

    /**
     * formatClaudeResponse — formats structured MCP tool response for display.
     */
    function formatClaudeResponse(toolName, data) {
        if (toolName === 'reportSplitPlan' && data && data.splits) {
            var parts = ['Revised plan (' + data.splits.length + ' splits):'];
            for (var i = 0; i < data.splits.length; i++) {
                var sp = data.splits[i];
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

    /**
     * processClaudeConvoResult — applies structured result to wizard state.
     */
    function processClaudeConvoResult(convo, toolName, data) {
        if (toolName === 'reportSplitPlan' && data && data.splits) {
            // Update the plan cache with the revised plan from Claude.
            if (st.planCache) {
                st.planCache.splits = data.splits;
                if (data.baseBranch) {
                    st.planCache.baseBranch = data.baseBranch;
                }
            }
        }
        // reportResolution results are handled by the existing error resolution flow.
        // The user can manually apply the suggestion or use auto-resolve.
    }

    /**
     * pollClaudeConvo — tick handler to check async send/wait progress.
     */
    function pollClaudeConvo(s) {
        var convo = s.claudeConvo;

        // If still sending, keep polling.
        if (convo.sending) {
            return [s, tea.tick(200, 'claude-convo-poll')];
        }

        // Async operation completed. UI will update on next render.
        return [s, null];
    }

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
            return [s, tea.tick(500, 'restart-claude-poll')];
        }

        if (choice === 'fallback-heuristic') {
            s.claudeCrashDetected = false;
            st.claudeCrashDetected = false;
            prSplit.runtime.mode = 'heuristic';
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
            return [s, tea.tick(500, 'resolve-poll')];

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
                (typeof tuiMux.hasChild !== 'function' || tuiMux.hasChild())) {
                tuiMux.switchTo('claude');
            }
            return [s, null];

        case 'skip':
            // Transition to EQUIV_CHECK happened in handleErrorResolutionState.
            // Dispatch equivalence check via async startEquivCheck.
            s.isProcessing = true;
            return startEquivCheck(s);

        case 'retry':
            // Transition to PLAN_GENERATION happened. Re-run analysis.
            return startAnalysis(s);

        case 'abort':
            // Transition to CANCELLED happened. Quit the wizard.
            return [s, tea.quit()];

        default:
            return [s, null];
        }
    }

    // -----------------------------------------------------------------------
    //  Program Launch (T025 + T027)
    // -----------------------------------------------------------------------

    var _wizardModel = createWizardModel();

    // Export for Go-side launching and test access.
    prSplit._wizardModel = _wizardModel;
    prSplit._createWizardModel = createWizardModel;

    // startWizard — called by pr_split.go to launch the BubbleTea wizard.
    // Blocks the calling goroutine until the user exits the wizard.
    prSplit.startWizard = function() {
        return tea.run(_wizardModel, {altScreen: true, mouse: true});
    };

    // -----------------------------------------------------------------------
    //  Mode Registration — commands remain for programmatic/test dispatch.
    //  The BubbleTea wizard above is launched by pr_split.go for interactive
    //  use. This registration exposes all commands so existing tests and
    //  the scripting API continue to work.
    // -----------------------------------------------------------------------

    ctx.run('register-mode', function() {
        var runtime = prSplit.runtime;
        tui.registerMode({
            name: prSplit._MODE_NAME,
            tui: {
                title: 'PR Split',
                prompt: '(pr-split) > '
            },
            onEnter: function() {
                output.print('PR Split Wizard active. Type "help" for commands.');
                output.print('');
                output.print('Config: base=' + runtime.baseBranch + ' strategy=' + runtime.strategy +
                    ' max=' + runtime.maxFiles + (runtime.dryRun ? ' [DRY RUN]' : ''));
            },
            commands: function() {
                return buildCommands(tuiState);
            }
        });
    });

    ctx.run('enter-pr-split', function() {
        tui.switchMode(prSplit._MODE_NAME);
    });

})(globalThis.prSplit);

// ---------------------------------------------------------------------------
//  CommonJS exports for require() compatibility (test environments).
// ---------------------------------------------------------------------------
if (typeof module !== 'undefined' && module.exports) {
    module.exports = globalThis.prSplit;
}
