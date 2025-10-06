// Generic Goal Interpreter
// This script is a simple, declarative interpreter that reads goal configuration
// from Go and registers the appropriate mode. All goal-specific logic is defined
// in the Go Goal struct, not here.

(function() {
    'use strict';

    // GOAL_CONFIG is injected by Go before this script runs
    if (typeof GOAL_CONFIG === 'undefined') {
        throw new Error("GOAL_CONFIG not defined - this script must be called with configuration from Go");
    }

    const config = GOAL_CONFIG;
    const nextIntegerId = require('osm:nextIntegerId');
    const {buildContext, contextManager} = require('osm:ctxutil');

    // State key for context items
    const CONTEXT_ITEMS_KEY = "contextItems";

    // Getter/setter for context items
    function items() {
        return tui.getState(CONTEXT_ITEMS_KEY) || [];
    }

    function setItems(v) {
        tui.setState(CONTEXT_ITEMS_KEY, v);
    }

    // Build prompt from configuration
    function buildPrompt() {
        // Get current state values for template interpolation
        const stateVars = {};
        if (config.StateKeys) {
            for (const key in config.StateKeys) {
                stateVars[key] = tui.getState(key);
            }
        }

        // Build context txtar
        const fullContext = buildContext(items(), {toTxtar: () => context.toTxtar()});

        // Create prompt builder
        const pb = tui.createPromptBuilder(config.Name, "Build " + config.Description + " prompt");
        
        // Use the PromptTemplate from Go configuration
        let promptText = config.PromptTemplate || "";

        // Perform template substitutions
        // Replace {{.Description | upper}}
        promptText = promptText.replace(/\{\{\.Description \| upper\}\}/g, (config.Description || "").toUpperCase());

        // Replace {{.Description}}
        promptText = promptText.replace(/\{\{\.Description\}\}/g, config.Description || "");

        // Replace {{.PromptInstructions}}
        let instructions = config.PromptInstructions || "";
        
        // Handle dynamic instruction substitutions from PromptOptions
        if (config.PromptOptions) {
            // Generic replacement for any option map (e.g., docTypeInstructions, testTypeInstructions)
            for (const optionKey in config.PromptOptions) {
                const optionMap = config.PromptOptions[optionKey];
                if (typeof optionMap === 'object' && optionMap !== null) {
                    // Find the corresponding state key (e.g., docType, testType)
                    // by removing "Instructions" suffix from option key
                    const stateKeyBase = optionKey.replace(/Instructions$/, '');
                    const stateValue = stateVars[stateKeyBase];
                    
                    if (stateValue && optionMap[stateValue]) {
                        const placeholder = "{{." + optionKey.charAt(0).toUpperCase() + optionKey.slice(1) + "}}";
                        instructions = instructions.replace(placeholder, optionMap[stateValue]);
                    }
                }
            }
        }

        // Handle framework info placeholder (used when a framework state variable is set)
        if (stateVars.framework && stateVars.framework !== "auto") {
            instructions = instructions.replace("{{.FrameworkInfo}}", "\nUse the " + stateVars.framework + " testing framework.");
        } else {
            instructions = instructions.replace("{{.FrameworkInfo}}", "");
        }

        // Replace state variable references
        instructions = instructions.replace(/\{\{\.StateKeys\.(\w+)\}\}/g, function(match, key) {
            return stateVars[key] || "";
        });

        promptText = promptText.replace(/\{\{\.PromptInstructions\}\}/g, instructions);

        // Replace {{.ContextHeader}}
        promptText = promptText.replace(/\{\{\.ContextHeader\}\}/g, config.ContextHeader || "CONTEXT");

        // Replace {{.ContextTxtar}}
        promptText = promptText.replace(/\{\{\.ContextTxtar\}\}/g, fullContext);

        pb.setTemplate(promptText);
        return pb.build();
    }

    // Create context manager
    const ctxmgr = contextManager({
        getItems: items,
        setItems: setItems,
        nextIntegerId: nextIntegerId,
        buildPrompt: buildPrompt
    });

    // Build commands from configuration
    function buildCommands() {
        const commands = {};

        // Guard against undefined Commands array
        const commandConfigs = config.Commands || [];

        for (let i = 0; i < commandConfigs.length; i++) {
            const cmdConfig = commandConfigs[i];

            if (cmdConfig.Type === "contextManager") {
                // Use the base context manager command
                if (ctxmgr.commands[cmdConfig.Name]) {
                    commands[cmdConfig.Name] = ctxmgr.commands[cmdConfig.Name];
                    // Override description if provided
                    if (cmdConfig.Description) {
                        commands[cmdConfig.Name] = {
                            ...ctxmgr.commands[cmdConfig.Name],
                            description: cmdConfig.Description
                        };
                    }
                }
            } else if (cmdConfig.Type === "custom") {
                // Create handler function from string using new Function()
                // This provides a more constrained scope than dynamic evaluation
                let handler;
                try {
                    // Parse the handler string as a function expression
                    // Remove the "function (args)" wrapper if present and extract the body
                    let handlerCode = cmdConfig.Handler.trim();
                    
                    // If it's a function expression like "function (args) { ... }"
                    // we need to extract just the body
                    const funcMatch = handlerCode.match(/^function\s*\([^)]*\)\s*\{([\s\S]*)\}$/);
                    if (funcMatch) {
                        handlerCode = funcMatch[1];
                    }
                    
                    // Create function with access to necessary variables
                    // new Function is safer than eval as it only has access to global scope
                    handler = new Function('args', 'ctxmgr', 'output', 'tui', 'buildPrompt', handlerCode);
                    
                    // Wrap to provide the correct context
                    const wrappedHandler = function(args) {
                        return handler.call(this, args, ctxmgr, output, tui, buildPrompt);
                    };
                    
                    commands[cmdConfig.Name] = {
                        description: cmdConfig.Description || "",
                        usage: cmdConfig.Usage || "",
                        argCompleters: cmdConfig.ArgCompleters || [],
                        handler: wrappedHandler
                    };
                } catch (e) {
                    output.print("Error creating handler for command " + cmdConfig.Name + ": " + e);
                    continue;
                }
            } else if (cmdConfig.Type === "help") {
                commands[cmdConfig.Name] = {
                    description: "Show help",
                    handler: help
                };
            }
        }

        return commands;
    }

    // Banner function
    function banner() {
        if (config.BannerText) {
            output.print(config.BannerText);
        }
        output.print("Type 'help' for commands. Use '" + config.Name + "' to return here later.");
    }

    // Help function
    function help() {
        if (config.HelpText) {
            output.print(config.HelpText);
        } else {
            output.print("No help available for this goal.");
        }
    }

    // Initialize the mode
    ctx.run("register-mode", function () {
        // Initialize state keys
        const onEnterFunc = function () {
            if (config.StateKeys) {
                for (const key in config.StateKeys) {
                    if (tui.getState(key) === undefined || tui.getState(key) === null) {
                        tui.setState(key, config.StateKeys[key]);
                    }
                }
            }
            banner();
            help();
        };

        const onExitFunc = function () {
            output.print("Goodbye!");
        };

        // Register the mode
        tui.registerMode({
            name: config.Name,
            tui: {
                title: config.TUITitle || config.Name,
                prompt: config.TUIPrompt || "> ",
                enableHistory: config.EnableHistory || false,
                historyFile: config.HistoryFile || ""
            },
            onEnter: onEnterFunc,
            onExit: onExitFunc,
            commands: buildCommands()
        });
    });
})();
