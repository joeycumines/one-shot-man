/*---
description: goja compat json 32
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":32}').x, 32, 'json parse 32');
