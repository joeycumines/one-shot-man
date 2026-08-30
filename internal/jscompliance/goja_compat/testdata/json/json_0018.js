/*---
description: goja compat json 18
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":18}').x, 18, 'json parse 18');
