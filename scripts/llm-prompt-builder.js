// LLM Prompt Builder Mode - Demonstrates rich TUI for building one-shot prompts
// Usage: one-shot-man script -i scripts/llm-prompt-builder.js

ctx.log("Initializing LLM Prompt Builder mode...");

// Register the LLM prompt builder mode
tui.registerMode({
    name: "llm-prompt-builder",
    tui: {
        title: "LLM Prompt Builder",
        prompt: "[prompt-builder]> ",
        enableHistory: true,
        historyFile: ".llm-prompt-history"
    },

    onEnter: function () {
        output.print("Welcome to LLM Prompt Builder!");
        output.print("This mode helps you build and refine prompts for LLM services.");
        output.print("");
        output.print("Available commands:");
        output.print("  new <title>          - Create a new prompt");
        output.print("  load <title>         - Load an existing prompt");
        output.print("  template <text>      - Set the prompt template");
        output.print("  var <key> <value>    - Set a template variable");
        output.print("  build                - Build the current prompt");
        output.print("  preview              - Preview the current prompt");
        output.print("  save <notes>         - Save current version");
        output.print("  versions             - List all versions");
        output.print("  restore <version>    - Restore a specific version");
        output.print("  export               - Export prompt data");
        output.print("");

        // Initialize empty state
        tui.setState("currentPrompt", null);
        tui.setState("prompts", {});
    },

    onExit: function () {
        output.print("Exiting LLM Prompt Builder mode. Goodbye!");
    },

    commands: {
        "new": {
            description: "Create a new prompt",
            usage: "new <title> [description]",
            handler: function (args) {
                if (args.length < 1) {
                    output.print("Usage: new <title> [description]");
                    return;
                }

                var title = args[0];
                var description = args.slice(1).join(" ") || "A new LLM prompt";

                var prompt = tui.createPromptBuilder(title, description);
                tui.setState("currentPrompt", prompt);

                // Store in prompts collection
                var prompts = tui.getState("prompts") || {};
                prompts[title] = prompt;
                tui.setState("prompts", prompts);

                output.print("Created new prompt: " + title);
                output.print("Description: " + description);
                output.print("Use 'template' to set the prompt template.");
            }
        },

        "load": {
            description: "Load an existing prompt",
            usage: "load <title>",
            handler: function (args) {
                if (args.length < 1) {
                    output.print("Usage: load <title>");
                    return;
                }

                var title = args[0];
                var prompts = tui.getState("prompts") || {};

                if (prompts[title]) {
                    tui.setState("currentPrompt", prompts[title]);
                    output.print("Loaded prompt: " + title);
                } else {
                    output.print("Prompt not found: " + title);
                    output.print("Available prompts: " + Object.keys(prompts).join(", "));
                }
            }
        },

        "template": {
            description: "Set the prompt template",
            usage: "template <text>",
            handler: function (args) {
                var prompt = tui.getState("currentPrompt");
                if (!prompt) {
                    output.print("No active prompt. Use 'new' to create one.");
                    return;
                }

                var template = args.join(" ");
                prompt.setTemplate(template);

                output.print("Template set:");
                output.print(template);
                output.print("");
                output.print("Use variables like {{variable_name}} in your template.");
                output.print("Set variables with: var <name> <value>");
            }
        },

        "var": {
            description: "Set a template variable",
            usage: "var <key> <value>",
            handler: function (args) {
                var prompt = tui.getState("currentPrompt");
                if (!prompt) {
                    output.print("No active prompt. Use 'new' to create one.");
                    return;
                }

                if (args.length < 2) {
                    output.print("Usage: var <key> <value>");
                    return;
                }

                var key = args[0];
                var value = args.slice(1).join(" ");

                prompt.setVariable(key, value);

                output.print("Set variable: " + key + " = " + value);
            }
        },

        "build": {
            description: "Build the current prompt",
            handler: function (args) {
                var prompt = tui.getState("currentPrompt");
                if (!prompt) {
                    output.print("No active prompt. Use 'new' to create one.");
                    return;
                }

                var built = prompt.build();
                output.print("Built prompt:");
                output.print("--------------------------------------------------");
                output.print(built);
                output.print("--------------------------------------------------");
            }
        },

        "preview": {
            description: "Preview the current prompt with metadata",
            handler: function (args) {
                var prompt = tui.getState("currentPrompt");
                if (!prompt) {
                    output.print("No active prompt. Use 'new' to create one.");
                    return;
                }

                output.print(prompt.preview());
            }
        },

        "save": {
            description: "Save current version",
            usage: "save [notes]",
            handler: function (args) {
                var prompt = tui.getState("currentPrompt");
                if (!prompt) {
                    output.print("No active prompt. Use 'new' to create one.");
                    return;
                }

                var notes = args.join(" ") || "Version saved from terminal";
                var tags = ["terminal", "manual"];

                prompt.saveVersion(notes, tags);

                var stats = prompt.stats();
                output.print("Saved version " + stats.versions);
                output.print("Notes: " + notes);
            }
        },

        "versions": {
            description: "List all versions",
            handler: function (args) {
                var prompt = tui.getState("currentPrompt");
                if (!prompt) {
                    output.print("No active prompt. Use 'new' to create one.");
                    return;
                }

                var versions = prompt.listVersions();
                if (versions.length === 0) {
                    output.print("No versions saved yet. Use 'save' to create a version.");
                    return;
                }

                output.print("Prompt versions:");
                for (var i = 0; i < versions.length; i++) {
                    var v = versions[i];
                    output.print("  v" + v.version + " - " + v.createdAt + " - " + v.notes);
                    if (v.tags && v.tags.length > 0) {
                        output.print("    Tags: " + v.tags.join(", "));
                    }
                }
            }
        },

        "restore": {
            description: "Restore a specific version",
            usage: "restore <version>",
            handler: function (args) {
                var prompt = tui.getState("currentPrompt");
                if (!prompt) {
                    output.print("No active prompt. Use 'new' to create one.");
                    return;
                }

                if (args.length < 1) {
                    output.print("Usage: restore <version>");
                    return;
                }

                var versionNum = parseInt(args[0]);
                if (isNaN(versionNum)) {
                    output.print("Invalid version number: " + args[0]);
                    return;
                }

                try {
                    prompt.restoreVersion(versionNum);
                    output.print("Restored to version " + versionNum);
                } catch (e) {
                    output.print("Error: " + e.message);
                }
            }
        },

        "export": {
            description: "Export prompt data",
            handler: function (args) {
                var prompt = tui.getState("currentPrompt");
                if (!prompt) {
                    output.print("No active prompt. Use 'new' to create one.");
                    return;
                }

                var data = prompt.export();
                output.print("Prompt export:");
                output.print(JSON.stringify(data, null, 2));
            }
        },

        "list": {
            description: "List all prompts",
            handler: function (args) {
                var prompts = tui.getState("prompts") || {};
                var names = Object.keys(prompts);

                if (names.length === 0) {
                    output.print("No prompts created yet. Use 'new' to create one.");
                    return;
                }

                output.print("Available prompts:");
                for (var i = 0; i < names.length; i++) {
                    var name = names[i];
                    var prompt = prompts[name];
                    var stats = prompt.stats();
                    output.print("  " + name + " - " + stats.description + " (" + stats.versions + " versions)");
                }
            }
        }
    }
});

ctx.log("LLM Prompt Builder mode registered!");
