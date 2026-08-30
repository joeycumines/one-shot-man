/*---
description: goja compat json 12
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":12}').x, 12, 'json parse 12');
