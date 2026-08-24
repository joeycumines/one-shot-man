/*---
description: goja compat json 23
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":23}').x, 23, 'json parse 23');
