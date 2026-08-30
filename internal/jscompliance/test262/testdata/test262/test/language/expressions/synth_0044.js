/*---
description: Synthetic test262 case 44 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":44}').x, 44, 'json 44');
