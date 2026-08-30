/*---
description: Synthetic test262 case 756 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":756}').x, 756, 'json 756');
