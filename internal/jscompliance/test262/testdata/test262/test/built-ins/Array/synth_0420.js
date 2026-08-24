/*---
description: Synthetic test262 case 420 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":420}').x, 420, 'json 420');
