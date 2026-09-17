#!/usr/bin/env osm script

// Terminal Capture -> freeze Demo - demonstrates composing osm:termmux with
// osm:freezeterm.
//
// Run: osm script scripts/example-16-freeze-capture.js
//
// This script demonstrates:
//   1. Capturing terminal content with tuiMux.capture(id)
//   2. Rendering that capture to an SVG artifact with osm:freezeterm
//   3. Handling the case where the external freeze CLI is not installed
//
// Neither module knows about the other: the script is the composition point.

var termmux = require('osm:termmux');
var freezeterm = require('osm:freezeterm');
var osmod = require('osm:os');

// --- Helpers ---

function demoCommand() {
    // The command stays alive briefly so the example can read the screen
    // before the session exits and is cleaned up.
    if (osmod.platform() === 'windows') {
        return { cmd: 'cmd', args: ['/c', 'echo osm capture demo & ping -n 3 127.0.0.1 >nul'] };
    }
    return { cmd: '/bin/sh', args: ['-c', 'printf "osm capture demo\\nsecond line\\n"; sleep 2'] };
}

function delay(ms) {
    return new Promise(function(resolve) { setTimeout(resolve, ms); });
}

// Poll capture() until the marker appears; the session may exit as soon as the
// command finishes, so read the screen while the output is fresh.
async function captureUntil(mgr, sid, marker) {
    for (var attempt = 0; attempt < 100; attempt++) {
        var capture = mgr.capture(sid);
        if (capture !== null && capture.plain.indexOf(marker) >= 0) {
            return capture;
        }
        await delay(20);
    }
    return mgr.capture(sid);
}

(async function main() {
    output.print('=== Terminal Capture -> freeze Demo ===\n');

    var source = demoCommand();
    var bounded = await termmux.newBoundedSession({ cmd: source.cmd, args: source.args, rows: 10, cols: 40 });

    var capture = await captureUntil(bounded.mgr, bounded.sid, 'osm capture demo');
    if (capture === null) {
        output.print('no capture available (session has no published snapshot)');
        await bounded.session.close();
        return;
    }
    output.printf('captured %d plain characters, %d ANSI characters', capture.plain.length, capture.ansi.length);

    // --- Render the capture through the external freeze CLI ---

    var info = await freezeterm.info();
    if (!info.available) {
        output.print('\nfreeze is not installed; skipping render.');
        output.print('Install it with: go install github.com/charmbracelet/freeze@latest');
        output.printf('info error: %s', info.error);
        await bounded.session.close();
        return;
    }
    output.printf('\nfreeze %s (%s)', info.version, info.commit || 'unknown commit');

    // renderText produces SVG text from a temporary artifact; render() with an
    // `output` path writes a caller-owned file, and PNG results only expose a
    // path because binary data never crosses the JS boundary.
    try {
        var svg = await freezeterm.renderText(capture.ansi, {
            theme: 'charm',
            window: true,
            border: { radius: 6, width: 1 },
        });
        output.printf('rendered SVG: %d characters', svg.length);
        output.print(svg.split('\n')[0]);
    } catch (e) {
        output.printf('render failed: %s', e.message || e);
    }

    // A ranged capture works the same way: capture only the first two rows.
    var head = bounded.mgr.capture(bounded.sid, { start: 0, end: 2, joinWrapped: true });
    if (head !== null) {
        try {
            var headSVG = await freezeterm.renderText(head.ansi, { padding: [8] });
            output.printf('ranged SVG: %d characters', headSVG.length);
        } catch (e) {
            output.printf('ranged render failed: %s', e.message || e);
        }
    }

    await bounded.session.close();
    output.print('\nDone.');
})();
