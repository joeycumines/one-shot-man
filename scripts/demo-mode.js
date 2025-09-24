// Demo Mode - Simple example of rich TUI mode system
// Usage: osm script -i scripts/demo-mode.js

ctx.log("Setting up demo mode...");

// Register a simple demo mode
tui.registerMode({
    name: "demo",
    tui: {
        title: "Demo Mode",
        prompt: "[demo]> ",
        enableHistory: false
    },

    onEnter: function () {
        output.print("Entered demo mode!");
        output.print("This is a simple demonstration of the mode system.");

        // Initialize some state
        tui.setState("counter", 0);
        tui.setState("messages", []);
    },

    onExit: function () {
        output.print("Leaving demo mode...");
        var counter = tui.getState("counter");
        output.print("Final counter value: " + counter);
    },

    commands: {
        "count": {
            description: "Increment the counter",
            handler: function (args) {
                var counter = tui.getState("counter") || 0;
                counter++;
                tui.setState("counter", counter);
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
                var messages = tui.getState("messages") || [];
                messages.push(text);
                tui.setState("messages", messages);

                output.print("Added message: " + text);
                output.print("Total messages: " + messages.length);
            }
        },

        "show": {
            description: "Show current state",
            handler: function (args) {
                var counter = tui.getState("counter");
                var messages = tui.getState("messages");

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
