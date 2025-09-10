// Demo Mode - Simple example of rich TUI mode system
// Usage: one-shot-man script -i scripts/demo-mode.js

ctx.log("Setting up demo mode...");

// Register a simple demo mode
tui.registerMode({
    name: "demo",
    tui: {
        title: "Demo Mode",
        prompt: "[demo]> ",
        enableHistory: false
    },
    
    onEnter: function() {
        console.log("Entered demo mode!");
        console.log("This is a simple demonstration of the mode system.");
        
        // Initialize some state
        tui.setState("counter", 0);
        tui.setState("messages", []);
    },
    
    onExit: function() {
        console.log("Leaving demo mode...");
        var counter = tui.getState("counter");
        console.log("Final counter value: " + counter);
    },
    
    commands: {
        "count": {
            description: "Increment the counter",
            handler: function(args) {
                var counter = tui.getState("counter") || 0;
                counter++;
                tui.setState("counter", counter);
                console.log("Counter: " + counter);
            }
        },
        
        "message": {
            description: "Add a message to the list",
            usage: "message <text>",
            handler: function(args) {
                if (args.length === 0) {
                    console.log("Usage: message <text>");
                    return;
                }
                
                var text = args.join(" ");
                var messages = tui.getState("messages") || [];
                messages.push(text);
                tui.setState("messages", messages);
                
                console.log("Added message: " + text);
                console.log("Total messages: " + messages.length);
            }
        },
        
        "show": {
            description: "Show current state",
            handler: function(args) {
                var counter = tui.getState("counter");
                var messages = tui.getState("messages");
                
                console.log("Current state:");
                console.log("  Counter: " + counter);
                console.log("  Messages: " + messages.length);
                
                if (messages.length > 0) {
                    console.log("  Recent messages:");
                    for (var i = Math.max(0, messages.length - 3); i < messages.length; i++) {
                        console.log("    " + (i + 1) + ". " + messages[i]);
                    }
                }
            }
        },
        
        "js": {
            description: "Execute JavaScript code in mode context",
            usage: "js <code>",
            handler: function(args) {
                if (args.length === 0) {
                    console.log("Usage: js <code>");
                    console.log("Example: js console.log('Hello from demo mode!')");
                    return;
                }
                
                var code = args.join(" ");
                try {
                    // This would execute in the current JavaScript context
                    eval(code);
                } catch (e) {
                    console.log("Error: " + e.message);
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
    handler: function(args) {
        console.log(args.join(" "));
    }
});

ctx.log("Demo mode registered!");
ctx.log("Available modes: " + tui.listModes().join(", "));
ctx.log("Switch to demo mode with: mode demo");