/*---
description: goja compat json 16
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":16}').x, 16, 'json parse 16');
