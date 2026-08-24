/*---
description: goja compat json 1
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":1}').x, 1, 'json parse 1');
