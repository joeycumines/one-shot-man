/*---
description: Synthetic test262 case 20 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":20}').x, 20, 'json 20');
