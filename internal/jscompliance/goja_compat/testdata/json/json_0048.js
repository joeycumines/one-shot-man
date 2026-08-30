/*---
description: goja compat json 48
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":48}').x, 48, 'json parse 48');
