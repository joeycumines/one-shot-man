/*---
description: goja compat array 86
includes: [assert.js]
---*/
assert.sameValue([1,2,3].map(x=>x*2).length, 3, 'array map 86');
