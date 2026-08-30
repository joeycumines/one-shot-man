/*---
description: goja compat json 41
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":41}').x, 41, 'json parse 41');
