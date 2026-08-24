/*---
description: goja compat json 3
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":3}').x, 3, 'json parse 3');
