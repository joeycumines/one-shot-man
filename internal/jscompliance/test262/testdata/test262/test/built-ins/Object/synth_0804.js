/*---
description: Synthetic test262 case 804 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":804}').x, 804, 'json 804');
