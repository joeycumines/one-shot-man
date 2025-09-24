// Code Review: Single-prompt code review with context (baked-in version)
// This is the built-in version of the code-review script with embedded template

const nextIntegerId = require('osm:nextIntegerId');
const {parseArgv, formatArgv} = require('osm:argv');
const {openEditor: osOpenEditor, clipboardCopy, fileExists, getenv} = require('osm:os');
const {buildContext} = require('osm:ctxutil');
const interop = require('osm:interop');

// State keys
const STATE = {
    mode: "review",             // fixed mode name
    contextItems: "contextItems" // array of { id, type: file|diff|note, label, payload }
};

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
    output.print("Commands: add, diff, note, list, edit, remove, show, copy, export, import, commit, help, exit");
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
    const pb = tui.createPromptBuilder("review", "Build code review prompt");
    pb.setTemplate(codeReviewTemplate);
    const fullContext = buildContext(items(), {toTxtar: () => context.toTxtar()});
    pb.setVariable("context_txtar", fullContext);
    return pb.build();
}

function openEditor(title, initial) {
    const res = osOpenEditor(title, initial || "");
    if (typeof res === 'string') return res;
    // Some engines may return [value, error]; we standardize to string
    return "" + res;
}

// Simple id generator (shared with prompt_flow)
function nextId(list) {
    let max = 0;
    for (const it of list) max = Math.max(max, it.id || 0);
    return max + 1;
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
                const argv = (args && args.length > 0) ? args.slice(0) : ["HEAD~1"];
                const label = "git diff " + formatArgv(argv);

                // Store lazy diff item - actual execution happens in buildPrompt
                addItem("lazy-diff", label, argv);
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
                    let line = "[" + it.id + "] [" + it.type + "] " + (it.label || "");
                    if (it.type === 'file' && it.label && !fileExists(it.label)) {
                        line += " (missing)";
                    }
                    output.print(line);
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
                    const initial = Array.isArray(list[idx].payload) ? formatArgv(list[idx].payload) : (list[idx].payload || "HEAD~1");
                    const edited = openEditor("diff-spec-" + id, initial);
                    const argv = parseArgv((edited || "").trim());
                    list[idx].payload = argv.length ? argv : ["HEAD~1"];
                    list[idx].label = "git diff " + formatArgv(list[idx].payload);
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
                try {
                    clipboardCopy(text);
                    output.print("Code review prompt copied to clipboard.");
                } catch (e) {
                    output.print("Clipboard error: " + (e && e.message ? e.message : e));
                }
            }
        },
        help: {description: "Show help", handler: help},
        export: {
            description: "Export context to shared interop storage",
            usage: "export [name]",
            handler: function (args) {
                try {
                    const items = tui.getState(STATE.contextItems) || [];
                    const sharedContext = {
                        CreatedAt: new Date().toISOString(),
                        UpdatedAt: new Date().toISOString(),
                        Version: "1.0",
                        SourceMode: "code_review",
                        ContextItems: items,
                        CodeReview: {
                            Template: typeof codeReviewTemplate !== 'undefined' ? codeReviewTemplate : ""
                        }
                    };
                    
                    if (args.length > 0) {
                        // Save with custom name
                        const customPath = args[0] + ".osm-interop.json";
                        interop.setCachePath(customPath);
                        interop.save(sharedContext);
                        output.print("Exported " + items.length + " context items to: " + customPath);
                        // Reset to default path
                        interop.setCachePath(".osm-interop.json");
                    } else {
                        interop.save(sharedContext);
                        output.print("Exported " + items.length + " context items to: " + interop.getCachePath());
                    }
                } catch (e) {
                    output.print("Export error: " + (e && e.message ? e.message : e));
                }
            }
        },
        import: {
            description: "Import context from shared interop storage",
            usage: "import [name]",
            handler: function (args) {
                try {
                    if (args.length > 0) {
                        // Load from custom name
                        const customPath = args[0] + ".osm-interop.json";
                        interop.setCachePath(customPath);
                    }
                    
                    if (!interop.exists()) {
                        output.print("No shared context found at: " + interop.getCachePath());
                        if (args.length > 0) {
                            // Reset to default path
                            interop.setCachePath(".osm-interop.json");
                        }
                        return;
                    }
                    
                    const sharedContext = interop.load();
                    const importedItems = sharedContext.ContextItems || [];
                    const currentItems = tui.getState(STATE.contextItems) || [];
                    
                    // Merge items, avoiding duplicates by checking labels
                    const existingLabels = new Set(currentItems.map(item => item.label));
                    let addedCount = 0;
                    
                    for (const item of importedItems) {
                        if (!existingLabels.has(item.label)) {
                            // Assign new ID to avoid conflicts
                            item.id = nextId(currentItems);
                            currentItems.push(item);
                            addedCount++;
                        }
                    }
                    
                    tui.setState(STATE.contextItems, currentItems);
                    output.print("Imported " + addedCount + " new items from: " + interop.getCachePath());
                    output.print("Source: " + (sharedContext.SourceMode || "unknown"));
                    
                    if (args.length > 0) {
                        // Reset to default path
                        interop.setCachePath(".osm-interop.json");
                    }
                } catch (e) {
                    output.print("Import error: " + (e && e.message ? e.message : e));
                }
            }
        },
        commit: {
            description: "Generate commit message from context",
            handler: function () {
                try {
                    const items = tui.getState(STATE.contextItems) || [];
                    if (items.length === 0) {
                        output.print("No context items to generate commit message from");
                        return;
                    }
                    
                    const commitData = interop.generateCommitMessage(items, {});
                    const conventionalCommit = commitData.type + ": " + commitData.subject;
                    
                    output.print("Generated commit message:");
                    output.print(conventionalCommit);
                    output.print("");
                    output.print(commitData.body);
                    
                    // Try to copy to clipboard
                    try {
                        clipboardCopy(conventionalCommit);
                        output.print("Commit message copied to clipboard!");
                    } catch (clipErr) {
                        output.print("Could not copy to clipboard: " + (clipErr && clipErr.message ? clipErr.message : clipErr));
                    }
                } catch (e) {
                    output.print("Commit generation error: " + (e && e.message ? e.message : e));
                }
            }
        },
    };
}

// Auto-switch into review mode when this script loads
ctx.run("enter-review", function () {
    tui.switchMode(STATE.mode);
});
