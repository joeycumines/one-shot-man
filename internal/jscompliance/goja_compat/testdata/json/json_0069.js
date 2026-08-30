/*---
description: goja compat json 69
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":69}').x, 69, 'json parse 69');
