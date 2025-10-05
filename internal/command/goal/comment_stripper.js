// Comment Stripper Goal: Remove useless comments and refactor useful ones
// This goal helps clean up codebases by removing redundant comments while preserving valuable ones

const nextIntegerId = require('osm:nextIntegerId');
const {buildContext, contextManager} = require('osm:ctxutil');

// Goal metadata
const GOAL_META = {
    name: "Comment Stripper",
    description: "Remove useless comments and refactor useful ones",
    category: "code-refactoring",
    usage: "Analyzes code files and removes redundant comments while preserving valuable documentation"
};

// State management
const STATE = {
    mode: "comment-stripper",
    contextItems: "contextItems"
};

function items() {
    return tui.getState(STATE.contextItems) || [];
}

function setItems(v) {
    tui.setState(STATE.contextItems, v);
}

function buildPrompt() {
    const goal = `Analyze the provided code and remove useless comments while refactoring useful ones according to these rules:

1. **Remove useless comments:**
   - Comments that merely repeat what the code does (e.g., "increment counter" for i++)
   - Outdated comments that no longer match the code
   - TODO/FIXME comments that are no longer relevant
   - Commented-out code that serves no purpose
   - Redundant header comments in simple functions

2. **Preserve and refactor useful comments:**
   - Business logic explanations and reasoning
   - Complex algorithm explanations
   - Performance considerations
   - Security implications
   - API documentation and usage examples
   - Configuration and setup instructions
   - Workaround explanations

3. **Refactoring guidelines:**
   - Move complex logic explanations closer to the relevant code
   - Convert inline comments to proper documentation where appropriate
   - Ensure remaining comments add genuine value
   - Fix grammar, spelling, and formatting of preserved comments
   - Update outdated but still relevant comments

4. **Output format:**
   - Show the cleaned code with explanations for each change
   - List removed comments with reasons for removal
   - List preserved/refactored comments with explanations
   - Provide a summary of the improvements made

Maintain all functionality and behavior of the original code while improving its readability and maintainability.`;

    const pb = tui.createPromptBuilder("comment-stripper", "Build comment stripper prompt");
    pb.setTemplate(`**${GOAL_META.description.toUpperCase()}**

{{goal}}

## CODE TO ANALYZE

{{context_txtar}}`);

    const fullContext = buildContext(items(), {toTxtar: () => context.toTxtar()});
    pb.setVariable("goal", goal);
    pb.setVariable("context_txtar", fullContext);
    return pb.build();
}

// Create context manager
const ctxmgr = contextManager({
    getItems: items,
    setItems: setItems,
    nextIntegerId: nextIntegerId,
    buildPrompt: buildPrompt
});

function buildCommands() {
    return {
        ...ctxmgr.commands,
        run: {
            description: "Quick run - add files and show prompt",
            usage: "run [file ...]",
            argCompleters: ["file"],
            handler: function (args) {
                if (args.length === 0) {
                    output.print("Usage: run [file ...]");
                    return;
                }
                // Add files using the base command
                ctxmgr.commands.add.handler(args);
                // Show prompt
                output.print("\n" + buildPrompt());
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
            title: "Comment Stripper",
            prompt: "(comment-stripper) > ",
            enableHistory: true,
            historyFile: ".comment-stripper_history"
        },
        onEnter: function () {
            if (!tui.getState(STATE.contextItems)) {
                tui.setState(STATE.contextItems, []);
            }
            banner();
            help();
        },
        onExit: function () {
            output.print("Goodbye!");
        },
        commands: buildCommands()
    });
});

function banner() {
    output.print("Comment Stripper: Remove useless comments, refactor useful ones");
    output.print("Type 'help' for commands. Use 'comment-stripper' to return here later.");
}

function help() {
    output.print("Commands: add, note, list, edit, remove, show, copy, run, help, exit");
}

// Export metadata for the goals system
if (typeof module !== 'undefined' && module.exports) {
    module.exports = GOAL_META;
}