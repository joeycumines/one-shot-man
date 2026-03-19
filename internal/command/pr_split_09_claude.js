'use strict';
// pr_split_09_claude.js — Claude Code Executor & prompt system
// Dependencies: chunks 00, 02 must be loaded first
// Late-binds: exec (00), template (00), detectGoModulePath (02), fileExtension (00), runtime (00)
//
// Exports: ClaudeCodeExecutor, renderPrompt, renderClassificationPrompt,
//          renderSplitPlanPrompt, renderConflictPrompt, detectLanguage,
//          CLASSIFICATION_PROMPT_TEMPLATE, SPLIT_PLAN_PROMPT_TEMPLATE,
//          CONFLICT_RESOLUTION_PROMPT_TEMPLATE

(function(prSplit) {

    // --- ClaudeCodeExecutor ---

    function ClaudeCodeExecutor(config) {
        this.command = config.claudeCommand || '';
        this.args = config.claudeArgs || [];
        this.model = config.claudeModel || '';
        this.configDir = config.claudeConfigDir || '';
        this.env = config.claudeEnv || {};
        this.resolved = null;
        this.handle = null;
        this.sessionId = null;
        this.cm = null;
    }

    // resolve determines which Claude binary to use.
    // Priority: explicit config > 'claude' on PATH > 'ollama' on PATH > error.
    ClaudeCodeExecutor.prototype.resolve = function() {
        var exec = prSplit._modules.exec;
        if (this.command) {
            var check = exec.execv(['which', this.command]);
            if (check.code !== 0) {
                return { error: 'Claude command not found: ' + this.command };
            }
            this.resolved = { command: this.command, type: 'explicit' };
            return { error: null };
        }

        var claudeCheck = exec.execv(['which', 'claude']);
        if (claudeCheck.code === 0) {
            var versionCheck = exec.execv(['claude', '--version']);
            if (versionCheck.code !== 0) {
                return {
                    error: 'Claude found at ' + claudeCheck.stdout.trim() +
                           ' but version check failed (exit ' + versionCheck.code + '): ' +
                           (versionCheck.stderr || versionCheck.stdout || '').trim()
                };
            }
            this.resolved = { command: 'claude', type: 'claude-code' };
            return { error: null };
        }

        var ollamaCheck = exec.execv(['which', 'ollama']);
        if (ollamaCheck.code === 0) {
            if (this.model) {
                var listCheck = exec.execv(['ollama', 'list']);
                if (listCheck.code !== 0) {
                    return {
                        error: 'Ollama found but list command failed (exit ' + listCheck.code + '): ' +
                               (listCheck.stderr || listCheck.stdout || '').trim()
                    };
                }
                var modelOutput = (listCheck.stdout || '');
                if (modelOutput.indexOf(this.model) === -1) {
                    return {
                        error: 'Ollama found but model ' + this.model + ' not available. ' +
                               'Available models: ' + modelOutput.trim().split('\n').slice(1).join(', ')
                    };
                }
            }
            this.resolved = { command: 'ollama', type: 'ollama' };
            return { error: null };
        }

        return {
            error: 'No Claude-compatible binary found. Install Claude Code CLI ' +
                   '(claude) or Ollama (ollama), or set --claude-command explicitly.'
        };
    };

    // resolveAsync determines which Claude binary to use, non-blocking.
    // Uses exec.spawn() instead of exec.execv() so the TUI event loop
    // is not frozen during subprocess execution.
    // Accepts an optional progressFn(msg) callback for status updates.
    // Returns a Promise<{ error: string|null }>.
    ClaudeCodeExecutor.prototype.resolveAsync = async function(progressFn) {
        var exec = prSplit._modules.exec;
        var self = this;

        // Helper: run a command async, return {stdout, stderr, code}.
        async function runAsync(args) {
            var child = exec.spawn(args[0], args.slice(1));
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

        function progress(msg) {
            if (progressFn) progressFn(msg);
        }

        if (self.command) {
            progress('Resolving binary: ' + self.command + '…');
            var check = await runAsync(['which', self.command]);
            if (check.code !== 0) {
                return { error: 'Claude command not found: ' + self.command };
            }
            self.resolved = { command: self.command, type: 'explicit' };
            return { error: null };
        }

        progress('Resolving binary…');
        var claudeCheck = await runAsync(['which', 'claude']);
        if (claudeCheck.code === 0) {
            progress('Checking version…');
            var versionCheck = await runAsync(['claude', '--version']);
            if (versionCheck.code !== 0) {
                return {
                    error: 'Claude found at ' + claudeCheck.stdout.trim() +
                           ' but version check failed (exit ' + versionCheck.code + '): ' +
                           (versionCheck.stderr || versionCheck.stdout || '').trim()
                };
            }
            self.resolved = { command: 'claude', type: 'claude-code' };
            return { error: null };
        }

        var ollamaCheck = await runAsync(['which', 'ollama']);
        if (ollamaCheck.code === 0) {
            progress('Checking Ollama…');
            if (self.model) {
                var listCheck = await runAsync(['ollama', 'list']);
                if (listCheck.code !== 0) {
                    return {
                        error: 'Ollama found but list command failed (exit ' + listCheck.code + '): ' +
                               (listCheck.stderr || listCheck.stdout || '').trim()
                    };
                }
                var modelOutput = (listCheck.stdout || '');
                if (modelOutput.indexOf(self.model) === -1) {
                    return {
                        error: 'Ollama found but model ' + self.model + ' not available. ' +
                               'Available models: ' + modelOutput.trim().split('\n').slice(1).join(', ')
                    };
                }
            }
            self.resolved = { command: 'ollama', type: 'ollama' };
            return { error: null };
        }

        return {
            error: 'No Claude-compatible binary found. Install Claude Code CLI ' +
                   '(claude) or Ollama (ollama), or set --claude-command explicitly.'
        };
    };

    // spawn creates an MCP session and launches the Claude process.
    // Returns a Promise that resolves with { error, sessionId }.
    ClaudeCodeExecutor.prototype.spawn = async function(sessionId, opts) {
        var exec = prSplit._modules.exec;
        opts = opts || {};

        if (!this.cm) {
            try {
                this.cm = require('osm:claudemux');
            } catch (e) {
                return { error: 'osm:claudemux module not available: ' + e.message };
            }
        }

        var resolveResult = await this.resolveAsync();
        if (resolveResult.error) {
            return { error: resolveResult.error };
        }

        this.sessionId = sessionId || ('prsplit-' + Date.now());

        if (!opts.mcpConfigPath) {
            return { error: 'mcpConfigPath is required (provided by osm:mcpcallback)' };
        }
        var mcpConfigPath = opts.mcpConfigPath;

        var spawnOpts;
        try {
            var registry = this.cm.newRegistry();
            var provider;
            var baseArgs = (this.args || []).concat(['--mcp-config', mcpConfigPath]);

            if (this.resolved.type === 'claude-code') {
                baseArgs = ['--dangerously-skip-permissions'].concat(baseArgs);
                provider = this.cm.claudeCode({
                    command: this.resolved.command,
                    mcp: true
                });
            } else if (this.resolved.type === 'explicit') {
                var basename = this.resolved.command.replace(/^.*[\/\\]/, '');
                if (basename.indexOf('claude') !== -1) {
                    baseArgs = ['--dangerously-skip-permissions'].concat(baseArgs);
                }
                provider = this.cm.claudeCode({
                    command: this.resolved.command,
                    mcp: true
                });
            } else if (this.resolved.type === 'ollama') {
                provider = this.cm.ollama({
                    command: this.resolved.command,
                    model: this.model || '',
                    mcp: true
                });
            } else {
                return { error: 'unknown provider type: ' + this.resolved.type };
            }

            spawnOpts = {
                model: this.model || undefined,
                env: this.env || {},
                args: baseArgs
            };

            registry.register(provider);
            this.handle = registry.spawn(provider.name(), spawnOpts);
        } catch (e) {
            var cmdDesc = this.resolved.command;
            if (spawnOpts && spawnOpts.args && spawnOpts.args.length > 0) {
                cmdDesc += ' ' + spawnOpts.args.join(' ');
            }
            return {
                error: 'Claude spawn failed: ' + (e.message || String(e)) +
                       '\n  Command attempted: ' + cmdDesc +
                       '\n  Provider type: ' + this.resolved.type
            };
        }

        log.printf('Claude executor: spawned command=%s type=%s session=%s args=%s',
            this.resolved.command, this.resolved.type, this.sessionId,
            JSON.stringify(spawnOpts && spawnOpts.args || []));

        // Post-spawn health check: verify process is still alive.
        if (this.handle && typeof this.handle.isAlive === 'function') {
            // Non-blocking 300ms delay — yields event loop for BubbleTea rendering.
            await new Promise(function(resolve) { setTimeout(resolve, 300); });
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

                var cmdDesc2 = this.resolved.command;
                if (spawnOpts && spawnOpts.args && spawnOpts.args.length > 0) {
                    cmdDesc2 += ' ' + spawnOpts.args.join(' ');
                }
                var diagnostic = 'Claude process exited immediately after spawn.';
                if (lastOutput) {
                    diagnostic += '\n  Process output: ' + lastOutput.trim().substring(0, 500);
                }
                diagnostic += '\n  Command: ' + cmdDesc2;
                diagnostic += '\n  Provider: ' + this.resolved.type;
                return { error: diagnostic };
            }
        }

        return { error: null, sessionId: this.sessionId };
    };

    ClaudeCodeExecutor.prototype.isAvailable = function() {
        // Synchronous check: only returns true if already resolved.
        // For TUI contexts, use isAvailableAsync() to avoid blocking.
        if (this.resolved) return true;
        var result = this.resolve();
        return !result.error;
    };

    // isAvailableAsync: non-blocking version of isAvailable.
    // Returns a Promise<boolean>. Safe to call from BubbleTea update handlers.
    ClaudeCodeExecutor.prototype.isAvailableAsync = async function() {
        if (this.resolved) return true;
        var result = await this.resolveAsync();
        return !result.error;
    };

    ClaudeCodeExecutor.prototype.close = function() {
        if (this.handle && typeof this.handle.close === 'function') {
            try { this.handle.close(); } catch (e) { log.debug('close: handle.close failed: ' + (e.message || e)); }
        }
        this.handle = null;
        this.sessionId = null;
        this.resolved = null;
    };

    // captureDiagnostic: Attempt to read last output from the dying
    // process handle for post-mortem analysis.
    ClaudeCodeExecutor.prototype.captureDiagnostic = function() {
        if (!this.handle) return '';
        var output = '';
        if (typeof this.handle.receive === 'function') {
            try {
                var chunk = this.handle.receive();
                if (chunk) { output = chunk; }
            } catch (e) { log.debug('captureDiagnostic: read failed (expected for dead process): ' + (e.message || e)); }
        }
        return output;
    };

    // restart: Close the current session and spawn a new one.
    // Returns a Promise (same shape as spawn): { error, sessionId }.
    ClaudeCodeExecutor.prototype.restart = async function(sessionId, opts) {
        log.printf('ClaudeCodeExecutor.restart: closing existing session');
        this.close();
        var resolveResult = await this.resolveAsync();
        if (resolveResult.error) {
            return { error: 'restart resolve failed: ' + resolveResult.error };
        }
        log.printf('ClaudeCodeExecutor.restart: spawning new session');
        return await this.spawn(sessionId, opts);
    };

    // --- Prompt Templates (Go text/template syntax) ---

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

    // --- Prompt Rendering ---
    //
    //  T111 DESIGN NOTE: renderPrompt returns { text: string, error: string|null }.
    //  ALL callers MUST check .error before using .text. When .error is non-null,
    //  .text is the empty string '' — sending it to Claude produces undefined
    //  behavior (silent empty prompt). Current call sites in chunks 08 and 10
    //  correctly check .error (see automatedSplit step 3, resolveConflictsWithClaude,
    //  claude-fix strategy). This contract is enforced by test coverage:
    //  TestRenderPrompt_TemplateModuleNull, TestRenderPrompt_TemplateExecuteThrows.

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
            MaxFilesPerSplit: config.maxFilesPerSplit || runtime.maxFiles || 0,
            PreferIndependent: config.preferIndependent || false
        });
    }

    function renderConflictPrompt(conflict) {
        return renderPrompt(CONFLICT_RESOLUTION_PROMPT_TEMPLATE, {
            BranchName: conflict.branchName || '',
            Files: conflict.files || [],
            ExitCode: conflict.exitCode || 1,
            ErrorOutput: conflict.errorOutput || '',
            GoModContent: conflict.goModContent || ''
        });
    }

    // --- Language Detection ---

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
        // T112: Also track raw extension frequencies for unknown fallback.
        // Denylist: documentation/config extensions that aren't programming
        // languages.  These should never win the "most common unknown ext"
        // contest.
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
        // T112: Fallback — use the most common unrecognized extension as
        // a pseudo-language name (e.g., '.proto' → 'Proto').  Only fires
        // when no known language was detected AND the repo contains files
        // with a non-documentation extension.
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
                // Strip leading dot and capitalize: '.proto' → 'Proto'
                var raw = bestExt.replace(/^\./, '');
                best = raw.charAt(0).toUpperCase() + raw.slice(1);
            }
        }
        return best || 'unknown';
    }

    // --- Exports ---

    prSplit.ClaudeCodeExecutor = ClaudeCodeExecutor;
    prSplit.renderPrompt = renderPrompt;
    prSplit.renderClassificationPrompt = renderClassificationPrompt;
    prSplit.renderSplitPlanPrompt = renderSplitPlanPrompt;
    prSplit.renderConflictPrompt = renderConflictPrompt;
    prSplit.detectLanguage = detectLanguage;
    prSplit.CLASSIFICATION_PROMPT_TEMPLATE = CLASSIFICATION_PROMPT_TEMPLATE;
    prSplit.SPLIT_PLAN_PROMPT_TEMPLATE = SPLIT_PLAN_PROMPT_TEMPLATE;
    prSplit.CONFLICT_RESOLUTION_PROMPT_TEMPLATE = CONFLICT_RESOLUTION_PROMPT_TEMPLATE;

})(globalThis.prSplit);
