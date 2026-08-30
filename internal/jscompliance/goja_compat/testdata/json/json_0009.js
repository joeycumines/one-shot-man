/*---
description: goja compat json 9
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":9}').x, 9, 'json parse 9');
