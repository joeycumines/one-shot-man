/*---
description: Synthetic test262 case 404 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue(JSON.parse('{"x":404}').x, 404, 'json 404');
