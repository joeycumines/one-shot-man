/*---
description: goja compat json 33
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":33}').x, 33, 'json parse 33');
