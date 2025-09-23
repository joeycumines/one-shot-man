// Demo script to show completion precedence fix
// This shows that mode commands now properly override built-in commands

tui.registerMode({
    name: "demo",
    tui: {
        title: "Demo Mode - Completion Precedence",
        prompt: "[demo]> "
    },
    onEnter: function() {
        output.print("=== Completion Precedence Demo ===");
        output.print("Type 'he' and press TAB to see completion suggestions.");
        output.print("You should only see 'help Show custom help' (not 'help Built-in command')");
        output.print("This demonstrates that mode commands take precedence over built-in commands.");
        output.print("Type 'exit' to return to main prompt.");
    },
    commands: {
        help: {
            description: "Show custom help",
            handler: function() {
                output.print("🎉 This is the CUSTOM help command from the mode!");
                output.print("The fix ensures this command takes precedence over the built-in help.");
                output.print("Before the fix, you would see BOTH completions:");
                output.print("  - help   Built-in command");
                output.print("  - help   Show custom help");
                output.print("Now you only see the mode's help command! ✅");
            }
        },
        test: {
            description: "Test custom command",
            handler: function() {
                output.print("This is a custom test command in demo mode.");
            }
        }
    }
});

// Switch to demo mode immediately
tui.switchMode("demo");
