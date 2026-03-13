# WIP — Work In Progress (Takumi's Desperate Diary)

## Current Task: T33 — tuiMux bootstrap (COMPLETING — Rule of Two pending)

## Session State
- **Branch:** `wip`
- **Last Commit:** T32 (wip@726b4f6e)
- **Blueprint Status:** T01-T33 Done. T34-T72 Not Started.
- **Tests baseline:** ALL packages PASS (`make all`). 937s for internal/command with -race.
- **Session start:** 2026-03-13 10:37:36 (9h mandate)
- **Blueprint Schema:** Tasks use `acceptanceCriteria` (array of strings), NOT `acceptance` (string).

## T33 Findings (this session)
### Architecture is CONNECTED — no bootstrap gap
- Go-side: `termmux.New()` → `engine.SetGlobal("tuiMux", WrapMux(mux))` (pr_split.go:339,345)
- JS-side: `automatedSplit()` pipeline calls `tuiMux.attach(claudeExecutor.handle)` (pipeline.js:1438)
- Type chain: JS handle → Goja export → `map[string]any{"_goHandle": *ptyAgentHandle}` → `resolveChild` Case 2 → `StringIO` assertion → `WrapStringIO` → `Mux.Attach()` → VTerm + teeLoop
- Re-attach on restart: tui_core.js:2357-2361
- renderClaudePane already shows "No Claude session attached" placeholder

### Changes Made
1. `pollClaudeScreenshot`: Added `hasChild()` guard — clears screen state when no child, continues polling
2. `switchTo` (3 sites): Added backward-compatible `hasChild()` guard — `(typeof tuiMux.hasChild !== 'function' || tuiMux.hasChild())`
3. 6 new unit tests: NoMux, NoChild, WithChild, SplitViewDisabled, SwitchTo_NoChild, SwitchTo_WithChild

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
1. **IMMEDIATE:** Rule of Two review on T33 diff → commit
2. T34: Convert runAnalysisStep to async
3. T35: Convert runExecutionStep to async
4. Continue T36-T72 sequentially
