#!/usr/bin/env osm script
// example-15-bouncing-logo.js — Bouncing Terminal: a nested PTY session bouncing around the screen
//
// Demonstrates: osm:termmux (newBoundedSession, newControlRouter, enableMouseForward,
// mouseToSGR, mouseDrag/handleMouseDrag), osm:termui/compositor (panes, chrome, hit),
// osm:bubbletea (tea.run, tea.tick, tea.newModel), osm:flag (argument parsing),
// osm:lipgloss (border styling).
//
// A fully interactive nested terminal session that physically bounces around the screen.
// Cross-platform: uses /bin/sh on Unix, cmd.exe on Windows.
//
// Usage:
//   osm script scripts/example-15-bouncing-logo.js
//   osm script scripts/example-15-bouncing-logo.js --cmd /bin/bash
//   osm script scripts/example-15-bouncing-logo.js -- bash -l

var termmux = require('osm:termmux');
var tea = require('osm:bubbletea');
var comp = require('osm:termui/compositor');
var lipgloss = require('osm:lipgloss');
var flag = require('osm:flag');

var isWindows = false;
try { if (require('osm:os').getenv('OS') === 'Windows_NT') isWindows = true; } catch (_) {}
var DEFAULT_CMD = isWindows ? 'cmd.exe' : '/bin/sh';

var fs = flag.newFlagSet('bouncing-logo');
fs.string('cmd', DEFAULT_CMD, 'command to run in the bouncing PTY');
fs.bool('smoke', false, 'run a non-interactive smoke test and exit');
var parseResult = fs.parse(args);
if (parseResult.error !== null) throw new Error('flag parse failed: ' + parseResult.error);
var CMD = fs.get('cmd');
var TARGET_ARGS = fs.args().length === 0 ? [] : fs.args();
var SMOKE = fs.get('smoke');

var CONFIG = {
    rows: 10,
    cols: 30,
    tickMs: 120,
    speedX: 1,
    speedY: 1,
    paneW: 32,
    paneH: 12,
    minW: 12,
    maxW: 62,
    minH: 7,
    maxH: 32,
    step: 2,
    controlsHeight: 2,
    borderWidth: 1,
};

var bounded = termmux.newBoundedSession({
    cmd: CMD,
    args: TARGET_ARGS,
    rows: CONFIG.rows,
    cols: CONFIG.cols,
});
var session = bounded.session;
var mgr = bounded.mgr;
var sid = bounded.sid;

mgr.resize(CONFIG.cols, CONFIG.rows + CONFIG.controlsHeight);

var state = {
    width: 80,
    height: 24,
    paneX: 1,
    paneY: 1,
    paneW: CONFIG.paneW,
    paneH: CONFIG.paneH,
    velX: CONFIG.speedX,
    velY: CONFIG.speedY,
    paused: false,
    bounces: 0,
    controlsHeight: CONFIG.controlsHeight,
    quit: false,
};

var c = comp.compositor({ width: state.width, height: state.height });
var borderStyle = lipgloss.newStyle().border(lipgloss.normalBorder()).foreground('#7AA2F7');
var chromeStyle = lipgloss.newStyle().foreground('#C0CAF5').background('#24283B');
var statusStyle = lipgloss.newStyle().foreground('#565F89');

var router = termmux.newControlRouter({
    keys: {
        'ctrl+p': 'pause',
        'ctrl+b': 'bigger',
        'ctrl+s': 'smaller',
        'ctrl+q': 'quit',
        'ctrl+c': 'quit',
    },
    chordMode: {
        prefix: 'ctrl+x',
        actions: {
            's': 'smaller',
            'b': 'bigger',
            'p': 'pause',
            'q': 'quit',
            'ctrl+c': 'quit',
        },
    },
});

var forwardMouse = termmux.enableMouseForward({
    sessionManager: mgr,
    sessionId: sid,
    compositor: c,
    paneId: 'pty',
    paneX: function() { return state.paneX; },
    paneY: function() { return state.paneY; },
    borderWidth: CONFIG.borderWidth,
    mouseToSGR: termmux.mouseToSGR,
});

function clampPane() {
    var maxX = state.width - state.paneW;
    var maxY = state.height - state.paneH - state.controlsHeight;
    if (state.paneX < 0) state.paneX = 0;
    if (state.paneX > maxX) state.paneX = maxX;
    if (state.paneY < 0) state.paneY = 0;
    if (state.paneY > maxY) state.paneY = maxY;
    if (maxX < 0) state.paneX = 0;
    if (maxY < 0) state.paneY = 0;
}

function resizeSession() {
    var innerW = Math.max(1, state.paneW - 2 * CONFIG.borderWidth);
    var innerH = Math.max(1, state.paneH - 2 * CONFIG.borderWidth);
    try { session.resize(innerH, innerW); } catch (_) {}
    try { mgr.resize(innerH, innerW); } catch (_) {}
}

function bigger() {
    if (state.paneW + CONFIG.step <= CONFIG.maxW) state.paneW += CONFIG.step;
    if (state.paneH + CONFIG.step <= CONFIG.maxH) state.paneH += CONFIG.step;
    clampPane();
    resizeSession();
}

function smaller() {
    if (state.paneW - CONFIG.step >= CONFIG.minW) state.paneW -= CONFIG.step;
    if (state.paneH - CONFIG.step >= CONFIG.minH) state.paneH -= CONFIG.step;
    clampPane();
    resizeSession();
}

function tick() {
    if (state.paused) return;
    state.paneX += state.velX;
    state.paneY += state.velY;

    var maxX = state.width - state.paneW;
    var maxY = state.height - state.paneH - state.controlsHeight;
    var bounced = false;
    if (state.paneX <= 0) {
        state.paneX = 0;
        state.velX = Math.abs(state.velX);
        bounced = true;
    } else if (state.paneX >= maxX) {
        state.paneX = Math.max(0, maxX);
        state.velX = -Math.abs(state.velX);
        bounced = true;
    }
    if (state.paneY <= 0) {
        state.paneY = 0;
        state.velY = Math.abs(state.velY);
        bounced = true;
    } else if (state.paneY >= maxY) {
        state.paneY = Math.max(0, maxY);
        state.velY = -Math.abs(state.velY);
        bounced = true;
    }
    if (bounced) state.bounces++;
}

function renderPane() {
    var snap = mgr.snapshot(sid);
    var text = snap && snap.plainText ? snap.plainText : '';
    var innerW = Math.max(1, state.paneW - 2 * CONFIG.borderWidth);
    var innerH = Math.max(1, state.paneH - 2 * CONFIG.borderWidth);
    return comp.renderBordered(text, lipgloss.normalBorder(), innerW, innerH);
}

function renderChrome() {
    var help = '^P pause  ^B bigger  ^S smaller  ^Q quit  ^X prefix';
    var status = 'x:' + state.paneX + ' y:' + state.paneY + ' w:' + state.paneW + ' h:' + state.paneH +
        ' bounces:' + state.bounces + (state.paused ? ' [PAUSED]' : '');
    var line = help + ' | ' + status;
    var width = Math.max(0, state.width);
    if (line.length > width) line = line.substring(0, width);
    else if (line.length < width) line = line + ' '.repeat(width - line.length);
    return chromeStyle.render(line);
}

function updateCompositor() {
    c.resize(state.width, state.height);
    c.updatePane({ id: 'pty', content: renderPane() });
    c.updateChrome({ id: 'status', content: renderChrome() });
}

function handleResize(width, height) {
    state.width = width;
    state.height = height;
    clampPane();
    updateCompositor();
    try { mgr.resize(height - state.controlsHeight, width); } catch (_) {}
}

function handleKey(key) {
    var r = router.handleKey(key);
    if (r.handled) {
        switch (r.action) {
            case 'pause': state.paused = !state.paused; return true;
            case 'bigger': bigger(); return true;
            case 'smaller': smaller(); return true;
            case 'quit': state.quit = true; return true;
        }
        return true;
    }
    var termBytes = termmux.keyToTermBytes(key, false, false);
    if (termBytes !== null && termBytes.length > 0) {
        mgr.input(termBytes);
    }
    return false;
}

function initModel() {
    c.addPane({
        id: 'pty',
        content: renderPane(),
        bounds: { x: state.paneX, y: state.paneY, width: state.paneW, height: state.paneH },
        z: 0,
    });
    c.addChrome({
        id: 'status',
        content: renderChrome(),
        bounds: { x: 0, y: Math.max(0, state.height - state.controlsHeight), width: state.width, height: state.controlsHeight },
        z: 10,
    });
    return [{}, tea.tick(CONFIG.tickMs, 'tick')];
}

function updateModel(msg, model) {
    if (msg.type === 'Tick') {
        tick();
        updateCompositor();
        return [model, tea.tick(CONFIG.tickMs, 'tick')];
    }
    if (msg.type === 'WindowSize') {
        handleResize(msg.width, msg.height);
        return [model, null];
    }
    if (msg.type === 'Key') {
        handleKey(msg.key);
        if (state.quit) return [model, tea.quit()];
        return [model, null];
    }
    if (msg.type === 'MouseClick' || msg.type === 'MouseMotion' || msg.type === 'MouseRelease' || msg.type === 'MouseWheel') {
        var drag = termmux.handleMouseDrag({ manager: mgr, msg: msg });
        if (drag && drag.handled) return [model, drag.cmd];
        forwardMouse(msg);
        return [model, null];
    }
    return [model, null];
}

function viewModel(model) {
    return { content: c.render(), altScreen: true, mouseMode: true };
}

function runSmoke() {
    handleResize(80, 24);
    for (var i = 0; i < 10; i++) {
        tick();
        updateCompositor();
    }
    var view = viewModel({});
    output.print('smoke: rendered content length=' + (view.content || '').length);
    output.print('smoke: bounceCount=' + state.bounces);
    if (state.bounces <= 0) throw new Error('smoke: expected bounces after ticks');
    try { session.close(); } catch (_) {}
    try { mgr.close(); } catch (_) {}
}

if (SMOKE) {
    runSmoke();
} else {
    var program = tea.newModel({
        init: initModel,
        update: updateModel,
        view: viewModel,
    });
    tea.run(program);
    try { session.close(); } catch (_) {}
    try { mgr.close(); } catch (_) {}
}
