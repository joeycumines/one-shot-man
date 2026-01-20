#!/usr/bin/env osm script

// ============================================================================
// example-05-pick-and-place.js
// Pick-and-Place Simulator demonstrating osm:pabt PA-BT planning integration
// ============================================================================

// WARNING RE: LOGGING:
// This is an _interactive_ terminal application. It sets the terminal to
// raw mode. DO NOT use console logging within the program itself (tea.run).
// Instead, use the built-in 'log' global, for application logs (slog).

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
    // Layout:
    //    +-----------------------------------------------------------+
    //    |                                                           |
    //    |    @ (Actor starts here)                                  |
    //    |                                                           |
    //    |          +-------------------------------+                |
    //    |          |                               |                |
    //    |     +--->|   ■ ■ (blockade)  □ (target)  |                |
    //    |     |    |                               |                |
    //    |     |    +-------------------------------+                |
    //    |   GOAL                                                    |
    //    |    ○ (deliver target here)                                |
    //    |                                                           |
    //    +-----------------------------------------------------------+
    //
    // The actor must:
    // 1. Enter the room through the left gap
    // 2. Clear blockade cubes (pick and dispose outside)
    // 3. Pick the target cube
    // 4. Exit the room
    // 5. Deliver target to goal
    //
    // TRUE PARAMETRIC ACTIONS:
    // - MoveTo(entityId) - Generated dynamically based on failed condition
    // - Pick(cubeId) - Pick up any reachable cube
    // - Place() - Place held item at goal
    // ============================================================================

    const ENV_WIDTH = 80;
    const ENV_HEIGHT = 24;

    // Room bounds (the enclosed area)
    const ROOM_MIN_X = 20;
    const ROOM_MAX_X = 55;
    const ROOM_MIN_Y = 6;
    const ROOM_MAX_Y = 16;
    const ROOM_GAP_X = 20; // Entry on left wall
    const ROOM_GAP_Y = 11; // Gap Y position

    // Entity IDs
    const TARGET_ID = 1;
    const GOAL_ID = 1;
    const DUMPSTER_ID = 2;
    const STAGING_AREA_ID = 3; // Where target can be temporarily placed

    // Blockade cube IDs (generated dynamically)
    var BLOCKADE_IDS = [];
    // Goal blockade IDs - movable wall around the goal
    var GOAL_BLOCKADE_IDS = [];

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

        // 2. Blockade Cubes - REMOVED for simpler test scenario
        // Path to target is now clear - only the goal is blocked
        // This isolates the conflict resolution testing to just the goal blockade wall

        // 3. Goal Blockade Cubes - Single blockade for testing PA-BT conflict resolution
        // When goal is blocked, PA-BT should find the Deposit_GoalBlockade_X action to clear path
        let goalBlockadeId = 100;
        // Re-enabled: Testing conflict resolution with single blockade
        const goalBlockadePositions = [
            {x: 8, y: 17}    // Directly N of goal (blocking direct approach)
        ];
        for (const pos of goalBlockadePositions) {
            cubesInit.push([goalBlockadeId, {
                id: goalBlockadeId,
                x: pos.x,
                y: pos.y,
                deleted: false,
                isGoalBlockade: true,
                type: 'goal_blockade'
            }]);
            GOAL_BLOCKADE_IDS.push(goalBlockadeId);
            goalBlockadeId++;
        }

        // 4. Room Walls (static obstacles)
        let wallId = 1000;

        function addWall(x, y) {
            // Leave gap at entry point
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

        // Top and bottom walls
        for (let x = ROOM_MIN_X; x <= ROOM_MAX_X; x++) {
            addWall(x, ROOM_MIN_Y);
            addWall(x, ROOM_MAX_Y);
        }
        // Left and right walls
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
                [GOAL_ID, {id: GOAL_ID, x: 8, y: 18, forTarget: true}],
                [DUMPSTER_ID, {id: DUMPSTER_ID, x: 8, y: 4, forBlockade: true}],
                [STAGING_AREA_ID, {id: STAGING_AREA_ID, x: 5, y: 15, forStaging: true}]
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
    // Pathfinding
    // ============================================================================

    function buildBlockedSet(state, ignoreCubeId) {
        const blocked = new Set();
        const key = (x, y) => x + ',' + y;
        const actor = state.actors.get(state.activeActorId);

        let debugBlockedCubes = [];
        let allGoalBlockades = [];
        state.cubes.forEach(c => {
            // Track ALL goal blockades for debugging
            if (c.id >= 100 && c.id <= 107) {
                allGoalBlockades.push({id: c.id, x: Math.round(c.x), y: Math.round(c.y), deleted: c.deleted});
            }

            if (c.deleted) return;
            if (c.id === ignoreCubeId) return;
            if (actor.heldItem && c.id === actor.heldItem.id) return;
            blocked.add(key(Math.round(c.x), Math.round(c.y)));
            if (c.id >= 100 && c.id <= 107) {
                debugBlockedCubes.push({id: c.id, x: Math.round(c.x), y: Math.round(c.y)});
            }
        });

        // DEBUG: Log ALL goal blockades (including deleted) vs blocked set
        if (state.tickCount === 5) {
            log.error("BLOCKED_SET_DEBUG_TICK5", {
                tick: state.tickCount,
                allGoalBlockadesInMap: allGoalBlockades.length,
                allGoalBlockades: JSON.stringify(allGoalBlockades),
                goalBlockadesInBlockedSet: debugBlockedCubes.length,
                positions: JSON.stringify(debugBlockedCubes),
                ignoreCubeId: ignoreCubeId,
                heldItemId: actor.heldItem ? actor.heldItem.id : -1
            });
        }
        if (state.tickCount === 700) {
            log.error("BLOCKED_SET_DEBUG_TICK700", {
                tick: state.tickCount,
                allGoalBlockadesInMap: allGoalBlockades.length,
                allGoalBlockades: JSON.stringify(allGoalBlockades),
                goalBlockadesInBlockedSet: debugBlockedCubes.length,
                positions: JSON.stringify(debugBlockedCubes),
                ignoreCubeId: ignoreCubeId,
                heldItemId: actor.heldItem ? actor.heldItem.id : -1
            });
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
            const dx = Math.abs(current.x - Math.round(targetX));
            const dy = Math.abs(current.y - Math.round(targetY));

            if (dx <= 1 && dy <= 1) {
                // DEBUG: Log when we find the goal to understand how we got there
                if (Math.round(targetX) === 8 && Math.round(targetY) === 18) {
                    log.warn("BFS_FOUND_GOAL_PATH", {
                        currentCell: current.x + "," + current.y,
                        target: targetX + "," + targetY,
                        dx: dx,
                        dy: dy,
                        distance: current.dist
                    });
                }
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

        // CRITICAL DEBUG: Track heldItemId synchronization
        const heldId = actor.heldItem ? actor.heldItem.id : -1;
        bb.set('heldItemId', heldId);

        // Log EVERY time heldItemId changes to non-negative
        if (heldId >= 0) {
            log.warn("SYNC_HELD_POSITIVE", {
                tick: state.tickCount,
                heldId: heldId,
                actorX: ax,
                actorY: ay,
                verifyGet: bb.get('heldItemId')
            });
        }

        // CRITICAL FIX: needToPlaceTarget is ONLY true when holding target AND
        // goal blockades still exist. This prevents Place_Target_Temporary from
        // being selected after blockades are cleared (which caused infinite pickup/drop loop).
        const holdingTarget = actor.heldItem && actor.heldItem.id === TARGET_ID;
        const goalBlockadesExist = GOAL_BLOCKADE_IDS.some(function (id) {
            const c = state.cubes.get(id);
            return c && !c.deleted;
        });
        bb.set('needToPlaceTarget', holdingTarget && goalBlockadesExist);

        // Target cube
        const target = state.cubes.get(TARGET_ID);
        if (target && !target.deleted) {
            const info = getPathInfo(state, ax, ay, target.x, target.y, TARGET_ID);
            bb.set('reachable_cube_' + TARGET_ID, info.reachable);
            bb.set('distance_cube_' + TARGET_ID, info.distance);
            // FIX: Use Euclidean distance for atEntity_X, NOT BFS distance!
            // BFS distance counts grid steps, but actual "at entity" means within interaction range.
            // Using BFS distance < 2 caused atEntity_1=true when agent was 1 BFS step away
            // but still > 1.8 Euclidean distance away.
            const tdx = target.x - ax;
            const tdy = target.y - ay;
            const euclideanDistToTarget = Math.sqrt(tdx * tdx + tdy * tdy);
            bb.set('atEntity_' + TARGET_ID, euclideanDistToTarget <= 1.8);
        } else {
            bb.set('reachable_cube_' + TARGET_ID, false);
            bb.set('atEntity_' + TARGET_ID, false);
        }

        // Blockade cubes
        BLOCKADE_IDS.forEach(id => {
            const cube = state.cubes.get(id);
            if (cube && !cube.deleted) {
                const info = getPathInfo(state, ax, ay, cube.x, cube.y, id);
                bb.set('reachable_cube_' + id, info.reachable);
                bb.set('distance_cube_' + id, info.distance);
                // FIX: Use Euclidean distance for atEntity_X
                const bdx = cube.x - ax;
                const bdy = cube.y - ay;
                const euclideanDistToBlockade = Math.sqrt(bdx * bdx + bdy * bdy);
                bb.set('atEntity_' + id, euclideanDistToBlockade <= 1.8);
            } else {
                bb.set('reachable_cube_' + id, false);
                bb.set('atEntity_' + id, false);
            }
        });

        // Goal blockade cubes (around the goal)
        GOAL_BLOCKADE_IDS.forEach(id => {
            const cube = state.cubes.get(id);
            if (cube && !cube.deleted) {
                const info = getPathInfo(state, ax, ay, cube.x, cube.y, id);
                bb.set('reachable_cube_' + id, info.reachable);
                bb.set('distance_cube_' + id, info.distance);
                // FIX: Use Euclidean distance for atEntity_X
                const gbdx = cube.x - ax;
                const gbdy = cube.y - ay;
                const euclideanDistToGoalBlockade = Math.sqrt(gbdx * gbdx + gbdy * gbdy);
                bb.set('atEntity_' + id, euclideanDistToGoalBlockade <= 1.8);
                // Track per-blockade cleared status (false = not cleared yet)
                bb.set('goalBlockade_' + id + '_cleared', false);
                if (state.tickCount % 200 === 0 || state.tickCount < 10) {
                    log.warn("BLOCKADE_STATUS", {id: id, cleared: false, exists: true, tick: state.tickCount});
                }
            } else {
                bb.set('reachable_cube_' + id, false);
                bb.set('atEntity_' + id, false);
                // Track per-blockade cleared status (true = cleared/deleted)
                bb.set('goalBlockade_' + id + '_cleared', true);
                if (state.tickCount % 200 === 0 || state.tickCount < 10) {
                    log.warn("BLOCKADE_STATUS", {id: id, cleared: true, exists: false, tick: state.tickCount});
                }
            }
        });

        // Goals
        state.goals.forEach(goal => {
            const info = getPathInfo(state, ax, ay, goal.x, goal.y);
            bb.set('reachable_goal_' + goal.id, info.reachable);
            bb.set('distance_goal_' + goal.id, info.distance);
            // FIX: Use Euclidean distance for atGoal_X, NOT BFS distance!
            // BFS distance counts grid steps, but MoveTo uses Euclidean distance <= 1.8.
            // Using BFS distance < 2 caused atGoal_1=true when agent was 1 BFS step away
            // but still > 1.8 Euclidean distance away, creating an infinite loop.
            const gdx = goal.x - ax;
            const gdy = goal.y - ay;
            const euclideanDistToGoal = Math.sqrt(gdx * gdx + gdy * gdy);
            bb.set('atGoal_' + goal.id, euclideanDistToGoal <= 1.8);  // Match MoveTo threshold

            // DEBUG: Log goal reachability for debugging EVERY TICK for main goal
            if (goal.id === GOAL_ID) {
                const atGoalValue = euclideanDistToGoal <= 1.8;
                if (atGoalValue || state.tickCount % 50 === 1) {
                    log.error("GOAL_STATUS_DEBUG", {
                        tick: state.tickCount,
                        goalType: 'DELIVERY',
                        actorPos: ax + "," + ay,
                        goalPos: goal.x + "," + goal.y,
                        reachable: info.reachable,
                        bfsDistance: info.distance,
                        euclideanDist: Math.round(euclideanDistToGoal * 100) / 100,
                        atGoal: atGoalValue,
                        goalBlockadesRemaining: GOAL_BLOCKADE_IDS.filter(function (id) {
                            const c = state.cubes.get(id);
                            return c && !c.deleted;
                        }).length,
                        heldItemId: actor.heldItem ? actor.heldItem.id : -1
                    });
                }
            }
        });

        // Special: Check if goal path is clear (no goal blockades remaining)
        // This is used by Deliver_Target to ensure path clearing happened
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

    // Cache for dynamically created actions to avoid recreating them every tick
    var actionCache = new Map();

    // Create a MoveTo action for a cube or goal
    // extraEffects: optional array of {key, Value} pairs for additional effects
    function createMoveToAction(state, entityType, entityId, extraEffects) {
        // Include extraEffects in cache key to ensure different variants are cached separately
        const extraKey = extraEffects ? JSON.stringify(extraEffects.map(function (e) {
            return e.key;
        })) : '';
        const cacheKey = 'MoveTo_' + entityType + '_' + entityId + extraKey;
        if (actionCache.has(cacheKey)) {
            return actionCache.get(cacheKey);
        }

        const name = 'MoveTo_' + entityType + '_' + entityId;
        let targetKey, reachableKey;

        if (entityType === 'cube') {
            targetKey = 'atEntity_' + entityId;
            reachableKey = 'reachable_cube_' + entityId;
        } else if (entityType === 'goal') {
            targetKey = 'atGoal_' + entityId;
            reachableKey = 'reachable_goal_' + entityId;
        }

        const conditions = [];

        // CRITICAL: For goal 1 (delivery goal), use per-blockade conditions instead of reachability
        // This allows PA-BT to find Deposit_GoalBlockade_X actions when blocked
        // These are TRUTHFUL conditions - if a blockade is deleted, it IS cleared
        // For other entities, use the standard reachability precondition
        if (entityType === 'goal' && entityId === GOAL_ID && GOAL_BLOCKADE_IDS.length > 0) {
            // Add a condition for EACH blockade that must be cleared
            // PA-BT will find Deposit_GoalBlockade_X actions to satisfy each
            log.warn("MOVETO_GOAL_1_CONDITIONS", {
                blockadeCount: GOAL_BLOCKADE_IDS.length,
                blockadeIds: GOAL_BLOCKADE_IDS.join(",")
            });
            for (var i = 0; i < GOAL_BLOCKADE_IDS.length; i++) {
                const blockadeId = GOAL_BLOCKADE_IDS[i];
                log.debug("MoveTo_goal_1: Adding blockade condition", {blockadeId: blockadeId, index: i});
                conditions.push({
                    key: 'goalBlockade_' + blockadeId + '_cleared',
                    Match: function (v) {
                        return v === true;
                    }
                });
            }
        } else {
            // Standard reachability precondition for non-goal-1 entities
            // Also used for goal 1 when there are NO blockades
            if (entityType === 'goal' && entityId === GOAL_ID) {
                log.warn("MOVETO_GOAL_1_NO_BLOCKADES", {
                    blockadeCount: GOAL_BLOCKADE_IDS.length,
                    reason: "Using standard reachability"
                });
            }
            conditions.push({
                key: reachableKey, Match: function (v) {
                    return v === true;
                }
            });
        }

        const effects = [
            {key: targetKey, Value: true}
        ];

        // Add extra effects if provided (used for heuristic effects like reachable_goal_1)
        if (extraEffects) {
            for (var i = 0; i < extraEffects.length; i++) {
                effects.push(extraEffects[i]);
            }
        }

        const tickFn = function () {
            if (state.gameMode !== 'automatic') {
                return bt.running;
            }

            // Log when MoveTo_goal_1 is being executed
            if (entityType === 'goal' && entityId === GOAL_ID) {
                log.error("MOVETO_GOAL_1_TICK", {
                    tick: state.tickCount,
                    actorX: state.actors.get(state.activeActorId).x,
                    actorY: state.actors.get(state.activeActorId).y
                });
            }

            const actor = state.actors.get(state.activeActorId);
            let targetX, targetY, ignoreCubeId;

            if (entityType === 'cube') {
                const cube = state.cubes.get(entityId);
                if (!cube || cube.deleted) {
                    log.debug(name + " success: cube gone", {entityId: entityId});
                    return bt.success;
                }
                targetX = cube.x;
                targetY = cube.y;
                ignoreCubeId = entityId;
            } else {
                const goal = state.goals.get(entityId);
                if (!goal) {
                    log.error(name + " failed: goal not found", {entityId: entityId});
                    return bt.failure;
                }
                targetX = goal.x;
                targetY = goal.y;
                ignoreCubeId = -1;
            }

            const dx = targetX - actor.x;
            const dy = targetY - actor.y;
            const dist = Math.sqrt(dx * dx + dy * dy);

            // Debug log: track MoveTo execution
            log.debug("MoveTo tick", {
                name: name,
                actorX: Math.round(actor.x),
                actorY: Math.round(actor.y),
                targetX: Math.round(targetX),
                targetY: Math.round(targetY),
                dist: Math.round(dist * 10) / 10
            });

            if (dist <= 1.8) {
                log.debug(name + " success: in range", {dist: dist});
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
                // CRITICAL: Return FAILURE when no path - this triggers PA-BT replanning!
                // This is essential for conflict resolution: agent holding target can't reach goal,
                // so PA-BT replans and discovers it needs to clear blockades but hands are full.
                log.warn(name + " FAILED: no path to destination - triggering replan", {
                    actorX: Math.round(actor.x),
                    actorY: Math.round(actor.y),
                    targetX: Math.round(targetX),
                    targetY: Math.round(targetY)
                });
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

        // =========================================================================
        // ACTION GENERATOR - TRUE PARAMETRIC ACTIONS
        // =========================================================================
        // This generator is called by PA-BT when it needs actions to satisfy
        // a failed condition. Instead of registering MoveTo_1, MoveTo_10, MoveTo_11...
        // we generate them ON DEMAND based on what condition failed.
        // =========================================================================

        state.pabtState.setActionGenerator(function (failedCondition) {
            const actions = [];
            const key = failedCondition.key;
            const currentHeldId = state.blackboard.get('heldItemId');
            const isHoldingItem = state.blackboard.get('heldItemExists');

            // CRITICAL DEBUG: Log to stderr to see what condition is failing
            if (state.tickCount > 280 && state.tickCount % 100 === 0) {
                log.debug("[PABT_DEBUG] tick=" + state.tickCount + " failedKey=" + key +
                    " held=" + currentHeldId +
                    " reachable_3=" + state.blackboard.get('reachable_goal_' + STAGING_AREA_ID));
            }

            log.info("ActionGenerator called for failed condition", {
                failedKey: key,
                heldItemExists: isHoldingItem,
                heldItemId: currentHeldId,
                goalPathCleared: state.blackboard.get('goalPathCleared'),
                reachable_goal_1: state.blackboard.get('reachable_goal_1'),
                reachable_goal_2: state.blackboard.get('reachable_goal_' + DUMPSTER_ID),
                reachable_goal_3: state.blackboard.get('reachable_goal_' + STAGING_AREA_ID),
                atGoal_3: state.blackboard.get('atGoal_' + STAGING_AREA_ID),
                tick: state.tickCount
            });

            // Pattern match on the failed condition key
            // atEntity_X means we need MoveTo actions for cubes
            if (key && typeof key === 'string' && key.startsWith('atEntity_')) {
                const entityId = parseInt(key.replace('atEntity_', ''), 10);
                if (!isNaN(entityId)) {
                    // Generate MoveTo action for this specific cube
                    const cube = state.cubes.get(entityId);
                    if (cube && !cube.deleted) {
                        actions.push(createMoveToAction(state, 'cube', entityId));
                        log.debug("Generated MoveTo action", {target: 'cube', entityId: entityId});
                    }
                }
            }

            // atGoal_X means we need MoveTo actions for goals
            if (key && typeof key === 'string' && key.startsWith('atGoal_')) {
                const goalId = parseInt(key.replace('atGoal_', ''), 10);
                if (!isNaN(goalId)) {
                    const goal = state.goals.get(goalId);
                    if (goal) {
                        // ALWAYS return MoveTo for the requested goal.
                        // Let PA-BT discover precondition failures (reachable_goal_X)
                        // and expand from there to find blockade clearing path.
                        actions.push(createMoveToAction(state, 'goal', goalId));
                        log.debug("Generated MoveTo action for goal", {target: 'goal', goalId: goalId});
                    }
                }
            }

            // =========================================================================
            // Handle reachable_goal_X failures - when goal is blocked
            // DO NOT generate heuristic actions here! PA-BT must find registered
            // actions (Place_Target_Temporary, Pick_GoalBlockade_X, etc.) which have
            // reachable_goal_1=true as effect. ActionGenerator should only generate
            // MoveTo actions for preconditions of those registered actions.
            //
            // LESSON LEARNED: Heuristic effects break PA-BT's trust model. When an
            // action with effect X=true succeeds, PA-BT assumes X is actually true.
            // If it's a heuristic (lie), PA-BT enters an infinite loop.
            // =========================================================================
            if (key && typeof key === 'string' && key.startsWith('reachable_goal_')) {
                const goalId = parseInt(key.replace('reachable_goal_', ''), 10);
                if (goalId === GOAL_ID) {
                    // Log the conflict but DO NOT return any actions.
                    // PA-BT will find registered actions (Place_Target_Temporary,
                    // Pick_GoalBlockade_X, Deposit_GoalBlockade_X) which have this effect.
                    log.info("CONFLICT: reachable_goal_1 failed, letting PA-BT find registered actions", {
                        heldItemExists: isHoldingItem,
                        heldItemId: currentHeldId,
                        goalBlocked: true
                    });
                    // Return empty - DO NOT add heuristic MoveTo actions!
                }
            }

            // Keep the Case 3 hands empty logic ONLY for logging, but don't add heuristic effects
            if (key && typeof key === 'string' && key.startsWith('reachable_goal_')) {
                const goalId = parseInt(key.replace('reachable_goal_', ''), 10);
                if (goalId === GOAL_ID && !isHoldingItem) {
                    log.debug("BLOCKADE_CLEARING: reachable_goal_1 failed with empty hands, PA-BT should find Pick_GoalBlockade_X", {
                        heldItemExists: isHoldingItem
                    });
                    // DO NOT generate any actions here - let PA-BT find registered
                    // Pick_GoalBlockade_X actions which have reachable_goal_1=true effect.
                }
            }

            // =========================================================================
            // Handle goalBlockade_X_cleared failures - when a specific blockade needs clearing
            // PA-BT should find Deposit_GoalBlockade_X which has truthful effect
            // =========================================================================
            if (key && typeof key === 'string' && key.startsWith('goalBlockade_') && key.endsWith('_cleared')) {
                const blockadeId = parseInt(key.replace('goalBlockade_', '').replace('_cleared', ''), 10);
                log.info("BLOCKADE_CONDITION_FAILED: goalBlockade_" + blockadeId + "_cleared", {
                    blockadeId: blockadeId,
                    heldItemExists: isHoldingItem,
                    heldItemId: currentHeldId
                });
                // DO NOT generate actions - PA-BT will find Deposit_GoalBlockade_X
            }

            // =========================================================================
            // CRITICAL FIX: Handle heldItemId/heldItemExists failures when holding
            // something unexpected. If hands are full and PA-BT needs different item,
            // we need to generate appropriate MoveTo actions:
            //    - If holding TARGET: MoveTo staging area (for Place_Target_Temporary)
            //    - If holding blockade: MoveTo dumpster (for Deposit_Blockade_X)
            // =========================================================================
            if (key === 'heldItemId' || key === 'heldItemExists') {
                if (isHoldingItem && currentHeldId > 0) {
                    if (currentHeldId === TARGET_ID) {
                        // Only generate MoveTo staging area if goal blockades still exist!
                        // After blockades are cleared, we DON'T want to go to staging -
                        // we want to go directly to the goal.
                        const goalBlockadesRemaining = GOAL_BLOCKADE_IDS.filter(function (id) {
                            const c = state.cubes.get(id);
                            return c && !c.deleted;
                        }).length;

                        if (goalBlockadesRemaining > 0) {
                            // Holding TARGET and goal is blocked - need to place at staging area
                            // This allows PA-BT to find Place_Target_Temporary
                            log.info("ActionGenerator: Holding TARGET with blocked goal, generating MoveTo staging area", {
                                currentlyHolding: currentHeldId,
                                stagingAreaId: STAGING_AREA_ID,
                                goalBlockadesRemaining: goalBlockadesRemaining,
                                reason: "conflict_resolution_path"
                            });
                            actions.push(createMoveToAction(state, 'goal', STAGING_AREA_ID));
                        } else {
                            // Goal is clear! Don't generate MoveTo staging - let PA-BT find
                            // the atGoal_1 condition expansion which generates MoveTo_goal_1
                            log.info("ActionGenerator: Holding TARGET with CLEAR goal path, NOT generating staging MoveTo", {
                                currentlyHolding: currentHeldId,
                                goalBlockadesRemaining: goalBlockadesRemaining,
                                reason: "goal_path_clear_deliver_directly"
                            });
                        }
                    } else {
                        // Holding a blockade - can deposit at dumpster
                        log.info("ActionGenerator: Holding blockade, generating MoveTo dumpster", {
                            currentlyHolding: currentHeldId,
                            dumpsterId: DUMPSTER_ID
                        });
                        actions.push(createMoveToAction(state, 'goal', DUMPSTER_ID));
                    }
                }
            }

            return actions;
        });

        // =========================================================================
        // STATIC ACTIONS (Not parametric - finite set)
        // =========================================================================

        // Helper to register actions
        const reg = function (name, conds, effects, tickFn) {
            const conditions = conds.map(function (c) {
                return {
                    key: c.k,
                    Match: function (v) {
                        return c.v === undefined ? v === true : v === c.v;
                    }
                };
            });
            const effectList = effects.map(function (e) {
                return {key: e.k, Value: e.v};
            });

            const wrappedTickFn = function () {
                const a = actor();
                if (state.gameMode !== 'automatic') {
                    return bt.running;
                }
                log.info("PA-BT action executing", {
                    action: name,
                    tick: state.tickCount,
                    actorX: Math.round(a.x),
                    actorY: Math.round(a.y),
                    held: a.heldItem ? a.heldItem.id : -1
                });
                const result = tickFn();
                const resultName = result === bt.success ? 'SUCCESS' : (result === bt.failure ? 'FAILURE' : 'RUNNING');
                log.info("PA-BT action result", {
                    action: name,
                    result: resultName,
                    tick: state.tickCount
                });
                return result;
            };

            const node = bt.createLeafNode(wrappedTickFn);
            state.pabtState.RegisterAction(name, pabt.newAction(name, conditions, effectList, node));
        };

        // ---------------------------------------------------------------------
        // Pick_Target: Pick up the target cube
        // Condition: atEntity_TARGET_ID, hands empty
        // Effect: holding target
        // NOTE: We REMOVED the reachable_goal_1 precondition because:
        // 1. It requires actions with effect reachable_goal_1=true
        // 2. No single action can truthfully make this true (requires 8 blockade clears)
        // 3. Heuristic effects break PA-BT's trust model
        //
        // The conflict must arise NATURALLY:
        //    1. Agent picks target
        //    2. Agent tries atGoal_1 → MoveTo fails (path blocked)
        //    3. PA-BT needs to clear path, but hands are FULL
        //    4. PA-BT finds Place_Target_Temporary to free hands
        //    5. Agent clears blockades, retrieves target, delivers
        // ---------------------------------------------------------------------
        // CRITICAL CONDITION ORDERING: heldItemExists=false FIRST to ensure
        // PA-BT searches for Place actions before MoveTo when hands are full.
        reg('Pick_Target',
            [
                {k: 'heldItemExists', v: false},
                {k: 'atEntity_' + TARGET_ID, v: true}
            ],
            [{k: 'heldItemId', v: TARGET_ID}, {k: 'heldItemExists', v: true}],
            function () {
                const a = actor();
                const t = state.cubes.get(TARGET_ID);
                if (!t || t.deleted) {
                    log.error("Pick_Target failed: target missing");
                    return bt.failure;
                }
                log.info("Picking up target cube", {id: TARGET_ID});
                t.deleted = true;
                a.heldItem = {id: TARGET_ID};
                // CRITICAL: Sync blackboard IMMEDIATELY so PA-BT post-condition check passes!
                // PA-BT runs on a separate Go goroutine and checks post-conditions
                // immediately after action returns SUCCESS. Without this sync,
                // the blackboard would still have heldItemId=-1.
                if (state.blackboard) {
                    state.blackboard.set('heldItemId', TARGET_ID);
                    state.blackboard.set('heldItemExists', true);
                }
                return bt.success;
            }
        );

        // ---------------------------------------------------------------------
        // Deliver_Target: Place target at goal
        // Condition: holding target, at goal
        // Effect: win condition
        // NOTE: NO goalPathCleared condition here! The conflict must arise NATURALLY:
        //    1. Agent picks target
        //    2. Agent tries atGoal_1 → FAILS (path blocked by goal wall)
        //    3. PA-BT tries MoveTo but can't reach due to blockades
        //    4. PA-BT needs to clear blockades, but hands are FULL (holding target)
        //    5. PA-BT generates Place_Target_Temporary to free hands
        //    6. Agent clears blockades
        //    7. Agent retrieves target and delivers
        // ---------------------------------------------------------------------
        reg('Deliver_Target',
            [
                {k: 'heldItemId', v: TARGET_ID},
                {k: 'atGoal_' + GOAL_ID, v: true}
            ],
            // CRITICAL FIX: Only include cubeDeliveredAtGoal=true as effect!
            // Previously included heldItemExists=false which caused PA-BT to consider
            // Deliver_Target when looking for actions to empty hands. This created an
            // infinite loop: heldItemExists=false → Deliver_Target → atGoal_1 →
            // goalBlockade_100_cleared → Deposit_GoalBlockade → heldItemId=100 →
            // Pick_GoalBlockade → heldItemExists=false → [LOOP]
            // By removing heldItemExists/heldItemId effects, PA-BT will now correctly
            // find Place_Target_Temporary for emptying hands.
            [{k: 'cubeDeliveredAtGoal', v: true}],
            function () {
                const a = actor();
                if (!a.heldItem || a.heldItem.id !== TARGET_ID) {
                    log.warn("Deliver_Target failed: not holding target");
                    return bt.failure;
                }
                // CRITICAL: Verify we're ACTUALLY at the delivery goal!
                // PA-BT may have been tricked by heuristic effects.
                const deliveryGoal = state.goals.get(GOAL_ID);
                if (!deliveryGoal) {
                    log.error("Deliver_Target failed: delivery goal not found");
                    return bt.failure;
                }
                const dx = deliveryGoal.x - a.x;
                const dy = deliveryGoal.y - a.y;
                const distToGoal = Math.sqrt(dx * dx + dy * dy);
                if (distToGoal > 2) {
                    // NOT at delivery goal - PA-BT was fooled by heuristic effects!
                    log.warn("Deliver_Target failed: not at delivery goal (dist=" + distToGoal.toFixed(1) + "), returning FAILURE to trigger replan", {
                        actorX: a.x, actorY: a.y,
                        goalX: deliveryGoal.x, goalY: deliveryGoal.y,
                        distance: distToGoal
                    });
                    return bt.failure;
                }
                log.info("Delivering target to goal");
                a.heldItem = null;
                state.targetDelivered = true;
                state.winConditionMet = true;
                // CRITICAL: Sync blackboard IMMEDIATELY so PA-BT post-condition check passes!
                if (state.blackboard) {
                    state.blackboard.set('cubeDeliveredAtGoal', true);
                    state.blackboard.set('heldItemExists', false);
                    state.blackboard.set('heldItemId', -1);
                }
                return bt.success;
            }
        );

        // ---------------------------------------------------------------------
        // Pick_Blockade_X: Pick up a blockade cube to clear path
        // This is one action per blockade cube, but that's OK because the
        // number of blockade cubes is SMALL and FIXED.
        // The KEY insight is that picking a blockade cube affects reachability
        // of the TARGET - this is encoded in the effects.
        // ---------------------------------------------------------------------
        BLOCKADE_IDS.forEach(function (id) {
            reg('Pick_Blockade_' + id,
                [{k: 'atEntity_' + id, v: true}, {k: 'heldItemExists', v: false}],
                [
                    {k: 'heldItemId', v: id},
                    {k: 'heldItemExists', v: true},
                    // CRITICAL HEURISTIC: Picking a blockade makes target reachable
                    // This tells PA-BT that clearing blockades helps reach the target
                    {k: 'reachable_cube_' + TARGET_ID, v: true}
                ],
                function () {
                    const a = actor();
                    const c = state.cubes.get(id);
                    if (!c || c.deleted) {
                        log.debug("Pick_Blockade skipped: cube gone", {cubeId: id});
                        return bt.success;
                    }
                    log.info("Picking up blockade cube", {cubeId: id});
                    c.deleted = true;
                    a.heldItem = {id: id};
                    // CRITICAL: Sync blackboard IMMEDIATELY so PA-BT post-condition check passes!
                    if (state.blackboard) {
                        state.blackboard.set('heldItemId', id);
                        state.blackboard.set('heldItemExists', true);
                    }
                    return bt.success;
                }
            );

            // Deposit blockade at dumpster
            reg('Deposit_Blockade_' + id,
                [{k: 'heldItemId', v: id}, {k: 'atGoal_' + DUMPSTER_ID, v: true}],
                [{k: 'heldItemExists', v: false}, {k: 'heldItemId', v: -1}],
                function () {
                    const a = actor();
                    log.info("Depositing blockade at dumpster", {cubeId: id});
                    a.heldItem = null;
                    // CRITICAL: Sync blackboard IMMEDIATELY so PA-BT post-condition check passes!
                    if (state.blackboard) {
                        state.blackboard.set('heldItemExists', false);
                        state.blackboard.set('heldItemId', -1);
                    }
                    return bt.success;
                }
            );
        });

        // ---------------------------------------------------------------------
        // Place_Target_Temporary: Place target at staging area to free hands
        // This enables conflict resolution - when goal is blocked and hands full
        // Condition: holding target, at staging area, AND goal blockades exist
        // Effect: hands empty (so we can pick up blockades)
        // NOTE: REMOVED reachable_goal_1=true heuristic - it's a lie that breaks PA-BT
        // CRITICAL FIX: Added needToPlaceTarget=true condition to prevent this action
        // from being selected after goal blockades are cleared (which caused infinite
        // pickup/drop loop when agent was near staging area).
        // ---------------------------------------------------------------------
        reg('Place_Target_Temporary',
            [
                {k: 'heldItemId', v: TARGET_ID},
                {k: 'atGoal_' + STAGING_AREA_ID, v: true},
                {k: 'needToPlaceTarget', v: true}  // Only when blockades still exist!
            ],
            [
                {k: 'heldItemExists', v: false},
                {k: 'heldItemId', v: -1}
                // REMOVED: {k: 'reachable_goal_' + GOAL_ID, v: true} - heuristic that breaks PA-BT
            ],
            function () {
                const a = actor();
                if (!a.heldItem || a.heldItem.id !== TARGET_ID) {
                    log.warn("Place_Target_Temporary failed: not holding target");
                    return bt.failure;
                }
                const stagingGoal = state.goals.get(STAGING_AREA_ID);
                log.info("CONFLICT_RESOLUTION: Placing target at staging area", {
                    x: stagingGoal.x,
                    y: stagingGoal.y,
                    reason: "freeing_hands_to_clear_goal_wall"
                });
                // Re-create target cube at staging location
                state.cubes.set(TARGET_ID, {
                    id: TARGET_ID,
                    x: stagingGoal.x,
                    y: stagingGoal.y,
                    deleted: false,
                    isTarget: true,
                    type: 'target'
                });
                a.heldItem = null;
                // CRITICAL: Sync blackboard IMMEDIATELY so PA-BT post-condition check passes!
                if (state.blackboard) {
                    state.blackboard.set('heldItemExists', false);
                    state.blackboard.set('heldItemId', -1);
                }
                return bt.success;
            }
        );

        // ---------------------------------------------------------------------
        // Goal Blockade Actions: Pick up and deposit goal wall cubes
        // These are the movable wall around the goal that requires clearing
        // NOTE: We REMOVED the reachable_goal_1=true heuristic effect because:
        // 1. It's a LIE - picking ONE blockade doesn't clear the path
        // 2. PA-BT trusts action effects - lying breaks the planning model
        // 3. The effect caused infinite loops when PA-BT tried to use it
        //
        // Instead, PA-BT should find these actions via a different mechanism:
        // - When reachable_goal_1 fails, we need a custom approach
        //
        // CRITICAL CONDITION ORDERING:
        // heldItemExists=false MUST come FIRST so PA-BT checks it before atEntity_X.
        // If atEntity_X comes first, PA-BT will try MoveTo when blocked instead of
        // searching for Place actions to empty hands. This causes infinite loops.
        // ---------------------------------------------------------------------
        GOAL_BLOCKADE_IDS.forEach(function (id) {
            reg('Pick_GoalBlockade_' + id,
                [{k: 'heldItemExists', v: false}, {k: 'atEntity_' + id, v: true}],
                [
                    {k: 'heldItemId', v: id},
                    {k: 'heldItemExists', v: true}
                    // REMOVED: {k: 'reachable_goal_' + GOAL_ID, v: true} - heuristic that breaks PA-BT
                ],
                function () {
                    const a = actor();
                    const c = state.cubes.get(id);
                    if (!c || c.deleted) {
                        log.debug("Pick_GoalBlockade skipped: cube gone", {cubeId: id});
                        return bt.success;
                    }
                    log.info("GOAL_WALL_CLEAR: Picking up goal blockade cube", {cubeId: id});
                    c.deleted = true;
                    a.heldItem = {id: id};
                    // CRITICAL: Sync blackboard IMMEDIATELY so PA-BT post-condition check passes!
                    if (state.blackboard) {
                        state.blackboard.set('heldItemId', id);
                        state.blackboard.set('heldItemExists', true);
                    }
                    return bt.success;
                }
            );

            // Deposit goal blockade at dumpster
            // TRUTHFUL EFFECT: Depositing a specific blockade marks it as cleared.
            // This is NOT a heuristic - depositing blockade X really does clear it permanently.
            // PA-BT will find this action when goalBlockade_X_cleared fails, and the effect
            // will be TRUE after execution.
            reg('Deposit_GoalBlockade_' + id,
                [{k: 'heldItemId', v: id}, {k: 'atGoal_' + DUMPSTER_ID, v: true}],
                [
                    {k: 'heldItemExists', v: false},
                    {k: 'heldItemId', v: -1},
                    // TRUTHFUL: This specific blockade is now cleared
                    {k: 'goalBlockade_' + id + '_cleared', v: true}
                ],
                function () {
                    const a = actor();
                    log.info("GOAL_WALL_CLEAR: Depositing goal blockade at dumpster", {cubeId: id});
                    a.heldItem = null;
                    // CRITICAL: Sync blackboard IMMEDIATELY so PA-BT post-condition check passes!
                    if (state.blackboard) {
                        state.blackboard.set('heldItemExists', false);
                        state.blackboard.set('heldItemId', -1);
                        state.blackboard.set('goalBlockade_' + id + '_cleared', true);
                    }
                    return bt.success;
                }
            );
        });
    }

    // ============================================================================
    // Rendering
    // ============================================================================

    function getDebugOverlayJSON(state) {
        const actor = state.actors.get(state.activeActorId);
        const cube1 = state.cubes.get(TARGET_ID);
        let tgtX = -1, tgtY = -1;
        if (cube1 && !cube1.deleted) {
            tgtX = Math.round(cube1.x);
            tgtY = Math.round(cube1.y);
        }

        let blkCount = 0;
        BLOCKADE_IDS.forEach(function (id) {
            const cube = state.cubes.get(id);
            if (cube && !cube.deleted) blkCount++;
        });

        let goalBlkCount = 0;
        GOAL_BLOCKADE_IDS.forEach(function (id) {
            const cube = state.cubes.get(id);
            if (cube && !cube.deleted) goalBlkCount++;
        });

        const held = actor.heldItem ? actor.heldItem.id : -1;
        const win = state.winConditionMet ? 1 : 0;

        // Debug: check dumpster reachability
        const dumpsterReachable = state.blackboard ? state.blackboard.get('reachable_goal_' + DUMPSTER_ID) : null;
        const goalReachable = state.blackboard ? state.blackboard.get('reachable_goal_' + GOAL_ID) : null;

        return JSON.stringify({
            m: state.gameMode === 'automatic' ? 'a' : 'm',
            t: state.tickCount,
            x: Math.round(actor.x),
            y: Math.round(actor.y),
            h: held,
            w: win,
            a: tgtX > -1 ? tgtX : undefined,
            b: tgtY > -1 ? tgtY : undefined,
            n: blkCount,
            g: goalBlkCount,  // Goal blockade count
            dr: dumpsterReachable ? 1 : 0,  // Dumpster reachable
            gr: goalReachable ? 1 : 0  // Goal reachable
        });
    }

    function getRenderBuffer(state, width, height) {
        if (state.renderBuffer === null || state.renderBufferWidth !== width || state.renderBufferHeight !== height) {
            state.renderBufferWidth = width;
            state.renderBufferHeight = height;
            state.renderBuffer = new Array(width * height);
            for (let i = 0; i < state.renderBuffer.length; i++) {
                state.renderBuffer[i] = ' ';
            }
        }
        return state.renderBuffer;
    }

    function bufferIndex(x, y, width) {
        return y * width + x;
    }

    function clearBuffer(buffer) {
        for (let i = 0; i < buffer.length; i++) {
            buffer[i] = ' ';
        }
    }

    function getAllSprites(state) {
        const sprites = [];

        state.actors.forEach(function (actor) {
            sprites.push({x: actor.x, y: actor.y, char: '@', width: 1, height: 1});
            if (actor.heldItem) {
                sprites.push({x: actor.x, y: actor.y - 0.5, char: '◆', width: 1, height: 1});
            }
        });

        state.cubes.forEach(function (cube) {
            if (!cube.deleted) {
                // DISTINCT SPRITES for different object types
                let spriteChar = '█'; // Default
                if (cube.type === 'target') {
                    spriteChar = '◇'; // Diamond for target cube
                } else if (cube.type === 'blockade') {
                    spriteChar = '▓'; // Dark shade for path blockades
                } else if (cube.type === 'goal_blockade') {
                    spriteChar = '▒'; // Medium shade for goal wall blockades
                } else if (cube.type === 'wall') {
                    spriteChar = '█'; // Solid for static walls
                }
                sprites.push({x: cube.x, y: cube.y, char: spriteChar, width: 1, height: 1});
            }
        });

        state.goals.forEach(function (goal) {
            // DISTINCT SPRITES for different goal types
            let goalChar = '○'; // Default
            if (goal.forTarget) {
                goalChar = '◎'; // Bullseye for target delivery goal
            } else if (goal.forBlockade) {
                goalChar = '⊙'; // Circled dot for dumpster
            } else if (goal.forStaging) {
                goalChar = '◌'; // Dotted circle for staging area
            }
            sprites.push({x: goal.x, y: goal.y, char: goalChar, width: 1, height: 1});
        });

        return sprites;
    }

    function renderPlayArea(state) {
        const width = state.width;
        const height = state.height;
        const buffer = getRenderBuffer(state, width, height);

        clearBuffer(buffer);

        const spaceX = Math.floor((width - state.spaceWidth) / 2);
        for (let y = 0; y < height; y++) {
            buffer[bufferIndex(spaceX, y, width)] = '│';
        }

        const sprites = getAllSprites(state);
        sprites.sort(function (a, b) {
            return a.y - b.y;
        });

        for (const sprite of sprites) {
            if (sprite.char === '') continue;
            const startX = Math.floor(sprite.x);
            const startY = Math.floor(sprite.y);
            for (let dy = 0; dy < sprite.height; dy++) {
                for (let dx = 0; dx < sprite.width; dx++) {
                    const x = startX + dx;
                    const y = startY + dy;
                    if (x >= 0 && x < width && y >= 0 && y < height) {
                        buffer[bufferIndex(x, y, width)] = sprite.char;
                    }
                }
            }
        }

        const hudX = state.spaceWidth + 2;
        let hudY = 2;

        function drawHudLine(text) {
            for (let i = 0; i < text.length && hudX + i < width; i++) {
                const idx = bufferIndex(hudX + i, hudY, width);
                if (idx < buffer.length) {
                    buffer[idx] = text[i];
                }
            }
            hudY++;
        }

        const actor = state.actors.get(state.activeActorId);
        drawHudLine('═════════════════════════');
        drawHudLine(' PICK-AND-PLACE SIM');
        drawHudLine('═════════════════════════');
        drawHudLine('');
        drawHudLine('Mode: ' + state.gameMode.toUpperCase());
        if (state.debugMode) drawHudLine('DEBUG: ON');
        drawHudLine('');
        drawHudLine('Actor Position:');
        drawHudLine('  X: ' + actor.x.toFixed(1));
        drawHudLine('  Y: ' + actor.y.toFixed(1));
        drawHudLine('  Holding: ' + (actor.heldItem ? 'Yes (' + actor.heldItem.id + ')' : 'No'));
        drawHudLine('');
        drawHudLine('Blockade: ' + BLOCKADE_IDS.filter(function (id) {
            const c = state.cubes.get(id);
            return c && !c.deleted;
        }).length + ' remaining');
        drawHudLine('Goal Wall: ' + GOAL_BLOCKADE_IDS.filter(function (id) {
            const c = state.cubes.get(id);
            return c && !c.deleted;
        }).length + ' remaining');
        drawHudLine('');

        if (state.winConditionMet) {
            drawHudLine('*** GOAL ACHIEVED! ***');
        } else {
            drawHudLine('Goal: Deliver target to ○');
        }

        drawHudLine('');
        drawHudLine('CONTROLS:');
        drawHudLine('  [M] Mode: Auto/Manual');
        drawHudLine('  [Q] Quit');
        drawHudLine('');
        drawHudLine('═════════════════════════');

        const rows = [];
        for (let y = 0; y < height; y++) {
            rows.push(buffer.slice(y * width, (y + 1) * width).join(''));
        }

        let output = rows.join('\n');

        if (state.debugMode) {
            // Compact output to avoid line wrapping/scrolling issues in tests
            output += '__place_debug_start__' + getDebugOverlayJSON(state) + '__place_debug_end__';
        }

        return output;
    }

    // ============================================================================
    // Bubbletea Model
    // ============================================================================

    function init() {
        const state = initializeSimulation();

        // DEBUG: Verify cube 100 exists immediately after init
        const cube100 = state.cubes.get(100);
        log.error("INIT_CUBE_100_CHECK", {
            cube100Exists: !!cube100,
            cube100: cube100 ? JSON.stringify(cube100) : "NOT_FOUND",
            allGoalBlockadeIds: JSON.stringify(GOAL_BLOCKADE_IDS),
            cubesSize: state.cubes.size
        });

        log.info("Pick-and-Place simulation initialized", {
            width: state.width,
            height: state.height,
            cubes: state.cubes.size,
            goals: state.goals.size,
            blockades: BLOCKADE_IDS.length,
            goalBlockades: GOAL_BLOCKADE_IDS.length
        });

        state.blackboard = new bt.Blackboard();
        state.pabtState = pabt.newState(state.blackboard);

        setupPABTActions(state);
        syncToBlackboard(state);

        // CRITICAL DEBUG: Log to stderr
        log.error("[INIT] reachable_goal_3=" + state.blackboard.get('reachable_goal_' + STAGING_AREA_ID) +
            " reachable_goal_1=" + state.blackboard.get('reachable_goal_' + GOAL_ID) +
            " reachable_goal_2=" + state.blackboard.get('reachable_goal_' + DUMPSTER_ID));

        log.info("Initial blackboard sync complete", {
            reachable_target: state.blackboard.get('reachable_cube_' + TARGET_ID),
            reachable_goal: state.blackboard.get('reachable_goal_' + GOAL_ID),
            reachable_dumpster: state.blackboard.get('reachable_goal_' + DUMPSTER_ID),
            reachable_staging: state.blackboard.get('reachable_goal_' + STAGING_AREA_ID),
            heldItemExists: state.blackboard.get('heldItemExists'),
            atEntity_target: state.blackboard.get('atEntity_' + TARGET_ID)
        });

        const goalConditions = [
            {
                key: 'cubeDeliveredAtGoal',
                Match: function (value) {
                    return value === true;
                }
            }
        ];
        state.pabtPlan = pabt.newPlan(state.pabtState, goalConditions);

        const rootNode = state.pabtPlan.Node();
        state.ticker = bt.newTicker(100, rootNode);

        return [state, tea.tick(16, 'tick')];
    }

    function update(state, msg) {
        if (msg.type === 'Tick' && msg.id === 'tick') {
            state.tickCount++;
            syncToBlackboard(state);

            if (state.tickCount % 20 === 0) {
                const actor = state.actors.get(state.activeActorId);
                log.info("Simulation telemetry", {
                    tick: state.tickCount,
                    mode: state.gameMode,
                    actorX: Math.round(actor.x),
                    actorY: Math.round(actor.y),
                    held: actor.heldItem ? actor.heldItem.id : -1,
                    blockadeRemaining: BLOCKADE_IDS.filter(function (id) {
                        const c = state.cubes.get(id);
                        return c && !c.deleted;
                    }).length,
                    goalBlockadeRemaining: GOAL_BLOCKADE_IDS.filter(function (id) {
                        const c = state.cubes.get(id);
                        return c && !c.deleted;
                    }).length,
                    win: state.winConditionMet
                });
            }

            return [state, tea.tick(16, 'tick')];
        }

        // ========================================================================
        // MANUAL MODE MOUSE CONTROL (PICK & PLACE)
        // ========================================================================
        if (msg.type === 'Mouse' && msg.event === 'press' && state.gameMode === 'manual') {
            const actor = state.actors.get(state.activeActorId);

            // Calculate simulation grid coordinates from mouse position
            // The play area is centered when width > spaceWidth
            const spaceX = Math.floor((state.width - state.spaceWidth) / 2);
            const clickX = msg.x - spaceX;
            const clickY = msg.y;

            // Ignore clicks outside the play area
            if (clickX >= 0 && clickX < state.spaceWidth && clickY >= 0 && clickY < state.height) {

                // Identify the best candidate cell adjacent to the actor
                // "Closest available or free cell" logic relative to the click
                let bestCand = null;
                let minClickDist = Infinity;

                // Scan 3x3 area around actor for adjacency
                for (let dy = -1; dy <= 1; dy++) {
                    for (let dx = -1; dx <= 1; dx++) {
                        // We check 8-way adjacency (including diagonals) similar to AI range checks
                        if (dx === 0 && dy === 0) continue;

                        const neighborX = Math.round(actor.x) + dx;
                        const neighborY = Math.round(actor.y) + dy;

                        // Bounds check
                        if (neighborX < 0 || neighborX >= state.spaceWidth || neighborY < 0 || neighborY >= state.height) continue;

                        // Calculate distance from this neighbor to the Mouse Click
                        const cdx = clickX - neighborX;
                        const cdy = clickY - neighborY;
                        const distToClick = Math.sqrt(cdx * cdx + cdy * cdy);

                        if (distToClick < minClickDist) {
                            // Check cell occupancy
                            let occupiedBy = null;
                            for (const [id, c] of state.cubes) {
                                if (!c.deleted && Math.round(c.x) === neighborX && Math.round(c.y) === neighborY) {
                                    occupiedBy = c;
                                    break;
                                }
                            }

                            if (actor.heldItem) {
                                // PLACING: Look for empty cell (or special goals)
                                if (!occupiedBy) {
                                    // Valid placement target
                                    bestCand = {action: 'place', x: neighborX, y: neighborY};
                                    minClickDist = distToClick;
                                }
                                // Note: Goals (dumpster/delivery) are not in 'cubes' map so they count as "not occupiedBy"
                                // which is correct. The placement logic below handles the specific goal effects.
                            } else {
                                // PICKING: Look for occupied cell (non-static)
                                if (occupiedBy && !occupiedBy.isStatic) {
                                    bestCand = {action: 'pick', x: neighborX, y: neighborY, target: occupiedBy};
                                    minClickDist = distToClick;
                                }
                            }
                        }
                    }
                }

                // Execute the resolved manual action
                if (bestCand) {
                    if (bestCand.action === 'pick') {
                        // --- MANUAL PICK ---
                        const target = bestCand.target;
                        log.info("MANUAL: Picking up item", {id: target.id});

                        target.deleted = true;
                        actor.heldItem = {id: target.id};

                        // Sync global state
                        if (state.blackboard) {
                            state.blackboard.set('heldItemId', target.id);
                            state.blackboard.set('heldItemExists', true);
                        }
                    } else if (bestCand.action === 'place') {
                        // --- MANUAL PLACE ---
                        const heldId = actor.heldItem.id;
                        const isDumpster = (bestCand.x === 8 && bestCand.y === 4); // DUMPSTER_ID coords
                        const isDelivery = (bestCand.x === 8 && bestCand.y === 18 && heldId === TARGET_ID); // GOAL_ID coords

                        if (isDelivery) {
                            // DELIVER TARGET
                            log.info("MANUAL: Delivering target to goal");
                            actor.heldItem = null;
                            state.targetDelivered = true;
                            state.winConditionMet = true;
                            if (state.blackboard) {
                                state.blackboard.set('cubeDeliveredAtGoal', true);
                                state.blackboard.set('heldItemExists', false);
                                state.blackboard.set('heldItemId', -1);
                            }
                        } else if (isDumpster) {
                            // DEPOSIT AT DUMPSTER (Destroy item)
                            log.info("MANUAL: Depositing item at dumpster", {id: heldId});
                            actor.heldItem = null;
                            // Update cleared flags if it was a goal blockade
                            if (GOAL_BLOCKADE_IDS.includes(heldId)) {
                                if (state.blackboard) state.blackboard.set('goalBlockade_' + heldId + '_cleared', true);
                            }
                            if (state.blackboard) {
                                state.blackboard.set('heldItemExists', false);
                                state.blackboard.set('heldItemId', -1);
                            }
                        } else {
                            // NORMAL PLACE (Restore to grid)
                            log.info("MANUAL: Placing item", {id: heldId, x: bestCand.x, y: bestCand.y});

                            // Retrieve original metadata to preserve type/flags
                            const original = state.cubes.get(heldId);
                            if (original) {
                                original.x = bestCand.x;
                                original.y = bestCand.y;
                                original.deleted = false;
                                actor.heldItem = null;
                                if (state.blackboard) {
                                    state.blackboard.set('heldItemExists', false);
                                    state.blackboard.set('heldItemId', -1);
                                }
                            }
                        }
                    }
                    syncToBlackboard(state);
                }
            }
        }

        if (msg.type === 'Key') {
            const key = msg.key;

            if (key === 'q' || key === 'Q') {
                return [state, tea.quit()];
            }

            if (key === 'm' || key === 'M') {
                const oldMode = state.gameMode;
                state.gameMode = state.gameMode === 'automatic' ? 'manual' : 'automatic';
                log.info("Mode toggled", {from: oldMode, to: state.gameMode});
                return [state, null];
            }

            if (key === '`') {
                state.debugMode = !state.debugMode;
                return [state, null];
            }

            if (state.gameMode === 'manual') {
                const actor = state.actors.get(state.activeActorId);
                const moveSpeed = 1.0;
                let nextX = actor.x;
                let nextY = actor.y;

                if (key === 'ArrowUp' || key === 'w' || key === 'W') {
                    nextY = Math.max(1, actor.y - moveSpeed);
                } else if (key === 'ArrowDown' || key === 's' || key === 'S') {
                    nextY = Math.min(state.height - 1, actor.y + moveSpeed);
                } else if (key === 'ArrowLeft' || key === 'a' || key === 'A') {
                    nextX = Math.max(1, actor.x - moveSpeed);
                } else if (key === 'ArrowRight' || key === 'd' || key === 'D') {
                    nextX = Math.min(state.spaceWidth - 1, actor.x + moveSpeed);
                }

                // ================================================================
                // STRICT COLLISION DETECTION
                // ================================================================
                let blocked = false;

                // Check if the target cell is occupied by any non-deleted cube
                for (const [id, cube] of state.cubes) {
                    if (!cube.deleted &&
                        Math.round(cube.x) === Math.round(nextX) &&
                        Math.round(cube.y) === Math.round(nextY)) {
                        blocked = true;
                        break;
                    }
                }

                if (!blocked) {
                    actor.x = nextX;
                    actor.y = nextY;
                    syncToBlackboard(state);
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
        return renderPlayArea(state);
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
    console.error('');
    console.error('========================================');
    console.error('PROGRAM INIT FAILED');
    console.error('========================================');
    printFatalError(e);
    console.error('');
    console.error('If this is a module loading error, ensure you are running:');
    console.error('  osm script scripts/example-05-pick-and-place.js');
    console.error('========================================');
    throw e;
}

// run the program
try {
    tea.run(program, {altScreen: true});
} catch (e) {
    console.error('');
    printFatalError(e);
    throw e;
}
