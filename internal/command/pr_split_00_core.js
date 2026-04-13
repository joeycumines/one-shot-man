'use strict';
// pr_split_00_core.js — Core: requires, style, config, runtime, internal helpers
//
// Initializes globalThis.prSplit and defines foundational utilities used by
// every subsequent chunk. This file MUST be loaded first.
//
// Go-injected globals expected:
//   prSplitConfig  — {baseBranch, strategy, maxFiles, branchPrefix, verifyCommand, ...}
//   config.name    — "pr-split"
//   prSplit._cancelSource — cancellation callback (optional; used by isCancelled/isPaused/isForceCancelled)
//
// See docs/architecture-pr-split-chunks.md for the full chunk loading contract.

(function() {
    // --- Module imports ---

    var bt = require('osm:bt');
    var exec = require('osm:exec');

    // TUI-only modules: loaded conditionally so that the script can also be
    // require()'d from test environments that lack these modules.
    var template, shared;
    try {
        template = require('osm:text/template');
        shared = require('osm:sharedStateSymbols');
    } catch (e) {
        template = null;
        shared = null;
    }

    // Optional: os module for file I/O (plan persistence).
    var osmod;
    try {
        osmod = require('osm:os');
    } catch (e) {
        osmod = null;
    }

    // Optional: lipgloss for styled terminal output.
    var lip;
    try {
        lip = require('osm:lipgloss');
    } catch (e) {
        lip = null;
    }

    // --- Styled Output Helpers ---

    // style creates terminal-styled text using Lipgloss when available.
    // Degrades gracefully to plain text when Lipgloss is not loaded.
    var style = (function() {
        if (!lip) {
            return {
                success: function(s) { return s; },
                error: function(s) { return s; },
                warning: function(s) { return s; },
                info: function(s) { return s; },
                header: function(s) { return s; },
                dim: function(s) { return s; },
                bold: function(s) { return s; },
                diffAdd: function(s) { return s; },
                diffRemove: function(s) { return s; },
                diffHunk: function(s) { return s; },
                diffMeta: function(s) { return s; },
                diffContext: function(s) { return s; },
                progressBar: function(current, total, width) {
                    width = width || 20;
                    var filled = total > 0 ? Math.round((current / total) * width) : 0;
                    var bar = '';
                    for (var i = 0; i < width; i++) {
                        bar += i < filled ? '█' : '░';
                    }
                    return '[' + bar + '] ' + current + '/' + total;
                }
            };
        }

        var successStyle = lip.newStyle().foreground('#22c55e').bold();
        var errorStyle = lip.newStyle().foreground('#ef4444').bold();
        var warningStyle = lip.newStyle().foreground('#f59e0b');
        var infoStyle = lip.newStyle().foreground('#3b82f6');
        var headerStyle = lip.newStyle().foreground('#a78bfa').bold();
        var dimStyle = lip.newStyle().foreground('#6b7280');
        var boldStyle = lip.newStyle().bold();
        var barFilledStyle = lip.newStyle().foreground('#22c55e');
        var barEmptyStyle = lip.newStyle().foreground('#374151');

        // Diff-specific styles — replaces hardcoded ANSI codes.
        var diffAddStyle = lip.newStyle().foreground('#22c55e');    // green
        var diffRemoveStyle = lip.newStyle().foreground('#ef4444'); // red
        var diffHunkStyle = lip.newStyle().foreground('#06b6d4');   // cyan
        var diffMetaStyle = lip.newStyle().bold();                  // bold
        var diffContextStyle = lip.newStyle().foreground('#6b7280');// gray

        return {
            success: function(s) { return successStyle.render(s); },
            error: function(s) { return errorStyle.render(s); },
            warning: function(s) { return warningStyle.render(s); },
            info: function(s) { return infoStyle.render(s); },
            header: function(s) { return headerStyle.render(s); },
            dim: function(s) { return dimStyle.render(s); },
            bold: function(s) { return boldStyle.render(s); },
            diffAdd: function(s) { return diffAddStyle.render(s); },
            diffRemove: function(s) { return diffRemoveStyle.render(s); },
            diffHunk: function(s) { return diffHunkStyle.render(s); },
            diffMeta: function(s) { return diffMetaStyle.render(s); },
            diffContext: function(s) { return diffContextStyle.render(s); },
            progressBar: function(current, total, width) {
                width = width || 20;
                var filled = total > 0 ? Math.round((current / total) * width) : 0;
                var bar = '';
                for (var i = 0; i < width; i++) {
                    if (i < filled) {
                        bar += barFilledStyle.render('█');
                    } else {
                        bar += barEmptyStyle.render('░');
                    }
                }
                return '[' + bar + '] ' + current + '/' + total;
            }
        };
    })();

    // --- Configuration & Constants ---

    // Read injected configuration with defaults.
    var cfg = (typeof prSplitConfig !== 'undefined') ? prSplitConfig : {};
    var COMMAND_NAME = (typeof config !== 'undefined' && config.name) ? config.name : 'pr-split';
    var MODE_NAME = 'pr-split';

    // --- Discovery & Scoping ---

    // discoverVerifyCommand auto-detects the verification command for the
    // working directory. Checks for Makefile, makefile, and GNUmakefile.
    // Prefers 'gmake' (GNU Make on macOS) over 'make' if available.
    // Returns the best make command, or '' if no Makefile found.
    function discoverVerifyCommand(dir) {
        var sep = isWindows() ? '\\' : '/';
        var names = ['Makefile', 'makefile', 'GNUmakefile'];
        var hasMakefile = false;
        for (var i = 0; i < names.length; i++) {
            var path = dir ? (dir + sep + names[i]) : names[i];
            if (osmod && osmod.fileExists(path)) {
                hasMakefile = true;
                break;
            } else if (!osmod) {
                // Fallback: use platform-aware file existence check.
                var result = isWindows()
                    ? exec.execv(['cmd.exe', '/C', 'if exist "' + path + '" (exit 0) else (exit 1)'])
                    : exec.execv(['test', '-f', path]);
                if (result.code === 0) { hasMakefile = true; break; }
            }
        }
        if (!hasMakefile) return '';

        // Prefer gmake (GNU Make) if available — required on macOS where
        // the system 'make' is BSD Make 3.81 which cannot run GNU Makefiles.
        // Late-bind through prSplit._lookupBinary so tests can override.
        var lbFn = (typeof prSplit !== 'undefined' && prSplit._lookupBinary) || lookupBinary;
        var gmakeLookup = lbFn('gmake');
        if (gmakeLookup.found) return 'gmake';

        return 'make';
    }

    // T071: Language detection helpers for scopedVerifyCommand.
    // Each detector returns the scoped test command for a homogeneous set of
    // files of that language, or null if the file set is not exclusively that
    // language. Detectors are tried in order; first match wins.

    var _langDetectors = [
        {
            // Go: run `go test -race` on unique package directories.
            name: 'go',
            test: function(f) { return f.match(/\.go$/) !== null; },
            build: function(files) {
                var pkgDirs = {};
                for (var i = 0; i < files.length; i++) {
                    var lastSlash = files[i].lastIndexOf('/');
                    var dir = lastSlash >= 0
                        ? './' + files[i].substring(0, lastSlash) + '/...'
                        : './...';
                    pkgDirs[dir] = true;
                }
                var pkgs = Object.keys(pkgDirs).sort();
                return pkgs.length > 0 ? 'go test -race ' + pkgs.join(' ') : null;
            }
        },
        {
            // JavaScript / TypeScript: run jest with file paths.
            name: 'js',
            test: function(f) { return f.match(/\.(js|jsx|ts|tsx|mjs|cjs)$/) !== null; },
            build: function(files) {
                // Collect unique directories for --testPathPattern.
                var dirs = {};
                for (var i = 0; i < files.length; i++) {
                    var lastSlash = files[i].lastIndexOf('/');
                    var dir = lastSlash >= 0 ? files[i].substring(0, lastSlash) : '.';
                    dirs[dir] = true;
                }
                var dirList = Object.keys(dirs).sort();
                // Build a regex pattern matching any of the directories.
                var pattern = dirList.length === 1
                    ? dirList[0]
                    : '(' + dirList.join('|') + ')';
                return 'npx jest --passWithNoTests --testPathPattern ' + JSON.stringify(pattern);
            }
        },
        {
            // Python: run pytest with file paths.
            name: 'python',
            test: function(f) { return f.match(/\.py$/) !== null; },
            build: function(files) {
                // Collect unique directories.
                var dirs = {};
                for (var i = 0; i < files.length; i++) {
                    var lastSlash = files[i].lastIndexOf('/');
                    var dir = lastSlash >= 0 ? files[i].substring(0, lastSlash) : '.';
                    dirs[dir] = true;
                }
                var dirList = Object.keys(dirs).sort();
                return 'python -m pytest ' + dirList.join(' ');
            }
        },
        {
            // Rust: run cargo test (scoping to specific crates is complex,
            // so we scope to the workspace root; still better than 'make').
            name: 'rust',
            test: function(f) { return f.match(/\.rs$/) !== null; },
            build: function(_files) {
                return 'cargo test';
            }
        }
    ];

    // scopedVerifyCommand returns a verification command scoped to the
    // packages/directories affected by the given files. Supports Go, JS/TS,
    // Python, and Rust. For mixed-language file sets, returns the fallback
    // command (full verification). The scoped command runs the appropriate
    // test runner for the detected language.
    //
    // T071: Extended from Go-only to multi-language support.
    function scopedVerifyCommand(files, fallbackCommand) {
        if (!files || files.length === 0) {
            return fallbackCommand;
        }

        // Only scope when the fallback is a known build/test runner.
        // Custom verify commands (e.g. 'true', 'echo ok') must not be replaced.
        var scopable = (fallbackCommand === 'make' ||
                        fallbackCommand === 'gmake' ||
                        (typeof fallbackCommand === 'string' && (
                            fallbackCommand.indexOf('go test') >= 0 ||
                            fallbackCommand.indexOf('jest') >= 0 ||
                            fallbackCommand.indexOf('pytest') >= 0 ||
                            fallbackCommand.indexOf('cargo test') >= 0
                        )));
        if (!scopable) {
            return fallbackCommand;
        }

        // Detect language: all files must match a single detector.
        var matchedDetector = null;
        for (var d = 0; d < _langDetectors.length; d++) {
            var detector = _langDetectors[d];
            var allMatch = true;
            for (var i = 0; i < files.length; i++) {
                if (!files[i] || !files[i].match) {
                    return fallbackCommand; // null/undefined file → fall back.
                }
                if (!detector.test(files[i])) {
                    allMatch = false;
                    break;
                }
            }
            if (allMatch) {
                matchedDetector = detector;
                break;
            }
        }

        if (!matchedDetector) {
            // Mixed languages or unknown files → fall back.
            return fallbackCommand;
        }

        var cmd = matchedDetector.build(files);
        return cmd || fallbackCommand;
    }

    // --- Mutable Runtime Config ---

    var runtime = {
        baseBranch:    cfg.baseBranch    || 'main',
        dir:           cfg.dir           || '.',
        strategy:      cfg.strategy      || 'directory',
        maxFiles:      cfg.maxFiles      || 10,
        branchPrefix:  cfg.branchPrefix  || 'split/',
        verifyCommand: cfg.verifyCommand || discoverVerifyCommand('.') || '',
        dryRun:        cfg.dryRun        || false,
        jsonOutput:    cfg.jsonOutput    || false,
        mode:          cfg.mode          || 'heuristic',
        retryBudget:   typeof cfg.retryBudget === 'number' ? cfg.retryBudget : 3,
        view:          cfg.view          || 'toggle',
        autoMerge:     cfg.autoMerge     || false,
        mergeMethod:   cfg.mergeMethod   || 'squash'
    };

    // --- Internal Helpers ---

    // gitExec runs a git command, optionally in the given directory.
    // Returns {stdout: string, stderr: string, code: number}.
    function gitExec(dir, args) {
        var cmd = ['git'];
        if (dir && dir !== '' && dir !== '.') {
            cmd.push('-C');
            cmd.push(dir);
        }
        for (var i = 0; i < args.length; i++) {
            cmd.push(args[i]);
        }
        return exec.execv(cmd);
    }

    // gitExecAsync runs a git command asynchronously using exec.spawn().
    // Returns a Promise<{stdout: string, stderr: string, code: number, error: boolean, message: string}>.
    // Unlike gitExec (blocking), this does NOT freeze the event loop during execution,
    // allowing BubbleTea to continue rendering and processing events.
    //
    // T30: Critical for unblocking the TUI event loop during git operations.
    // T44: Optional third parameter `options` with `outputFn(line)` callback
    //      to stream output chunks to the TUI Output tab in real-time.
    async function gitExecAsync(dir, args, options) {
        var gitArgs = [];
        if (dir && dir !== '' && dir !== '.') {
            gitArgs.push('-C');
            gitArgs.push(dir);
        }
        for (var i = 0; i < args.length; i++) {
            gitArgs.push(args[i]);
        }

        var outputFn = (options && typeof options.outputFn === 'function') ? options.outputFn : null;

        // T44: Fall back to global output capture function if no explicit outputFn.
        // This allows all gitExecAsync calls (including those in analysis/execution chunks)
        // to pipe output to the TUI Output tab without modifying every call site.
        if (!outputFn && typeof prSplit._outputCaptureFn === 'function') {
            outputFn = prSplit._outputCaptureFn;
        }

        // T44: Emit command header to output callback.
        if (outputFn) {
            outputFn('\u276f git ' + args.join(' '));
        }

        var child = exec.spawn('git', gitArgs);

        // Collect stdout and stderr in parallel to avoid deadlock
        // (process may fill stderr buffer before we finish reading stdout).
        async function readAll(stream, streamOutputFn) {
            var buf = '';
            while (true) {
                var chunk = await stream.read();
                if (chunk.done) break;
                if (chunk.value !== undefined && chunk.value !== null) {
                    var chunkStr = String(chunk.value);
                    buf += chunkStr;
                    // T44: Stream each chunk line-by-line to the output callback.
                    if (streamOutputFn) {
                        var chunkLines = chunkStr.split('\n');
                        for (var cl = 0; cl < chunkLines.length; cl++) {
                            var line = chunkLines[cl];
                            // Skip trailing empty line from split (last \n produces empty string).
                            if (cl === chunkLines.length - 1 && line === '') continue;
                            streamOutputFn(line);
                        }
                    }
                }
            }
            return buf;
        }

        var results = await Promise.all([
            readAll(child.stdout, outputFn),
            readAll(child.stderr, outputFn),
            child.wait()
        ]);

        var stdout = results[0];
        var stderr = results[1];
        var waitResult = results[2];
        var code = (waitResult && waitResult.code !== undefined) ? waitResult.code : 0;

        return {
            stdout: stdout,
            stderr: stderr,
            code: code,
            error: code !== 0,
            message: code !== 0 ? 'exit status ' + code : ''
        };
    }

    // shellExecAsync runs an arbitrary shell command asynchronously via the
    // platform shell (sh -c on Unix, cmd.exe /C on Windows).
    // Returns a Promise<{stdout: string, stderr: string, code: number, error: boolean, message: string}>.
    // Uses shellSpawnAsync internally, so the event loop is NOT frozen,
    // allowing BubbleTea to continue rendering and processing events.
    //
    // T078/T109: Generic async shell execution for conflict resolution strategies,
    // verify commands, and resolution commands. Follows the same readAll + Promise.all
    // pattern as gitExecAsync for consistent behavior.
    //
    // Options:
    //   outputFn(line) — callback to stream each output line (for TUI Output tab)
    async function shellExecAsync(command, options) {
        options = options || {};
        var outputFn = (options && typeof options.outputFn === 'function') ? options.outputFn : null;

        // Fall back to global output capture function (same as gitExecAsync).
        if (!outputFn && typeof prSplit._outputCaptureFn === 'function') {
            outputFn = prSplit._outputCaptureFn;
        }

        // Emit command header to output callback.
        if (outputFn) {
            outputFn('\u276f ' + command);
        }

        // Use platform shell for command dispatch.
        var child = shellSpawnAsync(command);

        // Collect stdout and stderr in parallel (same readAll pattern as gitExecAsync).
        async function readAll(stream, streamOutputFn) {
            var buf = '';
            while (true) {
                var chunk = await stream.read();
                if (chunk.done) break;
                if (chunk.value !== undefined && chunk.value !== null) {
                    var chunkStr = String(chunk.value);
                    buf += chunkStr;
                    if (streamOutputFn) {
                        var chunkLines = chunkStr.split('\n');
                        for (var cl = 0; cl < chunkLines.length; cl++) {
                            var line = chunkLines[cl];
                            if (cl === chunkLines.length - 1 && line === '') continue;
                            streamOutputFn(line);
                        }
                    }
                }
            }
            return buf;
        }

        var results = await Promise.all([
            readAll(child.stdout, outputFn),
            readAll(child.stderr, outputFn),
            child.wait()
        ]);

        var stdout = results[0];
        var stderr = results[1];
        var waitResult = results[2];
        var code = (waitResult && waitResult.code !== undefined) ? waitResult.code : 0;

        return {
            stdout: stdout,
            stderr: stderr,
            code: code,
            error: code !== 0,
            message: code !== 0 ? 'exit status ' + code : ''
        };
    }

    // resolveDir resolves a directory path to an absolute path. When dir is
    // empty, falsy, or '.', it resolves to the current working directory.
    // This prevents git operations from being affected by later CWD changes
    // (e.g. during test cleanup ordering races).
    //
    // Resolution order:
    //   1. If dir is already an absolute/non-default path, use it.
    //   2. If runtime.dir is configured (set by tests or CLI), use it.
    //   3. Fall back to '.' (uses process CWD — correct in production).
    function resolveDir(dir) {
        if (dir && dir !== '' && dir !== '.') {
            return dir;
        }
        // Use the runtime-configured dir if set to a non-default value.
        // This allows tests to pass an absolute path via config overrides,
        // and avoids calling exec.execv(['pwd']) which would interfere with
        // test mocks.
        if (runtime.dir && runtime.dir !== '' && runtime.dir !== '.') {
            return runtime.dir;
        }
        return dir || '.';
    }

    // isWindows returns true when running on Windows.
    function isWindows() {
        if (osmod && typeof osmod.platform === 'function') {
            return osmod.platform() === 'windows';
        }
        return false;
    }

    // T103: worktreeTmpPath generates a temporary worktree path in the system
    // temp directory, avoiding the fragile `dir + '/../'` pattern that breaks
    // at filesystem root or non-writable parent directories.  The random
    // suffix provides sufficient entropy to avoid collisions from concurrent
    // invocations.
    function worktreeTmpPath(prefix) {
        var base = '/tmp';
        if (osmod && typeof osmod.getenv === 'function') {
            var envTmp = osmod.getenv('TMPDIR') || osmod.getenv('TMP') || osmod.getenv('TEMP');
            if (envTmp) {
                base = envTmp;
            }
        }
        // Strip trailing slash or backslash (macOS TMPDIR often ends with
        // '/', Windows TEMP often ends with '\').
        if (base.length > 1) {
            var last = base.charAt(base.length - 1);
            if (last === '/' || last === '\\') {
                base = base.substring(0, base.length - 1);
            }
        }
        var sep = isWindows() ? '\\' : '/';
        var entropy = Date.now() + '-' + Math.floor(Math.random() * 1000000).toString(36);
        return base + sep + '.' + prefix + entropy;
    }

    // shellQuote wraps a string for safe shell interpolation.
    // On Unix: single-quote escaping.  On Windows: double-quote with ^ escaping
    // for cmd.exe metacharacters.
    function shellQuote(s) {
        if (isWindows()) {
            // cmd.exe: wrap in double quotes, escape special characters.
            // Inside double quotes, ^ escapes ", !, and ^ itself.
            // % must be doubled (%% not ^%) to prevent variable expansion.
            var escaped = s.replace(/%/g, '%%').replace(/["!^]/g, '^$&');
            return '"' + escaped + '"';
        }
        return "'" + s.replace(/'/g, "'\\''") + "'";
    }

    // lookupBinary checks whether a named binary exists on PATH.
    // Returns {found: bool, path: string} using 'where.exe' on Windows,
    // 'which' on Unix.
    function lookupBinary(name) {
        var cmd = isWindows() ? 'where.exe' : 'which';
        var result = exec.execv([cmd, name]);
        return {
            found: result.code === 0,
            path: (result.stdout || '').split('\n')[0].trim()
        };
    }

    // lookupBinaryAsync is the non-blocking version of lookupBinary.
    // Returns a Promise<{found: bool, path: string}>.
    async function lookupBinaryAsync(name) {
        var cmd = isWindows() ? 'where.exe' : 'which';
        var child = exec.spawn(cmd, [name]);
        var stdout = '';
        while (true) {
            var chunk = await child.stdout.read();
            if (chunk.done) break;
            if (chunk.value !== undefined && chunk.value !== null) {
                stdout += String(chunk.value);
            }
        }
        var waitResult = await child.wait();
        return {
            found: waitResult && waitResult.code === 0,
            path: stdout.split('\n')[0].trim()
        };
    }

    // shellSpawnSync runs a shell command string synchronously through the
    // platform shell (sh -c on Unix, cmd.exe /C on Windows).
    // Returns the exec result object.
    function shellSpawnSync(shellCmd, opts) {
        if (isWindows()) {
            return exec.execStream(['cmd.exe', '/C', shellCmd], opts || {});
        }
        return exec.execStream(['sh', '-c', shellCmd], opts || {});
    }

    // shellSpawnAsync runs a shell command string asynchronously through the
    // platform shell (sh -c on Unix, cmd.exe /C on Windows).
    // Returns the spawned child process.
    function shellSpawnAsync(shellCmd) {
        if (isWindows()) {
            return exec.spawn('cmd.exe', ['/C', shellCmd]);
        }
        return exec.spawn('sh', ['-c', shellCmd]);
    }

    // gitAddChangedFiles stages modified, new, and deleted files while
    // filtering out known pr-split tool artifacts (e.g., .pr-split-plan.json).
    function gitAddChangedFiles(dir) {
        var status = gitExec(dir, ['status', '--porcelain']);
        if (status.code !== 0 || status.stdout.trim() === '') {
            return;
        }
        var EXCLUDED = ['.pr-split-plan.json'];
        var lines = status.stdout.split('\n');
        var files = [];
        for (var i = 0; i < lines.length; i++) {
            var line = lines[i];
            if (line.length < 3) continue;
            var path = line.substring(3);
            var arrowIdx = path.indexOf(' -> ');
            if (arrowIdx >= 0) {
                path = path.substring(arrowIdx + 4);
            }
            if (path.charAt(0) === '"' && path.charAt(path.length - 1) === '"') {
                path = path.substring(1, path.length - 1);
            }
            var excluded = false;
            for (var e = 0; e < EXCLUDED.length; e++) {
                if (path === EXCLUDED[e] || path.indexOf('/' + EXCLUDED[e]) >= 0) {
                    excluded = true;
                    break;
                }
            }
            if (!excluded) {
                files.push(path);
            }
        }
        if (files.length > 0) {
            var addArgs = ['add', '--'];
            for (var f = 0; f < files.length; f++) {
                addArgs.push(files[f]);
            }
            var addResult = gitExec(dir, addArgs);
            if (addResult.code !== 0 && typeof log !== 'undefined' && log.warn) {
                log.warn('pr-split: git add failed in gitAddChangedFiles: ' + addResult.stderr.trim());
            }
        }
    }

    // gitAddChangedFilesAsync is the non-blocking version of gitAddChangedFiles.
    // Uses gitExecAsync (exec.spawn) so the event loop stays responsive during BubbleTea TUI.
    // T31: async version for pipeline use.
    async function gitAddChangedFilesAsync(dir) {
        var gitExecAsync = prSplit._gitExecAsync;
        var status = await gitExecAsync(dir, ['status', '--porcelain']);
        if (status.code !== 0 || status.stdout.trim() === '') {
            return;
        }
        var EXCLUDED = ['.pr-split-plan.json'];
        var lines = status.stdout.split('\n');
        var files = [];
        for (var i = 0; i < lines.length; i++) {
            var line = lines[i];
            if (line.length < 3) continue;
            var path = line.substring(3);
            var arrowIdx = path.indexOf(' -> ');
            if (arrowIdx >= 0) {
                path = path.substring(arrowIdx + 4);
            }
            if (path.charAt(0) === '"' && path.charAt(path.length - 1) === '"') {
                path = path.substring(1, path.length - 1);
            }
            var excluded = false;
            for (var e = 0; e < EXCLUDED.length; e++) {
                if (path === EXCLUDED[e] || path.indexOf('/' + EXCLUDED[e]) >= 0) {
                    excluded = true;
                    break;
                }
            }
            if (!excluded) {
                files.push(path);
            }
        }
        if (files.length > 0) {
            var addArgs = ['add', '--'];
            for (var f = 0; f < files.length; f++) {
                addArgs.push(files[f]);
            }
            var addResult = await gitExecAsync(dir, addArgs);
            if (addResult.code !== 0 && typeof log !== 'undefined' && log.warn) {
                log.warn('pr-split: git add failed in gitAddChangedFilesAsync: ' + addResult.stderr.trim());
            }
        }
    }

    // dirname extracts the directory at the given depth from a file path.
    // Handles both forward and back slashes for Windows compatibility.
    function dirname(filepath, depth) {
        depth = depth || 1;
        // Normalise to forward slashes for splitting.
        var normalised = filepath.replace(/\\/g, '/');
        var parts = normalised.split('/');
        if (parts.length <= 1) {
            return '.';
        }
        var taken = parts.slice(0, Math.min(depth, parts.length - 1));
        return taken.join('/');
    }

    // fileExtension returns the file extension including the dot, or ''.
    // Handles both forward and back slashes for Windows compatibility.
    function fileExtension(filepath) {
        var normalised = filepath.replace(/\\/g, '/');
        var base = normalised.split('/').pop();
        var dot = base.lastIndexOf('.');
        if (dot <= 0) {
            return '';
        }
        return base.substring(dot);
    }

    // sanitizeBranchName replaces non-safe characters with hyphens.
    function sanitizeBranchName(name) {
        return name.replace(/[^a-zA-Z0-9_/-]/g, '-');
    }

    // padIndex pads a number to at least 2 digits: 1 → '01', 12 → '12'.
    function padIndex(n) {
        var s = String(n);
        while (s.length < 2) {
            s = '0' + s;
        }
        return s;
    }

    // --- Cooperative Cancellation (via injectable callback) ---
    //
    // The auto-split TUI was removed in T27. Cancellation is
    // now driven by a _cancelSource callback injected before pipeline runs.
    // Callers set prSplit._cancelSource = function(query) { ... } before
    // calling automatedSplit(). query is 'cancelled', 'paused', or 'forceCancelled'.
    // The callback returns true/false. When no callback is set, returns false.

    // isCancelled checks cooperative cancellation.
    // Returns true when the user has requested cancellation.
    function isCancelled() {
        var src = (typeof prSplit !== 'undefined' && prSplit && prSplit._cancelSource);
        return typeof src === 'function' && src('cancelled');
    }

    // isPaused checks whether the user requested a pause.
    // The pipeline should save a checkpoint and exit cleanly when this
    // returns true.
    function isPaused() {
        var src = (typeof prSplit !== 'undefined' && prSplit && prSplit._cancelSource);
        return typeof src === 'function' && src('paused');
    }

    // isForceCancelled checks whether the user has double-cancelled,
    // signalling that child processes should be killed immediately.
    function isForceCancelled() {
        var src = (typeof prSplit !== 'undefined' && prSplit && prSplit._cancelSource);
        return typeof src === 'function' && src('forceCancelled');
    }

    // --- Initialize globalThis.prSplit ---

    globalThis.prSplit = globalThis.prSplit || {};
    var prSplit = globalThis.prSplit;

    // Shared mutable state container for cross-chunk communication.
    prSplit._state = {};

    // Module references (shared with other chunks via prSplit._modules).
    prSplit._modules = {
        bt: bt,
        exec: exec,
        template: template,
        shared: shared,
        osmod: osmod,
        lip: lip
    };

    // Style helpers.
    prSplit._style = style;

    // Configuration.
    prSplit._cfg = cfg;
    prSplit._COMMAND_NAME = COMMAND_NAME;
    prSplit._MODE_NAME = MODE_NAME;

    // Mutable runtime config.
    prSplit.runtime = runtime;

    // Internal helpers.
    prSplit._gitExec = gitExec;
    prSplit._gitExecAsync = gitExecAsync;
    prSplit._shellExecAsync = shellExecAsync;
    prSplit._resolveDir = resolveDir;
    prSplit._shellQuote = shellQuote;
    prSplit._worktreeTmpPath = worktreeTmpPath;
    prSplit._isWindows = isWindows;
    prSplit._lookupBinary = lookupBinary;
    prSplit._lookupBinaryAsync = lookupBinaryAsync;
    prSplit._shellSpawnSync = shellSpawnSync;
    prSplit._shellSpawnAsync = shellSpawnAsync;
    prSplit._gitAddChangedFiles = gitAddChangedFiles;
    prSplit._gitAddChangedFilesAsync = gitAddChangedFilesAsync;
    prSplit._dirname = dirname;
    prSplit._fileExtension = fileExtension;
    prSplit._sanitizeBranchName = sanitizeBranchName;
    prSplit._padIndex = padIndex;

    // Discovery functions.
    prSplit.discoverVerifyCommand = discoverVerifyCommand;
    prSplit.scopedVerifyCommand = scopedVerifyCommand;

    // Cooperative cancellation.
    prSplit.isCancelled = isCancelled;
    prSplit.isPaused = isPaused;
    prSplit.isForceCancelled = isForceCancelled;
})();
