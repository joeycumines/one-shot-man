/*---
description: Synthetic test262 case 468 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":468}').x, 468, 'json 468');
