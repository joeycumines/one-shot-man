/*---
description: goja compat promise 77
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(77) instanceof Promise, true, 'promise instance 77'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 77');
