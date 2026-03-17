'use strict';
// pr_split_15b_tui_chrome.js — TUI: Chrome renderers (title bar, nav bar, status bar, panes)
// Dependencies: pr_split_15a_tui_styles.js must be loaded first.

(function(prSplit) {
    if (typeof tui === 'undefined' || typeof ctx === 'undefined' || typeof output === 'undefined') { return; }

    var styles = prSplit._wizardStyles;
    var COLORS = prSplit._wizardColors;
    var layoutMode = prSplit._layoutMode;
    var SPINNER_FRAMES = prSplit._SPINNER_FRAMES;
    var lipgloss = prSplit._lipgloss;
    var zone = prSplit._zone;
    var truncate = prSplit._truncate;
    var repeatStr = prSplit._repeatStr;

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

        // T200: Compute focused element once for all nav buttons.
        // Previous code checked last-position which was always nav-cancel,
        // inverting the highlight: nav-cancel rendered as focused "Next"
        // while actual nav-next got no highlight. Fix: ID-based lookup.
        var focusElems = prSplit._getFocusElements ? prSplit._getFocusElements(s) : [];
        var focusIdx = s.focusIndex || 0;
        var focusedElemId = (focusElems[focusIdx] || {}).id || '';

        // Back button (not on first screen; hidden at veryNarrow).
        var backBtn = '';
        if (!veryNarrow && stepIdx > 0 && !s.isProcessing) {
            var backLabel = narrow ? '\u2190' : '\u2190 Back';
            var backStyle = (focusedElemId === 'nav-back')
                ? styles.focusedSecondaryButton() : styles.secondaryButton();
            backBtn = zone.mark('nav-back', backStyle.render(backLabel));
        }

        // Cancel button — T201: gets focus styling when tabbed to.
        var cancelLabel = narrow ? '\u00d7' : 'Cancel';
        var cancelStyle = (focusedElemId === 'nav-cancel')
            ? styles.focusedSecondaryButton() : styles.secondaryButton();
        var cancelBtn = zone.mark('nav-cancel',
            cancelStyle.render(cancelLabel));

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
                // T200: Direct ID check — not position-based.
                var nextBtnStyle = (focusedElemId === 'nav-next')
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
        // T309: Only show Ctrl+] Claude when a Claude child is actually attached.
        var hasMuxChild = typeof tuiMux !== 'undefined' && tuiMux &&
            (typeof tuiMux.hasChild !== 'function' || tuiMux.hasChild());
        var leftParts;
        if (hasMuxChild) {
            leftParts = veryNarrow ? 'C-]' : 'Ctrl+] Claude';
        } else {
            leftParts = '';
        }
        if (!veryNarrow) {
            if (leftParts) leftParts += '  ';
            leftParts += 'Ctrl+L Split';
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
    prSplit._renderClaudeQuestionPrompt = renderClaudeQuestionPrompt;

})(globalThis.prSplit);
