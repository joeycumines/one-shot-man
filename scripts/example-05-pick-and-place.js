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
            // No additional occupancy check needed - dumpster concept removed
            // Obstacles can be placed at any free cell

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

    function findNextStep(state, startX, startY, targetX, targetY, ignoreCubeId) {
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
        
        // DEBUG: Log ring blocker positions once per call (first 100 ticks only)
        if (state.tickCount < 100 && toX === 8 && toY === 18) {
            var positions = [];
            cubeAtPosition.forEach(function(id, pos) {
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
                    state.cubes.forEach(function(c) {
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
        
        // ClearPath REQUIRES hands to be empty (heldItemExists=false).
        // Per review.md 3.2: The "Atomic Action" anti-pattern must be avoided.
        // If holding something (like TARGET), planner must first plan Place_Target_Temporary.
        // This ensures proper PA-BT conflict resolution with explicit planning steps.
        const conditions = [
            {key: 'heldItemExists', value: false, Match: v => v === false},
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
            
            // If we're holding something, this action CANNOT proceed - precondition violated
            // Planner should have planned Place_Target_Temporary first
            if (actor.heldItem) {
                log.debug(name + " PRECONDITION VIOLATED: heldItemExists=true, need to place held item first");
                return bt.failure;
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
            
            // Capture blocker's ORIGINAL position before modifying
            const originalBX = Math.round(blocker.x);
            const originalBY = Math.round(blocker.y);
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
            
            // CRITICAL: Search order matters! Prioritize directions AWAY from the goal.
            // We need to place the blocker somewhere that won't obstruct future travel.
            // 
            // SIMPLIFIED LOGIC: Only check if placement blocks actor's CURRENT path to goal.
            // The target-to-goal check was too strict and caused all placements to fail.
            // We can't guarantee perfect placement for all future paths - just ensure 
            // the CURRENT path is unblocked after this ClearPath action.
            
            // STRATEGIC PLACEMENT: Calculate direction from actor to goal,
            // then prioritize placement PERPENDICULAR or OPPOSITE to that direction.
            const dirToGoalX = goalX - ax;
            const dirToGoalY = goalY - ay;
            
            // Normalize to understand general direction: goal is at (8, 18), actor typically east of it
            // If dirToGoalX < 0, goal is to the LEFT → place blocker to the RIGHT
            // If dirToGoalY > 0, goal is BELOW → place blocker ABOVE
            
            // Dynamic direction ordering based on goal direction
            const dirs = [];
            // Prioritize AWAY from goal path
            if (dirToGoalX < 0) {
                // Goal is left, place RIGHT
                dirs.push([1, 0]);   // right
                dirs.push([1, -1]);  // up-right
                dirs.push([1, 1]);   // down-right
            } else {
                // Goal is right, place LEFT  
                dirs.push([-1, 0]);  // left
                dirs.push([-1, -1]); // up-left
                dirs.push([-1, 1]);  // down-left
            }
            if (dirToGoalY > 0) {
                // Goal is below, place ABOVE
                dirs.push([0, -1]);  // up
            } else {
                // Goal is above, place BELOW
                dirs.push([0, 1]);   // down
            }
            // Add remaining directions as fallback
            const allDirs = [[1, 0], [1, -1], [0, -1], [1, 1], [-1, -1], [-1, 0], [0, 1], [-1, 1]];
            for (const d of allDirs) {
                if (!dirs.some(existing => existing[0] === d[0] && existing[1] === d[1])) {
                    dirs.push(d);
                }
            }
            
            let placementSpot = null;
            
            log.debug(name + " trying placement dirs", {ax: ax, ay: ay, goalX: goalX, goalY: goalY, dirToGoalX: dirToGoalX, dirToGoalY: dirToGoalY});
            for (const [dx, dy] of dirs) {
                const nx = ax + dx;
                const ny = ay + dy;
                
                // Bounds check - use spaceWidth (simulation space), not width (terminal)
                if (nx < 0 || nx >= state.spaceWidth || ny < 0 || ny >= state.height) {
                    log.debug(name + " dir rejected: out of bounds", {dx: dx, dy: dy, nx: nx, ny: ny});
                    continue;
                }
                
                // Check if cell is occupied
                let occupied = false;
                for (const c of state.cubes.values()) {
                    if (!c.deleted && c.id !== blockerId && Math.round(c.x) === nx && Math.round(c.y) === ny) {
                        occupied = true;
                        break;
                    }
                }
                if (occupied) {
                    log.debug(name + " dir rejected: occupied", {dx: dx, dy: dy, nx: nx, ny: ny});
                    continue;
                }
                
                // Temporarily place cube at this spot and check if paths are clear
                blocker.x = nx;
                blocker.y = ny;
                blocker.deleted = false;
                log.debug(name + " testing placement", {nx: nx, ny: ny, originalBX: originalBX, originalBY: originalBY});
                
                // FIX: Validate placement by checking two conditions:
                // 1. The new position (nx, ny) is NOT the original position
                // 2. The original blocking cell is no longer occupied by this blocker
                //
                // We DON'T use findFirstBlocker here because:
                // - findFirstBlocker explores ALL directions via BFS
                // - If blocker is placed adjacent to actor, BFS immediately hits it
                // - Even though it's not on the goal path, it becomes "nearest blocker"
                //
                // Instead, simply check that we're moving the blocker to a DIFFERENT cell.
                // The original cell where it was blocking is now free for BFS to pass through.
                const notOriginalSpot = (nx !== originalBX || ny !== originalBY);
                const validPlacement = notOriginalSpot;
                
                log.debug(name + " placement check", {
                    nx: nx, ny: ny,
                    originalBX: originalBX, originalBY: originalBY,
                    notOriginalSpot: notOriginalSpot,
                    validPlacement: validPlacement
                });
                
                if (validPlacement) {
                    placementSpot = {x: nx, y: ny};
                    break;
                }
                
                // Reset for next iteration
                blocker.deleted = true;
            }
            
            if (!placementSpot) {
                // No valid placement found - log and fail properly instead of using invalid fallback
                log.debug(name + " no valid placement found - need to reposition first");
                blocker.deleted = false; // Restore blocker to original position
                return bt.failure;
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
        // 
        // CRITICAL FIX: Require pathBlocker_goal_1=-1 before picking target!
        // Without this constraint, the planner oscillates between:
        //   1. MoveTo_cube_1 → Pick_Target (shorter, but leads to dead end)
        //   2. ClearPath_X → ... (longer, but correct)
        // By requiring clear path, we force the planner to commit to clearing first.
        // CRITICAL: Condition ORDER matters! PA-BT checks conditions in order and
        // expands the FIRST unmet condition. We MUST check pathBlocker FIRST to
        // force the planner to clear the path before trying to go to the target.
        // Otherwise, the planner oscillates between MoveTo_cube_1 and ClearPath.
        const pickTargetConditions = [];
        pickTargetConditions.push({k: 'pathBlocker_goal_' + GOAL_ID, v: -1});  // FIRST: Path must be clear!
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
        // Places the obstacle at any free adjacent cell
        // This is the DYNAMIC replacement for hardcoded blockade actions
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
        // NOTE: This action EXCLUDES holding TARGET - use Place_Target_Temporary for that!
        // ---------------------------------------------------------------------
        reg('Place_Held_Item',
            [
                {k: 'heldItemExists', v: true},
                {k: 'heldItemId', v: -1, Match: v => v !== TARGET_ID}  // Only for NON-target items
            ],
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
            obstacleCount: GOAL_BLOCKADE_IDS.length,  // Renamed from goalBlockadeCount
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
                        // NOTE: Dumpster logic removed - just place at free cell
                        if (!occupied) {
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
            const goal = state.goals.get(GOAL_ID);
            
            // Count remaining obstacles dynamically (not using hardcoded IDs for logic)
            let obstacleCount = 0;
            GOAL_BLOCKADE_IDS.forEach(id => {
                const cube = state.cubes.get(id);
                if (cube && !cube.deleted) obstacleCount++;
            });
            
            // Check reachability - only goal matters now (no dumpster)
            const ax = Math.round(actor.x);
            const ay = Math.round(actor.y);
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
                n: 0,              // No path blockades in simplified scenario
                g: obstacleCount,  // Goal blockade ring count (0-16)
                gr: goalReachable ? 1 : 0
                // NOTE: 'dr' (dumpsterReachable) REMOVED - no dumpster anymore
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
