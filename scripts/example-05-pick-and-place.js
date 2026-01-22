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
 *    Do not assert that an atomic action satisfies a high-level aggregate condition
 *    unless it physically guarantees it in a single tick.
 *    - BAD: Action `Pick_Obstacle` has Effect `isPathClear = true`.
 *      (Reasoning: If 8 obstacles exist, picking one does NOT make the path clear.
 *      The Planner will loop infinitely: Plan Pick -> Execute -> Verify (Path still blocked) -> Replan Pick.)
 *    - GOOD: Action `Pick_Obstacle_A` has Effect `isCleared(Obstacle_A) = true`.
 *      (Reasoning: The planner generates a sequence to clear A, then B, then C, until `isPathClear` is naturally satisfied).
 *
 * 2. STATE GRANULARITY (The "Reward Sparsity" Problem)
 *    PA-BT requires granular feedback to measure progress. Do not use binary flags
 *    for multi-step states.
 *    - The Blackboard must reflect incremental progress. If a wall is composed of
 *      multiple entities, the Precondition must be `!Overlaps(Entity_ID)`, not `!Blocked`.
 *
 * 3. DYNAMIC ACTION GENERATION COMPLETENESS
 *    When decomposing high-level conditions (e.g., `reachable`), the `ActionGenerator`
 *    must be capable of expanding ALL dependency chains.
 *    - If `Pick` requires `Hands_Empty`, and `Hands_Empty` fails, the generator
 *      MUST produce a valid `Place/Drop` action.
 *    - Failure to generate a valid bridging action for a deep dependency will cause
 *      the Go-side planner to enter an infinite expansion loop, saturating the
 *      `RunOnLoopSync` bridge and freezing the JavaScript simulation tick.
 *
 * **WARNING RE: LOGGING:** This is an _interactive_ terminal application.
 *    It sets the terminal to raw mode.
 *    DO NOT use console logging within the program itself (tea.run).
 *    Instead, use the built-in 'log' global, for application logs (slog).
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

    // ============================================================================
    // SCENARIO CONFIGURATION
    // ============================================================================
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
    // ============================================================================

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
    const DUMPSTER_ID = 2;
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

    // Helper: Check if point is within the Blockade Ring (surrounding the goal area)
    // The ring is a 5x5 area minus the 3x3 goal area = 16 cells
    function isInBlockadeRing(x, y) {
        const ringMinX = GOAL_CENTER_X - GOAL_RADIUS - 1;
        const ringMaxX = GOAL_CENTER_X + GOAL_RADIUS + 1;
        const ringMinY = GOAL_CENTER_Y - GOAL_RADIUS - 1;
        const ringMaxY = GOAL_CENTER_Y + GOAL_RADIUS + 1;
        
        const inRingBounds = x >= ringMinX && x <= ringMaxX && y >= ringMinY && y <= ringMaxY;
        return inRingBounds && !isInGoalArea(x, y);
    }

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
                    isGoalBlockade: true,
                    type: 'goal_blockade'
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
                [GOAL_ID, {id: GOAL_ID, x: GOAL_CENTER_X, y: GOAL_CENTER_Y, forTarget: true}],
                [DUMPSTER_ID, {id: DUMPSTER_ID, x: 8, y: 4, forBlockade: true}]
            ]),

            blackboard: null,
            pabtPlan: null,
            pabtState: null,
            activeActorId: 1,
            gameMode: 'automatic',
            tickCount: 0,

            winConditionMet: false,
            targetDelivered: false,

            renderBuffer: null,
            renderBufferWidth: 0,
            renderBufferHeight: 0,

            debugMode: os.getenv('OSM_TEST_MODE') === '1'
        };
    }

    // ============================================================================
    // Logic Helpers
    // ============================================================================

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
            // Check walls/static
            if (!occupied) {
                // Check goals? Goals are usually walkable/placeable unless they are Dumpsters
                // Actually, for generic placement, we avoid placing ON the dumpster to avoid accidental deletion
                // unless intended.
                if (nx === 8 && ny === 4) occupied = true; // Avoid accidental dumpster drop
            }

            if (!occupied) return {x: nx, y: ny};
        }
        return null;
    }

    // ============================================================================
    // Pathfinding
    // ============================================================================

    function buildBlockedSet(state, ignoreCubeId) {
        const blocked = new Set();
        const key = (x, y) => x + ',' + y;
        const actor = state.actors.get(state.activeActorId);

        state.cubes.forEach(c => {
            if (c.deleted) return;
            if (c.id === ignoreCubeId) return;
            if (actor.heldItem && c.id === actor.heldItem.id) return;
            blocked.add(key(Math.round(c.x), Math.round(c.y)));
        });

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

                if (nx < 1 || nx >= state.width - 1 || ny < 1 || ny >= state.height - 1) continue;
                if (visited.has(nKey)) continue;
                if (blocked.has(nKey)) continue;

                visited.add(nKey);
                queue.push({x: nx, y: ny, dist: current.dist + 1});
            }
        }

        return {reachable: false, distance: Infinity};
    }

    function findNextStep(state, startX, startY, targetX, targetY, ignoreCubeId) {
        const blocked = buildBlockedSet(state, ignoreCubeId);
        const key = (x, y) => x + ',' + y;
        const iStartX = Math.round(startX);
        const iStartY = Math.round(startY);
        const iTargetX = Math.round(targetX);
        const iTargetY = Math.round(targetY);

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

            if (nx < 1 || nx >= state.width - 1 || ny < 1 || ny >= state.height - 1) continue;
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

                if (nx < 1 || nx >= state.width - 1 || ny < 1 || ny >= state.height - 1) continue;
                if (blocked.has(nKey) && !(nx === iTargetX && ny === iTargetY)) continue;
                if (visited.has(nKey)) continue;

                visited.add(nKey);
                queue.push({x: nx, y: ny, firstX: cur.firstX, firstY: cur.firstY});
            }
        }

        return null;
    }

    // ============================================================================
    // Dynamic Blocker Discovery (PA-BT Dynamic Obstacle Handling)
    // ============================================================================
    
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
                return null; // Path exists, no blocker
            }
            
            const dirs = [[0, 1], [0, -1], [1, 0], [-1, 0]];
            for (const [ox, oy] of dirs) {
                const nx = current.x + ox;
                const ny = current.y + oy;
                const nKey = key(nx, ny);
                
                if (nx < 1 || nx >= state.width - 1 || ny < 1 || ny >= state.height - 1) continue;
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
        
        // Path is blocked - return the nearest movable blocker
        if (frontier.length > 0) {
            // Sort by distance to actor and return closest
            frontier.sort((a, b) => {
                const distA = Math.abs(a.x - fromX) + Math.abs(a.y - fromY);
                const distB = Math.abs(b.x - fromX) + Math.abs(b.y - fromY);
                return distA - distB;
            });
            return frontier[0].id;
        }
        
        return null; // No movable blockers found (blocked by walls only)
    }

    // ============================================================================
    // Blackboard Synchronization
    // ============================================================================

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

        // Entities - track proximity ONLY (path blockers computed on-demand by ActionGenerator)
        // OPTIMIZATION: Removed per-cube pathBlocker computation - it's O(cubes × map_area) per tick!
        // Path blockers are now computed lazily when the planner needs them.
        state.cubes.forEach(cube => {
            if (!cube.deleted) {
                const dist = Math.sqrt(Math.pow(cube.x - ax, 2) + Math.pow(cube.y - ay, 2));
                const atEntity = dist <= 1.8;
                bb.set('atEntity_' + cube.id, atEntity);
            } else {
                bb.set('atEntity_' + cube.id, false);
            }
        });

        // Goals - track proximity and path blockers
        state.goals.forEach(goal => {
            const dist = Math.sqrt(Math.pow(goal.x - ax, 2) + Math.pow(goal.y - ay, 2));
            // 1.5 threshold ensures physical adjacency for goal area
            bb.set('atGoal_' + goal.id, dist <= 1.5);
            
            // Check path blocker to goal - ONLY when holding target!
            // When not holding target: set to -1 (irrelevant, agent should go get target first)
            // When holding target: compute from ACTOR to goal (we need to carry target there)
            const goalX = Math.round(goal.x);
            const goalY = Math.round(goal.y);
            const actor = state.actors.get(state.activeActorId);
            let blocker = null;
            
            if (actor.heldItem && actor.heldItem.id === TARGET_ID) {
                // Holding target: compute path from ACTOR to goal
                blocker = findFirstBlocker(state, ax, ay, goalX, goalY, TARGET_ID);
            }
            // else: Not holding target - don't compute blocker, leave as -1
            // This allows agent to go pick up target first without clearing goal path
            
            // Convert null (no blocker) to -1 for blackboard consistency
            bb.set('pathBlocker_goal_' + goal.id, blocker === null ? -1 : blocker);
        });

        bb.set('cubeDeliveredAtGoal', state.winConditionMet);
    }

    // ============================================================================
    // TRUE PARAMETRIC ACTIONS via ActionGenerator
    // ============================================================================

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

        // DYNAMIC PRECONDITION for GOAL MoveTo only:
        // - Goal MoveTo requires clear path to prevent deadlock (holding target, blocked)
        // - Entity MoveTo does NOT require clear path - it handles blocking via runtime pathfinding
        // This is because clearing path to an entity may itself be blocked, creating infinite regression
        const conditions = [];
        if (entityType === 'goal') {
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

            // CRITICAL FIX (ISSUE-002): Reduced threshold from 2.5 to 1.5
            // Must match syncToBlackboard threshold for atGoal to prevent livelock
            const threshold = (entityType === 'goal' && entityId === GOAL_ID) ? 1.5 : 1.5;

            if (dist <= threshold) {
                log.debug("MoveTo " + name + " reached target, dist=" + dist.toFixed(2) + ", threshold=" + threshold);
                return bt.success;
            }

            const nextStep = findNextStep(state, actor.x, actor.y, targetX, targetY, ignoreCubeId);
            if (nextStep) {
                const stepDx = nextStep.x - actor.x;
                const stepDy = nextStep.y - actor.y;
                const oldX = actor.x, oldY = actor.y;
                actor.x += Math.sign(stepDx) * Math.min(1.0, Math.abs(stepDx));
                actor.y += Math.sign(stepDy) * Math.min(1.0, Math.abs(stepDy));
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

    // =========================================================================
    // createPickObstacleAction: Dynamic pick action for ANY obstacle cube
    // Generated on-demand by ActionGenerator when planner needs heldItemId=X
    // =========================================================================
    function createPickObstacleAction(state, cubeId) {
        const name = 'Pick_Obstacle_' + cubeId;
        
        const conditions = [
            {key: 'heldItemExists', Match: v => v === false},
            {key: 'atEntity_' + cubeId, Match: v => v === true}
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
        
        const node = bt.createLeafNode(function() {
            return state.gameMode === 'automatic' ? tickFn() : bt.running;
        });
        
        const action = pabt.newAction(name, conditions, effects, node);
        log.debug("Created dynamic action: " + name);
        return action;
    }

    // =========================================================================
    // createClearPathAction: Composite action to clear a blocker from path
    // This is a sequence: MoveTo blocker -> Pick_Obstacle_X -> Place_Obstacle
    // Generated when MoveTo fails due to blocked path
    // =========================================================================
    function createClearPathAction(state, blockerId, destinationKey) {
        const name = 'ClearPath_' + blockerId + '_for_' + destinationKey;
        
        // ClearPath only needs to be at the blocker location
        // NOTE: No heldItemExists precondition! The action handles placing held items internally.
        // This avoids the planner deadlock when holding target but needing to clear path.
        const conditions = [
            {key: 'atEntity_' + blockerId, value: true, Match: v => v === true}
        ];
        
        // Effect: The path blocker is cleared (cube moved to new location)
        // We set pathBlocker_destination = -1 to indicate path is clear
        const effects = [
            {key: 'pathBlocker_' + destinationKey, Value: -1}
        ];
        
        const tickFn = function () {
            log.info("BT TICK: " + name + " executing at tick " + state.tickCount);
            if (state.gameMode !== 'automatic') return bt.running;
            
            const actor = state.actors.get(state.activeActorId);
            
            // If we're already holding something (like the TARGET), place it temporarily
            // then continue to the blocker clearing phase IN THE SAME TICK (atomic operation)
            // This prevents PA-BT replan from picking up the target again.
            if (actor.heldItem) {
                const heldId = actor.heldItem.id;
                // Place it somewhere nearby (not at goal area, not blocking path)
                const spot = getFreeAdjacentCell(state, actor.x, actor.y, false);
                if (!spot) {
                    log.debug(name + " no free spot to place held item");
                    return bt.failure;
                }
                
                // Re-instantiate the cube at the free spot
                const cube = state.cubes.get(heldId);
                if (cube) {
                    cube.deleted = false;
                    cube.x = spot.x;
                    cube.y = spot.y;
                }
                actor.heldItem = null;
                
                if (state.blackboard) {
                    state.blackboard.set('heldItemExists', false);
                    state.blackboard.set('heldItemId', -1);
                }
                
                log.info("PA-BT action executing", {
                    action: name,
                    subAction: "PlaceHeldItem",
                    result: "CONTINUING",
                    tick: state.tickCount,
                    placedId: heldId,
                    placedAt: {x: spot.x, y: spot.y}
                });
                
                // FALL THROUGH to blocker clearing - do NOT return!
                // This makes the entire operation atomic in one tick.
            }
            
            // Check if blocker still exists
            const blocker = state.cubes.get(blockerId);
            if (!blocker || blocker.deleted) {
                log.debug(name + " blocker already cleared");
                // Mark path as clear
                if (state.blackboard) {
                    state.blackboard.set('pathBlocker_' + destinationKey, -1);
                }
                return bt.success;
            }
            
            // Check if we're close enough to pick the blocker
            const dx = blocker.x - actor.x;
            const dy = blocker.y - actor.y;
            const dist = Math.sqrt(dx * dx + dy * dy);
            
            if (dist > PICK_THRESHOLD) {
                // Need to get closer - return failure to trigger MoveTo expansion
                log.debug(name + " need to get closer to blocker", {dist: dist, threshold: PICK_THRESHOLD});
                return bt.failure;
            }
            
            // ATOMIC pick-and-place: pick up the blocker and immediately place it
            // to avoid mid-operation re-planning when heldItemExists becomes true
            blocker.deleted = true;
            
            // Find a placement spot that does NOT block the path to goal
            // We need to find a spot where, if we place the cube there, the path is still clear
            const target = state.cubes.get(TARGET_ID);
            const goals = [...state.goals.values()];
            const goal = goals.length > 0 ? goals[0] : null;
            const goalX = goal ? goal.x : 0;
            const goalY = goal ? goal.y : 0;
            
            // Try each adjacent cell and check if it would block the path
            const ax = Math.round(actor.x);
            const ay = Math.round(actor.y);
            
            // CRITICAL: Search order matters! Prioritize directions AWAY from the goal approach path.
            // The approach is from target (45, 11) to goal (8, 18), i.e., from right-top to left-bottom.
            // Good placement spots are: LEFT of goal (x < goalX-2) or BELOW goal (y > goalY+2)
            // So we should try: [-1, 0] (left), [0, 1] (down) before [1, 0] (right), [0, -1] (up)
            // Prioritize going perpendicular/away from the approach path
            const dirs = [
                [-1, 0],  // left - away from approach corridor
                [-1, 1],  // down-left - away from approach
                [0, 1],   // down - might be below goal
                [-1, -1], // up-left
                [1, 1],   // down-right  
                [1, 0],   // right - toward approach, less ideal
                [0, -1],  // up - toward approach, less ideal
                [1, -1]   // up-right - toward approach, least ideal
            ];
            let placementSpot = null;
            
            for (const [dx, dy] of dirs) {
                const nx = ax + dx;
                const ny = ay + dy;
                
                // Bounds check
                if (nx < 0 || nx >= state.width || ny < 0 || ny >= state.height) continue;
                
                // Check if cell is occupied
                let occupied = false;
                for (const c of state.cubes.values()) {
                    if (!c.deleted && c.id !== blockerId && Math.round(c.x) === nx && Math.round(c.y) === ny) {
                        occupied = true;
                        break;
                    }
                }
                if (occupied) continue;
                
                // Temporarily place cube at this spot and check if path is clear
                blocker.x = nx;
                blocker.y = ny;
                blocker.deleted = false;
                
                // Check if placing blocker here clears the ACTOR→GOAL path
                // Note: We check actor→goal, not target→goal, because the actor carries the target
                const stillBlocked = goal ? findFirstBlocker(state, ax, ay, goalX, goalY, TARGET_ID) : null;
                
                // If THIS cube is no longer the first blocker, we've cleared it from the path!
                // (Either path is now clear, or a different cube is the new first blocker)
                if (stillBlocked !== blockerId) {
                    placementSpot = {x: nx, y: ny};
                    break;
                }
                
                // Reset for next iteration
                blocker.deleted = true;
            }
            
            if (!placementSpot) {
                // Fallback: just use any free adjacent cell (better than failing)
                placementSpot = getFreeAdjacentCell(state, actor.x, actor.y, false);
            }
            
            if (!placementSpot) {
                log.debug(name + " no placement spot found - failing");
                blocker.deleted = false; // Restore blocker
                return bt.failure;
            }
            
            // Place the cube at the chosen spot
            blocker.deleted = false;
            blocker.x = placementSpot.x;
            blocker.y = placementSpot.y;
            
            // Blackboard stays clean (no held item, path is clear)
            if (state.blackboard) {
                state.blackboard.set('heldItemExists', false);
                state.blackboard.set('heldItemId', -1);
                state.blackboard.set('pathBlocker_' + destinationKey, -1);
            }
            
            log.info("PA-BT action executing", {
                action: name,
                subAction: "ClearPath_ATOMIC",
                result: "SUCCESS",
                tick: state.tickCount,
                blockerId: blockerId,
                placedAt: {x: placementSpot.x, y: placementSpot.y}
            });
            
            return bt.success;
        };
        
        const node = bt.createLeafNode(function() {
            return state.gameMode === 'automatic' ? tickFn() : bt.running;
        });
        
        const action = pabt.newAction(name, conditions, effects, node);
        log.debug("Created dynamic action: " + name);
        return action;
    }

    function setupPABTActions(state) {
        const actor = () => state.actors.get(state.activeActorId);

        // =====================================================================
        // ACTION GENERATOR - MUST return actions for ALL conditions the planner queries!
        // 
        // PA-BT does NOT auto-discover registered actions. The actionGenerator
        // MUST explicitly return actions that can satisfy the failed condition.
        // 
        // Key insight from graphjsimpl_test.go:
        // - failedCondition.key = the condition key being checked
        // - failedCondition.value = the TARGET value the planner needs to achieve
        // =====================================================================
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
                // -----------------------------------------------------------------
                // cubeDeliveredAtGoal: The GOAL condition!
                // To achieve cubeDeliveredAtGoal=true, we need Deliver_Target
                // But Deliver_Target requires heldItemId=TARGET_ID and atGoal_1=true
                // -----------------------------------------------------------------
                if (key === 'cubeDeliveredAtGoal') {
                    // Return Deliver_Target action (must be created or retrieved)
                    // The registered action should be returned here
                    log.debug("ACTION_GENERATOR: returning Deliver_Target for cubeDeliveredAtGoal");
                    const deliverAction = state.pabtState.GetAction('Deliver_Target');
                    if (deliverAction) {
                        actions.push(deliverAction);
                    }
                }
                
                // -----------------------------------------------------------------
                // heldItemId: Planner needs a specific item to be held
                // Dynamically create Pick_Obstacle_X for any cube (no hardcoded IDs)
                // -----------------------------------------------------------------
                if (key === 'heldItemId') {
                    const itemId = targetValue;
                    if (itemId === TARGET_ID) {
                        log.debug("ACTION_GENERATOR: returning Pick_Target for heldItemId=" + itemId);
                        const pickAction = state.pabtState.GetAction('Pick_Target');
                        if (pickAction) {
                            actions.push(pickAction);
                        }
                    } else if (typeof itemId === 'number' && itemId !== -1 && itemId !== TARGET_ID) {
                        // Dynamic obstacle picking - create action on-the-fly for ANY cube
                        const cube = state.cubes.get(itemId);
                        if (cube && !cube.deleted) {
                            log.debug("ACTION_GENERATOR: creating Pick_Obstacle_" + itemId);
                            const pickObstacle = createPickObstacleAction(state, itemId);
                            if (pickObstacle) actions.push(pickObstacle);
                        }
                    }
                }
                
                // -----------------------------------------------------------------
                // heldItemExists: Planner needs hands to be free (false) or holding (true)
                // Return Place_Held_Item, Place_Target_Temporary, or Place_Obstacle
                // -----------------------------------------------------------------
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
                    }
                }
                
                // -----------------------------------------------------------------
                // atEntity_X: Planner needs actor at entity location
                // Return MoveTo_cube_X
                // -----------------------------------------------------------------
                if (key.startsWith('atEntity_')) {
                    const entityId = parseInt(key.replace('atEntity_', ''), 10);
                    if (!isNaN(entityId)) {
                        log.info("ACTION_GENERATOR: creating MoveTo for entity", {entityId: entityId, tick: state.tickCount});
                        // NOTE: No pathBlocker constraint for moving to entities!
                        // The agent should go to the target first, then clear the goal path.
                        actions.push(createMoveToAction(state, 'cube', entityId));
                    }
                }
                
                // -----------------------------------------------------------------
                // atGoal_X: Planner needs actor at goal location
                // Return MoveTo_goal_X
                // -----------------------------------------------------------------
                if (key.startsWith('atGoal_')) {
                    const goalId = parseInt(key.replace('atGoal_', ''), 10);
                    if (!isNaN(goalId)) {
                        log.debug("ACTION_GENERATOR: creating MoveTo for goal", {goalId: goalId});
                        actions.push(createMoveToAction(state, 'goal', goalId));
                    }
                }
                
                // -----------------------------------------------------------------
                // pathBlocker_X: Dynamic obstacle detection
                // When MoveTo fails due to blocked path, syncToBlackboard sets pathBlocker_X = cubeId
                // ActionGenerator creates ClearPath actions dynamically
                // -----------------------------------------------------------------
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
                    if (targetValue === -1) {
                        if (typeof currentBlocker === 'number' && currentBlocker !== -1) {
                            const cube = state.cubes.get(currentBlocker);
                            if (cube && !cube.deleted) {
                                log.debug("ACTION_GENERATOR: creating ClearPath for pathBlocker", {destination: destId, blockerId: currentBlocker});
                                // Generate ClearPath action dynamically
                                const clearPathAction = createClearPathAction(state, currentBlocker, destId);
                                if (clearPathAction) actions.push(clearPathAction);
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

        // ---------------------------------------------------------------------
        // Pick_Target
        // ---------------------------------------------------------------------
        // CRITICAL: Require path to goal CLEAR before picking up target!
        // Otherwise PA-BT will pick up target first, then be stuck because:
        // - To deliver: need to MoveTo_goal_1
        // - MoveTo_goal_1 needs pathBlocker_goal_1=-1
        // - To clear path: need to pick up the blocker
        // Pick_Target: Pick up the target cube when at its location
        // NOTE: No pathBlocker_goal constraint! Agent should:
        // 1. Go to target (may have obstacles on path to target, but not on path to goal yet)
        // 2. Pick up target
        // 3. Move toward goal, clearing blockers as encountered
        const pickTargetConditions = [];
        pickTargetConditions.push({k: 'heldItemExists', v: false});
        pickTargetConditions.push({k: 'atEntity_' + TARGET_ID, v: true});
        reg('Pick_Target',
            pickTargetConditions,
            [{k: 'heldItemId', v: TARGET_ID}, {k: 'heldItemExists', v: true}],
            function () {
                const a = actor();
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

        // ---------------------------------------------------------------------
        // Deliver_Target: Place target INTO Goal Area
        // ---------------------------------------------------------------------
        reg('Deliver_Target',
            [{k: 'heldItemId', v: TARGET_ID}, {k: 'atGoal_' + GOAL_ID, v: true}],
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

        // ---------------------------------------------------------------------
        // Place_Obstacle: Generic action to place any held obstacle
        // Places the obstacle at any free adjacent cell (NOT at a pre-defined dumpster)
        // This is the DYNAMIC replacement for all Deposit_Blockade_X actions
        // ---------------------------------------------------------------------
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

        // ---------------------------------------------------------------------
        // Place_Target_Temporary: Conflict resolution action
        // When holding target but goal is blocked, place target temporarily
        // so hands are free to clear blockades.
        // ---------------------------------------------------------------------
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

        // ---------------------------------------------------------------------
        // Place_Held_Item (Generic Drop to free hands)
        // Satisfies the removal of Staging Area logic.
        // REFACTORED: No heldItemIsBlockade check - just place any held item
        // Place_Obstacle is preferred for obstacles; this is a fallback for any item
        // ---------------------------------------------------------------------
        reg('Place_Held_Item',
            [{k: 'heldItemExists', v: true}],
            [{k: 'heldItemExists', v: false}, {k: 'heldItemId', v: -1}],
            function () {
                const a = actor();
                if (!a.heldItem) return bt.success;

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

    // ============================================================================
    // Rendering & Helpers
    // ============================================================================

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
                else if (c.type === 'goal_blockade') ch = '▒';
                sprites.push({x: c.x, y: c.y, char: ch, width: 1, height: 1});
            }
        });
        state.goals.forEach(g => {
            let ch = '○';
            if (g.forTarget) ch = '◎';
            else if (g.forBlockade) ch = '⊙';
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

        const rows = [];
        for (let y = 0; y < height; y++) rows.push(buffer.slice(y * width, (y + 1) * width).join(''));
        return rows.join('\n');
    }

    // ============================================================================
    // Model Update & Init
    // ============================================================================

    function init() {
        const state = initializeSimulation();
        state.blackboard = new bt.Blackboard();
        state.pabtState = pabt.newState(state.blackboard);
        setupPABTActions(state);
        syncToBlackboard(state);
        
        log.info("Pick-and-Place simulation initialized", {
            actorX: state.actors.get(state.activeActorId).x,
            actorY: state.actors.get(state.activeActorId).y,
            goalBlockadeCount: GOAL_BLOCKADE_IDS.length,
            targetId: TARGET_ID,
            mode: state.gameMode
        });

        const goalConditions = [{key: 'cubeDeliveredAtGoal', Match: v => v === true}];
        state.pabtPlan = pabt.newPlan(state.pabtState, goalConditions);
        state.ticker = bt.newTicker(100, state.pabtPlan.Node());

        return [state, tea.tick(16, 'tick')];
    }

    function update(state, msg) {
        if (msg.type === 'Tick' && msg.id === 'tick') {
            state.tickCount++;
            
            // Periodic status logging (every 50 ticks or first 5)
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
            
            // Check for ticker errors periodically
            if (state.ticker && state.tickCount % 50 === 0) {
                const tickerErr = state.ticker.err();
                if (tickerErr) {
                    log.error('BT TICKER ERROR', {error: String(tickerErr), tick: state.tickCount});
                }
            }
            
            syncToBlackboard(state);
            return [state, tea.tick(16, 'tick')];
        }

        if (msg.type === 'Mouse' && msg.event === 'press' && state.gameMode === 'manual') {
            const actor = state.actors.get(state.activeActorId);
            const spaceX = Math.floor((state.width - state.spaceWidth) / 2);
            const clickX = msg.x - spaceX;
            const clickY = msg.y;

            // Generic Manipulation Logic for Manual Control
            // 1. Identify closest interactable object/cell
            const dist = (x1, y1, x2, y2) => Math.sqrt(Math.pow(x1 - x2, 2) + Math.pow(y1 - y2, 2));

            // Check adjacent cells
            const dirs = [[0, 0], [0, 1], [0, -1], [1, 0], [-1, 0], [1, 1], [1, -1], [-1, 1], [-1, -1]];
            for (let d of dirs) {
                const nx = Math.round(actor.x) + d[0];
                const ny = Math.round(actor.y) + d[1];

                // If click is on this adjacent cell
                if (Math.abs(clickX - nx) < 0.5 && Math.abs(clickY - ny) < 0.5) {
                    if (actor.heldItem) {
                        // Try Place
                        let occupied = false;
                        for (let c of state.cubes.values()) {
                            if (!c.deleted && Math.round(c.x) === nx && Math.round(c.y) === ny) occupied = true;
                        }
                        // Special: Dumpster
                        if (nx === 8 && ny === 4) {
                            actor.heldItem = null; // Dispose
                        } else if (!occupied) {
                            // Place Generic
                            const c = state.cubes.get(actor.heldItem.id);
                            if (c) {
                                c.deleted = false;
                                c.x = nx;
                                c.y = ny;
                            }
                            actor.heldItem = null;
                            if (isInGoalArea(nx, ny) && c.id === TARGET_ID) state.winConditionMet = true;
                        }
                    } else {
                        // Try Pick
                        for (let c of state.cubes.values()) {
                            if (!c.deleted && !c.isStatic && Math.round(c.x) === nx && Math.round(c.y) === ny) {
                                c.deleted = true;
                                actor.heldItem = {id: c.id};
                                break;
                            }
                        }
                    }
                    break;
                }
            }
        }

        if (msg.type === 'Key') {
            if (msg.key === 'q') return [state, tea.quit()];
            if (msg.key === 'm') state.gameMode = state.gameMode === 'automatic' ? 'manual' : 'automatic';

            // Manual movement keys
            if (state.gameMode === 'manual') {
                const actor = state.actors.get(state.activeActorId);
                let dx = 0, dy = 0;
                if (msg.key === 'w') dy = -1;
                if (msg.key === 's') dy = 1;
                if (msg.key === 'a') dx = -1;
                if (msg.key === 'd') dx = 1;

                if (dx !== 0 || dy !== 0) {
                    const nx = actor.x + dx;
                    const ny = actor.y + dy;
                    // Collision check
                    let blocked = false;
                    for (let c of state.cubes.values()) {
                        if (!c.deleted && Math.round(c.x) === Math.round(nx) && Math.round(c.y) === Math.round(ny)) blocked = true;
                    }
                    if (!blocked) {
                        actor.x = nx;
                        actor.y = ny;
                    }
                }
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
        
        // Debug JSON overlay for test harness (only in test mode)
        if (state.debugMode) {
            const actor = state.actors.get(state.activeActorId);
            const target = state.cubes.get(TARGET_ID);
            const dumpster = state.goals.get(DUMPSTER_ID);
            const goal = state.goals.get(GOAL_ID);
            
            // Count remaining blockades
            let blockadeCount = 0;
            let goalBlockadeCount = 0;
            GOAL_BLOCKADE_IDS.forEach(id => {
                const cube = state.cubes.get(id);
                if (cube && !cube.deleted) goalBlockadeCount++;
            });
            
            // Check reachability
            const ax = Math.round(actor.x);
            const ay = Math.round(actor.y);
            const dumpsterReachable = dumpster ? getPathInfo(state, ax, ay, dumpster.x, dumpster.y).reachable : false;
            const goalReachable = goal ? getPathInfo(state, ax, ay, goal.x, goal.y).reachable : false;
            
            const debugJSON = JSON.stringify({
                m: state.gameMode === 'automatic' ? 'a' : 'm',
                t: state.tickCount,
                x: Math.round(actor.x * 10) / 10,
                y: Math.round(actor.y * 10) / 10,
                h: actor.heldItem ? actor.heldItem.id : -1,
                w: state.winConditionMet ? 1 : 0,
                a: target && !target.deleted ? target.x : null,
                b: target && !target.deleted ? target.y : null,
                n: blockadeCount,
                g: goalBlockadeCount,
                dr: dumpsterReachable ? 1 : 0,
                gr: goalReachable ? 1 : 0
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


} catch (e) {
    printFatalError(e);
    throw e;
}

try {
    tea.run(program, {altScreen: true});
} catch (e) {
    printFatalError(e);
    throw e;
}
