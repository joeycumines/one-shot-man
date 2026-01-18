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

function initializeSimulation() {
    return {
        // Simulation bounds
        width: 80,
        height: 24,
        spaceWidth: 56,

        // All entities in JavaScript Maps
        actors: new Map([
            [1, {id: 1, x: 10, y: 12, heldItem: null}]
        ]),
        cubes: new Map([
            [1, {id: 1, x: 25, y: 10, deleted: false}],
            [2, {id: 2, x: 25, y: 15, deleted: false}]
        ]),
        goals: new Map([
            [1, {id: 1, x: 55, y: 12}]
        ]),

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

    // Sync cube state
    state.cubes.forEach((cube, id) => {
        bb.set('cube_' + id + '_deleted', cube.deleted);
        if (!cube.deleted) {
            bb.set('cube_' + id + '_x', cube.x);
            bb.set('cube_' + id + '_y', cube.y);

            // Calculate distance to actor
            const dx = cube.x - actor.x;
            const dy = cube.y - actor.y;
            const dist = Math.sqrt(dx*dx + dy*dy);
            bb.set('cube_' + id + '_distance', dist);
            bb.set('cube_' + id + '_inRange', dist < 1.5);
        }
    });

    // Sync goal state
    state.goals.forEach((goal, id) => {
        bb.set('goal_' + id + '_x', goal.x);
        bb.set('goal_' + id + '_y', goal.y);

        // Calculate distance to actor
        const dx = goal.x - actor.x;
        const dy = goal.y - actor.y;
        const dist = Math.sqrt(dx*dx + dy*dy);
        bb.set('goal_' + id + '_distance', dist);
    });

    // Track if any cube has been delivered to goal
    bb.set('cubeDeliveredAtGoal', state.winConditionMet);
}

// ============================================================================
// PA-BT Action Setup
// ============================================================================

function setupPABTActions(state) {
    const bb = state.blackboard;

    // Action 1: Move to cube 1
    const moveToCube1Action = pabt.newAction(
        'moveToCube1',
        [
            {
                key: 'cube_1_deleted',
                Match: function(value) {
                    return value === false; // Cube exists
                }
            },
            {
                key: 'heldItemExists',
                Match: function(value) {
                    return value === false; // Not holding anything
                }
            }
        ],
        [ // Effects
            {key: 'cube_1_inRange', Value: true} // Will be in range after moving
        ],
        bt.createLeafNode(function() {
            const actor = state.actors.get(state.activeActorId);
            const cube = state.cubes.get(1);

            if (!cube || cube.deleted) return bt.failure;

            // Move towards cube
            const dx = cube.x - actor.x;
            const dy = cube.y - actor.y;
            const dist = Math.sqrt(dx*dx + dy*dy);

            if (dist > 0.1) {
                actor.x += Math.sign(dx) * Math.min(1, dist);
                actor.y += Math.sign(dy) * Math.min(1, dist);
            }

            return bt.running;
        })
    );

    // Action 2: Pick up cube 1
    const pickCube1Action = pabt.newAction(
        'pickCube1',
        [
            {
                key: 'cube_1_deleted',
                Match: function(value) {
                    return value === false; // Cube exists
                }
            },
            {
                key: 'heldItemExists',
                Match: function(value) {
                    return value === false; // Not holding anything
                }
            },
            {
                key: 'cube_1_inRange',
                Match: function(value) {
                    return value === true; // In range
                }
            }
        ],
        [ // Effects
            {key: 'cube_1_deleted', Value: true}, // Cube deleted
            {key: 'heldItemExists', Value: true}  // Now holding something
        ],
        bt.createLeafNode(function() {
            const actor = state.actors.get(state.activeActorId);
            const cube = state.cubes.get(1);

            if (!cube || cube.deleted || !actor) return bt.failure;

            // Pick up cube
            cube.deleted = true;
            actor.heldItem = {id: 1, originalX: cube.x, originalY: cube.y};

            return bt.success;
        })
    );

    // Action 3: Move to goal 1
    const moveToGoal1Action = pabt.newAction(
        'moveToGoal1',
        [
            {
                key: 'heldItemExists',
                Match: function(value) {
                    return value === true; // Holding something
                }
            }
        ],
        [ // Effects
            {key: 'cubeDeliveredAtGoal', Value: true} // Goal achieved when at goal
        ],
        bt.createLeafNode(function() {
            const actor = state.actors.get(state.activeActorId);
            const goal = state.goals.get(1);

            if (!goal || !actor) return bt.failure;

            // Move towards goal
            const dx = goal.x - actor.x;
            const dy = goal.y - actor.y;
            const dist = Math.sqrt(dx*dx + dy*dy);

            if (dist > 0.1) {
                actor.x += Math.sign(dx) * Math.min(1, dist);
                actor.y += Math.sign(dy) * Math.min(1, dist);
            }

            // Check if at goal (within 1.5 units)
            const goalDist = Math.sqrt(Math.pow(goal.x - actor.x, 2) + Math.pow(goal.y - actor.y, 2));
            if (goalDist < 1.5) {
                // Drop cube at goal location
                actor.heldItem = null;
                state.winConditionMet = true;
            }

            return bt.running;
        })
    );

    // Register all actions with the PA-BT State
    // CRITICAL: Actions MUST be explicitly registered for PA-BT planner to use them
    // RegisterAction takes (name, action) - name is used for debugging/logging
    state.pabtState.RegisterAction('moveToCube1', moveToCube1Action);
    state.pabtState.RegisterAction('pickCube1', pickCube1Action);
    state.pabtState.RegisterAction('moveToGoal1', moveToGoal1Action);
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

    // Build cube positions (only non-deleted cubes)
    let cub1X = -1, cub1Y = -1, cub2X = -1, cub2Y = -1;
    const cube1 = state.cubes.get(1);
    const cube2 = state.cubes.get(2);

    if (cube1 && !cube1.deleted) {
        cub1X = Math.round(cube1.x);
        cub1Y = Math.round(cube1.y);
    }
    if (cube2 && !cube2.deleted) {
        cub2X = Math.round(cube2.x);
        cub2Y = Math.round(cube2.y);
    }

    // Held cube ID
    const held = actor.heldItem ? actor.heldItem.id : -1;

    // Win condition (as 0/1 for compactness)
    const win = state.winConditionMet ? 1 : 0;

    // ULTRA-compact: Use single-char keys
    // ~75 chars: {"m":"a","t":123,"x":10,"y":12,"h":1,"w":0,"a":25,"b":10,"d":25,"e":15}
    return JSON.stringify({
        m: state.gameMode === 'automatic' ? 'a' : 'm',
        t: state.tickCount,
        x: Math.round(actor.x),
        y: Math.round(actor.y),
        h: held,
        w: win,
        a: cub1X > -1 ? cub1X : undefined,
        b: cub1Y > -1 ? cub1Y : undefined,
        d: cub2X > -1 ? cub2X : undefined,
        e: cub2Y > -1 ? cub2Y : undefined
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
    const dist = Math.sqrt(dx*dx + dy*dy);
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
            Match: function(value) {
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
                    const dist = Math.sqrt(dx*dx + dy*dy);

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
