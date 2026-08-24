/*---
description: goja compat json 51
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":51}').x, 51, 'json parse 51');
