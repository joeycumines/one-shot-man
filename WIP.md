# WIP.md — Takumi's Desperate Diary

## Current State
- All implementation changes complete and verified
- `make` (build + lint + test): ALL GREEN, 43/43 packages pass
- Ready for Rule of Two verification and commit

## Changes Made

### 1. Fix ctrl+] toggle during auto-split (T003, T004, T005)
**File:** `internal/command/pr_split.go`
- Pre-declare `autoSplitModel` so the toggle closure can reference it
- Toggle callback now calls `p.ReleaseTerminal()` before `RunPassthrough` 
  and `p.RestoreTerminal()` after — prevents BubbleTea/passthrough stdin/stdout conflicts
- Updated comments to explain the ReleaseTerminal/RestoreTerminal pattern

### 2. Attach Claude handle to tuiMux during auto-split (T005)
**File:** `internal/command/pr_split_script.js`
- After "Spawn Claude" step succeeds, call `tuiMux.attach(claudeExecutor.handle)`
- This enables ctrl+] to forward stdin/stdout to Claude during the pipeline
- Non-fatal: if attach fails, toggle just won't work (logged as warning)

### 3. Detach from tuiMux on cleanup (T001, T002)
**File:** `internal/command/pr_split_script.js`  
- `cleanupExecutor()` now calls `tuiMux.detach()` before closing Claude
- Prevents ctrl+] from trying to forward to a dead child process
- Best-effort: handles already-detached case gracefully

### 4. Post-send cancellation check (T002)
**File:** `internal/command/pr_split_script.js`
- Added `isCancelled()` check immediately after `handle.send()` in the
  classification step — reduces latency between cancel and detection

### 5. Remove loadStrategyPlugin (T013)
**File:** `internal/command/pr_split_script.js`
- Removed `loadStrategyPlugin()` function — used `eval()`, no tests, security risk
- Removed export entry and section header comment
- Per AGENTS.md: "code that is Purpose-less and Untested" = AI slop

### 6. New unit tests (T015, T016)
**File:** `internal/termui/mux/autosplit_test.go`
- `TestAutoSplitModel_CancelThenDone_TriggersQuit`: verifies two-phase
  cancellation dance (q → cancelled → DoneMsg → tea.Quit)
- `TestAutoSplitModel_ToggleKey_FullCycleWithCallback`: verifies ctrl+]
  dispatches callback, returns AutoSplitToggleMsg, model handles it
- `TestAutoSplitModel_ToggleKey_NotTriggeredWhenDone`: verifies toggle
  behavior when pipeline is complete
- `TestAutoSplitModel_ToggleKey_NotTriggeredWhenCancelled`: verifies
  toggle still works during pending cancellation

## Verification Results
- `make build`: PASS
- `make lint`: PASS (vet, staticcheck, deadcode — zero issues)
- `make test`: PASS (43/43 packages, zero failures, race detection enabled)

## Pre-Existing Infrastructure
- TestMain with -integration, -claude-command, -claude-arg, -integration-model flags
- 14 integration tests (TestIntegration_AutoSplitWithClaude, TestIntegration_RealClaudeCode, etc.)
- project.mk integration-test-prsplit target with ?= variables

## Remaining Not Started Tasks
- T010: Verify ClaudeCodeExecutor.spawn() arg passing chain  
- T011: Integration test for cancellation (TestIntegration_AutoSplitCancel)
- T012: Ensure auto-split restores original branch on all exit paths
- T014: Verify scroll behavior after cancellation
- T018: Complex integration test (TestIntegration_AutoSplitComplexGoProject)
- T019: Ensure finishTUI handles no-TUI case
- T020: Comprehensive error messages on spawn failure
