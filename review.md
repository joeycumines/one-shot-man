# PA-BT Implementation Issues and Fixes

## 1. PA-BT Principles & Architectural Standard

**Status:** *Mandatory Compliance*
**Source:** *Behavior Trees in Robotics and AI (Colledanchise & Ögren)*

To ensure the system functions as a dynamic planner rather than a scripted state machine, the implementation must adhere strictly to the **Planning and Acting using Behavior Trees (PA-BT)** formulation.

### 1.1 The Postcondition-Precondition-Action (PPA) Unit

The fundamental atom of the system is not the Action, but the PPA expansion. The planner does not generate a linear queue of tasks; it generates a tree that satisfies conditions.

* **Structure:** `Fallback(Condition C, Sequence(Preconditions(A), A))`
* **Lazy Expansion:** The planner **must not** expand a subtree unless the Postcondition  is currently `Failure`. Expanding conditions that are already met or not yet relevant leads to computational waste and logic cycles.

### 1.2 Truthful Effects (The Anti-Livelock Rule)

For the reactive loop to function, the **Descriptive Model** (what the planner thinks an action does) must match the **Operational Model** (what the physics engine actually does).

* **Rule:** If Action  claims `Effect: X=True`, executing  must physically result in .
* **Violation:** If `MoveTo` claims success based on a distance of , but `Pick` requires a distance of , the planner enters a Livelock (believes it is at the goal, attempts to pick, fails, believes it is at the goal).

### 1.3 Conflict Resolution via Priority Inversion

When a new goal (e.g., clearing a specific obstacle) requires an action that conflicts with the current state (e.g., "Hands Empty" vs "Holding Target"):

* The planner must identify the conflict.
* The new subtree (the repair) must be prioritized **Leftward** in the sequence.
* *Example:* `Sequence(ClearPath, FetchTarget)` rather than `Sequence(FetchTarget, ClearPath)`.

---

## 2. Critical Runtime Fixes

**Status:** *High Severity Bugs*
**Consensus:** *Unanimous (Documents 1, 2, Review)*

Before architectural refactoring, the following low-level defects must be patched to prevent infinite loops and state corruption.

### 2.1 The Geometric Heuristic Mismatch

**The Issue:** A violation of **Truthful Effects**. The Blackboard reports `MoveTo` success at , but the `Deliver/Pick` actions require physical adjacency ().
**The Fix:** Align the Blackboard success condition with the strictest physical requirement of the dependent actions.

```javascript
// In syncToBlackboard AND createMoveToAction
// Recommendation: Tighten threshold to ensure physical adjacency.
// 2.5 is mathematically unsafe for grid-based interaction.
const threshold = (entityType === 'goal' && entityId === GOAL_ID) ? 1.5 : 1.8;

```

### 2.2 Disable Object Caching

**The Issue:** The implementation caches `pabt.newAction` instances. BT nodes contain internal state (`Running`, `childIndex`). Reusing node instances across different branches or ticks creates "Zombie State," where a node behaves as if it is running from a previous context.
**The Fix:**

* **Remove:** `if (actionCache.has(cacheKey)) ...`
* **Requirement:** Always instantiate fresh BT nodes and Action wrappers in the Generator. Garbage collection cost is negligible compared to correctness.

### 2.3 Infinite Loop Guard

**The Issue:** Infinite expansion depth during unsolvable scenarios.
**The Fix:** Maintain the hard cap on expansion iterations (e.g., 10,000 or 1M operations) and return a hard failure to halt the agent rather than crashing the browser.

---

## 3. Architectural Overhaul: Dynamic Obstacle Handling

**Status:** *Architectural Refactoring*
**Consensus:** *Documents 1 & 2 refute the "Atomic ClearPath" and "God-Precondition" patterns found in the legacy code.*

The system must transition from **Static Pre-calculation** to **Dynamic Discovery**.

### 3.1 The "God-Precondition" Anti-Pattern

**Current (Incorrect) Behavior:** `Pick_Target` preconditions explicitly list every potential blockade (`goalBlockade_100_cleared`, ... `115`).

* **Why it fails:** This couples the atomic action to the global map layout. It forces the planner to "solve" the map before execution begins, violating the **Lazy Expansion** principle.

### 3.2 The "Atomic Action" Anti-Pattern

**Current (Incorrect) Behavior:** `ClearPath` is a single action that handles moving to, picking up, and placing an obstacle in one tick.

* **Why it fails:** This violates PA-BT granularity. If the atomic action fails mid-execution (e.g., placement spot blocked), the planner has no visibility into the partial state. It creates a "Black Box" that prevents reactive repair.

### 3.3 The Solution: The "Bridge Action" Pattern

We must implement a **Bridge Action** that dynamically links a failure (Blocked Path) to a solution (Clear Specific Obstacle) without hardcoding map data into the `Pick` action.

#### Step 1: Purify `Pick_Target`

Remove all blockade-related preconditions. `Pick_Target` should only assert local requirements.

```javascript
// Corrected Action Template
reg('Pick_Target',
    // Preconditions
    [
        { k: 'heldItemExists', v: false }, 
        { k: 'atEntity_' + TARGET_ID, v: true } 
    ],
    // Effects
    [
        { k: 'heldItemId', v: TARGET_ID }, 
        { k: 'heldItemExists', v: true }
    ],
    function () { /* Standard Pick Logic */ }
);

```

#### Step 2: Implement "Path-to-Target" Visibility

**Critical Fix from Doc 1 & 2:** The system currently only checks blockers for the *Goal*. It must also check blockers for the *Target* before the target is picked up.

**Updated Logic:**
The Planner attempts to satisfy `atEntity_TARGET`. This expands to `MoveTo(Target)`.
If the path is blocked, `MoveTo` must fail (or a `reachable_Target` condition must fail).

#### Step 3: The Dynamic Bridge in `ActionGenerator`

The Generator intercepts the failed condition (`reachable_X` or `atEntity_X`). It runs A* locally to identify the *specific* obstructing entity ID.

```javascript
// In ActionGenerator
state.pabtState.setActionGenerator(function (failedCondition) {
    const actions = [];
    const key = failedCondition.key;

    // Trigger: Planner cannot reach a destination (Target OR Goal)
    if (key.startsWith('reachable_') || key.startsWith('atEntity_')) {
        const targetId = extractId(key); 
        
        // 1. Run A* to find the FIRST blocking entity on the path
        const blockerId = findFirstBlockerOnPath(state, actor, targetId);
        
        if (blockerId !== -1) {
            // 2. Generate the Bridge Action
            // Name: ClearPathTo_<target>_via_<blocker>
            const actionName = `ClearBlocker_${blockerId}_For_${targetId}`;
            
            // The Bridge Action does NOT execute logic. 
            // It simply asserts: "If Blocker X is cleared, Path is reachable."
            const dummyTick = function () { return bt.success; };
            const node = bt.createLeafNode(dummyTick);
            
            actions.push(pabt.newAction(
                actionName,
                // PRECONDITION: The specific blocker must be cleared
                [{ key: `goalBlockade_${blockerId}_cleared`, Match: v => v === true }], 
                // EFFECT: The path is now considered reachable
                [{ key: key, Value: true }], 
                node
            ));
        }
    }
    return actions;
});

```

#### Step 4: Recursive Resolution

Once the Bridge Action is inserted, the planner sees a new unmet precondition: `goalBlockade_123_cleared`.

* The Generator now offers a **standard** `Pick` and `Place` sequence to satisfy this.
* This decomposes the "Atomic ClearPath" into discrete PA-BT nodes:
1. `Pick(Blocker)`
2. `Place(Blocker, valid_spot)`



---

## 4. Addressing Logic Gaps & Blind Spots

**Status:** *Logic Correction*
**Consensus:** *Document 2 identifies fatal flaws in "Self-Blockage" and "Silent Failure".*

### 4.1 "Self-Blockage" Blindness

**The Issue:** When the agent drops the target to clear a path, it stops calculating `pathBlocker` (because the calculation is currently gated by `if (heldItem == TARGET)`).
**The Fix:** Remove the conditional gate. The blackboard must compute path blockers for **all** relevant destinations (Goal and Target) every tick, regardless of what the agent is holding.

### 4.2 Silent Failure in `Place_Obstacle`

**The Issue:** `Place_Obstacle` secretly rejects the Target ID inside its tick function, but the Action Template does not declare this restriction.
**The Consequence:** The planner plans to use `Place_Obstacle` to move the target out of the way, but the action fails repeatedly at runtime (Livelock).
**The Fix:** 1.  **Explicit Preconditions:** If `Place_Obstacle` cannot handle the target, add a precondition: `{ k: 'heldItemIsTarget', v: false }`.
2.  **Generalize Placement:** Ideally, allow `Place_Obstacle` to place *any* item (including the target) into a safe, non-blocking zone. This requires a `FindSafeSpot` routine that checks A* connectivity before placing.

---

## 5. Summary of Implementation Checklist

To achieve internal consistency and robustness, perform these steps in order:

1. **Runtime Sanitization:**
* Set `MoveTo` success threshold to `1.5` (or consistent strict adjacency).
* Disable `actionCache`.


2. **Blackboard Logic Update:**
* Remove `if (holding target)` check for blocker detection.
* Compute `pathBlocker_to_Goal` AND `pathBlocker_to_Target` every tick using A*.


3. **Action Refactoring:**
* **Strip** `Pick_Target` of all 16 `goalBlockade` preconditions.
* **Refactor** `Place_Obstacle` to be truthful about what it can place, or generalize it to handle the Target.


4. **Generator Logic Update (The Bridge):**
* Implement the `findFirstBlockerOnPath` logic within the Generator (or access cached Blackboard result).
* Inject `ClearBlocker` Bridge Actions when `atEntity/reachable` fails.
* Ensure `ClearBlocker` preconditions link to `goalBlockade_ID_cleared`.


5. **Verify Decomposition:**
* Ensure `goalBlockade_ID_cleared` is achieved by `Deposit_Blockade` (Pick -> Place), **not** by a magical atomic function.
