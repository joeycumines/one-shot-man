/*---
description: goja compat json 7
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":7}').x, 7, 'json parse 7');
