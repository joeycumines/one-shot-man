# PR-Split UI Componentisation Analysis

## Current State

The `pr-split` TUI is implemented as ~9,800 lines of JavaScript (30 chunked files) running through Goja + BubbleTea. There is exactly **one** Go-backed UI component: the **scrollbar** (`internal/termui/scrollbar/` + `internal/builtin/termui/scrollbar/`). Everything else is inline JavaScript rendering logic — style factories, layout math, focus management, zone hit-testing, and string composition — all written directly in the chunk files.

The existing scrollbar pattern is:

| Layer | Location | Role |
|---|---|---|
| **Core Go** | `internal/termui/scrollbar/scrollbar.go` | Model, constructor (functional options), `View()` |
| **JS Bindings** | `internal/builtin/termui/scrollbar/scrollbar.go` | `Require()` → Goja object with setters/getters/`view()` |
| **Registration** | `internal/builtin/register.go:173` | `registry.RegisterNativeModule("osm:termui/scrollbar", ...)` |
| **JS Usage** | `pr_split_15a_tui_styles.js:14` | `require('osm:termui/scrollbar')` |

Every other UI element follows the same *inline JS* pattern: factory functions produce `lipgloss.Style` objects, rendering functions compose strings, and state lives in the monolithic model object (`pr_split_16f_tui_model.js`, ~80 fields).

## Selection Criteria

A component is a good candidate for Go+JS componentisation when it meets **most** of these:

1. **Has encapsulated state** — not just pure rendering; has internal model (offsets, selections, dimensions, timers)
2. **Is used in multiple places** — duplicated or near-duplicated rendering logic across screens
3. **Has non-trivial rendering math** — layout calculations that are error-prone in JS
4. **Would benefit from Go-side testing** — edge cases, golden-file rendering tests, fuzzing
5. **Has a clear API surface** — well-defined inputs (setters) and outputs (View string)
6. **Is likely to be reused** — by other commands, scripts, or future BubbleTea v2 migration

## Top 5 Candidates

---

### 1. Terminal Pane (Bordered Terminal Content with Focus State)

**Current location:** `pr_split_15b_tui_chrome.js` — four pane renderers sharing a common skeleton but diverging in details:
- `renderClaudePane()` (lines 388-502, ~115 lines)
- `renderOutputPane()` (lines 606-687, ~82 lines)
- `renderVerifyPane()` (lines 689-784, ~96 lines)
- `renderShellPane()` (lines 788-843, ~56 lines)

**Total:** ~350 lines across 4 renderers. They share ~40% structural similarity (height budget, scroll offset, border wrapping) but diverge significantly in logic:
- **Claude pane**: ANSI detection, dual content source (screen vs. screenshot), mux fallback, mode tags (`[plain]`), input indicator
- **Output pane**: Simplest — no ANSI detection, no session check, read-only
- **Verify pane**: Dynamic border colors (warning/success based on running/paused), branch labels, elapsed time formatting, pause/input tags, trailing ANSI-only line trimming
- **Shell pane**: Most different — no scroll offset math (uses simple slice), worktree path display, uses `lipgloss.truncate()` instead of `maxWidth()`, completely different placeholder

**Common skeleton across all four:**
1. Calculate content budget (height - borders - title line)
2. Parse content into lines (ANSI-styled or plain text fallback)
3. Calculate visible window from scroll offset
4. Truncate each line to width with `lipgloss.maxWidth()` or equivalent
5. Pad to fill allocated height
6. Wrap in bordered box with dynamic border color
7. Add title line with metadata tags
8. Add scroll indicator (`[live]` or `[X%]`)
9. Add placeholder when no content

**Why componentise:**
- **Duplicated rendering math:** Scroll offset clamping, line truncation with ANSI awareness, height padding, placeholder centering — all reimplemented 4 times with subtle differences
- **State:** `contentLines[]`, `viewportHeight`, `yOffset`, `totalLines`, `isFocused`, `title`, `borderColor`, `placeholder`
- **ANSI handling:** All four panes need ANSI-aware truncation — currently done ad-hoc per-pane with slightly different approaches
- **Focus state:** Border color changes based on focus — currently a manual `if (focused) ... else ...` in each renderer
- **Scroll indicator:** Each pane computes `[live]` vs `[X%]` independently with slightly different math
- **Go-side testing:** Edge cases (empty content, single line, content shorter than viewport, content exactly viewport height, ANSI-only content, extremely long lines) are currently untested

**Proposed Go API (following scrollbar pattern — functional options, value receiver `View()`):**
```go
type TerminalPane struct {
    Content       []string        // raw content lines
    Title         string          // title bar text
    Placeholder   string          // shown when Content is empty
    ViewportH     int
    ViewportW     int
    YOffset       int
    IsFocused     bool
    BorderColor   lipgloss.Color
    FocusColor    lipgloss.Color
    DefaultColor  lipgloss.Color
    ShowScrollIndicator bool
}
func NewTerminalPane(opts ...Option) TerminalPane
func (m TerminalPane) View() string
```

**Proposed JS API (setters only in bindings layer, matching scrollbar pattern):**
```javascript
var pane = terminalpaneLib.new();
pane.setTitle('Claude [auto]');
pane.setContent(lines);  // array of strings
pane.setViewportSize(w, h);
pane.setFocused(true);
pane.setBorderColors(COLORS.primary, COLORS.border);
pane.setPlaceholder('No Claude session');
pane.scrollToBottom();
var rendered = pane.view();
```

---

### 2. Progress Bar

**Current location:** `pr_split_15a_tui_styles.js:223-231` (`renderProgressBar`)
**Lines of code:** 9 lines

**Actual implementation:**
```javascript
function renderProgressBar(percent, width) {
    var barW = Math.max(10, (width || 40) - 10);
    var filled = Math.round(barW * Math.min(1, Math.max(0, percent)));
    var empty = barW - filled;
    var bar = styles.progressFull().render(repeatStr('\u2588', filled)) +
              styles.progressEmpty().render(repeatStr('\u2591', empty));
    var pctStr = Math.round(percent * 100) + '%';
    return bar + '  ' + pctStr;
}
```

**Used in 4 production call sites** (all in `pr_split_15c_tui_screens.js`):
- Line 214 — Analysis screen (step progress)
- Line 599 — Execution screen (per-branch creation progress)
- Line 893 — Execution screen (verification progress)
- Line 922 — Equivalence check screen

**Why componentise:**
- **State:** `percent` (0-1), `width`, with existing clamping via `Math.min(1, Math.max(0, percent))`
- **Enhancement potential:** Currently uses only full block characters (█/░). A Go component could implement 8-level Unicode sub-character precision (▏▎▍▌▋▊▉█) like Charm's bubbles progressbar
- **Currently correct but limited:** The JS implementation already handles edge cases (clamping, min width). A Go component would add: percentage label positioning options, pulse animation frames for indeterminate state, gradient fills
- **Smallest surface area:** Easiest component to implement first, proves the Go+JS pipeline

**Proposed Go API:**
```go
type ProgressBar struct {
    Percent     float64       // 0.0 - 1.0, clamped
    Width       int           // total bar width (includes label space)
    FilledChar  string        // default "█"
    EmptyChar   string        // default "░"
    FilledStyle lipgloss.Style
    EmptyStyle  lipgloss.Style
    ShowPercent bool          // append "XX%" suffix
}
func NewProgressBar(opts ...Option) ProgressBar
func (m ProgressBar) View() string
```

**Proposed JS API:**
```javascript
var pb = progressbarLib.new();
pb.setPercent(0.65);
pb.setWidth(40);
pb.setFilledForeground(COLORS.success);
pb.setEmptyForeground(COLORS.border);
var rendered = pb.view();  // "████████████████████████████████░░░░░░░░  65%"
```

---

### 3. Split-View Tab Bar

**Current location:** Tab rendering is embedded within each pane renderer in `pr_split_15b_tui_chrome.js`. Mouse handling for tabs is in `pr_split_16f_tui_model.js` (`handleMouseClick`). Focus cycling for tabs is in `pr_split_16a_tui_focus.js`.

**Lines of code:** ~60 lines scattered across 3 files

**Current implementation:** Tab bar is rendered inline within the split-view pane area:
```
┌─────────────────────────────────────┐
│ [Claude] [Output] [Verify] [Shell] │  ← tab bar
├─────────────────────────────────────┤
│                                     │
│         Pane content                │
│                                     │
└─────────────────────────────────────┘
```

**Why componentise:**
- **State:** `tabs[]` (id, label), `activeTab`, `focusIndex`, `width`
- **Focus-aware:** Active tab gets different styling, focused tab gets yet another style — currently handled with manual conditional logic
- **Mouse handling:** Click-to-switch handled in `handleMouseClick()` with manually constructed per-tab zone IDs (`split-tab-claude`, `split-tab-output`, etc.)
- **Keyboard handling:** Ctrl+Tab cycling handled in focus system with separate logic
- **Zone ID fragility:** Tab zone IDs are manually constructed in the renderer and must match exactly in the mouse handler — a classic source of bugs
- **Reusable:** Any future multi-pane command would need this exact same pattern

**Proposed Go API:**
```go
type Tab struct {
    ID    string
    Label string
}

type TabBar struct {
    Tabs          []Tab
    Active        int              // index of active tab
    Focus         int              // index of focused tab (for keyboard nav)
    Width         int
    TabStyle      lipgloss.Style
    ActiveStyle   lipgloss.Style
    FocusedStyle  lipgloss.Style
}
func NewTabBar(tabs []Tab, opts ...Option) TabBar
func (m TabBar) ActiveID() string
func (m TabBar) View() string
```

**Proposed JS API:**
```javascript
var tabBar = tabbarLib.new();
tabBar.addTab('claude', 'Claude');
tabBar.addTab('output', 'Output');
tabBar.addTab('verify', 'Verify');
tabBar.addTab('shell', 'Shell');
tabBar.setActive(0);
tabBar.setFocus(0);
tabBar.setWidth(s.width - 4);
var rendered = tabBar.view();
// Zone IDs auto-generated: "tab-claude", "tab-output", etc.
```

---

### 4. Focus-Aware Button

**Current location:** Used everywhere — every screen's action buttons. Pattern is:
```javascript
var isFocused = (focusedElemId === 'button-id');
var style = isFocused ? styles.focusedButton() : styles.primaryButton();
zone.mark('button-id', style.render('Label'));
```

**Lines of code:** This pattern appears ~40+ times across `pr_split_15c_tui_screens.js`, `pr_split_15d_tui_dialogs.js`, and `pr_split_15b_tui_chrome.js`

**Button variants currently in styles:**
- `primaryButton()` — bold, white-on-purple, padded
- `secondaryButton()` — text-on-surface, rounded border
- `disabledButton()` — muted-on-surface, no border
- `focusedButton()` — bold, white-on-amber
- `focusedSecondaryButton()` — width-stable focus variant
- `focusedErrorBadge()` — width-stable focus for error badges

**Why componentise:**
- **Duplication:** Every button manually computes focus state, selects style, wraps in zone — trivial individually but 40+ instances is a maintenance burden
- **Width-stable focus:** Secondary buttons and error badges need the SAME border in focused/unfocused states to prevent layout shift — this is a subtle requirement that's easy to get wrong manually
- **Zone integration:** Every button must be wrapped in `zone.mark(id, ...)` for mouse hit-testing
- **Focus pointer:** Some buttons get a `▸` prefix when focused (strategy selection, plan review actions)
- **Caveat:** The rendering logic itself is trivial (3 lines). The value is in standardising the focus + zone + style combination and eliminating per-button conditional logic

**Proposed Go API:**
```go
type ButtonVariant int

const (
    VariantPrimary ButtonVariant = iota
    VariantSecondary
    VariantDisabled
)

type Button struct {
    ID          string
    Label       string
    Variant     ButtonVariant
    Focused     bool
    Enabled     bool
    ShowPointer bool         // render "▸ " prefix when focused
    Width       int          // fixed width for alignment
}
func NewButton(id, label string, opts ...Option) Button
func (m Button) View() string
```

**Proposed JS API:**
```javascript
var btn = buttonLib.new('plan-edit', 'Edit Plan');
btn.setVariant('primary');  // or 'secondary', 'disabled'
btn.setFocused(true);
btn.setShowPointer(true);
var rendered = btn.view();
// Auto-registers zone with ID 'btn-plan-edit'
```

---

### 5. Confirmation Dialog / Overlay Card

**Current location:** `pr_split_15d_tui_dialogs.js` — multiple dialog renderers:
- `viewConfirmCancelOverlay()` (lines 349-396, ~48 lines)
- `viewMoveFileDialog()` (lines 469-518, ~50 lines)
- `viewRenameSplitDialog()` (lines 522-562, ~41 lines)
- `viewMergeSplitsDialog()` (lines 566-614, ~49 lines)
- `viewHelpOverlay()` (lines 279-345, ~67 lines)

**Total:** ~255 lines of similar overlay/dialog code

**Common pattern:**
1. Calculate overlay dimensions (centered, max width/height)
2. Build title line (bold, styled)
3. Build content lines (variable)
4. Build action buttons (focus-aware, zone-wrapped)
5. Wrap in bordered card with `styles.activeCard().width(overlayW).render()`
6. Center on screen with `lipgloss.place(w, h, lipgloss.Center, lipgloss.Center, panel, {whitespaceChars})`
7. Zone-scan the final output

**Why componentise:**
- **State:** `title`, `contentLines[]`, `buttons[]` (id, label, variant), `maxWidth`, `maxHeight`, `focusIndex`
- **Layout math:** Centering, dimension clamping, content truncation when overflow
- **Focus management:** Tab/Shift+Tab cycling through buttons within the dialog
- **Reusable:** Any future command needing a confirmation dialog or modal would benefit
- **Caveat:** The `View(screenW, screenH int)` signature differs from the scrollbar's parameterless `View()`. This is because dialogs need screen dimensions for centering. This is a deliberate API difference — the dialog is an overlay component, not an inline one.

**Proposed Go API:**
```go
type DialogButton struct {
    ID      string
    Label   string
    Variant ButtonVariant
}

type Dialog struct {
    Title       string
    Content     []string        // content lines
    Buttons     []DialogButton
    FocusIndex  int
    MaxWidth    int
    MaxHeight   int
    BorderColor lipgloss.Color
}
func NewDialog(title string, opts ...Option) Dialog
func (m Dialog) AddButton(id, label string, opts ...ButtonOption)
func (m Dialog) SetContent(lines []string)
func (m Dialog) View(screenW, screenH int) string  // centered overlay — differs from scrollbar's View()
func (m Dialog) FocusedButtonID() string
```

**Proposed JS API:**
```javascript
var dialog = dialogLib.new('Confirm Cancel');
dialog.setContent([
    'Are you sure you want to cancel?',
    'This will clean up all created branches.',
    ''
]);
dialog.addButton('confirm-yes', 'Yes, Cancel', {variant: 'danger'});
dialog.addButton('confirm-no', 'No, Go Back', {variant: 'secondary'});
dialog.setFocusIndex(1);
var rendered = dialog.view(s.width, s.height);
// Auto-registers zones: "dlg-confirm-yes", "dlg-confirm-no"
```

---

## Summary Matrix

| Rank | Component | Duplication | State | Math | Reuse | Notes |
|------|-----------|-------------|-------|------|-------|-------|
| 1 | **Terminal Pane** | High (350 LOC × 4, ~40% shared skeleton) | High | High (scroll, ANSI, resize) | High | Biggest win, most complex |
| 2 | **Progress Bar** | 4 call sites | Low | Low (currently), Medium (with sub-char blocks) | Medium | Smallest surface, easiest first |
| 3 | **Tab Bar** | ~60 LOC scattered | Medium | Low | High | Zone ID fragility is the key driver |
| 4 | **Focus Button** | 40+ sites (trivial pattern) | Low | Low | Very High | Highest reuse, lowest complexity |
| 5 | **Dialog/Overlay** | ~255 LOC × 5 | Medium | Medium (centering, clamp) | High | `View(w,h)` differs from scrollbar pattern |

## Implementation Notes

### BubbleTea v2 Consideration

The user noted that the pattern will change with BubbleTea v2. This is fine because:

1. **Go components are pure view functions** — they take state and return a string. This is independent of BubbleTea's `Model`/`Init`/`Update`/`View` interface. Whether v2 changes the component interface or not, a `View() string` method remains useful.
2. **JS bindings are an abstraction layer** — the Goja bindings layer can be updated independently of the Go core. When v2 arrives, only the bindings need adjustment.
3. **The scrollbar proves the pattern works** — it already follows this exact structure and will migrate cleanly.
4. **Honest caveat:** The Dialog component's `View(screenW, screenH int)` signature is a deliberate deviation from the scrollbar's parameterless `View()`. This is because overlays need screen dimensions for centering. If BubbleTea v2 introduces a standard overlay component interface, this may need adjustment.

### Recommended Implementation Order

Ordered by **smallest surface area first** (prove the pipeline, then scale up):

1. **Progress Bar** — 9 lines of JS, 4 call sites, proves the Go+JS pipeline with minimal risk
2. **Focus Button** — trivial rendering but 40+ sites; standardises a pattern used everywhere
3. **Tab Bar** — medium complexity, eliminates zone ID fragility
4. **Terminal Pane** — biggest individual component, eliminates 350 lines of duplication
5. **Dialog/Overlay** — most complex, but benefits from patterns established by 1-4

### API Consistency with Scrollbar Pattern

All proposed components follow the scrollbar's established pattern:

| Aspect | Scrollbar | All proposed components |
|--------|-----------|------------------------|
| Constructor | `New(opts ...Option) Model` | `NewXxx(opts ...Option) Xxx` |
| Option type | `type Option func(*Model)` | Same |
| View method | `func (m Model) View() string` | Same (except Dialog: `View(screenW, screenH int)`) |
| Setters | JS bindings layer only | JS bindings layer only |
| Value vs pointer | Value receiver | Value receiver |

**Exception:** The Dialog's `View(screenW, screenH int)` takes parameters because overlays need screen dimensions for centering. This is a deliberate API difference — the Dialog is an overlay component, not an inline one.

### File Structure (per component)

Each component follows the scrollbar's structure:
```
internal/termui/<component>/
    <component>.go          # Core Go implementation
    <component>_test.go     # Unit tests
internal/builtin/termui/<component>/
    <component>.go          # Goja bindings
    <component>_test.go     # Binding tests
```

Registration in `internal/builtin/register.go`:
```go
registry.RegisterNativeModule(prefix+"termui/<component>", componentmod.Require())
```

JS usage:
```javascript
var componentLib = require('osm:termui/<component>');
```

### Components Considered But Not Selected

- **Status Bar** (`renderStatusBar`, 89 lines) — complex but screen-specific, not reusable across commands
- **Step Dots** (`renderStepDots`, 16 lines) — too small, tightly coupled to wizard step model
- **Claude Conversation Overlay** (99 lines) — a specialised dialog variant; would be covered by the Dialog component if it supports scrollable content areas
