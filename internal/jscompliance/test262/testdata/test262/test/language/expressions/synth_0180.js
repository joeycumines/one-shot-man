/*---
description: Synthetic test262 case 180 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":180}').x, 180, 'json 180');
