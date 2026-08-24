/*---
description: goja compat json 49
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":49}').x, 49, 'json parse 49');
