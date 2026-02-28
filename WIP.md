# WIP — Takumi's State Dump

## Session
- Branch: `wip` (tracking origin/wip)
- Session start: 2026-02-28 13:31:08 (9-hour mandate active, ends ~22:31)
- Timer file: scratch/.session-start

## Blueprint State
- T001-T066: All Done (17 commits, historical)
- T067: NEXT — Run baseline integration tests
- T068-T135: Not Started — pipeline hardening, timeouts, heartbeats, dependency-aware verification, etc.

## Blueprint Rewrite Summary (this session)
Rewrote blueprint.json from scratch. Preserved 3 representative Done tasks. Added 69 new tasks (T067-T135) covering:
- Pipeline hang fixes (wall-clock timeouts in resolveConflicts + resolveConflictsWithClaude)
- Heartbeat monitoring during pollForFile
- Dependency-aware verification (skip downstream when upstream fails)
- verifyCommand default change (make test → make)
- reportResolution pre-existing failure support
- Timeout propagation to claude-fix strategy
- sendToHandle Enter delay configurability + EAGAIN retry
- Per-branch retry budget
- Deadcode audit (conversationHistory, telemetryData, detectLanguage)
- Cross-platform build fixes (Windows, Linux)
- Per-branch verify timeout
- fileStatuses 'T' handling
- Claude auto-detection verification
- Session persistence (savePlan/loadPlan in auto-split, crash recovery)
- TUI feedback improvements
- VTerm re-render integration test
- Rate-limiting MCP calls
- Cancellation edge cases
- Integration + unit tests for all changes
- Timeout architecture documentation

## Key Files
- blueprint.json: /Users/joeyc/dev/one-shot-man/blueprint.json
- pr_split_script.js: /Users/joeyc/dev/one-shot-man/internal/command/pr_split_script.js (~4800 lines)
- pr_split.go: /Users/joeyc/dev/one-shot-man/internal/command/pr_split.go
- termmux.go: /Users/joeyc/dev/one-shot-man/internal/termmux/termmux.go
- Integration tests: /Users/joeyc/dev/one-shot-man/internal/command/pr_split_integration_test.go
- project.mk: /Users/joeyc/dev/one-shot-man/project.mk (integration targets)
- config.mk: /Users/joeyc/dev/one-shot-man/config.mk (custom targets)

## T067 Baseline Results (2026-02-28)

### make make-all-with-log → EXIT CODE 2 (PRE-EXISTING FAILURE)
- **Failure:** `TestPickAndPlace_MousePick_HoldingItem` in internal/command/pick_and_place_unix_test.go:1322
- **Cause:** Timing-dependent game test — actor fails to pick up cube due to PTY frame lag. PRE-EXISTING, not related to pr-split.
- **All other packages:** PASS (termmux 12.9s, ptyio 5.9s, statusbar 6.1s, ui 6.0s, vt 4.8s, scripting 487.9s)

### make integration-test-prsplit-mcp → EXIT CODE 0 (ALL PASS)
- TestIntegration_AutoSplitMockMCP completed full 8-step pipeline in 2.35s
- Steps: Analyze(65ms)→Spawn(0ms)→Send(53ms)→Receive(1ms)→Plan(0ms)→Execute(925ms)→Verify(195ms)→Equivalence(39ms)
- Created 4 split branches: api-types, cli-serve, db-migration, docs-update
- 6 independence pairs verified, 2 Claude interactions

### make integration-test-termmux → EXIT CODE 0 (ALL PASS)
- termmux: 8.9s (9 integration tests + 30+ unit tests)
- ptyio: 1.9s (3 integration tests + 7 unit tests)
- statusbar: 2.2s (17 tests)
- ui: 2.5s (90+ tests including plan editor, split view, key handling)
- vt: 3.0s (80+ tests including fuzz tests)
- **Zero failures across all termmux packages**

## Immediate Next Steps
1. T093: Claude auto-detection verification
2. T094-T096: Session persistence (savePlan/loadPlan, --resume, crash recovery)
3. T097+: Conversation history persistence, TUI feedback, etc.

## Completed This Session
- T067: Baseline tests documented
- T068: Wall-clock timeout in resolveConflicts() — AUTOMATED_DEFAULTS.resolveWallClockTimeoutMs = 7200000
- T069: Wall-clock timeout in resolveConflictsWithClaude() — wallClockMs parameter, deadline checks
- T070: Heartbeat check in pollForFile() — aliveCheckFn 6th parameter, every 10 iterations
- T071: Thread aliveCheckFn through automatedSplit() — 4 pollForFile calls get heartbeat
- T072: Dependency tracking in createSplitPlan() — dependencies field on each split
- T073: verifySplits() skips branches when upstream dependencies fail — failedBranches set
- T074: automatedSplit() separates skipped from real failures, report.skippedDueToDepFailure
- T075: Default verifyCommand changed from 'make test' to 'make'
- T076: reportResolution MCP tool accepts preExistingFailure (mcp.go + prompt)
- T077: resolveConflictsWithClaude handles preExistingFailure — no retry, report.preExistingFailures
- T078: Verified timeout propagation chain, identified claude-fix gap
- T079: claude-fix strategy.fix() now accepts options.resolveTimeoutMs, threaded from resolveConflicts()
- T080: SEND_ENTER_DELAY_MS module-level var + setSendEnterDelay() setter, sendToHandle uses it
- T081: EAGAIN retry in sendToHandle fallback path — 3 retries with 10ms delay
- T082: Per-branch retry budget in resolveConflicts() — perBranchRetryBudget option, branchRetries tracking, while-loop retry wrapper
- T083: Renamed maxRetries → maxAttemptsPerBranch in resolveConflictsWithClaude + test
- T084: Audit conversationHistory — intentional (plan persistence + REPL command), documented
- T085: Audit telemetryData — intentional (REPL 'telemetry' command + opt-in save), documented
- T086: Audit detectLanguage() — intentional (classification prompt template), documented
- T089: verifySplit() wraps command in `timeout` utility, detects exit code 124. verifySplits() accepts options.verifyTimeoutMs. 3 sub-tests pass.
- T090: AUTOMATED_DEFAULTS.verifyTimeoutMs = 600000 (10 min). Threaded through automatedSplit() Step 7 + REPL propagation. Test for default value passes.
- T091: Explicit 'T' (type change) handling in executeSplit() with log.warn. TestExecuteSplit_TypeChange passing.
- T092: claude-fix strategy extracts aliveCheckFn from options, passes to pollForFile as 6th arg. Fixed positional arg bug (was passing to stepName slot). resolveConflicts() threads aliveCheckFn through strategyOptions. TestPrSplitCommand_ResolveConflicts_AliveCheckFnThreaded passing.
