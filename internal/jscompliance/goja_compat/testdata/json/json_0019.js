/*---
description: goja compat json 19
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":19}').x, 19, 'json parse 19');
