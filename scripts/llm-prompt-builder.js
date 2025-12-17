// LLM Prompt Builder - (Legacy) Demo Script
// Usage: osm script -i scripts/llm-prompt-builder.js

ctx.log("Initializing LLM Prompt Builder mode...");

const MODE_NAME = "llm-prompt-builder";

// Define state keys using Symbols
const stateKeys = {
    currentPrompt: Symbol("currentPrompt"),
    prompts: Symbol("prompts")
};

// Define state using new API
const state = tui.createState(MODE_NAME, {
    [stateKeys.currentPrompt]: {
        defaultValue: null
    },
    [stateKeys.prompts]: {
        defaultValue: {}
    }
});

// N.B. This was implemented in Go, but was removed due to its general lack of utility.
// It was ported to JS largely to support existing integration tests.
class PromptBuilder {
    constructor(title, description) {
        this.title = title;
        this.description = description;
        this.template = "";
        this.variables = {};
        this.history = [];
        this.current = null;
    }

    setTemplate(template) {
        this.template = template;
    }

    setVariable(key, value) {
        this.variables[key] = value;
    }

    build() {
        var content = this.template;

        // Replace variables in the template
        // Go logic: placeholder := fmt.Sprintf("{{%s}}", key)
        for (var key in this.variables) {
            if (this.variables.hasOwnProperty(key)) {
                var placeholder = "{{" + key + "}}";
                var replacement = String(this.variables[key]);
                // Mimic strings.ReplaceAll
                content = content.split(placeholder).join(replacement);
            }
        }

        return content;
    }

    saveVersion(notes, tags) {
        // Deep copy current variables
        var variablesCopy = {};
        for (var k in this.variables) {
            if (this.variables.hasOwnProperty(k)) {
                variablesCopy[k] = this.variables[k];
            }
        }

        var version = {
            version: this.history.length + 1,
            content: this.build(),
            variables: variablesCopy,
            createTime: new Date().toISOString(),
            notes: notes,
            tags: tags
        };

        this.history.push(version);
        this.current = version;
    }

    restoreVersion(versionNum) {
        if (versionNum < 1 || versionNum > this.history.length) {
            throw new Error("version " + versionNum + " not found");
        }

        // Get version (1-based index to 0-based)
        var version = this.history[versionNum - 1];

        // Go logic: pb.Template = version.Content
        this.template = version.content;

        // Go logic: Restore variables (deep copy back)
        this.variables = {};
        for (var k in version.variables) {
            if (version.variables.hasOwnProperty(k)) {
                this.variables[k] = version.variables[k];
            }
        }

        this.current = version;
    }

    listVersions() {
        var versions = [];
        for (var i = 0; i < this.history.length; i++) {
            var v = this.history[i];
            versions.push({
                version: v.version,
                createTime: v.createTime,
                notes: v.notes,
                tags: v.tags,
                content: v.content
            });
        }
        return versions;
    }

    export() {
        return {
            title: this.title,
            description: this.description,
            template: this.template,
            variables: this.variables,
            current: this.build(),
            versions: this.listVersions()
        };
    }

    // Helpers inferred from existing JS usage
    stats() {
        return {
            description: this.description,
            versions: this.history.length
        };
    }

    preview() {
        var out = "Title: " + this.title + "\n";
        out += "Description: " + this.description + "\n";
        out += "Variables: " + JSON.stringify(this.variables) + "\n";
        out += "--- Build Preview ---\n";
        out += this.build();
        return out;
    }
}

// In-memory storage of prompt instances so JS class methods remain available.
// Using the StateManager for complex JS objects will convert them to plain
// Go-native maps and strip methods — keep runtime objects in memory instead.
const _prompts = {};
let _currentPrompt = null;

// Register the LLM prompt builder mode
tui.registerMode({
    name: MODE_NAME,
    tui: {
        title: "LLM Prompt Builder",
        prompt: "[prompt-builder]> ",
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
    },

    onExit: function () {
        output.print("Exiting LLM Prompt Builder mode. Goodbye!");
    },

    commands: function () {
        return {
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

                    var prompt = new PromptBuilder(title, description);

                    // Keep runtime prompt instances in-memory so methods are preserved
                    _currentPrompt = prompt;
                    _prompts[title] = prompt;

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

                    if (_prompts[title]) {
                        _currentPrompt = _prompts[title];
                        output.print("Loaded prompt: " + title);
                    } else {
                        output.print("Prompt not found: " + title);
                        output.print("Available prompts: " + Object.keys(_prompts).join(", "));
                    }
                }
            },

            "template": {
                description: "Set the prompt template",
                usage: "template <text>",
                handler: function (args) {
                    var prompt = _currentPrompt;
                    if (!prompt) {
                        output.print("No active prompt. Use 'new' to create one.");
                        return;
                    }

                    var template = args.join(" ");
                    prompt.setTemplate(template);

                    output.print("Template set:");
                    output.print(template);
                    output.print("");
                    output.print("Use variables like {{variableName}} in your template.");
                    output.print("Set variables with: var <name> <value>");
                }
            },

            "var": {
                description: "Set a template variable",
                usage: "var <key> <value>",
                handler: function (args) {
                    var prompt = _currentPrompt;
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
                    var prompt = _currentPrompt;
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
                    var prompt = _currentPrompt;
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
                    var prompt = _currentPrompt;
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
                    var prompt = _currentPrompt;
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
                        output.print("  v" + v.version + " - " + v.createTime + " - " + v.notes);
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
                    var prompt = _currentPrompt;
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
                    var prompt = _currentPrompt;
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
                    var names = Object.keys(_prompts);

                    if (names.length === 0) {
                        output.print("No prompts created yet. Use 'new' to create one.");
                        return;
                    }

                    output.print("Available prompts:");
                    for (var i = 0; i < names.length; i++) {
                        var name = names[i];
                        var prompt = _prompts[name];
                        var stats = prompt.stats();
                        output.print("  " + name + " - " + stats.description + " (" + stats.versions + " versions)");
                    }
                }
            }
        };
    }
});

ctx.log("LLM Prompt Builder mode registered!");
