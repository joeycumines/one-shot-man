#!/usr/bin/env osm script
// example-15-bouncing-logo.js — Bouncing Terminal: a nested PTY session bouncing around the screen
//
// Demonstrates: osm:termmux (newBouncePresenter — full lifecycle in one call),
// osm:bubbletea (TUI engine), osm:bubblezone (mouse zones),
// osm:flag (argument parsing).
//
// A fully interactive nested terminal session that physically bounces around the screen.
// Supports: keyboard input forwarding, mouse forwarding with coordinate translation,
// resize, pause/resume, pane size adjustment, chord mode, and clean exit.
// Cross-platform: uses /bin/sh on Unix, cmd.exe on Windows.
//
// Usage:
//   osm script scripts/example-15-bouncing-logo.js
//   osm script scripts/example-15-bouncing-logo.js --cmd /bin/bash
//   osm script scripts/example-15-bouncing-logo.js -- bash -l

var termmux = require('osm:termmux');
var tea = require('osm:bubbletea');
var flag = require('osm:flag');
var compMod = require('osm:termui/compositor');
var zone = require('osm:bubblezone');

var isWindows = false;
try { if (require('osm:os').getenv('OS') === 'Windows_NT') isWindows = true; } catch (_) {}
var DEFAULT_CMD = isWindows ? 'cmd.exe' : '/bin/sh';

var fs = flag.newFlagSet('bouncing-logo');
fs.string('cmd', DEFAULT_CMD, 'command to run in the bouncing PTY');
var parseResult = fs.parse(args);
if (parseResult.error !== null) throw new Error('flag parse failed: ' + parseResult.error);
var CMD = fs.get('cmd');
var targetArgs = fs.args().length === 0 ? [] : fs.args();

var p = termmux.newBouncePresenter({
    cmd: CMD, args: targetArgs, rows: 10, cols: 30, tickMs: 120,
    speed: { x: 1, y: 1 },
    paneSize: { w: 32, h: 12, minW: 12, maxW: 62, minH: 7, maxH: 32, step: 2 },
    controlsHeight: 2, borderWidth: 1,
    keys: { 'ctrl+p': 'pause', 'ctrl+b': 'bigger', 'ctrl+s': 'smaller', 'ctrl+q': 'quit', 'ctrl+c': 'quit' },
    chordMode: { prefix: 'ctrl+x', actions: { s: 'smaller', b: 'bigger', p: 'pause', q: 'quit', 'ctrl+c': 'quit' } },
    colors: {
        bg: '#1A1B26', surface: '#24283B', chrome: '#292E42', border: '#3B4261',
        muted: '#565F89', dim: '#787C99', text: '#C0CAF5', bright: '#D5D6DB',
        cyan: '#7DCFFF', blue: '#7AA2F7', purple: '#BB9AF7', green: '#9ECE6A',
        brightGreen: '#73DACA', yellow: '#E0AF68', orange: '#FF9E64', red: '#F7768E',
    },
    tea: tea,
    compositor: compMod,
});

function update(msg, m) {
    var result = p.handleMsg(msg);
    if (result === null || result === undefined) return [m, null];
    return [m, result[1]];
}

function view(m) {
    var v = p.render();
    var content = v.content || '';
    if (v._controlsLine) {
        var pad = v._pad || '';
        zone.scan(pad + v._controlsLine);
    }
    return {
        content: content, altScreen: true, mouseMode: 'allMotion', reportFocus: true,
        windowTitle: v.windowTitle || 'Bouncing Terminal',
        foregroundColor: v.foregroundColor, backgroundColor: v.backgroundColor,
    };
}

tea.run(tea.newModel({
    init: function() {
        var tickResult = tea.tick(120, 'tick');
        return [p, tickResult];
    },
    update: update,
    view: view,
}));
