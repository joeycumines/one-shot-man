// Prompt Flow: Single-file, goal/context/template-driven prompt builder (baked-in version)
// This is the built-in version of the prompt-flow script with embedded template

const {buildContext, contextManager} = require('osm:ctxutil');
const nextIntegerId = require('osm:nextIntegerId');

// Fixed mode name
const MODE_NAME = "flow";

// Define state contract with Symbol keys
const StateKeys = tui.createStateContract(MODE_NAME, {
    phase: {
        description: MODE_NAME + ":phase",
        defaultValue: "INITIAL"
    },
    goal: {
        description: MODE_NAME + ":goal",
        defaultValue: ""
    },
    template: {
        description: MODE_NAME + ":template",
        defaultValue: null
    },
    metaPrompt: {
        description: MODE_NAME + ":metaPrompt",
        defaultValue: ""
    },
    taskPrompt: {
        description: MODE_NAME + ":taskPrompt",
        defaultValue: ""
    },
    contextItems: {
        description: MODE_NAME + ":contextItems",
        defaultValue: []
    }
});

// Expose limited state hooks for automated tests (no-op for regular users)
let __testStateAccessor = null;
if (typeof globalThis !== "undefined") {
    globalThis.__promptFlowTestHooks = {
        withState: function (callback) {
            if (typeof callback === "function" && __testStateAccessor) {
                callback(__testStateAccessor);
            }
        }
    };
}

// Expose addItem for test access - will be set after ctxmgr is created
let addItem;

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

function buildCommands(state) {
    __testStateAccessor = {state: state, StateKeys: StateKeys};

    // Phase helpers using the injected state accessor
    function getPhase() {
        return state.get(StateKeys.phase);
    }

    function setPhase(phase) {
        state.set(StateKeys.phase, phase);
    }

    function getGoal() {
        return state.get(StateKeys.goal);
    }

    function setGoal(v) {
        state.set(StateKeys.goal, v);
        if ((v || "").trim() && getPhase() === "INITIAL") {
            setPhase("CONTEXT_BUILDING");
        }
    }

    function getTemplate() {
        const template = state.get(StateKeys.template);
        // Check for null, undefined, and empty string - all should use default
        return (template !== null && template !== undefined && template !== "") ? template : defaultTemplate();
    }

    function setTemplate(v) {
        state.set(StateKeys.template, v);
    }

    function getMetaPrompt() {
        return state.get(StateKeys.metaPrompt);
    }

    function setMetaPrompt(v) {
        state.set(StateKeys.metaPrompt, v);
    }

    function getTaskPrompt() {
        return state.get(StateKeys.taskPrompt);
    }

    function setTaskPrompt(v) {
        state.set(StateKeys.taskPrompt, v);
    }

    function buildContextTxtar() {
        return buildContext(state.get(StateKeys.contextItems), {toTxtar: () => context.toTxtar()});
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

    // Create context manager with the injected state accessor
    const ctxmgr = contextManager({
        getItems: () => state.get(StateKeys.contextItems),
        setItems: (v) => state.set(StateKeys.contextItems, v),
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

    // Export for test access
    addItem = ctxmgr.addItem;

    // Use base commands from contextManager for common operations
    const baseCommands = ctxmgr.commands;

    return {
        ...baseCommands,
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
                if (state.get(StateKeys.template) !== null) output.print("[template] set");

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
        name: MODE_NAME,
        stateContract: StateKeys,
        tui: {
            title: "Prompt Flow",
            prompt: "(prompt-builder) > ",
            enableHistory: true,
            historyFile: ".prompt-flow_history"
        },
        onEnter: function (_, stateObj) {
            // Initialize template if null (lazy initialization pattern)
            if (stateObj.state.get(StateKeys.template) === null) {
                stateObj.state.set(StateKeys.template, defaultTemplate());
            }
            banner();
            help();
        },
        onExit: function (_, stateObj) {
            output.print("Exiting Prompt Flow.");
        },
        commands: buildCommands
    });

    tui.registerCommand({
        name: "flow",
        description: "Switch to Prompt Flow mode",
        handler: function () {
            tui.switchMode(MODE_NAME);
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

// Auto-switch into flow mode when this script loads
ctx.run("enter-flow", function () {
    tui.switchMode(MODE_NAME);
});
