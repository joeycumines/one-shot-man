#!/usr/bin/env osm script

// Multi-Agent Team Demo - demonstrates role-based agent coordination
// using the pool, panel, and provider registry primitives.
//
// Run: osm script scripts/example-11-multi-agent-team.js
//
// This script demonstrates:
//   1. Role definitions (planner, coder, reviewer)
//   2. A shared coordination bus pattern via the provider registry
//   3. A pool of worker agents dispatched by role
//   4. A panel for multi-agent output display

var cm = require('osm:claudemux');

output.print("=== Multi-Agent Team Demo ===\n");

// --- 1. Define roles ---
output.print("1. Role Definitions");

var roles = {
    planner: {
        system: "You are a planning agent. Break down tasks and create execution plans.",
        tools: ["search", "read", "list"],
        maxTurns: 5
    },
    coder: {
        system: "You are a coding agent. Implement features with clean, tested code.",
        tools: ["search", "read", "edit", "write", "test"],
        maxTurns: 20
    },
    reviewer: {
        system: "You are a code review agent. Analyze code for correctness and style.",
        tools: ["search", "read", "diff"],
        maxTurns: 10
    }
};

for (var roleName in roles) {
    var r = roles[roleName];
    output.printf("  [%s] tools=%s, maxTurns=%d",
        roleName, JSON.stringify(r.tools), r.maxTurns);
}

output.print("");

// --- 2. Create provider registry ---
output.print("2. Provider Registry");

var registry = cm.newRegistry();
var cc = cm.claudeCode();
registry.register(cc);

output.printf("  Providers: %s", JSON.stringify(registry.list()));
output.print("");

// --- 3. Create a pool as the agent dispatch system ---
output.print("3. Agent Pool (dispatch system)");

var pool = cm.newPool({ maxSize: 3 });
pool.start();

// Register pool workers (agents) with role labels
var workerPlanner = pool.addWorker("planner-1");
var workerCoder = pool.addWorker("coder-1");
var workerReviewer = pool.addWorker("reviewer-1");

output.printf("  Pool state: %s (%d workers)",
    pool.stats().stateName, pool.stats().workerCount);
output.print("");

// --- 4. Create a panel as the multi-agent display ---
output.print("4. Multi-Agent Panel (display)");

var panel = cm.newPanel({ maxPanes: 3, scrollbackSize: 100 });
panel.start();

panel.addPane("planner-1", "Planner");
panel.addPane("coder-1", "Coder");
panel.addPane("reviewer-1", "Reviewer");

output.printf("  Panes: %d", panel.paneCount());
output.printf("  Active: %s (%s)",
    panel.activePane().id, panel.activePane().title);

// Add sample output to each pane
panel.appendOutput("planner-1", "Analyzing requirements...");
panel.appendOutput("planner-1", "Plan: 3 phases detected");
panel.appendOutput("coder-1", "Writing implementation...");
panel.appendOutput("coder-1", "Added 45 lines to core.go");
panel.appendOutput("reviewer-1", "Reviewing changes...");
panel.appendOutput("reviewer-1", "Found 2 minor style issues");

// Update health for each pane
panel.updateHealth("planner-1", { state: "idle", errorCount: 0, taskCount: 3 });
panel.updateHealth("coder-1", { state: "idle", errorCount: 0, taskCount: 2 });
panel.updateHealth("reviewer-1", { state: "idle", errorCount: 0, taskCount: 1 });

output.printf("  Status: %s", panel.statusBar());
output.print("");

// --- 5. Simulate coordination messages ---
output.print("5. Coordination Messages (via panel output)");

// Simulate a planning -> coding -> review pipeline
var pipeline = [
    { from: "planner-1", line: "[COORD] Phase 1 complete: architecture defined" },
    { from: "coder-1", line: "[COORD] Phase 2 complete: implementation ready" },
    { from: "reviewer-1", line: "[COORD] Phase 3 complete: review passed, 2 fixes applied" },
    { from: "planner-1", line: "[COORD] All phases complete. Total: 3 agents, 0 errors" }
];

for (var i = 0; i < pipeline.length; i++) {
    var msg = pipeline[i];
    panel.appendOutput(msg.from, msg.line);
    output.printf("  [%s] %s", msg.from, msg.line);
}

output.print("");

// --- 6. Show panel snapshot (dashboard) ---
output.print("6. Panel Snapshot (dashboard view)");

var snap = panel.snapshot();
output.printf("  State: %s", snap.stateName);
output.printf("  Active pane index: %d", snap.activeIdx);

for (var i = 0; i < snap.panes.length; i++) {
    var p = snap.panes[i];
    var indicator = p.isActive ? "[*]" : " [ ]";
    output.printf("    %s %s: %d lines, health=%s, errors=%d",
        indicator, p.id, p.lines, p.health.state, p.health.errorCount);
}

output.print("");

// --- 7. Pool dispatch simulation ---
output.print("7. Pool Dispatch (round-robin across agents)");

// Simulate distributing tasks across pool workers
var tasks = ["refactor auth", "add tests", "fix bug #42", "update docs"];
var dispatched = 0;

for (var i = 0; i < tasks.length; i++) {
    var worker = pool.tryAcquire();
    if (worker) {
        output.printf("  Task %d: '%s' -> worker %s (%s)",
            i, tasks[i], worker.id, worker.stateName());
        pool.release(worker);
        dispatched++;
    } else {
        output.printf("  Task %d: '%s' -> no worker available", i, tasks[i]);
    }
}

var finalStats = pool.stats();
output.printf("\n  Dispatched: %d/%d tasks", dispatched, tasks.length);
output.printf("  Pool: %d inflight, %d workers",
    finalStats.inflight, finalStats.workerCount);

output.print("\n\n=== Multi-Agent Team Demo Complete ===");
