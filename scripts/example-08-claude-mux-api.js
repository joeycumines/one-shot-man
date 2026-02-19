// example-08-claude-mux-api.js — Demonstrate the osm:claudemux building blocks.
//
// Usage:
//   osm script scripts/example-08-claude-mux-api.js
//
// This non-interactive script exercises all the public building blocks of the
// claude-mux module:  parser, guard, MCP guard, supervisor, pool, and panel.
// It does NOT spawn real Claude Code instances — it exercises the APIs in
// isolation so you can see how they compose.

'use strict';

var cm = require('osm:claudemux');

// ---------------------------------------------------------------------------
//  1. Parser — event classification and pattern listing
// ---------------------------------------------------------------------------
log.info('[1/6] Parser');

var parser = cm.newParser();

// List built-in patterns.
var patterns = parser.patterns();
log.printf('  Built-in patterns: %d', patterns.length);
for (var i = 0; i < Math.min(5, patterns.length); i++) {
    log.printf('    [%d] %s → %s', i, patterns[i].name, patterns[i].eventTypeName);
}
if (patterns.length > 5) {
    log.printf('    ... and %d more', patterns.length - 5);
}

// Add a custom pattern and verify it shows up.
parser.addPattern('custom-deploy', '(?i)deploying to', cm.EVENT_COMPLETION);
var updated = parser.patterns();
log.printf('  After custom add: %d patterns', updated.length);

// Parse a few sample lines.
var samples = [
    'Rate limited. Please try again in 30 seconds.',
    'Do you want to allow write access? [Y/n]',
    'Calling tool: readFile',
    'Deploying to production cluster',
    'The quick brown fox jumps over the lazy dog',
];
for (var s = 0; s < samples.length; s++) {
    var ev = parser.parse(samples[s]);
    log.printf('  parse(%q) → %s (pattern=%s)',
        samples[s].substring(0, 40),
        cm.eventTypeName(ev.type),
        ev.pattern || '(none)');
}

// ---------------------------------------------------------------------------
//  2. Guard — PTY output monitor with rate-limit backoff / crash detection
// ---------------------------------------------------------------------------
log.info('[2/6] Guard');

var guardCfg = cm.defaultGuardConfig();
log.printf('  Default config: rateLimit.enabled=%v, crash.maxRestarts=%d',
    guardCfg.rateLimit.enabled, guardCfg.crash.maxRestarts);

var guard = cm.newGuard({
    rateLimit:     { enabled: true, initialDelayMs: 1000, maxDelayMs: 30000, multiplier: 2, resetAfterMs: 60000 },
    permission:    { enabled: true, policy: cm.PERMISSION_POLICY_DENY },
    crash:         { enabled: true, maxRestarts: 3 },
    outputTimeout: { enabled: true, timeoutMs: 120000 },
});

// Simulate a rate-limit event.
var now = Date.now();
var ge = guard.processEvent({ type: cm.EVENT_RATE_LIMIT, line: 'Rate limited' }, now);
if (ge) {
    log.printf('  Rate-limit event → action=%s reason=%q',
        cm.guardActionName(ge.action), ge.reason);
}

// Check guard state.
var gs = guard.state();
log.printf('  State: rateLimitCount=%d, crashCount=%d', gs.rateLimitCount, gs.crashCount);

// ---------------------------------------------------------------------------
//  3. MCP Guard — tool call frequency / repetition / allowlist monitor
// ---------------------------------------------------------------------------
log.info('[3/6] MCP Guard');

var mcpGuard = cm.newMCPGuard({
    noCallTimeout:   { enabled: true, timeoutMs: 600000 },
    frequencyLimit:  { enabled: true, windowMs: 10000, maxCalls: 50 },
    repeatDetection: { enabled: true, maxRepeats: 5, windowSize: 10, matchTool: true, matchArgHash: true },
    toolAllowlist:   { enabled: true, allowedTools: ['addFile', 'buildPrompt', 'listContext'] },
});

// Simulate allowed tool calls.
var ts = Date.now();
var r1 = mcpGuard.processToolCall({ toolName: 'addFile', arguments: '{}', timestampMs: ts });
log.printf('  processToolCall(addFile) → %s', r1 ? cm.guardActionName(r1.action) : 'ok');

var r2 = mcpGuard.processToolCall({ toolName: 'deleteFile', arguments: '{}', timestampMs: ts + 100 });
log.printf('  processToolCall(deleteFile) → %s', r2 ? cm.guardActionName(r2.action) : 'ok');

var mcpState = mcpGuard.state();
log.printf('  State: totalCalls=%d, recentCount=%d', mcpState.totalCalls, mcpState.recentCount);

// ---------------------------------------------------------------------------
//  4. Supervisor — error recovery state machine
// ---------------------------------------------------------------------------
log.info('[4/6] Supervisor');

var sup = cm.newSupervisor({ maxRetries: 3, maxForceKills: 1, retryDelayMs: 5000 });
sup.start();

var snap = sup.snapshot();
log.printf('  State after start: %s', snap.stateName);

// Simulate a PTY error → retry.
var d1 = sup.handleError('connection reset', cm.ERROR_CLASS_PTY_ERROR);
log.printf('  handleError(PTY_ERROR) → action=%s, newState=%s',
    d1.actionName, d1.newStateName);

// Confirm recovery so we can trigger another error.
sup.confirmRecovery();
snap = sup.snapshot();
log.printf('  After confirmRecovery: state=%s, retries=%d',
    snap.stateName, snap.retryCount);

// Shutdown.
var sd = sup.shutdown();
log.printf('  shutdown() → action=%s', sd.actionName);
sup.confirmStopped();
log.printf('  Final state: %s, cancelled=%v', sup.snapshot().stateName, sup.cancelled());

// ---------------------------------------------------------------------------
//  5. Pool — concurrent worker management
// ---------------------------------------------------------------------------
log.info('[5/6] Pool');

var pool = cm.newPool({ maxSize: 3 });
pool.start();

pool.addWorker('w-1');
pool.addWorker('w-2');
pool.addWorker('w-3');

var stats = pool.stats();
log.printf('  Pool: state=%s, workers=%d/%d, inflight=%d',
    stats.stateName, stats.workerCount, stats.maxSize, stats.inflight);

// Acquire, do work, release.
var w = pool.acquire();
log.printf('  Acquired worker: %s (state=%s)', w.id, w.stateName());

// Release with no error.
pool.release(w);
stats = pool.stats();
log.printf('  After release: inflight=%d', stats.inflight);

// Drain and close.
pool.drain();
pool.waitDrained();
var closed = pool.close();
log.printf('  Closed pool, returned %d workers', closed.length);

// ---------------------------------------------------------------------------
//  6. Panel — multi-instance TUI coordination
// ---------------------------------------------------------------------------
log.info('[6/6] Panel');

var panel = cm.newPanel({ maxPanes: 4, scrollbackSize: 500 });
panel.start();

panel.addPane('inst-1', 'Agent 1');
panel.addPane('inst-2', 'Agent 2');
panel.addPane('inst-3', 'Agent 3');

log.printf('  Panes: %d, active=%d', panel.paneCount(), panel.activeIndex());

// Append some output.
for (var line = 0; line < 10; line++) {
    panel.appendOutput('inst-1', 'Agent 1 output line ' + line);
    panel.appendOutput('inst-2', 'Agent 2 output line ' + line);
}

// Update health.
panel.updateHealth('inst-1', { state: 'running', errorCount: 0, taskCount: 5 });
panel.updateHealth('inst-2', { state: 'error', errorCount: 3, taskCount: 2 });
panel.updateHealth('inst-3', { state: 'idle', errorCount: 0, taskCount: 0 });

log.printf('  StatusBar: %s', panel.statusBar());

// Switch panes via Alt+2.
var ir = panel.routeInput('alt+2');
log.printf('  routeInput(alt+2) → action=%s, active=%d', ir.action, panel.activeIndex());

// Get visible lines for active pane.
var visible = panel.getVisibleLines('inst-2', 5);
log.printf('  Visible lines (inst-2, height=5): %d lines', visible.length);

// Snapshot.
var psnap = panel.snapshot();
log.printf('  Snapshot: state=%s, panes=%d, statusBar=%q',
    psnap.stateName, psnap.panes.length, psnap.statusBar);

panel.close();
log.printf('  Panel closed.');

// ---------------------------------------------------------------------------
//  Summary
// ---------------------------------------------------------------------------
log.info('All claude-mux building blocks exercised successfully.');
