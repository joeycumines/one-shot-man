/*---
description: goja compat json 59
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":59}').x, 59, 'json parse 59');
