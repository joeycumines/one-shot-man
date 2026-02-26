# WIP: Fix auto-split claude integration — COMPLETE

## Root Cause: FIXED

**MUTEX DEADLOCK in `internal/builtin/pty/pty.go`** — Process.Write() held `p.mu` during blocking
`p.ptyFile.Write()` kernel call. When PTY buffer fills, Write blocks WHILE HOLDING THE LOCK.
Signal/Close acquire same lock → deadlock. Fixed by releasing lock before I/O (matching Read() pattern).

## Changes Made

### T026: PTY deadlock fix (`internal/builtin/pty/pty.go`)
- `Write()`: Save `p.ptyFile` under lock, release, then do I/O
- `Close()`: Added fallback `syscall.Kill(pid, SIGKILL)` for zombie processes
- Tests: `TestProcess_WriteSignalDeadlock`, `TestProcess_CloseWhileWriteBlocked`

### T027: Timeout guard (`internal/command/pr_split.go`)
- `prSplitSendWithCancel`: 10s timeout on `<-done` after `kill()` — never blocks forever

### T028: Termtest integration tests (`internal/command/pr_split_pty_unix_test.go`)
- `TestPTY_AutoSplit_SendBlockedCancelWorks`: Direct PTY test of cancel during blocked write
- `TestPTY_AutoSplit_EndToEnd`: Full osm binary + mock Claude + auto-split TUI
- `TestPTY_AutoSplit_CancelDuringBlockedSend`: Deadlock regression with real TUI
- Fix: Use `WriteString("q")` not `Send("q")` — Send is for named keys only
- Added `test-pr-split-pty` make target in config.mk

## Verification
- `make` (build + lint + test): ALL PASS, exit code 0
- PTY tests: 6/6 pass (27s)
- Full suite: 0 failures across all packages

## Remaining
- T018: Complex Go project AI integration test (not started)
