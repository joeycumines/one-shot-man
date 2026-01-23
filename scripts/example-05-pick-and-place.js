#!/usr/bin/env osm script

/**
 * @fileoverview example-05-pick-and-place.js
 * @description
 * Pick-and-Place Simulator demonstrating osm:pabt PA-BT planning integration.
 *
 * ARCHITECTURAL WARNING: "TRUTHFUL EFFECTS" REQUIRED
 *
 * This implementation relies on a Reactive Planning Loop (Plan -> Execute -> Verify).
 * To prevent Livelocks (Infinite Replanning) and Deadlocks (Bridge Freezes),
 * you must adhere to the following strict architectural constraints derived from
 * "Behavior Trees in Robotics and AI" (Colledanchise/Ögren).
 *
 * 1. NO HEURISTIC EFFECTS (The "Lying Action" Anti-Pattern)
 * Do not assert that an atomic action satisfies a high-level aggregate condition
 * unless it physically guarantees it in a single tick.
 * - BAD: Action `Pick_Obstacle` has Effect `isPathClear = true`.
 * (Reasoning: If 8 obstacles exist, picking one does NOT make the path clear.
 * The Planner will loop infinitely: Plan Pick -> Execute -> Verify (Path still blocked) -> Replan Pick.)
 * - GOOD: Action `Pick_Obstacle_A` has Effect `isCleared(Obstacle_A) = true`.
 * (Reasoning: The planner generates a sequence to clear A, then B, then C, until `isPathClear` is naturally satisfied).
 *
 * 2. STATE GRANULARITY (The "Reward Sparsity" Problem)
 * PA-BT requires granular feedback to measure progress. Do not use binary flags
 * for multi-step states.
 * - The Blackboard must reflect incremental progress. If a wall is composed of
 * multiple entities, the Precondition must be `!Overlaps(Entity_ID)`, not `!Blocked`.
 *
 * 3. DYNAMIC ACTION GENERATION COMPLETENESS
 * When decomposing high-level conditions (e.g., `reachable`), the `ActionGenerator`
 * must be capable of expanding ALL dependency chains.
 * - If `Pick` requires `Hands_Empty`, and `Hands_Empty` fails, the generator
 * MUST produce a valid `Place/Drop` action.
 * - Failure to generate a valid bridging action for a deep dependency will cause
 * the Go-side planner to enter an infinite expansion loop, saturating the
 * `RunOnLoopSync` bridge and freezing the JavaScript simulation tick.
 *
 * **WARNING RE: LOGGING:** This is an _interactive_ terminal application.
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
    const PICK_THRESHOLD = 1.8; // Distance threshold for picking up cubes

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

        return {
            width: ENV_WIDTH,
            height: ENV_HEIGHT,
            spaceWidth: 60,

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
            manualKeys: new Map(), // Track key press/release state for custom key-hold support
            manualKeyLastSeen: new Map(), // Track last time each key was pressed for release detection

            winConditionMet: false,
            targetDelivered: false,

            renderBuffer: null,
            renderBufferWidth: 0,
            renderBufferHeight: 0,

            debugMode: os.getenv('OSM_TEST_MODE') === '1'
        };
    }

    // Logic Helpers

    // manualKeysMovement: Handle WASD movement in manual mode based on pressed keys
    // Returns true if movement occurred, false otherwise
    function manualKeysMovement(state, actor) {
        if (!state.manualKeys || state.manualKeys.size === 0) return false;

        const getDir = () => {
            let dx = 0, dy = 0;
            if (state.manualKeys.get('w')) dy = -1;
            if (state.manualKeys.get('s')) dy = 1;
            if (state.manualKeys.get('a')) dx = -1;
            if (state.manualKeys.get('d')) dx = 1;

            // Diagonal handling: allow diagonal movement if both axes pressed
            return {dx, dy};
        };

        const {dx, dy} = getDir();

        if (dx === 0 && dy === 0) return false;

        const nx = actor.x + dx;
        const ny = actor.y + dy;
        let blocked = false;

        // Boundary check first
        if (Math.round(nx) < 0 || Math.round(nx) >= state.spaceWidth ||
            Math.round(ny) < 0 || Math.round(ny) >= state.height) {
            blocked = true;
        }

        // Fast collision check (only if not already blocked by boundary)
        if (!blocked) {
            for (const c of state.cubes.values()) {
                if (!c.deleted && Math.round(c.x) === Math.round(nx) && Math.round(c.y) === Math.round(ny)) {
                    if (actor.heldItem && c.id === actor.heldItem.id) continue;
                    blocked = true;
                    break;
                }
            }
        }

        if (!blocked) {
            actor.x = nx;
            actor.y = ny;
        }
        return !blocked; // Return true if movement occurred successfully
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
    function findPath(state, startX, startY, targetX, targetY, ignoreCubeId) {
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

        while (queue.length > 0) {
            const cur = queue.shift();

            if (cur.x === iTargetX && cur.y === iTargetY) {
                found = true;
                finalNode = cur;
                break;
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

        if (!found) return null;

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
            log.debug("findFirstBlocker to goal: cubeAtPosition=" + positions.join(","));
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
                    log.debug("findFirstBlocker: reached adjacency at (" + current.x + "," + current.y + ") to goal");
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
            log.info("BT TICK: " + name + " executing at tick " + state.tickCount);
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
                    log.debug("MoveTo " + name + " target deleted, returning success");
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
                log.debug("MoveTo " + name + " reached target, dist=" + dist.toFixed(2) + ", threshold=" + threshold);
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
                log.debug("MoveTo " + name + " step: (" + oldX + "," + oldY + ") -> (" + actor.x + "," + actor.y + ") via nextStep(" + nextStep.x + "," + nextStep.y + ")");
                return bt.running;
            } else {
                log.warn("MoveTo " + name + " pathfinding FAILED at actor(" + actor.x + "," + actor.y + ") -> target(" + targetX + "," + targetY + ")");
                return bt.failure;
            }
        };

        const node = bt.createLeafNode(tickFn);
        const action = pabt.newAction(name, conditions, effects, node);
        // CRITICAL FIX: Removed actionCache.set() - always create fresh instances
        return action;
    }

    // createPickGoalBlockadeAction: Pick action for ring blockers (IDs >= 100)
    // Named "Pick_GoalBlockade_X" to match test harness expectations
    // Generated on-demand by ActionGenerator when planner needs heldItemId=X
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
            log.info("BT TICK: " + name + " executing at tick " + state.tickCount);
            if (state.gameMode !== 'automatic') return bt.running;

            const actor = state.actors.get(state.activeActorId);
            const cube = state.cubes.get(cubeId);

            if (!cube || cube.deleted) {
                log.debug(name + " target already deleted, success");
                return bt.success;
            }

            // Check if we're close enough
            const dx = cube.x - actor.x;
            const dy = cube.y - actor.y;
            const dist = Math.sqrt(dx * dx + dy * dy);

            if (dist > PICK_THRESHOLD) {
                log.debug(name + " too far from cube", {dist: dist, threshold: PICK_THRESHOLD});
                return bt.failure;
            }

            // Pick it up
            cube.deleted = true;
            actor.heldItem = {id: cubeId};

            log.info("PA-BT action executing", {
                action: name,
                result: "SUCCESS",
                tick: state.tickCount,
                cubeId: cubeId,
                actorX: actor.x,
                actorY: actor.y
            });

            if (state.blackboard) {
                state.blackboard.set('heldItemId', cubeId);
                state.blackboard.set('heldItemExists', true);
            }
            return bt.success;
        };

        const node = bt.createLeafNode(function () {
            return state.gameMode === 'automatic' ? tickFn() : bt.running;
        });

        const action = pabt.newAction(name, conditions, effects, node);
        log.debug("Created dynamic action: " + name);
        return action;
    }

    // createDepositGoalBlockadeAction: Place a ring blocker to clear the path
    // Named "Deposit_GoalBlockade_X" to match test harness expectations
    // Effect: pathBlocker_goal_1 = -1 (path is now clear)
    function createDepositGoalBlockadeAction(state, cubeId, destinationKey) {
        const name = 'Deposit_GoalBlockade_' + cubeId;

        // Precondition: Must be holding THIS specific blocker
        const conditions = [
            {key: 'heldItemId', value: cubeId, Match: v => v === cubeId}
        ];

        // Effect: Path is cleared
        const effects = [
            {key: 'heldItemExists', Value: false},
            {key: 'heldItemId', Value: -1},
            {key: 'pathBlocker_' + destinationKey, Value: -1}
        ];

        const tickFn = function () {
            log.info("BT TICK: " + name + " executing at tick " + state.tickCount);
            if (state.gameMode !== 'automatic') return bt.running;

            const actor = state.actors.get(state.activeActorId);

            // Must be holding something
            if (!actor.heldItem || actor.heldItem.id !== cubeId) {
                log.debug(name + " not holding the correct blocker");
                return bt.failure;
            }

            // Find a valid placement spot that won't block the path
            const goal = [...state.goals.values()][0];
            const goalX = goal ? goal.x : 0;
            const goalY = goal ? goal.y : 0;
            const ax = Math.round(actor.x);
            const ay = Math.round(actor.y);

            // Prioritize directions AWAY from goal
            const dirToGoalX = goalX - ax;
            const dirToGoalY = goalY - ay;
            const dirs = [];

            if (dirToGoalX < 0) {
                dirs.push([1, 0], [1, -1], [1, 1]);
            } else {
                dirs.push([-1, 0], [-1, -1], [-1, 1]);
            }
            if (dirToGoalY > 0) {
                dirs.push([0, -1]);
            } else {
                dirs.push([0, 1]);
            }
            // Add fallbacks
            const allDirs = [[1, 0], [1, -1], [0, -1], [1, 1], [-1, -1], [-1, 0], [0, 1], [-1, 1]];
            for (const d of allDirs) {
                if (!dirs.some(e => e[0] === d[0] && e[1] === d[1])) {
                    dirs.push(d);
                }
            }

            let placementSpot = null;
            const blocker = state.cubes.get(cubeId);

            for (const [dx, dy] of dirs) {
                const nx = ax + dx;
                const ny = ay + dy;

                // Bounds check
                if (nx < 0 || nx >= state.spaceWidth || ny < 0 || ny >= state.height) continue;

                // Check if occupied
                let occupied = false;
                for (const c of state.cubes.values()) {
                    if (!c.deleted && c.id !== cubeId && Math.round(c.x) === nx && Math.round(c.y) === ny) {
                        occupied = true;
                        break;
                    }
                }
                if (occupied) continue;

                // Valid spot found
                placementSpot = {x: nx, y: ny};
                break;
            }

            if (!placementSpot) {
                log.debug(name + " no valid placement spot found");
                return bt.failure;
            }

            // Place the blocker
            if (blocker) {
                blocker.x = placementSpot.x;
                blocker.y = placementSpot.y;
                blocker.deleted = false;
            }

            actor.heldItem = null;
            log.debug("PA-BT: HeldItem cleared (blocker placed)", {tick: state.tickCount});

            log.info("PA-BT action executing", {
                action: name,
                result: "SUCCESS",
                tick: state.tickCount,
                blockerId: cubeId,
                placedAt: placementSpot
            });

            if (state.blackboard) {
                state.blackboard.set('heldItemExists', false);
                state.blackboard.set('heldItemId', -1);
                state.blackboard.set('pathBlocker_' + destinationKey, -1);
            }

            return bt.success;
        };

        const node = bt.createLeafNode(function () {
            return state.gameMode === 'automatic' ? tickFn() : bt.running;
        });

        const action = pabt.newAction(name, conditions, effects, node);
        log.debug("Created dynamic action: " + name);
        return action;
    }

    function setupPABTActions(state) {
        const actor = () => state.actors.get(state.activeActorId);

        // ACTION GENERATOR - MUST return actions for ALL conditions the planner queries!
        //
        // PA-BT does NOT auto-discover registered actions. The actionGenerator
        // MUST explicitly return actions that can satisfy the failed condition.
        //
        // Key insight from graphjsimpl_test.go:
        // - failedCondition.key = the condition key being checked
        // - failedCondition.value = the TARGET value the planner needs to achieve
        state.pabtState.setActionGenerator(function (failedCondition) {
            const actions = [];
            const key = failedCondition.key;
            const targetValue = failedCondition.value;

            // Log EVERY call to actionGenerator to track planner activity
            log.info("ACTION_GENERATOR called", {
                failedKey: key,
                failedValue: targetValue,
                failedKeyType: typeof key,
                tick: state.tickCount
            });

            if (key && typeof key === 'string') {
                // cubeDeliveredAtGoal: The GOAL condition!
                // To achieve cubeDeliveredAtGoal=true, we need Deliver_Target
                // But Deliver_Target requires heldItemId=TARGET_ID and atGoal_1=true
                if (key === 'cubeDeliveredAtGoal') {
                    // Return Deliver_Target action (must be created or retrieved)
                    // The registered action should be returned here
                    log.debug("ACTION_GENERATOR: returning Deliver_Target for cubeDeliveredAtGoal");
                    const deliverAction = state.pabtState.GetAction('Deliver_Target');
                    if (deliverAction) {
                        actions.push(deliverAction);
                    }
                }

                // heldItemId: Planner needs a specific item to be held
                // Dynamically create Pick_Obstacle_X for any cube (no hardcoded IDs)
                if (key === 'heldItemId') {
                    const itemId = targetValue;
                    if (itemId === TARGET_ID) {
                        log.debug("ACTION_GENERATOR: returning Pick_Target for heldItemId=" + itemId);
                        const pickAction = state.pabtState.GetAction('Pick_Target');
                        if (pickAction) {
                            actions.push(pickAction);
                        }
                    } else if (typeof itemId === 'number' && itemId !== -1 && itemId !== TARGET_ID) {
                        // Dynamic goal blockade picking - create action on-the-fly for ANY blocker
                        // Uses Pick_GoalBlockade_X naming to match test harness expectations
                        const cube = state.cubes.get(itemId);
                        if (cube && !cube.deleted) {
                            log.debug("ACTION_GENERATOR: creating Pick_GoalBlockade_" + itemId);
                            const pickGoalBlockade = createPickGoalBlockadeAction(state, itemId);
                            if (pickGoalBlockade) actions.push(pickGoalBlockade);
                        }
                    }
                }

                // heldItemExists: Planner needs hands to be free (false) or holding (true)
                // Return Place_Held_Item, Place_Target_Temporary, Place_Obstacle,
                // AND Deposit_GoalBlockade_X if holding a ring blocker!
                if (key === 'heldItemExists') {
                    if (targetValue === false) {
                        log.debug("ACTION_GENERATOR: returning Place actions for heldItemExists=false");
                        // Return place actions - planner will choose based on preconditions
                        const placeHeldItem = state.pabtState.GetAction('Place_Held_Item');
                        const placeTargetTemp = state.pabtState.GetAction('Place_Target_Temporary');
                        const placeObstacle = state.pabtState.GetAction('Place_Obstacle');
                        if (placeHeldItem) actions.push(placeHeldItem);
                        if (placeTargetTemp) actions.push(placeTargetTemp);
                        if (placeObstacle) actions.push(placeObstacle);

                        // CRITICAL FIX: If holding a ring blocker (ID >= 100),
                        // also return Deposit_GoalBlockade_X which properly places
                        // the blocker AWAY from the goal path!
                        const currentHeldId = state.blackboard.get('heldItemId');
                        if (typeof currentHeldId === 'number' && currentHeldId >= 100) {
                            log.info("ACTION_GENERATOR: holding ring blocker, adding Deposit action", {
                                heldId: currentHeldId
                            });
                            // Find which goal this blocker is blocking
                            const goalId = 1; // Assuming single goal
                            const depositAction = createDepositGoalBlockadeAction(state, currentHeldId, 'goal_' + goalId);
                            if (depositAction) actions.push(depositAction);
                        }
                    }
                }

                // atEntity_X: Planner needs actor at entity location
                // Return MoveTo_cube_X
                if (key.startsWith('atEntity_')) {
                    const entityId = parseInt(key.replace('atEntity_', ''), 10);
                    if (!isNaN(entityId)) {
                        log.info("ACTION_GENERATOR: creating MoveTo for entity", {
                            entityId: entityId,
                            tick: state.tickCount
                        });
                        // NOTE: No pathBlocker constraint for moving to entities!
                        // The agent should go to the target first, then clear the goal path.
                        actions.push(createMoveToAction(state, 'cube', entityId));
                    }
                }

                // atGoal_X: Planner needs actor at goal location
                // Return MoveTo_goal_X
                if (key.startsWith('atGoal_')) {
                    const goalId = parseInt(key.replace('atGoal_', ''), 10);
                    if (!isNaN(goalId)) {
                        log.debug("ACTION_GENERATOR: creating MoveTo for goal", {goalId: goalId});
                        actions.push(createMoveToAction(state, 'goal', goalId));
                    }
                }

                // pathBlocker_X: Dynamic obstacle detection
                // When MoveTo fails due to blocked path, syncToBlackboard sets pathBlocker_X = cubeId
                // ActionGenerator creates decomposed Pick + Deposit actions per PA-BT principles
                //
                // CRITICAL FIX (2026-01-23): Must return BOTH Pick_GoalBlockade AND Deposit_GoalBlockade
                // The planner needs Pick to satisfy Deposit's precondition (heldItemId=X).
                // Without Pick in the returned actions, planner cannot chain them!
                if (key.startsWith('pathBlocker_')) {
                    const destId = key.replace('pathBlocker_', '');
                    // Get current blocker from blackboard
                    const currentBlocker = state.blackboard.get(key);
                    log.info("ACTION_GENERATOR: pathBlocker handler", {
                        key: key,
                        targetValue: targetValue,
                        currentBlocker: currentBlocker,
                        destination: destId
                    });

                    // When value == -1 is desired (path clear), we need to clear the blocker
                    // PA-BT DECOMPOSITION: Return BOTH Pick_GoalBlockade_X AND Deposit_GoalBlockade_X
                    // Pick achieves heldItemId=X, Deposit achieves pathBlocker=-1
                    // Planner will chain: MoveTo(blocker) -> Pick -> Deposit
                    if (targetValue === -1) {
                        if (typeof currentBlocker === 'number' && currentBlocker !== -1) {
                            const cube = state.cubes.get(currentBlocker);
                            // NOTE: The cube might be picked up (deleted=true) but we still need
                            // to generate actions. The cube exists in state.cubes even when held.
                            if (cube) {
                                log.debug("ACTION_GENERATOR: creating Pick + Deposit for pathBlocker", {
                                    destination: destId,
                                    blockerId: currentBlocker,
                                    cubeDeleted: cube.deleted
                                });

                                // CRITICAL: Return Pick_GoalBlockade FIRST so planner can chain
                                // Pick -> Deposit sequence for clearing the path
                                const pickAction = createPickGoalBlockadeAction(state, currentBlocker);
                                if (pickAction) actions.push(pickAction);

                                // Then Deposit which achieves pathBlocker_X = -1
                                const depositAction = createDepositGoalBlockadeAction(state, currentBlocker, destId);
                                if (depositAction) actions.push(depositAction);
                            }
                        } else {
                            log.debug("ACTION_GENERATOR: pathBlocker already clear or invalid", {currentBlocker: currentBlocker});
                        }
                    }
                }
            }

            log.debug("ACTION_GENERATOR returning", {actionCount: actions.length, failedKey: key});
            return actions;
        });

        const reg = function (name, conds, effects, tickFn) {
            // CRITICAL: Include 'value' field so ActionGenerator can read failedCondition.value
            const conditions = conds.map(c => ({
                key: c.k,
                value: c.v,  // ActionGenerator needs this to know the target value
                Match: v => c.v === undefined ? v === true : v === c.v
            }));
            const effectList = effects.map(e => ({key: e.k, Value: e.v}));
            const node = bt.createLeafNode(() => state.gameMode === 'automatic' ? tickFn() : bt.running);
            state.pabtState.RegisterAction(name, pabt.newAction(name, conditions, effectList, node));
        };

        // Pick_Target
        // Pick_Target: Pick up the target cube when at its location
        //
        // CONFLICT RESOLUTION PATTERN (per review.md 1.3):
        // Agent picks target first, then discovers path to goal is blocked.
        // Planner must then:
        //    1. Place_Target_Temporary (free hands)
        //    2. Pick_GoalBlockade + Deposit_GoalBlockade (clear path)
        //    3. Pick_Target again (retrieve)
        //    4. Deliver_Target
        //
        // NOTE: We do NOT require pathBlocker_goal_1=-1 as precondition!
        // This allows the agent to pick up target first, then handle blocking.
        const pickTargetConditions = [];
        pickTargetConditions.push({k: 'heldItemExists', v: false});
        pickTargetConditions.push({k: 'atEntity_' + TARGET_ID, v: true});
        reg('Pick_Target',
            pickTargetConditions,
            [{k: 'heldItemId', v: TARGET_ID}, {k: 'heldItemExists', v: true}],
            function () {
                const a = actor();
                // Runtime guard: Cannot pick if already holding something
                if (a.heldItem) {
                    log.warn("Pick_Target BLOCKED - already holding item", {
                        heldItemId: a.heldItem.id,
                        tick: state.tickCount
                    });
                    return bt.failure;
                }
                const t = state.cubes.get(TARGET_ID);
                if (!t || t.deleted) return bt.failure;

                t.deleted = true;
                a.heldItem = {id: TARGET_ID};

                log.info("PA-BT action executing", {
                    action: "Pick_Target",
                    result: "SUCCESS",
                    tick: state.tickCount,
                    actorX: a.x,
                    actorY: a.y
                });

                if (state.blackboard) {
                    state.blackboard.set('heldItemId', TARGET_ID);
                    state.blackboard.set('heldItemExists', true);
                }
                return bt.success;
            }
        );

        // Deliver_Target: Place target INTO Goal Area
        reg('Deliver_Target',
            [{k: 'atGoal_' + GOAL_ID, v: true}, {k: 'heldItemId', v: TARGET_ID}],
            [{k: 'cubeDeliveredAtGoal', v: true}],
            function () {
                const a = actor();
                if (!a.heldItem || a.heldItem.id !== TARGET_ID) return bt.failure;

                // Generic Place Logic targeted at Goal Area
                const spot = getFreeAdjacentCell(state, a.x, a.y, true); // true = targetGoalArea
                if (!spot) return bt.failure; // Not close enough to valid goal cell

                log.info("PA-BT action executing", {
                    action: "Deliver_Target",
                    result: "SUCCESS",
                    tick: state.tickCount,
                    actorX: a.x,
                    actorY: a.y,
                    deliveredAt: {x: spot.x, y: spot.y}
                });

                a.heldItem = null;
                state.targetDelivered = true;
                state.winConditionMet = true;

                // Visual placement
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
            }
        );

        // Place_Obstacle: Generic action to place any held obstacle
        // Places the obstacle at any free adjacent cell
        // This is the DYNAMIC replacement for hardcoded blockade actions
        reg('Place_Obstacle',
            [{k: 'heldItemExists', v: true}],
            [{k: 'heldItemExists', v: false}, {k: 'heldItemId', v: -1}],
            function () {
                const a = actor();
                if (!a.heldItem) return bt.failure;

                const heldId = a.heldItem.id;

                // Don't place the TARGET using this action
                if (heldId === TARGET_ID) {
                    return bt.failure;
                }

                // CRITICAL: Don't place ring blockers (ID >= 100) using this action!
                // Use Deposit_GoalBlockade_X instead, which places blockers AWAY from goal path.
                if (heldId >= 100) {
                    log.debug("Place_Obstacle: refusing ring blocker " + heldId + ", use Deposit_GoalBlockade instead");
                    return bt.failure;
                }

                // Find a free spot nearby (NOT in goal area)
                const spot = getFreeAdjacentCell(state, a.x, a.y, false);
                if (!spot) {
                    log.debug("Place_Obstacle: no free adjacent cell found");
                    return bt.failure;
                }

                // Re-instantiate the cube at the new location
                const cube = state.cubes.get(heldId);
                if (cube) {
                    cube.deleted = false;
                    cube.x = spot.x;
                    cube.y = spot.y;
                }

                a.heldItem = null;

                log.info("PA-BT action executing", {
                    action: "Place_Obstacle",
                    result: "SUCCESS",
                    tick: state.tickCount,
                    obstacleId: heldId,
                    placedAt: {x: spot.x, y: spot.y},
                    actorX: a.x,
                    actorY: a.y
                });

                if (state.blackboard) {
                    state.blackboard.set('heldItemExists', false);
                    state.blackboard.set('heldItemId', -1);
                }
                return bt.success;
            }
        );

        // Place_Target_Temporary: Conflict resolution action
        // When holding target but goal is blocked, place target temporarily
        // so hands are free to clear blockades.
        reg('Place_Target_Temporary',
            [{k: 'heldItemId', v: TARGET_ID}],
            [{k: 'heldItemExists', v: false}, {k: 'heldItemId', v: -1}],
            function () {
                const a = actor();
                if (!a.heldItem || a.heldItem.id !== TARGET_ID) return bt.failure;

                const spot = getFreeAdjacentCell(state, a.x, a.y, false); // false = don't target goal area
                if (!spot) return bt.failure;

                log.info("PA-BT action executing", {
                    action: "Place_Target_Temporary",
                    result: "SUCCESS",
                    tick: state.tickCount,
                    actorX: a.x,
                    actorY: a.y,
                    placedAt: {x: spot.x, y: spot.y}
                });

                // Re-instantiate target in the world at temporary location
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
            }
        );

        // Place_Held_Item (Generic Drop to free hands)
        // Satisfies the removal of Staging Area logic.
        // REFACTORED: No heldItemIsBlockade check - just place any held item
        // Place_Obstacle is preferred for obstacles; this is a fallback for any item
        // NOTE: This action EXCLUDES holding TARGET - use Place_Target_Temporary for that!
        // The runtime tick function checks for TARGET_ID and returns failure if holding target.
        // We CANNOT use heldItemId in preconditions because the planner interprets the 'value'
        // field as "this is the value I need to achieve", which causes infinite loops.
        reg('Place_Held_Item',
            [
                {k: 'heldItemExists', v: true}
                // NOTE: Removed heldItemId precondition - runtime checks for TARGET instead
            ],
            [{k: 'heldItemExists', v: false}, {k: 'heldItemId', v: -1}],
            function () {
                const a = actor();
                if (!a.heldItem) return bt.success;

                // Runtime guard: Don't place TARGET with this action
                if (a.heldItem.id === TARGET_ID) {
                    return bt.failure;
                }

                // Runtime guard: Don't place ring blockers (ID >= 100) with this action
                // Use Deposit_GoalBlockade_X instead, which places blockers AWAY from goal path.
                if (a.heldItem.id >= 100) {
                    log.debug("Place_Held_Item: refusing ring blocker " + a.heldItem.id + ", use Deposit_GoalBlockade instead");
                    return bt.failure;
                }

                const spot = getFreeAdjacentCell(state, a.x, a.y);
                if (!spot) return bt.failure; // No space

                log.info("PA-BT action executing", {
                    action: "Place_Held_Item",
                    result: "SUCCESS",
                    tick: state.tickCount,
                    id: a.heldItem.id,
                    actorX: a.x,
                    actorY: a.y,
                    placedAt: {x: spot.x, y: spot.y}
                });

                // Re-instantiate the item in the world
                const itemId = a.heldItem.id;
                const c = state.cubes.get(itemId);
                if (c) {
                    c.deleted = false;
                    c.x = spot.x;
                    c.y = spot.y;
                }

                a.heldItem = null;
                if (state.blackboard) {
                    state.blackboard.set('heldItemExists', false);
                    state.blackboard.set('heldItemId', -1);
                }
                return bt.success;
            }
        );
    }

    // Rendering & Helpers

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
        const buffer = new Array(width * height || 0).fill(' ');

        // Draw Play Area Border
        const spaceX = Math.floor((width - state.spaceWidth) / 2);
        for (let y = 0; y < height; y++) buffer[y * width + spaceX] = '│';

        // Draw Goal Area Outline
        const cx = GOAL_CENTER_X, cy = GOAL_CENTER_Y, r = GOAL_RADIUS;
        // Visual indicator of Goal Area floor (dots)
        for (let gy = cy - r; gy <= cy + r; gy++) {
            for (let gx = cx - r; gx <= cx + r; gx++) {
                if (gx >= 0 && gx < width && gy >= 0 && gy < height) {
                    const idx = gy * width + gx;
                    if (buffer[idx] === ' ') buffer[idx] = '·';
                }
            }
        }

        const sprites = getAllSprites(state).sort((a, b) => a.y - b.y);
        for (const s of sprites) {
            const sx = Math.floor(s.x);
            const sy = Math.floor(s.y);
            if (sx >= 0 && sx < width && sy >= 0 && sy < height) {
                buffer[sy * width + sx] = s.char;
            }
        }

        // HUD
        let hudY = 2;
        const hudX = state.spaceWidth + 2;
        const draw = (txt) => {
            for (let i = 0; i < txt.length && hudX + i < width; i++) buffer[hudY * width + hudX + i] = txt[i];
            hudY++;
        };

        draw('═════════════════════════');
        draw(' PICK-AND-PLACE SIM');
        draw('═════════════════════════');
        draw('Mode: ' + state.gameMode.toUpperCase());
        if (state.paused) draw('*** PAUSED ***');
        draw('Goal: 3x3 Area');
        draw('Tick: ' + state.tickCount);  // Force bubbletea renderer to see change
        if (state.winConditionMet) draw('*** GOAL ACHIEVED! ***');
        draw('');
        draw('CONTROLS');
        draw('────────');
        draw('[Q] Quit');
        draw('[M] Toggle Mode');
        draw('[WASD] Move (manual)');
        draw('[Space] Pause');
        draw('[Mouse] Click to Move/Interact');

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

        log.info("Pick-and-Place simulation initialized", {
            actorX: state.actors.get(state.activeActorId).x,
            actorY: state.actors.get(state.activeActorId).y,
            obstacleCount: GOAL_BLOCKADE_IDS.length,
            targetId: TARGET_ID,
            mode: state.gameMode
        });

        const goalConditions = [{key: 'cubeDeliveredAtGoal', Match: v => v === true}];
        state.pabtPlan = pabt.newPlan(state.pabtState, goalConditions);
        state.ticker = bt.newTicker(100, state.pabtPlan.Node());

        return [state, tea.tick(16, 'tick')];
    }

    function update(state, msg) {
        // SIMULATION TICK
        if (msg.type === 'Tick' && msg.id === 'tick') {
            if (state.paused) {
                return [state, tea.tick(16, 'tick')];
            }

            state.tickCount++;

            // Handle Manual Click-to-Move Path Traversal
            // We traverse the pre-calculated path instead of BFS-ing every tick.
            if (state.gameMode === 'manual' && state.manualPath.length > 0) {
                const actor = state.actors.get(state.activeActorId);
                const nextPoint = state.manualPath[0];

                const dx = nextPoint.x - actor.x;
                const dy = nextPoint.y - actor.y;
                const dist = Math.sqrt(dx * dx + dy * dy);

                // Collision check for next position
                if (dist >= 0.1) {
                    const nextX = actor.x + Math.sign(dx) * Math.min(1.0, Math.abs(dx));
                    const nextY = actor.y + Math.sign(dy) * Math.min(1.0, Math.abs(dy));
                    let nextBlocked = false;

                    for (const c of state.cubes.values()) {
                        if (!c.deleted && Math.round(c.x) === Math.round(nextX) && Math.round(c.y) === Math.round(nextY)) {
                            if (actor.heldItem && c.id === actor.heldItem.id) continue;
                            nextBlocked = true;
                            log.warn("Path collision detected during manual movement", {
                                x: nextX,
                                y: nextY,
                                tick: state.tickCount
                            });
                            break;
                        }
                    }

                    if (nextBlocked) {
                        // Path is now blocked - abort movement
                        state.manualPath = [];
                        state.manualMoveTarget = null;
                        state.pathStuckTicks = 0;
                    }
                }

                if (state.manualPath.length > 0) {
                    if (dist < 0.1) {
                        // Reached waypoint
                        actor.x = nextPoint.x;
                        actor.y = nextPoint.y;
                        state.manualPath.shift(); // Remove reached point
                        state.pathStuckTicks = 0; // Reset stuck counter on progress
                    } else {
                        const oldDist = dist;
                        // Move towards waypoint (Speed = 1.0, consistent with findNextStep)
                        actor.x += Math.sign(dx) * Math.min(1.0, Math.abs(dx));
                        actor.y += Math.sign(dy) * Math.min(1.0, Math.abs(dy));

                        // Stuck detection: if distance didn't decrease, increment counter
                        const newDist = Math.sqrt(Math.pow(nextPoint.x - actor.x, 2) + Math.pow(nextPoint.y - actor.y, 2));
                        if (newDist >= oldDist - 0.01) {
                            state.pathStuckTicks++;
                        } else {
                            state.pathStuckTicks = 0; // Reset on progress
                        }

                        // Abort if stuck for ~1 second (62 ticks at 16ms)
                        if (state.pathStuckTicks > 60) {
                            log.warn("Path traversal stuck - aborting movement", {tick: state.tickCount});
                            state.manualPath = [];
                            state.manualMoveTarget = null;
                            state.pathStuckTicks = 0;
                        }
                    }
                }
            } else if (state.gameMode === 'manual' && state.manualMoveTarget) {
                // Fallback cleanup if path is empty but target remains (shouldn't happen with new logic)
                state.manualMoveTarget = null;
            }

            // Handle Manual WASD Movement in Tick handler for smooth key-hold
            // This enables custom key-hold support with controlled repeat rate
            if (state.gameMode === 'manual') {
                const actor = state.actors.get(state.activeActorId);
                const now = Date.now();

                // Key release detection: remove keys not seen recently (500ms timeout)
                const KEY_RELEASE_TIMEOUT_MS = 500;
                for (const [key, lastSeen] of state.manualKeyLastSeen.entries()) {
                    if (now - lastSeen > KEY_RELEASE_TIMEOUT_MS) {
                        state.manualKeys.delete(key);
                        state.manualKeyLastSeen.delete(key);
                    }
                }

                // Movement based on pressed keys
                const moved = manualKeysMovement(state, actor);
                if (moved) {
                    // Interrupt any click-movement when using keyboard
                    state.manualPath = [];
                    state.manualMoveTarget = null;
                }
            }

            if (state.debugMode && (state.tickCount <= 5 || state.tickCount % 50 === 0)) {
                const actor = state.actors.get(state.activeActorId);
                log.debug('Tick status', {
                    tick: state.tickCount,
                    actorX: actor.x,
                    actorY: actor.y,
                    heldItem: actor.heldItem ? actor.heldItem.id : -1,
                    gameMode: state.gameMode
                });
            }

            if (state.ticker && state.tickCount % 50 === 0) {
                const tickerErr = state.ticker.err();
                if (tickerErr) {
                    log.error('BT TICKER ERROR', {error: String(tickerErr), tick: state.tickCount});
                }
            }

            // Sync blackboard: In manual mode, use lightweight sync to avoid expensive pathfinding
            // In automatic mode, run full sync with pathfinding
            if (state.gameMode === 'automatic') {
                syncToBlackboard(state); // Full sync with pathfinding
            } else {
                // Lightweight sync: update only actor position and held item state
                if (state.blackboard) {
                    const actor = state.actors.get(state.activeActorId);
                    state.blackboard.set('actorX', actor.x);
                    state.blackboard.set('actorY', actor.y);
                    state.blackboard.set('heldItemExists', actor.heldItem !== null);
                    state.blackboard.set('heldItemId', actor.heldItem ? actor.heldItem.id : -1);
                    // Note: Skip pathBlocker calculations in manual mode for performance
                }
            }
            return [state, tea.tick(16, 'tick')];
        }

        // MOUSE INTERACTION (Manual Mode Only)
        if (msg.type === 'Mouse' && msg.event === 'press' && state.gameMode === 'manual') {
            const actor = state.actors.get(state.activeActorId);
            const spaceX = Math.floor((state.width - state.spaceWidth) / 2);
            const clickX = msg.x - spaceX;
            const clickY = msg.y;

            // Bounds check
            if (clickX < 0 || clickX >= state.spaceWidth || clickY < 0 || clickY >= state.height) {
                return [state, null];
            }

            // Identify clicked entity
            let clickedCube = null;
            for (const c of state.cubes.values()) {
                if (!c.deleted && Math.round(c.x) === clickX && Math.round(c.y) === clickY) {
                    clickedCube = c;
                    break;
                }
            }

            const dist = Math.sqrt(Math.pow(clickX - actor.x, 2) + Math.pow(clickY - actor.y, 2));
            const isHolding = actor.heldItem !== null;
            let performedAction = false;

            log.debug("Manual Mouse: Click details", {clickX, clickY, actorX: actor.x, actorY: actor.y, isHolding, clickedCube: clickedCube ? `id:${clickedCube.id}` : null});

            if (isHolding) {
                // PLACE: Can place if empty cell and within adjacency (approx 1.5)
                if (!clickedCube && dist <= 1.5) {
                    const heldId = actor.heldItem.id;
                    const c = state.cubes.get(heldId);
                    if (c) {
                        c.deleted = false;
                        c.x = clickX;
                        c.y = clickY;
                        actor.heldItem = null;
                        performedAction = true;
                        log.debug("MANUAL: HeldItem cleared (item placed)", {heldId, tick: state.tickCount});

                        // Check win condition
                        if (heldId === TARGET_ID && isInGoalArea(clickX, clickY)) {
                            state.winConditionMet = true;
                        }
                        log.info("Manual Place", {id: heldId, at: {x: clickX, y: clickY}});
                    }
                } else {
                    log.debug("Manual Place: Failed", {reason: clickedCube ? "clicked on occupied cube" : "too far", clickedCube, dist});
                }
            } else {
                // PICK: Can pick if valid cube and within PICK_THRESHOLD
                if (clickedCube && !clickedCube.isStatic && dist <= PICK_THRESHOLD) {
                    clickedCube.deleted = true;
                    actor.heldItem = {id: clickedCube.id};
                    performedAction = true;
                    log.info("Manual Pick", {id: clickedCube.id, at: {x: clickX, y: clickY}});
                } else {
                }
            }

            // Click-to-Move Path Calculation (Calculate Once, Traverse Later)
            if (performedAction) {
                state.manualPath = []; // Stop moving if we interacted
                state.manualMoveTarget = null;
                state.pathStuckTicks = 0;
            } else {
                // Calculate full path NOW
                const ignoreId = actor.heldItem ? actor.heldItem.id : -1;
                const path = findPath(state, actor.x, actor.y, clickX, clickY, ignoreId);

                if (path && path.length > 0) {
                    log.debug("Manual Move: Path calculated", {steps: path.length, target: {x: clickX, y: clickY}});
                    state.manualPath = path;
                    state.manualMoveTarget = {x: clickX, y: clickY};
                } else {
                    log.debug("Manual Move: No path found or already there");
                    state.manualPath = [];
                    state.manualMoveTarget = null;
                }
            }

            // Force immediate update by returning null command (wait for next natural tick) or just state
            return [state, null];
        }

        // KEYBOARD INTERACTION
        if (msg.type === 'Key') {
            // Get actor reference for mode switch logging
            const actor = state.actors.get(state.activeActorId);
            
            if (msg.key === 'q') return [state, tea.quit()];
            if (msg.key === 'm') {
                const wasManual = state.gameMode === 'manual';
                const oldMode = state.gameMode;
                const newMode = state.gameMode === 'automatic' ? 'manual' : 'automatic';
                state.gameMode = newMode;
                state.manualMoveTarget = null;
                state.manualPath = [];
                state.pathStuckTicks = 0;

                log.debug('Mode switch: Setting gameMode', {oldMode, newMode, wasManual, hasHeldItem: actor.heldItem !== null, heldItemId: actor.heldItem ? actor.heldItem.id : null});

                // Debug logging
                log.debug('Mode switch', {oldMode, newMode, wasManual});

                // When resuming automatic mode, sync blackboard with full pathfinding
                if (wasManual && state.gameMode === 'automatic') {
                    syncToBlackboard(state);
                }
                return [state, null];
            }
            if (msg.key === ' ') {
                state.paused = !state.paused;
                return [state, null];
            }

            // Escape key: Cancel current movement path
            if (msg.key === 'escape') {
                state.manualPath = [];
                state.manualMoveTarget = null;
                state.pathStuckTicks = 0;
                return [state, null];
            }

            // WASD Key Tracking for Manual Mode - Key Press
            if (state.gameMode === 'manual' && ['w', 'a', 's', 'd'].includes(msg.key)) {
                // Track key press state for custom key-hold
                state.manualKeys.set(msg.key, true);
                state.manualKeyLastSeen.set(msg.key, Date.now()); // Track press timestamp for release detection
                return [state, null];
            }

            // WASD Key Release detection - timeout-based approach
            // Bubbletea doesn't provide explicit KeyRelease for keyboard
            // We detect releases by checking if a key hasn't been seen for a short period
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

            // Build mk (manualKeys) array for debug output
            const mk = [];
            if (state.gameMode === 'manual') {
                for (const key of ['w', 'a', 's', 'd']) {
                    if (state.manualKeys && state.manualKeys.get(key)) {
                        mk.push(key);
                    }
                }
            }

            const debugJSON = JSON.stringify({
                m: state.gameMode === 'automatic' ? 'a' : 'm',
                t: state.tickCount,
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
                pst: state.pathStuckTicks,
                mk: mk.length > 0 ? mk : null
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
        }
    });

    // SAFE EXPORTS:
    // 1. Check if 'module' exists (Node/Test Env)
    // 2. Export functions for testing
    // MUST BE INSIDE THE SCOPE where init/update are defined
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

// EXECUTION CONTROL:
// Run ONLY if we are in OSM runtime.
// WE ASSUME: In OSM runtime, 'module' is UNDEFINED.
// WE ASSUME: In Test runtime, 'module' is DEFINED.
// So: Run if module is undefined.
// OR: Run if module IS defined but require.main === module.
{
    let shouldRun = true;
    if (typeof module !== 'undefined') {
        // We are in Node-like environment. Check if main.
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
