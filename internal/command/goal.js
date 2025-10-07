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
    const template = require('osm:text/template');

    // Build state contract from GOAL_CONFIG.StateKeys
    const stateContractDef = {
        contextItems: {
            description: config.Name + ":contextItems",
            defaultValue: []
        }
    };

    // Add state keys from config
    if (config.StateKeys) {
        for (const key in config.StateKeys) {
            stateContractDef[key] = {
                description: config.Name + ":" + key,
                defaultValue: config.StateKeys[key]
            };
        }
    }

    const StateKeys = tui.createStateContract(config.Name, stateContractDef);

    // Build commands from configuration
    function buildCommands(state) {
        // Build prompt from configuration
        function buildPrompt() {
            // Get current state values for template interpolation
            const stateVars = {};
            if (config.StateKeys) {
                for (const key in config.StateKeys) {
                    stateVars[key] = state.get(StateKeys[key]);
                }
            }

            // Build context txtar
            const fullContext = buildContext(state.get(StateKeys.contextItems), {toTxtar: () => context.toTxtar()});

            // Use the PromptTemplate from Go configuration
            const promptText = config.PromptTemplate || "";

            // Prepare template data
            const templateData = {
                Description: config.Description || "",
                ContextHeader: config.ContextHeader || "CONTEXT",
                ContextTxtar: fullContext,
                StateKeys: stateVars
            };

            // Handle PromptInstructions with dynamic substitutions
            let instructions = config.PromptInstructions || "";

            // Handle dynamic instruction substitutions from PromptOptions
            if (config.PromptOptions) {
                for (const optionKey in config.PromptOptions) {
                    const optionMap = config.PromptOptions[optionKey];
                    if (typeof optionMap === 'object' && optionMap !== null) {
                        const stateKeyBase = optionKey.replace(/Instructions$/, '');
                        const stateValue = stateVars[stateKeyBase];
                        if (stateValue && optionMap[stateValue]) {
                            templateData[optionKey.charAt(0).toUpperCase() + optionKey.slice(1)] = optionMap[stateValue];
                        }
                    }
                }
            }

            // Handle framework info
            if (stateVars.framework && stateVars.framework !== "auto") {
                templateData.FrameworkInfo = "\nUse the " + stateVars.framework + " testing framework.";
            } else {
                templateData.FrameworkInfo = "";
            }

            templateData.PromptInstructions = instructions;

            // Create template with custom functions
            const tmpl = template.new("goal");
            tmpl.funcs({
                upper: function(s) {
                    return s.toUpperCase();
                }
            });
            tmpl.parse(promptText);

            return tmpl.execute(templateData);
        }

        // Create context manager
        const ctxmgr = contextManager({
            getItems: () => state.get(StateKeys.contextItems),
            setItems: (v) => state.set(StateKeys.contextItems, v),
            nextIntegerId: nextIntegerId,
            buildPrompt: buildPrompt
        });

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
                    handler = new Function('args', 'ctxmgr', 'output', 'tui', 'buildPrompt', 'state', 'StateKeys', handlerCode);

                    // Wrap to provide the correct context
                    const wrappedHandler = function(args) {
                        return handler.call(this, args, ctxmgr, output, tui, buildPrompt, state, StateKeys);
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
        // Register the mode
        tui.registerMode({
            name: config.Name,
            stateContract: StateKeys,
            tui: {
                title: config.TUITitle || config.Name,
                prompt: config.TUIPrompt || "> ",
                enableHistory: config.EnableHistory || false,
                historyFile: config.HistoryFile || ""
            },
            onEnter: function (_, stateObj) {
                banner();
                help();
            },
            onExit: function (_, stateObj) {
                output.print("Goodbye!");
            },
            commands: buildCommands
        });
    });
})();
