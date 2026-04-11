# TUI Keymap Reference

Complete keyboard shortcut reference for the `osm pr-split` interactive wizard.

Source of truth: `handleKeyMessage` and `handleOverlays` in
`internal/command/pr_split_16e_tui_update.js`.

## Global Navigation

Available on all screens unless an overlay is active.

| Key            | Action                           |
|----------------|----------------------------------|
| `?` / `F1`     | Toggle help overlay              |
| `Tab`          | Next field / option              |
| `Shift+Tab`    | Previous field / option          |
| `Enter`        | Confirm / select                 |
| `Esc`          | Back / close overlay             |
| `Ctrl+C`       | Cancel wizard (confirm dialog)   |

## Scrolling

| Key            | Action                           |
|----------------|----------------------------------|
| `j` / `↓`      | Move down / scroll               |
| `k` / `↑`      | Move up / scroll                 |
| `PgUp` / `PgDn`| Scroll page                      |
| `Home` / `End` | Jump to top / bottom             |

## Plan Editor

Active in PLAN_EDITOR and PLAN_REVIEW states.

| Key              | Action                         |
|------------------|--------------------------------|
| `e`              | Edit / rename selected split   |
| `Space`          | Toggle file checkbox           |
| `Shift+↑`       | Reorder file up within split   |
| `Shift+↓`       | Reorder file down within split |

## Branch Building

Active in BRANCH_BUILDING and EQUIV_CHECK states.

| Key              | Action                                            |
|------------------|---------------------------------------------------|
| `e`              | Expand / collapse verify output                   |
| `z`              | Pause / resume verify process (SIGSTOP / SIGCONT) |
| `Ctrl+C`         | Interrupt current verify (2× = force kill)        |
| `p`              | Mark branch as passed (override)                  |
| `f`              | Mark branch as failed                             |
| `c`              | Continue / skip branch                            |

The `z`, `Ctrl+C`, `p`, `f`, `c` keys require an active verify session
(BRANCH_BUILDING state only).

## Split View

| Key              | Action                                    |
|------------------|-------------------------------------------|
| `Ctrl+L`         | Toggle split view on / off                |
| `Ctrl+Tab`       | Switch focus: wizard ↔ bottom pane        |
| `Ctrl+O`         | Cycle bottom pane tabs (Claude → Output → Verify) |
| `Ctrl+]`         | Full passthrough (focused pane)           |
| `Ctrl+=` / `Ctrl+-` | Resize split view ratio (±10%)         |

`Ctrl+Tab` and the tab cycle keys are only active when split view is
enabled (`Ctrl+L`). They do not consume input when split view is off.

### Bottom Pane — Output Tab

Read-only scrollback. All keys consumed (not forwarded).

| Key              | Action                   |
|------------------|--------------------------|
| `j` / `↓`        | Scroll down              |
| `k` / `↑`        | Scroll up                |
| `PgUp` / `PgDn`  | Scroll 5 lines           |
| `Home` / `End`   | Jump to top / bottom     |

### Bottom Pane — Verify Tab (PTY mode)

When a live PTY verify session is active, most keys are forwarded to the
child terminal process. Reserved keys that are NOT forwarded:

`Ctrl+Tab`, `Ctrl+L`, `Ctrl+O`, `Ctrl+]`, `Ctrl++`, `Ctrl+=`, `Ctrl+-`, `F1`

### Bottom Pane — Verify Tab (fallback mode)

When PTY is unavailable, the verify tab shows scrollable output:

| Key              | Action                   |
|------------------|--------------------------|
| `j` / `↓`        | Scroll down              |
| `k` / `↑`        | Scroll up                |
| `PgUp` / `PgDn`  | Scroll 5 lines           |
| `Home` / `End`   | Jump to top / bottom     |

### Bottom Pane — Claude Tab

Claude pane supports scrolling and PTY forwarding. Reserved keys that
are NOT forwarded:

`Ctrl+Tab`, `Ctrl+L`, `Ctrl+O`, `Ctrl+]`, `Ctrl++`, `Ctrl+=`, `Ctrl+-`,
`↑`, `↓`, `j`, `k`, `PgUp`, `PgDn`, `Home`, `End`, `F1`

## Passthrough Mode

| Key              | Action                           |
|------------------|----------------------------------|
| `Ctrl+]`         | Exit passthrough, return to TUI  |

Passthrough gives the focused session full terminal control. The toggle
key (`Ctrl+]`, byte `0x1D`) is intercepted by the Go-level
`toggleModel` wrapper before JavaScript runs.

## Overlays

### Help Overlay

| Key              | Action        |
|------------------|---------------|
| *(any key)*      | Close overlay |

### Confirm Cancel

| Key              | Action                         |
|------------------|--------------------------------|
| `Tab`            | Cycle Yes / No focus           |
| `Shift+Tab`      | Cycle focus (reverse)          |
| `Enter`          | Activate focused button        |
| `y`              | Confirm cancel (quit)          |
| `n` / `Esc`      | Dismiss (continue)             |

### Report Overlay

| Key              | Action              |
|------------------|----------------------|
| `Esc` / `Enter` / `q` | Close overlay   |
| `c`              | Copy report to clipboard |
| `j` / `↓`        | Scroll down          |
| `k` / `↑`        | Scroll up            |
| `PgDn` / `Space` | Half page down       |
| `PgUp`           | Half page up         |
| `Home` / `g`     | Jump to top          |
| `End`            | Jump to bottom       |

### Editor Dialogs

All editor dialogs close with `Esc`.

**Move File:** `j`/`↓` and `k`/`↑` navigate targets, `Enter` confirms.

**Rename Split:** Type to edit, `Backspace` to delete, `Enter` to save.

**Merge Splits:** `j`/`↓` and `k`/`↑` navigate, `Space` toggles
selection, `Enter` confirms merge.

### Claude Conversation

| Key              | Action                         |
|------------------|--------------------------------|
| `Esc`            | Close conversation             |
| `Enter`          | Send message                   |
| `Backspace`      | Delete character               |
| `Ctrl+U`         | Clear input line               |
| `↑` / `PgUp`    | Scroll history up              |
| `↓` / `PgDn`    | Scroll history down            |

### Claude Question Input

| Key              | Action                         |
|------------------|--------------------------------|
| `Esc`            | Dismiss question               |
| `Enter`          | Send response to Claude PTY    |
| `Backspace`      | Delete character               |
| `Ctrl+U`         | Clear input                    |
| *(any char)*     | Accumulate in input buffer     |

### Inline Title Editing (Plan Editor)

| Key              | Action                         |
|------------------|--------------------------------|
| `Enter`          | Save title                     |
| `Esc`            | Cancel without saving          |
| `Backspace`      | Delete character               |
| `Ctrl+U`         | Clear text                     |

### Config Field Editing

| Key              | Action                         |
|------------------|--------------------------------|
| `Enter`          | Commit value                   |
| `Esc`            | Cancel editing                 |
| `Backspace`      | Delete character               |
| `Ctrl+U`         | Clear field                    |

Numeric fields (e.g., maxFiles) accept digits only.

## Dispatch Priority

When multiple handlers could match, the TUI processes in this order:

1. **Window resize** — always handled first
2. **State transition reset** — clears focus index
3. **Overlays** — checked in order: help → confirm cancel → report →
   editor dialog → Claude conversation → Claude question →
   inline title edit → config field edit
4. **Per-type dispatch** — Key → ToggleReturn → Mouse → Tick

`Ctrl+]` is intercepted by the Go-level `toggleModel` wrapper before
any JavaScript dispatch runs, so it effectively has the highest
priority.
