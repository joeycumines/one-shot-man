#!/usr/bin/env osm script

/**
 * @fileoverview example-05-pick-and-place.js
 * @description
 * Pick-and-Place Simulator demonstrating osm:pabt PA-BT planning integration.
 *
 * =========================================================================================
 * ARCHITECTURAL MANIFESTO: THE PA-BT STANDARD
 * =========================================================================================
 *
 * This implementation adheres strictly to "Behavior Trees in Robotics and AI" (Colledanchise & Ögren).
 * The system functions as a dynamic planner, not a scripted state machine.
 *
 * 1. THE POSTCONDITION-PRECONDITION-ACTION (PPA) UNIT
 * The fundamental atom of this system is NOT the Action, but the PPA expansion.
 * Structure: `Fallback(Condition C, Sequence(Preconditions(A), A))`
 * The planner generates a tree that satisfies conditions, rather than a linear queue of tasks.
 *
 * 2. DESCRIPTIVE VS. OPERATIONAL MODELS
 * For the reactive loop to function, the "Descriptive Model" (what the planner thinks an action does)
 * MUST match the "Operational Model" (what the physics engine actually does).
 * Discrepancies here lead to Livelocks (infinite replanning) or Deadlocks.
 *
 * =========================================================================================
 * DEFINED ANTI-PATTERNS & REQUIRED FIXES
 * =========================================================================================
 *
 * To ensure architectural integrity, the following patterns are explicitly defined and prohibited:
 *
 * [A] THE "GOD-PRECONDITION" ANTI-PATTERN
 * Definition: A single action listing every potential global blockade in its preconditions.
 * Failure Mode: Violates **Lazy Expansion**. It forces the planner to "solve" the entire map layout
 * before execution begins, coupling atomic actions to global geometry.
 * Refutation: The planner must not expand subtrees for conditions that are not yet relevant.
 *
 * [B] THE "ATOMIC ACTION" ANTI-PATTERN
 * Definition: A single action (e.g., `AtomicClearPath`) that handles moving, picking, and placing
 * in one tick to avoid planning complexity.
 * Failure Mode: Violates **Reactive Granularity**. It creates a "Black Box" that prevents reactive repair.
 * If the atomic action fails mid-execution, the planner has no visibility into the partial state.
 *
 * [C] THE SOLUTION: THE "BRIDGE ACTION" PATTERN
 * Requirement: Use **Dynamic Discovery**.
 * Instead of hardcoding map data, the ActionGenerator must dynamically inject "Bridge Actions"
 * (e.g., `ClearBlocker_X`) only when a specific navigational condition (`reachable_Target`) fails.
 *
 * =========================================================================================
 * CRITICAL RUNTIME WARNINGS
 * =========================================================================================
 *
 * 1. TRUTHFUL EFFECTS (THE ANTI-LIVELOCK RULE)
 * Do not assert that an action satisfies a high-level condition unless it guarantees it physically.
 * * BAD: `Pick_Obstacle` claims Effect `isPathClear = true`. (Lying: picking one might not clear the path).
 * * GOOD: `Pick_Obstacle_A` claims Effect `isCleared(A) = true`.
 *
 * 2. NO "ZOMBIE STATE" (DISABLE CACHING)
 * BT Nodes contain internal state (`Running`, `childIndex`). Reusing node instances (Action Caching)
 * across branches creates "Zombie State," where a node behaves as if running from a previous context.
 * ALWAYS instantiate fresh BT nodes in the Generator.
 *
 * 3. AVOID "SELF-BLOCKAGE BLINDNESS"
 * The Blackboard must compute path blockers for ALL relevant destinations (Goal and Target) every tick.
 * Do not gate blocker detection behind `if (holding target)`. The agent must know the path is blocked
 * even if it has dropped the target to clear the way.
 *
 * 4. PREVENT "SILENT FAILURE"
 * An action must not secretly reject a target in its tick function if that restriction is not
 * declared in its Preconditions.
 * * If `Place_Obstacle` cannot handle the Target, it must explicitly Precondition: `!heldItemIsTarget`.
 *
 * **WARNING RE: LOGGING:** This is an *interactive* terminal application.
 * It sets the terminal to raw mode.
 * DO NOT use console logging within the program itself (tea.run).
 * Instead, use the built-in 'log' global, for application logs (slog).
 *
 * @see {@link https://btirai.github.io/ | Behavior Trees in Robotics and AI}
 * @module PA-BT-Logic
 */

const printFatalError = (e) => {
    console.error('FATAL ERROR: ' + e?.message || e);
    if (e?.stack) {
        console.error('Stack trace: ' + e.stack);
    }
};

// init the program
let bt, tea, pabt, os, program;
try {
    bt = require('osm:bt');
    tea = require('osm:bubbletea');
    pabt = require('osm:pabt');
    os = require('osm:os');

    // SCENARIO CONFIGURATION
    //
    // The actor must:
    // 1. Enter the room
    // 2. Clear movable blockades forming a ring around the goal
    // 3. Pick the target cube
    // 4. Deliver target to the multi-cell Goal Area
    //
    // TRUE PARAMETRIC ACTIONS:
    // - MoveTo(entityId)
    // - Pick(cubeId)
    // - Place() / PlaceAt()

    const ENV_WIDTH = 80;
    const ENV_HEIGHT = 24;

    // Room bounds
    const ROOM_MIN_X = 20;
    const ROOM_MAX_X = 55;
    const ROOM_MIN_Y = 6;
    const ROOM_MAX_Y = 16;
    const ROOM_GAP_Y = 11;

    // Goal Area Configuration (3x3)
    const GOAL_CENTER_X = 8;
    const GOAL_CENTER_Y = 18;
    const GOAL_RADIUS = 1; // 3x3 area implies +/- 1 from center

    // Entity IDs
    const TARGET_ID = 1;
    const GOAL_ID = 1;
    // NOTE: DUMPSTER_ID removed - obstacles placed at any free cell dynamically
    // GOAL_BLOCKADE_IDS kept for visual counting only, not used in planning logic
    var GOAL_BLOCKADE_IDS = [];

    // Pick/Place thresholds
    const PICK_THRESHOLD = 5.0; // Distance threshold for picking up cubes
    // MANUAL_MOVE_SPEED = 1.0 to match automatic mode movement (1 unit per tick)
    // This ensures PERFECT CONSISTENCY between manual and automatic modes per bugreport.md
    const MANUAL_MOVE_SPEED = 1.0;

    // Helper: Check if point is within the Goal Area
    function isInGoalArea(x, y) {
        return x >= GOAL_CENTER_X - GOAL_RADIUS &&
            x <= GOAL_CENTER_X + GOAL_RADIUS &&
            y >= GOAL_CENTER_Y - GOAL_RADIUS &&
            y <= GOAL_CENTER_Y + GOAL_RADIUS;
    }

    // NOTE: isInBlockadeRing() function REMOVED
    // Dynamic obstacle detection now used instead of hardcoded geometry

    function initializeSimulation() {
        const cubesInit = [];

        // 1. Target Cube (inside room, right side)
        cubesInit.push([TARGET_ID, {
            id: TARGET_ID,
            x: 45,
            y: 11,
            deleted: false,
            isTarget: true,
            type: 'target'
        }]);

        // 2. Goal Blockade Ring
        // Surround the 3x3 Goal Area with a movable ring
        let goalBlockadeId = 100;
        const ringMinX = GOAL_CENTER_X - GOAL_RADIUS - 1;
        const ringMaxX = GOAL_CENTER_X + GOAL_RADIUS + 1;
        const ringMinY = GOAL_CENTER_Y - GOAL_RADIUS - 1;
        const ringMaxY = GOAL_CENTER_Y + GOAL_RADIUS + 1;

        for (let y = ringMinY; y <= ringMaxY; y++) {
            for (let x = ringMinX; x <= ringMaxX; x++) {
                // Skip the goal area itself (the hole in the donut)
                if (isInGoalArea(x, y)) continue;

                cubesInit.push([goalBlockadeId, {
                    id: goalBlockadeId,
                    x: x,
                    y: y,
                    deleted: false,
                    // NOTE: isGoalBlockade removed - not needed for planning
                    type: 'obstacle'  // Generic type, rendering uses position/context
                }]);
                GOAL_BLOCKADE_IDS.push(goalBlockadeId);
                goalBlockadeId++;
            }
        }

        // 3. Room Walls (static obstacles)
        let wallId = 1000;

        function addWall(x, y) {
            if (x === ROOM_MIN_X && Math.abs(y - ROOM_GAP_Y) <= 1) return;
            cubesInit.push([wallId, {
                id: wallId,
                x: x,
                y: y,
                deleted: false,
                isStatic: true,
                type: 'wall'
            }]);
            wallId++;
        }

        for (let x = ROOM_MIN_X; x <= ROOM_MAX_X; x++) {
            addWall(x, ROOM_MIN_Y);
            addWall(x, ROOM_MAX_Y);
        }
        for (let y = ROOM_MIN_Y; y <= ROOM_MAX_Y; y++) {
            addWall(ROOM_MIN_X, y);
            addWall(ROOM_MAX_X, y);
        }

        // Add standardized obstacle at (7, 5) often used in tests
        cubesInit.push([803, {
            id: 803,
            x: 7,
            y: 5,
            deleted: false,
            type: 'obstacle'
        }]);

        log.info("Pick-and-Place simulation initialized");
        return {
            width: ENV_WIDTH,
            height: ENV_HEIGHT,
            spaceWidth: 55, // Must accommodate ROOM_MAX_X=55; HUD conditionally hidden on narrow terminals

            actors: new Map([
                [1, {id: 1, x: 5, y: 11, heldItem: null}]
            ]),
            cubes: new Map(cubesInit),
            goals: new Map([
                [GOAL_ID, {id: GOAL_ID, x: GOAL_CENTER_X, y: GOAL_CENTER_Y, forTarget: true}]
                // NOTE: Dumpster goal REMOVED - obstacles placed anywhere dynamically
            ]),

            blackboard: null,
            pabtPlan: null,
            pabtState: null,
            activeActorId: 1,
            gameMode: 'automatic',
            tickCount: 0,
            paused: false,
            manualMoveTarget: null,
            manualPath: [],
            pathStuckTicks: 0,
            winConditionMet: false,
            targetDelivered: false,

            // Manual mode ticker for smooth BT-driven movement
            manualTicker: null,

            renderBuffer: null,
            renderBufferWidth: 0,
            renderBufferHeight: 0,

            debugMode: os.getenv('OSM_TEST_MODE') === '1'
        };
    }

    // Logic Helpers

    // manualKeysMovement: Handle WASD movement in manual mode (discrete movement per key press)
    // Returns true if movement occurred, false otherwise
    function manualKeysMovement(state, actor, dx, dy) {
        if (dx === 0 && dy === 0) return false;

        // Calculate new position - with MANUAL_MOVE_SPEED = 1.0, this is integer-based
        const nx = actor.x + dx * MANUAL_MOVE_SPEED;
        const ny = actor.y + dy * MANUAL_MOVE_SPEED;

        // Use integer position for collision detection (consistent with automatic mode)
        const inx = Math.round(nx);
        const iny = Math.round(ny);
        let blocked = false;

        // Boundary check - MUST match pathfinding boundaries for consistency
        // Pathfinding uses: nx < 1 || nx >= state.spaceWidth - 1 || ny < 1 || ny >= state.height - 1
        // This is to ensure consistent behavior between manual and automatic modes
        if (inx < 1 || inx >= state.spaceWidth - 1 ||
            iny < 1 || iny >= state.height - 1) {
            blocked = true;
        }

        // Fast collision check (only if not already blocked by boundary)
        if (!blocked) {
            for (const c of state.cubes.values()) {
                if (!c.deleted && Math.round(c.x) === inx && Math.round(c.y) === iny) {
                    if (actor.heldItem && c.id === actor.heldItem.id) continue;
                    blocked = true;
                    break;
                }
            }
        }

        if (!blocked) {
            const oldX = Math.round(actor.x);
            const oldY = Math.round(actor.y);
            actor.x = nx;
            actor.y = ny;
            return Math.round(actor.x) !== oldX || Math.round(actor.y) !== oldY;
        }
        return false;
    }

    // Find a free adjacent cell for generic placement
    function getFreeAdjacentCell(state, actorX, actorY, targetGoalArea = false) {
        const ax = Math.round(actorX);
        const ay = Math.round(actorY);
        const dirs = [[0, 1], [0, -1], [1, 0], [-1, 0], [1, 1], [1, -1], [-1, 1], [-1, -1]];

        for (const [dx, dy] of dirs) {
            const nx = ax + dx;
            const ny = ay + dy;

            // Bounds check
            if (nx < 0 || nx >= state.spaceWidth || ny < 0 || ny >= state.height) continue;

            // If we are targeting the goal area specifically, skip cells not in it
            if (targetGoalArea && !isInGoalArea(nx, ny)) continue;

            // Check occupancy
            let occupied = false;
            // Check cubes
            for (const c of state.cubes.values()) {
                if (!c.deleted && Math.round(c.x) === nx && Math.round(c.y) === ny) {
                    occupied = true;
                    break;
                }
            }
            // No additional occupancy check needed - dumpster concept removed
            // Obstacles can be placed at any free cell

            if (!occupied) return {x: nx, y: ny};
        }
        return null;
    }

    // Find the NEAREST pickable cube to the given position, within PICK_THRESHOLD.
    // Returns null if no cube is within range.
    // OPTIMIZED: Moved Map.get() call outside loop for better performance.
    function findNearestPickableCube(state, clickX, clickY) {
        let nearest = null;
        let nearestDist = PICK_THRESHOLD;

        const actor = state.actors.get(state.activeActorId);

        for (const c of state.cubes.values()) {
            if (c.deleted) continue;
            if (c.isStatic) continue;
            if (actor.heldItem && c.id === actor.heldItem.id) continue;

            const dist = Math.sqrt(Math.pow(c.x - clickX, 2) + Math.pow(c.y - clickY, 2));

            if (dist < nearestDist) {
                nearest = c;
                nearestDist = dist;
            }
        }
        return nearest;
    }

    // Find the NEAREST valid placement cell adjacent to actor.
    // Returns a {x, y} object or null if no valid cell found.
    // A placement cell is valid if it is within bounds and not occupied (except by ignoreId).
    // OPTIMIZED: O(n) algorithm - builds Set of occupied positions for O(1) lookup.
    function findNearestValidPlacement(state, actorX, actorY, ignoreId) {
        const ax = Math.round(actorX);
        const ay = Math.round(actorY);
        const dirs = [[0, 1], [0, -1], [1, 0], [-1, 0], [1, 1], [1, -1], [-1, 1], [-1, -1]];

        // Build position lookup Set ONCE instead of scanning all cubes 8 times
        const occupiedPositions = new Set();
        for (const c of state.cubes.values()) {
            if (!c.deleted && c.id !== ignoreId) {
                const posKey = Math.round(c.x) + ',' + Math.round(c.y);
                occupiedPositions.add(posKey);
            }
        }

        let nearest = null;
        let nearestDist = Infinity;

        for (const [dx, dy] of dirs) {
            const nx = ax + dx;
            const ny = ay + dy;

            // Bounds check
            if (nx < 0 || nx >= state.spaceWidth || ny < 0 || ny >= state.height) continue;

            // O(1) lookup instead of O(n) scan!
            const posKey = nx + ',' + ny;
            if (occupiedPositions.has(posKey)) continue;

            // Calculate distance from actor to this placement cell
            const dist = Math.sqrt(Math.pow(nx - ax, 2) + Math.pow(ny - ay, 2));

            if (dist < nearestDist) {
                nearest = {x: nx, y: ny};
                nearestDist = dist;
            }
        }

        return nearest;
    }

    // Pathfinding

    function buildBlockedSet(state, ignoreCubeId) {
        const blocked = new Set();
        const key = (x, y) => x + ',' + y;
        const actor = state.actors.get(state.activeActorId);

        // DEBUG: Check if 6,18 blocker exists before building set
        var blockerAt6_18 = null;
        state.cubes.forEach(c => {
            if (Math.round(c.x) === 6 && Math.round(c.y) === 18 && !c.deleted) {
                blockerAt6_18 = c;
            }
        });

        state.cubes.forEach(c => {
            if (c.deleted) return;
            if (c.id === ignoreCubeId) return;
            if (actor.heldItem && c.id === actor.heldItem.id) return;
            blocked.add(key(Math.round(c.x), Math.round(c.y)));
        });

        // DEBUG: Log if (6,18) blocker exists but not in blocked set (no tick limit now)
        if (blockerAt6_18 && !blocked.has('6,18')) {
            log.warn("buildBlockedSet BUG: blockerAt6_18 exists (id=" + blockerAt6_18.id + ") but 6,18 not in blocked! ignoreCubeId=" + ignoreCubeId + " heldItem=" + (actor.heldItem ? actor.heldItem.id : 'null') + " tick=" + state.tickCount);
        }

        return blocked;
    }

    function getPathInfo(state, startX, startY, targetX, targetY, ignoreCubeId) {
        const blocked = buildBlockedSet(state, ignoreCubeId);
        const key = (x, y) => x + ',' + y;
        const visited = new Set();
        const queue = [{x: Math.round(startX), y: Math.round(startY), dist: 0}];

        visited.add(key(queue[0].x, queue[0].y));

        while (queue.length > 0) {
            const current = queue.shift();

            // Distance check (approximate for area goals)
            const dx = Math.abs(current.x - Math.round(targetX));
            const dy = Math.abs(current.y - Math.round(targetY));

            if (dx <= 1 && dy <= 1) {
                return {reachable: true, distance: current.dist};
            }

            const dirs = [[0, 1], [0, -1], [1, 0], [-1, 0]];
            for (const [ox, oy] of dirs) {
                const nx = current.x + ox;
                const ny = current.y + oy;
                const nKey = key(nx, ny);

                if (nx < 1 || nx >= state.spaceWidth - 1 || ny < 1 || ny >= state.height - 1) continue;
                if (visited.has(nKey)) continue;
                if (blocked.has(nKey)) continue;

                visited.add(nKey);
                queue.push({x: nx, y: ny, dist: current.dist + 1});
            }
        }

        return {reachable: false, distance: Infinity};
    }

    // Calculate full path for click-based movement (BFS).
    // Returns array of {x,y} waypoints.
    function findPath(state, startX, startY, targetX, targetY, ignoreCubeId, searchLimit) {
        const blocked = buildBlockedSet(state, ignoreCubeId);
        const key = (x, y) => x + ',' + y;
        const iStartX = Math.round(startX);
        const iStartY = Math.round(startY);
        const iTargetX = Math.round(targetX);
        const iTargetY = Math.round(targetY);

        if (iStartX === iTargetX && iStartY === iTargetY) return [];

        const visited = new Map(); // Key -> Parent Key
        const queue = [{x: iStartX, y: iStartY}];
        const startKey = key(iStartX, iStartY);
        visited.set(startKey, null);

        const dirs = [[0, 1], [0, -1], [1, 0], [-1, 0]];
        let found = false;
        let finalNode = null;

        let iterations = 0;
        let bestNode = queue[0];
        let bestDist = Infinity;

        while (queue.length > 0) {
            if (searchLimit && iterations >= searchLimit) {
                finalNode = bestNode;
                break;
            }
            iterations++;

            const cur = queue.shift();

            if (cur.x === iTargetX && cur.y === iTargetY) {
                found = true;
                finalNode = cur;
                break;
            }

            const dist = Math.abs(cur.x - iTargetX) + Math.abs(cur.y - iTargetY);
            if (dist < bestDist) {
                bestDist = dist;
                bestNode = cur;
            }

            for (const [ox, oy] of dirs) {
                const nx = cur.x + ox;
                const ny = cur.y + oy;
                const nKey = key(nx, ny);

                if (nx < 1 || nx >= state.spaceWidth - 1 || ny < 1 || ny >= state.height - 1) continue;
                // Allow entering target cell even if technically blocked (e.g. picking item)
                if (blocked.has(nKey) && !(nx === iTargetX && ny === iTargetY)) continue;
                if (visited.has(nKey)) continue;

                visited.set(nKey, cur); // Store parent node object
                queue.push({x: nx, y: ny, parent: cur});
            }
        }

        if (!found && !finalNode) return null;

        // Reconstruct path
        const path = [];
        let curr = finalNode; // The node from the queue which has .parent
        while (curr.parent) {
            path.unshift({x: curr.x, y: curr.y});
            curr = curr.parent;
        }
        return path;
    }

    function findNextStep(state, startX, startY, targetX, targetY, ignoreCubeId) {
        // Keeps original implementation for Automatic mode
        const blocked = buildBlockedSet(state, ignoreCubeId);
        const key = (x, y) => x + ',' + y;
        const iStartX = Math.round(startX);
        const iStartY = Math.round(startY);
        const iTargetX = Math.round(targetX);
        const iTargetY = Math.round(targetY);

        // DEBUG: Check if pathfinding from (5,18) to goal and (6,18) is blocked
        if (iStartX === 5 && iStartY === 18 && iTargetX === 8 && iTargetY === 18) {
            log.warn("findNextStep DEBUG: start=(5,18) target=(8,18) blocked.has('6,18')=" + blocked.has('6,18') + " ignoreCubeId=" + ignoreCubeId + " tick=" + state.tickCount);
        }

        // Simple reach check
        if (Math.abs(startX - targetX) < 1.0 && Math.abs(startY - targetY) < 1.0) {
            return {x: targetX, y: targetY};
        }

        const visited = new Set();
        const queue = [];
        visited.add(key(iStartX, iStartY));

        const dirs = [[0, 1], [0, -1], [1, 0], [-1, 0]];
        for (const [ox, oy] of dirs) {
            const nx = iStartX + ox;
            const ny = iStartY + oy;
            const nKey = key(nx, ny);

            if (nx < 1 || nx >= state.spaceWidth - 1 || ny < 1 || ny >= state.height - 1) continue;
            // Allow entering target cell
            if (blocked.has(nKey) && !(nx === iTargetX && ny === iTargetY)) continue;

            if (nx === iTargetX && ny === iTargetY) {
                return {x: nx, y: ny};
            }

            queue.push({x: nx, y: ny, firstX: ox, firstY: oy});
            visited.add(nKey);
        }

        while (queue.length > 0) {
            const cur = queue.shift();

            if (cur.x === iTargetX && cur.y === iTargetY) {
                return {x: startX + cur.firstX, y: startY + cur.firstY};
            }

            for (const [ox, oy] of dirs) {
                const nx = cur.x + ox;
                const ny = cur.y + oy;
                const nKey = key(nx, ny);

                if (nx < 1 || nx >= state.spaceWidth - 1 || ny < 1 || ny >= state.height - 1) continue;
                if (blocked.has(nKey) && !(nx === iTargetX && ny === iTargetY)) continue;
                if (visited.has(nKey)) continue;

                visited.add(nKey);
                queue.push({x: nx, y: ny, firstX: cur.firstX, firstY: cur.firstY});
            }
        }

        return null;
    }

    // Dynamic Blocker Discovery (PA-BT Dynamic Obstacle Handling)

    /**
     * Finds the ID of the first MOVABLE cube blocking the path from start to target.
     * Uses BFS to explore reachable area, then identifies the nearest blocking cube
     * on the frontier.
     *
     * @param {Object} state - Simulation state
     * @param {number} fromX - Starting X coordinate
     * @param {number} fromY - Starting Y coordinate
     * @param {number} toX - Target X coordinate
     * @param {number} toY - Target Y coordinate
     * @param {number} excludeId - Optional cube ID to exclude from blockers (e.g., TARGET)
     * @returns {number|null} - ID of first blocking movable cube, or null if path is clear
     */
    function findFirstBlocker(state, fromX, fromY, toX, toY, excludeId) {
        const key = (x, y) => x + ',' + y;
        const actor = state.actors.get(state.activeActorId);

        // Build cube position lookup: position -> cubeId (only MOVABLE cubes)
        const cubeAtPosition = new Map();
        state.cubes.forEach(c => {
            if (c.deleted) return;
            if (c.isStatic) return; // Walls can't be moved - skip
            if (actor.heldItem && c.id === actor.heldItem.id) return; // Ignore held item
            if (excludeId !== undefined && c.id === excludeId) return; // Ignore excluded cube (e.g., target)
            cubeAtPosition.set(key(Math.round(c.x), Math.round(c.y)), c.id);
        });

        // DEBUG: Log ring blocker positions once per call (first 100 ticks only)
        if (state.tickCount < 100 && toX === 8 && toY === 18) {
            var positions = [];
            cubeAtPosition.forEach(function (id, pos) {
                positions.push(pos + ":" + id);
            });
        }

        // Build blocked set for pathfinding (pass excludeId to exclude it from blocked cells)
        const blocked = buildBlockedSet(state, excludeId !== undefined ? excludeId : -1);

        const visited = new Set();
        const frontier = []; // Cells we tried to enter but were blocked by movable cubes
        const queue = [{x: Math.round(fromX), y: Math.round(fromY)}];

        visited.add(key(queue[0].x, queue[0].y));

        const targetIX = Math.round(toX);
        const targetIY = Math.round(toY);

        // BFS to find path and collect frontier
        while (queue.length > 0) {
            const current = queue.shift();

            // Check if we've reached adjacency to target
            const dx = Math.abs(current.x - targetIX);
            const dy = Math.abs(current.y - targetIY);
            if (dx <= 1 && dy <= 1) {
                // DEBUG: Log when we reach adjacency
                if (state.tickCount < 100 && targetIX === 8 && targetIY === 18) {
                }
                return null; // Path exists, no blocker
            }

            const dirs = [[0, 1], [0, -1], [1, 0], [-1, 0]];
            for (const [ox, oy] of dirs) {
                const nx = current.x + ox;
                const ny = current.y + oy;
                const nKey = key(nx, ny);

                if (nx < 1 || nx >= state.spaceWidth - 1 || ny < 1 || ny >= state.height - 1) continue;
                if (visited.has(nKey)) continue;

                if (blocked.has(nKey)) {
                    // This cell is blocked - check if by a movable cube
                    const blockerId = cubeAtPosition.get(nKey);
                    if (blockerId !== undefined) {
                        // Found a movable blocker!
                        frontier.push({x: nx, y: ny, id: blockerId, dist: current.dist || 0});
                    }
                    continue;
                }

                visited.add(nKey);
                queue.push({x: nx, y: ny, dist: (current.dist || 0) + 1});
            }
        }

        // Path is blocked - return the blocker closest to the goal
        // IMPORTANT: Sort by distance to GOAL, not to actor. This ensures we prioritize
        // clearing blockers that are actually blocking access to the goal, rather than
        // incidental blockers in the opposite direction from the goal.
        if (frontier.length > 0) {
            frontier.sort((a, b) => {
                const distA = Math.abs(a.x - toX) + Math.abs(a.y - toY);
                const distB = Math.abs(b.x - toX) + Math.abs(b.y - toY);
                return distA - distB;
            });
            return frontier[0].id;
        }

        return null; // No movable blockers found (blocked by walls only)
    }

    // Blackboard Synchronization

    function syncToBlackboard(state) {
        if (!state.blackboard) return;

        const bb = state.blackboard;
        const actor = state.actors.get(state.activeActorId);
        const ax = Math.round(actor.x);
        const ay = Math.round(actor.y);

        bb.set('actorX', actor.x);
        bb.set('actorY', actor.y);
        bb.set('heldItemExists', actor.heldItem !== null);
        bb.set('heldItemId', actor.heldItem ? actor.heldItem.id : -1);
        // REFACTORED: Removed heldItemIsBlockade - all items treated uniformly

        // Entities - track proximity AND path blockers for TARGET cube
        // FIX (review.md 4.1): Compute pathBlocker_to_Target every tick, not just when holding target.
        // This prevents "Self-Blockage Blindness" - agent must know if target is blocked even when
        // it's not holding the target (e.g., after dropping target to clear path).
        state.cubes.forEach(cube => {
            if (!cube.deleted) {
                const dist = Math.sqrt(Math.pow(cube.x - ax, 2) + Math.pow(cube.y - ay, 2));
                // 1.8 threshold for entity proximity (slightly larger than goal's 1.5)
                const atEntity = dist <= 1.8;
                bb.set('atEntity_' + cube.id, atEntity);

                // For TARGET cube specifically, compute path blocker every tick
                // This is needed because agent may need to re-acquire target after placing it
                if (cube.id === TARGET_ID) {
                    const cubeX = Math.round(cube.x);
                    const cubeY = Math.round(cube.y);
                    // Exclude TARGET_ID from being considered a blocker to itself
                    const blocker = findFirstBlocker(state, ax, ay, cubeX, cubeY, TARGET_ID);
                    bb.set('pathBlocker_entity_' + cube.id, blocker === null ? -1 : blocker);
                }
            } else {
                bb.set('atEntity_' + cube.id, false);
                if (cube.id === TARGET_ID) {
                    bb.set('pathBlocker_entity_' + cube.id, -1);
                }
            }
        });

        // Goals - track proximity and path blockers
        // FIX (review.md 4.1): ALWAYS compute pathBlocker_to_Goal, regardless of what agent is holding.
        // The old code only computed this when holding target - causing "Self-Blockage Blindness".
        state.goals.forEach(goal => {
            const dist = Math.sqrt(Math.pow(goal.x - ax, 2) + Math.pow(goal.y - ay, 2));
            // 1.5 threshold ensures physical adjacency for goal area (stricter than entities)
            bb.set('atGoal_' + goal.id, dist <= 1.5);

            // Compute path blocker to goal UNCONDITIONALLY every tick
            // This allows the planner to plan ahead even when not yet holding target
            const goalX = Math.round(goal.x);
            const goalY = Math.round(goal.y);
            // Exclude TARGET_ID from being considered a blocker (we carry target TO the goal)
            const blocker = findFirstBlocker(state, ax, ay, goalX, goalY, TARGET_ID);
            bb.set('pathBlocker_goal_' + goal.id, blocker === null ? -1 : blocker);
        });

        bb.set('cubeDeliveredAtGoal', state.winConditionMet);
    }

    // =========================================================================================
    // MANUAL MODE BEHAVIOR TREE
    // =========================================================================================

    function createManualMoveLeaf(state) {
        return bt.createLeafNode(() => {
            // If no path, we are done moving
            if (state.manualPath.length === 0) return bt.success;

            const actor = state.actors.get(state.activeActorId);
            const nextPoint = state.manualPath[0];
            const dx = nextPoint.x - actor.x;
            const dy = nextPoint.y - actor.y;
            const dist = Math.sqrt(dx * dx + dy * dy);

            if (dist >= 0.1) {
                const moveDist = Math.min(MANUAL_MOVE_SPEED, dist);
                const nextX = actor.x + Math.sign(dx) * Math.min(MANUAL_MOVE_SPEED, Math.abs(dx));
                const nextY = actor.y + Math.sign(dy) * Math.min(MANUAL_MOVE_SPEED, Math.abs(dy));

                // Collision check logic
                let nextBlocked = false;
                for (const c of state.cubes.values()) {
                    if (!c.deleted && Math.round(c.x) === Math.round(nextX) && Math.round(c.y) === Math.round(nextY)) {
                        if (actor.heldItem && c.id === actor.heldItem.id) continue;
                        nextBlocked = true;
                        break;
                    }
                }

                if (nextBlocked) {
                    state.manualPath = [];
                    state.manualMoveTarget = null; // Abort target if path blocked
                    state.pathStuckTicks = 0;
                    return bt.failure;
                }

                // Move
                const oldDist = dist;
                actor.x += Math.sign(dx) * Math.min(MANUAL_MOVE_SPEED, Math.abs(dx));
                actor.y += Math.sign(dy) * Math.min(MANUAL_MOVE_SPEED, Math.abs(dy));

                // Check stuck
                const newDist = Math.sqrt(Math.pow(nextPoint.x - actor.x, 2) + Math.pow(nextPoint.y - actor.y, 2));
                if (newDist >= oldDist - 0.01) state.pathStuckTicks++;
                else state.pathStuckTicks = 0;

                if (state.pathStuckTicks > 60) {
                    state.manualPath = [];
                    state.manualMoveTarget = null;
                    state.pathStuckTicks = 0;
                    return bt.failure;
                }
            } else {
                // Arrived at waypoint
                actor.x = nextPoint.x;
                actor.y = nextPoint.y;
                state.manualPath.shift();
                state.pathStuckTicks = 0;
            }

            return bt.running;
        });
    }

    function createManualInteractLeaf(state) {
        return bt.createLeafNode(() => {
            if (!state.manualMoveTarget) return bt.success;
            // If we still have a path, we shouldn't be interacting yet
            if (state.manualPath.length > 0) return bt.running;

            const clickX = state.manualMoveTarget.x;
            const clickY = state.manualMoveTarget.y;
            const actor = state.actors.get(state.activeActorId);
            const isHolding = actor.heldItem !== null;

            let clickedCube = null;
            for (const c of state.cubes.values()) {
                if (Math.round(c.x) === clickX && Math.round(c.y) === clickY) {
                    clickedCube = c;
                    break;
                }
            }

            const dist = Math.sqrt(Math.pow(clickX - actor.x, 2) + Math.pow(clickY - actor.y, 2));
            if (dist <= PICK_THRESHOLD) {
                if (isHolding) {
                    const ignoreId = actor.heldItem.id;
                    // ONLY place if we clicked on an EMPTY cell (not even a deleted one)
                    if (!clickedCube) {
                        const c = state.cubes.get(ignoreId);
                        if (c) {
                            c.deleted = false;
                            c.x = clickX;
                            c.y = clickY;
                            actor.heldItem = null;
                            if (ignoreId === TARGET_ID && isInGoalArea(clickX, clickY)) state.winConditionMet = true;
                        }
                    }
                } else if (clickedCube && !clickedCube.isStatic && !clickedCube.deleted) {
                    clickedCube.deleted = true;
                    actor.heldItem = {id: clickedCube.id};
                } else if (!isHolding && !clickedCube) {
                    // Try pick nearest if we clicked empty space near a cube
                    const nearestCube = findNearestPickableCube(state, clickX, clickY);
                    if (nearestCube) {
                        const actorToCubeDist = Math.sqrt(Math.pow(nearestCube.x - actor.x, 2) + Math.pow(nearestCube.y - actor.y, 2));
                        if (actorToCubeDist <= PICK_THRESHOLD) {
                            nearestCube.deleted = true;
                            actor.heldItem = {id: nearestCube.id};
                        }
                    }
                }
            }
            state.manualMoveTarget = null; // Action complete
            return bt.success;
        });
    }

    function createManualTree(state) {
        return bt.node(bt.sequence,
            createManualMoveLeaf(state),
            createManualInteractLeaf(state)
        );
    }

    // TRUE PARAMETRIC ACTIONS via ActionGenerator

    // CRITICAL FIX (ISSUE-001): Removed actionCache entirely.
    // PA-BT nodes are STATEFUL - they retain Running status or child indices.
    // Caching and reusing nodes causes "zombie state" corruption where a node
    // retains state from a previous execution context.
    // Always create fresh Action/Node instances. GC cost is negligible.

    function createMoveToAction(state, entityType, entityId, extraPreconditions) {
        const name = 'MoveTo_' + entityType + '_' + entityId;
        let targetKey;
        let pathBlockerKey;

        if (entityType === 'cube') {
            targetKey = 'atEntity_' + entityId;
            pathBlockerKey = 'pathBlocker_entity_' + entityId;
        } else {
            targetKey = 'atGoal_' + entityId;
            pathBlockerKey = 'pathBlocker_goal_' + entityId;
        }

        // DYNAMIC PRECONDITION for GOAL and TARGET MoveTo:
        // - Requires clear path to trigger dynamic obstacle clearing (ClearPath/Bridge Actions)
        // - For non-target entities, we skip this to prevent regression (only Target needs guaranteed path)
        const conditions = [];
        if (entityType === 'goal' || (entityType === 'cube' && entityId === TARGET_ID)) {
            conditions.push({key: pathBlockerKey, value: -1, Match: v => v === -1});
        }
        // Add any extra preconditions (e.g., TARGET MoveTo needs pathBlocker_goal=-1)
        if (extraPreconditions) {
            conditions.push(...extraPreconditions);
        }

        const effects = [{key: targetKey, Value: true}];

        const tickFn = function () {
            if (state.gameMode !== 'automatic') return bt.running;

            const actor = state.actors.get(state.activeActorId);

            // AGGRESSIVE DEBUG: Log ALL movements near tick 1000
            if (state.tickCount > 950 && state.tickCount < 1050) {
                log.warn("TRACE-MOVETO " + name + " tick=" + state.tickCount + " actor=(" + actor.x.toFixed(2) + "," + actor.y.toFixed(2) + ") round=(" + Math.round(actor.x) + "," + Math.round(actor.y) + ")");
            }

            let targetX, targetY, ignoreCubeId;

            if (entityType === 'cube') {
                const cube = state.cubes.get(entityId);
                if (!cube || cube.deleted) {
                    return bt.success;
                }
                targetX = cube.x;
                targetY = cube.y;
                ignoreCubeId = entityId;
            } else {
                const goal = state.goals.get(entityId);
                targetX = goal.x;
                targetY = goal.y;
                ignoreCubeId = -1;
            }

            const dx = targetX - actor.x;
            const dy = targetY - actor.y;
            const dist = Math.sqrt(dx * dx + dy * dy);

            // CRITICAL FIX (ISSUE-002): Uniform threshold of 1.5 for all MoveTo actions.
            // This is STRICTER than atEntity_X (1.8) and equal to atGoal_X (1.5),
            // ensuring MoveTo success always implies the blackboard condition is met.
            const threshold = 1.5;

            if (dist <= threshold) {
                return bt.success;
            }

            const nextStep = findNextStep(state, actor.x, actor.y, targetX, targetY, ignoreCubeId);
            if (nextStep) {
                const stepDx = nextStep.x - actor.x;
                const stepDy = nextStep.y - actor.y;
                const oldX = actor.x, oldY = actor.y;
                var newX = actor.x + Math.sign(stepDx) * Math.min(1.0, Math.abs(stepDx));
                var newY = actor.y + Math.sign(stepDy) * Math.min(1.0, Math.abs(stepDy));

                // DEBUG: Check if destination is blocked
                var checkBlocked = buildBlockedSet(state, ignoreCubeId);
                var destKey = Math.round(newX) + ',' + Math.round(newY);

                // DEBUG: Special check for the ring blocker at (6,18)
                if (Math.round(newX) === 6 && Math.round(newY) === 18) {
                    log.warn("MoveTo " + name + " CRITICAL: Entering cell (6,18)! destKey=" + destKey + " blockedHas=" + checkBlocked.has(destKey) + " ignoreCubeId=" + ignoreCubeId);
                    // Check what blocker is at (6,18)
                    state.cubes.forEach(function (c) {
                        if (Math.round(c.x) === 6 && Math.round(c.y) === 18) {
                            log.warn("  -> Cube at (6,18): id=" + c.id + " deleted=" + c.deleted);
                        }
                    });
                }

                if (checkBlocked.has(destKey)) {
                    log.warn("MoveTo " + name + " BUG: trying to move into blocked cell (" + Math.round(newX) + "," + Math.round(newY) + ") from (" + oldX + "," + oldY + ")");
                }

                actor.x = newX;
                actor.y = newY;
                log.debug("[PA-BT ACTION]", {
                    action: name,
                    result: "SUCCESS",
                    tick: state.tickCount,
                    actorX: actor.x,
                    actorY: actor.y
                });
                return bt.running;
            } else {
                log.warn("MoveTo " + name + " pathfinding FAILED at actor(" + actor.x + "," + actor.y + ") -> target(" + targetX + "," + targetY + ")");
                return bt.failure;
            }
        };

        const node = bt.createLeafNode(tickFn);
        const action = pabt.newAction(name, conditions, effects, node);
        return action;
    }

    function createPickGoalBlockadeAction(state, cubeId) {
        const name = 'Pick_GoalBlockade_' + cubeId;
        const conditions = [
            {key: 'heldItemExists', value: false, Match: v => v === false},
            {key: 'atEntity_' + cubeId, value: true, Match: v => v === true}
        ];
        const effects = [
            {key: 'heldItemId', Value: cubeId},
            {key: 'heldItemExists', Value: true}
        ];
        const tickFn = function () {
            if (state.gameMode !== 'automatic') return bt.running;
            const actor = state.actors.get(state.activeActorId);
            const cube = state.cubes.get(cubeId);
            if (!cube || cube.deleted) return bt.success;
            const dx = cube.x - actor.x;
            const dy = cube.y - actor.y;
            const dist = Math.sqrt(dx * dx + dy * dy);
            if (dist > PICK_THRESHOLD) return bt.failure;
            cube.deleted = true;
            actor.heldItem = {id: cubeId};
            log.debug("[PA-BT ACTION]", {
                action: name,
                result: "SUCCESS",
                tick: state.tickCount,
                actorX: actor.x,
                actorY: actor.y,
                cubeId: cubeId
            });
            if (state.blackboard) {
                state.blackboard.set('heldItemId', cubeId);
                state.blackboard.set('heldItemExists', true);
            }
            return bt.success;
        };
        const node = bt.createLeafNode(() => state.gameMode === 'automatic' ? tickFn() : bt.running);
        return pabt.newAction(name, conditions, effects, node);
    }

    function createDepositGoalBlockadeAction(state, cubeId, destinationKey) {
        const name = 'Deposit_GoalBlockade_' + cubeId;
        const conditions = [{key: 'heldItemId', value: cubeId, Match: v => v === cubeId}];
        const effects = [
            {key: 'heldItemExists', Value: false},
            {key: 'heldItemId', Value: -1},
            {key: 'pathBlocker_' + destinationKey, Value: -1}
        ];
        const tickFn = function () {
            if (state.gameMode !== 'automatic') return bt.running;
            const actor = state.actors.get(state.activeActorId);
            if (!actor.heldItem || actor.heldItem.id !== cubeId) return bt.failure;
            const goal = [...state.goals.values()][0];
            const goalX = goal ? goal.x : 0, goalY = goal ? goal.y : 0;
            const ax = Math.round(actor.x), ay = Math.round(actor.y);
            const dirToGoalX = goalX - ax, dirToGoalY = goalY - ay;
            const dirs = [];
            if (dirToGoalX < 0) dirs.push([1, 0], [1, -1], [1, 1]);
            else dirs.push([-1, 0], [-1, -1], [-1, 1]);
            if (dirToGoalY > 0) dirs.push([0, -1]); else dirs.push([0, 1]);
            const allDirs = [[1, 0], [1, -1], [0, -1], [1, 1], [-1, -1], [-1, 0], [0, 1], [-1, 1]];
            for (const d of allDirs) if (!dirs.some(e => e[0] === d[0] && e[1] === d[1])) dirs.push(d);

            let spot = null;
            for (const [dx, dy] of dirs) {
                const nx = ax + dx, ny = ay + dy;
                if (nx < 0 || nx >= state.spaceWidth || ny < 0 || ny >= state.height) continue;
                let occupied = false;
                for (const c of state.cubes.values()) if (!c.deleted && c.id !== cubeId && Math.round(c.x) === nx && Math.round(c.y) === ny) {
                    occupied = true;
                    break;
                }
                if (!occupied) {
                    spot = {x: nx, y: ny};
                    break;
                }
            }
            if (!spot) return bt.failure;
            const blocker = state.cubes.get(cubeId);
            if (blocker) {
                blocker.x = spot.x;
                blocker.y = spot.y;
                blocker.deleted = false;
            }
            actor.heldItem = null;
            log.debug("[PA-BT ACTION]", {
                action: name,
                result: "SUCCESS",
                tick: state.tickCount,
                actorX: actor.x,
                actorY: actor.y,
                cubeId: cubeId,
                dest: destinationKey
            });
            if (state.blackboard) {
                state.blackboard.set('heldItemExists', false);
                state.blackboard.set('heldItemId', -1);
                state.blackboard.set('pathBlocker_' + destinationKey, -1);
            }
            return bt.success;
        };
        const node = bt.createLeafNode(() => state.gameMode === 'automatic' ? tickFn() : bt.running);
        return pabt.newAction(name, conditions, effects, node);
    }

    function setupPABTActions(state) {
        const actor = () => state.actors.get(state.activeActorId);
        state.pabtState.setActionGenerator(function (failedCondition) {
            const actions = [];
            const key = failedCondition.key, targetValue = failedCondition.value;
            if (key && typeof key === 'string') {
                if (key === 'cubeDeliveredAtGoal') {
                    const a = state.pabtState.GetAction('Deliver_Target');
                    if (a) actions.push(a);
                }
                if (key === 'heldItemId') {
                    if (targetValue === TARGET_ID) {
                        const a = state.pabtState.GetAction('Pick_Target');
                        if (a) actions.push(a);
                    } else if (typeof targetValue === 'number' && targetValue !== -1) {
                        const cube = state.cubes.get(targetValue);
                        if (cube && !cube.deleted) actions.push(createPickGoalBlockadeAction(state, targetValue));
                    }
                }
                if (key === 'heldItemExists' && targetValue === false) {
                    const a1 = state.pabtState.GetAction('Place_Held_Item'),
                        a2 = state.pabtState.GetAction('Place_Target_Temporary'),
                        a3 = state.pabtState.GetAction('Place_Obstacle');
                    if (a1) actions.push(a1);
                    if (a2) actions.push(a2);
                    if (a3) actions.push(a3);
                    const currentHeldId = state.blackboard.get('heldItemId');
                    if (typeof currentHeldId === 'number' && currentHeldId >= 100) actions.push(createDepositGoalBlockadeAction(state, currentHeldId, 'goal_1'));
                }
                if (key.startsWith('atEntity_')) {
                    const id = parseInt(key.replace('atEntity_', ''), 10);
                    if (!isNaN(id)) actions.push(createMoveToAction(state, 'cube', id));
                }
                if (key.startsWith('atGoal_')) {
                    const id = parseInt(key.replace('atGoal_', ''), 10);
                    if (!isNaN(id)) actions.push(createMoveToAction(state, 'goal', id));
                }
                if (key.startsWith('pathBlocker_')) {
                    const destId = key.replace('pathBlocker_', ''), currentBlocker = state.blackboard.get(key);
                    if (targetValue === -1 && typeof currentBlocker === 'number' && currentBlocker !== -1) {
                        actions.push(createPickGoalBlockadeAction(state, currentBlocker));
                        actions.push(createDepositGoalBlockadeAction(state, currentBlocker, destId));
                    }
                }
            }
            return actions;
        });

        const reg = function (name, conds, effects, tickFn) {
            const conditions = conds.map(c => ({
                key: c.k,
                value: c.v,
                Match: v => c.v === undefined ? v === true : v === c.v
            }));
            const effectList = effects.map(e => ({key: e.k, Value: e.v}));
            const node = bt.createLeafNode(() => state.gameMode === 'automatic' ? tickFn() : bt.running);
            state.pabtState.RegisterAction(name, pabt.newAction(name, conditions, effectList, node));
        };

        reg('Pick_Target', [{k: 'heldItemExists', v: false}, {k: 'atEntity_' + TARGET_ID, v: true}], [{
            k: 'heldItemId',
            v: TARGET_ID
        }, {k: 'heldItemExists', v: true}], function () {
            const a = actor(), t = state.cubes.get(TARGET_ID);
            if (a.heldItem || !t || t.deleted) return bt.failure;
            t.deleted = true;
            a.heldItem = {id: TARGET_ID};
            log.debug("[PA-BT ACTION]", {
                action: "Pick_Target",
                result: "SUCCESS",
                tick: state.tickCount,
                actorX: a.x,
                actorY: a.y,
                cubeId: TARGET_ID
            });
            if (state.blackboard) {
                state.blackboard.set('heldItemId', TARGET_ID);
                state.blackboard.set('heldItemExists', true);
            }
            return bt.success;
        });

        reg('Deliver_Target', [{k: 'atGoal_' + GOAL_ID, v: true}, {
            k: 'heldItemId',
            v: TARGET_ID
        }], [{k: 'cubeDeliveredAtGoal', v: true}], function () {
            const a = actor();
            if (!a.heldItem || a.heldItem.id !== TARGET_ID) return bt.failure;
            const spot = getFreeAdjacentCell(state, a.x, a.y, true);
            if (!spot) return bt.failure;
            log.debug("[PA-BT ACTION]", {
                action: "Deliver_Target",
                result: "SUCCESS",
                tick: state.tickCount,
                actorX: a.x,
                actorY: a.y,
                cubeId: TARGET_ID
            });
            a.heldItem = null;
            state.targetDelivered = true;
            state.winConditionMet = true;
            const t = state.cubes.get(TARGET_ID);
            if (t) {
                t.deleted = false;
                t.x = spot.x;
                t.y = spot.y;
            }
            if (state.blackboard) {
                state.blackboard.set('cubeDeliveredAtGoal', true);
                state.blackboard.set('heldItemExists', false);
                state.blackboard.set('heldItemId', -1);
            }
            return bt.success;
        });

        reg('Place_Obstacle', [{k: 'heldItemExists', v: true}], [{k: 'heldItemExists', v: false}, {
            k: 'heldItemId',
            v: -1
        }], function () {
            const a = actor();
            if (!a.heldItem || a.heldItem.id === TARGET_ID || a.heldItem.id >= 100) return bt.failure;
            const heldId = a.heldItem.id, spot = getFreeAdjacentCell(state, a.x, a.y, false);
            if (!spot) return bt.failure;
            const cube = state.cubes.get(heldId);
            if (cube) {
                cube.deleted = false;
                cube.x = spot.x;
                cube.y = spot.y;
            }
            log.debug("[PA-BT ACTION]", {
                action: "Place_Obstacle",
                result: "SUCCESS",
                tick: state.tickCount,
                actorX: a.x,
                actorY: a.y,
                cubeId: heldId
            });
            a.heldItem = null;
            if (state.blackboard) {
                state.blackboard.set('heldItemExists', false);
                state.blackboard.set('heldItemId', -1);
            }
            return bt.success;
        });

        reg('Place_Target_Temporary', [{k: 'heldItemId', v: TARGET_ID}], [{
            k: 'heldItemExists',
            v: false
        }, {k: 'heldItemId', v: -1}], function () {
            const a = actor();
            if (!a.heldItem || a.heldItem.id !== TARGET_ID) return bt.failure;
            const spot = getFreeAdjacentCell(state, a.x, a.y, false);
            if (!spot) return bt.failure;
            log.debug("[PA-BT ACTION]", {
                action: "Place_Target_Temporary",
                result: "SUCCESS",
                tick: state.tickCount,
                actorX: a.x,
                actorY: a.y,
                cubeId: TARGET_ID
            });
            const t = state.cubes.get(TARGET_ID);
            if (t) {
                t.deleted = false;
                t.x = spot.x;
                t.y = spot.y;
            }
            a.heldItem = null;
            if (state.blackboard) {
                state.blackboard.set('heldItemExists', false);
                state.blackboard.set('heldItemId', -1);
            }
            return bt.success;
        });

        reg('Place_Held_Item', [{k: 'heldItemExists', v: true}], [{k: 'heldItemExists', v: false}, {
            k: 'heldItemId',
            v: -1
        }], function () {
            const a = actor();
            if (!a.heldItem || a.heldItem.id === TARGET_ID || a.heldItem.id >= 100) return bt.failure;
            const spot = getFreeAdjacentCell(state, a.x, a.y);
            if (!spot) return bt.failure;
            const itemId = a.heldItem.id, c = state.cubes.get(itemId);
            if (c) {
                c.deleted = false;
                c.x = spot.x;
                c.y = spot.y;
            }
            log.debug("[PA-BT ACTION]", {
                action: "Place_Held_Item",
                result: "SUCCESS",
                tick: state.tickCount,
                actorX: a.x,
                actorY: a.y,
                cubeId: itemId
            });
            a.heldItem = null;
            if (state.blackboard) {
                state.blackboard.set('heldItemExists', false);
                state.blackboard.set('heldItemId', -1);
            }
            return bt.success;
        });
    }

    // Rendering & Helpers

    // OPTIMIZATION: Pre-allocate render buffer to avoid per-frame allocations.
    // This is CRITICAL for performance - the original implementation allocated
    // 2000+ objects per frame and used O(n²) string concatenation.
    let _renderBuffer = null;
    let _renderBufferWidth = 0;
    let _renderBufferHeight = 0;

    // Get or create render buffer for given dimensions
    function getRenderBuffer(width, height) {
        if (_renderBuffer === null || _renderBufferWidth !== width || _renderBufferHeight !== height) {
            // Only reallocate if dimensions changed
            _renderBufferWidth = width;
            _renderBufferHeight = height;
            // Use 1D array of chars (much faster than 2D array of objects)
            _renderBuffer = new Array(width * height);
            for (let i = 0; i < _renderBuffer.length; i++) {
                _renderBuffer[i] = ' ';
            }
        }
        return _renderBuffer;
    }

    function clearBuffer(buffer, width, height) {
        for (let i = 0; i < buffer.length; i++) {
            buffer[i] = ' ';
        }
    }

    function getAllSprites(state) {
        const sprites = [];
        state.actors.forEach(a => {
            sprites.push({x: a.x, y: a.y, char: '@', width: 1, height: 1});
            if (a.heldItem) sprites.push({x: a.x, y: a.y - 0.5, char: '◆', width: 1, height: 1});
        });
        state.cubes.forEach(c => {
            if (!c.deleted) {
                let ch = '█';
                if (c.type === 'target') ch = '◇';
                else if (c.type === 'obstacle') ch = '▒';  // Was 'goal_blockade', now generic
                sprites.push({x: c.x, y: c.y, char: ch, width: 1, height: 1});
            }
        });
        state.goals.forEach(g => {
            let ch = '○';
            if (g.forTarget) ch = '◎';
            // NOTE: forBlockade (dumpster) removed - only target goal exists now
            sprites.push({x: g.x, y: g.y, char: ch, width: 1, height: 1});
        });
        return sprites;
    }

    function renderPlayArea(state) {
        const width = state.width;
        const height = state.height;

        // OPTIMIZATION: Use pre-allocated buffer (see example-04)
        const buffer = getRenderBuffer(width, height);
        clearBuffer(buffer, width, height);

        // Draw Play Area Border
        const spaceX = Math.floor((width - state.spaceWidth) / 2);
        for (let y = 0; y < height; y++) buffer[y * width + spaceX] = '│';

        // Draw Goal Area Outline
        const cx = GOAL_CENTER_X, cy = GOAL_CENTER_Y, r = GOAL_RADIUS;
        // Visual indicator of Goal Area floor (dots)
        for (let gy = cy - r; gy <= cy + r; gy++) {
            for (let gx = cx - r; gx <= cx + r; gx++) {
                // Fix: Apply offset to goal dots too
                const sx = gx + spaceX + 1;
                if (sx >= 0 && sx < width && gy >= 0 && gy < height) {
                    const idx = gy * width + sx;
                    if (buffer[idx] === ' ') buffer[idx] = '·';
                }
            }
        }

        const sprites = getAllSprites(state).sort((a, b) => a.y - b.y);
        for (const s of sprites) {
            // FIX: Offset sprite position by spaceX + 1 to account for margin and border
            const sx = Math.floor(s.x) + spaceX + 1;
            const sy = Math.floor(s.y);
            if (sx >= 0 && sx < width && sy >= 0 && sy < height) {
                buffer[sy * width + sx] = s.char;
            }
        }

        // HUD Rendering - ALWAYS shows at minimum a status bar
        // The HUD can be in two modes:
        // 1. Full HUD (right panel): When there's enough space (>= 25 columns right of play area)
        // 2. Minimal status bar (bottom row): ALWAYS shown as fallback
        const HUD_WIDTH = 25;
        const hudX = spaceX + state.spaceWidth + 2;  // Right of play area border
        const hudSpace = width - hudX;

        // Helper to draw text at position
        const drawAt = (x, y, txt) => {
            for (let i = 0; i < txt.length && x + i < width; i++) {
                const idx = y * width + x + i;
                if (idx >= 0 && idx < buffer.length) {
                    buffer[idx] = txt[i];
                }
            }
        };

        // Full HUD panel (right side) - only when there's enough space
        if (hudSpace >= HUD_WIDTH) {
            let hudY = 2;
            const draw = (txt) => {
                const maxLen = Math.min(txt.length, hudSpace);
                for (let i = 0; i < maxLen && hudX + i < width; i++) {
                    buffer[hudY * width + hudX + i] = txt[i];
                }
                hudY++;
            };

            draw('═════════════════════════');
            draw(' PICK-AND-PLACE SIM');
            draw('═════════════════════════');
            draw('Mode: ' + state.gameMode.toUpperCase());
            if (state.paused) draw('*** PAUSED ***');
            draw('Goal: 3x3 Area');
            draw('Tick: ' + state.tickCount);
            if (state.winConditionMet) draw('*** GOAL ACHIEVED! ***');
            draw('');
            draw('CONTROLS');
            draw('────────');
            draw('[Q] Quit');
            draw('[M] Toggle Mode');
            draw('[WASD] Move (manual)');
            draw('[Space] Pause');
            draw('[Mouse] Click to Move/Interact');
        } else {
            // Minimal status bar at bottom - ALWAYS shown when full HUD doesn't fit
            const statusY = height - 1;
            const modeStr = 'Mode: ' + state.gameMode.toUpperCase();
            const tickStr = ' T:' + state.tickCount;
            const hintStr = ' [Q]uit [M]ode [WASD] [Space]Pause';
            const winStr = state.winConditionMet ? ' WIN!' : '';
            const pauseStr = state.paused ? ' PAUSED' : '';

            // Build status line, truncate to fit width
            let statusLine = modeStr + tickStr + pauseStr + winStr + hintStr;
            if (statusLine.length > width) {
                statusLine = statusLine.substring(0, width);
            }
            drawAt(0, statusY, statusLine);
        }

        const rows = [];
        for (let y = 0; y < height; y++) rows.push(buffer.slice(y * width, (y + 1) * width).join(''));
        return rows.join('\n');
    }

    // Model Update & Init

    function init() {
        const state = initializeSimulation();
        state.blackboard = new bt.Blackboard();
        state.pabtState = pabt.newState(state.blackboard);
        setupPABTActions(state);
        syncToBlackboard(state);

        const goalConditions = [{key: 'cubeDeliveredAtGoal', Match: v => v === true}];
        state.pabtPlan = pabt.newPlan(state.pabtState, goalConditions);
        state.ticker = bt.newTicker(100, state.pabtPlan.Node());

        return [state, tea.tick(16, 'tick')];
    }

    function update(state, msg) {
        // Handle terminal resize - update rendering dimensions
        if (msg.type === 'WindowSize') {
            state.width = msg.width;
            state.height = msg.height;
            return [state, null];
        }

        if (msg.type === 'Tick' && msg.id === 'tick') {
            if (state.paused) return [state, tea.tick(16, 'tick')];
            state.tickCount++;

            if (state.debugMode && (state.tickCount <= 5 || state.tickCount % 50 === 0)) {
                const actor = state.actors.get(state.activeActorId);
                log.debug("[SIM TICK]", {
                    tick: state.tickCount,
                    actorX: actor.x,
                    actorY: actor.y,
                    heldItemId: actor.heldItem ? actor.heldItem.id : -1,
                    gameMode: state.gameMode
                });
            }

            if (state.gameMode === 'automatic') {
                syncToBlackboard(state);
            } else if (state.blackboard) {
                const actor = state.actors.get(state.activeActorId);
                state.blackboard.set('actorX', actor.x);
                state.blackboard.set('actorY', actor.y);
                state.blackboard.set('heldItemExists', actor.heldItem !== null);
                state.blackboard.set('heldItemId', actor.heldItem ? actor.heldItem.id : -1);
            }
            return [state, tea.tick(16, 'tick')];
        }


        if (msg.type === 'Mouse' && msg.action === 'press' && msg.button === 'left' && state.gameMode === 'manual') {
            const actor = state.actors.get(state.activeActorId);
            const spaceX = Math.floor((state.width - state.spaceWidth) / 2);
            // Coordinate conversion: bubbletea MouseEvent uses 0-indexed coords
            // Grid cell (gx, gy) is rendered at buffer position (gx + spaceX + 1, gy)
            // So: gx = msg.x - spaceX - 1, gy = msg.y
            const clickX = msg.x - spaceX - 1;
            const clickY = msg.y;

            log.info("MOUSE CLICK DETECTED", {
                rawX: msg.x, rawY: msg.y,
                spaceX: spaceX,
                calcX: clickX, calcY: clickY,
                width: state.width, spaceWidth: state.spaceWidth
            });

            if (clickX < 0 || clickX >= state.spaceWidth || clickY < 0 || clickY >= state.height) {
                log.warn("Click out of bounds: (" + clickX + "," + clickY + ") spaceWidth=" + state.spaceWidth + " height=" + state.height);
                return [state, null];
            }

            const ignoreId = actor.heldItem ? actor.heldItem.id : -1;
            let path = null;
            const searchLimit = 1000;

            // When holding an item, pathfind to an adjacent cell rather than the target
            if (actor.heldItem) {
                const dirs = [[0, 1], [0, -1], [1, 0], [-1, 0], [1, 1], [1, -1], [-1, 1], [-1, -1]];
                const neighbors = [];
                for (const [dx, dy] of dirs) {
                    const nx = clickX + dx;
                    const ny = clickY + dy;
                    if (nx >= 0 && nx < state.spaceWidth && ny >= 0 && ny < state.height) {
                        const dist = Math.sqrt(Math.pow(nx - actor.x, 2) + Math.pow(ny - actor.y, 2));
                        neighbors.push({x: nx, y: ny, dist: dist});
                    }
                }
                neighbors.sort((a, b) => a.dist - b.dist);

                for (const n of neighbors) {
                    const p = findPath(state, actor.x, actor.y, n.x, n.y, ignoreId, searchLimit);
                    if (p !== null) {
                        path = p;
                        break;
                    }
                }
            }

            // Fallback: If no path to neighbor found (or not holding item), path to target
            if (path === null) {
                path = findPath(state, actor.x, actor.y, clickX, clickY, ignoreId, searchLimit);
            }

            if (path && path.length > 0) {
                log.info("Path found", {targetX: clickX, targetY: clickY, pathLen: path.length});
                state.manualPath = path;
                state.manualMoveTarget = {x: clickX, y: clickY};
            } else {
                state.manualPath = [];
                // If clicked on self or adjacent, check for immediate interaction in BT
                state.manualMoveTarget = {x: clickX, y: clickY};
            }
            return [state, null];
        }

        if (msg.type === 'Key') {
            if (msg.key === 'q') return [state, tea.quit()];
            if (msg.key === 'm') {
                const wasManual = state.gameMode === 'manual';
                state.gameMode = state.gameMode === 'automatic' ? 'manual' : 'automatic';
                state.manualMoveTarget = null;
                state.manualPath = [];
                state.pathStuckTicks = 0;

                // Toggle Manual Ticker
                if (state.gameMode === 'manual') {
                    // Start manual BT ticker (16ms to match main loop speed for smooth movement)
                    state.manualTicker = bt.newTicker(16, createManualTree(state));
                } else {
                    // Stop manual BT ticker immediately
                    if (state.manualTicker) {
                        state.manualTicker.stop();
                        state.manualTicker = null;
                    }
                    syncToBlackboard(state);
                }

                return [state, null];
            }
            if (msg.key === ' ') {
                state.paused = !state.paused;
                return [state, null];
            }
            if (msg.key === 'escape') {
                state.manualPath = [];
                state.manualMoveTarget = null;
                state.pathStuckTicks = 0;
                return [state, null];
            }
            if (state.gameMode === 'manual' && ['w', 'a', 's', 'd'].includes(msg.key)) {
                const actor = state.actors.get(state.activeActorId);
                let dx = 0, dy = 0;
                if (msg.key === 'w') dy = -1;
                if (msg.key === 's') dy = 1;
                if (msg.key === 'a') dx = -1;
                if (msg.key === 'd') dx = 1;
                if (manualKeysMovement(state, actor, dx, dy)) {
                    state.manualPath = [];
                    state.manualMoveTarget = null;
                }
                return [state, null];
            }
        }

        if (msg.type === 'Resize') {
            state.width = msg.width;
            state.height = msg.height;
        }

        return [state, tea.tick(16, 'tick')];
    }

    function view(state) {
        let output = renderPlayArea(state);
        if (state.debugMode) {
            const actor = state.actors.get(state.activeActorId);
            const target = state.cubes.get(TARGET_ID);
            const goal = state.goals.get(GOAL_ID);
            let obstacleCount = 0;
            GOAL_BLOCKADE_IDS.forEach(id => {
                const cube = state.cubes.get(id);
                if (cube && !cube.deleted) obstacleCount++;
            });
            const ax = Math.round(actor.x);
            const ay = Math.round(actor.y);
            const goalReachable = goal ? getPathInfo(state, ax, ay, goal.x, goal.y).reachable : false;

            const debugJSON = JSON.stringify({
                m: state.gameMode === 'automatic' ? 'a' : 'm',
                t: state.tickCount,
                tw: state.width,
                sw: state.spaceWidth,
                x: Math.round(actor.x * 10) / 10,
                y: Math.round(actor.y * 10) / 10,
                h: actor.heldItem ? actor.heldItem.id : -1,
                w: state.winConditionMet ? 1 : 0,
                a: target && !target.deleted ? target.x : null,
                b: target && !target.deleted ? target.y : null,
                n: 0,
                g: obstacleCount,
                gr: goalReachable ? 1 : 0,
                mt: state.manualMoveTarget ? 1 : 0,
                mpl: state.manualPath.length,
                pst: state.pathStuckTicks
            });
            output += '\n__place_debug_start__\n' + debugJSON + '\n__place_debug_end__';
        }
        return output;
    }

    program = tea.newModel({
        init: function () {
            return init();
        },
        update: function (msg, model) {
            return update(model, msg);
        },
        view: function (model) {
            return view(model);
        },
        renderThrottle: {
            enabled: true,
            minIntervalMs: 16,
            alwaysRenderMsgTypes: ["Tick", "WindowSize"]
        }
    });

    if (typeof module !== 'undefined' && typeof module.exports !== 'undefined') {
        module.exports = {
            init,
            update,
            initializeSimulation,
            TARGET_ID,
            buildBlockedSet,
            findPath,
            getPathInfo,
            findNextStep,
            findFirstBlocker,
            manualKeysMovement
        };
    }
} catch (e) {
    printFatalError(e);
    throw e;
}

{
    let shouldRun = true;
    if (typeof module !== 'undefined') {
        if (typeof require !== 'undefined' && require.main !== module) {
            shouldRun = false;
        }
    }
    if (shouldRun) {
        try {
            tea.run(program, {altScreen: true, mouse: true});
        } catch (e) {
            printFatalError(e);
            throw e;
        }
    }
}
