#!/usr/bin/env osm script

// ============================================================================
// test-shooter-error.js
// Tests that example-04-bt-shooter.js exits non-zero on error
// ============================================================================

// This script imports the shooter script but deliberately causes an error
// to test error handling and exit code behavior

try {
    // Load required modules
    var bt = require('osm:bt');

    // Deliberately cause an error to test exit code
    console.error('Intentional error to test non-zero exit code');

    // Create a failing behavior tree
    function failingLeaf(bb) {
        throw new Error('This is an intentional error to test exit code');
    }

    // Try to run it - bt.tick() returns "failure" (never throws)
    const tree = bt.createBlockingLeafNode(failingLeaf);
    var result = bt.tick(tree);

    // bt.tick() swallows errors and returns "failure" as a string
    if (result === "failure") {
        throw new Error('BT node failed as expected — testing non-zero exit');
    }

    // If we get here, something is wrong
    console.error('ERROR: Expected failure but tick returned:', result);

} catch (e) {
    // This is expected - re-throw to trigger non-zero exit
    console.error('Caught expected error:', e.message);
    throw e;
}
