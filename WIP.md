# WIP — Active Session State

## Session Start
- **Timestamp**: 2026-03-05 03:16:49 (tracked in scratch/.session-start)
- **Mandate**: 9 hours of continuous improvement
- **Elapsed**: ~16h (context window 8 of session)

## Current Phase: B00 VERIFIED — Rule of Two, then B01

### Completed This Session
1. G01: Commit verified test fixes
2. R28-T01/T02/T03: Test coverage additions
3. R00b-01/02/03: Claudemux cleanup (5454 lines deleted)
4. R00-01/02/03: Termmux/ui relocation
5. R32-01/02: Stale reference cleanup
6. R29/R30/R31-01/02/03: Documentation updates
7. R00a: Git state isolation verification
8. R39-01/02: Cross-platform validation (Linux + Windows)
9. B00: Fix test git state mutation — commit e655943

### B00 Fix Summary
**Root Cause:** Tests with `os.Chdir(tmpDir)` + `dir:'.'` in JS caused
`gitExec('.')` to run bare git in go test package CWD. LIFO cleanup ordering
restored CWD before engine cleanup.

**Fix (5 layers):**
1. `runtime.dir` config field in JS — tests inject absolute temp dir
2. `resolveDir()` uses `runtime.dir` (no exec.execv fallback)
3. All gitExec callers go through resolveDir for dir resolution
4. `chdirTestPipeline` LIFO ordering fix + autosplit recovery tests converted
5. `loadChunkEngine` injects `t.TempDir()` when no dir configured

**KEY INSIGHT:** `loadPrSplitEngine`/`loadPrSplitEngineWithEval` MUST NOT
have auto-dir injection — their callers do `os.Chdir(testGitRepoDir)` and
need CWD to be their test repo. Only `loadChunkEngine` needs it.

### Next Steps
- Rule of Two verification on B00
- B01: ANSI escape codes in split branch messages
- B02: GraphQL error in gh pr create
- Continue blueprint tasks (R28.1-R28.4, R41, R42, BP-01, W00-W14)
