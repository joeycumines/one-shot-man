/*---
description: goja compat json 57
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":57}').x, 57, 'json parse 57');
