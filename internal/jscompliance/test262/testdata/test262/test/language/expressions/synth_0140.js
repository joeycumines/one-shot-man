/*---
description: Synthetic test262 case 140 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":140}').x, 140, 'json 140');
