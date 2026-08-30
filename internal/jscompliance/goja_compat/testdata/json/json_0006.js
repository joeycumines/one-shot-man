/*---
description: goja compat json 6
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":6}').x, 6, 'json parse 6');
