/*---
description: goja compat json 68
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":68}').x, 68, 'json parse 68');
