/*---
description: Synthetic test262 case 980 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":980}').x, 980, 'json 980');
