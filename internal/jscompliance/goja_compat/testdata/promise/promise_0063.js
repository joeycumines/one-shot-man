/*---
description: goja compat promise 63
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(63) instanceof Promise, true, 'promise instance 63'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 63');
