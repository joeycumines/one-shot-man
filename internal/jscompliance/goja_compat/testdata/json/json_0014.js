/*---
description: goja compat json 14
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":14}').x, 14, 'json parse 14');
