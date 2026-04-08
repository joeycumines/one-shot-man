# Task 63 Review — Session Persistence Across pr-split Restarts

**Verdict: PASS**

## Summary

The implementation correctly introduces session persistence with: atomic file I/O, thread-safe state export via the worker channel, platform-specific PID liveness checks, deep-copied export config, JS-side auto-save on lifecycle events, and defer-based cleanup on normal exit. The architecture respects the termmux extractability constraint (no upward imports), the single-worker-goroutine concurrency model, and the Go-modules-for-reuse / JS-for-app-logic split. Two minor inconsistencies (noted below) are not blockers.

## Detailed Findings

### 1. Thread Safety — ExportState via Worker Channel ✅

`ExportState()` calls `m.sendRequest(reqExportState, nil)` → worker dispatches `handleExportState()` which reads `m.sessions`, `m.activeID`, `m.termRows`, `m.termCols` — all worker-owned fields. The `sendRequest` path (line 483 of manager.go) blocks on the reply channel and handles worker-not-running / context cancellation. **No concurrent access to worker state.**

### 2. Atomic Writes ✅

`SaveManagerState` writes to `path + ".tmp"` then `os.Rename(tmp, path)`. On rename failure, the tmp file is cleaned up. Go's `os.Rename` on Windows uses `MoveFileEx` with `MOVEFILE_REPLACE_EXISTING`, so overwriting an existing file works on all platforms. Single-threaded JS prevents concurrent saves from racing on the temp path.

### 3. Cross-Platform PID Checks ✅

- **Unix** (`persistence_unix.go`): `os.FindProcess(pid)` + `Signal(syscall.Signal(0))`. Standard idiom. Note: returns false for processes owned by other users (EPERM), but this is correct for the use case (own child processes).
- **Windows** (`persistence_windows.go`): `syscall.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))` + `CloseHandle`. Correct — handle is always closed on success.
- **Both**: `ProcessAlive` guards `pid <= 0` before dispatching.
- **Tests**: `TestProcessAlive_Self` (own PID) and `TestProcessAlive_Invalid` (0, -1, large PID) cover the critical paths.

### 4. Deep Copy in ExportConfig ✅

`CaptureSession.ExportConfig()` copies the `cfg` struct by value, then explicitly allocates new slices/maps for `Args` and `Env`. The test (`TestCaptureSession_ExportConfig`) mutates the exported copy and verifies the original is unchanged. All reference-type fields in `CaptureConfig` (`Args []string`, `Env map[string]string`) are covered — the remaining fields are value types.

### 5. JS Chunk Guard ✅

The IIFE checks `typeof tuiMux === 'undefined' || !tuiMux` and returns immediately. In test environments without tuiMux, the chunk is inert. The secondary guard on `prSplitConfig.persistStatePath` prevents auto-save registration when no path is configured.

### 6. Cleanup Logic ✅

In `pr_split.go` Execute (line ~327):
```go
defer termmux.RemoveManagerState(stateFile)
```
This runs on **all** normal function returns (success or error). Only actual crashes (SIGKILL, unrecovered panics) leave the file for the next startup to detect. This matches the stated design: "Crash exits leave the file in place intentionally — that's the resume case." The JS-side `cleanupStateFile()` is separately available for explicit cleanup from script logic.

### 7. Error Handling ✅

- `SaveManagerState`: nil state → error. MkdirAll, MarshalIndent, WriteFile, Rename → all wrapped with `fmt.Errorf` and `%w`.
- `LoadManagerState`: missing file → `(nil, nil)`. Corrupt JSON → error. Wrong version → error.
- `RemoveManagerState`: missing file → nil (intentional). Real errors → propagated.
- JS bindings: all errors panic with `runtime.NewGoError(err)` or `runtime.NewTypeError(...)`, which is the correct Goja error propagation pattern.
- JS-side `_persistState`: catches and logs errors — persistence is explicitly best-effort to avoid disrupting the TUI.

### 8. Naming Conventions ✅

No prepositions in method names: `ExportState`, `ExportConfig`, `SaveManagerState`, `LoadManagerState`, `RemoveManagerState`, `ProcessAlive`. Structured logging uses lowercase, punctuation-free messages with camelCase key-value attributes (verified in both Go and JS).

### 9. Termmux Extractability ✅

`persistence.go` imports only stdlib (`encoding/json`, `errors`, `fmt`, `os`, `path/filepath`, `time`). No imports from `internal/builtin`, `internal/command`, or any other internal package. The platform files import only `os`+`syscall` (Unix) and `syscall` (Windows).

---

## Minor Findings (Non-Blocking)

### M1. `removeState` Missing Empty Path Validation

`saveState` and `loadState` both validate `path == ""` and panic with a TypeError. `removeState` (module.go line 1401) checks argument count but NOT empty string. Functionally harmless (`RemoveManagerState("")` → `os.Remove("")` → ENOENT → swallowed), but inconsistent with sibling bindings.

### M2. `SessionTarget` Lacks JSON Tags

`SessionTarget` fields (`ID`, `Name`, `Kind`) have no `json:"..."` tags. When serialized inside `PersistedSession.Target`, the on-disk JSON uses Go PascalCase (`"ID"`, `"Name"`, `"Kind"`) while every other field in the schema uses camelCase. Round-trips work because `json.Unmarshal` is case-insensitive, and the JS layer uses `persistedStateToJS` (which manually maps to lowercase). However, the on-disk format is inconsistent with the rest of the schema, which could confuse external tooling or manual inspection.

### M3. Round-Trip Test Doesn't Assert Time Fields

`TestSaveLoadManagerState_RoundTrip` verifies Version, ActiveID, dims, and all session struct fields — but never compares `SavedAt` or `LastActive` after deserialization. Not a correctness risk (Go's `time.Time` JSON encoding is well-tested), but a coverage gap that could mask future regressions if the time format changes.

---

## Acceptance Criteria Trace

| Criterion | Evidence |
|---|---|
| Detect previous session on startup, offer resume | `loadPreviousState()` in JS chunk loads state, annotates PID liveness, stores on `prSplit.previousState` |
| Re-attach live processes | `s.alive = tuiMux.processAlive(s.pid)` annotates each session; TUI can inspect and re-attach |
| Restart dead processes | Restart config exported via `ExportConfig()` → `command`, `args`, `dir`, `env` available in state |
| Clean up on clean exit | Go `defer termmux.RemoveManagerState(stateFile)` + JS `prSplit.persistence.cleanup()` |
