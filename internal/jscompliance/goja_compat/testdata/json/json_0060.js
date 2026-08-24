/*---
description: goja compat json 60
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":60}').x, 60, 'json parse 60');
