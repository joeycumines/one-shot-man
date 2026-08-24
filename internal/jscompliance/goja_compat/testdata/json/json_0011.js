/*---
description: goja compat json 11
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":11}').x, 11, 'json parse 11');
