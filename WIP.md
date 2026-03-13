# WIP — Work In Progress (Takumi's Desperate Diary)

## Current Task: T43 — Graceful error recovery for stale/missing branch on CONFIG screen

## Session State
- **Branch:** `wip`
- **Last Commit:** T42 (pending amend)
- **Blueprint Status:** T01-T42 Done. T43-T72 Not Started.
- **Tests baseline:** ALL packages PASS (49/49). ~794s for internal/command with -race.
- **Session start:** 2026-03-13 10:37:36 (9h mandate)
- **Blueprint Schema:** Tasks use `acceptanceCriteria` (array of strings), NOT `acceptance` (string).

## T42 Changes (this session — Rule of Two PASSED)
### Default to Claude strategy when Claude is available
- **Added:** `userHasSelectedStrategy: false` to init state (pr_split_16_tui_core.js:123)
- **Changed:** First WindowSize handler returns `tea.batch(tea.clearScreen(), tea.tick(1, 'auto-detect-claude'))` instead of just `tea.clearScreen()`
- **Added:** `auto-detect-claude` tick handler: skips if userHasSelectedStrategy or if claudeCheckStatus already set; else calls handleClaudeCheck(s)
- **Modified:** `runClaudeCheckAsync` success path: sets `prSplit.runtime.mode = 'auto'` when `!s.userHasSelectedStrategy`
- **Set `userHasSelectedStrategy = true`** on all 4 manual paths: mouse strategy click, mouse Test Connection, keyboard strategy activate, keyboard Test Connection
- **Updated views:** '(auto-selected)' hint when Claude available and auto-detected; 'Using heuristic strategy' (was 'Fell back to Heuristic strategy')
- **7 new tests:** AutoDetectClaudeOnStartup, AutoDetectSkipsWhenUserSelected, ManualSelectSetsFlag, AutoDetectUnavailableFallback, AutoDetectSkipsWhenAlreadyChecking, ViewShowsAutoStrategyHint, InitReturnsBatchCommand

## T41 Changes (this session — Rule of Two PASSED)
### Fix inline title editing + navigation conflict
- **Added:** Defense-in-depth guard in `handleListNav`: `if (s.editorTitleEditing) return [s, null];`
- **Context:** Title editing interceptor (lines 309-348) already catches all keys before handleListNav reaches them. Guard prevents corruption if future code bypasses interceptor.
- **5 new tests:** JKDoesNotMoveFile, ArrowsSwallowed, HandleListNavGuard_DirectCall, SplitIdxStable, FocusCycleBlocked

## T40 Changes (this session — Rule of Two PASSED)
### Complete tab navigation across ALL screens
- **Added:** FINALIZATION focus elements: `[final-report, final-create-prs, final-done, nav-next]` (4 elements)
- **Added:** handleFocusActivate handlers for `toggle-advanced`, `final-report`, `final-create-prs`, `final-done`
- **Added:** CONFIG `toggle-advanced` in focus system with `▸` pointer indicator (dynamic index)
- **Added:** ERROR_RESOLUTION `nav-next` (after error-ask-claude, both normal and crash mode)
- **Added:** BRANCH_BUILDING `'e'` keyboard shortcut for expand/collapse verify output
- **Added:** viewFinalizationScreen focus-aware button styling (focusedButton/primaryButton/secondaryButton)
- **Added:** Help overlay "Branch Building" section with `e` and `Ctrl+C` shortcuts
- **Updated:** 6 existing tests (CONFIG nav-next index 3→4, FINALIZATION element counts, crash mode)
- **10 new tests:** FinalizationFocusElements, FinalizationTabCycling, FinalizationEnterActivatesButtons, FinalizationFocusIndicators, ConfigToggleAdvancedFocus, ConfigToggleAdvancedVisualIndicator, ErrorResolutionNavNext, BranchBuildingExpandCollapseKeyboard, HelpOverlayBranchBuildingSection, ElementCountParity
- **Rule of Two:** Pass 1 FAIL (nav-next missing from FINALIZATION) → fixed → Pass 1 v2 PASS → Pass 2 PASS

## T38 Changes (this session)
### Fix split-view Tab behavior — cycle elements within active pane
- **Changed:** Tab → Ctrl+Tab for pane switching (pr_split_16_tui_core.js:~418)
- **Result:** Tab now cycles wizard focusable elements even in split-view (via handleNavDown/handleNavUp at normal path)
- **Changed:** CLAUDE_RESERVED_KEYS: removed 'tab', added 'ctrl+tab' — Tab forwards to Claude PTY when Claude focused
- **Added:** `case 'tab': return '\t'` in keyToTermBytes for PTY forwarding
- **Updated:** Help overlay: added "Claude Integration" section with Ctrl+Tab, Ctrl+L, Ctrl+]/Ctrl±
- **Updated:** Pane divider hint: "Ctrl+Tab: switch  Ctrl+L: close"
- **Preserved:** focusIndex NOT reset on Ctrl+L toggle (was already correct)
- **Updated:** 2 existing tests (SplitView_TabFocusSwitch, TabBehaviorInSplitView) — Tab→Ctrl+Tab
- 7 new tests: TabCyclesFocusInSplitViewWizard, CtrlTabSwitchesPanes, TabForwardedToClaudePTY, CtrlLPreservesFocusIndex, HelpOverlayBindings, PaneDividerHint, TabInClaudeFocusDoesNotCycleWizard

## T39 Changes (this session)
### Fix expand/collapse state management — per-item, not global reset
- **Fixed:** verify-collapse guard: only fires when `vbranch === s.expandedVerifyBranch` (defensive)
- **Added:** Escape collapses `expandedVerifyBranch` and `showAdvanced` before back-navigation in `handleBack`
- **Normalized:** Chevrons: Advanced Options now uses ▶/▼ (U+25B6/U+25BC) matching verify output (was ▸/▾)
- **Fixed:** Expanded Advanced Options header now has `zone.mark('toggle-advanced', ...)` for clickable collapse
- **Note:** `expandedSplitIdx` does NOT exist — plan review has no expand/collapse, only selection
- 6 new tests: VerifyCollapseGuard, AccordionBehavior, EscapeCollapsesBeforeBackNav, AdvancedOptionsToggle, ChevronConsistency, ExpandResetOnExecution

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
6. ~~**Tab BROKEN in split-view**~~ → **FIXED T38 — Tab cycles wizard elements, Ctrl+Tab switches panes**
7. ~~**Expand/collapse BROKEN**~~ → **FIXED T39 — collapse guard, escape collapses, chevrons normalized**
8. **Integration tests SHALLOW**: no wizard+real-Claude, no Mux lifecycle, no TUI rendering tests

## Next Steps
1. **IMMEDIATE:** Commit T41 (Rule of Two PASSED)
2. T42: Default to Claude strategy when Claude is available
3. Continue T43-T72 sequentially
