/*---
description: goja compat promise 29
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(29) instanceof Promise, true, 'promise instance 29'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 29');
