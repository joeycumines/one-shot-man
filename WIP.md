# WIP - Pick-and-Place PA-BT Conflict Resolution Demo

## Current Goal
UNDERSTAND: Study textbook go-pabt example and apply correct pattern

## SESSION STATUS (2026-01-21)
Test loops infinitely:
1. `MoveTo_goal_3` succeeds (actor at staging area)
2. `MoveTo_goal_1` fails (no path due to blockades)
3. ActionGenerator returns `MoveTo_goal_3` for `reachable_goal_1` failure
4. LOOP: Steps 1-3 repeat forever

## CRITICAL INSIGHT FROM TEXTBOOK
From `github.com/joeycumines/go-pabt/examples/tcell-pick-and-place/logic/logic.go`:
- **ALL effects are TRUTHFUL** - Pick effect is `h = cube`, Place effect is `o_i = p`, Move effect is `o_r = p`
- **PA-BT generates actions for ALL possible (x,y) positions and sprites**
- **NO "heuristic" effects** - every effect describes what actually happens
- **Preconditions check COLLISION-FREE paths** - noCollisionConds checks if path is clear

## WHY CURRENT APPROACH FAILS
1. `Place_Target_Temporary` exists but PA-BT never finds it for `reachable_goal_1`
2. ActionGenerator returns `MoveTo_goal_3` with **LYING** heuristic effect `reachable_goal_1=true`
3. Moving to staging area does NOT make goal reachable - actor still holds item, blockades still exist
4. Textbook approach: effects describe POSITION changes, not abstract "reachability"

## THE CORRECT FIX
Model effects like textbook - use POSITION-BASED effects:
1. `Place_Target_Temporary`: effect = `itemPosition_1 = staging_area`, `holding = null`
2. `Pick_GoalBlockade_X`: effect = `holding = X`, `itemPosition_X = null` (picked up = no longer blocking)
3. `Place_Blockade_Dumpster`: effect = `itemPosition_X = dumpster`, `holding = null`
4. Let PA-BT discover path: place target → clear blockades → retrieve target → deliver

## ALTERNATIVE: Runtime Validation (SIMPLER)
Instead of redesigning effects, add runtime checks in actions:
1. `Deliver_Target` ALREADY validates actor is at goal (added this session)
2. Problem: PA-BT loops because no action advances state
3. Need: Action that ACTUALLY changes state when `reachable_goal_1` fails

## High-Level Action Plan
1. **STUDY textbook logic.go pattern** (CURRENT)
2. **Redesign effects to be truthful** (model positions, not reachability)
3. **VERIFY PA-BT discovers conflict resolution path**
4. **TEST: make-all-with-log passes**

## Progress Log
- [Session N-3] Fixed timeout/freeze with heuristic effects in ActionGenerator
- [Session N-2] Test completes (~22s) but wrong path taken
- [Session N-1] ROOT CAUSE: Heuristic effects violate PA-BT's trust model
- [Session N] Added runtime validation to Deliver_Target
- [Session N] Removed heuristic from atGoal_1 handler → now returns truthful MoveTo_goal_1
- [Session N] Added reachable_goal_1=true effect to Place_Target_Temporary
- [Session N] PROBLEM: Test loops - Place_Target_Temporary never selected

## References
- See `./blueprint.json` for detailed task breakdown
- Script: `scripts/example-05-pick-and-place.js`
- Textbook: https://github.com/joeycumines/go-pabt/examples/tcell-pick-and-place/logic/logic.go
