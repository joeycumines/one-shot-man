# JavaScript Example Architecture (example-05-pick-and-place.js)

**Date:** 2026-01-19  
**Purpose:** Define the JavaScript-side architecture for the pick-and-place example  
**Status:** DESIGN PHASE

---

## Reference Analysis

The Go reference (`tcell-pick-and-place`) has this structure:

```
examples/tcell-pick-and-place/
├── main.go       # Entry point, tcell UI
├── logic/        # GLUE CODE (PA-BT integration)
│   └── logic.go  # Implements pabt.IState, action templates
└── sim/          # APPLICATION TYPES (simulation domain)
    ├── shape.go  # Shape, Space interfaces
    ├── sprite.go # Sprite, Actor, Cube, Goal
    ├── state.go  # State, simulation state management
    └── ...       # Other sim components
```

**Key Insight:** All domain types (Shape, Space, Sprite, Actor, Cube, Goal, State) live in `sim/` package. The `logic/` package only contains glue code implementing `pabt.IState`.

---

## osm:pabt JavaScript Equivalent

For osm:pabt, the mapping is:

| Reference (Go)     | osm:pabt (JS) | Location |
|-------------------|---------------|----------|
| `sim/` package    | JS module     | `example-05-pick-and-place.js` |
| `logic/` package  | `pabt` module | `internal/builtin/pabt/` |
| `pabt.IState`     | `pabt.State`  | Go layer (wraps Blackboard) |
| `pabt.Condition`  | JS object     | JS layer |
| `pabt.Effect`     | JS object     | JS layer |
| `pabt.IAction`    | JS object     | JS layer |

---

## JavaScript Domain Types

### 1. Shape Class

```javascript
class Shape {
    constructor(x, y, width, height) {
        this.x = x;
        this.y = y;
        this.width = width;
        this.height = height;
    }
    
    position() { return { x: this.x, y: this.y }; }
    size() { return { w: this.width, h: this.height }; }
    
    setPosition(x, y) {
        this.x = x;
        this.y = y;
    }
    
    clone() {
        return new Shape(this.x, this.y, this.width, this.height);
    }
    
    collides(other) {
        return !(this.x + this.width <= other.x ||
                 other.x + other.width <= this.x ||
                 this.y + this.height <= other.y ||
                 other.y + other.height <= this.y);
    }
    
    distance(other) {
        const dx = Math.max(0, Math.max(other.x - (this.x + this.width), 
                                        this.x - (other.x + other.width)));
        const dy = Math.max(0, Math.max(other.y - (this.y + this.height), 
                                        this.y - (other.y + other.height)));
        return Math.sqrt(dx * dx + dy * dy);
    }
}
```

### 2. Space Enum

```javascript
const Space = {
    FLOOR: "floor",
    AIR: "air"
};
```

### 3. Sprite Classes

```javascript
class Sprite {
    constructor(shape, space = Space.FLOOR) {
        this._shape = shape;
        this._space = space;
        this._velocity = { dx: 0, dy: 0 };
        this._stopped = false;
        this._deleted = false;
    }
    
    position() { return this._shape.position(); }
    size() { return this._shape.size(); }
    shape() { return this._shape; }
    space() { return this._space; }
    velocity() { return this._velocity; }
    stopped() { return this._stopped; }
    deleted() { return this._deleted; }
    
    collides(otherSpace, otherShape) {
        if (this._space !== otherSpace) return false;
        return this._shape.collides(otherShape);
    }
}

class Actor extends Sprite {
    constructor(shape, criteria = new Map()) {
        super(shape);
        this._criteria = criteria;
        this._heldItem = null;
        this._keyboard = false;
    }
    
    criteria() { return this._criteria; }
    heldItem() { return this._heldItem; }
    keyboard() { return this._keyboard; }
}

class Cube extends Sprite {
    constructor(shape) {
        super(shape);
    }
}

class Goal extends Sprite {
    constructor(shape) {
        super(shape);
    }
}
```

### 4. State Variables (Variable Keys)

```javascript
// Variable keys are objects that implement stateVar pattern
// They are used to identify what part of state to query

class HeldItemVar {
    constructor(actor) {
        this.actor = actor;
    }
    
    stateVar(sim) {
        return { item: this.actor.heldItem() };
    }
}

class PositionVar {
    constructor(sprite) {
        this.sprite = sprite;
    }
    
    stateVar(sim) {
        const positions = new Map();
        for (const [sprite, value] of sim.sprites()) {
            positions.set(sprite, {
                space: value.space(),
                shape: value.shape()
            });
        }
        return { positions };
    }
}
```

### 5. Conditions

```javascript
// Conditions are objects with key + Match function
function simpleCond(key, matchFn) {
    return {
        key: key,
        Match: matchFn
    };
}

// Example: Actor is near a cube
const nearCubeCondition = simpleCond(
    new PositionVar(actor),
    (value) => {
        const actorPos = value.positions.get(actor);
        const cubePos = value.positions.get(cube);
        return actorPos && cubePos && 
               actorPos.shape.distance(cubePos.shape) <= PICKUP_DISTANCE;
    }
);
```

### 6. Effects

```javascript
// Effects are objects with key + Value
function simpleEffect(key, value) {
    return {
        key: key,
        Value: value
    };
}

// Example: Actor now holds the cube
const holdCubeEffect = simpleEffect(
    new HeldItemVar(actor),
    { item: cube }
);
```

### 7. Actions

```javascript
// Actions bundle conditions, effects, and a behavior tree node
function createAction(name, conditions, effects, node) {
    return pabt.newAction(name, conditions, effects, node);
}

// Pick action template
function templatePick(state, actor, sprite, snapshot) {
    const ox = sprite.shape().position().x;
    const oy = sprite.shape().position().y;
    
    return createAction(
        `pick-${sprite.id}`,
        [
            // Sprite is still at original position
            simpleCond(new PositionVar(sprite), (v) => {
                const pos = v.positions.get(sprite);
                return pos && pos.shape && 
                       pos.shape.x === ox && pos.shape.y === oy;
            }),
            // Actor is not holding anything
            simpleCond(new HeldItemVar(actor), (v) => v.item === null),
            // Actor is near sprite
            simpleCond(new PositionVar(actor), (v) => {
                const actorPos = v.positions.get(actor);
                const spritePos = v.positions.get(sprite);
                return actorPos && spritePos &&
                       actorPos.shape.distance(spritePos.shape) <= PICKUP_DISTANCE;
            })
        ],
        [
            // Actor now holds the sprite
            simpleEffect(new HeldItemVar(actor), { item: sprite }),
            // Sprite is no longer visible (in hands)
            simpleEffect(new PositionVar(sprite), { 
                positions: computePickedPositions(snapshot, sprite) 
            })
        ],
        pickNode(actor, sprite)
    );
}
```

---

## State Implementation

### JavaScript PA-BT State Wrapper

```javascript
class PickAndPlaceState {
    constructor(simulation, actor) {
        this._simulation = simulation;
        this._actor = actor;
    }
    
    // Implements pabt.IState.Variable(key)
    variable(key) {
        if (typeof key.stateVar === 'function') {
            return key.stateVar(this._simulation);
        }
        throw new Error(`Unexpected key type: ${typeof key}`);
    }
    
    // Implements pabt.IState.Actions(failed)
    actions(failed) {
        const key = failed.key;
        const snapshot = this._simulation.state();
        const result = [];
        
        for (const [sprite, value] of snapshot.sprites) {
            if (sprite === this._actor) continue;
            
            if (sprite instanceof Cube) {
                // Template pick actions
                const pickActions = this.templatePick(failed, snapshot, sprite);
                result.push(...pickActions.filter(a => 
                    a.effects.some(e => e.key === key && failed.Match(e.Value))
                ));
                
                // Template place actions for all positions
                for (let x = 0; x < snapshot.spaceWidth; x++) {
                    for (let y = 0; y < snapshot.spaceHeight; y++) {
                        const placeActions = this.templatePlace(failed, snapshot, x, y, sprite);
                        result.push(...placeActions.filter(a =>
                            a.effects.some(e => e.key === key && failed.Match(e.Value))
                        ));
                    }
                }
            }
        }
        
        // Template move actions
        for (let x = 0; x < snapshot.spaceWidth; x++) {
            for (let y = 0; y < snapshot.spaceHeight; y++) {
                const moveActions = this.templateMove(failed, snapshot, x, y);
                result.push(...moveActions.filter(a =>
                    a.effects.some(e => e.key === key && failed.Match(e.Value))
                ));
            }
        }
        
        return result;
    }
    
    templatePick(failed, snapshot, sprite) { /* ... */ }
    templatePlace(failed, snapshot, x, y, sprite) { /* ... */ }
    templateMove(failed, snapshot, x, y) { /* ... */ }
}
```

---

## Integration with osm:pabt

### JavaScript Entry Point

```javascript
const pabt = require("osm:pabt");
const bt = require("osm:bt");

// Create simulation (all JavaScript domain types)
const simulation = new Simulation(config);

// Create PA-BT state for each actor
for (const actor of simulation.state().planConfig.actors) {
    const state = new PickAndPlaceState(simulation, actor);
    
    // Define success conditions
    const successConditions = [];
    for (const [pair, _] of actor.criteria()) {
        successConditions.push([
            simpleCond(new PositionVar(pair.cube), (v) => {
                const cubePos = v.positions.get(pair.cube);
                const goalPos = v.positions.get(pair.goal);
                return cubePos && goalPos &&
                       cubePos.shape.collides(goalPos.shape);
            })
        ]);
    }
    
    // Create PA-BT plan
    const plan = pabt.newPlan(state, successConditions);
    
    // Run behavior tree
    const ticker = bt.newTicker(plan.node(), { interval: 10 });
    ticker.start();
}
```

---

## Key Design Decisions

### 1. ALL Application Types in JavaScript

- ✅ Shape, Space, Sprite, Actor, Cube, Goal
- ✅ State variables (HeldItemVar, PositionVar)
- ✅ Simulation state management
- ✅ Action templates (Pick, Place, Move)

### 2. Go Layer is Pure Glue

- ✅ State wraps Blackboard
- ✅ Action struct holds conditions/effects/node
- ✅ Condition/Effect are JS objects
- ✅ No application-specific types

### 3. Variable Key Protocol

JavaScript variable keys must implement `stateVar(simulation)` method that returns the current value. This is called by the Go State.Variable() wrapper.

### 4. Effect Matching in Actions()

The `actions()` method must filter actions based on whether their effects match the failed condition. This is critical for correct PA-BT planning.

---

## Migration Path

1. Create `examples/example-05-pick-and-place.js`
2. Implement Shape, Space, Sprite classes
3. Implement PickAndPlaceState with action templates
4. Wire up to osm:pabt module
5. Add simulation loop (simpler than tcell, can use TUI primitives)

---

## Testing Strategy

1. **Unit Tests:** Test JavaScript domain types in isolation
2. **Integration Tests:** Verify PA-BT planning produces correct action sequences
3. **Parity Tests:** Compare plan outputs to reference Go implementation
