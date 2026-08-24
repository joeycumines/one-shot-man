/*---
description: goja compat json 47
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":47}').x, 47, 'json parse 47');
