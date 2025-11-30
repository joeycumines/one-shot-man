// Code Review: Single-prompt code review with context (baked-in version)
// This is the built-in version of the code-review script with embedded template

const nextIntegerId = require('osm:nextIntegerId');
const {buildContext, contextManager} = require('osm:ctxutil');
const template = require('osm:text/template');

// Import shared symbols
const shared = require('osm:sharedStateSymbols');

// config.name is injected by Go as "code-review"
const COMMAND_NAME = config.name;
// The mode exposed to the TUI is a short name users can switch to.
// Historically the command is called "code-review" while the single
// mode it exposes is called "review". Keep that separation so tests
// and user expectations remain stable.
const MODE_NAME = "review";

// Create the single state accessor with only shared contextItems
const state = tui.createState(COMMAND_NAME, {
    [shared.contextItems]: {defaultValue: []}
});

// Globals for test access - these will be populated when buildCommands is called
let parseArgv, formatArgv, items, buildPrompt, commands;

// Build commands with state accessor - called when mode is first used
function buildCommands(stateArg) {
    const ctxmgr = contextManager({
        getItems: () => stateArg.get(shared.contextItems) || [],
        setItems: (v) => stateArg.set(shared.contextItems, v),
        nextIntegerId: nextIntegerId,
        buildPrompt: () => {
            const fullContext = buildContext(stateArg.get(shared.contextItems), {toTxtar: () => context.toTxtar()});
            return template.execute(codeReviewTemplate, {
                contextTxtar: fullContext
            });
        }
    });

    // Export for test access as both module-level and global variables
    parseArgv = ctxmgr.parseArgv;
    formatArgv = ctxmgr.formatArgv;
    items = ctxmgr.getItems;
    buildPrompt = ctxmgr.buildPrompt;

    // Also set as global variables for cross-script access
    globalThis.parseArgv = parseArgv;
    globalThis.formatArgv = formatArgv;
    globalThis.items = items;
    globalThis.buildPrompt = buildPrompt;

    // Build the commands object
    const commandsObj = {
        ...ctxmgr.commands,
        note: {
            ...ctxmgr.commands.note,
            usage: "note [text|--goals]",
            handler: function (args) {
                if (args.length === 1 && args[0] === "--goals") {
                    // Show goal-based review notes
                    output.print("Pre-written review focus areas:");
                    output.print("  note goal:comments     - Focus on comment quality and usefulness");
                    output.print("  note goal:docs         - Focus on documentation completeness");
                    output.print("  note goal:tests        - Focus on test coverage and quality");
                    // The following areas are not first-class 'goals' yet, but you can still use them as review focuses:
                    output.print("  note goal:performance  - Focus on performance implications (review focus)");
                    output.print("  note goal:security     - Focus on security considerations (review focus)");
                    return;
                }
                if (args.length === 1 && args[0].startsWith("goal:")) {
                    const goalType = args[0].substring(5);
                    const goalNotes = {
                        "comments": "Focus on comment quality: Are comments useful and up-to-date? Remove any redundant comments that merely repeat the code. Ensure complex logic is well-explained.",
                        "docs": "Focus on documentation completeness: Is the code properly documented? Are API changes reflected in documentation? Are usage examples clear and current?",
                        "tests": "Focus on test coverage and quality: Are there sufficient tests for new functionality? Do tests cover edge cases? Are test names descriptive?",
                        "performance": "Focus on performance implications: Are there any performance bottlenecks? Is memory usage optimal? Are algorithms efficient for the expected scale?",
                        "security": "Focus on security considerations: Are inputs properly validated? Are there any potential security vulnerabilities? Is sensitive data handled correctly?"
                    };

                    if (goalNotes[goalType]) {
                        const id = ctxmgr.addItem("note", "review-focus", goalNotes[goalType]);
                        output.print("Added goal-based review note [" + id + "]: " + goalType);
                    } else {
                        output.print("Unknown goal type: " + goalType);
                        output.print("Use 'note --goals' to see available goal types.");
                    }
                    return;
                }
                // Delegate to base implementation
                return ctxmgr.commands.note.handler(args);
            }
        },
        show: {
            ...ctxmgr.commands.show,
            description: "Show the code review prompt"
        },
    };

    // Export for test access
    commands = commandsObj;
    globalThis.commands = commandsObj;

    return commandsObj;
}

// Initialize the mode
ctx.run("register-mode", function () {
    tui.registerMode({
        name: MODE_NAME,
        tui: {
            title: "Code Review",
            prompt: "(review) > ",
            enableHistory: true,
            historyFile: ".code-review_history"
        },
        onEnter: function () {
            // Commands are already built by the commands() function call during mode switch
            // Show a compact, single-line initial message so startup is concise.
            output.print("Type 'help' for commands. Tip: Try 'note --goals'.");
        },
        // Return the commands built with the current state accessor
        commands: function () {
            return buildCommands(state);
        }
    });
});

// Auto-switch into review mode when this script loads
ctx.run("enter-review", function () {
    tui.switchMode(MODE_NAME);
});
