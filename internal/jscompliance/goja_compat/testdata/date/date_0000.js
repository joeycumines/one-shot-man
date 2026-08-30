/*---
description: goja compat date 0
includes: [assert.js]
---*/
assert.sameValue(new Date(0).getTime(), 0, 'date 0');
