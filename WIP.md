# WIP: T038-T045 COMPLETE + Security Hardening — Ready for Commit

## Status
ALL CORE FIXES IMPLEMENTED, VERIFIED, AND RULE OF TWO PASSED.

### Completed Tasks
- T038-T039: EAGAIN fix (ensureBlockingFd + retry) + tests
- T040-T042: getOrCreateSession replaces all 13 "session not found" blocks
- T043: sendToHandle 50ms delay between text and Enter writes
- T044: Existing sendToHandle tests still pass (no change needed)
- T045: TestMCPServer_AutoCreateSession_E2E added — full lifecycle test
- Security hardening: getOrCreateSession validates session IDs via mcpValidateSessionID

### Rule of Two Results
- Pass 1: PASS — scratch/review-pass-1-reset.md
- Pass 2: PASS — scratch/review-pass-2-reset.md
- All 13 getOrCreateSession call sites verified for mutex safety
- `make` (build + lint + test) passes clean

## Files Modified
- internal/termui/mux/mux.go — ensureBlockingFd + EAGAIN retry
- internal/termui/mux/mux_blocking_unix.go — NEW
- internal/termui/mux/mux_blocking_other.go — NEW
- internal/termui/mux/mux_blocking_unix_test.go — NEW
- internal/termui/mux/mux_test.go — EAGAIN retry test
- internal/command/mcp.go — getOrCreateSession with validation + 13 call sites
- internal/command/mcp_test.go — 10 tests updated + 1 new E2E test
- internal/command/mcp_security_test.go — 1 test updated
- internal/command/pr_split_script.js — 50ms ENTER_DELAY_MS

## Remaining Tasks (T046-T053)
T046-T047: More integration tests (PTY + real Claude)
T048-T049: VTerm virtual terminal buffer
T050: ReleaseTerminal/RestoreTerminal race
T051-T053: Various verification and cleanup
