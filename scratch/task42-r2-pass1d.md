# Task 42 — Review Pass 1d: termmux-architecture.md

**Reviewer:** Takumi (review subagent)
**Date:** 2026-04-08
**Artifact:** `scratch/termmux-architecture.md`
**Source files cross-referenced:** `manager.go`, `session.go`, `capture.go`, `eventbus.go`, `passthrough.go`, `side.go`, `sgrmouse.go`, `errors.go`, `write.go`

---

## Verdict: **FAIL** — 2 findings (1 medium, 1 minor)

---

## Findings

### F1 (MEDIUM): `Snapshot()` return type is `*ScreenSnapshot`, not `ScreenSnapshot`

**Location:** Overview box (line ~99) and migration table (line ~483)

**Doc says:**
```
│     Snapshot(id) → ScreenSnapshot                               │
```
and:
```
4. `Mux.ChildScreen()` → `mgr.Snapshot(id).ANSI`
```

**Source says (manager.go:533):**
```go
func (m *SessionManager) Snapshot(id SessionID) *ScreenSnapshot {
    resp := m.sendRequest(reqSnapshot, id)
    if resp.value == nil {
        return nil
    }
    snap, _ := resp.value.(*ScreenSnapshot)
    return snap
}
```

**Impact:** The method returns a *pointer* that **can be nil** (when session doesn't exist or was removed). An implementer relying on the overview box signature would write:
```go
snap := mgr.Snapshot(id)
text := snap.PlainText // nil pointer dereference
```
The migration table line `mgr.Snapshot(id).ANSI` directly demonstrates a nil-deref pattern.

**Fix:** Change overview box to `Snapshot(id) → *ScreenSnapshot (nil if not found)` and change migration table to show a nil guard: `snap := mgr.Snapshot(id); if snap != nil { snap.ANSI }`.

---

### F2 (MINOR): `sgrmouse.filterMouseForStatusBar` — no such qualified name

**Location:** Line 448

**Doc says:**
> mouse event routing via `sgrmouse.filterMouseForStatusBar`

**Source says (sgrmouse.go:144):**
```go
func filterMouseForStatusBar(buf []byte, termRows, statusBarLines int) (out []byte, statusBarClicked bool) {
```

**Impact:** `filterMouseForStatusBar` is a package-level function in the `termmux` package, defined in file `sgrmouse.go`. The dot-qualified `sgrmouse.filterMouseForStatusBar` implies either a sub-package or a type receiver that does not exist. An implementer searching for this reference in the codebase would not find it.

**Fix:** Change to `filterMouseForStatusBar` (or `filterMouseForStatusBar in sgrmouse.go` for clarity).

---

## Verified Correct (Spot-Check Summary)

| Check | Result |
|-------|--------|
| Method signatures (Register, Unregister, Activate, Input, Resize, ActiveID, Sessions, Subscribe, Unsubscribe, Close, Started) | ✅ All match source |
| InteractiveSession interface (Write, Resize, Close, Done, Reader) | ✅ Exact match |
| SessionState enum (Created, Running, Exited, Closed) | ✅ Match source |
| State transitions (Created→Running, Running→Exited, Exited→Closed, Created→Closed) | ✅ Match validTransition() and handleSessionOutput() |
| ScreenSnapshot fields (Gen, PlainText, ANSI, FullScreen, Rows, Cols, Timestamp) | ✅ All match |
| SessionInfo fields (ID, Target, State, IsActive) | ✅ All match |
| Request kinds (reqRegister through reqDisablePassthroughTee) | ✅ All 12 kinds match source, correct order |
| EventKind enum (7 values: Registered, Activated, Output, Exited, Closed, Resize, Bell) | ✅ Match source |
| Event.Data "Currently unused — always nil" | ✅ Correct: `emit()` never sets Data |
| EventBus mutex semantics (non-blocking send, lock held during delivery) | ✅ Match Publish() source |
| Select loop pseudo-code (3 cases: ctx.Done, reqChan, mergedOutput) | ✅ Match Run() source |
| Shutdown Path 1 (ctx.Done → shutdownSessions → closeReqChan → return ctx.Err → deferred) | ✅ Match source |
| Shutdown Path 2 (Close → closeReqChan → blocks <-done → worker sees closed chan → shutdownSessions → return nil) | ✅ Match source |
| Deferred cleanup order (readerCancel → eventBus.Close → close(done)) — LIFO | ✅ Correct LIFO order |
| shutdownSessions descending ID sort | ✅ Match sortSessionIDs() |
| readerCancel is defer in Run(), NOT inside shutdownSessions() | ✅ Correct |
| One readerCtx for all sessions (not per-session) | ✅ Correct |
| Functional options: WithTermSize, WithRequestBuffer, WithMergedOutputBuffer | ✅ All three match source |
| managedSession field names in ASCII diagram | ✅ All 7 fields match source struct |
| Worker state field names in ASCII diagram | ✅ All 7 fields match source struct |
| SessionManager reqChan, eventBus, mergedOutput channels | ✅ Match source |
| Passthrough: stdin goroutine writes directly via activeWriter() bypassing worker | ✅ Match source |
| Passthrough: output tee via passthroughWriter happens BEFORE VTerm.Write in handleSessionOutput | ✅ Match source (lines 775-781 of manager.go) |
| Passthrough: ExitReason enum (ExitToggle, ExitChildExit, ExitContext, ExitError) | ✅ Match side.go |
| PassthroughConfig fields (Stdin, Stdout, TermFd, ToggleKey, TermState, BlockingGuard, StatusBar, ResizeFn, RestoreScreen) | ✅ All match source |
| Concurrency table entries (Worker, PTY Reader, stdin→PTY, PTY→stdout tee, EventBus, JS drainer) | ✅ Structurally accurate |
| Mutex usage table (only EventBus.subscribers) | ✅ No mutex in manager.go |
| EOF sentinel (nil data field in sessionOutput) | ✅ Match startReaderGoroutine and handleSessionOutput |
| Created→Closed on exit-without-output (/bin/true case) | ✅ Match handleSessionOutput source |

---

## Summary

27+ prior fixes have brought this document to near-perfect accuracy. Two findings remain:

1. **Snapshot return type** — the only signature in the overview box that omits pointer/nil semantics, directly impacting safe API usage patterns. The migration table compounds this by showing a nil-deref pattern as idiomatic.
2. **sgrmouse prefix** — a minor naming error in prose that could waste an implementer's time searching.

Both are quick fixes. After addressing these, the document achieves the stated acceptance criterion.
