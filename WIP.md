# WIP: T078 Final Verification — PTY Deadlock Fix

## Status
Session started 2026-02-27 09:18. T051-T077 all Done.
**T078 COMPLETE** — `make all` passes clean (exit 0).

### PTY Integration Test Deadlock Fix
Root cause: In JS `cleanupExecutor()`, `tuiMux.detach()` was called BEFORE
`claudeExecutor.close()`. `Detach()` blocks on `<-bgReaderDone`, but the
background reader is stuck in `child.Read()` because the child PTY hasn't
been closed yet → deadlock.

**Files modified for the fix:**
1. **internal/command/pr_split_script.js** — Reordered `cleanupExecutor()`:
   close executor FIRST, then detach mux.
2. **internal/termui/mux/mux.go** — Two changes:
   a. `Detach()`: timeout-based wait (3s) instead of indefinite block.
      Nils bgReaderDone/bgChildEOF so re-attachment works clean.
   b. `backgroundReader()`: Takes channels as args from Attach() instead of
      re-reading under lock (fixes close-of-nil-channel panic when Detach
      nils channels before goroutine captures refs). Also checks `vt == nil`
      as signal to stop forwarding after Detach.
3. **internal/termui/mux/mux_test.go** — Fixed `TestBackgroundReader_PreventStarvation`
   timing: push data AFTER passthrough is active, not before.

### Completed This Session (all tasks)
T051-T078 all Done. 121 VTerm tests, 193+ mux tests, all PTY integration tests pass.
`make all` (build + vet + staticcheck + deadcode + tests) = exit 0.

## Next Steps
- Rule of Two verification
- Git commit
