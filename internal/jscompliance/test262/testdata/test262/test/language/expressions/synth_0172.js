/*---
description: Synthetic test262 case 172 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":172}').x, 172, 'json 172');
