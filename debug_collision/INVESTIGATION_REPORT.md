# Collision Detection Bug Investigation Report

## Test Case
Test: "Collision Detection Stops Movement" in `TestManualMode_WASD_Movement_T13`

### Setup
- Actor at position (30, 12)
- Cube 701 placed at (31, 12) using `addCube(t, vm, state, 701, 31, 12, false, "obstacle")`
- Press 'd' key, then trigger Tick
- **Expected:** Actor STAYS at (30, 12) - collision should prevent movement  
- **Actual:** Actor MOVES to (31, 12) - collision NOT detected

---

## Investigation Results

### Script Collision Logic (lines 229-255 in example-05-pick-and-place.js)

The collision detection logic is **CORRECT**:

```javascript
const nx = actor.x + dx;  // 30 + 1 = 31
const ny = actor.y + dy;  // 12 + 0 = 12
let blocked = false;

// Boundary check
if (Math.round(nx) < 0 || Math.round(nx) >= state.spaceWidth ||
    Math.round(ny) < 0 || Math.round(ny) >= state.height) {
    blocked = true;
}

// Fast collision check
if (!blocked) {
    for (const c of state.cubes.values()) {
        if (!c.deleted && Math.round(c.x) === Math.round(nx) && Math.round(c.y) === Math.round(ny)) {
            if (actor.heldItem && c.id === actor.heldItem.id) continue;
            blocked = true;
            break;
        }
    }
}

if (!blocked) {
    actor.x = nx;  // This is executing!
    actor.y = ny;
}
```

### Root Cause Not in Collision Logic

Standalone testing confirmed that the collision logic **does work correctly**:
- When cube is at (31, 12) and actor tries to move to (31, 12)
- The condition `Math.round(c.x) === Math.round(nx)` matches (31 === 31) ✓
- The condition `Math.round(c.y) === Math.round(ny)` matches (12 === 12) ✓
- Collision is detected, `blocked = true`, actor stays at (30, 12) ✓

### Actual Root Cause: Test State Contamination

The **real bug** is in the test setup, not the script logic!

#### Test Sequence
All T13 subtests share the same `state` object:
1. **Test 1: Single Key Press Moves Actor Once**
   - Presses 'w' key → `manualKeys = { "w" }`
   - Actor moves from (30, 12) to (30, 11)
   - `manualKeys` never cleared

2. **Test 2: Key Hold Moves Continuously**
   - Presses 'd' key → `manualKeys = { "w", "d" }`
   - Actor moves right 5 times from (30, 12) to (35, 12)
   - `manualKeys` still contains both keys

3. **Test 3: Multiple Keys Pressed Diagonal Movement**
   - Presses 'w' key → `manualKeys = { "w", "d" }` (w already there)
   - Actor moves diagonally from (30, 12) to (31, 11)
   - `manualKeys` still contains both keys

4. **Test 4: Collision Detection Stops Movement** ← **FAILS HERE**
   - Adds cube 701 at (31, 12)
   - Presses 'd' key → `manualKeys = { "w", "d" }` (d already there)
   - Trigger Tick

#### What Happens in Test 4

When the Tick handler calls `manualKeysMovement()`:

```javascript
const getDir = () => {
    let dx = 0, dy = 0;
    if (state.manualKeys.get('w')) dy = -1;  // TRUE! From test 1 & 3
    if (state.manualKeys.get('s')) dy = 1;
    if (state.manualKeys.get('a')) dx = -1;
    if (state.manualKeys.get('d')) dx = 1;     // TRUE! From test 2 & 4
    return {dx, dy};
};

const {dx, dy} = getDir();  // dx=1, dy=-1 (DIAGONAL!)
```

Actor calculates target position:
```javascript
const nx = actor.x + dx;  // 30 + 1 = 31
const ny = actor.y + dy;  // 12 + (-1) = 11  ← NOT 12!
```

Collision check for position **(31, 11)**:
- Cube 701 is at **(31, 12)**
- `Math.round(31) === Math.round(31)` ✓ - X matches
- `Math.round(12) === Math.round(11)` ✗ - Y DOES NOT match
- **No collision detected!**

Actor moves to **(31, 11)** instead of staying at (30, 12).

Test expectation: `actor.x === 30`
Actual: `actor.x === 31`
**TEST FAILS**

---

### Why Keys Aren't Cleared in Tests

The script has key release detection based on timeout:

```javascript
// Key release detection: remove keys not seen recently (500ms timeout)
const KEY_RELEASE_TIMEOUT_MS = 500;
for (const [key, lastSeen] of state.manualKeyLastSeen.entries()) {
    if (now - lastSeen > KEY_RELEASE_TIMEOUT_MS) {
        state.manualKeys.delete(key);
        state.manualKeyLastSeen.delete(key);
    }
}
```

In production (real-time):
- Keys are released after 500ms of no press events
- Works correctly for normal usage

In tests:
- Tests execute in <100ms total
- No 500ms timeout passes between subtests
- `manualKeys` accumulates all pressed keys
- No cleanup happens

---

## Proposed Fixes

### Option 1: Fix Each Subtest to Clear Keys (Recommended)
Add this helper to each failing subtest that presses keys:

```go
// Helper to clear keys before test
func clearManualKeys(t *testing.T, vm *goja.Runtime, state *goja.Object) {
    manualKeys := state.Get("manualKeys").ToObject(vm)
    clearFn, _ := goja.AssertFunction(manualKeys.Get("clear"))
    clearFn(manualKeys)
    
    manualKeyLastSeen := state.Get("manualKeyLastSeen").ToObject(vm)
    clearFn2, _ := goja.AssertFunction(manualKeyLastSeen.Get("clear"))
    clearFn2(manualKeyLastSeen)
}

// Usage in collision test:
t.Run("Collision Detection Stops Movement", func(t *testing.T) {
    clearManualKeys(t, vm, state)  // ← Clear accumulated keys
    
    _ = actor.Set("x", 30)
    _ = actor.Set("y", 12)
    // ... rest of test
})
```

### Option 2: Modify Script to Add Clear Method
Add helper to script for test use:

```javascript
// Helper function for testing - clear all manual keys manually
function clearManualKeysForTest(state) {
    state.manualKeys.clear();
    state.manualKeyLastSeen.clear();
}
```

Export from script and call in tests before key operations.

### Option 3: Separate Test State per Subtest
Each subtest calls `setupPickAndPlaceTest()` to get fresh state.

**Pros:** Complete isolation, no state contamination
**Cons:** Slower, more overhead

---

## Summary

| Question | Answer |
|----------|--------|
| Is collision logic broken? | **NO** - Logic is correct |
| Is addCube causing issue? | **NO** - Cube properly registered |
| Is there timing/initialization issue? | **NO** - Not related to timing |
| Is cube not found in iteration? | **NO** - Cube is accessible |
| Is test state contaminated? | **YES** - `manualKeys` accumulates across subtests |

**Root Cause:** `state.manualKeys` Map is never cleared between T13 subtests, causing accumulated key presses from previous tests to be active during the collision test. The actor moves diagonally (d+w) instead of just right (d), and the diagonal target position (31, 11) doesn't collide with cube at (31, 12).

**Fix:** Clear `manualKeys` and `manualKeyLastSeen` Maps at the start of each subtest that tests key-based movement.
