# T419: Falsy-Value `||` Anti-Pattern Audit

**Date:** 2026-03-23  
**Scope:** JS chunks `pr_split_09_*.js` through `pr_split_16f_*.js`  
**Auditor:** Takumi (automated scan + manual classification)

## Background

T399 identified and fixed `||` patterns that lose intentionally-falsy values
(`""`, `0`, `false`) in `pr_split_10d_pipeline_orchestrator.js` (lines 97–98).
This audit extends the scan to ALL JS chunks for the same anti-pattern.

## Methodology

1. Scanned ~20 files containing ~600 `||` usages
2. Classified each as **BUG** (falsy value lost) or **SAFE**
3. Found **22 BUG instances** across 9 files

## Findings Summary

| Severity | Count | Category |
|----------|-------|----------|
| HIGH     | 2     | `exitCode \|\| 1` — turns success into failure |
| MEDIUM   | 11    | Timeout `\|\| default` — zero timeout silently ignored |
| MEDIUM   | 5     | `maxFiles \|\| 10` — "unlimited" intent lost |
| LOW      | 4     | Health poll, wall-clock multiplier, conversation history |

## All Bugs Fixed

### BUG-1: `pr_split_09_claude.js` — `exitCode || 1`
Exit code 0 (success) replaced with 1 (error) in conflict prompt.
**Fix:** `typeof conflict.exitCode === 'number' ? conflict.exitCode : 1`

### BUG-2: `pr_split_10c_pipeline_resolve.js` — `exitCode || 1`
Same pattern in resolution prompt.
**Fix:** `typeof fail.exitCode === 'number' ? fail.exitCode : 1`

### BUG-3 to BUG-8: `pr_split_10d_pipeline_orchestrator.js` — timeout || default
Eight timeout fields (classify, plan, resolve, commandMs, pollInterval,
pipelineTimeout, stepTimeout, watchdogIdle) used `||` instead of `typeof`.
T399 already fixed `maxResolveRetries` and `maxReSplits` on adjacent lines
with `typeof` checks, but left these unfixed.
**Fix:** `typeof config.xxxMs === 'number' ? config.xxxMs : AUTOMATED_DEFAULTS.xxxMs`

### BUG-9: `pr_split_10d_pipeline_orchestrator.js:355` — heartbeatTimeoutMs
**Fix:** `typeof config.heartbeatTimeoutMs === 'number' ? ... : ...`

### BUG-10–11: `pr_split_10d_pipeline_orchestrator.js:816,935` — verifyTimeoutMs
Two call sites for `verifySplits` used `config.verifyTimeoutMs || ...`.
**Fix:** Same typeof pattern.

### BUG-12: `pr_split_10c_pipeline_resolve.js:187` — wall-clock multiplier
Combined: `(timeouts.resolve || default) * (maxAttemptsPerBranch || 3)`.
**Fix:** typeof checks for both values.

### BUG-13: `pr_split_09_claude.js:260` — spawnHealthCheckDelayMs
**Fix:** typeof check with ternary.

### BUG-14: `pr_split_09_claude.js:461` — maxFilesPerSplit
`config.maxFilesPerSplit || runtime.maxFiles || 0` — if config is 0, falls through.
**Fix:** Nested typeof ternaries.

### BUG-15: `pr_split_14a_tui_commands_core.js:487` — parseInt || 10
`set max 0` gives 10 instead of 0.
**Fix:** `var n = parseInt(value, 10); runtime.maxFiles = isNaN(n) ? 10 : n;`

### BUG-16: `pr_split_15c_tui_screens.js:126` — display maxFiles || 10
**Fix:** `String(typeof runtime.maxFiles === 'number' ? runtime.maxFiles : 10)`

### BUG-17: `pr_split_16a_tui_focus.js:920` — edit field maxFiles || 10
**Fix:** Same typeof pattern.

### BUG-18: `pr_split_16f_tui_model.js:213` — mouse-click edit path
**Fix:** Same typeof pattern.

### BUG-19: `pr_split_16d_tui_handlers_claude.js:317` — claudeHealthPollMs
**Fix:** typeof check.

### BUG-20: `pr_split_11_utilities.js:166` — maxConversationHistory
Setting to 0 (disable history) gives 100 instead.
**Fix:** typeof check with ternary.

## Safe Patterns (~580 instances)

| Pattern | Count | Why Safe |
|---------|-------|----------|
| `e.message \|\| String(e)` | ~60 | Empty message = no info, fallback correct |
| `x.name \|\| 'unnamed'` | ~30 | Empty name needs placeholder |
| `x \|\| []` / `x \|\| {}` | ~40 | Arrays/objects are truthy |
| `s.width \|\| 80` | ~30 | 0 width is not valid |
| `s.focusIndex \|\| 0` | ~50 | `0 \|\| 0 = 0` — safe |
| `opts = opts \|\| {}` | ~8 | Normalizing undefined |
| `runtime.dryRun \|\| false` | ~6 | `false \|\| false = false` |
| Boolean guards `!x \|\| !x.y` | ~40 | Not value assignment |
| Counter init `(c \|\| 0) + 1` | ~4 | `0 \|\| 0 = 0` |
| Other | ~300+ | Strings, objects, or intentional fallbacks |

## Test Coverage

New test file: `pr_split_falsy_fix_test.go` — 8 test functions validating:
- maxFiles=0 preserved (set command + display)
- exitCode=0 preserved (zero + undefined fallback)
- timeout=0 via typeof check
- maxFilesPerSplit=0 preserved
- maxConversationHistory=0 preserved
- AUTOMATED_DEFAULTS fields all present as numbers
