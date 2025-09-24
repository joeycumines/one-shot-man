// Prompt Flow: Single-file, goal/context/template-driven prompt builder (baked-in version)
// This is the built-in version of the prompt-flow script with embedded template

const {openEditor: osOpenEditor, fileExists, clipboardCopy} = require('osm:os');
const {formatArgv} = require('osm:argv');
const {buildContext} = require('osm:ctxutil');

// State keys
const STATE = {
    mode: "flow",               // fixed mode name
    phase: "phase",             // INITIAL | CONTEXT_BUILDING | META_GENERATED | TASK_PROMPT_SET
    goal: "goal",               // string
    template: "template",       // string (meta-prompt template)
    metaPrompt: "metaPrompt",   // string (generated meta-prompt)
    taskPrompt: "taskPrompt",   // string (the prompt to perform the task)
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
    // Get configuration values, providing sensible defaults
    const config = (typeof scriptConfig !== 'undefined') ? scriptConfig : {};
    const title = config["ui.title"] || "Prompt Flow";
    const prompt = config["ui.prompt"] || "(prompt-builder) > ";
    const historyFile = config["ui.history-file"] || ".prompt-flow_history";
    const enableHistory = config["ui.enable-history"] !== "false"; // defaults to true
    
    tui.registerMode({
        name: STATE.mode,
        tui: {
            title: title,
            prompt: prompt,
            enableHistory: enableHistory,
            historyFile: historyFile
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
    const config = (typeof scriptConfig !== 'undefined') ? scriptConfig : {};
    const bannerText = config["ui.banner"] || "Prompt Flow: goal/context/template -> generate -> use -> assemble";
    const showHelp = config["ui.show-help-on-start"] !== "false"; // defaults to true
    
    output.print(bannerText);
    output.print("Type 'help' for commands. Use 'flow' to return here later.");
}

function help() {
    const config = (typeof scriptConfig !== 'undefined') ? scriptConfig : {};
    const helpText = config["ui.help-text"] || "Commands: goal, add, diff, note, list, edit, remove, template, generate, use, show [meta|prompt], copy [meta|prompt], help, exit";
    output.print(helpText);
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

function setMetaPrompt(p) {
    tui.setState(STATE.metaPrompt, p);
}

function getMetaPrompt() {
    return tui.getState(STATE.metaPrompt) || "";
}

function setTaskPrompt(p) {
    tui.setState(STATE.taskPrompt, p);
}

function getTaskPrompt() {
    return tui.getState(STATE.taskPrompt) || "";
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

// Build the combined context string from notes, diffs (always refreshed), and tracked files (txtar)
function buildContextString() {
    return buildContext(items(), {toTxtar: () => context.toTxtar()});
}

function buildMetaPrompt() {
    const fullContext = buildContextString();

    const pb = tui.createPromptBuilder("meta", "Build meta-prompt");
    pb.setTemplate(getTemplate());
    pb.setVariable("goal", getGoal());
    pb.setVariable("context_txtar", fullContext);
    return pb.build();
}

function assembleFinal() {
    const parts = [];
    const p = getTaskPrompt();
    if (p) parts.push(p.trim());
    parts.push("\n---\n## IMPLEMENTATIONS/CONTEXT\n---\n");
    parts.push(buildContextString());
    return parts.join("\n");
}

function openEditor(title, initial) {
    const res = osOpenEditor(title, initial || "");
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
                const argv = (args && args.length > 0) ? args.slice(0) : ["HEAD~1"]; // default like code-review
                const label = "git diff " + (argv.length ? formatArgv(argv) : "");
                addItem("lazy-diff", label, argv);
                output.print("Added diff: " + label + " (will be executed when generating output)");
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
            description: "List goal, template, prompts, and items",
            handler: function () {
                if (getGoal()) output.print("[goal] " + getGoal());
                if (getTemplate()) output.print("[template] set");
                const meta = getMetaPrompt();
                if (meta) output.print("[meta] " + meta.slice(0, 80) + (meta.length > 80 ? "..." : ""));
                const task = getTaskPrompt();
                if (task) output.print("[prompt] " + task.slice(0, 80) + (task.length > 80 ? "..." : ""));
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
            description: "Edit goal/template/meta/prompt by id or name",
            usage: "edit <id|goal|template|meta|prompt>",
            handler: function (args) {
                if (args.length < 1) {
                    output.print("Usage: " + this.usage);
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
                if (target === 'meta') {
                    if (getPhase() === "INITIAL" || getPhase() === "CONTEXT_BUILDING") {
                        output.print("Meta prompt not generated yet. Use 'generate' first.");
                        return;
                    }
                    setMetaPrompt(openEditor('meta-prompt', getMetaPrompt()));
                    return;
                }
                if (target === 'prompt') {
                    if (getPhase() !== "TASK_PROMPT_SET") {
                        output.print("Task prompt not set yet. Use 'use' first.");
                        return;
                    }
                    const edited = openEditor('task-prompt', getTaskPrompt());
                    if (edited && edited.trim()) {
                        setTaskPrompt(edited.trim());
                        setPhase("TASK_PROMPT_SET");
                        output.print("Task prompt updated.");
                    } else {
                        // If cleared, revert to meta-generated phase to restore sensible defaults
                        setTaskPrompt("");
                        setPhase("META_GENERATED");
                        output.print("Task prompt cleared. Reverted to meta-prompt phase.");
                    }
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
            description: "Generate the meta-prompt and reset the task prompt",
            handler: function () {
                output.print("Generating meta-prompt...");
                const meta = buildMetaPrompt();
                setMetaPrompt(meta);
                // Wipe out the old task prompt and reset phase
                setTaskPrompt("");
                setPhase("META_GENERATED");
                output.print("Meta-prompt generated. You can 'show meta', 'copy meta', or provide the task prompt with 'use'.");
            }
        },
        use: {
            description: "Set or edit the task prompt",
            usage: "use [text]",
            handler: function (args) {
                if (getPhase() !== "META_GENERATED" && getPhase() !== "TASK_PROMPT_SET") {
                    output.print("Please generate the meta-prompt first using 'generate'.");
                    return;
                }
                let text;
                if (args.length === 0) {
                    text = openEditor("task-prompt", getTaskPrompt());
                } else {
                    text = args.join(" ");
                }

                if (text && text.trim()) {
                    setTaskPrompt(text.trim());
                    setPhase("TASK_PROMPT_SET");
                    output.print("Task prompt set. You can now 'show' or 'copy' the final output.");
                } else {
                    output.print("Task prompt not set (no content provided).");
                }
            }
        },
        show: {
            description: "Show meta, task prompt, or final output",
            usage: "show [meta|prompt]",
            handler: function (args) {
                const target = args[0] || "";
                if (target === 'meta') {
                    output.print(getMetaPrompt());
                } else if (target === 'prompt') {
                    output.print(getTaskPrompt());
                } else {
                    // Default behavior
                    if (getPhase() === 'TASK_PROMPT_SET') {
                        output.print(assembleFinal());
                    } else {
                        output.print(getMetaPrompt());
                    }
                }
            }
        },
        copy: {
            description: "Copy meta, task prompt, or final output to clipboard",
            usage: "copy [meta|prompt]",
            handler: function (args) {
                const target = args[0] || "";
                let text;
                let label;

                if (target === 'meta') {
                    text = getMetaPrompt();
                    label = "Meta prompt";
                } else if (target === 'prompt') {
                    text = getTaskPrompt();
                    label = "Task prompt";
                } else {
                    // Default behavior
                    if (getPhase() === 'TASK_PROMPT_SET') {
                        text = assembleFinal();
                        label = "Final output";
                    } else {
                        text = getMetaPrompt();
                        label = "Meta prompt";
                    }
                }

                try {
                    clipboardCopy(text);
                    output.print(label + " copied to clipboard.");
                } catch (e) {
                    output.print("Clipboard error: " + (e && e.message ? e.message : e));
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
