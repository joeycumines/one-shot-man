/*---
description: Synthetic test262 case 788 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":788}').x, 788, 'json 788');
