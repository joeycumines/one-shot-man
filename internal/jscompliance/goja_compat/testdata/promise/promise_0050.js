/*---
description: goja compat promise 50
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(50) instanceof Promise, true, 'promise instance 50'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 50');
