/*---
description: goja compat json 30
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":30}').x, 30, 'json parse 30');
