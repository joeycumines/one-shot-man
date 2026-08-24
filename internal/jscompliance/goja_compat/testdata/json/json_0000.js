/*---
description: goja compat json 0
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":0}').x, 0, 'json parse 0');
