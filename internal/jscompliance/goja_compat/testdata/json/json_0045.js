/*---
description: goja compat json 45
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":45}').x, 45, 'json parse 45');
