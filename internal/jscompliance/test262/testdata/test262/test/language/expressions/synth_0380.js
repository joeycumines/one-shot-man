/*---
description: Synthetic test262 case 380 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":380}').x, 380, 'json 380');
