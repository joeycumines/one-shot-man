/*---
description: goja compat json 37
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":37}').x, 37, 'json parse 37');
