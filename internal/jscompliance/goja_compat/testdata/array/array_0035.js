/*---
description: goja compat array 35
includes: [assert.js]
---*/
assert.sameValue([1,2,3].map(x=>x*1).length, 3, 'array map 35');
