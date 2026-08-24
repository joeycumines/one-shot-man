/*---
description: Synthetic test262 case 484 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":484}').x, 484, 'json 484');
