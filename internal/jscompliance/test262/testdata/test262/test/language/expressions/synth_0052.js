/*---
description: Synthetic test262 case 52 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":52}').x, 52, 'json 52');
