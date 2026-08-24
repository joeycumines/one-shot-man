/*---
description: goja compat json 10
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":10}').x, 10, 'json parse 10');
