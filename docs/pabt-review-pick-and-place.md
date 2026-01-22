# PA-BT Principles Review: Pick-and-Place Implementation

## Executive Summary

The current pick-and-place implementation exhibits **fundamental violations** of PA-BT (Precondition-Action Behavior Tree) principles. The design encodes static, global knowledge into preconditions rather than allowing the planner to dynamically discover and resolve obstacles. This review identifies the core violations and provides architectural recommendations.

---

## 1. Core PA-BT Principles Being Violated

### 1.1 Violation: "God-Precondition" Anti-Pattern

**The Problem:**
`Pick_Target` lists ALL 16 blockade clearances as explicit preconditions:
```
Preconditions: [blockade_0_cleared, blockade_1_cleared, ..., blockade_15_cleared]
```

**Why This Violates PA-BT:**

PA-BT requires preconditions to represent **necessary physical constraints** for action execution—not omniscient world-state requirements. The agent holding the target cube doesn't *physically* require that blockade_7 be cleared; it only requires:
- Agent is at target location
- Agent hand is empty
- Target is graspable

The "16 blockades must be cleared" is a **planning artifact** that the backward-chaining planner should *discover*, not a truth about the Pick action itself.

**Correct Precondition Set for `Pick_Target`:**
```
Preconditions:
  - agent_at(target_location)
  - hand_empty
  - target.graspable
```

That's it. The path-blocking discovery is the planner's job.

### 1.2 Violation: Hardcoded Obstacle Registry (GOAL_BLOCKADE_IDS)

**The Problem:**
The system maintains `GOAL_BLOCKADE_IDS = [0, 1, 2, ..., 15]`—a static array encoding which objects are "blockades."

**Why This Violates PA-BT:**

PA-BT systems must be **reactive to world state**, not dependent on pre-enumerated object classifications. Consider:
- What if blockade_7 is removed by an external actor?
- What if a NEW obstacle (not in the array) appears?
- What if the target itself becomes an obstacle to another goal?

The system becomes **brittle** and unable to handle environmental perturbations.

**PA-BT Principle:** Objects are just objects. Their role as "obstacle" is determined **at planning time** based on geometric/logical interference with the current goal path.

### 1.3 Violation: Pre-Defined "Dumpster" Location

**The Problem:**
Obstacles are moved to a specific `DUMPSTER_LOCATION`—a hardcoded coordinate.

**Why This Violates PA-BT:**

This violates the principle of **minimal commitment**. The planner should not care *where* an obstacle goes—only that it's *no longer blocking*. Forcing a specific destination:
1. Creates artificial constraints (dumpster must be reachable)
2. May cause new blockages (path to dumpster blocked)
3. Adds unnecessary action complexity

**PA-BT Principle:** Place actions should target "any valid free location" with a preference heuristic (e.g., nearest free spot, minimum energy).

### 1.4 Violation: Non-Truthful Effects

**The Problem:**
If clearing blockade_5 implicitly depends on clearing blockade_3 first (to reach blockade_5), this dependency is not represented in the action's preconditions/effects.

**PA-BT "Truthful Effects" Requirement:**

An action's declared effects must **actually occur** when preconditions are met and the action executes. If `ClearBlockade(5)` declares effect `blockade_5_cleared` but fails because blockade_3 is in the way, the effect was **not truthful**.

**Correct Approach:**
- `ClearBlockade(X)` should have precondition: `path_clear(agent, X)`
- The planner then recursively plans to achieve `path_clear(agent, X)` if blocked

---

## 2. Required Architectural Changes

### 2.1 Remove Static Obstacle Registry

**Before:**
```javascript
const GOAL_BLOCKADE_IDS = [0, 1, 2, ..., 15];
```

**After:**
No registry. Obstacles are discovered dynamically via path queries:
```javascript
function getBlockingObjects(from, to, worldState) {
  return worldState.objects.filter(obj => 
    obj.blocksPath(from, to) && obj.isMovable
  );
}
```

### 2.2 Replace Dumpster with "Free Location" Query

**Before:**
```javascript
moveTo(DUMPSTER_LOCATION);
place(obstacle);
```

**After:**
```javascript
const freeSpot = findNearestFreeLocation(agent.position, worldState);
moveTo(freeSpot);
place(obstacle);
```

The `findNearestFreeLocation` function returns any unoccupied, reachable location—preferring low-cost positions.

### 2.3 Minimal Preconditions for Core Actions

| Action | Correct Preconditions | Incorrect Preconditions |
|--------|----------------------|------------------------|
| `Pick(obj)` | `at(obj.location)`, `hand_empty`, `obj.graspable` | Any blockade clearance states |
| `Place(obj, loc)` | `holding(obj)`, `at(loc)`, `loc.unoccupied` | Specific destination requirements |
| `MoveTo(loc)` | `path_clear(current, loc)` OR triggers replanning | None (should fail if blocked) |

### 2.4 Introduce Path-Checking Precondition

Navigation actions must have a **dynamic precondition** that checks path viability:

```javascript
Action: MoveTo(destination)
Preconditions:
  - path_exists(agent.location, destination)  // DYNAMIC CHECK
Effects:
  - agent_at(destination)
```

When `path_exists` fails, the planner's backward-chaining should identify:
1. What object(s) block the path
2. What actions can achieve path clearance (move the blocking object)

---

## 3. ActionGenerator Recommendations

### 3.1 Dynamic Action Generation for Path-Clearing

The ActionGenerator should **not** pre-enumerate all possible obstacle-clearing actions. Instead:

```javascript
function generateClearPathActions(goalState, worldState) {
  const path = computePath(agent.location, goalState.targetLocation);
  
  if (path.blocked) {
    const blockingObj = path.firstBlockingObject;
    const freeSpots = findFreeLocations(worldState);
    
    // Generate ONE action per viable relocation spot (or just the best one)
    return freeSpots.slice(0, 3).map(spot => ({
      action: 'Relocate',
      object: blockingObj,
      destination: spot,
      preconditions: [
        `holding(${blockingObj.id})`,
        `at(${spot})`,
        `${spot}.unoccupied`
      ],
      effects: [
        `path_segment_clear(${path.blockedSegment})`
      ]
    }));
  }
  
  return [];
}
```

### 3.2 Lazy Expansion Principle

Do NOT generate all 16 × N relocation actions upfront. Generate actions **on demand** as the planner explores:

1. Planner needs: `agent_at(target_location)`
2. Discovers: path is blocked by `obstacle_3`
3. Generates: `Relocate(obstacle_3, free_spot_A)` (just-in-time)
4. Plans preconditions of Relocate (may discover more blocks)

### 3.3 The "Bridge Action" Pattern

A **Bridge Action** is an action whose primary purpose is to enable another action—not to achieve a goal directly.

**When to use Bridge Actions:**
- When the direct path to goal is obstructed
- When a precondition cannot be satisfied in the current world state
- When temporary state changes are needed to enable goal achievement

**Example Bridge Action for Path-Clearing:**

```javascript
BridgeAction: RelocateBlockingObject
Trigger: path_blocked(A, B)
Steps:
  1. If holding something: Place it at nearest free spot (preserve it)
  2. Pick up blocking object
  3. Place blocking object at any free location NOT on path(A, B)
  4. Return to original task
  
This is NOT a goal—it's an enabler.
```

**Critical:** Bridge Actions should be **automatically synthesized** by the planner when forward progress is blocked, NOT hardcoded as explicit goal preconditions.

---

## 4. Dynamic Obstacle Discovery Implementation

### 4.1 World State Query API

Replace static obstacle knowledge with dynamic queries:

```javascript
class WorldState {
  // Query: What blocks path from A to B?
  getPathObstacles(from, to) {
    return this.objects.filter(obj => 
      this.pathIntersects(from, to, obj.bounds)
    );
  }
  
  // Query: Is this location free for placement?
  isFreeLocation(loc) {
    return !this.objects.some(obj => obj.occupies(loc));
  }
  
  // Query: Find N nearest free locations from reference point
  findFreeLocations(reference, count = 5) {
    return this.grid
      .filter(cell => this.isFreeLocation(cell))
      .sort((a, b) => distance(reference, a) - distance(reference, b))
      .slice(0, count);
  }
}
```

### 4.2 Reactive Replanning Trigger

When world state changes unexpectedly:

```javascript
function onWorldStateChange(change) {
  if (change.type === 'OBJECT_APPEARED' || change.type === 'OBJECT_MOVED') {
    // Check if change affects current plan
    const currentPlan = planner.getCurrentPlan();
    const affectedActions = currentPlan.filter(action => 
      action.dependsOn(change.affectedRegion)
    );
    
    if (affectedActions.length > 0) {
      // Invalidate and replan from current state
      planner.replanFrom(getCurrentWorldState());
    }
  }
}
```

### 4.3 Planning Loop with Dynamic Discovery

```javascript
function planToGoal(goal, worldState) {
  const openGoals = [goal];
  const plan = [];
  
  while (openGoals.length > 0) {
    const currentGoal = openGoals.pop();
    
    // Find action that achieves currentGoal
    const candidateActions = actionGenerator.getActionsFor(currentGoal, worldState);
    
    for (const action of candidateActions) {
      // Check preconditions - THIS IS WHERE DISCOVERY HAPPENS
      const unmetPreconditions = action.preconditions.filter(p => 
        !worldState.satisfies(p)
      );
      
      if (unmetPreconditions.length === 0) {
        plan.push(action);
      } else {
        // Preconditions become subgoals - DYNAMIC CHAINING
        openGoals.push(...unmetPreconditions);
        plan.push(action);
      }
    }
  }
  
  return plan.reverse(); // Execute in forward order
}
```

---

## 5. Summary of Required Changes

| Current Design | Required Change | PA-BT Principle |
|----------------|-----------------|-----------------|
| `GOAL_BLOCKADE_IDS` array | Dynamic `getPathObstacles()` query | Reactive World Model |
| `DUMPSTER_LOCATION` | `findNearestFreeLocation()` | Minimal Commitment |
| 16 preconditions on Pick_Target | 3 physical preconditions only | Minimal Preconditions |
| Pre-enumerated clear actions | Just-in-time action generation | Lazy Expansion |
| Static obstacle-to-dumpster plan | Bridge Actions with any-free-spot | Dynamic Bridging |
| No replanning on world change | `onWorldStateChange` trigger | Reactive Replanning |

---

## 6. Conclusion

The current implementation uses PA-BT syntax but violates PA-BT semantics. It encodes a **specific solution** (clear 16 blockades to dumpster) rather than expressing the **problem** (deliver target to goal).

A correct PA-BT implementation would:
1. Express only **true physical preconditions** for actions
2. **Discover** obstacles dynamically via path queries
3. **Generate** clearing actions on-demand with minimal commitment
4. **Replan** when the world changes unexpectedly

The agent should not "know" there are 16 blockades. It should only know: "I want to get to X. Something is in my way. I'll move it somewhere—anywhere—and try again."

---

*Document Author: Takumi*  
*Review Date: 2026-01-22*  
*Status: Theoretical Analysis Complete*
