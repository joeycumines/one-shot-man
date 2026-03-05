# W01: PR-Split Wizard State Machine Design

## Overview

The wizard state machine replaces Go-side `prsplit_autosplit_model.go` with a
JS-driven controller that owns the pipeline lifecycle. The state machine lives
in `pr_split_13_tui.js` and interacts with the BubbleTea model (Go-side) via
the existing `autoSplitTUI` bridge for rendering only.

**Design Principle:** The JS state machine is the *orchestrator*. The Go-side
BubbleTea model is the *renderer*. JS tells Go "show step X as running." Go
never drives transitions.

## States

```
┌─────────┐
│  IDLE   │
└────┬────┘
     │ user triggers auto-split
     ▼
┌──────────┐     baseline failure     ┌──────────────┐
│  CONFIG  │ ──────────────────────→ │ BASELINE_FAIL │
└────┬─────┘                          └──────┬────────┘
     │ baseline OK                      │ override / abort
     ▼                                  ▼
┌──────────────────┐
│ PLAN_GENERATION  │  (analyze → classify → plan)
└────┬─────────────┘
     │ plan generated
     ▼
┌──────────────┐    user edits plan     ┌─────────────┐
│ PLAN_REVIEW  │ ◄─────────────────── │ PLAN_EDITOR │
│              │ ──────────────────→  │             │
└────┬─────────┘                       └─────────────┘
     │ user approves plan
     ▼
┌──────────────────┐
│ BRANCH_BUILDING  │  (execute → verify → resolve)
└────┬─────────────┘
     │ all branches done
     ▼
┌──────────────┐    conflicts found   ┌──────────────────┐
│  EQUIV_CHECK │ ◄───────────────── │ ERROR_RESOLUTION │
└────┬─────────┘                     └──────────────────┘
     │ tree-hash matches
     ▼
┌──────────────┐
│ FINALIZATION │  (report, cleanup, PR creation menu)
└────┬─────────┘
     │
     ▼
┌──────┐
│ DONE │
└──────┘

Cross-cutting states (reachable from any active state):
  CANCELLED    — user pressed q (graceful)
  FORCE_CANCEL — user pressed q twice (SIGKILL)
  PAUSED       — user pressed Ctrl+P (checkpoint + exit)
  ERROR        — unrecoverable error
```

## State Definitions

### IDLE
- **Entry:** Session start or previous run completed.
- **Actions:** None. Waiting for user command.
- **Transitions:** `auto-split` command → CONFIG.

### CONFIG
- **Entry:** User triggers auto-split.
- **Actions:**
  1. Validate configuration (required flags, paths).
  2. Check for `--resume` flag → skip to BRANCH_BUILDING with cached plan.
  3. Run baseline verification (`verifyCommand` on base branch).
- **Data:** `{ config, resumeData? }`
- **Transitions:**
  - Baseline OK → PLAN_GENERATION
  - Baseline failure → BASELINE_FAIL
  - `--resume` with valid checkpoint → BRANCH_BUILDING
  - Invalid config → ERROR

### BASELINE_FAIL
- **Entry:** Baseline verification failed.
- **Actions:** Display failure details and menu:
  - **Override** — proceed anyway (user takes responsibility)
  - **Abort** — cancel
- **Transitions:**
  - Override → PLAN_GENERATION
  - Abort → CANCELLED

### PLAN_GENERATION
- **Entry:** Baseline passed (or overridden).
- **Actions:**
  1. Analyze diff (chunk 01)
  2. Spawn Claude (chunk 09) — or fallback to heuristic
  3. Send classification request
  4. Receive classification (MCP callback)
  5. Generate split plan
- **Sub-steps tracked:** Each step has status (Pending/Running/Done/Failed).
- **Data:** `{ analysis, groups, plan }`
- **Transitions:**
  - Plan generated → PLAN_REVIEW
  - Claude unavailable → heuristic fallback → PLAN_REVIEW
  - All retries exhausted → ERROR
  - User cancel → CANCELLED

### PLAN_REVIEW
- **Entry:** Plan generated.
- **Actions:** Display plan summary. Menu:
  - **Approve** — proceed to execution
  - **Edit** — open plan editor
  - **Regenerate** — re-classify with feedback
  - **Cancel** — abort
- **Data:** `{ plan, editHistory }`
- **Transitions:**
  - Approve → BRANCH_BUILDING
  - Edit → PLAN_EDITOR (sub-state, returns to PLAN_REVIEW)
  - Regenerate → PLAN_GENERATION (with feedback context)
  - Cancel → CANCELLED

### PLAN_EDITOR
- **Entry:** User chose "Edit" from PLAN_REVIEW.
- **Actions:** Interactive plan editing (rename, move files, merge, delete splits).
- **Data:** `{ items, cursor, expanded, moveMode }`
- **Transitions:**
  - Exit → PLAN_REVIEW (with modified plan)
  - Cancel → PLAN_REVIEW (with original plan)

### BRANCH_BUILDING
- **Entry:** Plan approved (or resumed from checkpoint).
- **Actions:**
  1. Execute splits (chunk 05) — create branches per plan
  2. Verify splits (chunk 06) — test each branch
  3. Track per-branch status (Pending/Running/Passed/Failed/Skipped/PreExisting)
- **Sub-state: Branch Verification Pipeline**
  - Each branch independently: BranchPending → BranchRunning → BranchDone
  - Failed branches trigger resolution prompt
- **Data:** `{ splits, branchStates, failedBranches }`
- **Transitions:**
  - All pass → EQUIV_CHECK
  - Some fail → ERROR_RESOLUTION
  - User cancel → CANCELLED
  - User pause → PAUSED (saves checkpoint)

### ERROR_RESOLUTION
- **Entry:** One or more branches failed verification.
- **Actions:** Per-branch resolution menu:
  - **Auto-Resolve** — send to Claude for patching (chunk 08)
  - **Manual Fix** — drop to shell, user fixes, re-verify
  - **Skip** — mark branch as skipped
  - **Abort** — cancel entire pipeline
- **Data:** `{ failedBranches, resolutions, reSplitSuggested }`
- **Transitions:**
  - All resolved → EQUIV_CHECK (or re-verify → BRANCH_BUILDING)
  - Re-split suggested → PLAN_GENERATION (with conflict context)
  - Skip all → EQUIV_CHECK
  - Abort → CANCELLED

### EQUIV_CHECK
- **Entry:** All branches passed or skipped.
- **Actions:** Run equivalence check (tree hash comparison).
- **Data:** `{ equivalenceResult }`
- **Transitions:**
  - Match → FINALIZATION
  - Mismatch → FINALIZATION (with warning)

### FINALIZATION
- **Entry:** Pipeline complete.
- **Actions:**
  1. Generate report (branches, timings, Claude interactions).
  2. Show summary. Menu:
     - **Create PRs** — run `gh pr create` for each branch
     - **Copy Report** — copy to clipboard
     - **View Details** — expand per-branch stats
     - **Done** — return to IDLE
- **Data:** `{ report, prUrls }`
- **Transitions:**
  - Any menu option → (perform action, stay in FINALIZATION)
  - Done → DONE

### DONE
- **Entry:** User dismissed results.
- **Actions:** Cleanup resources (Claude process, MCP callbacks).
- **Transitions:** → IDLE

### Cross-Cutting States

#### CANCELLED
- **Entry:** User pressed `q` from any active state.
- **Actions:**
  1. Set cancellation flag (checked by pipeline at next checkpoint).
  2. Wait for in-flight operations to complete gracefully.
  3. Cleanup resources.
- **Transitions:** → DONE

#### FORCE_CANCEL
- **Entry:** User pressed `q` twice (or Ctrl+C).
- **Actions:**
  1. Send SIGKILL to Claude (if running).
  2. Immediate cleanup.
- **Transitions:** → DONE

#### PAUSED
- **Entry:** User pressed `Ctrl+P` from BRANCH_BUILDING or PLAN_GENERATION.
- **Actions:**
  1. Save checkpoint (plan, execution state, branch states).
  2. Print resume instructions.
- **Transitions:** → DONE (can be resumed with `--resume`)

#### ERROR
- **Entry:** Unrecoverable error from any state.
- **Actions:**
  1. Display error with context.
  2. Save partial state for debugging.
  3. Cleanup resources.
- **Transitions:** → DONE

## Transition Matrix

| From \ To | IDLE | CONFIG | BASELINE_FAIL | PLAN_GEN | PLAN_REVIEW | PLAN_EDITOR | BRANCH_BLDG | ERROR_RES | EQUIV_CHK | FINAL | DONE | CANCEL | FORCE | PAUSED | ERROR |
|-----------|------|--------|---------------|----------|-------------|-------------|-------------|-----------|-----------|-------|------|--------|-------|--------|-------|
| IDLE      |      | ✓      |               |          |             |             |             |           |           |       |      |        |       |        |       |
| CONFIG    |      |        | ✓             | ✓        |             |             | ✓ (resume)  |           |           |       |      | ✓      |       |        | ✓     |
| BASELINE_FAIL |  |        |               | ✓        |             |             |             |           |           |       |      | ✓      |       |        |       |
| PLAN_GEN  |      |        |               |          | ✓           |             |             |           |           |       |      | ✓      | ✓     | ✓      | ✓     |
| PLAN_REVIEW |    |        |               | ✓        |             | ✓           | ✓           |           |           |       |      | ✓      |       |        |       |
| PLAN_EDITOR |    |        |               |          | ✓           |             |             |           |           |       |      |        |       |        |       |
| BRANCH_BLDG |    |        |               |          |             |             |             | ✓         | ✓         |       |      | ✓      | ✓     | ✓      | ✓     |
| ERROR_RES |      |        |               | ✓        |             |             | ✓           |           | ✓         |       |      | ✓      | ✓     |        |       |
| EQUIV_CHK |      |        |               |          |             |             |             |           |           | ✓     |      | ✓      |       |        | ✓     |
| FINAL     |      |        |               |          |             |             |             |           |           | ✓     | ✓    |        |       |        |       |
| DONE      | ✓    |        |               |          |             |             |             |           |           |       |      |        |       |        |       |
| CANCEL    |      |        |               |          |             |             |             |           |           |       | ✓    |        |       |        |       |
| FORCE     |      |        |               |          |             |             |             |           |           |       | ✓    |        |       |        |       |
| PAUSED    |      |        |               |          |             |             |             |           |           |       | ✓    |        |       |        |       |
| ERROR     |      |        |               |          |             |             |             |           |           |       | ✓    |        |       |        |       |

## Implementation Plan

### JS API

```javascript
// WizardState constructor
function WizardState() {
    this.current = 'IDLE';
    this.data = {};
    this.history = [];      // state transition log
    this.checkpoint = null; // for pause/resume
}

// Guarded transition
WizardState.prototype.transition = function(to, data) {
    if (!VALID_TRANSITIONS[this.current] || !VALID_TRANSITIONS[this.current][to]) {
        throw new Error('Invalid transition: ' + this.current + ' → ' + to);
    }
    this.history.push({ from: this.current, to: to, at: Date.now() });
    this.current = to;
    if (data) Object.assign(this.data, data);
};

// Cross-cutting state checks
WizardState.prototype.cancel = function() {
    if (['DONE', 'CANCELLED', 'FORCE_CANCEL', 'PAUSED', 'ERROR'].indexOf(this.current) >= 0) return;
    this.transition('CANCELLED');
};

WizardState.prototype.forceCancel = function() {
    if (['DONE', 'FORCE_CANCEL'].indexOf(this.current) >= 0) return;
    this.transition('FORCE_CANCEL');
};

WizardState.prototype.pause = function() {
    if (['BRANCH_BUILDING', 'PLAN_GENERATION'].indexOf(this.current) < 0) return;
    this.transition('PAUSED');
};

// Convenience
WizardState.prototype.isTerminal = function() {
    return ['DONE', 'CANCELLED', 'FORCE_CANCEL', 'PAUSED', 'ERROR'].indexOf(this.current) >= 0;
};
```

### Integration Points

1. **autoSplitTUI bridge** — JS calls `autoSplitTUI.sendStepStart()` etc. for rendering.
   The wizard state machine replaces the implicit pipeline→TUI state flow.

2. **tuiMux** — JS toggles Claude pane via `tuiMux.switchTo('claude')`. State machine
   tracks which pane is active.

3. **prSplitConfig** — Config values flow into CONFIG state on entry.

4. **prSplit._state** — Cache objects (analysisCache etc.) remain. State machine adds
   `.wizard` field for wizard-specific state.

### Backward Compatibility

During transition:
- The REPL commands in chunk 13 continue to work alongside the wizard.
- Running `auto-split` in REPL enters CONFIG state.
- Manual commands (analyze, group, plan, execute) update caches but don't use state machine.
- The wizard is opt-in: `--wizard` flag or TUI mode auto-detection.

### Error Recovery Architecture

Each active state has an error handler:
1. Catch error.
2. Log to telemetry.
3. If retryable: retry N times with backoff.
4. If not retryable: show error menu (Retry / Skip / Abort).
5. Menu choice drives transition.

The state machine guarantees no state is "stuck" — every state has at least one
outward transition path. Terminal states always reach DONE → IDLE.
