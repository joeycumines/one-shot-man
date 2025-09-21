// Code Review: Single-prompt code review with context (baked-in version)
// This is the built-in version of the code-review script with embedded template

// State keys
const STATE = {
    mode: "review",             // fixed mode name
    contextItems: "contextItems" // array of { id, type: file|diff|note, label, payload }
};

// Simple id generator
function nextId(list) {
    let max = 0;
    for (const it of list) max = Math.max(max, it.id || 0);
    return max + 1;
}

// Initialize the mode
ctx.run("register-mode", function () {
    tui.registerMode({
        name: STATE.mode,
        tui: {
            title: "Code Review",
            prompt: "(code-review) > ",
            enableHistory: true,
            historyFile: ".code-review_history"
        },
        onEnter: function () {
            if (!tui.getState(STATE.contextItems)) {
                tui.setState(STATE.contextItems, []);
            }
            banner();
            help();
        },
        onExit: function () {
            output.print("Exiting Code Review.");
        },
        commands: buildCommands()
    });

    tui.registerCommand({
        name: "review",
        description: "Switch to Code Review mode",
        handler: function () {
            tui.switchMode(STATE.mode);
        }
    });
});

function banner() {
    output.print("Code Review: context -> single prompt for PR review");
    output.print("Type 'help' for commands. Use 'review' to return here later.");
}

function help() {
    output.print("Commands: add, diff, note, list, edit, remove, show, copy, help, exit");
}

function items() {
    return tui.getState(STATE.contextItems) || [];
}

function setItems(v) {
    tui.setState(STATE.contextItems, v);
}

function addItem(type, label, payload) {
    const list = items();
    const id = nextId(list);
    list.push({id, type, label, payload});
    setItems(list);
    return id;
}

function buildPrompt() {
    // Execute any lazy-diff items and convert them to regular diff items
    const list = items();
    for (let i = 0; i < list.length; i++) {
        const item = list[i];
        if (item.type === "lazy-diff") {
            // Execute the git diff command now
            const diffArgs = item.payload.split(" ");
            const argv = ["git", "diff"].concat(diffArgs);
            const res = system.execv(argv);
            
            if (res && res.error) {
                // If git diff fails, store the error as the payload
                item.payload = "Error executing git diff: " + res.message;
                item.type = "diff-error";
            } else {
                // Store the actual diff output
                item.payload = res.stdout || "";
                item.type = "diff";
            }
        }
    }
    setItems(list);
    
    // Leverage context manager txtar dump
    const txtar = context.toTxtar();
    const pb = tui.createPromptBuilder("review", "Build code review prompt");
    pb.setTemplate(codeReviewTemplate);
    pb.setVariable("context_txtar", txtar);
    return pb.build();
}

function openEditor(title, initial) {
    const res = system.openEditor(title, initial || "");
    if (typeof res === 'string') return res;
    // Some engines may return [value, error]; we standardize to string
    return "" + res;
}

function buildCommands() {
    return {
        add: {
            description: "Add file content to context",
            usage: "add [file ...]",
            handler: function (args) {
                if (args.length === 0) {
                    const edited = openEditor("paths", "# one path per line\n");
                    args = edited.split(/\r?\n/).map(s => s.trim()).filter(Boolean);
                }
                for (const p of args) {
                    try {
                        const err = context.addPath(p);
                        if (err && err.message) {
                            output.print("add error: " + err.message);
                            continue;
                        }
                        addItem("file", p, "");
                        output.print("Added file: " + p);
                    } catch (e) {
                        output.print("add error: " + e);
                    }
                }
            }
        },
        diff: {
            description: "Add git diff output to context (default: HEAD~1)",
            usage: "diff [commit-spec]",
            handler: function (args) {
                // Default to HEAD~1 if no args provided, otherwise use provided args
                const diffSpec = (args && args.length > 0) ? args.join(" ") : "HEAD~1";
                const label = "git diff " + diffSpec;
                
                // Store lazy diff item - actual execution happens in buildPrompt
                addItem("lazy-diff", label, diffSpec);
                output.print("Added diff: " + label + " (will be executed when generating prompt)");
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
                    output.print("[" + it.id + "] [" + it.type + "] " + (it.label || ""));
                }
            }
        },
        edit: {
            description: "Edit context item by id",
            usage: "edit <id>",
            handler: function (args) {
                if (args.length < 1) {
                    output.print("Usage: edit <id>");
                    return;
                }
                const id = parseInt(args[0], 10);
                if (isNaN(id)) {
                    output.print("Invalid id: " + args[0]);
                    return;
                }
                const list = items();
                const idx = list.findIndex(x => x.id === id);
                if (idx === -1) {
                    output.print("Not found: " + id);
                    return;
                }
                // Disallow editing file items since file content is sourced from disk via context engine
                if (list[idx].type === 'file') {
                    output.print("Editing file content directly is not supported. Please edit the file on disk.");
                    return;
                }
                // For lazy-diff items, edit the git diff command specification
                if (list[idx].type === 'lazy-diff') {
                    const edited = openEditor("diff-spec-" + id, list[idx].payload || "HEAD~1");
                    list[idx].payload = edited.trim();
                    list[idx].label = "git diff " + edited.trim();
                    setItems(list);
                    output.print("Updated diff specification [" + id + "]");
                    return;
                }
                const edited = openEditor("item-" + id, list[idx].payload || "");
                list[idx].payload = edited;
                setItems(list);
                output.print("Edited [" + id + "]");
            }
        },
        remove: {
            description: "Remove a context item by id",
            usage: "remove <id>",
            handler: function (args) {
                if (args.length < 1) {
                    output.print("Usage: remove <id>");
                    return;
                }
                const id = parseInt(args[0], 10);
                const list = items();
                const idx = list.findIndex(x => x.id === id);
                if (idx === -1) {
                    output.print("Not found: " + id);
                    return;
                }
                const it = list[idx];
                // If a file item, also remove from Go context manager using the label (path)
                if (it.type === 'file' && it.label) {
                    try {
                        const err = context.removePath(it.label);
                        if (err) {
                            const msg = (err && err.message) ? err.message : ("" + err);
                            output.print("Error: " + msg);
                            return; // Abort removal if backend failed
                        }
                    } catch (e) {
                        output.print("Error: " + e);
                        return; // Abort removal if exception thrown
                    }
                }
                list.splice(idx, 1);
                setItems(list);
                output.print("Removed [" + id + "]");
            }
        },
        show: {
            description: "Show the code review prompt",
            handler: function () {
                output.print(buildPrompt());
            }
        },
        copy: {
            description: "Copy code review prompt to clipboard",
            handler: function () {
                const text = buildPrompt();
                const err = system.clipboardCopy(text);
                if (err && err.message) {
                    output.print("Clipboard error: " + err.message);
                } else {
                    output.print("Code review prompt copied to clipboard.");
                }
            }
        },
        help: {description: "Show help", handler: help},
    };
}

// Auto-switch into review mode when this script loads
ctx.run("enter-review", function () {
    tui.switchMode(STATE.mode);
});