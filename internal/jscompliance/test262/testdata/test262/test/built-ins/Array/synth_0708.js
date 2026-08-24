/*---
description: Synthetic test262 case 708 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":708}').x, 708, 'json 708');
