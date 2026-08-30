/*---
description: Synthetic test262 case 36 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":36}').x, 36, 'json 36');
