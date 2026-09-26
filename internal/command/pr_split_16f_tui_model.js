'use strict';
// pr_split_16f_tui_model.js — TUI: BubbleTea model factory, mouse handling, view, program launch
// Dependencies: chunks 00-16e must be loaded first.
// Requires Go-injected globals: tui, ctx, output, log, prSplitConfig, tuiMux.

(function(prSplit) {

    if (typeof tui === 'undefined' || typeof ctx === 'undefined' ||
        typeof output === 'undefined') { return; }

    // Cross-chunk imports — libraries from chunks 15a/15d.
    var tea = prSplit._tea;
    var lipgloss = prSplit._lipgloss;
    var zone = prSplit._zone;
    var viewportLib = prSplit._viewportLib;
    var scrollbarLib = prSplit._scrollbarLib;
    var COLORS = prSplit._COLORS;
    var resolveColor = prSplit._resolveColor;
    var repeatStr = prSplit._repeatStr;
    var styles = prSplit._wizardStyles;

    // Cross-chunk imports — renderers from chunks 15b/15c/15d.
    var renderTitleBar = prSplit._renderTitleBar;
    var renderNavBar = prSplit._renderNavBar;
    var renderStatusBar = prSplit._renderStatusBar;
    var renderAgentPane = prSplit._renderAgentPane;
    var renderOutputPane = prSplit._renderOutputPane;
    var renderVerifyPane = prSplit._renderVerifyPane;
    var viewForState = prSplit._viewForState;
    var viewHelpOverlay = prSplit._viewHelpOverlay;
    var viewConfirmCancelOverlay = prSplit._viewConfirmCancelOverlay;
    var viewReportOverlay = prSplit._viewReportOverlay;
    var syncReportOverlay = prSplit._syncReportOverlay;
    var viewAgentConvoOverlay = prSplit._viewAgentConvoOverlay;

    // Cross-chunk imports — state from chunks 13-14.
    var st = prSplit._state;
    var tuiState = prSplit._tuiState;
    var WizardState = prSplit.WizardState;
    var buildReport = prSplit._buildReport;
    var buildCommands = prSplit._buildCommands;
    var handlePlanReviewState = prSplit._handlePlanReviewState;
    var handleFinalizationState = prSplit._handleFinalizationState;

    // Cross-chunk imports — from chunk 16a.
    var syncMainViewport = prSplit._syncMainViewport;
    var handleBack = prSplit._handleBack;
    var handleNext = prSplit._handleNext;
    var enterPlanEditor = prSplit._enterPlanEditor;
    var handlePauseResume = prSplit._handlePauseResume;
    var handlePauseQuit = prSplit._handlePauseQuit;
    var viewEditorDialog = prSplit._viewEditorDialog;
    var getFocusElements = prSplit._getFocusElements;
    var getInteractivePaneSession = prSplit._getInteractivePaneSession;
    // Task 8: Shell tab removed — openVerifyWorktreeShell removed entirely.

    // Cross-chunk imports — from chunks 16b/16c/16d.
    var startEquivCheck = prSplit._startEquivCheck;
    var startPRCreation = prSplit._startPRCreation;
    var formatReportForDisplay = prSplit._formatReportForDisplay;
    var handleErrorResolutionChoice = prSplit._handleErrorResolutionChoice;
    var openAgentConvo = prSplit._openAgentConvo;

    // Cross-chunk imports — from chunk 16e (mouse helpers).
    var mouseToTermBytes = prSplit._mouseToTermBytes;
    var computeSplitPaneContentOffset = prSplit._computeSplitPaneContentOffset;
    var writeMouseToPane = prSplit._writeMouseToPane;
    var C = prSplit._TUI_CONSTANTS;

    function appendTuiOutput(state, text) {
        var normalized = text == null ? '' : String(text);
        normalized = normalized.replace(/\r\n?/g, '\n');
        var lines = normalized.split('\n');
        if (lines.length > 1 && lines[lines.length - 1] === '') {
            lines.pop();
        }
        if (lines.length === 0) {
            lines.push('');
        }

        for (var i = 0; i < lines.length; i++) {
            state.outputLines.push(lines[i]);
        }

        var cap = (C && C.OUTPUT_BUFFER_CAP) || 5000;
        if (state.outputLines.length > cap) {
            state.outputLines = state.outputLines.slice(-cap);
        }
        if (state.outputAutoScroll) {
            state.outputViewOffset = 0;
        }
    }

    prSplit._tuiOutputActive = false;
    prSplit._pendingTuiOutput = [];
    prSplit._routeTuiOutput = function(text) {
        if (!prSplit._tuiOutputActive) {
            return false;
        }
        var state = prSplit._toggleModelState;
        if (!state || !state.outputLines) {
            prSplit._pendingTuiOutput.push(text == null ? '' : String(text));
            return true;
        }
        appendTuiOutput(state, text);
        if (typeof prSplit._stateRefresh === 'function') {
            prSplit._stateRefresh();
        }
        return true;
    };

    // --- Mouse Handlers ---

    function isPointOnDivider(s, msg) {
        if (!s || !s.splitViewEnabled) return false;
        var h = s.height || C.DEFAULT_ROWS;
        var chromeH = prSplit._CHROME_ESTIMATE || 8;
        var vpH = Math.max(3, h - chromeH);
        var minP = 3;
        var wH = Math.max(minP, Math.floor(vpH * (s.splitViewRatio || 0.6)));
        wH = Math.min(wH, vpH - minP - 1);
        var vpHt = (s.vp && typeof s.vp.height === 'function') ? s.vp.height() : wH;
        var divY = 2 + vpHt;
        return Math.abs(msg.y - divY) <= 1;
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
            var activeVerifySession = getInteractivePaneSession(s, 'verify');
            if (activeVerifySession && !s.verifyShellExited) {
                var now = Date.now();
                if (s.lastVerifyInterruptTime > 0 && (now - s.lastVerifyInterruptTime) < C.SIGKILL_WINDOW_MS) {
                    try {
                        var killResult = activeVerifySession.kill();
                        prSplit._trackPaneOutcome(s, killResult, null, function(e) {
                            log.debug('quit: verifySession.kill failed: ' + (e.message || e));
                        });
                    } catch (e) { log.debug('quit: verifySession.kill failed: ' + (e.message || e)); }
                } else {
                    try {
                        var interruptResult = activeVerifySession.interrupt();
                        prSplit._trackPaneOutcome(s, interruptResult, null, function(e) {
                            log.debug('quit: verifySession.interrupt failed: ' + (e.message || e));
                        });
                    } catch (e) { log.debug('quit: verifySession.interrupt failed: ' + (e.message || e)); }
                }
                s.lastVerifyInterruptTime = now;
                return [s, null];
            }
            s.showConfirmCancel = true;
            s.confirmCancelFocus = 0;  // T031: reset focus to 'Yes' on open
            return [s, null];
        }
        if (zone.inBounds('nav-next', msg)) {
            return handleNext(s);
        }
        // Agent status badge — T45: click to re-open split-view if closed.
        // Task 5: Use pinned SessionID proxy instead of raw session() check.
        if (zone.inBounds('agent-status', msg)) {
            var agentPane = getInteractivePaneSession(s, 'agent');
            if (agentPane && typeof agentPane.isRunning === 'function' &&
                agentPane.isRunning()) {
                // T45: If split-view is not open, open it (re-clears manual dismiss).
                if (!s.splitViewEnabled) {
                    s.splitViewEnabled = true;
                    s.splitViewFocus = 'wizard';
                    s.splitViewTab = 'agent';
                    s.agentManuallyDismissed = false;
                    syncMainViewport(s); // T120: sync dimensions after toggle.
                    return [s, tea.tick(C.TICK_INTERVAL_MS, 'agent-screenshot')];
                }
                // Already open — switch to Agent tab.
                s.splitViewTab = 'agent';
                s.splitViewFocus = 'agent';
            }
            return [s, null];
        }
        // T44: Split-view tab bar clicks.
        if (s.splitViewEnabled) {
            if (zone.inBounds('split-tab-agent', msg)) {
                s.splitViewTab = 'agent';
                s.splitViewFocus = 'agent';
                return [s, null];
            }
            if (zone.inBounds('split-tab-output', msg)) {
                s.splitViewTab = 'output';
                return [s, null];
            }
            if (zone.inBounds('split-tab-verify', msg)) {
                s.splitViewTab = 'verify';
                return [s, null];
            }
        }
        // Split-view divider click -> initiate drag.
        if (s.splitViewEnabled && (zone.inBounds('split-divider', msg) || isPointOnDivider(s, msg))) {
            s.dividerDragging = true;
            s.dragStartY = msg.y;
            return [s, null];
        }
        // T46: Agent question prompt click — activate input.
        if (s.agentQuestionDetected && zone.inBounds('agent-question-input', msg)) {
            s.agentQuestionInputActive = true;
            return [s, null];
        }
        // T327/T328: Forward unmatched press to focused child terminal.
        // While the live pane is active on the agent tab, unmatched presses
        // inside the rendered pane box go to the termpane first; everything
        // else keeps the legacy byte-forwarding path.
        if (s.splitViewEnabled && s.splitViewTab !== 'output') {
            var liveInside = false;
            if (s.splitViewTab === 'agent' && typeof prSplit._agentLiveActive === 'function' && prSplit._agentLiveActive()) {
                try { liveInside = prSplit._isPointInAgentPane(msg.x, msg.y); } catch (e) {}
            }
            var h = s.height || C.DEFAULT_ROWS;
            var vpH = Math.max(3, h - 8);
            var minP = 3;
            var wH = Math.max(minP, Math.floor(vpH * (s.splitViewRatio || 0.6)));
            wH = Math.min(wH, vpH - minP - 1);
            var divRow = 2 + wH;
            var insideBottom = liveInside || (msg.y > divRow && msg.y < (h - 4));
            if (insideBottom) {
                if (s.splitViewFocus !== 'agent') {
                    s.splitViewFocus = 'agent';
                }
                if (liveInside && typeof prSplit._routeMouseToAgentTermpane === 'function') {
                    var liveOk = false;
                    try {
                        liveOk = prSplit._routeMouseToAgentTermpane(msg);
                        if (liveOk && typeof prSplit._trackPaneOutcome === 'function') {
                            prSplit._trackPaneOutcome(s, liveOk, null, function(e) {
                                log.debug('agent live press dispatch failed', { error: e.message || String(e) });
                            });
                        }
                    } catch (e) {
                        log.debug('agent live press dispatch failed', { error: e.message || String(e) });
                    }
                    if (liveOk) return [s, null];
                }
                var ofs = computeSplitPaneContentOffset(s);
                var mb = mouseToTermBytes(msg, ofs.row, ofs.col);
                if (mb && writeMouseToPane(mb, s)) {
                    return [s, null];
                }
                return [s, null];
            }
        }
        // Screen-specific zone clicks.
        return handleScreenMouseClick(msg, s);
    }

    function handleScreenMouseClick(msg, s) {
        // Preserved-session ERROR actions mirror the a/f/d key dispatch.
        if (s.wizardState === 'ERROR' && prSplit._agentEvidence &&
            (s.errorDetails || '').indexOf('reportClassification') >= 0) {
            if (zone.inBounds('err-show-agent', msg)) {
                s.splitViewEnabled = true;
                s.splitViewTab = 'agent';
                return [s, null];
            }
            if (zone.inBounds('err-focus-agent', msg)) {
                s.splitViewEnabled = true;
                s.splitViewTab = 'agent';
                s.splitViewFocus = 'agent';
                return [s, null];
            }
            if (zone.inBounds('err-discard-session', msg)) {
                var paneClose = Promise.resolve();
                if (typeof prSplit._noteAgentDetached === 'function') {
                    try {
                        paneClose = Promise.resolve(prSplit._noteAgentDetached());
                    } catch (e) {
                        log.debug('error discard detach failed', { error: e.message || String(e) });
                    }
                }
                // Same deferred-quit ordering as the key-path discard:
                // detach now, cleanup on the promise chain, quit on the
                // next error-discard-quit tick after close settles.
                var stt = prSplit._state;
                s.errorDiscardPending = true;
                prSplit._agentEvidence = null;
                if (stt) stt.agentEvidence = null;
                try {
                    if (typeof prSplit.cleanupExecutor !== 'function') {
                        throw new Error('cleanupExecutor missing');
                    }
                    var finishDiscard = function() {
                        s.errorDiscardPending = false;
                        s.errorDiscardReady = true;
                    };
                    var discardCleanup = paneClose.then(function() {
                        return prSplit.cleanupExecutor();
                    }).then(function() {
                        var mcpCb = prSplit._mcpCallbackObj;
                        if (!mcpCb || typeof mcpCb.close !== 'function') return null;
                        return mcpCb.close().catch(function(e) {
                            log.debug('error discard mcp close failed', { error: e.message || String(e) });
                        }).then(function() {
                            prSplit._mcpCallbackObj = null;
                            var inner = prSplit._state;
                            if (inner) inner.mcpCallbackObj = null;
                        });
                    });
                    prSplit._trackPaneOutcome(s, discardCleanup, finishDiscard, function(e) {
                        log.debug('error discard cleanup failed', { error: e.message || String(e) });
                        finishDiscard();
                    });
                } catch (e) {
                    log.debug('error discard cleanup failed', { error: e.message || String(e) });
                    s.errorDiscardPending = false;
                    s.errorDiscardReady = true;
                }
                return [s, tea.tick(C.TICK_INTERVAL_MS, 'error-discard-quit')];
            }
        }
        // Config screen: strategy selection, advanced toggle, Test Connection.
        if (s.wizardState === 'CONFIG' || s.wizardState === 'IDLE') {
            // If a config field is being edited and the click is outside that field,
            // cancel editing (like blur on a form input).
            if (s.configFieldEditing) {
                var editZoneId = 'config-' + s.configFieldEditing;
                if (!zone.inBounds(editZoneId, msg)) {
                    s.configFieldEditing = null;
                    s.configFieldValue = '';
                }
            }
            var strategies = ['auto', 'heuristic', 'directory'];
            for (var si = 0; si < strategies.length; si++) {
                if (zone.inBounds('strategy-' + strategies[si], msg)) {
                    prSplit.runtime.mode = strategies[si];
                    s.userHasSelectedStrategy = true; // T42: manual selection overrides auto-detect
                    // Trigger Agent check for 'auto'.
                    if (strategies[si] === 'auto') {
                        s.agentCheckStatus = 'checking';
                        return [s, tea.tick(1, 'check-agent')];
                    }
                    // Clear check status when switching away.
                    s.agentCheckStatus = null;
                    s.agentResolvedInfo = null;
                    s.agentCheckError = null;
                    s.agentCheckRunning = false;
                    s.agentCheckProgressMsg = '';
                    return [s, null];
                }
            }
            // Test Connection button.
            if (zone.inBounds('test-agent', msg)) {
                if (s.agentCheckStatus === 'checking') return [s, null];
                s.agentCheckStatus = 'checking';
                prSplit.runtime.mode = 'auto';
                s.userHasSelectedStrategy = true; // T42: manual action overrides auto-detect
                return [s, tea.tick(1, 'check-agent')];
            }
            if (zone.inBounds('toggle-advanced', msg)) {
                s.showAdvanced = !s.showAdvanced;
                if (!s.showAdvanced) {
                    s.configFieldEditing = null;
                    s.configFieldValue = '';
                    // Clamp focus index to new (shorter) element list.
                    var newElems = getFocusElements(s);
                    if (s.focusIndex >= newElems.length) {
                        s.focusIndex = Math.max(0, newElems.length - 1);
                    }
                }
                return [s, null];
            }
            // Advanced option field clicks — enter inline edit mode.
            var configFields = ['config-maxFiles', 'config-branchPrefix', 'config-verifyCommand'];
            for (var cfi = 0; cfi < configFields.length; cfi++) {
                if (zone.inBounds(configFields[cfi], msg)) {
                    var fieldName = configFields[cfi].replace('config-', '');
                    var runtime = prSplit.runtime;
                    s.configFieldEditing = fieldName;
                    if (fieldName === 'maxFiles') {
                        s.configFieldValue = String(typeof runtime.maxFiles === 'number' ? runtime.maxFiles : 10);
                    } else if (fieldName === 'branchPrefix') {
                        s.configFieldValue = runtime.branchPrefix || 'split/';
                    } else if (fieldName === 'verifyCommand') {
                        s.configFieldValue = runtime.verifyCommand || 'true';
                    }
                    // Update focus to match the clicked field.
                    var elems = getFocusElements(s);
                    for (var ei = 0; ei < elems.length; ei++) {
                        if (elems[ei].id === configFields[cfi]) {
                            s.focusIndex = ei;
                            break;
                        }
                    }
                    return [s, null];
                }
            }
            // Dry run checkbox toggle.
            if (zone.inBounds('config-dryRun', msg)) {
                prSplit.runtime.dryRun = !prSplit.runtime.dryRun;
                // Update focus to match the clicked checkbox.
                var elems = getFocusElements(s);
                for (var ei = 0; ei < elems.length; ei++) {
                    if (elems[ei].id === 'config-dryRun') {
                        s.focusIndex = ei;
                        break;
                    }
                }
                return [s, null];
            }
        }

        // Execution: expand/collapse verification output + interrupt.
        if (s.wizardState === 'BRANCH_BUILDING' && st.planCache && st.planCache.splits) {
            var activeVerifySession = getInteractivePaneSession(s, 'verify');
            // T059: Pause/Resume active verify session via dedicated buttons.
            if (activeVerifySession && zone.inBounds('verify-pause', msg)) {
                if (!s.verifyPaused) {
                    try {
                        var pauseResult = activeVerifySession.pause();
                        var markPaused = function() { s.verifyPaused = true; prSplit._transitionVerifyPhase(s, prSplit._verifyPhases.PAUSED); };
                        prSplit._trackPaneOutcome(s, pauseResult, markPaused, function(e) {
                            log.printf('verify: pause failed: %s', e.message || String(e));
                        });
                    } catch (e) {
                        log.printf('verify: pause failed: %s', e.message || String(e));
                    }
                }
                return [s, null];
            }
            if (activeVerifySession && zone.inBounds('verify-resume', msg)) {
                if (s.verifyPaused) {
                    try {
                        var resumeResult = activeVerifySession.resume();
                        var markRunning = function() { s.verifyPaused = false; prSplit._transitionVerifyPhase(s, prSplit._verifyPhases.RUNNING); };
                        prSplit._trackPaneOutcome(s, resumeResult, markRunning, function(e) {
                            log.printf('verify: resume failed: %s', e.message || String(e));
                        });
                    } catch (e) {
                        log.printf('verify: resume failed: %s', e.message || String(e));
                    }
                }
                return [s, null];
            }
            // Task 8: Shell tab removed — verify pane IS the interactive shell.
            // Interrupt active verify session via stop button.
            // Same double-click pattern as Ctrl+C: first click sends
            // SIGINT, second click within 2s sends SIGKILL.
            if (activeVerifySession && !s.verifyShellExited && zone.inBounds('verify-interrupt', msg)) {
                var now = Date.now();
                if (s.lastVerifyInterruptTime > 0 && (now - s.lastVerifyInterruptTime) < C.SIGKILL_WINDOW_MS) {
                    try {
                        var killResult = activeVerifySession.kill();
                        prSplit._trackPaneOutcome(s, killResult, null, function(e) {
                            log.debug('cancelVerify: verifySession.kill failed: ' + (e.message || e));
                        });
                    } catch (e) { log.debug('cancelVerify: verifySession.kill failed: ' + (e.message || e)); }
                } else {
                    try {
                        var interruptResult = activeVerifySession.interrupt();
                        prSplit._trackPaneOutcome(s, interruptResult, null, function(e) {
                            log.debug('cancelVerify: verifySession.interrupt failed: ' + (e.message || e));
                        });
                    } catch (e) { log.debug('cancelVerify: verifySession.interrupt failed: ' + (e.message || e)); }
                }
                s.lastVerifyInterruptTime = now;
                return [s, null];
            }
            // T007 (Task 7): PASS / FAIL / CONTINUE buttons for persistent verify shell.
            // These set the user completion signal consumed by pollVerifySession.
            if (s.verifyShellExited && zone.inBounds('verify-pass', msg)) {
                if (typeof prSplit._handleVerifySignal === 'function') {
                    return prSplit._handleVerifySignal(s, 'pass');
                }
            }
            if (s.verifyShellExited && zone.inBounds('verify-fail', msg)) {
                if (typeof prSplit._handleVerifySignal === 'function') {
                    return prSplit._handleVerifySignal(s, 'fail');
                }
            }
            if (s.verifyShellExited && zone.inBounds('verify-continue', msg)) {
                if (typeof prSplit._handleVerifySignal === 'function') {
                    return prSplit._handleVerifySignal(s, 'continue');
                }
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
            // Ask Agent: open conversation overlay for plan review feedback.
            if (zone.inBounds('ask-agent', msg) && !s.isProcessing) {
                return openAgentConvo(s, 'plan-review');
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
            if (s.agentCrashDetected) {
                if (zone.inBounds('resolve-restart-agent', msg)) {
                    return handleErrorResolutionChoice(s, 'restart-agent');
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
            // Ask Agent about error resolution.
            if (zone.inBounds('error-ask-agent', msg)) {
                return openAgentConvo(s, 'error-resolution');
            }
        }

        // T061/T079: EQUIV_CHECK — Re-verify and Revise Plan buttons.
        if (s.wizardState === 'EQUIV_CHECK' && !s.isProcessing) {
            if (zone.inBounds('equiv-reverify', msg)) {
                s.isProcessing = true;
                s.equivalenceResult = null;
                // Reset verifyPhase — re-running equiv from terminal COMPLETE/FAILED.
                prSplit._resetVerifyPhase(s);
                prSplit._transitionVerifyPhase(s, prSplit._verifyPhases.EQUIV_CHECK);
                return startEquivCheck(s);
            }
            if (zone.inBounds('equiv-revise', msg)) {
                try { s.wizard.transition('PLAN_REVIEW'); } catch (te) { log.debug('error: wizard.transition failed: ' + (te.message || te)); }
                s.wizardState = s.wizard.current;
                s.isProcessing = false;
                // T308: Clean up equiv state on revise (same cleanup as handleBack).
                s.equivRunning = false;
                s.equivError = null;
                s.equivalenceResult = null;
                return [s, null];
            }
            if (zone.inBounds('nav-next', msg)) {
                try { s.wizard.transition('FINALIZATION'); } catch (te) { log.debug('wizard: transition to FINALIZATION failed: ' + (te.message || te)); }
                s.wizardState = s.wizard.current;
                return [s, null];
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
                syncReportOverlay(s);
                return [s, null];
            }
            if (zone.inBounds('final-create-prs', msg)) {
                handleFinalizationState(s.wizard, 'create-prs');
                return startPRCreation(s);
            }
            if (zone.inBounds('final-done', msg)) {
                handleFinalizationState(s.wizard, 'done');
                s.wizardState = 'DONE';
                return prSplit._quitAfterAgentTermpane(s);
            }
        }

        // T084: PAUSED screen — resume or quit.
        if (s.wizardState === 'PAUSED') {
            if (zone.inBounds('pause-resume', msg)) {
                return handlePauseResume(s);
            }
            if (zone.inBounds('pause-quit', msg)) {
                return handlePauseQuit(s);
            }
        }

        return [s, null];
    }

    // --- View Function ---

    function wizardViewImpl(s) {
        var w = s.width || 80;
        var h = s.height || C.DEFAULT_ROWS;

        // Title bar.
        var titleBar = renderTitleBar(s);

        // Divider.
        var divider = styles.divider().render(repeatStr('\u2500', w));

        // Navigation bar.
        // T120: _isNavNextFocused is now computed locally inside
        // renderNavBar — no state mutation in _viewFn.
        var navBar = renderNavBar(s);

        // Status bar.
        var statusBar = renderStatusBar(s);

        // Screen content.
        var screenContent = viewForState(s);

        // Wrap in viewport.
        if (s.vp) {
            // T120: Viewport width/height are sized by syncMainViewport()
            // in the update handler (WindowSize, split-view toggle).
            // Here we only compute layout dimensions for rendering, call
            // setContent (per-frame), and sync the scrollbar post-content.
            var chromeH = lipgloss.height(titleBar) + 2 + lipgloss.height(navBar) + lipgloss.height(statusBar);
            var vpHeight = Math.max(3, h - chromeH);

            // Determine if split-view is viable at current terminal size
            // without mutating s.splitViewEnabled (view purity).
            var splitViewViable = s.splitViewEnabled && s.wizardState !== 'ERROR';
            var wizardH = 0;
            var agentH = 0;
            if (splitViewViable) {
                // Split-view: wizard top, Agent bottom.
                // -1 for the pane divider between them.
                // Minimum split requires 3 + 1 + 3 = 7 lines.
                var minPaneH = 3;
                wizardH = Math.max(minPaneH, Math.floor(vpHeight * s.splitViewRatio));
                // Clamp wizardH so Agent pane gets at least minPaneH.
                wizardH = Math.min(wizardH, vpHeight - minPaneH - 1);
                agentH = vpHeight - wizardH - 1;

                if (wizardH < minPaneH || agentH < minPaneH) {
                    // Terminal too small for split view; render normal
                    // but don't mutate state — restored on next resize.
                    splitViewViable = false;
                }
            }
            if (splitViewViable) {

                // Wizard viewport — height already set by syncMainViewport.
                s.vp.setContent(screenContent);

                if (s.scrollbar) {
                    s.scrollbar.setViewportHeight(s.vp.height());
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
                // T44: Tab bar: [Agent] [Output] with active/inactive styling.
                var agentTabLabel = s.splitViewTab === 'agent'
                    ? styles.primaryButton().render(' Agent ')
                    : styles.dim().render(' Agent ');
                var outputTabLabel = s.splitViewTab === 'output'
                    ? styles.primaryButton().render(' Output ')
                    : styles.dim().render(' Output ');
                var outputCount = (s.outputLines && s.outputLines.length > 0)
                    ? styles.dim().render('(' + s.outputLines.length + ')') : '';
                var verifyTabLabel = '';
                // T352: Show Verify tab during both CaptureSession and fallback paths.
                if (getInteractivePaneSession(s, 'verify') || s.verifyFallbackRunning || s.verifyScreen) {
                    verifyTabLabel = s.splitViewTab === 'verify'
                        ? styles.primaryButton().render(' Verify ')
                        : styles.dim().render(' Verify ');
                }
                // Task 8: Shell tab removed from tab bar.
                var tabBar = zone.mark('split-tab-agent', agentTabLabel) + ' ' +
                    zone.mark('split-tab-output', outputTabLabel) +
                    (outputCount ? ' ' + outputCount : '') +
                    (verifyTabLabel ? ' ' + zone.mark('split-tab-verify', verifyTabLabel) : '');
                var focusLabel = s.splitViewFocus === 'wizard'
                    ? '\u25b2 Wizard'
                    : (s.splitViewTab === 'output' ? '\u25bc Output'
                       : (s.splitViewTab === 'verify' ? '\u25bc Verify' : '\u25bc Agent'));

                var metaStyled;
                var metaRaw;
                var fillChar;
                var junctionLeft;
                var junctionRight;

                if (s.dividerDragging) {
                    var tabName = s.splitViewTab === 'output' ? 'Output'
                        : (s.splitViewTab === 'verify' ? 'Verify' : 'Agent');
                    var pct = Math.round((agentH / Math.max(1, vpHeight - 1)) * 100);
                    var resizeBadge = '[ \u2195 Resizing: ' + tabName + ' ' + pct + '% (' + agentH + ' rows) ]';
                    metaRaw = ' ' + resizeBadge;
                    metaStyled = ' ' + styles.statusActive().render(resizeBadge);
                    fillChar = '\u2550';      // ═
                    junctionLeft = '\u2561 '; // ╡
                    junctionRight = ' \u255e';// ╞
                } else {
                    var splitHint = '\u2195 Drag to resize  Ctrl+Tab: cycle  Ctrl+O: tab  Ctrl+L: close';
                    if (w < 115) {
                        splitHint = '\u2195 Drag  Ctrl+Tab: cycle  Ctrl+O: tab';
                    }
                    metaRaw = ' \u00b7 ' + focusLabel + ' \u00b7 ' + splitHint;
                    metaStyled = styles.dim().render(metaRaw);
                    fillChar = '\u2500';      // ─
                    junctionLeft = '\u2524 '; // ┤
                    junctionRight = ' \u251c';// ├
                }

                var tabBarW = lipgloss.width(tabBar);
                var labelW = tabBarW + lipgloss.width(metaRaw) + 4;
                var leftLen = Math.max(1, Math.floor((w - labelW) / 2));
                var rightLen = Math.max(1, Math.ceil((w - labelW) / 2));
                var leftFill = repeatStr(fillChar, leftLen);
                var rightFill = repeatStr(fillChar, rightLen);

                var leftStyled = s.dividerDragging
                    ? styles.statusActive().render(leftFill + junctionLeft)
                    : styles.divider().render(leftFill + junctionLeft);
                var rightStyled = s.dividerDragging
                    ? styles.statusActive().render(junctionRight + rightFill)
                    : styles.divider().render(junctionRight + rightFill);

                var paneDivider = zone.mark('split-divider', leftStyled + tabBar + metaStyled + rightStyled);

                // Task 8: Bottom pane — Agent, Output, Verify tabs only.
                var bottomPane;
                if (s.splitViewTab === 'output') {
                    bottomPane = renderOutputPane(s, w, agentH);
                } else if (s.splitViewTab === 'verify') {
                    bottomPane = renderVerifyPane(s, w, agentH);
                } else if (typeof prSplit._agentLiveActive === 'function' && prSplit._agentLiveActive()) {
                    try {
                        bottomPane = renderAgentPane(s, w, agentH);
                        if (!bottomPane) bottomPane = prSplit._renderAgentLivePane(s, w, agentH);
                    } catch (e) {
                        log.debug('agent live render fallback', { error: e.message || String(e) });
                        bottomPane = renderAgentPane(s, w, agentH);
                    }
                } else {
                    bottomPane = renderAgentPane(s, w, agentH);
                }

                screenContent = lipgloss.joinVertical(lipgloss.Left,
                    wizardPane, paneDivider, bottomPane);
            } else {
                // Normal (non-split) viewport — height set by syncMainViewport.
                s.vp.setContent(screenContent);

                if (s.scrollbar) {
                    s.scrollbar.setViewportHeight(s.vp.height());
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

        // Overlay: Agent Conversation (T16).
        if (s.agentConvo.active) {
            var convoPanel = viewAgentConvoOverlay(s);
            fullView = lipgloss.place(w, h,
                lipgloss.Center, lipgloss.Center,
                convoPanel,
                {whitespaceChars: '\u2591', whitespaceForeground: COLORS.border});
        }

        return zone.scan(fullView);
    }

    // --- BubbleTea Model Factory (slimmed — delegates to chunk imports) ---

    function createWizardModel() {
        var wizard = new WizardState();
        prSplit._wizardState = wizard;

        // Track transitions to update the TUI model state.
        wizard.onTransition(function(from, to, data) {
            log.printf('wizard: %s \u2192 %s', from, to);
        });

        var vp = viewportLib.new(80, C.DEFAULT_ROWS);
        vp.setMouseWheelEnabled(true);
        var sb = scrollbarLib.new();

        // Dedicated viewport + scrollbar for the report overlay.
        var reportVp = viewportLib.new(80, 20);
        reportVp.setMouseWheelEnabled(true);
        var reportSb = scrollbarLib.new();

        // Named lifecycle functions — exported for unit testing and model startup.
        var _initStateFn = function() {
            return {
                // Wizard state.
                wizard: wizard,
                wizardState: 'IDLE',

                // Dimensions.
                width: 80,
                height: C.DEFAULT_ROWS,

                // Viewport.
                vp: vp,
                scrollbar: sb,

                // Time.
                startTime: Date.now(),

                // UI state.
                showHelp: false,
                showConfirmCancel: false,
                confirmCancelFocus: 0,  // T031: 0=Yes, 1=No
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

                // Config field inline editing state.
                configFieldEditing: null,       // field name being edited (e.g. 'maxFiles') or null
                configFieldValue: '',           // current text buffer for inline edit
                editorCheckedFiles: {},         // { 'splitIdx-fileIdx': true } for checked files
                editorValidationErrors: [],     // validation errors from save attempt
                editorFileDetailExpanded: false, // show enhanced file detail panel

                paneOperations: [],
                _verifyPaneCleanupPending: false,
                _verifyPaneCleanupPromise: null,
                _verifyPaneCleanupToken: null,
                _verifyAdvanceAfterCleanup: false,
                _verifyTimeoutKill: null,
                _verifyTimeoutKillPending: false,
                _executionStartPending: false,

                // Focus system.
                focusIndex: 0,
                _prevWizardState: null,

                // Agent availability (CONFIG screen).
                agentCheckStatus: null,   // null | 'checking' | 'available' | 'unavailable'
                agentResolvedInfo: null,  // null | { command, type }
                agentCheckError: null,    // null | error string
                agentCheckRunning: false, // true while async resolveAsync is running
                agentCheckProgressMsg: '', // progress message from resolveAsync
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

                // Verification lifecycle phase (state machine defined in
                // chunk 13). Tracks what the verification subsystem is doing
                // independently from the wizard's high-level state.
                verifyPhase: prSplit._verifyPhases.NOT_STARTED,

                // Verification state (per-branch, after branch creation).
                verificationResults: [],   // Array parallel to splits
                verifyingIdx: -1,          // -1=not started, 0..N=in progress, N=done
                verifyOutput: {},          // { branchName: [line, ...] }
                expandedVerifyBranch: null, // branchName for expandable output
                errorFromState: '',        // state to return to from ERROR
                errorSplitViewState: null, // { enabled, focus, tab } snapshot

                // Live verification session (CaptureSession).
                activeVerifySession: null,     // CaptureSession JS object (or null)
                activeVerifyWorktree: null,    // worktree path for cleanup
                activeVerifyBranch: null,      // branch name being verified
                activeVerifyDir: null,         // base dir for worktree cleanup
                activeVerifyStartTime: 0,      // start time for duration tracking
                verifyElapsedMs: 0,            // T058: elapsed ms updated each poll tick
                verifyScreen: '',              // T321: ANSI-styled VTerm screen from CaptureSession
                verifyViewportOffset: 0,       // scroll offset (lines from bottom)
                verifyAutoScroll: true,        // auto-scroll to bottom
                lastVerifyInterruptTime: 0,    // timestamp of last Ctrl+C interrupt
                verifyPaused: false,           // T059: true when verify is paused (SIGSTOP)

                // T007 (Task 7): Persistent verify shell user-signal state.
                // These are set by handleVerifySignal (p/f/c keyboard shortcuts or buttons)
                // and consumed by pollVerifySession to record the branch result.
                verifySignal: false,           // true when user pressed p/f/c
                verifySignalChoice: null,      // 'pass' | 'fail' | 'continue'
                verifySignalBranch: null,      // branchName when signal was set
                verifyMode: null,              // 'interactive' | 'oneshot' | 'textonly'
                verifyShellExited: false,     // true when persistent shell exited (user typed 'exit')
                verifyHint: '',               // suggested verify command text

                // Fallback verification (async, when CaptureSession unavailable).
                verifyFallbackRunning: false,  // true while async verifySplitAsync is running
                verifyFallbackError: null,     // error string from fallback verification

                // Split-view (Agent window-in-window).
                splitViewEnabled: false,       // true when split-view is active
                splitViewRatio: 0.6,           // wizard gets this fraction of content height
                splitViewFocus: 'wizard',      // 'wizard' or 'agent' — which pane is focused
                splitViewTab: 'agent',        // Task 8: 'agent' | 'output' | 'verify' — active tab
                agentScreenshot: '',          // cached plain-text screenshot from tuiMux
                agentScreen: '',              // cached ANSI-styled screen from tuiMux (T28)
                agentViewOffset: 0,           // scroll offset in Agent pane (lines from bottom)

                // T45: Auto-attach Agent pane state.
                agentAutoAttached: false,     // true once auto-attach has fired (prevents re-trigger)
                agentManuallyDismissed: false, // true when user explicitly closed split-view via Ctrl+L
                agentAutoAttachNotif: '',     // transient notification text (auto-dismissed after 5s)
                agentAutoAttachNotifAt: 0,    // Date.now() when notification was set

                // T073: Clipboard flash notification (Report overlay copy).
                clipboardFlash: '',             // transient flash text after copy attempt
                clipboardFlashAt: 0,            // Date.now() when flash was set

                // T62: Split-view copy/paste selection state.
                selectionActive: false,         // true when text selection is in progress
                selectionPane: '',              // pane with active selection: 'agent' | 'output' | 'verify'
                selectionStartRow: 0,           // selection anchor row (0-indexed)
                selectionStartCol: 0,           // selection anchor column (0-indexed)
                selectionEndRow: 0,             // selection end row (0-indexed, moves with keyboard/mouse)
                selectionEndCol: 0,             // selection end column (0-indexed)
                selectionByMouse: false,        // true if selection was initiated by mouse drag
                selectedText: '',               // extracted plain text of current selection

                // T46: Agent question detection state.
                agentQuestionDetected: false,  // true when question pattern detected in Agent output
                agentQuestionLine: '',         // the detected question line from Agent's output
                agentQuestionInputText: '',    // user's response text buffer
                agentQuestionInputActive: false, // user has focused the inline response input
                agentConversations: [],        // Q&A history [{question: string, answer: string, ts: number}]

                // T44: Process output capture state.
                outputLines: [],               // accumulated output buffer (strings, may contain ANSI)
                outputViewOffset: 0,           // scroll offset in output pane (lines from bottom)
                outputAutoScroll: true,        // auto-scroll to latest output

                // Agent conversation (T16).
                agentConvo: {
                    active: false,             // conversation overlay is visible
                    context: null,             // 'plan-review' | 'error-resolution' | null
                    history: [],               // [{ role: 'user'|'agent', text: string, ts: number }]
                    inputText: '',             // current text buffer
                    sending: false,            // async send in flight
                    waitingForTool: null,       // MCP tool being waited on (or null)
                    lastError: null,           // last error string
                    scrollOffset: 0,           // scroll offset in history view
                    spawnProgress: null        // T5: on-demand spawn progress message
                },
                agentOnDemandSpawning: false, // T5: async Agent spawn in flight

                // Auto-split pipeline state.
                autoSplitRunning: false,
                autoSplitResult: null,

                // Agent crash detection.
                agentCrashDetected: false,
                lastAgentHealthCheckMs: 0,

                // Results.
                equivalenceResult: null,
                errorDetails: null,

                // PR creation state (T095+T076).
                prCreationRunning: false,   // true while async createPRs is in flight
                prCreationError: null,      // error string from createPRs or null
                prCreationResults: null,    // array of per-PR results or null
                prCreationProgressMsg: '',  // real-time progress message from progressFn
                prCreationDryRun: false,    // T077: true if results are from dry-run

                // Task 10: Resume awareness — truthful previous session state.
                resumeFound: false,             // true if previous state file was found
                resumeSessions: [],             // [{name, kind, status, pid}] from previousState
                resumeStale: false,             // true if state is >24h old
                resumeAgeMs: 0,                 // age of state file in ms
                resumeNotifDismissed: false,    // true once user dismissed resume notification

                // First render flag.
                needsInitClear: true
            };
        };

        // Model init returns a startup heartbeat command so the update loop
        // can keep draining mux events even when the TUI is otherwise idle.
        var applyResumeState = function(state) {
            var prev = prSplit.previousState;
            if (!prev) return;
            var meta = prev._resumeMeta || {};
            state.resumeFound = true;
            state.resumeStale = !!meta.stale;
            state.resumeAgeMs = meta.ageMs || 0;
            var sessions = prev.sessions || [];
            state.resumeSessions = [];
            for (var ri = 0; ri < sessions.length; ri++) {
                var rs = sessions[ri];
                state.resumeSessions.push({
                    name: (rs.target && rs.target.name) || 'unnamed',
                    kind: (rs.target && rs.target.kind) || 'unknown',
                    status: rs.status || 'unknown',
                    pid: rs.pid || 0
                });
            }
            log.info('resume: previous state detected', {
                sessionCount: sessions.length,
                stale: state.resumeStale,
                ageMs: state.resumeAgeMs
            });
        };

        var _initModelFn = function() {
            var state = _initStateFn();

            // Task 10: Populate resume state from previousState if available.
            // previousState loads asynchronously now; when it lands after
            // init, applyResumeState applies it and schedules a re-render.
            if (!prSplit.previousState && typeof prSplit.previousStatePromise === 'object' && prSplit.previousStatePromise) {
                prSplit.previousStatePromise.then(function(prev) {
                    if (!prev) return;
                    prSplit.previousState = prev;
                    applyResumeState(state);
                    prSplit._stateRefresh && prSplit._stateRefresh();
                });
            }
            if (prSplit.previousState) {
                applyResumeState(state);
            }

            // T10: Store current model state reference so _onToggle can
            // access focused pane and active sessions for passthrough dispatch.
            prSplit._toggleModelState = state;
            var pendingOutput = prSplit._pendingTuiOutput || [];
            prSplit._pendingTuiOutput = [];
            for (var pi = 0; pi < pendingOutput.length; pi++) {
                appendTuiOutput(state, pendingOutput[pi]);
            }
            return [ state, tea.tick(C.TICK_INTERVAL_MS, 'mux-poll') ];
        };

        // Unit tests still expect the exported wizard init helper to be
        // state-only, so keep a thin wrapper for that contract. Export the
        // full model init path separately for tests that need to prove
        // startup behavior that depends on prSplit.previousState.
        var _initFn = function() {
            return _initStateFn();
        };

        var _updateFn = function(msg, s) {
            return prSplit._wizardUpdateImpl(msg, s);
        };

        var _viewFn = function(s) {
            var cursor = null;
            try {
                if (s.splitViewEnabled && s.splitViewFocus === 'agent' && s.splitViewTab === 'agent' &&
                    typeof prSplit._agentLiveActive === 'function' && prSplit._agentLiveActive() &&
                    typeof prSplit._agentLiveCursor === 'function') {
                    cursor = prSplit._agentLiveCursor();
                }
            } catch (e) {
                log.debug('agent live cursor read failed', { error: e.message || String(e) });
                cursor = null;
            }
            return {
                content: wizardViewImpl(s),
                altScreen: true,
                mouseMode: 'allMotion',
                cursor: cursor
            };
        };

        var model = tea.newModel({
            init: _initModelFn,

            update: _updateFn,

            view: _viewFn
        });

        // Export lifecycle functions for unit testing.
        // _wizardView exports the string-returning impl (not the v2 object
        // wrapper) so tests can call .indexOf / .split directly.
        prSplit._wizardInit = _initFn;
        prSplit._wizardModelInit = _initModelFn;
        prSplit._wizardUpdate = _updateFn;
        prSplit._wizardView = wizardViewImpl;
        // NOTE: prSplit._getFocusElements is now exported by chunk 16a.

        return model;
    }

    // --- Program Launch (T025 + T027) ---

    var _wizardModel = createWizardModel();

    // Export for Go-side launching and test access.
    prSplit._wizardModel = _wizardModel;
    prSplit._createWizardModel = createWizardModel;

    // T394/T10: onToggle callback for Ctrl+] passthrough. Extracted as a named
    // function so tests can exercise the guard logic independently.
    // T10: Now dispatches to any focused interactive pane, not only mux.
    // Task 5: Session-specific — uses pinned SessionID via proxy passthrough
    // instead of raw tuiMux.switchTo() which targets the active session.
    prSplit._onToggle = async function(options) {
        var tuiState = prSplit._toggleModelState;
        var focusTab = tuiState && tuiState.splitViewTab || 'agent';
        var focusPane = tuiState && tuiState.splitViewFocus || 'wizard';

        log.printf('ctrl+] toggle: focusPane=%s focusTab=%s', focusPane, focusTab);

        var targetTab = (focusPane === 'wizard') ? 'agent' : focusTab;

        var session = (typeof prSplit._getInteractivePaneSession === 'function')
            ? prSplit._getInteractivePaneSession(tuiState, targetTab)
            : null;

        if (session && typeof session.passthrough === 'function' &&
            typeof session.isRunning === 'function') {
            var running = session.isRunning();
            if (running && typeof running.then === 'function') running = await running;
            if (running) {
                log.printf('ctrl+] toggle: dispatching to %s session passthrough', targetTab);
                return await session.passthrough(options);
            }
        }

        log.printf('ctrl+] toggle: no child available, skipping');
        return {skipped: true, reason: 'no_child'};
    };

    // startWizard — called by pr_split.go to launch the BubbleTea wizard.
    // Blocks the calling goroutine until the user exits the wizard.
    //
    // T394: Pass toggleKey + onToggle so BubbleTea wraps the model in a
    // toggleModel. This ensures Ctrl+] passthrough properly releases
    // BubbleTea's cancelreader before RunPassthrough reads stdin, avoiding
    // data corruption from concurrent stdin readers.
    prSplit.startWizard = function() {
        prSplit._tuiOutputActive = true;
        prSplit._pendingTuiOutput = [];
        if (typeof output._setTUIOutputActive === 'function') {
            output._setTUIOutputActive(true);
        }

        var previousExit = globalThis.__postBubbleTeaExit;
        globalThis.__postBubbleTeaExit = function() {
            prSplit._tuiOutputActive = false;
            prSplit._pendingTuiOutput = [];
            if (typeof output._setTUIOutputActive === 'function') {
                output._setTUIOutputActive(false);
            }
            if (typeof previousExit === 'function') {
                return previousExit();
            }
        };

        try {
            return tea.run(_wizardModel, {
                toggleKey: 0x1D, // Ctrl+]
                onToggle: prSplit._onToggle
            });
        } catch (e) {
            prSplit._tuiOutputActive = false;
            prSplit._pendingTuiOutput = [];
            if (typeof output._setTUIOutputActive === 'function') {
                output._setTUIOutputActive(false);
            }
            throw e;
        }
    };

    // --- Mode Registration ---
    //  Commands remain for programmatic/test dispatch.
    //  The BubbleTea wizard above is launched by pr_split.go for interactive
    //  use. This registration exposes all commands so existing tests and
    //  the scripting API continue to work.

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

    // Cross-chunk exports.
    prSplit._handleMouseClick = handleMouseClick;
    prSplit._handleScreenMouseClick = handleScreenMouseClick;

})(globalThis.prSplit);

// --- CommonJS exports for require() compatibility (test environments). ---
if (typeof module !== 'undefined' && module.exports) {
    module.exports = globalThis.prSplit;
}
