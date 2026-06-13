# termmux Architecture

The `termmux` package provides a terminal multiplexer and virtual terminal emulator subsystem for `osm`. It manages PTY-backed child processes, renders their output through a VT220/xterm-compatible emulator (`vt`), captures point-in-time screen snapshots, and supports raw terminal passthrough with toggle-key switching.

## 1. Overview

`termmux` sits at the intersection of three concerns:

- **Process lifecycle** — spawning, resizing, signaling, and waiting on child processes in PTYs.
- **Terminal emulation** — parsing ANSI escape sequences (CSI, ESC, OSC, DCS, C0/C1) and maintaining a cell-based screen buffer with scrollback.
- **Session management** — multiplexing multiple sessions through a single worker goroutine, publishing screen snapshots, and delivering typed events.

The subsystem is the foundation for interactive TUI workflows (code review, prompt flow, super-document) where `osm` swaps between its own UI and a child shell, capturing the shell's full VT output as structured data.

### Key Design Decisions

- **Single worker goroutine** — All state mutation for `SessionManager` flows through a single goroutine. Public methods dispatch requests via `reqChan`; the worker dispatches synchronously. This eliminates locks on session state.
- **Separation of PTY and emulation** — `pty/` manages process and file descriptors. `vt/` emulates the terminal. `SessionManager` wires them together through `CaptureSession`.
- **Snapshot-first rendering** — Screen content is never rendered directly during output processing. Instead, the worker computes an immutable `ScreenSnapshot` (with lazy `sync.Once` rendering) and publishes it. Consumers read snapshots, not live state.
- **Non-blocking output fan-out** — `EventBus` uses copy-on-write subscriber maps with non-blocking channel sends. Dropped events are silently counted via `EventsDropped()`.

## 2. Package Structure

```
internal/termmux/                    # Main package (termmux)
├── doc.go                           # Package overview
├── defaults.go                      # DefaultRows=24, DefaultCols=80, buffer sizes
├── config.go                        # DefaultToggleKey (Ctrl+], 0x1D)
├── errors.go                        # ErrNoChild, ErrPassthroughActive, ErrPauseNotSupported, ErrResumeNotSupported
├── side.go                          # ExitReason type + constants (toggle, child-exit, context, error, suspended)
├── session.go                       # InteractiveSession interface, SessionTarget, SessionKind
├── stringio.go                      # StringIO interface for agent handle adapter
├── stringio_session.go              # StringIOSession: wraps StringIO as InteractiveSession
├── manager.go                       # SessionManager, managedSession, ScreenSnapshot, SessionState
├── capture.go                       # CaptureSession: PTY command executor with drain/pause
├── passthrough.go                   # SessionManager.Passthrough: raw terminal toggle mode
├── passthrough_config.go            # PassthroughConfig, TerminalIO, PassthroughOptions, UIConfig
├── passthrough_signal_unix.go       # watchSignals + SIGTSTP handling (Unix)
├── passthrough_signal_windows.go    # watchSignals stub (Windows: no SIGTSTP)
├── forward_stdin.go                 # forwardStdin: stdin→PTY with toggle key + SGR mouse filtering
├── eventbus.go                      # EventBus, Event, EventKind, typed accessors
├── persistence.go                   # PersistedManagerState, SaveManagerState, LoadManagerState
├── persistence_unix.go / _windows.go # Platform-specific persistence helpers
├── signal_unix.go / _windows.go     # Platform-specific signal handling
├── resize_unix.go / _windows.go     # Platform-specific resize handling
├── input.go                         # KeyToTermBytes, MouseToSGR
├── sgrmouse.go                      # SGR mouse sequence parsing + status bar filtering
├── layout.go                        # PaneGeometry, SplitLayout
├── write.go                         # writeOrLog utility
├── platform_unix.go / _windows.go   # Platform-specific platform guards
├── pty/                             # PTY management (creack/pty on Unix, ConPTY on Windows)
│   ├── pty.go                       # Process struct, Spawn, platform-agnostic methods
│   ├── pty_unix.go                  # Unix: creack/pty wrapper
│   ├── pty_windows.go               # Windows: ConPTY wrapper
│   ├── pty_signal_unix.go           # Unix: SIGSTOP, SIGCONT support
│   ├── pty_signal_windows.go        # Windows: no SIGSTOP/SIGCONT
│   ├── pty_unix_test.go
│   ├── pty_windows_test.go
│   ├── pty_test.go
│   ├── pty_splitcmd_test.go
│   ├── termios_darwin.go            # Darwin-specific termios
│   └── termios_linux.go             # Linux-specific termios
├── ptyio/                           # PTY I/O primitives
│   ├── doc.go                       # Package overview
│   ├── ptyio.go                     # PTY interface, TermState (MakeRaw/Restore/GetSize), RealTermState
│   ├── reader.go                    # BufferedReader: buffered channel-based read loop
│   ├── blocking_unix.go             # UnixBlockingGuard: fcntl O_NONBLOCK clearing
│   ├── blocking_windows.go          # Windows stub (no-op)
│   ├── reader_test.go
│   ├── blocking_unix_test.go
│   ├── integration_test.go
│   └── bench_test.go
├── statusbar/                       # Status bar renderer
│   ├── doc.go
│   └── statusbar.go                 # StatusBar: render, scroll region, toggle key
│   └── statusbar_test.go
└── vt/                              # Virtual terminal emulator
    ├── doc.go                       # Comprehensive escape sequence compliance table
    ├── vt.go                        # VTerm: screen buffers, parser, UTF-8, CSI/ESC handlers, Snapshot
    ├── screen.go                    # Screen: cell grid, scrollback, dirty-region, save/restore cursor
    ├── parser.go                    # ANSI state machine: CSI/ESC/OSC/DCS parsing
    ├── sgr.go                       # SGR attribute parsing (8/256/truecolor) + diff computation
    ├── csi.go                       # CSI dispatch: cursor, erasure, scrolling, modes, DA, DSR
    ├── esc.go                       # ESC dispatch: DECSC/DECRC, RI/IND/NEL/RIS/HTS/DECALN
    ├── utf8.go                      # UTF-8 byte-by-byte accumulator
    ├── render.go                    # RenderFullScreen, RenderContentANSI, RenderAll
    ├── render_raster.go             # PNG image generation (16+256color+RGB palette) + SaveRasterPNG
    ├── defaults.go                  # DefaultRows=24, DefaultCols=80, MaxScrollback=10000, MaxProtocolLength=4096
    └── [24 test files covering each feature area]
```

**File count**: ~100 Go source files across the termmux package tree (including tests).

## 3. Core Abstractions

### 3.1 VTerm (`vt/vt.go`)

The virtual terminal emulator is a concurrent-safe VT220/xterm emulator. Each `VTerm` owns two `Screen` instances (primary + alternate) and an ANSI state machine parser.

```
VTerm
├── primary   *Screen   (reflows on resize, scrollback buffer, MaxScrollback=10000)
├── alternate *Screen   (no scrollback, no reflow, fixed dimensions)
├── active    *Screen   (points to primary or alternate)
├── parser    *Parser   (table-driven ANSI state machine)
├── utf8      UTF8Accum (per-byte UTF-8 accumulation)
├── csi       CSIHandler (dispatches CSI sequences)
├── esc       ESCHandler (dispatches ESC sequences)
└── mu        sync.Mutex (concurrent safety)
```

**Key methods**:

| Method | Description |
|---|---|
| `NewVTerm(rows, cols)` | Creates primary + alternate screens, wires callback handlers |
| `Write([]byte) (int, error)` | Processes bytes through ANSI state machine (fast path for printable ASCII runs) |
| `Resize(rows, cols)` | Resizes both screens in-place |
| `Snapshot() *VTermSnapshot` | Single-lock capture of plain text + ANSI + full screen + all mode state |
| `RenderFullScreen() string` | Flicker-free CUP+EL+SGR output for terminal restore |
| `ContentANSI() string` | SGR-only rendering (no positioning) for TUI embedding |
| `String() string` | Plain text representation for diagnostics |
| `CursorPosition() (row, col)` | Cursor position under lock |
| `EnterCopyMode()` / `ExitCopyMode()` | Copy/scroll mode with selection support |
| `SelectedText() string` | Text selection including scrollback rows |
| `CopySelection()` | Sends selection via OSC 52 clipboard sequence |
| `FocusIn()` / `FocusOut()` | Sends ESC[I / ESC[O to child via ResponseWriter |
| `ScrollUp(n)` / `ScrollDown(n)` | Adjust ScrollOffset within [0, totalLines] |
| `SetScrollback(n)` | Change max scrollback (rebuilds ring buffer) |
| `ActiveScreen() *Screen` | Deep copy of active screen state |

**Alt screen switching** (`vt.go`, lines 291–373) — Three modes supported:

- **Mode 1049** (DECSET ?1049h): Saves ALL cursor/attribute/mode state to `Saved1049*` fields, clears alternate screen, activates it. On restore: restores all saved state, clamps cursor to screen bounds.
- **Mode 1047**: Switches to alternate, clears it on exit (per xterm spec). No cursor save.
- **Mode 47**: Switches without saving cursor or clearing.

**Fast path** (`vt.go`, lines 128–140): In `StateGround` with no special modes, the writer batches contiguous printable ASCII bytes (0x20–0x7E) directly into `Screen.PutASCII()`, bypassing the per-byte parser dispatch, UTF-8 checks, and charset mapping.

### 3.2 Screen (`vt/screen.go`)

The cell grid and mode state for a terminal viewport.

```
Screen
├── Cells [][]Cell                  (row × col 2D grid)
├── CurRow, CurCol int              (cursor position, 0-indexed)
├── CurAttr Attr                    (current SGR attributes)
├── ScrollTop, ScrollBot int        (scroll region, 1-indexed)
├── Scrollback [][]Cell             (ring buffer for scrolled content)
├── ScrollbackLen, ScrollbackHead, MaxScrollback, ScrollOffset int
├── TabStops []bool
├── Rows, Cols int
├── G0Charset, G1Charset, GL int    (character set state)
├── DirtyRowMin, DirtyRowMax int    (dirty region tracking for incremental rendering)
├── PendingWrap, CursorVisible      (cursor state)
├── MouseTracking MouseTrackingMode  (0/1/2/3)
├── MouseSGR, HighlightTracking     (mouse mode flags)
├── BracketedPaste, ApplicationCursor, KeypadApplication
├── AutoWrap, SynchronizedOutput, OriginMode, InsertMode, LineFeedNewLine
├── SavedRow, SavedCol, SavedAttr   (DECSC/DECRC save state)
└── Saved1049Row, Saved1049Col...   (mode 1049 save state, separate from DECSC)
```

**Cell structure** (`vt/screen.go`, lines 18–27):

```go
type Cell struct {
    Ch         rune    // character (0 for wide-char placeholder, space for blank)
    Attr       Attr    // colors/style flags
    SecondHalf bool    // right half of a CJK/double-width character
}
```

**Key methods**: `NewScreen(rows, cols)`, `Resize(rows, cols)` (reflow for primary, simple for alternate), `DirtyRange()`, `ClearDirty()`, `Clear()`, `SoftReset()`, `LineFeed()`, `ReverseIndex()`, `EraseDisplay(mode)`, `EraseLine(mode)`, `InsertLines()`, `DeleteLines()`, `InsertChars()`, `DeleteChars()`, `EraseChars()`, `PutChar(rune)`, `PutASCII([]byte)` (fast path for ASCII runs), `CurrentSGR()`, `CurrentScrollRegion()`.

**Dirty region tracking**: Every cell write marks the cell as dirty. `DirtyRange()` returns the inclusive `[minRow, maxRow]` range of modified rows. `ClearDirty()` resets the tracking. Used by `RenderContentANSIDirty()` for incremental rendering.

### 3.3 Parser (`vt/parser.go`)

A table-driven ANSI escape sequence state machine with 7 states:

```
StateGround → StateEscape (on 0x1B)
StateEscape → StateCSI (on [ or # or %) → dispatches CSI sequences
StateEscape → StateESCIntermediate (on 0x20–0x2F) → accumulates intermediate bytes
StateEscape → StateCharset → charset designation (ESC ( B, ESC ) 0, etc.)
StateGround → StateOSC (on ESC ]) → OSC sequences (window title, working dir, clipboard)
StateGround → StateDCS (on ESC P) → DCS sequences (sixel, DECSTRQSS)
```

**Key methods**:

| Method | Description |
|---|---|
| `Feed(b byte) (Action, byte)` | Processes one byte, returns action + final byte for dispatch |
| `Params() []int` | Returns parsed CSI parameters (mutated in place per call) |
| `SubParams(idx) []int` | Returns colon-separated sub-params at index |
| `HasIntermediate(b byte) bool` | Checks for an intermediate byte |
| `OSCData() (code int, data string)` | Returns the numeric OSC code and string payload |
| `DCSData() []byte` | Returns accumulated DCS payload bytes |
| `Reset()` | Returns to StateGround |
| `CurState() State` | Returns current parser state |

**Constraints**: Maximum protocol length of 4096 bytes for OSC and DCS sequences (prevents unbounded buffer growth from hostile input).

### 3.4 InteractiveSession (`session.go`)

The minimal interface for terminal endpoints managed by `SessionManager`:

```go
type InteractiveSession interface {
    Write([]byte) (int, error)  // Send bytes to PTY stdin
    Resize(rows, cols int) error // Resize PTY (delivers SIGWINCH)
    Close() error               // Terminate and release resources
    Done() <-chan struct{}       // Closed on session termination
    Reader() <-chan []byte       // Stream raw PTY output chunks
}
```

**Implementations**:
- `CaptureSession` — PTY-backed command execution (`capture.go`)
- `StringIOSession` — String-based agent handle adapter (`stringio_session.go`)

### 3.5 SessionManager (`manager.go`)

The central orchestrator. Manages multiple `InteractiveSession` instances through a **single worker goroutine**.

```
SessionManager
├── reqChan chan request                      // Public API → worker
├── mergedOutput chan sessionOutput           // Per-session reader → worker
├── eventBus *EventBus                        // Pub/sub event system
├── done chan struct{}                        // Manager shutdown
├── started chan struct{}                     // Worker readiness
├── readerCtx context.Context / readerCancel  // Lifespan of all reader goroutines
├── sessions map[SessionID]*managedSession    // All registered sessions (worker-owned)
├── activeID SessionID                        // Currently active session
├── nextID SessionID                          // Monotonically increasing (starts at 1)
├── termRows, termCols int                    // Global terminal dimensions
├── snapshotGen uint64                        // Global snapshot generation counter
└── passthroughSessionID SessionID            // Session in passthrough mode
```

**Constructor options** (`manager.go`, lines 276–288):

```go
NewSessionManager(opts ...ManagerOption) *SessionManager
    WithTermSize(rows, cols int)       // Default: 24×80
    WithRequestBuffer(cap int)         // Default: 64
    WithMergedOutputBuffer(cap int)    // Default: 64
```

**Key public methods** (all dispatched to worker via `sendRequest`):

| Method | Description |
|---|---|
| `Run(ctx)` | Starts the worker loop; blocks until ctx cancelled or Close called |
| `Close()` | Signals shutdown |
| `Started() <-chan struct{}` | Ready signal |
| `Register(session, target)` | Registers a session, creates VTerm + snapshot, spawns reader goroutine |
| `Unregister(id)` | Removes a session, terminates VTerm output |
| `Activate(id)` | Sets the active session |
| `Input(data)` | Sends bytes to the active session's writer |
| `Resize(rows, cols)` | Resizes all sessions' VTerms and PTYs |
| `ResizeSession(id, rows, cols)` | Resizes a specific session |
| `Snapshot(id)` | Returns an immutable `*ScreenSnapshot` |
| `Screen(id)` | Returns a deep copy of the active screen (`*vt.Screen`) |
| `Sessions()` | Returns `[]SessionInfo` with state and target metadata |
| `Subscribe(bufSize)` | Subscribes to EventBus, returns `(id, <-chan Event)` |
| `Unsubscribe(id)` | Removes a subscriber |
| `Passthrough(ctx, cfg)` | Enters raw terminal mode for active session |
| `ExportState()` / `RestoreFromState()` | Persistence: serialize/deserialize manager state |

**Worker loop** (`manager.go`, lines 573–591):

```go
for {
    select {
    case <-ctx.Done():
        return
    case req, ok := <-m.reqChan:
        m.dispatch(req)
    case so := <-m.mergedOutput:
        m.handleSessionOutput(so)
    }
}
```

### 3.6 ScreenSnapshot (`manager.go`)

An immutable, concurrent-safe point-in-time capture of a VTerm screen:

```go
type ScreenSnapshot struct {
    Gen uint64                    // Monotonic snapshot counter
    screen *vt.Screen             // Pointer to the live screen (read-only from this point)
    Rows, Cols int
    CursorRow, CursorCol int

    // Mode state snapshot
    MouseTracking int             // 0/1/2/3
    MouseSGR, InsertMode, BracketedPaste bool
    ApplicationCursor, KeypadApplication bool
    CursorShape int               // 0=default, 1=blink-block, 2=steady-block, 3=blink-underline, 4=steady-underline, 5=blink-bar, 6=steady-bar
    FocusReporting, AutoWrap, SynchronizedOutput, LineFeedNewLine bool

    // Lazy rendering with sync.Once
    plainText, ansi, fullScreen string
    oncePlainText, onceANSI, onceFullScreen sync.Once

    // Timestamp of capture
    Timestamp time.Time
}
```

Lazy rendering methods (`GetPlainText()`, `GetANSI()`, `GetFullScreen()`) each use `sync.Once` for thread-safe computation. The `Gen` field increments with each snapshot, enabling consumers to detect stale data.

## 4. Data Flow

### 4.1 Output Pipeline

```
Child Process
    │
    │ PTY master fd (proc.File())
    ▼
┌─────────────────────────────┐
│ BufferedReader.ReadLoop()   │  Reads from PTY fd into 32KB internal buffer
│ (per CaptureSession)        │  Copies chunks to avoid pinning backing array
│                             │  Sends on buffered channel (capacity: DefaultChannelBuffer=64)
└──────────┬──────────────────┘
           │ Output channel
           ▼
┌─────────────────────────────────────────────┐
│ readerLoop()                                │
│ ┌─────────────────────┬───────────────────┐ │
│ │ outputCh            │ passthroughOutput │ │
│ │ (Reader() consumer) │ (os.Stdout during │ │
│ │ SessionManager      │  passthrough)     │ │
│ └─────────┬───────────┴────────┬──────────┘ │
└───────────┼────────────────────┼─────────────┘
            │                    │
            ▼                    ▼
    ┌───────────────┐    ┌──────────────┐
    │ mergedOutput  │    │ os.Stdout    │
    │ channel       │    │              │
    └───────┬───────┘    └──────────────┘
            │
            ▼
┌─────────────────────────────────────────────┐
│ SessionManager.handleSessionOutput(so)      │
│  1. Write chunk → VTerm.Write(chunk)        │
│  2. Create ScreenSnapshot (lazy rendering)   │
│  3. Atomic store to managedSession.screen   │
│  4. Emit EventSessionOutput to EventBus     │
│  5. Skip event if SynchronizedOutput active │
│  6. Tee to passthroughOutput if active      │
└─────────────────────────────────────────────┘
```

### 4.2 Input Pipeline

```
User stdin (raw terminal mode)
    │
    │ os.Stdin
    ▼
┌──────────────────────────────────────┐
│ forwardStdin() goroutine             │
│ ┌──────────────────┬───────────────┐ │
│ │ Toggle key scan  │ SGR mouse     │ │
│ │ (Ctrl+])         │ pre-process   │ │
│ │ (status bar)     │ (filter)      │ │
│ └────────┬─────────┴───────┬───────┘ │
└──────────┼─────────────────┼─────────┘
           │                 │
           ▼                 ▼
    ┌──────────────┐  ┌────────────┐
    │ VT buffer    │  │ status bar │
    │ (VTerm)      │  │ clicks     │
    └──────┬───────┘  └────────────┘
           │
           ▼
    ┌──────────────────────────────┐
    │ activeWriter() → PTY stdin   │
    │ (managedSession.writer)      │
    └──────────────────────────────┘
```

### 4.3 Snapshot Publication

```
handleSessionOutput(so)
    │
    ├── so.screen.Write(so.data)  → updates VTerm cell grid
    │
    ├── vTerm.Snapshot()          → single mutex lock, computes:
    │   ├── plainText (plain chars only)
    │   ├── ANSI (SGR-styled, no positioning)
    │   ├── FullScreen (CUP+EL+SGR flicker-free)
    │   └── All mode state booleans
    │
    ├── managedSession.screen.Store(&ScreenSnapshot{...})  → atomic.Pointer
    │
    ├── EventBus.Publish(EventSessionOutput, ...)          → non-blocking
    │
    └── (if passthroughActive) → writeSo(so) to stdout
```

## 5. Session Lifecycle

### 5.1 State Machine

```
SessionCreated
    │
    ├──► SessionRunning     (output is flowing)
    │       │
    │       ▼
    │   SessionExited       (child process terminated)
    │       │
    │       ▼
    │   SessionClosed       (unregistered from manager)
    │
    └──► SessionClosed     (closed before running, e.g., registration failure)
```

**State transitions enforced by `SessionManager`** (`manager.go`, lines 33–49):

| Transition | Trigger |
|---|---|
| Created → Running | `handleRegister` succeeds |
| Running → Exited | EOF on PTY read (reader exits) |
| Exited → Closed | `handleUnregister` |
| Any → Closed | `Close()` called on manager |

### 5.2 Registration Flow

```
SessionManager.Register(session, target)
    │
    ├── sendRequest(reqRegister{...})
    │
    ├── handleRegister:
    │   ├── nextID++
    │   ├── Create VTerm(rows, cols)
    │   ├── Wire callbacks:
    │   │   ├── BellFn: log bell event to EventBus
    │   │   ├── ResponseWriter: DA/DSR responses → child stdin
    │   │   ├── OSCHandler: OSC 0/2→title, 7→working dir, 52→clipboard
    │   │   └── DCSHandler: raw DCS payload delivery
    │   ├── Create ScreenSnapshot (initial state)
    │   ├── Create managedSession
    │   ├── Spawn per-session reader goroutine:
    │   │   └── waitForReader(session.Reader()) → poll at 10ms interval
    │   │       └── ForEach chunk: send to mergedOutput
    │   └── Return SessionID
    │
    └── EventBus.Publish(EventSessionRegistered, sessionID)
```

### 5.3 Reader Goroutine Pattern

Each registered session gets a dedicated reader goroutine:

```go
go func() {
    for {
        ch := waitForReader(session)  // poll 10ms until Reader() returns non-nil
        if ch == nil { return }
        for data := range ch {
            select {
            case m.mergedOutput <- sessionOutput{
                sessionID: id, data: data,
            }:
            case <-m.done:
                return
            }
        }
        // Channel closed → EOF
        select {
        case m.mergedOutput <- sessionOutput{
            sessionID: id, eof: true,
        }:
        case <-m.done:
            return
        }
    }
}()
```

### 5.4 Termination

```
Close()
    │
    ├── cancel readerCtx → all reader goroutines exit
    │
    ├── For each managedSession:
    │   ├── Set state → Closed
    │   └── Publish EventSessionClosed
    │
    └── Close reqChan
```

## 6. Passthrough Mode

Passthrough enters raw terminal I/O mode, taking over the terminal for direct stdin→PTY and PTY→stdout forwarding while the worker goroutine continues processing output through VTerm for snapshot capture.

### 6.1 SessionManager.Passthrough (`passthrough.go`)

**Entry sequence**:

1. Get active session's writer via `activeWriter()`
2. Set terminal to raw mode via `MakeRaw()` (preserves state for restore on exit)
3. Ensure stdin is in blocking mode via `EnsureBlocking()` (prevents EAGAIN)
4. Setup status bar: render, set scroll region, resize VTerm to account for status bar
5. Screen display: either restore VTerm (flicker-free FullScreen output) or clear screen (`ESC[2J`)
6. Enable output tee via `enablePassthroughTee(activeID, stdout)` — reader output is also written to stdout
7. Start SIGWINCH watcher goroutine (`watchResize`) — resizes all sessions + status bar on terminal resize
8. Build SGR mouse pre-processor if status bar is active — filters mouse clicks targeting the status bar row
9. Start stdin→PTY forwarding (`forwardStdin`) with toggle key detection and optional pre-processor
10. Start signal watcher (`watchSignals`) — SIGINT/SIGQUIT/SIGTSTP forwarding
11. Enter main select loop waiting for: stdin result, session events, signal result, context cancel

**Exit detection** (four paths):

| Source | Event | ExitReason |
|---|---|---|
| `forwardStdin` goroutine | Toggle key pressed | `ExitToggle` |
| `forwardStdin` goroutine | PTY write error | `ExitError` |
| EventBus | `EventSessionExited` / `EventSessionClosed` | `ExitChildExit` |
| Signal watcher | SIGTSTP (Unix only) | `ExitSuspended` |
| Context | `ctx.Done()` | `ExitContext` |

### 6.2 CaptureSession.Passthrough (`capture.go`, lines 520–621)

A simpler variant of `SessionManager.Passthrough` used for standalone capture sessions:

- No status bar
- No VTerm restore (clears screen with `ESC[2J`)
- Reader loop continues during passthrough, forwarding chunks to `passthroughOutput` (stdout) when `passthroughActive` is true
- Returns immediately on `ctx.Done()`

### 6.3 stdin→PTY Forwarding (`forward_stdin.go`)

```
forwardStdin(ctx, resultCh, cfg)
    │
    ├── Loop:
    │   ├── Read from cfg.Stdin into buf (DefaultPassthroughReadBufferSize=4096)
    │   ├── EAGAIN retry (defense-in-depth)
    │   ├── If preProcess is set:
    │   │   ├── Prepend carry-over bytes from previous partial SGR prefix
    │   │   ├── Call filterMouseForStatusBar()
    │   │   ├── If clicked: return ExitToggle
    │   │   └── Discard filtered-out mouse sequences
    │   ├── Scan for toggle key byte (cfg.ToggleKey, default 0x1D = Ctrl+])
    │   │   └── If found: return ExitToggle
    │   └── Write to cfg.Writer (PTY stdin)
    │
    └── On ctx.Done or error: send forwardResult{reason, err} to resultCh
```

### 6.4 SGR Mouse Filtering for Status Bar (`sgrmouse.go`)

When a status bar is rendered, mouse clicks targeting the status bar row must be intercepted before being forwarded to the child process (which would "poison" its PTY stdin buffer).

```
filterMouseForStatusBar(buf, termRows, statusBarLines) → (filtered, partial, clicked)
    │
    ├── Scan buf for SGR mouse sequence prefix: ESC [ < Ps ; Px ; Py [Mm]
    │   ├── If full sequence found and y targets status bar row:
    │   │   ├── Return filtered buf (mouse sequence removed)
    │   │   ├── Return partial bytes from next sequence (if any)
    │   │   └── Return clicked=true
    │   ├── If partial sequence (incomplete prefix):
    │   │   ├── Return carry-over bytes for next read
    │   │   └── Return clicked=false (not yet determined)
    │   └── If sequence doesn't target status bar:
    │       └── Return unmodified buf
    │
    └── Overflow guard: reject SGR mouse values > MaxCoord (10000) to prevent 32-bit overflow
```

### 6.5 Signal Forwarding

**Unix** (`passthrough_signal_unix.go`):

```
watchSignals(ctx, sigResultCh, signalChild)
    ├── Ignore SIGTTOU and SIGTTIN (prevents background process group hangs)
    ├── Listen for SIGINT → forward to child, return ExitToggle
    ├── Listen for SIGQUIT → forward to child, return ExitToggle
    └── Listen for SIGTSTP → forward to child, return ExitSuspended
```

**Windows** (`passthrough_signal_windows.go`):

No SIGTSTP support (ConPTY does not support process suspension). Only SIGINT and SIGQUIT.

### 6.6 Exit Reasons (`side.go`)

| Constant | Value | Meaning |
|---|---|---|
| `ExitToggle` | 0 | User pressed the toggle key |
| `ExitChildExit` | 1 | Child process exited (EOF on PTY read) |
| `ExitContext` | 2 | Context was cancelled |
| `ExitError` | 3 | I/O error occurred |
| `ExitSuspended` | 4 | Child process stopped (SIGTSTP, Unix only) |

## 7. Capture Mode

`CaptureSession` is a standalone PTY-backed command executor with real-time output capture. It provides a simplified alternative to `SessionManager` for cases where only raw output forwarding is needed — no terminal multiplexing, toggle keys, status bar, or raw-mode management.

### 7.1 Configuration (`CaptureConfig`)

| Field | Default | Description |
|---|---|---|
| `Command` | (required) | Executable path or name |
| `Args` | `nil` | Command arguments |
| `Dir` | CWD | Working directory |
| `Env` | `nil` | Additional environment variables (merged with `os.Environ()`) |
| `Rows` | 24 | Terminal row count |
| `Cols` | 80 | Terminal column count |
| `DrainTimeout` | 5s | Max wait time for reader to exit after PTY close |
| `SkipDrain` | `false` | Skip the drain wait (useful after `Kill()`) |

### 7.2 Lifecycle

```
NewCaptureSession(cfg)
    │
    ├── Start(ctx)
    │   ├── pty.Spawn(ctx, SpawnConfig{...})  → creates PTY + child process
    │   ├── ptyio.NewBufferedReader(proc.File(), 16)
    │   ├── go reader.ReadLoop(readerCtx)     → reads PTY into channel
    │   └── go readerLoop()                    → forwards chunks to outputCh
    │       │
    │       ├── For each chunk from reader.Output():
    │       │   ├── Copy chunk (avoid pinning 32KB buffer)
    │       │   ├── Non-blocking send to outputCh (drop if full)
    │       │   └── If passthroughActive: writeOrLog to passthroughOutput
    │       │
    │       └── After channel close:
    │           ├── proc.Wait() → capture exitCode/exitErr
    │           └── close(cs.done)
    │
    ├── Read from cs.Reader() → <-chan []byte (stream of output chunks)
    │
    ├── Wait() → (code int, err error)
    │   └── Blocks on <-cs.done, then returns exitCode/exitErr
    │
    ├── Pause() → SIGSTOP / Resume() → SIGCONT (Unix only)
    │
    ├── Interrupt() → SIGINT / Kill() → SIGKILL
    │
    ├── Resize(rows, cols) → proc.Resize() (delivers SIGWINCH)
    │
    └── Close()
        ├── cancel context (stops child)
        ├── cancel readerReadLoop
        ├── proc.Close() (closes PTY fd)
        └── if !SkipDrain:
            └── Wait on <-cs.done with DrainTimeout
```

### 7.3 Drain Timeout and SkipDrain

`Close()` waits for the reader goroutine to finish after the PTY fd is closed. This is necessary because the reader loop needs to process any remaining buffered output before the process exits.

- **Default drain timeout**: 5 seconds (`DefaultDrainTimeout`).
- **`SkipDrain = true`**: Skips the wait entirely. Use this after `Kill()` when you don't care about remaining output.
- **The timeout is a safety net**: `proc.Close()` closes the PTY fd, which causes `BufferedReader.ReadLoop` to exit, which closes the output channel, which causes `readerLoop` to exit. The timeout handles edge cases where fd closure doesn't unblock immediately.

### 7.4 Output Dropping

When `outputCh` is full (the SessionManager consumer is slow), chunks are silently dropped and counted via `atomic.Int64` (`droppedOutput`). This prevents the reader loop from stalling, which would block the PTY and potentially deadlock the child process.

## 8. Platform Differences

### 8.1 PTY Allocation

| Platform | Backend | Key File | Features |
|---|---|---|---|
| **Unix** (Linux, macOS, BSD) | `creack/pty` | `pty/pty_unix.go` | Full feature set: SIGSTOP, SIGCONT, SIGWINCH, full signal set |
| **Windows** | ConPTY (Console PTY) | `pty/pty_windows.go` | SIGINT, SIGQUIT only. No SIGSTOP/SIGCONT. Uses CreatePseudoConsole API. |

**ConPTY specifics** (`pty/pty_windows.go`):
- Uses `kernel32.CreatePseudoConsole` / `ClosePseudoConsole`
- Input pipe → ConPTY, output pipe ← ConPTY
- PTY master is the output pipe handle (`ConOut`)
- PTY writer is the input pipe handle (`ConIn`)
- Windows does not support process suspension → `Pause()`/`Resume()` return `ErrPauseNotSupported` / `ErrResumeNotSupported`

### 8.2 Signal Handling

| Signal | Unix | Windows |
|---|---|---|
| SIGINT | Forwarded, causes ExitToggle | Forwarded, causes ExitToggle |
| SIGQUIT | Forwarded, causes ExitToggle | Forwarded, causes ExitToggle |
| SIGTSTP | Forwarded, causes ExitSuspended | Not available |
| SIGWINCH | Handled by `watchResize` | Not available (handled differently) |
| SIGTTOU/SIGTTIN | Ignored in `init()` | N/A |

### 8.3 Blocking Guard

| Platform | File | Behavior |
|---|---|---|
| Unix | `ptyio/blocking_unix.go` | `UnixBlockingGuard` uses `fcntl` to clear O_NONBLOCK on the fd. `EnsureBlocking()` saves flags and clears O_NONBLOCK. |
| Windows | `ptyio/blocking_windows.go` | No-op. Windows console I/O is always blocking. |

### 8.4 Persistence

| Platform | File | Behavior |
|---|---|---|
| Unix | `persistence_unix.go` | Uses `os.Getwd()` for current directory resolution. |
| Windows | `persistence_windows.go` | Uses `os.Getwd()` with Windows path handling. |

### 8.5 Resize Handling

| Platform | File | Behavior |
|---|---|---|
| Unix | `resize_unix.go` | Listens for SIGWINCH via `watchResize`. Calls resize callback with new terminal dimensions. |
| Windows | `resize_windows.go` | No SIGWINCH support. Resize handled through other mechanisms (e.g., Bubble Tea resize messages). |

## 9. Event System

### 9.1 EventBus (`eventbus.go`)

High-performance, lock-free event fan-out using copy-on-write subscriber maps.

```
EventBus
├── subscribers atomic.Pointer[subscriberMap]  // Copy-on-write
├── drops atomic.Int64                          // Dropped event count
└── bufferSize int                              // Per-subscriber channel capacity
```

**Subscriber map**:

```go
type subscriberMap struct {
    nextID int
    subs   map[int]*subscriber
}
type subscriber struct {
    id   int
    ch   chan<- Event
}
```

**Publish** (lock-free subscriber read):

```go
func (e *EventBus) Publish(kind EventKind, sessionID SessionID, data any) {
    sm := e.subscribers.Load()
    sm.mu.RLock()
    defer sm.mu.RUnlock()
    for _, sub := range sm.subs {
        select {
        case sub.ch <- Event{Kind: kind, SessionID: sessionID, Data: data, Time: time.Now()}:
        default:
            e.drops.Add(1)  // Non-blocking: drop and count
        }
    }
}
```

**Subscribe** (atomic swap on subscriber map mutation):

```go
func (e *EventBus) Subscribe(ch chan<- Event) int {
    for {
        sm := e.subscribers.Load()
        newID := sm.nextID + 1
        newMap := &subscriberMap{
            nextID: newID,
            subs:   make(map[int]*subscriber, len(sm.subs)+1),
        }
        // Copy existing subscribers
        for id, sub := range sm.subs {
            newMap.subs[id] = sub
        }
        // Add new subscriber
        newMap.subs[newID] = &subscriber{id: newID, ch: ch}
        // Atomic CAS
        if e.subscribers.CompareAndSwap(sm, newMap) {
            return newID
        }
        // CAS failed: retry
    }
}
```

### 9.2 EventKinds (`eventbus.go`, lines 14–47)

| Kind | Data Type | Description |
|---|---|---|
| `EventSessionRegistered` | `nil` | Session was registered with the manager |
| `EventSessionActivated` | `nil` | Session was activated (became active) |
| `EventSessionOutput` | `[]byte` | Raw PTY output chunk |
| `EventSessionExited` | `nil` | Child process exited |
| `EventSessionClosed` | `nil` | Session was removed from manager |
| `EventResize` | `[2]int{rows, cols}` | Terminal was resized |
| `EventBell` | `nil` | BEL (0x07) received |
| `EventTitle` | `string` | Window title (OSC 0/2) |
| `EventWorkingDirectory` | `string` | Working directory (OSC 7, as URI) |
| `EventClipboard` | `string` | Clipboard content (OSC 52, base64 payload) |

### 9.3 Typed Accessors (`eventbus.go`, lines 265–286)

```go
func (e *Event) DataAsBytes() ([]byte, bool)   // For EventSessionOutput
func (e *Event) DataAsDims() ([2]int, bool)     // For EventResize
```

## 10. JS Binding Layer

### 10.1 Module Registration (`builtin/termmux/module.go`)

The package is registered as the native module `osm:termmux` via the `Require` function:

```go
func Require(ctx context.Context, input io.Reader, output io.Writer) func(*goja.Runtime, *goja.Object)
```

### 10.2 Constants Exposed to JS

| JS Constant | Go Value | Description |
|---|---|---|
| `EXIT_TOGGLE` | `"toggle"` | Passthrough exited via toggle key |
| `EXIT_CHILD_EXIT` | `"childExit"` | Passthrough exited via child process exit |
| `EXIT_CONTEXT` | `"context"` | Passthrough exited via context cancellation |
| `EXIT_ERROR` | `"error"` | Passthrough exited via I/O error |
| `SIDE_OSM` | `"osm"` | Side: OSM (parent) |
| `SIDE_CLAUDE` | `"claude"` | Side: Claude (child) |
| `DEFAULT_TOGGLE_KEY` | `0x1D` | Ctrl+] byte value |
| `EVENT_EXIT` | `"exit"` | Child process exited |
| `EVENT_RESIZE` | `"resize"` | Terminal was resized |
| `EVENT_FOCUS` | `"focus"` | Focus side change |
| `EVENT_BELL` | `"bell"` | Bell received |
| `EVENT_OUTPUT` | `"output"` | Child output chunk |
| `EVENT_REGISTERED` | `"registered"` | Session registered |
| `EVENT_ACTIVATED` | `"activated"` | Session activated |
| `EVENT_CLOSED` | `"closed"` | Session closed |
| `EVENT_TERMINAL_RESIZE` | `"terminal-resize"` | Terminal resized |

### 10.3 Factory Functions

**`newCaptureSession(command, args?, opts?)`** — Creates a `CaptureSession` and wraps it as a JS object:

```javascript
const session = termmux.newCaptureSession("bash", ["--norc"], {
    dir: "/path",
    rows: 40,
    cols: 120,
    env: { TERM: "xterm-256color" }
});
```

**`newSessionManager(opts?)`** — Creates a `SessionManager` and wraps it:

```javascript
const mgr = termmux.newSessionManager({
    rows: 40,
    cols: 120,
    requestBuffer: 128,
    outputBuffer: 128,
    title: "Terminal"
});
```

### 10.4 CaptureSession JS Wrapper (17 methods)

| Method | Description |
|---|---|
| `start()` | Spawn command in PTY, start output capture |
| `interrupt()` | Send SIGINT |
| `kill()` | Send SIGKILL |
| `pause()` | Send SIGSTOP (Unix only) |
| `resume()` | Send SIGCONT (Unix only) |
| `isPaused()` → `boolean` | Check pause state |
| `resize(rows, cols)` | Resize PTY |
| `wait()` → `{code, error?}` | Block until exit + drain |
| `write(data)` | Send raw bytes to PTY stdin |
| `sendEOF()` | Send Ctrl-D |
| `close()` | Terminate and release |
| `pid()` → `number` | Child PID |
| `exitCode()` → `number` | Exit code (-1 if not exited) |
| `isDone()` → `boolean` | Non-blocking completion check |
| `passthrough(cfg?)` → `{reason, error?}` | Enter raw terminal mode |
| `reader()` → `string|null` | Blocking read next chunk |
| `readAvailable()` → `string|null` | Non-blocking drain of buffered chunks |

### 10.5 SessionManager JS Wrapper (35+ methods)

**Core session management**: `run`, `started`, `close`, `register`, `unregister`, `activate`, `input`, `resize`, `resizeSession`, `termSize`, `snapshot`, `activeID`, `isDone`, `sessions`, `eventsDropped`, `subscribe`, `unsubscribe`, `passthrough`

**Mux-equivalent convenience**: `attach`, `detach`, `hasChild`, `switchTo`, `screenshot`, `childScreen`, `writeToChild`, `session`, `lastActivityMs`, `setStatus`, `setToggleKey`, `setStatusEnabled`, `setResizeFunc`, `on`, `off`, `pollEvents`, `activeSide`, `fromModel`

**Persistence**: `exportState`, `saveState`, `loadState`, `restoreState`, `removeState`, `processAlive`

**Raster**: `renderRaster(id, options?)` → `{width, height, path}` — Generates PNG image of VTerm screen

### 10.6 Event Bus → JS Bridge

A goroutine subscribes to the `SessionManager`'s `EventBus` and maps Go events to JS events:

```go
// goroutine started by WrapSessionManager
go func() {
    defer mgr.Unsubscribe(busID)
    for {
        select {
        case <-ctx.Done():
            return
        case evt, ok := <-busCh:
            // Map Go Event → JS event via events.queue()
            // events.queue() writes to a buffered channel (capacity 64)
            // Events are drained on the JS goroutine by pollEvents()
        }
    }
}()
```

The bridge handles:
- `EventSessionRegistered` → JS event `"registered"` with `sessionId`
- `EventSessionActivated` → JS event `"activated"` with `sessionId`
- `EventSessionExited` → JS event `"exit"` with `pane: "claude"`, `sessionId`
- `EventSessionClosed` → JS event `"closed"` with `sessionId`
- `EventResize` → JS event `"terminal-resize"` with `rows`, `cols`
- `EventBell` → JS event `"bell"` with `pane: "claude"`, `sessionId`
- `EventSessionOutput` → JS event `"output"` with `pane: "claude"`, `sessionId`, `chunk`

**Non-blocking delivery**: If the `pending` channel is full (64 buffered), events are dropped silently. This prevents blocking non-JS goroutines.

### 10.7 JS Event Listener System

```
on(event, callback) → id          // Register callback, returns numeric ID
off(id) → boolean                 // Remove listener by ID
pollEvents() → number              // Drain pending async events, return count delivered
```

The `muxEvents` struct manages listeners and the pending queue:

```go
type muxEvents struct {
    mu        sync.Mutex
    listeners map[int]*eventListener
    nextID    int
    pending   chan pendingEvent  // Buffered channel (cap 64)
}
```

- `emit()` — Called on the JS goroutine. Snapshots matching listeners under lock, releases lock, then invokes callbacks (Goja is not thread-safe).
- `queue()` — Called from non-JS goroutines. Non-blocking send to `pending` channel. Drops if full.
- `drain()` — Called from `pollEvents()` on JS goroutine. Drains all pending events, delivers to matching listeners.

### 10.8 Input Encoding Utilities

**`keyToTermBytes(key, appCursor?, appKeypad?)`** — Converts BubbleTea-style key names to terminal byte sequences:

| Key Category | Example | Output (appCursor=false) | Output (appCursor=true) |
|---|---|---|---|
| Navigation | `"up"` | `CSI A` | `SS3 A` |
| Function | `"f1"` | `CSI P1` | `SS3 P1` |
| Ctrl+letter | `"ctrl+a"` | `0x01` | — |
| Alt+key | `"alt+a"` | `ESC a` | — |
| Printables | `"a"` | `a` | — |

**`mouseToSGR(event, offsetRow?, offsetCol?)`** — Encodes mouse events as SGR sequences:

```
ESC [ < Ps ; Px ; Py M/m
```

Where `Ps = button + modifiers` (Shift+4, Alt+8, Ctrl+16, Motion+32, Release suffix m) and `Px`/`Py` are column/row coordinates with optional offset.

### 10.9 Layout Utilities

**`splitLayout(config)`** — Creates a `SplitLayout` from config:

```javascript
const layout = termmux.splitLayout({
    totalChromeRows: 1,
    topPaneHeaderRows: 0,
    dividerRows: 1,
    bottomPaneHeaderRows: 0,
    leftChromeCol: 0,
    minPaneRows: 5
});

const { top, bottom } = layout.compute(rows, cols, 0.5);
// top/bottom: { row, col, rows, cols, offsetMouse(screenRow, screenCol) }
```

### 10.10 Go ↔ JS Type Mapping

| Go Type | JS Type | Notes |
|---|---|---|
| `SessionID` (uint64) | `number` | Monotonically increasing |
| `CaptureConfig` | `{ command, args[], dir, rows, cols, env{}` | Object passed to factory |
| `ExitReason` | `"toggle" \| "childExit" \| "context" \| "error"` | camelCase strings |
| `EventKind` | `"exit" \| "resize" \| "focus" \| "bell" \| "output" \| "registered" \| "activated" \| "closed" \| "terminal-resize"` | String constants |
| `ScreenSnapshot` | `{ gen, plainText, ansi, fullScreen, rows, cols, cursorRow, cursorCol, timestamp }` | Lazy rendering on JS side |
| `PaneGeometry` | `{ row, col, rows, cols, offsetMouse(row, col) }` | JS object with method |
| `*goja.Object` | JavaScript object | `_goSession` / `_goSessionManager` data properties store Go pointers |
