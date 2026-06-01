#!/usr/bin/env osm script

// example-split-pane.js — Terminal multiplexer compositing demo
//
// Demonstrates the osm:termui/compositor module rendering two live panes
// in a split layout. Uses bubbletea's event loop to animate pane content
// and cycle focus between panes with Tab.
//
// Run: osm script scripts/example-split-pane.js

const tea = require('osm:bubbletea');
const comp = require('osm:termui/compositor');
const lipgloss = require('osm:lipgloss');

// --- Configuration ---
const WIDTH = 80;
const HEIGHT = 24;
const HALF_W = Math.floor(WIDTH / 2);
const TICK_MS = 100;

// --- Styles ---
const borderStyle = lipgloss.newStyle().foreground('#7D56F4');
const focusedLabel = lipgloss.newStyle().foreground('#EE6FF8').bold(true);
const unfocusedLabel = lipgloss.newStyle().foreground('#666');
const contentStyle = lipgloss.newStyle().foreground('#AAA');
const helpStyle = lipgloss.newStyle().foreground('#555');

// --- State ---
let focusIdx = 0; // which pane is focused (0 or 1)
let tick = 0;
let c = null; // compositor instance

function buildBorderChrome(width, height, focused) {
    const ch = focused ? '█' : '░';
    const top = ch.repeat(width);
    const bottom = ch.repeat(width);
    const midLine = ch + ' '.repeat(width - 2) + ch;
    const lines = [top];
    for (let i = 0; i < height - 2; i++) {
        lines.push(midLine);
    }
    lines.push(bottom);
    return lines.join('\n');
}

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

// --- Bubbletea Model ---
const program = tea.newModel({
    init: function() {
        // Create compositor with canvas size
        c = comp.compositor({ width: WIDTH, height: HEIGHT });

        // Add two panes side by side
        c.addPane({
            id: 'left',
            content: buildPaneContent(0, true, 0),
            bounds: { x: 0, y: 0, width: HALF_W - 1, height: HEIGHT - 2 },
            z: 0
        });
        c.addPane({
            id: 'right',
            content: buildPaneContent(1, false, 0),
            bounds: { x: HALF_W + 1, y: 0, width: HALF_W - 1, height: HEIGHT - 2 },
            z: 0
        });

        // Add vertical divider chrome
        const divLine = '│'.repeat(HEIGHT - 2);
        c.addChrome({
            id: 'divider',
            content: divLine,
            bounds: { x: HALF_W, y: 0, width: 1, height: HEIGHT - 2 },
            z: 10
        });

        // Add help bar at bottom
        const helpText = helpStyle.render(' Tab: switch focus │ q: quit ');
        c.addChrome({
            id: 'helpbar',
            content: helpText,
            bounds: { x: 0, y: HEIGHT - 2, width: WIDTH, height: 1 },
            z: 20
        });

        // Add top border chrome for focused pane
        const borderL = buildBorderChrome(HALF_W - 1, 1, true);
        c.addChrome({
            id: 'border-left',
            content: borderStyle.render(borderL),
            bounds: { x: 0, y: 0, width: HALF_W - 1, height: 1 },
            z: 15
        });
        const borderR = buildBorderChrome(HALF_W - 1, 1, false);
        c.addChrome({
            id: 'border-right',
            content: borderStyle.render(borderR),
            bounds: { x: HALF_W + 1, y: 0, width: HALF_W - 1, height: 1 },
            z: 15
        });

        return [{ tick: 0, focusIdx: 0 }, tea.tick(TICK_MS, 'tick')];
    },

    update: function(msg, model) {
        if (msg.type === 'Tick') {
            tick = (model.tick || 0) + 1;

            // Update pane contents with new tick
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
                const borderL = buildBorderChrome(HALF_W - 1, 1, newFocus === 0);
                const borderR = buildBorderChrome(HALF_W - 1, 1, newFocus === 1);
                c.updateChrome({ id: 'border-left', content: borderStyle.render(borderL) });
                c.updateChrome({ id: 'border-right', content: borderStyle.render(borderR) });
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
