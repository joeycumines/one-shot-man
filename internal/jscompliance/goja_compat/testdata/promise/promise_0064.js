/*---
description: goja compat promise 64
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(64) instanceof Promise, true, 'promise instance 64'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 64');
