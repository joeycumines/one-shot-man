/*---
description: goja compat promise 10
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(10) instanceof Promise, true, 'promise instance 10'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 10');
