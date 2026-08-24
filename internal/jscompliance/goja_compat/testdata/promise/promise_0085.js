/*---
description: goja compat promise 85
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(85) instanceof Promise, true, 'promise instance 85'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 85');
