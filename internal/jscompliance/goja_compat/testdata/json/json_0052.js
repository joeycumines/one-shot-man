/*---
description: goja compat json 52
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":52}').x, 52, 'json parse 52');
