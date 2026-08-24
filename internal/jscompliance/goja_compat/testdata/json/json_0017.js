/*---
description: goja compat json 17
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":17}').x, 17, 'json parse 17');
