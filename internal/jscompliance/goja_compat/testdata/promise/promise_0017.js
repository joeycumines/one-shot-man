/*---
description: goja compat promise 17
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(17) instanceof Promise, true, 'promise instance 17'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 17');
