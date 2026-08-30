/*---
description: goja compat json 61
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":61}').x, 61, 'json parse 61');
