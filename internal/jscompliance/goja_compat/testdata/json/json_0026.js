/*---
description: goja compat json 26
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":26}').x, 26, 'json parse 26');
