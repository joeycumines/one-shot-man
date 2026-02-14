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
// mode it exposes used to be called "review". Use a single consistent
// mode id of "code-review" now so the TUI mode and command name match.
const MODE_NAME = "code-review";

// Create the single state accessor with only shared contextItems
const state = tui.createState(COMMAND_NAME, {
    [shared.contextItems]: {defaultValue: []}
});

// Globals for test access - these will be populated when buildCommands is called
let parseArgv, formatArgv, items, buildPrompt, commands;

// Build commands with state accessor - called when mode is first used
function buildCommands(stateArg) {
    const ctxmgrOpts = {
        getItems: () => stateArg.get(shared.contextItems) || [],
        setItems: (v) => stateArg.set(shared.contextItems, v),
        nextIntegerId: nextIntegerId,
        buildPrompt: () => {
            const fullContext = buildContext(stateArg.get(shared.contextItems), {toTxtar: () => context.toTxtar()});
            return template.execute(codeReviewTemplate, {
                contextTxtar: fullContext
            });
        }
    };
    // Pass config-defined hot-snippets to contextManager if available.
    if (typeof CONFIG_HOT_SNIPPETS !== 'undefined' && Array.isArray(CONFIG_HOT_SNIPPETS) && CONFIG_HOT_SNIPPETS.length > 0) {
        ctxmgrOpts.hotSnippets = CONFIG_HOT_SNIPPETS;
    }
    const ctxmgr = contextManager(ctxmgrOpts);

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
        'review-chunks': {
            description: "Split large diffs into LLM-sized chunks and copy",
            usage: "review-chunks [N]",
            handler: function (args) {
                // Build the full prompt text and extract diff sections.
                var text = buildPrompt();
                var chunks = splitDiff(text, defaultMaxDiffLines);

                if (!chunks || chunks.length <= 1) {
                    output.print("Prompt fits in a single chunk (" +
                        text.split('\n').length + " lines). Use 'copy' instead.");
                    return;
                }

                // No argument: show chunk summary.
                if (!args || args.length === 0) {
                    output.print("Prompt splits into " + chunks.length + " chunks:");
                    for (var i = 0; i < chunks.length; i++) {
                        output.print("  Chunk " + (chunks[i].index + 1) + "/" +
                            chunks[i].total + ": " +
                            chunks[i].files.join(", ") +
                            " (" + chunks[i].lines + " lines)");
                    }
                    output.print("");
                    output.print("Usage: review-chunks <N>  — copy chunk N to clipboard");
                    return;
                }

                // Copy a specific chunk.
                var chunkNum = parseInt(args[0], 10);
                if (isNaN(chunkNum) || chunkNum < 1 || chunkNum > chunks.length) {
                    output.print("Invalid chunk number. Valid range: 1-" + chunks.length);
                    return;
                }

                var chunk = chunks[chunkNum - 1];

                // Re-use the review template with only this chunk's diff as context.
                var chunkContext = "## Code Review — Chunk " + (chunk.index + 1) +
                    "/" + chunk.total + "\n" +
                    "Files: " + chunk.files.join(", ") + "\n\n" +
                    "```diff\n" + chunk.content + "\n```";
                var chunkPrompt = template.execute(codeReviewTemplate, {
                    contextTxtar: chunkContext
                });

                try {
                    ctxmgr.clipboardCopy(chunkPrompt);
                    output.print("Chunk " + chunkNum + "/" + chunks.length +
                        " copied to clipboard (" + chunk.lines + " lines, files: " +
                        chunk.files.join(", ") + ").");
                    if (chunkNum < chunks.length) {
                        output.print("Next: review-chunks " + (chunkNum + 1));
                    } else {
                        output.print("All chunks copied.");
                    }
                } catch (e) {
                    output.print("Clipboard error: " + (e && e.message ? e.message : e));
                }
            }
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
            prompt: "(code-review) > ",
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

// Auto-switch into code-review mode when this script loads
ctx.run("enter-code-review", function () {
    tui.switchMode(MODE_NAME);
});
