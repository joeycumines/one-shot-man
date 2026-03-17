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
- **T313**: Done ✅ — committed `cf793425` — split pr_split_14_tui_commands.js into 2 files
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
- **T314 Cleanup**: Done ✅ — committed `2715a153` — removed 33 dead IIFE-scope vars across 8 files
  - Files: 10c, 15a, 15b, 15c, 15d, 16a, 16e, 16f
  - Rule of Two: Pass 1 FAIL→fix→Pass 1v2 FAIL→fix→Pass 1v3 PASS + Pass 2 PASS ✓
- **Next**: T315 (design PR Split test package structure)

---

## T315 Design: PR Split Test Package Structure

### Current State Audit

**68 test files** in `internal/command/`, all `package command`. Total internal/command test time: ~1011s.
(Note: `pr_split_test.go` has no Test functions — it's purely a helper/type definitions file, but it is still a `_test.go` file.)

**Engine Creation Patterns** (4 main chains, all rooted in unexported internals):

| Pattern | Defined In | Used By | Depends On |
|---------|-----------|---------|------------|
| `loadChunkEngine(t, overrides, chunkNames...)` | 00_core_test.go | ~15 files | `scriptCommandBase.PrepareEngine` (unexported), `prSplitChunks` (unexported) |
| `loadTUIEngine(t)` | 13_tui_test.go | ~4 files | `loadChunkEngine` + `allChunksThrough12` + `setupTUIMocks` const |
| `loadTUIEngineWithHelpers(t)` | 16_helpers_test.go | ~20 files | `loadTUIEngine` + `chunk16Helpers` const |
| `loadPrSplitEngineWithEval(t, overrides)` | pr_split_test.go | ~25 files | `scriptCommandBase.PrepareEngine` + `loadChunkedScript` (unexported) |

**Cross-file Helpers** (shared across multiple test files):

| Helper | Defined In | Used In (count) |
|--------|-----------|-----------------|
| `setupTestGitRepo` | pr_split_test.go | 3 files (21× total) |
| `setupTestPipeline` / `chdirTestPipeline` | pr_split_test.go | ~15 files |
| `gitMockSetupJS` | pr_split_verification_test.go | 6 files (31× total) |
| `jsStringLiteral` | pr_split_pipeline_test.go | 2 files |
| `numVal` | 16_helpers_test.go | 2 files |
| `buildOSMBinary` | pr_split_pty_unix_test.go | 3 files (unix-only) |
| `safeBuffer` | pr_split_test.go | 3 files |
| `TestPipeline` / `TestPipelineOpts` / `TestPipelineFile` | pr_split_test.go | ~15 files |

**Build Tags**: 4 files have `//go:build unix` (all PTY-based E2E tests).

### Fundamental Constraint: Go Import Cycles

The test files are `package command` (internal tests), NOT `package command_test` (external tests). This means **any package imported by these test files cannot itself import `internal/command`** — Go detects import cycles at the package level, regardless of test vs production code boundaries.

**Consequence**: The `prsplittest` helper package **must NOT import `internal/command`**. It must provide engine creation and chunk loading using only public APIs.

### Feasibility Analysis: Public API Engine Creation

`PrepareEngine` (the unexported method used by all engine helpers) calls:
1. `scripting.NewEngineDetailed(ctx, stdout, stderr, session, store, logFile, bufSize, level, opts...)` — **PUBLIC**
2. `resolveLogConfig(...)` — unexported, but tests need only defaults (nil logFile, LevelInfo)
3. `maybeStartCleanupScheduler(...)` — unexported, **not needed** for tests
4. `engine.SetTestMode(true)` — public, **not needed** for chunk tests
5. `injectConfigHotSnippets(...)` — unexported, **not needed** for tests
6. `modulePathOpts(cfg)` — returns nil for default config, **not needed**

**Verdict**: Engine creation is fully replicable using `scripting.NewEngineDetailed()` + defaults. ✅

### Feasibility Analysis: Chunk Loading Without Importing command

The `prSplitChunks` variable (unexported) contains `{name, *source}` pairs. In the helper package, we need the chunk JS content.

**Solution**: Read `pr_split_*.js` files from disk at test time using `os.ReadFile()` + lexicographic filename sort. This works because:
- Filenames are numerically prefixed: `00_core`, `01_analysis`, ..., `10a_pipeline_config`, `10b_pipeline_send`, ..., `16f_tui_model`
- Lexicographic sort produces correct load order: `10a` < `10b` < `10c` < `10d` < `11` (because '0' < '1' at position 1)
- The `internal/command/` directory path is discoverable via `runtime.Caller(0)` or `go list -m -json`

**Tradeoff**: Chunk ordering is implicitly defined by filenames rather than the explicit `prSplitChunks` array. If a chunk is renamed to break sort order, tests will fail — but this is actually a GOOD invariant to enforce.

### Target Package Structure

```
internal/command/prsplittest/
├── engine.go       — NewTestEngine, LoadChunks, MakeEvalJS, ChunkNames
├── eval.go         — EvalJS (convenience), EvalJSTimeout
├── repo.go         — InitTestRepo, RunGitCmd, GitBranchList, SetupTestGitRepo
├── pipeline.go     — TestPipeline, TestPipelineOpts, TestPipelineFile, SetupTestPipeline
├── tui.go          — NewTUIEngine, NewTUIEngineWithHelpers, SetupTUIMocks, Chunk16Helpers
├── types.go        — SafeBuffer, ChunkEntry
├── assertions.go   — NumVal, AssertContains, GitMockSetupJS
└── doc.go          — Package documentation
```

**Import graph**:
```
prsplittest → internal/scripting, internal/config  (NO import of internal/command)
command test files → prsplittest  (NO cycle)
```

### Functions to Export from prsplittest

#### Engine Layer (engine.go + types.go)

| Export | Replaces | Signature |
|--------|----------|-----------|
| `ChunkEntry` | anonymous struct in prSplitChunks | `type ChunkEntry struct { Name string; Source string }` |
| `SafeBuffer` | safeBuffer type | `type SafeBuffer struct { ... }` (exported, thread-safe) |
| `NewTestEngine(t)` | `loadChunkEngine` engine setup | `func(testing.TB, ...EngineOption) *Engine` |
| `Engine.LoadChunks(names...)` | chunk-by-name loading | method on wrapper |
| `Engine.EvalJS(code)` | `makeEvalJS` result | method returning `(any, error)` |
| `Engine.SetGlobal(k, v)` | `engine.SetGlobal` | delegates to underlying scripting.Engine |
| `ChunkNames()` | `allChunksThrough12`, `allChunksThrough11` | `func() []string` — returns all discovered chunk names |
| `ChunkNamesThrough(prefix)` | `allChunksThrough12` | `func(string) []string` — returns chunks up to prefix |

#### TUI Layer (tui.go)

| Export | Replaces | Signature |
|--------|----------|-----------|
| `SetupTUIMocks` | const in 13_tui_test.go | `const string` (JS source) |
| `Chunk16Helpers` | const in 16_helpers_test.go | `const string` (JS source) |
| `NewTUIEngine(t)` | `loadTUIEngine` | `func(testing.TB) *Engine` — loads all chunks through 12, injects mocks, loads 13-16 |
| `NewTUIEngineWithHelpers(t)` | `loadTUIEngineWithHelpers` | `func(testing.TB) *Engine` — extends NewTUIEngine with Chunk16Helpers |

#### Repo Layer (repo.go)

| Export | Replaces | Signature |
|--------|----------|-----------|
| `RunGitCmd(t, dir, args...)` | `runGitCmd` | `func(testing.TB, string, ...string) string` |
| `InitTestRepo(t)` | `setupTestGitRepo` | `func(testing.TB) string` |
| `GitBranchList(t, dir)` | `gitBranchList` | `func(testing.TB, string) []string` |
| `GitMockSetupJS()` | `gitMockSetupJS` | `func() string` |

#### Pipeline Layer (pipeline.go)

| Export | Replaces | Signature |
|--------|----------|-----------|
| `TestPipeline` | struct in pr_split_test.go | exported struct |
| `TestPipelineOpts` | struct in pr_split_test.go | exported struct |
| `TestPipelineFile` | struct in pr_split_test.go | exported struct |
| `SetupTestPipeline(t, opts)` | `setupTestPipeline` | `func(testing.TB, TestPipelineOpts) *TestPipeline` |

**Note**: `setupTestPipeline` uses `loadPrSplitEngineWithEval` internally, which accesses `loadChunkedScript` (unexported). The prsplittest version will replicate this using its own engine creation + chunk loading from disk. The behavior is functionally identical since the JS chunks are the same source.

### Migration Plan by Test File

#### Phase 1: Unit Tests (T317) — 14 chunk-level files

| File | Current Helpers Used | Migration |
|------|---------------------|-----------|
| pr_split_00_core_test.go | **Defines** loadChunkEngine, makeEvalJS | Keep as thin wrapper calling prsplittest.NewTestEngine |
| pr_split_01_analysis_test.go | loadChunkEngine, gitInit, gitRun | Use prsplittest.NewTestEngine + prsplittest.RunGitCmd |
| pr_split_02_grouping_test.go | loadChunkEngine | Use prsplittest.NewTestEngine |
| pr_split_03_planning_test.go | loadChunkEngine | Use prsplittest.NewTestEngine |
| pr_split_04_validation_test.go | loadChunkEngine + local evalValidation | Use prsplittest, keep evalValidation local |
| pr_split_05_execution_test.go | loadChunkEngine + local setupExecRepo | Use prsplittest, keep local setup |
| pr_split_06_verification_test.go | loadChunkEngine + local setup | Use prsplittest, keep local helpers |
| pr_split_07_prcreation_test.go | loadChunkEngine | Use prsplittest.NewTestEngine |
| pr_split_08_conflict_test.go | loadChunkEngine | Use prsplittest.NewTestEngine |
| pr_split_09_claude_test.go | loadChunkEngine | Use prsplittest.NewTestEngine |
| pr_split_10_pipeline_test.go | loadChunkEngine | Use prsplittest.NewTestEngine |
| pr_split_11_utilities_test.go | loadChunkEngine | Use prsplittest, retire allChunksThrough11 |
| pr_split_12_exports_test.go | loadChunkEngine | Use prsplittest, retire allChunksThrough12 |
| pr_split_template_unit_test.go | loadPrSplitEngineWithEval | Use prsplittest.NewTestEngine (load all) |

#### Phase 2: TUI Tests (T318) — ~25 files

| File Group | Current | Migration |
|------------|---------|-----------|
| pr_split_13_tui_test.go | **Defines** loadTUIEngine, setupTUIMocks | Keep thin: calls prsplittest.NewTUIEngine |
| pr_split_15_tui_views_test.go | loadTUIEngine | Use prsplittest.NewTUIEngine |
| pr_split_16_helpers_test.go | **Defines** loadTUIEngineWithHelpers | Keep thin: calls prsplittest.NewTUIEngineWithHelpers |
| pr_split_16_*_test.go (18 files) | loadTUIEngineWithHelpers, numVal | Use prsplittest directly |
| pr_split_tui_hang_test.go | loadTUIEngineRaw + manual wiring | Use prsplittest.NewTUIEngine variant |
| pr_split_tui_subcommands_test.go | chdirTestPipeline | Use prsplittest helpers |
| pr_split_edge_hardening_test.go | mixed (eval + TUI + pipeline) | Use appropriate prsplittest helpers |

#### Phase 3: Pipeline/Integration/E2E Tests (T317 continued) — ~26 files

| File | Current | Migration |
|------|---------|-----------|
| pr_split_verification_test.go | loadPrSplitEngineWithEval + **defines** gitMockSetupJS | Move gitMockSetupJS to prsplittest |
| pr_split_pipeline_test.go | loadPrSplitEngineWithEval + **defines** jsStringLiteral | Use prsplittest, keep local parsers |
| pr_split_grouping_test.go | loadPrSplitEngineWithEval | Use prsplittest |
| pr_split_planning_test.go | loadPrSplitEngineWithEval + gitMockSetupJS | Use prsplittest |
| pr_split_execution_test.go | loadPrSplitEngineWithEval + gitMockSetupJS | Use prsplittest |
| pr_split_analysis_test.go | loadPrSplitEngineWithEval + gitMockSetupJS | Use prsplittest |
| pr_split_mode_autofix_test.go | loadPrSplitEngineWithEval + setupTestGitRepo | Use prsplittest |
| pr_split_bt_test.go | loadPrSplitEngineWithEval + setupTestPipeline | Use prsplittest |
| pr_split_autosplit_recovery_test.go | loadPrSplitEngineWithEval + chdirTestPipeline | Use prsplittest |
| pr_split_benchmark_test.go | loadPrSplitEngineWithEval + setupTestPipeline | Use prsplittest |
| pr_split_local_integration_test.go | multiple variants | Use appropriate prsplittest helpers |
| pr_split_corruption_test.go | loadChunkEngine + setupTestPipeline | Use prsplittest |
| pr_split_createprs_test.go | loadPrSplitEngineWithEval | Use prsplittest |
| pr_split_pipeline_smoke_test.go | loadChunkEngine (allChunksThrough13) | Use prsplittest.ChunkNamesThrough("13") |
| pr_split_conflict_retry_test.go | loadPrSplitEngineWithEval + setupTestGitRepo | Use prsplittest |
| pr_split_prompt_test.go | loadPrSplitEngineWithEval + setupTestPipeline | Use prsplittest |
| pr_split_heuristic_run_test.go | setupTestGitRepo + **local** setup variants | Use prsplittest.InitTestRepo, keep local variants |
| pr_split_autofix_strategy_test.go | loadPrSplitEngineWithEval | Use prsplittest |
| pr_split_claude_config_test.go | runGitCmd + **local** setupDependencyGoRepo | Use prsplittest.RunGitCmd, keep local setup |
| pr_split_session_cancel_test.go | loadPrSplitEngineWithEval + setupTestPipeline + gitMockSetupJS | Use prsplittest |
| pr_split_scope_misc_test.go | loadPrSplitEngineWithEval | Use prsplittest |
| pr_split_wizard_integration_test.go | loadPrSplitEngineWithEval | Use prsplittest |
| pr_split_integration_test.go | setupTestPipeline + **local** runGit | Use prsplittest, keep local runGit |
| pr_split_complex_project_test.go | Standalone git repo tests | Minimal change (uses own git setup) |
| pr_split_cmd_meta_test.go | Standalone — direct NewPrSplitCommand() | No migration needed (no shared helpers used) |

#### Phase 4: E2E/PTY Tests (unix-only, T319 build tag tagging + migration)

| File | Current | Migration |
|------|---------|-----------|
| pr_split_pty_unix_test.go | **Defines** buildOSMBinary, osmBinaryOnce/Path/Err | Keep as-is (defines unix-only helpers) |
| pr_split_binary_e2e_test.go | buildOSMBinary + **local** helpers | Add `//go:build prsplit_slow && unix` tag |
| pr_split_tui_pty_hang_test.go | buildOSMBinary + **local** helpers | Add `//go:build prsplit_slow && unix` tag |
| pr_split_termmux_observation_test.go | buildOSMBinary | Add `//go:build prsplit_slow && unix` tag |

#### Helper Source File Fate: `pr_split_test.go`

After migration, `pr_split_test.go` (the helper-only `_test.go` file defining `setupTestGitRepo`, `safeBuffer`, `TestPipeline*`, `loadPrSplitEngine*`, etc.) will be **reduced to thin wrappers**:
- `safeBuffer`, `TestPipeline`, `TestPipelineOpts`, `TestPipelineFile` → moved to `prsplittest/types.go`
- `setupTestGitRepo`, `setupTestPipeline`, `chdirTestPipeline` → moved to `prsplittest/repo.go` and `prsplittest/pipeline.go`
- `loadPrSplitEngine`, `loadPrSplitEngineWithEval` → replaced by `prsplittest.NewTestEngine` variants
- `runGitCmd`, `gitBranchList`, `filterPrefix` → moved to `prsplittest/repo.go`
- File will either be **deleted entirely** or reduced to a compatibility shim that delegates to prsplittest (final decision during T317 execution).

### Test Binary Splitting Strategy (T319)

**Approach**: Build tags + Make targets (NOT separate packages).

**Rationale**: Separate test packages require exporting or bridging all internals, adding complexity for marginal gain. Build tags within the same package keep tests in `package command` (full internal access) while allowing selective compilation.

#### Slow Tests (tagged `//go:build prsplit_slow`)

| File | Est. Time | Current Tags |
|------|-----------|-------------|
| pr_split_binary_e2e_test.go | ~120s | `//go:build unix` |
| pr_split_tui_pty_hang_test.go | ~60s | `//go:build unix` |
| pr_split_termmux_observation_test.go | ~90s | `//go:build unix` |
| pr_split_autosplit_recovery_test.go | ~120s | none |
| pr_split_integration_test.go | ~180s | none |
| pr_split_complex_project_test.go | ~60s | none |
| pr_split_wizard_integration_test.go | ~60s | none |
| pr_split_local_integration_test.go | ~120s | none |
| pr_split_benchmark_test.go | ~30s | none |

**Estimated slow total**: ~840s
**Estimated fast total**: ~170s (remaining unit + TUI tests)

#### Build Tag Design

```go
// Slow tests get BOTH their existing tag AND the new one:
//go:build unix && prsplit_slow    // for PTY tests
//go:build prsplit_slow            // for integration tests

// Default: tests WITHOUT the tag run in the "fast" pass.
// No tag changes needed for fast tests — they compile by default.
```

#### Make Targets (config.mk)

```makefile
.PHONY: test-prsplit-fast test-prsplit-slow test-prsplit-all test-prsplit-unit test-prsplit-tui test-prsplit-e2e

test-prsplit-fast:
	go test -timeout=300s -count=1 ./internal/command/... 2>&1 | fold -w 200 | tail -n 40

test-prsplit-slow:
	go test -timeout=600s -count=1 -tags=prsplit_slow ./internal/command/... 2>&1 | fold -w 200 | tail -n 40

test-prsplit-unit:
	go test -timeout=300s -count=1 -run 'TestCore|TestAnalysis|TestGrouping|TestPlanning|TestValidation|TestExecution|TestVerification|TestPRCreation|TestConflict|TestClaude|TestPipeline|TestUtilities|TestExports|TestTemplate' ./internal/command/... 2>&1 | fold -w 200 | tail -n 40

test-prsplit-tui:
	go test -timeout=600s -count=1 -run 'TestTUI|TestViews|TestFocus|TestBench|TestKeyboard|TestMouse|TestOverlay|TestSplitView|TestAsync|TestHang|TestAutoSplit|TestLifecycle|TestConfig|TestClaude|TestPreexisting|TestRestart|TestSync|TestVerify|TestVterm' ./internal/command/... 2>&1 | fold -w 200 | tail -n 40

test-prsplit-e2e:
	go test -timeout=600s -count=1 -tags=prsplit_slow -run 'TestBinaryE2E|TestTermmux|TestPTY' ./internal/command/... 2>&1 | fold -w 200 | tail -n 40

test-prsplit-all:
	go test -timeout=900s -count=1 -tags=prsplit_slow ./internal/command/... 2>&1 | fold -w 200 | tail -n 60
```

### Expected Performance Improvement

| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| Full suite (`test-prsplit-all`) | ~1011s | ~1011s | 0% (same tests) |
| Fast feedback (`test-prsplit-fast`) | ~1011s | **~170s** | **83% faster** |
| Unit only (`test-prsplit-unit`) | ~1011s | **~90s** | **91% faster** |
| CI parallel (fast ∥ slow) | ~1011s | **~840s** | 17% (limited by slow) |

The primary win is the **fast feedback loop**: `make test-prsplit-fast` for sub-3-minute iteration.

### Blockers and Compromises

1. **Chunk ordering duplication**: `prsplittest` discovers chunk order via filename sorting, while production code uses the explicit `prSplitChunks` array. A rename that breaks sort order will cause test failures — this is acceptable (and desirable as a safety net).

2. **`setupTestPipeline` complexity**: This helper creates git repos, loads engines, sets up pipelines, and provides dispatch functions. Replicating this in `prsplittest` without importing `command` requires duplicating ~150 lines of setup logic. The alternative is to keep `setupTestPipeline` in a test file within `package command` that delegates to `prsplittest` for engine creation but handles pipeline-specific wiring locally.

3. **Build tag maintenance**: Adding `//go:build prsplit_slow` to slow test files is a one-time change. Future tests must be tagged appropriately — this is documented in the package's doc.go.

4. **No separate test binary**: Tests remain in one Go package, compiled to one binary. True binary-level parallelism would require separate packages, which is blocked by the import cycle constraint unless we export test-only functions from `command`. This is a deliberate tradeoff: simplicity over maximum parallelism.

5. **Disk I/O for chunk loading**: `prsplittest` reads `.js` files from disk (~500KB total) on every test engine creation. This adds ~1ms per engine creation — negligible vs the 30s+ test execution time.

### Decision: Start with Phased Implementation

- **T316**: Done ✅ — committed `77e6305f` — prsplittest package + 4 test migrations
  - Created 8-file package: doc.go, types.go, chunks.go, engine.go, eval.go, assertions.go, repo.go, tui.go
  - Migrated 4 test files: 02_grouping (10), 07_prcreation (6), 08_conflict (13), 09_claude (11) — 40 calls total
  - Deleted leftover pr_split_10_pipeline.js (2097 lines)
  - Fixed claudemux/pr_split_test.go chunk list (10→10a-10d)
  - Updated ADR-001 + prompt anchor stability doc
  - Added .deadcodeignore pattern for prsplittest/*
  - Build: PASS, Rule of Two: Pass 1 PASS + Pass 2 PASS ✓
  - M1 fix: NewTUIEngine uses eng.LoadChunks (not evalJS loop) for TUI chunks — preserves script names in stack traces
- **T317**: Next — migrate remaining unit test files to use prsplittest helpers
- **T318**: Migrate TUI tests (~25 files).
- **T319**: Add build tags to slow tests + Make targets. Measure timing.
