/*---
description: goja compat array 92
includes: [assert.js]
---*/
assert.sameValue([1,2,3].map(x=>x*3).length, 3, 'array map 92');
