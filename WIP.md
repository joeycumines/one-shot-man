# WIP — PR Split P0 Bug Fixes

## Status: RULE OF TWO PASSED — Ready to commit

### Session Context
- Branch: `wip`
- Previous commits: `7477aae`, `55ec7e6`
- CURRENT: P0 bug fixes + 13 new tests. Rule of Two PASSED (2 contiguous + fitness FIT).

### What changed (UNCOMMITTED — ready to commit)

#### Bug Fixes

1. **TUI command dispatch error swallowing (tui_commands.go)**
   - `ErrCommandNotFound` sentinel error
   - `executor()` now distinguishes "not found" (JS fallback) from handler errors (display to user)
   
2. **Panic recovery (tui_manager.go)**
   - `executeCommand()` uses named return + defer/recover for Go handlers
   
3. **fileStatuses threading (pr_split_script.js)**
   - `analyzeDiff()` uses `--name-status` instead of `--name-only`
   - Returns `fileStatuses` map (file → A/M/D/R/C/T)
   - Rename/copy: only track new path
   - Unmerged paths rejected with clear error
   - Unknown status codes rejected with whitelist
   
4. **Deleted files (pr_split_script.js executeSplit)**
   - `fileStatuses` is REQUIRED (no silent fallback)
   - Status 'D' → `git rm --ignore-unmatch -f`
   - Missing entries → explicit error naming the file
   
5. **Pre-existing branches (pr_split_script.js executeSplit)**
   - Pre-flight loop: delete existing branches before recreation
   - Enables re-running pr-split without manual cleanup
   
6. **Try/catch on critical handlers (pr_split_script.js)**
   - `run`, `analyze`, `execute` handlers wrapped

#### New Tests (13 total)

- `tui_coverage_gaps_test.go`: HandlerError, HandlerPanic, JSFallback (3)
- `pr_split_test.go`: RunHeuristicEndToEnd, RunZeroChanges, RunDryRun, HelpCommand, RunWithDeletedFiles, RunRerun (6)
- `claudemux/pr_split_test.go`: AnalyzeDiff_FileStatuses, WithDeletedFiles, RerunDeletesBranches, NoFileStatuses, MissingFileStatus + fixes to MissingFile (7)

### Next Steps (T009+)
- T009: Wire runtime.aiMode into run handler
- T010: Implement provider registry lifecycle in TUI
- T011-T012: AI classification + mocked tests
- T014-T018: Integration tests with real OLLAMA

**control_test.go** — 5 new tests for send/GetStatus error paths:
- `TestControl_Send_ServerClosesImmediately` — empty response → error
- `TestControl_Send_MalformedResponse` — garbage JSON → unmarshal error
- `TestControl_GetStatus_NonOKResponse` — ok=false → error propagation
- `TestControl_GetStatus_InvalidResultJSON` — ok=true but bad Result → decode error
- `TestControl_EnqueueTask_InvalidResultJSON` — ok=true but bad Result → decode error

#### Coverage improvements (before → after)
| Function | Before | After | Δ |
|----------|--------|-------|---|
| instance.Create | 57.1% | ≥90% | +33%+ |
| guard.computeBackoff | 68.4% | 84.2% | +15.8% |
| control.GetStatus | 66.7% | 80.0% | +13.3% |
| control.send | 68.4% | 78.9% | +10.5% |
| **Total package** | **94.2%** | **94.8%** | **+0.6%** |

### Build status (last verified)
- `go build ./...` PASS
- All claudemux tests PASS with `-race` (6.0s)
- All 15 new tests PASS

### Rule of Two (for this diff)
- Not yet run — PENDING

### Files touched (since 55ec7e6)
- `internal/builtin/claudemux/guard_test.go` — 6 new overflow tests
- `internal/builtin/claudemux/instance_test.go` — 4 new error-path tests + runtime/strings imports
- `internal/builtin/claudemux/control_test.go` — 5 new error-path tests + fmt/net/runtime imports

### Next steps
1. Rule of Two review gate on diff vs HEAD
2. Commit
3. Continue coverage push (T034-T039: remaining functions)
4. Integration test with ollama (T024-T027)
5. Cross-platform verification (T040-T042)
