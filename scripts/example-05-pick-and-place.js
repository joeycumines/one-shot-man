#!/usr/bin/env osm script

// ============================================================================
// example-05-pick-and-place.js
// Pick-and-Place Simulator demonstrating osm:pabt PA-BT planning integration
// ============================================================================

// Top-level error handler - ensures ANY uncaught error triggers non-zero exit
try {
    // Load required modules with error handling
    var bt, tea, pabt;
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

// ============================================================================
// Simulation State
// ============================================================================

    // =========================================================================
    // BLOCKADE SCENARIO DESIGN
    // =========================================================================
    // The actor must navigate through a complex obstacle course:
    //
    // Layout (not to scale):
    //   
    //   GOAL2 (5,4)    [Drop zone for cleared cubes - top]
    //        
    //   ACTOR (8,12)   |  BLOCKADE WALL  |   TARGET (45,12)   GOAL1 (55,12)
    //        @         |  ████████████   |        ◆              ○
    //                  |  x=25, y=6-18   |
    //                  |  (7 cubes)      |
    //                               
    //   GOAL3 (5,20)   [Drop zone for cleared cubes - bottom]
    //
    // The blockade is a VERTICAL WALL of 7 cubes (IDs 2-8) at x=25.
    // The actor CANNOT reach the target cube without removing blockade cubes.
    // Each blockade cube must be picked up and deposited at goal 2 or 3.
    // Only after clearing a gap can the actor reach the target (cube 1).
    // Finally, the target must be delivered to goal 1.
    //
    // This demonstrates COMPLEX MULTI-STEP PLANNING:
    // 1. Planner recognizes target is unreachable (blockade intact)
    // 2. Planner generates sub-goals: clear blockade cubes
    // 3. For each blockade cube: move → pick → deposit at drop zone
    // 4. Once gap exists: move to target → pick → deliver to goal 1
    // =========================================================================

    // Blockade cube IDs: 2, 3, 4, 5, 6, 7, 8 (7 cubes total)
    const BLOCKADE_CUBE_IDS = [2, 3, 4, 5, 6, 7, 8];
    const BLOCKADE_X = 25;
    const BLOCKADE_Y_START = 6;
    const BLOCKADE_Y_SPACING = 2;

    function initializeSimulation() {
        // Create blockade cubes
        const cubesInit = [
            // Cube 1 (TARGET): Behind the blockade - the cube we must deliver
            [1, {id: 1, x: 45, y: 12, deleted: false, isTarget: true}]
        ];

        // Add blockade cubes (vertical wall at x=25)
        for (let i = 0; i < BLOCKADE_CUBE_IDS.length; i++) {
            const id = BLOCKADE_CUBE_IDS[i];
            const y = BLOCKADE_Y_START + (i * BLOCKADE_Y_SPACING);
            cubesInit.push([id, {id: id, x: BLOCKADE_X, y: y, deleted: false, isBlockade: true}]);
        }

        return {
            // Simulation bounds
            width: 80,
            height: 24,
            spaceWidth: 60,

            // All entities in JavaScript Maps
            actors: new Map([
                [1, {id: 1, x: 8, y: 12, heldItem: null}]
            ]),
            cubes: new Map(cubesInit),
            goals: new Map([
                // Goal 1: Final destination for TARGET cube (cube 1)
                [1, {id: 1, x: 55, y: 12, forTarget: true}],
                // Goal 2: Drop zone for blockade cubes (top)
                [2, {id: 2, x: 5, y: 4, forBlockade: true}],
                // Goal 3: Drop zone for blockade cubes (bottom)
                [3, {id: 3, x: 5, y: 20, forBlockade: true}]
            ]),

            // Track which drop zone to use next (alternates between goal 2 and 3)
            nextDropZone: 2,

            // PA-BT state
            blackboard: null,
            pabtPlan: null,
            ticker: null,

            activeActorId: 1,
            gameMode: 'automatic', // 'automatic' = PA-BT planning, 'manual' = keyboard controls
            tickCount: 0,
            lastTickTime: Date.now(),

            // Pre-allocated render buffer
            renderBuffer: null,
            renderBufferWidth: 0,
            renderBufferHeight: 0,

            // Win condition
            winConditionMet: false,

            // Debug mode - enabled by default for E2E test harness compatibility
            // Toggle with backtick (`) key
            debugMode: true
        };
    }

// ============================================================================
// Blackboard Synchronization
// ============================================================================

    function syncToBlackboard(state) {
        if (!state.blackboard) return;

        const bb = state.blackboard;
        const actor = state.actors.get(state.activeActorId);

        // Sync actor position
        bb.set('actorX', actor.x);
        bb.set('actorY', actor.y);
        bb.set('heldItemExists', actor.heldItem !== null);
        bb.set('heldItemId', actor.heldItem ? actor.heldItem.id : -1);

        // Sync cube state
        state.cubes.forEach((cube, id) => {
            bb.set('cube_' + id + '_deleted', cube.deleted);
            if (!cube.deleted) {
                bb.set('cube_' + id + '_x', cube.x);
                bb.set('cube_' + id + '_y', cube.y);

                // Calculate distance to actor
                const dx = cube.x - actor.x;
                const dy = cube.y - actor.y;
                const dist = Math.sqrt(dx * dx + dy * dy);
                bb.set('cube_' + id + '_distance', dist);
                bb.set('cube_' + id + '_inRange', dist < 2.0);
            } else {
                // Deleted cubes have infinite distance
                bb.set('cube_' + id + '_distance', Infinity);
                bb.set('cube_' + id + '_inRange', false);
            }
        });

        // =========================================================================
        // BLOCKADE STATE CALCULATION
        // =========================================================================
        // The blockade is BLOCKING if ANY blockade cube still exists at the wall
        // position (x=25, within the Y range that blocks horizontal movement).
        //
        // For the actor to reach the target (behind the wall), there must be a
        // GAP in the blockade at a Y position the actor can pass through.
        // =========================================================================

        // Check which blockade cubes are still in blocking position
        let blockadeCubesAtWall = 0;
        let closestGapY = null;
        const actorY = actor.y;

        // Track Y positions that are blocked
        const blockedYPositions = new Set();
        BLOCKADE_CUBE_IDS.forEach(id => {
            const cube = state.cubes.get(id);
            if (cube && !cube.deleted) {
                // Cube is "at wall" if it's near the blockade X position
                if (Math.abs(cube.x - BLOCKADE_X) < 3) {
                    blockadeCubesAtWall++;
                    // This Y band is blocked (cube blocks ±1 Y units around it)
                    for (let dy = -1; dy <= 1; dy++) {
                        blockedYPositions.add(Math.round(cube.y + dy));
                    }
                    // Track that this cube is at the wall (for planner conditions)
                    bb.set('cube_' + id + '_atWall', true);
                } else {
                    // Cube exists but is NOT at the wall (e.g., deposited at drop zone)
                    bb.set('cube_' + id + '_atWall', false);
                }
            } else {
                // Cube is deleted/held - not at wall
                bb.set('cube_' + id + '_atWall', false);
            }
        });

        // Check if there's a gap the actor can pass through
        // Actor needs a clear Y position near their current Y
        let canPassBlockade = false;
        for (let testY = Math.max(1, actorY - 3); testY <= Math.min(22, actorY + 3); testY++) {
            if (!blockedYPositions.has(testY)) {
                canPassBlockade = true;
                closestGapY = testY;
                break;
            }
        }

        // Also check if actor is already past the blockade
        if (actor.x > BLOCKADE_X + 2) {
            canPassBlockade = true;
        }

        bb.set('blockadeCubesAtWall', blockadeCubesAtWall);
        bb.set('blockadeCleared', blockadeCubesAtWall === 0);
        bb.set('canPassBlockade', canPassBlockade);
        bb.set('gapY', closestGapY);

        // Can reach target if blockade has a passable gap or is cleared
        const targetCube = state.cubes.get(1);
        const targetReachable = canPassBlockade && targetCube && !targetCube.deleted;
        bb.set('targetReachable', targetReachable);

        // Sync goal state
        state.goals.forEach((goal, id) => {
            bb.set('goal_' + id + '_x', goal.x);
            bb.set('goal_' + id + '_y', goal.y);

            // Calculate distance to actor
            const dx = goal.x - actor.x;
            const dy = goal.y - actor.y;
            const dist = Math.sqrt(dx * dx + dy * dy);
            bb.set('goal_' + id + '_distance', dist);
            bb.set('goal_' + id + '_inRange', dist < 2.0);
        });

        // Track if target cube has been delivered to goal 1
        bb.set('cubeDeliveredAtGoal', state.winConditionMet);
    }

// ============================================================================
// PA-BT Action Setup
// ============================================================================

    function setupPABTActions(state) {
        // =========================================================================
        // BLOCKADE CLEARING ACTIONS
        // =========================================================================
        // For each blockade cube (IDs 2-8), we need:
        // 1. Move to blockade cube N
        // 2. Pick up blockade cube N
        // 3. Deposit at drop zone (goal 2 or 3)
        //
        // The planner will select the appropriate cube to clear based on
        // which one is closest or most accessible.
        // =========================================================================

        // Create actions for each blockade cube
        BLOCKADE_CUBE_IDS.forEach(function(cubeId) {
            const cubeKey = 'cube_' + cubeId;

            // Action: Move to blockade cube N
            // CRITICAL: Only targets cubes ACTUALLY AT THE WALL (not deposited at drop zones)
            const moveToBlockadeCubeAction = pabt.newAction(
                'moveToBlockade_' + cubeId,
                [
                    {
                        key: cubeKey + '_deleted',
                        Match: function (value) {
                            return value === false; // Cube exists
                        }
                    },
                    {
                        // *** FIX: Only target cubes AT THE BLOCKADE WALL ***
                        // Cubes deposited at drop zones have atWall=false
                        key: cubeKey + '_atWall',
                        Match: function (value) {
                            return value === true; // Must be at the blockade wall
                        }
                    },
                    {
                        key: 'heldItemExists',
                        Match: function (value) {
                            return value === false; // Not holding anything
                        }
                    }
                ],
                [ // Effects
                    {key: cubeKey + '_inRange', Value: true}
                ],
                bt.createLeafNode(function () {
                    const actor = state.actors.get(state.activeActorId);
                    const cube = state.cubes.get(cubeId);

                    if (!cube || cube.deleted) return bt.failure;

                    const dx = cube.x - actor.x;
                    const dy = cube.y - actor.y;
                    const dist = Math.sqrt(dx * dx + dy * dy);

                    if (dist > 1.8) {
                        actor.x += Math.sign(dx) * Math.min(1.5, Math.abs(dx));
                        actor.y += Math.sign(dy) * Math.min(1.5, Math.abs(dy));
                        return bt.running;
                    }

                    return bt.success;
                })
            );

            // Action: Pick up blockade cube N
            // CRITICAL: Only pick cubes AT THE WALL (prevents re-picking deposited cubes)
            const pickBlockadeCubeAction = pabt.newAction(
                'pickBlockade_' + cubeId,
                [
                    {
                        key: cubeKey + '_deleted',
                        Match: function (value) {
                            return value === false; // Cube exists
                        }
                    },
                    {
                        // *** FIX: Only target cubes AT THE BLOCKADE WALL ***
                        key: cubeKey + '_atWall',
                        Match: function (value) {
                            return value === true; // Must be at the blockade wall
                        }
                    },
                    {
                        key: 'heldItemExists',
                        Match: function (value) {
                            return value === false; // Not holding anything
                        }
                    },
                    {
                        key: cubeKey + '_inRange',
                        Match: function (value) {
                            return value === true; // In range
                        }
                    }
                ],
                [ // Effects: Cube removed from blockade, actor holding it
                    {key: cubeKey + '_deleted', Value: true},
                    {key: cubeKey + '_atWall', Value: false}, // No longer at wall
                    {key: 'heldItemExists', Value: true},
                    {key: 'heldItemId', Value: cubeId},
                    // Picking up a blockade cube MAY clear the blockade
                    {key: 'canPassBlockade', Value: true}
                ],
                bt.createLeafNode(function () {
                    const actor = state.actors.get(state.activeActorId);
                    const cube = state.cubes.get(cubeId);

                    if (!cube || cube.deleted || !actor) return bt.failure;

                    cube.deleted = true;
                    actor.heldItem = {id: cubeId, originalX: cube.x, originalY: cube.y};

                    return bt.success;
                })
            );

            // Register blockade cube actions
            state.pabtState.RegisterAction('moveToBlockade_' + cubeId, moveToBlockadeCubeAction);
            state.pabtState.RegisterAction('pickBlockade_' + cubeId, pickBlockadeCubeAction);
        });

        // =========================================================================
        // DROP ZONE ACTIONS
        // =========================================================================
        // When holding a blockade cube, deposit it at a drop zone (goal 2 or 3)
        // =========================================================================

        // Action: Deposit blockade cube at drop zone (goal 2)
        const depositAtDropZone2Action = pabt.newAction(
            'depositAtDropZone2',
            [
                {
                    key: 'heldItemExists',
                    Match: function (value) {
                        return value === true; // Must be holding something
                    }
                },
                {
                    // Must NOT be holding the target cube (cube 1)
                    key: 'heldItemId',
                    Match: function (value) {
                        return value !== 1; // Not holding target cube
                    }
                }
            ],
            [ // Effects: Hands free, blockade cube deposited
                {key: 'heldItemExists', Value: false},
                {key: 'heldItemId', Value: -1}
            ],
            bt.createLeafNode(function () {
                const actor = state.actors.get(state.activeActorId);
                const dropZone = state.goals.get(2);

                if (!dropZone || !actor || !actor.heldItem) return bt.failure;
                if (actor.heldItem.id === 1) return bt.failure; // Don't deposit target cube here

                const dx = dropZone.x - actor.x;
                const dy = dropZone.y - actor.y;
                const dist = Math.sqrt(dx * dx + dy * dy);

                if (dist > 1.8) {
                    actor.x += Math.sign(dx) * Math.min(1.5, Math.abs(dx));
                    actor.y += Math.sign(dy) * Math.min(1.5, Math.abs(dy));
                    return bt.running;
                }

                // Drop the cube at drop zone
                const heldId = actor.heldItem.id;
                const cube = state.cubes.get(heldId);
                if (cube) {
                    cube.deleted = false;
                    cube.x = dropZone.x + (Math.random() * 2 - 1); // Slight offset
                    cube.y = dropZone.y + (Math.random() * 2 - 1);
                }
                actor.heldItem = null;
                return bt.success;
            })
        );

        // Action: Deposit blockade cube at drop zone (goal 3)
        const depositAtDropZone3Action = pabt.newAction(
            'depositAtDropZone3',
            [
                {
                    key: 'heldItemExists',
                    Match: function (value) {
                        return value === true; // Must be holding something
                    }
                },
                {
                    // Must NOT be holding the target cube (cube 1)
                    key: 'heldItemId',
                    Match: function (value) {
                        return value !== 1; // Not holding target cube
                    }
                }
            ],
            [ // Effects: Hands free, blockade cube deposited
                {key: 'heldItemExists', Value: false},
                {key: 'heldItemId', Value: -1}
            ],
            bt.createLeafNode(function () {
                const actor = state.actors.get(state.activeActorId);
                const dropZone = state.goals.get(3);

                if (!dropZone || !actor || !actor.heldItem) return bt.failure;
                if (actor.heldItem.id === 1) return bt.failure; // Don't deposit target cube here

                const dx = dropZone.x - actor.x;
                const dy = dropZone.y - actor.y;
                const dist = Math.sqrt(dx * dx + dy * dy);

                if (dist > 1.8) {
                    actor.x += Math.sign(dx) * Math.min(1.5, Math.abs(dx));
                    actor.y += Math.sign(dy) * Math.min(1.5, Math.abs(dy));
                    return bt.running;
                }

                // Drop the cube at drop zone
                const heldId = actor.heldItem.id;
                const cube = state.cubes.get(heldId);
                if (cube) {
                    cube.deleted = false;
                    cube.x = dropZone.x + (Math.random() * 2 - 1); // Slight offset
                    cube.y = dropZone.y + (Math.random() * 2 - 1);
                }
                actor.heldItem = null;
                return bt.success;
            })
        );

        state.pabtState.RegisterAction('depositAtDropZone2', depositAtDropZone2Action);
        state.pabtState.RegisterAction('depositAtDropZone3', depositAtDropZone3Action);

        // =========================================================================
        // TARGET CUBE ACTIONS (main objective)
        // =========================================================================
        // These actions are for the TARGET cube (cube 1), which must be
        // delivered to goal 1 to win.
        // CRITICAL: Target can only be reached when blockade is passable.
        // =========================================================================

        // Action: Move to target cube (cube 1)
        const moveToTargetAction = pabt.newAction(
            'moveToTarget',
            [
                {
                    key: 'cube_1_deleted',
                    Match: function (value) {
                        return value === false; // Target exists
                    }
                },
                {
                    key: 'heldItemExists',
                    Match: function (value) {
                        return value === false; // Not holding anything
                    }
                },
                {
                    // *** CRITICAL BLOCKADE CONDITION ***
                    key: 'canPassBlockade',
                    Match: function (value) {
                        return value === true; // Must have path through blockade
                    }
                }
            ],
            [ // Effects
                {key: 'cube_1_inRange', Value: true},
                {key: 'targetReachable', Value: true}
            ],
            bt.createLeafNode(function () {
                const actor = state.actors.get(state.activeActorId);
                const target = state.cubes.get(1);

                if (!target || target.deleted) return bt.failure;

                const dx = target.x - actor.x;
                const dy = target.y - actor.y;
                const dist = Math.sqrt(dx * dx + dy * dy);

                if (dist > 1.8) {
                    actor.x += Math.sign(dx) * Math.min(1.5, Math.abs(dx));
                    actor.y += Math.sign(dy) * Math.min(1.5, Math.abs(dy));
                    return bt.running;
                }

                return bt.success;
            })
        );

        // Action: Pick up target cube (cube 1)
        const pickTargetAction = pabt.newAction(
            'pickTarget',
            [
                {
                    key: 'cube_1_deleted',
                    Match: function (value) {
                        return value === false; // Target exists
                    }
                },
                {
                    key: 'heldItemExists',
                    Match: function (value) {
                        return value === false; // Not holding anything
                    }
                },
                {
                    key: 'cube_1_inRange',
                    Match: function (value) {
                        return value === true; // In range
                    }
                }
            ],
            [ // Effects
                {key: 'cube_1_deleted', Value: true},
                {key: 'heldItemExists', Value: true},
                {key: 'heldItemId', Value: 1}
            ],
            bt.createLeafNode(function () {
                const actor = state.actors.get(state.activeActorId);
                const target = state.cubes.get(1);

                if (!target || target.deleted || !actor) return bt.failure;

                target.deleted = true;
                actor.heldItem = {id: 1, originalX: target.x, originalY: target.y};

                return bt.success;
            })
        );

        // Action: Deliver target to goal 1 (WIN!)
        const deliverToGoalAction = pabt.newAction(
            'deliverToGoal',
            [
                {
                    key: 'heldItemExists',
                    Match: function (value) {
                        return value === true; // Must be holding something
                    }
                },
                {
                    key: 'heldItemId',
                    Match: function (value) {
                        return value === 1; // Must be holding target cube
                    }
                }
            ],
            [ // Effects
                {key: 'cubeDeliveredAtGoal', Value: true} // WIN!
            ],
            bt.createLeafNode(function () {
                const actor = state.actors.get(state.activeActorId);
                const goal = state.goals.get(1);

                if (!goal || !actor || !actor.heldItem) return bt.failure;
                if (actor.heldItem.id !== 1) return bt.failure; // Must be target cube

                const dx = goal.x - actor.x;
                const dy = goal.y - actor.y;
                const dist = Math.sqrt(dx * dx + dy * dy);

                if (dist > 1.8) {
                    actor.x += Math.sign(dx) * Math.min(1.5, Math.abs(dx));
                    actor.y += Math.sign(dy) * Math.min(1.5, Math.abs(dy));
                    return bt.running;
                }

                // At goal - drop target cube and WIN!
                actor.heldItem = null;
                state.winConditionMet = true;
                return bt.success;
            })
        );

        // Register target cube actions
        state.pabtState.RegisterAction('moveToTarget', moveToTargetAction);
        state.pabtState.RegisterAction('pickTarget', pickTargetAction);
        state.pabtState.RegisterAction('deliverToGoal', deliverToGoalAction);
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

        // Count blockade cubes still at wall position
        let blkCount = 0;
        BLOCKADE_CUBE_IDS.forEach(function(id) {
            const cube = state.cubes.get(id);
            if (cube && !cube.deleted && Math.abs(cube.x - BLOCKADE_X) < 3) {
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
            n: blkCount // Number of blockade cubes still at wall
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
        const actor = state.actors.get(state.activeActorId);
        const goal = state.goals.get(1);

        if (!actor || !goal) {
            state.winConditionMet = false;
            return;
        }

        // Check if actor has delivered a cube to goal
        // Win condition: actor at goal and not holding anything
        const dx = goal.x - actor.x;
        const dy = goal.y - actor.y;
        const dist = Math.sqrt(dx * dx + dy * dy);
        const droppedCube = !actor.heldItem && dist < 1.5;

        state.winConditionMet = droppedCube;
    }

// ============================================================================
// Bubbletea Model
// ============================================================================

    function init() {
        const state = initializeSimulation();

        // Initialize blackboard
        state.blackboard = new bt.Blackboard();

        // Create PA-BT State wrapper for blackboard
        state.pabtState = pabt.newState(state.blackboard);

        // Setup PA-BT actions and register them
        setupPABTActions(state);

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

            // Sync read-only data to blackboard for PA-BT planner
            syncToBlackboard(state);

            // Check win condition periodically
            if (state.tickCount % 10 === 0) {
                checkWinCondition(state);
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
                state.gameMode = state.gameMode === 'automatic' ? 'manual' : 'automatic';
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
