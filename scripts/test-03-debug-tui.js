#!/usr/bin/env osm script --test

// Debug script to test TUI API bindings

ctx.log("Testing TUI API bindings...");

// Test if tui object exists
if (typeof tui !== 'undefined') {
    ctx.log("✓ tui object is available");

    // Test basic functions
    if (typeof tui.listModes === 'function') {
        ctx.log("✓ tui.listModes is available");
        var modes = tui.listModes();
        ctx.log("Available modes: " + modes.join(", "));
    } else {
        ctx.log("✗ tui.listModes is not available");
    }

    if (typeof tui.registerCommand === 'function') {
        ctx.log("✓ tui.registerCommand is available");

        // Test simple command registration
        try {
            tui.registerCommand({
                name: "test",
                description: "A test command",
                handler: function (args) {
                    output.print("Test command executed with args: " + args.join(" "));
                }
            });
            ctx.log("✓ Successfully registered test command");
        } catch (e) {
            ctx.log("✗ Failed to register test command: " + e.message);
        }
    } else {
        ctx.log("✗ tui.registerCommand is not available");
    }

    if (typeof tui.registerMode === 'function') {
        ctx.log("✓ tui.registerMode is available");

        // Test simple mode registration
        try {
            tui.registerMode({
                name: "test-mode",
                tui: {
                    title: "Test Mode",
                    prompt: "[test]> "
                },
                onEnter: function () {
                    output.print("Entered test mode");
                },
                commands: {}
            });
            ctx.log("✓ Successfully registered test mode");
        } catch (e) {
            ctx.log("✗ Failed to register test mode: " + e.message);
        }
    } else {
        ctx.log("✗ tui.registerMode is not available");
    }

} else {
    ctx.log("✗ tui object is not available");
}

ctx.log("TUI API test completed");
