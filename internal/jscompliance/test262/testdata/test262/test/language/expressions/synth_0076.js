/*---
description: Synthetic test262 case 76 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":76}').x, 76, 'json 76');
