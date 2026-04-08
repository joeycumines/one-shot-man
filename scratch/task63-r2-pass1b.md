# Task 63 Review — R2 Pass 1b

**Verdict: PASS**

## Summary

The diff implements a clean, well-layered session persistence infrastructure for pr-split. Go provides reusable types (`PersistedManagerState`, `PersistedSession`), atomic file I/O (`SaveManagerState`/`LoadManagerState`/`RemoveManagerState`), platform-specific PID liveness detection, and a thread-safe `ExportState` method dispatched through the worker goroutine. JS provides the pr-split-specific orchestration (event-driven auto-save, startup resume detection, clean exit). Tests cover round-trip serialization, corruption, version mismatch, nil state, atomic write integrity, PID validity, `ExportConfig` deep copy, and a live `ExportState` integration test. The two prior-review fixes (M1, M3) are verified correct.

## Prior Fix Verification

- **M1** (`removeState` empty-path validation): Confirmed at module.go:1401-1412 — checks both missing argument and empty string, consistent with `saveState` and `loadState`. ✓
- **M3** (round-trip time field assertions): Confirmed `SavedAt` and `LastActive` are set with `Truncate(time.Millisecond)`, asserted via `.Equal()`. ✓

## Detailed Findings

### Correctness — Verified Clean

1. **Thread safety**: `ExportState` sends `reqExportState` through the worker's request channel; `handleExportState` runs exclusively on the worker goroutine and reads only worker-owned fields (`m.sessions`, `m.activeID`, `m.termRows`, `m.termCols`). No concurrent mutation possible.

2. **Interface dispatch**: `SessionPIDProvider` and `SessionConfigProvider` are optional interfaces probed via type assertions. `CaptureSession` implements both; the mock `controllableSession` implements neither. Both paths are tested (integration test for mock, `ExportConfig` test for `CaptureSession`).

3. **Deep copy in `ExportConfig`**: Args slice and Env map are independently copied. Mutation test in `TestCaptureSession_ExportConfig` proves no aliasing.

4. **Atomic write**: temp file + rename. Go's `os.Rename` on Windows uses `MoveFileEx` with `MOVEFILE_REPLACE_EXISTING`, so overwriting the target works cross-platform. The test verifies no `.tmp` file remains.

5. **Platform-specific `processAlive`**: Unix uses signal-0 (correct and standard); Windows uses `OpenProcess` with `PROCESS_QUERY_LIMITED_INFORMATION` (correct — signal-0 doesn't exist on Windows). Both have correct build tags. `ProcessAlive` guards `pid <= 0` before dispatch.

6. **Version check**: `LoadManagerState` rejects `state.Version != persistenceVersion`. `TestLoadManagerState_WrongVersion` covers this.

7. **Cleanup ordering**: Go defer `RemoveManagerState` runs after the TUI wizard exits (after `ExecuteScript` returns). JS auto-save event listeners fire *during* the wizard's lifetime on the Goja goroutine. No race: JS stops before the Go defer runs.

8. **JS event names**: `SAVE_EVENTS = ['registered', 'activated', 'exit', 'closed']` — all present in `validEvents` map. The `on()` binding validates event names, so a typo would be caught at runtime.

9. **JS guard clauses**: The persistence chunk early-returns if `tuiMux` is undefined or `statePath` is empty. This correctly disables persistence when the session directory is unavailable.

10. **`persistedStateToJS` conversion**: Times converted to `UnixMilli()` (appropriate for JS `Date.now()` semantics). `SessionState` converted to `int(s.State)` (stable iota ordering). Optional fields (Command, Args, Dir, Env) only included when non-zero, keeping the JS object sparse.

### Observations — Not Blockers

1. **Non-deterministic session ordering**: `handleExportState` iterates `m.sessions` (a map), so the `Sessions` slice order is non-deterministic. No current consumer depends on ordering (JS uses `sessionId` to identify sessions), but if future code assumes the first element is the "primary" session, it would be wrong. Acceptable for infrastructure — consumers should use `sessionId`-based lookup.

2. **Duplicate `storage.SessionDirectory()` call**: Called once in `setupEngineGlobals` (for JS config) and once in `run()` (for Go cleanup defer). Same deterministic value but duplicated logic. Not a correctness issue.

3. **`processAlive` EPERM behavior**: On Unix, signal-0 to a different-user process returns EPERM → `processAlive` returns false. Correct for this use case (child processes are always same-user). The godoc comment doesn't mention this subtlety but the current callers are unaffected.

4. **PID 4_000_000 test**: Technically reachable on 64-bit Linux (`/proc/sys/kernel/pid_max` defaults to 4194304). The test uses `t.Log` (not `t.Error`) for this case — correct handling.

### Acceptance Criteria Coverage

| AC | Status | Notes |
|----|--------|-------|
| Detect previous session on startup | ✓ | `loadPreviousState()` → `prSplit.previousState` |
| Live process re-attachment | Infrastructure ✓ | PID liveness detection provided; actual PTY re-attach is a TUI-layer concern beyond this task's scope |
| Dead process restart | Infrastructure ✓ | Command/Args/Dir/Env persisted and loadable; actual restart orchestration is TUI-layer |
| State file cleanup on clean exit | ✓ | Go defer + JS cleanup function (idempotent, handles missing file) |

### Architecture Compliance

- termmux package has no imports from `internal/builtin` or `internal/command` ✓
- All mutable state in `handleExportState` accessed on worker goroutine ✓
- No prepositions in method names (`ExportState`, `ExportConfig`, `ProcessAlive`, `SaveManagerState`, etc.) ✓
- Structured logging in JS: lowercase event-phrased messages, camelCase attributes ✓
- Go as reusable module / JS for app-specific logic boundary maintained ✓
