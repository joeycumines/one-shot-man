/*---
description: Synthetic test262 case 732 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":732}').x, 732, 'json 732');
