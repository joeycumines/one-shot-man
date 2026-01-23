// Mimic exact Go test behavior step by step

console.log('=== SIMULATING EXACT TEST SEQUENCE ===\n');

// Step 1: "Single Key Press Moves Actor Once"
console.log('TEST 1: Single Key Press Moves Actor Once');
console.log('  Actor reset to x=30, y=12');
console.log('  Press "w" key');
console.log('  manualKeys now contains: { "w": true }');
console.log('  Tick: actor moves from y=12 to y=11 (up)');
let actor = { x: 30, y: 12 };
let manualKeys = new Map();
manualKeys.set('w', true);
actor.y = 11;
console.log(`  Actor now at (${actor.x}, ${actor.y})`);
console.log(`  manualKeys is still: { "w": true } - NOT cleared!\n`);

// Step 2: "Key Hold Moves Continuously"
console.log('TEST 2: Key Hold Moves Continuously');
console.log('  Actor reset to x=30, y=12');
actor.x = 30;
actor.y = 12;
console.log('  Press "d" key');
manualKeys.set('d', true);
console.log(`  manualKeys now contains: { "w": true, "d": true } - w is STILL there!`);
console.log('  Tick: manualKeysMovement gets dx=1 (from d), dy=0 (no s), moves right');
actor.x = 31;
console.log('  Tick again: moves right again');
actor.x = 32;
console.log('  Tick again: moves right again');
actor.x = 33;
console.log('  Tick again: moves right again');
actor.x = 34;
console.log('  Tick again: moves right again');
actor.x = 35;
console.log(`  Actor now at (${actor.x}, ${actor.y})`);
console.log(`  manualKeys is STILL: { "w": true, "d": true } - NEITHER key cleared!\n`);

// Step 3: "Multiple Keys Pressed Diagonal Movement"
console.log('TEST 3: Multiple Keys Pressed Diagonal Movement');
console.log('  Actor reset to x=30, y=12');
actor.x = 30;
actor.y = 12;
console.log('  Press "w" key (already there from test 1, but pressed again so lastSeen updates)');
console.log('  manualKeys still has: { "w": true, "d": true }');
console.log('  Tick: manualKeysMovement gets dx=1 (from d), dy=-1 (from w), moves diagonal');
actor.x = 31;
actor.y = 11;
console.log(`  Actor now at (${actor.x}, ${actor.y})`);
console.log(`  manualKeys is STILL: { "w": true, "d": true }\n`);

// Step 4: "Collision Detection Stops Movement"
console.log('TEST 4: Collision Detection Stops Movement');
console.log('  Actor reset to x=30, y=12');
actor.x = 30;
actor.y = 12;
console.log('  Add cube 701 at (31, 12)');
const cube = { id: 701, x: 31, y: 12, deleted: false };
const cubes = new Map([[701, cube]]);
console.log('  Press "d" key');
manualKeys.set('d', true);
console.log(`  manualKeys now contains: { "w": true, "d": true } - w STILL there from test 1 & 3!`);
console.log();
console.log('  Tick: manualKeysMovement called');
console.log('    getDir() returns: dx=1 (from d), dy=-1 (from w) <-- THIS IS THE BUG!');
console.log('    nx = actor.x + dx = 30 + 1 = 31');
console.log('    ny = actor.y + dy = 12 + (-1) = 11');
console.log('    Collision check for position (31, 11):');
console.log('      Cube 701 is at (31, 12) - different position!');
console.log('      Math.round(31) === Math.round(31) ✓ - X matches');
console.log('      Math.round(12) === Math.round(11) ✗ - Y DOES NOT match!');
console.log('    Collision check PASSED (not blocked)');
console.log('    Actor moves to (31, 11)');
actor.x = 31;
actor.y = 11;
console.log();
console.log(`  Actor now at (${actor.x}, ${actor.y})`);
console.log('  Test expects actor at (30, 12) - TEST FAILS!\n');

console.log('=== ROOT CAUSE IDENTIFIED ===\n');
console.log('The manualKeys Map is never CLEARED between subtests!');
console.log('');
console.log('Test sequence builds up keys in manualKeys:');
console.log('  Test 1: Press "w" -> manualKeys = { "w" }');
console.log('  Test 2: Press "d" -> manualKeys = { "w", "d" }');
console.log('  Test 3: Press "w".+ -> manualKeys = { "w", "d" } (already there)');
console.log('  Test 4: Press "d".+ -> manualKeys = { "w", "d" } (already there)');
console.log('');
console.log('When Test 4 runs, it thinks both "w" and "d" are pressed!');
console.log('');
console.log('The collision check happens for TARGET POSITION (31, 11) not (31, 12)');
console.log('Because actor tries to move diagonally: +1 X, -1 Y');
console.log('');
console.log('Position (31, 11) is empty, so movement succeeds!');
console.log('Actor moves to (31, 11) instead of staying at (30, 12).');
console.log('Test checks actor.x === 30, but actor.x is now 31 -> FAIL');
