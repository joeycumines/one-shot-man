'use strict';
// pr_split_09_agent.js — Agent Code Executor & prompt system
// Dependencies: chunks 00, 02 must be loaded first
// Late-binds: exec (00), template (00), detectGoModulePath (02), fileExtension (00), runtime (00)
//
// Exports: AgentCodeExecutor, renderPrompt, renderClassificationPrompt,
//          renderSplitPlanPrompt, renderConflictPrompt, detectLanguage,
//          CLASSIFICATION_PROMPT_TEMPLATE, SPLIT_PLAN_PROMPT_TEMPLATE,
//          CONFLICT_RESOLUTION_PROMPT_TEMPLATE
//
// Implementation uses osm:aimux (generic agent/process multiplexer),
// provider-agnostic and not tied to any specific LLM CLI.

(function(prSplit) {

    var aimux = require('osm:aimux');

    // --- AgentCodeExecutor ---

    function AgentCodeExecutor(config) {
        this.command = config.agentCommand || '';
        this.args = config.agentArgs || [];
        this.env = config.agentEnv || {};
        this.resolved = null;
        this.provider = '';
        this.handle = null;
        this.sessionId = null;
        // Optional TUIStateMachine integration for event-driven state tracking.
        // These are created in initEventTracking() after spawn succeeds.
        // If aimux does not provide the constructors (e.g. stripped test
        // builds), all remain null and the existing screenshot-based
        // detection and isAlive() polling continue to work unchanged.
        this.stateMachine = null;
        this.eventStream = null;
        this.healthMonitor = null;
        this._eventLoopRunning = false;
        this._eventLoopStop = false;
    }

    // runAsync runs a command via osm:exec spawn and returns buffered output.
    async function runAsync(exec, argv) {
        var child = await exec.spawn(argv[0], argv.slice(1));
        async function readAll(stream) {
            var buf = '';
            while (true) {
                var chunk = await stream.read();
                if (chunk.done) break;
                if (chunk.value !== undefined && chunk.value !== null) {
                    buf += String(chunk.value);
                }
            }
            return buf;
        }
        var results = await Promise.all([
            readAll(child.stdout),
            readAll(child.stderr),
            child.wait()
        ]);
        return {
            stdout: results[0],
            stderr: results[1],
            code: (results[2] && results[2].code !== undefined) ? results[2].code : 0
        };
    }

    // lookupBinary resolves a command on PATH. Prefers the pr-split helper if
    // available, otherwise falls back to osm:exec "which"/"where.exe".
    async function lookupBinary(exec, cmd) {
        if (typeof prSplit._lookupBinaryAsync === 'function') {
            return await prSplit._lookupBinaryAsync(cmd);
        }
        var isWin = typeof osmod !== 'undefined' && osmod &&
            typeof osmod.getenv === 'function' &&
            (osmod.getenv('OS') || osmod.getenv('GOOS') || '').indexOf('windows') !== -1;
        var argv = isWin ? ['where.exe', cmd] : ['which', cmd];
        var result = await runAsync(exec, argv);
        var path = (result.stdout || '').split('\n')[0].trim();
        return { found: result.code === 0 && path !== '', path: path };
    }

    // resolveAsync determines the provider binary, non-blocking.
    AgentCodeExecutor.prototype.resolveAsync = async function(progressFn) {
        var exec = prSplit._modules.exec;
        var self = this;
        function progress(msg) {
            if (progressFn) progressFn(msg);
        }

        if (self.command) {
            progress('Resolving binary: ' + self.command + '…');
            var check = await lookupBinary(exec, self.command);
            if (!check.found) {
                return { error: 'Agent command not found: ' + self.command };
            }
            self.resolved = { command: self.command, type: 'explicit' };
            return { error: null };
        }

        return {
            error: 'No agent command configured. Set --agent-command to the agent CLI executable.'
        };
    };

    // resolve synchronous fallback.
    AgentCodeExecutor.prototype.resolve = async function() {
        var exec = prSplit._modules.exec;
        var self = this;
        async function lookupSync(cmd) {
            if (typeof prSplit._lookupBinary === 'function') {
                return await prSplit._lookupBinary(cmd);
            }
            var result = await exec.execv(['which', cmd]);
            var path = (result.stdout || '').split('\n')[0].trim();
            return { found: result.code === 0 && path !== '', path: path };
        }

        if (self.command) {
            var check = await lookupSync(self.command);
            if (!check.found) {
                return { error: 'Agent command not found: ' + self.command };
            }
            self.resolved = { command: self.command, type: 'explicit' };
            return { error: null };
        }

        return {
            error: 'No agent command configured. Set --agent-command to the agent CLI executable.'
        };
    };

    // buildAgentArgv is the single source for the agent argv. userArgs come
    // first, then exactly one --mcp-config pair. defaultArgs is empty by
    // construction so provider concatenation (defaultArgs plus opts.Args at
    // provider_process.go:57) cannot duplicate the flag.
    // Matches both --mcp-config forms: bare ('--mcp-config', value in the
    // next argv element) and attached ('--mcp-config=/path'). Exact-element
    // counting alone would miss the attached form, letting a user-supplied
    // --mcp-config=x slip past the exactly-one assertion and produce a
    // duplicate MCP config pair at spawn.
    function isMcpConfigArg(arg) {
        return arg === '--mcp-config' || (typeof arg === 'string' && arg.indexOf('--mcp-config=') === 0);
    }

    function countMcpConfig(argv) {
        var n = 0;
        for (var i = 0; i < (argv || []).length; i++) {
            if (isMcpConfigArg(argv[i])) n++;
        }
        return n;
    }

    AgentCodeExecutor.prototype.buildAgentArgv = function(mcpConfigPath) {
        if (!mcpConfigPath) return { error: 'mcpConfigPath is required (provided by osm:mcpcallback)', argv: null };
        var userArgs = this.args || [];
        for (var i = 0; i < userArgs.length; i++) {
            if (isMcpConfigArg(userArgs[i])) {
                return { error: 'agent argv must contain exactly one --mcp-config (remove --mcp-config from --agent-arg; it is managed)', argv: null };
            }
        }
        var argv = userArgs.concat(['--mcp-config', mcpConfigPath]);
        if (countMcpConfig(argv) !== 1) {
            return { error: 'agent argv must contain exactly one --mcp-config', argv: null };
        }
        return { error: null, argv: argv };
    };

    function commandBaseName(command) {
        var value = String(command || '').replace(/\\/g, '/');
        var slash = value.lastIndexOf('/');
        return slash >= 0 ? value.substring(slash + 1) : value;
    }

    function isOpenCodeCommand(command) {
        var name = commandBaseName(command).toLowerCase();
        return name === 'opencode' || name === 'opencode.exe';
    }

    async function buildOpenCodeConfigContent(mcpConfigPath, existingContent) {
        var osmod = prSplit._modules && prSplit._modules.osmod;
        if (!osmod || typeof osmod.readFile !== 'function') {
            return { error: 'Unable to read MCP config for opencode: osm:os module is unavailable' };
        }
        var readResult;
        try {
            readResult = await osmod.readFile(mcpConfigPath);
        } catch (e) {
            return { error: 'Unable to read MCP config for opencode: ' + (e.message || String(e)) };
        }
        if (!readResult || readResult.error || typeof readResult.content !== 'string') {
            return {
                error: 'Unable to read MCP config for opencode: ' +
                    ((readResult && readResult.error) || 'empty config')
            };
        }

        var source;
        try {
            source = JSON.parse(readResult.content);
        } catch (e) {
            return { error: 'Invalid MCP config for opencode: ' + (e.message || String(e)) };
        }
        var legacyServers = source && source.mcpServers;
        var callback = legacyServers && legacyServers['osm-callback'];
        if (!callback || (typeof callback.command !== 'string' && !Array.isArray(callback.command))) {
            return { error: 'MCP config for opencode is missing the osm-callback command' };
        }
        var command = Array.isArray(callback.command)
            ? callback.command.slice()
            : [callback.command];
        if (Array.isArray(callback.args)) {
            command = command.concat(callback.args);
        }
        if (command.length === 0 || command[0] === '') {
            return { error: 'MCP config for opencode is missing the osm-callback command' };
        }

        var config = {};
        if (existingContent !== undefined && existingContent !== null && existingContent !== '') {
            try {
                config = JSON.parse(existingContent);
            } catch (e) {
                return { error: 'Invalid OPENCODE_CONFIG_CONTENT: ' + (e.message || String(e)) };
            }
            if (!config || typeof config !== 'object' || Array.isArray(config)) {
                return { error: 'OPENCODE_CONFIG_CONTENT must be a JSON object' };
            }
        }
        if (!config.mcp) config.mcp = {};
        if (typeof config.mcp !== 'object' || Array.isArray(config.mcp)) {
            return { error: 'OPENCODE_CONFIG_CONTENT.mcp must be a JSON object' };
        }
        config.mcp['osm-callback'] = {
            type: 'local',
            command: command
        };
        return { error: null, content: JSON.stringify(config) };
    }

    AgentCodeExecutor.prototype.buildAgentInvocation = async function(mcpConfigPath) {
        if (!this.resolved || !this.resolved.command) {
            return { error: 'Agent command must be resolved before building its invocation' };
        }

        if (!isOpenCodeCommand(this.resolved.command)) {
            var generic = this.buildAgentArgv(mcpConfigPath);
            return {
                error: generic.error,
                argv: generic.argv,
                env: this.env || {},
                provider: 'generic'
            };
        }

        var userArgs = this.args || [];
        for (var i = 0; i < userArgs.length; i++) {
            if (isMcpConfigArg(userArgs[i])) {
                return { error: 'agent argv must contain exactly one managed MCP configuration (remove --mcp-config from --agent-arg)' };
            }
        }
        var existingContent = this.env && this.env.OPENCODE_CONFIG_CONTENT;
        var configResult = await buildOpenCodeConfigContent(mcpConfigPath, existingContent);
        if (configResult.error) return { error: configResult.error };

        var env = {};
        var configuredEnv = this.env || {};
        for (var key in configuredEnv) {
            if (Object.prototype.hasOwnProperty.call(configuredEnv, key)) {
                env[key] = configuredEnv[key];
            }
        }
        env.OPENCODE_CONFIG_CONTENT = configResult.content;
        return {
            error: null,
            argv: userArgs.slice(),
            env: env,
            provider: 'opencode'
        };
    };

    // spawn creates a process-backed agent handle through osm:aimux.
    AgentCodeExecutor.prototype.spawn = async function(sessionId, opts) {
        var exec = prSplit._modules.exec;
        opts = opts || {};

        var resolveResult = await this.resolveAsync();
        if (resolveResult.error) {
            return { error: resolveResult.error };
        }

        this.sessionId = sessionId || ('prsplit-' + Date.now());

        if (!opts.mcpConfigPath) {
            return { error: 'mcpConfigPath is required (provided by osm:mcpcallback)' };
        }

        var invocation = await this.buildAgentInvocation(opts.mcpConfigPath);
        if (invocation.error) {
            return { error: invocation.error };
        }
        this.provider = invocation.provider || 'generic';
        var baseArgs = invocation.argv;
        var provider = aimux.processProvider({
            name: 'agent',
            command: this.resolved.command,
            defaultArgs: [],
            capabilities: { mcp: true, streaming: true, multiTurn: true, resizable: true }
        });

        var spawnOpts = {
            args: baseArgs,
            env: invocation.env
        };
        if (typeof tuiMux !== 'undefined' && tuiMux && typeof tuiMux.termSize === 'function') {
            var sz = tuiMux.termSize();
            if (sz && sz.rows > 0 && sz.cols > 0) {
                spawnOpts.rows = sz.rows;
                spawnOpts.cols = sz.cols;
            }
        }

        var cmdDesc = this.resolved.command;
        if (baseArgs.length > 0) {
            cmdDesc += ' ' + baseArgs.join(' ');
        }

        // Generic providers receive one managed --mcp-config pair. Opencode
        // receives the same callback through OPENCODE_CONFIG_CONTENT instead.
        if (invocation.provider === 'generic' && countMcpConfig(baseArgs) !== 1) {
            return { error: 'agent argv must contain exactly one --mcp-config at provider boundary' };
        }

        try {
            var registry = aimux.newRegistry();
            registry.register(provider);
            this.handle = await registry.spawn('agent', spawnOpts);
        } catch (e) {
            return {
                error: 'Agent spawn failed: ' + (e.message || String(e)) +
                       '\n  Command attempted: ' + cmdDesc +
                       '\n  Provider type: ' + this.resolved.type
            };
        }

        log.printf('Agent executor: spawned command=%s type=%s session=%s args=%s',
            this.resolved.command, this.resolved.type, this.sessionId,
            JSON.stringify(baseArgs));

        // Post-spawn health check.
        if (this.handle && typeof this.handle.isAlive === 'function') {
            var healthCheckDelayMs = (prSplit.AUTOMATED_DEFAULTS && typeof prSplit.AUTOMATED_DEFAULTS.spawnHealthCheckDelayMs === 'number') ? prSplit.AUTOMATED_DEFAULTS.spawnHealthCheckDelayMs : 300;
            await new Promise(function(resolve) { setTimeout(resolve, healthCheckDelayMs); });
            if (!this.handle.isAlive()) {
                var lastOutput = '';
                if (typeof this.handle.receiveAsync === 'function') {
                    try {
                        var chunk = await this.handle.receiveAsync();
                        if (chunk) { lastOutput = chunk; }
                    } catch (readErr) { log.debug('drain: read failed (expected for dead process): ' + (readErr.message || readErr)); }
                }
                try { await this.handle.close(); } catch (closeErr) { log.debug('drain handle close failed', { error: closeErr.message || String(closeErr) }); }
                this.handle = null;

                var diagnostic = 'Agent process exited immediately after spawn.';
                if (lastOutput) {
                    diagnostic += '\n  Process output: ' + lastOutput.trim().substring(0, 500);
                }
                diagnostic += '\n  Command: ' + cmdDesc;
                diagnostic += '\n  Provider: ' + this.resolved.type;
                return { error: diagnostic };
            }
        }

        this.initEventTracking();

        return { error: null, sessionId: this.sessionId, argv: baseArgs };
    };

    // initEventTracking creates the optional TUIStateMachine, EventStream,
    // and HealthMonitor after a successful spawn. Each is created
    // independently — if one fails, the others are still attempted.
    // All are optional: if aimux does not expose the constructor or the
    // handle lacks the required methods, the field stays null and the
    // existing screenshot-based detection and isAlive() polling continue.
    AgentCodeExecutor.prototype.initEventTracking = function() {
        try {
            if (typeof aimux.newTUIStateMachine === 'function') {
                this.stateMachine = aimux.newTUIStateMachine();
            }
        } catch (e) {
            log.debug('initEventTracking: stateMachine creation failed', { error: e.message || String(e) });
            this.stateMachine = null;
        }

        try {
            if (typeof aimux.newEventStream === 'function' && this.handle) {
                this.eventStream = aimux.newEventStream(this.handle, aimux.newParser());
            }
        } catch (e) {
            log.debug('initEventTracking: eventStream creation failed', { error: e.message || String(e) });
            this.eventStream = null;
        }

        try {
            if (typeof aimux.newHealthMonitor === 'function' && this.handle) {
                this.healthMonitor = aimux.newHealthMonitor(this.handle, 5000);
            }
        } catch (e) {
            log.debug('initEventTracking: healthMonitor creation failed', { error: e.message || String(e) });
            this.healthMonitor = null;
        }

        this.startEventLoop();
    };

    // startEventLoop reads output lines from the handle via
    // receiveEventAsync and feeds each line to the state machine's
    // processOutput method. The loop is Promise-based and non-blocking:
    // each iteration awaits a single line, then schedules the next via
    // setTimeout(0) to yield to the JS event loop. The loop stops when
    // stopEventLoop is called, the handle dies, or receiveEventAsync
    // resolves null (EOF / unsupported).
    AgentCodeExecutor.prototype.startEventLoop = function() {
        if (this._eventLoopRunning) return;
        if (!this.stateMachine || !this.handle) return;
        if (typeof this.handle.receiveEventAsync !== 'function') return;

        this._eventLoopRunning = true;
        this._eventLoopStop = false;

        var self = this;
        var sm = this.stateMachine;

        function loop() {
            if (self._eventLoopStop || !self.handle) {
                self._eventLoopRunning = false;
                return;
            }
            if (typeof self.handle.isAlive === 'function' && !self.handle.isAlive()) {
                self._eventLoopRunning = false;
                return;
            }

            self.handle.receiveEventAsync().then(function(line) {
                if (self._eventLoopStop || !self.handle) {
                    self._eventLoopRunning = false;
                    return;
                }
                if (line === null || line === undefined) {
                    self._eventLoopRunning = false;
                    return;
                }
                if (line !== '') {
                    try {
                        sm.processOutput(line);
                    } catch (e) {
                        log.debug('eventLoop: processOutput failed', { error: e.message || String(e) });
                    }
                }
                setTimeout(loop, 0);
            }).catch(function(err) {
                log.debug('eventLoop: receiveEventAsync error', { error: err.message || String(err) });
                self._eventLoopRunning = false;
            });
        }

        loop();
    };

    AgentCodeExecutor.prototype.stopEventLoop = function() {
        this._eventLoopStop = true;
        this._eventLoopRunning = false;
    };

    AgentCodeExecutor.prototype.isAvailable = async function() {
        if (this.resolved) return true;
        var result = await this.resolve();
        return !result.error;
    };

    AgentCodeExecutor.prototype.isAvailableAsync = async function() {
        if (this.resolved) return true;
        var result = await this.resolveAsync();
        return !result.error;
    };

    AgentCodeExecutor.prototype.close = async function() {
        this.stopEventLoop();

        if (this.eventStream && typeof this.eventStream.close === 'function') {
            try { this.eventStream.close(); } catch (e) { log.debug('close eventstream close failed', { error: e.message || String(e) }); }
        }
        this.eventStream = null;

        if (this.healthMonitor && typeof this.healthMonitor.close === 'function') {
            try { this.healthMonitor.close(); } catch (e) { log.debug('close healthmonitor close failed', { error: e.message || String(e) }); }
        }
        this.healthMonitor = null;

        this.stateMachine = null;

        if (this.handle && typeof this.handle.close === 'function') {
            try { await this.handle.close(); } catch (e) { log.debug('close handle close failed', { error: e.message || String(e) }); }
        }
        this.handle = null;
        this.sessionId = null;
        this.resolved = null;
    };

    AgentCodeExecutor.prototype.captureDiagnostic = async function() {
        if (!this.handle) return '';
        var output = '';
        if (typeof this.handle.receiveAsync === 'function') {
            try {
                var chunk = await this.handle.receiveAsync();
                if (chunk) { output = chunk; }
            } catch (e) { log.debug('captureDiagnostic: read failed: ' + (e.message || e)); }
        }
        return output;
    };

    AgentCodeExecutor.prototype.restart = async function(sessionId, opts) {
        log.printf('AgentCodeExecutor.restart: closing existing session');
        await this.close();
        var resolveResult = await this.resolveAsync();
        if (resolveResult.error) {
            return { error: 'restart resolve failed: ' + resolveResult.error };
        }
        log.printf('AgentCodeExecutor.restart: spawning new session');
        return await this.spawn(sessionId, opts);
    };

    AgentCodeExecutor.prototype.kill = async function() {
        this.stopEventLoop();

        if (this.eventStream && typeof this.eventStream.close === 'function') {
            try { this.eventStream.close(); } catch (e) { log.debug('kill eventstream close failed', { error: e.message || String(e) }); }
        }
        this.eventStream = null;

        if (this.healthMonitor && typeof this.healthMonitor.close === 'function') {
            try { this.healthMonitor.close(); } catch (e) { log.debug('kill healthmonitor close failed', { error: e.message || String(e) }); }
        }
        this.healthMonitor = null;

        this.stateMachine = null;

        if (this.handle && typeof this.handle.close === 'function') {
            try { await this.handle.close(); } catch (e) { log.debug('kill handle close failed', { error: e.message || String(e) }); }
        }
        this.handle = null;
        this.resolved = null;
    };

    // --- Prompt Templates ---

    var CLASSIFICATION_PROMPT_TEMPLATE =
        'You are a solo staff engineer acting as a weaver at a loom (Commit-Loom methodology), ' +
        'responsible for splitting a large pull request into an ordered sequence of maintainer-grade, ' +
        'self-contained stacked PRs that a strict reviewer would happily merge one at a time.\n\n' +
        'The repository uses {{.Language}}' +
        '{{if .ModulePath}} with module path `{{.ModulePath}}`{{end}}.\n' +
        'The base branch is `{{.BaseBranch}}`.\n\n' +
        '## Changed Files\n\n' +
        'The following files have been modified (status: A=added, M=modified, D=deleted, R=renamed):\n\n' +
        '{{range $path, $status := .FileStatuses}}' +
        '- `{{$path}}` ({{$status}})\n' +
        '{{end}}\n' +
        '## Commit-Loom Principles\n\n' +
        '1. **Self-Contained Units over Atomic Micro-Splits**: Group tightly coupled changes together. ' +
        'A PR should be the largest coherent unit a strict reviewer can evaluate in one sitting. ' +
        'Rolling up interdependent changes into a single PR avoids intermediate compilation failures, ' +
        'broken tests, and artificial temporary shims. Never split coupled changes merely to achieve "atomic" commits.\n' +
        '2. **Strict Dependency Layering**:\n' +
        '   - Layer 1 (Foundations & Models): Core types, data schemas, migrations, configuration primitives.\n' +
        '   - Layer 2 (Mechanics & Domain Logic): Core algorithms, domain services, internal packages building on Layer 1.\n' +
        '   - Layer 3 (Callers & User Interfaces): Public APIs, CLI commands, HTTP handlers, TUI components exposing Layer 2.\n' +
        '   - Layer 4 (Validation & Documentation): Integration/E2E tests, documentation, examples.\n' +
        '   Earlier layers must never depend on later layers.\n' +
        '3. **Independent Self-Sufficiency**: Every PR in the stack must build, pass linters, and pass tests cleanly.\n\n' +
        '{{if gt .MaxGroups 0}}Use at most {{.MaxGroups}} groups.{{end}}\n\n' +
        '## Output Format\n\n' +
        'Use the `reportClassification` MCP tool to report your results. ' +
        'The `categories` parameter is an array of category objects. Each category has:\n' +
        '- `name`: Short identifier for the group (e.g., "01-foundation-models", "02-auth-core")\n' +
        '- `description`: Git commit message for the split branch. This MUST be specific to the actual code changes — not generic.\n' +
        '- `files`: Array of file paths belonging to this category\n' +
        '- `title`: (Optional) Imperative PR title in conventional commit format (e.g., "feat(auth): implement token validation")\n' +
        '- `summary`: (Optional) Architectural summary of why this PR exists and its role in the overall change\n' +
        '- `keyChanges`: (Optional) Array of bullet points highlighting key decisions and notable changes\n' +
        '- `verificationSteps`: (Optional) Command or instructions to verify this layer in isolation (e.g., "gmake test")\n' +
        '- `rationale`: (Optional) Why this PR is self-contained and why it is placed at this layer in the stack\n\n' +
        '### Commit Message Requirements\n\n' +
        'Each category description becomes the git commit message for that split branch. Follow these rules:\n' +
        '- Be specific: "Add user authentication middleware" not "misc changes"\n' +
        '- Reference what changed: mention the package, module, or feature area\n' +
        '- No placeholder messages like "various updates", "cleanup", or "other changes"\n' +
        '- No catch-all categories unless absolutely necessary (prefer specific groupings)\n' +
        '- If the project uses conventional commits, follow that style\n\n' +
        'Also assess which groups are independent (can be merged in any order). ' +
        'If any groups can merge independently, mention this in your response.\n';

    var SPLIT_PLAN_PROMPT_TEMPLATE =
        'You are a solo staff engineer ordering a series of stacked pull requests following Commit-Loom discipline.\n' +
        'Based on the file classification below, create an ordered split plan for native GitHub Stacked PRs.\n\n' +
        '## Classification\n\n' +
        '{{range $path, $category := .Classification}}' +
        '- `{{$path}}` → {{$category}}\n' +
        '{{end}}\n' +
        '## Constraints\n\n' +
        '- Branch prefix: `{{.BranchPrefix}}`\n' +
        '{{if gt .MaxFilesPerSplit 0}}- Maximum {{.MaxFilesPerSplit}} files per split\n{{end}}' +
        '{{if .PreferIndependent}}- Prefer independently mergeable splits when possible\n{{end}}\n' +
        '## Commit-Loom Planning Principles\n\n' +
        '1. **Self-Contained Units over Atomic Micro-Splits**: Group tightly coupled changes together. Each PR must be a coherent, reviewable unit that can be understood and merged without requiring broken intermediate states.\n' +
        '2. **Strict Dependency Layering**: Order from foundational layers (models, schemas, types) to domain mechanics, then caller surfaces/APIs, and finally integration tests and docs. Earlier stages must not depend on later stages.\n' +
        '3. **Independent Self-Sufficiency**: Each stage when stacked must build, pass linters, and pass tests cleanly.\n' +
        '4. **Stacked PR Visibility**: Each stage will become a layer in a native GitHub Stacked PR chain with dedicated Stack Map navigation.\n\n' +
        '## Output Format\n\n' +
        'Use the `reportSplitPlan` MCP tool. Each stage in the `stages` array needs:\n' +
        '- `name`: Branch name suffix (e.g., "01-models", "02-auth")\n' +
        '- `files`: Array of file paths in this split\n' +
        '- `message`: Git commit message (imperative sentence)\n' +
        '- `order`: 0-based execution order\n' +
        '- `title`: (Optional) Imperative PR title\n' +
        '- `summary`: (Optional) Architectural summary of this PR layer\n' +
        '- `keyChanges`: (Optional) Array of key changes / bullet points\n' +
        '- `verificationSteps`: (Optional) Test/build commands to independently verify this layer\n' +
        '- `rationale`: (Optional) Layering rationale and why it is self-contained\n';

    var CONFLICT_RESOLUTION_PROMPT_TEMPLATE =
        'A split branch failed verification. Help fix it.\n\n' +
        '## Branch: `{{.BranchName}}`\n\n' +
        '### Files in this branch\n' +
        '{{range .Files}}- `{{.}}`\n{{end}}\n' +
        '### Verification Error (exit code {{.ExitCode}})\n\n' +
        '```\n{{.ErrorOutput}}\n```\n\n' +
        '{{if .GoModContent}}### go.mod content\n\n```\n{{.GoModContent}}\n```\n\n{{end}}' +
        '## Task\n\n' +
        'Analyze the error and propose a fix using the `reportResolution` MCP tool ' +
        'for branch `{{.BranchName}}`.\n\n' +
        'You can suggest:\n' +
        '- File patches (full file content replacements)\n' +
        '- Commands to run (e.g., `go mod tidy`)\n' +
        '- If the split is fundamentally broken, set `reSplitSuggested: true` ' +
        'with a reason explaining which files conflict\n' +
        '- If this failure also exists on the base branch (pre-existing), set ' +
        '`preExistingFailure: true` with `preExistingDetails` explaining the issue\n';

    function renderPrompt(tmplStr, data) {
        var template = prSplit._modules.template;
        if (!template) {
            return { text: '', error: 'osm:text/template module not available' };
        }
        try {
            var text = template.execute(tmplStr, data);
            return { text: text, error: null };
        } catch (e) {
            return { text: '', error: 'template render failed: ' + (e.message || String(e)) };
        }
    }

    function renderClassificationPrompt(analysis, config) {
        config = config || {};
        var detectGoModulePath = prSplit.detectGoModulePath;
        var runtime = prSplit.runtime;
        var modulePath = detectGoModulePath ? detectGoModulePath() : '';
        var language = modulePath ? 'Go' : detectLanguage(analysis.files);
        return renderPrompt(CLASSIFICATION_PROMPT_TEMPLATE, {
            Language: language,
            ModulePath: modulePath,
            BaseBranch: analysis.baseBranch || runtime.baseBranch,
            FileStatuses: analysis.fileStatuses || {},
            MaxGroups: config.maxGroups || 0
        });
    }

    function renderSplitPlanPrompt(classification, config) {
        config = config || {};
        var runtime = prSplit.runtime;
        return renderPrompt(SPLIT_PLAN_PROMPT_TEMPLATE, {
            Classification: classification,
            BranchPrefix: config.branchPrefix || runtime.branchPrefix || 'split/',
            MaxFilesPerSplit: typeof config.maxFilesPerSplit === 'number' ? config.maxFilesPerSplit : (typeof runtime.maxFiles === 'number' ? runtime.maxFiles : 0),
            PreferIndependent: config.preferIndependent || false
        });
    }

    function renderConflictPrompt(conflict) {
        return renderPrompt(CONFLICT_RESOLUTION_PROMPT_TEMPLATE, {
            BranchName: conflict.branchName || '',
            Files: conflict.files || [],
            ExitCode: typeof conflict.exitCode === 'number' ? conflict.exitCode : 1,
            ErrorOutput: conflict.errorOutput || '',
            GoModContent: conflict.goModContent || ''
        });
    }

    function detectLanguage(files) {
        var fileExtension = prSplit._fileExtension;
        var counts = {};
        var langMap = {
            '.go': 'Go', '.js': 'JavaScript', '.ts': 'TypeScript',
            '.jsx': 'JavaScript', '.tsx': 'TypeScript',
            '.mjs': 'JavaScript', '.cjs': 'JavaScript',
            '.py': 'Python', '.rb': 'Ruby', '.rs': 'Rust',
            '.java': 'Java', '.c': 'C', '.cpp': 'C++',
            '.cc': 'C++', '.cxx': 'C++', '.h': 'C', '.hpp': 'C++',
            '.cs': 'C#', '.swift': 'Swift', '.kt': 'Kotlin',
            '.m': 'Objective-C', '.mm': 'Objective-C++',
            '.vue': 'Vue', '.svelte': 'Svelte', '.dart': 'Dart',
            '.php': 'PHP', '.scala': 'Scala', '.zig': 'Zig',
            '.lua': 'Lua', '.r': 'R', '.R': 'R',
            '.pl': 'Perl', '.pm': 'Perl', '.ex': 'Elixir',
            '.exs': 'Elixir', '.clj': 'Clojure', '.hs': 'Haskell',
            '.ml': 'OCaml', '.fs': 'F#', '.sol': 'Solidity',
            '.tf': 'Terraform', '.nix': 'Nix', '.sh': 'Shell',
            '.bash': 'Shell', '.zsh': 'Shell',
            '.html': 'HTML', '.htm': 'HTML',
            '.css': 'CSS', '.scss': 'SCSS', '.sass': 'Sass', '.less': 'Less'
        };
        var skipExts = {
            '.md': true, '.txt': true, '.rst': true,
            '.json': true, '.yaml': true, '.yml': true, '.toml': true,
            '.xml': true, '.csv': true, '.lock': true, '.sum': true,
            '.cfg': true, '.ini': true, '.env': true, '.conf': true,
            '.svg': true, '.png': true, '.jpg': true, '.jpeg': true,
            '.gif': true, '.ico': true, '.woff': true, '.woff2': true,
            '.eot': true, '.ttf': true, '.otf': true
        };
        var extCounts = {};
        for (var i = 0; i < (files || []).length; i++) {
            var ext = fileExtension(files[i]);
            if (!ext) continue;
            var lang = langMap[ext];
            if (lang) {
                counts[lang] = (counts[lang] || 0) + 1;
            } else if (!skipExts[ext]) {
                extCounts[ext] = (extCounts[ext] || 0) + 1;
            }
        }
        var best = '';
        var bestCount = 0;
        for (var k in counts) {
            if (counts[k] > bestCount) {
                best = k;
                bestCount = counts[k];
            }
        }
        if (!best) {
            var bestExt = '';
            var bestExtCount = 0;
            for (var e in extCounts) {
                if (extCounts[e] > bestExtCount) {
                    bestExt = e;
                    bestExtCount = extCounts[e];
                }
            }
            if (bestExt) {
                var raw = bestExt.replace(/^\./, '');
                best = raw.charAt(0).toUpperCase() + raw.slice(1);
            }
        }
        return best || 'unknown';
    }

    // --- Exports ---

    prSplit.AgentCodeExecutor = AgentCodeExecutor;
    prSplit.renderPrompt = renderPrompt;
    prSplit.renderClassificationPrompt = renderClassificationPrompt;
    prSplit.renderSplitPlanPrompt = renderSplitPlanPrompt;
    prSplit.renderConflictPrompt = renderConflictPrompt;
    prSplit.detectLanguage = detectLanguage;
    prSplit.CLASSIFICATION_PROMPT_TEMPLATE = CLASSIFICATION_PROMPT_TEMPLATE;
    prSplit.SPLIT_PLAN_PROMPT_TEMPLATE = SPLIT_PLAN_PROMPT_TEMPLATE;
    prSplit.CONFLICT_RESOLUTION_PROMPT_TEMPLATE = CONFLICT_RESOLUTION_PROMPT_TEMPLATE;

})(globalThis.prSplit);
