/*---
description: Synthetic test262 case 12 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":12}').x, 12, 'json 12');
