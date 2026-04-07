# Task 42 — Review Pass 1f

**Reviewer:** Takumi (exhaustive source-code cross-reference)  
**Date:** 2026-04-08  
**Document:** `scratch/termmux-architecture.md`  
**Source files checked:** manager.go, session.go, capture.go, eventbus.go, passthrough.go, sgrmouse.go, side.go, config.go, errors.go, resize_unix.go, resize_windows.go, write.go

---

## Verdict: **PASS**

---

## Key Items Verified

### 1. Method signatures in overview box

All 12 public methods verified against source:

| Method | Doc Signature | Source Match |
|--------|--------------|-------------|
| `Register(session, target) → (SessionID, error)` | ✓ | manager.go:486 |
| `Unregister(id) → error` | ✓ | manager.go:501 |
| `Activate(id) → error` | ✓ | manager.go:506 |
| `Input(data) → error` | ✓ | manager.go:511 |
| `Resize(rows, cols) → error` | ✓ | manager.go:516 |
| `Snapshot(id) → *ScreenSnapshot` | ✓ (nil-safe) | manager.go:521 |
| `ActiveID() → SessionID` | ✓ | manager.go:530 |
| `Sessions() → []SessionInfo` | ✓ | manager.go:538 |
| `Subscribe(bufSize) → (int, <-chan Event)` | ✓ | manager.go:458 |
| `Unsubscribe(id) → bool` | ✓ | manager.go:464 |
| `Close()` | ✓ | manager.go:435 |
| `Started() → <-chan struct{}` | ✓ | manager.go:445 |
| `Run(ctx) → error` | ✓ | manager.go:404 |

### 2. Struct field listings

**ScreenSnapshot** — all 7 fields match source (Gen, PlainText, ANSI, FullScreen, Rows, Cols, Timestamp). ✓  
**SessionInfo** — all 4 fields match source (ID, Target, State, IsActive). ✓  
**managedSession** — all 7 fields match source (session, vterm, snapshot, state, target, lastActive, passthroughWriter). ✓  
**Worker state fields** — all 7 match (sessions, activeID, nextID, termRows, termCols, snapshotGen, passthroughSessionID). ✓  
**SessionManager struct** — reqChan, mergedOutput, eventBus, done, started, readerCtx, readerCancel all confirmed. ✓  
**PassthroughConfig** — all 9 fields match source in capture.go:508–544. ✓

### 3. State transitions

Verified against `validTransition()` (manager.go:66–80) AND the bypass paths:

- **Created → Running**: `validTransition` allows it; `handleSessionOutput` triggers on first output (manager.go:764). ✓  
- **Running → Exited**: `validTransition` allows it; `handleSessionOutput` EOF path (manager.go:741–743). ✓  
- **Exited → Closed**: `validTransition` allows it; `handleUnregister` and `shutdownSessions`. ✓  
- **Created → Closed**: `validTransition` allows it; `handleUnregister` (no validTransition check, line 642–652) and `handleSessionOutput` EOF for Created (manager.go:744–753). ✓  
- **Running → Closed**: `validTransition` does NOT allow this (line 71: `return next == SessionExited`). Doc correctly states "bypasses Exited — validTransition not checked." Confirmed in `handleUnregister` (line 647: `ms.state = SessionClosed` for any non-Closed state) and `shutdownSessions` (line 959: same pattern). ✓

### 4. Worker select loop pseudo-code

Doc pseudo-code matches source `Run()` (manager.go:404–430) exactly:
- `ctx.Done()` → `shutdownSessions()` + `closeReqChan()` + `return ctx.Err()` ✓
- `reqChan` closed (`!ok`) → `shutdownSessions()` + `return nil` ✓
- `mergedOutput` → `handleSessionOutput(so)` ✓
- Deferred order (LIFO): `readerCancel()`, `eventBus.Close()`, `close(m.done)` ✓

### 5. Shutdown — both paths

**Path 1 (ctx.Done):** Matches source exactly. ✓  
**Path 2 (Close):** `closeReqChan()` → `<-m.done` matches source (manager.go:435–438). ✓  
**`shutdownSessions()`:** Descending ID sort confirmed via `sortSessionIDs` insertion sort with `ids[j] > ids[j-1]` (manager.go:973). ✓  
**`readerCancel()` timing:** Deferred in `Run()`, fires AFTER `shutdownSessions()` returns. NOT called inside `shutdownSessions()`. ✓  
**ONE readerCtx:** Confirmed — single `context.WithCancel(ctx)` in `Run()`, shared by all reader goroutines. ✓

### 6. Passthrough mode

- **stdin goroutine:** Writes directly to `w` (activeWriter result) bypassing reqInput. Confirmed in passthrough.go:143–196. ✓  
- **Worker tee:** `passthroughWriter` written BEFORE `VTerm.Write()` in `handleSessionOutput` (manager.go:772–776 then 779). ✓  
- **No separate PTY→stdout goroutine:** Confirmed — tee is inline in worker's `handleSessionOutput`. ✓  
- **enablePassthroughTee:** Uses `passthroughTeePayload{w, id}` to set `passthroughWriter` on the specific session (manager.go:884–893). ✓  
- **ExitReason enum:** All 4 values match source side.go:8–17. ✓

### 7. Concurrency table entries

All 6 rows verified:
- Worker select on reqChan/mergedOutput/eventBus ✓
- Per-session reader goroutines (startReaderGoroutine) ✓
- Passthrough stdin→PTY bypasses worker ✓
- Passthrough PTY→stdout is worker tee (atomic.Pointer load, inline in handleSessionOutput) ✓
- EventBus non-blocking send ✓
- Mutex usage table: only EventBus.subscribers ✓

### 8. Functional option names (ManagerOption type)

- `type ManagerOption func(*SessionManager)` ✓ (manager.go:334)
- `WithTermSize(rows, cols int) ManagerOption` ✓ (manager.go:337)
- `WithRequestBuffer(cap int) ManagerOption` ✓ (manager.go:344)
- `WithMergedOutputBuffer(cap int) ManagerOption` ✓ (manager.go:350)

### 9. Event.Data — "always nil"

Doc says: "Currently unused — always nil."  
Source `emit()` (eventbus.go:204–210) constructs `Event{Kind, SessionID, Time}` — Data is never set, defaults to nil. ✓  
Note: source Event godoc claims Data carries payloads (e.g., `[]byte` for Output, `[2]int` for Resize) but implementation never populates them. Doc accurately reflects actual behavior.

### 10. Migration table — nil-safe Snapshot

Doc: `if snap := mgr.Snapshot(id); snap != nil { snap.ANSI }`.  
Source `handleSnapshot` returns `response{}` (nil value) when session not found (manager.go:710–712). `Snapshot()` correctly returns nil when `resp.value == nil` (manager.go:523). ✓

### 11. Package-level function names

- `filterMouseForStatusBar` — sgrmouse.go:144 (unexported, package-level) ✓
- `writeOrLog` — write.go:11 ✓
- `sortSessionIDs` — manager.go:969 ✓
- `waitForReader` — manager.go:837 ✓
- `watchResize` — resize_unix.go:15 / resize_windows.go:8 ✓

### 12. Interface definitions (InteractiveSession)

All 5 methods match source session.go:96–119:
- `Write([]byte) (int, error)` ✓
- `Resize(rows, cols int) error` ✓
- `Close() error` ✓
- `Done() <-chan struct{}` ✓
- `Reader() <-chan []byte` ✓

---

## Observation (non-blocking, informational)

The overview box header says "Methods (each sends request, awaits reply)" but `Subscribe`, `Unsubscribe`, `Close`, and `Started` bypass request/reply entirely — `Subscribe`/`Unsubscribe` call `eventBus` directly (mutex-protected), `Close` closes reqChan, `Started` returns a channel. The **detailed sections** of the doc correctly describe each mechanism (EventBus section, Shutdown section). This is a grouping simplification in the overview box, not a factual error in the detailed architecture. The accurate information IS present in the document; only the overview header is slightly overgeneralized.

Not flagging as FAIL because: (a) the correct behavior IS documented in the detailed sections, (b) the overview box is a summary diagram meant to list all public API methods in one place, and (c) no implementer reading the full document would be misled since the Shutdown and EventBus sections are unambiguous.
