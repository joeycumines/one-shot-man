// Convert-to-Agent-Prompt: Interactive converter from goal/context to structured agentic AI prompt

const STATE = {
    mode: "convert-to-agent-prompt",
    phase: "convertToAgentPromptPhase",
    goal: "convertToAgentPromptGoal",
    contextItems: "convertToAgentPromptContextItems",
    template: "convertToAgentPromptTemplate",
    metaPrompt: "convertToAgentPromptMetaPrompt",
    agentPrompt: "convertToAgentPromptAgentPrompt"
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
            title: "Convert to Agent Prompt",
            prompt: "(agent-prompt-converter) > ",
            enableHistory: true,
            historyFile: ".convert-to-agent-prompt_history"
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
            output.print("Exiting Convert to Agent Prompt mode.");
        },
        commands: buildCommands()
    });

    tui.registerCommand({
        name: "convert-to-agent-prompt",
        description: "Convert goal/context to structured agentic AI prompt",
        handler: function () {
            tui.switchMode(STATE.mode);
        }
    });
});

function banner() {
    output.print("Convert to Agent Prompt: goal/context -> structured agent prompt");
    output.print("Type 'help' for commands. Use 'convert-to-agent-prompt' to return here later.");
}

function help() {
    output.print("Commands: goal, add, diff, note, list, edit, remove, template, generate, show, copy, help, exit");
}

function defaultTemplate() {
    return convertToAgentPromptTemplate;
}

function setPhase(p) {
    tui.setState(STATE.phase, p);
}

function getPhase() {
    return tui.getState(STATE.phase) || "INITIAL";
}

function setGoal(text) {
    tui.setState(STATE.goal, text);
}

function getGoal() {
    return tui.getState(STATE.goal) || "";
}

function getTemplate() {
    return tui.getState(STATE.template) || defaultTemplate();
}

function setTemplate(t) {
    tui.setState(STATE.template, t);
}

function setMetaPrompt(p) {
    tui.setState(STATE.metaPrompt, p);
}

function getMetaPrompt() {
    return tui.getState(STATE.metaPrompt) || "";
}

function setAgentPrompt(p) {
    tui.setState(STATE.agentPrompt, p);
}

function getAgentPrompt() {
    return tui.getState(STATE.agentPrompt) || "";
}

function items() {
    return tui.getState(STATE.contextItems) || [];
}

function setItems(v) {
    tui.setState(STATE.contextItems, v);
}

function addItem(type, label, payload) {
    const list = items();
    const item = {
        id: nextId(list),
        type: type,
        label: label,
        payload: payload
    };
    list.push(item);
    setItems(list);
    return item;
}

// Build the combined context string from notes, diffs (always refreshed), and tracked files (txtar)
function buildContextString() {
    const list = items();
    const parts = [];

    for (const item of list) {
        if (item.type === "note") {
            parts.push("=== NOTE: " + item.label + " ===");
            parts.push(item.payload);
            parts.push("");
        } else if (item.type === "lazy-diff") {
            parts.push("=== DIFF: " + item.label + " ===");
            try {
                const result = system.execv(["git", "diff"].concat(item.payload || []));
                if (result.error) {
                    parts.push("ERROR: " + result.message);
                } else {
                    parts.push(result.stdout || "(no diff output)");
                }
            } catch (e) {
                parts.push("ERROR: " + e);
            }
            parts.push("");
        } else if (item.type === "file") {
            // Get file content from context
            try {
                const err = context.addPath(item.label);
                if (err && err.message) {
                    parts.push("=== FILE ERROR: " + item.label + " ===");
                    parts.push(err.message);
                    parts.push("");
                }
            } catch (e) {
                parts.push("=== FILE ERROR: " + item.label + " ===");
                parts.push("" + e);
                parts.push("");
            }
        }
    }

    // Get the txtar context for files
    const contextStr = context.buildContext();
    if (contextStr && contextStr.trim()) {
        parts.push(contextStr);
    }

    return parts.join("\n");
}

function buildMetaPrompt() {
    const fullContext = buildContextString();

    const pb = tui.createPromptBuilder("meta", "Build agent prompt generator");
    pb.setTemplate(getTemplate());
    pb.setVariable("goal", getGoal());
    pb.setVariable("context_txtar", fullContext);
    return pb.build();
}

function assembleFinal() {
    const agentPrompt = getAgentPrompt();
    if (agentPrompt && agentPrompt.trim()) {
        const parts = [];
        parts.push(agentPrompt.trim());
        
        const context = buildContextString();
        if (context && context.trim()) {
            parts.push("\n---\n## CONTEXT/IMPLEMENTATIONS\n---\n");
            parts.push(context);
        }
        
        return parts.join("\n");
    } else {
        return buildMetaPrompt();
    }
}

function openEditor(title, initial) {
    const res = osOpenEditor(title, initial || "");
    if (typeof res === 'string') return res;
    // Some engines may return [value, error]; we standardize to string
    return "" + res;
}

function formatArgv(argv) {
    return argv.map(s => s.includes(" ") ? '"' + s + '"' : s).join(" ");
}

function buildCommands() {
    return {
        goal: {
            description: "Set or edit the goal/description",
            usage: "goal [text]",
            handler: function (args) {
                if (args.length === 0) {
                    const edited = openEditor("goal", getGoal());
                    if (edited && edited.trim()) setGoal(edited.trim());
                } else {
                    setGoal(args.join(" "));
                }
                output.print("Goal set.");
            }
        },
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
            description: "Add git diff (lazy; refreshed on generate/show)",
            usage: "diff [args]",
            handler: function (args) {
                const argv = (args && args.length > 0) ? args.slice(0) : ["HEAD~1"];
                const label = "git diff " + (argv.length ? formatArgv(argv) : "");
                addItem("lazy-diff", label, argv);
                output.print("Added diff: " + label + " (will be executed when generating output)");
            }
        },
        note: {
            description: "Add a freeform note",
            usage: "note [text]",
            handler: function (args) {
                let text;
                if (args.length === 0) {
                    text = openEditor("note", "");
                } else {
                    text = args.join(" ");
                }
                if (text && text.trim()) {
                    const item = addItem("note", "Note #" + (items().length + 1), text.trim());
                    output.print("Added note: " + item.label);
                } else {
                    output.print("No note added (empty)");
                }
            }
        },
        list: {
            description: "List goal, template, prompts, and context items",
            usage: "list",
            handler: function () {
                output.print("=== GOAL ===");
                const goal = getGoal();
                output.print(goal || "(not set)");

                output.print("\n=== TEMPLATE ===");
                const template = getTemplate();
                output.print(template.slice(0, 200) + (template.length > 200 ? "..." : ""));

                const metaPrompt = getMetaPrompt();
                if (metaPrompt) {
                    output.print("\n=== META PROMPT ===");
                    output.print(metaPrompt.slice(0, 200) + (metaPrompt.length > 200 ? "..." : ""));
                }

                const agentPrompt = getAgentPrompt();
                if (agentPrompt) {
                    output.print("\n=== AGENT PROMPT ===");
                    output.print(agentPrompt.slice(0, 200) + (agentPrompt.length > 200 ? "..." : ""));
                }

                output.print("\n=== CONTEXT ITEMS ===");
                const list = items();
                if (list.length === 0) {
                    output.print("(none)");
                } else {
                    for (const item of list) {
                        output.print("[" + item.id + "] " + item.type + ": " + item.label);
                    }
                }
            }
        },
        edit: {
            description: "Edit items by ID or name",
            usage: "edit <id|goal|template|meta|agent>",
            handler: function (args) {
                if (args.length === 0) {
                    output.print("Usage: edit <id|goal|template|meta|agent>");
                    return;
                }
                const target = args[0];
                if (target === "goal") {
                    const edited = openEditor("goal", getGoal());
                    if (edited !== null) setGoal(edited.trim());
                } else if (target === "template") {
                    const edited = openEditor("template", getTemplate());
                    if (edited !== null) setTemplate(edited);
                } else if (target === "meta") {
                    const edited = openEditor("meta-prompt", getMetaPrompt());
                    if (edited !== null) setMetaPrompt(edited);
                } else if (target === "agent") {
                    const edited = openEditor("agent-prompt", getAgentPrompt());
                    if (edited !== null) {
                        setAgentPrompt(edited.trim());
                        if (!edited.trim()) {
                            output.print("Agent prompt cleared. Reverted to meta-prompt phase.");
                        }
                    }
                } else {
                    const id = parseInt(target, 10);
                    if (isNaN(id)) {
                        output.print("Invalid target: " + target);
                        return;
                    }
                    const list = items();
                    const item = list.find(x => x.id === id);
                    if (!item) {
                        output.print("Item not found: " + id);
                        return;
                    }
                    if (item.type === "note") {
                        const edited = openEditor("note", item.payload);
                        if (edited !== null) item.payload = edited.trim();
                    } else if (item.type === "lazy-diff") {
                        const current = item.payload ? formatArgv(item.payload) : "";
                        const edited = openEditor("diff args", current);
                        if (edited !== null) {
                            const newArgs = edited.trim().split(/\s+/).filter(s => s);
                            item.payload = newArgs.length ? newArgs : ["HEAD~1"];
                            item.label = "git diff " + formatArgv(item.payload);
                        }
                    } else {
                        output.print("Cannot edit " + item.type + " items");
                    }
                    setItems(list);
                }
                output.print("Edit complete.");
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
                const id = parseInt(args[0], 10);
                if (isNaN(id)) {
                    output.print("Invalid ID: " + args[0]);
                    return;
                }
                const list = items();
                const index = list.findIndex(x => x.id === id);
                if (index === -1) {
                    output.print("Item not found: " + id);
                    return;
                }
                const item = list[index];
                if (item.type === "file") {
                    try {
                        context.removePath(item.label);
                    } catch (e) {
                        // Ignore errors - file might not be tracked
                    }
                }
                list.splice(index, 1);
                setItems(list);
                output.print("Removed: " + item.label);
            }
        },
        template: {
            description: "Edit the agent prompt template",
            usage: "template",
            handler: function () {
                const edited = openEditor("template", getTemplate());
                if (edited !== null) {
                    setTemplate(edited);
                    output.print("Template updated.");
                }
            }
        },
        generate: {
            description: "Generate the meta-prompt and reset agent prompt",
            usage: "generate",
            handler: function () {
                const metaPrompt = buildMetaPrompt();
                setMetaPrompt(metaPrompt);
                setAgentPrompt(""); // Reset agent prompt
                output.print("Generated meta-prompt. Use 'show' to view or 'use' to set agent prompt.");
            }
        },
        use: {
            description: "Set or edit the agent prompt",
            usage: "use [text]",
            handler: function (args) {
                if (args.length === 0) {
                    const edited = openEditor("agent-prompt", getAgentPrompt());
                    if (edited !== null) {
                        setAgentPrompt(edited.trim());
                        if (!edited.trim()) {
                            output.print("Agent prompt cleared. Reverted to meta-prompt phase.");
                        } else {
                            output.print("Agent prompt set.");
                        }
                    }
                } else {
                    setAgentPrompt(args.join(" "));
                    output.print("Agent prompt set.");
                }
            }
        },
        show: {
            description: "Show content",
            usage: "show [meta|agent]",
            handler: function (args) {
                const what = args.length > 0 ? args[0] : "default";
                if (what === "meta") {
                    output.print(getMetaPrompt() || buildMetaPrompt());
                } else if (what === "agent") {
                    const agentPrompt = getAgentPrompt();
                    if (agentPrompt) {
                        output.print(agentPrompt);
                    } else {
                        output.print("Agent prompt not set. Use 'generate' first, then 'use' to set it.");
                    }
                } else {
                    // Default behavior: show final assembled output
                    output.print(assembleFinal());
                }
            }
        },
        copy: {
            description: "Copy content to clipboard",
            usage: "copy [meta|agent]",
            handler: function (args) {
                const what = args.length > 0 ? args[0] : "default";
                let content;
                if (what === "meta") {
                    content = getMetaPrompt() || buildMetaPrompt();
                } else if (what === "agent") {
                    content = getAgentPrompt();
                    if (!content) {
                        output.print("Agent prompt not set. Use 'generate' first, then 'use' to set it.");
                        return;
                    }
                } else {
                    // Default behavior: copy final assembled output
                    content = assembleFinal();
                }
                try {
                    system.clipboardCopy(content);
                    output.print("Copied to clipboard.");
                } catch (e) {
                    output.print("Clipboard error: " + (e && e.message ? e.message : e));
                }
            }
        },
        help: {description: "Show help", handler: help},
    };
}

// Auto-switch into converter mode when this script loads
ctx.run("enter-convert-mode", function () {
    tui.switchMode(STATE.mode);
});