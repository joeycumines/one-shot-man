/*---
description: goja compat array 14
includes: [assert.js]
---*/
assert.sameValue([1,2,3].map(x=>x*5).length, 3, 'array map 14');
