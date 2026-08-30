/*---
description: goja compat promise 89
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(89) instanceof Promise, true, 'promise instance 89'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 89');
