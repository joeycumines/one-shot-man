#!/usr/bin/env osm script
// ============================================================================
// example-15-claude-autopilot.js
// Claude Code Autopilot — Compositor-based PTY monitor with dashboard overlay
//
// Architecture:
//   - osm:termui/compositor — Z-ordered pane composition (PTY pane z=0 + chrome z=10)
//   - osm:bubblezone — Zone-based mouse hit testing (mark/scan/inBounds)
//   - osm:termmux — PTY management, mouseToSGR, keyToTermBytes, event bus
//   - osm:bubbletea — TUI framework
//   - osm:lipgloss — Styling
//   - osm:flag — Argument parsing
//
// The PTY output fills the entire screen as a compositor pane (z=0).
// Dashboard chrome overlays the top (title, state) and bottom (activity,
// controls, status bar) as compositor chrome layers (z=10).
//
// Mouse events in the PTY visible area are forwarded to the child via
// termmux.mouseToSGR(). Keyboard events not handled by dashboard controls
// are forwarded to the child via termmux.keyToTermBytes().
//
// Usage:
//   osm script scripts/example-15-claude-autopilot.js -- claude [args...]
//   osm script scripts/example-15-claude-autopilot.js --cmd /path/to/claude
//
// Controls (keyboard):
//   a                   Toggle autopilot on/off
//   k                   Manual kick (inject status query)
//   i                   Enter input mode (type text, Enter sends to PTY)
//   g                   Trigger network error recovery (Ctrl+C + Enter)
//   up / down           Scroll PTY preview
//   shift+up / shift+down  Scroll activity log
//   q / Ctrl+C          Quit
//
// Controls (mouse):
//   Click on buttons     Trigger the corresponding action (bubblezone hit test)
//   Click in PTY area    Forward to child via mouseToSGR
//   Mouse motion         Forward to child (hover effects)
//   Scroll wheel         Scroll PTY preview
// ============================================================================

try {
    var termmux = require('osm:termmux');
    var tea = require('osm:bubbletea');
    var lip = require('osm:lipgloss');
    var flag = require('osm:flag');
    var compMod = require('osm:termui/compositor');
    var zone = require('osm:bubblezone');
} catch (e) {
    throw new Error('Failed to load modules: ' + e.message);
}

// -- Constants ---------------------------------------------------------------
var TICK_MS = 500;
var IDLE_THRESHOLD_TICKS = 20;            // 10s before considering idle
var STUCK_THRESHOLD_TICKS = 60;           // 30s before kick
var KICK_INTERVAL_TICKS = 120;            // 60s between kicks
var INTERVENTION_COOLDOWN_TICKS = 10;     // 5s between interventions
var LOG_MAX_ENTRIES = 100;                // Ring buffer size for activity log
var HEADER_HEIGHT = 3;                    // title + state + separator
var FOOTER_HEIGHT = 7;                    // separator + activity hdr + 2 log + spacer + controls + status

var SPINNER_CHARS = '⠋⠙⠹⠸⡌⡤⡠⠰⠀';
var STATE_INDICATORS = {
    EMPTY:         '○',
    ACTIVE:        '●',
    IDLE_PROMPT:   '◐',
    NETWORK_ERROR: '⚠',
    LOADING:       '◜',
    WAITING:       '⧖',
    ERROR:         '✘'
};

// -- Theme (Tokyo Night) ----------------------------------------------------
var C = {
    bg:           '#1A1B26',
    surface:      '#24283B',
    chrome:       '#292E42',
    border:       '#3B4261',
    muted:        '#565F89',
    dim:          '#787C99',
    text:         '#C0CAF5',
    bright:       '#D5D6DB',
    cyan:         '#7DCFFF',
    blue:         '#7AA2F7',
    purple:       '#BB9AF7',
    green:        '#9ECE6A',
    brightGreen:  '#73DACA',
    yellow:       '#E0AF68',
    orange:       '#FF9E64',
    red:          '#F7768E',
    barGreen:     '#9ECE6A',
};

// -- Argument Parsing --------------------------------------------------------
var fs = flag.newFlagSet('autopilot');
fs.string('cmd', 'claude', 'command to wrap in PTY');
fs.string('kick-msg', 'Provide a brief status update on your current task', 'kick message');
fs.bool('autopilot', false, 'enable autopilot immediately');

var parseResult = fs.parse(args);
if (parseResult.error !== null) {
    throw new Error('flag parse failed: ' + parseResult.error);
}

var CMD = fs.get('cmd');
var KICK_MSG = fs.get('kick-msg');
var AUTO_ENABLED = fs.get('autopilot');

var targetArgs = fs.args();
if (targetArgs.length === 0) {
    targetArgs = [];
}

// -- ANSI Utility Functions --------------------------------------------------

// Measure the visual width of a string, ignoring ANSI escape sequences.
function visualWidth(str) {
    var w = 0;
    var inEscape = false;
    for (var i = 0; i < str.length; i++) {
        var ch = str.charCodeAt(i);
        if (inEscape) {
            if (ch >= 0x40 && ch <= 0x7E) { inEscape = false; }
            continue;
        }
        if (ch === 0x1B) { inEscape = true; continue; }
        if (ch < 0x20) continue;
        w++;
    }
    return w;
}

// Truncate an ANSI string to fit within maxVisualWidth cells.
function ansiTruncate(str, maxVisualWidth) {
    var w = 0;
    var result = '';
    var inEscape = false;
    var escBuf = '';
    for (var i = 0; i < str.length; i++) {
        var ch = str.charCodeAt(i);
        if (inEscape) {
            escBuf += str.charAt(i);
            if (ch >= 0x40 && ch <= 0x7E) {
                result += escBuf;
                escBuf = '';
                inEscape = false;
            }
            continue;
        }
        if (ch === 0x1B) {
            inEscape = true;
            escBuf = '\x1B';
            continue;
        }
        if (ch < 0x20) {
            result += str.charAt(i);
            continue;
        }
        if (w >= maxVisualWidth) { break; }
        result += str.charAt(i);
        w++;
    }
    if (inEscape) { result += escBuf; }
    result += '\x1b[0m';
    return result;
}

// Strip all ANSI escape sequences, returning plain text.
function stripAnsi(str) {
    var out = '';
    var inEscape = false;
    for (var i = 0; i < str.length; i++) {
        var ch = str.charCodeAt(i);
        if (inEscape) {
            if (ch >= 0x40 && ch <= 0x7E) { inEscape = false; }
            continue;
        }
        if (ch === 0x1B) { inEscape = true; continue; }
        out += str.charAt(i);
    }
    return out;
}

// -- Formatting Functions ----------------------------------------------------

function formatTickTime(ticks) {
    var totalSec = Math.floor(ticks * TICK_MS / 1000);
    var h = Math.floor(totalSec / 3600);
    var m = Math.floor((totalSec % 3600) / 60);
    var s = totalSec % 60;
    var mm = m < 10 ? '0' + m : '' + m;
    var ss = s < 10 ? '0' + s : '' + s;
    if (h > 0) {
        var hh = h < 10 ? '0' + h : '' + h;
        return hh + ':' + mm + ':' + ss;
    }
    return mm + ':' + ss;
}

function padRight(s, n) {
    s = String(s);
    while (s.length < n) s += ' ';
    return s.substring(0, n);
}

// -- State Detection Engine --------------------------------------------------

function analyzeSnapshot(text) {
    if (!text || text.length === 0) return 'EMPTY';

    var lower = text.toLowerCase();

    // Network errors
    if (lower.indexOf('network error') >= 0 ||
        lower.indexOf('connection reset') >= 0 ||
        lower.indexOf('econnrefused') >= 0 ||
        lower.indexOf('connection refused') >= 0) {
        return 'NETWORK_ERROR';
    }

    // Spinners
    for (var i = 0; i < SPINNER_CHARS.length; i++) {
        if (text.indexOf(SPINNER_CHARS.charAt(i)) >= 0) {
            return 'LOADING';
        }
    }
    if (lower.indexOf('thinking') >= 0 || lower.indexOf('loading') >= 0) {
        return 'LOADING';
    }

    // Timeout
    if (lower.indexOf('timeout') >= 0) {
        return 'NETWORK_ERROR';
    }

    // Generic errors
    if (lower.indexOf('error') >= 0 || lower.indexOf('failed') >= 0 || lower.indexOf('exception') >= 0) {
        return 'ERROR';
    }

    // Waiting states
    if (lower.indexOf('waiting') >= 0 || lower.indexOf('press any key') >= 0) {
        return 'WAITING';
    }

    // Idle prompt detection
    var lines = text.split('\n');
    var lastLine = '';
    for (var li = lines.length - 1; li >= 0; li--) {
        lastLine = lines[li].replace(/^\s+|\s+$/g, '');
        if (lastLine.length > 0) break;
    }
    if (lastLine.length > 0) {
        var firstChar = lastLine.charAt(0);
        if (firstChar === '>' || firstChar === '$' || firstChar === '#') {
            return 'IDLE_PROMPT';
        }
        if (lastLine.indexOf('?> ') >= 0 || lastLine.indexOf('% ') >= 0) {
            return 'IDLE_PROMPT';
        }
    }

    return 'ACTIVE';
}

// -- Activity Log ------------------------------------------------------------

function addLogEntry(model, type, message) {
    model.activityLog.push({
        tick: model.tickCount,
        time: formatTickTime(model.tickCount),
        type: type,
        message: message
    });
    if (model.activityLog.length > LOG_MAX_ENTRIES) {
        model.activityLog.shift();
    }
    model.lastIntervention = type + ': ' + message;
}

// -- Button Helpers ----------------------------------------------------------

// Map compositor chrome ID to dashboard control key.
function buttonIdToKey(chromeId) {
    if (chromeId === 'btn-toggle') return 'a';
    if (chromeId === 'btn-kick') return 'k';
    if (chromeId === 'btn-input') return 'i';
    if (chromeId === 'btn-recover') return 'g';
    if (chromeId === 'btn-quit') return 'q';
    return null;
}

// Map bubbletea mouse button string to termmux mouse button string.
function mapMouseButton(btn) {
    if (btn === 'left') return 'left';
    if (btn === 'right') return 'right';
    if (btn === 'middle') return 'middle';
    if (btn === 'up' || btn === 'wheel up') return 'wheelUp';
    if (btn === 'down' || btn === 'wheel down') return 'wheelDown';
    return 'none';
}

// -- Dashboard Model ---------------------------------------------------------

function newDashboardModel(mgrRef, sidRef) {
    return {
        mgr: mgrRef,
        sid: sidRef,

        // Terminal dimensions
        width: 80,
        height: 24,

        // Compositor
        compObj: null,

        // Snapshot data
        snapshotAnsi: '',
        snapshotText: '',
        snapRows: 0,
        snapCols: 80,
        snapGen: 0,
        snapCursorRow: 0,
        snapCursorCol: 0,
        snapTimestamp: 0,
        snapStalenessMs: 0,

        // Autopilot state
        autopilotEnabled: AUTO_ENABLED,
        detectedState: 'EMPTY',
        stateTicks: 0,
        tickCount: 0,

        // Intervention tracking
        interventionCount: 0,
        lastInterventionTick: -9999,
        lastKickTick: -9999,

        // Activity log (ring buffer)
        activityLog: [],
        lastIntervention: '',
        activityScrollOffset: 0,

        // Child process
        childExited: false,

        // Input mode
        inputMode: false,
        inputBuffer: '',

        // PTY preview scroll
        scrollOffset: 0,

        // UI state
        focused: true,

        // Bubblezone controls line (set by renderChrome for zone.scan in view)
        controlsLine: '',

        // Bell notifications
        bellCount: 0
    };
}

// -- Shared state for event bus callbacks ------------------------------------
var sharedState = { m: null };

// -- Chrome Rendering --------------------------------------------------------
// Each chrome layer is rendered via lipgloss and placed at z=10.
// Buttons are individual chrome layers for compositor hit testing.

function renderChrome(m) {
    var W = m.width;
    var H = m.height;

    // -- Styles --
    var sTitle = lip.newStyle().bold(true).foreground(C.bright).background(C.chrome).padding(0, 1);
    var sDim = lip.newStyle().foreground(C.muted);
    var sDimBright = lip.newStyle().foreground(C.dim);
    var sKey = lip.newStyle().foreground(C.yellow).bold(true);
    var sBlue = lip.newStyle().foreground(C.blue).bold(true);
    var sCyan = lip.newStyle().foreground(C.cyan);
    var sGreen = lip.newStyle().foreground(C.green).bold(true);
    var sRed = lip.newStyle().foreground(C.red).bold(true);
    var sOrange = lip.newStyle().foreground(C.orange).bold(true);
    var sYellow = lip.newStyle().foreground(C.yellow).bold(true);
    var sOk = lip.newStyle().foreground(C.green);
    var sErr = lip.newStyle().foreground(C.red);
    var sStatusBar = lip.newStyle().background(C.barGreen).foreground(C.bg).bold(true);
    var sInputMode = lip.newStyle().foreground(C.cyan).bold(true);
    var sSeparator = lip.newStyle().foreground(C.border);

    // State color mapping
    var stateStyle = sGreen;
    if (m.detectedState === 'NETWORK_ERROR' || m.detectedState === 'ERROR') stateStyle = sRed;
    else if (m.detectedState === 'LOADING') stateStyle = sYellow;
    else if (m.detectedState === 'WAITING' || m.detectedState === 'IDLE_PROMPT') stateStyle = sOrange;
    else if (m.detectedState === 'EMPTY') stateStyle = sDim;

    var apLabel = m.autopilotEnabled ? 'ON' : 'OFF';
    var apStyle = m.autopilotEnabled ? sGreen : sErr;

    // -- Row 0: Title bar --
    var titleText = 'Claude Code Autopilot';
    if (m.snapCols !== m.width) {
        titleText += '  ' + m.snapCols + '×' + m.snapRows;
    }
    m.compObj.updateChrome({ id: 'title-bar', content: sTitle.render(padRight(titleText, W)) });

    // -- Row 1: State line --
    var stateLine = apStyle.render('AP:' + apLabel) + sDim.render(' │ ');
    stateLine += stateStyle.render(m.detectedState) + sDim.render(' │ ');
    stateLine += sDimBright.render(formatTickTime(m.tickCount));
    if (m.interventionCount > 0) {
        stateLine += sDim.render(' │ Interventions: ') + sCyan.render('' + m.interventionCount);
    }
    if (m.snapStalenessMs >= 0 && m.snapStalenessMs > 5000) {
        stateLine += sDim.render(' │ ') + sOrange.render('stale ' + Math.floor(m.snapStalenessMs / 1000) + 's');
    }
    if (m.snapGen > 0) {
        stateLine += sDim.render(' │ gen:') + sDimBright.render('' + m.snapGen);
    }
    if (!m.focused) {
        stateLine += sDim.render(' │ ') + sRed.render('UNFOCUSED');
    }
    m.compObj.updateChrome({ id: 'state-line', content: stateLine });

    // -- Row 2: Separator --
    m.compObj.updateChrome({ id: 'sep-top', content: sSeparator.render(padRight('', W).replace(/ /g, '─')) });

    // -- Footer chrome --
    var footerY = H - FOOTER_HEIGHT + 1;

    // -- Separator above activity --
    m.compObj.updateChrome({ id: 'sep-bottom', content: sSeparator.render(padRight('', W).replace(/ /g, '─')) });

    // -- Activity header --
    var logCount = m.activityLog.length;
    var logHeader = sBlue.render('▸ Activity');
    if (logCount > 0) {
        logHeader += sDim.render(' (' + logCount + ' entries)');
    }
    m.compObj.updateChrome({ id: 'activity-hdr', content: logHeader });

    // -- Activity log lines --
    var logVisibleLines = 2;
    var logStart = Math.max(0, logCount - logVisibleLines - m.activityScrollOffset);
    for (var li = 0; li < logVisibleLines; li++) {
        var logIdx = logStart + li;
        var chromeId = 'activity-' + li;
        if (logIdx < logCount && logIdx >= 0) {
            var entry = m.activityLog[logIdx];
            var entryStyle = sOk;
            if (entry.type === 'NETWORK_CANCEL' || entry.type === 'RECOVERY') entryStyle = sRed;
            else if (entry.type === 'AUTO_KICK' || entry.type === 'MANUAL_KICK') entryStyle = sYellow;
            else if (entry.type === 'INPUT' || entry.type === 'PASTE') entryStyle = sCyan;
            else if (entry.type === 'EXIT') entryStyle = sOrange;

            var entryText = sDimBright.render(entry.time) + ' ' +
                entryStyle.render(padRight(entry.type, 16)) + ' ' +
                sDim.render(entry.message);
            if (visualWidth(entryText) > W) {
                entryText = ansiTruncate(entryText, W);
            }
            m.compObj.updateChrome({ id: chromeId, content: entryText });
        } else if (logCount === 0 && li === 0) {
            m.compObj.updateChrome({ id: chromeId, content: sDim.render('  No interventions yet.') });
        } else {
            m.compObj.updateChrome({ id: chromeId, content: '' });
        }
    }

    // -- Controls bar: buttons or input mode (single chrome layer with zone.mark) --
    if (m.inputMode) {
        // Input mode replaces buttons on this row to avoid overlap
        m.controlsLine = '';
        m.compObj.updateChrome({ id: 'controls-bar', content: sInputMode.render('INPUT> ') + m.inputBuffer + '_' });
    } else {
        var toggleLabel = '[' + (m.autopilotEnabled ? '●' : '○') + '] Toggle';
        var kickLabel = '[k] Kick';
        var inputLabel = '[i] Input';
        var recoverLabel = '[g] Recover';
        var quitLabel = '[q] Quit';

        m.controlsLine = zone.mark('btn-toggle', sKey.render(toggleLabel));
        m.controlsLine += '  ';
        m.controlsLine += zone.mark('btn-kick', sKey.render(kickLabel));
        m.controlsLine += '  ';
        m.controlsLine += zone.mark('btn-input', sKey.render(inputLabel));
        m.controlsLine += '  ';
        m.controlsLine += zone.mark('btn-recover', sKey.render(recoverLabel));
        m.controlsLine += '  ';
        m.controlsLine += zone.mark('btn-quit', sKey.render(quitLabel));

        m.compObj.updateChrome({ id: 'controls-bar', content: m.controlsLine });
    }

    // -- Status bar --
    var timerText = formatTickTime(m.tickCount);
    var stateTag = m.detectedState;
    var barContent = ' [1] ' + timerText + '  ' + stateTag + '  ' + W + '×' + H + ' ';
    var barPad = Math.max(0, W - visualWidth(barContent));
    m.compObj.updateChrome({ id: 'status-bar', content: sStatusBar.render(barContent + padRight('', barPad)) });
}

// Initialize compositor with all chrome layers in their fixed positions.
function initCompositor(m) {
    var W = m.width;
    var H = m.height;

    // Create compositor with canvas dimensions
    m.compObj = compMod.compositor({ width: W, height: H });

    // PTY pane: full screen at z=0, content updated via updatePaneIfNew
    m.compObj.addPane({ id: 'pty', content: '', bounds: { x: 0, y: 0, width: W, height: H }, z: 0 });

    // Chrome layers at z=10: header rows
    m.compObj.addChrome({ id: 'title-bar', content: '', bounds: { x: 0, y: 0, width: W, height: 1 }, z: 10 });
    m.compObj.addChrome({ id: 'state-line', content: '', bounds: { x: 0, y: 1, width: W, height: 1 }, z: 10 });
    m.compObj.addChrome({ id: 'sep-top', content: '', bounds: { x: 0, y: 2, width: W, height: 1 }, z: 10 });

    // Footer chrome: rows from (H - FOOTER_HEIGHT + 1) to (H - 1)
    var footerY = H - FOOTER_HEIGHT + 1;
    m.compObj.addChrome({ id: 'sep-bottom', content: '', bounds: { x: 0, y: footerY, width: W, height: 1 }, z: 10 });
    m.compObj.addChrome({ id: 'activity-hdr', content: '', bounds: { x: 0, y: footerY + 1, width: W, height: 1 }, z: 10 });
    m.compObj.addChrome({ id: 'activity-0', content: '', bounds: { x: 0, y: footerY + 2, width: W, height: 1 }, z: 10 });
    m.compObj.addChrome({ id: 'activity-1', content: '', bounds: { x: 0, y: footerY + 3, width: W, height: 1 }, z: 10 });
    // Row footerY+4 = H-2 (spacer row between activity log and controls). Controls bar
    // serves double duty: buttons in normal mode, input prompt in input mode.
    m.compObj.addChrome({ id: 'controls-bar', content: '', bounds: { x: 0, y: H - 2, width: W, height: 1 }, z: 10 });
    m.compObj.addChrome({ id: 'status-bar', content: '', bounds: { x: 0, y: H - 1, width: W, height: 1 }, z: 10 });
}

// -- Dashboard Update --------------------------------------------------------

function dashboardUpdate(msg, model) {
    var m = model;
    sharedState.m = m;

    // -- WindowSize --
    if (msg.type === 'WindowSize') {
        m.width = msg.width;
        m.height = msg.height;
        // Rebuild compositor with new dimensions
        initCompositor(m);
        try { m.mgr.resize(m.height, m.width); } catch (e) {}
        return [m, tea.tick(TICK_MS, 'tick')];
    }

    // -- Tick --
    if (msg.type === 'Tick') {
        m.tickCount++;

        // Drain event bus
        try { m.mgr.pollEvents(); } catch (e) {}

        // Check child exit
        try {
            if (m.mgr.isDone(m.sid)) {
                if (!m.childExited) {
                    m.childExited = true;
                    addLogEntry(m, 'EXIT', 'child process exited');
                }
                return [m, tea.quit()];
            }
        } catch (e) {}

        // Take snapshot
        var snap = null;
        try { snap = m.mgr.snapshot(m.sid); } catch (e) {}

        if (snap) {
            m.snapshotAnsi = snap.ansi || '';
            m.snapshotText = snap.plainText || '';
            m.snapRows = snap.rows || 0;
            m.snapCols = snap.cols || m.width;
            m.snapGen = snap.gen || 0;
            m.snapCursorRow = snap.cursorRow || 0;
            m.snapCursorCol = snap.cursorCol || 0;
            m.snapTimestamp = snap.timestamp || 0;

            try { m.snapStalenessMs = m.mgr.lastActivityMs(m.sid); } catch (e) { m.snapStalenessMs = -1; }

            // State detection
            var prevState = m.detectedState;
            m.detectedState = analyzeSnapshot(snap.plainText);
            if (m.detectedState === prevState) { m.stateTicks++; } else { m.stateTicks = 0; }

            // Update PTY pane via compositor with generation-checked caching
            // (same pattern as splitlayout.go line 551)
            m.compObj.updatePaneIfNew({ id: 'pty', content: m.snapshotAnsi, gen: m.snapGen });

            // Autopilot interventions
            if (m.autopilotEnabled) {
                var ticksSinceIntervention = m.tickCount - m.lastInterventionTick;
                var canIntervene = ticksSinceIntervention >= INTERVENTION_COOLDOWN_TICKS;

                if (canIntervene) {
                    // NETWORK_ERROR -> Ctrl+C + Enter
                    if (m.detectedState === 'NETWORK_ERROR') {
                        try {
                            m.mgr.input('\x03');
                            m.mgr.input('\r');
                            m.lastInterventionTick = m.tickCount;
                            m.interventionCount++;
                            addLogEntry(m, 'NETWORK_CANCEL', 'sent Ctrl+C + Enter');
                        } catch (e) {}
                    }

                    // WAITING + idle -> Enter
                    if (m.detectedState === 'WAITING' && m.stateTicks >= IDLE_THRESHOLD_TICKS) {
                        try {
                            m.mgr.input('\r');
                            m.lastInterventionTick = m.tickCount;
                            m.interventionCount++;
                            addLogEntry(m, 'IDLE_ENTER', 'sent Enter to waiting process');
                        } catch (e) {}
                    }

                    // IDLE_PROMPT + stuck -> kick message
                    if (m.detectedState === 'IDLE_PROMPT' && m.stateTicks >= STUCK_THRESHOLD_TICKS) {
                        var ticksSinceKick = m.tickCount - m.lastKickTick;
                        if (ticksSinceKick >= KICK_INTERVAL_TICKS) {
                            try {
                                m.mgr.input(KICK_MSG + '\r');
                                m.lastKickTick = m.tickCount;
                                m.lastInterventionTick = m.tickCount;
                                m.interventionCount++;
                                addLogEntry(m, 'AUTO_KICK', KICK_MSG);
                            } catch (e) {}
                        }
                    }
                }
            }
        }

        return [m, tea.tick(TICK_MS, 'tick')];
    }

    // -- MouseClick with zone.inBounds for buttons + compositor.hit for PTY area --
    if (msg.type === 'MouseClick') {
        // Fine-grained button hit testing via bubblezone
        var btnIds = ['btn-toggle', 'btn-kick', 'btn-input', 'btn-recover', 'btn-quit'];
        for (var bi = 0; bi < btnIds.length; bi++) {
            if (zone.inBounds(btnIds[bi], msg)) {
                var btnKey = buttonIdToKey(btnIds[bi]);
                if (btnKey) {
                    return handleControlKey(m, btnKey);
                }
            }
        }
        // Coarse PTY area detection via compositor hit test
        var hitResult = m.compObj.hit(msg.x, msg.y);
        if (hitResult.hit && hitResult.id === 'pty') {
            var sgr = termmux.mouseToSGR(
                { type: 'MouseClick', button: mapMouseButton(msg.button), x: msg.x, y: msg.y },
                0, 0
            );
            if (sgr) { try { m.mgr.input(sgr); } catch (e) {} }
        }
        return [m, null];
    }

    // -- MouseMotion: forward to PTY for hover effects --
    if (msg.type === 'MouseMotion') {
        var hitMotion = m.compObj.hit(msg.x, msg.y);
        if (hitMotion.hit && hitMotion.id === 'pty') {
            var sgrMotion = termmux.mouseToSGR(
                { type: 'MouseMotion', button: mapMouseButton(msg.button), x: msg.x, y: msg.y },
                0, 0
            );
            if (sgrMotion) { try { m.mgr.input(sgrMotion); } catch (e) {} }
        }
        return [m, null];
    }

    // -- MouseRelease: forward to PTY --
    if (msg.type === 'MouseRelease') {
        var hitRelease = m.compObj.hit(msg.x, msg.y);
        if (hitRelease.hit && hitRelease.id === 'pty') {
            var sgrRelease = termmux.mouseToSGR(
                { type: 'MouseRelease', button: mapMouseButton(msg.button), x: msg.x, y: msg.y },
                0, 0
            );
            if (sgrRelease) { try { m.mgr.input(sgrRelease); } catch (e) {} }
        }
        return [m, null];
    }

    // -- MouseWheel: scroll PTY preview --
    if (msg.type === 'MouseWheel') {
        if (msg.button === 'up' || msg.button === 'wheel up') {
            m.scrollOffset = Math.max(0, m.scrollOffset - 3);
            return [m, null];
        }
        if (msg.button === 'down' || msg.button === 'wheel down') {
            var maxScroll = Math.max(0, (m.snapRows || 0) - 1);
            m.scrollOffset = Math.min(maxScroll, m.scrollOffset + 3);
            return [m, null];
        }
        return [m, null];
    }

    // -- Focus --
    if (msg.type === 'Focus') { m.focused = true; return [m, null]; }

    // -- Blur --
    if (msg.type === 'Blur') { m.focused = false; return [m, null]; }

    // -- Key --
    if (msg.type === 'Key') {
        // Input mode: accumulate text, Enter sends to PTY
        if (m.inputMode) {
            if (msg.key === 'escape' || msg.key === 'ctrl+c') {
                m.inputMode = false;
                m.inputBuffer = '';
                return [m, null];
            }
            if (msg.key === 'enter') {
                try {
                    m.mgr.input(m.inputBuffer + '\r');
                    addLogEntry(m, 'INPUT', m.inputBuffer);
                    m.lastInterventionTick = m.tickCount;
                } catch (e) {}
                m.inputBuffer = '';
                m.inputMode = false;
                return [m, null];
            }
            if (msg.key === 'backspace') {
                if (m.inputBuffer.length > 0) {
                    m.inputBuffer = m.inputBuffer.substring(0, m.inputBuffer.length - 1);
                }
                return [m, null];
            }
            // Paste support
            if (msg.text && msg.text.length > 1) {
                m.inputBuffer += msg.text;
                return [m, null];
            }
            if (msg.key.length === 1 && msg.text && msg.text.length >= 1) {
                m.inputBuffer += msg.text;
            }
            return [m, null];
        }

        // Scroll controls
        if (msg.key === 'up') {
            m.scrollOffset = Math.max(0, m.scrollOffset - 1);
            return [m, null];
        }
        if (msg.key === 'down') {
            var maxScroll2 = Math.max(0, (m.snapRows || 0) - 1);
            m.scrollOffset = Math.min(maxScroll2, m.scrollOffset + 1);
            return [m, null];
        }

        // Activity log scroll
        if (msg.key === 'shift+up') {
            m.activityScrollOffset = Math.max(0, m.activityScrollOffset - 1);
            return [m, null];
        }
        if (msg.key === 'shift+down') {
            m.activityScrollOffset = Math.min(
                Math.max(0, m.activityLog.length - 3),
                m.activityScrollOffset + 1
            );
            return [m, null];
        }

        // Dashboard control keys
        var ctrlResult = handleControlKey(m, msg.key);
        if (ctrlResult !== null) return ctrlResult;

        // Forward unhandled keys to PTY via keyToTermBytes
        var termBytes = termmux.keyToTermBytes(msg.key);
        if (termBytes !== null) {
            try { m.mgr.input(termBytes); } catch (e) {}
        } else if (msg.text) {
            try { m.mgr.input(msg.text); } catch (e) {}
        }
        return [m, null];
    }

    // -- Paste --
    if (msg.type === 'Paste') {
        if (m.inputMode && msg.content) {
            m.inputBuffer += msg.content;
            return [m, null];
        }
        if (msg.content) {
            try {
                m.mgr.input(msg.content);
                addLogEntry(m, 'PASTE', msg.content.substring(0, 40));
            } catch (e) {}
        }
        return [m, null];
    }

    return [m, null];
}

// -- Control Key Handler -----------------------------------------------------
// Returns [m, cmd] if handled, null if not handled (caller should forward to PTY).

function handleControlKey(m, key) {
    if (key === 'a') {
        m.autopilotEnabled = !m.autopilotEnabled;
        addLogEntry(m, 'TOGGLE', 'autopilot ' + (m.autopilotEnabled ? 'ENABLED' : 'DISABLED'));
        return [m, null];
    }
    if (key === 'k') {
        try {
            m.mgr.input(KICK_MSG + '\r');
            m.lastKickTick = m.tickCount;
            m.lastInterventionTick = m.tickCount;
            m.interventionCount++;
            addLogEntry(m, 'MANUAL_KICK', KICK_MSG);
        } catch (e) {}
        return [m, null];
    }
    if (key === 'i') {
        m.inputMode = true;
        m.inputBuffer = '';
        return [m, null];
    }
    if (key === 'g') {
        try {
            m.mgr.input('\x03');
            m.mgr.input('\r');
            m.lastInterventionTick = m.tickCount;
            m.interventionCount++;
            addLogEntry(m, 'RECOVERY', 'sent Ctrl+C + Enter');
        } catch (e) {}
        return [m, null];
    }
    if (key === 'q' || key === 'ctrl+c') {
        return [m, tea.quit()];
    }
    return null; // Not handled — caller should forward to PTY
}

// -- Dashboard View ----------------------------------------------------------

function dashboardView(model) {
    var m = model;

    // Update all chrome layer content
    renderChrome(m);

    // Render the compositor (PTY pane + chrome layers)
    var rendered = m.compObj.render();

    // Bubblezone zone.scan tracks positions via \n characters only — it cannot parse
    // the compositor's cursor positioning sequences (e.g. \x1b[row;colH). To register
    // zones at correct terminal coordinates, we scan the controls bar content separately
    // as a simple newline-padded string, then use the compositor output (with invisible
    // zero-width markers left in place) as the actual display content.
    var controlsY = m.height - 2;
    var scanPadding = '';
    for (var si = 0; si < controlsY; si++) scanPadding += '\n';
    if (m.inputMode) {
        // Input mode has no zones to register, but scan to clear stale zones
        zone.scan(scanPadding);
    } else if (m.controlsLine) {
        zone.scan(scanPadding + m.controlsLine);
    }

    // Cursor positioning
    var cursorObj = null;
    if (m.inputMode) {
        // In input mode, cursor at end of input buffer on the controls-bar row
        cursorObj = {
            x: 7 + visualWidth(m.inputBuffer),
            y: m.height - 2,
            shape: 'bar',
            blink: true,
            color: C.cyan
        };
    } else {
        // Normal mode: cursor at child process position (visible if not behind chrome)
        var cursorRow = m.snapCursorRow;
        var cursorCol = m.snapCursorCol;
        var cursorVisible = cursorRow >= HEADER_HEIGHT &&
            cursorRow < m.height - FOOTER_HEIGHT + 1 &&
            cursorCol >= 0 && cursorCol < m.width;
        if (cursorVisible) {
            cursorObj = {
                x: cursorCol,
                y: cursorRow,
                shape: 'bar',
                blink: true,
                color: C.cyan
            };
        }
    }

    return {
        content: rendered,
        altScreen: true,
        mouseMode: 'allMotion',
        reportFocus: true,
        windowTitle: 'Claude Code Autopilot — ' + m.detectedState,
        cursor: cursorObj,
        foregroundColor: C.text,
        backgroundColor: C.bg
    };
}

// -- Main -- Setup and Run ---------------------------------------------------

var session;
try {
    session = termmux.newCaptureSession(CMD, targetArgs, { rows: 24, cols: 80 });
    session.start();
} catch (e) {
    output.print('Failed to start capture session: ' + e.message);
    throw e;
}

var mgr;
var sid;
try {
    mgr = termmux.newSessionManager({ rows: 24, cols: 80 });
    mgr.run();
    mgr.started();

    sid = mgr.register(session, { name: 'claude', kind: 'capture' });
    mgr.activate(sid);
} catch (e) {
    output.print('Failed to register session: ' + e.message);
    session.close();
    throw e;
}

// -- Event Bus Listeners ----------------------------------------------------

mgr.on('exit', function(data) {
    if (sharedState.m) {
        sharedState.m.childExited = true;
        addLogEntry(sharedState.m, 'EXIT', 'session ended (reason: ' + (data.reason || 'unknown') + ')');
    }
});

mgr.on('bell', function(data) {
    if (sharedState.m) {
        sharedState.m.bellCount++;
        addLogEntry(sharedState.m, 'BELL', 'terminal bell received');
    }
});

mgr.on('terminal-resize', function(data) {
    if (sharedState.m && data) {
        addLogEntry(sharedState.m, 'RESIZE', (data.rows || '?') + 'x' + (data.cols || '?'));
    }
});

// -- Launch BubbleTea Dashboard ----------------------------------------------

var initialModel = newDashboardModel(mgr, sid);
initCompositor(initialModel);

var dashboardProgram = tea.newModel({
    init: function() {
        return [initialModel, tea.tick(TICK_MS, 'tick')];
    },
    update: dashboardUpdate,
    view: dashboardView
});

tea.run(dashboardProgram);
