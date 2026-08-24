/*---
description: goja compat json 62
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":62}').x, 62, 'json parse 62');
