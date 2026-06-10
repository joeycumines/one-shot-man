#!/usr/bin/env osm script
// example-15-bouncing-logo.js — Bouncing Terminal: a nested PTY session bouncing around the screen
//
// Demonstrates: osm:termmux (PTY management), osm:termui/compositor (layered rendering),
// osm:bubbletea (TUI engine), osm:lipgloss (styling), osm:bubblezone (mouse zones),
// osm:unicodetext (proper Unicode width), osm:flag (argument parsing).
//
// A fully interactive nested terminal session that physically bounces around the screen.
// Supports: keyboard input forwarding, mouse forwarding with coordinate translation,
// resize, pause/resume, pane size adjustment, and clean exit.
// Cross-platform: uses /bin/sh on Unix, cmd.exe on Windows.
//
// Usage:
//   osm script scripts/example-15-bouncing-logo.js
//   osm script scripts/example-15-bouncing-logo.js --cmd /bin/bash
//   osm script scripts/example-15-bouncing-logo.js -- bash -l

// -- Module Requires ----------------------------------------------------------

var termmux, tea, lip, flag, compMod, zone, unicodetext;

try {
    termmux = require('osm:termmux');
    tea = require('osm:bubbletea');
    lip = require('osm:lipgloss');
    flag = require('osm:flag');
    compMod = require('osm:termui/compositor');
    zone = require('osm:bubblezone');
    unicodetext = require('osm:unicodetext');
} catch (e) {
    throw new Error('Failed to load modules: ' + e.message);
}

// -- Constants ----------------------------------------------------------------

var TICK_MS = 120;
var BOUNCE_SPEED_X = 1;
var BOUNCE_SPEED_Y = 1;
var BORDER_WIDTH = 1;
var DEFAULT_PANE_WIDTH = 32;
var DEFAULT_PANE_HEIGHT = 12;
var MIN_PANE_WIDTH = 12;
var MIN_PANE_HEIGHT = 7;
var MAX_PANE_WIDTH = 62;
var MAX_PANE_HEIGHT = 32;
var CONTROLS_HEIGHT = 2;   // 1 row buttons + 1 row status
var PANE_SIZE_STEP = 2;

// -- Cross-Platform Shell Detection -------------------------------------------

var isWindows = false;
try {
    var osMod = require('osm:os');
    if (osMod && osMod.getenv('OS') === 'Windows_NT') {
        isWindows = true;
    }
} catch (_) {}

var DEFAULT_CMD = isWindows ? 'cmd.exe' : '/bin/sh';

// -- Argument Parsing ---------------------------------------------------------

var fs = flag.newFlagSet('bouncing-logo');
fs.string('cmd', DEFAULT_CMD, 'command to run in the bouncing PTY');

var parseResult = fs.parse(args);
if (parseResult.error !== null) {
    throw new Error('flag parse failed: ' + parseResult.error);
}

var CMD = fs.get('cmd');
var targetArgs = fs.args();
if (targetArgs.length === 0) {
    targetArgs = [];
}

// -- Theme (Tokyo Night) -----------------------------------------------------

var C = {
    bg:          '#1A1B26',
    surface:     '#24283B',
    chrome:      '#292E42',
    border:      '#3B4261',
    muted:       '#565F89',
    dim:         '#787C99',
    text:        '#C0CAF5',
    bright:      '#D5D6DB',
    cyan:        '#7DCFFF',
    blue:        '#7AA2F7',
    purple:      '#BB9AF7',
    green:       '#9ECE6A',
    brightGreen: '#73DACA',
    yellow:      '#E0AF68',
    orange:      '#FF9E64',
    red:         '#F7768E',
};

var borderStyle = lip.newStyle().
    border(lip.normalBorder()).
    borderForeground(C.cyan).
    padding(0);

// -- Formatting Helpers -------------------------------------------------------

function padRight(s, n) {
    s = String(s);
    var w = unicodetext.width(s);
    if (w >= n) return unicodetext.truncate(s, n, '');
    var padLen = n - w;
    var pad = '';
    while (pad.length < padLen) pad += ' ';
    return s + pad;
}

function formatTickTime(ticks) {
    var totalSec = Math.floor(ticks * TICK_MS / 1000);
    var m = Math.floor(totalSec / 60);
    var s = totalSec % 60;
    var mm = m < 10 ? '0' + m : '' + m;
    var ss = s < 10 ? '0' + s : '' + s;
    return mm + ':' + ss;
}

// -- Logging Helper -----------------------------------------------------------

function logCatch(label, e) {
    try { log.warn('bouncing-logo catch', { label: label, error: e.message || String(e) }); } catch (_) {}
}

// -- Button Helpers -----------------------------------------------------------

function buttonIdToKey(chromeId) {
    if (chromeId === 'btn-pause') return 'ctrl+p';
    if (chromeId === 'btn-bigger') return 'ctrl+b';
    if (chromeId === 'btn-smaller') return 'ctrl+s';
    if (chromeId === 'btn-quit') return 'ctrl+q';
    return null;
}

// mapMouseButton converts BubbleTea's mouse button name to the format
// expected by Go's termmux.MouseToSGR. BubbleTea and Go share the same
// naming for standard buttons ("left", "middle", "right", "wheel up",
// "wheel down", "none"), so recognized names pass through unchanged.
// Exotic names like "button 10" or "button 11" (from extended mouse
// protocols) are not supported by Go's MouseToSGR and map to "none".
function mapMouseButton(btn) {
    if (btn === 'left' || btn === 'middle' || btn === 'right' ||
        btn === 'wheel up' || btn === 'wheel down' ||
        btn === 'wheel left' || btn === 'wheel right' ||
        btn === 'backward' || btn === 'forward' || btn === 'none') {
        return btn;
    }
    return 'none';
}

// -- Dashboard Model ----------------------------------------------------------

function newBouncingModel(mgrRef, sidRef) {
    return {
        mgr: mgrRef,
        sid: sidRef,

        // Terminal dimensions
        width: 80,
        height: 24,

        // Compositor
        compObj: null,

        // Bounce state
        paneX: 0,
        paneY: 0,
        paneW: DEFAULT_PANE_WIDTH,
        paneH: DEFAULT_PANE_HEIGHT,
        velX: BOUNCE_SPEED_X,
        velY: BOUNCE_SPEED_Y,
        paused: false,
        bounceCount: 0,

        // Snapshot data
        snapshotAnsi: '',
        snapshotText: '',
        snapGen: 0,
        mouseTracking: 0,
        mouseSGR: false,

        // Child process
        childExited: false,

        // Bell notifications
        bellCount: 0,

        // UI state
        focused: true,
        controlsLine: '',
        tickCount: 0,
        chordMode: false
    };
}

// -- Shared state for event bus callbacks -------------------------------------

var sharedState = { m: null };

// -- Compositor Setup ---------------------------------------------------------

function initCompositor(m) {
    var W = m.width;
    var H = m.height;

    m.compObj = compMod.compositor({ width: W, height: H });

    // PTY pane (bouncing, z=0)
    m.compObj.addPane({
        id: 'pty',
        content: '',
        bounds: { x: m.paneX, y: m.paneY, width: m.paneW, height: m.paneH },
        z: 0
    });

    // Fixed chrome: controls bar + status bar (z=10)
    var controlsY = H - CONTROLS_HEIGHT;
    m.compObj.addChrome({ id: 'controls', content: '', bounds: { x: 0, y: controlsY, width: W, height: 1 }, z: 10 });
    m.compObj.addChrome({ id: 'status', content: '', bounds: { x: 0, y: controlsY + 1, width: W, height: 1 }, z: 10 });
}

// -- Rebuild the bouncing pane at new position --------------------------------

function repositionPtyPane(m) {
    try {
        m.compObj.removePane('pty');
    } catch (_) {}
    // lipgloss .width(N) includes borders in N, so content area = N - borderSize.
    // The VT is sized to (paneW - 2, paneH - 2) = (innerW, innerH).
    // To make the content area match the VT, we pass the full pane dimensions
    // so lipgloss subtracts its 2-char border and produces paneW-2 cols of content.
    var bordered = borderStyle.width(m.paneW).height(m.paneH).render(m.snapshotAnsi || '');
    m.compObj.addPane({
        id: 'pty',
        content: bordered,
        bounds: { x: m.paneX, y: m.paneY, width: m.paneW, height: m.paneH },
        z: 0
    });
}

// -- Chrome Rendering ---------------------------------------------------------

function renderControls(m) {
    var W = m.width;

    var sKey = lip.newStyle().foreground(C.yellow).bold(true);
    var sDim = lip.newStyle().foreground(C.muted);
    var sCyan = lip.newStyle().foreground(C.cyan);
    var sGreen = lip.newStyle().foreground(C.green).bold(true);
    var sRed = lip.newStyle().foreground(C.red).bold(true);
    var sStatus = lip.newStyle().background(C.surface).foreground(C.dim);
    var sStatusVal = lip.newStyle().background(C.surface).foreground(C.cyan);

    // -- Controls bar (row 0: buttons) --
    var chordPrefix = m.chordMode ? sKey.render('C-x-') + '  ' : '';
    var pauseLabel = m.paused ? '[^P] Resume' : '[^P] Pause';
    var biggerLabel = '[^B] Bigger';
    var smallerLabel = '[^S] Smaller';
    var quitLabel = '[^Q] Quit';

    m.controlsLine = chordPrefix;
    m.controlsLine += zone.mark('btn-pause', sKey.render(pauseLabel));
    m.controlsLine += '  ';
    m.controlsLine += zone.mark('btn-bigger', sKey.render(biggerLabel));
    m.controlsLine += '  ';
    m.controlsLine += zone.mark('btn-smaller', sKey.render(smallerLabel));
    m.controlsLine += '  ';
    m.controlsLine += zone.mark('btn-quit', sRed.render(quitLabel));

    // Fill the rest with dim dots
    var controlsWidth = unicodetext.width(m.controlsLine);
    if (controlsWidth < W) {
        // Zone markers are zero-width but contribute to the raw string length
        // Just pad with spaces to fill the row
        m.controlsLine += sDim.render(padRight('', W - controlsWidth));
    }

    m.compObj.updateChrome({ id: 'controls', content: m.controlsLine });

    // -- Status bar (row 1: info) --
    var posText = sStatusVal.render(m.paneX + ',' + m.paneY);
    var sizeText = sStatusVal.render(m.paneW + 'x' + m.paneH);
    var bouncesText = sStatusVal.render('' + m.bounceCount);
    var timeText = sStatusVal.render(formatTickTime(m.tickCount));
    var stateText = m.childExited ? sRed.render('EXITED') : sGreen.render('RUNNING');
    var focusText = m.focused ? '' : sStatus.render(' ') + sRed.render('UNFOCUSED');

    var statusContent = sStatus.render(' Pos:') + posText;
    statusContent += sStatus.render(' Size:') + sizeText;
    statusContent += sStatus.render(' Bounces:') + bouncesText;
    if (m.bellCount > 0) {
        statusContent += sStatus.render(' Bells:') + sStatusVal.render('' + m.bellCount);
    }
    statusContent += sStatus.render(' ') + stateText;
    statusContent += sStatus.render(' ') + timeText;
    statusContent += focusText;

    var statusWidth = unicodetext.width(statusContent);
    if (statusWidth < W) {
        statusContent += sStatus.render(padRight('', W - statusWidth));
    }

    m.compObj.updateChrome({ id: 'status', content: statusContent });
}

// -- Bounce Physics -----------------------------------------------------------

function updateBounce(m) {
    if (m.paused) return;

    m.paneX += m.velX;
    m.paneY += m.velY;

    var maxPaneX = m.width - m.paneW;
    var maxPaneY = (m.height - CONTROLS_HEIGHT) - m.paneH;

    // Bounce off right/left walls
    if (m.paneX >= maxPaneX) {
        m.paneX = maxPaneX;
        m.velX = -Math.abs(m.velX);
        m.bounceCount++;
    } else if (m.paneX <= 0) {
        m.paneX = 0;
        m.velX = Math.abs(m.velX);
        m.bounceCount++;
    }

    // Bounce off bottom/top walls
    if (m.paneY >= maxPaneY) {
        m.paneY = maxPaneY;
        m.velY = -Math.abs(m.velY);
        m.bounceCount++;
    } else if (m.paneY <= 0) {
        m.paneY = 0;
        m.velY = Math.abs(m.velY);
        m.bounceCount++;
    }

    // Safety clamp
    if (maxPaneX > 0) {
        m.paneX = Math.max(0, Math.min(m.paneX, maxPaneX));
    } else {
        m.paneX = 0;
    }
    if (maxPaneY > 0) {
        m.paneY = Math.max(0, Math.min(m.paneY, maxPaneY));
    } else {
        m.paneY = 0;
    }
}

// -- Mouse Forwarding Guard ---------------------------------------------------

// Only forward mouse events when the child process has explicitly enabled mouse
// tracking (DECSET 1000/1002/1003). Without this guard, mouse SGR escape
// sequences are injected into a shell that never requested them, causing raw
// escape text to appear on screen (e.g. "35;1;5M" after typing `stty sane`).
//
// mouseTracking values: 0=none, 1=basic, 2=button-event, 3=any-event
// mouseSGR: true when child enabled SGR encoding (DECSET 1006)

function shouldForwardMouse(m) {
    return m.mouseTracking > 0;
}

// -- Update -------------------------------------------------------------------

function bouncingUpdate(msg, model) {
    var m = model;
    sharedState.m = m;

    // -- WindowSize --
    if (msg.type === 'WindowSize') {
        m.width = msg.width;
        m.height = msg.height;

        m.compObj.resize(m.width, m.height);

        // Rebuild chrome at new positions
        var controlsY = m.height - CONTROLS_HEIGHT;
        try { m.compObj.removeChrome('controls'); } catch (_) {}
        try { m.compObj.removeChrome('status'); } catch (_) {}
        m.compObj.addChrome({ id: 'controls', content: '', bounds: { x: 0, y: controlsY, width: m.width, height: 1 }, z: 10 });
        m.compObj.addChrome({ id: 'status', content: '', bounds: { x: 0, y: controlsY + 1, width: m.width, height: 1 }, z: 10 });

        // Rebuild pane at current position
        repositionPtyPane(m);

        // Resize child PTY
        try { m.mgr.resize(m.paneH - 2 * BORDER_WIDTH, m.paneW - 2 * BORDER_WIDTH); } catch (e) { logCatch('resize', e); }

        return [m, tea.tick(TICK_MS, 'tick')];
    }

    // -- Tick --
    if (msg.type === 'Tick') {
        m.tickCount++;

        // Bounce physics
        updateBounce(m);

        // Drain termmux event bus
        try { m.mgr.pollEvents(); } catch (e) { logCatch('pollEvents', e); }

        // Check child exit
        try {
            if (m.mgr.isDone(m.sid)) {
                if (!m.childExited) {
                    m.childExited = true;
                }
                return [m, tea.quit()];
            }
        } catch (e) { logCatch('isDone', e); }

        // Take snapshot
        var snap = null;
        try { snap = m.mgr.snapshot(m.sid); } catch (e) { logCatch('snapshot', e); }

        if (snap) {
            m.snapshotAnsi = snap.ansi || '';
            m.snapshotText = snap.plainText || '';
            m.snapGen = snap.gen || 0;
            m.mouseTracking = snap.mouseTracking || 0;
            m.mouseSGR = snap.mouseSGR || false;

            // Update PTY pane content and position
            repositionPtyPane(m);
        } else {
            // Still reposition even if no new snapshot
            repositionPtyPane(m);
        }

        return [m, tea.tick(TICK_MS, 'tick')];
    }

    // -- MouseClick with zone + compositor hit test --
    if (msg.type === 'MouseClick') {
        // Check bubblezone buttons first
        var btnIds = ['btn-pause', 'btn-bigger', 'btn-smaller', 'btn-quit'];
        for (var bi = 0; bi < btnIds.length; bi++) {
            if (zone.inBounds(btnIds[bi], msg)) {
                var btnKey = buttonIdToKey(btnIds[bi]);
                if (btnKey) {
                    return handleControlKey(m, btnKey);
                }
            }
        }

        // Forward to PTY only if child has enabled mouse tracking
        if (shouldForwardMouse(m)) {
            var hitResult = m.compObj.hit(msg.x, msg.y);
            if (hitResult.hit && hitResult.id === 'pty') {
                var relX = msg.x - m.paneX - BORDER_WIDTH;
                var relY = msg.y - m.paneY - BORDER_WIDTH;
                var sgr = termmux.mouseToSGR(
                    { type: 'MouseClick', button: mapMouseButton(msg.button), x: relX, y: relY },
                    0, 0
                );
                if (sgr) { try { m.mgr.input(sgr); } catch (e) { logCatch('mouse-click-sgr', e); } }
            }
        }
        return [m, null];
    }

    // -- MouseMotion: forward to PTY for hover effects --
    if (msg.type === 'MouseMotion') {
        if (shouldForwardMouse(m)) {
            var hitMotion = m.compObj.hit(msg.x, msg.y);
            if (hitMotion.hit && hitMotion.id === 'pty') {
                var relXM = msg.x - m.paneX - BORDER_WIDTH;
                var relYM = msg.y - m.paneY - BORDER_WIDTH;
                var sgrMotion = termmux.mouseToSGR(
                    { type: 'MouseMotion', button: 'none', x: relXM, y: relYM },
                    0, 0
                );
                if (sgrMotion) { try { m.mgr.input(sgrMotion); } catch (e) { logCatch('mouse-motion-sgr', e); } }
            }
        }
        return [m, null];
    }

    // -- MouseRelease: forward to PTY --
    if (msg.type === 'MouseRelease') {
        if (shouldForwardMouse(m)) {
            var hitRelease = m.compObj.hit(msg.x, msg.y);
            if (hitRelease.hit && hitRelease.id === 'pty') {
                var relXR = msg.x - m.paneX - BORDER_WIDTH;
                var relYR = msg.y - m.paneY - BORDER_WIDTH;
                var sgrRelease = termmux.mouseToSGR(
                    { type: 'MouseRelease', button: 'left', x: relXR, y: relYR },
                    0, 0
                );
                if (sgrRelease) { try { m.mgr.input(sgrRelease); } catch (e) { logCatch('mouse-release-sgr', e); } }
            }
        }
        return [m, null];
    }

    // -- MouseWheel: forward to PTY --
    if (msg.type === 'MouseWheel') {
        if (shouldForwardMouse(m)) {
            var hitWheel = m.compObj.hit(msg.x, msg.y);
            if (hitWheel.hit && hitWheel.id === 'pty') {
                var relXW = msg.x - m.paneX - BORDER_WIDTH;
                var relYW = msg.y - m.paneY - BORDER_WIDTH;
                var sgrWheel = termmux.mouseToSGR(
                    { type: 'MouseClick', button: mapMouseButton(msg.button), x: relXW, y: relYW },
                    0, 0
                );
                if (sgrWheel) { try { m.mgr.input(sgrWheel); } catch (e) { logCatch('mouse-wheel-sgr', e); } }
            }
        }
        return [m, null];
    }

    // -- Focus --
    if (msg.type === 'Focus') { m.focused = true; return [m, null]; }

    // -- Blur --
    if (msg.type === 'Blur') { m.focused = false; return [m, null]; }

    // -- Key --
    if (msg.type === 'Key') {
        // Chord mode: waiting for second key after ctrl+x
        if (m.chordMode) {
            m.chordMode = false;
            var ck = msg.key;
            if (ck === 's' || ck === 'ctrl+s') { return handleControlKey(m, 'ctrl+s'); }
            if (ck === 'b' || ck === 'ctrl+b') { return handleControlKey(m, 'ctrl+b'); }
            if (ck === 'p' || ck === 'ctrl+p') { return handleControlKey(m, 'ctrl+p'); }
            if (ck === 'q' || ck === 'ctrl+q') { return handleControlKey(m, 'ctrl+q'); }
            if (ck === 'ctrl+c') { return handleControlKey(m, 'ctrl+c'); }
            if (ck === 'esc' || ck === 'ctrl+x') { return [m, null]; }
            // Unrecognized chord key: fall through to normal handling
        }

        // Check control keys first
        var ctrlResult = handleControlKey(m, msg.key);
        if (ctrlResult !== null) return ctrlResult;

        // Forward unhandled keys to PTY
        var termBytes = termmux.keyToTermBytes(msg.key);
        if (termBytes !== null) {
            try { m.mgr.input(termBytes); } catch (e) { logCatch('key-forward', e); }
        } else if (msg.text) {
            try { m.mgr.input(msg.text); } catch (e) { logCatch('key-text', e); }
        }
        return [m, null];
    }

    // -- Paste --
    if (msg.type === 'Paste') {
        if (msg.content) {
            try { m.mgr.input(msg.content); } catch (e) { logCatch('paste', e); }
        }
        return [m, null];
    }

    return [m, null];
}

// -- Control Key Handler ------------------------------------------------------

function handleControlKey(m, key) {
    // Pause/resume bounce animation — ctrl+p only (bare 'p' forwarded to PTY)
    if (key === 'ctrl+p') {
        m.paused = !m.paused;
        return [m, null];
    }
    // Bigger pane — ctrl+b only (bare 'b' forwarded to PTY)
    if (key === 'ctrl+b') {
        if (m.paneW < MAX_PANE_WIDTH) {
            m.paneW = Math.min(MAX_PANE_WIDTH, m.paneW + PANE_SIZE_STEP);
            m.paneH = Math.min(MAX_PANE_HEIGHT, m.paneH + PANE_SIZE_STEP);
            try { m.mgr.resize(m.paneH - 2 * BORDER_WIDTH, m.paneW - 2 * BORDER_WIDTH); } catch (e) { logCatch('resize-bigger', e); }
            repositionPtyPane(m);
        }
        return [m, null];
    }
    // Smaller pane — ctrl+s only (bare 's' forwarded to PTY)
    if (key === 'ctrl+s') {
        if (m.paneW > MIN_PANE_WIDTH) {
            m.paneW = Math.max(MIN_PANE_WIDTH, m.paneW - PANE_SIZE_STEP);
            m.paneH = Math.max(MIN_PANE_HEIGHT, m.paneH - PANE_SIZE_STEP);
            try { m.mgr.resize(m.paneH - 2 * BORDER_WIDTH, m.paneW - 2 * BORDER_WIDTH); } catch (e) { logCatch('resize-smaller', e); }
            repositionPtyPane(m);
        }
        return [m, null];
    }
    // Quit — ctrl+c or ctrl+q (bare 'q' forwarded to PTY)
    if (key === 'ctrl+c' || key === 'ctrl+q') {
        return [m, tea.quit()];
    }
    // Enter chord mode — ctrl+x followed by action key
    if (key === 'ctrl+x') {
        m.chordMode = true;
        return [m, null];
    }
    return null; // Not handled — caller should forward to PTY
}

// -- View ---------------------------------------------------------------------

function bouncingView(model) {
    var m = model;

    // Guard against null compositor (before init)
    if (!m || !m.compObj) return { content: 'Initializing...', altScreen: true };

    // Render controls chrome
    try { renderControls(m); } catch (e) { logCatch('renderControls', e); }

    // Render the compositor (PTY pane + chrome layers)
    var rendered = '';
    try {
        rendered = m.compObj.render();
    } catch (e) {
        rendered = 'Render error: ' + (e.message || String(e));
    }

    // Ensure content is never empty (prevents [object Object] fallback)
    if (!rendered) rendered = '\n';

    // Zone scan for the controls bar
    var controlsY = m.height - 2;
    var scanPadding = '';
    for (var si = 0; si < controlsY; si++) scanPadding += '\n';
    if (m.controlsLine) {
        zone.scan(scanPadding + m.controlsLine);
    } else {
        zone.scan(scanPadding);
    }

    return {
        content: rendered,
        altScreen: true,
        mouseMode: 'allMotion',
        reportFocus: true,
        windowTitle: 'Bouncing Terminal \u2014 ' + m.bounceCount + ' bounces',
        foregroundColor: C.text,
        backgroundColor: C.bg
    };
}

// -- Main: Setup and Run ------------------------------------------------------

var session;
try {
    session = termmux.newCaptureSession(CMD, targetArgs, { rows: DEFAULT_PANE_HEIGHT - 2 * BORDER_WIDTH, cols: DEFAULT_PANE_WIDTH - 2 * BORDER_WIDTH });
    session.start();
} catch (e) {
    output.print('Failed to start capture session: ' + e.message);
    throw e;
}

var mgr;
var sid;
try {
    mgr = termmux.newSessionManager({ rows: DEFAULT_PANE_HEIGHT - 2 * BORDER_WIDTH, cols: DEFAULT_PANE_WIDTH - 2 * BORDER_WIDTH });
    mgr.run();
    mgr.started();

    sid = mgr.register(session, { name: 'bouncing', kind: 'capture' });
    mgr.activate(sid);
} catch (e) {
    output.print('Failed to register session: ' + e.message);
    try { if (sid) mgr.deactivate(sid); } catch (_) {}
    try { session.close(); } catch (_) {}
    throw e;
}

// -- Event Bus Listeners ------------------------------------------------------

mgr.on('exit', function(data) {
    if (sharedState.m) {
        sharedState.m.childExited = true;
    }
});

mgr.on('bell', function(data) {
    if (sharedState.m) {
        sharedState.m.bellCount++;
    }
});

// -- Launch BubbleTea ---------------------------------------------------------

var initialModel = newBouncingModel(mgr, sid);
initCompositor(initialModel);

var bouncingProgram = tea.newModel({
    init: function() {
        return [initialModel, tea.tick(TICK_MS, 'tick')];
    },
    update: bouncingUpdate,
    view: bouncingView
});

tea.run(bouncingProgram);
