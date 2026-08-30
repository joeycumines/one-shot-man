/*---
description: goja compat json 65
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":65}').x, 65, 'json parse 65');
