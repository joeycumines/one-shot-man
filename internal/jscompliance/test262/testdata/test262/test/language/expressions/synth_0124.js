/*---
description: Synthetic test262 case 124 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":124}').x, 124, 'json 124');
