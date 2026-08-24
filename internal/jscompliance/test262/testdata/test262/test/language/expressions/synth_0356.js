/*---
description: Synthetic test262 case 356 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":356}').x, 356, 'json 356');
