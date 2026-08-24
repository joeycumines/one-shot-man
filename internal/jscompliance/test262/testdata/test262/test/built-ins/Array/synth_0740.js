/*---
description: Synthetic test262 case 740 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":740}').x, 740, 'json 740');
