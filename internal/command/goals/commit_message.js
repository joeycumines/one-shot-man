// Commit Message Goal: Generate commit messages based on Kubernetes style guidelines
// This goal helps generate commit messages using diff context following K8s semantic conventions

const {buildContext} = require('osm:ctxutil');

// Goal metadata
const GOAL_META = {
    name: "Commit Message",
    description: "Generate Kubernetes-style commit messages from diffs and context",
    category: "git-workflow",
    usage: "Generates commit messages following Kubernetes semantic guidelines from git diffs and additional context"
};

// State management
const STATE = {
    mode: "commit-message",
    contextItems: "contextItems"
};

// Initialize the mode
ctx.run("register-mode", function () {
    tui.registerMode({
        name: STATE.mode,
        tui: {
            title: "Commit Message",
            prompt: "(commit-message) > ",
            enableHistory: true,
            historyFile: ".commit-message_history"
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
    output.print("Commit Message: Generate Kubernetes-style commit messages");
    output.print("Type 'help' for commands. Use 'commit-message' to return here later.");
}

function help() {
    output.print("Commands: add, diff, note, list, edit, remove, show, copy, run, help, exit");
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
    const goal = `You MUST produce a commit message strictly utilizing the following syntax / style / semantics.

Kubernetes (K8s) commit message guidelines emphasize clarity, conciseness, and adherence to a specific format for better code history and reviewability. ATTN: This style is explicitly NOT "conventional commit" formatted. The core principles are:

Subject Line:
    Concise Summary: The first line, the subject, should provide a brief summary of the change.
    Length: Aim for 50 characters or less, and do not exceed 72 characters.
    Imperative Mood: Use the imperative mood (e.g., "Add feature," "Fix bug," not "Added feature" or "Adds feature").
    Capitalization: Capitalize the first word of the subject unless it's a lowercase symbol or identifier. 
    No Period: Do not end the subject line with a period. 

Body:
    Blank Line Separation: Add a single blank line between the subject and the body.
    Detailed Explanation: The body should explain the "what" and "why" of the commit, providing context and rationale for the changes. Avoid simply restating "how" the change was implemented, as the code itself will show that.
    Wrap at 72 Characters: Wrap the lines of the body at 72 characters for readability. 

General Guidelines:
    Avoid GitHub Keywords/Mentions:
    Do not use GitHub keywords (e.g., "fixes #123") or @mentions within the commit message itself. These belong in the Pull Request description.
    Squash Small Commits:
    For minor changes like typos or style fixes, consider squashing commits to maintain a cleaner git history.
    Meaningful Messages:
    Avoid vague messages like "fixed stuff" or "updated code." Strive for clear and meaningful descriptions.

Generate a commit message that follows these guidelines based on the provided diff and context information.`;

    const pb = tui.createPromptBuilder("commit-message", "Build commit message prompt");
    pb.setTemplate(`**${GOAL_META.description.toUpperCase()}**

{{goal}}

## DIFF CONTEXT / CHANGES

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
        diff: {
            description: "Add git diff output to context",
            usage: "diff [git-command-args...]",
            handler: function (args) {
                let gitCmd = "git diff";
                if (args.length > 0) {
                    gitCmd = "git " + args.join(" ");
                }
                const id = addItem("lazy-diff", gitCmd, {command: gitCmd});
                output.print("Added lazy diff [" + id + "] " + gitCmd);
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
            description: "Show the commit message prompt",
            handler: function () {
                output.print(buildPrompt());
            }
        },
        copy: {
            description: "Copy commit message prompt to clipboard",
            handler: function () {
                const text = buildPrompt();
                try {
                    clipboardCopy(text);
                    output.print("Commit message prompt copied to clipboard.");
                } catch (e) {
                    output.print("Clipboard error: " + (e && e.message ? e.message : e));
                }
            }
        },
        run: {
            description: "Quick run - add git diff and show prompt",
            usage: "run [git-diff-args...]",
            handler: function (args) {
                // Add git diff by default or with provided args
                let gitCmd = "git diff";
                if (args.length > 0) {
                    gitCmd = "git " + args.join(" ");
                }
                const id = addItem("lazy-diff", gitCmd, {command: gitCmd});
                output.print("Added lazy diff [" + id + "] " + gitCmd);
                
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