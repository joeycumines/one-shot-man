// Test script to demonstrate the new go-prompt integration

// Register a test mode with advanced prompt configuration
tui.registerMode({
    name: "test",
    tui: {
        title: "Test Mode",
        prompt: "[test]> ",
    },
    onEnter: function () {
        ctx.log("Entered test mode - creating advanced prompt");

        // Create an advanced prompt with custom configuration
        var promptConfig = {
            name: "testPrompt",
            title: "Advanced Test Prompt",
            prefix: "advanced> ",
            colors: {
                prefix: "cyan",
                input: "white",
                suggestionText: "yellow",
                suggestionBackground: "black",
                selectedSuggestionBackground: "cyan"
            },
            history: {
                enabled: true,
                file: ".advanced_history",
                size: 500
            }
        };

        try {
            var promptHandle = tui.createAdvancedPrompt(promptConfig);
            ctx.log("Created advanced prompt with handle: " + promptHandle);

            // Register a completer for file completion
            tui.registerCompleter("fileCompleter", function (document) {
                var word = document.getWordBeforeCursor();
                var files = context.listPaths();
                var suggestions = [];

                for (var i = 0; i < files.length; i++) {
                    if (files[i].indexOf(word) !== -1) {
                        suggestions.push({
                            text: files[i],
                            description: "File from context"
                        });
                    }
                }

                return suggestions;
            });

            // Set the completer for our prompt
            tui.setCompleter(promptHandle, "fileCompleter");
            ctx.log("Registered file completer for advanced prompt");

            // Register a key binding
            tui.registerKeyBinding("ctrl-h", function () {
                output.print("Help: This is a test mode with advanced go-prompt integration!");
                return true; // re-render
            });

            ctx.log("Registered Ctrl-H key binding");
            ctx.log("Type 'help' for commands, 'advanced' to run advanced prompt, or 'exit' to quit");

        } catch (e) {
            ctx.error("Failed to create advanced prompt: " + e);
        }
    },
    commands: {
        advanced: {
            description: "Run the advanced prompt",
            handler: function () {
                try {
                    output.print("Starting advanced prompt...");
                    tui.runPrompt("testPrompt");
                } catch (e) {
                    output.print("Error running advanced prompt: " + e);
                }
            }
        }
    }
});

ctx.log("Test mode registered successfully");
