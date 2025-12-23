// Generic Goal Interpreter
// This script is a simple, declarative interpreter that reads goal configuration
// from Go and registers the appropriate mode. All goal-specific logic is defined
// using the Goal struct (see goal.go), not here.

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
    const shared = require('osm:sharedStateSymbols');

    // Build the initial state - a record to look up symbols, and a contract for the accessor.
    const stateKeys = {};
    const stateVars = {};
    for (const [stringKey, opts] of [
        ['contextItems', {defaultValue: []}],
    ]) {
        if (!Object.hasOwn(shared, stringKey)) {
            throw new Error("Unknown shared state key: " + stringKey);
        }
        const symbolKey = shared[stringKey];
        if (typeof symbolKey !== 'symbol' ||
            typeof opts !== 'object' ||
            opts === null ||
            Array.isArray(opts) ||
            !Array.isArray(opts.defaultValue)) {
            throw new Error("Invalid state definition for: " + symbolKey);
        }
        if (stringKey in stateKeys || symbolKey in stateVars) {
            throw new Error("State key collision: " + stringKey);
        }
        stateKeys[stringKey] = symbolKey;
        stateVars[symbolKey] = opts;
    }
    if (config.stateVars) {
        for (const stringKey in config.stateVars) {
            if (typeof stringKey !== 'string' || !Object.hasOwn(config.stateVars, stringKey)) {
                continue;
            }

            if (stringKey in stateKeys) {
                throw new Error("State key collision: " + stringKey);
            }

            // The description _shouldn't_ matter but FYI this aligns with the
            // shared state symbol convention (e.g. "osm:shared/contextItems").
            const symbolKey = Symbol(`goal:${config.name}/${stringKey}`);
            stateKeys[stringKey] = symbolKey;
            stateVars[symbolKey] = {
                defaultValue: config.stateVars[stringKey]
            };
        }
    }

    // Create the state accessor, using the contact we built above.
    const state = tui.createState(config.name, stateVars);

    const templateFuncs = {
        upper: function (s) {
            return s.toUpperCase();
        },
    };

    // the banner is able to overridden via config
    const bannerTemplate = template.new("bannerTemplate");
    bannerTemplate.funcs(templateFuncs);
    bannerTemplate.parse(config.bannerTemplate ||
        '{{- /* 1. Construct the string variables to measure length */ -}}\n' +
        '{{- $vars := "" -}}\n' +
        '{{- range .notableVariables -}}\n' +
        '    {{- $vars = printf "%s %s=%s" $vars . (index $.stateKeys .) -}}\n' +
        '{{- end -}}\n' +
        '\n' +
        '{{- $fullLine := "" -}}\n' +
        '{{- if .notableVariables -}}\n' +
        '    {{- $fullLine = printf "=== %s:%s ===" .tuiTitle $vars -}}\n' +
        '{{- else -}}\n' +
        '    {{- $fullLine = printf "=== %s ===" .tuiTitle -}}\n' +
        '{{- end -}}\n' +
        '\n' +
        '{{- /* 2. Rendering Logic */ -}}\n' +
        '{{- if le (len $fullLine) 120 -}}\n' +
        '=== {{ .tuiTitle }}{{ if .notableVariables }}:{{ range .notableVariables }} {{ . }}={{ index $.stateKeys . }}{{ end }}{{ end }} ===\n' +
        '{{- else -}}\n' +
        '=== {{ .tuiTitle }} ===\n' +
        '{{- range .notableVariables }}\n' +
        '  {{ . }}={{ index $.stateKeys . }}\n' +
        '{{- end }}\n' +
        '{{- end }}\n' +
        'Type \'help\' for commands.');

    // Additional help text to append to the built-in help command
    const usageTemplate = config.usageTemplate ? template.new("usageTemplate") : null;
    if (usageTemplate) {
        usageTemplate.funcs(templateFuncs);
        usageTemplate.parse("\n" + config.usageTemplate);
    }

    // Build prompt from configuration
    function buildPrompt() {
        const templateData = buildBaseTemplateData(); // N.B. func defined below

        templateData.contextTxtar = buildContext(state.get(stateKeys.contextItems), {toTxtar: () => context.toTxtar()});

        // The promptInstructions string is itself a template. We must execute it
        // first using the template data we've just constructed to resolve any
        // dynamic values before injecting it into the main prompt template.
        const instructionsTmpl = template.new("instructions");
        // Ensure the instructions template has access to the same helper functions.
        instructionsTmpl.funcs(templateFuncs);
        instructionsTmpl.parse(config.promptInstructions || "");
        templateData.promptInstructions = instructionsTmpl.execute(templateData);

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
            description: config.description || "",
            contextHeader: config.contextHeader || "CONTEXT",
            stateKeys: {},
            promptOptions: config.promptOptions,
            tuiTitle,
            notableVariables: config.notableVariables || [],
        };

        for (const stringKey in stateKeys) {
            if (Object.hasOwn(stateKeys, stringKey)) {
                templateData.stateKeys[stringKey] = state.get(stateKeys[stringKey]);
            }
        }

        // Handle dynamic instruction substitutions from promptOptions.
        // This maps state keys to specific instructions based on the current state value.
        // E.g., if stateKeys.language is "ES", it looks for promptOptions.languageInstructions["ES"].
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

                // 4. Verify this state key exists in the built / extracted stateKeys.
                if (!Object.hasOwn(templateData.stateKeys, stateKey)) {
                    continue;
                }

                // 5. Get the current active value for this state (e.g., "EN", "dark_mode", etc.).
                const stateValue = templateData.stateKeys[stateKey];

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
                    // MUST be lowerCamelCase: e.g., typeInstructions
                    const targetKey = stateKey + instructionsSuffix;
                    templateData[targetKey] = instructionsValue;
                    // mapping applied
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
                try {
                    // Parse the handler string as a function expression
                    // Remove the "function (args)" wrapper if present and extract the body
                    let handlerCode = cmdConfig.handler.trim();

                    // If it's a function expression like "function (args) { ... }"
                    // we need to extract just the body
                    const funcMatch = handlerCode.match(/^function\s*\([^)]*\)\s*\{([\s\S]*)}$/);
                    if (funcMatch) {
                        handlerCode = funcMatch[1];
                    }

                    // Create function with access to necessary variables
                    // new Function is safer than eval as it only has access to global scope

                    // WARNING: This MUST be _lazily_ evaluated to avoid capturing stale state.
                    const getVars = (args) => [
                        ['args', args],
                        ['output', output],
                        ['tui', tui],
                        ['ctxmgr', ctxmgr],
                        ['state', state],
                        ['stateKeys', stateKeys],
                        ['buildPrompt', buildPrompt],
                    ];

                    const handler = new Function(
                        ...getVars().map(v => v[0]),
                        handlerCode);

                    function wrappedHandler(args) {
                        return handler.call(this, ...getVars(args).map(v => v[1]));
                    }

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
        if (usageTemplate) {
            const templateData = buildBaseTemplateData();
            const usageText = usageTemplate.execute(templateData);
            output.print(usageText);
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
            },
            onEnter: printBanner,
            commands: buildCommands,
        });
    });
})();
