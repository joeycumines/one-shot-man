/*---
description: goja compat array 11
includes: [assert.js]
---*/
assert.sameValue([1,2,3].map(x=>x*2).length, 3, 'array map 11');
