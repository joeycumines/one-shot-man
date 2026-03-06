# WIP — Active Session State

## Session Start
- **Timestamp**: 2026-03-06 08:35:45 (tracked in scratch/.session-start)
- **Mandate**: 9 hours of continuous improvement (until 17:35:45 2026-03-06)

## Current Phase: HANA INTERVENTION COMPLETE — Committing H-series + T27-T46

### H-Series Fixes — ALL COMPLETE, RULE OF TWO PASSED
- **H1**: Removed ALL autoSplitTUI dead code from pipeline.js (hasTUI, emitOutput, updateDetail, step, finishTUI, sendWithRetry, verify callbacks)
- **H2**: Rewired cooperative cancellation to prSplit._cancelSource query-based callback
- **H3**: Fixed discoverVerifyCommand to prefer gmake over make on macOS
- **H4**: Updated all sendToHandle tests (direct handle.send() path)
- **Output fix**: tui_manager.go SetTUISink(nil) during executor for direct stdout
- **6 test files updated**: integration, recovery, prompt, pipeline, conflict_retry, session_cancel, complex_project, core
- **Rule of Two**: 2 contiguous PASS reviews + fitness review PASSED
- **Build**: GREEN (5,961+ tests, build, lint all pass)

### Commit Scope
- Staged: Deleted Go TUI (AutoSplitModel 2246 lines, PlanEditor 378 lines, UI helpers, state machine doc), new SGR mouse (sgrmouse.go 189 lines + tests 269 lines), termmux enhancements (bell, activity, mouse)
- Unstaged: H-series JS/Go/test fixes, docs, blueprint

### Next After Commit
- T47: Cross-platform validation (make-all-in-container)
- T48+: Remaining blueprint tasks
- BLOCKED: T40-T41 (model doesn't support MCP)

### Phase 1 Summary (T01-T15) — COMPLETE
- 7 mock MCP integration tests: All GREEN (14.2s total)
- make all GREEN, make integration-test-prsplit-mcp GREEN

### Phase 2 Wizard Handlers (T16-T26) — COMPLETE
- **42 handler tests**: All PASS in pr_split_13_tui_test.go
- 8 state handlers implemented in pr_split_13_tui.js

### Phase 2 E2E Tests (T28) — COMPLETE
- **4 E2E wizard flow tests**: All PASS
- AllCommandNames updated to 31 commands (added abort, override)

### T27: Remove old Go TUI — COMPLETE
- 7 files deleted, ~300 lines removed from pr_split.go
- Fixed ExitReason.String deadcode

### T29: Wizard Integration Tests — COMPLETE
- 3 full-engine integration tests, all PASS

### T30: W-series Verification Gate — COMPLETE
- make all GREEN, integration-test-prsplit-mcp GREEN, integration-test-termmux GREEN

### T31: Bell Interception — COMPLETE
- Added SetBellFunc(func()) to Mux struct
- Module.go wires bell → events.queue(EventBell, {pane:"claude"})
- JS bell listener in pr_split_13_tui.js: status bar flash + log
- 5 new tests (3 mux + 2 module), all PASS

### T32: Process State HUD Overlay — COMPLETE
- Added lastWriteAt atomic.Int64 to Mux struct, LastWriteTime() method
- Module.go: lastActivityMs() JS function (ms since last child write, -1 if never)
- pr_split_13_tui.js: _getActivityInfo() (threshold-based activity icons), _getLastOutputLines(), _renderHudPanel() (lipgloss border + fallback), hud command
- 7 new tests (2 termmux + 2 module + 3 chunk13), total 67 chunk13 tests
- make test GREEN (exit 0), make lint GREEN

### T33: Mouse Support in Termmux — COMPLETE
- Created `internal/termmux/sgrmouse.go`: SGR mouse protocol parser
  - SGRMouseEvent struct (Button, X, Y, Release)
  - parseSGRMouse() — parses CSI < Ps;Px;Py [Mm] sequences
  - filterMouseForStatusBar() — intercepts left clicks on status bar row
  - IsPress(), IsWheel(), IsLeftClick() helper methods
- Created `internal/termmux/sgrmouse_test.go`: 21 unit tests (11 parser + 10 filter)
- Wired filterMouseForStatusBar into RunPassthrough stdin→child goroutine
  - Left click on status bar row → ExitToggle (same as toggle key)
  - Wheel events, right clicks, releases pass through to child
  - Reads termRows under lock for resize safety
- 3 RunPassthrough integration tests added to termmux_test.go:
  - TestRunPassthrough_MouseClickStatusBarToggle
  - TestRunPassthrough_MouseClickAboveStatusBar
  - TestRunPassthrough_MouseClickStatusBarDisabled
- make build ✅, make lint ✅, make test ✅ (full suite ~10min)

### Next: T38 (Enhanced error handling), T39+

### T38: Enhanced Error Handling (Auto-retry, Subshell, Skip) — COMPLETE
- **pr_split_10_pipeline.js**:
  - `isTransientError(errStr)` function: classifies "rate limit", "429", "quota", "overloaded", "timeout", "EAGAIN", "ECONNRESET", "EPIPE", "temporarily unavailable" as transient
  - Exponential backoff in retry loop: `2000 * Math.pow(2, attempt - 1)` capped at 30s, skipped on first attempt
  - sendToHandle error: transient → continue, permanent → break
  - Consecutive validation failure limit: 2 failures → break
  - **FIXED** falsy-or bug: `config.maxResolveRetries || default` treated 0 as falsy (JS `0 || 3` = 3). Now uses `typeof config.maxResolveRetries === 'number'` check. Same fix for `maxReSplits`. This bug caused VerifyFailure test to time out at 60s (3 retries × backoff + 60s wallclock instead of 0 retries).
- **pr_split_13_tui.js**:
  - ERROR_RESOLUTION 'skip' choice: builds `skipBranchSet` from failedBranches, stores on `st.skipBranchSet` + `wizard.data.skippedBranches`, transitions to EQUIV_CHECK
  - ERROR_RESOLUTION 'manual' choice: prints failed branch names with errors, transitions to BRANCH_BUILDING
  - `handleEquivCheckState`: surfaces `skippedCount` in return object
  - `create-prs` REPL command: filters `st.planCache.splits` against `st.skipBranchSet` before PR creation
- VERIFIED: Build GREEN, Lint GREEN, 67+ chunk13 PASS, 17 executeSplit PASS, 7/7 MCP integration PASS (VerifyFailure: 0.63s)

### Next: T39 (Error recovery integration tests), T40+

### T39: Error Recovery Integration Tests — COMPLETE
- **4 new integration tests** in `pr_split_autosplit_recovery_test.go`:
  1. `TestIntegration_AutoSplitMockMCP_ErrorRecovery_ClassificationTimeout` (0.75s) — No classification injected, classifyTimeoutMs:500 → aborts with "timeout waiting for reportClassification after 500ms"
  2. `TestIntegration_AutoSplitMockMCP_ErrorRecovery_PlanFallbackToLocal` (5.61s) — Classification injected but NOT plan, planTimeoutMs:500 → pipeline falls back to local plan generation via createSplitPlan(), both splits execute
  3. `TestIntegration_AutoSplitMockMCP_ErrorRecovery_ExecutionFailure` (0.31s) — Plan references nonexistent file "nonexistent.go" → executeSplit aborts with "file ... has no entry in plan.fileStatuses"
  4. `TestIntegration_AutoSplitMockMCP_ErrorRecovery_AllBranchesFailVerify` (0.61s) — All branches fail verify (`exit 1`), but pipeline correctly identifies them as pre-existing failures (feature branch itself fails), completes without crash
- **Key learning**: Pre-existing failure detection — pipeline runs verify on feature branch first. If a branch fails on feature, its failure on a split branch is "pre-existing" (not a regression), so Verify splits step shows no error and preExistingFailures array is populated instead.
- Initially wrote test names as `TestIntegration_ErrorRecovery_*` — renamed to `TestIntegration_AutoSplitMockMCP_ErrorRecovery_*` to match make target's `-run 'TestIntegration_AutoSplitMockMCP'` pattern.
- Total: 11 MCP mock integration tests, ALL PASS (21.6s)
- make build ✅, make lint ✅, integration-test-prsplit-mcp ✅ (11/11 PASS)

### Next: T40 (Run real-Claude integration tests), T41+

### T40: Run Real-Claude Integration Tests — BLOCKED
- Added `verifyClaudeAuth(t)` to 4 tests across 2 files
- Added per-step timeouts (300s classify/plan/resolve) to ComplexGoProject test
- Root cause: model `gpt-oss:20b-cloud` does NOT support MCP tool invocation
- Pipeline hangs waiting for tool call that never comes
- **BLOCKED** pending model change (need a model that supports MCP tools)

### T42: Naming Consistency Audit — COMPLETE
- Subagent audit: 147 identifiers, 94.6% compliance → 100% post-fix
- **P0 (visibility)**: `_isPaused` → `isPaused`, `_isForceCancelled` → `isForceCancelled`
  - Files: core.js, pipeline.js, exports.js, core_test.go, integration_test.go, test.go
- **P1 (verb-first)**: `emptyResult` → `createEmptyResult` (analysis.js, 5 sites)
- **P1 (verb-first)**: `numberOrDefault` → `resolveNumber` (pipeline.js, 15 sites)
- **P2 (verb-first)**: `safeScreenshot` → `captureScreenshot` (pipeline.js, 2 sites)
- **P2 (verb-first)**: `textTailAnchor` → `getTextTailAnchor` (pipeline.js, 2 sites)
- **Accepted**: WIZARD_VALID_TRANSITIONS/TERMINAL_STATES prefix — intentional namespace-prefixed export pattern
- Doc written: scratch/t42-naming-conventions.md
- make build ✅, make lint ✅, integration-test-prsplit-mcp ✅ (11/11 PASS)

### Next: T43 (Error message clarity audit)

### T37: Git Worktree Isolation — COMPLETE
- **executeSplit** (pr_split_05_execution.js): Creates temporary worktree (`git worktree add --detach`) for all branch operations. User's CWD never touched. Removed `savedBranch`/`restoreSafely()`.
- **verifySplit** (pr_split_06_verification.js): Creates per-branch temporary worktree. Tries proper branch checkout first (preserves HEAD name for verify commands that check branch), falls back to `--detach` if branch is already checked out elsewhere.
- **verifySplits** (pr_split_06_verification.js): Removed `savedBranch`/`restoreBranch` pattern. Each verifySplit call manages its own worktree.
- **resolveConflictsWithClaude** (pr_split_10_pipeline.js): Patches now applied in temporary worktree on failing branch (`git worktree add <path> <branchName>`). Fixes pre-existing bug where patches were applied to the user's branch instead of the split branch. Added `shellQuote` binding.
- **resolveConflicts** (pr_split_08_conflict.js): Per-branch worktree for fix strategies. Strategies receive worktree dir as `dir` param. Claude-fix strategy writes patches to `dir + '/' + patch.file`. Commands run with `cd <worktreeDir> &&`.
- **automatedSplit** (pr_split_10_pipeline.js): Removed `originalBranch` save/restore — no longer needed.
- **TUI baseline** (pr_split_13_tui.js): Removed post-baseline checkout restore.
- VERIFIED: 17 executeSplit unit+integration tests PASS, 7 MCP integration tests PASS
- ConflictResolution test now shows only 1 conflict attempt (previously 3) — patches correctly applied to split branch via worktree

### T36: API Tidy and Signal Handling — COMPLETE (no changes needed)
- Audit: 97 exports, 100% camelCase verb-first naming
- 3 categorical error patterns (IO, validator, transform) — all appropriate for domain
- WizardState: cancel(), forceCancel(), cooperative cancellation via isCancelled() polling

### T35: Replace socat with native IPC — COMPLETE
- Created `internal/command/mcp_bridge.go`: `mcp-bridge` CLI command
  - Bidirectional stdio↔socket proxy (UDS or TCP)
  - `Execute(["unix"|"tcp", address])` — connects and bridges io.Copy both directions
  - Half-close (CloseWrite) when stdin EOF for clean shutdown
- Created `internal/command/mcp_bridge_test.go`: 7 tests
  - TestMcpBridgeCommand_BadArgs (4 subtests)
  - TestMcpBridgeCommand_ConnectFail
  - TestMcpBridgeCommand_BidirectionalCopy_TCP (echo server)
  - TestMcpBridgeCommand_BidirectionalCopy_Unix (echo server)
  - TestMcpBridgeCommand_LargePayload (256KB)
  - TestMcpBridgeCommand_ServerClosesFirst (server EOF)
  - TestMcpBridgeCommand_Name, TestCloseWrite
- Modified `internal/builtin/mcpcallbackmod/mcpcallback.go`:
  - `generateFiles()`: replaced hardcoded `"socat"` command with `os.Executable()` → `osm mcp-bridge <transport> <address>`
  - Removed socat-specific `UNIX-CONNECT:` / `TCP:` arg formatting
- Modified `cmd/osm/main.go`: registered `NewMcpBridgeCommand()`
- Modified `internal/command/pr_split_integration_test.go`:
  - Removed socat pre-flight check (`exec.LookPath("socat")`)
  - Uses `os.Executable()` → `osmExe mcp-bridge unix <sockPath>` in test MCP config
- Modified `internal/builtin/mcpcallbackmod/mcpcallback_test.go`:
  - Fixed skip message (removed socat mention)
- make build ✅, make lint ✅, integration-test-prsplit-mcp ✅ (7/7 PASS)

### T34: Remove Session ID from MCP Payloads — COMPLETE
- Removed `{{.SessionID}}` from 3 prompt templates in pr_split_09_claude.js (classification, split plan, conflict)
- Removed `SessionID` from 3 render functions in pr_split_09_claude.js
- Removed `sessionId` property from 3 MCP tool schemas in pr_split_10_pipeline.js (reportClassification, reportSplitPlan, reportResolution)
- Removed `sessionId` from 3 render calls in pr_split_10_pipeline.js (classification, conflict, re-classify prompt)
- Updated 8 tests across 4 test files:
  - pr_split_prompt_test.go: 4 tests (removed sessionId, flipped 2 assertions to verify NO session ID)
  - pr_split_09_claude_test.go: removed sessionId from conflict test config, flipped assertion
  - pr_split_template_unit_test.go: 3 tests (removed sessionId from configs, flipped 2 assertions)
  - pr_split_autosplit_recovery_test.go: flipped 1 assertion
- Internal tracking (executor.sessionId, pipeline sessionId var, spawn returns) preserved — NOT in payloads
- Grep verification: no session IDs in MCP tool schemas or prompt templates
- make build ✅, make lint ✅, integration-test-prsplit-mcp ✅ (7/7 PASS)

### Key Files
- `internal/termmux/termmux.go` — Mux with SetBellFunc, BellFn, lastWriteAt, LastWriteTime, SGR mouse filtering
- `internal/termmux/sgrmouse.go` — SGR mouse parser and status bar click filter
- `internal/termmux/sgrmouse_test.go` — 21 mouse tests
- `internal/builtin/termmux/module.go` — Bell event wiring, lastActivityMs(), JS facade
- `internal/command/pr_split_13_tui.js` — HUD overlay, bell listener, wizard handlers, TUI commands (32 commands)
- `internal/command/pr_split_13_tui_test.go` — 67 tests
- `internal/command/pr_split_wizard_integration_test.go` — 3 integration tests
- `blueprint.json` — source of truth
