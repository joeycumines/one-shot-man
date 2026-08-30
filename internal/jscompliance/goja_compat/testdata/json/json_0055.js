/*---
description: goja compat json 55
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":55}').x, 55, 'json parse 55');
