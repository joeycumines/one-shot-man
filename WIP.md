# WIP - Pick-and-Place DYNAMIC Obstacle Handling

## Current Goal
Complete architectural overhaul: Remove ALL hardcoded blockade handling, implement DYNAMIC obstacle discovery per PA-BT principles. **MUST complete EVERY task in blueprint.json.**

## Status
- **Date**: 2026-01-22
- **Phase**: EXECUTING GROUP E - Integration Tests

## Reference
- See `./blueprint.json` for EXHAUSTIVE task list (7 groups with review cycles)
- See `./review.md` for full analysis of issues to fix

## Policy (MANDATORY)
> ALL tasks and subtasks contained within this blueprint MUST be completed in their entirety. Deferring, skipping, or omitting any part of the plan is strictly prohibited.

## Architecture Principle
**Agent MUST NOT bake in ANY knowledge of blockade layout.** Obstacles are discovered dynamically via pathfinding and cleared to arbitrary locations.

## Completed Groups ✅
- ✅ Group 0: Infrastructure Fixes (3 tasks)
- ✅ Group A: Infrastructure Cleanup (5 tasks)
- ✅ Group A Review: (3 review tasks)
- ✅ Group B: Blackboard Improvements (3 tasks)
- ✅ Group B Review: (3 review tasks)
- ✅ Group C: Action/Generator Refinement (4 tasks)
- ✅ Group C Review: (3 review tasks)
- ✅ Group D: Cleanup & Simplification (4 tasks)
- ✅ Group D Review: (3 review tasks)

## Current Focus: Group E - Integration Tests
- ⏳ E-T01: Add TestPickAndPlaceDynamicBlockerDetection
- ⏳ E-T02: Add TestPickAndPlaceClearPathAction
- ⏳ **E-T03: Enable/Fix TestPickAndPlaceConflictResolution** ← CURRENT
  - **ISSUE DISCOVERED**: Test enabled but FAILING
  - Test expects `Place_Target_Temporary` action (CONFLICT_RESOLUTION event)
  - Test expects `Pick_GoalBlockade_*` / `Deposit_GoalBlockade_*` actions (GOAL_WALL_CLEAR events)
  - **ROOT CAUSE**: ClearPath action is an "Atomic Action" anti-pattern (review.md 3.2)
  - ClearPath internally handles placing held target - planner never plans Place_Target_Temporary
  - **FIX REQUIRED**: Add `heldItemExists: false` precondition to ClearPath, let planner plan separate steps
- ⏳ E-T04: Add TestPickAndPlaceNoHardcodedBlockades
- ⏳ E-T05: Run full test suite 100% pass

## Pending Groups
- Group E Review: 3 review cycle tasks
- Group F: Final Verification (4 tasks)

## Already Implemented
- ✅ findFirstBlocker() function
- ✅ createClearPathAction() for dynamic clearing
- ✅ createPickObstacleAction() for dynamic picking
- ✅ ActionGenerator handles pathBlocker_* keys
- ✅ Threshold set to 1.5
- ✅ actionCache removed
