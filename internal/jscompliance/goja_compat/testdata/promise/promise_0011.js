/*---
description: goja compat promise 11
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(11) instanceof Promise, true, 'promise instance 11'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 11');
