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

        // Build standard commands
        function buildCommands() {
            return {
                add: {
                    description: "Add file content to context",
                    usage: "add [file ...]",
                    argCompleters: ["file"],
                    handler: function (args) {
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
                        if (list[idx].type === 'lazy-diff') {
                            const initial = Array.isArray(list[idx].payload) ? formatArgv(list[idx].payload) : (list[idx].payload || "");
                            const edited = openEditor("diff-spec-" + id, initial);
                            const argv = parseArgv((edited || "").trim());
                            // Store an empty payload to represent "no args provided" so
                            // runtime defaults can be applied when generating the prompt.
                            list[idx].payload = argv.length ? argv : [];
                            list[idx].label = argv.length ? "git diff " + formatArgv(list[idx].payload) : "git diff (default: HEAD)";
                            setItems(list);
                            output.print("Updated diff specification [" + id + "]");
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
                        output.print(buildPrompt());
                    }
                },
                copy: {
                    description: "Copy prompt to clipboard",
                    handler: function () {
                        const text = buildPrompt();
                        try {
                            clipboardCopy(text);
                            output.print("Prompt copied to clipboard.");
                        } catch (e) {
                            output.print("Clipboard error: " + (e && e.message ? e.message : e));
                        }
                    }
                }
            };
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
            commands: buildCommands()
        };
    }

    // Export the factory function
    exports.contextManager = contextManager;
})(typeof module !== 'undefined' && module.exports ? module.exports : this);
