# PA-BT Reference Architecture Analysis

**Date:** 2026-01-19  
**Source:** `github.com/joeycumines/go-pabt/examples/tcell-pick-and-place`  
**Purpose:** Document the CORRECT separation of concerns between GLUE CODE and APPLICATION TYPES

---

## Executive Summary

The go-pabt reference implementation demonstrates a clear separation:

1. **`logic/logic.go`** = **GLUE CODE** - Implements `pabt.IState`, uses types from `sim/` package
2. **`sim/*.go`** = **APPLICATION TYPES** - Defines domain-specific types (Actor, Cube, Goal, Shape, Simulation)

**CRITICAL INSIGHT:** The logic package does NOT define simulation types. It USES them from the sim package.

---

## Package Structure

```
go-pabt/examples/tcell-pick-and-place/
├── main.go              # Entry point, creates sim + logic, runs BT
├── logic/
│   └── logic.go         # GLUE CODE - implements pabt.IState interface
└── sim/
    ├── shape.go         # APPLICATION TYPE - Shape geometry
    ├── state.go         # APPLICATION TYPES - Actor, Cube, Goal, Sprite, State
    ├── sim.go           # SIMULATION - Movement, collision, rendering
    └── *_test.go        # Tests for sim package
```

---

## GLUE CODE: logic/logic.go

### What It Does

1. **Implements `pabt.IState` interface** via `pickAndPlace` struct
2. **References types from `sim/` package** - does NOT define them
3. **Defines PA-BT primitives** - `simpleCond`, `simpleEffect`, `simpleAction`
4. **Implements action templates** - `templatePick`, `templatePlace`, `templateMove`

### Key Patterns

#### State Implementation

```go
type pickAndPlace struct {
    ctx        context.Context
    simulation sim.Simulation  // USES sim.Simulation
    actor      sim.Actor       // USES sim.Actor
    // ...
}

var _ pabt.IState = (*pickAndPlace)(nil)

func (p *pickAndPlace) Variable(key any) (any, error) {
    switch key := key.(type) {
    case stateVar:
        return key.stateVar(p)  // Delegates to stateVar interface
    default:
        return nil, fmt.Errorf(`unexpected key (%T): %v`, key, key)
    }
}

func (p *pickAndPlace) Actions(failed pabt.Condition) (actions []pabt.IAction, err error) {
    // Templates actions based on failed condition
    // Uses sim types to query current state
}
```

#### Variable Keys (stateVar interface)

```go
type stateVar interface {
    stateVar(state stateInterface) (any, error)
}

// heldItemVar - tracks what actor is holding
type heldItemVar struct {
    Actor sim.Actor
}

// positionVar - tracks sprite positions
type positionVar struct {
    Sprite sim.Sprite
}
```

#### Simple PA-BT Primitives

```go
// simpleCond implements pabt.Condition
type simpleCond struct {
    key   any
    match func(r any) bool
}

func (c *simpleCond) Key() any              { return c.key }
func (c *simpleCond) Match(value any) bool  { return c.match(value) }

// simpleEffect implements pabt.Effect
type simpleEffect struct {
    key   any
    value any
}

func (e *simpleEffect) Key() any    { return e.key }
func (e *simpleEffect) Value() any  { return e.value }

// simpleAction implements pabt.IAction
type simpleAction struct {
    conditions []pabt.IConditions
    effects    pabt.Effects
    node       bt.Node
}

func (a *simpleAction) Conditions() []pabt.IConditions { return a.conditions }
func (a *simpleAction) Effects() pabt.Effects          { return a.effects }
func (a *simpleAction) Node() bt.Node                  { return a.node }
```

#### Action Templates

```go
// templatePick - templates actions to pick up a sprite
func (p *pickAndPlace) templatePick(failed pabt.Condition, snapshot *sim.State, sprite sim.Sprite) ([]pabt.IAction, error) {
    // Conditions:
    // 1. Sprite is at known position
    // 2. Actor is not holding anything
    // 3. Actor is within pickup distance of sprite
    
    // Effects:
    // 1. Actor will hold the sprite
    // 2. Sprite position becomes nil (held, not on floor)
    
    // Node:
    // 1. Mark running
    // 2. Execute async pickup behavior
}

// templatePlace - templates actions to place a held sprite
func (p *pickAndPlace) templatePlace(failed pabt.Condition, snapshot *sim.State, x, y int32, sprite sim.Sprite) ([]pabt.IAction, error) {
    // Conditions:
    // 1. Actor is holding the sprite
    // 2. Actor is at position (x, y)
    // 3. No collisions with other sprites at release position
    
    // Effects:
    // 1. Actor will not hold anything
    // 2. Sprite will be at calculated release position
    
    // Node:
    // Execute async place behavior
}

// templateMove - templates actions to move actor to position
func (p *pickAndPlace) templateMove(failed pabt.Condition, snapshot *sim.State, x, y int32) ([]pabt.IAction, error) {
    // Path planning from current position to (x, y)
    // Check for collisions along path
    
    // Conditions:
    // 1. No collisions with any sprites along the path
    
    // Effects:
    // 1. Actor position will be (x, y)
    
    // Node:
    // Execute async movement behavior
}
```

---

## APPLICATION TYPES: sim/*.go

### sim/shape.go - Geometry

```go
type Shape interface {
    Position() (x, y int32)
    SetPosition(x, y int32)
    Size() (w, h int32)
    Center() (x, y int32)
    Closest(x, y int32) (cx, cy int32)
    Distance(shape Shape) float64
    Collides(shape Shape) bool
    Clone() Shape
}

// Implementation: shapeRectangle
type shapeRectangle struct{ X, Y, W, H int32 }
```

### sim/state.go - Sprites and State

```go
type Sprite interface {
    Position() (x, y float64)
    Size() (w, h int32)
    Shape() Shape
    Space() Space
    Velocity() (dx, dy float64)
    Stopped() bool
    Image() []rune
    Collides(space Space, shape Shape) bool
    Deleted() bool
}

// Actor - player/AI controlled entity
type Actor struct {
    spriteState
    actorState
}

// Cube - pickable object
type Cube struct {
    spriteState
    cubeState
}

// Goal - target location
type Goal struct {
    spriteState
    goalState
}

// State - snapshot of simulation
type State struct {
    SpaceWidth     int32
    SpaceHeight    int32
    PickupDistance float64
    Sprites        map[Sprite]Sprite
    PlanConfig     PlanConfig
}
```

### sim/sim.go - Simulation Logic

- Tick-based simulation loop
- Movement physics
- Collision detection
- Input handling
- Rendering (tcell)

---

## Mapping to osm:pabt

### Go Layer (internal/builtin/pabt/) = GLUE CODE

Should contain ONLY:

| Reference | osm:pabt Equivalent |
|-----------|---------------------|
| `simpleCond` | `SimpleCond`, `JSCondition`, `ExprCondition`, `FuncCondition` |
| `simpleEffect` | `SimpleEffect`, `JSEffect`, `Effect` |
| `simpleAction` | `SimpleAction`, `Action` |
| `pickAndPlace` (State impl) | `State` (wrapping Blackboard) |
| `stateVar` interface | Key normalization in `State.Variable()` |

**MUST NOT contain:** Shape, Simulation, Space, Sprite, Actor, Cube, Goal, etc.

### JavaScript Layer (examples/example-05-pick-and-place.js) = APPLICATION TYPES

Should contain ALL of:

| Reference | JavaScript Equivalent |
|-----------|----------------------|
| `sim.Shape` | `Shape` class/object |
| `sim.Sprite` | `Sprite` base type |
| `sim.Actor` | `Actor` type extending Sprite |
| `sim.Cube` | `Cube` type extending Sprite |
| `sim.Goal` | `Goal` type extending Sprite |
| `sim.State` | `State` object with sprites, space dimensions |
| `sim.Simulation` | `Simulation` with Move/Grasp/Release |
| `positionVar` | Position tracking pattern |
| `heldItemVar` | Held item tracking pattern |
| `templatePick/Place/Move` | Action templates in JavaScript |

---

## Condition.Match Differentiation (go-pabt docs)

From `pabt.go` documentation:

> Note that values of this type will be passed into State.Actions as-is, in order to facilitate handling of condition and/or failure specific action templating behavior. **Failure-specific behavior MAY require stateful conditions or similar, along with a way to differentiate calls to Condition.Match with values from the actual state (State.Variable) vs values from effects (Effect.Value).**

This means:
1. During planning, `Match` is called with `Effect.Value` (predicted future state)
2. During execution, `Match` is called with `State.Variable` (actual current state)
3. Stateful conditions may need to differentiate these cases

**Implementation approach for osm:pabt:**
- Track "evaluation context" (planning vs execution)
- Or use separate condition types for planning vs execution
- Or design conditions to work identically for both cases

---

## Key Architectural Constraints

1. **Go layer is GLUE only** - connects PA-BT to JavaScript domain
2. **JavaScript owns domain types** - all application-specific types in JS
3. **Blackboard uses JSON types only** - primitives that encoding/json can handle
4. **ExprCondition is pure Go** - ZERO Goja calls during Match
5. **JSCondition uses Bridge** - thread-safe access via RunOnLoopSync

---

## Implementation Checklist

- [ ] `internal/builtin/pabt/` contains NO application types
- [ ] `examples/example-05-pick-and-place.js` contains ALL application types
- [ ] Shape implementation in JavaScript matches `sim/shape.go` API
- [ ] Sprite/Actor/Cube/Goal in JavaScript matches `sim/state.go` API
- [ ] Action templates in JavaScript match `logic/logic.go` patterns
- [ ] ExprCondition.Match() has ZERO Goja calls
- [ ] JSCondition.Match() uses Bridge.RunOnLoopSync
- [ ] State.Variable() works with any key type (normalizes to string)
- [ ] State.Actions() returns actions with matching effects
