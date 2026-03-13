# WIP — Work In Progress (Takumi's Desperate Diary)

## Current Task: T35 — async runExecutionStep (COMPLETING — Rule of Two pending)

## Session State
- **Branch:** `wip`
- **Last Commit:** T34 (wip@49a032df)
- **Blueprint Status:** T01-T35 Done. T36-T72 Not Started.
- **Tests baseline:** ALL packages PASS (pick-and-place flaky due to build cache, unrelated). ~134s for pr-split with -race.
- **Session start:** 2026-03-13 10:37:36 (9h mandate)
- **Blueprint Schema:** Tasks use `acceptanceCriteria` (array of strings), NOT `acceptance` (string).

## T35 Changes (this session)
### Converted runExecutionStep to async Promise+poll pattern
- **Removed:** sync `runExecutionStep(s, stepIdx)` and `exec-step-0/1/2` tick IDs
- **Removed:** dead `handleEquivCheckState` captured variable (no longer called)
- **Added:** `runExecutionAsync(s)` — async func using `executeSplitAsync` with progressFn callback
- **Added:** `handleExecutionPoll(s)` — 100ms poll for execution completion, chains to verify or equiv
- **Added:** `startEquivCheck(s)` — launches async equiv check pipeline
- **Added:** `runEquivCheckAsync(s)` — async func using `verifyEquivalenceAsync` with try/catch + null guard
- **Added:** `handleEquivPoll(s)` — 100ms poll for equiv completion
- **Modified:** `startExecution(s)` — now launches runExecutionAsync + polls at 100ms
- **Modified:** 4 callers of exec-step-2 → use startEquivCheck(s) instead
- **Modified:** viewExecutionScreen — shows branchIdx/branchTotal during async execution
- **Fixed:** wizard state desync — all transition sites use `try { wizard.transition(X); } catch {}; s.wizardState = s.wizard.current;`
- **Fixed:** cancel handler (`updateConfirmCancel`) — added `s.isProcessing = false` before `wizard.cancel()`
- **Fixed:** cancel guards — all async funcs check `s.wizard.current === 'CANCELLED'` at every await boundary
- **Hardened:** runAnalysisAsync (from T34) — same cancel guards + transition pattern applied retroactively
- **Init state:** Added executionRunning/Error/NextStep/BranchTotal/ProgressMsg, equivRunning/Error
- 15 new tests: 5 ExecutionPoll, 4 EquivPoll, NoSyncCallsRemain, HappyPath, Error, Progress, CancelDuringExecution, WizardStateSync

## T34 Changes (committed: 49a032df)
- runAnalysisStep → async runAnalysisAsync + handleAnalysisPoll
- 9 new tests

## CRITICAL FINDINGS FROM DEEP ANALYSIS (2026-03-13)
1. ~~EVENT LOOP BLOCKING: runAnalysisStep~~ → **FIXED T34**
2. ~~EVENT LOOP BLOCKING: runExecutionStep~~ → **FIXED T35**
3. **EVENT LOOP BLOCKING**: handleClaudeCheck (line 2049) calls sync executor.resolve() — NO async version exists
4. ~~EVENT LOOP BLOCKING: verifySplit/verifyEquivalence~~ → **verifyEquivalenceAsync USED T35**
5. ~~tuiMux BOOTSTRAP GAP~~ → **VERIFIED T33 — architecture connected**
6. **Tab BROKEN in split-view**: Tab at ~line 405 only toggles between panes instead of cycling elements
7. **Expand/collapse BROKEN**: collapse sets expandedVerifyBranch=null clearing ALL state
8. **Integration tests SHALLOW**: no wizard+real-Claude, no Mux lifecycle, no TUI rendering tests

## Next Steps
1. **IMMEDIATE:** Rule of Two review on T35 diff → commit
2. T36: Create resolveAsync + convert handleClaudeCheck
3. Continue T37-T72 sequentially
