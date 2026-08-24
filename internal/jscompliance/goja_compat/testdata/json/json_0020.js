/*---
description: goja compat json 20
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":20}').x, 20, 'json parse 20');
