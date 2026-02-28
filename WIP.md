# WIP — Session State (Takumi's Desperate Diary)

## Session Start
- **Started:** 2026-02-28 20:36:23
- **Mandate:** 9 hours minimum (ends ~2026-03-01 05:36:23)
- **Phase:** EXECUTING — T1-T88 in sequence

## Last Commit
- **Hash:** 66be949
- **Subject:** Harden pr-split pipeline: single-write sendToHandle, targeted git add
- **Files:** 7 changed, 782 insertions, 201 deletions
- **Rule of Two:** PASS (2 contiguous issue-free reviews + fitness review)

## Current Task
- **Next:** T5 — Audit integration test coverage
- **Status:** Starting

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
1. Pre-T1 bug fixes (gitAddChangedFiles, sendToHandle single-write, commit error checking, test fixes)
2. Rule of Two review gate passed
3. Committed 66be949
