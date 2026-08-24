/*---
description: Synthetic test262 case 876 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":876}').x, 876, 'json 876');
