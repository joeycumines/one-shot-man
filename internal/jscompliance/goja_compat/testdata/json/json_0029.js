/*---
description: goja compat json 29
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":29}').x, 29, 'json parse 29');
