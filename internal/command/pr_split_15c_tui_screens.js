'use strict';
// pr_split_15c_tui_screens.js — TUI: Wizard screen renderers (Config through Verification)
// Dependencies: pr_split_15a_tui_styles.js and pr_split_15b_tui_chrome.js must be loaded first.

(function(prSplit) {
    if (typeof tui === 'undefined' || typeof ctx === 'undefined' || typeof output === 'undefined') { return; }

    var st = prSplit._state;
    var styles = prSplit._wizardStyles;
    var COLORS = prSplit._wizardColors;
    var C = prSplit._TUI_CONSTANTS;
    var layoutMode = prSplit._layoutMode;
    var lipgloss = prSplit._lipgloss;
    var zone = prSplit._zone;
    var truncate = prSplit._truncate;
    var renderProgressBar = prSplit._renderProgressBar;
    var renderClaudeQuestionPrompt = prSplit._renderClaudeQuestionPrompt;
    var getInteractivePaneSession = prSplit._getInteractivePaneSession;

    // --- Screen Renderers (T009-T017) ---
    //
    //  Each screen returns a string for the content area.
    //  The update function sets model state; view calls the appropriate
    //  screen renderer based on wizardState.

    // ----- Screen 1: Configuration (IDLE / CONFIG) -----

    function viewConfigScreen(s) {
        var runtime = prSplit.runtime;
        var lines = [];

        // Repository info.
        lines.push(styles.bold().render('Repository'));
        lines.push('  ' + styles.fieldValue().render(runtime.dir || '.'));
        lines.push('');

        // Config form (non-editable display for now, wizard sets these).
        var srcBranch = (st.analysisCache && st.analysisCache.currentBranch) || '(auto-detect)';
        var srcLabel = styles.bold().render('Source Branch');
        var srcField = styles.activeCard().width(
            Math.max(20, (s.width || 80) - lipgloss.width(srcLabel) - 8)
        ).render(styles.fieldValue().render(srcBranch));
        lines.push('  ' + lipgloss.joinHorizontal(lipgloss.Left, srcLabel, '  ', srcField));
        lines.push('');

        var targetLabel = styles.bold().render('Target Branch');
        var targetField = styles.activeCard().width(
            Math.max(20, (s.width || 80) - lipgloss.width(targetLabel) - 8)
        ).render(styles.fieldValue().render(runtime.baseBranch || 'main'));
        lines.push('  ' + lipgloss.joinHorizontal(lipgloss.Left, targetLabel, '  ', targetField));
        lines.push('');

        // Strategy selection.
        lines.push(styles.bold().render('Strategy'));
        var strategies = ['auto', 'heuristic', 'directory'];
        var currentMode = runtime.mode || 'heuristic';
        // Focus: indices 0-2 map to strategies in CONFIG screen.
        var focusIdx = s.focusIndex || 0;
        for (var si = 0; si < strategies.length; si++) {
            var strat = strategies[si];
            var selected = (strat === currentMode);
            var isFocused = (focusIdx === si);
            var bullet = selected ? styles.primaryButton().render(' \u25cf ') : '  \u25cb ';
            var focusPointer = isFocused ? styles.statusActive().render('\u25b8 ') : '  ';
            var label = styles.label().render(strat.charAt(0).toUpperCase() + strat.slice(1));
            var stratId = 'strategy-' + strat;
            lines.push(focusPointer + zone.mark(stratId, bullet + ' ' + label));
        }

        // Claude availability status (shown after strategy buttons).
        if (s.claudeCheckStatus === 'checking') {
            var checkMsg = s.claudeCheckProgressMsg || 'Checking Claude availability\u2026';
            lines.push('');
            lines.push('  ' + styles.statusActive().render('\u23f3 ' + checkMsg));
        }
        if (s.claudeCheckStatus === 'available' && s.claudeResolvedInfo) {
            lines.push('');
            var claudeStatusMsg = s.userHasSelectedStrategy
                ? ' \u2713 Claude available '
                : ' \u2713 Claude available \u2014 using auto strategy ';
            lines.push('  ' + styles.successBadge().render(claudeStatusMsg));
            lines.push('    Command:  ' + styles.fieldValue().render(s.claudeResolvedInfo.command || '?'));
            lines.push('    Provider: ' + styles.fieldValue().render(s.claudeResolvedInfo.type || '?'));
        }
        if (s.claudeCheckStatus === 'unavailable') {
            lines.push('');
            lines.push('  ' + styles.errorBadge().render(' \u2717 Claude unavailable '));
            if (s.claudeCheckError) {
                lines.push('    ' + styles.dim().render(s.claudeCheckError));
            }
            lines.push('    ' + styles.dim().render('\u2192 Using heuristic strategy'));
        }

        // Test Connection button.
        // Compute focused element using the actual focus system.
        var focusElems = prSplit._getFocusElements ? prSplit._getFocusElements(s) : [];
        var focusedElemId = (focusElems[focusIdx] || {}).id || '';
        if (currentMode === 'auto' || s.claudeCheckStatus) {
            var testFocused = (focusedElemId === 'test-claude');
            var testBtnStyle = testFocused ? styles.focusedSecondaryButton() : styles.secondaryButton();
            lines.push('  ' + zone.mark('test-claude',
                testBtnStyle.render(' Test Connection ')));
        }
        lines.push('');

        // Advanced options toggle.
        // focusElems and focusedElemId already computed above.

        var advFocused = (focusedElemId === 'toggle-advanced');
        var advPrefix = advFocused ? styles.statusActive().render('\u25b8 ') : '  ';
        if (s.showAdvanced) {
            lines.push(advPrefix + zone.mark('toggle-advanced',
                styles.dim().render('\u25bc Advanced Options')));

            // Compute label width for column alignment.
            var labels = ['Max files per chunk', 'Branch prefix', 'Verify command'];
            var maxLabelLen = 0;
            for (var li = 0; li < labels.length; li++) {
                if (labels[li].length > maxLabelLen) maxLabelLen = labels[li].length;
            }
            var padLabel = function(lbl) {
                var pad = '';
                for (var pi = lbl.length; pi < maxLabelLen; pi++) pad += ' ';
                return lbl + pad;
            };

            // Render each advanced field with zone mark, focus indicator, and edit mode.
            var fieldDefs = [
                { id: 'config-maxFiles',       label: 'Max files per chunk', field: 'maxFiles',       value: String(typeof runtime.maxFiles === 'number' ? runtime.maxFiles : 10) },
                { id: 'config-branchPrefix',   label: 'Branch prefix',       field: 'branchPrefix',   value: runtime.branchPrefix || 'split/' },
                { id: 'config-verifyCommand',  label: 'Verify command',      field: 'verifyCommand',  value: runtime.verifyCommand || 'true' }
            ];
            for (var fi = 0; fi < fieldDefs.length; fi++) {
                var fd = fieldDefs[fi];
                var isFieldFocused = (focusedElemId === fd.id);
                var isEditing = (s.configFieldEditing === fd.field);
                var fieldPrefix = isFieldFocused ? ('  ' + styles.statusActive().render('\u25b8') + ' ') : '    ';
                var displayVal;
                if (isEditing) {
                    // Show edit buffer with cursor indicator.
                    var editBuf = (s.configFieldValue || '') + '\u2588';
                    displayVal = styles.primaryButton().render(' ' + editBuf + ' ');
                } else if (isFieldFocused) {
                    displayVal = styles.focusedButton().render(' ' + fd.value + ' ');
                } else {
                    displayVal = styles.fieldValue().render(fd.value);
                }
                lines.push(fieldPrefix + zone.mark(fd.id,
                    padLabel(fd.label) + ':  ' + displayVal));
            }

            // Dry run checkbox with zone mark and focus indicator.
            var dryFocused = (focusedElemId === 'config-dryRun');
            var dryPrefix = dryFocused ? ('  ' + styles.statusActive().render('\u25b8') + ' ') : '    ';
            var dryCheck = runtime.dryRun ? '\u2611' : '\u2610';
            var dryLabel = dryFocused
                ? styles.focusedButton().render(' ' + dryCheck + ' Dry run ')
                : dryCheck + ' Dry run';
            lines.push(dryPrefix + zone.mark('config-dryRun', dryLabel));
        } else {
            lines.push(advPrefix + zone.mark('toggle-advanced',
                styles.dim().render('\u25b6 Advanced Options')));
        }

        // T43: Inline config validation error.
        if (s.configValidationError) {
            lines.push('');
            lines.push('  ' + styles.errorBadge().render(' \u26a0 Configuration Error '));
            lines.push('  ' + styles.errorCard().width(Math.max(30, (s.width || 80) - 12)).render(
                s.configValidationError));
            // Show available branches if populated.
            if (s.availableBranches && s.availableBranches.length > 0) {
                lines.push('');
                lines.push('  ' + styles.bold().render('Available branches:'));
                for (var bi = 0; bi < s.availableBranches.length && bi < 15; bi++) {
                    lines.push('    \u2022 ' + styles.fieldValue().render(s.availableBranches[bi]));
                }
                if (s.availableBranches.length > 15) {
                    lines.push('    ' + styles.dim().render('... and ' + (s.availableBranches.length - 15) + ' more'));
                }
            }
        }

        return lines.join('\n');
    }

    // ----- Screen 2: Analysis (PLAN_GENERATION) -----

    function viewAnalysisScreen(s) {
        var lines = [];
        var runtime = prSplit.runtime;

        lines.push(styles.bold().render('Analyzing Changes'));
        lines.push('  ' + styles.fieldValue().render(
            (st.analysisCache && st.analysisCache.currentBranch || '?') +
            ' \u2192 ' + (runtime.baseBranch || 'main')
        ));
        lines.push('');

        // Progress steps.
        var steps = s.analysisSteps || [];
        for (var i = 0; i < steps.length; i++) {
            var step = steps[i];
            var icon = step.done ? styles.successBadge().render(' \u2713 ') :
                       step.active ? styles.warningBadge().render(' \u25b6 ') :
                       styles.dim().render(' \u25cb ');
            var stepLabel = styles.label().render(step.label);
            var stepTime = step.elapsed ? styles.dim().render(' (' + step.elapsed + 'ms)') : '';
            lines.push('  ' + icon + ' ' + stepLabel + stepTime);
        }

        if (s.analysisProgress >= 0) {
            lines.push('');
            lines.push('  ' + renderProgressBar(s.analysisProgress, (s.width || 80) - 8));
        }

        // T016: Always show abort hint during active analysis (not just slow).
        if (s.isProcessing && s.analysisRunning && !s.analysisSlowWarning) {
            lines.push('');
            lines.push(styles.dim().render('  Press Ctrl+C to cancel'));
        }

        // T002: Slow analysis timeout warning.
        if (s.analysisSlowWarning && s.analysisRunning) {
            var elSec = Math.floor((s.analysisElapsedMs || 0) / 1000);
            lines.push('');
            lines.push('  ' + styles.warningBadge().render(' \u23f1 Taking longer than expected ') +
                styles.dim().render('  ' + elSec + 's elapsed'));
            lines.push(styles.dim().render(
                '  This may indicate a large repo, slow network, or a credential prompt.'));
            lines.push(styles.dim().render(
                '  Press Ctrl+C to cancel.'));
        }

        // Show analysis results if available.
        if (st.analysisCache && st.analysisCache.files && !st.analysisCache.error) {
            lines.push('');
            lines.push(styles.successBadge().render(' Analysis Complete '));
            lines.push('  Changed files: ' +
                styles.fieldValue().render(String(st.analysisCache.files.length)));

            // File list (abbreviated).
            var maxShow = Math.min(10, st.analysisCache.files.length);
            for (var f = 0; f < maxShow; f++) {
                var file = st.analysisCache.files[f];
                var status = (st.analysisCache.fileStatuses && st.analysisCache.fileStatuses[file]) || '?';
                lines.push('    [' + status + '] ' + truncate(file, (s.width || 80) - 16));
            }
            if (st.analysisCache.files.length > maxShow) {
                lines.push(styles.dim().render(
                    '    ... and ' + (st.analysisCache.files.length - maxShow) + ' more'));
            }
        }

        if (st.analysisCache && st.analysisCache.error) {
            lines.push('');
            lines.push(styles.errorBadge().render(' Error ') + ' ' +
                styles.errorCard().render(st.analysisCache.error));
        }

        // T46: Inline Claude question prompt (during active analysis).
        var questionPrompt = renderClaudeQuestionPrompt(s);
        if (questionPrompt) {
            lines.push(questionPrompt);
        }

        return lines.join('\n');
    }

    // ----- Screen 3: Plan Review (PLAN_REVIEW) -----

    function viewPlanReviewScreen(s) {
        var lines = [];

        if (!st.planCache) {
            lines.push(styles.warningBadge().render(' No Plan ') +
                ' Run analysis first.');
            return lines.join('\n');
        }

        var plan = st.planCache;
        var mode = layoutMode(s);
        lines.push(styles.bold().render('Split Plan Overview'));
        lines.push('  Splits: ' + styles.fieldValue().render(String(plan.splits.length)));
        lines.push('  Base: ' + styles.fieldValue().render(plan.baseBranch || 'main'));

        // T099: Show amber warning when auto-strategy selection has low
        // confidence (score gap < 0.15 between winner and runner-up).
        if (s.strategyNeedsConfirm && s.strategyAlternatives && s.strategyAlternatives.length >= 2) {
            var alts = s.strategyAlternatives;
            var winner = alts[0];
            var runnerUp = alts[1];
            var winPct = Math.round(winner.score * 100);
            var runPct = Math.round(runnerUp.score * 100);
            lines.push('');
            lines.push(styles.warningBadge().render(' Low Confidence ') +
                ' Auto-selected ' + styles.bold().render(winner.name) + ' (' + winPct + '%)' +
                ' \u2014 ' + styles.dim().render(runnerUp.name + ' (' + runPct + '%) scored similarly.'));
            if (s.autoStrategyName) {
                lines.push('  Strategy: ' + styles.fieldValue().render(s.autoStrategyName));
            }
        }

        lines.push('');

        var w = (s.width || 80) - 8;
        var selectedIdx = s.selectedSplitIdx || 0;
        var focusIdx = s.focusIndex || 0;
        var splitCount = plan.splits.length;

        if (mode === 'wide' && splitCount > 0) {
            // Wide: side-by-side — compact card list (left) + detail (right).
            var leftW = Math.min(Math.floor(w * 0.35), 40);
            var rightW = w - leftW - 3; // 3 for separator column.
            var leftLines = [];
            var rightLines = [];

            // Left: compact card summary list.
            for (var i = 0; i < plan.splits.length; i++) {
                var split = plan.splits[i];
                var isSelected = (i === selectedIdx);
                var isFocused = (focusIdx === i);
                var bullet = isSelected ? styles.primaryButton().render(' \u25b6 ')
                    : '  ' + (i + 1) + '.';
                var nameStr = truncate(split.name || 'split-' + i, leftW - 8);
                var label = isFocused
                    ? styles.statusActive().render(nameStr)
                    : (isSelected ? styles.bold().render(nameStr) : styles.label().render(nameStr));
                var filesStr = styles.dim().render(' (' + split.files.length + ')');
                var cardId = 'split-card-' + i;
                leftLines.push(zone.mark(cardId, bullet + ' ' + label + filesStr));
            }

            // Right: detail for selected split.
            var sel = plan.splits[selectedIdx];
            if (sel) {
                rightLines.push(styles.bold().render(sel.name || 'split-' + selectedIdx));
                if (sel.message) {
                    rightLines.push(styles.dim().render(sel.message));
                }
                rightLines.push(styles.fieldValue().render(
                    sel.files.length + ' file' + (sel.files.length !== 1 ? 's' : '')));
                rightLines.push('');
                if (sel.files) {
                    for (var fi = 0; fi < sel.files.length; fi++) {
                        var fStatus = (plan.fileStatuses && plan.fileStatuses[sel.files[fi]]) || '?';
                        rightLines.push('[' + fStatus + '] ' + truncate(sel.files[fi], rightW - 6));
                    }
                }
            }

            // Join columns with a vertical separator.
            var separator = styles.divider().render('\u2502');
            var leftBlock = leftLines.join('\n');
            var rightBlock = rightLines.join('\n');
            var leftStyled = lipgloss.newStyle().width(leftW).render(leftBlock);
            var rightStyled = lipgloss.newStyle().width(rightW).render(rightBlock);
            lines.push(lipgloss.joinHorizontal(lipgloss.Top,
                leftStyled, ' ' + separator + ' ', rightStyled));
        } else {
            // Standard / Compact: single-column card layout.
            for (var i = 0; i < plan.splits.length; i++) {
                var split = plan.splits[i];
                var isSelected = (i === selectedIdx);
                var isFocused = (focusIdx === i);
                var cardStyle = isFocused ? styles.focusedCard() :
                                isSelected ? styles.activeCard() : styles.inactiveCard();
                var cardId = 'split-card-' + i;

                var cardContent = '';
                cardContent += styles.bold().render(
                    (i + 1) + '. ' + (split.name || 'split-' + i)) + '\n';
                cardContent += styles.dim().render(split.message || '') + '\n';
                cardContent += styles.fieldValue().render(
                    split.files.length + ' file' + (split.files.length !== 1 ? 's' : ''));

                // Show files for selected split.
                if (isSelected && split.files) {
                    cardContent += '\n';
                    for (var fi = 0; fi < split.files.length; fi++) {
                        var fStatus = (plan.fileStatuses && plan.fileStatuses[split.files[fi]]) || '?';
                        cardContent += '\n  [' + fStatus + '] ' + truncate(split.files[fi], w - 10);
                    }
                }

                lines.push(zone.mark(cardId, cardStyle.width(w).render(cardContent)));
            }
        }

        // Action buttons.
        var editFocused = (focusIdx === splitCount);
        var regenFocused = (focusIdx === splitCount + 1);
        var editBtnStyle = editFocused ? styles.focusedSecondaryButton() : styles.secondaryButton();
        var regenBtnStyle = regenFocused ? styles.focusedSecondaryButton() : styles.secondaryButton();
        var askClaudeFocused = (focusIdx === splitCount + 2);
        var askClaudeStyle = askClaudeFocused ? styles.focusedSecondaryButton() : styles.secondaryButton();
        lines.push('');
        if (layoutMode(s) === 'compact') {
            lines.push(zone.mark('plan-edit', editBtnStyle.render('Edit \u270f')));
            lines.push(zone.mark('plan-regenerate', regenBtnStyle.render('Regen \ud83d\udd04')));
            lines.push(zone.mark('ask-claude', askClaudeStyle.render('Ask Claude \ud83e\udd16')));
        } else {
            lines.push(lipgloss.joinHorizontal(lipgloss.Center,
                zone.mark('plan-edit', editBtnStyle.render('Edit Plan \u270f')),
                '  ',
                zone.mark('plan-regenerate', regenBtnStyle.render('Regenerate \ud83d\udd04')),
                '  ',
                zone.mark('ask-claude', askClaudeStyle.render('Ask Claude \ud83e\udd16'))
            ));
        }

        return lines.join('\n');
    }

    // ----- Screen 4: Plan Editor (PLAN_EDITOR) -----

    function viewPlanEditorScreen(s) {
        var lines = [];

        if (!st.planCache) {
            lines.push('No plan to edit.');
            return lines.join('\n');
        }

        lines.push(styles.bold().render('Edit Split Plan'));
        var compact = layoutMode(s) === 'compact';
        if (compact) {
            lines.push(styles.dim().render('Tab: splits  j/k: files  Space: check  e: rename  Shift+\u2191\u2193: move'));
        } else {
            lines.push(styles.dim().render(
                'Tab: cycle splits  j/k: select file  Space: check  ' +
                'e: rename split  Shift+\u2191/\u2193: reorder'));
        }
        lines.push('');

        // Validation error banner (T17).
        var valErrors = s.editorValidationErrors || [];
        if (valErrors.length > 0) {
            lines.push(styles.errorBadge().render(' Validation Errors '));
            for (var ve = 0; ve < valErrors.length; ve++) {
                lines.push('  ' + styles.dim().render('\u2022 ' + valErrors[ve]));
            }
            lines.push('');
        }

        var plan = st.planCache;
        var selectedIdx = s.selectedSplitIdx || 0;
        var selectedFileIdx = s.selectedFileIdx || 0;
        var w = (s.width || 80) - 8;
        var checkedFiles = s.editorCheckedFiles || {};

        for (var i = 0; i < plan.splits.length; i++) {
            var split = plan.splits[i];
            var isSelected = (i === selectedIdx);

            // Split header: badge + name (or inline edit) + file count.
            var badge = isSelected
                ? styles.primaryButton().render(' \u25b6 ')
                : '  ' + (i + 1) + '. ';

            var nameDisplay;
            if (s.editorTitleEditing && s.editorTitleEditingIdx === i) {
                // Inline title edit mode.
                var editText = s.editorTitleText || '';
                nameDisplay = styles.bold().render(editText) +
                    styles.dim().render('\u2588') +
                    styles.dim().render('  (Enter to save, Esc to cancel)');
            } else {
                nameDisplay = styles.bold().render(split.name || 'split-' + i);
            }
            var filesLabel = styles.dim().render(split.files.length + ' file' +
                (split.files.length !== 1 ? 's' : ''));
            var cardId = 'edit-split-' + i;

            lines.push(zone.mark(cardId, badge + ' ' + nameDisplay + '  ' + filesLabel));

            // File list with checkboxes (T17).
            if (isSelected && split.files) {
                for (var fi = 0; fi < split.files.length; fi++) {
                    var fileId = 'edit-file-' + i + '-' + fi;
                    var fileKey = i + '-' + fi;
                    var isChecked = !!checkedFiles[fileKey];
                    var isFileFocused = (fi === selectedFileIdx);

                    var checkbox = isChecked
                        ? styles.successBadge().render('\u2713')
                        : styles.dim().render('\u2610');
                    var filePath = split.files[fi];
                    var fileStyle = isFileFocused
                        ? styles.bold()
                        : styles.dim();

                    lines.push('    ' + zone.mark(fileId,
                        checkbox + ' ' + fileStyle.render(truncate(filePath, w - 10))));
                }

                // File detail panel (T17).
                if (split.files[selectedFileIdx]) {
                    var detailFile = split.files[selectedFileIdx];
                    lines.push('');
                    lines.push('    ' + styles.dim().render('\u2500\u2500\u2500 File Detail \u2500\u2500\u2500'));
                    lines.push('    ' + styles.bold().render('Path: ') +
                        styles.dim().render(detailFile));
                    lines.push('    ' + styles.bold().render('Split: ') +
                        styles.dim().render(split.name || 'split-' + i));
                    lines.push('    ' + styles.bold().render('Position: ') +
                        styles.dim().render((selectedFileIdx + 1) + ' of ' + split.files.length));

                    // Checked file count for this split.
                    var checkedCount = 0;
                    for (var ck = 0; ck < split.files.length; ck++) {
                        if (checkedFiles[i + '-' + ck]) checkedCount++;
                    }
                    if (checkedCount > 0) {
                        lines.push('    ' + styles.bold().render('Checked: ') +
                            styles.dim().render(checkedCount + ' file' +
                                (checkedCount !== 1 ? 's' : '') + ' selected'));
                    }
                }
            }

            // Editor action buttons.
            if (isSelected) {
                // Focus: split cards = 0..N-1; buttons = N, N+1, N+2.
                var focusIdx = s.focusIndex || 0;
                var splitCount = plan.splits.length;
                var moveFocused = (focusIdx === splitCount);
                var renameFocused = (focusIdx === splitCount + 1);
                var mergeFocused = (focusIdx === splitCount + 2);
                var moveBtnStyle = moveFocused ? styles.focusedSecondaryButton() : styles.secondaryButton();
                var renameBtnStyle = renameFocused ? styles.focusedSecondaryButton() : styles.secondaryButton();
                var mergeBtnStyle = mergeFocused ? styles.focusedSecondaryButton() : styles.secondaryButton();
                lines.push('');
                if (compact) {
                    lines.push(zone.mark('editor-move', moveBtnStyle.render('Move')));
                    lines.push(zone.mark('editor-rename', renameBtnStyle.render('Rename')));
                    lines.push(zone.mark('editor-merge', mergeBtnStyle.render('Merge')));
                } else {
                    lines.push(lipgloss.joinHorizontal(lipgloss.Center,
                        zone.mark('editor-move', moveBtnStyle.render('Move File')),
                        '  ',
                        zone.mark('editor-rename', renameBtnStyle.render('Rename Split')),
                        '  ',
                        zone.mark('editor-merge', mergeBtnStyle.render('Merge Splits'))
                    ));
                }
            }
        }

        return lines.join('\n');
    }

    // ----- Screen 5: Execution (BRANCH_BUILDING) -----

    // T378: Sub-functions extracted from viewExecutionScreen to reduce its
    // 322-line body to a slim orchestrator. Each helper renders one distinct
    // section and pushes directly into the supplied `lines` array.

    // Render per-branch creation status icons (error/success/active/pending)
    // and overall progress bar for the branch-creation phase.
    function renderSplitExecutionList(s, lines) {
        var splits = st.planCache.splits;
        var results = s.executionResults || [];
        var currentIdx = s.executingIdx || 0;

        for (var i = 0; i < splits.length; i++) {
            var split = splits[i];
            var result = results[i];
            var icon, statusText;

            if (result && result.error) {
                icon = styles.errorBadge().render(' \u2718 ');
                statusText = styles.errorCard().width((s.width || 80) - 16).render(
                    split.name + '\n' + result.error);
            } else if (result) {
                icon = styles.successBadge().render(' \u2713 ');
                statusText = styles.label().render(split.name) +
                    styles.dim().render(' \u2192 ' + (result.sha ? result.sha.substring(0, 8) : ''));
            } else if (i === currentIdx && s.isProcessing) {
                icon = styles.warningBadge().render(' \u25b6 ');
                var activeLabel = split.name + '...';
                // Show per-branch progress if available from async progressFn.
                if (s.executionProgressMsg && s.executionBranchTotal > 0) {
                    activeLabel = split.name + ' (' + (currentIdx + 1) + '/' + s.executionBranchTotal + ')';
                }
                statusText = styles.statusActive().render(activeLabel);
            } else {
                icon = styles.dim().render(' \u25cb ');
                statusText = styles.dim().render(split.name);
            }

            lines.push('  ' + icon + ' ' + statusText);
        }

        // Overall progress (branch creation phase).
        if (splits.length > 0 && results.length < splits.length) {
            lines.push('');
            var progress = results.length / splits.length;
            lines.push('  ' + renderProgressBar(progress, (s.width || 80) - 8));
        }
    }

    // T107: Show warning when any branches had git-ignored files skipped.
    function renderSkippedFilesWarning(results, lines) {
        var hasSkipped = false;
        for (var ski = 0; ski < results.length; ski++) {
            if (results[ski] && results[ski].skippedFiles && results[ski].skippedFiles.length > 0) {
                if (!hasSkipped) {
                    lines.push('');
                    lines.push(styles.warningBadge().render(' Skipped Files ') +
                        styles.dim().render(' Files matched .gitignore and were excluded:'));
                    hasSkipped = true;
                }
                for (var skf = 0; skf < results[ski].skippedFiles.length; skf++) {
                    lines.push('    ' + styles.dim().render(
                        '\u25cb ' + results[ski].name + ': ' + results[ski].skippedFiles[skf]));
                }
            }
        }
    }

    // Per-branch verification status list (skipped/passed/failed/active/pending)
    // with expandable output for failed branches.
    function renderVerificationStatusList(s, splits, lines) {
        var verifyResults = s.verificationResults || [];
        var verifyIdx = s.verifyingIdx;

        for (var vi = 0; vi < splits.length; vi++) {
            var vr = verifyResults[vi];
            var vicon, vtext;
            var branchName = splits[vi].name;

            if (vr && vr.skipped) {
                vicon = styles.dim().render(' \u2014 ');
                vtext = styles.dim().render(branchName + ' (skipped)');
            } else if (vr && vr.passed) {
                vicon = styles.successBadge().render(' \u2713 ');
                var durationStr = vr.duration ? ' (' + (vr.duration / 1000).toFixed(1) + 's)' : '';
                vtext = styles.label().render(branchName) +
                    styles.dim().render(durationStr);
                // T006: Show override badge for manually passed branches.
                if (vr.manualOverride) {
                    vtext += ' ' + styles.warningBadge().render(' manual \u2713 ');
                }
            } else if (vr && !vr.passed) {
                vicon = styles.errorBadge().render(' \u2718 ');
                var vdurStr = vr.duration ? ' (' + (vr.duration / 1000).toFixed(1) + 's)' : '';
                vtext = styles.label().render(branchName) +
                    styles.dim().render(vdurStr);
                if (vr.preExisting) {
                    vtext += ' ' + styles.warningBadge().render(' pre-existing ');
                }
                // Error summary.
                if (vr.error) {
                    lines.push('  ' + vicon + ' ' + vtext);
                    lines.push('    ' + styles.dim().render(vr.error));
                    // Expandable output.
                    var outputLines = s.verifyOutput && s.verifyOutput[branchName];
                    if (outputLines && outputLines.length > 0) {
                        if (s.expandedVerifyBranch === branchName) {
                            lines.push('    ' + zone.mark('verify-collapse-' + branchName,
                                styles.dim().render('\u25bc Hide Output')));
                            var maxLines = Math.min(outputLines.length, 20);
                            for (var ol = 0; ol < maxLines; ol++) {
                                lines.push('    ' + styles.dim().render(outputLines[ol]));
                            }
                            if (outputLines.length > 20) {
                                lines.push('    ' + styles.dim().render(
                                    '... (' + (outputLines.length - 20) + ' more lines)'));
                            }
                        } else {
                            lines.push('    ' + zone.mark('verify-expand-' + branchName,
                                styles.dim().render('\u25b6 Show Output (' + outputLines.length + ' lines)')));
                        }
                    }
                    continue; // Already pushed icon+text above.
                }
            } else if (vi === verifyIdx && s.isProcessing) {
                vicon = styles.warningBadge().render(' \u25b6 ');
                // T058: Show elapsed time on the active verify branch.
                var activeElapsed = '';
                if (s.verifyElapsedMs > 0) {
                    var activeSecs = Math.floor(s.verifyElapsedMs / 1000);
                    var verifyTimeout = (typeof prSplitConfig !== 'undefined' && prSplitConfig.timeoutMs) ? prSplitConfig.timeoutMs : 0;
                    if (verifyTimeout > 0) {
                        var totalSecs = Math.floor(verifyTimeout / 1000);
                        activeElapsed = ' (' + activeSecs + 's / ' + totalSecs + 's)';
                    } else {
                        activeElapsed = ' (' + activeSecs + 's)';
                    }
                }
                vtext = styles.statusActive().render(branchName + '...' + activeElapsed);
            } else {
                vicon = styles.dim().render(' \u25cb ');
                vtext = styles.dim().render(branchName);
            }

            lines.push('  ' + vicon + ' ' + vtext);
        }
    }

    // T351: Live CaptureSession viewport with ANSI-aware truncation,
    // scrollbar, pause/resume/shell/interrupt controls, and bordered frame.
    function renderLiveVerifyViewport(s, lines) {
        var verifySession = getInteractivePaneSession(s, 'verify');
        if (!verifySession && !s.verifyScreen) { return; }
        lines.push('');
        var liveOutput = s.verifyScreen || '';
        var liveLines = liveOutput.split('\n');
        // Remove trailing empty lines from VTerm screen output.
        // screen() may include ANSI reset codes on empty lines,
        // so we check visual width rather than string equality.
        while (liveLines.length > 0 && lipgloss.width(liveLines[liveLines.length - 1]) === 0) {
            liveLines.pop();
        }

        var viewWidth = Math.max(40, (s.width || 80) - 8);
        var viewHeight = C.INLINE_VIEW_HEIGHT; // content rows inside the border
        // T058: Show elapsed time with timeout progress in viewport title.
        var elapsed = ((s.verifyElapsedMs || (Date.now() - s.activeVerifyStartTime)) / 1000).toFixed(1);
        var vpTimeoutMs = (typeof prSplitConfig !== 'undefined' && prSplitConfig.timeoutMs) ? prSplitConfig.timeoutMs : 0;
        var elapsedSuffix;
        if (vpTimeoutMs > 0) {
            var vpTotalSecs = Math.floor(vpTimeoutMs / 1000);
            elapsedSuffix = elapsed + 's / ' + vpTotalSecs + 's';
        } else {
            elapsedSuffix = elapsed + 's';
        }
        var titleText = ' Verifying: ' + s.activeVerifyBranch + ' (' + elapsedSuffix + ') ';
        // T059: Show paused indicator in viewport title.
        if (s.verifyPaused) {
            titleText = ' \u23f8 PAUSED: ' + s.activeVerifyBranch + ' (' + elapsedSuffix + ') ';
        }

        // Determine visible window (auto-scroll or manual offset).
        var totalLines = liveLines.length;
        var startLine;
        if (s.verifyAutoScroll || s.verifyViewportOffset <= 0) {
            startLine = Math.max(0, totalLines - viewHeight);
        } else {
            startLine = Math.max(0, totalLines - viewHeight - s.verifyViewportOffset);
        }
        var endLine = Math.min(totalLines, startLine + viewHeight);

        // Build viewport content lines with ANSI-aware truncation.
        var contentLines = [];
        for (var vl = startLine; vl < endLine; vl++) {
            var ln = liveLines[vl] || '';
            // T005: Use lipgloss.width for ANSI-aware visual width,
            // and lipgloss maxWidth for ANSI-safe truncation.
            var visualW = lipgloss.width(ln);
            if (visualW > viewWidth - 2) {
                ln = lipgloss.newStyle().maxWidth(viewWidth - 2).render(ln);
            }
            contentLines.push(ln);
        }
        // Pad to fill viewport height.
        while (contentLines.length < viewHeight) {
            contentLines.push('');
        }

        var vpContent = contentLines.join('\n');

        // Scrollbar indicator.
        var scrollIndicator = '';
        if (totalLines > viewHeight) {
            if (s.verifyAutoScroll) {
                scrollIndicator = ' [auto-scroll]';
            } else {
                var scrollPct = Math.round((startLine / Math.max(1, totalLines - viewHeight)) * 100);
                scrollIndicator = ' [' + scrollPct + '%]';
            }
        }

        // Footer with keybinding hints.
        // T351: Only show interactive controls when CaptureSession
        // is active. Fallback (plain text) shows just scroll hint.
        var footer;
        if (verifySession) {
            // T059: Pause/Resume button for verify subprocess.
            var pauseResumeBtn;
            if (s.verifyPaused) {
                pauseResumeBtn = zone.mark('verify-resume',
                    styles.focusedSecondaryButton().render('\u25b6 Resume'));
            } else {
                pauseResumeBtn = zone.mark('verify-pause',
                    styles.secondaryButton().render('\u23f8 Pause'));
            }
            // T007/T338: Open Shell in verify worktree.
            // Disabled on Windows where CaptureSession is unavailable.
            var openShellBtn = '';
            if (s.activeVerifyWorktree) {
                if (typeof prSplit.canSpawnInteractiveShell === 'function' &&
                    !prSplit.canSpawnInteractiveShell()) {
                    openShellBtn = styles.dim().render('\ue795 Shell (Unix only)');
                } else {
                    openShellBtn = zone.mark('verify-open-shell',
                        styles.secondaryButton().render('\ue795 Shell'));
                }
            }
            var interruptHint = zone.mark('verify-interrupt', styles.dim().render(
                'Ctrl+C: Stop  2\u00d7Ctrl+C: Force Kill'));
            var scrollHint = styles.dim().render(
                '\u2191\u2193: Scroll' + scrollIndicator);
            if (openShellBtn) {
                footer = lipgloss.joinHorizontal(lipgloss.Center,
                    scrollHint, '  ', pauseResumeBtn, '  ',
                    openShellBtn, '  ', interruptHint);
            } else {
                footer = lipgloss.joinHorizontal(lipgloss.Center,
                    scrollHint, '  ', pauseResumeBtn, '  ', interruptHint);
            }
        } else {
            // Fallback path — no interactive controls, just scroll.
            footer = styles.dim().render(
                '\u2191\u2193: Scroll' + scrollIndicator + '  (fallback output)');
        }

        // Render bordered viewport using lipgloss.
        // T059: Use dim border when paused to visually indicate suspended state.
        var vpBorderColor = s.verifyPaused ? COLORS.muted : COLORS.warning;
        var vpStyle = lipgloss.newStyle()
            .border(lipgloss.roundedBorder())
            .borderForeground(vpBorderColor)
            .width(viewWidth)
            .padding(0, 1);

        lines.push('  ' + styles.warningBadge().render(titleText));
        lines.push(vpStyle.render(vpContent));
        lines.push('  ' + footer);
    }

    // Verification summary with pass/fail/skip counts and manual override hint.
    function renderVerificationSummary(s, splits, lines) {
        var verifyResults = s.verificationResults || [];
        if (verifyResults.length !== splits.length) { return; }
        var passCount = 0;
        var failCount = 0;
        var skipCount = 0;
        for (var vs = 0; vs < verifyResults.length; vs++) {
            if (verifyResults[vs].skipped) skipCount++;
            else if (verifyResults[vs].passed) passCount++;
            else failCount++;
        }
        lines.push('');
        var summaryLine = '  ' + styles.successBadge().render(' ' + passCount + ' passed ');
        if (failCount > 0) {
            summaryLine += ' ' + styles.errorBadge().render(' ' + failCount + ' failed ');
        }
        if (skipCount > 0) {
            summaryLine += ' ' + styles.dim().render(' ' + skipCount + ' skipped');
        }
        lines.push(summaryLine);
        // T006: Hint for manual override when there are failures.
        if (failCount > 0 && !getInteractivePaneSession(s, 'verify')) {
            lines.push('  ' + styles.dim().render('Press p to mark a failed branch as passed'));
        }
    }

    // Orchestrator — delegates to the sub-functions above.
    function viewExecutionScreen(s) {
        var lines = [];

        lines.push(styles.bold().render('Executing Split Plan'));
        lines.push('');

        if (!st.planCache || !st.planCache.splits) {
            lines.push('No plan to execute.');
            return lines.join('\n');
        }

        var splits = st.planCache.splits;
        var results = s.executionResults || [];

        renderSplitExecutionList(s, lines);
        renderSkippedFilesWarning(results, lines);

        // --- Per-branch verification section ---
        var verifyResults = s.verificationResults || [];
        var verifyIdx = s.verifyingIdx;
        if (verifyIdx >= 0 || verifyResults.length > 0) {
            lines.push('');
            lines.push(styles.bold().render('Verifying Branches'));
            lines.push('');

            renderVerificationStatusList(s, splits, lines);

            // Verification progress bar + live viewport.
            if (verifyResults.length < splits.length && s.isProcessing) {
                renderLiveVerifyViewport(s, lines);
                lines.push('');
                var vProgress = verifyResults.length / splits.length;
                lines.push('  ' + renderProgressBar(vProgress, (s.width || 80) - 8));
            }

            renderVerificationSummary(s, splits, lines);
        }

        // T46: Inline Claude question prompt (during active execution).
        var execQuestionPrompt = renderClaudeQuestionPrompt(s);
        if (execQuestionPrompt) {
            lines.push(execQuestionPrompt);
        }

        return lines.join('\n');
    }

    // ----- Screen 6: Verification (EQUIV_CHECK) -----

    function viewVerificationScreen(s) {
        var lines = [];

        lines.push(styles.bold().render('Verifying Equivalence'));
        lines.push('');

        if (s.isProcessing) {
            // T064: Show which step is currently running.
            var step = (s.equivStep) ? s.equivStep : 'Checking tree hash equivalence';
            lines.push('  ' + styles.warningBadge().render(' \u25b6 ') +
                ' ' + step + '...');
            lines.push('');
            lines.push('  ' + renderProgressBar(0.5, (s.width || 80) - 8));
        } else if (s.equivalenceResult) {
            var equiv = s.equivalenceResult;
            if (equiv.equivalent) {
                lines.push('  ' + styles.successBadge().render(' PASS ') +
                    ' Tree hashes match!');
                lines.push('');
                lines.push(styles.dim().render(
                    '  All splits merge to produce identical content as the source branch.'));
            } else if (equiv.error) {
                lines.push('  ' + styles.errorBadge().render(' ERROR ') + ' ' + equiv.error);
            } else {
                // T118: Fix field names (was equiv.expected/equiv.actual — always undefined).
                lines.push('  ' + styles.errorBadge().render(' FAIL ') +
                    ' Tree hash mismatch');
                if (equiv.splitTree) {
                    lines.push('    Split tree:  ' + styles.fieldValue().render(equiv.splitTree));
                }
                if (equiv.sourceTree) {
                    lines.push('    Source tree: ' + styles.fieldValue().render(equiv.sourceTree));
                }

                // T118/T064: Display diffFiles list when available.
                if (equiv.diffFiles && equiv.diffFiles.length > 0) {
                    lines.push('');
                    lines.push('  ' + styles.bold().render('Files differing from source (' + equiv.diffFiles.length + '):'));
                    var maxShow = 20;
                    for (var d = 0; d < Math.min(equiv.diffFiles.length, maxShow); d++) {
                        lines.push('    ' + styles.dim().render('\u2022 ') + equiv.diffFiles[d]);
                    }
                    if (equiv.diffFiles.length > maxShow) {
                        lines.push(styles.dim().render('    ... and ' + (equiv.diffFiles.length - maxShow) + ' more'));
                    }
                }

                // T118: Display diffSummary if available.
                if (equiv.diffSummary) {
                    lines.push('');
                    lines.push(styles.dim().render('  ' + equiv.diffSummary));
                }

                // T064: "Next steps" hint on failure.
                lines.push('');
                lines.push(styles.dim().render('  Possible causes: files moved between splits, uncommitted changes,'));
                lines.push(styles.dim().render('  or merge conflicts during branch creation. Try re-verifying or'));
                lines.push(styles.dim().render('  revising the split plan.'));
            }

            // T064: Skipped branches with detail.
            if (equiv.skippedBranches && equiv.skippedBranches.length > 0) {
                lines.push('');
                lines.push(styles.warningBadge().render(' Note ') +
                    ' Skipped ' + equiv.skippedBranches.length + ' branch(es):');
                for (var sk = 0; sk < equiv.skippedBranches.length; sk++) {
                    var branch = equiv.skippedBranches[sk];
                    if (typeof branch === 'object' && branch.name) {
                        lines.push('    ' + styles.dim().render('\u2022 ') + branch.name +
                            (branch.reason ? styles.dim().render(' \u2014 ' + branch.reason) : ''));
                    } else {
                        lines.push('    ' + styles.dim().render('\u2022 ') + String(branch));
                    }
                }
            }

            // T061: Action buttons (visible when not processing).
            if (!equiv.equivalent) {
                lines.push('');
                var zone = prSplit._modules.zone;
                // T300: Compute focused element for focus-aware button styling.
                var focusElems = prSplit._getFocusElements ? prSplit._getFocusElements(s) : [];
                var focusIdx = s.focusIndex || 0;
                var focusedElemId = (focusElems[focusIdx] || {}).id || '';
                var reverifyStyle = (focusedElemId === 'equiv-reverify')
                    ? styles.focusedSecondaryButton() : styles.secondaryButton();
                var reviseStyle = (focusedElemId === 'equiv-revise')
                    ? styles.focusedSecondaryButton() : styles.secondaryButton();
                var continueStyle = (focusedElemId === 'nav-next')
                    ? styles.focusedButton() : styles.primaryButton();
                if (zone) {
                    lines.push('  ' + lipgloss.joinHorizontal(lipgloss.Center,
                        zone.mark('equiv-reverify', reverifyStyle.render(' Re-verify ')),
                        '  ',
                        zone.mark('equiv-revise', reviseStyle.render(' Revise Plan ')),
                        '  ',
                        zone.mark('nav-next', continueStyle.render(' Continue \u25b6 '))));
                } else {
                    lines.push('  ' + lipgloss.joinHorizontal(lipgloss.Center,
                        reverifyStyle.render(' Re-verify '),
                        '  ',
                        reviseStyle.render(' Revise Plan '),
                        '  ',
                        continueStyle.render(' Continue \u25b6 ')));
                }
            }
        }

        return lines.join('\n');
    }

    // --- Exports ---
    prSplit._viewConfigScreen = viewConfigScreen;
    prSplit._viewAnalysisScreen = viewAnalysisScreen;
    prSplit._viewPlanReviewScreen = viewPlanReviewScreen;
    prSplit._viewPlanEditorScreen = viewPlanEditorScreen;
    prSplit._viewExecutionScreen = viewExecutionScreen;
    prSplit._viewVerificationScreen = viewVerificationScreen;
    // T378: Exported sub-functions for unit testing.
    prSplit._renderSplitExecutionList = renderSplitExecutionList;
    prSplit._renderSkippedFilesWarning = renderSkippedFilesWarning;
    prSplit._renderVerificationStatusList = renderVerificationStatusList;
    prSplit._renderLiveVerifyViewport = renderLiveVerifyViewport;
    prSplit._renderVerificationSummary = renderVerificationSummary;

})(globalThis.prSplit);
