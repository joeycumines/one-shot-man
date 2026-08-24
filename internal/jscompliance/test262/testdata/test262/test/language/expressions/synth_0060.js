/*---
description: Synthetic test262 case 60 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":60}').x, 60, 'json 60');
