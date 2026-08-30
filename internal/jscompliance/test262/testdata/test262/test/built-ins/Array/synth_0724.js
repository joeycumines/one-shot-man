/*---
description: Synthetic test262 case 724 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":724}').x, 724, 'json 724');
