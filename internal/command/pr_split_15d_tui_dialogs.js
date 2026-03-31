'use strict';
// pr_split_15d_tui_dialogs.js — TUI: Finalization, dialogs, overlays, and state dispatcher
// Dependencies: pr_split_15a_tui_styles.js, 15b, and 15c must be loaded first.

(function(prSplit) {
    if (typeof tui === 'undefined' || typeof ctx === 'undefined' || typeof output === 'undefined') { return; }

    var st = prSplit._state;
    var styles = prSplit._wizardStyles;
    var COLORS = prSplit._wizardColors;
    var C = prSplit._TUI_CONSTANTS;
    var layoutMode = prSplit._layoutMode;
    var lipgloss = prSplit._lipgloss;
    var zone = prSplit._zone;
    var padRight = prSplit._padRight;
    var repeatStr = prSplit._repeatStr;
    var resolveColor = prSplit._resolveColor;
    var getInteractivePaneSession = prSplit._getInteractivePaneSession;

    // Screen renderers from 15c (needed by viewForState dispatcher).
    var viewConfigScreen = prSplit._viewConfigScreen;
    var viewAnalysisScreen = prSplit._viewAnalysisScreen;
    var viewPlanReviewScreen = prSplit._viewPlanReviewScreen;
    var viewPlanEditorScreen = prSplit._viewPlanEditorScreen;
    var viewExecutionScreen = prSplit._viewExecutionScreen;
    var viewVerificationScreen = prSplit._viewVerificationScreen;

    // ----- Screen 7: Finalization (FINALIZATION / DONE) -----

    function viewFinalizationScreen(s) {
        var lines = [];

        lines.push(styles.bold().render('PR Split Complete'));
        lines.push('');

        // Summary stats.
        var plan = st.planCache;
        if (plan && plan.splits) {
            lines.push('  Splits created: ' +
                styles.successBadge().render(' ' + plan.splits.length + ' '));
            lines.push('');

            for (var i = 0; i < plan.splits.length; i++) {
                var split = plan.splits[i];
                lines.push('    ' + (i + 1) + '. ' +
                    styles.fieldValue().render(split.name) +
                    styles.dim().render(' (' + split.files.length + ' files)'));
            }
        }

        // Equivalence result.
        if (s.equivalenceResult) {
            lines.push('');
            if (s.equivalenceResult.equivalent) {
                lines.push('  ' + styles.successBadge().render(' \u2705 Equivalence Verified '));
            } else {
                lines.push('  ' + styles.warningBadge().render(' \u26a0 Equivalence Not Verified '));
            }
        }

        // Elapsed time.
        if (s.startTime) {
            var totalSec = Math.floor((Date.now() - s.startTime) / 1000);
            lines.push('');
            lines.push(styles.dim().render(
                '  Total time: ' + Math.floor(totalSec / 60) + 'm ' + (totalSec % 60) + 's'));
        }

        // T095+T076: PR creation state display with real-time progress.
        if (s.prCreationRunning) {
            lines.push('');
            var progressDetail = s.prCreationProgressMsg || 'Pushing branches and creating pull requests';
            lines.push('  ' + styles.warningBadge().render(' Creating PRs\u2026 ') +
                styles.dim().render('  ' + progressDetail));
        }
        if (s.prCreationError) {
            lines.push('');
            lines.push('  ' + styles.errorBadge().render(' PR Creation Error '));
            lines.push('  ' + styles.dim().render(s.prCreationError));
        }
        if (s.prCreationResults && s.prCreationResults.length > 0 && !s.prCreationRunning) {
            lines.push('');
            // T077: Show dry-run badge prominently.
            if (s.prCreationDryRun) {
                lines.push('  ' + styles.warningBadge().render(' DRY RUN ') +
                    styles.dim().render('  No branches pushed, no PRs created.'));
                lines.push('');
            }
            lines.push(styles.bold().render('  PR Results:'));
            var created = 0, skipped = 0, failed = 0, dryRunCount = 0;
            for (var ri = 0; ri < s.prCreationResults.length; ri++) {
                var pr = s.prCreationResults[ri];
                if (pr.dryRun) {
                    dryRunCount++;
                    lines.push('    ' + styles.dim().render('\u25cb ' + pr.name +
                        (pr.dryRunMsg ? ': ' + pr.dryRunMsg : '')));
                } else if (pr.error) {
                    failed++;
                    lines.push('    ' + styles.errorBadge().render(' \u2718 ') + ' ' +
                        styles.fieldValue().render(pr.name) +
                        styles.dim().render(': ' + pr.error));
                } else if (pr.skipped) {
                    skipped++;
                    lines.push('    ' + styles.warningBadge().render(' \u2014 ') + ' ' +
                        styles.fieldValue().render(pr.name) +
                        styles.dim().render(': ' + (pr.skipReason || 'skipped')));
                } else if (pr.prUrl) {
                    created++;
                    lines.push('    ' + styles.successBadge().render(' \u2713 ') + ' ' +
                        styles.fieldValue().render(pr.name) +
                        styles.dim().render(' \u2192 ') +
                        styles.fieldValue().render(pr.prUrl));
                } else {
                    created++;
                    lines.push('    ' + styles.successBadge().render(' \u2713 ') + ' ' +
                        styles.fieldValue().render(pr.name) +
                        styles.dim().render(' (pushed)'));
                }
            }
            lines.push('');
            var summary = s.prCreationDryRun ?
                '  DRY RUN: ' + dryRunCount + ' simulated' :
                '  ' + created + ' created';
            if (skipped > 0) summary += ', ' + skipped + ' skipped';
            if (failed > 0) summary += ', ' + failed + ' failed';
            lines.push(styles.dim().render(summary));
        }

        // Actions.
        var compact = layoutMode(s) === 'compact';
        // Focus indices: 0=final-report, 1=final-create-prs, 2=final-done.
        var focusIdx = s.focusIndex || 0;
        var reportStyle  = (focusIdx === 0) ? styles.focusedSecondaryButton() : styles.secondaryButton();
        // T095: Disable Create PRs button while running or after results
        var canCreatePRs = !s.prCreationRunning && !s.prCreationResults;
        var createLabel = s.prCreationRunning ? 'Creating\u2026' :
                         (s.prCreationResults ? 'PRs Created' : 'Create PRs');
        var createStyle  = !canCreatePRs ? styles.disabledButton() :
                           ((focusIdx === 1) ? styles.focusedButton() : styles.primaryButton());
        var doneStyle    = (focusIdx === 2) ? styles.focusedButton() : styles.primaryButton();
        lines.push('');
        if (compact) {
            lines.push(zone.mark('final-report',
                reportStyle.render('View Report')));
            lines.push(zone.mark('final-create-prs',
                createStyle.render(createLabel)));
            lines.push(zone.mark('final-done',
                doneStyle.render('Done')));
        } else {
            lines.push(lipgloss.joinHorizontal(lipgloss.Center,
                zone.mark('final-report',
                    reportStyle.render('View Report')),
                '  ',
                zone.mark('final-create-prs',
                    createStyle.render(createLabel)),
                '  ',
                zone.mark('final-done',
                    doneStyle.render('Done'))
            ));
        }

        return lines.join('\n');
    }

    // ----- Error Resolution Overlay (T018) -----

    function viewErrorResolutionScreen(s) {
        var lines = [];

        // Crash-specific header.
        if (s.claudeCrashDetected) {
            lines.push(styles.errorBadge().render(' Claude Process Crashed '));
        } else {
            lines.push(styles.errorBadge().render(' Error Resolution '));
        }
        lines.push('');

        if (s.errorDetails) {
            lines.push(styles.errorCard().width((s.width || 80) - 8).render(
                s.errorDetails));
            lines.push('');
        }

        var failedBranches = (s.wizard && s.wizard.data && s.wizard.data.failedBranches) || [];
        if (failedBranches.length > 0) {
            lines.push(styles.bold().render('Failed Branches:'));
            for (var i = 0; i < failedBranches.length; i++) {
                var fb = failedBranches[i];
                var name = fb.name || fb;
                var err = fb.verifyError || fb.error || '';
                lines.push('  ' + styles.errorBadge().render(' \u2718 ') + ' ' +
                    styles.fieldValue().render(name) +
                    (err ? '\n    ' + styles.dim().render(err) : ''));
            }
            lines.push('');
        }

        // Resolution options — crash-specific or standard.
        var focusIdx = s.focusIndex || 0;
        var compact = layoutMode(s) === 'compact';

        if (s.claudeCrashDetected) {
            // Crash recovery options.
            var crashButtons = [
                {id: 'resolve-restart-claude',     label: 'Restart Claude', desc: 'Re-spawn Claude process and resume', isPrimary: true},
                {id: 'resolve-fallback-heuristic', label: 'Heuristic Mode', desc: 'Continue without Claude (local splitting)', isPrimary: false},
                {id: 'resolve-abort',              label: 'Abort',          desc: 'Cancel the split',                      isPrimary: false}
            ];
            lines.push(styles.bold().render('Recovery Options:'));
            lines.push('');
            for (var ci = 0; ci < crashButtons.length; ci++) {
                var cb = crashButtons[ci];
                var isCrashFocused = (focusIdx === ci);
                var crashBtnStyle;
                if (isCrashFocused) {
                    crashBtnStyle = cb.isPrimary ? styles.focusedButton() : styles.focusedSecondaryButton();
                } else if (cb.isPrimary) {
                    crashBtnStyle = styles.primaryButton();
                } else {
                    crashBtnStyle = styles.secondaryButton();
                }
                var crashLine = '  ' + zone.mark(cb.id, crashBtnStyle.render(cb.label));
                if (!compact) {
                    crashLine += styles.dim().render('  ' + cb.desc);
                }
                lines.push(crashLine);
                if (ci < crashButtons.length - 1) lines.push('');
            }
        } else {
            // Standard resolution options.
            var resolveButtons = [
                {id: 'resolve-auto',   label: 'Auto-Resolve', desc: 'Let Claude fix the issues',           isPrimary: true},
                {id: 'resolve-manual', label: 'Manual Fix',   desc: 'Switch to Claude pane to fix manually', isPrimary: false},
                {id: 'resolve-skip',   label: 'Skip',         desc: 'Skip failed branches',               isPrimary: false},
                {id: 'resolve-retry',  label: 'Retry',        desc: 'Regenerate plan from scratch',        isPrimary: false},
                {id: 'resolve-abort',  label: 'Abort',        desc: 'Cancel the split',                   isPrimary: false}
            ];
            lines.push(styles.bold().render('Choose Resolution:'));
            lines.push('');
            for (var ri = 0; ri < resolveButtons.length; ri++) {
                var rb = resolveButtons[ri];
                var isFocused = (focusIdx === ri);
                var btnStyle;
                if (isFocused) {
                    btnStyle = rb.isPrimary ? styles.focusedButton() : styles.focusedSecondaryButton();
                } else if (rb.isPrimary) {
                    btnStyle = styles.primaryButton();
                } else {
                    btnStyle = styles.secondaryButton();
                }
                var line = '  ' + zone.mark(rb.id, btnStyle.render(rb.label));
                if (!compact) {
                    line += styles.dim().render('  ' + rb.desc);
                }
                lines.push(line);
                if (ri < resolveButtons.length - 1) lines.push('');
            }

            // "Ask Claude" interactive conversation button (T16).
            if (st.claudeExecutor) {
                lines.push('');
                lines.push(styles.dim().render(repeatStr('\u2500', Math.min(40, (s.width || 80) - 12))));
                lines.push('');
                var askClaudeFocused = (focusIdx === resolveButtons.length);
                var askClaudeStyle = askClaudeFocused
                    ? styles.focusedSecondaryButton()
                    : styles.secondaryButton();
                lines.push('  ' + zone.mark('error-ask-claude',
                    askClaudeStyle.render('\ud83e\udd16 Ask Claude')) +
                    styles.dim().render('  Chat with Claude about this error'));
            }
        }

        return lines.join('\n');
    }

    // ----- Help Overlay (T019) -----

    function viewHelpOverlay(s) {
        var w = Math.min(64, (s.width || 80) - 4);
        var lines = [];
        var ws = s.wizardState || '';

        lines.push(styles.bold().render('Keyboard Shortcuts'));
        lines.push('');

        // -- Global Navigation (always shown) --
        lines.push(styles.label().render('Navigation'));
        lines.push(padRight('  ? / F1', 22) + 'Toggle this help');
        lines.push(padRight('  Tab', 22) + 'Next field / option');
        lines.push(padRight('  Shift+Tab', 22) + 'Previous field / option');
        lines.push(padRight('  Enter', 22) + 'Confirm / select');
        lines.push(padRight('  Esc', 22) + 'Back / close overlay');
        lines.push(padRight('  Ctrl+C', 22) + 'Cancel wizard');
        lines.push('');

        // -- Scrolling (always shown) --
        lines.push(styles.label().render('Scrolling'));
        lines.push(padRight('  j / \u2193', 22) + 'Move down / scroll');
        lines.push(padRight('  k / \u2191', 22) + 'Move up / scroll');
        lines.push(padRight('  PgUp / PgDn', 22) + 'Scroll page');
        lines.push(padRight('  Home / End', 22) + 'Jump to top / bottom');
        lines.push('');

        // -- Plan Editor (PLAN_EDITOR / PLAN_REVIEW only) --
        if (ws === 'PLAN_EDITOR' || ws === 'PLAN_REVIEW') {
            lines.push(styles.label().render('Plan Editor'));
            lines.push(padRight('  e', 22) + 'Edit / rename split');
            lines.push(padRight('  Space', 22) + 'Toggle file checkbox');
            lines.push(padRight('  Shift+\u2191 / \u2193', 22) + 'Reorder files');
            lines.push('');
        }

        // -- Branch Building (BRANCH_BUILDING / EQUIV_CHECK only) --
        if (ws === 'BRANCH_BUILDING' || ws === 'EQUIV_CHECK') {
            lines.push(styles.label().render('Branch Building'));
            lines.push(padRight('  e', 22) + 'Expand / collapse verify output');
            if (ws === 'BRANCH_BUILDING') {
                lines.push(padRight('  z', 22) + 'Pause / resume verify (SIGSTOP/SIGCONT)');
                lines.push(padRight('  Ctrl+C', 22) + 'Interrupt current verify (2x = force kill)');
                lines.push(padRight('  p', 22) + 'Mark failed branch as passed (override)');
            }
            // T007/T338: Open Shell hint — only shown when verify session is active.
            if (ws === 'BRANCH_BUILDING') {
                lines.push(padRight('  \ue795 Shell', 22) + 'Open interactive shell in worktree (Unix)');
            }
            lines.push('');
        }

        // -- Split View (always shown) --
        lines.push(styles.label().render('Split View'));
        lines.push(padRight('  Ctrl+L', 22) + 'Toggle split view');
        lines.push(padRight('  Ctrl+Tab', 22) + 'Focus wizard / terminal pane');
        lines.push(padRight('  Ctrl+O', 22) + 'Cycle tabs (Claude, Output, Verify, Shell)');
        lines.push(padRight('  Ctrl+]', 22) + 'Full Claude passthrough');
        lines.push(padRight('  Ctrl+= / Ctrl+-', 22) + 'Resize split view');
        // T337: Verify/Shell terminal tab interaction hints.
        if (ws === 'BRANCH_BUILDING' || ws === 'EQUIV_CHECK') {
            lines.push(padRight('  (type in pane)', 22) + 'Keys forwarded to focused terminal');
            lines.push(padRight('  Mouse in pane', 22) + 'Clicks forwarded (SGR mouse)');
        }

        var content = lines.join('\n');
        return styles.activeCard().width(w).render(content);
    }

    // ----- Confirm Cancel Overlay -----

    function viewConfirmCancelOverlay(s) {
        var w = Math.min(50, (s.width || 80) - 4);
        var lines = [];

        lines.push(styles.warningBadge().render(' Cancel Wizard? '));
        lines.push('');
        // T031: contextual text — defensive: if a verify session is still
        // referenced when the overlay opens (e.g. race between SIGINT cleanup
        // and user interaction), show a verification-specific warning.
        if (getInteractivePaneSession(s, 'verify')) {
            lines.push('A verification is in progress.');
            lines.push('Cancelling will abort it and clean up the worktree.');
        } else {
            lines.push('Are you sure you want to cancel the PR split?');
            // T081: Context-aware warning — branches already created persist
            // in the repo after cancel.  Tell the user so they aren't
            // surprised by orphaned split branches.
            var createdCount = (s.executionResults || []).filter(function(r) {
                return r && !r.error && r.sha;
            }).length;
            if (createdCount > 0) {
                lines.push(createdCount + ' branch' + (createdCount === 1 ? '' : 'es') +
                    ' already created \u2014 ' + styles.dim().render('these will remain in your repo.'));
                lines.push(styles.dim().render('Clean up with: osm pr-split --cleanup'));
            } else {
                lines.push('All progress will be lost.');
            }
        }
        lines.push('');
        // T031: focus-aware button rendering.
        var fi = s.confirmCancelFocus || 0;  // 0 = yes (cancel), 1 = no (continue)
        var yesBtn = fi === 0
            ? styles.focusedErrorBadge().render(' Yes, Cancel ')
            : styles.errorBadge().render(' Yes, Cancel ');
        var noBtn = fi === 1
            ? styles.focusedButton().render(' No, Continue ')
            : styles.primaryButton().render(' No, Continue ');
        lines.push(
            zone.mark('confirm-yes', yesBtn) +
            '    ' +
            zone.mark('confirm-no', noBtn)
        );
        lines.push('');
        lines.push(styles.dim().render('Tab to switch  ·  Enter to confirm  ·  Esc to dismiss'));

        var content = lines.join('\n');
        return styles.activeCard().width(w).render(content);
    }

    // ----- Report Overlay -----

    // Pure helper: compute overlay dimensions from terminal size.
    // No side effects — safe to call from both update and view.
    function computeReportOverlayDims(s) {
        var overlayW = Math.min(72, (s.width || 80) - 6);
        var overlayH = Math.max(8, (s.height || C.DEFAULT_ROWS) - 6);
        var vpW = Math.max(10, overlayW - 4); // card padding + scrollbar
        var vpH = Math.max(3, overlayH - 4);  // title + hints + borders
        return { overlayW: overlayW, overlayH: overlayH, vpW: vpW, vpH: vpH };
    }

    // Sync viewport and scrollbar dimensions to current terminal size.
    // Called from _updateFn when the report overlay opens or terminal resizes.
    function syncReportOverlay(s) {
        var dims = computeReportOverlayDims(s);
        if (s.reportVp) {
            s.reportVp.setWidth(dims.vpW);
            s.reportVp.setHeight(dims.vpH);
        }
        if (s.reportSb && s.reportVp) {
            s.reportSb.setViewportHeight(dims.vpH);
            s.reportSb.setContentHeight(s.reportVp.totalLineCount());
            s.reportSb.setYOffset(s.reportVp.yOffset());
            // Scrollbar cosmetics — set once on open, not every render.
            s.reportSb.setChars('\u2588', '\u2591');
            s.reportSb.setThumbForeground(resolveColor(COLORS.primary));
            s.reportSb.setTrackForeground(resolveColor(COLORS.border));
        }
    }

    // Sync scrollbar scroll position after viewport scroll events.
    // Lightweight — only updates yOffset and content height.
    function syncReportScrollbar(s) {
        if (s.reportSb && s.reportVp) {
            s.reportSb.setContentHeight(s.reportVp.totalLineCount());
            s.reportSb.setYOffset(s.reportVp.yOffset());
        }
    }

    // PURE view function — reads viewport/scrollbar state, writes nothing.
    function viewReportOverlay(s) {
        var dims = computeReportOverlayDims(s);

        // Title bar.
        var titleLine = styles.bold().render('  Split Report');
        var hintLine = styles.dim().render(
            '  j/k scroll • PgUp/PgDn page • c copy • Esc close');

        // T073: Clipboard flash notification.
        var flashLine = '';
        if (s.clipboardFlash && s.clipboardFlashAt) {
            var flashElapsed = Date.now() - s.clipboardFlashAt;
            if (flashElapsed < C.CLIPBOARD_FLASH_MS) {
                var flashStyle = s.clipboardFlash.indexOf('failed') >= 0
                    ? styles.errorBadge() : styles.successBadge();
                flashLine = '\n  ' + flashStyle.render(' ' + s.clipboardFlash + ' ');
            }
        }

        var vpView = s.reportVp ? s.reportVp.view() : (s.reportContent || '');
        var sbView = (s.reportSb) ? s.reportSb.view() : '';

        var scrollContent = lipgloss.joinHorizontal(lipgloss.Top, vpView, sbView);

        var inner = [titleLine, '', scrollContent, '', hintLine + flashLine].join('\n');
        return styles.activeCard().width(dims.overlayW).render(inner);
    }

    // ----- Move File Dialog Overlay -----

    function viewMoveFileDialog(s) {
        var overlayW = Math.min(55, (s.width || 80) - 6);
        var lines = [];

        var splitIdx = s.selectedSplitIdx || 0;
        var fileIdx = s.selectedFileIdx || 0;
        var splits = (st.planCache && st.planCache.splits) ? st.planCache.splits : [];
        var srcSplit = splits[splitIdx];
        var fileName = (srcSplit && srcSplit.files && srcSplit.files[fileIdx])
            ? srcSplit.files[fileIdx] : '(no file selected)';

        lines.push(styles.bold().render('Move File'));
        lines.push('');
        lines.push(styles.dim().render('File: ') + styles.fieldValue().render(fileName));
        lines.push(styles.dim().render('From: ') + styles.label().render(
            srcSplit ? srcSplit.name : '?'));
        lines.push('');
        lines.push(styles.dim().render('Select target split:'));
        lines.push('');

        var ds = s.editorDialogState || {};
        var targetCursor = ds.targetIdx || 0;
        var ti = 0;

        for (var i = 0; i < splits.length; i++) {
            if (i === splitIdx) continue;
            var isActive = (ti === targetCursor);
            var bullet = isActive
                ? styles.primaryButton().render(' \u25b6 ')
                : '   ';
            var splitName = styles.label().render(splits[i].name || 'split-' + i);
            var fileCount = styles.dim().render(
                (splits[i].files ? splits[i].files.length : 0) + ' files');
            lines.push(zone.mark('move-target-' + ti,
                bullet + ' ' + splitName + '  ' + fileCount));
            ti++;
        }

        lines.push('');
        lines.push(styles.dim().render('j/k navigate \u2022 Enter confirm \u2022 Esc cancel'));
        lines.push('');
        lines.push(lipgloss.joinHorizontal(lipgloss.Center,
            zone.mark('move-confirm', styles.primaryButton().render(' Move ')),
            '  ',
            zone.mark('move-cancel', styles.secondaryButton().render(' Cancel '))
        ));

        var content = lines.join('\n');
        return styles.activeCard().width(overlayW).render(content);
    }

    // ----- Rename Split Dialog Overlay -----

    function viewRenameSplitDialog(s) {
        var overlayW = Math.min(50, (s.width || 80) - 6);
        var lines = [];

        var splitIdx = s.selectedSplitIdx || 0;
        var splits = (st.planCache && st.planCache.splits) ? st.planCache.splits : [];
        var currentName = (splits[splitIdx] && splits[splitIdx].name) || '';

        var ds = s.editorDialogState || {};
        var inputText = ds.inputText !== undefined ? ds.inputText : currentName;

        lines.push(styles.bold().render('Rename Split'));
        lines.push('');
        lines.push(styles.dim().render('Current: ') +
            styles.fieldValue().render(currentName));
        lines.push('');
        lines.push(styles.dim().render('New name:'));

        // Render a text input field with a cursor indicator.
        var inputDisplay = inputText + '\u2588'; // Block cursor at end.
        var inputField = styles.activeCard().width(Math.max(20, overlayW - 8)).render(
            styles.label().render(inputDisplay));
        lines.push(inputField);

        // T096: Show validation error from branch name check.
        if (ds.validationError) {
            lines.push(styles.errorText().render('\u26a0 ' + ds.validationError));
        }

        lines.push('');
        lines.push(styles.dim().render('Type to edit \u2022 Enter confirm \u2022 Esc cancel'));
        lines.push('');
        lines.push(lipgloss.joinHorizontal(lipgloss.Center,
            zone.mark('rename-confirm', styles.primaryButton().render(' Rename ')),
            '  ',
            zone.mark('rename-cancel', styles.secondaryButton().render(' Cancel '))
        ));

        var content = lines.join('\n');
        return styles.activeCard().width(overlayW).render(content);
    }

    // ----- Merge Splits Dialog Overlay -----

    function viewMergeSplitsDialog(s) {
        var overlayW = Math.min(55, (s.width || 80) - 6);
        var lines = [];

        var splitIdx = s.selectedSplitIdx || 0;
        var splits = (st.planCache && st.planCache.splits) ? st.planCache.splits : [];
        var dstSplit = splits[splitIdx];
        var dstName = dstSplit ? dstSplit.name : '?';

        lines.push(styles.bold().render('Merge Splits'));
        lines.push('');
        lines.push(styles.dim().render('Merge selected splits into: ') +
            styles.fieldValue().render(dstName));
        lines.push('');

        var ds = s.editorDialogState || {};
        var selected = ds.selected || {};
        var cursorIdx = ds.cursorIdx || 0;
        var ci = 0;

        for (var i = 0; i < splits.length; i++) {
            if (i === splitIdx) continue;
            var isChecked = !!selected[i];
            var isCursor = (ci === cursorIdx);
            var checkbox = isChecked ? '\u2611' : '\u2610';
            var pointer = isCursor ? '\u25b6 ' : '  ';
            var nameStyle = isCursor
                ? styles.bold().render(splits[i].name || 'split-' + i)
                : styles.label().render(splits[i].name || 'split-' + i);
            var fileCount = styles.dim().render(
                ' (' + (splits[i].files ? splits[i].files.length : 0) + ' files)');
            lines.push(zone.mark('merge-item-' + ci,
                pointer + checkbox + ' ' + nameStyle + fileCount));
            ci++;
        }

        lines.push('');
        lines.push(styles.dim().render(
            'j/k navigate \u2022 Space toggle \u2022 Enter merge \u2022 Esc cancel'));
        lines.push('');
        lines.push(lipgloss.joinHorizontal(lipgloss.Center,
            zone.mark('merge-confirm', styles.primaryButton().render(' Merge ')),
            '  ',
            zone.mark('merge-cancel', styles.secondaryButton().render(' Cancel '))
        ));

        var content = lines.join('\n');
        return styles.activeCard().width(overlayW).render(content);
    }

    // Map wizard state to the correct screen renderer.
    function viewForState(s) {
        switch (s.wizardState) {
            case 'IDLE':
            case 'CONFIG':
            case 'BASELINE_FAIL':
                return viewConfigScreen(s);
            case 'PLAN_GENERATION':
                return viewAnalysisScreen(s);
            case 'PLAN_REVIEW':
                return viewPlanReviewScreen(s);
            case 'PLAN_EDITOR':
                return viewPlanEditorScreen(s);
            case 'BRANCH_BUILDING':
                return viewExecutionScreen(s);
            case 'ERROR_RESOLUTION':
                return viewErrorResolutionScreen(s);
            case 'EQUIV_CHECK':
                return viewVerificationScreen(s);
            case 'FINALIZATION':
            case 'DONE':
                return viewFinalizationScreen(s);
            case 'CANCELLED':
            case 'FORCE_CANCEL':
                return styles.warningBadge().render(' Cancelled ') +
                    '\n\nThe PR split was cancelled.';
            case 'PAUSED': {  // T084: dedicated PAUSED screen with resume/quit buttons
                var pausedFrom = (s.wizard && s.wizard.data && s.wizard.data.pausedFrom) || 'unknown';
                var lines = [];
                lines.push(styles.warningBadge().render(' Paused '));
                lines.push('');
                lines.push('Pipeline paused from ' + styles.fieldValue().render(pausedFrom) + ' state.');
                var ckpt = s.wizard && s.wizard.checkpoint;
                if (ckpt) {
                    lines.push('Checkpoint saved: ' + ckpt);
                }
                lines.push('');
                var focusElems = prSplit._getFocusElements ? prSplit._getFocusElements(s) : [];
                var focusIdx = s.focusIndex || 0;
                var focusedElemId = (focusElems[focusIdx] || {}).id || '';
                var resumeBtn = (focusedElemId === 'pause-resume')
                    ? styles.focusedButton().render(' Resume ')
                    : styles.primaryButton().render(' Resume ');
                var quitBtn = (focusedElemId === 'pause-quit')
                    ? styles.focusedSecondaryButton().render(' Quit ')
                    : styles.secondaryButton().render(' Quit ');
                lines.push(lipgloss.joinHorizontal(lipgloss.Center,
                    zone.mark('pause-resume', resumeBtn), '  ',
                    zone.mark('pause-quit', quitBtn)));
                lines.push('');
                lines.push(styles.dim().render('Press Enter to activate focused button, or Ctrl+C to cancel.'));
                return lines.join('\n');
            }
            case 'ERROR':
                var lines = [];
                lines.push(styles.errorBadge().render(' Error '));
                lines.push('');
                if (s.errorFromState) {
                    lines.push('Previous state: ' + styles.fieldValue().render(s.errorFromState));
                    lines.push('');
                }
                lines.push(s.errorDetails || 'An unexpected error occurred.');
                return lines.join('\n');
            default:
                return 'Unknown state: ' + s.wizardState;
        }
    }

    // Export functions defined in this file.
    prSplit._viewFinalizationScreen = viewFinalizationScreen;
    prSplit._viewErrorResolutionScreen = viewErrorResolutionScreen;
    prSplit._viewHelpOverlay = viewHelpOverlay;
    prSplit._viewConfirmCancelOverlay = viewConfirmCancelOverlay;
    prSplit._viewReportOverlay = viewReportOverlay;
    prSplit._syncReportOverlay = syncReportOverlay;
    prSplit._syncReportScrollbar = syncReportScrollbar;
    prSplit._computeReportOverlayDims = computeReportOverlayDims;
    prSplit._viewMoveFileDialog = viewMoveFileDialog;
    prSplit._viewRenameSplitDialog = viewRenameSplitDialog;
    prSplit._viewMergeSplitsDialog = viewMergeSplitsDialog;
    prSplit._viewForState = viewForState;

})(globalThis.prSplit);
