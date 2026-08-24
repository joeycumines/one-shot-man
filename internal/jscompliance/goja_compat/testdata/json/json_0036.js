/*---
description: goja compat json 36
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":36}').x, 36, 'json parse 36');
