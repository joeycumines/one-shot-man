# osm:pabt Go API Specification

**Date:** 2026-01-19  
**Version:** 1.0  
**Status:** Phase 1 Foundation Document

---

## Overview

This document specifies the minimal Go API for the osm:pabt module. The Go layer provides:

1. **PA-BT Primitives:** Wrappers around go-pabt interfaces
2. **Evaluation Modes:** JSCondition, ExprCondition, FuncCondition
3. **Module Loader:** JavaScript API surface via ModuleLoader

The Go layer is **NOT** responsible for:

- Application-specific types (Shape, Sprite, Simulation)
- Domain logic (pick, place, move actions)
- Game/simulation state management

---

## Core Interfaces

### State (implements pabtpkg.IState)

```go
type State struct {
    Blackboard *bt.Blackboard  // Underlying storage
    actions    *ActionRegistry // Thread-safe action storage
}

// Variable returns the value for a given key.
// Keys are normalized to strings for blackboard lookup.
// Supports: string, int*, uint*, float*, fmt.Stringer
func (s *State) Variable(key any) (any, error)

// Actions returns all executable actions for a failed condition.
// CRITICAL: Must filter actions based on effect matching (see FP3)
func (s *State) Actions(failed pabtpkg.Condition) ([]pabtpkg.IAction, error)

// RegisterAction adds an action to the registry
func (s *State) RegisterAction(name string, action pabtpkg.IAction)
```

### ActionRegistry

```go
type ActionRegistry struct {
    mu      sync.RWMutex
    actions map[string]pabtpkg.IAction
}

// Register adds an action (thread-safe)
func (r *ActionRegistry) Register(name string, action pabtpkg.IAction)

// Get retrieves an action by name
func (r *ActionRegistry) Get(name string) pabtpkg.IAction

// All returns all registered actions
func (r *ActionRegistry) All() []pabtpkg.IAction
```

### Action (implements pabtpkg.IAction)

```go
type Action struct {
    Name_       string
    Conditions_ []pabtpkg.IConditions
    Effects_    pabtpkg.Effects
    Node_       bt.Node
}

func (a *Action) Conditions() []pabtpkg.IConditions
func (a *Action) Effects() pabtpkg.Effects
func (a *Action) Node() bt.Node
```

---

## Condition Types

### JSCondition (EvalModeJavaScript)

```go
type JSCondition struct {
    key        any           // Variable key (passed to State.Variable)
    matchFunc  goja.Callable // JavaScript Match function
    bridge     *Bridge       // For thread-safe VM access
}

// Key returns the condition's variable key
func (c *JSCondition) Key() any

// Match evaluates the condition against a value
// CRITICAL: Uses Bridge.RunOnLoopSync for thread safety
func (c *JSCondition) Match(r any) bool
```

**Thread Safety:** All Goja calls go through Bridge.RunOnLoopSync to ensure single-threaded access to the JavaScript VM.

### ExprCondition (EvalModeExpr)

```go
type ExprCondition struct {
    key      any
    program  *vm.Program  // Compiled expr-lang program
    mu       sync.RWMutex // For thread-safe evaluation
}

// Key returns the condition's variable key
func (c *ExprCondition) Key() any

// Match evaluates the condition against a value
// CRITICAL: ZERO Goja calls - pure Go execution
func (c *ExprCondition) Match(r any) bool
```

**Performance:** ExprCondition is 10-100x faster than JSCondition because:
- No JavaScript VM context switching
- Compiled bytecode execution
- No event loop synchronization

### FuncCondition (Pure Go)

```go
type FuncCondition struct {
    key       any
    matchFunc func(any) bool
}

// Key returns the condition's variable key
func (c *FuncCondition) Key() any

// Match evaluates the condition against a value
func (c *FuncCondition) Match(r any) bool
```

---

## Effect Types

### Effect

```go
type Effect struct {
    key   any
    value any
}

func (e *Effect) Key() any
func (e *Effect) Value() any
```

### JSEffect

```go
type JSEffect struct {
    key   any        // Variable key (from JavaScript)
    value goja.Value // Effect value (from JavaScript)
}

func (e *JSEffect) Key() any
func (e *JSEffect) Value() any
```

---

## Simple Types (Go-Native)

### SimpleCond

```go
type SimpleCond struct {
    key       any
    matchFunc func(any) bool
}
```

### SimpleEffect

```go
type SimpleEffect struct {
    key   any
    value any
}
```

### SimpleAction

```go
type SimpleAction struct {
    conditions []pabtpkg.IConditions
    effects    pabtpkg.Effects
    node       bt.Node
}
```

---

## JavaScript API (ModuleLoader)

### Exported Constants

| Export | Value | Type |
|--------|-------|------|
| `Running` | `"running"` | `string` |
| `Success` | `"success"` | `string` |
| `Failure` | `"failure"` | `string` |

### Exported Functions

#### newState(blackboard)

Creates a PA-BT State wrapping a Blackboard.

```javascript
const state = pabt.newState(blackboard);
```

#### newSymbol(name)

Creates a symbol (passthrough for now).

```javascript
const sym = pabt.newSymbol("myKey");
```

#### newPlan(state, goals)

Creates a PA-BT plan with the given goals.

```javascript
const plan = pabt.newPlan(state, [
    [condition1, condition2],  // Goal 1: condition1 AND condition2
    [condition3]               // Goal 2: condition3
]);
```

**Parameters:**
- `state`: PA-BT State object (from newState or custom implementation)
- `goals`: Array of goal arrays (each inner array is AND'd conditions)

#### newAction(name, conditions, effects, node)

Creates a PA-BT Action.

```javascript
const action = pabt.newAction(
    "pick-cube",
    [[cond1, cond2]],  // conditions (array of IConditions)
    [effect1, effect2], // effects
    btNode              // behavior tree node
);
```

---

## Thread Safety Requirements

### Goja Access

All Goja VM access MUST go through Bridge.RunOnLoopSync:

```go
result, err := bridge.RunOnLoopSync(ctx, func(rt *goja.Runtime) (any, error) {
    // Safe to access rt here
    return someGojaCall(rt), nil
})
```

### State Access

State implementations must be thread-safe:
- ActionRegistry uses sync.RWMutex
- ExprCondition uses sync.RWMutex
- Blackboard is already thread-safe

---

## Error Handling

### Variable Lookup Errors

```go
value, err := state.Variable(key)
if err != nil {
    // Key type not supported or lookup failed
}
```

### Condition Evaluation Errors

JSCondition.Match() returns false on errors (logs error internally).
ExprCondition.Match() returns false on errors (logs error internally).

### Plan Creation Errors

```go
plan, err := pabt.INew(state, goals)
if err != nil {
    // Invalid goals or state
}
```

---

## Performance Considerations

### Condition Evaluation Hot Path

1. **Prefer ExprCondition** for frequently evaluated conditions
2. **Cache compiled programs** in ExprCondition
3. **Minimize Goja calls** in JSCondition.Match()

### Action Filtering

State.Actions() is called frequently during planning. Must be efficient:

```go
func (s *State) Actions(failed pabtpkg.Condition) ([]pabtpkg.IAction, error) {
    // O(n) where n = number of registered actions
    // Filter by effect matching (not condition evaluation)
}
```

---

## Parity with go-pabt

### Required Interfaces

| go-pabt | osm:pabt | Status |
|---------|----------|--------|
| `pabt.IState` | `State` | ✅ Implemented |
| `pabt.IAction` | `Action` | ✅ Implemented |
| `pabt.Condition` | JSCondition/ExprCondition/FuncCondition | ✅ Implemented |
| `pabt.Effect` | Effect/JSEffect | ✅ Implemented |
| `pabt.IPlan` | Created via newPlan | ✅ Implemented |

### Behavioral Requirements

1. **Variable() Semantics:** Must support stateVar-like patterns via JavaScript
2. **Actions() Semantics:** Must filter by effect matching (FP3)
3. **Condition.Match():** Must return bool (no errors in signature)
4. **Effect.Value():** Must return the effect's target value

---

## Migration Notes

### From Pre-Phase-0 Code

The following types were **REMOVED** and must be implemented in JavaScript:

- `Shape` → JavaScript class
- `Space` → JavaScript enum
- `Sprite`, `Actor`, `Cube`, `Goal` → JavaScript classes
- `Simulation` → JavaScript class
- `StateVar` → JavaScript interface pattern
- Templates (Pick, Place, Move) → JavaScript functions

### Variable Key Protocol

JavaScript variable keys should implement:

```javascript
class MyVarKey {
    constructor(data) {
        this.data = data;
    }
    
    stateVar(simulation) {
        // Return current value for this variable
        return { ... };
    }
}
```

The Go State.Variable() wrapper will call this method via Goja.
