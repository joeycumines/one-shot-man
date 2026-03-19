# WIP — Takumi's Desperate Diary

## Session 7 — Continued

### T367 Complete
- Added `t.Parallel()` to ~270 tests across 16 test files
- Reverted pr_split_tui_subcommands_test.go (35 tests) — uses os.Chdir via chdirTestPipeline
- Excluded TestParseClaudeEnv_MalformedInput — mutates slog.Default
- Race test: `go test -race -count=3` PASSED (128s)
- Cross-build: PASSED
- Rule of Two: Pass 1 had FAIL (caught subcommands os.Chdir), fixed, rerun. Pass 1 PASS, Pass 2 PASS.

### Commits (Session 7)
- 96b686bf: T365 fuzz tests (from session 6)
- 63fae190: T367 t.Parallel() ~270 tests
- 5fb3092b: T368 CLI flag E2E test
- 3bd8acfa: T371 duplicate test file cleanup
- (pending): T370 wizard integration test fix

### Next Tasks
- T369: Comment normalization
- T366: automatedSplit extraction (deferred — risky)

## Session 7 — t.Parallel() Research Report

### RESEARCH ONLY — t.Parallel() Candidate Analysis for `internal/command/pr_split_*_test.go`

**SCOPE:** 77 test files matching `pr_split_*_test.go` in `internal/command/`
**METHOD:** Systematic `grep_search` for `^func Test`, `t.Parallel()`, `os.Chdir` per file. Manual verification of false positives (e.g. `func Test` inside string literals).

---

## EXECUTIVE SUMMARY

| Category | Files | Tests | Actionable |
|----------|-------|-------|------------|
| A: Already 100% parallel | 38 | ~600 | No |
| B: Has candidates for t.Parallel() | 30 | ~411 candidates | **YES** |
| C: Unsafe (os.Chdir, cannot parallelize) | 5 | ~47 blocked | No |
| D: N/A (fuzz/bench/helpers/excluded) | 4 | — | No |

**Top Wins (safe, high candidate count):**
| Rank | File | Candidates | Risk |
|------|------|-----------|------|
| 1 | pr_split_13_tui_test.go | 94 | LOW — per-test engine, no shared state |
| 2 | pr_split_11_utilities_test.go | 36 | LOW — pure functions |
| 3 | pr_split_tui_subcommands_test.go | 35 | LOW — per-test engine, no shared state |
| 4 | pr_split_04_validation_test.go | 26 | LOW — pure validation |
| 5 | pr_split_autosplit_recovery_test.go | 26 | MEDIUM — integration tests, no os.Chdir |
| 6 | pr_split_cmd_meta_test.go | 14 | LOW — metadata/flag tests |
| 7 | pr_split_00_core_test.go | 13 | LOW — core unit tests |
| 8 | pr_split_08_conflict_test.go | 13 | LOW — chunk tests, per-test engine |
| 9 | pr_split_03_planning_test.go | 13 | LOW — uses os.WriteFile to t.TempDir() |

---

## CATEGORY A: ALREADY 100% PARALLEL ✅ (38 files, no action needed)

| File | Tests | Parallel |
|------|-------|----------|
| pr_split_15_tui_views_test.go | 62 | 62 |
| pr_split_16_config_output_test.go | 26 | 26 |
| pr_split_16_vterm_claude_pane_test.go | 22 | 22 |
| pr_split_16_vterm_lifecycle_test.go | 21 | 21 |
| pr_split_16_vterm_key_forwarding_test.go | 22 | 22 |
| pr_split_15_verify_pane_test.go | 6 | 6 |
| pr_split_16_overlays_test.go | 19 | 19 |
| pr_split_16_focus_nav_edge_test.go | 27 | 27 |
| pr_split_16_verify_fixes_test.go | 5 | 5 |
| pr_split_16_split_mouse_test.go | 22 | 22 |
| pr_split_16_mouse_bytes_test.go | 10 | 10 |
| pr_split_16_input_routing_test.go | 7 | 7 |
| pr_split_16_sync_avail_test.go | 6 | 6 |
| pr_split_16_async_pipeline_test.go | 35 | 35 |
| pr_split_16_restart_claude_test.go | 5 | 5 |
| pr_split_16_claude_attach_test.go | 42 | 42 |
| pr_split_16_keyboard_crash_test.go | 23 | 23 |
| pr_split_16_ctrl_bracket_test.go | 4 | 4+subtests |
| pr_split_template_unit_test.go | 18 | 18 |
| pr_split_grouping_test.go | 36 | 36 |
| pr_split_planning_test.go | 8 | 8 |
| pr_split_pipeline_test.go | 6 | 6 |
| pr_split_verification_test.go | 28 | 28 |
| pr_split_autofix_strategy_test.go | 16 | 16 |
| pr_split_conflict_retry_test.go | 50 | 50 |
| pr_split_createprs_test.go | 24 | 24 |
| pr_split_analysis_test.go | 31 | 31 |
| pr_split_execution_test.go | 9 | 9 |
| pr_split_15_golden_test.go | 5 | 5 |
| pr_split_16_preexisting_test.go | 9 | 9 |
| pr_split_16_e2e_lifecycle_test.go | 3 | 3 |
| pr_split_16_analysis_hang_test.go | 5 | 5 |
| pr_split_16_auto_split_equiv_test.go | 4 | 4 |
| pr_split_16_verify_expand_nav_test.go | 31 | 31 |
| pr_split_16_bench_test.go | 1 | 1 |
| pr_split_pipeline_smoke_test.go | 2 | 2 |
| pr_split_06b_shell_test.go | 6 | 6 |
| pr_split_06b_shell_degradation_test.go | 1 | 1+subtests |

---

## CATEGORY B: CANDIDATES FOR t.Parallel() (30 files, sorted by candidate count)

### TIER 1 — Safe, High Impact (LOW risk, ≥10 candidates)

**1. pr_split_13_tui_test.go — 94 candidates** (147 total, 53 already parallel)
- Engine: `NewTUIEngine(t)` / `NewChunkEngine(t, ...)` per test
- The first 53 parallel tests start around line 1037. Lines 32–1036 contain the 94 non-parallel tests
- No os.Chdir, no os.WriteFile, no shared state → **SAFE**

**2. pr_split_11_utilities_test.go — 36 candidates** (36 total, 0 parallel)
- Pure utility/helper function tests
- No os.Chdir, no os.WriteFile, no shared state → **SAFE**

**3. pr_split_tui_subcommands_test.go — 35 candidates** (35 total, 0 parallel)
- Per-test engine creation, command dispatch tests
- No os.Chdir, no os.WriteFile → **SAFE**

**4. pr_split_04_validation_test.go — 26 candidates** (26 total, 0 parallel)
- Pure validation logic tests
- No os.Chdir, no os.WriteFile → **SAFE**

**5. pr_split_cmd_meta_test.go — 14 candidates** (19 total, 5 parallel)
- Flag parsing, metadata, config tests
- No os.Chdir → **SAFE**
- Candidates: NonInteractive, Name, Description, Usage, SetupFlags, FlagParsing, FlagShortForm, FlagDefaults, FlagValidation, ExecuteWithArgs, ConfigColorOverrides, NilConfig, EmbeddedContent, ParseClaudeEnv_MalformedInput

**6. pr_split_00_core_test.go — 13 candidates** (13 total, 0 parallel)
- Core chunk tests
- No os.Chdir, no os.WriteFile → **SAFE**

**7. pr_split_08_conflict_test.go — 13 candidates** (13 total, 0 parallel)
- Conflict resolution chunk tests
- No os.Chdir → **SAFE**

**8. pr_split_03_planning_test.go — 13 candidates** (13 total, 0 parallel)
- Planning chunk tests
- Uses os.WriteFile to `t.TempDir()` paths (safe) — no os.Chdir → **SAFE**

**9. pr_split_02_grouping_test.go — 10 candidates** (10 total, 0 parallel)
- Grouping chunk tests
- No os.Chdir → **SAFE**

### TIER 2 — Moderate Risk (needs code review, no os.Chdir found)

**10. pr_split_autosplit_recovery_test.go — 26 candidates** (44 total, 17 parallel, 1 os.Chdir)
- os.Chdir ONLY in TestIntegration_PlanPersistence_RoundTrip (line 1335) — that test is excluded
- Remaining 26 candidates are MockMCP integration tests and unit tests
- ⚠️ REVIEW: Ensure MockMCP tests don't have hidden shared state
- Candidates: PipelineTimeout, SaveAndResume, CrashRecovery_AfterExecute, AutoSplitMockMCP, AllStepsReportTiming, HeuristicFallback_Report, CleanupOnFailure, CleanupOnFailure_Disabled, ErrorFeedback_ResumeInstructions, ResumeClaudeResolveFails, StepTimeout, AutoSplitMockMCP_DoubleInvocation, OverlappingFiles, VerifyFailure, CancelDuringExecution, ConflictResolution, WatchdogTimeout, ErrorRecovery_ClassificationTimeout, ErrorRecovery_PlanFallbackToLocal, ErrorRecovery_ExecutionFailure, ErrorRecovery_AllBranchesFailVerify, MalformedClassification, PartialClassification, EmptyCategories, MalformedPlan, LateClassification

**11. pr_split_termmux_observation_test.go — 12 candidates** (12 total, 0 parallel)
- VTerm observation integration tests
- No os.Chdir → likely safe
- ⚠️ REVIEW: These create TUI engines with real VTerm - check for pty resource conflicts

**12. pr_split_binary_e2e_test.go — 12 candidates** (12 total, 0 parallel)
- Binary E2E tests that build & run the actual binary
- No os.Chdir → likely safe
- ⚠️ REVIEW: May compile binary to shared path; check for port/resource conflicts

**13. pr_split_09_claude_test.go — 8 candidates** (11 total, 3 parallel)
- Claude executor chunk tests
- No os.Chdir → **SAFE**
- Candidates: Construct, Resolve_ExplicitNotFound, Resolve_ExplicitFound, DetectLanguage, RenderConflictPrompt, ModelNotAvailable, RenderPrompt_NoTemplate, Close

**14. pr_split_bt_test.go — 8 candidates** (11 total, 3 parallel)
- Behavior tree / diff rendering tests
- No os.Chdir → **SAFE**
- Candidates: RenderColorizedDiff_ContentPreserved, RenderColorizedDiff_EmptyInput, GetSplitDiff_InvalidIndex, GetSplitDiff_EmptyFiles, GetSplitDiff_NullPlan, GetSplitDiff_NegativeIndex, BuildReport_WithNullCaches, BuildReport_WithPopulatedCaches

**15. pr_split_10_pipeline_test.go — 7 candidates** (21 total, 14 parallel)
- Pipeline chunk tests
- No os.Chdir → **SAFE**
- Candidates: AUTOMATED_DEFAULTS, ClassificationToGroups_ArrayFormat, ClassificationToGroups_LegacyMapFormat, ClassificationToGroups_EmptyInput, SendToHandle_NullHandle, SendToHandle_MockHandle, SEND_TEXT_NEWLINE_DELAY_MS

**16. pr_split_scope_misc_test.go — 7 candidates** (12 total, 5 parallel)
- Miscellaneous scope tests
- No os.Chdir → **SAFE**
- Candidates: BuildDependencyGraph, RenderAsciiGraph, AnalyzeRetrospective, ConversationHistory, Telemetry, AutoMergeOptions

**17. pr_split_integration_test.go — 7 candidates** (34 real tests, 26 parallel, 1 os.Chdir)
- os.Chdir ONLY in TestIntegration_AutoSplitMockMCP_OutputObservation (line 1875) — excluded
- ⚠️ 4 false-positive `func Test` matches at lines 2384/2458/2500/2525 (inside string literals, not real tests)
- Remaining 7 are Claude integration tests (require Claude availability)
- Candidates: AutoSplitWithClaude_Pipeline, ClaudeMCP_Headless, ClaudeClassificationAccuracy, ClaudeSplitPlanQuality, ClaudeMCP_RoundTrip, ClaudeFallbackToHeuristic, ClaudeConflictResolution

### TIER 3 — Low Impact (≤6 candidates) or Needs Deeper Review

**18. pr_split_05_execution_test.go — 7 candidates** (7 total, 0 parallel)
- Execution chunk tests — No os.Chdir → **SAFE**

**19. pr_split_06_verification_test.go — 7 candidates** (7 total, 0 parallel)
- Verification chunk tests — No os.Chdir → **SAFE**

**20. pr_split_01_analysis_test.go — 6 candidates** (6 total, 0 parallel)
- Analysis chunk tests, creates git repos in t.TempDir() — No os.Chdir → **SAFE**

**21. pr_split_07_prcreation_test.go — 6 candidates** (6 total, 0 parallel)
- PR creation chunk tests — No os.Chdir → **SAFE**

**22. pr_split_claude_config_test.go — 5 candidates** (9 total, 2 parallel, 2 os.Chdir)
- os.Chdir in DependencyStrategy (84) and DependencyStrategyNonGo (138) — excluded
- Candidates: ClaudeConfigOverrides, FlagOverridesConfig, ClaudeConfigJSExposure, ClaudeArgsEmptySplit, ClaudeEnvParsing

**23. pr_split_12_exports_test.go — 4 candidates** (4 total, 0 parallel)
- Export chunk tests — No os.Chdir → **SAFE**

**24. pr_split_heuristic_run_test.go — 4 candidates** (23 total, 4 parallel, 15 os.Chdir)
- Most tests use os.Chdir — only 4 safe
- Candidates: TemplateContent (212), ScriptContent (228), ConfigInjection (254), ConfigOverrides (1135)

**25. pr_split_local_integration_test.go — 4 candidates** (18 total, 1 parallel, 13 os.Chdir)
- Most tests use os.Chdir — only 4 safe
- ⚠️ REVIEW: These are integration tests, check for hidden shared state
- Candidates: FileContentsOnSplitBranches (1302), SingleFileFeature (1400), EmptyFeatureBranch (1641), ExecuteEquivalenceCleanupRoundTrip (1683)

**26. pr_split_tui_hang_test.go — 2 candidates** (2 total, 0 parallel)
- TUI hang regression tests — no os.Chdir
- ⚠️ REVIEW: may use shared TUI resources

**27. pr_split_tui_pty_hang_test.go — 2 candidates** (2 total, 0 parallel)
- PTY hang tests — no os.Chdir
- ⚠️ REVIEW: spawns real PTY processes

**28. pr_split_corruption_test.go — 1 candidate** (2 total, 1 parallel)
- Candidate: TestChunk03_LoadPlan_NoVersionField — No os.Chdir → **SAFE**

**29. pr_split_complex_project_test.go — 1 candidate** (2 real tests, 1 parallel)
- Note: Lines 456+ contain `func Test` inside string literals (false positives)
- Candidate: TestIntegration_AutoSplitComplexGoProject — No os.Chdir
- ⚠️ REVIEW: Creates complex git repo, creates many temp files

**30. pr_split_benchmark_test.go — 1 candidate** (1 total, 0 parallel)
- Candidate: TestBenchmark_AutoSplitLargeRepo — No os.Chdir
- ⚠️ REVIEW: Performance benchmark, may be resource-intensive

---

## CATEGORY C: ⚠️ UNSAFE — Cannot Parallelize (os.Chdir throughout)

| File | Tests | Parallel | os.Chdir Tests | Candidates |
|------|-------|----------|---------------|------------|
| pr_split_wizard_integration_test.go | 15 | 0 | ALL 15 (explicit "NOT parallel" comments) | **0** |
| pr_split_edge_hardening_test.go | 17 | 11 | 6 (all remaining non-parallel) | **0** |
| pr_split_mode_autofix_test.go | 20 | 17 | 3 (all remaining non-parallel) | **0** |
| pr_split_session_cancel_test.go | 8 | 7 | 1 (only remaining non-parallel) | **0** |
| pr_split_prompt_test.go | 16 | 15 | 1 (only remaining non-parallel) | **0** |

---

## CATEGORY D: NOT APPLICABLE

| File | Reason |
|------|--------|
| pr_split_fuzz_test.go | 6 Fuzz functions only — t.Parallel() not applicable |
| pr_split_15_bench_test.go | 6 Benchmark functions only — no Test functions |
| pr_split_pty_unix_test.go | Empty or build-tag excluded (no matches) |
| pr_split_16_helpers_test.go | Helper functions only — no Test functions |

---

## KEY SAFETY PATTERNS

### Why JS Engine Tests Are Safe for t.Parallel()
- `prsplittest.NewTUIEngine(t)`, `NewTUIEngineWithHelpers(t)`, `NewChunkEngine(t, ...)`, `NewFullEngine(t, ...)` all create **isolated per-test JS engines**
- Defined in `internal/command/prsplittest/tui.go` (line 166) and `engine.go` (lines 143, 156)
- Each engine has its own goja.Runtime — no shared state between tests

### Why os.Chdir Tests Cannot Be Parallelized
- `os.Chdir` is **process-global** — changes working directory for ALL goroutines
- Even with `t.Cleanup(func() { _ = os.Chdir(oldDir) })`, parallel tests would race on cwd
- Fix: Refactor to pass directory as parameter instead of os.Chdir (future task, not in scope)

### Safe Patterns That Look Unsafe
- `os.WriteFile` to `t.TempDir()` paths → **SAFE** (unique per test)
- `os.MkdirAll` to `t.TempDir()` paths → **SAFE**
- Git repo creation in `t.TempDir()` → **SAFE**

---

## RECOMMENDED IMPLEMENTATION ORDER

1. **Quick Wins (Tier 1, LOW risk):** 94+36+35+26+14+13+13+13+10 = **254 tests** across 9 files
2. **Chunk Files (numbered 00-12):** Already isolated, just add `t.Parallel()` — **~100 tests** across 11 files  
3. **Medium Risk (Tier 2):** Requires code review for shared state — **~80 tests** across 8 files
4. **Low Impact (Tier 3):** Small files, individual review — **~30 tests** across 8 files

---

## Prior Session Context

### Session 6 (2026-03-19, continued)
Started: 13:44:09 | Target end: 22:44:09 | Elapsed: ~4h 30m

### Commits (Session 6)
- c8132637: T349 finalization (timeout 12m→20m, help overlay test fix)
- 6b83172e: T361 example.config.mk timeout sync + blueprint expansion
- 7055c1d9: T362 — log.debug added to 44 silenced catch blocks across 11 JS files
- 0aca8cf6: T363 — TUI_CONSTANTS object in 15a with 8 named constants, 42 replacements across 10 files
- b64d5ee0: T364 — Replace bare time.Sleep with poll-retry in binary E2E tests

### Task Status
- T300-T364: Done ✅
- T365: **In Progress** — Fuzz tests (6 functions), Rule of Two pending
- T366-T371: Not Started

### T365 Details
Created `pr_split_fuzz_test.go` with 6 fuzz functions:
1. FuzzClassificationParsing — classificationToGroups (89K execs/12s, PASS)
2. FuzzPlanValidation — validatePlan (115K execs/12s, PASS)
3. FuzzValidateClassification — validateClassification (122K execs/12s, PASS)
4. FuzzValidateSplitPlan — validateSplitPlan (140K execs/12s, PASS)
5. FuzzValidateResolution — validateResolution (122K execs/12s, PASS)
6. FuzzIsTransientError — isTransientError (129K execs/12s, PASS)

Found real crash: classificationToGroups passes through non-string description
(e.g. `description:[]`) — corpus entry saved to testdata/fuzz/

### Pre-existing Issues
- 3 JS files over 1000-line limit: 16c (1027), 16e (1073), 16f (1001)
- Shooter/PickAndPlace game tests timing out (pre-existing, not pr-split)
- Full test suite (make make-all-with-log) fails due to game test timeout, NOT pr-split

