'use strict';
// pr_split_15a_tui_styles.js — TUI: Colors, styles, layout mode, shared utilities
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
    prSplit._SPINNER_FRAMES = SPINNER_FRAMES;
    prSplit._truncate = truncate;
    prSplit._padRight = padRight;

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
