/*---
description: goja compat promise 2
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(2) instanceof Promise, true, 'promise instance 2'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 2');
