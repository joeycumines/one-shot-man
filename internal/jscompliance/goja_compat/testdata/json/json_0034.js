/*---
description: goja compat json 34
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":34}').x, 34, 'json parse 34');
