# Task 63 Review — Session Persistence Across pr-split Restarts

**Verdict: PASS**

## Summary

The implementation is correct, well-structured, and follows project conventions. Go provides reusable persistence primitives (save/load/remove/export/processAlive); JS wires the app-specific orchestration (auto-save on events, startup resume detection, clean-exit cleanup). All mutable SessionManager state is accessed exclusively via the worker goroutine through `reqExportState`. The termmux package has zero imports from `internal/builtin` or `internal/command` — extractability preserved. Tests are comprehensive with appropriate `testing.Short()` guards. Build, test-short, and lint all pass.

## Detailed Findings

### Correctness — Verified ✓

1. **Thread safety**: `handleExportState` runs on the worker goroutine and accesses only worker-owned fields (`m.sessions`, `m.activeID`, `m.termRows`, `m.termCols`). The `CaptureSession.Pid()` and `ExportConfig()` methods each acquire their own mutex. No data races.

2. **Data round-trip**: `SaveManagerState` → JSON → `LoadManagerState` preserves all fields. Tested by `TestSaveLoadManagerState_RoundTrip` with truncated timestamps (nanosecond loss acknowledged). Version check correctly rejects unknown versions.

3. **Atomic write**: Temp file + rename strategy prevents partial reads. On rename failure, the temp file is cleaned up. `RemoveManagerState` tolerates `ErrNotExist`. On Windows, Go's `os.Rename` uses `MoveFileEx(MOVEFILE_REPLACE_EXISTING)` — not truly atomic on NTFS, but acceptable for best-effort state resume.

4. **Platform-specific PID checks**: Unix uses signal-0 (standard POSIX liveness probe). Windows uses `OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION)` — correct and minimal-privilege. Both gated by `ProcessAlive(pid <= 0) → false`.

5. **Deep copy in `ExportConfig`**: Args slice and Env map are deep-copied. Test verifies mutation isolation.

6. **Clean exit**: Go `defer termmux.RemoveManagerState(stateFile)` in `Execute()` runs on normal return, error return, and panic. Crash exits (SIGKILL, segfault) skip the defer — intentional, enables resume. SIGINT/SIGTERM cancel context, function returns, defer runs — correct "graceful = clean" semantics. JS `cleanupStateFile()` also calls `removeState()` — double removal is idempotent (`ErrNotExist` swallowed).

7. **Path consistency**: Both the Go defer and the JS config compute `filepath.Join(storage.SessionDirectory(), "pr-split-mux.state.json")`. `SessionDirectory()` is deterministic. Paths match.

8. **JS bindings**: `saveState`, `loadState`, `removeState`, `processAlive`, `exportState` — all validate arguments (nil/empty checks), propagate errors via `panic(runtime.NewGoError(err))` (Goja convention), and correctly use `parent.` for standalone functions vs `mgr.` for worker-bound methods. `loadState` calls `parent.LoadManagerState` (file I/O, no manager needed) — safe to call before TUI starts.

9. **`persistedStateToJS`**: Correctly maps Go struct to JS-friendly `map[string]any` with camelCase keys. Conditionally includes `command`, `args`, `dir`, `env` only when populated.

### Observations — Non-blocking

1. **`SessionTarget` lacks JSON tags**: `SessionTarget` fields serialize as PascalCase (`"ID"`, `"Name"`, `"Kind"`) while `PersistedSession` fields use camelCase (`"sessionId"`, `"pid"`). The on-disk JSON has mixed casing. Round-trip works correctly (marshal and unmarshal use the same Go type). Purely a style inconsistency. Not introduced by this diff — `SessionTarget` is pre-existing.

2. **No compile-time interface assertions**: `CaptureSession` satisfies `SessionPIDProvider` and `SessionConfigProvider` at runtime via type assertion, but there's no `var _ SessionPIDProvider = (*CaptureSession)(nil)` guard. If someone later changes the method signature, the break would be silent (returns 0/empty rather than compile error). Consider adding.

3. **Non-deterministic session ordering**: `handleExportState` iterates `m.sessions` (a `map`), so `state.Sessions` array order varies between invocations. Not a correctness issue — sessions are identified by `sessionId` — but makes state file diffs noisy. Could sort by SessionID if stable output is desired.

4. **PID reuse**: `ProcessAlive` can return true for a different process that reused the old PID. This is a fundamental OS limitation. The JS correctly uses `alive` as a hint, not a guarantee. Appropriate for the resume UX (user confirms before re-attach).

5. **Test coverage gap**: Round-trip and integration tests don't exercise `SessionTarget.ID` (always empty). Consider adding a session with a non-empty `ID` to the round-trip fixture.

6. **`SessionState` serialized as integer**: Both the JSON file and JS objects represent state as an integer (0–3). Human-readable state names in the file would aid debugging. Low priority.

### Architecture Alignment ✓

- **Go as reusable modules**: `persistence.go` is a pure-Go, self-contained module. Zero awareness of pr-split, Claude, or TUI. Could be used by any termmux consumer.
- **JS for app-specific logic**: `pr_split_16g_persistence.js` handles the pr-split-specific wiring (event subscriptions, prSplit namespace, auto-load on init).
- **Worker goroutine discipline**: `ExportState()` → `sendRequest(reqExportState)` → worker processes `handleExportState()`. No direct field access from external goroutines.
- **No prepositions in method names**: `ExportState`, `ExportConfig`, `ProcessAlive`, `SaveManagerState`, `LoadManagerState`, `RemoveManagerState`. ✓
- **Structured logging**: JS uses `log.debug(msg, {attrs})` and `log.warn(msg, {attrs})` throughout. Messages are lowercase, punctuation-free, event-phrased. ✓
- **Termmux extractability**: persistence.go imports only stdlib. persistence_unix.go and persistence_windows.go import only stdlib + `os`/`syscall`. ✓
