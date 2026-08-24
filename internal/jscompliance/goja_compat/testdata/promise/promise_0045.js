/*---
description: goja compat promise 45
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(45) instanceof Promise, true, 'promise instance 45'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 45');
