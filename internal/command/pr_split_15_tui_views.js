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
    // WCAG AA compliant: all text-on-background pairs >= 4.5:1 contrast.
    // textOnColor is the inverse text for colored (non-surface) backgrounds.
    var COLORS = {
        primary:     {light: '#6D28D9', dark: '#A78BFA'},  // Purple accent
        secondary:   {light: '#4338CA', dark: '#818CF8'},  // Indigo
        success:     {light: '#15803D', dark: '#4ADE80'},  // Green
        warning:     {light: '#A16207', dark: '#FACC15'},  // Amber
        error:       {light: '#DC2626', dark: '#F87171'},  // Red
        muted:       {light: '#6B7280', dark: '#9CA3AF'},  // Gray
        surface:     {light: '#F3F4F6', dark: '#1F2937'},  // Card bg
        border:      {light: '#D1D5DB', dark: '#4B5563'},  // Borders
        text:        {light: '#111827', dark: '#F9FAFB'},  // Primary text
        textDim:     {light: '#6B7280', dark: '#9CA3AF'},  // Secondary text
        textOnColor: {light: '#FFFFFF', dark: '#000000'}   // Text on colored bg (WCAG AA)
    };

    // Braille spinner frames for processing animation (T051).
    var SPINNER_FRAMES = ['\u280b', '\u2819', '\u2839', '\u2838', '\u283c', '\u2834', '\u2826', '\u2827', '\u2807', '\u280f'];

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
                .foreground(COLORS.textOnColor)
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
                .foreground(COLORS.textOnColor)
                .background(COLORS.success)
                .padding(0, 1);
        },
        warningBadge: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground(COLORS.textOnColor)
                .background(COLORS.warning)
                .padding(0, 1);
        },
        errorBadge: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground(COLORS.textOnColor)
                .background(COLORS.error)
                .padding(0, 1);
        },
        primaryButton: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground(COLORS.textOnColor)
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
        statusQuiet: function() {
            return lipgloss.newStyle()
                .foreground(COLORS.textDim);
        },
        statusActive: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground(COLORS.warning);
        },
        focusedCard: function() {
            return lipgloss.newStyle()
                .border(lipgloss.doubleBorder())
                .borderForeground(COLORS.warning)
                .padding(1, 2);
        },
        focusedButton: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground(COLORS.textOnColor)
                .background(COLORS.warning)
                .padding(0, 2);
        },
        // T011: Width-stable focus style for secondaryButton elements.
        // secondaryButton has roundedBorder() (adds +2 H chars, +2 V lines).
        // focusedSecondaryButton ALSO has roundedBorder() so the dimensions
        // remain identical when focus toggles between the two.
        focusedSecondaryButton: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground(COLORS.textOnColor)
                .background(COLORS.warning)
                .padding(0, 2)
                .border(lipgloss.roundedBorder())
                .borderForeground(COLORS.warning);
        },
        // T031: Width-stable focus style for errorBadge elements.
        // errorBadge uses padding(0,1), so focusedErrorBadge must match.
        focusedErrorBadge: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground(COLORS.textOnColor)
                .background(COLORS.warning)
                .padding(0, 1);
        }
    };

    // Export styles for test access.
    prSplit._wizardStyles = styles;
    prSplit._wizardColors = COLORS;

    // -----------------------------------------------------------------------
    //  Layout Mode Helper (T07)
    //
    //  Returns 'compact' (<60), 'standard' (60-100), or 'wide' (>100).
    // -----------------------------------------------------------------------

    function layoutMode(s) {
        var w = s.width || 80;
        if (w < 60) return 'compact';
        if (w > 100) return 'wide';
        return 'standard';
    }

    // Export for testing.
    prSplit._layoutMode = layoutMode;

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

    // T025: Terminal/non-pipeline states override the step label to avoid
    // showing misleading "Finalization" when the wizard is actually
    // cancelled, paused, done, or errored.
    var STATE_LABEL_OVERRIDE = {
        'DONE':         'Done',
        'CANCELLED':    'Cancelled',
        'FORCE_CANCEL': 'Cancelled',
        'PAUSED':       'Paused',
        'ERROR':        'Error'
    };

    function renderTitleBar(s) {
        var stepIdx = STATE_TO_STEP[s.wizardState] || 0;
        var stepLabel = STATE_LABEL_OVERRIDE[s.wizardState] || STEP_LABELS[stepIdx] || 'Unknown';
        var stepNum = stepIdx + 1;
        var totalSteps = 7;
        var mode = layoutMode(s);

        // Elapsed time.
        var elapsed = s.startTime ? Math.floor((Date.now() - s.startTime) / 1000) : 0;
        var mins = Math.floor(elapsed / 60);
        var secs = elapsed % 60;
        var timeStr = mins + ':' + (secs < 10 ? '0' : '') + secs;

        var w = s.width || 80;

        if (mode === 'compact') {
            // Compact: dots + timer only, no title or step label.
            var dots = renderStepDots(s);
            var right = styles.dim().render('\u23f1 ' + timeStr);
            var leftW = lipgloss.width(dots);
            var rightW = lipgloss.width(right);
            var gap = Math.max(1, w - leftW - rightW);
            var gapStr = '';
            for (var i = 0; i < gap; i++) gapStr += ' ';
            return dots + gapStr + right;
        }

        var left = styles.titleBar().render('\ud83d\udd00 PR Split Wizard');
        var right = styles.stepIndicator().render(
            'Step ' + stepNum + '/' + totalSteps + ': ' + stepLabel + '  \u23f1 ' + timeStr
        );

        var leftW = lipgloss.width(left);
        var rightW = lipgloss.width(right);
        var gap = Math.max(1, w - leftW - rightW);
        var gapStr = '';
        for (var i = 0; i < gap; i++) gapStr += ' ';

        return left + gapStr + right;
    }

    function renderStepDots(s) {
        var stepIdx = STATE_TO_STEP[s.wizardState] || 0;
        var w = s.width || 80;
        // T027: At narrow widths, omit spaces between dots to save columns.
        var compact = w < 50;
        var dots = '';
        for (var i = 0; i < 7; i++) {
            if (i > 0 && !compact) dots += ' ';
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
        // T027: At very narrow widths, hide dots and back button entirely.
        // Priority order: Cancel > Next > Back > Dots.
        var veryNarrow = w < 35;

        // Back button (not on first screen; hidden at veryNarrow).
        var backBtn = '';
        if (!veryNarrow && stepIdx > 0 && !s.isProcessing) {
            var backLabel = narrow ? '\u2190' : '\u2190 Back';
            backBtn = zone.mark('nav-back',
                styles.secondaryButton().render(backLabel));
        }

        // Cancel button.
        var cancelLabel = narrow ? '\u00d7' : 'Cancel';
        var cancelBtn = zone.mark('nav-cancel',
            styles.secondaryButton().render(cancelLabel));

        // Dots (hidden at veryNarrow).
        var dots = veryNarrow ? '' : renderStepDots(s);

        // Next/action button.
        var nextBtn = '';
        if (s.isProcessing) {
            var frame = SPINNER_FRAMES[(s.spinnerFrame || 0) % SPINNER_FRAMES.length];
            nextBtn = styles.disabledButton().render(narrow ? frame : frame + ' Processing...');
        } else {
            var nextLabel = getNextButtonLabel(s);
            if (nextLabel) {
                if (narrow && nextLabel.length > 8) {
                    nextLabel = nextLabel.split(' ')[0];
                }
                // T120: Compute nav-next focus locally (pure — no state mutation).
                var focusElems = prSplit._getFocusElements ? prSplit._getFocusElements(s) : [];
                var isNavNextFocused = false;
                if (focusElems.length > 0) {
                    var lastElem = focusElems[focusElems.length - 1];
                    if (lastElem && lastElem.type === 'nav' &&
                        (s.focusIndex || 0) === focusElems.length - 1) {
                        isNavNextFocused = true;
                    }
                }
                var nextBtnStyle = isNavNextFocused
                    ? styles.focusedButton() : styles.primaryButton();
                nextBtn = zone.mark('nav-next',
                    nextBtnStyle.render(nextLabel + ' \u2192'));
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
        var rightParts = [];
        if (dots) rightParts.push(dots);
        if (nextBtn) rightParts.push(nextBtn);
        var rightSection = rightParts.length > 1
            ? lipgloss.joinHorizontal(lipgloss.Center, rightParts[0], '  ', rightParts[1])
            : (rightParts.length === 1 ? rightParts[0] : '');

        // Compose full nav bar: left ... right, spread across width.
        // T027: If content overflows, progressively drop dots to fit.
        var leftW = lipgloss.width(leftSection);
        var rightW = lipgloss.width(rightSection);
        if (leftW + rightW > w - 2 && dots) {
            // Overflow: rebuild right section without dots.
            rightParts = [];
            if (nextBtn) rightParts.push(nextBtn);
            rightSection = rightParts.length > 0 ? rightParts[0] : '';
            rightW = lipgloss.width(rightSection);
        }
        var gap = Math.max(1, w - leftW - rightW);
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

    // -----------------------------------------------------------------------
    //  T46: Inline Claude Question Prompt
    // -----------------------------------------------------------------------
    // When Claude asks a question during automated analysis/execution, this
    // renders a compact inline prompt at the bottom of the affected screen.
    // The user can type a response and press Enter to send it directly to
    // Claude's PTY (via tuiMux.writeToChild).

    function renderClaudeQuestionPrompt(s) {
        if (!s.claudeQuestionDetected) return '';

        var w = Math.max(40, (s.width || 80) - 4);
        var lines = [];

        // Question banner.
        var questionText = s.claudeQuestionLine || '(question detected)';
        if (questionText.length > w - 20) {
            questionText = questionText.substring(0, w - 23) + '...';
        }
        lines.push(styles.warningBadge().render(' \ud83e\udd16 Claude asks ') +
            ' ' + styles.fieldValue().render(questionText));

        // Input field.
        var inputPrefix;
        if (s.claudeQuestionInputActive) {
            inputPrefix = styles.bold().render('\u276f ');
        } else {
            inputPrefix = styles.dim().render('\u276f ');
        }
        var inputText = (s.claudeQuestionInputText || '') +
            (s.claudeQuestionInputActive ? styles.dim().render('\u2588') : '');
        var inputHint = s.claudeQuestionInputActive
            ? styles.dim().render('  Enter: send  Esc: dismiss')
            : styles.dim().render('  Type to respond or Esc to dismiss');

        lines.push('  ' + zone.mark('claude-question-input',
            inputPrefix + truncate(inputText, w - 16)) + inputHint);

        // Conversation history count (if any).
        if (s.claudeConversations && s.claudeConversations.length > 0) {
            lines.push('  ' + styles.dim().render(
                s.claudeConversations.length + ' prior Q&A exchange' +
                (s.claudeConversations.length !== 1 ? 's' : '')));
        }

        // Wrap in a subtle bordered box.
        var promptStyle = lipgloss.newStyle()
            .border(lipgloss.roundedBorder())
            .borderForeground(COLORS.warning)
            .width(w - 2)
            .padding(0, 1);

        return '\n' + promptStyle.render(lines.join('\n'));
    }

    function renderStatusBar(s) {
        var w = s.width || 80;
        var narrow = w < 60;
        var veryNarrow = w < 40;

        // Left: termmux toggle hint + split-view hint.
        var leftParts = veryNarrow ? 'C-]' : 'Ctrl+] Claude';
        if (!veryNarrow) {
            leftParts += '  Ctrl+L Split';
        }
        var left = styles.dim().render(leftParts);

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

        // T45: Transient notification banner (auto-dismiss via tick, not view).
        // T028: The notification is now dismissed by a 'dismiss-attach-notif'
        // tick handler in _updateFn, not here. The view only reads state.
        var notifLine = '';
        if (s.claudeAutoAttachNotif && s.claudeAutoAttachNotifAt) {
            var elapsed = Date.now() - s.claudeAutoAttachNotifAt;
            if (elapsed < 5000) {
                notifLine = styles.primaryButton().render(
                    ' \u2139 ' + s.claudeAutoAttachNotif + ' '
                ) + '\n';
            }
            // No else — dismiss is handled by 'dismiss-attach-notif' tick.
        }

        return notifLine + styles.dim().render(
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
        if (ms < 15000) return styles.statusIdle().render('\u23f3 Claude: idle (' + Math.round(ms / 1000) + 's)');
        return styles.statusQuiet().render('\ud83d\udca4 Claude: quiet');
    }

    // -----------------------------------------------------------------------
    //  Split-View: Claude Pane Renderer (T15)
    // -----------------------------------------------------------------------
    function renderClaudePane(s, width, height) {
        // T28: Prefer ANSI-styled content (claudeScreen), fall back to plain text.
        var ansiContent = s.claudeScreen || '';
        var plainContent = s.claudeScreenshot || '';
        var content = ansiContent || plainContent;
        var isANSI = !!ansiContent;
        var hasMux = (typeof tuiMux !== 'undefined' && tuiMux &&
            typeof tuiMux.childScreen === 'function');
        // Backward-compat: also check for screenshot-only mux.
        if (!hasMux) {
            hasMux = (typeof tuiMux !== 'undefined' && tuiMux &&
                typeof tuiMux.screenshot === 'function');
        }

        // Height budget: border adds 2 lines (top + bottom).
        // Content height = height - 2. First content line is the title.
        var contentH = Math.max(1, height - 2);
        var viewH = Math.max(1, contentH - 1); // lines for content text
        var viewW = Math.max(10, width - 6);    // border(2) + padding(2) + safety(2)

        // Focus indicator.
        var isFocused = (s.splitViewFocus === 'claude');
        var borderColor = isFocused ? COLORS.primary : COLORS.border;

        // Placeholder when no Claude session is available.
        if (!hasMux || !content) {
            var placeholder = styles.dim().render(
                hasMux ? 'Waiting for Claude output...'
                       : 'No Claude session attached');
            var hint = styles.dim().render('Ctrl+] to toggle Claude \u00b7 Ctrl+L to close split');

            var phLines = [];
            var phPadTop = Math.max(0, Math.floor((contentH - 2) / 2));
            for (var pi = 0; pi < phPadTop; pi++) phLines.push('');
            phLines.push(placeholder);
            phLines.push(hint);
            while (phLines.length < contentH) phLines.push('');

            var phStyle = lipgloss.newStyle()
                .border(lipgloss.roundedBorder())
                .borderForeground(borderColor)
                .width(width - 2)
                .height(contentH);
            return phStyle.render(phLines.join('\n'));
        }

        // Parse content into lines.
        var lines = content.split('\n');
        while (lines.length > 0 && lines[lines.length - 1] === '') {
            lines.pop();
        }

        var totalLines = lines.length;

        // Scroll indicator.
        var scrollInfo = '';
        if (totalLines > viewH) {
            if (s.claudeViewOffset <= 0) {
                scrollInfo = ' [live]';
            } else {
                var startForPct = Math.max(0, totalLines - viewH - s.claudeViewOffset);
                var pct = Math.round((startForPct / Math.max(1, totalLines - viewH)) * 100);
                scrollInfo = ' [' + pct + '%]';
            }
        }

        // Title line: show mode indicator (T28) and input indicator (T29).
        var modeTag = isANSI ? '' : ' [plain]';
        var inputTag = isFocused ? ' INPUT' : '';
        var titleText = styles.bold().render(' Claude' + inputTag + modeTag + scrollInfo + ' ');

        // Determine visible window based on scroll offset.
        var startLine;
        if (s.claudeViewOffset <= 0) {
            startLine = Math.max(0, totalLines - viewH);
        } else {
            startLine = Math.max(0, totalLines - viewH - s.claudeViewOffset);
        }
        var endLine = Math.min(totalLines, startLine + viewH);

        // Build viewport content with ANSI-aware line truncation.
        var contentLines = [titleText];
        for (var ci = startLine; ci < endLine; ci++) {
            var ln = lines[ci] || '';
            // Use lipgloss.width for ANSI-aware visual width calculation.
            var visualW = lipgloss.width(ln);
            if (visualW > viewW) {
                // Truncate: strip ANSI-unaware substring is risky, so we
                // attempt a visual-width-aware truncation. Since lipgloss
                // doesn't expose truncate(), we use a simple approach: if
                // the line has ANSI codes, trim iteratively; for plain text
                // use substring.
                if (isANSI) {
                    // For ANSI lines, let lipgloss.newStyle().maxWidth()
                    // handle truncation — it's ANSI-aware.
                    ln = lipgloss.newStyle().maxWidth(viewW).render(ln);
                } else {
                    ln = ln.substring(0, viewW - 3) + '...';
                }
            }
            contentLines.push(ln);
        }
        // Pad to fill contentH.
        while (contentLines.length < contentH) {
            contentLines.push('');
        }

        var paneStyle = lipgloss.newStyle()
            .border(lipgloss.roundedBorder())
            .borderForeground(borderColor)
            .width(width - 2)
            .height(contentH);

        return paneStyle.render(contentLines.join('\n'));
    }

    // -----------------------------------------------------------------------
    //  Claude Conversation Overlay (T16)
    // -----------------------------------------------------------------------
    function viewClaudeConvoOverlay(s) {
        var convo = s.claudeConvo;
        var w = Math.min((s.width || 80) - 4, 76);
        var h = Math.min((s.height || 24) - 4, 22);

        var lines = [];

        // Title.
        var contextLabel = convo.context === 'plan-review' ? 'Plan Review'
            : convo.context === 'error-resolution' ? 'Error Resolution'
            : 'Claude';
        lines.push(styles.bold().render('\ud83e\udd16 Ask Claude \u2014 ' + contextLabel));
        lines.push(styles.dim().render('Type your message. Enter to send. Esc to close.'));
        lines.push('');

        // Error banner.
        if (convo.lastError) {
            lines.push(styles.errorBadge().render(' Error ') + ' ' +
                styles.dim().render(truncate(convo.lastError, w - 12)));
            lines.push('');
        }

        // Conversation history.
        var historyHeight = h - 7; // title(2) + blank(1) + input(2) + status(1) + padding(1)
        if (convo.lastError) historyHeight -= 2;
        historyHeight = Math.max(3, historyHeight);

        var historyLines = [];
        for (var hi = 0; hi < convo.history.length; hi++) {
            var entry = convo.history[hi];
            if (entry.role === 'user') {
                historyLines.push(styles.primaryButton().render(' You ') + ' ' + entry.text);
            } else {
                var cLines = entry.text.split('\n');
                historyLines.push(styles.successBadge().render(' Claude '));
                for (var cl = 0; cl < cLines.length; cl++) {
                    historyLines.push('  ' + truncate(cLines[cl], w - 6));
                }
            }
            historyLines.push('');
        }

        // Apply scroll offset.
        var visibleHistory;
        var totalHistLines = historyLines.length;
        if (totalHistLines <= historyHeight) {
            visibleHistory = historyLines.slice();
        } else {
            var scrollOff = convo.scrollOffset || 0;
            var start = Math.max(0, totalHistLines - historyHeight - scrollOff);
            var end = Math.min(totalHistLines, start + historyHeight);
            visibleHistory = historyLines.slice(start, end);
        }

        // Pad to fill history area.
        while (visibleHistory.length < historyHeight) {
            visibleHistory.push('');
        }

        for (var vhi = 0; vhi < visibleHistory.length; vhi++) {
            lines.push(visibleHistory[vhi]);
        }

        // Separator.
        lines.push(styles.dim().render(repeatStr('\u2500', w - 4)));

        // Input field.
        var inputPrefix = convo.sending
            ? styles.dim().render('\u23f3 Sending...')
            : styles.bold().render('\u276f ');
        var inputText = convo.sending
            ? styles.dim().render('Waiting for Claude to respond...')
            : (convo.inputText || '') + styles.dim().render('\u2588');
        lines.push(inputPrefix + truncate(inputText, w - 8));

        // Status bar.
        var statusParts = [];
        if (convo.sending && convo.waitingForTool) {
            statusParts.push(styles.dim().render('Waiting for: ' + convo.waitingForTool));
        }
        if (convo.history.length > 0) {
            statusParts.push(styles.dim().render(convo.history.length + ' message' +
                (convo.history.length !== 1 ? 's' : '')));
        }
        if (statusParts.length > 0) {
            lines.push(statusParts.join(' \u00b7 '));
        }

        // Wrap in bordered box.
        var contentH = Math.max(1, lines.length);
        var overlayStyle = lipgloss.newStyle()
            .border(lipgloss.roundedBorder())
            .borderForeground(COLORS.primary)
            .width(w - 2)
            .height(contentH)
            .padding(0, 1);

        return overlayStyle.render(lines.join('\n'));
    }

    // -----------------------------------------------------------------------
    //  Split-View: Output Pane Renderer (T44)
    // -----------------------------------------------------------------------
    function renderOutputPane(s, width, height) {
        var lines = s.outputLines || [];
        var totalLines = lines.length;

        // Height budget: border adds 2 lines (top + bottom).
        var contentH = Math.max(1, height - 2);
        var viewH = Math.max(1, contentH - 1); // lines for content text (1 for title)
        var viewW = Math.max(10, width - 6);    // border(2) + padding(2) + safety(2)

        // Focus indicator.
        var isFocused = (s.splitViewFocus === 'claude' && s.splitViewTab === 'output');
        var borderColor = isFocused ? COLORS.primary : COLORS.border;

        // Placeholder when no output is available.
        if (totalLines === 0) {
            var placeholder = styles.dim().render('No process output yet');
            var hint = styles.dim().render('Output from git, make, and analysis will appear here');

            var phLines = [];
            var phPadTop = Math.max(0, Math.floor((contentH - 2) / 2));
            for (var pi = 0; pi < phPadTop; pi++) phLines.push('');
            phLines.push(placeholder);
            phLines.push(hint);
            while (phLines.length < contentH) phLines.push('');

            var phStyle = lipgloss.newStyle()
                .border(lipgloss.roundedBorder())
                .borderForeground(borderColor)
                .width(width - 2)
                .height(contentH);
            return phStyle.render(phLines.join('\n'));
        }

        // Scroll indicator.
        var scrollInfo = '';
        var offset = s.outputViewOffset || 0;
        if (totalLines > viewH) {
            if (offset <= 0) {
                scrollInfo = ' [live]';
            } else {
                var startForPct = Math.max(0, totalLines - viewH - offset);
                var pct = Math.round((startForPct / Math.max(1, totalLines - viewH)) * 100);
                scrollInfo = ' [' + pct + '%]';
            }
        }

        // Title line.
        var titleText = styles.bold().render(' Output' + scrollInfo +
            ' \u2014 ' + totalLines + ' line' + (totalLines !== 1 ? 's' : '') + ' ');

        // Determine visible window based on scroll offset.
        var startLine;
        if (offset <= 0) {
            startLine = Math.max(0, totalLines - viewH);
        } else {
            startLine = Math.max(0, totalLines - viewH - offset);
        }
        var endLine = Math.min(totalLines, startLine + viewH);

        // Build viewport content with ANSI-aware line truncation.
        var contentLines = [titleText];
        for (var ci = startLine; ci < endLine; ci++) {
            var ln = lines[ci] || '';
            var visualW = lipgloss.width(ln);
            if (visualW > viewW) {
                // Use lipgloss maxWidth for ANSI-safe truncation.
                ln = lipgloss.newStyle().maxWidth(viewW).render(ln);
            }
            contentLines.push(ln);
        }
        // Pad to fill contentH.
        while (contentLines.length < contentH) {
            contentLines.push('');
        }

        var paneStyle = lipgloss.newStyle()
            .border(lipgloss.roundedBorder())
            .borderForeground(borderColor)
            .width(width - 2)
            .height(contentH);

        return paneStyle.render(contentLines.join('\n'));
    }

    // Export chrome for testing.
    prSplit._renderTitleBar = renderTitleBar;
    prSplit._renderNavBar = renderNavBar;
    prSplit._renderStatusBar = renderStatusBar;
    prSplit._renderClaudePane = renderClaudePane;
    prSplit._renderOutputPane = renderOutputPane;
    prSplit._renderStepDots = renderStepDots;
    prSplit._viewClaudeConvoOverlay = viewClaudeConvoOverlay;

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
                { id: 'config-maxFiles',       label: 'Max files per chunk', field: 'maxFiles',       value: String(runtime.maxFiles || 10) },
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
            lines.push(
                zone.mark('plan-edit', editBtnStyle.render('Edit Plan \u270f')) +
                '  ' +
                zone.mark('plan-regenerate', regenBtnStyle.render('Regenerate \ud83d\udd04')) +
                '  ' +
                zone.mark('ask-claude', askClaudeStyle.render('Ask Claude \ud83e\udd16'))
            );
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
                    lines.push(
                        zone.mark('editor-move', moveBtnStyle.render('Move File')) +
                        '  ' +
                        zone.mark('editor-rename', renameBtnStyle.render('Rename Split')) +
                        '  ' +
                        zone.mark('editor-merge', mergeBtnStyle.render('Merge Splits'))
                    );
                }
            }
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

        // T107: Show warning when any branches had git-ignored files skipped.
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

        // ── Per-branch verification section ──────────────────────
        var verifyResults = s.verificationResults || [];
        var verifyIdx = s.verifyingIdx;
        if (verifyIdx >= 0 || verifyResults.length > 0) {
            lines.push('');
            lines.push(styles.bold().render('Verifying Branches'));
            lines.push('');

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

            // Verification progress bar.
            if (verifyResults.length < splits.length && s.isProcessing) {
                // ── Live CaptureSession viewport ─────────────────────
                if (s.activeVerifySession) {
                    lines.push('');
                    // T005: Use screen() for ANSI-escaped VT100 rendering
                    // instead of output() which strips all formatting.
                    var liveOutput = s.activeVerifySession.screen();
                    var liveLines = liveOutput.split('\n');
                    // Remove trailing empty lines from VTerm screen output.
                    // screen() may include ANSI reset codes on empty lines,
                    // so we check visual width rather than string equality.
                    while (liveLines.length > 0 && lipgloss.width(liveLines[liveLines.length - 1]) === 0) {
                        liveLines.pop();
                    }

                    var viewWidth = Math.max(40, (s.width || 80) - 8);
                    var viewHeight = 12; // content rows inside the border
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
                    var footer = styles.dim().render(
                        '\u2191\u2193: Scroll' + scrollIndicator + '  ' +
                        zone.mark('verify-interrupt', 'Ctrl+C: Stop  2\u00d7Ctrl+C: Force Kill'));

                    // Render bordered viewport using lipgloss.
                    var vpStyle = lipgloss.newStyle()
                        .border(lipgloss.roundedBorder())
                        .borderForeground(COLORS.warning)
                        .width(viewWidth)
                        .padding(0, 1);

                    lines.push('  ' + styles.warningBadge().render(titleText));
                    lines.push(vpStyle.render(vpContent));
                    lines.push('  ' + footer);
                }

                lines.push('');
                var vProgress = verifyResults.length / splits.length;
                lines.push('  ' + renderProgressBar(vProgress, (s.width || 80) - 8));
            }

            // Verification summary (after all complete).
            if (verifyResults.length === splits.length) {
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
                if (failCount > 0 && !s.activeVerifySession) {
                    lines.push('  ' + styles.dim().render('Press p to mark a failed branch as passed'));
                }
            }
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
                if (zone) {
                    lines.push('  ' + zone.mark('equiv-reverify', styles.secondaryButton().render(' Re-verify ')) +
                        '  ' + zone.mark('equiv-revise', styles.secondaryButton().render(' Revise Plan ')) +
                        '  ' + zone.mark('nav-next', styles.primaryButton().render(' Continue \u25b6 ')));
                } else {
                    lines.push('  ' + styles.secondaryButton().render(' Re-verify ') +
                        '  ' + styles.secondaryButton().render(' Revise Plan ') +
                        '  ' + styles.primaryButton().render(' Continue \u25b6 '));
                }
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
            lines.push(
                zone.mark('final-report',
                    reportStyle.render('View Report')) +
                '  ' +
                zone.mark('final-create-prs',
                    createStyle.render(createLabel)) +
                '  ' +
                zone.mark('final-done',
                    doneStyle.render('Done'))
            );
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
                lines.push(padRight('  Ctrl+C', 22) + 'Interrupt current verify (2x = force kill)');
                lines.push(padRight('  p', 22) + 'Mark failed branch as passed (override)');
            }
            lines.push('');
        }

        // -- Claude / Split View (always shown) --
        lines.push(styles.label().render('Claude Integration'));
        lines.push(padRight('  Ctrl+L', 22) + 'Toggle split view');
        lines.push(padRight('  Ctrl+Tab', 22) + 'Switch wizard / Claude pane');
        lines.push(padRight('  Ctrl+O', 22) + 'Switch Claude / Output tab');
        lines.push(padRight('  Ctrl+]', 22) + 'Full Claude passthrough');
        lines.push(padRight('  Ctrl+= / Ctrl+-', 22) + 'Resize split view');

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
        if (s.activeVerifySession) {
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
        var overlayH = Math.max(8, (s.height || 24) - 6);
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
            if (flashElapsed < 3000) {
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

        // T096: Show validation error from branch name check.
        if (ds.validationError) {
            lines.push(styles.errorText().render('\u26a0 ' + ds.validationError));
        }

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
                lines.push(zone.mark('pause-resume', resumeBtn) + '  ' + zone.mark('pause-quit', quitBtn));
                lines.push('');
                lines.push(styles.dim().render('Press Enter to activate focused button, or Ctrl+C to cancel.'));
                return lines.join('\n');
            }
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
    prSplit._syncReportOverlay = syncReportOverlay;
    prSplit._syncReportScrollbar = syncReportScrollbar;
    prSplit._computeReportOverlayDims = computeReportOverlayDims;
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
