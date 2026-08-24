/*---
description: goja compat promise 42
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(42) instanceof Promise, true, 'promise instance 42'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 42');
