/*---
description: goja compat json 31
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":31}').x, 31, 'json parse 31');
