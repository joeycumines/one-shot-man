/*---
description: Synthetic test262 case 324 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":324}').x, 324, 'json 324');
