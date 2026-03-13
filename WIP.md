# WIP — Work In Progress (Takumi's Desperate Diary)

## Current Task: T37 — async verifySplit fallback path (Rule of Two pending)

## Session State
- **Branch:** `wip`
- **Last Commit:** T36 (wip@3856bc10)
- **Blueprint Status:** T01-T37 Done. T38-T72 Not Started.
- **Tests baseline:** ALL packages PASS (pick-and-place flaky due to build cache, unrelated). ~134s for pr-split with -race.
- **Session start:** 2026-03-13 10:37:36 (9h mandate)
- **Blueprint Schema:** Tasks use `acceptanceCriteria` (array of strings), NOT `acceptance` (string).

## T37 Changes (this session)
### Converted verifySplit fallback path to async
- **Replaced:** sync `prSplit.verifySplit()` call in `runVerifyBranch` with async `prSplit.verifySplitAsync()` via `runVerifyFallbackAsync` + `.then()` + poll
- **Added:** `runVerifyFallbackAsync(s, branchName, dir, scopedCmd, timeoutMs)` — async func with cancel guards at both await boundaries
- **Added:** `handleVerifyFallbackPoll(s)` — 100ms poll, checks `verifyFallbackRunning`, handles `.then()` rejection errors, advances to next branch
- **Added:** Init state fields: `verifyFallbackRunning`, `verifyFallbackError`
- **Added:** Tick dispatch: `verify-fallback-poll` → `handleVerifyFallbackPoll`
- **Audit:** `cleanupBranches` NOT called from TUI at all — no conversion needed
- **Note:** `verifySplitAsync` still uses `exec.execStream` for the actual verify command (partially blocking). Full non-blocking requires CaptureSession path (which is the primary path).
- 8 new tests: LaunchesAsync, PollStillRunning, PollCompleted, AsyncHappyPath, AsyncError, AsyncThrows, NoSyncCalls, CancelDuringAsync

## T36 Changes (committed: 3856bc10)
- resolveAsync + handleClaudeCheck async conversion
- 11 new tests

## T35 Changes (committed: 7abdfcb9)
- runExecutionStep → async runExecutionAsync + handleExecutionPoll
- startEquivCheck + runEquivCheckAsync + handleEquivPoll
- 15 new tests

## T34 Changes (committed: 49a032df)
- runAnalysisStep → async runAnalysisAsync + handleAnalysisPoll
- 9 new tests

## CRITICAL FINDINGS FROM DEEP ANALYSIS (2026-03-13)
1. ~~EVENT LOOP BLOCKING: runAnalysisStep~~ → **FIXED T34**
2. ~~EVENT LOOP BLOCKING: runExecutionStep~~ → **FIXED T35**
3. ~~EVENT LOOP BLOCKING: handleClaudeCheck~~ → **FIXED T36 — resolveAsync uses exec.spawn**
4. ~~EVENT LOOP BLOCKING: verifySplit/verifyEquivalence~~ → **FIXED T35 (verifyEquivalenceAsync) + T37 (verifySplitAsync fallback)**
5. ~~tuiMux BOOTSTRAP GAP~~ → **VERIFIED T33 — architecture connected**
6. **Tab BROKEN in split-view**: Tab at ~line 405 only toggles between panes instead of cycling elements
7. **Expand/collapse BROKEN**: collapse sets expandedVerifyBranch=null clearing ALL state
8. **Integration tests SHALLOW**: no wizard+real-Claude, no Mux lifecycle, no TUI rendering tests

## Next Steps
1. **IMMEDIATE:** Rule of Two review on T37 diff → commit
2. T38: Fix split-view Tab behavior
3. Continue T39-T72 sequentially
