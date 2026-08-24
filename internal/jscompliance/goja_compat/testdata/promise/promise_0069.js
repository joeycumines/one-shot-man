/*---
description: goja compat promise 69
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(69) instanceof Promise, true, 'promise instance 69'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 69');
