# WIP.md — Takumi's Desperate Diary

## Session State
- **All checks pass**: build ✅ lint ✅ test ✅
- **Blueprint**: T001-T013, T015-T030 Done. Pending commit for T025-T028.
- **Commits**: f349929, d03e944, 505a57a, 5424f5b, 6f9aafd, 80ab683, **pending**

## Commit Log
1. `f349929` — Add cancellation, toggle, and scroll to auto-split TUI (507 ins, 17 del, 4 files)
2. `d03e944` — Add sub-step detail progress to auto-split TUI (122 ins, 1 del, 4 files)
3. `505a57a` — Remove dead code branch in toggle key dispatch (1 ins, 3 del)
4. `5424f5b` — Add edge-case tests for auto-split TUI (125 ins, 1 file)
5. `6f9aafd` — Rewrite auto-split cancel lifecycle and remove vaporware (244 ins, 298 del, 4 files)
6. `80ab683` — Add mock-MCP integration test for auto-split pipeline (658 ins, 3 files)
7. `pending` — Add timer, step counter, timeout flag, and Enter dismiss

## Current Work — T030 (Commit T025-T028)
T025-T028 implemented, Rule of Two passed, committing now.

### What Changed (T025-T028)
- **T025**: Enter key dismiss test + no-effect-when-running test
- **T026**: Elapsed wall-clock timer in TUI header, freezes on done
- **T027**: --timeout CLI flag with config fallback, JS propagation
- **T028**: Step counter (N/M format) in rendered steps
- **Extra**: Timer freeze regression test, zero-StartedAt guard

## Files Modified This Session (cumulative)
- internal/termui/mux/autosplit.go — pipelineStartedAt, timer header, step counter, zero-StartedAt guard
- internal/termui/mux/autosplit_test.go — Enter dismiss, timer, timer freeze, step counter tests
- internal/command/pr_split.go — timeout field, flag, config fallback, JS propagation
- internal/command/pr_split_script.js — timeout wiring in run + auto-split commands
- blueprint.json — T025-T030 added and marked Done
- WIP.md — updated
