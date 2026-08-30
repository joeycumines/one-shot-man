/*---
description: Synthetic test262 case 212 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":212}').x, 212, 'json 212');
