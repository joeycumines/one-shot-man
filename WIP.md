# WIP — Takumi's Desperate Diary

## Current State
**TASK:** T01 — Fix blocking startAnalysis() → tick-based async
**STATUS:** Code edits COMPLETE. Need build verification + Rule of Two + git commit.

## What Was Done (T01)
1. ✅ Added Tick message handler in `_updateFn` (between mouseWheel and final return)
   - Dispatches `analysis-step-0` through `analysis-step-3` to `runAnalysisStep(s, N)`
2. ✅ Refactored `startAnalysis(s)` to only set up state + return `tea.tick(1, 'analysis-step-0')`
   - No longer calls analyzeDiff/applyStrategy/createSplitPlan/validatePlan synchronously
3. ✅ Created `runAnalysisStep(s, stepIdx)` — runs ONE step per tick call:
   - Step 0: analyzeDiff (try/catch, error → ERROR state)
   - Step 1: applyStrategy
   - Step 2: createSplitPlan
   - Step 3: validatePlan → transition to PLAN_REVIEW
   - Each step returns `[s, tea.tick(1, 'analysis-step-N+1')]` to yield for render
4. ✅ Removed orphaned old synchronous code (duplicate Step 4: Validate block)
5. ✅ Cancellation: each step checks `!s.isProcessing` at top and bails

## Architecture Decision
- Using `tea.tick(1, 'analysis-step-N')` pattern (1ms ticks) to yield between steps
- Each step still blocks event loop during execution (~1-5s each)
- BUT between steps: BubbleTea renders, user can Ctrl+C to cancel
- This is the simplest approach using existing infrastructure (no Go changes)
- True non-blocking requires CaptureSession (T11-T12 territory)

## Next Steps
1. Run `make` or `make make-all-with-log` — verify build/lint/test pass
2. Rule of Two: spawn 2 contiguous review subagents
3. git-commit T01
4. Update blueprint.json: T01 → "Done"
5. Begin T02 (startExecution — same tick pattern)

## File Paths
- Blueprint: `/Users/joeyc/dev/one-shot-man/blueprint.json`
- TUI file: `internal/command/pr_split_13_tui.js` (~4000 lines)
- Go registration: `internal/command/pr_split.go`
- Reference impl: `internal/command/super_document_script.js`
- Design doc: `docs/pr-split-tui-design.md`
- User feedback: `scratch/idiot.md`
