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
    
    onEnter: function() {
        console.log("Welcome to LLM Prompt Builder!");
        console.log("This mode helps you build and refine prompts for LLM services.");
        console.log("");
        console.log("Available commands:");
        console.log("  new <title>          - Create a new prompt");
        console.log("  load <title>         - Load an existing prompt");
        console.log("  template <text>      - Set the prompt template");
        console.log("  var <key> <value>    - Set a template variable");
        console.log("  build                - Build the current prompt");
        console.log("  preview              - Preview the current prompt");
        console.log("  save <notes>         - Save current version");
        console.log("  versions             - List all versions");
        console.log("  restore <version>    - Restore a specific version");
        console.log("  export               - Export prompt data");
        console.log("");
        
        // Initialize empty state
        tui.setState("currentPrompt", null);
        tui.setState("prompts", {});
    },
    
    onExit: function() {
        console.log("Exiting LLM Prompt Builder mode. Goodbye!");
    },
    
    commands: {
        "new": {
            description: "Create a new prompt",
            usage: "new <title> [description]",
            handler: function(args) {
                if (args.length < 1) {
                    console.log("Usage: new <title> [description]");
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
                
                console.log("Created new prompt: " + title);
                console.log("Description: " + description);
                console.log("Use 'template' to set the prompt template.");
            }
        },
        
        "load": {
            description: "Load an existing prompt",
            usage: "load <title>",
            handler: function(args) {
                if (args.length < 1) {
                    console.log("Usage: load <title>");
                    return;
                }
                
                var title = args[0];
                var prompts = tui.getState("prompts") || {};
                
                if (prompts[title]) {
                    tui.setState("currentPrompt", prompts[title]);
                    console.log("Loaded prompt: " + title);
                } else {
                    console.log("Prompt not found: " + title);
                    console.log("Available prompts: " + Object.keys(prompts).join(", "));
                }
            }
        },
        
        "template": {
            description: "Set the prompt template",
            usage: "template <text>",
            handler: function(args) {
                var prompt = tui.getState("currentPrompt");
                if (!prompt) {
                    console.log("No active prompt. Use 'new' to create one.");
                    return;
                }
                
                var template = args.join(" ");
                prompt.setTemplate(template);
                
                console.log("Template set:");
                console.log(template);
                console.log("");
                console.log("Use variables like {{variable_name}} in your template.");
                console.log("Set variables with: var <name> <value>");
            }
        },
        
        "var": {
            description: "Set a template variable",
            usage: "var <key> <value>",
            handler: function(args) {
                var prompt = tui.getState("currentPrompt");
                if (!prompt) {
                    console.log("No active prompt. Use 'new' to create one.");
                    return;
                }
                
                if (args.length < 2) {
                    console.log("Usage: var <key> <value>");
                    return;
                }
                
                var key = args[0];
                var value = args.slice(1).join(" ");
                
                prompt.setVariable(key, value);
                
                console.log("Set variable: " + key + " = " + value);
            }
        },
        
        "build": {
            description: "Build the current prompt",
            handler: function(args) {
                var prompt = tui.getState("currentPrompt");
                if (!prompt) {
                    console.log("No active prompt. Use 'new' to create one.");
                    return;
                }
                
                var built = prompt.build();
                console.log("Built prompt:");
                console.log("─".repeat(50));
                console.log(built);
                console.log("─".repeat(50));
            }
        },
        
        "preview": {
            description: "Preview the current prompt with metadata",
            handler: function(args) {
                var prompt = tui.getState("currentPrompt");
                if (!prompt) {
                    console.log("No active prompt. Use 'new' to create one.");
                    return;
                }
                
                console.log(prompt.preview());
            }
        },
        
        "save": {
            description: "Save current version",
            usage: "save [notes]",
            handler: function(args) {
                var prompt = tui.getState("currentPrompt");
                if (!prompt) {
                    console.log("No active prompt. Use 'new' to create one.");
                    return;
                }
                
                var notes = args.join(" ") || "Version saved from terminal";
                var tags = ["terminal", "manual"];
                
                prompt.saveVersion(notes, tags);
                
                var stats = prompt.stats();
                console.log("Saved version " + stats.versions);
                console.log("Notes: " + notes);
            }
        },
        
        "versions": {
            description: "List all versions",
            handler: function(args) {
                var prompt = tui.getState("currentPrompt");
                if (!prompt) {
                    console.log("No active prompt. Use 'new' to create one.");
                    return;
                }
                
                var versions = prompt.listVersions();
                if (versions.length === 0) {
                    console.log("No versions saved yet. Use 'save' to create a version.");
                    return;
                }
                
                console.log("Prompt versions:");
                for (var i = 0; i < versions.length; i++) {
                    var v = versions[i];
                    console.log("  v" + v.version + " - " + v.createdAt + " - " + v.notes);
                    if (v.tags && v.tags.length > 0) {
                        console.log("    Tags: " + v.tags.join(", "));
                    }
                }
            }
        },
        
        "restore": {
            description: "Restore a specific version",
            usage: "restore <version>",
            handler: function(args) {
                var prompt = tui.getState("currentPrompt");
                if (!prompt) {
                    console.log("No active prompt. Use 'new' to create one.");
                    return;
                }
                
                if (args.length < 1) {
                    console.log("Usage: restore <version>");
                    return;
                }
                
                var versionNum = parseInt(args[0]);
                if (isNaN(versionNum)) {
                    console.log("Invalid version number: " + args[0]);
                    return;
                }
                
                try {
                    prompt.restoreVersion(versionNum);
                    console.log("Restored to version " + versionNum);
                } catch (e) {
                    console.log("Error: " + e.message);
                }
            }
        },
        
        "export": {
            description: "Export prompt data",
            handler: function(args) {
                var prompt = tui.getState("currentPrompt");
                if (!prompt) {
                    console.log("No active prompt. Use 'new' to create one.");
                    return;
                }
                
                var data = prompt.export();
                console.log("Prompt export:");
                console.log(JSON.stringify(data, null, 2));
            }
        },
        
        "list": {
            description: "List all prompts",
            handler: function(args) {
                var prompts = tui.getState("prompts") || {};
                var names = Object.keys(prompts);
                
                if (names.length === 0) {
                    console.log("No prompts created yet. Use 'new' to create one.");
                    return;
                }
                
                console.log("Available prompts:");
                for (var i = 0; i < names.length; i++) {
                    var name = names[i];
                    var prompt = prompts[name];
                    var stats = prompt.stats();
                    console.log("  " + name + " - " + stats.description + " (" + stats.versions + " versions)");
                }
            }
        }
    }
});

// Switch to the LLM prompt builder mode
tui.switchMode("llm-prompt-builder");

ctx.log("LLM Prompt Builder mode registered and activated!");
ctx.log("You are now in interactive mode. Type 'help' for available commands.");