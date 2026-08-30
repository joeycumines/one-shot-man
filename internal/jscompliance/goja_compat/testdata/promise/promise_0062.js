/*---
description: goja compat promise 62
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(62) instanceof Promise, true, 'promise instance 62'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 62');
