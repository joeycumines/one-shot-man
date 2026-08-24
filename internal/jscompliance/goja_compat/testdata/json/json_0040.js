/*---
description: goja compat json 40
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":40}').x, 40, 'json parse 40');
