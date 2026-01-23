# Simulation Consistency Test Plan
# For scripts/example-05-pick-and-place.js

**Document Version:** 1.0  
**Date:** 2026-01-23  
**Purpose:** Design comprehensive integration tests to verify identical physics, collision detection, and pathfinding behavior across manual and automatic modes

---

## Executive Summary

This test plan defines **T6-Physics**, **T7-Collision**, and **T8-Pathfinding** test suites to guarantee that the simulation produces **identical results** regardless of whether the player is in manual mode or the PA-BT planner is in automatic mode. These tests are critical because they verify the **single source of truth** for simulation mechanics.

**Key Principle:** Manual mode and automatic mode MUST use the SAME underlying physics engine, collision detection, and pathfinding algorithms. Any divergence is a **critical bug**.

---

## Test Architecture Overview

### Test Organization

- **File Location:** `internal/builtin/bubbletea/simulation_consistency_test.go` (NEW FILE)
- **Base Test Function:** `setupConsistencyTest()` - creates identical initial state for both modes
- **Mode Parallel Execution:** Run identical inputs in parallel on two state instances (manual/auto) and compare results
- **Helper Functions:** Re-use existing helpers from `manual_mode_comprehensive_test.go`

### Test State Structure

```javascript
// State Instance A (Manual Mode)
{
  gameMode: "manual",
  // Identical initial configuration
}

// State Instance B (Automatic Mode)
{
  gameMode: "automatic",
  // Identical initial configuration
}
```

### Comparison Methodology

For each test:
1. Clone initial state to create two instances (manual/auto)
2. Apply identical inputs/actions to both instances
3. Compare critical state properties:
   - Actor position (`actor.x`, `actor.y`)
   - Actor velocity/movement deltas
   - Cube positions
   - Blocked cell sets
   - Pathfinding results
   - Held item state

**Comparison Function:** `assertStatesEqual(manualState, autoState, tolerance)`

---

## Part 1: Identified Simulation Components

### 1.1 Movement/Physics Calculations

**Manual Mode:** `manualKeysMovement(state, actor)` (Lines 216-250)
```javascript
// Key physics calculations:
const {dx, dy} = getDir();  // Direction from manualKeys
const nx = actor.x + dx;      // Proposed new position
const ny = actor.y + dy;

// Boundary check: 
//   Math.round(nx) < 0 || Math.round(nx) >= state.spaceWidth
//   Math.round(ny) < 0 || Math.round(ny) >= state.height

// Collision check:
//   Iterate all cubes, check if cube deleted or held by actor
//   blocked = true if Math.round(c.x) === Math.round(nx) && same for y

// Movement application:
if (!blocked) {
  actor.x = nx;  // Unit movement per tick
  actor.y = ny;
}
```

**Automatic Mode:** `findNextStep()` + MoveTo tick function (Lines 640-700)
```javascript
// Key physics calculations (in createMoveToAction):
const nextStep = findNextStep(state, actor.x, actor.y, targetX, targetY, ignoreCubeId);
if (nextStep) {
  const stepDx = nextStep.x - actor.x;
  const stepDy = nextStep.y - actor.y;
  var newX = actor.x + Math.sign(stepDx) * Math.min(1.0, Math.abs(stepDx));
  var newY = actor.y + Math.sign(stepDy) * Math.min(1.0, Math.abs(stepDy));
  actor.x = newX;
  actor.y = newY;
}

// Uses same buildBlockedSet() for collision detection
```

**Critical Observation:** Both modes use:
- Unit movement (1.0 per tick for manual, Math.min(1.0, stepDist) for auto)
- Same boundary check logic
- Same collision detection via `buildBlockedSet()`

### 1.2 Collision Detection Logic

**Primary Function:** `buildBlockedSet(state, ignoreCubeId)` (Lines 507-530)
```javascript
function buildBlockedSet(state, ignoreCubeId) {
  const blocked = new Set();
  const key = (x, y) => x + ',' + y;
  const actor = state.actors.get(state.activeActorId);

  state.cubes.forEach(c => {
    if (c.deleted) return;  // Skip deleted cubes
    if (c.id === ignoreCubeId) return;  // Skip specified cube
    if (actor.heldItem && c.id === actor.heldItem.id) return;  // Skip held item
    blocked.add(key(Math.round(c.x), Math.round(c.y)));
  });

  return blocked;
}
```

**Boundary Checks:**
- Manual mode: Lines 233-236
- Automatic mode: `findPath()` Lines 573, `findNextStep()` Lines 687
- Both check: `nx < 1 || nx >= state.spaceWidth - 1 || ny < 1 || ny >= state.height - 1`

**Pick Threshold:** `PICK_THRESHOLD = 1.8` (Line 83)
- Used in: manual mouse pick (Lines 1644-1648), automatic pick actions
- Distance calculation: `Math.sqrt(Math.pow(x1-x2, 2) + Math.pow(y1-y2, 2))`

**Place Threshold:** `dist <= 1.5` (Lines 1456, 1662)
- Used in manual mouse place and automatic Deliver action
- Same distance formula as pick

### 1.3 Pathfinding Logic

**Function 1: getPathInfo (Line 533-559)**
```javascript
// BFS to check reachability and calculate distance
// Returns: {reachable: boolean, distance: number}
// Used for: path blocker detection in syncToBlackboard

Key algorithm:
- BFS from start position
- Stop when distance to target <= 1 (adjacent to goal)
- Return distance if reachable, Infinity otherwise
- Uses buildBlockedSet() for obstacle awareness
```

**Function 2: findPath (Line 565-604)**
```javascript
// BFS to find full path waypoints
// Returns: array of {x,y} waypoints or null if no path

Key algorithm:
- BFS from start to target cell
- Store parent pointers for path reconstruction
- Allow entering target cell even if blocked (for picking items)
- Return path excluding start, including target
// Uses buildBlockedSet() for obstacle awareness
```

**Function 3: findNextStep (Line 606-659)**
```javascript
// BFS to find next step direction only
// Returns: {x,y} coords for next position or null if unreachable

Key algorithm:
- BFS from start
- For first valid neighbor, compute firstX, firstY direction
- Return: {x: startX + firstX, y: startY + firstY}
// Uses buildBlockedSet() for obstacle awareness
```

**Function 4: findFirstBlocker (Line 661-733)**
```javascript
// Dynamic obstacle discovery for PA-BT
// Returns: ID of nearest movable cube blocking path, or null

Key algorithm:
- BFS from start to goal area
- Build cubeAtPosition map (only movable, non-static cubes)
- Collect frontier (blocked cells with movable cubes)
- Sort frontier by proximity to goal
- Return blockerId closest to goal
// Uses buildBlockedSet with excludeId parameter
```

---

## Part 2: Test Design - T6-Physics

### Test Suite: `TestSimulationConsistency_Physics_T6`

#### Test T6.1: Unit Movement Identical Vector Magnitude

**Objective:** Verify that single-tick movement produces identical position updates in both modes

**Setup:**
- Initial state: actor at (30, 12), empty field
- Manual state: press 'd' key (right direction)
- Auto state: simulate MoveTo action targeting (31, 12)

**Test Actions:**
1. Manual mode: Apply Tick message with manualKeys={'d': true}
2. Auto mode: Execute MoveTo tick with target (31, 12)

**Verification:**
```go
// Check both actors moved right by exactly 1.0
assert.Equal(t, 31.0, manualActor.x, "Manual mode actor should move right 1.0")
assert.Equal(t, 31.0, autoActor.x, "Auto mode actor should move right 1.0")
assert.Equal(t, manualActor.x, autoActor.x, "Positions should be identical")
assert.Equal(t, manualActor.y, autoActor.y, "Y positions should be identical")
```

**Helper Functions Needed:**
- `applyManualMovementTick(state, keys)` - applies tick with manualKeys set
- `simulateAutoMovementTick(state, targetX, targetY)` - executes MoveTo tick

---

#### Test T6.2: Diagonal Movement Consistency

**Objective:** Verify diagonal movement produces identical results

**Setup:**
- Initial state: actor at (30, 12), empty field
- Manual state: press 'w' and 'd' keys (up-right diagonal)
- Auto state: simulate MoveTo action targeting (31, 11)

**Test Actions:**
1. Manual mode: Apply Tick with manualKeys={'w': true, 'd': true}
2. Auto mode: Execute MoveTo tick with target (31, 11)

**Verification:**
```go
// Manual key-hold implementation moves both axes each tick
// findNextStep should also produce diagonal step if path is clear
assert.Equal(t, 31.0, manualActor.x)
assert.Equal(t, 11.0, manualActor.y)
assert.Equal(t, manualActor.x, autoActor.x, "Diagonal X positions identical")
assert.Equal(t, manualActor.y, autoActor.y, "Diagonal Y positions identical")
```

---

#### Test T6.3: Multi-Tick Movement Accumulation

**Objective:** Verify movement delta accumulates consistently over multiple ticks

**Setup:**
- Initial state: actor at (10, 10)
- Target: move actor 5 ticks right to (15, 10)

**Test Actions:**
```go
for i := 0; i < 5; i++ {
  // Manual: press 'd', Tick, release 'd'
  applyManualMovementTick(manualState, []string{"d"})
  // Auto: MoveTo toward (15, 10)
  simulateAutoMovementTick(autoState, 15, 10)
}
```

**Verification:**
```go
assertEqual(t, manualActor.x, autoActor.x, "Accumulated X positions identical")
assertEqual(t, manualActor.y, autoActor.y, "Accumulated Y positions identical")
assert.InDelta(t, 15.0, manualActor.x, 0.01, "Manual mode reached target")
assert.InDelta(t, 15.0, autoActor.x, 0.01, "Auto mode reached target")
```

---

#### Test T6.4: Non-Blocking Obstacle Detection

**Objective:** Verify actor correctly ignores held items when moving

**Setup:**
- Actor at (10, 10) holding cube ID 501
- Cube 501 currently "deleted" state (held)
- Another cube 502 at (12, 10)
- Target: move to (13, 10)

**Test Actions:**
```go
// Set up held item
heldItem := vm.NewObject()
heldItem.Set("id", 501)
actor.Set("heldItem", heldItem)

// Cube 501 marked deleted (held)
setCubeDeleted(state, 501, true)

// Cube 502 at (12, 10) - should block
addCube(vm, state, 502, 12, 10, false, "obstacle")

// Attempt movement
applyManualMovementTick(manualState, []string{"d"})
simulateAutoMovementTick(autoState, 13, 10)
```

**Verification:**
```go
// Both modes should stop at (11, 10), NOT at (10, 10)
// Held item (501) doesn't block, cube 502 blocks at (12, 10)
assert.Equal(t, 11.0, manualActor.x, "Should move past held item")
assert.Equal(t, 11.0, autoActor.x, "Should move past held item")
assert.Equal(t, manualActor.x, autoActor.x, "Blocking behavior identical")
```

---

#### Test T6.5: Movement Around Obstacle Pathing

**Objective:** Verify both modes can navigate around simple obstacles identically

**Setup:**
```
Initial: Actor @ (10, 10)
Obstacle:   @ (11, 10) - blocked cell
Goal area:  @ (15, 10)
```

**Test Actions:**
```go
// Setup obstacle
addCube(vm, state, 900, 11, 10, false, "obstacle")

// Auto mode: find path with findPath()
ignoreId := -1
path := findPath(autoState, 10, 10, 15, 10, ignoreId)

// Manual mode: click-to-move should compute same path
// Simulate mouse click at (15, 10)
msg := createMousePressMsg(15, 10)
updateFn(manualState, vm.ToValue(msg))
manualPath := manualState.Get("manualPath")
```

**Verification:**
```go
// Both should find the same path (go around obstacle, e.g., up then right)
assertPathIdentical(t, manualPath, path, "Pathfinding should produce identical routes")

// Execute 5 ticks of movement
for i := 0; i < 5; i++ {
  // Manual: traverse pre-computed path
  tickPathTraversal(manualState)
  // Auto: MoveTo action
  simulateAutoMovementTick(autoState, 15, 10)
}

// Positions should match
assertEqual(t, manualActor.x, autoActor.x, "Navigation positions identical")
```

**Helper Functions Needed:**
- `findPath(state, startX, startY, targetX, targetY, ignoreId)` - exported from script
- `createMousePressMsg(x, y)` - creates mouse press message
- `tickPathTraversal(state)` - simulates one tick of path following
- `assertPathIdentical(t, path1, path2, message)` - compares two path arrays

---

### Summary: T6-Physics Tests

| Test ID | Test Name | Scenario | Key Comparison |
|---------|-----------|----------|----------------|
| T6.1 | Unit Movement | Single-tick movement right | Position delta = 1.0 identical |
| T6.2 | Diagonal Movement | Up-right diagonal movement | Both axes moved identically |
| T6.3 | Multi-Tick Accumulation | 5 ticks of right movement | Final position identical |
| T6.4 | Held Item Non-Blocking | Move while holding item | Held item ignored identically |
| T6.5 | Obstacle Avoidance | Navigate around blocking cube | Path and positions identical |

---

## Part 3: Test Design - T7-Collision

### Test Suite: `TestSimulationConsistency_Collision_T7`

#### Test T7.1: Boundary Detection - Left Edge

**Objective:** Verify both modes correctly block movement at grid boundary

**Setup:**
- Actor at left edge: (1, 10)
- Attempt to move left (x would become 0, blocked)

**Test Actions:**
```go
_ = actor.Set("x", 1)
_ = actor.Set("y", 10)

// Manual: press 'a'
manualState.Set("gameMode", "manual")
applyManualMovementTick(manualState, []string{"a"})

// Auto: attempt MoveTo to (0, 10)
autoState.Set("gameMode", "automatic")
simulateAutoMovementTick(autoState, 0, 10)
```

**Verification:**
```go
// Both should stay at (1, 10) - blocked by boundary
assert.Equal(t, 1.0, manualActor.x, "Manual mode blocked at left edge")
assert.Equal(t, 1.0, autoActor.x, "Auto mode blocked at left edge")
assert.Equal(t, manualActor.x, autoActor.x, "Boundary blocking identical")
```

---

#### Test T7.2: Boundary Detection - Right Edge

**Objective:** Verify right edge boundary blocking

**Setup:**
- Actor at (59, 10) (spaceWidth=60)
- Attempt to move right (would be >= 60, blocked)

**Test Actions & Verification:**
```go
_ = actor.Set("x", 59)
// Both modes press 'd' rightwards
// Verification identical to T7.1
assert.Equal(t, 59.0, manualActor.x, "Manual blocked at right edge")
assert.Equal(t, 59.0, autoActor.x, "Auto blocked at right edge")
```

---

#### Test T7.3: Cube Collision Detection

**Objective:** Verify single cube blocks movement identically

**Setup:**
```
Actor @ (10, 10)
Cube  @ (11, 10) - will block right movement
```

**Test Actions:**
```go
addCube(vm, state, 701, 11, 10, false, "obstacle")

// Attempt movement right
applyManualMovementTick(manualState, []string{"d"})
simulateAutoMovementTick(autoState, 12, 10)  // Target (12, 10) blocked by (11, 10)
```

**Verification:**
```go
// Both should stay at (10, 10)
assert.Equal(t, 10.0, manualActor.x, "Manual blocked by cube")
assert.Equal(t, 10.0, autoActor.x, "Auto blocked by cube")
```

---

#### Test T7.4: Multiple Cubes Blocking

**Objective:** Verify multiple cubes each block movement correctly

**Setup:**
```
Actor   @ (10, 10)
Cube 1 @ (11, 10) - blocks right
Cube 2 @ (10, 11) - blocks down
Cube 3 @ (9, 10)  - blocks left
```

**Test Actions:**
```go
addCube(vm, state, 702, 11, 10, false, "obstacle")
addCube(vm, state, 703, 10, 11, false, "obstacle")
addCube(vm, state, 704, 9, 10, false, "obstacle")

// Test all 4 directions
testCases := []struct {
  key   string
  dx, dy float64
}{
  {"d", 1, 0},  // Right blocked
  {"s", 0, 1},  // Down blocked
  {"a", -1, 0}, // Left blocked
  {"w", 0, -1}, // Up - should work (no cube)
}

for _, tc := range testCases {
  // Manual: press key
  applyManualMovementTick(manualStateClone(tc.key), []string{tc.key})
  // Auto: MoveTo
  simulateAutoMovementTick(autoStateClone(tc.key), 10+tc.dx, 10+tc.dy)
  
  // Compare
  assert.Equal(t, manualActorClone(tc.key).x, autoActorClone(tc.key).x, 
    "Direction="+tc.key+" should have identical X")
}
```

---

#### Test T7.5: BuildBlockedSet Consistency

**Objective:** Directly compare `buildBlockedSet` output between modes

**Setup:**
- Create complex scene with:
  - 5 active cubes at various positions
  - 2 deleted cubes (should be ignored)
  - Actor holding 1 cube (should be ignored)
  - 1 static wall (should be considered)

**Test Actions:**
```go
// Add cubes
addCube(vm, state, 801, 5, 5, false, "obstacle")    // Active
addCube(vm, state, 802, 6, 5, false, "obstacle")    // Active
addCube(vm, state, 803, 7, 5, false, "obstacle")    // Active
addCube(vm, state, 804, 8, 5, false, "obstacle")    // Active
addCube(vm, state, 805, 9, 5, false, "obstacle")    // Active

// Delete 2 cubes
setCubeDeleted(state, 804, true)
setCubeDeleted(state, 805, true)

// Hold 1 cube
heldItem := vm.NewObject()
heldItem.Set("id", 801)
actor.Set("heldItem", heldItem)
setCubeDeleted(state, 801, true)  // Mark as deleted when held

// Add static wall
addCube(vm, state, 1001, 10, 5, true, "wall")
```

**Verification:**
```go
// Call buildBlockedSet on both state instances
// Since both modes use the SAME function, results MUST be identical
manualBlocked := callBuildBlockedSet(manualState, -1)
autoBlocked := callBuildBlockedSet(autoState, -1)

assert.Equal(t, manualBlocked, autoBlocked, 
  "buildBlockedSet must produce identical sets")

// Verify expected blocked cells
expectedBlocked := []string{"6,5", "7,5", "10,5"}  // 802, 803, static wall
assertContainsAll(t, expectedBlocked, manualBlocked, 
  "All expected cells in blocked set")

// Verify ignored cells NOT in set
ignoredCells := []string{"5,5", "8,5", "9,5"}  // 801 (held), 804/805 (deleted)
assertContainsNone(t, ignoredCells, manualBlocked, 
  "Ignored cells not in blocked set")
```

**Helper Functions Needed:**
- `callBuildBlockedSet(state, ignoreCubeId)` - invokes buildBlockedSet via Goja
- `assertContainsAll(t, expected, set, msg)` - verifies all expected strings in set
- `assertContainsNone(t, unexpected, set, msg)` - verifies none of unexpected in set

---

#### Test T7.6: Pick Threshold Calculation

**Objective:** Verify pick threshold (1.8) calculated identically

**Setup:**
```
Actor @ (10, 10)
Cube @ (11, 10) - distance = 1.0 < 1.8, should pick
Cube @ (15, 10) - distance = 5.0 > 1.8, should NOT pick
```

**Test Actions:**
```go
addCube(vm, state, 901, 11, 10, false, "obstacle")
addCube(vm, state, 902, 15, 10, false, "obstacle")

// Manual mode: click on (11, 10) - should succeed
msgNear := createMousePressMsg(11, 10)
updateFn(manualState, vm.ToValue(msgNear))

// Manual mode: click on (15, 10) - should fail
msgFar := createMousePressMsg(15, 10)
updateFn(manualState2, vm.ToValue(msgFar))
```

**Verification:**
```go
// Near distance should allow pick
heldItemNear := manualState.Get("actors").ToObject(vm).Get("get")

// Use get to fetch actor, then check heldItem
getFn, _ := goja.AssertFunction(manualState.Get("actors").ToObject(vm).Get("get"))
actorNearVal, _ := getFn(manualState.Get("actors").ToObject(vm), vm.ToValue(1))
actorNear := actorNearVal.ToObject(vm)
heldItemNear := actorNear.Get("heldItem")

assert.False(t, goja.IsNull(heldItemNear), 
  "Distance 1.0 < PICK_THRESHOLD should pick")

// Far distance should NOT pick
//
// Reset manualState2 fresh for far test
msgFar := createMousePressMsg(15, 10)
// ... apply ...
heldItemFar := actor2.Get("heldItem")
assert.True(t, goja.IsNull(heldItemFar), 
  "Distance 5.0 > PICK_THRESHOLD should NOT pick")
```

---

#### Test T7.7: Place Threshold Calculation

**Objective:** Verify place threshold (1.5) works identically

**Setup:**
- Actor at (10, 10) holding cube 951
- Empty cells at distances < 1.5 and > 1.5

**Test Actions:**
```go
// Set up held item
heldItem := vm.NewObject()
heldItem.Set("id", 951)
actor.Set("heldItem", heldItem)
setCubeDeleted(state, 951, true)

// Click adjacency (11, 10) - distance 1.0 < 1.5
msgNear := createMousePressMsg(11, 10)
updateFn(manualState, vm.ToValue(msgNear))

// Verify item placed
cube951 := getCube(t, vm, manualState, 951)
assert.Equal(t, int64(11), cube951.Get("x").ToInteger())
assert.Equal(t, int64(10), cube951.Get("y").ToInteger())
```

---

### Summary: T7-Collision Tests

| Test ID | Test Name | Scenario | Key Comparison |
|---------|-----------|----------|----------------|
| T7.1 | Left Edge Boundary | Block at x=1 | Both blocked identically |
| T7.2 | Right Edge Boundary | Block at x=59 | Both blocked identically |
| T7.3 | Single Cube Collision | Single blocking cube | Both blocked identically |
| T7.4 | Multiple Cubes Blocking | 4-direction testing | All directions blocked identically |
| T7.5 | BuildBlockedSet | Complex scene | Blocked sets identical bit-for-bit |
| T7.6 | Pick Threshold | Distance < 1.8 vs > 1.8 | Pick success/failure identical |
| T7.7 | Place Threshold | Distance < 1.5 | Place success identical |

---

## Part 4: Test Design - T8-Pathfinding

### Test Suite: `TestSimulationConsistency_Pathfinding_T8`

#### Test T8.1: getPathInfo - Unobstructed Path

**Objective:** Verify reachability and distance calculation identical

**Setup:**
- Start: (10, 10)
- Goal: (20, 10)
- Empty field, no obstacles

**Test Actions:**
```go
// Call getPathInfo on both modes
manualInfo := getPathInfo(manualState, 10, 10, 20, 10, -1)
autoInfo := getPathInfo(autoState, 10, 10, 20, 10, -1)
```

**Verification:**
```go
// Both should return reachable=true, distance=10
assert.True(t, manualInfo.Get("reachable").ToBoolean())
assert.True(t, autoInfo.Get("reachable").ToBoolean())
assert.Equal(t, float64(10), manualInfo.Get("distance").ToFloat())
assert.Equal(t, float64(10), autoInfo.Get("distance").ToFloat())
assert.Equal(t, manualInfo.Get("reachable"), autoInfo.Get("reachable"))
assert.Equal(t, manualInfo.Get("distance"), autoInfo.Get("distance"))
```

**Helper Functions Needed:**
- `getPathInfo(state, startX, startY, targetX, targetY, ignoreId)` - exported via module.exports

---

#### Test T8.2: getPathInfo - Blocked Path

**Objective:** Verify unreachability detected identically

**Setup:**
- Start: (10, 10)
- Goal: (20, 10)
- Wall of cubes blocking entire corridor

**Test Actions:**
```go
// Create wall from y=8 to y=12 at x=15
for y := 8; y <= 12; y++ {
  addCube(vm, manualStateFactory(), 1000+y, 15, int64(y), true, "wall")
  addCube(vm, autoStateFactory(), 1000+y, 15, int64(y), true, "wall")
}

manualInfo := getPathInfo(manualState, 10, 10, 20, 10, -1)
autoInfo := getPathInfo(autoState, 10, 10, 20, 10, -1)
```

**Verification:**
```go
assert.False(t, manualInfo.Get("reachable").ToBoolean())
assert.False(t, autoInfo.Get("reachable").ToBoolean())
assert.Equal(t, float64(math.Inf(1)), manualInfo.Get("distance").ToFloat())
assert.Equal(t, float64(math.Inf(1)), autoInfo.Get("distance").ToFloat())
```

---

#### Test T8.3: findPath - Clear Path Waypoints

**Objective:** Verify BFS path algorithm produces identical waypoints

**Setup:**
- Start: (10, 10)
- Goal: (15, 10)
- No obstacles

**Test Actions:**
```go
manualPath := findPath(manualState, 10, 10, 15, 10, -1)
autoPath := findPath(autoState, 10, 10, 15, 10, -1)
```

**Verification:**
```go
// Both should return same array of waypoints
assert.Equal(t, manualPath, autoPath, "Clear path waypoints identical")

// Expected: [(11,10), (12,10), (13,10), (14,10), (15,10)]
expectedLength := 5
assert.Equal(t, expectedLength, getPathLength(manualPath))
assert.Equal(t, expectedLength, getPathLength(autoPath))
```

**Helper Functions Needed:**
- `findPath(state, startX, startY, targetX, targetY, ignoreId)` - exported
- `getPathLength(pathObj)` - returns array length

---

#### Test T8.4: findPath - Obstacle Rerouting

**Objective:** Verify pathfinding finds same alternative route around obstacles

**Setup:**
```
Start: (5, 10)
Goal:  (15, 10)
Wall:  Blocked cells at x=10, y=8..12 (5-tall wall)
```

**Test Actions:**
```go
// Create wall
setup, err := setupPickAndPlaceTest(t)
require.NoError(t, err)
setupManual, _, manualState, _ := setup
setupAuto, _, autoState, _ := setupPickAndPlaceTest(t)

// Add wall to both states (avoid re-running setup twice by cloning)
// Instead, add cubes before computing path
for y := int64(8); y <= 12; y++ {
  addCube(t, setupManual.vms[vmsIdx], manualStates[statesIdx], 2000+y, 10, y, true, "wall")
  addCube(t, setupAuto.vms[vmsIdx], autoStates[statesIdx], 2000+y, 10, y, true, "wall")
}

manualPath := findPath(manualState, 5, 10, 15, 10, -1)
autoPath := findPath(autoState, 5, 10, 15, 10, -1)
```

**Verification:**
```go
// Both should find same route (e.g., go up to y=7, across, down)
assertPathIdentical(t, manualPath, autoPath, 
  "Alternative path around obstacle identical")

// Verify path goes around wall (does NOT pass through x=10 at y=8..12)
assertPathAvoidsRange(t, manualPath, 10, 10, 8, 12)
assertPathAvoidsRange(t, autoPath, 10, 10, 8, 12)
```

**Helper Functions Needed:**
- `assertPathIdentical(t, path1, path2, msg)` - deep compare
- `assertPathAvoidsRange(t, path, xMin, xMax, yMin, yMax)` - verify path clears obstacle area

---

#### Test T8.5: findNextStep - Immediate Direction

**Objective:** Verify first step direction matches

**Setup:**
- Start: (10, 10)
- Goal: (12, 10) - clear path right
- No obstacles

**Test Actions:**
```go
manualNext := findNextStep(manualState, 10, 10, 12, 10, -1)
autoNext := findNextStep(autoState, 10, 10, 12, 10, -1)
```

**Verification:**
```go
// Both should return {x: 11, y: 10} (next step right)
assert.Equal(t, 11.0, manualNext.Get("x").ToFloat())
assert.Equal(t, 11.0, autoNext.Get("x").ToFloat())
assert.Equal(t, 10.0, manualNext.Get("y").ToFloat())
assert.Equal(t, 10.0, autoNext.Get("y").ToFloat())
```

**Helper Functions Needed:**
- `findNextStep(state, startX, startY, targetX, targetY, ignoreId)` - exported

---

#### Test T8.6: findPath with ignoreCubeId Parameter

**Objective:** Verify pathfinding correctly ignores specified cube

**Setup:**
```
Start:  (10, 10)
Goal:   (15, 10)
Cube 1: @ (12, 10) - blocked
Cube 2: @ (13, 10) - target (should be ignored via ignoreId)
```

**Test Actions:**
```go
addCube(t, setupManual.vms[0], manualState, 3001, 12, 10, false, "obstacle")
addCube(t, setupManual.vms[0], manualState, 3002, 13, 10, false, "obstacle")

addCube(t, setupAuto.vms[0], autoState, 3001, 12, 10, false, "obstacle")
addCube(t, setupAuto.vms[0], autoState, 3002, 13, 10, false, "obstacle")

// Ignore cube 3002, so we can pass through (13, 10)
manualPath := findPath(manualState, 10, 10, 15, 10, 3002)
autoPath := findPath(autoState, 10, 10, 15, 10, 3002)
```

**Verification:**
```go
// Both should find path that goes through (13, 10) because it's ignored
assertPathContainsPoint(t, manualPath, 13, 10, 
  "Manual path should go through ignored cube")
assertPathContainsPoint(t, autoPath, 13, 10, 
  "Auto path should go through ignored cube")

// But should still avoid (12, 10) which is NOT ignored
assertPathAvoidsPoint(t, manualPath, 12, 10)
assertPathAvoidsPoint(t, autoPath, 12, 10)
```

---

#### Test T8.7: findFirstBlocker - Dynamic Obstacle Discovery

**Objective:** Verify PA-BT blocker discovery matches pathfinding

**Setup:**
```
Start:  (5, 18)
Goal:   (8, 18) - goal area
Cubes:  Obstacle ring around goal (goal blockade ring)
         Specifically: Cube at (6, 18) blocks path
```

**Test Actions:**
```go
// Setup goal blockade ring (matching script initialization)
// Cube at (6, 18) is the ring blocker
addCube(t, setupManual.vms[0], manualState, 100, 6, 18, false, "obstacle")
addCube(t, setupAuto.vms[0], autoState, 100, 6, 18, false, "obstacle")

// findFirstBlocker should find cube 100
manualBlocker := findFirstBlocker(manualState, 5, 18, 8, 18, -1)
autoBlocker := findFirstBlocker(autoState, 5, 18, 8, 18, -1)
```

**Verification:**
```go
// Both should return cube ID 100 (blocker at 6,18)
assert.Equal(t, int64(100), manualBlocker.ToInteger())
assert.Equal(t, int64(100), autoBlocker.ToInteger())
```

**Helper Functions Needed:**
- `findFirstBlocker(state, fromX, fromY, toX, toY, excludeId)` - may need to be exported via module.exports

---

#### Test T8.8: Manual Mouse Click Pathfinding vs findPath

**Objective:** Verify manual mode click-to-move uses same BFS as findPath

**Setup:**
- Actor at (10, 10)
- Click at (20, 10) across empty field

**Test Actions:**
```go
// Manual mode: trigger mouse click
msg := createMousePressMsg(20, 10)
updateFn(manualState, vm.ToValue(msg))
manualPath := manualState.Get("manualPath")

// Auto mode: compute path directly
autoPath := findPath(autoState, 10, 10, 20, 10, -1)
```

**Verification:**
```go
// Paths should be bit-for-bit identical
assertPathIdentical(t, manualPath, autoPath, 
  "Manual click-to-move path matches findPath")
```

---

### Summary: T8-Pathfinding Tests

| Test ID | Test Name | Scenario | Key Comparison |
|---------|-----------|----------|----------------|
| T8.1 | getPathInfo Clear | Unobstructed path | Reachable=true, distance=10 identical |
| T8.2 | getPathInfo Blocked | Path fully blocked | Reachable=false, distance=∞ identical |
| T8.3 | findPath Clear | Clear path waypoints | Waypoint arrays identical |
| T8.4 | findPath Obstacle | Obstacle rerouting | Alternative routes identical |
| T8.5 | findNextStep Direction | First step direction | Next step coords identical |
| T8.6 | ignoreCubeId Parameter | Path ignoring specific cube | Passed-through cells match |
| T8.7 | findFirstBlocker | Dynamic blocker discovery | Blocker ID matches |
| T8.8 | Click-to-Move vs findPath | Manual vs direct BFS | Paths identical |

---

## Part 5: Helper Functions Required

### Core Test Setup Functions

```go
// Clone state to create identical auto/manual copies
func cloneState(t *testing.T, vm *goja.Runtime, state *goja.Object) (*goja.Object, *goja.Runtime) {
  // Deep clone state object for second test instance
  // Create new VM with same scripts loaded
}

// Setup pair of identical states (manual + auto)
func setupConsistencyTestPair(t *testing.T) (*goja.Runtime, *goja.Object, *goja.Runtime, *goja.Object) {
  // Returns: (manualVM, manualState, autoVM, autoState)
}
```

### Movement Helper Functions

```go
// Apply manual mode tick WASD movement
func applyManualMovementTick(t *testing.T, vm *goja.Runtime, state *goja.Object, keys []string) error {
  // manualKeys = keys
  // msg = {type: "Tick", id: "tick"}
  // updateFn(state, msg)
}

// Simulate automatic mode MoveTo tick
func simulateAutoMovementTick(t *testing.T, vm *goja.Runtime, state *goja.Object, targetX, targetY float64) error {
  // Execute MoveTo action tick toward target
  // Use MoveTo tickFn from script
}
```

### Collision Helper Functions

```go
// Invoke buildBlockedSet from script
func callBuildBlockedSet(t *testing.T, vm *goja.Runtime, state *goja.Object, ignoreCubeId int64) map[string]bool {
  // Returns Set of "x,y" strings for blocked cells
}

// Parse blocked Set as Go map
func parseBlockedSet(t *testing.T, vm *goja.Runtime, blockedObj *goja.Object) map[string]bool {
  // Convert goja Map/Set to Go map[string]bool
}

// Assertions for blocked sets
func assertContainsAll(t *testing.T, expected []string, blocked map[string]bool, msg string)
func assertContainsNone(t *testing.T, unexpected []string, blocked map[string]bool, msg string)
```

### Pathfinding Helper Functions

```go
// Invoke getPathInfo from script
funcgetPathInfo(t *testing.T, vm *goja.Runtime, state *goja.Object, startX, startY, targetX, targetY, ignoreId float64) *goja.Object {
  // Returns {reachable: bool, distance: number}
}

// Invoke findPath from script
func findPath(t *testing.T, vm *goja.Runtime, state *goja.Object, startX, startY, targetX, targetY, ignoreId float64) *goja.Object {
  // Returns array of {x, y} waypoints
}

// Invoke findNextStep from script
func findNextStep(t *testing.T, vm *goja.Runtime, state *goja.Object, startX, startY, targetX, targetY, ignoreId float64) *goja.Object {
  // Returns {x, y} coordinates
}

// Invoke findFirstBlocker from script
func findFirstBlocker(t *testing.T, vm *goja.Runtime, state *goja.Object, fromX, fromY, toX, toY, excludeId float64) goja.Value {
  // Returns blocker ID or null/-1
}
```

### Path Comparison Helper Functions

```go
// Get length of path array
func getPathLength(t *testing.T, pathObj *goja.Object) int64

// Deep comparison of two path arrays
func assertPathIdentical(t *testing.T, path1Obj, path2Obj *goja.Object, msg string) {
  // Compare waypoint-by-waypoint
}

// Verify path contains specific point
func assertPathContainsPoint(t *testing.T, pathObj *goja.Object, x, y int64, msg string)

// Verify path does NOT contain specific point
func assertPathAvoidsPoint(t *testing.T, pathObj *goja.Object, x, y int64, msg string)

// Verify path clears rectangular area (xMin,yMin) to (xMax,yMax)
func assertPathAvoidsRange(t *testing.T, pathObj *goja.Object, xMin, xMax, yMin, yMax int64, msg string)
```

### Message Helper Functions

```go
// Create mouse press message
func createMousePressMsg(x, y int64) map[string]interface{} {
  return map[string]interface{}{
    "type":   "Mouse",
    "event":  "press",
    "x":      x + 10, // Add spaceX offset (spaceWidth=60, width=80, offset=10)
    "y":      y,
    "button": "left",
  }
}

// Create key message
func createKeyMsg(key string) map[string]interface{} {
  return map[string]interface{}{
    "type": "Key",
    "key":  key,
  }
}

// Create tick message
func createTickMsg() map[string]interface{} {
  return map[string]interface{}{
    "type": "Tick",
    "id":   "tick",
  }
}
```

### State Comparison Helper Functions

```go
// Compare actor positions with tolerance
func assertActorPositionsEqual(t *testing.T, manualState, autoState *goja.Object, tolerance float64) {
  // Get actor from both states
  // Compare x and y within tolerance
}

// Compare full state structure
func assertStatesEqual(t *testing.T, manualState, autoState *goja.Object, options StateCompareOptions) {
  // Deep compare:
  // - actor.x, actor.y
  // - cubes positions
  // - heldItem state
  // - tickCount
  // - gameMode (should differ by design)
}

type StateCompareOptions struct {
  IgnoreKeys      []string // keys to ignore in comparison
  Tolerance       float64  // numeric tolerance
  CompareCubes    bool     // compare cube map
  CompareActors   bool     // compare actor positions
}
```

---

## Part 6: Exported Functions Required from Script

To enable Go tests to call JavaScript functions, the following must be added to `module.exports` in `scripts/example-05-pick-and-place.js`:

```javascript
// Existing exports:
module.exports = {
  init,
  update,
  initializeSimulation,
  TARGET_ID
};

// ADDITIONAL EXPORTS for consistency tests:
module.exports.buildBlockedSet = buildBlockedSet;
module.exports.findPath = findPath;
module.exports.getPathInfo = getPathInfo;
module.exports.findNextStep = findNextStep;
module.exports.findFirstBlocker = findFirstBlocker;
module.exports.manualKeysMovement = manualKeysMovement;
```

**Rationale:** These functions are the core simulation primitives. Go tests need to invoke them directly to compare results across modes.

---

## Part 7: Test Coverage Matrix

| Component | Manual Mode | Auto Mode | Cross-Mode Consistency |
|-----------|-------------|-----------|----------------------|
| **Physics** | T6.1-T6.5 | ✓ | ✅ All T6 tests |
| **Collision** | T7.1-T7.7 | ✓ | ✅ All T7 tests |
| **Pathfinding** | T8.1-T8.8 | ✓ | ✅ All T8 tests |

**Total Test Cases:**
- T6-Physics: 5 tests
- T7-Collision: 7 tests
- T8-Pathfinding: 8 tests
- **Grand Total: 20 comprehensive integration tests**

---

## Part 8: Implementation Priority

### Phase 1: Foundation (Required first)
1. ✅ Export simulation functions from JavaScript (module.exports additions)
2. ✅ Implement core test setup and helper functions
3. ✅ Implement state cloning and comparison utilities

### Phase 2: Physics Tests (T6)
4. ✅ Implement T6.1 - Unit Movement
5. ✅ Implement T6.2 - Diagonal Movement
6. ✅ Implement T6.3 - Multi-Tick Accumulation
7. ✅ Implement T6.4 - Held Item Non-Blocking
8. ✅ Implement T6.5 - Obstacle Avoidance

### Phase 3: Collision Tests (T7)
9. ✅ Implement T7.1, T7.2 - Boundary Detection
10. ✅ Implement T7.3, T7.4 - Cube Collision
11. ✅ Implement T7.5 - BuildBlockedSet Consistency
12. ✅ Implement T7.6 - Pick Threshold
13. ✅ Implement T7.7 - Place Threshold

### Phase 4: Pathfinding Tests (T8)
14. ✅ Implement T8.1, T8.2 - getPathInfo
15. ✅ Implement T8.3, T8.4 - findPath
16. ✅ Implement T8.5, T8.6 - findNextStep + ignoreCubeId
17. ✅ Implement T8.7 - findFirstBlocker
18. ✅ Implement T8.8 - Click-to-Move vs findPath

### Phase 5: Integration & Review
19. ✅ Run all tests, debug failures
20. ✅ Update blueprint.json (mark T6-T8 complete)
21. ✅ Execute RG-3 review cycle

---

## Part 9: Integration with Existing Test Infrastructure

### File Structure

```
internal/builtin/bubbletea/
├── manual_mode_comprehensive_test.go   (T9-T13 + T14-T15 existing)
└── simulation_consistency_test.go      (NEW: T6-T8)
```

### Shared Helper Functions

The following helpers from `manual_mode_comprehensive_test.go` will be re-used:

- `setupPickAndPlaceTest(t)` - base test setup
- `getUpdateFn(t, exports)` - update function extractor
- `getActor(t, vm, state)` - actor accessor
- `addCube(t, vm, state, id, x, y, isStatic, cubeType)` - cube creator
- `getCube(t, vm, state, id)` - cube accessor
- `setCubeDeleted(t, vm, state, id, deleted)` - state updater
- `clearManualKeys(t, vm, state)` - manual state cleaner

### Code Organization

```go
// New file: simulation_consistency_test.go
package bubbletea

import (
  // ... imports
)

// ============================================================================
// SECTION A: Helper Functions (Phase 1)
// ============================================================================

// (Implementation of helper functions from Part 5)

// ============================================================================
// SECTION B: T6-Physics Tests (Phase 2)
// ============================================================================

func TestSimulationConsistency_Physics_T6(t *testing.T) {
  t.Run("T6.1: Unit Movement Identical Vector Magnitude", func(t *testing.T) { /* ... */ })
  t.Run("T6.2: Diagonal Movement Consistency", func(t *testing.T) { /* ... */ })
  // ... rest of T6 tests
}

// ============================================================================
// SECTION C: T7-Collision Tests (Phase 3)
// ============================================================================

func TestSimulationConsistency_Collision_T7(t *testing.T) {
  t.Run("T7.1: Boundary Detection - Left Edge", func(t *testing.T) { /* ... */ })
  // ... rest of T7 tests
}

// ============================================================================
// SECTION D: T8-Pathfinding Tests (Phase 4)
// ============================================================================

func TestSimulationConsistency_Pathfinding_T8(t *testing.T) {
  t.Run("T8.1: getPathInfo - Unobstructed Path", func(t *testing.T) { /* ... */ })
  // ... rest of T8 tests
}

// ============================================================================
// SECTION E: Cross-Mode Integration Tests (Optional Bonus)
// ============================================================================

func TestSimulationConsistency_CompleteScenario_T6_T7_T8(t *testing.T) {
  // Test a full simulation scenario:
  // 1. Pick up cube (T6 physics + T7 collision for pick threshold)
  // 2. Move to goal (T6 physics + T8 pathfinding)
  // 3. Navigate around obstacle (T7 collision + T8 pathfinding)
  // 4. Place cube in goal (T6 physics + T7 collision for place threshold)
  // Compare every tick's state between manual and auto modes
}
```

---

## Part 10: Success Criteria

### Code Quality Requirements

✅ **All tests must:**
- Use 2-second timeouts (per hang prevention strategy)
- Run sequentially (no `t.Parallel()`)
- Pass on all 3 OS platforms (Linux, macOS, Windows)
- Produce clear, actionable error messages on failure
- Update blueprint.json (mark T6-T8 as "completed")

### Test Coverage Goals

✅ **Physics Tests (T6):**
- ✅ Cover unit movement, diagonal movement, multi-tick accumulation
- ✅ Verify held-item non-blocking behavior
- ✅ Test obstacle navigation

✅ **Collision Tests (T7):**
- ✅ Cover all 4 boundary directions
- ✅ Test single and multiple collision objects
- ✅ Directly comparison of buildBlockedSet output
- ✅ Verify pick (1.8) and place (1.5) thresholds

✅ **Pathfinding Tests (T8):**
- ✅ Test all 4 pathfinding functions (getPathInfo, findPath, findNextStep, findFirstBlocker)
- ✅ Cover clear paths, blocked paths, obstacle rerouting
- ✅ Verify ignoreCubeId parameter behavior
- ✅ Compare manual click-to-move with direct BFS

### Integration Requirements

✅ **Tests run successfully:**
- ✅ As individual test files
- ✅ As part of full test suite (`go test ./...`)
- ✅ With race detector (`go test -race ./...`)
- ✅ With verbose output for debugging (`go test -v ./internal/builtin/bubbletea/`)

---

## Conclusion

This test plan provides a **comprehensive, phased approach** to implementing simulation consistency tests for `scripts/example-05-pick-and-place.js`. The design guarantees that manual and automatic modes produce **identical behavior** across all core simulation mechanics:

1. **Physics (T6):** Movement deltas, diagonal vectors, multi-tick accumulation
2. **Collision (T7):** Boundary detection, cube collision, threshold calculations
3. **Pathfinding (T8):** BFS algorithms, path generation, dynamic blocker discovery

**Key guarantee:** Once these tests pass, we have mathematical proof that the simulation is deterministic and mode-agnostic.

**Next Steps:**
1. Implement script exports (Phase 1)
2. Create helper functions (Phase 1)
3. Implement tests following Phase 2 → Phase 4 sequence
4. Run tests, debug, iterate
5. Update blueprint.json (mark T6-T8 complete)
6. Execute RG-3 review cycle

---

**Document End**
