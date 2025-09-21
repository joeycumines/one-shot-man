// Prompt Flow: Single-file, goal/context/template-driven prompt builder (baked-in version)
// This is the built-in version of the prompt-flow script with embedded template

// State keys
const STATE = {
    mode: "flow",               // fixed mode name
    phase: "phase",             // INITIAL | CONTEXT_BUILDING | GENERATED
    goal: "goal",               // string
    template: "template",       // string (meta-prompt template)
    prompt: "prompt",           // string (generated main prompt)
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
            title: "Prompt Flow",
            prompt: "(prompt-builder) > ",
            enableHistory: true,
            historyFile: ".prompt-flow_history"
        },
        onEnter: function () {
            if (!tui.getState(STATE.phase)) {
                tui.setState(STATE.phase, "INITIAL");
                tui.setState(STATE.contextItems, []);
                tui.setState(STATE.template, defaultTemplate());
            }
            banner();
            help();
        },
        onExit: function () {
            output.print("Exiting Prompt Flow.");
        },
        commands: buildCommands()
    });

    tui.registerCommand({
        name: "flow",
        description: "Switch to Prompt Flow mode",
        handler: function () {
            tui.switchMode(STATE.mode);
        }
    });
});

function banner() {
    output.print("Prompt Flow: goal/context/template -> generate -> assemble");
    output.print("Type 'help' for commands. Use 'flow' to return here later.");
}

function help() {
    output.print("Commands: goal, add, diff, note, list, edit, remove, template, generate, show [meta], copy [meta], help, exit");
}

function defaultTemplate() {
    // Use the embedded template content passed from the Go command
    // This ensures we use the single source of truth from prompt_flow_template.md
    return promptFlowTemplate || `!! N.B. only statements surrounded by "!!" are _instructions_. !!

!! Generate a prompt using the template for purposes of achieving the following goal. !!

!! **GOAL:** !!
{{goal}}

!! **IMPLEMENTATIONS/CONTEXT:** !!
{{context_txtar}}`;
}

function setPhase(p) {
    tui.setState(STATE.phase, p);
}

function getPhase() {
    return tui.getState(STATE.phase) || "INITIAL";
}

function setGoal(text) {
    tui.setState(STATE.goal, text);
    if (getPhase() === "INITIAL") setPhase("CONTEXT_BUILDING");
}

function getGoal() {
    return tui.getState(STATE.goal) || "";
}

function getTemplate() {
    return tui.getState(STATE.template) || "";
}

function setTemplate(t) {
    tui.setState(STATE.template, t);
}

function setPrompt(p) {
    tui.setState(STATE.prompt, p);
}

function getPrompt() {
    return tui.getState(STATE.prompt) || "";
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

function buildMetaPrompt() {
    // Leverage context manager txtar dump
    const txtar = context.toTxtar();
    const pb = tui.createPromptBuilder("meta", "Build meta-prompt");
    pb.setTemplate(getTemplate());
    pb.setVariable("goal", getGoal());
    pb.setVariable("context_txtar", txtar);
    return pb.build();
}

function assembleFinal() {
    const parts = [];
    const p = getPrompt();
    if (p) parts.push(p.trim());
    parts.push("\n---\n## IMPLEMENTATIONS/CONTEXT\n---\n");
    // Emit notes and diffs
    for (const it of items()) {
        if (it.type === "note") {
            parts.push("### Note: " + (it.label || "note") + "\n\n" + it.payload + "\n\n---\n");
        } else if (it.type === "diff") {
            parts.push("### Diff: " + (it.label || "git diff") + "\n\n```diff\n" + (it.payload || "") + "\n```\n\n---\n");
        }
    }
    // Also append the txtar dump for tracked files
    parts.push("```\n" + context.toTxtar() + "\n```");
    return parts.join("\n");
}

function openEditor(title, initial) {
    const res = system.openEditor(title, initial || "");
    if (typeof res === 'string') return res;
    // Some engines may return [value, error]; we standardize to string
    return "" + res;
}

function buildCommands() {
    return {
        goal: {
            description: "Set or edit the goal",
            usage: "goal [text]",
            handler: function (args) {
                if (args.length === 0) {
                    const edited = openEditor("goal", getGoal());
                    setGoal(edited.trim());
                } else {
                    setGoal(args.join(" "));
                }
                output.print("Goal set.");
            }
        },
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
            description: "Add git diff output to context",
            usage: "diff [args]",
            handler: function (args) {
                const argv = ["git", "diff"].concat(args || []);
                const res = system.execv(argv);
                if (res && res.error) {
                    output.print("git diff failed: " + res.message);
                    return;
                }
                const label = "git diff " + (args || []).join(" ");
                addItem("diff", label, res.stdout);
                output.print("Added diff: " + label);
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
            description: "List goal, template, prompt, and items",
            handler: function () {
                if (getGoal()) output.print("[goal] " + getGoal());
                if (getTemplate()) output.print("[template] set");
                if (getPrompt()) output.print("[prompt] " + getPrompt().slice(0, 80) + (getPrompt().length > 80 ? "..." : ""));
                for (const it of items()) {
                    output.print("[" + it.id + "] [" + it.type + "] " + (it.label || ""));
                }
            }
        },
        edit: {
            description: "Edit goal/template/prompt by id or name",
            usage: "edit <id|goal|template|prompt>",
            handler: function (args) {
                if (args.length < 1) {
                    output.print("Usage: edit <id|goal|template|prompt>");
                    return;
                }
                const target = args[0];
                if (target === 'goal') {
                    setGoal(openEditor('goal', getGoal()));
                    return;
                }
                if (target === 'template') {
                    setTemplate(openEditor('template', getTemplate()));
                    return;
                }
                if (target === 'prompt') {
                    setPrompt(openEditor('prompt', getPrompt()));
                    return;
                }
                const id = parseInt(target, 10);
                if (isNaN(id)) {
                    output.print("Invalid id: " + target);
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
        template: {
            description: "Edit the meta-prompt template",
            handler: function () {
                setTemplate(openEditor("template", getTemplate()));
            }
        },
        generate: {
            description: "Generate the main prompt using the meta-prompt",
            handler: function () {
                output.print("[flow] generate: start");
                const meta = buildMetaPrompt();
                output.print("[flow] generate: built meta");
                // For now, simple: take meta as the prompt seed. In a real flow, send to LLM.
                const edited = openEditor("generated-prompt", meta);
                output.print("[flow] generate: editor returned");
                setPrompt(edited);
                setPhase("GENERATED");
                output.print("Generated. You can now 'show' or 'copy'.");
            }
        },
        show: {
            description: "Show meta or final output",
            usage: "show [meta]",
            handler: function (args) {
                if ((args[0] || "") === 'meta' || getPhase() !== 'GENERATED') {
                    output.print(buildMetaPrompt());
                } else {
                    output.print(assembleFinal());
                }
            }
        },
        copy: {
            description: "Copy meta or final output to clipboard",
            usage: "copy [meta]",
            handler: function (args) {
                const meta = (args[0] || "") === 'meta';
                const text = meta ? buildMetaPrompt() : assembleFinal();
                const err = system.clipboardCopy(text);
                if (err && err.message) {
                    output.print("Clipboard error: " + err.message);
                } else {
                    output.print((meta ? "Meta" : "Final") + " output copied to clipboard.");
                }
            }
        },
        help: {description: "Show help", handler: help},
    };
}

// Auto-switch into flow mode when this script loads
ctx.run("enter-flow", function () {
    tui.switchMode(STATE.mode);
});
