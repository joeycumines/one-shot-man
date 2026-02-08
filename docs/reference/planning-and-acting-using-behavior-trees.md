---
title: osm:pabt Module Reference
description: PA-BT (Planning and Acting using Behavior Trees) integration for osm
tags:
  - pabt
  - planning
  - behavior tree
  - action templates
---

# osm:pabt Module Reference

The `osm:pabt` module provides a **Planning and Acting using Behavior Trees (PA-BT)** implementation for osm. It enables the dynamic synthesis of behavior trees at runtime using a library of declarative action templates.

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

<p align="center" width="100%">
<video src="https://drive.google.com/file/d/1sxj5ZqjtonkthBG60iPc8o-DVN8RZ5Y0/preview" width="80%" controls></video>
</p>

**PA-BT (Planning and Acting using Behavior Trees)** is a formalism for autonomous agents that unifies deliberative planning with reactive execution. It is distinct from traditional GOAP (Goal-Oriented Action Planning) or linear planners.

Rather than generating a fixed sequence of actions, PA-BT uses **backchaining** to iteratively synthesize a **Behavior Tree (BT)**. This tree structure inherently handles failure and unexpected success (serendipity) without requiring a separate "re-planning" trigger.

### Key Concepts

1.  **Backchaining**: The algorithm starts with a Goal. If the Goal is not met, it searches for an action whose **Postcondition (Effect)** satisfies the Goal. It then treats that action's **Preconditions** as new sub-goals.
2.  **The PPA Pattern**: The fundamental building block of PA-BT is the **Postcondition-Precondition-Action (PPA)** subtree. 
    * Instead of just running an action, the tree asks: *"Is the goal already done?"*
    * Structure: `Fallback(Goal, Sequence(Preconditions, Action))`
    * This ensures **reactivity**: if the `Goal` becomes true externally (e.g., a human hands the robot the item), the `Fallback` succeeds immediately, and the expensive `Action` is skipped.
3.  **Lazy Planning**: The tree is expanded only as deep as necessary to find the first executable action, allowing for efficient real-time performance.

---

## Action Templates: Core Concept

An **Action Template** is the descriptive model of a capability. In PA-BT, this maps directly to the PPA structure: the planner uses the **Effects** (Postconditions) and **Preconditions** to wire the tree, while the **Node** contains the executable logic.

### Template Structure

Each action template has three components:

```javascript
pabt.newAction(
    'Pick',                                         // 1. Name (for debugging)
    [{key: 'atCube', match: v => v === true}],      // 2. Preconditions (Requirements)
    [{key: 'heldItem', value: 1}],                  // 3. Effects (Postconditions)
    bt.createLeafNode(() => { /* execute */ })      // 4. Execution node (The Action)
)
```

| Component | PA-BT Term | Purpose |
| --- | --- | --- |
| **Preconditions** | *Preconditions* | Conditions that must be `Success` for the Action to run. If these fail, they become new sub-goals for expansion. |
| **Effects** | *Postconditions* | The state guaranteed to be true after the Action succeeds. Used by the planner for backchaining. |
| **Node** | *Action Node* | The primitive behavior (Leaf) that executes the logic. |

### How Synthesis Works

When you specify a goal like `{key: 'heldItem', match: v => v === 1}`:

1. **Root Check**: The system creates a root `Fallback` node checking the goal.
2. **Expansion (Backchaining)**: If the goal fails, the algorithm searches the library for an Action Template with a matching **Effect** (`Pick` sets `heldItem: 1`).
3. **Grafting**: A PPA subtree is grafted into the tree: `Fallback(Goal, Sequence(Preconditions, Pick))`.
4. **Recursion**: The `Preconditions` of `Pick` (e.g., `atCube`) are now checked.
5. **Iterative Refinement**: If `atCube` fails, the planner pauses execution of `Pick`, searches for an action that achieves `atCube` (e.g., `MoveTo`), and expands the tree further.

This results in a functional Behavior Tree that is executed by the standard ticker.

### Example: Pick-and-Place

**Goal**: `cubeDeliveredAtGoal === true`

**Action Templates**:

```javascript
// MoveTo:
//  Preconditions: [reachable_cube_<id>]
//  Effects:       [atEntity_<id>]

// Pick:
//  Preconditions: [atCube, handsEmpty]
//  Effects:       [heldItem]

// Deliver:
//  Preconditions: [atGoal, heldItem]
//  Effects:       [cubeDeliveredAtGoal]
```

**Synthesized Tree Structure**:
The resulting execution logic is not a flat list, but a reactive tree:

```text
Fallback
├── Condition: cubeDeliveredAtGoal? (Success -> Done)
└── Sequence
    ├── Fallback (Expansion for Precondition: atGoal)
    │   ├── Condition: atGoal?
    │   └── Sequence
    │       ├── ... (MoveTo logic) ...
    │       └── Action: MoveTo(goal)
    ├── Fallback (Expansion for Precondition: heldItem)
    │   ├── Condition: heldItem == cubeId?
    │   └── Sequence
    │       ├── Fallback (Expansion for Precondition: atCube)
    │       │   ├── Condition: atCube?
    │       │   └── Sequence
    │       │       └── Action: MoveTo(cube)
    │       └── Action: Pick
    └── Action: Deliver
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
    [{key: 'atCube', match: v => v === true}],   // preconditions
    [{key: 'heldItem', value: 1}],               // effects (postconditions)
    bt.createLeafNode(() => {
        cube.deleted = true;
        actor.heldItem = {id: 1};
        return bt.success;
    })
));

// 4. Create plan with goal
const plan = pabt.newPlan(state, [
    {key: 'heldItem', match: v => v === 1}
]);

// 5. Execute with BT ticker (use lowercase node() - Node() is deprecated)
const ticker = bt.newTicker(100, plan.node());

// 6. Game loop
function tick() {
    syncToBlackboard();  // MUST sync before each tick
    ticker.tick();
}
```

---

## Architecture

### Layered Design

The module employs a hybrid architecture to separate the **Symbolic Reasoning** (Planning) from the **Execution** (Acting).

```
┌──────────────────────────────────────────────────────────────┐
│                      JavaScript Layer                        │
│  ┌────────────────┐      ┌─────────────────────────────────┐  │
│  │ Application    │      │ Action Templates                │  │
│  │ State          │────▶│ • Pick, Place, MoveTo           │  │
│  │ (Actor, Cube)  │      │ • Defined in JavaScript         │  │
│  │                │      │ • Closures over JS state        │  │
│  └────────────────┘      └─────────────────────────────────┘  │
│          │                                                  │
│          │ syncToBlackboard()                               │
│          ▼                                                  │
│  ┌──────────────────────────────────────────────────────┐    │
│  │            bt.Blackboard (primitives only)           │    │
│  │  • actorX: 10, atCube: true, heldItem: null          │    │
│  └──────────────────────────────────────────────────────┘    │
│          │                                                  │
│          ▼                                                  │
│  ┌──────────────────────────────────────────────────────┐    │
│  │              pabt.State (wraps blackboard)           │    │
│  │  • Variable(key) - read from blackboard              │    │
│  │  • Actions(failed) - find relevant templates         │    │
│  └──────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────┘
                          │
                          ▼
┌──────────────────────────────────────────────────────────────┐
│                      Go Layer                                │
│  ┌──────────────────────────────────────────────────────┐    │
│  │              go-pabt Synthesis Engine                │    │
│  │  • Reads primitives from blackboard                  │    │
│  │  • Evaluates conditions (match functions)            │    │
│  │  • Performs Backchaining & Tree Expansion            │    │
│  │  • Returns bt.Node for execution                     │    │
│  └──────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────┘
```

### Key Principles

1. **One-way sync**: JavaScript → Blackboard. The planner treats the blackboard as the ground truth for the "World State".
2. **Primitives only**: The blackboard stores symbolic state (numbers, strings, booleans) used for logic.
3. **JavaScript execution**: The *Action Nodes* execute in the JS environment and mutate the actual application state.
4. **Go planning**: The synthesis algorithm runs in compiled Go, rapidly expanding the BT structure based on the symbolic state.

---

## API Reference

### `pabt.newState(blackboard)`

Creates a PA-BT State wrapper around a `bt.Blackboard`.

```javascript
const bb = new bt.Blackboard();
const state = pabt.newState(bb);
```

**Parameters:**

* `blackboard` - A `bt.Blackboard` instance

**Returns:** `State` object with methods:

* `variable(key)` - Get value from blackboard
* `registerAction(name, action)` - Register a static action template
* `setActionGenerator(fn)` - Set parametric action generator
* `get(key)` / `set(key, value)` - Direct blackboard access

---

### `pabt.newAction(name, conditions, effects, node)`

Creates a PA-BT Action Template.

```javascript
const action = pabt.newAction(
    'MoveTo',
    [
        {key: 'reachable', match: v => v === true}
    ],
    [
        {key: 'atTarget', value: true}
    ],
    bt.createLeafNode(() => {
        // Move actor toward target...
        return bt.running;
    })
);
```

**Parameters:**

* `name` (string) - Unique action identifier (for debugging).
* `conditions` (array) - **Preconditions**. A list of `{key, match}` objects.
* `effects` (array) - **Postconditions**. A list of `{key, value}` objects.
* `node` (bt.Node) - The **Action Node** logic.

**Condition Format:**

```javascript
{
    key: 'stateVariable',             // Blackboard key
    match: function(value) {          // match function
        return value === expectedValue;
    }
}
```

**Effect Format:**

```javascript
{
    key: 'stateVariable',    // Blackboard key
    value: expectedValue     // Value action achieves (used for Backchaining)
}
```

**Returns:** `Action` object implementing `pabt.IAction` interface.

**Important**: The `value` in effects is used **only** for the synthesis (planning) phase. The planner assumes that if this action succeeds, this key will hold this value. The actual state change must be implemented within the `node`.

---

### `pabt.newPlan(state, goalConditions)`

Initializes a PA-BT plan structure rooted at the given Goal.

```javascript
const plan = pabt.newPlan(state, [
    {key: 'targetDelivered', match: v => v === true}
]);
```

**Parameters:**

* `state` - A `pabt.State` object (from `pabt.newState`)
* `goalConditions` - Array of conditions representing success

**Returns:** `Plan` object with methods:

* `Node()` - Get the root `bt.Node` for execution (uppercase N!)

**Multiple Goals (OR logic):**
PA-BT handles disjunctive goals naturally. If multiple conditions are provided, the root is a `Fallback` node that checks them in order.

```javascript
// Success if EITHER condition is true
const plan = pabt.newPlan(state, [
    {key: 'locationA', match: v => v === true},
    {key: 'locationB', match: v => v === true}
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

* Actions with fixed targets (e.g., "Reload", "Crouch").
* Actions where the parameters are known at initialization.

---

### `state.setActionGenerator(generator)`

Sets a dynamic action generator for **Parametric Actions**. This is the mechanism for "Lazy Planning" where actions are instantiated only when a specific precondition fails.

```javascript
state.setActionGenerator(function(failedCondition) {
    const key = failedCondition.key;

    // Pattern match on condition key
    if (key && key.startsWith('atEntity_')) {
        const entityId = parseInt(key.replace('atEntity_', ''));
        // Generate MoveTo action specifically for this entity
        return [createMoveToAction(entityId)];
    }

    return [];  // No actions for this condition
});
```

**Parameters:**

* `generator` (function) - Called during tree expansion.
* Receives: `failedCondition` (object with `key` and `match` method).
* Returns: Array of actions (or empty array).

**When to use:**

* Actions parameterized by runtime data (e.g., `MoveTo(x, y)`).
* Large or infinite action spaces where registering all templates is impossible.

**Thread Safety:** If generator accesses JavaScript state, it must use `Bridge.RunOnLoopSync` (handled automatically in `osm:pabt`).

---

### `pabt.newExprCondition(key, expression)`

Creates a Go-native condition using `expr-lang` (fast path).

```javascript
// Instead of:
{key: 'distance', match: v => v < 50}

// Use:
pabt.newExprCondition('distance', 'value < 50')
```

**Parameters:**

* `key` - Blackboard key.
* `expression` - `expr-lang` expression string.

**Expression Environment:**

* `value` - The input value from the blackboard.

**Expression Examples:**

```javascript
// Equality
pabt.newExprCondition('status', 'value == "ready"')

// Comparison
pabt.newExprCondition('distance', 'value < 50 && value > 0')

// Null check
pabt.newExprCondition('item', 'value != nil')

// Length check
pabt.newExprCondition('items', 'len(value) > 0')
```

**Performance:** 10-100x faster than JavaScript conditions (~100ns vs ~5μs).

**When to use:**

* Arithmetic expressions.
* High-frequency condition checks (hot paths in the BT).

---

## Advanced Patterns

### Pattern 1: Static + Parametric Actions

This pattern combines pre-registered actions for common tasks with generated actions for dynamic targets.

```javascript
// STATIC: Fixed actions (Pick, Place)
state.registerAction('Pick', pabt.newAction('Pick',
    [{key: 'atCube', match: v => v === true}],
    [{key: 'heldItem', value: cubeId}],
    pickNode
));

// PARAMETRIC: MoveTo any entity
state.setActionGenerator(function(failedCondition) {
    const key = failedCondition.key;

    // Generate MoveTo actions on-demand during backchaining
    if (key.startsWith('atEntity_')) {
        const id = parseInt(key.replace('atEntity_', ''));
        return [createMoveToAction(id)];
    }

    return [];
});

```

---

### Pattern 2: Heuristic Effects for Planning

In PA-BT, **Postconditions** (Effects) are used to guide the search. You can list effects that are not strictly guaranteed but serve as heuristics to bias the planner.

```javascript
// Picking a blockade cube MIGHT make target reachable
// The planner uses this "Effect" to select this action when 'reachable_target' is needed.
state.registerAction('PickBlockade_1', pabt.newAction('PickBlockade_1',
    [{key: 'atEntity_1', match: v => v === true}],
    [
        {key: 'heldItem', value: 1},
        // HEURISTIC: Suggest this helps reach target
        {key: 'reachable_target', value: true}
    ],
    pickNode
));
```

**Trade-off:** If the heuristic is wrong, the action will execute, but the condition `reachable_target` may still be false afterwards. The PA-BT framework handles this by re-evaluating the tree; if the condition is still unsatisfied, it may attempt the action again or fail, depending on the tree structure (retries/loops).

---

### Pattern 3: Conflict Resolution via Staging

This resolves "Deadlock" situations where an agent holds an item but needs to perform an action requiring empty hands.

```javascript
// Problem: Goal blocked, holding target, need to pick up obstacle.
// Precondition of PickObstacle is "handsEmpty".

// Action: PlaceTemporary
// Effect: handsEmpty === true
state.registerAction('PlaceTemporary', pabt.newAction('PlaceTemporary',
    [{key: 'heldItem', match: v => v === targetId}],
    [{key: 'heldItem', value: null}],  // Frees hands (Postcondition)
    placeAtStagingNode
));

// The planner sees "handsEmpty" is failed, finds PlaceTemporary, and injects it.
```

---

### Pattern 4: Blackboard Sync Strategy

Efficient synchronization between JS state and the blackboard is critical for correct synthesis.

```javascript
function syncToBlackboard(state) {
    const bb = state.blackboard;
    const actor = state.actors.get(1);

    // ALWAYS sync: Core state
    bb.set('actorX', actor.x);
    bb.set('actorY', actor.y);
    bb.set('heldItem', actor.heldItem ? actor.heldItem.id : null);

    // CONDITIONALLY sync: Expensive computations
    // Use plan.Running() to distinguish execution ticks from planning ticks
    if (!state.plan || !state.plan.Running()) {
        // Not in middle of action execution -> Planner is likely evaluating options
        entities.forEach(entity => {
            const reachable = isReachable(actor, entity);
            bb.set(`reachable_${entity.id}`, reachable);
        });
    }
}
```

---

## Performance

### Benchmarks (Approximate)

| Operation | Time | Notes |
| --- | --- | --- |
| State.Variable() | ~500ns | Blackboard read with mutex |
| JSCondition.Match() | ~5μs | JavaScript evaluation + thread marshalling |
| ExprCondition.Match() | ~100ns | Go-native compiled expression |
| FuncCondition.Match() | ~50ns | Direct Go function call |
| plan.node() tick | ~10μs | Depends on tree depth |

### Optimization Strategies

1. **Use ExprCondition for hot paths**:
```javascript
// Bad: JavaScript condition (slow)
{key: 'distance', match: v => v < 100}

// Good: Expr condition (fast)
pabt.newExprCondition('distance', 'value < 100')
```


2. **Minimize blackboard keys**: Sync only the symbolic propositions required for planning, not the entire engine state.
3. **Cache parametric actions**: Avoid object allocation during synthesis.
4. **Batch blackboard updates**: Call `syncToBlackboard()` once per tick.

---

## Troubleshooting

### Plan doesn't find any actions

**Symptoms:** Plan immediately fails with no actions selected.

**Causes:**

1. **Backchaining Failure**: No action exists with an **Effect** that matches the failed condition.
2. **Blackboard Desync**: The blackboard key required by the condition is missing or has the wrong type.

**Debug:**

```javascript
// Check action registration
console.log('Registered actions:', state.actions.All().length);

// Check blackboard state
console.log('atCube:', bb.get('atCube'));
```

**Fix:**

* Verify that `Action.Effects` perfectly match the `Condition` keys and values.
* Ensure `setActionGenerator` correctly identifies the condition pattern.

---

### Plan executes but state doesn't change

**Symptoms:** Action nodes return `Success` but the application state remains unchanged.

**Causes:**

1. **Logic Separation**: The Action Node logic does not mutate the JS state.
2. **False Success**: The Node returns `bt.success` without doing the work.

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

* The Action Node is responsible for **grounding** the symbolic effect. It must mutate the JS variables such that the next `syncToBlackboard` reflects the change.

---

## See Also

- [bt-blackboard-usage.md](./bt-blackboard-usage.md) - Blackboard patterns
- [go-pabt reference](https://github.com/joeycumines/go-pabt) - Underlying algorithm
- [example-05-pick-and-place.js](../../scripts/example-05-pick-and-place.js) - Complete example
- [PA-BT Paper](https://github.com/joeycumines/go-pabt#references) - Original research
