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
- **⚠️ CRITICAL CONSTRAINT**: Blackboard ONLY supports types that `encoding/json` unmarshals to by default (bool, float64, int64, string, []interface{}, map[string]interface{}). NO custom structs, NO time.Time, NO channels.

---

## ⚠️ CRITICAL: Blackboard Type Constraints

### The JSON Unmarshal Constraint

**This is a hard architectural constraint.** The BT blackboard may ONLY support types that `encoding/json` unmarshals to by default when you unmarshal any valid JSON to a destination of type `any`.

This constraint exists because:

1. **Go planner interfaces** (like `pabt.State[T]`) use `any` type for blackboard values
2. **JSON unmarshal to `any`** has well-defined, limited type mappings
3. **Type safety** requires predictable behavior across the Go-JavaScript boundary
4. **Performance** avoids reflection overhead for custom types

### Supported Types (JSON-Compatible)

These types are explicitly permitted in the blackboard:

| Go Type              | JSON Type | Example in Blackboard       |
|----------------------|-----------|------------------------------|
| `bool`               | boolean   | `bb.set('isVisible', true)` |
| `float64`            | number    | `bb.set('x', 15.5)`          |
| `int64` (via float64)| number    | `bb.set('count', 42)`        |
| `string`             | string    | `bb.set('name', 'robot1')`   |
| `[]interface{}`      | array     | `bb.set('items', [1,2,3])`  |
| `map[string]interface{}` | object | `bb.set('pos', {x:10,y:20})` |

### Forbidden Types

These types are **explicitly prohibited** in the blackboard:

| Forbidden Type          | Reason                             |
|-------------------------|------------------------------------|
| Custom structs (e.g., `*Actor`) | Not JSON-unmarshalable to `any` |
| Pointers to custom types | Same as above                     |
| Interface types (other than `any`) | Ambiguous type resolution      |
| `time.Time`             | JSON requires custom encoding     |
| `chan`                  | Not serializable                  |
| `func`                  | Not serializable                  |
| `[]byte`                | Only []any allowed to support semantics closer to JS array |

### Data Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                        JavaScript World                              │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │    Full State (Maps, Objects, Complex Types)                     │  │
│  │                                                                  │  │
│  │  state.models.actors.get(1) = {                                  │  │
│  │    id: 1,                                                        │  │
│  │    x: 5,                                                         │  │
│  │    y: 10,                                                        │  │
│  │    heldItem: Cube {id: 1, x: 15, ...}                          │  │
│  │  }                                                               │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│                           │                                         │
│                           │ syncToBlackboards() ◄──────────────┐    │
│                           │ Extract ONLY primitives             │    │
│                           ▼                                    │    │
└───────────────────────────┼────────────────────────────────────┼────┘
                            │                                    │
┌───────────────────────────┼────────────────────────────────────┼────────┐
│  Blackboard Boundary ─────┼────────────────────────────────────┼────────│
│  (JSON Types Only) │                             │ │        │
└───────────────────────────┼────────────────────────────────────┼────────┘
                            │                                    │
                            │ READ-ONLY for Go Planner           │
                            ▼                                    │
┌───────────────────────────────────────────┐                    │
│           Go World - PA-BT Planner        │                    │
├───────────────────────────────────────────┤                    │
│  state.Variable('actorX') → 5             │                    │
│  state.Variable('actorY') → 10            │                    │
│  state.Variable('heldItemExists') → true  │                    │
│                                           │                    │
│  ✓ Reads primitives via pabt.State API   │                    │
│  ✗ Cannot access full Actor struct       │                    │
│  ✗ Cannot inspect heldItem complex obj   │                    │
└───────────────────────────────────────────┘                    │
                            │                                        │
                            │ Selected action executes              │
                            │──────────────────────────────────────┘
                            │
                    ┌───────▼────────────────────────────────────────┐
                    │          JavaScript World (Execution)          │
                    │  Action.node() → bt.Node executes            │
                    │  Direct closure access to state.models.actors │
                    │  mutate.actor.heldItem = cube                 │
                    │  mutate.cube.deleted = true                    │
                    └─────────────────────────────────────────────────┘

    ┌────────────────────────────────────────────────────────────────┐
    │                       ⚠️ PROHIBITED ⚠️                          │
    │                                                                  │
    │  DO NOT write this:                                              │
    │    bb.set('actor', {x: 5, y: 10, heldItem: {...}})  // NO!      │
    │                                                                  │
    │  DO NOT do this:                                                 │
    │    syncFromBlackboards()  // NO REVERSE SYNC!                   │
    │                                                                  │
    │  The blackboard is WRITE-ONLY from JS perspective.             │
    │  The Go planner is READ-ONLY from blackboard.                   │
    │  NO reverse flow of data.                                       │
    └────────────────────────────────────────────────────────────────┘
```

### Implementation Examples

#### ✅ CORRECT: Primitive Extraction

```javascript
// JavaScript - full state
const actor = state.models.actors.get(1);
const cube = state.models.cubes.get(1);

// syncToBlackboards - extract primitives ONLY
function syncToBlackboards(state) {
    const bb = state.pabt.blackboard;
    const actor = state.models.actors.get(1);
    const cube = state.models.cubes.get(1);

    // ✅ CORRECT: Primitive types only
    bb.set('actorX', actor.x);
    bb.set('actorY', actor.y);
    bb.set('heldItemExists', actor.heldItem !== null);
    bb.set('cubeDeleted', cube.deleted);

    // ✅ CORRECT: Simple arrays of primitives
    const cubeCoords = [cube.x, cube.y];
    bb.set('cubeCoords', cubeCoords);
}
```

#### ❌ WRONG: Complex Objects

```javascript
// ❌ WRONG: Entire complex object
const cube = state.models.cubes.get(1);
bb.set('cube1', cube);

// ❌ WRONG: Nested object with struct references
bb.set('actorWithItem', {
    actor: {id: 1, x: 5, y: 10},
    heldItem: cube  // References a complex struct!
});

// ❌ WRONG: Array of complex objects
const allCubes = Array.from(state.models.cubes.values());
bb.set('allCubes', allCubes);  // Arrays of structs!
```

#### ❌ WRONG: Reverse Sync

```javascript
// ❌ WRONG: Attempting sync from blackboard to JS state
function syncFromBlackboards(state) {
    const bb = state.pabt.blackboard;

    // ❌ This doesn't work! Blackboard is write-only from JS
    if (bb.has('newActorX')) {
        const newX = bb.get('newActorX');  // Was never set by Go!
        state.models.actors.get(1).x = newX;  // ❌ UNREACHABLE
        bb.delete('newActorX');
    }
}
```

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

// ❌ CRITICAL: This violates the JSON-unmarshal constraint!
// The object {x: 10, y: 15, deleted: false} becomes map[string]interface{}
// when type is `any`, but custom struct references won't work.
```

**Correct**:

```javascript
// CORRECT: Extract minimal planning primitives (JSON-compatible ONLY!)
state.models.cubes.get(1) // Full object in JS (for rendering, collision, etc.)
bb.set('cube_1_x', 10)       // Primitives for Go planner (float64)
bb.set('cube_1_y', 15)       // Primitives for Go planner (float64)
bb.set('cube_1_deleted', false)  // Primitives for Go planner (bool)

// ✅ These are ALL JSON-compatible types when unmarshaling to `any`
// ✅ Go planner can read them via pabt.State.Variable() safely
// ✅ No type ambiguity, no reflection overhead
```

### ❌ Using Custom Structs

```javascript
// ❌ WRONG: Attempting to put a custom struct in blackboard
// This is BLOCKED by the JSON-unmarshal constraint!

// Even if you could marshal it...
const serialized = JSON.stringify(actor);
bb.set('actor1', serialized);

// The Go planner receives a string, not a struct
// It cannot inspect fields, cannot modify, cannot use effectively
```

**Correct**:

```javascript
// ✅ CORRECT: Extract only the fields the planner needs
bb.set('actor1_x', actor.x);      // float64
bb.set('actor1_y', actor.y);      // float64
bb.set('actor1_heldId', actor.heldItem ? actor.heldItem.id : null);  // number or null

// The rest of the struct remains in JS for full feature access
```

### ❌ Using Time Types

```javascript
// ❌ WRONG: time.Time is not JSON-compatible
import * as time from 'osm:time';  // Hypothetical module
bb.set('spawnTime', time.now());

// Go side: receives string or number, not time.Time
// Cannot perform time arithmetic in Go planner
```

**Correct**:

```javascript
// ✅ CORRECT: Store as numbers (milliseconds or Unix timestamp)
bb.set('spawnTimeMs', Date.now());
bb.set('spawnTimeUnix', Math.floor(Date.now() / 1000));

// Go planner can compare numbers: Variable('spawnTimeMs') > threshold
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

- [PA-BT integration example](../../scripts/example-05-pick-and-place.js)
- [Pure JavaScript behavior tree example](../../scripts/example-04-bt-shooter.js)
- [osm:pabt module reference](./pabt.md)
- [bt.Blackboard implementation](../../internal/builtin/bt/blackboard.go)
- [bt Bridge API](../../internal/builtin/bt/bridge.go)

---

## Appendix: PA-BT Condition Evaluation Modes

The `osm:pabt` module supports two evaluation modes for conditions:

### JSCondition (Default)

JavaScript conditions evaluated via Goja runtime:

```javascript
// Create action with JavaScript condition
const pickAction = pabt.newAction('pick', [
    {
        key: 'heldItemExists',
        Match: function(value) {
            return value === false;  // JavaScript evaluation
        }
    }
], [...]);
```

**Characteristics:**
- Full JavaScript expressiveness
- Closure access to surrounding scope
- Evaluated on Go event loop via `RunOnLoopSync`
- ~50-100x slower than ExprCondition (still fast enough for most use cases)

### ExprCondition (Fast Path)

Go-native conditions using [expr-lang](https://github.com/antonmedv/expr):

```javascript
// Create action with expr-lang condition (Go-native, ZERO Goja calls)
const moveAction = pabt.newAction('move', [
    pabt.newExprCondition('actorX', 'Value < 50 && Value > 0')
], [...]);
```

**Characteristics:**
- Go-native bytecode compilation and execution
- **ZERO Goja calls during Match()**
- 10-100x faster than JSCondition
- Expression syntax: `Value` refers to the condition input value
- Limited to expr-lang supported operations (no closure access)

### When to Use Each

| Scenario | Recommended Mode |
|----------|-----------------|
| Complex logic with closures | JSCondition |
| Performance-critical planning | ExprCondition |
| String manipulation | JSCondition |
| Numeric comparisons | ExprCondition |
| Access to JS state | JSCondition |
| Simple boolean logic | ExprCondition |

### Memory Safety

Both modes are designed to avoid circular reference traps:

- **JSCondition**: Condition values are passed as primitives through blackboard (JSON-compatible types only)
- **ExprCondition**: Pure Go evaluation, no Goja runtime references held
- **FuncCondition**: Native Go functions, no JavaScript involvement
