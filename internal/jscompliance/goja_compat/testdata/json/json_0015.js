/*---
description: goja compat json 15
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":15}').x, 15, 'json parse 15');
