// Code Documentation Generator Goal: Generate comprehensive documentation for code
// This goal helps create thorough documentation for codebases

const {buildContext} = require('osm:ctxutil');

// Goal metadata
const GOAL_META = {
    name: "Code Documentation Generator",
    description: "Generate comprehensive documentation for code structures",
    category: "documentation",
    usage: "Analyzes code and generates detailed documentation including API docs, examples, and usage guides"
};

// State management
const STATE = {
    mode: "doc-generator",
    contextItems: "contextItems",
    docType: "docType"
};

const nextIntegerId = require('osm:nextIntegerId');
const {openEditor: osOpenEditor, clipboardCopy} = require('osm:os');

// Initialize the mode
ctx.run("register-mode", function () {
    tui.registerMode({
        name: STATE.mode,
        tui: {
            title: "Code Documentation Generator",
            prompt: "(doc-gen) > ",
            enableHistory: true,
            historyFile: ".doc-generator_history"
        },
        onEnter: function () {
            if (!tui.getState(STATE.contextItems)) {
                tui.setState(STATE.contextItems, []);
            }
            if (!tui.getState(STATE.docType)) {
                tui.setState(STATE.docType, "comprehensive");
            }
            banner();
            help();
        },
        onExit: function () {
            output.print("Goodbye!");
        },
        commands: buildCommands()
    });
});

function banner() {
    output.print("Code Documentation Generator: Create comprehensive code documentation");
    output.print("Type 'help' for commands. Use 'doc-generator' to return here later.");
}

function help() {
    output.print("Commands: add, note, list, type, edit, remove, show, copy, help, exit");
    output.print("Doc types: comprehensive, api, readme, inline, tutorial");
}

function items() {
    return tui.getState(STATE.contextItems) || [];
}

function setItems(v) {
    tui.setState(STATE.contextItems, v);
}

function addItem(type, label, payload) {
    const list = items();
    const id = nextIntegerId(list);
    list.push({id, type, label, payload});
    setItems(list);
    return id;
}

function buildPrompt() {
    const docType = tui.getState(STATE.docType) || "comprehensive";
    
    const docTypeInstructions = {
        comprehensive: `Generate comprehensive documentation including:
- Overview and purpose
- Architecture and design decisions
- API documentation with examples
- Usage guides and tutorials
- Configuration options
- Troubleshooting guides
- Contributing guidelines`,
        
        api: `Generate API documentation including:
- Function/method signatures with parameter descriptions
- Return value specifications
- Usage examples for each function
- Error handling information
- Type definitions and interfaces`,
        
        readme: `Generate a README.md file including:
- Project description and purpose
- Installation instructions
- Quick start guide
- Basic usage examples
- Configuration overview
- Links to additional documentation`,
        
        inline: `Generate inline code documentation:
- Add comprehensive comments to functions and methods
- Document complex algorithms and business logic
- Add type annotations and parameter descriptions
- Include usage examples in comments`,
        
        tutorial: `Generate step-by-step tutorials including:
- Learning objectives
- Prerequisites
- Detailed implementation steps
- Code examples with explanations
- Common pitfalls and solutions
- Next steps and advanced topics`
    };

    const goal = `Create ${docType} documentation for the provided code following these guidelines:

${docTypeInstructions[docType]}

**Documentation Standards:**
- Use clear, concise language appropriate for the target audience
- Include practical examples that users can copy and run
- Organize information logically with proper headings and structure
- Maintain consistency in formatting and style
- Ensure accuracy and completeness
- Include relevant links and cross-references

**Output Format:**
- Provide the documentation in appropriate format (Markdown, JSDoc, etc.)
- Include a brief explanation of the documentation structure
- Highlight key sections and important information
- Ensure the documentation is ready to use without further editing`;

    const pb = tui.createPromptBuilder("doc-generator", "Build documentation generator prompt");
    pb.setTemplate(`**${GOAL_META.description.toUpperCase()}**

{{goal}}

## CODE TO DOCUMENT

{{context_txtar}}`);
    
    const fullContext = buildContext(items(), {toTxtar: () => context.toTxtar()});
    pb.setVariable("goal", goal);
    pb.setVariable("context_txtar", fullContext);
    return pb.build();
}

function buildCommands() {
    return {
        add: {
            description: "Add file content to context",
            usage: "add [file ...]",
            argCompleters: ["file"],
            handler: function (args) {
                if (args.length === 0) {
                    const edited = osOpenEditor("paths", "\n# one path per line\n");
                    args = edited.split(/\r?\n/).map(s => s.trim()).filter(s => s && !s.startsWith('#'));
                }
                for (const p of args) {
                    try {
                        const err = context.addPath(p);
                        if (err && err.message) {
                            output.print("Error adding " + p + ": " + err.message);
                            continue;
                        }
                        const id = addItem("file", p, {path: p});
                        output.print("Added file [" + id + "] " + p);
                    } catch (e) {
                        output.print("Error: " + e);
                    }
                }
            }
        },
        note: {
            description: "Add a note about documentation requirements",
            usage: "note [text]",
            handler: function (args) {
                let text = args.join(" ");
                if (!text) text = osOpenEditor("note", "");
                const id = addItem("note", "note", text);
                output.print("Added note [" + id + "]");
            }
        },
        type: {
            description: "Set documentation type",
            usage: "type <comprehensive|api|readme|inline|tutorial>",
            handler: function (args) {
                if (args.length === 0) {
                    output.print("Current type: " + (tui.getState(STATE.docType) || "comprehensive"));
                    output.print("Available types: comprehensive, api, readme, inline, tutorial");
                    return;
                }
                const type = args[0].toLowerCase();
                const validTypes = ["comprehensive", "api", "readme", "inline", "tutorial"];
                if (!validTypes.includes(type)) {
                    output.print("Invalid type. Available: " + validTypes.join(", "));
                    return;
                }
                tui.setState(STATE.docType, type);
                output.print("Documentation type set to: " + type);
            }
        },
        list: {
            description: "List context items and settings",
            handler: function () {
                output.print("Documentation type: " + (tui.getState(STATE.docType) || "comprehensive"));
                output.print("Context items:");
                for (const it of items()) {
                    let line = "[" + it.id + "] [" + it.type + "] " + (it.label || "");
                    if (it.type === "note" && typeof it.payload === "string") {
                        const preview = it.payload.slice(0, 60);
                        line += ": " + preview + (it.payload.length > 60 ? "..." : "");
                    }
                    output.print(line);
                }
            }
        },
        edit: {
            description: "Edit a context item",
            usage: "edit <id>",
            handler: function (args) {
                if (args.length === 0) {
                    output.print("Usage: edit <id>");
                    return;
                }
                const id = parseInt(args[0]);
                const list = items();
                const idx = list.findIndex(it => it.id === id);
                if (idx === -1) {
                    output.print("Item [" + id + "] not found");
                    return;
                }
                const item = list[idx];
                if (item.type === "note") {
                    const edited = osOpenEditor("edit-note", item.payload);
                    if (edited !== null && edited.trim()) {
                        item.payload = edited.trim();
                        setItems(list);
                        output.print("Updated [" + id + "]");
                    }
                } else {
                    output.print("Cannot edit item type: " + item.type);
                }
            }
        },
        remove: {
            description: "Remove a context item",
            usage: "remove <id>",
            handler: function (args) {
                if (args.length === 0) {
                    output.print("Usage: remove <id>");
                    return;
                }
                const id = parseInt(args[0]);
                const list = items();
                const idx = list.findIndex(it => it.id === id);
                if (idx === -1) {
                    output.print("Item [" + id + "] not found");
                    return;
                }
                const item = list[idx];
                if (item.type === "file") {
                    try {
                        const err = context.removePath(item.payload.path);
                        if (err && err.message) {
                            output.print("Error removing from context: " + err.message);
                            return;
                        }
                    } catch (e) {
                        output.print("Error: " + e);
                        return;
                    }
                }
                list.splice(idx, 1);
                setItems(list);
                output.print("Removed [" + id + "]");
            }
        },
        show: {
            description: "Show the documentation generator prompt",
            handler: function () {
                output.print(buildPrompt());
            }
        },
        copy: {
            description: "Copy documentation generator prompt to clipboard",
            handler: function () {
                const text = buildPrompt();
                try {
                    clipboardCopy(text);
                    output.print("Documentation generator prompt copied to clipboard.");
                } catch (e) {
                    output.print("Clipboard error: " + (e && e.message ? e.message : e));
                }
            }
        },
        help: {description: "Show help", handler: help}
    };
}

// Export metadata for the goals system
if (typeof module !== 'undefined' && module.exports) {
    module.exports = GOAL_META;
}