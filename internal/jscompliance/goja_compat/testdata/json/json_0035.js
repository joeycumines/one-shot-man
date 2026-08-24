/*---
description: goja compat json 35
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":35}').x, 35, 'json parse 35');
