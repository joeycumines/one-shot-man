/*---
description: goja compat json 13
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":13}').x, 13, 'json parse 13');
