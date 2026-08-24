/*---
description: goja compat json 24
includes: [assert.js]
---*/
assert.sameValue(JSON.parse('{"x":24}').x, 24, 'json parse 24');
