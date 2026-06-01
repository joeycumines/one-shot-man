#!/usr/bin/env osm script

// example-split-pane.js — Terminal multiplexer compositing demo
//
// Demonstrates the osm:termui/compositor module rendering two live panes
// in a split layout. Uses bubbletea's event loop to animate pane content
// and cycle focus between panes with Tab.
//
// This demo uses the compositor directly (no termmux SessionManager).
// For live PTY sessions, see example-live-panes.js.
//
// Run: osm script scripts/example-split-pane.js

const tea = require('osm:bubbletea');
const comp = require('osm:termui/compositor');
const lipgloss = require('osm:lipgloss');

const WIDTH = 80;
const HEIGHT = 24;
const HALF_W = Math.floor(WIDTH / 2);
const TICK_MS = 100;

const borderStyle = lipgloss.newStyle().foreground('#7D56F4');
const focusedLabel = lipgloss.newStyle().foreground('#EE6FF8').bold(true);
const unfocusedLabel = lipgloss.newStyle().foreground('#666');
const contentStyle = lipgloss.newStyle().foreground('#AAA');
const helpStyle = lipgloss.newStyle().foreground('#555');

let focusIdx = 0;
let tick = 0;
let c = null;

function buildPaneContent(paneIdx, focused, t) {
    const label = focused
        ? focusedLabel.render(` Pane ${paneIdx + 1} [FOCUSED] `)
        : unfocusedLabel.render(` Pane ${paneIdx + 1} `);

    const spinner = ['⠋','⠙','⠹','⠸','⠼','⠴','⠦','⠧','⠇','⠏'];
    const s = spinner[t % spinner.length];

    const lines = [
        label,
        '',
        contentStyle.render(`  ${s} Tick: ${t}`),
        contentStyle.render(`  Size: ${HALF_W - 2}x${HEIGHT - 6}`),
        '',
        contentStyle.render('  This pane will receive input'),
        contentStyle.render('  when focused (Tab to switch)'),
    ];

    return lines.join('\n');
}

function buildBorderBar(width, focused) {
    const ch = focused ? '█' : '░';
    return ch.repeat(width);
}

const program = tea.newModel({
    init: function() {
        c = comp.compositor({ width: WIDTH, height: HEIGHT });

        c.addPane({
            id: 'left',
            content: buildPaneContent(0, true, 0),
            bounds: { x: 0, y: 1, width: HALF_W - 1, height: HEIGHT - 3 },
            z: 0
        });
        c.addPane({
            id: 'right',
            content: buildPaneContent(1, false, 0),
            bounds: { x: HALF_W + 1, y: 1, width: HALF_W - 1, height: HEIGHT - 3 },
            z: 0
        });

        c.addChrome({
            id: 'top-left',
            content: borderStyle.render(buildBorderBar(HALF_W - 1, true)),
            bounds: { x: 0, y: 0, width: HALF_W - 1, height: 1 },
            z: 15
        });
        c.addChrome({
            id: 'top-right',
            content: borderStyle.render(buildBorderBar(HALF_W - 1, false)),
            bounds: { x: HALF_W + 1, y: 0, width: HALF_W - 1, height: 1 },
            z: 15
        });

        const divLine = '│'.repeat(HEIGHT - 3);
        c.addChrome({
            id: 'divider',
            content: divLine,
            bounds: { x: HALF_W, y: 1, width: 1, height: HEIGHT - 3 },
            z: 10
        });

        c.addChrome({
            id: 'helpbar',
            content: helpStyle.render(' Tab: switch focus │ q: quit '),
            bounds: { x: 0, y: HEIGHT - 2, width: WIDTH, height: 1 },
            z: 20
        });

        return [{ tick: 0, focusIdx: 0 }, tea.tick(TICK_MS, 'tick')];
    },

    update: function(msg, model) {
        if (msg.type === 'Tick') {
            tick = (model.tick || 0) + 1;
            c.updatePane({ id: 'left', content: buildPaneContent(0, model.focusIdx === 0, tick) });
            c.updatePane({ id: 'right', content: buildPaneContent(1, model.focusIdx === 1, tick) });
            return [{ tick: tick, focusIdx: model.focusIdx }, tea.tick(TICK_MS, 'tick')];
        }

        if (msg.type === 'Key') {
            if (msg.key === 'q' || msg.key === 'ctrl+c') {
                return [model, tea.quit()];
            }
            if (msg.key === 'tab') {
                const newFocus = model.focusIdx === 0 ? 1 : 0;
                c.updateChrome({ id: 'top-left', content: borderStyle.render(buildBorderBar(HALF_W - 1, newFocus === 0)) });
                c.updateChrome({ id: 'top-right', content: borderStyle.render(buildBorderBar(HALF_W - 1, newFocus === 1)) });
                c.updatePane({ id: 'left', content: buildPaneContent(0, newFocus === 0, model.tick || 0) });
                c.updatePane({ id: 'right', content: buildPaneContent(1, newFocus === 1, model.tick || 0) });
                return [{ tick: model.tick || 0, focusIdx: newFocus }, null];
            }
        }

        return [model, null];
    },

    view: function(model) {
        if (!c) return { content: 'Initializing...', altScreen: true };
        return { content: c.render(), altScreen: true };
    }
});

tea.run(program);
