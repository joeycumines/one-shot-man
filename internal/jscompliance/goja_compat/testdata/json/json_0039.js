/*---
description: goja compat json 39
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":39}').x, 39, 'json parse 39');
