// contextManager: Factory function for creating reusable context management patterns
//
// This factory provides a standard pattern for managing context items (files, diffs, notes)
// and building REPL commands for interactive modes. It's designed to be composable and extensible.
//
// Usage:
//   const ctxmgr = contextManager({
//     getItems: function() { return tui.getState("items") || []; },
//     setItems: function(v) { tui.setState("items", v); },
//     nextIntegerId: require('osm:nextIntegerId'),
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
        const _nextIntegerId = require('osm:nextIntegerId');
        const {openEditor: _openEditor, clipboardCopy: _clipboardCopy, fileExists: _fileExists} = require('osm:os');
        const {formatArgv: _formatArgv, parseArgv: _parseArgv} = require('osm:argv');
        const {execv: _execv} = require('osm:exec');

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

        // Hot-snippet configuration
        const hotSnippets = options.hotSnippets || [];
        const noSnippetWarning = !!options.noSnippetWarning;

        // Build standard commands
        function buildCommands() {
            var cmds = {
                add: {
                    description: "Add file content to context",
                    usage: "add [--from-diff [commit-spec]] [file ...]",
                    argCompleters: ["file", "flag"],
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
                    usage: "diff [commit-spec]",
                    argCompleters: ["gitref"],
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
                    argCompleters: ["file"],
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
                        const id = parseInt(args[0], 10);
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
                    description: "Remove a context item by id",
                    usage: "remove <id>",
                    handler: function (args) {
                        if (args.length < 1) {
                            output.print("Usage: remove <id>");
                            return;
                        }
                        const id = parseInt(args[0], 10);
                        const list = getItems();
                        const idx = list.findIndex(x => x.id === id);
                        if (idx === -1) {
                            output.print("Not found: " + id);
                            return;
                        }
                        const it = list[idx];
                        if (it.type === 'file' && it.label) {
                            try {
                                const err = context.removePath(it.label);
                                if (err) {
                                    const msg = (err && err.message) ? err.message : ("" + err);
                                    // If the underlying remove failed due to the path being absent,
                                    // treat this as non-fatal: allow the item to be removed from
                                    // the persisted list so users can tidy up stale references.
                                    if (msg.indexOf('path not found') !== -1 || msg.indexOf('no such file') !== -1) {
                                        output.print("Info: file not present, removing from session state: " + it.label);
                                    } else {
                                        output.print("Error: " + msg);
                                        return;
                                    }
                                }
                            } catch (e) {
                                const msg = (e && e.message) ? e.message : ("" + e);
                                if (msg.indexOf('path not found') !== -1 || msg.indexOf('no such file') !== -1) {
                                    output.print("Info: file not present, removing from session state: " + it.label);
                                } else {
                                    output.print("Error: " + e);
                                    return;
                                }
                            }
                        }
                        list.splice(idx, 1);
                        setItems(list);
                        output.print("Removed [" + id + "]");
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
                            output.print("Prompt copied to clipboard.");
                            if (postCopyHint) {
                                output.print(postCopyHint);
                            }
                        } catch (e) {
                            output.print("Clipboard error: " + (e && e.message ? e.message : e));
                        }
                    }
                }
            };

            // Add hot-snippet commands
            for (var si = 0; si < hotSnippets.length; si++) {
                (function(snippet) {
                    cmds[snippet.name] = {
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
                description: "List available hot-snippets",
                handler: function () {
                    if (hotSnippets.length === 0) {
                        output.print("No hot-snippets configured.");
                        return;
                    }
                    for (var i = 0; i < hotSnippets.length; i++) {
                        var s = hotSnippets[i];
                        var marker = s.builtin ? " [embedded]" : "";
                        var preview = s.description || (s.text.length > 50 ? s.text.substring(0, 50) + "..." : s.text);
                        output.print(s.name + marker + " - " + preview);
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
