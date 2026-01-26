# Feasibility Report: Dynamic ClearPath Implementation for Pick-and-Place

**Date**: 2026-01-22
**Author**: Architecture Review
**Status**: FEASIBLE with caveats

---

## Executive Summary

This document analyzes the feasibility of replacing the current hardcoded blockade tracking (`DUMPSTER_ID`, `GOAL_BLOCKADE_IDS`, `isInBlockadeRing()`, `goalBlockade_X_cleared`) with a dynamic **ClearPath** mechanism that discovers and clears blocking cubes on-demand.

**Conclusion**: The approach is **architecturally sound and feasible**, but requires careful implementation to avoid infinite loops and performance issues. The proposed design follows the "Bridge Actions" pattern documented in `review.md`.

---

## 1. Pathfinding Extension Analysis

### 1.1 Current State

The existing BFS pathfinding functions are:

```javascript
// Builds set of blocked cells (excludes ignoreCubeId and held item)
function buildBlockedSet(state, ignoreCubeId) {
    const blocked = new Set();
    state.cubes.forEach(c => {
        if (c.deleted) return;
        if (c.id === ignoreCubeId) return;
        if (actor.heldItem && c.id === actor.heldItem.id) return;
        blocked.add(key(Math.round(c.x), Math.round(c.y)));
    });
    return blocked;
}

// Returns {reachable: boolean, distance: number}
function getPathInfo(state, startX, startY, targetX, targetY, ignoreCubeId);

// Returns next step toward target, or null if blocked
function findNextStep(state, startX, startY, targetX, targetY, ignoreCubeId);
```

### 1.2 Required Extension: `findFirstBlocker()`

**Question**: How to modify BFS to track which cube is blocking?

**Answer**: We can extend the BFS algorithm to track the **first** blocking cube encountered during expansion. When BFS fails to find a path, we perform a secondary scan to identify the nearest blocking entity.

```javascript
/**
 * Finds the ID of the first cube blocking the path from start to target.
 * Uses a two-phase approach:
 *   1. BFS to determine if path is blocked
 *   2. If blocked, scan frontier cells to find the nearest blocker
 *
 * @param {Object} state - Simulation state
 * @param {number} fromX - Starting X coordinate
 * @param {number} fromY - Starting Y coordinate
 * @param {number} toX - Target X coordinate
 * @param {number} toY - Target Y coordinate
 * @returns {number|null} - ID of first blocking cube, or null if path is clear
 */
function findFirstBlocker(state, fromX, fromY, toX, toY) {
    const key = (x, y) => x + ',' + y;
    const actor = state.actors.get(state.activeActorId);

    // Build cube position lookup: position -> cubeId
    const cubeAtPosition = new Map();
    state.cubes.forEach(c => {
        if (c.deleted) return;
        if (c.isStatic) return; // Walls can't be moved, ignore them
        if (actor.heldItem && c.id === actor.heldItem.id) return;
        cubeAtPosition.set(key(Math.round(c.x), Math.round(c.y)), c.id);
    });

    // Build blocked set (static obstacles only for initial path check)
    const staticBlocked = new Set();
    state.cubes.forEach(c => {
        if (c.deleted) return;
        if (actor.heldItem && c.id === actor.heldItem.id) return;
        staticBlocked.add(key(Math.round(c.x), Math.round(c.y)));
    });

    const visited = new Set();
    const frontier = []; // Cells we tried to enter but were blocked
    const queue = [{x: Math.round(fromX), y: Math.round(fromY)}];

    visited.add(key(queue[0].x, queue[0].y));

    const targetIX = Math.round(toX);
    const targetIY = Math.round(toY);

    // BFS to find path and collect frontier (blocked neighbors)
    while (queue.length > 0) {
        const current = queue.shift();

        // Check if we've reached adjacency to target
        const dx = Math.abs(current.x - targetIX);
        const dy = Math.abs(current.y - targetIY);
        if (dx <= 1 && dy <= 1) {
            return null; // Path exists, no blocker
        }

        const dirs = [[0, 1], [0, -1], [1, 0], [-1, 0]];
        for (const [ox, oy] of dirs) {
            const nx = current.x + ox;
            const ny = current.y + oy;
            const nKey = key(nx, ny);

            // Bounds check
            if (nx < 1 || nx >= state.width - 1 || ny < 1 || ny >= state.height - 1) continue;
            if (visited.has(nKey)) continue;

            if (staticBlocked.has(nKey)) {
                // This cell is blocked - add to frontier for later analysis
                frontier.push({x: nx, y: ny, parentX: current.x, parentY: current.y});
                visited.add(nKey); // Don't re-examine
            } else {
                // Cell is free - continue BFS
                visited.add(nKey);
                queue.push({x: nx, y: ny});
            }
        }
    }

    // Path is blocked - find the nearest movable blocker from frontier
    // Sort frontier by Manhattan distance to target (prioritize blockers closer to goal)
    frontier.sort((a, b) => {
        const distA = Math.abs(a.x - targetIX) + Math.abs(a.y - targetIY);
        const distB = Math.abs(b.x - targetIX) + Math.abs(b.y - targetIY);
        return distA - distB;
    });

    for (const cell of frontier) {
        const cubeId = cubeAtPosition.get(key(cell.x, cell.y));
        if (cubeId !== undefined) {
            const cube = state.cubes.get(cubeId);
            if (cube && !cube.isStatic) {
                return cubeId; // First movable blocker found
            }
        }
    }

    // All blockers are static walls - path is permanently blocked
    return null;
}
```

### 1.3 Can We Reuse `buildBlockedSet()`?

**Partially**. The current `buildBlockedSet()` conflates static and movable obstacles. For `findFirstBlocker()`, we need to:

1. **Identify** blockers (requires position → cubeId mapping)
2. **Filter** movable vs static (walls can't be cleared)

**Recommendation**: Create a new helper `buildBlockerMap()` that returns `Map<positionKey, cubeId>` for movable cubes only, and reuse `buildBlockedSet()` for the BFS traversal check.

---

## 2. Action Design Analysis

### 2.1 One Generic `ClearPath` vs `Move_Cube_X` per Cube

| Approach | Pros | Cons |
|----------|------|------|
| **Generic `ClearPath`** | Simple API, single action | Must internally decompose to Pick+MoveTo+Place; complex preconditions |
| **Per-Cube `Move_Cube_X`** | Composable with PA-BT; granular effects | Action explosion (N cubes = N×3 actions); ActionGenerator complexity |
| **Hybrid: `ClearPathTo_<target>`** | Planner sees single high-level action; internal decomposition handles details | Best of both; aligns with "Bridge Action" pattern |

**Recommendation**: Use the **Hybrid "Bridge Action"** pattern from `review.md`:

```javascript
// Bridge Action: ClearPathTo_Target
// Preconditions: [firstBlocker_cleared = true] (dynamically determined)
// Effects: [reachable_Target = true]
// Node: bt.success (dummy - exists only for planning)
```

The Bridge Action tells PA-BT "if you clear the first blocker, you'll make progress toward reachability." This is a **heuristic effect** - it doesn't guarantee full path clearance, but it guides the planner toward iterative clearance.

### 2.2 Handling "Place Anywhere" Requirement

Without a designated dumpster, cubes can be placed at any free adjacent cell. This is simpler than the current approach but has a key constraint:

**Critical**: Placing a cube at an arbitrary position might create NEW blockages elsewhere.

**Solution**: The `Place_ClearPath` action should prefer placement locations that don't block other paths. A simple heuristic:

```javascript
function findClearingSpot(state, actorX, actorY, targetPathX, targetPathY) {
    const candidates = getFreeAdjacentCells(state, actorX, actorY);

    // Filter: Don't place where it would block the path we're clearing
    return candidates.filter(spot => {
        // Simulate placing cube here - would it block the target path?
        const wouldBlock = isOnPath(state, actorX, actorY, targetPathX, targetPathY, spot.x, spot.y);
        return !wouldBlock;
    })[0] || candidates[0]; // Fallback to first available
}
```

For the initial implementation, a simpler approach: just place anywhere free. If this creates oscillation (pick-place loop), add the path-check heuristic.

---

## 3. Replanning Trigger Analysis

### 3.1 Current MoveTo Failure Flow

```
MoveTo tick:
  1. findNextStep() returns null (blocked)
  2. MoveTo returns bt.failure
  3. PA-BT replanning triggered
  4. State.Actions() called with failed condition
```

### 3.2 How Does PA-BT Know to Try ClearPath?

**Current behavior**: When `MoveTo` returns `bt.failure`, PA-BT replans from the parent node. It re-evaluates conditions and calls `State.Actions()` for any unsatisfied condition.

**The problem**: There's no condition key that explicitly represents "path is blocked." The planner just sees `atEntity_X = false` and generates another `MoveTo` - infinite loop.

**Solution**: Introduce a **reachable_X** condition family:

```javascript
// Blackboard sync (before tick)
function syncReachability(state) {
    const actor = state.actors.get(state.activeActorId);

    // For key entities (target, goal), sync reachability
    const pathInfo = getPathInfo(state, actor.x, actor.y, target.x, target.y);
    state.blackboard.set('reachable_entity_' + TARGET_ID, pathInfo.reachable);

    const goalPathInfo = getPathInfo(state, actor.x, actor.y, goal.x, goal.y);
    state.blackboard.set('reachable_goal_' + GOAL_ID, goalPathInfo.reachable);
}
```

Then modify `createMoveToAction()` to include reachability as a precondition:

```javascript
function createMoveToAction(state, entityType, entityId, extraEffects) {
    const reachableKey = entityType === 'cube'
        ? 'reachable_entity_' + entityId
        : 'reachable_goal_' + entityId;

    const conditions = [
        {key: reachableKey, match: v => v === true}  // NEW: Must be reachable
    ];

    // ... rest of action definition
}
```

### 3.3 ActionGenerator ClearPath Logic

When `reachable_X = false` triggers action generation:

```javascript
state.pabtState.setActionGenerator(function(failedCondition) {
    const actions = [];
    const key = failedCondition.key;

    // Handle reachability failures
    if (key && key.startsWith('reachable_')) {
        const targetIdStr = key.replace('reachable_entity_', '').replace('reachable_goal_', '');
        const targetId = parseInt(targetIdStr, 10);

        // Determine target position
        let targetX, targetY;
        if (key.includes('_entity_')) {
            const cube = state.cubes.get(targetId);
            if (cube) { targetX = cube.x; targetY = cube.y; }
        } else {
            const goal = state.goals.get(targetId);
            if (goal) { targetX = goal.x; targetY = goal.y; }
        }

        if (targetX !== undefined) {
            const actor = state.actors.get(state.activeActorId);
            const blockerId = findFirstBlocker(state, actor.x, actor.y, targetX, targetY);

            if (blockerId !== null) {
                // Generate ClearPath action for this specific blocker
                const clearAction = createClearPathAction(state, blockerId, key);
                actions.push(clearAction);
            }
        }
    }

    // ... existing handlers for other conditions
    return actions;
});
```

---

## 4. State Tracking Analysis

### 4.1 Without `goalBlockade_X_cleared`, What's Needed?

The current approach tracks clearance status of each specific blockade. With dynamic ClearPath:

**Minimum Required State**:

| Blackboard Key | Purpose |
|----------------|---------|
| `reachable_entity_X` | Is cube X reachable from actor position? (computed per tick) |
| `reachable_goal_X` | Is goal X reachable from actor position? (computed per tick) |
| `heldItemExists` | Is actor holding something? |
| `heldItemId` | ID of held item (or -1) |
| `atEntity_X` | Is actor adjacent to entity X? |
| `atGoal_X` | Is actor adjacent to goal X? |

**Removed State**:
- `goalBlockade_X_cleared` (N keys) → No longer needed; reachability is computed dynamically
- `DUMPSTER_ID` → No dumpster; place anywhere
- `GOAL_BLOCKADE_IDS` → No predefined blockade list; discover dynamically
- `isInBlockadeRing()` → No ring concept; just generic obstacles

### 4.2 Progress Tracking Without Explicit Cleared Flags

**How do we know we're making progress?**

1. **Reachability changes**: After clearing a blocker, `reachable_X` may become true (or path shortens)
2. **Actor position advances**: BFS distance to target decreases
3. **Win condition**: `cubeDeliveredAtGoal` becomes true

**Advantage**: No need to maintain N separate cleared flags. The environment is the source of truth.

**Caveat**: Must recompute reachability each tick (performance cost addressed in §5.2).

---

## 5. Potential Issues & Risk Analysis

### 5.1 Infinite Loop Risks

| Risk | Scenario | Mitigation |
|------|----------|------------|
| **Oscillation** | Pick cube A → Place at B → A now blocks another path → Pick A again | Place heuristic that avoids blocking active paths |
| **Impossible path** | All routes blocked by static walls | `findFirstBlocker` returns null for static-only blockage; MoveTo fails permanently |
| **Planner loop** | ActionGenerator returns same action repeatedly | PA-BT's existing 1M call limit (ISSUE-004 fix) |
| **Circular dependency** | ClearPath_A requires ClearPath_B requires ClearPath_A | Each ClearPath targets the FIRST blocker only; planner handles cascade |

**Critical Mitigation**: The "first blocker" approach ensures forward progress. We don't try to clear ALL blockers at once - just the nearest one, then replan.

### 5.2 Performance Concerns

| Operation | Frequency | Cost | Mitigation |
|-----------|-----------|------|------------|
| `findFirstBlocker()` | Per ActionGenerator call for reachable_* | O(V+E) BFS | Cache result per tick; invalidate on cube move |
| `syncReachability()` | Per tick | O(V+E) per target | Only sync for active planning targets; lazy evaluation |
| `buildBlockerMap()` | Per findFirstBlocker call | O(N) cubes | Memoize per tick |

**Recommendation**: Implement reachability caching:

```javascript
// Memoization for current tick
let reachabilityCache = null;
let reachabilityTick = -1;

function getReachability(state, targetId, isGoal) {
    if (reachabilityTick !== state.tickCount) {
        reachabilityCache = {};
        reachabilityTick = state.tickCount;
    }

    const cacheKey = (isGoal ? 'goal_' : 'entity_') + targetId;
    if (!(cacheKey in reachabilityCache)) {
        // Compute and cache
        const target = isGoal ? state.goals.get(targetId) : state.cubes.get(targetId);
        const actor = state.actors.get(state.activeActorId);
        const pathInfo = getPathInfo(state, actor.x, actor.y, target.x, target.y);
        reachabilityCache[cacheKey] = pathInfo.reachable;
    }
    return reachabilityCache[cacheKey];
}
```

### 5.3 Edge Cases

| Case | Expected Behavior | Implementation Note |
|------|-------------------|---------------------|
| All paths blocked by walls | `findFirstBlocker` returns null; planning fails | Add fallback error state |
| Target is inside wall | Same as above | Pre-validation of goal positions |
| Actor boxed in | No free cells for placement | `getFreeAdjacentCell` returns null; Place fails |
| Single-cell corridor | Only one path; clearing creates new block | Accept oscillation or implement "push" mechanic |

---

## 6. Code Skeleton: Key Functions

### 6.1 `findFirstBlocker()` (Complete implementation in §1.2)

### 6.2 `createClearPathAction()`

```javascript
/**
 * Creates a ClearPath bridge action for a specific blocker.
 * This action tells PA-BT: "clearing this blocker will improve reachability"
 *
 * The action decomposes internally to:
 *   1. MoveTo_cube_<blockerId>
 *   2. Pick_cube_<blockerId>
 *   3. Place (anywhere free)
 */
function createClearPathAction(state, blockerId, targetReachableKey) {
    const actionName = 'ClearPath_' + blockerId;

    // Preconditions: Must clear this specific blocker
    // This creates a dependency chain: ClearPath needs Pick, Pick needs MoveTo
    const conditions = [
        {key: 'cube_' + blockerId + '_removed', match: v => v === true}
    ];

    // Effect: After clearing blocker, target MAY become reachable
    // This is a HEURISTIC - we don't guarantee full path clearance
    const effects = [
        {key: targetReachableKey, Value: true}
    ];

    // Dummy tick: ClearPath is a planning-only action
    // Actual work is done by Pick/Place sequence
    const tickFn = () => bt.success;
    const node = bt.createLeafNode(tickFn);

    return pabt.newAction(actionName, conditions, effects, node);
}

/**
 * Action to pick up and remove a blocking cube
 */
function createPickBlockerAction(state, blockerId) {
    const name = 'Pick_Blocker_' + blockerId;

    const conditions = [
        {key: 'heldItemExists', match: v => v === false},
        {key: 'atEntity_' + blockerId, match: v => v === true}
    ];

    const effects = [
        {key: 'heldItemId', Value: blockerId},
        {key: 'heldItemExists', Value: true},
        {key: 'cube_' + blockerId + '_removed', Value: true}  // Blocker is now "removed" from path
    ];

    const tickFn = function() {
        const actor = state.actors.get(state.activeActorId);
        const cube = state.cubes.get(blockerId);
        if (!cube || cube.deleted) return bt.success;

        cube.deleted = true;
        actor.heldItem = {id: blockerId};

        state.blackboard.set('heldItemId', blockerId);
        state.blackboard.set('heldItemExists', true);
        return bt.success;
    };

    return pabt.newAction(name,
        conditions.map(c => ({key: c.key, match: c.match})),
        effects.map(e => ({key: e.key, Value: e.Value})),
        bt.createLeafNode(tickFn)
    );
}

/**
 * Generic Place action for path clearing (no specific destination)
 */
function createPlaceAnywhereAction(state) {
    const name = 'Place_Anywhere';

    const conditions = [
        {key: 'heldItemExists', match: v => v === true}
    ];

    const effects = [
        {key: 'heldItemExists', Value: false},
        {key: 'heldItemId', Value: -1}
    ];

    const tickFn = function() {
        const actor = state.actors.get(state.activeActorId);
        if (!actor.heldItem) return bt.success;

        const spot = getFreeAdjacentCell(state, actor.x, actor.y);
        if (!spot) return bt.failure;

        const cubeId = actor.heldItem.id;
        const cube = state.cubes.get(cubeId);
        if (cube) {
            cube.deleted = false;
            cube.x = spot.x;
            cube.y = spot.y;
        }

        actor.heldItem = null;
        state.blackboard.set('heldItemExists', false);
        state.blackboard.set('heldItemId', -1);
        return bt.success;
    };

    return pabt.newAction(name,
        conditions.map(c => ({key: c.key, match: c.match})),
        effects.map(e => ({key: e.key, Value: e.Value})),
        bt.createLeafNode(tickFn)
    );
}
```

### 6.3 Updated ActionGenerator

```javascript
state.pabtState.setActionGenerator(function(failedCondition) {
    const actions = [];
    const key = failedCondition.key;
    const targetValue = failedCondition.value;

    if (!key || typeof key !== 'string') return actions;

    // --- REACHABILITY HANDLING (NEW) ---
    if (key.startsWith('reachable_')) {
        const actor = state.actors.get(state.activeActorId);
        let targetX, targetY;

        if (key.startsWith('reachable_entity_')) {
            const entityId = parseInt(key.replace('reachable_entity_', ''), 10);
            const cube = state.cubes.get(entityId);
            if (cube && !cube.deleted) {
                targetX = cube.x;
                targetY = cube.y;
            }
        } else if (key.startsWith('reachable_goal_')) {
            const goalId = parseInt(key.replace('reachable_goal_', ''), 10);
            const goal = state.goals.get(goalId);
            if (goal) {
                targetX = goal.x;
                targetY = goal.y;
            }
        }

        if (targetX !== undefined) {
            const blockerId = findFirstBlocker(state, actor.x, actor.y, targetX, targetY);
            if (blockerId !== null) {
                // Generate the bridge action + its dependencies
                actions.push(createClearPathAction(state, blockerId, key));
                actions.push(createPickBlockerAction(state, blockerId));
                actions.push(createMoveToAction(state, 'cube', blockerId));
            }
        }
    }

    // --- CUBE REMOVED HANDLING (NEW) ---
    if (key.startsWith('cube_') && key.endsWith('_removed')) {
        const blockerId = parseInt(key.match(/cube_(\d+)_removed/)[1], 10);
        actions.push(createPickBlockerAction(state, blockerId));
        actions.push(createMoveToAction(state, 'cube', blockerId));
    }

    // --- EXISTING HANDLERS ---
    if (key === 'cubeDeliveredAtGoal') {
        const deliverAction = state.pabtState.GetAction('Deliver_Target');
        if (deliverAction) actions.push(deliverAction);
    }

    if (key === 'heldItemId') {
        if (targetValue === TARGET_ID) {
            const pickAction = state.pabtState.GetAction('Pick_Target');
            if (pickAction) actions.push(pickAction);
        }
    }

    if (key === 'heldItemExists' && targetValue === false) {
        actions.push(createPlaceAnywhereAction(state));
    }

    if (key.startsWith('atEntity_')) {
        const entityId = parseInt(key.replace('atEntity_', ''), 10);
        if (!isNaN(entityId)) {
            actions.push(createMoveToAction(state, 'cube', entityId));
        }
    }

    if (key.startsWith('atGoal_')) {
        const goalId = parseInt(key.replace('atGoal_', ''), 10);
        if (!isNaN(goalId)) {
            actions.push(createMoveToAction(state, 'goal', goalId));
        }
    }

    return actions;
});
```

---

## 7. Recommended Testing Approach

### 7.1 Unit Tests

| Test | Description | Validation |
|------|-------------|------------|
| `TestFindFirstBlocker_ClearPath` | Empty grid, actor to goal | Returns null |
| `TestFindFirstBlocker_SingleBlock` | One cube between actor and goal | Returns that cube's ID |
| `TestFindFirstBlocker_MultipleBlocks` | Chain of cubes | Returns nearest (by path distance) |
| `TestFindFirstBlocker_StaticWall` | Path blocked only by walls | Returns null |
| `TestFindFirstBlocker_MixedObstacles` | Wall + movable cube | Returns movable cube ID |

### 7.2 Integration Tests

| Test | Scenario | Expected Outcome |
|------|----------|------------------|
| `TestClearPath_SingleObstacle` | One cube blocks path to target | Actor clears cube, picks target, delivers |
| `TestClearPath_ChainedObstacles` | 3 cubes in a row | Actor clears iteratively (replan after each) |
| `TestClearPath_NoPathPossible` | Completely walled off | Graceful failure, no infinite loop |
| `TestClearPath_PlacementDoesntBlock` | Narrow corridor | Placed cube doesn't create new blockage |
| `TestClearPath_Performance` | 100 random obstacles | Completes in < 5 seconds |

### 7.3 Regression Tests

- Existing `TestPickAndPlaceConflictResolution` must still pass
- Existing `TestPickAndPlace_GoalDelivery` must still pass
- No test should exceed tick limit (currently ~6000 → should reduce significantly)

### 7.4 Debugging Strategy

```javascript
// Enable for development
const DEBUG_CLEARPATH = os.getenv('OSM_DEBUG_CLEARPATH') === '1';

function findFirstBlocker(state, fromX, fromY, toX, toY) {
    const result = /* ... BFS logic ... */;

    if (DEBUG_CLEARPATH) {
        log.debug('findFirstBlocker', {
            from: {x: fromX, y: fromY},
            to: {x: toX, y: toY},
            blockerId: result,
            tick: state.tickCount
        });
    }

    return result;
}
```

---

## 8. Implementation Phases

### Phase 1: Pathfinding Extension (Low Risk)
1. Implement `findFirstBlocker()`
2. Add unit tests
3. No changes to existing behavior

### Phase 2: ActionGenerator Augmentation (Medium Risk)
1. Add `reachable_*` condition handling
2. Add `ClearPath` bridge action generation
3. Keep existing blockade logic as fallback

### Phase 3: Remove Legacy Blockade Logic (High Risk)
1. Remove `DUMPSTER_ID`, `GOAL_BLOCKADE_IDS`
2. Remove `isInBlockadeRing()`, `goalBlockade_X_cleared`
3. Full integration testing

### Phase 4: Optimization (Post-validation)
1. Add reachability caching
2. Optimize `findFirstBlocker` performance
3. Profile and tune

---

## 9. Conclusion

**Feasibility**: ✅ APPROVED

The dynamic ClearPath approach is architecturally sound and addresses the "God-Precondition" anti-pattern identified in the review. Key success factors:

1. **First-blocker-only strategy** prevents infinite loops
2. **Heuristic effects** guide planner without guaranteeing full path clearance
3. **Existing PA-BT safety limits** (1M action calls) provide backstop
4. **Phased implementation** allows validation at each step

**Primary Risks**:
- Performance regression if reachability not cached
- Oscillation in degenerate map configurations
- Edge cases with static-only blockages

**Recommendation**: Proceed with Phase 1-2 implementation, validate with existing tests, then proceed to Phase 3.
