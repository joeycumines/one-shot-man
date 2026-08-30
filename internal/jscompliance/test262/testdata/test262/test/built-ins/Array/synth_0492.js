/*---
description: Synthetic test262 case 492 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":492}').x, 492, 'json 492');
