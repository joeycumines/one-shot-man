// Super Document: Visual TUI for merging documents
// This script uses osm:bubbletea and osm:lipgloss to create a full visual TUI application
// with mouse support, keyboard navigation, and styled components.

const tea = require('osm:bubbletea');
const lipgloss = require('osm:lipgloss');
const osm = require('osm:os');

// ============================================================================
// STYLES - Using lipgloss for all visual styling
// ============================================================================

const styles = {
    // Colors
    primary: '#7C3AED',
    secondary: '#10B981', 
    danger: '#EF4444',
    muted: '#6B7280',
    bg: '#1F2937',
    fg: '#F9FAFB',
    
    // Create style objects
    title: function() {
        return lipgloss.newStyle()
            .bold(true)
            .foreground(this.primary)
            .padding(0, 1)
            .marginBottom(1);
    },
    
    selected: function() {
        return lipgloss.newStyle()
            .bold(true)
            .foreground(this.fg)
            .background(this.primary)
            .padding(0, 1);
    },
    
    normal: function() {
        return lipgloss.newStyle()
            .foreground(this.fg)
            .padding(0, 1);
    },
    
    button: function() {
        return lipgloss.newStyle()
            .foreground(this.fg)
            .background(this.secondary)
            .padding(0, 2)
            .marginRight(1);
    },
    
    buttonActive: function() {
        return lipgloss.newStyle()
            .foreground(this.fg)
            .background(this.primary)
            .bold(true)
            .padding(0, 2)
            .marginRight(1);
    },
    
    dangerButton: function() {
        return lipgloss.newStyle()
            .foreground(this.fg)
            .background(this.danger)
            .padding(0, 1);
    },
    
    status: function() {
        return lipgloss.newStyle()
            .foreground(this.secondary)
            .italic(true);
    },
    
    error: function() {
        return lipgloss.newStyle()
            .foreground(this.danger)
            .bold(true);
    },
    
    help: function() {
        return lipgloss.newStyle()
            .foreground(this.muted)
            .marginTop(1);
    },
    
    box: function() {
        return lipgloss.newStyle()
            .border(lipgloss.roundedBorder())
            .borderForeground(this.primary)
            .padding(1, 2);
    },
    
    input: function() {
        return lipgloss.newStyle()
            .border(lipgloss.normalBorder())
            .borderForeground(this.secondary)
            .padding(0, 1);
    },
    
    document: function() {
        return lipgloss.newStyle()
            .border(lipgloss.normalBorder())
            .borderForeground(this.muted)
            .padding(0, 1)
            .marginBottom(1);
    },
    
    documentSelected: function() {
        return lipgloss.newStyle()
            .border(lipgloss.doubleBorder())
            .borderForeground(this.primary)
            .padding(0, 1)
            .marginBottom(1);
    },
    
    preview: function() {
        return lipgloss.newStyle()
            .foreground(this.muted)
            .maxWidth(60);
    }
};

// ============================================================================
// STATE MANAGEMENT - No global state, all state in model
// ============================================================================

const MODE_LIST = 'list';
const MODE_INPUT = 'input';
const MODE_VIEW = 'view';
const MODE_CONFIRM = 'confirm';

// Prompt template constant for easier customization
const PROMPT_HEADER = [
    'Implement a super-document that is INTERNALLY CONSISTENT based on a quorum or carefully-weighed analysis of the attached documents.',
    '',
    'Your goal is to coalesce AS MUCH INFORMATION AS POSSIBLE, in as raw form as possible, while **preserving internal consistency**.',
    '',
    'All information or content that you DON\'T manage to coalesce will be discarded, making it critical that you output as much as the requirement of internal consistency allows.'
];

function createInitialState() {
    return {
        documents: [],
        selectedIdx: 0,
        mode: MODE_LIST,
        inputBuffer: '',
        inputPrompt: '',
        inputField: '',
        width: 80,
        height: 24,
        statusMsg: '',
        clipboard: '',
        hasError: false,
        nextDocId: 1  // Document ID counter kept in state
    };
}

// ============================================================================
// DOCUMENT OPERATIONS
// ============================================================================

function addDocument(state, label, content) {
    const docId = state.nextDocId;
    state.nextDocId++;
    const doc = {
        id: docId,
        label: label || ('Document ' + (state.documents.length + 1)),
        content: content || ''
    };
    state.documents.push(doc);
    return doc;
}

function removeDocument(state, idx) {
    if (idx >= 0 && idx < state.documents.length) {
        const doc = state.documents[idx];
        state.documents.splice(idx, 1);
        if (state.selectedIdx >= state.documents.length && state.selectedIdx > 0) {
            state.selectedIdx--;
        }
        return doc;
    }
    return null;
}

function getSelectedDocument(state) {
    if (state.selectedIdx >= 0 && state.selectedIdx < state.documents.length) {
        return state.documents[state.selectedIdx];
    }
    return null;
}

// ============================================================================
// PROMPT BUILDING
// ============================================================================

function calculateBacktickFence(documents) {
    let maxLen = 0;
    for (let i = 0; i < documents.length; i++) {
        const content = documents[i].content || '';
        let currentRun = 0;
        for (let j = 0; j < content.length; j++) {
            if (content[j] === '`') {
                currentRun++;
                if (currentRun > maxLen) {
                    maxLen = currentRun;
                }
            } else {
                currentRun = 0;
            }
        }
    }
    let fenceLen = maxLen + 1;
    if (fenceLen < 3) fenceLen = 3;
    let fence = '';
    for (let i = 0; i < fenceLen; i++) fence += '`';
    return fence;
}

function buildFinalPrompt(documents) {
    const parts = [];
    
    // Header from constant
    for (let i = 0; i < PROMPT_HEADER.length; i++) {
        parts.push(PROMPT_HEADER[i]);
    }
    parts.push('');
    parts.push('---');
    parts.push('## DOCUMENTS');
    parts.push('---');
    parts.push('');
    
    const fence = calculateBacktickFence(documents);
    
    for (let i = 0; i < documents.length; i++) {
        const doc = documents[i];
        parts.push('Document ' + (i + 1) + ':');
        parts.push(fence);
        parts.push(doc.content);
        parts.push(fence);
        parts.push('');
    }
    
    return parts.join('\n');
}

// ============================================================================
// BUBBLETEA MODEL
// ============================================================================

function init() {
    return createInitialState();
}

function update(msg, state) {
    if (!msg || !msg.type) {
        return [state, null];
    }
    
    switch (msg.type) {
        case 'windowSize':
            state.width = msg.width;
            state.height = msg.height;
            return [state, null];
            
        case 'keyPress':
            return handleKeyPress(msg, state);
            
        case 'mouse':
            return handleMouse(msg, state);
    }
    
    return [state, null];
}

function handleKeyPress(msg, state) {
    switch (state.mode) {
        case MODE_LIST:
            return handleListKeys(msg, state);
        case MODE_INPUT:
            return handleInputKeys(msg, state);
        case MODE_VIEW:
            return handleViewKeys(msg, state);
        case MODE_CONFIRM:
            return handleConfirmKeys(msg, state);
    }
    return [state, null];
}

function handleListKeys(msg, state) {
    const key = msg.key;
    
    switch (key) {
        case 'q':
        case 'ctrl+c':
            return [state, tea.quit()];
            
        case 'up':
        case 'k':
            if (state.selectedIdx > 0) {
                state.selectedIdx--;
            }
            break;
            
        case 'down':
        case 'j':
            if (state.selectedIdx < state.documents.length - 1) {
                state.selectedIdx++;
            }
            break;
            
        case 'a':
            state.mode = MODE_INPUT;
            state.inputPrompt = 'Enter document content (or file:path to load):';
            state.inputField = 'content';
            state.inputBuffer = '';
            break;
            
        case 'l':
            state.mode = MODE_INPUT;
            state.inputPrompt = 'Enter file path:';
            state.inputField = 'file';
            state.inputBuffer = '';
            break;
            
        case 'e':
            if (state.documents.length > 0) {
                const doc = getSelectedDocument(state);
                if (doc) {
                    state.mode = MODE_INPUT;
                    state.inputPrompt = 'Edit content for document #' + doc.id + ':';
                    state.inputField = 'edit';
                    state.inputBuffer = doc.content;
                }
            }
            break;
            
        case 'r':
            if (state.documents.length > 0) {
                const doc = getSelectedDocument(state);
                if (doc) {
                    state.mode = MODE_INPUT;
                    state.inputPrompt = 'Enter new label for document #' + doc.id + ':';
                    state.inputField = 'label';
                    state.inputBuffer = doc.label;
                }
            }
            break;
            
        case 'd':
        case 'backspace':
            if (state.documents.length > 0) {
                const doc = getSelectedDocument(state);
                if (doc) {
                    state.mode = MODE_CONFIRM;
                    state.inputPrompt = 'Delete document #' + doc.id + ' "' + doc.label + '"? (y/n)';
                }
            }
            break;
            
        case 'v':
        case 'enter':
            if (state.documents.length > 0) {
                state.mode = MODE_VIEW;
                state.inputField = '';
            }
            break;
            
        case 'c':
            if (state.documents.length === 0) {
                state.statusMsg = 'No documents to copy!';
                state.hasError = true;
            } else {
                const prompt = buildFinalPrompt(state.documents);
                state.clipboard = prompt;
                state.statusMsg = 'Copied prompt (' + prompt.length + ' chars)';
                state.hasError = false;
            }
            break;
            
        case 'g':
            if (state.documents.length === 0) {
                state.statusMsg = 'No documents to generate!';
                state.hasError = true;
            } else {
                state.clipboard = buildFinalPrompt(state.documents);
                state.statusMsg = 'Generated prompt - press p to preview';
                state.hasError = false;
            }
            break;
            
        case 'p':
            if (state.clipboard) {
                state.mode = MODE_VIEW;
                state.inputField = 'preview';
            }
            break;
            
        case '?':
            state.statusMsg = 'a:add l:load e:edit r:rename d:delete v:view c:copy g:generate q:quit';
            state.hasError = false;
            break;
    }
    
    return [state, null];
}

function handleInputKeys(msg, state) {
    const key = msg.key;
    
    switch (key) {
        case 'enter':
            processInput(state);
            state.mode = MODE_LIST;
            state.inputBuffer = '';
            break;
            
        case 'escape':
            state.mode = MODE_LIST;
            state.inputBuffer = '';
            state.statusMsg = 'Cancelled';
            state.hasError = false;
            break;
            
        case 'backspace':
            if (state.inputBuffer.length > 0) {
                state.inputBuffer = state.inputBuffer.slice(0, -1);
            }
            break;
            
        case 'ctrl+c':
            return [state, tea.quit()];
            
        case 'space':
            state.inputBuffer += ' ';
            break;
            
        default:
            if (key.length === 1) {
                state.inputBuffer += key;
            }
            break;
    }
    
    return [state, null];
}

function handleViewKeys(msg, state) {
    const key = msg.key;
    
    switch (key) {
        case 'escape':
        case 'q':
        case 'enter':
            state.mode = MODE_LIST;
            state.inputField = '';
            break;
            
        case 'ctrl+c':
            return [state, tea.quit()];
    }
    
    return [state, null];
}

function handleConfirmKeys(msg, state) {
    const key = msg.key;
    
    switch (key) {
        case 'y':
        case 'Y':
            const doc = removeDocument(state, state.selectedIdx);
            if (doc) {
                state.statusMsg = 'Deleted document #' + doc.id;
                state.hasError = false;
            }
            state.mode = MODE_LIST;
            break;
            
        case 'n':
        case 'N':
        case 'escape':
            state.mode = MODE_LIST;
            state.statusMsg = 'Cancelled';
            state.hasError = false;
            break;
            
        case 'ctrl+c':
            return [state, tea.quit()];
    }
    
    return [state, null];
}

function handleMouse(msg, state) {
    if (msg.action !== 'press') {
        return [state, null];
    }
    
    const y = msg.y;
    const x = msg.x;
    
    // Document list starts around y=5
    const docStartY = 5;
    if (state.mode === MODE_LIST && y >= docStartY) {
        const docHeight = 3; // Each document takes about 3 lines
        const idx = Math.floor((y - docStartY) / docHeight);
        if (idx >= 0 && idx < state.documents.length) {
            state.selectedIdx = idx;
            
            // Check if clicking on remove button (right side)
            if (x >= state.width - 15 && msg.button === 'left') {
                const doc = getSelectedDocument(state);
                if (doc) {
                    state.mode = MODE_CONFIRM;
                    state.inputPrompt = 'Delete document #' + doc.id + ' "' + doc.label + '"? (y/n)';
                }
            }
        }
    }
    
    // Check button row (near bottom)
    const buttonRowY = state.height - 5;
    if (y >= buttonRowY && y <= buttonRowY + 2) {
        // [Add] button
        if (x >= 2 && x <= 12) {
            state.mode = MODE_INPUT;
            state.inputPrompt = 'Enter document content (or file:path to load):';
            state.inputField = 'content';
            state.inputBuffer = '';
        }
        // [Load] button
        else if (x >= 14 && x <= 28) {
            state.mode = MODE_INPUT;
            state.inputPrompt = 'Enter file path:';
            state.inputField = 'file';
            state.inputBuffer = '';
        }
        // [Copy] button
        else if (x >= 30 && x <= 44) {
            if (state.documents.length > 0) {
                state.clipboard = buildFinalPrompt(state.documents);
                state.statusMsg = 'Copied prompt (' + state.clipboard.length + ' chars)';
                state.hasError = false;
            } else {
                state.statusMsg = 'No documents to copy!';
                state.hasError = true;
            }
        }
    }
    
    return [state, null];
}

function processInput(state) {
    switch (state.inputField) {
        case 'content':
            let content = state.inputBuffer;
            let label = 'Document ' + (state.documents.length + 1);
            
            // Check if it's a file path
            if (content.indexOf('file:') === 0) {
                const path = content.slice(5).trim();
                const result = osm.readFile(path);
                if (result.error) {
                    state.statusMsg = 'Error reading file: ' + result.error;
                    state.hasError = true;
                    return;
                }
                content = result.content;
                label = path;
            }
            
            const doc = addDocument(state, label, content);
            state.statusMsg = 'Added document #' + doc.id;
            state.hasError = false;
            break;
            
        case 'file':
            const path = state.inputBuffer.trim();
            if (!path) {
                state.statusMsg = 'No file path provided';
                state.hasError = true;
                return;
            }
            const result = osm.readFile(path);
            if (result.error) {
                state.statusMsg = 'Error reading file: ' + result.error;
                state.hasError = true;
                return;
            }
            const loadedDoc = addDocument(state, path, result.content);
            state.statusMsg = 'Loaded document #' + loadedDoc.id + ' from ' + path;
            state.hasError = false;
            break;
            
        case 'edit':
            const editDoc = getSelectedDocument(state);
            if (editDoc) {
                editDoc.content = state.inputBuffer;
                state.statusMsg = 'Updated document #' + editDoc.id;
                state.hasError = false;
            }
            break;
            
        case 'label':
            const labelDoc = getSelectedDocument(state);
            if (labelDoc) {
                labelDoc.label = state.inputBuffer;
                state.statusMsg = 'Renamed document #' + labelDoc.id;
                state.hasError = false;
            }
            break;
    }
}

// ============================================================================
// VIEW RENDERING
// ============================================================================

function view(state) {
    switch (state.mode) {
        case MODE_INPUT:
            return renderInputMode(state);
        case MODE_VIEW:
            return renderViewMode(state);
        case MODE_CONFIRM:
            return renderConfirmMode(state);
        default:
            return renderListMode(state);
    }
}

function renderListMode(state) {
    let output = '';
    
    // Title
    output += styles.title().render('📄 Super-Document Builder') + '\n\n';
    
    // Document count
    output += styles.normal().render('Documents: ' + state.documents.length) + '\n\n';
    
    // Document list
    if (state.documents.length === 0) {
        output += styles.help().render("No documents yet. Press 'a' to add or 'l' to load from file.") + '\n';
    } else {
        for (let i = 0; i < state.documents.length; i++) {
            output += renderDocument(state.documents[i], i === state.selectedIdx) + '\n';
        }
    }
    
    // Buttons
    output += '\n';
    output += renderButtons() + '\n';
    
    // Status message
    if (state.statusMsg) {
        output += '\n';
        if (state.hasError) {
            output += styles.error().render(state.statusMsg);
        } else {
            output += styles.status().render(state.statusMsg);
        }
    }
    
    // Help
    output += '\n';
    output += styles.help().render('a:add  l:load  e:edit  r:rename  d:delete  v:view  c:copy  g:generate  ?:help  q:quit');
    
    return output;
}

function renderDocument(doc, selected) {
    // Preview content
    let preview = doc.content || '';
    if (preview.length > 50) {
        preview = preview.slice(0, 50) + '...';
    }
    preview = preview.replace(/\n/g, ' ');
    
    const content = '#' + doc.id + ' [' + doc.label + ']\n' + 
                    styles.preview().render(preview) + '\n' +
                    '[X] Remove';
    
    if (selected) {
        return styles.documentSelected().render(content);
    }
    return styles.document().render(content);
}

function renderButtons() {
    const addBtn = styles.button().render('[A]dd');
    const loadBtn = styles.button().render('[L]oad File');
    const copyBtn = styles.buttonActive().render('[C]opy Prompt');
    
    return lipgloss.joinHorizontal(lipgloss.Top, addBtn, loadBtn, copyBtn);
}

function renderInputMode(state) {
    let output = '';
    
    output += styles.title().render('📝 Input') + '\n\n';
    output += state.inputPrompt + '\n\n';
    output += styles.input().render(state.inputBuffer + '▌') + '\n\n';
    output += styles.help().render('Enter: confirm  Esc: cancel');
    
    return output;
}

function renderViewMode(state) {
    let output = '';
    let content = '';
    let title = '';
    
    if (state.inputField === 'preview') {
        title = '📋 Generated Prompt Preview';
        content = state.clipboard;
    } else {
        const doc = getSelectedDocument(state);
        if (doc) {
            title = '📄 Document #' + doc.id + ': ' + doc.label;
            content = doc.content;
        }
    }
    
    output += styles.title().render(title) + '\n\n';
    
    // Limit content height
    let lines = content.split('\n');
    const maxLines = state.height - 8;
    if (lines.length > maxLines) {
        lines = lines.slice(0, maxLines);
        lines.push('... (content truncated)');
    }
    
    output += styles.box().render(lines.join('\n')) + '\n\n';
    output += styles.help().render("Press Esc, Enter, or 'q' to close");
    
    return output;
}

function renderConfirmMode(state) {
    let output = '';
    
    output += styles.title().render('⚠️ Confirm') + '\n\n';
    output += state.inputPrompt + '\n\n';
    
    const yesBtn = styles.dangerButton().render('[Y]es');
    const noBtn = styles.button().render('[N]o');
    output += lipgloss.joinHorizontal(lipgloss.Top, yesBtn, '  ', noBtn);
    
    return output;
}

// ============================================================================
// MAIN - Run the TUI application
// ============================================================================

// Create and run the bubbletea program with mouse support and alt screen
const model = tea.newModel({
    init: init,
    update: update,
    view: view
});

// Check for errors
if (model.error) {
    throw new Error('Failed to create model: ' + model.error);
}

// Run the TUI (this blocks until the user quits)
const result = tea.run(model, {
    altScreen: true,
    mouse: true
});

if (result && result.error) {
    throw new Error('TUI error: ' + result.error);
}
