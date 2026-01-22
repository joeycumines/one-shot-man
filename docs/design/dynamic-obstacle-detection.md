# Dynamic Obstacle Detection and Clearing System

**Design Document**  
**Date:** 2026-01-22  
**Status:** DESIGN PROPOSAL

---

## Table of Contents

1. [Problem Statement](#problem-statement)
2. [Architecture Overview](#architecture-overview)
3. [Pathfinding with Blocker Discovery](#pathfinding-with-blocker-discovery)
4. [Blackboard State Design](#blackboard-state-design)
5. [Action Definitions](#action-definitions)
6. [ActionGenerator Logic](#actiongenerator-logic)
7. [Planning Sequence Example](#planning-sequence-example)
8. [Key Design Decisions](#key-design-decisions)

---

## Problem Statement

### Current (WRONG) Implementation

The existing pick-and-place implementation uses a **hardcoded obstacle handling** approach:

```javascript
// WRONG: Pre-defined blockade IDs
GOAL_BLOCKADE_IDS = [100, 101, ..., 115];

// WRONG: Hardcoded dumpster location
goals.set(DUMPSTER_ID, {x: 8, y: 4, forBlockade: true});

// WRONG: Hardcoded geometry detection
function isInBlockadeRing(x, y) { /* fixed ring coords */ }

// WRONG: Static blockade-specific blackboard keys
bb.set('goalBlockade_100_cleared', false);
bb.set('goalBlockade_101_cleared', false);
// ... 14 more

// WRONG: God-precondition anti-pattern
Pick_Target.preconditions = [
    'goalBlockade_100_cleared',
    'goalBlockade_101_cleared',
    // ... all 16 blockades explicitly listed
];
```

**Problems with this approach:**

1. **Brittle**: Changing obstacle count/layout requires code changes
2. **Non-generalizable**: Only works for the specific "ring around goal" scenario
3. **Planner Bypass**: Developer manually encodes the solution instead of letting planner discover it
4. **Infinite Action Space**: Every obstacle requires dedicated Pick_Blockade_X + Deposit_Blockade_X actions

### Required (CORRECT) Behavior

- Obstacles are **generic cubes** with no special designation
- When agent tries to reach a target and path is blocked:
  1. Use pathfinding to **discover** which specific cube(s) are blocking
  2. Pick up the **nearest** blocking cube
  3. Place it at **any nearby free cell** (not a specific dumpster)
  4. **Retry** the original goal

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           GOAL CONDITION                                 │
│                    cubeDeliveredAtGoal = true                           │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         PA-BT PLANNER                                    │
│                                                                          │
│  1. Check: Is path to goal clear?                                       │
│     ├─ YES → Plan: [MoveTo_Goal, Deliver_Target]                        │
│     └─ NO  → Discover blocker, Plan: [ClearPath, Retry]                 │
│                                                                          │
│  2. ClearPath uses dynamic discovery:                                   │
│     ├─ findBlockerOnPath() identifies blocking cube ID                  │
│     ├─ ActionGenerator creates ClearObstacle_<ID> action                │
│     └─ After clearing, replanning retries MoveTo_Goal                   │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                      REACTIVE EXECUTION LOOP                             │
│                                                                          │
│  For each tick:                                                         │
│    1. syncToBlackboard() - update world state                           │
│    2. Plan.tick() - execute current plan step                           │
│    3. If MoveTo fails → replan with discovered blocker                  │
│    4. Repeat until goal achieved                                        │
└─────────────────────────────────────────────────────────────────────────┘
```

### Key Insight: Lazy Discovery vs Eager Enumeration

| Approach | Eager (Current) | Lazy (Proposed) |
|----------|-----------------|-----------------|
| Obstacle knowledge | All IDs known at init | Discovered on-demand |
| Preconditions | Explicit list of all blockers | Single `pathClear` check |
| Action generation | All Pick/Deposit at init | ClearObstacle_X on failure |
| Replanning trigger | Never (all planned upfront) | On pathfinding failure |

---

## Pathfinding with Blocker Discovery

### Core Function: `findBlockerOnPath`

The pathfinding algorithm must be enhanced to not just return "reachable/unreachable" but also **identify the first blocking entity** when unreachable.

```typescript
interface BlockerResult {
    blocked: boolean;           // true if path is blocked
    blockerId: number | null;   // ID of first blocking cube, or null
    blockerPosition: {x: number, y: number} | null;
}

/**
 * BFS pathfinding that identifies blocking obstacles.
 * 
 * @param state - Current simulation state
 * @param fromX - Starting X coordinate
 * @param fromY - Starting Y coordinate  
 * @param toX - Target X coordinate
 * @param toY - Target Y coordinate
 * @returns BlockerResult with blocking status and blocker info
 */
function findBlockerOnPath(
    state: SimulationState,
    fromX: number, 
    fromY: number,
    toX: number, 
    toY: number
): BlockerResult {
    const visited = new Set<string>();
    const key = (x: number, y: number) => `${x},${y}`;
    
    // Build obstacle map: position -> cube ID
    const obstacles = new Map<string, number>();
    state.cubes.forEach(cube => {
        if (!cube.deleted && !cube.isStatic) {
            obstacles.set(key(Math.round(cube.x), Math.round(cube.y)), cube.id);
        }
    });
    
    // BFS from target toward start (for "first blocker" from goal perspective)
    const queue = [{x: Math.round(toX), y: Math.round(toY)}];
    visited.add(key(queue[0].x, queue[0].y));
    
    const dirs = [[0, 1], [0, -1], [1, 0], [-1, 0]];
    let nearestBlocker: {id: number, x: number, y: number, dist: number} | null = null;
    
    while (queue.length > 0) {
        const current = queue.shift()!;
        
        // Check if reached start position
        if (Math.abs(current.x - fromX) <= 1 && Math.abs(current.y - fromY) <= 1) {
            return { blocked: false, blockerId: null, blockerPosition: null };
        }
        
        for (const [dx, dy] of dirs) {
            const nx = current.x + dx;
            const ny = current.y + dy;
            const nKey = key(nx, ny);
            
            if (visited.has(nKey)) continue;
            visited.add(nKey);
            
            // Check bounds
            if (nx < 0 || nx >= state.width || ny < 0 || ny >= state.height) continue;
            
            // Check for obstacle
            const obstacleId = obstacles.get(nKey);
            if (obstacleId !== undefined) {
                // Found a blocking obstacle - track nearest to start
                const distToStart = Math.abs(nx - fromX) + Math.abs(ny - fromY);
                if (!nearestBlocker || distToStart < nearestBlocker.dist) {
                    nearestBlocker = { id: obstacleId, x: nx, y: ny, dist: distToStart };
                }
                continue; // Don't expand through obstacles
            }
            
            queue.push({x: nx, y: ny});
        }
    }
    
    // Path blocked - return the nearest blocker
    if (nearestBlocker) {
        return { 
            blocked: true, 
            blockerId: nearestBlocker.id,
            blockerPosition: { x: nearestBlocker.x, y: nearestBlocker.y }
        };
    }
    
    // Completely isolated - no path possible
    return { blocked: true, blockerId: null, blockerPosition: null };
}
```

### Integration Point: MoveTo Tick Function

```javascript
function createMoveToAction(state, targetX, targetY, targetName) {
    return bt.createLeafNode(() => {
        const actor = state.actors.get(state.activeActorId);
        
        // Check reachability with blocker discovery
        const blockerResult = findBlockerOnPath(
            state, actor.x, actor.y, targetX, targetY
        );
        
        if (blockerResult.blocked) {
            // Record the blocker in blackboard for ActionGenerator
            if (blockerResult.blockerId !== null) {
                state.blackboard.set('pathBlockedBy', blockerResult.blockerId);
                state.blackboard.set('pathBlockedAt_x', blockerResult.blockerPosition.x);
                state.blackboard.set('pathBlockedAt_y', blockerResult.blockerPosition.y);
            }
            
            log.debug('MoveTo failed - path blocked by cube', {
                blockerId: blockerResult.blockerId,
                from: {x: actor.x, y: actor.y},
                to: {x: targetX, y: targetY}
            });
            
            return bt.failure; // Triggers replanning
        }
        
        // Clear any previous blocker state
        state.blackboard.set('pathBlockedBy', -1);
        
        // ... normal pathfinding step execution ...
        return bt.running;
    });
}
```

---

## Blackboard State Design

### Minimal Key Set

The dynamic approach requires **fewer** blackboard keys than the hardcoded approach:

| Key | Type | Description | When Updated |
|-----|------|-------------|--------------|
| `actorX`, `actorY` | number | Actor position | Every tick |
| `heldItemExists` | boolean | Is actor holding something? | On Pick/Place |
| `heldItemId` | number | ID of held item (-1 if none) | On Pick/Place |
| `pathBlockedBy` | number | ID of cube blocking current path (-1 if clear) | On MoveTo failure |
| `pathBlockedAt_x`, `pathBlockedAt_y` | number | Position of blocking cube | On MoveTo failure |
| `cube_<ID>_cleared` | boolean | Has this specific cube been moved out of the way? | On ClearObstacle success |
| `atEntity_<ID>` | boolean | Is actor adjacent to entity? | Every tick (computed) |
| `cubeDeliveredAtGoal` | boolean | Win condition | On Deliver success |

### What's NOT Needed

| Removed Key | Reason |
|-------------|--------|
| `goalBlockade_X_cleared` (x16) | Replaced by dynamic `pathBlockedBy` discovery |
| `reachable_cube_X`, `reachable_goal_X` | Expensive; handled by MoveTo failure |
| `goalPathCleared` | Aggregate computed from path check |
| `atGoal_DUMPSTER_ID` | No fixed dumpster - place anywhere |

### Dynamic vs Static Key Creation

```javascript
function syncToBlackboard(state) {
    const bb = state.blackboard;
    const actor = state.actors.get(state.activeActorId);
    
    // ALWAYS sync: Core state (small fixed set)
    bb.set('actorX', actor.x);
    bb.set('actorY', actor.y);
    bb.set('heldItemExists', actor.heldItem !== null);
    bb.set('heldItemId', actor.heldItem?.id ?? -1);
    
    // DYNAMICALLY sync: Only for entities we're interacting with
    // Don't pre-create atEntity_X for every cube - create on demand
    if (state.currentTargetId !== null) {
        const target = state.cubes.get(state.currentTargetId);
        if (target) {
            const dist = Math.hypot(target.x - actor.x, target.y - actor.y);
            bb.set(`atEntity_${state.currentTargetId}`, dist <= 1.8);
        }
    }
    
    // CLEAR stale dynamic keys when no longer relevant
    // (handled by action completion callbacks)
}
```

---

## Action Definitions

### Static Actions (Registered at Init)

These actions are always available and don't depend on dynamic discovery:

#### 1. Pick_Target

```javascript
{
    name: 'Pick_Target',
    preconditions: [
        { key: 'heldItemExists', Match: v => v === false },
        { key: 'atEntity_TARGET', Match: v => v === true }
        // NOTE: NO blockade preconditions! Planner discovers via MoveTo failure
    ],
    effects: [
        { key: 'heldItemId', Value: TARGET_ID },
        { key: 'heldItemExists', Value: true }
    ],
    tick: function() {
        // Pick up target cube
    }
}
```

#### 2. Deliver_Target

```javascript
{
    name: 'Deliver_Target',
    preconditions: [
        { key: 'heldItemId', Match: v => v === TARGET_ID },
        { key: 'atGoalArea', Match: v => v === true }
    ],
    effects: [
        { key: 'cubeDeliveredAtGoal', Value: true },
        { key: 'heldItemExists', Value: false }
    ],
    tick: function() {
        // Place target in goal area
    }
}
```

#### 3. Place_Held_Item (Generic)

```javascript
{
    name: 'Place_Held_Item',
    preconditions: [
        { key: 'heldItemExists', Match: v => v === true }
    ],
    effects: [
        { key: 'heldItemExists', Value: false },
        { key: 'heldItemId', Value: -1 }
    ],
    tick: function() {
        // Find nearest free cell and place held item there
        const spot = getFreeAdjacentCell(state, actor.x, actor.y);
        if (!spot) return bt.failure;
        
        // Place the cube at spot
        const cube = state.cubes.get(actor.heldItem.id);
        cube.x = spot.x;
        cube.y = spot.y;
        cube.deleted = false;
        
        // Mark this cube as "cleared" (moved from blocking position)
        state.blackboard.set(`cube_${cube.id}_cleared`, true);
        
        actor.heldItem = null;
        return bt.success;
    }
}
```

### Dynamic Actions (Generated by ActionGenerator)

These actions are created on-demand when the planner needs to clear a specific obstacle:

#### 4. ClearObstacle_X (Parametric)

```javascript
function createClearObstacleAction(state, cubeId) {
    return {
        name: `ClearObstacle_${cubeId}`,
        preconditions: [
            { key: 'heldItemExists', Match: v => v === false },
            { key: `atEntity_${cubeId}`, Match: v => v === true }
        ],
        effects: [
            // The obstacle is no longer blocking the path
            { key: `cube_${cubeId}_cleared`, Value: true },
            // Side effects
            { key: 'heldItemId', Value: cubeId },
            { key: 'heldItemExists', Value: true }
        ],
        tick: function() {
            const cube = state.cubes.get(cubeId);
            if (!cube || cube.deleted) return bt.success;
            
            cube.deleted = true;
            actor.heldItem = { id: cubeId };
            
            // Now we need to place it somewhere
            // This triggers Place_Held_Item as next action
            return bt.success;
        }
    };
}
```

#### 5. MoveTo_Entity_X (Parametric)

```javascript
function createMoveToEntityAction(state, entityId) {
    const entity = state.cubes.get(entityId) || state.goals.get(entityId);
    
    return {
        name: `MoveTo_${entityId}`,
        preconditions: [
            // NO reachability precondition - handled by failure + replanning
        ],
        effects: [
            { key: `atEntity_${entityId}`, Value: true }
        ],
        tick: function() {
            // Use blocker-discovering pathfinding
            const blockerResult = findBlockerOnPath(
                state, actor.x, actor.y, entity.x, entity.y
            );
            
            if (blockerResult.blocked) {
                state.blackboard.set('pathBlockedBy', blockerResult.blockerId);
                return bt.failure; // Triggers ClearObstacle via ActionGenerator
            }
            
            // ... execute movement step ...
        }
    };
}
```

---

## ActionGenerator Logic

The ActionGenerator is the **key bridge** between dynamic discovery and PA-BT planning.

### Core Logic

```javascript
state.pabtState.setActionGenerator(function(failedCondition) {
    const actions = [];
    const key = failedCondition.key;
    const targetValue = failedCondition.value;
    
    log.debug('ActionGenerator called', { key, targetValue });
    
    // ─────────────────────────────────────────────────────────────────
    // CASE 1: Planner needs actor at a specific entity
    // Generate MoveTo action for that entity
    // ─────────────────────────────────────────────────────────────────
    if (key?.startsWith('atEntity_')) {
        const entityId = parseInt(key.replace('atEntity_', ''), 10);
        if (!isNaN(entityId)) {
            actions.push(createMoveToEntityAction(state, entityId));
        }
    }
    
    // ─────────────────────────────────────────────────────────────────
    // CASE 2: Planner needs a specific cube cleared
    // Generate ClearObstacle action for that cube
    // ─────────────────────────────────────────────────────────────────
    if (key?.startsWith('cube_') && key.endsWith('_cleared')) {
        const match = key.match(/cube_(\d+)_cleared/);
        if (match) {
            const cubeId = parseInt(match[1], 10);
            actions.push(createClearObstacleAction(state, cubeId));
        }
    }
    
    // ─────────────────────────────────────────────────────────────────
    // CASE 3: Planner needs heldItemExists=false (hands free)
    // Return Place_Held_Item (already registered statically)
    // ─────────────────────────────────────────────────────────────────
    if (key === 'heldItemExists' && targetValue === false) {
        const placeAction = state.pabtState.GetAction('Place_Held_Item');
        if (placeAction) actions.push(placeAction);
    }
    
    // ─────────────────────────────────────────────────────────────────
    // CASE 4: MoveTo returned failure and set pathBlockedBy
    // This triggers the DYNAMIC DISCOVERY pattern
    // ─────────────────────────────────────────────────────────────────
    // When MoveTo fails, it sets pathBlockedBy in the blackboard.
    // The replanning will eventually need cube_X_cleared=true.
    // At that point, CASE 2 above generates ClearObstacle_X.
    //
    // The key insight: we DON'T need to check pathBlockedBy here.
    // The failure cascade naturally leads to the right condition.
    
    return actions;
});
```

### How Discovery Flows

```
1. Goal: cubeDeliveredAtGoal=true
   ↓ Planner selects Deliver_Target
   
2. Precondition: atGoal=true (FAILED)
   ↓ ActionGenerator returns MoveTo_Goal
   
3. MoveTo_Goal.tick() runs
   ↓ Path blocked by cube 105
   ↓ Sets pathBlockedBy=105
   ↓ Returns bt.failure
   
4. Replanning triggered
   ↓ Planner rechecks atGoal=true
   ↓ MoveTo_Goal still available, but WHY did it fail?
   
5. INSIGHT: MoveTo needs cube_105_cleared=true
   (This is implicit in the failure - we make it explicit)
   
6. Precondition: cube_105_cleared=true (FAILED)
   ↓ ActionGenerator returns ClearObstacle_105
   
7. ClearObstacle_105.preconditions:
   - heldItemExists=false ✓
   - atEntity_105=false (FAILED)
   
8. ActionGenerator returns MoveTo_105
   
9. Final plan: [MoveTo_105, ClearObstacle_105, Place_Held_Item, MoveTo_Goal, Deliver_Target]
```

---

## Planning Sequence Example

### Scenario

- Actor at (5, 11)
- Target cube at (45, 11) inside a room
- Goal area at (8, 18) surrounded by a ring of obstacles (IDs 100-115)
- Actor must: enter room → pick target → clear path → deliver

### Step-by-Step Execution

```
═══════════════════════════════════════════════════════════════════════════
TICK 0: Initial Planning
═══════════════════════════════════════════════════════════════════════════

Blackboard State:
  actorX=5, actorY=11
  heldItemExists=false
  cubeDeliveredAtGoal=false

Goal Check: cubeDeliveredAtGoal=true? → NO

Planner Expansion:
  Goal: cubeDeliveredAtGoal=true
    ← Effect of Deliver_Target
    
  Deliver_Target.preconditions:
    - heldItemId=TARGET_ID → FALSE
    - atGoalArea=true → FALSE
    
  Expand heldItemId=TARGET_ID:
    ← Effect of Pick_Target
    
  Pick_Target.preconditions:
    - heldItemExists=false → TRUE ✓
    - atEntity_TARGET=true → FALSE
    
  Expand atEntity_TARGET=true:
    ← ActionGenerator creates MoveTo_TARGET

Plan: [MoveTo_TARGET, Pick_Target, MoveTo_Goal, Deliver_Target]

═══════════════════════════════════════════════════════════════════════════
TICK 1-100: MoveTo_TARGET (Running)
═══════════════════════════════════════════════════════════════════════════

Actor moves from (5,11) toward (45,11)
Path is clear (room entrance at ROOM_GAP_Y)
Actor reaches (44,11) - adjacent to target

═══════════════════════════════════════════════════════════════════════════
TICK 101: Pick_Target (Success)
═══════════════════════════════════════════════════════════════════════════

Blackboard State:
  heldItemExists=true
  heldItemId=TARGET_ID

═══════════════════════════════════════════════════════════════════════════
TICK 102: MoveTo_Goal (Failure!)
═══════════════════════════════════════════════════════════════════════════

Actor at (44,11), goal at (8,18)
findBlockerOnPath() discovers: cube 100 at (6,16) blocks path

Blackboard State:
  pathBlockedBy=100
  pathBlockedAt_x=6, pathBlockedAt_y=16

MoveTo returns bt.failure → REPLAN

═══════════════════════════════════════════════════════════════════════════
TICK 103: Replanning with Blocker Knowledge
═══════════════════════════════════════════════════════════════════════════

Planner re-expands from Deliver_Target:
  - heldItemId=TARGET_ID → TRUE ✓
  - atGoalArea=true → FALSE
  
Expand atGoalArea=true:
  ← ActionGenerator creates MoveTo_Goal
  
MoveTo_Goal has implicit precondition:
  cube_100_cleared=true → FALSE
  
Expand cube_100_cleared=true:
  ← ActionGenerator creates ClearObstacle_100
  
ClearObstacle_100.preconditions:
  - heldItemExists=false → FALSE (holding target!)
  - atEntity_100=true → FALSE
  
Expand heldItemExists=false:
  ← ActionGenerator returns Place_Held_Item
  
NEW Plan: [Place_Held_Item, MoveTo_100, ClearObstacle_100, Place_Held_Item,
           MoveTo_TARGET, Pick_Target, MoveTo_Goal, Deliver_Target]

═══════════════════════════════════════════════════════════════════════════
TICK 104: Place_Held_Item (Success)
═══════════════════════════════════════════════════════════════════════════

Actor places target at nearby free cell (43,11)
Hands now free!

═══════════════════════════════════════════════════════════════════════════
TICK 105-120: MoveTo_100 (Running)
═══════════════════════════════════════════════════════════════════════════

Actor moves toward blocker 100 at (6,16)

═══════════════════════════════════════════════════════════════════════════
TICK 121: ClearObstacle_100 (Success)
═══════════════════════════════════════════════════════════════════════════

Actor picks up cube 100
Blackboard: heldItemExists=true, heldItemId=100

═══════════════════════════════════════════════════════════════════════════
TICK 122: Place_Held_Item (Success)
═══════════════════════════════════════════════════════════════════════════

Actor places cube 100 at nearest free cell (5,16)
Blackboard: cube_100_cleared=true

═══════════════════════════════════════════════════════════════════════════
TICK 123: MoveTo_Goal (Failure Again!)
═══════════════════════════════════════════════════════════════════════════

Still blocked! findBlockerOnPath() discovers cube 101 at (7,16)
Blackboard: pathBlockedBy=101

REPLAN → Similar sequence for cube 101...

═══════════════════════════════════════════════════════════════════════════
... After clearing all blocking cubes ...
═══════════════════════════════════════════════════════════════════════════

TICK 500: MoveTo_Goal (Success!)
Path is now clear. Actor reaches goal area.

TICK 501: Deliver_Target (Success!)
Target delivered. cubeDeliveredAtGoal=true

═══════════════════════════════════════════════════════════════════════════
GOAL ACHIEVED
═══════════════════════════════════════════════════════════════════════════
```

---

## Key Design Decisions

### 1. Why No God-Preconditions?

**God-precondition anti-pattern:**
```javascript
// BAD: Pick_Target lists ALL blockades as preconditions
Pick_Target.preconditions = ['blockade_100_cleared', 'blockade_101_cleared', ...];
```

**Problems:**
- Couples action to specific obstacle layout
- Planner has no choice - must clear ALL obstacles before ANYTHING
- Cannot adapt if obstacles change

**Better approach:**
- Pick_Target has NO obstacle preconditions
- MoveTo_Goal DISCOVERS obstacles dynamically
- Planner learns about obstacles through failure → replan loop

### 2. Why Place "Anywhere" Instead of Fixed Dumpster?

**Fixed dumpster approach:**
- Requires MoveTo_Dumpster for every obstacle
- Dumpster might be blocked too!
- Adds unnecessary travel distance

**Dynamic placement approach:**
- Place at ANY free adjacent cell
- Obstacle is cleared as long as it's not blocking the path
- Much faster execution

**Caveat:** If the free cell is STILL in the path, the obstacle isn't really cleared. 
Solution: `getFreeAdjacentCell` should prefer cells AWAY from the goal path.

### 3. How Does Planner Discover Obstacles Without Preconditions?

The magic happens through the **failure → blackboard → replan** loop:

1. **MoveTo fails** because path is blocked
2. **MoveTo sets** `pathBlockedBy=X` in blackboard
3. **Replanning** occurs
4. **ActionGenerator** checks blackboard, sees blocker
5. **ActionGenerator returns** `ClearObstacle_X`
6. **Planner incorporates** ClearObstacle_X into plan

Key insight: The blackboard acts as **implicit precondition state** that the ActionGenerator interprets.

### 4. Avoiding Infinite Replanning

**Risk:** If ClearObstacle always fails, we get infinite replan loop.

**Mitigations:**
1. **Track cleared cubes:** `cube_X_cleared=true` persists after clearing
2. **Hard replan limit:** Max 50 replans per goal attempt
3. **Monotonic progress:** Each successful clear removes one obstacle

### 5. Efficiency: Lazy vs Eager Blackboard Sync

**Eager (current):**
```javascript
// Every tick, compute reachability for ALL entities
entities.forEach(e => bb.set(`reachable_${e.id}`, isReachable(e)));
```

**Lazy (proposed):**
```javascript
// Only compute reachability WHEN NEEDED (in MoveTo tick function)
// Only sync atEntity for CURRENT target
```

**Benefit:** O(1) sync per tick instead of O(n) where n = entity count

---

## Summary

| Aspect | Hardcoded (Wrong) | Dynamic (Correct) |
|--------|-------------------|-------------------|
| Obstacle IDs | Pre-defined array | Discovered via pathfinding |
| Blackboard keys | 16+ blockade-specific | ~5 generic keys |
| Preconditions | God-precondition lists all | No obstacle preconditions |
| Dumpster | Fixed location | Place anywhere |
| Action count | O(n) per obstacle type | O(1) parametric |
| Adaptability | Breaks if layout changes | Works for any layout |

The dynamic approach respects the PA-BT paradigm: **declare effects, not sequences**, and let the planner discover the solution through reactive failure handling.
