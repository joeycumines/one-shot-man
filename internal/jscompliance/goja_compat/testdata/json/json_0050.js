/*---
description: goja compat json 50
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":50}').x, 50, 'json parse 50');
