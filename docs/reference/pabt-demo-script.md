# PA-BT Demo Script Documentation

**File:** `scripts/example-05-pick-and-place.js`  
**Purpose:** Demonstrates the Planning-Augmented Behavior Trees (PA-BT) implementation for autonomous agents

---

## Overview

The pick-and-place demo implements an autonomous agent that can:
1. Navigate to cubes and goals in a 2D space
2. Pick up cubes (targets and obstacles)
3. Place cubes at goal locations
4. Dynamically replan when the environment changes

This demonstrates core PA-BT concepts:
- **Static Actions**: Pre-registered actions like `Pick_Target`, `Deliver_Target`
- **Parametric Actions**: Dynamically generated actions via `ActionGenerator`
- **Planning**: Goal-based planning using the go-pabt library
- **Blackboard Sync**: State synchronization between JavaScript and Go

---

## Architecture

### Component Hierarchy

```
┌─────────────────────────────────────────────────────────────────┐
│                    Game Loop (main tick)                         │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │   TUI Display   │  │   PA-BT Planner  │  │   Action Gen    │  │
│  │   (bubbletea)   │  │   (go-pabt)      │  │  (ActionGen)    │  │
│  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘  │
│           │                    │                    │           │
│           └────────────────────┼────────────────────┘           │
│                                │                                │
│                    ┌───────────┴───────────┐                    │
│                    │     State Sync        │                    │
│                    │  (syncToBlackboard)   │                    │
│                    └───────────────────────┘                    │
│                                │                                │
│                    ┌───────────┴───────────┐                    │
│                    │      Blackboard       │                    │
│                    │   (go-pabt.State)     │                    │
│                    └───────────────────────┘                    │
└─────────────────────────────────────────────────────────────────┘
```

### Key Files

| File | Purpose |
|------|---------|
| `scripts/example-05-pick-and-place.js` | Main demo script |
| `internal/builtin/pabt/state.go` | Go state management |
| `internal/builtin/pabt/actions.go` | Action registry |
| `internal/builtin/pabt/require.go` | JS module loader |
| `internal/builtin/pabt/evaluation.go` | Condition evaluation |

---

## Core Concepts

### 1. Static Actions vs Parametric Actions

**Static Actions** are pre-registered with fixed conditions and effects:
```javascript
// Fixed action with specific conditions
pabt.newAction(
    "Pick_Target",
    [
        pabt.newExprCondition('heldItemId', 'value == nil', null),  // Must be empty-handed
        pabt.newExprCondition('targetId', 'value == ' + targetId, targetId)  // Target must exist
    ],
    [
        pabt.newEffect('heldItemId', targetId),  // Effect: set heldItemId to targetId
        pabt.newEffect('targetAt_' + targetId, actor.id)  // Effect: actor now holds target
    ],
    pickNode  // Behavior tree node
);
```

**Parametric Actions** are dynamically generated based on failed conditions:
```javascript
// Action generator creates actions on-demand
function(actionConditions) {
    // Find the failed condition
    for (const condition of actionConditions) {
        if (condition.key && condition.key.startsWith('atEntity_')) {
            // Extract entity ID from condition key (e.g., "atEntity_1" → "1")
            const entityId = condition.key.replace('atEntity_', '');
            // Create dynamic MoveTo action for this specific entity
            return createMoveToAction(entityId, condition);
        }
    }
    return null;  // No relevant parametric action
}
```

### 2. Planning Flow

```
1. Check Goal Conditions
   └─ pabt.newExprCondition('cubeDeliveredAtGoal', 'value == true', true)

2. If Goal Not Met:
   └─ Call pabt.newPlan(goalConditions, state)
      ├─ Filter relevant actions (based on failed conditions)
      ├─ Find path from current state to goal state
      └─ Return sequence of actions

3. Execute Plan Actions
   └─ Tick behavior tree with planned actions
```

### 3. Blackboard Synchronization

The `syncToBlackboard()` function synchronizes JavaScript game state to the Go blackboard:

```javascript
function syncToBlackboard(state) {
    // Sync actor properties
    syncValue(state, 'actorX', actor.x);
    syncValue(state, 'actorY', actor.y);
    syncValue(state, 'heldItemId', heldItem ? heldItem.id : null);
    
    // Sync cube positions
    for (const cube of cubes) {
        syncValue(state, 'atEntity_' + cube.id, cube.atEntity);
    }
    
    // Sync goal states
    for (const goal of goals) {
        syncValue(state, 'goalHas_' + goal.id, goal.hasItem);
    }
}
```

---

## Key Functions

### Setup Functions

| Function | Purpose |
|----------|---------|
| `setupPABTActions()` | Registers all static actions and sets the action generator |
| `createMoveToAction(entityId, condition)` | Creates parametric MoveTo actions |
| `createPickAction(cubeId)` | Creates Pick actions for specific cubes |
| `createPlaceAction(cubeId, location)` | Creates Place actions for specific cubes/locations |

### Core Functions

| Function | Purpose |
|----------|---------|
| `syncToBlackboard(state)` | Syncs JS state to Go blackboard |
| `tickPABT(tui)` | Main PA-BT tick function |
| `handlePABTResult(result)` | Handles plan success/failure |
| `planNextAction(state)` | Plans the next action based on current state |

---

## Modifying the Demo

### Adding a New Static Action

1. **Define the action conditions** (what must be true before executing):
```javascript
const conditions = [
    pabt.newExprCondition('heldItemId', 'value == null', null),  // Must be empty-handed
    // Add more preconditions...
];
```

2. **Define the action effects** (what changes after execution):
```javascript
const effects = [
    pabt.newEffect('someStateVariable', newValue),  // State changes
    // Add more effects...
];
```

3. **Register the action**:
```javascript
state.RegisterAction("My_New_Action", pabt.newAction(
    "My_New_Action",
    conditions,
    effects,
    myActionNode  // Behavior tree node
));
```

### Adding a New Parametric Action

1. **Update the action generator** to recognize new condition patterns:
```javascript
function(actionConditions) {
    for (const condition of actionConditions) {
        // Check for new pattern
        if (condition.key && condition.key.startsWith('myNewPattern_')) {
            const param = condition.key.replace('myNewPattern_', '');
            return createMyNewAction(param, condition);
        }
    }
    return null;
}
```

2. **Create the action factory**:
```javascript
function createMyNewAction(param, condition) {
    const conditions = [condition];  // Use the failed condition
    const effects = [
        pabt.newEffect('effectedState_' + param, true),
        // Add more effects...
    ];
    
    return pabt.newAction(
        "MyNew_" + param,
        conditions,
        effects,
        myNewNode
    );
}
```

### Modifying Goal Conditions

Change what the agent considers a "win":
```javascript
// Original: Deliver target to goal
const goalConditions = [
    pabt.newExprCondition('cubeDeliveredAtGoal', 'value == true', true)
];

// Modified: Deliver ALL cubes to ANY goal
const goalConditions = [
    pabt.newExprCondition('allCubesDelivered', 'value == true', true)
];
```

### Customizing State Sync

Add new state variables to the blackboard:
```javascript
function syncToBlackboard(state) {
    // Existing syncs...
    
    // Add custom state
    if (customGameState) {
        syncValue(state, 'customMetric', customGameState.metric);
        syncValue(state, 'energyLevel', customGameState.energy);
    }
}
```

---

## Common Patterns

### Pattern 1: Conditional Action Selection

```javascript
// Use expression conditions to select actions based on state
const conditions = [
    pabt.newExprCondition('energyLevel', 'value > 50', true),  // Only if energy > 50
    pabt.newExprCondition('targetInRange', 'value < 10', true)  // Target must be close
];
```

### Pattern 2: Multiple Effects

```javascript
// Actions can have multiple effects
const effects = [
    pabt.newEffect('heldItemId', itemId),           // Update held item
    pabt.newEffect('previousLocation', oldPos),     // Remember old position
    pabt.newEffect('energySpent', energySpent)      // Track resource usage
];
```

### Pattern 3: Handling Failures

```javascript
// Plan might fail - handle gracefully
function handlePABTResult(result) {
    if (result.error) {
        console.log("Plan failed:", result.error);
        // Fallback to manual control or retry
        return fallbackBehavior();
    }
    // Execute plan steps...
}
```

---

## Troubleshooting

### Issue: Agent Not Planning
**Check:** Is the action generator returning null?
**Fix:** Ensure the generator handles all condition key patterns

### Issue: Wrong Action Selected
**Check:** Are conditions properly specified?
**Fix:** Verify condition keys match the blackboard state keys

### Issue: Plan Never Completes
**Check:** Are goal conditions ever satisfiable?
**Fix:** Verify goal conditions can be achieved by action effects

### Issue: Performance Issues
**Check:** Is syncToBlackboard called too frequently?
**Fix:** Consider batching state updates or using dirty flags

---

## Performance Considerations

| Aspect | Recommendation |
|--------|----------------|
| **Sync Frequency** | Sync only changed values; use dirty flags |
| **Expression Complexity** | Use expr-lang conditions for performance (not JS) |
| **Action Count** | Keep static action count reasonable; use parametric for variety |
| **Plan Depth** | Limit plan depth to prevent exponential search |

---

## References

- **PA-BT Architecture**: `REVIEW_PABT.md`
- **PA-BT Reference**: `docs/reference/planning-and-acting-using-behavior-trees.md`
- **go-pabt Library**: https://github.com/joeycumines/go-pabt
- **expr-lang**: https://expr-lang.org/
