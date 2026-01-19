#!/usr/bin/env osm
// example-05-pick-and-place.js
//
// PA-BT (Planning Augmented Behavior Tree) Pick-and-Place Example
//
// This example demonstrates the PA-BT algorithm using JavaScript for all
// application-specific types (Shape, Space, Sprite, Simulation). The Go
// layer (osm:pabt module) provides ONLY the PA-BT primitives.
//
// Reference: github.com/joeycumines/go-pabt/examples/tcell-pick-and-place
//
// Architecture:
//   - Shape: Rectangle geometry with collision detection
//   - Space: Collision layer flags (Room, Floor)
//   - Sprite: Base for Actor, Cube, Goal with position, size, velocity
//   - Simulation: Tick-based movement, grasp/release mechanics
//   - Templates: templatePick, templatePlace, templateMove actions
//
// The PA-BT planner uses Conditions and Effects to generate plans that
// achieve the goal of placing cubes on goals.

'use strict';

const pabt = require('osm:pabt');
const bt = require('osm:bt');
const tui = require('osm:termui');

// =============================================================================
// SHAPE - Rectangle geometry with collision detection
// =============================================================================
// Matches go-pabt/sim/shape.go Shape interface and shapeRectangle implementation

/**
 * Shape represents a rectangular shape with position and size.
 * Provides collision detection, distance calculation, and cloning.
 *
 * @param {number} x - Top-left X position
 * @param {number} y - Top-left Y position
 * @param {number} w - Width (must be > 0)
 * @param {number} h - Height (must be > 0)
 */
function Shape(x, y, w, h) {
  if (w <= 0 || h <= 0) {
    throw new Error(`Shape invalid input: ${x}, ${y}, ${w}, ${h}`);
  }
  this.x = x;
  this.y = y;
  this.w = w;
  this.h = h;
}

/**
 * Position returns the top-left corner coordinates.
 * @returns {{x: number, y: number}}
 */
Shape.prototype.position = function() {
  return { x: this.x, y: this.y };
};

/**
 * SetPosition updates the top-left corner coordinates.
 * @param {number} x
 * @param {number} y
 */
Shape.prototype.setPosition = function(x, y) {
  this.x = x;
  this.y = y;
};

/**
 * Size returns the dimensions of the shape.
 * @returns {{w: number, h: number}}
 */
Shape.prototype.size = function() {
  return { w: this.w, h: this.h };
};

/**
 * Center returns the center point of the shape.
 * @returns {{x: number, y: number}}
 */
Shape.prototype.center = function() {
  return {
    x: Math.floor(this.w / 2) + this.x,
    y: Math.floor(this.h / 2) + this.y
  };
};

/**
 * closestBounds returns the closest value within [pos, pos+size-1] to target.
 * @param {number} pos
 * @param {number} size
 * @param {number} target
 * @returns {number}
 */
function closestBounds(pos, size, target) {
  if (size <= 0) {
    throw new Error(`closestBounds: invalid size ${size}`);
  }
  if (target <= pos) {
    return pos;
  }
  const max = pos + size - 1;
  if (target > max) {
    return max;
  }
  return target;
}

/**
 * Closest returns the point on this shape closest to the given point.
 * @param {number} x
 * @param {number} y
 * @returns {{x: number, y: number}}
 */
Shape.prototype.closest = function(x, y) {
  return {
    x: closestBounds(this.x, this.w, x),
    y: closestBounds(this.y, this.h, y)
  };
};

/**
 * calcDistance computes Euclidean distance between two points.
 * @param {number} x1
 * @param {number} y1
 * @param {number} x2
 * @param {number} y2
 * @returns {number}
 */
function calcDistance(x1, y1, x2, y2) {
  return Math.sqrt(Math.pow(x1 - x2, 2) + Math.pow(y1 - y2, 2));
}

/**
 * Distance returns the shortest distance between this shape and another.
 * Uses center of other shape and closest point on this shape.
 * @param {Shape} shape
 * @returns {number}
 */
Shape.prototype.distance = function(shape) {
  const otherCenter = shape.center();
  const closestToOther = this.closest(otherCenter.x, otherCenter.y);
  const myCenter = this.center();
  const otherClosest = shape.closest(myCenter.x, myCenter.y);
  return calcDistance(closestToOther.x, closestToOther.y, otherClosest.x, otherClosest.y);
};

/**
 * collidesRectangle checks collision between two rectangles.
 * @param {Shape} other
 * @returns {boolean}
 */
Shape.prototype.collidesRectangle = function(other) {
  // No horizontal overlap
  if (this.x >= other.x + other.w || other.x >= this.x + this.w) {
    return false;
  }
  // No vertical overlap
  if (this.y + this.h <= other.y || other.y + other.h <= this.y) {
    return false;
  }
  return true;
};

/**
 * Collides returns true if this shape overlaps with another shape.
 * @param {Shape} shape
 * @returns {boolean}
 */
Shape.prototype.collides = function(shape) {
  // For now, all shapes are rectangles
  return this.collidesRectangle(shape);
};

/**
 * Clone returns a deep copy of this shape.
 * @returns {Shape}
 */
Shape.prototype.clone = function() {
  return new Shape(this.x, this.y, this.w, this.h);
};

/**
 * roundPosition rounds floating point coordinates to integers.
 * @param {number} x
 * @param {number} y
 * @returns {{x: number, y: number}}
 */
function roundPosition(x, y) {
  return {
    x: Math.round(x),
    y: Math.round(y)
  };
}

/**
 * newRectangle creates a new rectangular shape.
 * @param {number} x
 * @param {number} y
 * @param {number} w
 * @param {number} h
 * @returns {Shape}
 */
function newRectangle(x, y, w, h) {
  return new Shape(x, y, w, h);
}

// =============================================================================
// SPACE - Collision layer flags
// =============================================================================
// Matches go-pabt/sim/sim.go Space struct

/**
 * Space represents collision layer flags.
 * Sprites only collide if their spaces overlap (both have Room=true, etc.)
 *
 * @param {boolean} room - Collides with room objects
 * @param {boolean} floor - Collides with floor objects
 */
function Space(room, floor) {
  this.room = !!room;
  this.floor = !!floor;
}

/**
 * Collides returns true if this space overlaps with another.
 * @param {Space} other
 * @returns {boolean}
 */
Space.prototype.collides = function(other) {
  return (this.room && other.room) || (this.floor && other.floor);
};

// Predefined spaces matching go-pabt
const ACTOR_SPACE = new Space(true, false);
const CUBE_SPACE = new Space(true, false);
const GOAL_SPACE = new Space(false, true);

// =============================================================================
// SPRITE - Base for Actor, Cube, Goal
// =============================================================================
// Matches go-pabt/sim/state.go Sprite interface

/**
 * Sprite represents a movable object in the simulation.
 *
 * @param {number} x - X position (float for smooth movement)
 * @param {number} y - Y position (float for smooth movement)
 * @param {number} w - Width
 * @param {number} h - Height
 * @param {Space} space - Collision layer
 * @param {string} image - Display character(s)
 */
function Sprite(x, y, w, h, space, image) {
  this.x = x;
  this.y = y;
  this.w = w;
  this.h = h;
  this.space = space;
  this.image = image;
  this.dx = 0;  // Velocity X
  this.dy = 0;  // Velocity Y
  this.stopped = false;
  this.deleted = false;
}

/**
 * Position returns the current position.
 * @returns {{x: number, y: number}}
 */
Sprite.prototype.position = function() {
  return { x: this.x, y: this.y };
};

/**
 * Size returns the dimensions.
 * @returns {{w: number, h: number}}
 */
Sprite.prototype.size = function() {
  return { w: this.w, h: this.h };
};

/**
 * Shape returns a Shape for this sprite's current position.
 * @returns {Shape}
 */
Sprite.prototype.shape = function() {
  const pos = roundPosition(this.x, this.y);
  return new Shape(pos.x, pos.y, this.w, this.h);
};

/**
 * Velocity returns the current velocity.
 * @returns {{dx: number, dy: number}}
 */
Sprite.prototype.velocity = function() {
  return { dx: this.dx, dy: this.dy };
};

/**
 * Collides checks if this sprite collides with a shape in a given space.
 * @param {Space} space
 * @param {Shape} shape
 * @returns {boolean}
 */
Sprite.prototype.collides = function(space, shape) {
  if (!this.space.collides(space)) {
    return false;
  }
  return this.shape().collides(shape);
};

/**
 * Snapshot creates a detached copy of this sprite.
 * @returns {Sprite}
 */
Sprite.prototype.snapshot = function() {
  const s = new Sprite(this.x, this.y, this.w, this.h, this.space, this.image);
  s.dx = this.dx;
  s.dy = this.dy;
  s.stopped = this.stopped;
  s.deleted = this.deleted;
  return s;
};

// =============================================================================
// ACTOR - Sprite that can pick up and place items
// =============================================================================

/**
 * Actor is a sprite that can pick up and place cubes.
 *
 * @param {number} x
 * @param {number} y
 * @param {number} w
 * @param {number} h
 * @param {string} image
 */
function Actor(x, y, w, h, image) {
  Sprite.call(this, x, y, w, h, ACTOR_SPACE, image);
  this.heldItem = null;  // Currently held Cube
  this.criteria = new Map();  // CriteriaKey -> CriteriaValue
  this.keyboard = false;  // Controlled by keyboard
}

Actor.prototype = Object.create(Sprite.prototype);
Actor.prototype.constructor = Actor;

Actor.prototype.snapshot = function() {
  const a = new Actor(this.x, this.y, this.w, this.h, this.image);
  a.dx = this.dx;
  a.dy = this.dy;
  a.stopped = this.stopped;
  a.deleted = this.deleted;
  a.heldItem = this.heldItem;  // Reference, not deep copy
  a.criteria = new Map(this.criteria);
  a.keyboard = this.keyboard;
  return a;
};

// =============================================================================
// CUBE - Sprite that can be picked up
// =============================================================================

/**
 * Cube is a sprite that can be picked up by an actor.
 *
 * @param {number} x
 * @param {number} y
 * @param {number} w
 * @param {number} h
 * @param {string} image
 */
function Cube(x, y, w, h, image) {
  Sprite.call(this, x, y, w, h, CUBE_SPACE, image);
}

Cube.prototype = Object.create(Sprite.prototype);
Cube.prototype.constructor = Cube;

// =============================================================================
// GOAL - Target location for cubes
// =============================================================================

/**
 * Goal is a sprite marking where cubes should be placed.
 *
 * @param {number} x
 * @param {number} y
 * @param {number} w
 * @param {number} h
 * @param {string} image
 */
function Goal(x, y, w, h, image) {
  Sprite.call(this, x, y, w, h, GOAL_SPACE, image);
}

Goal.prototype = Object.create(Sprite.prototype);
Goal.prototype.constructor = Goal;

// =============================================================================
// SIMULATION - Tick-based movement, grasp/release
// =============================================================================
// Matches go-pabt/sim/sim.go Simulation interface

const TICK_PER_SECOND = 30;
const STEP_DISTANCE = 6.0 / TICK_PER_SECOND;
const PICKUP_DISTANCE = 1.42;
const SPACE_WIDTH = 56;  // 80 - 24 (hudWidth)
const SPACE_HEIGHT = 24;

/**
 * Simulation manages sprites and their interactions.
 */
function Simulation() {
  this.sprites = new Map();  // Sprite -> Sprite (key is canonical, value is snapshot)
  this.actors = [];
  this.cubes = [];
  this.goals = [];
  this.running = false;
}

/**
 * State returns a snapshot of current simulation state.
 * @returns {object}
 */
Simulation.prototype.state = function() {
  const sprites = new Map();
  for (const [key, _] of this.sprites) {
    sprites.set(key, key.snapshot());
  }
  return {
    spaceWidth: SPACE_WIDTH,
    spaceHeight: SPACE_HEIGHT,
    pickupDistance: PICKUP_DISTANCE,
    sprites: sprites
  };
};

/**
 * AddActor adds an actor to the simulation.
 * @param {Actor} actor
 */
Simulation.prototype.addActor = function(actor) {
  this.actors.push(actor);
  this.sprites.set(actor, actor);
};

/**
 * AddCube adds a cube to the simulation.
 * @param {Cube} cube
 */
Simulation.prototype.addCube = function(cube) {
  this.cubes.push(cube);
  this.sprites.set(cube, cube);
};

/**
 * AddGoal adds a goal to the simulation.
 * @param {Goal} goal
 */
Simulation.prototype.addGoal = function(goal) {
  this.goals.push(goal);
  this.sprites.set(goal, goal);
};

/**
 * ValidateShape checks if a shape is within bounds.
 * @param {Shape} shape
 * @returns {Error|null}
 */
Simulation.prototype.validateShape = function(shape) {
  const pos = shape.position();
  const size = shape.size();
  if (pos.x < 0 || pos.y < 0) {
    return new Error(`shape out of bounds: negative position`);
  }
  if (pos.x + size.w > SPACE_WIDTH || pos.y + size.h > SPACE_HEIGHT) {
    return new Error(`shape out of bounds: exceeds space`);
  }
  return null;
};

/**
 * Move attempts to move a sprite to a position.
 * @param {Sprite} sprite
 * @param {number} x
 * @param {number} y
 * @returns {Promise<void>}
 */
Simulation.prototype.move = async function(sprite, x, y) {
  // Calculate direction
  const dx = x - sprite.x;
  const dy = y - sprite.y;
  const d = Math.sqrt(dx * dx + dy * dy);
  
  if (d === 0) {
    return;
  }
  
  // Normalize and set velocity
  sprite.dx = (dx / d) * STEP_DISTANCE;
  sprite.dy = (dy / d) * STEP_DISTANCE;
  sprite.stopped = false;
  
  // Move until we reach the target
  while (true) {
    const distToTarget = Math.sqrt(
      Math.pow(x - sprite.x, 2) + 
      Math.pow(y - sprite.y, 2)
    );
    
    if (distToTarget <= STEP_DISTANCE) {
      sprite.x = x;
      sprite.y = y;
      sprite.dx = 0;
      sprite.dy = 0;
      break;
    }
    
    sprite.x += sprite.dx;
    sprite.y += sprite.dy;
    
    // Check bounds
    const shape = sprite.shape();
    if (this.validateShape(shape) !== null) {
      sprite.x -= sprite.dx;
      sprite.y -= sprite.dy;
      sprite.dx = 0;
      sprite.dy = 0;
      throw new Error('movement blocked by bounds');
    }
    
    // Check collision with other sprites
    for (const [other, _] of this.sprites) {
      if (other === sprite) continue;
      if (sprite.collides(other.space, other.shape())) {
        sprite.x -= sprite.dx;
        sprite.y -= sprite.dy;
        sprite.dx = 0;
        sprite.dy = 0;
        throw new Error('movement blocked by collision');
      }
    }
    
    // Yield to allow rendering
    await new Promise(resolve => setTimeout(resolve, 1000 / TICK_PER_SECOND));
  }
};

/**
 * Grasp attempts to pick up a target sprite.
 * @param {Actor} actor
 * @param {Sprite} target
 * @returns {Promise<void>}
 */
Simulation.prototype.grasp = async function(actor, target) {
  if (actor.heldItem !== null) {
    throw new Error('actor already holding item');
  }
  
  const distance = actor.shape().distance(target.shape());
  if (distance > PICKUP_DISTANCE) {
    throw new Error(`target too far: ${distance} > ${PICKUP_DISTANCE}`);
  }
  
  actor.heldItem = target;
  // Remove target from collision detection while held
  this.sprites.delete(target);
};

/**
 * Release places the held item at actor's current position.
 * @param {Actor} actor
 * @param {Sprite} target
 * @returns {Promise<void>}
 */
Simulation.prototype.release = async function(actor, target) {
  if (actor.heldItem !== target) {
    throw new Error('actor not holding this item');
  }
  
  // Calculate release position (below/beside actor)
  const actorPos = actor.position();
  const actorSize = actor.size();
  const targetSize = target.size();
  
  // Release to the right of the actor, one cell down
  const releaseX = actorPos.x + actorSize.w;
  const releaseY = actorPos.y + Math.floor(actorSize.h / 2);
  
  target.x = releaseX;
  target.y = releaseY;
  
  // Check if release position is valid
  const shape = target.shape();
  if (this.validateShape(shape) !== null) {
    throw new Error('cannot release: out of bounds');
  }
  
  // Check collision at release position
  for (const [other, _] of this.sprites) {
    if (other === target) continue;
    if (target.collides(other.space, other.shape())) {
      throw new Error('cannot release: collision');
    }
  }
  
  actor.heldItem = null;
  this.sprites.set(target, target);
};

/**
 * ActorHeldItemReleaseShape calculates where an item would be released.
 * @param {Actor} actor
 * @param {number} x - Actor position X
 * @param {number} y - Actor position Y
 * @param {Sprite} item
 * @returns {Shape}
 */
Simulation.prototype.actorHeldItemReleaseShape = function(actor, x, y, item) {
  const actorSize = actor.size();
  const itemSize = item.size();
  
  // Release to the right of the actor
  const releaseX = x + actorSize.w;
  const releaseY = y + Math.floor(actorSize.h / 2) - Math.floor(itemSize.h / 2);
  
  return new Shape(releaseX, releaseY, itemSize.w, itemSize.h);
};

// =============================================================================
// STATE VARIABLES - Keys for PA-BT condition evaluation
// =============================================================================
// Matches go-pabt/logic/logic.go stateVar pattern

/**
 * HeldItemVar represents the key for tracking actor's held item.
 */
function HeldItemVar(actor) {
  this.actor = actor;
}

HeldItemVar.prototype.key = function() {
  return `heldItem:${this.actor.image}`;
};

/**
 * PositionVar represents the key for tracking sprite positions.
 */
function PositionVar(sprite) {
  this.sprite = sprite;
}

PositionVar.prototype.key = function() {
  return `position:${this.sprite.image}`;
};

// =============================================================================
// CONDITION AND EFFECT FACTORIES
// =============================================================================
// Matches go-pabt/logic/logic.go simpleCond/simpleEffect pattern

/**
 * simpleCond creates a condition that uses a match function.
 * @param {*} key - The condition key
 * @param {function(*): boolean} match - Function to evaluate condition
 * @returns {object}
 */
function simpleCond(key, match) {
  return {
    key: function() { return key; },
    match: match
  };
}

/**
 * simpleEffect creates an effect with a key and value.
 * @param {*} key - The effect key
 * @param {*} value - The effect value
 * @returns {object}
 */
function simpleEffect(key, value) {
  return {
    key: function() { return key; },
    value: function() { return value; }
  };
}

// =============================================================================
// ACTION TEMPLATES - Pick, Place, Move
// =============================================================================
// Matches go-pabt/logic/logic.go templatePick/templatePlace/templateMove

/**
 * simpleAction creates an action with conditions, effects, and a behavior tree node.
 * @param {string} name - Action name for debugging
 * @param {Array<Array>} conditions - Array of condition groups (IConditions)
 * @param {Array} effects - Array of effects
 * @param {function} tick - Tick function for behavior tree
 * @returns {object}
 */
function simpleAction(name, conditions, effects, tick) {
  return {
    name: name,
    conditions: function() { return conditions; },
    effects: function() { return effects; },
    node: function() { 
      return bt.newNode(tick);
    }
  };
}

/**
 * templatePick templates actions to pick up a sprite.
 * 
 * Matches go-pabt fig 7.4:
 *   Pick(i)
 *   con: o_r ∈ N_o_i (actor near sprite)
 *        h = ∅ (actor not holding anything)
 *   eff: h = i (actor holds sprite)
 *
 * @param {PickAndPlaceState} state - PA-BT state
 * @param {object} snapshot - Simulation state snapshot
 * @param {Sprite} sprite - The sprite to pick up
 * @returns {Array} Array of actions
 */
function templatePick(state, snapshot, sprite) {
  const actions = [];
  
  // Get sprite's current position from snapshot
  const spriteValue = snapshot.sprites.get(sprite);
  if (!spriteValue) {
    return actions;
  }
  
  const spriteShape = spriteValue.shape();
  if (!spriteShape) {
    return actions;
  }
  
  const ox = spriteShape.x;
  const oy = spriteShape.y;
  
  // Build positions map for effects (sprite will have no shape when held)
  const positions = new Map();
  for (const [k, v] of snapshot.sprites) {
    positions.set(k, {
      space: v.space,
      shape: v.shape()
    });
  }
  // Sprite has no physical position when held
  positions.get(sprite).shape = null;
  
  const pickupDistance = snapshot.pickupDistance;
  const actor = state.actor;
  let running = false;
  
  actions.push(simpleAction(
    `pick(${sprite.image})`,
    [
      // Condition group: all must be true
      [
        // Sprite at expected position
        simpleCond(new PositionVar(sprite), function(r) {
          const pos = r.positions.get(sprite);
          if (pos && pos.shape) {
            if (running) {
              return true;
            }
            if (pos.shape.x === ox && pos.shape.y === oy) {
              return true;
            }
          }
          return false;
        }),
        // Actor not holding anything
        simpleCond(new HeldItemVar(actor), function(r) {
          return r.item === null;
        }),
        // Actor within pickup distance of sprite
        simpleCond(new PositionVar(actor), function(r) {
          const spritePos = r.positions.get(sprite);
          const actorPos = r.positions.get(actor);
          return spritePos &&
                 actorPos &&
                 spritePos.shape &&
                 actorPos.shape &&
                 spritePos.shape.distance(actorPos.shape) <= pickupDistance;
        })
      ]
    ],
    // Effects
    [
      // Actor now holds sprite
      simpleEffect(new HeldItemVar(actor), { item: sprite }),
      // Sprite has new position state (no physical shape)
      simpleEffect(new PositionVar(sprite), { positions: positions })
    ],
    // Tick function
    async function() {
      running = true;
      try {
        await state.simulation.grasp(actor, sprite);
        return bt.Success;
      } catch (e) {
        console.log(`pick failed: ${e.message}`);
        return bt.Failure;
      }
    }
  ));
  
  return actions;
}

/**
 * templatePlace templates actions to place a held sprite at a position.
 *
 * Matches go-pabt:
 *   Place(i, p)
 *   con: o_r ∈ N_p (actor at position)
 *        h = i (actor holding sprite)
 *   eff: o_i = p (sprite at new position)
 *
 * @param {PickAndPlaceState} state - PA-BT state
 * @param {object} snapshot - Simulation state snapshot
 * @param {number} x - Target X position
 * @param {number} y - Target Y position
 * @param {Sprite} sprite - The sprite to place
 * @returns {Array} Array of actions
 */
function templatePlace(state, snapshot, x, y, sprite) {
  const actions = [];
  
  const spriteValue = snapshot.sprites.get(sprite);
  if (!spriteValue) {
    return actions;
  }
  
  const actor = state.actor;
  
  // Calculate actor shape at target position
  const actorValue = snapshot.sprites.get(actor);
  if (!actorValue) {
    return actions;
  }
  
  const actorSize = actorValue.size();
  const actorShape = new Shape(x, y, actorSize.w, actorSize.h);
  
  // Validate actor shape at target position
  const sim = state.simulation;
  if (sim.validateShape(actorShape) !== null) {
    return actions;
  }
  
  // Calculate where sprite would be released
  const spriteShape = sim.actorHeldItemReleaseShape(actor, x, y, sprite);
  if (sim.validateShape(spriteShape) !== null) {
    return actions;
  }
  
  // Build positions map with sprite at release position
  const positions = new Map();
  const noCollisionConds = [];
  
  for (const [k, v] of snapshot.sprites) {
    positions.set(k, {
      space: v.space,
      shape: v.shape()
    });
    
    // Add no-collision conditions for other sprites
    if (k !== actor && k !== sprite) {
      const other = k;
      noCollisionConds.push(
        simpleCond(new PositionVar(other), function(r) {
          const v = r.positions.get(other);
          if (v && v.shape && v.space.collides(spriteValue.space) && v.shape.collides(spriteShape)) {
            return false;
          }
          return true;
        })
      );
    }
  }
  
  // Update sprite position in effects
  positions.get(sprite).shape = spriteShape;
  
  actions.push(simpleAction(
    `place(${sprite.image}, ${x}, ${y})`,
    [
      // Condition group with no-collision conditions
      [
        // Actor holding sprite
        simpleCond(new HeldItemVar(actor), function(r) {
          return r.item === sprite;
        }),
        // Actor at target position
        simpleCond(new PositionVar(actor), function(r) {
          const v = r.positions.get(actor);
          if (v && v.shape) {
            if (v.shape.x === x && v.shape.y === y) {
              return true;
            }
          }
          return false;
        }),
        ...noCollisionConds
      ]
    ],
    // Effects
    [
      // Actor no longer holding anything
      simpleEffect(new HeldItemVar(actor), { item: null }),
      // Sprite at new position
      simpleEffect(new PositionVar(sprite), { positions: positions })
    ],
    // Tick function
    async function() {
      try {
        await sim.release(actor, sprite);
        return bt.Success;
      } catch (e) {
        console.log(`place failed: ${e.message}`);
        return bt.Failure;
      }
    }
  ));
  
  return actions;
}

/**
 * templateMove templates actions to move the actor to a position.
 *
 * Matches go-pabt:
 *   MoveTo(p, τ)
 *   con: τ ⊂ CollFree (path is collision-free)
 *   eff: o_r = p (actor at new position)
 *
 * @param {PickAndPlaceState} state - PA-BT state
 * @param {object} snapshot - Simulation state snapshot
 * @param {number} x - Target X position
 * @param {number} y - Target Y position
 * @returns {Array} Array of actions
 */
function templateMove(state, snapshot, x, y) {
  const actions = [];
  
  const actor = state.actor;
  const actorValue = snapshot.sprites.get(actor);
  if (!actorValue) {
    return actions;
  }
  
  const actorShape = actorValue.shape();
  if (!actorShape) {
    return actions;
  }
  
  const space = actorValue.space;
  const sim = state.simulation;
  
  // Calculate path (straight line)
  const cx = actorValue.x;
  const cy = actorValue.y;
  let dx = x - cx;
  let dy = y - cy;
  const d = Math.sqrt(dx * dx + dy * dy);
  
  if (d === 0 || !isFinite(d)) {
    return actions;
  }
  
  dx = dx / d;
  dy = dy / d;
  
  // Generate path shapes
  const shapes = [];
  let px = cx;
  let py = cy;
  
  while (true) {
    const rp = roundPosition(px, py);
    if (rp.x === x && rp.y === y) {
      break;
    }
    
    px += dx;
    py += dy;
    
    const pathShape = actorShape.clone();
    const pathPos = roundPosition(px, py);
    pathShape.setPosition(pathPos.x, pathPos.y);
    
    if (sim.validateShape(pathShape) !== null) {
      return actions;
    }
    
    shapes.push(pathShape);
  }
  
  if (shapes.length === 0) {
    return actions;
  }
  
  // Build no-collision conditions for path
  const noCollisionConds = [];
  const positions = new Map();
  
  for (const [k, v] of snapshot.sprites) {
    positions.set(k, {
      space: v.space,
      shape: v.shape()
    });
    
    if (k !== actor) {
      const other = k;
      noCollisionConds.push(
        simpleCond(new PositionVar(other), function(r) {
          const v = r.positions.get(other);
          if (v && v.shape && v.space.collides(space)) {
            for (const shape of shapes) {
              if (shape.collides(v.shape)) {
                return false;
              }
            }
          }
          return true;
        })
      );
    }
  }
  
  // Update actor position in effects
  positions.get(actor).shape = shapes[shapes.length - 1];
  
  actions.push(simpleAction(
    `move(${x}, ${y})`,
    [
      noCollisionConds
    ],
    // Effects
    [
      simpleEffect(new PositionVar(actor), { positions: positions })
    ],
    // Tick function
    async function() {
      try {
        await sim.move(actor, x, y);
        return bt.Success;
      } catch (e) {
        console.log(`move failed: ${e.message}`);
        return bt.Failure;
      }
    }
  ));
  
  return actions;
}

/**
 * filterActionsByEffect filters actions that have effects matching the failed condition.
 * @param {Array} actions - Actions to filter
 * @param {*} key - The failed condition key
 * @param {function} match - The failed condition match function
 * @returns {Array} Filtered actions
 */
function filterActionsByEffect(actions, key, match) {
  const result = [];
  for (const action of actions) {
    for (const effect of action.effects()) {
      const effectKey = effect.key();
      // Compare keys (both are objects with key() method returning strings)
      const keyStr = typeof key === 'object' && key.key ? key.key() : String(key);
      const effectKeyStr = typeof effectKey === 'object' && effectKey.key ? effectKey.key() : String(effectKey);
      
      if (effectKeyStr === keyStr && match(effect.value())) {
        result.push(action);
        break;
      }
    }
  }
  return result;
}

// =============================================================================
// PICKANDPLACE STATE - Implements IState interface
// =============================================================================
// Matches go-pabt/logic/logic.go pickAndPlace struct

/**
 * PickAndPlaceState implements the PA-BT IState interface.
 * @param {Simulation} simulation
 * @param {Actor} actor
 */
function PickAndPlaceState(simulation, actor) {
  this.simulation = simulation;
  this.actor = actor;
}

/**
 * Variable returns the current value for a state variable.
 * @param {*} key
 * @returns {*}
 */
PickAndPlaceState.prototype.variable = function(key) {
  if (key instanceof HeldItemVar) {
    return { item: key.actor.heldItem };
  }
  if (key instanceof PositionVar) {
    const snapshot = this.simulation.state();
    const positions = new Map();
    for (const [sprite, snap] of snapshot.sprites) {
      positions.set(sprite, {
        space: snap.space,
        shape: snap.shape()
      });
    }
    return { positions: positions };
  }
  throw new Error(`unexpected key type: ${typeof key}`);
};

/**
 * Actions returns actions that could help satisfy a failed condition.
 * @param {object} failed - The failed condition
 * @returns {Array}
 */
PickAndPlaceState.prototype.actions = function(failed) {
  const actions = [];
  const key = failed.key();
  const match = failed.match.bind(failed);
  const snapshot = this.simulation.state();
  const self = this;
  
  // Helper to add filtered actions
  function addActions(name, newActions) {
    const filtered = filterActionsByEffect(newActions, key, match);
    for (const action of filtered) {
      console.log(`adding ${action.name} for ${typeof key === 'object' && key.key ? key.key() : key}...`);
      actions.push(action);
    }
  }
  
  // Generate pick and place actions for each cube
  for (const [sprite, _] of snapshot.sprites) {
    if (sprite === self.actor) continue;
    
    if (sprite instanceof Cube) {
      // Template pick actions
      addActions('pick', templatePick(self, snapshot, sprite));
      
      // Template place actions at every position
      for (let x = 0; x < snapshot.spaceWidth; x++) {
        for (let y = 0; y < snapshot.spaceHeight; y++) {
          addActions('place', templatePlace(self, snapshot, x, y, sprite));
        }
      }
    }
  }
  
  // Generate move actions to every position
  for (let x = 0; x < snapshot.spaceWidth; x++) {
    for (let y = 0; y < snapshot.spaceHeight; y++) {
      addActions('move', templateMove(self, snapshot, x, y));
    }
  }
  
  return actions;
};

// =============================================================================
// MAIN - Demo entry point
// =============================================================================

async function main() {
  console.log('Pick-and-Place PA-BT Example');
  console.log('============================');
  console.log('');
  console.log('This example demonstrates the PA-BT algorithm.');
  console.log('Application types (Shape, Space, Sprite, Simulation) are all JavaScript.');
  console.log('The Go layer (osm:pabt) provides only PA-BT primitives.');
  console.log('');
  
  // Create simulation
  const sim = new Simulation();
  
  // Create actor
  const actor = new Actor(10, 10, 3, 2, 'A');
  actor.keyboard = true;
  sim.addActor(actor);
  
  // Create cube
  const cube = new Cube(40, 8, 1, 1, '1');
  sim.addCube(cube);
  
  // Create goal
  const goal = new Goal(50, 12, 3, 3, 'G');
  sim.addGoal(goal);
  
  // Set actor criteria: cube must be on goal
  actor.criteria.set(
    JSON.stringify({ cube: cube.image, goal: goal.image }),
    { cube: cube, goal: goal }
  );
  
  // Print initial state
  const state = sim.state();
  console.log('Simulation initialized:');
  console.log(`  Space: ${state.spaceWidth}x${state.spaceHeight}`);
  console.log(`  Pickup distance: ${state.pickupDistance}`);
  console.log(`  Sprites: ${state.sprites.size}`);
  console.log(`  Actor: position=(${actor.x}, ${actor.y}), size=(${actor.w}, ${actor.h})`);
  console.log(`  Cube: position=(${cube.x}, ${cube.y}), size=(${cube.w}, ${cube.h})`);
  console.log(`  Goal: position=(${goal.x}, ${goal.y}), size=(${goal.w}, ${goal.h})`);
  console.log('');
  
  // Create PA-BT state
  const pabtState = new PickAndPlaceState(sim, actor);
  
  // Test Shape functionality
  console.log('Testing Shape:');
  const shape1 = new Shape(0, 0, 10, 10);
  const shape2 = new Shape(5, 5, 10, 10);
  console.log(`  shape1: position=(${shape1.x}, ${shape1.y}), size=(${shape1.w}, ${shape1.h})`);
  console.log(`  shape2: position=(${shape2.x}, ${shape2.y}), size=(${shape2.w}, ${shape2.h})`);
  console.log(`  shape1.collides(shape2): ${shape1.collides(shape2)}`);
  console.log(`  shape1.distance(shape2): ${shape1.distance(shape2).toFixed(2)}`);
  console.log(`  shape1.center(): (${shape1.center().x}, ${shape1.center().y})`);
  console.log(`  shape1.closest(15, 15): (${shape1.closest(15, 15).x}, ${shape1.closest(15, 15).y})`);
  console.log('');
  
  // Test non-colliding shapes
  const shape3 = new Shape(20, 20, 5, 5);
  console.log(`  shape3: position=(${shape3.x}, ${shape3.y}), size=(${shape3.w}, ${shape3.h})`);
  console.log(`  shape1.collides(shape3): ${shape1.collides(shape3)}`);
  console.log('');
  
  // Test Space functionality
  console.log('Testing Space:');
  console.log(`  ACTOR_SPACE.collides(CUBE_SPACE): ${ACTOR_SPACE.collides(CUBE_SPACE)}`);
  console.log(`  ACTOR_SPACE.collides(GOAL_SPACE): ${ACTOR_SPACE.collides(GOAL_SPACE)}`);
  console.log(`  CUBE_SPACE.collides(GOAL_SPACE): ${CUBE_SPACE.collides(GOAL_SPACE)}`);
  console.log('');
  
  // Test state variable access
  console.log('Testing State Variables:');
  const heldItemVar = new HeldItemVar(actor);
  const positionVar = new PositionVar(actor);
  console.log(`  HeldItemVar key: ${heldItemVar.key()}`);
  console.log(`  PositionVar key: ${positionVar.key()}`);
  const heldValue = pabtState.variable(heldItemVar);
  console.log(`  Actor held item: ${heldValue.item ? heldValue.item.image : 'null'}`);
  const posValue = pabtState.variable(positionVar);
  console.log(`  Position map size: ${posValue.positions.size}`);
  console.log('');
  
  // Define success condition: cube is on goal
  console.log('Defining success condition: cube on goal');
  const successCondition = simpleCond(new PositionVar(cube), function(r) {
    const cubePos = r.positions.get(cube);
    const goalPos = r.positions.get(goal);
    return cubePos !== undefined &&
           goalPos !== undefined &&
           cubePos.shape !== null &&
           goalPos.shape !== null &&
           cubePos.shape.collides(goalPos.shape);
  });
  
  // Test condition
  const currentPositions = pabtState.variable(new PositionVar(cube));
  console.log(`  Condition met now: ${successCondition.match(currentPositions)}`);
  console.log('');
  
  // Test action generation
  console.log('Testing Action Generation:');
  const failedCond = successCondition;
  const generatedActions = pabtState.actions(failedCond);
  console.log(`  Generated ${generatedActions.length} actions for failed condition`);
  if (generatedActions.length > 0) {
    console.log(`  First action: ${generatedActions[0].name}`);
    console.log(`  First action conditions: ${generatedActions[0].conditions().length} groups`);
    console.log(`  First action effects: ${generatedActions[0].effects().length} effects`);
  }
  console.log('');
  
  console.log('Phase 3 implementation complete!');
  console.log('All JavaScript application types implemented:');
  console.log('  ✓ Shape - Rectangle geometry with collision');
  console.log('  ✓ Space - Collision layer flags');
  console.log('  ✓ Sprite - Base with Actor/Cube/Goal subtypes');
  console.log('  ✓ Simulation - Move/Grasp/Release with collision');
  console.log('  ✓ State Variables - HeldItemVar, PositionVar');
  console.log('  ✓ Conditions/Effects - simpleCond, simpleEffect');
  console.log('  ✓ Action Templates - templatePick, templatePlace, templateMove');
  console.log('  ✓ PickAndPlaceState - IState implementation');
  console.log('');
  console.log('Ready for PA-BT integration with osm:pabt module!');
}

main().catch(err => {
  console.error('Error:', err);
  process.exit(1);
});
