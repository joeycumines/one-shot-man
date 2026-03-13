# WIP — Work In Progress (Takumi's Desperate Diary)

## Current Task: T36 — async resolveAsync + handleClaudeCheck (Rule of Two pending)

## Session State
- **Branch:** `wip`
- **Last Commit:** T35 (wip@7abdfcb9)
- **Blueprint Status:** T01-T36 Done. T37-T72 Not Started.
- **Tests baseline:** ALL packages PASS (pick-and-place flaky due to build cache, unrelated). ~134s for pr-split with -race.
- **Session start:** 2026-03-13 10:37:36 (9h mandate)
- **Blueprint Schema:** Tasks use `acceptanceCriteria` (array of strings), NOT `acceptance` (string).

## T36 Changes (this session)
### Created resolveAsync and converted handleClaudeCheck to non-blocking
- **Added:** `ClaudeCodeExecutor.prototype.resolveAsync(progressFn)` in pr_split_09_claude.js — uses exec.spawn() for non-blocking which/version/list checks, accepts progressFn for per-step status updates
- **Converted:** `handleClaudeCheck(s)` to Promise+poll pattern — launches `runClaudeCheckAsync(s, executor)` + polls at 50ms
- **Added:** `runClaudeCheckAsync(s, executor)` — async func calling executor.resolveAsync with progressFn
- **Added:** `handleClaudeCheckPoll(s)` — checks claudeCheckRunning flag, continues poll or stops
- **Added:** Init state fields: claudeCheckRunning, claudeCheckProgressMsg
- **Added:** Tick dispatch: claude-check-poll → handleClaudeCheckPoll
- **Added:** Cache check: if st.claudeExecutor already resolved → skip async, use cached result
- **Added:** Belt-and-suspenders: `s.claudeCheckStatus = 'checking'` in async launch path
- **Updated:** View: shows claudeCheckProgressMsg during checking phase
- **Discovery:** prSplitConfig is ALWAYS defined in test harness (loadChunkEngine sets it). Tests must explicitly `delete globalThis.prSplitConfig` to test the guard path.
- 11 new tests: NoPrSplitConfig, CachedExecutor, LaunchesAsync, PollStillRunning, PollCompleted, AsyncHappyPath, AsyncError, AsyncThrows, OldSyncRemoved, ReentryGuard, SwitchAwayCleansUp

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
4. ~~EVENT LOOP BLOCKING: verifySplit/verifyEquivalence~~ → **verifyEquivalenceAsync USED T35**
5. ~~tuiMux BOOTSTRAP GAP~~ → **VERIFIED T33 — architecture connected**
6. **Tab BROKEN in split-view**: Tab at ~line 405 only toggles between panes instead of cycling elements
7. **Expand/collapse BROKEN**: collapse sets expandedVerifyBranch=null clearing ALL state
8. **Integration tests SHALLOW**: no wizard+real-Claude, no Mux lifecycle, no TUI rendering tests

## Next Steps
1. **IMMEDIATE:** Rule of Two review on T36 diff → commit
2. T37: Convert verifySplit fallback path to async in TUI
3. Continue T38-T72 sequentially
