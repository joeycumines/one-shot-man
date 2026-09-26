'use strict';
// pr_split_15a_tui_styles.js — TUI: Colors, styles, layout mode, shared utilities
// Dependencies: chunks 00-14 must be loaded first.

(function(prSplit) {

    if (typeof tui === 'undefined' || typeof ctx === 'undefined' ||
        typeof output === 'undefined') { return; }

    var tea = require('osm:bubbletea');
    var lipgloss = require('osm:lipgloss');
    var zone = require('osm:bubblezone');
    var viewportLib = require('osm:bubbles/viewport');
    var scrollbarLib = require('osm:termui/scrollbar');

    // --- COLORS & Styles (T006) ---
    //
    // Design spec: docs/pr-split-tui-design.md §2

    // Adaptive color palette: auto-detects light/dark terminal background.
    // Uses {light, dark} objects resolved by lipgloss.AdaptiveColor.
    // WCAG AA compliant: all text-on-background pairs >= 4.5:1 contrast.
    // textOnColor is the inverse text for colored (non-surface) backgrounds.
    var COLORS = {
        primary:     {light: '#1D4ED8', dark: '#3B82F6'},  // Blue 700 / 500
        secondary:   {light: '#4338CA', dark: '#818CF8'},  // Indigo 700 / 400
        success:     {light: '#047857', dark: '#10B981'},  // Emerald 700 / 500
        warning:     {light: '#B45309', dark: '#F59E0B'},  // Amber 700 / 500
        error:       {light: '#B91C1C', dark: '#EF4444'},  // Red 700 / 500
        muted:       {light: '#64748B', dark: '#64748B'},  // Slate gray
        surface:     {light: '#E2E8F0', dark: '#1E293B'},  // Slate card surface / button fill
        border:      {light: '#94A3B8', dark: '#334155'},  // Slate border
        text:        {light: '#0F172A', dark: '#F8FAFC'},  // Slate 900 / 50 text
        textDim:     {light: '#334155', dark: '#94A3B8'},  // Slate 700 / 400 text (high contrast)
        textOnColor: {light: '#FFFFFF', dark: '#000000'}   // High-contrast text on saturated badges
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
                .padding(0, 1);
        },
        inactiveCard: function() {
            return lipgloss.newStyle()
                .border(lipgloss.roundedBorder())
                .borderForeground(COLORS.border)
                .padding(0, 1);
        },
        errorCard: function() {
            return lipgloss.newStyle()
                .border(lipgloss.normalBorder())
                .borderForeground(COLORS.error)
                .padding(0, 1);
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
                .bold(true)
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
                .border(lipgloss.roundedBorder())
                .borderForeground(COLORS.warning)
                .padding(0, 1);
        },
        focusedButton: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground(COLORS.textOnColor)
                .background(COLORS.warning)
                .padding(0, 2);
        },
        // Width- and height-stable focus style for secondary buttons.
        focusedSecondaryButton: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground(COLORS.textOnColor)
                .background(COLORS.warning)
                .padding(0, 2)
                .border(lipgloss.roundedBorder())
                .borderForeground(COLORS.warning);
        },
        // Width-stable focus style for errorBadge elements.
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

    // --- Layout Mode Helper (T07) ---
    //
    // Returns 'compact' (<60), 'standard' (60-100), or 'wide' (>100).

    function layoutMode(s) {
        var w = s.width || 80;
        if (w < 60) return 'compact';
        if (w > 100) return 'wide';
        return 'standard';
    }

    // Export for testing.
    prSplit._layoutMode = layoutMode;

    // --- Helpers (T008) ---
    //
    // Progress bar, truncation, padding

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

    // TUI numeric constants — centralizes magic numbers used across chunks.
    var TUI_CONSTANTS = {
        OUTPUT_BUFFER_CAP: 5000,    // max output lines before trimming
        SIGKILL_WINDOW_MS: 2000,    // double-tap Ctrl+C kill window
        TRUNCATION_WIDTH: 120,      // max chars before truncating text
        CONVO_TIMEOUT_MS: 120000,   // 2-min conversation/command timeout
        TICK_INTERVAL_MS: 100,      // polling interval for tea.tick (ms)
        DEFAULT_ROWS: 24,           // fallback terminal row count
        INLINE_VIEW_HEIGHT: 12,     // min height for inline terminal view
        DISMISS_NOTIF_MS: 5000,     // auto-dismiss notification timeout (ms)
        ANALYSIS_TIMEOUT_MS: 60000, // slow-analysis warning threshold
        RESOLVE_POLL_MS: 500,       // conflict-resolution poll interval
        AGENT_CHECK_POLL_MS: 50,   // agent binary resolution fast-poll
        AUTO_SPLIT_POLL_MS: 500,    // auto-split pipeline poll interval
        AGENT_SCREENSHOT_POLL_MS: 500, // agent terminal screenshot poll
        AGENT_ACTIVE_POLL_MS: 100,     // fast poll when Agent is outputting
        AGENT_IDLE_POLL_MS: 2000,      // slow poll when idle (no recent output)
        AGENT_OUTPUT_IDLE_MS: 3000,    // Agent idle threshold (no output events)
        AGENT_BELL_FLASH_MS: 1500,     // bell visual indicator duration
        QUESTION_IDLE_MS: 2000,     // idle threshold before question detection
        QUESTION_SCAN_LINES: 15,    // trailing lines scanned for questions
        CONVO_POLL_MS: 200,         // agent conversation send/wait poll
        PLAN_REVISION_TIMEOUT_MS: 180000, // 3-min plan revision timeout
        SCREENSHOT_CAPTURE_CHARS: 500,    // max chars from screenshot capture
        CONVO_HISTORY_CAP: 100,     // conversation history cap
        CONVO_HISTORY_TRIM: 80,     // trim target when cap exceeded
        AGENT_QUESTION_SEND_TIMEOUT_MS: 10000, // pane write that never settles must not strand the prompt
        CLIPBOARD_FLASH_MS: 3000,   // clipboard copy flash duration
        AUTO_ATTACH_NOTIF_GUARD_MS: 4500, // auto-attach dismiss guard
        CLIPBOARD_FLASH_GUARD_MS: 2500,   // clipboard flash dismiss guard
        FAR_SCROLL_SENTINEL: 999999,      // scroll-to-top sentinel value
        PR_CREATION_POLL_MS: 200,         // PR creation poll interval
    };

    prSplit._renderProgressBar = renderProgressBar;
    prSplit._SPINNER_FRAMES = SPINNER_FRAMES;
    prSplit._truncate = truncate;
    prSplit._padRight = padRight;
    prSplit._TUI_CONSTANTS = TUI_CONSTANTS;

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
