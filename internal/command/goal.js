// Generic Goal Interpreter
// This script is a simple, declarative interpreter that reads goal configuration
// from Go and registers the appropriate mode. All goal-specific logic is defined
// in the Go Goal struct, not here.

(function () {
    'use strict';

    // GOAL_CONFIG is injected by Go before this script runs
    if (typeof GOAL_CONFIG === 'undefined') {
        throw new Error("GOAL_CONFIG not defined - this script must be called with configuration from Go");
    }

    const config = GOAL_CONFIG;
    const tuiTitle = config.tuiTitle || config.name;

    const nextIntegerId = require('osm:nextIntegerId');
    const {buildContext, contextManager} = require('osm:ctxutil');
    const template = require('osm:text/template');

    // Import shared symbols
    const shared = require('osm:sharedStateSymbols');

    // Initialize stateContractDef with shared contextItems
    const stateContractDef = {
        [shared.contextItems]: {defaultValue: []}
    };

    // Create command-specific symbols (that is, this command - NOT shared).
    const StateKeys = {contextItems: shared.contextItems}; // For convenience
    if (config.stateKeys) {
        for (const key in config.stateKeys) {
            if (!Object.hasOwn(config.stateKeys, key)) {
                continue;
            }

            // Create the symbol, namespaced by the goal name
            const symbol = Symbol(config.name + ":" + key);
            StateKeys[key] = symbol; // Store for JS-side access

            // Add definition to the contract
            stateContractDef[symbol] = {
                defaultValue: config.stateKeys[key]
            };
        }
    }

    // Create the state accessor
    const state = tui.createState(config.name, stateContractDef);

    const templateFuncs = {
        upper: function (s) {
            return s.toUpperCase();
        },
    };

    // the banner is able to overridden via config
    const bannerTemplate = template.new("bannerText");
    bannerTemplate.funcs(templateFuncs);
    bannerTemplate.parse(config.bannerText ||
        '=== {{ .tuiTitle }}{{ if .notableVariables }}:{{ range .notableVariables }} ' +
        '{{ . }}={{ index $.StateKeys . }}{{ end }}{{ end }} ===\n' +
        'Type \'help\' for commands.');

    // Build prompt from configuration
    function buildPrompt() {
        const templateData = buildBaseTemplateData(); // N.B. func defined below

        templateData.ContextTxtar = buildContext(state.get(StateKeys.contextItems), {toTxtar: () => context.toTxtar()});

        // The promptInstructions string is itself a template. We must execute it
        // first using the template data we've just constructed to resolve any
        // dynamic values before injecting it into the main prompt template.
        const instructionsTmpl = template.new("instructions");
        // Ensure the instructions template has access to the same helper functions.
        instructionsTmpl.funcs(templateFuncs);
        instructionsTmpl.parse(config.promptInstructions || "");
        templateData.PromptInstructions = instructionsTmpl.execute(templateData);

        // Create template with custom functions
        const tmpl = template.new("goal");
        tmpl.funcs(templateFuncs);
        tmpl.parse(config.promptTemplate || "");

        return tmpl.execute(templateData);
    }

    // Create context manager
    const ctxmgr = contextManager({
        getItems: () => state.get(shared.contextItems) || [],
        setItems: (v) => state.set(shared.contextItems, v),
        nextIntegerId,
        buildPrompt,
    });

    function buildBaseTemplateData() {
        // Prepare template data
        const templateData = {
            Description: config.description || "",
            ContextHeader: config.contextHeader || "CONTEXT",
            StateKeys: {},
            promptOptions: config.promptOptions,
            tuiTitle,
            notableVariables: config.notableVariables || [],
        };

        if (config.stateKeys) {
            for (const key in config.stateKeys) {
                if (Object.hasOwn(config.stateKeys, key)) {
                    templateData.StateKeys[key] = state.get(StateKeys[key]);
                }
            }
        }

        // Handle dynamic instruction substitutions from promptOptions.
        // This maps state keys to specific instructions based on the current state value.
        // E.g., if StateKeys.language is "ES", it looks for promptOptions.languageInstructions["ES"].
        if (typeof config.promptOptions === 'object' && config.promptOptions !== null) {
            const instructionsSuffix = 'Instructions';

            for (const optionKey in config.promptOptions) {
                // 1. Validate the optionKey format and existence.
                if (!Object.hasOwn(config.promptOptions, optionKey) ||
                    typeof optionKey !== 'string' ||
                    !optionKey.endsWith(instructionsSuffix) ||
                    optionKey === instructionsSuffix) {
                    continue;
                }

                const optionMap = config.promptOptions[optionKey];

                // 2. Validate that the value associated with the key is a map (object).
                if (typeof optionMap !== 'object' || optionMap === null) {
                    continue;
                }

                // 3. Derive the stateKey (e.g., "language" from "languageInstructions").
                const stateKey = optionKey.substring(0, optionKey.length - instructionsSuffix.length);

                // 4. Verify this state key exists in the configuration.
                if (!config.stateKeys || !Object.hasOwn(config.stateKeys, stateKey)) {
                    continue;
                }

                // 5. Get the current active value for this state (e.g., "EN", "dark_mode", etc.).
                const stateValue = templateData.StateKeys?.[stateKey];

                // Check if the state value is valid string.
                if (typeof stateValue !== 'string' || stateValue === '') {
                    continue;
                }

                // 6. DIRECT LOOKUP: Check if an instruction exists for this specific state value.
                // (Previously, this was wrapped in a redundant loop iterating over all map keys).
                if (!Object.hasOwn(optionMap, stateValue)) {
                    continue;
                }

                const instructionsValue = optionMap[stateValue];

                // 7. Apply the instruction if valid.
                if (typeof instructionsValue === 'string' && instructionsValue !== '') {
                    const targetKey = stateKey.charAt(0).toUpperCase() + stateKey.slice(1) + instructionsSuffix;
                    templateData[targetKey] = instructionsValue;
                }
            }
        }



        return templateData;
    }

    // Build commands from configuration
    function buildCommands() {
        const commands = {
            // N.B. This inherits the default description, and runs _after_ the built-in help.
            help: {handler: help},
        };

        const commandConfigs = config.commands || [];

        for (let i = 0; i < commandConfigs.length; i++) {
            const cmdConfig = commandConfigs[i];

            if (cmdConfig.type === "contextManager") {
                // Use the base context manager command
                if (ctxmgr.commands[cmdConfig.name]) {
                    commands[cmdConfig.name] = ctxmgr.commands[cmdConfig.name];
                    // Override description if provided
                    if (cmdConfig.description) {
                        commands[cmdConfig.name] = {
                            ...ctxmgr.commands[cmdConfig.name],
                            description: cmdConfig.description
                        };
                    }
                }
            } else if (cmdConfig.type === "custom") {
                // Create handler function from string using new Function()
                // This provides a more constrained scope than dynamic evaluation
                let handler;
                try {
                    // Parse the handler string as a function expression
                    // Remove the "function (args)" wrapper if present and extract the body
                    let handlerCode = cmdConfig.handler.trim();

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
                    const wrappedHandler = function (args) {
                        return handler.call(this, args, ctxmgr, output, tui, buildPrompt, state, StateKeys);
                    };

                    commands[cmdConfig.name] = {
                        description: cmdConfig.description || "",
                        usage: cmdConfig.usage || "",
                        argCompleters: cmdConfig.argCompleters || [],
                        handler: wrappedHandler
                    };
                } catch (e) {
                    output.print("Error creating handler for command " + cmdConfig.name + ": " + e);
                    continue;
                }
            }
        }

        return commands;
    }

    function printBanner() {
        const templateData = buildBaseTemplateData();
        const bannerText = bannerTemplate.execute(templateData);
        output.print(bannerText);
    }

    function help() {
        if (config.helpText) {
            output.print("\n" + config.helpText);
        }
    }

    // Initialize the mode
    ctx.run("register-mode", function () {
        // Register the mode
        tui.registerMode({
            name: config.name,
            tui: {
                title: tuiTitle,
                prompt: config.tuiPrompt || "> ",
                enableHistory: config.enableHistory || false,
                historyFile: config.historyFile || ""
            },
            onEnter: printBanner,
            commands: buildCommands,
        });
    });
})();
