/*---
description: Synthetic test262 case 444 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":444}').x, 444, 'json 444');
