/*---
description: goja compat json 64
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":64}').x, 64, 'json parse 64');
