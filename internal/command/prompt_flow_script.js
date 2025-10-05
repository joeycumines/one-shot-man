// Prompt Flow: Single-file, goal/context/template-driven prompt builder (baked-in version)
// This is the built-in version of the prompt-flow script with embedded template

const {buildContext, contextManager} = require('osm:ctxutil');
const nextIntegerId = require('osm:nextIntegerId');

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

// Phase helpers need to be defined early
function getPhase() {
    return tui.getState(STATE.phase) || "INITIAL";
}

function setPhase(phase) {
    tui.setState(STATE.phase, phase);
}

function items() {
    return tui.getState(STATE.contextItems) || [];
}

function setItems(v) {
    tui.setState(STATE.contextItems, v);
}

function getGoal() {
    return tui.getState(STATE.goal) || "";
}

function setGoal(v) {
    tui.setState(STATE.goal, v);
    if ((v || "").trim() && getPhase() === "INITIAL") {
        setPhase("CONTEXT_BUILDING");
    }
}

function getTemplate() {
    return tui.getState(STATE.template) || defaultTemplate();
}

function setTemplate(v) {
    tui.setState(STATE.template, v);
}

function getMetaPrompt() {
    return tui.getState(STATE.metaPrompt) || "";
}

function setMetaPrompt(v) {
    tui.setState(STATE.metaPrompt, v);
}

function getTaskPrompt() {
    return tui.getState(STATE.taskPrompt) || "";
}

function setTaskPrompt(v) {
    tui.setState(STATE.taskPrompt, v);
}

function buildContextTxtar() {
    return buildContext(items(), {toTxtar: () => context.toTxtar()});
}

function buildMetaPrompt() {
    const fullContext = buildContextTxtar();
    const pb = tui.createPromptBuilder("flow-meta", "Build meta-prompt");
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
    parts.push(buildContextTxtar());
    return parts.join("\n");
}

// Create context manager with prompt-flow specific configuration
// Note: we provide a custom buildPrompt that handles the phase-dependent logic
const ctxmgr = contextManager({
    getItems: items,
    setItems: setItems,
    nextIntegerId: nextIntegerId,
    buildPrompt: function() {
        // Phase-dependent prompt building
        if (getPhase() === 'TASK_PROMPT_SET') {
            return assembleFinal();
        } else {
            return getMetaPrompt();
        }
    }
});

// Expose addItem for test access
const {addItem} = ctxmgr;

function buildCommands() {
    // Use base commands from contextManager for common operations
    const baseCommands = ctxmgr.commands;

    return {
        goal: {
            description: "Set or edit the goal",
            usage: "goal [text|--prewritten]",
            handler: function (args) {
                if (args.length === 1 && args[0] === "--prewritten") {
                    // Show available pre-written goals
                    output.print("Available pre-written goals:");
                    output.print("  comment-stripper   - Remove useless comments and refactor useful ones");
                    output.print("  doc-generator      - Generate comprehensive documentation for code");
                    output.print("  test-generator     - Generate comprehensive test suites");
                    output.print("");
                    output.print("Usage: goal use:<goal-name> to use a pre-written goal");
                    return;
                }
                if (args.length === 1 && args[0].startsWith("use:")) {
                    const goalName = args[0].substring(4);
                    const prewrittenGoals = {
                        "comment-stripper": "Analyze the provided code and remove useless comments while refactoring useful ones according to coding best practices. Remove redundant comments that merely repeat what the code does, but preserve valuable business logic explanations, performance considerations, and complex algorithm documentation. Ensure the cleaned code maintains all functionality while improving readability.",
                        "doc-generator": "Generate comprehensive documentation for the provided code including API documentation, usage examples, configuration guides, and developer notes. Create clear, well-structured documentation that helps users understand and effectively use the code.",
                        "test-generator": "Generate comprehensive test suites for the provided code including unit tests, integration tests, and edge case coverage. Create thorough tests that verify functionality, handle error conditions, and provide good code coverage while following testing best practices."
                    };

                    if (prewrittenGoals[goalName]) {
                        setGoal(prewrittenGoals[goalName]);
                        output.print("Pre-written goal '" + goalName + "' set successfully.");
                        return;
                    } else {
                        output.print("Unknown pre-written goal: " + goalName);
                        return;
                    }
                }
                const goalText = args.join(" ");
                if (!goalText) {
                    // No arguments: open editor to edit current goal
                    const currentGoal = getGoal();
                    const newGoal = ctxmgr.openEditor("goal", currentGoal);
                    if (newGoal && newGoal !== currentGoal) {
                        setGoal(newGoal);
                        output.print("Goal updated.");
                    }
                    return;
                }
                setGoal(goalText);
                output.print("Goal set.");
            }
        },
        template: {
            description: "Set or edit the meta-prompt template",
            usage: "template <edit>",
            handler: function (args) {
                if (args.length === 0 || args[0] === "edit") {
                    const currentTemplate = getTemplate();
                    const newTemplate = ctxmgr.openEditor("template", currentTemplate);
                    if (newTemplate && newTemplate !== currentTemplate) {
                        setTemplate(newTemplate);
                        output.print("Template updated.");
                    }
                } else {
                    output.print("Usage: template [edit]");
                }
            }
        },
        generate: {
            description: "Generate the meta-prompt (phase: CONTEXT_BUILDING -> META_GENERATED)",
            usage: "generate",
            handler: function () {
                if (!getGoal().trim()) {
                    output.print("Error: Please set a goal first using the 'goal' command.");
                    return;
                }
                setPhase("CONTEXT_BUILDING");
                const metaPrompt = buildMetaPrompt();
                setMetaPrompt(metaPrompt);
                setTaskPrompt("");
                setPhase("META_GENERATED");
                output.print("Meta-prompt generated. You can 'show meta', 'copy meta', or provide the task prompt with 'use'.");
            }
        },
        use: {
            description: "Set the task prompt (phase: META_GENERATED -> TASK_PROMPT_SET)",
            usage: "use [text...]",
            handler: function (args) {
                const phase = getPhase();
                if (phase !== "META_GENERATED" && phase !== "TASK_PROMPT_SET") {
                    output.print("Please generate the meta-prompt first using 'generate'.");
                    return;
                }
                let prompt;
                if (args.length === 0) {
                    // No arguments: open editor to edit/set task prompt
                    const currentPrompt = getTaskPrompt();
                    prompt = ctxmgr.openEditor("task-prompt", currentPrompt);
                } else {
                    prompt = args.join(" ");
                }
                const trimmed = (prompt || "").trim();
                if (!trimmed) {
                    output.print("Task prompt not set (no content provided).");
                    return;
                }
                setTaskPrompt(trimmed);
                setPhase("TASK_PROMPT_SET");
                output.print("Task prompt set. Use 'show' or 'copy' to get the final prompt.");
            }
        },
        list: {
            ...baseCommands.list,
            handler: function (args) {
                // Show phase-specific information
                output.print("Phase: " + getPhase());
                const g = getGoal();
                if (g) output.print("[goal] " + g);
                if (getTemplate()) output.print("[template] set");

                const phase = getPhase();
                if (phase === "META_GENERATED" || phase === "TASK_PROMPT_SET") {
                    const mp = getMetaPrompt();
                    if (mp) output.print("[meta] " + mp.substring(0, 80) + (mp.length > 80 ? "..." : ""));
                }
                if (phase === "TASK_PROMPT_SET") {
                    const tp = getTaskPrompt();
                    if (tp) output.print("[prompt] " + tp.substring(0, 80) + (tp.length > 80 ? "..." : ""));
                }

                output.print("");
                // Delegate to base list command for context items
                baseCommands.list.handler(args);
            }
        },
        edit: {
            ...baseCommands.edit,
            usage: "edit <goal|template|meta|prompt|id>",
            handler: function (args) {
                if (args.length === 0) {
                    output.print("Usage: edit <goal|template|meta|prompt|id>");
                    return;
                }

                const target = args[0];

                // Handle special edit targets
                if (target === "goal") {
                    const currentGoal = getGoal();
                    const newGoal = ctxmgr.openEditor("goal", currentGoal);
                    if (newGoal && newGoal !== currentGoal) {
                        setGoal(newGoal);
                        output.print("Goal updated.");
                    }
                    return;
                }

                if (target === "template") {
                    const currentTemplate = getTemplate();
                    const newTemplate = ctxmgr.openEditor("template", currentTemplate);
                    if (newTemplate && newTemplate !== currentTemplate) {
                        setTemplate(newTemplate);
                        output.print("Template updated.");
                    }
                    return;
                }

                if (target === "meta") {
                    if (getPhase() === "INITIAL" || getPhase() === "CONTEXT_BUILDING") {
                        output.print("Error: Meta-prompt not generated yet. Use 'generate' first.");
                        return;
                    }
                    const currentMeta = getMetaPrompt();
                    const newMeta = ctxmgr.openEditor("meta-prompt", currentMeta);
                    if (newMeta && newMeta !== currentMeta) {
                        setMetaPrompt(newMeta);
                        output.print("Meta-prompt updated.");
                    }
                    return;
                }

                if (target === "prompt") {
                    if (getPhase() !== "TASK_PROMPT_SET") {
                        output.print("Error: Task prompt not set yet. Use 'use' first.");
                        return;
                    }
                    const currentPrompt = getTaskPrompt();
                    const newPrompt = ctxmgr.openEditor("task-prompt", currentPrompt);
                    const trimmed = (newPrompt || "").trim();
                    if (!trimmed) {
                        output.print("Task prompt not updated (no content provided).");
                        return;
                    }
                    if (trimmed !== currentPrompt) {
                        setTaskPrompt(trimmed);
                        output.print("Task prompt updated.");
                    }
                    return;
                }

                // Delegate to base edit command for numeric IDs
                baseCommands.edit.handler(args);
            }
        },
        // Delegate all other base commands
        add: baseCommands.add,
        diff: baseCommands.diff,
        note: baseCommands.note,
        remove: baseCommands.remove,
        show: {
            description: "Show meta, task prompt, or final output",
            usage: "show [meta|prompt]",
            handler: function (args) {
                const target = args[0] || "";
                if (target === 'meta') {
                    output.print(getMetaPrompt());
                    return;
                }
                if (target === 'prompt') {
                    output.print(getTaskPrompt());
                    return;
                }
                baseCommands.show.handler([]);
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
                    ctxmgr.clipboardCopy(text);
                    output.print(label + " copied to clipboard.");
                } catch (e) {
                    output.print("Clipboard error: " + (e && e.message ? e.message : e));
                }
            }
        },
        help: {
            description: "Show this help message",
            usage: "help",
            handler: function () {
                help();
            }
        }
    };
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
    output.print("Prompt Flow: goal/context/template -> generate -> use -> assemble");
    output.print("Type 'help' for commands. Use 'flow' to return here later.");
}

function help() {
    output.print("Commands: goal, add, diff, note, list, edit, remove, template, generate, use, show [meta|prompt], copy [meta|prompt], help, exit");
    output.print("Tip: Use 'goal --prewritten' to see available pre-written goals");
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

// Auto-switch into flow mode when this script loads
ctx.run("enter-flow", function () {
    tui.switchMode(STATE.mode);
});
