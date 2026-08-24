/*---
description: goja compat promise 68
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(68) instanceof Promise, true, 'promise instance 68'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 68');
