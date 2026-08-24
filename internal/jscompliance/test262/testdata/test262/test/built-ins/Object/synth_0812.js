/*---
description: Synthetic test262 case 812 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":812}').x, 812, 'json 812');
