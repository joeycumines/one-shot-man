/*---
description: goja compat json 43
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":43}').x, 43, 'json parse 43');
