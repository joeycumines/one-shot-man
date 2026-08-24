/*---
description: goja compat promise 56
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(56) instanceof Promise, true, 'promise instance 56'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 56');
