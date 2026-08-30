/*---
description: goja compat json 58
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":58}').x, 58, 'json parse 58');
