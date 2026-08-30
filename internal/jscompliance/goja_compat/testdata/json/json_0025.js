/*---
description: goja compat json 25
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":25}').x, 25, 'json parse 25');
