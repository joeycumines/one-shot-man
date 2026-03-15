# T001 Audit: Analysis Pipeline Hang Scenarios

**Date:** 2026-03-15  
**Scope:** `startAnalysis` → `runAnalysisAsync` → `handleAnalysisPoll` in `pr_split_16_tui_core.js`  
**Related:** `analyzeDiffAsync` in `pr_split_01_analysis.js`

## Architecture

```
User presses Enter on CONFIG (mode=heuristic)
  → startAnalysis(s)
    → isProcessing=true, analysisRunning=true
    → runAnalysisAsync(s).then(resolve, reject)
    → returns [s, tea.tick(100, 'analysis-poll')]

Every 100ms:
  → handleAnalysisPoll(s)
    if (!isProcessing && !analysisRunning) → stop
    if (analysisRunning) → re-tick
    if (analysisError) → ERROR state, stop
    else → stop (async fn handled state inline)
```

## Exit Paths in `runAnalysisAsync`

| EP | Trigger | `isProcessing=false` | `analysisRunning=false` | Wizard State | Poll Stops | Status |
|----|---------|---------------------|------------------------|--------------|------------|--------|
| EP-01 | Baseline verify fails | ✅ Direct | ✅ .then() | → CONFIG | ✅ | ✅ |
| EP-02 | Baseline throws | ✅ Direct | ✅ .then() | → CONFIG | ✅ | ✅ |
| EP-03 | Baseline throws + CANCELLED | ✅ confirmCancel | ✅ confirmCancel (T001) | CANCELLED | ✅ | ✅ |
| EP-04 | Cancel guard (after Step 0) | ✅ confirmCancel | ✅ confirmCancel (T001) | CANCELLED | ✅ | ✅ |
| EP-05 | analyzeDiffAsync throws | ✅ Direct | ✅ .then() | → ERROR | ✅ | ✅ |
| EP-06 | analyzeDiffAsync throws + CANCELLED | ✅ confirmCancel | ✅ confirmCancel (T001) | CANCELLED | ✅ | ✅ |
| EP-07 | analysisCache.error | ✅ Direct | ✅ .then() | → ERROR | ✅ | ✅ |
| EP-08 | No files found | ✅ Direct | ✅ .then() | → CONFIG | ✅ | ✅ |
| EP-09 | Cancel guard (before Step 2) | ✅ confirmCancel | ✅ confirmCancel (T001) | CANCELLED | ✅ | ✅ |
| EP-10 | applyStrategyAsync throws (T001) | ✅ Direct | ✅ .then() | → ERROR | ✅ | ✅ Fixed |
| EP-11 | applyStrategyAsync throws + CANCELLED (T001) | ✅ confirmCancel | ✅ confirmCancel (T001) | CANCELLED | ✅ | ✅ Fixed |
| EP-12 | Cancel guard (before Step 3) | ✅ confirmCancel | ✅ confirmCancel (T001) | CANCELLED | ✅ | ✅ |
| EP-13 | createSplitPlanAsync throws (T001) | ✅ Direct | ✅ .then() | → ERROR | ✅ | ✅ Fixed |
| EP-14 | createSplitPlanAsync throws + CANCELLED (T001) | ✅ confirmCancel | ✅ confirmCancel (T001) | CANCELLED | ✅ | ✅ Fixed |
| EP-15 | Validation fails | ✅ Direct | ✅ .then() | → ERROR | ✅ | ✅ |
| EP-16 | Success | ✅ Direct | ✅ .then() | → PLAN_REVIEW | ✅ | ✅ |

## Fixes Applied (T001)

### 1. try-catch around Steps 2 & 3
**Before:** `applyStrategyAsync` and `createSplitPlanAsync` had no try-catch. Exceptions propagated to the outer `.then(_, reject)` handler, then handled by `handleAnalysisPoll` — functional but adds 100ms latency and inconsistent error messaging.

**After:** Both wrapped in try-catch with CANCELLED guard, explicit `isProcessing=false`, wizard transition to ERROR, and descriptive error messages.

### 2. `analysisRunning` cleared in `confirmCancel()`
**Before:** Only `isProcessing` was cleared. `analysisRunning` stayed true until the Promise resolved, causing orphaned poll ticks.

**After:** Both `analysisRunning` and `autoSplitRunning` cleared in `confirmCancel()` for immediate poll termination.

## Remaining Risks (Deferred to T002)

### Hang Scenarios

| ID | Scenario | Component | Symptom | Mitigation |
|----|----------|-----------|---------|------------|
| HANG-A | `gitExecAsync` blocks on credential prompt | `analyzeDiffAsync` (Step 1), `applyStrategyAsync` (Step 2 for `dependency` strategy) | Spinner runs forever; `await` never resolves | Ctrl+C still works (BubbleTea can process messages while JS yields at `await`). After T001 `confirmCancel` fix, cancel terminates the poll immediately. Hung child process is NOT killed. |
| HANG-B | `verifyBaselineAsync` with timeout=0 | Step 0 (baseline verify) | Runaway verify command never completes | User's `verifyTimeoutMs` config defaults to 0 (no timeout). T002 will add a mandatory minimum default. |
| HANG-C | NFS/network filesystem operation blocks indefinitely | `createSplitPlanAsync` (Step 3) calls `git rev-parse` | Promise never resolves; same UX as HANG-A | Same as HANG-A: cancel works but child process orphaned. |

### Missing Safeguards

| ID | Safeguard | Current State | Required By |
|----|-----------|---------------|-------------|
| MS-1 | Global per-step timeout for `runAnalysisAsync` | No per-step timeouts; relies on child processes terminating | T002 |
| MS-2 | Default minimum `verifyTimeoutMs` (e.g. 60s) | Defaults to 0 (no timeout) | T002 |
| MS-3 | Child process tracking + SIGTERM on cancel | `confirmCancel` doesn't kill spawned git processes | T002 |
| MS-4 | Stale spinner detection (watchdog tick) | No watchdog; spinner animates but pipeline may be hung | T002 |
| MS-5 | `AbortController` integration for `gitExecAsync` | No cancellation token passed to subprocess calls | T002 |
| MS-6 | Progress heartbeat from async steps | Steps don't report heartbeats; only status bar updates at step boundaries | T002 |

### `verifySplitAsync` default timeout is 0
Step 0 (baseline verify) uses the user's `verifyTimeoutMs` config. Default is 0 (no timeout). A runaway verify command hangs the pipeline (see HANG-B). T002 will add a default minimum (see MS-2).

## Conclusion

All 16 explicit exit paths (EP-01 through EP-16) in `runAnalysisAsync` now correctly clear `isProcessing` and terminate the poll. Three hang scenarios (HANG-A/B/C) remain where child processes can block indefinitely — these require T002's timeout machinery and child process tracking (MS-1 through MS-6) to fully resolve.
