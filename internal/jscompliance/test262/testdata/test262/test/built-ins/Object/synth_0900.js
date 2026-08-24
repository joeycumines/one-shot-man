/*---
description: Synthetic test262 case 900 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":900}').x, 900, 'json 900');
