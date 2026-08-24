/*---
description: Synthetic test262 case 132 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":132}').x, 132, 'json 132');
