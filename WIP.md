# WIP - Pick-and-Place PA-BT Conflict Resolution Demo

## Current Goal
Modify example-05-pick-and-place.js to create a challenging scenario demonstrating PA-BT conflict resolution.

## Active Task
Task 1: Modify scenario to add movable wall AROUND goal

## High-Level Action Plan
1. **Scenario Modification**: Add tight movable wall around goal at (8,18)
2. **New Actions**: Add Place_Target_Temporary, Pick_GoalBlockade_X, Deposit_GoalBlockade_X
3. **Logging**: Add exhaustive structured logs for state verification
4. **Go Integration Test**: Build state mirroring, track deltas, verify programmatically

## Expected Agent Behavior (PA-BT Conflict Resolution)
1. Agent navigates to target, clears path blockades
2. Agent picks up target cube
3. Agent moves toward goal → **BLOCKED** by goal wall
4. PA-BT detects `atGoal_1` precondition fails for Deliver_Target
5. PA-BT replans: needs to clear goal wall, but hands full (holding target)
6. **CONFLICT**: Can't Pick_GoalBlockade while holding target
7. PA-BT generates Place_Target_Temporary action to free hands
8. Agent places target, picks goal blockade, deposits at dumpster
9. Agent retrieves target (second Pick_Target)
10. Agent delivers target to now-accessible goal
11. **WIN**

## Progress Log
- [2026-01-20] Initialized blueprint.json and WIP.md
- [2026-01-20] Research complete: PA-BT replanning, termtest patterns, current implementation

## References
- See `./blueprint.json` for detailed task breakdown
- Harness: `internal/command/pick_and_place_harness_test.go`
- Script: `scripts/example-05-pick-and-place.js`
