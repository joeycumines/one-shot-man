# Group E Review: Livelock Fix (pathBlocker Handler)

**Date**: 2026-01-23  
**Reviewer**: Takumi (匠)  
**Status**: ✅ **VERIFIED CORRECT**  

---

## Executive Summary

The livelock fix in Group E is **CORRECT**. The pathBlocker handler now returns both `createPickGoalBlockadeAction` and `createDepositGoalBlockadeAction`, allowing the PA-BT planner to chain the full Pick → Deposit sequence required to clear obstacles blocking the goal.

**Previously**: Handler only returned `Deposit_GoalBlockade_X`, causing infinite replanning because the planner couldn't satisfy Deposit's precondition (`heldItemId=X`) without a Pick action.

**Fixed**: Handler now returns BOTH actions. The planner can now chain:
1. `MoveTo_cube_X` (satisfies `atEntity_X`)
2. `Pick_GoalBlockade_X` (satisfies `heldItemId=X`)  
3. `Deposit_GoalBlockade_X` (satisfies `pathBlocker_goal_1=-1`)

---

## Detailed Analysis

### 1. pathBlocker Handler (Lines 1007-1044)

**Code Review:**
```javascript
if (key.startsWith('pathBlocker_')) {
    const destId = key.replace('pathBlocker_', '');
    const currentBlocker = state.blackboard.get(key);
    
    if (targetValue === -1) {
        if (typeof currentBlocker === 'number' && currentBlocker !== -1) {
            const cube = state.cubes.get(currentBlocker);
            if (cube) {
                // CRITICAL: Return Pick_GoalBlockade FIRST
                const pickAction = createPickGoalBlockadeAction(state, currentBlocker);
                if (pickAction) actions.push(pickAction);
                
                // Then Deposit which achieves pathBlocker_X = -1
                const depositAction = createDepositGoalBlockadeAction(state, currentBlocker, destId);
                if (depositAction) actions.push(depositAction);
            }
        }
    }
}
```

**Verdict**: ✅ CORRECT

- Handler correctly checks for `pathBlocker_` prefix
- Retrieves `currentBlocker` from blackboard (the cube ID blocking the path)
- Only generates actions when `targetValue === -1` (planner needs path cleared)
- Returns BOTH Pick and Deposit actions for the specific blocker
- Uses `destId` (e.g., `goal_1`) for the Deposit effect key

### 2. createPickGoalBlockadeAction (Lines 682-742)

**Preconditions:**
```javascript
const conditions = [
    { key: 'heldItemExists', value: false, Match: v => v === false },
    { key: 'atEntity_' + cubeId, value: true, Match: v => v === true }
];
```

**Effects:**
```javascript
const effects = [
    { key: 'heldItemId', Value: cubeId },
    { key: 'heldItemExists', Value: true }
];
```

**Verdict**: ✅ CORRECT

- Preconditions: Must have empty hands AND be at blocker location
- Effects: Sets `heldItemId=cubeId` (satisfies Deposit's precondition)
- Tick function properly deletes cube, sets `actor.heldItem`, updates blackboard
- Action name `Pick_GoalBlockade_X` matches test harness expectations (line 1201 of harness)

### 3. createDepositGoalBlockadeAction (Lines 751-862)

**Preconditions:**
```javascript
const conditions = [
    { key: 'heldItemId', value: cubeId, Match: v => v === cubeId }
];
```

**Effects:**
```javascript
const effects = [
    { key: 'heldItemExists', Value: false },
    { key: 'heldItemId', Value: -1 },
    { key: 'pathBlocker_' + destinationKey, Value: -1 }  // e.g., pathBlocker_goal_1 = -1
];
```

**Verdict**: ✅ CORRECT

- Precondition: Must be holding THIS specific blocker (`heldItemId=cubeId`)
- Effect: Clears `pathBlocker_{destinationKey}` to `-1`, satisfying MoveTo_goal precondition
- Placement logic prioritizes directions AWAY from goal (prevents re-blocking)
- Action name `Deposit_GoalBlockade_X` matches test harness expectations (line 1215)

### 4. Planner Chain Trace

**Scenario**: Planner needs `pathBlocker_goal_1 = -1`, current blocker is cube 105.

1. **ActionGenerator called with**: `key='pathBlocker_goal_1'`, `targetValue=-1`
2. **Handler reads**: `currentBlocker = state.blackboard.get('pathBlocker_goal_1')` → 105
3. **Handler returns**:
   - `Pick_GoalBlockade_105` (preconditions: `heldItemExists=false`, `atEntity_105=true`)
   - `Deposit_GoalBlockade_105` (preconditions: `heldItemId=105`)
4. **Planner expands Pick's precondition** `atEntity_105=true`:
   - ActionGenerator called with `key='atEntity_105'`, `targetValue=true`
   - Returns `MoveTo_cube_105` (no pathBlocker constraint per line 980-981)
5. **Final plan**: `MoveTo_cube_105` → `Pick_GoalBlockade_105` → `Deposit_GoalBlockade_105`

✅ **Chain complete and verified**

### 5. syncToBlackboard Correctness (Lines 492-552)

**Key Code:**
```javascript
state.goals.forEach(goal => {
    const dist = Math.sqrt(Math.pow(goal.x - ax, 2) + Math.pow(goal.y - ay, 2));
    bb.set('atGoal_' + goal.id, dist <= 1.5);

    // Compute path blocker to goal UNCONDITIONALLY every tick
    const goalX = Math.round(goal.x);
    const goalY = Math.round(goal.y);
    // Exclude TARGET_ID from being considered a blocker
    const blocker = findFirstBlocker(state, ax, ay, goalX, goalY, TARGET_ID);
    bb.set('pathBlocker_goal_' + goal.id, blocker === null ? -1 : blocker);
});
```

**Verdict**: ✅ CORRECT

- `pathBlocker_goal_1` computed every tick (not conditionally)
- Uses `findFirstBlocker` to find nearest MOVABLE cube on path to goal
- Excludes TARGET_ID from blockers (we carry target TO goal)
- Returns `-1` when path is clear, blocker ID otherwise

### 6. Edge Cases and Potential Loop Scenarios

#### Case A: Multiple blockers in series
**Scenario**: Path to goal blocked by cubes 105, 106, 107 in a line.

**Behavior**: 
- `findFirstBlocker` returns CLOSEST blocker to goal (sorted by distance to goal, line 347)
- After 105 is cleared, next tick `pathBlocker_goal_1` = 106
- Planner generates new Pick/Deposit for 106
- Repeat until all cleared

**Verdict**: ✅ CORRECT - Iterative clearing handled properly

#### Case B: Actor already holding a blocker
**Scenario**: `heldItemId=105`, planner needs `pathBlocker_goal_1=-1`.

**Behavior**:
- `pathBlocker` handler generates `Pick_GoalBlockade_105` + `Deposit_GoalBlockade_105`
- Pick has precondition `heldItemExists=false` → FAILS (already holding 105)
- Planner tries `Deposit_GoalBlockade_105` → precondition `heldItemId=105` → SUCCEEDS

**Verdict**: ✅ CORRECT - Planner correctly skips Pick when already holding the blocker

#### Case C: Blocker already picked (cube.deleted=true)
**Scenario**: Cube 105 is `deleted=true` (held by actor), handler called.

**Code check (line 1031-1033)**:
```javascript
const cube = state.cubes.get(currentBlocker);
// NOTE: The cube might be picked up (deleted=true) but we still need
// to generate actions. The cube exists in state.cubes even when held.
if (cube) {  // Only checks existence, not deleted status
```

**Verdict**: ✅ CORRECT - Comment explicitly addresses this; actions generated regardless of `deleted` status

#### Case D: Deposit places blocker back in path
**Scenario**: Agent deposits blocker but it lands where it blocks path again.

**Mitigation (lines 785-805)**: Deposit prioritizes directions AWAY from goal:
```javascript
// Prioritize directions AWAY from goal
const dirToGoalX = goalX - ax;
const dirToGoalY = goalY - ay;
const dirs = [];
if (dirToGoalX < 0) {
    dirs.push([1, 0], [1, -1], [1, 1]);
} else {
    dirs.push([-1, 0], [-1, -1], [-1, 1]);
}
```

**Verdict**: ⚠️ MOSTLY CORRECT - Heuristic helps but doesn't guarantee. However, if re-blocking occurs, `findFirstBlocker` will detect it next tick and planner will clear it again. System is self-correcting.

#### Case E: Infinite loop with unstable blocker detection
**Scenario**: Could `pathBlocker_goal_1` oscillate between values?

**Analysis**: 
- `findFirstBlocker` is deterministic given same state
- Blocker is removed from path when picked up (cube.deleted=true)
- After Deposit, blocker is at new location (away from goal)
- Unless placement creates NEW blocker (edge case D), no oscillation

**Verdict**: ✅ CORRECT - No oscillation risk under normal operation

### 7. Test Harness Compatibility

**Harness expectations (pick_and_place_harness_test.go)**:
```go
case strings.HasPrefix(action, "Pick_GoalBlockade_"):   // Line 1201
case strings.HasPrefix(action, "Deposit_GoalBlockade_"): // Line 1215
```

**Script action names**:
- `Pick_GoalBlockade_` + cubeId (line 684)
- `Deposit_GoalBlockade_` + cubeId (line 753)

**Verdict**: ✅ CORRECT - Names match exactly

### 8. Hardcoded 'goal_1' Assumption

**Code (line 965)**:
```javascript
const goalId = 1; // Assuming single goal
const depositAction = createDepositGoalBlockadeAction(state, currentHeldId, 'goal_' + goalId);
```

**Also in pathBlocker handler (line 1008)**:
```javascript
const destId = key.replace('pathBlocker_', '');  // Dynamic from key!
```

**Verdict**: ✅ ACCEPTABLE

- The `heldItemExists` handler uses hardcoded `goal_1` for the convenience of deposit action creation
- BUT the pathBlocker handler correctly extracts `destId` from the key itself
- Since scenario only has one goal (GOAL_ID=1), this is acceptable
- Multi-goal support would require refactoring, but is out of scope

---

## Unverified Assumptions (Explicit Trust Statements)

1. **PA-BT Planner Behavior**: I trust that go-pabt correctly expands action preconditions and chains actions. This is fundamental library behavior not verified in this review.

2. **BFS Pathfinding Correctness**: `findFirstBlocker` and `findNextStep` use BFS. I trust these are implemented correctly based on extensive prior testing.

3. **JavaScript Execution Order**: I assume the Goja runtime executes code deterministically and array iteration order is stable.

---

## Conclusion

The livelock fix is **VERIFIED CORRECT**. The change from returning only `Deposit_GoalBlockade_X` to returning BOTH `Pick_GoalBlockade_X` AND `Deposit_GoalBlockade_X` resolves the fundamental issue: the planner now has access to the full action chain needed to clear path blockers.

**Test Evidence**: `TestPickAndPlaceCompletion` passes, demonstrating WIN condition is achieved without tick stalls. `TestPickAndPlaceConflictResolution` verifies correct event sequence.

---

## Checklist

| Item | Status | Notes |
|------|--------|-------|
| pathBlocker handler returns Pick + Deposit | ✅ | Lines 1035-1042 |
| createPickGoalBlockadeAction correct | ✅ | Lines 682-742 |
| createDepositGoalBlockadeAction correct | ✅ | Lines 751-862 |
| Planner can chain Pick → Deposit | ✅ | Traced above |
| atEntity_X handler returns MoveTo | ✅ | Lines 976-982 |
| syncToBlackboard computes pathBlocker_goal | ✅ | Lines 533-549 |
| No infinite loop scenarios | ✅ | Edge cases analyzed |
| Action names match test harness | ✅ | Pick_GoalBlockade_, Deposit_GoalBlockade_ |
| Tests pass | ✅ | make-all-with-log PASS |
