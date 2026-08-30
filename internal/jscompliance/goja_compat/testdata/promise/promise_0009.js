/*---
description: goja compat promise 9
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(9) instanceof Promise, true, 'promise instance 9'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 9');
