// Code Review: Single-prompt code review with context (baked-in version)
// This is the built-in version of the code-review script with embedded template

const nextIntegerId = require('osm:nextIntegerId');
const {buildContext, contextManager} = require('osm:ctxutil');

// Fixed mode name
const MODE_NAME = "review";

// Define state contract with Symbol keys
const StateKeys = tui.createStateContract(MODE_NAME, {
    contextItems: {
        description: MODE_NAME + ":contextItems",
        defaultValue: []
    }
});

// Globals for test access - these will be populated when buildCommands is called
let parseArgv, formatArgv, items, buildPrompt, commands;

// Build commands with state accessor - called when mode is first used
function buildCommands(state) {

    const ctxmgr = contextManager({
        getItems: () => state.get(StateKeys.contextItems),
        setItems: (v) => state.set(StateKeys.contextItems, v),
        nextIntegerId: nextIntegerId,
        buildPrompt: () => {
            const pb = tui.createPromptBuilder("review", "Build code review prompt");
            pb.setTemplate(codeReviewTemplate);
            const fullContext = buildContext(state.get(StateKeys.contextItems), {toTxtar: () => context.toTxtar()});
            pb.setVariable("context_txtar", fullContext);
            return pb.build();
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
        help: {description: "Show help", handler: help}
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
        stateContract: StateKeys,
        tui: {
            title: "Code Review",
            prompt: "(code-review) > ",
            enableHistory: true,
            historyFile: ".code-review_history"
        },
        onEnter: function (_, stateObj) {
            // Commands are already built by the commands() function call during mode switch
            // We just need to show the banner and help
            banner();
            help();
        },
        onExit: function (_, stateObj) {
            output.print("Exiting Code Review.");
        },
        // Return the commands built with the current state accessor
        commands: buildCommands
    });

    tui.registerCommand({
        name: "review",
        description: "Switch to Code Review mode",
        handler: function () {
            tui.switchMode(MODE_NAME);
        }
    });
});

function banner() {
    output.print("Code Review: context -> single prompt for PR review");
    output.print("Type 'help' for commands. Use 'review' to return here later.");
}

function help() {
	output.print("Commands: add, diff, note, list, edit, remove, show, copy, help, exit");
	output.print("Tip: Use 'note --goals' to see goal-based review focuses");
}

// Auto-switch into review mode when this script loads
ctx.run("enter-review", function () {
	tui.switchMode(MODE_NAME);
});
