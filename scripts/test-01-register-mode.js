#!/usr/bin/env osm script -i

// Demo Mode - Simple example of rich TUI mode system

ctx.log("Setting up demo mode...");

const MODE_NAME = "demo";

// Define state using new API
const state = tui.createState(MODE_NAME, {
    counter: {
        defaultValue: 0
    },
    messages: {
        defaultValue: []
    }
});

// Register a simple demo mode
tui.registerMode({
    name: MODE_NAME,
    tui: {
        title: "Demo Mode",
        prompt: "[demo]> ",
        enableHistory: false
    },

    onEnter: function () {
        output.print("Entered demo mode!");
        output.print("This is a simple demonstration of the mode system.");
    },

    onExit: function () {
        output.print("Leaving demo mode...");
        var counter = state.get("counter");
        output.print("Final counter value: " + counter);
    },

    commands: function () {
        return {
            "count": {
                description: "Increment the counter",
                handler: function (args) {
                    var counter = state.get("counter");
                    counter++;
                    state.set("counter", counter);
                    output.print("Counter: " + counter);
                }
            },

            "message": {
                description: "Add a message to the list",
                usage: "message <text>",
                handler: function (args) {
                    if (args.length === 0) {
                        output.print("Usage: message <text>");
                        return;
                    }

                    var text = args.join(" ");
                    var messages = state.get("messages");
                    messages.push(text);
                    state.set("messages", messages);

                    output.print("Added message: " + text);
                    output.print("Total messages: " + messages.length);
                }
            },

            "show": {
                description: "Show current state",
                handler: function (args) {
                    var counter = state.get("counter");
                    var messages = state.get("messages");

                    output.print("Current state:");
                    output.print("  Counter: " + counter);
                    output.print("  Messages: " + messages.length);

                    if (messages.length > 0) {
                        output.print("  Recent messages:");
                        for (var i = Math.max(0, messages.length - 3); i < messages.length; i++) {
                            output.print("    " + (i + 1) + ". " + messages[i]);
                        }
                    }
                }
            },

            "js": {
                description: "Execute JavaScript code in mode context",
                usage: "js <code>",
                handler: function (args) {
                    if (args.length === 0) {
                        output.print("Usage: js <code>");
                        output.print("Example: js output.print('Hello from demo mode!')");
                        return;
                    }

                    var code = args.join(" ");
                    try {
                        // This would execute in the current JavaScript context
                        eval(code);
                    } catch (e) {
                        output.print("Error: " + e.message);
                    }
                }
            }
        };
    }
});

// Register a global command that works in any mode
tui.registerCommand({
    name: "echo",
    description: "Echo the arguments",
    usage: "echo <text>",
    handler: function (args) {
        output.print(args.join(" "));
    }
});

ctx.log("Demo mode registered!");
ctx.log("Available modes: " + tui.listModes().join(", "));
ctx.log("Switch to demo mode with: mode demo");
