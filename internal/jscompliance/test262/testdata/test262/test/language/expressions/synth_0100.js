/*---
description: Synthetic test262 case 100 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":100}').x, 100, 'json 100');
