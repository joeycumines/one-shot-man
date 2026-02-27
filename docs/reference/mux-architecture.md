# Terminal Multiplexer Architecture

> Internal documentation for the `internal/termui/mux` package.

## Overview

The terminal multiplexer (TUIMux) manages switching between two views of a terminal session:

1. **SideOsm** — osm's BubbleTea TUI (auto-split planner, progress display)
2. **SideClaude** — direct passthrough to a child PTY (e.g., Claude Code)

The user toggles between views with a configurable key (default: `Ctrl+\`). The multiplexer captures the child's terminal output in a virtual terminal buffer (VTerm) so the screen can be faithfully restored when toggling back.

## Component Diagram

```
┌─────────────────────────────────────────────────┐
│                    TUIMux                        │
│                                                  │
│  ┌──────────┐    ┌─────────┐    ┌────────────┐  │
│  │  stdin   │───>│ Toggle  │    │  Status    │  │
│  │ (user)   │    │ Detect  │    │  Bar       │  │
│  └──────────┘    └────┬────┘    └────────────┘  │
│                       │                          │
│           ┌───────────┼───────────┐              │
│           │           │           │              │
│     ┌─────▼─────┐  ┌─▼───────┐   │              │
│     │  SideOsm  │  │ Side    │   │              │
│     │ (BubbleTea│  │ Claude  │   │              │
│     │  TUI)     │  │ (Pass   │   │              │
│     └───────────┘  │ through)│   │              │
│                     └───┬─────┘   │              │
│                         │         │              │
│      ┌──────────────────▼─────────▼──────────┐   │
│      │         Background Reader              │   │
│      │  (permanent goroutine: child → VTerm)  │   │
│      └──────────────────┬────────────────────┘   │
│                         │                        │
│                    ┌────▼────┐                   │
│                    │  VTerm  │                   │
│                    │ (VT100  │                   │
│                    │ virtual │                   │
│                    │ buffer) │                   │
│                    └─────────┘                   │
│                         │                        │
│                    ┌────▼────┐                   │
│                    │  Child  │                   │
│                    │  PTY    │                   │
│                    └─────────┘                   │
└─────────────────────────────────────────────────┘
```

## Goroutine Model

### Goroutine Inventory

| Goroutine | Lifetime | Role |
|-----------|----------|------|
| **Main** | Full session | Runs TUI event loop or calls RunPassthrough |
| **Background Reader** | Attach → Detach | Reads child PTY → writes to VTerm, conditionally forwards to stdout |
| **Stdin Forwarder** | Each RunPassthrough call | Reads user stdin → writes to child PTY |

### Background Reader (permanent during attach)

The background reader is started by `Attach()` and runs until `Detach()`:

```
backgroundReader goroutine:
  loop:
    n, err := child.Read(buf)
    if err → signal bgChildEOF, return

    mu.Lock()
    vterm.Write(buf[:n])         // always — captures screen state
    if passthroughActive:
        stdout.Write(buf[:n])    // forward to real terminal
    mu.Unlock()
```

**Key design decisions:**

- **Always writes to VTerm** regardless of which side is active, preventing child starvation (pipe buffer fill → child blocks)
- **Conditionally forwards to stdout** only when passthrough is active
- **Holds mutex** during stdout writes to prevent interleaving with status bar or screen restore operations

### Toggle Sequence

When the user presses the toggle key in SideClaude:

```
1. passthroughActive = false   (stop forwarding)
2. Restore TUI alt-screen
3. Signal RunPassthrough to return
4. BubbleTea resumes (SideOsm)
5. Background reader continues writing to VTerm
```

When toggling back to SideClaude:

```
1. BubbleTea releases terminal
2. If first swap: clear screen + SIGWINCH
3. Else: write VTerm.Render() to restore screen
4. Render status bar (if enabled)
5. passthroughActive = true    (resume forwarding)
6. Enter RunPassthrough stdin→child loop
```

## VTerm State Machine

The VTerm parser is a byte-by-byte state machine implementing a subset of VT100/xterm:

```
                        ESC
stateGround ─────────> stateEscape
    │                     │
    │ (printable)         ├── '[' ──> stateCSI
    │ (control: LF,CR,   ├── ']' ──> stateOSC
    │  TAB,BS,BEL)        ├── 'P' ──> stateDCS
    │                     ├── '7' ──> DECSC (save cursor)
    │                     ├── '8' ──> DECRC (restore cursor)
    │                     ├── 'M' ──> RI (reverse index)
    │                     ├── 'H' ──> HTS (set tab stop)
    │                     ├── 'c' ──> RIS (full reset)
    │                     └── other → stateGround
    │
    │ (bytes >= 0x80)
    └──> UTF-8 accumulator (carry buffer across writes)
```

### Supported Sequences

| Category | Sequences |
|----------|-----------|
| **Cursor Movement** | CUU (A), CUD (B), CUF (C), CUB (D), CNL (E), CPL (F), CHA (G), CUP (H/f), VPA (d) |
| **Erase** | ED (J), EL (K), ECH (X) |
| **Scroll** | SU (S), SD (T), RI (ESC M), IND (ESC D) |
| **Insert/Delete** | IL (L), DL (M), ICH (@), DCH (P) |
| **Tab** | CHT (I), CBT (Z), HTS (ESC H), TBC (g) |
| **Attributes** | SGR (m) — 4-bit, 8-bit (256), 24-bit truecolor, bold, italic, underline, etc. |
| **Mode Set** | DECSET/DECRST (h/l) — ?25 (cursor visibility), ?47/?1047/?1049 (alt screen) |
| **Scroll Region** | DECSTBM (r) |
| **Cursor Save** | DECSC (ESC 7), DECRC (ESC 8), CSI s/u |
| **Consumed (no-op)** | OSC (title, etc.), DCS, DSR (n), DA (c), XTWINOPS (t) |

### Screen Buffer Structure

```
VTerm
├── primary: screenBuffer      (normal screen)
├── alternate: screenBuffer    (alt-screen apps: vim, less, etc.)
├── active → primary or alternate
├── mutex (sync.Mutex)
├── parser state machine
└── UTF-8 carry buffer

screenBuffer
├── cells [][]cell             (2D grid: [row][col])
├── curRow, curCol             (cursor position)
├── curAttr                    (current SGR attributes)
├── scrollTop, scrollBot       (scroll region, 1-indexed)
├── savedRow, savedCol, savedAttr  (DECSC/DECRC)
├── cursorVisible              (DECTCEM)
├── tabStops []bool            (configurable tab positions)
└── rows, cols                 (dimensions)
```

## Synchronization

### Mutex Scope

The VTerm has a `sync.Mutex` protecting all state access:

- `Write()` — locks for the entire write operation
- `Render()` — locks for the entire render
- `Resize()` — locks for the entire resize

The TUIMux holds its own mutex (`m.mu`) for stdout writes during passthrough:

- Background reader holds `m.mu` when forwarding to stdout
- Status bar rendering holds `m.mu`
- Screen restore on toggle-back holds `m.mu`

### No I/O Under Lock

The VTerm mutex protects only in-memory state (cells, cursor, parser state). No syscalls or I/O occur while the VTerm lock is held. The TUIMux mutex similarly only covers stdout.Write operations.

## Render Optimization

`Render()` generates ANSI escape sequences to reproduce the screen:

1. **Skip empty rows** — rows where all cells are default (space, no attributes) are omitted entirely
2. **Trim trailing spaces** — trailing default cells on non-empty rows are omitted
3. **SGR diffing** — only emits SGR changes between adjacent cells
4. **Cursor positioning** — emits CUP for each non-empty row
5. **Cursor visibility** — appends DECTCEM show/hide at the end

## Status Bar Integration

When the status bar is enabled:

- VTerm is sized to `(height - statusBarLines, width)` instead of full terminal height
- The real terminal's scroll region is set to `1..(height - statusBarLines)`
- The status bar occupies the bottom row(s), painted separately
- On resize, both VTerm and status bar adjust
- On toggle-back, status bar is re-rendered after VTerm.Render()

## Known Limitations

1. **No sixel/image support** — image protocols are consumed as DCS but not rendered
2. **No mouse reporting** — mouse events in passthrough mode are not captured
3. **No scrollback** — only the visible buffer is maintained
4. **No terminal response** — DSR, DA, DECRQSS queries are silently consumed (no response written to child)
5. **No bidirectional text** — RTL text rendering follows simple LTR cell placement
6. **OSC content discarded** — window titles, clipboard, hyperlinks are consumed but not stored
