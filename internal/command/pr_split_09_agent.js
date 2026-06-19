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
        this.model = config.agentModel || '';
        this.configDir = config.agentConfigDir || '';
        this.env = config.agentEnv || {};
        this.resolved = null;
        this.handle = null;
        this.sessionId = null;
        this.cm = aimux;
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
        var child = exec.spawn(argv[0], argv.slice(1));
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

        var provider;
        var baseArgs = (this.args || []).concat(['--mcp-config', opts.mcpConfigPath]);
        var providerName;
        if (this.resolved.type === 'agent' || this.resolved.type === 'agent-code' || this.resolved.type === 'explicit') {
            providerName = 'agent';
            provider = aimux.processProvider({
                name: providerName,
                command: this.resolved.command,
                defaultArgs: baseArgs,
                capabilities: { mcp: true, streaming: true, multiTurn: true, resizable: true }
            });
        } else if (this.resolved.type === 'ollama') {
            providerName = 'ollama';
            provider = aimux.processProvider({
                name: providerName,
                command: this.resolved.command,
                defaultArgs: baseArgs,
                capabilities: { mcp: true, streaming: true, multiTurn: true, resizable: true }
            });
        } else {
            return { error: 'unknown provider type: ' + this.resolved.type };
        }

        var spawnOpts = {
            args: baseArgs,
            env: this.env || {},
            model: this.model || undefined
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

        try {
            var registry = aimux.newRegistry();
            registry.register(provider);
            this.handle = registry.spawn(providerName, spawnOpts);
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
                if (typeof this.handle.receive === 'function') {
                    try {
                        var chunk = this.handle.receive();
                        if (chunk) { lastOutput = chunk; }
                    } catch (readErr) { log.debug('drain: read failed (expected for dead process): ' + (readErr.message || readErr)); }
                }
                try { this.handle.close(); } catch (closeErr) { log.debug('drain: handle.close failed: ' + (closeErr.message || closeErr)); }
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

        return { error: null, sessionId: this.sessionId };
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
                this.eventStream = aimux.newEventStream(this.handle, parser);
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

    AgentCodeExecutor.prototype.close = function() {
        this.stopEventLoop();

        if (this.eventStream && typeof this.eventStream.close === 'function') {
            try { this.eventStream.close(); } catch (e) { log.debug('close: eventStream.close failed', { error: e.message || String(e) }); }
        }
        this.eventStream = null;

        if (this.healthMonitor && typeof this.healthMonitor.close === 'function') {
            try { this.healthMonitor.close(); } catch (e) { log.debug('close: healthMonitor.close failed', { error: e.message || String(e) }); }
        }
        this.healthMonitor = null;

        this.stateMachine = null;

        if (this.handle && typeof this.handle.close === 'function') {
            try { this.handle.close(); } catch (e) { log.debug('close: handle.close failed', { error: e.message || String(e) }); }
        }
        this.handle = null;
        this.sessionId = null;
        this.resolved = null;
    };

    AgentCodeExecutor.prototype.captureDiagnostic = function() {
        if (!this.handle) return '';
        var output = '';
        if (typeof this.handle.receive === 'function') {
            try {
                var chunk = this.handle.receive();
                if (chunk) { output = chunk; }
            } catch (e) { log.debug('captureDiagnostic: read failed: ' + (e.message || e)); }
        }
        return output;
    };

    AgentCodeExecutor.prototype.restart = async function(sessionId, opts) {
        log.printf('AgentCodeExecutor.restart: closing existing session');
        this.close();
        var resolveResult = await this.resolveAsync();
        if (resolveResult.error) {
            return { error: 'restart resolve failed: ' + resolveResult.error };
        }
        log.printf('AgentCodeExecutor.restart: spawning new session');
        return await this.spawn(sessionId, opts);
    };

    AgentCodeExecutor.prototype.kill = function() {
        this.stopEventLoop();

        if (this.eventStream && typeof this.eventStream.close === 'function') {
            try { this.eventStream.close(); } catch (e) { log.debug('kill: eventStream.close failed', { error: e.message || String(e) }); }
        }
        this.eventStream = null;

        if (this.healthMonitor && typeof this.healthMonitor.close === 'function') {
            try { this.healthMonitor.close(); } catch (e) { log.debug('kill: healthMonitor.close failed', { error: e.message || String(e) }); }
        }
        this.healthMonitor = null;

        this.stateMachine = null;

        if (this.handle && typeof this.handle.close === 'function') {
            try { this.handle.close(); } catch (e) { log.debug('kill: handle.close failed', { error: e.message || String(e) }); }
        }
        this.handle = null;
        this.resolved = null;
    };

    // --- Ollama / model menu helpers (aimux does not provide provider-specific UI) ---

    var parser = aimux.newParser();

    function parseModelMenu(lines) {
        var models = [];
        var selected = null;
        for (var i = 0; i < (lines || []).length; i++) {
            var ev = parser.parse(lines[i]);
            if (ev.type === aimux.EVENT_MODEL_SELECT) {
                var itemEv = parser.parse(lines[i]);
                var fields = itemEv.fields || {};
                var name = fields.modelName || lines[i].replace(/^\s*[❯>]\s+/, '').trim();
                var sel = fields.selected === 'true';
                models.push(name);
                if (sel) selected = name;
            }
        }
        return { models: models, selected: selected, lines: lines };
    }

    function isLauncherMenu(menu) {
        if (!menu || !menu.models || menu.models.length === 0) return false;
        return menu.models.length > 3 && !menu.selected;
    }

    function navigateToModel(menu, target) {
        if (!menu || !menu.models || menu.models.length === 0) return null;
        var idx = menu.models.indexOf(target);
        if (idx < 0) {
            for (var i = 0; i < menu.models.length; i++) {
                if (menu.models[i].indexOf(target) !== -1) {
                    idx = i;
                    break;
                }
            }
        }
        if (idx < 0) return null;
        var current = menu.models.indexOf(menu.selected);
        if (current < 0) current = 0;
        var steps = idx - current;
        var keys = '';
        var key = steps < 0 ? '\x1b[A' : '\x1b[B';
        for (var s = 0; s < Math.abs(steps); s++) {
            keys += key;
        }
        keys += '\r';
        return keys;
    }

    function dismissLauncherKeys(menu) {
        if (!isLauncherMenu(menu)) return null;
        // Ollama launchers typically quit with 'q' and accept Enter.
        return 'q\r';
    }

    // Attach helpers to the module-style object the pipeline expects.
    var cmProxy = {
        newParser: function() { return aimux.newParser(); },
        eventTypeName: aimux.eventTypeName,
        parseModelMenu: parseModelMenu,
        isLauncherMenu: isLauncherMenu,
        navigateToModel: navigateToModel,
        dismissLauncherKeys: dismissLauncherKeys
    };
    // Expose constants on the proxy for backwards-compatible tests.
    cmProxy.EVENT_TEXT = aimux.EVENT_TEXT;
    cmProxy.EVENT_RATE_LIMIT = aimux.EVENT_RATE_LIMIT;
    cmProxy.EVENT_PERMISSION = aimux.EVENT_PERMISSION;
    cmProxy.EVENT_MODEL_SELECT = aimux.EVENT_MODEL_SELECT;
    cmProxy.EVENT_SSO_LOGIN = aimux.EVENT_SSO_LOGIN;
    cmProxy.EVENT_COMPLETION = aimux.EVENT_COMPLETION;
    cmProxy.EVENT_TOOL_USE = aimux.EVENT_TOOL_USE;
    cmProxy.EVENT_ERROR = aimux.EVENT_ERROR;
    cmProxy.EVENT_THINKING = aimux.EVENT_THINKING;

    // --- Prompt Templates ---

    var CLASSIFICATION_PROMPT_TEMPLATE =
        'You are a code reviewer helping split a large pull request into smaller, ' +
        'reviewable stacked PRs.\n\n' +
        'The repository uses {{.Language}}' +
        '{{if .ModulePath}} with module path `{{.ModulePath}}`{{end}}.\n' +
        'The base branch is `{{.BaseBranch}}`.\n\n' +
        '## Changed Files\n\n' +
        'The following files have been modified (status: A=added, M=modified, D=deleted, R=renamed):\n\n' +
        '{{range $path, $status := .FileStatuses}}' +
        '- `{{$path}}` ({{$status}})\n' +
        '{{end}}\n' +
        '## Task\n\n' +
        'Classify each file into a logical group for PR splitting. Group related changes together:\n' +
        '- Files in the same package/module that are tightly coupled\n' +
        '- Test files with the code they test\n' +
        '- Documentation with the features they document\n' +
        '- Refactoring changes separate from feature additions\n' +
        '- Infrastructure/config changes separate from application code\n\n' +
        '{{if gt .MaxGroups 0}}Use at most {{.MaxGroups}} groups.{{end}}\n\n' +
        '## Output Format\n\n' +
        'Use the `reportClassification` MCP tool to report your results. ' +
        'The `categories` parameter is an array of category objects. Each category has:\n' +
        '- `name`: Short identifier for the group (e.g., "types", "impl", "docs")\n' +
        '- `description`: Git commit message for the split branch. This MUST be specific to the actual code changes — not generic.\n' +
        '- `files`: Array of file paths belonging to this category\n\n' +
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
        'Based on the file classification below, create an ordered split plan for stacked PRs.\n\n' +
        '## Classification\n\n' +
        '{{range $path, $category := .Classification}}' +
        '- `{{$path}}` → {{$category}}\n' +
        '{{end}}\n' +
        '## Constraints\n\n' +
        '- Branch prefix: `{{.BranchPrefix}}`\n' +
        '{{if gt .MaxFilesPerSplit 0}}- Maximum {{.MaxFilesPerSplit}} files per split\n{{end}}' +
        '{{if .PreferIndependent}}- Prefer independently mergeable splits when possible\n{{end}}\n' +
        '## Task\n\n' +
        'Create an ordered plan where:\n' +
        '1. Each stage is a coherent, reviewable unit\n' +
        '2. Earlier stages should be foundations that later stages build on\n' +
        '3. Minimize cross-stage dependencies to reduce merge conflicts\n' +
        '4. Each stage should build and pass tests independently (when stacked)\n\n' +
        'Use the `reportSplitPlan` MCP tool. ' +
        'Each stage needs: name, files array, commit message, and order (0-based).\n';

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
