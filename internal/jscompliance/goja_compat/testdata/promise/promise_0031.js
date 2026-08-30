/*---
description: goja compat promise 31
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(31) instanceof Promise, true, 'promise instance 31'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 31');
