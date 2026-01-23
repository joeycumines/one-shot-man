// Enhanced debug script to trace the actual issue
const fs = require('fs');
const path = require('path');

// Read the actual script to extract its logic
const scriptPath = path.join(__dirname, '../scripts/example-05-pick-and-place.js');
const scriptContent = fs.readFileSync(scriptPath, 'utf8');

// Remove shebang
if (scriptContent.startsWith('#!')) {
    const idx = scriptContent.indexOf('\n');
    scriptContent.slice(idx + 1);
}

// Parse out key constants
const constantsMatch = scriptContent.match(/ENV_WIDTH\s*=\s*(\d+)/);
const ENV_WIDTH = constantsMatch ? parseInt(constantsMatch[1]) : 80;

const widthMatch = scriptContent.match(/spaceWidth:\s*(\d+)/);
const SPACE_WIDTH = widthMatch ? parseInt(widthMatch[1]) : 60;

console.log('=== Parsed Constants ===');
console.log(`ENV_WIDTH: ${ENV_WIDTH}`);
console.log(`SPACE_WIDTH: ${SPACE_WIDTH}`);
console.log();

// Find TARGET_ID constant 
const targetIdMatch = scriptContent.match(/TARGET_ID\s*=\s*(\d+)|from\s+'target'\s*,\s*(\d+)/);
const TARGET_ID = targetIdMatch ? parseInt(targetIdMatch[1] || targetIdMatch[2]) : 1;
console.log(`TARGET_ID: ${TARGET_ID}`);
console.log();

// Find room configuration
const roomMinXMatch = scriptContent.match(/ROOM_MIN_X\s*=\s*(\d+)/);
const roomMaxXMatch = scriptContent.match(/ROOM_MAX_X\s*=\s*(\d+)/);
const roomMinYMatch = scriptContent.match(/ROOM_MIN_Y\s*=\s*(\d+)/);
const roomMaxYMatch = scriptContent.match(/ROOM_MAX_Y\s*=\s*(\d+)/);

console.log('=== Room Configuration ===');
console.log(`ROOM_MIN_X: ${roomMinXMatch?.[1]}`);
console.log(`ROOM_MAX_X: ${roomMaxXMatch?.[1]}`);
console.log(`ROOM_MIN_Y: ${roomMinYMatch?.[1]}`);
console.log(`ROOM_MAX_Y: ${roomMaxYMatch?.[1]}`);
console.log();

// Now let's trace through what initializeSimulation would create
console.log('=== Simulating initializeSimulation() ===');
console.log();

// Extract the initialization code section
const initMatch = scriptContent.match(/function initializeSimulation\(\)\s*\{([\s\S]+?)^    \}/m);
if (initMatch) {
    console.log('Found initializeSimulation function');
    console.log();
}

// Count initial cubes in script
const targetCubeMatch = scriptContent.match(/id:\s*TARGET_ID.*x:\s*(\d+).*y:\s*(\d+)/s);
console.log('Target cube position:', targetCubeMatch ? `(${targetCubeMatch[1]}, ${targetCubeMatch[2]})` : 'not found');

// Check room wall positions
const WALL_X_RANGE = roomMaxXMatch && roomMinXMatch 
    ? [parseInt(roomMinXMatch[1]), parseInt(roomMaxXMatch[1])] 
    : [null, null];
const WALL_Y_RANGE = roomMaxYMatch && roomMinYMatch
    ? [parseInt(roomMinYMatch[1]), parseInt(roomMaxYMatch[1])]
    : [null, null];

console.log('Room wall X range:', WALL_X_RANGE);
console.log('Room wall Y range:', WALL_Y_RANGE);
console.log();

// The test position
const testX = 31;
const testY = 12;

console.log('=== Test Setup ===');
console.log(`Actor position: (30, 12)`);
console.log(`Cube 701 position: (${testX}, ${testY})`);
console.log();

// Check if (31, 12) is in wall range
const inWallXRange = WALL_X_RANGE[0] !== null && testX >= WALL_X_RANGE[0] && testX <= WALL_X_RANGE[1];
const inWallYRange = WALL_Y_RANGE[0] !== null && testY >= WALL_Y_RANGE[0] && testY <= WALL_Y_RANGE[1];
const isOnWallPerimeter = (testX === WALL_X_RANGE[0] || testX === WALL_X_RANGE[1] || testY === WALL_Y_RANGE[0] || testY === WALL_Y_RANGE[1]);

console.log(`Is (31, 12) in wall X range [${WALL_X_RANGE[0]}, ${WALL_X_RANGE[1]}]? ${inWallXRange}`);
console.log(`Is (31, 12) in wall Y range [${WALL_Y_RANGE[0]}, ${WALL_Y_RANGE[1]}]? ${inWallYRange}`);
console.log(`Is (31, 12) on wall perimeter? ${isOnWallPerimeter}`);
console.log();

console.log('=== HYPOTHESIS ===');
console.log('If the actor can move from (30, 12) to (31, 12) despite cube 701 at (31, 12):');
console.log();
console.log('POSSIBLE CAUSE 1: Key press not being registered in state.manualKeys');
console.log('  - The test sends {"type": "Key", "key": "d"}');
console.log('  - The script checks if state.manualKeys.get("d") returns true');
console.log('  - Check: Does the update function properly set manualKeys when gameMode is "manual"?');
console.log();
console.log('POSSIBLE CAUSE 2: The actor being retrieved is wrong');
console.log('  - Test uses actor := getActor() which gets state.actors.get(state.activeActorId)');
console.log('  - Script uses state.actors.get(state.activeActorId)');
console.log('  - These should be the same unless activeActorId changed');
console.log();
console.log('POSSIBLE CAUSE 3: manualKeysMovement is not being called');
console.log('  - The call is inside the Tick handler');
console.log('  - There might be an early return or condition preventing it');
console.log();
console.log('POSSIBLE CAUSE 4: The cube added by addCube is not in state.cubes.values()');
console.log('  - The test uses cubes.set(id, cube) where cubes := state.Get("cubes")');
console.log('  - The script loops for (const c of state.cubes.values())');
console.log('  - These should access the same Map');
console.log();
console.log('POSSIBLE CAUSE 5: The Test Setup Issue');
console.log('  - All subtests share the same actor, state, and vm');
console.log('  - Previous tests might have left state.cubes in a weird state');
console.log('  - The test does NOT clear cubes before adding cube 701');
console.log('  - Previous tests might have modified manualKeys or other state');
console.log();
console.log('=== MOST LIKELY CAUSE ===');
console.log('Looking at the test structure: all T13 subtests share the same setup');
console.log('The collision test runs AFTER other tests that modified the state');
console.log();
console.log('Previous tests: ');
console.log('  1. Single Key Press Moves Actor Once - moves actor, tests tick');
console.log('  2. Key Hold Moves Continuously - presses D multiple ticks');
console.log('  3. Multiple Keys Pressed Diagonal Movement - presses W and D');
console.log();
console.log('These tests all use actor.Set("x", 30) and actor.Set("y", 12)');
console.log('BUT they never clear state.cubes - they inherit the cubes from initializeSimulation()');
console.log();
console.log('The collision test also uses actor.Set("x", 30) and actor.Set("y", 12)');
console.log('It adds cube 701 at (31, 12)');
console.log();
console.log('Wait... let me check what happens in the Key message handler...');
