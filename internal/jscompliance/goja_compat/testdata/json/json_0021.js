/*---
description: goja compat json 21
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":21}').x, 21, 'json parse 21');
