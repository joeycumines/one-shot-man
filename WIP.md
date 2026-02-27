# WIP — Takumi's State Dump

## Session
- Branch: `wip` (tracking origin/wip)
- Last commit: `10f7d91` — "Replace internal/termui/mux with internal/termmux" (includes 9 pr-split fixes)
- Session timer: scratch/.session-start (2026-02-28 02:10:13)

## Blueprint State
- T001-T010: Done (9 fixes implemented, committed, Rule of Two verified)
- T011-T014: Done (VTerm.String, ChildExitOutput, Detach context, attachChild tests added)
- T015: In Progress (this file update)
- T016: Not Started (full make pipeline)

## Current Diff vs HEAD
- `internal/termmux/vt/vt_test.go` — 8 new TestVTerm_String_* tests
- `internal/termmux/termmux_test.go` — 6 new tests: ChildExitOutput (3), Detach context (2), Attach retry (1)
- `blueprint.json` — status updates for T008-T015
- `WIP.md` — this file

## Key Files
- `internal/termmux/termmux.go` — Core Mux (Attach/Detach/ChildExitOutput/teeLoop)
- `internal/termmux/vt/vt.go` — VTerm (String, Render, Write)
- `internal/command/pr_split.go` — Go bindings (attachChild, detach, switchToClaude)
- `internal/command/pr_split_script.js` — JS logic (spawn, health check, isAlive guards)

## Immediate Next Steps
1. Complete T015 (done now)
2. T016: Run `make` (full pipeline) to verify all tests pass
3. Rule of Two verification on the test additions
4. Commit the test additions
5. Expand scope: ideate new improvements, add tasks to blueprint
6. Continue indefinite cycling per DIRECTIVE.txt

## Key Design Decisions
- VTerm \n is LF only (no CR); tests use \r\n for newline+carriage-return
- ReadLoop context cancel only takes effect on next send, not during blocked Read()
- teeLoop takes teeDone/childEOF as params to prevent channel-reference races
- Detach waits up to detachTimeout for teeLoop, then continues regardless

## Status
Ready for full pipeline verification and commit.
