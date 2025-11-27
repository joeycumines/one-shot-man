// Test script for context manager re-hydration after session restore
// This verifies that the ContextManager state is correctly re-initialized
// when loading history from a persisted session.
//
// Usage: osm script -i scripts/test-context-rehydration.js

ctx.log("Testing ContextManager re-hydration...");

const {contextManager} = require('osm:ctxutil');
const nextIntegerId = require('osm:nextIntegerId');

const MODE_NAME = "context-test";
const sharedSymbols = require('osm:sharedStateSymbols');

// Define state using new API - contextItems is shared state
const state = tui.createState(MODE_NAME, {
    [sharedSymbols.contextItems]: {
        defaultValue: []
    }
});

// Register the test mode
tui.registerMode({
    name: MODE_NAME,
    tui: {
        title: "Context Re-hydration Test",
        prompt: "[context-test]> ",
        enableHistory: true
    },

    onEnter: function () {
        output.print("==================================================");
        output.print("Context Re-hydration Test Mode");
        output.print("==================================================");
        output.print("");
        output.print("This mode tests that the ContextManager correctly");
        output.print("re-hydrates its state after a session restart.");
        output.print("");
        output.print("Test procedure:");
        output.print("  1. Run: add <file1> <file2> <file3>");
        output.print("  2. Run: list (verify files are tracked)");
        output.print("  3. Run: test-txtar (verify toTxtar works)");
        output.print("  4. Run: exit");
        output.print("  5. Delete one of the test files");
        output.print("  6. Restart this script");
        output.print("  7. Run: list (verify restoration with missing marker)");
        output.print("  8. Run: remove <id> (verify remove works)");
        output.print("  9. Run: test-txtar (verify toTxtar works after remove)");
        output.print("");
    },

    commands: function () {
        const ctxmgr = contextManager({
            getItems: () => state.get(sharedSymbols.contextItems),
            setItems: (v) => state.set(sharedSymbols.contextItems, v),
            nextIntegerId: nextIntegerId,
            buildPrompt: function() {
                return "# Test Prompt\n\n" + context.toTxtar();
            }
        });

        const baseCommands = ctxmgr.commands;

        return {
            ...baseCommands,
            "test-txtar": {
                description: "Test context.toTxtar() functionality",
                handler: function (args) {
                    try {
                        const txtarContent = context.toTxtar();
                        output.print("=== toTxtar() output ===");
                        output.print(txtarContent);
                        output.print("=== End of toTxtar() ===");
                        output.print("");
                        output.print("✓ toTxtar() executed successfully");
                    } catch (e) {
                        output.print("✗ toTxtar() failed: " + e.message);
                    }
                }
            },
            "verify": {
                description: "Verify that ContextManager state is synchronized",
                handler: function (args) {
                    const items = state.get(sharedSymbols.contextItems);
                    const contextPaths = context.listPaths();

                    output.print("=== Verification Report ===");
                    output.print("");
                    output.print("Items in state: " + items.length);
                    output.print("Paths in ContextManager: " + contextPaths.length);
                    output.print("");

                    // Count file items
                    const fileItems = items.filter(it => it.type === "file");
                    output.print("File items in state: " + fileItems.length);

                    // Check for mismatches
                    let synchronized = true;
                    for (const item of fileItems) {
                        if (item.type === "file") {
                            const inContext = contextPaths.indexOf(item.label) !== -1;
                            if (!inContext) {
                                output.print("✗ File not in ContextManager: " + item.label);
                                synchronized = false;
                            }
                        }
                    }

                    if (synchronized) {
                        output.print("");
                        output.print("✓ State is synchronized!");
                    } else {
                        output.print("");
                        output.print("✗ State synchronization issue detected");
                    }
                }
            },
            "status": {
                description: "Show current session status",
                handler: function (args) {
                    const items = state.get(sharedSymbols.contextItems);
                    output.print("Session Status:");
                    output.print("  Total items: " + items.length);
                    output.print("  Files: " + items.filter(it => it.type === "file").length);
                    output.print("  Other: " + items.filter(it => it.type !== "file").length);
                }
            }
        };
    }
});

ctx.log("Context re-hydration test mode registered!");
ctx.log("Switch to test mode with: mode " + MODE_NAME);
