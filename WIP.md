# WIP - Pick-and-Place PA-BT Conflict Resolution Demo

## Current Goal
VERIFY AND FIX: Run integration test to understand current failure, then fix based on textbook pattern.

## SESSION STATUS (2026-01-20 - REBOOT)
Previous sessions used heuristic effects which VIOLATE PA-BT's trust model.
Root cause documented in `unconfirmed-academic-failure-analysis.md`.

## KEY INSIGHT FROM TEXTBOOK ANALYSIS
1. Effects MUST be TRUTHFUL - no heuristics
2. Use per-blockade conditions: `goalBlockade_X_cleared` instead of aggregate `reachable_goal_1`
3. Deposit_GoalBlockade_X has truthful effect `goalBlockade_X_cleared=true`
4. PA-BT discovers conflict resolution chain naturally

## EXPECTED PLANNING CHAIN
```
Goal: cubeDeliveredAtGoal=true
  → Deliver_Target (needs heldItemId=1, atGoal_1)
    → atGoal_1 fails
      → MoveTo_goal_1 (needs goalBlockade_100_cleared=true)
        → Deposit_GoalBlockade_100 (needs heldItemId=100, atGoal_2)
          → heldItemId=100 fails (currently heldItemId=1 or -1)
            → Pick_GoalBlockade_100 (needs atEntity_100, heldItemExists=false)
              → heldItemExists=false fails (holding target!)
                → Place_Target_Temporary (needs heldItemId=1, atGoal_3)
                  → DISCOVERED! Free hands, clear blockades, retrieve target.
```

## High-Level Action Plan
1. Run current test → capture failure mode
2. Analyze logs with subagent
3. Fix based on findings
4. Iterate until test passes
5. Scale to 8 blockades
6. Verify make-all-with-log passes

## Progress Log
- [2026-01-20] Reboot session - studying textbook pattern
- [2026-01-20] Understood: per-blockade conditions with truthful effects

## References
- See `./blueprint.json` for detailed task breakdown
- Root cause: `./unconfirmed-academic-failure-analysis.md`
- Script: `scripts/example-05-pick-and-place.js`
- Test: `internal/command/pick_and_place_harness_test.go`
