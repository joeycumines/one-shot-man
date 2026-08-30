/*---
description: goja compat promise 20
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(20) instanceof Promise, true, 'promise instance 20'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 20');
