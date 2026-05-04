'use strict';
// pr_split_16a_tui_focus.js — TUI: focus cycling, navigation, dialog update handlers, viewport sync
// Dependencies: chunks 00-15d must be loaded first.
// Requires Go-injected globals: tui, ctx, output, log, prSplitConfig.

(function(prSplit) {

    if (typeof tui === 'undefined' || typeof ctx === 'undefined' ||
        typeof output === 'undefined') { return; }

    // Cross-chunk imports — libraries.
    var tea = prSplit._tea;
    var zone = prSplit._zone;

    // Cross-chunk imports — state and handlers from chunks 13-14.
    var st = prSplit._state;
    var handlePlanReviewState = prSplit._handlePlanReviewState;
    var handlePlanEditorState = prSplit._handlePlanEditorState;
    var handleFinalizationState = prSplit._handleFinalizationState;

    // Cross-chunk imports — renderers/views from chunk 15.
    var viewMoveFileDialog = prSplit._viewMoveFileDialog;
    var viewRenameSplitDialog = prSplit._viewRenameSplitDialog;
    var viewMergeSplitsDialog = prSplit._viewMergeSplitsDialog;
    var syncReportOverlay = prSplit._syncReportOverlay;
    var buildReport = prSplit._buildReport;

    // Late-bound cross-chunk references (defined in later chunks, resolved at call time).
    function startAnalysis(s) { return prSplit._startAnalysis(s); }
    function startAutoAnalysis(s) { return prSplit._startAutoAnalysis(s); }
    function startExecution(s) { return prSplit._startExecution(s); }
    function startEquivCheck(s) { return prSplit._startEquivCheck(s); }
    function startPRCreation(s) { return prSplit._startPRCreation(s); }
    function openClaudeConvo(s, context) { return prSplit._openClaudeConvo(s, context); }
    function handleErrorResolutionChoice(s, choice) { return prSplit._handleErrorResolutionChoice(s, choice); }
    function formatReportForDisplay(report) { return prSplit._formatReportForDisplay(report); }
    var C = prSplit._TUI_CONSTANTS;
    var getInteractivePaneSession = prSplit._getInteractivePaneSession;

    // --- Shared Viewport Helpers ---

    // T120/T123: Sync main viewport and scrollbar dimensions from current
    // terminal size. Hoisted to IIFE scope so all update handlers can call
    // it — not just those inside createWizardModel.
    var CHROME_ESTIMATE = 8; // title(1) + 2 dividers + nav(~3) + status(~2)
    function syncMainViewport(s) {
        if (!s.vp) return;
        var w = s.width || 80;
        var h = s.height || C.DEFAULT_ROWS;
        var vpHeight = Math.max(3, h - CHROME_ESTIMATE);
        s.vp.setWidth(w);

        if (s.splitViewEnabled) {
            var minPaneH = 3;
            var wizardH = Math.max(minPaneH, Math.floor(vpHeight * (s.splitViewRatio || 0.5)));
            wizardH = Math.min(wizardH, vpHeight - minPaneH - 1);
            if (wizardH >= minPaneH) {
                s.vp.setHeight(wizardH);
            } else {
                s.vp.setHeight(vpHeight);
            }
        } else {
            s.vp.setHeight(vpHeight);
        }
    }

    function enterErrorState(s, details) {
        if (!s) return [s, null];

        if (!s.errorFromState) {
            s.errorFromState = (s.wizard && s.wizard.current && s.wizard.current !== 'ERROR')
                ? s.wizard.current
                : (s.wizardState || '');
        }

        if (!s.errorSplitViewState) {
            s.errorSplitViewState = {
                enabled: !!s.splitViewEnabled,
                focus: s.splitViewFocus || 'wizard',
                tab: s.splitViewTab || 'claude'
            };
        }

        s.splitViewEnabled = false;
        s.splitViewFocus = 'wizard';
        s.errorDetails = details || s.errorDetails || 'An unexpected error occurred.';

        if (typeof s.isProcessing === 'boolean') s.isProcessing = false;
        if (typeof s.analysisRunning === 'boolean') s.analysisRunning = false;
        if (typeof s.autoSplitRunning === 'boolean') s.autoSplitRunning = false;

        // Track error in verification phase if we were in a verification-related phase.
        var vp = s.verifyPhase;
        if (vp && vp !== prSplit._verifyPhases.NOT_STARTED &&
            vp !== prSplit._verifyPhases.COMPLETE &&
            vp !== prSplit._verifyPhases.ERROR) {
            prSplit._transitionVerifyPhase(s, prSplit._verifyPhases.ERROR);
        }

        try { s.wizard.transition('ERROR'); } catch (te) { log.debug('wizard: transition to ERROR failed: ' + (te.message || te)); }
        s.wizardState = s.wizard.current;
        return [s, null];
    }

    // --- Editor Dialog Handler — move-file, rename-split, merge-splits ---

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
        if (msg.type === 'MouseClick') {
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
                // T096: Validate branch name characters before accepting.
                if (text.length > 0 && prSplit.INVALID_BRANCH_CHARS && prSplit.INVALID_BRANCH_CHARS.test(text)) {
                    ds.validationError = 'Invalid branch name: contains forbidden character';
                    s.editorDialogState = ds;
                    return [s, null];
                }
                if (text.indexOf('..') !== -1) {
                    ds.validationError = 'Invalid branch name: contains ".."';
                    s.editorDialogState = ds;
                    return [s, null];
                }
                if (text.length >= 5 && text.slice(-5) === '.lock') {
                    ds.validationError = 'Invalid branch name: ends with ".lock"';
                    s.editorDialogState = ds;
                    return [s, null];
                }
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
                    ds.validationError = '';  // T096: clear on edit
                    s.editorDialogState = ds;
                }
                return [s, null];
            }
            // Typed character — single printable char (length 1).
            if (k.length === 1) {
                ds.inputText = (ds.inputText || '') + k;
                ds.validationError = '';  // T096: clear on edit
                s.editorDialogState = ds;
                return [s, null];
            }
        }

        // Mouse zone clicks.
        if (msg.type === 'MouseClick') {
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
            if (k === 'space') {
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
        if (msg.type === 'MouseClick') {
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

    // --- Navigation Handlers — Back / Next / Up / Down ---

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
            s.configFieldEditing = null;
            s.configFieldValue = '';
            // Clamp focus index to new (shorter) element list.
            var backElems = getFocusElements(s);
            if (s.focusIndex >= backElems.length) {
                s.focusIndex = Math.max(0, backElems.length - 1);
            }
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
            // T079: EQUIV_CHECK → PLAN_REVIEW (back-navigation).
            // T308: Clean up all equivalence state to prevent stale data
            // and orphaned async polling.
            case 'EQUIV_CHECK':
                if (!s.isProcessing) {
                    try { s.wizard.transition('PLAN_REVIEW'); } catch (te) { log.debug('applyEquiv: wizard.transition failed: ' + (te.message || te)); }
                    s.wizardState = s.wizard.current;
                    s.isProcessing = false;
                    s.equivRunning = false;
                    s.equivError = null;
                    s.equivalenceResult = null;
                }
                return [s, null];
            case 'ERROR': {
                var targetState = s.errorFromState || 'CONFIG';
                var restore = s.errorSplitViewState;
                if (restore) {
                    s.splitViewEnabled = !!restore.enabled;
                    s.splitViewFocus = restore.focus || 'wizard';
                    s.splitViewTab = restore.tab || 'claude';
                    s.errorSplitViewState = null;
                }
                s.errorFromState = '';
                if (targetState === 'ERROR') targetState = 'CONFIG';
                try { s.wizard.transition(targetState); } catch (te) { log.debug('errorBack: wizard.transition failed: ' + (te.message || te)); }
                s.wizardState = s.wizard.current;
                return [s, null];
            }
            default:
                return [s, null];
        }
    }

    // T084: Resume from PAUSED — transition back to the paused-from state
    // and re-trigger the appropriate pipeline. If pausedFrom is PLAN_GENERATION,
    // restart analysis. If BRANCH_BUILDING, restart the execution pipeline.
    function handlePauseResume(s) {
        if (s.wizardState !== 'PAUSED') return [s, null];
        var resumed = s.wizard.resume();
        if (!resumed) {
            return handlePauseQuit(s);
        }
        s.wizardState = s.wizard.current;
        // Re-trigger the pipeline for the resumed state.
        if (s.wizardState === 'PLAN_GENERATION') {
            return startAnalysis(s);
        }
        if (s.wizardState === 'BRANCH_BUILDING') {
            return startExecution(s);
        }
        // Defensive: unknown resumed state — cancel.
        return handlePauseQuit(s);
    }

    // T084: Quit from PAUSED — cancel the wizard and exit.
    function handlePauseQuit(s) {
        try { s.wizard.cancel(); } catch (te) { log.debug('cancelPipeline: wizard.cancel failed: ' + (te.message || te)); }
        s.wizardState = s.wizard.current;
        return [s, tea.quit()];
    }

    function handleNext(s) {
        if (s.isProcessing) {
            // T084: PAUSED is reachable while isProcessing is still set (paused mid-pipeline).
            if (s.wizardState === 'PAUSED') return handlePauseResume(s);
            return [s, null];
        }

        switch (s.wizardState) {
            case 'IDLE':
            case 'CONFIG':
                // Clear any active config field editing before leaving CONFIG.
                s.configFieldEditing = null;
                s.configFieldValue = '';
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
            case 'BRANCH_BUILDING':
                // T121: Safety net — if the user reaches BRANCH_BUILDING
                // (e.g., after automated split completes), advance to
                // EQUIV_CHECK so they can review equivalence results.
                prSplit._resetVerifyPhase(s);
                prSplit._transitionVerifyPhase(s, prSplit._verifyPhases.EQUIV_CHECK);
                s.isProcessing = true;
                return startEquivCheck(s);
            case 'EQUIV_CHECK':
                // T121: When equivalence results are already cached (automated
                // path) or the async check completed, allow manual advance.
                if (s.equivalenceResult && !s.isProcessing) {
                    try { s.wizard.transition('FINALIZATION'); } catch (te) { log.debug('wizard: transition to FINALIZATION failed: ' + (te.message || te)); }
                    s.wizardState = s.wizard.current;
                }
                return [s, null];
            case 'FINALIZATION':
                handleFinalizationState(s.wizard, 'done');
                s.wizardState = 'DONE';
                return [s, tea.quit()];
            default:
                return [s, null];
        }
    }

    // --- Focus System — keyboard-driven focus cycling across interactive elements ---

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
                // When advanced section is expanded, expose its fields.
                if (s.showAdvanced) {
                    elems.push({id: 'config-maxFiles',       type: 'field'});
                    elems.push({id: 'config-branchPrefix',   type: 'field'});
                    elems.push({id: 'config-verifyCommand',  type: 'field'});
                    elems.push({id: 'config-dryRun',         type: 'checkbox'});
                }
                elems.push({id: 'nav-next', type: 'nav'});
                elems.push({id: 'nav-cancel', type: 'nav'});  // T012
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
                elems.push({id: 'nav-cancel', type: 'nav'});  // T012
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
                elems.push({id: 'nav-cancel', type: 'nav'});  // T012
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
                // T5: Show Ask Claude when Claude binary is available (on-demand spawn).
                if ((st.claudeExecutor || s.claudeCheckStatus === 'available') && !s.claudeCrashDetected) {
                    elems.push({id: 'error-ask-claude', type: 'button'});
                }
                elems.push({id: 'nav-next', type: 'nav'});
                elems.push({id: 'nav-cancel', type: 'nav'});  // T012
                return elems;
            }
            case 'ERROR':
                return [
                    {id: 'nav-back', type: 'nav'},
                    {id: 'nav-cancel', type: 'nav'}
                ];
            case 'FINALIZATION': {
                var elems = [
                    {id: 'final-report',     type: 'button'},
                    {id: 'final-create-prs', type: 'button'},
                    {id: 'final-done',       type: 'button'}
                ];
                elems.push({id: 'nav-next', type: 'nav'});
                elems.push({id: 'nav-cancel', type: 'nav'});  // T012
                return elems;
            }
            // T061: EQUIV_CHECK focus elements — Re-verify, Revise Plan,
            // Continue. Only show actionable buttons when not processing.
            case 'EQUIV_CHECK': {
                var elems = [];
                if (!s.isProcessing && s.equivalenceResult) {
                    if (!s.equivalenceResult.equivalent) {
                        elems.push({id: 'equiv-reverify', type: 'button'});
                        elems.push({id: 'equiv-revise',   type: 'button'});
                    }
                    elems.push({id: 'nav-back', type: 'nav'});    // T301
                    elems.push({id: 'nav-next', type: 'nav'});
                    elems.push({id: 'nav-cancel', type: 'nav'});  // T012
                }
                return elems;
            }
            // T084: PAUSED — resume or quit.
            case 'PAUSED': {
                return [
                    {id: 'pause-resume', type: 'button'},
                    {id: 'pause-quit',   type: 'button'}
                ];
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
        // Config field editing also intercepts before this, but guard anyway.
        if (s.configFieldEditing) {
            return [s, null];
        }
        // CONFIG/IDLE/BASELINE_FAIL: j/k cycles focus like Tab/Shift-Tab.
        if (s.wizardState === 'CONFIG' || s.wizardState === 'IDLE' || s.wizardState === 'BASELINE_FAIL') {
            return delta > 0 ? handleNavDown(s) : handleNavUp(s);
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

        // T012: nav-cancel — open cancel confirmation (mirrors mouse handler).
        // During active verification, send interrupt instead of opening the
        // cancel dialog (same behavior as the mouse click on nav-cancel zone).
        if (focused.id === 'nav-cancel') {
            var activeVerifySession = getInteractivePaneSession(s, 'verify');
            if (activeVerifySession) {
                var now = Date.now();
                if (s.lastVerifyInterruptTime > 0 && (now - s.lastVerifyInterruptTime) < C.SIGKILL_WINDOW_MS) {
                    try { activeVerifySession.kill(); } catch (e) { log.debug('cancelVerify: verifySession.kill failed: ' + (e.message || e)); }
                } else {
                    try { activeVerifySession.interrupt(); } catch (e) { log.debug('cancelVerify: verifySession.interrupt failed: ' + (e.message || e)); }
                }
                s.lastVerifyInterruptTime = now;
                return [s, null];
            }
            s.showConfirmCancel = true;
            s.confirmCancelFocus = 0;  // T031: reset focus to 'Yes' on open
            return [s, null];
        }

        // T301: nav-back — invoke handleBack (mirrors mouse handler at line 1712).
        if (focused.id === 'nav-back') {
            return handleBack(s);
        }

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
            // T061: EQUIV_CHECK buttons.
            if (focused.id === 'equiv-reverify') {
                s.isProcessing = true;
                s.equivalenceResult = null;
                // Reset verifyPhase — re-running equiv from terminal COMPLETE/FAILED.
                prSplit._resetVerifyPhase(s);
                prSplit._transitionVerifyPhase(s, prSplit._verifyPhases.EQUIV_CHECK);
                return startEquivCheck(s);
            }
            // T079: Revise Plan — go back to PLAN_REVIEW from EQUIV_CHECK.
            // T308: Clean up equiv state on revise (same cleanup as handleBack).
            if (focused.id === 'equiv-revise') {
                try { s.wizard.transition('PLAN_REVIEW'); } catch (te) { log.debug('revise: wizard.transition failed: ' + (te.message || te)); }
                s.wizardState = s.wizard.current;
                s.isProcessing = false;
                s.equivRunning = false;
                s.equivError = null;
                s.equivalenceResult = null;
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
                syncReportOverlay(s);
                return [s, null];
            }
            if (focused.id === 'final-create-prs') {
                handleFinalizationState(s.wizard, 'create-prs');
                return startPRCreation(s);
            }
            if (focused.id === 'final-done') {
                handleFinalizationState(s.wizard, 'done');
                s.wizardState = 'DONE';
                return [s, tea.quit()];
            }
            // T084: PAUSED screen buttons.
            if (focused.id === 'pause-resume') {
                return handlePauseResume(s);
            }
            if (focused.id === 'pause-quit') {
                return handlePauseQuit(s);
            }
        }

        // Config field activation — enter inline edit mode.
        if (focused.type === 'field') {
            var fieldName = focused.id.replace('config-', '');
            var runtime = prSplit.runtime;
            s.configFieldEditing = fieldName;
            if (fieldName === 'maxFiles') {
                s.configFieldValue = String(typeof runtime.maxFiles === 'number' ? runtime.maxFiles : 10);
            } else if (fieldName === 'branchPrefix') {
                s.configFieldValue = runtime.branchPrefix || 'split/';
            } else if (fieldName === 'verifyCommand') {
                s.configFieldValue = runtime.verifyCommand || 'true';
            } else {
                s.configFieldValue = '';
            }
            return [s, null];
        }

        // Config checkbox toggle — dry run.
        if (focused.type === 'checkbox' && focused.id === 'config-dryRun') {
            prSplit.runtime.dryRun = !prSplit.runtime.dryRun;
            return [s, null];
        }

        return null;
    }

    // --- Cross-chunk exports ---
    prSplit._syncMainViewport = syncMainViewport;
    prSplit._CHROME_ESTIMATE = CHROME_ESTIMATE;
    prSplit._updateEditorDialog = updateEditorDialog;
    prSplit._updateMoveDialog = updateMoveDialog;
    prSplit._updateRenameDialog = updateRenameDialog;
    prSplit._updateMergeDialog = updateMergeDialog;
    prSplit._viewEditorDialog = viewEditorDialog;
    prSplit._enterPlanEditor = enterPlanEditor;
    prSplit._handleBack = handleBack;
    prSplit._enterErrorState = enterErrorState;
    prSplit._handlePauseResume = handlePauseResume;
    prSplit._handlePauseQuit = handlePauseQuit;
    prSplit._handleNext = handleNext;
    prSplit._getFocusElements = getFocusElements;
    prSplit._handleListNav = handleListNav;
    prSplit._handleNavDown = handleNavDown;
    prSplit._handleNavUp = handleNavUp;
    prSplit._syncSplitSelection = syncSplitSelection;
    prSplit._handleFocusActivate = handleFocusActivate;

})(globalThis.prSplit);
