// ============================================================================
// DESIGN SPECIFICATION (UI Layout & Architecture)
// ============================================================================
//
// 1. ARCHITECTURAL & LIFECYCLE SPECIFICATION
// ------------------------------------------
//
// - RUNTIME HOST: The TUI runs *inside* the `go-prompt` shell. It is not a
//   separate process. The `osm super-document` command initializes this view.
// - SHELL INTEGRATION: The "Open Shell" action (Hot: 's') / exits the TUI to
//   "drop into" a shell-like prompt. The state is preserved, and _mutable_ via
//   this prompt. The shell variant has more features, including the ability to
//   add files (with completion). A command (e.g., `tui`) restores the
//   full-screen view.
// - VIEWPORT STRATEGY: The UI is divided into
//   Scrollable Header+Content+Dynamic Button Area, and Fixed Footer.
// - INPUT STRATEGY: Input fields act as a "Form" with Tab-based focus cycling.
//
// 2. VISUAL LAYOUT (Responsive ASCII Blueprints)
// ----------------------------------------------
//
// SCENARIO A: WIDE TERMINAL (Standard Layout)
// +==============================================================================+
// |    📄 Super-Document Builder                                                 |
// |                                                                              |
// |   Documents: 4                                                               |
// | ┌──────────────────────────────────────────────────────────────────────────┐ |
// | │ ╔══════════════════════════════════════════════════════════════════════╗ │ |
// | │ ║ #1 [project-brief.md] (Optional Label)                    [X] Remove ║ │ |
// | │ ║                                                                      ║ │ |
// | │ ║  Preview text of the document content...                             ║ │ |
// | │ ╚══════════════════════════════════════════════════════════════════════╝ │ |
// | │ ┌──────────────────────────────────────────────────────────────────────┐ │ |
// | │ │ #2 (No Label)                                             [X] Remove │ │ |
// | │ │                                                                      │ │ |
// | │ │  Another document content preview...                                 │ │ |
// | │ └──────────────────────────────────────────────────────────────────────┘ │ |
// | │                                                                          │ |
// | │  ( ... documents 3 & 4 off-screen, accessible via Scroll/PgUp/PgDn ... ) │ |
// | └──────────────────────────────────────────────────────────────────────────┘ |
// |                                                                              |
// |  [A]dd  [L]oad  [C]opy  [S]hell                                              |
// |                                                                              |
// | ✓ Status: Ready                                                              |
// | ──────────────────────────────────────────────────────────────────────────── |
// |  a: Add  l: Load  e: Edit  d: Delete  c: Copy  s: Shell  q: Quit             |
// +==============================================================================+
//
// SCENARIO B: NARROW/VERTICAL TERMINAL (Responsive Stack)
// +==============================================================================+
// | Docs: 4                                                                      |
// | ┌──────────────────────────────────┐                                         |
// | │ ╔══════════════════════════════╗ │                                         |
// | │ ║ #1 [Label]        [X] Remove ║ │                                         |
// | │ ║                              ║ │                                         |
// | │ ║  Preview...                  ║ │                                         |
// | │ ╚══════════════════════════════╝ │                                         |
// | └──────────────────────────────────┘                                         |
// |                                                                              |
// |  ( [A]dd      ) ( [L]oad     )       <-- (Standard)                          |
// |  ( [Q]uit     )                      <-- (Standard, TODO REFLOW/REDESIGN)    |
// |  ( [C]opy     ) ( [S]hell    )       <-- (C: Purple BG / S: Orange BG)       |
// |                                                                              |
// | ✓ Status: Ready                                                              |
// | ──────────────────────────────────                                           |
// |  a: Add  l: Load  e: Edit                                                    |
// |  d: Del   c: Copy                                                            |
// |  s: Shell q: Quit                                                            |
// +==============================================================================+
//
// SCENARIO C: INPUT FORM (Textarea Mode)
// +==============================================================================+
// |    📝 Add / Edit Document                                                    |
// |                                                                              |
// |  Label (Optional):                                                           |
// |  ┌────────────────────────────────────────────────────────────────────────┐  |
// |  │ my-custom-label                                                        │  |
// |  └────────────────────────────────────────────────────────────────────────┘  |
// |                                                                              |
// |  Content (Multi-line):                                                       |
// |  ┌────────────────────────────────────────────────────────────────────────┐  |
// |  │ > Cursor starts here.                                                  │  |
// |  │                                                                        │  |
// |  │   Indentation is preserved.                                            │  |
// |  └────────────────────────────────────────────────────────────────────────┘  |
// |                                                                              |
// |             [    Submit    ]                  [    Cancel    ]               |
// |                                                                              |
// |  Tab: Cycle Focus    Enter: Newline (Text) / Submit (Button)    Esc: Cancel  |
// +==============================================================================+
//
// 3. RESPONSIVE LAYOUT & LIPGLOSS STRATEGY
// ----------------------------------------
// The layout is calculated dynamically in the `View` function using `msg.Width`
// and `msg.Height`.
//
// - LAYOUT COMPOSITION:
//   Use `lipgloss.JoinVertical(lipgloss.Top, header, viewport, buttonBar, footer)`
//
// - VIEWPORT (Document List):
//   The Viewport height is dynamic: `Height = TermHeight - (Header + Buttons + Footer)`.
//   Content within uses `lipgloss.Style` with borders.
//
// - BUTTON BAR (Responsive):
//   1. Calculate total width of buttons in a single row.
//   2. If `TotalWidth < TermWidth`:
//      Use `lipgloss.JoinHorizontal(lipgloss.Top, buttons...)` with spacing.
//   3. If `TotalWidth > TermWidth`:
//      Switch to vertical stack using `lipgloss.JoinVertical(lipgloss.Left, buttons...)`.
//      Buttons expand to fill width or center align.
//
// - SCROLLING:
//   **MUST use the `osm:bubbles/viewport` module (internal/builtin/bubbles/viewport).**
//   Integrate a `viewport` instance to render the document list (e.g., `vp.setContent(...)` and `vp.view()`),
//   and implement `PgUp`/`PgDn`/`Home`/`End` semantics via the viewport API. Do not use ad-hoc line slicing or custom clipping —
//   the registered `viewport` module is required for correct scrolling semantics and hit-testing.
//   TODO: integrate `viewportLib` into `renderList()`/`renderView()`, and fix viewport height calculation.
//
// 4. BEHAVIORAL RULES & INTERACTIONS
// ----------------------------------
//
// DOCUMENT LIST:
// - CLICK TARGETS:
//   1. [X] Remove Button: `x` range of the text "[X] Remove" + padding.
//      Action: Triggers Delete Confirmation.
//   2. Document Body (Remainder of box):
//      Action: Opens Input View (Edit Mode) with pre-filled data.
// - SELECTION: Keyboard `Up/Down` highlights the box with a **Purple Border**.
//   `Enter` on selected box opens Edit Mode.
//
// INPUT FORM:
// - NAVIGATION: `Tab` cycles: Label -> Content -> Submit -> Cancel -> Label.
// - LABEL: Optional. If empty, the system generates "Document N".
// - CONTENT: True Textarea. `Enter` adds newline. `Ctrl+Enter` submits form.
//
// GLOBAL BUTTONS:
// - [A]dd (a): Clears input buffers, opens Input View.
// - [L]oad (l): Triggers file loading dialog (Context dependant).
// - [E]dit (e): Opens the currently selected document for editing.
// - [V]iew (v): Opens the currently selected document in read-only view / Preview.
// - [D]elete (d): Triggers delete confirmation for selected document.
// - [C]opy (c): **Purple Style**. Copies the prompt content to clipboard.
// - [S]hell (s): **Orange Style**. Leaves the GUI mode and "drops" to a shell-style mode.
// - VISUALS: Buttons are terse text blocks with brackets.
//
// FOOTER:
// - Fixed to bottom. Contains help keys.
// - Has dynamic layout. Demonstrated above.
// - Footer component should be reusable across views.
//
// ============================================================================

const {buildContext, contextManager} = require('osm:ctxutil');
const nextIntegerId = require('osm:nextIntegerId');
const template = require('osm:text/template');
const tea = require('osm:bubbletea');
const lipgloss = require('osm:lipgloss');
const osm = require('osm:os');
const zone = require('osm:bubblezone');
const textareaLib = require('osm:bubbles/textarea');
const viewportLib = require('osm:bubbles/viewport'); // MUST be used: integrate the viewport instance into list rendering (see SCROLLING section below). Do NOT leave this import unused.
const scrollbarLib = require('osm:termui/scrollbar');

// Import shared symbols
const shared = require('osm:sharedStateSymbols');

// config.name is injected by Go as "super-document"
const COMMAND_NAME = config.name;

// Define command-specific symbols
const stateKeys = {
    documents: Symbol("documents"),
    selectedIndex: Symbol("selectedIndex"),
    nextDocId: Symbol("nextDocId"),
    template: Symbol("template"),
};

// ============================================================================
// RUNTIME-ONLY FLAGS (NEVER PERSISTED)
// ============================================================================
// These flags coordinate between the TUI command and the shell loop.
// They are MODULE-LEVEL VARIABLES, not part of the persistent state system.
// When the user presses 'q', we request exit. When they press 's', we drop to shell.
// The shell loop's exit checker reads these to decide whether to exit.

let _userRequestedShell = false;

// Create the single state accessor (ONLY for persistent data)
const state = tui.createState(COMMAND_NAME, {
    // Shared keys
    [shared.contextItems]: {defaultValue: []},

    // Command-specific keys (all persistent)
    [stateKeys.documents]: {defaultValue: []},
    [stateKeys.selectedIndex]: {defaultValue: 0},
    [stateKeys.nextDocId]: {defaultValue: 1},
    [stateKeys.template]: {defaultValue: null},
});

// ============================================================================
// CORE LOGIC & HELPERS
// ============================================================================

function defaultTemplate() {
    // Use the injected template or a fallback
    return superDocumentTemplate || `Implement a super-document that is INTERNALLY CONSISTENT based on a quorum or carefully-weighed analysis of the attached documents.

Your goal is to coalesce AS MUCH INFORMATION AS POSSIBLE, in as raw form as possible, while **preserving internal consistency**.

{{if .contextTxtar}}
---
## CONTEXT
---

{{.contextTxtar}}
{{end}}

---
## DOCUMENTS
---

{{range $idx, $doc := .documents}}
Document {{$doc.id}}:
\`\`\`\`\`
{{$doc.content}}
\`\`\`\`\`

{{end}}`;
}

function getTemplate() {
    const tmpl = state.get(stateKeys.template);
    return (tmpl !== null && tmpl !== undefined && tmpl !== "") ? tmpl : defaultTemplate();
}

function getDocuments() {
    return state.get(stateKeys.documents) || [];
}

function setDocuments(docs) {
    state.set(stateKeys.documents, docs);
}

function getNextDocId() {
    return state.get(stateKeys.nextDocId) || 1;
}

function incrementNextDocId() {
    state.set(stateKeys.nextDocId, getNextDocId() + 1);
}

function addDocument(label, content) {
    const docs = getDocuments();
    const id = getNextDocId();
    incrementNextDocId();
    const doc = {
        id: id,
        label: label || ('Document ' + (docs.length + 1)),
        content: content || ''
    };
    docs.push(doc);
    setDocuments(docs);
    return doc;
}

function removeDocumentById(id) {
    const docs = getDocuments();
    const idx = docs.findIndex(d => d.id === id);
    if (idx >= 0) {
        const doc = docs[idx];
        docs.splice(idx, 1);
        setDocuments(docs);

        // Adjust selected index if needed
        let selectedIdx = state.get(stateKeys.selectedIndex);
        if (selectedIdx >= docs.length && selectedIdx > 0) {
            state.set(stateKeys.selectedIndex, selectedIdx - 1);
        }
        return doc;
    }
    return null;
}

function getDocumentById(id) {
    return getDocuments().find(d => d.id === id);
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

function buildContextTxtar() {
    return buildContext(state.get(shared.contextItems), {toTxtar: () => context.toTxtar()});
}

function buildFinalPrompt() {
    const docs = getDocuments();
    const contextTxtar = buildContextTxtar();

    // Execute template
    return template.execute(getTemplate(), {
        documents: docs,
        contextTxtar: contextTxtar
    });
}


// ============================================================================
// VIEWPORT & LAYOUT HELPERS
// ============================================================================

// Calculate viewport height based on terminal size and fixed UI elements
// NOTE: This is now largely handled dynamically inside renderList to account
// for responsive button heights, but kept as a fallback utility.
function calculateViewportHeight(s) {
    const termHeight = s.height || 24;
    const fixedOverhead = 12; // Adjusted estimate
    return Math.max(5, termHeight - fixedOverhead);
}

// Lightweight preview helper: truncate first, then strip newlines to avoid
// allocating large temporary strings on each render for large documents.
function previewOf(content, maxLen) {
    const raw = content || '';
    let p = raw.substring(0, maxLen).replace(/\n/g, ' ');
    if (raw.length > maxLen) p += '...';
    return p;
}

// Generate a hash of document IDs to detect changes
function getDocsHash(docs) {
    if (!docs || docs.length === 0) return '';
    // Include length to catch wrapping changes.
    return docs.map(d => d.id + ':' + (d.content || '').length).join(',');
}

// Build the LayoutMap: maps document index to {top: pixelOffset, height: lineCount}
// This is used for:
// 1. Calculating scroll offset when selection changes
// 2. Hit-testing mouse clicks inside the viewport
function buildLayoutMap(docs, docContentWidth) {
    const layoutMap = [];
    let currentTop = 0;

    docs.forEach((doc, i) => {
        // Calculate the rendered height of this document box
        // Document box structure inside the border:
        // - Header line: #id [label] (1 line)
        // - Preview line (1 line)
        // - Remove button line (1 line)
        // Borders add +2 height (top/bottom)

        let prev = previewOf(doc.content, DESIGN.previewMaxLen);

        const docHeader = `#${doc.id} [${doc.label}]`;
        const docPreview = styles.preview().render(prev);
        const removeBtn = '[X] Remove';
        const docContent = lipgloss.joinVertical(
            lipgloss.Left,
            docHeader,
            docPreview,
            removeBtn
        );

        const style = styles.document(); // Use base style for height calculation
        // CRITICAL: Use consistent width passed from renderList
        const renderedDoc = style.width(docContentWidth).render(docContent);
        const docHeight = lipgloss.height(renderedDoc);

        layoutMap.push({
            top: currentTop,
            height: docHeight,
            docId: doc.id
        });

        currentTop += docHeight;
    });

    return layoutMap;
}

// Ensure the selected document is visible in the viewport
function ensureSelectionVisible(s) {
    if (!s.vp || !s.layoutMap || s.layoutMap.length === 0) return;
    if (s.selectedIdx < 0 || s.selectedIdx >= s.layoutMap.length) return;

    const entry = s.layoutMap[s.selectedIdx];
    const yOffset = s.vp.yOffset();
    const vpHeight = s.vp.height();

    // Check if selection is above the viewport
    if (entry.top < yOffset) {
        s.vp.setYOffset(entry.top);
        return;
    }

    // Check if selection is below the viewport
    const selectionBottom = entry.top + entry.height;
    if (selectionBottom > yOffset + vpHeight) {
        s.vp.setYOffset(selectionBottom - vpHeight);
    }
}

// Find which document was clicked based on viewport-relative coordinates
// Returns {index: docIndex, relativeLineInDoc: lineOffset} or null if no hit
function findClickedDocument(s, relativeY) {
    if (!s.layoutMap || s.layoutMap.length === 0) return null;

    // relativeY is already adjusted for viewport offset
    for (let i = 0; i < s.layoutMap.length; i++) {
        const entry = s.layoutMap[i];
        if (relativeY >= entry.top && relativeY < entry.top + entry.height) {
            return {
                index: i,
                relativeLineInDoc: relativeY - entry.top
            };
        }
    }
    return null;
}


// ============================================================================
// TUI IMPLEMENTATION (Bubble Tea)
// ============================================================================

// TUI Constants
const MODE_LIST = 'list';
const MODE_INPUT = 'input';
const MODE_VIEW = 'view';
const MODE_CONFIRM = 'confirm';

const FOCUS_LABEL = 0;
const FOCUS_CONTENT = 1;
const FOCUS_SUBMIT = 2;
const FOCUS_CANCEL = 3;
const FOCUS_MAX = 4;

const INPUT_ADD = 'add';
const INPUT_EDIT = 'edit';
const INPUT_LOAD = 'load';
const INPUT_RENAME = 'rename';

// TUI Design
const DESIGN = {
    buttonPaddingH: 2, buttonPaddingV: 0, buttonMarginR: 1,
    docPaddingH: 1, docPaddingV: 0, docMarginB: 1,
    previewMaxLen: 50, textareaHeight: 6
};

// TUI Styles
const COLORS = {
    primary: '#7C3AED', secondary: '#10B981', danger: '#EF4444',
    muted: '#6B7280', bg: '#1F2937', fg: '#F9FAFB',
    warning: '#F59E0B', focus: '#3B82F6'
};

const styles = {
    title: () => lipgloss.newStyle().bold(true).foreground(COLORS.primary).padding(0, 1),
    normal: () => lipgloss.newStyle().foreground(COLORS.fg).padding(0, 1),
    button: () => lipgloss.newStyle().foreground(COLORS.fg).background(COLORS.secondary).padding(DESIGN.buttonPaddingV, DESIGN.buttonPaddingH).marginRight(DESIGN.buttonMarginR),
    buttonFocused: () => lipgloss.newStyle().foreground(COLORS.fg).background(COLORS.primary).bold(true).padding(DESIGN.buttonPaddingV, DESIGN.buttonPaddingH).marginRight(DESIGN.buttonMarginR),
    buttonDanger: () => lipgloss.newStyle().foreground(COLORS.fg).background(COLORS.danger).padding(DESIGN.buttonPaddingV, DESIGN.buttonPaddingH).marginRight(DESIGN.buttonMarginR),
    buttonShell: () => lipgloss.newStyle().foreground(COLORS.fg).background(COLORS.warning).bold(true).padding(DESIGN.buttonPaddingV, DESIGN.buttonPaddingH).marginRight(DESIGN.buttonMarginR),
    status: () => lipgloss.newStyle().foreground(COLORS.secondary).italic(true),
    error: () => lipgloss.newStyle().foreground(COLORS.danger).bold(true),
    help: () => lipgloss.newStyle().foreground(COLORS.muted),
    separator: () => lipgloss.newStyle().foreground(COLORS.muted),
    box: () => lipgloss.newStyle().border(lipgloss.roundedBorder()).borderForeground(COLORS.primary).padding(1, 2),
    inputNormal: () => lipgloss.newStyle().border(lipgloss.normalBorder()).borderForeground(COLORS.muted).padding(0, 1),
    inputFocused: () => lipgloss.newStyle().border(lipgloss.normalBorder()).borderForeground(COLORS.focus).bold(true).padding(0, 1),
    document: () => lipgloss.newStyle().border(lipgloss.normalBorder()).borderForeground(COLORS.muted).padding(DESIGN.docPaddingV, DESIGN.docPaddingH).marginBottom(DESIGN.docMarginB),
    documentSelected: () => lipgloss.newStyle().border(lipgloss.doubleBorder()).borderForeground(COLORS.primary).padding(DESIGN.docPaddingV, DESIGN.docPaddingH).marginBottom(DESIGN.docMarginB),
    preview: () => lipgloss.newStyle().foreground(COLORS.muted),
    label: () => lipgloss.newStyle().foreground(COLORS.fg).bold(true),
};

// TUI Logic
function runVisualTui() {
    // Create viewport instance ONCE at init time (not on each render)
    const vp = viewportLib.new(80, 24);
    vp.mouseWheelEnabled(false); // We handle mouse wheel manually

    // Create input viewport instance ONCE at init time (not on each render)
    // This fixes the scroll reset bug where viewport was recreated in renderInput
    const inputVp = viewportLib.new(80, 24);
    inputVp.mouseWheelEnabled(false); // We handle mouse wheel manually

    const initialState = {
        // Core state linked to global state
        documents: getDocuments(),
        selectedIdx: state.get(stateKeys.selectedIndex),

        // Viewport instances (created once, reused)
        vp: vp,
        inputVp: inputVp,  // Persistent input viewport for MODE_INPUT scrolling
        listScrollbar: scrollbarLib.new(),
        inputScrollbar: scrollbarLib.new(),
        vpContentWidth: 0,

        // LayoutMap cache: maps document index -> {top: y, height: h}
        // Rebuilt only when documents array changes
        layoutMap: null,
        layoutMapDocsHash: '', // Hash to detect document changes

        // UI State
        mode: MODE_LIST,
        inputOperation: null,
        inputFocus: FOCUS_LABEL,
        labelBuffer: '',
        contentTextarea: null, // Native bubbles/textarea component
        editingDocId: null,
        viewContent: '',
        viewTitle: '',
        confirmPrompt: '',
        confirmDocId: null,
        statusMsg: '',
        hasError: false,
        clipboard: '',
        width: 80, height: 24,
        layout: {buttons: [], docBoxes: []},

        // Flag to force a full screen clear on first render
        // This ensures the title line is rendered correctly when re-entering TUI from shell
        needsInitClear: true
    };

    const model = tea.newModel({
        init: function () {
            return initialState;
        },
        update: function (msg, tuiState) {
            // DEBUG: Log all messages to understand what's being received
            if (typeof console !== 'undefined' && msg.type === 'mouse') {
                console.log('UPDATE DEBUG: msg.type=' + msg.type + ', msg.action=' + msg.action + ', msg.button=' + msg.button + ', msg.x=' + msg.x + ', msg.y=' + msg.y);
            }
            // Sync documents from global state on every update to ensure freshness
            tuiState.documents = getDocuments();

            if (msg.type === 'windowSize') {
                tuiState.width = msg.width;
                tuiState.height = msg.height;
                // Update viewport dimensions (will clamp yOffset automatically in Go)
                if (tuiState.vp) {
                    // NOTE: Viewport sizing is handled in `renderList` to account for
                    // dynamic button bar/footer heights. We intentionally do NOT set
                    // explicit width/height here to avoid a split source of truth.
                    // Keep the viewport instance intact so `renderList` can configure it.
                }
                // Invalidate layoutMap when width changes (affects text wrapping/heights)
                tuiState.layoutMap = null;

                // Force full screen clear on first windowSize message after init
                // This ensures all content (including title) is rendered correctly
                // when re-entering TUI from shell mode
                if (tuiState.needsInitClear) {
                    tuiState.needsInitClear = false;
                    return [tuiState, tea.clearScreen()];
                }
            } else if (msg.type === 'keyPress') {
                return handleKeys(msg, tuiState);
            } else if (msg.type === 'mouse' && msg.action === 'press') {
                return handleMouse(msg, tuiState);
            }
            return [tuiState, null];
        },
        view: function (tuiState) {
            return renderView(tuiState);
        }
    });

    return tea.run(model, {altScreen: true, mouse: true});
}

function handleKeys(msg, s) {
    const k = msg.key;
    const prevMode = s.mode; // Track mode before processing
    // Global quit from any mode if ctrl+c
    if (k === 'ctrl+c') return [s, tea.quit()];

    if (s.mode === MODE_LIST) {
        if (k === 'q') {
            // User wants to exit completely - use module-level flag (NOT persistent state)
            _userRequestedShell = false;
            return [s, tea.quit()];
        }
        if (k === 's') {
            // User explicitly requested to drop into shell - use module-level flag
            _userRequestedShell = true;
            return [s, tea.quit()];
        }

        // Arrow key navigation with auto-scroll
        if (k === 'up' || k === 'k') {
            s.selectedIdx = Math.max(0, s.selectedIdx - 1);
            ensureSelectionVisible(s);
        }
        if (k === 'down' || k === 'j') {
            s.selectedIdx = Math.min(s.documents.length - 1, s.selectedIdx + 1);
            ensureSelectionVisible(s);
        }

        // MANUAL PAGE KEY HANDLING - DO NOT forward to vp.update()!
        // PgDown: Move selection down by approximately a page worth of documents
        if (k === 'pgdown') {
            if (s.documents.length > 0 && s.vp) {
                const vpHeight = s.vp.height();
                // Estimate ~5 lines per document box
                const pageSize = Math.max(1, Math.floor(vpHeight / 5));
                s.selectedIdx = Math.min(s.documents.length - 1, s.selectedIdx + pageSize);
                ensureSelectionVisible(s);
            }
        }

        // PgUp: Move selection up by approximately a page worth of documents
        if (k === 'pgup') {
            if (s.documents.length > 0 && s.vp) {
                const vpHeight = s.vp.height();
                const pageSize = Math.max(1, Math.floor(vpHeight / 5));
                s.selectedIdx = Math.max(0, s.selectedIdx - pageSize);
                ensureSelectionVisible(s);
            }
        }

        // Home: Go to first document
        if (k === 'home') {
            if (s.documents.length > 0) {
                s.selectedIdx = 0;
                if (s.vp) s.vp.setYOffset(0);
            }
        }

        // End: Go to last document
        if (k === 'end') {
            if (s.documents.length > 0) {
                s.selectedIdx = s.documents.length - 1;
                ensureSelectionVisible(s);
            }
        }

        // Persist selection after any navigation
        state.set(stateKeys.selectedIndex, s.selectedIdx);

        if (k === 'a') {
            s.mode = MODE_INPUT;
            s.inputOperation = INPUT_ADD;
            s.inputFocus = FOCUS_LABEL;
            s.labelBuffer = '';
            // Create native textarea for content
            s.contentTextarea = textareaLib.new();
            s.contentTextarea.setPlaceholder("Enter document content...");
            s.contentTextarea.setWidth(Math.max(40, s.width - 10));
            s.contentTextarea.setHeight(DESIGN.textareaHeight);
        }
        if (k === 'l') {
            s.mode = MODE_INPUT;
            s.inputOperation = INPUT_LOAD;
            s.inputFocus = FOCUS_LABEL;
            s.labelBuffer = '';
            s.contentTextarea = null; // No textarea for load mode
        }
        if (k === 'e' || k === 'enter') {
            const doc = s.documents[s.selectedIdx];
            if (doc) {
                s.mode = MODE_INPUT;
                s.inputOperation = INPUT_EDIT;
                s.inputFocus = FOCUS_CONTENT;
                s.labelBuffer = doc.label;
                // Create native textarea with existing content
                s.contentTextarea = textareaLib.new();
                s.contentTextarea.setWidth(Math.max(40, s.width - 10));
                s.contentTextarea.setHeight(DESIGN.textareaHeight);
                s.contentTextarea.setValue(doc.content);
                s.contentTextarea.focus();
                s.editingDocId = doc.id;
            }
        }
        if (k === 'r') {
            const doc = s.documents[s.selectedIdx];
            if (doc) {
                s.mode = MODE_INPUT;
                s.inputOperation = INPUT_RENAME;
                s.inputFocus = FOCUS_LABEL;
                s.labelBuffer = doc.label;
                s.contentTextarea = null; // No textarea for rename mode
                s.editingDocId = doc.id;
            }
        }
        if (k === 'd' || k === 'backspace') {
            const doc = s.documents[s.selectedIdx];
            if (doc) {
                s.mode = MODE_CONFIRM;
                s.confirmPrompt = `Delete document #${doc.id} "${doc.label}"? (y/n)`;
                s.confirmDocId = doc.id;
            }
        }
        // 'v' key removed - view mode is redundant per AGENTS.md
        // 'g' key removed - generate button does nothing per AGENTS.md
        if (k === 'c') {
            if (s.documents.length === 0) {
                s.statusMsg = 'No documents!';
                s.hasError = true;
            } else {
                const prompt = buildFinalPrompt();
                s.clipboard = prompt;
                try {
                    // Call the system clipboard via osm:os module
                    osm.clipboardCopy(prompt);
                    s.statusMsg = `Copied prompt (${prompt.length} chars)`;
                    s.hasError = false;
                } catch (e) {
                    s.statusMsg = 'Clipboard error: ' + e;
                    s.hasError = true;
                }
            }
        }
        // 'p' preview removed - view mode is redundant per AGENTS.md
        if (k === '?') s.statusMsg = 'a:add l:load e:edit r:rename d:del c:copy s:shell q:quit';
    } else if (s.mode === MODE_INPUT) {
        // Keyboard scroll for input viewport (pgup/pgdown/home/end)
        if (k === 'pgdown' && s.inputVp) {
            s.inputVp.scrollDown(s.inputVp.height());
            return [s, null];
        }
        if (k === 'pgup' && s.inputVp) {
            s.inputVp.scrollUp(s.inputVp.height());
            return [s, null];
        }
        if (k === 'home' && s.inputVp) {
            s.inputVp.setYOffset(0);
            return [s, null];
        }
        if (k === 'end' && s.inputVp) {
            // Scroll to bottom - set yOffset to max
            const maxOffset = Math.max(0, s.inputVp.totalLineCount() - s.inputVp.height());
            s.inputVp.setYOffset(maxOffset);
            return [s, null];
        }
        if (k === 'esc') {
            s.mode = MODE_LIST;
            s.statusMsg = 'Cancelled';
        } else if (k === 'tab' || k === 'shift+tab') {
            // Handle focus transitions and blur/focus the textarea
            const oldFocus = s.inputFocus;
            if (k === 'tab') {
                s.inputFocus = (s.inputFocus + 1) % FOCUS_MAX;
            } else {
                // shift+tab: cycle backward
                s.inputFocus = (s.inputFocus - 1 + FOCUS_MAX) % FOCUS_MAX;
            }
            // Skip content focus for specific ops (forward)
            if (k === 'tab') {
                if (s.inputOperation === INPUT_RENAME && s.inputFocus === FOCUS_CONTENT) s.inputFocus = FOCUS_SUBMIT;
                if (s.inputOperation === INPUT_LOAD && s.inputFocus === FOCUS_CONTENT) s.inputFocus = FOCUS_SUBMIT;
            } else {
                // Skip content focus for specific ops (backward)
                if (s.inputOperation === INPUT_RENAME && s.inputFocus === FOCUS_CONTENT) s.inputFocus = FOCUS_LABEL;
                if (s.inputOperation === INPUT_LOAD && s.inputFocus === FOCUS_CONTENT) s.inputFocus = FOCUS_LABEL;
            }

            // Manage textarea focus
            if (s.contentTextarea) {
                if (s.inputFocus === FOCUS_CONTENT) {
                    s.contentTextarea.focus();
                } else if (oldFocus === FOCUS_CONTENT) {
                    s.contentTextarea.blur();
                }
            }
        } else if (k === 'ctrl+enter' || (k === 'enter' && s.inputFocus !== FOCUS_CONTENT)) {
            // Submit
            if (s.inputFocus === FOCUS_CANCEL) {
                s.mode = MODE_LIST;
                // Force full screen repaint when exiting form mode
                return [s, tea.clearScreen()];
            }

            // Get content from textarea if present
            const contentValue = s.contentTextarea ? s.contentTextarea.value() : '';

            // Process Submit
            if (s.inputOperation === INPUT_ADD) {
                const doc = addDocument(s.labelBuffer.trim(), contentValue);
                s.statusMsg = 'Added document #' + doc.id;
            } else if (s.inputOperation === INPUT_EDIT) {
                updateDocument(s.editingDocId, contentValue, s.labelBuffer.trim());
                s.statusMsg = 'Updated document';
            } else if (s.inputOperation === INPUT_RENAME) {
                updateDocument(s.editingDocId, undefined, s.labelBuffer.trim());
                s.statusMsg = 'Renamed document';
            } else if (s.inputOperation === INPUT_LOAD) {
                const res = osm.readFile(s.labelBuffer.trim());
                if (res.error) {
                    s.statusMsg = 'Error: ' + res.error;
                    s.hasError = true;
                    return [s, null];
                }
                const doc = addDocument(s.labelBuffer.trim(), res.content);
                s.statusMsg = 'Loaded document #' + doc.id;
            }
            // Refresh local state from global after mutation
            s.documents = getDocuments();
            s.mode = MODE_LIST;
            s.hasError = false;
        } else {
            // Field input handling
            if (s.inputFocus === FOCUS_LABEL) {
                if (k === 'backspace') s.labelBuffer = s.labelBuffer.slice(0, -1);
                else if (k.length === 1) s.labelBuffer += k;
            } else if (s.inputFocus === FOCUS_CONTENT && s.contentTextarea) {
                // Delegate to native textarea component
                s.contentTextarea.update(msg);
            }
        }
    } else if (s.mode === MODE_CONFIRM) {
        if (k === 'y' || k === 'Y') {
            const deletedId = s.confirmDocId;
            removeDocumentById(s.confirmDocId);
            s.documents = getDocuments();  // Refresh local state from global after mutation
            s.statusMsg = 'Deleted document #' + deletedId;
            s.mode = MODE_LIST;
        } else if (k === 'n' || k === 'N' || k === 'esc') {
            s.statusMsg = 'Cancelled';
            s.mode = MODE_LIST;
        }
    }
    // MODE_VIEW removed - view mode is redundant per AGENTS.md

    // Force full screen repaint when transitioning FROM a modal mode TO MODE_LIST
    // This ensures BubbleTea re-renders the entire screen including the title
    if (s.mode === MODE_LIST && prevMode !== MODE_LIST) {
        return [s, tea.clearScreen()];
    }
    return [s, null];
}

function handleMouse(msg, s) {
    // Guard: Only process left-button clicks for button/document activation
    // Wheel events should not trigger actions (they'll be handled as scroll if needed)
    const isLeftClick = msg.button === 'left';
    const isWheelEvent = msg.button === 'wheelUp' || msg.button === 'wheelDown' ||
        msg.button === 'wheelLeft' || msg.button === 'wheelRight';

    // Handle wheel events for scrolling in list mode via viewport
    if (isWheelEvent && s.mode === MODE_LIST && s.documents.length > 0 && s.vp) {
        if (msg.button === 'wheelUp') {
            s.vp.scrollUp(3); // Scroll viewport up by 3 lines
        } else if (msg.button === 'wheelDown') {
            s.vp.scrollDown(3); // Scroll viewport down by 3 lines
        }
        // Don't change selection on wheel - just scroll the viewport
        return [s, null];
    }

    // Handle wheel events for scrolling in input mode via inputVp
    if (isWheelEvent && s.mode === MODE_INPUT && s.inputVp) {
        if (msg.button === 'wheelUp') {
            s.inputVp.scrollUp(3); // Scroll input viewport up by 3 lines
        } else if (msg.button === 'wheelDown') {
            s.inputVp.scrollDown(3); // Scroll input viewport down by 3 lines
        }
        return [s, null];
    }

    // Only left-button presses trigger button/document activation
    if (!isLeftClick) {
        return [s, null];
    }

    // Use bubblezone for proper hit-testing - no hardcoded coordinates
    // Buttons are marked with zone IDs like "btn-add", "btn-load", etc.
    // Note: btn-edit, btn-view, btn-delete, btn-generate removed per AGENTS.md
    const buttonActions = [
        {id: 'btn-add', action: 'a'},
        {id: 'btn-load', action: 'l'},
        {id: 'btn-copy', action: 'c'},
        {id: 'btn-shell', action: 'shell'},
        {id: 'btn-submit', action: 'submit'},
        {id: 'btn-cancel', action: 'esc'},
        {id: 'btn-yes', action: 'y'},
        {id: 'btn-no', action: 'n'},
    ];

    // Check button zones (debug removed)
    for (const btn of buttonActions) {
        if (zone.inBounds(btn.id, msg)) {
            if (btn.action === 'shell') {
                _userRequestedShell = true;
                return [s, tea.quit()];
            }
            if (btn.action === 'submit') {
                // Handle submit button click - this is the fix for the nil pointer crash
                // We need to properly set focus and handle submission
                if (s.mode === MODE_INPUT && s.inputFocus !== FOCUS_SUBMIT) {
                    // First focus the submit button
                    if (s.inputFocus === FOCUS_CONTENT && s.contentTextarea) {
                        s.contentTextarea.blur();
                    }
                    s.inputFocus = FOCUS_SUBMIT;
                }
                // Now handle the enter key to submit
                return handleKeys({key: 'ctrl+enter'}, s);
            }
            return handleKeys({key: btn.action}, s);
        }
    }

    // Handle input field zones for mouse focus (MODE_INPUT only)
    if (s.mode === MODE_INPUT) {
        if (zone.inBounds('input-label', msg)) {
            // Click on label field - focus it
            if (s.inputFocus !== FOCUS_LABEL) {
                // Blur textarea if leaving content
                if (s.inputFocus === FOCUS_CONTENT && s.contentTextarea) {
                    s.contentTextarea.blur();
                }
                s.inputFocus = FOCUS_LABEL;
            }
            return [s, null];
        }
        if (zone.inBounds('input-content', msg)) {
            // Click on content textarea - focus it
            if (s.inputFocus !== FOCUS_CONTENT) {
                s.inputFocus = FOCUS_CONTENT;
                if (s.contentTextarea) {
                    s.contentTextarea.focus();
                }
            }
            return [s, null];
        }
    }

    // Document click handling using LayoutMap + coordinate math
    // zone.mark is NOT used for documents inside viewport (clipping destroys markers)
    if (s.mode === MODE_LIST && s.documents.length > 0 && s.layoutMap && s.vp) {
        // Calculate document-relative Y coordinate
        // msg.y is terminal-relative, we need to adjust for:
        // 1. Header height (title + blank + docs count + blank before viewport = 4 lines)
        // 2. Viewport scroll offset
        const headerHeight = 4;
        const clickY = msg.y;

        // Check if click is in the viewport area
        const vpTop = headerHeight;
        const vpHeight = s.vp.height();
        const vpBottom = vpTop + vpHeight;

        if (clickY >= vpTop && clickY < vpBottom) {
            // Ignore clicks on the right-side scrollbar column.
            if (s.vpContentWidth > 0 && msg.x >= s.vpContentWidth) {
                return [s, null];
            }
            // Convert to content-space coordinates
            const viewportRelativeY = clickY - vpTop;
            const contentY = viewportRelativeY + s.vp.yOffset();

            // Find which document was clicked and where within it
            const clickResult = findClickedDocument(s, contentY);
            if (clickResult !== null) {
                s.selectedIdx = clickResult.index;
                state.set(stateKeys.selectedIndex, clickResult.index);

                // Document box structure relative to LayoutMap:
                // Line 0: Top Border
                // Line 1: Header (#id [label])
                // Line 2: Preview
                // Line 3: [X] Remove button
                // Line 4: Bottom Border

                // Mapped Action Targets:
                if (clickResult.relativeLineInDoc === 1) {
                    // Line 1: Header -> Rename
                    return handleKeys({key: 'r'}, s);
                } else if (clickResult.relativeLineInDoc === 3) {
                    // Line 3: Remove -> Delete
                    return handleKeys({key: 'd'}, s);
                } else {
                    // All other lines -> Edit Content
                    return handleKeys({key: 'e'}, s);
                }
            }
        }
    }

    return [s, null];
}

function renderView(s) {
    s.layout = {buttons: [], docBoxes: []}; // Reset layout (kept for compatibility)
    let content;
    if (s.mode === MODE_INPUT) content = renderInput(s);
    // MODE_VIEW removed - view mode is redundant per AGENTS.md
    else if (s.mode === MODE_CONFIRM) content = renderConfirm(s);
    else content = renderList(s);

    // Wrap with zone.scan() to register zones and strip markers
    return zone.scan(content);
}

// Minimal Render Helpers (inlining logic for single-file)
function renderList(s) {
    const termWidth = s.width || 80;
    const termHeight = s.height || 24;
    const scrollbarWidth = 1;

    // Header section - ALWAYS use fresh style creation to avoid caching issues
    const titleStyle = lipgloss.newStyle().bold(true).foreground(COLORS.primary).padding(0, 1);
    const normalStyle = lipgloss.newStyle().foreground(COLORS.fg).padding(0, 1);

    const titleLine = titleStyle.render('📄 Super-Document Builder');
    const docsLine = normalStyle.render(`Documents: ${s.documents.length}`);

    const header = lipgloss.joinVertical(
        lipgloss.Left,
        titleLine,
        '',
        docsLine
    );

    // FIX: Explicitly calculate widths to avoid clipping.
    // 1. Viewport width: Available width aligned to right edge (termWidth - scrollbar)
    // 2. Document content width: Viewport width minus style overhead (Border + Padding)
    // Overhead = Border(2) + Padding(2) = 4
    const docStyleOverhead = 4;
    const viewportWidth = termWidth - scrollbarWidth;
    const docContentWidth = Math.max(40, viewportWidth - docStyleOverhead);

    // Update/rebuild LayoutMap if documents changed
    const docsHash = getDocsHash(s.documents);
    if (s.layoutMap === null || s.layoutMapDocsHash !== docsHash) {
        // Layout widths must match the viewport width we use below.
        s.layoutMap = buildLayoutMap(s.documents, docContentWidth);
        s.layoutMapDocsHash = docsHash;
    }

    // Document list section
    let docSection = '';
    if (s.documents.length === 0) {
        docSection = styles.help().render("No documents. Press 'a' to add, 'l' to load, 's' for shell.");
    } else {
        const docItems = [];
        s.documents.forEach((doc, i) => {
            const isSel = i === s.selectedIdx;
            let prev = previewOf(doc.content, DESIGN.previewMaxLen);

            const removeBtn = '[X] Remove';

            // Build document content line by line
            const docHeader = `#${doc.id} [${doc.label}]`;
            const docPreview = styles.preview().render(prev);
            const docContent = lipgloss.joinVertical(
                lipgloss.Left,
                docHeader,
                docPreview,
                removeBtn
            );

            // Apply selection style (double border for selected)
            const style = isSel ? styles.documentSelected() : styles.document();
            // Use explicitly calculated content width to prevent box clipping
            const renderedDoc = style.width(docContentWidth).render(docContent);

            docItems.push(renderedDoc);
        });
        docSection = lipgloss.joinVertical(lipgloss.Left, ...docItems);
    }

    // Button bar section with responsive layout
    // Per AGENTS.md design: [A]dd  [L]oad  [C]opy  [S]hell
    // Delete (d) is keyboard-only, Edit/View/Generate buttons removed
    const buttonList = [
        {id: 'btn-add', text: '[A]dd', style: styles.button()},
        {id: 'btn-load', text: '[L]oad', style: styles.button()},
        {id: 'btn-copy', text: '[C]opy', style: styles.buttonFocused()},
        {id: 'btn-shell', text: '[S]hell', style: styles.buttonShell()},
    ];

    // Render buttons with zone markers
    const renderedButtons = buttonList.map(b => zone.mark(b.id, b.style.render(b.text)));

    // Calculate total button width
    const totalButtonWidth = renderedButtons.reduce((sum, btn) => sum + lipgloss.width(btn), 0);

    // Responsive layout: stack vertically if buttons exceed terminal width
    let buttonSection;
    const availWidth = termWidth - 4; // Leave some margin

    if (totalButtonWidth > availWidth) {
        // Narrow: stack ALL buttons vertically
        buttonSection = lipgloss.joinVertical(lipgloss.Left, ...renderedButtons);
    } else {
        // Wide: horizontal layout
        buttonSection = lipgloss.joinHorizontal(lipgloss.Top, ...renderedButtons);
    }
    const buttonSectionHeight = lipgloss.height(buttonSection);

    // Status section
    let statusSection = '';
    if (s.statusMsg) {
        const statusStyle = s.hasError ? styles.error() : styles.status();
        const statusIcon = s.hasError ? '✗ ' : '✓ ';
        statusSection = statusStyle.render(statusIcon + s.statusMsg);
    }

    // Footer section
    const separatorWidth = Math.min(72, termWidth - 2);
    const separator = styles.separator().render('─'.repeat(separatorWidth));
    const helpText = styles.help().render('a:add l:load e:edit d:del c:copy s:shell q:quit ↑↓:nav pgup/pgdn:page');
    const footer = lipgloss.joinVertical(lipgloss.Left, separator, helpText);

    // ------------------------------------------------------------------------
    // DYNAMIC VIEWPORT HEIGHT CALCULATION
    // ------------------------------------------------------------------------
    // Per AGENTS.md: buttons should be INSIDE the scrollable area since they're
    // only useful for mouse users who have easy scrolling access.
    // Only the footer (hints) and header remain fixed.
    const headerHeight = lipgloss.height(header);
    const footerHeight = lipgloss.height(footer);
    const statusHeight = lipgloss.height(statusSection);
    const spacerHeight = 3; // Empty lines between sections

    // Calculate viewport height - buttons are now INSIDE the viewport
    const fixedHeight = headerHeight + footerHeight + statusHeight + spacerHeight;
    const vpHeight = Math.max(0, termHeight - fixedHeight);

    // Build scrollable content: documents + buttons
    // This allows buttons to scroll with the document list on small terminals
    let scrollableContent;
    if (s.documents.length === 0) {
        // No documents: show help message + buttons
        scrollableContent = lipgloss.joinVertical(
            lipgloss.Left,
            docSection,
            '',
            buttonSection
        );
    } else {
        // Has documents: documents + gap + buttons
        scrollableContent = lipgloss.joinVertical(
            lipgloss.Left,
            docSection,
            '',
            buttonSection
        );
    }

    // Integrate viewport + thin vertical scrollbar
    let visibleSection = scrollableContent;
    if (s.vp) {
        const vpWidth = viewportWidth; // Aligned to right edge (termWidth - scrollbar)
        s.vpContentWidth = vpWidth;
        s.vp.setWidth(vpWidth);
        s.vp.setHeight(vpHeight);
        s.vp.setContent(scrollableContent);
        const vpView = s.vp.view();

        if (s.listScrollbar) {
            s.listScrollbar.setViewportHeight(vpHeight);
            s.listScrollbar.setContentHeight(s.vp.totalLineCount());
            s.listScrollbar.setYOffset(s.vp.yOffset());
            s.listScrollbar.setChars("█", "░");
            // Use setThumbForeground for opaque █ character (full block draws foreground, not background)
            s.listScrollbar.setThumbForeground(COLORS.primary);
            s.listScrollbar.setTrackForeground(COLORS.muted);
        }

        const sbView = s.listScrollbar ? s.listScrollbar.view() : "";
        visibleSection = lipgloss.joinHorizontal(lipgloss.Top, vpView, sbView);
    }

    // Compose final view - buttons are now inside visibleSection (scrollable)
    return lipgloss.joinVertical(
        lipgloss.Left,
        header,
        '',
        visibleSection,
        '',
        statusSection,
        footer
    );
}

function renderInput(s) {
    const termWidth = s.width || 80;
    const termHeight = s.height || 24;
    const scrollbarWidth = 1;

    // Title based on operation
    let titleText;
    if (s.inputOperation === INPUT_ADD) titleText = '📝 Add Document';
    else if (s.inputOperation === INPUT_EDIT) titleText = '📝 Edit Document';
    else if (s.inputOperation === INPUT_RENAME) titleText = '📝 Rename Document';
    else titleText = '📂 Load File';

    const title = styles.title().render(titleText);

    // Label/Path field - wrap in zone for mouse focus
    const lblLabel = s.inputOperation === INPUT_LOAD ? 'File path:' : 'Label (optional):';
    const lblStyle = s.inputFocus === FOCUS_LABEL ? styles.inputFocused() : styles.inputNormal();
    const lblContent = s.labelBuffer + (s.inputFocus === FOCUS_LABEL ? '▌' : '');
    const lblFieldRendered = lipgloss.joinVertical(
        lipgloss.Left,
        styles.label().render(lblLabel),
        lblStyle.width(Math.max(40, termWidth - 10)).render(lblContent)
    );
    const lblField = zone.mark('input-label', lblFieldRendered);

    // Content field using native bubbles/textarea (not shown for LOAD or RENAME)
    let contentField = '';
    if (s.inputOperation !== INPUT_LOAD && s.inputOperation !== INPUT_RENAME && s.contentTextarea) {
        // Use native textarea view() for proper cursor handling
        const cntStyle = s.inputFocus === FOCUS_CONTENT ? styles.inputFocused() : styles.inputNormal();

        // Update textarea dimensions based on current terminal size.
        // Reserve a thin scrollbar column on the right.
        const fieldWidth = Math.max(40, termWidth - 10);
        const innerWidth = Math.max(10, fieldWidth - 4 - scrollbarWidth); // border(2) + padding(2)
        s.contentTextarea.setWidth(innerWidth);
        s.contentTextarea.setHeight(DESIGN.textareaHeight);

        // Get the native textarea view - this includes proper cursor rendering
        const textareaView = s.contentTextarea.view();

        // Render a scrollbar synced to textarea scroll state.
        let textareaWithScrollbar = textareaView;
        if (s.inputScrollbar) {
            const h = s.contentTextarea.height();
            const contentH = s.contentTextarea.lineCount();
            // Use the actual viewport yOffset from the textarea (via unsafe access)
            const yOffset = s.contentTextarea.yOffset();

            s.inputScrollbar.setViewportHeight(h);
            s.inputScrollbar.setContentHeight(contentH);
            s.inputScrollbar.setYOffset(yOffset);
            s.inputScrollbar.setChars("█", "░");
            // Use setThumbForeground for opaque █ character (full block draws foreground, not background)
            s.inputScrollbar.setThumbForeground(COLORS.primary);
            s.inputScrollbar.setTrackForeground(COLORS.muted);
            textareaWithScrollbar = lipgloss.joinHorizontal(lipgloss.Top, textareaView, s.inputScrollbar.view());
        }

        const contentFieldRendered = lipgloss.joinVertical(
            lipgloss.Left,
            styles.label().render('Content (multi-line):'),
            cntStyle.width(fieldWidth).render(textareaWithScrollbar)
        );
        // Wrap textarea area in zone for mouse focus
        contentField = zone.mark('input-content', contentFieldRendered);
    }

    // Buttons with zone markers
    const submitBtn = zone.mark('btn-submit',
        (s.inputFocus === FOCUS_SUBMIT ? styles.buttonFocused() : styles.button()).render('[Submit]'));
    const cancelBtn = zone.mark('btn-cancel',
        (s.inputFocus === FOCUS_CANCEL ? styles.buttonDanger() : styles.button()).render('[Cancel]'));
    const buttonRow = lipgloss.joinHorizontal(lipgloss.Top, submitBtn, cancelBtn);

    // Help text (fixed footer)
    const helpText = styles.help().render('Tab: Cycle Focus    Enter: Newline (Content) / Submit    Esc: Cancel');

    // Per AGENTS.md: The entire edit page (including buttons, except hints and title header) 
    // should scroll with a scrollbar
    // Build scrollable content: label + content + buttons
    const scrollableSections = [lblField];
    if (contentField) {
        scrollableSections.push('', contentField);
    }
    scrollableSections.push('', buttonRow);
    const scrollableContent = lipgloss.joinVertical(lipgloss.Left, ...scrollableSections);

    // Calculate available height for the scrollable area
    // Fixed elements: title (1 line) + gap (1) + gap before help (1) + help (1) = 4 lines
    const titleHeight = lipgloss.height(title);
    const helpHeight = lipgloss.height(helpText);
    const fixedHeight = titleHeight + helpHeight + 2; // 2 for gaps
    const scrollableHeight = Math.max(3, termHeight - fixedHeight);
    
    const scrollableContentHeight = lipgloss.height(scrollableContent);
    
    // If content fits, render directly without viewport
    // Otherwise use the persistent inputVp for scrolling (preserves scroll position)
    let visibleContent;
    if (scrollableContentHeight <= scrollableHeight) {
        // Content fits - no scrolling needed
        visibleContent = scrollableContent;
    } else {
        // Content too tall - use persistent inputVp for scrolling
        // NOTE: inputVp is stored in state to preserve scroll position across renders
        s.inputVp.setWidth(termWidth - scrollbarWidth);
        s.inputVp.setHeight(scrollableHeight);
        s.inputVp.setContent(scrollableContent);
        visibleContent = s.inputVp.view();
    }

    // Compose final view
    return lipgloss.joinVertical(
        lipgloss.Left,
        title,
        '',
        visibleContent,
        '',
        helpText
    );
}

// renderViewer removed - view mode is redundant per AGENTS.md

function renderConfirm(s) {
    const title = styles.title().render('⚠️ Confirm Delete');
    const prompt = s.confirmPrompt;

    // Buttons with zone markers
    const yesBtn = zone.mark('btn-yes', styles.buttonDanger().render('[Y]es'));
    const noBtn = zone.mark('btn-no', styles.button().render('[N]o'));
    const buttonRow = lipgloss.joinHorizontal(lipgloss.Top, yesBtn, noBtn);

    const helpText = styles.help().render('y: Confirm    n/Esc: Cancel');

    return lipgloss.joinVertical(
        lipgloss.Left,
        title,
        '',
        prompt,
        '',
        buttonRow,
        '',
        helpText
    );
}

// ============================================================================
// COMMANDS
// ============================================================================

function buildCommands() {
    // Context Manager with injected state
    const ctxmgr = contextManager({
        getItems: () => state.get(shared.contextItems) || [],
        setItems: (v) => state.set(shared.contextItems, v),
        nextIntegerId: nextIntegerId,
        buildPrompt: buildFinalPrompt
    });

    return {
        // Base context commands (add, list, remove, etc.)
        ...ctxmgr.commands,

        // --- Super Document Specific Commands ---

        "doc-add": {
            description: "Add a new document",
            usage: "doc-add [content] OR doc-add --file <path>",
            handler: function (args) {
                if (args.length >= 2 && args[0] === "--file") {
                    const path = args[1];
                    const res = osm.readFile(path);
                    if (res.error) {
                        output.print("Error: " + res.error);
                        return;
                    }
                    const doc = addDocument(path, res.content);
                    output.print(`Added document #${doc.id} from ${path}`);
                } else {
                    const content = args.join(" ");
                    const doc = addDocument(null, content);
                    output.print(`Added document #${doc.id}`);
                }
            }
        },
        "doc-rm": {
            description: "Remove a document",
            usage: "doc-rm <id>",
            handler: function (args) {
                if (!args[0]) {
                    output.print("Usage: doc-rm <id>");
                    return;
                }
                const doc = removeDocumentById(parseInt(args[0]));
                if (doc) output.print(`Removed document #${doc.id}`);
                else output.print("Document not found");
            }
        },
        "doc-list": {
            description: "List super-documents and context items",
            usage: "doc-list",
            handler: function () {
                const docs = getDocuments();
                const ctxItems = state.get(shared.contextItems) || [];
                
                if (docs.length === 0 && ctxItems.length === 0) {
                    output.print("No documents or context items.");
                    return;
                }
                
                // Show super documents
                if (docs.length > 0) {
                    output.print("Super Documents:");
                    docs.forEach(d => {
                        const prev = (d.content || "").substring(0, 50).replace(/\n/g, " ");
                        output.print(`  #${d.id} [${d.label}]: ${prev}...`);
                    });
                }
                
                // Show context items from ctxutil
                if (ctxItems.length > 0) {
                    if (docs.length > 0) output.print(""); // blank line separator
                    output.print("Context Items:");
                    ctxItems.forEach(item => {
                        const type = item.type || 'unknown';
                        const name = item.name || item.path || 'unnamed';
                        output.print(`  [${type}] ${name}`);
                    });
                }
            }
        },
        "doc-view": {
            description: "View a specific document content",
            usage: "doc-view <id>",
            handler: function (args) {
                if (!args[0]) {
                    output.print("Usage: doc-view <id>");
                    return;
                }
                const doc = getDocumentById(parseInt(args[0]));
                if (doc) {
                    output.print(`--- Document #${doc.id}: ${doc.label} ---`);
                    output.print(doc.content);
                    output.print("---");
                } else output.print("Document not found");
            }
        },
        "doc-clear": {
            description: "Clear all documents",
            handler: function () {
                setDocuments([]);
                output.print("All documents cleared.");
            }
        },
        "copy": {
            description: "Copy the final prompt to clipboard",
            handler: function () {
                if (getDocuments().length === 0) {
                    output.print("No documents.");
                    return;
                }
                const txt = buildFinalPrompt();
                try {
                    ctxmgr.clipboardCopy(txt);
                    output.print(`Copied ${txt.length} chars to clipboard.`);
                } catch (e) {
                    output.print("Clipboard error: " + e);
                }
            }
        },
        "tui": {
            description: "Open the Visual TUI interface",
            handler: function () {
                // Clear any previous shell request (module-level, NOT persistent)
                _userRequestedShell = false;

                try {
                    // Launch the embedded Bubble Tea app
                    const res = runVisualTui();
                    if (res && res.error) {
                        output.print("TUI Error: " + res.error);
                    }
                } finally {
                    // Check if user explicitly requested shell mode (module-level flag)
                    const wantedShell = _userRequestedShell;

                    // Clear the flag for next time
                    _userRequestedShell = false;

                    // If user pressed 'q' (not 's'), signal the shell loop to exit
                    // The shell loop's exit checker will see this and terminate gracefully
                    // This does NOT call os.Exit - the shell loop handles exit at the top level
                    if (!wantedShell) {
                        tui.requestExit();
                    } else {
                        // Inform the user they're now in shell mode and how to get back
                        output.print("Dropped to shell. Use the 'tui' command to return to the visual UI.");
                    }
                }
            }
        }
    };
}

// ============================================================================
// REGISTRATION
// ============================================================================

// config.name is injected by Go as "super-document"
// config.replMode is injected by Go based on --repl flag

tui.registerMode({
    name: COMMAND_NAME,
    tui: {
        title: "Super Document",
        prompt: `(${COMMAND_NAME}) > `,
    },
    onEnter: function () {
        output.print("Super Document Mode. Type 'tui' for visual interface, 'help' for commands.");
    },
    commands: function () {
        return buildCommands();
    },
    // If shellMode is true (--shell flag passed), don't auto-launch TUI
    // Otherwise, launch TUI immediately for visual interface
    initialCommand: config.shellMode ? undefined : "tui",
});

tui.switchMode(COMMAND_NAME);
