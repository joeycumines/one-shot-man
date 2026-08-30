/*---
description: goja compat json 63
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":63}').x, 63, 'json parse 63');
