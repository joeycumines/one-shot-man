# Task 42 — Review Pass 1e: scratch/termmux-architecture.md

**Reviewer:** Takumi (single-pass strict review)
**Date:** 2026-04-08
**Verdict:** FAIL (2 findings — both minor, but zero-tolerance policy)

---

## Findings

### Finding 1: `Option` type name mismatch (MINOR)

**Location:** Line 615, Module Extraction Readiness → Interface Boundaries

**Doc says:**
> All configuration via functional options (`Option` type)

**Actual code** (manager.go:339):
```go
type ManagerOption func(*SessionManager)
```

The type is `ManagerOption`, not `Option`. The doc itself uses the correct name `ManagerOption` later at line 645 (API Design Principles §4), making this an internal inconsistency *within* the document as well as a mismatch with the source code.

**Impact:** An implementer searching the codebase for `Option` type would find nothing. Misleading.

**Fix:** Change `Option` to `ManagerOption` on line 615.

---

### Finding 2: State transition table omits Running → Closed (MINOR)

**Location:** Core Types → SessionState, state transitions block (around line 175)

**Doc says:**
```
Created → Running  (automatic on first output received)
Running → Exited   (Reader channel closed, EOF sentinel processed by worker)
Exited  → Closed   (Unregister or shutdown)
Created → Closed   (Unregister before start, or process exited without producing output)
```

**Actual code — `handleUnregister` (manager.go:636–652):**
```go
func (m *SessionManager) handleUnregister(id SessionID) response {
    ms, ok := m.sessions[id]
    if !ok { ... }
    if ms.state == SessionClosed {
        return response{err: fmt.Errorf("%w: already closed", ErrInvalidTransition)}
    }
    // Transitions ANY non-Closed state to Closed — including Running.
    _ = ms.session.Close()
    ms.state = SessionClosed
    delete(m.sessions, id)
    ...
}
```

**Actual code — `shutdownSessions` (manager.go:951–966):**
```go
for _, id := range ids {
    ms := m.sessions[id]
    if ms.state != SessionClosed {
        _ = ms.session.Close()
        ms.state = SessionClosed  // Any state → Closed
        ...
    }
}
```

Both `handleUnregister` and `shutdownSessions` bypass `validTransition()` and can transition `Running → Closed` directly. This is a real runtime transition that the doc's state table does not list.

**Note:** The code's own godoc comment on `SessionState` has the same gap — the doc faithfully mirrors it. But the architecture doc claims these transitions are "enforced by worker," while the worker itself has two code paths (`handleUnregister`, `shutdownSessions`) that violate the formal state machine.

**Impact:** An implementer writing code that handles session state changes might assume `Running` always transitions through `Exited` before reaching `Closed`. That assumption is wrong if `Unregister()` or shutdown occurs while a session is `Running`.

**Fix:** Add `Running → Closed (Unregister or shutdown — bypasses Exited)` to the state transition table. Also consider adding `Exited → Closed` trigger clarification: `(Unregister, shutdown, OR handleSessionOutput EOF from Created)`.

---

## Verified (no issues found)

The following were cross-referenced against source code and confirmed accurate:

| Check | Status |
|-------|--------|
| **Overview box method signatures** — all 13 methods (Register, Unregister, Activate, Input, Resize, Snapshot, ActiveID, Sessions, Subscribe, Unsubscribe, Close, Started, Run) match actual return types, parameter types, and parameter counts | ✅ |
| **Snapshot return type** — `*ScreenSnapshot` (pointer), nil if not found | ✅ |
| **Subscribe return type** — `(int, <-chan Event)` | ✅ |
| **SessionManager internal fields** — reqChan, eventBus, mergedOutput, done, started, readerCtx, readerCancel, sessions, activeID, nextID, termRows, termCols, snapshotGen, passthroughSessionID — all names and types match | ✅ |
| **managedSession fields** — session, vterm, snapshot, state, target, lastActive, passthroughWriter — all names and types match (including `atomic.Pointer[io.Writer]`) | ✅ |
| **Worker select loop pseudo-code** — three cases (ctx.Done, reqChan with ok check, mergedOutput) match actual `Run()` implementation | ✅ |
| **Merged output channel** — sessionOutput{id, data} with nil-data EOF sentinel, exactly as described | ✅ |
| **Shutdown Path 1 (ctx.Done)** — shutdownSessions → closeReqChan → return ctx.Err() → deferred readerCancel, eventBus.Close, close(done) | ✅ |
| **Shutdown Path 2 (Close)** — closeReqChan → <-done; worker sees !ok → shutdownSessions → return nil → same defers | ✅ |
| **shutdownSessions** — descending ID sort (sortSessionIDs), readerCancel is defer in Run not called inside shutdownSessions, ONE readerCtx for all sessions | ✅ |
| **Passthrough flow** — stdin goroutine bypasses worker (writes directly via activeWriter), output tee goes through worker (passthroughWriter in handleSessionOutput before VTerm.Write) | ✅ |
| **PassthroughConfig struct** — all 9 fields (Stdin, Stdout, TermFd, ToggleKey, TermState, BlockingGuard, StatusBar, ResizeFn, RestoreScreen) match actual definition in capture.go:508 | ✅ |
| **ExitReason constants** — ExitToggle, ExitChildExit, ExitContext, ExitError in correct iota order (side.go) | ✅ |
| **Concurrency model table** — 6 rows match actual goroutine architecture | ✅ |
| **Mutex usage table** — EventBus.subscribers is the only mutex, confirmed sole `sync.Mutex` in package (excluding CaptureSession which is separate) | ✅ |
| **Functional option names** — WithTermSize, WithRequestBuffer, WithMergedOutputBuffer all exist in manager.go with correct signatures | ✅ |
| **Event.Data** — doc says "always nil", confirmed: only `emit()` calls `Publish()` in production code, and emit() never sets Data | ✅ |
| **EventBus mechanics** — mutex held for entire Publish delivery loop, non-blocking sends with select/default, Subscribe default bufSize 64 | ✅ |
| **Package-level function** — `filterMouseForStatusBar` correctly identified as package-level in sgrmouse.go (not qualified with `termmux.`) | ✅ |
| **ScreenSnapshot fields** — Gen, PlainText, ANSI, FullScreen, Rows, Cols, Timestamp — all match, including VTerm method names (String, ContentANSI, RenderFullScreen) | ✅ |
| **SessionInfo fields** — ID, Target, State, IsActive — match exactly | ✅ |
| **Request/Reply types** — request{kind, payload, reply}, response{value, err} — match exactly | ✅ |
| **requestKind constants** — all 12 constants (reqRegister through reqDisablePassthroughTee) match actual code | ✅ |
| **InteractiveSession interface** — Write, Resize, Close, Done, Reader — all 5 methods match | ✅ |
| **Session output pipeline** — PTY → BufferedReader (cap 16 confirmed) → per-session goroutine → mergedOutput (cap 64) → worker → VTerm + snapshot + event | ✅ |
| **Migration table** — nil-safe Snapshot usage `if snap := mgr.Snapshot(id); snap != nil { snap.ANSI }` correctly handles nil return | ✅ |
| **API Design Principles §4** — correctly uses `ManagerOption` (line 645), matching source | ✅ |
| **NewSessionManager defaults** — reqChan cap 64, mergedOutput cap 64, nextID 1, termRows 24, termCols 80 — all match | ✅ |
