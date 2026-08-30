/*---
description: goja compat promise 54
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(54) instanceof Promise, true, 'promise instance 54'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 54');
