/*---
description: goja compat json 28
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":28}').x, 28, 'json parse 28');
