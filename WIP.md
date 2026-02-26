# WIP: Fix auto-split claude integration

## Root Cause Identified

**MUTEX DEADLOCK in `internal/builtin/pty/pty.go`**

`Process.Write()` holds `p.mu` (mutex) during a blocking `p.ptyFile.Write()` kernel call.
When the PTY buffer fills (Claude hasn't started reading stdin yet), the write blocks WHILE HOLDING THE LOCK.

Then `Process.Signal()` tries to acquire the same lock → DEADLOCK.
`Process.Close()` also tries to acquire the same lock → DEADLOCK.

This cascades through:
- `prSplitSendWithCancel` calls `kill()` → `Signal("SIGKILL")` → deadlock
- Cancel flag is detected but signal can't be delivered
- Force cancel same issue
- `cleanupExecutor()` calls `handle.close()` → `proc.Close()` → deadlock
- Process becomes unkillable from within (external SIGKILL still works)

## Fix Plan

1. **Fix `pty.Process.Write()`** — Release mutex before blocking I/O (same pattern as Read())
2. **Add timeout guard to `prSplitSendWithCancel`** — Don't block on `<-done` forever after kill
3. **Write deadlock reproduction test** — `TestPTY_WriteSignalDeadlock`
4. **Write termtest PTY integration test** — Full auto-split with mock Claude
5. **Fix other issues** — double help, signal forwarding, cleanupExecutor robustness

## Files to Modify

- `internal/builtin/pty/pty.go` — Fix Write() mutex pattern
- `internal/command/pr_split.go` — Add timeout to sendWithCancel
- `internal/builtin/pty/pty_test.go` — Add deadlock regression test
- `internal/command/pr_split_integration_test.go` — Add termtest integration tests

## Current Step

Writing deadlock reproduction test + fix.
