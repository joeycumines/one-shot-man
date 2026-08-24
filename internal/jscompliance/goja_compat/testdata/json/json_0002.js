/*---
description: goja compat json 2
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":2}').x, 2, 'json parse 2');
