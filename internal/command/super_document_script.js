// Super Document: TUI for merging documents into a single internally consistent document
// This script uses the osm:bubbletea and osm:lipgloss modules for the UI

const {buildContext, contextManager} = require('osm:ctxutil');
const nextIntegerId = require('osm:nextIntegerId');
const template = require('osm:text/template');
const osm = require('osm:os');

// Import shared symbols
const shared = require('osm:sharedStateSymbols');

// config.name is injected by Go as "super-document"
const COMMAND_NAME = config.name;

// Define command-specific symbols for state management
const stateKeys = {
    documents: Symbol("documents"),
    selectedIndex: Symbol("selectedIndex"),
    mode: Symbol("mode"),  // 'list', 'edit', 'view'
    editBuffer: Symbol("editBuffer"),
};

// Create the single state accessor
const state = tui.createState(COMMAND_NAME, {
    // Shared keys
    [shared.contextItems]: {defaultValue: []},

    // Command-specific keys
    [stateKeys.documents]: {defaultValue: []},
    [stateKeys.selectedIndex]: {defaultValue: 0},
    [stateKeys.mode]: {defaultValue: "list"},
    [stateKeys.editBuffer]: {defaultValue: ""},
});

// Expose addItem for test access - will be set after ctxmgr is created
let addItem;

// Helper function to calculate the backtick fence length needed to safely escape content
function calculateBacktickFence(contents) {
    let maxLength = 0;
    for (const content of contents) {
        if (!content) continue;
        let currentRun = 0;
        for (const ch of content) {
            if (ch === '`') {
                currentRun++;
                if (currentRun > maxLength) {
                    maxLength = currentRun;
                }
            } else {
                currentRun = 0;
            }
        }
    }
    const fenceLen = Math.max(maxLength + 1, 5);
    return '`'.repeat(fenceLen);
}

// Build the final prompt from documents
function buildFinalPrompt() {
    const documents = state.get(stateKeys.documents) || [];
    const contextItems = state.get(shared.contextItems) || [];
    
    // Build context txtar if we have context items
    let contextTxtar = "";
    if (contextItems.length > 0) {
        contextTxtar = buildContext(contextItems, {toTxtar: () => context.toTxtar()});
    }
    
    // Prepare documents for template
    const docData = documents.map((doc, idx) => ({
        id: doc.id,
        label: doc.label || ("Document " + (idx + 1)),
        content: doc.content || "",
    }));
    
    // Calculate safe fence for all document contents
    const allContents = docData.map(d => d.content);
    const fence = calculateBacktickFence(allContents);
    
    // Build the prompt manually to handle fence safely
    const parts = [
        "Implement a super-document that is INTERNALLY CONSISTENT based on a quorum or carefully-weighed analysis of the attached documents.",
        "",
        "Your goal is to coalesce AS MUCH INFORMATION AS POSSIBLE, in as raw form as possible, while **preserving internal consistency**.",
        "",
        "All information or content that you DON'T manage to coalesce will be discarded, making it critical that you output as much as the requirement of internal consistency allows.",
    ];
    
    if (contextTxtar) {
        parts.push("");
        parts.push("---");
        parts.push("## CONTEXT");
        parts.push("---");
        parts.push("");
        parts.push(contextTxtar);
    }
    
    parts.push("");
    parts.push("---");
    parts.push("## DOCUMENTS");
    parts.push("---");
    parts.push("");
    
    for (let i = 0; i < docData.length; i++) {
        const doc = docData[i];
        parts.push("Document " + (i + 1) + ":");
        parts.push(fence);
        parts.push(doc.content);
        parts.push(fence);
        parts.push("");
    }
    
    return parts.join("\n");
}

// Create context manager for context items
const ctxmgr = contextManager({
    getItems: () => state.get(shared.contextItems) || [],
    setItems: (v) => state.set(shared.contextItems, v),
    nextIntegerId: nextIntegerId,
    buildPrompt: buildFinalPrompt,
});

// Export for test access
addItem = ctxmgr.addItem;

// Document management functions
function getDocuments() {
    return state.get(stateKeys.documents) || [];
}

function setDocuments(docs) {
    state.set(stateKeys.documents, docs);
}

function addDocument(content, label) {
    const docs = getDocuments();
    const id = nextIntegerId();
    const doc = {
        id: id,
        label: label || ("Document " + (docs.length + 1)),
        content: content || "",
    };
    docs.push(doc);
    setDocuments(docs);
    return doc;
}

function removeDocument(id) {
    const docs = getDocuments();
    const idx = docs.findIndex(d => d.id === id);
    if (idx >= 0) {
        docs.splice(idx, 1);
        setDocuments(docs);
        // Adjust selected index if needed
        const selectedIndex = state.get(stateKeys.selectedIndex);
        if (selectedIndex >= docs.length) {
            state.set(stateKeys.selectedIndex, Math.max(0, docs.length - 1));
        }
        return true;
    }
    return false;
}

function updateDocument(id, content, label) {
    const docs = getDocuments();
    const doc = docs.find(d => d.id === id);
    if (doc) {
        if (content !== undefined) doc.content = content;
        if (label !== undefined) doc.label = label;
        setDocuments(docs);
        return true;
    }
    return false;
}

function getSelectedDocument() {
    const docs = getDocuments();
    const idx = state.get(stateKeys.selectedIndex);
    if (idx >= 0 && idx < docs.length) {
        return docs[idx];
    }
    return null;
}

// Build command handlers
function buildCommands() {
    const baseCommands = ctxmgr.commands;
    
    return {
        ...baseCommands,
        
        // Add a new document
        add: {
            description: "Add a new document",
            usage: "add [content] or add --file <path>",
            handler: function(args) {
                if (args.length >= 2 && args[0] === "--file") {
                    // Load from file
                    const filePath = args[1];
                    const result = osm.readFile(filePath);
                    if (result.error) {
                        output.print("Error reading file: " + result.error);
                        return;
                    }
                    const doc = addDocument(result.content, filePath);
                    output.print("Added document #" + doc.id + " from file: " + filePath);
                } else {
                    // Add with content from args
                    const content = args.join(" ");
                    const doc = addDocument(content);
                    output.print("Added document #" + doc.id);
                    if (!content) {
                        output.print("Use 'edit " + doc.id + "' to add content");
                    }
                }
            },
        },
        
        // Remove a document
        remove: {
            description: "Remove a document by ID",
            usage: "remove <id>",
            handler: function(args) {
                if (args.length < 1) {
                    output.print("Usage: remove <id>");
                    return;
                }
                const id = parseInt(args[0], 10);
                if (isNaN(id)) {
                    output.print("Invalid document ID: " + args[0]);
                    return;
                }
                if (removeDocument(id)) {
                    output.print("Removed document #" + id);
                } else {
                    output.print("Document #" + id + " not found");
                }
            },
        },
        
        // Edit a document
        edit: {
            description: "Edit a document's content",
            usage: "edit <id> [new content]",
            handler: function(args) {
                if (args.length < 1) {
                    output.print("Usage: edit <id> [new content]");
                    return;
                }
                const id = parseInt(args[0], 10);
                if (isNaN(id)) {
                    output.print("Invalid document ID: " + args[0]);
                    return;
                }
                const content = args.slice(1).join(" ");
                if (updateDocument(id, content)) {
                    output.print("Updated document #" + id);
                } else {
                    output.print("Document #" + id + " not found");
                }
            },
        },
        
        // View a document
        view: {
            description: "View a document's content",
            usage: "view <id>",
            handler: function(args) {
                if (args.length < 1) {
                    output.print("Usage: view <id>");
                    return;
                }
                const id = parseInt(args[0], 10);
                if (isNaN(id)) {
                    output.print("Invalid document ID: " + args[0]);
                    return;
                }
                const docs = getDocuments();
                const doc = docs.find(d => d.id === id);
                if (doc) {
                    output.print("Document #" + doc.id + " (" + doc.label + "):");
                    output.print("---");
                    output.print(doc.content || "(empty)");
                    output.print("---");
                } else {
                    output.print("Document #" + id + " not found");
                }
            },
        },
        
        // Label a document
        label: {
            description: "Set a document's label",
            usage: "label <id> <new label>",
            handler: function(args) {
                if (args.length < 2) {
                    output.print("Usage: label <id> <new label>");
                    return;
                }
                const id = parseInt(args[0], 10);
                if (isNaN(id)) {
                    output.print("Invalid document ID: " + args[0]);
                    return;
                }
                const label = args.slice(1).join(" ");
                if (updateDocument(id, undefined, label)) {
                    output.print("Updated label for document #" + id + ": " + label);
                } else {
                    output.print("Document #" + id + " not found");
                }
            },
        },
        
        // List all documents
        list: {
            description: "List all documents",
            usage: "list",
            handler: function(args) {
                const docs = getDocuments();
                if (docs.length === 0) {
                    output.print("No documents. Use 'add' to add a document.");
                    return;
                }
                output.print("Documents:");
                for (let i = 0; i < docs.length; i++) {
                    const doc = docs[i];
                    const preview = (doc.content || "").substring(0, 50).replace(/\n/g, " ");
                    const suffix = (doc.content || "").length > 50 ? "..." : "";
                    output.print("  #" + doc.id + " [" + doc.label + "]: " + preview + suffix);
                }
            },
        },
        
        // Load document from file
        load: {
            description: "Load a document from a file",
            usage: "load <path> [label]",
            handler: function(args) {
                if (args.length < 1) {
                    output.print("Usage: load <path> [label]");
                    return;
                }
                const filePath = args[0];
                const label = args.slice(1).join(" ") || filePath;
                const result = osm.readFile(filePath);
                if (result.error) {
                    output.print("Error reading file: " + result.error);
                    return;
                }
                const doc = addDocument(result.content, label);
                output.print("Loaded document #" + doc.id + " from: " + filePath);
            },
        },
        
        // Generate the final prompt
        generate: {
            description: "Generate the final super-document prompt",
            usage: "generate",
            handler: function(args) {
                const docs = getDocuments();
                if (docs.length === 0) {
                    output.print("No documents to generate from. Use 'add' or 'load' to add documents.");
                    return;
                }
                const prompt = buildFinalPrompt();
                output.print("Generated Super-Document Prompt:");
                output.print("=".repeat(40));
                output.print(prompt);
                output.print("=".repeat(40));
            },
        },
        
        // Copy the final prompt to clipboard (via output for now)
        copy: {
            description: "Copy the generated prompt (outputs for manual copy)",
            usage: "copy",
            handler: function(args) {
                const docs = getDocuments();
                if (docs.length === 0) {
                    output.print("No documents to copy. Use 'add' or 'load' to add documents.");
                    return;
                }
                const prompt = buildFinalPrompt();
                output.print(prompt);
                output.print("");
                output.print("--- Copy the above content ---");
            },
        },
        
        // Clear all documents
        clear: {
            description: "Clear all documents",
            usage: "clear",
            handler: function(args) {
                setDocuments([]);
                state.set(stateKeys.selectedIndex, 0);
                output.print("Cleared all documents");
            },
        },
    };
}

// Register all commands
const commands = buildCommands();

// Register modes
tui.registerMode({
    name: COMMAND_NAME,
    tui: {
        title: "Super Document",
        prompt: "super-document> ",
        enableHistory: true,
        historyFile: ".super-document_history"
    },
    onEnter: function() {
        // Show welcome message
        output.print("Super-Document - TUI for merging documents");
        output.print("Type 'help' for available commands, 'quit' to exit");

        // List existing documents if any
        const existingDocs = getDocuments();
        if (existingDocs.length > 0) {
            output.print("Loaded " + existingDocs.length + " existing document(s)");
            commands.list.handler([]);
        }
    },
    commands: function() {
        return buildCommands();
    }
});

// Switch to document mode
tui.switchMode(COMMAND_NAME);
