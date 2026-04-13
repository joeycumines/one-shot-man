#!/usr/bin/env osm script

console.log('[TEST] Starting minimal bubbletea test...');

const tea = require('osm:bubbletea');
console.log('[TEST] tea module loaded');

const program = tea.newModel({
    init: function() {
        console.log('[TEST] init() called');
        return [{ count: 0 }, tea.tick(16, 'tick')];
    },
    update: function(msg, model) {
        if (msg.type === 'Tick') {
            return [{ count: model.count + 1 }, tea.tick(16, 'tick')];
        }
        if (msg.type === 'Key') {
            if (msg.key === 'q') {
                return [model, tea.quit()];
            }
        }
        return [model, null];
    },
    view: function(model) {
        return { content: 'Count: ' + model.count + '\nPress q to quit', altScreen: true };
    }
});

console.log('[TEST] Calling tea.run()...');
console.log('[TEST] program:', typeof program);
console.log('[TEST] program.init:', typeof program.init);
console.log('[TEST] program.update:', typeof program.update);
console.log('[TEST] program.view:', typeof program.view);

try {
    const result = tea.run(program);
    console.log('[TEST] tea.run() returned:', JSON.stringify(result));
} catch (e) {
    console.log('[TEST] tea.run() threw:', e.message);
    throw e;
}

console.log('[TEST] Script exiting...');
