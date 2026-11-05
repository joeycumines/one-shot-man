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

    function effectiveVerifyTimeoutMs(value) {
        if (typeof value === 'number' && value > 0) return value;
        var defaults = prSplit.AUTOMATED_DEFAULTS || {};
        return (typeof defaults.verifyTimeoutMs === 'number' && defaults.verifyTimeoutMs > 0)
            ? defaults.verifyTimeoutMs
            : 600000;
    }

    // --- Discovery & Scoping ---

    // discoverVerifyCommand auto-detects the verification command for the
    // working directory. Checks for Makefile, makefile, and GNUmakefile.
    // Prefers 'gmake' (GNU Make on macOS) over 'make' if available.
    // Returns the best make command, or '' if no Makefile found.
    async function discoverVerifyCommand(dir) {
        var sep = isWindows() ? '\\' : '/';
        var names = ['Makefile', 'makefile', 'GNUmakefile'];
        var hasMakefile = false;
        for (var i = 0; i < names.length; i++) {
            var path = dir ? (dir + sep + names[i]) : names[i];
            if (osmod && (await osmod.fileExists(path)).exists) {
                hasMakefile = true;
                break;
            } else if (!osmod) {
                var result = isWindows()
                    ? await exec.execv(['cmd.exe', '/C', 'if exist "' + path + '" (exit 0) else (exit 1)'])
                    : await exec.execv(['test', '-f', path]);
                if (result.code === 0) { hasMakefile = true; break; }
            }
        }
        if (!hasMakefile) return '';

        // Prefer gmake (GNU Make) if available — required on macOS where
        // the system 'make' is BSD Make 3.81 which cannot run GNU Makefiles.
        // Late-bind through prSplit._lookupBinary so tests can override.
        var lbFn = (typeof prSplit !== 'undefined' && prSplit._lookupBinary) || lookupBinary;
        var gmakeLookup = await lbFn('gmake');
        if (gmakeLookup.found) return 'gmake';

        return 'make';
    }

    // T071: Language detection helpers for scopedVerifyCommand.
    // Each detector returns the scoped test command for a homogeneous set of
    // files of that language, or null if the file set is not exclusively that
    // language. Detectors are tried in order; first match wins.

    function scopedShellArg(value) {
        return /^[A-Za-z0-9_./-]+$/.test(value) ? value : shellQuote(value);
    }

    // Jest --testPathPattern takes a regex. Always JSON.stringify (double
    // quotes) so the emitted command is identical on Unix and Windows:
    // parens and pipes are safe inside double quotes in both sh and
    // cmd.exe, while shellQuote is deliberately platform-dependent.
    function scopedRegexArg(value) {
        return JSON.stringify(value);
    }

    function safeScopedPath(value) {
        if (typeof value !== 'string' || value.trim() === '' ||
            value.indexOf(String.fromCharCode(0)) !== -1) {
            return false;
        }
        var normalized = value.split(String.fromCharCode(92)).join('/');
        if (normalized.charAt(0) === '/' || /^[A-Za-z]:\//.test(normalized) ||
            normalized.indexOf(':') !== -1) {
            return false;
        }
        var parts = normalized.split('/');
        for (var i = 0; i < parts.length; i++) {
            if (parts[i] === '..') return false;
        }
        return true;
    }

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
                return pkgs.length > 0 ? 'go test -race ' + pkgs.map(scopedShellArg).join(' ') : null;
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
                return 'npx jest --passWithNoTests --testPathPattern ' + scopedRegexArg(pattern);
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
                return 'python -m pytest ' + dirList.map(scopedShellArg).join(' ');
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
        for (var safeIndex = 0; safeIndex < files.length; safeIndex++) {
            if (!safeScopedPath(files[safeIndex])) {
                return fallbackCommand;
            }
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
        verifyCommand: cfg.verifyCommand || '',
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
    async function gitExec(dir, args) {
        var cmd = ['git'];
        if (dir && dir !== '' && dir !== '.') {
            cmd.push('-C');
            cmd.push(dir);
        }
        for (var i = 0; i < args.length; i++) {
            cmd.push(args[i]);
        }
        return await exec.execv(cmd);
    }

    // collectSpawnOutput drains a spawned child's stdout and stderr while
    // waiting for its exit status. A stream the exec layer had to cut before EOF
    // ends with truncated: true (a writer outside the process group still held
    // the pipe); that is reported, never hidden, because a prefix of the output
    // is not the command's result.
    //
    // Returns {stdout, stderr, code, signal, truncated}. code falls back to 0
    // when the handle reported no status, matching execv's contract.
    async function collectSpawnOutput(child, outputFn) {
        var truncated = false;

        async function readAll(stream, streamOutputFn) {
            var buf = '';
            while (true) {
                var chunk = await stream.read();
                if (chunk.done) {
                    if (chunk.truncated === true) truncated = true;
                    break;
                }
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

        // Collect stdout and stderr in parallel to avoid deadlock
        // (the command may fill stderr while we drain stdout). wait() is
        // guarded: a failed process-tree close rejects it, and that rejection
        // is an exec-layer failure of the command, not an unhandled rejection
        // of the whole collection.
        var waitResult = {};
        var waitError = null;
        var results = await Promise.all([
            readAll(child.stdout, outputFn),
            readAll(child.stderr, outputFn),
            child.wait().then(function (r) { waitResult = r || {}; }, function (err) { waitError = err; })
        ]);
        var code = (waitResult.code !== undefined) ? waitResult.code : 0;
        if (waitError) {
            code = -1;
        }
        // A cut capture is an exec-layer failure, exactly as execv reports it:
        // the caller gets the prefix it received plus a status it cannot mistake
        // for the command's own. (execv may instead surface the process's own
        // non-zero exit code alongside error true — both shapes are failures;
        // no consumer may treat either as success.)
        var waitMessage = waitError ? String((waitError && waitError.message) || waitError) : '';
        return {
            stdout: results[0],
            stderr: results[1],
            code: truncated || waitError ? -1 : code,
            signal: waitResult.signal || null,
            truncated: truncated,
            error: truncated || waitError || code !== 0 ? true : false,
            message: truncated
                ? 'command output capture was cut short before EOF: the output is incomplete'
                : (waitError
                    ? waitMessage
                    : (code !== 0 ? 'exit status ' + code : ''))
        };
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

        var child = await exec.spawn('git', gitArgs);
        var collected = await collectSpawnOutput(child, outputFn);
        return {
            stdout: collected.stdout,
            stderr: collected.stderr,
            code: collected.code,
            error: collected.error,
            message: collected.message
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
    //   timeoutMs — native child-process deadline; the binding owns termination
    async function shellExecAsync(command, options) {
        options = options || {};
        var outputFn = (options && typeof options.outputFn === 'function') ? options.outputFn : null;
        var timeoutMs = (options && typeof options.timeoutMs === 'number' && options.timeoutMs > 0) ? options.timeoutMs : 0;
        var startedAt = Date.now();

        // Fall back to global output capture function (same as gitExecAsync).
        if (!outputFn && typeof prSplit._outputCaptureFn === 'function') {
            outputFn = prSplit._outputCaptureFn;
        }

        // Emit command header to output callback.
        if (outputFn) {
            outputFn('\u276f ' + command);
        }

        // Use platform shell for command dispatch.
        var child = await shellSpawnAsync(command, {
            timeoutMs: (typeof options.timeoutMs === 'number' && options.timeoutMs > 0) ? options.timeoutMs : 0
        });

        var collected = await collectSpawnOutput(child, outputFn);
        var message = collected.message;
        // A configured timeout that elapsed is a deterministic timeout failure,
        // not an unknown-verdict truncation: evaluate it BEFORE the truncated
        // gate so a native timeoutMs expiry is reported as a timeout even when
        // the killed process's streams were also cut.
        var elapsedMs = Date.now() - startedAt;
        var timedOut = timeoutMs > 0 && elapsedMs >= timeoutMs;
        if (timedOut) {
            message = 'timed out after ' + timeoutMs + 'ms';
        } else if (!collected.truncated && collected.signal) {
            message = String(collected.signal);
        }

        return {
            stdout: collected.stdout,
            stderr: collected.stderr,
            code: collected.code,
            error: collected.error || timedOut,
            message: message
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

    function absolutePath(path) {
        if (osmod && typeof osmod.isAbsolute === 'function') {
            return osmod.isAbsolute(path);
        }
        if (typeof path !== 'string' || path === '') {
            return false;
        }
        if (/^[A-Za-z]:[\\/]/.test(path) || /^\\\\/.test(path)) {
            return true;
        }
        return path.charAt(0) === '/';
    }

    async function fileExistsSync(path) {
        if (osmod && typeof osmod.fileExists === 'function') {
            var r = await osmod.fileExists(path);
            return !!(r && r.exists);
        }
        return isWindows()
            ? (await exec.execv(['cmd.exe', '/C', 'if exist "' + path + '" (exit 0) else (exit 1)'])).code === 0
            : (await exec.execv(['test', '-f', path])).code === 0;
    }

    // lookupBinary checks whether a named binary exists on PATH, or whether an
    // explicit absolute command path exists on disk.
    // Returns {found: bool, path: string} using 'where.exe' on Windows,
    // 'which' on Unix for PATH lookups. found requires a non-empty resolved
    // path, so a lookup whose output was cut short can never report a prefix
    // as a binary.
    async function lookupBinary(name) {
        if (absolutePath(name)) {
            var exists = await fileExistsSync(name);
            return {
                found: exists,
                path: exists ? name : ''
            };
        }
        var cmd = isWindows() ? 'where.exe' : 'which';
        var e = (typeof prSplit !== 'undefined' && prSplit._modules && prSplit._modules.exec) || exec;
        var result = await e.execv([cmd, name]);
        var path = (result.stdout || '').split('\n')[0].trim();
        return {
            found: result.code === 0 && !result.error && path !== '',
            path: path
        };
    }

    // lookupBinaryAsync is the non-blocking version of lookupBinary.
    // Returns a Promise<{found: bool, path: string}>. The spawned lookup drains
    // through the shared collectSpawnOutput, so a stream the exec layer had to
    // cut before EOF is an exec-layer failure (code -1, error true), never a
    // {found: true, path: '<prefix>'}.
    async function lookupBinaryAsync(name) {
        if (absolutePath(name)) {
            var exists = await fileExistsSync(name);
            return {
                found: exists,
                path: exists ? name : ''
            };
        }
        var cmd = isWindows() ? 'where.exe' : 'which';
        // Late-bind exec through prSplit._modules so test compat shims
        // that reassign the global 'exec' are visible here.
        var e = (typeof prSplit !== 'undefined' && prSplit._modules && prSplit._modules.exec) || exec;
        var child = await e.spawn(cmd, [name]);
        var collected = await collectSpawnOutput(child, null);
        var path = (collected.stdout || '').split('\n')[0].trim();
        return {
            found: !collected.truncated && collected.code === 0 && path !== '',
            path: path
        };
    }

    // shellSpawnSync runs a shell command string synchronously through the
    // platform shell (sh -c on Unix, cmd.exe /C on Windows).
    // Returns the exec result object.
    async function shellSpawnSync(shellCmd, opts) {
        opts = opts || {};
        var argv = isWindows()
            ? ['cmd.exe', '/C', shellCmd]
            : ['sh', '-c', shellCmd];
        var execOpts = null;
        if (typeof opts.timeoutMs === 'number' && opts.timeoutMs > 0) {
            execOpts = { timeoutMs: opts.timeoutMs };
        }
        var result = execOpts
            ? await exec.execv(argv, execOpts)
            : await exec.execv(argv);
        if (opts.onStdout && result.stdout) { opts.onStdout(result.stdout); }
        if (opts.onStderr && result.stderr) { opts.onStderr(result.stderr); }
        return result;
    }

    // shellSpawnAsync runs a shell command string asynchronously through the
    // platform shell (sh -c on Unix, cmd.exe /C on Windows).
    // Returns the spawned child process.
    async function shellSpawnAsync(shellCmd, opts) {
        opts = opts || {};
        var spawnOpts = null;
        if (typeof opts.timeoutMs === 'number' && opts.timeoutMs > 0) {
            spawnOpts = { timeoutMs: opts.timeoutMs };
        }
        if (isWindows()) {
            return spawnOpts
                ? await exec.spawn('cmd.exe', ['/C', shellCmd], spawnOpts)
                : await exec.spawn('cmd.exe', ['/C', shellCmd]);
        }
        return spawnOpts
            ? await exec.spawn('sh', ['-c', shellCmd], spawnOpts)
            : await exec.spawn('sh', ['-c', shellCmd]);
    }

    // gitAddChangedFiles stages modified, new, and deleted files while
    // filtering out known pr-split tool artifacts (e.g., .pr-split-plan.json).
    async function gitAddChangedFiles(dir) {
        var status = await gitExec(dir, ['status', '--porcelain']);
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
            var addResult = await gitExec(dir, addArgs);
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

    // execFailReason renders a failed gitExec/gitExecAsync result for an
    // error string. git's stderr is the primary evidence, but a nonzero
    // exit can carry an EMPTY stderr — a silent failure (the process died
    // before writing, a spawn/fork resource error, a bare exit status) —
    // and an error string of just "<command> failed: " is then
    // undiagnosable. The exec layer's message field always holds the
    // Go-side error (including an aborted-capture diagnostic when output
    // was cut short); it is appended whenever it adds information the
    // stderr does not already carry.
    var execFailReason = function(result) {
        var stderr = (result.stderr || '').trim();
        var message = (result.message === undefined || result.message === null) ? '' : String(result.message);
        if (message === '' || stderr.indexOf(message) >= 0) {
            return stderr;
        }
        return stderr === '' ? message : stderr + ' (exec: ' + message + ')';
    };

    // Internal helpers.
    prSplit._collectSpawnOutput = collectSpawnOutput;
    prSplit._gitExec = gitExec;
    prSplit._gitExecAsync = gitExecAsync;
    prSplit._shellExecAsync = shellExecAsync;
    prSplit._resolveDir = resolveDir;
    prSplit._execFailReason = execFailReason;
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
    prSplit._effectiveVerifyTimeoutMs = effectiveVerifyTimeoutMs;

    // Discovery functions.
    prSplit.discoverVerifyCommand = discoverVerifyCommand;
    prSplit.scopedVerifyCommand = scopedVerifyCommand;

    // Cooperative cancellation.
    prSplit.isCancelled = isCancelled;
    prSplit.isPaused = isPaused;
    prSplit.isForceCancelled = isForceCancelled;
})();
