/*---
description: goja compat promise 49
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(49) instanceof Promise, true, 'promise instance 49'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 49');
