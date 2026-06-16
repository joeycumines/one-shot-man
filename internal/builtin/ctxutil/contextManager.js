// contextManager: Factory function for creating reusable context management patterns
//
// This factory provides a standard pattern for managing context items (files, diffs, notes)
// and building REPL commands for interactive modes. It's designed to be composable and extensible.
//
// Usage:
//   const ctxmgr = contextManager({
//     getItems: function() { return tui.getState("items") || []; },
//     setItems: function(v) { tui.setState("items", v); },
//     nextIntegerId: require('osm:nextIntegerID'),
//     addItem: function(type, label, payload) { ... }
//   });
//
//   // Access standard commands
//   const commands = {
//     ...ctxmgr.commands,
//     // Override or extend as needed
//     note: {
//       ...ctxmgr.commands.note,
//       handler: function(args) {
//         // Custom logic
//         return ctxmgr.commands.note.handler(args);
//       }
//     }
//   };

(function (exports) {
    'use strict';

    function contextManager(options) {
        // N.B. These can't be until after init is complete.
        const _nextIntegerId = require('osm:nextIntegerID');
        const {openEditor: _openEditor, clipboardCopy: _clipboardCopy, fileExists: _fileExists} = require('osm:os');
        const {formatArgv: _formatArgv, parseArgv: _parseArgv} = require('osm:argv');
        const {execv: _execv} = require('osm:exec');
        const {count: _tokenCount, byteCount: _byteCount, lineCount: _lineCount} = require('osm:tokenizer');
        const _fmt = require('osm:format');

        options = options || {};

        const getItems = options.getItems || function () {
            throw new Error("getItems must be provided");
        };
        const setItems = options.setItems || function (items) {
            throw new Error("setItems must be provided");
        };
        const nextIntegerId = options.nextIntegerId || _nextIntegerId;

        const addItem = options.addItem || function (type, label, payload) {
            const list = getItems();
            const id = nextIntegerId(list);
            list.push({id: id, type: type, label: label, payload: payload});
            setItems(list);
            return id;
        };

        const buildPrompt = options.buildPrompt || function () {
            throw new Error("buildPrompt must be provided");
        };

        const openEditor = options.openEditor || _openEditor;

        const clipboardCopy = options.clipboardCopy || _clipboardCopy;

        const fileExists = options.fileExists || _fileExists;

        const formatArgv = options.formatArgv || _formatArgv;

        const parseArgv = options.parseArgv || _parseArgv;

        const execv = options.execv || _execv;

        // ---- copy notification uses osm:format module ----

        // Refresh all file-type context items to pick up new files in directories
        // and updated content. Errors are silently ignored (e.g., deleted files).
        function _refreshFileItems(getItems) {
            if (typeof context === 'undefined' || !context || typeof context.refreshPath !== 'function') {
                return;
            }
            for (const it of getItems()) {
                if (it.type === 'file' && it.label) {
                    try {
                        context.refreshPath(it.label);
                    } catch (e) {
                        // Ignore errors from deleted/inaccessible files
                    }
                }
            }
        }

        // Post-copy hint: if set, printed after successful copy
        const postCopyHint = options.postCopyHint || "";

        // Hot-snippet configuration: if not explicitly provided, auto-detect
        // from CONFIG_HOT_SNIPPETS global (injected by Go via injectConfigHotSnippets).
        var hotSnippets;
        if ('hotSnippets' in options) {
            hotSnippets = options.hotSnippets || [];
        } else if (typeof CONFIG_HOT_SNIPPETS !== 'undefined' && Array.isArray(CONFIG_HOT_SNIPPETS) && CONFIG_HOT_SNIPPETS.length > 0) {
            hotSnippets = CONFIG_HOT_SNIPPETS;
        } else {
            hotSnippets = [];
        }
        const noSnippetWarning = !!options.noSnippetWarning;

        function _parseDecimalInteger(arg) {
            const s = String(arg).trim();
            if (!/^[+-]?\d+$/.test(s)) {
                return NaN;
            }
            return parseInt(s, 10);
        }

        function _missingPathMessage(message) {
            return message.indexOf('path not found') !== -1 || message.indexOf('no such file') !== -1;
        }

        function _removeErrorMessage(value) {
            return (value && value.message) ? value.message : ("" + value);
        }

        function _canRemoveFileItem(it) {
            if (it.type !== 'file' || !it.label) {
                return true;
            }
            try {
                const err = context.removePath(it.label);
                if (!err) {
                    return true;
                }
                const msg = _removeErrorMessage(err);
                if (_missingPathMessage(msg)) {
                    output.print("Info: file not present, removing from session state: " + it.label);
                    return true;
                }
                output.print("Error: " + msg);
                return false;
            } catch (e) {
                const msg = _removeErrorMessage(e);
                if (_missingPathMessage(msg)) {
                    output.print("Info: file not present, removing from session state: " + it.label);
                    return true;
                }
                output.print("Error: " + msg);
                return false;
            }
        }

        // Build standard commands
        function buildCommands() {
            var cmds = {
                add: {
                    description: "Add file content to context",
                    usage: "add [--from-diff [commit-spec]] [file ...]",
                    argCompleters: ["file", "gitref", "flag"],
                    flagDefs: [
                        {name: "from-diff", description: "Add all files changed in a git diff"}
                    ],
                    handler: function (args) {
                        // Handle --from-diff flag
                        if (args.length > 0 && args[0] === "--from-diff") {
                            var argv = ["git", "diff", "--name-only"];
                            for (var i = 1; i < args.length; i++) {
                                argv.push(args[i]);
                            }
                            var result = execv(argv);
                            if (result.error) {
                                output.print("git diff --name-only failed: " + result.message);
                                return;
                            }
                            var paths = result.stdout.split(/\r?\n/)
                                .map(function(s) { return s.trim(); })
                                .filter(function(s) { return s !== ""; });
                            if (paths.length === 0) {
                                output.print("No files found in diff.");
                                return;
                            }
                            for (var j = 0; j < paths.length; j++) {
                                try {
                                    var err = context.addPath(paths[j]);
                                    if (err && err.message) {
                                        output.print("add error: " + err.message);
                                        continue;
                                    }
                                    addItem("file", paths[j], "");
                                    output.print("Added file: " + paths[j]);
                                } catch (e) {
                                    output.print("add error: " + e);
                                }
                            }
                            return;
                        }
                        if (args.length === 0) {
                            const edited = openEditor("paths", "\n# one path per line\n");
                            args = edited.split(/\r?\n/).map(s => s.trim()).filter(s => s && !s.startsWith('#'));
                        }
                        for (const p of args) {
                            try {
                                const err = context.addPath(p);
                                if (err && err.message) {
                                    output.print("add error: " + err.message);
                                    continue;
                                }
                                addItem("file", p, "");
                                output.print("Added file: " + p);
                            } catch (e) {
                                output.print("add error: " + e);
                            }
                        }
                    }
                },
                diff: {
                    description: "Add git diff output to context (default: HEAD)",
                    usage: "diff [--staged|--stat|--name-only] [commit-spec]",
                    argCompleters: ["gitref", "flag"],
                    flagDefs: [
                        {name: "staged", description: "Staged changes (index vs HEAD)"},
                        {name: "stat", description: "Show diffstat summary"},
                        {name: "name-only", description: "Show only names of changed files"}
                    ],
                    handler: function (args) {
                        // Don't bake the runtime default into the stored payload. Store an
                        // empty payload to indicate "no args provided" and let the Go
                        // runtime decide the most appropriate default (HEAD if it yields
                        // a useful diff, otherwise fall back to legacy behaviour).
                        // Note: an empty array payload is a deliberate signal meaning
                        // "use the runtime default when executing". Consumers should
                        // treat stored payloads as read-only and avoid mutating them
                        // while executing lazy diffs.
                        const argv = (args && args.length > 0) ? args.slice(0) : [];
                        const label = argv.length ? "git diff " + formatArgv(argv) : "git diff (default: HEAD)";
                        addItem("lazy-diff", label, argv);
                        output.print("Added diff: " + label + " (will be executed when generating prompt)");
                    }
                },
                exec: {
                    description: "Add command output to context (lazy: re-executed at prompt time)",
                    usage: "exec <command> [args...]",
                    argCompleters: ["executable"],
                    handler: function (args) {
                        if (args.length === 0) {
                            output.print("Usage: exec <command> [args...]");
                            return;
                        }
                        var argv = args.slice(0);
                        var label = formatArgv(argv);
                        addItem("lazy-exec", label, argv);
                        output.print("Added exec: " + label + " (will be executed when generating prompt)");
                    }
                },
                note: {
                    description: "Add a freeform note",
                    usage: "note [text]",
                    handler: function (args) {
                        let text = args.join(" ");
                        if (!text) text = openEditor("note", "");
                        const id = addItem("note", "note", text);
                        output.print("Added note [" + id + "]");
                    }
                },
                list: {
                    description: "List context items",
                    handler: function () {
                        for (const it of getItems()) {
                            let line = "[" + it.id + "] [" + it.type + "] " + (it.label || "");
                            if (it.type === 'file' && it.label && !fileExists(it.label)) {
                                line += " (missing)";
                            }
                            output.print(line);
                        }
                    }
                },
                edit: {
                    description: "Edit context item by id",
                    usage: "edit <id>",
                    handler: function (args) {
                        if (args.length < 1) {
                            output.print("Usage: edit <id>");
                            return;
                        }
                        const id = _parseDecimalInteger(args[0]);
                        if (isNaN(id)) {
                            output.print("Invalid id: " + args[0]);
                            return;
                        }
                        const list = getItems();
                        const idx = list.findIndex(x => x.id === id);
                        if (idx === -1) {
                            output.print("Not found: " + id);
                            return;
                        }
                        if (list[idx].type === 'file') {
                            output.print("Editing file content directly is not supported. Please edit the file on disk.");
                            return;
                        }
                        if (list[idx].type === 'lazy-diff' || list[idx].type === 'lazy-exec') {
                            const isExec = list[idx].type === 'lazy-exec';
                            const initial = Array.isArray(list[idx].payload) ? formatArgv(list[idx].payload) : (list[idx].payload || "");
                            const editorTitle = (isExec ? "exec" : "diff") + "-spec-" + id;
                            const edited = openEditor(editorTitle, initial);
                            const argv = parseArgv((edited || "").trim());
                            if (isExec && argv.length === 0) {
                                output.print("Command cannot be empty");
                                return;
                            }
                            // For lazy-diff, store an empty payload to represent "no args
                            // provided" so runtime defaults can be applied when generating
                            // the prompt. For lazy-exec, store the argv as-is.
                            list[idx].payload = isExec ? argv : (argv.length ? argv : []);
                            if (isExec) {
                                list[idx].label = formatArgv(list[idx].payload);
                            } else {
                                list[idx].label = argv.length ? "git diff " + formatArgv(list[idx].payload) : "git diff (default: HEAD)";
                            }
                            setItems(list);
                            output.print("Updated " + (isExec ? "exec" : "diff") + " specification [" + id + "]");
                            return;
                        }
                        const edited = openEditor("item-" + id, list[idx].payload || "");
                        list[idx].payload = edited;
                        setItems(list);
                        output.print("Edited [" + id + "]");
                    }
                },
                remove: {
                    description: "Remove context items by id",
                    usage: "remove <id> [id ...]",
                    handler: function (args) {
                        if (!args || args.length < 1) {
                            output.print("Usage: remove <id> [id ...]");
                            return;
                        }

                        // Removal is best-effort and per-item. A failed file-path removal only
                        // skips that item; successes collected so far are still committed to the
                        // persisted list. This mirrors the pre-multi-id contract and avoids
                        // leaving dangling items whose files were already removed.
                        const list = getItems();
                        const removeIndices = [];
                        const seen = new Set();
                        for (const arg of args) {
                            const id = _parseDecimalInteger(arg);
                            if (isNaN(id)) {
                                output.print("Invalid id: " + arg);
                                continue;
                            }
                            if (seen.has(id)) {
                                continue;
                            }
                            seen.add(id);

                            const idx = list.findIndex(x => x.id === id);
                            if (idx === -1) {
                                output.print("Not found: " + id);
                                continue;
                            }

                            const it = list[idx];
                            if (!_canRemoveFileItem(it)) {
                                continue;
                            }

                            removeIndices.push(idx);
                            output.print("Removed [" + id + "]");
                        }

                        if (removeIndices.length === 0) {
                            return;
                        }

                        const next = list.slice();
                        for (const idx of removeIndices.sort((a, b) => b - a)) {
                            next.splice(idx, 1);
                        }
                        setItems(next);
                    }
                },
                show: {
                    description: "Show the prompt",
                    handler: function () {
                        _refreshFileItems(getItems);
                        output.print(buildPrompt());
                    }
                },
                copy: {
                    description: "Copy prompt to clipboard",
                    handler: function () {
                        _refreshFileItems(getItems);
                        const text = buildPrompt();
                        try {
                            clipboardCopy(text);
                            const tokCnt = _tokenCount(text);
                            const lineCnt = _lineCount(text);
                            const byteCnt = _byteCount(text);
                            const byteStr = _fmt.formatBytes(byteCnt);
                            output.print(
                                "Prompt copied to clipboard. \u2502 " + _fmt.formatNum(tokCnt) + " tokens \u00b7 " +
                                lineCnt + " lines \u00b7 " + byteStr + " \u2502"
                            );
                            if (postCopyHint) {
                                output.print(postCopyHint);
                            }
                        } catch (e) {
                            output.print("Clipboard error: " + (e && e.message ? e.message : e));
                        }
                    }
                }
            };

            // Add hot-snippet commands (prefixed with "hot-" to avoid collisions)
            for (var si = 0; si < hotSnippets.length; si++) {
                (function(snippet) {
                    var cmdName = "hot-" + snippet.name;
                    cmds[cmdName] = {
                        description: snippet.description || ("Hot snippet: " + snippet.name),
                        handler: function () {
                            if (snippet.builtin && !noSnippetWarning) {
                                output.print("Note: Using embedded snippet '" + snippet.name + "'. Override in config to customize.");
                            }
                            try {
                                clipboardCopy(snippet.text);
                                output.print("Copied snippet '" + snippet.name + "' to clipboard.");
                            } catch (e) {
                                output.print("Clipboard error: " + (e && e.message ? e.message : e));
                            }
                        }
                    };
                })(hotSnippets[si]);
            }

            // Add snippets listing command
            cmds['snippets'] = {
                description: "List available hot-snippets (use hot-<name> to copy)",
                handler: function () {
                    if (hotSnippets.length === 0) {
                        output.print("No hot-snippets configured.");
                        return;
                    }
                    for (var i = 0; i < hotSnippets.length; i++) {
                        var s = hotSnippets[i];
                        var marker = s.builtin ? " [embedded]" : "";
                        var preview = s.description || (s.text.length > 50 ? s.text.substring(0, 50) + "..." : s.text);
                        output.print("hot-" + s.name + marker + " - " + preview);
                    }
                }
            };

            return cmds;
        }

        // Return the context manager object
        return {
            getItems: getItems,
            setItems: setItems,
            nextIntegerId: nextIntegerId,
            addItem: addItem,
            buildPrompt: buildPrompt,
            openEditor: openEditor,
            clipboardCopy: clipboardCopy,
            fileExists: fileExists,
            formatArgv: formatArgv,
            parseArgv: parseArgv,
            execv: execv,
            commands: buildCommands()
        };
    }

    // Export the factory function
    exports.contextManager = contextManager;
})(typeof module !== 'undefined' && module.exports ? module.exports : this);
