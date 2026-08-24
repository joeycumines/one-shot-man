/*---
description: Synthetic test262 case 532 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":532}').x, 532, 'json 532');
