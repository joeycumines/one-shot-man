# WIP: T42-T49 — Expansion cycle after T38-T41

## Status: IN-PROGRESS — T42 starting

### Last Commit: f255961 (T38+T40 edge case + cancel tests)

### Blueprint State:
- T01-T36: Done (committed)
- T37: Blocked (Claude auth)
- T38-T41: Done (T38+T40 committed f255961, T39 verified existing, T41 expansion complete)
- T42-T49: Not Started (new expansion tasks)

### Current Work: T42 — BT node factory tests
- Need to test 8 factory functions: createAnalyzeNode, createGroupNode, createPlanNode, createSplitNode, createVerifyNode, createEquivalenceNode, createSelectStrategyNode, createWorkflowTree
- Context: pr_split_script.js ~line 4117-4275

### Parallel targets (no code dependencies):
- T44: renderColorizedDiff, getSplitDiff, buildReport behavioral tests
- T45: Fix btCommitChanges git add -A
- T46: Replace cat calls for Windows portability
- T47: Log ExitReason in pr_split.go
- T48: Log strategy failures in resolveConflicts

### Files to modify:
- internal/command/pr_split_bt_test.go (new — BT node tests)
- internal/command/pr_split_scope_misc_test.go (enhance existing)
- internal/command/pr_split_script.js (T45, T46, T48 fixes)
- internal/command/pr_split.go (T47 fix)
