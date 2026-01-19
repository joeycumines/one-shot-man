#!/usr/bin/env osm script

// ============================================================================
// example-05-pick-and-place.js
// Pick-and-Place Simulator demonstrating osm:pabt PA-BT planning integration
// ============================================================================

// Top-level error handler - ensures ANY uncaught error triggers non-zero exit
try {
    // Load required modules with error handling
    var bt, tea, pabt, os;
    try {
        bt = require('osm:bt');
    } catch (e) {
        console.error('Error: Failed to load osm:bt module. Make sure you are running with "osm script"');
        throw e;
    }
    try {
        tea = require('osm:bubbletea');
    } catch (e) {
        console.error('Error: Failed to load osm:bubbletea module. Make sure you are running with "osm script"');
        throw e;
    }
    try {
        pabt = require('osm:pabt');
    } catch (e) {
        console.error('Error: Failed to load osm:pabt module. Make sure you are running with "osm script"');
        throw e;
    }
    try {
        os = require('osm:os');
    } catch (e) {
        console.error('Error: Failed to load osm:os module. Make sure you are running with "osm script"');
        throw e;
    }

    // ============================================================================
    // Simulation State
    // ============================================================================

    // =========================================================================
    // SCENARIO CONFIGURATION
    // =========================================================================

    // Environment constants
    const ENV_WIDTH = 90;
    const ENV_HEIGHT = 24;

    // Target location (center-ish)
    const TARGET_ID = 1;
    const TARGET_X = 40;
    const TARGET_Y = 12;

    // Inner Ring (Box around target) - MOVABLE OBSTACLES (Blockade)
    // Needs to be cleared.
    const INNER_RING_RADIUS_X = 4;
    const INNER_RING_RADIUS_Y = 3;
    const INNER_RING_IDS = [];

    // Outer Ring (Static Wall) - STATIC OBSTACLES
    // Has two gaps: entry on left, exit on right
    const OUTER_RING_MIN_X = 15;
    const OUTER_RING_MAX_X = 50;
    const OUTER_RING_MIN_Y = 5;
    const OUTER_RING_MAX_Y = 20;
    const GAP_LEFT_X = 15;  // Entry point on left wall
    const GAP_RIGHT_X = 50; // Exit point on right wall
    const GAP_Y = 12;       // Same y-level for both gaps
    // Static wall IDs start at 1000

    // Goal locations
    const GOAL_FINAL_ID = 1; // Delivery point for target
    const GOAL_DROP_ID = 2;  // Dumpster for cleared obstacles

    // Patrol cubes
    const PATROL_IDS = [101, 102];

    function initializeSimulation() {
        // Collect all entities
        const cubesInit = [];

        // 1. Target Cube
        cubesInit.push([TARGET_ID, {
            id: TARGET_ID,
            x: TARGET_X,
            y: TARGET_Y,
            deleted: false,
            isTarget: true,
            type: 'target'
        }]);

        // 2. Inner Ring (Movable Blockade)
        // Create a tight box around target
        let nextId = 2;
        for (let x = TARGET_X - INNER_RING_RADIUS_X; x <= TARGET_X + INNER_RING_RADIUS_X; x += 2) {
            for (let y = TARGET_Y - INNER_RING_RADIUS_Y; y <= TARGET_Y + INNER_RING_RADIUS_Y; y += 2) {
                // Skip center (where target is)
                if (Math.abs(x - TARGET_X) < 2 && Math.abs(y - TARGET_Y) < 2) continue;

                cubesInit.push([nextId, {
                    id: nextId,
                    x: x,
                    y: y,
                    deleted: false,
                    isBlockade: true,
                    type: 'blockade'
                }]);
                INNER_RING_IDS.push(nextId);
                nextId++;
            }
        }

        // 3. Patrol Cubes (Dynamic)
        // Moving vertically at x=30 and x=50 inside the outer ring
        cubesInit.push([101, {
            id: 101,
            x: 30,
            y: 8,
            deleted: false,
            isPatrol: true,
            vy: 0.5,
            minY: 6,
            maxY: 18,
            type: 'patrol'
        }]);
        cubesInit.push([102, {
            id: 102,
            x: 45,  // Moved from x=50 to avoid blocking the right gap exit
            y: 16,
            deleted: false,
            isPatrol: true,
            vy: -0.5,
            minY: 6,
            maxY: 18,
            type: 'patrol'
        }]);

        // 4. Outer Ring (Static Walls)
        // We represent static walls as special "cube" entities with big IDs
        // or just handle them in collision logic?
        // Let's add them as static entities to be visualized and collided with
        let staticId = 1000;

        function addStatic(x, y) {
            // Leave gap on LEFT wall (entry)
            if (Math.abs(x - GAP_LEFT_X) < 2 && Math.abs(y - GAP_Y) < 2) return;
            // Leave gap on RIGHT wall (exit)
            if (Math.abs(x - GAP_RIGHT_X) < 2 && Math.abs(y - GAP_Y) < 2) return;
            cubesInit.push([staticId, {
                id: staticId,
                x: x,
                y: y,
                deleted: false,
                isStatic: true,
                type: 'wall'
            }]);
            staticId++;
        }

        // Top/Bottom walls - extend from x=1 to close off left side and prevent going around
        for (let x = 1; x <= OUTER_RING_MAX_X; x += 1) {
            addStatic(x, OUTER_RING_MIN_Y);
            addStatic(x, OUTER_RING_MAX_Y);
        }
        // Left/Right walls
        for (let y = OUTER_RING_MIN_Y; y <= OUTER_RING_MAX_Y; y += 1) {
            addStatic(OUTER_RING_MIN_X, y);
            addStatic(OUTER_RING_MAX_X, y);
        }

        return {
            // Simulation bounds
            width: ENV_WIDTH,
            height: ENV_HEIGHT,
            spaceWidth: 60,

            // Entities
            actors: new Map([
                [1, {id: 1, x: 5, y: 12, heldItem: null}] // Start on left, outside
            ]),
            cubes: new Map(cubesInit),
            goals: new Map([
                [GOAL_FINAL_ID, {id: GOAL_FINAL_ID, x: 60, y: 12, forTarget: true}], // Moved to x=60 (safe distance from x=50 wall)
                [GOAL_DROP_ID, {id: GOAL_DROP_ID, x: 5, y: 20, forBlockade: true}] // Dumpster
            ]),

            // State
            blackboard: null,
            pabtPlan: null,
            activeActorId: 1,
            gameMode: 'automatic',
            tickCount: 0,
            lastTickTime: Date.now(),

            // Win state
            winConditionMet: false,
            targetDelivered: false,

            // Rendering
            renderBuffer: null,
            renderBufferWidth: 0,
            renderBufferHeight: 0,

            debugMode: os.getenv('OSM_TEST_MODE') === '1'
        };
    }

    // ============================================================================
    // Blackboard Synchronization
    // ============================================================================

    // Simple BFS to check reachability and distance
    function getPathInfo(state, startX, startY, targetX, targetY, ignoreCubeId = -1) {
        const width = state.width;
        const height = state.height;
        const visited = new Set();
        const queue = [{x: startX, y: startY, dist: 0}];
        const key = (x, y) => x + ',' + y;

        visited.add(key(startX, startY));

        // Create collision map for this frame
        const blocked = new Set();

        // Static walls
        state.cubes.forEach(c => {
            if (c.deleted) return;
            if (c.id === ignoreCubeId) return; // Ignore specific cube (e.g. the one we want to pick)
            if (c.id === state.actors.get(state.activeActorId).heldItem?.id) return; // Ignore held item

            // Cubes occupy their integer position
            blocked.add(key(Math.round(c.x), Math.round(c.y)));
            // Patrols might sweep, but for now treat as instantaneous obstacles
        });

        // Add Outer Ring boundaries (explicit coordinates if not in cubes list, but we added them as cubes)

        while (queue.length > 0) {
            const current = queue.shift();

            // Check success (allow being adjacent for "reachability" to pick)
            const dx = Math.abs(current.x - targetX);
            const dy = Math.abs(current.y - targetY);
            if (dx <= 1.5 && dy <= 1.5) {
                return {reachable: true, distance: current.dist};
            }

            // Neighbors
            const dirs = [[0, 1], [0, -1], [1, 0], [-1, 0]];
            for (const [ox, oy] of dirs) {
                const nx = current.x + ox;
                const ny = current.y + oy;
                const nKey = key(nx, ny);

                if (nx < 1 || nx >= width - 1 || ny < 1 || ny >= height - 1) continue;
                if (visited.has(nKey)) continue;
                if (blocked.has(nKey)) continue;

                visited.add(nKey);
                queue.push({x: nx, y: ny, dist: current.dist + 1});
            }
        }

        return {reachable: false, distance: Infinity};
    }

    function syncToBlackboard(state) {
        if (!state.blackboard) return;

        const bb = state.blackboard;
        const actor = state.actors.get(state.activeActorId);
        const ax = Math.round(actor.x);
        const ay = Math.round(actor.y);

        // Sync standard actor state
        bb.set('actorX', actor.x);
        bb.set('actorY', actor.y);
        bb.set('heldItemExists', actor.heldItem !== null);
        bb.set('heldItemId', actor.heldItem ? actor.heldItem.id : -1);

        // Create SHARED collision map for this frame to optimize BFS
        const key = (x, y) => x + ',' + y;
        const globalBlocked = new Set();
        state.cubes.forEach(c => {
            if (c.deleted) return;
            if (c.id === actor.heldItem?.id) return;
            globalBlocked.add(key(Math.round(c.x), Math.round(c.y)));
        });

        // Helper for reachability that uses the shared blocked set
        const checkReachable = (tx, ty, ignoreId = -1) => {
            return getPathInfoWithBlocked(state, ax, ay, tx, ty, globalBlocked, ignoreId);
        };

        // 1. Target
        const target = state.cubes.get(TARGET_ID);
        if (target && !target.deleted) {
            const info = checkReachable(Math.round(target.x), Math.round(target.y), TARGET_ID);
            bb.set('pathClear_' + TARGET_ID, info.reachable);
            bb.set('distance_' + TARGET_ID, info.distance);
            bb.set('inRange_' + TARGET_ID, info.distance < 2.5);
        } else {
            bb.set('pathClear_' + TARGET_ID, true);
            bb.set('inRange_' + TARGET_ID, false);
        }

        // 2. Inner Ring Cubes (Blockade)
        INNER_RING_IDS.forEach(id => {
            const cube = state.cubes.get(id);
            if (cube && !cube.deleted) {
                const info = checkReachable(Math.round(cube.x), Math.round(cube.y), id);
                bb.set('pathClear_' + id, info.reachable);
                bb.set('distance_' + id, info.distance);
                bb.set('inRange_' + id, info.distance < 2.5);
            } else {
                bb.set('pathClear_' + id, true);
                bb.set('inRange_' + id, false);
            }
        });

        // 3. Goals
        state.goals.forEach(goal => {
            const info = checkReachable(Math.round(goal.x), Math.round(goal.y));
            bb.set('pathClear_Goal_' + goal.id, info.reachable);
            bb.set('distance_Goal_' + goal.id, info.distance);
            bb.set('inRange_Goal_' + goal.id, info.distance < 2.5);
        });

        // 4. Win condition state
        bb.set('cubeDeliveredAtGoal', state.winConditionMet);
    }

    // Optimized BFS that accepts a pre-populated blocked set
    function getPathInfoWithBlocked(state, startX, startY, targetX, targetY, blockedSet, ignoreId = -1) {
        const width = state.width;
        const height = state.height;
        const visited = new Set();
        const queue = [{x: startX, y: startY, dist: 0}];
        const key = (x, y) => x + ',' + y;

        visited.add(key(startX, startY));

        let currentBlocked = blockedSet;
        let ignoreKey = null;
        if (ignoreId !== -1) {
            const c = state.cubes.get(ignoreId);
            if (c) ignoreKey = key(Math.round(c.x), Math.round(c.y));
        }

        while (queue.length > 0) {
            const current = queue.shift();

            const dx = Math.abs(current.x - targetX);
            const dy = Math.abs(current.y - targetY);
            if (dx <= 1.5 && dy <= 1.5) {
                return {reachable: true, distance: current.dist};
            }

            const dirs = [[0, 1], [0, -1], [1, 0], [-1, 0]];
            for (const [ox, oy] of dirs) {
                const nx = current.x + ox;
                const ny = current.y + oy;
                const nKey = key(nx, ny);

                if (nx < 1 || nx >= width - 1 || ny < 1 || ny >= height - 1) continue;
                if (visited.has(nKey)) continue;
                if (currentBlocked.has(nKey) && nKey !== ignoreKey) continue;

                visited.add(nKey);
                queue.push({x: nx, y: ny, dist: current.dist + 1});
            }
        }

        return {reachable: false, distance: Infinity};
    }


    // ============================================================================
    // PA-BT Action Setup
    // ============================================================================

    function findNextStep(state, startX, startY, targetX, targetY, ignoreCubeId = -1) {
        // Integerize check
        const iStartX = Math.round(startX);
        const iStartY = Math.round(startY);
        const iTargetX = Math.round(targetX);
        const iTargetY = Math.round(targetY);
        const width = state.width;
        const height = state.height;

        // Trivial case: already there-ish
        if (Math.abs(startX - targetX) < 1.0 && Math.abs(startY - targetY) < 1.0) {
            return {x: targetX, y: targetY};
        }

        const visited = new Set();
        const queue = [];
        const key = (x, y) => x + ',' + y;

        // Identify obstacles
        const blocked = new Set();
        state.cubes.forEach(c => {
            if (c.deleted) return;
            if (c.id === ignoreCubeId) return;
            if (c.id === state.actors.get(state.activeActorId).heldItem?.id) return;
            blocked.add(key(Math.round(c.x), Math.round(c.y)));
        });

        // BFS
        visited.add(key(iStartX, iStartY));
        const dirs = [[0, 1], [0, -1], [1, 0], [-1, 0]]; // 4-way movement

        // Initial neighbors
        for (const [ox, oy] of dirs) {
            const nx = iStartX + ox;
            const ny = iStartY + oy;
            const nKey = key(nx, ny);

            if (nx < 1 || nx >= width - 1 || ny < 1 || ny >= height - 1) continue;
            if (blocked.has(nKey)) continue;

            // If this neighbor IS the target, go there
            if (nx === iTargetX && ny === iTargetY) {
                return {x: nx, y: ny};
            }

            queue.push({x: nx, y: ny, firstX: ox, firstY: oy});
            visited.add(nKey);
        }

        while (queue.length > 0) {
            const cur = queue.shift();

            // Reached target?
            if (cur.x === iTargetX && cur.y === iTargetY) {
                // Return step in direction of first move
                return {x: startX + cur.firstX, y: startY + cur.firstY};
            }

            for (const [ox, oy] of dirs) {
                const nx = cur.x + ox;
                const ny = cur.y + oy;
                const nKey = key(nx, ny);

                if (nx < 1 || nx >= width - 1 || ny < 1 || ny >= height - 1) continue;
                if (blocked.has(nKey)) continue;
                if (visited.has(nKey)) continue;

                visited.add(nKey);
                queue.push({x: nx, y: ny, firstX: cur.firstX, firstY: cur.firstY});
            }
        }

        // Fallback: if no path, try direct (or stay put)
        return null;
    }

    function setupPABTActions(state) {
        // Helper to register simple actions
        const reg = (name, conds, effects, tickFn) => {
            // Convert simple condition objects to Match functions
            const conditions = conds.map(c => ({
                key: c.k,
                Match: v => c.v === undefined ? v === true : v === c.v
            }));
            const effectList = effects.map(e => ({key: e.k, Value: e.v}));

            // Wrap tickFn to pause in manual mode AND log every execution
            const wrappedTickFn = () => {
                const a = state.actors.get(state.activeActorId);
                if (state.gameMode !== 'automatic') {
                    // In manual mode, PA-BT is paused - return running to hold state
                    log.debug("PA-BT paused (manual mode)", {action: name, tick: state.tickCount});
                    return bt.running;
                }
                // Log action start
                log.info("PA-BT action executing", {
                    action: name,
                    tick: state.tickCount,
                    actorX: Math.round(a.x),
                    actorY: Math.round(a.y),
                    held: a.heldItem ? a.heldItem.id : -1
                });
                const result = tickFn();
                // Log action result
                const resultName = result === bt.success ? 'SUCCESS' : (result === bt.failure ? 'FAILURE' : 'RUNNING');
                log.info("PA-BT action result", {
                    action: name,
                    result: resultName,
                    tick: state.tickCount,
                    actorX: Math.round(a.x),
                    actorY: Math.round(a.y),
                    held: a.heldItem ? a.heldItem.id : -1
                });
                return result;
            };

            const node = bt.createLeafNode(wrappedTickFn);
            state.pabtState.RegisterAction(name, pabt.newAction(name, conditions, effectList, node));
        };

        const actor = () => state.actors.get(state.activeActorId);

        // ---------------------------------------------------------------------
        // 1. MoveTo(Target)
        // ---------------------------------------------------------------------
        // Condition: pathClear_1
        // Effect: inRange_1
        reg('MoveTo_Target',
            [{k: 'pathClear_' + TARGET_ID, v: true}],
            [{k: 'inRange_' + TARGET_ID, v: true}],
            () => {
                const a = actor();
                const t = state.cubes.get(TARGET_ID);
                if (!t || t.deleted) {
                    log.debug("MoveTo_Target skipped: target already gone");
                    return bt.success;
                }

                const dx = t.x - a.x;
                const dy = t.y - a.y;
                const dist = Math.sqrt(dx * dx + dy * dy);
                if (dist <= 1.8) {
                    log.debug("MoveTo_Target success: in range", {dist: dist});
                    return bt.success;
                }

                log.debug("Executing MoveTo action", {target: "TargetCube", dist: dist});
                const nextStep = findNextStep(state, a.x, a.y, t.x, t.y, TARGET_ID);
                if (nextStep) {
                    const stepDx = nextStep.x - a.x;
                    const stepDy = nextStep.y - a.y;
                    a.x += Math.sign(stepDx) * Math.min(1.0, Math.abs(stepDx));
                    a.y += Math.sign(stepDy) * Math.min(1.0, Math.abs(stepDy));
                } else {
                    log.warn("MoveTo_Target stuck: no path", {agentX: a.x, agentY: a.y});
                }
                return bt.running;
            }
        );

        // ---------------------------------------------------------------------
        // 2. Pick(Target)
        // ---------------------------------------------------------------------
        // Condition: inRange_1, handsEmpty
        // Effect: holding_1, !handsEmpty
        reg('Pick_Target',
            [{k: 'inRange_' + TARGET_ID, v: true}, {k: 'heldItemExists', v: false}],
            [{k: 'heldItemId', v: TARGET_ID}, {k: 'heldItemExists', v: true}],
            () => {
                const a = actor();
                const t = state.cubes.get(TARGET_ID);
                if (!t || t.deleted) {
                    log.error("Pick_Target failed: target missing/deleted");
                    return bt.failure;
                }

                log.info("Picking up target cube", {id: TARGET_ID});
                t.deleted = true;
                a.heldItem = {id: TARGET_ID};
                return bt.success;
            }
        );

        // ---------------------------------------------------------------------
        // 3. MoveTo(Goal)
        // ---------------------------------------------------------------------
        reg('MoveTo_Goal',
            [{k: 'pathClear_Goal_' + GOAL_FINAL_ID, v: true}],
            [{k: 'inRange_Goal_' + GOAL_FINAL_ID, v: true}],
            () => {
                const a = actor();
                const g = state.goals.get(GOAL_FINAL_ID);

                const dx = g.x - a.x;
                const dy = g.y - a.y;
                const dist = Math.sqrt(dx * dx + dy * dy);
                if (dist <= 1.8) {
                    log.debug("MoveTo_Goal success: in range", {dist: dist});
                    return bt.success;
                }

                log.debug("Executing MoveTo action", {target: "Goal", dist: dist});
                const nextStep = findNextStep(state, a.x, a.y, g.x, g.y);
                if (nextStep) {
                    const stepDx = nextStep.x - a.x;
                    const stepDy = nextStep.y - a.y;
                    a.x += Math.sign(stepDx) * Math.min(1.0, Math.abs(stepDx));
                    a.y += Math.sign(stepDy) * Math.min(1.0, Math.abs(stepDy));
                } else {
                    log.warn("MoveTo_Goal stuck: no path", {agentX: a.x, agentY: a.y});
                    // Fallback to dumb move if no path (e.g. goal is blocked?)
                    a.x += Math.sign(dx) * Math.min(1.0, Math.abs(dx));
                    a.y += Math.sign(dy) * Math.min(1.0, Math.abs(dy));
                }
                return bt.running;
            }
        );

        // ---------------------------------------------------------------------
        // 4. Deliver(Target) -> Place at Goal
        // ---------------------------------------------------------------------
        // CRITICAL: heldItemId must be checked BEFORE inRange_Goal to ensure
        // PA-BT backward chaining picks up the target FIRST before going to goal.
        // If inRange_Goal is first, robot goes to goal empty-handed!
        reg('Deliver_Target',
            [{k: 'heldItemId', v: TARGET_ID}, {k: 'inRange_Goal_' + GOAL_FINAL_ID, v: true}],
            [
                {k: 'cubeDeliveredAtGoal', v: true},
                {k: 'heldItemExists', v: false},
                {k: 'heldItemId', v: -1}
            ],
            () => {
                const a = actor();
                if (!a.heldItem || a.heldItem.id !== TARGET_ID) {
                    log.warn("Deliver_Target failed: incorrect item held", {
                        expected: TARGET_ID,
                        held: a.heldItem ? a.heldItem.id : "none"
                    });
                    return bt.failure;
                }
                log.info("Delivering target to goal", {
                    targetId: TARGET_ID,
                    goalId: GOAL_FINAL_ID
                });
                a.heldItem = null;
                state.targetDelivered = true;
                state.winConditionMet = true; // Set win condition!
                return bt.success;
            }
        );

        // =====================================================================
        // BLOCKADE CLEARING LOGIC
        // =====================================================================
        // For each blockade cube:
        // MoveTo(Cube) -> Pick(Cube) -> MoveTo(Drop) -> Place(Drop)
        // CRITICAL: Pick(Cube) has the side effect of making pathClear_Target TRUE
        // This is the heuristic we give the planner.
        // =====================================================================

        INNER_RING_IDS.forEach(id => {
            const cubeKey = id;

            // MoveTo(BlockadeCube)
            reg('MoveTo_' + id,
                [{k: 'pathClear_' + id, v: true}],
                [{k: 'inRange_' + id, v: true}],
                () => {
                    const a = actor();
                    const c = state.cubes.get(id);
                    if (!c || c.deleted) {
                        log.debug("MoveTo skipped: cube already gone", {cubeId: id});
                        return bt.success;
                    }

                    const dx = c.x - a.x;
                    const dy = c.y - a.y;
                    const dist = Math.sqrt(dx * dx + dy * dy);
                    if (dist <= 1.8) {
                        log.debug("MoveTo success: in range of cube", {cubeId: id, dist: dist});
                        return bt.success;
                    }

                    const nextStep = findNextStep(state, a.x, a.y, c.x, c.y, id);
                    if (nextStep) {
                        const stepDx = nextStep.x - a.x;
                        const stepDy = nextStep.y - a.y;
                        a.x += Math.sign(stepDx) * Math.min(1.0, Math.abs(stepDx));
                        a.y += Math.sign(stepDy) * Math.min(1.0, Math.abs(stepDy));
                        // log.debug("Moving towards cube", {cubeId: id, agentX: a.x, agentY: a.y});
                    } else {
                        log.warn("MoveTo stuck: no path to cube", {cubeId: id, agentX: a.x, agentY: a.y});
                    }
                    return bt.running;
                }
            );

            // Pick(BlockadeCube)
            // Side effect: pathClear_Target = true (Heuristic)
            reg('Pick_' + id,
                [{k: 'inRange_' + id, v: true}, {k: 'heldItemExists', v: false}],
                [
                    {k: 'heldItemId', v: id},
                    {k: 'heldItemExists', v: true},
                    {k: 'pathClear_' + TARGET_ID, v: true} // THE MAGIC BIT
                ],
                () => {
                    const a = actor();
                    const c = state.cubes.get(id);
                    if (!c || c.deleted) {
                        log.warn("Pick failed: cube missing/deleted", {cubeId: id});
                        return bt.failure;
                    }

                    log.info("Picking up blockade cube", {cubeId: id});
                    c.deleted = true;
                    a.heldItem = {id: id};
                    return bt.success;
                }
            );

            // Deposit(BlockadeCube) -> Drops at Goal 2
            reg('Deposit_' + id,
                [{k: 'heldItemId', v: id}], // Requires holding this specific cube
                [{k: 'heldItemExists', v: false}, {k: 'heldItemId', v: -1}],
                () => {
                    const a = actor();
                    const drop = state.goals.get(GOAL_DROP_ID);

                    // Move to drop zone if not there
                    const dx = drop.x - a.x;
                    const dy = drop.y - a.y;
                    const dist = Math.sqrt(dx * dx + dy * dy);
                    if (dist > 2.0) {
                        const nextStep = findNextStep(state, a.x, a.y, drop.x, drop.y);
                        if (nextStep) {
                            const stepDx = nextStep.x - a.x;
                            const stepDy = nextStep.y - a.y;
                            a.x += Math.sign(stepDx) * Math.min(1.0, Math.abs(stepDx));
                            a.y += Math.sign(stepDy) * Math.min(1.0, Math.abs(stepDy));
                        }
                        return bt.running;
                    }

                    // Drop it
                    const c = state.cubes.get(id);
                    if (c) {
                        log.info("Dropping blockade cube at dumpster", {cubeId: id});
                        c.deleted = false;
                        c.x = drop.x + (Math.random() * 2 - 1);
                        c.y = drop.y + (Math.random() * 2 - 1);
                        // Make sure it doesn't block again immediately
                    }
                    a.heldItem = null;
                    return bt.success;
                }
            );
        });
    }

    // ============================================================================
    // Render Buffer
    // ============================================================================

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

    function clearBuffer(buffer, width, height) {
        for (let i = 0; i < buffer.length; i++) {
            buffer[i] = ' ';
        }
    }

    // ============================================================================
    // Rendering
    // ============================================================================

    // Generate ultra-compact JSON for E2E test harness scraping
    // NOTE: Keep this VERY SHORT to avoid terminal line-wrapping truncation!
    // Terminal is typically 80 chars wide. JSON must be < 80 chars.
    function getDebugOverlayJSON(state) {
        const actor = state.actors.get(state.activeActorId);

        // Target cube (cube 1) position
        const cube1 = state.cubes.get(1);
        let tgtX = -1, tgtY = -1;
        if (cube1 && !cube1.deleted) {
            tgtX = Math.round(cube1.x);
            tgtY = Math.round(cube1.y);
        }

        // Count blockade cubes (inner ring) still active
        let blkCount = 0;
        INNER_RING_IDS.forEach(function (id) {
            const cube = state.cubes.get(id);
            if (cube && !cube.deleted) {
                blkCount++;
            }
        });

        // Held cube ID
        const held = actor.heldItem ? actor.heldItem.id : -1;

        // Win condition (as 0/1 for compactness)
        const win = state.winConditionMet ? 1 : 0;

        // ULTRA-compact: Use single-char keys
        // Keys: m=mode, t=tick, x/y=actor pos, h=held, w=win, a/b=target pos, n=blockade count
        return JSON.stringify({
            m: state.gameMode === 'automatic' ? 'a' : 'm',
            t: state.tickCount,
            x: Math.round(actor.x),
            y: Math.round(actor.y),
            h: held,
            w: win,
            a: tgtX > -1 ? tgtX : undefined,
            b: tgtY > -1 ? tgtY : undefined,
            n: blkCount // Number of active inner ring cubes
        });
    }

    function getAllSprites(state) {
        const sprites = [];

        // Render actors
        state.actors.forEach(actor => {
            sprites.push({
                x: actor.x,
                y: actor.y,
                char: actor.id === state.activeActorId ? '@' : '+',
                width: 1,
                height: 1
            });
        });

        // Render held item above actor
        state.actors.forEach(actor => {
            if (actor.heldItem) {
                sprites.push({
                    x: actor.x,
                    y: actor.y - 0.5,
                    char: '■',
                    width: 1,
                    height: 1
                });
            }
        });

        // Render cubes
        state.cubes.forEach(cube => {
            if (!cube.deleted) {
                sprites.push({
                    x: cube.x,
                    y: cube.y,
                    char: '█',
                    width: 1,
                    height: 1
                });
            }
        });

        // Render goals
        state.goals.forEach(goal => {
            sprites.push({
                x: goal.x,
                y: goal.y,
                char: '○',
                width: 1,
                height: 1
            });
        });

        return sprites;
    }

    function renderPlayArea(state) {
        const width = state.width;
        const height = state.height;
        const buffer = getRenderBuffer(state, width, height);

        // Clear buffer
        clearBuffer(buffer, width, height);

        // Draw separator line
        const spaceX = Math.floor((width - state.spaceWidth) / 2);
        for (let y = 0; y < height; y++) {
            buffer[bufferIndex(spaceX, y, width)] = '│';
        }

        // Get and draw all sprites
        const sprites = getAllSprites(state);

        // Y-sort sprites (rendering order)
        sprites.sort((a, b) => a.y - b.y);

        // Draw sprites (with clipping)
        for (const sprite of sprites) {
            if (sprite.char === '') continue;

            const startX = Math.floor(sprite.x);
            const startY = Math.floor(sprite.y);

            for (let dy = 0; dy < sprite.height; dy++) {
                for (let dx = 0; dx < sprite.width; dx++) {
                    const x = startX + dx;
                    const y = startY + dy;

                    // Clip to bounds
                    if (x >= 0 && x < width && y >= 0 && y < height) {
                        buffer[bufferIndex(x, y, width)] = sprite.char;
                    }
                }
            }
        }

        // Draw HUD on right side
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
        if (state.debugMode) {
            drawHudLine('DEBUG: ON');
        }
        drawHudLine('');
        drawHudLine('Actor Position:');
        drawHudLine('  X: ' + actor.x.toFixed(1));
        drawHudLine('  Y: ' + actor.y.toFixed(1));
        drawHudLine('  Holding: ' + (actor.heldItem ? 'Yes' : 'No'));
        drawHudLine('');
        drawHudLine('Cubes: ' + state.cubes.size);
        drawHudLine('Goals: ' + state.goals.size);
        drawHudLine('');

        if (state.winConditionMet) {
            drawHudLine('*** GOAL ACHIEVED! ***');
        } else {
            drawHudLine('Goal: Move cube to ○');
        }

        drawHudLine('');
        drawHudLine('CONTROLS:');
        drawHudLine('  [M] Mode: Auto/Manual');
        drawHudLine('  [X] Move cube (test)');
        drawHudLine('  [Q] Quit');
        if (state.gameMode === 'manual') {
            drawHudLine('  [WASD/Arrows] Move');
            drawHudLine('  [1-2] Pick cube');
            drawHudLine('  [R] Drop item');
        }
        drawHudLine('');
        drawHudLine('═════════════════════════');

        // Convert buffer to string
        const rows = [];
        for (let y = 0; y < height; y++) {
            rows.push(buffer.slice(y * width, (y + 1) * width).join(''));
        }

        let output = rows.join('\n');

        // Append debug JSON if debug mode is enabled (for E2E test harness)
        if (state.debugMode) {
            output += '\n__place_debug_start__\n' + getDebugOverlayJSON(state) + '\n__place_debug_end__';
        }

        return output;
    }

    // ============================================================================
    // Update Logic
    // ============================================================================

    function checkWinCondition(state) {
        // CRITICAL: Win condition is MONOTONIC - once true, stays true forever
        // This prevents the infinite loop where win is set then unset when actor moves
        if (state.winConditionMet) {
            return; // Already won, nothing more to check
        }

        const actor = state.actors.get(state.activeActorId);
        const goal = state.goals.get(1);

        if (!actor || !goal) {
            return; // Don't set to false - just return
        }

        // Win condition requires:
        // 1. Target cube (cube 1) was delivered (targetDelivered flag set by deliverToGoal)
        // 2. Actor is at or near goal 1
        // 3. Actor is not holding anything (dropped the cube)
        if (!state.targetDelivered) {
            return; // Target not yet delivered - can't win
        }

        const dx = goal.x - actor.x;
        const dy = goal.y - actor.y;
        const dist = Math.sqrt(dx * dx + dy * dy);

        if (dist < 2.5 && !actor.heldItem) {
            state.winConditionMet = true; // WIN! This is permanent.
        }
    }

    // ============================================================================
    // Bubbletea Model
    // ============================================================================

    function init() {
        const state = initializeSimulation();

        // Log startup
        if (typeof log !== 'undefined' && log.info) {
            log.info("Pick-and-Place simulation initialized", {
                width: state.width,
                height: state.height,
                cubes: state.cubes.size,
                goals: state.goals.size
            });
        }

        // Initialize blackboard
        state.blackboard = new bt.Blackboard();

        // Create PA-BT State wrapper for blackboard
        state.pabtState = pabt.newState(state.blackboard);

        // Setup PA-BT actions and register them
        setupPABTActions(state);

        // CRITICAL: Sync state to blackboard BEFORE creating plan
        // Otherwise, all pathClear_X conditions are undefined and planning fails!
        syncToBlackboard(state);
        log.info("Initial blackboard sync complete", {
            pathClear_Target: state.blackboard.get('pathClear_' + TARGET_ID),
            pathClear_Goal: state.blackboard.get('pathClear_Goal_' + GOAL_FINAL_ID),
            actorPos: [state.actors.get(state.activeActorId).x, state.actors.get(state.activeActorId).y]
        });

        // Create PA-BT plan with PABT State (not raw blackboard)
        const goalConditions = [
            {
                key: 'cubeDeliveredAtGoal',
                Match: function (value) {
                    return value === true;
                }
            }
        ];
        state.pabtPlan = pabt.newPlan(state.pabtState, goalConditions);

        // Create background BT ticker (PA-BT planner runs at 100ms)
        // NOTE: Plan.Node() with uppercase N
        const rootNode = state.pabtPlan.Node();
        state.ticker = bt.newTicker(100, rootNode);

        return [state, tea.tick(16, 'tick')];
    }

    function update(state, msg) {
        if (msg.type === 'Tick' && msg.id === 'tick') {
            state.tickCount++;

            // Update Patrols (Dynamic Obstacles)
            state.cubes.forEach(cube => {
                if (cube.isPatrol && !cube.deleted) {
                    cube.y += cube.vy;
                    // Bounce logic
                    if (cube.y <= cube.minY || cube.y >= cube.maxY) {
                        cube.vy *= -1;
                        cube.y = Math.max(cube.minY, Math.min(cube.maxY, cube.y));
                    }
                }
            });

            // Sync read-only data to blackboard for PA-BT planner
            syncToBlackboard(state);

            // Check win condition periodically
            if (state.tickCount % 10 === 0) {
                checkWinCondition(state);
            }

            // Periodic telemetry (every 20 ticks = ~2 seconds)
            if (state.tickCount % 20 === 0) {
                const actor = state.actors.get(state.activeActorId);
                const bb = state.blackboard;

                // Count active blockade cubes
                let activeBlockade = 0;
                INNER_RING_IDS.forEach(id => {
                    const cube = state.cubes.get(id);
                    if (cube && !cube.deleted) activeBlockade++;
                });

                // Get key blackboard values
                const pathClear1 = bb.get('pathClear_' + TARGET_ID);
                const inRange1 = bb.get('inRange_' + TARGET_ID);
                const dist1 = bb.get('distance_' + TARGET_ID);

                log.info("Simulation telemetry", {
                    tick: state.tickCount,
                    mode: state.gameMode,
                    actorX: Math.round(actor.x),
                    actorY: Math.round(actor.y),
                    held: actor.heldItem ? actor.heldItem.id : -1,
                    activeBlockade: activeBlockade,
                    pathClearToTarget: pathClear1,
                    inRangeOfTarget: inRange1,
                    distanceToTarget: dist1,
                    win: state.winConditionMet
                });
            }

            return [state, tea.tick(16, 'tick')];
        }

        // Keyboard controls
        if (msg.type === 'Key') {
            const key = msg.key;

            // Quit
            if (key === 'q' || key === 'Q') {
                return [state, tea.quit()];
            }

            // Toggle mode
            if (key === 'm' || key === 'M') {
                const oldMode = state.gameMode;
                state.gameMode = state.gameMode === 'automatic' ? 'manual' : 'automatic';
                log.info("Mode toggled", {
                    from: oldMode,
                    to: state.gameMode,
                    tick: state.tickCount
                });
                return [state, tea.tick(16, 'tick')];
            }

            // Toggle debug mode
            if (key === '`') {
                state.debugMode = !state.debugMode;
                return [state, tea.tick(16, 'tick')];
            }

            // Move cube 1 to a new random position (for testing unexpected circumstances / replanning)
            // This works in BOTH automatic and manual mode
            if (key === 'x' || key === 'X') {
                const cube = state.cubes.get(1);
                if (cube && !cube.deleted) {
                    // Move cube to opposite side of play area
                    cube.x = cube.x < 28 ? 45 : 15;
                    cube.y = cube.y < 12 ? 18 : 6;
                    // Force blackboard sync to trigger replanning
                    syncToBlackboard(state);
                }
                return [state, tea.tick(16, 'tick')];
            }

            // Manual controls
            if (state.gameMode === 'manual') {
                const actor = state.actors.get(state.activeActorId);
                const moveSpeed = 1.0;

                // Movement
                if (key === 'ArrowUp' || key === 'w' || key === 'W') {
                    actor.y = Math.max(1, actor.y - moveSpeed);
                } else if (key === 'ArrowDown' || key === 's' || key === 'S') {
                    actor.y = Math.min(state.height - 1, actor.y + moveSpeed);
                } else if (key === 'ArrowLeft' || key === 'a' || key === 'A') {
                    actor.x = Math.max(1, actor.x - moveSpeed);
                } else if (key === 'ArrowRight' || key === 'd' || key === 'D') {
                    actor.x = Math.min(state.spaceWidth - 1, actor.x + moveSpeed);
                }

                // Pick cube
                if ((key === '1' || key === '2') && !actor.heldItem) {
                    const cubeId = parseInt(key, 10);
                    const cube = state.cubes.get(cubeId);

                    if (cube && !cube.deleted) {
                        const dx = cube.x - actor.x;
                        const dy = cube.y - actor.y;
                        const dist = Math.sqrt(dx * dx + dy * dy);

                        if (dist < 2.0) {
                            cube.deleted = true;
                            actor.heldItem = {id: cubeId, originalX: cube.x, originalY: cube.y};
                            checkWinCondition(state);
                        }
                    }
                }

                // Drop item
                if ((key === 'r' || key === 'R') && actor.heldItem) {
                    actor.heldItem = null;
                    checkWinCondition(state);
                }

                return [state, tea.tick(16, 'tick')];
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

    // ============================================================================
    // Entry Point
    // ============================================================================

    const program = tea.newModel({
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

    // Wrap execution with try/catch for error handling
    try {
        console.log('');
        console.log('═════════════════════════════════════════');
        console.log('  PICK-AND-PLACE SIMULATOR');
        console.log('  Demonstrating PA-BT Planning');
        console.log('═════════════════════════════════════════');
        console.log('');
        console.log('The actor (@) will automatically plan and');
        console.log('execute actions to move a cube (█) to');
        console.log('the goal location (○).');
        console.log('');
        console.log('CONTROLS:');
        console.log('  [M] Mode: Auto/Manual');
        console.log('  [`] Toggle debug JSON overlay');
        console.log('  [Q] Quit');
        console.log('');
        console.log('Press any key to start...');
        console.log('');

        // Run the program
        tea.run(program, {altScreen: true});

        console.log('');
        console.log('Program exited successfully.');
    } catch (e) {
        console.error('');
        console.error('FATAL ERROR: ' + e.message);
        console.error('Stack trace: ' + e.stack);
        throw e;
    }

} catch (e) {
    // Top-level error handler - ensures non-zero exit code
    console.error('');
    console.error('========================================');
    console.error('PROGRAM STARTUP FAILED');
    console.error('========================================');
    console.error('Error: ' + e.message);
    if (e.stack) {
        console.error('Stack: ' + e.stack);
    }
    console.error('');
    console.error('If this is a module loading error, ensure you are running:');
    console.error('  osm script scripts/example-05-pick-and-place.js');
    console.error('========================================');

    // Throw to trigger Go's panic recovery (non-zero exit)
    throw e;
}
