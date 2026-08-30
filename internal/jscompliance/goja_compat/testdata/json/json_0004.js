/*---
description: goja compat json 4
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":4}').x, 4, 'json parse 4');
