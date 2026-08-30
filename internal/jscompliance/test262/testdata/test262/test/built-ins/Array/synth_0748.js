/*---
description: Synthetic test262 case 748 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":748}').x, 748, 'json 748');
