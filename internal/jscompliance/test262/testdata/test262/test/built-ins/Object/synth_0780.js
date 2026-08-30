/*---
description: Synthetic test262 case 780 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":780}').x, 780, 'json 780');
