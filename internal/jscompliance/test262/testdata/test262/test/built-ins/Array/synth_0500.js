/*---
description: Synthetic test262 case 500 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":500}').x, 500, 'json 500');
