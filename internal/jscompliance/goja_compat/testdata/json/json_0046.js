/*---
description: goja compat json 46
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":46}').x, 46, 'json parse 46');
