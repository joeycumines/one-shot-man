# WIP — Work In Progress (Takumi's Desperate Diary)

## Current Task: T34 — async runAnalysisStep (COMPLETING — Rule of Two pending)

## Session State
- **Branch:** `wip`
- **Last Commit:** T33 (wip@aa4cf0eb)
- **Blueprint Status:** T01-T34 Done. T35-T72 Not Started.
- **Tests baseline:** ALL 49 packages PASS (`make all`). ~740s for internal/command with -race.
- **Session start:** 2026-03-13 10:37:36 (9h mandate)
- **Blueprint Schema:** Tasks use `acceptanceCriteria` (array of strings), NOT `acceptance` (string).

## T34 Changes (this session)
### Converted runAnalysisStep to async Promise+poll pattern
- **Removed:** sync `runAnalysisStep(s, stepIdx)` function and `analysis-step-0..3` tick IDs
- **Added:** `runAnalysisAsync(s)` — async function using `analyzeDiffAsync` + `createSplitPlanAsync`
- **Added:** `handleAnalysisPoll(s)` — 100ms poll handler for spinner animation + completion detection
- **Modified:** `startAnalysis(s)` — now launches `.then()` pattern and polls at 100ms
- **Pure compute stays inline:** `applyStrategy` + `validatePlan` (no I/O, sub-ms) called inline in async func
- **Cancel:** `if (!s.isProcessing) return;` checks between every step
- 9 new tests: AnalysisPoll_StillRunning, _Cancelled, _ErrorFromPromise, _CompletedSuccess,
  AnalysisAsync_HappyPath, _AnalyzeDiffError, _NoChanges, _ValidationFailure, _NoSyncCallsRemain

## T33 Findings (prior)
### Architecture is CONNECTED — no bootstrap gap
- Go-side: `termmux.New()` → `engine.SetGlobal("tuiMux", WrapMux(mux))` (pr_split.go:339,345)
- JS-side: `automatedSplit()` pipeline calls `tuiMux.attach(claudeExecutor.handle)` (pipeline.js:1438)
- hasChild() guards added to pollClaudeScreenshot + 3 switchTo sites

## CRITICAL FINDINGS FROM DEEP ANALYSIS (2026-03-13)
1. ~~EVENT LOOP BLOCKING: runAnalysisStep~~ → **FIXED T34**
2. **EVENT LOOP BLOCKING**: runExecutionStep calls sync executeSplit (line 2431) — executeSplitAsync EXISTS but NOT called from TUI
3. **EVENT LOOP BLOCKING**: handleClaudeCheck (line 2049) calls sync executor.resolve() — NO async version exists
4. **EVENT LOOP BLOCKING**: verifySplit/verifyEquivalence called sync from TUI fallback paths
5. ~~tuiMux BOOTSTRAP GAP~~ → **VERIFIED T33 — architecture connected**
6. **Tab BROKEN in split-view**: Tab at ~line 405 only toggles between panes instead of cycling elements
7. **Expand/collapse BROKEN**: collapse sets expandedVerifyBranch=null clearing ALL state
8. **Integration tests SHALLOW**: no wizard+real-Claude, no Mux lifecycle, no TUI rendering tests

## Next Steps
1. **IMMEDIATE:** Rule of Two review on T34 diff → commit
2. T35: Convert runExecutionStep to async
3. Continue T36-T72 sequentially
