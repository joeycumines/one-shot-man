/*---
description: goja compat json 67
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":67}').x, 67, 'json parse 67');
