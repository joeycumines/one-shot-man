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
  - **LATEST DISCOVERY (tick 794 debug)**:
    - Precondition reordering FIXED: actor now reaches goal area (5,15) ✅
    - ClearPath_100_for_goal_1 IS executing and succeeds at tick 790 ✅
    - BUT: placement validation uses `findFirstBlocker` which returns NEAREST cube to actor
    - Cube 100 placed at (4,14) or (5,16) - still detected as "blocking" because:
      - `findFirstBlocker` adds ALL blocked cells encountered by BFS to frontier
      - Sorts by distance to ACTOR (wrong! design doc says distance to TARGET)
      - Cube 100 at (4,14) is distance 2 from actor, same as ring cubes
    - **FIX**: Change placement validation to use `getPathInfo().reachable` instead of `findFirstBlocker() !== blockerId`
    - This checks if path to goal is ACTUALLY clear, not just if specific cube is still blocking
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
