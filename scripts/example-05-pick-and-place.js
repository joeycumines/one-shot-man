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

    // Helper: Check if point is within the Goal Area
    function isInGoalArea(x, y) {
        return x >= GOAL_CENTER_X - GOAL_RADIUS &&
            x <= GOAL_CENTER_X + GOAL_RADIUS &&
            y >= GOAL_CENTER_Y - GOAL_RADIUS &&
            y <= GOAL_CENTER_Y + GOAL_RADIUS;
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

        // Entities
        state.cubes.forEach(cube => {
            if (!cube.deleted) {
                const info = getPathInfo(state, ax, ay, cube.x, cube.y, cube.id);
                bb.set('reachable_cube_' + cube.id, info.reachable);

                const dist = Math.sqrt(Math.pow(cube.x - ax, 2) + Math.pow(cube.y - ay, 2));
                bb.set('atEntity_' + cube.id, dist <= 1.8);

                // Track blockade clearance
                if (cube.isGoalBlockade) {
                    bb.set('goalBlockade_' + cube.id + '_cleared', false);
                }
            } else {
                bb.set('reachable_cube_' + cube.id, false);
                bb.set('atEntity_' + cube.id, false);
                if (cube.isGoalBlockade) {
                    bb.set('goalBlockade_' + cube.id + '_cleared', true);
                }
            }
        });

        // Goals
        state.goals.forEach(goal => {
            const info = getPathInfo(state, ax, ay, goal.x, goal.y);
            bb.set('reachable_goal_' + goal.id, info.reachable);

            const dist = Math.sqrt(Math.pow(goal.x - ax, 2) + Math.pow(goal.y - ay, 2));

            // For Goal Area, check proximity to center implies proximity to area edges?
            // With 1.8 reach, and radius 1, distance to center <= 2.8 effectively?
            // Let's stick to standard Euclidean check to Center for "AtGoal" signal to PA-BT,
            // but rely on pathfinder/MoveTo to handle the last mile.
            bb.set('atGoal_' + goal.id, dist <= 2.5);
        });

        // Special: Check if goal path is clear
        let goalBlockadesRemaining = 0;
        GOAL_BLOCKADE_IDS.forEach(id => {
            const cube = state.cubes.get(id);
            if (cube && !cube.deleted) goalBlockadesRemaining++;
        });
        bb.set('goalPathCleared', goalBlockadesRemaining === 0);

        bb.set('cubeDeliveredAtGoal', state.winConditionMet);
    }

    // ============================================================================
    // TRUE PARAMETRIC ACTIONS via ActionGenerator
    // ============================================================================

    var actionCache = new Map();

    function createMoveToAction(state, entityType, entityId, extraEffects) {
        const extraKey = extraEffects ? JSON.stringify(extraEffects.map(e => e.key)) : '';
        const cacheKey = 'MoveTo_' + entityType + '_' + entityId + extraKey;
        if (actionCache.has(cacheKey)) return actionCache.get(cacheKey);

        const name = 'MoveTo_' + entityType + '_' + entityId;
        let targetKey, reachableKey;

        if (entityType === 'cube') {
            targetKey = 'atEntity_' + entityId;
            reachableKey = 'reachable_cube_' + entityId;
        } else {
            targetKey = 'atGoal_' + entityId;
            reachableKey = 'reachable_goal_' + entityId;
        }

        const conditions = [];

        // For Goal 1 (Delivery), require blockades cleared
        if (entityType === 'goal' && entityId === GOAL_ID && GOAL_BLOCKADE_IDS.length > 0) {
            for (let id of GOAL_BLOCKADE_IDS) {
                conditions.push({
                    key: 'goalBlockade_' + id + '_cleared',
                    Match: v => v === true
                });
            }
        } else {
            conditions.push({
                key: reachableKey, Match: v => v === true
            });
        }

        const effects = [{key: targetKey, Value: true}];
        if (extraEffects) effects.push(...extraEffects);

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

            // Slightly wider acceptance for Goal Area
            const threshold = (entityType === 'goal' && entityId === GOAL_ID) ? 2.5 : 1.8;

            if (dist <= threshold) {
                log.debug("MoveTo " + name + " reached target, dist=" + dist.toFixed(2) + ", threshold=" + threshold);
                return bt.success;
            }

            const nextStep = findNextStep(state, actor.x, actor.y, targetX, targetY, ignoreCubeId);
            if (nextStep) {
                const stepDx = nextStep.x - actor.x;
                const stepDy = nextStep.y - actor.y;
                actor.x += Math.sign(stepDx) * Math.min(1.0, Math.abs(stepDx));
                actor.y += Math.sign(stepDy) * Math.min(1.0, Math.abs(stepDy));
                return bt.running;
            } else {
                log.warn("MoveTo " + name + " pathfinding FAILED at actor(" + actor.x + "," + actor.y + ") -> target(" + targetX + "," + targetY + ")");
                return bt.failure;
            }
        };

        const node = bt.createLeafNode(tickFn);
        const action = pabt.newAction(name, conditions, effects, node);
        actionCache.set(cacheKey, action);
        return action;
    }

    function setupPABTActions(state) {
        const actor = () => state.actors.get(state.activeActorId);

        state.pabtState.setActionGenerator(function (failedCondition) {
            const actions = [];
            const key = failedCondition.key;
            
            // Log EVERY call to actionGenerator to track planner activity
            log.info("ACTION_GENERATOR called", {
                failedKey: key,
                failedKeyType: typeof key,
                tick: state.tickCount
            });

            if (key && typeof key === 'string') {
                if (key.startsWith('atEntity_')) {
                    const entityId = parseInt(key.replace('atEntity_', ''), 10);
                    if (!isNaN(entityId)) {
                        log.debug("ACTION_GENERATOR: creating MoveTo for entity", {entityId: entityId});
                        actions.push(createMoveToAction(state, 'cube', entityId));
                    }
                }
                if (key.startsWith('atGoal_')) {
                    const goalId = parseInt(key.replace('atGoal_', ''), 10);
                    if (!isNaN(goalId)) {
                        log.debug("ACTION_GENERATOR: creating MoveTo for goal", {goalId: goalId});
                        actions.push(createMoveToAction(state, 'goal', goalId));
                    }
                }
                // CRITICAL: When planner needs heldItemExists=false (to pick something new),
                // but actor is holding something, generate Place_Held_Item or Place_Target_Temporary
                if (key === 'heldItemExists') {
                    // The planner is checking this condition. If Match expects false and we're holding,
                    // we need to generate actions to free hands.
                    // Note: Place_Held_Item and Place_Target_Temporary are already registered.
                    // But we may need to dynamically generate them here for deep dependency chains.
                    log.debug("Action generator: heldItemExists condition requested", {key: key});
                }
                // Generate blockade clearing dependencies
                if (key.startsWith('goalBlockade_') && key.endsWith('_cleared')) {
                    const match = key.match(/goalBlockade_(\d+)_cleared/);
                    if (match) {
                        const blockadeId = parseInt(match[1], 10);
                        // Generate MoveTo for this blockade
                        log.debug("ACTION_GENERATOR: creating MoveTo for blockade", {blockadeId: blockadeId});
                        actions.push(createMoveToAction(state, 'cube', blockadeId));
                    }
                }
            }
            
            log.debug("ACTION_GENERATOR returning", {actionCount: actions.length, failedKey: key});
            return actions;
        });

        const reg = function (name, conds, effects, tickFn) {
            const conditions = conds.map(c => ({
                key: c.k, Match: v => c.v === undefined ? v === true : v === c.v
            }));
            const effectList = effects.map(e => ({key: e.k, Value: e.v}));
            const node = bt.createLeafNode(() => state.gameMode === 'automatic' ? tickFn() : bt.running);
            state.pabtState.RegisterAction(name, pabt.newAction(name, conditions, effectList, node));
        };

        // ---------------------------------------------------------------------
        // Pick_Target
        // ---------------------------------------------------------------------
        reg('Pick_Target',
            [{k: 'heldItemExists', v: false}, {k: 'atEntity_' + TARGET_ID, v: true}],
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
        // Pick_Blockade_X
        // ---------------------------------------------------------------------
        GOAL_BLOCKADE_IDS.forEach(function (id) {
            reg('Pick_Blockade_' + id,
                [{k: 'heldItemExists', v: false}, {k: 'atEntity_' + id, v: true}],
                [{k: 'heldItemId', v: id}, {k: 'heldItemExists', v: true}, {k: 'reachable_cube_' + TARGET_ID, v: true}],
                function () {
                    const a = actor();
                    const c = state.cubes.get(id);
                    if (!c || c.deleted) return bt.success;
                    c.deleted = true;
                    a.heldItem = {id: id};
                    
                    log.info("PA-BT action executing", {
                        action: "Pick_Blockade_" + id,
                        result: "SUCCESS",
                        tick: state.tickCount,
                        blockadeId: id,
                        actorX: a.x,
                        actorY: a.y
                    });
                    
                    if (state.blackboard) {
                        state.blackboard.set('heldItemId', id);
                        state.blackboard.set('heldItemExists', true);
                    }
                    return bt.success;
                }
            );

            // ---------------------------------------------------------------------
            // Deposit_Blockade (Dumpster)
            // ---------------------------------------------------------------------
            reg('Deposit_Blockade_' + id,
                [{k: 'heldItemId', v: id}, {k: 'atGoal_' + DUMPSTER_ID, v: true}],
                [{k: 'heldItemExists', v: false}, {k: 'heldItemId', v: -1}, {
                    k: 'goalBlockade_' + id + '_cleared',
                    v: true
                }],
                function () {
                    const a = actor();
                    
                    log.info("PA-BT action executing", {
                        action: "Deposit_Blockade_" + id,
                        result: "SUCCESS",
                        tick: state.tickCount,
                        blockadeId: id,
                        actorX: a.x,
                        actorY: a.y
                    });
                    
                    a.heldItem = null;
                    if (state.blackboard) {
                        state.blackboard.set('heldItemExists', false);
                        state.blackboard.set('heldItemId', -1);
                        state.blackboard.set('goalBlockade_' + id + '_cleared', true);
                    }
                    return bt.success;
                }
            );
        });

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
                    actorY: a.y
                });

                // Re-instantiate the item in the world
                const c = state.cubes.get(a.heldItem.id);
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
        // Debug log every 50 ticks (not every tick to avoid log spam)
        if (msg.type === 'Tick' && msg.id === 'tick') {
            state.tickCount++;
            
            // Check ticker error status periodically
            if (state.ticker && (state.tickCount <= 10 || state.tickCount % 50 === 0)) {
                const tickerErr = state.ticker.err();
                if (tickerErr) {
                    log.error('BT TICKER ERROR', {error: String(tickerErr), tick: state.tickCount});
                }
            }
            
            if (state.tickCount <= 5 || state.tickCount % 50 === 0) {
                const actor = state.actors.get(state.activeActorId);
                log.debug('Tick status', {
                    tick: state.tickCount,
                    actorX: actor.x,
                    actorY: actor.y,
                    heldItem: actor.heldItem ? actor.heldItem.id : -1,
                    gameMode: state.gameMode
                });
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
