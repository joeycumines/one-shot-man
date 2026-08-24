/*---
description: goja compat promise 0
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(0) instanceof Promise, true, 'promise instance 0'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 0');
