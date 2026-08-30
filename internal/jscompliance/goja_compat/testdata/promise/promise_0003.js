/*---
description: goja compat promise 3
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(3) instanceof Promise, true, 'promise instance 3'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 3');
