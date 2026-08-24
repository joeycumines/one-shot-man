/*---
description: goja compat json 27
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":27}').x, 27, 'json parse 27');
