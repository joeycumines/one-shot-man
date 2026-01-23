// Debug script to investigate collision detection failure
let bt = null;
let tea = null;
let pabt = null;
let os = null;
let vm = null;

try {
    // Mock modules if run directly with Node
    if (typeof require === 'function') {
        bt = { createLeafNode: fn => fn };
        tea = {
            newModel: cfg => ({ ...cfg }),
            quit: () => null,
            tick: () => null
        };
        pabt = { newAction: () => {} };
        os = { getenv: k => '' };
    } else {
        bt = require('osm:bt');
        tea = require('osm:bubbletea');
        pabt = require('osm:pabt');
        os = require('osm:os');
    }

    const TARGET_ID = 1;
    const GOAL_ID = 2;
    const GOAL_CENTER_X = 8;
    const GOAL_CENTER_Y = 18;
    const GOAL_RADIUS = 1;
    const ENV_WIDTH = 80;
    const ENV_HEIGHT = 24;
    const ROOM_MIN_X = 20;
    const ROOM_MAX_X = 55;
    const ROOM_MIN_Y = 6;
    const ROOM_MAX_Y = 16;
    const ROOM_GAP_Y = 11;

    // Simplified initialization for debugging
    function createTestState() {
        return {
            width: ENV_WIDTH,
            height: ENV_HEIGHT,
            spaceWidth: 60,
            actors: new Map([
                [1, {id: 1, x: 30, y: 12, heldItem: null}]
            ]),
            cubes: new Map(), // Start empty like test
            goals: new Map([]),
            manualKeys: new Map(),
            manualKeyLastSeen: new Map(),
        };
    }

    // The movement logic from the script
    function manualKeysMovement(state, actor) {
        if (!state.manualKeys || state.manualKeys.size === 0) return false;

        const getDir = () => {
            let dx = 0, dy = 0;
            if (state.manualKeys.get('w')) dy = -1;
            if (state.manualKeys.get('s')) dy = 1;
            if (state.manualKeys.get('a')) dx = -1;
            if (state.manualKeys.get('d')) dx = 1;
            return {dx, dy};
        };

        const {dx, dy} = getDir();

        if (dx === 0 && dy === 0) return false;

        console.log(`[DEBUG manualKeysMovement] dx=${dx}, dy=${dy}`);
        console.log(`[DEBUG manualKeysMovement] Actor at (${actor.x}, ${actor.y})`);
        console.log(`[DEBUG manualKeysMovement] Target position will be (${actor.x + dx}, ${actor.y + dy})`);

        const nx = actor.x + dx;
        const ny = actor.y + dy;
        let blocked = false;

        // Boundary check
        console.log(`[DEBUG] Boundary check: nx=${nx}, ny=${ny}`);
        console.log(`[DEBUG] spaceWidth=${state.spaceWidth}, height=${state.height}`);
        console.log(`[DEBUG] Math.round(nx)=${Math.round(nx)}, Math.round(ny)=${Math.round(ny)}`);

        if (Math.round(nx) < 0 || Math.round(nx) >= state.spaceWidth ||
            Math.round(ny) < 0 || Math.round(ny) >= state.height) {
            blocked = true;
            console.log(`[DEBUG] BLOCKED by boundary`);
        }

        // Fast collision check
        console.log(`[DEBUG] before collision check: blocked=${blocked}`);
        console.log(`[DEBUG] state.cubes has ${state.cubes.size} entries`);
        console.log(`[DEBUG] Iterating through state.cubes.values()...`);

        if (!blocked) {
            let cubeIndex = 0;
            for (const c of state.cubes.values()) {
                cubeIndex++;
                console.log(`[DEBUG] Cube ${cubeIndex}: id=${c.id}, x=${c.x}, y=${c.y}, deleted=${c.deleted}, type=${typeof c}`);
                console.log(`[DEBUG]   Math.round(c.x)=${Math.round(c.x)}, Math.round(c.y)=${Math.round(c.y)}`);
                console.log(`[DEBUG]   Math.round(nx)=${Math.round(nx)}, Math.round(ny)=${Math.round(ny)}`);
                console.log(`[DEBUG]   Comparing: ${Math.round(c.x)} === ${Math.round(nx)} && ${Math.round(c.y)} === ${Math.round(ny)}`);

                if (!c.deleted && Math.round(c.x) === Math.round(nx) && Math.round(c.y) === Math.round(ny)) {
                    console.log(`[DEBUG]   MATCH FOUND!`);
                    if (actor.heldItem && c.id === actor.heldItem.id) {
                        console.log(`[DEBUG]   But it's the held item, continuing...`);
                        continue;
                    }
                    blocked = true;
                    console.log(`[DEBUG] BLOCKED by cube`);
                    break;
                }
            }
            console.log(`[DEBUG] after iterating ${cubeIndex} cubes, blocked=${blocked}`);
        }

        console.log(`[DEBUG] Final blocked status: ${blocked}`);

        if (!blocked) {
            actor.x = nx;
            actor.y = ny;
            console.log(`[DEBUG] Moved actor to (${actor.x}, ${actor.y})`);
        } else {
            console.log(`[DEBUG] Actor stayed at (${actor.x}, ${actor.y})`);
        }
        return !blocked;
    }

    // Test the scenario
    console.log('=== TEST SCENARIO: Collision Detection Stops Movement ===');
    console.log();

    const state = createTestState();
    const actor = state.actors.get(1);

    console.log('Step 1: Initial state');
    console.log(`  Actor position: (${actor.x}, ${actor.y})`);
    console.log(`  Cubes in state: ${state.cubes.size}`);
    console.log();

    console.log('Step 2: Add cube 701 at (31, 12)');
    // Simulating what addCube does in the test
    const cube = {
        id: 701,
        x: 31,
        y: 12,
        deleted: false,
        type: 'obstacle',
        isStatic: false
    };
    state.cubes.set(701, cube);
    console.log(`  Added cube: id=${cube.id}, x=${cube.x}, y=${cube.y}, deleted=${cube.deleted}`);
    console.log(`  Total cubes: ${state.cubes.size}`);
    console.log();

    console.log('Step 3: Press "d" key (move right)');
    state.manualKeys.set('d', true);
    console.log(`  manualKeys has "d": ${state.manualKeys.get('d')}`);
    console.log(`  manualKeys size: ${state.manualKeys.size}`);
    console.log();

    console.log('Step 4: Execute manualKeysMovement');
    const moved = manualKeysMovement(state, actor);
    console.log();

    console.log('=== RESULTS ===');
    console.log(`  Movement occurred: ${moved}`);
    console.log(`  Final actor position: (${actor.x}, ${actor.y})`);
    console.log(`  Expected actor position: (30, 12) (blocked)`);
    console.log(`  Test ${moved ? 'FAIL' : 'PASS'}: Actor ${moved ? 'moved' : 'stayed'} as ${moved ? 'expected to be blocked' : 'expected'}`);
    console.log();

    if (moved && actor.x === 31) {
        console.log('=== BUG CONFIRMED ===');
        console.log('Collision check FAILED to block movement!');
    } else {
        console.log('=== NO BUG FOUND ===');
        console.log('Collision check worked correctly.');
    }

} catch (e) {
    console.error('Error:', e);
    process.exit(1);
}
