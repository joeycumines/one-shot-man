/*---
description: goja compat promise 18
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(18) instanceof Promise, true, 'promise instance 18'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 18');
