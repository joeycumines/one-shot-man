/*---
description: Synthetic test262 case 412 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":412}').x, 412, 'json 412');
