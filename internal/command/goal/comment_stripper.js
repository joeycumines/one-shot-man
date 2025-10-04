// Comment Stripper Goal: Remove useless comments and refactor useful ones
// This goal helps clean up codebases by removing redundant comments while preserving valuable ones

const {buildContext} = require('osm:ctxutil');

// Goal metadata
const GOAL_META = {
    name: "Comment Stripper",
    description: "Remove useless comments and refactor useful ones",
    category: "code-refactoring",
    usage: "Analyzes code files and removes redundant comments while preserving valuable documentation"
};

// State management
const STATE = {
    mode: "comment-stripper",
    contextItems: "contextItems"
};

// Initialize the mode
ctx.run("register-mode", function () {
    tui.registerMode({
        name: STATE.mode,
        tui: {
            title: "Comment Stripper",
            prompt: "(comment-stripper) > ",
            enableHistory: true,
            historyFile: ".comment-stripper_history"
        },
        onEnter: function () {
            if (!tui.getState(STATE.contextItems)) {
                tui.setState(STATE.contextItems, []);
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
    output.print("Comment Stripper: Remove useless comments, refactor useful ones");
    output.print("Type 'help' for commands. Use 'comment-stripper' to return here later.");
}

function help() {
    output.print("Commands: add, note, list, edit, remove, show, copy, run, help, exit");
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

const nextIntegerId = require('osm:nextIntegerId');
const {parseArgv, formatArgv} = require('osm:argv');
const {openEditor: osOpenEditor, clipboardCopy} = require('osm:os');

function openEditor(key, content) {
    return osOpenEditor(key, content);
}

function buildPrompt() {
    const goal = `Analyze the provided code and remove useless comments while refactoring useful ones according to these rules:

1. **Remove useless comments:**
   - Comments that merely repeat what the code does (e.g., "increment counter" for i++)
   - Outdated comments that no longer match the code
   - TODO/FIXME comments that are no longer relevant
   - Commented-out code that serves no purpose
   - Redundant header comments in simple functions

2. **Preserve and refactor useful comments:**
   - Business logic explanations and reasoning
   - Complex algorithm explanations
   - Performance considerations
   - Security implications
   - API documentation and usage examples
   - Configuration and setup instructions
   - Workaround explanations

3. **Refactoring guidelines:**
   - Move complex logic explanations closer to the relevant code
   - Convert inline comments to proper documentation where appropriate
   - Ensure remaining comments add genuine value
   - Fix grammar, spelling, and formatting of preserved comments
   - Update outdated but still relevant comments

4. **Output format:**
   - Show the cleaned code with explanations for each change
   - List removed comments with reasons for removal
   - List preserved/refactored comments with explanations
   - Provide a summary of the improvements made

Maintain all functionality and behavior of the original code while improving its readability and maintainability.`;

    const pb = tui.createPromptBuilder("comment-stripper", "Build comment stripper prompt");
    pb.setTemplate(`**${GOAL_META.description.toUpperCase()}**

{{goal}}

## CODE TO ANALYZE

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
                    const edited = openEditor("paths", "\n# one path per line\n");
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
            description: "Add a freeform note",
            usage: "note [text]",
            handler: function (args) {
                let text = args.join(" ");
                if (!text) text = openEditor("note", "");
                const id = addItem("note", "note", text);
                output.print("Added note [" + id + "]");
            }
        },
        list: {
            description: "List context items",
            handler: function () {
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
                    const edited = openEditor("edit-note", item.payload);
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
            description: "Show the comment stripper prompt",
            handler: function () {
                output.print(buildPrompt());
            }
        },
        copy: {
            description: "Copy comment stripper prompt to clipboard",
            handler: function () {
                const text = buildPrompt();
                try {
                    clipboardCopy(text);
                    output.print("Comment stripper prompt copied to clipboard.");
                } catch (e) {
                    output.print("Clipboard error: " + (e && e.message ? e.message : e));
                }
            }
        },
        run: {
            description: "Quick run - add files and show prompt",
            usage: "run [file ...]",
            argCompleters: ["file"],
            handler: function (args) {
                if (args.length === 0) {
                    output.print("Usage: run [file ...]");
                    return;
                }
                // Add files
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
                // Show prompt
                output.print("\n" + buildPrompt());
            }
        },
        help: {description: "Show help", handler: help}
    };
}

// Export metadata for the goals system
if (typeof module !== 'undefined' && module.exports) {
    module.exports = GOAL_META;
}