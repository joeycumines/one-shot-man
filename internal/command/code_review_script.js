// Code Review: Single-prompt code review with context (baked-in version)
// This is the built-in version of the code-review script with embedded template

const nextIntegerId = require('osm:nextIntegerId');
const {buildContext, contextManager} = require('osm:ctxutil');

// State keys
const STATE = {
    mode: "review",             // fixed mode name
    contextItems: "contextItems" // array of { id, type: file|diff|note, label, payload }
};

// Helper functions for state management
function items() {
    return tui.getState(STATE.contextItems) || [];
}

function setItems(v) {
    tui.setState(STATE.contextItems, v);
}

function buildPrompt() {
    const pb = tui.createPromptBuilder("review", "Build code review prompt");
    pb.setTemplate(codeReviewTemplate);
    const fullContext = buildContext(items(), {toTxtar: () => context.toTxtar()});
    pb.setVariable("context_txtar", fullContext);
    return pb.build();
}

// Create context manager with code-review specific configuration
const ctxmgr = contextManager({
    getItems: items,
    setItems: setItems,
    nextIntegerId: nextIntegerId,
    buildPrompt: buildPrompt
});

// Expose parseArgv and formatArgv for test access
const {parseArgv, formatArgv} = ctxmgr;

// Build commands by extending the base contextManager commands
function buildCommands() {
    return {
        add: ctxmgr.commands.add,
        diff: ctxmgr.commands.diff,
        list: ctxmgr.commands.list,
        edit: ctxmgr.commands.edit,
        remove: ctxmgr.commands.remove,
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
            description: "Show the code review prompt",
            handler: ctxmgr.commands.show.handler
        },
        copy: {
            description: "Copy code review prompt to clipboard",
            handler: function () {
                const text = buildPrompt();
                try {
                    ctxmgr.clipboardCopy(text);
                    output.print("Code review prompt copied to clipboard.");
                } catch (e) {
                    output.print("Clipboard error: " + (e && e.message ? e.message : e));
                }
            }
        },
        help: {description: "Show help", handler: help}
    };
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
    output.print("Tip: Use 'note --goals' to see goal-based review focuses");
}

// Auto-switch into review mode when this script loads
ctx.run("enter-review", function () {
    tui.switchMode(STATE.mode);
});
