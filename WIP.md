# WIP — Work In Progress (Takumi's Desperate Diary)

## Current Task: T32 — Integration test baseline (COMPLETING — Rule of Two pending)

## Session State
- **Branch:** `wip`
- **Last Commit:** T31 (wip@3403da70 — blueprint rewrite)
- **Blueprint Status:** T01-T31 + T61-T62-old Done. T32 IN PROGRESS (fixes applied, `make all` PASSES). T33-T72 Not Started.
- **Tests baseline:** ALL 49 packages PASS (`make all` exit 0). Zero failures. 708s for internal/command.
- **Session start:** 2026-03-13 10:37:36 (9h mandate)
- **Blueprint Schema:** Tasks use `acceptanceCriteria` (array of strings), NOT `acceptance` (string).

## T32 Fixes Applied (this session)
1. **5 unit test fixes** (async IIFE wrappers):
   - `pr_split_09_claude_test.go`: 2 restart tests → `(async function() { await ex.restart(...) })()`
   - `pr_split_integration_test.go`: 3 spawn tests → `(async function() { await originalSpawn.call(...) })()`
2. **7 VTerm integration test fixes** (REPL mode + prompt pattern):
   - `pr_split_termmux_observation_test.go`: All 7 test functions + vtermStartConsole helper
   - Added `-interactive=false` flag to force REPL mode instead of BubbleTea TUI
   - Changed `Contains("pr-split")` → `Contains("(pr-split)")` (11 instances total)
3. **Baseline doc:** `scratch/integration-test-baseline.md` — updated with post-fix state
4. **Make target:** `config.mk` has `test-targeted` for focused test runs
5. **Verification:** `make all` → 49/49 packages PASS, 0 failures

## CRITICAL FINDINGS FROM DEEP ANALYSIS (2026-03-13)
1. **EVENT LOOP BLOCKING**: runAnalysisStep (line 1938) calls sync analyzeDiff/applyStrategy/createSplitPlan — async versions EXIST but NOT used from TUI
2. **EVENT LOOP BLOCKING**: runExecutionStep calls sync executeSplit (line 2431) — executeSplitAsync EXISTS but NOT called from TUI
3. **EVENT LOOP BLOCKING**: handleClaudeCheck (line 2049) calls sync executor.resolve() — NO async version exists
4. **EVENT LOOP BLOCKING**: verifySplit/verifyEquivalence called sync from TUI fallback paths
5. **tuiMux BOOTSTRAP GAP**: Claude process not properly attached to Mux in TUI context — childScreen() returns empty
6. **Tab BROKEN in split-view**: Tab at ~line 405 only toggles between panes instead of cycling elements
7. **Expand/collapse BROKEN**: collapse sets expandedVerifyBranch=null clearing ALL state
8. **Integration tests SHALLOW**: no wizard+real-Claude, no Mux lifecycle, no TUI rendering tests

## Next Steps
1. **IMMEDIATE:** Rule of Two review on current diff → commit as T32
2. T33: tuiMux bootstrap fix
3. T34: Convert runAnalysisStep to async
4. Continue T35-T72 sequentially
