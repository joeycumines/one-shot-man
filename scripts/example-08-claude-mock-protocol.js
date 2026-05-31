#!/usr/bin/env osm script

// Claude Mock Protocol Demo - demonstrates protocol mode with mockclaude provider.
//
// Run: osm script scripts/example-08-claude-mock-protocol.js
//
// This script demonstrates:
//   1. Creating a mock provider, registering it, and spawning in MODE_PROTOCOL
//   2. waitReady, send a user message, receive response
//   3. State transitions and event classification
//   4. Rate limit scenario
//   5. Clean shutdown with close()

var cm = require('osm:claudemux');

output.print("=== 1. Provider Registration ===");

var provider = cm.mockClaude({ processingMs: 50 });
output.printf("Provider name: %s", provider.name());

var caps = provider.capabilities();
output.printf("  MCP: %s, streaming: %s, multiTurn: %s",
    caps.mcp, caps.streaming, caps.multiTurn);

var reg = cm.newRegistry();
reg.register(provider);

output.printf("\nRegistered providers (%d):", reg.list().length);
var names = reg.list();
for (var i = 0; i < names.length; i++) {
    output.printf("  [%d] %s", i, names[i]);
}

output.print("");

output.print("=== 2. Spawn & Wait-Ready ===");

try {
    var handle = reg.spawn("mock-claude", { mode: cm.MODE_PROTOCOL });
    output.printf("Handle spawned. isAlive: %s", handle.isAlive());

    // Wait for the agent to become ready (max 10 seconds)
    handle.waitReady(10000);
    output.printf("Agent is ready. isAlive: %s", handle.isAlive());

        output.print("\n=== 3. Send Prompt & Receive ===");

    handle.send("hello world");
    output.print("  [sent: 'hello world']");

    // Receive lines from the protocol stream. The mock sends 5 lines:
    // system/init, assistant/text (ready), assistant/text (thinking),
    // assistant/text (response), result/success.
    var responseParts = [];
    var linesReceived = 0;
    var maxLines = 20;

    while (linesReceived < maxLines) {
        var line = handle.receive();
        if (line === "" || line === undefined) {
            break;
        }
        linesReceived++;
        output.printf("  [recv %d] %s", linesReceived, line);
        responseParts.push(line);

        // The result/success event signals the turn is complete.
        // In NDJSON format this appears as {"type":"result","subtype":"success",...}
        if (line.indexOf('"result"') !== -1 && line.indexOf('"success"') !== -1) {
            output.print("  --> Completion detected");
            break;
        }
    }

        output.print("\n=== 4. Event Classification ===");

    var parser = cm.newParser();
    output.printf("Built-in patterns: %d", parser.patterns().length);

    for (var i = 0; i < responseParts.length; i++) {
        var ev = parser.parse(responseParts[i]);
        var typeName = cm.eventTypeName(ev.type);
        output.printf("  line[%d] type=%d (%s) pattern=%s",
            i, ev.type, typeName, ev.pattern || "(none)");
        if (ev.fields && Object.keys(ev.fields).length > 0) {
            var fieldKeys = Object.keys(ev.fields);
            for (var j = 0; j < fieldKeys.length; j++) {
                output.printf("    .%s = %s", fieldKeys[j], ev.fields[fieldKeys[j]]);
            }
        }
    }

        output.print("\n=== 5. Rate Limit Classification ===");

    var rateLimitLine = '{"type":"system","subtype":"rate_limit","retryAfterMs":5000}';
    var rateEv = parser.parse(rateLimitLine);
    output.printf("Rate limit event: type=%d (%s), pattern=%s",
        rateEv.type, cm.eventTypeName(rateEv.type), rateEv.pattern);
    if (rateEv.fields && rateEv.fields["retryAfterMs"]) {
        output.printf("  retryAfterMs: %s", rateEv.fields["retryAfterMs"]);
    }

    // Classify normal text to show default classification
    var textEv = parser.parse("just some normal output");
    output.printf("Normal text: type=%d (%s), line=%s",
        textEv.type, cm.eventTypeName(textEv.type), textEv.line);

    // Classify a thinking event
    var thinkLine = '{"type":"assistant","subtype":"text","content":"thinking...","thinking":true}';
    var thinkEv = parser.parse(thinkLine);
    output.printf("Thinking: type=%d (%s), pattern=%s, content=%s",
        thinkEv.type, cm.eventTypeName(thinkEv.type), thinkEv.pattern,
        thinkEv.fields && thinkEv.fields["content"] ? thinkEv.fields["content"] : "(none)");

        output.print("\n=== 6. Handle State ===");
    output.printf("isAlive: %s", handle.isAlive());

    // Close the handle first so the mock process exits cleanly.
    // Without close(), wait() would block forever because the
    // mockclaude process stays alive waiting for more stdin input.
    handle.close();
    output.print("  Handle closed.");

    var waitResult = handle.wait();
    output.printf("wait result: code=%s, error=%s",
        waitResult.code, waitResult.error || "(none)");

} catch (e) {
    output.printf("Error during protocol demo: %s", e.message || e);
}

output.print("\n=== 7. Parser Patterns ===");
var parser2 = cm.newParser();
var patterns = parser2.patterns();
output.printf("Total patterns: %d", patterns.length);
for (var i = 0; i < Math.min(5, patterns.length); i++) {
    var p = patterns[i];
    output.printf("  [%d] %-15s event=%d (%s) pattern=%s",
        i, p.name, p.eventType, cm.eventTypeName(p.eventType), p.pattern);
}

output.print("\nDone. Protocol mode demo complete.");
