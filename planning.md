# PA-BT Pick-and-Place Planning Document

**Created:** 2026-01-22  
**Status:** ACTIVE - Implementation pending  

---

## 1. Initial Concrete Plan

### The Problem

The current `scripts/example-05-pick-and-place.js` implementation **fundamentally violates PA-BT principles** by:

1. **Hardcoding obstacle knowledge**: `GOAL_BLOCKADE_IDS = [100..115]`
2. **Pre-defined disposal location**: `DUMPSTER_ID` at (8,4)
3. **Static geometry detection**: `isInBlockadeRing(x, y)` hardcodes the ring structure
4. **God-Precondition anti-pattern**: `Pick_Target` lists ALL 16 blockade clearances as explicit preconditions
5. **Forced obstacle routing**: `Deposit_Blockade_X` requires moving to dumpster

### The Correct Behavior (per Hana's directive)

The agent MUST NOT bake in ANY knowledge of the "blockade". The implementation MUST:

1. **Navigate to goal item** to pick it up
2. **If blocked, replan**: 
   - Place goal item down (if holding)
   - Move ONE blocking object out of the way to an **arbitrary, low-effort location** (NOT a pre-defined bin)
3. **Return to goal item**, pick it up, deliver to goal
4. **Be RESILIENT** to environmental changes (objects appearing/disappearing unexpectedly)

### Architecture Changes Required

| Remove | Add |
|--------|-----|
| `GOAL_BLOCKADE_IDS` array | Dynamic blocker discovery via `findFirstBlocker()` |
| `DUMPSTER_ID` and dumpster goal | Place obstacles at any free adjacent cell |
| `isInBlockadeRing()` function | No geometry-specific code |
| `goalBlockade_X_cleared` blackboard keys | `reachable_target` computed dynamically |
| `Deposit_Blockade_X` actions (16) | Generic `ClearObstacle` action |
| God-preconditions on `Pick_Target` | Minimal preconditions: `heldItemExists=false`, `atEntity_TARGET` |

### Core Implementation Tasks

1. **Add `findFirstBlocker()` function** - BFS that returns ID of first blocking cube
2. **Remove hardcoded blockade tracking** - Delete GOAL_BLOCKADE_IDS, DUMPSTER_ID, isInBlockadeRing
3. **Refactor actions**:
   - `Pick_Target`: Only require `heldItemExists=false` and `atEntity_TARGET`
   - `ClearObstacle_X`: Generic action generated dynamically for any blocking cube
   - `Place_Anywhere`: Generic drop action at nearest free cell
4. **Refactor ActionGenerator**:
   - When `atEntity_X` fails and path is blocked, call `findFirstBlocker()`
   - Generate `ClearObstacle_<blockerId>` action dynamically
5. **Handle replanning cycle**:
   - MoveTo returns `bt.failure` when blocked
   - PA-BT replans, ActionGenerator provides ClearObstacle action
   - After clearing, replanning naturally retries MoveTo

---

## 2. Subagent Review 1: PA-BT Principles

**Focus**: Core PA-BT theory and principle violations

### Findings

#### Four Core PA-BT Violations Identified

1. **God-Precondition Anti-Pattern**
   - Current: `Pick_Target` has 16 explicit blockade preconditions
   - Problem: Developer encodes the solution, bypasses planner
   - Fix: Remove all blockade preconditions; let planner discover obstacles dynamically

2. **Hardcoded Obstacle Registry**
   - Current: `GOAL_BLOCKADE_IDS = [100, 101, ..., 115]`
   - Problem: Non-generalizable, requires code changes for different layouts
   - Fix: Treat all cubes uniformly; discover blockers via pathfinding

3. **Pre-defined Dumpster Location**
   - Current: `DUMPSTER_ID` at (8,4) where obstacles MUST go
   - Problem: Violates PA-BT's "minimal commitment" principle
   - Fix: Place obstacles at any nearby free cell

4. **Non-Truthful Effects**
   - Current: `goalBlockade_X_cleared` implies knowledge of "cleared" state
   - Problem: Effect doesn't represent actual world change (position)
   - Fix: Track only observable state changes (cube positions, reachability)

#### Architectural Requirements

- **Dynamic Path-Obstacle Queries**: Replace static registry with runtime discovery
- **Free Location Finder**: Generic `getFreeAdjacentCell()` for placement
- **Minimal Preconditions**: Actions only require physically necessary conditions
- **Reactive Replanning**: Let failures trigger discovery and recovery

#### ActionGenerator Recommendations

```javascript
// CORRECT: Lazy expansion - generate actions on-demand
actionGenerator(failedCondition) {
    if (failedCondition.key.startsWith('atEntity_')) {
        const targetId = extractId(failedCondition.key);
        const blockerId = findFirstBlocker(state, actor.x, actor.y, target.x, target.y);
        if (blockerId !== null) {
            return [createClearObstacleAction(state, blockerId)];
        }
    }
    // ... other condition handlers
}
```

---

## 3. Subagent Review 2: Dynamic Obstacle Detection Design

**Focus**: Technical design for dynamic blocker discovery

### Pathfinding with Blocker Discovery

```typescript
interface BlockerResult {
    blocked: boolean;
    blockerId: number | null;
    blockerPosition: {x: number, y: number} | null;
}

function findBlockerOnPath(state, fromX, fromY, toX, toY): BlockerResult
```

**Algorithm**:
1. Run BFS from actor to target
2. Track frontier cells (cells tried but blocked)
3. If path not found, scan frontier for nearest movable cube
4. Return that cube's ID

### Blackboard State Design (Simplified)

| Key | Type | Purpose |
|-----|------|---------|
| `actorX`, `actorY` | number | Actor position |
| `heldItemExists` | boolean | Whether holding something |
| `heldItemId` | number | ID of held item (-1 if none) |
| `atEntity_X` | boolean | Proximity to entity X |
| `atGoal_X` | boolean | Proximity to goal X |
| `cubeDeliveredAtGoal` | boolean | Win condition |

**Removed**: All `goalBlockade_X_cleared` keys, `isInBlockadeRing`, `DUMPSTER_ID`

### Action Definitions

**Static Actions** (registered at init):
- `Pick_Target`: Pre `[heldItemExists=false, atEntity_TARGET]`, Eff `[heldItemId=TARGET, heldItemExists=true]`
- `Deliver_Target`: Pre `[heldItemId=TARGET, atGoal_GOAL]`, Eff `[cubeDeliveredAtGoal=true]`
- `Place_Held_Item`: Pre `[heldItemExists=true]`, Eff `[heldItemExists=false, heldItemId=-1]`

**Parametric Actions** (generated dynamically):
- `MoveTo_Entity_X`: Generated for any entity when `atEntity_X` fails
- `MoveTo_Goal_X`: Generated for any goal when `atGoal_X` fails
- `ClearObstacle_X`: Generated when MoveTo fails due to blocker X

### Planning Sequence Example

```
TICK 0: Goal = cubeDeliveredAtGoal
  → Need: Deliver_Target (requires heldItemId=TARGET, atGoal_GOAL)
  → Need: Pick_Target (requires atEntity_TARGET, heldItemExists=false)
  → Need: MoveTo_Entity_TARGET

TICK 1-50: MoveTo executing, moving toward TARGET at (45,11)
  → Actor moves from (5,11) toward (45,11)
  → Enters room through gap at (20, 10-12)
  → Reaches TARGET, picks it up

TICK 51: Now holding TARGET, need atGoal_GOAL
  → MoveTo_Goal_GOAL executing
  → Path to goal (8,18) is BLOCKED by cubes in ring

TICK 52: MoveTo returns bt.failure (path blocked)
  → Replanning triggered
  → ActionGenerator called with failedKey='atGoal_GOAL'
  → findFirstBlocker() returns cube ID 106 (nearest blocker)
  → ClearObstacle_106 action generated

TICK 53-60: ClearObstacle_106 executing
  → Need heldItemExists=false → Place_Held_Item (places TARGET at (7,17))
  → Need atEntity_106 → MoveTo_Entity_106
  → Pick cube 106
  → Place at (6,17) (nearest free cell)

TICK 61: ClearObstacle complete, retry atGoal_GOAL
  → Path may still be blocked → repeat clear cycle
  → Eventually path clears, deliver TARGET
```

---

## 4. Subagent Review 3: Implementation Feasibility

**Focus**: Practical implementation concerns and risks

### Pathfinding Extension

**Approach**: Extend BFS to track blocked frontier cells
```javascript
function findFirstBlocker(state, fromX, fromY, toX, toY) {
    // Phase 1: BFS to determine if path exists
    // Phase 2: If blocked, find nearest movable cube on frontier
    // Returns: cubeId or null
}
```

**Reuse**: Can build on `buildBlockedSet()` but need position→cubeId mapping

### Action Design Recommendation

**Hybrid "Bridge Action" pattern**:
- High-level `ClearObstacle_X` action visible to planner
- Internally decomposes to: Place (if holding) → MoveTo → Pick → MoveTo (free spot) → Place

**"Place anywhere" handling**:
- Use existing `getFreeAdjacentCell(state, actorX, actorY, false)`
- Add heuristic to avoid placing back on the path being cleared

### Replanning Trigger

**Key insight**: MoveTo returns `bt.failure` when pathfinding fails

**Mechanism**:
1. MoveTo tick function calls `findNextStep()`
2. If `findNextStep()` returns null → `return bt.failure`
3. PA-BT detects failure → triggers replanning
4. ActionGenerator called with `atEntity_X` or `atGoal_X` as failed condition
5. ActionGenerator checks `findFirstBlocker()` → generates `ClearObstacle_X`

### State Tracking (Simplified)

**Key insight**: Don't track "cleared" status; track reachability dynamically

- Remove: `goalBlockade_X_cleared` (16 keys)
- Add: Nothing! Reachability computed on-demand via pathfinding

### Risk Analysis

| Risk | Severity | Mitigation |
|------|----------|------------|
| Infinite clear loop | HIGH | Clear only first blocker; let replanning find next |
| Performance (pathfinding in ActionGenerator) | MEDIUM | Cache reachability per tick; memoize |
| All paths blocked (walls) | LOW | `findFirstBlocker()` only returns movable cubes |
| Actor boxed in | LOW | Place action already handles no-space case |

### Testing Approach

1. **Unit tests**: `findFirstBlocker()` with various layouts
2. **Integration tests**: Single obstacle clearing sequence
3. **Full scenario test**: Complete ring clearing → goal delivery
4. **Regression tests**: Existing PA-BT tests must pass
5. **Stress tests**: Randomized obstacle layouts

---

## 5. Synthesized Implementation Plan

Based on the three reviews, here is the prioritized implementation order:

### Phase 1: Infrastructure (Prerequisites)

1. **Add `findFirstBlocker()` function** (new pathfinding helper)
2. **Remove hardcoded constants**: `GOAL_BLOCKADE_IDS`, `DUMPSTER_ID`, `isInBlockadeRing()`
3. **Remove `goalBlockade_X_cleared` blackboard keys** from syncToBlackboard

### Phase 2: Action Refactoring

4. **Refactor `Pick_Target`**: Remove ALL blockade preconditions
5. **Remove `Deposit_Blockade_X` actions** (16 registrations)
6. **Add generic `ClearObstacle` action generation** in ActionGenerator
7. **Modify `Place_Held_Item`**: Allow placing any item anywhere (remove blockade restriction)

### Phase 3: ActionGenerator Overhaul

8. **Refactor ActionGenerator for dynamic discovery**:
   - When `atEntity_X` or `atGoal_X` fails, check if path is blocked
   - Call `findFirstBlocker()` to identify blocker
   - Generate `ClearObstacle_<blockerId>` action

### Phase 4: Testing & Verification

9. **Update tests** to reflect new behavior
10. **Run full test suite** - must pass 100%
11. **Verify with manual testing** - visual confirmation

---

## 6. Next Steps

1. Update `blueprint.json` with EXHAUSTIVE task list
2. Begin implementation starting with Phase 1
3. Test after each phase before proceeding
4. Update `WIP.md` continuously

---

## References

- `./review.md` - Master analysis document
- `./docs/design/dynamic-obstacle-detection.md` - Dynamic detection design
- `./docs/design/pick-and-place-clearpath-feasibility.md` - Feasibility analysis
