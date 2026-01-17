---
title: osm:bt Blackboard usage guide
description: When to (and when NOT to) use bt.Blackboard for behavior trees
tags:
  - bt
  - blackboard
  - behavior tree
  - goja
  - performance
---

# osm:bt Blackboard Usage Guide

**Quick summary**

- **Use bt.Blackboard**: ONLY when using Go-based composites (PA-BT planning, Go-side conditions)
- **Avoid bt.Blackboard**: For pure JavaScript behavior trees - direct object access is faster and simpler
- **JavaScript state is the single source of truth**: Blackboard is a minimal bridge, not the primary state store
- **Sync only planning inputs**: Copy primitive values to blackboard before Go planning; BT nodes read/write JS state directly

---

## Architecture: Three State Management Patterns

### Pattern A: Pure JavaScript Behavior Trees (NO BLACKBOARD)

**Use case**: Simple AI, demos, prototyping, any scenario without Go planning systems

**State management**:

- JavaScript objects/maps are the single source of truth
- BT leaf functions capture state via closure
- Direct property access (O(1) with zero overhead)

**Example**:

```javascript
// Actor state - single source of truth
const enemy = {
    id: 1,
    x: 40,
    y: 10,
    health: 50,
    target: null
};

// Leaf captures state via closure
const checkInRangeLeaf = bt.createLeafNode(() => {
    const dx = state.player.x - enemy.x;
    const dy = state.player.y - enemy.y;
    const dist = Math.sqrt(dx * dx + dy * dy);
    return dist < 10 ? bt.success : bt.failure;
});

// Leaf directly mutates state
const attackLeaf = bt.createLeafNode(() => {
    enemy.target = state.player.id;
    return bt.success;
});

// Pure JS composition
const tree = bt.node(bt.sequence,
    checkInRangeLeaf,
    attackLeaf
);

// Execution - no blackboard involved
const ticker = bt.newTicker(100, tree);
```

**Benefits**:

- ✅ **Fastest**: Direct object access, no mutex overhead
- ✅ **Simple**: Single source of truth, no sync bugs
- ✅ **Type-safe**: JavaScript objects have dynamic types
- ✅ **Flexible**: State can be any JS structure (objects, Maps, Sets)

**When to choose**:

- Pure JavaScript behavior trees (no Go composites)
- Interactive demos and prototypes
- < 100 entities (performance difference negligible either way)
- Learning/practicing behavior trees

**When to avoid**:

- ❌ Using Go-based PA-BT planning (requires Go-readable state)
- ❌ Need thread-safety between Go and JS
- ❌ State accessed from Go code (outside BT execution)

---

### Pattern B: Blackboard as MinimalPlanning Bridge (SYNC ONE-WAY)

**Use case**: Go-based PA-BT planning algorithm (go-pabt) selects actions in Go

**State management**:

- JavaScript objects/maps = single source of truth
- `bt.Blackboard` holds minimal planning inputs only (primitive values)
- **One-way sync**: JS → Blackboard (before planning tick)
- **No reverse sync**: BT nodes execute in JS and read/write JS state directly

**Example**:

```javascript
// Primary state store (JavaScript)
const state = {
    models: {
        actors: new Map([[1, {id: 1, x: 5, y: 10, heldItem: null}]]),
        cubes: new Map([[1, {id: 1, x: 15, y: 15, deleted: false}]]),
        goals: new Map([[1, {id: 1, x: 40, y: 12}]])
    },
    pabt: {
        blackboard: new bt.Blackboard(),  // Only for Go planner inputs
        plan: null,
        ticker: null
    }
};

// PA-BT action with JS execution
const pickCubeAction = pabt.newAction({
    conditions: [
        {
            key: 'heldItemExists',
            Match: (v) => v === false  // Go planner reads from blackboard
        },
        {
            key: 'cube_1_within_range',
            Match: (v) => {
                // Complex JS calculation via closure
                const actor = state.models.actors.get(1);
                const cube = state.models.cubes.get(1);
                const dx = cube.x - actor.x;
                const dy = cube.y - actor.y;
                return Math.sqrt(dx * dx + dy * dy) < 1.5;
            }
        }
    ],
    effects: [  // Declared for Go planning, NOT applied to blackboard
        {key: 'heldItemExists', Value: true},
        {key: 'cube_1_deleted', Value: true}
    ],
    node: bt.createLeafNode(() => {  // Executes in JS!
        // Direct JS mutation - no blackboard involved
        const actor = state.models.actors.get(1);
        const cube = state.models.cubes.get(1);
        cube.deleted = true;
        actor.heldItem = cube;
        return bt.success;
    })
});

// One-way sync: JS → Blackboard (planning inputs)
function syncPlanningInputs(state) {
    const bb = state.pabt.blackboard;
    const actor = state.models.actors.get(1);
    const cube = state.models.cubes.get(1);

    // Copy ONLY primitive values for Go planner
    bb.set('actorX', actor.x);
    bb.set('actorY', actor.y);
    bb.set('heldItemExists', actor.heldItem !== null);
    bb.set('cube_1_deleted', cube.deleted);
    bb.set('cube_1_x', cube.x);
    bb.set('cube_1_y', cube.y);
}

// Update loop
function update(state, msg) {
    if (msg.type === 'Tick') {
        syncPlanningInputs(state);  // Prepare Go planner inputs
        // PA-BT planner evaluates blackboard via Go pabt.State interface
        // Selected action.node() executes in JS (reads/writes state.models directly)
    }
}
```

**Why one-way sync?**

- **Blackboard is read-only for planner**: Go planner needs `Variable(key)` to evaluate conditions
- **BT node execution is pure JS**: The `bt.Node` returned by PA-BT runs in JavaScript world
- **No sync from blackboard**: Execution happens after planning, outputs go directly to `state.models`
- **Efficiency**: Only sync minimal primitive values (numbers, booleans), not entire objects

**Key principle**: Blackboard = Go planning bridge, NOT a state cache. Go planner reads blackboard; BT node mutates JS state.

---

### Pattern C: Full Blackboard State (TWO-WAY SYNC)

**Use case**: Mixed execution where Go code needs to control and observe mutations

**State management**:

- `bt.Blackboard` is the canonical state store
- Two-way sync between blackboard and JS state
- Go and JS both read/write blackboard

**Example**:

```javascript
// Blackboard-owned state (rarely needed in practice)
const state = {
    blackboard: new bt.Blackboard(),
    models: {}
};

// Initialize blackboard
state.blackboard.set('actorX', 5);
state.blackboard.set('actorY', 10);

// Two-way sync
function syncToBlackboards(state) {
    state.models.actors.forEach(actor => {
        state.blackboard.set(actor.id + '_x', actor.x);
    });
}

function syncFromBlackboards(state) {
    if (state.blackboard.has('newActorX')) {
        const newX = state.blackboard.get('newActorX');
        state.models.activeActor.x = newX;
        state.blackboard.delete('newActorX');
    }
}
```

**When to choose**:

- ❌ **Rarely needed** - consider if you ACTUALLY need Go to control mutations
- Possible use case: Go-side game engine with JS AI plugins
- Possible use case: Multi-language integration where Go owns lifecycle

**When to avoid**:

- Pure JS behavior trees (use Pattern A)
- PA-BT planning (use Pattern B)
- Simple demos (unnecessary complexity)

---

## Performance Analysis

### bt.Blackboard Overhead

**Read operations** (`get`, `has`, `keys`):

```
Go: RLock mutex → read map[key] → RUnlock
JS: goja bridge call → Go function → mutex → return
```

**Write operations** (`set`, `delete`):

```
Go: Lock mutex → map[key] = value → Unlock
JS: goja bridge call → Go function → mutex → return
```

**Benchmarks** (approximate):

- Direct JS property access: ~10ns
- bt.Blackboard.get(): ~500ns (50x slower due to mutex + goja bridge)
- bt.Blackboard.set(): ~800ns (80x slower)

### Impact on Real-World Scenarios

| Scenario         | Pattern     | Tick Rate | Performance                   |
|------------------|-------------|-----------|-------------------------------|
| < 50 entities    | A (JS-only) | 60 Hz     | Negligible (both fine)        |
| < 50 entities    | B (PA-BT)   | 60 Hz     | Sync cost < 1ms               |
| 100-500 entities | A (JS-only) | 60 Hz     | JS-only preferred             |
| 100-500 entities | B (PA-BT)   | 30 Hz     | Sync cost ~2-5ms (acceptable) |
| 1000+ entities   | A (JS-only) | 60 Hz     | JS-only required              |

**Rule of thumb**: Use Pattern A unless you specifically need Go planning. The performance difference matters at scale (1000+ entities) or high frequencies (>60 Hz).

---

## Decision Tree: Which Pattern to Use?

```
START

Do you need Go-based PA-BT planning?
├─ YES → Pattern B (Blackboard as planning bridge)
│         └─ One-way sync: JS → Blackboard (primitives only)
│
└─ NO → Does Go code control execution?
          ├─ YES → Pattern C (Full blackboard state)
          │         └─ Two-way sync (rarely needed)
          │
          └─ NO → Pattern A (Pure JavaScript)
                    └─ No blackboard, direct object access
```

---

## Common Pitfalls

### ❌ Using Blackboard Unnecessarily

```javascript
// WRONG: Blackboard for pure JS BT (adds unnecessary complexity)
const bb = new bt.Blackboard();
bb.set('enemyX', 50);
bb.set('enemyHP', 100);

const moveLeaf = bt.createLeafNode(() => {
    const x = bb.get('enemyX');  // Unnecessary mutex overhead
    const newX = x + 1;
    bb.set('enemyX', newX);
    return bt.success;
});
```

**Correct**:

```javascript
// BETTER: Direct JS object for pure JS BT
const enemy = {x: 50, hp: 100};

const moveLeaf = bt.createLeafNode(() => {
    enemy.x += 1;  // Direct mutation, zero overhead
    return bt.success;
});
```

### ❌ Two-Way Sync for PA-BT

```javascript
// WRONG: Two-way sync for PA-BT (unnecessary and error-prone)
function syncFromBlackboards(state) {
    if (bb.has('pickResult')) {  // BT node never writes this to blackboard!
        const cubeId = bb.get('pickResult');
        // ...
    }
}
```

**Correct**:

```javascript
// CORRECT: BT node mutates JS state directly
const pickNode = bt.createLeafNode(() => {
    const actor = state.models.actors.get(1);
    const cube = state.models.cubes.get(1);
    cube.deleted = true;
    actor.heldItem = cube;
    return bt.success;
});

// No syncFromBlackboards needed!
```

### ❌ Complex Objects in Blackboard

```javascript
// WRONG: Passing entire JS objects to Go planner
bb.set('cube_1', {x: 10, y: 15, deleted: false});

// Go planner evaluates: pabt.State.Variable('cube_1')
// Problem: Go can only treat value as `any` type
// Complex object inspection in Go is inefficient and error-prone
```

**Correct**:

```javascript
// CORRECT: Extract minimal planning primitives
state.models.cubes.get(1) // Full object in JS (for rendering, collision, etc.)
bb.set('cube_1_x', 10)       // Primitives for Go planner
bb.set('cube_1_y', 15)
bb.set('cube_1_deleted', false)
```

---

## Reference: bt.Blackboard API

### Constructor

```javascript
const bb = new bt.Blackboard();
```

### Methods

| Method            | Signature                    | Purpose                       | Thread-Safe |
|-------------------|------------------------------|-------------------------------|-------------|
| `get(key)`        | `any get(string key)`        | Retrieve value or `undefined` | ✅           |
| `set(key, value)` | `set(string key, any value)` | Store or update value         | ✅           |
| `has(key)`        | `boolean has(string key)`    | Check if key exists           | ✅           |
| `delete(key)`     | `delete(string key)`         | Remove key                    | ✅           |
| `keys()`          | `string[] keys()`            | Get all key names             | ✅           |
| `clear()`         | `clear()`                    | Remove all keys               | ✅           |
| `len()`           | `number len()`               | Get key count                 | ✅           |

### Threading Note

`bt.Blackboard` is implemented in Go with `sync.RWMutex`. All operations are thread-safe from any goroutine, including:

- JavaScript event loop goroutine (via `goja` calls)
- Go-side goroutines (direct access)
- Concurrent readers via `RLock`
- Exclusive writers via `Lock`

---

## Migration Guide

### From Pattern C to Pattern A

If you're using blackboard unnecessarily:

1. Identify where `new bt.Blackboard()` is called
2. Replace blackboard with JS object/Map
3. Refactor `bb.get(key)` to `object.prop` or `map.get(key)`
4. Refactor `bb.set(key, val)` to `object.prop = val` or `map.set(key, val)`
5. Remove sync functions if they only copy JS → blackboard

Example refactoring:

```javascript
// BEFORE (Pattern C)
const state = {
    blackboard: new bt.Blackboard(),
    models: {}
};

function init() {
    state.blackboard.set('score', 0);
    state.blackboard.set('health', 100);
}

function update() {
    const score = state.blackboard.get('score');
    state.blackboard.set('score', score + 10);
}

// AFTER (Pattern A)
const state = {
    models: {
        game: {score: 0, health: 100}
    }
};

function init() {
    state.models.game.score = 0;
    state.models.game.health = 100;
}

function update() {
    state.models.game.score += 10;  // Simpler, clearer
}
```

### From Pattern C to Pattern B

If you're using blackboard and need PA-BT:

1. Keep blackboard for Go planner inputs
2. Move primary state to JavaScript objects/Maps
3. Change sync to one-way: JS → blackboard (primitives only)
4. Remove `syncFromBlackboards()` (BT nodes execute in JS)
5. Add closure capture for leaf functions accessing JS state

---

## See Also

- [PA-BT integration example](example-05-pick-and-place.js) (forthcoming)
- [Pure JavaScript behavior tree example](example-04-bt-shooter.js) (current)
- [bt.Blackboard implementation](../../internal/builtin/bt/blackboard.go)
- [bt Bridge API](../../internal/builtin/bt/bridge.go)
