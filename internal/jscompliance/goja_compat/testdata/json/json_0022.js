/*---
description: goja compat json 22
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":22}').x, 22, 'json parse 22');
