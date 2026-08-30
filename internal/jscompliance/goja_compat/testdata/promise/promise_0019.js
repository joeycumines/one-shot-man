/*---
description: goja compat promise 19
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(19) instanceof Promise, true, 'promise instance 19'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 19');
