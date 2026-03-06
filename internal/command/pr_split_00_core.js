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
    // -----------------------------------------------------------------------
    //  Module imports
    // -----------------------------------------------------------------------

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

    // -----------------------------------------------------------------------
    //  Styled Output Helpers
    // -----------------------------------------------------------------------

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

    // -----------------------------------------------------------------------
    //  Configuration & Constants
    // -----------------------------------------------------------------------

    // Read injected configuration with defaults.
    var cfg = (typeof prSplitConfig !== 'undefined') ? prSplitConfig : {};
    var COMMAND_NAME = (typeof config !== 'undefined' && config.name) ? config.name : 'pr-split';
    var MODE_NAME = 'pr-split';

    // -----------------------------------------------------------------------
    //  Discovery & Scoping
    // -----------------------------------------------------------------------

    // discoverVerifyCommand auto-detects the verification command for the
    // working directory. Checks for Makefile, makefile, and GNUmakefile.
    // Prefers 'gmake' (GNU Make on macOS) over 'make' if available.
    // Returns the best make command, or '' if no Makefile found.
    function discoverVerifyCommand(dir) {
        var names = ['Makefile', 'makefile', 'GNUmakefile'];
        var hasMakefile = false;
        for (var i = 0; i < names.length; i++) {
            var path = dir ? (dir + '/' + names[i]) : names[i];
            if (osmod && osmod.fileExists(path)) {
                hasMakefile = true;
                break;
            } else if (!osmod) {
                var result = exec.execv(['test', '-f', path]);
                if (result.code === 0) { hasMakefile = true; break; }
            }
        }
        if (!hasMakefile) return '';

        // Prefer gmake (GNU Make) if available — required on macOS where
        // the system 'make' is BSD Make 3.81 which cannot run GNU Makefiles.
        var gmakeCheck = exec.execv(['gmake', '--version']);
        if (gmakeCheck.code === 0) return 'gmake';

        return 'make';
    }

    // scopedVerifyCommand returns a verification command scoped to the Go
    // packages affected by the given files. If any non-Go file is present,
    // returns the fallback command (full verification). The scoped command
    // runs `go test -race` on each unique package directory.
    function scopedVerifyCommand(files, fallbackCommand) {
        if (!files || files.length === 0) {
            return fallbackCommand;
        }

        // Only scope when the fallback is a known build/test runner.
        // Custom verify commands (e.g. 'true', 'echo ok') must not be replaced.
        var scopable = (fallbackCommand === 'make' ||
                        (typeof fallbackCommand === 'string' && fallbackCommand.indexOf('go test') >= 0));
        if (!scopable) {
            return fallbackCommand;
        }

        var pkgDirs = {};
        for (var i = 0; i < files.length; i++) {
            var f = files[i];
            // Non-Go files → fall back to full verification.
            if (!f || !f.match || !f.match(/\.go$/)) {
                return fallbackCommand;
            }
            // Extract directory: "internal/cmd/foo.go" → "./internal/cmd/..."
            var lastSlash = f.lastIndexOf('/');
            var dir = lastSlash >= 0 ? './' + f.substring(0, lastSlash) + '/...' : './...';
            pkgDirs[dir] = true;
        }

        var pkgs = Object.keys(pkgDirs).sort();
        if (pkgs.length === 0) {
            return fallbackCommand;
        }

        return 'go test -race ' + pkgs.join(' ');
    }

    // -----------------------------------------------------------------------
    //  Mutable Runtime Config
    // -----------------------------------------------------------------------

    var runtime = {
        baseBranch:    cfg.baseBranch    || 'main',
        dir:           cfg.dir           || '.',
        strategy:      cfg.strategy      || 'directory',
        maxFiles:      cfg.maxFiles      || 10,
        branchPrefix:  cfg.branchPrefix  || 'split/',
        verifyCommand: cfg.verifyCommand || discoverVerifyCommand('.') || 'make',
        dryRun:        cfg.dryRun        || false,
        jsonOutput:    cfg.jsonOutput    || false,
        mode:          cfg.mode          || 'heuristic',
        retryBudget:   typeof cfg.retryBudget === 'number' ? cfg.retryBudget : 3,
        view:          cfg.view          || 'toggle',
        autoMerge:     cfg.autoMerge     || false,
        mergeMethod:   cfg.mergeMethod   || 'squash'
    };

    // -----------------------------------------------------------------------
    //  Internal Helpers
    // -----------------------------------------------------------------------

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

    // shellQuote wraps a string in single quotes, escaping embedded quotes.
    function shellQuote(s) {
        return "'" + s.replace(/'/g, "'\\''") + "'";
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

    // dirname extracts the directory at the given depth from a file path.
    function dirname(filepath, depth) {
        depth = depth || 1;
        var parts = filepath.split('/');
        if (parts.length <= 1) {
            return '.';
        }
        var taken = parts.slice(0, Math.min(depth, parts.length - 1));
        return taken.join('/');
    }

    // fileExtension returns the file extension including the dot, or ''.
    function fileExtension(filepath) {
        var base = filepath.split('/').pop();
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

    // -----------------------------------------------------------------------
    //  Cooperative Cancellation (via injectable callback)
    // -----------------------------------------------------------------------
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

    // -----------------------------------------------------------------------
    //  Initialize globalThis.prSplit
    // -----------------------------------------------------------------------

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
    prSplit._resolveDir = resolveDir;
    prSplit._shellQuote = shellQuote;
    prSplit._gitAddChangedFiles = gitAddChangedFiles;
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
