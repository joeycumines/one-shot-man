#!/usr/bin/env osm script

// example-13-split-pane.js — Terminal multiplexer compositing demo
//
// Demonstrates the osm:termui/compositor module rendering two panes
// in a split layout with Tab focus cycling. Uses the compositor
// directly (no termmux SessionManager needed).
//
// Run: osm script scripts/example-13-split-pane.js

const tea = require('osm:bubbletea');
const comp = require('osm:termui/compositor');
const lipgloss = require('osm:lipgloss');

const WIDTH = 80;
const HEIGHT = 24;
const HALF_W = WIDTH / 2;
const LEFT_W = Math.floor(HALF_W);
const RIGHT_W = WIDTH - LEFT_W;
const PANE_H = HEIGHT - 2;
const TICK_MS = 100;

const borderFg = lipgloss.newStyle().foreground('#7D56F4');
const borderBg = lipgloss.newStyle().foreground('#555');
const focusedLabel = lipgloss.newStyle().foreground('#EE6FF8').bold(true);
const unfocusedLabel = lipgloss.newStyle().foreground('#666');
const contentStyle = lipgloss.newStyle().foreground('#AAA');
const helpStyle = lipgloss.newStyle().foreground('#555');

let focusIdx = 0;
let tick = 0;
let c = null;

function buildPaneContent(paneIdx, focused, t, w, h) {
    const label = focused
        ? focusedLabel.render(` Pane ${paneIdx + 1} [FOCUSED] `)
        : unfocusedLabel.render(` Pane ${paneIdx + 1} `);

    const spinner = ['⠋','⠙','⠹','⠸','⠼','⠴','⠦','⠧','⠇','⠏'];
    const s = spinner[t % spinner.length];

    const lines = [
        label,
        '',
        contentStyle.render(`  ${s} Tick: ${t}`),
        contentStyle.render(`  Size: ${w}x${h}`),
        '',
        contentStyle.render('  This pane will receive input'),
        contentStyle.render('  when focused (Tab to switch)'),
    ];

    return lines.join('\n');
}

function buildTopBar(w, focused) {
    const ch = focused ? '▄' : '▀';
    return ch.repeat(w);
}

const program = tea.newModel({
    init: function() {
        c = comp.compositor({ width: WIDTH, height: HEIGHT });

        c.addPane({
            id: 'left',
            content: buildPaneContent(0, true, 0, LEFT_W, PANE_H),
            bounds: { x: 0, y: 1, width: LEFT_W, height: PANE_H },
            z: 0
        });
        c.addPane({
            id: 'right',
            content: buildPaneContent(1, false, 0, RIGHT_W, PANE_H),
            bounds: { x: LEFT_W, y: 1, width: RIGHT_W, height: PANE_H },
            z: 0
        });

        c.addChrome({
            id: 'top-left',
            content: borderFg.render(buildTopBar(LEFT_W, true)),
            bounds: { x: 0, y: 0, width: LEFT_W, height: 1 },
            z: 15
        });
        c.addChrome({
            id: 'top-right',
            content: borderBg.render(buildTopBar(RIGHT_W, false)),
            bounds: { x: LEFT_W, y: 0, width: RIGHT_W, height: 1 },
            z: 15
        });

        const divLine = borderBg.render('│'.repeat(PANE_H));
        c.addChrome({
            id: 'divider',
            content: divLine,
            bounds: { x: LEFT_W, y: 1, width: 1, height: PANE_H },
            z: 10
        });

        c.addChrome({
            id: 'helpbar',
            content: helpStyle.render(' Tab: switch focus │ q: quit '),
            bounds: { x: 0, y: HEIGHT - 1, width: WIDTH, height: 1 },
            z: 20
        });

        return [{ tick: 0, focusIdx: 0 }, tea.tick(TICK_MS, 'tick')];
    },

    update: function(msg, model) {
        if (msg.type === 'Tick') {
            tick = (model.tick || 0) + 1;
            c.updatePane({ id: 'left', content: buildPaneContent(0, model.focusIdx === 0, tick, LEFT_W, PANE_H) });
            c.updatePane({ id: 'right', content: buildPaneContent(1, model.focusIdx === 1, tick, RIGHT_W, PANE_H) });
            return [{ tick: tick, focusIdx: model.focusIdx }, tea.tick(TICK_MS, 'tick')];
        }

        if (msg.type === 'Key') {
            if (msg.key === 'q' || msg.key === 'ctrl+c') {
                return [model, tea.quit()];
            }
            if (msg.key === 'tab') {
                const newFocus = model.focusIdx === 0 ? 1 : 0;
                c.updateChrome({ id: 'top-left', content: (newFocus === 0 ? borderFg : borderBg).render(buildTopBar(LEFT_W, newFocus === 0)) });
                c.updateChrome({ id: 'top-right', content: (newFocus === 1 ? borderFg : borderBg).render(buildTopBar(RIGHT_W, newFocus === 1)) });
                c.updatePane({ id: 'left', content: buildPaneContent(0, newFocus === 0, model.tick || 0, LEFT_W, PANE_H) });
                c.updatePane({ id: 'right', content: buildPaneContent(1, newFocus === 1, model.tick || 0, RIGHT_W, PANE_H) });
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
