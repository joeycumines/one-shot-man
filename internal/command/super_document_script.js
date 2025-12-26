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
// |    📄 Super-Document Builder                                 ✓ Status: Ready |
// | ┌──────────────────────────────────────────────────────────────────────────┐ |
// | │                                                                          │ |
// | │ Documents: 4                                                             │ |
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
// | │                                                                          │ |
// | │ (  [A]dd  ) ( [L]oad  ) ( [C]opy  ) ( [S]hell ) ( [R]eset ) ( [Q]uit  )  │ |
// | │                                                                          │ |
// | └──────────────────────────────────────────────────────────────────────────┘ |
// |                                                                              |
// | ──────────────────────────────────────────────────────────────────────────── |
// |  a: Add  l: Load  e: Edit  d: Delete  c: Copy  s: Shell  r: Reset  q: Quit   |
// +==============================================================================+
//
// SCENARIO B: NARROW/VERTICAL TERMINAL (Responsive Stack)
// +==============================================================================+
// | 📄 Super-Document Builder                                                    |
// | ✓ Status: Ready                                                              |
// | ┌──────────────────────────────────┐                                         |
// | │                                  │                                         |
// | │ Docs: 4                          │                                         |
// | │ ╔══════════════════════════════╗ │                                         |
// | │ ║ #1 [Label]        [X] Remove ║ │                                         |
// | │ ║                              ║ │                                         |
// | │ ║  Preview...                  ║ │                                         |
// | │ ╚══════════════════════════════╝ │                                         |
// | └──────────────────────────────────┘                                         |
// |                                                                              |
// |  (  [A]dd  )  ( [L]oad  )                                                    |
// |  ( [C]opy  )  ( [S]hell )                                                    |
// |  ( [R]eset )  ( [Q]uit  )                                                    |
// |                                                                              |
// | ──────────────────────────────────                                           |
// |  a: Add  l: Load  e: Edit                                                    |
// |  d: Del   c: Copy                                                            |
// |  s: Shell r: Reset q: Quit                                                   |
// +==============================================================================+
//
// SCENARIO C: INPUT FORM (Textarea Mode)
// +==============================================================================+
// |    📝 Add / Edit Document                                   [ ↑ ] [ ↓ ]      |
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

// Strict validation of injected globals
// The Go host must inject 'superDocumentTemplate' to prevent drift/errors.
if (typeof superDocumentTemplate !== 'string' || superDocumentTemplate === '') {
    throw new Error("Critical configuration error: global 'superDocumentTemplate' is missing or empty. The host application must inject this.");
}

function getTemplate() {
    const tmpl = state.get(stateKeys.template);
    // Return persistent state override if set, otherwise the mandatory global default.
    return (tmpl !== null && tmpl !== undefined && tmpl !== "") ? tmpl : superDocumentTemplate;
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
        id: id, label: label || ('Document ' + (docs.length + 1)), content: content || ''
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
        documents: docs, contextTxtar: contextTxtar
    });
}

// ============================================================================
// VIEWPORT & LAYOUT HELPERS
// ============================================================================

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
        // Document box structure (with border and padding):
        // - Top border (1 line)
        // - Header line(s): #id [label] (may wrap)
        // - Preview line(s) (may wrap)
        // - Remove button line (1 line, always "[X] Remove")
        // - Bottom border (1 line)

        let prev = previewOf(doc.content, DESIGN.previewMaxLen);

        const docHeader = `#${doc.id} [${doc.label}]`;
        const docPreview = styles.preview().render(prev);
        const removeBtn = '[X] Remove';

        // Render each component individually to measure their heights at the content width
        // Subtract border padding (docPaddingH * 2) and border itself (2) from width
        const contentInnerWidth = Math.max(1, docContentWidth - (DESIGN.docPaddingH * 2) - 2);

        // For each element, we need to compute rendered height
        // We use a temporary style to measure wrapped height
        const measuringStyle = lipgloss.newStyle().width(contentInnerWidth);
        const headerRendered = measuringStyle.render(docHeader);
        const previewRendered = measuringStyle.render(docPreview);

        const headerHeight = lipgloss.height(headerRendered);
        const previewHeight = lipgloss.height(previewRendered);
        const removeBtnHeight = 1; // Remove button is always 1 line

        // Calculate removeButton line offset within the document box:
        // topBorder (1) + header + preview = position where removeBtn starts
        const removeButtonLineOffset = 1 + headerHeight + previewHeight;

        // Calculate bottom border line offset:
        // removeButtonLineOffset + removeBtnHeight = bottom border position
        const bottomBorderLineOffset = removeButtonLineOffset + removeBtnHeight;

        const docContent = lipgloss.joinVertical(lipgloss.Left, docHeader, docPreview, removeBtn);
        const style = styles.document(); // Use base style for height calculation
        // CRITICAL: Use consistent width passed from renderList
        const renderedDoc = style.width(docContentWidth).render(docContent);
        const docHeight = lipgloss.height(renderedDoc);

        layoutMap.push({
            top: currentTop,
            height: docHeight,
            docId: doc.id,
            removeButtonLineOffset: removeButtonLineOffset,
            bottomBorderLineOffset: bottomBorderLineOffset,
            headerHeight: headerHeight
        });

        currentTop += docHeight;
    });

    return layoutMap;
}

// Ensure the selected document or focused buttons are visible in the viewport
// This provides a holistic fix for "viewport must follow focus"
function ensureSelectionVisible(s) {
    if (!s.vp) return;

    // Case 1: Button Focus (Bottom of Viewport)
    // If buttons are focused, we must ensure the bottom of the content is visible.
    // We scroll to the totalLineCount to ensure the absolute bottom is reached,
    // solving rounding errors where manual page scrolling might fall short.
    if (s.focusedButtonIdx >= 0) {
        const totalLines = s.vp.totalLineCount();
        s.vp.setYOffset(totalLines); // Viewport clamps this to the maximum valid offset
        return;
    }

    // Case 2: Document Selection (Standard LayoutMap Logic)
    if (!s.layoutMap || s.layoutMap.length === 0) return;
    if (s.selectedIdx < 0 || s.selectedIdx >= s.layoutMap.length) return;

    // Adjust for the new top padding and count line in the scrollable area
    // Padding (1) + Count Line (1) = 2 lines offset
    const scrollableOffset = 2;

    const entry = s.layoutMap[s.selectedIdx];
    const top = entry.top + scrollableOffset;
    const yOffset = s.vp.yOffset();
    const vpHeight = s.vp.height();

    // Check if selection is above the viewport
    if (top < yOffset) {
        s.vp.setYOffset(top);
        return;
    }

    // Check if selection is below the viewport
    const selectionBottom = top + entry.height;
    if (selectionBottom > yOffset + vpHeight) {
        s.vp.setYOffset(selectionBottom - vpHeight);
    }
}

// Find which document was clicked based on viewport-relative coordinates
// Returns {index: docIndex, relativeLineInDoc: lineOffset} or null if no hit
function findClickedDocument(s, relativeY) {
    if (!s.layoutMap || s.layoutMap.length === 0) return null;

    // Adjust for the new top padding and count line in the scrollable area
    // relativeY is relative to viewport content.
    // The documents start after: Padding (1) + Count Line (1) = 2 lines.
    const scrollableOffset = 2;
    const adjustedY = relativeY - scrollableOffset;

    if (adjustedY < 0) return null; // Clicked on padding or count line

    // relativeY is already adjusted for viewport offset
    for (let i = 0; i < s.layoutMap.length; i++) {
        const entry = s.layoutMap[i];
        if (adjustedY >= entry.top && adjustedY < entry.top + entry.height) {
            return {
                index: i, relativeLineInDoc: adjustedY - entry.top
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

// Button definitions for keyboard navigation
// Order matches the ASCII design: [A]dd  [L]oad  [C]opy  [S]hell  [R]eset  [Q]uit
const BUTTONS = [{id: 'btn-add', key: 'a', label: '[A]dd'}, {
    id: 'btn-load',
    key: 'l',
    label: '[L]oad'
}, {id: 'btn-copy', key: 'c', label: '[C]opy'}, {id: 'btn-shell', key: 's', label: '[S]hell'}, {
    id: 'btn-reset',
    key: 'r',
    label: '[R]eset'
}, {id: 'btn-quit', key: 'q', label: '[Q]uit'},];

// TUI Design
const DESIGN = {
    buttonPaddingH: 2,
    buttonPaddingV: 0,
    buttonMarginR: 1,
    docPaddingH: 1,
    docPaddingV: 0,
    docMarginB: 1,
    previewMaxLen: 50,
    // Removed fixed textarea constraints as requested
};

// TUI Styles - Strict configuration requirement
if (typeof config !== 'object' || config === null || !config.theme) {
    throw new Error("Critical configuration error: 'config.theme' is missing. The host application must inject the theme.");
}

{
    const requiredThemeKeys = ['primary', 'secondary', 'danger', 'muted', 'bg', 'fg', 'warning', 'focus'];
    const missingThemeKeys = requiredThemeKeys.filter(key => !config.theme[key]);
    if (missingThemeKeys.length > 0) {
        throw new Error("Critical configuration error: 'config.theme' is missing required keys: " + missingThemeKeys.join(', '));
    }
}

const COLORS = config.theme;

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
    jumpIcon: () => lipgloss.newStyle().foreground(COLORS.primary).bold(true).padding(0, 1).height(1),
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
        inputScrollbar: scrollbarLib.new(), // Outer page scrollbar
        // textareaScrollbar removed as textarea expands fully
        vpContentWidth: 0,

        // LayoutMap cache: maps document index -> {top: y, height: h}
        // Rebuilt only when documents array changes
        layoutMap: null,
        layoutMapDocsHash: '', // Hash to detect document changes

        // Viewport unlock flag for input mode:
        // When true, the viewport can scroll freely without snapping to cursor.
        // Set true on scroll events, set false on typing (to snap back to cursor).
        inputViewportUnlocked: false,

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
        width: 80,
        height: 24,
        layout: {buttons: [], docBoxes: []},

        // Button navigation: -1 = document list focused, 0+ = button index focused
        focusedButtonIdx: -1,

        // Textarea content area bounds for mouse click positioning
        // These are set in renderInput and used in handleMouse
        textareaBounds: {
            // Screen position of the textarea content area (inside border/padding)
            contentTop: 0,       // Y offset from screen top to first content line
            contentLeft: 0,      // X offset from screen left to first content column
            lineNumberWidth: 0,  // Width of line numbers column
            promptWidth: 0,      // Width of prompt column
        },

        // Flag to force a full screen clear on first render
        // This ensures the title line is rendered correctly when re-entering TUI from shell
        needsInitClear: true
    };

    const model = tea.newModel({
        init: function () {
            return initialState;
        }, update: function (msg, tuiState) {
            // DEBUG: Log all messages to understand what's being received
            if (typeof console !== 'undefined' && msg.type === 'Mouse') {
                console.log('UPDATE DEBUG: msg.type=' + msg.type + ', msg.action=' + msg.action + ', msg.button=' + msg.button + ', msg.x=' + msg.x + ', msg.y=' + msg.y);
            }
            // Sync documents from global state on every update to ensure freshness
            tuiState.documents = getDocuments();

            if (msg.type === 'WindowSize') {
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
            } else if (msg.type === 'Key') {
                return handleKeys(msg, tuiState);
            } else if (msg.type === 'Mouse' && msg.action === 'press') {
                return handleMouse(msg, tuiState);
            }
            return [tuiState, null];
        }, view: function (tuiState) {
            return renderView(tuiState);
        }
    });

    return tea.run(model, {altScreen: true, mouse: true});
}

function configureTextarea(ta, width) {
    ta.setPlaceholder("Enter document content...");
    ta.setWidth(Math.max(40, width - 10));
    // Initial height 1, will be expanded in renderInput
    ta.setHeight(1);
    ta.setShowLineNumbers(true);

    // CRITICAL FIX: The upstream bubbles/textarea has a default MaxHeight of 99 lines.
    // This prevents users from entering more than ~100 lines via Enter key.
    // Set MaxHeight to 0 to disable the limit entirely.
    ta.setMaxHeight(0);

    // HIGH VISIBILITY STYLING & FIXES
    // Ensure selected text is visible (fix black bar) by setting specific style.
    // IMPORTANT: We set prompt to {} (no styling) to prevent double-rendering issues.
    // When Prompt has styling (e.g., foreground), it renders with ANSI codes like \x1b[37m.
    // Then CursorLine's Render() wraps that already-styled string, causing lipgloss to
    // treat the escape sequences as literal characters and style each one individually.
    // By clearing Prompt style, we let CursorLine handle ALL styling for the current line.
    ta.setFocusedStyle({
        prompt: {},  // Clear default prompt styling to prevent double-render corruption
        cursorLine: {
            // NOTE: Do NOT use underline here! When underline is true, lipgloss iterates
            // character-by-character to handle underlineSpaces, which corrupts any
            // ANSI escape codes in the input (from nested Cursor.View() styling).
            // The bubbles textarea has a bug where it applies CursorLine styling twice:
            // 1. m.Cursor.TextStyle = computedCursorLine()
            // 2. style.Render(m.Cursor.View()) where style is also CursorLine
            // Without underline, lipgloss uses a faster path that preserves ANSI codes.
            //
            // FIX: Use focus color as background highlight to make cursor line visible.
            // Previously used COLORS.bg (white) which made text invisible on white terminals.
            background: COLORS.focus, // Blue highlight for current line (visible on white)
            foreground: COLORS.bg     // White text on blue background for contrast
        },
        cursorLineNumber: {
            foreground: COLORS.warning,
            bold: true
        },
        text: {
            foreground: COLORS.fg
        },
        placeholder: {
            foreground: COLORS.muted
        }
    });

    // NOTE: Do NOT set cursor style via setCursorForeground when CursorLine styling is active!
    // bubbles textarea.go line 1170 does: style.Render(m.Cursor.View())
    // Cursor.View() already renders with TextStyle, then CursorLine wraps it AGAIN,
    // causing ANSI escape codes to be treated as literal text (double-render corruption).
    // The cursor will use the CursorLine styling instead.

    // Set cursor block styling to ensure visibility on both light and dark backgrounds.
    // The cursor block (the blinking character) needs explicit foreground/background
    // to be visible when the CursorLine styling is applied.
    ta.setCursorStyle({
        foreground: COLORS.warning, // Amber/yellow cursor for high visibility
        background: COLORS.primary  // Indigo background for contrast
    });
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

        // Arrow key navigation with auto-scroll (documents + buttons)
        if (k === 'up' || k === 'k') {
            if (s.focusedButtonIdx >= 0) {
                // Currently on buttons - up goes back to last document
                s.focusedButtonIdx = -1;
                if (s.documents.length > 0) {
                    s.selectedIdx = s.documents.length - 1;
                    ensureSelectionVisible(s);
                }
            } else if (s.selectedIdx > 0) {
                // In document list - move up normally
                s.selectedIdx = s.selectedIdx - 1;
                ensureSelectionVisible(s);
            } else if (s.selectedIdx === 0) {
                // At first document - deselect and scroll to top
                // This enables "de-highlight everything" to reach the document count area
                s.selectedIdx = -1; // No document selected
                if (s.vp) s.vp.setYOffset(0); // Scroll viewport to absolute top
            } else if (s.selectedIdx < 0 && s.vp && s.vp.yOffset() > 0) {
                // Already deselected - just ensure we're at the absolute top
                s.vp.setYOffset(0);
            }
        }
        if (k === 'down' || k === 'j') {
            if (s.focusedButtonIdx >= 0) {
                // Already on buttons - down doesn't do anything (buttons are at bottom)
            } else if (s.selectedIdx < 0) {
                // No document selected - select first document
                if (s.documents.length > 0) {
                    s.selectedIdx = 0;
                    ensureSelectionVisible(s);
                } else {
                    // No documents - go to buttons
                    s.focusedButtonIdx = 0;
                    ensureSelectionVisible(s);
                }
            } else if (s.selectedIdx >= s.documents.length - 1) {
                // At bottom of document list - move to first button
                s.focusedButtonIdx = 0;
                ensureSelectionVisible(s); // Holistic fix: ensure buttons are visible
            } else {
                // In document list - move down normally
                s.selectedIdx = Math.min(s.documents.length - 1, s.selectedIdx + 1);
                ensureSelectionVisible(s);
            }
        }
        // Left/Right for button navigation
        if (k === 'left' || k === 'h') {
            if (s.focusedButtonIdx > 0) {
                s.focusedButtonIdx--;
            }
        }
        if (k === 'right' || k === 'l') {
            if (s.focusedButtonIdx >= 0 && s.focusedButtonIdx < BUTTONS.length - 1) {
                s.focusedButtonIdx++;
            }
        }
        // Tab cycles: docs -> buttons (forward)
        if (k === 'tab') {
            if (s.focusedButtonIdx < 0) {
                // From docs, go to first button
                s.focusedButtonIdx = 0;
                ensureSelectionVisible(s); // Ensure buttons visible
            } else if (s.focusedButtonIdx < BUTTONS.length - 1) {
                // Move to next button
                s.focusedButtonIdx++;
            } else {
                // From last button, wrap to docs
                s.focusedButtonIdx = -1;
                s.selectedIdx = 0;
                if (s.vp) s.vp.setYOffset(0);
            }
        }
        // Shift+Tab cycles: buttons -> docs (backward)
        if (k === 'shift+tab') {
            if (s.focusedButtonIdx > 0) {
                // Move to previous button
                s.focusedButtonIdx--;
            } else if (s.focusedButtonIdx === 0) {
                // From first button, go to last doc
                s.focusedButtonIdx = -1;
                if (s.documents.length > 0) {
                    s.selectedIdx = s.documents.length - 1;
                    ensureSelectionVisible(s); // Scroll back to doc
                }
            } else {
                // From docs, go to last button
                s.focusedButtonIdx = BUTTONS.length - 1;
                ensureSelectionVisible(s); // Ensure buttons visible
            }
        }
        // Enter on focused button activates it
        if (k === 'enter' && s.focusedButtonIdx >= 0) {
            const btn = BUTTONS[s.focusedButtonIdx];
            if (btn) {
                // Simulate the key press for the button action
                // Re-dispatch as the button's key
                // (use the same handling code below)
                if (btn.key === 'q') {
                    _userRequestedShell = false;
                    return [s, tea.quit()];
                }
                if (btn.key === 's') {
                    _userRequestedShell = true;
                    return [s, tea.quit()];
                }
                // For other buttons, set k to the button's key and fall through
                // We need to handle them specially to avoid code duplication
                // Note: a, l, c, r are handled below, so let's just set a marker
                // Actually, simpler: just set k and let it fall through
                // But k is a const! Need to refactor...
                // For now, handle inline:
                if (btn.key === 'a') {
                    s.mode = MODE_INPUT;
                    s.inputOperation = INPUT_ADD;
                    s.inputFocus = FOCUS_LABEL;
                    s.labelBuffer = '';
                    // Create native textarea for content
                    s.contentTextarea = textareaLib.new();
                    configureTextarea(s.contentTextarea, s.width);
                    s.focusedButtonIdx = -1; // Clear button focus when entering input mode
                    s.inputViewportUnlocked = false; // Reset viewport lock on mode entry
                }
                if (btn.key === 'l') {
                    s.mode = MODE_INPUT;
                    s.inputOperation = INPUT_LOAD;
                    s.inputFocus = FOCUS_LABEL;
                    s.labelBuffer = '';
                    s.contentTextarea = null; // No textarea for load mode
                    s.focusedButtonIdx = -1; // Clear button focus
                    s.inputViewportUnlocked = false; // Reset viewport lock on mode entry
                }
                if (btn.key === 'c') {
                    // Copy all documents
                    const allContent = s.documents.map(d => '# ' + d.label + '\n\n' + d.content).join('\n\n---\n\n');
                    if (allContent) {
                        s.clipboard = allContent;
                        s.statusMsg = 'Copied ' + s.documents.length + ' document(s) to clipboard';
                        s.hasError = false;
                    } else {
                        s.statusMsg = 'No documents to copy';
                        s.hasError = true;
                    }
                    s.focusedButtonIdx = -1;
                }
                if (btn.key === 'r') {
                    // Reset all - confirm
                    if (s.documents.length > 0) {
                        s.mode = MODE_CONFIRM;
                        s.confirmPrompt = 'Delete ALL ' + s.documents.length + ' document(s)? [y/N]';
                        s.confirmDocId = -1; // -1 means "reset all"
                    } else {
                        s.statusMsg = 'No documents to reset';
                        s.hasError = true;
                    }
                    s.focusedButtonIdx = -1;
                }
            }
        }

        // MANUAL PAGE KEY HANDLING - DO NOT forward to vp.update()!
        // PgDown: Move selection down by approximately a page worth of documents
        // If already at last document, move focus to buttons
        if (k === 'pgdown') {
            if (s.focusedButtonIdx >= 0) {
                // Already on buttons - no further down to go
            } else if (s.documents.length > 0 && s.vp) {
                const vpHeight = s.vp.height();
                // Estimate ~5 lines per document box
                const pageSize = Math.max(1, Math.floor(vpHeight / 5));
                const newIdx = Math.min(s.documents.length - 1, s.selectedIdx + pageSize);
                if (newIdx === s.selectedIdx && s.selectedIdx >= s.documents.length - 1) {
                    // At last document and trying to go further - move to buttons
                    s.focusedButtonIdx = 0;
                } else {
                    s.selectedIdx = newIdx;
                }
                ensureSelectionVisible(s);
            } else if (s.documents.length === 0) {
                // No documents - jump to buttons
                s.focusedButtonIdx = 0;
                ensureSelectionVisible(s);
            }
        }

        // PgUp: Move selection up by approximately a page worth of documents
        // If on buttons, return to document list
        if (k === 'pgup') {
            if (s.focusedButtonIdx >= 0) {
                // On buttons - go back to last document
                s.focusedButtonIdx = -1;
                if (s.documents.length > 0) {
                    s.selectedIdx = s.documents.length - 1;
                    ensureSelectionVisible(s);
                }
            } else if (s.selectedIdx < 0) {
                // Already deselected - just ensure we're at absolute top
                if (s.vp) s.vp.setYOffset(0);
            } else if (s.documents.length > 0 && s.vp) {
                const vpHeight = s.vp.height();
                const pageSize = Math.max(1, Math.floor(vpHeight / 5));
                const newIdx = Math.max(0, s.selectedIdx - pageSize);
                if (newIdx === 0 && s.selectedIdx === 0) {
                    // Already at first document - deselect and scroll to top
                    s.selectedIdx = -1;
                    s.vp.setYOffset(0);
                } else {
                    s.selectedIdx = newIdx;
                    ensureSelectionVisible(s);
                }
            } else if (s.documents.length === 0 && s.vp && s.vp.yOffset() > 0) {
                // No documents but viewport scrolled - scroll to top
                s.vp.setYOffset(0);
            }
        }

        // Home: Go to first document and clear button focus
        if (k === 'home') {
            s.focusedButtonIdx = -1;
            if (s.documents.length > 0) {
                s.selectedIdx = 0;
                if (s.vp) s.vp.setYOffset(0);
            }
        }

        // End: Go to last button (truly the end of the page)
        if (k === 'end') {
            s.focusedButtonIdx = BUTTONS.length - 1;
            ensureSelectionVisible(s); // Holistic fix ensures scroll to bottom
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
            configureTextarea(s.contentTextarea, s.width);
            s.focusedButtonIdx = -1; // Clear button focus
            s.inputViewportUnlocked = false; // Reset viewport lock on mode entry
        }
        if (k === 'l') {
            s.mode = MODE_INPUT;
            s.inputOperation = INPUT_LOAD;
            s.inputFocus = FOCUS_LABEL;
            s.labelBuffer = '';
            s.contentTextarea = null; // No textarea for load mode
            s.focusedButtonIdx = -1; // Clear button focus
            s.inputViewportUnlocked = false; // Reset viewport lock on mode entry
        }
        if (k === 'e' || (k === 'enter' && s.focusedButtonIdx < 0)) {
            // Edit document (only if no button is focused - 'enter' on button is handled above)
            const doc = s.documents[s.selectedIdx];
            if (doc) {
                s.mode = MODE_INPUT;
                s.inputOperation = INPUT_EDIT;
                s.inputFocus = FOCUS_CONTENT;
                s.labelBuffer = doc.label;
                // Create native textarea with existing content
                s.contentTextarea = textareaLib.new();
                configureTextarea(s.contentTextarea, s.width);

                s.contentTextarea.setValue(doc.content);
                s.contentTextarea.focus();
                s.editingDocId = doc.id;
                s.focusedButtonIdx = -1; // Clear button focus
                s.inputViewportUnlocked = false; // Reset viewport lock on mode entry
            }
        }
        // 'r' = Reset (clear all documents) per ASCII design
        if (k === 'r') {
            if (s.documents.length > 0) {
                s.mode = MODE_CONFIRM;
                s.confirmPrompt = `Reset ALL ${s.documents.length} documents? This cannot be undone. (y/n)`;
                s.confirmDocId = -1; // Special ID for reset-all
            } else {
                s.statusMsg = 'No documents to reset';
            }
            s.focusedButtonIdx = -1; // Clear button focus
        }
        // 'R' (uppercase) = Rename selected document title
        if (k === 'R') {
            const doc = s.documents[s.selectedIdx];
            if (doc) {
                s.mode = MODE_INPUT;
                s.inputOperation = INPUT_RENAME;
                s.inputFocus = FOCUS_LABEL;
                s.labelBuffer = doc.label;
                s.contentTextarea = null; // No textarea for rename mode
                s.editingDocId = doc.id;
                s.inputViewportUnlocked = false; // Reset viewport lock on mode entry
            }
            s.focusedButtonIdx = -1; // Clear button focus
        }
        if (k === 'd' || k === 'backspace') {
            const doc = s.documents[s.selectedIdx];
            if (doc) {
                s.mode = MODE_CONFIRM;
                s.confirmPrompt = `Delete document #${doc.id} "${doc.label}"? (y/n)`;
                s.confirmDocId = doc.id;
            }
            s.focusedButtonIdx = -1; // Clear button focus
        }
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
            s.focusedButtonIdx = -1; // Clear button focus
        }
        if (k === '?') s.statusMsg = 'a:add l:load e:edit R:rename d:del c:copy s:shell r:reset q:quit';
    } else if (s.mode === MODE_INPUT) {
        // PRECISE SCROLL & EVENT PROPAGATION FOR INPUT MODE

        // Explicitly handle PageUp/PageDown to scroll outer viewport effectively
        if (s.inputVp && (k === 'pgdown' || k === 'pgup')) {
            const pageSize = s.inputVp.height();
            if (k === 'pgup') {
                s.inputVp.scrollUp(pageSize);
            } else {
                s.inputVp.scrollDown(pageSize);
            }
            // CRITICAL: Unlock viewport so it doesn't snap back to cursor
            s.inputViewportUnlocked = true;
            return [s, null];
        }

        if (k === 'esc') {
            s.mode = MODE_LIST;
            s.statusMsg = 'Cancelled';
            s.inputViewportUnlocked = false; // Reset on mode exit
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
                    // When focusing textarea, reset viewport unlock so cursor is visible
                    s.inputViewportUnlocked = false;
                } else if (oldFocus === FOCUS_CONTENT) {
                    s.contentTextarea.blur();
                }
            }
        } else if (k === 'ctrl+enter' || (k === 'enter' && s.inputFocus !== FOCUS_CONTENT)) {
            // Submit
            if (s.inputFocus === FOCUS_CANCEL) {
                s.mode = MODE_LIST;
                s.inputViewportUnlocked = false; // Reset on mode exit
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
            s.inputViewportUnlocked = false; // Reset on mode exit
        } else {
            // Field input handling
            // Extract paste flag from message (bracketed paste mode)
            const isPaste = msg.paste === true;

            if (s.inputFocus === FOCUS_LABEL) {
                // Use Go-based validation for label input
                const validation = tea.isValidLabelInput(k, isPaste);
                if (validation.valid) {
                    if (k === 'backspace') {
                        s.labelBuffer = s.labelBuffer.slice(0, -1);
                    } else if (isPaste) {
                        // Paste event - extract content from bracketed format [content]
                        // and append to label (stripping brackets if present)
                        let pasteContent = k;
                        if (k.startsWith('[') && k.endsWith(']') && k.length > 2) {
                            pasteContent = k.slice(1, -1);
                        }
                        // For labels, only take first line and strip newlines
                        pasteContent = pasteContent.split('\n')[0].replace(/\r/g, '');
                        s.labelBuffer += pasteContent;
                    } else {
                        // Single printable character - add to label
                        s.labelBuffer += k;
                    }
                }
                // Silently discard ALL invalid input (garbage escape sequences, etc.)
            } else if (s.inputFocus === FOCUS_CONTENT && s.contentTextarea) {
                // INTERCEPT SPECIAL HOTKEYS before delegating to textarea
                // The upstream bubbles/textarea has different keybindings than web forms:
                //   - ctrl+a = line start (not select all)
                //   - ctrl+e = line end
                // We intercept some keys for browser-like behavior.

                // Ctrl+A: Select All - copy all content to clipboard
                // Since upstream textarea doesn't support selection ranges,
                // we implement "select all" as "copy all to clipboard"
                if (k === 'ctrl+a') {
                    const allContent = s.contentTextarea.value();
                    if (allContent) {
                        try {
                            osm.clipboardCopy(allContent);
                            s.statusMsg = 'All content copied to clipboard (' + allContent.length + ' chars)';
                            s.hasError = false;
                        } catch (e) {
                            s.statusMsg = 'Clipboard error: ' + e;
                            s.hasError = true;
                        }
                    } else {
                        s.statusMsg = 'No content to select';
                        s.hasError = false;
                    }
                    return [s, null];
                }

                // Ctrl+Home: Go to beginning of document
                if (k === 'ctrl+home') {
                    s.contentTextarea.setPosition(0, 0);
                    s.inputViewportUnlocked = false;
                    return [s, null];
                }

                // Ctrl+End: Go to end of document
                if (k === 'ctrl+end') {
                    s.contentTextarea.selectAll(); // Moves to absolute end
                    s.inputViewportUnlocked = false;
                    return [s, null];
                }

                // Use Go-based validation for textarea input
                // This prevents garbage (fragmented escape sequences from rapid scroll)
                // from being inserted into the document content.
                const validation = tea.isValidTextareaInput(k, isPaste);
                if (msg.type === 'Key' && validation.valid) {
                    // Delegate to native textarea component
                    s.contentTextarea.update(msg);
                    // CRITICAL: Reset viewport unlock when user types, so view snaps to cursor
                    s.inputViewportUnlocked = false;
                }
                // Silently discard ALL invalid input (garbage)
            }
        }
    } else if (s.mode === MODE_CONFIRM) {
        if (k === 'y' || k === 'Y') {
            if (s.confirmDocId === -1) {
                // Reset all documents
                const count = s.documents.length;
                setDocuments([]);
                s.documents = [];
                s.selectedIdx = 0;
                state.set(stateKeys.selectedIndex, 0);
                s.statusMsg = 'Reset: cleared ' + count + ' documents';
            } else {
                // Delete single document
                const deletedId = s.confirmDocId;
                removeDocumentById(s.confirmDocId);
                s.documents = getDocuments();  // Refresh local state from global after mutation
                s.statusMsg = 'Deleted document #' + deletedId;
            }
            s.mode = MODE_LIST;
        } else if (k === 'n' || k === 'N' || k === 'esc') {
            s.statusMsg = 'Cancelled';
            s.mode = MODE_LIST;
        }
    }

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
    // Use msg.isWheel property from MouseEventToJS for proper wheel detection
    // This aligns with tea.MouseEvent.IsWheel() and handles all wheel button types
    const isWheelEvent = msg.isWheel === true;

    // Handle wheel events for scrolling in list mode via viewport
    if (isWheelEvent && s.mode === MODE_LIST && s.documents.length > 0 && s.vp) {
        // Mouse button strings now match tea.MouseEvent.String(): "wheel up", "wheel down", etc.
        if (msg.button === 'wheel up') {
            s.vp.scrollUp(3); // Scroll viewport up by 3 lines
        } else if (msg.button === 'wheel down') {
            s.vp.scrollDown(3); // Scroll viewport down by 3 lines
        }
        // Don't change selection on wheel - just scroll the viewport
        return [s, null];
    }

    // Handle wheel events for scrolling in input mode via inputVp
    if (isWheelEvent && s.mode === MODE_INPUT && s.inputVp) {
        // PRECISE MOUSE SCROLL PROPAGATION
        // CHANGE: Removed scroll capture in textarea. Events now always bubble to outer viewport.

        // Standard outer scroll
        if (msg.button === 'wheel up') {
            s.inputVp.scrollUp(3); // Scroll input viewport up by 3 lines
        } else if (msg.button === 'wheel down') {
            s.inputVp.scrollDown(3); // Scroll input viewport down by 3 lines
        }
        // CRITICAL: Unlock viewport so it doesn't snap back to cursor on next render
        s.inputViewportUnlocked = true;
        return [s, null];
    }

    // Only left-button presses trigger button/document activation
    if (!isLeftClick) {
        return [s, null];
    }

    // Use bubblezone for proper hit-testing - no hardcoded coordinates
    // Buttons are marked with zone IDs like "btn-add", "btn-load", etc.
    const buttonActions = [{id: 'btn-add', action: 'a'}, {id: 'btn-load', action: 'l'}, {
        id: 'btn-copy',
        action: 'c'
    }, {id: 'btn-shell', action: 'shell'}, {id: 'btn-reset', action: 'reset'}, {
        id: 'btn-quit',
        action: 'quit'
    }, {id: 'btn-submit', action: 'submit'}, {id: 'btn-cancel', action: 'esc'}, {
        id: 'btn-yes',
        action: 'y'
    }, {id: 'btn-no', action: 'n'},
        // Jump icons
        {id: 'btn-top', action: 'jump-top'}, {id: 'btn-bottom', action: 'jump-bottom'}
    ];

    // Check button zones (debug removed)
    for (const btn of buttonActions) {
        if (zone.inBounds(btn.id, msg)) {
            if (btn.action === 'shell') {
                _userRequestedShell = true;
                return [s, tea.quit()];
            }
            if (btn.action === 'quit') {
                _userRequestedShell = false;
                return [s, tea.quit()];
            }
            if (btn.action === 'jump-top') {
                if (s.mode === MODE_INPUT && s.inputVp) {
                    // De-focus textarea first (per requirement: defocus then scroll)
                    if (s.inputFocus === FOCUS_CONTENT && s.contentTextarea) {
                        s.contentTextarea.blur();
                        s.inputFocus = FOCUS_LABEL; // Move focus to label field
                    }
                    s.inputVp.setYOffset(0);
                    // CRITICAL: Unlock viewport so it stays at top
                    s.inputViewportUnlocked = true;
                }
                return [s, null];
            }
            if (btn.action === 'jump-bottom') {
                if (s.mode === MODE_INPUT && s.inputVp) {
                    const maxOffset = Math.max(0, s.inputVp.totalLineCount() - s.inputVp.height());
                    s.inputVp.setYOffset(maxOffset);
                    // CRITICAL: Unlock viewport so it stays at bottom
                    s.inputViewportUnlocked = true;
                }
                return [s, null];
            }
            if (btn.action === 'reset') {
                // Reset clears all documents - show confirmation
                if (s.documents.length > 0) {
                    s.mode = MODE_CONFIRM;
                    s.confirmPrompt = `Reset ALL ${s.documents.length} documents? This cannot be undone. (y/n)`;
                    s.confirmDocId = -1; // Special ID for reset-all
                    return [s, null];
                } else {
                    s.statusMsg = 'No documents to reset';
                    return [s, null];
                }
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

        // COORDINATE-BASED TEXTAREA HIT DETECTION
        // Zone-based detection fails for large scrolled documents because the zone marker
        // doesn't correctly account for viewport scroll offset. Use coordinate math instead.
        //
        // Layout (Y-axis from top of screen):
        //   titleHeight (1 line) - fixed header
        //   [viewport starts here - scrollable area]
        //     lblField (label field with border)
        //     gap (1 empty line)
        //     contentLabel ("Content (multi-line):" - 1 line)
        //     border top (1 line)
        //     [TEXTAREA CONTENT - variable height]
        //     border bottom (1 line)
        //   [buttons, etc.]
        //   footer - fixed
        //
        // We need to check if the click is within the VISIBLE textarea bounds.
        let clickedInTextareaArea = false;
        if (s.textareaBounds && s.inputVp) {
            const titleHeight = 1;
            const vpYOffset = s.inputVp.yOffset();
            const vpHeight = s.inputVp.height();

            // Screen Y position of viewport top and bottom
            const vpScreenTop = titleHeight;
            const vpScreenBottom = vpScreenTop + vpHeight;

            // Check if click is within viewport vertical bounds
            if (msg.y >= vpScreenTop && msg.y < vpScreenBottom) {
                // Convert screen Y to content-space Y
                const viewportRelativeY = msg.y - titleHeight;
                const contentY = viewportRelativeY + vpYOffset;

                // Textarea content area starts at textareaBounds.contentTop in content-space
                // and extends for the height of the textarea (accounting for soft-wrapping)
                const textareaContentTop = s.textareaBounds.contentTop;
                // CRITICAL FIX: Use visualLineCount for proper height calculation with soft-wrapped lines
                const textareaVisualLines = s.contentTextarea ?
                    (s.contentTextarea.visualLineCount ? s.contentTextarea.visualLineCount() : s.contentTextarea.lineCount()) : 0;
                // Content area height = visual lines + buffer (the height we set)
                const textareaContentHeight = Math.max(1, textareaVisualLines + 1);
                const textareaContentBottom = textareaContentTop + textareaContentHeight;

                // Check if content Y is within textarea content area
                if (contentY >= textareaContentTop && contentY < textareaContentBottom) {
                    // Also check X bounds - should be within the field width
                    const fieldLeftEdge = 0; // Leftmost edge of the field
                    const fieldRightEdge = s.textareaBounds.fieldWidth || s.width;
                    if (msg.x >= fieldLeftEdge && msg.x < fieldRightEdge) {
                        clickedInTextareaArea = true;
                    }
                }
            }
        }

        // Also try zone-based detection as fallback for smaller content
        const zoneHit = zone.inBounds('input-content', msg);

        if (clickedInTextareaArea || zoneHit) {
            // Click on content textarea - focus it and position cursor
            if (s.inputFocus !== FOCUS_CONTENT) {
                s.inputFocus = FOCUS_CONTENT;
                if (s.contentTextarea) {
                    s.contentTextarea.focus();
                }
            }

            // GO-NATIVE CLICK HANDLING
            // Uses handleClickAtScreenCoords() which does ALL coordinate translation in Go.
            // This replaces manual JS coordinate math for PERFORMANCE and CORRECTNESS.
            // The Go method handles: screen→viewport→content→textarea→visual→logical mapping.
            if (s.contentTextarea && isLeftClick && !isWheelEvent) {
                const titleHeight = 1;

                // Try GO-NATIVE method first (does all math in Go for performance)
                if (s.contentTextarea.handleClickAtScreenCoords) {
                    const hitResult = s.contentTextarea.handleClickAtScreenCoords(msg.x, msg.y, titleHeight);
                    if (hitResult.hit) {
                        // Cursor was successfully positioned by Go
                        s.inputViewportUnlocked = false;
                    }
                } else if (s.textareaBounds && s.contentTextarea.performHitTest) {
                    // Fallback: Use legacy manual coordinate calculation
                    const vpYOffset = s.inputVp ? s.inputVp.yOffset() : 0;
                    const viewportRelativeY = msg.y - titleHeight;
                    const contentRelativeY = viewportRelativeY + vpYOffset;
                    const visualY = contentRelativeY - s.textareaBounds.contentTop;
                    const visualX = Math.max(0, msg.x - s.textareaBounds.contentLeft);

                    if (visualY >= 0) {
                        const hitResult = s.contentTextarea.performHitTest(visualX, visualY);
                        s.contentTextarea.setPosition(hitResult.row, hitResult.col);
                        s.inputViewportUnlocked = false;
                    }
                } else if (s.textareaBounds) {
                    // Last resort fallback for backwards compatibility
                    const vpYOffset = s.inputVp ? s.inputVp.yOffset() : 0;
                    const viewportRelativeY = msg.y - titleHeight;
                    const contentRelativeY = viewportRelativeY + vpYOffset;
                    const visualY = contentRelativeY - s.textareaBounds.contentTop;
                    const visualX = Math.max(0, msg.x - s.textareaBounds.contentLeft);

                    if (visualY >= 0) {
                        s.contentTextarea.setPosition(visualY, visualX);
                        s.inputViewportUnlocked = false;
                    }
                }
            }
            return [s, null];
        }

        // CATCH-ALL BLUR LOGIC (FIXED)
        // Only blur if clicking OUTSIDE the scrollable viewport area entirely.
        // Previously this would fire for any click that didn't match a zone,
        // causing incorrect blurs when clicking on large scrolled documents.
        if (s.inputFocus === FOCUS_CONTENT && s.contentTextarea && s.inputVp) {
            const titleHeight = 1;
            const vpHeight = s.inputVp.height();
            const vpScreenTop = titleHeight;
            const vpScreenBottom = vpScreenTop + vpHeight;

            // Only blur if click is OUTSIDE the viewport Y bounds
            // (clicking within viewport but outside textarea should keep focus)
            if (msg.y < vpScreenTop || msg.y >= vpScreenBottom) {
                s.contentTextarea.blur();
                s.inputFocus = FOCUS_LABEL;
            }
            // If click is within viewport but not on textarea, we still keep focus
            // to avoid jarring blur/focus cycles. User can Tab to move focus.
        }
    }

    // Document click handling using LayoutMap + coordinate math
    // zone.mark is NOT used for documents inside viewport (clipping destroys markers)
    if (s.mode === MODE_LIST && s.documents.length > 0 && s.layoutMap && s.vp) {
        // Calculate document-relative Y coordinate
        // msg.y is terminal-relative, we need to adjust for:
        // 1. Header height (title + status = 2 lines typically, dependent on logic)
        // 2. Viewport scroll offset
        // 3. Viewport internal padding (1 line) + Count line (1 line)

        // Re-calculate header height exactly as in renderList to get precise top
        // FIX: Let's store headerHeight in state.
        const headerHeight = s.headerHeight || 2; // Default to 2 if not set

        const clickY = msg.y;
        const vpTop = headerHeight;
        const vpHeight = s.vp.height();
        const vpBottom = vpTop + vpHeight;

        if (clickY >= vpTop && clickY < vpBottom) {
            // Ignore clicks on the right-side scrollbar column.
            if (s.vpContentWidth > 0 && msg.x >= s.vpContentWidth) {
                return [s, null];
            }
            // Convert to content-space coordinates
            // Subtract Header Top
            const viewportRelativeY = clickY - vpTop;
            const contentY = viewportRelativeY + s.vp.yOffset();

            // Find which document was clicked and where within it
            // findClickedDocument handles the internal padding/count offset logic
            const clickResult = findClickedDocument(s, contentY);
            if (clickResult !== null) {
                // Use pre-computed structural line offsets from layoutMap
                const entry = s.layoutMap[clickResult.index];

                // Use the pre-computed offsets that account for wrapped content
                const removeButtonLineOffset = entry.removeButtonLineOffset;
                const bottomBorderLineOffset = entry.bottomBorderLineOffset;
                const headerHeight = entry.headerHeight;

                // If the click landed on the bottom border or the margin below the
                // document, treat it as a no-op. Previously these clicks fell into
                // the "other lines" case and opened edit mode for the previous
                // document when the user clicked the gap between documents.
                if (clickResult.relativeLineInDoc >= bottomBorderLineOffset) {
                    // Do not change selection or trigger actions for gap clicks
                    return [s, null];
                }

                // Click is within the actual document content area; make it selected
                s.selectedIdx = clickResult.index;
                state.set(stateKeys.selectedIndex, clickResult.index);

                // Mapped Action Targets:
                // Line 0: Top border - no action (falls through to edit)
                // Lines 1 to 1+headerHeight-1: Header -> Rename
                // Lines after header and before removeButton: Preview -> Edit
                // removeButtonLineOffset: Remove button -> Delete
                const headerEndLine = 1 + headerHeight; // First line after header ends

                if (clickResult.relativeLineInDoc >= 1 && clickResult.relativeLineInDoc < headerEndLine) {
                    // Header area -> Rename (use 'R' uppercase since 'r' is Reset)
                    return handleKeys({key: 'R'}, s);
                } else if (clickResult.relativeLineInDoc === removeButtonLineOffset) {
                    // Remove button line -> Delete
                    return handleKeys({key: 'd'}, s);
                } else if (clickResult.relativeLineInDoc > 0 && clickResult.relativeLineInDoc < removeButtonLineOffset) {
                    // Preview or other content area -> Edit Content
                    return handleKeys({key: 'e'}, s);
                } else {
                    // Top border (line 0) - just select, no action
                    return [s, null];
                }
            }
        }
    }

    return [s, null];
}

function renderView(s) {
    s.layout = {buttons: [], docBoxes: []}; // Reset layout
    let content;
    if (s.mode === MODE_INPUT) content = renderInput(s);
    else if (s.mode === MODE_CONFIRM) content = renderConfirm(s); else content = renderList(s);

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

    // Status Section (Created early for header placement)
    let statusSection = '';
    if (s.statusMsg) {
        const statusStyle = s.hasError ? styles.error() : styles.status();
        const statusIcon = s.hasError ? '✗ ' : '✓ ';
        statusSection = statusStyle.render(statusIcon + s.statusMsg);
    }

    // Header Construction
    const titleLine = titleStyle.render('📄 Super-Document Builder');

    // Calculate available width for header layout to decide between Wide (Side-by-Side) or Narrow (Stacked)
    // Structure SCENARIO A: Title [Spacer] Status
    // Structure SCENARIO B: Title \n Status
    let topHeaderLine;
    const titleWidth = lipgloss.width(titleLine);
    const statusWidth = lipgloss.width(statusSection);
    // Reserve at least 2 spaces gap
    const gap = termWidth - titleWidth - statusWidth;

    if (gap >= 2) {
        // SCENARIO A: Wide Terminal
        // Use a filler to push status to the right
        const spacer = lipgloss.newStyle().width(gap).render('');
        topHeaderLine = lipgloss.joinHorizontal(lipgloss.Top, titleLine, spacer, statusSection);
    } else {
        // SCENARIO B: Narrow Terminal
        topHeaderLine = lipgloss.joinVertical(lipgloss.Left, titleLine, statusSection);
    }

    // Header contains Title + Status only now.
    // Document count moved to viewport per requirement.
    const header = topHeaderLine;

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
            const docContent = lipgloss.joinVertical(lipgloss.Left, docHeader, docPreview, removeBtn);

            // Apply selection style (double border for selected)
            // Only show as selected if button focus is not active
            const isVisuallySelected = isSel && s.focusedButtonIdx < 0;
            const style = isVisuallySelected ? styles.documentSelected() : styles.document();
            // Use explicitly calculated content width to prevent box clipping
            const renderedDoc = style.width(docContentWidth).render(docContent);

            docItems.push(renderedDoc);
        });
        docSection = lipgloss.joinVertical(lipgloss.Left, ...docItems);
    }

    // Button bar section with responsive layout
    // Per ASCII design: [A]dd  [L]oad  [C]opy  [S]hell  [R]eset  [Q]uit
    const buttonList = BUTTONS.map((btn, idx) => {
        let style;
        const isFocused = s.focusedButtonIdx === idx;
        if (isFocused) {
            style = styles.buttonFocused();
        } else if (btn.key === 'r') {
            style = styles.buttonDanger();
        } else if (btn.key === 's') {
            style = styles.buttonShell();
        } else if (btn.key === 'c') {
            style = styles.buttonFocused();
        } else {
            style = styles.button();
        }
        return {id: btn.id, text: btn.label, style: style};
    });

    const renderedButtons = buttonList.map(b => zone.mark(b.id, b.style.render(b.text)));
    const totalButtonWidth = renderedButtons.reduce((sum, btn) => sum + lipgloss.width(btn), 0);

    let buttonSection;
    const availWidth = termWidth - 4; // Leave some margin

    if (totalButtonWidth > availWidth) {
        // SCENARIO B: Narrow terminal - use 2-column grid layout per ASCII design:
        // |  (  [A]dd  )  ( [L]oad  )  |
        // |  ( [C]opy  )  ( [S]hell )  |
        // |  ( [R]eset )  ( [Q]uit  )  |
        const rows = [];
        for (let i = 0; i < renderedButtons.length; i += 2) {
            const btn1 = renderedButtons[i];
            const btn2 = renderedButtons[i + 1] || ''; // Handle odd number of buttons
            const row = lipgloss.joinHorizontal(lipgloss.Top, btn1, btn2);
            rows.push(row);
        }
        buttonSection = lipgloss.joinVertical(lipgloss.Left, ...rows);
    } else {
        // SCENARIO A: Wide terminal - horizontal layout
        buttonSection = lipgloss.joinHorizontal(lipgloss.Top, ...renderedButtons);
    }

    // Footer section
    const separatorWidth = Math.min(72, termWidth - 2);
    const separator = styles.separator().render('─'.repeat(separatorWidth));
    const helpText = styles.help().render('a:add l:load e:edit d:del c:copy s:shell r:reset q:quit ↑↓:nav');
    const footer = lipgloss.joinVertical(lipgloss.Left, separator, helpText);

    // ------------------------------------------------------------------------
    // DYNAMIC VIEWPORT HEIGHT CALCULATION
    // ------------------------------------------------------------------------
    const headerHeight = lipgloss.height(header);
    s.headerHeight = headerHeight; // Store for mouse hit testing
    const footerHeight = lipgloss.height(footer);
    const spacerHeight = 0;

    const fixedHeight = headerHeight + footerHeight + spacerHeight;
    const vpHeight = Math.max(0, termHeight - fixedHeight);

    // Build scrollable content:
    // 1. Top Padding (1 line)
    // 2. Count Line
    // 3. Documents
    // 4. Buttons
    // 5. Bottom Padding (1 line)
    const docsLine = normalStyle.render(`Documents: ${s.documents.length}`);
    const paddingLine = " "; // Blank line

    // Join elements into the scrollable content
    const scrollableContent = lipgloss.joinVertical(
        lipgloss.Left,
        paddingLine,
        docsLine,
        docSection,
        "", // Gap
        buttonSection,
        paddingLine
    );

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
            s.listScrollbar.setThumbForeground(COLORS.primary);
            s.listScrollbar.setTrackForeground(COLORS.muted);
        }

        const sbView = s.listScrollbar ? s.listScrollbar.view() : "";
        visibleSection = lipgloss.joinHorizontal(lipgloss.Top, vpView, sbView);
    }

    // Compose final view
    return lipgloss.joinVertical(lipgloss.Left, header, visibleSection, footer);
}

function renderInput(s) {
    const termWidth = s.width || 80;
    const termHeight = s.height || 24;
    const scrollbarWidth = 1;

    // Title based on operation
    let titleText;
    if (s.inputOperation === INPUT_ADD) titleText = '📝 Add Document'; else if (s.inputOperation === INPUT_EDIT) titleText = '📝 Edit Document'; else if (s.inputOperation === INPUT_RENAME) titleText = '📝 Rename Document'; else titleText = '📂 Load File';

    const titleStyle = styles.title();
    const title = titleStyle.render(titleText);

    // Jump Icons
    const btnTop = zone.mark('btn-top', styles.jumpIcon().render('[ ↑ ]'));
    const btnBot = zone.mark('btn-bottom', styles.jumpIcon().render('[ ↓ ]'));
    const spacer = lipgloss.newStyle().width(Math.max(2, termWidth - lipgloss.width(title) - lipgloss.width(btnTop) - lipgloss.width(btnBot) - 4)).render('');
    const headerRow = lipgloss.joinHorizontal(lipgloss.Top, title, spacer, btnTop, btnBot);

    // Label/Path field
    const lblLabel = s.inputOperation === INPUT_LOAD ? 'File path:' : 'Label (optional):';
    const lblStyle = s.inputFocus === FOCUS_LABEL ? styles.inputFocused() : styles.inputNormal();
    const lblContent = s.labelBuffer + (s.inputFocus === FOCUS_LABEL ? '▌' : '');
    const lblFieldRendered = lipgloss.joinVertical(lipgloss.Left, styles.label().render(lblLabel), lblStyle.width(Math.max(40, termWidth - 10)).render(lblContent));
    const lblField = zone.mark('input-label', lblFieldRendered);

    // Content field
    let contentField = '';
    // Calculate heights for cursor tracking offset
    let preContentHeight = lipgloss.height(lblFieldRendered) + 1; // +1 gap

    if (s.inputOperation !== INPUT_LOAD && s.inputOperation !== INPUT_RENAME && s.contentTextarea) {
        const cntStyle = s.inputFocus === FOCUS_CONTENT ? styles.inputFocused() : styles.inputNormal();

        // Update textarea dimensions
        const fieldWidth = Math.max(40, termWidth - 10);
        const innerWidth = Math.max(10, fieldWidth - 4 - scrollbarWidth); // border(2) + padding(2)
        s.contentTextarea.setWidth(innerWidth);

        // CRITICAL FIX: Use visualLineCount() instead of lineCount() for height calculation.
        // This accounts for soft-wrapped lines and fixes the viewport clipping bug where
        // the bottom of wrapped documents was invisible.
        const visualLines = Math.max(1, s.contentTextarea.visualLineCount ? s.contentTextarea.visualLineCount() : s.contentTextarea.lineCount());
        // Add buffer for cursor and end-of-buffer character
        s.contentTextarea.setHeight(visualLines + 1);

        const textareaView = s.contentTextarea.view();

        const contentLabel = styles.label().render('Content (multi-line):');
        const contentFieldRendered = lipgloss.joinVertical(lipgloss.Left, contentLabel, cntStyle.width(fieldWidth).render(textareaView));
        contentField = zone.mark('input-content', contentFieldRendered);

        // Update pre-content height for cursor calculation
        preContentHeight += lipgloss.height(contentLabel) + 1; // border top

        // CALCULATE TEXTAREA BOUNDS FOR MOUSE CLICK POSITIONING
        // These bounds are used in handleMouse to translate click coordinates
        // to textarea row/column positions.
        //
        // Screen layout (Y axis):
        //   titleHeight (1 line for header row)
        //   [scrollable content starts here - this is viewport content]
        //     lblField (label field with border)
        //     gap (1 empty line)
        //     contentLabel ("Content (multi-line):" - 1 line)
        //     border top (1 line from cntStyle border)
        //     [TEXTAREA CONTENT STARTS HERE]
        //
        // X axis:
        //   border left (1 char from cntStyle border)
        //   padding left (1 char from cntStyle padding)
        //   prompt (from textarea, e.g., "▌ ")
        //   line numbers (if enabled, e.g., " 1 ")
        //   [TEXTAREA TEXT CONTENT STARTS HERE]
        //
        // Line numbers: textarea uses ShowLineNumbers=true, which adds " N " format
        // The line number width is dynamic based on total lines, but typically 4 chars

        const titleHeight = 1; // Header row
        const lblFieldHeight = lipgloss.height(lblFieldRendered);
        const gapHeight = 1;
        const contentLabelHeight = lipgloss.height(contentLabel);
        const borderTop = 1;

        // Calculate Y offset to first textarea content line (relative to viewport content)
        const textareaContentStartY = lblFieldHeight + gapHeight + contentLabelHeight + borderTop;

        // Calculate X offset to first textarea text character
        // Border (1) + Padding (1) = 2 from the left edge of the field
        const borderLeft = 1;
        const paddingLeft = 1;
        // CRITICAL FIX: Use reservedInnerWidth() from Go instead of hardcoding.
        // This returns the total inner reserved width = promptWidth + lineNumberWidth
        // This is dynamically calculated by bubbles/textarea based on ShowLineNumbers.
        // Previously this was hardcoded as promptWidth=2 + lineNumberWidth=4 = 6
        const reservedInner = s.contentTextarea.reservedInnerWidth ? s.contentTextarea.reservedInnerWidth() : 6;

        // Store bounds in state for handleMouse to use
        // These bounds are critical for proper mouse click handling, especially
        // for large documents where zone-based detection fails.
        s.textareaBounds = {
            // Offset from start of scrollable content to first content row
            contentTop: textareaContentStartY,
            // Offset from left edge of screen to first text character
            // borderLeft + paddingLeft + reservedInner (prompt + line numbers)
            contentLeft: borderLeft + paddingLeft + reservedInner,
            // Store the reserved inner width from Go (prompt + line numbers)
            reservedInnerWidth: reservedInner,
            // Store additional info for precise calculations
            fieldWidth: fieldWidth,
            innerWidth: innerWidth,
            // Store content height for hit detection (use visual lines for proper soft-wrap handling)
            contentHeight: visualLines + 1,
            // Store the border widths for X-axis hit detection
            borderLeft: borderLeft,
            paddingLeft: paddingLeft,
        };
    }

    // Buttons
    const submitBtn = zone.mark('btn-submit', (s.inputFocus === FOCUS_SUBMIT ? styles.buttonFocused() : styles.button()).render('[Submit]'));
    const cancelBtn = zone.mark('btn-cancel', (s.inputFocus === FOCUS_CANCEL ? styles.buttonDanger() : styles.button()).render('[Cancel]'));
    const buttonRow = lipgloss.joinHorizontal(lipgloss.Top, submitBtn, cancelBtn);

    // Footer - Fixed layout
    const sep = styles.separator().render('─'.repeat(termWidth));
    // Dynamic footer text based on operation
    let fText = 'Tab: Cycle Focus    Enter: Newline/Submit    Esc: Cancel';
    if (s.inputOperation === INPUT_EDIT || s.inputOperation === INPUT_ADD) {
        fText += '    PgUp/PgDn: Scroll';
    }
    const helpText = styles.help().render(fText);
    const footer = lipgloss.joinVertical(lipgloss.Left, sep, helpText);
    const footerHeight = lipgloss.height(footer);

    // Build scrollable content for the OUTER viewport
    const scrollableSections = [lblField];
    if (contentField) {
        scrollableSections.push('', contentField);
    }
    scrollableSections.push('', buttonRow);
    const scrollableContent = lipgloss.joinVertical(lipgloss.Left, ...scrollableSections);

    // Calculate available height for the scrollable area
    const titleHeight = lipgloss.height(headerRow);
    const fixedHeight = titleHeight + footerHeight + 1; // 1 for gaps
    const scrollableHeight = Math.max(3, termHeight - fixedHeight);

    const scrollableContentHeight = lipgloss.height(scrollableContent);

    // OUTER Viewport + Scrollbar logic
    let visibleContent;
    // Always use inputVp logic to support scrolling
    s.inputVp.setWidth(termWidth - scrollbarWidth);
    s.inputVp.setHeight(scrollableHeight);
    s.inputVp.setContent(scrollableContent);

    // GO-NATIVE VIEWPORT CONTEXT SETUP
    // Configure the textarea with the current viewport state so it can do
    // ALL coordinate calculations in Go (for handleClickAtScreenCoords and getScrollSyncInfo).
    if (s.contentTextarea && s.contentTextarea.setViewportContext && s.textareaBounds) {
        s.contentTextarea.setViewportContext({
            outerYOffset: s.inputVp.yOffset(),
            textareaContentTop: s.textareaBounds.contentTop,
            textareaContentLeft: s.textareaBounds.contentLeft,
            outerViewportHeight: scrollableHeight,
            preContentHeight: preContentHeight
        });
    }

    // CURSOR VISIBILITY LOGIC
    // Only auto-scroll to keep cursor visible if viewport is NOT unlocked.
    // When unlocked (user is scrolling freely), don't interfere with their scroll position.
    // The unlock flag is reset when user types, so the view will snap back to cursor on input.
    if (s.contentTextarea && s.inputFocus === FOCUS_CONTENT && !s.inputViewportUnlocked) {
        // GO-NATIVE SCROLL SYNC: Use getScrollSyncInfo() to get all sync data in ONE call.
        // This is more performant than calling cursorVisualLine(), line(), etc. separately.
        if (s.contentTextarea.getScrollSyncInfo) {
            const syncInfo = s.contentTextarea.getScrollSyncInfo();
            // cursorAbsY is already calculated in Go (preContentHeight + cursorVisualLine)
            const cursorAbsY = syncInfo.cursorAbsY;
            const vpY = s.inputVp.yOffset();
            const vpH = s.inputVp.height();

            // Scroll if out of bounds
            if (cursorAbsY < vpY) {
                s.inputVp.setYOffset(cursorAbsY);
            } else if (cursorAbsY >= vpY + vpH) {
                s.inputVp.setYOffset(cursorAbsY - vpH + 1);
            }
        } else {
            // Fallback: Use legacy separate method calls
            const cursorVisualLineIdx = s.contentTextarea.cursorVisualLine ?
                s.contentTextarea.cursorVisualLine() : s.contentTextarea.line();
            const cursorAbsY = preContentHeight + cursorVisualLineIdx;
            const vpY = s.inputVp.yOffset();
            const vpH = s.inputVp.height();

            if (cursorAbsY < vpY) {
                s.inputVp.setYOffset(cursorAbsY);
            } else if (cursorAbsY >= vpY + vpH) {
                s.inputVp.setYOffset(cursorAbsY - vpH + 1);
            }
        }
    }

    const vpView = s.inputVp.view();

    // Show OUTER scrollbar if content exceeds height OR if configured
    // Sync outer scrollbar
    if (s.inputScrollbar) {
        s.inputScrollbar.setViewportHeight(scrollableHeight);
        s.inputScrollbar.setContentHeight(scrollableContentHeight);
        s.inputScrollbar.setYOffset(s.inputVp.yOffset());
        s.inputScrollbar.setChars("█", "░");
        s.inputScrollbar.setThumbForeground(COLORS.primary);
        s.inputScrollbar.setTrackForeground(COLORS.muted);
        visibleContent = lipgloss.joinHorizontal(lipgloss.Top, vpView, s.inputScrollbar.view());
    } else {
        visibleContent = vpView;
    }

    // Compose final view
    return lipgloss.joinVertical(lipgloss.Left, headerRow, '', visibleContent, footer);
}

function renderConfirm(s) {
    const title = styles.title().render('⚠️ Confirm Delete');
    const prompt = s.confirmPrompt;

    // Buttons with zone markers
    const yesBtn = zone.mark('btn-yes', styles.buttonDanger().render('[Y]es'));
    const noBtn = zone.mark('btn-no', styles.button().render('[N]o'));
    const buttonRow = lipgloss.joinHorizontal(lipgloss.Top, yesBtn, noBtn);

    const helpText = styles.help().render('y: Confirm    n/Esc: Cancel');

    return lipgloss.joinVertical(lipgloss.Left, title, '', prompt, '', buttonRow, '', helpText);
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

        // Extend the base 'list' command to include the documents
        list: {
            ...ctxmgr.commands.list, description: "List super-documents and context items", handler: function (args) {
                const docs = getDocuments();

                // Show documents first - our core, non-standard context.
                if (docs.length > 0) {
                    output.print("Documents:");
                    docs.forEach(d => {
                        const prev = (d.content || "").substring(0, 50).replace(/\n/g, " ");
                        output.print(`  #${d.id} [${d.label}]: ${prev}${d.content && d.content.length > 50 ? '...' : ''}`);
                    });
                    output.print(""); // blank line separator
                }

                // Delegate to base list command for context items (files, diffs, notes)
                // This shows the critical IDs that were missing per review.md
                ctxmgr.commands.list.handler(args);

                // If both are empty, show helpful message
                if (docs.length === 0) {
                    const ctxItems = state.get(shared.contextItems) || [];
                    if (ctxItems.length === 0) {
                        output.print("No documents or context items. Use 'doc-add' or 'add' to add content.");
                    }
                }
            }
        },

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
        }, "doc-rm": {
            description: "Remove a document", usage: "doc-rm <id>", handler: function (args) {
                if (!args[0]) {
                    output.print("Usage: doc-rm <id>");
                    return;
                }
                const doc = removeDocumentById(parseInt(args[0]));
                if (doc) output.print(`Removed document #${doc.id}`); else output.print("Document not found");
            }
        }, "doc-view": {
            description: "View a specific document content", usage: "doc-view <id>", handler: function (args) {
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
        }, "doc-clear": {
            description: "Clear all documents", handler: function () {
                setDocuments([]);
                output.print("All documents cleared.");
            }
        }, "copy": {
            description: "Copy the final prompt to clipboard", handler: function () {
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
        }, "tui": {
            description: "Open the Visual TUI interface", handler: function () {
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
    name: COMMAND_NAME, tui: {
        title: "Super Document", prompt: `(${COMMAND_NAME}) > `,
    }, onEnter: function () {
        output.print("Super Document Mode. Type 'tui' for visual interface, 'help' for commands.");
    }, commands: function () {
        return buildCommands();
    }, // If shellMode is true (--shell flag passed), don't auto-launch TUI
    // Otherwise, launch TUI immediately for visual interface
    initialCommand: config.shellMode ? undefined : "tui",
});

tui.switchMode(COMMAND_NAME);
