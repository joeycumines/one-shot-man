/*---
description: goja compat promise 70
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(70) instanceof Promise, true, 'promise instance 70'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 70');
