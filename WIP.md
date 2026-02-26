# WIP.md — Takumi's Desperate Diary

## Current State
- All implementation changes complete and verified  
- `make` (build + lint + test): ALL GREEN, 43/43 packages pass
- Two commits on wip branch: 39ff255 (tracking), pending (T012+T020)

## Latest Changes (this session)

### T012: Branch restore on all exit paths
**File:** `internal/command/pr_split_script.js`
- Save `originalBranch` via `git rev-parse --abbrev-ref HEAD` at automatedSplit() entry
- Restore before final return — fixes re-split cycle leaving user on baseBranch
- Guard: only restores if originalBranch is non-empty (handles rev-parse failure)

### T020: Improved spawn error messages
**File:** `internal/command/pr_split_script.js`
- Hoisted spawnOpts before try block so catch can reference it
- Error now includes: command attempted, args, provider type
- Null guard chain for cases where exception fires before spawnOpts assignment

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
