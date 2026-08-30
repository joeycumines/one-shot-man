/*---
description: Synthetic test262 case 700 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":700}').x, 700, 'json 700');
