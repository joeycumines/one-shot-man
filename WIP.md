# WIP - Pick-and-Place PA-BT Fix

## Current Goal
Fix the PA-BT planning logic so the actor actually picks up and places cubes instead of just moving around.

## Status
- **Started**: 2026-01-21
- **Current Task**: Task 9 - Fix PA-BT Planning Logic

## Recent Progress
- ✅ Fixed terminal width issue (was 80 cols, now 200 cols) - JSON was being truncated
- ✅ TestPickAndPlaceInitialState PASSES
- ✅ TestPickAndPlaceLogging PASSES  
- ⚠️ TestPickAndPlaceConflictResolution FAILING - stuck at tick 74, actor moves but no cube manipulation

## Current Issue
Simulation stuck:
- Actor moves from x=5 to x=41 (good - pathfinding works)
- But `goalBlockadeCount` stays at 16 (bad - no cubes removed)
- `heldItemId` stays at -1 (bad - never picks up anything)

## Investigation Needed
1. Check if PA-BT planner is generating pick/place actions
2. Check if action execution is being triggered
3. Check if actor-to-cube distance check is working for pickup

## High Level Action Plan
1. ✅ Fix terminal width for JSON parsing
2. ✅ Verify basic tests pass (Initial, Logging)
3. → Diagnose why actor doesn't pick cubes (check action execution logs)
4. Fix action execution or generation
5. Run ConflictResolution test
6. Run Completion test
7. Run full test suite
8. Clean up debug code

## Reference: ./blueprint.json for detailed task tracking
