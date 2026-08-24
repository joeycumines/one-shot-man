/*---
description: Synthetic test262 case 4 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":4}').x, 4, 'json 4');
