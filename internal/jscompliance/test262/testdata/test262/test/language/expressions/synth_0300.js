/*---
description: Synthetic test262 case 300 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":300}').x, 300, 'json 300');
