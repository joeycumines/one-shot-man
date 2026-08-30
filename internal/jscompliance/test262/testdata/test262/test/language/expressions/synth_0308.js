/*---
description: Synthetic test262 case 308 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":308}').x, 308, 'json 308');
