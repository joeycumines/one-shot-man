/*---
description: goja compat promise 23
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(23) instanceof Promise, true, 'promise instance 23'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 23');
