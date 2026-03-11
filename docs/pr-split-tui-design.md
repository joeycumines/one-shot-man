# PR-Split TUI: Comprehensive UI/UX Design Specification

> **Status:** Implemented — BubbleTea wizard in pr_split_13_tui.js (~3570 lines), Go-side launcher in pr_split.go, 41+ new unit tests
> **Author:** Takumi (匠)
> **Date:** 2026-03-08
> **Implements:** `osm pr-split` graphical wizard mode
> **Runtime:** osm:bubbletea + osm:lipgloss + osm:bubblezone + osm:bubbles/* + osm:termmux

---

## 1. Product Vision

**PR-Split** is a developer tool that takes a large, unwieldy branch — the kind
AI pair-programming produces every day — and decomposes it into small,
reviewable pull requests. The TUI is the primary interface: a multi-step
**wizard** that guides the developer from raw diff to published PRs.

The tool must make developers **want** to use it. Not because they have to, but
because it makes the painful (splitting AI-generated code into reviewable
chunks) feel effortless and even satisfying. Every pixel of terminal space
must serve this goal.

### Design North Stars

1. **Wizard, not REPL.** Full-screen graphical UI with distinct screens, not a
   command-line prompt with text commands.
2. **Dual input.** Every action reachable via both keyboard shortcuts AND mouse
   clicks. Bubblezone marks every interactive element.
3. **Progressive disclosure.** Each wizard step shows only what's relevant.
   Advanced options are tucked behind expandable panels.
4. **Visual feedback.** Every state change has a visual response within 16ms.
   Progress bars, spinners, color transitions, and status updates.
5. **termmux integration.** The Claude AI conversation runs in a split-pane
   that the user toggles into and out of with Ctrl+]. Notifications
   (bell events) surface as clickable status bar badges.
6. **Responsive.** Adapts gracefully from 80×24 to 200×60 terminals.
7. **Zero AI slop.** Intentional typography, consistent color palette, aligned
   columns, proper Unicode box-drawing. A human cared.

---

## 2. Color Palette & Typography

### Colors

```
Background:  Terminal default (respect user theme)
Primary:     #7C3AED (violet-600) — accent, selected items, active buttons
Secondary:   #6366F1 (indigo-500) — secondary actions, links
Success:     #10B981 (emerald-500) — completed steps, passing checks
Warning:     #F59E0B (amber-500) — attention needed, in-progress
Error:       #EF4444 (red-500) — failures, blocking issues
Muted:       #6B7280 (gray-500) — disabled, secondary text
Surface:     #1F2937 (gray-800) — card backgrounds, panels
Border:      #374151 (gray-700) — borders, dividers
Text:        #F9FAFB (gray-50) — primary text
TextDim:     #9CA3AF (gray-400) — secondary text
```

### Typography Hierarchy

```
H1:  Bold + Primary color + Emoji prefix      "🔀 PR Split Wizard"
H2:  Bold + Text color                        "Plan Overview"
Body: Default                                  Regular content
Dim:  TextDim color + Faint                    Helper text, hints
Code: Monospace (inherent in terminal)         File paths, branch names
Badge: Inverted (bg=color, fg=textOnColor)     Status badges
```

### Borders

```
Active panel:   roundedBorder() + Primary borderForeground
Inactive panel: roundedBorder() + Border borderForeground
Selected card:  doubleBorder() + Primary borderForeground
Normal card:    normalBorder() + Border borderForeground
Error card:     normalBorder() + Error borderForeground
```

---

## 3. Layout Architecture

### Global Chrome (Every Screen)

```
+════════════════════════════════════════════════════════════════════+
│  🔀 PR Split Wizard               Step 2/6: Analysis    ⏱ 1:23  │  ← Title Bar
│  ─────────────────────────────────────────────────────────────── │
│                                                                   │
│                     [ SCREEN CONTENT AREA ]                       │  ← Main Content
│                                                                   │
│  ─────────────────────────────────────────────────────────────── │
│  ← Back     [ Cancel ]     Step: ●●○○○○               Next →    │  ← Navigation Bar
│  ─────────────────────────────────────────────────────────────── │
│  Ctrl+] Toggle Claude │ ? Help │ 🔔 Claude: idle                │  ← Status Bar
+════════════════════════════════════════════════════════════════════+
```

**Title Bar** (2 lines, fixed top):
- Left: Tool name with emoji
- Center: Current step label
- Right: Elapsed time

**Navigation Bar** (2 lines, fixed bottom):
- Left: Back button (if applicable)
- Center: Cancel button + step dots
- Right: Next/action button

**Status Bar** (1 line, fixed bottom):
- Left: termmux toggle hint
- Center: Help shortcut
- Right: Claude status badge (clickable via bubblezone → switches to Claude pane)

### Content Area

The content area between chrome elements uses a **viewport** for scrolling.
Each screen populates it differently. The viewport has:
- Scrollbar (right edge, via osm:termui/scrollbar)
- Mouse wheel scrolling enabled
- PgUp/PgDn, Home/End support

---

## 4. Screen Designs

### Screen 1: Configuration (IDLE → CONFIG)

**Purpose:** Set up the split parameters before analysis begins.

```
+════════════════════════════════════════════════════════════════════════+
│  🔀 PR Split Wizard                      Step 1/6: Configure   ⏱ 0:00 │
│  ──────────────────────────────────────────────────────────────────── │
│                                                                        │
│   Repository: ~/dev/my-project                                         │
│   Current Branch: feature/big-refactor (247 files changed)             │
│                                                                        │
│  ╭──────────────────────────────────────────────────────────────────╮  │
│  │  Source Branch                                                    │  │
│  │  ┌──────────────────────────────────────────────────────────┐    │  │
│  │  │ feature/big-refactor                                     │    │  │
│  │  └──────────────────────────────────────────────────────────┘    │  │
│  │                                                                   │  │
│  │  Target Branch                                                    │  │
│  │  ┌──────────────────────────────────────────────────────────┐    │  │
│  │  │ main                                                     │    │  │
│  │  └──────────────────────────────────────────────────────────┘    │  │
│  │                                                                   │  │
│  │  Strategy          ╭────────────────────────────────────────╮    │  │
│  │  ● Auto (AI)       │ Let Claude analyze and propose the     │    │  │
│  │  ○ Heuristic       │ optimal split. Requires agent infra.   │    │  │
│  │  ○ Manual          ╰────────────────────────────────────────╯    │  │
│  │                                                                   │  │
│  │  ▸ Advanced Options ─────────────────────                        │  │
│  ╰──────────────────────────────────────────────────────────────────╯  │
│                                                                        │
│  ──────────────────────────────────────────────────────────────────── │
│                   [ Cancel ]    ●○○○○○           [ Start Analysis → ] │
│  ──────────────────────────────────────────────────────────────────── │
│  Ctrl+] Toggle Claude │ Tab: Next Field │ Enter: Select │ ? Help      │
+════════════════════════════════════════════════════════════════════════+
```

**Interactions:**
- Tab/Shift+Tab cycle through form fields
- Enter selects radio options
- Click on any field to focus it
- "Advanced Options" expands on click/Enter to reveal:
  - Max files per chunk (numeric input)
  - PR title template (text input)
  - Include verification step (checkbox)
  - Custom grouping rules (textarea)
- [Start Analysis →] is the primary action (highlighted in Primary color)

**Advanced Options Expanded:**

```
│  ▾ Advanced Options ─────────────────────                        │
│    Max files per chunk: [ 15 ]                                   │
│    PR title template:   [ chore(split): {title} ({n}/{total}) ]  │
│    ☑ Run equivalence verification                                │
│    ☑ Auto-create pull requests                                   │
│    ☐ Dry run (preview only)                                      │
```

### Screen 2: Analysis (CONFIG → PLAN_GENERATION)

**Purpose:** Show progress while analyzing the diff and generating a split plan.

```
+════════════════════════════════════════════════════════════════════════+
│  🔀 PR Split Wizard                      Step 2/6: Analysis    ⏱ 0:45 │
│  ──────────────────────────────────────────────────────────────────── │
│                                                                        │
│   Analyzing feature/big-refactor → main                                │
│                                                                        │
│   ████████████████████████████░░░░░░░░░░░░░░  65%  Grouping files     │
│                                                                        │
│  ╭─ Diff Summary ───────────────────────────────────────────────────╮  │
│  │  Files:    247 changed  (182 modified, 41 added, 24 deleted)     │  │
│  │  Lines:    +12,847 / -3,291                                      │  │
│  │  Packages: 14 Go packages, 6 JS modules                         │  │
│  ╰──────────────────────────────────────────────────────────────────╯  │
│                                                                        │
│  ╭─ Analysis Steps ─────────────────────────────────────────────────╮  │
│  │  ✓  Parse diff                                         0.8s     │  │
│  │  ✓  Identify file dependencies                         2.1s     │  │
│  │  ✓  Classify change types                              1.4s     │  │
│  │  ⟳  Group files by logical unit                        ...      │  │
│  │  ○  Generate split plan via Claude                              │  │
│  │  ○  Validate plan constraints                                   │  │
│  ╰──────────────────────────────────────────────────────────────────╯  │
│                                                                        │
│  ╭─ Live Log ──────────────── (scroll: ↑↓ PgUp PgDn) ──────────╮ ▲ │
│  │  [INFO]  Parsed 247 files in 0.8s                             │ █ │
│  │  [INFO]  Found 14 Go packages with cross-references           │ █ │
│  │  [INFO]  Classified: 89 refactor, 67 feature, 51 fix, 40 chore│ ░ │
│  │  [INFO]  Grouping by package + dependency graph...            │ ░ │
│  ╰───────────────────────────────────────────────────────────────╯ ▼ │
│                                                                        │
│  ──────────────────────────────────────────────────────────────────── │
│  ← Back     [ Pause ⏸ ]  [ Cancel ]    ●●○○○○                       │
│  ──────────────────────────────────────────────────────────────────── │
│  Ctrl+] Toggle Claude │ ? Help │ 🔔 Claude: analyzing (65%)          │
+════════════════════════════════════════════════════════════════════════+
```

**Interactions:**
- Log viewport scrolls with mouse wheel, PgUp/PgDn
- [Pause] pauses the pipeline and shows resume option
- Claude status badge is CLICKABLE → toggles to termmux Claude pane
- Progress bar animates in real-time
- Steps show ✓ (done), ⟳ (in-progress spinner), ○ (pending)

**When Claude is working (termmux integration):**

The notification badge in the status bar shows Claude's state:
- `🔔 Claude: analyzing (65%)` — active, with progress
- `🔔 Claude: idle` — waiting
- `🔔 Claude: ⚠ needs input` — attention badge, PULSES (bold+unbold cycle)
- Clicking the badge OR pressing Ctrl+] switches to the Claude terminal pane

### Screen 3: Plan Review (PLAN_GENERATION → PLAN_REVIEW)

**Purpose:** Present the proposed split plan for the user to approve or modify.

```
+════════════════════════════════════════════════════════════════════════+
│  🔀 PR Split Wizard                  Step 3/6: Review Plan     ⏱ 2:15 │
│  ──────────────────────────────────────────────────────────────────── │
│                                                                        │
│  Plan: 6 chunks from 247 files                                         │
│                                                                        │
│  ╭─ Chunk Overview ─────────────────────────────────────────────────╮  │
│  │                                                                   │  │
│  │  ╔══ 1. "Core type refactors" ══════════════════════════════╗    │  │
│  │  ║  📁 34 files  │  +1,892 / -445  │  deps: none            ║    │  │
│  │  ║  internal/types/*.go, internal/config/types.go           ║    │  │
│  │  ║  ▸ Show all files                                        ║    │  │
│  │  ╚══════════════════════════════════════════════════════════════╝    │  │
│  │                                                                   │  │
│  │  ┌── 2. "Command infrastructure" ────────────────────────────┐    │  │
│  │  │  📁 28 files  │  +2,104 / -312  │  deps: chunk 1         │    │  │
│  │  │  internal/command/*.go, cmd/osm/main.go                   │    │  │
│  │  │  ▸ Show all files                                         │    │  │
│  │  └───────────────────────────────────────────────────────────────┘    │  │
│  │                                                                   │  │
│  │  ┌── 3. "JavaScript runtime updates" ────────────────────────┐    │  │
│  │  │  📁 67 files  │  +4,231 / -1,102  │  deps: chunks 1,2    │    │  │
│  │  │  internal/scripting/*.go, internal/builtin/**/*.go        │    │  │
│  │  │  ▸ Show all files                                         │    │  │
│  │  └───────────────────────────────────────────────────────────────┘    │  │
│  │                                                                   │  │
│  │  ... (scroll for chunks 4-6) ...                              │ ▲ │  │
│  │                                                                   │ █ │  │
│  ╰───────────────────────────────────────────────────────────────╯ ▼ │  │
│                                                                        │
│  ╭─ Selected: Chunk 1 ─────────────────────────────────────────────╮  │
│  │  Title: Core type refactors                                      │  │
│  │  Files: internal/types/session.go (+45,-12)                      │  │
│  │         internal/types/config.go (+89,-34)                       │  │
│  │         internal/types/command.go (+112,-67)                     │  │
│  │         ... 31 more (▸ expand)                                   │  │
│  │  Rationale: Foundational type changes that other chunks depend   │  │
│  │             on. Must be merged first.                            │  │
│  ╰──────────────────────────────────────────────────────────────────╯  │
│                                                                        │
│  ──────────────────────────────────────────────────────────────────── │
│  ← Back  [ Edit Plan ✏ ] [ Regenerate 🔄 ] [ Cancel ]  ●●●○○○  [ Accept → ] │
│  ──────────────────────────────────────────────────────────────────── │
│  ↑↓: Select Chunk │ Enter: Expand │ m: Move File │ Ctrl+] Claude │ ? │
+════════════════════════════════════════════════════════════════════════+
```

**Interactions:**
- ↑↓ or click to select a chunk (highlights with doubleBorder)
- Enter or click "▸ Show all files" to expand file list
- `m` key to move selected file to a different chunk (opens move dialog)
- `e` or [Edit Plan ✏] to enter the Plan Editor
- [Accept →] proceeds to execution
- [Regenerate 🔄] sends back to Claude with optional feedback
- Chunk cards are bubblezone-marked for click selection

**Move File Dialog (overlay):**

```
╭────────────────────────────────────────╮
│  Move: internal/types/session.go       │
│                                        │
│  From: Chunk 1 (Core type refactors)   │
│  To:                                   │
│    ○ Chunk 2 (Command infrastructure)  │
│    ○ Chunk 3 (JS runtime updates)      │
│    ○ Chunk 4 (Test infrastructure)     │
│    ○ Chunk 5 (Documentation)           │
│    ○ Chunk 6 (Build & CI)             │
│    ○ New chunk...                      │
│                                        │
│  [ Cancel ]              [ Move → ]    │
╰────────────────────────────────────────╯
```

### Screen 4: Plan Editor (PLAN_REVIEW → PLAN_EDITOR)

**Purpose:** Edit chunk titles, descriptions, and file assignments in detail.

```
+════════════════════════════════════════════════════════════════════════+
│  🔀 PR Split Wizard                    Step 3/6: Edit Plan     ⏱ 3:02 │
│  ──────────────────────────────────────────────────────────────────── │
│                                                                        │
│  Editing Chunk 1 of 6                                                  │
│                                                                        │
│  Title:                                                                │
│  ┌────────────────────────────────────────────────────────────────┐   │
│  │ Core type refactors                                            │   │
│  └────────────────────────────────────────────────────────────────┘   │
│                                                                        │
│  Description:                                                          │
│  ┌────────────────────────────────────────────────────────────────┐   │
│  │ Foundational type changes that other chunks depend on. Includes│   │
│  │ session types, config types, and command interfaces.           │   │
│  │                                                                │   │
│  │                                                                │   │
│  └────────────────────────────────────────────────────────────────┘   │
│                                                                        │
│  Files (34):                                                           │
│  ┌─────────────────────────────────────────────────────────────┐  ▲  │
│  │  ☑ internal/types/session.go          +45  -12             │  █  │
│  │  ☑ internal/types/config.go           +89  -34             │  █  │
│  │  ☑ internal/types/command.go          +112 -67             │  ░  │
│  │  ☑ internal/config/types.go           +23  -8              │  ░  │
│  │  ☐ internal/config/loader.go          +56  -23  ← uncheck │  ░  │
│  │  ...                                                        │  ░  │
│  └─────────────────────────────────────────────────────────────┘  ▼  │
│                                                                        │
│  ──────────────────────────────────────────────────────────────────── │
│  [ ← Prev Chunk ]  [ Cancel ]  ●●●○○○  [ Next Chunk → ] [ Save ✓ ]  │
│  ──────────────────────────────────────────────────────────────────── │
│  Tab: Next Field │ Space: Toggle File │ Ctrl+] Claude │ ? Help        │
+════════════════════════════════════════════════════════════════════════+
```

**Interactions:**
- Tab cycles between Title textarea, Description textarea, File list
- In file list: Space toggles file inclusion, ↑↓ selects
- Unchecked files become "unassigned" and show warning on Save
- [← Prev Chunk] / [Next Chunk →] navigate between chunks
- [Save ✓] validates and returns to Plan Review

### Screen 5: Execution (BRANCH_BUILDING)

**Purpose:** Show real-time progress as branches are created and commits applied.

```
+════════════════════════════════════════════════════════════════════════+
│  🔀 PR Split Wizard                  Step 4/6: Executing      ⏱ 4:30 │
│  ──────────────────────────────────────────────────────────────────── │
│                                                                        │
│  Building 6 branches...                                                │
│                                                                        │
│  ╭─ Progress ───────────────────────────────────────────────────────╮  │
│  │                                                                   │  │
│  │  Chunk 1: Core type refactors                                    │  │
│  │  ████████████████████████████████████████████████ 100%  ✓ Done   │  │
│  │  Branch: split/core-type-refactors                               │  │
│  │                                                                   │  │
│  │  Chunk 2: Command infrastructure                                 │  │
│  │  ████████████████████████████████░░░░░░░░░░░░░░░  68%  Building │  │
│  │  Branch: split/command-infrastructure                            │  │
│  │  Applying: internal/command/registry.go (15/28 files)            │  │
│  │                                                                   │  │
│  │  Chunk 3: JavaScript runtime updates                             │  │
│  │  ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░   0%  Queued   │  │
│  │                                                                   │  │
│  │  Chunk 4: Test infrastructure                                    │  │
│  │  ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░   0%  Queued   │  │
│  │                                                                   │  │
│  │  Chunk 5: Documentation                                          │  │
│  │  ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░   0%  Queued   │  │
│  │                                                                   │  │
│  │  Chunk 6: Build & CI                                             │  │
│  │  ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░   0%  Queued   │  │
│  │                                                                   │  │
│  ╰──────────────────────────────────────────────────────────────────╯  │
│                                                                        │
│  ╭─ Activity ───────────────────────────────────────────────────────╮  │
│  │  [12:34:56] Created branch split/core-type-refactors            │  │
│  │  [12:34:57] Applied 34 files to split/core-type-refactors       │  │
│  │  [12:35:02] Created branch split/command-infrastructure         │  │
│  │  [12:35:03] Applying files to split/command-infrastructure...   │  │
│  ╰──────────────────────────────────────────────────────────────────╯  │
│                                                                        │
│  ──────────────────────────────────────────────────────────────────── │
│     [ Pause ⏸ ]  [ Cancel ]    ●●●●○○                               │
│  ──────────────────────────────────────────────────────────────────── │
│  Ctrl+] Toggle Claude │ ? Help │ 🔔 Claude: building chunk 2         │
+════════════════════════════════════════════════════════════════════════+
```

**Interactions:**
- Progress bars update in real-time
- [Pause] suspends after current chunk completes
- Activity log scrolls automatically (viewport with mousewheel)
- Claude badge shows activity, clickable to toggle into Claude terminal
- If error occurs → automatically transitions to Error Resolution screen

**Error Resolution Overlay:**

```
╭─ ⚠ Error: Conflict in Chunk 2 ──────────────────────────────────────╮
│                                                                       │
│  File: internal/command/registry.go                                   │
│  Error: Cherry-pick conflict at line 45                               │
│                                                                       │
│  ╭─ Conflict Diff ──────────────────────────────────────────╮        │
│  │  <<<<<<< HEAD                                             │        │
│  │  func NewRegistry() *Registry {                           │        │
│  │  =======                                                  │        │
│  │  func NewRegistry(opts ...Option) *Registry {             │        │
│  │  >>>>>>> split/command-infrastructure                     │        │
│  ╰───────────────────────────────────────────────────────────╯        │
│                                                                       │
│  [ Ask Claude 🤖 ] [ Skip File ] [ Abort Chunk ] [ Manual Fix ]      │
│                                                                       │
╰───────────────────────────────────────────────────────────────────────╯
```

### Screen 6: Verification (EQUIV_CHECK)

**Purpose:** Confirm that the split accounts for all changes — nothing lost, nothing duplicated.

```
+════════════════════════════════════════════════════════════════════════+
│  🔀 PR Split Wizard                Step 5/6: Verification     ⏱ 6:12 │
│  ──────────────────────────────────────────────────────────────────── │
│                                                                        │
│  Equivalence Check: Verifying all changes are accounted for            │
│                                                                        │
│  ╭─ Results ────────────────────────────────────────────────────────╮  │
│  │                                                                   │  │
│  │  ✅ Total lines:     Match (original: +12,847/-3,291)            │  │
│  │  ✅ File coverage:   247/247 files accounted for                 │  │
│  │  ✅ No duplicates:   0 files appear in multiple chunks           │  │
│  │  ✅ Dependency order: All chunk dependencies are satisfiable     │  │
│  │  ⚠️  Semantic check:  2 warnings (non-blocking)                  │  │
│  │                                                                   │  │
│  ╰──────────────────────────────────────────────────────────────────╯  │
│                                                                        │
│  ╭─ Warnings ──────────────────────────────────────────────────────╮  │
│  │                                                                   │  │
│  │  ⚠ Chunk 3 adds internal/scripting/helpers.go which references   │  │
│  │    types from internal/types/config.go (in chunk 1). Ensure      │  │
│  │    chunk 1 merges first.                                         │  │
│  │                                                                   │  │
│  │  ⚠ Chunk 5 modifies docs/architecture.md which references        │  │
│  │    code paths introduced in chunks 2 and 3.                      │  │
│  │                                                                   │  │
│  ╰──────────────────────────────────────────────────────────────────╯  │
│                                                                        │
│  ──────────────────────────────────────────────────────────────────── │
│  ← Back    [ Re-verify ]  [ Cancel ]    ●●●●●○       [ Continue → ]  │
│  ──────────────────────────────────────────────────────────────────── │
│  Ctrl+] Toggle Claude │ ? Help │ 🔔 Claude: idle                     │
+════════════════════════════════════════════════════════════════════════+
```

**Interactions:**
- Warnings are expandable (click/Enter for details)
- [Re-verify] runs the check again
- [Continue →] proceeds to Finalization
- If critical failures → blocks progression (button disabled + red outline)

### Screen 7: Finalization (FINALIZATION → DONE)

**Purpose:** Summary and PR creation.

```
+════════════════════════════════════════════════════════════════════════+
│  🔀 PR Split Wizard                 Step 6/6: Finalize        ⏱ 7:45 │
│  ──────────────────────────────────────────────────────────────────── │
│                                                                        │
│  ╭─ Summary ───────────────────────────────────────────────────────╮  │
│  │                                                                   │  │
│  │  ✅ Split Complete                                                │  │
│  │                                                                   │  │
│  │  Source:  feature/big-refactor                                    │  │
│  │  Target:  main                                                    │  │
│  │  Chunks:  6 branches created                                      │  │
│  │  Time:    7 minutes 45 seconds                                    │  │
│  │                                                                   │  │
│  ╰──────────────────────────────────────────────────────────────────╯  │
│                                                                        │
│  ╭─ Branches ──────────────────────────────────────────────────────╮  │
│  │                                                                   │  │
│  │  1. split/core-type-refactors         34 files  +1,892 / -445   │  │
│  │  2. split/command-infrastructure      28 files  +2,104 / -312   │  │
│  │  3. split/js-runtime-updates          67 files  +4,231 / -1,102 │  │
│  │  4. split/test-infrastructure         52 files  +2,508 / -890   │  │
│  │  5. split/documentation               38 files  +1,245 / -198   │  │
│  │  6. split/build-and-ci                28 files  +867  / -344    │  │
│  │                                                                   │  │
│  ╰──────────────────────────────────────────────────────────────────╯  │
│                                                                        │
│  ╭─ Actions ───────────────────────────────────────────────────────╮  │
│  │                                                                   │  │
│  │  ( Create All PRs )  ( Export Report )  ( Copy Summary )         │  │
│  │                                                                   │  │
│  ╰──────────────────────────────────────────────────────────────────╯  │
│                                                                        │
│  ──────────────────────────────────────────────────────────────────── │
│           [ Done ✓ ]           ●●●●●●                                │
│  ──────────────────────────────────────────────────────────────────── │
│  Ctrl+] Toggle Claude │ ? Help │ 🔔 Claude: idle                     │
+════════════════════════════════════════════════════════════════════════+
```

**Interactions:**
- (Create All PRs) — creates GitHub PRs for all chunks (uses osm:exec + git)
- (Export Report) — writes split report to clipboard
- (Copy Summary) — copies summary text to clipboard
- [Done ✓] — exits the wizard cleanly

---

## 5. termmux Integration

### Architecture

The pr-split wizard runs as the "OSM side" of a termmux session. The
"Claude side" is the Claude agent process (launched via ClaudeCodeExecutor).

```
┌─────────────────────────┐
│     osm:bubbletea       │  ← Wizard UI (this design)
│     (Model/Update/View) │
│           │              │
│     osm:termmux         │  ← Terminal multiplexer
│      ┌────┴────┐        │
│      │         │         │
│   OSM Pane  Claude Pane  │
│   (wizard)  (agent PTY)  │
└─────────────────────────┘
```

### Toggle Behavior

- **Ctrl+]** toggles between OSM wizard and Claude terminal
- While in Claude pane, the wizard is "frozen" (no updates rendered)
- When returning to OSM pane, the wizard re-renders from current state
- The status bar badge updates from Claude events (bell, output) even while
  the wizard is active (via `mux.on('bell', ...)` and `mux.on('output', ...)`)

### termmux.fromModel Integration

```javascript
const mux = termmux.newMux({
    toggleKey: 0x1D,
    statusEnabled: true,
    initialStatus: "PR Split Wizard"
});

// The wizard model wraps inside termmux
const {model: wrappedModel, options} = mux.fromModel(wizardModel, {
    altScreen: true,
    toggleKey: 0x1D
});

tea.run(wrappedModel, options);
```

### Notification System

When Claude completes an action or needs attention:

1. Claude process writes output → termmux captures via `mux.on('output', ...)`
2. Wizard state updates the badge: `🔔 Claude: done ✓` or `🔔 Claude: ⚠ error`
3. Badge is marked with bubblezone → clicking it calls `mux.switchTo()`
4. Bell events (`mux.on('bell', ...)`) trigger a visible pulse on the badge

---

## 6. State Management

### Wizard State Machine

The wizard uses a finite state machine (FSM) that maps directly to screens:

```
IDLE → CONFIG → PLAN_GENERATION → PLAN_REVIEW → BRANCH_BUILDING → EQUIV_CHECK → FINALIZATION → DONE
                      ↑                  │              ↑                │
                      │           PLAN_EDITOR           │         ERROR_RESOLUTION
                      └──────── (regenerate) ───────────┘
```

**State-to-Screen Mapping:**

| FSM State        | Screen                  | Primary Action               |
|------------------|-------------------------|------------------------------|
| IDLE             | (entry, auto-advance)   | —                            |
| CONFIG           | Screen 1: Configuration | [Start Analysis →]           |
| PLAN_GENERATION  | Screen 2: Analysis      | (auto-advance on completion) |
| PLAN_REVIEW      | Screen 3: Plan Review   | [Accept →]                   |
| PLAN_EDITOR      | Screen 4: Plan Editor   | [Save ✓]                     |
| BRANCH_BUILDING  | Screen 5: Execution     | (auto-advance on completion) |
| ERROR_RESOLUTION | Screen 5: Error Overlay | [Ask Claude] / [Skip]        |
| EQUIV_CHECK      | Screen 6: Verification  | [Continue →]                 |
| FINALIZATION     | Screen 7: Finalization  | [Done ✓]                     |
| DONE             | (exit wizard)           | —                            |
| PAUSED           | Screen 5 + pause badge  | [Resume]                     |
| CANCELLED        | (exit wizard)           | —                            |
| ERROR            | Error overlay           | [Retry] / [Abort]            |

### BubbleTea Model Structure

```javascript
function initModel() {
    return {
        // FSM
        wizardState: "IDLE",
        previousState: null,

        // Window
        width: 80,
        height: 24,

        // Screen-specific state
        config: {
            sourceBranch: "",
            targetBranch: "main",
            strategy: "auto",
            maxFilesPerChunk: 15,
            prTemplate: "chore(split): {title} ({n}/{total})",
            runVerification: true,
            autoCreatePRs: true,
            dryRun: false,
            focusedField: 0,
            showAdvanced: false,
        },

        analysis: {
            progress: 0,
            steps: [],
            diffSummary: null,
            logs: [],
        },

        plan: {
            chunks: [],
            selectedChunk: 0,
            expandedChunk: -1,
        },

        editor: {
            editingChunk: 0,
            titleTextarea: null,    // osm:bubbles/textarea instance
            descTextarea: null,     // osm:bubbles/textarea instance
            fileList: [],
            focusedField: 0,
        },

        execution: {
            chunkProgress: [],      // [{name, percent, status, branch}]
            activityLog: [],
            currentChunk: 0,
            error: null,
        },

        verification: {
            results: [],
            warnings: [],
            passed: false,
        },

        finalization: {
            branches: [],
            prsCreated: [],
            elapsed: 0,
        },

        // Shared
        viewport: null,             // osm:bubbles/viewport instance
        scrollbar: null,            // osm:termui/scrollbar instance
        startTime: Date.now(),
        claudeStatus: "idle",
        claudeNotification: null,
        mux: null,                  // osm:termmux instance
    };
}
```

---

## 7. Keyboard Shortcuts (Global)

| Key        | Action                          | Context           |
|------------|---------------------------------|-------------------|
| Ctrl+]     | Toggle Claude terminal          | Always            |
| Ctrl+C     | Cancel / exit                   | Always            |
| ?          | Show help overlay               | Non-input screens |
| Tab        | Next field / element            | Forms             |
| Shift+Tab  | Previous field / element        | Forms             |
| Enter      | Activate / select               | Buttons, options  |
| Esc        | Close overlay / back            | Overlays          |
| ↑↓         | Navigate items                  | Lists             |
| PgUp/PgDn  | Scroll viewport                 | Scrollable areas  |
| Home/End   | Jump to top/bottom              | Scrollable areas  |
| Space      | Toggle checkbox                 | Checkboxes        |

### Screen-Specific Shortcuts

| Screen     | Key | Action                    |
|------------|-----|---------------------------|
| Plan Review| m   | Move file to chunk        |
| Plan Review| e   | Edit selected chunk       |
| Execution  | p   | Pause/resume              |
| Any        | q   | Quit (with confirmation)  |

---

## 8. Responsive Layout Rules

### Breakpoints

| Width   | Layout                                              |
|---------|-----------------------------------------------------|
| < 60    | **Compact:** Stack all panels vertically, minimal chrome |
| 60-100  | **Standard:** Single-column, full chrome            |
| > 100   | **Wide:** Side-by-side detail panels where applicable |

### Wide Layout Example (Plan Review, >100 cols)

```
+═══════════════════════════════════════════════════════════════════════════════════════+
│  🔀 PR Split Wizard                         Step 3/6: Review Plan            ⏱ 2:15 │
│  ──────────────────────────────────────────────────────────────────────────────────── │
│                                                                                       │
│  ╭─ Chunks ───────────────────────────╮ ╭─ Detail: Chunk 1 ──────────────────────╮  │
│  │                                     │ │                                         │  │
│  │  ▸ 1. Core type refactors  (34)    │ │  Title: Core type refactors             │  │
│  │    2. Command infra        (28)    │ │                                         │  │
│  │    3. JS runtime update    (67)    │ │  Files: 34 files (+1,892 / -445)        │  │
│  │    4. Test infra           (52)    │ │  ─────────────────────────────────────  │  │
│  │    5. Documentation        (38)    │ │  internal/types/session.go  +45  -12    │  │
│  │    6. Build & CI           (28)    │ │  internal/types/config.go   +89  -34    │  │
│  │                                     │ │  internal/types/command.go  +112 -67    │  │
│  │                                     │ │  internal/config/types.go   +23  -8     │  │
│  │                                     │ │  ...                                    │  │
│  │                                     │ │                                         │  │
│  │                                     │ │  Rationale:                             │  │
│  │                                     │ │  Foundational type changes that other   │  │
│  │                                     │ │  chunks depend on. Must merge first.    │  │
│  ╰─────────────────────────────────────╯ ╰─────────────────────────────────────────╯  │
│                                                                                       │
│  ──────────────────────────────────────────────────────────────────────────────────── │
│  ← Back  [ Edit ✏ ] [ Regenerate 🔄 ] [ Cancel ]  ●●●○○○              [ Accept → ]  │
│  ──────────────────────────────────────────────────────────────────────────────────── │
│  ↑↓: Select │ Enter: Expand │ m: Move │ Ctrl+] Claude │ ? Help                       │
+═══════════════════════════════════════════════════════════════════════════════════════+
```

---

## 9. Accessibility & Polish

### Animation

- Step transitions: brief fade via Lipgloss faint toggle (2 frames)
- Progress bars: smooth increment (not jumpy)
- Spinner for "in-progress" steps: Unicode braille pattern `⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏`
- Badge pulse: bold on/off cycle at 500ms via tick commands

### Error States

- Error overlays use `Error` color border
- Blocking errors disable the primary action button (muted color, "─" instead of "→")
- Non-blocking warnings show ⚠️ but don't block progression
- All errors show actionable recovery options — never a dead end

### Empty States

- No chunks yet → "No split plan generated. Press Enter to start analysis."
- No files in chunk → "This chunk is empty. Add files or it will be removed."
- No Claude connection → "Claude unavailable. Using heuristic mode."

### Confirmation Dialogs

```
╭─ Confirm Cancel ────────────────────────────────╮
│                                                   │
│  Are you sure you want to cancel?                 │
│  This will abandon the current split operation.   │
│                                                   │
│  Created branches will be preserved for recovery. │
│                                                   │
│           [ No, Continue ]    [ Yes, Cancel ]     │
│                                                   │
╰───────────────────────────────────────────────────╯
```

---

## 10. Implementation Strategy

### File Structure

The TUI implementation replaces the current `pr_split_13_tui.js` entirely.
It should be structured as:

```
pr_split_13_tui.js      — TUI entry point, model init, run()
                            Uses: osm:bubbletea, osm:termmux
                            Contains: initModel, update (message router), view (screen router)
                            Delegates to screen renderers and state handlers
```

All screen rendering functions and state handlers live in the same file
(matching the existing chunked architecture pattern where chunk 13 = TUI).
The file is expected to be ~800-1200 lines — comparable to
`super_document_script.js`.

### Key Dependencies

```javascript
const tea = require('osm:bubbletea');
const lipgloss = require('osm:lipgloss');
const zone = require('osm:bubblezone');
const textareaLib = require('osm:bubbles/textarea');
const viewportLib = require('osm:bubbles/viewport');
const scrollbarLib = require('osm:termui/scrollbar');
const termmux = require('osm:termmux');
```

### Integration with Existing Chunks

The TUI chunk (13) calls functions from prior chunks via `prSplit.*`:
- `prSplit.analyzeDiff()` (chunk 01) — during analysis screen
- `prSplit.selectStrategy()` (chunk 02) — during config screen
- `prSplit.createSplitPlan()` (chunk 03) — during plan generation
- `prSplit.validateSplitPlan()` (chunk 04) — during plan review
- `prSplit.executeSplit()` (chunk 05) — during execution
- `prSplit.verifyEquivalence()` (chunk 06) — during verification
- `prSplit.createPRs()` (chunk 07) — during finalization
- `prSplit.resolveConflicts()` (chunk 08) — during error resolution
- `prSplit.ClaudeCodeExecutor` (chunk 09) — Claude integration
- `prSplit.automatedSplit()` (chunk 10) — full pipeline orchestration

### Testing Strategy

1. **Unit tests** for each screen render function (given state → expected view)
2. **State machine tests** for all FSM transitions
3. **Integration tests** using real bubbletea test driver (if available)
4. **termmux integration tests** for toggle behavior
5. **Visual regression** via VHS tape recordings

---

## 11. Comparison: Current vs. Target

### Current (pr_split_13_tui.js)

- go-prompt REPL with text commands
- No graphical elements
- No mouse support
- No visual progress indicators
- Commands as wizard actions (type "start", "review", "accept")
- State machine exists but drives a text-based flow

### Target (this design)

- Full-screen BubbleTea wizard with 7 distinct screens
- Mouse support via bubblezone on every interactive element
- Visual progress bars, spinners, status badges
- Responsive layouts (compact, standard, wide)
- termmux integration for Claude terminal switching
- Clickable notifications for Claude status
- Intentional color palette and typography
- Every pixel of terminal space serves the UX

---

## 12. BubbleTea Lifecycle Routing

### init()

```javascript
function init() {
    // Detect current branch, auto-populate config
    const branch = prSplit.gitExec('rev-parse', '--abbrev-ref', 'HEAD').trim();
    return [
        tea.batch(
            tea.windowSize(),           // Get initial dimensions
            tea.enterAltScreen(),       // Full-screen mode
            tea.enableReportFocus(),    // Track focus events
        ),
        // Model initialized via initModel() with branch auto-detected
    ];
}
```

### update() — Message Router

The update function is a two-level dispatch: first by message type (global
handlers), then by wizard state (screen-specific handlers).

```javascript
function update(model, msg) {
    // ─── Global handlers (apply regardless of screen) ───
    if (msg.type === 'WindowSizeMsg') {
        model.width = msg.width;
        model.height = msg.height;
        if (model.viewport) {
            model.viewport.setWidth(msg.width - 4);
            model.viewport.setHeight(contentHeight(model));
        }
        return [model, null];
    }

    if (msg.type === 'KeyMsg') {
        // Ctrl+C → quit with confirmation
        if (msg.ctrl && msg.key === 'c') {
            model.showConfirmCancel = true;
            return [model, null];
        }
        // ? → help overlay (only when not in text input)
        if (msg.runes === '?' && !isTextInputFocused(model)) {
            model.showHelp = !model.showHelp;
            return [model, null];
        }
    }

    if (msg.type === 'MouseMsg') {
        // Claude badge click → toggle to Claude terminal
        if (zone.inBounds('claude-badge', msg) && msg.action === 'press') {
            return [model, function() { model.mux.switchTo(); }];
        }
    }

    // ─── Overlay handlers (intercept when overlay is active) ───
    if (model.showConfirmCancel) {
        return updateConfirmCancel(model, msg);
    }
    if (model.showHelp) {
        return updateHelpOverlay(model, msg);
    }
    if (model.showMoveDialog) {
        return updateMoveDialog(model, msg);
    }
    if (model.execution && model.execution.error) {
        return updateErrorOverlay(model, msg);
    }

    // ─── Screen-specific handlers ───
    switch (model.wizardState) {
        case 'CONFIG':           return updateConfig(model, msg);
        case 'PLAN_GENERATION':  return updateAnalysis(model, msg);
        case 'PLAN_REVIEW':      return updatePlanReview(model, msg);
        case 'PLAN_EDITOR':      return updatePlanEditor(model, msg);
        case 'BRANCH_BUILDING':  return updateExecution(model, msg);
        case 'EQUIV_CHECK':      return updateVerification(model, msg);
        case 'FINALIZATION':     return updateFinalization(model, msg);
        default:                 return [model, null];
    }
}
```

### view() — Screen Router

```javascript
function view(model) {
    const sections = [];

    // Title bar (always rendered)
    sections.push(renderTitleBar(model));
    sections.push(renderDivider(model.width));

    // Screen content
    switch (model.wizardState) {
        case 'CONFIG':           sections.push(viewConfig(model)); break;
        case 'PLAN_GENERATION':  sections.push(viewAnalysis(model)); break;
        case 'PLAN_REVIEW':      sections.push(viewPlanReview(model)); break;
        case 'PLAN_EDITOR':      sections.push(viewPlanEditor(model)); break;
        case 'BRANCH_BUILDING':  sections.push(viewExecution(model)); break;
        case 'EQUIV_CHECK':      sections.push(viewVerification(model)); break;
        case 'FINALIZATION':     sections.push(viewFinalization(model)); break;
    }

    // Navigation bar
    sections.push(renderDivider(model.width));
    sections.push(renderNavBar(model));

    // Status bar
    sections.push(renderDivider(model.width));
    sections.push(renderStatusBar(model));

    // Compose vertically
    let output = lipgloss.joinVertical(lipgloss.Left, ...sections);

    // Overlay compositing (rendered OVER the base screen)
    if (model.showConfirmCancel) {
        output = compositeOverlay(output, renderConfirmCancel(model), model);
    }
    if (model.showHelp) {
        output = compositeOverlay(output, renderHelp(model), model);
    }
    if (model.showMoveDialog) {
        output = compositeOverlay(output, renderMoveDialog(model), model);
    }
    if (model.execution && model.execution.error) {
        output = compositeOverlay(output, renderErrorOverlay(model), model);
    }

    // Final zone scan for mouse hit-testing
    return zone.scan(output);
}
```

### Overlay Composition

Overlays are rendered as centered, bordered panels placed over the base
screen content using `lipgloss.place()`:

```javascript
function compositeOverlay(base, overlay, model) {
    // Center the overlay in the terminal
    const placed = lipgloss.place(
        model.width,
        model.height,
        lipgloss.Center,
        lipgloss.Center,
        overlay
    );
    // The overlay replaces the base content at the center
    // In a real terminal, this works because characters overwrite
    return placed;
}
```

### Form Focus Cycling

Config and Editor screens use a focus ring implemented as an integer index:

```javascript
function cycleFocus(model, direction) {
    const fields = getFieldsForScreen(model);
    model.focusedField = (model.focusedField + direction + fields.length) % fields.length;

    // Blur all, focus the selected
    fields.forEach((f, i) => {
        if (f.blur) f.blur();
        if (i === model.focusedField && f.focus) f.focus();
    });

    return model;
}
```

Each field knows its own type (textarea, radio group, checkbox, button)
and renders with appropriate highlight when focused:

```javascript
function renderFormField(field, focused) {
    const border = focused
        ? lipgloss.roundedBorder()
        : lipgloss.normalBorder();
    const borderColor = focused ? COLORS.primary : COLORS.border;

    // Textareas render their own view
    if (field.type === 'textarea') {
        return lipgloss.newStyle()
            .border(border)
            .borderForeground(borderColor)
            .render(field.textarea.view());
    }

    // Radio groups render options with selection indicator
    if (field.type === 'radio') {
        return field.options.map((opt, i) =>
            (i === field.selected ? '● ' : '○ ') + opt.label
        ).join('\n');
    }

    // Checkboxes
    if (field.type === 'checkbox') {
        return (field.checked ? '☑ ' : '☐ ') + field.label;
    }
}
```

---

## Appendix A: Style Constants

```javascript
// Color palette
const COLORS = {
    primary:   '#7C3AED',
    secondary: '#6366F1',
    success:   '#10B981',
    warning:   '#F59E0B',
    error:     '#EF4444',
    muted:     '#6B7280',
    surface:   '#1F2937',
    border:    '#374151',
    text:      '#F9FAFB',
    textDim:   '#9CA3AF',
};

// Reusable styles
const styles = {
    titleBar: lipgloss.newStyle()
        .bold(true)
        .foreground(COLORS.text)
        .padding(0, 1),

    stepIndicator: lipgloss.newStyle()
        .foreground(COLORS.textDim),

    activeCard: lipgloss.newStyle()
        .border(lipgloss.doubleBorder())
        .borderForeground(COLORS.primary)
        .padding(0, 1),

    inactiveCard: lipgloss.newStyle()
        .border(lipgloss.normalBorder())
        .borderForeground(COLORS.border)
        .padding(0, 1),

    errorCard: lipgloss.newStyle()
        .border(lipgloss.normalBorder())
        .borderForeground(COLORS.error)
        .padding(0, 1),

    successBadge: lipgloss.newStyle()
        .background(COLORS.success)
        .foreground('#000000')
        .padding(0, 1),

    warningBadge: lipgloss.newStyle()
        .background(COLORS.warning)
        .foreground('#000000')
        .padding(0, 1),

    errorBadge: lipgloss.newStyle()
        .background(COLORS.error)
        .foreground('#FFFFFF')
        .padding(0, 1),

    primaryButton: lipgloss.newStyle()
        .background(COLORS.primary)
        .foreground('#FFFFFF')
        .bold(true)
        .padding(0, 2),

    secondaryButton: lipgloss.newStyle()
        .border(lipgloss.normalBorder())
        .borderForeground(COLORS.secondary)
        .foreground(COLORS.secondary)
        .padding(0, 1),

    disabledButton: lipgloss.newStyle()
        .foreground(COLORS.muted)
        .faint(true)
        .padding(0, 2),

    progressFull: lipgloss.newStyle()
        .foreground(COLORS.primary),

    progressEmpty: lipgloss.newStyle()
        .foreground(COLORS.border),

    divider: lipgloss.newStyle()
        .foreground(COLORS.border),
};
```

## Appendix B: Component Helpers

```javascript
// Render a progress bar
function renderProgressBar(percent, width) {
    const filled = Math.round((width - 2) * percent / 100);
    const empty = (width - 2) - filled;
    return styles.progressFull.render('█'.repeat(filled)) +
           styles.progressEmpty.render('░'.repeat(empty)) +
           ' ' + percent + '%';
}

// Render step dots
function renderStepDots(current, total) {
    let dots = '';
    for (let i = 0; i < total; i++) {
        dots += (i < current) ? '●' : '○';
    }
    return dots;
}

// Render the title bar
function renderTitleBar(model) {
    const title = '🔀 PR Split Wizard';
    const step = 'Step ' + stepNumber(model) + '/6: ' + stepName(model);
    const elapsed = formatElapsed(Date.now() - model.startTime);
    const w = model.width;
    const left = styles.titleBar.bold(true).render(title);
    const right = styles.stepIndicator.render(step + '  ⏱ ' + elapsed);
    return lipgloss.joinHorizontal(lipgloss.Center,
        lipgloss.placeHorizontal(w / 2, lipgloss.Left, left),
        lipgloss.placeHorizontal(w / 2, lipgloss.Right, right)
    );
}

// Render a clickable button
function renderButton(id, label, style) {
    return zone.mark(id, style.render(label));
}

// Render the navigation bar
function renderNavBar(model) {
    const parts = [];
    if (canGoBack(model)) {
        parts.push(renderButton('btn-back', '← Back', styles.secondaryButton));
    }
    parts.push(renderButton('btn-cancel', 'Cancel', styles.secondaryButton));
    parts.push(renderStepDots(stepNumber(model), 6));
    const primary = getPrimaryAction(model);
    if (primary) {
        const style = primary.enabled ? styles.primaryButton : styles.disabledButton;
        parts.push(renderButton('btn-primary', primary.label, style));
    }
    return lipgloss.joinHorizontal(lipgloss.Center, ...parts.map(p => '  ' + p + '  '));
}

// Render the status bar
function renderStatusBar(model) {
    const toggle = 'Ctrl+] Toggle Claude';
    const help = '? Help';
    const claude = renderClaudeBadge(model);
    return lipgloss.joinHorizontal(lipgloss.Top,
        styles.divider.render(toggle),
        '  │  ',
        styles.divider.render(help),
        '  │  ',
        claude
    );
}

// Render the Claude status badge (clickable!)
function renderClaudeBadge(model) {
    const status = model.claudeStatus;
    let badge;
    if (status === 'idle') {
        badge = styles.divider.render('🔔 Claude: idle');
    } else if (status === 'error' || status === 'needs-input') {
        badge = styles.warningBadge.render('🔔 Claude: ⚠ ' + status);
    } else {
        badge = styles.successBadge.render('🔔 Claude: ' + status);
    }
    return zone.mark('claude-badge', badge);
}
```
