---
title: osm:pabt Module Reference
description: PA-BT (Planning-Augmented Behavior Tree) integration for osm
tags:
  - pabt
  - planning
  - behavior tree
  - goja
---

# osm:pabt Module Reference

The `osm:pabt` module provides a Planning-Augmented Behavior Tree (PA-BT) implementation for osm. It enables goal-directed planning integrated with reactive behavior tree execution.

## Quick Start

```javascript
const bt = require('osm:bt');
const pabt = require('osm:pabt');

// 1. Create blackboard and PA-BT state
const bb = new bt.Blackboard();
const state = pabt.newState(bb);

// 2. Register actions
const moveAction = pabt.newAction('move', 
    [{key: 'atGoal', Match: v => v === false}],  // Preconditions
    [{key: 'atGoal', Value: true}],               // Effects
    bt.createLeafNode(() => { /* execute */ return bt.success; })
);
state.registerAction(moveAction);

// 3. Create plan with goal conditions
const plan = pabt.newPlan(state, [
    {key: 'atGoal', Match: v => v === true}
]);

// 4. Run with BT ticker
const ticker = bt.newTicker(100, plan.Node());
```

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         JavaScript Layer                             │
│  ┌───────────────────┐    ┌───────────────────┐                     │
│  │ Application Types │    │ Action Definitions│                     │
│  │ (Actor, Cube, etc)│◄──►│ (move, pick, etc) │                     │
│  └───────────────────┘    └───────────────────┘                     │
│           │                        │                                 │
│           │ syncToBlackboard()     │ node execution                 │
│           ▼                        ▼                                 │
│  ┌───────────────────┐    ┌───────────────────┐                     │
│  │   bt.Blackboard   │◄──►│   pabt.State      │                     │
│  │ (primitives only) │    │ (wraps blackboard)│                     │
│  └───────────────────┘    └───────────────────┘                     │
│                                    │                                 │
│                                    │ pabt.newPlan()                 │
│                                    ▼                                 │
│                           ┌───────────────────┐                     │
│                           │   pabt.Plan       │                     │
│                           │ (go-pabt planner) │                     │
│                           └───────────────────┘                     │
│                                    │                                 │
│                                    │ plan.Node()                    │
│                                    ▼                                 │
│                           ┌───────────────────┐                     │
│                           │   bt.Ticker       │                     │
│                           │ (reactive loop)   │                     │
│                           └───────────────────┘                     │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                            Go Layer                                  │
│  ┌───────────────────┐    ┌───────────────────┐                     │
│  │ pabt.State        │    │ pabt.Action       │                     │
│  │ (IState interface)│    │ (IAction interf.) │                     │
│  └───────────────────┘    └───────────────────┘                     │
│           │                        │                                 │
│           │ Variable()             │ Conditions(), Effects()        │
│           ▼                        ▼                                 │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │                      go-pabt Library                            │  │
│  │  github.com/joeycumines/go-pabt                                │  │
│  │  • pabt.INew[T] - planning algorithm                          │  │
│  │  • Condition.Match() evaluation                                │  │
│  │  • Effect application                                          │  │
│  └───────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

### Key Principles

1. **Application types in JavaScript only**: Shapes, sprites, simulation logic are NOT in Go
2. **Blackboard as planning bridge**: Go planner reads primitives from blackboard
3. **One-way sync**: JavaScript → Blackboard (no reverse sync)
4. **BT nodes execute in JavaScript**: Action node closures mutate JS state directly

---

## API Reference

### `pabt.newState(blackboard)`

Creates a PA-BT State wrapper around a `bt.Blackboard`.

```javascript
const bb = new bt.Blackboard();
const state = pabt.newState(bb);
```

**Parameters:**
- `blackboard` - A `bt.Blackboard` instance

**Returns:** `State` object with methods:
- `variable(key)` - Get value from blackboard
- `registerAction(action)` - Register an action for planning
- `actions(failedConditions)` - Get actions that could address failed conditions

---

### `pabt.newAction(name, conditions, effects, node?)`

Creates a PA-BT action with preconditions, effects, and optional execution node.

```javascript
const moveAction = pabt.newAction(
    'moveToCube',
    [  // Preconditions
        {key: 'heldItemExists', Match: v => v === false},
        {key: 'cubeDeleted', Match: v => v === false}
    ],
    [  // Effects (declared for planning)
        {key: 'atCube', Value: true}
    ],
    bt.createLeafNode(() => {  // Execution node (optional)
        // Move actor toward cube
        actor.x += Math.sign(cube.x - actor.x);
        return bt.running;
    })
);
```

**Parameters:**
- `name` - Unique action identifier (string)
- `conditions` - Array of condition objects with `{key, Match(value) => boolean}`
- `effects` - Array of effect objects with `{key, Value}`
- `node` (optional) - `bt.Node` to execute when action is selected

**Returns:** `Action` object implementing `pabt.IAction[any]` interface

---

### `pabt.newPlan(state, goalConditions)`

Creates a PA-BT plan with goal conditions.

```javascript
const plan = pabt.newPlan(state, [
    {key: 'cubeDeliveredAtGoal', Match: v => v === true}
]);
```

**Parameters:**
- `state` - A `pabt.State` object (from `pabt.newState`)
- `goalConditions` - Array of condition objects representing the goal

**Returns:** `Plan` object with methods:
- `Node()` - Get the root `bt.Node` for this plan (uppercase N!)

---

### `pabt.newExprCondition(key, expression)`

Creates a Go-native condition using expr-lang (fast path, ZERO Goja calls).

```javascript
const fastCondition = pabt.newExprCondition('actorX', 'Value < 50');
```

**Parameters:**
- `key` - Blackboard key to evaluate
- `expression` - expr-lang expression (use `Value` to reference input)

**Returns:** `Condition` object compatible with `pabt.newAction`

**Expression examples:**
- `'Value == true'` - Equality check
- `'Value < 50 && Value > 0'` - Range check
- `'Value != nil'` - Null check
- `'len(Value) > 0'` - Array/string length

---

### Status Constants

```javascript
pabt.StatusRunning  // bt.running
pabt.StatusSuccess  // bt.success
pabt.StatusFailure  // bt.failure
```

---

## Condition Evaluation Modes

The module supports three condition types:

### JSCondition (Default)

JavaScript functions evaluated via Goja runtime:

```javascript
{
    key: 'distance',
    Match: function(value) {
        return value < 10;
    }
}
```

### ExprCondition (Fast Path)

Go-native expr-lang evaluation (10-100x faster):

```javascript
pabt.newExprCondition('distance', 'Value < 10')
```

### FuncCondition (Go Native)

For Go-side conditions (used internally):

```go
// In Go code
FuncCondition{
    KeyVal: "distance",
    MatchFunc: func(v any) bool {
        if f, ok := v.(float64); ok {
            return f < 10
        }
        return false
    },
}
```

---

## Usage Patterns

### Pattern 1: Complete Pick-and-Place

```javascript
const bt = require('osm:bt');
const pabt = require('osm:pabt');

// Simulation state
const state = {
    actors: new Map([[1, {id: 1, x: 10, y: 12, heldItem: null}]]),
    cubes: new Map([[1, {id: 1, x: 25, y: 10, deleted: false}]]),
    goals: new Map([[1, {id: 1, x: 55, y: 12}]]),
    blackboard: new bt.Blackboard(),
    pabtState: null,
    plan: null
};

// Initialize PA-BT
state.pabtState = pabt.newState(state.blackboard);

// Sync function: JS → Blackboard
function syncToBlackboard(state) {
    const bb = state.blackboard;
    const actor = state.actors.get(1);
    const cube = state.cubes.get(1);
    
    bb.set('actorX', actor.x);
    bb.set('actorY', actor.y);
    bb.set('heldItemExists', actor.heldItem !== null);
    bb.set('cubeDeleted', cube.deleted);
    bb.set('cubeX', cube.x);
    bb.set('cubeY', cube.y);
}

// Define actions
const moveToCubeAction = pabt.newAction('moveToCube',
    [
        {key: 'cubeDeleted', Match: v => v === false},
        {key: 'heldItemExists', Match: v => v === false}
    ],
    [{key: 'atCube', Value: true}],
    bt.createLeafNode(() => {
        const actor = state.actors.get(1);
        const cube = state.cubes.get(1);
        actor.x += Math.sign(cube.x - actor.x);
        actor.y += Math.sign(cube.y - actor.y);
        return bt.running;
    })
);

const pickCubeAction = pabt.newAction('pickCube',
    [
        {key: 'atCube', Match: v => v === true},
        {key: 'heldItemExists', Match: v => v === false}
    ],
    [{key: 'heldItemExists', Value: true}],
    bt.createLeafNode(() => {
        const actor = state.actors.get(1);
        const cube = state.cubes.get(1);
        cube.deleted = true;
        actor.heldItem = cube;
        return bt.success;
    })
);

// Register actions
state.pabtState.registerAction(moveToCubeAction);
state.pabtState.registerAction(pickCubeAction);

// Create plan
state.plan = pabt.newPlan(state.pabtState, [
    {key: 'cubeDeliveredAtGoal', Match: v => v === true}
]);

// Run with ticker
const ticker = bt.newTicker(100, state.plan.Node());
```

### Pattern 2: Replanning on Unexpected Circumstances

The PA-BT algorithm automatically replans when:
1. An action's preconditions become false during execution
2. The goal conditions change
3. New actions become available

```javascript
// During simulation, if cube position changes unexpectedly:
function moveCubeRandomly(state) {
    const cube = state.cubes.get(1);
    cube.x = Math.random() * 50;
    cube.y = Math.random() * 20;
    // On next tick, PA-BT will detect the change via syncToBlackboard
    // and replan to reach the new position
}
```

---

## Thread Safety

- **bt.Blackboard**: Thread-safe (sync.RWMutex)
- **pabt.State**: Thread-safe for reads, single-writer for registerAction
- **pabt.ActionRegistry**: Thread-safe (sync.RWMutex)
- **Condition evaluation**: Depends on mode:
  - JSCondition: Evaluated on event loop goroutine
  - ExprCondition: Pure Go, thread-safe
  - FuncCondition: Pure Go, thread-safe

---

## Performance

### Benchmarks (approximate)

| Operation | Time |
|-----------|------|
| State.Variable() read | ~500ns |
| JSCondition.Match() | ~5μs |
| ExprCondition.Match() | ~100ns |
| FuncCondition.Match() | ~50ns |
| Plan.Node() tick | ~10μs |

### Recommendations

1. Use ExprCondition for performance-critical conditions
2. Keep blackboard keys minimal (sync only what planner needs)
3. Avoid complex objects in blackboard (primitives only)
4. Batch blackboard updates in syncToBlackboard()

---

## Troubleshooting

### Plan doesn't select any action

1. Check that actions are registered: `state.registerAction(action)`
2. Verify blackboard has expected values: `console.log(bb.get('key'))`
3. Ensure preconditions can be satisfied

### Action executes but state doesn't update

1. BT node must mutate JS state directly (not blackboard)
2. syncToBlackboard must be called each tick
3. Check that action's node returns correct status

### Performance issues

1. Switch to ExprCondition for hot conditions
2. Reduce sync frequency if acceptable
3. Minimize blackboard key count

---

## See Also

- [bt-blackboard-usage.md](./bt-blackboard-usage.md) - Blackboard patterns
- [go-pabt reference](https://github.com/joeycumines/go-pabt) - Underlying algorithm
- [example-05-pick-and-place.js](../../scripts/example-05-pick-and-place.js) - Complete example
