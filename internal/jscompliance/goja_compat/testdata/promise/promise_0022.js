/*---
description: goja compat promise 22
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(22) instanceof Promise, true, 'promise instance 22'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 22');
