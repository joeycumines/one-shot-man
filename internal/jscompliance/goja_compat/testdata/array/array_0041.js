/*---
description: goja compat array 41
includes: [assert.js]
---*/
assert.sameValue([1,2,3].map(x=>x*2).length, 3, 'array map 41');
