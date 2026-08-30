/*---
description: goja compat json 42
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":42}').x, 42, 'json parse 42');
