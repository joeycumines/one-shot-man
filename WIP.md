# WIP — Takumi's Desperate Diary

## Session 3 Start
2026-03-17 — Blueprint creation session for PR Split UI/termmux/refactor mandate

### Mandate (from Hana-san)
Fix 6 production issues:
1. EQUIV_CHECK button interactivity & visual rendering bugs
2. Cancel/Back button styling inconsistencies
3. Verify shell integration via termmux (FULLY-FEATURED MULTIPLEX TERMINAL)
4. Visual correctness audit across all screens
5. JS source file splitting (files too large)
6. Test package restructuring for performance

### Domain Model (First-Hand Knowledge)

#### EQUIV_CHECK Button Bugs (Root Causes CONFIRMED)
- **Not highlightable**: `viewVerificationScreen()` (pr_split_15_tui_views.js:1880-1890) NEVER applies focus styling. Always uses static `secondaryButton()`/`primaryButton()`. Other screens (config:1046, plan-review:1339, finalization:2002) DO compute focus state.
- **Not clickable**: Buttons ARE zone-marked, but string concatenation (`+`) of multi-line bordered elements produces broken zone boundaries. `secondaryButton()` renders 3-line output (border top/content/border bottom). When concatenated with `+`, subsequent buttons stack vertically instead of aligning horizontally.
- **nav-back missing**: `getFocusElements()` for EQUIV_CHECK (pr_split_16_tui_core.js:2469-2479) includes `[equiv-reverify, equiv-revise, nav-next, nav-cancel]` but NOT `nav-back`. Back button is zone-marked in renderNavBar and mouse-clickable, but unreachable via Tab.
- **Cancel borderless**: `renderNavBar()` uses `focusedButton()` (NO border) for focused Cancel/Back, but `secondaryButton()` (HAS roundedBorder) for unfocused. This causes 3-line→1-line layout shift on focus. Should use `focusedSecondaryButton()` (same border, different colors) per T011 pattern.
- **String concat vs joinHorizontal**: renderNavBar uses `lipgloss.joinHorizontal` (correct). viewVerificationScreen, viewPlanReviewScreen, viewFinalizationScreen all use `+` concatenation (BROKEN for bordered elements).

#### JS File Sizes (CONFIRMED)
- pr_split_16_tui_core.js: **5,089 lines** (29% of codebase!)
- pr_split_15_tui_views.js: **2,569 lines** (has T060 documented split plan)
- pr_split_10_pipeline.js: **2,097 lines**
- pr_split_14_tui_commands.js: **1,303 lines**
- Total 17 files, 17,670 lines

#### Termmux Shell Integration (CONFIRMED APPROACH)
- **Existing primitives**: `termmux.newCaptureSession(cmd, args, opts)`, `mux.attach(handle)`, `mux.switchTo()` (blocking passthrough), `mux.detach()`
- **No `spawnInteractive()` exists** — but all pieces are there in JS bindings
- **Shell button**: T211/T007 stub exists in pr_split_16_tui_core.js:5885-5920 (pause verify → TODO spawnInteractive → resume)
- **Design**: Create JS-level `spawnShellSession()` orchestrating existing primitives: newCaptureSession→detach Claude→attach shell→switchTo→cleanup→re-attach Claude

#### Test Structure (CONFIRMED)
- 68 PR Split test files in `package command` (internal/command/)
- internal/command: 1011s (17 min), internal/scripting: 477s (8 min)
- Tests access internal JS via eval (`prSplit._dirname()`, etc.)
- No separate test package exists
- Shared helpers duplicated across files

#### Chunk Loading Mechanism
- `//go:embed` directives in pr_split.go (lines 25-79)
- `prSplitChunks` ordered array (lines 82-106)
- `loadChunkedScript()` iterates array (lines 108-119)
- Adding new chunks: add embed directive + array entry, NO other changes needed

### Blueprint: 36 tasks (T300-T335)
- Block A (T300-T307): EQUIV_CHECK button fixes + visual audit
- Block B (T308): State management hardening
- Block C (T320-T330): Termmux shell integration (feature)
- Block D (T309): Claude mux investigation
- Block E (T310-T314): JS file splitting
- Block F (T315-T319): Test package restructuring
- Block G (T331-T335): Infrastructure, benchmarks, docs, finalization

### Blueprint Review Status
- **Pass 1**: FAIL → 5 findings (C1,C2,M1,M2,M3) → ALL FIXED
- **Pass 2**: FAIL → 4 findings (C3,M4,M5,L6) → ALL FIXED
- **Pass 3**: FAIL → 3 critical (T312 fabricated classifyWithClaude/planWithClaude, T310 fabricated viewPlanningScreen/viewSplitDetailScreen/viewFileManagementScreen, T310 wrong name viewCancelConfirm→viewConfirmCancelOverlay) + 3 moderate (T310 15b missing functions, T300 already implemented, line numbers) → ALL FIXED
- **Pass 4**: **PASS** — 1 LOW finding (file line count off by 10)
- **Pass 5**: **PASS** — 1 LOW finding (T311 handler names self-mitigating)
- **RULE OF TWO: SATISFIED** ✅ (Passes 4+5 contiguous on same diff)

### Blueprint Status: FINALIZED
36 tasks (T300-T335), 6 blocks (A-F), all verified against source code.
Ready for execution.

### Session Progress
- **T300**: Done ✅ — committed `5a6efad4` (EQUIV_CHECK focus styling)
- **T301**: Done ✅ — committed `274e460d` (nav-back in EQUIV_CHECK focus elements)
- **T302**: Done ✅ — committed `48ea09ba` (Back/Cancel use focusedSecondaryButton)
- **T302b**: Done ✅ — cross-build verified: linux/amd64, darwin/amd64, windows/amd64 all OK
- **T303+T304+T305**: Done ✅ — committed `b8cc14f1` (joinHorizontal for EQUIV_CHECK, PlanReview, Finalization)
- **T306**: Done ✅ — committed `1abfae19` (full visual audit: 6 more joinHorizontal fixes + AllScreens_NoBrokenBorders test)
  - BUG1: viewPlanEditorScreen editor-move/rename/merge
  - BUG2: viewExecutionScreen verify footer (conditional openShellBtn)
  - BUG3-5: Move/Rename/Merge dialogs confirm+cancel
  - BUG6: PAUSED screen resume+quit
  - Rule of Two: Pass 1 FAIL (test gaps) → fixed → Pass 2 PASS + Pass 3 PASS ✓
- **T307**: Done ✅ — committed `9d263936` (EQUIV_CHECK mouse click tests)
- **T308**: Done ✅ — committed `04857394` (equiv state cleanup on back-navigation)
  - Fixed handleBack, mouse equiv-revise, keyboard equiv-revise
  - Added async guards in runEquivCheckAsync
  - 4-scenario test: mouse back, keyboard back, keyboard revise, isProcessing guard
- **T309**: Done ✅ — Ctrl+] Claude switching fix
  - Root cause: `tuiMux.hasChild()` returns false when Claude not attached, handler silently no-ops
  - Status bar unconditionally showed "Ctrl+] Claude" regardless of child attachment
  - Fix 1: Status bar now conditional on `tuiMux.hasChild()` (pr_split_15_tui_views.js)
  - Fix 2: Ctrl+] handler logs diagnostics + flashes notification when Claude unavailable (pr_split_16_tui_core.js)
  - 8-scenario test: 4 key handler tests + 4 status bar tests
- **Next**: T310 (JS file splitting)
- **T310**: Done ✅ — Split pr_split_15_tui_views.js into 4 files
  - 15a_tui_styles.js: 274 lines (styles, colors, layout, shared utilities)
  - 15b_tui_chrome.js: 692 lines (title bar, nav bar, status bar, panes)
  - 15c_tui_screens.js: 985 lines (6 screen renderers — justified hub file)
  - 15d_tui_dialogs.js: 688 lines (finalization, dialogs, overlays, viewForState)
  - Original pr_split_15_tui_views.js deleted
  - All tests pass, full build clean
- **Next**: T311 (split pr_split_16_tui_core.js)
- **T311**: Done ✅ — committed `849523ab` — Split pr_split_16_tui_core.js into 6 files
  - 16a_tui_focus.js: 974 lines (focus, navigation, dialog update, viewport sync)
  - 16b_tui_handlers_pipeline.js: 825 lines (analysis, execution, equiv, PR creation)
  - 16c_tui_handlers_verify.js: 879 lines (verify, error resolution, confirm cancel, Claude conversation)
  - 16d_tui_handlers_claude.js: 839 lines (Claude automation, key bytes, question detection, screenshot)
  - 16e_tui_update.js: 924 lines (main update dispatch — wizardUpdateImpl extracted from _updateFn)
  - 16f_tui_model.js: 939 lines (BubbleTea model, mouse, view, launch)
  - Key refactoring: _updateFn/viewFn extracted from createWizardModel closure as standalone functions
  - Original pr_split_16_tui_core.js (5126 lines) deleted
  - All tests pass, full build clean
  - Rule of Two: Pass 1 PASS + Pass 2 PASS ✓
- **Next**: T312 (split pr_split_10_pipeline.js)
- **T312**: Done ✅ — Split pr_split_10_pipeline.js into 4 files
  - 10a_pipeline_config.js: 246 lines (AUTOMATED_DEFAULTS, SEND_* constants, utility funcs)
  - 10b_pipeline_send.js: 483 lines (captureScreenshot, prompt detection, anchor stability, sendToHandle)
  - 10c_pipeline_resolve.js: 400 lines (waitForLogged, heuristicFallback, resolveConflictsWithClaude)
  - 10d_pipeline_orchestrator.js: 995 lines (automatedSplit — justified hub file >800 lines)
  - Critical fix: added cross-chunk IIFE-scope imports in 10c (isTransientError, AUTOMATED_DEFAULTS, etc.)
  - Critical fix: added 9 late-bind entries in 10d for 10a/10b/10c functions
  - Original pr_split_10_pipeline.js (2097 lines) deleted
  - All tests pass, full build clean
  - Rule of Two: Pass 1 PASS + Pass 2 PASS ✓
- **Next**: T313 (split pr_split_14_tui_commands.js)
- **T313**: IN PROGRESS — split pr_split_14_tui_commands.js into 2 files
  - 14a_tui_commands_core.js: ~630 lines (17 core workflow commands: analyze through run)
  - 14b_tui_commands_ext.js: 709 lines (15 ext commands + buildCommands orchestrator + HUD overlay + bell handler)
  - buildCommands split: 14a exports _buildCoreCommands, 14b imports and merges via Object.assign pattern
  - Original file deleted via git rm
  - Updated: pr_split.go (embed directives + prSplitChunks), pr_split_13_tui_test.go (3 locations), pr_split_tui_hang_test.go (1 location)
  - Fixed dead vars in 14a (removed tuiState, WizardState, handleConfigState, handleBaselineFailState — only used in 14b)
  - Fixed stale comment in pr_split_binary_e2e_test.go (referenced old filename)
  - Fixed config.mk .PHONY mismatch (was git-rm-old-chunk16, now git-rm-old-chunks)
  - Build: PASS (zero FAIL)
  - Rule of Two: Pass 1 (first attempt) FAIL → 4 issues fixed → Pass 1 (retry) PASS + Pass 2 PASS ✓
  - Committed as `cf793425`
- **T314**: Done ✅ — Chunk cross-reference and closure variable audit
  - Full report written to `scratch/T314-audit.md`
  - **BLOCKING issues: 0** — zero runtime errors, zero missing exports, zero circular deps
  - **NON-BLOCKING: 17 dead variables** across 7 files (IIFE-scope imports never used in file body)
  - **Clean files: 9 of 16** — zero issues
  - Loading order: verified correct (strict dependency ordering)
  - Late-bind patterns: all 7 shim sites verified correct (all reference later-loaded chunks)
  - Native module access: verified correct (Go globals never accessed via prSplit namespace)
  - Export completeness: verified (all imports resolve against earlier-chunk exports)
  - Recommended fix: delete each dead `var x = prSplit._xxx;` line — trivial one-liner per var
- **Next**: T315 (design PR Split test package structure)
