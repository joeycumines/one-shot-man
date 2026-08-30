/*---
description: goja compat json 5
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":5}').x, 5, 'json parse 5');
