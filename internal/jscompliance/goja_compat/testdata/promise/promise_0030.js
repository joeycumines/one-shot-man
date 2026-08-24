/*---
description: goja compat promise 30
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(30) instanceof Promise, true, 'promise instance 30'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 30');
