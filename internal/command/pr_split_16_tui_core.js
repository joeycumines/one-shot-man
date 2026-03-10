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
    var viewForState = prSplit._viewForState;
    var viewHelpOverlay = prSplit._viewHelpOverlay;
    var viewConfirmCancelOverlay = prSplit._viewConfirmCancelOverlay;
    var viewReportOverlay = prSplit._viewReportOverlay;
    var viewMoveFileDialog = prSplit._viewMoveFileDialog;
    var viewRenameSplitDialog = prSplit._viewRenameSplitDialog;
    var viewMergeSplitsDialog = prSplit._viewMergeSplitsDialog;

    // Cross-chunk imports — state and handlers from chunks 13-14.
    var st = prSplit._state;
    var tuiState = prSplit._tuiState;
    var WizardState = prSplit.WizardState;
    var buildReport = prSplit._buildReport;
    var buildCommands = prSplit._buildCommands;
    var handleConfigState = prSplit._handleConfigState;
    var handlePlanReviewState = prSplit._handlePlanReviewState;
    var handlePlanEditorState = prSplit._handlePlanEditorState;
    var handleEquivCheckState = prSplit._handleEquivCheckState;
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

                // Focus system.
                focusIndex: 0,
                _prevWizardState: null,

                // Analysis progress.
                analysisSteps: [],
                analysisProgress: -1,

                // Execution state.
                executionResults: [],
                executingIdx: 0,

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
                    return [s, tea.clearScreen()];
                }
                return [s, null];
            }

            // Reset focus index on wizard state transition.
            if (s.wizardState !== s._prevWizardState) {
                s.focusIndex = 0;
                s._prevWizardState = s.wizardState;
            }

            // Overlays intercept all input when active.
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

            // Global key bindings.
            if (msg.type === 'Key') {
                var k = msg.key;
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
                if (k === 'e' && s.wizardState === 'PLAN_REVIEW' && !s.isProcessing) {
                    // Enter plan editor.
                    s.wizard.transition('PLAN_EDITOR');
                    s.wizardState = 'PLAN_EDITOR';
                    return [s, null];
                }
                // termmux toggle.
                if (k === 'ctrl+]') {
                    if (typeof tuiMux !== 'undefined' && tuiMux &&
                        typeof tuiMux.switchTo === 'function') {
                        tuiMux.switchTo('claude');
                    }
                    return [s, null];
                }
            }

            // Mouse handling.
            // Wheel events must be checked BEFORE press — wheel events
            // have action:"press" AND isWheel:true, so the press guard
            // would intercept them and send them to handleMouseClick.
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

            // Tick-based async analysis steps.
            // Each step runs synchronously but yields between steps for rendering and cancel.
            if (msg.type === 'Tick') {
                if (msg.id === 'analysis-step-0') {
                    return runAnalysisStep(s, 0);
                }
                if (msg.id === 'analysis-step-1') {
                    return runAnalysisStep(s, 1);
                }
                if (msg.id === 'analysis-step-2') {
                    return runAnalysisStep(s, 2);
                }
                if (msg.id === 'analysis-step-3') {
                    return runAnalysisStep(s, 3);
                }
                // Execution steps.
                if (msg.id === 'exec-step-0') {
                    return runExecutionStep(s, 0);
                }
                if (msg.id === 'exec-step-1') {
                    return runExecutionStep(s, 1);
                }
                // Automated pipeline polling.
                if (msg.id === 'auto-poll') {
                    return handleAutoSplitPoll(s);
                }
                // Resolve-conflicts polling.
                if (msg.id === 'resolve-poll') {
                    return handleResolvePoll(s);
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
                s.vp.setHeight(vpHeight);
                s.vp.setContent(screenContent);

                // Scrollbar.
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
        if (msg.type === 'Key') {
            var k = msg.key;
            if (k === 'y' || k === 'enter') {
                s.showConfirmCancel = false;
                s.wizard.cancel();
                s.wizardState = 'CANCELLED';
                return [s, tea.quit()];
            }
            if (k === 'n' || k === 'esc') {
                s.showConfirmCancel = false;
                return [s, null];
            }
        }
        if (msg.type === 'Mouse' && msg.action === 'press') {
            if (zone.inBounds('confirm-yes', msg)) {
                s.showConfirmCancel = false;
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
            s.showConfirmCancel = true;
            return [s, null];
        }
        if (zone.inBounds('nav-next', msg)) {
            return handleNext(s);
        }
        // Claude status badge.
        if (zone.inBounds('claude-status', msg)) {
            if (typeof tuiMux !== 'undefined' && tuiMux &&
                typeof tuiMux.switchTo === 'function') {
                tuiMux.switchTo('claude');
            }
            return [s, null];
        }
        // Screen-specific zone clicks.
        return handleScreenMouseClick(msg, s);
    }

    function handleScreenMouseClick(msg, s) {
        // Config screen: strategy selection, advanced toggle.
        if (s.wizardState === 'CONFIG' || s.wizardState === 'IDLE') {
            var strategies = ['auto', 'heuristic', 'directory'];
            for (var si = 0; si < strategies.length; si++) {
                if (zone.inBounds('strategy-' + strategies[si], msg)) {
                    prSplit.runtime.mode = strategies[si];
                    return [s, null];
                }
            }
            if (zone.inBounds('toggle-advanced', msg)) {
                s.showAdvanced = !s.showAdvanced;
                return [s, null];
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
                s.wizard.transition('PLAN_EDITOR');
                s.wizardState = 'PLAN_EDITOR';
                return [s, null];
            }
            if (zone.inBounds('plan-regenerate', msg) && !s.isProcessing) {
                handlePlanReviewState(s.wizard, 'regenerate');
                s.wizardState = 'CONFIG';
                s.wizard.reset();
                s.wizard.transition('CONFIG');
                return [s, null];
            }
        }

        // Plan editor: split selection, file selection, action buttons.
        if (s.wizardState === 'PLAN_EDITOR') {
            if (st.planCache && st.planCache.splits) {
                for (var i = 0; i < st.planCache.splits.length; i++) {
                    if (zone.inBounds('edit-split-' + i, msg)) {
                        s.selectedSplitIdx = i;
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
            var resolveChoices = ['auto', 'manual', 'skip', 'retry', 'abort'];
            for (var ri = 0; ri < resolveChoices.length; ri++) {
                if (zone.inBounds('resolve-' + resolveChoices[ri], msg)) {
                    return handleErrorResolutionChoice(s, resolveChoices[ri] === 'auto' ? 'auto-resolve' : resolveChoices[ri]);
                }
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

    function handleBack(s) {
        switch (s.wizardState) {
            case 'PLAN_REVIEW':
                s.wizardState = 'CONFIG';
                s.wizard.reset();
                s.wizard.transition('CONFIG');
                return [s, null];
            case 'PLAN_EDITOR':
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
            case 'PLAN_EDITOR':
                // Save edits and return to review.
                handlePlanEditorState(s.wizard, 'save', st.planCache);
                s.wizardState = 'PLAN_REVIEW';
                return [s, null];
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
                return [
                    {id: 'strategy-auto',      type: 'strategy'},
                    {id: 'strategy-heuristic',  type: 'strategy'},
                    {id: 'strategy-directory',  type: 'strategy'},
                    {id: 'nav-next',            type: 'nav'}
                ];
            }
            case 'PLAN_REVIEW': {
                var elems = [];
                var n = (st.planCache && st.planCache.splits) ? st.planCache.splits.length : 0;
                for (var i = 0; i < n; i++) {
                    elems.push({id: 'split-card-' + i, type: 'card'});
                }
                elems.push({id: 'plan-edit',       type: 'button'});
                elems.push({id: 'plan-regenerate',  type: 'button'});
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
                return [
                    {id: 'resolve-auto',   type: 'button'},
                    {id: 'resolve-manual', type: 'button'},
                    {id: 'resolve-skip',   type: 'button'},
                    {id: 'resolve-retry',  type: 'button'},
                    {id: 'resolve-abort',  type: 'button'}
                ];
            }
            default:
                return [];
        }
    }

    // List navigation (j/k/up/down): navigates splits only, clamped.
    // Also syncs focusIndex to the selected split card.
    function handleListNav(s, delta) {
        if (s.wizardState === 'PLAN_REVIEW' || s.wizardState === 'PLAN_EDITOR') {
            var splitCount = (st.planCache && st.planCache.splits)
                ? st.planCache.splits.length : 0;
            if (splitCount > 0) {
                var newIdx = (s.selectedSplitIdx || 0) + delta;
                newIdx = Math.max(0, Math.min(newIdx, splitCount - 1));
                s.selectedSplitIdx = newIdx;
                // Keep focusIndex in sync with the split card.
                s.focusIndex = newIdx;
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
            return [s, null];
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
                s.wizard.transition('PLAN_EDITOR');
                s.wizardState = 'PLAN_EDITOR';
                return [s, null];
            }
            if (focused.id === 'plan-regenerate' && !s.isProcessing) {
                handlePlanReviewState(s.wizard, 'regenerate');
                s.wizardState = 'CONFIG';
                s.wizard.reset();
                s.wizard.transition('CONFIG');
                return [s, null];
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
        }

        return null;
    }

    // -----------------------------------------------------------------------
    //  Async Pipeline Handlers — drive wizard state machine
    // -----------------------------------------------------------------------

    function startAnalysis(s) {
        s.isProcessing = true;
        s.analysisProgress = 0;
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
            s.isProcessing = false;
            s.errorDetails = configResult.error;
            s.wizardState = 'ERROR';
            return [s, null];
        }

        // Transition to CONFIG if needed.
        if (s.wizard.current === 'IDLE') {
            s.wizard.transition('CONFIG');
        }

        // Dispatch first analysis step via tick to yield for rendering.
        // Each step runs on the next tick, allowing BubbleTea to render
        // progress between steps and letting the user cancel with Ctrl+C.
        return [s, tea.tick(1, 'analysis-step-0')];
    }

    // runAnalysisStep: Runs a single analysis step synchronously, then
    // dispatches the next step via tick. Each step blocks the event loop
    // for its duration (~1-5s), but between steps the UI can render and
    // the user can cancel.
    function runAnalysisStep(s, stepIdx) {
        // If processing was cancelled (e.g., user hit Ctrl+C between steps),
        // bail out without running the step.
        if (!s.isProcessing) {
            return [s, null];
        }

        switch (stepIdx) {
        case 0: {
            // Step 1: Analyze diff.
            s.analysisSteps[0].active = true;
            var analysisStart = Date.now();
            try {
                st.analysisCache = prSplit.analyzeDiff({ baseBranch: prSplit.runtime.baseBranch });
            } catch (e) {
                s.isProcessing = false;
                s.errorDetails = 'Analysis failed: ' + (e.message || String(e));
                s.wizardState = 'ERROR';
                return [s, null];
            }
            s.analysisSteps[0].done = true;
            s.analysisSteps[0].active = false;
            s.analysisSteps[0].elapsed = Date.now() - analysisStart;
            s.analysisProgress = 0.25;

            if (st.analysisCache.error) {
                s.isProcessing = false;
                s.errorDetails = st.analysisCache.error;
                s.wizardState = 'ERROR';
                return [s, null];
            }
            if (!st.analysisCache.files || st.analysisCache.files.length === 0) {
                s.isProcessing = false;
                s.errorDetails = 'No changes found between branches.';
                s.wizardState = 'CONFIG';
                return [s, null];
            }

            // Yield for render, then dispatch step 1.
            return [s, tea.tick(1, 'analysis-step-1')];
        }
        case 1: {
            // Step 2: Group files.
            s.analysisSteps[1].active = true;
            var groupStart = Date.now();
            st.groupsCache = prSplit.applyStrategy(
                st.analysisCache.files, prSplit.runtime.strategy);
            s.analysisSteps[1].done = true;
            s.analysisSteps[1].active = false;
            s.analysisSteps[1].elapsed = Date.now() - groupStart;
            s.analysisProgress = 0.5;

            // Yield for render, then dispatch step 2.
            return [s, tea.tick(1, 'analysis-step-2')];
        }
        case 2: {
            // Step 3: Create plan.
            s.analysisSteps[2].active = true;
            var planStart = Date.now();
            st.planCache = prSplit.createSplitPlan(st.groupsCache, {
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

            // Yield for render, then dispatch step 3.
            return [s, tea.tick(1, 'analysis-step-3')];
        }
        case 3: {
            // Step 4: Validate.
            s.analysisSteps[3].active = true;
            var validation = prSplit.validatePlan(st.planCache);
            s.analysisSteps[3].done = true;
            s.analysisSteps[3].active = false;
            s.analysisProgress = 1.0;

            if (!validation.valid) {
                s.isProcessing = false;
                s.errorDetails = 'Plan validation failed: ' + validation.errors.join('; ');
                s.wizardState = 'ERROR';
                return [s, null];
            }

            s.isProcessing = false;

            // Transition wizard to PLAN_GENERATION then PLAN_REVIEW.
            if (s.wizard.current === 'CONFIG') {
                s.wizard.transition('PLAN_GENERATION');
            }
            s.wizard.transition('PLAN_REVIEW');
            s.wizardState = 'PLAN_REVIEW';
            return [s, null];
        }
        default:
            return [s, null];
        }
    }

    // startAutoAnalysis: Dispatches the full automated pipeline (Claude
    // classification → plan → execute → verify) as an async Promise.
    // The pipeline runs on the JS event loop independently. We poll for
    // completion via ticks so BubbleTea can render progress and the user
    // can cancel.
    function startAutoAnalysis(s) {
        s.isProcessing = true;
        s.analysisProgress = 0;
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
            s.isProcessing = false;
            s.errorDetails = configResult.error;
            s.wizardState = 'ERROR';
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
    // automatedSplit pipeline has completed. Updates progress indicators
    // and handles the final result.
    function handleAutoSplitPoll(s) {
        // If cancelled, stop polling.
        if (!s.isProcessing) {
            return [s, null];
        }

        // Still running — update progress from pipeline state and poll again.
        if (s.autoSplitRunning) {
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
        return [s, tea.tick(1, 'exec-step-1')];
    }

    function startExecution(s) {
        if (!st.planCache || !st.planCache.splits || st.planCache.splits.length === 0) {
            s.errorDetails = 'No plan to execute.';
            return [s, null];
        }

        s.isProcessing = true;
        s.executionResults = [];
        s.executingIdx = 0;

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

        // Dispatch first execution step via tick to yield for rendering.
        return [s, tea.tick(1, 'exec-step-0')];
    }

    // runExecutionStep: Runs a single execution step synchronously, then
    // dispatches the next step via tick. Yields between steps so the UI
    // can render progress and the user can cancel.
    function runExecutionStep(s, stepIdx) {
        // If processing was cancelled between steps, bail out.
        if (!s.isProcessing) {
            return [s, null];
        }

        switch (stepIdx) {
        case 0: {
            // Step 1: Execute the split plan (create branches).
            try {
                var result = prSplit.executeSplit(st.planCache);
                if (result.error) {
                    s.isProcessing = false;
                    s.errorDetails = result.error;
                    s.wizard.data.failedBranches = result.results ?
                        result.results.filter(function(r) { return r.error; }) : [];
                    s.wizard.transition('ERROR_RESOLUTION');
                    s.wizardState = 'ERROR_RESOLUTION';
                    return [s, null];
                }
                st.executionResultCache = result.results;
                s.executionResults = result.results || [];
            } catch (e) {
                s.isProcessing = false;
                s.errorDetails = 'Execution error: ' + (e.message || String(e));
                s.wizard.transition('ERROR_RESOLUTION');
                s.wizardState = 'ERROR_RESOLUTION';
                return [s, null];
            }

            // Yield for render, then run equivalence check.
            return [s, tea.tick(1, 'exec-step-1')];
        }
        case 1: {
            // Step 2: Equivalence check.
            // Guard: only transition if not already in EQUIV_CHECK
            // (handleResolvePoll pre-transitions for the skip/resolve paths).
            if (s.wizard.current !== 'EQUIV_CHECK') {
                s.wizard.transition('EQUIV_CHECK');
            }
            s.wizardState = 'EQUIV_CHECK';

            var equivResult = handleEquivCheckState(s.wizard, st.planCache);
            s.isProcessing = false;
            s.equivalenceResult = equivResult.equivalence || {};
            s.wizardState = s.wizard.current;

            return [s, null];
        }
        default:
            return [s, null];
        }
    }

    function handleErrorResolutionChoice(s, choice) {
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
                typeof tuiMux.switchTo === 'function') {
                tuiMux.switchTo('claude');
            }
            return [s, null];

        case 'skip':
            // Transition to EQUIV_CHECK happened in handleErrorResolutionState.
            // Dispatch equivalence check via the existing exec-step-1 handler.
            s.isProcessing = true;
            return [s, tea.tick(1, 'exec-step-1')];

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
