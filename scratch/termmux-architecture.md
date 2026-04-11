# Termmux Architecture: Channel-Based Session Manager

## Document Purpose

This document defines the architectural direction for the `termmux` package redesign. It is the authoritative reference for all blueprint tasks that implement the SessionManager, its supporting types, and the JS shim layer. Every architectural decision is justified by (a) analysis of the existing codebase, (b) lessons drawn from tmux's production-proven design, and (c) the stated design constraints.

---

## Design Constraints (Non-Negotiable)

1. **Module extractability.** `termmux` will eventually become its own Go module (`go get …/termmux`). Zero imports from `internal/builtin`, `internal/command`, `internal/scripting`, or any other osm-internal package. The JS shim (`internal/builtin/termmux/`) is the ONLY integration point and lives outside `termmux`.
2. **Single worker goroutine owns all mutable state.** No mutex-protected state. All mutations flow through a request channel and are applied by one goroutine. This is the Go idiomatic translation of tmux's single-threaded `event_loop(EVLOOP_ONCE) + server_loop()` pattern.
3. **Copy-on-write for read access.** Screen snapshots, session lists, and other frequently-read state are published as immutable snapshots via `atomic.Pointer[T]`. Readers never block on the worker.
4. **Channel-based request/reply (ping-pong).** Every mutation is a `request` sent to the worker's `reqChan`, with a caller-provided `reply chan`. The worker processes the request and responds. The caller blocks on the reply channel.
5. **Rigorous lifecycle management.** Every component has a defined state machine with explicit transitions. No implicit cleanup via GC. Context cancellation propagates deterministically.
6. **Breaking changes are mandatory.** Existing callers (Mux, CaptureSession, JS shim, pr_split) will be refactored to use the new architecture. No compatibility shims.
7. **No prepositions in names.** Per project convention.

---

## Tmux Lessons Applied

### Lesson 1: Single-Threaded State Owner (from `server.c` / `proc.c`)

Tmux runs a single-threaded event loop:
```c
do {
    event_loop(EVLOOP_ONCE);
} while (!exit && !server_loop());
```

All state mutations (session creation, pane spawning, key routing, resize propagation) happen within this single thread. There are **zero locks** in tmux's core. This gives tmux deterministic behavior, freedom from deadlocks, and trivial reasoning about state consistency.

**Go translation:** A single goroutine reads from a request channel and processes all mutations sequentially. Read-only data is exposed via atomic pointers to immutable snapshots.

### Lesson 2: Containment Hierarchy (from `tmux.h` structs)

Tmux organizes state hierarchically:
```
Session → Window (via winlink) → Pane → Screen (grid of cells)
```

Each level has:
- A unique numeric ID (monotonically increasing)
- Bidirectional links (pane → window, window → session)
- Lifecycle flags (`PANE_EXITED`, `SESSION_ALERTED`)
- Activity timestamps

**Go translation:** We adopt a two-level hierarchy for now: `SessionManager → managedSession`. Each `managedSession` wraps an `InteractiveSession` and adds worker-owned metadata (state, screen snapshot, activity). The hierarchy is extensible to add Windows/Layouts later.

### Lesson 3: Buffered Event I/O (from `bufferevent` pattern)

Tmux uses libevent `bufferevent` for PTY I/O: a background event handler pumps data into buffers, and the main loop drains them. This decouples I/O speed from processing speed.

**Go translation:** Each session's PTY output is read by a dedicated goroutine that sends chunks to the worker via a channel. The worker processes these chunks (updates VTerm, publishes snapshot, emits events) without blocking on I/O.

### Lesson 4: Input Routing via Active Session (from `server-client.c`)

Tmux routes client input to `client->session->curw->window->active` (the active pane). Key bindings are checked first; unbound keys pass through.

**Go translation:** The SessionManager maintains an `activeSessionID`. Input arriving via `Input()` is routed to the active session's PTY. The routing is a simple map lookup — no scanning or iteration.

### Lesson 5: Resize Propagation (from `resize.c` / `layout.c`)

Tmux propagates resize events through the layout tree:
```
SIGWINCH → resize_window() → layout_resize() → per-pane ioctl(TIOCSWINSZ)
```

**Go translation:** Resize is a request to the worker. The worker updates all session dimensions (or, in the future, walks the layout tree) and sends `TIOCSWINSZ` to each PTY.

### Lesson 6: Delta Rendering (from `screen-write.c` / `screen-redraw.c`)

Tmux tracks per-cell dirty state and only renders deltas. This is critical for performance.

**Go translation:** The COW screen snapshot includes a generation counter. Consumers can diff against their last-seen generation. (This is handled by the existing VTerm dirty flags but will be surfaced in the snapshot API.)

---

## Architecture Overview

```
┌────────────────────────────────────────────────────────────────┐
│                    JS Shim Layer                               │
│            internal/builtin/termmux/module.go                  │
│   (request/reply calls to SessionManager methods)              │
└──────────────────────┬─────────────────────────────────────────┘
                       │ method calls
┌──────────────────────▼─────────────────────────────────────────┐
│                  SessionManager (Public API)                    │
│            internal/termmux/manager.go                          │
│                                                                 │
│   Methods (each sends request, awaits reply):                   │
│     Register(session, target) → (SessionID, error)              │
│     Unregister(id) → error                                      │
│     Activate(id) → error                                        │
│     Input(data) → error     (routes to active session)          │
│     Resize(rows, cols) → error (broadcasts to all sessions)     │
│     Snapshot(id) → *ScreenSnapshot (nil if not found)           │
│     ActiveID() → SessionID                                      │
│     Sessions() → []SessionInfo (COW list)                       │
│     Subscribe(bufSize) → (int, <-chan Event)                    │
│     Unsubscribe(id) → bool                                      │
│     Close()                                                      │
│     Started() → <-chan struct{}  (blocks until worker ready)    │
│                                                                 │
│   Internal:                                                     │
│     reqChan  chan request      ← mutations flow in               │
│     eventBus *EventBus        → events flow out                 │
│     Run(ctx) → error (the worker loop)                           │
└──────────────────────┬─────────────────────────────────────────┘
                       │ owns (single goroutine)
┌──────────────────────▼─────────────────────────────────────────┐
│              Worker Goroutine (state owner)                      │
│                                                                 │
│  State (NEVER accessed outside this goroutine):                 │
│    sessions              map[SessionID]*managedSession           │
│    activeID              SessionID                               │
│    nextID                SessionID (monotonic counter)           │
│    termRows              int                                     │
│    termCols              int                                     │
│    snapshotGen           uint64 (monotonic snapshot counter)     │
│    passthroughSessionID  SessionID (guards concurrent PT)        │
│                                                                 │
│  select loop:                                                   │
│    case req := <-reqChan:                                       │
│        apply mutation, reply                                     │
│    case out := <-mergedOutput (per-session PTY output):         │
│        update VTerm, publish snapshot, emit event                │
│    (exit detected via nil-data EOF sentinel in mergedOutput)      │
│    case <-ctx.Done():                                            │
│        graceful shutdown                                         │
│                                                                 │
└──────────────────────┬─────────────────────────────────────────┘
                       │ manages
┌──────────────────────▼─────────────────────────────────────────┐
│             managedSession (per-session state)                   │
│                                                                 │  
│   session    InteractiveSession  (the actual PTY/process)       │
│   vterm      *vt.VTerm           (screen buffer, worker-owned)  │
│   snapshot   atomic.Pointer[ScreenSnapshot]  (COW, read-safe)   │
│   state      SessionState       (Created→Running→Exited→Closed) │
│   target     SessionTarget      (metadata)                      │
│   lastActive time.Time                                          │
│   passthroughWriter  atomic.Pointer[io.Writer]  (tee output)   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Core Types

### SessionID

```go
// SessionID is a unique, monotonically increasing identifier for a managed session.
type SessionID uint64
```

### SessionState

```go
type SessionState int

const (
    SessionCreated  SessionState = iota // Registered, not yet producing output
    SessionRunning                       // Active PTY, producing output
    SessionExited                        // Process exited, output drained
    SessionClosed                        // Resources released
)
```

State transitions (enforced by worker):
```
Created → Running  (automatic on first output received)
Running → Exited   (Reader channel closed, EOF sentinel processed by worker)
Running → Closed   (Unregister or shutdown bypasses Exited — validTransition not checked)
Exited  → Closed   (Unregister or shutdown)
Created → Closed   (Unregister before start, or process exited without producing output)
```

### ScreenSnapshot (COW)

```go
// ScreenSnapshot is an immutable point-in-time capture of a session's screen.
// Published via atomic.Pointer by the worker goroutine. Safe for concurrent reads.
type ScreenSnapshot struct {
    Gen        uint64    // Monotonically increasing generation
    PlainText  string    // Screen content without ANSI (for search/capture)
    ANSI       string    // Screen content with ANSI escapes (for TUI embedding — SGR-only)
    FullScreen string    // CUP-positioned screen content for terminal restoration (passthrough re-entry)
    Rows       int
    Cols       int
    Timestamp  time.Time // When this snapshot was taken
}
```

- `ANSI` uses `vterm.ContentANSI()` — SGR-attributed text suitable for embedding in a TUI component
- `FullScreen` uses `vterm.RenderFullScreen()` — CUP-positioned text suitable for restoring the terminal screen after passthrough exit (flicker-free re-entry)
```

### SessionInfo (COW)

```go
// SessionInfo is an immutable summary of a session, safe for concurrent reads.
type SessionInfo struct {
    ID       SessionID
    Target   SessionTarget
    State    SessionState
    IsActive bool
}
```

### Request/Reply

```go
type requestKind int

const (
    reqRegister requestKind = iota
    reqUnregister
    reqActivate
    reqInput
    reqResize
    reqSnapshot
    reqActiveID
    reqSessions
    reqClose
    reqActiveWriter           // Passthrough: get io.Writer to active session's PTY
    reqEnablePassthroughTee   // Passthrough: enable raw output tee to io.Writer
    reqDisablePassthroughTee  // Passthrough: disable raw output tee
)

type request struct {
    kind    requestKind
    payload any
    reply   chan<- response
}

type response struct {
    value any
    err   error
}
```

### Event Bus

```go
type EventKind int

const (
    EventSessionRegistered EventKind = iota
    EventSessionActivated
    EventSessionOutput
    EventSessionExited
    EventSessionClosed
    EventResize
    EventBell
)

type Event struct {
    Kind      EventKind
    SessionID SessionID
    Data      any       // Currently unused — always nil. Reserved for future per-kind payloads.
    Time      time.Time
}

// EventBus provides fan-out event delivery to multiple subscribers.
type EventBus struct {
    // Implementation: Uses sync.Mutex-protected subscriber list.
    // Subscribers receive events on buffered channels.
    // Full channels drop events (non-blocking send).
}
```

The EventBus is a separate concern from the worker. It is safe for concurrent use because:
- `Subscribe()` / `Unsubscribe()` are mutex-protected (infrequent operations)
- `Publish()` iterates the subscriber list under the mutex and does non-blocking sends (the mutex is held for the entire delivery loop — no snapshot copy)
- This is the ONE place where a mutex is acceptable — it protects the subscriber list, not session state

---

## Worker Goroutine Design

### Select Loop

```go
func (m *SessionManager) Run(ctx context.Context) error {
    defer close(m.done)
    defer m.eventBus.Close()
    
    for {
        select {
        case <-ctx.Done():
            m.shutdownSessions()
            m.closeReqChan()
            return ctx.Err()
        case req, ok := <-m.reqChan:
            if !ok {
                m.shutdownSessions()
                return nil
            }
            m.dispatch(req)
        case so := <-m.mergedOutput:
            m.handleSessionOutput(so)
        }
    }
}
```

### Merged Output Channel Pattern

Instead of `reflect.Select` (which is slow and complex), use a fan-in pattern. Each session's PTY reader goroutine sends `sessionOutput{id, data}` to a shared, buffered channel:

```go
type sessionOutput struct {
    id   SessionID
    data []byte // nil = EOF/done
}

// mergedOutput is a single buffered channel that all session readers write to.
// Capacity sized to prevent backpressure under normal load.
mergedOutput chan sessionOutput
```

When a session is registered, its reader goroutine is started and writes to `mergedOutput`. When a session exits, the reader sends a nil-data sentinel and stops.

### Non-Blocking Worker Guarantee

The worker MUST NOT block on:
- **PTY writes** (input routing): Use buffered write with timeout, or queue writes for a per-session writer goroutine
- **VTerm parsing**: Bound parsing time per chunk (current VTerm is already fast — single Write per chunk)
- **Event publishing**: EventBus.Publish uses non-blocking sends
- **Snapshot publishing**: atomic.Pointer.Store is wait-free

The worker MAY block on:
- **Channel receive** in select (this is intended — it's the idle state)
- **Request processing** (kept fast — map lookups and assignments)

### Graceful Shutdown

Two shutdown paths exist:

**Path 1: `ctx.Done()` (context cancellation)**
```
ctx.Done() received
  → shutdownSessions() — close all registered sessions
  → closeReqChan() — close request channel
  → return ctx.Err()
  → deferred: readerCancel(), eventBus.Close(), close(m.done)
```

**Path 2: `Close()` (explicit close)**
```
Close() calls closeReqChan() then blocks on <-m.done
  → Worker sees reqChan closed (req, ok := <-m.reqChan; !ok)
  → shutdownSessions()
  → return nil
  → deferred: readerCancel(), eventBus.Close(), close(m.done)
```

In both paths, buffered requests are abandoned — callers blocked in
`sendRequest` will panic-recover with `ErrManagerNotRunning` when they
detect the closed reqChan or the done channel.

`shutdownSessions()` collects all session IDs into a sorted slice (descending
by ID for deterministic shutdown order), then iterates: for each session, calls
`session.Close()` and transitions state to Closed. Note: `readerCancel()` is a
defer in `Run()` that fires AFTER `shutdownSessions()` returns — it is NOT
called inside `shutdownSessions()`. The manager has ONE readerCtx for all
sessions, not per-session contexts.

---

## Session Output Pipeline

For each registered session, the following pipeline operates:

```
PTY fd
  ↓ (dedicated reader goroutine, runs independently)
BufferedReader.ReadLoop()
  ↓ (chan []byte, cap 16)
Session Reader Goroutine (per-session)
  ↓ wraps as sessionOutput{id, data}
mergedOutput channel (shared, cap 64)
  ↓ (consumed by worker goroutine)
Worker:
  1. VTerm.Write(data)  — parse ANSI, update screen
  2. snapshot := ScreenSnapshot{PlainText: vterm.String(), ANSI: vterm.ContentANSI(), FullScreen: vterm.RenderFullScreen(), ...}
  3. managedSession.snapshot.Store(&snapshot)  — COW publish
  4. EventBus.Publish(Event{Kind: EventSessionOutput, ...})
```

This pipeline has these properties:
- **Backpressure:** If `mergedOutput` is full, the per-session reader blocks (not the worker)
- **Isolation:** One slow session cannot block another (they have separate BufferedReader channels)
- **Non-blocking worker:** VTerm.Write and snapshot generation are CPU-bound, not I/O-bound

---

## Input Routing

```
Caller: mgr.Input(data)
  → request{reqInput, data, reply}
  → worker receives
  → worker looks up activeID in sessions map
  → worker calls session.Write(data)  (direct PTY write)
  → reply <- response{nil, err}
```

For non-blocking input (critical for TUI responsiveness):
- The worker's input handling is a single `session.Write()` call
- PTY writes are fast (kernel buffer)
- If PTY write blocks (full kernel buffer), the worker blocks briefly — acceptable for single-user CLI

---

## Passthrough Mode

Passthrough (raw terminal forwarding) is fundamentally different from normal operation. During passthrough:
- The SessionManager still runs (worker goroutine active)
- But stdin/stdout are directly piped to the active session's PTY
- The TUI is suspended

Passthrough is implemented as a method on SessionManager:

```go
func (m *SessionManager) Passthrough(ctx context.Context, cfg PassthroughConfig) (ExitReason, error)
```

`PassthroughConfig` includes StatusBar integration:
```go
type PassthroughConfig struct {
    Stdin         io.Reader
    Stdout        io.Writer
    TermFd        int
    ToggleKey     byte
    TermState     ptyio.TermState
    BlockingGuard ptyio.BlockingGuard
    StatusBar     *statusbar.StatusBar // optional — see below
    ResizeFn      func(rows, cols uint16) error // optional — resize propagation
    RestoreScreen bool // true = flicker-free re-entry via VTerm snapshot
}
```

When `StatusBar` is non-nil, passthrough sets a scroll region to reserve the bottom row for the status bar, mirrors the existing `Mux.RunPassthrough()` behavior: height subtraction, status bar rendering, mouse event routing via `filterMouseForStatusBar` (package-level in termmux, file sgrmouse.go), and re-rendering after VTerm restore (using `ScreenSnapshot.FullScreen` for flicker-free terminal restoration).

`ExitReason` encodes why passthrough ended:
```go
type ExitReason int

const (
    ExitToggle    ExitReason = iota // User pressed toggle key
    ExitChildExit                   // Active session's process exited
    ExitContext                     // Context was canceled
    ExitError                       // An I/O error occurred
)
```

Internally:
1. Enter raw terminal mode
2. Start stdin→PTY forwarding goroutine (with toggle key detection — writes directly to session via `activeWriter()`, bypassing reqInput)
3. Enable passthrough tee: set `passthroughWriter` on the active managed session so the **worker goroutine** writes each output chunk to stdout via `handleSessionOutput()` before `VTerm.Write()` — no separate PTY→stdout goroutine is created
4. Block until toggle key pressed, child exits, or context canceled
5. Disable passthrough tee, restore terminal mode
6. Return exit reason

During passthrough, the worker continues operating (receiving output, updating snapshots). The stdin→PTY goroutine bypasses the worker for direct input (low latency). Output flows THROUGH the worker goroutine: PTY → mergedOutput → worker's handleSessionOutput() → passthroughWriter tee → stdout, then VTerm.Write() for screen capture.

---

## Integration with Existing Types

### Mux → SessionManager Migration

The existing `Mux` type becomes a **thin convenience wrapper** around `SessionManager` for the single-session use case, or is eliminated entirely. The migration path:

1. `Mux.Attach(session)` → `mgr.Register(session)` + `mgr.Activate(id)`
2. `Mux.Detach()` → `mgr.Unregister(id)`
3. `Mux.RunPassthrough()` → `mgr.Passthrough(ctx, cfg)`
4. `Mux.ChildScreen()` → `if snap := mgr.Snapshot(id); snap != nil { snap.ANSI }`
5. `Mux.WriteToChild()` → `mgr.Input(data)`
6. `Mux.HasChild()` → `len(mgr.Sessions()) > 0`

Decision: **Eliminate Mux entirely.** The SessionManager provides all Mux functionality plus multi-session support. Keeping Mux creates a redundant abstraction.

### CaptureSession → InteractiveSession

CaptureSession already implements InteractiveSession. It continues to do so. The SessionManager wraps it in a `managedSession` which adds:
- Worker-owned VTerm (moved from CaptureSession to managedSession)
- COW snapshot
- State tracking
- Output channel wiring

CaptureSession's own VTerm becomes redundant. The VTerm is owned by the worker, not by the session. This eliminates the existing concurrency issues with CaptureSession's mutex-protected VTerm.

### InteractiveSession Interface Changes

The interface must be trimmed to minimize what the worker delegates:

```go
type InteractiveSession interface {
    // Write sends input data to the session's stdin.
    Write([]byte) (int, error)
    
    // Resize changes the terminal dimensions. 
    Resize(rows, cols int) error
    
    // Close releases all resources (SIGTERM → SIGKILL → fd close).
    Close() error
    
    // Done returns a channel that closes when the session has fully exited.
    Done() <-chan struct{}
    
    // Reader returns the channel from which PTY output is read.
    // This is consumed by the SessionManager's merged output pipeline.
    Reader() <-chan []byte
}
```

Removed from InteractiveSession:
- `Output()` / `Screen()` — now provided by SessionManager via COW snapshots (VTerm is worker-owned)
- `Target()` / `SetTarget()` — now managed by SessionManager metadata
- `IsRunning()` — now derived from SessionState

This is a **breaking change**. All existing implementations and callers must be updated.

---

## JS Shim Architecture

The JS shim (`internal/builtin/termmux/module.go`) wraps SessionManager for JavaScript:

### Module Exports

```javascript
const termmux = require('osm:termmux');

// Factory
const mgr = termmux.newSessionManager({
    rows: 24,
    cols: 80
});

// Register sessions
const claudeID = mgr.register(claudeSession);
const verifyID = mgr.register(verifySession);

// Activate
mgr.activate(claudeID);

// Read screen (COW snapshot, non-blocking)
const snap = mgr.snapshot(claudeID);
console.log(snap.ansi);

// Send input (routes to active)
mgr.input("ls -la\n");

// Events
mgr.on('sessionOutput', (evt) => { ... });
mgr.on('sessionExited', (evt) => { ... });

// Passthrough (blocking)
const result = mgr.passthrough({ toggleKey: 0x1D });
```

### Implementation Pattern

Each JS method:
1. Validates arguments (throws on invalid input)
2. Sends a request to the worker via the Go SessionManager method
3. Blocks on the reply channel (Goja runtime is single-threaded, this is fine)
4. Returns the response to JS, or throws if error

**Critical: Run() and Goja deadlock prevention.** `SessionManager.Run()` blocks until the context is canceled. Goja is single-threaded. The JS constructor (`termmux.newSessionManager()`) MUST NOT call `Run()` directly — it would deadlock. Instead, the Go-side module code spawns `go mgr.Run(ctx)` in a background goroutine (with ctx derived from the JS runtime's lifecycle) before returning the JS wrapper object. The JS code never sees `run()` — it's an internal Go concern.

Event delivery uses the existing `muxEvents` async queue + drain pattern:
- Background event subscriber goroutine receives events from EventBus
- Queues them to the pending events channel
- JS calls `pollEvents()` to drain and emit to JS callbacks

### Passthrough in JS

`mgr.passthrough()` is a blocking JS call that:
1. Calls `SessionManager.Passthrough()` (which takes over stdin/stdout)
2. Returns `{reason, error?, childOutput?}` when passthrough exits

During passthrough, the Goja runtime is blocked. This is identical to the current `switchTo()` behavior.

---

## Module Extraction Readiness

For `termmux` to become a standalone module:

### Allowed Dependencies
- Go standard library
- `github.com/creack/pty` (PTY creation)
- `golang.org/x/term` (terminal raw mode)
- `golang.org/x/sys` (low-level syscalls)
- Internal sub-packages only: `termmux/pty`, `termmux/ptyio`, `termmux/vt`, `termmux/statusbar`

### Forbidden Dependencies
- Anything under `internal/` (builtin, command, config, scripting, etc.)
- `github.com/dop251/goja` (JS runtime — belongs in the shim)
- Any BubbleTea/Lipgloss types (TUI framework — belongs in the shim)
- Any osm-specific types or interfaces

### Interface Boundaries
- `InteractiveSession` is defined in `termmux` (implementable by external code)
- `SessionManager` is the primary entry point
- `EventBus` is usable standalone
- All configuration via functional options (`ManagerOption` type)

---

## Concurrency Model Summary

| Component | Goroutine | Blocking? | Communication |
|-----------|-----------|-----------|---------------|
| SessionManager.Run | Worker (1) | Blocks on select | reqChan (in), mergedOutput (in), eventBus (out) |
| PTY Reader | Per-session | Blocks on PTY read | BufferedReader output → mergedOutput |
| Passthrough stdin→PTY | Temporary | Blocks on stdin read | Direct write to PTY (bypasses worker) |
| Passthrough PTY→stdout | Worker (tee) | Non-blocking (inline in handleSessionOutput) | passthroughWriter atomic.Pointer |
| EventBus subscriber | Per-subscriber | Non-blocking send | Buffered event channel |
| JS event drainer | Part of JS runtime | On demand (pollEvents) | pendingEvents channel |

### Mutex Usage (Minimized)

| Component | What's Protected | Why Not Channel |
|-----------|-----------------|-----------------|
| EventBus.subscribers | Subscriber list (add/remove) | Infrequent; simpler than channel |

Everything else is channel-based or atomic.

---

## API Design Principles

1. **Methods return values, not references.** Snapshot, SessionInfo, etc. are value types. Callers get copies, not pointers into worker state.
2. **Errors are explicit.** Every mutating method returns an error. No silent failures.
3. **Context propagation.** `Run(ctx)` propagates cancellation to all sessions. Individual session contexts are derived from the manager's context.
4. **Functional options for configuration.** `NewSessionManager(...ManagerOption)` with `WithTermSize(rows, cols)`, `WithRequestBuffer(cap)`, `WithMergedOutputBuffer(cap)`.
5. **No constructor side effects.** `New` allocates. `Run` starts the worker. Separate concerns.
6. **Clean close.** `Close()` is idempotent and blocks until shutdown is complete (waits on `<-m.done`).

---

## Future Extensibility (Anticipated)

### Windows/Layouts (tmux-like)
The `managedSession` can be extended with a `Window` wrapper:
```
SessionManager → Window → []managedSession (with layout tree)
```
The worker already handles multi-session state. Adding layout computation is a new request type (`reqLayout`) that updates session dimensions based on a layout tree.

### Session Persistence/Detach
Since the worker owns all state, serialization is straightforward: snapshot the worker's state (sessions map, active ID, dimensions) and restore on reattach.

### Multiple Clients
Following tmux's model, multiple "clients" (e.g., multiple TUI instances) can subscribe to the same SessionManager via the EventBus. Input routing would need a client → session affinity model.

---

## Migration Strategy

### Phase 1: Core Types & Worker
- Define SessionID, SessionState, ScreenSnapshot, SessionInfo
- Implement request/reply protocol
- Implement worker goroutine with select loop
- Implement EventBus
- Unit tests for worker (using mock sessions)

### Phase 2: Session Integration
- Refactor InteractiveSession interface (remove VTerm ownership)
- Move VTerm to managedSession (worker-owned)
- Wire CaptureSession to new interface
- Implement merged output pipeline
- Integration tests

### Phase 3: SessionManager Public API
- Implement all public methods (Register, Unregister, Activate, Input, Resize, Snapshot, etc.)
- Implement Passthrough mode
- Implement graceful shutdown
- Comprehensive tests with race detection

### Phase 4: Mux Elimination
- Migrate all Mux callers to SessionManager
- Delete Mux type
- Update all tests

### Phase 5: JS Shim
- Rewrite `internal/builtin/termmux/module.go` to wrap SessionManager
- Update event delivery to use EventBus
- Migrate all JS code (pr_split scripts)
- End-to-end tests

### Phase 6: Caller Migration
- Update `internal/command/pr_split.go` to use new APIs
- Update all JS scripts
- Delete deprecated code
