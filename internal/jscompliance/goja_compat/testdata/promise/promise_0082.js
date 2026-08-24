/*---
description: goja compat promise 82
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(82) instanceof Promise, true, 'promise instance 82'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 82');
