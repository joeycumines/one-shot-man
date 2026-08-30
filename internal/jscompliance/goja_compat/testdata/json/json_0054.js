/*---
description: goja compat json 54
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":54}').x, 54, 'json parse 54');
