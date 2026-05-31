#!/usr/bin/env osm script

// Agent Panel Demo - demonstrates the multi-agent panel with keyboard
// routing, health tracking, and snapshot/restore patterns.
//
// Run: osm script scripts/example-12-agent-panel.js
//
// This script demonstrates:
//   1. Panel lifecycle (start, add/remove panes, close)
//   2. Keyboard input routing (pane switching, scroll)
//   3. Health tracking per agent pane
//   4. Snapshot and dashboard rendering

var cm = require('osm:claudemux');

output.print("=== Agent Panel Demo ===\n");

// --- 1. Create panel ---
output.print("1. Panel Setup");

var panel = cm.newPanel({ maxPanes: 4, scrollbackSize: 500 });
panel.start();

// Add panes for each agent role
var panes = [
    { id: "agent-planner", title: "Planner" },
    { id: "agent-coder", title: "Coder" },
    { id: "agent-reviewer", title: "Reviewer" }
];

for (var i = 0; i < panes.length; i++) {
    var idx = panel.addPane(panes[i].id, panes[i].title);
    output.printf("  Added pane[%d]: %s (%s)", idx, panes[i].id, panes[i].title);
}

output.printf("  Total panes: %d", panel.paneCount());
output.printf("  Config: maxPanes=%d, scrollback=%d",
    panel.config().maxPanes, panel.config().scrollbackSize);

output.print("");

// --- 2. Simulate agent activity ---
output.print("2. Agent Activity (output simulation)");

var activity = [
    { pane: "agent-planner", line: "[INFO] Starting analysis of 12 files..." },
    { pane: "agent-planner", line: "[INFO] Found 3 independent sub-projects" },
    { pane: "agent-planner", line: "[PLAN] Phase 1: refactor core package" },
    { pane: "agent-coder", line: "[EDIT] core/api.go: +34 -12 lines" },
    { pane: "agent-coder", line: "[EDIT] core/handler.go: +28 -5 lines" },
    { pane: "agent-reviewer", line: "[REVIEW] core/api.go: 0 critical, 2 warnings" },
    { pane: "agent-reviewer", line: "[REVIEW] Suggested: add error wrapper for context propagation" },
];

for (var i = 0; i < activity.length; i++) {
    var a = activity[i];
    panel.appendOutput(a.pane, a.line);
    output.printf("  %s: %s", a.pane, a.line);
}

output.print("");

// --- 3. Simulate keyboard input routing ---
output.print("3. Keyboard Input Routing");

var keys = ["alt+1", "alt+2", "pgup", "pgdown", "x", "alt+3"];
for (var i = 0; i < keys.length; i++) {
    var result = panel.routeInput(keys[i]);
    output.printf("  Key '%s' -> action=%s, consumed=%s, target=%s",
        keys[i],
        result.action,
        result.consumed,
        result.targetPaneID || "(panel)"
    );
}

output.printf("  Currently active pane: %s", panel.activePane().id);

output.print("");

// --- 4. Switch to specific pane ---
output.print("4. Pane Navigation");

panel.setActive(1);
output.printf("  After setActive(1): active=%s (%s)",
    panel.activePane().id, panel.activePane().title);

panel.setActive(2);
output.printf("  After setActive(2): active=%s (%s)",
    panel.activePane().id, panel.activePane().title);

// Get visible lines from active pane
var lines = panel.getVisibleLines(panel.activePane().id, 5);
output.printf("  Visible lines in '%s': %d",
    panel.activePane().id, lines.length);
for (var i = 0; i < lines.length; i++) {
    output.printf("    %s", lines[i]);
}

output.print("");

// --- 5. Health tracking ---
output.print("5. Agent Health Tracking");

panel.updateHealth("agent-planner", { state: "idle", errorCount: 0, taskCount: 3 });
panel.updateHealth("agent-coder", { state: "running", errorCount: 0, taskCount: 2 });
panel.updateHealth("agent-reviewer", { state: "running", errorCount: 1, taskCount: 1 });

for (var i = 0; i < panes.length; i++) {
    var pane = panel.activePane();
    // Re-read each pane's health from snapshot
}

var snap = panel.snapshot();
for (var i = 0; i < snap.panes.length; i++) {
    var p = snap.panes[i];
    var icon = "";
    if (p.health.state === "running") icon = "●";
    else if (p.health.state === "error") icon = "✖";
    else icon = "○";
    output.printf("  %s [%s] state=%s errors=%d tasks=%d %s",
        icon, p.id, p.health.state, p.health.errorCount,
        p.health.taskCount, p.isActive ? "(active)" : "");
}

output.print("");

// --- 6. Status bar ---
output.print("6. Status Bar");
output.printf("  %s", panel.statusBar());

output.print("");

// --- 7. Dashboard view (snapshot) ---
output.print("7. Full Dashboard (Snapshot)");

output.printf("  Panel state: %s (%d)", snap.stateName, snap.state);
output.printf("  Active index: %d", snap.activeIdx);

// Summary of all panes
var totalLines = 0;
for (var i = 0; i < snap.panes.length; i++) {
    totalLines += snap.panes[i].lines;
}
output.printf("  Total lines across all panes: %d", totalLines);

output.print("");

// --- 8. Add a new temporary agent ---
output.print("8. Dynamic Agent Addition");

var idx = panel.addPane("agent-debugger", "Debugger");
output.printf("  Added: agent-debugger at index %d", idx);
panel.appendOutput("agent-debugger", "[DEBUG] Attaching to crash dump...");
panel.appendOutput("agent-debugger", "[DEBUG] Root cause: nil pointer in handler.go:42");
output.printf("  Pane count after add: %d", panel.paneCount());

output.print("");

// --- 9. Remove an agent ---
output.print("9. Dynamic Agent Removal");

panel.removePane("agent-reviewer");
output.printf("  After removing reviewer: %d panes remain", panel.paneCount());

var finalActive = panel.activePane();
output.printf("  Active after removal: %s", finalActive ? finalActive.id : "none");

output.print("");

// --- 10. Final snapshot ---
output.print("10. Final State");

var finalSnap = panel.snapshot();
output.printf("  State: %s", finalSnap.stateName);
output.printf("  Panes: %d", finalSnap.panes.length);
for (var i = 0; i < finalSnap.panes.length; i++) {
    var p = finalSnap.panes[i];
    output.printf("    [%s] id=%s lines=%d active=%s",
        i, p.id, p.lines, p.isActive);
}

output.print("\n=== Agent Panel Demo Complete ===");
