/*---
description: goja compat json 44
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":44}').x, 44, 'json parse 44');
