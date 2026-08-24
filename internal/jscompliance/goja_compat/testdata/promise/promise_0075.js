/*---
description: goja compat promise 75
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(75) instanceof Promise, true, 'promise instance 75'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 75');
