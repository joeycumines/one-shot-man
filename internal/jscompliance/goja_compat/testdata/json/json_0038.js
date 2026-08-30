/*---
description: goja compat json 38
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":38}').x, 38, 'json parse 38');
