/*---
description: goja compat promise 37
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(37) instanceof Promise, true, 'promise instance 37'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 37');
