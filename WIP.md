# WIP - Pick-and-Place DYNAMIC Obstacle Handling

## Current Goal
Complete architectural overhaul: Remove ALL hardcoded blockade handling, implement DYNAMIC obstacle discovery per PA-BT principles. **MUST complete EVERY task in blueprint.json.**

## Status
- **Date**: 2026-01-23
- **Phase**: EXECUTING CRITICAL GROUP - ClearPath Decomposition

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

## Current Focus: CRITICAL - ClearPath Decomposition
### DONE:
- ✅ CRIT-01: Removed createClearPathAction entirely
- ✅ CRIT-02: Created createPickGoalBlockadeAction (Pick_GoalBlockade_X)
- ✅ CRIT-03: Created createDepositGoalBlockadeAction (Deposit_GoalBlockade_X)
- ✅ CRIT-04: Updated ActionGenerator to produce decomposed actions
- ✅ Removed pathBlocker_goal_1 precondition from Pick_Target (allows conflict resolution pattern)

### CURRENT ISSUE:
- Place_Target_Temporary executes at tick 3536 ✅
- But then tick gets STUCK at 3538
- Agent placed target at (9,19)
- Now needs to: Pick_GoalBlockade → Deposit_GoalBlockade → Pick_Target → Deliver
- Something in the planning loop is causing infinite expansion

### Next Steps:
1. Investigate why tick is stuck after Place_Target_Temporary
2. Check if ActionGenerator is returning proper actions for clearing the blocker
3. May need to add pathBlocker detection when NOT holding target
