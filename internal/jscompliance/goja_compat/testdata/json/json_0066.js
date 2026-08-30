/*---
description: goja compat json 66
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":66}').x, 66, 'json parse 66');
