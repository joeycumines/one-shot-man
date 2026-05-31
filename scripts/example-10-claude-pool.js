#!/usr/bin/env osm script

// Claude Pool Demo - demonstrates multi-instance management with registry,
// handles, and reliable prompter.
//
// Run: osm script scripts/example-10-claude-pool.js
//
// This script demonstrates:
//   1. Creating a registry with a mock provider
//   2. Spawning multiple handles from the same provider
//   3. Using ReliablePrompter for each handle
//   4. Sequential execution with timing
//   5. Pool-style worker management

var cm = require('osm:claudemux');

output.print("=== 1. Provider & Registry ===");

var provider = cm.mockClaude({ processingMs: 30 });
output.printf("Provider: %s", provider.name());
output.printf("  MCP: %s, streaming: %s, multiTurn: %s",
    provider.capabilities().mcp,
    provider.capabilities().streaming,
    provider.capabilities().multiTurn);

var reg = cm.newRegistry();
reg.register(provider);
output.printf("Registry has %d provider(s): %s",
    reg.list().length, reg.list().join(", "));

output.print("");

output.print("=== 2. Spawn Multiple Handles ===");

var handleCount = 3;
var handles = [];

for (var i = 0; i < handleCount; i++) {
    var name = "mock-pool-" + i;
    var handle = reg.spawn(name, { mode: cm.MODE_PROTOCOL });
    handles.push({
        id: name,
        handle: handle
    });
    output.printf("  [%s] spawned, isAlive: %s", name, handle.isAlive());
}

output.print("");

output.print("=== 3. Pool State ===");

var pool = cm.newPool({ maxSize: 4 });
pool.start();

var poolWorkers = [];
for (var i = 0; i < handleCount; i++) {
    var worker = pool.addWorker("worker-" + i);
    poolWorkers.push(worker);
    output.printf("  Added worker[%d]: id=%s, state=%d (%s)",
        i, worker.id, worker.state(), worker.stateName());
}

var poolStats = pool.stats();
output.printf("\nPool stats: workers=%d, maxSize=%d, inflight=%d",
    poolStats.workerCount, poolStats.maxSize, poolStats.inflight);
output.printf("Pool state: %d (%s)", poolStats.state, poolStats.stateName);
output.printf("Workers:");
for (var i = 0; i < poolStats.workers.length; i++) {
    var w = poolStats.workers[i];
    output.printf("    [%s] state=%d (%s) tasks=%d errors=%d",
        w.id, w.state, w.stateName, w.taskCount, w.errorCount);
}

output.print("");

output.print("=== 4. Reliable Prompt Execution ===");

var poolConfig = pool.config();
output.printf("Pool config: maxSize=%d", poolConfig.maxSize);
output.print("");

var prompts = [
    "hello from instance 0",
    "hello from instance 1",
    "hello from instance 2"
];

var results = [];

for (var i = 0; i < handles.length; i++) {
    var h = handles[i];
    var promptText = prompts[i];

    output.printf("--- Handle: %s ---", h.id);
    output.printf("  Prompt: \"%s\"", promptText);

    try {
        // Create a reliable prompter for this handle
        var rp = cm.newReliablePrompter(h.handle, provider, {
            readyTimeout: 10000,
            acceptTimeout: 5000,
            responseTimeout: 15000,
            maxRetries: 1
        });

        // Send the prompt and wait for a complete response
        // Returns: { responseText, duration, stateTransitions[], events[] }
        var result = rp.sendPrompt(promptText);

        results.push({
            id: h.id,
            prompt: promptText,
            responseText: result.responseText,
            duration: result.duration,
            transitions: result.stateTransitions,
            events: result.events
        });

        output.printf("  Response (%dms): %s",
            result.duration,
            result.responseText.substring(0, 60).replace(/\n/g, " "));
        output.printf("  State transitions: %d", result.stateTransitions.length);
        for (var j = 0; j < result.stateTransitions.length; j++) {
            var st = result.stateTransitions[j];
            output.printf("    [%d] %d(%s) -> %d(%s)",
                j, st.from, cm.tuiStateName(st.from),
                st.to, cm.tuiStateName(st.to));
        }
        output.printf("  Events: %d", result.events.length);
        for (var j = 0; j < Math.min(3, result.events.length); j++) {
            var ev = result.events[j];
            output.printf("    [%d] type=%d (%s) pattern=%s",
                j, ev.type, cm.eventTypeName(ev.type), ev.pattern || "(none)");
        }
        if (result.events.length > 3) {
            output.printf("    ... and %d more", result.events.length - 3);
        }

        rp.close();

    } catch (e) {
        results.push({
            id: h.id,
            prompt: promptText,
            error: (e.message || String(e)).substring(0, 80)
        });
        output.printf("  Exception: %s", e.message || e);
    }

    output.print("");
}

output.print("=== 5. Results Summary ===");

for (var i = 0; i < results.length; i++) {
    var r = results[i];
    if (r.error) {
        output.printf("  [%s] ERROR: %s", r.id, r.error);
    } else {
        output.printf("  [%s] %dms - prompt: \"%s\" response: \"%s...\"",
            r.id, r.duration, r.prompt, (r.responseText || "").substring(0, 50));
    }
}

output.print("\n=== 6. Cleanup ===");

for (var i = 0; i < handles.length; i++) {
    handles[i].handle.close();
    output.printf("  Closed handle: %s", handles[i].id);
}

// Release pool workers
for (var i = 0; i < poolWorkers.length; i++) {
    pool.release(poolWorkers[i]);
    output.printf("  Released worker: %s", poolWorkers[i].id);
}

pool.drain();
output.printf("Pool drained. Final stats:");
var finalStats = pool.stats();
output.printf("  workers=%d, inflight=%d",
    finalStats.workerCount, finalStats.inflight);
pool.close();
output.print("  Pool closed.");

output.print("\nDone. Pool demo complete.");
