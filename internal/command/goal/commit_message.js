// Commit Message Goal: Generate commit messages based on Kubernetes style guidelines
// This goal helps generate commit messages using diff context following K8s semantic conventions

const nextIntegerId = require('osm:nextIntegerId');
const {buildContext, contextManager} = require('osm:ctxutil');

// Goal metadata
const GOAL_META = {
    name: "Commit Message",
    description: "Generate Kubernetes-style commit messages from diffs and context",
    category: "git-workflow",
    usage: "Generates commit messages following Kubernetes semantic guidelines from git diffs and additional context"
};

// State management
const STATE = {
    mode: "commit-message",
    contextItems: "contextItems"
};

function items() {
    return tui.getState(STATE.contextItems) || [];
}

function setItems(v) {
    tui.setState(STATE.contextItems, v);
}

function buildPrompt() {
    const goal = `You MUST produce a commit message strictly utilizing the following syntax / style / semantics.

Kubernetes (K8s) commit message guidelines emphasize clarity, conciseness, and adherence to a specific format for better code history and reviewability. ATTN: This style is explicitly NOT "conventional commit" formatted. The core principles are:

Subject Line:
    Concise Summary: The first line, the subject, should provide a brief summary of the change.
    Length: Aim for 50 characters or less, and do not exceed 72 characters.
    Imperative Mood: Use the imperative mood (e.g., "Add feature," "Fix bug," not "Added feature" or "Adds feature").
    Capitalization: Capitalize the first word of the subject unless it's a lowercase symbol or identifier.
    No Period: Do not end the subject line with a period.

Body:
    Blank Line Separation: Add a single blank line between the subject and the body.
    Detailed Explanation: The body should explain the "what" and "why" of the commit, providing context and rationale for the changes. Avoid simply restating "how" the change was implemented, as the code itself will show that.
    Wrap at 72 Characters: Wrap the lines of the body at 72 characters for readability.

General Guidelines:
    Avoid GitHub Keywords/Mentions:
    Do not use GitHub keywords (e.g., "fixes #123") or @mentions within the commit message itself. These belong in the Pull Request description.
    Squash Small Commits:
    For minor changes like typos or style fixes, consider squashing commits to maintain a cleaner git history.
    Meaningful Messages:
    Avoid vague messages like "fixed stuff" or "updated code." Strive for clear and meaningful descriptions.

Generate a commit message that follows these guidelines based on the provided diff and context information.`;

    const pb = tui.createPromptBuilder("commit-message", "Build commit message prompt");
    pb.setTemplate(`**${GOAL_META.description.toUpperCase()}**

{{goal}}

## DIFF CONTEXT / CHANGES

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
            description: "Quick run - add git diff and show prompt",
            usage: "run [git-diff-args...]",
            handler: function (args) {
                // Add git diff using the base command
                ctxmgr.commands.diff.handler(args.length > 0 ? args : []);
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
            title: "Commit Message",
            prompt: "(commit-message) > ",
            enableHistory: true,
            historyFile: ".commit-message_history"
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
    output.print("Commit Message: Generate Kubernetes-style commit messages");
    output.print("Type 'help' for commands. Use 'commit-message' to return here later.");
}

function help() {
    output.print("Commands: add, diff, note, list, edit, remove, show, copy, run, help, exit");
}

// Export metadata for the goals system
if (typeof module !== 'undefined' && module.exports) {
    module.exports = GOAL_META;
}