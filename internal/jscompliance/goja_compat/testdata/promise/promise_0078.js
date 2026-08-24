/*---
description: goja compat promise 78
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(78) instanceof Promise, true, 'promise instance 78'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 78');
