# WIP — Session State (Takumi's Desperate Diary)

## Session Start
- **Started:** 2026-02-28 20:36:23
- **Mandate:** 9 hours minimum (ends ~2026-03-01 05:36:23)
- **Phase:** EXECUTING — T1-T88 in sequence

## Last Commit
- **Hash:** 4b68e2b
- **Subject:** Enhance mock ClaudeCodeExecutor with prompt capture and assertions
- **Files:** 2 changed, pr_split_test.go (mock enhancement), blueprint.json (replan T6-T9)

## Current Task
- **Next:** T11 — Design configurable stdout control for osm script
- **Status:** T10 complete (investigation)

## T5 Integration Test Coverage Gap Analysis

**12 test files** in internal/command/ covering pr-split, ~150+ test functions total.

### Pipeline Stage Coverage Matrix

| Stage | Pipeline Step | Coverage | Key Tests |
|-------|-------------|----------|-----------|
| 1 | Analyze Diff | PARTIAL | TestAnalyzeDiff_EdgeCases, TestIntegration_HeuristicSplitEndToEnd |
| 2 | Spawn Claude | ✅ COVERED | 9 tests: SpawnArgs, SpawnHealthCheck, IsAliveGuard, ClaudeCodeExecutor_Resolve |
| 3 | Send Classification | ✅ COVERED | 10 tests: SendToHandle (5), SendWithCancel (4), Pipeline (1) |
| 4 | Receive Classification | ✅ COVERED | TestPollForFile (5 subtests), TestIntegration_AutoSplitWithClaude_Pipeline |
| 5 | Validate Classification | PARTIAL | Only implicit validation within receive step; no standalone test |
| 6 | Generate Split Plan | ✅ COVERED | TestValidatePlan (14 subtests), TestIntegration_HeuristicSplitEndToEnd |
| 7 | Execute Split | ✅ COVERED | TestExecuteSplit (8), TypeChange, RenamedFile, CopiedFile, ValidationErrors |
| 8 | Verify Splits | ✅ COVERED | TestVerifySplits_MockExec, PerBranchTimeout, SkipsDependencyFailures, FailedBranch |
| 9 | Resolve Conflicts | PARTIAL | TestResolveConflicts (8 subtests), TestIntegration_AutoSplitCancel (cancellation only) |
| 10 | Verify Equivalence | ✅ COVERED | 17 tests across 3 files |

### Critical Gaps
1. **Stage 5**: No standalone classification validation tests (uncategorized grouping, file assignment)
2. **Stage 9**: Re-split triggering and multi-retry cycles not fully exercised
3. **End-to-end**: TestIntegration_AutoSplitMockMCP (pr_split_test.go:9178) is the only mock E2E; run via `make integration-test-prsplit-mcp`
4. **Stage 1**: Diff analysis well-covered for edge cases but classification prompt rendering tested in pr_split_prompt_test.go (11 tests)

## T4 Root Cause: resolveConflictsWithClaude Prompt Sending Failure

**Root Cause (CONFIRMED):** Missing null checks on `claudeExecutor.handle` at 3 of 4 `sendToHandle` call sites. The handle can become null/stale via two pathways:

**Pathway 1 (Resume Path):** When resuming from cached plan (line ~2930), if `claudeExecutor.spawn()` fails (line ~2947), `sessionId`, `resultDir`, and `aliveCheckFn` remain `null` — but the pipeline continues into verify and resolve steps, calling `sendToHandle(claudeExecutor.handle, ...)` on a null handle.

**Pathway 2 (Process Death):** Claude process can die mid-pipeline. `aliveCheckFn` only runs every 10 poll iterations (~5s). Between heartbeats, `sendToHandle()` sends to a dead process.

**All 4 sendToHandle call sites:**
| Line | Context | Null Guard |
|------|---------|------------|
| 1750 | `claudeExecutor.fix()` — conflict strategy | ✅ YES (line 1732) |
| 2781 | `automatedSplit()` Step 3 — classification | ❌ NO |
| 3016 | `resolveConflictsWithClaude()` — re-classify | ❌ NO |
| 3220 | `resolveConflictsWithClaude()` — conflict prompt | ❌ NO |

**Handle Lifecycle:**
- Created: `claudeExecutor.spawn()` at line 2139 sets `this.handle = registry.spawn(...)`
- Nullified: `close()` at line 2214 sets `this.handle = null`
- Nullified: Post-spawn health check at line 2179 sets `this.handle = null` on immediate death
- Cleanup: `cleanupExecutor()` at line 3312 calls `claudeExecutor.close()` → nullifies handle

**Fix:** Add pre-send validation `if (!claudeExecutor || !claudeExecutor.handle) return { error: '...' }` at lines 2781, 3016, 3220 (matching the pattern at line 1732). Guard the resolve step entrance with `if (!sessionId || !claudeExecutor || !claudeExecutor.handle)` to skip conflict resolution entirely when Claude is unavailable.

## T3 Root Cause: Verification "Skip" Bug

**Root Cause (CONFIRMED):** The `step('Verify splits', fn)` wrapper at pr_split_script.js:2920 ALWAYS returns `{ error: null, failures: realFailures, allPassed: verifyObj.allPassed }`. The `step()` function at line 2600 checks only `result.error` to determine TUI status. Since `error` is always `null`, the TUI shows ✓ (green) for "Verify splits" even when:
- Multiple branches fail verification
- All branches are skipped due to dependency failures
- No actual verification ran

**Hypothesis Results:**
- H1 (git checkout fails silently): DISPROVED — gitExec result IS checked at line 1210
- H2 (verify runs on wrong branch): DISPROVED — checkout happens before verify, failures propagate correctly
- H3 (TUI suppresses sub-100ms results): DISPROVED — issue is in step() wrapper, not TUI rendering

**Fix Target:** T48 — either propagate `allPassed: false` into `result.error`, or modify `step()` to check additional fields.

**Test:** `TestVerifySplits_FailedBranch_AllPassedFalse` in pr_split_verification_test.go demonstrates verifySplits correctly returns allPassed=false (function is correct, bug is in the step wrapper).

## T1 Diagnosis: Windows Build Failures

### Category A: Missing Windows Skip Guards (TEST)
| File | Lines | Issue |
|------|-------|-------|
| `internal/builtin/claudemux/coverage_gaps_test.go` | 137, 176, 194, 216 | 4 tests use `net.Listen("unix",...)` / `net.Dial("unix",...)` WITHOUT `runtime.GOOS == "windows"` skip guard. Other tests in `control_test.go` properly skip. |

### Category B: Unguarded UDS in Production Code (RUNTIME)
| File | Line | Issue |
|------|------|-------|
| `internal/builtin/claudemux/control.go` | 103 | `net.Listen("unix", s.sockPath)` has no `runtime.GOOS` guard or build tag. Will fail on Windows if UDS not supported. Note: Windows 10 1803+ supports AF_UNIX, so may work on CI (windows-latest = Server 2022). |

### Category C: `sh -c` Shell Execution (RUNTIME)
| File | Lines (approx) | Sites |
|------|----------------|-------|
| `internal/command/pr_split_script.js` | 1219, 1539, 1565, 1596, 1625, 1648, 1653, 1659, 1663, 1665, 1779, 1891, 1938 | 13 sites calling `exec.execv(['sh', '-c', ...])`. Also uses `timeout` utility at line 1216. NOTE: GitHub Actions windows-latest has Git Bash in PATH, so `sh` may be available. Tests skip via pr_split_test.go guards. |

### Category D: `which` Command Usage (RUNTIME)
| File | Lines (approx) | Sites |
|------|----------------|-------|
| `internal/command/pr_split_script.js` | 1594, 2006, 2015, 2031 | 4 sites using `exec.execv(['which', ...])`. Windows uses `where.exe` instead. |

### Category E: Unix Utilities in Shell Commands (RUNTIME)
| File | Lines (approx) | Issue |
|------|----------------|-------|
| `internal/command/pr_split_script.js` | 1596 | `find . -name "*.go" -exec goimports -w {} +` — Unix-only |
| `internal/command/pr_split_script.js` | 1653 | `grep -rl ... \| head -1` — Unix-only |

### Already Properly Handled
- `internal/termmux/` — proper `//go:build` tags (platform_windows.go, resize_windows.go)
- `internal/storage/` — proper platform files (filelock_windows.go, atomic_write_windows.go)
- `internal/session/` — proper platform files (session_windows.go)
- `internal/builtin/pty/` — proper build tags (pty_windows.go returns ErrNotSupported)
- `internal/builtin/claudemux/control_test.go` — 5 tests properly skip on Windows
- `internal/builtin/claudemux/provider_test.go` — 3 tests properly skip (PTY)
- `internal/builtin/claudemux/pr_split_test.go` — skips "PR split uses sh -c"

## Completed This Session

## T10 Stdout Pollution Investigation — COMPLETE

### Initialization Call Sequence
```
ScriptingCommand.Execute(ctx, stdout, stderr)
  → PrepareEngine(ctx, stdout, stderr)
    → NewEngineDetailed(ctx, stdout, stderr, ...)
      → NewTerminalIOStdio()            // creates TUIWriter wrapping os.Stdout
      → NewTUILogger(stdout, ...)        // ring buffer + optional file, NOT stdout
      → NewTUIManagerWithConfig(ctx, ... terminalIO.TUIWriter, sessionID, store)
        → initializeStateManager(sessionID, store)
```

### Key Architectural Finding: stdout Parameter Divorce
The original `stdout` parameter is **NOT** passed to TUI manager. Instead:
- `NewTerminalIOStdio()` creates a fresh `TUIWriter` → `prompt.NewStdoutWriter()` → **os.Stdout directly**
- Engine's `TUILogger` receives original `stdout` param for log routing, but logs go to ring buffer during init
- Result: TUI output always goes to real os.Stdout, bypassing any test buffer or redirect

### Stdout Writes During Initialization

| # | File | Line | Content | Stream | MCP Risk | Classification |
|---|------|------|---------|--------|----------|----------------|
| 1 | `tui_manager.go` | 114 | `"Warning: Failed to initialize state persistence (session %q): %v\n"` | **stdout** (via TUIWriter) | **HIGH** | Informational — can suppress |
| 2 | `state_manager.go` | 82 | `"WARNING: Session schema version mismatch..."` | stderr (Go log.Printf) | Low | Informational — stderr OK |
| 3 | `engine_core.go` | 482 | Panic stack trace | stderr | Low | Necessary — stderr OK |
| 4 | Startup logger | validateModulePaths | Invalid path warnings | stderr | Low | Informational — stderr OK |

### Critical Finding
**Only ONE stdout write during init: `tui_manager.go:114`** — writes to TUIWriter (wraps os.Stdout) when primary state persistence backend fails. This is conditional (requires storage failure) but when it fires, it would corrupt MCP stdio JSON-RPC protocol.

### TUILogger is Safe During Init
- Stores logs in memory ring buffer via slog handler
- `PrintToTUI()` writes to `l.tuiWriter` (original stdout) but is only called from user script code, not init
- Default slog handler does NOT write to stdout — writes to file handler if configured, otherwise buffer only

### Fix Strategy (for T11-T12)
- Route tui_manager.go:114 warning through logger or stderr instead of TUIWriter
- Consider: `--quiet-init` flag or `output.setSuppressInit(true)` JS API
- Simplest fix: Change `fmt.Fprintf(output, ...)` to `fmt.Fprintf(os.Stderr, ...)` at line 114

## T11 Design: Configurable Stdout Control for osm script

### Analysis
T10 found exactly **1 stdout write** during initialization: `tui_manager.go:114`. All other init writes already go to stderr or the log ring buffer. This substantially reduces the design scope.

### Options Considered

| Option | Complexity | Backward Compat | Correct? |
|--------|-----------|-----------------|----------|
| A: `--quiet-init` CLI flag | Medium | Yes (opt-in) | Overkill for 1 write |
| B: `output.setSuppressInit(true)` JS API | High | Yes (opt-in) | Overkill, fires after init |
| C: Route warning to stderr | **Minimal** | **Yes** — warnings on stderr are Unix convention | **Yes** |
| D: Route warning through slog | Medium | Yes | Over-engineered |

### Chosen: Option C — Route warning to stderr

**Rationale:**
1. There is exactly one stdout write during init. A whole suppression framework is unwarranted.
2. The write is a WARNING about storage failure. Unix convention: warnings and errors go to stderr.
3. No API surface changes. No config flags. No backward compatibility concerns.
4. Existing scripts see no behavior change (they weren't expecting this warning on stdout anyway — it only fires on storage failure).
5. MCP stdio is immediately clean — zero stdout bytes before user script.

### Changes Required

| File | Change |
|------|--------|
| `internal/scripting/tui_manager.go:114` | `fmt.Fprintf(output, "Warning: ...")` → `fmt.Fprintf(os.Stderr, "Warning: ...")` |

### Initialization Sequence (After Fix)
```
ScriptingCommand.Execute()
  → PrepareEngine()
    → NewEngineDetailed()
      → NewTerminalIOStdio()          // TUIWriter → os.Stdout
      → NewTUILogger(stdout, ...)      // ring buffer, no stdout writes
      → NewTUIManagerWithConfig()
        → initializeStateManager()
          → storage failure?
            → YES: fmt.Fprintf(os.Stderr, "Warning:...") [STDERR, not stdout]
            → NO: proceed silently
      → setupGlobals()
  → engine.ExecuteScript(script)       // FIRST stdout write is user-controlled
```

### Backward Compatibility Guarantee
- **Before fix:** Warning goes to stdout (via TUIWriter). Only fires on storage failure (rare).
- **After fix:** Warning goes to stderr. Any tool capturing stdout sees no behavior change for normal operation. The warning was never part of the "API surface" — it's an error condition.
- **Test impact:** No tests rely on this warning appearing on stdout. Tests that mock storage failures would need to capture stderr to see it.

### Future-Proofing
If new stdout writes are introduced during init in the future, this design decision establishes the precedent: **all diagnostic/warning/error output during initialization goes to stderr**. The engine's stdout is reserved for user-controlled output only.
1. Pre-T1 bug fixes (gitAddChangedFiles, sendToHandle single-write, commit error checking, test fixes)
2. Rule of Two review gate passed
3. Committed 66be949
