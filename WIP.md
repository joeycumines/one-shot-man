# WIP — Session (T001-T005)

## Current State

- **Blueprint.json**: Rewritten with new task numbering T001-T168
- **T001**: FIXED — Added `sync.RWMutex` guarding function-variable path overrides in storage/paths.go. Created `getSessionDirectory()`, `getSessionFilePath()`, `getSessionLockFilePath()` accessors. Updated all production call sites (inspector.go, cleanup.go, paths.go, global_metadata_test.go).
- **T002**: FIXED — Replaced `os.Readlink(dir)` with `filepath.EvalSymlinks(dir)` in pty_test.go TestSpawn_WorkingDirectory.
- **T002 bonus**: FIXED — TestSpawn_EchoHello was also failing (empty output from fast-exiting PTY command). Changed test to `proc.Wait()` first, then drain PTY output.
- **T003**: Verified NOT broken — TestPRSplit_CleanupBranches passes 3x with -race.
- **Bubbletea race**: FIXED — Added 50ms sleep before slaveForBT.Close() in TestRunProgram_Lifecycle to let bubbletea's handleResize goroutine finish.
- **Goal autodiscovery test leak**: FIXED (prior session) — Added `goal.autodiscovery=false` to test registries.

## Files Changed

- `blueprint.json` — Full rewrite
- `internal/storage/paths.go` — RWMutex + accessor functions
- `internal/storage/inspector.go` — Use `getSessionDirectory()`, `getSessionLockFilePath()`
- `internal/storage/cleanup.go` — Use `getSessionDirectory()`
- `internal/storage/global_metadata_test.go` — Use `getSessionDirectory()`
- `internal/builtin/pty/pty_test.go` — EvalSymlinks + EchoHello fix
- `internal/builtin/bubbletea/run_program_test.go` — Race fix
- `internal/command/goal_test.go` — autodiscovery=false (prior session)
- `internal/command/completion_command_test.go` — autodiscovery=false (prior session)
- `config.mk` — Updated run-test target

## Immediate Next Step

1. Run `make make-all-with-log` — expect all green
2. Rule of Two verification
3. Commit
4. Start T011: Eventloop migration
