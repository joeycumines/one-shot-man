/*---
description: goja compat json 8
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":8}').x, 8, 'json parse 8');
