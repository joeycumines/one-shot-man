/*---
description: goja compat json 56
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":56}').x, 56, 'json parse 56');
