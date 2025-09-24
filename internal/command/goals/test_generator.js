// Test Generator Goal: Generate comprehensive tests for code
// This goal helps create thorough test suites for existing code

const {buildContext} = require('osm:ctxutil');

// Goal metadata
const GOAL_META = {
    name: "Test Generator",
    description: "Generate comprehensive test suites for existing code",
    category: "testing",
    usage: "Analyzes code and generates unit tests, integration tests, and edge case coverage"
};

// State management
const STATE = {
    mode: "test-generator",
    contextItems: "contextItems",
    testType: "testType",
    framework: "framework"
};

const nextIntegerId = require('osm:nextIntegerId');
const {openEditor: osOpenEditor, clipboardCopy} = require('osm:os');

// Initialize the mode
ctx.run("register-mode", function () {
    tui.registerMode({
        name: STATE.mode,
        tui: {
            title: "Test Generator",
            prompt: "(test-gen) > ",
            enableHistory: true,
            historyFile: ".test-generator_history"
        },
        onEnter: function () {
            if (!tui.getState(STATE.contextItems)) {
                tui.setState(STATE.contextItems, []);
            }
            if (!tui.getState(STATE.testType)) {
                tui.setState(STATE.testType, "unit");
            }
            if (!tui.getState(STATE.framework)) {
                tui.setState(STATE.framework, "auto");
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
    output.print("Test Generator: Create comprehensive test suites for your code");
    output.print("Type 'help' for commands. Use 'test-generator' to return here later.");
}

function help() {
    output.print("Commands: add, note, list, type, framework, edit, remove, show, copy, help, exit");
    output.print("Test types: unit, integration, e2e, performance, security");
    output.print("Frameworks: auto, jest, mocha, go, pytest, junit, rspec");
}

function items() {
    return tui.getState(STATE.contextItems) || [];
}

function setItems(v) {
    tui.setState(STATE.contextItems, v);
}

function addItem(type, label, payload) {
    const list = items();
    const id = nextIntegerId(list);
    list.push({id, type, label, payload});
    setItems(list);
    return id;
}

function buildPrompt() {
    const testType = tui.getState(STATE.testType) || "unit";
    const framework = tui.getState(STATE.framework) || "auto";
    
    const testTypeInstructions = {
        unit: `Generate comprehensive unit tests including:
- Test all public methods and functions
- Cover all branches and edge cases
- Test error conditions and exception handling
- Mock external dependencies appropriately
- Include boundary value testing
- Test both positive and negative scenarios`,
        
        integration: `Generate integration tests including:
- Test interactions between components
- Verify data flow between modules
- Test API endpoints and database interactions
- Cover cross-component workflows
- Test configuration and setup scenarios
- Include realistic test data scenarios`,
        
        e2e: `Generate end-to-end tests including:
- Test complete user workflows
- Verify UI interactions and responses
- Test data persistence across operations
- Cover critical user journeys
- Include performance expectations
- Test error recovery scenarios`,
        
        performance: `Generate performance tests including:
- Benchmark critical functions and operations
- Test resource usage and memory leaks
- Measure response times and throughput
- Test under various load conditions
- Include stress testing scenarios
- Set performance baselines and thresholds`,
        
        security: `Generate security tests including:
- Test input validation and sanitization
- Verify authentication and authorization
- Test for common vulnerabilities (XSS, SQL injection, etc.)
- Check data encryption and protection
- Test access controls and permissions
- Include penetration testing scenarios`
    };

    const frameworkInfo = framework !== "auto" ? `\nUse the ${framework} testing framework.` : "";

    const goal = `Generate ${testType} tests for the provided code following these guidelines:

${testTypeInstructions[testType]}${frameworkInfo}

**Test Quality Standards:**
- Write clear, descriptive test names that explain what is being tested
- Use the AAA pattern (Arrange, Act, Assert) for test structure
- Include setup and teardown code where appropriate
- Mock external dependencies and side effects
- Use meaningful test data that represents real-world scenarios
- Add comments explaining complex test logic or edge cases
- Ensure tests are deterministic and not flaky
- Group related tests logically

**Coverage Requirements:**
- Achieve high code coverage (aim for 90%+ for unit tests)
- Cover all public interfaces and methods
- Test both successful and failure paths
- Include edge cases and boundary conditions
- Test error handling and validation logic

**Output Format:**
- Provide complete, runnable test files
- Include necessary imports and setup code
- Add brief explanations for complex test scenarios
- Organize tests in logical groups or describe blocks
- Include any additional test utilities or helpers needed`;

    const pb = tui.createPromptBuilder("test-generator", "Build test generator prompt");
    pb.setTemplate(`**${GOAL_META.description.toUpperCase()}**

{{goal}}

## CODE TO TEST

{{context_txtar}}`);
    
    const fullContext = buildContext(items(), {toTxtar: () => context.toTxtar()});
    pb.setVariable("goal", goal);
    pb.setVariable("context_txtar", fullContext);
    return pb.build();
}

function buildCommands() {
    return {
        add: {
            description: "Add file content to context",
            usage: "add [file ...]",
            argCompleters: ["file"],
            handler: function (args) {
                if (args.length === 0) {
                    const edited = osOpenEditor("paths", "\n# one path per line\n");
                    args = edited.split(/\r?\n/).map(s => s.trim()).filter(s => s && !s.startsWith('#'));
                }
                for (const p of args) {
                    try {
                        const err = context.addPath(p);
                        if (err && err.message) {
                            output.print("Error adding " + p + ": " + err.message);
                            continue;
                        }
                        const id = addItem("file", p, {path: p});
                        output.print("Added file [" + id + "] " + p);
                    } catch (e) {
                        output.print("Error: " + e);
                    }
                }
            }
        },
        note: {
            description: "Add a note about test requirements",
            usage: "note [text]",
            handler: function (args) {
                let text = args.join(" ");
                if (!text) text = osOpenEditor("note", "");
                const id = addItem("note", "note", text);
                output.print("Added note [" + id + "]");
            }
        },
        type: {
            description: "Set test type",
            usage: "type <unit|integration|e2e|performance|security>",
            handler: function (args) {
                if (args.length === 0) {
                    output.print("Current type: " + (tui.getState(STATE.testType) || "unit"));
                    output.print("Available types: unit, integration, e2e, performance, security");
                    return;
                }
                const type = args[0].toLowerCase();
                const validTypes = ["unit", "integration", "e2e", "performance", "security"];
                if (!validTypes.includes(type)) {
                    output.print("Invalid type. Available: " + validTypes.join(", "));
                    return;
                }
                tui.setState(STATE.testType, type);
                output.print("Test type set to: " + type);
            }
        },
        framework: {
            description: "Set testing framework",
            usage: "framework <auto|jest|mocha|go|pytest|junit|rspec>",
            handler: function (args) {
                if (args.length === 0) {
                    output.print("Current framework: " + (tui.getState(STATE.framework) || "auto"));
                    output.print("Available frameworks: auto, jest, mocha, go, pytest, junit, rspec");
                    return;
                }
                const fw = args[0].toLowerCase();
                const validFrameworks = ["auto", "jest", "mocha", "go", "pytest", "junit", "rspec"];
                if (!validFrameworks.includes(fw)) {
                    output.print("Invalid framework. Available: " + validFrameworks.join(", "));
                    return;
                }
                tui.setState(STATE.framework, fw);
                output.print("Testing framework set to: " + fw);
            }
        },
        list: {
            description: "List context items and settings",
            handler: function () {
                output.print("Test type: " + (tui.getState(STATE.testType) || "unit"));
                output.print("Framework: " + (tui.getState(STATE.framework) || "auto"));
                output.print("Context items:");
                for (const it of items()) {
                    let line = "[" + it.id + "] [" + it.type + "] " + (it.label || "");
                    if (it.type === "note" && typeof it.payload === "string") {
                        const preview = it.payload.slice(0, 60);
                        line += ": " + preview + (it.payload.length > 60 ? "..." : "");
                    }
                    output.print(line);
                }
            }
        },
        edit: {
            description: "Edit a context item",
            usage: "edit <id>",
            handler: function (args) {
                if (args.length === 0) {
                    output.print("Usage: edit <id>");
                    return;
                }
                const id = parseInt(args[0]);
                const list = items();
                const idx = list.findIndex(it => it.id === id);
                if (idx === -1) {
                    output.print("Item [" + id + "] not found");
                    return;
                }
                const item = list[idx];
                if (item.type === "note") {
                    const edited = osOpenEditor("edit-note", item.payload);
                    if (edited !== null && edited.trim()) {
                        item.payload = edited.trim();
                        setItems(list);
                        output.print("Updated [" + id + "]");
                    }
                } else {
                    output.print("Cannot edit item type: " + item.type);
                }
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
                const id = parseInt(args[0]);
                const list = items();
                const idx = list.findIndex(it => it.id === id);
                if (idx === -1) {
                    output.print("Item [" + id + "] not found");
                    return;
                }
                const item = list[idx];
                if (item.type === "file") {
                    try {
                        const err = context.removePath(item.payload.path);
                        if (err && err.message) {
                            output.print("Error removing from context: " + err.message);
                            return;
                        }
                    } catch (e) {
                        output.print("Error: " + e);
                        return;
                    }
                }
                list.splice(idx, 1);
                setItems(list);
                output.print("Removed [" + id + "]");
            }
        },
        show: {
            description: "Show the test generator prompt",
            handler: function () {
                output.print(buildPrompt());
            }
        },
        copy: {
            description: "Copy test generator prompt to clipboard",
            handler: function () {
                const text = buildPrompt();
                try {
                    clipboardCopy(text);
                    output.print("Test generator prompt copied to clipboard.");
                } catch (e) {
                    output.print("Clipboard error: " + (e && e.message ? e.message : e));
                }
            }
        },
        help: {description: "Show help", handler: help}
    };
}

// Export metadata for the goals system
if (typeof module !== 'undefined' && module.exports) {
    module.exports = GOAL_META;
}