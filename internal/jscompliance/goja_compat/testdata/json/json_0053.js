/*---
description: goja compat json 53
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":53}').x, 53, 'json parse 53');
