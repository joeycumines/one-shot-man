---
title: osm:pabt Module Reference
description: PA-BT (Planning-Augmented Behavior Tree) integration for osm
tags:
  - pabt
  - planning
  - behavior tree
  - action templates
---

# osm:pabt Module Reference

The `osm:pabt` module provides a Planning-Augmented Behavior Tree (PA-BT) implementation for osm. It enables goal-directed planning using declarative action templates integrated with reactive behavior tree execution.

## Table of Contents

- [What is PA-BT?](#what-is-pa-bt)
- [Action Templates: Core Concept](#action-templates-core-concept)
- [Quick Start](#quick-start)
- [Architecture](#architecture)
- [API Reference](#api-reference)
- [Advanced Patterns](#advanced-patterns)
- [Performance](#performance)
- [Troubleshooting](#troubleshooting)

---

## What is PA-BT?

**PA-BT (Planning-Augmented Behavior Trees)** combines two powerful AI approaches:

1. **Behavior Trees (BTs)**: Reactive execution model that responds to changing conditions
2. **GOAP (Goal-Oriented Action Planning)**: Declarative planning that constructs action sequences to achieve goals

Unlike traditional BTs where you manually construct the tree structure, PA-BT uses **action templates** to automatically build execution plans at runtime based on current state and desired goals.

### Key Benefits

- **Declarative**: Specify WHAT actions do (effects), not WHEN to use them
- **Automatic Planning**: Planner constructs action sequences to achieve goals
- **Reactive**: Plans adapt when preconditions change or unexpected events occur
- **Composable**: Action templates are reusable building blocks

---

## Action Templates: Core Concept

An **action template** is a declarative specification of what an action achieves, independent of when or how it will be used in the plan.

### Template Structure

Each action template has three components:

```javascript
pabt.newAction(
    'Pick',                                          // 1. Name (for debugging)
    [{key: 'atCube', Match: v => v === true}],      // 2. Preconditions
    [{key: 'heldItem', Value: 1}],                  // 3. Effects
    bt.createLeafNode(() => { /* execute */ })      // 4. Execution node
)
```

| Component | Purpose | Example |
|-----------|---------|---------|
| **Name** | Identifies the action for debugging | `'Pick'`, `'MoveTo_42'` |
| **Preconditions** | What must be true BEFORE executing | `atCube === true` |
| **Effects** | What becomes true AFTER executing | `heldItem === 1` |
| **Node** | BT node that implements the action | `bt.createLeafNode(...)` |

### How Planning Works

When you specify a goal like `{key: 'heldItem', Match: v => v === 1}`:

1. **Goal Check**: Is `heldItem === 1` true? If not, planning begins
2. **Find Actions**: Search for actions whose **effects** satisfy the goal (e.g., `Pick` has effect `heldItem: 1`)
3. **Check Preconditions**: Does `Pick` have preconditions? (`atCube === true`)
4. **Recursive Planning**: If preconditions fail, find actions to satisfy them (e.g., `MoveTo` makes `atCube === true`)
5. **Construct Plan**: Build action sequence: `[MoveTo → Pick]`
6. **Execute**: Run the plan via BT ticker, replanning when conditions change

### Example: Pick-and-Place

**Goal**: Deliver cube to goal (`cubeDeliveredAtGoal === true`)

**Action Templates**:
```javascript
// MoveTo: Makes actor adjacent to entity
MoveTo(entityId):
    Preconditions: [reachable_cube_<id> === true]
    Effects:       [atEntity_<id> === true]

// Pick: Picks up item at location
Pick:
    Preconditions: [atCube === true, handsEmpty === true]
    Effects:       [heldItem === cubeId]

// Deliver: Places item at goal
Deliver:
    Preconditions: [atGoal === true, heldItem === cubeId]
    Effects:       [cubeDeliveredAtGoal === true]
```

**Automatic Plan Construction**:
```
Goal: cubeDeliveredAtGoal === true
  ↓ Find action with matching effect
Deliver (effect satisfies goal)
  ↓ Check preconditions
Precondition: heldItem === cubeId (FALSE)
  ↓ Find action with matching effect
Pick (effect: heldItem === cubeId)
  ↓ Check preconditions
Precondition: atCube === true (FALSE)
  ↓ Find action with matching effect
MoveTo(cube) (effect: atEntity_cube === true)
  ↓ All preconditions satisfied
PLAN: [MoveTo(cube) → Pick → MoveTo(goal) → Deliver]
```

---

## Quick Start

```javascript
const bt = require('osm:bt');
const pabt = require('osm:pabt');

// 1. Create blackboard and PA-BT state
const bb = new bt.Blackboard();
const state = pabt.newState(bb);

// 2. Sync application state to blackboard (primitives only!)
function syncToBlackboard() {
    bb.set('actorX', actor.x);
    bb.set('actorY', actor.y);
    bb.set('atCube', Math.abs(actor.x - cube.x) < 2);
    bb.set('handsEmpty', actor.heldItem === null);
}

// 3. Register static action templates
state.registerAction('Pick', pabt.newAction('Pick',
    [{key: 'atCube', Match: v => v === true}],   // preconditions
    [{key: 'heldItem', Value: 1}],                // effects
    bt.createLeafNode(() => {
        cube.deleted = true;
        actor.heldItem = {id: 1};
        return bt.success;
    })
));

// 4. Create plan with goal
const plan = pabt.newPlan(state, [
    {key: 'heldItem', Match: v => v === 1}
]);

// 5. Execute with BT ticker
const ticker = bt.newTicker(100, plan.Node());

// 6. Game loop
function tick() {
    syncToBlackboard();  // MUST sync before each tick
    ticker.tick();
}
```

---

## Architecture

### Layered Design

```
┌──────────────────────────────────────────────────────────────┐
│                   JavaScript Layer                            │
│  ┌────────────────┐     ┌─────────────────────────────────┐  │
│  │ Application    │     │ Action Templates                │  │
│  │ State          │────▶│ • Pick, Place, MoveTo           │  │
│  │ (Actor, Cube)  │     │ • Defined in JavaScript         │  │
│  └────────────────┘     │ • Closures over JS state        │  │
│         │               └─────────────────────────────────┘  │
│         │ syncToBlackboard()                                 │
│         ▼                                                     │
│  ┌──────────────────────────────────────────────────────┐    │
│  │           bt.Blackboard (primitives only)            │    │
│  │  • actorX: 10, atCube: true, heldItem: null         │    │
│  └──────────────────────────────────────────────────────┘    │
│         │                                                     │
│         ▼                                                     │
│  ┌──────────────────────────────────────────────────────┐    │
│  │              pabt.State (wraps blackboard)           │    │
│  │  • Variable(key) - read from blackboard             │    │
│  │  • Actions(failed) - find relevant templates        │    │
│  └──────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────┘
                          │
                          ▼
┌──────────────────────────────────────────────────────────────┐
│                      Go Layer                                 │
│  ┌──────────────────────────────────────────────────────┐    │
│  │              go-pabt Planning Algorithm              │    │
│  │  • Reads primitives from blackboard                 │    │
│  │  • Evaluates conditions (Match functions)           │    │
│  │  • Constructs action sequence                       │    │
│  │  • Returns bt.Node for execution                    │    │
│  └──────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────┘
```

### Key Principles

1. **One-way sync**: JavaScript → Blackboard (no reverse)
2. **Primitives only**: Blackboard stores numbers, strings, booleans
3. **JavaScript execution**: Action nodes run in JS, mutate JS state
4. **Go planning**: Planner reads primitives, builds action sequences

### Why This Design?

- **Separation of concerns**: Application logic (JS) vs planning (Go)
- **Performance**: Planning in compiled Go, execution in flexible JS
- **Type safety**: Primitives bridge the language boundary cleanly
- **Composability**: Same planning algorithm for any application

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
- `registerAction(name, action)` - Register a static action template
- `setActionGenerator(fn)` - Set parametric action generator
- `get(key)` / `set(key, value)` - Direct blackboard access

---

### `pabt.newAction(name, conditions, effects, node)`

Creates a PA-BT action template.

```javascript
const action = pabt.newAction(
    'MoveTo',
    [
        {key: 'reachable', Match: v => v === true}
    ],
    [
        {key: 'atTarget', Value: true}
    ],
    bt.createLeafNode(() => {
        // Move actor toward target...
        return bt.running;
    })
);
```

**Parameters:**
- `name` (string) - Unique action identifier (for debugging)
- `conditions` (array) - Preconditions as `{key, Match}` objects
- `effects` (array) - Effects as `{key, Value}` objects
- `node` (bt.Node) - Behavior tree node for execution

**Condition Format:**
```javascript
{
    key: 'stateVariable',              // Blackboard key
    Match: function(value) {            // Match function
        return value === expectedValue;
    }
}
```

**Effect Format:**
```javascript
{
    key: 'stateVariable',    // Blackboard key
    Value: expectedValue     // Value action achieves (for planning only!)
}
```

**Returns:** `Action` object implementing `pabt.IAction` interface

**Important**: The `Value` in effects is used ONLY for planning. The actual state change happens in the `node` execution!

---

### `pabt.newPlan(state, goalConditions)`

Creates a PA-BT plan with goal conditions.

```javascript
const plan = pabt.newPlan(state, [
    {key: 'targetDelivered', Match: v => v === true}
]);
```

**Parameters:**
- `state` - A `pabt.State` object (from `pabt.newState`)
- `goalConditions` - Array of conditions representing success

**Returns:** `Plan` object with methods:
- `Node()` - Get the root `bt.Node` for execution (uppercase N!)

**Multiple Goals (OR logic):**
```javascript
// Success if EITHER condition is true
const plan = pabt.newPlan(state, [
    {key: 'locationA', Match: v => v === true},
    {key: 'locationB', Match: v => v === true}
]);
```

---

### `state.registerAction(name, action)`

Registers a static action template.

```javascript
const pickAction = pabt.newAction('Pick', conditions, effects, node);
state.registerAction('Pick', pickAction);
```

**When to use:**
- Fixed, finite set of actions
- Actions known at initialization

**Alias:** `state.RegisterAction` (uppercase)

---

### `state.setActionGenerator(generator)`

Sets a dynamic action generator for **parametric actions**.

```javascript
state.setActionGenerator(function(failedCondition) {
    const key = failedCondition.key;
    
    // Pattern match on condition key
    if (key && key.startsWith('atEntity_')) {
        const entityId = parseInt(key.replace('atEntity_', ''));
        // Generate MoveTo action for this specific entity
        return [createMoveToAction(entityId)];
    }
    
    return [];  // No actions for this condition
});
```

**Parameters:**
- `generator` (function) - Called when planning needs actions
  - Receives: `failedCondition` (object with `key` and `Match` method)
  - Returns: Array of actions (or empty array)

**When to use:**
- Infinite or very large action spaces
- Actions parameterized by runtime data (e.g., entity IDs)
- TRUE parametric actions like `MoveTo(entityId)`

**Performance Note:** Generator is called during planning. Cache created actions to avoid recreation.

**Alias:** `state.SetActionGenerator` (uppercase)

**Thread Safety:** If generator accesses JavaScript state, it must use `Bridge.RunOnLoopSync` (handled automatically in `osm:pabt`).

---

### `pabt.newExprCondition(key, expression)`

Creates a Go-native condition using expr-lang (fast path).

```javascript
// Instead of:
{key: 'distance', Match: v => v < 50}

// Use:
pabt.newExprCondition('distance', 'Value < 50')
```

**Parameters:**
- `key` - Blackboard key
- `expression` - expr-lang expression string

**Expression Environment:**
- `Value` - The input value from blackboard

**Expression Examples:**
```javascript
// Equality
pabt.newExprCondition('status', 'Value == "ready"')

// Comparison
pabt.newExprCondition('distance', 'Value < 50 && Value > 0')

// Null check
pabt.newExprCondition('item', 'Value != nil')

// Length check
pabt.newExprCondition('items', 'len(Value) > 0')
```

**Performance:** 10-100x faster than JavaScript conditions (~100ns vs ~5μs)

**When to use:**
- Simple comparisons
- Arithmetic expressions
- Performance-critical conditions

**When NOT to use:**
- Complex JavaScript logic
- Conditions requiring closure state
- Conditions referencing multiple blackboard keys

---

## Advanced Patterns

### Pattern 1: Static + Parametric Actions

Combine static actions for fixed operations with parametric actions for dynamic targets.

```javascript
// STATIC: Fixed actions (Pick, Place)
state.registerAction('Pick', pabt.newAction('Pick',
    [{key: 'atCube', Match: v => v === true}],
    [{key: 'heldItem', Value: cubeId}],
    pickNode
));

// PARAMETRIC: MoveTo any entity
state.setActionGenerator(function(failedCondition) {
    const key = failedCondition.key;
    
    // Generate MoveTo actions on-demand
    if (key.startsWith('atEntity_')) {
        const id = parseInt(key.replace('atEntity_', ''));
        return [createMoveToAction(id)];
    }
    
    return [];
});
```

**Why this works:**
- Pick/Place actions don't need parameters (1 target)
- MoveTo needs parameters (many possible targets)
- Generator creates MoveTo actions dynamically based on which entity planner needs

---

### Pattern 2: Heuristic Effects for Planning

Use **heuristic effects** to guide the planner without strict guarantees.

```javascript
// Picking a blockade cube MIGHT make target reachable
// (depends on which blockade, world layout, etc.)
// But we can use it as a HEURISTIC for planning
state.registerAction('PickBlockade_1', pabt.newAction('PickBlockade_1',
    [{key: 'atEntity_1', Match: v => v === true}],
    [
        {key: 'heldItem', Value: 1},
        // HEURISTIC: Suggest this helps reach target
        {key: 'reachable_target', Value: true}
    ],
    pickNode
));
```

**When to use:**
- Exact effects are hard to compute statically
- Effect depends on complex world state
- Want to bias planner toward certain actions

**Trade-off:** May construct plans that don't work; rely on replanning when preconditions fail

---

### Pattern 3: Conflict Resolution via Staging

Use intermediate goals to resolve conflicts (e.g., "hands full but need to pick up obstacle").

```javascript
// Problem: Goal blocked, holding target, need to clear obstacle
// Solution: Staging area + temporary placement

// Place target temporarily to free hands
state.registerAction('PlaceTemporary', pabt.newAction('PlaceTemporary',
    [{key: 'heldItem', Match: v => v === targetId}],
    [{key: 'heldItem', Value: null}],  // Frees hands
    placeAtStagingNode
));

// Now hands free → can pick obstacle → can clear path → can retrieve target
```

**Planning sequence:**
1. Goal blocked, holding target
2. Planner needs hands free to pick obstacle
3. Finds `PlaceTemporary` (effect: hands free)
4. After placing: picks obstacle, clears path, retrieves target, delivers

---

### Pattern 4: Blackboard Sync Strategy

Efficient synchronization between JS state and blackboard.

```javascript
function syncToBlackboard(state) {
    const bb = state.blackboard;
    const actor = state.actors.get(1);
    
    // ALWAYS sync: Core state
    bb.set('actorX', actor.x);
    bb.set('actorY', actor.y);
    bb.set('heldItem', actor.heldItem ? actor.heldItem.id : null);
    
    // CONDITIONALLY sync: Expensive computations
    // Only sync reachability if planner might need it
    if (!state.plan || !state.plan.Running()) {
        // Not in middle of action execution → might be planning
        entities.forEach(entity => {
            const reachable = isReachable(actor, entity);
            bb.set(`reachable_${entity.id}`, reachable);
        });
    }
}
```

**Best practices:**
- Sync primitives only (numbers, strings, booleans)
- Sync derived state (distances, reachability) not raw positions
- Batch updates in single sync function
- Call sync BEFORE each ticker tick

---

### Pattern 5: Action Caching for Parametric Actions

Cache dynamically created actions to avoid recreation overhead.

```javascript
const actionCache = new Map();

function createMoveToAction(entityId) {
    const cacheKey = `MoveTo_${entityId}`;
    
    // Return cached if exists
    if (actionCache.has(cacheKey)) {
        return actionCache.get(cacheKey);
    }
    
    // Create new action
    const action = pabt.newAction(
        cacheKey,
        [{key: `reachable_${entityId}`, Match: v => v === true}],
        [{key: `atEntity_${entityId}`, Value: true}],
        createExecutionNode(entityId)
    );
    
    // Cache for future use
    actionCache.set(cacheKey, action);
    return action;
}

state.setActionGenerator(function(failedCondition) {
    const key = failedCondition.key;
    if (key.startsWith('atEntity_')) {
        const id = parseInt(key.replace('atEntity_', ''));
        return [createMoveToAction(id)];  // Uses cache
    }
    return [];
});
```

---

## Performance

### Benchmarks (Approximate)

| Operation | Time | Notes |
|-----------|------|-------|
| State.Variable() | ~500ns | Blackboard read with mutex |
| JSCondition.Match() | ~5μs | JavaScript evaluation + thread marshalling |
| ExprCondition.Match() | ~100ns | Go-native compiled expression |
| FuncCondition.Match() | ~50ns | Direct Go function call |
| Plan.Node() tick | ~10μs | Depends on action count and complexity |

### Optimization Strategies

1. **Use ExprCondition for hot paths**
   ```javascript
   // Bad: JavaScript condition (slow)
   {key: 'distance', Match: v => v < 100}
   
   // Good: Expr condition (fast)
   pabt.newExprCondition('distance', 'Value < 100')
   ```

2. **Minimize blackboard keys**
   ```javascript
   // Bad: Sync everything
   bb.set('actor_x', actor.x);
   bb.set('actor_y', actor.y);
   bb.set('cube_x', cube.x);
   bb.set('cube_y', cube.y);
   bb.set('distance', Math.sqrt(...));  // Derived
   
   // Good: Sync only what planner needs
   bb.set('distance', computeDistance(actor, cube));
   ```

3. **Cache parametric actions**
   - Don't recreate actions every time generator is called
   - Use Map to cache by entity ID or parameters

4. **Batch blackboard updates**
   - Single `syncToBlackboard()` function per tick
   - Avoid scattered `bb.set()` calls throughout code

5. **Lazy sync expensive computations**
   - Check `plan.Running()` to detect planning phase
   - Only compute reachability, pathfinding during planning

---

## Troubleshooting

### Plan doesn't find any actions

**Symptoms:** Plan immediately fails with no actions selected

**Causes:**
1. Actions not registered
2. Blackboard missing required keys
3. Preconditions never satisfiable

**Debug:**
```javascript
// Check action registration
console.log('Registered actions:', state.actions.All().length);

// Enable PA-BT debugging
// Set environment variable: OSM_DEBUG_PABT=1

// Check blackboard state
console.log('atCube:', bb.get('atCube'));
console.log('handsEmpty:', bb.get('handsEmpty'));
```

**Fix:**
- Ensure `state.registerAction()` called for static actions
- Verify `setActionGenerator()` returns non-empty array
- Check blackboard has all keys referenced in preconditions

---

### Plan executes but state doesn't change

**Symptoms:** Action nodes run but application state unchanged

**Causes:**
1. Node doesn't mutate JavaScript state
2. syncToBlackboard not called after execution
3. Node returns wrong status

**Debug:**
```javascript
const action = pabt.newAction('Pick',
    conditions,
    effects,
    bt.createLeafNode(() => {
        console.log('Pick executing!');
        console.log('Before:', actor.heldItem);
        
        // Actually mutate state
        actor.heldItem = {id: 1};
        
        console.log('After:', actor.heldItem);
        return bt.success;  // Must return correct status!
    })
);
```

**Fix:**
- Action node MUST mutate JavaScript state (not blackboard directly)
- Call `syncToBlackboard()` BEFORE each tick
- Ensure node returns `bt.success`, `bt.failure`, or `bt.running`

---

### Plan replans constantly (thrashing)

**Symptoms:** Planner keeps restarting, never makes progress

**Causes:**
1. Effects don't match actual state changes
2. Preconditions fail unexpectedly during execution
3. Goal condition never satisfied

**Debug:**
```javascript
// Check if effects match reality
state.registerAction('Pick', pabt.newAction('Pick',
    conditions,
    [{key: 'heldItem', Value: 1}],  // EFFECT says this
    bt.createLeafNode(() => {
        actor.heldItem = {id: 1};   // CODE must do this!
        return bt.success;
    })
));

// Check goal is achievable
const plan = pabt.newPlan(state, [
    {key: 'heldItem', Match: v => {
        console.log('Goal check: heldItem =', v);
        return v === 1;
    }}
]);
```

**Fix:**
- Effects MUST accurately describe state changes
- Node implementation MUST match declared effects
- Goal condition MUST be satisfiable by some action chain

---

### ActionGenerator not called

**Symptoms:** Parametric actions not generated

**Causes:**
1. Generator not set
2. Condition key doesn't match generator pattern
3. Static action already satisfies condition

**Debug:**
```javascript
state.setActionGenerator(function(failedCondition) {
    console.log('Generator called! Key:', failedCondition.key);
    
    const key = failedCondition.key;
    if (key.startsWith('atEntity_')) {
        console.log('Generating MoveTo for:', key);
        const id = parseInt(key.replace('atEntity_', ''));
        return [createMoveToAction(id)];
    }
    
    console.log('No match for key:', key);
    return [];
});
```

**Fix:**
- Verify generator is set: `state.setActionGenerator(...)`
- Check condition keys match generator pattern matching
- Ensure no static action already provides the effect

---

## See Also

- [bt-blackboard-usage.md](./bt-blackboard-usage.md) - Blackboard patterns
- [go-pabt reference](https://github.com/joeycumines/go-pabt) - Underlying algorithm
- [example-05-pick-and-place.js](../../scripts/example-05-pick-and-place.js) - Complete example
- [PA-BT Paper](https://github.com/joeycumines/go-pabt#references) - Original research
