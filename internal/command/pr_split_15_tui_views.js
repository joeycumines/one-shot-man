'use strict';
// pr_split_15_tui_views.js — TUI: BubbleTea library imports, colors, styles, chrome, screen views
// Dependencies: chunks 00-14 must be loaded first.

(function(prSplit) {

    if (typeof tui === 'undefined' || typeof ctx === 'undefined' ||
        typeof output === 'undefined') { return; }

    var st = prSplit._state;

    var tea = require('osm:bubbletea');
    var lipgloss = require('osm:lipgloss');
    var zone = require('osm:bubblezone');
    var viewportLib = require('osm:bubbles/viewport');
    var scrollbarLib = require('osm:termui/scrollbar');

    // -----------------------------------------------------------------------
    //  COLORS & Styles (T006)
    //
    //  Design spec: docs/pr-split-tui-design.md §2
    // -----------------------------------------------------------------------

    // Adaptive color palette: auto-detects light/dark terminal background.
    // Uses {light, dark} objects resolved by lipgloss.AdaptiveColor.
    // Palette: high-contrast, distinct hues, WCAG AA compliant.
    var COLORS = {
        primary:   {light: '#6D28D9', dark: '#A78BFA'},  // Purple accent
        secondary: {light: '#4338CA', dark: '#818CF8'},  // Indigo
        success:   {light: '#15803D', dark: '#4ADE80'},  // Green
        warning:   {light: '#A16207', dark: '#FACC15'},  // Amber
        error:     {light: '#DC2626', dark: '#F87171'},  // Red
        muted:     {light: '#6B7280', dark: '#9CA3AF'},  // Gray
        surface:   {light: '#F3F4F6', dark: '#1F2937'},  // Card bg
        border:    {light: '#D1D5DB', dark: '#4B5563'},  // Borders
        text:      {light: '#111827', dark: '#F9FAFB'},  // Primary text
        textDim:   {light: '#6B7280', dark: '#9CA3AF'}   // Secondary text
    };

    // Resolve adaptive color to a plain string (for APIs that don't support objects).
    function resolveColor(c) {
        if (typeof c === 'string') return c;
        if (c && typeof c === 'object' && c.light && c.dark) {
            return lipgloss.hasDarkBackground() ? c.dark : c.light;
        }
        return '';
    }

    var styles = {
        titleBar: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground(COLORS.text)
                .background(COLORS.primary)
                .padding(0, 1);
        },
        stepIndicator: function() {
            return lipgloss.newStyle()
                .foreground(COLORS.textDim);
        },
        activeCard: function() {
            return lipgloss.newStyle()
                .border(lipgloss.roundedBorder())
                .borderForeground(COLORS.primary)
                .padding(1, 2);
        },
        inactiveCard: function() {
            return lipgloss.newStyle()
                .border(lipgloss.roundedBorder())
                .borderForeground(COLORS.border)
                .padding(1, 2);
        },
        errorCard: function() {
            return lipgloss.newStyle()
                .border(lipgloss.normalBorder())
                .borderForeground(COLORS.error)
                .padding(1, 2);
        },
        successBadge: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground('#000000')
                .background(COLORS.success)
                .padding(0, 1);
        },
        warningBadge: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground('#000000')
                .background(COLORS.warning)
                .padding(0, 1);
        },
        errorBadge: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground('#000000')
                .background(COLORS.error)
                .padding(0, 1);
        },
        primaryButton: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground(COLORS.text)
                .background(COLORS.primary)
                .padding(0, 2);
        },
        secondaryButton: function() {
            return lipgloss.newStyle()
                .foreground(COLORS.text)
                .background(COLORS.surface)
                .padding(0, 2)
                .border(lipgloss.roundedBorder())
                .borderForeground(COLORS.border);
        },
        disabledButton: function() {
            return lipgloss.newStyle()
                .foreground(COLORS.muted)
                .background(COLORS.surface)
                .padding(0, 2);
        },
        progressFull: function() {
            return lipgloss.newStyle()
                .foreground(COLORS.success);
        },
        progressEmpty: function() {
            return lipgloss.newStyle()
                .foreground(COLORS.border);
        },
        divider: function() {
            return lipgloss.newStyle()
                .foreground(COLORS.border);
        },
        dim: function() {
            return lipgloss.newStyle()
                .foreground(COLORS.textDim);
        },
        bold: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground(COLORS.text);
        },
        label: function() {
            return lipgloss.newStyle()
                .foreground(COLORS.text);
        },
        fieldValue: function() {
            return lipgloss.newStyle()
                .foreground(COLORS.secondary);
        },
        statusIdle: function() {
            return lipgloss.newStyle()
                .foreground(COLORS.muted);
        },
        statusActive: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground(COLORS.warning);
        }
    };

    // Export styles for test access.
    prSplit._wizardStyles = styles;
    prSplit._wizardColors = COLORS;

    // -----------------------------------------------------------------------
    //  Chrome Renderers (T007)
    //
    //  Title bar, navigation bar, status bar, step dots
    // -----------------------------------------------------------------------

    var STEP_LABELS = [
        'Configure',
        'Analysis',
        'Review Plan',
        'Edit Plan',
        'Execution',
        'Verification',
        'Finalization'
    ];

    // Map wizard states to step indices (0-based).
    var STATE_TO_STEP = {
        'IDLE':             0,
        'CONFIG':           0,
        'BASELINE_FAIL':    0,
        'PLAN_GENERATION':  1,
        'PLAN_REVIEW':      2,
        'PLAN_EDITOR':      3,
        'BRANCH_BUILDING':  4,
        'ERROR_RESOLUTION': 4,
        'EQUIV_CHECK':      5,
        'FINALIZATION':     6,
        'DONE':             6,
        'CANCELLED':        6,
        'FORCE_CANCEL':     6,
        'PAUSED':           6,
        'ERROR':            6
    };

    function renderTitleBar(s) {
        var stepIdx = STATE_TO_STEP[s.wizardState] || 0;
        var stepLabel = STEP_LABELS[stepIdx] || 'Unknown';
        var stepNum = stepIdx + 1;
        var totalSteps = 7;

        // Elapsed time.
        var elapsed = s.startTime ? Math.floor((Date.now() - s.startTime) / 1000) : 0;
        var mins = Math.floor(elapsed / 60);
        var secs = elapsed % 60;
        var timeStr = mins + ':' + (secs < 10 ? '0' : '') + secs;

        var left = styles.titleBar().render('\ud83d\udd00 PR Split Wizard');
        var right = styles.stepIndicator().render(
            'Step ' + stepNum + '/' + totalSteps + ': ' + stepLabel + '  \u23f1 ' + timeStr
        );

        var w = s.width || 80;
        var leftW = lipgloss.width(left);
        var rightW = lipgloss.width(right);
        var gap = Math.max(1, w - leftW - rightW);
        var gapStr = '';
        for (var i = 0; i < gap; i++) gapStr += ' ';

        return left + gapStr + right;
    }

    function renderStepDots(s) {
        var stepIdx = STATE_TO_STEP[s.wizardState] || 0;
        var dots = '';
        for (var i = 0; i < 7; i++) {
            if (i <= stepIdx) {
                dots += styles.progressFull().render('\u25cf');
            } else {
                dots += styles.progressEmpty().render('\u25cb');
            }
        }
        return dots;
    }

    function renderNavBar(s) {
        var stepIdx = STATE_TO_STEP[s.wizardState] || 0;
        var w = s.width || 80;
        var narrow = w < 50;

        // Back button (not on first screen).
        var backBtn = '';
        if (stepIdx > 0 && !s.isProcessing) {
            var backLabel = narrow ? '\u2190' : '\u2190 Back';
            backBtn = zone.mark('nav-back',
                styles.secondaryButton().render(backLabel));
        }

        // Cancel button.
        var cancelLabel = narrow ? '\u00d7' : 'Cancel';
        var cancelBtn = zone.mark('nav-cancel',
            styles.secondaryButton().render(cancelLabel));

        // Dots.
        var dots = renderStepDots(s);

        // Next/action button.
        var nextBtn = '';
        if (s.isProcessing) {
            nextBtn = styles.disabledButton().render(narrow ? '...' : 'Processing...');
        } else {
            var nextLabel = getNextButtonLabel(s);
            if (nextLabel) {
                if (narrow && nextLabel.length > 8) {
                    nextLabel = nextLabel.split(' ')[0];
                }
                nextBtn = zone.mark('nav-next',
                    styles.primaryButton().render(nextLabel + ' \u2192'));
            }
        }

        // Build left section (back + cancel).
        var leftParts = [];
        if (backBtn) leftParts.push(backBtn);
        leftParts.push(cancelBtn);
        var leftSection = leftParts.length > 1
            ? lipgloss.joinHorizontal(lipgloss.Center, leftParts[0], '  ', leftParts[1])
            : leftParts[0];

        // Build right section (dots + next).
        var rightParts = [dots];
        if (nextBtn) rightParts.push(nextBtn);
        var rightSection = rightParts.length > 1
            ? lipgloss.joinHorizontal(lipgloss.Center, rightParts[0], '  ', rightParts[1])
            : rightParts[0];

        // Compose full nav bar: left ... right, spread across width.
        var leftW = lipgloss.width(leftSection);
        var rightW = lipgloss.width(rightSection);
        var gap = Math.max(2, w - leftW - rightW);
        var spacer = lipgloss.newStyle().width(gap).render('');

        return lipgloss.joinHorizontal(lipgloss.Center, leftSection, spacer, rightSection);
    }

    function getNextButtonLabel(s) {
        switch (s.wizardState) {
            case 'IDLE': case 'CONFIG':
                return 'Start Analysis';
            case 'PLAN_REVIEW':
                return 'Execute Plan';
            case 'PLAN_EDITOR':
                return 'Save & Review';
            case 'ERROR_RESOLUTION':
                return 'Retry';
            case 'FINALIZATION':
                return 'Finish';
            default:
                return '';
        }
    }

    function renderStatusBar(s) {
        var w = s.width || 80;
        var narrow = w < 60;
        var veryNarrow = w < 40;

        // Left: termmux toggle hint.
        var left = styles.dim().render(veryNarrow ? 'C-]' : 'Ctrl+] Claude');

        // Center: help.
        var center = veryNarrow ? '' : styles.dim().render('? Help');

        // Right: Claude process status.
        var claudeStatus = getClaudeStatusText(s);
        var right = narrow ? '' : zone.mark('claude-status', claudeStatus);

        // Build status line with guaranteed minimum spacing.
        var items = [left];
        if (center) items.push(center);
        if (right) items.push(right);

        var totalItemW = 0;
        for (var ii = 0; ii < items.length; ii++) {
            totalItemW += lipgloss.width(items[ii]);
        }

        var statusLine;
        if (items.length === 1) {
            statusLine = items[0];
        } else {
            // Distribute remaining space evenly between items.
            var gapCount = items.length - 1;
            var remaining = Math.max(gapCount * 2, w - totalItemW);
            var perGap = Math.floor(remaining / gapCount);
            var parts = items[0];
            for (var ix = 1; ix < items.length; ix++) {
                var g = '';
                for (var gi = 0; gi < perGap; gi++) g += ' ';
                parts += g + items[ix];
            }
            statusLine = parts;
        }

        return styles.dim().render(
            styles.divider().render(repeatStr('\u2500', w))
        ) + '\n' + statusLine;
    }

    function getClaudeStatusText(s) {
        if (typeof tuiMux === 'undefined' || !tuiMux ||
            typeof tuiMux.lastActivityMs !== 'function') {
            return styles.statusIdle().render('\ud83d\udca4 Claude: N/A');
        }
        var ms = tuiMux.lastActivityMs();
        if (ms < 0) return styles.statusIdle().render('\u23f8\ufe0f Claude: no output');
        if (ms < 2000) return styles.statusActive().render('\ud83d\udd04 Claude: LIVE');
        if (ms < 10000) return styles.statusIdle().render('\u23f3 Claude: idle (' + Math.round(ms / 1000) + 's)');
        return styles.statusIdle().render('\ud83d\udca4 Claude: quiet');
    }

    // Export chrome for testing.
    prSplit._renderTitleBar = renderTitleBar;
    prSplit._renderNavBar = renderNavBar;
    prSplit._renderStatusBar = renderStatusBar;
    prSplit._renderStepDots = renderStepDots;

    // -----------------------------------------------------------------------
    //  Helpers (T008)
    //
    //  Progress bar, truncation, padding
    // -----------------------------------------------------------------------

    function renderProgressBar(percent, width) {
        var barW = Math.max(10, (width || 40) - 10);
        var filled = Math.round(barW * Math.min(1, Math.max(0, percent)));
        var empty = barW - filled;
        var bar = styles.progressFull().render(repeatStr('\u2588', filled)) +
                  styles.progressEmpty().render(repeatStr('\u2591', empty));
        var pctStr = Math.round(percent * 100) + '%';
        return bar + '  ' + pctStr;
    }

    function truncate(str, maxLen) {
        if (!str) return '';
        if (str.length <= maxLen) return str;
        return str.substring(0, maxLen - 3) + '...';
    }

    function padRight(str, width) {
        str = str || '';
        while (str.length < width) str += ' ';
        return str;
    }

    function repeatStr(ch, n) {
        var s = '';
        for (var i = 0; i < n; i++) s += ch;
        return s;
    }

    prSplit._renderProgressBar = renderProgressBar;

    // -----------------------------------------------------------------------
    //  Screen Renderers (T009-T017)
    //
    //  Each screen returns a string for the content area.
    //  The update function sets model state; view calls the appropriate
    //  screen renderer based on wizardState.
    // -----------------------------------------------------------------------

    // ----- Screen 1: Configuration (IDLE / CONFIG) -----

    function viewConfigScreen(s) {
        var runtime = prSplit.runtime;
        var lines = [];

        // Repository info.
        lines.push(styles.bold().render('Repository'));
        lines.push('  ' + styles.fieldValue().render(runtime.dir || '.'));
        lines.push('');

        // Config form (non-editable display for now, wizard sets these).
        lines.push(styles.bold().render('Source Branch'));
        var srcBranch = (st.analysisCache && st.analysisCache.currentBranch) || '(auto-detect)';
        lines.push('  ' + styles.activeCard().width(Math.max(20, (s.width || 80) - 12)).render(
            styles.fieldValue().render(srcBranch)
        ));
        lines.push('');

        lines.push(styles.bold().render('Target Branch'));
        lines.push('  ' + styles.activeCard().width(Math.max(20, (s.width || 80) - 12)).render(
            styles.fieldValue().render(runtime.baseBranch || 'main')
        ));
        lines.push('');

        // Strategy selection.
        lines.push(styles.bold().render('Strategy'));
        var strategies = ['auto', 'heuristic', 'directory'];
        var currentMode = runtime.mode || 'heuristic';
        for (var si = 0; si < strategies.length; si++) {
            var strat = strategies[si];
            var selected = (strat === currentMode);
            var bullet = selected ? styles.primaryButton().render(' \u25cf ') : '  \u25cb ';
            var label = styles.label().render(strat.charAt(0).toUpperCase() + strat.slice(1));
            var stratId = 'strategy-' + strat;
            lines.push('  ' + zone.mark(stratId, bullet + ' ' + label));
        }
        lines.push('');

        // Advanced options toggle.
        if (s.showAdvanced) {
            lines.push(styles.dim().render('\u25be Advanced Options'));
            lines.push('  Max files per chunk: ' +
                styles.fieldValue().render(String(runtime.maxFiles || 10)));
            lines.push('  Branch prefix:       ' +
                styles.fieldValue().render(runtime.branchPrefix || 'split/'));
            lines.push('  Verify command:      ' +
                styles.fieldValue().render(runtime.verifyCommand || 'true'));
            lines.push('  ' + (runtime.dryRun ? '\u2611' : '\u2610') + ' Dry run');
        } else {
            lines.push(zone.mark('toggle-advanced',
                styles.dim().render('\u25b8 Advanced Options')));
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
        lines.push(styles.bold().render('Split Plan Overview'));
        lines.push('  Splits: ' + styles.fieldValue().render(String(plan.splits.length)));
        lines.push('  Base: ' + styles.fieldValue().render(plan.baseBranch || 'main'));
        lines.push('');

        // Split cards.
        var w = (s.width || 80) - 8;
        var selectedIdx = s.selectedSplitIdx || 0;

        for (var i = 0; i < plan.splits.length; i++) {
            var split = plan.splits[i];
            var isSelected = (i === selectedIdx);
            var cardStyle = isSelected ? styles.activeCard() : styles.inactiveCard();
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

        // Action buttons.
        lines.push('');
        lines.push(
            zone.mark('plan-edit', styles.secondaryButton().render('Edit Plan \u270f')) +
            '  ' +
            zone.mark('plan-regenerate', styles.secondaryButton().render('Regenerate \ud83d\udd04'))
        );

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
        lines.push(styles.dim().render(
            'Click a split to select. Use Move/Rename buttons to modify.'));
        lines.push('');

        var plan = st.planCache;
        var selectedIdx = s.selectedSplitIdx || 0;
        var w = (s.width || 80) - 8;

        for (var i = 0; i < plan.splits.length; i++) {
            var split = plan.splits[i];
            var isSelected = (i === selectedIdx);
            var badge = isSelected ? styles.primaryButton().render(' \u25b6 ') : '  ' + (i + 1) + '. ';
            var name = styles.bold().render(split.name || 'split-' + i);
            var files = styles.dim().render(split.files.length + ' files');
            var cardId = 'edit-split-' + i;

            lines.push(zone.mark(cardId, badge + ' ' + name + '  ' + files));

            if (isSelected && split.files) {
                for (var fi = 0; fi < split.files.length; fi++) {
                    var fileId = 'edit-file-' + i + '-' + fi;
                    lines.push('    ' + zone.mark(fileId,
                        styles.dim().render(split.files[fi])));
                }
            }
        }

        if (isSelected) {
            lines.push('');
            lines.push(
                zone.mark('editor-move', styles.secondaryButton().render('Move File')) +
                '  ' +
                zone.mark('editor-rename', styles.secondaryButton().render('Rename Split')) +
                '  ' +
                zone.mark('editor-merge', styles.secondaryButton().render('Merge Splits'))
            );
        }

        return lines.join('\n');
    }

    // ----- Screen 5: Execution (BRANCH_BUILDING) -----

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
                statusText = styles.statusActive().render(split.name + '...');
            } else {
                icon = styles.dim().render(' \u25cb ');
                statusText = styles.dim().render(split.name);
            }

            lines.push('  ' + icon + ' ' + statusText);
        }

        // Overall progress.
        if (splits.length > 0) {
            lines.push('');
            var progress = results.length / splits.length;
            lines.push('  ' + renderProgressBar(progress, (s.width || 80) - 8));
        }

        return lines.join('\n');
    }

    // ----- Screen 6: Verification (EQUIV_CHECK) -----

    function viewVerificationScreen(s) {
        var lines = [];

        lines.push(styles.bold().render('Verifying Equivalence'));
        lines.push('');

        if (s.isProcessing) {
            lines.push('  ' + styles.warningBadge().render(' \u25b6 ') +
                ' Checking tree hash equivalence...');
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
                lines.push('  ' + styles.errorBadge().render(' FAIL ') +
                    ' Tree hash mismatch');
                if (equiv.expected) {
                    lines.push('    Expected: ' + styles.fieldValue().render(equiv.expected));
                }
                if (equiv.actual) {
                    lines.push('    Actual:   ' + styles.fieldValue().render(equiv.actual));
                }
            }

            // Skipped branches note.
            if (equiv.skippedBranches && equiv.skippedBranches.length > 0) {
                lines.push('');
                lines.push(styles.warningBadge().render(' Note ') +
                    ' Skipped ' + equiv.skippedBranches.length + ' branch(es)');
            }
        }

        return lines.join('\n');
    }

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

        // Actions.
        lines.push('');
        lines.push(
            zone.mark('final-report',
                styles.secondaryButton().render('View Report')) +
            '  ' +
            zone.mark('final-create-prs',
                styles.primaryButton().render('Create PRs')) +
            '  ' +
            zone.mark('final-done',
                styles.primaryButton().render('Done'))
        );

        return lines.join('\n');
    }

    // ----- Error Resolution Overlay (T018) -----

    function viewErrorResolutionScreen(s) {
        var lines = [];

        lines.push(styles.errorBadge().render(' Error Resolution '));
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

        // Resolution options.
        lines.push(styles.bold().render('Choose Resolution:'));
        lines.push('');
        lines.push('  ' + zone.mark('resolve-auto',
            styles.primaryButton().render('Auto-Resolve')) +
            styles.dim().render('  Let Claude fix the issues'));
        lines.push('');
        lines.push('  ' + zone.mark('resolve-manual',
            styles.secondaryButton().render('Manual Fix')) +
            styles.dim().render('  Switch to Claude pane to fix manually'));
        lines.push('');
        lines.push('  ' + zone.mark('resolve-skip',
            styles.secondaryButton().render('Skip')) +
            styles.dim().render('  Skip failed branches'));
        lines.push('');
        lines.push('  ' + zone.mark('resolve-retry',
            styles.secondaryButton().render('Retry')) +
            styles.dim().render('  Regenerate plan from scratch'));
        lines.push('');
        lines.push('  ' + zone.mark('resolve-abort',
            styles.secondaryButton().render('Abort')) +
            styles.dim().render('  Cancel the split'));

        return lines.join('\n');
    }

    // ----- Help Overlay (T019) -----

    function viewHelpOverlay(s) {
        var w = Math.min(60, (s.width || 80) - 4);
        var lines = [];

        lines.push(styles.bold().render('Keyboard Shortcuts'));
        lines.push('');
        lines.push(padRight('  ? / F1', 20) + 'Toggle this help');
        lines.push(padRight('  Tab', 20) + 'Next field / option');
        lines.push(padRight('  Shift+Tab', 20) + 'Previous field / option');
        lines.push(padRight('  Enter', 20) + 'Confirm / select');
        lines.push(padRight('  Esc', 20) + 'Back / close overlay');
        lines.push(padRight('  Ctrl+C', 20) + 'Cancel wizard');
        lines.push(padRight('  Ctrl+]', 20) + 'Toggle Claude pane');
        lines.push(padRight('  j / \u2193', 20) + 'Move down');
        lines.push(padRight('  k / \u2191', 20) + 'Move up');
        lines.push(padRight('  PgUp / PgDn', 20) + 'Scroll page');
        lines.push(padRight('  Home / End', 20) + 'Jump to top / bottom');

        var content = lines.join('\n');
        return styles.activeCard().width(w).render(content);
    }

    // ----- Confirm Cancel Overlay -----

    function viewConfirmCancelOverlay(s) {
        var w = Math.min(50, (s.width || 80) - 4);
        var lines = [];

        lines.push(styles.warningBadge().render(' Cancel Wizard? '));
        lines.push('');
        lines.push('Are you sure you want to cancel the PR split?');
        lines.push('All progress will be lost.');
        lines.push('');
        lines.push(
            zone.mark('confirm-yes',
                styles.errorBadge().render(' Yes, Cancel ')) +
            '    ' +
            zone.mark('confirm-no',
                styles.primaryButton().render(' No, Continue '))
        );

        var content = lines.join('\n');
        return styles.activeCard().width(w).render(content);
    }

    // ----- Report Overlay -----

    function viewReportOverlay(s) {
        var overlayW = Math.min(72, (s.width || 80) - 6);
        var overlayH = Math.max(8, (s.height || 24) - 6);

        // Title bar.
        var titleLine = styles.bold().render('  Split Report');
        var hintLine = styles.dim().render(
            '  j/k scroll • PgUp/PgDn page • c copy • Esc close');

        // Use the dedicated report viewport + scrollbar.
        var vpH = Math.max(3, overlayH - 4); // Reserve lines for title + hints + borders.
        if (s.reportVp) {
            s.reportVp.setWidth(Math.max(10, overlayW - 4)); // account for card padding + scrollbar
            s.reportVp.setHeight(vpH);
            // Content is already set via s.reportVp.setContent() in the click handler.
        }

        var vpView = s.reportVp ? s.reportVp.view() : (s.reportContent || '');
        var sbView = '';
        if (s.reportSb && s.reportVp) {
            s.reportSb.setViewportHeight(vpH);
            s.reportSb.setContentHeight(s.reportVp.totalLineCount());
            s.reportSb.setYOffset(s.reportVp.yOffset());
            s.reportSb.setChars('\u2588', '\u2591');
            s.reportSb.setThumbForeground(resolveColor(COLORS.primary));
            s.reportSb.setTrackForeground(resolveColor(COLORS.border));
            sbView = s.reportSb.view();
        }

        var scrollContent = lipgloss.joinHorizontal(lipgloss.Top, vpView, sbView);

        var inner = [titleLine, '', scrollContent, '', hintLine].join('\n');
        return styles.activeCard().width(overlayW).render(inner);
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
        lines.push(
            zone.mark('move-confirm', styles.primaryButton().render(' Move ')) +
            '  ' +
            zone.mark('move-cancel', styles.secondaryButton().render(' Cancel '))
        );

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

        lines.push('');
        lines.push(styles.dim().render('Type to edit \u2022 Enter confirm \u2022 Esc cancel'));
        lines.push('');
        lines.push(
            zone.mark('rename-confirm', styles.primaryButton().render(' Rename ')) +
            '  ' +
            zone.mark('rename-cancel', styles.secondaryButton().render(' Cancel '))
        );

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
        lines.push(
            zone.mark('merge-confirm', styles.primaryButton().render(' Merge ')) +
            '  ' +
            zone.mark('merge-cancel', styles.secondaryButton().render(' Cancel '))
        );

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
            case 'ERROR':
                return styles.errorBadge().render(' Error ') +
                    '\n\n' + (s.errorDetails || 'An unexpected error occurred.');
            default:
                return 'Unknown state: ' + s.wizardState;
        }
    }

    // Export screen renderers for testing.
    prSplit._viewConfigScreen = viewConfigScreen;
    prSplit._viewAnalysisScreen = viewAnalysisScreen;
    prSplit._viewPlanReviewScreen = viewPlanReviewScreen;
    prSplit._viewPlanEditorScreen = viewPlanEditorScreen;
    prSplit._viewExecutionScreen = viewExecutionScreen;
    prSplit._viewVerificationScreen = viewVerificationScreen;
    prSplit._viewFinalizationScreen = viewFinalizationScreen;
    prSplit._viewErrorResolutionScreen = viewErrorResolutionScreen;
    prSplit._viewHelpOverlay = viewHelpOverlay;
    prSplit._viewConfirmCancelOverlay = viewConfirmCancelOverlay;
    prSplit._viewReportOverlay = viewReportOverlay;
    prSplit._viewMoveFileDialog = viewMoveFileDialog;
    prSplit._viewRenameSplitDialog = viewRenameSplitDialog;
    prSplit._viewMergeSplitsDialog = viewMergeSplitsDialog;
    prSplit._viewForState = viewForState;

    // Cross-chunk exports — libraries and utilities for subsequent chunks.
    prSplit._tea = tea;
    prSplit._lipgloss = lipgloss;
    prSplit._zone = zone;
    prSplit._viewportLib = viewportLib;
    prSplit._scrollbarLib = scrollbarLib;
    prSplit._COLORS = COLORS;
    prSplit._resolveColor = resolveColor;
    prSplit._repeatStr = repeatStr;

})(globalThis.prSplit);
