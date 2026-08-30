#!/usr/bin/env osm script
// example-15-bouncing-logo.js — Bouncing Terminal: a full-colour, mouse/keyboard-capable PTY bouncing around the screen
//
// Demonstrates a thin JavaScript orchestration layer over reusable Go primitives:
//   - osm:termmux      (bounded session, control router, copy mode, prefix dispatcher)
//   - osm:termui/termpane (real BubbleTea-model terminal pane: colour, mouse, keys, resize)
//   - osm:termui/compositor (panes, chrome, generation-cached rendering)
//   - osm:bubbletea    (model/update/view/run/tick)
//   - osm:lipgloss     (border/style helpers)
//   - osm:flag         (typed flags)
//   - osm:os           (environment detection)
//
// Usage:
//   osm script scripts/example-15-bouncing-logo.js
//   osm script scripts/example-15-bouncing-logo.js --cmd /bin/bash
//   osm script scripts/example-15-bouncing-logo.js -- bash -l

var termmux = require('osm:termmux');
var tea = require('osm:bubbletea');
var tp = require('osm:termui/termpane');
var comp = require('osm:termui/compositor');
var lipgloss = require('osm:lipgloss');
var flag = require('osm:flag');
var os = require('osm:os');

// ── Configuration ────────────────────────────────────────────────────────────

var isWindows = false;
try { if (os.getenv('OS') === 'Windows_NT') isWindows = true; } catch (_) {}
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

// ── Bootstrap termmux session ─────────────────────────────────────────────────

var bounded = null;
var mgr = null;
var sid = null;
var pane = null;
var sessionReady = termmux.newBoundedSession({
    cmd: CMD,
    args: TARGET_ARGS,
    rows: CONFIG.paneH - 2 * CONFIG.borderWidth,
    cols: CONFIG.paneW - 2 * CONFIG.borderWidth,
    passthrough: false,
}).then(function(b) {
    bounded = b;
    mgr = bounded.mgr;
    sid = bounded.sid;
    pane = tp.termpane({
        manager: mgr,
        sessionId: sid,
        bounds: { x: state.paneX, y: state.paneY, width: state.paneW, height: state.paneH },
    });
});

// ── Model / Physics ──────────────────────────────────────────────────────────

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
    tickCount: 0,
    controlsHeight: CONFIG.controlsHeight,
    quit: false,
    muxPrefix: false,
    message: '',
    messageUntil: 0,
    cursor: null,
};

var c = comp.compositor({ width: state.width, height: state.height });

var chromeStyle = lipgloss.newStyle().foreground('#C0CAF5').background('#24283B');
var titleStyle = lipgloss.newStyle().foreground('#7AA2F7').bold(true);
var statusStyle = lipgloss.newStyle().foreground('#565F89');
var highlightStyle = lipgloss.newStyle().foreground('#E0AF68').bold(true);
var warningStyle = lipgloss.newStyle().foreground('#FF5370').bold(true);

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
            'c': 'copyMode',
            'ctrl+c': 'quit',
        },
    },
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

function paneInnerSize() {
    return {
        w: Math.max(1, state.paneW - 2 * CONFIG.borderWidth),
        h: Math.max(1, state.paneH - 2 * CONFIG.borderWidth),
    };
}

function paneOuterBounds() {
    return { x: state.paneX, y: state.paneY, width: state.paneW, height: state.paneH };
}

function resizePane() {
    var inner = paneInnerSize();
    pane.setBounds(paneOuterBounds());
    pane.update({ type: 'WindowSize', width: inner.w, height: inner.h });
}

function bigger() {
    if (state.paneW + CONFIG.step <= CONFIG.maxW) state.paneW += CONFIG.step;
    if (state.paneH + CONFIG.step <= CONFIG.maxH) state.paneH += CONFIG.step;
    clampPane();
    resizePane();
    setMessage('bigger');
}

function smaller() {
    if (state.paneW - CONFIG.step >= CONFIG.minW) state.paneW -= CONFIG.step;
    if (state.paneH - CONFIG.step >= CONFIG.minH) state.paneH -= CONFIG.step;
    clampPane();
    resizePane();
    setMessage('smaller');
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
    state.tickCount++;
}

function setMessage(text) {
    state.message = text;
    state.messageUntil = Date.now() + 1000;
}

function activeMessage() {
    if (state.message && Date.now() < state.messageUntil) return state.message;
    return '';
}

// ── Rendering ─────────────────────────────────────────────────────────────────

function renderTitle() {
    var title = 'Bouncing Terminal.';
    var width = Math.max(0, state.width);
    var pad = Math.max(0, width - title.length);
    var left = Math.floor(pad / 2);
    var line = ' '.repeat(left) + title + ' '.repeat(pad - left);
    if (line.length > width) line = line.substring(0, width);
    return titleStyle.render(line);
}

function renderChrome() {
    var prefix = router.inChordMode() ? 'C-x-' : '';
    var help = '^P Pause  ^B Big  ^S Small  ^Q Quit  ^X Prefix';
    var runState = state.paused ? 'PAUSED' : 'RUNNING';
    var status = 'Bounces:' + state.bounces + ' ' + runState +
        ' x:' + state.paneX + ' y:' + state.paneY + ' w:' + state.paneW + ' h:' + state.paneH;
    var msg = activeMessage();
    if (msg) {
        var label = msg === 'copyMode' ? 'COPY MODE' : msg;
        status = '[' + label + '] ' + status;
    }
    var line = prefix + help + ' | ' + status;
    var width = Math.max(0, state.width);
    if (line.length > width) line = line.substring(0, width);
    else if (line.length < width) line = line + ' '.repeat(width - line.length);
    return chromeStyle.render(line);
}

function updateCompositor() {
    c.resize(state.width, state.height);

    pane.setBounds(paneOuterBounds());

    var v = pane.view();
    var b = paneOuterBounds();
    c.updatePaneBounds({
        id: 'pty',
        x: b.x,
        y: b.y,
        width: b.width,
        height: b.height,
    });
    c.updatePaneIfNew({
        id: 'pty',
        content: v.content,
        gen: v.gen,
    });

    state.cursor = v.cursor || null;

    c.updateChrome({ id: 'title', content: renderTitle() });
    c.updateChromeBounds({
        id: 'title',
        x: 0,
        y: 0,
        width: state.width,
        height: 1,
    });
    c.updateChrome({ id: 'status', content: renderChrome() });
    c.updateChromeBounds({
        id: 'status',
        x: 0,
        y: Math.max(0, state.height - state.controlsHeight),
        width: state.width,
        height: state.controlsHeight,
    });
}

// ── Controls / Input ───────────────────────────────────────────────────────────

function handleKey(msg) {
    if (state.muxPrefix) {
        state.muxPrefix = false;
        var dispatch = termmux.handlePrefixKey({ manager: mgr, key: msg.key });
        if (dispatch && dispatch.action && dispatch.action !== 'Cancel') {
            setMessage(dispatch.action);
        }
        return true;
    }
    if (msg.key === 'ctrl+a') {
        state.muxPrefix = true;
        setMessage('prefix');
        return true;
    }

    var r = router.handleKey(msg.key);
    if (r.handled) {
        switch (r.action) {
            case 'pause':
                state.paused = !state.paused;
                setMessage(state.paused ? 'paused' : 'resumed');
                return true;
            case 'bigger':
                bigger();
                return true;
            case 'smaller':
                smaller();
                return true;
            case 'copyMode':
                try {
                    if (mgr.isCopyModeActive(sid)) mgr.exitCopyMode(sid);
                    else mgr.enterCopyMode(sid);
                } catch (_) {}
                setMessage('copyMode');
                return true;
            case 'quit':
                state.quit = true;
                return true;
        }
        return true;
    }

    // Forward all other keys to the bouncing terminal pane.
    pane.update(msg);
    return false;
}

function handleMouse(msg) {
    var hit = c.hit(msg.x, msg.y);
    if (hit.hit && hit.id === 'pty') {
        pane.update(msg);
    }
}

function handleResize(width, height) {
    state.width = width;
    state.height = height;
    clampPane();
    resizePane();
    updateCompositor();
}

function cleanup() {
    try { pane.close(); } catch (_) {}
    try { bounded.session.close(); } catch (_) {}
    try { bounded.mgr.close(); } catch (_) {}
}

// ── Bubble Tea callbacks ────────────────────────────────────────────────────────

function initModel() {
    clampPane();
    resizePane();

    c.addPane({
        id: 'pty',
        content: '',
        bounds: paneOuterBounds(),
        z: 0,
    });
    c.addChrome({
        id: 'title',
        content: renderTitle(),
        bounds: { x: 0, y: 0, width: state.width, height: 1 },
        z: 10,
    });
    c.addChrome({
        id: 'status',
        content: renderChrome(),
        bounds: { x: 0, y: Math.max(0, state.height - state.controlsHeight), width: state.width, height: state.controlsHeight },
        z: 10,
    });

    updateCompositor();

    return [state, tea.tick(CONFIG.tickMs, 'tick')];
}

function updateModel(msg, model) {
    if (msg.type === 'Tick') {
        tick();
        updateCompositor();
        return [state, tea.tick(CONFIG.tickMs, 'tick')];
    }
    if (msg.type === 'WindowSize') {
        handleResize(msg.width, msg.height);
        return [state, null];
    }
    if (msg.type === 'Key') {
        handleKey(msg);
        if (state.quit) {
            cleanup();
            return [state, tea.quit()];
        }
        return [state, null];
    }
    if (msg.type === 'KeyRelease') {
        return [state, null];
    }
    if (msg.type === 'MouseClick' || msg.type === 'MouseRelease' || msg.type === 'MouseMotion' || msg.type === 'MouseWheel') {
        handleMouse(msg);
        return [state, null];
    }
    return [state, null];
}

function viewModel(model) {
    return {
        content: c.render(),
        altScreen: true,
        mouseMode: 'allMotion',
        reportFocus: false,
        disableBracketedPasteMode: true,
        cursor: state.cursor,
    };
}

// ── Smoke test ─────────────────────────────────────────────────────────────────

function runSmoke() {
    handleResize(80, 24);
    initModel();
    for (var i = 0; i < 20; i++) {
        tick();
        updateCompositor();
    }

    // Pump one dummy input so the pane has a chance to let the PTY child output.
    pane.update({ type: 'Key', key: 'enter' });
    updateCompositor();

    var view = viewModel({});
    var content = view.content || '';
    output.print('smoke: rendered content=' + JSON.stringify(content.substring(0, 200)));
    output.print('smoke: rendered content length=' + content.length);
    output.print('smoke: bounceCount=' + state.bounces);
    output.print('smoke: title=' + (content.indexOf('Bouncing Terminal') >= 0));
    output.print('smoke: running=' + (content.indexOf('RUNNING') >= 0));
    output.print('smoke: ansi=' + (content.indexOf('\x1b[') >= 0));
    output.print('smoke: controls=' + (content.indexOf('Pause') >= 0 && content.indexOf('Quit') >= 0));
    if (state.bounces <= 0) throw new Error('smoke: expected bounces after ticks');
    cleanup();
}

// ── Entry point ─────────────────────────────────────────────────────────────

sessionReady.then(function() {
    if (SMOKE) {
        runSmoke();
    } else {
        var program = tea.newModel({
            init: initModel,
            update: updateModel,
            view: viewModel,
        });
        tea.run(program);
    }
});
