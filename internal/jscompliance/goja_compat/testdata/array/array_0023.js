/*---
description: goja compat array 23
includes: [assert.js]
---*/
assert.sameValue([1,2,3].map(x=>x*4).length, 3, 'array map 23');
